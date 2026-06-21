import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { queryKeys } from "@/query/cache";
import { applyRealtimeEvent, createRealtimeInvalidationBatcher, invalidateRealtimeBackedQueries } from "./queryBridge";

function seededClient() {
  const client = new QueryClient();
  client.setQueryData(queryKeys.sessions.list("range"), ["session"]);
  client.setQueryData(queryKeys.attendance.detail("session-1"), ["attendance"]);
  client.setQueryData(queryKeys.operationsCalendar.range("range"), { sessions: [] });
  client.setQueryData(queryKeys.absences.list("active"), ["absence"]);
  client.setQueryData(queryKeys.absences.detail("absence-1"), { id: "absence-1" });
  client.setQueryData(queryKeys.absenceStats, { pending_count: 1 });
  client.setQueryData(queryKeys.teacherDashboards.detail("teacher-1", "2026-06-01"), { sessions: [] });
  client.setQueryData(queryKeys.courses.list("active"), ["course"]);
  client.setQueryData(queryKeys.courseRosters.detail("course-1"), ["student"]);
  return client;
}

describe("realtime query bridge", () => {
  it("marks all session-derived projections stale", async () => {
    const client = seededClient();
    await applyRealtimeEvent(client, { type: "session.updated", channel: "sessions:all", id: "session-1" });

    expect(client.getQueryState(queryKeys.sessions.list("range"))?.isInvalidated).toBe(true);
    expect(client.getQueryState(queryKeys.attendance.detail("session-1"))?.isInvalidated).toBe(true);
    expect(client.getQueryState(queryKeys.operationsCalendar.range("range"))?.isInvalidated).toBe(true);
    expect(client.getQueryState(queryKeys.teacherDashboards.detail("teacher-1", "2026-06-01"))?.isInvalidated).toBe(true);
  });

  it("invalidates absence stats instead of trusting event ordering", async () => {
    const client = seededClient();
    await applyRealtimeEvent(client, {
      type: "absent.stats.updated",
      channel: "absent:stats",
      payload: { pending_count: 7, reviewed_count: 2 },
    });

    expect(client.getQueryData(queryKeys.absenceStats)).toEqual({ pending_count: 1 });
    expect(client.getQueryState(queryKeys.absenceStats)?.isInvalidated).toBe(true);
  });

  it("does not replace valid stats with a malformed payload", async () => {
    const client = seededClient();
    await applyRealtimeEvent(client, { type: "absent.stats.updated", channel: "absent:stats", payload: {} });
    expect(client.getQueryData(queryKeys.absenceStats)).toEqual({ pending_count: 1 });
  });

  it("invalidates every active operational family after reconnect", async () => {
    const client = seededClient();
    await invalidateRealtimeBackedQueries(client);

    expect(client.getQueryState(queryKeys.sessions.list("range"))?.isInvalidated).toBe(true);
    expect(client.getQueryState(queryKeys.absences.list("active"))?.isInvalidated).toBe(true);
    expect(client.getQueryState(queryKeys.teacherDashboards.detail("teacher-1", "2026-06-01"))?.isInvalidated).toBe(true);
    expect(client.getQueryState(queryKeys.courses.list("active"))?.isInvalidated).toBe(true);
    expect(client.getQueryState(queryKeys.courseRosters.detail("course-1"))?.isInvalidated).toBe(true);
  });

  it("coalesces duplicate domain events within a short burst", async () => {
    vi.useFakeTimers();
    const client = seededClient();
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const batcher = createRealtimeInvalidationBatcher(client, 100);

    batcher.enqueue({ type: "session.updated", channel: "sessions:all", id: "session-1" });
    batcher.enqueue({ type: "session.updated", channel: "sessions:all", id: "session-2" });
    await vi.advanceTimersByTimeAsync(100);

    expect(invalidate).toHaveBeenCalledTimes(4);
    batcher.dispose();
    vi.useRealTimers();
  });
});
