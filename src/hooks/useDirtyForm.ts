import { useState, useEffect, useCallback, useMemo } from "react";

interface UseDirtyFormOptions {
  warnBeforeUnload?: boolean;
}

interface UseDirtyFormReturn<T> {
  isDirty: boolean;
  dirtyFields: Record<string, boolean>;
  setInitialState: (values: T) => void;
  markPristine: (field: keyof T) => void;
  reset: () => void;
  initialValues: T | null;
}

export function useDirtyForm<T extends Record<string, unknown>>(
  initialValues: T | null,
  currentValues: T | null,
  options: UseDirtyFormOptions = {}
): UseDirtyFormReturn<T> {
  const { warnBeforeUnload = false } = options;
  const [baseline, setBaseline] = useState<T | null>(initialValues);

  const dirtyFields = useMemo(() => {
    if (!baseline || !currentValues) return {} as Record<string, boolean>;
    const fields: Record<string, boolean> = {};
    const allKeys = new Set([...Object.keys(baseline), ...Object.keys(currentValues)]);
    for (const key of allKeys) {
      if (baseline[key as keyof T] !== currentValues[key as keyof T]) {
        fields[key] = true;
      }
    }
    return fields;
  }, [baseline, currentValues]);

  const isDirty = Object.keys(dirtyFields).length > 0;

  useEffect(() => {
    if (warnBeforeUnload && isDirty) {
      const handler = (e: BeforeUnloadEvent) => {
        e.preventDefault();
        e.returnValue = "";
      };
      window.addEventListener("beforeunload", handler);
      return () => window.removeEventListener("beforeunload", handler);
    }
  }, [warnBeforeUnload, isDirty]);

  const setInitialState = useCallback((values: T) => setBaseline(values), []);
  const markPristine = useCallback((field: keyof T) => {
    setBaseline((prev) => (prev ? { ...prev, [field]: currentValues?.[field] } : prev));
  }, [currentValues]);
  const reset = useCallback(() => setBaseline(null), []);

  return { isDirty, dirtyFields, setInitialState, markPristine, reset, initialValues: baseline };
}
