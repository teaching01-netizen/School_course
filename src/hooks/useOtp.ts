import { useCallback, useEffect, useMemo, useRef, useState } from "react";

type StoredOtpState = {
  token: string;
  expiresAt: number | null;
};

function readStoredOtp(storageKey: string): StoredOtpState | null {
  try {
    const raw = window.sessionStorage.getItem(storageKey);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<StoredOtpState>;
    if (typeof parsed.token !== "string" || !parsed.token) return null;
    return {
      token: parsed.token,
      expiresAt: typeof parsed.expiresAt === "number" ? parsed.expiresAt : null,
    };
  } catch {
    return null;
  }
}

function writeStoredOtp(storageKey: string, value: StoredOtpState | null) {
  try {
    if (!value) {
      window.sessionStorage.removeItem(storageKey);
      return;
    }
    window.sessionStorage.setItem(storageKey, JSON.stringify(value));
  } catch {
    // ignore storage failures
  }
}

export function useOtp(storageKey: string) {
  const [code, setCode] = useState("");
  const [token, setToken] = useState<string | null>(null);
  const [expiresAt, setExpiresAt] = useState<number | null>(null);
  const [tick, forceTick] = useState(0);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    const stored = readStoredOtp(storageKey);
    if (stored) {
      setToken(stored.token);
      setExpiresAt(stored.expiresAt);
    }
  }, [storageKey]);

  useEffect(() => {
    if (expiresAt == null) return;
    const schedule = () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      const delay = Math.max(0, expiresAt - Date.now());
      // expiry already passed — tick immediately
      if (delay === 0) {
        forceTick((v) => v + 1);
        return;
      }
      timeoutRef.current = setTimeout(() => {
        forceTick((v) => v + 1);
        // reschedule if still not expired (handles long delays split)
        const remaining = Math.max(0, expiresAt - Date.now());
        if (remaining > 0) schedule();
      }, delay);
    };
    schedule();
    const onVisibility = () => {
      if (!document.hidden) {
        forceTick((v) => v + 1);
        schedule();
      }
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    };
  }, [expiresAt]);

  // fallback tick while no expiry (keeps secondsLeft at 0 stable without leaking interval)
  useEffect(() => {
    if (expiresAt != null) return;
    // no timer needed when no expiry
    return;
  }, [expiresAt]);

  const secondsLeft = useMemo(() => {
    if (!expiresAt) return 0;
    return Math.max(0, Math.ceil((expiresAt - Date.now()) / 1000));
  }, [expiresAt, tick]);

  const persistToken = useCallback((nextToken: string, nextExpiresAt?: number | null) => {
    setToken(nextToken);
    const next = nextExpiresAt ?? expiresAt;
    setExpiresAt(next ?? null);
    writeStoredOtp(storageKey, { token: nextToken, expiresAt: next ?? null });
  }, [expiresAt, storageKey]);

  const clearStoredToken = useCallback(() => {
    setToken(null);
    setExpiresAt(null);
    writeStoredOtp(storageKey, null);
  }, [storageKey]);

  const setAndPersistExpiresAt = useCallback((nextExpiresAt: number | null) => {
    setExpiresAt(nextExpiresAt);
    if (token) {
      writeStoredOtp(storageKey, { token, expiresAt: nextExpiresAt });
    }
  }, [storageKey, token]);

  return useMemo(() => ({
    code,
    setCode,
    token,
    setToken,
    expiresAt,
    setExpiresAt: setAndPersistExpiresAt,
    secondsLeft,
    persistToken,
    clearStoredToken,
  }), [code, setCode, token, setToken, expiresAt, setAndPersistExpiresAt, secondsLeft, persistToken, clearStoredToken]);
}
