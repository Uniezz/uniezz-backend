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
