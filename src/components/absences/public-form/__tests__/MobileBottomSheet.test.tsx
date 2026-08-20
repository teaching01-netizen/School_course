import { render, screen } from "@testing-library/react";
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
