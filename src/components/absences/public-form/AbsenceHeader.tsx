import { ChevronLeft } from "lucide-react";

type AbsenceHeaderProps = {
  onBack?: () => void;
  progress: number;
  progressLabel: string;
};

export default function AbsenceHeader({ onBack, progress, progressLabel }: AbsenceHeaderProps) {
  return (
    <div className="absence-app-header">
      <div className="absence-app-header__identity">
        <p className="text-[12px] font-semibold uppercase tracking-[0.16em] text-[var(--color-wi-primary)]">Warwick Institute</p>
        <p className="mt-0.5 text-[15px] font-semibold text-[var(--color-wi-text)]">Report absence</p>
      </div>
      {onBack ? (
        <div className="absence-app-header__nav">
          <button
            type="button"
            onClick={onBack}
            className="wi-press inline-flex min-h-11 items-center gap-0.5 rounded-lg px-1 text-[15px] font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
          >
            <ChevronLeft className="h-5 w-5" aria-hidden="true" />
            Back
          </button>
        </div>
      ) : null}
      <div
        className="absence-header-progress"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(progress * 100)}
        aria-label={progressLabel}
      >
        <div className="absence-header-progress__track" aria-hidden="true">
          <div className="absence-header-progress__fill" style={{ width: `${progress * 100}%` }} />
        </div>
      </div>
    </div>
  );
}
