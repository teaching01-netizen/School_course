import { useCallback, useEffect, useMemo, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useSearchParams } from "react-router-dom";
import { apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import type { AbsenceStatus, CalendarAbsence, CalendarAbsenceDay, CalendarResponse, CalendarSessionBrief, CalendarSitInStudent } from "../types";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import Button from "../components/ui/Button";
import EmptyState from "../components/ui/EmptyState";
import SidePanel, { type AbsencePanelTab } from "../components/absences/SidePanel";
import SitInListView from "../components/absences/SitInListView";

type CalendarShowMode = "all" | "sessions" | "absences" | "sit-ins";
type CalendarViewMode = "week" | "month" | "list";

function getMonday(d: Date): Date {
  const date = new Date(d);
  const day = date.getDay();
  const diff = day === 0 ? -6 : 1 - day;
  date.setDate(date.getDate() + diff);
  date.setHours(0, 0, 0, 0);
  return date;
}

function addDays(d: Date, n: number): Date {
  const r = new Date(d);
  r.setDate(r.getDate() + n);
  return r;
}

function formatDay(d: Date): string {
  return d.toLocaleDateString("en-GB", { weekday: "short", day: "numeric" });
}

function formatMonth(d: Date): string {
  return d.toLocaleDateString("en-GB", { month: "short", year: "numeric" });
}

function formatFullDayLabel(dayKey: string): string {
  return new Date(`${dayKey}T00:00:00`).toLocaleDateString("en-GB", {
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
  });
}

function yyyyMmDd(d: Date): string {
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function sessionDayKey(session: CalendarSessionBrief): string {
  return yyyyMmDd(new Date(session.start_at));
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
}

function getSessionLabel(session: CalendarSessionBrief): string {
  return session.subject_name?.trim() || session.course_name?.trim() || session.course_code?.trim() || "Session";
}

function getAbsenceStudentLabel(absence: CalendarAbsence): string {
  const name = absence.student_name?.trim();
  return name ? `${absence.wcode} · ${name}` : absence.wcode;
}

function getAbsenceSubjectLabel(absence: CalendarAbsence): string {
  return absence.subject_name?.trim() || absence.subject_code?.trim() || "Subject";
}

function getSitInLabel(absence: CalendarAbsence): string {
  switch (absence.sit_in_method) {
    case "zoom":
      return "Zoom";
    case "physical":
      return absence.sit_in_subject_name?.trim() || absence.sit_in_course_name?.trim() || "To arrange";
    case "teacher_case":
      return "To arrange";
    default:
      return "To arrange";
  }
}

function getSitInVisitorLabel(student: CalendarSitInStudent): string {
  const name = student.nickname?.trim() || student.student_name?.trim();
  const course = student.from_course_name?.trim() || student.from_course_code;
  return name ? `${name} (${student.wcode}) — ${course}` : `${student.wcode} — ${course}`;
}

function absencePuckColor(count: number): string {
  if (count === 0) return "bg-gray-100 text-gray-400";
  if (count <= 3) return "bg-green-100 text-green-700";
  if (count <= 6) return "bg-amber-100 text-amber-700";
  return "bg-red-100 text-red-700";
}

function absenceInlineClasses(absence: CalendarAbsence): string {
  switch (absence.sit_in_method) {
    case "physical":
      return "border-amber-200 bg-amber-50/70";
    case "zoom":
      return "border-sky-200 bg-sky-50/70";
    default:
      return "border-rose-200 bg-rose-50/70";
  }
}

function absencesOnDate(day: CalendarAbsenceDay | undefined): CalendarAbsence[] {
  return day?.absences ?? [];
}

function dayCellOddEven(date: Date): string {
  return date.getDate() % 2 === 0 ? "bg-gray-50/50" : "";
}

function getMonthStart(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), 1);
}
function getMonthEnd(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth() + 1, 0);
}
function getMonthGrid(d: Date): Date[] {
  const start = getMonthStart(d);
  const startDay = start.getDay();
  const grid: Date[] = [];
  // Pad to Monday (align with week view)
  const padStart = new Date(start);
  padStart.setDate(padStart.getDate() - (startDay === 0 ? 6 : startDay - 1));
  const end = getMonthEnd(d);
  const endDay = end.getDay();
  const padEnd = new Date(end);
  padEnd.setDate(padEnd.getDate() + (endDay === 0 ? 0 : 7 - endDay));
  const cursor = new Date(padStart);
  while (cursor <= padEnd) {
    grid.push(new Date(cursor));
    cursor.setDate(cursor.getDate() + 1);
  }
  return grid;
}

export default function OperationsCalendar() {
  const { addToast } = useToast();
  const [searchParams, setSearchParams] = useSearchParams();
  const viewParam = searchParams.get("view");
  const viewMode: CalendarViewMode = viewParam === "week" || viewParam === "month" || viewParam === "list" ? viewParam : "month";
  const showParam = searchParams.get("show");
  const showMode: CalendarShowMode =
    showParam === "all" || showParam === "sessions" || showParam === "absences" || showParam === "sit-ins"
      ? showParam
      : "sit-ins";
  const subjectParam = searchParams.get("subject") ?? "";
  const statusParam = searchParams.get("status") ?? "";
  const [weekStart, setWeekStart] = useState(() => getMonday(new Date()));
  const [monthStart, setMonthStart] = useState(() => getMonthStart(new Date()));
  const [sessions, setSessions] = useState<CalendarSessionBrief[]>([]);
  const [absenceDays, setAbsenceDays] = useState<CalendarAbsenceDay[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedDay, setSelectedDay] = useState<string | null>(null);
  const [panelTab, setPanelTab] = useState<AbsencePanelTab>("sit-ins");

  const weekDates = useMemo(() => {
    return Array.from({ length: 7 }, (_, i) => addDays(weekStart, i));
  }, [weekStart]);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      let rangeStart: Date, rangeEnd: Date;
      if (viewMode === "month" || viewMode === "list") {
        rangeStart = getMonthStart(monthStart);
        rangeEnd = getMonthEnd(monthStart);
      } else {
        rangeStart = weekStart;
        rangeEnd = addDays(weekStart, 6);
      }
      const calData = await apiJson<CalendarResponse>(
        `/api/v1/operations/calendar?start=${yyyyMmDd(rangeStart)}&end=${yyyyMmDd(rangeEnd)}`,
        { method: "GET" }
      );
      setSessions(calData.sessions);
      setAbsenceDays(calData.absence_days);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load calendar data");
    } finally {
      setLoading(false);
    }
  }, [addToast, weekStart, monthStart, viewMode]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  const goPrevWeek = () => setWeekStart((prev) => addDays(prev, -7));
  const goNextWeek = () => setWeekStart((prev) => addDays(prev, 7));
  const goToday = () => setWeekStart(getMonday(new Date()));

  const goPrevMonth = () => setMonthStart((prev) => new Date(prev.getFullYear(), prev.getMonth() - 1, 1));
  const goNextMonth = () => setMonthStart((prev) => new Date(prev.getFullYear(), prev.getMonth() + 1, 1));
  const goTodayMonth = () => setMonthStart(getMonthStart(new Date()));

  function setViewMode(mode: CalendarViewMode) {
    const params = new URLSearchParams(searchParams);
    params.set("view", mode);
    setSearchParams(params);
  }

  function setShowMode(mode: CalendarShowMode) {
    const params = new URLSearchParams(searchParams);
    params.set("show", mode);
    setSearchParams(params, { replace: true });
    setSelectedDay(null);
  }

  function setSubjectFilter(value: string) {
    const params = new URLSearchParams(searchParams);
    if (value) params.set("subject", value);
    else params.delete("subject");
    setSearchParams(params, { replace: true });
    setSelectedDay(null);
  }

  function setStatusFilter(value: string) {
    const params = new URLSearchParams(searchParams);
    if (value) params.set("status", value);
    else params.delete("status");
    setSearchParams(params, { replace: true });
    setSelectedDay(null);
  }

  function openPanel(day: string, tab: AbsencePanelTab = showMode === "absences" ? "absences" : "sit-ins") {
    setPanelTab(tab);
    setSelectedDay(day);
  }

  useEffect(() => {
    setSelectedDay(null);
  }, [viewMode, weekStart, monthStart]);

  const subjects = useMemo(() => {
    const map = new Map<string, string>();
    for (const session of sessions) {
      const label = getSessionLabel(session);
      if (!map.has(session.course_id)) {
        map.set(session.course_id, label);
      }
    }
    return [...map.entries()].sort((a, b) => a[1].localeCompare(b[1]));
  }, [sessions]);

  const validSubjectIds = useMemo(() => new Set(subjects.map(([courseId]) => courseId)), [subjects]);
  const subjectFilter = subjectParam && validSubjectIds.has(subjectParam) ? subjectParam : "";
  const statusFilter: AbsenceStatus | "" =
    statusParam === "pending" || statusParam === "reviewed" || statusParam === "actioned" || statusParam === "cancelled" ? statusParam : "";

  const filteredSessions = useMemo(() => {
    let data = sessions;

    if (showMode === "absences") {
      return [];
    }

    if (subjectFilter) {
      data = data.filter((s) => s.course_id === subjectFilter);
    }

    if (showMode === "sit-ins") {
      data = data.filter((s) => (s.sit_in_students?.length ?? 0) > 0);
    }

    return data;
  }, [sessions, subjectFilter, showMode]);

  const filteredAbsenceDays = useMemo(() => {
    if (showMode === "sessions" || showMode === "sit-ins") {
      return [];
    }

    if (!statusFilter) return absenceDays;
    return absenceDays.map((day) => ({
      ...day,
      absences: day.absences.filter((a) => a.status === statusFilter),
    }));
  }, [absenceDays, statusFilter, showMode]);

  const sessionsByDay = useMemo(() => {
    const map = new Map<string, CalendarSessionBrief[]>();
    for (const session of filteredSessions) {
      const day = sessionDayKey(session);
      if (!map.has(day)) map.set(day, []);
      map.get(day)!.push(session);
    }
    return map;
  }, [filteredSessions]);

  const absencesByDay = useMemo(() => {
    const map = new Map<string, CalendarAbsenceDay>();
    for (const day of filteredAbsenceDays) {
      map.set(day.date, day);
    }
    return map;
  }, [filteredAbsenceDays]);

  const selectedDayAbsences = useMemo(() => {
    if (!selectedDay) return [];
    return absencesByDay.get(selectedDay)?.absences ?? [];
  }, [absencesByDay, selectedDay]);

  const selectedDaySessions = useMemo(() => {
    if (!selectedDay) return [];
    return sessionsByDay.get(selectedDay) ?? [];
  }, [sessionsByDay, selectedDay]);

  const totalVisibleAbsences = useMemo(() => {
    return absenceDays.reduce((count, day) => count + day.absences.length, 0);
  }, [absenceDays]);

  const totalVisibleSitIns = useMemo(() => {
    return sessions.reduce((count, session) => count + (session.sit_in_students?.length ?? 0), 0);
  }, [sessions]);

  const visibleAbsenceCount = filteredAbsenceDays.reduce((count, day) => count + day.absences.length, 0);
  const hasVisibleCalendarActivity = filteredSessions.length > 0 || visibleAbsenceCount > 0;
  const hasAnySitIns = sessions.some((session) => (session.sit_in_students?.length ?? 0) > 0);
  const showSubjectFilter = showMode === "all" || showMode === "sessions" || showMode === "sit-ins";
  const showStatusFilter = showMode === "all" || showMode === "absences";
  const activeFilterCount = (subjectFilter ? 1 : 0) + (statusFilter ? 1 : 0);

  function clearFilters() {
    const params = new URLSearchParams(searchParams);
    params.set("show", "sit-ins");
    params.delete("subject");
    params.delete("status");
    setSearchParams(params, { replace: true });
  }

  if (loading) return <LoadingSkeleton type="table" lines={10} />;

  return (
    <div className="w-full">
      <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-[32px] font-bold text-[var(--color-wi-text)]">Calendar</h1>
          <p className="text-sm text-gray-500">Combined view of scheduled sessions and student absences.</p>
        </div>
        <div className="flex items-center gap-2">
          <div className="flex rounded-sm border border-gray-300 bg-white text-sm">
            <button onClick={() => setViewMode("week")} className={`flex items-center gap-1 px-3 py-1.5 ${viewMode === "week" ? "bg-gray-100 text-gray-900 font-medium" : "text-gray-500 hover:text-gray-900"}`}>Week</button>
            <button onClick={() => setViewMode("month")} className={`flex items-center gap-1 px-3 py-1.5 ${viewMode === "month" ? "bg-gray-100 text-gray-900 font-medium" : "text-gray-500 hover:text-gray-900"}`}>Month</button>
            <button onClick={() => setViewMode("list")} className={`flex items-center gap-1 px-3 py-1.5 ${viewMode === "list" ? "bg-gray-100 text-gray-900 font-medium" : "text-gray-500 hover:text-gray-900"}`}>List</button>
          </div>
          <Button variant="secondary" size="sm" onClick={viewMode === "week" ? goToday : goTodayMonth}>
            Today
          </Button>
          <div className="flex items-center gap-1 text-sm font-medium text-gray-700">
            <button
              onClick={viewMode === "week" ? goPrevWeek : goPrevMonth}
              className="rounded-sm p-1 hover:bg-gray-100"
              aria-label="Previous"
            >
              <ChevronLeft className="h-4 w-4" />
            </button>
            <span className="min-w-[180px] text-center">
              {viewMode === "month"
                ? monthStart.toLocaleDateString("en-GB", { month: "long", year: "numeric" })
                : viewMode === "list"
                  ? monthStart.toLocaleDateString("en-GB", { month: "long", year: "numeric" })
                : formatMonth(weekStart)}
            </span>
            <button
              onClick={viewMode === "week" ? goNextWeek : goNextMonth}
              className="rounded-sm p-1 hover:bg-gray-100"
              aria-label="Next"
            >
              <ChevronRight className="h-4 w-4" />
            </button>
          </div>
        </div>
      </div>

      <div className="mb-4 rounded-sm border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-600">
        Summary: <strong className="text-gray-900">{totalVisibleAbsences}</strong> absences |{" "}
        <strong className="text-gray-900">{totalVisibleSitIns}</strong> sit-in assignments
      </div>

      <section className="mb-4 rounded-sm border border-gray-200 bg-white p-3">
        <div className="flex flex-wrap gap-3">
          <span className="inline-flex min-h-[32px] items-center rounded-full bg-gray-100 px-3 text-xs font-semibold text-gray-600">
            Filters ({activeFilterCount})
          </span>
          <label className="flex items-center gap-2 text-sm text-gray-600">
            Show:
            <select
              aria-label="Show"
              value={showMode}
              onChange={(e) => setShowMode(e.target.value as CalendarShowMode)}
              className="text-sm"
            >
              <option value="all">All activity</option>
              <option value="sessions">Sessions only</option>
              <option value="absences">Absences only</option>
              <option value="sit-ins">Sit-ins only</option>
            </select>
          </label>
          {showSubjectFilter ? (
            <select aria-label="Subject" value={subjectFilter} onChange={(e) => setSubjectFilter(e.target.value)} className="text-sm">
              <option value="">All subjects</option>
              {subjects.map(([courseId, label]) => <option key={courseId} value={courseId}>{label}</option>)}
            </select>
          ) : null}
          {showStatusFilter ? (
            <select aria-label="Status" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="text-sm">
              <option value="">All statuses</option>
              <option value="pending">Pending</option>
              <option value="reviewed">Reviewed</option>
              <option value="actioned">Actioned</option>
              <option value="cancelled">Cancelled</option>
            </select>
          ) : null}
        </div>
      </section>

      {viewMode === "list" ? (
        <SitInListView sessions={filteredSessions} absenceDays={filteredAbsenceDays.length ? filteredAbsenceDays : absenceDays} onClearFilters={clearFilters} hasAnySitIns={hasAnySitIns} />
      ) : showMode !== "all" && !hasVisibleCalendarActivity ? (
        <div className="rounded-sm border border-gray-200 bg-white">
          <EmptyState
            message={
              showMode === "sit-ins"
                ? "No sit-in assignments match these filters."
                : showMode === "absences"
                  ? "No absences match these filters."
                  : "No sessions match these filters."
            }
          />
        </div>
      ) : viewMode === "month" ? (
        <div className="grid grid-cols-7 gap-px overflow-hidden rounded-sm border border-gray-200 bg-gray-200" style={{ minHeight: "300px" }}>
          {["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"].map((d) => (
            <div key={d} className="bg-gray-50 px-2 py-1 text-center text-xs font-semibold text-gray-500">{d}</div>
          ))}
          {(() => {
            const grid = getMonthGrid(monthStart);
            const todayStr = yyyyMmDd(new Date());
            return grid.map((date) => {
              const dayStr = yyyyMmDd(date);
              const dayAbsences = absencesByDay.get(dayStr);
              const daySessions = sessionsByDay.get(dayStr) ?? [];
              const dayAbsenceRows = absencesOnDate(dayAbsences);
              const absenceCount = dayAbsenceRows.length;
              const isToday = todayStr === dayStr;
              const isCurrentMonth = date.getMonth() === monthStart.getMonth();
              const dayLabel = formatFullDayLabel(dayStr);

              return (
                <div
                  key={dayStr}
                  className={`min-h-[80px] bg-white p-1 ${isToday ? "ring-2 ring-inset ring-[var(--color-wi-primary)]" : ""} ${!isCurrentMonth ? "opacity-40" : ""}`}
                >
                  <button
                    type="button"
                    onClick={() => openPanel(dayStr)}
                    aria-label={`Open details for ${dayLabel}`}
                    className={`mb-1 flex h-5 w-5 items-center justify-center rounded-full text-xs font-medium ${isToday ? "bg-[var(--color-wi-primary)] text-white" : "text-gray-700 hover:bg-gray-100"}`}
                  >
                    {date.getDate()}
                  </button>
                  <div className="space-y-1">
                    {daySessions.slice(0, 2).map((s) => (
                      <button
                        key={s.id}
                        type="button"
                        onClick={() => openPanel(dayStr, "sit-ins")}
                        aria-label={`Open details for ${getSessionLabel(s)} on ${dayLabel}`}
                        className="w-full rounded-sm bg-blue-50 px-1 py-0.5 text-left text-[10px] text-blue-700"
                      >
                        <div className="space-y-0.5">
                          <p className="truncate">{getSessionLabel(s)} {formatTime(s.start_at)}</p>
                          {s.sit_in_students && s.sit_in_students.length > 0 ? (
                            <p className="truncate text-[10px] text-amber-700">
                              <span className="font-semibold">Visitors:</span>{" "}
                              {s.sit_in_students.slice(0, 2).map((student, idx) => (
                                <span key={`${student.wcode}-${student.absence_id}`}>
                                  {idx > 0 && ", "}
                                  {getSitInVisitorLabel(student)}
                                </span>
                              ))}
                              {s.sit_in_students.length > 2 ? (
                                <span className="text-amber-500"> +{s.sit_in_students.length - 2} more</span>
                              ) : null}
                            </p>
                          ) : null}
                        </div>
                      </button>
                    ))}
                    {daySessions.length > 2 ? (
                      <button
                        type="button"
                        onClick={() => openPanel(dayStr, "sit-ins")}
                        aria-label={`View all session details for ${dayLabel}`}
                        className="w-full px-1 text-left text-[10px] text-gray-400 hover:text-gray-600"
                      >
                        +{daySessions.length - 2} more
                      </button>
                    ) : null}
                    {dayAbsenceRows.slice(0, 2).map((absence) => (
                      <button
                        key={absence.id}
                        type="button"
                        onClick={() => openPanel(dayStr, "absences")}
                        aria-label={`Open details for ${getAbsenceStudentLabel(absence)} on ${dayLabel}`}
                        className={`w-full rounded-sm border-l-2 px-1.5 py-1 text-left text-[10px] leading-snug ${absenceInlineClasses(absence)}`}
                      >
                        <p className="truncate font-semibold text-gray-900">{getAbsenceStudentLabel(absence)}</p>
                        <p className="truncate text-[10px] text-amber-700">
                          <span className="font-semibold">Leave:</span> {getAbsenceSubjectLabel(absence)}
                        </p>
                        <p className="truncate text-[10px] text-sky-700">
                          <span className="font-semibold">Sit-in:</span> {getSitInLabel(absence)}
                        </p>
                      </button>
                    ))}
                    {absenceCount > 2 ? (
                      <button
                        type="button"
                        onClick={() => openPanel(dayStr, "absences")}
                        aria-label={`View all absence details for ${dayLabel}`}
                        className="w-full px-1 text-left text-[10px] text-gray-400 hover:text-gray-600"
                      >
                        +{absenceCount - 2} more
                      </button>
                    ) : null}
                  </div>
                </div>
              );
            });
          })()}
        </div>
      ) : (
        <div
          className="grid grid-cols-7 gap-px overflow-hidden rounded-sm border border-gray-200 bg-gray-200"
          style={{ minHeight: "400px" }}
        >
          {weekDates.map((date) => {
            const dayStr = yyyyMmDd(date);
            const dayAbsences = absencesByDay.get(dayStr);
            const daySessions = sessionsByDay.get(dayStr) ?? [];
            const dayAbsenceRows = absencesOnDate(dayAbsences);
            const absenceCount = dayAbsenceRows.length;
            const isToday = yyyyMmDd(new Date()) === dayStr;
            const dayLabel = formatFullDayLabel(dayStr);

            return (
              <div
                key={dayStr}
                className={`flex min-h-[200px] flex-col bg-white ${dayCellOddEven(date)} ${isToday ? "ring-2 ring-inset ring-[var(--color-wi-primary)]" : ""}`}
              >
                <button
                  type="button"
                  onClick={() => openPanel(dayStr)}
                  aria-label={`Open details for ${dayLabel}`}
                  className={`sticky top-0 z-10 border-b border-gray-100 px-2 py-2 text-center ${isToday ? "bg-blue-50" : ""}`}
                >
                  <p className={`text-xs font-semibold ${isToday ? "text-[var(--color-wi-primary)]" : "text-gray-600"}`}>
                    {formatDay(date)}
                  </p>
                  <span
                    className={`mt-1 inline-flex min-w-[28px] items-center justify-center rounded-full px-2 py-0.5 text-xs font-medium transition-colors ${absencePuckColor(absenceCount)}`}
                    aria-hidden="true"
                  >
                    {absenceCount}
                  </span>
                </button>

                <div className="flex-1 space-y-1 overflow-y-auto p-1.5">
                  {daySessions.map((session) => (
                    <button
                      key={session.id}
                      type="button"
                      onClick={() => openPanel(dayStr, "sit-ins")}
                      aria-label={`Open details for ${getSessionLabel(session)} on ${dayLabel}`}
                      className="w-full rounded-sm border border-gray-100 bg-white px-2 py-1.5 text-left text-xs shadow-sm transition-shadow hover:shadow-md"
                    >
                      <p className="font-semibold text-gray-800">{getSessionLabel(session)}</p>
                      <p className="text-gray-500">{formatTime(session.start_at)} &ndash; {formatTime(session.end_at)}</p>
                      {session.room_name ? <p className="truncate text-gray-400">{session.room_name}</p> : null}
                      {session.sit_in_students && session.sit_in_students.length > 0 ? (
                        <div className="mt-1 border-t border-gray-100 pt-1">
                          <p className="text-[10px] text-amber-700">
                            <span className="font-semibold">Visitors:</span>{" "}
                            {session.sit_in_students.slice(0, 2).map((student, idx) => (
                              <span key={student.wcode}>
                                {idx > 0 && ", "}
                                {getSitInVisitorLabel(student)}
                              </span>
                            ))}
                            {session.sit_in_students.length > 2 ? (
                              <span className="text-amber-500"> +{session.sit_in_students.length - 2} more</span>
                            ) : null}
                          </p>
                        </div>
                      ) : null}
                    </button>
                  ))}
                  {dayAbsenceRows.slice(0, 2).map((absence) => (
                    <button
                      key={absence.id}
                      type="button"
                      onClick={() => openPanel(dayStr, "absences")}
                      aria-label={`Open details for ${getAbsenceStudentLabel(absence)} on ${dayLabel}`}
                      className={`block w-full rounded-sm border-l-2 px-2 py-1.5 text-left text-[11px] shadow-sm transition-colors hover:shadow-md ${absenceInlineClasses(absence)}`}
                    >
                      <p className="truncate font-semibold text-gray-900">{getAbsenceStudentLabel(absence)}</p>
                      <p className="truncate text-amber-700">
                        <span className="font-semibold">Leave:</span> {getAbsenceSubjectLabel(absence)}
                      </p>
                      <p className="truncate text-sky-700">
                        <span className="font-semibold">Sit-in:</span> {getSitInLabel(absence)}
                      </p>
                    </button>
                  ))}
                  {daySessions.length === 0 && absenceCount === 0 ? (
                    <p className="px-1 py-4 text-center text-xs text-gray-300">No activity</p>
                  ) : null}
                  {absenceCount > 2 ? (
                    <button
                      type="button"
                      onClick={() => openPanel(dayStr, "absences")}
                      aria-label={`View all absence details for ${dayLabel}`}
                      className="w-full px-1 text-left text-[10px] text-gray-400 hover:text-gray-600"
                    >
                      +{absenceCount - 2} more absences
                    </button>
                  ) : null}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {selectedDay ? (
        <SidePanel
          dayKey={selectedDay}
          sessions={selectedDaySessions}
          absences={selectedDayAbsences}
          initialTab={panelTab}
          onClose={() => setSelectedDay(null)}
        />
      ) : null}
    </div>
  );
}
