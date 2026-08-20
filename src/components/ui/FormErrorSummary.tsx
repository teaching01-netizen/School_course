import { useEffect, useRef } from "react";
import { AlertTriangle } from "lucide-react";

interface FormErrorSummaryProps {
  errors: Record<string, string>;
  touched?: Record<string, boolean>;
  autoFocus?: boolean;
  className?: string;
}

export default function FormErrorSummary({ errors, touched, autoFocus = true, className = "" }: FormErrorSummaryProps) {
  const entries = Object.entries(errors).filter(([field]) => !touched || touched[field]);
  const ref = useRef<HTMLDivElement>(null);
  const hasErrors = entries.length > 0;

  useEffect(() => {
    if (hasErrors && autoFocus && ref.current) {
      ref.current.focus();
    }
  }, [hasErrors, autoFocus]);

  if (!hasErrors) return null;

  const handleClick = (field: string) => {
    const el = document.getElementById(`field-${field}`);
    el?.focus();
  };

  return (
    <div
      ref={ref}
      tabIndex={-1}
      role="alert"
      aria-live="assertive"
      className={`rounded-sm border border-[var(--color-wi-red)]/25 bg-[var(--color-wi-danger-bg)] px-3 py-2 text-sm outline-none ${className}`}
    >
      <p className="mb-1 flex items-center gap-1.5 font-semibold text-[var(--color-wi-red-dark)]">
        <AlertTriangle className="w-4 h-4" aria-hidden="true" />
        Please fix the following {entries.length === 1 ? "error" : `${entries.length} errors`}:
      </p>
      <ul className="list-disc pl-5 space-y-0.5">
        {entries.map(([field, msg]) => (
          <li key={field}>
            <button
              type="button"
              onClick={() => handleClick(field)}
              className="text-left text-sm text-[var(--color-wi-red)] underline hover:text-[var(--color-wi-red-dark)]"
            >
              {msg}
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
