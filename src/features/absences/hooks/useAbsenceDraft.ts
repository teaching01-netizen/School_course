import { useCallback, useEffect, useRef, useState } from "react";
import {
  ABSENCE_DRAFT_SCHEMA_VERSION,
  clearAbsenceDraft,
  readAbsenceDraft,
  writeAbsenceDraft,
  type AbsenceDraftStep,
  type AbsenceDraftV1,
} from "../storage/absenceDraftStorage";

type DraftValues = Omit<AbsenceDraftV1, "schemaVersion" | "updatedAt">;

// Drafts are best-effort resume snapshots; a short trailing debounce keeps
// typing (and the re-render it triggers) off the synchronous localStorage path.
const DRAFT_DEBOUNCE_MS = 300;

export function useAbsenceDraft() {
  const [draft, setDraft] = useState<AbsenceDraftV1 | null>(() => readAbsenceDraft());
  const pendingRef = useRef<DraftValues | null>(null);
  const timerRef = useRef<number | null>(null);

  const cancelPending = useCallback(() => {
    if (timerRef.current != null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    pendingRef.current = null;
  }, []);

  const flushPending = useCallback(() => {
    const values = pendingRef.current;
    if (!values) return;
    cancelPending();
    const nextDraft: AbsenceDraftV1 = {
      ...values,
      schemaVersion: ABSENCE_DRAFT_SCHEMA_VERSION,
      updatedAt: Date.now(),
    };
    writeAbsenceDraft(nextDraft);
    setDraft(nextDraft);
  }, [cancelPending]);

  // Persist whatever is pending before the page is unloaded or hidden so a
  // debounced write can never lose the tail end of the user's input.
  useEffect(() => {
    const flush = () => flushPending();
    window.addEventListener("pagehide", flush);
    document.addEventListener("visibilitychange", flush);
    return () => {
      window.removeEventListener("pagehide", flush);
      document.removeEventListener("visibilitychange", flush);
      flushPending();
    };
  }, [flushPending]);

  const saveDraft = useCallback((values: DraftValues) => {
    pendingRef.current = values;
    if (timerRef.current != null) window.clearTimeout(timerRef.current);
    timerRef.current = window.setTimeout(flushPending, DRAFT_DEBOUNCE_MS);
  }, [flushPending]);

  const clearDraft = useCallback(() => {
    cancelPending();
    clearAbsenceDraft();
    setDraft(null);
  }, [cancelPending]);

  return { draft, saveDraft, clearDraft };
}

export type { AbsenceDraftStep, AbsenceDraftV1, DraftValues };
