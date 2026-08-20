import { beforeEach, describe, expect, it } from "vitest";
import {
  ABSENCE_DRAFT_STORAGE_KEY,
  clearAbsenceDraft,
  readAbsenceDraft,
  writeAbsenceDraft,
  type AbsenceDraftV1,
} from "../absenceDraftStorage";

const draft: AbsenceDraftV1 = {
  schemaVersion: 1,
  updatedAt: 1_700_000_000_000,
  wcode: "W250389",
  collectedEmail: "student@example.edu",
  step: 2,
  selectedSubjectIds: ["subject-math"],
  selectedSessionIds: ["session-1"],
  sitInSelections: { "session-1": "sit-in-1" },
  sitInPriorityLevels: { "session-1": 2 },
  reason: "Medical appointment",
};

describe("absence draft storage", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it("round-trips a versioned recoverable draft", () => {
    writeAbsenceDraft(draft);

    expect(window.sessionStorage.getItem(ABSENCE_DRAFT_STORAGE_KEY)).toContain('"schemaVersion":1');
    expect(readAbsenceDraft()).toEqual(draft);
  });

  it("rejects malformed, wrong-version, and unsafe drafts", () => {
    window.sessionStorage.setItem(ABSENCE_DRAFT_STORAGE_KEY, "{broken");
    expect(readAbsenceDraft()).toBeNull();

    window.sessionStorage.setItem(
      ABSENCE_DRAFT_STORAGE_KEY,
      JSON.stringify({ ...draft, schemaVersion: 2 }),
    );
    expect(readAbsenceDraft()).toBeNull();

    window.sessionStorage.setItem(
      ABSENCE_DRAFT_STORAGE_KEY,
      JSON.stringify({ ...draft, wcode: "not-a-wcode", selectedSessionIds: "session-1" }),
    );
    expect(readAbsenceDraft()).toBeNull();
  });

  it("clears only the absence draft record", () => {
    window.sessionStorage.setItem("unrelated", "keep me");
    writeAbsenceDraft(draft);

    clearAbsenceDraft();

    expect(window.sessionStorage.getItem(ABSENCE_DRAFT_STORAGE_KEY)).toBeNull();
    expect(window.sessionStorage.getItem("unrelated")).toBe("keep me");
  });
});
