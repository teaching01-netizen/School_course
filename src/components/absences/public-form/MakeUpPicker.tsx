import { useEffect, useRef, useState } from "react";
import SearchableSelect from "@/components/ui/SearchableSelect";
import { ChevronRight } from "lucide-react";
import MobileBottomSheet from "./MobileBottomSheet";

export type MakeUpOption = {
  value: string;
  label: string;
  disabled?: boolean;
  description?: string;
};

type MakeUpPickerProps = {
  id: string;
  label: string;
  value: string;
  options: MakeUpOption[];
  onChange: (value: string) => void;
  disabled?: boolean;
};

export default function MakeUpPicker({ id, label, value, options, onChange, disabled = false }: MakeUpPickerProps) {
  const [sheetOpen, setSheetOpen] = useState(false);
  // null = nothing chosen yet; "" = an explicit "Not yet selected" (clear).
  const [pendingValue, setPendingValue] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const selectedOption = options.find((option) => option.value === value);
  const selectOptions = [{ value: "", label: "Not yet selected" }, ...options];

  useEffect(() => {
    if (value && selectedOption?.disabled) onChange("");
  }, [onChange, selectedOption?.disabled, value]);

  return (
    <div className="space-y-2">
      <label htmlFor={id} className="block text-xs font-semibold text-[var(--color-wi-text-light)]">{label}</label>
      <SearchableSelect
        id={id}
        name={id}
        value={value}
        options={selectOptions}
        disabled={disabled || options.length === 0}
        onChange={(event) => onChange(event.target.value)}
        className="hidden min-h-12 w-full rounded-xl border border-[var(--color-wi-border)] bg-white px-3 text-base text-[var(--color-wi-text)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20 sm:block"
      />

      <button
        data-make-up-trigger="true"
        ref={triggerRef}
        type="button"
        disabled={disabled || options.length === 0}
        aria-haspopup="dialog"
        aria-expanded={sheetOpen}
        onClick={() => {
          setPendingValue(value || null);
          setQuery("");
          setSheetOpen(true);
        }}
        className="flex min-h-12 w-full items-center justify-between gap-3 rounded-xl border border-[var(--color-wi-border)] bg-white px-3 text-left text-base text-[var(--color-wi-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]/30 disabled:cursor-not-allowed disabled:opacity-60 sm:hidden"
      >
        <span className={selectedOption ? "font-medium" : "text-[var(--color-wi-text-light)]"}>{selectedOption?.label ?? "Choose a make-up class"}</span>
        <ChevronRight className="h-5 w-5 shrink-0 text-[var(--color-wi-text-light)]" aria-hidden="true" />
      </button>

      {selectedOption?.description ? (
        <p role="status" className="text-xs text-amber-700">
          {selectedOption.description}
        </p>
      ) : null}

      <MobileBottomSheet
        open={sheetOpen}
        title="Choose a make-up class"
        onClose={() => setSheetOpen(false)}
        restoreFocusRef={triggerRef}
      >
        <input
          aria-label={`Search ${label.toLowerCase()}`}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search classes…"
          className="mb-3 h-11 w-full rounded-xl border border-[var(--color-wi-border)] px-4 text-base placeholder:text-[var(--color-wi-text-light)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
        />
        <fieldset className="space-y-2">
          <legend className="sr-only">{label}</legend>
          {(() => {
            const needle = query.trim().toLowerCase();
            const filteredOptions = options.filter((option) => `${option.label} ${option.value}`.toLowerCase().includes(needle));
            if (filteredOptions.length === 0) {
              return (
                <p className="py-6 text-center text-sm text-[var(--color-wi-text-light)]">
                  No classes match{needle ? ` “${query.trim()}”` : ""}.
                </p>
              );
            }
            return (
              <>
                <label className="flex min-h-14 items-center gap-3 rounded-xl border border-[var(--color-wi-border)] px-4 py-3 has-[:checked]:border-[var(--color-wi-primary)] has-[:checked]:bg-[var(--color-wi-primary)]/5">
                  <input
                    type="radio"
                    name={`${id}-mobile`}
                    value=""
                    checked={pendingValue === ""}
                    onChange={() => setPendingValue("")}
                    className="h-5 w-5 border-[var(--color-wi-border)] text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20"
                  />
                  <span className="min-w-0 break-words text-sm font-medium text-[var(--color-wi-text-light)]">Not yet selected</span>
                </label>
                {filteredOptions.map((option) => (
                  <label key={option.value} className="flex min-h-14 items-center gap-3 rounded-xl border border-[var(--color-wi-border)] px-4 py-3 has-[:checked]:border-[var(--color-wi-primary)] has-[:checked]:bg-[var(--color-wi-primary)]/5">
                    <input
                      type="radio"
                      name={`${id}-mobile`}
                      value={option.value}
                      checked={pendingValue === option.value}
                      disabled={option.disabled}
                      onChange={(event) => setPendingValue(event.target.value)}
                      className="h-5 w-5 border-[var(--color-wi-border)] text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20"
                    />
                    <span className="min-w-0 break-words text-sm font-medium text-[var(--color-wi-text)]">
                      <span className="block">{option.label}</span>
                      {option.description ? <span className="mt-0.5 block text-xs font-normal text-[var(--color-wi-text-light)]">{option.description}</span> : null}
                    </span>
                  </label>
                ))}
              </>
            );
          })()}
          <button
            type="button"
            disabled={pendingValue == null}
            onClick={() => {
              onChange(pendingValue ?? "");
              setSheetOpen(false);
            }}
            className="mt-3 min-h-12 w-full rounded-xl bg-[var(--color-wi-primary)] px-4 text-base font-semibold text-white hover:bg-[var(--color-wi-primary-dark)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:bg-[var(--color-wi-row-alt)] disabled:text-[var(--color-wi-text-light)]"
          >
            Confirm make-up class
          </button>
        </fieldset>
      </MobileBottomSheet>
    </div>
  );
}
