import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAbsenceDraft, type DraftValues } from "../../hooks/useAbsenceDraft";
import {
  ABSENCE_DRAFT_STORAGE_KEY,
  readAbsenceDraft,
  writeAbsenceDraft,
  type AbsenceDraftV1,
} from "../absenceDraftStorage";

/**
 * The resume-window contract lives at the persistence layer: the draft that is
 * about to be restored is the source of truth until the restore runs, so
 * auto-save must be suspended while it exists, and the stored draft must
 * round-trip intact through the window.
 */

const resumeDraft: AbsenceDraftV1 = {
  schemaVersion: 1,
  updatedAt: 1_700_000_000_000,
  wcode: "W250389",
  collectedEmail: "student@example.edu",
  step: 1,
  selectedSubjectIds: ["subject-math"],
  selectedSessionIds: ["session-math-1"],
  sitInSelections: { "session-math-1": "sit-in-1" },
  sitInPriorityLevels: { "session-math-1": 2 },
  reason: "Appointment",
};

/** The cleared in-form state that precedes a restore (the historical clobber). */
const preRestoreState: DraftValues = {
  wcode: resumeDraft.wcode,
  collectedEmail: "",
  step: 0,
  selectedSubjectIds: [],
  selectedSessionIds: [],
  sitInSelections: {},
  sitInPriorityLevels: {},
  reason: "",
};

const restoredState: DraftValues = {
  wcode: resumeDraft.wcode,
  collectedEmail: resumeDraft.collectedEmail,
  step: 3,
  selectedSubjectIds: ["subject-math"],
  selectedSessionIds: ["session-math-1", "session-physics-1"],
  sitInSelections: { "session-math-1": "sit-in-1", "session-physics-1": "sit-in-2" },
  sitInPriorityLevels: { "session-math-1": 2 },
  reason: "Appointment: Dentist",
};

function writtenFields(stored: AbsenceDraftV1 | null) {
  expect(stored).not.toBeNull();
  const { schemaVersion, updatedAt, ...fields } = stored as AbsenceDraftV1;
  expect(schemaVersion).toBe(1);
  expect(updatedAt).toBeGreaterThanOrEqual(resumeDraft.updatedAt);
  return fields;
}

describe("absence draft resume window (auto-save vs. pending restore snapshot)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    window.sessionStorage.clear();
  });

  afterEach(() => {
    vi.useRealTimers();
    window.sessionStorage.clear();
  });

  it("never lets auto-save overwrite the stored draft while a restore snapshot is pending", () => {
    // A returning student's full report is on disk when the page loads.
    writeAbsenceDraft(resumeDraft);
    const { result } = renderHook(() => useAbsenceDraft());

    // The hook seeds the pending restore from the stored draft and suspends
    // auto-save for the whole window (resume prompt -> re-verification).
    expect(result.current.restoreDraft).toEqual(resumeDraft);

    // The pre-restore form state is empty. A naive auto-save at this point is
    // exactly what used to wipe the report before it could be restored — the
    // hook must refuse it, and keep refusing on every attempt.
    act(() => {
      result.current.saveDraft(preRestoreState);
      result.current.saveDraft({ ...preRestoreState, reason: "Typed while waiting" });
    });
    act(() => {
      vi.advanceTimersByTime(2_000);
    });

    expect(readAbsenceDraft()).toEqual(resumeDraft);
    expect(result.current.draft).toEqual(resumeDraft);
    expect(window.sessionStorage.getItem(ABSENCE_DRAFT_STORAGE_KEY)).toContain("session-math-1");
  });

  it("releases the snapshot on beginRestore(null) and round-trips the restored state intact", () => {
    writeAbsenceDraft(resumeDraft);
    const { result } = renderHook(() => useAbsenceDraft());
    expect(result.current.restoreDraft).toEqual(resumeDraft);

    // The classes restore consumed the snapshot; auto-save resumes.
    act(() => {
      result.current.beginRestore(null);
    });
    expect(result.current.restoreDraft).toBeNull();

    // The restored in-form state is persisted and reads back field-for-field.
    act(() => {
      result.current.saveDraft(restoredState);
    });
    act(() => {
      vi.advanceTimersByTime(2_000);
    });

    expect(writtenFields(readAbsenceDraft())).toEqual(restoredState);
    expect(writtenFields(result.current.draft)).toEqual(restoredState);
  });

  it("keeps auto-save suspended when a fresh snapshot is re-seeded mid-flow", () => {
    // An in-progress report is auto-saved while the student works.
    writeAbsenceDraft(resumeDraft);
    const { result } = renderHook(() => useAbsenceDraft());
    act(() => {
      result.current.beginRestore(null); // first restore consumed
    });

    // The student wanders back to Identify and re-enters the same ID; the
    // stored report becomes a pending restore again (no pre-restore clobber).
    act(() => {
      result.current.beginRestore(readAbsenceDraft());
    });
    expect(result.current.restoreDraft).toEqual(resumeDraft);

    act(() => {
      result.current.saveDraft(preRestoreState);
    });
    act(() => {
      vi.advanceTimersByTime(2_000);
    });
    expect(readAbsenceDraft()).toEqual(resumeDraft);
  });

  it("Start over (clearDraft) releases the snapshot, clears storage, and re-enables auto-save", () => {
    writeAbsenceDraft(resumeDraft);
    const { result } = renderHook(() => useAbsenceDraft());
    expect(result.current.restoreDraft).toEqual(resumeDraft);

    act(() => {
      result.current.clearDraft();
    });
    expect(result.current.restoreDraft).toBeNull();
    expect(result.current.draft).toBeNull();
    expect(readAbsenceDraft()).toBeNull();

    // A brand-new report auto-saves normally again.
    act(() => {
      result.current.saveDraft(restoredState);
    });
    act(() => {
      vi.advanceTimersByTime(2_000);
    });
    expect(writtenFields(readAbsenceDraft())).toEqual(restoredState);
  });

  it("returns independent copies so a consumer editing a read cannot corrupt the stored draft", () => {
    writeAbsenceDraft(resumeDraft);

    const first = readAbsenceDraft()!;
    const second = readAbsenceDraft()!;
    first.selectedSessionIds.push("session-mutated");
    first.sitInSelections["session-math-1"] = "sit-in-mutated";

    expect(readAbsenceDraft()).toEqual(second);
    expect(readAbsenceDraft()).toEqual(resumeDraft);
  });
});
