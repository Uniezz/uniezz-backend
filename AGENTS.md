# Rules for agents writing code in this repository

These rules are strict. Code that breaks one is not done, even when it builds and its tests pass.

**Reference modules.** `internal/auth`, `internal/media` and `internal/profile` are the reference for how code looks here. When a rule below leaves something open, copy what they do. Two parts of them are legacy and not to be copied: `profile/programme_*` (the handler calls the repository and the repository opens a transaction), and the `"failed to …"` error wording in auth repositories. `feed`, `moderation` and `faq` are not references: they break many of these rules.

**Write patterns.** When a change adds a counter column, row locks, a ranking or total stored on write, or a create endpoint clients may retry, read [docs/CONVENTIONS.md](docs/CONVENTIONS.md) first and follow it. When it adds a background job, read [docs/jobs.md](docs/jobs.md) first.

## Done means

A change is done only when all of these hold:

1. `mise run fmt` and `mise run lint` are clean.
2. `mise run openapi` was run and `api/openapi.yaml` is in the same commit as the change.
3. The integration tests pass locally: `mise run db:up`, then `mise run test` (or `test:env`). CI runs `go test -short`, which skips every database test, so CI passing proves nothing about them.
4. `oasdiff breaking` against `main` reports nothing, unless the developer decided on the breaking change.
5. Migrations go `up`, `down` and `up` again cleanly.

## Module layout

A module is one package under `internal/<module>`:

| File | Holds |
|------|-------|
| `module.go` | package doc, dependency interfaces, `Module`, `New`, `Register` |
| `errors.go` | sentinel errors and `errorResponses` |
| `text.go` | length limits as consts and the `cleanX` functions that apply them |
| `<resource>_handler.go` | HTTP only: operations, `<op>Input` / `<op>Output`, exported response types, handlers |
| `<feature>.go` | business logic, one feature per file, named after what it does (`claim.go`, `profile_media.go`) |
| `<resource>_repository.go` | SQL only: the repository, its `xRow`, `xFields`, `xFilter` and cursor structs |
| `<hook>.go` | a method another module calls, named after it (`report_target.go`, `init_profile.go`) |
| `<task>_task.go` | a background job: its args type and handler |

A file holds only what its name says. A sub-resource with its own endpoints gets its own handler, logic and repository files.

Names:

| Kind | Name | Example |
|------|------|---------|
| Huma request / response wrapper | `<handlerMethod>Input`, `<handlerMethod>Output`, one pair per operation | `createProgrammeInput` |
| Request body shared by operations | `<resource>Request` | `programmeRequest` |
| Response resource | exported plain noun, never `DTO` or `Response` | `Profile`, `ProfileSummary` |
| Handler to logic, a write | `<x>Change` | `profileChange` |
| Handler to logic, a read | `<x>Query` | `profileQuery` |
| Logic to repository, a write | `<x>Fields`, built by `<x>Change.fields()` | `profileFields` |
| Logic to repository, a read | `<x>Filter` | `profileFilter` |
| Database row | `<x>Row` | `programmeRow` |
| Repository | `<x>Repository`, built by `new<X>Repository(db)` | `profileRepository` |

Export only `Module`, `New`, sentinel errors, response types, enum types and their consts, job args, and the methods and functions another package actually calls. Everything else is unexported.

## Module, New, Register

- `module.go` starts with a package doc of one to three lines that says what the module owns.
- `Module` is a struct with unexported fields, returned by value from `New`.
- `New` wraps the pool once, `db := database.New(pool)`, and passes that `db` to every repository and, as a `database.Transactor` in the `tx` field, to the logic. It registers the module's jobs and wraps their errors as `fmt.Errorf("<module>: %w", err)`.
- Every dependency arrives through `New`'s arguments. Nothing is wired onto a module after construction.
- `Register(api huma.API)` has a value receiver, builds one small handler struct per resource (`profileHandler{profile: m}`), and calls `huma.Register` once per operation. It must work on a zero-value `Module`, because `cmd/openapi` builds the spec from `server.Modules{}`.

## Layers

**Handlers** do HTTP only:
- Read the user first, then leave a blank line:
  ```go
  userID, ok := auth.UserIDFrom(ctx)
  if !ok {
  	return nil, httpapi.Unauthorized()
  }
  ```
  Name it `viewerID` when the operation reads other students' data.
- Copy the input fields into a `<x>Change` or `<x>Query` and call one method on `Module`, even when that method only passes through to a repository. A handler never touches a repository field, never parses or validates, and never builds a status code.
- Map every error with `return nil, errorResponses.Error(err)`.
- Fill list outputs with `out := &xOutput{}`, then `out.Body.Items = …`.

**Logic** (methods on `Module` with a value receiver) owns every rule:
- Validate and normalise on value types before any I/O, then build the `<x>Fields` or `<x>Filter`.
- Wrap writes that must be atomic in `m.tx.WithTx(ctx, func(ctx context.Context) error {…})`, and pass the closure's `ctx` to every call inside it. Only logic opens transactions.
- Run network and storage work (HEAD requests, `media.Claim`, object deletes, `jobs.Wake`) outside the transaction. Run it before the transaction when it decides whether the write may happen, and after commit when it follows from the write.
- Prefer one guarded write (`UPDATE … WHERE gender IS NULL`, then `RowsAffected`) over a read followed by a write. When a read must come first, lock the row in that read (`FOR UPDATE` or `FOR NO KEY UPDATE`). Lock several rows of one table in id order.
- Count a rate limit inside the transaction of the write it limits, so a rejected or failed write costs no quota. Rate limits use `ratelimit.New` and map `ratelimit.ErrLimited` to the module's `ErrRateLimited`.
- Treat "nothing to do" as success: deleting something already gone, or sending the current value again, returns success.
- Pass data (`actorID`, `force bool`) to helpers, never callbacks. Write each operation and handler out in full; a factory or a helper that takes a function is not allowed.

**Repositories** are the only code that knows table and column names:
- `type xRepository struct{ db database.DB }` with pointer receivers. Every query runs on `r.db.Conn(ctx)`.
- One method per query, named with a short verb: `get`, `getMany`, `list`, `create`, `update`, `replace`, `delete`, or a domain verb (`markReady`, `setGender`). A method does exactly what its name says.
- Return `xRow` structs or primitives, never a response type.
- Keep a `const xColumns` and a `scanX(row pgx.Row, …)` helper for every row read more than once.
- Use `const query = …` for static SQL. Build dynamic SQL only with `database.Args` (`var args database.Args; arg := args.Add`), collecting conditions in `where []string` and adding `WHERE` only when the list is not empty. SQL fragments shared between queries take their placeholders from `database.Args`, never fixed `$n`.
- Put ownership in the `WHERE` clause (`WHERE id = $1 AND user_id = $2`), so another student's row is simply not found.
- Map `pgx.ErrNoRows` to `ErrNotFound`, and `RowsAffected() == 0` on an update or delete by id to `ErrNotFound`. Name Postgres error codes as consts (`foreignKeyViolation = "23503"`).
- Wrap every other error as `fmt.Errorf("<verb> <noun>: %w", err)`: lowercase, fixed text, no "failed to", no module or table name.
- Multi-row reads use `defer rows.Close()`, a `rows.Next()` loop and a final `rows.Err()` check.
- Never run one query per row. Load related data in one query (`WHERE id = ANY($1)`, `unnest`) or through a batch method on the owning module (`profile.Summaries`).

## Between modules

- A module reads only its own tables. To read another module's data, it joins a view that the owner publishes in its own migration (`ready_media`, `public_profiles`, `anonymous_profiles`, `blocked_pairs`). The view is the owner's contract, and it applies the owner's privacy rules, so readers cannot bypass them. Anything that needs code, such as building a URL, stays a Go call to the owner (`media.URLOf`).
- `users` belongs to auth and profile. Every other module reads `public_profiles` or `anonymous_profiles`, or calls profile.
- Use another module through a small unexported interface in `module.go` that lists only the methods called. On the line above it, assert the real type: `var _ mediaService = media.Module{}`, or `var _ limiter = (*ratelimit.Limiter)(nil)`.
- When a lower module calls back into a higher one, the lower module declares the exported interface (`auth.ProfileInitializer`) and the higher one asserts it. `internal/server` wires them together.
- Map another module's sentinel errors to your own at the boundary (`media.ErrNotFound` becomes `ErrInvalidAvatar`).
- A method that another module calls inside its own transaction takes that `ctx` and does its writes there.
- A student blocked in either direction (`blocked_pairs`) does not appear in any read of other students, and a direct read of them returns 404 `not_found`.

## HTTP API

- Every operation is a literal `var xOperation = auth.Protected(huma.Operation{…})` (drop `auth.Protected` only for public operations). Set its fields in this order: `OperationID`, `Method`, `Path`, `Summary`, `Description`, `Tags`, `DefaultStatus`, `Errors`.
- `OperationID` is camelCase, starts with a verb, and uses `My` for the caller's own resource (`getMyProfile`, `deleteMyAvatar`).
- Methods: `POST` creates, `PUT` replaces every field or sets an idempotent toggle, `PATCH` updates the fields present, and `DELETE` removes or unsets. A toggle is `PUT` and `DELETE` on the same path. Paths use plural nouns, `/<resource>/me` for the caller's own data, and `{id}` as the path parameter (`ID uuid.UUID \`path:"id" format:"uuid"\``).
- Every writable resource has a `GET`, and its `PATCH` returns the updated resource.
- `Summary` is a short third-person phrase with no final period ("Get the logged-in student's profile") that names every kind of target the operation accepts. `Description` gives the client-visible behaviour (null meanings, idempotency, limits, what is left out) in at most three sentences, with no internals.
- `Tags` is the module name in lowercase, and the first path segment matches it.
- A create sets `DefaultStatus: http.StatusCreated`. An operation without a body sets `DefaultStatus: http.StatusNoContent`, returns `(*struct{}, error)` and ends with `return nil, nil`.
- `Errors` lists every 4xx status the operation can return, except 401, which `auth.Protected` adds.
- JSON names are camelCase and mean the same thing in every response (`universityId`, `avatarUrl`).
- Field tags:
  - every `uuid.UUID` gets `format:"uuid"`;
  - every nullable response field is a pointer with `nullable:"true"` and a `doc` saying when it is null;
  - every enum gets `enum:"a,b"` matching the Go consts and the database `CHECK`;
  - inputs get `minLength`, `maxLength`, `maxItems`, `minimum`, `maximum` and `default`;
  - human-readable fields get `example`;
  - optional input fields are pointers with `,omitempty`;
  - a yes/no query parameter is a `bool`;
  - an optional enum is omitted when unset, never given an `""` value.
- Input `maxLength` is generous enough for raw text. The real limit is checked in logic after cleaning, and `doc` states it.
- Collections in responses are `[]`, never `null` (`text.NonNil`, `make([]T, 0, n)`).
- A response enum that will grow uses `x-extensible-enum` through a `TransformSchema` method, as `moderation.reportTargetType` does, so adding a value is not a breaking change.
- A `GET` does not write. When it must (opaque per-viewer tokens), its `Description` says so and a cleanup job prunes the rows.

## Lists

Every list endpoint paginates by cursor:
- Embed `httpapi.PageQuery` (`cursor`, `limit` 1–50, default 20) in the input, and return `{"items": […], "nextCursor": "…" | null}`.
- The cursor is an opaque `httpapi.EncodeCursor` of the last item's sort keys. Its struct lives in the repository file, with short JSON keys (`"c"`, `"i"`).
- Query with a keyset (`(created_at, id) < (…)`) and fetch `limit + 1` rows; trim with `httpapi.Page`.
- An index matches the list's order, created in the migration that adds the list.
- No offset pagination. A list that is fixed-size by design (a swipe deck, a top 5) says so in its `Description`.

## Errors

`errors.go` has exactly two blocks: `var ( ErrX = errors.New("lowercase message") )` and `var errorResponses = httpapi.ErrorResponses{ {Err: ErrX, Status: http.StatusY, Code: "snake_case"} }`.
- Reuse the shared codes for the shared meanings: `invalid_input` 400, `not_found` 404, `rate_limited` 429, `invalid_cursor` 400 (every paginating module maps `httpapi.ErrInvalidCursor`). A domain error gets its own code (`gender_immutable`).
- Order matters: the first `errors.Is` match wins, so a wrapper error comes before its cause.
- Wrap with the sentinel first: `fmt.Errorf("%w: unsupported kind %q", ErrInvalidUpload, kind)`.
- Errors that are returned are not logged; `httpapi` reports 5xx and the job runner reports job failures. An unexpected error that is swallowed goes to `telemetry.ReportError(ctx, "lowercase operation", err, slog.String("snake_key", v))`. A harmless failure of a best-effort step is `slog.WarnContext` with typed `slog.Attr` values.

## Values, enums and text

- Every enum is a named string type with one const per value (`type Gender string`), written once in Go and once in the migration's `CHECK`. SQL receives the const as a parameter, never as a literal.
- Every limit, size, count, age and duration is a named, unexported const at the top of the file that uses it (`uploadURLTTL = 10 * time.Minute`). SQL receives it as a parameter (`$1::interval`), never as a second literal. Use the database clock (`NOW()`), not Go's, for times stored or compared in SQL.
- IDs are `uuid.UUID` everywhere past the handler.
- Clean all user-written text with `text.Plain(s, keepNewlines)` and count length with `utf8.RuneCountInString` after cleaning. Use `text.Tags` for tag lists and `text.SearchWords` for search input, and translate their errors into `ErrInvalidInput`. A helper two modules need goes into `internal/text`, not into a second copy.
- A field where an empty string clears the value is a `*string` stored with a set flag: `CASE WHEN $set THEN $value ELSE col END`.
- A set-once field rejects any new value with its own error code, and an omitted field leaves it unchanged.
- No personal data, secrets or hardcoded identities in source. Authorisation never depends on matching an email string; privileged users come from configuration.

## Migrations

- Create migrations with `mise run migrate:create <name>`: `YYYYMMDDHHMMSS_snake_name.sql`, with a real time. A migration changes one module's tables, and a view gets its own migration.
- Every migration has `-- +goose Up` and a full `-- +goose Down` that undoes it in reverse order. Before restoring a narrower `CHECK`, Down deletes the rows that would break it.
- Tables:
  - `id UUID PRIMARY KEY DEFAULT gen_random_uuid()` and `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`;
  - mutable rows add `updated_at` and a `set_<table>_updated_at` trigger calling `update_updated_at_column()`;
  - enums are `TEXT … CHECK (x IN (…))`, and limits are `CHECK`s too;
  - every `REFERENCES users(id)` chooses its delete policy on purpose: `ON DELETE CASCADE`, or no cascade plus a `ForgetUser` hook.
- Indexes are named `idx_<table>_<columns>` and unique ones `uq_<table>_<what>`. Every foreign key that a user delete cascades through, and every reverse lookup, has an index.
- A table that only grows gets a cleanup job.
- SQL keywords are uppercase. Migrations indent with 4 spaces.

## Comments

Code has almost none: about one comment line per thousand in the reference modules. Write a comment only for the package doc, or for a reason the code cannot show (why an index gets used, why a lock is needed). Explain everything else in the pull request or in chat. Unexported identifiers have no doc comments. TODOs go into issues.

## Tests

- Database tests start with `setupTestDB(t)`. It skips under `-short` and when `DATABASE_URL` is unset.
- Shared fixtures go in `helpers_test.go`: a `testEnv` with the pool, the `humatest` API (with the real `auth` guard and real sessions), the module, and `decode[T](t, rec, status) T` and `expectError(t, rec, status, code)` helpers.
- Tests for `<file>.go` go in `<file>_test.go`. A test about one behaviour may be named after it (`gender_test.go`).
- The database is shared with every other package and with leftovers of crashed runs:
  - every fixture that hits a unique index is randomised (`"profile-"+uuid.NewString()+"@…"`);
  - each fixture registers its own `t.Cleanup` right after insert and ignores errors there;
  - a test asserts only on rows it created, filtering by its own ids or a per-test marker, never on a global count, an empty table, or the first row.
- Each `t.Run` creates its own fixtures and passes when run alone with `-run`.
- Every protected operation has a case that calls it without a session and expects 401.
- Statuses are `http.StatusX` constants. Use only the standard library: `t.Fatalf` when the test cannot go on, `t.Errorf` otherwise, and `errors.Is` for sentinels.
- `t.Run` names are lowercase sentences about behaviour ("sending the current avatar again changes nothing"). Table tests are `[]struct{name …}` run with `t.Run(tc.name, …)`.
- Concurrency rules (double taps, crossing writes, racing deletes) get a test that runs the requests at once and checks the result against the raw rows. Time-dependent code takes an injected clock (`now func() time.Time`); tests never sleep to a wall-clock boundary.
- A fake for another module embeds the interface, overrides only what the test needs, and writes to the real tables when foreign keys or views depend on them.

## Commits

Conventional Commits: `type(scope): summary`. The type is `feat`, `fix`, `refactor`, `perf`, `test`, `docs` or `ci`. The scope is the module, `api` or `deploy`, and is left out when the change crosses modules. The summary is lowercase and imperative, names the behaviour rather than the files, and has no final period. The body, wrapped at 72 columns, says what changed for clients and why: endpoints and fields by name, and new migrations. Commits carry no AI attribution.
