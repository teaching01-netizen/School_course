import { ChevronLeft } from "lucide-react";
import clsx from "clsx";

type StickyFooterProps = {
  currentStep: number;
  totalSteps: number;
  canProceed: boolean;
  loading?: boolean;
  onBack: () => void;
  onPrimary: () => void;
  primaryLabel: string;
};

export default function StickyFooter({
  currentStep,
  totalSteps,
  canProceed,
  loading = false,
  onBack,
  onPrimary,
  primaryLabel,
}: StickyFooterProps) {
  return (
    <div className="fixed bottom-0 left-0 right-0 z-40 border-t border-[var(--color-wi-border)] bg-white/95 backdrop-blur-sm">
      <div className="mx-auto flex min-h-16 max-w-[640px] items-center justify-between gap-3 px-4 py-2 sm:px-6">
        <div className="flex items-center gap-3">
          {currentStep > 0 ? (
            <button
              type="button"
              onClick={onBack}
              className="inline-flex min-h-[48px] items-center gap-1 text-sm font-semibold text-[var(--color-wi-text-light)] transition-colors motion-reduce:transition-none hover:text-[var(--color-wi-text)]"
            >
              <ChevronLeft className="h-4 w-4" />
              Back
            </button>
          ) : null}
          <div
            className="flex items-center gap-1.5"
            role="progressbar"
            aria-label="Form progress"
            aria-valuemin={1}
            aria-valuemax={totalSteps}
            aria-valuenow={currentStep + 1}
            aria-valuetext={`Step ${currentStep + 1} of ${totalSteps}`}
          >
            {Array.from({ length: totalSteps }, (_, i) => (
              <span
                key={i}
                className={clsx(
                  "inline-block h-2 w-2 rounded-full transition-colors motion-reduce:transition-none",
                  i <= currentStep ? "bg-[var(--color-wi-primary)]" : "bg-[var(--color-wi-border)]",
                )}
              />
            ))}
          </div>
        </div>
        <button
          type="button"
          onClick={onPrimary}
          disabled={!canProceed || loading}
          className={clsx(
            "min-h-[48px] min-w-[120px] rounded-xl px-5 text-base font-semibold transition-colors motion-reduce:transition-none",
            canProceed && !loading
              ? "bg-[var(--color-wi-primary)] text-white hover:bg-[var(--color-wi-primary-dark)]"
              : "cursor-not-allowed bg-gray-100 text-gray-400",
          )}
        >
          {loading ? (
            <span className="inline-flex items-center gap-2">
              <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
              </svg>
              Submitting...
            </span>
          ) : (
            primaryLabel
          )}
        </button>
      </div>
    </div>
  );
}
