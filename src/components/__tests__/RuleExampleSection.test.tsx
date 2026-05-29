import { expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { RuleExampleSection, buildExampleScenario } from "../RuleExampleSection";
import type { SitInRuleCreateInput } from "../../types";

const baseForm: SitInRuleCreateInput = {
  name: "Test Rule",
  type: "level_ladder",
  predicate: { level_1_action: "zoom", min_level_for_sit_lower: 2 },
  description: "",
};

it("renders example scenario always visible (no toggle)", () => {
  render(<RuleExampleSection form={baseForm} />);
  expect(screen.getByText(/If a Level 1 student is absent/)).toBeInTheDocument();
});

it("does not render expand/collapse button", () => {
  render(<RuleExampleSection form={baseForm} />);
  expect(screen.queryByRole("button", { name: /example scenario/i })).not.toBeInTheDocument();
});

it("buildExampleScenario returns level_ladder scenario with zoom", () => {
  expect(buildExampleScenario(baseForm)).toBe(
    "If a Level 1 student is absent, they attend a Zoom session. Non-top students sit in the next higher level. Top-level students sit in the level below."
  );
});

it("buildExampleScenario returns level_ladder scenario with physical", () => {
  const form = { ...baseForm, predicate: { level_1_action: "physical", min_level_for_sit_lower: 3 } };
  expect(buildExampleScenario(form)).toBe(
    "If a Level 1 student is absent, they attend a physical class. Non-top students sit in the next higher level. Top-level students sit in the level below."
  );
});

it("buildExampleScenario returns cross_section scenario", () => {
  const form: SitInRuleCreateInput = {
    ...baseForm,
    type: "cross_section",
    predicate: { section_match: "cross_section", occurrence_match: "same_occurrence_number", day_match: "same_day", last_class_excluded: true },
  };
  expect(buildExampleScenario(form)).toBe(
    "Students can sit in a different section's session on the same day (except the last session)."
  );
});

it("buildExampleScenario returns any_day_except_last scenario", () => {
  const form: SitInRuleCreateInput = {
    ...baseForm,
    type: "any_day_except_last",
    predicate: { day_match: "any_day", last_class_excluded: true },
  };
  expect(buildExampleScenario(form)).toBe(
    "Students can sit in any session on any day, except the final class of the course."
  );
});

it("buildExampleScenario returns rank_chain scenario", () => {
  const form: SitInRuleCreateInput = {
    ...baseForm,
    type: "rank_chain",
    predicate: { chains: [{ from: 1, to: 2 }] },
  };
  expect(buildExampleScenario(form)).toBe("Rank chain allowlist: Rank 1 → Rank 2.");
});

it("buildExampleScenario returns rank_chain empty message", () => {
  const form: SitInRuleCreateInput = {
    ...baseForm,
    type: "rank_chain",
    predicate: { chains: [] },
  };
  expect(buildExampleScenario(form)).toBe("No rank chains configured yet.");
});

it("buildExampleScenario returns teacher_case_by_case scenario", () => {
  const form: SitInRuleCreateInput = {
    ...baseForm,
    type: "teacher_case_by_case",
    predicate: { auto_assign: false, requires_teacher_approval: true },
  };
  expect(buildExampleScenario(form)).toBe(
    "Sit-in requests are assigned manually and requires teacher approval."
  );
});
