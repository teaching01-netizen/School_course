import { useMemo, useCallback } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { CalendarPlus } from "lucide-react";
import { differenceInDays } from "date-fns";
import DateRangeSlot from "./DateRangeSlot";
import Button from "@/components/ui/Button";

interface DateRange {
  id: string;
  from: Date | undefined;
  to: Date | undefined;
}

interface DateRangePickerProps {
  value: DateRange[];
  onChange: (ranges: DateRange[]) => void;
  maxDays: number;
  error?: string;
}

// ---------- validation helpers ----------

function rangesOverlap(a: DateRange, b: DateRange): boolean {
  if (!a.from || !a.to || !b.from || !b.to) return false;
  return a.from <= b.to && b.from <= a.to;
}

function validateRanges(
  ranges: DateRange[],
  maxDays: number,
): string | undefined {
  for (let i = 0; i < ranges.length; i++) {
    const r = ranges[i];
    if (r.from && r.to && r.from > r.to) {
      return `Range ${i + 1}: start date must be before or equal to end date.`;
    }
  }

  // overlap check
  for (let i = 0; i < ranges.length; i++) {
    for (let j = i + 1; j < ranges.length; j++) {
      if (rangesOverlap(ranges[i], ranges[j])) {
        return `Ranges ${i + 1} and ${j + 1} overlap.`;
      }
    }
  }

  // total days across all complete ranges
  let total = 0;
  for (const r of ranges) {
    if (r.from && r.to) {
      total += differenceInDays(r.to, r.from) + 1; // inclusive
    }
  }
  if (total > maxDays) {
    return `Total days across all ranges (${total}) exceeds the maximum of ${maxDays}.`;
  }

  return undefined;
}

// ---------- animation variants ----------

const slotVariants = {
  initial: { opacity: 0, y: -8 },
  animate: {
    opacity: 1,
    y: 0,
    transition: { duration: 0.2, ease: "easeOut" as const },
  },
  exit: {
    opacity: 0,
    y: -8,
    transition: { duration: 0.15, ease: "easeIn" as const },
  },
};

// ---------- component ----------

export default function DateRangePicker({
  value,
  onChange,
  maxDays,
  error: externalError,
}: DateRangePickerProps) {
  const internalError = useMemo(
    () => validateRanges(value, maxDays),
    [value, maxDays],
  );

  const displayError = externalError ?? internalError;

  // last slot has both dates filled?
  const lastSlotComplete = useMemo(() => {
    const last = value[value.length - 1];
    return last !== undefined && last.from !== undefined && last.to !== undefined;
  }, [value]);

  // ---------- handlers ----------

  const handleFromChange = useCallback(
    (index: number, date: Date | undefined) => {
      const next = value.map((r, i) =>
        i === index ? { ...r, from: date } : r,
      );
      onChange(next);
    },
    [value, onChange],
  );

  const handleToChange = useCallback(
    (index: number, date: Date | undefined) => {
      const next = value.map((r, i) =>
        i === index ? { ...r, to: date } : r,
      );
      onChange(next);
    },
    [value, onChange],
  );

  const handleRemove = useCallback(
    (index: number) => {
      const next = value.filter((_, i) => i !== index);
      onChange(next);
    },
    [value, onChange],
  );

  const handleAdd = useCallback(() => {
    onChange([...value, { id: crypto.randomUUID(), from: undefined, to: undefined }]);
  }, [value, onChange]);

  // ---------- render ----------

  return (
    <div className="flex flex-col" role="group" aria-label="Date ranges">
      {/* Slot list */}
      <AnimatePresence initial={false}>
        {value.map((range, idx) => (
          <motion.div
            key={range.id}
            variants={slotVariants}
            initial="initial"
            animate="animate"
            exit="exit"
            className="overflow-visible"
          >
            <DateRangeSlot
              index={idx}
              fromDate={range.from}
              toDate={range.to}
              onFromChange={(date) => handleFromChange(idx, date)}
              onToChange={(date) => handleToChange(idx, date)}
              onRemove={() => handleRemove(idx)}
              canRemove={value.length > 1}
              maxDays={maxDays}
            />
          </motion.div>
        ))}
      </AnimatePresence>

      {/* "+ Add another date range" CTA */}
      <div className="flex justify-center mt-3">
        <Button
          variant="secondary"
          size="sm"
          onClick={handleAdd}
          disabled={!lastSlotComplete}
          className="rounded-lg border-dashed border-2 border-gray-300 px-4 hover:border-[var(--color-wi-primary)] hover:text-[var(--color-wi-primary)] hover:bg-blue-50/50 transition-all duration-150"
        >
          <CalendarPlus className="h-4 w-4 mr-1.5" />
          Add more dates
        </Button>
      </div>

      {/* Error display */}
      {displayError && (
        <p role="alert" className="mt-2 text-sm text-[var(--color-wi-red)] font-medium bg-red-50 border border-red-200 rounded-lg px-3 py-2">{displayError}</p>
      )}
    </div>
  );
}

export type { DateRange, DateRangePickerProps };
