import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { motion } from "framer-motion";
import { ChevronLeft } from "lucide-react";
import { useNavigate } from "react-router-dom";
import clsx from "clsx";
import { newIdempotencyKey, ApiRequestError } from "@/api/client";
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
  countSelectedSessions,
  getSelectedSessionsForGroup,
  groupByDay,
  isDayGroupSelected,
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

type StepIndex = 0 | 1 | 2;

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
  const [collectedEmail, setCollectedEmail] = useState("");
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
  const emailSatisfied = !!(lookup?.email_crm?.trim() || lookup?.email_system?.trim() || collectedEmail.trim());
  const canProceedFromVerify = !!lookup && emailSatisfied && verificationSatisfied;
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

  const canSubmit = selectedSubjectCount > 0 && selectedSessionCount > 0 && reason.trim().length > 0 && !verificationBlocked && !missingSitIn;

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
    if (step !== 1 || !lookup) return;
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
      const response = await lookupStudentByWcode(cleaned);
      setLookup(response);
      setLookupInput(cleaned);
      setSelectedSubjectIds([]);
      setCollectedEmail("");
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

  const handleSessionGroupToggle = (sessionIds: string[]) => {
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
      if (selectedSessionCount >= maxSessions) return current;
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

  async function handleSubmitAbsence() {
    setSubmissionError(null);
    setPageError(null);
    if (!validateStepOne()) return;
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
      } else {
        setSubmissionError(error instanceof Error ? error.message : "Could not submit your absence");
      }
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
      <div className="mx-auto max-w-lg px-4 pb-24 pt-6">
        <StepIndicator
          steps={STEP_LABELS}
          currentStep={step}
          onStepClick={(s) => s < step && goToStep(s as StepIndex)}
        />

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
                              className="min-h-[40px] w-full rounded-lg border border-[var(--color-wi-border)] bg-white px-3.5 text-sm text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                              placeholder="e.g. student@example.com"
                              value={collectedEmail}
                              onChange={(e) => setCollectedEmail(e.target.value)}
                            />
                            {!collectedEmail.trim() && (
                              <p className="text-xs text-[var(--color-wi-amber)]">An email is required so we can contact you about your absence.</p>
                            )}
                          </div>
                        )}

                        <div className="border-t border-[var(--color-wi-border)] mt-4 pt-4">
                          <StepCoverVerification
                            wcode={lookup.wcode}
                            parentPhone={lookup.parent_phone}
                            allowSubmitWithoutOtp={config.notifications?.allow_submit_without_otp ?? false}
                            adminContact={config.admin_contact}
                            verification={verification}
                            completed={verificationSatisfied}
                            onSatisfied={handleVerificationSatisfied}
                            onRestart={handleVerificationRestart}
                            onRestored={handleVerificationRestored}
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
                              const sessionGroups = groupByDay(group.sessions);
                              const selectedCount = sessionGroups.filter((sessionGroup) => isDayGroupSelected(sessionGroup, selectedSessionIds)).length;
                              const groupLabel = group.subject_name?.trim() || group.course_name?.trim() || group.course_code;
                              return (
                                <div key={group.course_id} className="rounded-lg border border-[var(--color-wi-border)] bg-white overflow-hidden shadow-sm">
                                  <div className="flex items-center justify-between gap-2 border-b border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] px-4 py-3">
                                    <span className="text-sm font-semibold text-[var(--color-wi-text)] truncate">{groupLabel} ({sessionGroups.length} class day{sessionGroups.length !== 1 ? "s" : ""})</span>
                                    <span className="text-xs font-semibold text-[var(--color-wi-text-light)] shrink-0">{selectedCount} selected</span>
                                  </div>
                                  {group.absence_rate_exceeded ? (
                                    <div className="p-4">
                                      <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2.5 text-sm text-amber-800">
                                        <svg className="mt-0.5 h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                          <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
                                        </svg>
                                        <span>
                                          You have reached the maximum absences allowed for this course.
                                          {group.existing_absence_count != null && group.total_session_count != null && group.total_session_count > 0
                                            ? ` (${group.existing_absence_count} absence${group.existing_absence_count !== 1 ? "s" : ""} used, max ${Math.floor((group.total_session_count - 1) / 5)})`
                                            : ""}
                                        </span>
                                      </div>
                                    </div>
                                  ) : (
                                  <div className="space-y-2 p-4">
                                    {sessionGroups.map((dayGroup) => {
                                      const session = dayGroup.items[0];
                                      const sessionIds = dayGroup.items.map((item) => item.id);
                                      const selected = isDayGroupSelected(dayGroup, selectedSessionIds);
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
                                          "rounded-lg border px-4 py-3 transition-colors",
                                          selected ? "border-[var(--color-wi-primary)]/30 bg-[var(--color-wi-primary)]/5" : "border-[var(--color-wi-border)] bg-white",
                                        )}>
                                          <div className="flex items-center gap-3">
                                            <input
                                              type="checkbox"
                                              id={`session-${dayGroup.id}`}
                                              checked={selected}
                                              disabled={!selected && atMaxSessions}
                                              onChange={() => handleSessionGroupToggle(sessionIds)}
                                              className="h-4 w-4 shrink-0 rounded border-[var(--color-wi-border)] text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20 disabled:opacity-50 disabled:cursor-not-allowed"
                                            />
                                            <label htmlFor={`session-${dayGroup.id}`} className="min-w-0 cursor-pointer flex-1">
                                              <span className="text-sm font-semibold text-[var(--color-wi-text)]">
                                                {formatDate(dayGroup.date)} {formatTime(dayGroup.start_at)}-{formatTime(dayGroup.end_at)}
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
