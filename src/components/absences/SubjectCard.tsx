import clsx from "clsx";

type SubjectCardProps = {
  id: string;
  name: string;
  code: string;
  selected: boolean;
  onToggle: () => void;
  disabled?: boolean;
};

export default function SubjectCard({
  id,
  name,
  code,
  selected,
  onToggle,
  disabled = false,
}: SubjectCardProps) {
  return (
    <label
      htmlFor={`subject-${id}`}
      className={clsx(
        "flex min-h-[56px] w-full cursor-pointer items-center gap-4 px-4 py-3 transition-all duration-150",
        !disabled && selected && "bg-[var(--color-wi-primary)]/5",
        !disabled && !selected && "hover:bg-gray-50",
        disabled && "cursor-not-allowed opacity-60",
      )}
    >
      <div
        className={clsx(
          "flex h-5 w-5 shrink-0 items-center justify-center rounded border-2 transition-colors",
          selected
            ? "border-[var(--color-wi-primary)] bg-[var(--color-wi-primary)]"
            : "border-gray-300 bg-white",
        )}
        aria-hidden="true"
      >
        {selected && (
          <svg className="h-3 w-3 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
          </svg>
        )}
      </div>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-semibold text-[var(--color-wi-text)]">{name}</span>
        <span className="block truncate text-xs text-[var(--color-wi-text-light)]">{code}</span>
      </span>
      {selected && !disabled && (
        <span className="shrink-0 text-xs font-semibold text-[var(--color-wi-primary)]">
          Selected
        </span>
      )}
      <input
        id={`subject-${id}`}
        type="checkbox"
        checked={selected}
        onChange={onToggle}
        disabled={disabled}
        className="sr-only"
      />
    </label>
  );
}
