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
  onToggle: () => void;
  reduceMotion?: boolean | null;
  children?: ReactNode;
};

export default function SessionDayCard({ dayGroup, selected, alreadyAbsent, disabled, onToggle, reduceMotion = false, children }: SessionDayCardProps) {
  return (
    <div className={clsx(
      "rounded-xl border px-4 py-3 transition-colors motion-reduce:transition-none",
      selected ? "border-[var(--color-wi-primary)]/40 bg-[var(--color-wi-primary)]/5" : "border-[var(--color-wi-border)] bg-white",
      disabled && !selected && "opacity-70",
    )}>
      <div className="flex min-h-12 items-center gap-3">
        <input
          type="checkbox"
          id={`session-${dayGroup.id}`}
          name={`session-${dayGroup.id}`}
          checked={selected}
          disabled={alreadyAbsent || disabled}
          onChange={onToggle}
          className="h-5 w-5 shrink-0 rounded border-[var(--color-wi-border)] text-[var(--color-wi-primary)] focus:ring-[var(--color-wi-primary)]/20 disabled:cursor-not-allowed disabled:opacity-50"
        />
        <label htmlFor={`session-${dayGroup.id}`} className={clsx("min-w-0 flex-1 cursor-pointer", (alreadyAbsent || disabled) && "cursor-not-allowed")}>
          <span className="block break-words text-sm font-semibold leading-5 text-[var(--color-wi-text)]">
            {formatDate(dayGroup.date)} {formatTime(dayGroup.start_at)}-{formatTime(dayGroup.end_at)}
          </span>
          {alreadyAbsent ? <span className="mt-0.5 block text-xs font-medium text-[var(--color-wi-text-light)]">Already reported</span> : null}
          {disabled && !alreadyAbsent && !selected ? <span className="mt-0.5 block text-xs text-[var(--color-wi-text-light)]">No more days available</span> : null}
        </label>
      </div>
      {selected && children ? (
        <motion.div
          initial={reduceMotion ? false : { opacity: 0, scale: 0.98 }}
          animate={{ opacity: 1, scale: 1 }}
          transition={reduceMotion ? { duration: 0 } : undefined}
          className="mt-3 border-t border-[var(--color-wi-border)] pt-3 sm:ml-8"
        >
          {children}
        </motion.div>
      ) : null}
    </div>
  );
}
