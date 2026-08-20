import { useRef, useEffect, useCallback } from "react";

export function useMainInert() {
  const inertCounts = useRef<Map<string, number>>(new Map());
  const scrollLockCounts = useRef<Map<string, number>>(new Map());
  const previousOverflowRef = useRef<string>("");

  const sync = useCallback(() => {
    if (typeof document === "undefined") return;
    const main = document.querySelector("main");
    if (main) {
      const inertCount = [...inertCounts.current.values()].reduce((a, b) => a + b, 0);
      if (inertCount > 0) {
        if ("inert" in HTMLElement.prototype) main.setAttribute("inert", "");
        else main.setAttribute("aria-hidden", "true");
      } else {
        main.removeAttribute("inert");
        main.removeAttribute("aria-hidden");
      }
    }
    const lockCount = [...scrollLockCounts.current.values()].reduce((a, b) => a + b, 0);
    if (lockCount > 0) document.body.style.overflow = "hidden";
    else document.body.style.overflow = previousOverflowRef.current;
  }, []);

  const add = useCallback(
    (key: string) => {
      if (typeof document === "undefined") return;
      const lockTotalBefore = [...scrollLockCounts.current.values()].reduce((a, b) => a + b, 0);
      if (lockTotalBefore === 0) previousOverflowRef.current = document.body.style.overflow;
      inertCounts.current.set(key, (inertCounts.current.get(key) ?? 0) + 1);
      scrollLockCounts.current.set(key, (scrollLockCounts.current.get(key) ?? 0) + 1);
      sync();
    },
    [sync],
  );

  const remove = useCallback(
    (key: string) => {
      if (typeof document === "undefined") return;
      const nextInert = Math.max(0, (inertCounts.current.get(key) ?? 0) - 1);
      if (nextInert === 0) inertCounts.current.delete(key);
      else inertCounts.current.set(key, nextInert);
      const nextLock = Math.max(0, (scrollLockCounts.current.get(key) ?? 0) - 1);
      if (nextLock === 0) scrollLockCounts.current.delete(key);
      else scrollLockCounts.current.set(key, nextLock);
      sync();
    },
    [sync],
  );

  useEffect(() => {
    return () => {
      if (typeof document === "undefined") return;
      inertCounts.current.clear();
      scrollLockCounts.current.clear();
      sync();
    };
  }, [sync]);

  return { add, remove };
}
