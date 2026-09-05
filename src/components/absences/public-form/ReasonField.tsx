import clsx from "clsx";

type ReasonFieldProps = {
  value: string;
  onChange: (value: string) => void;
  error?: string | null;
  required?: boolean;
  /** Overrides the default "Reason for absence" label. */
  label?: string;
  /**
   * "always" (default) shows the character count and progress bar at all
   * times. "near-limit" keeps the field quiet and only reveals the count
   * as the student approaches the 500-character maximum.
   */
  counter?: "always" | "near-limit";
};

export default function ReasonField({
  value,
  onChange,
  error,
  required = true,
  label = "Reason for absence",
  counter = "always",
}: ReasonFieldProps) {
  const errorId = "reason-error";
  const count = value.length;
  const percentage = Math.min((count / 500) * 100, 100);
  const nearLimit = count > 400;

  return (
    <section className="absence-reason-field">
      <div className="mb-2 flex flex-wrap items-end justify-between gap-2">
        <label htmlFor="absence-reason" className="text-[15px] font-semibold text-[var(--color-wi-text)]">
          {label}{required ? <span aria-hidden="true"> *</span> : null}
        </label>
        {/* Visual-only: announcing "N/500" on every keystroke would flood
            screen readers; the textarea's own output is already announced. */}
        {counter === "always" || nearLimit ? (
          <span className={clsx(
            "text-xs tabular-nums",
            count >= 500 ? "font-semibold text-[var(--color-wi-red)]" : count > 450 ? "font-semibold text-[var(--color-wi-amber-ink)]" : "text-[var(--color-wi-text-light)]",
          )}>
            {count}/500 characters
          </span>
        ) : null}
      </div>
      {counter === "always" ? (
        <div className="mb-2 h-1.5 overflow-hidden rounded-full bg-[var(--color-wi-row-alt)]" aria-hidden="true">
          <div
            className={clsx(
              "h-full rounded-full transition-[width,background-color] duration-300 motion-reduce:transition-none",
              count > 450 ? "bg-[var(--color-wi-amber)]" : count > 0 ? "bg-[var(--color-wi-primary)]" : "bg-transparent",
            )}
            style={{ width: `${percentage}%` }}
          />
        </div>
      ) : null}
      <textarea
        id="absence-reason"
        name="reason"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        maxLength={500}
        required={required}
        placeholder="Tell us why you'll be away from class..."
        aria-invalid={Boolean(error)}
        aria-describedby={error ? errorId : undefined}
        className={clsx(
          "min-h-[120px] w-full resize-y rounded-xl border bg-white px-4 py-3 text-[17px] leading-6 text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:outline-none focus:ring-2",
          error ? "border-[var(--color-wi-red)] focus:ring-[var(--color-wi-red)]/20" : "border-[var(--color-wi-border)] focus:border-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20",
        )}
      />
      {error ? <p id={errorId} role="alert" className="mt-1.5 text-sm text-[var(--color-wi-red)]">{error}</p> : null}
    </section>
  );
}