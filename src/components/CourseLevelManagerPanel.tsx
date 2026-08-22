import { useEffect, useMemo, useState } from "react";
import { apiJson } from "../api/client";
import { detectGaps } from "../utils/levels";
import type { CourseLevelItem, RootCourseGroupInfo } from "../utils/levels";
import { useToast } from "../hooks/useToast";
import Button from "./ui/Button";
import Input from "./ui/Input";
import SearchableSelect from "./ui/SearchableSelect";
import TypeaheadSelect from "./TypeaheadSelect";
import type { SitInRule } from "../types";

const UNGROUPED_ID = "__ungrouped__";

type CourseLevelManagerPanelProps = {
  courses: CourseLevelItem[];
  groups: RootCourseGroupInfo[];
  rules: SitInRule[];
  onCoursesChange: (update: (courses: CourseLevelItem[]) => CourseLevelItem[]) => void;
  onGroupsChange: (update: (groups: RootCourseGroupInfo[]) => RootCourseGroupInfo[]) => void;
};

type LevelStatus = "ready" | "attention" | "empty";

function levelStatus(level: number | null): { label: string; className: string } {
  if (level === null) return { label: "Not set", className: "border-wi-line bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]" };
  if (level === 1) return { label: "Zoom", className: "border-[var(--color-wi-primary)]/20 bg-[var(--color-wi-callout)] text-[var(--color-wi-primary)]" };
  return { label: "Eligible", className: "border-[var(--color-wi-green)]/20 bg-[var(--color-wi-callout)] text-[var(--color-wi-green)]" };
}

function statusDot(status: LevelStatus): string {
  if (status === "ready") return "bg-[var(--color-wi-green)]";
  if (status === "attention") return "bg-[var(--color-wi-amber)]";
  return "bg-[var(--color-wi-faint)]";
}

function groupStatus(courses: CourseLevelItem[], hasSitInRule = true): { status: LevelStatus; label: string; gaps: number } {
  if (courses.length === 0) return { status: "empty", label: "Empty", gaps: 0 };
  const cycles = new Map<string, CourseLevelItem[]>();
  for (const course of courses) {
    const cycleCourses = cycles.get(course.cycle_id) ?? [];
    cycleCourses.push(course);
    cycles.set(course.cycle_id, cycleCourses);
  }
  const gaps = [...cycles.values()].reduce((total, cycleCourses) => total + detectGaps(cycleCourses).length, 0);
  const hasUnset = courses.some((course) => course.level === null);
  if (courses.some((course) => course.level !== null) && !hasSitInRule) return { status: "attention", label: "Rule needed", gaps };
  if (hasUnset || gaps > 0) return { status: "attention", label: "Needs attention", gaps };
  return { status: "ready", label: "Ready", gaps: 0 };
}

function groupCourses(courses: CourseLevelItem[], groupId: string): CourseLevelItem[] {
  return courses
    .filter((course) => groupId === UNGROUPED_ID ? course.root_course_group_id === null : course.root_course_group_id === groupId)
    .sort((a, b) => {
      if (a.level === null && b.level !== null) return 1;
      if (a.level !== null && b.level === null) return -1;
      if (a.level !== null && b.level !== null && a.level !== b.level) return a.level - b.level;
      return a.code.localeCompare(b.code);
    });
}

function StatusBadge({ level }: { level: number | null }) {
  const status = levelStatus(level);
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full border px-2 py-1 text-xs font-medium ${status.className}`}>
      <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
      {status.label}
    </span>
  );
}

export default function CourseLevelManagerPanel({ courses, groups, rules, onCoursesChange, onGroupsChange }: CourseLevelManagerPanelProps) {
  const { addToast } = useToast();
  const [selectedGroupId, setSelectedGroupId] = useState("");
  const [newGroupName, setNewGroupName] = useState("");
  const [editingGroupId, setEditingGroupId] = useState<string | null>(null);
  const [editingGroupName, setEditingGroupName] = useState("");
  const [groupError, setGroupError] = useState<string | null>(null);
  const [savingGroup, setSavingGroup] = useState(false);
  const [savingCourseId, setSavingCourseId] = useState<string | null>(null);
  const [savingSitInRule, setSavingSitInRule] = useState(false);
  const [draftLevels, setDraftLevels] = useState<Record<string, string>>({});
  const [newLevelCourseId, setNewLevelCourseId] = useState("");
  const [newLevel, setNewLevel] = useState("");

  const groupOptions = useMemo(() => {
    const ungroupedCount = courses.filter((course) => course.root_course_group_id === null).length;
    return [
      ...groups.map((group) => ({ id: group.id, name: group.name, courseCount: courses.filter((course) => course.root_course_group_id === group.id).length, virtual: false })),
      { id: UNGROUPED_ID, name: "Unassigned", courseCount: ungroupedCount, virtual: true },
    ];
  }, [courses, groups]);

  const selectedGroup = groupOptions.find((group) => group.id === selectedGroupId) ?? groupOptions[0] ?? null;
  const selectedRootGroup = selectedGroup && !selectedGroup.virtual ? groups.find((group) => group.id === selectedGroup.id) ?? null : null;
  const selectedCourses = useMemo(
    () => selectedGroup ? groupCourses(courses, selectedGroup.id) : [],
    [courses, selectedGroup],
  );
  const unassignedCourses = useMemo(
    () => selectedCourses.filter((course) => course.level === null),
    [selectedCourses],
  );

  useEffect(() => {
    if (selectedGroupId && groupOptions.some((group) => group.id === selectedGroupId)) return;
    setSelectedGroupId(groupOptions[0]?.id ?? UNGROUPED_ID);
  }, [groupOptions, selectedGroupId]);

  useEffect(() => {
    setDraftLevels(Object.fromEntries(courses.map((course) => [course.id, course.level?.toString() ?? ""])));
  }, [courses]);

  const suggestedCourse = unassignedCourses[0] ?? selectedCourses[0];
  const highestLevel = selectedCourses.reduce((highest, course) => Math.max(highest, course.level ?? 0), 0);
  const activeNewLevelCourseId = newLevelCourseId || suggestedCourse?.id || "";
  const activeNewLevel = newLevel || String(highestLevel + 1);
  const selectedSitInRule = rules.find((rule) => rule.id === selectedRootGroup?.sit_in_rule_id) ?? null;
  const selectedGroupHasLevels = selectedCourses.some((course) => course.level !== null);
  const selectedGroupNeedsSitInRule = Boolean(selectedRootGroup && selectedGroupHasLevels && !selectedRootGroup.sit_in_rule_id);
  const selectedGroupCanAssignLevels = selectedGroup?.virtual || Boolean(selectedRootGroup?.sit_in_rule_id);

  const summary = useMemo(() => {
    const cycles = new Map<string, CourseLevelItem[]>();
    for (const course of courses) {
      const key = `${course.root_course_group_id ?? UNGROUPED_ID}:${course.cycle_id}`;
      const cycleCourses = cycles.get(key) ?? [];
      cycleCourses.push(course);
      cycles.set(key, cycleCourses);
    }
    return {
      totalCourses: courses.length,
      assigned: courses.filter((course) => course.level !== null).length,
      gaps: [...cycles.values()].reduce((total, cycleCourses) => total + detectGaps(cycleCourses).length, 0),
      notSet: courses.filter((course) => course.level === null).length,
    };
  }, [courses]);

  async function saveLevel(course: CourseLevelItem, rawLevel: string) {
    const level = rawLevel.trim() === "" ? null : Number(rawLevel);
    if (level !== null && (!Number.isInteger(level) || level < 1)) {
      addToast("error", "Level must be a whole number greater than zero");
      return;
    }
    const courseGroup = course.root_course_group_id ? groups.find((group) => group.id === course.root_course_group_id) : null;
    if (level !== null && courseGroup && !courseGroup.sit_in_rule_id) {
      addToast("error", "Assign a sit-in rule to this course group before saving a level");
      return;
    }
    setSavingCourseId(course.id);
    try {
      await apiJson(`/api/v1/admin/courses/${course.id}/level`, {
        method: "PUT",
        body: JSON.stringify({ level, cycle_id: course.cycle_id }),
      });
      onCoursesChange((previous) => previous.map((item) => item.id === course.id ? { ...item, level } : item));
      addToast("success", `${course.code} level saved`);
    } catch (error) {
      addToast("error", error instanceof Error ? error.message : "Failed to save level");
    } finally {
      setSavingCourseId(null);
    }
  }

  async function saveSitInRule(ruleId: string | null) {
    if (!selectedRootGroup || !ruleId) {
      if (selectedRootGroup) addToast("error", "A sit-in rule is required for levelled course groups");
      return;
    }
    setSavingSitInRule(true);
    try {
      await apiJson(`/api/v1/admin/root-course-groups/${selectedRootGroup.id}`, {
        method: "PUT",
        body: JSON.stringify({ sit_in_rule_id: ruleId }),
      });
      onGroupsChange((previous) => previous.map((group) => group.id === selectedRootGroup.id ? { ...group, sit_in_rule_id: ruleId } : group));
      addToast("success", "Sit-in rule assigned");
    } catch (error) {
      addToast("error", error instanceof Error ? error.message : "Failed to update sit-in rule");
    } finally {
      setSavingSitInRule(false);
    }
  }

  async function addLevel() {
    const course = selectedCourses.find((item) => item.id === activeNewLevelCourseId);
    if (course) {
      await saveLevel(course, activeNewLevel);
      setNewLevelCourseId("");
      setNewLevel("");
    }
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

  async function deleteGroup(groupId: string) {
    if (!window.confirm("Delete this group? Its courses will become unassigned.")) return;
    setGroupError(null);
    try {
      await apiJson(`/api/v1/admin/root-course-groups/${groupId}`, { method: "DELETE" });
      onGroupsChange((previous) => previous.filter((group) => group.id !== groupId));
      onCoursesChange((previous) => previous.map((course) => course.root_course_group_id === groupId ? { ...course, root_course_group_id: null, root_course_group_name: null } : course));
      setSelectedGroupId(UNGROUPED_ID);
      setNewLevelCourseId("");
      setNewLevel("");
      addToast("success", "Course group deleted");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to delete course group";
      setGroupError(message);
      addToast("error", message);
    }
  }

  async function assignCourse(courseId: string) {
    if (!selectedGroup || selectedGroup.virtual || savingCourseId) return;
    const course = courses.find((item) => item.id === courseId);
    if (!course || course.root_course_group_id === selectedGroup.id) return;
    if (course.level !== null && !selectedRootGroup?.sit_in_rule_id) {
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
    const nextGroup = groups.find((group) => group.id === groupId) ?? null;
    const nextGroupId = groupId || null;
    if (course.root_course_group_id === nextGroupId) return;
    if (course.level !== null && nextGroup && !nextGroup.sit_in_rule_id) {
      addToast("error", "Assign a sit-in rule before moving a levelled course into this group");
      return;
    }
    setSavingCourseId(course.id);
    try {
      await apiJson(`/api/v1/admin/courses/${course.id}/root-course-group`, { method: "PUT", body: JSON.stringify({ root_course_group_id: nextGroupId }) });
      onCoursesChange((previous) => previous.map((item) => item.id === course.id ? { ...item, root_course_group_id: nextGroupId, root_course_group_name: nextGroup?.name ?? null } : item));
      addToast("success", `${course.code} group updated`);
    } catch (error) {
      addToast("error", error instanceof Error ? error.message : "Failed to update course group");
    } finally {
      setSavingCourseId(null);
    }
  }

  const courseOptions = courses.map((course) => ({
    value: course.id,
    label: `${course.code} — ${course.name}`,
    description: `${course.subject_name} · ${course.cycle_label}`,
    keywords: `${course.code} ${course.name} ${course.subject_code} ${course.subject_name}`,
    disabled: course.root_course_group_id === selectedGroup?.id,
  }));
  const groupCourseOptions = [{ value: "", label: "— Unassigned —" }, ...groups.map((group) => ({ value: group.id, label: group.name }))];

  return (
    <div className="space-y-4">
      {groupError ? <div className="rounded-sm border border-[var(--color-wi-red)]/20 bg-[var(--color-wi-danger-bg)] px-3 py-2 text-sm text-[var(--color-wi-red)]" role="alert">{groupError}</div> : null}

      <section className="grid grid-cols-2 gap-2 rounded-md border border-wi-line bg-white p-3 sm:grid-cols-4" aria-label="Course level status summary">
        <SummaryMetric label="Total courses" value={summary.totalCourses} tone="blue" />
        <SummaryMetric label="Assigned levels" value={summary.assigned} tone="green" />
        <SummaryMetric label="Gaps" value={summary.gaps} tone="amber" />
        <SummaryMetric label="Not set" value={summary.notSet} tone="red" />
      </section>

      <div className="grid min-h-[28rem] gap-4 lg:grid-cols-[15rem_minmax(0,1fr)]">
        <aside className="overflow-hidden rounded-md border border-wi-line bg-white" aria-label="Course groups">
          <div className="flex items-center justify-between border-b border-wi-line px-3 py-3"><h3 className="text-sm font-semibold text-[var(--color-wi-text)]">Course groups</h3><span className="rounded bg-[var(--color-wi-row-alt)] px-1.5 py-0.5 text-xs text-[var(--color-wi-text-light)]">{groupOptions.length}</span></div>
          <div className="max-h-[25rem] overflow-y-auto">
            {groupOptions.map((group) => {
              const groupInfo = group.virtual ? null : groups.find((item) => item.id === group.id);
              const details = groupStatus(groupCourses(courses, group.id), group.virtual || Boolean(groupInfo?.sit_in_rule_id));
              const isSelected = group.id === selectedGroup?.id;
              return (
                <div key={group.id} className={`border-b border-wi-line-soft ${isSelected ? "bg-[var(--color-wi-selected)]" : "hover:bg-[var(--color-wi-row-alt)]"}`}>
                  <div className="flex items-center gap-2 px-3 py-2.5">
                    <span className={`h-2 w-2 shrink-0 rounded-full ${statusDot(details.status)}`} aria-hidden="true" />
                    {editingGroupId === group.id ? <Input aria-label={`Rename ${group.name}`} value={editingGroupName} onChange={(event) => setEditingGroupName(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") void renameGroup(group.id); if (event.key === "Escape") setEditingGroupId(null); }} size="sm" autoFocus className="min-w-0" /> : <button type="button" onClick={() => { setSelectedGroupId(group.id); setNewLevelCourseId(""); setNewLevel(""); }} aria-pressed={isSelected} className={`min-w-0 flex-1 truncate text-left text-sm ${isSelected ? "font-semibold text-[var(--color-wi-primary)]" : "text-[var(--color-wi-text)]"}`}>{group.name}</button>}
                    <span className="shrink-0 rounded bg-[var(--color-wi-row-alt)] px-1.5 py-0.5 text-xs text-[var(--color-wi-text-light)]">{group.courseCount}</span>
                  </div>
                  {!group.virtual ? <div className="flex justify-end gap-2 px-3 pb-2">{editingGroupId === group.id ? <><button type="button" onClick={() => void renameGroup(group.id)} disabled={savingGroup} className="text-xs font-medium text-[var(--color-wi-primary)]">Save</button><button type="button" onClick={() => setEditingGroupId(null)} className="text-xs text-[var(--color-wi-text-light)]">Cancel</button></> : <><button type="button" onClick={() => { setEditingGroupId(group.id); setEditingGroupName(group.name); }} className="text-xs text-[var(--color-wi-primary)]">Rename</button><button type="button" onClick={() => void deleteGroup(group.id)} className="text-xs text-[var(--color-wi-red)]">Delete</button></>}</div> : null}
                </div>
              );
            })}
          </div>
          <div className="border-t border-wi-line p-3"><label htmlFor="new-course-group" className="sr-only">New course group name</label><div className="flex gap-2"><Input id="new-course-group" value={newGroupName} onChange={(event) => setNewGroupName(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") void createGroup(); }} placeholder="New group name" size="sm" /><Button size="sm" onClick={() => void createGroup()} disabled={!newGroupName.trim()} loading={savingGroup}>Add</Button></div></div>
        </aside>

        <section className="min-w-0 rounded-md border border-wi-line bg-white" aria-labelledby="selected-course-group-heading">
          {selectedGroup ? <>
            <div className="border-b border-wi-line px-4 py-4">
              <div className="flex flex-wrap items-start justify-between gap-3"><div><p className="text-xs font-medium uppercase tracking-[0.12em] text-[var(--color-wi-text-light)]">Selected group</p><h3 id="selected-course-group-heading" className="mt-1 text-lg font-semibold text-[var(--color-wi-text)]">{selectedGroup.name}</h3><p className="mt-1 text-xs text-[var(--color-wi-text-light)]">Edit level assignments and status for every course in this group.</p></div><span className="inline-flex items-center gap-1.5 rounded-full border border-wi-line px-2.5 py-1 text-xs font-medium text-[var(--color-wi-text-light)]"><span className={`h-1.5 w-1.5 rounded-full ${statusDot(groupStatus(selectedCourses, selectedGroup.virtual || Boolean(selectedRootGroup?.sit_in_rule_id)).status)}`} aria-hidden="true" />{groupStatus(selectedCourses, selectedGroup.virtual || Boolean(selectedRootGroup?.sit_in_rule_id)).label}</span></div>
              {!selectedGroup.virtual ? <div className={`mt-4 rounded-sm border px-3 py-3 ${selectedGroupNeedsSitInRule ? "border-[var(--color-wi-amber)]/40 bg-amber-50" : "border-wi-line bg-[var(--color-wi-row-alt)]"}`} aria-label="Sit-in rule configuration"><div className="flex flex-wrap items-start justify-between gap-3"><div className="min-w-0 flex-1"><p className={`text-sm font-semibold ${selectedGroupNeedsSitInRule ? "text-amber-900" : "text-[var(--color-wi-text)]"}`}>{selectedGroupNeedsSitInRule ? "Sit-in rule required" : "Sit-in rule"}</p><p className={`mt-1 text-xs ${selectedGroupNeedsSitInRule ? "text-amber-800" : "text-[var(--color-wi-text-light)]"}`}>{selectedGroupNeedsSitInRule ? "Levelled courses need a sit-in rule before this group can be marked Ready. The rule determines the sit-in destination." : selectedSitInRule ? `Configured: ${selectedSitInRule.name}` : "No rule assigned. Students cannot be matched to a sit-in destination."}</p></div><div className="w-full sm:w-64"><label htmlFor="selected-group-sit-in-rule" className="sr-only">Sit-in rule for {selectedGroup.name}</label><SearchableSelect id="selected-group-sit-in-rule" aria-label={`Sit-in rule for ${selectedGroup.name}`} value={selectedRootGroup?.sit_in_rule_id ?? ""} onChange={(event) => void saveSitInRule(event.target.value || null)} disabled={savingSitInRule || rules.length === 0} size="sm">{!selectedRootGroup?.sit_in_rule_id ? <option value="">No rule assigned</option> : null}{rules.map((rule) => <option key={rule.id} value={rule.id}>{rule.name}</option>)}</SearchableSelect>{rules.length === 0 ? <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">No sit-in rules are available. Create one in Sit-in Rules first.</p> : null}</div></div></div> : null}
              <div className="mt-3 rounded-sm bg-[var(--color-wi-row-alt)] p-3"><div className="flex flex-wrap items-end gap-3"><div className="min-w-[16rem] flex-1"><label htmlFor="add-level-course" className="mb-1 block text-xs font-medium text-[var(--color-wi-text-light)]">Add level to an unassigned course</label><TypeaheadSelect id="add-level-course" value={activeNewLevelCourseId} onChange={setNewLevelCourseId} options={selectedCourses.map((course) => ({ value: course.id, label: `${course.code} — ${course.name}`, keywords: `${course.code} ${course.name} ${course.subject_name}`, disabled: course.level !== null }))} placeholder={!selectedGroupCanAssignLevels ? "Assign a sit-in rule first" : unassignedCourses.length > 0 ? "Choose a course" : "All courses have levels"} disabled={!selectedGroupCanAssignLevels || unassignedCourses.length === 0 || savingCourseId !== null} /></div><div className="w-28"><label htmlFor="add-level-number" className="mb-1 block text-xs font-medium text-[var(--color-wi-text-light)]">Level number</label><Input id="add-level-number" type="number" min={1} step={1} value={activeNewLevel} onChange={(event) => setNewLevel(event.target.value)} disabled={!selectedGroupCanAssignLevels || unassignedCourses.length === 0 || savingCourseId !== null} /></div><Button onClick={() => void addLevel()} disabled={!selectedGroupCanAssignLevels || !activeNewLevelCourseId || unassignedCourses.length === 0 || savingCourseId !== null} loading={savingCourseId === activeNewLevelCourseId}>Add level</Button></div><p className="mt-2 text-xs text-[var(--color-wi-text-light)]">{selectedGroupCanAssignLevels ? "Assign a level to an unassigned course. Existing rows can be edited below." : "Assign a sit-in rule before adding or editing course levels."}</p></div>
              {!selectedGroup.virtual ? <div className="mt-3 flex flex-wrap items-center gap-2"><span className="text-xs font-medium text-[var(--color-wi-text-light)]">Add course to {selectedGroup.name}</span><TypeaheadSelect id="add-course-to-group" aria-label={`Add course to ${selectedGroup.name}`} value="" onChange={(value) => void assignCourse(value)} options={courseOptions} placeholder="Search by course code or subject" disabled={savingCourseId !== null} className="min-w-[min(24rem,100%)] flex-1" /></div> : null}
            </div>
            <div className="overflow-x-auto"><table className="min-w-[44rem] w-full text-sm"><caption className="sr-only">Course levels in {selectedGroup.name}</caption><thead className="bg-[var(--color-wi-row-alt)] text-left text-xs text-[var(--color-wi-text-light)]"><tr><th scope="col" className="px-4 py-2.5 font-medium">Level</th><th scope="col" className="px-4 py-2.5 font-medium">Course</th><th scope="col" className="px-4 py-2.5 font-medium">Cycle</th><th scope="col" className="px-4 py-2.5 font-medium">Subject</th><th scope="col" className="px-4 py-2.5 font-medium">Status</th><th scope="col" className="px-4 py-2.5 text-right font-medium">Actions</th></tr></thead><tbody>
              {selectedCourses.map((course) => { const draftLevel = draftLevels[course.id] ?? ""; const dirty = draftLevel !== (course.level?.toString() ?? ""); const levelValue = draftLevel === "" ? null : Number(draftLevel); return <tr key={course.id} className="border-t border-wi-line-soft hover:bg-[var(--color-wi-row-alt)]"><td className="px-4 py-3"><Input aria-label={`Level for ${course.code}`} type="number" min={1} step={1} value={draftLevel} onChange={(event) => setDraftLevels((previous) => ({ ...previous, [course.id]: event.target.value }))} size="sm" className="w-20 border-[var(--color-wi-primary)]" /></td><td className="px-4 py-3"><p className="font-mono text-xs font-semibold text-[var(--color-wi-text)]">{course.code}</p><p className="mt-0.5 text-xs text-[var(--color-wi-text-light)]">{course.name}</p></td><td className="px-4 py-3 text-xs text-[var(--color-wi-text-light)]">{course.cycle_label}</td><td className="px-4 py-3 text-xs text-[var(--color-wi-text-light)]">{course.subject_name}</td><td className="px-4 py-3"><StatusBadge level={Number.isInteger(levelValue) ? levelValue : null} /></td><td className="px-4 py-3 text-right"><div className="flex items-center justify-end gap-2"><Button variant="secondary" size="sm" disabled={!dirty} loading={savingCourseId === course.id} onClick={() => void saveLevel(course, draftLevel)}>Save</Button><TypeaheadSelect aria-label={`Move ${course.code} to group`} value={course.root_course_group_id ?? ""} onChange={(value) => void moveCourse(course, value)} options={groupCourseOptions} placeholder="Group" disabled={savingCourseId === course.id} className="w-32" /></div></td></tr>; })}
              {selectedCourses.length === 0 ? <tr><td colSpan={6} className="px-4 py-12 text-center text-sm text-[var(--color-wi-text-light)]">No courses in this group yet.</td></tr> : null}
            </tbody></table></div>
          </> : <div className="flex min-h-[28rem] items-center justify-center p-8 text-sm text-[var(--color-wi-text-light)]">Add a course group to start managing levels.</div>}
        </section>
      </div>
    </div>
  );
}

function SummaryMetric({ label, value, tone }: { label: string; value: number; tone: "blue" | "green" | "amber" | "red" }) {
  const toneClass = { blue: "bg-[var(--color-wi-primary)]", green: "bg-[var(--color-wi-green)]", amber: "bg-[var(--color-wi-amber)]", red: "bg-[var(--color-wi-red)]" }[tone];
  return <div className="flex items-center gap-2 border-r border-wi-line px-2 last:border-r-0"><span className={`h-2 w-2 shrink-0 rounded-full ${toneClass}`} aria-hidden="true" /><div><p className="text-base font-semibold tabular-nums text-[var(--color-wi-text)]">{value}</p><p className="text-xs text-[var(--color-wi-text-light)]">{label}</p></div></div>;
}
