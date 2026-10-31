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
- Only repositories touch the database. They hold a `database.DB` and run every query on `r.db.Conn(ctx)`.
- Logic that needs several writes to be atomic holds a `database.Transactor` and wraps them in `WithTx(ctx, func(ctx) error)`. Repositories called with that `ctx` run inside the transaction, in any module. A nested `WithTx` joins the open one.
