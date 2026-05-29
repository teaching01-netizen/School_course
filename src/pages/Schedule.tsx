import { useEffect, useMemo, useState } from "react";
import { ApiRequestError, apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import { utcISOToZoneDate, zoneLocalInputToUTCISO } from "../utils/timezone";
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
import SessionActions from "../components/SessionActions";
import { PreflightIndicator, getSaveButtonLabel, isSaveDisabled } from "../components/PreflightIndicator";
import SessionOccurrenceForm from "../components/SessionOccurrenceForm";
import SeriesFormFields from "../components/SeriesFormFields";
import AttendancePanel from "../components/AttendancePanel";
import useInstituteMeta from "../hooks/useInstituteMeta";
import useLookups from "../hooks/useLookups";
import { useCreateSession } from "../hooks/useCreateSession";
import { useEditSession } from "../hooks/useEditSession";
import { useAttendanceModal } from "../hooks/useAttendanceModal";
import { usePreflight } from "../hooks/usePreflight";
import { validateSeriesPreflight, type SeriesPreflightForm } from "../utils/preflight";
import { useFormValidation } from "../hooks/useFormValidation";
import TypeaheadSelect from "../components/TypeaheadSelect";
import {
  yyyyMmDd,
  type Session,
  type StaleEditDetails,
} from "@/types";

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

  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(false);
  const [viewMode, setViewMode] = useState<"week" | "table">("week");
  const [cancelingId, setCancelingId] = useState<string | null>(null);

  const load = async () => {
    try {
      setLoading(true);
      const start = zoneLocalInputToUTCISO(`${startDate}T${startTime}`, zone);
      const end = zoneLocalInputToUTCISO(`${endDate}T${endTime}`, zone);
      if (!start || !end) {
        addToast("error", "Invalid date/time range");
        return;
      }
      const items = await apiJson<Session[]>(`/api/v1/sessions?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`, {
        method: "GET",
      });
      setSessions(items);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load sessions");
    } finally {
      setLoading(false);
    }
  };

  // --- Create Session hook ---
  const create = useCreateSession(load, addToast, zone);
  // --- Edit Session hook ---
  const edit = useEditSession(load, addToast, zone);
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
      const key = s.start_at.slice(0, 10);
      const arr = map.get(key) ?? [];
      arr.push(s);
      map.set(key, arr);
    }
    for (const arr of map.values()) arr.sort((a, b) => a.start_at.localeCompare(b.start_at));
    return map;
  }, [sessions]);

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // --- Cancel occurrence ---
  const cancelOccurrence = (sess: Session) => {
    setConfirmCancelOccurrence({ session: sess });
  };

  const handleConfirmCancelOccurrence = async () => {
    const sess = confirmCancelOccurrence.session;
    if (!sess) return;
    setConfirmCancelOccurrence({ session: null });
    setCancelingId(sess.id);
    try {
      await apiJson<{ ok: boolean }>(`/api/v1/sessions/${sess.id}`, {
        method: "DELETE",
        body: JSON.stringify({ expected_version: sess.version }),
      });
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
  const openEditSeriesThisAndFuture = async (sess: Session) => {
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
  };

  // --- Open Edit Series (Entire) ---
  const openEditSeriesEntire = async (sess: Session) => {
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
  };

  // --- Cancel Series ---
  const openCancelSeries = async (sess: Session) => {
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
  };

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

  // --- Series preflight ---
  const runSeriesPreflight = async () => {
    if (!seriesOpen) return;
    const validated = validateSeriesPreflight(seriesForm as SeriesPreflightForm, seriesUseCount);
    if (!validated) { seriesPreflight.reset(); return; }
    await seriesPreflight.check({
      course_id: seriesForm.course_id,
      teacher_id: seriesForm.teacher_id,
      room_id: validated.room_id,
      weekdays: validated.weekdays,
      start_local_time: seriesForm.start_local_time,
      duration_minutes: seriesForm.duration_minutes,
      start_date: seriesForm.start_date,
      end_date: validated.end_date,
      count: validated.count,
      start_at: "",
      end_at: "",
    });
  };

  useEffect(() => { void runSeriesPreflight(); }, [
    seriesOpen, seriesUseCount, seriesForm.course_id, seriesForm.room_id, seriesForm.teacher_id,
    seriesForm.start_local_time, seriesForm.duration_minutes, seriesForm.start_date, seriesForm.end_date,
    seriesForm.count, ...seriesForm.weekdays,
  ]);

  // --- Edit Series (This & Future) preflight ---
  const runEditSeriesPreflight = async () => {
    if (!editSeriesOpen || !editSeriesForm || editSeriesLoading) { editSeriesPreflight.reset(); return; }
    const weekdays = editSeriesForm.weekdays.map((v, idx) => (v ? idx : null)).filter((v): v is number => v != null);
    if (weekdays.length === 0 || !editSeriesForm.start_local_time || !editSeriesPivotDate) { editSeriesPreflight.reset(); return; }
    if (editSeriesForm.duration_minutes <= 0) { editSeriesPreflight.reset(); return; }
    if (editSeriesUseCount) { if (!Number.isFinite(editSeriesForm.count) || editSeriesForm.count <= 0) { editSeriesPreflight.reset(); return; } }
    else { if (!editSeriesForm.end_date) { editSeriesPreflight.reset(); return; } }
    await editSeriesPreflight.check({
      course_id: editSeriesForm.course_id, teacher_id: editSeriesForm.teacher_id, room_id: editSeriesForm.room_id,
      weekdays, start_local_time: editSeriesForm.start_local_time, duration_minutes: editSeriesForm.duration_minutes,
      start_date: editSeriesPivotDate, end_date: editSeriesUseCount ? null : editSeriesForm.end_date,
      count: editSeriesUseCount ? editSeriesForm.count : null, start_at: "", end_at: "",
    });
  };

  useEffect(() => { void runEditSeriesPreflight(); }, [
    editSeriesOpen, editSeriesLoading, editSeriesUseCount, editSeriesPivotDate,
    editSeriesForm?.course_id, editSeriesForm?.room_id, editSeriesForm?.teacher_id,
    editSeriesForm?.start_local_time, editSeriesForm?.duration_minutes, editSeriesForm?.end_date,
    editSeriesForm?.count, ...(editSeriesForm?.weekdays ?? []),
  ]);

  // --- Edit Series (Entire) preflight ---
  const runEditSeriesEntirePreflight = async () => {
    if (!editSeriesEntireOpen || !editSeriesEntireForm || editSeriesEntireLoading) { editSeriesEntirePreflight.reset(); return; }
    const weekdays = editSeriesEntireForm.weekdays.map((v, idx) => (v ? idx : null)).filter((v): v is number => v != null);
    if (weekdays.length === 0 || !editSeriesEntireForm.start_local_time || !editSeriesEntireFromDate) { editSeriesEntirePreflight.reset(); return; }
    if (editSeriesEntireForm.duration_minutes <= 0) { editSeriesEntirePreflight.reset(); return; }
    if (editSeriesEntireUseCount) { if (!Number.isFinite(editSeriesEntireForm.count) || editSeriesEntireForm.count <= 0) { editSeriesEntirePreflight.reset(); return; } }
    else { if (!editSeriesEntireForm.end_date) { editSeriesEntirePreflight.reset(); return; } }
    await editSeriesEntirePreflight.check({
      course_id: editSeriesEntireForm.course_id, teacher_id: editSeriesEntireForm.teacher_id, room_id: editSeriesEntireForm.room_id,
      weekdays, start_local_time: editSeriesEntireForm.start_local_time, duration_minutes: editSeriesEntireForm.duration_minutes,
      start_date: editSeriesEntireFromDate, end_date: editSeriesEntireUseCount ? null : editSeriesEntireForm.end_date,
      count: editSeriesEntireUseCount ? editSeriesEntireForm.count : null, start_at: "", end_at: "",
    });
  };

  useEffect(() => { void runEditSeriesEntirePreflight(); }, [
    editSeriesEntireOpen, editSeriesEntireLoading, editSeriesEntireUseCount, editSeriesEntireFromDate,
    editSeriesEntireForm?.course_id, editSeriesEntireForm?.room_id, editSeriesEntireForm?.teacher_id,
    editSeriesEntireForm?.start_local_time, editSeriesEntireForm?.duration_minutes, editSeriesEntireForm?.end_date,
    editSeriesEntireForm?.count, ...(editSeriesEntireForm?.weekdays ?? []),
  ]);

  // --- Create Series submit ---
  const submitSeries = async () => {
    if (!seriesValidation.validateAll()) return;
    const weekdays = seriesForm.weekdays.map((v, idx) => (v ? idx : null)).filter((v): v is number => v != null);
    if (!seriesForm.course_id || !seriesForm.teacher_id) { addToast("error", "Course and Teacher are required"); return; }
    if (weekdays.length === 0) { addToast("error", "Select at least one weekday"); return; }
    if (!seriesForm.start_local_time) { addToast("error", "Start time is required"); return; }
    if (!seriesForm.start_date) { addToast("error", "Start date is required"); return; }
    if (seriesForm.duration_minutes <= 0) { addToast("error", "Duration must be > 0"); return; }
    if (seriesUseCount) { if (!Number.isFinite(seriesForm.count) || seriesForm.count <= 0) { addToast("error", "Count must be > 0"); return; } }
    else { if (!seriesForm.end_date) { addToast("error", "End date is required"); return; } }
    if (seriesPreflight.loading) { addToast("error", "Checking availability…"); return; }
    if (seriesPreflight.status !== "available" && seriesPreflight.status !== "provisional") { addToast("error", "Preflight must pass before saving"); return; }
    try {
      setSeriesCreating(true);
      await apiJson("/api/v1/series", {
        method: "POST",
        body: JSON.stringify({
          course_id: seriesForm.course_id, room_id: seriesForm.room_id ? seriesForm.room_id : null,
          teacher_id: seriesForm.teacher_id, weekdays, start_local_time: seriesForm.start_local_time,
          duration_minutes: seriesForm.duration_minutes, start_date: seriesForm.start_date,
          end_date: seriesUseCount ? null : seriesForm.end_date, count: seriesUseCount ? seriesForm.count : null,
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
    const weekdays = editSeriesForm.weekdays.map((v, idx) => (v ? idx : null)).filter((v): v is number => v != null);
    if (weekdays.length === 0) { addToast("error", "Select at least one weekday"); return; }
    if (editSeriesPreflight.status !== "available" && editSeriesPreflight.status !== "provisional") { addToast("error", "Preflight must pass before saving"); return; }
    try {
      await apiJson(`/api/v1/series/${encodeURIComponent(editSeriesForm.series_id)}`, {
        method: "PATCH",
        body: JSON.stringify({
          pivot_date: editSeriesPivotDate, weekdays, start_local_time: editSeriesForm.start_local_time,
          duration_minutes: editSeriesForm.duration_minutes, end_date: editSeriesUseCount ? null : editSeriesForm.end_date,
          count: editSeriesUseCount ? editSeriesForm.count : null, expected_version: editSeriesForm.expected_version,
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
    const weekdays = editSeriesEntireForm.weekdays.map((v, idx) => (v ? idx : null)).filter((v): v is number => v != null);
    if (weekdays.length === 0) { addToast("error", "Select at least one weekday"); return; }
    if (editSeriesEntirePreflight.status !== "available" && editSeriesEntirePreflight.status !== "provisional") { addToast("error", "Preflight must pass before saving"); return; }
    try {
      await apiJson(`/api/v1/series/${encodeURIComponent(editSeriesEntireForm.series_id)}/entire`, {
        method: "PATCH",
        body: JSON.stringify({
          expected_version: editSeriesEntireForm.expected_version, course_id: editSeriesEntireForm.course_id,
          room_id: editSeriesEntireForm.room_id, teacher_id: editSeriesEntireForm.teacher_id, weekdays,
          start_local_time: editSeriesEntireForm.start_local_time, duration_minutes: editSeriesEntireForm.duration_minutes,
          end_date: editSeriesEntireUseCount ? null : editSeriesEntireForm.end_date, count: editSeriesEntireUseCount ? editSeriesEntireForm.count : null,
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
        <div className="text-xs text-gray-500 mb-3 text-right">
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
        <div className="border border-gray-200 rounded-sm overflow-hidden">
          <div className="grid" style={{ gridTemplateColumns: `repeat(${Math.max(daysInRange.length, 1)}, minmax(0, 1fr))` }}>
            {daysInRange.map((d) => {
              const key = d.toISOString().slice(0, 10);
              const items = sessionsByDay.get(key) ?? [];
              return (
                <div key={key} className="border-r border-gray-200 last:border-r-0">
                  <div className="bg-gray-50 border-b border-gray-200 px-3 py-2">
                    <div className="text-sm font-semibold text-gray-800">{key}</div>
                    <div className="text-xs text-gray-500">{items.length} session(s)</div>
                  </div>
                  <div className="p-2 space-y-2">
                    {items.length === 0 ? (
                      <div className="text-xs text-gray-400 px-1 py-3">No sessions</div>
                    ) : (
                      items.map((s) => {
                        const course = courseById.get(s.course_id);
                        const room = s.room_id ? roomById.get(s.room_id) : null;
                        const teacher = teacherById.get(s.teacher_id);
                        return (
                          <div key={s.id} className="w-full text-left border border-gray-200 rounded-sm px-2 py-2 hover:bg-gray-50">
                            <div className="text-xs font-mono text-gray-600">{s.start_at.slice(11, 16)}–{s.end_at.slice(11, 16)} UTC</div>
                            <div className="text-sm text-gray-900 font-semibold">{course ? `${course.code} — ${course.name}` : s.course_id}</div>
                            <div className="text-xs text-gray-600">{(room ? room.name : s.room_id ? s.room_id : "[NOT SET]")} • {teacher ? teacher.username : s.teacher_id}</div>
                            <SessionActions
                              session={s} cancelingId={cancelingId}
                              onAttendance={(sess) => void attendance.openAttendance(sess)}
                              onEdit={(sess) => edit.openModal(sess)}
                              onCancel={cancelOccurrence}
                              onEditSeriesTandF={(sess) => void openEditSeriesThisAndFuture(sess)}
                              onEditSeriesEntire={(sess) => void openEditSeriesEntire(sess)}
                              onCancelSeries={(sess) => void openCancelSeries(sess)}
                            />
                          </div>
                        );
                      })
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      ) : (
        <div className="overflow-x-auto"><table className="w-full text-[13px]">
          <thead>
            <tr className="border-b-2 border-gray-300">
              <th className="text-left py-2 px-2 font-semibold">Start</th>
              <th className="text-left py-2 px-2 font-semibold">End</th>
              <th className="text-left py-2 px-2 font-semibold">Course</th>
              <th className="text-left py-2 px-2 font-semibold">Room</th>
              <th className="text-left py-2 px-2 font-semibold">Teacher</th>
              <th className="text-left py-2 px-2 font-semibold"></th>
            </tr>
          </thead>
          <tbody>
            {sessions.map((s) => (
              <tr key={s.id} className="border-b border-gray-200 hover:bg-gray-50">
                <td className="py-2 px-2 font-mono text-xs text-gray-600">{s.start_at}</td>
                <td className="py-2 px-2 font-mono text-xs text-gray-600">{s.end_at}</td>
                <td className="py-2 px-2 font-mono text-xs text-gray-600">{courseById.get(s.course_id)?.code ?? s.course_id}</td>
                <td className="py-2 px-2 font-mono text-xs text-gray-600">{s.room_id ? (roomById.get(s.room_id)?.name ?? s.room_id) : "[NOT SET]"}</td>
                <td className="py-2 px-2 font-mono text-xs text-gray-600">{teacherById.get(s.teacher_id)?.username ?? s.teacher_id}</td>
                <td className="py-2 px-2 text-right">
                  <SessionActions
                    session={s} cancelingId={cancelingId}
                    onAttendance={(sess) => void attendance.openAttendance(sess)}
                    onEdit={(sess) => edit.openModal(sess)}
                    onCancel={cancelOccurrence}
                    onEditSeriesTandF={(sess) => void openEditSeriesThisAndFuture(sess)}
                    onEditSeriesEntire={(sess) => void openEditSeriesEntire(sess)}
                    onCancelSeries={(sess) => void openCancelSeries(sess)}
                  />
                </td>
              </tr>
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
                disabled={create.creating || isSaveDisabled({ status: create.preflight.status, loading: create.preflight.loading })}
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
                onClick={edit.submit}
                disabled={edit.saving || isSaveDisabled({ status: edit.preflight.status, loading: edit.preflight.loading })}
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

      {seriesOpen && (
        <Modal
          title="Create Series"
          onClose={() => setSeriesOpen(false)}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setSeriesOpen(false)}>Cancel</Button>
              <Button variant="primary" size="sm" onClick={submitSeries} disabled={seriesCreating || isSaveDisabled({ status: seriesPreflight.status, loading: seriesPreflight.loading })} loading={seriesPreflight.loading || seriesCreating}>
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
            <div className="flex items-center gap-2 text-xs text-gray-400 mb-1">
              <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-gray-900 text-white text-[10px] font-semibold">1</span>
              <span>Course & Teacher</span>
              <span className="text-gray-300 mx-1">→</span>
              <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-gray-200 text-gray-600 text-[10px] font-semibold">2</span>
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
                { label: "Start time", value: seriesForm.start_local_time },
                { label: "Duration", value: seriesForm.duration_minutes > 0 ? String(seriesForm.duration_minutes) : "" },
                { label: "Start date", value: seriesForm.start_date },
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
              <Button variant="primary" size="sm" onClick={submitEditSeriesThisAndFuture} disabled={editSeriesLoading || isSaveDisabled({ status: editSeriesPreflight.status, loading: editSeriesPreflight.loading })} loading={editSeriesPreflight.loading}>
                {getSaveButtonLabel({ status: editSeriesPreflight.status, loading: editSeriesPreflight.loading }, "Save", editSeriesPreflight.details)}
              </Button>
            </>
          }
        >
          {editSeriesLoading || !editSeriesForm ? (
            <div className="py-6 text-center text-sm text-gray-400">
              <span className="inline-block w-4 h-4 border-2 border-gray-400 border-t-transparent rounded-full animate-spin mr-2" aria-hidden="true" />
              Loading…
            </div>
          ) : (
            <div className="space-y-6">
              <div className="rounded-sm border border-gray-200 bg-gray-50 px-3 py-2 text-sm">
                <div className="font-medium text-gray-800">Scope</div>
                <div className="text-xs text-gray-700">Pivot date (Bangkok): <span className="font-mono">{editSeriesPivotDate}</span> (includes the selected occurrence)</div>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                <FormField name="es-course" label="Course (fixed)">
                  <div className="px-2 py-1.5 text-sm border border-gray-200 rounded-sm bg-gray-50">{courseById.get(editSeriesForm.course_id)?.code ?? editSeriesForm.course_id}</div>
                </FormField>
                <FormField name="es-room" label="Room (fixed)">
                  <div className="px-2 py-1.5 text-sm border border-gray-200 rounded-sm bg-gray-50">{editSeriesForm.room_id ? (roomById.get(editSeriesForm.room_id)?.name ?? editSeriesForm.room_id) : "[NOT SET]"}</div>
                </FormField>
                <FormField name="es-teacher" label="Teacher (fixed)">
                  <div className="px-2 py-1.5 text-sm border border-gray-200 rounded-sm bg-gray-50">{teacherById.get(editSeriesForm.teacher_id)?.username ?? editSeriesForm.teacher_id}</div>
                </FormField>
              </div>
              <SeriesFormFields
                weekdays={editSeriesForm.weekdays}
                onWeekdayChange={(idx) => { setEditSeriesForm(prev => { const next = prev.weekdays.slice(); next[idx] = !next[idx]; return { ...prev, weekdays: next }; }); }}
                startLocalTime={editSeriesForm.start_local_time}
                onStartLocalTimeChange={(v) => setEditSeriesForm(prev => ({ ...prev, start_local_time: v }))}
                durationMinutes={editSeriesForm.duration_minutes}
                onDurationMinutesChange={(v) => setEditSeriesForm(prev => ({ ...prev, duration_minutes: v }))}
                useCount={editSeriesUseCount}
                onUseCountChange={setEditSeriesUseCount}
                count={editSeriesForm.count}
                onCountChange={(v) => setEditSeriesForm(prev => ({ ...prev, count: v }))}
                endDate={editSeriesForm.end_date}
                onEndDateChange={(v) => setEditSeriesForm(prev => ({ ...prev, end_date: v }))}
                prefix="es-"
              />
              <PreflightIndicator preflight={editSeriesPreflight} coursesById={courseById} teachersById={teacherById} roomsById={roomById}
                requiredFields={[
                  { label: "Start time", value: editSeriesForm.start_local_time },
                  { label: "Duration", value: editSeriesForm.duration_minutes > 0 ? String(editSeriesForm.duration_minutes) : "" },
                  { label: "End date", value: editSeriesForm.end_date },
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
              <Button variant="primary" size="sm" onClick={submitEditSeriesEntire} disabled={editSeriesEntireLoading || isSaveDisabled({ status: editSeriesEntirePreflight.status, loading: editSeriesEntirePreflight.loading })} loading={editSeriesEntirePreflight.loading}>
                {getSaveButtonLabel({ status: editSeriesEntirePreflight.status, loading: editSeriesEntirePreflight.loading }, "Save", editSeriesEntirePreflight.details)}
              </Button>
            </>
          }
        >
          {editSeriesEntireLoading || !editSeriesEntireForm ? (
            <div className="py-6 text-center text-sm text-gray-400">
              <span className="inline-block w-4 h-4 border-2 border-gray-400 border-t-transparent rounded-full animate-spin mr-2" aria-hidden="true" />
              Loading…
            </div>
          ) : (
            <div className="space-y-6">
              <div className="rounded-sm border border-gray-200 bg-gray-50 px-3 py-2 text-sm">
                <div className="font-medium text-gray-800">Scope</div>
                <div className="text-xs text-gray-700">Applies to future occurrences from (Bangkok): <span className="font-mono">{editSeriesEntireFromDate}</span></div>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                <FormField name="ese-course_id" label="Course">
                  <TypeaheadSelect value={editSeriesEntireForm.course_id} onChange={(v) => setEditSeriesEntireForm(prev => ({ ...prev, course_id: v }))} options={courseOptions} placeholder="Search course…" />
                </FormField>
                <FormField name="ese-room_id" label="Room">
                  <Select size="sm" value={editSeriesEntireForm.room_id ?? ""} onChange={(e) => setEditSeriesEntireForm(prev => ({ ...prev, room_id: e.target.value ? e.target.value : null }))}>
                    <option value="">[NOT SET] (Provisional)</option>
                    {rooms.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
                  </Select>
                </FormField>
                <FormField name="ese-teacher_id" label="Teacher">
                  <TypeaheadSelect value={editSeriesEntireForm.teacher_id} onChange={(v) => setEditSeriesEntireForm(prev => ({ ...prev, teacher_id: v }))} options={teacherOptions} placeholder="Search teacher…" />
                </FormField>
              </div>
              <SeriesFormFields
                weekdays={editSeriesEntireForm.weekdays}
                onWeekdayChange={(idx) => { setEditSeriesEntireForm(prev => { const next = prev.weekdays.slice(); next[idx] = !next[idx]; return { ...prev, weekdays: next }; }); }}
                startLocalTime={editSeriesEntireForm.start_local_time}
                onStartLocalTimeChange={(v) => setEditSeriesEntireForm(prev => ({ ...prev, start_local_time: v }))}
                durationMinutes={editSeriesEntireForm.duration_minutes}
                onDurationMinutesChange={(v) => setEditSeriesEntireForm(prev => ({ ...prev, duration_minutes: v }))}
                useCount={editSeriesEntireUseCount}
                onUseCountChange={setEditSeriesEntireUseCount}
                count={editSeriesEntireForm.count}
                onCountChange={(v) => setEditSeriesEntireForm(prev => ({ ...prev, count: v }))}
                endDate={editSeriesEntireForm.end_date}
                onEndDateChange={(v) => setEditSeriesEntireForm(prev => ({ ...prev, end_date: v }))}
                prefix="ese-"
              />
              <PreflightIndicator preflight={editSeriesEntirePreflight} coursesById={courseById} teachersById={teacherById} roomsById={roomById}
                requiredFields={[
                  { label: "Course", value: editSeriesEntireForm.course_id },
                  { label: "Teacher", value: editSeriesEntireForm.teacher_id },
                  { label: "Start time", value: editSeriesEntireForm.start_local_time },
                  { label: "Duration", value: editSeriesEntireForm.duration_minutes > 0 ? String(editSeriesEntireForm.duration_minutes) : "" },
                  { label: "End date", value: editSeriesEntireForm.end_date },
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
            <div className="py-6 text-center text-sm text-gray-400">
              <span className="inline-block w-4 h-4 border-2 border-gray-400 border-t-transparent rounded-full animate-spin mr-2" aria-hidden="true" />
              Loading…
            </div>
          ) : (
            <div className="space-y-6">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Scope</label>
                  <Select size="sm" value={cancelSeriesScope} onChange={(e) => setCancelSeriesScope(e.target.value as any)}>
                    <option value="this_and_future">This & future (includes selected occurrence)</option>
                    <option value="entire_series_future_only">Entire series (future only)</option>
                  </Select>
                </div>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Pivot date (Bangkok)</label>
                  <input
                    type="date"
                    value={cancelSeriesPivotDate}
                    onChange={(e) => setCancelSeriesPivotDate(e.target.value)}
                    disabled={cancelSeriesScope !== "this_and_future"}
                    className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm disabled:bg-gray-50"
                  />
                </div>
              </div>
              <div className="text-xs text-gray-600">Uses optimistic concurrency; if someone changed the series, you'll get a stale edit and can retry.</div>
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
