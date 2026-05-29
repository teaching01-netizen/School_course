import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import RulePredicateForm from "../RulePredicateForm";

it("renders level_ladder fields with defaults", () => {
  render(
    <RulePredicateForm
      ruleType="level_ladder"
      predicate={{}}
      onChange={vi.fn()}
    />
  );
  expect(screen.getByRole("combobox", { name: /level 1 action/i })).toBeInTheDocument();
  expect(screen.getByRole("spinbutton", { name: /minimum level for sit-down/i })).toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: /level 1 action/i })).toHaveValue("zoom");
  expect(screen.getByRole("spinbutton", { name: /minimum level for sit-down/i })).toHaveValue(2);
});

it("level_ladder reflects provided predicate values", () => {
  render(
    <RulePredicateForm
      ruleType="level_ladder"
      predicate={{ level_1_action: "physical", min_level_for_sit_lower: 3 }}
      onChange={vi.fn()}
    />
  );
  expect(screen.getByRole("combobox", { name: /level 1 action/i })).toHaveValue("physical");
  expect(screen.getByRole("spinbutton", { name: /minimum level for sit-down/i })).toHaveValue(3);
});

it("level_ladder default uses min_level_for_sit_lower key (not min_level)", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(
    <RulePredicateForm
      ruleType="level_ladder"
      predicate={{}}
      onChange={onChange}
    />
  );
  await user.clear(screen.getByRole("spinbutton", { name: /minimum level for sit-down/i }));
  await user.type(screen.getByRole("spinbutton", { name: /minimum level for sit-down/i }), "4");
  expect(onChange).toHaveBeenCalledWith(
    expect.objectContaining({ min_level_for_sit_lower: 4 })
  );
  expect(onChange).not.toHaveBeenCalledWith(
    expect.objectContaining({ min_level: expect.anything() })
  );
});

it("renders cross_section fields with defaults", () => {
  render(
    <RulePredicateForm
      ruleType="cross_section"
      predicate={{}}
      onChange={vi.fn()}
    />
  );
  expect(screen.getByRole("combobox", { name: /section match/i })).toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: /occurrence match/i })).toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: /day match/i })).toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: /last class excluded/i })).toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: /section match/i })).toHaveValue("cross_section");
  expect(screen.getByRole("combobox", { name: /occurrence match/i })).toHaveValue("same_occurrence_number");
  expect(screen.getByRole("combobox", { name: /day match/i })).toHaveValue("any");
  expect(screen.getByRole("checkbox", { name: /last class excluded/i })).toBeChecked();
});

it("renders any_day_except_last fields with defaults", () => {
  render(
    <RulePredicateForm
      ruleType="any_day_except_last"
      predicate={{}}
      onChange={vi.fn()}
    />
  );
  expect(screen.getByRole("combobox", { name: /day match/i })).toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: /last class excluded/i })).toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: /day match/i })).toHaveValue("any_day");
  expect(screen.getByRole("checkbox", { name: /last class excluded/i })).toBeChecked();
});

it("rank_chain renders add button and shared fields", () => {
  render(
    <RulePredicateForm
      ruleType="rank_chain"
      predicate={{}}
      onChange={vi.fn()}
    />
  );
  expect(screen.getByRole("button", { name: /add chain/i })).toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: /last class excluded/i })).toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: /day match/i })).toBeInTheDocument();
});

it("rank_chain adds and removes chain rows", async () => {
  const user = userEvent.setup();
  render(
    <RulePredicateForm
      ruleType="rank_chain"
      predicate={{ chains: [{ from: 1, to: 2 }] }}
      onChange={vi.fn()}
    />
  );
  expect(screen.getByRole("spinbutton", { name: /from rank/i })).toHaveValue(1);
  expect(screen.getByRole("spinbutton", { name: /to rank/i })).toHaveValue(2);
  await user.click(screen.getByRole("button", { name: /remove chain/i }));
  expect(screen.queryByRole("spinbutton", { name: /from rank/i })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: /add chain/i }));
  expect(screen.getByRole("spinbutton", { name: /from rank/i })).toBeInTheDocument();
  expect(screen.getByRole("spinbutton", { name: /to rank/i })).toBeInTheDocument();
});

it("renders teacher_case_by_case fields with defaults", () => {
  render(
    <RulePredicateForm
      ruleType="teacher_case_by_case"
      predicate={{}}
      onChange={vi.fn()}
    />
  );
  expect(screen.getByRole("checkbox", { name: /auto assign/i })).toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: /requires teacher approval/i })).toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: /auto assign/i })).not.toBeChecked();
  expect(screen.getByRole("checkbox", { name: /requires teacher approval/i })).toBeChecked();
});

it("calls onChange with updated predicate on field change", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(
    <RulePredicateForm
      ruleType="level_ladder"
      predicate={{}}
      onChange={onChange}
    />
  );
  await user.selectOptions(screen.getByRole("combobox", { name: /level 1 action/i }), "physical");
  expect(onChange).toHaveBeenCalledWith(
    expect.objectContaining({ level_1_action: "physical" })
  );
});

it("shows advanced JSON toggle and textarea", async () => {
  const user = userEvent.setup();
  render(
    <RulePredicateForm
      ruleType="level_ladder"
      predicate={{}}
      onChange={vi.fn()}
    />
  );
  const toggle = screen.getByRole("button", { name: /advanced: edit json/i });
  expect(toggle).toBeInTheDocument();
  expect(screen.queryByRole("textbox", { name: /json predicate/i })).not.toBeInTheDocument();
  await user.click(toggle);
  expect(screen.getByRole("textbox", { name: /json predicate/i })).toBeInTheDocument();
});

it("renders tooltip info icons for level_ladder fields", () => {
  render(
    <RulePredicateForm
      ruleType="level_ladder"
      predicate={{}}
      onChange={vi.fn()}
    />
  );
  const infoButtons = screen.getAllByRole("button").filter(
    (btn) => btn.querySelector(".lucide-info") !== null
  );
  expect(infoButtons.length).toBeGreaterThanOrEqual(2);
});
