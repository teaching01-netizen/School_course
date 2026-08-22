import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import MakeUpPicker from "../MakeUpPicker";

const options = [
  { value: "sit-in-1", label: "Saturday 22 Aug · 13:00" },
  { value: "sit-in-2", label: "Sunday 23 Aug · 10:00" },
];

describe("MakeUpPicker", () => {
  it("keeps a native select fallback for wide layouts", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<MakeUpPicker id="sit-in-session-1" label="Make-up class" value="" options={options} onChange={onChange} />);

    await user.selectOptions(screen.getByRole("combobox", { name: "Make-up class" }), "sit-in-1");
    expect(onChange).toHaveBeenCalledWith("sit-in-1");
  });

  it("uses a focus-managed sheet for narrow layouts", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<MakeUpPicker id="sit-in-session-1" label="Make-up class" value="" options={options} onChange={onChange} />);
    const trigger = screen.getByRole("button", { name: /choose a make-up class/i });
    expect(trigger).toHaveAttribute("data-make-up-trigger", "true");
    await user.click(trigger);
    const dialog = screen.getByRole("dialog", { name: /choose a make-up class/i });
    expect(screen.getByRole("button", { name: "Confirm make-up class" })).toBeDisabled();
    await user.click(screen.getByRole("radio", { name: /Saturday 22 Aug/i }));
    await user.click(screen.getByRole("button", { name: "Confirm make-up class" }));

    expect(dialog).not.toBeInTheDocument();
    expect(onChange).toHaveBeenCalledWith("sit-in-1");
  });

  it("lets narrow layouts clear a previous selection like the native select", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<MakeUpPicker id="sit-in-session-1" label="Make-up class" value="sit-in-1" options={options} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: /saturday 22 aug/i }));

    await user.click(screen.getByRole("radio", { name: /not yet selected/i }));
    await user.click(screen.getByRole("button", { name: "Confirm make-up class" }));

    expect(onChange).toHaveBeenCalledWith("");
  });

  it("shows an empty state when the sheet search matches nothing", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<MakeUpPicker id="sit-in-session-1" label="Make-up class" value="" options={options} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: /choose a make-up class/i }));
    await user.type(screen.getByRole("textbox", { name: /search make-up class/i }), "zzz");

    expect(screen.getByText(/no classes match/i)).toBeInTheDocument();
    expect(screen.queryByRole("radio", { name: /saturday 22 aug/i })).not.toBeInTheDocument();
  });
});
