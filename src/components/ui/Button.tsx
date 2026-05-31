import { type ButtonHTMLAttributes, forwardRef } from "react";

type ButtonVariant = "primary" | "secondary" | "danger" | "ghost";
type ButtonSize = "sm" | "md" | "lg";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
}

const variantClasses: Record<ButtonVariant, string> = {
  primary:
    "bg-[var(--color-wi-primary)] text-white border-transparent hover:bg-[var(--color-wi-primary-dark)] focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]/30",
  secondary:
    "bg-white text-[var(--color-wi-text)] border-gray-300 hover:bg-gray-50 focus-visible:ring-2 focus-visible:ring-gray-300",
  danger:
    "bg-[var(--color-wi-red)] text-white border-transparent hover:bg-[var(--color-wi-red-dark)] focus-visible:ring-2 focus-visible:ring-[var(--color-wi-red)]/30",
  ghost:
    "bg-transparent text-[var(--color-wi-text)] border-transparent hover:bg-gray-100 focus-visible:ring-2 focus-visible:ring-gray-300",
};

const sizeClasses: Record<ButtonSize, string> = {
  sm: "px-2 py-1 text-xs min-h-[28px]",
  md: "px-3 py-1.5 text-sm min-h-[34px]",
  lg: "px-4 py-2 text-sm min-h-[40px]",
};

function Spinner() {
  return (
    <svg
      className="animate-spin -ml-1 mr-1.5 h-4 w-4"
      xmlns="http://www.w3.org/2000/svg"
      fill="none"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path
        className="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
      />
    </svg>
  );
}

const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ variant = "primary", size = "md", loading, disabled, className = "", children, ...props }, ref) => {
    return (
      <button
        ref={ref}
        className={`inline-flex items-center justify-center font-medium transition-colors duration-150 border rounded-sm focus-visible:outline-none disabled:opacity-50 disabled:cursor-not-allowed ${variantClasses[variant]} ${sizeClasses[size]} ${className}`}
        disabled={disabled || loading}
        aria-busy={loading}
        type="button"
        {...props}
      >
        {loading && <Spinner />}
        {children}
      </button>
    );
  }
);

Button.displayName = "Button";
export default Button;
export type { ButtonProps, ButtonVariant, ButtonSize };
