import { type InputHTMLAttributes, forwardRef } from "react";

type InputSize = "sm" | "md";

interface InputProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "size"> {
  size?: InputSize;
  error?: boolean;
  describedBy?: string;
}

const sizeClasses: Record<InputSize, string> = {
  sm: "px-2 py-1 text-sm",
  md: "px-2.5 py-1.5 text-sm",
};

const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ size = "md", error, describedBy, className = "", ...props }, ref) => {
    return (
      <input
        ref={ref}
        className={`w-full rounded-sm border transition-colors duration-150 placeholder:text-[var(--color-wi-faint)] focus-visible:outline-none focus:border-[var(--color-wi-primary)] focus:ring-3 focus:ring-[var(--color-wi-primary)]/15 ${
          error
            ? "border-[var(--color-wi-red)] focus:border-[var(--color-wi-red)] focus:ring-[var(--color-wi-red)]/15"
            : "border-[var(--color-wi-line)]"
        } ${sizeClasses[size]} ${className}`}
        aria-invalid={error}
        aria-describedby={describedBy}
        {...props}
      />
    );
  }
);

Input.displayName = "Input";
export default Input;
export type { InputProps, InputSize };