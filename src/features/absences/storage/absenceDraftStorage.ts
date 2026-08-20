import { isWCode, normalizeLookupWcode } from "../domain/studentIdentity";

export const ABSENCE_DRAFT_STORAGE_KEY = "warwick.absence.draft.v1";
export const ABSENCE_DRAFT_SCHEMA_VERSION = 1 as const;

export type AbsenceDraftStep = 0 | 1 | 2 | 3;

export type AbsenceDraftV1 = {
  schemaVersion: typeof ABSENCE_DRAFT_SCHEMA_VERSION;
  updatedAt: number;
  wcode: string;
  collectedEmail?: string;
  step: AbsenceDraftStep;
  selectedSubjectIds: string[];
  selectedSessionIds: string[];
  sitInSelections: Record<string, string>;
  sitInPriorityLevels: Record<string, number>;
  reason: string;
};

type StorageLike = Pick<Storage, "getItem" | "setItem" | "removeItem">;

function getSessionStorage(storage?: StorageLike): StorageLike | null {
  if (storage) return storage;
  if (typeof window === "undefined") return null;
  try {
    return window.sessionStorage;
  } catch {
    return null;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === "string");
}

function isStringRecord(value: unknown): value is Record<string, string> {
  return isRecord(value) && Object.values(value).every((item) => typeof item === "string");
}

function isNumberRecord(value: unknown): value is Record<string, number> {
  return isRecord(value) && Object.values(value).every((item) => typeof item === "number" && Number.isFinite(item));
}

function isDraft(value: unknown): value is AbsenceDraftV1 {
  if (!isRecord(value)) return false;
  if (value.schemaVersion !== ABSENCE_DRAFT_SCHEMA_VERSION) return false;
  if (typeof value.updatedAt !== "number" || !Number.isFinite(value.updatedAt)) return false;
  if (typeof value.wcode !== "string" || !isWCode(value.wcode)) return false;
  if (![0, 1, 2, 3].includes(value.step as number)) return false;
  if (!isStringArray(value.selectedSubjectIds)) return false;
  if (!isStringArray(value.selectedSessionIds)) return false;
  if (!isStringRecord(value.sitInSelections)) return false;
  if (!isNumberRecord(value.sitInPriorityLevels)) return false;
  if (typeof value.reason !== "string" || value.reason.length > 500) return false;
  if (value.collectedEmail !== undefined && typeof value.collectedEmail !== "string") return false;
  return true;
}

function toStoredDraft(draft: AbsenceDraftV1): AbsenceDraftV1 {
  return {
    schemaVersion: ABSENCE_DRAFT_SCHEMA_VERSION,
    updatedAt: draft.updatedAt,
    wcode: normalizeLookupWcode(draft.wcode),
    ...(draft.collectedEmail ? { collectedEmail: draft.collectedEmail } : {}),
    step: draft.step,
    selectedSubjectIds: [...draft.selectedSubjectIds],
    selectedSessionIds: [...draft.selectedSessionIds],
    sitInSelections: { ...draft.sitInSelections },
    sitInPriorityLevels: { ...draft.sitInPriorityLevels },
    reason: draft.reason.slice(0, 500),
  };
}

export function readAbsenceDraft(storage?: StorageLike): AbsenceDraftV1 | null {
  const target = getSessionStorage(storage);
  if (!target) return null;
  try {
    const raw = target.getItem(ABSENCE_DRAFT_STORAGE_KEY);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    if (!isDraft(parsed)) {
      target.removeItem(ABSENCE_DRAFT_STORAGE_KEY);
      return null;
    }
    return {
      ...parsed,
      wcode: normalizeLookupWcode(parsed.wcode),
      selectedSubjectIds: [...parsed.selectedSubjectIds],
      selectedSessionIds: [...parsed.selectedSessionIds],
      sitInSelections: { ...parsed.sitInSelections },
      sitInPriorityLevels: { ...parsed.sitInPriorityLevels },
    };
  } catch {
    try { target.removeItem(ABSENCE_DRAFT_STORAGE_KEY); } catch { }
    return null;
  }
}

export function writeAbsenceDraft(draft: AbsenceDraftV1, storage?: StorageLike): void {
  const target = getSessionStorage(storage);
  if (!target) return;
  try {
    target.setItem(ABSENCE_DRAFT_STORAGE_KEY, JSON.stringify(toStoredDraft(draft)));
  } catch {
    // Storage is an optional recovery enhancement; form behavior remains in memory.
  }
}

export function clearAbsenceDraft(storage?: StorageLike): void {
  const target = getSessionStorage(storage);
  if (!target) return;
  try { target.removeItem(ABSENCE_DRAFT_STORAGE_KEY); } catch { }
}
