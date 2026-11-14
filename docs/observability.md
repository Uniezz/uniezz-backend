# Observability

The API reports to Sentry: logs and traces over OpenTelemetry (OTLP/HTTP), errors through the Sentry Go SDK, and `/healthz/ready` through a Sentry uptime monitor. Everything is also written to stdout as JSON, so `docker logs` keeps working when Sentry is unreachable.

Locally and in tests, none of it is configured: logs go to stdout only, and nothing is exported.

## What goes where

| Signal | Where it comes from | stdout | Sentry |
| ------ | ------------------- | ------ | ------ |
| Request log | `telemetry.Middleware`: one record per request with `method`, `route` (the pattern, not the path), `status`, `duration_ms`, `client_ip`, `request_id`, `trace_id`, `span_id`. 5xx is `ERROR`, 4xx `WARN`, the rest `INFO`. `/healthz/*` is not logged. | yes | Logs, if `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` is set |
| Other logs | `slog`, with `service`, `env` and `version` on every record | yes | Logs |
| Traces | `otelhttp` server span per request, named `GET /route`; `otelpgx` child span per query. `/healthz/*` is not traced. | — | Traces, if `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` is set |
| Errors | `telemetry.ReportError` for unexpected failures (5xx responses, USOS login failures) and recovered panics | `ERROR` record | Issues, if `SENTRY_DSN` is set; linked to the request's trace and tagged with `request_id` and `route` |

4xx responses are expected and never create issues. Error-level logs stay logs; only `ReportError` and panics create issues, so each failure is captured once.

Every response carries `X-Request-ID`: the incoming one if it is 1–64 characters of `A-Z a-z 0-9 . _ -`, a new UUID otherwise. CORS exposes it to the web app.

`service.name`, `service.version` (the build SHA) and `deployment.environment.name` are the same on logs, traces and Sentry events: the Sentry release and environment default to the build SHA and `ENV`.

## What is never sent

Tokens, OTP and login codes, cookies, `Authorization` headers, USOS consumer secrets, OAuth tokens and verifiers, request and response bodies, raw paths and query strings, and SQL bind values. This holds for stdout, exported logs, trace attributes and Sentry events:

- Log attributes with a sensitive key (`token`, `secret`, `authorization`, `cookie`, `verifier`, `otp`, `code`, …) are replaced with `[REDACTED]`. Messages, errors and panic values are scrubbed for `oauth_verifier=…`, `Bearer …`, `"token":"…"`, passwords in URLs and similar.
- Spans carry the route instead of the raw path (`url.path` is overwritten), and no query string. Queries are traced without bind values; all SQL in this repository is parameterized.
- Sentry collects no request data, headers, cookies, bodies, query strings or user info, and keeps no breadcrumbs. `BeforeSend` scrubs exception values and messages again before an event leaves the process.

The tests in `internal/telemetry` check each output against a set of representative secrets.

## Configuration (Doppler `stg` and `prd`)

Staging and production report to the same Sentry project and are told apart by environment. In Sentry: **Settings → Projects → uniezz-api → Client Keys (DSN)**. Copy the DSN, then open the **OpenTelemetry** tab and copy the complete traces and logs URLs and the auth header. The URLs already end in `/v1/traces` and `/v1/logs`; use them as they are.

| Variable | Value |
| -------- | ----- |
| `SENTRY_DSN` | The DSN |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | `https://o….ingest….sentry.io/api/<project>/integration/otlp/v1/traces` |
| `OTEL_EXPORTER_OTLP_TRACES_HEADERS` | `x-sentry-auth=sentry sentry_key=<public key>` |
| `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` | `https://o….ingest….sentry.io/api/<project>/integration/otlp/v1/logs` |
| `OTEL_EXPORTER_OTLP_LOGS_HEADERS` | `x-sentry-auth=sentry sentry_key=<public key>` |
| `OTEL_SERVICE_NAME` | `uniezz-api` |
| `OTEL_TRACES_SAMPLER` | `parentbased_traceidratio` |
| `OTEL_TRACES_SAMPLER_ARG` | `1.0` while verifying, then see [Sampling](#sampling-and-quotas) |
| `SENTRY_ENVIRONMENT` | Leave unset: it defaults to `ENV` (`staging`, `production`) |
| `SENTRY_RELEASE` | Leave unset: it defaults to the build SHA, the same as `service.version` |

The deploy job writes the whole config to the server's `.env`, so the next deploy picks the values up. To apply a change without new code, re-run the latest **Deploy staging** or **Deploy production** workflow.

Doppler `dev` has none of these, so telemetry stays off locally.

## Sampling and quotas

Only traces are sampled. Request logs and errors are never sampled.

The server ignores the sampling decision in an incoming `traceparent` (each request starts a new trace and only links the client's), so clients cannot force sampling. `/healthz/*` is neither traced nor logged, and pool-acquire spans are off. Health checks run every 10 s from Docker and Traefik and would otherwise dominate both quotas.

A request trace is about 1 server span plus one span per query, usually 2–6 spans. Spans per month ≈ requests per month × `OTEL_TRACES_SAMPLER_ARG` × spans per request. Log volume is about 300 bytes per request record.

The free Developer plan includes per month 5k errors, 5M spans, 5 GB of logs and one uptime monitor, with 30 days of retention. It cannot be charged: once a quota is used up, Sentry drops new data of that kind until the month resets. Check the current numbers in **Settings → Subscription / Usage**.

At about 5 spans per request, 5M spans cover about 1M traced requests per month, shared by staging and production, so both can keep `1.0` for now. Lower the rate once traffic gets close to the span quota; otherwise traces stop for the rest of the month.

## Sentry setup (no code)

### Uptime monitor

**Alerts → Create Alert → Uptime Monitor** (in newer UIs, **Monitors → Uptime**):

- URL `https://api.uniezz.pl/healthz/ready`, method `GET`, interval 1 minute (the free plan has one monitor, so it watches production)
- Expected: HTTP 200 (`/healthz/ready` answers 503 while shutting down or without a database)
- Environment `production`; notify on downtime and on recovery

### Email alerts

On the free Developer plan, Sentry delivers alerts by email only. Slack needs the Team plan or above; do not upgrade silently.

- **Issue alert**: a new issue is created, environment `production` → email.
- **Uptime**: down and recovered → email.

## Telegram alerts

Production posts unexpected errors to the team Telegram chat, without Sentry and without a paid plan. Deploy results go to the same chat from the deploy workflows.

- **What alerts:** every `ReportError` and every recovered panic: 5xx responses, USOS login failures and failed background jobs. 4xx responses never alert.
- **What a message holds:** environment, operation, route, version and request ID. The error text is scrubbed like Sentry events and cut to 300 characters; search the request ID in Sentry or `docker logs` for the rest.
- **Noise limits:**
  - one message per operation and route every 10 minutes; the next message says how many were held back;
  - at most 20 messages an hour;
  - alerts that do not fit in the queue are counted and dropped, so a request never waits for Telegram.
- **Configuration:** set `TELEGRAM_BOT_TOKEN` and `TELEGRAM_ALERT_CHAT_ID` in Doppler `prd` only. Without the chat ID, alerts are off, so staging and local runs post nothing.

## Verifying on staging

1. Set the variables above with `OTEL_TRACES_SAMPLER_ARG=1.0` and deploy.
2. Call the API a few times, for example `GET /auth/universities` and `GET /auth/usos/start?university=umcs&platform=web`.
3. In **Explore → Logs**, each request has one record with the fields above; it links to its trace.
4. In **Explore → Traces**, the request shows a server span with its database query spans.
5. An unexpected error creates one issue linked to its trace. To induce one without code, start a login at a university whose USOS key is wrong: `/auth/usos/start` fails upstream, the browser is redirected with `error=provider_unavailable`, and the failure is reported. Panics are covered by the tests in `internal/telemetry`, which also check that the server keeps serving.
6. `docker logs uniezz-stg-api-1` (or `uniezz-prd-api-1`) on the server still shows JSON lines.
7. Lower `OTEL_TRACES_SAMPLER_ARG` and re-run the deploy.
