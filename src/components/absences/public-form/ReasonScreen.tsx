import { useEffect } from "react";
import clsx from "clsx";
import { Check } from "lucide-react";
import ReasonField from "./ReasonField";

export type ReasonCategory = { value: string; label: string };

export const DEFAULT_REASON_CATEGORIES: ReasonCategory[] = [
  { value: "not_feeling_well", label: "Not feeling well" },
  { value: "appointment", label: "Appointment" },
  { value: "school_activity", label: "School activity" },
  { value: "family_commitment", label: "Family commitment" },
  { value: "travel", label: "Travel" },
  { value: "other", label: "Other" },
];

type ReasonScreenEmail = {
  required: boolean;
  value: string;
  onChange: (next: string) => void;
  invalid: boolean;
};

type ReasonScreenProps = {
  categories: ReasonCategory[];
  selected: string | null;
  detail: string;
  requireDetailFor: (value: string) => boolean;
  allowFreeText: boolean;
  required: boolean;
  onSelect: (value: string | null) => void;
  onDetailChange: (value: string) => void;
  error?: string | null;
  /** Shown inside Details when the school has no address on file for updates. */
  email?: ReasonScreenEmail;
  /** Focus the update-email field when the student arrives to edit it. */
  initialFocusOnEmail?: boolean;
};

export default function ReasonScreen({
  categories,
  selected,
  detail,
  requireDetailFor,
  allowFreeText,
  required,
  onSelect,
  onDetailChange,
  error = null,
  email,
  initialFocusOnEmail = false,
}: ReasonScreenProps) {
  const showDetail = Boolean(selected) && (allowFreeText || requireDetailFor(selected ?? ""));

  useEffect(() => {
    if (!initialFocusOnEmail || !email) return;
    const field = document.getElementById("absence-update-email");
    field?.focus();
  }, [initialFocusOnEmail, email]);
  const detailRequired = Boolean(selected) && requireDetailFor(selected ?? "");

  return (
    <div className="mx-auto w-full max-w-2xl">
      <h1 className="text-[28px] font-bold leading-tight tracking-tight text-[var(--color-wi-text)]">
        Why will you be away?
      </h1>
      <p className="mt-2 text-[17px] leading-relaxed text-[var(--color-wi-text-light)]">
        Pick the closest match{required ? "" : " — or tell us in your own words"}.
      </p>

      <div className="mt-7 space-y-2.5" role="radiogroup" aria-label="Reason">
        {categories.map((category) => {
          const isSelected = selected === category.value;
          return (
            <button
              key={category.value}
              type="button"
              role="radio"
              aria-checked={isSelected}
              onClick={() => onSelect(isSelected ? null : category.value)}
              className={clsx(
                "wi-press flex min-h-14 w-full items-center gap-3.5 rounded-xl border px-4 py-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]",
                isSelected
                  ? "border-[var(--color-wi-primary)]/50 bg-[var(--color-wi-primary)]/5"
                  : "border-[var(--color-wi-border)] bg-white hover:bg-[var(--color-wi-row-alt)] active:bg-[var(--color-wi-row-alt)]",
              )}
            >
              <span
                aria-hidden="true"
                className={clsx(
                  "flex h-6 w-6 shrink-0 items-center justify-center rounded-full border-2 transition-colors motion-reduce:transition-none",
                  isSelected
                    ? "border-[var(--color-wi-primary)] bg-[var(--color-wi-primary)] text-white"
                    : "border-[var(--color-wi-border)] bg-white",
                )}
              >
                {isSelected ? <Check className="h-4 w-4" strokeWidth={3} /> : null}
              </span>
              <span className="text-[15px] font-semibold text-[var(--color-wi-text)]">{category.label}</span>
            </button>
          );
        })}
      </div>

      {error ? (
        <p role="alert" className="mt-3 text-[15px] leading-snug text-[var(--color-wi-red)]">
          {error}
        </p>
      ) : null}

      {selected && !showDetail ? (
        <p className="mt-3 text-[13px] text-[var(--color-wi-text-light)]">
          That&apos;s enough — we won&apos;t ask for more detail for this reason.
        </p>
      ) : null}

      {showDetail ? (
        <div className="mt-7">
          <ReasonField
            value={detail}
            onChange={onDetailChange}
            error={null}
            required={detailRequired}
            label={detailRequired ? "Tell us a little more" : "Anything else we should know?"}
            counter="near-limit"
          />
        </div>
      ) : null}

      {email ? (
        <section aria-label="Where should we send updates?" className="mt-7 border-t border-[var(--color-wi-line)] pt-6">
          <h2 className="text-[20px] font-bold leading-tight tracking-tight text-[var(--color-wi-text)]">
            Where should we send updates?
          </h2>
          <p className="mt-1 text-[15px] leading-relaxed text-[var(--color-wi-text-light)]">
            We need an email to send updates about this absence.
          </p>
          <div className="mt-4">
            <label htmlFor="absence-update-email" className="block text-[15px] font-semibold text-[var(--color-wi-text)]">
              Email{email.required ? <span aria-hidden="true"> *</span> : null}
            </label>
            <input
              id="absence-update-email"
              type="email"
              autoComplete="email"
              className={clsx(
                "mt-2 h-[52px] w-full rounded-xl border bg-white px-4 text-[17px] text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:outline-none focus:ring-2",
                email.invalid
                  ? "border-[var(--color-wi-red)] focus:border-[var(--color-wi-red)] focus:ring-[var(--color-wi-red)]/20"
                  : "border-[var(--color-wi-border)] focus:border-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20",
              )}
              placeholder="name@example.com"
              value={email.value}
              onChange={(event) => email.onChange(event.target.value)}
              aria-invalid={email.invalid}
              aria-describedby={email.invalid ? "absence-update-email-error" : undefined}
            />
            {email.invalid ? (
              <p id="absence-update-email-error" role="alert" className="mt-2 text-[15px] leading-snug text-[var(--color-wi-red)]">
                Enter a valid email to continue.
              </p>
            ) : null}
          </div>
        </section>
      ) : null}
    </div>
  );
}