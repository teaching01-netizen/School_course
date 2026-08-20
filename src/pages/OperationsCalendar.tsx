import { useEffect, useMemo, useState } from "react";
import { DateTime } from "luxon";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useSearchParams } from "react-router-dom";
import { useToast } from "../hooks/useToast";
import type { AbsenceStatus, CalendarAbsence, CalendarAbsenceDay, CalendarResponse, CalendarSessionBrief } from "../types";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import Button from "../components/ui/Button";
import EmptyState from "../components/ui/EmptyState";
import SidePanel, { type AbsencePanelTab } from "../components/absences/SidePanel";
import SitInListView from "../components/absences/SitInListView";
import {
  formatFullDayLabel,
  getAbsenceStudentLabel,
  getAbsenceSubjectLabel,
  getSessionLabel,
  getSitInLabel,
  getSitInVisitorLabel,
  sessionDayKey,
} from "../components/absences/calendarDisplay";
import { formatUTCToZone, shiftZoneMonthKey, startOfZoneMonthKey, utcISOToZoneDate } from "../utils/timezone";
import useInstituteMeta from "../hooks/useInstituteMeta";
import { queryKeys } from "../query/cache";
import { useOperationalQuery } from "../query/useOperationalQuery";

type CalendarShowMode = "all" | "sessions" | "absences" | "sit-ins";
type CalendarViewMode = "week" | "month" | "list";

function addDaysToKey(dayKey: string, zone: string, days: number): string {
  const dt = DateTime.fromISO(dayKey, { zone });
  return dt.isValid ? dt.plus({ days }).toFormat("yyyy-MM-dd") : dayKey;
}

function mondayOf(dayKey: string, zone: string): string {
  const dt = DateTime.fromISO(dayKey, { zone });
  return dt.isValid ? dt.startOf("week").toFormat("yyyy-MM-dd") : dayKey;
}

function formatMonthLabel(dayKey: string, zone: string): string {
  const dt = DateTime.fromISO(dayKey, { zone });
  return dt.isValid ? dt.setLocale("en-GB").toFormat("MMMM yyyy") : "";
}

function absencePuckColor(count: number): string {
  if (count === 0) return "bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]";
  if (count <= 3) return "bg-[var(--color-wi-green-bg)] text-[var(--color-wi-green-dark)]";
  if (count <= 6) return "bg-[var(--color-wi-amber-bg)] text-[var(--color-wi-amber)]";
  return "bg-[var(--color-wi-danger-bg)] text-[var(--color-wi-red)]";
}

function absenceInlineClasses(absence: CalendarAbsence): string {
  switch (absence.sit_in_method) {
    case "physical":
      return "bg-[var(--color-wi-amber-bg)] text-[var(--color-wi-amber)]";
    case "zoom":
      return "bg-[var(--color-wi-blue-bg)] text-[var(--color-wi-primary-dark)]";
    default:
      return "bg-[var(--color-wi-danger-bg)] text-[var(--color-wi-red)]";
  }
}

function absencesOnDate(day: CalendarAbsenceDay | undefined): CalendarAbsence[] {
  return day?.absences ?? [];
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

  const { serverNow, instituteTZ, loaded: instituteMetaLoaded } = useInstituteMeta();
  const zone = instituteTZ ?? "Asia/Bangkok";
  const fallbackNowIso = useMemo(() => new Date().toISOString(), []);
  const todayKey = useMemo(
    () => utcISOToZoneDate(serverNow ?? fallbackNowIso, zone),
    [fallbackNowIso, serverNow, zone],
  );

  const [weekStartKey, setWeekStartKey] = useState<string | null>(null);
  const [monthStartKey, setMonthStartKey] = useState<string | null>(null);
  const [selectedDay, setSelectedDay] = useState<string | null>(null);
  const [panelTab, setPanelTab] = useState<AbsencePanelTab>("sit-ins");

  // Seed the viewed week and month from the institute-zone today once the
  // server meta has loaded, so the calendar matches the school's calendar day.
  useEffect(() => {
    if (!instituteMetaLoaded || weekStartKey !== null || monthStartKey !== null) return;
    const day = todayKey ?? utcISOToZoneDate(fallbackNowIso, zone);
    if (!day) return;
    setWeekStartKey(mondayOf(day, zone));
    setMonthStartKey(startOfZoneMonthKey(day, zone));
  }, [fallbackNowIso, instituteMetaLoaded, monthStartKey, todayKey, weekStartKey, zone]);

  const weekDates = useMemo(() => {
    if (!weekStartKey) return [];
    const start = DateTime.fromISO(weekStartKey, { zone });
    if (!start.isValid) return [];
    return Array.from({ length: 7 }, (_, i) => start.plus({ days: i }));
  }, [weekStartKey, zone]);

  const monthGrid = useMemo(() => {
    if (!monthStartKey) return [];
    const start = DateTime.fromISO(monthStartKey, { zone });
    if (!start.isValid) return [];
    const startPad = start.weekday - 1;
    const monthEnd = start.endOf("month").startOf("day");
    const endPad = monthEnd.weekday === 7 ? 0 : 7 - monthEnd.weekday;
    const gridStart = start.minus({ days: startPad });
    const gridEnd = monthEnd.plus({ days: endPad });
    const days: DateTime[] = [];
    for (let cursor = gridStart; cursor.toMillis() <= gridEnd.toMillis(); cursor = cursor.plus({ days: 1 })) {
      days.push(cursor);
    }
    return days;
  }, [monthStartKey, zone]);

  const calendarRequest = useMemo(() => {
    if (viewMode === "month" || viewMode === "list") {
      if (!monthStartKey) return null;
      const end = DateTime.fromISO(monthStartKey, { zone }).endOf("month").toFormat("yyyy-MM-dd");
      return `/api/v1/operations/calendar?start=${monthStartKey}&end=${end}`;
    }
    if (!weekStartKey) return null;
    const end = DateTime.fromISO(weekStartKey, { zone }).plus({ days: 6 }).toFormat("yyyy-MM-dd");
    return `/api/v1/operations/calendar?start=${weekStartKey}&end=${end}`;
  }, [monthStartKey, viewMode, weekStartKey, zone]);
  const calendarQuery = useOperationalQuery<CalendarResponse>(
    queryKeys.operationsCalendar.range(calendarRequest),
    calendarRequest,
  );
  const sessions = calendarQuery.data?.sessions ?? [];
  const absenceDays = calendarQuery.data?.absence_days ?? [];
  const loading = calendarQuery.isPending;

  useEffect(() => {
    if (calendarQuery.error) addToast("error", calendarQuery.error.message || "Failed to load calendar data");
  }, [addToast, calendarQuery.error]);

  const goPrevWeek = () => setWeekStartKey((prev) => (prev ? addDaysToKey(prev, zone, -7) : prev));
  const goNextWeek = () => setWeekStartKey((prev) => (prev ? addDaysToKey(prev, zone, 7) : prev));
  const goTodayWeek = () => {
    const day = todayKey;
    if (!day) return;
    setWeekStartKey(mondayOf(day, zone));
  };

  const goPrevMonth = () => setMonthStartKey((prev) => (prev ? shiftZoneMonthKey(prev, zone, -1) : prev));
  const goNextMonth = () => setMonthStartKey((prev) => (prev ? shiftZoneMonthKey(prev, zone, 1) : prev));
  const goTodayMonth = () => {
    const day = todayKey;
    if (!day) return;
    setMonthStartKey(startOfZoneMonthKey(day, zone));
  };

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
  }, [viewMode, weekStartKey, monthStartKey]);

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

  // Subject names present on any session in the range, used to apply the
  // subject filter to absence rows too (absences carry subject codes/names,
  // not course ids).
  const subjectNameSet = useMemo(() => {
    const set = new Set<string>();
    for (const session of sessions) {
      for (const candidate of [session.subject_name, session.course_name, session.course_code]) {
        if (candidate) set.add(candidate.trim().toLowerCase());
      }
    }
    return set;
  }, [sessions]);

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

    const keep = (absence: CalendarAbsence): boolean => {
      if (statusFilter ? absence.status !== statusFilter : absence.status === "cancelled") return false;
      if (subjectFilter) {
        const key = (absence.subject_name ?? absence.subject_code ?? "").trim().toLowerCase();
        if (!key || !subjectNameSet.has(key)) return false;
      }
      return true;
    };

    return absenceDays
      .map((day) => ({ ...day, absences: day.absences.filter(keep) }))
      .filter((day) => day.absences.length > 0);
  }, [absenceDays, showMode, statusFilter, subjectFilter, subjectNameSet]);

  const sessionsByDay = useMemo(() => {
    const map = new Map<string, CalendarSessionBrief[]>();
    for (const session of filteredSessions) {
      const day = sessionDayKey(session, zone);
      if (!map.has(day)) map.set(day, []);
      map.get(day)!.push(session);
    }
    return map;
  }, [filteredSessions, zone]);

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

  const allVisibleAbsences = useMemo(() => filteredAbsenceDays.flatMap((day) => day.absences), [filteredAbsenceDays]);

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

  if (loading || !weekStartKey || !monthStartKey) return <LoadingSkeleton type="table" lines={10} />;

  const monthStartDt = DateTime.fromISO(monthStartKey, { zone });
  const monthLabel = monthStartDt.isValid ? monthStartDt.setLocale("en-GB").toFormat("MMMM yyyy") : "";
  const weekLabel = weekStartKey ? formatMonthLabel(weekStartKey, zone) : "";

  return (
    <div className="w-full">
      <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-[32px] font-bold text-[var(--color-wi-text)]">Calendar</h1>
          <p className="text-sm text-[var(--color-wi-text-light)]">Combined view of scheduled sessions and student absences.</p>
        </div>
        <div className="flex items-center gap-2">
          <div className="flex rounded-sm border border-wi-line bg-white text-sm">
            <button onClick={() => setViewMode("week")} className={`flex items-center gap-1 px-3 py-1.5 ${viewMode === "week" ? "bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text)] font-medium" : "text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text)]"}`}>Week</button>
            <button onClick={() => setViewMode("month")} className={`flex items-center gap-1 px-3 py-1.5 ${viewMode === "month" ? "bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text)] font-medium" : "text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text)]"}`}>Month</button>
            <button onClick={() => setViewMode("list")} className={`flex items-center gap-1 px-3 py-1.5 ${viewMode === "list" ? "bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text)] font-medium" : "text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text)]"}`}>List</button>
          </div>
          <Button variant="secondary" size="sm" onClick={viewMode === "week" ? goTodayWeek : goTodayMonth}>
            Today
          </Button>
          <div className="flex items-center gap-1 text-sm font-medium text-[var(--color-wi-text-light)]">
            <button
              onClick={viewMode === "week" ? goPrevWeek : goPrevMonth}
              className="rounded-sm p-1 hover:bg-[var(--color-wi-row-alt)]"
              aria-label="Previous"
            >
              <ChevronLeft className="h-4 w-4" />
            </button>
            <span className="min-w-[180px] text-center">
              {viewMode === "month" ? monthLabel : weekLabel}
            </span>
            <button
              onClick={viewMode === "week" ? goNextWeek : goNextMonth}
              className="rounded-sm p-1 hover:bg-[var(--color-wi-row-alt)]"
              aria-label="Next"
            >
              <ChevronRight className="h-4 w-4" />
            </button>
          </div>
        </div>
      </div>

      <div className="mb-4 rounded-sm border border-wi-line bg-white px-4 py-2.5 text-sm text-[var(--color-wi-text-light)]">
        Summary: <strong className="text-[var(--color-wi-text)]">{totalVisibleAbsences}</strong> absences |{" "}
        <strong className="text-[var(--color-wi-text)]">{totalVisibleSitIns}</strong> sit-in assignments
      </div>

      <section className="mb-4 rounded-sm border border-wi-line bg-white p-3">
        <div className="flex flex-wrap gap-3">
          <span className="inline-flex min-h-[32px] items-center rounded-full bg-[var(--color-wi-row-alt)] px-3 text-xs font-semibold text-[var(--color-wi-text-light)]">
            Filters ({activeFilterCount})
          </span>
          <label className="flex items-center gap-2 text-sm text-[var(--color-wi-text-light)]">
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
        <SitInListView
          sessions={filteredSessions}
          absenceDays={filteredAbsenceDays.length ? filteredAbsenceDays : absenceDays}
          absences={allVisibleAbsences}
          mode={showMode === "absences" ? "absences" : "sit-ins"}
          zone={zone}
          hasFilters={activeFilterCount > 0}
          hasAnySitIns={hasAnySitIns}
          onClearFilters={clearFilters}
        />
      ) : showMode !== "all" && !hasVisibleCalendarActivity ? (
        <div className="rounded-sm border border-wi-line bg-white">
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
        <div className="overflow-hidden rounded-sm border border-wi-line bg-white" style={{ minHeight: "300px" }}>
          <div className="grid grid-cols-7 border-b border-wi-line bg-[var(--color-wi-row-alt)] text-center text-[10px] font-semibold uppercase tracking-wider text-[var(--color-wi-text-light)]">
            {["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"].map((d) => (
              <div key={d} className="py-1.5">{d}</div>
            ))}
          </div>
          <div className="grid grid-cols-7">
            {monthGrid.map((day) => {
              const dayStr = day.toFormat("yyyy-MM-dd");
              const dayAbsences = absencesByDay.get(dayStr);
              const daySessions = sessionsByDay.get(dayStr) ?? [];
              const dayAbsenceRows = absencesOnDate(dayAbsences);
              const absenceCount = dayAbsenceRows.length;
              const isToday = todayKey === dayStr;
              const isCurrentMonth = day.year === monthStartDt.year && day.month === monthStartDt.month;
              const dayLabel = formatFullDayLabel(dayStr);

              return (
                <div
                  key={dayStr}
                  className={`min-h-[80px] border-b border-r border-wi-line p-1 last:border-r-0 ${isToday ? "ring-1 ring-inset ring-[var(--color-wi-primary)]" : ""} ${!isCurrentMonth ? "bg-[var(--color-wi-row-alt)]" : ""}`}
                >
                  <button
                    type="button"
                    onClick={() => openPanel(dayStr)}
                    aria-label={`Open details for ${dayLabel}`}
                    className={`mb-1 flex h-5 w-full items-center text-[10px] leading-none ${isToday ? "justify-center" : "justify-end font-medium text-[var(--color-wi-faint)]"}`}
                  >
                    {isToday ? (
                      <span className="flex h-5 w-5 items-center justify-center rounded-full bg-[var(--color-wi-primary)] font-bold text-white">
                        {day.day}
                      </span>
                    ) : (
                      day.day
                    )}
                  </button>
                  <div className="space-y-1">
                    {daySessions.slice(0, 2).map((s) => (
                      <button
                        key={s.id}
                        type="button"
                        onClick={() => openPanel(dayStr, "sit-ins")}
                        aria-label={`Open details for ${getSessionLabel(s)} on ${dayLabel}`}
                        className="w-full rounded-sm bg-[var(--color-wi-blue-bg)] px-1.5 py-1 text-left text-[10px] text-[var(--color-wi-primary-dark)] hover:bg-[var(--color-wi-selected)]"
                      >
                        <div className="space-y-0.5">
                          <p className="truncate">{getSessionLabel(s)} {formatUTCToZone(s.start_at, zone, "HH:mm") ?? "--:--"}</p>
                          {s.sit_in_students && s.sit_in_students.length > 0 ? (
                            <p className="truncate text-[10px] text-[var(--color-wi-amber)]">
                              <span className="font-semibold">Visitors:</span>{" "}
                              {s.sit_in_students.slice(0, 2).map((student, idx) => (
                                <span key={`${student.wcode}-${student.absence_id}`}>
                                  {idx > 0 && ", "}
                                  {getSitInVisitorLabel(student)}
                                </span>
                              ))}
                              {s.sit_in_students.length > 2 ? (
                                <span className="text-[var(--color-wi-amber)]"> +{s.sit_in_students.length - 2} more</span>
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
                        className="w-full px-1 text-left text-[10px] text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text-light)]"
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
                        className={`w-full rounded-sm px-1.5 py-1 text-left text-[10px] leading-snug hover:bg-[var(--color-wi-selected)] ${absenceInlineClasses(absence)}`}
                      >
                        <p className="truncate font-semibold text-[var(--color-wi-text)]">{getAbsenceStudentLabel(absence)}</p>
                        <p className="truncate text-[10px] text-[var(--color-wi-amber)]">
                          <span className="font-semibold">Leave:</span> {getAbsenceSubjectLabel(absence)}
                        </p>
                        <p className="truncate text-[10px] text-[var(--color-wi-primary-dark)]">
                          <span className="font-semibold">Sit-in:</span> {getSitInLabel(absence)}
                        </p>
                      </button>
                    ))}
                    {absenceCount > 2 ? (
                      <button
                        type="button"
                        onClick={() => openPanel(dayStr, "absences")}
                        aria-label={`View all absence details for ${dayLabel}`}
                        className="w-full px-1 text-left text-[10px] text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text-light)]"
                      >
                        +{absenceCount - 2} more
                      </button>
                    ) : null}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      ) : (
        <div
          className="grid grid-cols-7 gap-px overflow-hidden rounded-sm border border-wi-line bg-[var(--color-wi-row-alt)]"
          style={{ minHeight: "400px" }}
        >
          {weekDates.map((day) => {
            const dayStr = day.toFormat("yyyy-MM-dd");
            const dayAbsences = absencesByDay.get(dayStr);
            const daySessions = sessionsByDay.get(dayStr) ?? [];
            const dayAbsenceRows = absencesOnDate(dayAbsences);
            const absenceCount = dayAbsenceRows.length;
            const isToday = todayKey === dayStr;
            const dayLabel = formatFullDayLabel(dayStr);

            return (
              <div
                key={dayStr}
                className={`flex min-h-[200px] flex-col bg-white ${day.day % 2 === 0 ? "bg-[var(--color-wi-row-alt)]/50" : ""} ${isToday ? "ring-2 ring-inset ring-[var(--color-wi-primary)]" : ""}`}
              >
                <button
                  type="button"
                  onClick={() => openPanel(dayStr)}
                  aria-label={`Open details for ${dayLabel}`}
                  className={`sticky top-0 z-10 border-b border-wi-line-soft px-2 py-2 text-center ${isToday ? "bg-[var(--color-wi-blue-bg)]" : ""}`}
                >
                  <p className={`text-xs font-semibold ${isToday ? "text-[var(--color-wi-primary)]" : "text-[var(--color-wi-text-light)]"}`}>
                    {day.setLocale("en-GB").toFormat("EEE d")}
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
                      className="w-full rounded-sm border border-wi-line-soft bg-white px-2 py-1.5 text-left text-xs shadow-sm transition-shadow hover:shadow-md"
                    >
                      <p className="font-semibold text-[var(--color-wi-text)]">{getSessionLabel(session)}</p>
                      <p className="text-[var(--color-wi-text-light)]">{formatUTCToZone(session.start_at, zone, "HH:mm") ?? "--:--"} &ndash; {formatUTCToZone(session.end_at, zone, "HH:mm") ?? "--:--"}</p>
                      {session.room_name ? <p className="truncate text-[var(--color-wi-text-light)]">{session.room_name}</p> : null}
                      {session.sit_in_students && session.sit_in_students.length > 0 ? (
                        <div className="mt-1 border-t border-wi-line-soft pt-1">
                          <p className="text-[10px] text-[var(--color-wi-amber)]">
                            <span className="font-semibold">Visitors:</span>{" "}
                            {session.sit_in_students.slice(0, 2).map((student, idx) => (
                              <span key={student.wcode}>
                                {idx > 0 && ", "}
                                {getSitInVisitorLabel(student)}
                              </span>
                            ))}
                            {session.sit_in_students.length > 2 ? (
                              <span className="text-[var(--color-wi-amber)]"> +{session.sit_in_students.length - 2} more</span>
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
                      className={`block w-full rounded-sm px-2 py-1.5 text-left text-[11px] shadow-sm transition-colors hover:shadow-md ${absenceInlineClasses(absence)}`}
                    >
                      <p className="truncate font-semibold text-[var(--color-wi-text)]">{getAbsenceStudentLabel(absence)}</p>
                      <p className="truncate text-[var(--color-wi-amber)]">
                        <span className="font-semibold">Leave:</span> {getAbsenceSubjectLabel(absence)}
                      </p>
                      <p className="truncate text-[var(--color-wi-primary-dark)]">
                        <span className="font-semibold">Sit-in:</span> {getSitInLabel(absence)}
                      </p>
                    </button>
                  ))}
                  {daySessions.length === 0 && absenceCount === 0 ? (
                    <p className="px-1 py-4 text-center text-xs text-[var(--color-wi-text-light)]">No activity</p>
                  ) : null}
                  {absenceCount > 2 ? (
                    <button
                      type="button"
                      onClick={() => openPanel(dayStr, "absences")}
                      aria-label={`View all absence details for ${dayLabel}`}
                      className="w-full px-1 text-left text-[10px] text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text-light)]"
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