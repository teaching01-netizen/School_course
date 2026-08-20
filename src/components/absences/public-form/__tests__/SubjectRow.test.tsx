import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import SubjectRow from "../SubjectRow";

function ControlledSubject() {
  const [selected, setSelected] = useState(false);
  return (
    <SubjectRow
      id="subject-math"
      name="A very long subject name that should wrap instead of overflowing"
      selected={selected}
      onToggle={() => setSelected((current) => !current)}
    />
  );
}

describe("SubjectRow", () => {
  it("uses a real checkbox and lets the entire row toggle it", async () => {
    const user = userEvent.setup();
    render(<ControlledSubject />);

    const checkbox = screen.getByRole("checkbox", { name: /very long subject name/i });
    expect(checkbox).toHaveAttribute("id", "subject-subject-math");
    expect(checkbox).not.toBeDisabled();

    await user.click(screen.getByText(/very long subject name/i));
    expect(checkbox).toBeChecked();
  });
});
