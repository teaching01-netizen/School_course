import { motion } from "framer-motion";
import clsx from "clsx";
import type { ReactNode } from "react";
import { formatDate, formatTime } from "@/utils/date";
import type { SessionInSubject } from "@/features/absences/types";

export type SessionDayGroup = {
  id: string;
  date: string;
  start_at: string;
  end_at: string;
  items: SessionInSubject[];
};

type SessionDayCardProps = {
  dayGroup: SessionDayGroup;
  selected: boolean;
  alreadyAbsent: boolean;
  disabled: boolean;
  disabledReason?: "absence_limit_reached" | "remaining_days_selected";
  disabledDetail?: string;
  onToggle: () => void;
  reduceMotion?: boolean | null;
  children?: ReactNode;
};

export default function SessionDayCard({ dayGroup, selected, alreadyAbsent, disabled, disabledReason, disabledDetail, onToggle, reduceMotion = false, children }: SessionDayCardProps) {
  return (
    <div data-selected={selected} className={clsx(
      "absence-session-day rounded-xl border px-4 py-3 transition-colors motion-reduce:transition-none",
      selected ? "border-[var(--color-wi-primary)]/40 bg-[var(--color-wi-primary)]/5" : "border-[var(--color-wi-border)] bg-white",
      disabled && !selected && "opacity-70",
    )}>
      <label htmlFor={`session-${dayGroup.id}`} className={clsx("absence-session-day__choice flex min-h-12 items-center gap-3", alreadyAbsent || disabled ? "cursor-not-allowed" : "cursor-pointer")}>
        <input
          type="checkbox"
          id={`session-${dayGroup.id}`}
          name={`session-${dayGroup.id}`}
          checked={selected}
          disabled={alreadyAbsent || disabled}
          onChange={onToggle}
          className="h-5 w-5 shrink-0 rounded border-[var(--color-wi-border)] text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20 disabled:cursor-not-allowed disabled:opacity-50"
        />
        <span className="absence-session-day__label min-w-0 flex-1">
          <span className="block break-words text-sm font-semibold leading-5 text-[var(--color-wi-text)]">
            <span className="absence-session-day__date">{formatDate(dayGroup.date)}</span>{" "}
            <span className="absence-session-day__time">{formatTime(dayGroup.start_at)}-{formatTime(dayGroup.end_at)}</span>
          </span>
          {alreadyAbsent ? <span className="mt-0.5 block text-xs font-medium text-[var(--color-wi-text-light)]">Absence already submitted</span> : null}
          {disabled && !alreadyAbsent && !selected && disabledReason === "absence_limit_reached" ? (
            <span className="mt-0.5 block text-xs text-[var(--color-wi-text-light)]">Can't select — absence limit reached</span>
          ) : null}
          {disabled && !alreadyAbsent && !selected && disabledReason === "remaining_days_selected" ? (
            <span className="mt-0.5 block text-xs text-[var(--color-wi-text-light)]">All remaining days selected</span>
          ) : null}
          {disabled && !alreadyAbsent && !selected && disabledDetail ? (
            <span className="mt-0.5 block text-xs text-[var(--color-wi-text-light)]">{disabledDetail}</span>
          ) : null}
        </span>
      </label>
      {selected && children ? (
        <motion.div
          initial={reduceMotion ? false : { opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ duration: reduceMotion ? 0 : 0.16, ease: "easeOut" }}
          className="absence-session-makeup mt-3 border-t border-[var(--color-wi-border)] pt-3 sm:ml-8"
        >
          {children}
        </motion.div>
      ) : null}
    </div>
  );
}
