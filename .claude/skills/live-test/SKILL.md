---
name: live-test
description: Run the real API locally (Postgres, MinIO, background jobs) and drive a feature end-to-end over HTTP with curl, then check the rows and stored objects it left. Use before merging a pull request, or when the user asks to "test live", "check it works locally", or wants proof beyond the Go tests.
---

# Live testing the API locally

The Go tests call handlers through `humatest` with fakes for other modules. A live test runs the
real binary from `cmd/api`, with real jobs, real MinIO and the real migrations, and drives it over
HTTP. Run one before a pull request is merged, for every endpoint and job the pull request touches.

## 1. Start Postgres and MinIO

`mise run db:up` starts `app_postgres` on port 5432 and MinIO on 9000. Check first that nothing
else holds those ports; another project's containers often do:

```bash
lsof -iTCP:5432 -sTCP:LISTEN -n -P; lsof -iTCP:9000 -sTCP:LISTEN -n -P
```

If they are free, use `mise run db:up` and `docker compose up -d minio minio-init`.
If they are taken, do not stop the other project's containers. Run this project's containers on
free ports with the same volumes:

```bash
docker rm -f app_postgres 2>/dev/null
docker run -d --name uniezz-pg-5433 -p 5433:5432 \
  -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgrespassword -e POSTGRES_DB=app_dev \
  -v uniezz-backend_pgdata:/var/lib/postgresql/data postgres:17-alpine
docker run -d --name uniezz-minio-9100 -p 9100:9000 \
  -v uniezz-backend_miniodata:/data cgr.dev/chainguard/minio:latest server /data
docker run --rm --add-host=host.docker.internal:host-gateway \
  -e MC_HOST_local=http://minioadmin:minioadmin@host.docker.internal:9100 \
  cgr.dev/chainguard/minio-client:latest mb --ignore-existing local/uniezz-media
```

The rest of this file uses ports 5433 and 9100; use 5432 and 9000 if `db:up` worked.

## 2. Migrate, including down and up

```bash
DB="postgres://postgres:postgrespassword@localhost:5433/app_dev?sslmode=disable"
mise exec -- goose -dir migrations postgres "$DB" up
# For each new migration of the pull request: down, then up again.
mise exec -- goose -dir migrations postgres "$DB" down
mise exec -- goose -dir migrations postgres "$DB" up
```

## 3. Run the API

Run it on the host from the working tree, with secrets from Doppler and local storage, as a
background command (`run_in_background: true`):

```bash
doppler run -c dev -- env \
  DATABASE_URL="$DB" PORT=8081 ENV=development \
  S3_BUCKET=uniezz-media S3_REGION=us-east-1 S3_FORCE_PATH_STYLE=true \
  S3_ACCESS_KEY_ID=minioadmin S3_SECRET_ACCESS_KEY=minioadmin \
  S3_ENDPOINT=http://localhost:9100 S3_PUBLIC_ENDPOINT=http://localhost:9100 \
  JOB_POLL_INTERVAL=2s \
  go run ./cmd/api > "$SCRATCHPAD/live-api.log" 2>&1
```

Wait for `curl -s -o /dev/null -w '%{http_code}' localhost:8081/healthz/live` to return `200`.
`JOB_POLL_INTERVAL=2s` makes queued jobs run within seconds. Without Doppler, put the same values
in `.env` (see `.env.example`).

## 4. Sign in without email or USOS

Create the student in SQL, then insert a one-time login code and exchange it through the real
`POST /auth/exchange`. The code is a base64url string; `login_codes.code_hash` is the SHA-256 of
its raw bytes:

```bash
PSQL="docker exec -i uniezz-pg-5433 psql -U postgres -d app_dev -tA"

login() { # $1 email; prints "<user id> <session token>"
  local uid raw code hash tok
  uid=$($PSQL -c "INSERT INTO users (university_id, email, first_name, last_name)
                  VALUES ('umcs', '$1', 'Live', 'Test') RETURNING id" | head -1)
  raw=$(openssl rand -hex 32)
  code=$(echo -n "$raw" | xxd -r -p | base64 | tr '+/' '-_' | tr -d '=')
  hash=$(echo -n "$raw" | xxd -r -p | openssl dgst -sha256 -hex | awk '{print $NF}')
  $PSQL -c "INSERT INTO login_codes (code_hash, user_id) VALUES ('\\x$hash', '$uid')" >/dev/null
  tok=$(curl -s -X POST localhost:8081/auth/exchange -H 'Content-Type: application/json' \
        -d "{\"code\":\"$code\"}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])')
  echo "$uid $tok"
}
```

Give every email a per-run marker (`live-$(date +%s)-a@umcs.pl`) so cleanup can find the rows.
Use two students when the feature has another student's view (likes, reports, blocks, profiles).

## 5. Upload media the way a client does

`POST /media/uploads` returns `uploadUrl` and the exact `headers` to send. The PUT must carry
all of them, `If-None-Match: *` included, or MinIO rejects it and later claims fail with 400:

```bash
up=$(curl -s -X POST localhost:8081/media/uploads -H "Authorization: Bearer $TOKEN" \
     -H 'Content-Type: application/json' -d "{\"kind\":\"avatar\",\"contentType\":\"image/png\",\"size\":$SIZE}")
curl -s -X PUT "$(echo "$up" | python3 -c 'import sys,json;print(json.load(sys.stdin)["uploadUrl"])')" \
     -H 'Content-Type: image/png' -H 'If-None-Match: *' --data-binary @file.png
```

## 6. Drive the feature and check the effects

Write the scenario as a bash script in the scratchpad and run it with `bash`. macOS ships bash
3.2, which mis-parses JSON with escaped quotes inside `"$( ... )"`; build request bodies in a
variable first (`BODY='{"targetId":"'$ID'"}'`) and pass `-d "$BODY"`.

For each changed endpoint check the status code, then the side effects the response does not
show:

- rows: query them with `$PSQL`, filtered by the ids the run created;
- stored objects: list the bucket and look for the `object_key`s from `media`:
  ```bash
  docker run --rm --add-host=host.docker.internal:host-gateway \
    -e MC_HOST_local=http://minioadmin:minioadmin@host.docker.internal:9100 \
    cgr.dev/chainguard/minio-client:latest ls -r local/uniezz-media
  ```
- queued jobs: `SELECT kind, status, attempts, last_error FROM jobs WHERE payload->>'userId' = '…'`.
  A scheduled job (daily cleanups) does not run on demand; insert a row to run it now:
  `INSERT INTO jobs (kind, payload) VALUES ('<kind>', '{}')`, then wait a few poll intervals.
- the log: `grep '"level":"ERROR"' "$SCRATCHPAD/live-api.log"` should print nothing.

Report each check with its observed value, not only "works".

## 7. Clean up, every time

- Delete the rows the run created, by its marker or ids, children before parents (reports,
  likes, ratings, media, then users). Never a bare `DELETE FROM <table>`.
- Stop the API: `lsof -iTCP:8081 -sTCP:LISTEN -n -P`, then `kill <pid>`. `go run` leaves the
  built binary running after its wrapper exits.
- Leave the database containers running unless the user asks otherwise, and tell them which
  ports they are on.
