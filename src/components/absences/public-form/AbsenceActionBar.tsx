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
}: AbsenceActionBarProps) {
  const [slow, setSlow] = useState(false);
  useEffect(() => {
    if (!loading) {
      setSlow(false);
      return;
    }
    const timer = window.setTimeout(() => setSlow(true), 4000);
    return () => window.clearTimeout(timer);
  }, [loading]);

  return (
    <div className="absence-action-bar">
      <div className="absence-action-bar__inner">
        {showBack ? (
          <button
            type="button"
            onClick={onBack}
            disabled={loading}
            className="inline-flex min-h-12 items-center gap-1 rounded-lg px-3 text-[15px] font-semibold text-[var(--color-wi-text-light)] transition-colors motion-reduce:transition-none hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] active:scale-[0.98] motion-reduce:active:scale-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] disabled:opacity-50"
          >
            <ChevronLeft className="h-5 w-5" aria-hidden="true" />
            <span>Back</span>
          </button>
        ) : <span aria-hidden="true" className="min-w-20" />}
        {showPrimary ? (
          <button
            type="button"
            onClick={onPrimary}
            disabled={!canProceed || loading}
            className={clsx(
              "min-h-[52px] min-w-[10rem] rounded-xl px-5 text-[17px] font-semibold transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2",
              canProceed && !loading
                ? "bg-[var(--color-wi-primary)] text-white hover:bg-[var(--color-wi-primary-dark)] active:scale-[0.99] motion-reduce:active:scale-100"
                : "cursor-not-allowed bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]",
            )}
          >
            {loading ? (
              <span className="inline-flex items-center justify-center gap-2">
                <LoaderCircle className="h-5 w-5 animate-spin motion-reduce:animate-none" aria-hidden="true" />
                {slow ? loadingSlowLabel : loadingLabel}
              </span>
            ) : primaryLabel}
          </button>
        ) : <span aria-hidden="true" className="min-w-20" />}
      </div>
    </div>
  );
}