import { UNGROUPED_KEY } from "../utils/levels";

interface RootCourseGroupRailProps {
  rootGroups: Array<{
    rootCourseGroupId: string | null;
    label: string;
    courseCount?: number;
    assignedCount?: number;
  }>;
  selectedRootGroupId: string | null;
  onSelectRootGroup: (id: string | null) => void;
}

export default function RootCourseGroupRail({
  rootGroups,
  selectedRootGroupId,
  onSelectRootGroup,
}: RootCourseGroupRailProps) {
  return (
    <nav className="w-48 shrink-0 border-r border-gray-200 pr-3" aria-label="Root Course Groups">
      <div className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-2 px-2">
        Root Course Groups
      </div>
      <ul className="space-y-0.5">
        <li>
          <button
            onClick={() => onSelectRootGroup(null)}
            className={`w-full text-left px-2 py-1.5 text-sm rounded-sm transition-colors ${
              selectedRootGroupId === null
                ? "bg-blue-50 text-blue-700 font-medium"
                : "text-gray-600 hover:bg-gray-50"
            }`}
          >
            All Groups
          </button>
        </li>
        {rootGroups.map((g) => {
          const isSelected = g.rootCourseGroupId === selectedRootGroupId;
          const total = g.courseCount ?? 0;
          const assigned = g.assignedCount ?? 0;
          const complete = total > 0 && assigned === total;
          const partial = assigned > 0 && assigned < total;
          return (
            <li key={g.rootCourseGroupId ?? UNGROUPED_KEY}>
              <button
                onClick={() => onSelectRootGroup(g.rootCourseGroupId)}
                className={`w-full text-left px-2 py-1.5 text-sm rounded-sm transition-colors ${
                  isSelected
                    ? "bg-blue-50 text-blue-700 font-medium"
                    : "text-gray-600 hover:bg-gray-50"
                }`}
              >
                <div className="flex items-center justify-between">
                  <span>{g.label}</span>
                  {total > 0 && (
                    <span
                      className={`text-[10px] font-medium px-1.5 py-0.5 rounded-full ${
                        complete
                          ? "bg-green-100 text-green-700"
                          : partial
                            ? "bg-amber-100 text-amber-700"
                            : "bg-gray-100 text-gray-500"
                      }`}
                    >
                      {assigned}/{total}
                    </span>
                  )}
                </div>
              </button>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
