import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { Check, CheckCircle, ChevronLeft, ChevronRight } from "lucide-react";
import { useNavigate } from "react-router-dom";
import clsx from "clsx";
import { apiJson, newIdempotencyKey } from "@/api/client";
import Button from "@/components/ui/Button";
import Input from "@/components/ui/Input";
import PageHeading from "@/components/ui/PageHeading";
import LoadingSkeleton from "@/components/ui/LoadingSkeleton";
import EmptyState from "@/components/ui/EmptyState";
import CourseChip from "@/components/absences/CourseChip";

import StepCoverVerification from "@/components/absences/StepCoverVerification";
import { useToast } from "@/hooks/useToast";
import { useConnectivity } from "@/hooks/useConnectivity";
import { useOtp } from "@/hooks/useOtp";
import { useWizard } from "@/hooks/useWizard";
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

const STEP_LABELS = ["Find your profile", "Parent confirmation", "Courses & classes"] as const;
const SESSION_STORAGE_KEY = "warwick-absence-form-state-v3";
const VERIFICATION_STORAGE_KEY = `${SESSION_STORAGE_KEY}:parent-verification`;

const DEFAULT_NOTIFICATIONS: AbsenceNotificationsSettings = {
  sms_parent_enabled: true,
  sms_parent_template: "Warwick Institute: {{student_name}} ได้แจ้งความประสงค์ขอลาเรียน กรุณาแจ้งรหัส {{code}} ให้แก่นักเรียน เพื่อยืนยันว่าผู้ปกครองได้รับทราบแล้ว",
  sms_success_template: "Warwick Institute: {{nickname}} ได้แจ้งลาเรียน {{absence_summary}} และมีกำหนดเข้าเรียนชดเชย {{sit_in_summary}} ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ",
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

function activeGroupForLookup(
  lookup: StudentLookupResponse | null,
  selectedSubjectIds: string[],
  activeCourseIndex: number,
) {
  if (!lookup || lookup.subjects.length === 0) return null;
  const activeFromIndex = lookup.subjects[activeCourseIndex];
  if (activeFromIndex && selectedSubjectIds.includes(activeFromIndex.id)) {
    return activeFromIndex;
  }
  const selected = lookup.subjects.find((subject) => selectedSubjectIds.includes(subject.id));
  return selected ?? lookup.subjects[0];
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
            if (!priority.sit_in_course && (priority.available_sessions ?? []).length === 0) {
              return "";
            }
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

function getStudentDisplayName(lookup: StudentLookupResponse | null) {
  return lookup?.display_name?.trim() || lookup?.nickname?.trim() || lookup?.full_name?.trim() || "";
}

function getSelectedSessionDates(groups: SubjectSessions[], selected: Set<string>) {
  const dates = new Set<string>();
  for (const group of groups) {
    for (const session of group.sessions) {
      if (selected.has(session.id)) {
        dates.add(session.date);
      }
    }
  }
  return [...dates].sort();
}

function summarizeSelectedSessionDates(dates: string[]) {
  if (dates.length === 0) return "";
  if (dates.length <= 3) {
    return dates.map((date) => formatDate(date)).join(", ");
  }
  return `${formatDate(dates[0])}, ${formatDate(dates[1])}, +${dates.length - 2} more`;
}

function getSelectedSessionsForGroup(group: SubjectSessions, selected: Set<string>) {
  return group.sessions
    .filter((session) => selected.has(session.id))
    .slice()
    .sort((a, b) => a.start_at.localeCompare(b.start_at));
}

function firstPriorityLevel(group: SubjectSessions): number {
  const priorities = group.sit_in?.priorities ?? [];
  if (priorities.length === 0) return 1;
  return Math.min(...priorities.map((priority) => priority.level));
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

function hasServerPriorityReveal(group: SubjectSessions): boolean {
  return group.sit_in?.current_priority_level !== undefined || group.sit_in?.has_next_priority !== undefined;
}

function priorityOrdinal(level: number): string {
  const mod100 = level % 100;
  if (mod100 >= 11 && mod100 <= 13) return `${level}th`;
  switch (level % 10) {
    case 1:
      return `${level}st`;
    case 2:
      return `${level}nd`;
    case 3:
      return `${level}rd`;
    default:
      return `${level}th`;
  }
}

function sessionsInRangePath(
  wcode: string,
  dateFrom: string,
  dateTo: string,
  options?: { courseIds?: string[]; satVerbalAfterPriority?: number },
): string {
  const params = new URLSearchParams({
    wcode,
    date_from: dateFrom,
    date_to: dateTo,
  });
  if (options?.courseIds && options.courseIds.length > 0) {
    params.set("course_ids", options.courseIds.join(","));
  }
  if (options?.satVerbalAfterPriority !== undefined) {
    params.set("sat_verbal_after_priority", String(options.satVerbalAfterPriority));
  }
  return `/api/v1/absences/sessions-in-range?${params.toString()}`;
}

function selectedSitInCourseIDForGroup(
  group: SubjectSessions,
  selectedMissedSessionIds: string[],
  sitInSelections: Record<string, string>,
): string | null {
  if (group.sit_in?.sit_in_method !== "physical") {
    return group.sit_in?.sit_in_course?.id?.trim() || group.course_id.trim() || null;
  }

  const priorities = group.sit_in.priorities ?? [];
  if (priorities.length === 0) {
    return group.sit_in.sit_in_course?.id?.trim() || group.course_id.trim() || null;
  }

  const courseIDs = new Set<string>();
  for (const missedSessionID of selectedMissedSessionIds) {
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
  if (courseIDs.size === 0) return group.sit_in.sit_in_course?.id?.trim() || group.course_id.trim() || null;
  return null;
}

function formatBatchAbsenceSummary(absence: ManagedAbsence) {
  const className = absence.subject_name?.trim() || absence.course_name?.trim() || absence.course_code?.trim() || "";
  const dates = getAbsenceSessionDateLabels(absence);
  if (!className && !dates) {
    return "To arrange";
  }
  if (!dates) {
    return className || "To arrange";
  }
  if (!className) {
    return dates;
  }
  return `${className} (${dates})`;
}

function getAbsenceSessionDateLabels(absence: ManagedAbsence) {
  const sessions = absence.missed_sessions ?? [];
  const dates = new Set<string>();
  for (const session of sessions) {
    if (session.start_at) {
      dates.add(session.start_at.slice(0, 10));
    }
  }
  const labels = [...dates].sort().map((date) => formatDate(date));
  if (labels.length > 0) {
    return labels.join(", ");
  }
  if (absence.date_from && absence.date_to) {
    if (absence.date_from === absence.date_to) {
      return formatDate(absence.date_from);
    }
    return `${formatDate(absence.date_from)} - ${formatDate(absence.date_to)}`;
  }
  return "";
}

function formatBatchSitInSummary(absence: ManagedAbsence) {
  const method = absence.sit_in_method?.trim();
  if (method === "zoom") {
    return "Zoom";
  }
  const sessions = absence.sit_ins ?? [];
  const sessionLabels = sessions
    .filter((session) => session.start_at)
    .map((session) => `${formatDate(session.start_at.slice(0, 10))} ${formatTime(session.start_at)}-${formatTime(session.end_at)}`);
  if (method !== "physical") {
    if (sessionLabels.length > 0) {
      return `To arrange (${sessionLabels.join(", ")})`;
    }
    return "To arrange";
  }
  if (sessionLabels.length > 0) {
    const className = absence.sit_in_subject_name?.trim() || absence.sit_in_course_name?.trim() || absence.sit_in_course_code?.trim() || "Make-up class";
    return `${className} (${sessionLabels.join(", ")})`;
  }
  const label = absence.sit_in_subject_name?.trim() || absence.sit_in_course_name?.trim() || absence.sit_in_course_code?.trim();
  if (label) return label;
  return "To arrange";
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

/* ------------------------------------------------------------------ */
/*  Form Error Summary Region                                         */
/* ------------------------------------------------------------------ */
function FormErrorSummary({
  pageError,
  submissionError,
  verificationBlocked,
  lookupError,
  sessionsError,
  lookup,
  online,
  justRestored,
  onClearPageError,
  onClearSubmissionError,
  onGoToVerification,
}: {
  pageError: string | null;
  submissionError: string | null;
  verificationBlocked: boolean;
  lookupError: string | null;
  sessionsError: string | null;
  lookup: StudentLookupResponse | null;
  online: boolean;
  justRestored: boolean;
  onClearPageError: () => void;
  onClearSubmissionError: () => void;
  onGoToVerification: () => void;
}) {
  const [showExpanded, setShowExpanded] = useState(false);

  const items = useMemo(() => {
    const result: Array<{
      type: string;
      message: string;
      dismissible: boolean;
      role: "alert" | "status";
      onDismiss?: () => void;
    }> = [];

    if (submissionError) {
      result.push({ type: "error", message: submissionError, dismissible: true, role: "alert", onDismiss: onClearSubmissionError });
    }
    if (verificationBlocked) {
      result.push({ type: "verification_blocked", message: "Your parent's verification has expired. Please verify again.", dismissible: false, role: "alert" });
    }
    if (lookupError) {
      result.push({ type: "error", message: lookupError, dismissible: false, role: "alert" });
    }
    if (sessionsError) {
      result.push({ type: "error", message: sessionsError, dismissible: false, role: "alert" });
    }
    if (pageError) {
      result.push({ type: "error", message: pageError, dismissible: true, role: "alert", onDismiss: onClearPageError });
    }
    if (lookup && !lookup.parent_phone) {
      result.push({ type: "warning", message: "No parent phone number is on file for this student. Contact admin at Tel. 02-658-4880 Line Official: @warwick.", dismissible: false, role: "status" });
    }
    if (!online) {
      result.push({ type: "offline", message: "You're offline. Your progress is saved locally.", dismissible: false, role: "status" });
    } else if (justRestored) {
      result.push({ type: "restored", message: "Back online!", dismissible: false, role: "status" });
    }

    return result;
  }, [pageError, submissionError, verificationBlocked, lookupError, sessionsError, lookup, online, justRestored, onClearPageError, onClearSubmissionError, onGoToVerification]);

  if (items.length === 0) return null;

  const visible = showExpanded ? items : [items[0]];
  const hiddenCount = items.length - visible.length;

  return (
    <div className="space-y-2">
      {visible.map((item, index) => {
        const isTop = index === 0;
        const role = isTop ? item.role : "status";

        if (item.type === "verification_blocked") {
          return (
            <div key={index} role={role} className="flex items-center justify-between gap-3 rounded-sm border border-amber-250 bg-amber-50 p-4 text-sm text-amber-900 animate-fade-in shadow-sm">
              <div>
                <strong>Verification expired.</strong> {item.message}
              </div>
              <button
                type="button"
                className="shrink-0 inline-flex items-center rounded-sm bg-amber-200/60 px-3 py-1 text-xs font-semibold text-amber-950 hover:bg-amber-200/90 transition-colors"
                onClick={onGoToVerification}
              >
                Go to verification
              </button>
            </div>
          );
        }

        if (item.type === "warning") {
          return (
            <div key={index} role={role} className="rounded-sm border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 animate-fade-in shadow-sm">
              {item.message}
            </div>
          );
        }

        if (item.type === "offline") {
          return (
            <div
              key={index}
              role={role}
              aria-live="polite"
              className="rounded-sm border border-amber-200 bg-amber-50 px-4 py-3 text-sm font-medium text-amber-900 animate-fade-in shadow-sm"
            >
              {item.message}
            </div>
          );
        }

        if (item.type === "restored") {
          return (
            <div
              key={index}
              role={role}
              aria-live="polite"
              className="rounded-sm border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm font-medium text-emerald-900 animate-fade-in shadow-sm"
            >
              {item.message}
            </div>
          );
        }

        return (
          <div
            key={index}
            role={role}
            className="flex items-start justify-between gap-3 rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-950 animate-fade-in shadow-sm"
          >
            <div className="flex-1">{item.message}</div>
            {item.dismissible && (
              <button
                type="button"
                onClick={() => item.onDismiss?.()}
                className="shrink-0 rounded-sm p-1 text-red-800 hover:bg-red-100 transition-colors"
                aria-label="Dismiss error"
              >
                <svg className="h-4 w-4" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
                  <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                </svg>
              </button>
            )}
          </div>
        );
      })}

      {items.length > 1 && !showExpanded ? (
        <button
          type="button"
          onClick={() => setShowExpanded((v) => !v)}
          className="text-xs text-gray-500 hover:text-gray-700 underline transition-colors"
        >
          {hiddenCount} more issue{hiddenCount === 1 ? "" : "s"}
        </button>
      ) : null}
      {items.length > 1 && showExpanded ? (
        <button
          type="button"
          onClick={() => setShowExpanded((v) => !v)}
          className="text-xs text-gray-500 hover:text-gray-700 underline transition-colors"
        >
          Show less
        </button>
      ) : null}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Main Component                                                    */
/* ------------------------------------------------------------------ */
export default function AbsenceForm() {
  const navigate = useNavigate();
  const { addToast } = useToast();
  const reduceMotion = useReducedMotion();
  const { online, justRestored } = useConnectivity();
  const { step, direction, goTo, back, isTransitioning, setIsTransitioning } = useWizard(0);
  const verification = useOtp(VERIFICATION_STORAGE_KEY);
  const submissionIdempotencyKey = useRef(newIdempotencyKey());

  const [config, setConfig] = useState<AbsenceFormConfig>(DEFAULT_CONFIG);
  const [configLoading, setConfigLoading] = useState(true);
  const [lookupInput, setLookupInput] = useState("");
  const [lookup, setLookup] = useState<StudentLookupResponse | null>(null);
  const [lookupLoading, setLookupLoading] = useState(false);
  const [lookupError, setLookupError] = useState<string | null>(null);
  const [selectedSubjectIds, setSelectedSubjectIds] = useState<string[]>([]);
  const [activeCourseIndex, setActiveCourseIndex] = useState(0);
  const [dateFrom, setDateFrom] = useState(() => dateToLocalISO(new Date()));
  const [dateTo, setDateTo] = useState(() => dateToLocalISO(new Date(Date.now() + 30 * 24 * 60 * 60 * 1000)));
  const [reason, setReason] = useState("");
  const [sessions, setSessions] = useState<SubjectSessions[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [sessionsError, setSessionsError] = useState<string | null>(null);

  const [selectedSessionIds, setSelectedSessionIds] = useState<Set<string>>(new Set());
  const [sitInSelections, setSitInSelections] = useState<Record<string, string>>({});
  const [sitInPriorityLevels, setSitInPriorityLevels] = useState<Record<string, number>>({});
  const [sitInPriorityHistory, setSitInPriorityHistory] = useState<Record<string, Record<number, SubjectSessions>>>({});
  const [revealingPrioritySessionIds, setRevealingPrioritySessionIds] = useState<Set<string>>(new Set());
  const [pageError, setPageError] = useState<string | null>(null);
  const [courseAnnouncement, setCourseAnnouncement] = useState("");
  const [verificationSatisfied, setVerificationSatisfied] = useState(false);
  const [verificationBlocked, setVerificationBlocked] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submissionError, setSubmissionError] = useState<string | null>(null);
  const [finalResults, setFinalResults] = useState<ManagedAbsence[] | null>(null);
  const stepHeadingRefs = useRef<Array<HTMLHeadingElement | null>>([]);
  const resultHeadingRef = useRef<HTMLHeadingElement | null>(null);
  const listboxRef = useRef<HTMLDivElement | null>(null);
  const typeaheadRef = useRef<{ buffer: string; timer: number | null }>({ buffer: "", timer: null });

  const selectedSubjectCount = selectedSubjectIds.length;
  const selectedSessionCount = countSelectedSessions(sessions, selectedSessionIds);
  const selectedSessionDates = useMemo(
    () => getSelectedSessionDates(sessions, selectedSessionIds),
    [sessions, selectedSessionIds],
  );
  const selectedSessionDatesLabel = useMemo(
    () => summarizeSelectedSessionDates(selectedSessionDates),
    [selectedSessionDates],
  );
  const maxSessions = config.sit_in.max_sessions_per_absence;
  const atMaxSessions = selectedSessionCount >= maxSessions;
  const canProceedFromVerify = !!lookup && verificationSatisfied;
  const studentDisplayName = getStudentDisplayName(lookup);

  const missingSitIn = useMemo(() => {
    for (const group of sessions) {
      if (!selectedSubjectIds.includes(group.subject_id)) continue;
      if (group.sit_in?.sit_in_method !== "physical") continue;
      for (const session of group.sessions) {
        if (selectedSessionIds.has(session.id) && !sitInSelections[session.id]) {
          return true;
        }
      }
    }
    return false;
  }, [sessions, selectedSubjectIds, selectedSessionIds, sitInSelections]);

  const canSubmit =
    selectedSubjectCount > 0 &&
    selectedSessionCount > 0 &&
    reason.trim().length > 0 &&
    !verificationBlocked &&
    !missingSitIn;


  useEffect(() => {
    let active = true;
    void apiJson<AbsenceFormConfig>("/api/v1/absence-form-config", { method: "GET" })
      .then((data) => {
        if (!active) return;
        const notifications: AbsenceNotificationsSettings = {
          sms_parent_enabled: data.notifications?.sms_parent_enabled ?? DEFAULT_NOTIFICATIONS.sms_parent_enabled,
          sms_parent_template: data.notifications?.sms_parent_template ?? DEFAULT_NOTIFICATIONS.sms_parent_template,
          sms_success_template: data.notifications?.sms_success_template ?? DEFAULT_NOTIFICATIONS.sms_success_template,
          allow_submit_without_otp:
            data.notifications?.allow_submit_without_otp ?? DEFAULT_NOTIFICATIONS.allow_submit_without_otp,
        };
        const adminContact: AdminContactSettings = {
          email: data.admin_contact?.email ?? DEFAULT_ADMIN_CONTACT.email,
          phone: data.admin_contact?.phone ?? DEFAULT_ADMIN_CONTACT.phone,
          hours: data.admin_contact?.hours ?? DEFAULT_ADMIN_CONTACT.hours,
        };
        setConfig({
          ...DEFAULT_CONFIG,
          ...data,
          form: { ...DEFAULT_CONFIG.form, ...data.form },
          sit_in: { ...DEFAULT_CONFIG.sit_in, ...data.sit_in },
          notifications,
          admin_contact: adminContact,
        });
      })
      .catch((error: unknown) => {
        addToast("error", error instanceof Error ? error.message : "Failed to load form settings");
      })
      .finally(() => {
        if (active) setConfigLoading(false);
      });
    return () => {
      active = false;
    };
  }, [addToast]);

  // Fetch sessions when step 2 with valid lookup + dates
  useEffect(() => {
    if (step !== 2) return;
    if (!lookup) return;
    if (!dateFrom || !dateTo || daysBetween(dateFrom, dateTo) < 0) {
      setSessions([]);
      return;
    }

    const controller = new AbortController();
    setSessionsLoading(true);
    setSessionsError(null);

    void apiJson<SessionsInRangeResponse>(
      sessionsInRangePath(lookup.wcode, dateFrom, dateTo),
      { method: "GET", signal: controller.signal },
    )
      .then((data) => {
        if (!controller.signal.aborted) {
          setSessions(data.subjects);
        }
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setSessions([]);
        setSessionsError(error instanceof Error ? error.message : "Couldn't load your classes");
      })
      .finally(() => {
        if (!controller.signal.aborted) setSessionsLoading(false);
      });

    return () => controller.abort();
  }, [step, lookup, dateFrom, dateTo]);

  useEffect(() => {
    if (!lookup) return;
    const snapshot = {
      step,
      lookup,
      lookupInput,
      selectedSubjectIds,
      activeCourseIndex,
      dateFrom,
      dateTo,
      reason,
      selectedSessionIds: [...selectedSessionIds],
      sitInSelections,
      sitInPriorityLevels,
    };
    try {
      window.sessionStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify(snapshot));
    } catch {
      // ignore storage failures
    }
  }, [
    lookup,
    lookupInput,
    selectedSubjectIds,
    activeCourseIndex,
    dateFrom,
    dateTo,
    reason,
    selectedSessionIds,
    sitInSelections,
    sitInPriorityLevels,
    step,
  ]);

  useEffect(() => {
    try {
      const raw = window.sessionStorage.getItem(SESSION_STORAGE_KEY);
      if (!raw) return;
      const parsed = JSON.parse(raw) as Partial<{
        step: StepIndex;
        lookup: StudentLookupResponse;
        lookupInput: string;
        selectedSubjectIds: string[];
        activeCourseIndex: number;
        dateFrom: string;
        dateTo: string;
        reason: string;
        selectedSessionIds: string[];
        sitInSelections: Record<string, string>;
      }>;

      type AbsenceSnapshot = typeof parsed;
      const typedParsed = parsed as AbsenceSnapshot & { sitInPriorityLevels?: Record<string, number> };
      if (typedParsed.lookup) setLookup(typedParsed.lookup);
      if (typeof typedParsed.lookupInput === "string") setLookupInput(typedParsed.lookupInput);
      if (Array.isArray(typedParsed.selectedSubjectIds)) setSelectedSubjectIds(typedParsed.selectedSubjectIds);
      if (typeof typedParsed.activeCourseIndex === "number") setActiveCourseIndex(typedParsed.activeCourseIndex);
      if (typeof typedParsed.dateFrom === "string") setDateFrom(typedParsed.dateFrom);
      if (typeof typedParsed.dateTo === "string") setDateTo(typedParsed.dateTo);
      if (typeof typedParsed.reason === "string") setReason(typedParsed.reason);
      if (Array.isArray(typedParsed.selectedSessionIds)) setSelectedSessionIds(new Set(typedParsed.selectedSessionIds));
      if (typedParsed.sitInSelections) setSitInSelections(typedParsed.sitInSelections);
      if (typedParsed.sitInPriorityLevels) setSitInPriorityLevels(typedParsed.sitInPriorityLevels);
      if (typeof parsed.step === "number") {
        goTo(parsed.step as StepIndex);
      }
    } catch {
      // ignore restore failures
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (lookup && selectedSubjectIds.length > 0) {
      const activeId = lookup.subjects[activeCourseIndex]?.id;
      if (!activeId || !selectedSubjectIds.includes(activeId)) {
        const fallbackIndex = lookup.subjects.findIndex((subject) => selectedSubjectIds.includes(subject.id));
        setActiveCourseIndex(Math.max(0, fallbackIndex));
      }
    }
  }, [selectedSubjectIds, lookup, activeCourseIndex]);

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
    // Note: we do NOT set verificationSatisfied here. Having a token only
    // means an OTP was sent, not verified. The StepCoverVerification
    // component calls onSatisfied when the session is actually verified.
  }, [verification]);

  const handleVerificationSatisfied = useCallback(() => {
    setVerificationSatisfied(true);
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
      const response = await apiJson<StudentLookupResponse>(`/api/v1/absences/student-lookup?wcode=${encodeURIComponent(cleaned)}`, {
        method: "GET",
      });
      setLookup(response);
      setLookupInput(cleaned);
      setSelectedSubjectIds([]);
      setActiveCourseIndex(0);
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
    setSelectedSubjectIds((current) => {
      const activeGroupBefore = activeGroupForLookup(lookup, current, activeCourseIndex);
      const next = current.includes(subjectId) ? current.filter((id) => id !== subjectId) : [...current, subjectId];
      if (lookup) {
        const activeGroupAfter = activeGroupForLookup(lookup, next, activeCourseIndex);
        if (activeGroupBefore && activeGroupAfter && activeGroupBefore.id !== activeGroupAfter.id) {
          const index = lookup.subjects.findIndex((subject) => subject.id === activeGroupAfter.id);
          setActiveCourseIndex(Math.max(0, index));
        }
      }
      return next;
    });
  };

  const selectAllSubjects = () => {
    if (!lookup) return;
    setSelectedSubjectIds(lookup.subjects.map((subject) => subject.id));
    setCourseAnnouncement("Selected all courses.");
  };

  const deselectAllSubjects = () => {
    if (!lookup) return;
    setSelectedSubjectIds([]);
    setCourseAnnouncement("Deselected all courses.");
  };

  const handleSessionToggle = (sessionId: string) => {
    setSelectedSessionIds((current) => {
      if (current.has(sessionId)) {
        const next = new Set(current);
        next.delete(sessionId);
        setSitInSelections((currentSitIns) => {
          const nextSitIns = { ...currentSitIns };
          delete nextSitIns[sessionId];
          return nextSitIns;
        });
        return next;
      }
      if (current.size >= maxSessions) {
        return current;
      }
      const next = new Set(current);
      next.add(sessionId);
      return next;
    });
  };

  const handleSitInSelect = (sessionId: string, sitInSessionId: string) => {
    setSitInSelections((current) => {
      if (!sitInSessionId) {
        const next = { ...current };
        delete next[sessionId];
        return next;
      }
      return { ...current, [sessionId]: sitInSessionId };
    });
  };

  const handleNotAvailable = async (group: SubjectSessions, sessionId: string) => {
    const currentLevel = sitInPriorityLevels[sessionId] || group.sit_in?.current_priority_level || firstPriorityLevel(group);
    if (lookup && hasServerPriorityReveal(group)) {
      setRevealingPrioritySessionIds((current) => new Set(current).add(sessionId));
      setSitInSelections((prev) => {
        const next = { ...prev };
        delete next[sessionId];
        return next;
      });
      setSitInPriorityHistory((prev) => ({
        ...prev,
        [sessionId]: {
          ...(prev[sessionId] ?? {}),
          [currentLevel]: group,
        },
      }));
      try {
        const data = await apiJson<SessionsInRangeResponse>(
          sessionsInRangePath(lookup.wcode, dateFrom, dateTo, {
            courseIds: [group.course_id],
            satVerbalAfterPriority: currentLevel,
          }),
          { method: "GET" },
        );
        const updatedGroup = data.subjects.find((subject) => subject.course_id === group.course_id);
        if (!updatedGroup) {
          setPageError("No more make-up times are available for this class.");
          return;
        }
        setSessions((current) =>
          current.map((subject) => (subject.course_id === group.course_id ? updatedGroup : subject)),
        );
        const updatedLevel = updatedGroup.sit_in?.current_priority_level ?? firstPriorityLevel(updatedGroup);
        setSitInPriorityLevels((prev) => ({
          ...prev,
          [sessionId]: updatedLevel,
        }));
        setSitInPriorityHistory((prev) => ({
          ...prev,
          [sessionId]: {
            ...(prev[sessionId] ?? {}),
            [updatedLevel]: updatedGroup,
          },
        }));
      } catch (error) {
        setPageError(error instanceof Error ? error.message : "Couldn't load other make-up times");
      } finally {
        setRevealingPrioritySessionIds((current) => {
          const next = new Set(current);
          next.delete(sessionId);
          return next;
        });
      }
      return;
    }

    const nextLevel = nextPriorityLevel(group, currentLevel);
    if (nextLevel == null) return;
    setSitInPriorityLevels((prev) => ({
      ...prev,
      [sessionId]: nextLevel,
    }));
    setSitInSelections((prev) => {
      const next = { ...prev };
      delete next[sessionId];
      return next;
    });
  };

  const handlePreviousPriority = (group: SubjectSessions, sessionId: string) => {
    const currentLevel = sitInPriorityLevels[sessionId] || group.sit_in?.current_priority_level || firstPriorityLevel(group);

    if (hasServerPriorityReveal(group)) {
      const history = sitInPriorityHistory[sessionId] ?? {};
      const previousLevel = Object.keys(history)
        .map(Number)
        .filter((level) => level < currentLevel)
        .sort((a, b) => b - a)[0];
      const previousGroup = previousLevel !== undefined ? history[previousLevel] : undefined;
      if (!previousGroup) return;

      setSessions((current) =>
        current.map((subject) => (subject.course_id === group.course_id ? previousGroup : subject)),
      );
      setSitInPriorityLevels((prev) => ({
        ...prev,
        [sessionId]: previousLevel,
      }));
      setSitInSelections((prev) => {
        const next = { ...prev };
        delete next[sessionId];
        return next;
      });
      return;
    }

    const previousLevel = previousPriorityLevel(group, currentLevel);
    if (previousLevel == null) return;
    setSitInPriorityLevels((prev) => ({
      ...prev,
      [sessionId]: previousLevel,
    }));
    setSitInSelections((prev) => {
      const next = { ...prev };
      delete next[sessionId];
      return next;
    });
  };

  const handleCourseKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!lookup || lookup.subjects.length === 0) return;
    const count = lookup.subjects.length;
    let nextIndex = activeCourseIndex;

    switch (event.key) {
      case "ArrowDown":
      case "ArrowRight":
        event.preventDefault();
        nextIndex = (activeCourseIndex + 1) % count;
        break;
      case "ArrowUp":
      case "ArrowLeft":
        event.preventDefault();
        nextIndex = (activeCourseIndex - 1 + count) % count;
        break;
      case "Home":
        event.preventDefault();
        nextIndex = 0;
        break;
      case "End":
        event.preventDefault();
        nextIndex = count - 1;
        break;
      case " ":
        event.preventDefault();
        {
          const subject = lookup.subjects[activeCourseIndex];
          if (subject) toggleSubject(subject.id);
        }
        return;
      case "Enter":
        event.preventDefault();
        {
          const subject = lookup.subjects[activeCourseIndex];
          if (subject) toggleSubject(subject.id);
        }
        return;
      default:
        // Handle typeahead
        if (event.key.length === 1) {
          const char = event.key.toLowerCase();
          const buffer = typeaheadRef.current.buffer + char;
          typeaheadRef.current.buffer = buffer;
          if (typeaheadRef.current.timer) window.clearTimeout(typeaheadRef.current.timer);
          typeaheadRef.current.timer = window.setTimeout(() => {
            typeaheadRef.current.buffer = "";
            typeaheadRef.current.timer = null;
          }, 500);

          const index = lookup.subjects.findIndex((subject) => subject.code.toLowerCase().startsWith(buffer));
          if (index !== -1) {
            nextIndex = index;
          }
        }
        break;
    }

    if (nextIndex !== activeCourseIndex) {
      setActiveCourseIndex(nextIndex);
      const subject = lookup.subjects[nextIndex];
      if (subject) {
        setCourseAnnouncement(`Focused ${subject.code} ${subject.name}.`);
      }
    }
  };

  function validateStepTwo() {
    if (selectedSubjectIds.length === 0) {
      setPageError("Select at least one course.");
      return false;
    }
    if (!reason.trim()) {
      setPageError("Please tell us why you'll be away.");
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
        setPageError(
          `${group.subject_name || group.course_name} spans more than ${config.form.max_date_range_days} days. Split it into separate submissions.`,
        );
        return null;
      }

      const selectedSessIds = selectedGroupSessions.map((session) => session.id);
      const sitInSessionIds = selectedSessIds.map((id) => sitInSelections[id]).filter((id): id is string => !!id);
      const sitInMethod = group.sit_in?.sit_in_method;
      const payload: AbsenceBatchCreateItem = {
        subject_id: group.subject_id,
        course_id: group.course_id,
        date_from: dateFrom,
        date_to: dateTo,
        reason: reason.trim() || undefined,
        missed_session_ids: selectedSessIds,
        sit_in_session_ids: sitInSessionIds,
      };
      if (sitInMethod === "physical" || sitInMethod === "zoom") {
        payload.sit_in_method = sitInMethod;
      }
      const sitInCourseID = selectedSitInCourseIDForGroup(group, selectedSessIds, sitInSelections);
      if (sitInCourseID === null) {
        setPageError(`${group.subject_name || group.course_name} has sit-in selections from more than one priority class. Split them into separate submissions.`);
        return null;
      }
      if (sitInCourseID) {
        payload.sit_in_course_id = sitInCourseID;
      }
      payloads.push(payload);
    }

    return payloads;
  }

  async function handleSubmitAbsence() {
    setSubmissionError(null);
    setPageError(null);
    if (!validateStepTwo()) {
      return;
    }
    if (!lookup) {
      setPageError("Search for your profile first.");
      return;
    }
    const payloads = buildSubmissionPayloads();
    if (payloads === null) {
      return;
    }
    if (payloads.length === 0) {
      setPageError("Select at least one class to submit.");
      return;
    }

    try {
      setIsSubmitting(true);
      const response = await apiJson<AbsenceBatchCreateResponse>("/api/v1/absences/batch", {
        method: "POST",
        headers: {
          "Idempotency-Key": submissionIdempotencyKey.current,
        },
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
      try {
        window.sessionStorage.removeItem(SESSION_STORAGE_KEY);
      } catch {
        // ignore
      }
    } catch (error) {
      setSubmissionError(error instanceof Error ? error.message : "Could not submit your absence");
    } finally {
      setIsSubmitting(false);
    }
  }

  if (finalResults) {
    const submittedCount = finalResults.length;
    const successMessage = submittedCount === 1
      ? "Your absence request has been sent and is waiting for review."
      : `Your ${submittedCount} absence requests have been sent and are waiting for review.`;
    return (
      <div className="min-h-screen bg-[radial-gradient(circle_at_top,_rgba(17,24,39,0.03),_transparent_40%),linear-gradient(180deg,_#f8fafc_0%,_#ffffff_100%)] px-4 py-8">
        <div className="mx-auto max-w-3xl space-y-5">
          <PageHeading>Absence form</PageHeading>
          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ type: "spring", damping: 20, stiffness: 100 }}
          >
            <section
              className="rounded-sm border border-emerald-200 bg-white p-5 shadow-sm animate-fade-in"
              aria-live="polite"
            >
              <div className="flex items-center gap-3">
                <CheckCircle className="h-7 w-7 text-emerald-500" aria-hidden="true" />
                <h2 ref={resultHeadingRef} tabIndex={-1} className="text-xl font-semibold text-[var(--color-wi-text)]">
                  {submittedCount === 1 ? "Absence submitted" : `${submittedCount} absences submitted`}
                </h2>
              </div>
              <p className="mt-2 text-sm text-gray-700 font-medium">{successMessage}</p>
            </section>
          </motion.div>
          <section className="rounded-sm border border-gray-200 bg-white p-5 shadow-sm">
            <h3 className="text-sm font-semibold text-gray-900">Submitted classes</h3>
            <div className="mt-4 space-y-3">
              {finalResults.map((absence) => {
                const label = absence.subject_code?.trim() || absence.subject_name?.trim() || absence.course_code?.trim() || absence.course_name?.trim() || "Submitted class";
                return (
                  <article key={absence.id} className="rounded-sm border border-gray-200 bg-gray-50 p-4">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div className="min-w-0">
                        <p className="text-sm font-semibold text-gray-900">{label}</p>
                        <p className="text-xs text-gray-600">{formatBatchAbsenceSummary(absence)}</p>
                      </div>
                      <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-semibold text-emerald-800">Pending review</span>
                    </div>
                    <div className="mt-3 grid gap-2 text-sm text-gray-700">
                      <p>
                        <span className="font-medium text-gray-900">Absence:</span>{" "}
                        {formatBatchAbsenceSummary(absence)}
                      </p>
                      <p>
                        <span className="font-medium text-gray-900">Make-up:</span>{" "}
                        {formatBatchSitInSummary(absence)}
                      </p>
                    </div>
                  </article>
                );
              })}
            </div>
          </section>
        </div>
      </div>
    );
  }

  const stepTransition = {
    initial: { opacity: 0, x: reduceMotion ? 0 : direction === "forward" ? 20 : -20 },
    animate: { opacity: 1, x: 0 },
    exit: { opacity: 0, x: reduceMotion ? 0 : direction === "forward" ? -20 : 20 },
    transition: { duration: reduceMotion ? 0 : 0.22, ease: "easeInOut" as const },
  };

  return (
    <div className="min-h-screen bg-[radial-gradient(circle_at_top,_rgba(17,24,39,0.03),_transparent_40%),linear-gradient(180deg,_#f8fafc_0%,_#ffffff_100%)] px-4 py-8">
      <div className="mx-auto max-w-4xl space-y-5">
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-2">
            <PageHeading>Absence form</PageHeading>
            <p className="max-w-2xl text-sm text-gray-700 font-medium">
              Your parent or guardian will need to confirm by text message.
            </p>
          </div>
          <Button variant="secondary" size="sm" onClick={() => navigate("/")}>
            Exit
          </Button>
        </div>

        {configLoading ? (
          <LoadingSkeleton type="text" lines={3} />
        ) : null}

        <FormErrorSummary
          pageError={pageError}
          submissionError={submissionError}
          verificationBlocked={verificationBlocked}
          lookupError={lookupError}
          sessionsError={sessionsError}
          lookup={lookup}
          online={online}
          justRestored={justRestored}
          onClearPageError={() => setPageError(null)}
          onClearSubmissionError={() => setSubmissionError(null)}
          onGoToVerification={() => goTo(1)}
        />

        <div className="rounded-sm border border-gray-200 bg-white p-3 shadow-sm">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            {STEP_LABELS.map((label, index) => (
              <motion.button
                key={label}
                type="button"
                whileTap={reduceMotion ? undefined : { scale: 0.95 }}
                transition={{ type: "spring", stiffness: 400, damping: 25 }}
                className={clsx(
                  "inline-flex min-h-[44px] items-center gap-2 rounded-sm px-3 py-2 transition-all",
                  index === step
                    ? "bg-[var(--color-wi-primary)]/10 text-[var(--color-wi-primary)] font-semibold"
                    : index < step
                      ? "text-gray-700 hover:bg-gray-50 font-medium"
                      : "text-gray-600 font-normal hover:text-gray-700"
                )}
                onClick={() => {
                  if (index < step) goTo(index as StepIndex);
                }}
                disabled={index > step || isTransitioning}
                aria-label={`Step ${index + 1}: ${label}`}
              >
                <span className="inline-flex h-5 w-5 items-center justify-center rounded-full border border-current/25 text-[10px] font-bold">
                  {index < step ? <Check className="h-3 w-3" /> : index + 1}
                </span>
                <span className="hidden sm:inline">{label}</span>
              </motion.button>
            ))}
          </div>
        </div>

        <AnimatePresence mode="popLayout">
          <motion.div
            key={step}
            {...stepTransition}
            onAnimationComplete={() => setIsTransitioning(false)}
            className="space-y-5"
          >
            {step === 0 ? (
              <section className="rounded-sm border border-gray-200 bg-white p-5 shadow-sm">
                <h2
                  ref={(node) => {
                    stepHeadingRefs.current[0] = node;
                  }}
                  tabIndex={-1}
                  className="text-xl font-semibold text-[var(--color-wi-text)] mb-4"
                >
                  Find your profile
                </h2>
                <div className="grid gap-4">
                  <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
                    <label className="block text-sm font-medium text-gray-800">
                      Student ID (W-Code)
                      <Input
                        className="mt-1"
                        placeholder="e.g. W250389"
                        value={lookupInput}
                        onChange={(event) => setLookupInput(event.target.value)}
                        onKeyDown={(event) => {
                          if (event.key === "Enter") {
                            event.preventDefault();
                            void handleLookup();
                          }
                        }}
                      />
                    </label>
                    <div className="flex items-end">
                      <Button
                        variant="primary"
                        size="lg"
                        loading={lookupLoading}
                        onClick={() => void handleLookup()}
                      >
                        Search
                      </Button>
                    </div>
                  </div>

                  {lookup ? (
                    <div className="space-y-4 animate-fade-in mt-4">
                      {/* Student Profile Card */}
                      <div className="rounded-sm border border-gray-250 bg-gray-50 p-5 shadow-sm">
                        <h3 className="text-xs font-semibold uppercase tracking-wider text-gray-600 mb-2">Your profile</h3>
                        <div className="flex flex-wrap items-start justify-between gap-3">
                          <div>
                            <p className="text-base font-semibold text-[var(--color-wi-text)]">{studentDisplayName || lookup.full_name}</p>
                            <p className="text-sm font-mono text-gray-700 mt-0.5">{lookup.wcode}</p>
                          </div>
                          <div className="rounded-full border border-gray-300 bg-white px-3 py-1 text-xs font-medium text-gray-700">
                            {lookup.parent_phone ? `Parent's phone ${maskPhone(lookup.parent_phone)}` : "No parent phone — contact admin at Tel. 02-658-4880"}
                          </div>
                        </div>
                      </div>

                      {/* Verify parent CTA */}
                      <div className="flex flex-col items-end gap-2">
                        {!lookup.parent_phone ? (
                          <p className="text-xs text-amber-700 text-right max-w-[280px]">
                            No parent phone on file. Contact admin at Tel. 02-658-4880 Line Official: @warwick.
                          </p>
                        ) : null}
                        <Button variant="primary" size="lg" onClick={() => goTo(1)} disabled={!lookup.parent_phone}>
                          Verify with parent
                          <ChevronRight className="ml-2 h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                  ) : null}
                </div>
              </section>
            ) : null}

            {step === 1 ? (
              <section className="rounded-sm border border-gray-200 bg-white p-5 shadow-sm">
                <div className="mb-4 space-y-1">
                  <h2
                    ref={(node) => {
                      stepHeadingRefs.current[1] = node;
                    }}
                    tabIndex={-1}
                    className="text-xl font-semibold text-[var(--color-wi-text)]"
                  >
                    Parent confirmation
                  </h2>
                  {lookup ? (
                    <p className="text-sm text-gray-600 font-normal">
                      {studentDisplayName || lookup.full_name} ({lookup.wcode}) · Parent's phone {maskPhone(lookup.parent_phone) || "not on file — contact admin at Tel. 02-658-4880"}
                    </p>
                  ) : null}
                </div>
                {lookup ? (
                  <div className="space-y-6">
                    <StepCoverVerification
                      wcode={lookup.wcode}
                      parentPhone={lookup.parent_phone}
                      allowSubmitWithoutOtp={config.notifications?.allow_submit_without_otp ?? false}
                      adminContact={config.admin_contact}
                      verification={verification}
                      completed={verificationSatisfied}
                      onSatisfied={handleVerificationSatisfied}
                      onWcodeChange={() => {
                        setLookup(null);
                        setLookupError(null);
                        setSelectedSubjectIds([]);
                        setActiveCourseIndex(0);
                        setDateFrom(dateToLocalISO(new Date()));
                        setDateTo(dateToLocalISO(new Date(Date.now() + 30 * 24 * 60 * 60 * 1000)));
                        setReason("");
                        setSessions([]);
                        setSelectedSessionIds(new Set());
                        setSitInSelections({});
                        setPageError(null);
                        setSubmissionError(null);
                        setVerificationSatisfied(false);
                        verification.clearStoredToken();
                        verification.setCode("");
                        goTo(0);
                      }}
                    />

                    {verificationSatisfied ? (
                      <motion.div
                        initial={{ opacity: 0, y: 12 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ type: "spring", damping: 20, stiffness: 100 }}
                        className="flex flex-wrap items-center justify-between gap-3 rounded-sm border border-emerald-250 bg-emerald-50/50 p-5 text-sm shadow-sm"
                      >
                        <div className="text-emerald-950 font-medium">
                          Parent confirmed! Now choose your courses and the class day.
                        </div>
                        <Button
                          variant="primary"
                          size="lg"
                          disabled={!canProceedFromVerify}
                          onClick={() => goTo(2)}
                        >
                          Continue
                          <ChevronRight className="ml-2 h-4 w-4" />
                        </Button>
                      </motion.div>
                    ) : null}

                    <div className="flex flex-wrap items-center justify-between gap-3 pt-2">
                      <Button variant="secondary" onClick={() => back()}>
                        <ChevronLeft className="mr-1 h-4 w-4" />
                        Back
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="rounded-sm border border-gray-200 bg-gray-50 p-5 text-sm text-gray-650 font-medium">
                    Search for a student first.
                  </div>
                )}
              </section>
            ) : null}

            {step === 2 ? (
              <section className="rounded-sm border border-gray-200 bg-white p-5 shadow-sm">
                <h2
                  ref={(node) => {
                    stepHeadingRefs.current[2] = node;
                  }}
                  tabIndex={-1}
                  className="text-xl font-semibold text-[var(--color-wi-text)] mb-4"
                >
                  Courses & classes
                </h2>
                <div className="space-y-6">
                  {lookup ? (
                    <div className="space-y-6">
                      <div className="space-y-3">
                        <div className="flex items-center justify-between gap-2">
                           <h3 className="text-sm font-semibold text-[var(--color-wi-text)]">Choose your courses</h3>
                          <div className="flex gap-2">
                            {selectedSubjectCount < lookup.subjects.length ? (
                              <Button variant="secondary" size="sm" onClick={selectAllSubjects}>
                                Select all
                              </Button>
                            ) : null}
                            {selectedSubjectCount > 0 ? (
                              <Button variant="secondary" size="sm" onClick={deselectAllSubjects}>
                                Deselect all
                              </Button>
                            ) : null}
                          </div>
                        </div>
                        <div className="rounded-sm border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-700">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="rounded-full bg-gray-900 px-2 py-0.5 text-xs font-semibold text-white">
                              {studentDisplayName || lookup.full_name}
                            </span>
                            <span className="font-mono text-xs text-gray-600">{lookup.wcode}</span>
                            {selectedSessionCount > 0 ? (
                              <span
                                className="rounded-full bg-amber-100 px-2 py-0.5 text-xs font-semibold text-amber-800"
                                title={selectedSessionDatesLabel || undefined}
                              >
                                {selectedSessionCount} session{selectedSessionCount === 1 ? "" : "s"} across {selectedSessionDates.length} day{selectedSessionDates.length === 1 ? "" : "s"}
                              </span>
                            ) : (
                              <span className="text-xs text-gray-500">Pick sessions first, then choose the sit-in.</span>
                            )}
                          </div>
                        </div>
                        <div
                          ref={listboxRef}
                          role="listbox"
                          aria-multiselectable="true"
                          aria-label="Choose your courses"
                          tabIndex={0}
                          onKeyDown={handleCourseKeyDown}
                          className="grid gap-2 sm:grid-cols-2"
                        >
                          {lookup.subjects.map((subject) => (
                            <CourseChip
                              key={subject.id}
                              id={`course-chip-${subject.id}`}
                              name={subject.name}
                              code={subject.code}
                              selected={selectedSubjectIds.includes(subject.id)}
                              tabIndex={-1}
                              onToggle={() => toggleSubject(subject.id)}
                              disabled={false}
                            />
                          ))}
                        </div>
                        <div role="status" aria-live="polite" className="sr-only">
                          {courseAnnouncement}
                        </div>
                      </div>

                      {selectedSubjectIds.length > 0 && dateFrom && dateTo ? (
                        <div className="space-y-4">
                          <div className="flex items-center justify-between gap-2">
                             <h3 className="text-sm font-semibold text-[var(--color-wi-text)]">Classes</h3>
                            <span className="text-xs font-semibold text-gray-600">
                              {selectedSessionCount}/{maxSessions} selected
                            </span>
                          </div>
                          
                          {sessionsLoading ? (
                            <LoadingSkeleton type="table" lines={3} />
                          ) : null}

                          {sessions.filter(s => selectedSubjectIds.includes(s.subject_id)).length === 0 && !sessionsLoading ? (
                            <EmptyState message="No classes found for the courses and dates you picked." />
                          ) : null}

                          {sessions.filter(s => selectedSubjectIds.includes(s.subject_id)).map((group) => {
                            const selectedCount = group.sessions.filter((session) => selectedSessionIds.has(session.id)).length;
                            const groupLabel = group.subject_name?.trim() || group.course_name?.trim() || group.course_code;

                            return (
                              <section key={group.course_id} className="overflow-hidden rounded-sm border border-gray-250 bg-white">
                                <div className="flex w-full items-center justify-between gap-2 border-b border-gray-150 bg-gray-50/50 px-3 py-3 text-sm font-semibold text-[var(--color-wi-text)] sm:px-4">
                                  <span className="min-w-0 truncate">▼ {groupLabel} ({group.sessions.length} classes)</span>
                                  <span className="shrink-0 text-xs font-semibold text-gray-650">
                                    {selectedCount} selected
                                  </span>
                                </div>
                                <div className="space-y-4 p-5">
                                  <p className="text-xs text-gray-500 font-medium">Select the day you want to miss</p>

                                  <div className="space-y-2">
                                    {group.sessions.map((session) => {
                                      const selected = selectedSessionIds.has(session.id);
                                      const currentSitIn = sitInSelections[session.id] || "";
                                      const sitIn = group.sit_in;
                                      const sitInAvailable = sitIn?.available_sessions ?? [];
                                      const hasPriorities = Boolean(sitIn?.priorities && sitIn.priorities.length > 0);
                                      const currentLevel = sitIn
                                        ? sitInPriorityLevels[session.id] || sitIn.current_priority_level || firstPriorityLevel(group)
                                        : firstPriorityLevel(group);
                                      const currentPriorities = hasPriorities ? prioritiesForLevel(group, currentLevel) : [];
                                      const sitInClassLabel = getCurrentSitInDisplayName(sitIn, currentPriorities, groupLabel, sessions);

                                      return (
                                        <div
                                          key={session.id}
                                          className={clsx(
                                            "rounded-sm border px-3 py-3 transition-colors sm:px-4 sm:py-3.5",
                                            selected ? "border-[var(--color-wi-primary)] bg-[var(--color-wi-primary)]/5" : "border-gray-200 bg-white"
                                          )}
                                        >
                                          <div className="min-w-0 text-sm text-[var(--color-wi-text)]">
                                            <div className="flex items-center gap-2 min-w-0 sm:gap-3">
                                              <input
                                                type="checkbox"
                                                id={`session-${session.id}`}
                                                checked={selected}
                                                disabled={!selected && atMaxSessions}
                                                onChange={() => handleSessionToggle(session.id)}
                                                className="h-4 w-4 shrink-0 rounded border-gray-300 text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20 disabled:opacity-50 disabled:cursor-not-allowed"
                                              />
                                              <label htmlFor={`session-${session.id}`} className="min-w-0 cursor-pointer">
                                                <span className="block font-semibold sm:truncate">{formatDate(session.date)} {formatTime(session.start_at)}-{formatTime(session.end_at)}</span>
                                              </label>
                                            </div>
                                          </div>
                                          {selected ? (
                                            <motion.div
                                              initial={{ opacity: 0, scale: 0.95 }}
                                              animate={{ opacity: 1, scale: 1 }}
                                              className="mt-3 rounded-sm border border-amber-200 bg-amber-50/50 p-3"
                                            >
                                              <div className="mb-3 grid gap-1 text-sm">
                                                <p className="font-medium text-amber-900">
                                                  <span className="rounded-full bg-amber-200 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-900">
                                                    Leave
                                                  </span>
                                                  <span className="ml-2">Subject: {groupLabel}</span>
                                                </p>
                                                <p className="font-medium text-sky-800">
                                                  <span className="rounded-full bg-sky-100 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-sky-700">
                                                    Sit-in
                                                  </span>
                                                  <span className="ml-2">Subject: {sitIn && sitIn.sit_in_method === "physical" ? sitInClassLabel : sitIn && sitIn.sit_in_method === "zoom" ? "Zoom" : "To arrange"}</span>
                                                </p>
                                              </div>
                                               {sitIn && sitIn.sit_in_method === "physical" ? (
                                                (() => {
                                                  if (hasPriorities) {
                                                    const serverReveal = hasServerPriorityReveal(group);
                                                    const currentPriority = currentPriorities[0];
                                                    const nextLevel = nextPriorityLevel(group, currentLevel);
                                                    const hasMorePriorities = serverReveal ? Boolean(sitIn.has_next_priority) : nextLevel !== null;
                                                    const hasPreviousPriority = serverReveal
                                                      ? Object.keys(sitInPriorityHistory[session.id] ?? {}).some((level) => Number(level) < currentLevel)
                                                      : previousPriorityLevel(group, currentLevel) !== null;
                                                    const revealingPriority = revealingPrioritySessionIds.has(session.id);
                                                    const currentPriorityAvailable = currentPriorities.flatMap(priority => priority.available_sessions ?? []);

                                                    if (!currentPriority) {
                                                      return (
                                                        <div className="text-sm text-gray-600">
                                                          <p className="font-medium">No more options available</p>
                                                          <p className="text-xs text-gray-500 mt-0.5">Staff will contact you to arrange a make-up class.</p>
                                                        </div>
                                                      );
                                                    }

                                                    return (
                                                      <>
                                                        <motion.div
                                                          key={`${session.id}-${currentLevel}`}
                                                          initial={reduceMotion ? false : { opacity: 0, y: 6 }}
                                                          animate={{ opacity: 1, y: 0 }}
                                                          transition={{ duration: 0.18, ease: "easeOut" }}
                                                          className="rounded-md border border-amber-200 bg-white/80 p-3 shadow-sm shadow-amber-900/5"
                                                        >
                                                          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                                                            <div className="min-w-0">
                                                              <div className="mb-1 flex items-center gap-2">
                                                                <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-amber-100 px-1.5 text-[11px] font-semibold text-amber-800 ring-1 ring-amber-200">
                                                                  {currentLevel}
                                                                </span>
                                                                <span className="text-[11px] font-semibold uppercase tracking-wide text-amber-700">
                                                                  {priorityOrdinal(currentLevel)} choice
                                                                </span>
                                                              </div>
                                                              <p className="text-sm font-semibold leading-5 text-gray-900">
                                                                {currentPriorities.length === 1 ? currentPriority.label : `${priorityOrdinal(currentLevel)} Priority`}
                                                              </p>
                                                            </div>

                                                            {(hasPreviousPriority || hasMorePriorities) && (
                                                              <div className="inline-flex w-full shrink-0 overflow-hidden rounded-full border border-gray-200 bg-gray-50 p-0.5 shadow-sm sm:w-fit">
                                                                {hasPreviousPriority && (
                                                                  <button
                                                                    type="button"
                                                                    disabled={revealingPriority}
                                                                    onClick={() => handlePreviousPriority(group, session.id)}
                                                                    aria-label="See previous times"
                                                                    className="inline-flex h-8 flex-1 items-center justify-center gap-1 rounded-full px-2.5 text-xs font-medium text-gray-600 transition hover:bg-white hover:text-gray-950 hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-400 disabled:cursor-not-allowed disabled:opacity-50 sm:flex-none"
                                                                  >
                                                                    <ChevronLeft className="h-3.5 w-3.5" aria-hidden="true" />
                                                                    <span>Back</span>
                                                                  </button>
                                                                )}
                                                                {hasMorePriorities && (
                                                                  <button
                                                                    type="button"
                                                                    disabled={revealingPriority}
                                                                    onClick={() => void handleNotAvailable(group, session.id)}
                                                                    className="inline-flex h-8 flex-1 items-center justify-center gap-1 rounded-full px-3 text-xs font-semibold text-gray-700 transition hover:bg-white hover:text-gray-950 hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-400 disabled:cursor-not-allowed disabled:opacity-50 sm:flex-none"
                                                                  >
                                                                    <span>{revealingPriority ? "Loading..." : "See other times"}</span>
                                                                    {!revealingPriority && <ChevronRight className="h-3.5 w-3.5" aria-hidden="true" />}
                                                                  </button>
                                                                )}
                                                              </div>
                                                            )}
                                                          </div>

                                                          <label className="mt-3 block text-xs font-medium text-gray-600" htmlFor={`sit-in-${session.id}`}>
                                                            Make-up class
                                                          </label>
                                                          {currentPriorityAvailable.length === 0 ? (
                                                            <p className="mt-1.5 rounded-md border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-600 sm:max-w-[420px]">
                                                              No available make-up class for this priority.
                                                            </p>
                                                          ) : (
                                                            <select
                                                              id={`sit-in-${session.id}`}
                                                              value={currentSitIn}
                                                              onChange={(e) => handleSitInSelect(session.id, e.target.value)}
                                                              className="mt-1.5 w-full min-w-0 max-w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 shadow-sm transition focus:border-amber-500 focus:outline-none focus:ring-2 focus:ring-amber-200 sm:max-w-[420px]"
                                                            >
                                                              <option value="">Not yet selected</option>
                                                              {currentPriorities.flatMap(priority =>
                                                                (priority.available_sessions ?? []).map(c => (
                                                                  <option key={`${priority.sit_in_course?.id ?? "course"}:${c.id}`} value={c.id}>
                                                                    {getSitInSessionLabel(c, priority.sit_in_course, groupLabel, sessions)}
                                                                  </option>
                                                                ))
                                                              )}
                                                            </select>
                                                          )}
                                                        </motion.div>
                                                      </>
                                                    );
                                                  }

                                                  // Fallback: flat single-level sit-in (no priorities)
                                                  return (
                                                    <>
                                                      <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-amber-800 mb-2">
                                                         Pick a make-up class
                                                      </div>
                                                         <p className="text-xs text-gray-600 mb-2 truncate">
                                                             Sit-in class: {sitInClassLabel}
                                                         </p>
                                                       <div className="flex flex-col gap-2 text-sm sm:flex-row sm:items-center sm:justify-end">
                                                         <span className="text-gray-700 font-medium">Make-up class:</span>
                                                         <select
                                                           value={currentSitIn}
                                                           onChange={(e) => handleSitInSelect(session.id, e.target.value)}
                                                           className="w-full min-w-0 max-w-full rounded-sm border border-gray-300 py-1.5 px-2 text-sm focus:border-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20 sm:w-auto sm:max-w-[320px]"
                                                         >
                                                            <option value="">— Not yet —</option>
                                                            {sitInAvailable.map(c => (
                                                              <option key={c.id} value={c.id}>
                                                                 {getSitInSessionLabel(c, sitIn?.sit_in_course, groupLabel, sessions)}
                                                              </option>
                                                            ))}
                                                          </select>
                                                       </div>
                                                    </>
                                                  );
                                                })()
                                              ) : sitIn && sitIn.sit_in_method === "zoom" ? (
                                                <div className="space-y-1 text-sm text-gray-700">
                                                  <div className="flex items-center gap-2">
                                                    <span className="inline-flex h-5 w-5 items-center justify-center rounded-full bg-blue-100 text-[10px] font-bold text-blue-700">Z</span>
                                                    <span className="font-medium">Online make-up (Zoom)</span>
                                                  </div>
                                                  <p className="text-xs text-gray-500 ml-7">Staff will send a Zoom link — no need to pick a class</p>
                                                </div>
                                              ) : sitIn && sitIn.sit_in_method === "teacher_case" ? (
                                                <div className="flex items-center gap-2 text-sm text-amber-700">
                                                  <span className="text-xs font-semibold">To arrange</span>
                                                </div>
                                              ) : (
                                                <div className="text-sm text-gray-600">
                                                  <p className="font-medium">To arrange</p>
                                                  <p className="text-xs text-gray-500 mt-0.5">Staff will contact you to set up a make-up class.</p>
                                                </div>
                                              )}
                                            </motion.div>
                                          ) : null}
                                        </div>
                                      );
                                    })}
                                  </div>
                                </div>
                              </section>
                            );
                          })}
                        </div>
                      ) : null}

                      <div className="space-y-2">
                        <label className="block text-sm font-medium text-gray-700">
                          Reason for absence <span className="text-red-500">*</span>
                        </label>
                        <div className="flex items-center justify-between mb-1">
                          <span />
                          <span className={clsx("text-xs font-semibold", reason.length > 450 ? (reason.length >= 500 ? "text-red-600" : "text-amber-600") : "text-gray-600")}>
                            {reason.length}/500
                          </span>
                        </div>
                        <textarea
                          className="w-full min-h-[96px] rounded-sm border border-gray-300 px-3 py-2 text-sm text-gray-850 focus:border-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20"
                          value={reason}
                          onChange={(event) => setReason(event.target.value)}
                          maxLength={500}
                          placeholder="Tell us why you'll be away from class..."
                          required
                        />
                        <div className="h-1 w-full bg-gray-100 rounded-full overflow-hidden">
                          <div
                            className={clsx(
                              "h-full transition-all duration-150",
                              reason.length >= 475 ? "bg-red-500" : reason.length >= 400 ? "bg-amber-500" : "bg-[var(--color-wi-primary)]"
                            )}
                            style={{ width: `${(reason.length / 500) * 100}%` }}
                          />
                        </div>
                      </div>

                      <div className="flex flex-wrap items-center justify-between gap-3 rounded-sm border border-gray-200 bg-gray-50 p-5 text-sm">
                        <Button variant="secondary" onClick={() => back()}>
                          <ChevronLeft className="mr-1 h-4 w-4" />
                          Back
                        </Button>
                        <Button
                          variant="primary"
                          size="lg"
                          disabled={!canSubmit}
                          loading={isSubmitting}
                          onClick={() => void handleSubmitAbsence()}
                        >
                          Submit absence
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <div className="rounded-sm border border-gray-200 bg-gray-50 p-5 text-sm text-gray-650 font-medium">
                    Search for your profile first.
                    </div>
                  )}
                </div>
              </section>
            ) : null}
          </motion.div>
        </AnimatePresence>

        <div className="sr-only" aria-live="polite">
          {courseAnnouncement}
        </div>
      </div>
    </div>
  );
}
