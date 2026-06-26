import { useState, useCallback } from "react";
import { apiJson } from "@/api/client";
import type { Student } from "@/types/shared";
import { formatConflictToastMessage } from "@/utils/conflictErrors";
import { cachePolicies, queryClient, queryKeys } from "@/query/cache";
import { useRealtime } from "@/hooks/useRealtime";
import type { Session, AttendanceOverride } from "../types";

export interface UseAttendanceModalReturn {
  session: Session | null;
  roster: Student[];
  overrides: AttendanceOverride[];
  loading: boolean;
  includeWcode: string;
  setIncludeWcode: React.Dispatch<React.SetStateAction<string>>;
  includeAdding: boolean;
  openAttendance: (sess: Session) => Promise<void>;
  closeAttendance: () => void;
  upsertAttendance: (studentId: string, status: "included" | "excluded") => Promise<void>;
  deleteAttendance: (studentId: string) => Promise<void>;
  addIncludedByWcode: () => Promise<void>;
}

export function useAttendanceModal(
  addToast: (type: "success" | "error" | "warning" | "info", msg: string) => void,
): UseAttendanceModalReturn {
  const [session, setSession] = useState<Session | null>(null);
  const [roster, setRoster] = useState<Student[]>([]);
  const [overrides, setOverrides] = useState<AttendanceOverride[]>([]);
  const [loading, setLoading] = useState(false);
  const [includeWcode, setIncludeWcode] = useState("");
  const [includeAdding, setIncludeAdding] = useState(false);

  const loadAttendanceState = useCallback(async (sess: Session) => {
    const [rosterData, overridesData] = await Promise.all([
      queryClient.fetchQuery({
        queryKey: queryKeys.courseRosters.detail(sess.course_id),
        queryFn: () => apiJson<Student[]>(`/api/v1/courses/${sess.course_id}/students`, { method: "GET" }),
        ...cachePolicies.semiStatic,
      }),
      queryClient.fetchQuery({
        queryKey: queryKeys.attendance.detail(sess.id),
        queryFn: () => apiJson<AttendanceOverride[]>(`/api/v1/sessions/${sess.id}/attendance`, { method: "GET" }),
        ...cachePolicies.operational,
      }),
    ]);
    const rosterIds = new Set(rosterData.map((s) => s.id));
    const missing = overridesData.map((o) => o.student_id).filter((sid) => !rosterIds.has(sid));
    const extra = missing.length
      ? await Promise.all(missing.map((sid) => queryClient.fetchQuery({
          queryKey: ["students", "detail", sid],
          queryFn: () => apiJson<Student>(`/api/v1/students/${sid}`, { method: "GET" }),
          ...cachePolicies.semiStatic,
        })))
      : [];
    const merged = [...rosterData, ...extra].filter(
      (value, index, array) => array.findIndex((candidate) => candidate.id === value.id) === index,
    );
    setRoster(merged);
    setOverrides(overridesData);
  }, []);

  const openAttendance = useCallback(async (sess: Session) => {
    try {
      setLoading(true);
      setSession(sess);
      await loadAttendanceState(sess);
      setIncludeWcode("");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load attendance");
      setSession(null);
    } finally {
      setLoading(false);
    }
  }, [addToast, loadAttendanceState]);

  const closeAttendance = useCallback(() => {
    setSession(null);
    setRoster([]);
    setOverrides([]);
  }, []);

  const invalidateAttendanceProjections = useCallback(async (sess: Session) => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.sessions.all, refetchType: "active" }),
      queryClient.invalidateQueries({ queryKey: queryKeys.attendance.detail(sess.id), refetchType: "none" }),
      queryClient.invalidateQueries({ queryKey: queryKeys.teacherDashboards.all, refetchType: "active" }),
    ]);
  }, []);

  useRealtime(
    ["sessions:all", "courses:all"],
    (event) => {
      if (!session) return;
      if (event.channel === "sessions:all" && event.id === session.id) void loadAttendanceState(session);
      if (event.channel === "courses:all" && event.id === session.course_id) void loadAttendanceState(session);
    },
    {
      enabled: session != null,
      debounceMs: 100,
      onReconnect: () => {
        if (!session) return;
        void Promise.all([
          queryClient.invalidateQueries({ queryKey: queryKeys.courseRosters.detail(session.course_id), refetchType: "none" }),
          queryClient.invalidateQueries({ queryKey: queryKeys.attendance.detail(session.id), refetchType: "none" }),
        ]).then(() => loadAttendanceState(session));
      },
    },
  );

  const upsertAttendance = useCallback(async (studentId: string, status: "included" | "excluded") => {
    if (!session) return;
    await apiJson(`/api/v1/sessions/${session.id}/attendance`, {
      method: "PUT",
      body: JSON.stringify({ student_id: studentId, status }),
    });
    await invalidateAttendanceProjections(session);
    await loadAttendanceState(session);
  }, [session, invalidateAttendanceProjections, loadAttendanceState]);

  const deleteAttendance = useCallback(async (studentId: string) => {
    if (!session) return;
    await apiJson(`/api/v1/sessions/${session.id}/attendance/${studentId}`, { method: "DELETE" });
    await invalidateAttendanceProjections(session);
    await loadAttendanceState(session);
  }, [session, invalidateAttendanceProjections, loadAttendanceState]);

  const addIncludedByWcode = useCallback(async () => {
    if (!session) return;
    const w = includeWcode.trim();
    if (!w) return;
    try {
      setIncludeAdding(true);
      const st = await apiJson<Student>(`/api/v1/students/by-wcode?wcode=${encodeURIComponent(w)}`, { method: "GET" });
      await upsertAttendance(st.id, "included");
      addToast("success", `Included ${st.wcode}`);
      setIncludeWcode("");
    } catch (err) {
      addToast("error", formatConflictToastMessage(err, "Failed to include student"));
    } finally {
      setIncludeAdding(false);
    }
  }, [session?.id, includeWcode, addToast, upsertAttendance]);

  return {
    session,
    roster,
    overrides,
    loading,
    includeWcode,
    setIncludeWcode,
    includeAdding,
    openAttendance,
    closeAttendance,
    upsertAttendance,
    deleteAttendance,
    addIncludedByWcode,
  };
}
