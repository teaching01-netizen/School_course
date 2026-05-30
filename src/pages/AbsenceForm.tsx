import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { Check, ChevronLeft, ChevronRight, Copy } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { apiJson, newIdempotencyKey } from "@/api/client";
import Button from "@/components/ui/Button";
import Input from "@/components/ui/Input";
import PageHeading from "@/components/ui/PageHeading";
import LoadingSkeleton from "@/components/ui/LoadingSkeleton";
import EmptyState from "@/components/ui/EmptyState";
import CourseChip from "@/components/absences/CourseChip";
import StepCoverVerification from "@/components/absences/StepCoverVerification";
import ConfirmationSummary from "@/components/absences/ConfirmationSummary";
import { useToast } from "@/hooks/useToast";
import { useConnectivity } from "@/hooks/useConnectivity";
import { useOtp } from "@/hooks/useOtp";
import { useWizard } from "@/hooks/useWizard";
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

function formatDate(iso: string): string {
  return new Date(`${iso}T00:00:00`).toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString("en-GB", {
    weekday: "short",
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
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
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submissionError, setSubmissionError] = useState<string | null>(null);
  const [finalResult, setFinalResult] = useState<ManagedAbsence | null>(null);
  const [copiedReference, setCopiedReference] = useState(false);

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
    daysBetween(dateFrom, dateTo) >= 0;
  const canProceedToReview =
    !!activeGroup &&
    !!dateFrom &&
    !!dateTo &&
    daysBetween(dateFrom, dateTo) >= 0 &&
    selectedSessionCount > 0;

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
  }, [lookup, selectedSubjectIds, activeCourseIndex]);

  useEffect(() => {
    if (!verificationSatisfied && step > 0) {
      goTo(0);
    }
  }, [goTo, step, verificationSatisfied]);

  useEffect(() => {
    if (!lookup) {
      goTo(0);
    }
  }, [goTo, lookup]);

  useEffect(() => {
    const heading = stepHeadingRefs.current[step];
    if (heading) {
      window.setTimeout(() => heading.focus({ preventScroll: false }), 0);
    }
  }, [step]);

  useEffect(() => {
    if (finalResult) {
      window.setTimeout(() => resultHeadingRef.current?.focus({ preventScroll: false }), 0);
    }
  }, [finalResult]);

  useEffect(() => {
    if (!pageError) return;
    const timer = window.setTimeout(() => setPageError(null), 5000);
    return () => window.clearTimeout(timer);
  }, [pageError]);

  useEffect(() => {
    const beforeUnload = (event: BeforeUnloadEvent) => {
      if (!lookup || finalResult) return;
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", beforeUnload);
    return () => window.removeEventListener("beforeunload", beforeUnload);
  }, [lookup, finalResult]);

  useEffect(() => {
    if (justRestored) {
      addToast("info", "Connection restored");
    }
  }, [justRestored, addToast]);

  function updateCourseAnnouncement(next: string) {
    setCourseAnnouncement(next);
  }

  const handleVerificationSatisfied = useCallback(() => {
    setVerificationSatisfied(true);
    setPageError(null);
  }, []);

  function applySelectedSubjects(nextSet: Set<string>, preferredActive?: string) {
    if (!lookup) return;
    const ordered = lookup.subjects.filter((subject) => nextSet.has(subject.id)).map((subject) => subject.id);
    setSelectedSubjectIds(ordered);
    if (preferredActive && ordered.includes(preferredActive)) {
      setActiveCourseIndex(Math.max(0, lookup.subjects.findIndex((subject) => subject.id === preferredActive)));
      return;
    }
    if (ordered.length > 0) {
      setActiveCourseIndex(Math.max(0, lookup.subjects.findIndex((subject) => subject.id === ordered[0])));
    } else {
      setActiveCourseIndex(0);
    }
  }

  function toggleSubject(subjectId: string) {
    if (!lookup) return;
    const next = new Set(selectedSubjectIds);
    if (next.has(subjectId)) {
      next.delete(subjectId);
    } else {
      next.add(subjectId);
    }
    applySelectedSubjects(next, subjectId);
    const selectedCount = next.size;
    const subject = lookup.subjects.find((item) => item.id === subjectId);
    updateCourseAnnouncement(
      `${subject?.code ?? "Course"} ${next.has(subjectId) ? "selected" : "deselected"} (${selectedCount} of ${lookup.subjects.length} selected)`,
    );
  }

  function toggleAllSubjects() {
    if (!lookup) return;
    const allSelected = selectedSubjectIds.length === lookup.subjects.length;
    const next = new Set<string>();
    if (!allSelected) {
      lookup.subjects.forEach((subject) => next.add(subject.id));
    }
    applySelectedSubjects(next, lookup.subjects[0]?.id);
    updateCourseAnnouncement(
      allSelected
        ? "All courses deselected"
        : `${lookup.subjects.length} courses selected`,
    );
  }

  function handleCourseKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (!lookup || lookup.subjects.length === 0) return;

    const currentIndex = Math.max(0, Math.min(activeCourseIndex, lookup.subjects.length - 1));
    let nextIndex = currentIndex;

    if (event.key === "ArrowRight" || event.key === "ArrowDown") {
      nextIndex = (currentIndex + 1) % lookup.subjects.length;
    } else if (event.key === "ArrowLeft" || event.key === "ArrowUp") {
      nextIndex = (currentIndex - 1 + lookup.subjects.length) % lookup.subjects.length;
    } else if (event.key === "Home") {
      nextIndex = 0;
    } else if (event.key === "End") {
      nextIndex = lookup.subjects.length - 1;
    } else if (event.key === " " || event.key === "Enter") {
      event.preventDefault();
      toggleSubject(lookup.subjects[currentIndex].id);
      return;
    } else if (event.key.length === 1) {
      const buffer = `${typeaheadRef.current.buffer}${event.key}`.toLowerCase();
      typeaheadRef.current.buffer = buffer;
      if (typeaheadRef.current.timer) {
        window.clearTimeout(typeaheadRef.current.timer);
      }
      typeaheadRef.current.timer = window.setTimeout(() => {
        typeaheadRef.current.buffer = "";
      }, 500);

      const matchIndex = lookup.subjects.findIndex((subject) => {
        const search = `${subject.code} ${subject.name}`.toLowerCase();
        return search.startsWith(buffer);
      });
      if (matchIndex >= 0) {
        nextIndex = matchIndex;
      } else {
        return;
      }
    } else {
      return;
    }

    event.preventDefault();
    setActiveCourseIndex(nextIndex);
    listboxRef.current?.setAttribute("aria-activedescendant", `course-chip-${lookup.subjects[nextIndex].id}`);
  }

  async function handleLookup() {
    const value = lookupInput.trim();
    if (!value) {
      setLookupError("Enter your W-Code");
      return;
    }

    setLookupLoading(true);
    setLookupError(null);
    try {
      const response = await apiJson<StudentLookupResponse>(
        `/api/v1/absences/student-lookup?wcode=${encodeURIComponent(value)}`,
        { method: "GET" },
      );
      setLookup(response);
      setSelectedSubjectIds(response.subjects.map((subject) => subject.id));
      setActiveCourseIndex(0);
      setDateFrom("");
      setDateTo("");
      setReasonCategory("");
      setReason("");
      setSessions([]);
      setSelectedSessionIds(new Set());
      setCoverSessionIds(new Set());
      setVerificationSatisfied(false);
      verification.clearStoredToken();
      verification.setCode("");
      setSubmissionError(null);
      setPageError(null);
      addToast("success", "Student found");
    } catch (error) {
      setLookup(null);
      setSelectedSubjectIds([]);
      setActiveCourseIndex(0);
      setVerificationSatisfied(false);
      verification.clearStoredToken();
      verification.setCode("");
      setSubmissionError(null);
      setLookupError(error instanceof Error ? error.message : "Student not found");
    } finally {
      setLookupLoading(false);
    }
  }

  function handleSessionToggle(sessionId: string) {
    setSelectedSessionIds((current) => {
      const next = new Set(current);
      if (next.has(sessionId)) {
        next.delete(sessionId);
        setCoverSessionIds((covers) => {
          const nextCovers = new Set(covers);
          nextCovers.delete(sessionId);
          return nextCovers;
        });
      } else {
        next.add(sessionId);
      }
      return next;
    });
  }

  function handleCoverToggle(sessionId: string) {
    if (!selectedSessionIds.has(sessionId)) return;
    setCoverSessionIds((current) => {
      const next = new Set(current);
      if (next.has(sessionId)) {
        next.delete(sessionId);
      } else {
        next.add(sessionId);
      }
      return next;
    });
  }

  function toggleAllSessionsForGroup(group: SubjectSessions, selected: boolean) {
    setSelectedSessionIds((current) => {
      const next = new Set(current);
      for (const session of group.sessions) {
        if (selected) {
          next.add(session.id);
        } else {
          next.delete(session.id);
          setCoverSessionIds((covers) => {
            const nextCovers = new Set(covers);
            nextCovers.delete(session.id);
            return nextCovers;
          });
        }
      }
      return next;
    });
  }

  function toggleAllCoversForGroup(group: SubjectSessions) {
    const allCovered = group.sessions.every((session) => coverSessionIds.has(session.id));
    setCoverSessionIds((current) => {
      const next = new Set(current);
      group.sessions.forEach((session) => {
        if (!selectedSessionIds.has(session.id)) return;
        if (allCovered) {
          next.delete(session.id);
        } else {
          next.add(session.id);
        }
      });
      return next;
    });
  }

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

  const renderStatusBanner = () => {
    if (online && !justRestored) return null;
    return (
      <div
        className={`rounded-sm border px-4 py-3 text-sm ${
          online ? "border-emerald-200 bg-emerald-50 text-emerald-900" : "border-amber-200 bg-amber-50 text-amber-900"
        }`}
        role="status"
        aria-live="polite"
      >
        {online ? "Connection restored." : "You are offline. Your selections are saved locally."}
      </div>
    );
  };

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
          {renderStatusBanner()}
          <section
            className="rounded-sm border border-emerald-200 bg-white p-5 shadow-sm"
            aria-live="polite"
          >
            <h2 ref={resultHeadingRef} tabIndex={-1} className="text-xl font-semibold text-[var(--color-wi-text)]">
              Submission complete
            </h2>
            <p className="mt-2 text-sm text-gray-600">
              Your absence has been saved and is waiting for review.
            </p>
            <div className="mt-4 flex flex-wrap items-center gap-2 rounded-sm border border-gray-200 bg-gray-50 px-4 py-3">
              <span className="text-xs uppercase tracking-wide text-gray-500">Reference</span>
              <span className="font-mono text-sm font-semibold">{reference}</span>
              <Button variant="secondary" size="sm" onClick={() => void copyReference()}>
                <Copy className="mr-1 h-4 w-4" />
                {copiedReference ? "Copied" : "Copy"}
              </Button>
            </div>
          </section>
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
    exit: { opacity: 0, x: reduceMotion ? 0 : direction === "forward" ? -12 : 12 },
    transition: { duration: reduceMotion ? 0 : direction === "forward" ? 0.25 : 0.18, ease: "easeOut" as const },
  };

  return (
    <div className="min-h-screen bg-[radial-gradient(circle_at_top,_rgba(17,24,39,0.03),_transparent_40%),linear-gradient(180deg,_#f8fafc_0%,_#ffffff_100%)] px-4 py-8">
      <div className="mx-auto max-w-4xl space-y-5">
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-2">
            <PageHeading>Report an Absence</PageHeading>
            <p className="max-w-2xl text-sm text-gray-600">
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

        {renderStatusBanner()}

        {pageError ? (
          <div role="alert" className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-900">
            {pageError}
          </div>
        ) : null}

        {lookupError ? (
          <div role="alert" className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-900">
            {lookupError}
          </div>
        ) : null}

        {sessionsError ? (
          <div role="alert" className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-900">
            {sessionsError}
          </div>
        ) : null}

        {lookup && parentPhoneMissing ? (
          <div role="status" className="rounded-sm border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
            No parent phone number is on file for this student. Contact the school office before submitting.
          </div>
        ) : null}

        <div className="rounded-sm border border-gray-200 bg-white p-3 shadow-sm">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            {STEP_LABELS.map((label, index) => (
              <button
                key={label}
                type="button"
                className={`inline-flex min-h-[44px] items-center gap-2 rounded-sm px-3 py-2 ${
                  index === step
                    ? "bg-[var(--color-wi-primary)]/10 text-[var(--color-wi-primary)]"
                    : index < step
                      ? "text-gray-700 hover:bg-gray-50"
                      : "text-gray-400"
                }`}
                onClick={() => {
                  if (index < step) goTo(index as StepIndex);
                }}
                disabled={index > step || isTransitioning}
                aria-label={`Step ${index + 1}: ${label}`}
              >
                <span className="inline-flex h-5 w-5 items-center justify-center rounded-full border border-current/20 text-[10px] font-bold">
                  {index < step ? <Check className="h-3 w-3" /> : index + 1}
                </span>
                <span className="hidden sm:inline">{label}</span>
              </button>
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
                  className="text-xl font-semibold text-[var(--color-wi-text)]"
                >
                  Lookup & verify
                </h2>
                <div className="mt-4 grid gap-4">
                  <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
                    <label className="block text-sm text-gray-700">
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
                    <div className="space-y-4 rounded-sm border border-gray-200 bg-gray-50 p-4">
                      <div className="flex flex-wrap items-start justify-between gap-3">
                        <div>
                          <p className="text-sm font-semibold text-[var(--color-wi-text)]">{lookup.full_name}</p>
                          <p className="text-sm text-gray-600">{lookup.wcode}</p>
                        </div>
                        <div className="rounded-full border border-gray-200 bg-white px-3 py-1 text-xs text-gray-600">
                          {lookup.parent_phone ? `Parent phone ${maskPhone(lookup.parent_phone)}` : "No parent phone on file"}
                        </div>
                      </div>

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
                          setVerificationSatisfied(false);
                          verification.clearStoredToken();
                          verification.setCode("");
                        }}
                      />

                      {verificationSatisfied ? (
                        <div className="flex flex-wrap items-center justify-between gap-3 rounded-sm border border-gray-200 bg-white p-4 text-sm">
                          <div className="text-gray-600">
                            Parent verified. Proceed to select courses and dates.
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
                        </div>
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
                  className="text-xl font-semibold text-[var(--color-wi-text)]"
                >
                  Courses & dates
                </h2>
                <div className="mt-4 space-y-4">
                  {lookup ? (
                    <div className="space-y-4">
                      <div className="space-y-2">
                        <div className="flex items-center justify-between gap-2">
                          <h3 className="text-sm font-semibold text-[var(--color-wi-text)]">Select your courses</h3>
                          <button
                            type="button"
                            className="text-sm font-medium text-[var(--color-wi-primary)] hover:underline"
                            onClick={toggleAllSubjects}
                          >
                            {selectedSubjectCount === lookup.subjects.length ? "Deselect all" : "Select all"}
                          </button>
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
                        <p className="text-xs text-gray-500">
                          Use arrow keys to move, Space to toggle a course, and Enter to toggle the focused course.
                        </p>
                      </div>

                      <div className="grid gap-3 sm:grid-cols-2">
                        <label className="block text-sm text-gray-700">
                          From
                          <Input
                            className="mt-1"
                            type="date"
                            value={dateFrom}
                            onChange={(event) => setDateFrom(event.target.value)}
                          />
                        </label>
                        <label className="block text-sm text-gray-700">
                          To
                          <Input
                            className="mt-1"
                            type="date"
                            value={dateTo}
                            onChange={(event) => setDateTo(event.target.value)}
                          />
                        </label>
                      </div>

                      <div className="grid gap-3">
                        <label className="block text-sm text-gray-700">
                          Reason category
                          <select
                            className="mt-1 w-full rounded-sm border border-gray-300 bg-white px-3 py-2 text-sm"
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
                        <label className="block text-sm text-gray-700">
                          <div className="flex items-center justify-between">
                            <span>Free-text details</span>
                            <span className={`text-xs ${reason.length > 450 ? (reason.length >= 500 ? "text-red-600" : "text-amber-600") : "text-gray-500"}`}>
                              {reason.length}/500
                            </span>
                          </div>
                          <textarea
                            className="mt-1 min-h-[96px] w-full rounded-sm border border-gray-300 px-3 py-2 text-sm"
                            value={reason}
                            onChange={(event) => setReason(event.target.value)}
                            maxLength={500}
                          />
                        </label>
                      </div>

                      <div className="flex flex-wrap items-center justify-between gap-3 rounded-sm border border-gray-200 bg-white p-4 text-sm">
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
                    <div className="rounded-sm border border-gray-200 bg-gray-50 p-4 text-sm text-gray-600">
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
                    <div className="rounded-sm border border-gray-200 bg-gray-50 p-4 text-sm text-gray-700">
                      <div className="font-semibold text-[var(--color-wi-text)]">{activeGroup.code}</div>
                      <div>{activeGroup.name}</div>
                      <div className="mt-1 text-xs text-gray-500">
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
                        <fieldset key={group.subject_id} className="rounded-sm border border-gray-200 bg-white">
                          <legend className="flex w-full items-center justify-between gap-3 border-b border-gray-100 px-4 py-3 text-sm font-semibold text-[var(--color-wi-text)]">
                            <span>
                              {group.subject_code} - {group.subject_name}
                            </span>
                            <span className="text-xs font-normal text-gray-500">
                              {selectedCount} selected, {coveredCount} cover
                            </span>
                          </legend>
                          <div className="space-y-2 p-4">
                            <div className="flex flex-wrap items-center gap-2">
                              <Button variant="secondary" size="sm" onClick={() => toggleAllSessionsForGroup(group, !allSelected)}>
                                {allSelected ? "Deselect all" : "Select all"}
                              </Button>
                              <Button variant="secondary" size="sm" onClick={() => toggleAllCoversForGroup(group)}>
                                {allCovered ? "None cover" : "All cover"}
                              </Button>
                              {coveredCount === 0 ? (
                                <span className="rounded-full bg-gray-100 px-3 py-1 text-xs text-gray-600">No cover needed</span>
                              ) : null}
                            </div>

                            <div className="space-y-2">
                              {group.sessions.map((session) => {
                                const selected = selectedSessionIds.has(session.id);
                                const covered = coverSessionIds.has(session.id);
                                return (
                                  <div
                                    key={session.id}
                                    className={`flex flex-col gap-3 rounded-sm border px-3 py-3 sm:flex-row sm:items-center sm:justify-between ${
                                      selected ? "border-[var(--color-wi-primary)] bg-[var(--color-wi-primary)]/5" : "border-gray-200 bg-white"
                                    }`}
                                  >
                                    <label className="flex flex-1 cursor-pointer items-start gap-3 text-sm text-[var(--color-wi-text)]">
                                      <input
                                        type="checkbox"
                                        checked={selected}
                                        onChange={() => handleSessionToggle(session.id)}
                                        className="mt-1 h-4 w-4 rounded border-gray-300 text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20"
                                      />
                                      <span>
                                        <span className="block font-medium">{formatDate(session.date)}</span>
                                        <span className="block text-xs text-gray-500">
                                          {formatDateTime(session.start_at)} - {formatDateTime(session.end_at)}
                                        </span>
                                      </span>
                                    </label>
                                    <label className="flex items-center gap-2 text-sm text-gray-700">
                                      <input
                                        type="checkbox"
                                        checked={covered}
                                        disabled={!selected}
                                        onChange={() => handleCoverToggle(session.id)}
                                        className="h-4 w-4 rounded border-gray-300 text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20 disabled:cursor-not-allowed"
                                      />
                                      Needs cover
                                    </label>
                                  </div>
                                );
                              })}
                            </div>
                          </div>
                        </fieldset>
                      );
                    })}

                    <div className="flex flex-wrap items-center justify-between gap-3 rounded-sm border border-gray-200 bg-gray-50 p-4 text-sm">
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
                  <div className="rounded-sm border border-gray-200 bg-gray-50 p-4 text-sm text-gray-600">
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
                  <div className="rounded-sm border border-gray-200 bg-gray-50 p-4 text-sm text-gray-700">
                    <div className="text-xs uppercase tracking-wide text-gray-500">Parent phone</div>
                    <div className="mt-1 font-medium">{maskPhone(lookup?.parent_phone) || "Not available"}</div>
                  </div>
                  <div className="rounded-sm border border-gray-200 bg-gray-50 p-4 text-sm text-gray-700">
                    <div className="text-xs uppercase tracking-wide text-gray-500">Date range</div>
                    <div className="mt-1 font-medium">
                      {dateFrom ? formatDate(dateFrom) : "Start date"} - {dateTo ? formatDate(dateTo) : "End date"}
                    </div>
                  </div>
                </div>

                <div className="rounded-sm border border-gray-200 bg-white p-4">
                  <div className="text-sm font-semibold text-[var(--color-wi-text)]">
                    {activeGroup?.code} - {activeGroup?.name}
                  </div>
                  <div className="mt-2 text-sm text-gray-700">
                    {selectedSessionCount} session{selectedSessionCount === 1 ? "" : "s"} selected, {coverSessionCount} cover session{coverSessionCount === 1 ? "" : "s"}
                  </div>
                  {reasonCategoryLabel ? (
                    <div className="mt-2 text-sm text-gray-600">
                      Reason: {reasonCategoryLabel}
                    </div>
                  ) : null}
                  {reason ? <div className="mt-2 text-sm text-gray-600">{reason}</div> : null}
                </div>

                <div className="rounded-sm border border-gray-200 bg-gray-50 p-4 text-sm text-gray-700">
                  <h3 className="font-semibold text-[var(--color-wi-text)]">What happens next?</h3>
                  <p className="mt-2">
                    We will save this absence and place it in the review queue once you submit.
                  </p>
                </div>

                {submissionError ? (
                  <div role="alert" className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-900">
                    {submissionError}
                  </div>
                ) : null}

                <div className="flex flex-wrap items-center justify-between gap-3">
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
