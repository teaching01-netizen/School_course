import { useEffect, useRef } from "react";
import type { UsePreflightReturn, PreflightParams } from "./usePreflight";

type DebouncedPreflightOptions = {
  enabled: boolean;
  delayMs?: number;
};

export function useDebouncedPreflight(
  preflight: UsePreflightReturn,
  params: PreflightParams | null,
  options: DebouncedPreflightOptions,
): void {
  const { enabled, delayMs = 300 } = options;
  const timerRef = useRef<number | null>(null);

  useEffect(() => {
    if (!enabled || !params) {
      preflight.reset();
      return;
    }

    window.clearTimeout(timerRef.current ?? undefined);

    timerRef.current = window.setTimeout(() => {
      preflight.check(params);
    }, delayMs);

    return () => {
      if (timerRef.current != null) {
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [enabled, params, delayMs]);
}
