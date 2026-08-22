import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ApiRequestError, apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import { clampDateRange } from "../utils/time";
import { formatZoneDateKey, utcISOToZoneDate, utcISOToZoneLocalInput, zoneLocalInputToUTCISO } from "../utils/timezone";
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import Select from "../components/ui/Select";
import FormField from "../components/ui/FormField";
import FormErrorSummary from "../components/ui/FormErrorSummary";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import EmptyState from "../components/ui/EmptyState";
import Modal from "../components/Modal";
import ConfirmModal from "../components/ConfirmModal";
import ScheduleFilters from "../components/ScheduleFilters";
import { PreflightIndicator, getSaveButtonLabel } from "../components/PreflightIndicator";
import SessionOccurrenceForm from "../components/SessionOccurrenceForm";
import SeriesFormFields from "../components/SeriesFormFields";
import AttendancePanel from "../components/AttendancePanel";
import { SessionWeekCard, SessionTableRow, type SessionInlineEdit } from "../components/schedule/SessionCards";
import ImpactAcknowledgementModal, { type ImpactSummary } from "../components/scheduleImpact/ImpactAcknowledgementModal";
import useInstituteMeta from "../hooks/useInstituteMeta";
import useLookups from "@/features/scheduling/hooks/useLookups";
import { useCreateSession } from "@/features/scheduling/hooks/useCreateSession";
import { useEditSession } from "@/features/scheduling/hooks/useEditSession";
import { useAttendanceModal } from "@/features/scheduling/hooks/useAttendanceModal";
import { usePreflight, type PreflightParams } from "@/features/scheduling/hooks/usePreflight";
import { useDebouncedPreflight } from "@/features/scheduling/hooks/useDebouncedPreflight";
import usePreflightGate from "@/features/scheduling/hooks/usePreflightGate";
import { validateSeriesPreflight, type SeriesPreflightForm } from "../utils/preflight";
import { useFormValidation } from "../hooks/useFormValidation";
import TypeaheadSelect from "../components/TypeaheadSelect";
import {
  localDateTimeToUTCISO,
  yyyyMmDd,
  type Session,
  type StaleEditDetails,
} from "@/types";
import { queryClient, queryKeys } from "../query/cache";
import { mapListItems, useSmartMutation } from "../query/useSmartMutation";
import { useOperationalQuery } from "../query/useOperationalQuery";

export default function Schedule() {
  const { addToast } = useToast();
  const today = useMemo(() => new Date(), []);
  const { serverNow, instituteTZ } = useInstituteMeta();
  const { courses, rooms, teachers, courseById, roomById, teacherById, courseOptions, teacherOptions } = useLookups();
  const [startDate, setStartDate] = useState(yyyyMmDd(today));
  const [endDate, setEndDate] = useState(yyyyMmDd(new Date(today.getTime() + 7 * 24 * 60 * 60 * 1000)));
  const [startTime, setStartTime] = useState("00:00");
  const [endTime, setEndTime] = useState("23:59");
  const zone = instituteTZ ?? "Asia/Bangkok";

  const [viewMode, setViewMode] = useState<"week" | "table">("week");
  const [cancelingId, setCancelingId] = useState<string | null>(null);

  const sessionRequest = useMemo(() => {
    const { endDate: cappedEnd, clamped } = clampDateRange(startDate, endDate);
    const start = zoneLocalInputToUTCISO(`${startDate}T${startTime}`, zone);
    const end = zoneLocalInputToUTCISO(`${cappedEnd}T${endTime}`, zone);
    const url = start && end
      ? `/api/v1/sessions?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`
      : null;
    return { url, clamped };
  }, [endDate, endTime, startDate, startTime, zone]);
  const sessionsQuery = useOperationalQuery<Session[]>(
    queryKeys.sessions.list(sessionRequest.url ?? "invalid"),
    sessionRequest.url,
  );
  const sessions = sessionRequest.url ? sessionsQuery.data ?? [] : [];
  // Impact summary is fetched as a query keyed by the session id+version
  // signature: content changes refetch it, identical polls are free, and
  // switching back to a previously viewed range reuses the cached summary.
  const sessionSignature = useMemo(
    () => sessions.map((session) => `${session.id}:${session.version}`).join(","),
    [sessions],
  );
  const scheduleIssuesQuery = useQuery<{ sessions: Record<string, { open_count: number }> }>({
    queryKey: ["schedule-issues-summary", sessionSignature],
    queryFn: ({ signal }) =>
      apiJson<{ sessions: Record<string, { open_count: number }> }>("/api/v1/operations/schedule-issues/summary", {
        method: "POST",
        body: JSON.stringify({ session_ids: sessions.map((session) => session.id) }),
        signal,
      }),
    enabled: sessions.length > 0,
    staleTime: 30_000,
    gcTime: 5 * 60_000,
  }, queryClient);
  const impactedSessionIDs = useMemo(() => {
    const data = scheduleIssuesQuery.data?.sessions;
    if (data == null || typeof data !== "object" || Array.isArray(data)) return new Set<string>();
    return new Set(
      Object.entries(data)
        .filter(([, summary]) => (summary?.open_count ?? 0) > 0)
        .map(([sessionID]) => sessionID),
    );
  }, [scheduleIssuesQuery.data]);
  // Optimistic session mutations: the cached list is patched instantly and the
  // post-settle invalidation reconciles with server truth (see useSmartMutation).
  const cancelSessionMutation = useSmartMutation<Session, unknown>({
    mutationFn: (session) =>
      apiJson(`/api/v1/sessions/${session.id}`, {
        method: "DELETE",
        body: JSON.stringify({ expected_version: session.version }),
      }),
    optimistic: (session) => [
      {
        keyPrefix: queryKeys.sessions.all,
        patch: (data) => mapListItems<Session>(data, (item) => (item.id === session.id ? null : item)),
      },
    ],
    invalidates: [queryKeys.sessions.all],
  });
  const inlineEditMutation = useSmartMutation<
    { sessionId: string; expectedVersion: number; course_id: string; room_id: string | null; teacher_id: string; start_at: string; end_at: string; acknowledgeImpact: boolean },
    { change_id?: string }
  >({
    mutationFn: (vars) =>
      apiJson<{ change_id?: string }>(`/api/v1/sessions/${vars.sessionId}`, {
        method: "PATCH",
        body: JSON.stringify({
          expected_version: vars.expectedVersion,
          course_id: vars.course_id,
          room_id: vars.room_id,
          teacher_id: vars.teacher_id,
          start_at: vars.start_at,
          end_at: vars.end_at,
          ...(vars.acknowledgeImpact ? { acknowledge_impact: true } : {}),
        }),
      }),
    optimistic: (vars) => [
      {
        keyPrefix: queryKeys.sessions.all,
        patch: (data) =>
          mapListItems<Session>(data, (item) =>
            item.id === vars.sessionId
              ? { ...item, course_id: vars.course_id, room_id: vars.room_id, teacher_id: vars.teacher_id, start_at: vars.start_at, end_at: vars.end_at }
              : item,
          ),
      },
    ],
    invalidates: [queryKeys.sessions.all],
  });
  const loading = sessionRequest.url != null && sessionsQuery.isPending;
  const refetchSessions = sessionsQuery.refetch;
  const load = useCallback(async () => {
    if (!sessionRequest.url) {
      addToast("error", "Invalid date/time range");
      return;
    }
    await refetchSessions();
  }, [addToast, refetchSessions, sessionRequest.url]);

  useEffect(() => {
    if (sessionRequest.clamped) addToast("info", "Date range capped to 14 days");
    if (!sessionRequest.url) addToast("error", "Invalid date/time range");
  }, [addToast, sessionRequest.clamped, sessionRequest.url]);

  useEffect(() => {
    if (sessionsQuery.error) addToast("error", sessionsQuery.error.message || "Failed to load sessions");
  }, [addToast, sessionsQuery.error]);

  // --- Create Session hook ---
  const create = useCreateSession(load, addToast, zone);
  // --- Edit Session hook ---
  const edit = useEditSession(load, addToast, zone);
  const [inlineEditSession, setInlineEditSession] = useState<Session | null>(null);
  const [inlineEditForm, setInlineEditForm] = useState({
    course_id: "",
    room_id: "",
    teacher_id: "",
    start_local: "",
    end_local: "",
  });
  const [inlineSaving, setInlineSaving] = useState(false);
  const [inlineImpact, setInlineImpact] = useState<ImpactSummary | null>(null);
  const inlinePreflight = usePreflight();
  const inlineGate = usePreflightGate(inlinePreflight, {
    requiredFields: [inlineEditForm.course_id, inlineEditForm.teacher_id, inlineEditForm.start_local, inlineEditForm.end_local],
  });
  // --- Attendance modal hook ---
  const attendance = useAttendanceModal(addToast);

  // --- Series create ---
  const [seriesOpen, setSeriesOpen] = useState(false);
  const [seriesCreating, setSeriesCreating] = useState(false);
  const [seriesUseCount, setSeriesUseCount] = useState(false);
  const [seriesForm, setSeriesForm] = useState({
    course_id: "",
    room_id: "" as string,
    teacher_id: "",
    weekdays: [false, false, false, false, false, false, false] as boolean[],
    start_local_time: "16:00",
    duration_minutes: 120,
    start_date: startDate,
    end_date: endDate,
    count: 10,
  });
  const seriesPreflight = usePreflight("preflight_series");
  const seriesValidatedForm = useMemo(
    () => validateSeriesPreflight(seriesForm as SeriesPreflightForm, seriesUseCount),
    [seriesForm, seriesUseCount]
  );
  const seriesGate = usePreflightGate(seriesPreflight, {
    requiredFields: [
      seriesForm.course_id,
      seriesForm.teacher_id,
      seriesForm.start_local_time,
      seriesForm.duration_minutes > 0 ? String(seriesForm.duration_minutes) : "",
      seriesForm.start_date,
      seriesForm.weekdays.some(Boolean) ? "1" : "",
      seriesUseCount ? (Number.isFinite(seriesForm.count) && seriesForm.count > 0 ? String(seriesForm.count) : "") : seriesForm.end_date,
    ],
    isFormValid: seriesValidatedForm != null,
  });

  const seriesSchema = {
    course_id: [{ type: "required" as const, message: "Course is required" }],
    teacher_id: [{ type: "required" as const, message: "Teacher is required" }],
    start_local_time: [{ type: "required" as const, message: "Start time is required" }],
    duration_minutes: [{ type: "min" as const, value: 1, message: "Duration must be at least 1 minute" }],
    start_date: [{ type: "required" as const, message: "Start date is required" }],
  };
  const seriesValidation = useFormValidation(seriesSchema, {
    course_id: seriesForm.course_id,
    teacher_id: seriesForm.teacher_id,
    start_local_time: seriesForm.start_local_time,
    duration_minutes: seriesForm.duration_minutes,
    start_date: seriesForm.start_date,
  });

  // --- Edit Series (This & Future) ---
  const [editSeriesOpen, setEditSeriesOpen] = useState(false);
  const [editSeriesLoading, setEditSeriesLoading] = useState(false);
  const [editSeriesUseCount, setEditSeriesUseCount] = useState(false);
  const [editSeriesPivotDate, setEditSeriesPivotDate] = useState<string>("");
  const [editSeriesForm, setEditSeriesForm] = useState<{
    series_id: string;
    expected_version: number;
    course_id: string;
    room_id: string | null;
    teacher_id: string;
    weekdays: boolean[];
    start_local_time: string;
    duration_minutes: number;
    end_date: string;
    count: number;
  } | null>(null);
  const editSeriesPreflight = usePreflight("preflight_series");
  const editSeriesValidatedForm = useMemo(() => {
    if (!editSeriesForm || !editSeriesPivotDate) return null;
    return validateSeriesPreflight(
      {
        course_id: editSeriesForm.course_id,
        room_id: editSeriesForm.room_id ?? "",
        teacher_id: editSeriesForm.teacher_id,
        weekdays: editSeriesForm.weekdays,
        start_local_time: editSeriesForm.start_local_time,
        duration_minutes: editSeriesForm.duration_minutes,
        start_date: editSeriesPivotDate,
        end_date: editSeriesUseCount ? "" : editSeriesForm.end_date,
        count: editSeriesUseCount ? editSeriesForm.count : 0,
      },
      editSeriesUseCount
    );
  }, [editSeriesForm, editSeriesPivotDate, editSeriesUseCount]);
  const editSeriesGate = usePreflightGate(editSeriesPreflight, {
    requiredFields: [
      editSeriesForm?.course_id ?? "",
      editSeriesForm?.teacher_id ?? "",
      editSeriesForm?.start_local_time ?? "",
      editSeriesForm && editSeriesForm.duration_minutes > 0 ? String(editSeriesForm.duration_minutes) : "",
      editSeriesPivotDate,
      editSeriesForm?.weekdays.some(Boolean) ? "1" : "",
      editSeriesUseCount ? (editSeriesForm && Number.isFinite(editSeriesForm.count) && editSeriesForm.count > 0 ? String(editSeriesForm.count) : "") : editSeriesForm?.end_date ?? "",
    ],
    isFormValid: editSeriesValidatedForm != null,
  });

  // --- Edit Series (Future Only / Entire) ---
  const [editSeriesEntireOpen, setEditSeriesEntireOpen] = useState(false);
  const [editSeriesEntireLoading, setEditSeriesEntireLoading] = useState(false);
  const [editSeriesEntireUseCount, setEditSeriesEntireUseCount] = useState(false);
  const [editSeriesEntireFromDate, setEditSeriesEntireFromDate] = useState<string>("");
  const [editSeriesEntireForm, setEditSeriesEntireForm] = useState<{
    series_id: string;
    expected_version: number;
    course_id: string;
    room_id: string | null;
    teacher_id: string;
    weekdays: boolean[];
    start_local_time: string;
    duration_minutes: number;
    end_date: string;
    count: number;
  } | null>(null);
  const editSeriesEntirePreflight = usePreflight("preflight_series");
  const editSeriesEntireValidatedForm = useMemo(() => {
    if (!editSeriesEntireForm || !editSeriesEntireFromDate) return null;
    return validateSeriesPreflight(
      {
        course_id: editSeriesEntireForm.course_id,
        room_id: editSeriesEntireForm.room_id ?? "",
        teacher_id: editSeriesEntireForm.teacher_id,
        weekdays: editSeriesEntireForm.weekdays,
        start_local_time: editSeriesEntireForm.start_local_time,
        duration_minutes: editSeriesEntireForm.duration_minutes,
        start_date: editSeriesEntireFromDate,
        end_date: editSeriesEntireUseCount ? "" : editSeriesEntireForm.end_date,
        count: editSeriesEntireUseCount ? editSeriesEntireForm.count : 0,
      },
      editSeriesEntireUseCount
    );
  }, [editSeriesEntireForm, editSeriesEntireFromDate, editSeriesEntireUseCount]);
  const editSeriesEntireGate = usePreflightGate(editSeriesEntirePreflight, {
    requiredFields: [
      editSeriesEntireForm?.course_id ?? "",
      editSeriesEntireForm?.teacher_id ?? "",
      editSeriesEntireForm?.start_local_time ?? "",
      editSeriesEntireForm && editSeriesEntireForm.duration_minutes > 0 ? String(editSeriesEntireForm.duration_minutes) : "",
      editSeriesEntireFromDate,
      editSeriesEntireForm?.weekdays.some(Boolean) ? "1" : "",
      editSeriesEntireUseCount ? (editSeriesEntireForm && Number.isFinite(editSeriesEntireForm.count) && editSeriesEntireForm.count > 0 ? String(editSeriesEntireForm.count) : "") : editSeriesEntireForm?.end_date ?? "",
    ],
    isFormValid: editSeriesEntireValidatedForm != null,
  });

  // --- Cancel Series ---
  const [cancelSeriesOpen, setCancelSeriesOpen] = useState(false);
  const [cancelSeriesLoading, setCancelSeriesLoading] = useState(false);
  const [cancelSeriesScope, setCancelSeriesScope] = useState<"this_and_future" | "entire_series_future_only">("this_and_future");
  const [cancelSeriesPivotDate, setCancelSeriesPivotDate] = useState<string>("");
  const [cancelSeriesForm, setCancelSeriesForm] = useState<{ series_id: string; expected_version: number } | null>(null);

  const [confirmCancelOccurrence, setConfirmCancelOccurrence] = useState<{ session: Session | null }>({ session: null });
  const [confirmCancelSeriesModal, setConfirmCancelSeriesModal] = useState(false);

  const daysInRange = useMemo(() => {
    const start = new Date(`${startDate}T00:00:00.000Z`);
    const end = new Date(`${endDate}T00:00:00.000Z`);
    if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime()) || end < start) return [];
    const out: Date[] = [];
    for (let d = new Date(start); d <= end && out.length < 14; d = new Date(d.getTime() + 24 * 60 * 60 * 1000)) {
      out.push(d);
    }
    return out;
  }, [startDate, endDate]);

  const sessionsByDay = useMemo(() => {
    const map = new Map<string, Session[]>();
    for (const s of sessions) {
      const key = utcISOToZoneDate(s.start_at, zone);
      if (!key) continue;
      const arr = map.get(key) ?? [];
      arr.push(s);
      map.set(key, arr);
    }
    for (const arr of map.values()) arr.sort((a, b) => a.start_at.localeCompare(b.start_at));
    return map;
  }, [sessions, zone]);

  // --- Cancel occurrence ---
  const cancelOccurrence = useCallback((sess: Session) => {
    setConfirmCancelOccurrence({ session: sess });
  }, []);

  const openInlineEdit = useCallback((sess: Session) => {
    setInlineEditSession(sess);
    setInlineEditForm({
      course_id: sess.course_id,
      room_id: sess.room_id ?? "",
      teacher_id: sess.teacher_id,
      start_local: utcISOToZoneLocalInput(sess.start_at, zone) ?? "",
      end_local: utcISOToZoneLocalInput(sess.end_at, zone) ?? "",
    });
    inlinePreflight.reset();
  }, [inlinePreflight, zone]);

  const closeInlineEdit = useCallback(() => {
    setInlineEditSession(null);
    setInlineEditForm({ course_id: "", room_id: "", teacher_id: "", start_local: "", end_local: "" });
    inlinePreflight.reset();
  }, [inlinePreflight]);

  // Debounced preflight: one network check per settled input window instead of
  // one per keystroke.
  const inlinePreflightParams = useMemo<PreflightParams | null>(() => {
    if (!inlineEditSession) return null;
    if (!inlineEditForm.course_id || !inlineEditForm.teacher_id || !inlineEditForm.start_local || !inlineEditForm.end_local) return null;
    const startISO = localDateTimeToUTCISO(inlineEditForm.start_local, zone);
    const endISO = localDateTimeToUTCISO(inlineEditForm.end_local, zone);
    if (!startISO || !endISO || endISO <= startISO) return null;
    return {
      session_id: inlineEditSession.id,
      course_id: inlineEditForm.course_id,
      room_id: inlineEditForm.room_id || null,
      teacher_id: inlineEditForm.teacher_id,
      start_at: startISO,
      end_at: endISO,
    };
  }, [
    inlineEditSession,
    inlineEditForm.course_id,
    inlineEditForm.room_id,
    inlineEditForm.teacher_id,
    inlineEditForm.start_local,
    inlineEditForm.end_local,
    zone,
  ]);
  useDebouncedPreflight(inlinePreflight, inlinePreflightParams, { enabled: inlineEditSession != null });

  const { canSave: inlineCanSave, isChecking: inlineIsChecking } = inlineGate;

  const submitInlineEdit = useCallback(async (acknowledgeImpact = false) => {
    if (!inlineEditSession) return;
    if (!inlineCanSave) {
      addToast("error", inlineIsChecking ? "Checking availability…" : "Preflight must pass before saving");
      return;
    }
    const startISO = localDateTimeToUTCISO(inlineEditForm.start_local, zone);
    const endISO = localDateTimeToUTCISO(inlineEditForm.end_local, zone);
    if (!startISO || !endISO || endISO <= startISO) {
      addToast("error", "Invalid start/end time");
      return;
    }
    setInlineSaving(true);
    try {
      const roomId = inlineEditForm.room_id || null;
      const preview = await apiJson<{ requires_acknowledgement?: boolean; impact_summary?: ImpactSummary }>(`/api/v1/sessions/${inlineEditSession.id}/change-preview`, {
        method: "POST",
        body: JSON.stringify({
          expected_version: inlineEditSession.version,
          course_id: inlineEditForm.course_id,
          room_id: roomId,
          teacher_id: inlineEditForm.teacher_id,
          start_at: startISO,
          end_at: endISO,
        }),
      });
      if (preview.requires_acknowledgement && !acknowledgeImpact) {
        setInlineImpact(preview.impact_summary ?? {});
        return;
      }
      const result = await inlineEditMutation.mutateAsync({
        sessionId: inlineEditSession.id,
        expectedVersion: inlineEditSession.version,
        course_id: inlineEditForm.course_id,
        room_id: roomId,
        teacher_id: inlineEditForm.teacher_id,
        start_at: startISO,
        end_at: endISO,
        acknowledgeImpact: Boolean(preview.requires_acknowledgement),
      });
      addToast("success", result.change_id ? "Updated session. Impact review queued." : "Updated session");
      closeInlineEdit();
      await load();
    } catch (err) {
      if (err instanceof ApiRequestError) {
        if (err.code === "stale_edit") {
          addToast("error", "Stale edit: reloaded latest session. Please review and save again.");
          const reloaded = await apiJson<Session[]>(`/api/v1/sessions?ids=${inlineEditSession.id}`, { method: "GET" });
          const updated = reloaded[0];
          if (updated) {
            setInlineEditSession(updated);
            setInlineEditForm({
              course_id: updated.course_id,
              room_id: updated.room_id ?? "",
              teacher_id: updated.teacher_id,
              start_local: utcISOToZoneLocalInput(updated.start_at, zone) ?? "",
              end_local: utcISOToZoneLocalInput(updated.end_at, zone) ?? "",
            });
          }
          await load();
          return;
        }
        addToast("error", `${err.code}: ${err.message}`);
        return;
      }
      addToast("error", err instanceof Error ? err.message : "Update failed");
    } finally {
      setInlineSaving(false);
    }
  }, [inlineEditSession, inlineCanSave, inlineIsChecking, inlineEditForm, zone, addToast, closeInlineEdit, load, inlineEditMutation]);

  const handleConfirmCancelOccurrence = async () => {
    const sess = confirmCancelOccurrence.session;
    if (!sess) return;
    setConfirmCancelOccurrence({ session: null });
    setCancelingId(sess.id);
    try {
      await cancelSessionMutation.mutateAsync(sess);
      addToast("success", "Canceled session");
      await load();
    } catch (err) {
      if (err instanceof ApiRequestError) {
        if (err.code === "stale_edit") {
          addToast("error", "Stale edit: reloaded latest session. Please try again.");
          await load();
          return;
        }
        addToast("error", `${err.code}: ${err.message}`);
        return;
      }
      addToast("error", err instanceof Error ? err.message : "Cancel failed");
    } finally {
      setCancelingId(null);
    }
  };

  // --- Open Series create ---
  const openSeries = () => {
    setSeriesOpen(true);
    setSeriesUseCount(false);
    setSeriesForm({
      course_id: courses[0]?.id ?? "",
      room_id: "",
      teacher_id: teachers[0]?.id ?? "",
      weekdays: [false, false, false, false, false, false, false],
      start_local_time: "16:00",
      duration_minutes: 120,
      start_date: startDate,
      end_date: endDate,
      count: 10,
    });
  };

  // --- Open Edit Series (This & Future) ---
  const openEditSeriesThisAndFuture = useCallback(async (sess: Session) => {
    if (!sess.series_id) {
      addToast("error", "This session is not part of a series");
      return;
    }
    const pivot = utcISOToZoneDate(sess.start_at, zone);
    if (!pivot) {
      addToast("error", "Invalid session start time");
      return;
    }
    try {
      setEditSeriesOpen(true);
      setEditSeriesLoading(true);
      setEditSeriesPivotDate(pivot);
      const series = await apiJson<{
        id: string; course_id: string; room_id: string | null; teacher_id: string;
        weekdays: number[]; start_local_time: string; duration_minutes: number;
        start_date: string; end_date: string; count: number | null; version: number;
      }>(`/api/v1/series/${encodeURIComponent(sess.series_id)}`, { method: "GET" });
      const weekdayFlags = [false, false, false, false, false, false, false];
      for (const wd of series.weekdays ?? []) { if (wd >= 0 && wd <= 6) weekdayFlags[wd] = true; }
      const useCount = series.count != null;
      setEditSeriesUseCount(useCount);
      setEditSeriesForm({
        series_id: series.id, expected_version: series.version, course_id: series.course_id,
        room_id: series.room_id, teacher_id: series.teacher_id, weekdays: weekdayFlags,
        start_local_time: series.start_local_time, duration_minutes: series.duration_minutes,
        end_date: series.end_date || "", count: (series.count ?? 10) as number,
      });
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load series");
      setEditSeriesOpen(false);
      setEditSeriesForm(null);
    } finally {
      setEditSeriesLoading(false);
    }
  }, [addToast, zone]);

  // --- Open Edit Series (Entire) ---
  const openEditSeriesEntire = useCallback(async (sess: Session) => {
    if (!sess.series_id) {
      addToast("error", "This session is not part of a series");
      return;
    }
    try {
      setEditSeriesEntireOpen(true);
      setEditSeriesEntireLoading(true);
      const fromDate = serverNow ? utcISOToZoneDate(serverNow, zone) : null;
      setEditSeriesEntireFromDate(fromDate ?? startDate);
      const series = await apiJson<{
        id: string; course_id: string; room_id: string | null; teacher_id: string;
        weekdays: number[]; start_local_time: string; duration_minutes: number;
        start_date: string; end_date: string; count: number | null; version: number;
      }>(`/api/v1/series/${encodeURIComponent(sess.series_id)}`, { method: "GET" });
      const weekdayFlags = [false, false, false, false, false, false, false];
      for (const wd of series.weekdays ?? []) { if (wd >= 0 && wd <= 6) weekdayFlags[wd] = true; }
      const useCount = series.count != null;
      setEditSeriesEntireUseCount(useCount);
      setEditSeriesEntireForm({
        series_id: series.id, expected_version: series.version, course_id: series.course_id,
        room_id: series.room_id, teacher_id: series.teacher_id, weekdays: weekdayFlags,
        start_local_time: series.start_local_time, duration_minutes: series.duration_minutes,
        end_date: series.end_date || "", count: (series.count ?? 10) as number,
      });
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load series");
      setEditSeriesEntireOpen(false);
      setEditSeriesEntireForm(null);
    } finally {
      setEditSeriesEntireLoading(false);
    }
  }, [addToast, serverNow, zone, startDate]);

  // --- Cancel Series ---
  const openCancelSeries = useCallback(async (sess: Session) => {
    if (!sess.series_id) { addToast("error", "This session is not part of a series"); return; }
    const pivot = utcISOToZoneDate(sess.start_at, zone);
    if (!pivot) { addToast("error", "Invalid session start time"); return; }
    try {
      setCancelSeriesOpen(true);
      setCancelSeriesLoading(true);
      setCancelSeriesScope("this_and_future");
      setCancelSeriesPivotDate(pivot);
      const series = await apiJson<{ id: string; version: number }>(`/api/v1/series/${encodeURIComponent(sess.series_id)}`, { method: "GET" });
      setCancelSeriesForm({ series_id: series.id, expected_version: series.version });
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load series");
      setCancelSeriesOpen(false);
      setCancelSeriesForm(null);
    } finally {
      setCancelSeriesLoading(false);
    }
  }, [addToast, zone]);

  const submitCancelSeries = async () => {
    if (!cancelSeriesForm) return;
    if (cancelSeriesScope === "this_and_future" && !cancelSeriesPivotDate) { addToast("error", "pivot_date required"); return; }
    setConfirmCancelSeriesModal(true);
  };

  const handleConfirmCancelSeries = async () => {
    setConfirmCancelSeriesModal(false);
    if (!cancelSeriesForm) return;
    try {
      setCancelSeriesLoading(true);
      await apiJson<{ series_id: string; sessions_canceled: number }>(`/api/v1/series/${encodeURIComponent(cancelSeriesForm.series_id)}/cancel`, {
        method: "POST",
        body: JSON.stringify({ scope: cancelSeriesScope, pivot_date: cancelSeriesScope === "this_and_future" ? cancelSeriesPivotDate : "", expected_version: cancelSeriesForm.expected_version }),
      });
      addToast("success", "Series canceled");
      setCancelSeriesOpen(false);
      setCancelSeriesForm(null);
      await load();
    } catch (err) {
      if (err instanceof ApiRequestError) {
        if (err.code === "stale_edit") {
          const details = err.details as StaleEditDetails;
          const cur = details?.current;
          if (cur && typeof cur.version === "number") {
            setCancelSeriesForm((p) => (p ? { ...p, expected_version: cur.version } : p));
          }
          addToast("error", "Stale edit: reloaded latest series version. Please retry.");
          return;
        }
        addToast("error", `${err.code}: ${err.message}`);
        return;
      }
      addToast("error", err instanceof Error ? err.message : "Cancel failed");
    } finally {
      setCancelSeriesLoading(false);
    }
  };

  // Stable handlers + a per-session inline-edit bundle keep memoized cards
  // from re-rendering while one card's inline form is being typed into.
  const handleAttendance = useCallback((sess: Session) => { void attendance.openAttendance(sess); }, [attendance.openAttendance]);
  const handleEditSeriesTandF = useCallback((sess: Session) => { void openEditSeriesThisAndFuture(sess); }, [openEditSeriesThisAndFuture]);
  const handleEditSeriesEntire = useCallback((sess: Session) => { void openEditSeriesEntire(sess); }, [openEditSeriesEntire]);
  const handleCancelSeries = useCallback((sess: Session) => { void openCancelSeries(sess); }, [openCancelSeries]);
  const activeInlineEdit: SessionInlineEdit | null = inlineEditSession
    ? {
        form: inlineEditForm,
        setForm: setInlineEditForm,
        preflight: inlinePreflight,
        canSave: inlineCanSave,
        isChecking: inlineIsChecking,
        saving: inlineSaving,
        submit: () => { void submitInlineEdit(); },
        close: closeInlineEdit,
      }
    : null;

  // --- Series preflights (debounced: one check per settled input window) ---
  const seriesPreflightParams = useMemo<PreflightParams | null>(() => {
    if (!seriesOpen || !seriesValidatedForm) return null;
    return {
      course_id: seriesForm.course_id,
      teacher_id: seriesForm.teacher_id,
      room_id: seriesValidatedForm.room_id,
      weekdays: seriesValidatedForm.weekdays,
      start_local_time: seriesForm.start_local_time,
      duration_minutes: seriesForm.duration_minutes,
      start_date: seriesForm.start_date,
      end_date: seriesValidatedForm.end_date,
      count: seriesValidatedForm.count,
      start_at: "",
      end_at: "",
    };
  }, [seriesOpen, seriesValidatedForm, seriesForm.course_id, seriesForm.teacher_id, seriesForm.start_local_time, seriesForm.duration_minutes, seriesForm.start_date]);
  useDebouncedPreflight(seriesPreflight, seriesPreflightParams, { enabled: seriesOpen });

  // --- Edit Series (This & Future) preflight ---
  const editSeriesPreflightParams = useMemo<PreflightParams | null>(() => {
    if (!editSeriesOpen || !editSeriesForm || editSeriesLoading || !editSeriesValidatedForm) return null;
    return {
      series_id: editSeriesForm.series_id,
      course_id: editSeriesForm.course_id,
      teacher_id: editSeriesForm.teacher_id,
      room_id: editSeriesValidatedForm.room_id,
      weekdays: editSeriesValidatedForm.weekdays,
      start_local_time: editSeriesForm.start_local_time,
      duration_minutes: editSeriesForm.duration_minutes,
      start_date: editSeriesPivotDate,
      end_date: editSeriesValidatedForm.end_date,
      count: editSeriesValidatedForm.count,
      start_at: "",
      end_at: "",
    };
  }, [editSeriesOpen, editSeriesForm, editSeriesLoading, editSeriesValidatedForm, editSeriesPivotDate]);
  useDebouncedPreflight(editSeriesPreflight, editSeriesPreflightParams, { enabled: editSeriesOpen });

  // --- Edit Series (Entire) preflight ---
  const editSeriesEntirePreflightParams = useMemo<PreflightParams | null>(() => {
    if (!editSeriesEntireOpen || !editSeriesEntireForm || editSeriesEntireLoading || !editSeriesEntireValidatedForm) return null;
    return {
      series_id: editSeriesEntireForm.series_id,
      course_id: editSeriesEntireForm.course_id,
      teacher_id: editSeriesEntireForm.teacher_id,
      room_id: editSeriesEntireValidatedForm.room_id,
      weekdays: editSeriesEntireValidatedForm.weekdays,
      start_local_time: editSeriesEntireForm.start_local_time,
      duration_minutes: editSeriesEntireForm.duration_minutes,
      start_date: editSeriesEntireFromDate,
      end_date: editSeriesEntireValidatedForm.end_date,
      count: editSeriesEntireValidatedForm.count,
      start_at: "",
      end_at: "",
    };
  }, [editSeriesEntireOpen, editSeriesEntireForm, editSeriesEntireLoading, editSeriesEntireValidatedForm, editSeriesEntireFromDate]);
  useDebouncedPreflight(editSeriesEntirePreflight, editSeriesEntirePreflightParams, { enabled: editSeriesEntireOpen });

  // --- Create Series submit ---
  const submitSeries = async () => {
    if (!seriesValidation.validateAll()) return;
    if (!seriesGate.canSave) {
      addToast("error", seriesGate.isChecking ? "Checking availability…" : "Preflight must pass before saving");
      return;
    }
    try {
      setSeriesCreating(true);
      await apiJson("/api/v1/series", {
        method: "POST",
        body: JSON.stringify({
          course_id: seriesForm.course_id,
          room_id: seriesValidatedForm?.room_id ?? (seriesForm.room_id ? seriesForm.room_id : null),
          teacher_id: seriesForm.teacher_id,
          weekdays: seriesValidatedForm?.weekdays ?? seriesForm.weekdays.map((v, idx) => (v ? idx : null)).filter((v): v is number => v != null),
          start_local_time: seriesForm.start_local_time,
          duration_minutes: seriesForm.duration_minutes,
          start_date: seriesForm.start_date,
          end_date: seriesValidatedForm?.end_date ?? (seriesUseCount ? null : seriesForm.end_date),
          count: seriesValidatedForm?.count ?? (seriesUseCount ? seriesForm.count : null),
        }),
      });
      addToast("success", "Series created");
      setSeriesOpen(false);
      await load();
    } catch (err) {
      if (err instanceof ApiRequestError && err.code) { addToast("error", `${err.code}: ${err.message}`); }
      else { addToast("error", err instanceof Error ? err.message : "Create failed"); }
    } finally { setSeriesCreating(false); }
  };

  // --- Edit Series (This & Future) submit ---
  const submitEditSeriesThisAndFuture = async () => {
    if (!editSeriesForm) return;
    if (!editSeriesGate.canSave) {
      addToast("error", editSeriesGate.isChecking ? "Checking availability…" : "Preflight must pass before saving");
      return;
    }
    try {
      await apiJson(`/api/v1/series/${encodeURIComponent(editSeriesForm.series_id)}`, {
        method: "PATCH",
        body: JSON.stringify({
          pivot_date: editSeriesPivotDate,
          course_id: editSeriesForm.course_id,
          room_id: editSeriesValidatedForm?.room_id ?? editSeriesForm.room_id,
          teacher_id: editSeriesForm.teacher_id,
          weekdays: editSeriesValidatedForm?.weekdays ?? editSeriesForm.weekdays.map((v, idx) => (v ? idx : null)).filter((v): v is number => v != null),
          start_local_time: editSeriesForm.start_local_time,
          duration_minutes: editSeriesForm.duration_minutes,
          end_date: editSeriesValidatedForm?.end_date ?? (editSeriesUseCount ? null : editSeriesForm.end_date),
          count: editSeriesValidatedForm?.count ?? (editSeriesUseCount ? editSeriesForm.count : null), expected_version: editSeriesForm.expected_version,
        }),
      });
      addToast("success", "Series updated (this & future)");
      setEditSeriesOpen(false);
      setEditSeriesForm(null);
      await load();
    } catch (err) {
      if (err instanceof ApiRequestError && err.code) {
        if (err.code === "stale_edit" && err.details) {
          const stale = err.details as StaleEditDetails;
          if (stale.current && editSeriesForm) {
            const weekdayFlags = [false, false, false, false, false, false, false];
            for (const wd of stale.current.weekdays ?? []) { if (wd >= 0 && wd <= 6) weekdayFlags[wd] = true; }
            const useCount = stale.current.count != null;
            setEditSeriesUseCount(useCount);
            setEditSeriesForm({
              series_id: stale.current.id, expected_version: stale.current.version,
              course_id: stale.current.course_id, room_id: stale.current.room_id, teacher_id: stale.current.teacher_id,
              weekdays: weekdayFlags, start_local_time: stale.current.start_local_time ?? editSeriesForm.start_local_time,
              duration_minutes: stale.current.duration_minutes, end_date: stale.current.end_date || "",
              count: (stale.current.count ?? editSeriesForm.count) as number,
            });
            addToast("error", "Stale edit: reloaded latest series. Please review and save again.");
            return;
          }
        }
        addToast("error", `${err.code}: ${err.message}`);
      } else { addToast("error", err instanceof Error ? err.message : "Update failed"); }
    }
  };

  // --- Edit Series (Entire) submit ---
  const submitEditSeriesEntire = async () => {
    if (!editSeriesEntireForm) return;
    if (!editSeriesEntireGate.canSave) {
      addToast("error", editSeriesEntireGate.isChecking ? "Checking availability…" : "Preflight must pass before saving");
      return;
    }
    try {
      await apiJson(`/api/v1/series/${encodeURIComponent(editSeriesEntireForm.series_id)}/entire`, {
        method: "PATCH",
        body: JSON.stringify({
          expected_version: editSeriesEntireForm.expected_version,
          course_id: editSeriesEntireForm.course_id,
          room_id: editSeriesEntireValidatedForm?.room_id ?? editSeriesEntireForm.room_id,
          teacher_id: editSeriesEntireForm.teacher_id,
          weekdays: editSeriesEntireValidatedForm?.weekdays ?? editSeriesEntireForm.weekdays.map((v, idx) => (v ? idx : null)).filter((v): v is number => v != null),
          start_local_time: editSeriesEntireForm.start_local_time,
          duration_minutes: editSeriesEntireForm.duration_minutes,
          end_date: editSeriesEntireValidatedForm?.end_date ?? (editSeriesEntireUseCount ? null : editSeriesEntireForm.end_date), count: editSeriesEntireValidatedForm?.count ?? (editSeriesEntireUseCount ? editSeriesEntireForm.count : null),
        }),
      });
      addToast("success", "Series updated (future only)");
      setEditSeriesEntireOpen(false);
      setEditSeriesEntireForm(null);
      await load();
    } catch (err) {
      if (err instanceof ApiRequestError && err.code) {
        if (err.code === "stale_edit") {
          const details = err.details as StaleEditDetails;
          const cur = details?.current;
          if (cur) {
            addToast("error", "Stale edit: reloaded latest series. Please review and save again.");
            const weekdayFlags = [false, false, false, false, false, false, false];
            for (const wd of cur.weekdays ?? []) { if (wd >= 0 && wd <= 6) weekdayFlags[wd] = true; }
            const useCount = cur.count != null;
            setEditSeriesEntireUseCount(useCount);
            setEditSeriesEntireForm({
              series_id: cur.id, expected_version: cur.version, course_id: cur.course_id,
              room_id: cur.room_id, teacher_id: cur.teacher_id, weekdays: weekdayFlags,
              start_local_time: cur.start_local_time ?? "16:00", duration_minutes: cur.duration_minutes,
              end_date: cur.end_date || "", count: (cur.count ?? 10) as number,
            });
            return;
          }
        }
        addToast("error", `${err.code}: ${err.message}`);
      } else { addToast("error", err instanceof Error ? err.message : "Update failed"); }
    }
  };

  return (
    <div>
      <div className="flex items-baseline justify-between gap-3">
        <PageHeading>Schedule</PageHeading>
        <div className="text-xs text-[var(--color-wi-text-light)] mb-3 text-right">
          {instituteTZ ? `TZ: ${instituteTZ}` : ""}
          {serverNow ? `${instituteTZ ? " • " : ""}Server now: ${serverNow}` : ""}
        </div>
      </div>

      <ScheduleFilters
        startDate={startDate}
        endDate={endDate}
        startTime={startTime}
        endTime={endTime}
        viewMode={viewMode}
        onChangeStartDate={setStartDate}
        onChangeEndDate={setEndDate}
        onChangeStartTime={setStartTime}
        onChangeEndTime={setEndTime}
        onRefresh={load}
        onViewModeChange={setViewMode}
        onOpenCreate={() => create.openModal({ course_id: courses[0]?.id, teacher_id: teachers[0]?.id })}
        onOpenSeries={openSeries}
      />

      {viewMode === "week" ? (
        <div className="border border-wi-line rounded-sm overflow-hidden">
          <div className="grid" style={{ gridTemplateColumns: `repeat(${Math.max(daysInRange.length, 1)}, minmax(0, 1fr))` }}>
            {daysInRange.map((d) => {
              const key = d.toISOString().slice(0, 10);
              const items = sessionsByDay.get(key) ?? [];
              const dateLabel = formatZoneDateKey(key, zone, "EEE d MMM") ?? key;
              return (
                <div key={key} className="border-r border-wi-line last:border-r-0">
                  <div className="bg-[var(--color-wi-row-alt)] border-b border-wi-line px-3 py-2">
                    <div className="text-sm font-semibold text-[var(--color-wi-text)]">{dateLabel}</div>
                    <div className="text-xs text-[var(--color-wi-text-light)]">{items.length} session(s)</div>
                  </div>
                  <div className="p-2 space-y-2">
                    {items.length === 0 ? (
                      <div className="text-xs text-[var(--color-wi-text-light)] px-1 py-3">No sessions</div>
                    ) : (
                      items.map((s) => (
                        <SessionWeekCard
                          key={s.id}
                          session={s}
                          courseById={courseById}
                          roomById={roomById}
                          teacherById={teacherById}
                          courseOptions={courseOptions}
                          teacherOptions={teacherOptions}
                          rooms={rooms}
                          zone={zone}
                          impacted={impactedSessionIDs.has(s.id)}
                          cancelingId={cancelingId}
                          inlineEdit={inlineEditSession?.id === s.id ? activeInlineEdit : null}
                          onAttendance={handleAttendance}
                          onEdit={edit.openModal}
                          onCancel={cancelOccurrence}
                          onEditSeriesTandF={handleEditSeriesTandF}
                          onEditSeriesEntire={handleEditSeriesEntire}
                          onCancelSeries={handleCancelSeries}
                          onOpenInlineEdit={openInlineEdit}
                        />
                      ))
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      ) : (
        <div className="overflow-x-auto"><table className="w-full text-[13px]">
          <caption className="sr-only">Schedule</caption>
          <thead>
            <tr className="border-b border-wi-line">
              <th scope="col" className="text-left py-2 px-2 font-semibold">Start</th>
              <th scope="col" className="text-left py-2 px-2 font-semibold">End</th>
              <th scope="col" className="text-left py-2 px-2 font-semibold">Course</th>
              <th scope="col" className="text-left py-2 px-2 font-semibold">Room</th>
              <th scope="col" className="text-left py-2 px-2 font-semibold">Teacher</th>
              <th scope="col" className="text-left py-2 px-2 font-semibold"></th>
            </tr>
          </thead>
          <tbody>
            {sessions.map((s) => (
              <SessionTableRow
                key={s.id}
                session={s}
                courseById={courseById}
                roomById={roomById}
                teacherById={teacherById}
                courseOptions={courseOptions}
                teacherOptions={teacherOptions}
                rooms={rooms}
                zone={zone}
                impacted={impactedSessionIDs.has(s.id)}
                cancelingId={cancelingId}
                inlineEdit={inlineEditSession?.id === s.id ? activeInlineEdit : null}
                onAttendance={handleAttendance}
                onEdit={edit.openModal}
                onCancel={cancelOccurrence}
                onEditSeriesTandF={handleEditSeriesTandF}
                onEditSeriesEntire={handleEditSeriesEntire}
                onCancelSeries={handleCancelSeries}
                onOpenInlineEdit={openInlineEdit}
              />
            ))}
          </tbody>
        </table></div>
      )}

      {loading && <LoadingSkeleton type="table" lines={3} />}
      {!loading && sessions.length === 0 && <EmptyState message="No sessions for this date range. Use the toolbar above to create a session or series." />}

      {create.open && (
        <Modal
          title="Create Session"
          onClose={create.closeModal}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={create.closeModal}>Cancel</Button>
              <Button
                variant="primary" size="sm"
                onClick={create.submit}
                disabled={create.creating || !create.gate.canSave}
                loading={create.preflight.loading || create.creating}
              >
                {create.creating ? "Creating…" : getSaveButtonLabel({ status: create.preflight.status, loading: create.preflight.loading }, "Create", create.preflight.details)}
              </Button>
            </>
          }
        >
          <div className="space-y-6">
            <SessionOccurrenceForm
              form={create.form}
              setForm={create.setForm}
              courseOptions={courseOptions}
              teacherOptions={teacherOptions}
              rooms={rooms}
              prefix="create-"
            />
            <PreflightIndicator preflight={create.preflight} coursesById={courseById} teachersById={teacherById} roomsById={roomById}
              requiredFields={[
                { label: "Course", value: create.form.course_id },
                { label: "Teacher", value: create.form.teacher_id },
                { label: "Start", value: create.form.start_local },
                { label: "End", value: create.form.end_local },
              ]}
            />
          </div>
        </Modal>
      )}

      {edit.open && edit.session && (
        <Modal
          title="Edit Session"
          onClose={edit.closeModal}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={edit.closeModal}>Cancel</Button>
              <Button
                variant="primary" size="sm"
                onClick={() => void edit.submit()}
                disabled={edit.saving || !edit.gate.canSave}
                loading={edit.preflight.loading || edit.saving}
              >
                {edit.saving ? "Saving…" : getSaveButtonLabel({ status: edit.preflight.status, loading: edit.preflight.loading }, "Save", edit.preflight.details)}
              </Button>
            </>
          }
        >
          <div className="space-y-6">
            <SessionOccurrenceForm
              form={edit.form}
              setForm={edit.setForm}
              courseOptions={courseOptions}
              teacherOptions={teacherOptions}
              rooms={rooms}
              prefix="edit-"
            />
            <PreflightIndicator preflight={edit.preflight} coursesById={courseById} teachersById={teacherById} roomsById={roomById}
              requiredFields={[
                { label: "Course", value: edit.form.course_id },
                { label: "Teacher", value: edit.form.teacher_id },
                { label: "Start", value: edit.form.start_local },
                { label: "End", value: edit.form.end_local },
              ]}
            />
          </div>
        </Modal>
      )}

      {edit.pendingImpact ? <ImpactAcknowledgementModal summary={edit.pendingImpact} saving={edit.saving} onBack={edit.dismissImpact} onConfirm={() => void edit.confirmImpact()} /> : null}
      {inlineImpact ? <ImpactAcknowledgementModal summary={inlineImpact} saving={inlineSaving} onBack={() => setInlineImpact(null)} onConfirm={() => { setInlineImpact(null); void submitInlineEdit(true); }} /> : null}

      {seriesOpen && (
        <Modal
          title="Create Series"
          onClose={() => setSeriesOpen(false)}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setSeriesOpen(false)}>Cancel</Button>
              <Button variant="primary" size="sm" onClick={submitSeries} disabled={seriesCreating || !seriesGate.canSave} loading={seriesPreflight.loading || seriesCreating}>
                {seriesCreating ? "Creating…" : getSaveButtonLabel({ status: seriesPreflight.status, loading: seriesPreflight.loading }, "Create", seriesPreflight.details)}
              </Button>
            </>
          }
        >
          <div className="space-y-6">
            <FormErrorSummary errors={seriesValidation.errors} touched={seriesValidation.touched} />
            <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
              <FormField name="series-course_id" label="Course" error={seriesValidation.errors.course_id} touched={seriesValidation.touched.course_id} required>
                <TypeaheadSelect value={seriesForm.course_id} onChange={(v) => setSeriesForm(prev => ({ ...prev, course_id: v }))} options={courseOptions} placeholder="Search course…" />
              </FormField>
              <FormField name="series-room_id" label="Room">
                <Select size="sm" value={seriesForm.room_id} onChange={(e) => setSeriesForm(prev => ({ ...prev, room_id: e.target.value }))}>
                  <option value="">[NOT SET] (Provisional)</option>
                  {rooms.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
                </Select>
              </FormField>
              <FormField name="series-teacher_id" label="Teacher" error={seriesValidation.errors.teacher_id} touched={seriesValidation.touched.teacher_id} required>
                <TypeaheadSelect value={seriesForm.teacher_id} onChange={(v) => setSeriesForm(prev => ({ ...prev, teacher_id: v }))} options={teacherOptions} placeholder="Search teacher…" />
              </FormField>
            </div>
            <div className="flex items-center gap-2 text-xs text-[var(--color-wi-text-light)] mb-1">
              <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-[var(--color-wi-nav)] text-white text-[10px] font-semibold">1</span>
              <span>Course & Teacher</span>
              <span className="text-[var(--color-wi-text-light)] mx-1">→</span>
              <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)] text-[10px] font-semibold">2</span>
              <span>When & How Often</span>
            </div>
            <SeriesFormFields
              weekdays={seriesForm.weekdays}
              onWeekdayChange={(idx) => { setSeriesForm(prev => { const next = prev.weekdays.slice(); next[idx] = !next[idx]; return { ...prev, weekdays: next }; }); }}
              startLocalTime={seriesForm.start_local_time}
              onStartLocalTimeChange={(v) => setSeriesForm(prev => ({ ...prev, start_local_time: v }))}
              durationMinutes={seriesForm.duration_minutes}
              onDurationMinutesChange={(v) => setSeriesForm(prev => ({ ...prev, duration_minutes: v }))}
              useCount={seriesUseCount}
              onUseCountChange={setSeriesUseCount}
              count={seriesForm.count}
              onCountChange={(v) => setSeriesForm(prev => ({ ...prev, count: v }))}
              endDate={seriesForm.end_date}
              onEndDateChange={(v) => setSeriesForm(prev => ({ ...prev, end_date: v }))}
              startDate={seriesForm.start_date}
              onStartDateChange={(v) => setSeriesForm(prev => ({ ...prev, start_date: v }))}
              errors={seriesValidation.errors}
              touched={seriesValidation.touched}
              prefix="series-"
            />
            <PreflightIndicator preflight={seriesPreflight} coursesById={courseById} teachersById={teacherById} roomsById={roomById}
              requiredFields={[
                { label: "Course", value: seriesForm.course_id },
                { label: "Teacher", value: seriesForm.teacher_id },
                { label: "Weekdays", value: seriesForm.weekdays.some(Boolean) ? "selected" : "" },
                { label: "Start time", value: seriesForm.start_local_time },
                { label: "Duration", value: seriesForm.duration_minutes > 0 ? String(seriesForm.duration_minutes) : "" },
                { label: "Start date", value: seriesForm.start_date },
                { label: seriesUseCount ? "Count" : "End date", value: seriesUseCount ? (Number.isFinite(seriesForm.count) && seriesForm.count > 0 ? String(seriesForm.count) : "") : seriesForm.end_date },
              ]}
            />
          </div>
        </Modal>
      )}

      {editSeriesOpen && (
        <Modal
          title="Edit Series (This & Future)"
          onClose={() => { setEditSeriesOpen(false); setEditSeriesForm(null); }}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => { setEditSeriesOpen(false); setEditSeriesForm(null); }}>Cancel</Button>
              <Button variant="primary" size="sm" onClick={submitEditSeriesThisAndFuture} disabled={editSeriesLoading || !editSeriesGate.canSave} loading={editSeriesPreflight.loading}>
                {getSaveButtonLabel({ status: editSeriesPreflight.status, loading: editSeriesPreflight.loading }, "Save", editSeriesPreflight.details)}
              </Button>
            </>
          }
        >
          {editSeriesLoading || !editSeriesForm ? (
            <div className="py-6 text-center text-sm text-[var(--color-wi-text-light)]">
              <span className="inline-block w-4 h-4 border-2 border-wi-line border-t-transparent rounded-full animate-spin mr-2" aria-hidden="true" />
              Loading…
            </div>
          ) : (
            <div className="space-y-6">
              <div className="rounded-sm border border-wi-line bg-[var(--color-wi-row-alt)] px-3 py-2 text-sm">
                <div className="font-medium text-[var(--color-wi-text)]">Scope</div>
                <div className="text-xs text-[var(--color-wi-text-light)]">Pivot date (Bangkok): <span className="font-mono">{editSeriesPivotDate}</span> (includes the selected occurrence)</div>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                <FormField name="es-course" label="Course (fixed)">
                  <div className="px-2 py-1.5 text-sm border border-wi-line rounded-sm bg-[var(--color-wi-row-alt)]">{courseById.get(editSeriesForm.course_id)?.code ?? editSeriesForm.course_id}</div>
                </FormField>
                <FormField name="es-room" label="Room (fixed)">
                  <div className="px-2 py-1.5 text-sm border border-wi-line rounded-sm bg-[var(--color-wi-row-alt)]">{editSeriesForm.room_id ? (roomById.get(editSeriesForm.room_id)?.name ?? editSeriesForm.room_id) : "[NOT SET]"}</div>
                </FormField>
                <FormField name="es-teacher" label="Teacher (fixed)">
                  <div className="px-2 py-1.5 text-sm border border-wi-line rounded-sm bg-[var(--color-wi-row-alt)]">{teacherById.get(editSeriesForm.teacher_id)?.full_name || teacherById.get(editSeriesForm.teacher_id)?.username || editSeriesForm.teacher_id}</div>
                </FormField>
              </div>
              <SeriesFormFields
                weekdays={editSeriesForm.weekdays}
                onWeekdayChange={(idx) => {
                  setEditSeriesForm((prev) => {
                    if (!prev) return prev;
                    const next = prev.weekdays.slice();
                    next[idx] = !next[idx];
                    return { ...prev, weekdays: next };
                  });
                }}
                startLocalTime={editSeriesForm.start_local_time}
                onStartLocalTimeChange={(v) => {
                  setEditSeriesForm((prev) => (prev ? { ...prev, start_local_time: v } : prev));
                }}
                durationMinutes={editSeriesForm.duration_minutes}
                onDurationMinutesChange={(v) => {
                  setEditSeriesForm((prev) => (prev ? { ...prev, duration_minutes: v } : prev));
                }}
                useCount={editSeriesUseCount}
                onUseCountChange={setEditSeriesUseCount}
                count={editSeriesForm.count}
                onCountChange={(v) => {
                  setEditSeriesForm((prev) => (prev ? { ...prev, count: v } : prev));
                }}
                endDate={editSeriesForm.end_date}
                onEndDateChange={(v) => {
                  setEditSeriesForm((prev) => (prev ? { ...prev, end_date: v } : prev));
                }}
                prefix="es-"
              />
              <PreflightIndicator preflight={editSeriesPreflight} coursesById={courseById} teachersById={teacherById} roomsById={roomById}
                requiredFields={[
                  { label: "Weekdays", value: editSeriesForm.weekdays.some(Boolean) ? "selected" : "" },
                  { label: "Start time", value: editSeriesForm.start_local_time },
                  { label: "Duration", value: editSeriesForm.duration_minutes > 0 ? String(editSeriesForm.duration_minutes) : "" },
                  { label: editSeriesUseCount ? "Count" : "End date", value: editSeriesUseCount ? (Number.isFinite(editSeriesForm.count) && editSeriesForm.count > 0 ? String(editSeriesForm.count) : "") : editSeriesForm.end_date },
                ]}
              />
            </div>
          )}
        </Modal>
      )}

      {editSeriesEntireOpen && (
        <Modal
          title="Edit Series (Future Only)"
          onClose={() => { setEditSeriesEntireOpen(false); setEditSeriesEntireForm(null); }}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => { setEditSeriesEntireOpen(false); setEditSeriesEntireForm(null); }}>Cancel</Button>
              <Button variant="primary" size="sm" onClick={submitEditSeriesEntire} disabled={editSeriesEntireLoading || !editSeriesEntireGate.canSave} loading={editSeriesEntirePreflight.loading}>
                {getSaveButtonLabel({ status: editSeriesEntirePreflight.status, loading: editSeriesEntirePreflight.loading }, "Save", editSeriesEntirePreflight.details)}
              </Button>
            </>
          }
        >
          {editSeriesEntireLoading || !editSeriesEntireForm ? (
            <div className="py-6 text-center text-sm text-[var(--color-wi-text-light)]">
              <span className="inline-block w-4 h-4 border-2 border-wi-line border-t-transparent rounded-full animate-spin mr-2" aria-hidden="true" />
              Loading…
            </div>
          ) : (
            <div className="space-y-6">
              <div className="rounded-sm border border-wi-line bg-[var(--color-wi-row-alt)] px-3 py-2 text-sm">
                <div className="font-medium text-[var(--color-wi-text)]">Scope</div>
                <div className="text-xs text-[var(--color-wi-text-light)]">Applies to future occurrences from (Bangkok): <span className="font-mono">{editSeriesEntireFromDate}</span></div>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                <FormField name="ese-course_id" label="Course">
                  <TypeaheadSelect
                    value={editSeriesEntireForm.course_id}
                    onChange={(v) => {
                      setEditSeriesEntireForm((prev) => (prev ? { ...prev, course_id: v } : prev));
                    }}
                    options={courseOptions}
                    placeholder="Search course…"
                  />
                </FormField>
                <FormField name="ese-room_id" label="Room">
                  <Select
                    size="sm"
                    value={editSeriesEntireForm.room_id ?? ""}
                    onChange={(e) => {
                      setEditSeriesEntireForm((prev) =>
                        prev ? { ...prev, room_id: e.target.value ? e.target.value : null } : prev,
                      );
                    }}
                  >
                    <option value="">[NOT SET] (Provisional)</option>
                    {rooms.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
                  </Select>
                </FormField>
                <FormField name="ese-teacher_id" label="Teacher">
                  <TypeaheadSelect
                    value={editSeriesEntireForm.teacher_id}
                    onChange={(v) => {
                      setEditSeriesEntireForm((prev) => (prev ? { ...prev, teacher_id: v } : prev));
                    }}
                    options={teacherOptions}
                    placeholder="Search teacher…"
                  />
                </FormField>
              </div>
              <SeriesFormFields
                weekdays={editSeriesEntireForm.weekdays}
                onWeekdayChange={(idx) => {
                  setEditSeriesEntireForm((prev) => {
                    if (!prev) return prev;
                    const next = prev.weekdays.slice();
                    next[idx] = !next[idx];
                    return { ...prev, weekdays: next };
                  });
                }}
                startLocalTime={editSeriesEntireForm.start_local_time}
                onStartLocalTimeChange={(v) => {
                  setEditSeriesEntireForm((prev) => (prev ? { ...prev, start_local_time: v } : prev));
                }}
                durationMinutes={editSeriesEntireForm.duration_minutes}
                onDurationMinutesChange={(v) => {
                  setEditSeriesEntireForm((prev) => (prev ? { ...prev, duration_minutes: v } : prev));
                }}
                useCount={editSeriesEntireUseCount}
                onUseCountChange={setEditSeriesEntireUseCount}
                count={editSeriesEntireForm.count}
                onCountChange={(v) => {
                  setEditSeriesEntireForm((prev) => (prev ? { ...prev, count: v } : prev));
                }}
                endDate={editSeriesEntireForm.end_date}
                onEndDateChange={(v) => {
                  setEditSeriesEntireForm((prev) => (prev ? { ...prev, end_date: v } : prev));
                }}
                prefix="ese-"
              />
              <PreflightIndicator preflight={editSeriesEntirePreflight} coursesById={courseById} teachersById={teacherById} roomsById={roomById}
                requiredFields={[
                  { label: "Course", value: editSeriesEntireForm.course_id },
                  { label: "Teacher", value: editSeriesEntireForm.teacher_id },
                  { label: "Weekdays", value: editSeriesEntireForm.weekdays.some(Boolean) ? "selected" : "" },
                  { label: "Start time", value: editSeriesEntireForm.start_local_time },
                  { label: "Duration", value: editSeriesEntireForm.duration_minutes > 0 ? String(editSeriesEntireForm.duration_minutes) : "" },
                  { label: editSeriesEntireUseCount ? "Count" : "End date", value: editSeriesEntireUseCount ? (Number.isFinite(editSeriesEntireForm.count) && editSeriesEntireForm.count > 0 ? String(editSeriesEntireForm.count) : "") : editSeriesEntireForm.end_date },
                ]}
              />
            </div>
          )}
        </Modal>
      )}

      {cancelSeriesOpen && (
        <Modal
          title="Cancel Series"
          onClose={() => { setCancelSeriesOpen(false); setCancelSeriesForm(null); }}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => { setCancelSeriesOpen(false); setCancelSeriesForm(null); }}>Close</Button>
              <Button variant="danger" size="sm" onClick={submitCancelSeries} disabled={cancelSeriesLoading || !cancelSeriesForm} loading={cancelSeriesLoading}>
                {cancelSeriesLoading ? "Canceling…" : "Cancel series"}
              </Button>
            </>
          }
        >
          {cancelSeriesLoading && !cancelSeriesForm ? (
            <div className="py-6 text-center text-sm text-[var(--color-wi-text-light)]">
              <span className="inline-block w-4 h-4 border-2 border-wi-line border-t-transparent rounded-full animate-spin mr-2" aria-hidden="true" />
              Loading…
            </div>
          ) : (
            <div className="space-y-6">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">Scope</label>
                  <Select size="sm" value={cancelSeriesScope} onChange={(e) => setCancelSeriesScope(e.target.value as any)}>
                    <option value="this_and_future">This & future (includes selected occurrence)</option>
                    <option value="entire_series_future_only">Entire series (future only)</option>
                  </Select>
                </div>
                <div>
                  <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">Pivot date (Bangkok)</label>
                  <input
                    type="date"
                    value={cancelSeriesPivotDate}
                    onChange={(e) => setCancelSeriesPivotDate(e.target.value)}
                    disabled={cancelSeriesScope !== "this_and_future"}
                    className="w-full px-2 py-1.5 text-sm border border-wi-line rounded-sm disabled:bg-[var(--color-wi-row-alt)]"
                  />
                </div>
              </div>
              <div className="text-xs text-[var(--color-wi-text-light)]">Uses optimistic concurrency; if someone changed the series, you'll get a stale edit and can retry.</div>
            </div>
          )}
        </Modal>
      )}

      <ConfirmModal
        open={!!confirmCancelOccurrence.session}
        title="Cancel Session"
        message="Cancel this session occurrence?"
        variant="danger"
        confirmLabel="Cancel Session"
        onConfirm={handleConfirmCancelOccurrence}
        onCancel={() => setConfirmCancelOccurrence({ session: null })}
      />

      <ConfirmModal
        open={confirmCancelSeriesModal}
        title="Cancel Series"
        message={cancelSeriesScope === "this_and_future" ? "This will cancel this session and all future sessions in this series. Past sessions will remain for audit history. Continue?" : "This will cancel all future sessions in this series. Past sessions will remain for audit history. Continue?"}
        variant="danger"
        confirmLabel="Cancel Series"
        onConfirm={handleConfirmCancelSeries}
        onCancel={() => setConfirmCancelSeriesModal(false)}
      />

      {attendance.session && (
        <Modal
          title="Attendance (include/exclude)"
          onClose={attendance.closeAttendance}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={attendance.closeAttendance}>Close</Button>
            </>
          }
        >
          <AttendancePanel
            roster={attendance.roster}
            overrides={attendance.overrides}
            loading={attendance.loading}
            includeWcode={attendance.includeWcode}
            onIncludeWcodeChange={attendance.setIncludeWcode}
            includeAdding={attendance.includeAdding}
            onAddIncluded={attendance.addIncludedByWcode}
            onUpsert={attendance.upsertAttendance}
            onDelete={attendance.deleteAttendance}
            addToast={addToast}
          />
        </Modal>
      )}
    </div>
  );
}
