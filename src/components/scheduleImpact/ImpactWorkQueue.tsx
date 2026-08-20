import { AlertTriangle, Clock3, Info } from "lucide-react";
import Button from "../ui/Button";
import { issueMessage, subjectNameFor, urgencyFor } from "../../features/scheduleImpact/format";
import type { ScheduleImpactIssue } from "../../features/scheduleImpact/types";
import WorkQueueComparison from "./WorkQueueComparison";

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

function LegacyBadge({ quality }: { quality: "exact" | "reconstructed" | "unavailable" }) {
  if (quality === "exact") return null;
  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-[var(--color-wi-row-alt)] px-2 py-0.5 text-[10px] text-[var(--color-wi-text-light)]">
      <Info className="h-3 w-3" aria-hidden="true" />
      {quality === "reconstructed" ? "Reconstructed" : "No snapshot"}
    </span>
  );
}

export default function ImpactWorkQueue({ items, density, selectedID, onOpen }: ImpactWorkQueueProps) {
  if (items.length === 0) {
    return (
      <section className="rounded-sm border border-[var(--color-wi-line)] bg-white px-6 py-14 text-center">
        <Clock3 className="mx-auto h-9 w-9 text-[var(--color-wi-text-light)]" aria-hidden="true" />
        <h2 className="mt-3 text-base font-semibold text-[var(--color-wi-text)]">No student arrangements need attention</h2>
        <p className="mx-auto mt-1 max-w-md text-sm text-[var(--color-wi-text-light)]">Schedule changes are being monitored automatically.</p>
      </section>
    );
  }

  if (density === "compact") {
    return (
      <section className="overflow-hidden rounded-sm border border-[var(--color-wi-line)] bg-white">
        <div className="overflow-x-auto">
          <table className="min-w-[760px] text-sm">
            <thead className="bg-[var(--color-wi-row-alt)] text-xs text-[var(--color-wi-text-light)]">
              <tr><th>Student</th><th>Problem</th><th>Originally / Now</th><th>Urgency</th><th className="text-right">Action</th></tr>
            </thead>
            <tbody>
              {items.map((issue) => {
                const originalQuality = issue.assignment_context.original_session.quality;
                return (
                  <tr key={issue.id} className={selectedID === issue.id ? "bg-blue-50/70" : ""}>
                    <td>
                      <button type="button" onClick={() => onOpen(issue)} className="text-left font-medium text-[var(--color-wi-text)] hover:text-[var(--color-wi-primary)]">
                        {issue.student_name ?? issue.wcode}
                        <span className="block text-xs font-normal text-[var(--color-wi-text-light)]">
                          {subjectNameFor(issue)}
                        </span>
                      </button>
                    </td>
                    <td>
                      <span className="mr-2 align-middle"><SeverityTag issue={issue} /></span>
                      {originalQuality !== "exact" ? <span className="mr-1 align-middle"><LegacyBadge quality={originalQuality} /></span> : null}
                      {issueMessage(issue)}
                    </td>
                    <td className="whitespace-nowrap"><WorkQueueComparison issue={issue} /></td>
                    <td className="whitespace-nowrap text-[var(--color-wi-text-light)]">{urgencyFor(issue)}</td>
                    <td className="text-right"><Button variant="ghost" size="sm" onClick={() => onOpen(issue)}>Review</Button></td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>
    );
  }

  return (
    <section className="overflow-hidden rounded-sm border border-[var(--color-wi-line)] bg-white">
      <div className="divide-y divide-wi-line">
        {items.map((issue) => {
          const selected = selectedID === issue.id;
          const originalQuality = issue.assignment_context.original_session.quality;
          const current = issue.assignment_context.current_session;
          const isDeleted = !current || current.status === "deleted";
          return (
            <article key={issue.id} className={`px-4 py-4 transition-colors sm:px-5 ${selected ? "bg-blue-50/60" : "hover:bg-[var(--color-wi-row-alt)]"}`}>
              <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
                <button type="button" onClick={() => onOpen(issue)} className="min-w-0 flex-1 text-left focus-visible:rounded-sm">
                  <div className="flex flex-wrap items-center gap-2">
                    <SeverityTag issue={issue} />
                    {originalQuality !== "exact" ? <LegacyBadge quality={originalQuality} /> : null}
                    <span className="font-semibold text-[var(--color-wi-text)]">{issue.student_name ?? issue.wcode}</span>
                    <span className="text-sm text-[var(--color-wi-text-light)]">{subjectNameFor(issue)}</span>
                  </div>
                  <p className="mt-2 text-sm font-medium text-[var(--color-wi-text)]">{issueMessage(issue)}</p>
                  <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">
                    {isDeleted ? "The assigned session has been deleted." : issue.status === "needs_review" ? "Marked for review" : "Open the issue to compare options and confirm the safest action."}
                  </p>
                </button>
                <p className={`shrink-0 text-sm font-medium ${issue.severity === "critical" ? "text-red-700" : "text-amber-800"}`}>{urgencyFor(issue)}</p>
              </div>

              {/* Compact original/current comparison */}
              <div className="mt-4 rounded-sm border border-[var(--color-wi-line)] bg-[var(--color-wi-row-alt)]/50 p-3">
                <WorkQueueComparison issue={issue} />
              </div>

              <div className="mt-3 flex flex-wrap gap-2 sm:justify-end">
                <Button size="sm" onClick={() => onOpen(issue)}>Review</Button>
              </div>
            </article>
          );
        })}
      </div>
    </section>
  );
}
