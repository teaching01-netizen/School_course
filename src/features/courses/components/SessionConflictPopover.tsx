import { AlertTriangle, ArrowUpRight } from "lucide-react";
import { Link } from "react-router-dom";
import { Popover } from "@/components/ui/Popover";
import { formatUTCToZone } from "@/utils/timezone";
import type { SessionConflict } from "@/types";

type Props = {
  conflicts: SessionConflict[];
  currentCourseId: string;
  zone: string;
};

function conflictLabel(kind: SessionConflict["kind"]): string {
  return kind === "room_overlap" ? "Room overlap" : "Teacher overlap";
}

export function SessionConflictPopover({ conflicts, currentCourseId, zone }: Props) {
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
          aria-label={`Show ${conflicts.length} session conflict${conflicts.length === 1 ? "" : "s"}`}
        >
          <AlertTriangle className="h-3 w-3" aria-hidden="true" />
          Conflict{conflicts.length > 1 ? ` (${conflicts.length})` : ""}
        </button>
      }
      role="dialog"
      ariaLabel="Session conflict details"
      contentClassName="w-[min(24rem,calc(100vw-2rem))] p-3"
    >
      <div className="space-y-3">
        <div>
          <p className="text-sm font-semibold text-[var(--color-wi-text)]">Session conflict</p>
          <p className="mt-0.5 text-xs text-[var(--color-wi-text-light)]">This session overlaps another scheduled session.</p>
        </div>
        <ul className="space-y-2">
          {conflicts.map((conflict) => {
            const start = formatUTCToZone(conflict.conflicting_start_at, zone, "EEE d MMM, HH:mm") ?? conflict.conflicting_start_at;
            const end = formatUTCToZone(conflict.conflicting_end_at, zone, "HH:mm") ?? conflict.conflicting_end_at;
            const isOtherCourse = conflict.conflicting_course_id !== currentCourseId;
            return (
              <li key={`${conflict.kind}-${conflict.conflicting_session_id}`} className="rounded-sm border border-wi-line-soft bg-[var(--color-wi-callout)] p-2.5">
                <p className="text-xs font-semibold text-[var(--color-wi-red)]">{conflictLabel(conflict.kind)}</p>
                <p className="mt-1 text-xs text-[var(--color-wi-text)]">{start}–{end}</p>
                {isOtherCourse ? (
                  <Link
                    to={`/courses/${conflict.conflicting_course_id}`}
                    className="mt-1 inline-flex items-center gap-1 text-xs font-semibold text-[var(--color-wi-primary)] hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                  >
                    {conflict.conflicting_course_code} · {conflict.conflicting_course_name}
                    <ArrowUpRight className="h-3 w-3" aria-hidden="true" />
                  </Link>
                ) : (
                  <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">Another session in this course</p>
                )}
              </li>
            );
          })}
        </ul>
      </div>
    </Popover>
  );
}
