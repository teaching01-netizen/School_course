import { formatDate, formatTime } from "@/utils/date";
import { instituteDateKey } from "@/features/absences/domain/sessionGrouping";
import ScreenTitle from "./ScreenTitle";

type ResumeScreenProps = {
  /** Draft `updatedAt` timestamp (ms) when a full draft exists. */
  startedAt?: number;
  /** A short privacy-safe summary of what is saved, e.g. "2 classes · Appointment". */
  summary?: string;
  onContinue: () => void;
  onStartOver: () => void;
};

function startedCopy(startedAt?: number): string {
  if (!startedAt || !Number.isFinite(startedAt)) return "You started an absence report earlier.";
  const iso = new Date(startedAt).toISOString();
  const time = formatTime(iso);
  const sameDay = instituteDateKey(iso) === instituteDateKey(new Date().toISOString());
  if (sameDay) return `You started this report today at ${time}.`;
  return `You started this report on ${formatDate(iso)} at ${time}.`;
}

export default function ResumeScreen({
  startedAt,
  summary,
  onContinue,
  onStartOver,
}: ResumeScreenProps) {
  return (
    <div className="mx-auto w-full max-w-2xl">
      <ScreenTitle>
        Continue your absence report?

      </ScreenTitle>
      <p className="mt-2 text-[17px] leading-relaxed text-[var(--color-wi-text-light)]">
        {startedCopy(startedAt)}
      </p>

      <div className="mt-6 rounded-2xl border border-[var(--color-wi-border)] bg-white px-5 py-4">
        <p className="text-[13px] font-semibold uppercase tracking-[0.08em] text-[var(--color-wi-text-light)]">
          Saved report
        </p>
        <p className="mt-1 text-[15px] leading-relaxed text-[var(--color-wi-text)]">
          {summary ?? "Your selected classes and details are still saved."}
        </p>
      </div>

      <button
        type="button"
        onClick={onContinue}
        className="wi-press mt-10 flex h-[52px] w-full items-center justify-center rounded-xl bg-[var(--color-wi-primary)] px-5 text-[17px] font-semibold text-white hover:bg-[var(--color-wi-primary-dark)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2"
      >
        Continue
      </button>
      <button
        type="button"
        onClick={onStartOver}
        className="wi-press mt-3 flex h-[52px] w-full items-center justify-center rounded-xl px-5 text-[17px] font-semibold text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
      >
        Discard saved report and start over
      </button>
    </div>
  );
}