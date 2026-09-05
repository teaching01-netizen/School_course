import { useEffect, useState } from "react";
import clsx from "clsx";
import { ChevronLeft, LoaderCircle } from "lucide-react";

type AbsenceActionBarProps = {
  showBack?: boolean;
  showPrimary?: boolean;
  canProceed: boolean;
  loading?: boolean;
  loadingLabel?: string;
  /** Replaces the loading label when the operation runs long (~4s+). */
  loadingSlowLabel?: string;
  onBack: () => void;
  onPrimary: () => void;
  primaryLabel: string;
  /** Hint announced alongside a disabled primary (e.g. why Continue is disabled). */
  hint?: string;
};

export default function AbsenceActionBar({
  showBack = false,
  showPrimary = true,
  canProceed,
  loading = false,
  loadingLabel = "Submitting…",
  loadingSlowLabel = "Still submitting…",
  onBack,
  onPrimary,
  primaryLabel,
  hint,
}: AbsenceActionBarProps) {
  const [slow, setSlow] = useState(false);
  const [showSpinner, setShowSpinner] = useState(false);
  useEffect(() => {
    if (!loading) {
      setSlow(false);
      setShowSpinner(false);
      return;
    }
    // The label swap acknowledges the tap instantly; the spinner only appears
    // once the operation is actually taking long enough to need one, so fast
    // actions never flash a loader.
    const spinnerTimer = window.setTimeout(() => setShowSpinner(true), 350);
    const slowTimer = window.setTimeout(() => setSlow(true), 4000);
    return () => {
      window.clearTimeout(spinnerTimer);
      window.clearTimeout(slowTimer);
    };
  }, [loading]);
  const hintId = hint ? "absence-action-hint" : undefined;

  return (
    <div className="absence-action-bar">
      <div className="absence-action-bar__inner">
        {showBack ? (
          <button
            type="button"
            onClick={onBack}
            disabled={loading}
            className="wi-press inline-flex min-h-12 items-center gap-1 rounded-lg px-3 text-[15px] font-semibold text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] disabled:opacity-50"
          >
            <ChevronLeft className="h-5 w-5" aria-hidden="true" />
            <span>Back</span>
          </button>
        ) : <span aria-hidden="true" className="min-w-20" />}
        {showPrimary ? (
          <span className="flex min-w-[10rem] flex-col items-end gap-1">
            <button
              type="button"
              onClick={onPrimary}
              disabled={!canProceed || loading}
              aria-describedby={hint && !canProceed ? hintId : undefined}
              title={hint && !canProceed ? hint : undefined}
            className={clsx(
              "wi-press min-h-[52px] min-w-[10rem] rounded-xl px-5 text-[17px] font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2",
              canProceed && !loading
                ? "bg-[var(--color-wi-primary)] text-white hover:bg-[var(--color-wi-primary-dark)]"
                : "cursor-not-allowed bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]",
            )}
            >
              {loading ? (
                <span role="status" className="inline-flex items-center justify-center gap-2">
                  {showSpinner ? <LoaderCircle className="h-5 w-5 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : null}
                  {slow ? loadingSlowLabel : loadingLabel}
                </span>
              ) : primaryLabel}
            </button>
            {hint && !canProceed && !loading ? (
              <span id={hintId} className="max-w-[14rem] text-right text-[12px] leading-snug text-[var(--color-wi-text-light)]">
                {hint}
              </span>
            ) : null}
          </span>
        ) : <span aria-hidden="true" className="min-w-20" />}
      </div>
    </div>
  );
}
