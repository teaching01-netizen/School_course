import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import clsx from "clsx";
import { Check, ChevronDown, ChevronLeft, ChevronRight, Plus } from "lucide-react";
import { formatTime } from "@/utils/date";
import { instituteDateKey, groupByDay } from "@/features/absences/domain/sessionGrouping";
import type { SubjectSessions } from "@/features/absences/types";
import LoadingSkeleton from "@/components/ui/LoadingSkeleton";
import MobileBottomSheet from "./MobileBottomSheet";
import ScreenTitle, { ScreenSubtitle } from "./ScreenTitle";

type ScheduleScreenProps = {
  groups: SubjectSessions[];
  selectedIds: Set<string>;
  /** Returns true when the toggle was accepted (added/removed). */
  onToggleDay: (group: SubjectSessions, sessionIds: string[]) => boolean;
  /** Missed-session id → chosen make-up value, to warn when a change affects it. */
  sitInSelections?: Record<string, string>;
  onLimitTap: (group: SubjectSessions, rowKey: string) => void;
  limitNotice?: string | null;
  limitNoticeKey?: string | null;
  loading?: boolean;
  error?: string | null;
  onRetry?: () => void;
  draftNeedsReview?: boolean;
  onDismissDraftNotice?: () => void;
  /** mailto:/tel: target for the "Contact Student Services" support action shown under inline limit notices. */
  supportHref?: string;
};

/** One selectable "class" on a given day (a course/day grouping). */
type CalendarEvent = {
  key: string;
  dateKey: string;
  label: string;
  teacher?: string;
  timeLabel: string;
  sessionIds: string[];
  group: SubjectSessions;
  alreadyAbsent: boolean;
  limitReached: boolean;
  selected: boolean;
};

function dateKeyFromParts(year: number, monthIndex: number, day: number): string {
  return `${year}-${String(monthIndex + 1).padStart(2, "0")}-${String(day).padStart(2, "0")}`;
}

function parseDateKey(dateKey: string): Date {
  const [year, month, day] = dateKey.split("-").map(Number);
  return new Date(year, month - 1, day);
}

/** dateKey shifted by whole days (kept as a plain calendar date, no wall-clock math). */
function shiftDateKey(dateKey: string, dayDelta: number): string {
  const d = parseDateKey(dateKey);
  d.setDate(d.getDate() + dayDelta);
  return dateKeyFromParts(d.getFullYear(), d.getMonth(), d.getDate());
}

function dayLabel(dateKey: string): string {
  const d = parseDateKey(dateKey);
  if (Number.isNaN(d.getTime())) return dateKey;
  const today = instituteDateKey(new Date().toISOString());
  if (dateKey === today) return "Today";
  const tomorrow = new Date();
  tomorrow.setDate(tomorrow.getDate() + 1);
  if (dateKey === instituteDateKey(tomorrow.toISOString())) return "Tomorrow";
  const weekday = d.toLocaleDateString("en-GB", { weekday: "short" }).toUpperCase();
  const month = d.toLocaleDateString("en-GB", { month: "short" }).toUpperCase();
  const sameYear = d.getFullYear() === new Date().getFullYear();
  return sameYear ? `${weekday} ${d.getDate()} ${month}` : `${weekday} ${d.getDate()} ${month} ${d.getFullYear()}`;
}

function longDateLabel(dateKey: string): string {
  const d = parseDateKey(dateKey);
  if (Number.isNaN(d.getTime())) return dateKey;
  return d.toLocaleDateString("en-GB", { weekday: "long", day: "numeric", month: "long", year: "numeric" });
}

function monthTitle(year: number, monthIndex: number): string {
  return new Date(year, monthIndex, 1).toLocaleDateString("en-GB", { month: "long", year: "numeric" });
}

const WEEKDAY_HEADERS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

function buildEvents(
  groups: SubjectSessions[],
  selectedIds: Set<string>,
): CalendarEvent[] {
  const events: CalendarEvent[] = [];
  for (const group of groups) {
    for (const dayGroup of groupByDay(group.sessions)) {
      const reportableItems = dayGroup.items.filter((item) => !item.already_absent);
      const sessionIds = reportableItems.map((item) => item.id);
      const alreadyAbsent = reportableItems.length === 0;
      const label = group.merge_group_name?.trim() || group.subject_name?.trim() || group.course_name?.trim() || "Class";
      events.push({
        key: dayGroup.id,
        dateKey: dayGroup.date,
        label,
        teacher: group.teacher_name?.trim() || undefined,
        timeLabel: `${formatTime(dayGroup.start_at)}–${formatTime(dayGroup.end_at)}`,
        sessionIds,
        group,
        alreadyAbsent,
        limitReached: !alreadyAbsent && group.absence_limit_reached === true,
        selected: !alreadyAbsent && sessionIds.every((id) => selectedIds.has(id)),
      });
    }
  }
  events.sort((a, b) => a.dateKey.localeCompare(b.dateKey) || a.timeLabel.localeCompare(b.timeLabel));
  return events;
}

export default function ScheduleScreen({
  groups,
  selectedIds,
  onToggleDay,
  sitInSelections = {},
  onLimitTap,
  limitNotice = null,
  limitNoticeKey = null,
  loading = false,
  error = null,
  onRetry,
  draftNeedsReview = false,
  onDismissDraftNotice,
  supportHref,
}: ScheduleScreenProps) {
  const events = useMemo(() => buildEvents(groups, selectedIds), [groups, selectedIds]);

  const reportableEvents = useMemo(() => events.filter((event) => !event.alreadyAbsent), [events]);
  const byDate = useMemo(() => {
    const map = new Map<string, CalendarEvent[]>();
    for (const event of events) {
      const bucket = map.get(event.dateKey);
      if (bucket) bucket.push(event);
      else map.set(event.dateKey, [event]);
    }
    return map;
  }, [events]);
  const eventCountByDate = useMemo(() => {
    const counts = new Map<string, number>();
    for (const dateKey of byDate.keys()) {
      const reportable = (byDate.get(dateKey) ?? []).filter((event) => !event.alreadyAbsent).length;
      if (reportable > 0) counts.set(dateKey, reportable);
    }
    return counts;
  }, [byDate]);

  const selectedEvents = useMemo(() => events.filter((event) => event.selected), [events]);
  const selectedDateKeys = useMemo(() => new Set(selectedEvents.map((event) => event.dateKey)), [selectedEvents]);

  // ---- Calendar state -----------------------------------------------------
  const [focusedKey, setFocusedKey] = useState<string | null>(null);
  const [month, setMonth] = useState<{ year: number; month: number } | null>(null);
  const [expanded, setExpanded] = useState<boolean | null>(null);
  const [hydrated, setHydrated] = useState(false);
  const [updateNotice, setUpdateNotice] = useState<string | null>(null);

  // Keyboard focus position inside the calendar. Deliberately separate from
  // focusedKey: arrowing explores without changing the selected day/agenda.
  const [navKey, setNavKey] = useState<string | null>(null);
  const previousNavKey = useRef<string | null>(null);
  const gridRef = useRef<HTMLDivElement | null>(null);

  const todayKey = useMemo(() => instituteDateKey(new Date().toISOString()), []);

  const initialFocusKey = useMemo(() => {
    if (selectedEvents.length > 0) return selectedEvents[0].dateKey;
    // Start from where the student is most likely trying to go: today if it
    // holds a reportable class, otherwise the next reportable day.
    const todayHasReportable = reportableEvents.some((event) => event.dateKey === todayKey);
    if (todayHasReportable) return todayKey;
    const nextReportable = reportableEvents.find((event) => event.dateKey >= todayKey);
    return nextReportable?.dateKey ?? todayKey;
  }, [selectedEvents, reportableEvents, todayKey]);

  useEffect(() => {
    if (hydrated || loading) return;
    // Wait for the real schedule (or an error) before picking an initial date.
    if (error || reportableEvents.length > 0) {
      const focus = initialFocusKey;
      const focused = parseDateKey(focus);
      setFocusedKey(focus);
      setNavKey(focus);
      setMonth({ year: focused.getFullYear(), month: focused.getMonth() });
      setHydrated(true);
    }
  }, [hydrated, loading, error, reportableEvents.length, initialFocusKey]);

  useEffect(() => {
    if (expanded !== null) return;
    if (typeof window.matchMedia !== "function") { setExpanded(false); return; }
    const query = window.matchMedia("(min-width: 768px)");
    const apply = () => setExpanded(query.matches);
    apply();
    query.addEventListener?.("change", apply);
    return () => query.removeEventListener?.("change", apply);
  }, [expanded]);

  const monthWeeks = useMemo(() => {
    if (!month) return [];
    const first = new Date(month.year, month.month, 1);
    const mondayOffset = (first.getDay() + 6) % 7;
    const start = new Date(month.year, month.month, 1 - mondayOffset);
    const weeks: string[][] = [];
    const cursor = new Date(start);
    while (weeks.length < 6) {
      const week: string[] = [];
      for (let i = 0; i < 7; i += 1) {
        week.push(dateKeyFromParts(cursor.getFullYear(), cursor.getMonth(), cursor.getDate()));
        cursor.setDate(cursor.getDate() + 1);
      }
      weeks.push(week);
    }
    return weeks;
  }, [month]);

  // Collapsed mode shows the week around the keyboard focus (navKey), so
  // arrowing across a week edge rolls the rail without changing selection.
  const focusedWeekDays = useMemo(() => {
    const anchor = navKey ?? focusedKey;
    if (!anchor) return [];
    const date = parseDateKey(anchor);
    const mondayOffset = (date.getDay() + 6) % 7;
    date.setDate(date.getDate() - mondayOffset);
    const days: string[] = [];
    for (let i = 0; i < 7; i += 1) {
      days.push(dateKeyFromParts(date.getFullYear(), date.getMonth(), date.getDate()));
      date.setDate(date.getDate() + 1);
    }
    return days;
  }, [navKey, focusedKey]);

  const shiftMonth = (delta: number) => {
    setMonth((current) => {
      const next = new Date((current?.year ?? new Date().getFullYear()), (current?.month ?? new Date().getMonth()) + delta, 1);
      return { year: next.getFullYear(), month: next.getMonth() };
    });
  };

  /** Moves the keyboard focus by whole days, keeping the visible grid around it. */
  const shiftNav = (dayDelta: number) => {
    const base = navKey ?? focusedKey ?? todayKey;
    const next = shiftDateKey(base, dayDelta);
    setNavKey(next);
    const parsed = parseDateKey(next);
    setMonth((current) => (
      current && current.year === parsed.getFullYear() && current.month === parsed.getMonth()
        ? current
        : { year: parsed.getFullYear(), month: parsed.getMonth() }
    ));
  };

  const handleGridKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    // APG date-picker keys: arrows move by day/week, Home/End jump to the
    // start/end of the focused week. PageUp/PageDown stay unbound (month
    // jumps would fight the agenda model) — the group label names the keys
    // that actually work so nothing is promised that isn't handled.
    if (event.key === "Home" || event.key === "End") {
      event.preventDefault();
      const base = parseDateKey(navKey ?? focusedKey ?? todayKey);
      // Weeks render Monday-first: back up to Monday, or forward to Sunday.
      const mondayOffset = (base.getDay() + 6) % 7;
      shiftNav(event.key === "Home" ? -mondayOffset : 6 - mondayOffset);
      return;
    }
    const deltas: Record<string, number> = {
      ArrowLeft: -1,
      ArrowRight: 1,
      ArrowUp: -7,
      ArrowDown: 7,
    };
    const delta = deltas[event.key];
    if (delta === undefined) return;
    event.preventDefault();
    shiftNav(delta);
  };

  // Keep the focused cell's ring on the roved date. Never focus on hydrate:
  // pointer/keyboard changes move it, screen entry leaves focus alone.
  useEffect(() => {
    const previous = previousNavKey.current;
    previousNavKey.current = navKey;
    if (!navKey || previous === null) return;
    const cell = gridRef.current?.querySelector<HTMLElement>(`[data-date-key="${navKey}"]`);
    if (cell && document.activeElement !== cell) cell.focus({ preventScroll: true });
  }, [navKey]);

  const goToToday = () => {
    const target = reportableEvents.some((event) => event.dateKey === todayKey)
      ? todayKey
      : (reportableEvents.find((event) => event.dateKey >= todayKey)?.dateKey ?? todayKey);
    const parsed = parseDateKey(target);
    setMonth({ year: parsed.getFullYear(), month: parsed.getMonth() });
    setFocusedKey(target);
    setNavKey(target);
    setExpanded((current) => (current === null ? false : current));
  };

  const selectDate = (dateKey: string) => {
    setFocusedKey(dateKey);
    setNavKey(dateKey);
    if (!expanded && month) {
      const parsed = parseDateKey(dateKey);
      const sameMonth = parsed.getFullYear() === month.year && parsed.getMonth() === month.month;
      if (!sameMonth) setMonth({ year: parsed.getFullYear(), month: parsed.getMonth() });
    }
  };

  // ---- Removal undo -------------------------------------------------------
  // Stacked toasts: each removal gets its own Undo + expiry so removing a
  // second class never steals the first removal's chance to be undone.
  type RemovedToast = { key: string; event: CalendarEvent };
  // ---- Selected-list sheet -------------------------------------------------
  const [viewSheetOpen, setViewSheetOpen] = useState(false);
  const viewTriggerRef = useRef<HTMLButtonElement | null>(null);
  useEffect(() => {
    if (viewSheetOpen && selectedEvents.length === 0) setViewSheetOpen(false);
  }, [viewSheetOpen, selectedEvents.length]);

  const [removedToasts, setRemovedToasts] = useState<RemovedToast[]>([]);
  const toastSeq = useRef(0);
  const dismissRemovedToast = (key: string) => {
    setRemovedToasts((current) => current.filter((toast) => toast.key !== key));
  };

  const removeEvent = (event: CalendarEvent) => {
    const hadMakeUp = event.sessionIds.some((id) => sitInSelections[id]);
    onToggleDay(event.group, event.sessionIds);
    if (hadMakeUp) setUpdateNotice("Changing this class will update your make-up option.");
    toastSeq.current += 1;
    const key = `${event.key}:${toastSeq.current}`;
    setRemovedToasts((current) => [...current, { key, event }].slice(-3));
  };

  const undoRemovedToast = (key: string) => {
    const toast = removedToasts.find((candidate) => candidate.key === key);
    if (!toast) return;
    dismissRemovedToast(key);
    onToggleDay(toast.event.group, toast.event.sessionIds);
  };

  const toggleEvent = (event: CalendarEvent) => {
    setUpdateNotice(null);
    if (event.selected) {
      removeEvent(event);
      return;
    }
    const accepted = onToggleDay(event.group, event.sessionIds);
    if (!accepted && focusedKey) setFocusedKey(focusedKey); // keep the limit notice context stable
  };

  const isExpanded = expanded === true;
  const visibleWeeks = isExpanded || monthWeeks.length === 0 ? monthWeeks : [focusedWeekDays];
  const renderedDateKeys = useMemo(() => visibleWeeks.flat(), [visibleWeeks]);
  const activeKey = renderedDateKeys.includes(navKey ?? "")
    ? navKey
    : renderedDateKeys.includes(focusedKey ?? "")
      ? focusedKey
      : renderedDateKeys[0] ?? null;

  const focusedEvents = (focusedKey ? byDate.get(focusedKey) ?? [] : []).sort((a, b) =>
    a.timeLabel.localeCompare(b.timeLabel),
  );
  const focusedReportableCount = focusedEvents.filter((event) => !event.alreadyAbsent).length;
  const nextReportableAfterFocus = reportableEvents.find(
    (event) => focusedKey !== null && event.dateKey > focusedKey,
  );

  const showLimitRow = (event: CalendarEvent) => limitNoticeKey === event.key && limitNotice;

  // ---- Rendering ----------------------------------------------------------

  if (loading) {
    return (
      <div className="mx-auto w-full max-w-2xl lg:max-w-5xl">
        <ScreenTitle>
        Which class will you miss?

      </ScreenTitle>
        <div className="mt-8">
          <LoadingSkeleton type="table" lines={4} />
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="mx-auto w-full max-w-2xl lg:max-w-5xl">
        <ScreenTitle>
        Which class will you miss?

      </ScreenTitle>
        <div role="alert" className="mt-8 space-y-3 rounded-2xl border border-[var(--color-wi-red)]/20 bg-[var(--color-wi-danger-bg)] p-5">
          <p className="text-[15px] leading-snug text-[var(--color-wi-red)]">{error}</p>
          {onRetry ? (
            <button
              type="button"
              onClick={onRetry}
              className="wi-press min-h-11 rounded-lg border border-[var(--color-wi-red)]/40 px-3 text-[15px] font-semibold text-[var(--color-wi-red)] hover:bg-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-red)]/40"
            >
              Try again
            </button>
          ) : null}
        </div>
      </div>
    );
  }

  const nothingToShow = events.length === 0 || (reportableEvents.length === 0 && selectedIds.size === 0);

  if (nothingToShow) {
    return (
      <div className="mx-auto w-full max-w-xl">
        <ScreenTitle>
        Which class will you miss?

      </ScreenTitle>
        <div className="wi-card mt-8 rounded-2xl border border-[var(--color-wi-border)] bg-white p-8 text-center">
          <p className="text-[17px] font-semibold text-[var(--color-wi-text)]">
            {events.length > 0 ? "No more classes to report" : "No upcoming classes"}
          </p>
          <p className="mx-auto mt-1.5 max-w-sm text-[15px] leading-relaxed text-[var(--color-wi-text-light)]">
            {events.length > 0
              ? "Your upcoming classes have already been reported."
              : "We couldn't find any classes that you can report as absent right now."}
          </p>
          <p className="mt-4 text-[13px] text-[var(--color-wi-text-light)]">
            Need help? Contact Student Services.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-2xl lg:max-w-5xl">
      <ScreenTitle>
        Which class will you miss?

      </ScreenTitle>
      <ScreenSubtitle>
        Selecting a class includes all its sessions that day.

      </ScreenSubtitle>

      {draftNeedsReview ? (
        <div role="status" aria-live="polite" className="mt-6 rounded-xl border border-[var(--color-wi-amber)]/30 bg-[var(--color-wi-amber-bg)] px-4 py-3 text-[15px] text-[var(--color-wi-amber-ink)]">
          <p className="font-semibold">We restored your report.</p>
          <p className="mt-0.5">Check your classes below, then tap the button to continue. Continue stays disabled until then.</p>
          {onDismissDraftNotice ? (
            <button
              type="button"
              onClick={onDismissDraftNotice}
              className="mt-2 min-h-11 rounded-lg border border-[var(--color-wi-amber)] px-3 text-[15px] font-semibold hover:bg-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-amber)]"
            >
              I&apos;ve reviewed — continue
            </button>
          ) : null}
        </div>
      ) : null}

      <div className="lg:mt-6 lg:grid lg:grid-cols-12 lg:items-start lg:gap-6">
        {/* WHEN? — the calendar is navigation through time, not selection. */}
        <div className="lg:col-span-5">
          <div
            ref={gridRef}
            onKeyDown={handleGridKeyDown}
            className="wi-card mt-6 rounded-2xl border border-[var(--color-wi-border)] bg-white p-4 sm:p-5 lg:mt-0"
            role="group"
            aria-label="Your classes by day. Use the arrow keys to move between days, Home and End to jump to the start or end of the week, Enter to open a day."
          >
            <div className="flex items-center justify-between gap-2">
              {isExpanded ? (
                <div className="flex items-center gap-1">
                  <button
                    type="button"
                    onClick={() => shiftMonth(-1)}
                    aria-label="Previous month"
                    className="wi-press flex h-11 w-11 items-center justify-center rounded-lg text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] active:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                  >
                    <ChevronLeft className="h-5 w-5" aria-hidden="true" />
                  </button>
                  <button
                    type="button"
                    onClick={() => shiftMonth(1)}
                    aria-label="Next month"
                    className="wi-press flex h-11 w-11 items-center justify-center rounded-lg text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] active:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                  >
                    <ChevronRight className="h-5 w-5" aria-hidden="true" />
                  </button>
                </div>
              ) : <span />}
              <p className="text-[15px] font-semibold text-[var(--color-wi-text)]">
                {month ? monthTitle(month.year, month.month) : ""}
                {!isExpanded && focusedKey ? ` · ${dayLabel(focusedKey)}` : ""}
              </p>
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  onClick={goToToday}
                  className="wi-press min-h-11 rounded-lg px-2.5 text-[13px] font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary)]/5 active:bg-[var(--color-wi-primary)]/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                >
                  Today
                </button>
                <button
                  type="button"
                  onClick={() => setExpanded((current) => !(current === true))}
                  aria-expanded={isExpanded}
                  aria-label={isExpanded ? "Show one week" : "Show the whole month — find days further out"}
                  className="wi-press flex h-11 w-11 items-center justify-center rounded-lg text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] active:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                >
                  <ChevronDown
                    className={clsx("h-5 w-5 transition-transform motion-reduce:transition-none", isExpanded && "rotate-180")}
                    aria-hidden="true"
                  />
                </button>
              </div>
            </div>

            <div className="mt-3 grid grid-cols-7 gap-1 text-center">
              {WEEKDAY_HEADERS.map((header) => (
                <span key={header} className="py-1 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">
                  {header}
                </span>
              ))}
            </div>

            <div className="mt-1 space-y-1">
              {visibleWeeks.map((week, weekIndex) => (
                <div key={`${week[0]}-${weekIndex}`} className="grid grid-cols-7 gap-1">
                  {week.map((dateKey) => {
                    const parsed = parseDateKey(dateKey);
                    const inMonth = month ? parsed.getFullYear() === month.year && parsed.getMonth() === month.month : true;
                    const isFocused = focusedKey === dateKey;
                    const isToday = dateKey === todayKey;
                    const count = eventCountByDate.get(dateKey) ?? 0;
                    const hasSelection = selectedDateKeys.has(dateKey);
                    const labelParts = [
                      longDateLabel(dateKey),
                      isToday ? "Today" : null,
                      count > 0 ? `${count} ${count === 1 ? "class" : "classes"}` : "No classes",
                      hasSelection ? "Selected" : null,
                    ].filter(Boolean);
                    return (
                      <button
                        key={dateKey}
                        type="button"
                        onClick={() => selectDate(dateKey)}
                        data-date-key={dateKey}
                        aria-current={isFocused ? "date" : undefined}
                        tabIndex={dateKey === activeKey ? 0 : -1}
                        aria-label={labelParts.join(". ")}
                        className={clsx(
                          "wi-press flex min-h-12 flex-col items-center justify-center rounded-xl border py-1.5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]",
                          isFocused
                            ? "border-[var(--color-wi-primary)] bg-[var(--color-wi-primary)]/5"
                            : "border-transparent hover:bg-[var(--color-wi-row-alt)] active:bg-[var(--color-wi-row-alt)]",
                        )}
                      >
                        <span
                          className={clsx(
                            "flex h-8 w-8 items-center justify-center rounded-full text-[15px] tabular-nums transition-colors motion-reduce:transition-none",
                            hasSelection
                              ? "bg-[var(--color-wi-primary)] font-bold text-white"
                              : isToday
                                ? "font-bold text-[var(--color-wi-primary)]"
                                : isFocused
                                  ? "font-semibold text-[var(--color-wi-primary)]"
                                  : inMonth
                                    ? "text-[var(--color-wi-text)]"
                                    : "text-[var(--color-wi-text-light)]",
                          )}
                        >
                          {parsed.getDate()}
                        </span>
                        <span className="mt-0.5 flex h-2 items-center justify-center gap-0.5" aria-hidden="true">
                          {hasSelection ? (
                            <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-wi-primary)]" />
                          ) : count > 0 ? (
                            <>
                              {Array.from({ length: Math.min(count, 3) }, (_, index) => (
                                <span
                                  key={index}
                                  className={clsx("h-1.5 w-1.5 rounded-full", isFocused ? "bg-[var(--color-wi-primary)]" : "bg-[var(--color-wi-text-light)]")}
                                />
                              ))}
                              {count > 3 ? <span className="text-[9px] font-semibold text-[var(--color-wi-text-light)]">+{count - 3}</span> : null}
                            </>
                          ) : null}
                        </span>
                      </button>
                    );
                  })}
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* WHAT? — the daily agenda holds the actual decision. */}
        <div className="lg:col-span-7">
          <div className="mt-6 lg:mt-0">
            <div className="flex items-baseline justify-between gap-3">
              <h2 className="text-[20px] font-bold text-[var(--color-wi-text)]" aria-live="polite">
                {focusedKey ? dayLabel(focusedKey) : ""}
                <span className="font-normal text-[var(--color-wi-text-light)]">
                  {focusedKey ? ` · ${longDateLabel(focusedKey)}` : ""}
                </span>
              </h2>
              {focusedReportableCount > 0 ? (
                <span className="shrink-0 text-[13px] font-medium text-[var(--color-wi-text-light)]">
                  {focusedReportableCount === 1 ? "1 class" : `${focusedReportableCount} classes`}
                </span>
              ) : null}
            </div>

            {updateNotice ? (
              <p role="status" className="mt-3 rounded-xl border border-[var(--color-wi-amber)]/30 bg-[var(--color-wi-amber-bg)] px-4 py-2.5 text-[13px] leading-snug text-[var(--color-wi-amber-ink)]">
                {updateNotice}
              </p>
            ) : null}

            {focusedEvents.length === 0 ? (
              <div className="wi-card mt-3 rounded-2xl border border-[var(--color-wi-border)] bg-white p-6 text-center">
                <p className="text-[15px] font-semibold text-[var(--color-wi-text)]">No classes this day</p>
                {nextReportableAfterFocus ? (
                  <button
                    type="button"
                    onClick={() => selectDate(nextReportableAfterFocus.dateKey)}
                    className="wi-press mt-3 min-h-11 rounded-lg px-3 text-[15px] font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary)]/5 active:bg-[var(--color-wi-primary)]/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                  >
                    Next available class day · {dayLabel(nextReportableAfterFocus.dateKey)}
                  </button>
                ) : null}
              </div>
            ) : (
              <ul className="mt-3 space-y-2.5">
                {focusedEvents.map((event) => {
                  if (event.alreadyAbsent) {
                    return (
                      <li key={event.key}>
                        <div className="flex min-h-14 items-center gap-3.5 rounded-xl border border-dashed border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] px-4 py-3" aria-disabled="true">
                          <span aria-hidden="true" className="h-2 w-2 shrink-0 rounded-full bg-[var(--color-wi-text-light)]/40" />
                          <span className="min-w-0 flex-1">
                            <span className="block truncate text-[15px] font-semibold text-[var(--color-wi-text)]">{event.label}</span>
                            <span className="block truncate text-[13px] text-[var(--color-wi-text-light)]">
                              {event.timeLabel}{event.teacher ? ` · ${event.teacher}` : ""}
                            </span>
                          </span>
                          <span className="shrink-0 rounded-full bg-[var(--color-wi-text-light)]/10 px-2.5 py-0.5 text-[12px] font-semibold text-[var(--color-wi-text-light)]">Already reported</span>
                        </div>
                      </li>
                    );
                  }
                  if (event.limitReached) {
                    return (
                      <li key={event.key}>
                        <button
                          type="button"
                          aria-expanded={limitNoticeKey === event.key}
                          onClick={() => onLimitTap(event.group, event.key)}
                          className="wi-press flex min-h-14 w-full items-center gap-3.5 rounded-xl border border-[var(--color-wi-border)] bg-white px-4 py-3 text-left hover:bg-[var(--color-wi-row-alt)] active:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                        >
                          <span className="h-6 w-6 shrink-0 rounded-full border-2 border-[var(--color-wi-border)]" aria-hidden="true" />
                          <span className="min-w-0 flex-1">
                            <span className="block truncate text-[15px] font-semibold text-[var(--color-wi-text)]">{event.label}</span>
                            <span className="block truncate text-[13px] text-[var(--color-wi-text-light)]">
                              {event.timeLabel}{event.teacher ? ` · ${event.teacher}` : ""}
                            </span>
                          </span>
                          <span className="shrink-0 text-[13px] font-medium text-[var(--color-wi-amber-ink)]">Absence limit reached</span>
                        </button>
                        {showLimitRow(event) ? (
                          <p role="alert" className="mt-1.5 ml-11 rounded-lg bg-[var(--color-wi-amber-bg)] px-3 py-2 text-[13px] leading-snug text-[var(--color-wi-amber-ink)]">
                            {limitNotice}
                          </p>
                        ) : null}
                      </li>
                    );
                  }
                  return (
                    <li key={event.key}>
                      <label
                        className={clsx(
                          "wi-press relative flex min-h-14 w-full cursor-pointer items-center gap-3.5 rounded-xl border px-4 py-3 text-left",
                          "has-[:focus-visible]:outline has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-[var(--color-wi-primary)]",
                          event.selected
                            ? "border-[var(--color-wi-primary)]/50 bg-[var(--color-wi-primary)]/5"
                            : "border-[var(--color-wi-border)] bg-white hover:bg-[var(--color-wi-row-alt)] active:bg-[var(--color-wi-row-alt)]",
                        )}
                      >
                        <span
                          aria-hidden="true"
                          className={clsx(
                            "flex h-6 w-6 shrink-0 items-center justify-center rounded-full border-2 transition-colors motion-reduce:transition-none",
                            event.selected
                              ? "border-[var(--color-wi-primary)] bg-[var(--color-wi-primary)] text-white"
                              : "border-[var(--color-wi-border)] bg-white",
                          )}
                        >
                          {event.selected ? <Check className="h-4 w-4" strokeWidth={3} /> : null}
                        </span>
                        <span className="min-w-0 flex-1">
                            <span className="block truncate text-[15px] font-semibold text-[var(--color-wi-text)]">
                              {event.label}
                              {event.teacher ? <span className="font-normal text-[var(--color-wi-text-light)]"> · {event.teacher}</span> : null}
                            </span>
                            <span className="mt-0.5 block truncate text-[13px] text-[var(--color-wi-text-light)]">{event.timeLabel}</span>
                            {event.sessionIds.length > 1 ? (
                              <span className="mt-0.5 block text-[12px] text-[var(--color-wi-text-light)]">
                                Includes {event.sessionIds.length} sessions
                              </span>
                            ) : null}
                          </span>
                          {event.selected ? (
                            <span className="shrink-0 text-[13px] font-semibold text-[var(--color-wi-primary)]">Selected</span>
                          ) : null}
                        <input
                          type="checkbox"
                          name={`schedule-${event.key}`}
                          checked={event.selected}
                          onChange={() => toggleEvent(event)}
                          className="absolute h-px w-px opacity-0"
                        />
                      </label>
                      {showLimitRow(event) ? (
                        <div role="alert" className="mt-1.5 ml-11 rounded-lg bg-[var(--color-wi-amber-bg)] px-3 py-2 text-[13px] leading-snug text-[var(--color-wi-amber-ink)]">
                          <p>{limitNotice}</p>
                          {supportHref ? (
                            <a href={supportHref} className="mt-1 inline-block font-semibold underline underline-offset-2 hover:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]">
                              Contact Student Services
                            </a>
                          ) : null}
                        </div>
                      ) : null}
                    </li>
                  );
                })}
              </ul>
            )}
          </div>

          {/* Persistent selection summary — compact; the full list with removal
              controls is one View tap away so it never competes with the agenda. */}
          {selectedEvents.length > 0 ? (
            <div className="mt-8 space-y-2.5">
              <div className="wi-card flex items-center justify-between gap-3 rounded-2xl border border-[var(--color-wi-border)] bg-white px-4 py-3">
                <p className="min-w-0 text-[15px] font-semibold leading-snug text-[var(--color-wi-text)]">
                  {selectedEvents.length === 1 ? "1 class day selected" : `${selectedEvents.length} class days selected`}
                  <span className="block text-[13px] font-normal text-[var(--color-wi-text-light)]">
                    {selectedDateKeys.size === 1 ? "1 date" : `${selectedDateKeys.size} dates`}
                  </span>
                </p>
                <button
                  ref={viewTriggerRef}
                  type="button"
                  onClick={() => setViewSheetOpen(true)}
                  className="wi-press min-h-11 shrink-0 rounded-lg px-3 text-[15px] font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary)]/5 active:bg-[var(--color-wi-primary)]/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                >
                  View
                </button>
              </div>
              <button
                type="button"
                onClick={() => {
                  setViewSheetOpen(false);
                  if (nextReportableAfterFocus) selectDate(nextReportableAfterFocus.dateKey);
                  const reduceMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;
                  gridRef.current?.scrollIntoView({ behavior: reduceMotion ? "auto" : "smooth", block: "nearest" });
                }}
                className="wi-press flex min-h-11 w-full items-center justify-center gap-1.5 rounded-xl border border-[var(--color-wi-primary)]/40 bg-white px-5 text-[15px] font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary)]/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
              >
                <Plus className="h-5 w-5" aria-hidden="true" />
                Add another class
              </button>
            </div>
          ) : null}
        </div>
      </div>

      {/* The selected list lives behind a tap so removals stay close to the
          agenda instead of a second long list competing with it. */}
      <MobileBottomSheet
        open={viewSheetOpen}
        title="Selected classes"
        onClose={() => setViewSheetOpen(false)}
        restoreFocusRef={viewTriggerRef}
      >
        {selectedEvents.length === 0 ? (
          <p className="py-4 text-center text-[15px] text-[var(--color-wi-text-light)]">No classes selected.</p>
        ) : (
          <>
            <ul className="space-y-2">
              {selectedEvents.map((event) => (
                <li
                  key={event.key}
                  className="flex items-center gap-3 rounded-xl border border-[var(--color-wi-border)] bg-white px-4 py-3"
                >
                  <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-white">
                    <Check className="h-4 w-4" strokeWidth={3} aria-hidden="true" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[15px] font-semibold text-[var(--color-wi-text)]">{event.label}</span>
                    <span className="block truncate text-[13px] text-[var(--color-wi-text-light)]">
                      {dayLabel(event.dateKey)} · {event.timeLabel}
                    </span>
                  </span>
                  <button
                    type="button"
                    aria-label={`Remove ${event.label}`}
                    onClick={() => removeEvent(event)}
                    className="wi-press min-h-11 shrink-0 rounded-lg px-2 text-[13px] font-semibold text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] active:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                  >
                    Remove
                  </button>
                </li>
              ))}
            </ul>
            <button
              type="button"
              onClick={() => {
                setViewSheetOpen(false);
                if (nextReportableAfterFocus) selectDate(nextReportableAfterFocus.dateKey);
                const reduceMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;
                gridRef.current?.scrollIntoView({ behavior: reduceMotion ? "auto" : "smooth", block: "nearest" });
              }}
              className="wi-press mt-3 flex min-h-11 w-full items-center justify-center gap-1.5 rounded-xl border border-[var(--color-wi-border)] px-4 text-[15px] font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
            >
              <Plus className="h-5 w-5" aria-hidden="true" />
              Add another class
            </button>
          </>
        )}
      </MobileBottomSheet>

      {/* Undo lives where the thumb is: pinned to the bottom of the scrolling
          main, above the footer — never at the top of a screen you scrolled
          away from. */}
      {removedToasts.length > 0 ? (
        <div className="absence-undo-stack mt-6" aria-live="polite">
          {removedToasts.map((toast) => (
            <div
              key={toast.key}
              role="status"
              className="absence-undo-toast mx-auto flex w-full max-w-xl items-center justify-between gap-3 rounded-2xl border border-[var(--color-wi-border)] bg-white px-4 py-3 shadow-[0_0.5rem_1.75rem_rgb(15_23_42/0.16)]"
            >
              <p className="min-w-0 text-[14px] leading-snug text-[var(--color-wi-text)]">
                <span className="font-semibold">{toast.event.label}</span> removed
              </p>
              <div className="flex shrink-0 items-center gap-1">
                <button
                  type="button"
                  onClick={() => undoRemovedToast(toast.key)}
                  className="wi-press min-h-11 rounded-lg px-2.5 text-[14px] font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary)]/5 active:bg-[var(--color-wi-primary)]/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                >
                  Undo
                </button>
                <button
                  type="button"
                  onClick={() => dismissRemovedToast(toast.key)}
                  aria-label={`Dismiss ${toast.event.label} removal notice`}
                  className="wi-press flex min-h-11 min-w-11 items-center justify-center rounded-lg text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                >
                  <span aria-hidden="true">×</span>
                </button>
              </div>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

export { dayLabel };
