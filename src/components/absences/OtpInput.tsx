import { useEffect, useMemo, useRef, useState, type ChangeEvent, type ClipboardEvent } from "react";
import clsx from "clsx";

type OtpInputProps = {
  value: string;
  onChange: (next: string) => void;
  onComplete?: (code: string) => void;
  disabled?: boolean;
  error?: boolean;
  autoFocus?: boolean;
  describedBy?: string;
  label?: string;
};

function normalizeOtp(value: string): string {
  return value.replace(/\D/g, "").slice(0, 6);
}

export default function OtpInput({
  value,
  onChange,
  onComplete,
  disabled = false,
  error = false,
  autoFocus = false,
  describedBy,
  label = "Enter the code",
}: OtpInputProps) {
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [isFocused, setIsFocused] = useState(false);
  const digits = useMemo(() => normalizeOtp(value).padEnd(6, " "), [value]);
  const currentLength = normalizeOtp(value).length;

  useEffect(() => {
    if (autoFocus) {
      inputRef.current?.focus();
      inputRef.current?.select();
    }
  }, [autoFocus]);

  useEffect(() => {
    const normalized = normalizeOtp(value);
    if (normalized.length === 6) {
      onComplete?.(normalized);
    }
  }, [value, onComplete]);

  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    const next = normalizeOtp(event.target.value);
    onChange(next);
  };

  const handlePaste = (event: ClipboardEvent<HTMLInputElement>) => {
    const pasted = event.clipboardData.getData("text");
    if (!pasted) return;
    event.preventDefault();
    const next = normalizeOtp(pasted);
    onChange(next);
  };

  return (
    <div className="space-y-4">
      <label className="block text-sm font-medium text-[var(--color-wi-text)]">
        <span className="mb-3 block text-base font-semibold text-[var(--color-wi-text)]">{label}</span>
        <div
          className="relative flex items-center justify-center gap-3 cursor-pointer"
          onClick={() => inputRef.current?.focus()}
        >
          {Array.from({ length: 6 }, (_, index) => {
            const isCurrentEmpty = index === currentLength && currentLength < 6;
            return (
              <div
                key={index}
              className={clsx(
                "flex h-16 w-16 items-center justify-center rounded-md border bg-white font-mono text-3xl tabular-nums text-[var(--color-wi-text)] shadow-sm transition-all duration-150",
                error
                  ? "border-[var(--color-wi-red)]"
                  : clsx(
                      "border-gray-200 hover:border-[var(--color-wi-primary)]/40 hover:bg-gray-50",
                      index < currentLength && "border-[var(--color-wi-primary)]",
                      isCurrentEmpty && isFocused && "border-[var(--color-wi-primary)] ring-2 ring-[var(--color-wi-primary)]/20",
                    ),
              )}
                aria-hidden="true"
              >
                {digits[index] !== " " ? (
                  digits[index]
                ) : (
                  isCurrentEmpty && isFocused && (
                    <span className="animate-blink h-6 w-0.5 bg-[var(--color-wi-primary)]" />
                  )
                )}
              </div>
            );
          })}
          <input
            ref={inputRef}
            type="text"
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern="[0-9]*"
            maxLength={6}
            disabled={disabled}
            value={value}
            onChange={handleChange}
            onPaste={handlePaste}
            onFocus={() => setIsFocused(true)}
            onBlur={() => setIsFocused(false)}
            onInvalid={(event) => event.preventDefault()}
            aria-label={label}
            aria-describedby={describedBy}
            className="absolute left-0 top-0 h-px w-px opacity-0"
          />
        </div>
      </label>
    </div>
  );
}
