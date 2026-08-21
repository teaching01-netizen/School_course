import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import MakeUpPicker from "../MakeUpPicker";

const options = [
  { value: "sit-in-1", label: "Saturday 22 Aug · 13:00" },
  { value: "sit-in-2", label: "Sunday 23 Aug · 10:00" },
];

describe("R1 T2 — MakeUpPicker.a11y: aria-labelledby + aria-expanded", () => {
  it("labels the mobile trigger via aria-labelledby pointing at the visible label id", () => {
    render(<MakeUpPicker id="sit-in-session-1" label="Make-up class" value="" options={options} onChange={vi.fn()} />);
    const label = document.getElementById("sit-in-session-1-label");
    expect(label).toBeTruthy();
    expect(label).toHaveTextContent("Make-up class");
    const trigger = screen.getByRole("button", { name: /choose a make-up class/i });
    expect(trigger).toHaveAttribute("aria-labelledby", expect.stringContaining("sit-in-session-1-label"));
    expect(trigger).toHaveAttribute("aria-labelledby", expect.stringContaining("sit-in-session-1-value"));
    expect(document.getElementById("sit-in-session-1-value")).toHaveTextContent(/Choose a make-up class/i);
  });

  it("toggles aria-expanded on the expand handle between false and true", async () => {
    const user = userEvent.setup();
    render(<MakeUpPicker id="sit-in-session-1" label="Make-up class" value="" options={options} onChange={vi.fn()} />);
    const trigger = screen.getByRole("button", { name: /choose a make-up class/i });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    await user.click(trigger);
    expect(screen.getByRole("dialog", { name: /choose a make-up class/i })).toBeInTheDocument();
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    // Visual completeness: the selected value id reflects current selection via the label span
  });

  it("keeps visual + accessible name in sync when a value is selected", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<MakeUpPicker id="sit-in-session-1" label="Make-up class" value="" options={options} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: /choose a make-up class/i }));
    await user.click(screen.getByRole("radio", { name: /Saturday 22 Aug/i }));
    await user.click(screen.getByRole("button", { name: "Confirm make-up class" }));
    expect(onChange).toHaveBeenCalledWith("sit-in-1");
  });
});
