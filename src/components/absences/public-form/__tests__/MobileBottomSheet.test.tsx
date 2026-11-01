import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef, useState } from "react";
import { describe, expect, it } from "vitest";
import MobileBottomSheet from "../MobileBottomSheet";

function SheetHarness() {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  return (
    <>
      <button ref={triggerRef} type="button" onClick={() => setOpen(true)}>
        Choose make-up
      </button>
      <MobileBottomSheet
        open={open}
        title="Choose a make-up class"
        onClose={() => setOpen(false)}
        restoreFocusRef={triggerRef}
      >
        <button type="button" onClick={() => setOpen(false)}>Saturday class</button>
      </MobileBottomSheet>
    </>
  );
}

describe("MobileBottomSheet", () => {
  it("keeps a labelled, full-size close control beside the title", async () => {
    const user = userEvent.setup();
    render(<SheetHarness />);

    await user.click(screen.getByRole("button", { name: "Choose make-up" }));
    const dialog = screen.getByRole("dialog");
    const close = within(dialog).getByRole("button", { name: "Close sheet" });

    expect(close).toHaveClass("absence-mobile-sheet__dismiss");
    expect(within(dialog).getByRole("heading", { name: "Choose a make-up class" }).parentElement).toContainElement(close);

    await user.click(close);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("moves focus into the sheet and restores it to the trigger", async () => {
    const user = userEvent.setup();
    render(<SheetHarness />);

    const trigger = screen.getByRole("button", { name: "Choose make-up" });
    await user.click(trigger);

    expect(screen.getByRole("dialog", { name: "Choose a make-up class" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Saturday class" })).toHaveFocus();

    await user.click(screen.getByRole("button", { name: "Saturday class" }));
    expect(trigger).toHaveFocus();
  });
});
