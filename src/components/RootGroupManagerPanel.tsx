import Button from "./ui/Button";
import type { useRootCourseGroups } from "../hooks/useRootCourseGroups";
import type { GroupWithCount, RootCourseGroupInfo } from "../utils/levels";
import { apiJson } from "../api/client";

const PAGE_SIZE = 10;

interface RootGroupManagerPanelProps {
  groupState: ReturnType<typeof useRootCourseGroups>;
}

export default function RootGroupManagerPanel({ groupState }: RootGroupManagerPanelProps) {
  const {
    manageGroups,
    manageLoading,
    managePage,
    setManagePage,
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
  } = groupState;

  async function handleCreate() {
    const name = newGroupName.trim();
    if (!name) return;
    setSavingNewGroup(true);
    try {
      await createGroup(name);
      setNewGroupName("");
      await fetchManageGroups();
    } catch {
      // error handled upstream via toast
    } finally {
      setSavingNewGroup(false);
    }
  }

  async function handleRename(id: string) {
    const name = editingGroupName.trim();
    if (!name) return;
    setSavingEditGroup(true);
    try {
      await renameGroup(id, name);
      setEditingGroupId(null);
      await fetchManageGroups();
    } catch {
      // error handled upstream
    } finally {
      setSavingEditGroup(false);
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this group? Courses in this group will become ungrouped.")) return;
    try {
      await deleteGroup(id);
      // Refresh main groups list
      const allGroups = await apiJson<RootCourseGroupInfo[]>("/api/v1/admin/root-course-groups", { method: "GET" });
      setRootCourseGroups(allGroups);
      await fetchManageGroups();
    } catch {
      // error handled upstream
    }
  }

  const pageCount = Math.ceil(manageGroups.length / PAGE_SIZE);
  const paged = manageGroups.slice(managePage * PAGE_SIZE, (managePage + 1) * PAGE_SIZE);

  return (
    <div className="space-y-4">
      {/* Add new group */}
      <div className="flex items-center gap-2">
        <input
          type="text"
          placeholder="Group name"
          value={newGroupName}
          onChange={(e) => setNewGroupName(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") handleCreate(); }}
          className="flex-1 px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
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

      {/* Group list */}
      {manageLoading ? (
        <div className="text-sm text-gray-400 py-4 text-center">Loading…</div>
      ) : manageGroups.length === 0 ? (
        <div className="text-sm text-gray-400 py-4 text-center">No groups yet</div>
      ) : (
        <>
          <div className="border border-gray-200 rounded-sm">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
                  <th className="py-2 px-3 font-medium">Name</th>
                  <th className="py-2 px-3 font-medium w-24">Courses</th>
                  <th className="py-2 px-3 font-medium w-32" />
                </tr>
              </thead>
              <tbody>
                {paged.map((g: GroupWithCount) => (
                  <tr key={g.id} className="border-b border-gray-100 hover:bg-gray-50">
                    <td className="py-2 px-3">
                      {editingGroupId === g.id ? (
                        <div className="flex items-center gap-1">
                          <input
                            type="text"
                            value={editingGroupName}
                            onChange={(e) => setEditingGroupName(e.target.value)}
                            onKeyDown={(e) => {
                              if (e.key === "Enter") handleRename(g.id);
                              if (e.key === "Escape") setEditingGroupId(null);
                            }}
                            className="flex-1 px-2 py-1 text-sm border border-gray-300 rounded-sm"
                            autoFocus
                          />
                          <button
                            onClick={() => handleRename(g.id)}
                            disabled={!editingGroupName.trim() || savingEditGroup}
                            className="text-xs text-blue-600 hover:text-blue-800 px-1"
                          >
                            Save
                          </button>
                          <button
                            onClick={() => setEditingGroupId(null)}
                            className="text-xs text-gray-500 hover:text-gray-700 px-1"
                          >
                            Cancel
                          </button>
                        </div>
                      ) : (
                        <span className="text-gray-800">{g.name}</span>
                      )}
                    </td>
                    <td className="py-2 px-3 text-gray-500 text-xs">{g.course_count}</td>
                    <td className="py-2 px-3 text-right">
                      <button
                        onClick={() => {
                          setEditingGroupId(g.id);
                          setEditingGroupName(g.name);
                        }}
                        className="text-xs text-blue-600 hover:text-blue-800 mr-3"
                      >
                        Rename
                      </button>
                      <button
                        onClick={() => handleDelete(g.id)}
                        className="text-xs text-red-600 hover:text-red-800"
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {manageGroups.length > PAGE_SIZE && (
            <div className="flex items-center justify-center gap-2 mt-3">
              <Button
                variant="secondary"
                size="sm"
                disabled={managePage === 0}
                onClick={() => setManagePage((p) => Math.max(0, p - 1))}
              >
                Previous
              </Button>
              <span className="text-xs text-gray-500">
                Page {managePage + 1} of {pageCount}
              </span>
              <Button
                variant="secondary"
                size="sm"
                disabled={managePage >= pageCount - 1}
                onClick={() => setManagePage((p) => p + 1)}
              >
                Next
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
