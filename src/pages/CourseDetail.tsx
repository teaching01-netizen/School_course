import { Fragment, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import Modal from "../components/Modal";
import TypeaheadSelect from "../components/TypeaheadSelect";
import { ApiRequestError, apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import { useAuth } from "../hooks/useAuth";
import { usePreflight } from "@/hooks/usePreflight";
import { PreflightIndicator, PreflightBadge, getSaveButtonLabel, isSaveDisabled } from "@/components/PreflightIndicator";
import { formatUTCToZone, utcISOToZoneDate, zoneLocalInputToUTCISO, groupSessionKey } from "../utils/timezone";
import { AttendeeSection } from "../components/AttendeeSection";
import ScheduleSessionCard from "../components/ScheduleSessionCard";
import { validateSeriesPreflight, type SeriesPreflightForm } from "@/utils/preflight";
import { parseSchedulePaste } from "@/utils/schedulePaste";
import { addDays, format, startOfWeek } from 'date-fns';
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import Input from "../components/ui/Input";
import Select from "../components/ui/Select";
import ConfirmModal from "../components/ConfirmModal";
import EmptyState from "../components/ui/EmptyState";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import {
  yyyyMmDd,
  minutesBetween,
  fmtDuration,
  type Session,
  type Course,
  type Room,
  type User,
  type Student,
} from "@/types";

export default function CourseDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { addToast } = useToast();
  const { user } = useAuth();

  const [confirmDelete, setConfirmDelete] = useState(false);
  const [confirmRemoveStudent, setConfirmRemoveStudent] = useState<string | null>(null);
  const [crmEnabled, setCrmEnabled] = useState(false);
  const [crmLocked, setCrmLocked] = useState(false);

  const loadCrmFilter = async () => {
    if (!id) return;
    try {
      const res = await apiJson<{ enabled: boolean; locked: boolean; filter: any }>(
        `/api/v1/courses/${id}/crm-filter`,
        { method: "GET" },
      );
      setCrmEnabled(res.enabled);
      setCrmLocked(res.locked);
    } catch {
      // Not configured or not admin; ignore.
    }
  };

  const onRosterChanged = async () => {
    await loadRoster();
    await loadCrmFilter();
  };
  const [course, setCourse] = useState<Course | null>(null);
  const [loading, setLoading] = useState(true);
  const [deleting, setDeleting] = useState(false);
  const [roster, setRoster] = useState<Student[]>([]);
  const [rosterLoading, setRosterLoading] = useState(false);
  const [addingWcode, setAddingWcode] = useState("");
  const [adding, setAdding] = useState(false);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [rooms, setRooms] = useState<Room[]>([]);
  const [teachers, setTeachers] = useState<User[]>([]);
  const [instituteTZ, setInstituteTZ] = useState<string | null>(null);
  const [serverNow, setServerNow] = useState<string | null>(null);
  const today = useMemo(() => new Date(), []);
  const todayStr = useMemo(() => yyyyMmDd(today), [today]);
  const zone = instituteTZ ?? "Asia/Bangkok";

  const [viewMode, setViewMode] = useState<'table' | 'calendar'>('table');
  const [weekStart, setWeekStart] = useState(() => startOfWeek(new Date(), { weekStartsOn: 1 }));
  const weekEnd = useMemo(() => addDays(weekStart, 6), [weekStart]);

  const teacherById = useMemo(() => new Map(teachers.map((t) => [t.id, t.username])), [teachers]);
  const roomById = useMemo(() => new Map(rooms.map((r) => [r.id, r])), [rooms]);

  const sessionsByWeekdayAndHour = useMemo(() => {
    const map = new Map<string, Session[]>();
    for (const s of sessions) {
      const key = groupSessionKey(s.start_at, zone);
      if (!key) continue;
      const group = map.get(key);
      if (group) {
        group.push(s);
      } else {
        map.set(key, [s]);
      }
    }
    return map;
  }, [sessions, zone]);

  const roomNameById = useMemo(() => {
    const m = new Map<string, string>();
    for (const r of rooms) m.set(r.id, r.name);
    return m;
  }, [rooms]);
  const teacherOptions = useMemo(() => teachers.map((t) => ({ value: t.id, label: t.username, keywords: t.id })), [teachers]);

  const [editingSessionId, setEditingSessionId] = useState<string | null>(null);
  const [editForm, setEditForm] = useState({ date: "", begin: "", end: "", room_id: "" as string });
  const editPreflight = usePreflight();
  const [editSaving, setEditSaving] = useState(false);

  const getEditSession = () => sessions.find((s) => s.id === editingSessionId) ?? null;

  const openEditSession = (s: Session) => {
    const date = utcISOToZoneDate(s.start_at, zone) ?? s.start_at.slice(0, 10);
    const begin = formatUTCToZone(s.start_at, zone, "HH:mm") ?? s.start_at.slice(11, 16);
    const end = formatUTCToZone(s.end_at, zone, "HH:mm") ?? s.end_at.slice(11, 16);
    setEditingSessionId(s.id);
    setEditForm({ date, begin, end, room_id: s.room_id ?? "" });
    editPreflight.reset();
  };

  const cancelEditSession = () => {
    setEditingSessionId(null);
    setEditForm({ date: "", begin: "", end: "", room_id: "" });
    editPreflight.reset();
  };

  const runEditPreflight = async () => {
    const s = getEditSession();
    if (!s) {
      editPreflight.reset();
      return;
    }
    if (!editForm.date || !editForm.begin || !editForm.end) {
      editPreflight.reset();
      return;
    }
    const startISO = zoneLocalInputToUTCISO(`${editForm.date}T${editForm.begin}`, zone);
    const endISO = zoneLocalInputToUTCISO(`${editForm.date}T${editForm.end}`, zone);
    if (!startISO || !endISO || endISO <= startISO) {
      editPreflight.reset();
      return;
    }
    await editPreflight.check({
      session_id: s.id,
      course_id: s.course_id,
      room_id: editForm.room_id ? editForm.room_id : null,
      teacher_id: s.teacher_id,
      start_at: startISO,
      end_at: endISO,
    });
  };

  useEffect(() => {
    void runEditPreflight();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editingSessionId, zone, editForm.date, editForm.begin, editForm.end, editForm.room_id]);

  const submitEditSession = async () => {
    const s = getEditSession();
    if (!s) return;
    const startISO = zoneLocalInputToUTCISO(`${editForm.date}T${editForm.begin}`, zone);
    const endISO = zoneLocalInputToUTCISO(`${editForm.date}T${editForm.end}`, zone);
    if (!startISO || !endISO || endISO <= startISO) {
      addToast("error", "Invalid date/time");
      return;
    }
    if (editPreflight.loading) {
      addToast("error", "Checking availability…");
      return;
    }
    if (editPreflight.status !== "available" && editPreflight.status !== "provisional") {
      addToast("error", "Preflight must pass before saving");
      return;
    }
    try {
      setEditSaving(true);
      await apiJson<{ session: Session }>(`/api/v1/sessions/${s.id}`, {
        method: "PATCH",
        body: JSON.stringify({
          expected_version: s.version,
          course_id: s.course_id,
          room_id: editForm.room_id ? editForm.room_id : null,
          teacher_id: s.teacher_id,
          start_at: startISO,
          end_at: endISO,
        }),
      });
      addToast("success", "Updated session");
      cancelEditSession();
      await loadSessions();
    } catch (err) {
      if (err instanceof ApiRequestError) {
        if (err.code === "stale_edit") {
          addToast("error", "Stale edit: reloaded latest session. Please edit again.");
          cancelEditSession();
          await loadSessions();
          return;
        }
        addToast("error", `${err.code}: ${err.message}`);
        return;
      }
      addToast("error", err instanceof Error ? err.message : "Update failed");
    } finally {
      setEditSaving(false);
    }
  };

  useEffect(() => {
    (async () => {
      if (!id) return;
      try {
        setLoading(true);
        const c = await apiJson<Course>(`/api/v1/courses/${id}`, { method: "GET" });
        setCourse(c);
      } catch (err) {
        addToast("error", err instanceof Error ? err.message : "Failed to load course");
      } finally {
        setLoading(false);
      }
    })();
    void loadCrmFilter();
  }, [addToast, id]);

  const loadLookups = async () => {
    try {
      const [r, t] = await Promise.all([
        apiJson<Room[]>("/api/v1/rooms", { method: "GET" }),
        apiJson<User[]>("/api/v1/users?role=Teacher", { method: "GET" }),
      ]);
      setRooms(r);
      setTeachers(t);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load lookups");
    }
  };

  const loadRoster = async () => {
    if (!id) return;
    try {
      setRosterLoading(true);
      const st = await apiJson<Student[]>(`/api/v1/courses/${id}/students`, { method: "GET" });
      setRoster(st);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load roster");
    } finally {
      setRosterLoading(false);
    }
  };

  const loadSessions = async () => {
    if (!id) return;
    try {
      setSessionsLoading(true);
      const items = await apiJson<Session[]>(`/api/v1/courses/${id}/sessions`, { method: "GET" });
      setSessions(items);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load sessions");
    } finally {
      setSessionsLoading(false);
    }
  };

  useEffect(() => {
    void loadRoster();
    void loadSessions();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  useEffect(() => {
    void loadLookups();
    void (async () => {
      try {
        const meta = await apiJson<{ institute_tz: string; server_now: string }>(`/api/v1/meta/time`, { method: "GET" });
        setInstituteTZ(meta.institute_tz);
        setServerNow(meta.server_now);
      } catch {
        // Best-effort only.
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const onDelete = async () => {
    if (!id) return;
    try {
      setDeleting(true);
      await apiJson(`/api/v1/courses/${id}`, { method: "DELETE" });
      addToast("success", "Course archived");
      navigate("/courses");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Delete failed");
    } finally {
      setDeleting(false);
      setConfirmDelete(false);
    }
  };

  const removeStudent = async (studentId: string): Promise<void> => {
    setConfirmRemoveStudent(studentId);
  };

  const handleConfirmRemoveStudent = async () => {
    if (!id || !confirmRemoveStudent) return;
    try {
      await apiJson(`/api/v1/courses/${id}/students/${confirmRemoveStudent}`, { method: "DELETE" });
      addToast("success", "Removed student");
      setConfirmRemoveStudent(null);
      await loadRoster();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Remove failed");
    }
  };

  const addStudentByWcode = async () => {
    if (!id) return;
    const w = addingWcode.trim();
    if (!w) return;
    try {
      setAdding(true);
      // Find student by wcode via existing student lookup endpoint.
      const st = await apiJson<Student>(`/api/v1/students/${encodeURIComponent(w)}`, { method: "GET" });
      await apiJson(`/api/v1/courses/${id}/students`, { method: "POST", body: JSON.stringify({ student_id: st.id }) });
      addToast("success", "Added student");
      setAddingWcode("");
      await loadRoster();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Add failed");
    } finally {
      setAdding(false);
    }
  };

  const [createOpen, setCreateOpen] = useState(false);
  const [createTab, setCreateTab] = useState<"series" | "session" | "paste">("series");

  const [creatingSession, setCreatingSession] = useState(false);
  const [sessionForm, setSessionForm] = useState({
    room_id: "" as string, // "" => no room (send null)
    teacher_id: "",
    start_local: "",
    end_local: "",
  });
  const sessionPreflight = usePreflight();
  const [pasteText, setPasteText] = useState("");
  const [pasteTeacherId, setPasteTeacherId] = useState("");
  const [creatingPaste, setCreatingPaste] = useState(false);
  const parsedPaste = useMemo(() => parseSchedulePaste(pasteText), [pasteText]);
  const roomIdByPastedName = useMemo(() => {
    const map = new Map<string, string>();
    for (const room of rooms) map.set(room.name.trim().toLowerCase(), room.id);
    return map;
  }, [rooms]);

  const [creatingSeries, setCreatingSeries] = useState(false);
  const [seriesUseCount, setSeriesUseCount] = useState(false);
  const [seriesForm, setSeriesForm] = useState({
    room_id: "" as string,
    teacher_id: "",
    weekdays: [false, false, false, false, false, false, false] as boolean[],
    start_local_time: "16:00",
    duration_minutes: 120,
    start_date: todayStr,
    end_date: todayStr,
    count: 10,
  });
  const seriesPreflight = usePreflight("preflight_series");

  const openCreate = (tab: "series" | "session" | "paste" = "series") => {
    setCreateOpen(true);
    setCreateTab(tab);
    setSessionForm({
      room_id: "",
      teacher_id: teachers[0]?.id ?? "",
      start_local: "",
      end_local: "",
    });
    setPasteTeacherId(teachers[0]?.id ?? "");
    setPasteText("");
    sessionPreflight.reset();
    setSeriesUseCount(false);
    setSeriesForm({
      room_id: "",
      teacher_id: teachers[0]?.id ?? "",
      weekdays: [false, false, false, false, false, false, false],
      start_local_time: "16:00",
      duration_minutes: 120,
      start_date: todayStr,
      end_date: todayStr,
      count: 10,
    });
    seriesPreflight.reset();
  };

  const runSessionPreflight = async () => {
    if (!createOpen || createTab !== "session") return;
    if (!id || !sessionForm.teacher_id || !sessionForm.start_local || !sessionForm.end_local) {
      sessionPreflight.reset();
      return;
    }
    const startISO = zoneLocalInputToUTCISO(sessionForm.start_local, zone);
    const endISO = zoneLocalInputToUTCISO(sessionForm.end_local, zone);
    if (!startISO || !endISO || endISO <= startISO) {
      sessionPreflight.reset();
      return;
    }
    await sessionPreflight.check({
      session_id: null,
      course_id: id,
      room_id: sessionForm.room_id ? sessionForm.room_id : null,
      teacher_id: sessionForm.teacher_id,
      start_at: startISO,
      end_at: endISO,
    });
  };

  useEffect(() => {
    void runSessionPreflight();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [createOpen, createTab, id, zone, sessionForm.room_id, sessionForm.teacher_id, sessionForm.start_local, sessionForm.end_local]);

  const submitSession = async () => {
    if (!id || !sessionForm.teacher_id) return;
    const startISO = zoneLocalInputToUTCISO(sessionForm.start_local, zone);
    const endISO = zoneLocalInputToUTCISO(sessionForm.end_local, zone);
    if (!startISO || !endISO || endISO <= startISO) {
      addToast("error", "Invalid start/end");
      return;
    }
    if (sessionPreflight.loading) {
      addToast("error", "Checking availability…");
      return;
    }
    if (sessionPreflight.status !== "available" && sessionPreflight.status !== "provisional") {
      addToast("error", "Preflight must pass before saving");
      return;
    }
    try {
      setCreatingSession(true);
      await apiJson("/api/v1/sessions", {
        method: "POST",
        body: JSON.stringify({
          course_id: id,
          room_id: sessionForm.room_id ? sessionForm.room_id : null,
          teacher_id: sessionForm.teacher_id,
          start_at: startISO,
          end_at: endISO,
        }),
      });
      addToast("success", "Session created");
      setCreateOpen(false);
      await loadSessions();
    } catch (err) {
      if (err instanceof ApiRequestError && err.details) {
        addToast("error", `${err.code ?? "error"}: ${err.message}`);
        return;
      }
      addToast("error", err instanceof Error ? err.message : "Create failed");
    } finally {
      setCreatingSession(false);
    }
  };

  const submitPastedSessions = async () => {
    if (!id || !pasteTeacherId || parsedPaste.rows.length === 0 || parsedPaste.errors.length > 0) return;
    try {
      setCreatingPaste(true);
      for (const row of parsedPaste.rows) {
        const startISO = zoneLocalInputToUTCISO(`${row.date}T${row.begin}`, zone);
        const endISO = zoneLocalInputToUTCISO(`${row.date}T${row.end}`, zone);
        if (!startISO || !endISO || endISO <= startISO) {
          throw new Error(`Invalid time on pasted row ${row.rowNumber}`);
        }
        const roomID = row.classroom ? roomIdByPastedName.get(row.classroom.trim().toLowerCase()) ?? null : null;
        await apiJson("/api/v1/sessions", {
          method: "POST",
          body: JSON.stringify({
            course_id: id,
            room_id: roomID,
            teacher_id: pasteTeacherId,
            start_at: startISO,
            end_at: endISO,
          }),
        });
      }
      addToast("success", `Created ${parsedPaste.rows.length} sessions`);
      setCreateOpen(false);
      setPasteText("");
      await loadSessions();
    } catch (err) {
      if (err instanceof ApiRequestError && err.details) {
        addToast("error", `${err.code ?? "error"}: ${err.message}`);
        return;
      }
      addToast("error", err instanceof Error ? err.message : "Create pasted sessions failed");
    } finally {
      setCreatingPaste(false);
    }
  };

  const runSeriesPreflight = async () => {
    if (!createOpen || createTab !== "series") { seriesPreflight.reset(); return; }
    if (!id) { seriesPreflight.reset(); return; }
    const validated = validateSeriesPreflight(
      { ...seriesForm, course_id: id } as SeriesPreflightForm,
      seriesUseCount
    );
    if (!validated) { seriesPreflight.reset(); return; }
    await seriesPreflight.check({
      course_id: id,
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

  useEffect(() => {
    void runSeriesPreflight();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    createOpen,
    createTab,
    id,
    seriesUseCount,
    zone,
    seriesForm.room_id,
    seriesForm.teacher_id,
    seriesForm.start_local_time,
    seriesForm.duration_minutes,
    seriesForm.start_date,
    seriesForm.end_date,
    seriesForm.count,
    ...seriesForm.weekdays,
  ]);

  const submitSeries = async () => {
    if (!id) return;
    const validated = validateSeriesPreflight(
      { ...seriesForm, course_id: id } as SeriesPreflightForm,
      seriesUseCount
    );
    if (!validated) { addToast("error", "Please complete schedule fields"); return; }
    if (seriesPreflight.loading) {
      addToast("error", "Checking availability…");
      return;
    }
    if (seriesPreflight.status !== "available" && seriesPreflight.status !== "provisional") {
      addToast("error", "Preflight must pass before saving");
      return;
    }
    try {
      setCreatingSeries(true);
      await apiJson("/api/v1/series", {
        method: "POST",
        body: JSON.stringify({
          course_id: id,
          room_id: validated.room_id,
          teacher_id: seriesForm.teacher_id,
          weekdays: validated.weekdays,
          start_local_time: seriesForm.start_local_time,
          duration_minutes: seriesForm.duration_minutes,
          start_date: seriesForm.start_date,
          end_date: validated.end_date,
          count: validated.count,
        }),
      });
      addToast("success", "Series created");
      setCreateOpen(false);
      await loadSessions();
    } catch (err) {
      if (err instanceof ApiRequestError && err.details) {
        addToast("error", `${err.code ?? "error"}: ${err.message}`);
        return;
      }
      addToast("error", err instanceof Error ? err.message : "Create failed");
    } finally {
      setCreatingSeries(false);
    }
  };

  if (loading) return <LoadingSkeleton type="card" lines={3} />;
  if (!course) return <EmptyState message="Course not found" />;

  return (
    <div>
      <div className="flex items-baseline justify-between gap-3 mb-2">
        <PageHeading>
          Course <span className="text-gray-400">#{course.code}</span>
        </PageHeading>
        <div className="flex gap-2">
          <Link to={`/courses/${course.id}/edit`} className="px-3 py-1.5 text-sm bg-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary-dark)] text-white rounded-sm">
            Edit
          </Link>
          <Button variant="danger" size="md" onClick={() => setConfirmDelete(true)} loading={deleting}>
            {deleting ? "Archiving…" : "Delete"}
          </Button>
        </div>
      </div>

      <div className="border-b border-gray-200 pb-3 mb-6">
        <div className="text-sm text-gray-700">{course.name}</div>
        <div className="text-xs text-gray-400 font-mono">{course.id}</div>
      </div>

      <div className="mb-8">
        <div className="flex items-end justify-between gap-3 mb-3">
          <h2 className="text-[28px] font-bold text-gray-800">Schedule</h2>
          <div className="flex items-end gap-2 flex-wrap">
            <div className="inline-flex rounded-sm border border-gray-200 overflow-hidden self-end">
              <button
                type="button"
                onClick={() => setViewMode('table')}
                className={`px-2 py-1 text-[11px] ${viewMode === 'table' ? 'bg-gray-900 text-white' : 'bg-white hover:bg-gray-50 text-gray-700'}`}
              >
                Table
              </button>
              <button
                type="button"
                onClick={() => setViewMode('calendar')}
                className={`px-2 py-1 text-[11px] ${viewMode === 'calendar' ? 'bg-gray-900 text-white' : 'bg-white hover:bg-gray-50 text-gray-700'}`}
              >
                Calendar
              </button>
            </div>
            {viewMode === 'table' && (
              <>
                <div className="text-[11px] text-gray-400 self-end pb-1">
                  TZ: {zone}
                  {serverNow ? ` • Server now: ${serverNow}` : ""}
                </div>
                <Button variant="secondary" size="md" onClick={loadSessions}>Refresh</Button>
                <Button variant="primary" size="md" onClick={() => openCreate("series")}>Add…</Button>
              </>
            )}
            {viewMode === 'calendar' && (
              <div className="flex items-center gap-1.5 self-end pb-0.5">
                <Button variant="ghost" size="sm" onClick={() => setWeekStart(prev => addDays(prev, -7))}>
                  &lsaquo; Prev
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setWeekStart(startOfWeek(new Date(), { weekStartsOn: 1 }))}>
                  Today
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setWeekStart(prev => addDays(prev, 7))}>
                  Next &rsaquo;
                </Button>
                <span className="text-xs text-gray-500 ml-1 font-mono">
                  {format(weekStart, 'MMM d')} – {format(weekEnd, 'MMM d, yyyy')}
                </span>
              </div>
            )}
          </div>
        </div>

        {viewMode === 'calendar' ? (
          <div className="border border-gray-200 p-4 bg-white">
            <div className="overflow-x-auto"><table className="w-full text-[12px] border border-gray-200">
              <thead>
                <tr className="bg-gray-50">
                  <th className="text-left py-1 px-1 font-semibold border-r border-gray-200 w-12">Time</th>
                  {['MON', 'TUE', 'WED', 'THU', 'FRI'].map((d) => (
                    <th key={d} className="text-center py-1 px-1 font-semibold border-r border-gray-200 min-w-[100px]">{d}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {Array.from({ length: 24 }, (_, i) => `${String(i).padStart(2, '0')}:00`).map((slot) => (
                  <tr key={slot} className="border-b border-gray-200">
                    <td className="py-1 px-1 text-xs text-gray-500 font-medium border-r border-gray-200">{slot}</td>
                    {[1,2,3,4,5].map((day) => {
                      const sessList = sessionsByWeekdayAndHour.get(`${day}-${slot}`) ?? [];
                      return (
                        <td key={day} className="px-1 py-1 border-r border-gray-200 align-top">
                          {sessList.length > 0 ? (
                            <div className="space-y-0.5">
                              {sessList.map((sess) => {
                                const room = roomById.get(sess.room_id ?? '');
                                return (
                                  <ScheduleSessionCard
                                    key={sess.id}
                                    session={sess}
                                    course={course}
                                    room={room}
                                    teacherName={teacherById.get(sess.teacher_id)}
                                  />
                                );
                              })}
                            </div>
                          ) : sessionsLoading ? (
                            <div className="animate-pulse space-y-1.5">
                              <div className="h-7 bg-gray-100 rounded-sm" />
                              <div className="h-7 bg-gray-100 rounded-sm w-3/4" />
                            </div>
                          ) : null}
                        </td>
                      );
                    })}
                  </tr>
                ))}
              </tbody>
            </table></div>
          </div>
        ) : (
          <div className="border border-gray-200 rounded-sm overflow-hidden">
          <div className="overflow-x-auto"><table className="w-full text-[13px]">
            <thead className="bg-gray-50">
              <tr className="border-b border-gray-200">
                <th className="text-left py-2 px-3 font-semibold text-gray-700">Date</th>
                <th className="text-left py-2 px-3 font-semibold text-gray-700">Begin</th>
                <th className="text-left py-2 px-3 font-semibold text-gray-700">End</th>
                <th className="text-left py-2 px-3 font-semibold text-gray-700">Duration</th>
                <th className="text-left py-2 px-3 font-semibold text-gray-700">Classroom</th>
                <th className="text-left py-2 px-3 font-semibold text-gray-700">By</th>
              </tr>
            </thead>
            <tbody>
              {sessionsLoading ? (
                <tr>
                  <td colSpan={6}>
                    <LoadingSkeleton type="table" lines={3} />
                  </td>
                </tr>
              ) : sessions.length === 0 ? (
                <tr>
                  <td colSpan={6}>
                    <EmptyState message="No sessions in range" />
                  </td>
                </tr>
              ) : (
                sessions.map((s) => {
                  const mins = minutesBetween(s.start_at, s.end_at);
                  const dateLabel = utcISOToZoneDate(s.start_at, zone) ?? s.start_at.slice(0, 10);
                  const begin = formatUTCToZone(s.start_at, zone, "HH:mm") ?? s.start_at.slice(11, 16);
                  const end = formatUTCToZone(s.end_at, zone, "HH:mm") ?? s.end_at.slice(11, 16);
                  const isEditing = editingSessionId === s.id;
                  return (
                    <Fragment key={s.id}>
                      <tr className="border-b border-gray-100 hover:bg-gray-50">
                        <td className="py-2 px-3">
                          {isEditing ? (
                            <Input type="date" size="sm" value={editForm.date} onChange={(e) => setEditForm((p) => ({ ...p, date: e.target.value }))} />
                          ) : (
                            dateLabel
                          )}
                        </td>
                        <td className="py-2 px-3 font-mono text-xs text-gray-700">
                          {isEditing ? (
                            <Input type="time" size="sm" step={300} value={editForm.begin} onChange={(e) => setEditForm((p) => ({ ...p, begin: e.target.value }))} />
                          ) : (
                            begin
                          )}
                        </td>
                        <td className="py-2 px-3 font-mono text-xs text-gray-700">
                          {isEditing ? (
                            <Input type="time" size="sm" step={300} value={editForm.end} onChange={(e) => setEditForm((p) => ({ ...p, end: e.target.value }))} />
                          ) : (
                            end
                          )}
                        </td>
                        <td className="py-2 px-3 font-mono text-xs text-gray-700">{mins == null ? "—" : fmtDuration(mins)}</td>
                        <td className="py-2 px-3">
                          {isEditing ? (
                            <Select size="sm" value={editForm.room_id} onChange={(e) => setEditForm((p) => ({ ...p, room_id: e.target.value }))}>
                              <option value="">[NOT SET] (Provisional)</option>
                              {rooms.map((r) => (
                                <option key={r.id} value={r.id}>
                                  {r.name}
                                </option>
                              ))}
                            </Select>
                          ) : s.room_id ? (
                            <span className="inline-flex items-center px-2 py-0.5 rounded-sm text-xs bg-gray-200 text-gray-700">
                              {roomNameById.get(s.room_id) ?? "SET"}
                            </span>
                          ) : (
                            <span className="inline-flex items-center px-2 py-0.5 rounded-sm text-xs bg-[var(--color-wi-yellow)] text-white">[NOT SET]</span>
                          )}
                        </td>
                        <td className="py-2 px-3">
                          {isEditing ? (
                            <div className="flex items-center gap-2">
                              <Button
                                variant="primary"
                                size="sm"
                                onClick={submitEditSession}
                                disabled={editSaving || isSaveDisabled({ status: editPreflight.status, loading: editPreflight.loading }) || !editPreflight.status}
                                loading={editPreflight.loading || editSaving}
                              >
                                {editSaving ? "Saving…" : getSaveButtonLabel({ status: editPreflight.status, loading: editPreflight.loading }, "Save", editPreflight.details)}
                              </Button>
                              <Button variant="ghost" size="sm" onClick={cancelEditSession} disabled={editSaving}>
                                Cancel
                              </Button>
                              <PreflightBadge status={editPreflight.status} details={editPreflight.details} loading={editPreflight.loading} />
                            </div>
                          ) : (
                            <div className="flex items-center gap-2">
                              <Button variant="ghost" size="sm" onClick={() => openEditSession(s)}>
                                Edit
                              </Button>
                              <Link
                                to={`/schedule`}
                                className="inline-flex items-center px-2 py-1 rounded-sm text-xs bg-[var(--color-wi-blue)] hover:bg-[var(--color-wi-blue-dark)] text-white"
                              >
                                check-in
                              </Link>
                            </div>
                          )}
                        </td>
                      </tr>
                      {isEditing && editPreflight.details && (
                        <tr className="border-b border-gray-100 bg-red-50/40">
                          <td className="py-2 px-3" colSpan={6}>
                            <PreflightIndicator
                              preflight={editPreflight}
                              coursesById={course ? new Map([[course.id, course]]) : new Map()}
                              teachersById={new Map(teachers.map((t) => [t.id, t]))}
                              roomsById={roomById}
                              requiredFields={[
                                { label: "Date", value: editForm.date },
                                { label: "Start time", value: editForm.begin },
                                { label: "End time", value: editForm.end },
                              ]}
                            />
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  );
                })
              )}
            </tbody>
          </table></div>
        </div>
        )}
      </div>

      {createOpen && (
        <Modal
          title="Add to Schedule"
          onClose={() => setCreateOpen(false)}
          maxWidth="max-w-3xl"
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setCreateOpen(false)}>Close</Button>
              {createTab === "series" ? (
                <Button
                  variant="primary"
                  size="sm"
                  onClick={submitSeries}
                  disabled={creatingSeries || isSaveDisabled({ status: seriesPreflight.status, loading: seriesPreflight.loading })}
                  loading={seriesPreflight.loading || creatingSeries}
                >
                  {creatingSeries ? "Creating…" : getSaveButtonLabel({ status: seriesPreflight.status, loading: seriesPreflight.loading }, "Create series", seriesPreflight.details)}
                </Button>
              ) : createTab === "session" ? (
                <Button
                  variant="primary"
                  size="sm"
                  onClick={submitSession}
                  disabled={creatingSession || isSaveDisabled({ status: sessionPreflight.status, loading: sessionPreflight.loading })}
                  loading={sessionPreflight.loading || creatingSession}
                >
                  {creatingSession ? "Creating…" : getSaveButtonLabel({ status: sessionPreflight.status, loading: sessionPreflight.loading }, "Create session", sessionPreflight.details)}
                </Button>
              ) : (
                <Button
                  variant="primary"
                  size="sm"
                  onClick={submitPastedSessions}
                  disabled={creatingPaste || !pasteTeacherId || parsedPaste.rows.length === 0 || parsedPaste.errors.length > 0}
                  loading={creatingPaste}
                >
                  {creatingPaste ? "Creating…" : `Create ${parsedPaste.rows.length} sessions`}
                </Button>
              )}
            </>
          }
        >
          <div className="space-y-4">
            <div className="flex items-center justify-between gap-3">
              <div className="inline-flex rounded-sm border border-gray-200 overflow-hidden">
                <button
                  type="button"
                  onClick={() => setCreateTab("series")}
                  className={`px-3 py-1.5 text-sm ${createTab === "series" ? "bg-gray-900 text-white" : "bg-white hover:bg-gray-50 text-gray-700"}`}
                >
                  Recurring series
                </button>
                <button
                  type="button"
                  onClick={() => setCreateTab("session")}
                  className={`px-3 py-1.5 text-sm ${createTab === "session" ? "bg-gray-900 text-white" : "bg-white hover:bg-gray-50 text-gray-700"}`}
                >
                  One-off session
                </button>
                <button
                  type="button"
                  onClick={() => setCreateTab("paste")}
                  className={`px-3 py-1.5 text-sm ${createTab === "paste" ? "bg-gray-900 text-white" : "bg-white hover:bg-gray-50 text-gray-700"}`}
                >
                  Paste schedule
                </button>
              </div>
              <div className="text-xs text-gray-500">
                Course: <span className="font-mono">{course.code}</span> • TZ: <span className="font-mono">{zone}</span>
              </div>
            </div>

            {createTab === "session" ? (
              <div className="space-y-6">
                <div className="bg-gray-50 rounded-sm p-3 space-y-3">
                  <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Course & Teacher</div>
                  <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                    <div>
                      <label className="block text-xs text-gray-500 mb-1">Room</label>
                      <Select size="sm" value={sessionForm.room_id} onChange={(e) => setSessionForm((prev) => ({ ...prev, room_id: e.target.value }))}>
                        <option value="">[NOT SET] (Provisional)</option>
                        {rooms.map((r) => (
                          <option key={r.id} value={r.id}>
                            {r.name}
                          </option>
                        ))}
                      </Select>
                    </div>
                    <div className="md:col-span-2">
                      <label className="block text-xs text-gray-500 mb-1">Teacher</label>
                      <TypeaheadSelect
                        value={sessionForm.teacher_id}
                        onChange={(v) => setSessionForm((prev) => ({ ...prev, teacher_id: v }))}
                        options={teacherOptions}
                        placeholder="Search teacher…"
                      />
                    </div>
                  </div>
                </div>

                <div className="bg-gray-50 rounded-sm p-3 space-y-3">
                  <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Time</div>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                    <div>
                      <label className="block text-xs text-gray-500 mb-1">Start (local time)</label>
                      <input
                        type="datetime-local"
                        step={300}
                        value={sessionForm.start_local}
                        onChange={(e) => setSessionForm((prev) => ({ ...prev, start_local: e.target.value }))}
                        className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
                      />
                    </div>
                    <div>
                      <label className="block text-xs text-gray-500 mb-1">End (local time)</label>
                      <input
                        type="datetime-local"
                        step={300}
                        value={sessionForm.end_local}
                        onChange={(e) => setSessionForm((prev) => ({ ...prev, end_local: e.target.value }))}
                        className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
                      />
                    </div>
                  </div>
                </div>

                <PreflightIndicator
                  preflight={sessionPreflight}
                  coursesById={course ? new Map([[course.id, course]]) : new Map()}
                  teachersById={new Map(teachers.map((t) => [t.id, t]))}
                  roomsById={roomById}
                  requiredFields={[
                    { label: "Teacher", value: sessionForm.teacher_id },
                    { label: "Start", value: sessionForm.start_local },
                    { label: "End", value: sessionForm.end_local },
                  ]}
                />
              </div>
            ) : createTab === "paste" ? (
              <div className="space-y-4">
                <div className="bg-gray-50 rounded-sm p-3 space-y-3">
                  <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Teacher</div>
                  <TypeaheadSelect
                    value={pasteTeacherId}
                    onChange={setPasteTeacherId}
                    options={teacherOptions}
                    placeholder="Search teacher…"
                  />
                </div>

                <div className="bg-gray-50 rounded-sm p-3 space-y-3">
                  <label htmlFor="paste-schedule-rows" className="block text-xs font-semibold text-gray-500 uppercase tracking-wider">
                    Paste schedule rows
                  </label>
                  <textarea
                    id="paste-schedule-rows"
                    value={pasteText}
                    onChange={(e) => setPasteText(e.target.value)}
                    className="w-full min-h-40 px-2 py-1.5 text-sm font-mono border border-gray-300 rounded-sm focus-visible:outline-none focus:border-[var(--color-wi-primary)] focus:ring-3 focus:ring-[var(--color-wi-primary)]/15"
                    placeholder={"Date\tBegin\tEnd\tDuration\tClassroom\tConfirm\tBy\nSun 31 May 26\t13:00\t15:00\t02:00"}
                  />
                  {parsedPaste.errors.length > 0 && (
                    <div className="rounded-sm border border-red-200 bg-red-50 p-2 text-xs text-red-700">
                      {parsedPaste.errors.map((error) => (
                        <div key={`${error.rowNumber}-${error.message}`}>Row {error.rowNumber}: {error.message}</div>
                      ))}
                    </div>
                  )}
                </div>

                {parsedPaste.rows.length > 0 && (
                  <div className="border border-gray-200 rounded-sm overflow-hidden">
                    <div className="overflow-x-auto">
                      <table aria-label="Pasted schedule preview" className="w-full text-[12px]">
                        <thead className="bg-gray-50">
                          <tr className="border-b border-gray-200">
                            <th className="text-left py-2 px-2 font-semibold text-gray-700">Date</th>
                            <th className="text-left py-2 px-2 font-semibold text-gray-700">Begin</th>
                            <th className="text-left py-2 px-2 font-semibold text-gray-700">End</th>
                            <th className="text-left py-2 px-2 font-semibold text-gray-700">Duration</th>
                            <th className="text-left py-2 px-2 font-semibold text-gray-700">Classroom</th>
                          </tr>
                        </thead>
                        <tbody>
                          {parsedPaste.rows.map((row) => {
                            const matchedRoomId = row.classroom ? roomIdByPastedName.get(row.classroom.trim().toLowerCase()) : null;
                            return (
                              <tr key={row.rowNumber} className="border-b border-gray-100">
                                <td className="py-2 px-2 font-mono">{row.date}</td>
                                <td className="py-2 px-2 font-mono">{row.begin}</td>
                                <td className="py-2 px-2 font-mono">{row.end}</td>
                                <td className="py-2 px-2 font-mono">{row.duration || "—"}</td>
                                <td className="py-2 px-2">
                                  {row.classroom ? (
                                    <span className={matchedRoomId ? "text-gray-700" : "text-amber-700"}>
                                      {row.classroom}{matchedRoomId ? "" : " (not matched)"}
                                    </span>
                                  ) : (
                                    <span className="text-gray-400">[NOT SET]</span>
                                  )}
                                </td>
                              </tr>
                            );
                          })}
                        </tbody>
                      </table>
                    </div>
                  </div>
                )}
              </div>
            ) : (
              <div className="space-y-6">
                <div className="bg-gray-50 rounded-sm p-3 space-y-3">
                  <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Course & Teacher</div>
                  <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                    <div>
                      <label className="block text-xs text-gray-500 mb-1">Room</label>
                      <Select size="sm" value={seriesForm.room_id} onChange={(e) => setSeriesForm((prev) => ({ ...prev, room_id: e.target.value }))}>
                        <option value="">[NOT SET] (Provisional)</option>
                        {rooms.map((r) => (
                          <option key={r.id} value={r.id}>
                            {r.name}
                          </option>
                        ))}
                      </Select>
                    </div>
                    <div className="md:col-span-2">
                      <label className="block text-xs text-gray-500 mb-1">Teacher</label>
                      <TypeaheadSelect
                        value={seriesForm.teacher_id}
                        onChange={(v) => setSeriesForm((prev) => ({ ...prev, teacher_id: v }))}
                        options={teacherOptions}
                        placeholder="Search teacher…"
                      />
                    </div>
                  </div>
                </div>

                <div className="flex items-center gap-2 text-xs text-gray-400 mb-1">
                  <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-gray-900 text-white text-[10px] font-semibold">1</span>
                  <span>Course & Teacher</span>
                  <span className="text-gray-300 mx-1">→</span>
                  <span className="inline-flex items-center justify-center w-5 h-5 rounded-full bg-gray-200 text-gray-600 text-[10px] font-semibold">2</span>
                  <span>When & How Often</span>
                </div>

                <div className="bg-gray-50 rounded-sm p-3 space-y-3">
                  <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Schedule</div>
                  <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
                    <div className="md:col-span-2">
                      <label className="block text-xs text-gray-500 mb-1">Weekdays</label>
                      <div className="grid grid-cols-7 gap-1">
                        {["S", "M", "T", "W", "T", "F", "S"].map((label, idx) => (
                          <button
                            key={idx}
                            type="button"
                            onClick={() => {
                              setSeriesForm((prev) => {
                                const next = prev.weekdays.slice();
                                next[idx] = !next[idx];
                                return { ...prev, weekdays: next };
                              });
                            }}
                            className={`px-2 py-2 text-sm border rounded-sm ${
                              seriesForm.weekdays[idx] ? "bg-gray-900 text-white border-gray-900" : "bg-white hover:bg-gray-50 border-gray-300"
                            }`}
                          >
                            {label}
                          </button>
                        ))}
                      </div>
                    </div>
                    <div>
                      <label className="block text-xs text-gray-500 mb-1">Start time</label>
                      <input
                        type="time"
                        step={300}
                        value={seriesForm.start_local_time}
                        onChange={(e) => setSeriesForm((prev) => ({ ...prev, start_local_time: e.target.value }))}
                        className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
                      />
                    </div>
                    <div>
                      <label className="block text-xs text-gray-500 mb-1">Duration (min)</label>
                      <input
                        type="number"
                        min={5}
                        step={5}
                        value={seriesForm.duration_minutes}
                        onChange={(e) => setSeriesForm((prev) => ({ ...prev, duration_minutes: Number(e.target.value) }))}
                        className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
                      />
                    </div>
                  </div>

                  <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                    <div>
                      <label className="block text-xs text-gray-500 mb-1">Start date</label>
                      <input
                        type="date"
                        value={seriesForm.start_date}
                        onChange={(e) => setSeriesForm((prev) => ({ ...prev, start_date: e.target.value }))}
                        className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
                      />
                    </div>
                    <div>
                      <label className="block text-xs text-gray-500 mb-1">End bound</label>
                      <div className="flex items-center gap-2">
                        <label className="inline-flex items-center gap-2 text-sm text-gray-700">
                          <input type="checkbox" checked={seriesUseCount} onChange={(e) => setSeriesUseCount(e.target.checked)} />
                          Use count
                        </label>
                        <div className="text-xs text-gray-400">{seriesUseCount ? "Count" : "End date"}</div>
                      </div>
                    </div>
                    <div>
                      {seriesUseCount ? (
                        <>
                          <label className="block text-xs text-gray-500 mb-1">Count</label>
                          <input
                            type="number"
                            min={1}
                            step={1}
                            value={seriesForm.count}
                            onChange={(e) => setSeriesForm((prev) => ({ ...prev, count: Number(e.target.value) }))}
                            className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
                          />
                        </>
                      ) : (
                        <>
                          <label className="block text-xs text-gray-500 mb-1">End date</label>
                          <input
                            type="date"
                            value={seriesForm.end_date}
                            onChange={(e) => setSeriesForm((prev) => ({ ...prev, end_date: e.target.value }))}
                            className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
                          />
                        </>
                      )}
                    </div>
                  </div>
                </div>

                <PreflightIndicator
                  preflight={seriesPreflight}
                  coursesById={course ? new Map([[course.id, course]]) : new Map()}
                  teachersById={new Map(teachers.map((t) => [t.id, t]))}
                  roomsById={roomById}
                  requiredFields={[
                    { label: "Teacher", value: seriesForm.teacher_id },
                    { label: "Start time", value: seriesForm.start_local_time },
                    { label: "Duration", value: seriesForm.duration_minutes > 0 ? String(seriesForm.duration_minutes) : "" },
                    { label: "Start date", value: seriesForm.start_date },
                  ]}
                />
              </div>
            )}
          </div>
        </Modal>
      )}

      <AttendeeSection
        courseId={id!}
        isAdmin={user?.role === 'Admin'}
        crmEnabled={crmEnabled}
        crmLocked={crmLocked}
        roster={roster}
        rosterLoading={rosterLoading}
        addingWcode={addingWcode}
        adding={adding}
        onRosterChanged={onRosterChanged}
        onSetAddingWcode={setAddingWcode}
        onAddStudentByWcode={addStudentByWcode}
        onRemoveStudent={removeStudent}
      />

      <ConfirmModal
        open={confirmDelete}
        title="Archive Course"
        message="Archive (soft-delete) this course?"
        variant="danger"
        confirmLabel="Archive"
        loading={deleting}
        onConfirm={() => void onDelete()}
        onCancel={() => setConfirmDelete(false)}
      />

      <ConfirmModal
        open={!!confirmRemoveStudent}
        title="Remove Student"
        message="Remove this student from the course roster?"
        variant="danger"
        confirmLabel="Remove"
        onConfirm={() => void handleConfirmRemoveStudent()}
        onCancel={() => setConfirmRemoveStudent(null)}
      />
    </div>
  );
}
