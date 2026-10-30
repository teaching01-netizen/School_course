import clsx from "clsx";
import { ChevronLeft, ChevronRight, LoaderCircle } from "lucide-react";

type AbsenceActionBarProps = {
  currentStep: number;
  canProceed: boolean;
  loading?: boolean;
  loadingLabel?: string;
  loadingHint?: string;
  onBack: () => void;
  onPrimary: () => void;
  primaryLabel: string;
  hint?: string;
};

export default function AbsenceActionBar({ currentStep, canProceed, loading = false, loadingLabel = "Submitting...", loadingHint = "Submitting your absence… Keep this page open until it’s confirmed.", onBack, onPrimary, primaryLabel, hint }: AbsenceActionBarProps) {
  return (
    <div className="absence-action-bar">
      {loading ? <p id="absence-action-hint" role="status" className="absence-action-bar__hint">{loadingHint}</p> : hint ? <p id="absence-action-hint" className="absence-action-bar__hint">{hint}</p> : null}
      <div className="absence-action-bar__inner">
        {currentStep > 0 ? (
          <button
            type="button"
            aria-label="Back"
            onClick={onBack}
            disabled={loading}
            className="absence-action-bar__back inline-flex min-h-12 items-center gap-1 rounded-lg px-3 text-sm font-semibold text-[var(--color-wi-text-light)] transition-[color,background-color,transform] duration-150 active:scale-[0.97] motion-reduce:transition-none motion-reduce:active:scale-100 hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] disabled:opacity-50"
          >
            <ChevronLeft className="h-5 w-5" aria-hidden="true" />
            <span>Back</span>
          </button>
        ) : <span aria-hidden="true" className="absence-action-bar__spacer min-w-20" />}
        <button
          type="button"
          onClick={onPrimary}
          disabled={!canProceed || loading}
          aria-busy={loading}
          aria-describedby={hint || loading ? "absence-action-hint" : undefined}
          className={clsx(
            "absence-action-bar__primary min-h-12 min-w-[9.5rem] rounded-xl px-5 text-base font-semibold transition-[color,background-color,transform] duration-200 enabled:active:scale-[0.98] motion-reduce:transition-none motion-reduce:enabled:active:scale-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2",
            canProceed && !loading
              ? "bg-[var(--color-wi-primary)] text-white hover:bg-[var(--color-wi-primary-dark)]"
              : "cursor-not-allowed bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]",
          )}
        >
          {loading ? (
            <span className="inline-flex items-center justify-center gap-2">
              <LoaderCircle className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
              {loadingLabel}
            </span>
          ) : <><span>{primaryLabel}</span><ChevronRight className="h-4 w-4 shrink-0" aria-hidden="true" /></>}
        </button>
      </div>
    </div>
  );
}
