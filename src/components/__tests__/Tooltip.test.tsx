import { expect, it } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Tooltip } from "../ui/Tooltip";

it("renders info icon button", () => {
  render(<Tooltip content="Help text" />);
  expect(screen.getByRole("button")).toBeInTheDocument();
});

it("shows tooltip content on hover", async () => {
  const user = userEvent.setup();
  render(<Tooltip content="Help text" />);
  await user.hover(screen.getByRole("button"));
  expect(await screen.findByRole("tooltip")).toHaveTextContent("Help text");
});

it("hides tooltip content on mouse leave", async () => {
  const user = userEvent.setup();
  render(<Tooltip content="Help text" />);
  await user.hover(screen.getByRole("button"));
  expect(await screen.findByRole("tooltip")).toBeInTheDocument();
  await user.unhover(screen.getByRole("button"));
  expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
});

it("shows tooltip on focus", async () => {
  const user = userEvent.setup();
  render(<Tooltip content="Help text" />);
  await user.tab();
  expect(screen.getByRole("tooltip")).toHaveTextContent("Help text");
});

it("hides tooltip on blur", async () => {
  const user = userEvent.setup();
  render(<Tooltip content="Help text" />);
  await user.tab();
  expect(screen.getByRole("tooltip")).toBeInTheDocument();
  await user.tab();
  expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
});

it("sets aria-describedby on trigger when visible", async () => {
  const user = userEvent.setup();
  render(<Tooltip content="Help text" />);
  const trigger = screen.getByRole("button");
  expect(trigger).not.toHaveAttribute("aria-describedby");
  await user.hover(trigger);
  await waitFor(() => expect(trigger).toHaveAttribute("aria-describedby"));
  const tooltip = screen.getByRole("tooltip");
  expect(trigger.getAttribute("aria-describedby")).toBe(tooltip.id);
});
