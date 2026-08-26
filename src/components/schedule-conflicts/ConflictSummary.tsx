import { AlertTriangle, DoorOpen, GraduationCap, Users } from "lucide-react";
import type { ConflictSummaryResponse } from "@/features/scheduling/types/conflictOverview";

export function ConflictSummary({ summary }: Readonly<{ summary: ConflictSummaryResponse }>) {
  const cards = [
    { label: summary.total_conflicts === 1 ? "total conflict" : "total conflicts", value: summary.total_conflicts, icon: AlertTriangle, tone: "text-[var(--color-wi-red)]" },
    { label: "room overlaps", value: summary.room_overlaps, icon: DoorOpen, tone: "text-amber-700" },
    { label: "teacher overlaps", value: summary.teacher_overlaps, icon: Users, tone: "text-red-700" },
    { label: "student overlaps", value: summary.student_overlaps, icon: GraduationCap, tone: "text-blue-700" },
  ] as const;

  return (
    <section className="mb-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-label="Conflict summary">
      {cards.map(({ label, value, icon: Icon, tone }) => (
        <div key={label} className="rounded-sm border border-wi-line bg-white p-3">
          <div className="flex items-center justify-between gap-3">
            <div>
              <p className="text-2xl font-semibold tabular-nums text-[var(--color-wi-text)]">{value}</p>
              <p className="mt-0.5 text-xs font-medium text-[var(--color-wi-text-light)]">{value} {label}</p>
            </div>
            <Icon className={`h-5 w-5 ${tone}`} aria-hidden="true" />
          </div>
        </div>
      ))}
    </section>
  );
}
