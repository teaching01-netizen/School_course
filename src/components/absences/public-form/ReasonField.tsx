import clsx from "clsx";

type ReasonFieldProps = {
  value: string;
  onChange: (value: string) => void;
  error?: string | null;
  required?: boolean;
  publicForm?: boolean;
};

export default function ReasonField({ value, onChange, error, required = true, publicForm = false }: ReasonFieldProps) {
  const errorId = "reason-error";
  const count = value.length;
  const percentage = Math.min((count / 500) * 100, 100);

  return (
    <section className="absence-reason-field">
      <div className="mb-2 flex flex-wrap items-end justify-between gap-2">
        <label htmlFor="absence-reason" className="text-sm font-semibold text-[var(--color-wi-text)]">
          Reason for absence{required ? <span aria-hidden="true"> *</span> : null}
        </label>
        {/* Visual-only: announcing "N/500" on every keystroke would flood
            screen readers; the textarea's own output is already announced. */}
        {!publicForm ? <span className={clsx(
          "text-xs tabular-nums",
          !publicForm && count >= 500 ? "font-semibold text-[var(--color-wi-red)]" : count > 450 && count < 500 ? "font-semibold text-[var(--color-wi-amber)]" : "text-[var(--color-wi-text-light)]",
        )}>
          {count}/500 characters
        </span> : null}
      </div>
      {publicForm ? <p id="reason-help" className="mb-3 text-xs text-[var(--color-wi-text-light)]">Briefly tell us why you will miss these classes.</p> : <div className="mb-2 h-1.5 overflow-hidden rounded-full bg-[var(--color-wi-row-alt)]" aria-hidden="true">
        <div
          className={clsx(
            "h-full rounded-full transition-[width,background-color] duration-300 motion-reduce:transition-none",
            count > 450 ? "bg-[var(--color-wi-amber)]" : count > 0 ? "bg-[var(--color-wi-primary)]" : "bg-transparent",
          )}
          style={{ width: `${percentage}%` }}
        />
      </div>}
      <textarea
        id="absence-reason"
        name="reason"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        maxLength={500}
        required={required}
        placeholder="Tell us why you'll be away from class..."
        aria-invalid={Boolean(error)}
        aria-describedby={error ? errorId : publicForm ? "reason-help" : undefined}
        className={clsx(
          "min-h-[120px] w-full resize-y rounded-xl border bg-white px-4 py-3 text-base leading-6 text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:outline-none focus:ring-2",
          error ? "border-[var(--color-wi-red)] focus:ring-[var(--color-wi-red)]/20" : "border-[var(--color-wi-border)] focus:border-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20",
        )}
      />
      {publicForm ? <p className="mt-2 text-xs text-[var(--color-wi-text-light)]">{count === 500 ? "Character limit reached" : count > 450 ? `${500 - count} characters remaining` : "500 characters maximum"}</p> : null}
      {error ? <p id={errorId} className="mt-1.5 text-xs text-[var(--color-wi-red)]">{error}</p> : null}
    </section>
  );
}
