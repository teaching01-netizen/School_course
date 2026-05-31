import type { ChangeEvent } from "react";
import { format, addDays, isBefore, isAfter, startOfDay } from "date-fns";
import { Trash2 } from "lucide-react";
import Button from "@/components/ui/Button";

interface DateRangeSlotProps {
  index: number;
  fromDate: Date | undefined;
  toDate: Date | undefined;
  onFromChange: (date: Date | undefined) => void;
  onToChange: (date: Date | undefined) => void;
  onRemove: () => void;
  canRemove: boolean;
  maxDays: number;
}

function formatDisplayDate(date: Date | undefined): string {
  if (!date) return "Pick a date";
  return format(date, "EEE, MMM d");
}

function formatInputDate(date: Date | undefined): string {
  if (!date) return "";
  return format(date, "yyyy-MM-dd");
}

function parseInputDate(value: string): Date | undefined {
  if (!value) return undefined;
  const [year, month, day] = value.split("-").map(Number);
  if (!year || !month || !day) return undefined;
  return startOfDay(new Date(year, month - 1, day));
}

export default function DateRangeSlot({
  index,
  fromDate,
  toDate,
  onFromChange,
  onToChange,
  onRemove,
  canRemove,
  maxDays,
}: DateRangeSlotProps) {
  const today = startOfDay(new Date());
  const maxDate = addDays(today, maxDays);

  const fromMin = today;
  const fromMax = toDate && isBefore(toDate, maxDate) ? toDate : maxDate;
  const toMin = fromDate && isAfter(fromDate, today) ? fromDate : today;
  const toMax = maxDate;

  function handleDateChange(onChange: (date: Date | undefined) => void) {
    return (event: ChangeEvent<HTMLInputElement>) => {
      onChange(parseInputDate(event.target.value));
    };
  }

  const dateInputClass =
    "min-h-[44px] w-full rounded-lg border border-gray-200 bg-gray-50/50 px-3.5 py-2 text-sm text-[var(--color-wi-text)] transition-colors hover:bg-gray-100 focus:border-[var(--color-wi-blue)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-blue)]/15 sm:min-w-[160px]";

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-gray-200 bg-white px-4 py-3 shadow-sm sm:flex-row sm:items-center">
      {/* From date picker */}
      <div className="relative flex w-full flex-col items-stretch gap-1 sm:w-auto">
        <span className="text-xs font-medium text-[var(--color-wi-text-light)] sm:hidden">From</span>
        <input
          type="date"
          value={formatInputDate(fromDate)}
          min={formatInputDate(fromMin)}
          max={formatInputDate(fromMax)}
          onChange={handleDateChange(onFromChange)}
          className={dateInputClass}
          aria-label={`From date: ${formatDisplayDate(fromDate)}`}
        />
      </div>

      {/* Separator */}
      <span className="hidden sm:inline-flex items-center justify-center h-7 w-7 rounded-full bg-gray-100 text-[var(--color-wi-text-light)] text-sm" aria-hidden="true">
        →
      </span>

      {/* To date picker */}
      <div className="relative flex w-full flex-col items-stretch gap-1 sm:w-auto">
        <span className="text-xs font-medium text-[var(--color-wi-text-light)] sm:hidden">To</span>
        <input
          type="date"
          value={formatInputDate(toDate)}
          min={formatInputDate(toMin)}
          max={formatInputDate(toMax)}
          onChange={handleDateChange(onToChange)}
          className={dateInputClass}
          aria-label={`To date: ${formatDisplayDate(toDate)}`}
        />
      </div>

      {/* Remove button */}
      {canRemove && (
        <Button
          variant="ghost"
          size="sm"
          onClick={onRemove}
          aria-label={`Remove date range ${index + 1}`}
          className="ml-auto shrink-0 text-gray-400 hover:text-[var(--color-wi-red)] hover:bg-red-50 rounded-lg transition-colors"
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      )}

    </div>
  );
}
