import { describe, expect, it } from "vitest";
import { buildImpactCandidate } from "../test/builders";
import { isCandidateSelectable, capacityLabel } from "./candidateAssessment";

describe("isCandidateSelectable", () => {
  it("eligible with no conflict and available capacity is selectable", () => {
    const c = buildImpactCandidate({ eligible: true, student_conflicts: false, available_capacity: 4 });
    expect(isCandidateSelectable(c)).toBe(true);
  });

  it("ineligible candidate is not selectable", () => {
    const c = buildImpactCandidate({ eligible: false, available_capacity: 4 });
    expect(isCandidateSelectable(c)).toBe(false);
  });

  it("candidate with student conflicts and blocking reasons is not selectable", () => {
    const c = buildImpactCandidate({
      eligible: true,
      student_conflicts: true,
      available_capacity: 4,
      blocking_reasons: [{ code: "student_conflict", message: "Conflicts with regular class" }],
    });
    expect(isCandidateSelectable(c)).toBe(false);
  });

  it("full candidate (capacity 0) is not selectable", () => {
    const c = buildImpactCandidate({ eligible: true, available_capacity: 0 });
    expect(isCandidateSelectable(c)).toBe(false);
  });

  it("negative capacity is selectable (unlimited)", () => {
    const c = buildImpactCandidate({ eligible: true, available_capacity: -1 });
    expect(isCandidateSelectable(c)).toBe(true);
  });

  it("candidate with blocking reasons is not selectable even if eligible", () => {
    const c = buildImpactCandidate({
      eligible: true,
      available_capacity: 5,
      blocking_reasons: [{ code: "not_eligible", message: "Not eligible" }],
    });
    expect(isCandidateSelectable(c)).toBe(false);
  });

  it("candidate with empty blocking_reasons array is selectable", () => {
    const c = buildImpactCandidate({ eligible: true, available_capacity: 5, blocking_reasons: [] });
    expect(isCandidateSelectable(c)).toBe(true);
  });

  it("eligible with conflicts but no blocking_reasons is selectable (conflicts is visual only)", () => {
    const c = buildImpactCandidate({ eligible: true, student_conflicts: true, available_capacity: 5 });
    expect(isCandidateSelectable(c)).toBe(true);
  });
});

describe("capacityLabel", () => {
  it("shows seats available for positive capacity", () => {
    expect(capacityLabel(buildImpactCandidate({ available_capacity: 4 }))).toBe("4 seats available");
  });

  it("shows 'Full' for zero capacity", () => {
    expect(capacityLabel(buildImpactCandidate({ available_capacity: 0 }))).toBe("Full");
  });

  it("shows 'Capacity not limited' for negative capacity", () => {
    expect(capacityLabel(buildImpactCandidate({ available_capacity: -1 }))).toBe("Capacity not limited");
  });

  it("shows '1 seats available' for capacity 1", () => {
    expect(capacityLabel(buildImpactCandidate({ available_capacity: 1 }))).toBe("1 seats available");
  });
});
