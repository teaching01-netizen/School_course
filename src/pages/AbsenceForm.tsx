import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { Check, CheckCircle, ChevronLeft, ChevronRight, Copy } from "lucide-react";
import { useNavigate } from "react-router-dom";
import clsx from "clsx";
import { apiJson, newIdempotencyKey } from "@/api/client";
import Button from "@/components/ui/Button";
import Input from "@/components/ui/Input";
import PageHeading from "@/components/ui/PageHeading";
import LoadingSkeleton from "@/components/ui/LoadingSkeleton";
import EmptyState from "@/components/ui/EmptyState";
import CourseChip from "@/components/absences/CourseChip";
import DateRangeInput from "@/components/absences/DateRangeInput";
import StepCoverVerification from "@/components/absences/StepCoverVerification";
import ConfirmationSummary from "@/components/absences/ConfirmationSummary";
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

type StepIndex = 0 | 1 | 2 | 3;

const STEP_LABELS = ["Lookup & Verify", "Courses & Dates", "Sessions & Cover", "Review & Submit"] as const;
const SESSION_STORAGE_KEY = "warwick-absence-form-state-v2";
const VERIFICATION_STORAGE_KEY = `${SESSION_STORAGE_KEY}:parent-verification`;

const DEFAULT_NOTIFICATIONS: AbsenceNotificationsSettings = {
  sms_parent_enabled: true,
  sms_parent_template: "Your Warwick verification code is {{code}}.",
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

/* ------------------------------------------------------------------ */
/*  Form Error Summary Region                                         */
/* ------------------------------------------------------------------ */
function FormErrorSummary({
  pageError,
  submissionError,
  verificationBlocked,
  lookupError,
  sessionsError,
  parentPhoneMissing,
  lookup,
  online,
  justRestored,
  onClearPageError,
  onGoToVerification,
  onGoToStep: _onGoToStep,
}: {
  pageError: string | null;
  submissionError: string | null;
  verificationBlocked: boolean;
  lookupError: string | null;
  sessionsError: string | null;
  parentPhoneMissing: boolean;
  lookup: StudentLookupResponse | null;
  online: boolean;
  justRestored: boolean;
  onClearPageError: () => void;
  onGoToVerification: () => void;
  onGoToStep: (step: number) => void;
}) {
  const [showExpanded, setShowExpanded] = useState(false);

  const items = useMemo(() => {
    const result: Array<{
      type: string;
      message: string;
      dismissible: boolean;
      role: "alert" | "status";
    }> = [];

    if (submissionError) {
      result.push({ type: "error", message: submissionError, dismissible: true, role: "alert" });
    }
    if (pageError) {
      result.push({ type: "error", message: pageError, dismissible: true, role: "alert" });
    }
    if (verificationBlocked) {
      result.push({ type: "verification_blocked", message: "Your parent verification has expired. Please verify again.", dismissible: false, role: "alert" });
    }
    if (lookupError) {
      result.push({ type: "error", message: lookupError, dismissible: false, role: "alert" });
    }
    if (sessionsError) {
      result.push({ type: "error", message: sessionsError, dismissible: false, role: "alert" });
    }
    if (lookup && !lookup.parent_phone) {
      result.push({ type: "warning", message: "No parent phone number is on file for this student. Contact the school office before submitting.", dismissible: false, role: "status" });
    }
    if (!online) {
      result.push({ type: "offline", message: "You are offline. Your selections are saved locally.", dismissible: false, role: "status" });
    } else if (justRestored) {
      result.push({ type: "restored", message: "Connection restored.", dismissible: false, role: "status" });
    }

    return result;
  }, [pageError, submissionError, verificationBlocked, lookupError, sessionsError, parentPhoneMissing, lookup, online, justRestored]);

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
                Go to Step 1
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
                onClick={onClearPageError}
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

      {hiddenCount > 0 && (
        <button
          type="button"
          onClick={() => setShowExpanded((v) => !v)}
          className="text-xs text-gray-500 hover:text-gray-700 underline transition-colors"
        >
          {showExpanded
            ? "Show less"
            : `${hiddenCount} more issue${hiddenCount === 1 ? "" : "s"}`}
        </button>
      )}
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
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [reasonCategory, setReasonCategory] = useState("");
  const [reason, setReason] = useState("");
  const [sessions, setSessions] = useState<SubjectSessions[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [sessionsError, setSessionsError] = useState<string | null>(null);
  const [selectedSessionIds, setSelectedSessionIds] = useState<Set<string>>(new Set());
  const [coverSessionIds, setCoverSessionIds] = useState<Set<string>>(new Set());
  const [pageError, setPageError] = useState<string | null>(null);
  const [courseAnnouncement, setCourseAnnouncement] = useState("");
  const [verificationSatisfied, setVerificationSatisfied] = useState(false);
  const [verificationBlocked, setVerificationBlocked] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submissionError, setSubmissionError] = useState<string | null>(null);
  const [finalResult, setFinalResult] = useState<ManagedAbsence | null>(null);
  const [copiedReference, setCopiedReference] = useState(false);
  const [showReasonFields, setShowReasonFields] = useState(false);

  const stepHeadingRefs = useRef<Array<HTMLHeadingElement | null>>([]);
  const resultHeadingRef = useRef<HTMLHeadingElement | null>(null);
  const listboxRef = useRef<HTMLDivElement | null>(null);
  const typeaheadRef = useRef<{ buffer: string; timer: number | null }>({ buffer: "", timer: null });

  const activeGroup = useMemo(
    () => activeGroupForLookup(lookup, selectedSubjectIds, activeCourseIndex),
    [lookup, selectedSubjectIds, activeCourseIndex],
  );
  const activeSubjectId = activeGroup?.id ?? null;
  const selectedSubjectCount = selectedSubjectIds.length;
  const selectedSessionCount = countSelectedSessions(sessions, selectedSessionIds);
  const coverSessionCount = countSelectedSessions(sessions, coverSessionIds);
  const reasonCategoryLabel = useMemo(
    () => config.form.reason_categories.find((item) => item.value === reasonCategory)?.label ?? "",
    [config.form.reason_categories, reasonCategory],
  );
  const parentPhoneMissing = !lookup?.parent_phone || lookup.parent_phone.trim() === "";
  const canProceedFromVerify = !!lookup && verificationSatisfied;
  const canProceedToSessions =
    !!activeGroup &&
    selectedSubjectCount > 0 &&
    !!dateFrom &&
    !!dateTo &&
    daysBetween(dateFrom, dateTo) >= 0 &&
    !verificationBlocked;
  const canProceedToReview =
    !!activeGroup &&
    !!dateFrom &&
    !!dateTo &&
    daysBetween(dateFrom, dateTo) >= 0 &&
    selectedSessionCount > 0 &&
    !verificationBlocked;

  // Auto-expand reasons if loaded from storage
  useEffect(() => {
    if (reasonCategory || reason) {
      setShowReasonFields(true);
    }
  }, [reasonCategory, reason]);

  useEffect(() => {
    let active = true;
    void apiJson<AbsenceFormConfig>("/api/v1/absence-form-config", { method: "GET" })
      .then((data) => {
        if (!active) return;
        const notifications: AbsenceNotificationsSettings = {
          sms_parent_enabled: data.notifications?.sms_parent_enabled ?? DEFAULT_NOTIFICATIONS.sms_parent_enabled,
          sms_parent_template: data.notifications?.sms_parent_template ?? DEFAULT_NOTIFICATIONS.sms_parent_template,
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

  useEffect(() => {
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
        setSessionsError(error instanceof Error ? error.message : "Failed to load sessions");
      })
      .finally(() => {
        if (!controller.signal.aborted) setSessionsLoading(false);
      });

    return () => controller.abort();
  }, [lookup, dateFrom, dateTo]);

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
      reasonCategory,
      reason,
      selectedSessionIds: [...selectedSessionIds],
      coverSessionIds: [...coverSessionIds],
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
    reasonCategory,
    reason,
    selectedSessionIds,
    coverSessionIds,
    verificationSatisfied,
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
        reasonCategory: string;
        reason: string;
        selectedSessionIds: string[];
        coverSessionIds: string[];
      }>;
      if (parsed.lookup) setLookup(parsed.lookup);
      if (typeof parsed.lookupInput === "string") setLookupInput(parsed.lookupInput);
      if (Array.isArray(parsed.selectedSubjectIds)) setSelectedSubjectIds(parsed.selectedSubjectIds);
      if (typeof parsed.activeCourseIndex === "number") setActiveCourseIndex(parsed.activeCourseIndex);
      if (typeof parsed.dateFrom === "string") setDateFrom(parsed.dateFrom);
      if (typeof parsed.dateTo === "string") setDateTo(parsed.dateTo);
      if (typeof parsed.reasonCategory === "string") setReasonCategory(parsed.reasonCategory);
      if (typeof parsed.reason === "string") setReason(parsed.reason);
      if (Array.isArray(parsed.selectedSessionIds)) setSelectedSessionIds(new Set(parsed.selectedSessionIds));
      if (Array.isArray(parsed.coverSessionIds)) setCoverSessionIds(new Set(parsed.coverSessionIds));
      if (typeof parsed.step === "number") goTo(Math.max(0, Math.min(3, parsed.step)) as StepIndex);
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
      setLookupError("Enter a W-Code.");
      return;
    }

    try {
      setLookupLoading(true);
      const response = await apiJson<StudentLookupResponse>(`/api/v1/absences/student-lookup?wcode=${encodeURIComponent(cleaned)}`, {
        method: "GET",
      });
      setLookup(response);
      setSelectedSubjectIds(response.subjects.map((subject) => subject.id));
      setActiveCourseIndex(0);
      verification.clearStoredToken();
      verification.setCode("");
      setVerificationSatisfied(false);
    } catch (error) {
      setLookupError(error instanceof Error ? error.message : "Student not found");
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

  const toggleAllSubjects = () => {
    if (!lookup) return;
    if (selectedSubjectCount === lookup.subjects.length) {
      setSelectedSubjectIds([]);
      setCourseAnnouncement("Deselected all courses.");
    } else {
      setSelectedSubjectIds(lookup.subjects.map((subject) => subject.id));
      setCourseAnnouncement("Selected all courses.");
    }
  };

  const handleSessionToggle = (sessionId: string) => {
    setSelectedSessionIds((current) => {
      const next = new Set(current);
      if (next.has(sessionId)) {
        next.delete(sessionId);
        setCoverSessionIds((currentCovers) => {
          const nextCovers = new Set(currentCovers);
          nextCovers.delete(sessionId);
          return nextCovers;
        });
      } else {
        next.add(sessionId);
      }
      return next;
    });
  };

  const handleCoverToggle = (sessionId: string) => {
    setCoverSessionIds((current) => {
      const next = new Set(current);
      if (next.has(sessionId)) {
        next.delete(sessionId);
      } else {
        next.add(sessionId);
      }
      return next;
    });
  };

  const toggleAllSessionsForGroup = (group: SubjectSessions, forceValue: boolean) => {
    setSelectedSessionIds((current) => {
      const next = new Set(current);
      for (const session of group.sessions) {
        if (forceValue) {
          next.add(session.id);
        } else {
          next.delete(session.id);
          setCoverSessionIds((currentCovers) => {
            const nextCovers = new Set(currentCovers);
            nextCovers.delete(session.id);
            return nextCovers;
          });
        }
      }
      return next;
    });
  };

  const toggleAllCoversForGroup = (group: SubjectSessions) => {
    const selectedInGroup = group.sessions.filter((s) => selectedSessionIds.has(s.id));
    const allSelectedCovers = selectedInGroup.every((s) => coverSessionIds.has(s.id));

    setCoverSessionIds((current) => {
      const next = new Set(current);
      for (const session of selectedInGroup) {
        if (allSelectedCovers) {
          next.delete(session.id);
        } else {
          next.add(session.id);
        }
      }
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

  function validateStepOne() {
    if (selectedSubjectIds.length === 0) {
      setPageError("Select at least one course.");
      return false;
    }
    if (!dateFrom || !dateTo) {
      setPageError("Select both dates.");
      return false;
    }
    if (daysBetween(dateFrom, dateTo) < 0) {
      setPageError("The end date must be on or after the start date.");
      return false;
    }
    if (daysBetween(dateFrom, dateTo) > config.form.max_date_range_days) {
      setPageError(`Date range must be ${config.form.max_date_range_days} days or less.`);
      return false;
    }
    return true;
  }

  function validateStepTwo() {
    if (!activeGroup) {
      setPageError("Choose a course first.");
      return false;
    }
    if (selectedSubjectIds.length === 0) {
      setPageError("Select at least one course.");
      return false;
    }
    if (!dateFrom || !dateTo) {
      setPageError("Select both dates.");
      return false;
    }
    if (selectedSessionCount === 0) {
      setPageError("Select at least one session.");
      return false;
    }
    return true;
  }

  function buildSubmissionPayload() {
    if (!lookup || !activeGroup || !primarySessionGroup) return null;
    return {
      wcode: lookup.wcode,
      subject_id: activeGroup.id,
      course_id: primarySessionGroup.course_id,
      date_from: dateFrom,
      date_to: dateTo,
      reason_category: reasonCategory || undefined,
      reason: reason.trim() || undefined,
      sit_in_method: coverSessionCount > 0 ? "physical" : "zoom",
      sit_in_course_id: primarySessionGroup.course_id,
      sit_in_session_ids: [...coverSessionIds],
      verification_token: verificationSatisfied && verification.token ? verification.token : undefined,
    } as Record<string, unknown>;
  }

  async function handleSubmitAbsence() {
    setSubmissionError(null);
    setPageError(null);
    const payload = buildSubmissionPayload();
    if (!payload) {
      setPageError("Something is missing from the form.");
      return;
    }

    try {
      setIsSubmitting(true);
      const response = await apiJson<ManagedAbsence>("/api/v1/absences", {
        method: "POST",
        headers: {
          "Idempotency-Key": submissionIdempotencyKey.current,
        },
        body: JSON.stringify(payload),
      });
      setFinalResult(response);
      verification.clearStoredToken();
      verification.setCode("");
      try {
        window.sessionStorage.removeItem(SESSION_STORAGE_KEY);
      } catch {
        // ignore
      }
    } catch (error) {
      setSubmissionError(error instanceof Error ? error.message : "Could not submit the absence");
    } finally {
      setIsSubmitting(false);
    }
  }

  async function copyReference() {
    if (!finalResult) return;
    const reference = `ABS-${finalResult.id.slice(0, 8).toUpperCase()}`;
    try {
      await navigator.clipboard.writeText(reference);
      setCopiedReference(true);
      window.setTimeout(() => setCopiedReference(false), 2000);
    } catch {
      addToast("warning", "Could not copy the reference");
    }
  }

  function handleReset() {
    setLookupInput("");
    setLookup(null);
    setLookupError(null);
    setSelectedSubjectIds([]);
    setActiveCourseIndex(0);
    setDateFrom("");
    setDateTo("");
    setReasonCategory("");
    setReason("");
    setSessions([]);
    setSessionsError(null);
    setSelectedSessionIds(new Set());
    setCoverSessionIds(new Set());
    setPageError(null);
    setShowReasonFields(false);
    setCourseAnnouncement("");
    setVerificationSatisfied(false);
    setSubmissionError(null);
    setFinalResult(null);
    verification.clearStoredToken();
    verification.setCode("");
    submissionIdempotencyKey.current = newIdempotencyKey();
    try {
      window.sessionStorage.removeItem(SESSION_STORAGE_KEY);
    } catch {
      // ignore
    }
    goTo(0);
  }

  const activeSessions = useMemo(() => {
    if (!activeSubjectId) return [];
    return sessions.filter((group) => group.subject_id === activeSubjectId);
  }, [sessions, activeSubjectId]);
  const primarySessionGroup = activeSessions[0] ?? null;

  if (finalResult) {
    const reference = `ABS-${finalResult.id.slice(0, 8).toUpperCase()}`;
    return (
      <div className="min-h-screen bg-[radial-gradient(circle_at_top,_rgba(17,24,39,0.03),_transparent_40%),linear-gradient(180deg,_#f8fafc_0%,_#ffffff_100%)] px-4 py-8">
        <div className="mx-auto max-w-3xl space-y-5">
          <div className="flex items-center justify-between">
            <PageHeading>Report an Absence</PageHeading>
            <div className="flex gap-2">
              <Button variant="secondary" onClick={handleReset}>
                Report another
              </Button>
              <Button variant="secondary" onClick={() => navigate("/")}>
                Done
              </Button>
            </div>
          </div>
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
                  Submission complete
                </h2>
              </div>
              <p className="mt-2 text-sm text-gray-700 font-medium">
                Your absence has been saved and is waiting for review.
              </p>
              <motion.div
                initial={{ opacity: 0, x: -10 }}
                animate={{ opacity: 1, x: 0 }}
                transition={{ delay: 0.3, duration: 0.3 }}
                className="mt-4 flex flex-wrap items-center gap-2 rounded-sm border border-gray-250 bg-gray-50 px-4 py-3"
              >
                <span className="text-xs font-semibold uppercase tracking-wide text-gray-600">Reference</span>
                <span className="font-mono text-sm font-semibold">{reference}</span>
                <Button variant="secondary" size="sm" onClick={() => void copyReference()}>
                  <Copy className="mr-1 h-4 w-4" />
                  {copiedReference ? "Copied" : "Copy"}
                </Button>
              </motion.div>
            </section>
          </motion.div>
          <ConfirmationSummary
            mode="result"
            studentName={lookup?.full_name ?? finalResult.student_name ?? ""}
            wcode={finalResult.wcode}
            dateFrom={finalResult.date_from}
            dateTo={finalResult.date_to}
            reasonCategoryLabel={reasonCategoryLabel}
            reason={reason}
            confirmationText={config.form.confirmation_text || "Your absence has been saved."}
            subjects={[
              {
                subjectCode: finalResult.course_code ?? "",
                subjectName: finalResult.course_name ?? "",
                sessionCount: selectedSessionCount,
                sitInMethod: finalResult.sit_in_method ?? undefined,
                sitInCourseCode: finalResult.sit_in_course_code ?? undefined,
                submitStatus: "success",
              },
            ]}
          />
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
            <PageHeading>Report an Absence</PageHeading>
            <p className="max-w-2xl text-sm text-gray-700 font-medium">
              To submit this absence, your parent or guardian will need to confirm it by text message.
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
          parentPhoneMissing={parentPhoneMissing}
          lookup={lookup}
          online={online}
          justRestored={justRestored}
          onClearPageError={() => setPageError(null)}
          onGoToVerification={() => goTo(0)}
          onGoToStep={goTo}
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
                  Lookup & verify
                </h2>
                <div className="grid gap-4">
                  <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
                    <label className="block text-sm font-medium text-gray-800">
                      W-Code
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
                        Look up
                      </Button>
                    </div>
                  </div>

                  {lookup ? (
                    <div className="space-y-6 animate-fade-in mt-4">
                      {/* Clean visual separator and distinct Student Info card */}
                      <div className="rounded-sm border border-gray-250 bg-gray-50 p-5 shadow-sm">
                        <h3 className="text-xs font-semibold uppercase tracking-wider text-gray-600 mb-2">Student Profile</h3>
                        <div className="flex flex-wrap items-start justify-between gap-3">
                          <div>
                            <p className="text-base font-semibold text-[var(--color-wi-text)]">{lookup.full_name}</p>
                            <p className="text-sm font-mono text-gray-700 mt-0.5">{lookup.wcode}</p>
                          </div>
                          <div className="rounded-full border border-gray-300 bg-white px-3 py-1 text-xs font-medium text-gray-700">
                            {lookup.parent_phone ? `Parent phone ${maskPhone(lookup.parent_phone)}` : "No parent phone on file"}
                          </div>
                        </div>
                      </div>

                      <hr className="border-gray-200" />

                      {/* Verification section cleanly separate, not nested in a card wrapper */}
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
                          setDateFrom("");
                          setDateTo("");
                          setReasonCategory("");
                          setReason("");
                          setSessions([]);
                          setSelectedSessionIds(new Set());
                          setCoverSessionIds(new Set());
                           setPageError(null);
                           setSubmissionError(null);
                           setShowReasonFields(false);
                           setVerificationSatisfied(false);
                           verification.clearStoredToken();
                           verification.setCode("");
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
                            Parent verified successfully. Proceed to select courses and dates.
                          </div>
                          <Button
                            variant="primary"
                            size="lg"
                            disabled={!canProceedFromVerify}
                            onClick={() => goTo(1)}
                          >
                            Continue to courses
                            <ChevronRight className="ml-2 h-4 w-4" />
                          </Button>
                        </motion.div>
                      ) : null}
                    </div>
                  ) : null}
                </div>
              </section>
            ) : null}

            {step === 1 ? (
              <section className="rounded-sm border border-gray-200 bg-white p-5 shadow-sm">
                <h2
                  ref={(node) => {
                    stepHeadingRefs.current[1] = node;
                  }}
                  tabIndex={-1}
                  className="text-xl font-semibold text-[var(--color-wi-text)] mb-4"
                >
                  Courses & dates
                </h2>
                <div className="space-y-6">
                  {lookup ? (
                    <div className="space-y-6">
                      <div className="space-y-3">
                        <div className="flex items-center justify-between gap-2">
                          <h3 className="text-sm font-semibold text-[var(--color-wi-text)]">Select your courses</h3>
                          <Button
                            variant="secondary"
                            size="sm"
                            onClick={toggleAllSubjects}
                          >
                            {selectedSubjectCount === lookup.subjects.length ? "Deselect all" : "Select all"}
                          </Button>
                        </div>
                        <div
                          ref={listboxRef}
                          role="listbox"
                          aria-multiselectable="true"
                          aria-label="Select your courses"
                          aria-activedescendant={activeGroup ? `course-chip-${activeGroup.id}` : undefined}
                          tabIndex={0}
                          onKeyDown={handleCourseKeyDown}
                          className="grid gap-2 sm:grid-cols-2"
                        >
                          {lookup.subjects.map((subject) => (
                            <CourseChip
                              key={subject.id}
                              id={`course-chip-${subject.id}`}
                              code={subject.code}
                              cycle={subject.name}
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
                        <p className="text-xs text-gray-650 font-medium">
                          Use arrow keys to move, Space to toggle a course, and Enter to toggle the focused course.
                        </p>
                      </div>

                      <DateRangeInput
                        dateFrom={dateFrom}
                        dateTo={dateTo}
                        maxDays={config.form.max_date_range_days}
                        onDateFromChange={setDateFrom}
                        onDateToChange={setDateTo}
                      />

                      {/* Collapsible Disclosure Section for Reasons — only once courses + dates are partially filled */}
                      {selectedSubjectCount > 0 && dateFrom && dateTo ? (
                        <div className="border border-gray-200 rounded-sm overflow-hidden">
                          <button
                            type="button"
                            className="w-full flex items-center justify-between px-4 py-3 bg-gray-50 hover:bg-gray-100/70 transition-colors text-sm font-semibold text-gray-800"
                            onClick={() => setShowReasonFields(!showReasonFields)}
                            aria-expanded={showReasonFields}
                          >
                            <span className="flex items-center gap-2">
                              Add reason details
                              <span className="rounded-full bg-gray-100 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-gray-500">
                                Optional
                              </span>
                            </span>
                            <ChevronRight
                              className={clsx(
                                "h-4 w-4 text-gray-600 transition-transform duration-200",
                                showReasonFields && "rotate-90"
                              )}
                            />
                          </button>

                          <AnimatePresence initial={false}>
                            {showReasonFields && (
                              <motion.div
                                initial={{ opacity: 0 }}
                                animate={{ opacity: 1 }}
                                exit={{ opacity: 0 }}
                                transition={{ duration: 0.2 }}
                                className="overflow-hidden border-t border-gray-200"
                              >
                                <div className="p-4 bg-white">
                                  <div className="grid gap-4">
                                    <label className="block text-sm font-medium text-gray-700">
                                      Reason category
                                      <select
                                        className="mt-1 w-full rounded-sm border border-gray-300 bg-white px-3 py-2 text-sm text-gray-800 focus:border-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20"
                                        value={reasonCategory}
                                        onChange={(event) => setReasonCategory(event.target.value)}
                                      >
                                        <option value="">Select a reason…</option>
                                        {config.form.reason_categories.map((item) => (
                                          <option key={item.value} value={item.value}>
                                            {item.label}
                                          </option>
                                        ))}
                                      </select>
                                    </label>

                                    {reasonCategory ? (
                                      <motion.label
                                        initial={{ opacity: 0, y: -5 }}
                                        animate={{ opacity: 1, y: 0 }}
                                        className="block text-sm font-medium text-gray-700"
                                      >
                                        <div className="flex items-center justify-between mb-1">
                                          <span>Free-text details</span>
                                          <span className={clsx("text-xs font-semibold", reason.length > 450 ? (reason.length >= 500 ? "text-red-600" : "text-amber-600") : "text-gray-600")}>
                                            {reason.length}/500
                                          </span>
                                        </div>
                                        <textarea
                                          className="w-full min-h-[96px] rounded-sm border border-gray-300 px-3 py-2 text-sm text-gray-850 focus:border-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20"
                                          value={reason}
                                          onChange={(event) => setReason(event.target.value)}
                                          maxLength={500}
                                          placeholder="Provide details about the absence..."
                                        />
                                        {/* Visual Progress Bar for character length */}
                                        <div className="mt-1.5 h-1 w-full bg-gray-100 rounded-full overflow-hidden">
                                          <div
                                            className={clsx(
                                              "h-full transition-all duration-150",
                                              reason.length >= 475 ? "bg-red-500" : reason.length >= 400 ? "bg-amber-500" : "bg-[var(--color-wi-primary)]"
                                            )}
                                            style={{ width: `${(reason.length / 500) * 100}%` }}
                                          />
                                        </div>
                                      </motion.label>
                                    ) : null}
                                  </div>
                                </div>
                              </motion.div>
                            )}
                          </AnimatePresence>
                        </div>
                      ) : null}

                      <div className="flex flex-wrap items-center justify-between gap-3 rounded-sm border border-gray-200 bg-gray-50 p-5 text-sm">
                        <Button variant="secondary" onClick={() => back()}>
                          <ChevronLeft className="mr-1 h-4 w-4" />
                          Back
                        </Button>
                        <Button
                          variant="primary"
                          size="lg"
                          disabled={!canProceedToSessions}
                          onClick={() => {
                            if (validateStepOne()) {
                              goTo(2);
                            }
                          }}
                        >
                          Continue to sessions
                          <ChevronRight className="ml-2 h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <div className="rounded-sm border border-gray-200 bg-gray-50 p-5 text-sm text-gray-650 font-medium">
                      Look up a student first.
                    </div>
                  )}
                </div>
              </section>
            ) : null}

            {step === 2 ? (
              <section className="space-y-4 rounded-sm border border-gray-200 bg-white p-5 shadow-sm">
                <h2
                  ref={(node) => {
                    stepHeadingRefs.current[2] = node;
                  }}
                  tabIndex={-1}
                  className="text-xl font-semibold text-[var(--color-wi-text)]"
                >
                  Sessions & cover
                </h2>
                {activeGroup ? (
                  <div className="space-y-4">
                    <div className="rounded-sm border border-gray-250 bg-gray-50 p-5 text-sm text-gray-700">
                      <div className="font-semibold text-[var(--color-wi-text)]">{activeGroup.code}</div>
                      <div className="font-medium text-gray-800 mt-0.5">{activeGroup.name}</div>
                      <div className="mt-1.5 text-xs text-gray-650 font-semibold">
                        {selectedSubjectCount > 1
                          ? "This report will use the active course card below."
                          : "Selected course."}
                      </div>
                    </div>

                    {sessionsLoading ? (
                      <LoadingSkeleton type="table" lines={5} />
                    ) : null}

                    {activeSessions.length === 0 && !sessionsLoading ? (
                      <EmptyState message="No sessions found in this date range." />
                    ) : null}

                    {activeSessions.map((group) => {
                      const allSelected = group.sessions.every((session) => selectedSessionIds.has(session.id));
                      const allCovered = group.sessions.every((session) => coverSessionIds.has(session.id) && selectedSessionIds.has(session.id));
                      const selectedCount = group.sessions.filter((session) => selectedSessionIds.has(session.id)).length;
                      const coveredCount = group.sessions.filter((session) => coverSessionIds.has(session.id)).length;

                      return (
                        <fieldset key={group.subject_id} className="rounded-sm border border-gray-250 bg-white">
                          <legend className="flex w-full items-center justify-between gap-3 border-b border-gray-150 px-4 py-3 text-sm font-semibold text-[var(--color-wi-text)] bg-gray-50/50">
                            <span>
                              {group.subject_code} - {group.subject_name}
                            </span>
                            <span className="text-xs font-semibold text-gray-650">
                              {selectedCount} selected, {coveredCount} cover
                            </span>
                          </legend>
                          <div className="space-y-4 p-5">
                            <div className="flex flex-wrap items-center gap-2">
                              <Button variant="secondary" size="sm" onClick={() => toggleAllSessionsForGroup(group, !allSelected)}>
                                {allSelected ? "Deselect all" : "Select all"}
                              </Button>
                              <Button variant="secondary" size="sm" onClick={() => toggleAllCoversForGroup(group)}>
                                {allCovered ? "None cover" : "All cover"}
                              </Button>
                              {coveredCount === 0 ? (
                                <span className="rounded-full bg-slate-100 px-3 py-1 text-xs font-semibold text-slate-650 select-none">Cover optional</span>
                              ) : null}
                            </div>

                            <div className="space-y-2">
                              {group.sessions.map((session) => {
                                const selected = selectedSessionIds.has(session.id);
                                const covered = coverSessionIds.has(session.id);
                                return (
                                  <div
                                    key={session.id}
                                    className={`flex flex-col gap-2 rounded-sm border px-4 py-3.5 sm:flex-row sm:items-center transition-colors ${
                                      selected ? "border-[var(--color-wi-primary)] bg-[var(--color-wi-primary)]/5" : "border-gray-200 bg-white"
                                    }`}
                                  >
                                    <div className="flex flex-col sm:flex-row flex-1 items-start sm:items-center gap-2 sm:gap-3 text-sm text-[var(--color-wi-text)]">
                                      <div className="flex items-center gap-3">
                                        <input
                                          type="checkbox"
                                          id={`session-${session.id}`}
                                          checked={selected}
                                          onChange={() => handleSessionToggle(session.id)}
                                          className="h-4 w-4 rounded border-gray-300 text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20"
                                        />
                                        <label htmlFor={`session-${session.id}`} className="cursor-pointer">
                                          <div>
                                            <span className="block font-semibold">{formatDate(session.date)}</span>
                                            <span className="block text-xs text-gray-600 mt-0.5">
                                              {formatTime(session.start_at)} - {formatTime(session.end_at)}
                                            </span>
                                          </div>
                                        </label>
                                      </div>
                                      {selected ? (
                                        <motion.label
                                          initial={{ opacity: 0, scale: 0.95 }}
                                          animate={{ opacity: 1, scale: 1 }}
                                          className="ml-6 sm:ml-0 flex items-center gap-2 text-sm text-amber-800 bg-amber-50 border border-amber-200 rounded-sm px-2.5 py-1 select-none cursor-pointer shrink-0"
                                          onClick={(e) => e.stopPropagation()}
                                        >
                                          <input
                                            type="checkbox"
                                            checked={covered}
                                            onChange={() => handleCoverToggle(session.id)}
                                            className="h-4 w-4 rounded border-gray-350 text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20"
                                          />
                                          <span className="text-xs font-semibold">Needs cover</span>
                                        </motion.label>
                                      ) : null}
                                    </div>
                                  </div>
                                );
                              })}
                            </div>
                          </div>
                        </fieldset>
                      );
                    })}

                    <div className="flex flex-wrap items-center justify-between gap-3 rounded-sm border border-gray-200 bg-gray-50 p-5 text-sm">
                      <Button variant="secondary" onClick={() => back()}>
                        <ChevronLeft className="mr-1 h-4 w-4" />
                        Back
                      </Button>
                      <Button
                        variant="primary"
                        size="lg"
                        disabled={!canProceedToReview}
                        onClick={() => {
                          if (validateStepTwo()) {
                            goTo(3);
                          }
                        }}
                      >
                        Continue to review
                        <ChevronRight className="ml-2 h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="rounded-sm border border-gray-200 bg-gray-50 p-5 text-sm text-gray-650 font-medium">
                    Choose a course first.
                  </div>
                )}
              </section>
            ) : null}

            {step === 3 ? (
              <section className="space-y-4 rounded-sm border border-gray-200 bg-white p-5 shadow-sm">
                <h2
                  ref={(node) => {
                    stepHeadingRefs.current[3] = node;
                  }}
                  tabIndex={-1}
                  className="text-xl font-semibold text-[var(--color-wi-text)]"
                >
                  Review & submit
                </h2>
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="rounded-sm border border-gray-200 bg-gray-50 p-5 text-sm text-gray-700">
                    <div className="text-xs uppercase tracking-wide text-gray-600 font-semibold">Parent phone</div>
                    <div className="mt-1 font-semibold text-gray-800">{maskPhone(lookup?.parent_phone) || "Not available"}</div>
                  </div>
                  <div className="rounded-sm border border-gray-200 bg-gray-50 p-5 text-sm text-gray-700">
                    <div className="text-xs uppercase tracking-wide text-gray-600 font-semibold">Date range</div>
                    <div className="mt-1 font-semibold text-gray-800">
                      {dateFrom ? formatDate(dateFrom) : "Start date"} - {dateTo ? formatDate(dateTo) : "End date"}
                    </div>
                  </div>
                </div>

                <div className="rounded-sm border border-gray-200 bg-white p-5">
                  <div className="text-sm font-semibold text-[var(--color-wi-text)]">
                    {activeGroup?.code} - {activeGroup?.name}
                  </div>
                  <div className="mt-2 text-sm text-gray-700 font-medium">
                    {selectedSessionCount} session{selectedSessionCount === 1 ? "" : "s"} selected, {coverSessionCount} cover session{coverSessionCount === 1 ? "" : "s"}
                  </div>
                  {reasonCategoryLabel ? (
                    <div className="mt-2 text-sm text-gray-700 font-medium">
                      Reason: {reasonCategoryLabel}
                    </div>
                  ) : null}
                  {reason ? <div className="mt-2 text-sm text-gray-600 bg-gray-50 p-3 border border-gray-150 rounded-sm italic">{reason}</div> : null}
                </div>

                <div className="rounded-sm border border-gray-200 bg-gray-50 p-5 text-sm text-gray-700">
                  <h3 className="font-semibold text-[var(--color-wi-text)]">What happens next?</h3>
                  <p className="mt-2 text-gray-600">
                    We will save this absence and place it in the review queue once you submit.
                  </p>
                </div>

                <div className="flex flex-wrap items-center justify-between gap-3 pt-2">
                  <Button variant="secondary" onClick={() => back()}>
                    <ChevronLeft className="mr-1 h-4 w-4" />
                    Back
                  </Button>
                  <Button
                    variant="primary"
                    loading={isSubmitting}
                    onClick={() => void handleSubmitAbsence()}
                  >
                    Submit absence
                  </Button>
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
