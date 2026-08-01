import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ImpactAcknowledgementModal from "./ImpactAcknowledgementModal";

it("shows affected count and allows saving immediately", async () => {
  const onConfirm = vi.fn();
  const user = userEvent.setup();
  render(<ImpactAcknowledgementModal summary={{ direct_sit_in_assignments: 1, short_notice: true }} onBack={() => {}} onConfirm={onConfirm} />);

  expect(screen.getByRole("button", { name: /Save change and review/ })).not.toBeDisabled();
  expect(screen.getByText(/may affect 1 student arrangement/)).toBeInTheDocument();
  expect(screen.getByText(/Students will not be contacted until an administrator reviews/)).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: /Save change and review/ }));
  expect(onConfirm).toHaveBeenCalledOnce();
});

it("computes total affected from all summary fields", () => {
  render(
    <ImpactAcknowledgementModal
      summary={{ direct_sit_in_assignments: 2, missed_session_references: 1, predicted_student_overlaps: 3, potential_eligibility_changes: 1 }}
      onBack={() => {}}
      onConfirm={() => {}}
    />
  );
  expect(screen.getByText(/may affect 7 student arrangements/)).toBeInTheDocument();
});

it("calls onBack when back button is clicked", async () => {
  const onBack = vi.fn();
  const user = userEvent.setup();
  render(<ImpactAcknowledgementModal summary={{}} onBack={onBack} onConfirm={() => {}} />);
  await user.click(screen.getByRole("button", { name: "Back to editing" }));
  expect(onBack).toHaveBeenCalledOnce();
});

it("renders each summary row with singular and plural copy", () => {
  render(
    <ImpactAcknowledgementModal
      summary={{ direct_sit_in_assignments: 1, missed_session_references: 2, predicted_student_overlaps: 1, potential_eligibility_changes: 2 }}
      onBack={() => {}}
      onConfirm={() => {}}
    />
  );
  expect(screen.getByText("1 sit-in arrangement will be reviewed")).toBeInTheDocument();
  expect(screen.getByText("2 missed-session references may change")).toBeInTheDocument();
  expect(screen.getByText("1 possible student timetable overlap need review")).toBeInTheDocument();
  expect(screen.getByText("2 eligibility checks may change")).toBeInTheDocument();
});

it("pluralises the sit-in arrangement row", () => {
  render(<ImpactAcknowledgementModal summary={{ direct_sit_in_assignments: 2 }} onBack={() => {}} onConfirm={() => {}} />);
  expect(screen.getByText("2 sit-in arrangements will be reviewed")).toBeInTheDocument();
});

it("covers the remaining singular and plural variants", () => {
  render(
    <ImpactAcknowledgementModal
      summary={{ missed_session_references: 1, predicted_student_overlaps: 2, potential_eligibility_changes: 1 }}
      onBack={() => {}}
      onConfirm={() => {}}
    />
  );
  expect(screen.getByText("1 missed-session reference may change")).toBeInTheDocument();
  expect(screen.getByText("2 possible student timetable overlaps need review")).toBeInTheDocument();
  expect(screen.getByText("1 eligibility check may change")).toBeInTheDocument();
});

it("renders the short-notice row", () => {
  render(<ImpactAcknowledgementModal summary={{ short_notice: true }} onBack={() => {}} onConfirm={() => {}} />);
  expect(screen.getByText("This is a short-notice change and affected students may need prompt contact")).toBeInTheDocument();
});

it("shows the fallback row and zero total for an empty summary", () => {
  render(<ImpactAcknowledgementModal summary={{}} onBack={() => {}} onConfirm={() => {}} />);
  expect(screen.getByText("Affected student arrangements will be checked after this change is saved")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Save change and review 0" })).toBeInTheDocument();
});

it("disables both actions while saving", () => {
  render(<ImpactAcknowledgementModal summary={{ direct_sit_in_assignments: 1 }} saving onBack={() => {}} onConfirm={() => {}} />);

  const backButton = screen.getByRole("button", { name: "Back to editing" });
  const saveButton = screen.getByRole("button", { name: /Save change and review/ });
  expect(backButton).toBeDisabled();
  expect(saveButton).toBeDisabled();
});

it("shows the computed total on the save button", () => {
  render(<ImpactAcknowledgementModal summary={{ direct_sit_in_assignments: 3, predicted_student_overlaps: 4 }} onBack={() => {}} onConfirm={() => {}} />);
  expect(screen.getByRole("button", { name: "Save change and review 7" })).toBeInTheDocument();
});
