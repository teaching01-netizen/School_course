import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import useInstituteMeta from "../hooks/useInstituteMeta";
import useLookups from "@/features/scheduling/hooks/useLookups";
import type { Course } from "@/features/courses/types";
import { queryClient, queryKeys } from "../query/cache";
import { useOperationalQuery } from "../query/useOperationalQuery";
import { formatUTCToZone, zoneLocalInputToUTCISO } from "../utils/timezone";
import { yyyyMmDd, type Session } from "@/types";
import RoomAssignDropdown from "../components/RoomAssignDropdown";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import EmptyState from "../components/ui/EmptyState";

type BulkUpdateResult = {
  id: string;
  status: "updated" | "conflict" | "stale_edit" | "error";
  change_id?: string;
  session?: Session;
  error?: string;
  details?: unknown;
};

type BulkUpdateResponse = {
  batch_id: string;
  results: BulkUpdateResult[];
};

/**
 * Rooms claimed by another session that overlaps `session` are busy and must
 * not be offered as assignment targets. Adjacent sessions (end === start) do
 * not conflict — the DB uses half-open `[)` ranges.
 */
export function computeBusyRooms(session: Session, sessions: Session[], zone: string): Map<string, string> {
  const start = Date.parse(session.start_at);
  const end = Date.parse(session.end_at);
  const busy = new Map<string, string>();
  if (!Number.isFinite(start) || !Number.isFinite(end)) return busy;
  for (const other of sessions) {
    if (other.id === session.id || !other.room_id) continue;
    const oStart = Date.parse(other.start_at);
    const oEnd = Date.parse(other.end_at);
    if (!Number.isFinite(oStart) || !Number.isFinite(oEnd)) continue;
    if (oStart < end && oEnd > start) {
      const from = formatUTCToZone(other.start_at, zone, "HH:mm");
      const to = formatUTCToZone(other.end_at, zone, "HH:mm");
      busy.set(other.room_id, `Busy ${from}–${to}`);
    }
  }
  return busy;
}

function timeRange(session: Session, zone: string): string {
  const from = formatUTCToZone(session.start_at, zone, "HH:mm");
  const to = formatUTCToZone(session.end_at, zone, "HH:mm");
  return `${from}–${to}`;
}

export default function AssignRooms() {
  const { addToast } = useToast();
  const { instituteTZ } = useInstituteMeta();
  const { rooms, courseById, teacherById, loading: lookupsLoading } = useLookups();
  const zone = instituteTZ ?? "Asia/Bangkok";
  const [dateStr, setDateStr] = useState(() => yyyyMmDd(new Date()));
  const [pendingIds, setPendingIds] = useState<ReadonlySet<string>>(() => new Set());

  // Courses archived (or deleted) after a session was created are absent from
  // the live-only /api/v1/courses list, so resolve those ids individually via
  // /courses/{id} (which has no live-only filter) instead of rendering the raw
  // course uuid in the Subject column.
  const [resolvedCourses, setResolvedCourses] = useState<Map<string, Course>>(() => new Map());
  const [failedCourseIds, setFailedCourseIds] = useState<ReadonlySet<string>>(() => new Set());
  const requestedCourseIds = useRef<Set<string>>(new Set());

  const dayUrl = useMemo(() => {
    const start = zoneLocalInputToUTCISO(`${dateStr}T00:00`, zone);
    const end = zoneLocalInputToUTCISO(`${dateStr}T23:59`, zone);
    if (!start || !end) return null;
    return `/api/v1/sessions?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`;
  }, [dateStr, zone]);

  const listKey = dayUrl ? queryKeys.sessions.list(dayUrl) : null;
  const sessionsQuery = useOperationalQuery<Session[]>(listKey ?? ["sessions", "list", "invalid"], dayUrl);
  const sessions = dayUrl ? (sessionsQuery.data ?? []) : [];
  const loading = dayUrl != null && sessionsQuery.isPending;

  const sorted = useMemo(
    () => [...sessions].sort((a, b) => Date.parse(a.start_at) - Date.parse(b.start_at)),
    [sessions],
  );
  const unassignedCount = useMemo(() => sessions.filter((s) => !s.room_id).length, [sessions]);

  useEffect(() => {
    // Wait for the lookups list so active courses are not re-fetched one-by-one.
    if (lookupsLoading) return;
    const missing: string[] = [];
    for (const session of sorted) {
      const id = session.course_id;
      if (
        id &&
        !courseById.has(id) &&
        !resolvedCourses.has(id) &&
        !failedCourseIds.has(id) &&
        !requestedCourseIds.current.has(id)
      ) {
        missing.push(id);
      }
    }
    for (const id of new Set(missing)) {
      requestedCourseIds.current.add(id);
      apiJson<Course>(`/api/v1/courses/${id}`)
        .then((course) => {
          setResolvedCourses((prev) => {
            const next = new Map(prev);
            next.set(id, course);
            return next;
          });
        })
        .catch(() => {
          setFailedCourseIds((prev) => {
            const next = new Set(prev);
            next.add(id);
            return next;
          });
        });
    }
  }, [courseById, failedCourseIds, lookupsLoading, resolvedCourses, sorted]);

  const commit = useCallback(
    async (session: Session, roomId: string | null) => {
      if (pendingIds.has(session.id) || !dayUrl || !listKey) return;
      setPendingIds((prev) => new Set(prev).add(session.id));
      queryClient.setQueryData<Session[]>(listKey, (prev) =>
        prev?.map((s) => (s.id === session.id ? { ...s, room_id: roomId } : s)),
      );
      try {
        const res = await apiJson<BulkUpdateResponse>("/api/v1/sessions/bulk-update", {
          method: "POST",
          body: JSON.stringify({
            updates: [{ id: session.id, expected_version: session.version, room_id: roomId }],
          }),
        });
        const result = res.results[0];
        if (result?.status === "updated" && result.session) {
          const updated = result.session;
          queryClient.setQueryData<Session[]>(listKey, (prev) =>
            prev?.map((s) =>
              s.id === updated.id
                ? { ...s, room_id: updated.room_id, version: updated.version }
                : s,
            ),
          );
        } else if (result?.status === "stale_edit") {
          await sessionsQuery.refetch();
          addToast("warning", "This session changed elsewhere — reloaded. Try again.");
        } else if (result?.status === "conflict") {
          await sessionsQuery.refetch();
          addToast("warning", result.error ?? "Room is no longer available for this time.");
        } else {
          await sessionsQuery.refetch();
          addToast("error", result?.error ?? "Failed to assign room.");
        }
      } catch (err) {
        await sessionsQuery.refetch();
        addToast("error", err instanceof Error ? err.message : "Failed to assign room.");
      } finally {
        setPendingIds((prev) => {
          const next = new Set(prev);
          next.delete(session.id);
          return next;
        });
      }
    },
    [addToast, dayUrl, listKey, pendingIds, sessionsQuery],
  );

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <input
          type="date"
          value={dateStr}
          onChange={(e) => setDateStr(e.target.value)}
          className="px-2 py-1 text-sm border var(--color-wi-line) rounded-sm"
        />
        <p className="text-xs text-[var(--color-wi-text-light)]">
          {sessions.length} session{sessions.length !== 1 ? "s" : ""} ·{" "}
          {unassignedCount} unassigned
        </p>
      </div>

      {loading ? (
        <LoadingSkeleton type="table" lines={3} />
      ) : sorted.length === 0 ? (
        <EmptyState message={`No sessions on ${dateStr}.`} />
      ) : (
        <div>
          <div className="grid grid-cols-[88px_minmax(0,1fr)_152px_minmax(240px,auto)] items-center gap-4 border-b var(--color-wi-line) px-3 pb-1.5 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">
            <span>Time</span>
            <span>Subject</span>
            <span>Teacher</span>
            <span className="text-right">Room</span>
          </div>
          <ul className="divide-y divide-wi-line">
            {sorted.map((session) => {
              const course =
                courseById.get(session.course_id) ?? resolvedCourses.get(session.course_id);
              const teacher = teacherById.get(session.teacher_id);
              const busy = computeBusyRooms(session, sorted, zone);
              const pending = pendingIds.has(session.id);
              const subject = course?.subject_name || course?.name || "Unknown course";
              return (
                <li
                  key={session.id}
                  className={`grid grid-cols-[88px_minmax(0,1fr)_152px_minmax(240px,auto)] items-center gap-4 border-l-2 px-3 py-2 transition-colors duration-150 motion-reduce:transition-none ${
                    session.room_id
                      ? "border-transparent"
                      : "border-[var(--color-wi-amber)] bg-[var(--color-wi-amber-bg)]"
                  }`}
                >
                  <span className="font-mono text-xs text-[var(--color-wi-text)]">
                    {timeRange(session, zone)}
                  </span>
                  <span className="truncate text-[13px] font-medium text-[var(--color-wi-text)]" title={subject}>
                    {subject}
                  </span>
                  <span className="truncate text-xs text-[var(--color-wi-text-light)]" title={teacher ? teacher.full_name || teacher.username : session.teacher_id}>
                    {teacher ? teacher.full_name || teacher.username : session.teacher_id}
                  </span>
                  <span className="flex items-center justify-end gap-2">
                    {!session.room_id && (
                      <span className="shrink-0 text-[11px] font-semibold text-[var(--color-wi-amber)]">
                        Unassigned
                      </span>
                    )}
                    <RoomAssignDropdown
                      value={session.room_id}
                      onCommit={(roomId) => void commit(session, roomId)}
                      rooms={rooms}
                      busy={busy}
                      disabled={pending}
                      saving={pending}
                    />
                  </span>
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </div>
  );
}