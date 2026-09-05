import { useEffect, useRef, useState } from "react";
import clsx from "clsx";
import { Check, ChevronRight, LoaderCircle } from "lucide-react";
import MobileBottomSheet from "./MobileBottomSheet";

export type MakeUpOption = {
  value: string;
  name: string;
  date: string;
  time: string;
  teacher?: string;
};

type MakeUpScreenProps = {
  index: number;
  total: number;
  missedName: string;
  missedWhen: string;
  method: "physical" | "zoom" | "teacher_case" | "none";
  options: MakeUpOption[];
  selectedValue?: string;
  hasMoreTimes: boolean;
  loadingTimes?: boolean;
  notice?: string | null;
  zoomDescription?: string;
  onUse: (value: string) => void;
  onSeeMoreTimes: () => void;
};

export default function MakeUpScreen({
  index,
  total,
  missedName,
  missedWhen,
  method,
  options,
  selectedValue = "",
  hasMoreTimes = false,
  loadingTimes = false,
  notice = null,
  zoomDescription = "We'll send you the Zoom link before your class.",
  onUse,
  onSeeMoreTimes,
}: MakeUpScreenProps) {
  const [sheetOpen, setSheetOpen] = useState(false);
  const [pendingValue, setPendingValue] = useState<string | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    setSheetOpen(false);
    setPendingValue(null);
  }, [index, missedName]);

  const recommended = options.find((option) => option.value === selectedValue) ?? options[0];
  const pendingOption = pendingValue ? options.find((option) => option.value === pendingValue) : undefined;
  const currentOption = pendingOption ?? options.find((option) => option.value === selectedValue) ?? recommended;

  const noMakeUp = method === "none";
  const staffArranged = method === "teacher_case" || (method === "physical" && options.length === 0 && !hasMoreTimes);
  const needsMoreTimes = method === "physical" && options.length === 0 && hasMoreTimes;

  const primaryLabel = method === "physical" && options.length > 0
    ? "Continue with this make-up"
    : noMakeUp || staffArranged
      ? "Continue"
      : needsMoreTimes
        ? "See more times"
        : "Use this make-up";

  const handlePrimary = () => {
    if (method === "physical" && options.length > 0) {
      if (currentOption) onUse(currentOption.value);
    } else if (needsMoreTimes) {
      onSeeMoreTimes();
    } else {
      onUse("");
    }
  };

  const handleConfirmSheetChoice = () => {
    if (currentOption) {
      setSheetOpen(false);
      onUse(currentOption.value);
    }
  };

  return (
    <div className="mx-auto w-full max-w-xl">
      <div className="flex items-center justify-between gap-4">
        <h1 className="text-[28px] font-bold leading-tight tracking-tight text-[var(--color-wi-text)]">
          Your make-up
        </h1>
        {total > 1 ? (
          <span className="shrink-0 rounded-full bg-[var(--color-wi-row-alt)] px-3 py-1 text-[13px] font-semibold text-[var(--color-wi-text-light)]" role="status" aria-live="polite" aria-label={`Make-up ${index + 1} of ${total}`}>
            {index + 1} of {total}
          </span>
        ) : null}
      </div>
      {method === "physical" && options.length > 0 ? (
        <p className="mt-2 text-[17px] leading-relaxed text-[var(--color-wi-text-light)]">
          We found the best available option.
        </p>
      ) : staffArranged ? (
        <p className="mt-2 text-[17px] leading-relaxed text-[var(--color-wi-text-light)]">
          We&apos;ll arrange this for you.
        </p>
      ) : null}

      {notice ? (
        <div role="status" className="mt-5 rounded-xl border border-[var(--color-wi-amber)]/30 bg-[var(--color-wi-amber-bg)] px-4 py-3 text-[15px] leading-snug text-[var(--color-wi-amber)]">
          {notice}
        </div>
      ) : null}

      <div className="mt-6 rounded-2xl border border-[var(--color-wi-border)] bg-[var(--color-wi-bg)] px-5 py-3.5">
        <p className="text-[13px] font-semibold uppercase tracking-[0.08em] text-[var(--color-wi-text-light)]">Class you&apos;ll miss</p>
        <p className="mt-1 text-[17px] font-semibold text-[var(--color-wi-text)]">{missedName}</p>
        <p className="text-[15px] text-[var(--color-wi-text-light)]">{missedWhen}</p>
      </div>

      {method === "physical" && options.length > 0 ? (
        <>
          {currentOption ? (
          <div className="mt-4 rounded-2xl border border-[var(--color-wi-border)] bg-white px-5 py-5">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="text-[17px] font-semibold text-[var(--color-wi-text)]">{currentOption.name}</p>
                <p className="mt-0.5 text-[15px] text-[var(--color-wi-text)]">{currentOption.date}</p>
                <p className="text-[15px] text-[var(--color-wi-text)]">{currentOption.time}</p>
                {currentOption.teacher ? (
                  <p className="mt-1 text-[13px] text-[var(--color-wi-text-light)]">{currentOption.teacher}</p>
                ) : null}
              </div>
              <span className="shrink-0 rounded-full bg-[var(--color-wi-primary)]/10 px-3 py-1 text-[13px] font-semibold text-[var(--color-wi-primary)]">
                Recommended
              </span>
            </div>
          </div>
          ) : null}
          {pendingOption ? (
            <p role="status" className="mt-2 text-[13px] text-[var(--color-wi-text-light)]">
              Selected: {pendingOption.name} · {pendingOption.date} · {pendingOption.time}. Tap Continue to keep it.
            </p>
          ) : null}

          <button
            type="button"
            ref={triggerRef}
            onClick={() => setSheetOpen(true)}
            disabled={loadingTimes}
            aria-haspopup="dialog"
            aria-expanded={sheetOpen}
            className="wi-press mt-3 flex min-h-[52px] w-full items-center justify-center gap-1.5 rounded-xl border border-[var(--color-wi-border)] bg-white px-5 text-[17px] font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] disabled:opacity-60"
          >
            Choose another time
            <ChevronRight className="h-5 w-5" aria-hidden="true" />
          </button>

          <MobileBottomSheet
            open={sheetOpen}
            title="Choose another time"
            onClose={() => setSheetOpen(false)}
            restoreFocusRef={triggerRef}
          >
            {loadingTimes ? (
              <p role="status" className="py-8 text-center text-[15px] text-[var(--color-wi-text-light)]">
                Loading times…
              </p>
            ) : (
              <>
                <p className="mb-3 text-[13px] text-[var(--color-wi-text-light)]">
                  {missedName} · {missedWhen}
                </p>
                <ul className="space-y-2">
                  {options.map((option) => {
                    const isRecommended = recommended ? option.value === recommended.value : false;
                    const isSelected = option.value === (pendingValue ?? selectedValue);
                    return (
                      <li key={option.value}>
                        <button
                          type="button"
                          aria-pressed={isSelected}
                          onClick={() => {
                            setPendingValue(option.value);
                          }}
                          className={clsx(
                            "wi-press flex min-h-14 w-full items-center gap-3 rounded-xl border px-4 py-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]",
                            isSelected
                              ? "border-[var(--color-wi-primary)]/50 bg-[var(--color-wi-primary)]/5"
                              : "border-[var(--color-wi-border)] bg-white hover:bg-[var(--color-wi-row-alt)] active:bg-[var(--color-wi-row-alt)]",
                          )}
                        >
                          <span
                            aria-hidden="true"
                            className={clsx(
                              "flex h-6 w-6 shrink-0 items-center justify-center rounded-full border-2",
                              isSelected ? "border-[var(--color-wi-primary)] bg-[var(--color-wi-primary)] text-white" : "border-[var(--color-wi-border)] bg-white",
                            )}
                          >
                            {isSelected ? <Check className="h-4 w-4" strokeWidth={3} /> : null}
                          </span>
                          <span className="min-w-0 flex-1">
                            <span className="block truncate text-[15px] font-semibold text-[var(--color-wi-text)]">{option.name}</span>
                            <span className="block truncate text-[13px] text-[var(--color-wi-text-light)]">
                              {option.date} · {option.time}
                              {option.teacher ? ` · ${option.teacher}` : ""}
                            </span>
                          </span>
                          {isRecommended ? (
                            <span className="shrink-0 rounded-full bg-[var(--color-wi-primary)]/10 px-2.5 py-0.5 text-[12px] font-semibold text-[var(--color-wi-primary)]">
                              Recommended
                            </span>
                          ) : null}
                        </button>
                      </li>
                    );
                  })}
                </ul>
                {hasMoreTimes ? (
                  <button
                    type="button"
                    onClick={onSeeMoreTimes}
                    disabled={loadingTimes}
                    className="wi-press mt-3 flex min-h-12 w-full items-center justify-center gap-1.5 rounded-xl border border-[var(--color-wi-border)] px-4 text-[15px] font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] disabled:opacity-60"
                  >
                    {loadingTimes ? "Loading…" : "See more times"}
                  </button>
                ) : null}
                <div className="mt-3 flex gap-2">
                  <button
                    type="button"
                    onClick={() => {
                      setPendingValue(null);
                      setSheetOpen(false);
                    }}
                    className="wi-press flex min-h-12 flex-1 items-center justify-center rounded-xl border border-[var(--color-wi-border)] px-4 text-[15px] font-semibold text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                  >
                    Cancel
                  </button>
                  <button
                    type="button"
                    onClick={handleConfirmSheetChoice}
                    disabled={!currentOption || loadingTimes}
                    className="wi-press flex min-h-12 flex-[2] items-center justify-center rounded-xl bg-[var(--color-wi-primary)] px-4 text-[15px] font-semibold text-white hover:bg-[var(--color-wi-primary-dark)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2 disabled:opacity-60"
                  >
                    Confirm make-up
                  </button>
                </div>
                <p className="mt-2 text-center text-[12px] text-[var(--color-wi-text-light)]">
                  Choosing here only selects — nothing changes until you tap Confirm.
                </p>
              </>
            )}
          </MobileBottomSheet>
        </>
      ) : (
        <div className="mt-4 rounded-2xl border border-[var(--color-wi-border)] bg-white px-5 py-5">
          {method === "zoom" ? (
            <>
              <p className="text-[17px] font-semibold text-[var(--color-wi-text)]">Online</p>
              <p className="mt-1 text-[15px] leading-relaxed text-[var(--color-wi-text-light)]">{zoomDescription}</p>
            </>
          ) : noMakeUp ? (
            <>
              <p className="text-[17px] font-semibold text-[var(--color-wi-green-dark)]">No make-up needed ✓</p>
              <p className="mt-1 text-[15px] leading-relaxed text-[var(--color-wi-text-light)]">
                You don&apos;t need to attend another class for this absence. Nothing further needed.
              </p>
            </>
          ) : needsMoreTimes ? (
            <>
              <p className="text-[17px] font-semibold text-[var(--color-wi-primary)]">More times available</p>
              <p className="mt-1 text-[15px] leading-relaxed text-[var(--color-wi-text-light)]">
                No times at this level — tap See more times to check the next available options.
              </p>
            </>
          ) : (
            <>
              <p className="text-[17px] font-semibold text-[var(--color-wi-amber)]">We&apos;ll arrange it for you</p>
              <p className="mt-1 text-[15px] leading-relaxed text-[var(--color-wi-text-light)]">
                {method === "teacher_case"
                  ? "Your teacher will arrange your make-up. Student Services will contact you — nothing else is needed."
                  : "No suitable class is open right now. Student Services will contact you to arrange one — nothing else is needed."}
              </p>
            </>
          )}
        </div>
      )}

      <button
        type="button"
        onClick={handlePrimary}
        disabled={loadingTimes}
        className="wi-press mt-8 flex h-[52px] w-full items-center justify-center rounded-xl bg-[var(--color-wi-primary)] px-5 text-[17px] font-semibold text-white hover:bg-[var(--color-wi-primary-dark)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2 disabled:opacity-60"
      >
        {loadingTimes ? (
          <span role="status" className="inline-flex items-center justify-center gap-2">
            <LoaderCircle className="h-5 w-5 animate-spin motion-reduce:animate-none" aria-hidden="true" />
            Loading times…
          </span>
        ) : (
          primaryLabel
        )}
      </button>
    </div>
  );
}