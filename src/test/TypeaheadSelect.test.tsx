import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import TypeaheadSelect from "@/components/TypeaheadSelect";

const options = [
  { value: "t-1", label: "pees" },
  { value: "t-2", label: "alice" },
];

describe("TypeaheadSelect", () => {
  it("commits an exact typed label on blur", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<TypeaheadSelect value="" onChange={onChange} options={options} placeholder="Search teacher…" />);

    const input = screen.getByRole("combobox");
    await user.click(input);
    await user.type(input, "pees");
    fireEvent.blur(input);

    expect(onChange).toHaveBeenCalledWith("t-1");
  });

  it("does not commit a partial typed label on blur", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<TypeaheadSelect value="" onChange={onChange} options={options} placeholder="Search teacher…" />);

    const input = screen.getByRole("combobox");
    await user.click(input);
    await user.type(input, "pe");
    fireEvent.blur(input);

    expect(onChange).not.toHaveBeenCalled();
  });
});
