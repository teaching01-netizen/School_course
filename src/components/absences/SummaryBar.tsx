import clsx from "clsx";

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

export type DayBreakdown = {
  day: string;
  count: number;
  date: string;
};

export type SummaryBarProps = {
  absentCount: number;
  coverCount: number;
  dayBreakdown: DayBreakdown[];
  hasSelection: boolean;
  onBack: () => void;
  onScrollToDate: (date: string) => void;
  onSubmit: () => void;
};

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

export default function SummaryBar({
  absentCount,
  coverCount,
  dayBreakdown,
  hasSelection,
  onBack,
  onScrollToDate,
  onSubmit,
}: SummaryBarProps) {
  return (
    <div
      className={clsx(
        "sticky bottom-0 z-30 border-t border-gray-200 bg-white px-4 py-3 shadow-[0_-2px_8px_rgba(0,0,0,0.06)]",
      )}
      role="region"
      aria-label="Summary"
    >
      <div className="mx-auto flex max-w-3xl flex-wrap items-center justify-between gap-3">
        {/* Left: counts */}
        <p className="text-sm font-medium text-gray-800">
          Summary: {absentCount} absent, {coverCount} cover
        </p>

        {/* Centre: day chips */}
        {dayBreakdown.length > 0 && (
          <div className="flex flex-wrap gap-1.5" role="list" aria-label="Day breakdown">
            {dayBreakdown.map((d) => (
              <button
                key={d.date}
                type="button"
                role="listitem"
                className={clsx(
                  "rounded-full border border-gray-300 bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-200",
                )}
                onClick={() => onScrollToDate(d.date)}
              >
                {d.day}: {d.count}
              </button>
            ))}
          </div>
        )}

        {/* Right: navigation */}
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="inline-flex items-center justify-center rounded-sm border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 min-h-[34px]"
            onClick={onBack}
          >
            Back
          </button>

          <button
            type="button"
            disabled={!hasSelection}
            className={clsx(
              "inline-flex items-center justify-center rounded-sm border border-transparent px-3 py-1.5 text-sm font-medium transition-colors min-h-[34px]",
              hasSelection
                ? "bg-[var(--color-wi-primary)] text-white hover:bg-[var(--color-wi-primary-dark)]"
                : "cursor-not-allowed bg-gray-200 text-gray-400",
            )}
            onClick={onSubmit}
          >
            Submit
          </button>
        </div>
      </div>
    </div>
  );
}
