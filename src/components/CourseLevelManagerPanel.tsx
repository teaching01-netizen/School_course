import { useEffect, useMemo, useRef, useState } from "react";
import { ExternalLink } from "lucide-react";
import { Link } from "react-router-dom";
import { apiJson } from "../api/client";
import type { CourseLevelItem, CourseMergeGroupConfig, RootCourseGroupInfo } from "../utils/levels";
import { useToast } from "../hooks/useToast";
import Button from "./ui/Button";
import Input from "./ui/Input";
import SearchableSelect from "./ui/SearchableSelect";
import TypeaheadSelect from "./TypeaheadSelect";
import ConfirmModal from "./ConfirmModal";
import type { SitInRule } from "../types";

const UNGROUPED_ID = "__ungrouped__";
const SIT_IN_RULES_HREF = "/admin/operations?tab=sit-in-rules";
const MAX_LEVEL = 32767;
const EMPTY_MERGE_GROUPS: CourseMergeGroupConfig[] = [];

type CourseLevelManagerPanelProps = {
  courses: CourseLevelItem[];
  groups: RootCourseGroupInfo[];
  rules: SitInRule[];
  mergeGroups?: CourseMergeGroupConfig[];
  rulesError?: string | null;
  rulesRetrying?: boolean;
  onRetryRules?: () => void;
  mergeConfigError?: string | null;
  mergeConfigRetrying?: boolean;
  onRetryMergeConfig?: () => void;
  onCoursesChange: (update: (courses: CourseLevelItem[]) => CourseLevelItem[]) => void;
  onMergeGroupsChange: (update: (mergeGroups: CourseMergeGroupConfig[]) => CourseMergeGroupConfig[]) => void;
  onGroupsChange: (update: (groups: RootCourseGroupInfo[]) => RootCourseGroupInfo[]) => void;
};

type LevelStatus = "ready" | "attention" | "empty";

type EffectiveCourse = {
  groupCourse: CourseLevelItem | null;
  mergeGroup: CourseMergeGroupConfig | null;
  displayLevel: number | null;
  cycleId: string | null;
  cycleLabel: string;
  canEditLevel: boolean;
  readOnlyReason: string | null;
  canonicalKey: string;
  /** Course that owns the authoritative cycle for this row (the link source when one exists). */
  setupCourseId: string;
  /** A level is set but no cycle is, so readiness cannot be asserted. */
  missingCycle: boolean;
};

type EffectiveEntry = {
  key: string;
  courseId: string;
  code: string;
  subjectId: string;
  subjectCode: string;
  subjectName: string;
  rootGroupId: string | null;
  cycleId: string | null;
  cycleLabel: string;
  level: number | null;
  mergeGroupId: string | null;
  /** Course that owns the authoritative cycle for this entry (the link source when one exists). */
  setupCourseId: string;
  missingCycle: boolean;
};

type LevelConflict = {
  code: string;
  subjectName: string;
  cycleLabel: string;
};

function levelDraftKey(course: CourseLevelItem, row: EffectiveCourse | undefined): string {
  return row?.mergeGroup?.rule_source_course_code ? row.canonicalKey : course.id;
}

function levelStatus(level: number | null): { label: string; className: string } {
  if (level === null) return { label: "Not set", className: "border-wi-line bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text)]" };
  if (level === 1) return { label: "Zoom", className: "border-[var(--color-wi-primary)]/20 bg-[var(--color-wi-callout)] text-[var(--color-wi-primary)]" };
  return { label: "Eligible", className: "border-[var(--color-wi-green)]/20 bg-[var(--color-wi-callout)] text-[var(--color-wi-green-dark)]" };
}

function statusDot(status: LevelStatus): string {
  if (status === "ready") return "bg-[var(--color-wi-green)]";
  if (status === "attention") return "bg-[var(--color-wi-amber)]";
  return "bg-[var(--color-wi-faint)]";
}

function scopeKey(rootGroupId: string | null, subjectId: string, cycleId: string | null): string {
  return `${rootGroupId ?? UNGROUPED_ID}|${subjectId}|${cycleId ?? "__no_cycle__"}`;
}

function countGaps(entries: EffectiveEntry[]): number {
  const scopes = new Map<string, number[]>();
  for (const entry of entries) {
    if (entry.level === null) continue;
    const key = scopeKey(entry.rootGroupId, entry.subjectId, entry.cycleId);
    const levels = scopes.get(key) ?? [];
    levels.push(entry.level);
    scopes.set(key, levels);
  }
  let gaps = 0;
  for (const levels of scopes.values()) {
    const unique = [...new Set(levels)].sort((a, b) => a - b);
    if (unique.length < 2) continue;
    gaps += unique[unique.length - 1] - unique[0] + 1 - unique.length;
  }
  return gaps;
}

function StatusBadge({ level }: { level: number | null }) {
  const status = levelStatus(level);
  return (
    <span aria-label={`Level status: ${status.label}`} className={`inline-flex items-center gap-1.5 rounded-full border px-2 py-1 text-xs font-medium ${status.className}`}>
      <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
      {status.label}
    </span>
  );
}

function SummaryMetric({ label, value, tone }: { label: string; value: number; tone: "blue" | "green" | "amber" | "red" }) {
  const toneClass = { blue: "bg-[var(--color-wi-primary)]", green: "bg-[var(--color-wi-green)]", amber: "bg-[var(--color-wi-amber)]", red: "bg-[var(--color-wi-red)]" }[tone];
  return (
    <div role="group" aria-label={`${label}: ${value}`} className="flex items-center gap-2 border-r border-wi-line px-2 last:border-r-0">
      <span className={`h-2 w-2 shrink-0 rounded-full ${toneClass}`} aria-hidden="true" />
      <div>
        <p className="text-base font-semibold tabular-nums text-[var(--color-wi-text)]">{value}</p>
        <p className="text-xs text-[var(--color-wi-text-light)]">{label}</p>
      </div>
    </div>
  );
}

export default function CourseLevelManagerPanel({
  courses,
  groups,
  rules,
  mergeGroups = EMPTY_MERGE_GROUPS,
  rulesError = null,
  rulesRetrying = false,
  onRetryRules,
  mergeConfigError = null,
  mergeConfigRetrying = false,
  onRetryMergeConfig,
  onCoursesChange,
  onMergeGroupsChange,
  onGroupsChange,
}: CourseLevelManagerPanelProps) {
  const { addToast } = useToast();
  const [selectedGroupId, setSelectedGroupId] = useState("");
  const [newGroupName, setNewGroupName] = useState("");
  const [editingGroupId, setEditingGroupId] = useState<string | null>(null);
  const [editingGroupName, setEditingGroupName] = useState("");
  const [groupError, setGroupError] = useState<string | null>(null);
  const [savingGroup, setSavingGroup] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<RootCourseGroupInfo | null>(null);
  const [deletingGroup, setDeletingGroup] = useState(false);
  const [savingCourseId, setSavingCourseId] = useState<string | null>(null);
  const [savingSitInRule, setSavingSitInRule] = useState(false);
  const [ruleError, setRuleError] = useState<string | null>(null);
  const [draftLevels, setDraftLevels] = useState<Record<string, string>>({});
  const [rowErrors, setRowErrors] = useState<Record<string, string>>({});
  const [newLevelCourseId, setNewLevelCourseId] = useState("");
  const [newLevel, setNewLevel] = useState("");
  const persistedRef = useRef<Map<string, string>>(new Map());

  const effective = useMemo(() => {
    const mergeByCode = new Map<string, CourseMergeGroupConfig>();
    for (const group of mergeGroups) {
      for (const code of group.course_codes) mergeByCode.set(code, group);
    }
    const courseByCode = new Map(courses.map((course) => [course.code, course]));
    const rows = new Map<string, EffectiveCourse>();
    const entries: EffectiveEntry[] = [];
    const seenMergeScopes = new Set<string>();

    for (const course of courses) {
      const mergeGroup = mergeByCode.get(course.code) ?? null;
      if (!mergeGroup) {
        const cycleId = course.cycle_id || null;
        rows.set(course.id, {
          groupCourse: course,
          mergeGroup: null,
          displayLevel: course.level,
          cycleId,
          cycleLabel: course.cycle_label || "No cycle",
          canEditLevel: Boolean(cycleId),
          readOnlyReason: cycleId ? null : "Add a cycle to this course before setting its level.",
          canonicalKey: `course:${course.id}`,
          setupCourseId: course.id,
          missingCycle: course.level !== null && !cycleId,
        });
        entries.push({
          key: `course:${course.id}`,
          courseId: course.id,
          code: course.code,
          subjectId: course.subject_id,
          subjectCode: course.subject_code,
          subjectName: course.subject_name,
          rootGroupId: course.root_course_group_id,
          cycleId,
          cycleLabel: course.cycle_label || "No cycle",
          level: course.level,
          mergeGroupId: null,
          setupCourseId: course.id,
          missingCycle: course.level !== null && !cycleId,
        });
        continue;
      }

      const isContinuation = Boolean(mergeGroup.rule_source_course_code);
      const sourceCourse = mergeGroup.rule_source_course_code ? courseByCode.get(mergeGroup.rule_source_course_code) ?? null : null;
      const canonical = sourceCourse ?? courseByCode.get(mergeGroup.course_codes[0] ?? "") ?? course;
      const groupCourse = isContinuation ? sourceCourse : course;
      // The course list returns each member's raw level/cycle, while the merge
      // config returns one resolved effective value. For a continuation link the
      // config cycle is read from the source course (backend/internal/db/
      // course_groups_custom.go CourseMergeGroupConfigsList), so a null cycle means
      // the source has no cycle. Falling back to the member's cycle would invent one.
      const cycleId = mergeGroup.cycle_id ?? null;
      const cycleLabel = mergeGroup.cycle_label || "No cycle";
      const readOnlyReason = isContinuation
        ? null
        : "Level and cycle are managed by the merged course configuration in Sit-in Rules.";
      rows.set(course.id, {
        groupCourse,
        mergeGroup,
        displayLevel: mergeGroup.level,
        cycleId,
        cycleLabel,
        canEditLevel: isContinuation,
        readOnlyReason,
        canonicalKey: `merge:${mergeGroup.id}`,
        setupCourseId: canonical.id,
        missingCycle: mergeGroup.level !== null && !cycleId,
      });

      const readinessCourse = groupCourse ?? canonical;
      const readinessKey = isContinuation
        ? `merge:${mergeGroup.id}`
        : `merge:${mergeGroup.id}:${scopeKey(readinessCourse.root_course_group_id, readinessCourse.subject_id, cycleId)}`;
      if (!seenMergeScopes.has(readinessKey)) {
        seenMergeScopes.add(readinessKey);
        entries.push({
          key: readinessKey,
          courseId: readinessCourse.id,
          code: readinessCourse.code,
          subjectId: readinessCourse.subject_id,
          subjectCode: readinessCourse.subject_code,
          subjectName: readinessCourse.subject_name,
          rootGroupId: readinessCourse.root_course_group_id,
          cycleId,
          cycleLabel,
          level: mergeGroup.level,
          mergeGroupId: mergeGroup.id,
          setupCourseId: canonical.id,
          missingCycle: mergeGroup.level !== null && !cycleId,
        });
      }
    }

    return { rows, entries };
  }, [courses, mergeGroups]);

  const coursesByGroup = useMemo(() => {
    const grouped = new Map<string, CourseLevelItem[]>();
    for (const course of courses) {
      const groupCourse = effective.rows.get(course.id)?.groupCourse ?? course;
      const groupId = groupCourse.root_course_group_id ?? UNGROUPED_ID;
      const members = grouped.get(groupId) ?? [];
      members.push(course);
      grouped.set(groupId, members);
    }
    return grouped;
  }, [courses, effective]);

  const groupOptions = useMemo(() => [
    ...groups.map((group) => ({ id: group.id, name: group.name, courseCount: coursesByGroup.get(group.id)?.length ?? 0, virtual: false })),
    { id: UNGROUPED_ID, name: "Unassigned", courseCount: coursesByGroup.get(UNGROUPED_ID)?.length ?? 0, virtual: true },
  ], [coursesByGroup, groups]);

  const selectedGroup = groupOptions.find((group) => group.id === selectedGroupId) ?? groupOptions[0] ?? null;
  const selectedRootGroup = selectedGroup && !selectedGroup.virtual ? groups.find((group) => group.id === selectedGroup.id) ?? null : null;
  const selectedCourses = useMemo(() => {
    if (!selectedGroup) return [];
    return [...(coursesByGroup.get(selectedGroup.id) ?? [])].sort((a, b) => {
      const subject = a.subject_code.localeCompare(b.subject_code);
      if (subject !== 0) return subject;
      const cycleA = effective.rows.get(a.id)?.cycleLabel ?? a.cycle_label;
      const cycleB = effective.rows.get(b.id)?.cycleLabel ?? b.cycle_label;
      const cycle = cycleA.localeCompare(cycleB);
      if (cycle !== 0) return cycle;
      const levelA = effective.rows.get(a.id)?.displayLevel ?? a.level;
      const levelB = effective.rows.get(b.id)?.displayLevel ?? b.level;
      if (levelA === null && levelB !== null) return 1;
      if (levelA !== null && levelB === null) return -1;
      if (levelA !== null && levelB !== null && levelA !== levelB) return levelA - levelB;
      return a.code.localeCompare(b.code);
    });
  }, [coursesByGroup, effective, selectedGroup]);
  const addableCourses = useMemo(
    () => selectedCourses.filter((course) => {
      const row = effective.rows.get(course.id);
      return !row?.mergeGroup && row?.displayLevel === null && row?.canEditLevel;
    }),
    [effective, selectedCourses],
  );

  useEffect(() => {
    if (selectedGroupId && groupOptions.some((group) => group.id === selectedGroupId)) return;
    setSelectedGroupId(groupOptions[0]?.id ?? UNGROUPED_ID);
  }, [groupOptions, selectedGroupId]);

  useEffect(() => {
    const previousPersisted = persistedRef.current;
    const nextPersisted = new Map<string, string>();
    for (const course of courses) {
      const row = effective.rows.get(course.id);
      nextPersisted.set(levelDraftKey(course, row), row?.displayLevel?.toString() ?? "");
    }
    persistedRef.current = nextPersisted;
    setDraftLevels((previous) => {
      const next: Record<string, string> = {};
      for (const course of courses) {
        const row = effective.rows.get(course.id);
        const key = levelDraftKey(course, row);
        const baseline = nextPersisted.get(key) ?? "";
        const previousBaseline = previousPersisted.get(key);
        const draft = previous[key];
        // A draft that predates the first baseline (for example, typed before the
        // initial effect flushed) is still a user edit and must not be clobbered.
        const dirty = draft !== undefined && (previousBaseline === undefined || draft !== previousBaseline);
        next[key] = row?.canEditLevel && dirty ? draft : baseline;
      }
      return next;
    });
  }, [courses, effective]);

  const selectedGroupNeedsSitInRule = Boolean(selectedRootGroup && !selectedRootGroup.sit_in_rule_id);
  const canEditLevels = Boolean(selectedGroup && !selectedGroup.virtual && selectedRootGroup?.sit_in_rule_id && !mergeConfigError);
  const editingBlockedReason = mergeConfigError
    ? "Linked course settings are unavailable, so level editing is disabled until they load."
    : !selectedGroup || selectedGroup.virtual
      ? "Move courses into a course group with a sit-in rule before setting levels."
      : !selectedRootGroup?.sit_in_rule_id
        ? "Assign a sit-in rule before adding or editing course levels."
        : null;
  const selectedSitInRule = rules.find((rule) => rule.id === selectedRootGroup?.sit_in_rule_id) ?? null;
  const currentRuleUnavailable = Boolean(selectedRootGroup?.sit_in_rule_id && !selectedSitInRule);

  function groupStatusFor(group: { id: string; virtual: boolean }): { status: LevelStatus; label: string; gaps: number } {
    const rootGroupId = group.virtual ? null : group.id;
    const entries = effective.entries.filter((entry) => entry.rootGroupId === rootGroupId);
    if (entries.length === 0) return { status: "empty", label: "Empty", gaps: 0 };
    const gaps = countGaps(entries);
    const hasUnset = entries.some((entry) => entry.level === null);
    const hasMissingCycle = entries.some((entry) => entry.missingCycle);
    if (group.virtual) return { status: "attention", label: "Needs group", gaps };
    // Linked effective values are unavailable, so readiness cannot be asserted.
    if (mergeConfigError) return { status: "attention", label: "Needs attention", gaps };
    const hasRule = Boolean(groups.find((item) => item.id === group.id)?.sit_in_rule_id);
    if (!hasRule) return { status: "attention", label: "Rule needed", gaps };
    if (hasUnset || hasMissingCycle || gaps > 0) return { status: "attention", label: "Needs attention", gaps };
    return { status: "ready", label: "Ready", gaps: 0 };
  }

  const summary = useMemo(() => {
    const assigned = courses.reduce((total, course) => total + ((effective.rows.get(course.id)?.displayLevel ?? null) !== null ? 1 : 0), 0);
    const notSet = courses.length - assigned;
    return {
      totalCourses: courses.length,
      assigned,
      gaps: countGaps(effective.entries),
      notSet,
    };
  }, [courses, effective]);

  const dirtyCount = (courseList: CourseLevelItem[]) => new Set(courseList.filter((course) => {
    const row = effective.rows.get(course.id);
    const key = levelDraftKey(course, row);
    const baseline = row?.displayLevel?.toString() ?? "";
    return (draftLevels[key] ?? baseline) !== baseline;
  }).map((course) => levelDraftKey(course, effective.rows.get(course.id)))).size;
  const selectedDirtyCount = dirtyCount(selectedCourses);

  function clearRowError(courseId: string) {
    setRowErrors((previous) => {
      if (!(courseId in previous)) return previous;
      const next = { ...previous };
      delete next[courseId];
      return next;
    });
  }

  function setRowError(courseId: string, message: string) {
    setRowErrors((previous) => ({ ...previous, [courseId]: message }));
  }

  function selectGroup(groupId: string) {
    setSelectedGroupId(groupId);
    setNewLevelCourseId("");
    setNewLevel("");
    setGroupError(null);
    setRuleError(null);
  }

  // The database unique index is courses(subject_id, cycle_id, level) across the
  // whole courses table (migration 00017), so duplicate preflight must ignore root
  // course groups. Continuation levels use the link's effective rows and skip its
  // own merged entry when checking for duplicates.
  function conflictFor(course: CourseLevelItem, level: number, continuation: CourseMergeGroupConfig | null = null): LevelConflict | null {
    const row = effective.rows.get(course.id);
    const cycleId = continuation ? row?.cycleId ?? null : course.cycle_id || null;
    if (!cycleId) return null;
    if (continuation) {
      const conflict = effective.entries.find((entry) =>
        entry.mergeGroupId !== continuation.id
        && entry.subjectId === course.subject_id
        && entry.cycleId === cycleId
        && entry.level === level
      );
      if (!conflict) return null;
      return { code: conflict.code, subjectName: conflict.subjectName, cycleLabel: conflict.cycleLabel };
    }
    const conflict = courses.find((item) =>
      item.id !== course.id
      && item.subject_id === course.subject_id
      && (item.cycle_id || null) === cycleId
      && item.level === level
    );
    if (!conflict) return null;
    return {
      code: conflict.code,
      subjectName: conflict.subject_name,
      cycleLabel: conflict.cycle_label || "No cycle",
    };
  }

  function nextLevelFor(course: CourseLevelItem): number {
    const row = effective.rows.get(course.id);
    const key = scopeKey(course.root_course_group_id, course.subject_id, row?.cycleId ?? course.cycle_id);
    const levels = effective.entries
      .filter((entry) => scopeKey(entry.rootGroupId, entry.subjectId, entry.cycleId) === key && entry.level !== null)
      .map((entry) => entry.level as number);
    const highest = levels.length > 0 ? Math.max(...levels) : 0;
    return Math.min(MAX_LEVEL, highest + 1);
  }

  async function saveLevel(course: CourseLevelItem, rawLevel: string): Promise<boolean> {
    const row = effective.rows.get(course.id);
    if (!row) return false;
    const continuation = row.mergeGroup?.rule_source_course_code ? row.mergeGroup : null;
    if (mergeConfigError) {
      setRowError(course.id, "Linked course settings are unavailable. Retry before editing levels.");
      return false;
    }
    if (!row.canEditLevel) {
      setRowError(course.id, row.readOnlyReason ?? "This course's level is managed elsewhere.");
      return false;
    }
    if (!continuation && !row.cycleId) {
      setRowError(course.id, "Add a cycle to this course before setting its level.");
      return false;
    }
    const groupCourse = row.groupCourse ?? course;
    const courseGroup = groupCourse.root_course_group_id ? groups.find((group) => group.id === groupCourse.root_course_group_id) : null;
    if (!courseGroup) {
      setRowError(course.id, "Move this course into a course group with a sit-in rule before setting a level.");
      return false;
    }
    if (!courseGroup.sit_in_rule_id) {
      setRowError(course.id, "Assign a sit-in rule to this course group before saving a level.");
      return false;
    }

    const trimmed = rawLevel.trim();
    if (trimmed === "") {
      if (row.displayLevel !== null) {
        setRowError(course.id, "Levels cannot be removed from a levelled course. Move it to another course group instead.");
        return false;
      }
      return true;
    }

    const level = Number(trimmed);
    if (!Number.isInteger(level) || level < 1 || level > MAX_LEVEL) {
      setRowError(course.id, `Level must be a whole number between 1 and ${MAX_LEVEL}.`);
      return false;
    }
    const conflict = conflictFor(course, level, continuation);
    if (conflict) {
      setRowError(course.id, `Level ${level} is already used by ${conflict.code} in ${conflict.subjectName} (${conflict.cycleLabel}). Choose a different level.`);
      return false;
    }

    setSavingCourseId(course.id);
    setRowErrors((previous) => {
      if (!(course.id in previous)) return previous;
      const next = { ...previous };
      delete next[course.id];
      return next;
    });
    try {
      if (continuation) {
        await apiJson(`/api/v1/admin/course-merge-groups/${continuation.id}/level`, {
          method: "PUT",
          body: JSON.stringify({ level }),
        });
        onMergeGroupsChange((previous) => previous.map((group) => group.id === continuation.id ? { ...group, level } : group));
      } else {
        await apiJson(`/api/v1/admin/courses/${course.id}/level`, {
          method: "PUT",
          body: JSON.stringify({ level, cycle_id: course.cycle_id }),
        });
        onCoursesChange((previous) => previous.map((item) => item.id === course.id ? { ...item, level } : item));
      }
      const draftKey = levelDraftKey(course, row);
      persistedRef.current.set(draftKey, level.toString());
      setDraftLevels((previous) => ({ ...previous, [draftKey]: level.toString() }));
      addToast("success", continuation ? `${continuation.name} level saved for both linked courses` : `${course.code} level saved`);
      return true;
    } catch (error) {
      setRowError(course.id, error instanceof Error ? error.message : "Failed to save level");
      addToast("error", error instanceof Error ? error.message : "Failed to save level");
      return false;
    } finally {
      setSavingCourseId(null);
    }
  }

  async function saveSitInRule(ruleId: string | null): Promise<boolean> {
    if (!selectedRootGroup) return false;
    if (!ruleId) {
      setRuleError("A sit-in rule is required for levelled course groups.");
      return false;
    }
    setSavingSitInRule(true);
    setRuleError(null);
    try {
      await apiJson(`/api/v1/admin/root-course-groups/${selectedRootGroup.id}`, {
        method: "PUT",
        body: JSON.stringify({ sit_in_rule_id: ruleId }),
      });
      onGroupsChange((previous) => previous.map((group) => group.id === selectedRootGroup.id ? { ...group, sit_in_rule_id: ruleId } : group));
      addToast("success", "Sit-in rule assigned");
      return true;
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to update sit-in rule";
      setRuleError(message);
      addToast("error", message);
      return false;
    } finally {
      setSavingSitInRule(false);
    }
  }

  async function addLevel() {
    const course = selectedCourses.find((item) => item.id === newLevelCourseId);
    if (!course) {
      setRowErrors((previous) => ({ ...previous, __add__: "Choose a course before adding a level." }));
      return;
    }
    const saved = await saveLevel(course, newLevel);
    if (!saved) return;
    setNewLevelCourseId("");
    setNewLevel("");
    clearRowError("__add__");
  }

  async function createGroup() {
    const name = newGroupName.trim();
    if (!name) return;
    setSavingGroup(true);
    setGroupError(null);
    try {
      const created = await apiJson<RootCourseGroupInfo>("/api/v1/admin/root-course-groups", {
        method: "POST",
        body: JSON.stringify({ name }),
      });
      onGroupsChange((previous) => [...previous, { ...created, course_count: created.course_count ?? 0 }]);
      setNewGroupName("");
      setSelectedGroupId(created.id);
      setNewLevelCourseId("");
      setNewLevel("");
      addToast("success", "Course group added");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to add course group";
      setGroupError(message);
      addToast("error", message);
    } finally {
      setSavingGroup(false);
    }
  }

  async function renameGroup(groupId: string) {
    const name = editingGroupName.trim();
    if (!name) return;
    setSavingGroup(true);
    setGroupError(null);
    try {
      await apiJson(`/api/v1/admin/root-course-groups/${groupId}`, { method: "PUT", body: JSON.stringify({ name }) });
      onGroupsChange((previous) => previous.map((group) => group.id === groupId ? { ...group, name } : group));
      onCoursesChange((previous) => previous.map((course) => course.root_course_group_id === groupId ? { ...course, root_course_group_name: name } : course));
      setEditingGroupId(null);
      addToast("success", "Course group renamed");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to rename course group";
      setGroupError(message);
      addToast("error", message);
    } finally {
      setSavingGroup(false);
    }
  }

  function requestDeleteGroup(group: RootCourseGroupInfo) {
    const count = courses.filter((course) => course.root_course_group_id === group.id).length;
    if (count > 0) {
      setGroupError(coursesByGroup.get(group.id)?.length
        ? `Move ${count} course${count === 1 ? "" : "s"} out first. Course groups can only be deleted after every course is moved or unassigned.`
        : "This group is still saved as an original assignment on a continuation course. It cannot be deleted while that assignment exists.");
      return;
    }
    setGroupError(null);
    setDeleteTarget(group);
  }

  async function deleteGroup() {
    if (!deleteTarget) return;
    setDeletingGroup(true);
    setGroupError(null);
    try {
      await apiJson(`/api/v1/admin/root-course-groups/${deleteTarget.id}`, { method: "DELETE" });
      onGroupsChange((previous) => previous.filter((group) => group.id !== deleteTarget.id));
      onCoursesChange((previous) => previous.map((course) => course.root_course_group_id === deleteTarget.id ? { ...course, root_course_group_id: null, root_course_group_name: null } : course));
      setSelectedGroupId(UNGROUPED_ID);
      setNewLevelCourseId("");
      setNewLevel("");
      addToast("success", "Course group deleted");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to delete course group";
      setGroupError(message);
      addToast("error", message);
    } finally {
      setDeletingGroup(false);
      setDeleteTarget(null);
    }
  }

  async function assignCourse(courseId: string) {
    if (!selectedGroup || selectedGroup.virtual || savingCourseId || mergeConfigError) return;
    const row = effective.rows.get(courseId);
    const course = row?.groupCourse;
    if (!course || course.root_course_group_id === selectedGroup.id) return;
    if ((row.displayLevel ?? course.level) !== null && !selectedRootGroup?.sit_in_rule_id) {
      addToast("error", "Assign a sit-in rule before adding a levelled course to this group");
      return;
    }
    setSavingCourseId(course.id);
    try {
      await apiJson(`/api/v1/admin/courses/${course.id}/root-course-group`, { method: "PUT", body: JSON.stringify({ root_course_group_id: selectedGroup.id }) });
      onCoursesChange((previous) => previous.map((item) => item.id === course.id ? { ...item, root_course_group_id: selectedGroup.id, root_course_group_name: selectedGroup.name } : item));
      addToast("success", `${course.code} added to ${selectedGroup.name}`);
    } catch (error) {
      addToast("error", error instanceof Error ? error.message : "Failed to assign course");
    } finally {
      setSavingCourseId(null);
    }
  }

  async function moveCourse(course: CourseLevelItem, groupId: string) {
    const row = effective.rows.get(course.id);
    if (row?.mergeGroup) {
      setRowError(course.id, "Linked courses cannot be moved between course groups. Manage the link in Sit-in Rules.");
      return;
    }
    if (mergeConfigError) {
      setRowError(course.id, "Linked course settings are unavailable. Retry before moving courses.");
      return;
    }
    const nextGroup = groups.find((group) => group.id === groupId) ?? null;
    const nextGroupId = groupId || null;
    if (course.root_course_group_id === nextGroupId) return;
    if (!nextGroupId && (row?.displayLevel ?? course.level) !== null) {
      setRowError(course.id, "Levelled courses cannot be left unassigned. Move this course to another group with a sit-in rule instead.");
      return;
    }
    if ((row?.displayLevel ?? course.level) !== null && nextGroup && !nextGroup.sit_in_rule_id) {
      setRowError(course.id, "Assign a sit-in rule before moving a levelled course into this group.");
      return;
    }
    setSavingCourseId(course.id);
    clearRowError(course.id);
    try {
      await apiJson(`/api/v1/admin/courses/${course.id}/root-course-group`, { method: "PUT", body: JSON.stringify({ root_course_group_id: nextGroupId }) });
      onCoursesChange((previous) => previous.map((item) => item.id === course.id ? { ...item, root_course_group_id: nextGroupId, root_course_group_name: nextGroup?.name ?? null } : item));
      addToast("success", `${course.code} group updated`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to update course group";
      setRowError(course.id, message);
      addToast("error", message);
    } finally {
      setSavingCourseId(null);
    }
  }

  function handleNewLevelCourseChange(value: string) {
    setNewLevelCourseId(value);
    clearRowError("__add__");
    const course = selectedCourses.find((item) => item.id === value);
    setNewLevel(course ? String(nextLevelFor(course)) : "");
  }

  const courseOptions = courses
    .flatMap((course) => {
      const row = effective.rows.get(course.id);
      const groupCourse = row?.groupCourse;
      if (!groupCourse || groupCourse.root_course_group_id === selectedGroup?.id) return [];
      const linkedNote = row.mergeGroup?.rule_source_course_code ? " (moves both linked courses)" : "";
      return [{
        value: course.id,
        label: `${course.code} — ${course.name}${linkedNote}`,
        keywords: `${course.code} ${course.name} ${course.subject_code} ${course.subject_name}`,
      }];
    });
  const groupCourseOptions = [{ value: "", label: "— Unassigned —" }, ...groups.map((group) => ({ value: group.id, label: group.name }))];
  const selectedGroupDetails = selectedGroup ? groupStatusFor(selectedGroup) : { status: "empty" as LevelStatus, label: "Empty", gaps: 0 };
  const selectedEntries = selectedGroup && !selectedGroup.virtual
    ? effective.entries.filter((entry) => entry.rootGroupId === selectedGroup.id)
    : [];
  const selectedMissingCycleEntry = selectedEntries.find((entry) => entry.missingCycle) ?? null;
  const selectedLinkedCount = selectedCourses.filter((course) => effective.rows.get(course.id)?.mergeGroup).length;
  const selectedHasContinuation = selectedCourses.some((course) => Boolean(effective.rows.get(course.id)?.mergeGroup?.rule_source_course_code));
  const addError = rowErrors.__add__ ?? (newLevelCourseId ? rowErrors[newLevelCourseId] : undefined) ?? null;

  return (
    <div className="space-y-4">
      {mergeConfigError ? (
        <div role="alert" className="rounded-sm border border-[var(--color-wi-amber)]/40 bg-[var(--color-wi-amber-bg)] px-3 py-3 text-sm">
          <p className="font-semibold text-[var(--color-wi-text)]">Linked course settings could not be loaded.</p>
          <p className="mt-1 text-[var(--color-wi-text-light)]">
            Level editing is disabled and readiness is not shown, because a linked course's effective level and cycle are unknown until this list loads. Existing rule assignments are preserved.
            {mergeConfigError ? ` ${mergeConfigError}` : ""}
          </p>
          {onRetryMergeConfig ? (
            <Button variant="secondary" size="sm" className="mt-2" loading={mergeConfigRetrying} onClick={onRetryMergeConfig}>
              Retry linked course settings
            </Button>
          ) : null}
        </div>
      ) : null}
      {rulesError ? (
        <div role="alert" className="rounded-sm border border-[var(--color-wi-amber)]/40 bg-[var(--color-wi-amber-bg)] px-3 py-3 text-sm">
          <p className="font-semibold text-[var(--color-wi-text)]">Sit-in rules could not be loaded.</p>
          <p className="mt-1 text-[var(--color-wi-text-light)]">
            Existing rule assignments are preserved, but rule selection is disabled until the list loads.
            {rulesError ? ` ${rulesError}` : ""}
          </p>
          {onRetryRules ? (
            <Button variant="secondary" size="sm" className="mt-2" loading={rulesRetrying} onClick={onRetryRules}>
              Retry sit-in rules
            </Button>
          ) : null}
        </div>
      ) : null}
      {groupError ? <div className="rounded-sm border border-[var(--color-wi-red)]/20 bg-[var(--color-wi-danger-bg)] px-3 py-2 text-sm text-[var(--color-wi-red-dark)]" role="alert">{groupError}</div> : null}

      <section className="grid grid-cols-2 gap-2 rounded-md border border-wi-line bg-[var(--color-wi-callout)] p-3 sm:grid-cols-4" aria-label="Course level status summary">
        <SummaryMetric label="Total courses" value={summary.totalCourses} tone="blue" />
        <SummaryMetric label="Assigned levels" value={summary.assigned} tone="green" />
        <SummaryMetric label="Gaps" value={summary.gaps} tone="amber" />
        <SummaryMetric label="Not set" value={summary.notSet} tone="red" />
      </section>

      <section aria-label="Course group settings" className="rounded-md border border-wi-line bg-[var(--color-wi-row-alt)] p-3">
        <div className="flex flex-wrap items-end gap-2">
          <div className="min-w-[12rem] flex-1">
            <label htmlFor="new-course-group" className="mb-1 block text-xs font-medium text-[var(--color-wi-text)]">New course group</label>
            <Input
              id="new-course-group"
              value={newGroupName}
              onChange={(event) => setNewGroupName(event.target.value)}
              onKeyDown={(event) => { if (event.key === "Enter") void createGroup(); }}
              placeholder="New group name"
              size="sm"
            />
          </div>
          <Button size="sm" onClick={() => void createGroup()} disabled={!newGroupName.trim()} loading={savingGroup} aria-label="Add course group">Add group</Button>
        </div>
        <div className="mt-3 border-t border-wi-line pt-3 lg:hidden">
          {selectedRootGroup ? (
            <div className="flex flex-wrap items-end gap-2">
              {editingGroupId === selectedRootGroup.id ? (
                <>
                  <div className="min-w-[12rem] flex-1">
                    <label htmlFor="rename-course-group" className="mb-1 block text-xs font-medium text-[var(--color-wi-text)]">Group name</label>
                    <Input
                      id="rename-course-group"
                      aria-label={`Rename selected group ${selectedRootGroup.name}`}
                      value={editingGroupName}
                      onChange={(event) => setEditingGroupName(event.target.value)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter") void renameGroup(selectedRootGroup.id);
                        if (event.key === "Escape") setEditingGroupId(null);
                      }}
                      size="sm"
                    />
                  </div>
                  <Button size="sm" variant="secondary" onClick={() => void renameGroup(selectedRootGroup.id)} disabled={savingGroup} aria-label={`Save name for selected group ${selectedRootGroup.name}`}>Save</Button>
                  <Button size="sm" variant="ghost" onClick={() => setEditingGroupId(null)} aria-label={`Cancel rename for selected group ${selectedRootGroup.name}`}>Cancel</Button>
                </>
              ) : (
                <>
                  <p className="flex-1 text-xs text-[var(--color-wi-text)]">Rename or delete {selectedRootGroup.name}. A group can only be deleted once every course is moved out.</p>
                  <Button size="sm" variant="secondary" onClick={() => { setEditingGroupId(selectedRootGroup.id); setEditingGroupName(selectedRootGroup.name); }} aria-label={`Rename selected group ${selectedRootGroup.name}`}>Rename group</Button>
                  <Button size="sm" variant="ghost" className="text-[var(--color-wi-red)]" onClick={() => requestDeleteGroup(selectedRootGroup)} aria-label={`Delete selected group ${selectedRootGroup.name}`}>Delete group</Button>
                </>
              )}
            </div>
          ) : (
            <p className="text-xs text-[var(--color-wi-text)]">Select a course group to rename or delete it.</p>
          )}
        </div>
      </section>

      <div className="grid gap-4 lg:grid-cols-[15rem_minmax(0,1fr)]">
        <aside className="hidden overflow-hidden rounded-md border border-wi-line bg-white lg:block" aria-label="Course groups">
          <div className="flex items-center justify-between border-b border-wi-line px-3 py-3">
            <h3 className="text-sm font-semibold text-[var(--color-wi-text)]">Course groups</h3>
            <span className="rounded bg-[var(--color-wi-row-alt)] px-1.5 py-0.5 text-xs text-[var(--color-wi-text)]">{groupOptions.length}</span>
          </div>
          <div className="lg:max-h-[25rem] lg:overflow-y-auto">
            {groupOptions.map((group) => {
              const details = groupStatusFor(group);
              const isSelected = group.id === selectedGroup?.id;
              return (
                <div key={group.id} className={`border-b border-wi-line-soft ${isSelected ? "bg-[var(--color-wi-selected)]" : "hover:bg-[var(--color-wi-row-alt)]"}`}>
                  <div className="flex items-center gap-2 px-3 py-2.5">
                    <span className={`h-2 w-2 shrink-0 rounded-full ${statusDot(details.status)}`} aria-hidden="true" />
                    <div className="min-w-0 flex-1">
                      {editingGroupId === group.id ? (
                        <Input
                          aria-label={`Rename ${group.name}`}
                          value={editingGroupName}
                          onChange={(event) => setEditingGroupName(event.target.value)}
                          onKeyDown={(event) => {
                            if (event.key === "Enter") void renameGroup(group.id);
                            if (event.key === "Escape") setEditingGroupId(null);
                          }}
                          size="sm"
                          autoFocus
                        />
                      ) : (
                        <>
                          <button type="button" onClick={() => selectGroup(group.id)} aria-pressed={isSelected} className={`block w-full truncate text-left text-sm ${isSelected ? "font-semibold text-[var(--color-wi-primary-dark)]" : "text-[var(--color-wi-text)]"}`}>{group.name}</button>
                          <p className="mt-0.5 text-[11px] text-[var(--color-wi-text)]">{details.label}</p>
                        </>
                      )}
                    </div>
                    <span className="shrink-0 rounded bg-[var(--color-wi-row-alt)] px-1.5 py-0.5 text-xs text-[var(--color-wi-text)]">{group.courseCount}</span>
                  </div>
                  {!group.virtual ? (
                    <div className="flex justify-end gap-2 px-3 pb-2">
                      {editingGroupId === group.id ? (
                        <>
                          <Button size="sm" variant="secondary" onClick={() => void renameGroup(group.id)} disabled={savingGroup} aria-label={`Save name for ${group.name}`}>Save</Button>
                          <Button size="sm" variant="ghost" onClick={() => setEditingGroupId(null)} aria-label={`Cancel rename for ${group.name}`}>Cancel</Button>
                        </>
                      ) : (
                        <>
                          <Button size="sm" variant="ghost" onClick={() => { setEditingGroupId(group.id); setEditingGroupName(group.name); }} aria-label={`Rename ${group.name}`}>Rename</Button>
                          <Button size="sm" variant="ghost" className="text-[var(--color-wi-red)]" onClick={() => requestDeleteGroup(groups.find((item) => item.id === group.id) ?? { id: group.id, name: group.name, course_count: group.courseCount, sit_in_rule_id: null })} aria-label={`Delete ${group.name}`}>Delete</Button>
                        </>
                      )}
                    </div>
                  ) : null}
                </div>
              );
            })}
          </div>
        </aside>

        <section className="min-w-0 rounded-md border border-wi-line bg-white" aria-labelledby="selected-course-group-heading">
          {selectedGroup ? (
            <>
              <div className="border-b border-wi-line px-4 py-4">
                <div className="lg:hidden">
                  <label htmlFor="course-group-picker" className="mb-1 block text-xs font-medium text-[var(--color-wi-text-light)]">Course group</label>
                  <SearchableSelect
                    id="course-group-picker"
                    aria-label="Course group"
                    value={selectedGroup.id}
                    onChange={(event) => selectGroup(event.target.value)}
                    size="sm"
                  >
                    {groupOptions.map((group) => <option key={group.id} value={group.id}>{group.name} ({group.courseCount})</option>)}
                  </SearchableSelect>
                </div>
                <div className="mt-3 flex flex-wrap items-start justify-between gap-3 lg:mt-0">
                  <div>
                    <p className="text-xs font-medium uppercase tracking-[0.12em] text-[var(--color-wi-text-light)]">Selected group</p>
                    <h3 id="selected-course-group-heading" className="mt-1 text-lg font-semibold text-[var(--color-wi-text)]">{selectedGroup.name}</h3>
                    <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">Edit level assignments and status for every course in this group.</p>
                  </div>
                  <div className="flex flex-col items-start gap-1 sm:items-end">
                    <span className="inline-flex items-center gap-1.5 rounded-full border border-wi-line px-2.5 py-1 text-xs font-medium text-[var(--color-wi-text-light)]">
                      <span className={`h-1.5 w-1.5 rounded-full ${statusDot(selectedGroupDetails.status)}`} aria-hidden="true" />
                      {selectedGroupDetails.label}
                    </span>
                    {selectedDirtyCount > 0 ? <span className="text-xs font-medium text-[var(--color-wi-text)]">{selectedDirtyCount} unsaved {selectedDirtyCount === 1 ? "change" : "changes"}</span> : null}
                  </div>
                </div>

                {!selectedGroup.virtual ? (
                  <div className={`mt-4 rounded-sm border px-3 py-3 ${selectedGroupNeedsSitInRule ? "border-[var(--color-wi-amber)]/40 bg-[var(--color-wi-amber-bg)]" : "border-wi-line bg-[var(--color-wi-row-alt)]"}`} aria-label="Sit-in rule configuration">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="min-w-0 flex-1">
                        <h4 className="text-sm font-semibold text-[var(--color-wi-text)]">{selectedGroupNeedsSitInRule ? "Sit-in rule required" : "Sit-in rule"}</h4>
                        <p className="mt-1 text-xs text-[var(--color-wi-text)]">
                          {selectedGroupNeedsSitInRule
                            ? "Levelled courses need a sit-in rule before this group can be marked Ready. The rule determines the sit-in destination."
                            : selectedSitInRule
                              ? `Configured: ${selectedSitInRule.name}`
                              : currentRuleUnavailable
                                ? "A rule is assigned, but its details are unavailable."
                                : "No rule assigned. Students cannot be matched to a sit-in destination."}
                        </p>
                        {ruleError ? <p id="sit-in-rule-error" role="alert" className="mt-2 text-xs font-medium text-[var(--color-wi-red-dark)]">{ruleError}</p> : null}
                        <Link to={SIT_IN_RULES_HREF} className="mt-2 inline-flex min-h-[28px] items-center gap-1 text-xs font-medium text-[var(--color-wi-primary)] underline-offset-2 hover:underline">
                          Manage in Sit-in Rules
                          <ExternalLink className="h-3 w-3" aria-hidden="true" />
                        </Link>
                      </div>
                      <div className="w-full sm:w-64">
                        <label htmlFor="selected-group-sit-in-rule" className="sr-only">Sit-in rule for {selectedGroup.name}</label>
                        <SearchableSelect
                          id="selected-group-sit-in-rule"
                          aria-label={`Sit-in rule for ${selectedGroup.name}`}
                          value={selectedRootGroup?.sit_in_rule_id ?? ""}
                          onChange={(event) => void saveSitInRule(event.target.value || null)}
                          disabled={savingSitInRule || Boolean(rulesError)}
                          size="sm"
                          error={Boolean(ruleError)}
                          describedBy={ruleError ? "sit-in-rule-error" : undefined}
                        >
                          {!selectedRootGroup?.sit_in_rule_id ? <option value="">No rule assigned</option> : null}
                          {currentRuleUnavailable ? <option value={selectedRootGroup?.sit_in_rule_id ?? ""}>{rulesError ? "Current rule (list unavailable)" : "Current rule (not in list)"}</option> : null}
                          {rules.map((rule) => <option key={rule.id} value={rule.id}>{rule.name}</option>)}
                        </SearchableSelect>
                        {rules.length === 0 && !rulesError ? <p className="mt-1 text-xs text-[var(--color-wi-text)]">No sit-in rules are available. Create one in Sit-in Rules first.</p> : null}
                      </div>
                    </div>
                  </div>
                ) : null}

                {selectedMissingCycleEntry ? (
                  <div role="status" className="mt-3 rounded-sm border border-[var(--color-wi-amber)]/40 bg-[var(--color-wi-amber-bg)] px-3 py-3 text-xs">
                    <p className="font-semibold text-[var(--color-wi-text)]">A level is set without a cycle</p>
                    <p className="mt-1 text-[var(--color-wi-text)]">
                      This group cannot be Ready until every levelled course has a cycle, because levels are scoped by subject and cycle.
                    </p>
                    <Link to={`/courses/${selectedMissingCycleEntry.setupCourseId}`} className="mt-1 inline-flex min-h-[28px] items-center gap-1 font-medium text-[var(--color-wi-primary)] underline-offset-2 hover:underline">
                      Open course setup for {selectedMissingCycleEntry.code}
                      <ExternalLink className="h-3 w-3" aria-hidden="true" />
                    </Link>
                  </div>
                ) : null}

                {selectedLinkedCount > 0 ? (
                  <div className="mt-3 rounded-sm border border-wi-line bg-[var(--color-wi-callout)] px-3 py-3 text-xs">
                    <p className="font-semibold text-[var(--color-wi-text)]">{selectedLinkedCount} linked {selectedLinkedCount === 1 ? "course" : "courses"} in this group</p>
                    <p className="mt-1 text-[var(--color-wi-text)]">
                      {selectedHasContinuation
                        ? "Continuation links share one level. Edit either linked row to update both course IDs; saving overrides the source course's level for this link."
                        : "Merged course levels and cycles are managed by the linked course configuration in Sit-in Rules."}
                    </p>
                    <Link to={SIT_IN_RULES_HREF} className="mt-1 inline-flex min-h-[28px] items-center gap-1 font-medium text-[var(--color-wi-primary)] underline-offset-2 hover:underline">
                      Review links in Sit-in Rules
                      <ExternalLink className="h-3 w-3" aria-hidden="true" />
                    </Link>
                  </div>
                ) : null}

                <div className="mt-3 rounded-sm bg-[var(--color-wi-row-alt)] p-3">
                  <div className="flex flex-wrap items-end gap-3">
                    <div className="min-w-[16rem] flex-1">
                      <label htmlFor="add-level-course" className="mb-1 block text-xs font-medium text-[var(--color-wi-text)]">Course</label>
                      <TypeaheadSelect
                        id="add-level-course"
                        aria-label="Course for new level"
                        value={newLevelCourseId}
                        onChange={handleNewLevelCourseChange}
                        options={addableCourses.map((course) => ({
                          value: course.id,
                          label: `${course.code} — ${course.name}`,
                          keywords: `${course.code} ${course.name} ${course.subject_name}`,
                        }))}
                        placeholder={!canEditLevels ? "Level editing is disabled" : addableCourses.length > 0 ? "Choose a course" : "All courses have levels"}
                        disabled={!canEditLevels || addableCourses.length === 0 || savingCourseId !== null}
                        aria-invalid={Boolean(addError)}
                        aria-describedby={addError ? "add-level-error" : undefined}
                      />
                    </div>
                    <div className="w-28">
                      <label htmlFor="add-level-number" className="mb-1 block text-xs font-medium text-[var(--color-wi-text)]">Level</label>
                      <Input
                        id="add-level-number"
                        type="number"
                        min={1}
                        max={MAX_LEVEL}
                        step={1}
                        value={newLevel}
                        onChange={(event) => { setNewLevel(event.target.value); clearRowError("__add__"); }}
                        disabled={!canEditLevels || !newLevelCourseId || addableCourses.length === 0 || savingCourseId !== null}
                        error={Boolean(addError)}
                        describedBy={addError ? "add-level-error" : undefined}
                      />
                    </div>
                    <Button
                      onClick={() => void addLevel()}
                      disabled={!canEditLevels || !newLevelCourseId || newLevel.trim() === "" || addableCourses.length === 0 || savingCourseId !== null}
                      loading={savingCourseId === newLevelCourseId}
                      aria-label="Add level"
                    >
                      Add level
                    </Button>
                  </div>
                  {addError ? <p id="add-level-error" role="alert" className="mt-2 text-xs font-medium text-[var(--color-wi-red-dark)]">{addError}</p> : null}
                  <p className="mt-2 text-xs text-[var(--color-wi-text)]">{canEditLevels ? "Assign a level to an unassigned course. Existing rows can be edited below." : editingBlockedReason}</p>
                </div>

                {!selectedGroup.virtual ? (
                  <div className="mt-3 flex flex-wrap items-center gap-2">
                    <span className="text-xs font-medium text-[var(--color-wi-text-light)]">Add course to {selectedGroup.name}</span>
                    <TypeaheadSelect
                      id="add-course-to-group"
                      aria-label={`Add course to ${selectedGroup.name}`}
                      value=""
                      onChange={(value) => void assignCourse(value)}
                      options={courseOptions}
                      placeholder={courseOptions.length > 0 ? "Search by course code or subject" : "No courses available to add"}
                      disabled={savingCourseId !== null || Boolean(mergeConfigError) || courseOptions.length === 0}
                      className="min-w-[min(24rem,100%)] flex-1"
                    />
                  </div>
                ) : null}
              </div>

              <div className="overflow-x-auto border-t border-wi-line" role="region" aria-label="Course levels table" tabIndex={0}>
                <table className="block w-full text-sm md:table">
                  <caption className="sr-only">Course levels in {selectedGroup.name}</caption>
                  <thead className="hidden bg-[var(--color-wi-row-alt)] text-left text-xs text-[var(--color-wi-text)] md:table-header-group">
                    <tr>
                      <th scope="col" className="px-4 py-2.5 font-medium"><span className="text-[var(--color-wi-text)]">Level</span></th>
                      <th scope="col" className="px-4 py-2.5 font-medium"><span className="text-[var(--color-wi-text)]">Course</span></th>
                      <th scope="col" className="px-4 py-2.5 font-medium"><span className="text-[var(--color-wi-text)]">Cycle</span></th>
                      <th scope="col" className="px-4 py-2.5 font-medium"><span className="text-[var(--color-wi-text)]">Subject</span></th>
                      <th scope="col" className="px-4 py-2.5 font-medium"><span className="text-[var(--color-wi-text)]">Status</span></th>
                      <th scope="col" className="px-4 py-2.5 text-right font-medium"><span className="text-[var(--color-wi-text)]">Actions</span></th>
                    </tr>
                  </thead>
                  <tbody className="block md:table-row-group">
                    {selectedCourses.map((course) => {
                      const row = effective.rows.get(course.id);
                      const draftKey = levelDraftKey(course, row);
                      const baseline = row?.displayLevel?.toString() ?? "";
                      const draftLevel = draftLevels[draftKey] ?? baseline;
                      const dirty = draftLevel !== baseline;
                      const numeric = draftLevel.trim() === "" ? null : Number(draftLevel);
                      const statusLevel = numeric !== null && Number.isInteger(numeric) ? numeric : row?.displayLevel ?? null;
                      const rowError = rowErrors[course.id];
                      const noteId = `course-level-note-${course.id}`;
                      const errorId = `course-level-error-${course.id}`;
                      const linkedRowIsSaving = Boolean(
                        row?.mergeGroup?.rule_source_course_code
                        && row.mergeGroup.course_codes.some((code) => courses.some((item) => item.code === code && item.id === savingCourseId)),
                      );
                      const canEditRow = canEditLevels && Boolean(row?.canEditLevel) && savingCourseId !== course.id && !linkedRowIsSaving;
                      const continuationNote = row?.mergeGroup?.rule_source_course_code
                        ? "Shared level for both linked course IDs. Saving overrides the source course's level for this link."
                        : null;
                      const levelNote = continuationNote ?? row?.readOnlyReason;
                      return (
                        <tr key={course.id} className="block border-t border-wi-line-soft px-4 py-3 hover:bg-[var(--color-wi-row-alt)] md:table-row md:px-0 md:py-0">
                          <td className="block py-1 md:table-cell md:px-4 md:py-3">
                            <div className="flex items-center gap-2">
                              <span className="text-xs font-medium text-[var(--color-wi-text-light)] md:hidden">Level</span>
                              <Input
                                aria-label={`Level for ${course.code}`}
                                type="number"
                                min={1}
                                max={MAX_LEVEL}
                                step={1}
                                value={draftLevel}
                                onChange={(event) => {
                                  setDraftLevels((previous) => ({ ...previous, [draftKey]: event.target.value }));
                                  clearRowError(course.id);
                                }}
                                disabled={!canEditRow}
                                error={Boolean(rowError)}
                                describedBy={rowError ? errorId : levelNote ? noteId : undefined}
                                size="sm"
                                className="w-24 md:w-20"
                              />
                            </div>
                            {rowError ? <p id={errorId} role="alert" className="mt-1 max-w-xs text-xs font-medium text-[var(--color-wi-red-dark)]">{rowError}</p> : levelNote ? <p id={noteId} className="mt-1 max-w-xs text-xs text-[var(--color-wi-text-light)]">{levelNote}</p> : null}
                            {row && !row.cycleId ? (
                              <p className="mt-1 text-xs">
                                <Link to={`/courses/${row.setupCourseId}`} aria-label={`Open course setup for ${row.setupCourseId === course.id ? course.code : row.mergeGroup?.rule_source_course_code ?? course.code}`} className="font-medium text-[var(--color-wi-primary)] underline-offset-2 hover:underline">Open course setup</Link>
                              </p>
                            ) : null}
                            {row?.mergeGroup ? (
                              <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">
                                <Link to={SIT_IN_RULES_HREF} className="font-medium text-[var(--color-wi-primary)] underline-offset-2 hover:underline">Manage link in Sit-in Rules</Link>
                              </p>
                            ) : null}
                          </td>
                          <td className="block py-1 md:table-cell md:px-4 md:py-3">
                            <p className="font-mono text-xs font-semibold text-[var(--color-wi-text)]">{course.code}</p>
                            <p className="mt-0.5 text-xs text-[var(--color-wi-text-light)]">{course.name}</p>
                            {row?.mergeGroup ? (
                              <span className="mt-1 inline-flex rounded-sm border border-wi-line bg-[var(--color-wi-selected)] px-1.5 py-0.5 text-[11px] text-[var(--color-wi-text)]">
                                {row.mergeGroup.rule_source_course_code === course.code
                                  ? "Rule source (linked)"
                                  : row.mergeGroup.rule_source_course_code
                                    ? `Follows ${row.mergeGroup.rule_source_course_code}`
                                    : "Merged course"}
                              </span>
                            ) : null}
                          </td>
                          <td className="block py-1 text-xs text-[var(--color-wi-text-light)] md:table-cell md:px-4 md:py-3">
                            <span className="font-medium md:hidden">Cycle: </span>
                            {row?.cycleLabel ?? "No cycle"}
                          </td>
                          <td className="block py-1 text-xs text-[var(--color-wi-text-light)] md:table-cell md:px-4 md:py-3">
                            <span className="font-medium md:hidden">Subject: </span>
                            {course.subject_name}
                          </td>
                          <td className="block py-1 md:table-cell md:px-4 md:py-3">
                            <span className="mr-1 font-medium md:hidden">Status: </span>
                            <StatusBadge level={statusLevel} />
                          </td>
                          <td className="block py-2 md:table-cell md:px-4 md:py-3 md:text-right">
                            <div className="flex flex-wrap items-center gap-2 md:justify-end">
                              <Button
                                variant="secondary"
                                size="sm"
                                disabled={!dirty || !canEditRow}
                                loading={savingCourseId === course.id}
                                aria-label={`Save level for ${course.code}`}
                                onClick={() => void saveLevel(course, draftLevel)}
                              >
                                Save
                              </Button>
                              <TypeaheadSelect
                                aria-label={`Move ${course.code} to group`}
                                value={(row?.groupCourse ?? course).root_course_group_id ?? ""}
                                onChange={(value) => void moveCourse(course, value)}
                                options={groupCourseOptions}
                                placeholder="Group"
                                disabled={savingCourseId === course.id || Boolean(row?.mergeGroup) || Boolean(mergeConfigError)}
                                aria-invalid={Boolean(rowError)}
                                aria-describedby={rowError ? errorId : undefined}
                                className="w-36"
                              />
                            </div>
                            {row?.mergeGroup && row.groupCourse ? (
                              <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">
                                Use Add course in the destination group to move {row.mergeGroup.rule_source_course_code ? "both linked courses" : "this course"}.
                              </p>
                            ) : null}
                          </td>
                        </tr>
                      );
                    })}
                    {selectedCourses.length === 0 ? (
                      <tr className="block md:table-row">
                        <td colSpan={6} className="block px-4 py-12 text-center text-sm text-[var(--color-wi-text-light)] md:table-cell">No courses in this group yet.</td>
                      </tr>
                    ) : null}
                  </tbody>
                </table>
              </div>
            </>
          ) : (
            <div className="flex min-h-[28rem] items-center justify-center p-8 text-sm text-[var(--color-wi-text-light)]">Add a course group to start managing levels.</div>
          )}
        </section>
      </div>

      <ConfirmModal
        open={Boolean(deleteTarget)}
        title="Delete course group?"
        message={deleteTarget ? `Delete “${deleteTarget.name}”? It has no courses. Any associated absence priorities for this group will also be deleted. This cannot be undone.` : ""}
        confirmLabel="Delete group"
        variant="danger"
        loading={deletingGroup}
        onConfirm={() => void deleteGroup()}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
}
