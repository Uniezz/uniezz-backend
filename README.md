# Uniezz Backend

Go HTTP API for [Uniezz](docs/product-description/PRODUCT_DESCRIPTION.en.md), a platform for students of Lublin universities.

## Stack

| Concern | Choice |
|---------|--------|
| Language | Go 1.27.1 (pinned by mise) |
| Router | [chi](https://github.com/go-chi/chi) v5 |
| Database | PostgreSQL 17 via [pgx](https://github.com/jackc/pgx) v5 (`pgxpool`) |
| Migrations | [goose](https://github.com/pressly/goose) v3 |
| Toolchain / tasks | [mise](https://mise.jdx.dev) |
| Local infrastructure | Docker Compose |

## Requirements

- [mise](https://mise.jdx.dev/getting-started.html) — installs Go and goose at the pinned versions
- Docker (with Compose) for the local PostgreSQL instance

## Getting started

```sh
# 1. Install the pinned toolchain
mise install

# 2. Create your local environment file
cp .env.example .env

# 3. Start PostgreSQL and wait for its healthcheck
mise run db:up

# 4. Apply migrations
mise run migrate:up

# 5. Run the API
mise run dev
```

The server listens on `PORT` (default `8080`). Check it:

```sh
curl -i http://localhost:8080/health
```

## Configuration

Environment variables are loaded from `.env` by mise (`[env] _.file = '.env'`).

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | yes | — | PostgreSQL connection string. Startup fails if unset. |
| `PORT` | no | `8080` | HTTP listen port. |
| `ENV` | no | `development` | Environment name. |

## Tasks

All tasks are defined in `mise.toml`; run them with `mise run <task>`.

### Development

| Task | Description |
|------|-------------|
| `dev` | Run the API from source (`go run ./cmd/api`) |
| `fmt` | Format Go code |
| `vet` | Run `go vet` |
| `test` | Run tests with the race detector |

### Database

| Task | Description |
|------|-------------|
| `db:up` | Start the PostgreSQL container and wait for its healthcheck |
| `db:down` | Stop the container |
| `db:logs` | Follow the container logs |

### Migrations

| Task | Description |
|------|-------------|
| `migrate:status` | Show the status of all migrations |
| `migrate:up` | Apply all pending migrations |
| `migrate:down` | Roll back the most recent migration |
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

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Returns `200 OK`, or `503` once shutdown has begun |

## Testing

```sh
mise run test            # everything, including integration tests
go test -short ./...     # unit tests only
```

Integration tests in `internal/database` are skipped under `-short` and when `DATABASE_URL` is unset. Start the database with `mise run db:up` before running the full suite.

## Graceful shutdown

On `SIGINT` or `SIGTERM` the server:

1. Marks itself as shutting down, so `/health` starts returning `503`.
2. Waits 5 seconds so load balancers can drain it from rotation.
3. Shuts the HTTP server down with a 15-second timeout, forcing a close if that expires.

## CI

`.github/workflows/ci.yaml` runs on pushes and pull requests against `main`: format check, `go vet`, unit tests with the race detector (`-short`), and a build of `./cmd/api`.

> **Note:** the workflow currently lives at `cmd/.github/workflows/ci.yaml`. GitHub Actions only picks up workflows from the repository root, so it must be moved to `.github/workflows/ci.yaml` to run.

## Documentation

- [Product description](docs/product-description/PRODUCT_DESCRIPTION.en.md)
- [Authentication](docs/auth/AUTHENTICATION.en.md)
- [Sprint plan and tasks](docs/tasks/README.md)

Each document is also available in Ukrainian, Russian, and Polish.

## License

See [LICENCE.md](LICENCE.md).
