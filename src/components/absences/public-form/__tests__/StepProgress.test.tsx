import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import StepProgress from "../StepProgress";

const steps = [
  { label: "Student", description: "Confirm your profile" },
  { label: "Verify", description: "Parent confirmation" },
  { label: "Classes", description: "Select classes and make-up" },
  { label: "Review", description: "Confirm and submit" },
];

describe("StepProgress", () => {
  it("renders numbered steps without a separate progress bar", () => {
    const onStepClick = vi.fn();
    render(<StepProgress steps={steps} currentStep={2} onStepClick={onStepClick} />);

    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    expect(screen.getByText("Step 3 of 4: Classes")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Classes.*current/i })).toHaveTextContent("3");
    expect(screen.getByRole("button", { name: /Student.*completed/i })).toHaveTextContent("1");
    expect(screen.getByRole("button", { name: /Review/i })).toHaveTextContent("4");
  });

  it("prevents jumping to future steps", () => {
    const onStepClick = vi.fn();
    render(<StepProgress steps={steps} currentStep={2} onStepClick={onStepClick} />);

    expect(screen.getByRole("button", { name: /Classes.*current/i })).toHaveAttribute("tabindex", "-1");
    expect(screen.getByRole("button", { name: /Student.*completed/i })).toBeEnabled();
    expect(screen.getByRole("button", { name: /Review/i })).toBeDisabled();
  });

  it("allows completed steps to be revisited", async () => {
    const user = userEvent.setup();
    const onStepClick = vi.fn();
    render(<StepProgress steps={steps} currentStep={2} onStepClick={onStepClick} />);

    await user.click(screen.getByRole("button", { name: /Verify.*completed/i }));
    expect(onStepClick).toHaveBeenCalledWith(1);
  });
});
