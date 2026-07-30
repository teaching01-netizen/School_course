import { AlertTriangle, ChevronRight, Clock3 } from "lucide-react";
import Button from "../ui/Button";
import { formatBangkokDateTime, issueMessage, urgencyFor } from "../../features/scheduleImpact/format";
import type { ScheduleImpactIssue } from "../../features/scheduleImpact/types";

type Density = "comfortable" | "compact";

interface ImpactWorkQueueProps {
  items: ScheduleImpactIssue[];
  density: Density;
  selectedID: string | null;
  onOpen: (issue: ScheduleImpactIssue) => void;
}

function SeverityTag({ issue }: { issue: ScheduleImpactIssue }) {
  const critical = issue.severity === "critical";
  return (
    <span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-semibold ${critical ? "bg-red-50 text-red-700" : "bg-amber-50 text-amber-800"}`}>
      <AlertTriangle className="h-3 w-3" aria-hidden="true" />
      {critical ? "Critical" : "Warning"}
    </span>
  );
}

function CurrentPlan({ issue }: { issue: ScheduleImpactIssue }) {
  return (
    <div className="min-w-0">
      <p className="text-[11px] font-semibold uppercase tracking-wide text-gray-500">Current sit-in</p>
      <p className="mt-0.5 text-sm text-gray-700">{formatBangkokDateTime(issue.start_at, issue.end_at)}</p>
    </div>
  );
}

export default function ImpactWorkQueue({ items, density, selectedID, onOpen }: ImpactWorkQueueProps) {
  if (items.length === 0) {
    return (
      <section className="rounded-sm border border-gray-200 bg-white px-6 py-14 text-center">
        <Clock3 className="mx-auto h-9 w-9 text-gray-400" aria-hidden="true" />
        <h2 className="mt-3 text-base font-semibold text-gray-900">No student arrangements need attention</h2>
        <p className="mx-auto mt-1 max-w-md text-sm text-gray-500">Schedule changes are being monitored automatically.</p>
      </section>
    );
  }

  if (density === "compact") {
    return (
      <section className="overflow-hidden rounded-sm border border-gray-200 bg-white">
        <div className="overflow-x-auto">
          <table className="min-w-[760px] text-sm">
            <thead className="bg-gray-50 text-xs text-gray-600">
              <tr><th>Student</th><th>Problem</th><th>Current sit-in</th><th>Urgency</th><th className="text-right">Action</th></tr>
            </thead>
            <tbody>
              {items.map((issue) => (
                <tr key={issue.id} className={selectedID === issue.id ? "bg-blue-50/70" : ""}>
                  <td><button type="button" onClick={() => onOpen(issue)} className="text-left font-medium text-gray-900 hover:text-[var(--color-wi-primary)]">{issue.student_name ?? issue.wcode}<span className="block text-xs font-normal text-gray-500">{issue.course_code}</span></button></td>
                  <td><span className="mr-2 align-middle"><SeverityTag issue={issue} /></span>{issueMessage(issue)}</td>
                  <td className="whitespace-nowrap text-gray-700">{formatBangkokDateTime(issue.start_at, issue.end_at)}</td>
                  <td className="whitespace-nowrap text-gray-700">{urgencyFor(issue)}</td>
                  <td className="text-right"><Button variant="ghost" size="sm" onClick={() => onOpen(issue)}>Review</Button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    );
  }

  return (
    <section className="overflow-hidden rounded-sm border border-gray-200 bg-white">
      <div className="divide-y divide-gray-200">
        {items.map((issue) => {
          const candidate = issue.suggested_resolutions[0];
          const selected = selectedID === issue.id;
          return (
            <article key={issue.id} className={`px-4 py-4 transition-colors sm:px-5 ${selected ? "bg-blue-50/60" : "hover:bg-gray-50"}`}>
              <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
                <button type="button" onClick={() => onOpen(issue)} className="min-w-0 flex-1 text-left focus-visible:rounded-sm">
                  <div className="flex flex-wrap items-center gap-2">
                    <SeverityTag issue={issue} />
                    <span className="font-semibold text-gray-900">{issue.student_name ?? issue.wcode}</span>
                    <span className="text-sm text-gray-500">{issue.course_code}{issue.course_name ? ` · ${issue.course_name}` : ""}</span>
                  </div>
                  <p className="mt-2 text-sm font-medium text-gray-800">{issueMessage(issue)}</p>
                  <p className="mt-1 text-sm text-gray-500">{issue.status === "needs_review" ? `Marked for review${issue.assigned_to ? ` · ${issue.assigned_to}` : ""}` : "Open the issue to compare options and confirm the safest action."}</p>
                </button>
                <p className={`shrink-0 text-sm font-medium ${issue.severity === "critical" ? "text-red-700" : "text-amber-800"}`}>{urgencyFor(issue)}</p>
              </div>
              <div className="mt-4 grid gap-3 border-t border-gray-100 pt-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] md:items-end">
                <CurrentPlan issue={issue} />
                <div className="min-w-0">
                  <p className="text-[11px] font-semibold uppercase tracking-wide text-gray-500">Recommended</p>
                  <p className="mt-0.5 text-sm text-gray-700">{candidate ? `${formatBangkokDateTime(candidate.start_at, candidate.end_at)} · Candidate available` : "Review the student’s options"}</p>
                </div>
                <div className="flex flex-wrap gap-2 md:justify-end">
                  <Button size="sm" onClick={() => onOpen(issue)}>{candidate ? "Reassign" : "Review"}</Button>
                  <Button variant="secondary" size="sm" onClick={() => onOpen(issue)}>Choose another</Button>
                  <Button variant="ghost" size="sm" onClick={() => onOpen(issue)}>More <ChevronRight className="ml-0.5 h-3.5 w-3.5" aria-hidden="true" /></Button>
                </div>
              </div>
            </article>
          );
        })}
      </div>
    </section>
  );
}
