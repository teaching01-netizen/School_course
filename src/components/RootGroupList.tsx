import type { GroupWithCount } from "../utils/levels";

interface RootGroupListProps {
  groups: GroupWithCount[];
  manageLoading: boolean;
  selectedGroupId: string | null;
  editingGroupId: string | null;
  editingGroupName: string;
  savingEditGroup: boolean;
  onSelectGroup: (id: string) => void;
  onStartRename: (group: GroupWithCount) => void;
  onChangeEditingName: (name: string) => void;
  onSaveRename: (id: string) => void;
  onCancelRename: () => void;
  onDelete: (id: string) => void;
}

export default function RootGroupList({
  groups,
  manageLoading,
  selectedGroupId,
  editingGroupId,
  editingGroupName,
  savingEditGroup,
  onSelectGroup,
  onStartRename,
  onChangeEditingName,
  onSaveRename,
  onCancelRename,
  onDelete,
}: RootGroupListProps) {
  if (manageLoading) {
    return <div className="py-4 text-center text-sm text-[var(--color-wi-text-light)]">Loading…</div>;
  }

  if (groups.length === 0) {
    return <div className="py-4 text-center text-sm text-[var(--color-wi-text-light)]">No groups yet</div>;
  }

  return (
    <div className="max-h-[42vh] overflow-y-auto rounded-sm border border-wi-line notion-scrollbar">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-wi-line bg-[var(--color-wi-row-alt)] text-left text-[var(--color-wi-text-light)]">
            <th scope="col" className="px-3 py-2 font-medium">Name</th>
            <th scope="col" className="w-24 px-3 py-2 font-medium">Courses</th>
            <th scope="col" className="w-32 px-3 py-2 font-medium" />
          </tr>
        </thead>
        <tbody>
          {groups.map((group) => (
            <tr
              key={group.id}
              className={`border-b border-wi-line-soft ${selectedGroupId === group.id ? "bg-[var(--color-wi-selected)]" : "hover:bg-[var(--color-wi-row-alt)]"}`}
            >
              <td className="px-3 py-2">
                {editingGroupId === group.id ? (
                  <div className="flex items-center gap-1">
                    <input
                      type="text"
                      value={editingGroupName}
                      onChange={(event) => onChangeEditingName(event.target.value)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter") onSaveRename(group.id);
                        if (event.key === "Escape") onCancelRename();
                      }}
                      className="flex-1 rounded-sm border border-wi-line px-2 py-1 text-sm"
                      autoFocus
                    />
                    <button
                      type="button"
                      onClick={() => onSaveRename(group.id)}
                      disabled={!editingGroupName.trim() || savingEditGroup}
                      className="px-1 text-xs text-[var(--color-wi-primary)] hover:text-[var(--color-wi-primary-dark)]"
                    >
                      Save
                    </button>
                    <button
                      type="button"
                      onClick={onCancelRename}
                      className="px-1 text-xs text-[var(--color-wi-text-light)]"
                    >
                      Cancel
                    </button>
                  </div>
                ) : (
                  <button
                    type="button"
                    aria-label={`Select group ${group.name}`}
                    aria-pressed={selectedGroupId === group.id}
                    onClick={() => onSelectGroup(group.id)}
                    className="rounded-sm text-left font-medium text-[var(--color-wi-text)] hover:text-[var(--color-wi-primary)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]/40"
                  >
                    {group.name}
                  </button>
                )}
              </td>
              <td className="px-3 py-2 text-xs text-[var(--color-wi-text-light)]">{group.course_count}</td>
              <td className="px-3 py-2 text-right">
                <button
                  type="button"
                  onClick={() => onStartRename(group)}
                  className="mr-3 text-xs text-[var(--color-wi-primary)] hover:text-[var(--color-wi-primary-dark)]"
                >
                  Rename
                </button>
                <button
                  type="button"
                  onClick={() => onDelete(group.id)}
                  className="text-xs text-[var(--color-wi-red)] hover:text-[var(--color-wi-red-dark)]"
                >
                  Delete
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
