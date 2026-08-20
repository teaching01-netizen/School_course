import { useCallback, useState } from "react";
import {
  ABSENCE_DRAFT_SCHEMA_VERSION,
  clearAbsenceDraft,
  readAbsenceDraft,
  writeAbsenceDraft,
  type AbsenceDraftStep,
  type AbsenceDraftV1,
} from "../storage/absenceDraftStorage";

type DraftValues = Omit<AbsenceDraftV1, "schemaVersion" | "updatedAt">;

export function useAbsenceDraft() {
  const [draft, setDraft] = useState<AbsenceDraftV1 | null>(() => readAbsenceDraft());

  const saveDraft = useCallback((values: DraftValues) => {
    const nextDraft: AbsenceDraftV1 = {
      ...values,
      schemaVersion: ABSENCE_DRAFT_SCHEMA_VERSION,
      updatedAt: Date.now(),
    };
    writeAbsenceDraft(nextDraft);
    setDraft(nextDraft);
  }, []);

  const clearDraft = useCallback(() => {
    clearAbsenceDraft();
    setDraft(null);
  }, []);

  return { draft, saveDraft, clearDraft };
}

export type { AbsenceDraftStep, AbsenceDraftV1, DraftValues };
