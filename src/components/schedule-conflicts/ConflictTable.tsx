import { Fragment, useState } from "react";
import { ChevronRight } from "lucide-react";
import EmptyState from "@/components/ui/EmptyState";
import type { ConflictOverviewItem } from "@/features/scheduling/types/conflictOverview";
import { ConflictDetailPanel, formatDateTimeRange } from "./ConflictDetailPanel";
import { ConflictTypeBadge } from "./ConflictTypeBadge";

export function ConflictTable({ items, refreshing }: Readonly<{ items: readonly ConflictOverviewItem[]; refreshing: boolean }>) {
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set());
  if (items.length === 0) return <EmptyState message="No schedule conflicts match these filters" />;

  function toggle(id: string) {
    setExpanded((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  return (
    <div className="relative overflow-x-auto data-table-wrapper" aria-busy={refreshing}>
      {refreshing ? <div className="absolute inset-x-0 top-0 z-10 h-0.5 overflow-hidden" aria-hidden="true"><div className="h-full w-1/4 animate-wi-progress bg-[var(--color-wi-primary)] motion-reduce:animate-none" /></div> : null}
      <table className="w-full min-w-[58rem] text-[13px]">
        <caption className="sr-only">Schedule conflicts</caption>
        <thead><tr className="border-b border-wi-line">
          <th scope="col" className="w-10 px-2" />
          <th scope="col" className="px-2 py-2 text-left font-semibold text-[var(--color-wi-text-light)]">Type</th>
          <th scope="col" className="px-2 py-2 text-left font-semibold text-[var(--color-wi-text-light)]">Primary subject</th>
          <th scope="col" className="px-2 py-2 text-left font-semibold text-[var(--color-wi-text-light)]">Conflicts with</th>
          <th scope="col" className="px-2 py-2 text-left font-semibold text-[var(--color-wi-text-light)]">Date and time</th>
          <th scope="col" className="px-2 py-2 text-left font-semibold text-[var(--color-wi-text-light)]">Shared resource</th>
        </tr></thead>
        <tbody>{items.map((item) => {
          const isExpanded = expanded.has(item.id);
          return <Fragment key={item.id}>
            <tr className="border-b border-wi-line hover:bg-[var(--color-wi-row-alt)]">
              <td className="px-2 py-3"><button type="button" onClick={() => toggle(item.id)} className="flex h-7 w-7 items-center justify-center rounded-sm text-[var(--color-wi-text-light)] hover:bg-white hover:text-[var(--color-wi-text)]" aria-expanded={isExpanded} aria-controls={`conflict-details-${item.id}`} aria-label={isExpanded ? "Collapse conflict details" : "Expand conflict details"}><ChevronRight className={`h-4 w-4 transition-transform motion-reduce:transition-none ${isExpanded ? "rotate-90" : ""}`} /></button></td>
              <td className="px-2 py-3"><ConflictTypeBadge type={item.conflict_type} /></td>
              <td className="px-2 py-3"><p className="font-medium text-[var(--color-wi-text)]">{item.primary_session.subject_name}</p><p className="mt-0.5 text-xs text-[var(--color-wi-text-light)]">{item.primary_session.course_code}</p></td>
              <td className="px-2 py-3">{item.conflicting_sessions.map((session) => <div key={session.session_id}><span className="font-medium">{session.subject_name}</span><span className="ml-1 text-xs text-[var(--color-wi-text-light)]">{session.course_code}</span></div>)}</td>
              <td className="px-2 py-3 whitespace-nowrap">{formatDateTimeRange(item.primary_session.start_at, item.primary_session.end_at)}</td>
              <td className="px-2 py-3"><span className="font-medium">{item.shared_resource.name}</span>{item.affected_students.length > 1 ? <span className="ml-1 text-xs text-[var(--color-wi-text-light)]">+{item.affected_students.length - 1}</span> : null}</td>
            </tr>
            {isExpanded ? <tr className="border-b border-wi-line"><td colSpan={6} className="p-0"><ConflictDetailPanel conflict={item} /></td></tr> : null}
          </Fragment>;
        })}</tbody>
      </table>
    </div>
  );
}
