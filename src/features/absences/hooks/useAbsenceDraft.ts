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
  // The draft that still waits to be restored into the form (the resume
  // snapshot, seeded from the on-load draft and re-seeded when the same
  // Student ID is re-identified mid-flow). While it exists, auto-save is
  // suspended: the form's pre-restore state is cleared/empty and must never
  // overwrite the stored draft the student is returning to.
  const [restoreDraft, setRestoreDraft] = useState<AbsenceDraftV1 | null>(() => readAbsenceDraft());
  const restoreRef = useRef<AbsenceDraftV1 | null>(restoreDraft);
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
    // While a restore snapshot is pending the stored draft is the source the
    // form is about to restore from; refuse any auto-save that could clobber
    // it (e.g. the empty selection state shown before the restore runs).
    if (restoreRef.current) return;
    pendingRef.current = values;
    if (timerRef.current != null) window.clearTimeout(timerRef.current);
    timerRef.current = window.setTimeout(flushPending, DRAFT_DEBOUNCE_MS);
  }, [flushPending]);

  /** Marks `snapshot` as awaiting restore (auto-save suspended). Pass null
   *  once the restore is consumed, abandoned, or the report is discarded. */
  const beginRestore = useCallback((snapshot: AbsenceDraftV1 | null) => {
    cancelPending();
    restoreRef.current = snapshot;
    setRestoreDraft(snapshot);
  }, [cancelPending]);

  const clearDraft = useCallback(() => {
    cancelPending();
    clearAbsenceDraft();
    setDraft(null);
    // Discarding the report also ends any pending restore: nothing may
    // resurrect it and auto-save must not stay suspended for the next report.
    restoreRef.current = null;
    setRestoreDraft(null);
  }, [cancelPending]);

  return { draft, restoreDraft, saveDraft, clearDraft, beginRestore };
}

export type { AbsenceDraftStep, AbsenceDraftV1, DraftValues };
