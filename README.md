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
- [Doppler CLI](https://docs.doppler.com/docs/install-cli) — holds all secrets and env vars; ask a team member for access to the `uniezz-backend` project
- Docker (with Compose) for the local PostgreSQL instance — Docker Desktop on Windows/macOS

## Getting started

Tasks that need secrets (`dev`, `db:up`, `migrate:*`) call `doppler run -c dev` themselves, so you only ever type `mise run <task>`.

```sh
# 1. Install the pinned toolchain
mise install

# 2. Point git at the shared hooks (required, once per clone)
mise run setup

# 3. Log in to Doppler and link this directory to the dev config
doppler login
doppler setup -p uniezz-backend -c dev

# 4. Start PostgreSQL and wait for its healthcheck
mise run db:up

# 5. Apply migrations
mise run migrate:up

# 6. Run the API
mise run dev
```

> **Every new developer must run `mise run setup` after cloning the repository.** It points `core.hooksPath` at `.githooks`, so the shared git hooks are active for your clone. Git does not share hooks automatically — skip this step and the hooks will never run for you.

The server listens on `PORT` (default `8080`). Check it:

```sh
curl -i http://localhost:8080/health
```

### Windows from scratch

All commands below are for PowerShell.

1. Install the tools with winget, then **open a new terminal** so `PATH` picks them up:

   ```powershell
   winget install Git.Git
   winget install jdx.mise
   winget install Doppler.doppler
   winget install Docker.DockerDesktop
   ```

2. Start Docker Desktop and wait until it reports that the engine is running. `mise run db:up` fails if the engine is not running.

3. (Optional) To run `go`, `goose`, and `golangci-lint` directly, outside `mise run`, add the mise shims to your user `PATH` and open a new terminal:

   ```powershell
   [Environment]::SetEnvironmentVariable('PATH', "$env:LOCALAPPDATA\mise\shims;" + [Environment]::GetEnvironmentVariable('PATH', 'User'), 'User')
   ```

4. Clone the repository and follow the [Getting started](#getting-started) steps above. They are identical on Windows.

Windows notes:

- **mise runs tasks with `cmd.exe`.** `cmd.exe` does not expand `$VAR`, so never reference env vars as `$VAR` in a task's `run` line. Let the tool read the variable itself. For example, goose reads `GOOSE_DBSTRING`, `GOOSE_DRIVER`, and `GOOSE_MIGRATION_DIR`. For task arguments, use `{{usage.<name>}}`.
- **Do not wrap `mise run` in `doppler run`.** The tasks already inject secrets.
- **Port 5432 already in use?** A locally installed PostgreSQL service is probably holding the port. Stop it in `services.msc`, or change the host port in `compose.yaml` and in `DATABASE_URL` in Doppler.

## Configuration

Secrets and env vars live in Doppler (project `uniezz-backend`, configs `dev` / `stg` / `prd`) and are injected by `doppler run`. mise also loads `.env` if one exists (`[env] _.file = '.env'`). See `.env.example` for the local defaults.

| Variable         | Required       | Default       | Description                                                                          |
| ---------------- | -------------- | ------------- | ------------------------------------------------------------------------------------ |
| `DATABASE_URL`   | yes            | —             | PostgreSQL connection string. Startup fails if unset.                                |
| `GOOSE_DBSTRING` | for migrations | —             | Connection string for goose. In Doppler, set it to the reference `${DATABASE_URL}`.  |
| `PORT`           | no             | `8080`        | HTTP listen port.                                                                    |
| `ENV`            | no             | `development` | Environment name.                                                                    |

To add a variable to a config: `doppler secrets set NAME=value -p uniezz-backend -c dev`.

## Tasks

All tasks are defined in `mise.toml`; run them with `mise run <task>`.

### Development

| Task    | Description                                        |
| ------- | -------------------------------------------------- |
| `setup` | Point git at `.githooks` — required once per clone |
| `dev`   | Run the API from source (`go run ./cmd/api`)       |
| `fmt`   | Format Go code                                     |
| `vet`   | Run `go vet`                                       |
| `test`  | Run tests with the race detector                   |

### Database

| Task      | Description                                                 |
| --------- | ----------------------------------------------------------- |
| `db:up`   | Start the PostgreSQL container and wait for its healthcheck |
| `db:down` | Stop the container                                          |
| `db:logs` | Follow the container logs                                   |

### Migrations

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
internal/health/      /health endpoint
migrations/           goose SQL migrations
docs/                 Product description, authentication plan, sprint tasks (en/uk/ru/pl)
compose.yaml          Local PostgreSQL 17
mise.toml             Toolchain versions and tasks
```

## API

| Method | Path      | Description                                        |
| ------ | --------- | -------------------------------------------------- |
| `GET`  | `/health` | Returns `200 OK`, or `503` once shutdown has begun |

## Testing

```sh
doppler run -c dev -- mise run test   # everything, including integration tests
go test -short ./...                  # unit tests only
```

Integration tests in `internal/database` are skipped under `-short` and when `DATABASE_URL` is unset. The `test` task does not inject secrets itself, so wrap it in `doppler run` as shown above. Start the database with `mise run db:up` before running the full suite.

## Graceful shutdown

On `SIGINT` or `SIGTERM` the server:

1. Marks itself as shutting down, so `/health` starts returning `503`.
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
