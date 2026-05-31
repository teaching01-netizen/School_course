import { motion, useReducedMotion } from "framer-motion";
import clsx from "clsx";
import { formatDate } from "@/utils/date";

type SessionChipProps = {
  id: string;
  date: string;
  startTime: string;
  endTime: string;
  selected: boolean;
  alreadyAbsent: boolean;
  disabled?: boolean;
  onToggle: (id: string) => void;
  subjectCode?: string;
};

export default function SessionChip({
  id,
  date,
  startTime,
  endTime,
  selected,
  alreadyAbsent,
  disabled = false,
  onToggle,
  subjectCode,
}: SessionChipProps) {
  const reduceMotion = useReducedMotion();
  const isDisabled = alreadyAbsent || disabled;
  const ariaLabel = `${date} ${startTime}-${endTime} ${subjectCode ?? ""}`;

  const handleClick = () => {
    if (!isDisabled) {
      onToggle(id);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if ((e.key === "Enter" || e.key === " ") && !isDisabled) {
      e.preventDefault();
      onToggle(id);
    }
  };

  return (
    <motion.button
      type="button"
      role="checkbox"
      aria-checked={selected}
      aria-disabled={isDisabled}
      aria-label={ariaLabel}
      tabIndex={0}
      whileTap={reduceMotion ? undefined : { scale: 0.97 }}
      transition={{ type: "spring", stiffness: 400, damping: 25 }}
      className={clsx(
        "max-sm:w-full inline-flex items-center gap-1 max-sm:rounded-md rounded-full border px-3 py-2 text-sm font-medium transition-colors min-h-[44px]",
        isDisabled && "cursor-not-allowed",
        !isDisabled && selected && "max-sm:border-l-4 max-sm:border-l-green-500 max-sm:bg-green-50 bg-green-100 border-green-300 text-green-800",
        !isDisabled && !selected && "max-sm:border-l-4 max-sm:border-l-gray-300 max-sm:bg-gray-50 bg-gray-100 border-gray-300 text-gray-600",
        isDisabled && "max-sm:border-l-4 max-sm:border-l-gray-200 max-sm:bg-gray-25 bg-gray-50 border-gray-200 text-gray-400",
      )}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
    >
      {alreadyAbsent && (
        <span aria-hidden="true" className="text-xs">⊘</span>
      )}
      {selected && !alreadyAbsent && (
        <span aria-hidden="true" className="text-xs">✓</span>
      )}
      <span>
        {formatDate(date)} {startTime}–{endTime}
      </span>
    </motion.button>
  );
}
