import { useEffect, useRef, useState } from "react";
import SearchableSelect from "@/components/ui/SearchableSelect";
import { ChevronRight } from "lucide-react";
import MobileBottomSheet from "./MobileBottomSheet";
import type { SitInDisplayModel } from "@/features/absences/domain/sitInResolution";

export type MakeUpOption = {
  value: string;
  label: string;
  details?: string;
  disabled?: boolean;
  description?: string;
  conflictDetails?: string;
  display?: SitInDisplayModel;
};

type MakeUpPickerProps = {
  id: string;
  label: string;
  value: string;
  options: MakeUpOption[];
  onChange: (value: string) => void;
  disabled?: boolean;
  publicForm?: boolean;
  context?: string;
  error?: string;
};

function optionLines(option: MakeUpOption, publicForm: boolean) {
  const display = publicForm ? option.display : undefined;
  return {
    primary: display?.date ? [display.date, [display.startTime, display.endTime].filter(Boolean).join("–")].filter(Boolean).join(" · ") : option.label,
    secondary: display?.date ? [display.className, display.teacherName].filter(Boolean).join(" · ") : option.details,
  };
}

export default function MakeUpPicker({ id, label, value, options, onChange, disabled = false, publicForm = false, context, error }: MakeUpPickerProps) {
  const [sheetOpen, setSheetOpen] = useState(false);
  // null = nothing chosen yet; "" = an explicit "Not yet selected" (clear).
  const [pendingValue, setPendingValue] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const selectedOption = options.find((option) => option.value === value);
  const selectedLines = selectedOption ? optionLines(selectedOption, publicForm) : null;
  const pendingOption = options.find((option) => option.value === pendingValue);
  const pendingInvalid = Boolean(pendingValue && (!pendingOption || pendingOption.disabled));
  const canConfirm = !disabled && pendingValue !== null && !pendingInvalid;
  const needle = query.trim().toLowerCase();
  const filteredOptions = options.filter((option) => `${option.label} ${option.details ?? ""} ${option.description ?? ""} ${option.conflictDetails ?? ""} ${option.value}`.toLowerCase().includes(needle));
  const selectOptions = [
    { value: "", label: "Not yet selected" },
    ...options.map((option) => ({
      ...option,
      label: [option.label, option.details, option.description, option.conflictDetails].filter(Boolean).join(" — "),
    })),
  ];

  useEffect(() => {
    if (value && selectedOption?.disabled) onChange("");
  }, [onChange, selectedOption?.disabled, value]);

  useEffect(() => {
    if (!sheetOpen || typeof window.matchMedia !== "function") return;
    const wideLayout = window.matchMedia("(min-width: 640px)");
    const closeOnWideLayout = () => {
      if (!wideLayout.matches) return;
      setSheetOpen(false);
      window.requestAnimationFrame(() => document.getElementById(id)?.focus());
    };
    closeOnWideLayout();
    wideLayout.addEventListener("change", closeOnWideLayout);
    return () => wideLayout.removeEventListener("change", closeOnWideLayout);
  }, [id, sheetOpen]);

  const clearOption = (
    <label className="absence-makeup-option flex min-h-14 items-center gap-3 rounded-xl border border-[var(--color-wi-border)] px-4 py-3 has-[:checked]:border-[var(--color-wi-primary)] has-[:checked]:bg-[var(--color-wi-primary)]/5">
      <input type="radio" name={`${id}-mobile`} value="" checked={pendingValue === ""} onChange={() => setPendingValue("")} className="h-5 w-5 shrink-0 text-[var(--color-wi-primary)]" />
      <span className="min-w-0 text-sm font-medium text-[var(--color-wi-text-light)]">{publicForm ? "Clear selection" : "Not yet selected"}</span>
    </label>
  );

  return (
    <div id={`${id}-field`} className="space-y-2">
      <label htmlFor={id} className="block text-xs font-semibold text-[var(--color-wi-text-light)]">{label}</label>
      <SearchableSelect
        id={id}
        name={id}
        value={value}
        options={selectOptions}
        disabled={disabled || options.length === 0}
        aria-invalid={Boolean(error)}
        aria-describedby={error ? `${id}-error` : undefined}
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
        aria-invalid={Boolean(error)}
        aria-describedby={error ? `${id}-error` : undefined}
        aria-label={selectedLines ? `${label}: ${selectedLines.primary} ${selectedLines.secondary ?? ""}` : "Choose a make-up class"}
        onClick={() => {
          setPendingValue(value || null);
          setQuery("");
          setSheetOpen(true);
        }}
        className="flex min-h-12 w-full items-center justify-between gap-3 rounded-xl border border-[var(--color-wi-border)] bg-white px-3 text-left text-base text-[var(--color-wi-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]/30 disabled:cursor-not-allowed disabled:opacity-60 sm:hidden"
      >
        <span className={selectedOption ? "min-w-0 font-medium" : "text-[var(--color-wi-text-light)]"}>
          <span className="block">{selectedLines?.primary ?? "Choose a make-up class"}</span>
          {selectedLines?.secondary ? <span className="mt-1 block text-xs font-normal text-[var(--color-wi-text-light)]">{selectedLines.secondary}</span> : null}
        </span>
        <ChevronRight className="h-5 w-5 shrink-0 text-[var(--color-wi-text-light)]" aria-hidden="true" />
      </button>
      {error ? <p id={`${id}-error`} className="text-xs text-[var(--color-wi-red)]">{error}</p> : null}

      {selectedOption?.details ? (
        <p className="hidden text-xs text-[var(--color-wi-text-light)] sm:block">{selectedOption.details}</p>
      ) : null}
      {selectedOption?.description ? (
        <p role="status" className="text-xs text-[var(--color-wi-amber)]">
          {selectedOption.description}
        </p>
      ) : null}
      {selectedOption?.conflictDetails ? (
        <p className="text-xs text-[var(--color-wi-text-light)]">{selectedOption.conflictDetails}</p>
      ) : null}

      <MobileBottomSheet
        open={sheetOpen}
        publicForm={publicForm}
        title="Choose a make-up class"
        description={publicForm ? context : undefined}
        onClose={() => setSheetOpen(false)}
        restoreFocusRef={triggerRef}
        toolbar={
          <div className="flex items-center gap-2">
            <input
              aria-label={`Search ${label.toLowerCase()}`}
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search classes…"
              enterKeyHint="search"
              className="min-h-12 min-w-0 flex-1 rounded-xl border border-[var(--color-wi-border)] px-4 text-base placeholder:text-[var(--color-wi-text-light)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
            />
            {query ? <button type="button" onClick={() => setQuery("")} className="min-h-11 min-w-11 rounded-lg px-2 text-sm font-medium text-[var(--color-wi-primary)]">Clear search</button> : null}
          </div>
        }
        footer={
          <>
            {pendingInvalid ? (
              <p role="status" className="mb-3 text-sm text-[var(--color-wi-amber)]">This class is no longer available. Choose another time.</p>
            ) : publicForm ? (
              <p className="mb-3 text-sm text-[var(--color-wi-text-light)]">
                {pendingOption ? `Selected: ${optionLines(pendingOption, publicForm).primary}` : pendingValue === "" ? "Your make-up selection will be cleared." : "Choose a class, then confirm."}
              </p>
            ) : null}
            <button
              type="button"
              disabled={!canConfirm}
              onClick={() => {
                if (!canConfirm) return;
                onChange(pendingValue ?? "");
                setSheetOpen(false);
              }}
              className="min-h-12 w-full rounded-xl bg-[var(--color-wi-primary)] px-4 py-3 text-base font-semibold text-white hover:bg-[var(--color-wi-primary-dark)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:bg-[var(--color-wi-row-alt)] disabled:text-[var(--color-wi-text-light)]"
            >
              Confirm make-up class
            </button>
          </>
        }
      >
        <fieldset className="space-y-2">
          <legend className="sr-only">{label}</legend>
          {!publicForm ? clearOption : null}
          {filteredOptions.length === 0 ? <p className="py-6 text-center text-sm text-[var(--color-wi-text-light)]">No classes match{needle ? ` “${query.trim()}”` : ""}.</p> : null}
          {filteredOptions.map((option) => {
            const lines = optionLines(option, publicForm);
            return (
              <label key={option.value} className="absence-makeup-option flex min-h-14 items-center gap-3 rounded-xl border border-[var(--color-wi-border)] px-4 py-3 has-[:checked]:border-[var(--color-wi-primary)] has-[:checked]:bg-[var(--color-wi-primary)]/5">
                <input
                  type="radio"
                  name={`${id}-mobile`}
                  value={option.value}
                  checked={pendingValue === option.value}
                  disabled={option.disabled}
                  onChange={(event) => setPendingValue(event.target.value)}
                  className="h-5 w-5 shrink-0 border-[var(--color-wi-border)] text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20"
                />
                <span className="min-w-0 break-words text-sm font-medium text-[var(--color-wi-text)]">
                  <span className="block">{lines.primary}</span>
                  {lines.secondary ? <span className="mt-1 block text-xs font-normal text-[var(--color-wi-text-light)]">{lines.secondary}</span> : null}
                  {publicForm && option.disabled ? <span className="mt-1 block text-xs font-medium text-[var(--color-wi-text-light)]">Unavailable</span> : null}
                  {option.description ? <span className="mt-0.5 block text-xs font-normal text-[var(--color-wi-text-light)]">{option.description}</span> : null}
                  {option.conflictDetails ? <span className="mt-0.5 block text-xs font-normal text-[var(--color-wi-text-light)]">{option.conflictDetails}</span> : null}
                </span>
              </label>
            );
          })}
          {publicForm ? clearOption : null}
        </fieldset>
      </MobileBottomSheet>
    </div>
  );
}
