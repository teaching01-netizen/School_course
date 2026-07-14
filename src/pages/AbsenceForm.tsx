import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { motion, useReducedMotion } from "framer-motion";
import { ChevronLeft } from "lucide-react";
import clsx from "clsx";
import { newIdempotencyKey, ApiRequestError } from "@/api/client";
import LoadingSkeleton from "@/components/ui/LoadingSkeleton";
import StepIndicator from "@/components/absences/StepIndicator";
import SubjectCard from "@/components/absences/SubjectCard";
import StickyFooter from "@/components/absences/StickyFooter";
import StepCoverVerification from "@/components/absences/StepCoverVerification";
import StudentStep from "@/components/absences/public-form/StudentStep";
import VerificationStep from "@/components/absences/public-form/VerificationStep";
import ClassesStep from "@/components/absences/public-form/ClassesStep";
import ReviewStep from "@/components/absences/public-form/ReviewStep";
import FormAlert from "@/components/absences/public-form/FormAlert";
import { useToast } from "@/hooks/useToast";
import { useConnectivity } from "@/hooks/useConnectivity";
import { useOtp } from "@/hooks/useOtp";
import { formatDate, formatTime } from "@/utils/date";
import type {
  AbsenceFormConfig,
  ManagedAbsence,
  SubjectSessions,
  StudentLookupResponse,
} from "@/types";
import { DEFAULT_CONFIG, VERIFICATION_STORAGE_KEY } from "@/features/absences/constants";
import {
  loadAbsenceFormConfig,
  loadSessionsInRange,
  lookupStudentByWcode,
  submitAbsenceBatch,
} from "@/features/absences/api/absenceFormApi";
import {
  countSelectedAbsenceDays,
  countSelectedAbsenceDaysForGroup,
  getSelectedSessionsForGroup,
  groupByDay,
  mergedSessionValue,
} from "@/features/absences/domain/sessionGrouping";
import { buildSubmissionPayloads as buildAbsenceSubmissionPayloads } from "@/features/absences/domain/submissionPayload";
import {
  availableSessionsForMissedSessions,
  firstPriorityLevel,
  getCurrentSitInDisplayName,
  getReviewSitInLabel,
  getSitInCourseDisplayName,
  getSitInSessionGroupLabel,
  getSitInSessionLabel,
  groupWithSitInForMissedSession,
  hasPriorityLevel,
  hasServerPriorityReveal,
  nextPriorityLevel,
  previousPriorityLevel,
  prioritiesForLevel,
  rootAvailableSessionsForMissedSessions,
  sitInForMissedSession,
  unavailableSessionsForMissedSession,
} from "@/features/absences/domain/sitInResolution";
import {
  formatBatchAbsenceSummary,
  formatBatchSitInSummary,
} from "@/features/absences/domain/resultSummaries";
import {
  clearLegacyAbsenceDraft,
  clearStudentResume,
  readStudentResume,
  writeStudentResume,
} from "@/features/absences/storage/studentResumeStorage";
import { getStudentDisplayName, maskPhone, normalizeLookupWcode } from "@/features/absences/domain/studentIdentity";

type StepIndex = 0 | 1 | 2 | 3;

export default function AbsenceForm() {
  const { addToast } = useToast();
  const { online, justRestored } = useConnectivity();
  const verification = useOtp(VERIFICATION_STORAGE_KEY);
  const reduceMotion = useReducedMotion();
  const submissionIdempotencyKey = useRef(newIdempotencyKey());
  const lookupRequestId = useRef(0);

  const STEP_LABELS = [
    { label: "Student", description: "Confirm your profile" },
    { label: "Verify", description: "Parent confirmation" },
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
  const [collectedEmail, setCollectedEmail] = useState("");
  const [selectedSubjectIds, setSelectedSubjectIds] = useState<string[]>([]);
  const [expandedSubjectId, setExpandedSubjectId] = useState<string | null>(null);
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
  const pageAlertRef = useRef<HTMLDivElement | null>(null);

  const selectedAbsenceDayCount = useMemo(
    () => countSelectedAbsenceDays(sessions, selectedSessionIds),
    [sessions, selectedSessionIds],
  );
  const maxSessions = config.sit_in.max_sessions_per_absence;

  const remainingForGroup = useCallback(
    (group: SubjectSessions): number => {
      if (group.remaining_absence_days != null) return group.remaining_absence_days;
      return maxSessions;
    },
    [maxSessions],
  );
  const manualEmail = collectedEmail.trim();
  const manualEmailValid = /^[^\s@]+@[^\s@]+$/.test(manualEmail);
  const emailSatisfied = !!(
    lookup?.email_crm?.trim()
    || lookup?.email_system?.trim()
    || manualEmailValid
  );
  const canProceedFromStudent = !!lookup && emailSatisfied;
  const studentDisplayName = getStudentDisplayName(lookup);

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

  useEffect(() => {
    let active = true;
    void loadAbsenceFormConfig()
      .then((data) => {
        if (!active) return;
        setConfig(data);
      })
      .catch((error: unknown) => {
        addToast("error", error instanceof Error ? error.message : "Failed to load form settings");
      })
      .finally(() => { if (active) setConfigLoading(false); });
    return () => { active = false; };
  }, [addToast]);

  useEffect(() => {
    if (step !== 2 || !lookup) return;
    const controller = new AbortController();
    setSessionsLoading(true);
    setSessionsError(null);
    void loadSessionsInRange(lookup.wcode, undefined, undefined, {
      signal: controller.signal,
    })
      .then((data) => { if (!controller.signal.aborted) setSessions(data.subjects); })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setSessions([]);
        setSessionsError(error instanceof Error ? error.message : "Couldn't load your classes");
      })
      .finally(() => { if (!controller.signal.aborted) setSessionsLoading(false); });
    return () => controller.abort();
  }, [step, lookup]);

  useEffect(() => {
    let active = true;
    try {
      clearLegacyAbsenceDraft();
      const resume = readStudentResume();
      if (!resume) return () => { active = false; };
      setLookupInput(resume.wcode);
      if (resume.collectedEmail) setCollectedEmail(resume.collectedEmail);
      setLookupLoading(true);
      void lookupStudentByWcode(resume.wcode)
        .then((response) => { if (active) setLookup(response); })
        .catch((error: unknown) => {
          if (active) setLookupError(error instanceof Error ? error.message : "We couldn't refresh your profile");
        })
        .finally(() => { if (active) setLookupLoading(false); });
    } catch { }
    return () => { active = false; };
  }, []);

  useEffect(() => {
    if (!lookup) return;
    try { writeStudentResume({ wcode: lookup.wcode, collectedEmail }); } catch { }
  }, [lookup, collectedEmail]);

  useEffect(() => {
    if (!verification.token) {
      setVerificationBlocked(false);
      return;
    }
    const expiry = verification.expiresAt;
    if (expiry && expiry < Date.now()) {
      setVerificationBlocked(true);
      setVerificationSatisfied(false);
      setStep((current) => current >= 2 ? 1 : current);
      return;
    }
    setVerificationBlocked(false);
  }, [verification]);

  useEffect(() => {
    if (!verification.token || !verification.expiresAt) return;
    const enforceExpiry = () => {
      if (verification.expiresAt && verification.expiresAt <= Date.now()) {
        setVerificationBlocked(true);
        setVerificationSatisfied(false);
        setStep((current) => current >= 2 ? 1 : current);
      }
    };
    enforceExpiry();
    const timer = window.setInterval(enforceExpiry, 100);
    return () => window.clearInterval(timer);
  }, [verification.expiresAt, verification.token]);

  const handleVerificationSatisfied = useCallback(() => {
    setVerificationSatisfied(true);
    setStep(2);
  }, []);

  const handleVerificationRestart = useCallback(() => {
    verification.clearStoredToken();
    verification.setCode("");
    setVerificationSatisfied(false);
    setVerificationBlocked(false);
  }, [verification.clearStoredToken, verification.setCode]);

  const handleVerificationRestored = useCallback(() => {
    setVerificationSatisfied(true);
    setVerificationBlocked(false);
  }, []);

  const handleLookup = async () => {
    const requestId = ++lookupRequestId.current;
    setLookupError(null);
    setLookup(null);
    setPageError(null);
    const cleaned = normalizeLookupWcode(lookupInput);
    if (!cleaned) {
      setLookupLoading(false);
      setLookupError("Enter your Student ID (W-Code).");
      return;
    }
    try {
      setLookupLoading(true);
      const response = await lookupStudentByWcode(cleaned);
      if (requestId !== lookupRequestId.current) return;
      setLookup(response);
      setLookupInput(cleaned);
      setSelectedSubjectIds([]);
      setExpandedSubjectId(null);
      setCollectedEmail("");
      setReason("");
      setReasonError(null);
      setSessions([]);
      setSessionsError(null);
      setSelectedSessionIds(new Set());
      setSitInSelections({});
      setSitInPriorityLevels({});
      setSitInPriorityHistory({});
      setRevealingPrioritySessionIds(new Set());
      setSubmissionError(null);
      submissionIdempotencyKey.current = newIdempotencyKey();
      verification.clearStoredToken();
      verification.setCode("");
      setVerificationSatisfied(false);
      setVerificationBlocked(false);
    } catch (error) {
      if (requestId !== lookupRequestId.current) return;
      setLookupError(error instanceof Error ? error.message : "We couldn't find your profile");
    } finally {
      if (requestId === lookupRequestId.current) setLookupLoading(false);
    }
  };

  const toggleSubject = (subjectId: string) => {
    setSelectedSubjectIds((current) => {
      if (current.includes(subjectId)) {
        const next = current.filter((id) => id !== subjectId);
        setExpandedSubjectId((expanded) => expanded === subjectId ? next[0] ?? null : expanded);
        return next;
      }
      setExpandedSubjectId(subjectId);
      return [...current, subjectId];
    });
  };

  const handleSessionGroupToggle = (group: SubjectSessions, sessionIds: string[]) => {
    setSelectedSessionIds((current) => {
      const selected = sessionIds.every((sessionId) => current.has(sessionId));
      if (selected) {
        const next = new Set(current);
        for (const sessionId of sessionIds) next.delete(sessionId);
        setSitInSelections((cs) => {
          const n = { ...cs };
          for (const sessionId of sessionIds) delete n[sessionId];
          return n;
        });
        return next;
      }
      const remaining = remainingForGroup(group);
      const currentlySelectedDays = countSelectedAbsenceDaysForGroup(group, current);
      if (currentlySelectedDays + 1 > remaining) return current;
      if (currentlySelectedDays + 1 > maxSessions) return current;
      const next = new Set(current);
      for (const sessionId of sessionIds) next.add(sessionId);
      return next;
    });
  };

  const handleSitInSelectForSessions = (sessionIds: string[], sitInSessionId: string) => {
    setSitInSelections((current) => {
      const next = { ...current };
      for (const sessionId of sessionIds) {
        if (!sitInSessionId) delete next[sessionId];
        else next[sessionId] = sitInSessionId;
      }
      return next;
    });
  };

  const handleNotAvailable = async (group: SubjectSessions, sessionId: string) => {
    const currentLevel = sitInPriorityLevels[sessionId] || group.sit_in?.current_priority_level || firstPriorityLevel(group);
    if (lookup && hasServerPriorityReveal(group)) {
      setRevealingPrioritySessionIds((current) => new Set(current).add(sessionId));
      setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
      setSitInPriorityHistory((prev) => ({ ...prev, [sessionId]: { ...(prev[sessionId] ?? {}), [currentLevel]: group } }));
      try {
        const data = await loadSessionsInRange(
          lookup.wcode,
          undefined,
          undefined,
          undefined,
          { courseIds: [group.course_id], satVerbalAfterPriority: currentLevel },
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
    setPageError(null);
    setSubmissionError(null);
    setStep(next);
    try { window.scrollTo({ top: 0, behavior: "instant" as ScrollBehavior }); } catch { }
  }, []);

  useEffect(() => {
    if (!pageError && !submissionError) return;
    pageAlertRef.current?.focus();
  }, [pageError, submissionError]);

  function focusFirstInvalid(selector: string) {
    window.requestAnimationFrame(() => {
      document.querySelector<HTMLElement>(selector)?.focus();
    });
  }

  function validateClasses() {
    setPageError(null);
    setReasonError(null);
    if (selectedSubjectIds.length === 0) {
      setPageError("Select at least one course.");
      focusFirstInvalid('[id^="subject-"]');
      return false;
    }
    if (selectedAbsenceDayCount === 0) {
      setPageError("Select at least one class you will miss.");
      setExpandedSubjectId(selectedSubjectIds[0] ?? null);
      focusFirstInvalid('[id^="session-"]');
      return false;
    }
    if (missingSitIn) {
      setPageError("Pick a make-up class for all selected sessions before submitting.");
      const invalidGroup = sessions.find((group) =>
        selectedSubjectIds.includes(group.subject_id) && group.sessions.some((session) =>
          selectedSessionIds.has(session.id) && sitInForMissedSession(group, session.id)?.sit_in_method === "physical" && !sitInSelections[session.id]));
      setExpandedSubjectId(invalidGroup?.subject_id ?? selectedSubjectIds[0] ?? null);
      focusFirstInvalid('select[aria-label*="make-up" i], select');
      return false;
    }
    if (!reason.trim()) {
      setPageError("Please tell us why you'll be away.");
      setReasonError("Please tell us why you'll be away.");
      focusFirstInvalid("#absence-reason");
      return false;
    }
    return true;
  }

  async function handleSubmitAbsence() {
    setSubmissionError(null);
    setPageError(null);
    const verificationExpired = Boolean(verification.token && verification.expiresAt && verification.expiresAt < Date.now());
    if (!verificationSatisfied || verificationBlocked || verificationExpired) {
      setVerificationSatisfied(false);
      setVerificationBlocked(true);
      goToStep(1);
      return;
    }
    if (!validateClasses()) return;
    if (!lookup) { setPageError("Search for your profile first."); return; }
    const payloadResult = buildAbsenceSubmissionPayloads({
      lookupWcode: lookup.wcode,
      sessions,
      selectedSubjectIds,
      selectedSessionIds,
      sitInSelections,
      reason,
      maxDateRangeDays: config.form.max_date_range_days,
      sitInPriorityLevels,
      sitInPriorityHistory,
    });
    if (!payloadResult.ok) {
      setPageError(payloadResult.error);
      return;
    }
    const payloads = payloadResult.payloads;
    if (payloads.length === 0) { setPageError("Select at least one class to submit."); return; }
    try {
      setIsSubmitting(true);
      const response = await submitAbsenceBatch({
        idempotencyKey: submissionIdempotencyKey.current,
        wcode: lookup.wcode,
        email: collectedEmail.trim() || undefined,
        reason: reason.trim(),
        verificationToken: verificationSatisfied && verification.token ? verification.token : undefined,
        items: payloads,
      });
      setFinalResults(response.items);
      verification.clearStoredToken();
      verification.setCode("");
      try {
        clearLegacyAbsenceDraft();
        clearStudentResume();
      } catch { }
    } catch (error) {
      if (error instanceof ApiRequestError && error.code === "absence_limit_exceeded") {
        setSubmissionError("You have reached the maximum absences allowed for one or more courses. Please go back and remove those courses.");
      } else if (error instanceof TypeError) {
        setSubmissionError("Your connection was interrupted, so we couldn't confirm whether your absence was received. Stay on this page, check your connection, then tap Submit again. This retry will not create a duplicate.");
      } else {
        setSubmissionError(error instanceof Error ? error.message : "Could not submit your absence");
      }
    } finally {
      setIsSubmitting(false);
    }
  }

  const submissionOverlay = !finalResults && isSubmitting ? (
    <motion.div
      initial={reduceMotion ? false : { opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={reduceMotion ? { duration: 0 } : undefined}
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
                const label = absence.subject_name?.trim() || absence.course_name?.trim() || "Submitted class";
                return (
                  <article key={absence.id} className="rounded-lg border border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] p-4">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div className="min-w-0">
                        <p className="text-sm font-semibold text-[var(--color-wi-text)]">{label}</p>
                        <p className="text-xs text-[var(--color-wi-text-light)]">{formatBatchAbsenceSummary(absence)}</p>
                      </div>
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

        </div>
      </div>
    );
  }

  

  if (configLoading) {
    return (
      <div className="min-h-screen bg-[var(--color-wi-bg)]">
        <div className="mx-auto max-w-lg px-4 pb-24 pt-6">
          <LoadingSkeleton type="text" lines={3} />
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-[var(--color-wi-bg)]">
      <div className="mx-auto max-w-[640px] px-4 pb-24 pt-6 sm:px-6">
        <StepIndicator
          steps={STEP_LABELS}
          currentStep={step}
          onStepClick={(s) => s < step && goToStep(s as StepIndex)}
        />

        {pageError || submissionError ? <FormAlert alertRef={pageAlertRef} message={submissionError || pageError || ""} /> : null}

        <div className="space-y-6">
            {step === 0 && (
              <StudentStep>
                <div className="space-y-4">
                  <div>
                    <label htmlFor="wcode-input" className="block text-sm font-semibold text-[var(--color-wi-text)] mb-1.5">
                      Student ID (W-Code)
                    </label>
                    <div className="flex gap-3">
                      <div className="flex-1">
                        <input
                          id="wcode-input"
                          className="min-h-[48px] w-full rounded-xl border border-[var(--color-wi-border)] bg-white px-4 text-base text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
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
                        className="min-h-[48px] rounded-lg bg-[var(--color-wi-primary)] px-5 text-sm font-semibold text-white transition-colors motion-reduce:transition-none hover:bg-[var(--color-wi-primary-dark)] disabled:opacity-50"
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
                        </div>

                        {lookup.email_crm?.trim() ? (
                          <div className="mt-3 flex items-center gap-2 text-xs">
                            <span className="text-[var(--color-wi-text-light)]">Email:</span>
                            <span className="font-medium text-[var(--color-wi-text)]">{lookup.email_crm}</span>
                            <span className="rounded-full bg-blue-100 px-2 py-0.5 text-[10px] font-medium text-blue-700">CRM</span>
                          </div>
                        ) : lookup.email_system?.trim() ? (
                          <div className="mt-3 flex items-center gap-2 text-xs">
                            <span className="text-[var(--color-wi-text-light)]">Email:</span>
                            <span className="font-medium text-[var(--color-wi-text)]">{lookup.email_system}</span>
                            <span className="rounded-full bg-green-100 px-2 py-0.5 text-[10px] font-medium text-green-700">System</span>
                          </div>
                        ) : (
                          <div className="mt-3 space-y-1.5">
                            <label htmlFor="student-email" className="block text-xs font-medium text-[var(--color-wi-text-light)]">
                              Your email address <span className="text-[var(--color-wi-red)]">*</span>
                            </label>
                            <input
                              id="student-email"
                              type="email"
                              className="min-h-[48px] w-full rounded-xl border border-[var(--color-wi-border)] bg-white px-4 text-base text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                              placeholder="e.g. student@example.com"
                              value={collectedEmail}
                              onChange={(e) => setCollectedEmail(e.target.value)}
                            />
                            {!collectedEmail.trim() && (
                              <p className="text-xs text-[var(--color-wi-amber)]">An email is required so we can contact you about your absence.</p>
                            )}
                          </div>
                        )}

                      </div>

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
              </StudentStep>
            )}

            {step === 1 && (
              lookup ? (
                <VerificationStep
                  studentName={studentDisplayName || lookup.full_name}
                  wcode={lookup.wcode}
                  hasPhone={Boolean(lookup.parent_phone)}
                  phoneLabel={lookup.parent_phone ? `Verification phone: ${maskPhone(lookup.parent_phone)}` : "Verification phone: unavailable"}
                >
                    <StepCoverVerification
                      wcode={lookup.wcode}
                      parentPhone={lookup.parent_phone}
                      smsParentEnabled={config.notifications?.sms_parent_enabled ?? true}
                      adminContact={config.admin_contact}
                      verification={verification}
                      completed={verificationSatisfied}
                      onSatisfied={handleVerificationSatisfied}
                      onRestart={handleVerificationRestart}
                      onRestored={handleVerificationRestored}
                    />
                    {verificationBlocked ? (
                      <div role="alert" className="rounded-xl bg-[var(--color-wi-amber-bg)] p-4 text-sm text-[var(--color-wi-amber)]">
                        Your parent's verification has expired. Please verify again.
                      </div>
                    ) : null}
                </VerificationStep>
              ) : null
            )}

            {step === 2 && (
              <ClassesStep>

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
                          <span className="text-xs font-semibold text-[var(--color-wi-text-light)]">
                            {selectedAbsenceDayCount} selected
                            {sessions.filter(s => selectedSubjectIds.includes(s.subject_id)).reduce((sum, g) => sum + remainingForGroup(g), 0) > 0
                              ? ` (${sessions.filter(s => selectedSubjectIds.includes(s.subject_id)).reduce((sum, g) => sum + remainingForGroup(g), 0)} remaining)`
                              : ""}
                          </span>
                        </div>
                        <div className="mb-4 space-y-2 sm:hidden" aria-label="Selected subjects">
                          {selectedSubjectIds.map((subjectId) => {
                            const subject = lookup.subjects.find((item) => item.id === subjectId);
                            const selectedForSubject = sessions
                              .filter((group) => group.subject_id === subjectId)
                              .reduce((count, group) => count + countSelectedAbsenceDaysForGroup(group, selectedSessionIds), 0);
                            const expanded = expandedSubjectId === subjectId;
                            return (
                              <button
                                key={subjectId}
                                type="button"
                                aria-expanded={expanded}
                                aria-controls={sessions.filter((group) => group.subject_id === subjectId).map((group) => `subject-sessions-${subjectId}-${group.course_id}`).join(" ") || undefined}
                                onClick={() => setExpandedSubjectId(expanded ? null : subjectId)}
                                className="flex min-h-[48px] w-full items-center justify-between rounded-xl border border-[var(--color-wi-border)] bg-white px-4 text-left text-sm font-semibold text-[var(--color-wi-text)]"
                              >
                                <span>{subject?.name ?? subjectId}</span>
                                <span className="text-xs font-medium text-[var(--color-wi-text-light)]">
                                  {selectedForSubject > 0 ? `${selectedForSubject} class day${selectedForSubject === 1 ? "" : "s"} selected` : expanded ? "Open" : "Choose classes"}
                                </span>
                              </button>
                            );
                          })}
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
                              const sessionGroups = groupByDay(group.sessions);
                              const groupLabel = group.subject_name?.trim() || group.course_name?.trim();
                              const groupRemaining = remainingForGroup(group);
                              const selectedDaysInGroup = countSelectedAbsenceDaysForGroup(group, selectedSessionIds);
                              const effectiveRemaining = Math.max(0, groupRemaining - selectedDaysInGroup);
                              return (
                                <div
                                  key={group.course_id}
                                  id={`subject-sessions-${group.subject_id}-${group.course_id}`}
                                  className={clsx(
                                    "rounded-lg border border-[var(--color-wi-border)] bg-white overflow-hidden shadow-sm",
                                    expandedSubjectId !== group.subject_id && "hidden sm:block",
                                  )}
                                >
                                  <div className="flex items-center justify-between gap-2 border-b border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] px-4 py-3">
                                    <span className="text-sm font-semibold text-[var(--color-wi-text)] truncate">{groupLabel} ({sessionGroups.length} class day{sessionGroups.length !== 1 ? "s" : ""})</span>
                                    <span className="text-xs font-semibold text-[var(--color-wi-text-light)] shrink-0">
                                      {group.absence_limit_reached
                                        ? "Limit reached"
                                        : effectiveRemaining === 0
                                          ? "Limit reached"
                                          : `${effectiveRemaining} day${effectiveRemaining !== 1 ? "s" : ""} remaining`}
                                    </span>
                                  </div>
                                  {group.absence_limit_reached ? (
                                    <div className="p-4">
                                      <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2.5 text-sm text-amber-800">
                                        <svg className="mt-0.5 h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                          <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
                                        </svg>
                                        <span>
                                          You have reached the maximum absences allowed for this course.
                                          {group.used_absence_days != null && (group.maximum_absence_days != null || group.total_course_days != null)
                                            ? ` (${group.used_absence_days} absence day${group.used_absence_days !== 1 ? "s" : ""} used, max ${group.maximum_absence_days ?? Math.round((group.total_course_days ?? 0) / 5)})`
                                            : ""}
                                        </span>
                                      </div>
                                    </div>
                                  ) : (
                                  <div className="space-y-2 p-4">
                                    {sessionGroups.map((dayGroup) => {
                                      const session = dayGroup.items[0];
                                      const sessionIds = dayGroup.items
                                        .filter((item) => !item.already_absent)
                                        .map((item) => item.id);
                                      const alreadyAbsent = sessionIds.length === 0;
                                      const selected = !alreadyAbsent
                                        && sessionIds.every((sessionId) => selectedSessionIds.has(sessionId));
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
                                      const sitInAvailable = rootAvailableSessionsForMissedSessions(sitIn, sessionIds);
                                      const hasPriorities = Boolean(sitIn?.priorities && sitIn.priorities.length > 0);
                                      const currentPriorities = hasPriorities ? prioritiesForLevel(priorityGroup, currentLevel) : [];
                                      const sitInClassLabel = getCurrentSitInDisplayName(sitIn, currentPriorities, groupLabel, sessions);

                                      return (
                                        <div key={dayGroup.id} className={clsx(
                                          "rounded-lg border px-4 py-3 transition-colors motion-reduce:transition-none",
                                          selected ? "border-[var(--color-wi-primary)]/30 bg-[var(--color-wi-primary)]/5" : "border-[var(--color-wi-border)] bg-white",
                                        )}>
                                          <div className="flex items-center gap-3">
                                            <input
                                              type="checkbox"
                                              id={`session-${dayGroup.id}`}
                                              checked={selected}
                                              disabled={alreadyAbsent || (!selected && (effectiveRemaining === 0 || selectedDaysInGroup >= maxSessions))}
                                              onChange={() => handleSessionGroupToggle(group, sessionIds)}
                                              className="h-4 w-4 shrink-0 rounded border-[var(--color-wi-border)] text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20 disabled:opacity-50 disabled:cursor-not-allowed"
                                            />
                                            <label htmlFor={`session-${dayGroup.id}`} className="min-w-0 cursor-pointer flex-1">
                                              <span className="text-sm font-semibold text-[var(--color-wi-text)]">
                                                {formatDate(dayGroup.date)} {formatTime(dayGroup.start_at)}-{formatTime(dayGroup.end_at)}
                                              </span>
                                              {alreadyAbsent ? (
                                                <span className="ml-2 text-xs font-medium text-[var(--color-wi-text-light)]">Already reported</span>
                                              ) : null}
                                            </label>
                                          </div>
                                          {selected ? (
                                            <motion.div initial={reduceMotion ? false : { opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} transition={reduceMotion ? { duration: 0 } : undefined} className="mt-3 pl-7">
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
                                                      availableSessionsForMissedSessions(p, sessionIds));
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

                                                          {(hasPreviousPriority || hasMorePriorities) && (
                                                            <div className="inline-flex w-full shrink-0 overflow-hidden rounded-full border border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] p-0.5 sm:w-fit">
                                                              {hasPreviousPriority && (
                                                                <button
                                                                  type="button"
                                                                  disabled={revealingPriority}
                                                                  onClick={() => handlePreviousPriority(priorityGroup, session.id)}
                                                                  aria-label="See previous times"
                                                                  className="inline-flex h-8 flex-1 items-center justify-center gap-1 rounded-full px-2.5 text-xs font-medium text-[var(--color-wi-text-light)] transition motion-reduce:transition-none hover:bg-white hover:text-[var(--color-wi-text)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-amber)]/40 disabled:opacity-50 sm:flex-none"
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
                                                                  className="inline-flex h-8 flex-1 items-center justify-center gap-1 rounded-full px-3 text-xs font-semibold text-[var(--color-wi-text-light)] transition motion-reduce:transition-none hover:bg-white hover:text-[var(--color-wi-text)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-amber)]/40 disabled:opacity-50 sm:flex-none"
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
                                                            onChange={(e) => handleSitInSelectForSessions(sessionIds, e.target.value)}
                                                            className="mt-1.5 w-full rounded-md border border-[var(--color-wi-border)] bg-white px-3 py-2 text-sm text-[var(--color-wi-text)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                                                          >
                                                            <option value="">Not yet selected</option>
                                                            {currentPriorities.flatMap(p =>
                                                              groupByDay(availableSessionsForMissedSessions(p, sessionIds)).map((optionGroup) => (
                                                                <option key={`${p.sit_in_course?.id ?? "course"}:${optionGroup.id}`} value={mergedSessionValue(optionGroup.items)}>
                                                                  {getSitInSessionGroupLabel(optionGroup.items, p.sit_in_course, groupLabel, sessions)}
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
                                                          onChange={(e) => handleSitInSelectForSessions(sessionIds, e.target.value)}
                                                          className="w-full rounded-md border border-[var(--color-wi-border)] bg-white px-3 py-2 text-sm text-[var(--color-wi-text)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                                                        >
                                                          <option value="">— Not yet —</option>
                                                          {groupByDay(sitInAvailable).map((optionGroup) => (
                                                            <option key={optionGroup.id} value={mergedSessionValue(optionGroup.items)}>
                                                              {getSitInSessionGroupLabel(optionGroup.items, sitIn?.sit_in_course, groupLabel, sessions)}
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
                                )}
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
                                "h-full rounded-full transition-all duration-300 motion-reduce:transition-none",
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
                          "w-full min-h-[120px] rounded-xl border bg-white px-4 py-3 text-base text-[var(--color-wi-text)] focus:outline-none focus:ring-2",
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
                      {reasonError ? <p id="reason-error" className="text-xs text-[var(--color-wi-red)] mt-1.5">{reasonError}</p> : null}
                    </section>
                  </div>
                ) : (
                  <p className="text-sm text-[var(--color-wi-text-light)]">Search for your profile first.</p>
                )}
              </ClassesStep>
            )}

            {step === 3 && (
              <ReviewStep>
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
                          onClick={() => goToStep(2)}
                          className="min-h-[32px] text-xs font-semibold text-[var(--color-wi-primary)] transition-colors motion-reduce:transition-none hover:text-[var(--color-wi-primary-dark)]"
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
                              {groupByDay(selectedSessions).map((dayGroup) => (
                                <p key={dayGroup.id} className="text-xs text-[var(--color-wi-text-light)] mt-0.5">
                                  {formatDate(dayGroup.date)} {formatTime(dayGroup.start_at)}–{formatTime(dayGroup.end_at)}
                                  <span className="text-[var(--color-wi-text-light)]"> — Make-up: </span>
                                  <span className="font-medium text-[var(--color-wi-text)]">{getReviewSitInLabel(dayGroup.items[0], group, sitInSelections, sitInPriorityLevels, sitInPriorityHistory, sessions)}</span>
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
                          onClick={() => goToStep(2)}
                          className="min-h-[32px] text-xs font-semibold text-[var(--color-wi-primary)] transition-colors motion-reduce:transition-none hover:text-[var(--color-wi-primary-dark)]"
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
              </ReviewStep>
            )}
        </div>
      </div>

      <StickyFooter
        currentStep={step}
        totalSteps={4}
        canProceed={
          step === 0 ? canProceedFromStudent :
          step === 1 ? verificationSatisfied :
          step === 2 ? !sessionsLoading :
          step === 3 ? verificationSatisfied && !verificationBlocked : false
        }
        loading={isSubmitting}
        onBack={() => goToStep(Math.max(0, step - 1) as StepIndex)}
        onPrimary={() => {
          if (step === 0) goToStep(1);
          else if (step === 1) goToStep(2);
          else if (step === 2) {
            if (!validateClasses()) return;
            goToStep(3);
          } else if (step === 3) void handleSubmitAbsence();
        }}
        primaryLabel={
          step === 0 ? "Continue to verification" :
          step === 1 ? "Continue to classes" :
          step === 2 ? "Review absence" :
          "Submit absence"
        }
      />

      {submissionOverlay}
    </div>
  );
}
