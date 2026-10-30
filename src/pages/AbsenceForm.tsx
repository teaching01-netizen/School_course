import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { motion, useReducedMotion } from "framer-motion";
import { ChevronDown, ChevronLeft, LoaderCircle } from "lucide-react";
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
import { currentSitInOwners, makeUpPickerOptions, publicMakeUpChoicesBySession, type PublicMakeUpChoices } from "@/components/absences/public-form/makeUpChoices";
import FormAlert from "@/components/absences/public-form/FormAlert";
import { useToast } from "@/hooks/useToast";
import { useAbsenceDraft } from "@/features/absences/hooks/useAbsenceDraft";
import type { AbsenceDraftV1 } from "@/features/absences/storage/absenceDraftStorage";
import { useOtp } from "@/hooks/useOtp";
import { formatDate, formatTime } from "@/utils/date";
import type {
  AbsenceFormConfig,
  ManagedAbsence,
  PublicStudentLookupResponse,
  StudentLookupResponse,
  SubjectSessions,
  VerifiedStudentProfile,
} from "@/types";
import {
  DEFAULT_CONFIG,
  STAFF_ABSENCE_DRAFT_STORAGE_KEY,
  VERIFICATION_STORAGE_KEY,
} from "@/features/absences/constants";
import {
  loadAbsenceFormConfig,
  loadSessionsInRange,
  loadStudentProfile,
  loadStudentSessions,
  lookupStaffStudentByWcode,
  lookupStudentByWcode,
  submitAbsenceBatch,
  submitStaffAbsenceFormBatch,
  type StaffSessionsInRangeOptions,
} from "@/features/absences/api/absenceFormApi";
import {
  absenceScopeKey,
  combineSubjectGroups,
  combineSubjectPickerEntries,
  countSelectedAbsenceDays,
  countSelectedAbsenceDaysForScope,
  countSelectedAbsenceDaysForGroup,
  groupByDay,
  splitMergedSessionValue,
  uniqueValues,
  type SubjectPickerEntry,
} from "@/features/absences/domain/sessionGrouping";
import { buildSubmissionPayloads as buildAbsenceSubmissionPayloads, duplicateSitInSessionIds } from "@/features/absences/domain/submissionPayload";
import {
  availableSessionsForMissedSession,
  availableSessionsForMissedSessions,
  firstPriorityLevel,
  getReviewSitInLabel,
  appendTeacher,
  normalizeSitInDisplayModel,
  formatSitInDisplayDetails,
  formatSitInSubmissionConflictDetails,
  blockedSitInSessionIds,
  sitInOptionGroupsBySession,
  sitInOptionsByTargetAndSession,
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
  formatSubmittedAbsenceSummary,
  formatSubmittedSitInSummary,
  groupSubmittedAbsences,
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
type AbsenceFormMode = "public" | "staff";
type AbsenceLookupResponse = PublicStudentLookupResponse | StudentLookupResponse;

function isStaffLookupResponse(value: AbsenceLookupResponse): value is StudentLookupResponse {
  return "full_name" in value;
}

function profileFromStaffLookup(student: StudentLookupResponse): VerifiedStudentProfile {
  return {
    wcode: student.wcode,
    display_name: student.display_name?.trim() || student.full_name,
    email_on_file: Boolean(student.email?.trim() || student.email_crm?.trim() || student.email_system?.trim()),
    nickname_set: Boolean(student.nickname?.trim()),
    subjects: student.subjects.map(({ id, code, name, teacher_name, merge_group_id, merge_group_name }) => ({
      id,
      code,
      name,
      teacher_name,
      merge_group_id,
      merge_group_name,
    })),
  };
}

function isStudentSessionUnauthorized(error: unknown): boolean {
  return error instanceof ApiRequestError
    && error.status === 401
    && error.code === "unauthorized";
}

// Mirror of the make-up picker's option source: the sit-ins the server still
// offers for a missed session at the level the form will display. Restoring a
// draft keeps a saved make-up only while its saved sit-ins are still offered,
// so a resumed form never shows a selection the picker cannot render.
function offeredSitInSessionIds(
  group: SubjectSessions,
  missedSessionId: string,
  savedPriorityLevel?: number,
): Set<string> {
  const sessionGroup = groupWithSitInForMissedSession(group, missedSessionId);
  const sitIn = sessionGroup.sit_in;
  const offered = new Set<string>();
  if (!sitIn || sitIn.sit_in_method !== "physical") return offered;
  const baseLevel = sitIn.current_priority_level ?? firstPriorityLevel(sessionGroup);
  const level = savedPriorityLevel !== undefined && hasPriorityLevel(sessionGroup, savedPriorityLevel)
    ? savedPriorityLevel
    : baseLevel;
  const priorities = prioritiesForLevel(sessionGroup, level);
  if (priorities.length > 0) {
    for (const priority of priorities) {
      for (const session of availableSessionsForMissedSession(priority, missedSessionId)) {
        offered.add(session.id);
      }
    }
    return offered;
  }
  for (const session of rootAvailableSessionsForMissedSessions(sitIn, [missedSessionId])) {
    offered.add(session.id);
  }
  return offered;
}

export default function AbsenceForm({ mode = "public" }: { mode?: AbsenceFormMode }) {
  const isStaff = mode === "staff";
  const { addToast } = useToast();
  const verification = useOtp(VERIFICATION_STORAGE_KEY, !isStaff);
  const reduceMotion = useReducedMotion();
  const { draft: savedDraft, saveDraft, clearDraft } = useAbsenceDraft(
    isStaff ? STAFF_ABSENCE_DRAFT_STORAGE_KEY : undefined,
  );
  const draftRef = useRef<AbsenceDraftV1 | null>(savedDraft);
  const submissionIdempotencyKey = useRef(newIdempotencyKey());
  const lookupRequestId = useRef(0);

  const STEP_LABELS = isStaff
    ? [
      { label: "Student", description: "Select a student" },
      { label: "Classes", description: "Select classes & make-up" },
      { label: "Review", description: "Confirm and submit" },
    ]
    : [
      { label: "Student", description: "Choose the student" },
      { label: "Verify", description: "Parent confirmation" },
      { label: "Classes", description: "Select classes & make-up" },
      { label: "Review", description: "Confirm and submit" },
    ];

  const [step, setStep] = useState<StepIndex>(0);
  // Forward steps enter from the trailing edge, Back from the leading edge.
  const [stepMotion, setStepMotion] = useState({ step: 0, direction: 1 });
  if (stepMotion.step !== step) setStepMotion({ step, direction: step > stepMotion.step ? 1 : -1 });
  const classesStep: StepIndex = isStaff ? 1 : 2;
  const reviewStep: StepIndex = isStaff ? 2 : 3;
  const [config, setConfig] = useState<AbsenceFormConfig>(DEFAULT_CONFIG);
  const [configLoading, setConfigLoading] = useState(true);
  const [lookupInput, setLookupInput] = useState("");
  const [lookup, setLookup] = useState<AbsenceLookupResponse | null>(null);
  const [studentProfile, setStudentProfile] = useState<VerifiedStudentProfile | null>(null);
  const [lookupLoading, setLookupLoading] = useState(false);
  const [lookupError, setLookupError] = useState<string | null>(null);
  const studentIdInputRef = useRef<HTMLInputElement | null>(null);
  const selectedStudentHeadingRef = useRef<HTMLHeadingElement | null>(null);
  const [collectedEmail, setCollectedEmail] = useState("");
  const [emailTouched, setEmailTouched] = useState(false);
  const [selectedSubjectIds, setSelectedSubjectIds] = useState<string[]>([]);
  const [expandedSubjectId, setExpandedSubjectId] = useState<string | null>(null);
  const [reason, setReason] = useState("");
  // Nickname lives in memory only — it is never written to the saved draft,
  // mirroring the privacy rule for the enrolled parent phone.
  const [nickname, setNickname] = useState("");
  const [reasonError, setReasonError] = useState<string | null>(null);
  const [showClassValidation, setShowClassValidation] = useState(false);
  const [sessions, setSessions] = useState<SubjectSessions[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [sessionsError, setSessionsError] = useState<string | null>(null);
  const [sessionsReloadToken, setSessionsReloadToken] = useState(0);
  const [selectedSessionIds, setSelectedSessionIds] = useState<Set<string>>(new Set());
  const [sitInSelections, setSitInSelections] = useState<Record<string, string>>({});
  const [sitInPriorityLevels, setSitInPriorityLevels] = useState<Record<string, number>>({});
  const [sitInPriorityHistory, setSitInPriorityHistory] = useState<Record<string, Record<number, SubjectSessions>>>({});
  const [revealingPrioritySessionIds, setRevealingPrioritySessionIds] = useState<Set<string>>(new Set());
  const [sitInPriorityErrors, setSitInPriorityErrors] = useState<Record<string, string>>({});
  const [rejectedMakeUpSessionIds, setRejectedMakeUpSessionIds] = useState<Set<string>>(new Set());
  const revealRequestsRef = useRef(new Set<string>());
  const [pageError, setPageError] = useState<string | null>(null);
  const [verificationSatisfied, setVerificationSatisfied] = useState(false);
  const [verificationBlocked, setVerificationBlocked] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submissionError, setSubmissionError] = useState<string | null>(null);
  const [finalResults, setFinalResults] = useState<ManagedAbsence[] | null>(null);
  const resultHeadingRef = useRef<HTMLHeadingElement | null>(null);
  const pageAlertRef = useRef<HTMLDivElement | null>(null);
  const navigationFocusRef = useRef<string | null>(null);

  const selectedSubjectIdSet = useMemo(() => new Set(selectedSubjectIds), [selectedSubjectIds]);
  const selectedAbsenceDayCount = useMemo(
    () => countSelectedAbsenceDays(sessions.filter((group) => selectedSubjectIdSet.has(group.subject_id)), selectedSessionIds),
    [sessions, selectedSessionIds, selectedSubjectIdSet],
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
  const selectedGroups = useMemo(
    () => sessions.filter((group) => selectedSubjectIdSet.has(group.subject_id)),
    [sessions, selectedSubjectIdSet],
  );
  // One renderable block per absence scope: a merged course collapses its
  // source courses into a single block, keeping the source groups for
  // per-session sit-in resolution (sessions belong to real courses).
  const selectedBlocks = useMemo(() => combineSubjectGroups(selectedGroups), [selectedGroups]);
  const ownerGroupBySessionId = useMemo(() => {
    const owners = new Map<string, SubjectSessions>();
    for (const group of selectedGroups) {
      for (const session of group.sessions) owners.set(session.id, group);
    }
    return owners;
  }, [selectedGroups]);
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
  const showEmailError = emailTouched && Boolean(manualEmail) && !manualEmailValid;
  // A result applies only to the Student ID it was searched for; editing the
  // field hides it until the new Student ID has been looked up.
  const currentLookup = lookup && lookupInput.trim().toLowerCase() === lookup.wcode.trim().toLowerCase() ? lookup : null;
  const lookupStale = lookup !== null && currentLookup === null;
  const publicLookup = currentLookup && !isStaffLookupResponse(currentLookup) ? currentLookup : null;
  const staffLookup = currentLookup && isStaffLookupResponse(currentLookup) ? currentLookup : null;
  const emailSatisfied = !!publicLookup && (!publicLookup.email_input_required || manualEmailValid);
  const canProceedFromStudent = isStaff ? Boolean(staffLookup) : Boolean(publicLookup && emailSatisfied);
  // Pre-verification the public lookup provides only the stored nickname,
  // never a full-name fallback. The verified profile is authoritative afterward.
  const studentDisplayName = studentProfile?.display_name
    || staffLookup?.display_name
    || staffLookup?.full_name
    || publicLookup?.nickname?.trim()
    || "Student";
  const verifiedSubjects = studentProfile?.subjects ?? [];
  // One picker entry per absence scope: a merged course appears as one entry
  // even though it spans two subjects, and selecting it selects both.
  const pickerEntries = useMemo(() => combineSubjectPickerEntries(verifiedSubjects), [verifiedSubjects]);

  const publicChoicesBySession = useMemo(() => isStaff ? new Map<string, PublicMakeUpChoices>() : publicMakeUpChoicesBySession({
    sessions, selectedSubjectIds, selectedSessionIds, selections: sitInSelections,
    priorityLevels: sitInPriorityLevels, priorityHistory: sitInPriorityHistory,
    blockedSessionIds: rejectedMakeUpSessionIds,
  }), [isStaff, selectedSessionIds, sitInPriorityHistory, sitInPriorityLevels, sessions, selectedSubjectIds, sitInSelections, rejectedMakeUpSessionIds]);
  const invalidMakeUpIds = useMemo(() => new Set([...publicChoicesBySession].filter(([id, choices]) => {
    const selected = sitInSelections[id];
    return selected && !choices.options.some((option) => option.value === selected && !option.disabled);
  }).map(([id]) => id)), [publicChoicesBySession, sitInSelections]);

  const missingSitIn = useMemo(() => {
    for (const group of sessions) {
      if (!selectedSubjectIdSet.has(group.subject_id)) continue;
      for (const session of group.sessions) {
        if (!selectedSessionIds.has(session.id) || session.already_absent) continue;
        const sitIn = sitInForMissedSession(group, session.id);
        if (sitIn?.sit_in_method === "physical" && (!sitInSelections[session.id] || invalidMakeUpIds.has(session.id))) return true;
      }
    }
    return false;
  }, [sessions, selectedSubjectIdSet, selectedSessionIds, sitInSelections, invalidMakeUpIds]);

  useEffect(() => {
    if (isStaff || step !== 0 || lookupLoading || !publicLookup) return;
    selectedStudentHeadingRef.current?.focus({ preventScroll: true });
  }, [isStaff, lookupLoading, publicLookup, step]);

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

  const loadFormSessions = useCallback(
    (
      wcode: string,
      init?: Pick<RequestInit, "signal">,
      options?: Pick<StaffSessionsInRangeOptions, "courseIds" | "subjectIds" | "satVerbalAfterPriority">,
    ) => isStaff
      ? loadSessionsInRange(wcode, undefined, undefined, init, { ...options, studentView: true })
      : loadStudentSessions(undefined, undefined, init, options),
    [isStaff],
  );

  const handleStudentSessionExpired = useCallback(() => {
    if (!isStaff) {
      clearStudentSessionHint();
      verification.clearStoredToken();
      verification.setCode("");
    }
    setStudentProfile(null);
    setSessions([]);
    setVerificationSatisfied(false);
    setVerificationBlocked(!isStaff);
    setPageError(isStaff ? "Your staff session expired. Sign in again to continue." : "Your verified session expired. Verify again to continue.");
    setSubmissionError(null);
    setStep(isStaff ? 0 : 1);
  }, [isStaff, verification.clearStoredToken, verification.setCode]);

  useEffect(() => {
    if (step !== classesStep || !lookup) return;
    const controller = new AbortController();
    setSessionsLoading(true);
    setSessionsError(null);
    const profileRequest = isStaff && isStaffLookupResponse(lookup)
      ? Promise.resolve(profileFromStaffLookup(lookup))
      : loadStudentProfile();
    void Promise.all([
      profileRequest,
      loadFormSessions(lookup.wcode, { signal: controller.signal }),
    ])
      .then(([profile, data]) => {
        if (controller.signal.aborted) return;
        if (profile.wcode !== lookup.wcode) {
          setStudentProfile(null);
          setSessions([]);
          setSessionsError(isStaff ? "The selected student changed. Search again before continuing." : "Verification belongs to a different Student ID. Verify this Student ID again.");
          setStep(isStaff ? 0 : 1);
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

        // The fresh response is the source of truth: saved selections that the
        // server still offers are restored as-is, and anything that expired or
        // was removed is dropped here instead of behind a review gate.
        const groupBySessionId = new Map<string, SubjectSessions>();
        const validSessionIds = new Set<string>();
        for (const group of data.subjects) {
          for (const session of group.sessions) {
            validSessionIds.add(session.id);
            groupBySessionId.set(session.id, group);
          }
        }
        const restoredSessionIds = draft.selectedSessionIds.filter((sessionId) => validSessionIds.has(sessionId));
        const restoredSessionSet = new Set(restoredSessionIds);
        const restoredPriorityLevels: Record<string, number> = {};
        for (const [sessionId, priority] of Object.entries(draft.sitInPriorityLevels)) {
          if (restoredSessionSet.has(sessionId)) restoredPriorityLevels[sessionId] = priority;
        }
        const blockedSitInIds = blockedSitInSessionIds(data.subjects);
        const restoredSitIns: Record<string, string> = {};
        for (const [sessionId, sitInValue] of Object.entries(draft.sitInSelections)) {
          const ownerGroup = restoredSessionSet.has(sessionId) ? groupBySessionId.get(sessionId) : undefined;
          const savedSitInIds = ownerGroup && sitInValue ? splitMergedSessionValue(sitInValue) : [];
          if (!ownerGroup || savedSitInIds.length === 0) continue;
          const offered = offeredSitInSessionIds(ownerGroup, sessionId, restoredPriorityLevels[sessionId]);
          if (savedSitInIds.every((sitInId) => offered.has(sitInId) && !blockedSitInIds.has(sitInId))) {
            restoredSitIns[sessionId] = sitInValue;
          }
        }
        setSelectedSessionIds(restoredSessionSet);
        setSitInSelections(restoredSitIns);
        setSitInPriorityLevels(restoredPriorityLevels);
        setExpandedSubjectId(restoredSubjectIds[0] ?? null);
        draftRef.current = null;
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        if (!isStaff && isStudentSessionUnauthorized(error)) {
          handleStudentSessionExpired();
          return;
        }
        setStudentProfile(null);
        setSessions([]);
        setSessionsError(error instanceof Error ? error.message : "Couldn't load your classes");
    })
    .finally(() => { if (!controller.signal.aborted) setSessionsLoading(false); });
    return () => controller.abort();
  }, [classesStep, handleStudentSessionExpired, isStaff, loadFormSessions, lookup, sessionsReloadToken, step]);

  useEffect(() => {
    if (isStaff) return;
    let active = true;
    try {
      clearLegacyAbsenceDraft();
      const resume = readStudentResume();
      const draft = draftRef.current;
      const resumeWcode = draft?.wcode ?? resume?.wcode;
      if (!resumeWcode) return () => { active = false; };
      const requestId = ++lookupRequestId.current;
      const restoredEmail = draft?.collectedEmail ?? resume?.collectedEmail;
      setLookupInput(resumeWcode);
      if (restoredEmail) setCollectedEmail(restoredEmail);
      if (draft?.reason) setReason(draft.reason);
      setLookupLoading(true);
      void lookupStudentByWcode(resumeWcode)
        .then((response) => {
          if (!active || requestId !== lookupRequestId.current) return;
          setLookup(response);
          setStudentProfile(null);
          const restoreDraft = draftRef.current;
          const isSameStudent = Boolean(restoreDraft && normalizeLookupWcode(restoreDraft.wcode) === response.wcode);
          setSelectedSubjectIds([]);
          if (isSameStudent && restoreDraft) setReason(restoreDraft.reason);
        })
        .catch((error: unknown) => {
          if (active && requestId === lookupRequestId.current) setLookupError(error instanceof Error ? error.message : "We couldn't refresh your profile");
        })
        .finally(() => { if (active && requestId === lookupRequestId.current) setLookupLoading(false); });
    } catch { }
    return () => { active = false; };
  }, [isStaff]);

  useEffect(() => {
    if (isStaff || !publicLookup) return;
    try { writeStudentResume({ wcode: publicLookup.wcode, collectedEmail }); } catch { }
  }, [collectedEmail, isStaff, publicLookup]);

  useEffect(() => {
    if (!lookup || finalResults) return;
    saveDraft({
      wcode: lookup.wcode,
      collectedEmail: isStaff ? undefined : collectedEmail || undefined,
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
    isStaff,
    step,
    selectedSubjectIds,
    selectedSessionIds,
    sitInSelections,
    sitInPriorityLevels,
    reason,
    saveDraft,
  ]);

  useEffect(() => {
    if (isStaff) return;
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
  }, [isStaff, verification]);

  useEffect(() => {
    if (isStaff) return;
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
  }, [isStaff, verification.expiresAt, verification.token]);

  const handleVerificationSatisfied = useCallback(() => {
    setVerificationSatisfied(true);
    setStep(classesStep);
  }, [classesStep]);

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
    setShowClassValidation(false);
    if (!isStaff) clearStudentSessionHint();
    setLookup(null);
    setStudentProfile(null);
    if (!cleaned || !isWCode(cleaned)) {
      setLookupLoading(false);
      setLookupError("Enter a Student ID (W-Code).");
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
      const response = isStaff
        ? await lookupStaffStudentByWcode(cleaned, { studentView: true })
        : await lookupStudentByWcode(cleaned);
      if (requestId !== lookupRequestId.current) return;
      setLookup(response);
      setStudentProfile(isStaff && isStaffLookupResponse(response) ? profileFromStaffLookup(response) : null);
      setLookupInput(cleaned);
      setSelectedSubjectIds([]);
      setExpandedSubjectId(null);
      setCollectedEmail(shouldRestoreDraft ? draftForStudent?.collectedEmail ?? "" : "");
      setEmailTouched(false);
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
      setSitInPriorityErrors({});
      setRejectedMakeUpSessionIds(new Set());
      setSubmissionError(null);
      submissionIdempotencyKey.current = newIdempotencyKey();
      if (!isStaff) {
        verification.clearStoredToken();
        verification.setCode("");
      }
      setVerificationSatisfied(false);
      setVerificationBlocked(false);
    } catch (error) {
      if (requestId !== lookupRequestId.current) return;
      if (error instanceof TypeError) {
        setLookupError("Couldn't connect. Check your connection and try again.");
      } else {
        setLookupError(error instanceof Error ? error.message : "We couldn't find your profile");
      }
    } finally {
      if (requestId === lookupRequestId.current) setLookupLoading(false);
    }
  };

  const handleChangeStudent = () => {
    lookupRequestId.current += 1;
    setLookup(null);
    setStudentProfile(null);
    setLookupLoading(false);
    setLookupError(null);
    window.requestAnimationFrame(() => {
      studentIdInputRef.current?.focus();
      studentIdInputRef.current?.select();
    });
  };

  const togglePickerEntry = (entry: SubjectPickerEntry) => {
    const removing = new Set(entry.subjectIds);
    const allSelected = entry.subjectIds.every((id) => selectedSubjectIds.includes(id));
    if (allSelected) {
      setSelectedSubjectIds((current) => current.filter((id) => !removing.has(id)));
      setExpandedSubjectId((expanded) =>
        expanded !== null && removing.has(expanded)
          ? selectedSubjectIds.filter((id) => !removing.has(id))[0] ?? null
          : expanded,
      );
      return;
    }
    setSelectedSubjectIds((current) => [...current, ...entry.subjectIds.filter((id) => !current.includes(id))]);
    setExpandedSubjectId(entry.subjectIds[0]);
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
      setPageError(null);
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

  const handleNotAvailable = async (group: SubjectSessions, sessionId: string, browsingLevel?: number) => {
    const currentLevel = browsingLevel ?? (sitInPriorityLevels[sessionId] || group.sit_in?.current_priority_level || firstPriorityLevel(group));
    const requestKey = `${lookupRequestId.current}:${sessionId}`;
    if (revealRequestsRef.current.has(requestKey)) return;
    const requestId = lookupRequestId.current;
    setSitInPriorityErrors((current) => { const next = { ...current }; delete next[sessionId]; return next; });
    if (lookup && hasServerPriorityReveal(group)) {
      revealRequestsRef.current.add(requestKey);
      setRevealingPrioritySessionIds((current) => new Set(current).add(sessionId));
      if (isStaff) setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
      setSitInPriorityHistory((prev) => ({ ...prev, [sessionId]: { ...(prev[sessionId] ?? {}), [currentLevel]: group } }));
      try {
        const data = await loadFormSessions(
          lookup.wcode,
          undefined,
          { courseIds: [group.course_id], satVerbalAfterPriority: currentLevel },
        );
        if (requestId !== lookupRequestId.current) return;
        const updatedGroup = data.subjects.find((subject) => subject.course_id === group.course_id);
        if (!updatedGroup) throw new Error("No additional make-up times were returned.");
        const updatedSessionGroup = groupWithSitInForMissedSession(updatedGroup, sessionId);
        const updatedLevel = updatedSessionGroup.sit_in?.current_priority_level ?? firstPriorityLevel(updatedSessionGroup);
        if (updatedLevel <= currentLevel && updatedSessionGroup.sit_in?.has_next_priority) {
          throw new Error("No additional make-up times were returned.");
        }
        if (isStaff) setSitInPriorityLevels((prev) => ({ ...prev, [sessionId]: updatedLevel }));
        setSitInPriorityHistory((prev) => ({ ...prev, [sessionId]: { ...(prev[sessionId] ?? {}), [updatedLevel]: updatedSessionGroup } }));
      } catch (error) {
        if (requestId !== lookupRequestId.current) return;
        if (!isStaff && isStudentSessionUnauthorized(error)) {
          handleStudentSessionExpired();
          return;
        }
        if (isStaff) setPageError(error instanceof Error ? error.message : "Couldn't load other make-up times");
        else setSitInPriorityErrors((current) => ({ ...current, [sessionId]: "Couldn’t load more times. Your selections haven’t changed. Try again." }));
      } finally {
        revealRequestsRef.current.delete(requestKey);
        if (requestId === lookupRequestId.current) setRevealingPrioritySessionIds((current) => { const n = new Set(current); n.delete(sessionId); return n; });
      }
      return;
    }
    const nextLevel = nextPriorityLevel(group, currentLevel);
    if (nextLevel == null) return;
    if (isStaff) {
      setSitInPriorityLevels((prev) => ({ ...prev, [sessionId]: nextLevel }));
      setSitInSelections((prev) => { const n = { ...prev }; delete n[sessionId]; return n; });
    } else {
      setSitInPriorityHistory((prev) => ({ ...prev, [sessionId]: { ...(prev[sessionId] ?? {}), [currentLevel]: group, [nextLevel]: group } }));
    }
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

  const goToStep = useCallback((next: StepIndex, focusId?: string) => {
    navigationFocusRef.current = focusId ?? null;
    setPageError(null);
    setSubmissionError(null);
    setStep(next);
    try { window.scrollTo({ top: 0, behavior: "instant" as ScrollBehavior }); } catch { }
  }, []);

  useEffect(() => {
    const main = document.getElementById("absence-form-content");
    if (main) main.scrollTop = 0;
  }, [step]);

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
    const frame = window.requestAnimationFrame(() => {
      const main = document.getElementById("absence-form-content");
      if (!main) return;
      const requestedTarget = navigationFocusRef.current && document.getElementById(navigationFocusRef.current);
      if (requestedTarget) {
        navigationFocusRef.current = null;
        requestedTarget.focus();
        requestedTarget.scrollIntoView?.({ block: "nearest" });
        return;
      }
      const active = document.activeElement;
      if (active instanceof HTMLElement && active !== document.body && main.contains(active)) return;
      (main.querySelector<HTMLElement>("h1") ?? main).focus({ preventScroll: true });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [step, sessionsLoading]);

  useEffect(() => {
    if (finalResults) resultHeadingRef.current?.focus();
  }, [finalResults]);

  function focusFirstInvalid(selector: string, scopeId?: string) {
    window.requestAnimationFrame(() => {
      const scope = scopeId ? document.getElementById(scopeId) : document;
      const candidates = [...(scope?.querySelectorAll<HTMLElement>(selector) ?? [])];
      const target = candidates.find((candidate) => {
        const styles = window.getComputedStyle(candidate);
        return styles.display !== "none" && styles.visibility !== "hidden" && candidate.getClientRects().length > 0;
      }) ?? candidates[0];
      target?.focus();
      target?.scrollIntoView?.({ block: "nearest" });
    });
  }

  function validateClasses() {
    setShowClassValidation(true);
    setPageError(null);
    setSubmissionError(null);
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
    const logicalSitInSelections = selectedBlocks.flatMap((block) =>
      groupByDay(
        block.sessions.filter(
          (session) =>
            selectedSessionIds.has(session.id) &&
            !session.already_absent,
        ),
      ).map((dayGroup) => ({
        sit_in_session_ids: uniqueValues(
          dayGroup.items.flatMap((missedSession) =>
            splitMergedSessionValue(sitInSelections[missedSession.id]),
          ),
        ),
      })),
    );
    const duplicateIds = duplicateSitInSessionIds(logicalSitInSelections);
    if (duplicateIds.length > 0) {
      setPageError("The same sit-in session is selected for more than one absence day. Choose a different session before submitting.");
      return false;
    }
    if (missingSitIn) {
      setPageError(invalidMakeUpIds.size > 0 ? "A selected make-up class is no longer available. Choose another time for the highlighted date." : "Pick a make-up class for all selected sessions before submitting.");
      const invalidGroup = sessions.find((group) =>
        selectedSubjectIds.includes(group.subject_id) && group.sessions.some((session) =>
          selectedSessionIds.has(session.id) && sitInForMissedSession(group, session.id)?.sit_in_method === "physical" && (!sitInSelections[session.id] || invalidMakeUpIds.has(session.id))));
      setExpandedSubjectId(invalidGroup?.subject_id ?? selectedSubjectIds[0] ?? null);
      const invalidSession = invalidGroup?.sessions.find((session) =>
        selectedSessionIds.has(session.id) && sitInForMissedSession(invalidGroup, session.id)?.sit_in_method === "physical" && (!sitInSelections[session.id] || invalidMakeUpIds.has(session.id)));
      focusFirstInvalid('[data-make-up-trigger], select', invalidSession ? `sit-in-${invalidSession.id}-field` : undefined);
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
    if (!isStaff) {
      const verificationExpired = Boolean(verification.token && verification.expiresAt && verification.expiresAt < Date.now());
      if (!verificationSatisfied || verificationBlocked || verificationExpired) {
        setVerificationSatisfied(false);
        setVerificationBlocked(true);
        goToStep(1);
        return;
      }
    }
    if (!validateClasses()) return;
    if (!lookup) { setPageError("Search for your profile first."); return; }
    try {
      setIsSubmitting(true);
      let submissionSessions: SubjectSessions[];
      try {
        const latest = await loadFormSessions(lookup.wcode);
        submissionSessions = latest.subjects;
        setSessions(submissionSessions);
        if (!isStaff) {
          const freshChoices = publicMakeUpChoicesBySession({
            sessions: submissionSessions, selectedSubjectIds, selectedSessionIds, selections: sitInSelections,
            priorityLevels: sitInPriorityLevels, priorityHistory: sitInPriorityHistory,
            blockedSessionIds: rejectedMakeUpSessionIds,
          });
          const invalidChoice = [...freshChoices].find(([id, choices]) => {
            const value = sitInSelections[id];
            return value && !choices.options.some((option) => option.value === value && !option.disabled);
          });
          if (invalidChoice) {
            const owner = submissionSessions.find((group) => group.sessions.some((session) => session.id === invalidChoice[0]));
            setExpandedSubjectId(owner?.subject_id ?? selectedSubjectIds[0] ?? null);
            setStep(classesStep);
            setSubmissionError("A selected make-up class is no longer available. Your other choices are saved. Choose another time for the highlighted date.");
            return;
          }
        }
        const blockedSessionIds = blockedSitInSessionIds(submissionSessions);
        const selectedSitInSessionIds = new Set(
          Object.values(sitInSelections).flatMap((value) => splitMergedSessionValue(value)),
        );
        const staleSessionIds = [...selectedSitInSessionIds].filter((id) => blockedSessionIds.has(id));
        if (staleSessionIds.length > 0) {
          draftRef.current = null;
          if (isStaff) {
            setSitInSelections({});
            setSitInPriorityLevels({});
            setSitInPriorityHistory({});
          }
          setStep(classesStep);
          setSubmissionError("A selected sit-in session is no longer available. We refreshed the available sessions; choose another session and submit again.");
          return;
        }
      } catch (error) {
        if (!isStaff && isStudentSessionUnauthorized(error)) {
          handleStudentSessionExpired();
          return;
        }
        setSubmissionError(error instanceof Error ? error.message : "Couldn't refresh available sit-in sessions. Please try again.");
        return;
      }
      const payloadResult = buildAbsenceSubmissionPayloads({
        lookupWcode: lookup.wcode,
        sessions: submissionSessions,
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
      if (isStaff) {
        const response = await submitStaffAbsenceFormBatch({
          idempotencyKey: submissionIdempotencyKey.current,
          wcode: lookup.wcode,
          items: payloads,
        });
        setFinalResults(response.items);
      } else {
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
      }
      if (!isStaff) verification.setCode("");
      try {
        if (!isStaff) clearLegacyAbsenceDraft();
        if (!isStaff) clearStudentResume();
        clearDraft();
      } catch { }
    } catch (error) {
      if (!isStaff && isStudentSessionUnauthorized(error)) {
        handleStudentSessionExpired();
        return;
      }
      if (error instanceof ApiRequestError && error.code === "sit_in_session_already_used") {
        draftRef.current = null;
        if (isStaff) {
          setSitInSelections({});
          setSitInPriorityLevels({});
          setSitInPriorityHistory({});
        } else {
          const details = error.details;
          const reportedIds = details && typeof details === "object" && "session_ids" in details && Array.isArray(details.session_ids)
            ? details.session_ids.filter((id): id is string => typeof id === "string") : [];
          const chosenValues = [...new Set([...publicChoicesBySession.keys()].map((id) => sitInSelections[id]).filter(Boolean))];
          const rejectedIds = new Set(reportedIds.length ? reportedIds : chosenValues.length === 1 ? splitMergedSessionValue(chosenValues[0]) : []);
          setRejectedMakeUpSessionIds((current) => new Set([...current, ...rejectedIds]));
          const affected = selectedGroups.find((group) => group.sessions.some((session) => splitMergedSessionValue(sitInSelections[session.id]).some((id) => rejectedIds.has(id))));
          if (affected) setExpandedSubjectId(affected.subject_id);
        }
        setSessionsReloadToken((current) => current + 1);
        setStep(classesStep);
        setSubmissionError(formatSitInSubmissionConflictDetails(error.details) ?? "That sit-in session was just used for this student. We refreshed the available sessions; choose another session and submit again.");
      } else if (error instanceof ApiRequestError && error.code === "absence_limit_exceeded") {
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

  if (finalResults) {
    const submittedGroups = groupSubmittedAbsences(finalResults, sessions);
    const submittedCount = submittedGroups.length;
    const successMessage = isStaff
      ? submittedCount === 1
        ? "Your absence request has been recorded and is waiting for review."
        : `Your ${submittedCount} absence requests have been recorded and are waiting for review.`
      : submittedCount === 1
        ? "Your absence request has been sent and is waiting for review."
        : `Your ${submittedCount} absence requests have been sent and are waiting for review.`;
    const referenceId = finalResults[0]?.id?.slice(0, 8).toUpperCase() || "";
    return (
      <main data-form-mode={mode} className="absence-receipt min-h-screen bg-[var(--color-wi-bg)] px-4 py-8">
        <div className="mx-auto max-w-lg space-y-6">
          <p className="absence-receipt__identity">Warwick Institute <span>Student absence</span></p>
          <div className="absence-receipt__confirmation rounded-lg border border-[var(--color-wi-green)]/30 bg-white p-6 shadow-sm" aria-live="polite">
            <div className="flex items-center gap-3">
              <div className="absence-receipt__check flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-green)]/10">
                <svg aria-hidden="true" className="h-5 w-5 text-[var(--color-wi-green)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path pathLength={1} strokeLinecap="round" strokeLinejoin="round" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
              <div>
                <h1 ref={resultHeadingRef} tabIndex={-1} className="text-xl font-bold tracking-tight text-[var(--color-wi-text)]">
                  {isStaff
                    ? submittedCount === 1 ? "Absence recorded" : `${submittedCount} absences recorded`
                    : submittedCount === 1 ? "Absence submitted" : `${submittedCount} absences submitted`}
                </h1>
                {referenceId && (
                  <p className="text-xs text-[var(--color-wi-text-light)] mt-0.5">Reference: #{referenceId}</p>
                )}
              </div>
            </div>
            <p className="mt-3 text-sm text-[var(--color-wi-text-light)]">{successMessage}</p>
          </div>
          <div className="absence-receipt__classes rounded-lg border border-[var(--color-wi-border)] bg-white p-6 shadow-sm">
            <h3 className="text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wide">Submitted classes</h3>
            <div className="mt-4 space-y-3">
              {submittedGroups.map((group) => {
                return (
                  <article key={group.key} className="rounded-lg border border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] p-4">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div className="min-w-0">
                        <p className="text-sm font-semibold text-[var(--color-wi-text)]">{group.label}</p>
                      </div>
                    </div>
                    <div className="absence-receipt__pair mt-3 flex gap-4 text-sm text-[var(--color-wi-text-light)]">
                      <p><span className="font-medium text-[var(--color-wi-text)]">Absence:</span> {formatSubmittedAbsenceSummary(group)}</p>
                      <p><span className="font-medium text-[var(--color-wi-text)]">Make-up:</span> {formatSubmittedSitInSummary(group)}</p>
                    </div>
                  </article>
                );
              })}
            </div>
          </div>

        </div>
      </main>
    );
  }

  
  const actionCanProceed =
    step === 0 ? isStaff ? canProceedFromStudent : !lookupLoading && (!publicLookup || canProceedFromStudent) :
    !isStaff && step === 1 ? verificationSatisfied :
    step === classesStep ? !sessionsLoading && !sessionsError :
    step === reviewStep ? (isStaff || (verificationSatisfied && !verificationBlocked)) : false;

  const primaryActionLabel =
    step === 0 ? (isStaff ? "Continue to classes" : publicLookup ? "Continue" : "Find student") :
    !isStaff && step === 1 ? "Continue to classes" :
    step === classesStep ? "Review absence" :
    "Submit absence";

  const parentPhoneEnding = publicLookup?.parent_phone_hint?.replace(/\D/g, "").slice(-4);
  const actionHint = isStaff ? undefined
    : step === 0 ? !publicLookup ? undefined
      : !emailSatisfied ? "Add an email address to continue."
        : !publicLookup.parent_verification_available ? "Next: add a parent’s phone number for verification."
          : parentPhoneEnding ? `Next: parent verification using the phone ending in ${parentPhoneEnding}.`
            : "Next: parent verification."
    : step === 1 ? verificationSatisfied ? "Verification complete. Continue to your classes."
      : "Confirm using a code sent to the parent’s phone."
    : step === classesStep && sessionsLoading ? "Loading your courses and class dates…"
    : step === classesStep && sessionsError ? "Reload your classes to continue."
    : step === reviewStep ? "Your request will be sent to Warwick for review."
    : undefined;

  const visibleStep = isStaff
    ? step === classesStep ? 1 : step === reviewStep ? 2 : 0
    : step;

  const handleBack = () => {
    if (isStaff) {
      if (step === reviewStep) goToStep(classesStep);
      else if (step === classesStep) goToStep(0);
      return;
    }
    goToStep(Math.max(0, step - 1) as StepIndex);
  };

  const handleStepClick = (next: number) => {
    const target = isStaff
      ? [0, classesStep, reviewStep][next]
      : next;
    if (target !== undefined && target < step) goToStep(target as StepIndex);
  };

  const handlePrimaryAction = () => {
    if (step === 0) {
      if (!isStaff && !publicLookup) void handleLookup();
      else goToStep(isStaff ? classesStep : 1);
    }
    else if (!isStaff && step === 1) goToStep(classesStep);
    else if (step === classesStep) {
      if (!validateClasses()) return;
      goToStep(reviewStep);
    } else if (step === reviewStep) {
      void handleSubmitAbsence();
    }
  };

  if (configLoading) {
    return (
      <AbsenceAppShell
        mode={mode}
        header={<AbsenceAppHeader steps={STEP_LABELS} currentStep={0} />}
        footer={
          <AbsenceActionBar
            currentStep={visibleStep}
            canProceed={false}
            onBack={() => {}}
            onPrimary={() => {}}
            primaryLabel={primaryActionLabel}
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
      mode={mode}
      header={
        <AbsenceAppHeader
          steps={STEP_LABELS}
          currentStep={visibleStep}
          onStepClick={isSubmitting ? undefined : handleStepClick}
        />
      }
      footer={
          <AbsenceActionBar
            currentStep={visibleStep}
            canProceed={actionCanProceed}
            loading={isSubmitting || (!isStaff && step === 0 && lookupLoading)}
            loadingLabel={!isStaff && step === 0 ? "Finding student…" : undefined}
            loadingHint={!isStaff && step === 0 ? "Finding the student’s record…" : undefined}
            onBack={handleBack}
          onPrimary={handlePrimaryAction}
          primaryLabel={primaryActionLabel}
          hint={actionHint}
        />
      }
    >
      <div className="absence-form-content mx-auto w-full max-w-3xl py-6" inert={isSubmitting} aria-busy={isSubmitting}>
        {pageError || submissionError ? <FormAlert alertRef={pageAlertRef} message={submissionError || pageError || ""} /> : null}

        <p aria-live="polite" className="sr-only">
          Step {visibleStep + 1} of {STEP_LABELS.length}: {STEP_LABELS[visibleStep].label} — {STEP_LABELS[visibleStep].description}
        </p>

        <motion.div
          key={step}
          initial={reduceMotion ? false : { opacity: 0, x: 16 * stepMotion.direction }}
          animate={{ opacity: 1, x: 0 }}
          transition={reduceMotion ? { duration: 0 } : { duration: 0.26, ease: [0.22, 1, 0.36, 1] }}
          className="space-y-6"
        >
            {step === 0 && (
              <StudentStep publicForm={!isStaff}>
                <div className="absence-student-lookup space-y-4">
                  {!isStaff && !publicLookup ? <p className="text-base text-[var(--color-wi-text-light)]">Enter the student’s Warwick ID.</p> : null}
                  {isStaff || !publicLookup ? <div>
                    <label htmlFor="wcode-input" className="block text-sm font-semibold text-[var(--color-wi-text)] mb-1.5">
                      Student ID (W-Code)
                    </label>
                    <div className="absence-lookup-controls flex gap-3">
                      <div className="min-w-0 flex-1">
                        <input
                          ref={studentIdInputRef}
                          id="wcode-input"
                          className="min-h-[48px] w-full rounded-xl border border-[var(--color-wi-border)] bg-white px-4 text-base text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                          placeholder="e.g. W250389"
                          required
                          autoCapitalize="characters"
                          autoCorrect="off"
                          spellCheck={false}
                          enterKeyHint="search"
                          aria-invalid={Boolean(lookupError)}
                          aria-describedby={lookupError ? "wcode-error" : lookupStale ? "wcode-stale-hint" : undefined}
                          value={lookupInput}
                          onChange={(e) => {
                            const next = e.target.value;
                            if (lookupLoading && normalizeLookupWcode(next) !== normalizeLookupWcode(lookupInput)) {
                              lookupRequestId.current += 1;
                              setLookupLoading(false);
                            }
                            setLookupInput(next);
                            setLookupError(null);
                          }}
                          onKeyDown={(e) => { if (e.key === "Enter") void handleLookup(); }}
                        />
                      </div>
                      {isStaff ? <button
                        type="button"
                        onClick={() => void handleLookup()}
                        disabled={lookupLoading}
                        aria-busy={lookupLoading}
                        className={`inline-flex min-h-[48px] min-w-[6.75rem] items-center justify-center rounded-lg px-5 text-sm font-semibold transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2 disabled:opacity-50 ${currentLookup ? "border border-[var(--color-wi-primary)] bg-white text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary)]/5" : "bg-[var(--color-wi-primary)] text-white hover:bg-[var(--color-wi-primary-dark)]"}`}
                      >
                        {lookupLoading ? (
                          <span className="inline-flex items-center gap-2">
                            <LoaderCircle className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
                            <span>Searching…</span>
                          </span>
                        ) : "Search"}
                      </button> : null}
                    </div>
                    {lookupError ? (
                      <p id="wcode-error" role="alert" className="text-sm text-[var(--color-wi-red)] mt-1.5">{lookupError}</p>
                    ) : null}
                    {lookupStale ? (
                      <p id="wcode-stale-hint" role="status" className="text-sm text-[var(--color-wi-amber)] mt-1.5">
                        Search this Student ID before continuing.
                      </p>
                    ) : null}
                  </div> : null}

                  {currentLookup ? (
                    <motion.div
                      role={!isStaff ? "group" : undefined}
                      aria-label={!isStaff ? "Selected student" : undefined}
                      initial={isStaff || reduceMotion ? false : { opacity: 0, y: 8 }}
                      animate={{ opacity: 1, y: 0 }}
                      transition={{ duration: reduceMotion ? 0 : 0.24, ease: [0.22, 1, 0.36, 1] }}
                      className={isStaff ? "absence-lookup-result rounded-lg border border-[var(--color-wi-border)] bg-white p-5 shadow-sm" : "absence-student-identity"}
                    >
                        <div className="flex items-start justify-between gap-3">
                          <div className="min-w-0">
                            {isStaff ? <>
                              <p role="status" className="text-sm font-semibold text-[var(--color-wi-text)]">Student ID found</p>
                              <p className="text-xs font-mono text-[var(--color-wi-text-light)] mt-0.5">{currentLookup.wcode}</p>
                            </> : <>
                              <h2 ref={selectedStudentHeadingRef} tabIndex={-1} aria-describedby="selected-student-id" className="absence-student-identity__name">
                                {publicLookup?.nickname?.trim() || "Student"}
                              </h2>
                              <p id="selected-student-id" className="absence-student-identity__id">Student ID: <span>{currentLookup.wcode}</span></p>
                            </>}
                          </div>
                          {isStaff ? <span className="rounded-full bg-green-100 px-2 py-0.5 text-[10px] font-medium text-green-700">Ready to continue</span> : (
                            <button type="button" onClick={handleChangeStudent} className="absence-student-identity__change min-h-11 rounded-lg px-2 text-sm font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-row-alt)]">
                              Change student
                            </button>
                          )}
                        </div>
                        {isStaff ? (
                          <p className="mt-3 text-xs text-[var(--color-wi-text-light)]">
                            {studentDisplayName} is ready for class selection.
                          </p>
                        ) : publicLookup?.email_input_required ? (
                          <div className="mt-4 space-y-1.5">
                            <label htmlFor="student-email" className="block text-xs font-medium text-[var(--color-wi-text-light)]">
                              Email address <span aria-hidden="true" className="text-[var(--color-wi-red)]">*</span>
                            </label>
                            <input
                              id="student-email"
                              type="email"
                              required
                              autoComplete="email"
                              inputMode="email"
                              autoCapitalize="none"
                              spellCheck={false}
                              onBlur={() => setEmailTouched(true)}
                              className="min-h-[48px] w-full rounded-xl border border-[var(--color-wi-border)] bg-white px-4 text-base text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                              placeholder="e.g. student@example.com"
                              value={collectedEmail}
                              onChange={(e) => setCollectedEmail(e.target.value)}
                              aria-invalid={showEmailError || undefined}
                              aria-describedby={showEmailError ? "student-email-error" : "student-email-help"}
                            />
                            {showEmailError && (
                              <p id="student-email-error" className="text-xs text-[var(--color-wi-red)]">Enter a valid email address.</p>
                            )}
                            <p id="student-email-help" className="sr-only">Add an email address to continue.</p>
                          </div>
                        ) : null}
                    </motion.div>
                  ) : null}
                </div>
              </StudentStep>
            )}

            {!isStaff && step === 1 && (
              publicLookup ? (
                <VerificationStep
                  studentName={studentDisplayName}
                  wcode={publicLookup.wcode}
                  hasPhone={publicLookup.parent_verification_available}
                  phoneLabel={publicLookup.parent_verification_available
                    ? publicLookup.parent_phone_hint
                      ? `Parent phone: ${publicLookup.parent_phone_hint}`
                      : "A parent phone number is on file."
                    : "No parent phone yet — you'll add it below"}
                >
                  <StepCoverVerification
                    publicForm
                    lookupToken={publicLookup.lookup_token}
                    wcode={publicLookup.wcode}
                    parentVerificationAvailable={publicLookup.parent_verification_available}
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

            {step === classesStep && (
              <ClassesStep>

                {lookup ? (
                  <div className="absence-classes-layout">
                    <div className="absence-classes-layout__subjects">
                      <section>
                      <h2 className="text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wide mb-3">{isStaff ? "Which classes?" : "Select courses"}</h2>
                      {sessionsLoading ? (
                        <div role="status" aria-label="Loading courses">
                          <LoadingSkeleton type="text" lines={3} />
                          <p className="sr-only">Loading your courses and class dates…</p>
                        </div>
                      ) : sessionsError ? (
                        <div role="alert" className="space-y-3">
                          <p className="text-sm text-[var(--color-wi-red)]">{sessionsError}</p>
                          <button
                            type="button"
                            onClick={() => setSessionsReloadToken((token) => token + 1)}
                            className="min-h-11 rounded-lg border border-[var(--color-wi-red)]/40 px-3 text-sm font-semibold text-[var(--color-wi-red)] transition-colors motion-reduce:transition-none hover:bg-[var(--color-wi-danger-bg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-red)]/40"
                          >
                            Retry loading classes
                          </button>
                        </div>
                      ) : studentProfile && pickerEntries.length > 0 ? (
                        <div className="rounded-lg border border-[var(--color-wi-border)] bg-white divide-y divide-[var(--color-wi-border)] overflow-hidden">
                          {pickerEntries.map((entry) => (
                            <SubjectRow
                              key={entry.key}
                              id={entry.key}
                              name={isStaff ? appendTeacher(entry.label, entry.teacherName) : entry.label}
                              detail={!isStaff ? entry.teacherName : undefined}
                              selected={entry.subjectIds.every((id) => selectedSubjectIdSet.has(id))}
                              onToggle={() => togglePickerEntry(entry)}
                            />
                          ))}
                        </div>
                      ) : (
                        <p className="text-sm text-[var(--color-wi-text-light)]">No courses available.</p>
                      )}
                    </section>
                    </div>
                    <div className="absence-classes-layout__work space-y-6">
                    {!sessionsLoading && !sessionsError && selectedSubjectIds.length === 0 ? (
                      <div className="absence-classes-empty">
                        <p className="font-semibold text-[var(--color-wi-text)]">Start with a course</p>
                        <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">Select a course to see its class dates and make-up options.</p>
                      </div>
                    ) : null}
                    {selectedSubjectIds.length > 0 ? (
                      <section>
                        <div className="flex items-center justify-between mb-3">
                          <h2 className="text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wide">{isStaff ? "Classes to miss" : "Choose class dates"}</h2>
                          <span aria-live="polite" className="text-xs font-semibold text-[var(--color-wi-text-light)]">
                            {selectedAbsenceDayCount} class date{selectedAbsenceDayCount === 1 ? "" : "s"} selected
                            {isStaff && selectedSubjectRemainingDays > 0
                              ? ` (${selectedSubjectRemainingDays} remaining)`
                              : ""}
                          </span>
                        </div>
                        {isStaff ? <div className="mb-4 space-y-2 sm:hidden" aria-label="Selected classes">
                          {selectedBlocks.map((block) => {
                            const selectedForBlock = block.groups
                              .reduce((count, group) => count + countSelectedAbsenceDaysForGroup(group, selectedSessionIds), 0);
                            const expanded = expandedSubjectId !== null && block.subjectIds.includes(expandedSubjectId);
                            return (
                              <button
                                key={block.key}
                                type="button"
                                aria-expanded={expanded}
                                aria-controls={`class-block-${block.key}`}
                                onClick={() => setExpandedSubjectId(expanded ? null : block.subjectIds[0])}
                                className="flex min-h-[48px] w-full items-center justify-between rounded-xl border border-[var(--color-wi-border)] bg-white px-4 text-left text-sm font-semibold text-[var(--color-wi-text)]"
                              >
                                <span>{appendTeacher(block.label, block.teacherName)}</span>
                                <span className="text-xs font-medium text-[var(--color-wi-text-light)]">
                                  {selectedForBlock > 0 ? `${selectedForBlock} class day${selectedForBlock === 1 ? "" : "s"} selected` : expanded ? "Open" : "Choose classes"}
                                </span>
                              </button>
                            );
                          })}
                        </div> : null}
                        {sessionsLoading ? (
                          <LoadingSkeleton type="table" lines={3} />
                        ) : sessionsError ? null : selectedBlocks.length === 0 ? (
                          <p className="text-sm text-[var(--color-wi-text-light)]">No classes found for the selected courses.</p>
                        ) : (
                          <div className="space-y-4">
                            {selectedBlocks.map((block) => {
                              const sessionGroups = groupByDay(block.sessions);
                              const groupLabel = appendTeacher(block.label, block.teacherName);
                              const scopeKey = block.key;
                              const scopedGroups = scopeIndex.get(scopeKey) ?? [];
                              const groupRemaining = Math.max(...scopedGroups.map(remainingForGroup), 0);
                              const selectedDaysInGroup = selectedDaysByScope.get(scopeKey) ?? 0;
                              const effectiveRemaining = Math.max(0, groupRemaining - selectedDaysInGroup);
                              const quotaGroup = block.groups[0];
                              const quotaExhausted = block.groups.every((group) => group.absence_limit_reached) || groupRemaining === 0;
                              const displayedRemaining = quotaExhausted ? 0 : effectiveRemaining;
                              const expanded = block.subjectIds.includes(expandedSubjectId ?? "");
                              return (
                                <div
                                  key={block.key}
                                  id={`class-block-${block.key}`}
                                  tabIndex={-1}
                                  className={clsx(
                                    "absence-course-block rounded-lg border border-[var(--color-wi-border)] bg-white overflow-hidden shadow-sm",
                                    isStaff && !expanded && "hidden sm:block",
                                  )}
                                >
                                  {isStaff ? <div className="flex items-center justify-between gap-2 border-b border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] px-4 py-3">
                                    <span className="text-sm font-semibold text-[var(--color-wi-text)] truncate">{groupLabel} ({sessionGroups.length} class day{sessionGroups.length !== 1 ? "s" : ""})</span>
                                    <span className="text-xs font-semibold text-[var(--color-wi-text-light)] shrink-0">
                                      {quotaExhausted
                                        ? "Absence limit reached"
                                        : effectiveRemaining === 0
                                          ? "All remaining days selected"
                                          : `${displayedRemaining} day${displayedRemaining !== 1 ? "s" : ""} remaining`}
                                    </span>
                                  </div> : (
                                    <div className="absence-course-heading relative border-b border-[var(--color-wi-line)] p-4 pr-12 sm:pr-4">
                                      <h3 id={`class-heading-${block.key}`} className="text-base font-semibold text-[var(--color-wi-text)]">{block.label}</h3>
                                      {block.teacherName ? <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">{block.teacherName}</p> : null}
                                      <p id={`class-summary-${block.key}`} className="mt-2 text-xs text-[var(--color-wi-text-light)] sm:hidden">
                                        {selectedDaysInGroup > 0 ? `${selectedDaysInGroup} class day${selectedDaysInGroup === 1 ? "" : "s"} selected` : expanded ? "Open" : "Choose classes"}
                                      </p>
                                      <p id={`class-quota-${block.key}`} className="mt-1 text-xs text-[var(--color-wi-text-light)]">
                                        {quotaExhausted ? "Absence limit reached" : effectiveRemaining === 0 ? "All remaining days selected" : `${displayedRemaining} day${displayedRemaining === 1 ? "" : "s"} remaining`}
                                      </p>
                                      <button
                                        type="button"
                                        aria-labelledby={`class-heading-${block.key} class-summary-${block.key} class-quota-${block.key}`}
                                        aria-expanded={expanded}
                                        aria-controls={`class-dates-${block.key}`}
                                        onClick={() => setExpandedSubjectId(expanded ? null : block.subjectIds[0])}
                                        className="absence-course-disclosure absolute inset-0 flex items-start justify-end p-4 sm:hidden"
                                      >
                                        <ChevronDown aria-hidden="true" className={clsx("h-5 w-5 shrink-0 text-[var(--color-wi-text-light)]", expanded && "rotate-180")} />
                                      </button>
                                    </div>
                                  )}
                                  <div id={`class-dates-${block.key}`} className={clsx("absence-course-dates space-y-2 p-4", !isStaff && !expanded && "hidden sm:block")}>
                                      {sessionGroups.map((dayGroup) => {
                                        const session = dayGroup.items.find((item) => !item.already_absent) ?? dayGroup.items[0];
                                        const ownerGroup = ownerGroupBySessionId.get(session.id) ?? quotaGroup;
                                        const sessionIds = dayGroup.items
                                          .filter((item) => !item.already_absent)
                                          .map((item) => item.id);
                                        const alreadyAbsent = sessionIds.length === 0;
                                        const selected = !alreadyAbsent
                                          && sessionIds.every((sessionId) => selectedSessionIds.has(sessionId));
                                        const selectionDisabled = !selected && (quotaExhausted || effectiveRemaining === 0 || selectedDaysInGroup >= maxSessions);
                                        const disabledReason = quotaExhausted
                                          ? "absence_limit_reached"
                                          : selectionDisabled
                                            ? "remaining_days_selected"
                                            : undefined;
                                        const disabledDetail = quotaExhausted
                                          && quotaGroup.used_absence_days != null
                                          && (quotaGroup.maximum_absence_days != null || quotaGroup.total_course_days != null)
                                          ? `${quotaGroup.used_absence_days} absence day${quotaGroup.used_absence_days !== 1 ? "s" : ""} used, max ${quotaGroup.maximum_absence_days ?? Math.round((quotaGroup.total_course_days ?? 0) / 5)}`
                                          : undefined;
                                        const currentSitIn = sitInSelections[session.id] || "";
                                        const sessionGroup = groupWithSitInForMissedSession(ownerGroup, session.id);
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

                                      return (
                                        <SessionDayCard
                                          key={dayGroup.id}
                                          dayGroup={dayGroup}
                                          selected={selected}
                                          alreadyAbsent={alreadyAbsent}
                                          disabled={selectionDisabled}
                                          disabledReason={disabledReason}
                                          disabledDetail={disabledDetail}
                                          onToggle={() => handleSessionGroupToggle(ownerGroup, sessionIds)}
                                          reduceMotion={reduceMotion}
                                        >
                                              {sitIn && sitIn.sit_in_method === "physical" ? (
                                                !isStaff ? (() => {
                                                  const choices = publicChoicesBySession.get(session.id);
                                                  if (!choices) return null;
                                                  return <MakeUpPicker
                                                    publicForm
                                                    id={`sit-in-${session.id}`}
                                                    label="Make-up class"
                                                    context={`For ${block.label} · ${formatDate(dayGroup.date)} · ${formatTime(dayGroup.start_at)}–${formatTime(dayGroup.end_at)}`}
                                                    value={currentSitIn}
                                                    options={choices.options}
                                                    error={showClassValidation && !currentSitIn ? "Choose a make-up class for this date." : undefined}
                                                    supportContact={config.admin_contact}
                                                    emptyDescription={choices.unavailableMessage ? `${choices.unavailableMessage.replace(/\.$/, "")}. A make-up class is required for this date. Contact Warwick for help before continuing.` : undefined}
                                                    moreTimes={hasPriorities || choices.hasMore ? {
                                                      hasMore: choices.hasMore,
                                                      loading: revealingPrioritySessionIds.has(session.id),
                                                      error: sitInPriorityErrors[session.id],
                                                      onLoad: () => void handleNotAvailable(choices.latestGroup, session.id, choices.latestLevel),
                                                    } : undefined}
                                                    onChange={(value) => {
                                                      const option = choices.options.find((item) => item.value === value);
                                                      if (value && (!option || option.disabled)) return;
                                                      if (option?.priorityLevel !== undefined) {
                                                        const level = option.priorityLevel;
                                                        const selectedGroup = choices.groupsByLevel.get(level);
                                                        setSitInPriorityLevels((prev) => ({ ...prev, [session.id]: level }));
                                                        if (selectedGroup) setSitInPriorityHistory((prev) => ({ ...prev, [session.id]: { ...(prev[session.id] ?? {}), [level]: selectedGroup } }));
                                                      }
                                                      handleSitInSelectForSessions(sessionIds, value);
                                                    }}
                                                  />;
                                                })() : (() => {
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
                                                    const unavailableForCard = currentPriorityUnavailable.find((item) => item.session) ?? currentPriorityUnavailable[0];
                                                    const unavailableDisplay = unavailableForCard
                                                      ? normalizeSitInDisplayModel(
                                                        unavailableForCard.session ? [unavailableForCard.session] : [],
                                                        unavailableForCard.sitInCourse,
                                                        block.label,
                                                        sessions,
                                                        { status: "unavailable", reasonCode: unavailableForCard.reason_code },
                                                      )
                                                      : undefined;
                                                    const previousTimesAction = hasPreviousPriority ? (
                                                      <button
                                                        type="button"
                                                        disabled={revealingPriority}
                                                        onClick={() => handlePreviousPriority(priorityGroup, session.id)}
                                                        aria-label="See previous times"
                                                        className="inline-flex h-8 items-center justify-center gap-1 rounded-full px-2.5 text-xs font-medium text-[var(--color-wi-text-light)] transition motion-reduce:transition-none hover:bg-white hover:text-[var(--color-wi-text)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-amber)]/40 disabled:opacity-50"
                                                      >
                                                        <ChevronLeft className="h-3.5 w-3.5" />
                                                        <span>Previous times</span>
                                                      </button>
                                                    ) : null;
                                                    const otherTimesAction = hasMorePriorities ? (
                                                      <button
                                                        type="button"
                                                        disabled={revealingPriority}
                                                        onClick={() => void handleNotAvailable(priorityGroup, session.id)}
                                                        className="inline-flex h-8 items-center justify-center gap-1 rounded-full px-3 text-xs font-semibold text-[var(--color-wi-text-light)] transition motion-reduce:transition-none hover:bg-white hover:text-[var(--color-wi-text)] hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-amber)]/40 disabled:opacity-50"
                                                      >
                                                        <span>{revealingPriority ? "Loading..." : "See other times"}</span>
                                                        {!revealingPriority && (
                                                          <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                                            <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                                                          </svg>
                                                        )}
                                                      </button>
                                                    ) : null;

                                                    if (!currentPriority) {
                                                      return (
                                                        <div role="status" className="rounded-lg border border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] p-3 text-sm text-[var(--color-wi-text-light)]">
                                                          <p className="font-semibold text-[var(--color-wi-text)]">No make-up times available</p>
                                                          {!isStaff && !hasMorePriorities ? (
                                                            <p className="mt-0.5 text-xs">Staff will help arrange the next step.</p>
                                                          ) : null}
                                                          <div className="mt-2 flex flex-wrap gap-2">{previousTimesAction}{otherTimesAction}</div>
                                                        </div>
                                                      );
                                                    }

                                                    return (
                                                      <div className="absence-makeup-arrangement rounded-lg border border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] p-3">
                                                        {currentPriorityAvailable.length === 0 ? (
                                                          <div role="status" className="space-y-1.5 text-sm">
                                                            {hasMorePriorities && unavailableDisplay?.message ? (
                                                              <>
                                                                <p className="font-semibold text-[var(--color-wi-text)]">{unavailableDisplay.message}</p>
                                                                <p className="font-medium text-[var(--color-wi-text)]">{unavailableDisplay.className}</p>
                                                                {formatSitInDisplayDetails(unavailableDisplay) ? (
                                                                  <p className="text-xs text-[var(--color-wi-text-light)]">{formatSitInDisplayDetails(unavailableDisplay)}</p>
                                                                ) : null}
                                                              </>
                                                            ) : (
                                                              <>
                                                                <p className="font-semibold text-[var(--color-wi-text)]">No make-up times available</p>
                                                                {!isStaff ? <p className="text-xs text-[var(--color-wi-text-light)]">Staff will help arrange the next step.</p> : null}
                                                              </>
                                                            )}
                                                            <div className="flex flex-wrap gap-2 pt-1">{previousTimesAction}{otherTimesAction}</div>
                                                          </div>
                                                        ) : (
                                                          <>
                                                            <MakeUpPicker
                                                              publicForm={!isStaff}
                                                              error={!isStaff && showClassValidation && !currentSitIn ? "Choose a make-up class for this date." : undefined}
                                                              context={`For ${block.label} · ${formatDate(dayGroup.date)} · ${formatTime(dayGroup.start_at)}–${formatTime(dayGroup.end_at)}`}
                                                              id={`sit-in-${session.id}`}
                                                              label="Make-up class"
                                                              value={currentSitIn}
                                                              options={makeUpPickerOptions(sitInOptionsByTargetAndSession(currentPriorities, sessionIds), sessions, selectedSubjectIds, block.label, sitIn?.sit_in_course, Object.values(sitInSelections).flatMap(splitMergedSessionValue), currentSitIn, currentSitInOwners(sessions, selectedSubjectIds, selectedSessionIds, sitInSelections, sessionIds))}
                                                              onChange={(value) => handleSitInSelectForSessions(sessionIds, value)}
                                                            />
                                                            {(hasPreviousPriority || hasMorePriorities) && (
                                                              <div className="mt-2 flex flex-wrap gap-2">{previousTimesAction}{otherTimesAction}</div>
                                                            )}
                                                          </>
                                                        )}
                                                      </div>
                                                    );
                                                  }
                                                  return (
                                                    <div>
                                                      {sitInAvailable.length === 0 ? (
                                                        <div role="status" className="rounded-md border border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] px-3 py-2 text-sm">
                                                          <p className="font-semibold text-[var(--color-wi-text)]">No make-up times available</p>
                                                          {!isStaff ? <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">Staff will help arrange the next step.</p> : null}
                                                        </div>
                                                      ) : (
                                                        <>
                                                          {isStaff ? <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-[var(--color-wi-amber)]">Pick a make-up class</div> : null}
                                                          <MakeUpPicker
                                                            publicForm={!isStaff}
                                                            error={!isStaff && showClassValidation && !currentSitIn ? "Choose a make-up class for this date." : undefined}
                                                            context={`For ${block.label} · ${formatDate(dayGroup.date)} · ${formatTime(dayGroup.start_at)}–${formatTime(dayGroup.end_at)}`}
                                                            id={`sit-in-${session.id}`}
                                                            label="Make-up class"
                                                            value={currentSitIn}
                                                            options={makeUpPickerOptions(sitInOptionGroupsBySession(sitInAvailable, sitIn?.sit_in_course), sessions, selectedSubjectIds, block.label, sitIn?.sit_in_course, Object.values(sitInSelections).flatMap(splitMergedSessionValue), currentSitIn, currentSitInOwners(sessions, selectedSubjectIds, selectedSessionIds, sitInSelections, sessionIds))}
                                                            onChange={(value) => handleSitInSelectForSessions(sessionIds, value)}
                                                          />
                                                        </>
                                                      )}
                                                    </div>
                                                  );
                                                })()
                                              ) : sitIn && sitIn.sit_in_method === "zoom" ? (
                                                <div className="space-y-1 text-sm text-[var(--color-wi-text)]">
                                                  <div className="flex items-center gap-2">
                                                    <span className="inline-flex h-5 w-5 items-center justify-center rounded-full bg-[var(--color-wi-primary)]/10 text-[10px] font-bold text-[var(--color-wi-primary)]">Z</span>
                                                    <span className="font-medium">Online make-up (Zoom)</span>
                                                  </div>
                                                  <p className="text-xs text-[var(--color-wi-text-light)] ml-7">{isStaff ? "No need to pick a class" : "Staff will send a Zoom link — no need to pick a class"}</p>
                                                </div>
                                              ) : sitIn && sitIn.sit_in_method === "teacher_case" ? (
                                                <div className="flex items-center gap-2 text-sm text-[var(--color-wi-amber)]">
                                                  <span className="text-xs font-semibold">To arrange</span>
                                                </div>
                                              ) : (
                                                <div className="text-sm text-[var(--color-wi-text-light)]">
                                                  <p className="font-medium">To arrange</p>
                                                  {!isStaff ? (
                                                    <p className="text-xs text-[var(--color-wi-text-light)] mt-0.5">Staff will contact you to set up a make-up class.</p>
                                                  ) : null}
                                                </div>
                                              )}
                                        </SessionDayCard>
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

                    <ReasonField
                      publicForm={!isStaff}
                      value={reason}
                      onChange={(value) => {
                        setReason(value);
                        setReasonError(null);
                      }}
                      error={reasonError}
                      required
                    />
                    </div>
                  </div>
                ) : (
                  <p className="text-sm text-[var(--color-wi-text-light)]">Search for your profile first.</p>
                )}
              </ClassesStep>
            )}

            {step === reviewStep && (
              <ReviewStep>
                {lookup ? (
                  <div className="absence-review space-y-4">
                    <p className="text-sm text-[var(--color-wi-text-light)]">
                      <span className="font-medium text-[var(--color-wi-text)]">{studentDisplayName}</span> — {lookup.wcode}
                    </p>

                    {!isStaff && studentProfile?.nickname_set === false ? (
                      <div className="absence-review__nickname rounded-lg border border-[var(--color-wi-border)] bg-white px-5 py-4 space-y-2">
                        <label htmlFor="student-nickname" className="block text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">
                          Nickname <span className="normal-case tracking-normal font-medium">(optional)</span>
                        </label>
                        <input
                          id="student-nickname"
                          autoComplete="nickname"
                          aria-describedby="nickname-help"
                          maxLength={50}
                          className="min-h-[48px] w-full rounded-xl border border-[var(--color-wi-border)] bg-white px-4 text-base text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                          placeholder="What our staff can call you"
                          value={nickname}
                          onChange={(e) => setNickname(e.target.value)}
                        />
                        <p id="nickname-help" className="text-xs text-[var(--color-wi-text-light)]" data-testid="nickname-echo">
                          {nickname.trim()
                            ? <>Helps our staff recognise you — it will be saved as {maskNickname(nickname)}.</>
                            : <>No nickname saved yet. Adding one helps our staff recognise you.</>}
                        </p>
                      </div>
                    ) : null}

                    {/* Classes section */}
                    <div className="absence-review__section rounded-lg border border-[var(--color-wi-border)] bg-white">
                      <div className="flex items-center justify-between border-b border-[var(--color-wi-border)] px-5 py-3">
                        <h2 className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Classes</h2>
                        <button
                          type="button"
                          onClick={() => {
                            const subjectId = selectedSubjectIds[0];
                            setExpandedSubjectId(subjectId ?? null);
                            const block = selectedBlocks.find((entry) => entry.subjectIds.includes(subjectId));
                            goToStep(classesStep, block ? `class-block-${block.key}` : undefined);
                          }}
                          className="min-h-11 rounded-lg px-2 text-xs font-semibold text-[var(--color-wi-primary)] transition-colors motion-reduce:transition-none hover:bg-[var(--color-wi-primary)]/5 hover:text-[var(--color-wi-primary-dark)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                        >
                          Edit classes
                        </button>
                      </div>
                      <div className="px-5 py-4 space-y-3">
                        {selectedBlocks.map((block) => {
                          const selectedSessions = block.sessions
                            .filter((session) => selectedSessionIds.has(session.id) && !session.already_absent)
                            .sort((a, b) => a.start_at.localeCompare(b.start_at));
                          if (selectedSessions.length === 0) return null;
                          const groupLabel = appendTeacher(block.label, block.teacherName);
                          return (
                            <div key={block.key}>
                              <p className="text-sm font-semibold text-[var(--color-wi-text)]">{groupLabel}</p>
                              {groupByDay(selectedSessions).map((dayGroup) => (
                                <div key={dayGroup.id} className="absence-review-day text-xs text-[var(--color-wi-text-light)] mt-0.5">
                                  <p><span className="absence-review-day__label">Missing: </span>{formatDate(dayGroup.date)} {formatTime(dayGroup.start_at)}–{formatTime(dayGroup.end_at)}</p>
                                  <p><span>Make-up: </span><span className="font-medium text-[var(--color-wi-text)]">{getReviewSitInLabel(dayGroup.items[0], ownerGroupBySessionId.get(dayGroup.items[0].id) ?? block.groups[0], sitInSelections, sitInPriorityLevels, sitInPriorityHistory, sessions)}</span></p>
                                </div>
                              ))}
                            </div>
                          );
                        })}
                      </div>
                    </div>

                    {/* Reason section */}
                    <div className="absence-review__section rounded-lg border border-[var(--color-wi-border)] bg-white">
                      <div className="flex items-center justify-between border-b border-[var(--color-wi-border)] px-5 py-3">
                        <h2 className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Reason</h2>
                        <button
                          type="button"
                          onClick={() => goToStep(classesStep, "absence-reason")}
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
    </AbsenceAppShell>
  );
}
