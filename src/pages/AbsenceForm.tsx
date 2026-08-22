import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { motion, useReducedMotion } from "framer-motion";
import { ChevronLeft, LoaderCircle } from "lucide-react";
import clsx from "clsx";
import { newIdempotencyKey, ApiRequestError } from "@/api/client";
import LoadingSkeleton from "@/components/ui/LoadingSkeleton";
import AbsenceAppShell from "@/components/absences/public-form/AbsenceAppShell";
import AbsenceAppHeader from "@/components/absences/public-form/AbsenceAppHeader";
import StepCoverVerification from "@/components/absences/StepCoverVerification";
import StudentStep from "@/components/absences/public-form/StudentStep";
import VerificationStep from "@/components/absences/public-form/VerificationStep";
import ClassesStep from "@/components/absences/public-form/ClassesStep";
import ReviewStep from "@/components/absences/public-form/ReviewStep";
import AbsenceActionBar from "@/components/absences/public-form/AbsenceActionBar";
import SubjectRow from "@/components/absences/public-form/SubjectRow";
import ReasonField from "@/components/absences/public-form/ReasonField";
import SessionDayCard from "@/components/absences/public-form/SessionDayCard";
import MakeUpPicker from "@/components/absences/public-form/MakeUpPicker";
import FormAlert from "@/components/absences/public-form/FormAlert";
import { useToast } from "@/hooks/useToast";
import { useAbsenceDraft } from "@/features/absences/hooks/useAbsenceDraft";
import type { AbsenceDraftV1 } from "@/features/absences/storage/absenceDraftStorage";
import { useConnectivity } from "@/hooks/useConnectivity";
import { useOtp } from "@/hooks/useOtp";
import { formatDate, formatTime } from "@/utils/date";
import type {
  AbsenceFormConfig,
  ManagedAbsence,
  PublicStudentLookupResponse,
  SubjectSessions,
  VerifiedStudentProfile,
} from "@/types";
import { DEFAULT_CONFIG, VERIFICATION_STORAGE_KEY } from "@/features/absences/constants";
import {
  loadAbsenceFormConfig,
  loadStudentProfile,
  loadStudentSessions,
  lookupStudentByWcode,
  submitAbsenceBatch,
} from "@/features/absences/api/absenceFormApi";
import {
  absenceScopeKey,
  countSelectedAbsenceDays,
  countSelectedAbsenceDaysForScope,
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
  clearStudentSessionHint,
  readStudentResume,
  writeStudentResume,
} from "@/features/absences/storage/studentResumeStorage";
import { isWCode, maskNickname, normalizeLookupWcode } from "@/features/absences/domain/studentIdentity";

type StepIndex = 0 | 1 | 2 | 3;

function isStudentSessionUnauthorized(error: unknown): boolean {
  return error instanceof ApiRequestError
    && error.status === 401
    && error.code === "unauthorized";
}

export default function AbsenceForm() {
  const { addToast } = useToast();
  const { online, justRestored } = useConnectivity();
  const verification = useOtp(VERIFICATION_STORAGE_KEY);
  const reduceMotion = useReducedMotion();
  const { draft: savedDraft, saveDraft, clearDraft } = useAbsenceDraft();
  const draftRef = useRef<AbsenceDraftV1 | null>(savedDraft);
  const [draftNeedsReview, setDraftNeedsReview] = useState(false);
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
  const [lookup, setLookup] = useState<PublicStudentLookupResponse | null>(null);
  const [studentProfile, setStudentProfile] = useState<VerifiedStudentProfile | null>(null);
  const [lookupLoading, setLookupLoading] = useState(false);
  const [lookupError, setLookupError] = useState<string | null>(null);
  const [collectedEmail, setCollectedEmail] = useState("");
  const [selectedSubjectIds, setSelectedSubjectIds] = useState<string[]>([]);
  const [expandedSubjectId, setExpandedSubjectId] = useState<string | null>(null);
  const [reason, setReason] = useState("");
  // Nickname lives in memory only — it is never written to the saved draft,
  // mirroring the privacy rule for the enrolled parent phone.
  const [nickname, setNickname] = useState("");
  const [reasonError, setReasonError] = useState<string | null>(null);
  const [sessions, setSessions] = useState<SubjectSessions[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [sessionsError, setSessionsError] = useState<string | null>(null);
  const [sessionsReloadToken, setSessionsReloadToken] = useState(0);
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

  const selectedSubjectIdSet = useMemo(() => new Set(selectedSubjectIds), [selectedSubjectIds]);
  const selectedAbsenceDayCount = useMemo(
    () => countSelectedAbsenceDays(sessions, selectedSessionIds),
    [sessions, selectedSessionIds],
  );
  // Derived indexes: one pass over `sessions` per change instead of per render.
  // groupByDay results are additionally memoized inside sessionGrouping, so
  // these memos mostly cost Map lookups after the first computation.
  const scopeIndex = useMemo(() => {
    const byScope = new Map<string, SubjectSessions[]>();
    for (const group of sessions) {
      const key = absenceScopeKey(group);
      const bucket = byScope.get(key);
      if (bucket) bucket.push(group);
      else byScope.set(key, [group]);
    }
    return byScope;
  }, [sessions]);
  const selectedDaysByScope = useMemo(() => {
    const selectedDays = new Map<string, number>();
    for (const scopeKey of scopeIndex.keys()) {
      selectedDays.set(scopeKey, countSelectedAbsenceDaysForScope(scopeIndex.get(scopeKey)!, selectedSessionIds, scopeKey));
    }
    return selectedDays;
  }, [scopeIndex, selectedSessionIds]);
  const groupDayIndex = useMemo(() => {
    const byCourse = new Map<string, ReturnType<typeof groupByDay<SubjectSessions["sessions"][number]>>>();
    for (const group of sessions) byCourse.set(group.course_id, groupByDay(group.sessions));
    return byCourse;
  }, [sessions]);
  const selectedGroups = useMemo(
    () => sessions.filter((group) => selectedSubjectIdSet.has(group.subject_id)),
    [sessions, selectedSubjectIdSet],
  );
  const maxSessions = config.sit_in.max_sessions_per_absence;

  const remainingForGroup = useCallback(
    (group: SubjectSessions): number => {
      if (group.remaining_absence_days != null) return group.remaining_absence_days;
      return maxSessions;
    },
    [maxSessions],
  );
  const selectedSubjectRemainingDays = useMemo(() => {
    const remainingByScope = new Map<string, number>();
    for (const group of sessions) {
      if (!selectedSubjectIdSet.has(group.subject_id)) continue;
      const scopeKey = absenceScopeKey(group);
      const current = remainingByScope.get(scopeKey) ?? 0;
      remainingByScope.set(scopeKey, Math.max(current, remainingForGroup(group)));
    }
    return [...remainingByScope.values()].reduce((total, remaining) => total + remaining, 0);
  }, [remainingForGroup, selectedSubjectIdSet, sessions]);
  const manualEmail = collectedEmail.trim();
  const manualEmailValid = /^[^\s@]+@[^\s@]+$/.test(manualEmail);
  const emailSatisfied = !!lookup && (!lookup.email_input_required || manualEmailValid);
  const canProceedFromStudent = !!lookup && emailSatisfied;
  // Pre-verification the profile is not loaded, so the masked lookup hint
  // stands in for the name; after verification the real display name shows.
  const studentDisplayName = studentProfile?.display_name || lookup?.nickname_hint || "Student";
  const verifiedSubjects = studentProfile?.subjects ?? [];

  const missingSitIn = useMemo(() => {
    for (const group of sessions) {
      if (!selectedSubjectIdSet.has(group.subject_id)) continue;
      for (const session of group.sessions) {
        if (!selectedSessionIds.has(session.id)) continue;
        const sitIn = sitInForMissedSession(group, session.id);
        if (sitIn?.sit_in_method === "physical" && !sitInSelections[session.id]) return true;
      }
    }
    return false;
  }, [sessions, selectedSubjectIdSet, selectedSessionIds, sitInSelections]);

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
  const handleStudentSessionExpired = useCallback(() => {
    clearStudentSessionHint();
    verification.clearStoredToken();
    verification.setCode("");
    setStudentProfile(null);
    setSessions([]);
    setVerificationSatisfied(false);
    setVerificationBlocked(true);
    setPageError("Your verified session expired. Verify again to continue.");
    setSubmissionError(null);
    setStep(1);
  }, [verification.clearStoredToken, verification.setCode]);

  useEffect(() => {
    if (step !== 2 || !lookup || !online) return;
    const controller = new AbortController();
    setSessionsLoading(true);
    setSessionsError(null);
    void Promise.all([
      loadStudentProfile(),
      loadStudentSessions(undefined, undefined, { signal: controller.signal }),
    ])
      .then(([profile, data]) => {
        if (controller.signal.aborted) return;
        if (profile.wcode !== lookup.wcode) {
          setStudentProfile(null);
          setSessions([]);
          setSessionsError("Verification belongs to a different Student ID. Verify this Student ID again.");
          setStep(1);
          return;
        }
        setStudentProfile(profile);
        setSessions(data.subjects);
        const validSubjectIds = new Set(profile.subjects.map((subject) => subject.id));
        setSelectedSubjectIds((current) => current.filter((id) => validSubjectIds.has(id)));
        const draft = draftRef.current;
        const isSameStudent = Boolean(draft && normalizeLookupWcode(draft.wcode) === lookup.wcode);
        if (!isSameStudent || !draft) return;
        const restoredSubjectIds = draft.selectedSubjectIds.filter((subjectId) => validSubjectIds.has(subjectId));
        setSelectedSubjectIds(restoredSubjectIds);

        const validSessionIds = new Set(data.subjects.flatMap((group) => group.sessions.map((session) => session.id)));
        const restoredSessionIds = draft.selectedSessionIds.filter((sessionId) => validSessionIds.has(sessionId));
        const restoredSessionSet = new Set(restoredSessionIds);
        const missingSavedSessions = draft.selectedSessionIds.length - restoredSessionIds.length;
        const restoredSitIns: Record<string, string> = {};
        for (const [sessionId, sitInId] of Object.entries(draft.sitInSelections)) {
          if (restoredSessionSet.has(sessionId)) restoredSitIns[sessionId] = sitInId;
        }
        const restoredPriorityLevels: Record<string, number> = {};
        for (const [sessionId, priority] of Object.entries(draft.sitInPriorityLevels)) {
          if (restoredSessionSet.has(sessionId)) restoredPriorityLevels[sessionId] = priority;
        }
        setSelectedSessionIds(restoredSessionSet);
        setSitInSelections(restoredSitIns);
        setSitInPriorityLevels(restoredPriorityLevels);
        setExpandedSubjectId(restoredSubjectIds[0] ?? null);
        setDraftNeedsReview((current) => current || missingSavedSessions > 0);
        draftRef.current = null;
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        if (isStudentSessionUnauthorized(error)) {
          handleStudentSessionExpired();
          return;
        }
        setStudentProfile(null);
        setSessions([]);
        setSessionsError(error instanceof Error ? error.message : "Couldn't load your classes");
      })
      .finally(() => { if (!controller.signal.aborted) setSessionsLoading(false); });
    return () => controller.abort();
  }, [step, lookup, online, sessionsReloadToken, handleStudentSessionExpired]);

  useEffect(() => {
    let active = true;
    try {
      clearLegacyAbsenceDraft();
      const resume = readStudentResume();
      const draft = draftRef.current;
      const resumeWcode = draft?.wcode ?? resume?.wcode;
      if (!resumeWcode) return () => { active = false; };
      const restoredEmail = draft?.collectedEmail ?? resume?.collectedEmail;
      setLookupInput(resumeWcode);
      if (restoredEmail) setCollectedEmail(restoredEmail);
      if (draft?.reason) setReason(draft.reason);
      setLookupLoading(true);
      void lookupStudentByWcode(resumeWcode)
        .then((response) => {
          if (!active) return;
          setLookup(response);
          setStudentProfile(null);
          const restoreDraft = draftRef.current;
          const isSameStudent = Boolean(restoreDraft && normalizeLookupWcode(restoreDraft.wcode) === response.wcode);
          setSelectedSubjectIds([]);
          if (isSameStudent && restoreDraft) {
            setReason(restoreDraft.reason);
            setDraftNeedsReview(restoreDraft.selectedSubjectIds.length > 0);
          }
        })
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
    if (!lookup || finalResults) return;
    saveDraft({
      wcode: lookup.wcode,
      collectedEmail: collectedEmail || undefined,
      step,
      selectedSubjectIds: [...selectedSubjectIds],
      selectedSessionIds: [...selectedSessionIds],
      sitInSelections: { ...sitInSelections },
      sitInPriorityLevels: { ...sitInPriorityLevels },
      reason,
    });
  }, [
    lookup,
    finalResults,
    collectedEmail,
    step,
    selectedSubjectIds,
    selectedSessionIds,
    sitInSelections,
    sitInPriorityLevels,
    reason,
    saveDraft,
  ]);

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
    // One timeout at the expiry instant instead of polling a clock check.
    const timer = window.setTimeout(enforceExpiry, Math.max(0, verification.expiresAt - Date.now()));
    return () => window.clearTimeout(timer);
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
    const cleaned = normalizeLookupWcode(lookupInput);
    setLookupError(null);
    if (!online) {
      setLookupError("You're offline. Reconnect to search for your profile.");
      return;
    }
    clearStudentSessionHint();
    setLookup(null);
    setStudentProfile(null);
    if (!cleaned || !isWCode(cleaned)) {
      setLookupLoading(false);
      setLookupError("Enter your Student ID (W-Code).");
      return;
    }
    const draftForStudent = draftRef.current;
    const shouldRestoreDraft = Boolean(
      draftForStudent && normalizeLookupWcode(draftForStudent.wcode) === cleaned,
    );
    if (!shouldRestoreDraft) {
      draftRef.current = null;
      clearDraft();
    }
    try {
      setLookupLoading(true);
      const response = await lookupStudentByWcode(cleaned);
      if (requestId !== lookupRequestId.current) return;
      setLookup(response);
      setStudentProfile(null);
      setLookupInput(cleaned);
      setSelectedSubjectIds([]);
      setExpandedSubjectId(null);
      setCollectedEmail(shouldRestoreDraft ? draftForStudent?.collectedEmail ?? "" : "");
      setReason(shouldRestoreDraft ? draftForStudent?.reason ?? "" : "");
      setNickname("");
      setReasonError(null);
      setSessions([]);
      setSessionsError(null);
      setSelectedSessionIds(new Set());
      setSitInSelections({});
      setSitInPriorityLevels({});
      setSitInPriorityHistory({});
      setRevealingPrioritySessionIds(new Set());
      setDraftNeedsReview(Boolean(shouldRestoreDraft && draftForStudent && draftForStudent.selectedSubjectIds.length > 0));
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
    const wasSelected = selectedSubjectIds.includes(subjectId);
    if (wasSelected) {
      setSelectedSubjectIds((current) => current.filter((id) => id !== subjectId));
      setExpandedSubjectId((expanded) => {
        if (expanded !== subjectId) return expanded;
        return selectedSubjectIds.filter((id) => id !== subjectId)[0] ?? null;
      });
      return;
    }
    setSelectedSubjectIds((current) => [...current, subjectId]);
    setExpandedSubjectId(subjectId);
  };

  const handleSessionGroupToggle = (group: SubjectSessions, sessionIds: string[]) => {
    if (sessionIds.every((sessionId) => selectedSessionIds.has(sessionId))) {
      setSelectedSessionIds((current) => {
        const next = new Set(current);
        for (const sessionId of sessionIds) next.delete(sessionId);
        return next;
      });
      setSitInSelections((current) => {
        const next = { ...current };
        for (const sessionId of sessionIds) delete next[sessionId];
        return next;
      });
      return;
    }
    const next = new Set(selectedSessionIds);
    for (const sessionId of sessionIds) next.add(sessionId);
    const scopeKey = absenceScopeKey(group);
    const scopedGroups = scopeIndex.get(scopeKey) ?? [];
    const projectedDays = countSelectedAbsenceDaysForScope(scopedGroups, next, scopeKey);
    const remaining = Math.max(0, ...scopedGroups.map(remainingForGroup));
    if (projectedDays > remaining || projectedDays > maxSessions) {
      const label = group.merge_group_name?.trim() || group.course_name?.trim() || "this course";
      setPageError(`You can report ${remaining > 0 ? `only ${Math.min(remaining, maxSessions)} more absence day${Math.min(remaining, maxSessions) !== 1 ? "s" : ""} for ${label}` : `no more absences for ${label}`}. Remove a selected day first if you need to change your selection.`);
      return;
    }
    setPageError(null);
    setSelectedSessionIds(next);
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
        const data = await loadStudentSessions(
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

  // A step change swaps the whole working area; without this, focus can fall
  // back to <body> when the triggering control unmounts (e.g. "Edit classes").
  // Focus already placed inside the new step (an alert, a restored control)
  // keeps priority.
  const hasFocusedInitiallyRef = useRef(false);
  useEffect(() => {
    if (!hasFocusedInitiallyRef.current) {
      hasFocusedInitiallyRef.current = true;
      return;
    }
    window.requestAnimationFrame(() => {
      const main = document.getElementById("absence-form-content");
      if (!main) return;
      const active = document.activeElement;
      if (active instanceof HTMLElement && active !== document.body && main.contains(active)) return;
      main.focus();
    });
  }, [step]);

  useEffect(() => {
    if (finalResults) resultHeadingRef.current?.focus();
  }, [finalResults]);

  function focusFirstInvalid(selector: string) {
    window.requestAnimationFrame(() => {
      const candidates = [...document.querySelectorAll<HTMLElement>(selector)];
      const target = candidates.find((candidate) => {
        const styles = window.getComputedStyle(candidate);
        return styles.display !== "none" && styles.visibility !== "hidden" && candidate.getClientRects().length > 0;
      }) ?? candidates[0];
      target?.focus();
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
      focusFirstInvalid('[data-make-up-trigger], select[aria-label*="make-up" i], select');
      return false;
    }
    if (config.form.require_reason && !reason.trim()) {
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
    if (!online) {
      setSubmissionError("You're offline. Reconnect before submitting your absence.");
      return;
    }
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
      const nicknameForSubmission = studentProfile?.nickname_set === false && nickname.trim()
        ? nickname.trim()
        : undefined;
      let response: Awaited<ReturnType<typeof submitAbsenceBatch>>;
      try {
        response = await submitAbsenceBatch({
          idempotencyKey: submissionIdempotencyKey.current,
          email: collectedEmail.trim() || undefined,
          nickname: nicknameForSubmission,
          reason: reason.trim(),
          items: payloads,
        });
      } catch (error) {
        if (error instanceof ApiRequestError && error.code === "bad_nickname") {
          // Optional enrichment must never block the absence itself: a
          // nickname landed on file after the profile was loaded, so drop
          // it and submit once more under a fresh idempotency key.
          submissionIdempotencyKey.current = newIdempotencyKey();
          response = await submitAbsenceBatch({
            idempotencyKey: submissionIdempotencyKey.current,
            email: collectedEmail.trim() || undefined,
            reason: reason.trim(),
            items: payloads,
          });
        } else {
          throw error;
        }
      }
      setFinalResults(response.items);
      verification.setCode("");
      try {
        clearLegacyAbsenceDraft();
        clearStudentResume();
        clearDraft();
      } catch { }
    } catch (error) {
      if (isStudentSessionUnauthorized(error)) {
        handleStudentSessionExpired();
        return;
      }
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

  
  const classDataAvailable = online || sessions.length > 0;

  const actionCanProceed =
    step === 0 ? canProceedFromStudent :
    step === 1 ? verificationSatisfied :
    step === 2 ? classDataAvailable && !sessionsLoading && !draftNeedsReview :
    step === 3 ? verificationSatisfied && !verificationBlocked && online : false;

  const primaryActionLabel =
    step === 0 ? "Continue to verification" :
    step === 1 ? "Continue to classes" :
    step === 2 ? "Review absence" :
    "Submit absence";

  const handlePrimaryAction = () => {
    if (step === 0) goToStep(1);
    else if (step === 1) goToStep(2);
    else if (step === 2) {
      if (!validateClasses()) return;
      goToStep(3);
    } else {
      void handleSubmitAbsence();
    }
  };

  if (configLoading) {
    return (
      <AbsenceAppShell
        header={<AbsenceAppHeader steps={STEP_LABELS} currentStep={0} />}
        footer={
          <AbsenceActionBar
            currentStep={0}
            canProceed={false}
            onBack={() => {}}
            onPrimary={() => {}}
            primaryLabel="Continue to verification"
          />
        }
      >
        <div className="mx-auto w-full max-w-3xl px-0 py-6">
          <LoadingSkeleton type="text" lines={3} />
        </div>
      </AbsenceAppShell>
    );
  }

  return (
    <AbsenceAppShell
      header={
        <AbsenceAppHeader
          steps={STEP_LABELS}
          currentStep={step}
          onStepClick={(next) => { if (next < step) goToStep(next as StepIndex); }}
        />
      }
      footer={
        <AbsenceActionBar
          currentStep={step}
          canProceed={actionCanProceed}
          loading={isSubmitting}
          onBack={() => goToStep(Math.max(0, step - 1) as StepIndex)}
          onPrimary={handlePrimaryAction}
          primaryLabel={primaryActionLabel}
        />
      }
    >
      <div className="mx-auto w-full max-w-3xl py-6">
        {pageError || submissionError ? <FormAlert alertRef={pageAlertRef} message={submissionError || pageError || ""} /> : null}
        {!online ? (
          <div role="status" aria-live="polite" className="mb-4 rounded-xl border border-[var(--color-wi-amber)]/30 bg-[var(--color-wi-amber-bg)] px-4 py-3 text-sm text-[var(--color-wi-amber)]">
            You're offline. Network actions are paused; your current progress remains on this device.
          </div>
        ) : justRestored ? (
          <div role="status" aria-live="polite" className="mb-4 rounded-xl border border-[var(--color-wi-green)]/30 bg-[var(--color-wi-green)]/10 px-4 py-3 text-sm font-medium text-[var(--color-wi-green)]">
            Back online. Rechecking your classes...
          </div>
        ) : null}

        <p aria-live="polite" className="sr-only">
          Step {step + 1} of {STEP_LABELS.length}: {STEP_LABELS[step].label} — {STEP_LABELS[step].description}
        </p>

        <motion.div
          key={step}
          initial={reduceMotion ? false : { opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={reduceMotion ? { duration: 0 } : { duration: 0.18, ease: "easeOut" }}
          className="space-y-6"
        >
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
                        aria-busy={lookupLoading}
                        className="inline-flex min-h-[48px] min-w-[6.75rem] items-center justify-center rounded-lg bg-[var(--color-wi-primary)] px-5 text-sm font-semibold text-white transition-colors motion-reduce:transition-none hover:bg-[var(--color-wi-primary-dark)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2 disabled:opacity-50"
                      >
                        {lookupLoading ? (
                          <span className="inline-flex items-center gap-2">
                            <LoaderCircle className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
                            <span>Searching…</span>
                          </span>
                        ) : "Search"}
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
                            <p className="text-sm font-semibold text-[var(--color-wi-text)]">Student ID found</p>
                            <p className="text-xs font-mono text-[var(--color-wi-text-light)] mt-0.5">{lookup.wcode}</p>
                            {lookup.nickname_hint ? (
                              <p className="text-xs text-[var(--color-wi-text-light)] mt-1" data-testid="lookup-nickname-hint">
                                Nickname: {lookup.nickname_hint}
                              </p>
                            ) : null}
                          </div>
                          <span className="rounded-full bg-green-100 px-2 py-0.5 text-[10px] font-medium text-green-700">
                            Ready to verify
                          </span>
                        </div>
                        <p className="mt-3 text-xs text-[var(--color-wi-text-light)]">
                          Parent verification is available. Your student details will appear after verification.
                        </p>
                        {lookup.email_input_required ? (
                          <div className="mt-4 space-y-1.5">
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
                              aria-invalid={collectedEmail.trim() && !manualEmailValid ? true : undefined}
                              aria-describedby={collectedEmail.trim() && !manualEmailValid ? "student-email-error" : undefined}
                            />
                            {collectedEmail.trim() && !manualEmailValid && (
                              <p id="student-email-error" className="text-xs text-[var(--color-wi-amber)]">Enter a valid email to continue.</p>
                            )}
                          </div>
                        ) : (
                          <p className="mt-4 flex items-center gap-1.5 text-xs font-medium text-[var(--color-wi-text)]">
                            <svg className="h-3.5 w-3.5 text-[var(--color-wi-green)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5} aria-hidden="true">
                              <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                            </svg>
                            Your email is already saved — no need to enter it
                          </p>
                        )}
                      </div>
                    </div>
                  ) : null}
                </div>
              </StudentStep>
            )}

            {step === 1 && (
              lookup ? (
                <VerificationStep
                  studentName={studentDisplayName}
                  wcode={lookup.wcode}
                  hasPhone={lookup.parent_verification_available}
                  phoneLabel={lookup.parent_verification_available ? "Verification phone available" : "Verification phone unavailable"}
                >
                  <StepCoverVerification
                    lookupToken={lookup.lookup_token}
                    wcode={lookup.wcode}
                    online={online}
                    parentVerificationAvailable={lookup.parent_verification_available}
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
                  <div className="absence-classes-layout">
                    <div className="absence-classes-layout__subjects">
                      <section>
                      <h2 className="text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wide mb-3">Which classes?</h2>
                      {studentProfile && verifiedSubjects.length > 0 ? (
                        <div className="rounded-lg border border-[var(--color-wi-border)] bg-white divide-y divide-[var(--color-wi-border)] overflow-hidden">
                          {verifiedSubjects.map((subject) => (
                          <SubjectRow
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
                    </div>
                    <div className="absence-classes-layout__work space-y-6">
                    {draftNeedsReview ? (
                      <div role="status" aria-live="polite" className="rounded-xl border border-[var(--color-wi-amber)]/30 bg-[var(--color-wi-amber-bg)] px-4 py-3 text-sm text-[var(--color-wi-amber)]">
                        <p className="font-semibold">Your available classes changed.</p>
                        <p className="mt-1">Review the current classes before continuing.</p>
                        <button
                          type="button"
                          onClick={() => setDraftNeedsReview(false)}
                          className="mt-3 min-h-11 rounded-lg border border-[var(--color-wi-amber)] px-3 text-sm font-semibold hover:bg-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-amber)]"
                        >
                          Review updated classes
                        </button>
                      </div>
                    ) : null}

                    {selectedSubjectIds.length > 0 ? (
                      <section>
                        <div className="flex items-center justify-between mb-3">
                          <h2 className="text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wide">Classes to miss</h2>
                          <span aria-live="polite" className="text-xs font-semibold text-[var(--color-wi-text-light)]">
                            {selectedAbsenceDayCount} selected
                            {selectedSubjectRemainingDays > 0
                              ? ` (${selectedSubjectRemainingDays} remaining)`
                              : ""}
                          </span>
                        </div>
                        <div className="mb-4 space-y-2 sm:hidden" aria-label="Selected subjects">
                          {selectedSubjectIds.map((subjectId) => {
                            const subject = verifiedSubjects.find((item) => item.id === subjectId);
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
                          <div role="alert" className="space-y-2">
                            <p className="text-sm text-[var(--color-wi-red)]">{sessionsError}</p>
                            <button
                              type="button"
                              onClick={() => setSessionsReloadToken((token) => token + 1)}
                              className="min-h-11 rounded-lg border border-[var(--color-wi-red)]/40 px-3 text-sm font-semibold text-[var(--color-wi-red)] transition-colors motion-reduce:transition-none hover:bg-[var(--color-wi-danger-bg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-red)]/40"
                            >
                              Retry loading classes
                            </button>
                          </div>
                        ) : selectedGroups.length === 0 ? (
                          <p className="text-sm text-[var(--color-wi-text-light)]">No classes found for the selected courses.</p>
                        ) : (
                          <div className="space-y-4">
                            {selectedGroups.map((group) => {
                              const sessionGroups = groupDayIndex.get(group.course_id) ?? [];
                              const sourceLabel = group.subject_name?.trim() || group.course_name?.trim();
                              const groupLabel = group.merge_group_name?.trim()
                                ? `${group.merge_group_name.trim()} · ${sourceLabel}`
                                : sourceLabel;
                              const scopeKey = absenceScopeKey(group);
                              const scopedGroups = scopeIndex.get(scopeKey) ?? [];
                              const groupRemaining = Math.max(...scopedGroups.map(remainingForGroup), 0);
                              const selectedDaysInGroup = selectedDaysByScope.get(scopeKey) ?? 0;
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
                                  {group.merge_group_name ? (
                                    <div className="border-b border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] px-4 pb-3 text-xs text-[var(--color-wi-text-light)]">
                                      Shared absence quota across this merged course
                                    </div>
                                  ) : null}
                                  {group.absence_limit_reached ? (
                                    <div className="p-4">
                                      <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2.5 text-sm text-amber-800">
                                        <svg className="mt-0.5 h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                          <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
                                        </svg>
                                        <span>
                                          You have reached the maximum absences allowed for {group.merge_group_name ? "this merged course" : "this course"}.
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
                                        <SessionDayCard
                                          key={dayGroup.id}
                                          dayGroup={dayGroup}
                                          selected={selected}
                                          alreadyAbsent={alreadyAbsent}
                                          disabled={!selected && (effectiveRemaining === 0 || selectedDaysInGroup >= maxSessions)}
                                          onToggle={() => handleSessionGroupToggle(group, sessionIds)}
                                          reduceMotion={reduceMotion}
                                        >
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
                                                          <MakeUpPicker
                                                            id={`sit-in-${session.id}`}
                                                            label="Make-up class"
                                                            value={currentSitIn}
                                                            options={currentPriorities.flatMap((priority) =>
                                                              groupByDay(availableSessionsForMissedSessions(priority, sessionIds)).map((optionGroup) => ({
                                                                value: mergedSessionValue(optionGroup.items),
                                                                label: getSitInSessionGroupLabel(optionGroup.items, priority.sit_in_course, groupLabel, sessions),
                                                              })),
                                                            )}
                                                            onChange={(value) => handleSitInSelectForSessions(sessionIds, value)}
                                                          />
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
                                                      <MakeUpPicker
                                                        id={`sit-in-${session.id}`}
                                                        label="Make-up class"
                                                        value={currentSitIn}
                                                        options={groupByDay(sitInAvailable).map((optionGroup) => ({
                                                          value: mergedSessionValue(optionGroup.items),
                                                          label: getSitInSessionGroupLabel(optionGroup.items, sitIn?.sit_in_course, groupLabel, sessions),
                                                        }))}
                                                        onChange={(value) => handleSitInSelectForSessions(sessionIds, value)}
                                                      />
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
                                        </SessionDayCard>
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

                    <ReasonField
                      value={reason}
                      onChange={(value) => {
                        setReason(value);
                        setReasonError(null);
                      }}
                      error={reasonError}
                      required={config.form.require_reason}
                    />
                    </div>
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
                      <span className="font-medium text-[var(--color-wi-text)]">{studentDisplayName}</span> — {lookup.wcode}
                    </p>

                    {studentProfile?.nickname_set === false ? (
                      <div className="rounded-lg border border-[var(--color-wi-border)] bg-white px-5 py-4 space-y-2">
                        <label htmlFor="student-nickname" className="block text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">
                          Nickname <span className="normal-case tracking-normal font-medium">(optional)</span>
                        </label>
                        <input
                          id="student-nickname"
                          maxLength={50}
                          className="min-h-[48px] w-full rounded-xl border border-[var(--color-wi-border)] bg-white px-4 text-base text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                          placeholder="What our staff can call you"
                          value={nickname}
                          onChange={(e) => setNickname(e.target.value)}
                        />
                        <p className="text-xs text-[var(--color-wi-text-light)]" data-testid="nickname-echo">
                          {nickname.trim()
                            ? <>Helps our staff recognise you — it will be saved as {maskNickname(nickname)}.</>
                            : <>No nickname saved yet. Adding one helps our staff recognise you.</>}
                        </p>
                      </div>
                    ) : null}

                    {/* Classes section */}
                    <div className="rounded-lg border border-[var(--color-wi-border)] bg-white">
                      <div className="flex items-center justify-between border-b border-[var(--color-wi-border)] px-5 py-3">
                        <h2 className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Classes</h2>
                        <button
                          type="button"
                          onClick={() => goToStep(2)}
                          className="min-h-11 rounded-lg px-2 text-xs font-semibold text-[var(--color-wi-primary)] transition-colors motion-reduce:transition-none hover:bg-[var(--color-wi-primary)]/5 hover:text-[var(--color-wi-primary-dark)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                        >
                          Edit classes
                        </button>
                      </div>
                      <div className="px-5 py-4 space-y-3">
                        {selectedGroups.map((group) => {
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
                          className="min-h-11 rounded-lg px-2 text-xs font-semibold text-[var(--color-wi-primary)] transition-colors motion-reduce:transition-none hover:bg-[var(--color-wi-primary)]/5 hover:text-[var(--color-wi-primary-dark)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                        >
                          Edit reason
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
        </motion.div>
      </div>
      {submissionOverlay}
    </AbsenceAppShell>
  );
}
