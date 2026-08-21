import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import FormAlert from "../FormAlert";

describe("R1 T — FormAlert.a11y: single announcement", () => {
  it("renders exactly one role=alert wrapper with the message and is focusable via tabIndex=-1", () => {
    const { container } = render(<FormAlert message="Pick a make-up class for all selected sessions before submitting." />);
    const alerts = container.querySelectorAll('[role="alert"]');
    expect(alerts).toHaveLength(1);
    expect(screen.getByRole("alert")).toHaveTextContent(/Pick a make-up class/i);
    expect(screen.getByRole("alert")).toHaveAttribute("tabindex", "-1");
  });
});
