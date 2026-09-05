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
        Your parent is confirmed. We don&apos;t have an email for absence updates yet — add one to track this request.
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
            We&apos;ll use this to send updates about this absence. Use the Continue button below.
          </p>
        )}
      </div>

      {/* Primary lives in the sticky footer (AbsenceActionBar) so it stays visible with the keyboard open. Enter still submits via onSubmit. */}
    </div>
  );
}