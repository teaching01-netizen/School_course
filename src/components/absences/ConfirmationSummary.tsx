import { formatDate } from "@/utils/date";

export type ConfirmationSummaryProps = {
  studentName: string;
  wcode: string;
  dateFrom: string;
  dateTo: string;
  subjects: Array<{
    code: string;
    name: string;
    dates: string[];
    sitInLabel: string;
  }>;
  reason: string;
};

function daysBetween(from: string, to: string): number {
  const a = new Date(from + "T00:00:00");
  const b = new Date(to + "T00:00:00");
  return Math.round((b.getTime() - a.getTime()) / (1000 * 60 * 60 * 24)) + 1;
}

export default function ConfirmationSummary({
  studentName,
  wcode,
  dateFrom,
  dateTo,
  subjects,
  reason,
}: ConfirmationSummaryProps) {
  const dayCount = daysBetween(dateFrom, dateTo);

  return (
    <div className="space-y-4">
      <div className="text-sm text-[var(--color-wi-text-light)]">
        <p><span className="font-medium text-[var(--color-wi-text)]">Student:</span> {studentName} ({wcode})</p>
        <p><span className="font-medium text-[var(--color-wi-text)]">Dates:</span> {formatDate(dateFrom)} &ndash; {formatDate(dateTo)} ({dayCount} day{dayCount !== 1 ? "s" : ""})</p>
      </div>

      {subjects.length > 0 && (
        <div className="space-y-2">
          {subjects.map((s) => (
            <div key={s.code} className="rounded-lg border border-[var(--color-wi-line)] bg-[var(--color-wi-row-alt)] p-4">
              <p className="text-sm font-semibold text-[var(--color-wi-text)]">{s.code} — {s.name}</p>
              <p className="text-xs text-[var(--color-wi-text-light)]">{s.dates.join(" · ")}</p>
              {s.sitInLabel && <p className="text-xs text-[var(--color-wi-text-light)]">Make-up: {s.sitInLabel}</p>}
            </div>
          ))}
        </div>
      )}

      {reason && (
        <p className="text-sm text-[var(--color-wi-text-light)]"><span className="font-medium text-[var(--color-wi-text)]">Reason:</span> {reason}</p>
      )}
    </div>
  );
}
