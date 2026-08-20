import {
  LEGACY_SESSION_STORAGE_KEY,
  LEGACY_VERIFICATION_STORAGE_KEY,
  STUDENT_RESUME_STORAGE_KEY,
  STUDENT_SESSION_HINT_STORAGE_KEY,
} from "../constants";
import { normalizeLookupWcode } from "../domain/studentIdentity";

export type StudentResumeRecord = {
  wcode: string;
  collectedEmail?: string;
};

export function clearLegacyAbsenceDraft(storage: Storage = window.sessionStorage): void {
  storage.removeItem(LEGACY_SESSION_STORAGE_KEY);
  storage.removeItem(LEGACY_VERIFICATION_STORAGE_KEY);
}

export function readStudentResume(storage: Storage = window.sessionStorage): StudentResumeRecord | null {
  const raw = storage.getItem(STUDENT_RESUME_STORAGE_KEY);
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as Partial<{ wcode: string; collectedEmail: string }>;
    const wcode = normalizeLookupWcode(typeof parsed.wcode === "string" ? parsed.wcode : "");
    if (!wcode) {
      storage.removeItem(STUDENT_RESUME_STORAGE_KEY);
      return null;
    }
    return {
      wcode,
      collectedEmail: typeof parsed.collectedEmail === "string" ? parsed.collectedEmail : undefined,
    };
  } catch {
    storage.removeItem(STUDENT_RESUME_STORAGE_KEY);
    return null;
  }
}

export function writeStudentResume(record: StudentResumeRecord, storage: Storage = window.sessionStorage): void {
  storage.setItem(STUDENT_RESUME_STORAGE_KEY, JSON.stringify(record));
}

export function clearStudentResume(storage: Storage = window.sessionStorage): void {
  storage.removeItem(STUDENT_RESUME_STORAGE_KEY);
}

export function hasStudentSessionHint(storage: Storage = window.sessionStorage): boolean {
  try {
    return storage.getItem(STUDENT_SESSION_HINT_STORAGE_KEY) === "1";
  } catch {
    return false;
  }
}

export function markStudentSessionHint(storage: Storage = window.sessionStorage): void {
  try {
    storage.setItem(STUDENT_SESSION_HINT_STORAGE_KEY, "1");
  } catch { }
}

export function clearStudentSessionHint(storage: Storage = window.sessionStorage): void {
  try {
    storage.removeItem(STUDENT_SESSION_HINT_STORAGE_KEY);
  } catch { }
}