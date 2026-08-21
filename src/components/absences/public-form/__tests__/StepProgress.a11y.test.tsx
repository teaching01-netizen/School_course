import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import StepProgress from "../StepProgress";

const steps = [
  { label: "Student", description: "Confirm your profile" },
  { label: "Verify", description: "Parent confirmation" },
  { label: "Classes", description: "Select classes and make-up" },
  { label: "Review", description: "Confirm and submit" },
];

describe("R1 T1 — StepProgress.a11y: aria-current + Check SVG", () => {
  it("marks only the active step with aria-current=step and an accessible name", () => {
    render(<StepProgress steps={steps} currentStep={1} onStepClick={() => {}} />);
    const currents = document.querySelectorAll('[aria-current="step"]');
    expect(currents).toHaveLength(1);
    expect(currents[0]).toHaveAccessibleName(/Verify.*current/i);
  });

  it("renders a check SVG for completed steps and the index for current and future steps", () => {
    const { container } = render(<StepProgress steps={steps} currentStep={2} onStepClick={() => {}} />);
    // Completed step 0 shows a check, not the digit "1"
    const completedBtn = screen.getByRole("button", { name: /Student.*completed/i });
    expect(completedBtn.querySelector("svg")).toBeTruthy();
    expect(completedBtn).toHaveAccessibleName(/Student.*completed/i);

    // Current step shows its index and aria-current
    expect(screen.getByRole("button", { name: /Classes.*current/i })).toHaveAttribute("aria-current", "step");
    expect(screen.getByRole("button", { name: /Classes.*current/i })).toHaveTextContent("3");

    // Future step shows its index without a check
    expect(screen.getByRole("button", { name: /Review/i })).toHaveTextContent("4");
    expect(container.querySelectorAll('svg[aria-hidden="true"]').length).toBeGreaterThanOrEqual(1);
  });

  it("wraps steps in nav[aria-label=Progress] > ol[role=list] with visual-completeness guards", () => {
    const { container } = render(<StepProgress steps={steps} currentStep={2} onStepClick={() => {}} />);
    expect(screen.getByRole("navigation", { name: "Progress" })).toBeInTheDocument();
    expect(container.querySelector('ol[role="list"]')).toBeTruthy();
    expect(screen.getByText("Step 3 of 4: Classes")).toBeInTheDocument();
  });

  it("uses token vars and 150ms ease-ui transitions", () => {
    render(<StepProgress steps={steps} currentStep={1} onStepClick={() => {}} />);
    const btn = screen.getByRole("button", { name: /Student.*completed/i });
    // Gate: the component className must reference the R3 motion contract rather than hard-coding durations
    expect(btn.className).toMatch(/duration-\[150ms\]/);
    expect(btn.className).toMatch(/ease-\[var\(--ease-ui\)\]/);
    expect(btn.className).toMatch(/var\(--color-wi-primary\)/);
  });
});
