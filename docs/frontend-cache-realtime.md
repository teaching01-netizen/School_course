# Frontend Cache and Realtime Contract

Authenticated API data is cached only in memory with TanStack Query. Browser HTTP caching remains disabled with `cache: "no-store"`, and the complete query cache is cleared whenever the authenticated user ID changes or logout completes.

## Cache classes

| Class | Freshness | Retention | Examples |
|---|---:|---:|---|
| Reference | 5 minutes | 30 minutes | rooms, subjects, teachers, settings |
| Semi-static | 1 minute | 10 minutes | courses, students, rosters |
| Operational | immediately stale | 5 minutes | schedules, calendars, dashboards, absences, attendance |
| Sensitive detail | 15 seconds | 1 minute | teacher-visible absence reason/detail |

Operational queries refetch on focus and reconnect and every 30 seconds while the tab is visible. Previous data remains visible during background refreshes. Query consumers distinguish initial `loading` from background `refreshing` so retained data is not presented as a newly loaded range.

## Realtime behavior

The application owns one authenticated WebSocket connection. Components subscribe through `useRealtime`; they do not create sockets directly. HTTP remains authoritative, and events only invalidate cached projections.

- `sessions:all` invalidates sessions, attendance, operations calendar, and teacher dashboards.
- `absent:all` invalidates absence lists/details/stats, operations calendar, and teacher dashboards.
- `absent:stats` invalidates the stats query; the HTTP response remains authoritative so out-of-order events cannot regress the badge.
- `courses:all` invalidates course lists and the affected roster.

Duplicate events are batched by channel and resource ID. Repeated copies of one key coalesce, while distinct IDs and channels are all delivered. Reconnect uses capped exponential backoff with jitter, resubscribes active channels, invalidates every realtime-backed query family (including courses and rosters), and invokes recovery loaders for component-local state.

Mutation handlers invalidate their local projections immediately after success; backend events keep other clients synchronized after transaction commit.

## Backend delivery

Each backend process owns a bounded local WebSocket hub. Events are delivered locally first, then queued for PostgreSQL `LISTEN/NOTIFY` fanout so sockets connected to other replicas receive the same invalidation. Fanout envelopes include a version, event ID, and origin instance ID; the origin ignores its notification echo.

`LISTEN` requires a direct PostgreSQL connection or a session-pooling endpoint. When the primary `DATABASE_URL` is transaction-pooled, `REALTIME_DATABASE_URL` must provide that session-capable endpoint. Realtime uses a dedicated two-connection pool and the server waits for the listener before accepting HTTP traffic, failing startup instead of silently running without cross-replica delivery.

PostgreSQL notifications are best-effort rather than durable replay. The publish queue is bounded at 256 envelopes and broker failures are logged. Reconnect invalidation, focus refetches, and 30-second polling for operational queries provide recovery paths after a missed notification; this design does not claim durable event replay.

A WebSocket client has a 16-message outbound queue. Message 17 disconnects and unregisters a slow client and actively closes its network connection, forcing normal reconnect recovery instead of leaving a silently dead socket.

Inbound WebSocket commands are bounded:

- supported channels are `sessions:all`, `absent:all`, `absent:stats`, and `courses:all`;
- frame payloads are limited to 4 KiB;
- each connection may send at most 64 commands per minute;
- unsupported channels are ignored and sustained command overflow closes the connection.
