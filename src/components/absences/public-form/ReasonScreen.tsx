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
}: ReasonScreenProps) {
  const showDetail = Boolean(selected) && (allowFreeText || requireDetailFor(selected ?? ""));
  const detailRequired = Boolean(selected) && requireDetailFor(selected ?? "");

  return (
    <div className="mx-auto w-full max-w-xl">
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
                "flex min-h-14 w-full items-center gap-3.5 rounded-xl border px-4 py-3 text-left transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]",
                isSelected
                  ? "border-[var(--color-wi-primary)]/50 bg-[var(--color-wi-primary)]/5"
                  : "border-[var(--color-wi-border)] bg-white hover:bg-[var(--color-wi-row-alt)] active:bg-[var(--color-wi-row-alt)] active:scale-[0.995] motion-reduce:active:scale-100",
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
    </div>
  );
}