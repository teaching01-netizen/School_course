import { useState, useCallback } from "react";
import { apiJson } from "../api/client";

const STORAGE_KEY = "returns-desk-entries";
const TTL_MS = 7 * 24 * 60 * 60 * 1000; // 7 days

export type ReturnsDeskEntry = {
  id: string;
  courseId: string;
  courseCode: string;
  attemptedLevel: number | null;
  cycleId: string;
  error: { code: string; message: string; conflicts?: unknown };
  timestamp: number;
  expiresAt: number;
};

type AddInput = Omit<ReturnsDeskEntry, "id" | "timestamp" | "expiresAt">;

function readStore(): ReturnsDeskEntry[] {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "[]");
  } catch {
    return [];
  }
}

function writeStore(entries: ReturnsDeskEntry[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(entries));
}

export default function useReturnsDesk() {
  const [entries, setEntries] = useState<ReturnsDeskEntry[]>(readStore);

  const persist = useCallback((next: ReturnsDeskEntry[]) => {
    setEntries(next);
    writeStore(next);
  }, []);

  const addFailure = useCallback(
    (input: AddInput) => {
      const now = Date.now();
      const entry: ReturnsDeskEntry = {
        ...input,
        id: crypto.randomUUID(),
        timestamp: now,
        expiresAt: now + TTL_MS,
      };
      persist([...entries, entry]);
    },
    [entries, persist],
  );

  const removeFailure = useCallback(
    (id: string) => {
      persist(entries.filter((e) => e.id !== id));
    },
    [entries, persist],
  );

  const retryFailure = useCallback(
    async (entry: ReturnsDeskEntry): Promise<boolean> => {
      try {
        const course = await apiJson<{ version: number }>(
          `/api/v1/admin/courses/${entry.courseId}`,
        );
        await apiJson(`/api/v1/admin/courses/${entry.courseId}/level`, {
          method: "PUT",
          body: JSON.stringify({
            level: entry.attemptedLevel,
            cycle_id: entry.cycleId,
            expected_version: course.version,
          }),
        });
        persist(entries.filter((e) => e.id !== entry.id));
        return true;
      } catch {
        return false;
      }
    },
    [entries, persist],
  );

  const getActive = useCallback((): ReturnsDeskEntry[] => {
    const now = Date.now();
    const active = entries.filter((e) => e.expiresAt > now);
    if (active.length !== entries.length) persist(active);
    return active;
  }, [entries, persist]);

  const getGrouped = useCallback((): Record<string, ReturnsDeskEntry[]> => {
    const active = getActive();
    const groups: Record<string, ReturnsDeskEntry[]> = {};
    for (const entry of active) {
      const key = entry.error.code;
      (groups[key] ??= []).push(entry);
    }
    return groups;
  }, [getActive]);

  return {
    addFailure,
    removeFailure,
    retryFailure,
    getActive,
    getGrouped,
    totalCount: entries.filter((e) => e.expiresAt > Date.now()).length,
  };
}
