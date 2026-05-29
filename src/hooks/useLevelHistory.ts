import { useState, useEffect, useCallback, useMemo } from "react";

const STORAGE_KEY = "level-history";
const MAX_DEPTH = 20;

type LevelSnapshot = {
  id: string;
  timestamp: number;
  description: string;
  before: Record<string, number | null>;
  after: Record<string, number | null>;
};

function loadFromStorage(): LevelSnapshot[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.slice(-MAX_DEPTH);
  } catch {
    return [];
  }
}

export default function useLevelHistory() {
  const [snapshots, setSnapshots] = useState<LevelSnapshot[]>(() => loadFromStorage());

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(snapshots));
  }, [snapshots]);

  const pushSnapshot = useCallback(
    (
      description: string,
      before: Record<string, number | null>,
      after: Record<string, number | null>,
    ) => {
      const snapshot: LevelSnapshot = {
        id: crypto.randomUUID(),
        timestamp: Date.now(),
        description,
        before,
        after,
      };
      setSnapshots((prev) => {
        const next = [...prev, snapshot];
        return next.length > MAX_DEPTH ? next.slice(-MAX_DEPTH) : next;
      });
    },
    [],
  );

  const undoLast = useCallback((): Record<string, number | null> | null => {
    let restored: Record<string, number | null> | null = null;
    setSnapshots((prev) => {
      if (prev.length === 0) return prev;
      restored = prev[prev.length - 1].before;
      return prev.slice(0, -1);
    });
    return restored;
  }, []);

  const canUndo = useMemo(() => snapshots.length > 0, [snapshots]);
  const lastAction = useMemo(
    () => (snapshots.length > 0 ? snapshots[snapshots.length - 1].description : null),
    [snapshots],
  );

  return {
    pushSnapshot,
    undoLast,
    canUndo,
    lastAction,
    historyCount: snapshots.length,
  };
}
