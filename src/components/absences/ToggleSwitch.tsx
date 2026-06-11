import clsx from "clsx";

type ToggleSwitchProps = {
  checked: boolean;
  onChange: () => void;
  label: string;
  description?: string;
  disabled?: boolean;
  id: string;
};

export default function ToggleSwitch({
  checked,
  onChange,
  label,
  description,
  disabled = false,
  id,
}: ToggleSwitchProps) {
  return (
    <label
      role="switch"
      aria-checked={checked}
      htmlFor={id}
      className={clsx(
        "flex min-h-[52px] w-full cursor-pointer items-center justify-between gap-3 px-4 py-2.5 transition-colors",
        !disabled && "hover:bg-gray-50",
        disabled && "cursor-not-allowed opacity-60",
      )}
    >
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-semibold text-gray-900">{label}</span>
        {description && (
          <span className="block truncate text-xs text-gray-600">{description}</span>
        )}
      </span>
      <div
        className={clsx(
          "relative inline-flex h-6 w-10 shrink-0 rounded-full transition-colors",
          checked ? "bg-blue-600" : "bg-gray-200",
        )}
      >
        <div
          className={clsx(
            "mt-0.5 inline-block h-5 w-5 rounded-full bg-white shadow-sm ring-0 transition-transform",
            checked ? "translate-x-[18px]" : "translate-x-0.5",
          )}
        />
      </div>
      <input
        id={id}
        type="checkbox"
        checked={checked}
        onChange={onChange}
        disabled={disabled}
        className="sr-only"
      />
    </label>
  );
}
