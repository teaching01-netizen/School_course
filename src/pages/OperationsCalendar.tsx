import { useCallback, useEffect, useMemo, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Link, useSearchParams } from "react-router-dom";
import { apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import type { CalendarAbsence, CalendarAbsenceDay, CalendarResponse, CalendarSessionBrief } from "../types";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import Button from "../components/ui/Button";

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

function yyyyMmDd(d: Date): string {
  return d.toISOString().slice(0, 10);
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
  const viewMode = searchParams.get("view") === "month" ? "month" : "week";
  const [weekStart, setWeekStart] = useState(() => getMonday(new Date()));
  const [monthStart, setMonthStart] = useState(() => getMonthStart(new Date()));
  const [sessions, setSessions] = useState<CalendarSessionBrief[]>([]);
  const [absenceDays, setAbsenceDays] = useState<CalendarAbsenceDay[]>([]);
  const [loading, setLoading] = useState(true);
  const [subjectFilter, setSubjectFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [selectedDay, setSelectedDay] = useState<string | null>(null);

  const weekDates = useMemo(() => {
    return Array.from({ length: 7 }, (_, i) => addDays(weekStart, i));
  }, [weekStart]);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      let rangeStart: Date, rangeEnd: Date;
      if (viewMode === "month") {
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

  function setViewMode(mode: "week" | "month") {
    const params = new URLSearchParams(searchParams);
    if (mode === "month") params.set("view", "month");
    else params.delete("view");
    setSearchParams(params);
  }

  const filteredSessions = useMemo(() => {
    if (!subjectFilter) return sessions;
    return sessions.filter((s) => s.course_id === subjectFilter);
  }, [sessions, subjectFilter]);

  const filteredAbsenceDays = useMemo(() => {
    if (!statusFilter) return absenceDays;
    return absenceDays.map((day) => ({
      ...day,
      absences: day.absences.filter((a) => a.status === statusFilter),
    }));
  }, [absenceDays, statusFilter]);

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

  const sessionsByDay = useMemo(() => {
    const map = new Map<string, CalendarSessionBrief[]>();
    for (const session of filteredSessions) {
      const day = session.start_at.slice(0, 10);
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
          </div>
          <Button variant="secondary" size="sm" onClick={viewMode === "month" ? goTodayMonth : goToday}>
            Today
          </Button>
          <div className="flex items-center gap-1 text-sm font-medium text-gray-700">
            <button
              onClick={viewMode === "month" ? goPrevMonth : goPrevWeek}
              className="rounded-sm p-1 hover:bg-gray-100"
              aria-label="Previous"
            >
              <ChevronLeft className="h-4 w-4" />
            </button>
            <span className="min-w-[180px] text-center">
              {viewMode === "month"
                ? monthStart.toLocaleDateString("en-GB", { month: "long", year: "numeric" })
                : formatMonth(weekStart)}
            </span>
            <button
              onClick={viewMode === "month" ? goNextMonth : goNextWeek}
              className="rounded-sm p-1 hover:bg-gray-100"
              aria-label="Next"
            >
              <ChevronRight className="h-4 w-4" />
            </button>
          </div>
        </div>
      </div>

      <section className="mb-4 rounded-sm border border-gray-200 bg-white p-3">
        <div className="flex flex-wrap gap-3">
          <select aria-label="Subject" value={subjectFilter} onChange={(e) => setSubjectFilter(e.target.value)} className="text-sm">
            <option value="">All subjects</option>
            {subjects.map(([courseId, label]) => <option key={courseId} value={courseId}>{label}</option>)}
          </select>
          <select aria-label="Status" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="text-sm">
            <option value="">All statuses</option>
            <option value="pending">Pending</option>
            <option value="reviewed">Reviewed</option>
            <option value="actioned">Actioned</option>
            <option value="cancelled">Cancelled</option>
          </select>
        </div>
      </section>

      {viewMode === "month" ? (
        <div className="grid grid-cols-7 gap-px rounded-sm border border-gray-200 bg-gray-200 overflow-hidden" style={{ minHeight: "300px" }}>
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
              return (
                <div
                  key={dayStr}
                  className={`min-h-[80px] bg-white p-1 ${isToday ? "ring-2 ring-inset ring-[var(--color-wi-primary)]" : ""} ${!isCurrentMonth ? "opacity-40" : ""}`}
                >
                  <button
                    onClick={() => setSelectedDay(selectedDay === dayStr ? null : dayStr)}
                    className={`mb-1 flex h-5 w-5 items-center justify-center rounded-full text-xs font-medium ${isToday ? "bg-[var(--color-wi-primary)] text-white" : "text-gray-700 hover:bg-gray-100"}`}
                  >
                    {date.getDate()}
                  </button>
                  <div className="space-y-1">
                    {daySessions.slice(0, 2).map((s) => (
                      <div key={s.id} className="truncate rounded-sm bg-blue-50 px-1 py-0.5 text-[10px] text-blue-700">
                        {getSessionLabel(s)} {formatTime(s.start_at)}
                      </div>
                    ))}
                    {daySessions.length > 2 ? <div className="text-[10px] text-gray-400 px-1">+{daySessions.length - 2} more</div> : null}
                    {dayAbsenceRows.slice(0, 2).map((absence) => (
                    <div
                      key={absence.id}
                      className={`rounded-sm border-l-2 px-1.5 py-1 text-[10px] leading-snug ${absenceInlineClasses(absence)}`}
                    >
                      <p className="truncate font-semibold text-gray-900">{getAbsenceStudentLabel(absence)}</p>
                      <p className="truncate text-[10px] text-amber-700">
                        <span className="font-semibold">Leave:</span> {getAbsenceSubjectLabel(absence)}
                      </p>
                      <p className="truncate text-[10px] text-sky-700">
                        <span className="font-semibold">Sit-in:</span> {getSitInLabel(absence)}
                      </p>
                      </div>
                    ))}
                    {absenceCount > 2 ? <div className="text-[10px] text-gray-400 px-1">+{absenceCount - 2} more</div> : null}
                  </div>
                </div>
              );
            });
          })()}
        </div>
      ) : (
        <div
          className="grid grid-cols-7 gap-px rounded-sm border border-gray-200 bg-gray-200 overflow-hidden"
          style={{ minHeight: "400px" }}
        >
          {weekDates.map((date) => {
            const dayStr = yyyyMmDd(date);
            const dayAbsences = absencesByDay.get(dayStr);
            const daySessions = sessionsByDay.get(dayStr) ?? [];
            const dayAbsenceRows = absencesOnDate(dayAbsences);
            const absenceCount = dayAbsenceRows.length;
            const isToday = yyyyMmDd(new Date()) === dayStr;

            return (
              <div
                key={dayStr}
                className={`flex flex-col bg-white min-h-[200px] ${dayCellOddEven(date)} ${isToday ? "ring-2 ring-inset ring-[var(--color-wi-primary)]" : ""}`}
              >
                <div className={`sticky top-0 z-10 border-b border-gray-100 px-2 py-2 text-center ${isToday ? "bg-blue-50" : ""}`}>
                  <p className={`text-xs font-semibold ${isToday ? "text-[var(--color-wi-primary)]" : "text-gray-600"}`}>
                    {formatDay(date)}
                  </p>
                  <button
                    onClick={() => setSelectedDay(selectedDay === dayStr ? null : dayStr)}
                    className={`mt-1 inline-flex min-w-[28px] items-center justify-center rounded-full px-2 py-0.5 text-xs font-medium transition-colors ${absencePuckColor(absenceCount)}`}
                  >
                    {absenceCount}
                  </button>
                </div>

                <div className="flex-1 space-y-1 p-1.5 overflow-y-auto">
                  {daySessions.map((session) => (
                    <div
                      key={session.id}
                      className="rounded-sm border border-gray-100 bg-white px-2 py-1.5 text-xs shadow-sm hover:shadow-md transition-shadow cursor-default"
                    >
                      <p className="font-semibold text-gray-800">{getSessionLabel(session)}</p>
                      <p className="text-gray-500">{formatTime(session.start_at)} &ndash; {formatTime(session.end_at)}</p>
                      {session.room_name ? <p className="text-gray-400 truncate">{session.room_name}</p> : null}
                    </div>
                  ))}
                  {dayAbsenceRows.slice(0, 2).map((absence) => (
                    <Link
                      key={absence.id}
                      to={`/absences/${absence.id}`}
                      className={`block rounded-sm border-l-2 px-2 py-1.5 text-[11px] shadow-sm transition-colors hover:shadow-md ${absenceInlineClasses(absence)}`}
                    >
                      <p className="truncate font-semibold text-gray-900">{getAbsenceStudentLabel(absence)}</p>
                      <p className="truncate text-amber-700">
                        <span className="font-semibold">Leave:</span> {getAbsenceSubjectLabel(absence)}
                      </p>
                      <p className="truncate text-sky-700">
                        <span className="font-semibold">Sit-in:</span> {getSitInLabel(absence)}
                      </p>
                    </Link>
                  ))}
                  {daySessions.length === 0 && absenceCount === 0 ? (
                    <p className="px-1 py-4 text-center text-xs text-gray-300">No activity</p>
                  ) : null}
                  {absenceCount > 2 ? <p className="px-1 text-[10px] text-gray-400">+{absenceCount - 2} more absences</p> : null}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {selectedDay ? (
        <div className="mt-4 rounded-sm border border-gray-200 bg-white p-4">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="text-sm font-semibold text-gray-800">
              {new Date(selectedDay + "T00:00:00").toLocaleDateString("en-GB", { weekday: "long", day: "numeric", month: "long", year: "numeric" })}
            </h3>
            <button onClick={() => setSelectedDay(null)} className="text-xs text-gray-500 hover:text-gray-700">Close</button>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Absences ({selectedDayAbsences.length})</h4>
                  {selectedDayAbsences.length === 0 ? (
                <p className="text-sm text-gray-400">No absences this day.</p>
              ) : (
                <div className="space-y-2 max-h-64 overflow-y-auto">
                  {selectedDayAbsences.map((abs) => (
                    <Link key={abs.id} to={`/absences/${abs.id}`} className="block rounded-sm border border-gray-100 bg-gray-50 p-2 text-sm hover:bg-gray-100 transition-colors">
                      <p className="font-medium text-gray-800">{getAbsenceStudentLabel(abs)}</p>
                      <p className="text-xs text-amber-700"><span className="font-semibold">Leave:</span> {getAbsenceSubjectLabel(abs)}</p>
                      <p className="text-xs text-sky-700"><span className="font-semibold">Sit-in:</span> {getSitInLabel(abs)}</p>
                    </Link>
                  ))}
                </div>
              )}
            </div>
            <div>
              <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Sessions ({selectedDaySessions.length})</h4>
              {selectedDaySessions.length === 0 ? (
                <p className="text-sm text-gray-400">No sessions this day.</p>
              ) : (
                <div className="space-y-2 max-h-64 overflow-y-auto">
                  {selectedDaySessions.map((s) => (
                    <div key={s.id} className="rounded-sm border border-gray-100 bg-gray-50 p-2 text-sm">
                      <p className="font-medium text-gray-800">{getSessionLabel(s)}</p>
                      <p className="text-xs text-gray-500">{formatTime(s.start_at)} &ndash; {formatTime(s.end_at)}{s.room_name ? ` \u00b7 ${s.room_name}` : ""}</p>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
