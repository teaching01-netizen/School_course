import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { motion, useReducedMotion } from "framer-motion";
import { newIdempotencyKey, ApiRequestError } from "@/api/client";
import LoadingSkeleton from "@/components/ui/LoadingSkeleton";
import AbsenceAppShell from "@/components/absences/public-form/AbsenceAppShell";
import AbsenceHeader from "@/components/absences/public-form/AbsenceHeader";
import AbsenceActionBar from "@/components/absences/public-form/AbsenceActionBar";
import FormAlert from "@/components/absences/public-form/FormAlert";
import IdentifyScreen from "@/components/absences/public-form/IdentifyScreen";
import ResumeScreen from "@/components/absences/public-form/ResumeScreen";
import ConfirmStudentScreen from "@/components/absences/public-form/ConfirmStudentScreen";
import ParentConfirmScreen from "@/components/absences/public-form/ParentConfirmScreen";
import EmailScreen from "@/components/absences/public-form/EmailScreen";
import ScheduleScreen from "@/components/absences/public-form/ScheduleScreen";
import MakeUpScreen, { type MakeUpOption } from "@/components/absences/public-form/MakeUpScreen";
import ReasonScreen from "@/components/absences/public-form/ReasonScreen";
import ReviewScreen, { type ReviewSection } from "@/components/absences/public-form/ReviewScreen";
import SuccessScreen, { type SuccessGroup } from "@/components/absences/public-form/SuccessScreen";
import { useToast } from "@/hooks/useToast";
import { useAbsenceDraft } from "@/features/absences/hooks/useAbsenceDraft";
import type { AbsenceDraftV1 } from "@/features/absences/storage/absenceDraftStorage";
import { useOtp } from "@/hooks/useOtp";
import { formatDate, formatTime } from "@/utils/date";
import type {
  AbsenceFormConfig,
  ManagedAbsence,
  PublicStudentLookupResponse,
  SessionInSubject,
  SessionsInRangeResponse,
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
  countSelectedAbsenceDaysForScope,
  groupByDay,
  mergedSessionValue,
  splitMergedSessionValue,
  uniqueValues,
} from "@/features/absences/domain/sessionGrouping";
import { buildSubmissionPayloads as buildAbsenceSubmissionPayloads } from "@/features/absences/domain/submissionPayload";
import {
  appendTeacher,
  blockedSitInSessionIds,
  findSitInSessionConflicts,
  firstPriorityLevel,
  formatHistoricalSitInConflictDescription,
  getReviewSitInLabel,
  groupWithSitInForMissedSession,
  hasServerPriorityReveal,
  nextPriorityLevel,
  prioritiesForLevel,
  resolveSitInSubjectName,
  rootAvailableSessionsForMissedSessions,
  sitInOptionGroupsBySession,
  sitInOptionsByTargetAndSession,
  type SitInCourse,
  type SitInOptionGroup,
} from "@/features/absences/domain/sitInResolution";
import {
  getAbsenceSessionDateLabels,
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
import { isWCode, normalizeLookupWcode } from "@/features/absences/domain/studentIdentity";

type Screen =
  | "identify"
  | "resume"
  | "confirm"
  | "verify"
  | "email"
  | "classes"
  | "makeup"
  | "reason"
  | "review";

const SCREEN_ORDER: Screen[] = ["identify", "resume", "confirm", "verify", "email", "classes", "makeup", "reason", "review"];

const SCREEN_PROGRESS: Record<Screen, number> = {
  identify: 0.04,
  resume: 0.08,
  confirm: 0.12,
  verify: 0.28,
  email: 0.36,
  classes: 0.52,
  makeup: 0.72,
  reason: 0.86,
  review: 0.94,
};

const SCREEN_LABELS: Record<Screen, string> = {
  identify: "Identify",
  resume: "Resume your report",
  confirm: "Confirm your profile",
  verify: "Parent confirmation",
  email: "Contact email",
  classes: "Choose your classes",
  makeup: "Make-up class",
  reason: "Reason",
  review: "Review & submit",
};

const DEFAULT_REASON_CATEGORIES = [
  { value: "not_feeling_well", label: "Not feeling well" },
  { value: "appointment", label: "Appointment" },
  { value: "school_activity", label: "School activity" },
  { value: "family_commitment", label: "Family commitment" },
  { value: "travel", label: "Travel" },
  { value: "other", label: "Other" },
];

function isStudentSessionUnauthorized(error: unknown): boolean {
  return error instanceof ApiRequestError
    && error.status === 401
    && error.code === "unauthorized";
}

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+$/;

type SelectedDay = {
  key: string;
  group: SubjectSessions;
  items: SessionInSubject[];
  sessionIds: string[];
  date: string;
  start_at: string;
  end_at: string;
};

function buildSelectedDays(
  sessions: SubjectSessions[],
  selectedSubjectIds: string[],
  selectedSessionIds: Set<string>,
): SelectedDay[] {
  const selectedSubjects = new Set(selectedSubjectIds);
  const days: SelectedDay[] = [];
  for (const group of sessions) {
    if (!selectedSubjects.has(group.subject_id)) continue;
    for (const dayGroup of groupByDay(group.sessions)) {
      const items = dayGroup.items.filter((item) => !item.already_absent);
      const sessionIds = items.map((item) => item.id);
      if (items.length === 0 || !sessionIds.every((id) => selectedSessionIds.has(id))) continue;
      days.push({
        key: dayGroup.id,
        group,
        items,
        sessionIds,
        date: dayGroup.date,
        start_at: dayGroup.start_at,
        end_at: dayGroup.end_at,
      });
    }
  }
  days.sort((a, b) => a.date.localeCompare(b.date) || a.start_at.localeCompare(b.start_at));
  return days;
}

function classLabel(group: SubjectSessions): string {
  return appendTeacher(
    group.merge_group_name?.trim() || group.subject_name?.trim() || group.course_name?.trim() || "Class",
    group.teacher_name,
  );
}

function selectedDayWhen(day: SelectedDay): string {
  return `${formatDate(day.date)} · ${formatTime(day.start_at)}–${formatTime(day.end_at)}`;
}

type MakeUpPlan = {
  method: "physical" | "zoom" | "teacher_case" | "none";
  options: MakeUpOption[];
  hasMoreTimes: boolean;
};

function buildMakeUpOptions(
  optionGroups: SitInOptionGroup[],
  sessions: SubjectSessions[],
  selectedSubjectIds: string[],
  selectedSitInIds: string[],
  currentSelectionIds: Set<string>,
  fallbackLabel: string,
): MakeUpOption[] {
  const selectedCounts = new Map<string, number>();
  for (const id of selectedSitInIds) selectedCounts.set(id, (selectedCounts.get(id) ?? 0) + 1);

  const options: MakeUpOption[] = [];
  for (const group of optionGroups) {
    const items = group.items;
    if (items.length === 0) continue;
    const conflicts = findSitInSessionConflicts(items, sessions, selectedSubjectIds);
    const historical = items.map(formatHistoricalSitInConflictDescription).find(Boolean);
    const duplicate = items.some((item) => (selectedCounts.get(item.id) ?? 0) > 0 && !currentSelectionIds.has(item.id));
    // Unavailable options are hidden entirely — the student only ever sees
    // choices that can actually succeed.
    if (conflicts.length > 0 || historical || duplicate) continue;

    const first = items[0];
    const sitInCourse: SitInCourse | undefined = group.sitInCourse;
    const name = resolveSitInSubjectName(sitInCourse, sessions)
      || sitInCourse?.name?.trim()
      || first.subject_name?.trim()
      || first.course_name?.trim()
      || first.class_name?.trim()
      || fallbackLabel;
    const teacher = first.teacher_name?.trim()
      || sessions.find((subject) => subject.course_id === sitInCourse?.id)?.teacher_name?.trim();
    const last = items[items.length - 1];
    options.push({
      value: mergedSessionValue(items),
      name,
      date: formatDate(first.start_at),
      time: `${formatTime(first.start_at)}–${formatTime(last.end_at)}`,
      teacher,
    });
  }
  return options;
}

function makeUpPlan(
  day: SelectedDay,
  sessions: SubjectSessions[],
  selectedSubjectIds: string[],
  sitInSelections: Record<string, string>,
  sitInPriorityLevels: Record<string, number>,
  sitInPriorityHistory: Record<string, Record<number, SubjectSessions>>,
): MakeUpPlan {
  const group = day.group;
  const first = day.items[0];
  if (!first) return { method: "none" as const, options: [], hasMoreTimes: false };
  const firstId = first.id;
  const sessionGroup = groupWithSitInForMissedSession(group, firstId);
  const sitIn = sessionGroup.sit_in;
  const method = sitIn?.sit_in_method === "zoom"
    ? "zoom"
    : sitIn?.sit_in_method === "teacher_case"
      ? "teacher_case"
      : sitIn?.sit_in_method === "none" || !sitIn
        ? "none"
        : "physical";

  if (method !== "physical") return { method, options: [], hasMoreTimes: false };

  const hasPriorities = Boolean(sitIn?.priorities && sitIn.priorities.length > 0);
  const currentLevel = sitInPriorityLevels[firstId]
    ?? sitIn?.current_priority_level
    ?? firstPriorityLevel(sessionGroup);
  const priorityGroup = sitInPriorityHistory[firstId]?.[currentLevel] ?? sessionGroup;
  const groupLabel = classLabel(group);
  const currentSelectionIds = new Set(splitMergedSessionValue(sitInSelections[firstId]));
  const selectedSitInIds = Object.values(sitInSelections).flatMap(splitMergedSessionValue);

  if (hasPriorities) {
    const currentPriorities = prioritiesForLevel(priorityGroup, currentLevel);
    const optionGroups = sitInOptionsByTargetAndSession(currentPriorities, day.sessionIds);
    const options = buildMakeUpOptions(optionGroups, sessions, selectedSubjectIds, selectedSitInIds, currentSelectionIds, groupLabel);
    const hasMoreTimes = nextPriorityLevel(priorityGroup, currentLevel) !== null
      || Boolean(priorityGroup.sit_in?.has_next_priority);
    return { method, options, hasMoreTimes };
  }

  const available = rootAvailableSessionsForMissedSessions(sitIn, day.sessionIds);
  const options = buildMakeUpOptions(
    sitInOptionGroupsBySession(available, sitIn?.sit_in_course),
    sessions,
    selectedSubjectIds,
    selectedSitInIds,
    currentSelectionIds,
    groupLabel,
  );
  return { method, options, hasMoreTimes: false };
}

function missingMakeUp(day: SelectedDay, plan: MakeUpPlan, sitInSelections: Record<string, string>): boolean {
  return plan.method === "physical"
    && plan.options.length > 0
    && !sitInSelections[day.items[0]?.id ?? ""];
}

function screenToStep(screen: Screen): 0 | 1 | 2 | 3 {
  if (screen === "classes") return 1;
  if (screen === "makeup") return 2;
  if (screen === "reason" || screen === "review") return 3;
  return 0;
}

export default function AbsenceForm() {
  const { addToast } = useToast();
  const verification = useOtp(VERIFICATION_STORAGE_KEY);
  const reduceMotion = useReducedMotion();
  const { draft: savedDraft, saveDraft, clearDraft } = useAbsenceDraft();
  const draftRef = useRef<AbsenceDraftV1 | null>(savedDraft);
  const [draftNeedsReview, setDraftNeedsReview] = useState(false);
  const submissionIdempotencyKey = useRef(newIdempotencyKey());
  const lookupRequestId = useRef(0);

  const [screen, setScreen] = useState<Screen>("identify");
  const screenRef = useRef<Screen>("identify");
  useEffect(() => { screenRef.current = screen; }, [screen]);

  const [config, setConfig] = useState<AbsenceFormConfig>(DEFAULT_CONFIG);
  const [configLoading, setConfigLoading] = useState(true);
  const [lookupInput, setLookupInput] = useState("");
  const [lookup, setLookup] = useState<PublicStudentLookupResponse | null>(null);
  const [studentProfile, setStudentProfile] = useState<VerifiedStudentProfile | null>(null);
  const [lookupLoading, setLookupLoading] = useState(false);
  const [lookupError, setLookupError] = useState<string | null>(null);
  const [identifySelectOnMount, setIdentifySelectOnMount] = useState(false);
  const [collectedEmail, setCollectedEmail] = useState("");
  const [selectedSubjectIds, setSelectedSubjectIds] = useState<string[]>([]);
  const [reason, setReason] = useState("");
  const [reasonError, setReasonError] = useState<string | null>(null);
  const [sessions, setSessions] = useState<SubjectSessions[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [sessionsError, setSessionsError] = useState<string | null>(null);
  const [sessionsReloadToken, setSessionsReloadToken] = useState(0);
  const [selectedSessionIds, setSelectedSessionIds] = useState<Set<string>>(new Set());
  const [sitInSelections, setSitInSelections] = useState<Record<string, string>>({});
  const [sitInPriorityLevels, setSitInPriorityLevels] = useState<Record<string, number>>({});
  const [sitInPriorityHistory, setSitInPriorityHistory] = useState<Record<string, Record<number, SubjectSessions>>>({});
  const [pageError, setPageError] = useState<string | null>(null);
  const [limitNotice, setLimitNotice] = useState<string | null>(null);
  const [limitNoticeKey, setLimitNoticeKey] = useState<string | null>(null);
  const [makeupNotice, setMakeupNotice] = useState<string | null>(null);
  const [makeupLoadingTimes, setMakeupLoadingTimes] = useState(false);
  const [verificationSatisfied, setVerificationSatisfied] = useState(false);
  const [verificationBlocked, setVerificationBlocked] = useState(false);
  const [confirmedPulse, setConfirmedPulse] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const submittingRef = useRef(false);
  const [submissionError, setSubmissionError] = useState<string | null>(null);
  const [finalResults, setFinalResults] = useState<ManagedAbsence[] | null>(null);
  const pageAlertRef = useRef<HTMLDivElement | null>(null);

  const manualEmail = collectedEmail.trim();
  const manualEmailValid = EMAIL_PATTERN.test(manualEmail);
  const emailRequired = Boolean(lookup?.email_input_required);
  const emailSatisfied = !emailRequired || manualEmailValid;

  const lookupRef = useRef(lookup);
  useEffect(() => { lookupRef.current = lookup; }, [lookup]);
  const emailRef = useRef(collectedEmail);
  useEffect(() => { emailRef.current = collectedEmail; }, [collectedEmail]);

  const studentDisplayName = studentProfile?.display_name || lookup?.nickname_hint || "Student";

  const resumeSummary = useMemo(() => {
    const draft = draftRef.current;
    if (!draft) return undefined;
    const classes = draft.selectedSessionIds.length === 1
      ? "1 class"
      : `${draft.selectedSessionIds.length} classes`;
    const reason = draft.reason.trim();
    return reason ? `${classes} · ${reason}` : classes;
  }, []);

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

  const remainingForGroup = useCallback(
    (group: SubjectSessions): number => {
      if (group.remaining_absence_days != null) return group.remaining_absence_days;
      return config.sit_in.max_sessions_per_absence;
    },
    [config.sit_in.max_sessions_per_absence],
  );

  const selectedDays = useMemo(
    () => buildSelectedDays(sessions, selectedSubjectIds, selectedSessionIds),
    [sessions, selectedSubjectIds, selectedSessionIds],
  );

  const [makeupIndex, setMakeupIndex] = useState(0);
  const currentMakeUpDay = selectedDays[Math.min(makeupIndex, Math.max(0, selectedDays.length - 1))];

  const makeupPlan = useMemo(
    () => currentMakeUpDay
      ? makeUpPlan(currentMakeUpDay, sessions, selectedSubjectIds, sitInSelections, sitInPriorityLevels, sitInPriorityHistory)
      : { method: "none" as const, options: [] as MakeUpOption[], hasMoreTimes: false },
    [currentMakeUpDay, sessions, selectedSubjectIds, sitInSelections, sitInPriorityLevels, sitInPriorityHistory],
  );

  const missingSitIn = useMemo(
    () => selectedDays.some((day) => {
      const plan = makeUpPlan(day, sessions, selectedSubjectIds, sitInSelections, sitInPriorityLevels, sitInPriorityHistory);
      return missingMakeUp(day, plan, sitInSelections);
    }),
    [selectedDays, sessions, selectedSubjectIds, sitInSelections, sitInPriorityLevels, sitInPriorityHistory],
  );

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
    setPageError("Your verified session expired. Confirm with your parent again to continue. Your absence details are still saved.");
    setSubmissionError(null);
    setScreen("verify");
  }, [verification.clearStoredToken, verification.setCode]);

  useEffect(() => {
    if (screen !== "classes" || !lookup) return;
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
          setSessionsError("Verification belongs to a different Student ID. Confirm this Student ID again.");
          setScreen("verify");
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
        setDraftNeedsReview((current) => current || missingSavedSessions > 0 || restoredSessionIds.length > 0);
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
  }, [screen, lookup, sessionsReloadToken, handleStudentSessionExpired]);

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
          // Returning students first see what is saved and choose to continue
          // or start over — never a silent jump back into the flow.
          setScreen("resume");
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
      step: screenToStep(screen),
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
    screen,
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
      setScreen((current) => SCREEN_ORDER.indexOf(current) >= SCREEN_ORDER.indexOf("verify") ? "verify" : current);
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
        setScreen((current) => SCREEN_ORDER.indexOf(current) >= SCREEN_ORDER.indexOf("verify") ? "verify" : current);
      }
    };
    enforceExpiry();
    const timer = window.setTimeout(enforceExpiry, Math.max(0, verification.expiresAt - Date.now()));
    return () => window.clearTimeout(timer);
  }, [verification.expiresAt, verification.token]);

  const advanceAfterVerification = useCallback(() => {
    const nextScreen: Screen = (lookupRef.current?.email_input_required && !EMAIL_PATTERN.test(emailRef.current.trim()))
      ? "email"
      : "classes";
    setScreen(nextScreen);
  }, []);

  const handleVerificationSatisfied = useCallback(() => {
    setVerificationSatisfied(true);
    setVerificationBlocked(false);
    setConfirmedPulse(true);
    window.setTimeout(() => {
      setConfirmedPulse(false);
      if (screenRef.current === "verify") advanceAfterVerification();
    }, 500);
  }, [advanceAfterVerification]);

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

  const goToScreen = useCallback((next: Screen) => {
    setPageError(null);
    setSubmissionError(null);
    setLimitNotice(null);
    setLimitNoticeKey(null);
    setScreen(next);
    try { window.scrollTo({ top: 0, behavior: "instant" as ScrollBehavior }); } catch { }
  }, []);

  const handleLookup = async () => {
    const requestId = ++lookupRequestId.current;
    const cleaned = normalizeLookupWcode(lookupInput);
    setLookupError(null);
    clearStudentSessionHint();
    setLookup(null);
    setStudentProfile(null);
    if (!cleaned || !isWCode(cleaned)) return;
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
      setCollectedEmail(shouldRestoreDraft ? draftForStudent?.collectedEmail ?? "" : "");
      setReason(shouldRestoreDraft ? draftForStudent?.reason ?? "" : "");
      setReasonError(null);
      setSessions([]);
      setSessionsError(null);
      setSelectedSessionIds(new Set());
      setSitInSelections({});
      setSitInPriorityLevels({});
      setSitInPriorityHistory({});
      setDraftNeedsReview(Boolean(shouldRestoreDraft && draftForStudent && draftForStudent.selectedSessionIds.length > 0));
      setSubmissionError(null);
      submissionIdempotencyKey.current = newIdempotencyKey();
      verification.clearStoredToken();
      verification.setCode("");
      setVerificationSatisfied(false);
      setVerificationBlocked(false);
      setMakeupNotice(null);
      setScreen("confirm");
    } catch (error) {
      if (requestId !== lookupRequestId.current) return;
      const notFound = error instanceof ApiRequestError && (error.status === 404 || error.status === 400);
      setLookupError(notFound
        ? "We couldn't find that student ID. Check it and try again."
        : error instanceof Error
          ? error.message
          : "We couldn't find that student ID. Check it and try again.");
    } finally {
      if (requestId === lookupRequestId.current) setLookupLoading(false);
    }
  };

  /** Toggles a whole day. Returns true when the change was accepted. */
  const handleToggleDay = useCallback((group: SubjectSessions, sessionIds: string[]): boolean => {
    const rowKey = sessionIds.join("|");
    setLimitNotice(null);
    setLimitNoticeKey(null);
    const allSelected = sessionIds.every((sessionId) => selectedSessionIds.has(sessionId));
    if (allSelected) {
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
      setSitInPriorityLevels((current) => {
        const next = { ...current };
        for (const sessionId of sessionIds) delete next[sessionId];
        return next;
      });
      setSelectedSubjectIds((current) => {
        const removed = new Set(sessionIds);
        const subjectStillSelected = sessions
          .filter((subject) => subject.subject_id === group.subject_id)
          .flatMap((subject) => subject.sessions)
          .some((session) => selectedSessionIds.has(session.id) && !removed.has(session.id));
        if (subjectStillSelected) return current;
        return current.filter((id) => id !== group.subject_id);
      });
      return true;
    }
    const next = new Set(selectedSessionIds);
    for (const sessionId of sessionIds) next.add(sessionId);
    const scopeKey = absenceScopeKey(group);
    const scopedGroups = scopeIndex.get(scopeKey) ?? [];
    const projectedDays = countSelectedAbsenceDaysForScope(scopedGroups, next, scopeKey);
    const remaining = Math.max(0, ...scopedGroups.map(remainingForGroup));
    if (projectedDays > remaining || projectedDays > config.sit_in.max_sessions_per_absence) {
      const label = group.merge_group_name?.trim() || group.subject_name?.trim() || group.course_name?.trim() || "this course";
      setLimitNotice(`You can't report another absence for ${label}. Please contact Student Services.`);
      setLimitNoticeKey(rowKey);
      return false;
    }
    setSelectedSessionIds(next);
    setSelectedSubjectIds((current) => current.includes(group.subject_id) ? current : [...current, group.subject_id]);
    return true;
  }, [selectedSessionIds, sessions, scopeIndex, remainingForGroup, config.sit_in.max_sessions_per_absence]);

  const handleSeeMoreTimes = useCallback(async () => {
    if (!currentMakeUpDay) return;
    const day = currentMakeUpDay;
    const group = day.group;
    const firstId = day.items[0]?.id;
    if (!firstId) return;
    const sessionGroup = groupWithSitInForMissedSession(group, firstId);
    const currentLevel = sitInPriorityLevels[firstId] ?? sessionGroup.sit_in?.current_priority_level ?? firstPriorityLevel(sessionGroup);
    if (lookup && hasServerPriorityReveal(sessionGroup)) {
      setMakeupLoadingTimes(true);
      try {
        const data: SessionsInRangeResponse = await loadStudentSessions(
          undefined,
          undefined,
          undefined,
          { courseIds: [group.course_id], satVerbalAfterPriority: currentLevel },
        );
        const updatedGroup = data.subjects.find((subject) => subject.course_id === group.course_id);
        if (!updatedGroup) { setPageError("No more make-up times are available for this class."); return; }
        const updatedSessionGroup = groupWithSitInForMissedSession(updatedGroup, firstId);
        const updatedLevel = updatedSessionGroup.sit_in?.current_priority_level
          ?? nextPriorityLevel(sessionGroup, currentLevel)
          ?? currentLevel;
        setSitInPriorityLevels((prev) => ({ ...prev, [firstId]: updatedLevel }));
        setSitInPriorityHistory((prev) => ({ ...prev, [firstId]: { ...(prev[firstId] ?? {}), [updatedLevel]: updatedSessionGroup } }));
      } catch (error) {
        setPageError(error instanceof Error ? error.message : "Couldn't load other make-up times");
      } finally {
        setMakeupLoadingTimes(false);
      }
      return;
    }
    const nextLevel = nextPriorityLevel(sessionGroup, currentLevel);
    if (nextLevel == null) return;
    setSitInPriorityLevels((prev) => ({ ...prev, [firstId]: nextLevel }));
  }, [currentMakeUpDay, lookup, sitInPriorityLevels, sitInPriorityHistory]);

  const handleUseMakeUp = useCallback((value: string) => {
    if (!currentMakeUpDay) return;
    const day = currentMakeUpDay;
    if (makeupPlan.method === "physical") {
      setSitInSelections((current) => {
        const next = { ...current };
        for (const sessionId of day.sessionIds) {
          if (!value) delete next[sessionId];
          else next[sessionId] = value;
        }
        return next;
      });
    }
    setMakeupNotice(null);
    if (makeupIndex + 1 < selectedDays.length) {
      setMakeupIndex((index) => index + 1);
    } else {
      setMakeupIndex(0);
      goToScreen("reason");
    }
  }, [currentMakeUpDay, makeupPlan.method, makeupIndex, selectedDays.length, goToScreen]);

  function validateReason(): boolean {
    setReasonError(null);
    if (config.form.require_reason && !reason.trim()) {
      setReasonError("Choose a reason or tell us why you'll be away.");
      return false;
    }
    return true;
  }

  async function handleSubmitAbsence() {
    if (submittingRef.current) return;
    submittingRef.current = true;
    setSubmissionError(null);
    setPageError(null);
    const verificationExpired = Boolean(verification.token && verification.expiresAt && verification.expiresAt < Date.now());
    if (!verificationSatisfied || verificationBlocked || verificationExpired) {
      setVerificationSatisfied(false);
      setVerificationBlocked(true);
      goToScreen("verify");
      return;
    }
    if (selectedDays.length === 0) { setPageError("Select at least one class you'll miss."); goToScreen("classes"); return; }
    if (missingSitIn) {
      setPageError("Pick a make-up class for all selected classes before submitting.");
      setMakeupIndex(0);
      goToScreen("makeup");
      return;
    }
    if (!validateReason()) {
      goToScreen("reason");
      return;
    }
    if (!lookup) { setPageError("Search for your profile first."); return; }
    try {
      setIsSubmitting(true);
      let submissionSessions: SubjectSessions[];
      try {
        const latest = await loadStudentSessions();
        submissionSessions = latest.subjects;
        setSessions(submissionSessions);
        const blockedSessionIds = blockedSitInSessionIds(submissionSessions);
        const selectedSitInSessionIds = new Set(
          Object.values(sitInSelections).flatMap((value) => splitMergedSessionValue(value)),
        );
        const staleSessionIds = [...selectedSitInSessionIds].filter((id) => blockedSessionIds.has(id));
        if (staleSessionIds.length > 0) {
          draftRef.current = null;
          setSitInSelections({});
          setSitInPriorityLevels({});
          setSitInPriorityHistory({});
          setMakeupIndex(0);
          setMakeupNotice("That class is no longer available. We've found the next available option.");
          goToScreen("makeup");
          return;
        }
      } catch (error) {
        if (isStudentSessionUnauthorized(error)) {
          handleStudentSessionExpired();
          return;
        }
        setSubmissionError(error instanceof Error ? error.message : "Couldn't refresh available make-up classes. Please try again.");
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
      let response: Awaited<ReturnType<typeof submitAbsenceBatch>>;
      try {
        response = await submitAbsenceBatch({
          idempotencyKey: submissionIdempotencyKey.current,
          email: collectedEmail.trim() || undefined,
          reason: reason.trim(),
          items: payloads,
        });
      } catch (error) {
        if (error instanceof ApiRequestError && error.code === "bad_nickname") {
          // Never expected in the redesigned flow (nickname collection was
          // removed), but a stale nickname on file must never block a report.
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
      if (error instanceof ApiRequestError && error.code === "sit_in_session_already_used") {
        draftRef.current = null;
        setSitInSelections({});
        setSitInPriorityLevels({});
        setSitInPriorityHistory({});
        setSessionsReloadToken((current) => current + 1);
        setMakeupIndex(0);
        setMakeupNotice("That class is no longer available. We've found the next available option.");
        goToScreen("makeup");
      } else if (error instanceof ApiRequestError && error.code === "absence_limit_exceeded") {
        setSubmissionError("One of your classes has reached the absence limit. Go back and remove it, or contact Student Services if you need help.");
      } else if (error instanceof TypeError) {
        setSubmissionError("We couldn't confirm the submission. Your connection was interrupted. It's safe to try again — you won't create a duplicate absence.");
      } else {
        setSubmissionError(error instanceof Error ? error.message : "Could not submit your absence");
      }
    } finally {
      setIsSubmitting(false);
      submittingRef.current = false;
    }
  }

  useEffect(() => {
    if (!pageError && !submissionError) return;
    pageAlertRef.current?.focus();
  }, [pageError, submissionError]);

  useEffect(() => {
    if (!finalResults) return;
    window.requestAnimationFrame(() => {
      const heading = document.getElementById("success-heading");
      heading?.focus();
    });
  }, [finalResults]);

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
  }, [screen]);

  const handleDone = useCallback(() => {
    setLookup(null);
    setStudentProfile(null);
    setSessions([]);
    setSelectedSubjectIds([]);
    setSelectedSessionIds(new Set());
    setSitInSelections({});
    setSitInPriorityLevels({});
    setSitInPriorityHistory({});
    setReason("");
    setReasonError(null);
    setCollectedEmail("");
    setLookupInput("");
    setFinalResults(null);
    setVerificationSatisfied(false);
    setVerificationBlocked(false);
    setConfirmedPulse(false);
    setMakeupNotice(null);
    setDraftNeedsReview(false);
    verification.clearStoredToken();
    verification.setCode("");
    try {
      clearLegacyAbsenceDraft();
      clearStudentResume();
      clearStudentSessionHint();
      clearDraft();
    } catch { }
    submissionIdempotencyKey.current = newIdempotencyKey();
    goToScreen("identify");
  }, [verification.clearStoredToken, verification.setCode, clearDraft, goToScreen]);

  const categoryOptions = useMemo(
    () => (config.form.reason_categories && config.form.reason_categories.length > 0 ? config.form.reason_categories : DEFAULT_REASON_CATEGORIES),
    [config.form.reason_categories],
  );
  const reasonCategory = useMemo(() => {
    return categoryOptions.find((category) => reason.startsWith(`${category.label}:`))
      ? categoryOptions.find((category) => reason.startsWith(`${category.label}:`))!.value
      : categoryOptions.find((category) => reason === category.label)?.value ?? null;
  }, [categoryOptions, reason]);

  const handleReasonCategorySelect = useCallback((value: string | null) => {
    setReasonError(null);
    if (value == null) {
      setReason("");
      return;
    }
    const category = categoryOptions.find((option) => option.value === value);
    if (!category) return;
    const currentDetail = reason.startsWith(`${category.label}:`) ? reason.slice(category.label.length + 1).trim() : "";
    setReason(currentDetail ? `${category.label}: ${currentDetail}` : category.label);
  }, [categoryOptions, reason]);

  const requireDetailFor = useCallback((value: string) => {
    return value === "other";
  }, []);

  const handleReasonDetailChange = useCallback((detail: string) => {
    setReasonError(null);
    const category = categoryOptions.find((option) => option.value === reasonCategory);
    if (!category) {
      setReason(detail);
      return;
    }
    setReason(detail.trim() ? `${category.label}: ${detail}` : category.label);
  }, [categoryOptions, reasonCategory]);

  const handlePrimaryAction = () => {
    if (screen === "classes") {
      if (showDoneOnClasses) {
        handleDone();
        return;
      }
      if (selectedDays.length === 0) { setPageError("Select at least one class you'll miss."); return; }
      setMakeupIndex(0);
      setMakeupNotice(null);
      goToScreen("makeup");
    } else if (screen === "reason") {
      if (!validateReason()) return;
      goToScreen("review");
    } else if (screen === "review") {
      void handleSubmitAbsence();
    } else if (screen === "verify" && verificationSatisfied && !verificationBlocked) {
      advanceAfterVerification();
    }
  };

  const handleBack = () => {
    if (screen === "review") goToScreen("reason");
    else if (screen === "reason" || screen === "makeup") goToScreen("classes");
    else if (screen === "classes") goToScreen(emailRequired && !emailSatisfied ? "email" : "verify");
    else if (screen === "email") goToScreen("verify");
    else if (screen === "verify") goToScreen("confirm");
    else if (screen === "confirm") {
      setLookup(null);
      // Keep the ID so a mistaken identity costs one keystroke, not a retype.
      setIdentifySelectOnMount(true);
      goToScreen("identify");
    }
  };

  if (configLoading) {
    return (
      <AbsenceAppShell
        header={<AbsenceHeader progress={0.04} progressLabel="Loading" />}
        footer={<AbsenceActionBar showBack={false} showPrimary={false} canProceed={false} onBack={() => {}} onPrimary={() => {}} primaryLabel="" />}
      >
        <div className="mx-auto w-full max-w-xl py-6">
          <LoadingSkeleton type="text" lines={3} />
        </div>
      </AbsenceAppShell>
    );
  }

  if (finalResults) {
    const submittedGroups = groupSubmittedAbsences(finalResults, sessions);
    const successGroups: SuccessGroup[] = submittedGroups.map((group) => {
      const dates = uniqueValues(group.absences.flatMap((absence) => getAbsenceSessionDateLabels(absence)));
      return {
        key: group.key,
        label: group.label,
        absence: dates.length > 0 ? dates.join(", ") : "Submitted",
        makeup: formatSubmittedSitInSummary(group),
      };
    });
    const referenceId = finalResults[0]?.id?.slice(0, 8).toUpperCase() || "";
    return (
      <AbsenceAppShell
        header={<AbsenceHeader progress={1} progressLabel="Absence submitted" />}
        footer={<AbsenceActionBar showBack={false} showPrimary={false} canProceed={false} onBack={() => {}} onPrimary={() => {}} primaryLabel="" />}
      >
        <SuccessScreen groups={successGroups} reference={referenceId} onDone={handleDone} />
      </AbsenceAppShell>
    );
  }

  const canProceedFromClasses = selectedSessionIds.size > 0 && !sessionsLoading && !draftNeedsReview;
  const canProceedFromReason = !config.form.require_reason || Boolean(reason.trim());
  // With nothing reportable, the only meaningful action is leaving the flow.
  const noReportableClasses = sessions.length === 0
    || sessions.every((group) => group.sessions.every((session) => session.already_absent));
  const showDoneOnClasses = screen === "classes" && !sessionsLoading && !sessionsError && noReportableClasses;

  const reviewSections: ReviewSection[] = [
    {
      key: "classes",
      title: "Classes",
      lines: selectedDays.map((day) => {
        const sitInLabel = getReviewSitInLabel(
          day.items[0],
          day.group,
          sitInSelections,
          sitInPriorityLevels,
          sitInPriorityHistory,
          sessions,
        );
        return `${classLabel(day.group)} — ${selectedDayWhen(day)} — Make-up: ${sitInLabel}`;
      }),
      onEdit: () => goToScreen("classes"),
    },
    {
      key: "reason",
      title: "Reason",
      lines: [reason.trim() || "No reason provided"],
      onEdit: () => goToScreen("reason"),
    },
  ];

  const showFooterPrimary = screen === "classes" || screen === "reason" || screen === "review"
    || (screen === "verify" && verificationSatisfied && !verificationBlocked);

  const footerPrimaryLabel =
    screen === "review" ? "Submit absence"
      : showDoneOnClasses ? "Done"
        : "Continue";

  return (
    <AbsenceAppShell
      header={
        <AbsenceHeader
          onBack={screen !== "identify" && screen !== "resume" ? handleBack : undefined}
          progress={SCREEN_PROGRESS[screen]}
          progressLabel={`Report absence — ${SCREEN_LABELS[screen]}`}
        />
      }
      footer={
        <AbsenceActionBar
          showBack={false}
          showPrimary={showFooterPrimary}
          canProceed={screen === "classes" ? (showDoneOnClasses ? true : canProceedFromClasses) : screen === "reason" ? canProceedFromReason : screen === "review" ? !isSubmitting : true}
          loading={isSubmitting}
          loadingLabel="Submitting…"
          onBack={handleBack}
          onPrimary={handlePrimaryAction}
          primaryLabel={footerPrimaryLabel}
        />
      }
    >
      <div
        className="mx-auto w-full max-w-3xl py-6"
        inert={isSubmitting && !finalResults ? true : undefined}
      >
        {pageError ? <FormAlert alertRef={pageAlertRef} message={pageError} /> : null}

        <p aria-live="polite" className="sr-only">
          {SCREEN_LABELS[screen]}
        </p>

        <motion.div
          key={screen}
          initial={reduceMotion ? false : { opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={reduceMotion ? { duration: 0 } : { duration: 0.18, ease: "easeOut" }}
        >
          {screen === "identify" && (
            <IdentifyScreen
              value={lookupInput}
              onChange={(value) => {
                setLookupInput(value);
                if (lookupError) setLookupError(null);
              }}
              onSubmit={() => void handleLookup()}
              loading={lookupLoading}
              error={lookupError}
              canContinue={isWCode(normalizeLookupWcode(lookupInput))}
              selectOnMount={identifySelectOnMount}
            />
          )}

          {screen === "resume" && lookup && (
            <ResumeScreen
              startedAt={draftRef.current?.updatedAt}
              summary={resumeSummary}
              onContinue={() => goToScreen("confirm")}
              onStartOver={handleDone}
            />
          )}

          {screen === "confirm" && lookup && (
            <ConfirmStudentScreen
              nameHint={lookup.nickname_hint}
              wcode={lookup.wcode}
              onYes={() => goToScreen("verify")}
              onNo={() => {
                setLookup(null);
                // Keep the ID selected so the student can overwrite it in one
                // keystroke instead of retyping from scratch.
                setIdentifySelectOnMount(true);
                setLookupError(null);
                goToScreen("identify");
              }}
            />
          )}

          {screen === "verify" && lookup && (
            <ParentConfirmScreen
              studentName={studentDisplayName}
              wcode={lookup.wcode}
              lookupToken={lookup.lookup_token}
              hasPhone={lookup.parent_verification_available}
              phoneHint={lookup.parent_phone_hint}
              smsParentEnabled={config.notifications?.sms_parent_enabled ?? true}
              adminContact={config.admin_contact}
              verification={verification}
              completed={confirmedPulse || verificationSatisfied}
              blocked={verificationBlocked}
              onSatisfied={handleVerificationSatisfied}
              onRestart={handleVerificationRestart}
              onRestored={handleVerificationRestored}
            />
          )}

          {screen === "email" && lookup && (
            <EmailScreen
              value={collectedEmail}
              onChange={setCollectedEmail}
              onSubmit={() => { if (emailSatisfied) goToScreen("classes"); }}
              canContinue={emailSatisfied}
            />
          )}

          {screen === "classes" && lookup && (
            <ScheduleScreen
              groups={sessions}
              selectedIds={selectedSessionIds}
              onToggleDay={handleToggleDay}
              sitInSelections={sitInSelections}
              onLimitTap={(group, rowKey) => {
                const label = group.merge_group_name?.trim() || group.subject_name?.trim() || group.course_name?.trim() || "this course";
                setLimitNotice(`You can't report another absence for ${label}. Please contact Student Services.`);
                setLimitNoticeKey(rowKey);
              }}
              limitNotice={limitNotice}
              limitNoticeKey={limitNoticeKey}
              loading={sessionsLoading}
              error={sessionsError}
              onRetry={() => setSessionsReloadToken((token) => token + 1)}
              draftNeedsReview={draftNeedsReview}
              onDismissDraftNotice={() => setDraftNeedsReview(false)}
            />
          )}

          {screen === "makeup" && currentMakeUpDay && (
            <MakeUpScreen
              index={makeupIndex}
              total={selectedDays.length}
              missedName={classLabel(currentMakeUpDay.group)}
              missedWhen={selectedDayWhen(currentMakeUpDay)}
              method={makeupPlan.method}
              options={makeupPlan.options}
              selectedValue={sitInSelections[currentMakeUpDay.items[0]?.id ?? ""] ?? ""}
              hasMoreTimes={makeupPlan.hasMoreTimes}
              loadingTimes={makeupLoadingTimes}
              notice={makeupNotice}
              zoomDescription={config.sit_in.zoom_description}
              onUse={handleUseMakeUp}
              onSeeMoreTimes={() => void handleSeeMoreTimes()}
            />
          )}

          {screen === "reason" && (
            <ReasonScreen
              categories={categoryOptions}
              selected={reasonCategory}
              detail={reasonCategory ? reason.split(": ").slice(1).join(": ") : reason}
              requireDetailFor={requireDetailFor}
              allowFreeText={config.form.allow_free_text_reason}
              required={config.form.require_reason}
              onSelect={handleReasonCategorySelect}
              onDetailChange={handleReasonDetailChange}
              error={reasonError}
            />
          )}

          {screen === "review" && lookup && (
            <ReviewScreen
              studentName={studentDisplayName}
              wcode={lookup.wcode}
              sections={reviewSections}
              notice={submissionError}
            />
          )}
        </motion.div>
      </div>
    </AbsenceAppShell>
  );
}