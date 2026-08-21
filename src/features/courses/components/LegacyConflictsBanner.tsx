import { TriangleAlert } from "lucide-react";
import type { LegacyCourseConflict } from "../types";

type Props = { conflicts: LegacyCourseConflict[] };

export function LegacyConflictsBanner({ conflicts }: Props) {
  if (conflicts.length === 0) return null;
  return (
    <div role="alert" className="rounded border border-amber-200 bg-amber-50 px-4 py-3 mb-4">
      <div className="flex items-start gap-3">
        <TriangleAlert className="h-5 w-5 text-amber-600 mt-0.5" />
        <div>
          <p className="font-medium text-amber-800">
            Legacy sync conflicts ({conflicts.length} open)
          </p>
          <ul className="mt-1 text-sm text-amber-700 space-y-1">
            {conflicts.slice(0, 5).map((c) => (
              <li key={c.id}>{c.conflict_type}: {c.message ?? c.category}</li>
            ))}
          </ul>
          <a href="/admin/legacy-sync" className="mt-2 inline-block text-sm text-amber-800 underline">
            View all in Legacy sync health
          </a>
        </div>
      </div>
    </div>
  );
}