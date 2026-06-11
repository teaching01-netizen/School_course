import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { motion } from "framer-motion";
import { ChevronLeft } from "lucide-react";
import { useNavigate } from "react-router-dom";
import clsx from "clsx";
import { apiJson, newIdempotencyKey } from "@/api/client";
import LoadingSkeleton from "@/components/ui/LoadingSkeleton";
import StepIndicator from "@/components/absences/StepIndicator";
import SubjectCard from "@/components/absences/SubjectCard";
import StickyFooter from "@/components/absences/StickyFooter";
import StepCoverVerification from "@/components/absences/StepCoverVerification";
import { useToast } from "@/hooks/useToast";
import { useConnectivity } from "@/hooks/useConnectivity";
import { useOtp } from "@/hooks/useOtp";
import { formatDate, formatTime } from "@/utils/date";
import type {
  AbsenceFormConfig,
  AbsenceNotificationsSettings,
  AdminContactSettings,
  ManagedAbsence,
  SessionsInRangeResponse,
  SubjectSessions,
  StudentLookupResponse,
} from "@/types";

type StepIndex = 0 | 1 | 2;
type AbsenceBatchCreateItem = {
  subject_id: string;
  course_id: string;
  date_from: string;
  date_to: string;
  reason?: string;
  sit_in_method?: string;
  sit_in_course_id?: string;
  missed_session_ids: string[];
  sit_in_session_ids: string[];
};
type AbsenceBatchCreateResponse = {
  items: ManagedAbsence[];
};

const SESSION_STORAGE_KEY = "warwick-absence-form-state-v3";
const VERIFICATION_STORAGE_KEY = `${SESSION_STORAGE_KEY}:parent-verification`;

const DEFAULT_NOTIFICATIONS: AbsenceNotificationsSettings = {
  sms_parent_enabled: true,
  sms_parent_template: "",
  sms_success_template: "",
  allow_submit_without_otp: false,
};

const DEFAULT_ADMIN_CONTACT: AdminContactSettings = {
  email: "",
  phone: "",
  hours: "",
};

const DEFAULT_CONFIG: AbsenceFormConfig = {
  form: {
    max_date_range_days: 30,
    require_reason: false,
    reason_categories: [],
    allow_free_text_reason: true,
    intro_text: "",
    confirmation_text: "",
  },
  sit_in: {
    auto_resolve_enabled: true,
    zoom_description: "Zoom session - no physical class attendance required.",
    max_sessions_per_absence: 10,
  },
  notifications: DEFAULT_NOTIFICATIONS,
  admin_contact: DEFAULT_ADMIN_CONTACT,
};

function dateToLocalISO(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

function daysBetween(from: string, to: string): number {
  return Math.round(
    (new Date(`${to}T00:00:00`).getTime() - new Date(`${from}T00:00:00`).getTime()) /
      (1000 * 60 * 60 * 24),
  );
}

function normalizeLookupWcode(input: string): string {
  const trimmed = input.trim();
  if (!trimmed) return "";
  return trimmed[0]?.toLowerCase() === "w" ? `W${trimmed.slice(1)}` : trimmed;
}

function maskPhone(phone?: string | null): string {
  if (!phone) return "";
  const digits = phone.replace(/\D/g, "");
  if (digits.length <= 4) return phone;
  return `${digits.slice(0, 3)} *** ${digits.slice(-3)}`;
}

function countSelectedSessions(groups: SubjectSessions[], selected: Set<string>): number {
  return groups.reduce(
    (total, group) => total + group.sessions.filter((session) => selected.has(session.id)).length,
    0,
  );
}

function getStudentDisplayName(lookup: StudentLookupResponse | null) {
  return lookup?.display_name?.trim() || lookup?.nickname?.trim() || lookup?.full_name?.trim() || "";
}

function getSelectedSessionsForGroup(group: SubjectSessions, selected: Set<string>) {
  return group.sessions
    .filter((session) => selected.has(session.id))
    .slice()
    .sort((a, b) => a.start_at.localeCompare(b.start_at));
}

type SitInAvailableSession = NonNullable<NonNullable<SubjectSessions["sit_in"]>["available_sessions"]>[number];
type SitInCourse = NonNullable<SubjectSessions["sit_in"]>["sit_in_course"];

function resolveSitInSubjectName(sitInCourse: SitInCourse, allSubjects: SubjectSessions[]): string | undefined {
  return sitInCourse?.subject_name?.trim() || allSubjects.find(s => s.course_id === sitInCourse?.id)?.subject_name?.trim();
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
          .map((priority) => {
            if (!priority.sit_in_course && (priority.available_sessions ?? []).length === 0) return "";
            return getPriorityTargetDisplayName(priority, fallbackSubjectName, allSubjects).trim();
          })
          .filter(Boolean),
      ),
    ];
    if (labels.length > 0) return labels.join(", ");
    return "Not available";
  }
  return getSitInCourseDisplayName(sitIn.sit_in_course, fallbackSubjectName, allSubjects);
}

function sessionsInRangePath(
  wcode: string,
  dateFrom: string,
  dateTo: string,
  options?: { courseIds?: string[]; satVerbalAfterPriority?: number },
): string {
  const params = new URLSearchParams({ wcode, date_from: dateFrom, date_to: dateTo });
  if (options?.courseIds && options.courseIds.length > 0) {
    params.set("course_ids", options.courseIds.join(","));
  }
  if (options?.satVerbalAfterPriority !== undefined) {
    params.set("sat_verbal_after_priority", String(options.satVerbalAfterPriority));
  }
  return `/api/v1/absences/sessions-in-range?${params.toString()}`;
}

function sitInForMissedSession(group: SubjectSessions, missedSessionId: string) {
  return group.sit_in?.sit_in_by_missed_session?.[missedSessionId] ?? group.sit_in;
}

function groupWithSitInForMissedSession(group: SubjectSessions, missedSessionId: string): SubjectSessions {
  const sitIn = sitInForMissedSession(group, missedSessionId);
  if (!sitIn || sitIn === group.sit_in) return group;
  return { ...group, sit_in: sitIn };
}

function availableSessionsForMissedSession(
  priority: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>[number],
  missedSessionId: string,
) {
  const available = priority.available_sessions ?? [];
  if (!available.some((session) => session.missed_session_id)) return available;
  return available.filter((session) => session.missed_session_id === missedSessionId);
}

function unavailableSessionsForMissedSession(
  priority: NonNullable<NonNullable<SubjectSessions["sit_in"]>["priorities"]>[number],
  missedSessionId: string,
) {
  const unavailable = priority.unavailable_sessions ?? [];
  if (!unavailable.some((session) => session.missed_session_id)) return unavailable;
  return unavailable.filter((session) => session.missed_session_id === missedSessionId);
}

function rootAvailableSessionsForMissedSession(
  sitIn: SubjectSessions["sit_in"],
  missedSessionId: string,
) {
  const available = sitIn?.available_sessions ?? [];
  if (!available.some((session) => session.missed_session_id)) return available;
  return available.filter((session) => session.missed_session_id === missedSessionId);
}

function hasServerPriorityReveal(group: SubjectSessions): boolean {
  return group.sit_in?.current_priority_level !== undefined || group.sit_in?.has_next_priority !== undefined;
}

function firstPriorityLevel(group: SubjectSessions): number {
  const priorities = group.sit_in?.priorities ?? [];
  if (priorities.length === 0) return 1;
  return Math.min(...priorities.map((priority) => priority.level));
}

function hasPriorityLevel(group: SubjectSessions, level: number): boolean {
  return (group.sit_in?.priorities ?? []).some((priority) => priority.level === level);
}

function nextPriorityLevel(group: SubjectSessions, currentLevel: number): number | null {
  const levels = [...new Set((group.sit_in?.priorities ?? []).map((priority) => priority.level))]
    .filter((level) => level > currentLevel)
    .sort((a, b) => a - b);
  return levels[0] ?? null;
}

function previousPriorityLevel(group: SubjectSessions, currentLevel: number): number | null {
  const levels = [...new Set((group.sit_in?.priorities ?? []).map((priority) => priority.level))]
    .filter((level) => level < currentLevel)
    .sort((a, b) => b - a);
  return levels[0] ?? null;
}

function prioritiesForLevel(group: SubjectSessions, level: number) {
  return (group.sit_in?.priorities ?? []).filter((priority) => priority.level === level);
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

function getReviewSitInLabel(
  missedSession: { id: string },
  group: SubjectSessions,
  sitInSelections: Record<string, string>,
  priorityLevels: Record<string, number>,
  priorityHistory: Record<string, Record<number, SubjectSessions>>,
  allSubjects: SubjectSessions[],
): string {
  const requestedLevel = priorityLevels[missedSession.id];
  const sitInGroup = requestedLevel
    ? priorityHistory[missedSession.id]?.[requestedLevel] ?? groupWithSitInForMissedSession(group, missedSession.id)
    : groupWithSitInForMissedSession(group, missedSession.id);
  const sitIn = sitInGroup.sit_in;
  if (!sitIn) return "To arrange";
  if (sitIn.sit_in_method === "zoom") return "Zoom";
  if (sitIn.sit_in_method === "teacher_case") return "To arrange";
  if (sitIn.sit_in_method !== "physical") return "To arrange";
  const sitInSessionId = sitInSelections[missedSession.id];
  if (!sitInSessionId) return "Not yet selected";
  const priorities = sitIn.priorities ?? [];
  const groupLabel = group.subject_name?.trim() || group.course_name?.trim() || group.course_code;
  const rootMatch = rootAvailableSessionsForMissedSession(sitIn, missedSession.id).find((s) => s.id === sitInSessionId);
  if (rootMatch) {
    return getSitInSessionLabel(rootMatch, sitIn.sit_in_course, groupLabel, allSubjects);
  }
  for (const p of priorities) {
    const available = availableSessionsForMissedSession(p, missedSession.id);
    const match = available.find((s) => s.id === sitInSessionId);
    if (match) {
      return getSitInSessionLabel(match, p.sit_in_course, groupLabel, allSubjects);
    }
  }
  return "Make-up class selected";
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
  return `${className} — ${formatDate(session.start_at.slice(0, 10))} ${formatTime(session.start_at)}-${formatTime(session.end_at)}`;
}

function selectedSitInCourseIDForGroup(
  group: SubjectSessions,
  selectedMissedSessionIds: string[],
  sitInSelections: Record<string, string>,
): string | null {
  if (group.sit_in?.sit_in_method !== "physical" && !group.sit_in?.sit_in_by_missed_session) {
    return group.sit_in?.sit_in_course?.id?.trim() || group.course_id.trim() || null;
  }
  const courseIDs = new Set<string>();
  for (const missedSessionID of selectedMissedSessionIds) {
    const sitIn = sitInForMissedSession(group, missedSessionID);
    if (sitIn?.sit_in_method !== "physical") {
      const courseID = sitIn?.sit_in_course?.id?.trim() || group.course_id.trim();
      if (courseID) courseIDs.add(courseID);
      continue;
    }
    const priorities = sitIn.priorities ?? [];
    if (priorities.length === 0) {
      const courseID = sitIn.sit_in_course?.id?.trim() || group.course_id.trim();
      if (courseID) courseIDs.add(courseID);
      continue;
    }
    const sitInSessionID = sitInSelections[missedSessionID];
    if (!sitInSessionID) continue;
    for (const priority of priorities) {
      const hasSession = (priority.available_sessions ?? []).some((session) => session.id === sitInSessionID);
      const courseID = priority.sit_in_course?.id?.trim();
      if (hasSession && courseID) {
        courseIDs.add(courseID);
        break;
      }
    }
  }
  if (courseIDs.size === 1) return [...courseIDs][0];
  if (courseIDs.size === 0) return group.sit_in?.sit_in_course?.id?.trim() || group.course_id.trim() || null;
  return null;
}

function formatBatchAbsenceSummary(absence: ManagedAbsence) {
  const className = absence.subject_name?.trim() || absence.course_name?.trim() || absence.course_code?.trim() || "";
  const dates = getAbsenceSessionDateLabels(absence);
  if (!className && !dates) return "To arrange";
  if (!dates) return className || "To arrange";
  if (!className) return dates;
  return `${className} (${dates})`;
}

function getAbsenceSessionDateLabels(absence: ManagedAbsence) {
  const sessions = absence.missed_sessions ?? [];
  const dates = new Set<string>();
  for (const session of sessions) {
    if (session.start_at) dates.add(session.start_at.slice(0, 10));
  }
  const labels = [...dates].sort().map((date) => formatDate(date));
  if (labels.length > 0) return labels.join(", ");
  if (absence.date_from && absence.date_to) {
    if (absence.date_from === absence.date_to) return formatDate(absence.date_from);
    return `${formatDate(absence.date_from)} - ${formatDate(absence.date_to)}`;
  }
  return "";
}

function formatBatchSitInSummary(absence: ManagedAbsence) {
  const method = absence.sit_in_method?.trim();
  if (method === "zoom") return "Zoom";
  const sessions = absence.sit_ins ?? [];
  const sessionLabels = sessions
    .filter((session) => session.start_at)
    .map((session) => `${formatDate(session.start_at.slice(0, 10))} ${formatTime(session.start_at)}-${formatTime(session.end_at)}`);
  if (method !== "physical") {
    return sessionLabels.length > 0 ? `To arrange (${sessionLabels.join(", ")})` : "To arrange";
  }
  if (sessionLabels.length > 0) {
    const className = absence.sit_in_subject_name?.trim() || absence.sit_in_course_name?.trim() || absence.sit_in_course_code?.trim() || "Make-up class";
    return `${className} (${sessionLabels.join(", ")})`;
  }
  const label = absence.sit_in_subject_name?.trim() || absence.sit_in_course_name?.trim() || absence.sit_in_course_code?.trim();
  if (label) return label;
  return "To arrange";
}

export default function AbsenceForm() {
  const navigate = useNavigate();
  const { addToast } = useToast();
  const { online, justRestored } = useConnectivity();
  const verification = useOtp(VERIFICATION_STORAGE_KEY);
  const submissionIdempotencyKey = useRef(newIdempotencyKey());

  const STEP_LABELS = [
    { label: "Student", description: "Verify your profile" },
    { label: "Classes", description: "Select classes & make-up" },
    { label: "Review", description: "Confirm and submit" },
  ];

  const [step, setStep] = useState<StepIndex>(0);
  const [config, setConfig] = useState<AbsenceFormConfig>(DEFAULT_CONFIG);
  const [configLoading, setConfigLoading] = useState(true);
  const [lookupInput, setLookupInput] = useState("");
  const [lookup, setLookup] = useState<StudentLookupResponse | null>(null);
  const [lookupLoading, setLookupLoading] = useState(false);
  const [lookupError, setLookupError] = useState<string | null>(null);
  const [selectedSubjectIds, setSelectedSubjectIds] = useState<string[]>([]);
  const [reason, setReason] = useState("");
  const [reasonError, setReasonError] = useState<string | null>(null);
  const [sessions, setSessions] = useState<SubjectSessions[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [sessionsError, setSessionsError] = useState<string | null>(null);
  const [selectedSessionIds, setSelectedSessionIds] = useState<Set<string>>(new Set());
  const [sitInSelections, setSitInSelections] = useState<Record<string, string>>({});
  const [sitInPriorityLevels, setSitInPriorityLevels] = useState<Record<string, number>>({});
  const [sitInPriorityHistory, setSitInPriorityHistory] = useState<Record<string, Record<number, SubjectSessions>>>({});
  const [revealingPrioritySessionIds, setRevealingPrioritySessionIds] = useState<Set<string>>(new Set());
  const [pageError, setPageError] = useState<string | null>(null);
  const [verificationSatisfied, setVerificationSatisfied] = useState(false);
  const [verificationBlocked, setVerificationBlocked] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submissionError, setSubmissionError] = useState<string | null>(null);
  const [finalResults, setFinalResults] = useState<ManagedAbsence[] | null>(null);
  const resultHeadingRef = useRef<HTMLHeadingElement | null>(null);

  const selectedSubjectCount = selectedSubjectIds.length;
  const selectedSessionCount = useMemo(
    () => countSelectedSessions(sessions, selectedSessionIds),
    [sessions, selectedSessionIds],
  );
  const maxSessions = config.sit_in.max_sessions_per_absence;
  const atMaxSessions = selectedSessionCount >= maxSessions;
  const canProceedFromVerify = !!lookup && verificationSatisfied;
  const studentDisplayName = getStudentDisplayName(lookup);
  const sessionLookupWindow = useMemo(() => {
    const today = new Date();
    return {
      dateFrom: dateToLocalISO(today),
      dateTo: dateToLocalISO(new Date(today.getTime() + config.form.max_date_range_days * 24 * 60 * 60 * 1000)),
    };
  }, [config.form.max_date_range_days]);

  const missingSitIn = useMemo(() => {
    for (const group of sessions) {
      if (!selectedSubjectIds.includes(group.subject_id)) continue;
      for (const session of group.sessions) {
        if (!selectedSessionIds.has(session.id)) continue;
        const sitIn = sitInForMissedSession(group, session.id);
        if (sitIn?.sit_in_method === "physical" && !sitInSelections[session.id]) return true;
      }
    }
    return false;
  }, [sessions, selectedSubjectIds, selectedSessionIds, sitInSelections]);

  const canSubmit = selectedSubjectCount > 0 && selectedSessionCount > 0 && reason.trim().length > 0 && !verificationBlocked && !missingSitIn;

  useEffect(() => {
    let active = true;
    void apiJson<AbsenceFormConfig>("/api/v1/absence-form-config", { method: "GET" })
      .then((data) => {
        if (!active) return;
        const notifications: AbsenceNotificationsSettings = {
          sms_parent_enabled: data.notifications?.sms_parent_enabled ?? DEFAULT_NOTIFICATIONS.sms_parent_enabled,
          sms_parent_template: data.notifications?.sms_parent_template ?? DEFAULT_NOTIFICATIONS.sms_parent_template,
          sms_success_template: data.notifications?.sms_success_template ?? DEFAULT_NOTIFICATIONS.sms_success_template,
          allow_submit_without_otp: data.notifications?.allow_submit_without_otp ?? DEFAULT_NOTIFICATIONS.allow_submit_without_otp,
        };
        const adminContact: AdminContactSettings = {
          email: data.admin_contact?.email ?? DEFAULT_ADMIN_CONTACT.email,
          phone: data.admin_contact?.phone ?? DEFAULT_ADMIN_CONTACT.phone,
          hours: data.admin_contact?.hours ?? DEFAULT_ADMIN_CONTACT.hours,
        };
        setConfig({ ...DEFAULT_CONFIG, ...data, form: { ...DEFAULT_CONFIG.form, ...data.form }, sit_in: { ...DEFAULT_CONFIG.sit_in, ...data.sit_in }, notifications, admin_contact: adminContact });
      })
      .catch((error: unknown) => {
        addToast("error", error instanceof Error ? error.message : "Failed to load form settings");
      })
      .finally(() => { if (active) setConfigLoading(false); });
    return () => { active = false; };
  }, [addToast]);

  useEffect(() => {
    if (step !== 1 || !lookup) return;
    const controller = new AbortController();
    setSessionsLoading(true);
    setSessionsError(null);
    void apiJson<SessionsInRangeResponse>(
      sessionsInRangePath(lookup.wcode, sessionLookupWindow.dateFrom, sessionLookupWindow.dateTo),
      { method: "GET", signal: controller.signal },
    )
      .then((data) => { if (!controller.signal.aborted) setSessions(data.subjects); })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setSessions([]);
        setSessionsError(error instanceof Error ? error.message : "Couldn't load your classes");
      })
      .finally(() => { if (!controller.signal.aborted) setSessionsLoading(false); });
    return () => controller.abort();
  }, [step, lookup, sessionLookupWindow.dateFrom, sessionLookupWindow.dateTo]);

  useEffect(() => {
    if (!lookup) return;
    const snapshot = { step, lookup, lookupInput, selectedSubjectIds, reason, selectedSessionIds: [...selectedSessionIds], sitInSelections, sitInPriorityLevels, sitInPriorityHistory };
    try { window.sessionStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify(snapshot)); } catch { }
  }, [lookup, lookupInput, selectedSubjectIds, reason, selectedSessionIds, sitInSelections, sitInPriorityLevels, sitInPriorityHistory, step]);

  useEffect(() => {
    try {
      const raw = window.sessionStorage.getItem(SESSION_STORAGE_KEY);
      if (!raw) return;
      const parsed = JSON.parse(raw) as Partial<{
        step: StepIndex; lookup: StudentLookupResponse; lookupInput: string;
        selectedSubjectIds: string[]; reason: string; selectedSessionIds: string[];
        sitInSelections: Record<string, string>; sitInPriorityLevels: Record<string, number>;
        sitInPriorityHistory: Record<string, Record<number, SubjectSessions>>;
      }>;
      if (parsed.lookup) setLookup(parsed.lookup);
      if (typeof parsed.lookupInput === "string") setLookupInput(parsed.lookupInput);
      if (Array.isArray(parsed.selectedSubjectIds)) setSelectedSubjectIds(parsed.selectedSubjectIds);
      if (typeof parsed.reason === "string") setReason(parsed.reason);
      if (Array.isArray(parsed.selectedSessionIds)) setSelectedSessionIds(new Set(parsed.selectedSessionIds));
      if (parsed.sitInSelections) setSitInSelections(parsed.sitInSelections);
      if (parsed.sitInPriorityLevels) setSitInPriorityLevels(parsed.sitInPriorityLevels);
      if (parsed.sitInPriorityHistory) setSitInPriorityHistory(parsed.sitInPriorityHistory);
      if (typeof parsed.step === "number") setStep(parsed.step as StepIndex);
    } catch { }
  }, []);

  useEffect(() => {
    if (!verification.token) {
      setVerificationSatisfied(false);
      setVerificationBlocked(false);
      return;
    }
    const expiry = verification.expiresAt;
    if (expiry && expiry < Date.now()) {
      setVerificationBlocked(true);
      setVerificationSatisfied(false);
      return;
    }
    setVerificationBlocked(false);
  }, [verification]);

  const handleVerificationSatisfied = useCallback(() => {
    setVerificationSatisfied(true);
    setStep(1);
  }, []);

  const handleLookup = async () => {
    setLookupError(null);
    setLookup(null);
    setPageError(null);
    const cleaned = normalizeLookupWcode(lookupInput);
    if (!cleaned) {
      setLookupError("Enter your Student ID (W-Code).");
      return;
    }
    try {
      setLookupLoading(true);
      const response = await apiJson<StudentLookupResponse>(
        `/api/v1/absences/student-lookup?wcode=${encodeURIComponent(cleaned)}`,
        { method: "GET" },
      );
      setLookup(response);
      setLookupInput(cleaned);
      setSelectedSubjectIds([]);
      verification.clearStoredToken();
      verification.setCode("");
      setVerificationSatisfied(false);
    } catch (error) {
      setLookupError(error instanceof Error ? error.message : "We couldn't find your profile");
    } finally {
      setLookupLoading(false);
    }
  };

  const toggleSubject = (subjectId: string) => {
    setSelectedSubjectIds((current) =>
      current.includes(subjectId) ? current.filter((id) => id !== subjectId) : [...current, subjectId],
    );
  };

  const handleSessionToggle = (sessionId: string) => {
    setSelectedSessionIds((current) => {
      if (current.has(sessionId)) {
        const next = new Set(current);
        next.delete(sessionId);
        setSitInSelections((cs) => { const n = { ...cs }; delete n[sessionId]; return n; });
        return next;
      }
      if (current.size >= maxSessions) return current;
      const next = new Set(current);
      next.add(sessionId);
      return next;
    });
  };

  const handleSitInSelect = (sessionId: string, sitInSessionId: string) => {
    setSitInSelections((current) => {
      if (!sitInSessionId) { const n = { ...current }; delete n[sessionId]; return n; }
      return { ...current, [sessionId]: sitInSessionId };
    });
  };

  const handleNotAvailable = async (group: SubjectSessions, sessionId: string) => {
    const currentLevel = sitInPriorityLevels[sessionId] || group.sit_in?.current_priority_level || firstPriorityLevel(group);
    if (lookup && hasServerPriorityReveal(group)) {
      setRevealingPrioritySessionIds((current) => new Set(current).add(sessionId));
      setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
      setSitInPriorityHistory((prev) => ({ ...prev, [sessionId]: { ...(prev[sessionId] ?? {}), [currentLevel]: group } }));
      try {
        const data = await apiJson<SessionsInRangeResponse>(
          sessionsInRangePath(lookup.wcode, sessionLookupWindow.dateFrom, sessionLookupWindow.dateTo, {
            courseIds: [group.course_id], satVerbalAfterPriority: currentLevel,
          }),
          { method: "GET" },
        );
        const updatedGroup = data.subjects.find((subject) => subject.course_id === group.course_id);
        if (!updatedGroup) { setPageError("No more make-up times are available for this class."); return; }
        const updatedSessionGroup = groupWithSitInForMissedSession(updatedGroup, sessionId);
        const updatedLevel = updatedSessionGroup.sit_in?.current_priority_level ?? firstPriorityLevel(updatedSessionGroup);
        setSitInPriorityLevels((prev) => ({ ...prev, [sessionId]: updatedLevel }));
        setSitInPriorityHistory((prev) => ({ ...prev, [sessionId]: { ...(prev[sessionId] ?? {}), [updatedLevel]: updatedSessionGroup } }));
      } catch (error) {
        setPageError(error instanceof Error ? error.message : "Couldn't load other make-up times");
      } finally {
        setRevealingPrioritySessionIds((current) => { const n = new Set(current); n.delete(sessionId); return n; });
      }
      return;
    }
    const nextLevel = nextPriorityLevel(group, currentLevel);
    if (nextLevel == null) return;
    setSitInPriorityLevels((prev) => ({ ...prev, [sessionId]: nextLevel }));
    setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
  };

  const handlePreviousPriority = (group: SubjectSessions, sessionId: string) => {
    const currentLevel = sitInPriorityLevels[sessionId] || group.sit_in?.current_priority_level || firstPriorityLevel(group);
    if (hasServerPriorityReveal(group)) {
      const history = sitInPriorityHistory[sessionId] ?? {};
      const previousLevel = Object.keys(history).map(Number).filter((level) => level < currentLevel).sort((a, b) => b - a)[0];
      const previousGroup = previousLevel !== undefined ? history[previousLevel] : undefined;
      if (!previousGroup) return;
      setSitInPriorityLevels((prev) => ({ ...prev, [sessionId]: previousLevel }));
      setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
      return;
    }
    const previousLevel = previousPriorityLevel(group, currentLevel);
    if (previousLevel == null) return;
    setSitInPriorityLevels((prev) => ({ ...prev, [sessionId]: previousLevel }));
    setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
  };

  const goToStep = useCallback((next: StepIndex) => {
    setStep(next);
    try { window.scrollTo({ top: 0, behavior: "instant" as ScrollBehavior }); } catch { }
  }, []);

  function validateStepOne() {
    setReasonError(null);
    if (selectedSubjectIds.length === 0) {
      setPageError("Select at least one course.");
      return false;
    }
    if (!reason.trim()) {
      setReasonError("Please tell us why you'll be away.");
      return false;
    }
    if (missingSitIn) {
      setPageError("Pick a make-up class for all selected sessions before submitting.");
      return false;
    }
    return true;
  }

  function buildSubmissionPayloads() {
    if (!lookup) return [];
    const payloads: AbsenceBatchCreateItem[] = [];
    for (const group of sessions) {
      if (!selectedSubjectIds.includes(group.subject_id)) continue;
      const selectedGroupSessions = getSelectedSessionsForGroup(group, selectedSessionIds);
      if (selectedGroupSessions.length === 0) continue;
      const selectedDates = [...new Set(selectedGroupSessions.map((session) => session.date))].sort();
      const dateFrom = selectedDates[0];
      const dateTo = selectedDates[selectedDates.length - 1];
      if (daysBetween(dateFrom, dateTo) > config.form.max_date_range_days) {
        setPageError(`${group.subject_name || group.course_name} spans more than ${config.form.max_date_range_days} days. Split it into separate submissions.`);
        return null;
      }
      const selectedSessIds = selectedGroupSessions.map((session) => session.id);
      const sitInSessionIds = selectedSessIds.map((id) => sitInSelections[id]).filter((id): id is string => !!id);
      const sitInMethod = group.sit_in?.sit_in_method;
      const payload: AbsenceBatchCreateItem = {
        subject_id: group.subject_id, course_id: group.course_id,
        date_from: dateFrom, date_to: dateTo,
        reason: reason.trim() || undefined,
        missed_session_ids: selectedSessIds, sit_in_session_ids: sitInSessionIds,
      };
      if (sitInMethod === "physical" || sitInMethod === "zoom") payload.sit_in_method = sitInMethod;
      const sitInCourseID = selectedSitInCourseIDForGroup(group, selectedSessIds, sitInSelections);
      if (sitInCourseID === null) {
        setPageError(`${group.subject_name || group.course_name} has sit-in selections from more than one priority class. Split them into separate submissions.`);
        return null;
      }
      if (sitInCourseID) payload.sit_in_course_id = sitInCourseID;
      payloads.push(payload);
    }
    return payloads;
  }

  async function handleSubmitAbsence() {
    setSubmissionError(null);
    setPageError(null);
    if (!validateStepOne()) return;
    if (!lookup) { setPageError("Search for your profile first."); return; }
    const payloads = buildSubmissionPayloads();
    if (payloads === null) return;
    if (payloads.length === 0) { setPageError("Select at least one class to submit."); return; }
    try {
      setIsSubmitting(true);
      const response = await apiJson<AbsenceBatchCreateResponse>("/api/v1/absences/batch", {
        method: "POST",
        headers: { "Idempotency-Key": submissionIdempotencyKey.current },
        body: JSON.stringify({
          wcode: lookup.wcode,
          reason: reason.trim(),
          verification_token: verificationSatisfied && verification.token ? verification.token : undefined,
          items: payloads,
        }),
      });
      setFinalResults(response.items);
      verification.clearStoredToken();
      verification.setCode("");
      try { window.sessionStorage.removeItem(SESSION_STORAGE_KEY); } catch { }
    } catch (error) {
      setSubmissionError(error instanceof Error ? error.message : "Could not submit your absence");
    } finally {
      setIsSubmitting(false);
    }
  }

  const submissionOverlay = !finalResults && isSubmitting ? (
    <motion.div
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      className="fixed inset-0 z-50 flex items-center justify-center bg-white/80 backdrop-blur-sm"
      role="status"
      aria-live="polite"
      aria-label="Submitting absence request"
    >
      <div className="flex flex-col items-center gap-4">
        <svg
          className="h-10 w-10 animate-spin text-[var(--color-wi-primary)]"
          viewBox="0 0 24 24"
          fill="none"
          aria-hidden="true"
        >
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path
            className="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
          />
        </svg>
        <p className="text-sm font-semibold text-[var(--color-wi-text)]">Submitting your absence...</p>
        <p className="text-xs text-[var(--color-wi-text-light)]">Please wait while we process your request.</p>
      </div>
    </motion.div>
  ) : null;

  if (finalResults) {
    const submittedCount = finalResults.length;
    const successMessage = submittedCount === 1
      ? "Your absence request has been sent and is waiting for review."
      : `Your ${submittedCount} absence requests have been sent and are waiting for review.`;
    const referenceId = finalResults[0]?.id?.slice(0, 8).toUpperCase() || "";
    return (
      <div className="min-h-screen bg-[var(--color-wi-bg)] px-4 py-8">
        <div className="mx-auto max-w-lg space-y-6">
          <div className="rounded-lg border border-[var(--color-wi-green)]/30 bg-white p-6 shadow-sm" aria-live="polite">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-green)]/10">
                <svg className="h-5 w-5 text-[var(--color-wi-green)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
              <div>
                <h2 ref={resultHeadingRef} tabIndex={-1} className="text-xl font-bold tracking-tight text-[var(--color-wi-text)]">
                  {submittedCount === 1 ? "Absence submitted" : `${submittedCount} absences submitted`}
                </h2>
                {referenceId && (
                  <p className="text-xs text-[var(--color-wi-text-light)] mt-0.5">Reference: #{referenceId}</p>
                )}
              </div>
            </div>
            <p className="mt-3 text-sm text-[var(--color-wi-text-light)]">{successMessage}</p>
          </div>
          <div className="rounded-lg border border-[var(--color-wi-border)] bg-white p-6 shadow-sm">
            <h3 className="text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wide">Submitted classes</h3>
            <div className="mt-4 space-y-3">
              {finalResults.map((absence) => {
                const label = absence.subject_code?.trim() || absence.subject_name?.trim() || absence.course_code?.trim() || absence.course_name?.trim() || "Submitted class";
                return (
                  <article key={absence.id} className="rounded-lg border border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] p-4">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div className="min-w-0">
                        <p className="text-sm font-semibold text-[var(--color-wi-text)]">{label}</p>
                        <p className="text-xs text-[var(--color-wi-text-light)]">{formatBatchAbsenceSummary(absence)}</p>
                      </div>
                      <span className="rounded-full bg-[var(--color-wi-green)]/10 px-2.5 py-0.5 text-xs font-semibold text-[var(--color-wi-green)]">Pending review</span>
                    </div>
                    <div className="mt-3 flex gap-4 text-sm text-[var(--color-wi-text-light)]">
                      <p><span className="font-medium text-[var(--color-wi-text)]">Absence:</span> {formatBatchAbsenceSummary(absence)}</p>
                      <p><span className="font-medium text-[var(--color-wi-text)]">Make-up:</span> {formatBatchSitInSummary(absence)}</p>
                    </div>
                  </article>
                );
              })}
            </div>
          </div>
          <div className="flex gap-3">
            <button
              type="button"
              onClick={() => navigate("/absences")}
              className="min-h-[48px] flex-1 rounded-lg bg-[var(--color-wi-primary)] px-4 text-sm font-semibold text-white transition-colors hover:bg-[var(--color-wi-primary-dark)]"
            >
              View my absences
            </button>
            <button
              type="button"
              onClick={() => navigate("/")}
              className="min-h-[48px] flex-1 rounded-lg border border-[var(--color-wi-border)] bg-white px-4 text-sm font-semibold text-[var(--color-wi-text)] transition-colors hover:bg-[var(--color-wi-bg)]"
            >
              Back to home
            </button>
          </div>
        </div>
      </div>
    );
  }

  

  return (
    <div className="min-h-screen bg-[var(--color-wi-bg)]">
      <div className="mx-auto max-w-lg px-4 pb-24 pt-6">
        <StepIndicator
          steps={STEP_LABELS}
          currentStep={step}
          onStepClick={(s) => s < step && goToStep(s as StepIndex)}
        />

        {configLoading ? <LoadingSkeleton type="text" lines={3} /> : null}

        {pageError ? (
          <div role="alert" className="mb-6 rounded-lg bg-[var(--color-wi-danger-bg)] p-4 text-sm text-[var(--color-wi-red)]">{pageError}</div>
        ) : null}
        {submissionError ? (
          <div role="alert" className="mb-6 rounded-lg bg-[var(--color-wi-danger-bg)] p-4 text-sm text-[var(--color-wi-red)]">{submissionError}</div>
        ) : null}

        <div className="space-y-6">
            {step === 0 && (
              <>
                <h1 className="text-2xl font-bold tracking-tight text-[var(--color-wi-text)]">Find your profile</h1>
                <div className="space-y-4">
                  <div>
                    <label htmlFor="wcode-input" className="block text-sm font-semibold text-[var(--color-wi-text)] mb-1.5">
                      Student ID (W-Code)
                    </label>
                    <div className="flex gap-3">
                      <div className="flex-1">
                        <input
                          id="wcode-input"
                          className="min-h-[48px] w-full rounded-lg border border-[var(--color-wi-border)] bg-white px-4 text-sm text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                          placeholder="e.g. W250389"
                          value={lookupInput}
                          onChange={(e) => setLookupInput(e.target.value)}
                          onKeyDown={(e) => { if (e.key === "Enter") void handleLookup(); }}
                        />
                      </div>
                      <button
                        type="button"
                        onClick={() => void handleLookup()}
                        disabled={lookupLoading}
                        className="min-h-[48px] rounded-lg bg-[var(--color-wi-primary)] px-5 text-sm font-semibold text-white transition-colors hover:bg-[var(--color-wi-primary-dark)] disabled:opacity-50"
                      >
                        {lookupLoading ? "..." : "Search"}
                      </button>
                    </div>
                    {lookupError ? (
                      <p role="alert" className="text-sm text-[var(--color-wi-red)] mt-1.5">{lookupError}</p>
                    ) : null}
                  </div>

                  {lookup ? (
                    <div className="space-y-4">
                      <div className="rounded-lg border border-[var(--color-wi-border)] bg-white p-5 shadow-sm">
                        <div className="flex items-start justify-between gap-3">
                          <div>
                            <p className="text-sm font-semibold text-[var(--color-wi-text)]">{studentDisplayName || lookup.full_name}</p>
                            <p className="text-xs font-mono text-[var(--color-wi-text-light)] mt-0.5">{lookup.wcode}</p>
                          </div>
                          {lookup.parent_phone ? (
                            <span className="text-xs text-[var(--color-wi-text-light)] whitespace-nowrap">Parent: {maskPhone(lookup.parent_phone)}</span>
                          ) : (
                            <span className="text-xs text-[var(--color-wi-amber)] whitespace-nowrap">No parent phone</span>
                          )}
                        </div>
                        <div className="border-t border-[var(--color-wi-border)] mt-4 pt-4">
                          <StepCoverVerification
                            wcode={lookup.wcode}
                            parentPhone={lookup.parent_phone}
                            allowSubmitWithoutOtp={config.notifications?.allow_submit_without_otp ?? false}
                            adminContact={config.admin_contact}
                            verification={verification}
                            completed={verificationSatisfied}
                            onSatisfied={handleVerificationSatisfied}
                          />
                        </div>
                      </div>

                      {verificationBlocked ? (
                        <div role="alert" className="rounded-lg bg-[var(--color-wi-amber-bg)] p-4 text-sm text-[var(--color-wi-amber)]">
                          Your parent's verification has expired. Please verify again.
                        </div>
                      ) : null}

                      {!online ? (
                        <div role="status" aria-live="polite" className="rounded-lg bg-[var(--color-wi-amber-bg)] px-4 py-3 text-sm font-medium text-[var(--color-wi-amber)]">
                          You're offline. Your progress is saved locally.
                        </div>
                      ) : justRestored ? (
                        <div role="status" aria-live="polite" className="rounded-lg bg-[var(--color-wi-green)]/10 px-4 py-3 text-sm font-medium text-[var(--color-wi-green)]">
                          Back online!
                        </div>
                      ) : null}
                    </div>
                  ) : null}
                </div>
              </>
            )}

            {step === 1 && (
              <>
                <h1 className="text-2xl font-bold tracking-tight text-[var(--color-wi-text)]">Courses & classes</h1>

                {lookup ? (
                  <div className="space-y-6">
                    <section>
                      <h2 className="text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wide mb-3">Which classes?</h2>
                      {lookup.subjects.length > 0 ? (
                        <div className="rounded-lg border border-[var(--color-wi-border)] bg-white divide-y divide-[var(--color-wi-border)] overflow-hidden">
                          {lookup.subjects.map((subject) => (
                            <SubjectCard
                              key={subject.id}
                              id={subject.id}
                              name={subject.name}
                              code={subject.code}
                              selected={selectedSubjectIds.includes(subject.id)}
                              onToggle={() => toggleSubject(subject.id)}
                            />
                          ))}
                        </div>
                      ) : (
                        <p className="text-sm text-[var(--color-wi-text-light)]">No courses available.</p>
                      )}
                    </section>

                    {selectedSubjectIds.length > 0 ? (
                      <section>
                        <div className="flex items-center justify-between mb-3">
                          <h2 className="text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wide">Classes to miss</h2>
                          <span className="text-xs font-semibold text-[var(--color-wi-text-light)]">{selectedSessionCount}/{maxSessions} selected</span>
                        </div>
                        {sessionsLoading ? (
                          <LoadingSkeleton type="table" lines={3} />
                        ) : sessionsError ? (
                          <p role="alert" className="text-sm text-[var(--color-wi-red)]">{sessionsError}</p>
                        ) : sessions.filter(s => selectedSubjectIds.includes(s.subject_id)).length === 0 ? (
                          <p className="text-sm text-[var(--color-wi-text-light)]">No classes found for the selected courses.</p>
                        ) : (
                          <div className="space-y-4">
                            {sessions.filter(s => selectedSubjectIds.includes(s.subject_id)).map((group) => {
                              const selectedCount = group.sessions.filter((s) => selectedSessionIds.has(s.id)).length;
                              const groupLabel = group.subject_name?.trim() || group.course_name?.trim() || group.course_code;
                              return (
                                <div key={group.course_id} className="rounded-lg border border-[var(--color-wi-border)] bg-white overflow-hidden shadow-sm">
                                  <div className="flex items-center justify-between gap-2 border-b border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] px-4 py-3">
                                    <span className="text-sm font-semibold text-[var(--color-wi-text)] truncate">{groupLabel} ({group.sessions.length} classes)</span>
                                    <span className="text-xs font-semibold text-[var(--color-wi-text-light)] shrink-0">{selectedCount} selected</span>
                                  </div>
                                  <div className="space-y-2 p-4">
                                    {group.sessions.map((session) => {
                                      const selected = selectedSessionIds.has(session.id);
                                      const currentSitIn = sitInSelections[session.id] || "";
                                      const sessionGroup = groupWithSitInForMissedSession(group, session.id);
                                      const baseSitIn = sessionGroup.sit_in;
                                      const baseLevel = baseSitIn?.current_priority_level || firstPriorityLevel(sessionGroup);
                                      const requestedLevel = baseSitIn
                                        ? sitInPriorityLevels[session.id] || baseLevel
                                        : firstPriorityLevel(sessionGroup);
                                      const requestedPriorityGroup = sitInPriorityHistory[session.id]?.[requestedLevel] ?? sessionGroup;
                                      const currentLevel = hasPriorityLevel(requestedPriorityGroup, requestedLevel)
                                        ? requestedLevel : baseLevel;
                                      const priorityGroup = sitInPriorityHistory[session.id]?.[currentLevel] ?? sessionGroup;
                                      const sitIn = priorityGroup.sit_in;
                                      const sitInAvailable = sitIn?.available_sessions ?? [];
                                      const hasPriorities = Boolean(sitIn?.priorities && sitIn.priorities.length > 0);
                                      const currentPriorities = hasPriorities ? prioritiesForLevel(priorityGroup, currentLevel) : [];
                                      const sitInClassLabel = getCurrentSitInDisplayName(sitIn, currentPriorities, groupLabel, sessions);

                                      return (
                                        <div key={session.id} className={clsx(
                                          "rounded-lg border px-4 py-3 transition-colors",
                                          selected ? "border-[var(--color-wi-primary)]/30 bg-[var(--color-wi-primary)]/5" : "border-[var(--color-wi-border)] bg-white",
                                        )}>
                                          <div className="flex items-center gap-3">
                                            <input
                                              type="checkbox"
                                              id={`session-${session.id}`}
                                              checked={selected}
                                              disabled={!selected && atMaxSessions}
                                              onChange={() => handleSessionToggle(session.id)}
                                              className="h-4 w-4 shrink-0 rounded border-[var(--color-wi-border)] text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20 disabled:opacity-50 disabled:cursor-not-allowed"
                                            />
                                            <label htmlFor={`session-${session.id}`} className="min-w-0 cursor-pointer flex-1">
                                              <span className="text-sm font-semibold text-[var(--color-wi-text)]">
                                                {formatDate(session.date)} {formatTime(session.start_at)}-{formatTime(session.end_at)}
                                              </span>
                                            </label>
                                          </div>
                                          {selected ? (
                                            <motion.div initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} className="mt-3 pl-7">
                                              {sitIn && sitIn.sit_in_method === "physical" ? (
                                                (() => {
                                                  if (hasPriorities) {
                                                    const serverReveal = hasServerPriorityReveal(priorityGroup);
                                                    const currentPriority = currentPriorities[0];
                                                    const nextLevel = nextPriorityLevel(priorityGroup, currentLevel);
                                                    const hasMorePriorities = serverReveal ? Boolean(sitIn.has_next_priority) : nextLevel !== null;
                                                    const hasPreviousPriority = serverReveal
                                                      ? Object.keys(sitInPriorityHistory[session.id] ?? {}).some((l) => Number(l) < currentLevel)
                                                      : previousPriorityLevel(priorityGroup, currentLevel) !== null;
                                                    const revealingPriority = revealingPrioritySessionIds.has(session.id);
                                                    const currentPriorityAvailable = currentPriorities.flatMap(p =>
                                                      availableSessionsForMissedSession(p, session.id));
                                                    const currentPriorityUnavailable = currentPriorities.flatMap(p =>
                                                      unavailableSessionsForMissedSession(p, session.id).map((u) => ({ ...u, sitInCourse: p.sit_in_course })));

                                                    if (!currentPriority) {
                                                      return (
                                                        <div className="text-sm text-[var(--color-wi-text-light)]">
                                                          <p className="font-medium">No more options available</p>
                                                          <p className="text-xs text-[var(--color-wi-text-light)] mt-0.5">Staff will contact you to arrange a make-up class.</p>
                                                        </div>
                                                      );
                                                    }

                                                    return (
                                                      <div className="rounded-lg border border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] p-3">
                                                        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                                                          <div className="min-w-0">
                                                            <div className="mb-1 flex items-center gap-2">
                                                              <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-[var(--color-wi-amber-bg)] px-1.5 text-[11px] font-semibold text-[var(--color-wi-amber)] ring-1 ring-[var(--color-wi-amber)]/30">
                                                                {currentLevel}
                                                              </span>
                                                              <span className="text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-amber)]">
                                                                {priorityOrdinal(currentLevel)} choice
                                                              </span>
                                                            </div>
                                                            <p className="text-sm font-semibold leading-5 text-[var(--color-wi-text)]">
                                                              {currentPriorities.length === 1 ? currentPriority.label : `${priorityOrdinal(currentLevel)} Priority`}
                                                            </p>
                                                          </div>
                                                          {(hasPreviousPriority || hasMorePriorities) && (
                                                            <div className="inline-flex w-full shrink-0 overflow-hidden rounded-full border border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] p-0.5 sm:w-fit">
                                                              {hasPreviousPriority && (
                                                                <button
                                                                  type="button"
                                                                  disabled={revealingPriority}
                                                                  onClick={() => handlePreviousPriority(priorityGroup, session.id)}
                                                                  aria-label="See previous times"
                                                                  className="inline-flex h-8 flex-1 items-center justify-center gap-1 rounded-full px-2.5 text-xs font-medium text-[var(--color-wi-text-light)] transition hover:bg-white hover:text-[var(--color-wi-text)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-amber)]/40 disabled:opacity-50 sm:flex-none"
                                                                >
                                                                  <ChevronLeft className="h-3.5 w-3.5" />
                                                                  <span>Back</span>
                                                                </button>
                                                              )}
                                                              {hasMorePriorities && (
                                                                <button
                                                                  type="button"
                                                                  disabled={revealingPriority}
                                                                  onClick={() => void handleNotAvailable(priorityGroup, session.id)}
                                                                  className="inline-flex h-8 flex-1 items-center justify-center gap-1 rounded-full px-3 text-xs font-semibold text-[var(--color-wi-text-light)] transition hover:bg-white hover:text-[var(--color-wi-text)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-amber)]/40 disabled:opacity-50 sm:flex-none"
                                                                >
                                                                  <span>{revealingPriority ? "Loading..." : "See other times"}</span>
                                                                  {!revealingPriority && (
                                                                    <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                                                      <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                                                                    </svg>
                                                                  )}
                                                                </button>
                                                              )}
                                                            </div>
                                                          )}
                                                        </div>
                                                        <label className="mt-3 block text-xs font-medium text-[var(--color-wi-text-light)]" htmlFor={`sit-in-${session.id}`}>
                                                          Make-up class
                                                        </label>
                                                        {currentPriorityAvailable.length === 0 ? (
                                                          <div className="mt-1.5 space-y-2">
                                                            <p className="rounded-md border border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] px-3 py-2 text-sm text-[var(--color-wi-text-light)]">
                                                              No available make-up class for this priority.
                                                            </p>
                                                            {currentPriorityUnavailable.length > 0 ? (
                                                              <div className="rounded-md border border-[var(--color-wi-amber)]/30 bg-[var(--color-wi-amber-bg)] px-3 py-2 text-xs text-[var(--color-wi-amber)]">
                                                                <p className="font-semibold">Checked same-number slot:</p>
                                                                <ul className="mt-1 space-y-1">
                                                                  {currentPriorityUnavailable.map((unavailable, index) => {
                                                                    const checkedSession = unavailable.session;
                                                                    const slotLabel = checkedSession
                                                                      ? getSitInSessionLabel(checkedSession, unavailable.sitInCourse, groupLabel, sessions)
                                                                      : `${getSitInCourseDisplayName(unavailable.sitInCourse, groupLabel, sessions) || "Target section"} class #${unavailable.occurrence_number ?? "?"}`;
                                                                    return (
                                                                      <li key={`${unavailable.reason_code}-${checkedSession?.id ?? index}`}>
                                                                        <span className="font-medium">{slotLabel}</span>
                                                                        <span className="text-[var(--color-wi-amber)]"> — {unavailable.reason}</span>
                                                                      </li>
                                                                    );
                                                                  })}
                                                                </ul>
                                                              </div>
                                                            ) : null}
                                                          </div>
                                                        ) : (
                                                          <select
                                                            id={`sit-in-${session.id}`}
                                                            value={currentSitIn}
                                                            onChange={(e) => handleSitInSelect(session.id, e.target.value)}
                                                            className="mt-1.5 w-full rounded-md border border-[var(--color-wi-border)] bg-white px-3 py-2 text-sm text-[var(--color-wi-text)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                                                          >
                                                            <option value="">Not yet selected</option>
                                                            {currentPriorities.flatMap(p =>
                                                              availableSessionsForMissedSession(p, session.id).map(c => (
                                                                <option key={`${p.sit_in_course?.id ?? "course"}:${c.id}`} value={c.id}>
                                                                  {getSitInSessionLabel(c, p.sit_in_course, groupLabel, sessions)}
                                                                </option>
                                                              ))
                                                            )}
                                                          </select>
                                                        )}
                                                      </div>
                                                    );
                                                  }
                                                  return (
                                                    <div>
                                                      <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-[var(--color-wi-amber)] mb-2">
                                                        Pick a make-up class
                                                      </div>
                                                      <p className="text-xs text-[var(--color-wi-text-light)] mb-2 truncate">Sit-in class: {sitInClassLabel}</p>
                                                      <div className="flex flex-col gap-2 text-sm sm:flex-row sm:items-center sm:justify-end">
                                                        <span className="text-[var(--color-wi-text)] font-medium">Make-up class:</span>
                                                        <select
                                                          value={currentSitIn}
                                                          onChange={(e) => handleSitInSelect(session.id, e.target.value)}
                                                          className="w-full rounded-md border border-[var(--color-wi-border)] bg-white px-3 py-2 text-sm text-[var(--color-wi-text)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                                                        >
                                                          <option value="">— Not yet —</option>
                                                          {sitInAvailable.map(c => (
                                                            <option key={c.id} value={c.id}>
                                                              {getSitInSessionLabel(c, sitIn?.sit_in_course, groupLabel, sessions)}
                                                            </option>
                                                          ))}
                                                        </select>
                                                      </div>
                                                    </div>
                                                  );
                                                })()
                                              ) : sitIn && sitIn.sit_in_method === "zoom" ? (
                                                <div className="space-y-1 text-sm text-[var(--color-wi-text)]">
                                                  <div className="flex items-center gap-2">
                                                    <span className="inline-flex h-5 w-5 items-center justify-center rounded-full bg-[var(--color-wi-primary)]/10 text-[10px] font-bold text-[var(--color-wi-primary)]">Z</span>
                                                    <span className="font-medium">Online make-up (Zoom)</span>
                                                  </div>
                                                  <p className="text-xs text-[var(--color-wi-text-light)] ml-7">Staff will send a Zoom link — no need to pick a class</p>
                                                </div>
                                              ) : sitIn && sitIn.sit_in_method === "teacher_case" ? (
                                                <div className="flex items-center gap-2 text-sm text-[var(--color-wi-amber)]">
                                                  <span className="text-xs font-semibold">To arrange</span>
                                                </div>
                                              ) : (
                                                <div className="text-sm text-[var(--color-wi-text-light)]">
                                                  <p className="font-medium">To arrange</p>
                                                  <p className="text-xs text-[var(--color-wi-text-light)] mt-0.5">Staff will contact you to set up a make-up class.</p>
                                                </div>
                                              )}
                                            </motion.div>
                                          ) : null}
                                        </div>
                                      );
                                    })}
                                  </div>
                                </div>
                              );
                            })}
                          </div>
                        )}
                      </section>
                    ) : null}

                    <section>
                      <label htmlFor="absence-reason" className="text-sm font-semibold text-[var(--color-wi-text)] uppercase tracking-wide mb-3 block">
                        Reason for absence
                      </label>
                      <div className="flex items-center justify-between mb-1.5">
                        <span className="text-xs text-[var(--color-wi-text-light)]">{reason.length}/500 characters</span>
                        <div className="flex items-center gap-2">
                          <div className="h-1.5 w-24 overflow-hidden rounded-full bg-gray-200">
                            <div
                              className={clsx(
                                "h-full rounded-full transition-all duration-300",
                                reason.length > 450 ? "bg-[var(--color-wi-amber)]" : reason.length > 0 ? "bg-[var(--color-wi-primary)]" : "bg-transparent",
                              )}
                              style={{ width: `${Math.min((reason.length / 500) * 100, 100)}%` }}
                            />
                          </div>
                          <span className={clsx(
                            "text-xs font-semibold tabular-nums",
                            reason.length > 450 ? (reason.length >= 500 ? "text-[var(--color-wi-red)]" : "text-[var(--color-wi-amber)]") : "text-[var(--color-wi-text-light)]",
                          )}>
                            {reason.length}/500
                          </span>
                        </div>
                      </div>
                      <textarea
                        id="absence-reason"
                        className={clsx(
                          "w-full min-h-[100px] rounded-lg border px-4 py-3 text-sm text-[var(--color-wi-text)] focus:outline-none focus:ring-2",
                          reasonError
                            ? "border-[var(--color-wi-red)] focus:ring-[var(--color-wi-red)]/20"
                            : "border-[var(--color-wi-border)] focus:ring-[var(--color-wi-primary)]/20",
                        )}
                        value={reason}
                        onChange={(e) => { setReason(e.target.value); setReasonError(null); }}
                        maxLength={500}
                        placeholder="Tell us why you'll be away from class..."
                        aria-describedby={reasonError ? "reason-error" : undefined}
                        required
                      />
                      {reasonError ? <p id="reason-error" role="alert" className="text-xs text-[var(--color-wi-red)] mt-1.5">{reasonError}</p> : null}
                    </section>
                  </div>
                ) : (
                  <p className="text-sm text-[var(--color-wi-text-light)]">Search for your profile first.</p>
                )}
              </>
            )}

            {step === 2 && (
              <>
                <h1 className="text-2xl font-bold tracking-tight text-[var(--color-wi-text)]">Review your absence</h1>
                {lookup ? (
                  <div className="space-y-4">
                    <p className="text-sm text-[var(--color-wi-text-light)]">
                      <span className="font-medium text-[var(--color-wi-text)]">{studentDisplayName || lookup.full_name}</span> — {lookup.wcode}
                    </p>

                    {/* Classes section */}
                    <div className="rounded-lg border border-[var(--color-wi-border)] bg-white">
                      <div className="flex items-center justify-between border-b border-[var(--color-wi-border)] px-5 py-3">
                        <h2 className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Classes</h2>
                        <button
                          type="button"
                          onClick={() => goToStep(1)}
                          className="text-xs font-semibold text-[var(--color-wi-primary)] hover:text-[var(--color-wi-primary-dark)] transition-colors min-h-[32px]"
                        >
                          Edit
                        </button>
                      </div>
                      <div className="px-5 py-4 space-y-3">
                        {sessions.filter(s => selectedSubjectIds.includes(s.subject_id)).map((group) => {
                          const selectedSessions = getSelectedSessionsForGroup(group, selectedSessionIds);
                          if (selectedSessions.length === 0) return null;
                          const groupLabel = group.subject_name?.trim() || group.course_name?.trim() || group.course_code;
                          return (
                            <div key={group.course_id}>
                              <p className="text-sm font-semibold text-[var(--color-wi-text)]">{groupLabel}</p>
                              {selectedSessions.map((s) => (
                                <p key={s.id} className="text-xs text-[var(--color-wi-text-light)] mt-0.5">
                                  {formatDate(s.date)} {formatTime(s.start_at)}–{formatTime(s.end_at)}
                                  <span className="text-[var(--color-wi-text-light)]"> — Make-up: </span>
                                  <span className="font-medium text-[var(--color-wi-text)]">{getReviewSitInLabel(s, group, sitInSelections, sitInPriorityLevels, sitInPriorityHistory, sessions)}</span>
                                </p>
                              ))}
                            </div>
                          );
                        })}
                      </div>
                    </div>

                    {/* Reason section */}
                    <div className="rounded-lg border border-[var(--color-wi-border)] bg-white">
                      <div className="flex items-center justify-between border-b border-[var(--color-wi-border)] px-5 py-3">
                        <h2 className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Reason</h2>
                        <button
                          type="button"
                          onClick={() => goToStep(1)}
                          className="text-xs font-semibold text-[var(--color-wi-primary)] hover:text-[var(--color-wi-primary-dark)] transition-colors min-h-[32px]"
                        >
                          Edit
                        </button>
                      </div>
                      <div className="px-5 py-4">
                        <p className="text-sm text-[var(--color-wi-text)]">{reason || <span className="text-[var(--color-wi-text-light)] italic">No reason provided</span>}</p>
                      </div>
                    </div>
                  </div>
                ) : null}
              </>
            )}
        </div>
      </div>

      <StickyFooter
        currentStep={step}
        totalSteps={3}
        canProceed={
          step === 0 ? canProceedFromVerify :
          step === 1 ? canSubmit :
          step === 2 ? true : false
        }
        loading={isSubmitting}
        onBack={() => goToStep(Math.max(0, step - 1) as StepIndex)}
        onPrimary={() => {
          if (step === 0) goToStep(1);
          else if (step === 1) {
            setPageError(null);
            setReasonError(null);
            if (selectedSubjectIds.length === 0) { setPageError("Select at least one course."); return; }
            if (!reason.trim()) { setReasonError("Please tell us why you'll be away."); return; }
            goToStep(2);
          } else if (step === 2) void handleSubmitAbsence();
        }}
        primaryLabel={
          step === 0 ? "Continue" :
          step === 1 ? "Review & Submit" :
          "Submit"
        }
      />

      {submissionOverlay}
    </div>
  );
}
