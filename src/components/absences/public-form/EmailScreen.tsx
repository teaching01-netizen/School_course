import clsx from "clsx";

type EmailScreenProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit: () => void;
  loading?: boolean;
  error?: string | null;
  canContinue?: boolean;
};

export default function EmailScreen({
  value,
  onChange,
  onSubmit,
  loading = false,
  error = null,
  canContinue = false,
}: EmailScreenProps) {
  const errorId = "email-screen-error";
  return (
    <div className="mx-auto w-full max-w-xl">
      <h1 className="text-[28px] font-bold leading-tight tracking-tight text-[var(--color-wi-text)]">
        Where should we send updates?
      </h1>
      <p className="mt-2 text-[17px] leading-relaxed text-[var(--color-wi-text-light)]">
        We don&apos;t have an email address for you yet.
      </p>

      <div className="mt-8">
        <label htmlFor="student-email" className="block text-[15px] font-semibold text-[var(--color-wi-text)]">
          Email
        </label>
        <input
          id="student-email"
          type="email"
          autoComplete="email"
          className={clsx(
            "mt-2 h-[52px] w-full rounded-xl border bg-white px-4 text-[17px] text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:outline-none focus:ring-2",
            error
              ? "border-[var(--color-wi-red)] focus:border-[var(--color-wi-red)] focus:ring-[var(--color-wi-red)]/20"
              : "border-[var(--color-wi-border)] focus:border-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20",
          )}
          placeholder="name@example.com"
          value={value}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && canContinue && !loading) onSubmit();
          }}
          aria-invalid={Boolean(error)}
          aria-describedby={error ? errorId : undefined}
        />
        {error ? (
          <p id={errorId} role="alert" className="mt-2 text-[15px] leading-snug text-[var(--color-wi-red)]">
            {error}
          </p>
        ) : (
          <p className="mt-2 text-[13px] text-[var(--color-wi-text-light)]">
            We&apos;ll use this to send updates about this absence.
          </p>
        )}
      </div>

      <button
        type="button"
        onClick={onSubmit}
        disabled={!canContinue || loading}
        className={clsx(
          "mt-10 flex h-[52px] w-full items-center justify-center rounded-xl px-5 text-[17px] font-semibold transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2",
          canContinue && !loading
            ? "bg-[var(--color-wi-primary)] text-white hover:bg-[var(--color-wi-primary-dark)]"
            : "cursor-not-allowed bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]",
        )}
      >
        Continue
      </button>
    </div>
  );
}