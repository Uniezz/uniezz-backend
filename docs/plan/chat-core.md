# Plan: chat core

Implements the architecture in [docs/briefs/realtime-concurrent-chat.md](../briefs/realtime-concurrent-chat.md) as one pull request: direct and group conversations, messages with images, ordering and catch-up, read state, the live stream, and the hooks for reports and account deletion. Product features on top of this core (reactions, edits, typing, invite links, files, voice, cards) are not part of it.

The brief is the authority. The product spec attached to issue #108 is useful for naming and JSON shapes the web already expects, but where it disagrees with the brief, follow the brief (brief, section 6).

## Who writes the code

`CLAUDE.md` says the developer types production code. For this plan the developer delegated the implementation to an agent session; the agent may create and edit production files for this plan only, and must still follow `AGENTS.md` and `docs/CONVENTIONS.md` exactly. If the session prompt does not state this delegation, stop and ask.

## Read first

1. `AGENTS.md` (every rule applies; the reference modules are `auth`, `media`, `profile`).
2. `docs/CONVENTIONS.md` (counters, locks, precomputed aggregates, idempotent creates).
3. The brief, all of section 4.
4. `internal/connections` (pair ordering with `LEAST`/`GREATEST`, `blocked_pairs`, `banned_users`, pagination), `internal/media/claim.go` and `delete.go`, `internal/profile/delete_account_task.go`, `internal/moderation/module.go` and `report.go`, `internal/auth/guard.go`, `cmd/api/main.go`, `internal/server/server.go`.
5. `.claude/skills/live-test/SKILL.md` (required before opening the pull request).

## Scope

In:

- Direct conversations: get-or-create, hidden without a Connection or with a block.
- Groups: create, rename, add members, remove members, leave, ownership transfer, deletion of an empty group, system messages.
- Messages: send text and up to 5 images, history by `seq`, catch-up by `change_seq`, get one, delete for everyone (tombstone).
- Read positions, stored unread counters, the unread total.
- The live stream (SSE) with the hub, heartbeats, write deadlines, session checks, stream limits, and drain on shutdown.
- The `message` report target and chat's `ForgetUser`.

Out: everything listed as deferred in brief section 6, moderator removal of chat messages, load testing (a later task with its own harness).

## Data model

Migrations are created with `mise run migrate:create <name>`, one module per migration, each with a full Down. Column names below are a sketch; follow `AGENTS.md` for naming, `CHECK`s and indexes.

### `chat_core` (chat)

```sql
CREATE TABLE chat_conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind TEXT NOT NULL CHECK (kind IN ('direct', 'group')),
    direct_user_low UUID REFERENCES users(id) ON DELETE CASCADE,
    direct_user_high UUID REFERENCES users(id) ON DELETE CASCADE,
    name TEXT CHECK (char_length(name) BETWEEN 1 AND 80),
    last_seq BIGINT NOT NULL DEFAULT 0,
    change_seq BIGINT NOT NULL DEFAULT 0,
    last_message_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chat_conversations_shape_check CHECK (
        (kind = 'direct' AND direct_user_low < direct_user_high AND name IS NULL)
     OR (kind = 'group' AND direct_user_low IS NULL AND direct_user_high IS NULL AND name IS NOT NULL))
);
-- uq_chat_conversations_direct_pair ON (direct_user_low, direct_user_high) WHERE kind = 'direct'
-- idx_chat_conversations_direct_user_high ON (direct_user_high) WHERE kind = 'direct'
-- set_chat_conversations_updated_at trigger

CREATE TABLE chat_members (
    conversation_id UUID NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_read_seq BIGINT NOT NULL DEFAULT 0,
    unread_count INT NOT NULL DEFAULT 0 CHECK (unread_count >= 0),
    PRIMARY KEY (conversation_id, user_id)
);
-- idx_chat_members_user_id ON (user_id, conversation_id)
-- uq_chat_members_owner ON (conversation_id) WHERE role = 'owner'

CREATE TABLE chat_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    change_seq BIGINT NOT NULL,
    sender_id UUID REFERENCES users(id) ON DELETE SET NULL,
    kind TEXT NOT NULL CHECK (kind IN ('text', 'system')),
    body TEXT CHECK (char_length(body) <= 4000),
    payload JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(payload) = 'object'),
    client_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_chat_messages_seq UNIQUE (conversation_id, seq),
    CONSTRAINT uq_chat_messages_change_seq UNIQUE (conversation_id, change_seq)
);
-- uq_chat_messages_client ON (sender_id, client_id) WHERE client_id IS NOT NULL
-- idx_chat_messages_sender_id ON (sender_id)

CREATE TABLE chat_message_media (
    message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    position SMALLINT NOT NULL CHECK (position BETWEEN 0 AND 4),
    media_id UUID NOT NULL UNIQUE REFERENCES media(id),
    PRIMARY KEY (message_id, position)
);
```

- The two unique constraints on `(conversation_id, seq)` and `(conversation_id, change_seq)` also serve as the indexes for history (`seq DESC`) and catch-up (`change_seq ASC`).
- `sender_id` is `ON DELETE SET NULL` on purpose: `ForgetUser` clears the content, then the user delete leaves the tombstone with no sender.
- `payload` holds only system messages: `{"event": "member_added", "actorId": "…", "params": {…}}`. Events: `group_created`, `renamed`, `member_added`, `member_removed`, `member_left`, `owner_changed`. Never translated text.
- No table only grows without a bound except messages, which are kept by design (brief 4.8), so there is no prune job.

### `connected_pairs_view` (connections)

A view like `blocked_pairs`, owned by connections, so chat can filter direct conversations in list queries without reading the `connections` table:

```sql
CREATE VIEW connected_pairs AS
SELECT user_low AS user_id, user_high AS other_id FROM connections
UNION ALL
SELECT user_high, user_low FROM connections;
```

### `reports_message_target` (moderation)

Add `'message'` to `reports_target_type_check`, following `20261111121200_reports_study_targets.sql`; Down deletes `message` reports first.

## Changes in other modules

| Module | Change |
| --- | --- |
| `auth` | Export `SessionIDFrom(ctx) (uuid.UUID, bool)` next to `UserIDFrom`, and `(m Module) SessionActive(ctx, sessionID) (bool, error)` with the same conditions as `GetActiveSessionByHash` (not expired, user not deleted or banned). Tests for both. |
| `connections` | The `connected_pairs` view migration. `AreConnected` already exists. |
| `moderation` | A `chatTargets` interface (`ReportTarget(ctx, reporterID, messageID uuid.UUID) (uuid.UUID, map[string]any, error)`), the `message` target type in the request enum and its `x-extensible-enum`, mapping chat's not-found error to moderation's. |
| `profile` | Chat as a `forgetter` in `deleteAccountTask`, after study and before media. |
| `server` | `Modules.Chat`, constructed after profile, connections and media; `m.Chat.Register(api)`. |
| `cmd/api` | `modules.Chat.Drain()` right after `isShuttingDown.Store(true)`, before the readiness delay. |

## Package `internal/chat`

| File | Holds |
| --- | --- |
| `module.go` | package doc, interfaces (`profiles`, `connectionChecker`, `mediaService`, `sessions`, `limiter`), `Module`, `New`, `Register`, `Drain` |
| `errors.go` | sentinels and `errorResponses` |
| `text.go` | limits and `cleanBody`, `cleanName` |
| `access.go` | every access rule (brief 4.5) |
| `conversation_handler.go`, `conversation.go`, `conversation_repository.go` | open direct, create group, list, get, rename |
| `member_handler.go`, `member.go`, `member_repository.go` | list, add, remove, leave, ownership transfer, read position, unread total |
| `message_handler.go`, `message.go`, `message_repository.go` | send, history, catch-up, get, delete |
| `system_message.go` | writing system messages inside the caller's transaction |
| `hub.go` | subscribers, publish, limits, drain |
| `event.go` | event types and encoding |
| `stream_handler.go` | the SSE operation |
| `report_target.go`, `forget_user.go` | hooks |

`Register` must work on a zero `Module` (`cmd/openapi`). The hub is created in `New`; it starts no goroutines of its own (each stream handler runs its own loop).

## Operations

All under `/chat`, tag `chat`, `auth.Authenticated`. Non-members get `404 not_found`.

| OperationID | Method and path | Notes |
| --- | --- | --- |
| `openDirectConversation` | `POST /chat/conversations/direct` | Body `{userId}`. `201` when created, `200` when it exists. `404` without Connection, with a block, or for an unknown, deleted or banned student; `400` for self. |
| `createGroupConversation` | `POST /chat/conversations/groups` | Body `{name, memberIds}` (1–49 others, each the creator's unblocked Connection). `Idempotency-Key` optional, stored on the `group_created` system message as `client_id`. Rate limit 5 per hour. |
| `listChatConversations` | `GET /chat/conversations` | Newest `last_message_at` first (conversations without messages by `created_at`), keyset cursor. Each item has `lastSeq`, `changeSeq`, `lastReadSeq`, `unreadCount`, the last visible message, `peer` for direct, `memberCount` and `myRole` for groups. |
| `getChatConversation` | `GET /chat/conversations/{id}` | Same item shape. |
| `updateChatConversation` | `PATCH /chat/conversations/{id}` | `{name}`; owner only, `403 forbidden` otherwise (a role rule); system message `renamed`. |
| `listChatMembers` | `GET /chat/conversations/{id}/members` | Owner first, then by `joined_at`; blocked students left out. |
| `addChatMembers` | `POST /chat/conversations/{id}/members` | `{userIds}`; any member, only their own unblocked Connections; `409 group_full` above 50; already-members are skipped. |
| `removeChatMember` | `DELETE /chat/conversations/{id}/members/{userId}` | Owner only. |
| `leaveChatConversation` | `DELETE /chat/conversations/{id}/members/me` | Groups only. Ownership passes to the longest-standing member; the last member leaving deletes the conversation. |
| `listChatMessages` | `GET /chat/conversations/{id}/messages` | Two exclusive modes: `cursor` (newest first by `seq`) or `changedAfter` (oldest change first, `nextCursor` while more remain). Both leave out messages from students blocked with the viewer or banned. |
| `sendChatMessage` | `POST /chat/conversations/{id}/messages` | Required `Idempotency-Key` (a UUID). Body `{body, attachmentIds}`. `201`, `Idempotent-Replayed`, `410`, `422`, `429`, `400 invalid_media`. |
| `getChatMessage` | `GET /chat/messages/{id}` | Refreshes presigned URLs. |
| `deleteChatMessage` | `DELETE /chat/messages/{id}` | Sender only; tombstone; `204`, deleting twice is `204`. |
| `markChatRead` | `PUT /chat/conversations/{id}/read` | `{seq}` between 0 and `last_seq`. Returns `{lastReadSeq, unreadCount}`. |
| `getChatUnread` | `GET /chat/unread` | `{totalUnread, unreadConversations}` in one query on `idx_chat_members_user_id`. |
| `openChatStream` | `GET /chat/stream` | SSE, see below. |

The `Message` response: `id`, `conversationId`, `seq`, `changeSeq`, `kind`, `sender` (profile summary, or `{"id": null, "deleted": true}`), `body`, `payload`, `clientId`, `attachments` (`mediaId`, `url`, `expiresAt`), `createdAt`, `deletedAt`. A tombstone has `body = null`, `attachments = []`, `payload = {}`.

## Write paths

Every write that changes a conversation takes the conversation row first (`UPDATE chat_conversations … RETURNING` or `SELECT … FOR NO KEY UPDATE`), then member rows in user-id order, then messages. Rate limits are counted before the lock, inside the same transaction.

Send, in order:

1. Clean and validate; parse the key as a UUID.
2. Replay lookup by `(sender_id, client_id)` without a lock; a hit returns it.
3. `media.CheckUpload` for every attachment, outside the transaction.
4. Transaction: rate limit; `UPDATE chat_conversations SET last_seq = last_seq + 1, change_seq = change_seq + 1, last_message_at = NOW() WHERE id = $1 RETURNING …`; access check (membership, for direct: `AreConnected` and no block); replay check again (a hit rolls back); insert; `media.MarkReady` in id order; `unread_count + 1` for other members who are not blocked with the sender (one statement); sender's `last_read_seq = seq, unread_count = 0`; load recipient ids (members minus students blocked with the sender).
5. After commit: `hub.Publish(recipients, message.created)` and `conversation.updated` is not needed (the client derives the last message from `message.created`).

Delete: lock the conversation (`change_seq + 1`), check sender and not already deleted, set `deleted_at`, clear `body` and `payload`, set the new `change_seq`, delete `chat_message_media` rows and `media.MarkDeleting` them; after commit `media.Delete` and publish `message.updated`. Unread counters are not changed.

Read: `UPDATE chat_members SET last_read_seq = GREATEST(last_read_seq, $seq)`; then `unread_count = 0` when it reached `last_seq`, else one `COUNT` of visible messages from others after it. Publish `read.updated` to the reader and, for a direct conversation, the peer.

Membership changes (add, remove, leave, rename, create) write a system message through the same `seq`/`change_seq` path, so clients catch them up like any message, then publish `message.created` and `conversation.updated` to members, and `conversation.removed` to a student who left or was removed.

## Hub and stream

- `subscriber`: user id, session id, buffered `chan []byte` of 256, a `done` channel, opened-at time.
- `Subscribe(userID, sessionID)` registers it; with 5 open for the user it closes the oldest; with 5,000 in total it returns an error mapped to `503`.
- `Publish(userIDs, event)` encodes once and does a non-blocking send to each subscriber; a full channel closes that subscriber. It takes no request context and never touches the database.
- `Drain()` sends `reconnect` with `afterMs` random in 0–5000 to every subscriber and closes them.
- The handler is a `huma.StreamResponse`. Get the `http.ResponseWriter` with `humachi.Unwrap`, write headers (`Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`), send `ready`, then loop on the subscriber channel, a 20-second heartbeat ticker (`: ping`), a 5-minute session ticker (`sessions.SessionActive`, then `auth_expired` and return), the subscriber's `done`, and the request context. Before every write call `http.NewResponseController(w).SetWriteDeadline(now + 30s)`, then `Flush`.
- The server's `WriteTimeout` is 10 seconds. The per-write deadline only works if every wrapper (`otelhttp`, chi `WrapResponseWriter`, Huma) lets `ResponseController` reach the real writer through `Unwrap`. Prove it first with a test in `cmd/api` that serves a stream through `newRouter` for longer than 10 seconds of wall time with heartbeats (use a short heartbeat in the test). If a wrapper hides `Unwrap` or `Flush`, fix that before anything else.
- Document the `200` response as `text/event-stream` in the operation with the event schemas (Huma does not generate it for a streaming body).
- Metrics through `telemetry.RecordGauge`/`RecordCount`: open streams, events published, subscribers closed for a full queue, drains. No user ids as labels; never log message text or tokens.

## Order of commits

Each commit leaves `mise run fmt`, `lint` and the tests green.

1. Migrations `chat_core` and `connected_pairs`; module skeleton, wiring, `openDirectConversation`, `getChatConversation`, `listChatConversations`.
2. Groups: create, rename, members, add, remove, leave, ownership, system messages.
3. Send, history, catch-up, get, with idempotency and the rate limit.
4. Images on messages and delete for everyone.
5. Read positions, unread counters, `getChatUnread`.
6. `auth` session exports; the wrapper test; hub; stream; publishing from every write path.
7. Drain in `cmd/api`.
8. Report target (migration in moderation) and `ForgetUser` wired into account deletion.
9. `mise run openapi`, `oasdiff breaking` against `main`, final review against `AGENTS.md`.

## Tests

Follow `AGENTS.md`, "Tests". `helpers_test.go` builds a `testEnv` with the real auth guard and sessions, real `connections` and `profile`, a fake media service (as in `feed`), an injected clock, and `connect(t, a, b)` / `block(t, a, b)` fixtures. Required, at minimum:

- 401 for every operation, in one table.
- Direct: same conversation for both orders and under concurrent opens; 404 without Connection, with a block, for a banned student; 400 for self; hidden from the list after unmatch and back with its history after a new Connection.
- Send: 20 concurrent sends give `seq` 1–20 and 20 distinct `change_seq`, and `unread_count` equals the raw count; 10 concurrent replays of one key give one message; replay with another body 422; replay after delete 410; empty and 4,001-character bodies 400; non-member 404; rate limit 429.
- History and catch-up: cursor walks all pages without duplicates while new messages arrive; `changedAfter` returns new messages and tombstones of older ones in `change_seq` order; blocked and banned senders never appear.
- Read: monotonic; stale `seq` does not move it back; `seq` above `last_seq` is 400; own messages never count; blocked senders do not count.
- Groups: membership rules and the cap of 50 under concurrent adds; owner-only rename and removal (403); ownership transfer; the last leave deletes the conversation and its media links.
- Delete: tombstone clears body and media, keeps `seq`, bumps `change_seq`, calls `MarkDeleting`.
- Hub (no database): fan-out to every stream of a user; a full queue closes only that subscriber; the sixth stream closes the oldest; the global cap; `Drain` sends `reconnect`.
- Stream (through `httptest.Server`): `ready`, a `message.created` after a send, heartbeats, the stream outliving `WriteTimeout`, `auth_expired` after the session is deleted (inject a short session-check interval through a module field set by tests, like `now`), `reconnect` on drain.
- `ForgetUser`: owned groups transferred, messages tombstoned with media released, the user row deletable afterwards, a second run changes nothing.
- Report target: member can report, non-member gets not found, snapshot without context.
- Migrations up, down, up.

## Live test before opening the pull request

Use the `live-test` skill: real API binary, Postgres, MinIO, jobs. Students are created and signed in as the skill shows; Connections are inserted directly (`INSERT INTO connections (user_low, user_high, mode) VALUES (LEAST(a,b), GREATEST(a,b), 'friends')`). Streams are read with `curl -N -H "Authorization: Bearer …" localhost:8081/chat/stream` in the background, writing to files in the scratchpad.

Every scenario below must pass, and the pull request description lists each one with the observed values (ids, `seq`, timings, status codes), not just "works":

1. **Direct conversation.** A and B connected, C not. A opens with B twice: same id, `201` then `200`. C opens with A: `404`. A opens with A: `400`.
2. **Live delivery.** B and A have streams open. A sends: B's and A's streams receive `message.created` (A's with the `clientId`) within a second.
3. **Idempotency.** The same key and body again: same id, `Idempotent-Replayed: true`, no new row. Same key, other body: `422`.
4. **Concurrency.** 20 sends in parallel (`xargs -P 20`): `SELECT seq, change_seq` shows 1–N without gaps or duplicates; B's `unread_count` equals the number of A's messages.
5. **Read.** B marks the last `seq` read: `unreadCount` 0, A's stream receives `read.updated`; marking an older `seq` afterwards does not lower `last_read_seq`.
6. **Catch-up.** Stop B's stream. A sends three messages and deletes an older one. B lists conversations (`changeSeq` grew) and calls `changedAfter` with its old cursor: exactly the three messages and the tombstone, in order.
7. **Stream survives.** A stream stays open for at least 2 minutes and receives heartbeats every 20 seconds (beyond the 10-second `WriteTimeout`).
8. **Restart.** With streams open, send SIGTERM to the API: each stream receives `reconnect` before closing, and shutdown takes less than the 15-second limit. Start the API again, reconnect, receive `ready`, and catch up messages sent while it was down (insert them after restart before reconnecting). The log has no `ERROR` lines.
9. **Unmatch.** Delete the A–B Connection: A's send returns `404`, the conversation leaves both lists. Insert the Connection again: the conversation and its history are back.
10. **Group.** A creates a group with B and C (both A's Connections). B blocks C. C sends: A's stream receives it, B's does not, B's `unread_count` does not change, B's history and member list leave C out. A (owner) leaves: B or C, whoever joined first, becomes owner, system messages are present. Everyone leaves: the conversation row is gone.
11. **Images.** Upload an image with `kind: chat` as the skill shows, send it as an attachment: the message's URL downloads with `200`. Delete the message: the media row is deleting, and after the media cleanup job the object is gone from the bucket.
12. **Report.** B reports A's message: a `message` report with a snapshot exists; C (not a member) reporting the same message gets `404`.
13. **Account deletion.** Delete A's account through the real endpoint, wait for the job: A's messages are tombstones with `sender_id` NULL, A's groups have a new owner, A's user row is gone, the job succeeded.
14. **Session end.** Log B out with a stream open: the stream receives `auth_expired` at the next session check. If waiting 5 minutes is too slow, record that this case is covered by the stream test instead.

Clean up as the skill says: rows by the run's marker, stop the API, leave the containers running.

## Pull request

- Title `feat(chat): add direct and group chat with live updates`, one PR, commits as listed above with messages per `AGENTS.md` (no AI attribution).
- The description lists the new operations, the migrations, the changes in `auth`, `connections`, `moderation`, `profile`, `server`, `cmd/api`, the live-test results scenario by scenario, and what is deliberately left for later (brief section 6).
- Done means the `AGENTS.md` checklist: `mise run fmt`, `mise run lint`, `mise run openapi` with `api/openapi.yaml` committed, `mise run db:up` and `mise run test` green, `oasdiff breaking` clean, migrations up/down/up.
