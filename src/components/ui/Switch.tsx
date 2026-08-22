import { cn } from "@/utils/cn";

type SwitchProps = {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  /** Required: a switch must announce what it controls to screen readers. */
  "aria-label": string;
  disabled?: boolean;
  className?: string;
};

/** Compact inline switch for boolean values in dense rows (property panels,
 *  table cells). Button-based so it is natively keyboard-operable; the visual
 *  track + knob are purely decorative. */
export function Switch({ checked, onCheckedChange, disabled = false, className, "aria-label": ariaLabel }: SwitchProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={() => onCheckedChange(!checked)}
      className={cn(
        "relative inline-flex h-[18px] w-8 shrink-0 cursor-pointer items-center rounded-full transition-colors duration-150 focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] disabled:cursor-not-allowed disabled:opacity-60 motion-reduce:transition-none",
        checked ? "bg-[var(--color-wi-primary)]" : "bg-[var(--color-wi-row-alt)] ring-1 ring-inset ring-[var(--color-wi-line)]",
        className,
      )}
    >
      <span
        aria-hidden="true"
        className={cn(
          "pointer-events-none inline-block h-3.5 w-3.5 rounded-full bg-white shadow-sm transition-transform duration-150 motion-reduce:transition-none",
          checked ? "translate-x-[15px]" : "translate-x-[2.5px]",
        )}
      />
    </button>
  );
}
