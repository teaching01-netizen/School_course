import clsx from "clsx";
import { Check } from "lucide-react";

type SubjectRowProps = {
  id: string;
  name: string;
  selected: boolean;
  onToggle: () => void;
  disabled?: boolean;
  detail?: string;
};

export default function SubjectRow({ id, name, selected, onToggle, disabled = false, detail }: SubjectRowProps) {
  return (
    <label
      className={clsx(
        "relative flex min-h-[56px] w-full items-center gap-4 px-4 py-3 text-left transition-colors motion-reduce:transition-none",
        disabled ? "cursor-not-allowed opacity-60" : "cursor-pointer hover:bg-[var(--color-wi-row-alt)]",
        selected && !disabled && "bg-[var(--color-wi-primary)]/5",
        "has-[:focus-visible]:outline has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-[-2px] has-[:focus-visible]:outline-[var(--color-wi-primary)]",
      )}
    >
      <span
        aria-hidden="true"
        className={clsx(
          "flex h-6 w-6 shrink-0 items-center justify-center rounded-md border-2 transition-colors motion-reduce:transition-none",
          selected ? "border-[var(--color-wi-primary)] bg-[var(--color-wi-primary)] text-white" : "border-[var(--color-wi-line)] bg-white",
        )}
      >
        {selected ? <Check className="h-4 w-4" strokeWidth={3} /> : null}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block break-words text-sm font-semibold leading-5 text-[var(--color-wi-text)]">{name}</span>
        {detail ? <span className="mt-0.5 block text-xs text-[var(--color-wi-text-light)]">{detail}</span> : null}
      </span>
      {selected && !disabled ? (
        <span className="shrink-0 text-xs font-semibold text-[var(--color-wi-primary)]">Selected</span>
      ) : null}
      <input
        id={`subject-${id}`}
        name={`subject-${id}`}
        type="checkbox"
        checked={selected}
        onChange={onToggle}
        disabled={disabled}
        className="absolute inset-0 h-full w-full cursor-pointer opacity-0"
      />
    </label>
  );
}
