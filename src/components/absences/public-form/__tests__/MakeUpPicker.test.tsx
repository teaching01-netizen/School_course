import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { SitInDisplayModel } from "@/features/absences/domain/sitInResolution";
import MakeUpPicker from "../MakeUpPicker";

const options = [
  { value: "sit-in-1", label: "Saturday 22 Aug · 13:00" },
  { value: "sit-in-2", label: "Sunday 23 Aug · 10:00" },
];

const onDay = (dateKey: string, date: string, startTime = "09:00", endTime = "10:00"): SitInDisplayModel => ({
  className: "Calculus III",
  subjectName: "Mathematics",
  teacherName: "AJ. Nice",
  date,
  dateKey,
  startTime,
  endTime,
  status: "available",
});

const dayOptions = [
  { value: "day-17", label: "Calculus III", details: "AJ. Nice · 17 Jan · 09:00–10:00", display: onDay("2026-01-17", "17 Jan") },
  { value: "day-18", label: "Calculus III", details: "AJ. Nice · 18 Jan · 10:00–11:00", display: onDay("2026-01-18", "18 Jan", "10:00", "11:00") },
  { value: "day-19", label: "Calculus III", details: "AJ. Nice · 19 Jan · 13:00–14:00", display: onDay("2026-01-19", "19 Jan", "13:00", "14:00") },
];

describe("MakeUpPicker", () => {
  it("preserves the confirmed and staged choices while more times load, fail, and append", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const onLoad = vi.fn();
    const props = { publicForm: true, id: "make-up", label: "Make-up class", value: "sit-in-1", options, onChange };
    const { rerender } = render(<MakeUpPicker {...props} moreTimes={{ hasMore: true, loading: false, onLoad }} />);
    await user.click(screen.getByRole("button", { name: /saturday 22 aug/i }));
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("radio", { name: /sunday/i }));
    await user.click(within(dialog).getByRole("button", { name: "See other times" }));
    expect(onLoad).toHaveBeenCalledOnce();
    rerender(<MakeUpPicker {...props} moreTimes={{ hasMore: true, loading: true, onLoad }} />);
    expect(within(dialog).getByRole("button", { name: "Loading other times…" })).toBeDisabled();
    expect(within(dialog).getByRole("radio", { name: /sunday/i })).toBeChecked();
    rerender(<MakeUpPicker {...props} moreTimes={{ hasMore: true, loading: false, error: "Couldn’t load more times.", onLoad }} />);
    expect(within(dialog).getByText("Couldn’t load more times.")).toBeInTheDocument();
    expect(within(dialog).getByRole("radio", { name: /sunday/i })).toBeChecked();
    rerender(<MakeUpPicker {...props} options={[...options, { value: "third", label: "Monday 24 Aug" }]} moreTimes={{ hasMore: false, loading: false, onLoad }} />);
    expect(within(dialog).getByRole("radio", { name: /monday/i })).toBeInTheDocument();
    expect(within(dialog).getByRole("radio", { name: /sunday/i })).toBeChecked();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(onChange).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: /saturday 22 aug/i }));
    expect(within(screen.getByRole("dialog")).getByRole("radio", { name: /saturday/i })).toBeChecked();
  });

  it("stages public removal as an action and allows cancellation before confirming", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<MakeUpPicker publicForm id="make-up" label="Make-up class" value="sit-in-1" options={options} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: /saturday 22 aug/i }));
    let dialog = screen.getByRole("dialog");
    expect(within(dialog).queryByRole("radio", { name: /clear|remove|not yet/i })).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Remove make-up selection" }));
    expect(onChange).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await user.click(screen.getByRole("button", { name: /saturday 22 aug/i }));
    dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Remove make-up selection" }));
    await user.click(within(dialog).getByRole("button", { name: "Remove make-up selection" }));
    expect(onChange).toHaveBeenCalledExactlyOnceWith("");
  });

  it("keeps an invalidated public choice visible and requires a replacement", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const props = { publicForm: true, id: "make-up", label: "Make-up class", value: "sit-in-1", options, onChange };
    const { rerender } = render(<MakeUpPicker {...props} />);
    rerender(<MakeUpPicker {...props} options={options.map((option) => ({ ...option, disabled: option.value === "sit-in-1", description: option.value === "sit-in-1" ? "Overlaps Chemistry" : undefined }))} />);
    expect(onChange).not.toHaveBeenCalled();
    const trigger = screen.getByRole("button", { name: /saturday 22 aug/i });
    expect(trigger).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("status")).toHaveTextContent("Overlaps Chemistry");
    await user.click(trigger);
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("button", { name: "Confirm make-up class" })).toBeDisabled();
    await user.click(within(dialog).getByRole("radio", { name: /sunday/i }));
    await user.click(within(dialog).getByRole("button", { name: "Confirm make-up class" }));
    expect(onChange).toHaveBeenCalledExactlyOnceWith("sit-in-2");
  });

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

  it("keeps a staged choice visible in the confirmation summary after switching days", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<MakeUpPicker publicForm id="make-up" label="Make-up class" value="" options={dayOptions} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: /choose a make-up/i }));
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("radio", { name: "Sunday 18 January" }));
    await user.click(within(dialog).getByRole("radio", { name: /18 Jan · 10:00–11:00/ }));
    await user.click(within(dialog).getByRole("radio", { name: "Saturday 17 January" }));
    expect(within(dialog).queryByRole("radio", { name: /18 Jan · 10:00–11:00/ })).not.toBeInTheDocument();
    expect(within(dialog).getByText(/selected: 18 Jan · 10:00–11:00/i)).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Confirm make-up class" }));
    expect(onChange).toHaveBeenCalledExactlyOnceWith("day-18");
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

  it("filters the listed times to the chosen day and marks a day with no available times", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const withReason = [
      { value: "day-17", label: "Calculus III", details: "AJ. Nice · 17 Jan · 09:00–10:00", display: onDay("2026-01-17", "17 Jan") },
      {
        value: "overlap-19", label: "Chemistry", details: "AJ. Nice · 19 Jan · 13:00–14:00", display: onDay("2026-01-19", "19 Jan", "13:00", "14:00"),
        disabled: true, description: "Overlaps another scheduled class", conflictDetails: "Chemistry Lab C3 · Mon, 19 Jan · 13:00–14:30",
      },
    ];
    render(<MakeUpPicker publicForm id="make-up" label="Make-up class" value="" options={withReason} onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: /choose a make-up/i }));
    const dialog = screen.getByRole("dialog");

    expect(within(dialog).getByRole("radiogroup", { name: "Make-up date" })).toBeInTheDocument();
    expect(within(dialog).queryByRole("radio", { name: /19 Jan · 13:00–14:00/ })).not.toBeInTheDocument();

    await user.click(within(dialog).getByRole("radio", { name: "Monday 19 January — no times available" }));

    expect(within(dialog).getByRole("radio", { name: /19 Jan · 13:00–14:00/ })).toBeDisabled();
    expect(within(dialog).getByText("Overlaps another scheduled class")).toBeInTheDocument();
    expect(within(dialog).getByText("Chemistry Lab C3 · Mon, 19 Jan · 13:00–14:30")).toBeInTheDocument();
    expect(within(dialog).getByText(/No times available on Mon,\s+19\s+Jan\s+2026\./)).toBeInTheDocument();
    expect(within(dialog).queryByText(/None of these times are available/)).not.toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Confirm make-up class" })).toBeDisabled();
  });

  it("shows a single-day caption instead of chips when every time is on one day", async () => {
    const user = userEvent.setup();
    render(<MakeUpPicker publicForm id="make-up" label="Make-up class" value="" options={[dayOptions[0]]} onChange={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: /choose a make-up/i }));
    const dialog = screen.getByRole("dialog");

    expect(within(dialog).queryByRole("radiogroup")).not.toBeInTheDocument();
    expect(within(dialog).getByText(/Sat,\s+17\s+Jan\s+2026/)).toBeInTheDocument();
    expect(within(dialog).getByRole("radio", { name: /17 Jan · 09:00–10:00/ })).toBeInTheDocument();
  });

  it("keeps a flat list without a day strip for options without day keys", async () => {
    const user = userEvent.setup();
    render(<MakeUpPicker publicForm id="make-up" label="Make-up class" value="" options={options} onChange={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: /choose a make-up/i }));
    const dialog = screen.getByRole("dialog");

    expect(within(dialog).queryByRole("radiogroup")).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("textbox")).not.toBeInTheDocument();
    expect(within(dialog).getByRole("radio", { name: /sunday 23 aug/i })).toBeInTheDocument();
  });

  it("switches to the day revealed by See other times and focuses its first available time", async () => {
    const user = userEvent.setup();
    const onLoad = vi.fn();
    const props = { publicForm: true, id: "make-up", label: "Make-up class", value: "", onChange: vi.fn() };
    const { rerender } = render(<MakeUpPicker {...props} options={[dayOptions[0]]} moreTimes={{ hasMore: true, loading: false, onLoad }} />);
    await user.click(screen.getByRole("button", { name: /choose a make-up/i }));
    const dialog = screen.getByRole("dialog");

    await user.click(within(dialog).getByRole("button", { name: "See other times" }));
    expect(onLoad).toHaveBeenCalledOnce();
    rerender(<MakeUpPicker {...props} options={dayOptions} moreTimes={{ hasMore: true, loading: false, onLoad }} />);

    await waitFor(() => expect(within(dialog).getByRole("radio", { name: "Sunday 18 January" })).toBeChecked());
    expect(within(dialog).getByRole("radio", { name: /18 Jan · 10:00–11:00/ })).toHaveFocus();
  });

  it("falls back to an available day when a reveal drops the day being shown", async () => {
    const user = userEvent.setup();
    const props = { publicForm: true, id: "make-up", label: "Make-up class", value: "", onChange: vi.fn() };
    const { rerender } = render(<MakeUpPicker {...props} options={dayOptions} />);
    await user.click(screen.getByRole("button", { name: /choose a make-up/i }));
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("radio", { name: "Monday 19 January" }));
    expect(within(dialog).getByRole("radio", { name: /19 Jan · 13:00–14:00/ })).toBeInTheDocument();

    // The next batch replaces the option set and no longer offers that day.
    rerender(<MakeUpPicker {...props} options={[dayOptions[0]]} />);

    expect(within(dialog).queryByRole("radiogroup")).not.toBeInTheDocument();
    expect(within(dialog).getByText(/Sat,\s+17\s+Jan\s+2026/)).toBeInTheDocument();
    expect(within(dialog).getByRole("radio", { name: /17 Jan · 09:00–10:00/ })).toBeInTheDocument();
  });

  it("offers See other times inside the sheet when the visible times are blocked", async () => {
    const user = userEvent.setup();
    const onLoad = vi.fn();
    const onChange = vi.fn();
    render(
      <MakeUpPicker
        id="sit-in-staff"
        label="Make-up class"
        value=""
        options={[{ value: "used", label: "SAT Verbal Rank 2 C3", details: "AJ. NICE · Sun, 8 Nov · 17:00–20:20", disabled: true, description: "You already chose this make-up class for another absence. Choose a different time." }]}
        onChange={onChange}
        moreTimes={{ hasMore: true, loading: false, onLoad }}
      />,
    );
    await user.click(screen.getByRole("button", { name: /choose a make-up class/i }));
    const dialog = screen.getByRole("dialog");

    expect(within(dialog).getByRole("button", { name: "Confirm make-up class" })).toBeDisabled();
    // The reveal lives inside the sheet, so staff never have to close the
    // dialog and hunt for the action on the class card behind it.
    const seeOtherTimes = within(dialog).getByRole("button", { name: "See other times" });
    expect(dialog.querySelector(".absence-mobile-sheet__body")).toContainElement(seeOtherTimes);

    await user.click(seeOtherTimes);
    expect(onLoad).toHaveBeenCalledOnce();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("gives the staff sheet the same day strip without a search box", async () => {
    const user = userEvent.setup();
    render(<MakeUpPicker id="sit-in-staff" label="Make-up class" value="" options={dayOptions} onChange={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: /choose a make-up class/i }));
    const dialog = screen.getByRole("dialog");

    expect(within(dialog).queryByRole("textbox")).not.toBeInTheDocument();
    expect(within(dialog).getByRole("radiogroup", { name: "Make-up date" })).toBeInTheDocument();
    expect(within(dialog).getByRole("radio", { name: "Saturday 17 January" })).toHaveFocus();
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
