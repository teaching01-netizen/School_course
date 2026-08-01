import { createContext, useCallback, useContext, useMemo, useState } from "react";

const STORAGE_KEY = "schedule_impact_flags";

function readFlags(): Record<string, boolean> {
  const fromUrl = new URLSearchParams(window.location.search).get("ff");
  if (fromUrl) {
    const flags: Record<string, boolean> = {};
    fromUrl.split(",").forEach((name) => {
      const trimmed = name.trim();
      if (trimmed) flags[trimmed] = true;
    });
    return flags;
  }

  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) return JSON.parse(raw) as Record<string, boolean>;
  } catch {
    // corrupt storage — treat as empty
  }
  return {};
}

function writeFlags(flags: Record<string, boolean>): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(flags));
  } catch {
    // storage full or unavailable — silently ignore
  }
}

type FeatureFlagContextValue = {
  flags: Record<string, boolean>;
  setFlag: (name: string, enabled: boolean) => void;
};

const FeatureFlagContext = createContext<FeatureFlagContextValue>({
  flags: {},
  setFlag: () => {},
});

export function FeatureFlagProvider({ children }: { children: React.ReactNode }) {
  const [flags, setFlags] = useState<Record<string, boolean>>(readFlags);

  const setFlag = useCallback((name: string, enabled: boolean) => {
    setFlags((prev) => {
      const next = { ...prev, [name]: enabled };
      writeFlags(next);
      return next;
    });
  }, []);

  const value = useMemo(() => ({ flags, setFlag }), [flags, setFlag]);

  return (
    <FeatureFlagContext.Provider value={value}>
      {children}
    </FeatureFlagContext.Provider>
  );
}

export function useFeatureFlag(flagName: string): boolean {
  const { flags } = useContext(FeatureFlagContext);
  return flags[flagName] === true;
}

export function setFeatureFlag(flagName: string, enabled: boolean): void {
  try {
    const current = readFlags();
    const next = { ...current, [flagName]: enabled };
    writeFlags(next);
  } catch {
    // storage unavailable
  }
}
