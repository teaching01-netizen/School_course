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
      <div className="text-sm text-gray-600">
        <p><span className="font-medium text-gray-900">Student:</span> {studentName} ({wcode})</p>
        <p><span className="font-medium text-gray-900">Dates:</span> {formatDate(dateFrom)} &ndash; {formatDate(dateTo)} ({dayCount} day{dayCount !== 1 ? "s" : ""})</p>
      </div>

      {subjects.length > 0 && (
        <div className="space-y-2">
          {subjects.map((s) => (
            <div key={s.code} className="rounded-lg border border-gray-100 bg-gray-50 p-4">
              <p className="text-sm font-semibold text-gray-900">{s.code} — {s.name}</p>
              <p className="text-xs text-gray-600">{s.dates.join(" · ")}</p>
              {s.sitInLabel && <p className="text-xs text-gray-500">Make-up: {s.sitInLabel}</p>}
            </div>
          ))}
        </div>
      )}

      {reason && (
        <p className="text-sm text-gray-600"><span className="font-medium text-gray-900">Reason:</span> {reason}</p>
      )}
    </div>
  );
}
