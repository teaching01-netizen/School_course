import { Check } from "lucide-react";
import { motion, useReducedMotion } from "framer-motion";
import clsx from "clsx";

type CourseChipProps = {
  id?: string;
  name: string;
  code: string;
  selected: boolean;
  onToggle: () => void;
  disabled?: boolean;
  tabIndex?: number;
};

export default function CourseChip({
  id,
  name,
  code,
  selected,
  onToggle,
  disabled = false,
  tabIndex = -1,
}: CourseChipProps) {
  const reduceMotion = useReducedMotion();

  return (
    <motion.button
      id={id}
      type="button"
      role="option"
      aria-selected={selected}
      tabIndex={tabIndex}
      disabled={disabled}
      onClick={onToggle}
      whileHover={reduceMotion ? undefined : { scale: 1.02 }}
      whileTap={reduceMotion ? undefined : { scale: 0.95 }}
      transition={{ type: "spring", stiffness: 400, damping: 25 }}
      className={clsx(
        "flex min-h-[44px] min-w-[44px] items-start gap-3 rounded-sm border px-3 py-2 text-left transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]/20",
        selected
          ? "border-[var(--color-wi-primary)] bg-[var(--color-wi-primary)]/10 text-[var(--color-wi-text)]"
          : "border-gray-200 bg-white text-[var(--color-wi-text)] hover:bg-gray-50",
        disabled && "cursor-not-allowed opacity-60",
      )}
    >
      <span className="mt-0.5 flex h-5 w-5 items-center justify-center rounded-full border border-current/20">
        {selected ? <Check className="h-4 w-4" aria-hidden="true" /> : null}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-semibold">{name}</span>
        <span className={clsx("block truncate text-xs", selected ? "text-gray-700" : "text-gray-600")}>{code}</span>
      </span>
    </motion.button>
  );
}
