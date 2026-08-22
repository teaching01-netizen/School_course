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
});
