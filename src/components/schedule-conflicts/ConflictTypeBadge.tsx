import type { ConflictType } from "@/features/scheduling/types/conflictOverview";

const badgeConfig = {
  room_overlap: { label: "Room", className: "border-amber-200 bg-amber-50 text-amber-800" },
  teacher_overlap: { label: "Teacher", className: "border-red-200 bg-red-50 text-red-700" },
  student_overlap: { label: "Student", className: "border-blue-200 bg-blue-50 text-blue-700" },
} as const satisfies Record<ConflictType, { readonly label: string; readonly className: string }>;

export function ConflictTypeBadge({ type }: Readonly<{ type: ConflictType }>) {
  const config = badgeConfig[type];
  return <span className={`inline-flex rounded-full border px-2 py-0.5 text-xs font-medium ${config.className}`}>{config.label}</span>;
}
