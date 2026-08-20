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
  md: "px-2.5 py-1.5 text-sm",
};

const Select = forwardRef<HTMLSelectElement, SelectProps>(
  ({ size = "md", error, placeholder, describedBy, className = "", children, ...props }, ref) => {
    return (
      <select
        ref={ref}
        className={`w-full cursor-pointer rounded-sm border transition-[background-color,border-color,box-shadow,color] duration-150 appearance-none bg-no-repeat pr-8 hover:border-[var(--color-wi-text-light)] focus-visible:outline-none focus:border-[var(--color-wi-primary)] focus:ring-3 focus:ring-[var(--color-wi-primary)]/15 disabled:cursor-not-allowed disabled:bg-wi-bg disabled:opacity-60 motion-reduce:transition-none select-chevron ${
          error
            ? "border-[var(--color-wi-red)] focus:border-[var(--color-wi-red)] focus:ring-[var(--color-wi-red)]/15"
            : "border-[var(--color-wi-line)]"
        } ${sizeClasses[size]} ${className}`}
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