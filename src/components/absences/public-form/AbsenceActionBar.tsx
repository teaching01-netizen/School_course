import clsx from "clsx";
import { ChevronLeft, LoaderCircle } from "lucide-react";

type AbsenceActionBarProps = {
  currentStep: number;
  canProceed: boolean;
  loading?: boolean;
  onBack: () => void;
  onPrimary: () => void;
  primaryLabel: string;
};

export default function AbsenceActionBar({ currentStep, canProceed, loading = false, onBack, onPrimary, primaryLabel }: AbsenceActionBarProps) {
  return (
    <div className="absence-action-bar">
      <div className="absence-action-bar__inner">
        {currentStep > 0 ? (
          <button
            type="button"
            onClick={onBack}
            className="inline-flex min-h-12 items-center gap-1 rounded-lg px-3 text-sm font-semibold text-[var(--color-wi-text-light)] transition-colors motion-reduce:transition-none hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
          >
            <ChevronLeft className="h-5 w-5" aria-hidden="true" />
            <span>Back</span>
          </button>
        ) : <span aria-hidden="true" className="min-w-20" />}
        <button
          type="button"
          onClick={onPrimary}
          disabled={!canProceed || loading}
          className={clsx(
            "min-h-12 min-w-[9.5rem] rounded-xl px-5 text-base font-semibold transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2",
            canProceed && !loading
              ? "bg-[var(--color-wi-primary)] text-white hover:bg-[var(--color-wi-primary-dark)]"
              : "cursor-not-allowed bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]",
          )}
        >
          {loading ? (
            <span className="inline-flex items-center justify-center gap-2">
              <LoaderCircle className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
              Submitting...
            </span>
          ) : primaryLabel}
        </button>
      </div>
    </div>
  );
}
