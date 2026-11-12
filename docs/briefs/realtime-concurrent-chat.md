# Realtime Concurrent Chat Brief

Status: draft for discussion; implementation has not started.

This brief records the architecture discussion, separates accepted decisions from recommendations, and outlines small implementation slices. Open questions are intentional review items, not permission to silently choose product behavior. The implementation plan becomes actionable as the relevant questions are resolved.

## 1. Problem and intent

Uniezz needs private and group conversations with persistent history and live updates. The team wants to understand and build a manageable system rather than generate a large implementation or adopt infrastructure before it has a purpose.

The immediate audience is an application with no users yet. The MVP supports groups of at most 50 participants. University-wide conversations with 1,000 or more participants, potentially a shared conversation across universities, are a future goal rather than an MVP requirement.

Start with correctness, recovery, operating costs, and measured capacity. Choose the client transport and libraries after establishing these requirements. This document does not select WebSockets, SSE, GraphQL subscriptions, WebRTC, or a chat library.

## 2. Accepted decisions

| Decision | Consequence |
| --- | --- |
| Build on the existing Go backend and PostgreSQL architecture. | Reuse existing modules and conventions rather than introduce a separate chat platform now. |
| The backend is the only application client of PostgreSQL. RLS is outside this design. | Enforce authorization in backend operations and coordinate conflicting operations with transactions, constraints, and locks. Revisit RLS if the access model changes. |
| Persist messages in PostgreSQL. | Process memory and live event channels are not the authoritative message history. |
| Keep active connections in a local, in-process hub. | Clients reconnect after a restart; the hub is rebuilt from connections, while history and authorization state come from PostgreSQL. |
| Start with one API instance on the existing VPS/Compose deployment model. | No Redis, RabbitMQ, Kafka, or Kubernetes requirement for the MVP. |
| Limit MVP groups to 50 participants. | Larger groups require a later capacity review, not an early distributed fan-out subsystem. |
| Recover missed messages through persisted history and synchronization. | A live connection is an optimization for delivery latency, not the only route to messages. |
| Reuse existing transactions, idempotency, rate limiting, constraints, telemetry, media, and durable job infrastructure where appropriate. | These are building blocks; chat-specific integration and failure behavior still need implementation and verification. |
| Work in small, understandable increments. | Tasks should be manually implementable in roughly half a day to one day once their decisions and dependencies are settled. |

These decisions do not establish a production capacity guarantee, uptime commitment, or delivery latency SLA.

## 3. Existing project context

- [Product description](../product-description/PRODUCT_DESCRIPTION.en.md): direct messages, groups, new-message notifications, and moderated text/media sharing.
- [Sprint tasks](../tasks/TASKS.en.md#sprint-3--chat): private/group chat, history, live messages, reconnect, attachments, and an in-app unread indication. OS push notifications are explicitly outside that sprint.
- [Module conventions](../CONVENTIONS.md): repositories own SQL; business logic coordinates transactions; lists use keyset pagination; retryable creates use idempotency keys.
- [Background jobs](../jobs.md): PostgreSQL-backed jobs, worker goroutines inside the API, leases, retry/backoff, and execution tokens. Handlers must tolerate repeat execution.
- [Deployment](../../deploy/stg/compose.yaml): Traefik and one API container; PostgreSQL is hosted separately on Neon.
- [Authentication](../../internal/auth/guard.go): database-backed opaque bearer sessions. Do not assume the JWT design in older briefs describes the current implementation.
- [Rate limiting](../../internal/ratelimit/ratelimit.go): PostgreSQL-backed fixed windows, not a Redis token bucket.
- [Media URLs](../../internal/media/url.go): public URLs for public kinds; expiring signed URLs for private kinds. Issuing a private chat URL still needs conversation authorization.

The user's current staging VPS is approximately 2 vCPU, 4 GB RAM, and 40 GB disk at about EUR 6.50/month. These are user-provided estimates, not a verified current provider offer. The machine also serves other API features.

Existing infrastructure reduces implementation work; it does not mean chat delivery, recovery, access revocation, or capacity have already been proven. The earlier 9.5-day chat sprint estimate is not a validated estimate for every slice in this brief.

## 4. Why PostgreSQL is sufficient to start

Tool capability lists are not a dependency checklist. Sessions, jobs, counters, rate limits, and many other operations can be implemented with either PostgreSQL or Redis, with different performance, persistence, and operational trade-offs.

PostgreSQL already supplies the persistence, transactions, constraints, indexes, and job coordination needed here. Add another system when a concrete requirement or measurement justifies its cost.

| Tool | Potential future reason to add it | What does not justify it by itself |
| --- | --- | --- |
| Redis cache | Measured repeated-read cost or latency; an acceptable staleness policy exists. | A general expectation that chat needs Redis. |
| Redis Pub/Sub | Forward transient live events between API processes, with recovery from PostgreSQL. | Expecting persistence, replay, or client delivery confirmation from Pub/Sub. |
| Redis Streams | A demonstrated need for retained streams/replay and the associated operating model. | Needing ordinary persisted chat history. |
| RabbitMQ | A requirement for broker routing, durable work queues, independent consumers, or operational isolation that existing jobs do not satisfy. | Reaching two API instances or an arbitrary user count. |
| Kafka | A requirement for a retained partitioned event log, replay, and independent downstream consumers. | Having a realtime feature or large chat rooms alone. |
| Kubernetes | Enough deployment/operations complexity to justify cluster orchestration. | Running two containers or needing more RAM. |

Redis can persist data and is more than a cache, but it is not needed as an alternative primary database in this design. PostgreSQL covering the current requirements does not make Redis generally redundant. RabbitMQ and Kafka can be useful well below Netflix scale; requirements, not company size, determine their value.

Avoid a premature generic broker framework. Keep message persistence/business rules separate from live delivery. If multiple instances become necessary, preserve the event contract while implementing and testing network failure semantics explicitly.

## 5. Proposed MVP behavior

The following details are recommendations until the linked open questions are answered.

### Scope

- Direct conversations and private groups with at most 50 participants, including the owner.
- Persistent text history with cursor pagination.
- Live updates for authorized connected clients and synchronization after gaps/reconnect.
- Multiple connections per user; clients deduplicate repeated events.
- An unread indication based on a monotonic read position, subject to Q6.
- Private attachments using the existing media subsystem, subject to Q8.
- Minimal report/access-control behavior agreed before launch, subject to Q10.

Outside the MVP: groups above 50, public university rooms, OS push, global online presence, typing indicators, search, reactions, edits, user-facing deletion, end-to-end encryption, multiple API instances, and a per-recipient delivered badge. Their priority can be changed explicitly during review.

### Write path and guarantees

```text
authenticate -> validate -> authorize under the write transaction
             -> persist message and retry identity -> commit
             -> confirm persistence to sender
             -> notify authorized local connections

missed or repeated live event -> synchronize from PostgreSQL -> deduplicate by ID
```

- Confirm a successful send only after commit. A lost response may require a retry with the same idempotency key.
- Preserve the repository's idempotency contract and make the key lifetime clear to clients; do not assume a deduplication guarantee beyond retained keys.
- Define a canonical per-conversation order suitable for recovery. Do not assume a wall-clock timestamp or globally allocated sequence alone gives a safe incremental cursor across concurrent commits.
- A database rollback creates neither a visible message nor a live event for that message.
- A crash after commit may delay live notification. Reconnect and active-session reconciliation must recover the gap, even if the connection itself never closes.
- Do not claim end-to-end exactly-once delivery. Persist once within the retry contract, tolerate repeated events, and converge through synchronization.
- No PostgreSQL transaction or pooled database connection stays occupied for the lifetime of a client connection.

### Conflicting operations

Choose and document one lock order shared by message writes and membership changes. Revalidate membership in the coordinated transaction. A removal that wins this ordering prevents the later write; an earlier authorized write can commit before removal.

Apply the same discipline to direct-conversation uniqueness, concurrent group additions at the 50-member cap, and monotonic read-position updates. A policy check or an application-level check outside the coordinated transaction is insufficient on its own.

### Hub and resource bounds

The hub owns local connections and subscriptions, not durable delivery truth. Each connection has bounded queued bytes/events and a bounded write time. Disconnect a slow client when it exceeds the agreed limits; the client recovers through synchronization. One slow recipient must not block the whole conversation.

Long-lived connections need session-expiry/revocation handling and subscription removal after access changes. A successful initial authentication is not permanent authorization for every conversation.

### Recovery and delivery status

Distinguish: saved in PostgreSQL, accepted by a broker if one is introduced, processed by a backend consumer, acknowledged by a device, and read by a user. None implies the next.

The recommended MVP exposes persistence and read state, while deferring a delivered badge. A broker ACK, empty queue, or successful socket write is not a device receipt. Offline users recover from history instead of retaining one unbounded live-delivery job per recipient.

The synchronization contract must cover the race between loading history and receiving new events, pagination during concurrent writes, and gap detection while a connection remains open. Q5 selects the exact contract before implementation.

### Background work and batching

Keep the normal 50-member live fan-out local initially. Reuse PostgreSQL jobs for durable deferred side effects when required; do not create one durable job per recipient by default.

Start with commit-then-local-notify plus mandatory reconciliation. If requirements demand retrying notification attempts independently of client reconciliation, add a transactional outbox/job in the same transaction as the message. A worker must tolerate duplicate execution. Outbox publication still does not establish device receipt.

The current runner defaults to 30-second polling, with a local wakeup for newly committed jobs. Deferred retries and claims by other instances can wait for polling; reducing the interval also increases Neon activity. Do not silently put the latency-critical path behind the current default polling behavior.

Batch size, worker count, connection count, and instance count are separate controls. More RAM can hold larger buffers/batches; CPU, network, database contention, latency requirements, and fairness determine whether larger batches help. Prefer bounded work and measure before tuning.

## 6. Implementation stages

This is a proposed backlog, not authorization to implement. Each row is a focused backend/documentation slice, including its relevant checks; estimates assume familiarity with the existing code and resolved prerequisite decisions. Frontend implementation is separate, but a real test client is required for integration. Split a row further if its acceptance condition cannot reasonably be completed in one day.

### Stage 0 — Resolve contracts

| ID | Task | Depends on | Size | Done when |
| --- | --- | --- | --- | --- |
| C0.1 | Resolve group creation, membership, and visibility rules. | Q1–Q3, Q10, Q13 | 0.5–1 day | A permissions matrix defines who can create, add, remove, leave, read past history, and report content; retention/deletion rules and whether the cap includes the owner are recorded. |
| C0.2 | Define ordering, retry identity, synchronization, and read semantics. | C0.1, Q5–Q7 | 1 day | Worked examples cover concurrent commits, lost responses, history/live overlap, offline reconnect, active-connection gaps, and two devices. |
| C0.3 | Choose transport and client authentication approach. | C0.2, Q4 | 0.5–1 day | A short decision records transport, library, browser/mobile support, credential handling, session revalidation, and shutdown behavior. |
| C0.4 | Set operational targets and bounds. | Q8–Q9, C0.3 | 0.5 day | Record message/attachment limits, per-user connection limits, buffer bounds, revocation deadline, reconciliation interval, and load-test targets. |

### Stage 1 — Persisted conversations without realtime

| ID | Task | Depends on | Size | Done when |
| --- | --- | --- | --- | --- |
| C1.1 | Add direct-conversation storage and creation. | C0.1 | 1 day | Reversed participant order and concurrent requests produce one shared thread; invalid/self-chat behavior follows the contract. |
| C1.2 | List a user's conversations. | C1.1 | 0.5–1 day | Cursor pagination is stable and excludes unrelated conversations. |
| C1.3 | Create private groups. | C0.1, C1.1 | 1 day | Name, owner, and distinct initial members are persisted atomically; more than 50 participants is rejected. |
| C1.4 | Add members with authorization and a hard cap. | C1.3 | 1 day | Concurrent additions cannot exceed 50; only permitted actors can add members. |
| C1.5 | Remove members and leave groups. | C1.4 | 1 day | Ownership/last-member behavior follows C0.1; the membership/write lock order is documented. |

### Stage 2 — Correct message history

| ID | Task | Depends on | Size | Done when |
| --- | --- | --- | --- | --- |
| C2.1 | Persist authorized text messages in canonical conversation order. | C0.2, C1.5 | 1 day | Valid writes survive restart; concurrent sends/removals follow the chosen order; outsider and oversized writes fail. |
| C2.2 | Apply existing idempotency conventions to send. | C2.1 | 0.5–1 day | Concurrent retries return one message; reuse with different content fails; the retention contract is documented. |
| C2.3 | Add paginated message history. | C2.1 | 0.5–1 day | Only permitted users can read; pages behave correctly while new messages arrive. |
| C2.4 | Add the incremental catch-up operation. | C0.2, C2.3 | 1 day | A client can recover all messages after its cursor across multiple pages; concurrent commit ordering cannot silently skip messages. |
| C2.5 | Persist monotonic read positions. | C0.2, C2.4 | 0.5–1 day | A stale device cannot move read state backwards or mark inaccessible/nonexistent positions as read. |
| C2.6 | Expose conversation unread indication. | C2.5, C1.2 | 0.5–1 day | Repeated reads and own-message behavior match Q6; listing avoids a query for each conversation. |

### Stage 3 — Local realtime delivery

| ID | Task | Depends on | Size | Done when |
| --- | --- | --- | --- | --- |
| C3.1 | Implement the local connection registry and bounds. | C0.3–C0.4 | 1 day | Multiple connections register/unregister safely; slow-client overflow releases resources; focused concurrent tests pass. |
| C3.2 | Expose an authenticated live connection. | C3.1 | 1 day | A test client connects using the current session system; invalid credentials and disallowed origins where applicable are rejected; credentials are not logged. |
| C3.3 | Authorize conversation subscriptions and notify after commit. | C3.2, C2.2 | 1 day | Authorized devices receive persisted IDs/order; rollback produces no event; one slow connection does not block other recipients. |
| C3.4 | Integrate catch-up and live-event overlap handling. | C3.3, C2.4 | 1 day | A test client reconnects, deduplicates overlap, and recovers a deliberately suppressed event while staying connected. |
| C3.5 | Apply membership removal to active subscriptions. | C3.3, C1.5 | 0.5–1 day | Removed members stop receiving new content within the agreed bound; concurrent removal/fan-out follows the documented rule. |
| C3.6 | Handle session revocation, expiry, and graceful shutdown. | C3.2, C0.4 | 1 day | Revoked/expired sessions lose access within the agreed bound; shutdown closes live connections explicitly and a client can reconnect and catch up. |

### Stage 4 — Complete the proposed MVP scope

| ID | Task | Depends on | Size | Done when |
| --- | --- | --- | --- | --- |
| C4.1 | Attach existing private media to messages. | Q8, C2.2 | 1 day | Only owned, ready, allowed media can be claimed; claim and message creation are atomic; invalid size/type is rejected. |
| C4.2 | Authorize attachment downloads. | C4.1, C1.5 | 0.5–1 day | Outsiders cannot obtain new signed URLs; behavior after membership removal and already-issued URL expiry is documented and tested. |
| C4.3 | Add chat report targets using existing moderation. | Q10, C2.3 | 0.5–1 day | Authorized users can report permitted chat targets without exposing private content through unrelated APIs. |
| C4.4 | Apply chat action and connection limits. | C0.4, C3.2 | 0.5–1 day | Send, connect, and subscribe limits have explicit rejection behavior; existing rate limiting is reused where suitable. |
| C4.5 | Add chat operational signals. | C3.4 | 0.5–1 day | Observe active connections, queued bytes, slow-client disconnects, commit-to-receive latency, catch-up errors, and any chat job backlog; avoid sensitive content and high-cardinality metric labels. |

### Stage 5 — Establish the MVP capacity envelope

| ID | Task | Depends on | Size | Done when |
| --- | --- | --- | --- | --- |
| C5.1 | Build a deterministic load client for connections and message fan-out. | C3.4, C0.4 | 1 day | It runs from a separate host, creates repeatable direct/group traffic, correlates sent/received IDs, and reports latency and missing/duplicate events. |
| C5.2 | Add slow-reader and reconnect scenarios. | C5.1 | 0.5–1 day | The harness can pause reads, drop clients, reconnect them, and verify recovered history. |
| C5.3 | Run the staging capacity experiment. | C4.4–C4.5, C5.2 | 1 day | A report records load, dataset, instance/DB configuration, other API traffic, CPU/RAM/network, DB pool waits, latency, errors, and a safe operating margin. |
| C5.4 | Exercise restart/failure recovery and document operation. | C5.3 | 1 day | Commit-before-notify failure, API restart, and temporary DB failure recover without missing persisted history or duplicate stored messages within the retry contract; a short runbook explains diagnosis and recovery. |

Stages 0–5 define the proposed MVP release boundary. No later stage is required simply to launch groups of 50. Resolving an open question may explicitly remove or split a slice; do not interpret this list as a fixed delivery-date commitment.

### Stage 6 — Multiple instances, when justified

Trigger: measured resource saturation after query/buffer tuning and consideration of a larger VPS, or an explicit availability/deployment requirement. More containers on the same VPS share the same resources and host failure risk; one Go process can already use multiple CPUs.

Proposed sequence, to split into similarly small tasks when this stage is selected:

1. Write a two-instance failure/routing acceptance test and identify connection ownership.
2. Choose an inter-instance event channel based on required loss/replay semantics. Redis Pub/Sub with PostgreSQL reconciliation is one candidate, not a preselected dependency.
3. Add publication and local dispatch; verify users on different instances and multiple devices receive the event.
4. Address producer commit/publish failure with an outbox if the selected guarantee requires it; validate duplicate processing and worker pickup latency.
5. Verify shared access revocation, session handling, rate limits, and rolling-deploy reconnects.
6. Load-test again; evaluate separate VPS hosts, load-balancer availability, and broker failure behavior.

Distributing arbitrary recipient batches is not sufficient: a worker can write only to connections held by its own process unless it routes through their owner. A single RabbitMQ queue with competing API consumers does not automatically broadcast to every API. Kafka consumer groups likewise require an intentional routing design.

### Stage 7 — Large university conversations

Future goal: 1,000+ participants, potentially up to the relevant student population. Registered group membership is distinct from online recipients and active senders.

Before increasing the cap, measure fan-out and hot-conversation contention; review discovery/join rules, moderation, unread calculations, notification amplification, reconciliation traffic, and abuse limits. Consider bounded recipient batches, coalesced updates, routing by instance, and dedicated workers only where measurements support them. Kafka, RabbitMQ, or Kubernetes remain optional outcomes of that review.

## 7. Capacity and operating-cost experiment

There is no justified conversion from 2 vCPU / 4 GB RAM to a guaranteed number of chat users. A provider's concurrent-connection plan limit is a commercial quota, not a benchmark for our machine. One user can open several connections.

Approximate live delivery demand is incoming messages/second multiplied by online recipient connections/message. Include the sender's other devices, event overhead, and any future read/typing events. Group membership alone does not determine demand.

Suggested experiment points, pending Q9: 100, 500, then 1,000 open connections with controlled activity and groups capped at 50. These are test points, not promised supported limits. Run mostly-idle, sustained-active, short-burst, slow-reader, and reconnect scenarios. Keep the load generator off the measured VPS and record Neon placement, dataset size, query plans, pool limits, cold starts, and background polling.

Report a supported workload envelope with spare capacity, not just the highest connection count reached. Set proposed latency/error targets before running the experiment. A test passes only if persisted history remains complete after reconciliation and buffers remain bounded.

Direct operating costs include the VPS, PostgreSQL compute/storage, media storage/egress, monitoring, backups/restore capability, and any redundant infrastructure. Free development time does not eliminate maintenance or recovery work. Do not assume free Neon usage implies a particular backup/retention guarantee. Keep text events small; media bytes belong in object storage.

## 8. Open questions — grill-me review queue

The user requested a recorded question list rather than a blocking interview. Questions below include a recommendation and are ordered by dependency. Ask one at a time when the interview resumes. Recommendations are not accepted decisions.

| ID | Question | Recommended answer | Blocks |
| --- | --- | --- | --- |
| Q1 | Who can initiate a direct chat, and do university boundaries restrict it? | Permit authenticated, eligible Uniezz users to contact each other across universities; reject self-chat. Confirm any contact/block restrictions before launch. | C0.1, C1.1 |
| Q2 | Who creates groups, adds/removes members, and what happens when the owner leaves? | Private, owner-managed groups; members may leave; transfer ownership before the owner leaves, with explicit empty-group behavior. | C0.1, C1.3–C1.5 |
| Q3 | Can new members see old history, and can removed members still fetch it? | New members see existing history; removed members cannot fetch history or obtain new attachment URLs. Already received content cannot be recalled. Confirm this privacy trade-off explicitly. | C0.1, C2.3, C4.2 |
| Q4 | Which initial clients must work, and how can they authenticate a long-lived connection? | Support the actual web/mobile clients using existing opaque sessions; pick transport/library after checking client credential constraints. Do not introduce JWTs just for chat. | C0.3 |
| Q5 | What order and cursor contract prevents gaps, including history/live overlap and a suppressed event on an open connection? | Canonical per-conversation order coordinated with commit visibility; bounded periodic reconciliation plus reconnect catch-up. Define subscription/history handoff and pagination examples before code. | C0.2, C2.1, C2.4, C3.4 |
| Q6 | What exactly means unread, read, and delivered across devices? | A user-level monotonic read position advanced by an explicit client action; own messages do not create unread. Device synchronization stays separate. Defer delivered badges. | C0.2, C2.5–C2.6 |
| Q7 | How long must a delayed retry retain its identity? | Start with the existing idempotency retention contract; clients stop automatic resends beyond that window and reconcile uncertain sends. If offline composition requires longer retries, choose a durable client-message identity explicitly. | C0.2, C2.2 |
| Q8 | Which attachments, size limits, and download-revocation behavior belong in MVP? | Reuse approved media kinds/limits and private signed URLs; authorize every new URL request. Accept bounded access until an issued URL expires, unless a stricter download proxy is required. | C0.4, C4.1–C4.2 |
| Q9 | What workload, latency, outage tolerance, and spending ceiling define success? | Measure an initial 100-connection workload with explicit send rates and 50-member groups; use 500/1,000 only as experiments. Keep one instance, accept reconnect during deploys, and agree numeric limits before claiming capacity. | C0.4, C5.1–C5.4 |
| Q10 | Which abuse and privacy controls are launch requirements? | Include reporting and action limits; explicitly decide blocking requirements and moderator access to private content. Defer automated moderation/search and E2EE unless required. | C0.1, C4.3, release scope |
| Q11 | How quickly must logout/removal stop an already-open connection from receiving content? | Immediate local invalidation when feasible plus bounded revalidation; document the deadline and already-in-flight data behavior. Recheck this design before multi-instance rollout. | C0.4, C3.5–C3.6 |
| Q12 | Are notification attempts required to retry independently of reconciliation? | For the initial MVP, commit-then-notify with bounded reconciliation is sufficient. Add durable jobs/outbox only if the accepted guarantee demands them; never call job completion a device receipt. | C3.3–C3.4, possible additional slice |
| Q13 | What are the message-retention, account-deletion, and group-deletion rules? | Agree retention and content attribution/anonymization before storage implementation; make no promise that messages live forever. Design removal effects across history, read positions, attachments, and moderation records. | C0.1, C2.1, release review |

Questions answerable from the repository should be investigated there instead of sent back to the user. In particular, sessions, the PostgreSQL job runner, private-media URL support, and rate-limiter implementation have already been checked.

## 9. References

- [PostgreSQL row security](https://www.postgresql.org/docs/current/ddl-rowsecurity.html): optional access policies do not independently resolve races involving other rows/tables.
- [Redis Pub/Sub](https://redis.io/docs/latest/develop/pubsub/): transient at-most-once delivery; recovery must come from another source when events matter.
- [RabbitMQ acknowledgements and publisher confirms](https://www.rabbitmq.com/docs/confirms): producer acceptance and consumer processing are separate from application device receipts.
- [RabbitMQ streams](https://www.rabbitmq.com/docs/streams): RabbitMQ also supports retained streams; queue-versus-log comparisons need to name the feature used.
- [Kubernetes components](https://kubernetes.io/docs/concepts/overview/components/): orchestration does not create extra machine resources or remove a single host's failure domain.
- [OWASP live-connection security guidance](https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html): authorization, credential handling, session lifecycle, resource bounds, and logging considerations; not a transport selection.
- [Ably SDKs](https://ably.com/docs/sdks): backend SDKs including Go exist for Pub/Sub; evaluate Ably Chat's separate capabilities before excluding it on SDK grounds.
- [Stream Chat pricing](https://getstream.io/chat/pricing/): plan quotas and fees are provider terms, not measurements of self-hosted capacity. Recheck terms if selecting a managed service.
