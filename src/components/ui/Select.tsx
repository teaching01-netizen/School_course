import { type SelectHTMLAttributes, forwardRef } from "react";

type SelectSize = "sm" | "md";

interface SelectProps extends Omit<SelectHTMLAttributes<HTMLSelectElement>, "size"> {
  size?: SelectSize;
  error?: boolean;
  placeholder?: string;
  describedBy?: string;
}

const sizeClasses: Record<SelectSize, string> = {
  sm: "px-2 py-1 text-sm",
  md: "px-3 py-2 text-sm",
};

const Select = forwardRef<HTMLSelectElement, SelectProps>(
  ({ size = "md", error, placeholder, describedBy, className = "", children, ...props }, ref) => {
    return (
      <select
        ref={ref}
        className={`w-full rounded-sm border transition-colors duration-150 appearance-none bg-no-repeat pr-8 focus-visible:outline-none focus:border-[var(--color-wi-primary)] focus:ring-3 focus:ring-[var(--color-wi-primary)]/15 ${
          error
            ? "border-[var(--color-wi-red)] focus:border-[var(--color-wi-red)] focus:ring-[var(--color-wi-red)]/15"
            : "border-gray-300"
        } ${sizeClasses[size]} ${className}`}
        style={{
          backgroundImage: `url("data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' fill='none' viewBox='0 0 20 20'%3e%3cpath stroke='%236B7280' stroke-linecap='round' stroke-linejoin='round' stroke-width='1.5' d='M6 8l4 4 4-4'/%3e%3c/svg%3e")`,
          backgroundPosition: `right 0.5rem center`,
          backgroundSize: `1.25rem`,
          backgroundRepeat: "no-repeat",
        }}
        aria-invalid={error}
        aria-describedby={describedBy}
        {...props}
      >
        {placeholder && (
          <option value="" disabled hidden>
            {placeholder}
          </option>
        )}
        {children}
      </select>
    );
  }
);

Select.displayName = "Select";
export default Select;
export type { SelectProps, SelectSize };
