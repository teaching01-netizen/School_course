import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import MakeUpPicker from "../MakeUpPicker";

const options = [
  { value: "sit-in-1", label: "Saturday 22 Aug · 13:00" },
  { value: "sit-in-2", label: "Sunday 23 Aug · 10:00" },
];

describe("MakeUpPicker", () => {
  it("opens the public picker for browsing and cancels staged changes without committing", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<MakeUpPicker publicForm id="make-up" label="Make-up class" value="sit-in-1" options={options} onChange={onChange} context="For Mathematics · Mon, 19 Oct" />);
    const trigger = screen.getByRole("button", { name: /saturday 22 aug/i });
    await user.click(trigger);
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("heading")).toHaveFocus();
    expect(dialog).toHaveAccessibleDescription("For Mathematics · Mon, 19 Oct");
    await user.click(within(dialog).getByRole("radio", { name: /sunday/i }));
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(onChange).not.toHaveBeenCalled();
    expect(trigger).toHaveFocus();
    await user.click(trigger);
    expect(within(screen.getByRole("dialog")).getByRole("radio", { name: /saturday/i })).toBeChecked();
    fireEvent(screen.getByRole("dialog"), new Event("cancel", { cancelable: true }));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("keeps a filtered-out staged choice visible in the confirmation summary", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<MakeUpPicker publicForm id="make-up" label="Make-up class" value="" options={options} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: /choose a make-up/i }));
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("radio", { name: /sunday/i }));
    await user.type(within(dialog).getByRole("textbox"), "zzz");
    expect(within(dialog).getByText(/selected: sunday 23 aug/i)).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Confirm make-up class" }));
    expect(onChange).toHaveBeenCalledExactlyOnceWith("sit-in-2");
  });

  it.each(["removed", "disabled"])("rejects a staged choice that becomes %s while the sheet is open", async (change) => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const props = { publicForm: true, id: "make-up", label: "Make-up class", value: "", options, onChange };
    const { rerender } = render(<MakeUpPicker {...props} />);
    await user.click(screen.getByRole("button", { name: /choose a make-up/i }));
    await user.click(screen.getByRole("radio", { name: /sunday/i }));
    rerender(<MakeUpPicker {...props} options={change === "removed" ? [options[0]] : options.map((option) => ({ ...option, disabled: option.value === "sit-in-2" }))} />);
    const confirm = screen.getByRole("button", { name: "Confirm make-up class" });
    expect(confirm).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent(/no longer available/i);
    await user.click(confirm);
    expect(onChange).not.toHaveBeenCalled();
  });

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

  it("shows the overlap reason and disables the conflicting session", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <MakeUpPicker
        id="sit-in-session-1"
        label="Make-up class"
        value=""
        options={[{
          value: "sit-in-conflict",
          label: "SAT Verbal Reading Rank 4 C3",
          details: "AJ. NICE · Sun, 27 Sep · 09:00–12:20",
          disabled: true,
          description: "You already have another class at this time.",
          conflictDetails: "Chemistry Lab C3 · Sun, 27 Sep · 10:00–11:30",
        }]}
        onChange={onChange}
      />,
    );

    const nativeOption = screen.getByRole("option", { name: /sat verbal reading rank 4 c3.*aj\. nice.*sun, 27 sep/i });
    expect(nativeOption).toBeDisabled();
    expect(nativeOption).toHaveTextContent(/You already have another class at this time/);
    await user.click(screen.getByRole("button", { name: /choose a make-up class/i }));
    expect(screen.getByText("You already have another class at this time.")).toBeInTheDocument();
    expect(screen.getByText("Chemistry Lab C3 · Sun, 27 Sep · 10:00–11:30")).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /sat verbal reading rank 4 c3/i })).toBeDisabled();
    expect(screen.queryByText(/submission will be blocked/i)).not.toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
  });
});
