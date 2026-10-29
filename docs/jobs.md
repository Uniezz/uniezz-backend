# Background jobs

`internal/jobs` runs durable tasks inside the API process. Jobs live in the `jobs` table; the database is the source of truth, and the in-process wakeup only shortens pickup time.

## How it works

- A module defines a typed task (`jobs.Define[Args]("module.task-name.v1")`), registers its handler in its constructor (`jobs.Register`) and enqueues inside its own transaction (`jobs.Enqueue(ctx, tx, task, args)`). A rolled-back transaction leaves no job behind.
- After commit, the producer calls `Runner.Wake`. Workers also drain due work on startup and every 30 seconds, so a missed wakeup only delays a job.
- A worker claims one job at a time with `FOR UPDATE SKIP LOCKED`, gets a lease and an execution token, and runs the handler with the task's timeout. Only the holder of the current token can record the result.
- Errors are retried with exponential backoff and jitter (10 s doubling up to 10 min) until `MaxAttempts`. `jobs.Permanent(err)` fails the job at once. A job whose worker died is claimed again when its lease expires.
- Workers only claim kinds they have a handler for. Pending jobs of unknown kinds are reported to Sentry at startup.
- `JOB_WORKERS` (default 1) sets how many jobs run at the same time.

Delivery is at least once: handlers must be safe to run twice.

## Tasks

| Kind | Owner | What it does |
|------|-------|--------------|
| `profile.import-avatar.v1` | profile | Copies the USOS photo of a new account into our storage and sets it as the avatar, unless the student already chose one. |

## Operations

Failed jobs stay in the table with `last_error`. Inspect them:

```sql
SELECT id, kind, attempts, last_error, finished_at
FROM jobs
WHERE status = 'failed'
ORDER BY finished_at DESC;
```

After fixing the cause, retry one deliberately:

```sql
UPDATE jobs
SET status = 'pending', attempts = 0, available_at = NOW(), finished_at = NULL
WHERE id = '<job id>' AND status = 'failed';
```

Check backlog age before raising `JOB_WORKERS`:

```sql
SELECT kind, count(*), min(available_at)
FROM jobs
WHERE status = 'pending' AND available_at <= NOW()
GROUP BY kind;
```
