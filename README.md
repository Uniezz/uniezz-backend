# Uniezz Backend

Go HTTP API for [Uniezz](docs/product-description/PRODUCT_DESCRIPTION.en.md), a platform for students of Lublin universities.

## Stack

| Concern              | Choice                                                               |
| -------------------- | -------------------------------------------------------------------- |
| Language             | Go 1.27.1 (pinned by mise)                                           |
| Router               | [chi](https://github.com/go-chi/chi) v5                              |
| Database             | PostgreSQL 17 via [pgx](https://github.com/jackc/pgx) v5 (`pgxpool`) |
| Migrations           | [goose](https://github.com/pressly/goose) v3                         |
| Toolchain / tasks    | [mise](https://mise.jdx.dev)                                         |
| Local infrastructure | Docker Compose                                                       |

## Requirements

- [Git](https://git-scm.com) — on Windows, Git for Windows (it also provides the `bash` the git hooks run under)
- [mise](https://mise.jdx.dev/getting-started.html) — installs Go, goose, and golangci-lint at the pinned versions
- Docker with Compose 2.32 or newer — runs the whole development stack; Docker Desktop on Windows/macOS. The engine must be running.
- Secrets, one of:
  - [Doppler CLI](https://docs.doppler.com/docs/install-cli) with access to the `uniezz-backend` project (ask a team member), or
  - a `.env` file in the repository root, if you have no Doppler seat. See [Without Doppler](#without-doppler).

Running the stack needs no Go or goose on the host: they run in the containers. mise still installs them for `fmt`, `lint`, `test` and `migrate:create`.

## Getting started

One-time setup, once per clone:

```sh
# 1. Install the pinned toolchain
mise install

# 2. Point git at the shared hooks (required)
mise run setup

# 3. Log in to Doppler and link this directory to the dev config
doppler login
doppler setup -p uniezz-backend -c dev
```

Everyday workflow:

```sh
mise run dev     # without Doppler: mise run dev:env
# edit code: the API rebuilds and restarts by itself
# Ctrl+C stops everything
```

`mise run dev` runs `docker compose up --build --watch` under `doppler run -c dev`. It:

1. builds the development image (`Dockerfile.dev`: Go and goose, pinned to the same versions as `mise.toml`);
2. starts PostgreSQL and waits for its healthcheck;
3. runs `goose up` in the `migrate` container. If a migration fails, the API does not start and Compose prints goose's error;
4. starts the API from the mounted source on port `8080`;
5. watches `cmd/`, `internal/`, `go.mod` and `go.sum`, and restarts the API on every change. A restart takes about 6 seconds, because the API drains for 5 seconds before shutting down. Changes to test files do not trigger a restart.

Ctrl+C stops the API (with its graceful shutdown), then PostgreSQL. The database lives in the `pgdata` volume and survives restarts. New migrations are applied on the next `mise run dev`, or right away with `mise run migrate:up`. To start from an empty database, run `mise run dev:reset`.

### Without Doppler

Doppler's free plan has three seats. Without a seat, keep the secrets in `.env` instead:

1. Copy `.env.example` to `.env` in the repository root and fill in the USOS keys. Ask a team member for the values. `.env` is git-ignored; never commit it.
2. Skip step 3 of the one-time setup.
3. Use the `:env` variants of the tasks that need secrets: `mise run dev:env` and `mise run test:env`. The other tasks need no secrets and work as they are.

Compose reads `.env` from the repository root by itself, and mise loads it into the environment of every task. The file must be named exactly `.env`; nothing reads `.env.local`.

> **Every new developer must run `mise run setup` after cloning the repository.** It points `core.hooksPath` at `.githooks`, so the shared git hooks are active for your clone. Git does not share hooks automatically — skip this step and the hooks will never run for you.

Check that the API is up:

```sh
curl -i http://localhost:8080/healthz/ready
```

### Windows from scratch

All commands below are for PowerShell.

1. Install the tools with winget, then **open a new terminal** so `PATH` picks them up:

   ```powershell
   winget install Git.Git
   winget install jdx.mise
   winget install Doppler.doppler   # skip without a Doppler seat
   winget install Docker.DockerDesktop
   ```

2. Start Docker Desktop and wait until it reports that the engine is running. Otherwise every Docker task fails with `open //./pipe/dockerDesktopLinuxEngine: The system cannot find the file specified`.

3. (Optional) To run `go`, `goose`, and `golangci-lint` directly, outside `mise run`, add the mise shims to your user `PATH` and open a new terminal:

   ```powershell
   [Environment]::SetEnvironmentVariable('PATH', "$env:LOCALAPPDATA\mise\shims;" + [Environment]::GetEnvironmentVariable('PATH', 'User'), 'User')
   ```

4. Clone the repository and follow the [Getting started](#getting-started) steps above. They are identical on Windows.

Windows notes:

- **mise runs tasks with `cmd.exe`.** `cmd.exe` does not expand `$VAR`, so never reference env vars as `$VAR` in a task's `run` line. Let the tool read the variable itself. For example, goose reads `GOOSE_DBSTRING`, `GOOSE_DRIVER`, and `GOOSE_MIGRATION_DIR`. For task arguments, use `{{usage.<name>}}`.
- **Do not wrap `mise run` in `doppler run`.** The tasks already inject secrets.
- **Port 5432 or 8080 already in use?** Compose fails with `ports are not available` or `port is already allocated`. For 5432, a locally installed PostgreSQL service is the usual cause: stop it in `services.msc`, or change the host port in `compose.yaml` and in `DATABASE_URL` in Doppler or `.env`. For 8080, stop whatever else is listening there, such as an API started with `go run` outside the stack.
- **`'doppler' is not recognized`?** The Doppler CLI is not installed or not on `PATH`. Install it, or follow [Without Doppler](#without-doppler).

## Configuration

Secrets and env vars live in Doppler (project `uniezz-backend`, configs `dev` / `stg` / `prd`) and are injected by `doppler run`. Developers without Doppler keep them in `.env`. mise loads `.env` if one exists (`[env] _.file = '.env'`). See `.env.example`.

In the development stack, `compose.yaml` sets `DATABASE_URL`, `PORT` and `ENV` for the containers itself and passes the other variables below through. `DATABASE_URL` from Doppler or `.env` is used only by code running on the host, such as the integration tests.

| Variable                                                | Required   | Default                  | Description                                                          |
| ------------------------------------------------------- | ---------- | ------------------------ | -------------------------------------------------------------------- |
| `DATABASE_URL`                                          | yes        | —                        | PostgreSQL connection string. Startup fails if unset.                |
| `UMCS_USOS_CONSUMER_KEY`, `UMCS_USOS_CONSUMER_SECRET`   | yes        | —                        | USOS credentials for UMCS. Startup fails if unset.                   |
| `UMLUB_USOS_CONSUMER_KEY`, `UMLUB_USOS_CONSUMER_SECRET` | yes        | —                        | USOS credentials for UMLub. Startup fails if unset.                  |
| `RESEND_API_KEY`                                        | yes        | —                        | Resend API key for OTP emails. Startup fails if unset.               |
| `OTP_EMAIL_TEMPLATE_ID`                                 | yes        | —                        | ID of the Resend-hosted OTP email template. Startup fails if unset.  |
| `OTP_HASH_KEY`                                          | yes        | —                        | HMAC key for stored OTP codes, at least 32 characters. Never change it once in use. |
| `BACKEND_PUBLIC_URL`                                    | no         | `http://localhost:8080`  | Public URL of the API; the USOS callback URL is built from it.       |
| `APP_WEB_URL`                                           | no         | `http://localhost:3000`  | Web app URL.                                                         |
| `APP_MOBILE_SCHEME`                                     | no         | `uniezz://auth/callback` | Mobile app callback URL.                                             |
| `CORS_ALLOWED_ORIGINS`                                  | no         | `APP_WEB_URL`            | Comma-separated allowed origins.                                     |
| `MAIL_FROM`                                             | no         | `Uniezz <no-reply@mail.uniezz.pl>` | Sender of OTP emails; the domain must be verified in Resend. |
| `OTP_TEST_EMAILS`                                       | no         | —                        | Comma-separated emails accepted for OTP login at any OTP university, for testing. Startup fails if set when `ENV=production`. |
| `PORT`                                                  | no         | `8080`                   | HTTP listen port.                                                    |
| `ENV`                                                   | no         | `development`            | Environment name.                                                    |
| `GOOSE_DBSTRING`                                        | staging CI | —                        | Connection string for goose in the staging deploy. Not used locally. |

To add a variable to a config: `doppler secrets set NAME=value -p uniezz-backend -c dev`.

## Tasks

All tasks are defined in `mise.toml`; run them with `mise run <task>`.

### Development

| Task        | Description                                                                              |
| ----------- | ---------------------------------------------------------------------------------------- |
| `setup`     | Point git at `.githooks` — required once per clone                                       |
| `dev`       | Start PostgreSQL, apply migrations and run the API with hot reload; secrets from Doppler |
| `dev:env`   | Same as `dev`, with secrets from `.env`                                                  |
| `dev:reset` | Stop the stack and delete its volumes: the database data and Go caches                   |
| `fmt`       | Format Go code                                                                           |
| `vet`       | Run `go vet`                                                                             |
| `test`      | Run tests with the race detector; secrets from Doppler                                   |
| `test:env`  | Same as `test`, with `DATABASE_URL` from `.env`                                          |

### Database

| Task      | Description                                                                      |
| --------- | -------------------------------------------------------------------------------- |
| `db:up`   | Start only PostgreSQL in the background, e.g. for the integration tests          |
| `db:down` | Stop the whole stack, e.g. after `db:up` or if `dev` was not stopped with Ctrl+C |
| `db:logs` | Follow the PostgreSQL logs                                                       |

For the logs of the other containers, run `docker compose logs -f api` (or `migrate`).

### Migrations

`migrate:status`, `migrate:up` and `migrate:down` run goose in the `migrate` container (`docker compose run --rm migrate goose <command>`). They start PostgreSQL if it is not running and need no secrets. `migrate:create` only writes a file, so it uses the goose on the host.

| Task                    | Description                                 |
| ----------------------- | ------------------------------------------- |
| `migrate:status`        | Show the status of all migrations           |
| `migrate:up`            | Apply all pending migrations                |
| `migrate:down`          | Roll back the most recent migration         |
| `migrate:create <name>` | Create a new SQL migration in `migrations/` |

## Project layout

```
cmd/api/              Entry point: config load, DB pool, router, graceful shutdown
internal/config/      Environment configuration
internal/database/    pgxpool setup (25 max conns, 2 min, 5m idle, 1h lifetime)
internal/health/      /healthz/live and /healthz/ready endpoints
migrations/           goose SQL migrations
docs/                 Product description, authentication plan, sprint tasks (en/uk/ru/pl)
compose.yaml          Local development stack: PostgreSQL 17, migrations, API
Dockerfile.dev        Development image: Go toolchain and goose
Dockerfile            Production image
mise.toml             Toolchain versions and tasks
```

## API

| Method | Path             | Description                                                 |
| ------ | ---------------- | ----------------------------------------------------------- |
| `GET`  | `/healthz/live`  | Liveness check                                              |
| `GET`  | `/healthz/ready` | Readiness check: `200 OK`, or `503` once shutdown has begun |

The full API is described in `api/openapi.yaml`.

## Testing

```sh
mise run db:up          # the integration tests need PostgreSQL
mise run test           # everything, including integration tests; without Doppler: mise run test:env
go test -short ./...    # unit tests only
```

The tests run on the host and reach PostgreSQL through `DATABASE_URL` (`localhost:5432`). Integration tests are skipped under `-short` and when `DATABASE_URL` is unset.

## Graceful shutdown

On `SIGINT` or `SIGTERM` the server:

1. Marks itself as shutting down, so `/healthz/ready` starts returning `503`.
2. Waits 5 seconds so load balancers can drain it from rotation.
3. Shuts the HTTP server down with a 15-second timeout, forcing a close if that expires.

## CI

`.github/workflows/ci.yaml` runs on pushes and pull requests against `main`: format check, `go vet`, unit tests with the race detector (`-short`), and a build of `./cmd/api`.

## Documentation

- [Product description](docs/product-description/PRODUCT_DESCRIPTION.en.md)
- [Authentication](docs/auth/AUTHENTICATION.en.md)
- [Sprint plan and tasks](docs/tasks/README.md)

Each document is also available in Ukrainian, Russian, and Polish.

## License

See [LICENCE.md](LICENCE.md).
