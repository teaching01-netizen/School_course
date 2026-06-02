import { expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { RulePreviewPanel, buildSummary, buildConditionBadge, RULE_TYPE_DESCRIPTIONS } from "../RulePreviewPanel";
import type { SitInRuleCreateInput } from "../../types";

const baseForm: SitInRuleCreateInput = {
  name: "Test Rule",
  type: "level_ladder",
  predicate: { min_level_for_sit_lower: 2 },
  description: "",
};

it("renders rule preview panel", () => {
  render(<RulePreviewPanel form={baseForm} />);
  expect(screen.getByText("Rule Preview")).toBeInTheDocument();
});

it("shows summary with name and type description", () => {
  render(<RulePreviewPanel form={baseForm} />);
  expect(screen.getByText("Test Rule: Level 1 students get Zoom; higher-level students sit in the next level up")).toBeInTheDocument();
});

it("shows summary without name using type description only", () => {
  render(<RulePreviewPanel form={{ ...baseForm, name: "" }} />);
  expect(screen.getByText("Level 1 students get Zoom; higher-level students sit in the next level up")).toBeInTheDocument();
});

it("renders level ladder visual for level_ladder type", () => {
  render(<RulePreviewPanel form={baseForm} />);
  expect(screen.getByText("Sit-In Level Map")).toBeInTheDocument();
  expect(screen.getByText("Level 1")).toBeInTheDocument();
  expect(screen.getByText("Level 4")).toBeInTheDocument();
});

it("renders day labels for non-level_ladder types", () => {
  const form: SitInRuleCreateInput = {
    ...baseForm,
    type: "cross_section",
    predicate: { section_match: "cross_section", occurrence_match: "same_occurrence_number", day_match: "any", last_class_excluded: true },
  };
  render(<RulePreviewPanel form={form} />);
  expect(screen.getByText("Mon")).toBeInTheDocument();
  expect(screen.getByText("Fri")).toBeInTheDocument();
});

it("shows condition badge for level_ladder", () => {
  render(<RulePreviewPanel form={baseForm} />);
  expect(screen.getByText("Level 1: Zoom | Min Level: 2")).toBeInTheDocument();
});

it("buildSummary returns type description when name is empty", () => {
  const form = { ...baseForm, name: "" };
  expect(buildSummary(form)).toBe(RULE_TYPE_DESCRIPTIONS.level_ladder.description);
});

it("buildSummary includes name when provided", () => {
  expect(buildSummary(baseForm)).toBe("Test Rule: Level 1 students get Zoom; higher-level students sit in the next level up");
});

it("buildConditionBadge returns cross_section badge", () => {
  const form: SitInRuleCreateInput = {
    ...baseForm,
    type: "cross_section",
    predicate: { section_match: "cross_section", occurrence_match: "same_occurrence_number", day_match: "any", last_class_excluded: true },
  };
  expect(buildConditionBadge(form)).toBe("Section: Cross | Occurrence: Same# | Day: Any | No Last");
});

it("buildConditionBadge returns any_day_except_last badge", () => {
  const form: SitInRuleCreateInput = {
    ...baseForm,
    type: "any_day_except_last",
    predicate: { day_match: "same_day", last_class_excluded: true },
  };
  expect(buildConditionBadge(form)).toBe("Day: Same | No Last");
});

it("buildConditionBadge returns rank_chain badge", () => {
  const form: SitInRuleCreateInput = {
    ...baseForm,
    type: "rank_chain",
    predicate: { chains: [{ from: 1, to: 2 }, { from: 2, to: 3 }] },
  };
  expect(buildConditionBadge(form)).toBe("1→2, 2→3");
});

it("buildConditionBadge returns teacher_case_by_case badge", () => {
  const form: SitInRuleCreateInput = {
    ...baseForm,
    type: "teacher_case_by_case",
    predicate: { auto_assign: false, requires_teacher_approval: true },
  };
  expect(buildConditionBadge(form)).toBe("Manual | Approval");
});
