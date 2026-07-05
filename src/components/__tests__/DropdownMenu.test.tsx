import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DropdownMenu } from "../ui/DropdownMenu";

const ITEMS = [
  { label: "Special Approve", onClick: vi.fn() },
  { label: "Override", onClick: vi.fn() },
  { label: "Cancel", onClick: vi.fn() },
  { label: "Delete", onClick: vi.fn(), danger: true },
];

function renderMenu(items = ITEMS) {
  return render(<DropdownMenu items={items} />);
}

describe("DropdownMenu", () => {
  it("renders trigger button with accessible label", () => {
    renderMenu();
    expect(screen.getByRole("button", { name: /more actions/i })).toBeInTheDocument();
  });

  it("sets aria-haspopup and aria-expanded on trigger", () => {
    renderMenu();
    const trigger = screen.getByRole("button", { name: /more actions/i });
    expect(trigger).toHaveAttribute("aria-haspopup", "menu");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
  });

  it("opens menu on trigger click", async () => {
    renderMenu();
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: /more actions/i }));

    expect(screen.getByRole("menu")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /more actions/i })).toHaveAttribute("aria-expanded", "true");
  });

  it("closes menu on second trigger click (toggle)", async () => {
    renderMenu();
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: /more actions/i }));
    expect(screen.getByRole("menu")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /more actions/i }));
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("closes menu on outside click", async () => {
    renderMenu();
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: /more actions/i }));
    expect(screen.getByRole("menu")).toBeInTheDocument();

    await user.click(document.body);
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("renders all menu items with role=menuitem", async () => {
    renderMenu();
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: /more actions/i }));

    const menuItems = screen.getAllByRole("menuitem");
    expect(menuItems).toHaveLength(4);
    expect(menuItems[0]).toHaveTextContent("Special Approve");
    expect(menuItems[1]).toHaveTextContent("Override");
    expect(menuItems[2]).toHaveTextContent("Cancel");
    expect(menuItems[3]).toHaveTextContent("Delete");
  });

  it("calls item.onClick and closes menu when item clicked", async () => {
    const onClick = vi.fn();
    const items = [{ label: "Test Action", onClick }];
    renderMenu(items);
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: /more actions/i }));
    await user.click(screen.getByRole("menuitem", { name: /test action/i }));

    expect(onClick).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("applies danger styling to danger items", async () => {
    renderMenu();
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: /more actions/i }));

    const deleteItem = screen.getByRole("menuitem", { name: /delete/i });
    expect(deleteItem.className).toContain("text-red-600");
  });

  it("disables menu items with disabled prop", async () => {
    const items = [
      { label: "Disabled Action", onClick: vi.fn(), disabled: true },
      { label: "Enabled Action", onClick: vi.fn() },
    ];
    renderMenu(items);
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: /more actions/i }));

    const disabledItem = screen.getByRole("menuitem", { name: /disabled action/i });
    expect(disabledItem).toBeDisabled();

    const enabledItem = screen.getByRole("menuitem", { name: /enabled action/i });
    expect(enabledItem).not.toBeDisabled();
  });

  it("does not call onClick for disabled items", async () => {
    const onClick = vi.fn();
    const items = [{ label: "Disabled Action", onClick, disabled: true }];
    renderMenu(items);
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: /more actions/i }));
    await user.click(screen.getByRole("menuitem", { name: /disabled action/i }));

    expect(onClick).not.toHaveBeenCalled();
    expect(screen.getByRole("menu")).toBeInTheDocument();
  });

  it("does not close menu when disabled item is clicked", async () => {
    const items = [{ label: "Disabled Action", onClick: vi.fn(), disabled: true }];
    renderMenu(items);
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: /more actions/i }));
    await user.click(screen.getByRole("menuitem", { name: /disabled action/i }));

    expect(screen.getByRole("menu")).toBeInTheDocument();
  });

  it("renders empty menu when items array is empty", async () => {
    renderMenu([]);
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: /more actions/i }));

    const menu = screen.getByRole("menu");
    expect(menu).toBeInTheDocument();
    expect(screen.queryAllByRole("menuitem")).toHaveLength(0);
  });
});
