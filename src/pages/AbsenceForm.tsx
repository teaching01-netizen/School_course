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

const STEP_LABELS = ["Find your profile", "Parent confirmation", "Courses & classes"] as const;
const SESSION_STORAGE_KEY = "warwick-absence-form-state-v3";
const VERIFICATION_STORAGE_KEY = `${SESSION_STORAGE_KEY}:parent-verification`;

const DEFAULT_NOTIFICATIONS: AbsenceNotificationsSettings = {
  sms_parent_enabled: true,
  sms_parent_template: "Warwick Institute: {{student_name}} ได้แจ้งความประสงค์ขอลาเรียน กรุณาแจ้งรหัส {{code}} ให้แก่นักเรียน เพื่อยืนยันว่าผู้ปกครองได้รับทราบแล้ว",
  sms_success_template: "Warwick Institute: {{nickname}} ได้แจ้งลาเรียนคลาส {{class_name}} ในวันที่ {{absence_date}} และมีกำหนดเข้าเรียนชดเชย คลาส {{sit_in_class}} ในวันที่ {{sit_in_date_time}} ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ",
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
  return allSubjects.find(s => s.course_id === sitInCourse?.id)?.subject_name?.trim();
}

function getSitInSessionLabel(
  session: SitInAvailableSession,
  sitInCourse: SitInCourse,
  fallbackSubjectName: string,
  allSubjects: SubjectSessions[],
) {
  const sitInSubjectName = resolveSitInSubjectName(sitInCourse, allSubjects);
  const className =
    session.class_name?.trim() ||
    session.subject_name?.trim() ||
    session.course_name?.trim() ||
    sitInCourse?.name?.trim() ||
    sitInSubjectName ||
    session.subject_code?.trim() ||
    session.course_code?.trim() ||
    sitInCourse?.code?.trim() ||
    fallbackSubjectName;

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
      result.push({ type: "warning", message: "No parent phone number is on file for this student. Contact the school office before submitting.", dismissible: false, role: "status" });
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
  const [pageError, setPageError] = useState<string | null>(null);
  const [courseAnnouncement, setCourseAnnouncement] = useState("");
  const [verificationSatisfied, setVerificationSatisfied] = useState(false);
  const [verificationBlocked, setVerificationBlocked] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submissionError, setSubmissionError] = useState<string | null>(null);
  const [finalResult, setFinalResult] = useState<ManagedAbsence | null>(null);
  const stepHeadingRefs = useRef<Array<HTMLHeadingElement | null>>([]);
  const resultHeadingRef = useRef<HTMLHeadingElement | null>(null);
  const listboxRef = useRef<HTMLDivElement | null>(null);
  const typeaheadRef = useRef<{ buffer: string; timer: number | null }>({ buffer: "", timer: null });

  const selectedSubjectCount = selectedSubjectIds.length;
  const selectedSessionCount = countSelectedSessions(sessions, selectedSessionIds);
  const maxSessions = config.sit_in.max_sessions_per_absence;
  const atMaxSessions = selectedSessionCount >= maxSessions;
  const canProceedFromVerify = !!lookup && verificationSatisfied;

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
      `/api/v1/absences/sessions-in-range?wcode=${encodeURIComponent(lookup.wcode)}&date_from=${dateFrom}&date_to=${dateTo}`,
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
      if (parsed.lookup) setLookup(parsed.lookup);
      if (typeof parsed.lookupInput === "string") setLookupInput(parsed.lookupInput);
      if (Array.isArray(parsed.selectedSubjectIds)) setSelectedSubjectIds(parsed.selectedSubjectIds);
      if (typeof parsed.activeCourseIndex === "number") setActiveCourseIndex(parsed.activeCourseIndex);
      if (typeof parsed.dateFrom === "string") setDateFrom(parsed.dateFrom);
      if (typeof parsed.dateTo === "string") setDateTo(parsed.dateTo);
      if (typeof parsed.reason === "string") setReason(parsed.reason);
      if (Array.isArray(parsed.selectedSessionIds)) setSelectedSessionIds(new Set(parsed.selectedSessionIds));
      if (parsed.sitInSelections) setSitInSelections(parsed.sitInSelections);
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
    const cleaned = lookupInput.trim();
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
    
    const payloads: Record<string, unknown>[] = [];
    
    for (const subjectId of selectedSubjectIds) {
      const group = sessions.find(g => g.subject_id === subjectId);
      if (!group) continue;
      
      const selectedSessIds = group.sessions.filter(s => selectedSessionIds.has(s.id)).map(s => s.id);
      if (selectedSessIds.length === 0) continue;
      
      const sitInSessionIds = selectedSessIds.map(id => sitInSelections[id]).filter(Boolean);
      const sitInMethod = group.sit_in?.sit_in_method ?? "zoom";
      
      payloads.push({
        wcode: lookup.wcode,
        subject_id: group.subject_id,
        course_id: group.course_id,
        date_from: dateFrom,
        date_to: dateTo,
        reason: reason.trim() || undefined,
        sit_in_method: sitInMethod,
        sit_in_course_id: group.sit_in?.sit_in_course?.id ?? group.course_id,
        sit_in_session_ids: sitInSessionIds,
        verification_token: verificationSatisfied && verification.token ? verification.token : undefined,
      });
    }
    
    return payloads;
  }

  async function handleSubmitAbsence() {
    setSubmissionError(null);
    setPageError(null);
    if (!validateStepTwo()) {
      return;
    }
    const payloads = buildSubmissionPayloads();
    if (payloads.length === 0) {
      setPageError("Select at least one class to submit.");
      return;
    }

    try {
      setIsSubmitting(true);
      let lastResult: ManagedAbsence | null = null;
      for (const payload of payloads) {
        lastResult = await apiJson<ManagedAbsence>("/api/v1/absences", {
          method: "POST",
          headers: {
            "Idempotency-Key": submissionIdempotencyKey.current + "-" + String(payload.subject_id),
          },
          body: JSON.stringify(payload),
        });
      }
      setFinalResult(lastResult);
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

  if (finalResult) {
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
                  Absence submitted
                </h2>
              </div>
              <p className="mt-2 text-sm text-gray-700 font-medium">
                Your absence request has been sent and is waiting for review.
              </p>
            </section>
          </motion.div>
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
          <LoadingSkeleton type="card" lines={3} />
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
                            <p className="text-base font-semibold text-[var(--color-wi-text)]">{lookup.full_name}</p>
                            <p className="text-sm font-mono text-gray-700 mt-0.5">{lookup.wcode}</p>
                          </div>
                          <div className="rounded-full border border-gray-300 bg-white px-3 py-1 text-xs font-medium text-gray-700">
                            {lookup.parent_phone ? `Parent's phone ${maskPhone(lookup.parent_phone)}` : "No parent phone in records"}
                          </div>
                        </div>
                      </div>

                      {/* Verify parent CTA */}
                      <div className="flex justify-end">
                        <Button variant="primary" size="lg" onClick={() => goTo(1)}>
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
                      {lookup.full_name} ({lookup.wcode}) · Parent's phone {maskPhone(lookup.parent_phone) || "not on file"}
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
                          Parent confirmed! Now choose your courses and dates.
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

                            return (
                              <section key={group.subject_id} className="overflow-hidden rounded-sm border border-gray-250 bg-white">
                                <div className="flex w-full items-center justify-between gap-2 border-b border-gray-150 bg-gray-50/50 px-3 py-3 text-sm font-semibold text-[var(--color-wi-text)] sm:px-4">
                                  <span className="min-w-0 truncate">▼ {group.subject_code} - {group.subject_name} ({group.sessions.length} classes)</span>
                                  <span className="shrink-0 text-xs font-semibold text-gray-650">
                                    {selectedCount} selected
                                  </span>
                                </div>
                                <div className="space-y-4 p-5">
                                  <p className="text-xs text-gray-500 font-medium">Select day of class you want to absence</p>

                                  <div className="space-y-2">
                                    {group.sessions.map((session) => {
                                      const selected = selectedSessionIds.has(session.id);
                                      const currentSitIn = sitInSelections[session.id] || "";
                                      const sitIn = group.sit_in;
                                      const sitInAvailable = sitIn?.available_sessions ?? [];

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
                                              {sitIn && sitIn.sit_in_method === "physical" ? (
                                                <>
                                                  <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-amber-800 mb-2">
                                                    <span className="inline-flex h-4 w-4 items-center justify-center rounded-full bg-amber-200 text-[10px] font-bold">2</span>
                                                     Pick a make-up class
                                                  </div>
                                                   <p className="text-xs text-gray-600 mb-2 truncate">
                                                       Absence class: {resolveSitInSubjectName(sitIn?.sit_in_course, sessions) || group.subject_name}
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
                                                             {getSitInSessionLabel(c, sitIn?.sit_in_course, group.subject_name, sessions)}
                                                           </option>
                                                         ))}
                                                       </select>
                                                   </div>
                                                </>
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
                                                  <span className="text-xs font-semibold">Needs teacher approval</span>
                                                </div>
                                              ) : (
                                                <div className="text-sm text-gray-600">
                                                  <p className="font-medium">Make-up to be arranged</p>
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
