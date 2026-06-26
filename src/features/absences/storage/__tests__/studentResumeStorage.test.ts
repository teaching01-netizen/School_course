import { describe, expect, it } from "vitest";
import { readStudentResume, writeStudentResume } from "../studentResumeStorage";
import { STUDENT_RESUME_STORAGE_KEY } from "../../constants";

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
});
