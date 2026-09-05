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
import ScheduleScreen from "@/components/absences/public-form/ScheduleScreen";
import MakeUpScreen, { type MakeUpOption } from "@/components/absences/public-form/MakeUpScreen";
import ReasonScreen from "@/components/absences/public-form/ReasonScreen";
import ReviewScreen, { type ReviewSection } from "@/components/absences/public-form/ReviewScreen";
import SuccessScreen, { type SuccessGroup } from "@/components/absences/public-form/SuccessScreen";
import { useToast } from "@/hooks/useToast";
import { useConnectivity } from "@/hooks/useConnectivity";
import { useAbsenceDraft } from "@/features/absences/hooks/useAbsenceDraft";
import { readAbsenceDraft } from "@/features/absences/storage/absenceDraftStorage";
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
  findChosenSitInOverlaps,
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
import { daysBetween } from "@/features/absences/domain/dateRange";

type Screen =
  | "identify"
  | "resume"
  | "confirm"
  | "verify"
  | "classes"
  | "makeup"
  | "reason"
  | "review";

const SCREEN_ORDER: Screen[] = ["identify", "resume", "confirm", "verify", "classes", "makeup", "reason", "review"];

const SCREEN_PROGRESS: Record<Screen, number> = {
  identify: 0.05,
  resume: 0.08,
  confirm: 0.12,
  verify: 0.3,
  classes: 0.6,
  makeup: 0.8,
  reason: 0.9,
  review: 0.95,
};

const SCREEN_LABELS: Record<Screen, string> = {
  identify: "Identify",
  resume: "Resume your report",
  confirm: "Confirm your profile",
  verify: "Parent confirmation",
  classes: "Choose your classes",
  makeup: "Make-up",
  reason: "Details",
  review: "Review",
};

// The five stages the student moves through. Identity, resume, confirm and
// the parent check all belong to the first stage; the receipt is the outcome.
const SCREEN_STAGE: Partial<Record<Screen, { step: number; name: string }>> = {
  identify: { step: 1, name: "Student" },
  resume: { step: 1, name: "Student" },
  confirm: { step: 1, name: "Student" },
  verify: { step: 1, name: "Student" },
  classes: { step: 2, name: "Classes" },
  makeup: { step: 3, name: "Make-up" },
  reason: { step: 4, name: "Details" },
  review: { step: 5, name: "Review" },
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

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

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

/** Joins a small list of names into a sentence: "A", "A and B", "A, B, and C". */
function joinWithAnd(names: string[]): string {
  if (names.length <= 1) return names[0] ?? "";
  if (names.length === 2) return `${names[0]} and ${names[1]}`;
  return `${names.slice(0, -1).join(", ")}, and ${names[names.length - 1]}`;
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
  const { online } = useConnectivity();
  const verification = useOtp(VERIFICATION_STORAGE_KEY);
  const reduceMotion = useReducedMotion();
  // `restoreDraft` is the draft awaiting restore; while it exists the hook
  // suspends auto-save so the stored snapshot cannot be clobbered by the
  // (cleared) in-form state that precedes the restore.
  const { draft, saveDraft, clearDraft, restoreDraft, beginRestore } = useAbsenceDraft();
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
  // Where to return after a mid-flow expiry bounce to verify (e.g. Review → verify → Review).
  const returnAfterVerifyRef = useRef<Screen | null>(null);
  // Track the lookup + reload scope that already produced the schedule, so
  // the agenda is loaded once verification succeeds and opening Classes does
  // not wipe to a loading skeleton or double-request. A fresh wcode or an
  // explicit retry still refetches.
  const loadedScheduleRef = useRef<{ wcode: string; token: number } | null>(null);

  const studentDisplayName = studentProfile?.display_name || lookup?.nickname_hint || "Student";

  const resumeSummary = useMemo(() => {
    const draft = restoreDraft;
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

  const [makeupFocusKey, setMakeupFocusKey] = useState<string | null>(null);
  // Session keys whose previously accepted make-up time became unavailable;
  // only those rows ask for a new choice — everything else is untouched.
  const [staleMarkedIds, setStaleMarkedIds] = useState<Set<string>>(new Set());
  // An edit launched from Review returns the student straight to Review once
  // the affected stage is settled — no re-walking the forward stages.
  const reviewEditRef = useRef(false);
  // Announced once when a review edit lands back on Review (rendered through
  // the screen's role=alert notice slot). Cleared on the next edit-out so a
  // stale "updated" note can never greet a fresh visit.
  const [reviewReturnNote, setReviewReturnNote] = useState<string | null>(null);
  // Focus the update-email field when an "Edit email" jump opens Details.
  const [focusReasonEmail, setFocusReasonEmail] = useState(false);

  const makeupPlanEntries = useMemo(
    () => selectedDays.map((day) => ({
      day,
      plan: makeUpPlan(day, sessions, selectedSubjectIds, sitInSelections, sitInPriorityLevels, sitInPriorityHistory),
    })),
    [selectedDays, sessions, selectedSubjectIds, sitInSelections, sitInPriorityLevels, sitInPriorityHistory],
  );

  const missingSitIn = useMemo(
    () => makeupPlanEntries.some(({ day, plan }) => missingMakeUp(day, plan, sitInSelections)),
    [makeupPlanEntries, sitInSelections],
  );

  // Client-side overlap validation: a chosen make-up must not overlap a class
  // the student still attends, nor another chosen make-up. Each affected row
  // is named and must be changed before the plan can be completed.
  const chosenSitInOverlaps = useMemo(() => {
    const rows = makeupPlanEntries.flatMap(({ day }) => {
      const key = day.items[0]?.id;
      const value = key ? sitInSelections[key] : undefined;
      if (!key || !value) return [];
      return [{ sessionKey: key, missedSessionId: key, group: day.group, value }];
    });
    return findChosenSitInOverlaps(rows, sessions, selectedSessionIds);
  }, [makeupPlanEntries, sessions, selectedSessionIds, sitInSelections]);
  const sitInOverlapMessages = useMemo(
    () => new Map(chosenSitInOverlaps.map((overlap) => [overlap.sessionKey, overlap.message])),
    [chosenSitInOverlaps],
  );
  const hasSitInOverlap = chosenSitInOverlaps.length > 0;

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
    loadedScheduleRef.current = null;
    setVerificationSatisfied(false);
    setVerificationBlocked(true);
    setConfirmedPulse(false);
    // Remember where the student was so re-verify can return them there.
    if (screenRef.current !== "verify" && screenRef.current !== "identify" && screenRef.current !== "confirm") {
      returnAfterVerifyRef.current = screenRef.current;
    }
    setPageError("Your verified session expired. Confirm with your parent again to continue. Your absence details are still saved.");
    setSubmissionError(null);
    setScreen("verify");
  }, [verification.clearStoredToken, verification.setCode]);

  useEffect(() => {
    if (!lookup) return;
    // Load once the student is verified (they are about to open Classes), and
    // refresh when Classes is entered with nothing loaded yet or a retry.
    const mayFetch = screen === "classes" || (screen === "verify" && verificationSatisfied);
    if (!mayFetch) return;
    // One attempt per lookup + reload scope: opening Classes right after the
    // prefetch (or after a failed load) must not fire a duplicate request that
    // clears the error or skips the student's own retry.
    const previous = loadedScheduleRef.current;
    if (previous && previous.wcode === lookup.wcode && previous.token === sessionsReloadToken) return;
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
        loadedScheduleRef.current = { wcode: lookup.wcode, token: sessionsReloadToken };
        const validSubjectIds = new Set(profile.subjects.map((subject) => subject.id));
        setSelectedSubjectIds((current) => current.filter((id) => validSubjectIds.has(id)));
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
        loadedScheduleRef.current = { wcode: lookup.wcode, token: sessionsReloadToken };
      })
      .finally(() => { if (!controller.signal.aborted) setSessionsLoading(false); });
    return () => controller.abort();
    // sessions is deliberately read (not a dep): the guard dedupes by reload
    // token, and listing sessions here would re-run the fetch on every change.
  }, [screen, lookup, verificationSatisfied, sessionsReloadToken, handleStudentSessionExpired]);

  // Apply a saved draft only once the student is actually on Classes with the
  // schedule loaded — never during the pre-verification resume decision, so
  // saved content is not revealed (or silently re-applied) before the student
  // confirms who they are and a parent verifies them.
  useEffect(() => {
    const draft = restoreDraft;
    if (screen !== "classes" || !lookup || !draft) return;
    if (sessionsLoading || sessionsError || sessions.length === 0) return;
    if (!studentProfile || studentProfile.wcode !== lookup.wcode) return;
    if (normalizeLookupWcode(draft.wcode) !== lookup.wcode) return;
    const validSubjectIds = new Set(studentProfile.subjects.map((subject) => subject.id));
    const validSessionIds = new Set(sessions.flatMap((group) => group.sessions.map((session) => session.id)));
    const restoredSubjectIds = draft.selectedSubjectIds.filter((subjectId) => validSubjectIds.has(subjectId));
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
    setSelectedSubjectIds(restoredSubjectIds);
    setSelectedSessionIds(restoredSessionSet);
    setSitInSelections(restoredSitIns);
    setSitInPriorityLevels(restoredPriorityLevels);
    setDraftNeedsReview((current) => current || missingSavedSessions > 0 || restoredSessionIds.length > 0);
    // Restore consumed: allow auto-save again so the in-form state is what
    // gets persisted from here on.
    beginRestore(null);
  }, [screen, lookup, restoreDraft, studentProfile, sessions, sessionsLoading, sessionsError, beginRestore]);

  useEffect(() => {
    let active = true;
    try {
      clearLegacyAbsenceDraft();
      const resume = readStudentResume();
      const draft = restoreDraft;
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
          // A returning student confirms identity and passes the parent check
          // before any saved-report details are revealed. The choice to resume
          // or start over is offered right after verification.
          setScreen("confirm");
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
    // The hook itself refuses to persist while a restore snapshot is pending.
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

  // The verified session lapsed while the student was past the verify screen.
  // Remember exactly where they were so re-verification returns them there —
  // their selections and the screen they were on are all preserved.
  const bounceToVerifyAfterExpiry = useCallback(() => {
    const current = screenRef.current;
    setVerificationBlocked(true);
    setVerificationSatisfied(false);
    if (current !== "verify" && current !== "identify" && current !== "confirm") {
      returnAfterVerifyRef.current = current;
    }
    setPageError("Your verified session expired. Confirm with your parent again to continue. Your absence details are still saved.");
    setSubmissionError(null);
    setScreen((currentScreen) =>
      SCREEN_ORDER.indexOf(currentScreen) >= SCREEN_ORDER.indexOf("verify") ? "verify" : currentScreen,
    );
  }, []);

  useEffect(() => {
    if (!verification.token) {
      setVerificationBlocked(false);
      return;
    }
    const expiry = verification.expiresAt;
    if (expiry && expiry < Date.now()) {
      bounceToVerifyAfterExpiry();
      return;
    }
    setVerificationBlocked(false);
  }, [verification, bounceToVerifyAfterExpiry]);

  useEffect(() => {
    if (!verification.token || !verification.expiresAt) return;
    const enforceExpiry = () => {
      if (!(verification.expiresAt && verification.expiresAt <= Date.now())) return;
      // On the verify screen there is nothing to lose: bounce immediately so
      // the code form (not a stale Confirmed) is showing. Past verify, warn
      // in place instead of yanking context mid-task (WCAG 2.2.1/3.2.1) — the
      // submit gate and the resume gate already refuse unverified progress,
      // and the next Continue routes through re-verify with the return
      // destination preserved.
      if (screenRef.current === "verify") {
        bounceToVerifyAfterExpiry();
        return;
      }
      setVerificationBlocked(true);
      setVerificationSatisfied(false);
      setConfirmedPulse(false);
      if (screenRef.current !== "identify" && screenRef.current !== "confirm") {
        returnAfterVerifyRef.current = screenRef.current;
      }
      setPageError("Your verified session expired. Confirm with your parent again to continue. Your absence details are still saved.");
      setSubmissionError(null);
    };
    enforceExpiry();
    // One-shot is honest here: a stored OTP token only exists in the
    // pre-verify code-entry window (handleVerify clears it on success and
    // the verified parent session is enforced server-side at submit).
    // Past verify there is nothing to count down — the submit gate and the
    // primary-action gate refuse unverified progress, and the server is the
    // source of truth for session lapse.
    const timer = window.setTimeout(enforceExpiry, Math.max(0, verification.expiresAt - Date.now()));
    return () => window.clearTimeout(timer);
  }, [verification.expiresAt, verification.token, bounceToVerifyAfterExpiry]);

  const advanceAfterVerification = useCallback(() => {
    const pending = returnAfterVerifyRef.current;
    if (pending && pending !== "verify" && pending !== "identify" && pending !== "confirm") {
      returnAfterVerifyRef.current = null;
      setScreen(pending);
      return;
    }
    returnAfterVerifyRef.current = null;
    if (restoreDraft) {
      // A saved report is waiting: the student decides here — after identity
      // and parent verification — whether to resume it or start over. Saved
      // details are only revealed once they are verified.
      setScreen("resume");
      return;
    }
    // Required email is collected in Details, after classes and make-up.
    setScreen("classes");
    // restoreDraft is read deliberately: the advance runs once per verified
    // session and must decide on the snapshot that exists at that moment.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [restoreDraft]);

  const handleVerificationSatisfied = useCallback(() => {
    setVerificationSatisfied(true);
    setVerificationBlocked(false);
    // The Confirmed panel stays until the student taps Continue: a timed
    // advance would yank context from screen-reader and slow users (WCAG 3.2.1
    // — no change of context on input without the user's request).
    setConfirmedPulse(true);
  }, []);

  // The Confirmed panel marks a *fresh* satisfaction. Back-navigation lands on
  // verify already satisfied (the session is still valid), which must show
  // the re-enterable code form — never a dead-end Confirmed with no way back.
  useEffect(() => {
    if (screen === "verify" && !verificationSatisfied) setConfirmedPulse(false);
  }, [screen, verificationSatisfied]);

  const handleVerificationRestart = useCallback(() => {
    verification.clearStoredToken();
    verification.setCode("");
    setVerificationSatisfied(false);
    setVerificationBlocked(false);
    setConfirmedPulse(false);
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
    // Scroll reset lives in the screen-change effect, which also covers
    // direct setScreen calls (expiry bounces, auto-advance) — not here.
    setScreen(next);
  }, []);

  const handleLookup = async () => {
    const requestId = ++lookupRequestId.current;
    const cleaned = normalizeLookupWcode(lookupInput);
    setLookupError(null);
    clearStudentSessionHint();
    setLookup(null);
    setStudentProfile(null);
    if (!cleaned || !isWCode(cleaned)) return;
    // Consult the live draft (sessionStorage stays current thanks to auto-save,
    // and untouched while a restore snapshot is pending). Re-identifying the
    // same Student ID must resume the saved report, not discard it — and
    // discarding a report (Start over / a different ID) must stay discarded.
    const draftForStudent = restoreDraft ?? readAbsenceDraft();
    const shouldRestoreDraft = Boolean(
      draftForStudent && normalizeLookupWcode(draftForStudent.wcode) === cleaned,
    );
    beginRestore(shouldRestoreDraft ? draftForStudent : null);
    if (!shouldRestoreDraft) clearDraft();
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
      loadedScheduleRef.current = null;
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
    // A scope's whole selection becomes one absence server-side, so a
    // selection that would stretch past the configured maximum span must be
    // explained here — not discovered at submission.
    const maxSpanDays = config.form.max_date_range_days;
    const scopedDateKeys = new Set<string>();
    for (const scoped of scopedGroups) {
      for (const dayGroup of groupByDay(scoped.sessions)) {
        const activeItems = dayGroup.items.filter((session) => !session.already_absent);
        if (activeItems.length > 0 && activeItems.every((session) => next.has(session.id))) {
          scopedDateKeys.add(dayGroup.date);
        }
      }
    }
    const sortedDateKeys = [...scopedDateKeys].sort();
    if (sortedDateKeys.length >= 2 && daysBetween(sortedDateKeys[0], sortedDateKeys[sortedDateKeys.length - 1]) > maxSpanDays) {
      const label = group.merge_group_name?.trim() || group.subject_name?.trim() || group.course_name?.trim() || "this course";
      setLimitNotice(`This class is further than ${maxSpanDays} days from your other ${label} classes in this report. Remove a class in between or report the later dates as a separate absence.`);
      setLimitNoticeKey(rowKey);
      return false;
    }
    setSelectedSessionIds(next);
    setSelectedSubjectIds((current) => current.includes(group.subject_id) ? current : [...current, group.subject_id]);
    return true;
  }, [selectedSessionIds, sessions, scopeIndex, remainingForGroup, config.sit_in.max_sessions_per_absence, config.form.max_date_range_days]);

  const handleSeeMoreTimes = useCallback(async (sessionKey: string) => {
    const day = selectedDays.find((candidate) => candidate.items[0]?.id === sessionKey);
    if (!day) return;
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
  }, [selectedDays, lookup, sitInPriorityLevels, sitInPriorityHistory]);

  /** Records a make-up choice for one missed class-day, keyed by its session. */
  const handleUseMakeUpTime = useCallback((sessionKey: string, value: string) => {
    const day = selectedDays.find((candidate) => candidate.items[0]?.id === sessionKey);
    if (!day) return;
    setSitInSelections((current) => {
      const next = { ...current };
      for (const sessionId of day.sessionIds) {
        if (!value) delete next[sessionId];
        else next[sessionId] = value;
      }
      return next;
    });
    setStaleMarkedIds((current) => {
      if (!current.has(sessionKey)) return current;
      const next = new Set(current);
      next.delete(sessionKey);
      return next;
    });
    setMakeupNotice(null);
  }, [selectedDays]);

  /** Clears only the make-up choices that are no longer available, leaving
   *  every unaffected choice intact. Returns true when something was removed. */
  const invalidateUnavailableSitIns = useCallback((freshSessions: SubjectSessions[]): boolean => {
    const blocked = blockedSitInSessionIds(freshSessions);
    const affected: string[] = [];
    for (const [missedId, value] of Object.entries(sitInSelections)) {
      if (value && splitMergedSessionValue(value).some((id) => blocked.has(id))) affected.push(missedId);
    }
    if (affected.length === 0) return false;
    const affectedSet = new Set(affected);
    const nextSelections = { ...sitInSelections };
    for (const id of affected) delete nextSelections[id];
    const nextLevels = { ...sitInPriorityLevels };
    for (const id of affected) delete nextLevels[id];
    const nextHistory = { ...sitInPriorityHistory };
    for (const id of affected) delete nextHistory[id];
    setSitInSelections(nextSelections);
    setSitInPriorityLevels(nextLevels);
    setSitInPriorityHistory(nextHistory);
    setStaleMarkedIds(affectedSet);
    setMakeupFocusKey(affected[0] ?? null);
    // Name each affected class-day so the student knows exactly what changed;
    // everything else in the plan is untouched. A class-day that no longer
    // exists in the fresh schedule has no row to re-choose, so it is skipped.
    const affectedDays = selectedDays.filter((day) =>
      day.sessionIds.some((sessionId) => affectedSet.has(sessionId)),
    );
    const names = affectedDays.map((day) => `${classLabel(day.group)} · ${formatDate(day.date)}`);
    const itemizedNotice = names.length <= 1
      ? `The make-up time you chose for ${names[0] ?? "a class you'll miss"} is no longer available — choose another time for it. Nothing else in your plan changed.`
      : `The make-up times you chose for ${joinWithAnd(names)} are no longer available — choose another time for each. Nothing else in your plan changed.`;
    setMakeupNotice(itemizedNotice);
    return true;
  }, [sitInSelections, sitInPriorityLevels, sitInPriorityHistory, selectedDays]);

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
      if (screenRef.current !== "verify") returnAfterVerifyRef.current = screenRef.current;
      goToScreen("verify");
      return;
    }
    if (selectedDays.length === 0) { setPageError("Select at least one class you'll miss."); goToScreen("classes"); return; }
    if (missingSitIn) {
      setPageError("Choose a make-up time for every class that needs one before submitting.");
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
          beginRestore(null);
          invalidateUnavailableSitIns(submissionSessions);
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
      // clearDraft below also releases any pending restore snapshot.
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
        beginRestore(null);
        try {
          const latest = await loadStudentSessions();
          setSessions(latest.subjects);
          invalidateUnavailableSitIns(latest.subjects);
        } catch {
          setMakeupNotice("A make-up time you chose could not be booked. Review your plan and try again.");
        }
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

  // Reason failures surface on the group (the footer owns the primary, so a
  // keyboard user never meets the inline error otherwise): move focus to it.
  // Fires on fresh failures and after submit-time revalidation navigates here.
  useEffect(() => {
    if (!reasonError) return;
    window.requestAnimationFrame(() => {
      document.getElementById("absence-reason-error")?.focus();
    });
  }, [reasonError, screen]);

  useEffect(() => {
    if (!finalResults) return;
    window.requestAnimationFrame(() => {
      const main = document.getElementById("absence-form-content");
      if (main && main.scrollTop !== 0) main.scrollTo({ top: 0, behavior: "auto" });
      const heading = document.getElementById("success-heading");
      heading?.focus();
    });
  }, [finalResults]);

  const hasFocusedInitiallyRef = useRef(false);
  useEffect(() => {
    // The shell's main is the real scroll container (the window never scrolls),
    // so every screen change must reset it — otherwise a long screen (classes)
    // followed by a short one (reason) lands the student mid-screen.
    window.requestAnimationFrame(() => {
      const main = document.getElementById("absence-form-content");
      if (!main) return;
      if (main.scrollTop !== 0) main.scrollTo({ top: 0, behavior: "auto" });
      if (!hasFocusedInitiallyRef.current) {
        hasFocusedInitiallyRef.current = true;
        return;
      }
      const active = document.activeElement;
      if (active instanceof HTMLElement && active !== document.body && main.contains(active)) return;
      main.focus();
    });
  }, [screen]);

  const handleDone = useCallback(() => {
    setLookup(null);
    setStudentProfile(null);
    setSessions([]);
    loadedScheduleRef.current = null;
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
    // Starting over discards the draft for good — a later lookup of the same
    // Student ID must not resurrect it (clearDraft below releases the snapshot).
    goToScreen("identify");
  }, [verification.clearStoredToken, verification.setCode, clearDraft, goToScreen]);

  const categoryOptions = useMemo(
    () => (config.form.reason_categories && config.form.reason_categories.length > 0 ? config.form.reason_categories : DEFAULT_REASON_CATEGORIES),
    [config.form.reason_categories],
  );
  const reasonCategory = useMemo(() => {
    // Split on the first ": " so a detail containing colons or another
    // label's text can't re-parse to the wrong category on revisit.
    const head = reason.includes(": ") ? reason.slice(0, reason.indexOf(": ")) : reason;
    return categoryOptions.find((category) => category.label === head)?.value ?? null;
  }, [categoryOptions, reason]);

  const handleReasonCategorySelect = useCallback((value: string | null) => {
    setReasonError(null);
    if (value == null) {
      setReason("");
      return;
    }
    const category = categoryOptions.find((option) => option.value === value);
    if (!category) return;
    const head = reason.includes(": ") ? reason.slice(0, reason.indexOf(": ")) : reason;
    const currentDetail = head === category.label && reason.includes(": ")
      ? reason.slice(reason.indexOf(": ") + 2).trim()
      : "";
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
    // A lapse that landed while working must re-verify before any advance:
    // the timer warns in place, and this gate routes the next Continue
    // through verify with the return destination preserved.
    if (screen !== "verify" && screen !== "review") {
      const lapsed = Boolean(verification.token && verification.expiresAt && verification.expiresAt < Date.now());
      if (lapsed || !verificationSatisfied || verificationBlocked) {
        bounceToVerifyAfterExpiry();
        return;
      }
    }
    if (screen === "classes") {
      if (showDoneOnClasses) {
        handleDone();
        return;
      }
      if (selectedDays.length === 0) { setPageError("Select at least one class you'll miss."); return; }
      setMakeupFocusKey(null);
      setMakeupNotice(null);
      if (reviewEditRef.current) {
        if (missingSitIn) {
          // A changed class needs a new make-up decision: land on the plan
          // focused on the first affected row; every other choice is kept.
          const firstNeedingKey = makeupPlanEntries
            .find(({ day, plan }) => missingMakeUp(day, plan, sitInSelections))
            ?.day.items[0]?.id ?? null;
          setMakeupFocusKey(firstNeedingKey);
          goToScreen("makeup");
          return;
        }
        reviewEditRef.current = false;
        setReviewReturnNote("Your classes are updated — review the changed details below.");
        goToScreen("review");
        return;
      }
      goToScreen("makeup");
    } else if (screen === "makeup") {
      if (missingSitIn) {
        setPageError("Choose a make-up time for every class that needs one before continuing.");
        return;
      }
      if (hasSitInOverlap) {
        setPageError("One of your make-up times overlaps another class you'll attend. Choose another time for it before continuing.");
        return;
      }
      if (reviewEditRef.current) {
        reviewEditRef.current = false;
        setReviewReturnNote("Your make-up choice is updated — review the changed details below.");
        goToScreen("review");
        return;
      }
      goToScreen("reason");
    } else if (screen === "reason") {
      if (!validateReason()) {
        // State update above renders the group error; focus it on the next
        // frame so repeat failures refocus even when the message is unchanged.
        window.requestAnimationFrame(() => {
          document.getElementById("absence-reason-error")?.focus();
        });
        return;
      }
      reviewEditRef.current = false;
      setFocusReasonEmail(false);
      setReviewReturnNote("Your details are updated — review the changed details below.");
      goToScreen("review");
    } else if (screen === "review") {
      void handleSubmitAbsence();
    } else if (screen === "verify" && verificationSatisfied && !verificationBlocked) {
      advanceAfterVerification();
    }
  };

  const handleBack = () => {
    // Back while editing from Review cancels the edit and returns to Review.
    if (reviewEditRef.current && (screen === "classes" || screen === "makeup" || screen === "reason")) {
      reviewEditRef.current = false;
      setFocusReasonEmail(false);
      goToScreen("review");
      return;
    }
    if (screen === "review") goToScreen("reason");
    else if (screen === "reason" || screen === "makeup") goToScreen("classes");
    else if (screen === "classes") goToScreen("verify");
    else if (screen === "verify") goToScreen("confirm");
    // Resume is a post-verify decision, not a forward stage: Back returns to
    // the verify screen it came from (a no-op if accessed any other way).
    else if (screen === "resume") goToScreen("verify");
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
        <div className="mx-auto w-full max-w-2xl py-6">
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
    const receiptStatus = finalResults[0]?.status;
    return (
      <AbsenceAppShell
        header={<AbsenceHeader progress={1} progressLabel="Absence report submitted" />}
        footer={<AbsenceActionBar showBack={false} showPrimary={false} canProceed={false} onBack={() => {}} onPrimary={() => {}} primaryLabel="" />}
      >
        <SuccessScreen groups={successGroups} reference={referenceId} status={receiptStatus} onDone={handleDone} />
      </AbsenceAppShell>
    );
  }

  const canProceedFromClasses = selectedSessionIds.size > 0 && !sessionsLoading && !draftNeedsReview;
  const canProceedFromReason = (!config.form.require_reason || Boolean(reason.trim()))
    && (!emailRequired || emailSatisfied);
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
      onEdit: () => {
        reviewEditRef.current = true;
        setReviewReturnNote(null);
        setMakeupFocusKey(null);
        goToScreen("classes");
      },
      onEditLine: (lineIndex: number) => {
        // Keep the stable identity of the row being changed so the plan
        // opens focused on it (Review content never shifts under the edit).
        setMakeupFocusKey(selectedDays[lineIndex]?.items[0]?.id ?? null);
        reviewEditRef.current = true;
        setReviewReturnNote(null);
        goToScreen("makeup");
      },
      editLineLabel: "Change time",
    },
    {
      key: "reason",
      title: "Reason",
      lines: [reason.trim() || "No reason provided"],
      onEdit: () => {
        reviewEditRef.current = true;
        setReviewReturnNote(null);
        goToScreen("reason");
      },
    },
    ...(studentProfile?.email_on_file || manualEmail ? [
      {
        key: "destination",
        title: "Update destination",
        lines: manualEmail
          ? [`Updates go to ${manualEmail}`]
          : ["Emails go to the address the school has on file."],
        // The school's on-file address can't be changed here; only a manual
        // address collected in Details is editable.
        editLabel: emailRequired ? "Edit email" : undefined,
        onEdit: emailRequired ? () => {
          reviewEditRef.current = true;
          setReviewReturnNote(null);
          setFocusReasonEmail(true);
          goToScreen("reason");
        } : undefined,
      },
    ] : []),
  ];

  // Footer owns the primary on every stage-level screen. Identify/confirm keep
  // their local buttons; the make-up rows use local sheet actions for content,
  // but advancing to Details is a footer decision. Verify/email always show
  // the footer so there is exactly one visible Next — disabled with a hint
  // until it can proceed.
  const showFooterPrimary = screen === "classes" || screen === "makeup" || screen === "reason" || screen === "review"
    || screen === "verify";

  // Standard stages share one calm reading column (~640-672px). Classes alone
  // widens on desktop so the agenda next to the calendar gets the space its
  // two-column layout needs; the shell, notices, and footer all track the
  // active stage's width.
  const contentWidth = screen === "classes" ? "max-w-2xl lg:max-w-5xl" : "max-w-2xl";

  const footerPrimaryLabel =
    screen === "review" ? "Submit absence"
      : showDoneOnClasses ? "Done"
        : "Continue";

  const currentStage = SCREEN_STAGE[screen];
  const stageLabel = currentStage ? `Step ${currentStage.step} of 5 · ${currentStage.name}` : undefined;

  // Honest, quiet "where your work lives" note: drafts are kept in this
  // browser tab only — never a promise of cross-device or forever storage.
  const savedInThisTab = Boolean(
    draft
    && lookup
    && !restoreDraft
    && normalizeLookupWcode(draft.wcode) === lookup.wcode
    && (draft.selectedSessionIds.length > 0 || draft.reason?.trim() || draft.collectedEmail?.trim())
    && (screen === "classes" || screen === "makeup" || screen === "reason" || screen === "review")
  );

  return (
    <AbsenceAppShell
      header={
        <AbsenceHeader
          onBack={screen !== "identify" ? handleBack : undefined}
          progress={SCREEN_PROGRESS[screen]}
          progressLabel={stageLabel ?? `Report absence — ${SCREEN_LABELS[screen]}`}
          stageLabel={stageLabel}
        />
      }
      footer={
        <AbsenceActionBar
          showBack={false}
          showPrimary={showFooterPrimary}
          canProceed={screen === "classes" ? (showDoneOnClasses ? true : canProceedFromClasses) : screen === "makeup" ? (!missingSitIn && !hasSitInOverlap && !makeupLoadingTimes && !sessionsLoading) : screen === "reason" ? canProceedFromReason : screen === "review" ? !isSubmitting : screen === "verify" ? (verificationSatisfied && !verificationBlocked) : true}
          loading={isSubmitting}
          loadingLabel="Submitting…"
          onBack={handleBack}
          onPrimary={handlePrimaryAction}
          primaryLabel={footerPrimaryLabel}
          hint={screen === "verify" && !verificationSatisfied ? "Enter the code from your parent's phone to continue." : screen === "classes" && !showDoneOnClasses && !canProceedFromClasses ? "Choose at least one class day to continue." : screen === "makeup" && missingSitIn ? "Choose a make-up time for every class that needs one." : screen === "makeup" && hasSitInOverlap ? "A make-up time overlaps another class — choose another time." : screen === "reason" && config.form.require_reason && !reason.trim() ? "Choose a reason to continue." : screen === "reason" && emailRequired && !emailSatisfied ? "Add a valid email so we can send updates." : undefined}
        />
      }
    >
      <div
        className={`mx-auto w-full py-6 ${contentWidth}`}
        inert={isSubmitting && !finalResults ? true : undefined}
      >
        {pageError ? <FormAlert alertRef={pageAlertRef} message={pageError} /> : null}

        {savedInThisTab ? (
          <p className="mb-4 text-right text-[13px] font-medium text-[var(--color-wi-text-light)]">
            Saved in this tab
          </p>
        ) : null}

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
              startedAt={restoreDraft?.updatedAt}
              summary={resumeSummary}
              onContinue={() => {
                // The resume choice is only offered after a valid verified
                // session; an expiry that lands here sends the student back
                // to confirm with their parent again — preserving the resume
                // decision itself so re-verify returns here, not to Classes.
                if (verificationBlocked || !verificationSatisfied) {
                  returnAfterVerifyRef.current = "resume";
                  goToScreen("verify");
                  return;
                }
                goToScreen("classes");
              }}
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
              online={online}
              phoneHint={lookup.parent_phone_hint}
              smsParentEnabled={config.notifications?.sms_parent_enabled ?? true}
              adminContact={config.admin_contact}
              verification={verification}
              completed={confirmedPulse || verificationSatisfied}
              blocked={verificationBlocked}
              onSatisfied={handleVerificationSatisfied}
              onRestart={handleVerificationRestart}
              onRestored={handleVerificationRestored}
              onContinue={advanceAfterVerification}
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
              supportHref={config.admin_contact?.email?.trim() ? `mailto:${config.admin_contact.email.trim()}` : undefined}
            />
          )}

          {screen === "makeup" && lookup && (
            <MakeUpScreen
              notice={makeupNotice}
              plans={makeupPlanEntries.map(({ day, plan }) => {
                const sessionKey = day.items[0]?.id ?? "";
                return {
                  sessionKey,
                  label: classLabel(day.group),
                  when: selectedDayWhen(day),
                  method: plan.method,
                  options: plan.options,
                  selectedValue: sitInSelections[sessionKey] ?? "",
                  hasMoreTimes: plan.hasMoreTimes,
                  needsAttention: staleMarkedIds.has(sessionKey) && !sitInSelections[sessionKey],
                  overlapMessage: sitInOverlapMessages.get(sessionKey),
                };
              })}
              focusSessionKey={makeupFocusKey}
              loadingTimes={makeupLoadingTimes}
              zoomDescription={config.sit_in.zoom_description}
              onUseTime={handleUseMakeUpTime}
              onSeeMoreTimes={(sessionKey: string) => void handleSeeMoreTimes(sessionKey)}
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
              email={
                emailRequired
                  ? {
                      required: true,
                      value: manualEmail,
                      onChange: setCollectedEmail,
                      invalid: manualEmail.length > 0 && !manualEmailValid,
                    }
                  : undefined
              }
              initialFocusOnEmail={focusReasonEmail}
            />
          )}

          {screen === "review" && lookup && (
            <ReviewScreen
              studentName={studentDisplayName}
              wcode={lookup.wcode}
              sections={reviewSections}
              notice={submissionError ?? reviewReturnNote}
            />
          )}
        </motion.div>
      </div>
    </AbsenceAppShell>
  );
}
