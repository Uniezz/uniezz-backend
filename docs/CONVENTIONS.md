# Write patterns

Rules for writes that need more than one statement to stay correct. The general code rules are in [AGENTS.md](../AGENTS.md).

## Counters and row locks

A counter stored on a row (`likes_count`, `comments_count`) changes in the transaction that inserts or deletes the rows it counts, and only when a row was really inserted (`ON CONFLICT DO NOTHING`) or deleted (`RowsAffected`). A double tap or a retry then changes nothing.

Take row locks top-down and at the strongest mode needed on first touch (`FOR NO KEY UPDATE`, `FOR UPDATE`, or the counter `UPDATE` itself), never `FOR SHARE` followed by an update. In feed the order is post, root comment, reply: every change to a comment tree locks its post first, so changes to one tree run one at a time. Read immutable parent ids without a lock to know what to lock, then re-check them under the lock.

## Precomputed aggregates

An aggregate that is read often (a ranking, a top list, a total) is stored ready to read and updated on write, not computed from the raw rows on every read. It follows the counter rule above: it changes in the transaction of the write that affects it, only when a row was really inserted or deleted, and in application code, not in a trigger. Index it in the order it is read, so a read is an index range scan of the rows returned (e.g. top 5 of the week: `WHERE week = $1 ORDER BY likes DESC LIMIT 5`). Rows that are no longer read, such as past weeks, are removed by a scheduled job.

## Idempotent creates

A create endpoint that clients may retry takes an optional `Idempotency-Key` header (1–255 characters, a UUID is fine). The key is stored per user and operation with a SHA-256 fingerprint of the validated input and the created resource's id, in the same transaction as the resource; a concurrent request with the same key waits on the key's unique index.

- Same key and input: `201` with the resource as it is now (edits and likes included) and `Idempotent-Replayed: true`. The response is not stored.
- Same key, other input: `422 idempotency_key_reused`.
- Same key after the resource was deleted: `410 resource_deleted`.
- Keys are honoured for at least 24 hours; a daily job removes older ones.

The fingerprint includes everything that defines the request, path parameters too (a comment's post id).
