import { AlertTriangle, ArrowUpRight } from "lucide-react";
import { Link } from "react-router-dom";
import { Popover } from "@/components/ui/Popover";
import { formatUTCToZone } from "@/utils/timezone";
import type { StudentConflict } from "@/types";

type Props = {
  conflicts: StudentConflict[];
  zone: string;
};

export function StudentConflictPopover({ conflicts, zone }: Props) {
  if (conflicts.length === 0) {
    return (
      <span className="inline-flex items-center rounded-sm border border-[var(--color-wi-green)]/25 bg-[var(--color-wi-green)]/8 px-1.5 py-0.5 text-[11px] font-medium text-[var(--color-wi-green)]">
        Clear
      </span>
    );
  }

  return (
    <Popover
      trigger={
        <button
          type="button"
          className="inline-flex items-center gap-1 rounded-sm border border-[var(--color-wi-red)]/25 bg-[var(--color-wi-danger-bg)] px-1.5 py-0.5 text-[11px] font-semibold text-[var(--color-wi-red)] transition-colors duration-150 hover:bg-[var(--color-wi-red)]/12 focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-red)] motion-reduce:transition-none"
          aria-label={`Show ${conflicts.length} student conflict${conflicts.length === 1 ? "" : "s"}`}
        >
          <AlertTriangle className="h-3 w-3" aria-hidden="true" />
          Conflict{conflicts.length > 1 ? ` (${conflicts.length})` : ""}
        </button>
      }
      role="dialog"
      ariaLabel="Student conflict details"
      contentClassName="w-[min(25rem,calc(100vw-2rem))] p-3"
    >
      <div className="space-y-3">
        <div>
          <p className="text-sm font-semibold text-[var(--color-wi-text)]">Student schedule conflict</p>
          <p className="mt-0.5 text-xs text-[var(--color-wi-text-light)]">This student is scheduled in another course at the same time.</p>
        </div>
        <ul className="space-y-2">
          {conflicts.map((conflict) => {
            const currentStart = formatUTCToZone(conflict.current_start_at, zone, "EEE d MMM, HH:mm") ?? conflict.current_start_at;
            const currentEnd = formatUTCToZone(conflict.current_end_at, zone, "HH:mm") ?? conflict.current_end_at;
            const conflictingStart = formatUTCToZone(conflict.conflicting_start_at, zone, "EEE d MMM, HH:mm") ?? conflict.conflicting_start_at;
            const conflictingEnd = formatUTCToZone(conflict.conflicting_end_at, zone, "HH:mm") ?? conflict.conflicting_end_at;
            return (
              <li key={`${conflict.current_session_id}-${conflict.conflicting_session_id}`} className="rounded-sm border border-wi-line-soft bg-[var(--color-wi-callout)] p-2.5">
                <p className="text-xs font-semibold text-[var(--color-wi-red)]">Overlapping sessions</p>
                <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">This course: {currentStart}–{currentEnd}</p>
                <p className="text-xs text-[var(--color-wi-text)]">Other course: {conflictingStart}–{conflictingEnd}</p>
                <Link
                  to={`/courses/${conflict.conflicting_course_id}`}
                  className="mt-1 inline-flex items-center gap-1 text-xs font-semibold text-[var(--color-wi-primary)] hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                >
                  {conflict.conflicting_course_code} · {conflict.conflicting_course_name}
                  <ArrowUpRight className="h-3 w-3" aria-hidden="true" />
                </Link>
              </li>
            );
          })}
        </ul>
      </div>
    </Popover>
  );
}
