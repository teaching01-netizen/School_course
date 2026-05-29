import { useEffect, useMemo, useState, useCallback } from "react";
import { Link } from "react-router-dom";
import SlideOver from "../components/SlideOver";
import RootCourseGroupRail from "../components/RootCourseGroupRail";
import LevelLadderCanvas from "../components/LevelLadderCanvas";
import CourseAssignmentSheet from "../components/CourseAssignmentSheet";
import AutoSitInToggle from "../components/AutoSitInToggle";
import ActiveCourseSelector from "../components/ActiveCourseSelector";
import RootGroupManagerPanel from "../components/RootGroupManagerPanel";
import { apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import { useRootCourseGroups } from "../hooks/useRootCourseGroups";
import { useAutoSitInPolicy } from "../hooks/useAutoSitInPolicy";
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import TypeaheadSelect from "../components/TypeaheadSelect";
import type { TypeaheadOption } from "../components/TypeaheadSelect";
import {
  getGapWarning,
  buildRootGroupHierarchy,
  computeLevelCompletion,
  detectGaps,
  UNGROUPED_KEY,
} from "../utils/levels";
import type { CourseLevelItem, BulkEditTarget } from "../utils/levels";
import type { LadderCellData } from "../components/LevelLadderCell";
import useReturnsDesk from "../hooks/useReturnsDesk";
import ReturnsDeskPanel from "../components/ReturnsDeskPanel";
import LevelStepper from "../components/LevelStepper";
import LevelBadge from "../components/LevelBadge";
import CourseLevelSearch from "../components/CourseLevelSearch";
import useLevelHistory from "../hooks/useLevelHistory";
import RuleSelector from "../components/RuleSelector";
import { useSitInRules } from "../hooks/useSitInRules";

type ViewMode = "classic" | "ladder";

type ActiveCoursesResponse = {
  subjects: Array<{
    subject_id: string;
    subject_code: string;
    subject_name: string;
    courses: Array<{
      course_id: string;
      course_code: string;
      course_name: string;
      cycle_id: string;
      cycle_label: string;
      is_active: boolean;
    }>;
  }>;
};

export default function CourseLevels() {
  const { addToast } = useToast();
  const [courses, setCourses] = useState<CourseLevelItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [editLevels, setEditLevels] = useState<Record<string, string>>({});
  const [savingCourse, setSavingCourse] = useState<Record<string, boolean>>({});
  const [activeCoursesMap, setActiveCoursesMap] = useState<Record<string, string>>({});
  const [savingActiveCourse, setSavingActiveCourse] = useState<Record<string, boolean>>({});

  // View mode
  const [viewMode, setViewMode] = useState<ViewMode>("classic");

  // Root group selection (Phase 1d rail)
  const [selectedRootGroupId, setSelectedRootGroupId] = useState<string | null>(null);

  // Collapse state per root group (Phase 1d)
  const [collapsedRootGroups, setCollapsedRootGroups] = useState<Record<string, boolean>>({});

  // Bulk edit
  const [bulkEditTarget, setBulkEditTarget] = useState<BulkEditTarget | null>(null);
  const [bulkLevels, setBulkLevels] = useState<Record<string, string>>({});
  const [savingBulk, setSavingBulk] = useState(false);
  const [verificationReport, setVerificationReport] = useState<string[] | null>(null);

  // Ladder view state (Phase 2+)
  const [selectedLadderCell, setSelectedLadderCell] = useState<{ level: number; cycleId: string } | null>(null);

  // Slide-over panel state (Phase 4)
  const [slideOverOpen, setSlideOverOpen] = useState(false);
  const [slideOverContent, setSlideOverContent] = useState<"groups" | null>(null);

  // Search
  const [searchTerm, setSearchTerm] = useState("");

  // Multi-select (for bulk operations from table)
  const [selectedCourseIds, setSelectedCourseIds] = useState<Set<string>>(new Set());

  // Last verified timestamp
  const [lastVerified, setLastVerified] = useState<Date | null>(null);

  // Hooks
  const groupState = useRootCourseGroups();
  const policyState = useAutoSitInPolicy();
  const {
    rootCourseGroups,
    setRootCourseGroups,
    fetchManageGroups,
  } = groupState;

  const {
    autoSitInToggles,
    savingPolicy,
    loadPolicies,
    savePolicy,
    setPolicyToggle,
    setPolicyInitialState,
    isDirty,
  } = policyState;

  const returnsDesk = useReturnsDesk();
  const levelHistory = useLevelHistory();
  const { rules: sitInRules } = useSitInRules();

  // Rule assignment state
  const [ruleAssignments, setRuleAssignments] = useState<Record<string, string | null>>({});
  const [savingRule, setSavingRule] = useState<Record<string, boolean>>({});

  // Load data
  useEffect(() => {
    (async () => {
      try {
        const [coursesData, policiesResp, groupsData, activeCoursesData] = await Promise.all([
          apiJson<CourseLevelItem[]>("/api/v1/admin/course-levels", { method: "GET" }),
          apiJson<import("../utils/levels").PolicyResponse>("/api/v1/admin/absence-policies", { method: "GET" }),
          apiJson<import("../utils/levels").RootCourseGroupInfo[]>("/api/v1/admin/root-course-groups", { method: "GET" }),
          apiJson<ActiveCoursesResponse>("/api/v1/admin/active-courses", { method: "GET" }),
        ]);
        setCourses(coursesData);
        setRootCourseGroups(groupsData);

        // Initialize rule assignments from data
        const ruleMap: Record<string, string | null> = {};
        for (const group of groupsData) {
          ruleMap[group.id] = group.sit_in_rule_id ?? null;
        }
        setRuleAssignments(ruleMap);

        // Build active course map: subject_id → active_course_id
        const activeMap: Record<string, string> = {};
        for (const subject of activeCoursesData.subjects) {
          const active = subject.courses.find((c) => c.is_active);
          if (active) {
            activeMap[subject.subject_id] = active.course_id;
          }
        }
        setActiveCoursesMap(activeMap);

        // Load policy data directly (no extra API call needed)
        const rootGroupPolicies = policiesResp.absence_policies?.root_course_groups ?? {};
        const toggles: Record<string, boolean> = {};
        for (const [rootCourseId, policy] of Object.entries(rootGroupPolicies)) {
          toggles[rootCourseId] = policy.auto_sit_in_enabled;
        }
        setPolicyInitialState(toggles);

        const levels: Record<string, string> = {};
        for (const c of coursesData) {
          levels[c.id] = c.level?.toString() ?? "";
        }
        setEditLevels(levels);
      } catch (err) {
        addToast("error", err instanceof Error ? err.message : "Failed to load data");
      } finally {
        setLoading(false);
      }
    })();
  }, [addToast, loadPolicies, setRootCourseGroups]);

  const rootGroupHierarchy = useMemo(
    () => buildRootGroupHierarchy(courses),
    [courses],
  );

  const overviewStats = useMemo(() => {
    const hierarchy = rootGroupHierarchy;
    const totalGroups = Object.keys(hierarchy).length;
    const groupsWithRule = rootCourseGroups.filter((g) => g.sit_in_rule_id).length;
    const groupsWithoutRule = totalGroups - groupsWithRule;

    let groupsWithGaps = 0;
    for (const rg of Object.values(hierarchy)) {
      const allCourses = rg.subjects.flatMap((s) =>
        Object.values(s.cycles).flatMap((c) => c.courses),
      );
      const previewCourses = allCourses.map((c) => ({
        ...c,
        level: editLevels[c.id] ? parseInt(editLevels[c.id], 10) : c.level,
      }));
      if (getGapWarning(previewCourses)) groupsWithGaps++;
    }

    return { totalGroups, groupsWithRule, groupsWithoutRule, groupsWithGaps };
  }, [rootGroupHierarchy, rootCourseGroups, editLevels]);

  // Build root group data for rail
  const rootGroupRailData = useMemo(() => {
    return Object.values(rootGroupHierarchy).map((rg) => {
      const allCourses = rg.subjects.flatMap((s) =>
        Object.values(s.cycles).flatMap((c) => c.courses),
      );
      const { assigned, total } = computeLevelCompletion(allCourses);
      return {
        rootCourseGroupId: rg.rootCourseGroupId,
        label: rg.label,
        courseCount: total,
        assignedCount: assigned,
      };
    });
  }, [rootGroupHierarchy]);

  const filteredRootGroups = useMemo(() => {
    let entries = Object.values(rootGroupHierarchy);
    if (selectedRootGroupId !== null) {
      entries = entries.filter((rg) => rg.rootCourseGroupId === selectedRootGroupId);
    }
    if (!searchTerm) return entries;

    const term = searchTerm.toLowerCase();
    return entries
      .map((rg) => ({
        ...rg,
        subjects: rg.subjects
          .map((subject) => ({
            ...subject,
            cycles: Object.fromEntries(
              Object.entries(subject.cycles).map(([cycleId, cycle]) => [
                cycleId,
                {
                  ...cycle,
                  courses: cycle.courses.filter(
                    (c) =>
                      c.code.toLowerCase().includes(term) ||
                      c.name.toLowerCase().includes(term) ||
                      subject.subjectCode.toLowerCase().includes(term) ||
                      subject.subjectName.toLowerCase().includes(term) ||
                      rg.label.toLowerCase().includes(term),
                  ),
                },
              ]),
            ),
          }))
          .filter((subject) =>
            Object.values(subject.cycles).some((cycle) => cycle.courses.length > 0),
          ),
      }))
      .filter((rg) => rg.subjects.length > 0);
  }, [rootGroupHierarchy, selectedRootGroupId, searchTerm]);

  // --- Level CRUD ---

  async function saveLevel(course: CourseLevelItem) {
    const levelStr = editLevels[course.id];
    const level = levelStr ? parseInt(levelStr, 10) : null;
    const previousLevel = course.level;
    setSavingCourse((p) => ({ ...p, [course.id]: true }));
    try {
      await apiJson(`/api/v1/admin/courses/${course.id}/level`, {
        method: "PUT",
        body: JSON.stringify({ level, cycle_id: course.cycle_id }),
      });
      addToast("success", "Level saved");
      setCourses((prev) =>
        prev.map((c) => (c.id === course.id ? { ...c, level } : c)),
      );
      // Record undo snapshot
      const before: Record<string, number | null> = { [course.id]: previousLevel };
      const after: Record<string, number | null> = { [course.id]: level };
      levelHistory.pushSnapshot(`Saved ${course.code} to Level ${level ?? "Not set"}`, before, after);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Save failed";
      addToast("error", message);
      returnsDesk.addFailure({
        courseId: course.id,
        courseCode: course.code,
        attemptedLevel: level,
        cycleId: course.cycle_id,
        error: { code: "save_failed", message },
      });
    } finally {
      setSavingCourse((p) => ({ ...p, [course.id]: false }));
    }
  }

  async function saveRootCoursePolicy(rootCourseId: string) {
    await savePolicy(rootCourseId, autoSitInToggles[rootCourseId]);
    addToast("success", "Policy saved");
  }

  async function saveRootCourse(course: CourseLevelItem, rootCourseGroupId: string | null) {
    try {
      await apiJson(`/api/v1/admin/courses/${course.id}/root-course-group`, {
        method: "PUT",
        body: JSON.stringify({ root_course_group_id: rootCourseGroupId }),
      });
      addToast("success", "Root course group saved");
      setCourses((prev) =>
        prev.map((c) => {
          if (c.id === course.id) {
            const rc = rootCourseGroupId
              ? rootCourseGroups.find((g) => g.id === rootCourseGroupId)
              : null;
            return {
              ...c,
              root_course_group_id: rootCourseGroupId,
              root_course_group_name: rc?.name ?? null,
            };
          }
          return c;
        }),
      );
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Save failed");
    }
  }

  async function saveActiveCourse(subjectId: string, courseId: string) {
    setSavingActiveCourse((prev) => ({ ...prev, [subjectId]: true }));
    try {
      await apiJson("/api/v1/admin/active-courses", {
        method: "PUT",
        body: JSON.stringify({ subject_id: subjectId, course_id: courseId }),
      });
      setActiveCoursesMap((prev) => ({ ...prev, [subjectId]: courseId }));
      addToast("success", "Active course updated");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to update active course");
    } finally {
      setSavingActiveCourse((prev) => ({ ...prev, [subjectId]: false }));
    }
  }

  async function saveRuleAssignment(rootCourseId: string, ruleId: string | null) {
    const currentRule = ruleAssignments[rootCourseId];
    const ruleName = sitInRules.find(r => r.id === ruleId)?.name ?? "None";
    const currentName = sitInRules.find(r => r.id === currentRule)?.name ?? "None";

    if (currentRule !== ruleId) {
      const confirmed = window.confirm(
        `Change sit-in rule for this group from "${currentName}" to "${ruleName}"?`
      );
      if (!confirmed) return;
    }

    setSavingRule(prev => ({ ...prev, [rootCourseId]: true }));
    try {
      await apiJson(`/api/v1/admin/root-course-groups/${rootCourseId}`, {
        method: "PUT",
        body: JSON.stringify({ sit_in_rule_id: ruleId }),
      });
      setRuleAssignments(prev => ({ ...prev, [rootCourseId]: ruleId }));
      addToast("success", "Rule assigned");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to assign rule");
    } finally {
      setSavingRule(prev => ({ ...prev, [rootCourseId]: false }));
    }
  }

  function openBulkEdit(target: { label: string; courses: CourseLevelItem[] }) {
    setBulkLevels(Object.fromEntries(target.courses.map((course) => [course.id, course.level?.toString() ?? ""])));
    setBulkEditTarget(target);
  }

  async function applyBulkEdit() {
    if (!bulkEditTarget) return;
    const changedCourses = bulkEditTarget.courses.filter(
      (course) => bulkLevels[course.id] !== (course.level?.toString() ?? ""),
    );
    setSavingBulk(true);

    // Capture before state for undo
    const beforeState: Record<string, number | null> = {};
    for (const course of changedCourses) {
      beforeState[course.id] = course.level;
    }

    try {
      const results = await Promise.allSettled(changedCourses.map((course) => {
        const value = bulkLevels[course.id];
        const level = value ? parseInt(value, 10) : null;
        return apiJson(`/api/v1/admin/courses/${course.id}/level`, {
          method: "PUT",
          body: JSON.stringify({ level, cycle_id: course.cycle_id }),
        }).then(() => ({ course, level }));
      }));

      // Apply successful changes
      const succeeded = results.filter((r): r is PromiseFulfilledResult<{ course: CourseLevelItem; level: number | null }> => r.status === "fulfilled");
      const failed = results.filter((r): r is PromiseRejectedResult => r.status === "rejected");

      if (succeeded.length > 0) {
        setCourses((previous) => previous.map((course) => {
          if (!succeeded.some((s) => s.value.course.id === course.id)) return course;
          const value = bulkLevels[course.id];
          return { ...course, level: value ? parseInt(value, 10) : null };
        }));
        setEditLevels((previous) => ({
          ...previous,
          ...Object.fromEntries(succeeded.map((s) => [s.value.course.id, bulkLevels[s.value.course.id] ?? ""])),
        }));

        // Record undo snapshot for successful changes
        const afterState: Record<string, number | null> = {};
        for (const s of succeeded) {
          afterState[s.value.course.id] = s.value.level;
        }
        levelHistory.pushSnapshot(
          `Bulk updated ${succeeded.length} course${succeeded.length === 1 ? "" : "s"}`,
          beforeState,
          afterState,
        );
      }

      // Track failures in Returns Desk
      for (let i = 0; i < failed.length; i++) {
        const course = changedCourses[i];
        const error = failed[i].reason;
        returnsDesk.addFailure({
          courseId: course.id,
          courseCode: course.code,
          attemptedLevel: bulkLevels[course.id] ? parseInt(bulkLevels[course.id], 10) : null,
          cycleId: course.cycle_id,
          error: {
            code: "bulk_update_failed",
            message: error instanceof Error ? error.message : "Update failed",
          },
        });
      }

      const successCount = succeeded.length;
      const failCount = failed.length;
      if (failCount === 0) {
        addToast("success", successCount === 0 ? "No changes to apply" : "Levels updated");
      } else {
        addToast("error", `${successCount} saved, ${failCount} failed (see Returns Desk)`);
      }
      setBulkEditTarget(null);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Bulk update failed");
    } finally {
      setSavingBulk(false);
    }
  }

  function verifyConfiguration() {
    const missingCount = courses.filter((course) => {
      const value = editLevels[course.id];
      return !value;
    }).length;
    let gapCount = 0;
    for (const rootGroup of Object.values(rootGroupHierarchy)) {
      for (const subject of rootGroup.subjects) {
        for (const [, cycle] of Object.entries(subject.cycles)) {
          const previewCourses = cycle.courses.map((course) => ({
            ...course,
            level: editLevels[course.id] ? parseInt(editLevels[course.id], 10) : null,
          }));
          if (getGapWarning(previewCourses)) gapCount += 1;
        }
      }
    }
    const report: string[] = [];
    if (missingCount > 0) report.push(`${missingCount} course${missingCount === 1 ? " has" : "s have"} no level set.`);
    if (gapCount > 0) report.push(`${gapCount} cycle${gapCount === 1 ? " has" : "s have"} a level gap.`);
    if (report.length === 0) report.push("All course configurations are valid.");
    setVerificationReport(report);
    setLastVerified(new Date());
  }

  function toggleCollapse(rootGroupId: string) {
    setCollapsedRootGroups((prev) => ({ ...prev, [rootGroupId]: !prev[rootGroupId] }));
  }

  function openSlideOver(content: "groups") {
    setSlideOverContent(content);
    setSlideOverOpen(true);
    if (content === "groups") {
      fetchManageGroups();
    }
  }

  // --- Ladder view helpers (Phase 2) ---

  const ladderData = useMemo(() => {
    const entries = Object.values(rootGroupHierarchy);
    const cycles = new Map<string, string>();
    const levelSet = new Set<number>();
    const cellMap = new Map<string, CourseLevelItem[]>();

    for (const rootGroup of entries) {
      for (const subject of rootGroup.subjects) {
        for (const [cycleId, cycle] of Object.entries(subject.cycles)) {
          cycles.set(cycleId, cycle.cycleLabel);
          for (const course of cycle.courses) {
            const lvl = editLevels[course.id] ? parseInt(editLevels[course.id], 10) : course.level;
            if (lvl !== null && !isNaN(lvl)) {
              levelSet.add(lvl);
              const key = `${cycleId}-${lvl}`;
              if (!cellMap.has(key)) cellMap.set(key, []);
              cellMap.get(key)!.push(course);
            }
          }
        }
      }
    }

    const sortedLevels = Array.from(levelSet).sort((a, b) => a - b);
    // Add gap levels
    const allLevels: number[] = [];
    if (sortedLevels.length >= 2) {
      for (let i = sortedLevels[0]; i <= sortedLevels[sortedLevels.length - 1]; i++) {
        allLevels.push(i);
      }
    } else {
      allLevels.push(...sortedLevels);
    }

    return {
      cycles: Array.from(cycles.entries()).map(([id, label]) => ({ cycleId: id, cycleLabel: label })),
      levels: allLevels,
      getCell: (level: number, cycleId: string): LadderCellData => {
        const key = `${cycleId}-${level}`;
        const coursesAtCell = cellMap.get(key);
        const gaps = detectGaps(
          Array.from(cellMap.entries())
            .filter(([k]) => k.startsWith(cycleId))
            .flatMap(([, cs]) => cs),
        );
        const isGap = gaps.some((g) => g.level === level);

        if (!coursesAtCell || coursesAtCell.length === 0) {
          if (isGap) {
            return { level, cycleId, status: "gap", errorMessage: "Level gap" };
          }
          return { level, cycleId, status: "vacant" };
        }
        if (coursesAtCell.length > 1) {
          return {
            level,
            cycleId,
            status: "overlap",
            courseCode: coursesAtCell.map((c) => c.code).join(", "),
            errorMessage: `${coursesAtCell.length} courses at same level`,
          };
        }
        const course = coursesAtCell[0];
        return {
          level,
          cycleId,
          courseId: course.id,
          courseCode: course.code,
          courseName: course.name,
          status: "active",
        };
      },
    };
  }, [rootGroupHierarchy, editLevels]);

  const selectedCoursesForSheet = useMemo(() => {
    if (!selectedLadderCell) return [];
    const { level, cycleId } = selectedLadderCell;
    return courses.filter((c) => {
      const lvl = editLevels[c.id] ? parseInt(editLevels[c.id], 10) : c.level;
      return c.cycle_id === cycleId && lvl === level;
    });
  }, [selectedLadderCell, courses, editLevels]);

  const handleLadderCellClick = useCallback((cell: LadderCellData) => {
    setSelectedLadderCell({ level: cell.level, cycleId: cell.cycleId });
  }, []);

  const handleCellDrop = useCallback(
    (fromLevel: number, toLevel: number, cycleId: string, courseId: string) => {
      // Find the course being dropped
      const course = courses.find((c) => c.id === courseId);
      if (!course) return;

      // Check if the target level is already occupied in this cycle
      const existingAtTarget = courses.find(
        (c) => {
          const lvl = editLevels[c.id] ? parseInt(editLevels[c.id], 10) : c.level;
          return c.cycle_id === cycleId && lvl === toLevel && c.id !== courseId;
        },
      );

      if (existingAtTarget) {
        // Swap: set existing course to fromLevel, dragged course to toLevel
        const swapFromLevel = fromLevel;
        setEditLevels((prev) => ({
          ...prev,
          [courseId]: toLevel.toString(),
          [existingAtTarget.id]: swapFromLevel.toString(),
        }));
        addToast("info", `Swapped ${course.code} ↔ ${existingAtTarget.code}`);
      } else {
        // Simple move
        setEditLevels((prev) => ({ ...prev, [courseId]: toLevel.toString() }));
      }
    },
    [courses, editLevels, addToast],
  );

  if (loading) return <LoadingSkeleton type="table" lines={5} />;

  return (
    <div className="w-full">
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div>
          <PageHeading>Course Levels</PageHeading>
          <p className="text-sm text-gray-500">
            Set level for each course in each cycle. Levels must be consecutive within a cycle.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Link
            to="/operations?tab=rule-inventory"
            className="text-xs font-medium text-[var(--color-wi-primary)] hover:underline"
          >
            Manage Rules
          </Link>
          <CourseLevelSearch value={searchTerm} onChange={setSearchTerm} />
          {/* View mode toggle */}
          <div className="flex border border-gray-300 rounded-sm overflow-hidden">
            <button
              onClick={() => setViewMode("classic")}
              className={`px-2.5 py-1 text-xs font-medium transition-colors ${
                viewMode === "classic"
                  ? "bg-blue-600 text-white"
                  : "bg-white text-gray-600 hover:bg-gray-50"
              }`}
            >
              Classic
            </button>
            <button
              onClick={() => setViewMode("ladder")}
              className={`px-2.5 py-1 text-xs font-medium transition-colors ${
                viewMode === "ladder"
                  ? "bg-blue-600 text-white"
                  : "bg-white text-gray-600 hover:bg-gray-50"
              }`}
            >
              Ladder
            </button>
          </div>
          {levelHistory.canUndo && (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => {
                const restored = levelHistory.undoLast();
                if (restored) {
                  // Re-send all restored levels as PUT calls
                  const entries = Object.entries(restored);
                  Promise.allSettled(
                    entries.map(([courseId, level]) => {
                      const course = courses.find((c) => c.id === courseId);
                      if (!course) return Promise.resolve();
                      return apiJson(`/api/v1/admin/courses/${courseId}/level`, {
                        method: "PUT",
                        body: JSON.stringify({ level, cycle_id: course.cycle_id }),
                      });
                    }),
                  ).then((results) => {
                    const successCount = results.filter((r) => r.status === "fulfilled").length;
                    addToast("success", `Undid ${successCount} change${successCount === 1 ? "" : "s"}`);
                    // Refresh courses from server
                    apiJson<CourseLevelItem[]>("/api/v1/admin/course-levels", { method: "GET" }).then(setCourses);
                    // Rebuild editLevels
                    const levels: Record<string, string> = {};
                    for (const [courseId, level] of Object.entries(restored)) {
                      levels[courseId] = level?.toString() ?? "";
                    }
                    setEditLevels((prev) => ({ ...prev, ...levels }));
                  });
                }
              }}
              title={levelHistory.lastAction ? `Undo: ${levelHistory.lastAction}` : "Undo last change"}
            >
              Undo
            </Button>
          )}
          <Button variant="secondary" size="sm" onClick={verifyConfiguration}>
            Verify All
          </Button>
          {lastVerified && (
            <span className="text-xs text-gray-400">
              Last verified: {lastVerified.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" })}
            </span>
          )}
          <Button
            variant="secondary"
            size="sm"
            onClick={() => openSlideOver("groups")}
          >
            Manage Groups
          </Button>
        </div>
      </div>

      {/* Info banner */}
      <div className="mb-5 border border-blue-100 bg-blue-50 px-4 py-3 text-sm text-blue-800 rounded-sm">
        <p className="font-medium mb-1">How sit-in rules work</p>
        <p>
          Each root course group needs a sit-in rule to determine how students make up missed classes.
          Level 1 students attend via Zoom. Higher-level students sit in at adjacent levels.
          For SAT and other subjects, assign specific rules via <strong>Manage Rules</strong>.
        </p>
      </div>

      {/* Status overview bar */}
      {overviewStats.totalGroups > 0 && (
        <div className="mb-4 flex items-center gap-3 text-xs">
          <span className="flex items-center gap-1">
            <span className="inline-block w-2 h-2 rounded-full bg-green-500" />
            {overviewStats.groupsWithRule} configured
          </span>
          {overviewStats.groupsWithoutRule > 0 && (
            <span className="flex items-center gap-1">
              <span className="inline-block w-2 h-2 rounded-full bg-amber-500" />
              {overviewStats.groupsWithoutRule} missing rule
            </span>
          )}
          {overviewStats.groupsWithGaps > 0 && (
            <span className="flex items-center gap-1">
              <span className="inline-block w-2 h-2 rounded-full bg-red-500" />
              {overviewStats.groupsWithGaps} with gaps
            </span>
          )}
        </div>
      )}

      {/* Verification report */}
      {verificationReport && (
        <div className="mb-5 border border-gray-200 bg-white px-4 py-3 rounded-sm text-sm text-gray-700" role="status">
          <p className="font-medium mb-1">Verification results</p>
          {verificationReport.map((message) => <p key={message}>{message}</p>)}
        </div>
      )}

      {/* Main content: rail + canvas or table */}
      <div className="flex gap-4">
        {/* Root Course Group Rail */}
        <RootCourseGroupRail
          rootGroups={rootGroupRailData}
          selectedRootGroupId={selectedRootGroupId}
          onSelectRootGroup={setSelectedRootGroupId}
        />

        {/* Content area */}
        <div className="flex-1 min-w-0">
          {filteredRootGroups.length === 0 ? (
            <div className="text-sm text-gray-400 py-8 text-center">
              {selectedRootGroupId
                ? "No data for selected root course group."
                : "No root course groups found. Courses must have a root course group assigned to appear here."}
            </div>
          ) : viewMode === "ladder" ? (
            /* ===== LADDER VIEW (Phase 2) ===== */
            <div>
              {filteredRootGroups.map((rootGroup) => {
                const rootKey = rootGroup.rootCourseGroupId ?? UNGROUPED_KEY;
                return (
                  <div key={rootKey} className="mb-6">
                    <div className="text-sm font-semibold text-gray-800 mb-3 pb-2 border-b border-gray-200">
                      {rootGroup.label}
                    </div>
                    {rootGroup.subjects.map((subject) => (
                      <div key={subject.subjectId} className="mb-6 ml-4">
                        <div className="text-xs font-semibold text-gray-600 uppercase tracking-wide mb-2">
                          {subject.subjectCode} — {subject.subjectName}
                        </div>
                        {Object.entries(subject.cycles).map(([cycleId, cycle]) => (
                          <div key={cycleId} className="mb-4 ml-4">
                            <div className="text-xs font-medium text-gray-500 uppercase tracking-wide mb-2">
                              {cycle.cycleLabel}
                            </div>
                            <LevelLadderCanvas
                              cycles={ladderData.cycles.filter((c) => c.cycleId === cycleId)}
                              levels={ladderData.levels}
                              getCell={(level, cId) => ladderData.getCell(level, cId)}
                              onCellClick={handleLadderCellClick}
                              onCellDrop={handleCellDrop}
                              selectedCell={selectedLadderCell}
                            />
                          </div>
                        ))}
                      </div>
                    ))}
                    <CourseAssignmentSheet
                      courses={selectedCoursesForSheet.length > 0 ? selectedCoursesForSheet : []}
                      onEditLevel={(course) => {
                        const el = document.getElementById(`level-input-${course.id}`);
                        el?.focus();
                      }}
                    />
                  </div>
                );
              })}
            </div>
          ) : (
            /* ===== CLASSIC TABLE VIEW (preserved for backward compat) ===== */
            filteredRootGroups.map((rootGroup) => {
              const rootKey = rootGroup.rootCourseGroupId ?? UNGROUPED_KEY;
              const isCollapsed = collapsedRootGroups[rootKey];
              const allRootCourses = rootGroup.subjects.flatMap((s) =>
                Object.values(s.cycles).flatMap((c) => c.courses),
              );
              const { assigned, total } = computeLevelCompletion(allRootCourses);

              return (
                <div key={rootKey} className="mb-8">
                  {/* Root group header with collapse toggle */}
                  <div
                    className="flex items-center gap-2 cursor-pointer select-none"
                    onClick={() => toggleCollapse(rootKey)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        toggleCollapse(rootKey);
                      }
                    }}
                    tabIndex={0}
                    role="button"
                    aria-expanded={!isCollapsed}
                    aria-label={`${rootGroup.label} (${assigned}/${total} levels assigned)`}
                  >
                    <span className="text-xs text-gray-400 transition-transform duration-150">
                      {isCollapsed ? "▶" : "▼"}
                    </span>
                    <div className="text-sm font-semibold text-gray-800 pb-2 border-b border-gray-200 flex-1">
                      <span>{rootGroup.label}</span>
                      <span
                        className={`ml-2 text-[10px] font-medium px-1.5 py-0.5 rounded-full ${
                          total > 0 && assigned === total
                            ? "bg-green-100 text-green-700"
                            : assigned > 0
                              ? "bg-amber-100 text-amber-700"
                              : "bg-gray-100 text-gray-500"
                        }`}
                      >
                        {assigned}/{total} levels
                      </span>
                    </div>
                  </div>

                  {!isCollapsed && (
                    <>
                      {/* Action bar for this root group */}
                      {rootGroup.rootCourseGroupId !== null && (
                        <div className="flex items-center gap-4 mb-2 mt-2 border-b border-gray-100 pb-2">
                          <ActiveCourseSelector
                            subjectId={rootGroup.subjects[0]?.subjectId ?? ""}
                            courses={rootGroup.subjects.flatMap((s) =>
                              Object.values(s.cycles).flatMap((cycle) =>
                                cycle.courses.map((c) => ({
                                  id: c.id,
                                  code: c.code,
                                  name: c.name,
                                  cycleLabel: c.cycle_label,
                                }))
                              )
                            )}
                            activeCourseId={activeCoursesMap[rootGroup.subjects[0]?.subjectId ?? ""] ?? null}
                            disabled={savingActiveCourse[rootGroup.subjects[0]?.subjectId ?? ""] ?? false}
                            onSelect={(courseId) => void saveActiveCourse(rootGroup.subjects[0]?.subjectId ?? "", courseId)}
                          />
                          <AutoSitInToggle
                            label={rootGroup.label}
                            enabled={rootGroup.rootCourseGroupId ? (autoSitInToggles[rootGroup.rootCourseGroupId] ?? true) : true}
                            dirty={rootGroup.rootCourseGroupId ? isDirty(rootGroup.rootCourseGroupId) : false}
                            saving={rootGroup.rootCourseGroupId ? (savingPolicy[rootGroup.rootCourseGroupId] ?? false) : false}
                            onToggle={(enabled) => rootGroup.rootCourseGroupId && setPolicyToggle(rootGroup.rootCourseGroupId, enabled)}
                            onSave={() => rootGroup.rootCourseGroupId && saveRootCoursePolicy(rootGroup.rootCourseGroupId)}
                          />
                          <RuleSelector
                            rules={sitInRules}
                            value={ruleAssignments[rootGroup.rootCourseGroupId ?? ""] ?? null}
                            onChange={(ruleId) => rootGroup.rootCourseGroupId && saveRuleAssignment(rootGroup.rootCourseGroupId, ruleId)}
                            disabled={savingRule[rootGroup.rootCourseGroupId ?? ""] ?? false}
                          />
                          <Button
                            variant="secondary"
                            size="sm"
                            aria-label={`Bulk edit levels for ${rootGroup.label}`}
                            onClick={() => openBulkEdit({
                              label: rootGroup.label,
                              courses: allRootCourses,
                            })}
                          >
                            Bulk Edit Levels
                          </Button>
                          <Link
                            to={`/absences?subject_id=${encodeURIComponent(rootGroup.subjects[0]?.subjectId ?? "")}`}
                            aria-label={`View absences for ${rootGroup.label}`}
                            className="text-xs font-medium text-[var(--color-wi-primary)] hover:underline"
                          >
                            View Absences
                          </Link>
                        </div>
                      )}

                      {/* Subjects within this root group */}
                      {rootGroup.subjects.map((subject) => (
                        <div key={subject.subjectId} className="mb-4 ml-4">
                          <div className="text-xs font-semibold text-gray-600 uppercase tracking-wide mb-2">
                            {subject.subjectCode} — {subject.subjectName}
                          </div>

                          {Object.entries(subject.cycles).map(([cycleId, cycle]) => {
                            const previewCourses = cycle.courses.map((course) => ({
                              ...course,
                              level: editLevels[course.id] ? parseInt(editLevels[course.id], 10) : null,
                            }));
                            const warning = getGapWarning(previewCourses);
                            const warnedLevel = Math.max(...previewCourses.map((course) => course.level ?? 0));
                            return (
                              <div key={cycleId} className="mb-4 ml-4">
                                <div className="text-xs font-medium text-gray-500 uppercase tracking-wide mb-2">
                                  {cycle.cycleLabel}
                                </div>

                                <table className="w-full text-sm border-collapse">
                                  <thead>
                                    <tr className="border-b border-gray-200 text-left text-gray-500">
                                      <th className="py-1.5 pr-3 font-medium w-8">
                                        <input
                                          type="checkbox"
                                          className="rounded-sm"
                                          ref={(el) => {
                                            if (el) {
                                              const allIds = cycle.courses.map((c) => c.id);
                                              const selectedInCycle = allIds.filter((id) => selectedCourseIds.has(id));
                                              el.indeterminate = selectedInCycle.length > 0 && selectedInCycle.length < allIds.length;
                                              el.checked = selectedInCycle.length === allIds.length && allIds.length > 0;
                                            }
                                          }}
                                          onChange={() => {
                                            const allIds = cycle.courses.map((c) => c.id);
                                            const allSelected = allIds.every((id) => selectedCourseIds.has(id));
                                            setSelectedCourseIds((prev) => {
                                              const next = new Set(prev);
                                              if (allSelected) {
                                                allIds.forEach((id) => next.delete(id));
                                              } else {
                                                allIds.forEach((id) => next.add(id));
                                              }
                                              return next;
                                            });
                                          }}
                                        />
                                      </th>
                                      <th className="py-1.5 pr-3 font-medium">Code</th>
                                      <th className="py-1.5 pr-3 font-medium">Name</th>
                                      <th className="py-1.5 pr-3 font-medium">Level</th>
                                      <th className="py-1.5 pr-3 font-medium">Group</th>
                                      <th className="py-1.5 pr-3 font-medium">Status</th>
                                      <th className="py-1.5 font-medium" />
                                    </tr>
                                  </thead>
                                  <tbody>
                                    {cycle.courses.map((course) => {
                                      const levelStr = editLevels[course.id] ?? "";
                                      const parsed = levelStr ? parseInt(levelStr, 10) : null;
                                      const displayLevel = parsed && !isNaN(parsed) ? parsed : null;
                                      const dirty = levelStr !== (course.level?.toString() ?? "");

                                      const rootCourseOptions: TypeaheadOption[] = [
                                        { value: "", label: "— None —" },
                                        ...rootCourseGroups.map((g) => ({
                                          value: g.id,
                                          label: g.name,
                                        })),
                                      ];

                                      return (
                                        <tr
                                          key={course.id}
                                          className="border-b border-gray-100 hover:bg-gray-50"
                                        >
                                          <td className="py-1.5 pr-3 w-8">
                                            <input
                                              type="checkbox"
                                              className="rounded-sm"
                                              checked={selectedCourseIds.has(course.id)}
                                              onChange={() => {
                                                setSelectedCourseIds((prev) => {
                                                  const next = new Set(prev);
                                                  if (next.has(course.id)) {
                                                    next.delete(course.id);
                                                  } else {
                                                    next.add(course.id);
                                                  }
                                                  return next;
                                                });
                                              }}
                                            />
                                          </td>
                                          <td className="py-1.5 pr-3 font-mono text-xs">{course.code}</td>
                                          <td className="py-1.5 pr-3 text-xs text-gray-600">{course.name}</td>
                                          <td className="py-1.5 pr-3">
                                            <LevelStepper
                                              value={displayLevel}
                                              onChange={(v) =>
                                                setEditLevels((prev) => ({
                                                  ...prev,
                                                  [course.id]: v?.toString() ?? "",
                                                }))
                                              }
                                            />
                                          </td>
                                          <td className="py-1.5 pr-3">
                                            <TypeaheadSelect
                                              value={course.root_course_group_id ?? ""}
                                              onChange={(val) =>
                                                saveRootCourse(course, val || null)
                                              }
                                              options={rootCourseOptions}
                                              placeholder="Select group"
                                              className="w-40"
                                            />
                                          </td>
                                          <td className="py-1.5 pr-3">
                                            {warning && warnedLevel === displayLevel ? (
                                              <span className="text-xs text-amber-600">{warning}</span>
                                            ) : (
                                              <LevelBadge level={displayLevel} />
                                            )}
                                          </td>
                                          <td className="py-1.5">
                                            <Button
                                              variant="primary"
                                              size="sm"
                                              disabled={!dirty}
                                              loading={savingCourse[course.id]}
                                              onClick={() => saveLevel(course)}
                                            >
                                              Save
                                            </Button>
                                          </td>
                                        </tr>
                                      );
                                    })}
                                  </tbody>
                                </table>
                              </div>
                            );
                          })}
                        </div>
                      ))}
                    </>
                  )}
                </div>
              );
            })
          )}
        </div>
      </div>

      {/* Bulk Edit SlideOver */}
      {bulkEditTarget && (
        <SlideOver
          title={`Bulk Edit Levels - ${bulkEditTarget.label}`}
          onClose={() => setBulkEditTarget(null)}
          footer={
            <div className="flex justify-end gap-2">
              <Button variant="secondary" size="sm" onClick={() => {
                setBulkLevels(Object.fromEntries(bulkEditTarget.courses.map((course) => [course.id, course.level?.toString() ?? ""])));
              }}>
                Reset to Current
              </Button>
              <Button variant="primary" size="sm" loading={savingBulk} onClick={applyBulkEdit}>
                Apply All Changes
              </Button>
            </div>
          }
        >
          <p className="text-sm text-gray-600 mb-3">
            Update levels together, then verify configuration to detect gaps before enabling automated assignment.
          </p>
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200 text-left text-gray-500">
                <th className="py-2 pr-3 font-medium">Course</th>
                <th className="py-2 pr-3 font-medium">Cycle</th>
                <th className="py-2 pr-3 font-medium">Current</th>
                <th className="py-2 font-medium">New Level</th>
              </tr>
            </thead>
            <tbody>
              {bulkEditTarget.courses.map((course) => (
                <tr key={course.id} className="border-b border-gray-100">
                  <td className="py-2 pr-3 font-mono text-xs">{course.code}</td>
                  <td className="py-2 pr-3 text-gray-500">{course.cycle_label}</td>
                  <td className="py-2 pr-3">{course.level ?? "Not set"}</td>
                  <td className="py-2">
                    <LevelStepper
                      value={bulkLevels[course.id] ? parseInt(bulkLevels[course.id], 10) : null}
                      onChange={(v) => setBulkLevels((previous) => ({
                        ...previous,
                        [course.id]: v?.toString() ?? "",
                      }))}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </SlideOver>
      )}

      {/* Slide-over: Manage Groups (Phase 4) */}
      {slideOverOpen && slideOverContent === "groups" && (
        <SlideOver
          title="Manage Groups"
          onClose={() => setSlideOverOpen(false)}
        >
          <RootGroupManagerPanel groupState={groupState} />
        </SlideOver>
      )}

      {/* Returns Desk */}
      <ReturnsDeskPanel
        isOpen={false}
        onClose={() => {}}
        entries={returnsDesk.getGrouped()}
        onRetry={returnsDesk.retryFailure}
        onDismiss={returnsDesk.removeFailure}
        totalCount={returnsDesk.totalCount}
      />
    </div>
  );
}
