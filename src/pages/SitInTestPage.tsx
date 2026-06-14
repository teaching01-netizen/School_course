import { useEffect, useMemo, useState } from "react";
import { apiJson } from "@/api/client";
import PageHeading from "@/components/ui/PageHeading";
import LoadingSkeleton from "@/components/ui/LoadingSkeleton";
import { formatDate, formatTime } from "@/utils/date";
import type {
  SessionsInRangeResponse,
  StudentLookupResponse,
  SubjectSessions,
  StudentLookupSubject,
} from "@/types";

// ─── Sit-in helper types (mirrors AbsenceForm.tsx) ───

type SitInAvailableSession = NonNullable<NonNullable<SubjectSessions["sit_in"]>["available_sessions"]>[number];
type SitInCourse = NonNullable<SubjectSessions["sit_in"]>["sit_in_course"];

const MERGED_SESSION_ID_SEPARATOR = "|";
const INSTITUTE_TIME_ZONE = "Asia/Bangkok";

// ─── Mirror helpers from AbsenceForm.tsx ───

function normalizeLookupWcode(input: string): string {
  const trimmed = input.trim();
  if (!trimmed) return "";
  return trimmed[0]?.toLowerCase() === "w" ? `W${trimmed.slice(1)}` : trimmed;
}

function instituteDateKey(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value.slice(0, 10);
  const parts = new Intl.DateTimeFormat("en-GB", {
    timeZone: INSTITUTE_TIME_ZONE,
    year: "numeric", month: "2-digit", day: "2-digit",
  }).formatToParts(date);
  const part = (t: string) => parts.find((p) => p.type === t)?.value ?? "";
  return `${part("year")}-${part("month")}-${part("day")}`;
}

function dayKey(item: { id?: string; start_at: string; end_at: string; date?: string }): string {
  return item.date ?? instituteDateKey(item.start_at);
}

function sortByStart<T extends { start_at: string }>(items: T[]): T[] {
  return items.slice().sort((a, b) => new Date(a.start_at).getTime() - new Date(b.start_at).getTime());
}

type MergedDayRange = { date: string; start_at: string; end_at: string };
type DayRangeGroup<T extends { id?: string; start_at: string; end_at: string; date?: string }> =
  MergedDayRange & { id: string; items: T[] };

function mergeRanges(ranges: { start_at: string; end_at: string }[]): { start_at: string; end_at: string } {
  let start = ranges[0].start_at;
  let end = ranges[0].end_at;
  for (const r of ranges) {
    if (new Date(r.start_at).getTime() < new Date(start).getTime()) start = r.start_at;
    if (new Date(r.end_at).getTime() > new Date(end).getTime()) end = r.end_at;
  }
  return { start_at: start, end_at: end };
}

function groupByDay<T extends { id?: string; start_at: string; end_at: string; date?: string }>(
  items: T[],
): DayRangeGroup<T>[] {
  const byDay = new Map<string, T[]>();
  for (const item of sortByStart(items)) {
    const key = dayKey(item);
    byDay.set(key, [...(byDay.get(key) ?? []), item]);
  }
  return [...byDay.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([date, dayItems]) => {
      const sorted = sortByStart(dayItems);
      const merged = mergeRanges(sorted);
      const id = sorted.map((item) => item.id ?? `${item.start_at}-${item.end_at}`).join(MERGED_SESSION_ID_SEPARATOR);
      return { id, date, start_at: merged.start_at, end_at: merged.end_at, items: sorted };
    });
}

function sitInForMissedSession(group: SubjectSessions, missedSessionId: string) {
  return group.sit_in?.sit_in_by_missed_session?.[missedSessionId] ?? group.sit_in;
}

function groupWithSitInForMissedSession(group: SubjectSessions, missedSessionId: string): SubjectSessions {
  const sitIn = sitInForMissedSession(group, missedSessionId);
  if (!sitIn || sitIn === group.sit_in) return group;
  return { ...group, sit_in: sitIn };
}

function isDayGroupSelected(
  group: DayRangeGroup<{ id: string; start_at: string; end_at: string; date?: string }>,
  selected: Set<string>,
): boolean {
  return group.items.every((session) => selected.has(session.id));
}

function countSelectedSessions(sessions: SubjectSessions[], selected: Set<string>): number {
  return sessions.reduce(
    (total, group) =>
      total + groupByDay(group.sessions).filter((g) => isDayGroupSelected(g, selected)).length,
    0,
  );
}

function firstPriorityLevel(group: SubjectSessions): number {
  const priorities = group.sit_in?.priorities ?? [];
  if (priorities.length === 0) return 1;
  return Math.min(...priorities.map((p) => p.level));
}

function nextPriorityLevel(group: SubjectSessions, currentLevel: number): number | null {
  const levels = [...new Set((group.sit_in?.priorities ?? []).map((p) => p.level))]
    .filter((l) => l > currentLevel)
    .sort((a, b) => a - b);
  return levels[0] ?? null;
}

function previousPriorityLevel(group: SubjectSessions, currentLevel: number): number | null {
  const levels = [...new Set((group.sit_in?.priorities ?? []).map((p) => p.level))]
    .filter((l) => l < currentLevel)
    .sort((a, b) => b - a);
  return levels[0] ?? null;
}

function hasPriorityLevel(group: SubjectSessions, level: number): boolean {
  return (group.sit_in?.priorities ?? []).some((p) => p.level === level);
}

function prioritiesForLevel(group: SubjectSessions, level: number) {
  return (group.sit_in?.priorities ?? []).filter((p) => p.level === level);
}

function hasServerPriorityReveal(group: SubjectSessions): boolean {
  return group.sit_in?.current_priority_level !== undefined || group.sit_in?.has_next_priority !== undefined;
}

function priorityOrdinal(level: number): string {
  const mod100 = level % 100;
  if (mod100 >= 11 && mod100 <= 13) return `${level}th`;
  switch (level % 10) {
    case 1: return `${level}st`;
    case 2: return `${level}nd`;
    case 3: return `${level}rd`;
    default: return `${level}th`;
  }
}

function resolveSitInSubjectName(sitInCourse: SitInCourse, allSubjects: SubjectSessions[]): string | undefined {
  return sitInCourse?.subject_name?.trim() ||
    allSubjects.find((s) => s.course_id === sitInCourse?.id)?.subject_name?.trim();
}

function getSitInCourseDisplayName(
  sitInCourse: SitInCourse,
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
) {
  return (
    resolveSitInSubjectName(sitInCourse, allSubjects) ||
    sitInCourse?.name?.trim() ||
    sitInCourse?.subject_code?.trim() ||
    fallbackSubjectName ||
    sitInCourse?.code?.trim() ||
    ""
  );
}

function getPriorityTargetDisplayName(
  priority: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>[number],
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
) {
  const courseName = getSitInCourseDisplayName(priority.sit_in_course, "", allSubjects);
  if (courseName) return courseName;
  const firstSession = priority.available_sessions?.[0];
  return (
    firstSession?.class_name?.trim() ||
    firstSession?.subject_name?.trim() ||
    firstSession?.course_name?.trim() ||
    firstSession?.subject_code?.trim() ||
    firstSession?.course_code?.trim() ||
    fallbackSubjectName
  );
}

function getCurrentSitInDisplayName(
  sitIn: SubjectSessions["sit_in"],
  currentPriorities: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>,
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
) {
  if (sitIn?.sit_in_method !== "physical") {
    return sitIn?.sit_in_method === "zoom" ? "Zoom" : "To arrange";
  }
  if (currentPriorities.length > 0) {
    const labels = [
      ...new Set(
        currentPriorities
          .map((p) => {
            if (!p.sit_in_course && (p.available_sessions ?? []).length === 0) return "";
            return getPriorityTargetDisplayName(p, fallbackSubjectName, allSubjects).trim();
          })
          .filter(Boolean),
      ),
    ];
    if (labels.length > 0) return labels.join(", ");
    return "Not available";
  }
  return getSitInCourseDisplayName(sitIn.sit_in_course, fallbackSubjectName, allSubjects);
}

function availableSessionsForMissedSessions(
  priority: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>[number],
  missedSessionIds: string[],
) {
  const available = priority.available_sessions ?? [];
  if (!available.some((s) => s.missed_session_id)) return available;
  return available.filter(
    (s) => (s.missed_session_id ? missedSessionIds.includes(s.missed_session_id) : false),
  );
}

function rootAvailableSessionsForMissedSessions(
  sitIn: SubjectSessions["sit_in"],
  missedSessionIds: string[],
) {
  const available = sitIn?.available_sessions ?? [];
  if (!available.some((s) => s.missed_session_id)) return available;
  return available.filter(
    (s) => (s.missed_session_id ? missedSessionIds.includes(s.missed_session_id) : false),
  );
}

function getSitInSessionLabel(
  session: SitInAvailableSession,
  sitInCourse: SitInCourse,
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
) {
  const className =
    resolveSitInSubjectName(sitInCourse, allSubjects) ||
    sitInCourse?.name?.trim() ||
    session.class_name?.trim() ||
    session.subject_name?.trim() ||
    session.course_name?.trim() ||
    sitInCourse?.subject_code?.trim() ||
    session.subject_code?.trim() ||
    session.course_code?.trim() ||
    fallbackSubjectName ||
    sitInCourse?.code?.trim();
  return `${className} — ${formatDate(dayKey(session))} ${formatTime(session.start_at)}-${formatTime(session.end_at)}`;
}

function getSitInSessionGroupLabel(
  sessions: SitInAvailableSession[],
  sitInCourse: SitInCourse,
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
) {
  if (sessions.length === 1) return getSitInSessionLabel(sessions[0], sitInCourse, fallbackSubjectName, allSubjects);
  const first = sessions[0];
  const className =
    resolveSitInSubjectName(sitInCourse, allSubjects) ||
    sitInCourse?.name?.trim() ||
    first.class_name?.trim() ||
    first.subject_name?.trim() ||
    first.course_name?.trim() ||
    sitInCourse?.subject_code?.trim() ||
    first.subject_code?.trim() ||
    first.course_code?.trim() ||
    fallbackSubjectName ||
    sitInCourse?.code?.trim();
  const range = groupByDay(sessions)[0];
  return `${className} — ${formatDate(range.date)} ${formatTime(range.start_at)}-${formatTime(range.end_at)}`;
}

function unavailableSessionsForMissedSession(
  priority: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>[number],
  missedSessionId: string,
) {
  const unavailable = priority.unavailable_sessions ?? [];
  if (!unavailable.some((s) => s.missed_session_id)) return unavailable;
  return unavailable.filter((s) => s.missed_session_id === missedSessionId);
}

// ─── Sit-in Test Page ───

function dateToLocalISO(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

const SIT_IN_METHOD_BADGE: Record<string, { color: string; label: string }> = {
  physical: { color: "bg-blue-100 text-blue-800", label: "Physical" },
  zoom: { color: "bg-purple-100 text-purple-800", label: "Zoom" },
  teacher_case: { color: "bg-amber-100 text-amber-800", label: "Teacher case" },
  none: { color: "bg-gray-100 text-gray-500", label: "None" },
};

type PriorityLevelCache = Record<string, Record<number, SubjectSessions>>;

export default function SitInTestPage() {
  const [wcodeInput, setWcodeInput] = useState("");
  const [lookup, setLookup] = useState<StudentLookupResponse | null>(null);
  const [lookupLoading, setLookupLoading] = useState(false);
  const [lookupError, setLookupError] = useState<string | null>(null);
  const [selectedSubjectIds, setSelectedSubjectIds] = useState<string[]>([]);
  const [sessions, setSessions] = useState<SubjectSessions[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [sessionsError, setSessionsError] = useState<string | null>(null);
  const [selectedSessionIds, setSelectedSessionIds] = useState<Set<string>>(new Set());
  const [, setSitInSelections] = useState<Record<string, string>>({});
  const [priorityLevels, setPriorityLevels] = useState<Record<string, number>>({});
  const [priorityHistory, setPriorityHistory] = useState<PriorityLevelCache>({});
  const [revealingIds, setRevealingIds] = useState<Set<string>>(new Set());

  const lookupName = lookup?.display_name?.trim() || lookup?.nickname?.trim() || lookup?.full_name?.trim() || "";

  const maxDateRange = 30;
  const lookupWindow = useMemo(() => {
    const today = new Date();
    return {
      dateFrom: dateToLocalISO(today),
      dateTo: dateToLocalISO(new Date(today.getTime() + maxDateRange * 24 * 60 * 60 * 1000)),
    };
  }, []);

  async function handleLookup() {
    setLookupError(null);
    setLookup(null);
    const cleaned = normalizeLookupWcode(wcodeInput);
    if (!cleaned) { setLookupError("Enter a W-Code."); return; }
    try {
      setLookupLoading(true);
      const response = await apiJson<StudentLookupResponse>(
        `/api/v1/absences/student-lookup?wcode=${encodeURIComponent(cleaned)}`,
        { method: "GET" },
      );
      setLookup(response);
      setWcodeInput(cleaned);
      resetSelection();
    } catch (err) {
      setLookupError(err instanceof Error ? err.message : "Lookup failed");
    } finally {
      setLookupLoading(false);
    }
  }

  function resetSelection() {
    setSelectedSubjectIds([]);
    setSessions([]);
    setSelectedSessionIds(new Set());
    setSitInSelections({});
    setPriorityLevels({});
    setPriorityHistory({});
    setRevealingIds(new Set());
  }

  function toggleSubject(subjectId: string) {
    setSelectedSubjectIds((prev) =>
      prev.includes(subjectId) ? prev.filter((id) => id !== subjectId) : [...prev, subjectId],
    );
  }

  function toggleSession(sessionIds: string[]) {
    setSelectedSessionIds((prev) => {
      const allSelected = sessionIds.every((id) => prev.has(id));
      if (allSelected) {
        const next = new Set(prev);
        for (const id of sessionIds) next.delete(id);
        return next;
      }
      if (selectedSessionCount >= maxSessions) return prev;
      const next = new Set(prev);
      for (const id of sessionIds) next.add(id);
      return next;
    });
  }

  // Fetch sessions when lookup or subject selections change
  useEffect(() => {
    if (!lookup || selectedSubjectIds.length === 0) {
      setSessions([]);
      return;
    }
    const controller = new AbortController();
    setSessionsLoading(true);
    setSessionsError(null);

    const params = new URLSearchParams({
      wcode: lookup.wcode,
      date_from: lookupWindow.dateFrom,
      date_to: lookupWindow.dateTo,
    });

    apiJson<SessionsInRangeResponse>(
      `/api/v1/absences/sessions-in-range?${params.toString()}`,
      { method: "GET", signal: controller.signal },
    )
      .then((data) => {
        if (!controller.signal.aborted) setSessions(data.subjects);
      })
      .catch((err) => {
        if (controller.signal.aborted) return;
        setSessions([]);
        setSessionsError(err instanceof Error ? err.message : "Failed to load sessions");
      })
      .finally(() => {
        if (!controller.signal.aborted) setSessionsLoading(false);
      });

    return () => controller.abort();
  }, [lookup, selectedSubjectIds, lookupWindow]);

  async function handleNotAvailable(group: SubjectSessions, sessionId: string) {
    const currentLevel = priorityLevels[sessionId] || group.sit_in?.current_priority_level || firstPriorityLevel(group);
    if (lookup && hasServerPriorityReveal(group)) {
      setRevealingIds((prev) => new Set(prev).add(sessionId));
      setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
      setPriorityHistory((prev) => ({
        ...prev,
        [sessionId]: { ...(prev[sessionId] ?? {}), [currentLevel]: group },
      }));
      try {
        const params = new URLSearchParams({
          wcode: lookup.wcode,
          date_from: lookupWindow.dateFrom,
          date_to: lookupWindow.dateTo,
          course_ids: group.course_id,
          sat_verbal_after_priority: String(currentLevel),
        });
        const data = await apiJson<SessionsInRangeResponse>(
          `/api/v1/absences/sessions-in-range?${params.toString()}`,
          { method: "GET" },
        );
        const updatedGroup = data.subjects.find((s) => s.course_id === group.course_id);
        if (!updatedGroup) return;
        const updatedSessionGroup = groupWithSitInForMissedSession(updatedGroup, sessionId);
        const updatedLevel = updatedSessionGroup.sit_in?.current_priority_level ?? firstPriorityLevel(updatedSessionGroup);
        setPriorityLevels((prev) => ({ ...prev, [sessionId]: updatedLevel }));
        setPriorityHistory((prev) => ({
          ...prev,
          [sessionId]: { ...(prev[sessionId] ?? {}), [updatedLevel]: updatedSessionGroup },
        }));
      } catch {
        // silent
      } finally {
        setRevealingIds((prev) => { const n = new Set(prev); n.delete(sessionId); return n; });
      }
      return;
    }
    const nextLevel = nextPriorityLevel(group, currentLevel);
    if (nextLevel == null) return;
    setPriorityLevels((prev) => ({ ...prev, [sessionId]: nextLevel }));
    setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
  }

  function handlePreviousPriority(group: SubjectSessions, sessionId: string) {
    const currentLevel = priorityLevels[sessionId] || group.sit_in?.current_priority_level || firstPriorityLevel(group);
    if (hasServerPriorityReveal(group)) {
      const history = priorityHistory[sessionId] ?? {};
      const previousLevel = Object.keys(history)
        .map(Number)
        .filter((l) => l < currentLevel)
        .sort((a, b) => b - a)[0];
      if (previousLevel === undefined) return;
      setPriorityLevels((prev) => ({ ...prev, [sessionId]: previousLevel }));
      setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
      return;
    }
    const prevLevel = previousPriorityLevel(group, currentLevel);
    if (prevLevel == null) return;
    setPriorityLevels((prev) => ({ ...prev, [sessionId]: prevLevel }));
    setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
  }

  const maxSessions = 10;
  const selectedSessionCount = useMemo(
    () => countSelectedSessions(sessions, selectedSessionIds),
    [sessions, selectedSessionIds],
  );
  const atMaxSessions = selectedSessionCount >= maxSessions;

  return (
    <div className="max-w-4xl">
      <div className="flex items-center gap-3 mb-6">
        <PageHeading>Sit-In Rule Test</PageHeading>
      </div>

      {/* Step 1: WCode Lookup */}
      <section className="rounded-lg border border-gray-200 bg-white p-5 mb-6">
        <h2 className="text-sm font-semibold text-gray-800 mb-3">1. Lookup Student</h2>
        <div className="flex gap-3">
          <input
            value={wcodeInput}
            onChange={(e) => setWcodeInput(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") void handleLookup(); }}
            placeholder="e.g. W250389"
            className="min-h-[44px] flex-1 rounded-lg border border-gray-300 px-4 text-sm focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20"
          />
          <button
            onClick={() => void handleLookup()}
            disabled={lookupLoading}
            className="min-h-[44px] rounded-lg bg-blue-600 px-5 text-sm font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {lookupLoading ? "..." : "Search"}
          </button>
        </div>
        {lookupError && <p className="text-sm text-red-600 mt-2">{lookupError}</p>}
        {lookup && (
          <div className="mt-4 rounded-lg bg-gray-50 border border-gray-200 p-4">
            <p className="text-sm font-semibold text-gray-800">{lookupName || lookup.full_name}</p>
            <p className="text-xs font-mono text-gray-500 mt-0.5">{lookup.wcode}</p>
          </div>
        )}
      </section>

      {/* Step 2: Select Subjects */}
      {lookup && lookup.subjects.length > 0 && (
        <section className="rounded-lg border border-gray-200 bg-white p-5 mb-6">
          <h2 className="text-sm font-semibold text-gray-800 mb-3">2. Select Subject(s)</h2>
          <div className="space-y-2">
            {lookup.subjects.map((subject: StudentLookupSubject) => (
              <label key={subject.id} className="flex items-center gap-3 cursor-pointer">
                <input
                  type="checkbox"
                  checked={selectedSubjectIds.includes(subject.id)}
                  onChange={() => toggleSubject(subject.id)}
                  className="h-4 w-4 rounded border-gray-300 text-blue-600"
                />
                <span className="text-sm text-gray-800">{subject.name}</span>
                <span className="text-xs text-gray-400 font-mono">{subject.code}</span>
              </label>
            ))}
          </div>
        </section>
      )}

      {/* Step 3: Sessions + Sit-In Results */}
      {selectedSubjectIds.length > 0 && (
        <section className="mb-6">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-semibold text-gray-800">3. Sessions &amp; Sit-In Results</h2>
            <span className="text-xs text-gray-500">{selectedSessionCount}/{maxSessions} selected</span>
          </div>

          {sessionsLoading && <LoadingSkeleton type="table" lines={3} />}
          {sessionsError && <p className="text-sm text-red-600">{sessionsError}</p>}

          {!sessionsLoading && sessions.length === 0 && selectedSubjectIds.length > 0 && (
            <p className="text-sm text-gray-400">No sessions found for selected subjects.</p>
          )}

          <div className="space-y-4">
            {sessions
              .filter((g) => selectedSubjectIds.includes(g.subject_id))
              .map((group) => {
                const groupLabel = group.subject_name?.trim() || group.course_name?.trim() || group.course_code;
                const sessionGroups = groupByDay(group.sessions);
                return (
                  <div key={group.course_id} className="rounded-lg border border-gray-200 bg-white overflow-hidden">
                    {/* Subject header with sit-in rule info */}
                    <div className="border-b border-gray-100 bg-gray-50 px-4 py-3">
                      <div className="flex items-center justify-between">
                        <span className="text-sm font-semibold text-gray-800">{groupLabel}</span>
                        {group.sit_in && (
                          <span
                            className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${
                              SIT_IN_METHOD_BADGE[group.sit_in.sit_in_method]?.color ?? "bg-gray-100 text-gray-500"
                            }`}
                          >
                            {SIT_IN_METHOD_BADGE[group.sit_in.sit_in_method]?.label ?? group.sit_in.sit_in_method}
                          </span>
                        )}
                      </div>
                      {/* Rule metadata */}
                      {group.sit_in && group.sit_in.rule_name && (
                        <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-gray-500">
                          <span>Rule: <span className="font-medium text-gray-700">{group.sit_in.rule_name}</span></span>
                          <span>Type: <span className="font-medium text-gray-700">{group.sit_in.rule_type}</span></span>
                          {group.sit_in.current_priority_level !== undefined && (
                            <span>Current priority: <span className="font-medium text-gray-700">{group.sit_in.current_priority_level}</span></span>
                          )}
                        </div>
                      )}
                    </div>

                    {/* Sessions */}
                    <div className="divide-y divide-gray-50">
                      {sessionGroups.map((dayGroup) => {
                        const sessionIds = dayGroup.items.map((item) => item.id);
                        const selected = isDayGroupSelected(dayGroup, selectedSessionIds);
                        const session = dayGroup.items[0];
                        const sessionGroup = groupWithSitInForMissedSession(group, session.id);
                        const baseSitIn = sessionGroup.sit_in;
                        const baseLevel = baseSitIn?.current_priority_level || firstPriorityLevel(sessionGroup);
                        const requestedLevel = baseSitIn
                          ? priorityLevels[session.id] || baseLevel
                          : firstPriorityLevel(sessionGroup);
                        const requestedPriorityGroup = priorityHistory[session.id]?.[requestedLevel] ?? sessionGroup;
                        const currentLevel = hasPriorityLevel(requestedPriorityGroup, requestedLevel)
                          ? requestedLevel
                          : baseLevel;
                        const priorityGroup = priorityHistory[session.id]?.[currentLevel] ?? sessionGroup;
                        const sitIn = priorityGroup.sit_in;
                        const hasPriorities = Boolean(sitIn?.priorities && sitIn.priorities.length > 0);
                        const currentPriorities = hasPriorities
                          ? prioritiesForLevel(priorityGroup, currentLevel)
                          : [];

                        return (
                          <div key={dayGroup.id} className="px-4 py-3">
                            <label className="flex items-start gap-3 cursor-pointer">
                              <input
                                type="checkbox"
                                checked={selected}
                                disabled={!selected && atMaxSessions}
                                onChange={() => toggleSession(sessionIds)}
                                className="mt-0.5 h-4 w-4 rounded border-gray-300 text-blue-600 disabled:opacity-50"
                              />
                              <div className="min-w-0 flex-1">
                                <span className="text-sm font-medium text-gray-800">
                                  {formatDate(dayGroup.date)} {formatTime(dayGroup.start_at)}–{formatTime(dayGroup.end_at)}
                                </span>
                              </div>
                            </label>

                            {/* Sit-in detail card */}
                            {selected && sitIn && (
                              <div className="mt-3 ml-7 rounded-lg border border-gray-200 bg-gray-50/50 p-4">
                                {renderSitInDetail(
                                  sitIn,
                                  hasPriorities,
                                  currentPriorities,
                                  currentLevel,
                                  priorityGroup,
                                  session,
                                  sessionIds,
                                  groupLabel,
                                  sessions,
                                  priorityHistory,
                                  revealingIds,
                                  (g, s) => void handleNotAvailable(g, s),
                                  (g, s) => handlePreviousPriority(g, s),
                                )}
                              </div>
                            )}

                            {selected && !sitIn && (
                              <div className="mt-3 ml-7 text-sm text-gray-400">
                                No sit-in information available.
                              </div>
                            )}
                          </div>
                        );
                      })}
                    </div>
                  </div>
                );
              })}
          </div>
        </section>
      )}

      {!lookup && (
        <div className="rounded-lg border border-dashed border-gray-300 bg-gray-50 p-8 text-center text-sm text-gray-400">
          Enter a W-Code above to start testing sit-in rules.
        </div>
      )}
    </div>
  );
}

function renderSitInDetail(
  sitIn: NonNullable<SubjectSessions["sit_in"]>,
  hasPriorities: boolean,
  currentPriorities: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>,
  currentLevel: number,
  priorityGroup: SubjectSessions,
  session: SubjectSessions["sessions"][number],
  sessionIds: string[],
  groupLabel: string,
  allSubjects: SubjectSessions[],
  priorityHistory: PriorityLevelCache,
  revealingIds: Set<string>,
  onNotAvailable: (group: SubjectSessions, sessionId: string) => void,
  onPreviousPriority: (group: SubjectSessions, sessionId: string) => void,
) {
  if (sitIn.sit_in_method === "zoom") {
    return (
      <div className="flex items-center gap-2 text-sm text-purple-700">
        <span className="font-medium">Zoom</span>
        <span className="text-gray-500">— Online make-up, no class selection needed</span>
      </div>
    );
  }
  if (sitIn.sit_in_method === "teacher_case") {
    return (
      <div className="text-sm text-amber-700">
        <span className="font-medium">To arrange</span>
        <span className="text-gray-500"> — Staff will contact the student</span>
      </div>
    );
  }
  if (sitIn.sit_in_method !== "physical") {
    return (
      <div className="text-sm text-gray-500">
        <span className="font-medium">To arrange</span>
      </div>
    );
  }

  if (hasPriorities && currentPriorities.length === 0) {
    return (
      <div className="text-sm text-gray-500">
        <p className="font-medium">No more options available</p>
        <p className="text-xs text-gray-400 mt-0.5">Staff will contact the student to arrange a make-up class.</p>
      </div>
    );
  }

  if (hasPriorities) {
    const currentPriority = currentPriorities[0];
    const nextLevel = nextPriorityLevel(priorityGroup, currentLevel);
    const serverReveal = hasServerPriorityReveal(priorityGroup);
    const hasMorePriorities = serverReveal ? Boolean(sitIn.has_next_priority) : nextLevel !== null;
    const hasPreviousPriority = serverReveal
      ? Object.keys(priorityHistory[session.id] ?? {}).some((l) => Number(l) < currentLevel)
      : previousPriorityLevel(priorityGroup, currentLevel) !== null;
    const revealingPriority = revealingIds.has(session.id);
    const currentAvailable = currentPriorities.flatMap((p) =>
      availableSessionsForMissedSessions(p, sessionIds),
    );
    const currentUnavailable = currentPriorities.flatMap((p) =>
      unavailableSessionsForMissedSession(p, session.id).map((u) => ({ ...u, sitInCourse: p.sit_in_course })),
    );

    if (!currentPriority) {
      return (
        <div className="text-sm text-gray-500">
          <p className="font-medium">No more options available</p>
          <p className="text-xs text-gray-400 mt-0.5">Staff will contact the student to arrange a make-up class.</p>
        </div>
      );
    }

    return (
      <div>
        {/* Priority level header */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-amber-100 px-1.5 text-xs font-semibold text-amber-700 ring-1 ring-amber-300">
              {currentLevel}
            </span>
            <span className="text-xs font-semibold uppercase tracking-wide text-amber-700">
              {priorityOrdinal(currentLevel)} choice
            </span>
            <span className="text-sm font-medium text-gray-800">
              {currentPriority.label}
            </span>
          </div>
          <div className="flex gap-1">
            {hasPreviousPriority && (
              <button
                onClick={() => onPreviousPriority(priorityGroup, session.id)}
                disabled={revealingPriority}
                className="rounded-full border border-gray-200 bg-white px-3 py-1 text-xs font-medium text-gray-600 hover:bg-gray-100 disabled:opacity-50"
              >
                &larr; Back
              </button>
            )}
            {hasMorePriorities && (
              <button
                onClick={() => onNotAvailable(priorityGroup, session.id)}
                disabled={revealingPriority}
                className="rounded-full border border-gray-200 bg-white px-3 py-1 text-xs font-semibold text-gray-600 hover:bg-gray-100 disabled:opacity-50"
              >
                {revealingPriority ? "Loading..." : "See other times &rarr;"}
              </button>
            )}
          </div>
        </div>

        {/* Available sessions */}
        {currentAvailable.length > 0 ? (
          <div className="mt-3">
            <p className="text-xs font-medium text-gray-500 mb-1.5">Available make-up sessions:</p>
            <div className="space-y-1">
                  {currentPriorities.flatMap((p) =>
                groupByDay(availableSessionsForMissedSessions(p, sessionIds)).map((optGroup) => (
                  <div
                    key={`${p.sit_in_course?.id ?? "course"}:${optGroup.id}`}
                    className="rounded-md border border-gray-200 bg-white px-3 py-2 text-xs text-gray-700"
                  >
                    {getSitInSessionGroupLabel(optGroup.items, p.sit_in_course, groupLabel, allSubjects)}
                  </div>
                )),
              )}
            </div>
          </div>
        ) : (
          <div className="mt-3 text-sm text-gray-500">
            No available make-up sessions for this priority.
          </div>
        )}

        {/* Unavailable sessions */}
        {currentUnavailable.length > 0 && (
          <div className="mt-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2">
            <p className="text-xs font-semibold text-amber-700">Checked same-number slot (unavailable):</p>
            <ul className="mt-1 space-y-1">
              {currentUnavailable.map((u, i) => {
                const checkedSession = (u as any).session;
                const label = checkedSession
                  ? getSitInSessionLabel(checkedSession, (u as any).sitInCourse, groupLabel, allSubjects)
                  : `Target section class #${u.occurrence_number ?? "?"}`;
                return (
                  <li key={`${(u as any).reason_code}-${i}`} className="text-xs text-amber-700">
                    <span className="font-medium">{label}</span>
                    <span> — {u.reason}</span>
                  </li>
                );
              })}
            </ul>
          </div>
        )}

        {/* Rule metadata */}
        <div className="mt-3 border-t border-gray-200 pt-2 text-xs text-gray-400">
          <span className="font-medium text-gray-500">Type:</span> {sitIn.rule_type ?? "—"}
          {sitIn.has_next_priority !== undefined && (
            <span className="ml-3">
              <span className="font-medium text-gray-500">Has next:</span> {String(sitIn.has_next_priority)}
            </span>
          )}
        </div>
      </div>
    );
  }

  // No priorities — simple sit-in course display
  const sitInAvailable = rootAvailableSessionsForMissedSessions(sitIn, sessionIds);
  const sitInClassLabel = getCurrentSitInDisplayName(sitIn, [], groupLabel, allSubjects);

  return (
    <div>
      <p className="text-xs font-semibold text-amber-700 uppercase tracking-wider mb-2">Pick a make-up class</p>
      <p className="text-xs text-gray-500 mb-2">Sit-in class: {sitInClassLabel}</p>

      {sitInAvailable.length > 0 ? (
        <div className="space-y-1">
          {groupByDay(sitInAvailable).map((optGroup) => (
            <div
              key={optGroup.id}
              className="rounded-md border border-gray-200 bg-white px-3 py-2 text-xs text-gray-700"
            >
              {getSitInSessionGroupLabel(optGroup.items, sitIn.sit_in_course, groupLabel, allSubjects)}
            </div>
          ))}
        </div>
      ) : (
        <p className="text-xs text-gray-400">No available sessions listed.</p>
      )}
    </div>
  );
}
