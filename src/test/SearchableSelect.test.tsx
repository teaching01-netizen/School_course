import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import SearchableSelect from "@/components/ui/SearchableSelect";

describe("SearchableSelect", () => {
  it("filters options in the popover and commits the chosen value", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    function ControlledSearchableSelect() {
      const [value, setValue] = useState("");
      return (
        <SearchableSelect
          value={value}
          onChange={(event) => {
            onChange(event);
            setValue(event.target.value);
          }}
          aria-label="Course"
        >
          <option value="math">Mathematics</option>
          <option value="physics">Physics</option>
        </SearchableSelect>
      );
    }

    render(<ControlledSearchableSelect />);

    await user.click(screen.getByRole("combobox", { name: "Course" }));
    const popover = screen.getByRole("dialog", { name: "Search options…" });
    await user.type(within(popover).getByRole("textbox", { name: "Search options…" }), "phys");

    expect(within(popover).getByRole("option", { name: "Physics" })).toBeInTheDocument();
    expect(within(popover).queryByRole("option", { name: "Mathematics" })).not.toBeInTheDocument();

    await user.click(within(popover).getByRole("option", { name: "Physics" }));

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange.mock.calls[0]?.[0]?.target.value).toBe("physics");
  });

  it("shows long course names in full instead of truncating them", async () => {
    const user = userEvent.setup();
    const longName =
      "IELTS Intensive Preparation Course for University Admission — Saturday Morning Cohort 2026";

    render(
      <SearchableSelect
        value=""
        aria-label="Course"
        options={[{ value: "ielts", label: longName }]}
      />,
    );

    await user.click(screen.getByRole("combobox", { name: "Course" }));
    const popover = screen.getByRole("dialog", { name: "Search options…" });

    const option = within(popover).getByRole("option", { name: longName });
    expect(option).toHaveTextContent(longName);

    // The label wraps (break-words) rather than clipping with truncate, and
    // the panel grows with its content (w-max capped by max-w) so long
    // course names stay readable.
    const label = option.querySelector("span > span");
    expect(label?.className).toContain("break-words");
    expect(label?.className).not.toContain("truncate");
    expect(popover.className).toContain("w-max");
    expect(popover.className).toContain("max-w-");
  });
});
