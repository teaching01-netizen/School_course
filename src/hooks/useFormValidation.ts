import { useState, useCallback, useMemo } from "react";

type RuleType = "required" | "minLength" | "maxLength" | "pattern" | "min" | "max" | "custom";

interface ValidationRule {
  type: RuleType;
  value?: number | string | RegExp;
  message: string;
  validate?: (value: unknown, formValues: Record<string, unknown>) => string | null;
}

type ValidationSchema<T> = {
  [K in keyof T]?: ValidationRule[];
};

interface UseFormValidationReturn<T> {
  errors: Record<string, string>;
  validate: (field: keyof T) => boolean;
  validateAll: () => boolean;
  isValid: boolean;
  touched: Record<string, boolean>;
  touch: (field: keyof T) => void;
  touchAll: () => void;
  clearErrors: () => void;
  clearFieldError: (field: keyof T) => void;
  setErrors: (errors: Record<string, string>) => void;
}

function evaluateRule(value: unknown, rule: ValidationRule, formValues: Record<string, unknown>): string | null {
  const strVal = typeof value === "string" ? value : "";
  const numVal = typeof value === "number" ? value : NaN;

  switch (rule.type) {
    case "required":
      if (value === undefined || value === null) return rule.message;
      if (typeof value === "string" && value.trim() === "") return rule.message;
      return null;
    case "minLength":
      if (strVal.length < (rule.value as number)) return rule.message;
      return null;
    case "maxLength":
      if (strVal.length > (rule.value as number)) return rule.message;
      return null;
    case "pattern":
      if (rule.value instanceof RegExp && !rule.value.test(strVal)) return rule.message;
      return null;
    case "min":
      if (isNaN(numVal) || numVal < (rule.value as number)) return rule.message;
      return null;
    case "max":
      if (isNaN(numVal) || numVal > (rule.value as number)) return rule.message;
      return null;
    case "custom":
      if (rule.validate) return rule.validate(value, formValues);
      return null;
    default:
      return null;
  }
}

export function useFormValidation<T extends Record<string, unknown>>(
  schema: ValidationSchema<T>,
  formValues: T
): UseFormValidationReturn<T> {
  const [errors, setErrorsState] = useState<Record<string, string>>({});
  const [touched, setTouched] = useState<Record<string, boolean>>({});

  const validate = useCallback(
    (field: keyof T): boolean => {
      const rules = schema[field];
      if (!rules) return true;
      const value = formValues[field];
      const formVals = formValues as Record<string, unknown>;
      for (const rule of rules) {
        if (rule.type !== "required" && (value === undefined || value === null || value === "")) continue;
        const error = evaluateRule(value, rule, formVals);
        if (error) {
          setErrorsState((prev) => ({ ...prev, [field as string]: error }));
          return false;
        }
      }
      setErrorsState((prev) => {
        const next = { ...prev };
        delete next[field as string];
        return next;
      });
      return true;
    },
    [schema, formValues]
  );

  const validateAll = useCallback((): boolean => {
    const fields = Object.keys(schema) as (keyof T)[];
    let allValid = true;
    for (const field of fields) {
      if (!validate(field)) allValid = false;
    }
    setTouched((prev) => {
      const next = { ...prev };
      for (const field of fields) next[field as string] = true;
      return next;
    });
    return allValid;
  }, [schema, validate]);

  const isValid = useMemo(() => Object.keys(errors).length === 0, [errors]);

  const touch = useCallback((field: keyof T) => {
    setTouched((prev) => ({ ...prev, [field as string]: true }));
  }, []);

  const touchAll = useCallback(() => {
    const fields = Object.keys(schema) as (keyof T)[];
    setTouched((prev) => {
      const next = { ...prev };
      for (const field of fields) next[field as string] = true;
      return next;
    });
  }, [schema]);

  const clearErrors = useCallback(() => setErrorsState({}), []);
  const clearFieldError = useCallback((field: keyof T) => {
    setErrorsState((prev) => {
      const next = { ...prev };
      delete next[field as string];
      return next;
    });
  }, []);
  const setErrors = useCallback((newErrors: Record<string, string>) => {
    setErrorsState((prev) => ({ ...prev, ...newErrors }));
  }, []);

  return { errors, validate, validateAll, isValid, touched, touch, touchAll, clearErrors, clearFieldError, setErrors };
}
