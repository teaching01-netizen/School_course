import { useEffect, useRef, useState } from "react";
import SearchableSelect from "@/components/ui/SearchableSelect";
import { ChevronRight, CircleAlert, LoaderCircle } from "lucide-react";
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
  priorityLevel?: number;
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
  moreTimes?: {
    hasMore: boolean;
    loading: boolean;
    error?: string;
    onLoad: () => void;
  };
  emptyDescription?: string;
  supportContact?: { email?: string; phone?: string };
};

function optionLines(option: MakeUpOption, publicForm: boolean) {
  const display = publicForm ? option.display : undefined;
  return {
    primary: display?.date ? [display.date, [display.startTime, display.endTime].filter(Boolean).join("–")].filter(Boolean).join(" · ") : option.label,
    secondary: display?.date ? [display.className, display.teacherName].filter(Boolean).join(" · ") : option.details,
  };
}

export default function MakeUpPicker({ id, label, value, options, onChange, disabled = false, publicForm = false, context, error, moreTimes, emptyDescription, supportContact }: MakeUpPickerProps) {
  const [sheetOpen, setSheetOpen] = useState(false);
  // null = nothing chosen yet; "" = an explicit "Not yet selected" (clear).
  const [pendingValue, setPendingValue] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const choicesRef = useRef<HTMLFieldSetElement | null>(null);
  const moreRequest = useRef<{ button: HTMLButtonElement; count: number } | null>(null);
  const selectedOption = options.find((option) => option.value === value);
  const lastSelection = useRef<MakeUpOption | undefined>(undefined);
  useEffect(() => { if (selectedOption) lastSelection.current = selectedOption; }, [selectedOption]);
  const displayedSelection = selectedOption ?? (lastSelection.current?.value === value ? lastSelection.current : undefined);
  const selectedLines = displayedSelection ? optionLines(displayedSelection, publicForm) : null;
  const selectionInvalid = Boolean(value && (!selectedOption || selectedOption.disabled));
  const pendingOption = options.find((option) => option.value === pendingValue);
  const pendingInvalid = Boolean(pendingValue && (!pendingOption || pendingOption.disabled));
  const canConfirm = !disabled && pendingValue !== null && !pendingInvalid;
  const availableCount = options.filter((option) => !option.disabled).length;
  const needle = query.trim().toLowerCase();
  const filteredOptions = options.filter((option) => `${option.label} ${option.details ?? ""} ${option.description ?? ""} ${option.conflictDetails ?? ""} ${option.value}`.toLowerCase().includes(needle));
  if (publicForm) filteredOptions.sort((a, b) => Number(Boolean(a.disabled)) - Number(Boolean(b.disabled)));
  const selectOptions = [
    { value: "", label: "Not yet selected" },
    ...options.map((option) => ({
      ...option,
      label: [option.label, option.details, option.description, option.conflictDetails].filter(Boolean).join(" — "),
    })),
  ];

  useEffect(() => {
    if (!publicForm && value && selectedOption?.disabled) onChange("");
  }, [onChange, publicForm, selectedOption?.disabled, value]);

  useEffect(() => {
    const request = moreRequest.current;
    if (!request || moreTimes?.loading || options.length <= request.count) return;
    moreRequest.current = null;
    if (!sheetOpen || request.button.isConnected) return;
    const newChoice = choicesRef.current?.querySelector<HTMLInputElement>(`input[data-option-index="${request.count}"]:not(:disabled)`);
    const status = choicesRef.current?.parentElement?.querySelector<HTMLElement>("[data-more-times-status]");
    (newChoice ?? status)?.focus({ preventScroll: true });
  }, [moreTimes?.loading, options.length, sheetOpen]);

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

  const moreTimesControl = moreTimes ? (
    <div className="absence-makeup-more">
      {moreTimes.error ? <p role="status" className="absence-makeup-more__error">{moreTimes.error}</p> : null}
      {moreTimes.hasMore || moreTimes.error ? (
        <button type="button" disabled={moreTimes.loading} onClick={(event) => {
          moreRequest.current = { button: event.currentTarget, count: options.length };
          moreTimes.onLoad();
        }} className="absence-makeup-more__button" aria-busy={moreTimes.loading}>
          {moreTimes.loading ? <LoaderCircle aria-hidden="true" className="h-4 w-4 animate-spin motion-reduce:animate-none" /> : null}
          {moreTimes.loading ? "Loading more times…" : moreTimes.error ? "Try loading more times again" : "Show more times"}
        </button>
      ) : <p role="status" tabIndex={-1} data-more-times-status className="text-xs text-[var(--color-wi-text-light)]">No more times to show.</p>}
    </div>
  ) : null;

  const noAvailableTimes = publicForm && availableCount === 0 ? (
    <div className="absence-makeup-empty">
      <p className="font-semibold">{options.length ? "None of these times are available" : "No make-up times available"}</p>
      <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">
        {moreTimes?.hasMore ? "Show more times to look for another class." : emptyDescription ?? "A make-up class is required for this date. Contact Warwick for help before continuing."}
      </p>
      {!moreTimes?.hasMore && (supportContact?.phone || supportContact?.email) ? (
        <div className="absence-makeup-support">
          {supportContact.phone ? <a href={`tel:${supportContact.phone.replace(/[^\d+]/g, "")}`}>Call Warwick</a> : null}
          {supportContact.email ? <a href={`mailto:${encodeURIComponent(supportContact.email)}`}>Email Warwick</a> : null}
        </div>
      ) : null}
    </div>
  ) : null;

  return (
    <div id={`${id}-field`} className="absence-makeup-picker space-y-2" data-selected={Boolean(value)} data-invalid={selectionInvalid || undefined}>
      <label htmlFor={id} className="block text-xs font-semibold text-[var(--color-wi-text-light)]">{label}</label>
      {publicForm ? <select
        id={id}
        name={id}
        value={value}
        disabled={disabled}
        aria-invalid={Boolean(error) || selectionInvalid}
        aria-describedby={[`${id}-context`, error ? `${id}-error` : "", selectionInvalid ? `${id}-invalid` : ""].filter(Boolean).join(" ")}
        onChange={(event) => onChange(event.target.value)}
        className="absence-makeup-native hidden min-h-12 w-full rounded-xl border border-[var(--color-wi-border)] bg-white px-3 text-base text-[var(--color-wi-text)] sm:block"
      >
        {value && !selectedOption ? <option value={value} disabled>{selectedLines?.primary ?? "Previous choice"} — no longer available</option> : null}
        {selectOptions.map((option) => <option key={option.value} value={option.value} disabled={"disabled" in option && option.disabled}>{option.label}</option>)}
      </select> : <SearchableSelect
        id={id}
        name={id}
        value={value}
        options={selectOptions}
        disabled={disabled || options.length === 0}
        error={Boolean(error)}
        aria-describedby={[`${id}-context`, error ? `${id}-error` : ""].filter(Boolean).join(" ")}
        onChange={(event) => onChange(event.target.value)}
        className="hidden min-h-12 w-full rounded-xl border border-[var(--color-wi-border)] bg-white px-3 text-base text-[var(--color-wi-text)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20 sm:block"
      />}

      <button
        data-make-up-trigger="true"
        ref={triggerRef}
        type="button"
        disabled={disabled || (!publicForm && options.length === 0)}
        aria-haspopup="dialog"
        aria-expanded={sheetOpen}
        aria-invalid={Boolean(error) || (publicForm && selectionInvalid)}
        aria-describedby={[`${id}-context`, error ? `${id}-error` : "", publicForm && selectionInvalid ? `${id}-invalid` : ""].filter(Boolean).join(" ")}
        aria-label={selectedLines ? `${label}: ${selectedLines.primary} ${selectedLines.secondary ?? ""}. Change time` : "Choose a make-up class"}
        onClick={() => {
          setPendingValue(value || null);
          setQuery("");
          setSheetOpen(true);
        }}
        className="flex min-h-12 w-full items-center justify-between gap-3 rounded-xl border border-[var(--color-wi-border)] bg-white px-3 py-3 text-left text-base text-[var(--color-wi-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]/30 disabled:cursor-not-allowed disabled:opacity-60 sm:hidden"
      >
        <span className={selectedOption ? "min-w-0 font-medium" : "text-[var(--color-wi-text-light)]"}>
          <span className="block">{selectedLines?.primary ?? "Choose a make-up class"}</span>
          {selectedLines?.secondary ? <span className="mt-1 block text-xs font-normal text-[var(--color-wi-text-light)]">{selectedLines.secondary}</span> : null}
          {publicForm && selectedLines ? <span className="absence-makeup-change">Change time</span> : null}
        </span>
        <ChevronRight className="h-5 w-5 shrink-0 text-[var(--color-wi-text-light)]" aria-hidden="true" />
      </button>
      <span id={`${id}-context`} className="sr-only">{context}</span>
      {error ? <p id={`${id}-error`} className="text-xs text-[var(--color-wi-red)]">{error}</p> : null}
      {publicForm && selectionInvalid ? <p id={`${id}-invalid`} role="status" className="absence-makeup-invalid">{selectedOption?.description ?? "This make-up time is no longer available."} Choose another time.{selectedOption?.conflictDetails ? ` ${selectedOption.conflictDetails}` : ""}</p> : null}
      {publicForm ? <div className="hidden sm:block">{noAvailableTimes}{moreTimesControl}</div> : null}

      {selectedOption?.details ? (
        <p className="hidden text-xs text-[var(--color-wi-text-light)] sm:block">{selectedOption.details}</p>
      ) : null}
      {!publicForm && selectedOption?.description ? (
        <p role="status" className="text-xs text-[var(--color-wi-amber)]">
          {selectedOption.description}
        </p>
      ) : null}
      {!publicForm && selectedOption?.conflictDetails ? (
        <p className="text-xs text-[var(--color-wi-text-light)]">{selectedOption.conflictDetails}</p>
      ) : null}

      <MobileBottomSheet
        open={sheetOpen}
        publicForm={publicForm}
        title="Choose a make-up class"
        description={publicForm ? context : undefined}
        summary={publicForm && selectedLines ? <div className="absence-makeup-current" data-invalid={selectionInvalid || undefined}>
          <span>{selectionInvalid ? "Previous choice · choose another time" : "Current choice"}</span>
          <p>{selectedLines.primary}</p>
        </div> : undefined}
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
              <p role="status" aria-live="polite" className="absence-makeup-confirmation mb-3 text-sm text-[var(--color-wi-text-light)]">
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
              {publicForm && pendingValue === "" ? "Remove make-up selection" : "Confirm make-up class"}
            </button>
          </>
        }
      >
        {noAvailableTimes}
        <fieldset ref={choicesRef} className="space-y-2">
          <legend className="sr-only">{label}</legend>
          {!publicForm ? clearOption : null}
          {filteredOptions.length === 0 && (needle || !publicForm) ? <p className="py-6 text-center text-sm text-[var(--color-wi-text-light)]">No classes match{needle ? ` “${query.trim()}”` : ""}.</p> : null}
          {filteredOptions.map((option) => {
            const lines = optionLines(option, publicForm);
            return (
              <label key={option.value} className="absence-makeup-option flex min-h-14 items-center gap-3 rounded-xl border border-[var(--color-wi-border)] px-4 py-3 has-[:checked]:border-[var(--color-wi-primary)] has-[:checked]:bg-[var(--color-wi-primary)]/5" data-unavailable={option.disabled || undefined}>
                <input
                  type="radio"
                  name={`${id}-mobile`}
                  value={option.value}
                  data-option-index={options.indexOf(option)}
                  checked={pendingValue === option.value}
                  disabled={option.disabled}
                  aria-describedby={option.disabled ? `${id}-option-${options.indexOf(option)}-reason` : undefined}
                  onChange={(event) => setPendingValue(event.target.value)}
                  className="h-5 w-5 shrink-0 border-[var(--color-wi-border)] text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20"
                />
                <span className="min-w-0 break-words text-sm font-medium text-[var(--color-wi-text)]">
                  <span className="block">{lines.primary}</span>
                  {lines.secondary ? <span className="mt-1 block text-xs font-normal text-[var(--color-wi-text-light)]">{lines.secondary}</span> : null}
                  {publicForm && option.value === value && !option.disabled ? <span className="absence-makeup-option__current">Current choice</span> : null}
                  {option.disabled || option.description || option.conflictDetails ? <span id={`${id}-option-${options.indexOf(option)}-reason`} className="absence-makeup-option__reason">
                    {publicForm && option.disabled ? <CircleAlert aria-hidden="true" className="h-4 w-4 shrink-0" /> : null}
                    <span>{option.description ?? (publicForm && option.disabled ? "This make-up class isn’t available." : "")}{option.conflictDetails ? <span className="mt-1 block font-normal">{option.conflictDetails}</span> : null}</span>
                  </span> : null}
                </span>
              </label>
            );
          })}
        </fieldset>
        {publicForm ? <>
          {moreTimesControl}
          {pendingValue !== "" && (value || pendingValue) ? <button type="button" onClick={() => setPendingValue("")} className="absence-makeup-remove">Remove make-up selection</button> : null}
        </> : null}
      </MobileBottomSheet>
    </div>
  );
}
