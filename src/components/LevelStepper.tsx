import { Minus, Plus } from "lucide-react";

type LevelStepperProps = {
  value: number | null;
  onChange: (v: number | null) => void;
  min?: number;
};

export default function LevelStepper({
  value,
  onChange,
  min = 1,
}: LevelStepperProps) {
  const handleDecrease = () => {
    if (value === null) {
      onChange(min);
    } else if (value > min) {
      onChange(value - 1);
    }
  };

  const handleIncrease = () => {
    if (value === null) {
      onChange(min);
    } else {
      onChange(value + 1);
    }
  };

  return (
    <div className="flex items-center gap-1">
      <button
        type="button"
        onClick={handleDecrease}
        className="flex h-6 w-6 items-center justify-center rounded-sm border border-wi-line text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)]"
        aria-label="Decrease level"
      >
        <Minus className="h-3 w-3" />
      </button>
      <span className="flex h-6 min-w-[24px] items-center justify-center text-sm font-semibold text-[var(--color-wi-text)]">
        {value ?? <span className="text-[var(--color-wi-text-light)]">&mdash;</span>}
      </span>
      <button
        type="button"
        onClick={handleIncrease}
        className="flex h-6 w-6 items-center justify-center rounded-sm border border-wi-line text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)]"
        aria-label="Increase level"
      >
        <Plus className="h-3 w-3" />
      </button>
    </div>
  );
}
