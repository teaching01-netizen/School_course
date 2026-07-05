import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronRight, ChevronLeft, Info } from "lucide-react";
import { apiJson } from "../../api/client";
import { loadSessionsInRange } from "../../features/absences/api/absenceFormApi";
import { useToast } from "../../hooks/useToast";
import { formatDate, formatTime } from "../../utils/date";
import {
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
  getSitInSessionGroupLabel,
  getReviewSitInLabel,
} from "../../features/absences/domain/sitInResolution";
import { selectedSitInCourseIDForGroup } from "../../features/absences/domain/submissionPayload";
import type {
  SubjectSessions,
  StudentLookupResponse,
  AbsenceFormConfig,
  SmsPreview,
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

function specialSitInSessionsForGroup(
  group: SubjectSessions,
): SpecialSitInAvailableSession[] {
  const fromSitIn = group.sit_in?.available_sessions ?? [];
  if (fromSitIn.length > 0) return fromSitIn;
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
    for (const optGroup of groupByDay(specialSitInSessionsForGroup(group))) {
      const value = mergedSessionValue(optGroup.items);
      const courseId = optGroup.items.find(
        (session) => session.course_id,
      )?.course_id;
      if (!value || !courseId || seen.has(value)) continue;
      seen.add(value);
      options.push({
        value,
        courseId,
        label: getSitInSessionGroupLabel(
          optGroup.items,
          undefined,
          fallbackLabel,
          subjectGroups,
        ),
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
                    : "bg-gray-200 text-gray-400"
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
                className={`h-px w-6 transition-colors ${i < currentIdx ? "bg-emerald-300" : "bg-gray-200"}`}
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
        <div key={i} className="h-12 animate-pulse rounded-lg bg-gray-100" />
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

  const selectedSessionCount = useMemo(() => {
    let count = 0;
    for (const group of sessions) {
      if (!selectedSubjectIds.includes(group.subject_id)) continue;
      for (const dayGroup of groupByDay(group.sessions)) {
        if (isDayGroupSelected(dayGroup, selectedSessionIds)) count++;
      }
    }
    return count;
  }, [sessions, selectedSubjectIds, selectedSessionIds]);

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
    const enrolledSelectedIds = selectedSubjectIds.filter((subjectId) =>
      enrolledSubjectIds.has(subjectId),
    );
    const specialSelectedIds = selectedSubjectIds.filter(
      (subjectId) => !enrolledSubjectIds.has(subjectId),
    );
    const enrolledRequest =
      enrolledSelectedIds.length > 0
        ? loadSessionsInRange(
            student.wcode,
            "1970-01-01",
            "2100-01-01",
            { signal: controller.signal },
            { bypassTiming: true },
          )
        : Promise.resolve({ subjects: [] });
    const specialRequest =
      specialSelectedIds.length > 0
        ? loadSessionsInRange(
            student.wcode,
            "1970-01-01",
            "2100-01-01",
            { signal: controller.signal },
            {
              bypassTiming: true,
              includeAllSubjects: true,
              subjectIds: specialSelectedIds,
            },
          )
        : Promise.resolve({ subjects: [] });
    void Promise.all([enrolledRequest, specialRequest])
      .then(([enrolledData, specialData]) => {
        if (controller.signal.aborted) return;
        setSessions([
          ...(enrolledData.subjects ?? []),
          ...(specialData.subjects ?? []),
        ]);
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
  }, [step, student, selectedSubjectIds, enrolledSubjectIds]);

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

  function toggleSubject(subjectId: string) {
    setSelectedSubjectIds((current) =>
      current.includes(subjectId)
        ? current.filter((id) => id !== subjectId)
        : [...current, subjectId],
    );
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

  function specialSitInCourseIdsForMissedSessions(
    missedSessionIds: string[],
  ): Set<string> {
    const courseIds = new Set<string>();
    for (const missedSessionId of missedSessionIds) {
      if ((sitInModes[missedSessionId] ?? "suggested") !== "special") continue;
      const selection = specialSitInSelections[missedSessionId];
      if (!selection?.subjectId || !selection.sessionValue) continue;
      const option = findSpecialSitInSessionOption(
        specialSitInSessionsBySubject[selection.subjectId] ?? [],
        selection.sessionValue,
      );
      if (option?.courseId) courseIds.add(option.courseId);
    }
    return courseIds;
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
      <div className="mb-3 inline-flex rounded-md border border-gray-200 bg-gray-100 p-0.5">
        <button
          type="button"
          onClick={() => setSitInModeForSessions(sessionIds, "suggested")}
          className={`rounded px-2.5 py-1 text-xs font-semibold transition ${
            mode === "suggested"
              ? "bg-white text-gray-900 shadow-sm"
              : "text-gray-500 hover:text-gray-700"
          }`}
        >
          Suggested
        </button>
        <button
          type="button"
          onClick={() => setSitInModeForSessions(sessionIds, "special")}
          className={`rounded px-2.5 py-1 text-xs font-semibold transition ${
            mode === "special"
              ? "bg-white text-gray-900 shadow-sm"
              : "text-gray-500 hover:text-gray-700"
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
              className="mb-1 block text-xs font-medium text-gray-600"
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
              className="mb-1 block text-xs font-medium text-gray-600"
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
          <p className="mt-2 text-xs text-gray-500">
            No sessions found for this subject.
          </p>
        ) : null}
      </div>
    );
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
      setStep("confirm");
    }
  }

  function handleBack() {
    if (step === "sessions") setStep("subjects");
    else if (step === "confirm") setStep("sessions");
    else if (step === "subjects") setStep("type");
  }

  async function lookupStudent() {
    if (!wcode.trim()) return;
    setLookingUp(true);
    setStudent(null);
    setSelectedSubjectIds([]);
    clearSubjectOptions();
    clearSpecialSitInState();
    try {
      const data = await apiJson<StudentLookupResponse>(
        `/api/v1/absences/student-lookup?wcode=${encodeURIComponent(wcode.trim())}`,
        { method: "GET" },
      );
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
    setSubmitting(true);
    const created: string[] = [];

    for (const group of sessions) {
      if (!selectedSubjectIds.includes(group.subject_id)) continue;
      const selectedSessions = getSelectedSessionsForGroup(
        group,
        selectedSessionIds,
      );
      if (selectedSessions.length === 0) continue;
      const dates = [...new Set(selectedSessions.map((s) => s.date))].sort();
      const dateFrom = dates[0];
      const dateTo = dates[dates.length - 1];
      if (!dateFrom || !dateTo) continue;

      const missedIds = selectedSessions.map((s) => s.id);
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
      const specialSitInCourseIds =
        specialSitInCourseIdsForMissedSessions(missedIds);
      if (specialSitInCourseIds.size > 1) {
        addToast(
          "error",
          `${group.subject_name || group.course_code}: use one special sit-in course per absence`,
        );
        setSubmitting(false);
        return;
      }
      const specialSitInCourseId = [...specialSitInCourseIds][0];
      if (specialSitInCourseId) sitInMethod = "physical";
      const sitInCourseId =
        specialSitInCourseId ??
        selectedSitInCourseIDForGroup(
          group,
          missedIds,
          sitInSelections,
          sitInPriorityLevels,
          sitInPriorityHistory,
        ) ??
        group.course_id;

      try {
        const res = await apiJson<{ id: string; sms_preview?: SmsPreview }>(
          "/api/v1/absences/staff-create",
          {
            method: "POST",
            body: JSON.stringify({
              wcode: student.wcode,
              subject_id: group.subject_id,
              course_id: group.course_id,
              date_from: dateFrom,
              date_to: dateTo,
              missed_session_ids: missedIds,
              sit_in_method: sitInMethod,
              sit_in_course_id: sitInCourseId,
              sit_in_session_ids: uniqueSitInSessionIds,
              reason_category: reasonCategory || undefined,
              reason: reason || undefined,
              status: absenceType === "special" ? "special_approved" : undefined,
            }),
          },
        );
        created.push(res.id);
      } catch (err) {
        addToast(
          "error",
          `${group.subject_name || group.course_code}: ${err instanceof Error ? err.message : "Failed"}`,
        );
      }
    }

    setSubmitting(false);
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
        // fall through to toast
      }
    }
    addToast(
      "success",
      `${created.length} absence${created.length !== 1 ? "s" : ""} created`,
    );
    onCreated();
  }

  async function handleSendSms() {
    if (createdAbsenceIds.length === 0) {
      addToast("error", "Missing absence ID, cannot send SMS");
      return;
    }
    setSendingSms(true);
    try {
      const res = await apiJson<{ sent: boolean; recipient_count: number }>(
        "/api/v1/absences/batch-send-success-sms",
        {
          method: "POST",
          body: JSON.stringify({ ids: createdAbsenceIds }),
        },
      );
      if (!res.sent) {
        addToast("error", "SMS was not sent");
        return;
      }
      addToast(
        "success",
        `SMS notification sent to ${res.recipient_count} recipient${res.recipient_count !== 1 ? "s" : ""}`,
      );
      onCreated();
    } catch (err) {
      addToast(
        "error",
        err instanceof Error ? err.message : "Failed to send SMS",
      );
    } finally {
      setSendingSms(false);
    }
  }

  function handleSkipSms() {
    if (sendingSms) return;
    addToast("success", "Absence(s) created successfully (SMS skipped)");
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
          <p className="text-sm text-gray-600">
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
                  : "border-gray-200 hover:border-gray-300"
              }`}
            >
              <div className="flex items-center gap-3">
                <div className={`rounded-full p-2 ${
                  absenceType === "normal" ? "bg-blue-100" : "bg-gray-100"
                }`}>
                  <Info className={`h-5 w-5 ${
                    absenceType === "normal" ? "text-blue-600" : "text-gray-500"
                  }`} />
                </div>
                <div>
                  <p className="font-medium text-gray-900">Normal Absence</p>
                  <p className="text-sm text-gray-500">Requires review and approval</p>
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
                  : "border-gray-200 hover:border-gray-300"
              }`}
            >
              <div className="flex items-center gap-3">
                <div className={`rounded-full p-2 ${
                  absenceType === "special" ? "bg-purple-100" : "bg-gray-100"
                }`}>
                  <Info className={`h-5 w-5 ${
                    absenceType === "special" ? "text-purple-600" : "text-gray-500"
                  }`} />
                </div>
                <div>
                  <p className="font-medium text-gray-900">Special Absence</p>
                  <p className="text-sm text-gray-500">Pre-approved, skips review</p>
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
              className="mb-1.5 block text-sm font-medium text-gray-700"
            >
              Student W-Code
            </label>
            <div className="flex gap-2">
              <input
                id="staff-wcode"
                type="text"
                autoComplete="off"
                className="flex-1 rounded-sm border border-gray-300 px-3 py-2 text-sm"
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
                  {student.full_name}
                </p>
                <p className="text-xs text-emerald-600">{student.wcode}</p>
              </div>

              {student.subjects.length > 0 ? (
                <div>
                  <label className="mb-2 block text-sm font-medium text-gray-700">
                    Subjects
                  </label>
                  <p className="mb-2 text-xs text-gray-500">
                    Select one or more subjects
                  </p>
                  <div className="divide-y divide-gray-100 rounded-lg border border-gray-200 overflow-hidden">
                    {student.subjects.map((subject) => (
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
                  {selectedSubjectIds.length > 0 ? (
                    <p className="mt-2 text-xs text-gray-500">
                      {selectedSubjectIds.length} subject
                      {selectedSubjectIds.length !== 1 ? "s" : ""} selected
                    </p>
                  ) : null}
                </div>
              ) : (
                <p className="text-sm text-gray-500">
                  No enrolled subjects found for this student.
                </p>
              )}

              <div>
                <label
                  htmlFor="staff-special-subject"
                  className="mb-2 block text-sm font-medium text-gray-700"
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
                  <p className="mt-1.5 text-xs text-gray-500">
                    Use this when staff need to record an absence outside the
                    enrolled subject list.
                  </p>
                )}
                {specialSubjectIds.length > 0 ? (
                  <div className="mt-3 divide-y divide-gray-100 overflow-hidden rounded-lg border border-amber-200 bg-amber-50/40">
                    {specialSubjectIds.map((subjectId) => {
                      const subject = subjectById.get(subjectId);
                      if (!subject) return null;
                      return (
                        <SubjectCard
                          key={subject.id}
                          id={`special-${subject.id}`}
                          name={subject.name}
                          code={`${subject.code} · special case`}
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
              ) : sessions.filter((s) =>
                  selectedSubjectIds.includes(s.subject_id),
                ).length === 0 ? (
                <p className="text-sm text-gray-400">
                  No classes found for the selected subjects.
                </p>
              ) : (
                <div className="space-y-4">
                  {sessions
                    .filter((s) => selectedSubjectIds.includes(s.subject_id))
                    .map((group) => {
                      const sessionGroups = groupByDay(
                        group.sessions.filter((s) => !s.already_absent),
                      );
                      const groupLabel =
                        group.subject_name?.trim() ||
                        group.course_name?.trim() ||
                        group.course_code;
                      if (sessionGroups.length === 0) return null;
                      return (
                        <div
                          key={group.course_id}
                          className="rounded-lg border border-gray-200 bg-white overflow-hidden shadow-sm"
                        >
                          <div className="flex items-center justify-between gap-2 border-b border-gray-200 bg-gray-50/50 px-4 py-3">
                            <span className="text-sm font-semibold text-gray-900 truncate">
                              {groupLabel} ({sessionGroups.length} class day
                              {sessionGroups.length !== 1 ? "s" : ""})
                            </span>
                            <span className="text-xs font-semibold text-gray-500 shrink-0">
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
                              const sessionGroup =
                                groupWithSitInForMissedSession(
                                  group,
                                  firstSessionId,
                                );
                              const sitIn = sessionGroup.sit_in;
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
                                  className={`rounded-lg border px-4 py-3 transition-colors ${selected ? "border-blue-300 bg-blue-50/30" : "border-gray-200 bg-white"}`}
                                >
                                  <div className="flex items-center gap-3">
                                    <input
                                      type="checkbox"
                                      id={`staff-session-${dayGroup.id}`}
                                      checked={selected}
                                      onChange={() =>
                                        handleSessionGroupToggle(sessionIds)
                                      }
                                      className="h-4 w-4 shrink-0 rounded border-gray-300 text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20"
                                    />
                                    <label
                                      htmlFor={`staff-session-${dayGroup.id}`}
                                      className="min-w-0 cursor-pointer flex-1"
                                    >
                                      <span className="text-sm font-medium text-gray-900">
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

                                          if (
                                            available.length === 0 &&
                                            currentPriorities.length > 0 &&
                                            unavailable.length === 0
                                          ) {
                                            return (
                                              <div className="text-sm text-gray-500">
                                                <p className="font-medium">
                                                  No more options available
                                                </p>
                                                <p className="text-xs text-gray-400 mt-0.5">
                                                  Admin will contact student to
                                                  arrange a make-up class.
                                                </p>
                                              </div>
                                            );
                                          }

                                          return (
                                            <div className="rounded-lg border border-gray-200 bg-gray-50/50 p-3">
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
                                                      className="inline-flex h-7 items-center gap-1 rounded-full px-2.5 text-xs font-medium text-gray-500 transition hover:bg-white hover:text-gray-700 hover:shadow-sm"
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
                                                      className="inline-flex h-7 items-center gap-1 rounded-full px-2.5 text-xs font-semibold text-gray-500 transition hover:bg-white hover:text-gray-700 hover:shadow-sm"
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
                                                <div>
                                                  <label
                                                    className="mb-1 block text-xs font-medium text-gray-500"
                                                    htmlFor={`staff-sit-in-${firstSessionId}`}
                                                  >
                                                    Make-up class
                                                  </label>
                                                  <select
                                                    id={`staff-sit-in-${firstSessionId}`}
                                                    value={currentSitIn}
                                                    onChange={(e) =>
                                                      handleSitInSelectForSessions(
                                                        sessionIds,
                                                        e.target.value,
                                                      )
                                                    }
                                                    className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                                                  >
                                                    <option value="">
                                                      Not yet selected
                                                    </option>
                                                    {currentPriorities.flatMap(
                                                      (p) =>
                                                        groupByDay(
                                                          availableSessionsForMissedSessions(
                                                            p,
                                                            sessionIds,
                                                          ),
                                                        ).map((optGroup) => (
                                                          <option
                                                            key={`${p.sit_in_course?.id ?? "course"}:${optGroup.id}`}
                                                            value={mergedSessionValue(
                                                              optGroup.items,
                                                            )}
                                                          >
                                                            {getSitInSessionGroupLabel(
                                                              optGroup.items,
                                                              p.sit_in_course,
                                                              groupLabel,
                                                              sessions,
                                                            )}
                                                          </option>
                                                        )),
                                                    )}
                                                  </select>
                                                </div>
                                              ) : unavailable.length > 0 ? (
                                                <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700">
                                                  <p className="font-semibold">
                                                    Checked same-number slot:
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
                                          <p className="mb-2 text-xs text-gray-500 truncate">
                                            Sit-in:{" "}
                                            {getSitInSessionGroupLabel(
                                              rootAvailableSessionsForMissedSessions(
                                                sitIn,
                                                sessionIds,
                                              ),
                                              sitIn.sit_in_course,
                                              groupLabel,
                                              sessions,
                                            ) ||
                                              sitIn.sit_in_course?.name ||
                                              "To arrange"}
                                          </p>
                                          <select
                                            value={currentSitIn}
                                            onChange={(e) =>
                                              handleSitInSelectForSessions(
                                                sessionIds,
                                                e.target.value,
                                              )
                                            }
                                            className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
                                          >
                                            <option value="">
                                              — Not yet —
                                            </option>
                                            {groupByDay(
                                              rootAvailableSessionsForMissedSessions(
                                                sitIn,
                                                sessionIds,
                                              ),
                                            ).map((optGroup) => (
                                              <option
                                                key={optGroup.id}
                                                value={mergedSessionValue(
                                                  optGroup.items,
                                                )}
                                              >
                                                {getSitInSessionGroupLabel(
                                                  optGroup.items,
                                                  sitIn.sit_in_course,
                                                  groupLabel,
                                                  sessions,
                                                )}
                                              </option>
                                            ))}
                                          </select>
                                        </div>
                                      ) : sitIn.sit_in_method === "zoom" ? (
                                        <div className="flex items-start gap-2 text-sm text-gray-700">
                                          <span className="inline-flex h-5 w-5 items-center justify-center rounded-full bg-blue-100 text-[10px] font-bold text-blue-700">
                                            Z
                                          </span>
                                          <span>
                                            Online make-up (Zoom) — no session
                                            selection needed
                                          </span>
                                        </div>
                                      ) : (
                                        <div className="text-sm text-gray-500">
                                          <p className="font-medium">
                                            To arrange
                                          </p>
                                          <p className="text-xs text-gray-400 mt-0.5">
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
                <p className="text-xs text-gray-500">
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
          <div className="rounded-lg border border-gray-200 bg-gray-50/50 p-4 space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <span className="text-xs text-gray-500">Student</span>
                <p className="text-sm font-medium text-gray-900">
                  {student?.full_name} ({student?.wcode})
                </p>
              </div>
              <span className="text-xs text-gray-500">
                {selectedSubjectIds.length} subject
                {selectedSubjectIds.length !== 1 ? "s" : ""}
              </span>
            </div>

            {sessions
              .filter((s) => selectedSubjectIds.includes(s.subject_id))
              .map((group) => {
                const selectedSessions = getSelectedSessionsForGroup(
                  group,
                  selectedSessionIds,
                );
                if (selectedSessions.length === 0) return null;
                const groupLabel =
                  group.subject_name?.trim() ||
                  group.course_name?.trim() ||
                  group.course_code;
                return (
                  <div key={group.course_id}>
                    <p className="text-sm font-semibold text-gray-900">
                      {groupLabel}
                    </p>
                    <div className="mt-1 space-y-1">
                      {groupByDay(selectedSessions).map((dayGroup) => (
                        <p key={dayGroup.id} className="text-xs text-gray-600">
                          {formatDate(dayGroup.date)}{" "}
                          {formatTime(dayGroup.start_at)}–
                          {formatTime(dayGroup.end_at)}
                          <span className="text-gray-400"> — Make-up: </span>
                          <span className="font-medium text-gray-800">
                            {getSpecialSitInReviewLabel(dayGroup.items[0].id) ??
                              getReviewSitInLabel(
                                dayGroup.items[0],
                                group,
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
              className="mb-1.5 block text-sm font-medium text-gray-700"
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
              className="mb-1.5 block text-sm font-medium text-gray-700"
            >
              Additional details (optional)
            </label>
            <textarea
              id="staff-reason"
              className="w-full rounded-sm border border-gray-300 px-3 py-2 text-sm"
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
