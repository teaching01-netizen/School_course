import clsx from "clsx";
import { CheckCircle, XCircle, Clock } from "lucide-react";
import { formatDate } from "@/utils/date";

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

export type SubjectSubmission = {
  subjectCode: string;
  subjectName: string;
  sessionCount: number;
  sitInMethod?: string;
  sitInCourseCode?: string;
  submitStatus?: "success" | "error" | "pending";
  submitError?: string;
  onRetry?: () => void;
};

export type ConfirmationSummaryProps = {
  studentName: string;
  wcode: string;
  dateFrom: string;
  dateTo: string;
  reasonCategory?: string;
  reasonCategoryLabel?: string;
  reason?: string;
  subjects: SubjectSubmission[];
  confirmationText?: string;
  mode: "review" | "result";
  onEdit?: () => void;
};

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function daysBetween(from: string, to: string): number {
  const a = new Date(from + "T00:00:00");
  const b = new Date(to + "T00:00:00");
  return Math.round((b.getTime() - a.getTime()) / (1000 * 60 * 60 * 24)) + 1;
}

/* ------------------------------------------------------------------ */
/*  Status indicator                                                   */
/* ------------------------------------------------------------------ */

function StatusIndicator({ status }: { status: "success" | "error" | "pending" }) {
  if (status === "success") {
    return <CheckCircle className="h-4 w-4 text-green-600" aria-hidden="true" />;
  }
  if (status === "error") {
    return <XCircle className="h-4 w-4 text-red-600" aria-hidden="true" />;
  }
  return <Clock className="h-4 w-4 text-gray-600" aria-hidden="true" />;
}

/* ------------------------------------------------------------------ */
/*  Review mode                                                        */
/* ------------------------------------------------------------------ */

function ReviewMode({
  studentName,
  wcode,
  dateFrom,
  dateTo,
  reasonCategoryLabel,
  reason,
  subjects,
  onEdit,
}: Omit<ConfirmationSummaryProps, "mode">) {
  const dayCount = daysBetween(dateFrom, dateTo);

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold text-gray-900">Review your absence</h2>

      {/* student info */}
      <div className="text-sm text-gray-700">
        <p>
          <span className="font-medium">Student:</span> {studentName} ({wcode})
        </p>
        <p>
          <span className="font-medium">Dates:</span>{" "}
          {formatDate(dateFrom)} &ndash; {formatDate(dateTo)} ({dayCount} day{dayCount !== 1 ? "s" : ""})
        </p>
        {reasonCategoryLabel && (
          <p>
            <span className="font-medium">Reason:</span> {reasonCategoryLabel}
          </p>
        )}
        {reason && (
          <p className="mt-1 text-gray-600">{reason}</p>
        )}
      </div>

      {/* subjects */}
      {subjects.length > 0 && (
        <div>
          <h3 className="mb-2 text-sm font-medium text-gray-900">Courses</h3>
          <ul className="space-y-1">
            {subjects.map((s) => (
              <li
                key={s.subjectCode}
                className="max-sm:flex-col max-sm:items-start flex flex-wrap items-center gap-2 rounded-sm px-2 py-1 text-sm text-gray-700 hover:bg-gray-50"
              >
                <span className="font-medium">{s.subjectCode}</span>
                <span>&mdash;</span>
                <span>{s.subjectName}</span>
                <span className="text-gray-600">
                  : {s.sessionCount} session{s.sessionCount !== 1 ? "s" : ""}
                </span>
                {s.sitInMethod && (
                  <span className="sm:ml-auto text-gray-600">
                    &rarr; Make-up: {s.sitInMethod}
                  </span>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* edit button */}
      {onEdit && (
        <button
          type="button"
          className="min-h-[44px] rounded-sm border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50"
          onClick={onEdit}
        >
          Edit
        </button>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Result mode                                                        */
/* ------------------------------------------------------------------ */

function ResultMode({
  confirmationText,
  subjects,
}: {
  confirmationText?: string;
  subjects: SubjectSubmission[];
}) {
  return (
    <div className="space-y-4">
      <h2 className="flex items-center gap-2 text-lg font-semibold text-gray-900">
        <CheckCircle className="h-5 w-5 text-green-600" aria-hidden="true" />
        Absence submitted
      </h2>

      {confirmationText && (
        <p className="text-sm text-gray-650">{confirmationText}</p>
      )}

      {/* subjects */}
      {subjects.length > 0 && (
        <ul className="space-y-2">
          {subjects.map((s) => (
            <li
              key={s.subjectCode}
              className={clsx(
                "max-sm:flex-col max-sm:items-start flex flex-wrap items-center gap-2 rounded-sm px-2 py-1 text-sm",
                s.submitStatus === "success" && "text-green-800",
                s.submitStatus === "error" && "text-red-800",
                s.submitStatus === "pending" && "text-gray-700",
              )}
            >
              <StatusIndicator status={s.submitStatus ?? "pending"} />
              <span className="font-medium">{s.subjectCode}</span>
              <span>&mdash;</span>
              <span>{s.subjectName}</span>
              <span className="text-gray-600">
                {s.submitStatus === "success" && "— Submitted"}
                {s.submitStatus === "error" && (
                  <>
                    — Failed
                    {s.submitError && ` (${s.submitError})`}
                  </>
                )}
                {s.submitStatus === "pending" && "— Pending"}
              </span>
              {s.submitStatus === "error" && s.onRetry && (
                <button
                  type="button"
                  className="sm:ml-auto min-h-[44px] rounded-sm border border-red-300 bg-white px-3 py-2 text-xs font-medium text-red-700 transition-colors hover:bg-red-100"
                  onClick={s.onRetry}
                >
                  Retry
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Main component                                                     */
/* ------------------------------------------------------------------ */

export default function ConfirmationSummary(props: ConfirmationSummaryProps) {
  const { mode, ...rest } = props;

  if (mode === "result") {
    return (
      <div className="rounded-sm border border-gray-200 bg-white p-5 shadow-sm">
        <ResultMode
          confirmationText={rest.confirmationText}
          subjects={rest.subjects}
        />
      </div>
    );
  }

  return (
    <div className="rounded-sm border border-gray-200 bg-white p-5 shadow-sm">
      <ReviewMode {...rest} />
    </div>
  );
}
