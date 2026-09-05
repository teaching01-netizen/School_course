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

/** One row of the make-up plan: a single missed class-day and its arrangement. */
export type MakeUpPlanView = {
  /** Stable identity of the missed class-day (its first missed session id). */
  sessionKey: string;
  label: string;
  when: string;
  method: "physical" | "zoom" | "teacher_case" | "none";
  options: MakeUpOption[];
  selectedValue: string;
  hasMoreTimes: boolean;
  /** A previously accepted time is no longer available; this row needs a new choice. */
  needsAttention: boolean;
  /** The chosen time overlaps a class the student attends or another make-up. */
  overlapMessage?: string;
};

type MakeUpScreenProps = {
  plans: MakeUpPlanView[];
  /** Session key to bring into view (e.g. an edit arrived from Review). */
  focusSessionKey?: string | null;
  loadingTimes?: boolean;
  zoomDescription?: string;
  notice?: string | null;
  onUseTime: (sessionKey: string, value: string) => void;
  onSeeMoreTimes: (sessionKey: string) => void;
};

type PlanCardProps = {
  plan: MakeUpPlanView;
  isFocused: boolean;
  loadingTimes: boolean;
  zoomDescription: string;
  onUseTime: (sessionKey: string, value: string) => void;
  onSeeMoreTimes: (sessionKey: string) => void;
};

const RECOMMENDED_RING =
  "border-[var(--color-wi-primary)]/50 bg-[var(--color-wi-primary)]/5";

function PlanCard({ plan, isFocused, loadingTimes, zoomDescription, onUseTime, onSeeMoreTimes }: PlanCardProps) {
  const [sheetOpen, setSheetOpen] = useState(false);
  const [pendingValue, setPendingValue] = useState<string | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!sheetOpen) return;
    setSheetOpen(false);
    setPendingValue(null);
  }, [plan.selectedValue, plan.options.length]);

  useEffect(() => {
    if (!isFocused || !rootRef.current) return;
    rootRef.current.scrollIntoView?.({ behavior: "smooth", block: "nearest" });
  }, [isFocused]);

  const recommended = plan.options[0];
  const selected = plan.options.find((option) => option.value === plan.selectedValue) ?? null;
  const preview = selected ?? recommended;
  const hasChosen = Boolean(selected);

  const sheetTitle = "Choose a make-up time";
  const currentSheetOption =
    plan.options.find((option) => option.value === pendingValue)
    ?? selected
    ?? recommended;

  const confirmSheetChoice = () => {
    if (!currentSheetOption) return;
    setSheetOpen(false);
    setPendingValue(null);
    onUseTime(plan.sessionKey, currentSheetOption.value);
  };

  const renderOutcome = () => {
    if (plan.method === "zoom") {
      return (
        <div className="rounded-xl border border-[var(--color-wi-border)] bg-white px-4 py-4">
          <p className="text-[15px] font-semibold text-[var(--color-wi-text)]">Online</p>
          <p className="mt-1 text-[14px] leading-relaxed text-[var(--color-wi-text-light)]">{zoomDescription}</p>
        </div>
      );
    }
    if (plan.method === "none") {
      return (
        <div className="rounded-xl border border-[var(--color-wi-border)] bg-white px-4 py-4">
          <p className="text-[15px] font-semibold text-[var(--color-wi-green-dark)]">No make-up needed ✓</p>
          <p className="mt-1 text-[14px] leading-relaxed text-[var(--color-wi-text-light)]">
            You don&apos;t need to attend another class for this absence. Nothing further needed.
          </p>
        </div>
      );
    }
    if (plan.method === "teacher_case" || (plan.method === "physical" && plan.options.length === 0 && !plan.hasMoreTimes)) {
      return (
        <div className="rounded-xl border border-[var(--color-wi-border)] bg-white px-4 py-4">
          <p className="text-[15px] font-semibold text-[var(--color-wi-amber)]">We&apos;ll arrange it for you</p>
          <p className="mt-1 text-[14px] leading-relaxed text-[var(--color-wi-text-light)]">
            {plan.method === "teacher_case"
              ? "Your teacher will arrange your make-up. Student Services will contact you — nothing else is needed."
              : "No suitable class is open right now. Student Services will contact you to arrange one — nothing else is needed."}
          </p>
        </div>
      );
    }
    if (plan.method === "physical" && plan.options.length === 0 && plan.hasMoreTimes) {
      return (
        <div className="rounded-xl border border-[var(--color-wi-border)] bg-white px-4 py-4">
          <p className="text-[15px] font-semibold text-[var(--color-wi-primary)]">More times available</p>
          <p className="mt-1 text-[14px] leading-relaxed text-[var(--color-wi-text-light)]">
            No times at this level — see more times to check the next available options.
          </p>
        </div>
      );
    }
    return null;
  };

  const renderPhysical = () => {
    if (!preview) return null;
    return (
      <>
        <div
          className={clsx(
            "rounded-xl border bg-white px-4 py-3.5",
            hasChosen ? "border-[var(--color-wi-border)]" : RECOMMENDED_RING,
          )}
        >
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <p className="text-[15px] font-semibold leading-snug text-[var(--color-wi-text)]">{preview.name}</p>
              <p className="mt-0.5 text-[13px] leading-snug text-[var(--color-wi-text)]">{preview.date} · {preview.time}</p>
              {preview.teacher ? <p className="mt-0.5 text-[13px] text-[var(--color-wi-text-light)]">{preview.teacher}</p> : null}
            </div>
            <span
              className={clsx(
                "shrink-0 rounded-full px-2.5 py-0.5 text-[12px] font-semibold",
                hasChosen
                  ? "bg-[var(--color-wi-green)]/10 text-[var(--color-wi-green-dark)]"
                  : "bg-[var(--color-wi-primary)]/10 text-[var(--color-wi-primary-dark)]",
              )}
            >
              {hasChosen ? "Chosen" : "Suggested"}
            </span>
          </div>
        </div>

        <button
          ref={triggerRef}
          type="button"
          onClick={() => {
            setPendingValue(null);
            setSheetOpen(true);
          }}
          aria-haspopup="dialog"
          aria-expanded={sheetOpen}
          className="wi-press mt-2 flex min-h-[48px] w-full items-center justify-center gap-1 rounded-xl border border-[var(--color-wi-border)] bg-white px-4 text-[15px] font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
        >
          {hasChosen ? "Change time" : "Choose a time"}
          <ChevronRight className="h-5 w-5" aria-hidden="true" />
        </button>

        {plan.overlapMessage && hasChosen ? (
          <p role="status" className="mt-2 rounded-lg bg-[var(--color-wi-amber-bg)] px-3 py-2 text-[13px] leading-snug text-[var(--color-wi-amber)]">
            {plan.overlapMessage}
          </p>
        ) : null}

        {plan.needsAttention && !hasChosen ? (
          <p role="status" className="mt-2 rounded-lg bg-[var(--color-wi-amber-bg)] px-3 py-2 text-[13px] leading-snug text-[var(--color-wi-amber)]">
            The time you chose is no longer available — choose another one. Nothing else in your plan changed.
          </p>
        ) : null}

        <MobileBottomSheet
          open={sheetOpen}
          title={sheetTitle}
          onClose={() => {
            setSheetOpen(false);
            setPendingValue(null);
          }}
          restoreFocusRef={triggerRef}
        >
          <p className="mb-3 text-[13px] text-[var(--color-wi-text-light)]">
            {plan.label} · {plan.when}
          </p>
          {loadingTimes ? (
            <p role="status" className="py-8 text-center text-[15px] text-[var(--color-wi-text-light)]">
              Loading times…
            </p>
          ) : (
            <>
              <ul className="space-y-2">
                {plan.options.map((option) => {
                  const isRecommended = recommended ? option.value === recommended.value : false;
                  const isSelected = option.value === (pendingValue ?? plan.selectedValue);
                  return (
                    <li key={option.value}>
                      <button
                        type="button"
                        aria-pressed={isSelected}
                        onClick={() => setPendingValue(option.value)}
                        className={clsx(
                          "wi-press flex min-h-14 w-full items-center gap-3 rounded-xl border px-4 py-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]",
                          isSelected
                            ? "border-[var(--color-wi-primary)]/50 bg-[var(--color-wi-primary)]/5"
                            : "border-[var(--color-wi-border)] bg-white hover:bg-[var(--color-wi-row-alt)]",
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
                          <span className="shrink-0 rounded-full bg-[var(--color-wi-primary)]/10 px-2.5 py-0.5 text-[12px] font-semibold text-[var(--color-wi-primary-dark)]">
                            Recommended
                          </span>
                        ) : null}
                      </button>
                    </li>
                  );
                })}
              </ul>
              {plan.hasMoreTimes ? (
                <button
                  type="button"
                  onClick={() => onSeeMoreTimes(plan.sessionKey)}
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
                  onClick={confirmSheetChoice}
                  disabled={!currentSheetOption}
                  className="wi-press flex min-h-12 flex-[2] items-center justify-center rounded-xl bg-[var(--color-wi-primary)] px-4 text-[15px] font-semibold text-white hover:bg-[var(--color-wi-primary-dark)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2 disabled:opacity-60"
                >
                  Use this time
                </button>
              </div>
              <p className="mt-2 text-center text-[12px] text-[var(--color-wi-text-light)]">
                Choosing here only selects — nothing changes until you tap Use this time.
              </p>
            </>
          )}
        </MobileBottomSheet>
      </>
    );
  };

  const staticOutcome = renderOutcome();

  return (
    <div
      ref={rootRef}
      data-makeup-row={plan.sessionKey}
      className={clsx(
        "rounded-2xl border bg-white p-4 sm:p-5",
        isFocused ? "border-[var(--color-wi-primary)] ring-2 ring-[var(--color-wi-primary)]/20" : "border-[var(--color-wi-border)]",
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-[17px] font-semibold text-[var(--color-wi-text)]">{plan.label}</p>
          <p className="mt-0.5 text-[14px] text-[var(--color-wi-text-light)]">Missing {plan.when}</p>
        </div>
        {plan.method === "none" ? (
          <span className="shrink-0 rounded-full bg-[var(--color-wi-green)]/10 px-2.5 py-0.5 text-[12px] font-semibold text-[var(--color-wi-green-dark)]">
            No make-up needed
          </span>
        ) : plan.method === "zoom" ? (
          <span className="shrink-0 rounded-full bg-[var(--color-wi-primary)]/10 px-2.5 py-0.5 text-[12px] font-semibold text-[var(--color-wi-primary)]">
            Online
          </span>
        ) : null}
      </div>

      <div className="mt-3.5 space-y-2">
        {plan.method === "physical" && plan.options.length > 0 ? (
          <>
            <p className="text-[13px] font-semibold uppercase tracking-[0.08em] text-[var(--color-wi-text-light)]">
              {hasChosen ? "Your make-up" : "Suggested make-up"}
            </p>
            {renderPhysical()}
          </>
        ) : plan.method === "physical" ? (
          <>
            {staticOutcome}
            {plan.hasMoreTimes ? (
              <button
                type="button"
                onClick={() => onSeeMoreTimes(plan.sessionKey)}
                disabled={loadingTimes}
                className="wi-press mt-2 flex min-h-11 w-full items-center justify-center rounded-xl border border-[var(--color-wi-primary)]/40 bg-white px-4 text-[15px] font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary)]/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] disabled:opacity-60"
              >
                {loadingTimes ? (
                  <span className="inline-flex items-center gap-2">
                    <LoaderCircle className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
                    Loading times…
                  </span>
                ) : (
                  "See more times"
                )}
              </button>
            ) : null}
          </>
        ) : (
          staticOutcome
        )}
      </div>
    </div>
  );
}

export default function MakeUpScreen({
  plans,
  focusSessionKey = null,
  loadingTimes = false,
  zoomDescription = "We'll send you the Zoom link before your class.",
  notice = null,
  onUseTime,
  onSeeMoreTimes,
}: MakeUpScreenProps) {
  if (plans.length === 0) return null;
  return (
    <div className="mx-auto w-full max-w-2xl">
      <h1 className="text-[28px] font-bold leading-tight tracking-tight text-[var(--color-wi-text)]">
        Your make-up
      </h1>
      <p className="mt-2 text-[17px] leading-relaxed text-[var(--color-wi-text-light)]">
        Here&apos;s how each class you&apos;ll miss will be made up. Suggestions are proposals — nothing is booked until you choose a time.
      </p>
      {notice ? (
        <div role="status" className="mt-5 rounded-xl border border-[var(--color-wi-amber)]/30 bg-[var(--color-wi-amber-bg)] px-4 py-3 text-[15px] leading-snug text-[var(--color-wi-amber)]">
          {notice}
        </div>
      ) : null}
      <div className="mt-7 space-y-5">
        {plans.map((plan) => (
          <PlanCard
            key={plan.sessionKey}
            plan={plan}
            isFocused={focusSessionKey === plan.sessionKey}
            loadingTimes={loadingTimes}
            zoomDescription={zoomDescription}
            onUseTime={onUseTime}
            onSeeMoreTimes={onSeeMoreTimes}
          />
        ))}
      </div>
    </div>
  );
}
