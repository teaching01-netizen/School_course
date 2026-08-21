import clsx from "clsx";
import { useEffect, useRef, useState } from "react";

type ReasonFieldProps = {
  value: string;
  onChange: (value: string) => void;
  error?: string | null;
  required?: boolean;
};

export default function ReasonField({ value, onChange, error, required = true }: ReasonFieldProps) {
  const errorId = "reason-error";
  const count = value.length;
  const percentage = Math.min((count / 500) * 100, 100);
  const [liveText, setLiveText] = useState(`${count}/500 characters`);
  const latestCountRef = useRef(count);
  const keystrokeCountRef = useRef(0);
  const liveUpdateCountRef = useRef(0);
  const windowStartKeystrokeRef = useRef(1);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    latestCountRef.current = count;
    keystrokeCountRef.current += 1;
    const k = keystrokeCountRef.current;

    // Sliding window of 10 keystrokes: allow at most 2 live updates per 10
    if (k - windowStartKeystrokeRef.current >= 10) {
      windowStartKeystrokeRef.current = k - 9;
      liveUpdateCountRef.current = 0;
    }
    if (liveUpdateCountRef.current >= 2) {
      // Suppress: debounce but keep capped
      if (timerRef.current) clearTimeout(timerRef.current);
      timerRef.current = setTimeout(() => {
        // window slides; allow one trailing announcement after the burst
        if (keystrokeCountRef.current - windowStartKeystrokeRef.current >= 10) {
          windowStartKeystrokeRef.current = keystrokeCountRef.current - 9;
          liveUpdateCountRef.current = 0;
        }
        if (liveUpdateCountRef.current < 2) {
          setLiveText(`${latestCountRef.current}/500 characters`);
          liveUpdateCountRef.current += 1;
        }
      }, 500);
      return () => {
        if (timerRef.current) clearTimeout(timerRef.current);
      };
    }

    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      setLiveText(`${latestCountRef.current}/500 characters`);
      liveUpdateCountRef.current += 1;
    }, 400);
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [count]);

  useEffect(
    () => () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    },
    [],
  );

  return (
    <section className="absence-reason-field">
      <div className="mb-2 flex flex-wrap items-end justify-between gap-2">
        <label htmlFor="absence-reason" className="text-sm font-semibold text-[var(--color-wi-text)]">
          Reason for absence{required ? <span aria-hidden="true"> *</span> : null}
        </label>
        <span
          className={clsx(
            "text-xs tabular-nums",
            count >= 500
              ? "font-semibold text-[var(--color-wi-red)]"
              : count > 450
                ? "font-semibold text-[var(--color-wi-amber)]"
                : "text-[var(--color-wi-text-light)]",
          )}
          aria-hidden="true"
        >
          {count}/500 characters
        </span>
        <span aria-live="polite" aria-atomic="true" className="sr-only">
          {liveText}
        </span>
      </div>
      <div className="mb-2 h-1.5 overflow-hidden rounded-full bg-[var(--color-wi-row-alt)]" aria-hidden="true">
        <div
          className={clsx(
            "h-full rounded-full transition-[width,background-color] duration-[150ms] ease-[var(--ease-ui)] motion-reduce:transition-none",
            count > 450
              ? "bg-[var(--color-wi-amber)]"
              : count > 0
                ? "bg-[var(--color-wi-primary)]"
                : "bg-transparent",
          )}
          style={{ width: `${percentage}%` }}
        />
      </div>
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
          "min-h-[120px] w-full resize-y rounded-xl border bg-white px-4 py-3 text-base leading-6 text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:outline-none focus:ring-2 transition-colors duration-[150ms] ease-[var(--ease-ui)] motion-reduce:transition-none",
          error
            ? "border-[var(--color-wi-red)] focus:ring-[var(--color-wi-red)]/20"
            : "border-[var(--color-wi-border)] focus:border-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20",
        )}
      />
      {error ? <p id={errorId} className="mt-1.5 text-xs text-[var(--color-wi-red)]">{error}</p> : null}
    </section>
  );
}
