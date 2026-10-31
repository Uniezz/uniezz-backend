# Module conventions

## Files

| File | Holds |
|------|-------|
| `module.go` | `Module`, `New`, `Register`, interfaces of dependencies |
| `errors.go` | errors and their HTTP status and code |
| `<resource>_handler.go` | HTTP only: operations, inputs, outputs, response types, handlers |
| `<resource>_repository.go` | SQL only, and row structs |
| `<feature>.go` | business logic, e.g. `avatar.go`, `upload.go` |
| `<task>_task.go` | a background job: its definition and handler |

## Names

| Kind | Name | Example |
|------|------|---------|
| Huma request / response wrapper | `<op>Input`, `<op>Output` | `createProgrammeInput` |
| Resource in a response | plain noun, exported | `Profile`, `Programme` |
| Request body shared by operations | `<resource>Request` | `programmeRequest` |
| Database row | `<name>Row` | `programmeRow` |

No `DTO` suffix.

## Dependencies and transactions

- `New` builds the module's internals; `Register` builds its HTTP handlers.
- `New` wraps the pool once, `db := database.New(pool)`, and passes that value to every repository and, as a `database.Transactor`, to the logic.
- Only repositories touch the database. They run every query on `r.db.Conn(ctx)`.
- Logic that needs several writes to be atomic holds a `database.Transactor` and wraps them in `WithTx(ctx, func(ctx) error)`. `WithTx` puts the open transaction into the `ctx` it passes to the callback; `Conn(ctx)` returns it, so repositories called with that `ctx` run inside the transaction, in any module. Pass that `ctx` on: a call made with another context (e.g. `context.Background()`) runs outside the transaction. A nested `WithTx` joins the open one.

## Lists

Every list endpoint paginates by cursor: embed `httpapi.PageQuery` (`cursor`, `limit` 1–50, default 20) in the input and return `{"items": [...], "nextCursor": "..." | null}`. The cursor is an opaque `httpapi.EncodeCursor` of the last item's sort key; query with keyset (`(created_at, id) < (...)`), fetch `limit + 1` rows to know whether a next page exists. No offset pagination.

## Reading another module's data

A module reads only its own tables. When a query needs another module's data, that module publishes a view in its migrations (e.g. `ready_media`: `id, kind, object_key` of media that may be shown), and the reader JOINs the view, never the table. The view is the owner's contract: change its columns only together with its readers. Anything that needs code, such as building a URL, stays a Go call to the owner (`media.URLOf`), not a database function.

## Counters and row locks

A counter stored on a row (`likes_count`, `comments_count`) changes in the transaction that inserts or deletes the rows it counts, and only when a row was really inserted (`ON CONFLICT DO NOTHING`) or deleted (`RowsAffected`). A double tap or a retry then changes nothing.

Take row locks top-down and at the strongest mode needed on first touch (`FOR NO KEY UPDATE`, `FOR UPDATE`, or the counter `UPDATE` itself), never `FOR SHARE` followed by an update. In feed the order is post, root comment, reply: every change to a comment tree locks its post first, so changes to one tree run one at a time. Read immutable parent ids without a lock to know what to lock, then re-check them under the lock.

## Idempotent creates

A create endpoint that clients may retry takes an optional `Idempotency-Key` header (1–255 characters, a UUID is fine). The key is stored per user and operation with a SHA-256 fingerprint of the validated input and the created resource's id, in the same transaction as the resource; a concurrent request with the same key waits on the key's unique index.

- Same key and input: `201` with the resource as it is now (edits and likes included) and `Idempotent-Replayed: true`. The response is not stored.
- Same key, other input: `422 idempotency_key_reused`.
- Same key after the resource was deleted: `410 resource_deleted`.
- Keys are honoured for at least 24 hours; a daily job removes older ones.

The fingerprint includes everything that defines the request, path parameters too (a comment's post id).

