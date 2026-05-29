import type { ReactNode } from "react";
import { cloneElement, isValidElement } from "react";

interface FormFieldProps {
  name: string;
  label: string;
  required?: boolean;
  error?: string;
  touched?: boolean;
  hint?: string;
  children: ReactNode;
  className?: string;
}

export default function FormField({ name, label, required, error, touched, hint, children, className = "" }: FormFieldProps) {
  const fieldId = `field-${name}`;
  const errorId = `${fieldId}-error`;
  const showError = touched && error;

  const child = isValidElement(children)
    ? cloneElement(children as React.ReactElement<{ id?: string; "aria-invalid"?: boolean; "aria-describedby"?: string }>, {
        id: fieldId,
        "aria-invalid": showError ? true : undefined,
        "aria-describedby": showError ? errorId : undefined,
      })
    : children;

  return (
    <div className={className}>
      <label htmlFor={fieldId} className="block text-sm font-medium text-gray-600 mb-1.5">
        {label}
        {required && <span className="text-[var(--color-wi-red)] ml-0.5" aria-hidden="true">*</span>}
      </label>
      {child}
      {showError && (
        <p id={errorId} role="alert" className="mt-1 text-xs text-[var(--color-wi-red)] flex items-center gap-1">
          {error}
        </p>
      )}
      {!showError && hint && (
        <p data-testid={`field-hint-${name}`} className="mt-1 text-xs text-red-600 flex items-center gap-1">
          {hint}
        </p>
      )}
      {!showError && !hint && <div className="mt-1 h-[1px]" />}
    </div>
  );
}
