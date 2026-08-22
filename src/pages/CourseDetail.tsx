import { useEffect, useMemo, useState, type MouseEventHandler, type ReactElement, type Ref } from "react";
import SearchableSelect from "@/components/ui/SearchableSelect";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { ArrowLeft, MoreVertical, Pencil, SlidersHorizontal } from "lucide-react";
import Modal from "../components/Modal";
import TypeaheadSelect, { type TypeaheadOption } from "../components/TypeaheadSelect";
import { ApiRequestError, apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import { useAuth } from "../hooks/useAuth";
import { usePreflight } from "@/features/scheduling/hooks/usePreflight";
import usePreflightGate from "@/features/scheduling/hooks/usePreflightGate";
import { useEditSession } from "@/features/scheduling/hooks/useEditSession";
import SessionEditorPopover, { type SessionEditorFocusField } from "@/features/scheduling/components/SessionEditorPopover";
import CreateSessionPopover from "@/features/scheduling/components/CreateSessionPopover";
import { PreflightIndicator, PreflightBadge, getSaveButtonLabel } from "@/components/PreflightIndicator";
import { formatUTCToZone, utcISOToZoneDate, zoneLocalInputToUTCISO } from "../utils/timezone";
import { AttendeeSection } from "../components/AttendeeSection";
import ScheduleSessionCard from "../components/ScheduleSessionCard";
import SeriesFormFields from "../components/SeriesFormFields";
import { validateSeriesPreflight, type SeriesPreflightForm } from "@/utils/preflight";
import { parseSchedulePaste } from "@/utils/schedulePaste";
import { isConflictDetails, formatConflictToastMessage } from "@/utils/conflictErrors";
import {
  addDays,
  addMonths,
  eachDayOfInterval,
  endOfMonth,
  endOfWeek,
  format,
  isSameDay,
  isSameMonth,
  startOfMonth,
  startOfWeek,
} from 'date-fns';
import Button from "../components/ui/Button";
import Select from "../components/ui/Select";
import FormField from "../components/ui/FormField";
import ConfirmModal from "../components/ConfirmModal";
import ImpactAcknowledgementModal from "../components/scheduleImpact/ImpactAcknowledgementModal";
import EmptyState from "../components/ui/EmptyState";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import { DropdownMenu } from "../components/ui/DropdownMenu";
import { Popover } from "../components/ui/Popover";
import { LegacyLinkButton } from "@/features/courses/components/LegacyLinkButton";
import { CourseTitle } from "@/features/courses/components/CourseTitle";
import { CoursePropertiesPanel } from "@/features/courses/components/CoursePropertiesPanel";
import { CourseInfoStrip } from "@/features/courses/components/CourseInfoStrip";
import { sumSessionMinutes } from "@/features/courses/domain/sessionUsage";
import {
  addCourseStudent,
  deleteCourse,
  getCourse,
  getCourseCrmFilter,
  getCourseSessions,
  getCourseCycles,
  getCourseStudents,
  getInstituteTimeMeta,
  getRooms,
  getStudentByWcode,
  getTeacherUsers,
  patchCourse,
  removeCourseStudent,
} from "@/features/courses/api/courseApi";
import {
  yyyyMmDd,
  minutesBetween,
  fmtDuration,
  type Session,
  type Course,
  type Room,
  type User,
  type Student,
  type ConflictDetails,
  type CourseEditChanges,
  type LegacyCourseConflict,
} from "@/types";
import { getCourseLegacyConflicts } from "@/features/courses/api/courseApi";
import { LegacyConflictsBanner } from "@/features/courses/components/LegacyConflictsBanner";
import { SessionConflictPopover } from "@/features/courses/components/SessionConflictPopover";
import { getCoursesReturnPath } from "@/features/courses/navigation";

export default function CourseDetail() {
  const { id } = useParams<{ id: string }>();
  const location = useLocation();
  const navigate = useNavigate();
  const coursesReturnPath = getCoursesReturnPath(location.state);
  const { addToast } = useToast();
  const { user } = useAuth();

  const [confirmDelete, setConfirmDelete] = useState(false);
  const [confirmRemoveStudent, setConfirmRemoveStudent] = useState<string | null>(null);
  const [crmEnabled, setCrmEnabled] = useState(false);
  const [crmLocked, setCrmLocked] = useState(false);

  const loadCrmFilter = async () => {
    if (!id) return;
    try {
      const res = await getCourseCrmFilter(id);
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
  const [loadError, setLoadError] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [roster, setRoster] = useState<Student[]>([]);
  const [rosterLoading, setRosterLoading] = useState(false);
  const [addingWcode, setAddingWcode] = useState("");
  const [adding, setAdding] = useState(false);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [rooms, setRooms] = useState<Room[]>([]);
  const [teachers, setTeachers] = useState<User[]>([]);
  const [cycles, setCycles] = useState<Awaited<ReturnType<typeof getCourseCycles>>>([]);
  /** Name of the field being saved, or null. Locks every property row so two
   *  edits cannot race the optimistic-concurrency version bump. */
  const [savingField, setSavingField] = useState<string | null>(null);
  const [instituteTZ, setInstituteTZ] = useState<string | null>(null);
  const [serverNow, setServerNow] = useState<string | null>(null);
  const today = useMemo(() => new Date(), []);
  const todayStr = useMemo(() => yyyyMmDd(today), [today]);
  const zone = instituteTZ ?? "Asia/Bangkok";
  const [impactedSessionIDs, setImpactedSessionIDs] = useState<Set<string>>(() => new Set());
  useEffect(() => {
    let active = true;
    if (sessions.length === 0) {
      setImpactedSessionIDs(new Set());
      return () => { active = false; };
    }
    void Promise.resolve().then(() => apiJson<{ sessions: Record<string, { open_count: number }> }>("/api/v1/operations/schedule-issues/summary", {
      method: "POST",
      body: JSON.stringify({ session_ids: sessions.map((session) => session.id) }),
    })).then((result) => {
      if (active) setImpactedSessionIDs(new Set(Object.entries(result.sessions).filter(([, value]) => value.open_count > 0).map(([sessionID]) => sessionID)));
    }).catch(() => {
      if (active) setImpactedSessionIDs(new Set());
    });
    return () => { active = false; };
  }, [sessions]);

  const [viewMode, setViewMode] = useState<'table' | 'calendar'>('table');
  /** Calendar sub-view (day / week / month), behaving like a normal calendar. */
  const [calendarMode, setCalendarMode] = useState<'day' | 'week' | 'month'>('week');
  /** Anchor date of the active calendar sub-view. */
  const [anchorDate, setAnchorDate] = useState<Date>(() => new Date());

  // Legacy sync conflicts for the current course.
  const [conflicts, setConflicts] = useState<LegacyCourseConflict[]>([]);

  /** Institute "today" from server time; falls back to the device clock. */
  const todayDate = useMemo(() => {
    const d = serverNow ? new Date(serverNow) : new Date();
    return Number.isNaN(d.getTime()) ? new Date() : d;
  }, [serverNow]);

  // Anchor the calendar to the institute's current date once known, so the
  // day/week/month views show the institute's period, not the operator's clock.
  useEffect(() => {
    if (!serverNow) return;
    const d = new Date(serverNow);
    if (!Number.isNaN(d.getTime())) setAnchorDate(d);
  }, [serverNow]);

  const teacherById = useMemo(() => new Map(teachers.map((t) => [t.id, t.full_name || t.username])), [teachers]);
  const teachersByIdMap = useMemo(() => new Map(teachers.map((t) => [t.id, t])), [teachers]);
  const roomById = useMemo(() => new Map(rooms.map((r) => [r.id, r])), [rooms]);
  const usedMinutes = useMemo(() => sumSessionMinutes(sessions), [sessions]);
  // Single shared map instead of a fresh allocation per rendered row.
  const coursesById = useMemo(() => (course ? new Map([[course.id, course]]) : new Map<string, Course>()), [course]);

  /** Sessions grouped by institute-local calendar day (yyyy-MM-dd), sorted by start. */
  const sessionsByDay = useMemo(() => {
    const map = new Map<string, Session[]>();
    for (const s of sessions) {
      const day = utcISOToZoneDate(s.start_at, zone);
      if (!day) continue;
      const list = map.get(day);
      if (list) {
        list.push(s);
      } else {
        map.set(day, [s]);
      }
    }
    for (const list of map.values()) {
      list.sort((a, b) => a.start_at.localeCompare(b.start_at));
    }
    return map;
  }, [sessions, zone]);

  /** Days to render for the active calendar sub-view, with the toolbar label. */
  const calendarRange = useMemo(() => {
    if (calendarMode === "day") {
      return { label: format(anchorDate, "EEEE, MMM d, yyyy"), days: [anchorDate] };
    }
    if (calendarMode === "week") {
      const start = startOfWeek(anchorDate, { weekStartsOn: 1 });
      const end = endOfWeek(anchorDate, { weekStartsOn: 1 });
      return {
        label: `${format(start, "MMM d")} – ${format(end, "MMM d, yyyy")}`,
        days: eachDayOfInterval({ start, end }),
      };
    }
    const start = startOfMonth(anchorDate);
    const end = endOfMonth(anchorDate);
    return {
      label: format(start, "MMMM yyyy"),
      days: eachDayOfInterval({ start: startOfWeek(start, { weekStartsOn: 1 }), end: endOfWeek(end, { weekStartsOn: 1 }) }),
    };
  }, [anchorDate, calendarMode]);

  const shiftCalendar = (direction: -1 | 1) => {
    setAnchorDate((prev) => {
      if (calendarMode === "day") return addDays(prev, direction);
      if (calendarMode === "week") return addDays(prev, direction * 7);
      return addMonths(prev, direction);
    });
  };

  const roomNameById = useMemo(() => {
    const m = new Map<string, string>();
    for (const r of rooms) m.set(r.id, r.name);
    return m;
  }, [rooms]);
  const teacherOptions = useMemo(() => teachers.map((t) => ({ value: t.id, label: t.full_name || t.username, keywords: t.id })), [teachers]);

  /** Which field of the session editor receives focus when it opens. */
  const [editFocus, setEditFocus] = useState<SessionEditorFocusField>("date");
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [bulkEditOpen, setBulkEditOpen] = useState(false);
  const [confirmDeleteSession, setConfirmDeleteSession] = useState<Session | null>(null);
  const [deletingSessionId, setDeletingSessionId] = useState<string | null>(null);

  const handleConfirmDeleteSession = async () => {
    const session = confirmDeleteSession;
    if (!session) return;

    try {
      setDeletingSessionId(session.id);
      await apiJson(`/api/v1/sessions/${session.id}`, {
        method: "DELETE",
        body: JSON.stringify({ expected_version: session.version }),
      });
      addToast("success", "Session permanently deleted");
      setConfirmDeleteSession(null);
      setSelectedIds((prev) => {
        const next = new Set(prev);
        next.delete(session.id);
        return next;
      });
      await loadSessions();
    } catch (err) {
      if (err instanceof ApiRequestError) {
        if (err.code === "stale_edit") {
          addToast("error", "Stale edit: reloaded latest session. Please try again.");
          await loadSessions();
          return;
        }
        addToast("error", `${err.code}: ${err.message}`);
        return;
      }
      addToast("error", err instanceof Error ? err.message : "Delete failed");
    } finally {
      setDeletingSessionId(null);
    }
  };

  const loadCourse = async () => {
    if (!id) return;
    try {
      setLoading(true);
      setLoadError(null);
      const c = await getCourse(id);
      setCourse(c);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "Failed to load course");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadCourse();
    void loadCrmFilter();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  // Fetch legacy sync conflicts when the course loads and has a legacy_course_id.
  useEffect(() => {
    if (!course?.legacy_course_id) {
      setConflicts([]);
      return;
    }
    let cancelled = false;
    getCourseLegacyConflicts(course.id)
      .then((res) => {
        if (!cancelled) setConflicts(res.open_conflicts);
      })
      .catch(() => {
        // Swallow: banner just doesn't show.
      });
    return () => { cancelled = true; };
  }, [course?.id, course?.legacy_course_id]);

  const toggleSelect = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (selectedIds.size === sessions.length) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(sessions.map((s) => s.id)));
    }
  };

  const loadLookups = async () => {
    try {
      const [r, t] = await Promise.all([
        getRooms(),
        getTeacherUsers(),
      ]);
      setRooms(r);
      setTeachers(t);
      try {
        setCycles(await getCourseCycles());
      } catch {
        setCycles([]);
      }
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load lookups");
    }
  };

  const loadRoster = async () => {
    if (!id) return;
    try {
      setRosterLoading(true);
      const st = await getCourseStudents(id);
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
      const items = await getCourseSessions(id);
      setSessions(items);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load sessions");
    } finally {
      setSessionsLoading(false);
    }
  };

  /** Session editing: form + debounced availability preflight + save live in
   *  the shared hook, the popover just presents them. */
  const edit = useEditSession(loadSessions, addToast, zone);

  const openEditSession = (s: Session, field: SessionEditorFocusField = "date") => {
    setEditFocus(field);
    if (edit.session?.id !== s.id) edit.openModal(s);
  };
  const closeEditSession = () => edit.closeModal();

  // If the session being edited disappears (deleted elsewhere or reloaded),
  // close the editor rather than leave it pointing at a ghost.
  useEffect(() => {
    if (!edit.open || !edit.session) return;
    if (sessions.length > 0 && !sessions.some((s) => s.id === edit.session!.id)) edit.closeModal();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [edit.open, edit.session?.id, sessions]);

  const cellValueClass =
    "min-h-6 cursor-pointer rounded-sm text-start text-[13px] text-[var(--color-wi-text)] transition-colors duration-150 hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] motion-reduce:transition-none";

  const renderSessionEditor = (
    trigger: ReactElement<{ ref?: Ref<HTMLElement>; onClick?: MouseEventHandler }>,
    s: Session,
    field: SessionEditorFocusField,
  ) => (
    <SessionEditorPopover
      open={edit.open && edit.session?.id === s.id}
      onOpenChange={(next) => { if (next) openEditSession(s, field); else closeEditSession(); }}
      trigger={trigger}
      focusField={editFocus}
      form={edit.form}
      setForm={edit.setForm}
      preflight={edit.preflight}
      canSave={edit.gate.canSave}
      saving={edit.saving}
      rooms={rooms}
      teacherOptions={teacherOptions}
      course={course}
      coursesById={coursesById}
      teachersById={teachersByIdMap}
      roomsById={roomById}
      onSave={() => void edit.submit()}
    />
  );

  const renderCreatePopover = (
    trigger: ReactElement<{ ref?: Ref<HTMLElement>; onClick?: MouseEventHandler }>,
  ) => (
    <CreateSessionPopover
      open={createPopoverOpen}
      onOpenChange={setCreatePopoverOpen}
      trigger={trigger}
      form={sessionForm}
      setForm={setSessionForm}
      preflight={sessionPreflight}
      canSave={sessionGate.canSave}
      saving={creatingSession}
      rooms={rooms}
      teacherOptions={teacherOptions}
      course={course}
      coursesById={coursesById}
      teachersById={teachersByIdMap}
      roomsById={roomById}
      onCreate={() => void submitSession()}
      onOpenSeries={() => { setCreatePopoverOpen(false); openCreate("series"); }}
      onOpenPaste={() => { setCreatePopoverOpen(false); openCreate("paste"); }}
    />
  );

  /** All sessions of one day in a single cell — time is shown per session, not
   *  by row position — so Day/Week present days the same way month cells do. */
  const renderDaySessions = (daySessions: Session[], showEmptyLabel: boolean) => {
    if (sessionsLoading && sessions.length === 0) {
      return (
        <div className="animate-pulse space-y-1.5 p-1">
          <div className="h-12 bg-[var(--color-wi-row-alt)] rounded-sm" />
          <div className="h-12 bg-[var(--color-wi-row-alt)] rounded-sm w-3/4" />
        </div>
      );
    }
    if (daySessions.length === 0) {
      return showEmptyLabel
        ? <p className="px-2 py-3 text-center text-[11px] text-[var(--color-wi-text-light)]">No sessions</p>
        : <div className="min-h-[420px]" aria-hidden="true" />;
    }
    return (
      <div className="space-y-1 p-1">
        {daySessions.map((sess) => {
          const room = roomById.get(sess.room_id ?? '');
          return (
            <div key={sess.id}>
              {renderSessionEditor(
                <button
                  type="button"
                  className="w-full cursor-pointer text-start"
                  aria-label={`Edit session ${formatUTCToZone(sess.start_at, zone, "EEE d MMM") ?? sess.start_at.slice(0, 10)}`}
                >
                  <ScheduleSessionCard
                    session={sess}
                    course={course ?? undefined}
                    room={room}
                    zone={zone}
                    teacherName={teacherById.get(sess.teacher_id)}
                  />
                </button>,
                sess,
                "start",
              )}
              <div className="mt-1 px-1">
                <SessionConflictPopover conflicts={sess.conflicts ?? []} currentCourseId={sess.course_id} zone={zone} />
              </div>
              {impactedSessionIDs.has(sess.id) ? <Link to="/operations/schedule-impact" className="mt-1 inline-block text-[11px] font-medium text-amber-700 hover:underline">Impact review open</Link> : null}
            </div>
          );
        })}
      </div>
    );
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
        const meta = await getInstituteTimeMeta();
        setInstituteTZ(meta.institute_tz);
        setServerNow(meta.server_now);
      } catch {
        // Best-effort only.
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const saveCourse = async (field: string, changes: CourseEditChanges): Promise<boolean> => {
    if (!id || !course) return false;
    if (savingField) return false;
    try {
      setSavingField(field);
      const updated = await patchCourse(id, {
        expected_version: course.version,
        code: course.code,
        name: changes.name ?? course.name,
        legacy_course_id: course.legacy_course_id ?? null,
        teachers:
          changes.teachers ??
          (course.teachers ?? []).map((t) => ({ teacher_id: t.id, is_primary: t.is_primary })),
        ...(changes.subject_id !== undefined ? { subject_id: changes.subject_id } : {}),
        ...(changes.course_type !== undefined ? { course_type: changes.course_type } : {}),
        ...(changes.year !== undefined ? { year: changes.year } : {}),
        ...(changes.hour !== undefined ? { hour: changes.hour } : {}),
        ...(changes.student_count !== undefined ? { student_count: changes.student_count } : {}),
        ...(changes.cycle_id !== undefined ? { cycle_id: changes.cycle_id } : {}),
        ...(changes.expiry_days !== undefined ? { expiry_days: changes.expiry_days } : {}),
        ...(changes.absence_form_visible !== undefined ? { absence_form_visible: changes.absence_form_visible } : {}),
      });
      setCourse(updated);
      // The visibility toggle changes what students can do right now, so its
      // confirmation states the consequence instead of a generic "updated".
      addToast(
        "success",
        field === "absence_form_visible"
          ? changes.absence_form_visible
            ? `${course.code} is now visible in the student absence form`
            : `Students can no longer select ${course.code} in the absence form`
          : "Course updated",
      );
      return true;
    } catch (err) {
      if (err instanceof ApiRequestError && err.code === "stale_edit") {
        const current = (err.details as { current?: Course } | undefined)?.current;
        if (current) {
          setCourse(current);
        } else {
          try {
            const latest = await getCourse(id);
            setCourse(latest);
          } catch {
            addToast("error", "Could not reload the latest course version. Please try again.");
            return false;
          }
        }
        addToast("error", "Another user changed this course. The latest version has been loaded.");
        return false;
      }
      if (err instanceof ApiRequestError && err.code === "teacher_in_use") {
        const details = err.details as
          | { teacher_name: string; future_session_count: number; earliest_session_start_at?: string | null }
          | undefined;
        if (details) {
          const earliest = details.earliest_session_start_at
            ? formatUTCToZone(details.earliest_session_start_at, zone, "d MMM yyyy, HH:mm") ?? details.earliest_session_start_at
            : null;
          addToast(
            "error",
            `${details.teacher_name} cannot be removed. They are assigned to ${details.future_session_count} future session${details.future_session_count === 1 ? "" : "s"}.` +
              `${earliest ? ` Earliest affected session: ${earliest}.` : ""} Review or reassign those sessions before removing this teacher.`,
          );
          return false;
        }
      }
      addToast("error", err instanceof Error ? err.message : "Update failed");
      return false;
    } finally {
      setSavingField(null);
    }
  };

  const onDelete = async () => {
    if (!id) return;
    try {
      setDeleting(true);
      await deleteCourse(id);
      addToast("success", "Course deleted");
      navigate("/courses");
    } catch (err) {
      // Keep the confirm modal open so the failure is recoverable in place.
      addToast("error", err instanceof Error ? err.message : "Delete failed");
    } finally {
      setDeleting(false);
    }
  };

  const removeStudent = async (studentId: string): Promise<void> => {
    setConfirmRemoveStudent(studentId);
  };

  const handleConfirmRemoveStudent = async () => {
    if (!id || !confirmRemoveStudent) return;
    try {
      await removeCourseStudent(id, confirmRemoveStudent);
      addToast("success", "Removed student");
      setConfirmRemoveStudent(null);
      await loadRoster();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Remove failed");
    }
  };

  const addStudentByWcode = async () => {
    if (!id) return { ok: false as const, error: "Missing course" };
    const w = addingWcode.trim();
    if (!w) return { ok: false as const, error: "Enter a W-code" };
    try {
      setAdding(true);
      // Find student by wcode via existing student lookup endpoint.
      const st = await getStudentByWcode(w);
      await addCourseStudent(id, st.id);
      addToast("success", "Added student");
      setAddingWcode("");
      await loadRoster();
      return { ok: true as const };
    } catch (err) {
      const message = formatConflictToastMessage(err, "Add failed");
      addToast("error", message);
      return { ok: false as const, error: message };
    } finally {
      setAdding(false);
    }
  };

  type PasteRowPreflight = {
    rowNumber: number;
    date: string;
    begin: string;
    end: string;
    duration: string;
    classroom: string;
    status: "available" | "provisional" | "blocked";
    conflict?: ConflictDetails;
    startISO: string | null;
    endISO: string | null;
    roomID: string | null;
  };

  const [createOpen, setCreateOpen] = useState(false);
  const [createPopoverOpen, setCreatePopoverOpen] = useState(false);
  const [createTab, setCreateTab] = useState<"series" | "paste">("series");

  const [creatingSession, setCreatingSession] = useState(false);
  const [sessionForm, setSessionForm] = useState({
    course_id: "",
    room_id: "" as string, // "" => no room (send null)
    teacher_id: "",
    start_local: "",
    end_local: "",
  });
  const sessionPreflight = usePreflight();
  const sessionGate = usePreflightGate(sessionPreflight, {
    requiredFields: [sessionForm.course_id, sessionForm.teacher_id, sessionForm.start_local, sessionForm.end_local],
  });
  const [pasteText, setPasteText] = useState("");
  const [pasteTeacherId, setPasteTeacherId] = useState("");
  const [creatingPaste, setCreatingPaste] = useState(false);
  const [pastePreflights, setPastePreflights] = useState<PasteRowPreflight[] | null>(null);
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
  const seriesValidatedForm = useMemo(
    () => validateSeriesPreflight(
      {
        ...seriesForm,
        course_id: id ?? "",
        room_id: seriesForm.room_id,
      } as SeriesPreflightForm,
      seriesUseCount,
    ),
    [id, seriesForm, seriesUseCount]
  );
  const seriesGate = usePreflightGate(seriesPreflight, {
    requiredFields: [
      id ?? "",
      seriesForm.teacher_id,
      seriesForm.start_local_time,
      seriesForm.duration_minutes > 0 ? String(seriesForm.duration_minutes) : "",
      seriesForm.start_date,
      seriesForm.weekdays.some(Boolean) ? "1" : "",
      seriesUseCount ? (Number.isFinite(seriesForm.count) && seriesForm.count > 0 ? String(seriesForm.count) : "") : seriesForm.end_date,
    ],
    isFormValid: seriesValidatedForm != null,
  });

  const openCreate = (tab: "series" | "paste" = "series") => {
    setCreateOpen(true);
    setCreateTab(tab);
    setPasteTeacherId(teachers[0]?.id ?? "");
    setPasteText("");
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

  const openCreatePopover = () => {
    edit.closeModal();
    setCreatePopoverOpen(true);
    setSessionForm({
      course_id: id ?? "",
      room_id: "",
      teacher_id: teachers[0]?.id ?? "",
      start_local: "",
      end_local: "",
    });
    sessionPreflight.reset();
  };

  const runSessionPreflight = async () => {
    if (!createPopoverOpen) { sessionPreflight.reset(); return; }
    if (!sessionForm.course_id || !sessionForm.teacher_id || !sessionForm.start_local || !sessionForm.end_local) {
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
      course_id: sessionForm.course_id,
      room_id: sessionForm.room_id ? sessionForm.room_id : null,
      teacher_id: sessionForm.teacher_id,
      start_at: startISO,
      end_at: endISO,
    });
  };

  useEffect(() => {
    void runSessionPreflight();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [createPopoverOpen, id, zone, sessionForm.room_id, sessionForm.teacher_id, sessionForm.start_local, sessionForm.end_local]);

  const submitSession = async () => {
    if (!sessionForm.course_id || !sessionForm.teacher_id) return;
    if (!sessionGate.canSave) {
      addToast("error", sessionGate.isChecking ? "Checking availability…" : "Preflight must pass before saving");
      return;
    }
    const startISO = zoneLocalInputToUTCISO(sessionForm.start_local, zone);
    const endISO = zoneLocalInputToUTCISO(sessionForm.end_local, zone);
    if (!startISO || !endISO || endISO <= startISO) {
      addToast("error", "Invalid start/end");
      return;
    }
    try {
      setCreatingSession(true);
      await apiJson("/api/v1/sessions", {
        method: "POST",
        body: JSON.stringify({
          course_id: sessionForm.course_id,
          room_id: sessionForm.room_id ? sessionForm.room_id : null,
          teacher_id: sessionForm.teacher_id,
          start_at: startISO,
          end_at: endISO,
        }),
      });
      addToast("success", "Session created");
      setCreatePopoverOpen(false);
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

      const rows = parsedPaste.rows.map((row) => {
        const startISO = zoneLocalInputToUTCISO(`${row.date}T${row.begin}`, zone);
        const endISO = zoneLocalInputToUTCISO(`${row.date}T${row.end}`, zone);
        const roomID = row.classroom ? roomIdByPastedName.get(row.classroom.trim().toLowerCase()) ?? null : null;
        return { ...row, startISO, endISO, roomID };
      });

      const invalidTime = rows.find((r) => !r.startISO || !r.endISO || r.endISO! <= r.startISO!);
      if (invalidTime) {
        addToast("error", `Invalid time on pasted row ${invalidTime.rowNumber}`);
        return;
      }

      const results = await Promise.allSettled(
        rows.map(async (row) => {
          const preflightRes = await apiJson<{ status: "available" | "provisional" }>("/api/v1/scheduling/preflight", {
            method: "POST",
            body: JSON.stringify({
              session_id: null,
              course_id: id,
              room_id: row.roomID,
              teacher_id: pasteTeacherId,
              start_at: row.startISO,
              end_at: row.endISO,
            }),
          });
          return { ...row, status: preflightRes.status as "available" | "provisional" };
        })
      );

      const preflights: PasteRowPreflight[] = results.map((r, i) => {
        const row = rows[i];
        if (r.status === "fulfilled") {
          return { ...row, status: r.value.status };
        }
        const err = r.reason;
        if (err instanceof ApiRequestError && isConflictDetails(err.details)) {
          return { ...row, status: "blocked" as const, conflict: err.details };
        }
        throw err;
      });

      const blocked = preflights.filter((p) => p.status === "blocked");
      if (blocked.length > 0) {
        setPastePreflights(preflights);
        return;
      }

      const created = await createSessionsFromPreflights(preflights);

      if (created > 0) {
        addToast("success", `Created ${created} sessions`);
        setCreateOpen(false);
        setPasteText("");
      }
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

  /** Creates every non-blocked row in parallel and reports partial failures.
   *  Returns how many sessions were created so callers can decide whether the
   *  flow completed cleanly. */
  const createSessionsFromPreflights = async (preflights: PasteRowPreflight[]): Promise<number> => {
    const creatable = preflights.filter((p) => p.status !== "blocked");
    const results = await Promise.allSettled(
      creatable.map((pf) =>
        apiJson("/api/v1/sessions", {
          method: "POST",
          body: JSON.stringify({
            course_id: id,
            room_id: pf.roomID,
            teacher_id: pasteTeacherId,
            start_at: pf.startISO,
            end_at: pf.endISO,
          }),
        }),
      ),
    );
    const failures = results.filter((r): r is PromiseRejectedResult => r.status === "rejected");
    if (failures.length > 0) {
      const reason = failures[0].reason;
      const message = reason instanceof ApiRequestError
        ? `${reason.code ?? "error"}: ${reason.message}`
        : reason instanceof Error ? reason.message : "Create failed";
      addToast(
        "error",
        `Created ${creatable.length - failures.length} of ${creatable.length} sessions. ${failures.length} failed — first error: ${message}`,
      );
    }
    return creatable.length - failures.length;
  };

  const createNonConflictingSessions = async () => {
    if (!pastePreflights) return;
    try {
      setCreatingPaste(true);
      const created = await createSessionsFromPreflights(pastePreflights);
      if (created > 0) {
        addToast("success", `Created ${created} session${created !== 1 ? "s" : ""}`);
        // Close on any progress so a retry can never re-create succeeded rows;
        // failed rows can be re-pasted after trimming the text.
        setPastePreflights(null);
        setPasteText("");
        setCreateOpen(false);
      }
      await loadSessions();
    } catch (err) {
      if (err instanceof ApiRequestError && err.details) {
        addToast("error", `${err.code ?? "error"}: ${err.message}`);
        return;
      }
      addToast("error", err instanceof Error ? err.message : "Create failed");
    } finally {
      setCreatingPaste(false);
    }
  };

  const runSeriesPreflight = async () => {
    if (!createOpen || createTab !== "series") { seriesPreflight.reset(); return; }
    if (!seriesValidatedForm) { seriesPreflight.reset(); return; }
    await seriesPreflight.check({
      course_id: id ?? "",
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
    if (!seriesGate.canSave) {
      addToast("error", seriesGate.isChecking ? "Checking availability…" : "Preflight must pass before saving");
      return;
    }
    const validated = seriesValidatedForm;
    if (!validated) { addToast("error", "Please complete schedule fields"); return; }
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
  if (!course) {
    if (loadError) {
      return (
        <div>
          <Link
            to={coursesReturnPath}
            className="mb-3 inline-flex items-center gap-1.5 text-[13px] font-medium text-[var(--color-wi-text-light)] transition-colors duration-150 hover:text-[var(--color-wi-text)] motion-reduce:transition-none"
          >
            <ArrowLeft size={14} aria-hidden="true" />
            Courses
          </Link>
          <div className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-700" role="alert">
            <p className="font-medium">Couldn&apos;t load this course</p>
            <p className="mt-1 text-red-600">{loadError}</p>
            <Button variant="secondary" size="sm" className="mt-3" onClick={() => void loadCourse()}>
              Retry
            </Button>
          </div>
        </div>
      );
    }
    return <EmptyState message="Course not found" />;
  }

  return (
    <div>
      <div className="mb-6">
        <Link
          to={coursesReturnPath}
          className="mb-3 inline-flex items-center gap-1.5 text-[13px] font-medium text-[var(--color-wi-text-light)] transition-colors duration-150 hover:text-[var(--color-wi-text)] motion-reduce:transition-none"
        >
          <ArrowLeft size={14} aria-hidden="true" />
          Courses
        </Link>
        <div className="flex items-start justify-between gap-3">
          <CourseTitle course={course} savingField={savingField} onSave={saveCourse} />
          <div className="flex items-center gap-1 pt-2">
            <Popover
              align="end"
              ariaLabel="Course properties"
              trigger={
                <button
                  type="button"
                  aria-label="Edit course properties"
                  title="Edit course properties"
                  className="inline-flex h-8 w-8 items-center justify-center rounded-sm text-[var(--color-wi-text-light)] transition-[background-color,color] duration-150 hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] aria-expanded:bg-[var(--color-wi-row-alt)] aria-expanded:text-[var(--color-wi-text)] focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] motion-reduce:transition-none"
                >
                  <SlidersHorizontal size={16} strokeWidth={2} aria-hidden="true" />
                </button>
              }
              contentClassName="w-[22rem] max-h-[calc(100dvh-2rem)] overflow-y-auto p-1.5 notion-scrollbar"
            >
              <CoursePropertiesPanel
                course={course}
                teacherOptions={teacherOptions}
                teacherNameById={teacherById}
                cycles={cycles}
                savingField={savingField}
                onSave={saveCourse}
              />
            </Popover>
            <DropdownMenu
              items={[
                { label: "Delete course", danger: true, onClick: () => setConfirmDelete(true) },
              ]}
            />
          </div>
        </div>
        <CourseInfoStrip
          course={course}
          teacherName={course.teachers?.length
            ? course.teachers.map((teacher) => teacherById.get(teacher.id) ?? teacher.full_name ?? teacher.username).join(", ")
            : course.primary_teacher_id ? teacherById.get(course.primary_teacher_id) : null}
          usedMinutes={usedMinutes}
        />
        {conflicts.length > 0 && <LegacyConflictsBanner conflicts={conflicts} />}
      </div>

      <div className="mb-8">
        <div className="flex flex-wrap items-end justify-between gap-x-3 gap-y-2 mb-3">
          <div className="flex items-end gap-2">
            <h2 className="text-xl font-semibold text-[var(--color-wi-text)]">Schedule</h2>
            <div className="inline-flex rounded-sm border border-wi-line overflow-hidden self-end" role="group" aria-label="View mode">
              <button
                type="button"
                onClick={() => { setViewMode('table'); edit.closeModal(); }}
                className={`px-2 py-1 text-[11px] transition-[background-color,color,transform] duration-150 active:scale-[0.96] motion-reduce:transition-none motion-reduce:transform-none focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] ${viewMode === 'table' ? 'bg-[var(--color-wi-nav)] text-white' : 'bg-white hover:bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]'}`}
                aria-pressed={viewMode === 'table'}
              >
                Table
              </button>
              <button
                type="button"
                onClick={() => { setViewMode('calendar'); edit.closeModal(); }}
                className={`px-2 py-1 text-[11px] transition-[background-color,color,transform] duration-150 active:scale-[0.96] motion-reduce:transition-none motion-reduce:transform-none focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] ${viewMode === 'calendar' ? 'bg-[var(--color-wi-nav)] text-white' : 'bg-white hover:bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]'}`}
                aria-pressed={viewMode === 'calendar'}
              >
                Calendar
              </button>
            </div>
          </div>
          <div className="flex items-end gap-2 flex-wrap">
            {user?.role === "Admin" && (
              <LegacyLinkButton course={course} onLinked={async () => {
                if (!id) return;
                const updated = await getCourse(id);
                setCourse(updated);
              }} />
            )}
            {viewMode === 'table' && (
              <>
                <div className="text-[11px] text-[var(--color-wi-text-light)] self-end pb-1">
                  TZ: {zone}
                  {serverNow ? ` • Server now: ${serverNow}` : ""}
                </div>
                <Button variant="secondary" size="md" onClick={loadSessions} loading={sessionsLoading} aria-label="Refresh schedule">Refresh</Button>
              </>
            )}
            {viewMode === 'calendar' && (
              <div className="flex flex-wrap items-center gap-1.5 self-end pb-0.5">
                <div className="inline-flex rounded-sm border border-wi-line overflow-hidden" role="group" aria-label="Calendar period">
                  {(["day", "week", "month"] as const).map((mode) => (
                    <button
                      key={mode}
                      type="button"
                      onClick={() => setCalendarMode(mode)}
                      aria-pressed={calendarMode === mode}
                      className={`px-2 py-1 text-[11px] transition-[background-color,color,transform] duration-150 active:scale-[0.96] motion-reduce:transition-none motion-reduce:transform-none focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] ${calendarMode === mode ? 'bg-[var(--color-wi-nav)] text-white' : 'bg-white hover:bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]'}`}
                    >
                      {mode.charAt(0).toUpperCase() + mode.slice(1)}
                    </button>
                  ))}
                </div>
                <Button variant="ghost" size="sm" onClick={() => shiftCalendar(-1)} aria-label={`Previous ${calendarMode}`}>
                  &lsaquo; Prev
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setAnchorDate(todayDate)} aria-label="Go to current date">
                  Today
                </Button>
                <Button variant="ghost" size="sm" onClick={() => shiftCalendar(1)} aria-label={`Next ${calendarMode}`}>
                  Next &rsaquo;
                </Button>
                <span className="text-xs text-[var(--color-wi-text-light)] ms-1 font-mono">
                  {calendarRange.label}
                </span>
              </div>
            )}
          </div>
        </div>

        {viewMode === 'calendar' ? (
          <div
            className={`border border-wi-line p-4 bg-white transition-opacity duration-150 motion-reduce:transition-none ${
              sessionsLoading && sessions.length > 0 ? "opacity-60 pointer-events-none" : ""
            }`}
            aria-busy={sessionsLoading}
          >
            <div className="hidden md:block">
              {calendarMode === 'month' ? (
                <div className="overflow-hidden rounded-sm border border-wi-line">
                  <div className="grid grid-cols-7 border-b border-b-wi-line bg-[var(--color-wi-row-alt)] text-center text-[11px] font-semibold uppercase tracking-wider text-[var(--color-wi-text-light)]">
                    {['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'].map((d) => (
                      <div key={d} className="py-1.5">{d}</div>
                    ))}
                  </div>
                  <div className="grid grid-cols-7">
                    {calendarRange.days.map((day) => {
                      const dayKey = format(day, 'yyyy-MM-dd');
                      const inMonth = isSameMonth(day, anchorDate);
                      const isToday = isSameDay(day, todayDate);
                      const daySessions = inMonth ? sessionsByDay.get(dayKey) ?? [] : [];
                      return (
                        <div
                          key={dayKey}
                          className={`min-h-[84px] border-b border-r border-r-wi-line p-1 last:border-r-0 ${isToday ? 'ring-1 ring-inset ring-[var(--color-wi-primary)]' : ''} ${!inMonth ? 'bg-[var(--color-wi-row-alt)]' : ''}`}
                        >
                          <button
                            type="button"
                            onClick={() => { setCalendarMode('day'); setAnchorDate(day); }}
                            aria-label={`Show ${format(day, 'EEEE, d MMMM yyyy')}`}
                            className={`mb-1 flex h-5 w-full cursor-pointer items-center rounded-sm text-[11px] leading-none transition-colors duration-150 hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] motion-reduce:transition-none ${isToday ? 'justify-center' : 'justify-end font-medium text-[var(--color-wi-text-light)]'}`}
                          >
                            {isToday ? (
                              <span className="flex h-5 w-5 items-center justify-center rounded-full bg-[var(--color-wi-primary)] font-bold text-white">{day.getDate()}</span>
                            ) : (
                              day.getDate()
                            )}
                          </button>
                          <div className="space-y-0.5">
                            {daySessions.slice(0, 3).map((sess) => {
                              const room = roomById.get(sess.room_id ?? '');
                              const startTime = formatUTCToZone(sess.start_at, zone, 'HH:mm') ?? sess.start_at.slice(11, 16);
                              return (
                                <div key={sess.id}>
                                  {renderSessionEditor(
                                    <button
                                      type="button"
                                      className="w-full truncate rounded-sm bg-[color-mix(in_oklab,var(--color-wi-primary)_10%,transparent)] px-1 py-0.5 text-start text-[10px] text-[var(--color-wi-primary-dark)] hover:bg-[var(--color-wi-selected)] focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)]"
                                      aria-label={`Edit session ${startTime}${room ? ` ${room.name}` : ''}`}
                                    >
                                      <span className="font-mono font-semibold">{startTime}</span> {room?.name ?? 'No room'}
                                    </button>,
                                    sess,
                                    "start",
                                  )}
                                  <div className="mt-0.5">
                                    <SessionConflictPopover conflicts={sess.conflicts ?? []} currentCourseId={sess.course_id} zone={zone} />
                                  </div>
                                </div>
                              );
                            })}
                            {daySessions.length > 3 ? (
                              <button
                                type="button"
                                onClick={() => { setCalendarMode('day'); setAnchorDate(day); }}
                                aria-label={`Show all sessions on ${format(day, 'EEEE, d MMMM yyyy')}`}
                                className="w-full cursor-pointer rounded-sm px-1 text-start text-[10px] text-[var(--color-wi-text-light)] transition-colors duration-150 hover:text-[var(--color-wi-text)] focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] motion-reduce:transition-none"
                              >
                                +{daySessions.length - 3} more
                              </button>
                            ) : null}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              ) : (
                <table className="w-full table-fixed text-[12px] border border-wi-line">
                  <caption className="sr-only">Calendar view</caption>
                  <thead>
                    <tr className="bg-[var(--color-wi-row-alt)]">
                      {calendarRange.days.map((day) => {
                        const isToday = isSameDay(day, todayDate);
                        return (
                          <th key={format(day, 'yyyy-MM-dd')} scope="col" className={`py-1 px-1 text-center font-semibold border-r border-r-wi-line last:border-r-0 ${isToday ? 'text-[var(--color-wi-primary-dark)]' : 'text-[var(--color-wi-text-light)]'}`}>
                            <div className="text-[10px] uppercase tracking-wider">{format(day, 'EEE')}</div>
                            <div className="text-[11px]">{format(day, 'd MMM')}</div>
                          </th>
                        );
                      })}
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      {calendarRange.days.map((day) => (
                        <td key={format(day, 'yyyy-MM-dd')} className={`border-r border-r-wi-line align-top last:border-r-0 ${isSameDay(day, todayDate) ? 'bg-[var(--color-wi-blue-bg)]' : ''}`}>
                          <div className="min-h-[420px]">
                            {renderDaySessions(sessionsByDay.get(format(day, 'yyyy-MM-dd')) ?? [], calendarMode === 'day')}
                          </div>
                        </td>
                      ))}
                    </tr>
                  </tbody>
                </table>
              )}
            </div>
            <div className="md:hidden text-center py-8 text-[var(--color-wi-text-light)] text-sm">
              <p>Calendar view is best on larger screens.</p>
              <p className="mt-1">Switch to Table view for mobile.</p>
            </div>
          </div>
        ) : (
          <div
            className={`border border-wi-line rounded-sm overflow-hidden transition-opacity duration-150 motion-reduce:transition-none ${
              sessionsLoading && sessions.length > 0 ? "opacity-60 pointer-events-none" : ""
            }`}
            aria-busy={sessionsLoading}
          >
          <div className="overflow-x-auto"><table className="w-full text-[13px]">
            <caption className="sr-only">Course schedule</caption>
            <thead className="bg-[var(--color-wi-row-alt)]">
              <tr className="border-b border-b-wi-line">
                <th scope="col" className="w-12 py-2 px-3 text-center">
                  <input
                    type="checkbox"
                    checked={selectedIds.size === sessions.length && sessions.length > 0}
                    ref={(el) => { if (el) el.indeterminate = selectedIds.size > 0 && selectedIds.size < sessions.length; }}
                    onChange={toggleSelectAll}
                    className="accent-gray-900"
                  />
                </th>
                <th scope="col" className="text-start py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">Date</th>
                <th scope="col" className="text-start py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">Begin</th>
                <th scope="col" className="text-start py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">End</th>
                <th scope="col" className="text-start py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">Duration</th>
                <th scope="col" className="text-start py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">Classroom</th>
                <th scope="col" className="text-start py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">By</th>
                <th scope="col" className="text-start py-2 px-3 font-semibold text-[var(--color-wi-text-light)]">Conflict</th>
                <th scope="col" className="w-14 py-2 px-2" aria-label="Row actions" />
              </tr>
            </thead>
            <tbody>
              {sessionsLoading && sessions.length === 0 ? (
                <tr>
                  <td colSpan={9}>
                    <LoadingSkeleton type="table" lines={3} />
                  </td>
                </tr>
              ) : sessions.length === 0 ? (
                <tr>
                  <td colSpan={9}>
                    <EmptyState message="No sessions in range" />
                  </td>
                </tr>
              ) : (
                sessions.map((s) => {
                  const mins = minutesBetween(s.start_at, s.end_at);
                  const dateLabel = formatUTCToZone(s.start_at, zone, "EEE d MMM yy") ?? s.start_at.slice(0, 10);
                  const begin = formatUTCToZone(s.start_at, zone, "HH:mm") ?? s.start_at.slice(11, 16);
                  const end = formatUTCToZone(s.end_at, zone, "HH:mm") ?? s.end_at.slice(11, 16);
                  const isEditing = edit.open && edit.session?.id === s.id;
                  // Actions stay revealed for selected/edited rows and on
                  // pointer-coarse viewports where hover cannot reveal them.
                  const rowActionsVisible = selectedIds.has(s.id) || isEditing;
                  return (
                    <tr
                      key={s.id}
                      className={`group border-b border-b-wi-line hover:bg-[var(--color-wi-row-alt)] ${selectedIds.has(s.id) ? "bg-[var(--color-wi-selected)]/50" : ""} ${isEditing ? "bg-[var(--color-wi-selected)]/40" : ""}`}
                    >
                      <td className="w-10 py-2 px-1 text-center">
                        <input
                          type="checkbox"
                          checked={selectedIds.has(s.id)}
                          onChange={() => toggleSelect(s.id)}
                          className="accent-gray-900"
                        />
                      </td>
                      <td className="py-2 px-3">
                        {renderSessionEditor(
                          <button
                            type="button"
                            onClick={() => openEditSession(s, "date")}
                            aria-label={`Edit session ${dateLabel}`}
                            className={`${cellValueClass} font-medium`}
                          >
                            {dateLabel}
                          </button>,
                          s,
                          "date",
                        )}
                        {impactedSessionIDs.has(s.id) ? <Link to="/operations/schedule-impact" className="ms-2 text-[11px] font-medium text-amber-700 hover:underline">Impact open</Link> : null}
                      </td>
                      <td className="py-2 px-3">
                        <button
                          type="button"
                          onClick={() => openEditSession(s, "start")}
                          className={`${cellValueClass} font-mono text-xs text-[var(--color-wi-text-light)]`}
                        >
                          {begin}
                        </button>
                      </td>
                      <td className="py-2 px-3">
                        <button
                          type="button"
                          onClick={() => openEditSession(s, "end")}
                          className={`${cellValueClass} font-mono text-xs text-[var(--color-wi-text-light)]`}
                        >
                          {end}
                        </button>
                      </td>
                      <td className="py-2 px-3 font-mono text-xs text-[var(--color-wi-text-light)]">{mins == null ? "—" : fmtDuration(mins)}</td>
                      <td className="py-2 px-3">
                        <button type="button" onClick={() => openEditSession(s, "room")} className={cellValueClass}>
                          {s.room_id ? (
                            <span className="inline-flex items-center rounded-sm bg-[var(--color-wi-row-alt)] px-2 py-0.5 text-xs text-[var(--color-wi-text-light)]">
                              {roomNameById.get(s.room_id) ?? "SET"}
                            </span>
                          ) : (
                            <span className="inline-flex items-center rounded-sm bg-[var(--color-wi-yellow)] px-2 py-0.5 text-xs text-white">Not set</span>
                          )}
                        </button>
                      </td>
                      <td className="py-2 px-3">
                        <button type="button" onClick={() => openEditSession(s, "teacher")} className={cellValueClass}>
                          <span className="inline-flex items-center rounded-sm border border-blue-200 bg-blue-50 px-2 py-0.5 text-xs text-blue-700">
                            {teacherById.get(s.teacher_id) ?? "—"}
                          </span>
                        </button>
                      </td>
                      <td className="py-2 px-3">
                        <SessionConflictPopover conflicts={s.conflicts ?? []} currentCourseId={s.course_id} zone={zone} />
                      </td>
                      <td className="py-2 px-2">
                        <div className={`flex items-center justify-end gap-1 max-md:opacity-100 transition-opacity duration-150 motion-reduce:transition-none ${rowActionsVisible ? "opacity-100" : "opacity-0 group-hover:opacity-100 focus-within:opacity-100"}`}>
                          <Button variant="ghost" size="sm" onClick={() => openEditSession(s, "date")} aria-label="Edit session">
                            <Pencil size={14} aria-hidden="true" />
                          </Button>
                          <DropdownMenu
                            items={[
                              { label: "Check in", onClick: () => navigate("/schedule") },
                              { label: "Delete", danger: true, onClick: () => setConfirmDeleteSession(s) },
                            ]}
                            trigger={<MoreVertical size={16} strokeWidth={2.25} aria-label="Session actions" />}
                          />
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table></div>
        </div>
        )}

{selectedIds.size > 0 && (
          <div className="flex items-center gap-3 mt-3 mb-2 animate-fade-in motion-reduce:animate-none">
            <span className="text-sm text-[var(--color-wi-text-light)] font-medium" aria-live="polite">{selectedIds.size} session{selectedIds.size !== 1 ? "s" : ""} selected</span>
            <Button variant="primary" size="sm" onClick={() => setBulkEditOpen(true)} disabled={selectedIds.size > 100}>
              Edit Selected
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setSelectedIds(new Set())}>
              Clear
            </Button>
          </div>
        )}

        {viewMode === 'table' && (
          <div className="mt-3">
            {renderCreatePopover(
              <Button variant="primary" size="md" onClick={openCreatePopover}>Add…</Button>
            )}
          </div>
        )}
      </div>

      {bulkEditOpen && (
        <BulkEditModal
          sessions={sessions.filter((s) => selectedIds.has(s.id))}
          rooms={rooms}
          teacherOptions={teacherOptions}
          zone={zone}
          onClose={() => setBulkEditOpen(false)}
          onSaved={async () => { setSelectedIds(new Set()); await loadSessions(); }}
        />
      )}

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
                  disabled={creatingSeries || !seriesGate.canSave}
                  loading={seriesPreflight.loading || creatingSeries}
                >
                  {creatingSeries ? "Creating…" : getSaveButtonLabel({ status: seriesPreflight.status, loading: seriesPreflight.loading }, "Create series", seriesPreflight.details)}
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
              <div className="inline-flex rounded-sm border border-wi-line overflow-hidden" role="tablist" aria-label="Schedule creation method">
                <button
                  type="button"
                  onClick={() => setCreateTab("series")}
                  className={`px-3 py-1.5 text-sm transition-[background-color,color,transform] duration-150 active:scale-[0.96] motion-reduce:transition-none motion-reduce:transform-none focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] ${createTab === "series" ? "bg-[var(--color-wi-nav)] text-white" : "bg-white hover:bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]"}`}
                  role="tab"
                  aria-selected={createTab === "series"}
                  aria-controls="series-panel"
                >
                  Recurring series
                </button>
                <button
                  type="button"
                  onClick={() => setCreateTab("paste")}
                  className={`px-3 py-1.5 text-sm transition-[background-color,color,transform] duration-150 active:scale-[0.96] motion-reduce:transition-none motion-reduce:transform-none focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] ${createTab === "paste" ? "bg-[var(--color-wi-nav)] text-white" : "bg-white hover:bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]"}`}
                  role="tab"
                  aria-selected={createTab === "paste"}
                  aria-controls="paste-panel"
                >
                  Paste schedule
                </button>
              </div>
              <div className="text-xs text-[var(--color-wi-text-light)]">
                Course: <span className="font-mono">{course.code}</span> • TZ: <span className="font-mono">{zone}</span>
              </div>
            </div>

            {createTab === "paste" ? (
              <div className="space-y-4" role="tabpanel" id="paste-panel" aria-labelledby="paste-tab">
                <div className="bg-[var(--color-wi-row-alt)] rounded-sm p-3 space-y-3">
                  <div className="text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wider">Teacher</div>
                  <TypeaheadSelect
                    value={pasteTeacherId}
                    onChange={setPasteTeacherId}
                    options={teacherOptions}
                    placeholder="Search teacher…"
                  />
                </div>

                <div className="bg-[var(--color-wi-row-alt)] rounded-sm p-3 space-y-3">
                  <label htmlFor="paste-schedule-rows" className="block text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wider">
                    Paste schedule rows
                  </label>
                  <textarea
                    id="paste-schedule-rows"
                    value={pasteText}
                    onChange={(e) => setPasteText(e.target.value)}
                    className="w-full min-h-40 px-2 py-1.5 text-sm font-mono border border-wi-line rounded-sm focus-visible:outline-none focus:border-[var(--color-wi-primary)] focus:ring-3 focus:ring-[var(--color-wi-primary)]/15"
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
                  <div className="border border-wi-line rounded-sm overflow-hidden">
      <div className="overflow-x-auto max-h-[50vh] overflow-y-auto">
                      <table aria-label="Pasted schedule preview" className="w-full text-[12px]">
                        <thead className="bg-[var(--color-wi-row-alt)]">
                          <tr className="border-b border-b-wi-line">
                            <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Date</th>
                            <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Begin</th>
                            <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">End</th>
                            <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Duration</th>
                            <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Classroom</th>
                          </tr>
                        </thead>
                        <tbody>
                          {parsedPaste.rows.map((row) => {
                            const matchedRoomId = row.classroom ? roomIdByPastedName.get(row.classroom.trim().toLowerCase()) : null;
                            return (
                              <tr key={row.rowNumber} className="border-b border-b-wi-line">
                                <td className="py-2 px-2 font-mono">{row.date}</td>
                                <td className="py-2 px-2 font-mono">{row.begin}</td>
                                <td className="py-2 px-2 font-mono">{row.end}</td>
                                <td className="py-2 px-2 font-mono">{row.duration || "—"}</td>
                                <td className="py-2 px-2">
                                  {row.classroom ? (
                                    <span className={matchedRoomId ? "text-[var(--color-wi-text-light)]" : "text-amber-700"}>
                                      {row.classroom}{matchedRoomId ? "" : " (not matched)"}
                                    </span>
                                  ) : (
                                    <span className="text-[var(--color-wi-text-light)]">Not set</span>
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
              <div className="space-y-6" role="tabpanel" id="series-panel" aria-labelledby="series-tab">
                <div className="bg-[var(--color-wi-row-alt)] rounded-sm p-3 space-y-3">
                  <div className="text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wider">Course & Teacher</div>
                  <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                    <FormField name="course-detail-series-room_id" label="Room">
                      <Select size="sm" value={seriesForm.room_id} onChange={(e) => setSeriesForm((prev) => ({ ...prev, room_id: e.target.value }))}>
                        <option value="">Not set (Provisional)</option>
                        {rooms.map((r) => (
                          <option key={r.id} value={r.id}>
                            {r.name}
                          </option>
                        ))}
                      </Select>
                    </FormField>
                    <FormField name="course-detail-series-teacher_id" label="Teacher" className="md:col-span-2">
                      <TypeaheadSelect
                        value={seriesForm.teacher_id}
                        onChange={(v) => setSeriesForm((prev) => ({ ...prev, teacher_id: v }))}
                        options={teacherOptions}
                        placeholder="Search teacher…"
                      />
                    </FormField>
                  </div>
                </div>
                <SeriesFormFields
                  weekdays={seriesForm.weekdays}
                  onWeekdayChange={(idx) => {
                    setSeriesForm((prev) => {
                      const next = prev.weekdays.slice();
                      next[idx] = !next[idx];
                      return { ...prev, weekdays: next };
                    });
                  }}
                  startLocalTime={seriesForm.start_local_time}
                  onStartLocalTimeChange={(v) => setSeriesForm((prev) => ({ ...prev, start_local_time: v }))}
                  durationMinutes={seriesForm.duration_minutes}
                  onDurationMinutesChange={(v) => setSeriesForm((prev) => ({ ...prev, duration_minutes: v }))}
                  useCount={seriesUseCount}
                  onUseCountChange={setSeriesUseCount}
                  count={seriesForm.count}
                  onCountChange={(v) => setSeriesForm((prev) => ({ ...prev, count: v }))}
                  endDate={seriesForm.end_date}
                  onEndDateChange={(v) => setSeriesForm((prev) => ({ ...prev, end_date: v }))}
                  startDate={seriesForm.start_date}
                  onStartDateChange={(v) => setSeriesForm((prev) => ({ ...prev, start_date: v }))}
                />
                <PreflightIndicator
                  preflight={seriesPreflight}
                  coursesById={coursesById}
                  teachersById={teachersByIdMap}
                  roomsById={roomById}
                  requiredFields={[
                    { label: "Course", value: id ?? "" },
                    { label: "Teacher", value: seriesForm.teacher_id },
                    { label: "Weekdays", value: seriesForm.weekdays.some(Boolean) ? "selected" : "" },
                    { label: "Start time", value: seriesForm.start_local_time },
                    { label: "Duration", value: seriesForm.duration_minutes > 0 ? String(seriesForm.duration_minutes) : "" },
                    { label: "Start date", value: seriesForm.start_date },
                    { label: seriesUseCount ? "Count" : "End date", value: seriesUseCount ? (Number.isFinite(seriesForm.count) && seriesForm.count > 0 ? String(seriesForm.count) : "") : seriesForm.end_date },
                  ]}
                />
              </div>
            )}
          </div>
        </Modal>
      )}

      {pastePreflights !== null && (
        <Modal
          title="Schedule Conflicts"
          onClose={() => setPastePreflights(null)}
          maxWidth="max-w-3xl"
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setPastePreflights(null)}>
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                onClick={createNonConflictingSessions}
                disabled={creatingPaste || pastePreflights.filter((p) => p.status !== "blocked").length === 0}
                loading={creatingPaste}
              >
                Add {pastePreflights.filter((p) => p.status !== "blocked").length} available session{pastePreflights.filter((p) => p.status !== "blocked").length !== 1 ? "s" : ""}
              </Button>
            </>
          }
        >
          <div className="space-y-3">
            <p className="text-sm text-[var(--color-wi-text-light)]">
              {pastePreflights.filter((p) => p.status === "blocked").length} of {pastePreflights.length} pasted session{pastePreflights.length !== 1 ? "s" : ""} {" "}
              {pastePreflights.filter((p) => p.status === "blocked").length === 1 ? "has" : "have"} scheduling conflicts.
            </p>
            <div className="border border-wi-line rounded-sm overflow-hidden">
              <div className="overflow-x-auto">
                <table className="w-full text-[12px]">
                  <caption className="sr-only">Schedule conflict preview</caption>
                  <thead className="bg-[var(--color-wi-row-alt)]">
                    <tr className="border-b border-b-wi-line">
                      <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Date</th>
                      <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Begin</th>
                      <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">End</th>
                      <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Classroom</th>
                      <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {pastePreflights.map((pf) => (
                      <tr key={pf.rowNumber} className="border-b border-b-wi-line">
                        <td className="py-2 px-2 font-mono">{pf.date}</td>
                        <td className="py-2 px-2 font-mono">{pf.begin}</td>
                        <td className="py-2 px-2 font-mono">{pf.end}</td>
                        <td className="py-2 px-2">
                          {pf.classroom ? (
                            <span>{pf.classroom}</span>
                          ) : (
                            <span className="text-[var(--color-wi-text-light)]">Not set</span>
                          )}
                        </td>
                        <td className="py-2 px-2">
                          <PreflightBadge status={pf.status} details={pf.conflict ?? null} loading={false} />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
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
        zone={zone}
      />

      <ConfirmModal
        open={confirmDelete}
        title="Delete Course"
        message="Permanently delete this course? This cannot be undone."
        variant="danger"
        confirmLabel="Delete"
        loading={deleting}
        onConfirm={() => void onDelete()}
        onCancel={() => setConfirmDelete(false)}
      />

      <ConfirmModal
        open={!!confirmDeleteSession}
        title="Delete Session"
        message="Permanently delete this session? This cannot be undone."
        variant="danger"
        confirmLabel="Delete Session"
        loading={!!deletingSessionId}
        onConfirm={() => void handleConfirmDeleteSession()}
        onCancel={() => setConfirmDeleteSession(null)}
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

      {edit.pendingImpact ? <ImpactAcknowledgementModal summary={edit.pendingImpact} saving={edit.saving} onBack={edit.dismissImpact} onConfirm={() => void edit.confirmImpact()} /> : null}
    </div>
  );
}

type BulkEditRow = {
  sessionId: string;
  version: number;
  date: string;
  begin: string;
  end: string;
  teacher_id: string;
  room_id: string;
  status: 'pending' | 'updated' | 'conflict' | 'stale_edit' | 'error';
  error?: string;
};

function BulkEditModal({
  sessions,
  rooms,
  teacherOptions,
  zone,
  onClose,
  onSaved,
}: {
  sessions: Session[];
  rooms: Room[];
  teacherOptions: TypeaheadOption[];
  zone: string;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const { addToast } = useToast();
  const [rows, setRows] = useState<BulkEditRow[]>(() =>
    sessions.map((s) => ({
      sessionId: s.id,
      version: s.version,
      date: utcISOToZoneDate(s.start_at, zone) ?? s.start_at.slice(0, 10),
      begin: formatUTCToZone(s.start_at, zone, "HH:mm") ?? s.start_at.slice(11, 16),
      end: formatUTCToZone(s.end_at, zone, "HH:mm") ?? s.end_at.slice(11, 16),
      teacher_id: s.teacher_id,
      room_id: s.room_id ?? "",
      status: 'pending' as const,
    }))
  );
  const [saving, setSaving] = useState(false);
  const [editMode, setEditMode] = useState<'per-row' | 'fill-all'>('per-row');
  const [fillValues, setFillValues] = useState<Partial<Pick<BulkEditRow, 'date' | 'begin' | 'end' | 'teacher_id' | 'room_id'>>>({});

  const updateField = (sessionId: string, field: keyof Omit<BulkEditRow, 'sessionId' | 'version' | 'status' | 'error'>, value: string) => {
    setRows((prev) =>
      prev.map((r) => (r.sessionId === sessionId ? { ...r, [field]: value, status: 'pending' as const, error: undefined } : r))
    );
  };

  const mergeRow = (r: BulkEditRow): BulkEditRow => {
    if (editMode !== 'fill-all') return r;
    return {
      ...r,
      ...(fillValues.date !== undefined ? { date: fillValues.date } : {}),
      ...(fillValues.begin !== undefined ? { begin: fillValues.begin } : {}),
      ...(fillValues.end !== undefined ? { end: fillValues.end } : {}),
      ...(fillValues.teacher_id !== undefined ? { teacher_id: fillValues.teacher_id } : {}),
      ...(fillValues.room_id !== undefined ? { room_id: fillValues.room_id } : {}),
    };
  };

  const switchMode = (mode: 'per-row' | 'fill-all') => {
    setEditMode(mode);
    setFillValues({});
    setRows((prev) => prev.map((r) => ({ ...r, status: 'pending' as const, error: undefined })));
  };

  const handleFillChange = (field: keyof typeof fillValues, value: string | undefined) => {
    setFillValues((prev) => ({ ...prev, [field]: value }));
    if (!saving) {
      setRows((prev) => prev.map((r) => ({ ...r, status: 'pending' as const, error: undefined })));
    }
  };

  const rowDuration = (r: BulkEditRow): string => {
    const startISO = zoneLocalInputToUTCISO(`${r.date}T${r.begin}`, zone);
    const endISO = zoneLocalInputToUTCISO(`${r.date}T${r.end}`, zone);
    if (!startISO || !endISO) return "—";
    const mins = minutesBetween(startISO, endISO);
    if (mins == null || mins <= 0) return "—";
    return fmtDuration(mins);
  };

  const hasDurationError = (r: BulkEditRow): boolean => {
    const startISO = zoneLocalInputToUTCISO(`${r.date}T${r.begin}`, zone);
    const endISO = zoneLocalInputToUTCISO(`${r.date}T${r.end}`, zone);
    if (!startISO || !endISO) return true;
    return endISO <= startISO;
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const baseRows = editMode === 'fill-all' ? rows.map(mergeRow) : rows;
      const updates = baseRows.map((r) => ({
        id: r.sessionId,
        expected_version: r.version,
        teacher_id: r.teacher_id,
        room_id: r.room_id || null,
        start_at: zoneLocalInputToUTCISO(`${r.date}T${r.begin}`, zone),
        end_at: zoneLocalInputToUTCISO(`${r.date}T${r.end}`, zone),
      }));

      const res = await apiJson<{ results: Array<{ id: string; status: string; error?: string; session?: any }> }>(
        '/api/v1/sessions/bulk-update',
        { method: 'POST', body: JSON.stringify({ updates }) }
      );

      const statusMap = new Map(res.results.map((r) => [r.id, r]));
      setRows((prev) =>
        prev.map((r) => {
          const result = statusMap.get(r.sessionId);
          if (!result) return { ...r, status: 'error' as const, error: 'No result returned' };
          return { ...r, status: result.status as BulkEditRow['status'], error: result.error };
        })
      );

      const updated = res.results.filter((r) => r.status === 'updated').length;
      const conflicts = res.results.filter((r) => r.status === 'conflict' || r.status === 'stale_edit').length;
      const errors = res.results.filter((r) => r.status === 'error').length;

      if (updated > 0) addToast('success', `Updated ${updated} session${updated !== 1 ? 's' : ''}`);
      if (conflicts > 0) addToast('warning', `${conflicts} session${conflicts !== 1 ? 's' : ''} had conflicts`);
      if (errors > 0) addToast('error', `${errors} session${errors !== 1 ? 's' : ''} failed`);

      if (errors === 0 && conflicts === 0 && updated > 0) {
        onClose();
        await onSaved();
      }
    } catch (err) {
      addToast('error', err instanceof Error ? err.message : 'Bulk update failed');
    } finally {
      setSaving(false);
    }
  };

  const effectiveRows = editMode === 'fill-all' ? rows.map(mergeRow) : rows;
  const canSave = effectiveRows.every((r) => !hasDurationError(r)) && !saving;

  return (
    <Modal
      title={`Bulk Edit (${sessions.length} session${sessions.length !== 1 ? 's' : ''})`}
      onClose={onClose}
      size="full"
      footer={
        <>
          <Button variant="secondary" size="sm" onClick={onClose} disabled={saving}>Cancel</Button>
          <Button variant="primary" size="sm" onClick={handleSave} disabled={!canSave} loading={saving}>
            {saving ? "Saving…" : "Save All"}
          </Button>
        </>
      }
    >
      <div className="inline-flex overflow-hidden rounded-sm border border-wi-line mb-3" role="group" aria-label="Bulk edit mode">
        <button
          type="button"
          onClick={() => switchMode('per-row')}
          aria-pressed={editMode === 'per-row'}
          className={`px-3 py-1 text-xs font-medium transition-[background-color,color,transform] duration-150 active:scale-[0.96] motion-reduce:transition-none motion-reduce:transform-none focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] ${editMode === 'per-row' ? 'bg-[var(--color-wi-nav)] text-white' : 'bg-white text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)]'}`}
        >
          Row Edit
        </button>
        <button
          type="button"
          onClick={() => switchMode('fill-all')}
          aria-pressed={editMode === 'fill-all'}
          className={`px-3 py-1 text-xs font-medium transition-[background-color,color,transform] duration-150 active:scale-[0.96] motion-reduce:transition-none motion-reduce:transform-none focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] ${editMode === 'fill-all' ? 'bg-[var(--color-wi-nav)] text-white' : 'bg-white text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)]'}`}
        >
          Apply to All
        </button>
      </div>

      {editMode === 'fill-all' && (
        <div className="border border-blue-200 rounded-sm p-3 mb-3 bg-blue-50">
          <div className="text-xs font-semibold text-blue-700 mb-2">Apply to all — only filled fields override each session</div>
          <div className="flex flex-wrap gap-3 items-end">
            <div>
              <label className="text-[10px] text-[var(--color-wi-text-light)] block mb-0.5">Date</label>
              <input type="date" value={fillValues.date ?? ''} onChange={(e) => handleFillChange('date', e.target.value || undefined)} className="w-32 px-1.5 py-1 text-xs border border-wi-line rounded-sm focus-visible:outline-none focus-visible:border-[var(--color-wi-primary)] focus-visible:ring-3 focus-visible:ring-[var(--color-wi-primary)]/15" />
            </div>
            <div>
              <label className="text-[10px] text-[var(--color-wi-text-light)] block mb-0.5">Begin</label>
              <input type="time" value={fillValues.begin ?? ''} onChange={(e) => handleFillChange('begin', e.target.value || undefined)} className="w-20 px-1.5 py-1 text-xs border border-wi-line rounded-sm focus-visible:outline-none focus-visible:border-[var(--color-wi-primary)] focus-visible:ring-3 focus-visible:ring-[var(--color-wi-primary)]/15" />
            </div>
            <div>
              <label className="text-[10px] text-[var(--color-wi-text-light)] block mb-0.5">End</label>
              <input type="time" value={fillValues.end ?? ''} onChange={(e) => handleFillChange('end', e.target.value || undefined)} className="w-20 px-1.5 py-1 text-xs border border-wi-line rounded-sm focus-visible:outline-none focus-visible:border-[var(--color-wi-primary)] focus-visible:ring-3 focus-visible:ring-[var(--color-wi-primary)]/15" />
            </div>
            <div>
              <label className="text-[10px] text-[var(--color-wi-text-light)] block mb-0.5">Classroom</label>
              <SearchableSelect value={fillValues.room_id ?? '__keep__'} onChange={(e) => handleFillChange('room_id', e.target.value === '__keep__' ? undefined : e.target.value)} className="px-1.5 py-1 text-xs border border-wi-line rounded-sm focus-visible:outline-none focus-visible:border-[var(--color-wi-primary)] focus-visible:ring-3 focus-visible:ring-[var(--color-wi-primary)]/15">
                <option value="__keep__">[KEEP ORIGINAL]</option>
                          <option value="">Not set</option>
                {rooms.map((room) => (
                  <option key={room.id} value={room.id}>{room.name}</option>
                ))}
              </SearchableSelect>
            </div>
            <div className="min-w-[160px]">
              <label className="text-[10px] text-[var(--color-wi-text-light)] block mb-0.5">Teacher</label>
              <TypeaheadSelect value={fillValues.teacher_id ?? ''} onChange={(v) => handleFillChange('teacher_id', v || undefined)} options={teacherOptions} placeholder="Set teacher for all…" />
            </div>
          </div>
        </div>
      )}

      <div className="overflow-x-auto">
        <table className="w-full text-[13px] border border-wi-line">
          <caption className="sr-only">Bulk edit sessions</caption>
          <thead className="bg-[var(--color-wi-row-alt)]">
            <tr className="border-b border-b-wi-line">
              <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">#</th>
              <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Date</th>
              <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Begin</th>
              <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">End</th>
              <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Dur</th>
              <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Classroom</th>
              <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Teacher</th>
              <th scope="col" className="text-start py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Status</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr>
                <td colSpan={8} className="py-4 text-center text-[var(--color-wi-text-light)]">No sessions selected</td>
              </tr>
            ) : (
              rows.map((r, rowIndex) => {
                const eff = editMode === 'fill-all' ? mergeRow(r) : r;
                const durStr = rowDuration(eff);
                const hasError = hasDurationError(eff);
                const isFillMode = editMode === 'fill-all';
                return (
                  <tr key={r.sessionId} className={`border-b border-b-wi-line hover:bg-[var(--color-wi-row-alt)] ${r.status === 'conflict' || r.status === 'error' || r.status === 'stale_edit' ? 'bg-red-50' : ''}`}>
                    <td className="py-1.5 px-2 font-mono text-xs text-[var(--color-wi-text-light)]">{rowIndex + 1}</td>
                    <td className="py-1.5 px-2">
                      {isFillMode ? (
                        <span className={`text-xs ${fillValues.date !== undefined ? 'bg-blue-50 px-1 -mx-1 rounded' : ''}`}>{eff.date}</span>
                      ) : (
                        <input type="date" value={r.date} onChange={(e) => updateField(r.sessionId, 'date', e.target.value)} className="w-32 px-1.5 py-1 text-xs border border-wi-line rounded-sm focus-visible:outline-none focus-visible:border-[var(--color-wi-primary)] focus-visible:ring-3 focus-visible:ring-[var(--color-wi-primary)]/15" />
                      )}
                    </td>
                    <td className="py-1.5 px-2">
                      {isFillMode ? (
                        <span className={`text-xs ${fillValues.begin !== undefined ? 'bg-blue-50 px-1 -mx-1 rounded' : ''}`}>{eff.begin}</span>
                      ) : (
                        <input type="time" value={r.begin} onChange={(e) => updateField(r.sessionId, 'begin', e.target.value)} className="w-20 px-1.5 py-1 text-xs border border-wi-line rounded-sm focus-visible:outline-none focus-visible:border-[var(--color-wi-primary)] focus-visible:ring-3 focus-visible:ring-[var(--color-wi-primary)]/15" />
                      )}
                    </td>
                    <td className="py-1.5 px-2">
                      {isFillMode ? (
                        <span className={`text-xs ${fillValues.end !== undefined ? 'bg-blue-50 px-1 -mx-1 rounded' : ''}`}>{eff.end}</span>
                      ) : (
                        <input type="time" value={r.end} onChange={(e) => updateField(r.sessionId, 'end', e.target.value)} className="w-20 px-1.5 py-1 text-xs border border-wi-line rounded-sm focus-visible:outline-none focus-visible:border-[var(--color-wi-primary)] focus-visible:ring-3 focus-visible:ring-[var(--color-wi-primary)]/15" />
                      )}
                    </td>
                    <td className={`py-1.5 px-2 font-mono text-xs ${hasError ? 'text-red-500' : 'text-[var(--color-wi-text-light)]'}`}>
                      {hasError ? "Invalid" : durStr}
                    </td>
                    <td className="py-1.5 px-2 min-w-[120px]">
                      {isFillMode ? (
                        <span className={`text-xs ${fillValues.room_id !== undefined ? 'bg-blue-50 px-1 -mx-1 rounded' : ''}`}>
                          {rooms.find((rm) => rm.id === eff.room_id)?.name ?? 'Not set'}
                        </span>
                      ) : (
                        <SearchableSelect value={r.room_id} onChange={(e) => updateField(r.sessionId, 'room_id', e.target.value)} className="w-full px-1.5 py-1 text-xs border border-wi-line rounded-sm focus-visible:outline-none focus-visible:border-[var(--color-wi-primary)] focus-visible:ring-3 focus-visible:ring-[var(--color-wi-primary)]/15">
                <option value="">Not set</option>
                          {rooms.map((room) => (
                            <option key={room.id} value={room.id}>{room.name}</option>
                          ))}
                        </SearchableSelect>
                      )}
                    </td>
                    <td className="py-1.5 px-2 min-w-[160px]">
                      {isFillMode ? (
                        <span className={`text-xs ${fillValues.teacher_id !== undefined ? 'bg-blue-50 px-1 -mx-1 rounded' : ''}`}>
                          {teacherOptions.find((teacher) => teacher.value === eff.teacher_id)?.label ?? eff.teacher_id}
                        </span>
                      ) : (
                        <TypeaheadSelect value={r.teacher_id} onChange={(v) => updateField(r.sessionId, 'teacher_id', v)} options={teacherOptions} placeholder="Search teacher…" />
                      )}
                    </td>
                    <td className="py-1.5 px-2">
                      <span className={`inline-flex items-center px-1.5 py-0.5 rounded-sm text-[11px] font-medium ${
                        r.status === 'pending' ? 'text-[var(--color-wi-text-light)]' :
                        r.status === 'updated' ? 'bg-green-100 text-green-700' :
                        r.status === 'conflict' || r.status === 'stale_edit' ? 'bg-red-100 text-red-700' :
                        'bg-red-100 text-red-700'
                      }`}>
                        {r.status === 'pending' ? 'Ready' :
                         r.status === 'updated' ? 'Saved' :
                         r.status === 'conflict' ? 'Conflict' :
                         r.status === 'stale_edit' ? 'Stale' :
                         'Error'}
                      </span>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </Modal>
  );
}
