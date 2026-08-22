import { useCallback, useEffect, useMemo, useState } from "react";

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

  useEffect(() => {
    const stored = readStoredOtp(storageKey);
    if (stored) {
      setToken(stored.token);
      setExpiresAt(stored.expiresAt);
    }
  }, [storageKey]);

  // No clock polling here: the consumer enforces expiry with a one-shot
  // timeout, so re-rendering the page every second would be pure waste.
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
    persistToken,
    clearStoredToken,
  }), [code, setCode, token, setToken, expiresAt, setAndPersistExpiresAt, persistToken, clearStoredToken]);
}
