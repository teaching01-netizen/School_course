import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronRight, ChevronLeft, Info } from "lucide-react";
import { ApiRequestError, apiJson } from "../../api/client";
import {
  loadSessionsInRange,
  lookupStaffStudentByWcode,
} from "../../features/absences/api/absenceFormApi";
import { useToast } from "../../hooks/useToast";
import { formatDate, formatTime } from "../../utils/date";
import {
  combineSubjectGroups,
  combineSubjectPickerEntries,
  absenceScopeKey,
  groupByDay,
  isDayGroupSelected,
  mergedSessionValue,
  getSelectedSessionsForGroup,
  splitMergedSessionValue,
} from "../../features/absences/domain/sessionGrouping";
import {
  sitInForMissedSession,
  groupWithSitInForMissedSession,
  availableSessionsForMissedSessions,
  unavailableSessionsForMissedSession,
  firstPriorityLevel,
  hasServerPriorityReveal,
  nextPriorityLevel,
  previousPriorityLevel,
  prioritiesForLevel,
  rootAvailableSessionsForMissedSessions,
  appendTeacher,
  findSitInSessionConflicts,
  formatSitInSessionConflictDescription,
  formatHistoricalSitInConflictDescription,
  formatSitInSubmissionConflictDetails,
  blockedSitInSessionIds,
  sitInOptionGroupsBySession,
  sitInOptionsByTargetAndSession,
  getSitInSessionGroupLabel,
  getSitInSessionSubjectTimeLabel,
  getReviewSitInLabel,
} from "../../features/absences/domain/sitInResolution";
import MakeUpPicker, {
  type MakeUpOption,
} from "./public-form/MakeUpPicker";
import {
  duplicateSitInSessionIds,
  mergeAbsenceBatchItemsByScope,
  selectedSitInCourseIDForGroup,
} from "../../features/absences/domain/submissionPayload";
import type {
  SubjectSessions,
  StudentLookupResponse,
  AbsenceFormConfig,
  SmsPreview,
  StaffCreateAbsenceRequest,
  StaffCreateAbsenceBatchResponse,
} from "../../types";
import Button from "../ui/Button";
import Select from "../ui/Select";
import Modal from "../Modal";
import SubjectCard from "./SubjectCard";
import SmsConfirmModal from "./SmsConfirmModal";

type ModalStep = "type" | "subjects" | "sessions" | "confirm";

type Props = {
  onClose: () => void;
  onCreated: () => void;
};

type StaffSubjectOption = {
  id: string;
  code: string;
  name: string;
};

type SitInMode = "suggested" | "special";

type SpecialSitInSelection = {
  subjectId: string;
  sessionValue: string;
};

type SpecialSitInAvailableSession = NonNullable<
  NonNullable<SubjectSessions["sit_in"]>["available_sessions"]
>[number];

type SpecialSitInSessionOption = {
  value: string;
  courseId: string;
  label: string;
};

const STEP_KEYS: ModalStep[] = ["type", "subjects", "sessions", "confirm"];

async function loadStaffSessionsForSubjects(
  wcode: string,
  selectedSubjectIds: string[],
  enrolledSubjectIds: Set<string>,
  signal?: AbortSignal,
): Promise<SubjectSessions[]> {
  const enrolledSelectedIds = selectedSubjectIds.filter((subjectId) =>
    enrolledSubjectIds.has(subjectId),
  );
  const specialSelectedIds = selectedSubjectIds.filter(
    (subjectId) => !enrolledSubjectIds.has(subjectId),
  );
  const enrolledRequest =
    enrolledSelectedIds.length > 0
      ? loadSessionsInRange(
          wcode,
          "1970-01-01",
          "2100-01-01",
          signal ? { signal } : undefined,
          { bypassTiming: true },
        )
      : Promise.resolve({ subjects: [] as SubjectSessions[] });
  const specialRequest =
    specialSelectedIds.length > 0
      ? loadSessionsInRange(
          wcode,
          "1970-01-01",
          "2100-01-01",
          signal ? { signal } : undefined,
          {
            bypassTiming: true,
            includeAllSubjects: true,
            subjectIds: specialSelectedIds,
          },
        )
      : Promise.resolve({ subjects: [] as SubjectSessions[] });
  const [enrolledData, specialData] = await Promise.all([
    enrolledRequest,
    specialRequest,
  ]);
  return [...(enrolledData.subjects ?? []), ...(specialData.subjects ?? [])];
}

function specialSitInSessionsForGroup(
  group: SubjectSessions,
): SpecialSitInAvailableSession[] {
  if (group.sit_in) return group.sit_in.available_sessions ?? [];
  return group.sessions
    .filter((session) => !session.already_absent)
    .map((session) => ({
      ...session,
      course_id: group.course_id,
      course_code: group.course_code,
      course_name: group.course_name,
      subject_code: group.subject_code,
      subject_name: group.subject_name,
    }));
}

function buildSpecialSitInSessionOptions(
  subjectGroups: SubjectSessions[],
): SpecialSitInSessionOption[] {
  const options: SpecialSitInSessionOption[] = [];
  const seen = new Set<string>();
  for (const group of subjectGroups) {
    const fallbackLabel =
      group.subject_name?.trim() ||
      group.course_name?.trim() ||
      group.course_code;
    for (const session of specialSitInSessionsForGroup(group)) {
      const value = session.id;
      const courseId = session.course_id;
      if (!value || !courseId || seen.has(value)) continue;
      seen.add(value);
      options.push({
        value,
        courseId,
        label: getSitInSessionSubjectTimeLabel([session], undefined, fallbackLabel, subjectGroups),
      });
    }
  }
  return options;
}

function findSpecialSitInSessionOption(
  subjectGroups: SubjectSessions[],
  sessionValue: string,
): SpecialSitInSessionOption | null {
  if (!sessionValue) return null;
  return (
    buildSpecialSitInSessionOptions(subjectGroups).find(
      (option) => option.value === sessionValue,
    ) ?? null
  );
}

function makeUpPickerOptions(
  optionGroups: Array<{
    items: NonNullable<NonNullable<SubjectSessions["sit_in"]>["available_sessions"]>[number][];
    sitInCourse?: NonNullable<SubjectSessions["sit_in"]>["sit_in_course"];
  }>,
  sessions: SubjectSessions[],
  selectedSubjectIds: string[],
  groupLabel: string,
  defaultSitInCourse?: NonNullable<SubjectSessions["sit_in"]>["sit_in_course"],
  selectedSitInSessionIds: string[] = [],
  currentValue = "",
): MakeUpOption[] {
  const selectedCounts = new Map<string, number>();
  for (const id of selectedSitInSessionIds) selectedCounts.set(id, (selectedCounts.get(id) ?? 0) + 1);
  const currentIds = new Set(splitMergedSessionValue(currentValue));
  return optionGroups.map((optionGroup) => {
    const conflicts = findSitInSessionConflicts(
      optionGroup.items,
      sessions,
      selectedSubjectIds,
    );
    const historicalDescription = optionGroup.items.map(formatHistoricalSitInConflictDescription).find(Boolean);
    const duplicateSelection = optionGroup.items.some((item) => (selectedCounts.get(item.id) ?? 0) > 0 && !currentIds.has(item.id));
    const duplicateDescription = duplicateSelection
      ? "This sit-in session is already selected for another absence day. Choose another session."
      : undefined;
    return {
      value: mergedSessionValue(optionGroup.items),
      label: getSitInSessionSubjectTimeLabel(optionGroup.items, optionGroup.sitInCourse ?? defaultSitInCourse, groupLabel, sessions),
      disabled: conflicts.length > 0 || Boolean(historicalDescription) || duplicateSelection,
      description: [formatSitInSessionConflictDescription(conflicts), historicalDescription, duplicateDescription].filter(Boolean).join(" ") || undefined,
    };
  });
}

function StepIndicator({ step }: { step: ModalStep }) {
  const currentIdx = STEP_KEYS.indexOf(step);
  return (
    <div className="mb-6 flex items-center gap-1.5">
      {STEP_KEYS.map((s, i) => {
        const isActive = s === step;
        const isComplete = i < currentIdx;
        return (
          <div key={s} className="flex items-center gap-1.5">
            <div
              className={`flex h-6 w-6 items-center justify-center rounded-full text-xs font-bold transition-colors ${
                isActive
                  ? "bg-[var(--color-wi-primary)] text-white"
                  : isComplete
                    ? "bg-emerald-500 text-white"
                    : "bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]"
              }`}
              aria-current={isActive ? "step" : undefined}
            >
              {isComplete ? (
                <svg
                  className="h-3.5 w-3.5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  strokeWidth={3}
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    d="M5 13l4 4L19 7"
                  />
                </svg>
              ) : (
                i + 1
              )}
            </div>
            {i < STEP_KEYS.length - 1 ? (
              <div
                className={`h-px w-6 transition-colors ${i < currentIdx ? "bg-emerald-300" : "bg-[var(--color-wi-row-alt)]"}`}
              />
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

function SkeletonRows({ count = 2 }: { count?: number }) {
  return (
    <div className="space-y-2">
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="h-12 animate-pulse rounded-lg bg-[var(--color-wi-row-alt)]" />
      ))}
    </div>
  );
}

export default function StaffCreateAbsenceModal({ onClose, onCreated }: Props) {
  const { addToast } = useToast();
  const [step, setStep] = useState<ModalStep>("type");
  const [absenceType, setAbsenceType] = useState<"normal" | "special">("normal");

  // Step 1: Student lookup + subject selection
  const [wcode, setWcode] = useState("");
  const [student, setStudent] = useState<StudentLookupResponse | null>(null);
  const [lookingUp, setLookingUp] = useState(false);
  const [selectedSubjectIds, setSelectedSubjectIds] = useState<string[]>([]);
  const [subjectOptions, setSubjectOptions] = useState<StaffSubjectOption[]>(
    [],
  );
  const [subjectOptionsLoading, setSubjectOptionsLoading] = useState(false);
  const [subjectOptionsError, setSubjectOptionsError] = useState<string | null>(
    null,
  );
  const [specialSubjectSelect, setSpecialSubjectSelect] = useState("");

  // Step 2: Sessions + sit-in
  const [sessions, setSessions] = useState<SubjectSessions[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [sessionsError, setSessionsError] = useState<string | null>(null);
  const [sessionsReloadToken, setSessionsReloadToken] = useState(0);
  const [selectedSessionIds, setSelectedSessionIds] = useState<Set<string>>(
    new Set(),
  );
  const [sitInSelections, setSitInSelections] = useState<
    Record<string, string>
  >({});
  const [sitInModes, setSitInModes] = useState<Record<string, SitInMode>>({});
  const [specialSitInSelections, setSpecialSitInSelections] = useState<
    Record<string, SpecialSitInSelection>
  >({});
  const [specialSitInSessionsBySubject, setSpecialSitInSessionsBySubject] =
    useState<Record<string, SubjectSessions[]>>({});
  const [specialSitInLoadingBySubject, setSpecialSitInLoadingBySubject] =
    useState<Record<string, boolean>>({});
  const [specialSitInErrorsBySubject, setSpecialSitInErrorsBySubject] =
    useState<Record<string, string>>({});
  const [sitInPriorityLevels, setSitInPriorityLevels] = useState<
    Record<string, number>
  >({});
  const [sitInPriorityHistory, setSitInPriorityHistory] = useState<
    Record<string, Record<number, SubjectSessions>>
  >({});
  const [revealingPrioritySessionIds, setRevealingPrioritySessionIds] =
    useState<Set<string>>(new Set());

  // Step 3: Confirm
  const [formConfig, setFormConfig] = useState<AbsenceFormConfig | null>(null);
  const [reasonCategory, setReasonCategory] = useState("");
  const [reason, setReason] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [smsPreview, setSmsPreview] = useState<SmsPreview | null>(null);
  const [createdAbsenceIds, setCreatedAbsenceIds] = useState<string[]>([]);
  const [sendingSms, setSendingSms] = useState(false);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const specialSitInControllersRef = useRef<Map<string, AbortController>>(
    new Map(),
  );

  const enrolledSubjectIds = useMemo(
    () => new Set((student?.subjects ?? []).map((subject) => subject.id)),
    [student],
  );

  const specialSubjectIds = useMemo(
    () =>
      selectedSubjectIds.filter(
        (subjectId) => !enrolledSubjectIds.has(subjectId),
      ),
    [selectedSubjectIds, enrolledSubjectIds],
  );

  const subjectById = useMemo(() => {
    const items = new Map<string, StaffSubjectOption>();
    for (const subject of subjectOptions) items.set(subject.id, subject);
    for (const subject of student?.subjects ?? [])
      items.set(subject.id, subject);
    return items;
  }, [student, subjectOptions]);

  const availableSpecialSubjectOptions = useMemo(
    () =>
      subjectOptions.filter(
        (subject) => !selectedSubjectIds.includes(subject.id),
      ),
    [selectedSubjectIds, subjectOptions],
  );

  const subjectPickerEntries = useMemo(
    () => combineSubjectPickerEntries(student?.subjects ?? []),
    [student],
  );

  const selectedSubjectEntryCount = useMemo(
    () =>
      subjectPickerEntries.filter((entry) =>
        entry.subjectIds.every((id) => selectedSubjectIds.includes(id)),
      ).length + specialSubjectIds.length,
    [selectedSubjectIds, specialSubjectIds, subjectPickerEntries],
  );

  const selectedSubjectGroups = useMemo(
    () =>
      sessions.filter((group) => selectedSubjectIds.includes(group.subject_id)),
    [sessions, selectedSubjectIds],
  );

  const selectedSubjectBlocks = useMemo(
    () => combineSubjectGroups(selectedSubjectGroups),
    [selectedSubjectGroups],
  );

  const ownerGroupBySessionId = useMemo(() => {
    const owners = new Map<string, SubjectSessions>();
    for (const group of selectedSubjectGroups) {
      for (const session of group.sessions) owners.set(session.id, group);
    }
    return owners;
  }, [selectedSubjectGroups]);

  const selectedSessionCount = useMemo(() => {
    return selectedSubjectBlocks.reduce(
      (count, block) =>
        count +
        groupByDay(block.sessions.filter((session) => !session.already_absent))
          .filter((dayGroup) => isDayGroupSelected(dayGroup, selectedSessionIds))
          .length,
      0,
    );
  }, [selectedSubjectBlocks, selectedSessionIds]);

  const missingSitIn = useMemo(() => {
    for (const group of sessions) {
      if (!selectedSubjectIds.includes(group.subject_id)) continue;
      for (const session of group.sessions) {
        if (!selectedSessionIds.has(session.id)) continue;
        const sitIn = sitInForMissedSession(group, session.id);
        if (sitIn?.sit_in_method === "physical" && !sitInSelections[session.id])
          return true;
      }
    }
    return false;
  }, [sessions, selectedSubjectIds, selectedSessionIds, sitInSelections]);

  const incompleteSpecialSitIn = useMemo(() => {
    for (const group of sessions) {
      if (!selectedSubjectIds.includes(group.subject_id)) continue;
      for (const session of group.sessions) {
        if (!selectedSessionIds.has(session.id)) continue;
        if ((sitInModes[session.id] ?? "suggested") !== "special") continue;
        const selection = specialSitInSelections[session.id];
        if (!selection?.subjectId || !selection.sessionValue) return true;
      }
    }
    return false;
  }, [
    sessions,
    selectedSubjectIds,
    selectedSessionIds,
    sitInModes,
    specialSitInSelections,
  ]);

  useEffect(() => {
    return () => {
      for (const controller of specialSitInControllersRef.current.values()) {
        controller.abort();
      }
      specialSitInControllersRef.current.clear();
    };
  }, []);

  // Load sessions when entering step "sessions"
  useEffect(() => {
    if (step !== "sessions" || !student || selectedSubjectIds.length === 0)
      return;
    const controller = new AbortController();
    setSessionsLoading(true);
    setSessionsError(null);
    void loadStaffSessionsForSubjects(
      student.wcode,
      selectedSubjectIds,
      enrolledSubjectIds,
      controller.signal,
    )
      .then((latestSessions) => {
        if (controller.signal.aborted) return;
        setSessions(latestSessions);
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setSessions([]);
        setSessionsError(
          error instanceof Error ? error.message : "Failed to load sessions",
        );
      })
      .finally(() => {
        if (!controller.signal.aborted) setSessionsLoading(false);
      });
    return () => controller.abort();
  }, [step, student, selectedSubjectIds, enrolledSubjectIds, sessionsReloadToken]);

  // Load form config when entering step "confirm"
  useEffect(() => {
    if (step !== "confirm" || formConfig) return;
    void apiJson<AbsenceFormConfig>("/api/v1/absence-form-config", {
      method: "GET",
    })
      .then((config) => setFormConfig(config))
      .catch(() => {
        addToast("error", "Failed to load form settings");
        setFormConfig(null);
      });
  }, [step, formConfig, addToast]);

  // Focus step heading when step changes
  useEffect(() => {
    headingRef.current?.focus();
  }, [step]);

  function syncAriaInvalid(el: EventTarget | null) {
    const input = el as (HTMLInputElement | HTMLSelectElement) | null;
    if (!input || typeof input.checkValidity !== "function") return;
    if (!input.checkValidity()) {
      input.setAttribute("aria-invalid", "true");
    } else {
      input.removeAttribute("aria-invalid");
    }
  }

  function toggleSubjects(subjectIds: string[]) {
    setSelectedSubjectIds((current) =>
      subjectIds.every((subjectId) => current.includes(subjectId))
        ? current.filter((id) => !subjectIds.includes(id))
        : [...current, ...subjectIds.filter((id) => !current.includes(id))],
    );
  }

  function toggleSubject(subjectId: string) {
    toggleSubjects([subjectId]);
  }

  function addSpecialSubject(subjectId: string) {
    if (!subjectId) return;
    setSelectedSubjectIds((current) =>
      current.includes(subjectId) ? current : [...current, subjectId],
    );
    setSpecialSubjectSelect("");
  }

  function clearSubjectOptions() {
    setSubjectOptions([]);
    setSubjectOptionsError(null);
    setSubjectOptionsLoading(false);
    setSpecialSubjectSelect("");
  }

  function clearSpecialSitInState() {
    for (const controller of specialSitInControllersRef.current.values()) {
      controller.abort();
    }
    specialSitInControllersRef.current.clear();
    setSitInModes({});
    setSpecialSitInSelections({});
    setSpecialSitInSessionsBySubject({});
    setSpecialSitInLoadingBySubject({});
    setSpecialSitInErrorsBySubject({});
  }

  async function loadSubjectOptions(fallbackSubjects: StaffSubjectOption[]) {
    setSubjectOptionsLoading(true);
    setSubjectOptionsError(null);
    try {
      const subjects = await apiJson<StaffSubjectOption[]>("/api/v1/subjects", {
        method: "GET",
      });
      setSubjectOptions(subjects);
    } catch {
      setSubjectOptions(fallbackSubjects);
      setSubjectOptionsError(
        "Could not load all subjects. Enrolled subjects are still available.",
      );
    } finally {
      setSubjectOptionsLoading(false);
    }
  }

  function loadSpecialSitInSessions(subjectId: string) {
    if (!student || !subjectId) return;
    if (
      specialSitInSessionsBySubject[subjectId] ||
      specialSitInLoadingBySubject[subjectId] ||
      specialSitInControllersRef.current.has(subjectId)
    ) {
      return;
    }
    const controller = new AbortController();
    specialSitInControllersRef.current.set(subjectId, controller);
    setSpecialSitInLoadingBySubject((current) => ({
      ...current,
      [subjectId]: true,
    }));
    setSpecialSitInErrorsBySubject((current) => {
      const next = { ...current };
      delete next[subjectId];
      return next;
    });
    void loadSessionsInRange(
      student.wcode,
      "1970-01-01",
      "2100-01-01",
      { signal: controller.signal },
      {
        bypassTiming: true,
        includeAllSubjects: true,
        subjectIds: [subjectId],
      },
    )
      .then((data) => {
        if (controller.signal.aborted) return;
        setSpecialSitInSessionsBySubject((current) => ({
          ...current,
          [subjectId]: data.subjects ?? [],
        }));
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        setSpecialSitInErrorsBySubject((current) => ({
          ...current,
          [subjectId]:
            error instanceof Error
              ? error.message
              : "Failed to load special sit-in sessions",
        }));
      })
      .finally(() => {
        if (specialSitInControllersRef.current.get(subjectId) === controller) {
          specialSitInControllersRef.current.delete(subjectId);
        }
        if (!controller.signal.aborted) {
          setSpecialSitInLoadingBySubject((current) => ({
            ...current,
            [subjectId]: false,
          }));
        }
      });
  }

  function handleSessionGroupToggle(sessionIds: string[]) {
    setSelectedSessionIds((current) => {
      const selected = sessionIds.every((id) => current.has(id));
      if (selected) {
        const next = new Set(current);
        for (const id of sessionIds) next.delete(id);
        setSitInSelections((cs) => {
          const n = { ...cs };
          for (const id of sessionIds) delete n[id];
          return n;
        });
        setSitInModes((currentModes) => {
          const next = { ...currentModes };
          for (const id of sessionIds) delete next[id];
          return next;
        });
        setSpecialSitInSelections((currentSelections) => {
          const next = { ...currentSelections };
          for (const id of sessionIds) delete next[id];
          return next;
        });
        return next;
      }
      const next = new Set(current);
      for (const id of sessionIds) next.add(id);
      return next;
    });
  }

  function handleSitInSelectForSessions(
    sessionIds: string[],
    sitInSessionId: string,
  ) {
    setSitInSelections((current) => {
      const next = { ...current };
      for (const id of sessionIds) {
        if (!sitInSessionId) delete next[id];
        else next[id] = sitInSessionId;
      }
      return next;
    });
  }

  function setSitInModeForSessions(sessionIds: string[], mode: SitInMode) {
    const firstSessionId = sessionIds[0];
    if (!firstSessionId) return;
    if ((sitInModes[firstSessionId] ?? "suggested") === mode) return;
    setSitInModes((current) => {
      const next = { ...current };
      for (const id of sessionIds) {
        if (mode === "suggested") delete next[id];
        else next[id] = mode;
      }
      return next;
    });
    setSitInSelections((current) => {
      const next = { ...current };
      for (const id of sessionIds) delete next[id];
      return next;
    });
    setSpecialSitInSelections((current) => {
      const next = { ...current };
      for (const id of sessionIds) delete next[id];
      return next;
    });
  }

  function handleSpecialSitInSubjectSelect(
    sessionIds: string[],
    subjectId: string,
  ) {
    setSpecialSitInSelections((current) => {
      const next = { ...current };
      for (const id of sessionIds) {
        if (!subjectId) delete next[id];
        else next[id] = { subjectId, sessionValue: "" };
      }
      return next;
    });
    setSitInSelections((current) => {
      const next = { ...current };
      for (const id of sessionIds) delete next[id];
      return next;
    });
    if (subjectId) loadSpecialSitInSessions(subjectId);
  }

  function handleSpecialSitInSessionSelect(
    sessionIds: string[],
    subjectId: string,
    sessionValue: string,
  ) {
    setSpecialSitInSelections((current) => {
      const next = { ...current };
      for (const id of sessionIds) {
        if (!subjectId) delete next[id];
        else next[id] = { subjectId, sessionValue };
      }
      return next;
    });
    handleSitInSelectForSessions(sessionIds, sessionValue);
  }

  async function handleNotAvailable(group: SubjectSessions, sessionId: string) {
    const currentLevel =
      sitInPriorityLevels[sessionId] ||
      group.sit_in?.current_priority_level ||
      firstPriorityLevel(group);
    if (student && hasServerPriorityReveal(group)) {
      setRevealingPrioritySessionIds((current) =>
        new Set(current).add(sessionId),
      );
      setSitInSelections((prev) => {
        const n = { ...prev };
        delete n[sessionId];
        return n;
      });
      setSitInPriorityHistory((prev) => ({
        ...prev,
        [sessionId]: { ...(prev[sessionId] ?? {}), [currentLevel]: group },
      }));
      try {
        const data = await loadSessionsInRange(
          student.wcode,
          undefined,
          undefined,
          undefined,
          {
            courseIds: [group.course_id],
            satVerbalAfterPriority: currentLevel,
          },
        );
        const updatedGroup = data.subjects.find(
          (subject) => subject.course_id === group.course_id,
        );
        if (!updatedGroup) {
          addToast(
            "error",
            "No more make-up times are available for this class.",
          );
          return;
        }
        const updatedSessionGroup = groupWithSitInForMissedSession(
          updatedGroup,
          sessionId,
        );
        const updatedLevel =
          updatedSessionGroup.sit_in?.current_priority_level ??
          firstPriorityLevel(updatedSessionGroup);
        setSitInPriorityLevels((prev) => ({
          ...prev,
          [sessionId]: updatedLevel,
        }));
        setSitInPriorityHistory((prev) => ({
          ...prev,
          [sessionId]: {
            ...(prev[sessionId] ?? {}),
            [updatedLevel]: updatedSessionGroup,
          },
        }));
      } catch (error) {
        addToast(
          "error",
          error instanceof Error
            ? error.message
            : "Couldn't load other make-up times",
        );
      } finally {
        setRevealingPrioritySessionIds((current) => {
          const n = new Set(current);
          n.delete(sessionId);
          return n;
        });
      }
      return;
    }
    const nextLvl = nextPriorityLevel(group, currentLevel);
    if (nextLvl == null) return;
    setSitInPriorityLevels((prev) => ({ ...prev, [sessionId]: nextLvl }));
    setSitInSelections((prev) => {
      const n = { ...prev };
      delete n[sessionId];
      return n;
    });
  }

  function handlePreviousPriority(group: SubjectSessions, sessionId: string) {
    const currentLevel =
      sitInPriorityLevels[sessionId] ||
      group.sit_in?.current_priority_level ||
      firstPriorityLevel(group);
    if (hasServerPriorityReveal(group)) {
      const history = sitInPriorityHistory[sessionId] ?? {};
      const previousLevel = Object.keys(history)
        .map(Number)
        .filter((lvl) => lvl < currentLevel)
        .sort((a, b) => b - a)[0];
      const previousGroup =
        previousLevel !== undefined ? history[previousLevel] : undefined;
      if (!previousGroup) return;
      setSitInPriorityLevels((prev) => ({
        ...prev,
        [sessionId]: previousLevel,
      }));
      setSitInSelections((prev) => {
        const n = { ...prev };
        delete n[sessionId];
        return n;
      });
      return;
    }
    const prevLvl = previousPriorityLevel(group, currentLevel);
    if (prevLvl == null) return;
    setSitInPriorityLevels((prev) => ({ ...prev, [sessionId]: prevLvl }));
    setSitInSelections((prev) => {
      const n = { ...prev };
      delete n[sessionId];
      return n;
    });
  }

  function canAdvanceFromSubjects(): boolean {
    return !!student && selectedSubjectIds.length > 0;
  }

  function canAdvanceFromSessions(): boolean {
    return selectedSessionCount > 0 && !incompleteSpecialSitIn;
  }

  function getSpecialSitInReviewLabel(missedSessionId: string): string | null {
    if ((sitInModes[missedSessionId] ?? "suggested") !== "special") return null;
    const selection = specialSitInSelections[missedSessionId];
    if (!selection?.subjectId || !selection.sessionValue) return null;
    return (
      findSpecialSitInSessionOption(
        specialSitInSessionsBySubject[selection.subjectId] ?? [],
        selection.sessionValue,
      )?.label ?? null
    );
  }

  function renderSitInModeToggle(sessionIds: string[], firstSessionId: string) {
    const mode = sitInModes[firstSessionId] ?? "suggested";
    return (
      <div className="mb-3 inline-flex rounded-md border border-wi-line bg-[var(--color-wi-row-alt)] p-0.5">
        <button
          type="button"
          onClick={() => setSitInModeForSessions(sessionIds, "suggested")}
          className={`rounded px-2.5 py-1 text-xs font-semibold transition ${
            mode === "suggested"
              ? "bg-white text-[var(--color-wi-text)] shadow-sm"
              : "text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text-light)]"
          }`}
        >
          Suggested
        </button>
        <button
          type="button"
          onClick={() => setSitInModeForSessions(sessionIds, "special")}
          className={`rounded px-2.5 py-1 text-xs font-semibold transition ${
            mode === "special"
              ? "bg-white text-[var(--color-wi-text)] shadow-sm"
              : "text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text-light)]"
          }`}
        >
          Special sit-in
        </button>
      </div>
    );
  }

  function renderSpecialSitInControls(
    sessionIds: string[],
    firstSessionId: string,
  ) {
    const selection = specialSitInSelections[firstSessionId] ?? {
      subjectId: "",
      sessionValue: "",
    };
    const subjectGroups = selection.subjectId
      ? (specialSitInSessionsBySubject[selection.subjectId] ?? [])
      : [];
    const sessionOptions = buildSpecialSitInSessionOptions(subjectGroups);
    const hasBlockedSession = subjectGroups.some((group) =>
      (group.sit_in?.unavailable_sessions ?? []).some(
        (unavailable) => unavailable.reason_code === "sit_in_session_already_used",
      ),
    );
    const loading = selection.subjectId
      ? Boolean(specialSitInLoadingBySubject[selection.subjectId])
      : false;
    const error = selection.subjectId
      ? specialSitInErrorsBySubject[selection.subjectId]
      : null;

    return (
      <div className="rounded-lg border border-amber-200 bg-amber-50/40 p-3">
        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <label
              className="mb-1 block text-xs font-medium text-[var(--color-wi-text-light)]"
              htmlFor={`staff-special-sit-in-subject-${firstSessionId}`}
            >
              Special sit-in subject
            </label>
            <Select
              id={`staff-special-sit-in-subject-${firstSessionId}`}
              size="sm"
              value={selection.subjectId}
              onChange={(e) =>
                handleSpecialSitInSubjectSelect(sessionIds, e.target.value)
              }
              disabled={subjectOptionsLoading || subjectOptions.length === 0}
              placeholder={
                subjectOptionsLoading ? "Loading subjects..." : "Subject"
              }
            >
              {subjectOptions.map((subject) => (
                <option key={subject.id} value={subject.id}>
                  {subject.code} — {subject.name}
                </option>
              ))}
            </Select>
          </div>
          <div>
            <label
              className="mb-1 block text-xs font-medium text-[var(--color-wi-text-light)]"
              htmlFor={`staff-special-sit-in-session-${firstSessionId}`}
            >
              Special sit-in session
            </label>
            <Select
              id={`staff-special-sit-in-session-${firstSessionId}`}
              size="sm"
              value={selection.sessionValue}
              onChange={(e) =>
                handleSpecialSitInSessionSelect(
                  sessionIds,
                  selection.subjectId,
                  e.target.value,
                )
              }
              disabled={
                !selection.subjectId || loading || sessionOptions.length === 0
              }
              placeholder={loading ? "Loading sessions..." : "Session"}
            >
              {sessionOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </Select>
          </div>
        </div>
        {error ? (
          <p role="alert" className="mt-2 text-xs text-red-600">
            {error}
          </p>
        ) : selection.subjectId && !loading && sessionOptions.length === 0 ? (
          <p className="mt-2 text-xs text-[var(--color-wi-text-light)]">
            {hasBlockedSession
              ? "The selected sit-in session is already used for this student. Choose another session."
              : "No sessions found for this subject."}
          </p>
        ) : null}
      </div>
    );
  }

  function hasDuplicateSelectedSitInSessions(): boolean {
    const datesBySitInSession = new Map<string, Set<string>>();
    for (const group of sessions) {
      if (!selectedSubjectIds.includes(group.subject_id)) continue;
      for (const missed of getSelectedSessionsForGroup(group, selectedSessionIds)) {
        const selection = (sitInModes[missed.id] ?? "suggested") === "special"
          ? specialSitInSelections[missed.id]?.sessionValue
          : sitInSelections[missed.id];
        for (const sessionId of splitMergedSessionValue(selection)) {
          const dates = datesBySitInSession.get(sessionId) ?? new Set<string>();
          dates.add(missed.date);
          datesBySitInSession.set(sessionId, dates);
        }
      }
    }
    return [...datesBySitInSession.values()].some((dates) => dates.size > 1);
  }

  function handleNext() {
    if (step === "type") {
      setStep("subjects");
    } else if (step === "subjects") {
      if (!canAdvanceFromSubjects()) {
        addToast(
          "error",
          !student ? "Look up a student first" : "Select at least one subject",
        );
        return;
      }
      setSelectedSessionIds(new Set());
      setSitInSelections({});
      clearSpecialSitInState();
      setSitInPriorityLevels({});
      setSitInPriorityHistory({});
      setSessions([]);
      setStep("sessions");
    } else if (step === "sessions") {
      if (selectedSessionCount === 0) {
        addToast("error", "Select at least one missed class");
        return;
      }
      if (!canAdvanceFromSessions()) {
        addToast("error", "Select a special sit-in subject and session");
        return;
      }
      if (hasDuplicateSelectedSitInSessions()) {
        addToast("error", "The same sit-in session is selected for more than one absence day. Choose a different session before continuing.");
        return;
      }
      setStep("confirm");
    }
  }

  function handleBack() {
    if (step === "sessions") setStep("subjects");
    else if (step === "confirm") setStep("sessions");
    else if (step === "subjects") setStep("type");
  }

  function handleSitInDayConflict(error?: unknown) {
    setSitInSelections({});
    setSpecialSitInSelections({});
    setSitInPriorityLevels({});
    setSitInPriorityHistory({});
    setSessionsReloadToken((current) => current + 1);
    setStep("sessions");
    addToast(
      "error",
      formatSitInSubmissionConflictDetails(error instanceof ApiRequestError ? error.details : undefined) ?? "That sit-in session was just used for this student. We refreshed the sessions; choose another session and submit again.",
    );
  }

  function handleDuplicateSitInSelection() {
    setSitInSelections({});
    setSpecialSitInSelections({});
    setSitInPriorityLevels({});
    setSitInPriorityHistory({});
    setSessionsReloadToken((current) => current + 1);
    setStep("sessions");
    addToast(
      "error",
      "The same sit-in session was selected more than once. We refreshed the sessions; choose a different session and submit again.",
    );
  }

  async function lookupStudent() {
    if (!wcode.trim()) return;
    setLookingUp(true);
    setStudent(null);
    setSelectedSubjectIds([]);
    try {
      const data = await lookupStaffStudentByWcode(wcode.trim());
      setStudent(data);
      await loadSubjectOptions(data.subjects);
    } catch (err) {
      addToast(
        "error",
        err instanceof Error ? err.message : "Student not found",
      );
    } finally {
      setLookingUp(false);
    }
  }

  async function handleSubmit() {
    if (!student) return;
    if (incompleteSpecialSitIn) {
      addToast("error", "Select a special sit-in subject and session");
      return;
    }
    if (hasDuplicateSelectedSitInSessions()) {
      handleDuplicateSitInSelection();
      return;
    }
    setSubmitting(true);
    let submissionSessions: SubjectSessions[];
    try {
      submissionSessions = await loadStaffSessionsForSubjects(
        student.wcode,
        selectedSubjectIds,
        enrolledSubjectIds,
      );
      setSessions(submissionSessions);
      const blockedSessionIds = blockedSitInSessionIds(submissionSessions);
      const selectedSitInSessionIds = new Set([
        ...Object.values(sitInSelections).flatMap((value) => splitMergedSessionValue(value)),
        ...Object.values(specialSitInSelections).flatMap((selection) =>
          splitMergedSessionValue(selection.sessionValue),
        ),
      ]);
      const staleSessionIds = [...selectedSitInSessionIds].filter((id) =>
        blockedSessionIds.has(id),
      );
      if (staleSessionIds.length > 0) {
        handleSitInDayConflict();
        setSubmitting(false);
        return;
      }
    } catch (error) {
      setSubmitting(false);
      addToast(
        "error",
        error instanceof Error
          ? `Couldn't refresh sit-in sessions: ${error.message}`
          : "Couldn't refresh sit-in sessions. Try again.",
      );
      return;
    }
    const created: string[] = [];
    const requests: StaffCreateAbsenceRequest[] = [];
    let sitInDayConflict = false;

    for (const group of submissionSessions) {
      if (!selectedSubjectIds.includes(group.subject_id)) continue;
      const selectedSessions = getSelectedSessionsForGroup(
        group,
        selectedSessionIds,
      );
      if (selectedSessions.length === 0) continue;

      const missedIds = selectedSessions.map((s) => s.id);
      const hasSpecialSitIn = missedIds.some(
        (id) => (sitInModes[id] ?? "suggested") === "special",
      );

      // --- Special sit-in path: one absence per distinct sit-in course ---
      // A single absence (e.g. absent from subject A) may be made up across
      // several other subjects (C, D, E, F) at once, so we emit one
      // staff-create record per distinct special sit-in course.
      if (hasSpecialSitIn) {
        const partitions = new Map<
          string,
          { missed: string[]; sessions: string[]; method: string | undefined }
        >();
        for (const missedId of missedIds) {
          const isSpecial =
            (sitInModes[missedId] ?? "suggested") === "special";
          let courseId: string;
          let sessionIds: string[];
          let method: string | undefined;
          if (isSpecial) {
            const sel = specialSitInSelections[missedId];
            const option =
              sel?.sessionValue
                ? findSpecialSitInSessionOption(
                    specialSitInSessionsBySubject[sel.subjectId] ?? [],
                    sel.sessionValue,
                  )
                : null;
            courseId = option?.courseId ?? group.course_id;
            sessionIds = sel?.sessionValue
              ? splitMergedSessionValue(sel.sessionValue)
              : [];
            method = courseId && sessionIds.length > 0 ? "physical" : undefined;
          } else {
            sessionIds = splitMergedSessionValue(sitInSelections[missedId]);
            courseId =
              selectedSitInCourseIDForGroup(
                group,
                [missedId],
                sitInSelections,
                sitInPriorityLevels,
                sitInPriorityHistory,
              ) ?? group.course_id;
            const sitIn = sitInForMissedSession(group, missedId);
            method =
              sitIn?.sit_in_method === "physical" ||
              sitIn?.sit_in_method === "zoom"
                ? sitIn.sit_in_method
                : undefined;
          }
          const bucket =
            partitions.get(courseId) ?? { missed: [], sessions: [], method };
          bucket.missed.push(missedId);
          for (const sid of sessionIds) {
            if (!bucket.sessions.includes(sid)) bucket.sessions.push(sid);
          }
          bucket.method = method ?? bucket.method;
          partitions.set(courseId, bucket);
        }

        for (const [courseId, bucket] of partitions) {
          const partSessions = selectedSessions.filter((s) =>
            bucket.missed.includes(s.id),
          );
          const dates = [...new Set(partSessions.map((s) => s.date))].sort();
          const dateFrom = dates[0];
          const dateTo = dates[dates.length - 1];
          if (!dateFrom || !dateTo) continue;
          requests.push({
            wcode: student.wcode,
            subject_id: group.subject_id,
            course_id: group.course_id,
            date_from: dateFrom,
            date_to: dateTo,
            missed_session_ids: bucket.missed,
            sit_in_method: bucket.method,
            sit_in_course_id: bucket.method ? courseId : undefined,
            sit_in_session_ids: bucket.sessions,
            reason_category: reasonCategory || undefined,
            reason: reason || undefined,
            status:
              absenceType === "special" ? "special_approved" : undefined,
          });
        }
        if (sitInDayConflict) break;
        continue;
      }

      // --- Standard single-course path (no special sit-ins) ---
      const dates = [...new Set(selectedSessions.map((s) => s.date))].sort();
      const dateFrom = dates[0];
      const dateTo = dates[dates.length - 1];
      if (!dateFrom || !dateTo) continue;

      const sitInSessionIds: string[] = [];
      let sitInMethod: string | undefined;

      for (const session of missedIds) {
        const selected = splitMergedSessionValue(sitInSelections[session]);
        for (const sid of selected) sitInSessionIds.push(sid);
        const sitIn = sitInForMissedSession(group, session);
        if (
          sitIn?.sit_in_method === "physical" ||
          sitIn?.sit_in_method === "zoom"
        ) {
          sitInMethod = sitIn.sit_in_method;
        }
      }

      const uniqueSitInSessionIds = [...new Set(sitInSessionIds)];
      const sitInCourseId =
        selectedSitInCourseIDForGroup(
          group,
          missedIds,
          sitInSelections,
          sitInPriorityLevels,
          sitInPriorityHistory,
        ) ?? group.course_id;

      requests.push({
        wcode: student.wcode,
        subject_id: group.subject_id,
        course_id: group.course_id,
        date_from: dateFrom,
        date_to: dateTo,
        missed_session_ids: missedIds,
        sit_in_method: sitInMethod,
        sit_in_course_id: sitInMethod ? sitInCourseId : undefined,
        sit_in_session_ids: uniqueSitInSessionIds,
        reason_category: reasonCategory || undefined,
        reason: reason || undefined,
        status: absenceType === "special" ? "special_approved" : undefined,
      });
    }

    const scopeKeyByCourseID = new Map(
      submissionSessions.map((group) => [group.course_id, absenceScopeKey(group)]),
    );
    const mergedRequests = mergeAbsenceBatchItemsByScope(
      requests.map((item) => ({
        scopeKey:
          scopeKeyByCourseID.get(item.course_id ?? "") ??
          `course:${item.course_id ?? ""}`,
        item,
      })),
    );
    requests.splice(0, requests.length, ...mergedRequests);

    if (duplicateSitInSessionIds(requests).length > 0) {
      handleDuplicateSitInSelection();
      setSubmitting(false);
      return;
    }

    if (requests.length > 0) {
      try {
        const res = await apiJson<StaffCreateAbsenceBatchResponse>(
          "/api/v1/absences/staff-create",
          {
            method: "POST",
            headers: { "X-Staff-Batch": "true" },
            body: JSON.stringify({ ...requests[0], items: requests }),
          },
        );
        created.push(...res.ids);
      } catch (err) {
        if (err instanceof ApiRequestError && err.code === "sit_in_session_already_used") {
          handleSitInDayConflict(err);
          sitInDayConflict = true;
        } else {
          addToast("error", err instanceof Error ? err.message : "Failed to create absences");
        }
      }
    }

    setSubmitting(false);
    if (sitInDayConflict) return;
    if (created.length === 0) return;
    setCreatedAbsenceIds(created);

    if (created.length > 0) {
      try {
        const preview = await apiJson<{ preview?: SmsPreview }>(
          "/api/v1/absences/batch-send-success-sms",
          {
            method: "POST",
            body: JSON.stringify({ ids: created, dry_run: true }),
          },
        );
        if (preview.preview && preview.preview.phones.length > 0) {
          setSmsPreview(preview.preview);
          return;
        }
      } catch {
        // fall through to email auto-send
      }

      try {
        const res = await apiJson<{ email_sent: boolean; queued?: boolean }>(
          "/api/v1/absences/batch-send-success-sms",
          {
            method: "POST",
            body: JSON.stringify({ ids: created }),
          },
        );
        if (res.email_sent || res.queued) {
          const channels = [
            ...(res.queued ? ["SMS queued"] : []),
            ...(res.email_sent ? ["Email notification sent"] : []),
          ];
          addToast(
            "success",
            `${created.length} absence${created.length !== 1 ? "s" : ""} created · ${channels.join(" · ")}`,
          );
        } else {
          addToast(
            "success",
            `${created.length} absence${created.length !== 1 ? "s" : ""} created`,
          );
        }
      } catch {
        addToast(
          "success",
          `${created.length} absence${created.length !== 1 ? "s" : ""} created`,
        );
      }
      onCreated();
      return;
    }
  }

  async function handleSendSms() {
    if (createdAbsenceIds.length === 0) {
      addToast("error", "Missing absence ID, cannot send notifications");
      return;
    }
    setSendingSms(true);
    try {
      const res = await apiJson<{ sent: boolean; queued?: boolean; sms_queued?: boolean; sms_sent: boolean; email_sent: boolean; recipient_count: number }>(
        "/api/v1/absences/batch-send-success-sms",
        {
          method: "POST",
          body: JSON.stringify({ ids: createdAbsenceIds }),
        },
      );
      const smsQueued = res.queued === true || res.sms_queued === true;
      if (!res.sent && !smsQueued) {
        addToast("error", "Notifications were not sent");
        return;
      }
      if (res.sms_sent) {
        const parts = ["SMS", ...(res.email_sent ? ["email"] : [])];
        addToast(
          "success",
          `${parts.join(" & ")} sent to ${res.recipient_count} recipient${res.recipient_count !== 1 ? "s" : ""}`,
        );
      } else if (smsQueued) {
        const parts = ["SMS queued", ...(res.email_sent ? ["email sent"] : [])];
        addToast(
          "success",
          `${parts.join(" · ")} to ${res.recipient_count} recipient${res.recipient_count !== 1 ? "s" : ""}`,
        );
      } else {
        addToast(
          "success",
          `email sent to ${res.recipient_count} recipient${res.recipient_count !== 1 ? "s" : ""}`,
        );
      }
      onCreated();
    } catch (err) {
      addToast(
        "error",
        err instanceof Error ? err.message : "Failed to send notifications",
      );
    } finally {
      setSendingSms(false);
    }
  }

  function handleSkipSms() {
    if (sendingSms) return;
    addToast("success", "Absence(s) created successfully");
    onCreated();
  }

  if (smsPreview && createdAbsenceIds.length > 0) {
    return (
      <SmsConfirmModal
        phones={smsPreview.phones}
        message={smsPreview.message}
        onSend={() => void handleSendSms()}
        onSkip={handleSkipSms}
        sending={sendingSms}
      />
    );
  }

  return (
    <Modal
      title="Create Absence"
      onClose={onClose}
      size="xl"
      footer={
        step === "confirm" ? (
          <>
            <div className="flex-1" />
            <Button variant="secondary" onClick={onClose}>
              Cancel
            </Button>
            <Button loading={submitting} onClick={() => void handleSubmit()}>
              Create Absence
            </Button>
          </>
        ) : (
          <>
            <div className="flex-1" />
            {step !== "type" ? (
              <Button variant="secondary" onClick={handleBack}>
                Back
              </Button>
            ) : null}
            <Button onClick={handleNext}>
              Next <ChevronRight className="ml-1 inline h-4 w-4" />
            </Button>
          </>
        )
      }
    >
      <StepIndicator step={step} />

      {/* Step 0: Absence Type */}
      {step === "type" && (
        <div className="space-y-5">
          <h2 ref={headingRef} tabIndex={-1} className="sr-only">
            Step 1: Select Absence Type
          </h2>
          <p className="text-sm text-[var(--color-wi-text-light)]">
            Choose the type of absence to create:
          </p>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <button
              type="button"
              aria-pressed={absenceType === "normal"}
              onClick={() => setAbsenceType("normal")}
              className={`rounded-lg border-2 p-6 text-left transition-colors ${
                absenceType === "normal"
                  ? "border-blue-500 bg-blue-50"
                  : "border-wi-line hover:border-wi-line"
              }`}
            >
              <div className="flex items-center gap-3">
                <div className={`rounded-full p-2 ${
                  absenceType === "normal" ? "bg-blue-100" : "bg-[var(--color-wi-row-alt)]"
                }`}>
                  <Info className={`h-5 w-5 ${
                    absenceType === "normal" ? "text-blue-600" : "text-[var(--color-wi-text-light)]"
                  }`} />
                </div>
                <div>
                  <p className="font-medium text-[var(--color-wi-text)]">Normal Absence</p>
                  <p className="text-sm text-[var(--color-wi-text-light)]">Requires review and approval</p>
                </div>
              </div>
            </button>
            <button
              type="button"
              aria-pressed={absenceType === "special"}
              onClick={() => setAbsenceType("special")}
              className={`rounded-lg border-2 p-6 text-left transition-colors ${
                absenceType === "special"
                  ? "border-purple-500 bg-purple-50"
                  : "border-wi-line hover:border-wi-line"
              }`}
            >
              <div className="flex items-center gap-3">
                <div className={`rounded-full p-2 ${
                  absenceType === "special" ? "bg-purple-100" : "bg-[var(--color-wi-row-alt)]"
                }`}>
                  <Info className={`h-5 w-5 ${
                    absenceType === "special" ? "text-purple-600" : "text-[var(--color-wi-text-light)]"
                  }`} />
                </div>
                <div>
                  <p className="font-medium text-[var(--color-wi-text)]">Special Absence</p>
                  <p className="text-sm text-[var(--color-wi-text-light)]">Pre-approved, skips review</p>
                </div>
              </div>
            </button>
          </div>
          {absenceType === "special" && (
            <div className="rounded-lg border border-purple-200 bg-purple-50 p-3 text-sm text-purple-700">
              <p>This absence will be created with <strong>Special Approved</strong> status and will not count toward the student&apos;s absence rate limit.</p>
            </div>
          )}
        </div>
      )}

      {/* Step 1: Student + Subjects */}
      {step === "subjects" && (
        <div className="space-y-5">
          <h2 ref={headingRef} tabIndex={-1} className="sr-only">
            Step 1: Select student and subjects
          </h2>
          <div className="field">
            <label
              htmlFor="staff-wcode"
              className="mb-1.5 block text-sm font-medium text-[var(--color-wi-text-light)]"
            >
              Student W-Code
            </label>
            <div className="flex gap-2">
              <input
                id="staff-wcode"
                type="text"
                autoComplete="off"
                className="flex-1 rounded-sm border border-wi-line px-3 py-2 text-sm"
                placeholder="e.g. W001234"
                required
                aria-errormessage="wcode-error"
                value={wcode}
                onChange={(e) => {
                  setWcode(e.target.value);
                  setStudent(null);
                  setSelectedSubjectIds([]);
                  clearSubjectOptions();
                  clearSpecialSitInState();
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && wcode.trim()) void lookupStudent();
                }}
                onBlur={(e) => syncAriaInvalid(e.currentTarget)}
                onInput={(e) => {
                  if (e.currentTarget.checkValidity())
                    e.currentTarget.removeAttribute("aria-invalid");
                }}
              />
              <Button
                variant="secondary"
                onClick={() => void lookupStudent()}
                loading={lookingUp}
              >
                Look up
              </Button>
            </div>
            <p id="wcode-error" className="error-msg" role="alert">
              Enter a student W-Code to continue
            </p>
          </div>

          {student ? (
            <>
              <div className="rounded-lg border border-emerald-200 bg-emerald-50/50 px-4 py-3">
                <p className="text-sm font-medium text-emerald-800">
                  {student.nickname?.trim() || student.full_name}
                </p>
                {student.nickname && student.full_name && student.nickname.trim() !== student.full_name ? (
                  <p className="text-xs text-emerald-700">{student.full_name}</p>
                ) : null}
                {student.school ? (
                  <p className="text-xs text-emerald-700">{student.school}</p>
                ) : null}
                <p className="text-xs text-emerald-600">{student.wcode}</p>
              </div>

              {student.subjects.length > 0 ? (
                <div>
                  <label className="mb-2 block text-sm font-medium text-[var(--color-wi-text-light)]">
                    Subjects
                  </label>
                  <p className="mb-2 text-xs text-[var(--color-wi-text-light)]">
                    Select one or more subjects
                  </p>
                  <div className="divide-y divide-wi-line rounded-lg border border-wi-line overflow-hidden">
                    {subjectPickerEntries.map((entry) => (
                      <SubjectCard
                        key={entry.key}
                        id={entry.key}
                        name={appendTeacher(entry.label, entry.teacherName)}
                        selected={entry.subjectIds.every((id) =>
                          selectedSubjectIds.includes(id),
                        )}
                        onToggle={() => toggleSubjects(entry.subjectIds)}
                      />
                    ))}
                  </div>
                  {selectedSubjectIds.length > 0 ? (
                    <p className="mt-2 text-xs text-[var(--color-wi-text-light)]">
                      {selectedSubjectEntryCount} subject
                      {selectedSubjectEntryCount !== 1 ? "s" : ""} selected
                    </p>
                  ) : null}
                </div>
              ) : (
                <p className="text-sm text-[var(--color-wi-text-light)]">
                  No enrolled subjects found for this student.
                </p>
              )}

              <div>
                <label
                  htmlFor="staff-special-subject"
                  className="mb-2 block text-sm font-medium text-[var(--color-wi-text-light)]"
                >
                  Special case subject
                </label>
                <Select
                  id="staff-special-subject"
                  value={specialSubjectSelect}
                  onChange={(e) => {
                    setSpecialSubjectSelect(e.target.value);
                    addSpecialSubject(e.target.value);
                  }}
                  disabled={
                    subjectOptionsLoading ||
                    availableSpecialSubjectOptions.length === 0
                  }
                  placeholder={
                    subjectOptionsLoading
                      ? "Loading subjects..."
                      : "Add a subject..."
                  }
                >
                  {availableSpecialSubjectOptions.map((subject) => (
                    <option key={subject.id} value={subject.id}>
                      {subject.code} — {subject.name}
                      {enrolledSubjectIds.has(subject.id) ? " (enrolled)" : ""}
                    </option>
                  ))}
                </Select>
                {subjectOptionsError ? (
                  <p role="alert" className="mt-1.5 text-xs text-amber-600">
                    {subjectOptionsError}
                  </p>
                ) : (
                  <p className="mt-1.5 text-xs text-[var(--color-wi-text-light)]">
                    Use this when staff need to record an absence outside the
                    enrolled subject list.
                  </p>
                )}
                {specialSubjectIds.length > 0 ? (
                  <div className="mt-3 divide-y divide-wi-line overflow-hidden rounded-lg border border-amber-200 bg-amber-50/40">
                    {specialSubjectIds.map((subjectId) => {
                      const subject = subjectById.get(subjectId);
                      if (!subject) return null;
                      return (
                        <SubjectCard
                          key={subject.id}
                          id={`special-${subject.id}`}
                          name={subject.name}
                          selected
                          onToggle={() => toggleSubject(subject.id)}
                        />
                      );
                    })}
                  </div>
                ) : null}
              </div>
            </>
          ) : null}
        </div>
      )}

      {/* Step 2: Classes + Make-up */}
      {step === "sessions" && (
        <div className="space-y-4">
          <h2 ref={headingRef} tabIndex={-1} className="sr-only">
            Step 2: Select missed classes and make-up sessions
          </h2>
          {selectedSubjectIds.length > 0 && student ? (
            <>
              {sessionsLoading ? (
                <SkeletonRows count={3} />
              ) : sessionsError ? (
                <p role="alert" className="text-sm text-red-600">
                  {sessionsError}
                </p>
              ) : selectedSubjectBlocks.length === 0 ? (
                <p className="text-sm text-[var(--color-wi-text-light)]">
                  No classes found for the selected subjects.
                </p>
              ) : (
                <div className="space-y-4">
                  {selectedSubjectBlocks.map((block) => {
                      const sessionGroups = groupByDay(
                        block.sessions.filter((s) => !s.already_absent),
                      );
                      const groupLabel = appendTeacher(
                        block.label,
                        block.teacherName,
                      );
                      if (sessionGroups.length === 0) return null;
                      return (
                        <div
                          key={block.key}
                          className="rounded-lg border border-wi-line bg-white overflow-hidden shadow-sm"
                        >
                          <div className="flex items-center justify-between gap-2 border-b border-wi-line bg-[var(--color-wi-row-alt)]/50 px-4 py-3">
                            <span className="text-sm font-semibold text-[var(--color-wi-text)] truncate">
                              {groupLabel} ({sessionGroups.length} class day
                              {sessionGroups.length !== 1 ? "s" : ""})
                            </span>
                            <span className="text-xs font-semibold text-[var(--color-wi-text-light)] shrink-0">
                              {
                                sessionGroups.filter((g) =>
                                  isDayGroupSelected(g, selectedSessionIds),
                                ).length
                              }{" "}
                              selected
                            </span>
                          </div>
                          <div className="space-y-2 p-4">
                            {sessionGroups.map((dayGroup) => {
                              const sessionIds = dayGroup.items.map(
                                (item) => item.id,
                              );
                              const selected = isDayGroupSelected(
                                dayGroup,
                                selectedSessionIds,
                              );
                              const firstSessionId = sessionIds[0];
                              const ownerGroup =
                                ownerGroupBySessionId.get(firstSessionId) ??
                                block.groups[0];
                              const sessionGroup =
                                groupWithSitInForMissedSession(
                                  ownerGroup,
                                  firstSessionId,
                                );
                              const sitIn = sessionGroup.sit_in;
                              const sitInUnavailable =
                                sitIn?.unavailable_sessions ?? [];
                              const sitInAvailable =
                                rootAvailableSessionsForMissedSessions(
                                  sitIn,
                                  sessionIds,
                                );
                              const hasPriorities = Boolean(
                                sitIn?.priorities &&
                                sitIn.priorities.length > 0,
                              );
                              const baseLevel =
                                sitIn?.current_priority_level ||
                                firstPriorityLevel(sessionGroup);
                              const currentLevel =
                                sitInPriorityLevels[firstSessionId] ||
                                baseLevel;
                              const priorityGroup =
                                sitInPriorityHistory[firstSessionId]?.[
                                  currentLevel
                                ] ?? sessionGroup;
                              const currentPriorities = hasPriorities
                                ? prioritiesForLevel(
                                    priorityGroup,
                                    currentLevel,
                                  )
                                : [];
                              const currentSitIn =
                                sitInSelections[firstSessionId] || "";

                              return (
                                <div
                                  key={dayGroup.id}
                                  className={`rounded-lg border px-4 py-3 transition-colors ${selected ? "border-blue-300 bg-blue-50/30" : "border-wi-line bg-white"}`}
                                >
                                  <div className="flex items-center gap-3">
                                    <input
                                      type="checkbox"
                                      id={`staff-session-${dayGroup.id}`}
                                      checked={selected}
                                      onChange={() =>
                                        handleSessionGroupToggle(sessionIds)
                                      }
                                      className="h-4 w-4 shrink-0 rounded border-wi-line text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20"
                                    />
                                    <label
                                      htmlFor={`staff-session-${dayGroup.id}`}
                                      className="min-w-0 cursor-pointer flex-1"
                                    >
                                      <span className="text-sm font-medium text-[var(--color-wi-text)]">
                                        {formatDate(dayGroup.date)}{" "}
                                        {formatTime(dayGroup.start_at)}-
                                        {formatTime(dayGroup.end_at)}
                                      </span>
                                    </label>
                                  </div>

                                  {selected && sitIn ? (
                                    <div className="mt-3 pl-7">
                                      {renderSitInModeToggle(
                                        sessionIds,
                                        firstSessionId,
                                      )}
                                      {(sitInModes[firstSessionId] ??
                                        "suggested") === "special" ? (
                                        renderSpecialSitInControls(
                                          sessionIds,
                                          firstSessionId,
                                        )
                                      ) : sitIn.sit_in_method === "physical" &&
                                        hasPriorities ? (
                                        (() => {
                                          const serverReveal =
                                            hasServerPriorityReveal(
                                              priorityGroup,
                                            );
                                          const nextLevelValue = serverReveal
                                            ? Boolean(sitIn.has_next_priority)
                                            : nextPriorityLevel(
                                                priorityGroup,
                                                currentLevel,
                                              ) !== null;
                                          const hasPreviousPriority =
                                            serverReveal
                                              ? Object.keys(
                                                  sitInPriorityHistory[
                                                    firstSessionId
                                                  ] ?? {},
                                                ).some(
                                                  (l) =>
                                                    Number(l) < currentLevel,
                                                )
                                              : previousPriorityLevel(
                                                  priorityGroup,
                                                  currentLevel,
                                                ) !== null;
                                          const revealing =
                                            revealingPrioritySessionIds.has(
                                              firstSessionId,
                                            );
                                          const available =
                                            currentPriorities.flatMap((p) =>
                                              availableSessionsForMissedSessions(
                                                p,
                                                sessionIds,
                                              ),
                                            );
                                          const unavailable =
                                            currentPriorities.flatMap((p) =>
                                              unavailableSessionsForMissedSession(
                                                p,
                                                firstSessionId,
                                              ),
                                            );
                                          const hasBlockedUnavailable = unavailable.some(
                                            (item) => item.reason_code === "sit_in_session_already_used",
                                          );

                                          if (
                                            available.length === 0 &&
                                            currentPriorities.length > 0 &&
                                            unavailable.length === 0
                                          ) {
                                            return (
                                              <div className="text-sm text-[var(--color-wi-text-light)]">
                                                <p className="font-medium">
                                                  No more options available
                                                </p>
                                                <p className="text-xs text-[var(--color-wi-text-light)] mt-0.5">
                                                  Admin will contact student to
                                                  arrange a make-up class.
                                                </p>
                                              </div>
                                            );
                                          }

                                          return (
                                            <div className="rounded-lg border border-wi-line bg-[var(--color-wi-row-alt)]/50 p-3">
                                              {(hasPreviousPriority ||
                                                nextLevelValue) && (
                                                <div className="mb-3 flex items-center gap-1.5">
                                                  {hasPreviousPriority && (
                                                    <button
                                                      type="button"
                                                      disabled={revealing}
                                                      onClick={() =>
                                                        handlePreviousPriority(
                                                          priorityGroup,
                                                          firstSessionId,
                                                        )
                                                      }
                                                      className="inline-flex h-7 items-center gap-1 rounded-full px-2.5 text-xs font-medium text-[var(--color-wi-text-light)] transition hover:bg-white hover:text-[var(--color-wi-text-light)] hover:shadow-sm"
                                                    >
                                                      <ChevronLeft className="h-3.5 w-3.5" />
                                                      Back
                                                    </button>
                                                  )}
                                                  {nextLevelValue && (
                                                    <button
                                                      type="button"
                                                      disabled={revealing}
                                                      onClick={() =>
                                                        void handleNotAvailable(
                                                          priorityGroup,
                                                          firstSessionId,
                                                        )
                                                      }
                                                      className="inline-flex h-7 items-center gap-1 rounded-full px-2.5 text-xs font-semibold text-[var(--color-wi-text-light)] transition hover:bg-white hover:text-[var(--color-wi-text-light)] hover:shadow-sm"
                                                    >
                                                      <span>
                                                        {revealing
                                                          ? "Loading..."
                                                          : "See other times"}
                                                      </span>
                                                      {!revealing && (
                                                        <ChevronRight className="h-3.5 w-3.5" />
                                                      )}
                                                    </button>
                                                  )}
                                                </div>
                                              )}
                                              {available.length > 0 ? (
                                                <MakeUpPicker
                                                  id={`staff-sit-in-${firstSessionId}`}
                                                  label="Make-up class"
                                                  value={currentSitIn}
                                                  options={makeUpPickerOptions(
                                                    sitInOptionsByTargetAndSession(
                                                      currentPriorities,
                                                      sessionIds,
                                                    ),
                                                    sessions,
                                                    selectedSubjectIds,
                                                    groupLabel,
                                                    sitIn?.sit_in_course,
                                                    [...Object.values(sitInSelections).flatMap(splitMergedSessionValue), ...Object.values(specialSitInSelections).flatMap((selection) => splitMergedSessionValue(selection.sessionValue))],
                                                    currentSitIn,
                                                  )}
                                                  onChange={(value) =>
                                                    handleSitInSelectForSessions(
                                                      sessionIds,
                                                      value,
                                                    )
                                                  }
                                                />
                                              ) : unavailable.length > 0 ? (
                                                <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700">
                                                  <p className="font-semibold">
                                                    {hasBlockedUnavailable
                                                      ? "This sit-in session is already used:"
                                                      : "Checked same-number slot:"}
                                                  </p>
                                                  <ul className="mt-1 space-y-1">
                                                    {unavailable.map(
                                                      (u, idx) => (
                                                        <li
                                                          key={`${u.reason_code}-${idx}`}
                                                        >
                                                          <span className="font-medium">
                                                            {getSitInSessionGroupLabel(
                                                              u.session
                                                                ? [u.session]
                                                                : [],
                                                              currentPriorities[0]
                                                                ?.sit_in_course,
                                                              groupLabel,
                                                              sessions,
                                                            )}
                                                          </span>
                                                          <span className="text-amber-600">
                                                            {" "}
                                                            — {u.reason}
                                                          </span>
                                                        </li>
                                                      ),
                                                    )}
                                                  </ul>
                                                </div>
                                              ) : null}
                                            </div>
                                          );
                                        })()
                                      ) : sitIn.sit_in_method === "physical" &&
                                        !hasPriorities ? (
                                        <div>
                                          <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-amber-600 mb-2">
                                            Pick a make-up class
                                          </div>
                                          <p className="mb-2 text-xs text-[var(--color-wi-text-light)] truncate">
                                            Sit-in:{" "}
                                            {(sitInAvailable.length > 0
                                              ? getSitInSessionGroupLabel(
                                                  sitInAvailable,
                                                  sitIn.sit_in_course,
                                                  groupLabel,
                                                  sessions,
                                                )
                                              : undefined) ||
                                              sitIn.sit_in_course?.name ||
                                              "To arrange"}
                                          </p>
                                          {sitInUnavailable.some((item) => item.reason_code === "sit_in_session_already_used") &&
                                          sitInAvailable.length === 0 ? (
                                            <div role="status" className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
                                              <p className="font-semibold">This sit-in session is already used.</p>
                                              <p className="mt-0.5 text-xs">Choose another sit-in session.</p>
                                            </div>
                                          ) : (
                                            <MakeUpPicker
                                              id={`staff-sit-in-${firstSessionId}`}
                                              label="Make-up class"
                                              value={currentSitIn}
                                              options={makeUpPickerOptions(
                                                sitInOptionGroupsBySession(
                                                  sitInAvailable,
                                                  sitIn.sit_in_course,
                                                ),
                                                sessions,
                                                selectedSubjectIds,
                                                groupLabel,
                                                sitIn.sit_in_course,
                                                [...Object.values(sitInSelections).flatMap(splitMergedSessionValue), ...Object.values(specialSitInSelections).flatMap((selection) => splitMergedSessionValue(selection.sessionValue))],
                                                currentSitIn,
                                              )}
                                              onChange={(value) =>
                                                handleSitInSelectForSessions(
                                                  sessionIds,
                                                  value,
                                                )
                                              }
                                            />
                                          )}
                                        </div>
                                      ) : sitIn.sit_in_method === "zoom" ? (
                                        <div className="flex items-start gap-2 text-sm text-[var(--color-wi-text-light)]">
                                          <span className="inline-flex h-5 w-5 items-center justify-center rounded-full bg-blue-100 text-[10px] font-bold text-blue-700">
                                            Z
                                          </span>
                                          <span>
                                            Online make-up (Zoom) — no session
                                            selection needed
                                          </span>
                                        </div>
                                      ) : (
                                        <div className="text-sm text-[var(--color-wi-text-light)]">
                                          <p className="font-medium">
                                            To arrange
                                          </p>
                                          <p className="text-xs text-[var(--color-wi-text-light)] mt-0.5">
                                            Admin will contact the student.
                                          </p>
                                        </div>
                                      )}
                                    </div>
                                  ) : null}
                                </div>
                              );
                            })}
                          </div>
                        </div>
                      );
                    })}
                </div>
              )}
              {selectedSessionCount > 0 ? (
                <p className="text-xs text-[var(--color-wi-text-light)]">
                  {selectedSessionCount} class day
                  {selectedSessionCount !== 1 ? "s" : ""} selected
                </p>
              ) : null}
            </>
          ) : null}
        </div>
      )}

      {/* Step 3: Confirm */}
      {step === "confirm" && (
        <div className="space-y-5">
          <h2 ref={headingRef} tabIndex={-1} className="sr-only">
            Step 3: Confirm and submit
          </h2>
          <div className="rounded-lg border border-wi-line bg-[var(--color-wi-row-alt)]/50 p-4 space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <span className="text-xs text-[var(--color-wi-text-light)]">Student</span>
                <p className="text-sm font-medium text-[var(--color-wi-text)]">
                  {student?.full_name} ({student?.wcode})
                </p>
              </div>
              <span className="text-xs text-[var(--color-wi-text-light)]">
                {selectedSubjectEntryCount} subject
                {selectedSubjectEntryCount !== 1 ? "s" : ""}
              </span>
            </div>

            {selectedSubjectBlocks.map((block) => {
                const selectedSessions = block.sessions
                  .filter(
                    (session) =>
                      selectedSessionIds.has(session.id) &&
                      !session.already_absent,
                  )
                  .sort((a, b) => a.start_at.localeCompare(b.start_at));
                if (selectedSessions.length === 0) return null;
                const groupLabel = appendTeacher(
                  block.label,
                  block.teacherName,
                );
                return (
                  <div key={block.key}>
                    <p className="text-sm font-semibold text-[var(--color-wi-text)]">
                      {groupLabel}
                    </p>
                    <div className="mt-1 space-y-1">
                      {groupByDay(selectedSessions).map((dayGroup) => (
                        <p key={dayGroup.id} className="text-xs text-[var(--color-wi-text-light)]">
                          {formatDate(dayGroup.date)}{" "}
                          {formatTime(dayGroup.start_at)}–
                          {formatTime(dayGroup.end_at)}
                          <span className="text-[var(--color-wi-text-light)]"> — Make-up: </span>
                          <span className="font-medium text-[var(--color-wi-text)]">
                            {getSpecialSitInReviewLabel(dayGroup.items[0].id) ??
                              getReviewSitInLabel(
                                dayGroup.items[0],
                                ownerGroupBySessionId.get(
                                  dayGroup.items[0].id,
                                ) ?? block.groups[0],
                                sitInSelections,
                                sitInPriorityLevels,
                                sitInPriorityHistory,
                                sessions,
                              )}
                          </span>
                        </p>
                      ))}
                    </div>
                  </div>
                );
              })}
          </div>

          {missingSitIn ? (
            <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700 flex items-start gap-2">
              <Info className="mt-0.5 h-4 w-4 shrink-0" />
              <span>
                Some classes don't have a make-up class selected. You can still
                create the absence.
              </span>
            </div>
          ) : null}

          <div className="field">
            <label
              htmlFor="staff-reason-category"
              className="mb-1.5 block text-sm font-medium text-[var(--color-wi-text-light)]"
            >
              Reason Category
            </label>
            <Select
              id="staff-reason-category"
              placeholder="Select a reason..."
              required
              aria-errormessage="reason-category-error"
              value={reasonCategory}
              onChange={(e) => setReasonCategory(e.target.value)}
              onBlur={(e) => syncAriaInvalid(e.currentTarget)}
              onInput={(e) => {
                if (e.currentTarget.checkValidity())
                  e.currentTarget.removeAttribute("aria-invalid");
              }}
            >
              {(formConfig?.form.reason_categories ?? []).map((cat) => (
                <option key={cat.value} value={cat.value}>
                  {cat.label}
                </option>
              ))}
            </Select>
            <p id="reason-category-error" className="error-msg" role="alert">
              Select a reason category
            </p>
          </div>

          <div>
            <label
              htmlFor="staff-reason"
              className="mb-1.5 block text-sm font-medium text-[var(--color-wi-text-light)]"
            >
              Additional details (optional)
            </label>
            <textarea
              id="staff-reason"
              className="w-full rounded-sm border border-wi-line px-3 py-2 text-sm"
              rows={3}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Optional note..."
            />
          </div>
        </div>
      )}
    </Modal>
  );
}
