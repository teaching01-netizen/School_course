import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import ReasonField from "../ReasonField";

function ControlledReason({ error }: { error?: string | null }) {
  const [value, setValue] = useState("");
  return <ReasonField value={value} onChange={setValue} error={error} />;
}

describe("ReasonField", () => {
  it("keeps a visible label, a 500-character boundary, and a live count", async () => {
    const user = userEvent.setup();
    render(<ControlledReason />);

    const reason = screen.getByRole("textbox", { name: /reason for absence/i });
    expect(reason).toHaveAttribute("maxLength", "500");
    await user.type(reason, "Medical appointment");
    expect(screen.getByText("19/500 characters")).toBeInTheDocument();
  });

  it("associates validation errors with the textarea", () => {
    render(<ControlledReason error="Please tell us why you'll be away." />);

    const reason = screen.getByRole("textbox", { name: /reason for absence/i });
    const error = screen.getByText("Please tell us why you'll be away.");
    expect(reason).toHaveAttribute("aria-describedby", error.id);
  });
});
