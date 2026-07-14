import { describe, expect, it } from "vitest";
import {
  clearLegacyAbsenceDraft,
  clearStudentResume,
  readStudentResume,
  writeStudentResume,
} from "../studentResumeStorage";
import {
  LEGACY_SESSION_STORAGE_KEY,
  LEGACY_VERIFICATION_STORAGE_KEY,
  STUDENT_RESUME_STORAGE_KEY,
} from "../../constants";

function storageWith(initial?: Record<string, string>): Storage {
  const values = new Map(Object.entries(initial ?? {}));
  return {
    get length() { return values.size; },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => { values.delete(key); },
    setItem: (key, value) => { values.set(key, value); },
  };
}

describe("student resume storage", () => {
  it("normalizes lowercase w-codes when reading resume data", () => {
    const storage = storageWith({
      [STUDENT_RESUME_STORAGE_KEY]: JSON.stringify({ wcode: " w250389 ", collectedEmail: "student@example.edu" }),
    });

    expect(readStudentResume(storage)).toEqual({
      wcode: "W250389",
      collectedEmail: "student@example.edu",
    });
  });

  it("removes malformed resume records", () => {
    const storage = storageWith({ [STUDENT_RESUME_STORAGE_KEY]: "{bad" });

    expect(readStudentResume(storage)).toBeNull();
    expect(storage.getItem(STUDENT_RESUME_STORAGE_KEY)).toBeNull();
  });

  it("writes only the resume record shape", () => {
    const storage = storageWith();

    writeStudentResume({ wcode: "W250389", collectedEmail: "student@example.edu" }, storage);

    expect(JSON.parse(storage.getItem(STUDENT_RESUME_STORAGE_KEY) ?? "{}")).toEqual({
      wcode: "W250389",
      collectedEmail: "student@example.edu",
    });
  });

  it("removes a resume record without a usable W-code", () => {
    const storage = storageWith({
      [STUDENT_RESUME_STORAGE_KEY]: JSON.stringify({
        wcode: "   ",
        collectedEmail: "student@example.edu",
      }),
    });

    expect(readStudentResume(storage)).toBeNull();
    expect(storage.getItem(STUDENT_RESUME_STORAGE_KEY)).toBeNull();
  });

  it("ignores a collected email with the wrong persisted type", () => {
    const storage = storageWith({
      [STUDENT_RESUME_STORAGE_KEY]: JSON.stringify({
        wcode: "W250389",
        collectedEmail: { address: "student@example.edu" },
      }),
    });

    expect(readStudentResume(storage)).toEqual({
      wcode: "W250389",
      collectedEmail: undefined,
    });
  });

  it("clears only legacy draft keys during migration", () => {
    const storage = storageWith({
      [LEGACY_SESSION_STORAGE_KEY]: "legacy-draft",
      [LEGACY_VERIFICATION_STORAGE_KEY]: "legacy-verification",
      [STUDENT_RESUME_STORAGE_KEY]: "current-resume",
    });

    clearLegacyAbsenceDraft(storage);

    expect(storage.getItem(LEGACY_SESSION_STORAGE_KEY)).toBeNull();
    expect(storage.getItem(LEGACY_VERIFICATION_STORAGE_KEY)).toBeNull();
    expect(storage.getItem(STUDENT_RESUME_STORAGE_KEY)).toBe("current-resume");
  });

  it("clears the current student resume without clearing unrelated session data", () => {
    const storage = storageWith({
      [STUDENT_RESUME_STORAGE_KEY]: "current-resume",
      unrelated: "keep-me",
    });

    clearStudentResume(storage);

    expect(storage.getItem(STUDENT_RESUME_STORAGE_KEY)).toBeNull();
    expect(storage.getItem("unrelated")).toBe("keep-me");
  });
});
