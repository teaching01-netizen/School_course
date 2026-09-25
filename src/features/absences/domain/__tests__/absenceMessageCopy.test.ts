import { describe, expect, it } from "vitest";
import { getAbsenceMessageCopy } from "../absenceMessageCopy";

describe("getAbsenceMessageCopy", () => {
  it.each([
    ["before_request_date", "This make-up time has passed"],
    ["outside_cutoff", "This date is outside the make-up period"],
    ["target_final_class", "The final class can't be used as a make-up"],
    ["overlaps_missed_class", "This make-up class overlaps your absence"],
    ["sit_in_session_already_used", "You've already used this make-up class"],
    ["same_occurrence_missing", "No matching lesson is available"],
  ])("maps %s to student-facing copy", (reasonCode, message) => {
    expect(getAbsenceMessageCopy(reasonCode)).toBe(message);
  });

  it("uses safe copy for unknown or missing reason codes", () => {
    expect(getAbsenceMessageCopy("backend wording that is unknown")).toBe("This make-up class isn’t available.");
    expect(getAbsenceMessageCopy()).toBe("This make-up class isn’t available.");
    expect(getAbsenceMessageCopy("toString")).toBe("This make-up class isn’t available.");
  });
});
