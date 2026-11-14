# Uniezz Backend

Go HTTP API for [Uniezz](docs/product-description/PRODUCT_DESCRIPTION.en.md), a platform for students of Lublin universities. Go, chi, Huma, PostgreSQL 17 (pgx), goose, mise.

## Getting started

Needs [mise](https://mise.jdx.dev/getting-started.html), Docker with Compose 2.32+, and the [Doppler CLI](https://docs.doppler.com/docs/install-cli) with access to the `uniezz-backend` project.

```sh
mise install                              # pinned Go, goose, golangci-lint
mise run setup                            # shared git hooks, required once per clone
doppler login && doppler setup -p uniezz-backend -c dev
mise run dev                              # PostgreSQL, migrations, API on :8080 with hot reload
```

Without a Doppler seat, copy `.env.example` to `.env`, ask a team member for the values, and use `mise run dev:env` / `mise run test:env`. `.env.example` lists every variable the API reads.

On Windows, install Git for Windows (the hooks run under its `bash`) and start Docker Desktop before any task.

## Everyday tasks

```sh
mise run db:up            # PostgreSQL only, for the integration tests
mise run test             # all tests, including the database ones
mise run fmt && mise run lint
mise run openapi          # regenerate api/openapi.yaml
mise run migrate:create <name>
mise tasks                # everything else
```

CI runs `go test -short`, which skips every database test. Run `mise run test` locally before you push.

The API is described in `api/openapi.yaml` and served at `/docs`.

## Environments

| Environment | URL | Database | Deploys |
| ----------- | --- | -------- | ------- |
| staging | https://api-stg.uniezz.pl | Neon | on every push to `release/**` |
| production | https://api.uniezz.pl | Supabase (session pooler) | manually: Actions → **Deploy production** on a `release/*` branch that staging already deployed |

Both run on one server: `deploy/edge` is the shared Traefik, `deploy/stg` and `deploy/prd` are the API stacks. Secrets come from Doppler configs `stg` and `prd`; the deploy job writes them to the server's `.env`. Each GitHub environment holds `DOPPLER_TOKEN` and `SSH_HOST`, `SSH_USER`, `SSH_PRIVATE_KEY`, `SSH_KNOWN_HOSTS`, so moving production to its own server only changes the `production` environment's secrets. Deploy results go to Telegram.

## Documentation

- [AGENTS.md](AGENTS.md): how code in this repository must look
- [docs/CONVENTIONS.md](docs/CONVENTIONS.md): write patterns (counters, locks, idempotent creates)
- [docs/jobs.md](docs/jobs.md): background jobs
- [docs/observability.md](docs/observability.md): logs, traces, errors and alerts
- [Authentication](docs/auth/AUTHENTICATION.en.md)

## License

See [LICENCE.md](LICENCE.md).
