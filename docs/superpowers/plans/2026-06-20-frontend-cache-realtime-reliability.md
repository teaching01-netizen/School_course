# Frontend Cache and Realtime Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make frontend cache invalidation and authenticated WebSocket delivery converge safely across bursts, reconnects, and multiple backend replicas.

**Architecture:** Preserve the local realtime hub, add a PostgreSQL `LISTEN/NOTIFY` fanout layer with origin deduplication, and make overflow close the actual socket. On the frontend, preserve distinct debounced events, expose reconnect recovery to local-state consumers, invalidate every realtime-backed query family, and keep HTTP authoritative for statistics.

**Tech Stack:** Go 1.x, pgx/v5, `golang.org/x/net/websocket`, React 19, TypeScript 5.9, TanStack Query 5, Vitest 4.

---

## File Map

- Create `backend/internal/realtime/fanout.go`: envelope and fanout contracts.
- Create `backend/internal/realtime/postgres_fanout.go`: PostgreSQL notification publisher/listener with reconnect.
- Create `backend/internal/realtime/postgres_fanout_test.go`: fake/two-instance behavior and optional PostgreSQL integration coverage.
- Modify `backend/internal/realtime/hub.go`: idempotent client closure, overflow removal, local/fanout separation.
- Modify `backend/internal/realtime/hub_test.go`: exact capacity boundary and cross-instance tests.
- Modify `backend/internal/httpapi/realtimehttp/routes.go`: connection-close propagation and bounded inbound protocol.
- Create `backend/internal/httpapi/realtimehttp/routes_test.go`: supported/unsupported channel and overflow socket behavior.
- Modify `backend/internal/httpapi/handler.go`: construct and start the PostgreSQL-backed realtime hub.
- Modify `backend/internal/httpapi/courseshttp/routes.go`: publish batch deletion and legacy-sync events.
- Modify `backend/internal/httpapi/courseshttp/routes_realtime_test.go`: publication regression tests.
- Modify `src/realtime/RealtimeProvider.tsx`: reconnect subscriptions and jitter helper.
- Modify `src/realtime/RealtimeProvider.test.tsx`: reconnect callback and timer-boundary tests.
- Modify `src/hooks/useRealtime.ts`: keyed event batching and reconnect callback option.
- Create `src/hooks/useRealtime.test.tsx`: duplicate, distinct-ID, distinct-channel, cleanup, and reconnect tests.
- Modify `src/realtime/queryBridge.tsx`: invalidate every realtime-backed root and stop direct stats writes.
- Modify `src/realtime/queryBridge.test.ts`: course/roster reconnect and stats invalidation tests.
- Modify `src/query/cache.ts`: stats query configuration remains HTTP authoritative.
- Modify `src/hooks/useApiQuery.ts`: expose `refreshing`.
- Modify `src/test/useApiQuery.test.ts`: retained-data refresh state regression.
- Modify `src/pages/TeacherDashboard.tsx`: consume `refreshing` for month transitions.
- Modify `src/components/Layout.tsx`: observe absence stats through TanStack Query and recover on reconnect.
- Modify `src/pages/AbsenceDetail.tsx`: reload current detail on reconnect.
- Modify `src/components/absences/KanbanView.tsx`: reload columns on reconnect.
- Modify `src/hooks/useAttendanceModal.ts`: reload an open modal on reconnect.
- Modify relevant existing component tests for reconnect recovery.
- Modify `docs/frontend-cache-realtime.md`: document fanout, overflow, reconnect, and HTTP-authoritative stats.

### Task 1: Close and unregister slow clients

- [x] Add failing hub tests proving messages 1-16 remain queued and message 17 closes `Done()` and removes the client from all hub indexes.
- [x] Run `cd backend && go test ./internal/realtime -run 'TestHub(SlowClient|BufferBoundary)' -count=1`; expect failure because `Done` and removal behavior do not exist.
- [x] Add a `done` channel and idempotent close path to `Client`; make `Hub.Publish` call `Client.Close()` when `trySend` returns false instead of closing only the send queue.
- [x] Add a failing HTTP test proving client closure causes the network connection to close without another inbound message.
- [x] Add a connection watcher in `handleWS` that closes the socket when `client.Done()` closes.
- [x] Re-run realtime package tests and the HTTP realtime package tests; expect pass.

### Task 2: Bound the inbound WebSocket protocol

- [x] Add failing table tests for the four allowed channels, an unknown channel, an empty channel, malformed JSON, payloads above the configured maximum, and the per-window message boundary.
- [x] Run `cd backend && go test ./internal/httpapi/realtimehttp -count=1`; expect failures for missing validation.
- [x] Add an allowlist helper, set `conn.MaxPayloadBytes`, and enforce a fixed-window command budget before dispatching subscribe/unsubscribe commands.
- [x] Ensure invalid commands do not allocate hub channel entries and sustained abuse closes the connection.
- [x] Re-run the package tests; expect pass.

### Task 3: Add PostgreSQL cross-replica fanout

- [x] Add failing tests around a fake fanout: one local publish is delivered locally once, a remote envelope is delivered once, and an origin echo is ignored.
- [x] Define `Envelope` with version, event ID, origin ID, channel, and event; define a narrow `Fanout` interface.
- [x] Refactor the hub into `publishLocal` plus public `Publish`; public publish broadcasts locally and sends one envelope through the injected fanout.
- [x] Implement `PostgresFanout` using a dedicated acquired pgx connection, `LISTEN warwick_realtime`, `pg_notify`, bounded payload validation, cancellation, and capped jittered reacquisition.
- [x] Add an optional integration test using the repository test database to prove two fanout instances exchange one notification.
- [x] Construct the fanout in `NewHandler`, start it for the process lifetime, and retain local-only behavior if publication temporarily fails.
- [x] Run `cd backend && go test ./internal/realtime ./internal/httpapi/realtimehttp -count=1`; expect pass.

### Task 4: Complete mutation publication

- [x] Extend course realtime tests so successful batch-deleted IDs each publish `courses:all`, failed IDs do not, and legacy sync publishes one `sessions:all` event only after success.
- [x] Run `cd backend && go test ./internal/httpapi/courseshttp -run Realtime -count=1`; expect failures for missing publications.
- [x] Capture successful batch IDs inside the committed idempotent transaction and publish afterward.
- [x] Publish a broad `sessions.updated` event after successful legacy sync when sessions were created.
- [x] Re-run course HTTP tests; expect pass.

### Task 5: Preserve distinct frontend events

- [x] Add hook tests using fake timers: duplicate channel/ID events coalesce, two absence IDs both deliver, session plus course both deliver, zero debounce remains immediate, and unmount discards pending work.
- [x] Run `npm test -- src/hooks/useRealtime.test.tsx`; expect failures because the current timer retains only one event.
- [x] Replace the single pending event with an insertion-ordered `Map` keyed by channel and resource ID or event type; flush every retained event through the latest handler ref.
- [x] Re-run the hook tests and existing realtime provider tests; expect pass.

### Task 6: Repair every projection after reconnect

- [x] Add failing provider tests for reconnect subscribers and deterministic tests for reconnect-delay bounds by injecting/stubbing randomness.
- [x] Extend the realtime context with `subscribeReconnect`; add `onReconnect` to `useRealtime` options using a latest callback ref.
- [x] Extend query-bridge tests to require courses and course-roster invalidation after reconnect.
- [x] Add reconnect callbacks to the layout stats loader, absence detail loader, Kanban column loader, and open attendance modal loader.
- [x] Run the related provider, query bridge, layout/page, and attendance tests; expect pass.

### Task 7: Make HTTP authoritative for stats and expose refreshing

- [x] Change the query-bridge stats test to expect invalidation/refetch instead of direct `setQueryData`.
- [x] Add a `useApiQuery` test that changes URL while retaining previous operational data and asserts `loading=false`, `refreshing=true`, then `refreshing=false` after resolution.
- [x] Run `npm test -- src/realtime/queryBridge.test.ts src/test/useApiQuery.test.ts`; expect failures.
- [x] Remove direct stats payload installation; invalidate the stats root like every other event.
- [x] Move the layout badge to an active TanStack stats query and use realtime only for invalidation/reconnect recovery.
- [x] Add `refreshing: query.isFetching` to `UseApiQueryResult` and use it for the teacher dashboard month transition indicator.
- [x] Re-run the focused frontend tests; expect pass.

### Task 8: Documentation and complete verification

- [x] Update `docs/frontend-cache-realtime.md` with PostgreSQL fanout, allowed channels, slow-client disconnect behavior, reconnect convergence, and HTTP-authoritative statistics.
- [x] Run `npm run typecheck`; expect exit 0.
- [x] Run `npm test`; expect at least the 520-test baseline plus the new regression tests, with no failures.
- [x] Run `npm run build`; expect exit 0.
- [x] Run `cd backend && go test ./...`; expect exit 0.
- [x] Run `git diff --check`; expect exit 0.
- [x] Review the final diff for mixed-version compatibility, goroutine cleanup, context cancellation, queue bounds, reconnect amplification, and accidental edits to unrelated user work.

## Acceptance Criteria

- [x] Message 17 disconnects a client whose 16-message queue is full, and the browser observes closure.
- [x] Unsupported channels and abusive inbound commands consume bounded resources.
- [ ] Two backend instances exchange realtime events through PostgreSQL without origin duplication. (Implementation and fake two-instance coverage pass; the live PostgreSQL test requires `TEST_DATABASE_URL`.)
- [x] Every distinct frontend event in a debounce window is delivered once.
- [x] Reconnect repairs query-backed and local-state consumers.
- [x] Every course mutation path that affects cached projections publishes the documented event.
- [x] Statistics are refreshed from HTTP and cannot regress from event reordering.
- [x] Background refresh is distinguishable from initial loading.
- [x] All full verification gates pass against the recorded baseline.
