# Realtime Concurrent Chat Brief

Status: architecture agreed on 2026-11-21. The first implementation step is [docs/plan/chat-core.md](../plan/chat-core.md).

This brief records the architecture of the chat and why it looks this way. It is the authority for the backend chat design. The product spec attached to issue #108 (`CHAT-BACKEND-SPEC.md`, generated with Codex under the frontend developer's guidance) is an input: section 6 lists what this design takes from it and what it rejects.

## 1. Problem and intent

Uniezz needs private and group conversations with persistent history and live updates. Students already discuss university matters in Telegram groups, so Telegram is the long-term reference for behavior and protocol patterns. We move toward it step by step, within the limits of a small team and one VPS.

Use cases:

- After a match in Connections, writing to the person to get to know them.
- Discussing courses, faculties, universities and student life (scholarships, dormitory applications).
- Private conversations between friends.
- Small groups, for example for a study programme.

The backend builds the architectural core: storage, ordering, synchronization, access checks, live delivery, and their failure behavior. Product features on top (reactions, polls, housing cards, voice notes) reuse that core and are added later, separately.

## 2. Platform decisions

| Decision | Consequence |
| --- | --- |
| Build on the existing Go backend and PostgreSQL. | No separate chat platform. |
| PostgreSQL is the only source of truth. | Process memory and the live stream are never the only place something exists. |
| One API instance with an in-process hub. | No Redis, RabbitMQ, Kafka, or Kubernetes. A restart drops every stream; clients reconnect and catch up. |
| Recover through persisted history. | A live stream lowers latency; it is not the delivery guarantee. |
| No outbox. | Commit, then publish in process. A crash between the two is recovered by client catch-up. |
| No PostgreSQL connection or transaction is held by a stream. | Streams cost memory and a file descriptor, not pool slots. |

Deployment: Traefik and one API container on a Hetzner CX23 VPS (2 vCPU, 4 GB RAM, Falkenstein). Staging PostgreSQL is Neon and should still be able to scale to zero; production is Supabase. Choose the Supabase `eu-central-1` region, and connect directly or in session mode, because the transaction-mode pooler conflicts with pgx prepared statements.

Add infrastructure only when a measurement or requirement justifies it. Redis Pub/Sub or PostgreSQL `LISTEN/NOTIFY` become relevant only with a second API instance; Kafka and RabbitMQ are not needed for chat at this scale.

## 3. How the browser reaches the API

The browser never holds the session token. Login sets an HttpOnly cookie, and a proxy (Vite in development, a Cloudflare Pages Function in production) turns it into `Authorization: Bearer`. Every browser call is same-origin `/api/...`. The mobile app sends the bearer token itself.

This decides the transport (section 4.1): the realtime channel must be an ordinary HTTP request that the proxy can forward.

## 4. Decisions

### 4.1 Transport: Server-Sent Events, behind a transport-agnostic hub

Most large messengers (Slack, Discord, WhatsApp Web) use WebSockets because their clients send many realtime frames over the same connection: typing, acknowledgements, presence, call signaling. Our clients send every command over REST, which already has the guard, OpenAPI, idempotency, rate limits and tests. The realtime channel only carries server-to-client events, which is exactly what SSE does.

SSE fits our constraints better:

- It is a plain `GET` with the normal auth guard, a JSON `401`, and an OpenAPI entry. The cookie proxy forwards it like any other request; a WebSocket would need the Pages Function to proxy an upgrade and inject the header, which is untested.
- It runs over HTTP/2 through Cloudflare, so one stream does not use up a browser connection slot.
- React Native uses a library such as `react-native-sse`; the mobile app sends its bearer header directly.

What SSE costs: typing indicators would be a `POST` every few seconds instead of a frame, which is negligible at our scale. Cloudflare closes connections idle for about 100 seconds, so the stream sends a heartbeat every 20 seconds.

The hub does not know the transport. It routes encoded events to subscribers; the SSE handler is one subscriber type. Adding a WebSocket endpoint later is one new handler with the same events.

### 4.2 Ordering and synchronization

Two per-conversation counters, as in Telegram channels, where message ids are separate from the update counter `pts`:

- `seq`: the message's position. Allocated with `UPDATE chat_conversations SET last_seq = last_seq + 1 ... RETURNING last_seq` in the send transaction. The row lock is held until commit, so positions are gapless and follow commit order within a conversation. Used for display order, read positions, and history cursors.
- `change_seq`: the sync cursor. Incremented under the same lock on every message change (creation, tombstone, and later edits or reactions), and stored on the changed message.

`GET /chat/conversations/{id}/messages?changedAfter=K` returns every message created or changed after `K`, oldest change first. A client stores `changeSeq` per conversation and catches up with that single call. A global `BIGSERIAL` or `created_at` cursor would skip a message whose transaction commits after a later one; a `seq`-only cursor would miss changes to older messages.

The client does not detect loss from gaps in the numbers, because a viewer legitimately does not see some messages (blocked senders, section 4.5). Loss is signaled by the stream ending instead:

- The hub never drops an event silently. When it cannot queue an event for a subscriber, it closes that subscriber's stream.
- Publishing never depends on the request context and never blocks.
- On every (re)connect the client receives `ready`, then catches up the conversations it has cached through the conversation list (`changeSeq` per item) and `changedAfter`. Opening a cached conversation always calls `changedAfter` first.

There is no event ring buffer and no `Last-Event-ID` replay: every deploy empties a ring buffer anyway, and catch-up by `change_seq` is one indexed query per conversation.

The conversation row is the first lock for every operation that changes a conversation: send, delete, add, remove, leave, rename. That is the shared lock order. Writes in one conversation are serialized; different conversations do not contend.

### 4.3 Send and retry identity

```text
validate -> media.CheckUpload (outside the transaction)
BEGIN
  count the send rate limit
  lock conversation (seq + 1, change_seq + 1) -> check access
  replay? (sender_id, client_id) exists -> roll back, return it
  insert message -> media.MarkReady in id order
  unread_count + 1 for other members who may see it; advance the sender's read position
  load recipient ids
COMMIT -> respond -> hub.Publish
```

- `Idempotency-Key` is required on send, as the house convention and the web client expect. Its value must be a UUID; it is stored on the message as `client_id` with `UNIQUE (sender_id, client_id)` and echoed in events, so the sending tab replaces its pending message with the stored one.
- The key lives as long as the message, not 24 hours, and needs no key table or prune job. Same key and body: `201` with `Idempotent-Replayed: true`. Same key, other body: `422 idempotency_key_reused`. Same key after the message was deleted: `410 resource_deleted`.
- The replay check runs under the conversation lock, so a concurrent retry waits and then finds the first message. A replay ends the transaction with a rollback, which also undoes the counter increments and the rate-limit count. A cheap replay lookup before the transaction answers most retries without taking the lock.

### 4.4 Read state and unread counts

- `chat_members.last_read_seq` only moves forward (`GREATEST`), capped at the conversation's `last_seq`.
- `chat_members.unread_count` is stored and changed on write, following the precomputed-aggregate rule in `docs/CONVENTIONS.md`: incremented for every other member who may see the message in the send transaction, reset to 0 when the read position reaches `last_seq`, recomputed with one bounded count otherwise.
- Sending advances the sender's own read position. Read changes emit `read.updated` to the reader's other streams and, in a direct conversation, to the other participant ("Seen").
- A stored counter is needed because blocked senders in a group are not visible to the viewer (section 4.5), so `last_seq - last_read_seq` would count messages the viewer cannot see.

### 4.5 Access

All access rules live in one place (`access.go`), so product changes do not touch storage or delivery. Non-members always get `404 not_found`.

- **Direct conversations:** only between two students with a Connection and no block in either direction; self-chat is `400`. One conversation per pair, get-or-create under concurrent requests. Without a Connection or with a block, the conversation is hidden from both lists and every access is `404`; restoring the Connection restores the history. The Connection check reads `connections` inside the send transaction without a lock: a send that races an unmatch can commit just after it, which is harmless because the conversation is hidden from then on.
- **Groups:** any student creates one and becomes the owner. Members add their own Connections that they have not blocked; members are added directly and may leave. The owner removes members and renames the group. When the owner leaves, the longest-standing member becomes owner. When the last member leaves, the group is deleted in that transaction. At most 50 members including the owner. Group members do not need Connections with each other.
- **Blocks inside groups:** `AGENTS.md` requires that a blocked student's content does not appear to the other. A viewer does not see messages from a student blocked in either direction, does not receive their events, does not count them as unread, and does not see them in the member list. The group itself keeps working.
- Banned and deleted students cannot open a stream (the session check fails) and are left out of profile reads like everywhere else.

### 4.6 Live delivery

- `GET /chat/stream` is a Huma operation with a streaming body. Frames: `id`-less `event:` plus one `data:` JSON line; a `: ping` comment every 20 seconds.
- Events are viewer-agnostic, small, and carry `conversationId` and `changeSeq` where they concern a conversation: `ready`, `message.created`, `message.updated` (tombstones), `conversation.updated`, `conversation.removed` (sent only to the student who left or was removed), `read.updated`, `reconnect`, `auth_expired`.
- Routing is by user: a stream receives events for all of its user's conversations without subscribing. Recipients are the member ids loaded in the write transaction, minus students blocked with the actor.
- Bounds: 256 queued events per stream (overflow closes the stream); a write deadline of 30 seconds set before every write; 5 streams per user (a sixth closes the oldest, because a dead mobile connection may not have been noticed yet); 5,000 streams per process (`503` above).
- The guard checks the session once at the start. Every 5 minutes the stream checks it again and sends `auth_expired` before closing when the session ended (logout, expiry, ban, deletion).
- On shutdown, while the readiness probe already fails, every stream receives `reconnect` with a random delay of 0–5 seconds and is closed, so `server.Shutdown` is not held open and clients do not reconnect all at once.
- Unmatch or block does not send a live event: `connections` would have to call back into chat, which depends on it. The conversation answers `404` and drops out of the next list.

### 4.7 Attachments

Images only (`image/jpeg`, `image/png`, `image/webp`, the existing private `KindChat`), at most 5 per message. `media.CheckUpload` runs before the transaction and `media.MarkReady` inside it in id order. URLs are presigned on every read and never stored. A tombstone releases its media with `MarkDeleting` in the transaction and `Delete` after commit. Files and voice notes need content-type and size changes in `media` and are a separate feature.

### 4.8 Deletion, accounts, moderation

- Deleting a message for everyone (by its sender) turns it into a tombstone: the row, `seq` and `change_seq` stay; text and media are cleared. It is the core mutation that `change_seq` must carry.
- Account deletion: chat's `ForgetUser` runs before media's. It transfers ownership of the user's groups, tombstones the user's messages, deletes direct conversations whose other participant can no longer see them, and removes memberships. `chat_messages.sender_id` is `ON DELETE SET NULL`, so tombstones survive the user row and render as a deleted student.
- Reports: a new `message` report target. The reporter must be able to see the message; the snapshot holds its text, kind and attachment count without surrounding messages, because those are other students' private content the reporter did not choose to share. Moderators can ban; removing a reported chat message is a follow-up.
- No automatic expiry of messages.

### 4.9 Limits

| Limit | Value |
| --- | --- |
| Message text | 1–4,000 characters after `text.Plain`; empty only with attachments |
| Attachments | 5 images |
| Group | 50 members, name 1–80 characters |
| Pages | `httpapi.PageQuery` (default 20, at most 50) |
| Sends | 30 per minute per student, counted in the send transaction |
| Group creation | 5 per hour per student |
| Streams | 5 per student, 5,000 per process, 256 queued events each |
| Heartbeat / write deadline / session check | 20 s / 30 s / 5 min |

## 5. Capacity targets

Measured after the core works, from a host outside the VPS, with groups of up to 50 and other API traffic running:

- 1,000 open streams pass; 2,000 is an experiment to find the limit.
- 20 messages per second on average, bursts of 100 per second for 30 seconds, 80% direct and 20% group conversations.
- Commit-to-receive latency p95 below 300 ms, p99 below 1 s; send response p95 below 200 ms.
- Zero lost messages after catch-up and zero duplicate stored messages, checked by `client_id`, `seq` and `change_seq`. This is the primary criterion.
- Chat memory at most 1 GB; queues stay bounded with deliberately slow readers.
- All clients reconnect and catch up within 30 seconds after a deploy.

Neon on staging stays awake while streams are open (session checks) and while the job runner polls every 30 seconds by default; raise `JobPollInterval` on staging if staging should sleep.

## 6. Review of the product spec (#108)

The spec's general direction matches this brief: Connections-only direct conversations, REST as the source of truth, a gapless per-conversation `seq` under the conversation lock, publishing after commit with recipients from the transaction, an in-memory hub, bounded queues, system messages with event codes instead of translated text, `404` for non-members, and viewer-agnostic events.

Taken from the spec: SSE as a Huma operation, the cookie-proxy constraint, the `Idempotency-Key` header contract, the 20-second heartbeat and per-write deadline, the 5-minute session check, drain with `reconnect`, the 4,000-character limit, at most 5 attachments, and blocked senders hidden inside groups.

Changed:

| Spec | This design | Why |
| --- | --- | --- |
| Ring buffer of 2,000 events with `Last-Event-ID` resume | Catch-up by `change_seq` after every connect | Every deploy empties the ring; the spec's own `resync` fetched only new messages (`after=<lastSeq>`) and missed changes to older ones. |
| `chat_request_keys` table pruned after 24 hours | `client_id` column, unique per sender | Deduplication for the message's lifetime, one table less, one job less. |
| A sixth stream is rejected with `429` | A sixth stream closes the oldest | A mobile client reconnecting after a network switch would be locked out until its dead stream times out. |
| Groups up to 200, members without Connections, invite links | 50 members, added from the adder's Connections, no invite links | Strangers seeing each other through invite links conflicts with the privacy rule that a profile is not revealed without a match. A product decision can widen the access rules later without touching the core. |
| Report snapshot with up to 5 earlier messages | The reported message only | Earlier messages are other students' private content; adding context needs a privacy-policy statement first. |

Deferred, because they are features on top of the core and do not change its architecture: reactions, replies, mentions, edits, delete for me, forwarding with media copies, message pins, chat pins, archive, mute and notification levels, typing, presence and last seen, delivered receipts, admin role, announcement mode, invite links and approval, search, link previews (an SSRF-sensitive fetcher), polls, events with RSVP, housing viewing slots and context cards, live location, files and voice notes.

Two of them need care when they are built:

- Delivered receipts in the spec mean "written to a stream", which is not a device receipt, and they add a batched database writer to the hub.
- Housing context cards assume that a student can write to the author of a housing post, but direct conversations require a Connection. The product has to decide that rule first.

## 7. Later stages

**More instances**, when measurements show one process is saturated after tuning and a larger VPS, or when availability requires it: choose an inter-instance channel (PostgreSQL `LISTEN/NOTIFY` on a direct connection, or Redis Pub/Sub) for transient fan-out, keep PostgreSQL catch-up as the recovery path, and re-test session checks, rate limits and rolling deploys.

**Large conversations** (1,000+ members, possibly system-created from USOS programmes): the per-conversation lock and the per-member unread update become hot; join rules, visibility of strangers, moderation, and notification amplification need their own review.

## 8. References

- [Telegram: working with updates](https://core.telegram.org/api/updates) and [updates.getChannelDifference](https://core.telegram.org/method/updates.getChannelDifference): per-channel `pts`, difference limits, gap handling.
- [Discord gateway](https://docs.discord.com/developers/docs/topics/gateway): resume or reconnect with a full state.
- [Zulip events system](https://zulip.readthedocs.io/en/latest/subsystems/events-system.html): per-session queues and heartbeats, the model this design does not use.
- [Postgres and guaranteed orderings](https://elixirforum.com/t/postgres-and-guaranteed-orderings-what-is-the-correct-way-to-do-this/70309) and [ordering in a Postgres outbox](https://event-driven.io/en/ordering_in_postgres_outbox/): why sequence values do not follow commit order.
- [Cloudflare Workers limits](https://developers.cloudflare.com/workers/platform/limits/): no duration limit for HTTP Workers; idle connections are cut by the CDN.
- [react-native-sse](https://www.npmjs.com/package/react-native-sse): SSE client for React Native.
- [OWASP WebSocket security](https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html): session lifecycle and resource bounds, which apply to any long-lived connection.
