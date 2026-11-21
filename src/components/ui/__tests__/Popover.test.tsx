import { describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Popover } from "../Popover";

function renderPopover(overrides: Partial<React.ComponentProps<typeof Popover>> = {}) {
  return render(
    <Popover ariaLabel="Edit value" trigger={<button type="button">Open</button>} {...overrides}>
      <div className="p-2">
        <input aria-label="Edit name" />
        <button type="button">Save</button>
      </div>
    </Popover>,
  );
}

describe("Popover", () => {
  it("keeps content hidden until the trigger is clicked", () => {
    renderPopover();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open" })).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByRole("button", { name: "Open" })).toHaveAttribute("aria-haspopup", "dialog");
  });

  it("opens on trigger click and wires dialog aria attributes", async () => {
    renderPopover();
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Open" }));

    const dialog = screen.getByRole("dialog");
    expect(dialog).toBeInTheDocument();
    expect(dialog).toHaveAccessibleName("Edit value");
    const trigger = screen.getByRole("button", { name: "Open" });
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(trigger).toHaveAttribute("aria-controls", dialog.id);
  });

  it("toggles closed on a second trigger click", async () => {
    renderPopover();
    const user = userEvent.setup();
    const trigger = screen.getByRole("button", { name: "Open" });

    await user.click(trigger);
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    await user.click(trigger);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("closes on Escape and restores focus to the trigger", async () => {
    renderPopover();
    const user = userEvent.setup();
    const trigger = screen.getByRole("button", { name: "Open" });

    await user.click(trigger);
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it("closes on outside mousedown", async () => {
    renderPopover();
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Open" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    await user.click(document.body);

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("stays open when interacting inside the panel", async () => {
    renderPopover();
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Open" }));
    const dialog = screen.getByRole("dialog");

    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(dialog).toBeInTheDocument();
  });

  it("moves focus to the first focusable control on open", async () => {
    renderPopover();
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Open" }));

    await waitFor(() => expect(screen.getByLabelText("Edit name")).toHaveFocus());
  });

  it("applies contentClassName to the panel", async () => {
    renderPopover({ contentClassName: "w-80 custom-panel" });
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Open" }));

    expect(screen.getByRole("dialog")).toHaveClass("w-80", "custom-panel");
  });

  it("caps the panel width to the viewport", async () => {
    renderPopover({ contentClassName: "w-80" });
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Open" }));

    const panel = screen.getByRole("dialog");
    const maxWidth = Number.parseFloat(panel.style.maxWidth);
    expect(Number.isFinite(maxWidth)).toBe(true);
    expect(maxWidth).toBeLessThanOrEqual(window.innerWidth);
  });

  it("uses the visible viewport when it is narrower than the layout viewport", async () => {
    const original = Object.getOwnPropertyDescriptor(window, "visualViewport");
    Object.defineProperty(window, "visualViewport", {
      configurable: true,
      value: {
        width: 320,
        height: 600,
        offsetLeft: 0,
        offsetTop: 0,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      },
    });
    try {
      renderPopover({ contentClassName: "w-80" });
      const user = userEvent.setup();

      await user.click(screen.getByRole("button", { name: "Open" }));

      const panel = screen.getByRole("dialog");
      await waitFor(() => expect(panel.style.maxWidth).toBe("312px"));
      const left = Number.parseFloat(panel.style.left);
      expect(left).toBeGreaterThanOrEqual(4);
      expect(left + 312).toBeLessThanOrEqual(320);
    } finally {
      if (original) Object.defineProperty(window, "visualViewport", original);
      else Object.defineProperty(window, "visualViewport", { configurable: true, value: undefined });
    }
  });

  it("portals nested popovers to the body instead of another popover panel", async () => {
    const user = userEvent.setup();
    render(
      <Popover ariaLabel="Outer popover" trigger={<button type="button">Open outer</button>}>
        <Popover ariaLabel="Inner popover" trigger={<button type="button">Open inner</button>}>
          <button type="button">Save inner</button>
        </Popover>
      </Popover>,
    );

    await user.click(screen.getByRole("button", { name: "Open outer" }));
    const outer = screen.getByRole("dialog", { name: "Outer popover" });
    expect(outer).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Open inner" }));
    const inner = screen.getByRole("dialog", { name: "Inner popover" });
    expect(inner).toBeInTheDocument();
    expect(outer).not.toContainElement(inner);
    expect(inner.parentElement).toBe(document.body);
  });

  it("keeps portal containment inside an actual modal dialog", async () => {
    const user = userEvent.setup();
    render(
      <div role="dialog" aria-modal="true" aria-label="Modal dialog">
        <Popover ariaLabel="Edit value" trigger={<button type="button">Open</button>}>
          <button type="button">Save</button>
        </Popover>
      </div>,
    );

    await user.click(screen.getByRole("button", { name: "Open" }));

    const modal = screen.getByRole("dialog", { name: "Modal dialog" });
    const panel = screen.getByRole("dialog", { name: "Edit value" });
    expect(modal).toContainElement(panel);
    expect(panel.parentElement).toBe(modal);
  });

  it("supports controlled open state via open/onOpenChange", async () => {
    const onOpenChange = vi.fn();
    const { rerender } = render(
      <Popover
        ariaLabel="Edit value"
        open={false}
        onOpenChange={onOpenChange}
        trigger={<button type="button">Open</button>}
      >
        <div>content</div>
      </Popover>,
    );
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Open" }));
    expect(onOpenChange).toHaveBeenCalledWith(true);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    rerender(
      <Popover
        ariaLabel="Edit value"
        open={true}
        onOpenChange={onOpenChange}
        trigger={<button type="button">Open</button>}
      >
        <div>content</div>
      </Popover>,
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Open" }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("flips above the trigger when there is not enough space below", async () => {
    renderPopover();
    const user = userEvent.setup();
    const originalHeight = window.innerHeight;

    await user.click(screen.getByRole("button", { name: "Open" }));
    const panel = screen.getByRole("dialog");
    const trigger = screen.getByRole("button", { name: "Open" });

    Object.defineProperty(panel, "offsetWidth", { configurable: true, value: 200 });
    Object.defineProperty(panel, "offsetHeight", { configurable: true, value: 100 });
    Object.defineProperty(trigger, "getBoundingClientRect", {
      configurable: true,
      value: () => ({
        top: 150, bottom: 170, left: 100, right: 300,
        width: 200, height: 20, x: 100, y: 150,
      }),
    });

    try {
      // Tight viewport: only 30px remain below the trigger, panel is 100px tall.
      Object.defineProperty(window, "innerHeight", { configurable: true, value: 200 });
      act(() => {
        window.dispatchEvent(new Event("resize"));
      });

      await waitFor(() => {
        expect(panel.style.top).toBe("46px"); // 150 - 100 - 4 gap = above the trigger
        expect(panel.style.left).toBe("100px"); // start-aligned, unclamped
      });
    } finally {
      Object.defineProperty(window, "innerHeight", { configurable: true, value: originalHeight });
    }
  });
});