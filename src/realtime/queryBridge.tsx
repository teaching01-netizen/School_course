import { useEffect, useMemo } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useRealtime, type RealtimeEvent } from "@/hooks/useRealtime";
import { queryKeys } from "@/query/cache";
import { invalidationKeysForEvent } from "./invalidation";

export async function applyRealtimeEvent(client: QueryClient, event: RealtimeEvent): Promise<void> {
  await Promise.all(
    invalidationKeysForEvent(event).map((queryKey) => client.invalidateQueries({ queryKey, refetchType: "active" })),
  );
}

export async function invalidateRealtimeBackedQueries(client: QueryClient): Promise<void> {
  const operationalRoots = [
    queryKeys.sessions.all,
    queryKeys.attendance.all,
    queryKeys.operationsCalendar.all,
    queryKeys.absences.all,
    queryKeys.absenceStats,
    queryKeys.teacherDashboards.all,
    queryKeys.courses.all,
    queryKeys.courseRosters.all,
  ];
  await Promise.all(operationalRoots.map((queryKey) => client.invalidateQueries({ queryKey, refetchType: "active" })));
}

export function createRealtimeInvalidationBatcher(client: QueryClient, delayMs = 100) {
  const pending = new Map<string, RealtimeEvent>();
  let timer: ReturnType<typeof setTimeout> | null = null;

  const flush = async () => {
    const events = [...pending.values()];
    pending.clear();
    timer = null;
    await Promise.all(events.map((event) => applyRealtimeEvent(client, event)));
  };

  return {
    enqueue(event: RealtimeEvent) {
      const key = event.channel === "courses:all" ? `${event.channel}:${event.id ?? "all"}` : event.channel;
      pending.set(key, event);
      if (timer == null) timer = setTimeout(() => { void flush(); }, delayMs);
    },
    flush,
    dispose() {
      if (timer != null) clearTimeout(timer);
      timer = null;
      pending.clear();
    },
  };
}

export function RealtimeQueryBridge() {
  const client = useQueryClient();
  const batcher = useMemo(() => createRealtimeInvalidationBatcher(client), [client]);
  useEffect(() => () => batcher.dispose(), [batcher]);
  useRealtime(
    ["sessions:all", "absent:all", "absent:stats", "courses:all"],
    (event) => batcher.enqueue(event),
  );
  return null;
}
