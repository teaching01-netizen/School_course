import { useCallback, useEffect, useMemo, useState } from "react";

type StoredOtpState = {
  token: string;
  expiresAt: number | null;
};

function readStoredOtp(storageKey: string): StoredOtpState | null {
  try {
    const raw = window.localStorage.getItem(storageKey);
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
      window.localStorage.removeItem(storageKey);
      return;
    }
    window.localStorage.setItem(storageKey, JSON.stringify(value));
  } catch {
    // ignore storage failures
  }
}

export function useOtp(storageKey: string) {
  const [code, setCode] = useState("");
  const [token, setToken] = useState<string | null>(null);
  const [expiresAt, setExpiresAt] = useState<number | null>(null);
  const [, forceTick] = useState(0);

  useEffect(() => {
    const stored = readStoredOtp(storageKey);
    if (stored) {
      setToken(stored.token);
      setExpiresAt(stored.expiresAt);
    }
  }, [storageKey]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      forceTick((value) => value + 1);
    }, 1000);
    return () => window.clearInterval(timer);
  }, []);

  const secondsLeft = useMemo(() => {
    if (!expiresAt) return 0;
    return Math.max(0, Math.ceil((expiresAt - Date.now()) / 1000));
  }, [expiresAt]);

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
