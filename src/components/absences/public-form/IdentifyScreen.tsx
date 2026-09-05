import { useEffect, useRef } from "react";
import clsx from "clsx";
import { LoaderCircle } from "lucide-react";
import ScreenTitle, { ScreenSubtitle } from "./ScreenTitle";

type IdentifyScreenProps = {
  value: string;
  onChange: (value: string) => void;
  onSubmit: () => void;
  loading?: boolean;
  error?: string | null;
  canContinue?: boolean;
  /** When true, focuses the field and selects its value on mount (e.g. after "Not me"). */
  selectOnMount?: boolean;
};

export default function IdentifyScreen({
  value,
  onChange,
  onSubmit,
  loading = false,
  error = null,
  canContinue = false,
  selectOnMount = false,
}: IdentifyScreenProps) {
  const errorId = "identify-error";
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!selectOnMount) return;
    const input = inputRef.current;
    input?.focus();
    input?.select();
  }, [selectOnMount]);

  return (
    <div className="mx-auto w-full max-w-2xl">
      <ScreenTitle>
        Report an absence

      </ScreenTitle>
      <ScreenSubtitle>
        Enter your student ID to begin.

      </ScreenSubtitle>

      <div className="mt-8">
        <label htmlFor="wcode-input" className="block text-[15px] font-semibold text-[var(--color-wi-text)]">
          Student ID
        </label>
        <input
          ref={inputRef}
          id="wcode-input"
          autoComplete="off"
          spellCheck={false}
          className={clsx(
            "mt-2 h-[52px] w-full rounded-xl border bg-white px-4 text-[17px] text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:outline-none focus:ring-2",
            error
              ? "border-[var(--color-wi-red)] focus:border-[var(--color-wi-red)] focus:ring-[var(--color-wi-red)]/20"
              : "border-[var(--color-wi-border)] focus:border-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20",
          )}
          placeholder="e.g. W250389"
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
        ) : null}
      </div>

      <div className="mt-8 flex items-center gap-2 text-[13px] text-[var(--color-wi-text-light)]">
        <svg className="h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true">
          <path strokeLinecap="round" strokeLinejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
        </svg>
        Your draft stays in this browser tab — never on a shared account.
      </div>

      <button
        type="button"
        onClick={onSubmit}
        disabled={!canContinue || loading}
        aria-busy={loading}
        className={clsx(
          "wi-press mt-10 flex h-[52px] w-full items-center justify-center rounded-xl px-5 text-[17px] font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2",
          canContinue && !loading
            ? "bg-[var(--color-wi-primary)] text-white hover:bg-[var(--color-wi-primary-dark)]"
            : "cursor-not-allowed bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]",
        )}
      >
        {loading ? (
          <span className="inline-flex items-center justify-center gap-2">
            <LoaderCircle className="h-5 w-5 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Finding your profile…
          </span>
        ) : (
          "Continue"
        )}
      </button>
    </div>
  );
}