import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import RuleSelector from "../RuleSelector";
import type { SitInRule } from "../../types";

const makeRule = (overrides: Partial<SitInRule> = {}): SitInRule => ({
  id: overrides.id ?? "rule-1",
  name: overrides.name ?? "Level 1→2 Ladder",
  type: overrides.type ?? "level_ladder",
  predicate: overrides.predicate ?? {},
  description: overrides.description ?? "Students at level 1 may sit into level 2 sessions.",
  created_at: overrides.created_at ?? "2025-01-01T00:00:00Z",
  updated_at: overrides.updated_at ?? "2025-01-01T00:00:00Z",
});

const LEVEL_2_RULE = makeRule({
  id: "rule-2",
  name: "Cross-Section Bridge",
  type: "cross_section",
  description: "Students may sit into any cross-section group.",
});

const rules: SitInRule[] = [makeRule(), LEVEL_2_RULE];

it("renders with no rules", () => {
  render(<RuleSelector rules={[]} value={null} onChange={vi.fn()} />);
  const select = screen.getByRole("combobox");
  expect(select).toBeInTheDocument();
  expect(select).toHaveValue("");
  expect(screen.getByText("No rule assigned")).toBeInTheDocument();
});

it("renders rules with type badges in optgroups", () => {
  render(<RuleSelector rules={rules} value={null} onChange={vi.fn()} />);
  const select = screen.getByRole("combobox");
  const optgroups = select.querySelectorAll("optgroup");
  expect(optgroups).toHaveLength(2);
  expect(optgroups[0]).toHaveAttribute("label", "Level Ladder");
  expect(optgroups[1]).toHaveAttribute("label", "Cross-Section");
  const options = select.querySelectorAll("option");
  expect(options[1]).toHaveTextContent("Level 1→2 Ladder — Level Ladder");
  expect(options[2]).toHaveTextContent("Cross-Section Bridge — Cross-Section");
});

it("shows description when a rule is selected", () => {
  render(<RuleSelector rules={rules} value="rule-1" onChange={vi.fn()} />);
  expect(
    screen.getByText("Students at level 1 may sit into level 2 sessions."),
  ).toBeInTheDocument();
});

it("does not show description when no rule is selected", () => {
  render(<RuleSelector rules={rules} value={null} onChange={vi.fn()} />);
  expect(
    screen.queryByText("Students at level 1 may sit into level 2 sessions."),
  ).not.toBeInTheDocument();
});

it("shows amber warning when no rule is assigned", () => {
  render(<RuleSelector rules={rules} value={null} onChange={vi.fn()} />);
  expect(
    screen.getByText("No sit-in rule — students cannot sit into this group's sessions"),
  ).toBeInTheDocument();
});

it("does not show warning when a rule is assigned", () => {
  render(<RuleSelector rules={rules} value="rule-1" onChange={vi.fn()} />);
  expect(
    screen.queryByText("No sit-in rule — students cannot sit into this group's sessions"),
  ).not.toBeInTheDocument();
});

it("calls onChange with rule id on selection", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(<RuleSelector rules={rules} value={null} onChange={onChange} />);
  const select = screen.getByRole("combobox");
  await user.selectOptions(select, "rule-2");
  expect(onChange).toHaveBeenCalledWith("rule-2");
});

it("calls onChange with null when no rule selected", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(<RuleSelector rules={rules} value="rule-1" onChange={onChange} />);
  const select = screen.getByRole("combobox");
  await user.selectOptions(select, "");
  expect(onChange).toHaveBeenCalledWith(null);
});

it("disables the select when disabled prop is true", () => {
  render(<RuleSelector rules={rules} value={null} onChange={vi.fn()} disabled />);
  expect(screen.getByRole("combobox")).toBeDisabled();
});

it("enables the select by default", () => {
  render(<RuleSelector rules={rules} value={null} onChange={vi.fn()} />);
  expect(screen.getByRole("combobox")).toBeEnabled();
});
