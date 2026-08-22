import { useEffect, useMemo, useState } from "react";
import type { useRootCourseGroups } from "../hooks/useRootCourseGroups";
import type { CourseLevelItem, RootCourseGroupInfo } from "../utils/levels";
import { apiJson } from "../api/client";
import Button from "./ui/Button";
import Input from "./ui/Input";
import GroupCourseAssignmentPanel from "./GroupCourseAssignmentPanel";
import RootGroupList from "./RootGroupList";

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

interface RootGroupManagerPanelProps {
  groupState: ReturnType<typeof useRootCourseGroups>;
  onCourseGroupChanged?: (change: {
    courseId: string;
    previousGroupId: string | null;
    nextGroupId: string;
  }) => void;
}

export default function RootGroupManagerPanel({ groupState, onCourseGroupChanged }: RootGroupManagerPanelProps) {
  const {
    manageGroups,
    manageLoading,
    newGroupName,
    setNewGroupName,
    savingNewGroup,
    setSavingNewGroup,
    editingGroupId,
    setEditingGroupId,
    editingGroupName,
    setEditingGroupName,
    savingEditGroup,
    setSavingEditGroup,
    createGroup,
    renameGroup,
    deleteGroup,
    fetchManageGroups,
    setRootCourseGroups,
    setManageGroups,
  } = groupState;

  const [courses, setCourses] = useState<CourseLevelItem[]>([]);
  const [courseLoading, setCourseLoading] = useState(true);
  const [courseError, setCourseError] = useState<string | null>(null);
  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null);
  const [savingCourseId, setSavingCourseId] = useState<string | null>(null);
  const [managerError, setManagerError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setCourseLoading(true);
    apiJson<CourseLevelItem[]>("/api/v1/admin/course-levels", { method: "GET" })
      .then((data) => {
        if (!cancelled) setCourses(data);
      })
      .catch((error: unknown) => {
        if (!cancelled) setCourseError(errorMessage(error, "Failed to load courses"));
      })
      .finally(() => {
        if (!cancelled) setCourseLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  const selectedGroup = useMemo(
    () => manageGroups.find((group) => group.id === selectedGroupId) ?? null,
    [manageGroups, selectedGroupId],
  );

  async function handleCreate() {
    const name = newGroupName.trim();
    if (!name) return;
    setSavingNewGroup(true);
    setManagerError(null);
    try {
      await createGroup(name);
      setNewGroupName("");
      await fetchManageGroups();
    } catch (error: unknown) {
      setManagerError(errorMessage(error, "Failed to create group"));
    } finally {
      setSavingNewGroup(false);
    }
  }

  async function handleRename(id: string) {
    const name = editingGroupName.trim();
    if (!name) return;
    setSavingEditGroup(true);
    setManagerError(null);
    try {
      await renameGroup(id, name);
      setEditingGroupId(null);
      await fetchManageGroups();
    } catch (error: unknown) {
      setManagerError(errorMessage(error, "Failed to rename group"));
    } finally {
      setSavingEditGroup(false);
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this group? Courses in this group will become ungrouped.")) return;
    setManagerError(null);
    try {
      await deleteGroup(id);
      // Refresh main groups list
      const allGroups = await apiJson<RootCourseGroupInfo[]>("/api/v1/admin/root-course-groups", { method: "GET" });
      setRootCourseGroups(allGroups);
      setCourses((previous) => previous.map((course) => course.root_course_group_id === id
        ? { ...course, root_course_group_id: null, root_course_group_name: null }
        : course));
      setSelectedGroupId((current) => current === id ? null : current);
      await fetchManageGroups();
    } catch (error: unknown) {
      setManagerError(errorMessage(error, "Failed to delete group"));
    }
  }

  async function handleAssignCourse(courseId: string) {
    if (!selectedGroupId || savingCourseId !== null) return;
    const course = courses.find((item) => item.id === courseId);
    if (!course || course.root_course_group_id === selectedGroupId) return;

    setSavingCourseId(courseId);
    setManagerError(null);
    try {
      await apiJson(`/api/v1/admin/courses/${courseId}/root-course-group`, {
        method: "PUT",
        body: JSON.stringify({ root_course_group_id: selectedGroupId }),
      });

      setCourses((previous) => previous.map((item) => item.id === courseId
        ? { ...item, root_course_group_id: selectedGroupId, root_course_group_name: selectedGroup?.name ?? null }
        : item));
      setManageGroups((previous) => previous.map((group) => {
        if (group.id === selectedGroupId) return { ...group, course_count: group.course_count + 1 };
        if (group.id === course.root_course_group_id) return { ...group, course_count: Math.max(0, group.course_count - 1) };
        return group;
      }));
      onCourseGroupChanged?.({
        courseId,
        previousGroupId: course.root_course_group_id,
        nextGroupId: selectedGroupId,
      });
    } catch (error: unknown) {
      setManagerError(errorMessage(error, "Failed to assign course"));
    } finally {
      setSavingCourseId(null);
    }
  }

  return (
    <div className="space-y-4">
      {managerError && (
        <div className="rounded-sm border border-[var(--color-wi-red)]/20 bg-[var(--color-wi-danger-bg)] px-3 py-2 text-sm text-[var(--color-wi-red)]" role="alert">
          {managerError}
        </div>
      )}

      {/* Add new group */}
      <div className="flex items-center gap-2">
        <Input
          type="text"
          aria-label="Group name"
          placeholder="Group name"
          value={newGroupName}
          onChange={(e) => setNewGroupName(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") void handleCreate(); }}
          className="flex-1"
        />
        <Button
          variant="primary"
          size="sm"
          disabled={!newGroupName.trim()}
          loading={savingNewGroup}
          onClick={handleCreate}
        >
          Add
        </Button>
      </div>

      <RootGroupList
        groups={manageGroups}
        manageLoading={manageLoading}
        selectedGroupId={selectedGroupId}
        editingGroupId={editingGroupId}
        editingGroupName={editingGroupName}
        savingEditGroup={savingEditGroup}
        onSelectGroup={setSelectedGroupId}
        onStartRename={(group) => {
          setEditingGroupId(group.id);
          setEditingGroupName(group.name);
        }}
        onChangeEditingName={setEditingGroupName}
        onSaveRename={(id) => void handleRename(id)}
        onCancelRename={() => setEditingGroupId(null)}
        onDelete={(id) => void handleDelete(id)}
      />

      {selectedGroup && (
        courseLoading ? (
          <p className="border-t border-wi-line pt-4 text-sm text-[var(--color-wi-text-light)]" role="status">Loading courses…</p>
        ) : courseError ? (
          <p className="border-t border-wi-line pt-4 text-sm text-[var(--color-wi-red)]" role="alert">{courseError}</p>
        ) : (
          <GroupCourseAssignmentPanel
            group={selectedGroup}
            courses={courses}
            savingCourseId={savingCourseId}
            onAssignCourse={(courseId) => void handleAssignCourse(courseId)}
          />
        )
      )}

      {!selectedGroup && manageGroups.length > 0 && (
        <p className="border-t border-wi-line pt-4 text-center text-sm text-[var(--color-wi-text-light)]">
          Select a group to manage its courses.
        </p>
      )}
    </div>
  );
}
