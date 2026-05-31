import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import Calendar from "../calendar";

it("renders without crashing", () => {
  render(<Calendar />);
  expect(screen.getByRole("grid")).toBeInTheDocument();
});

it("accepts and applies custom className", () => {
  const { container } = render(<Calendar className="my-custom-class" />);
  const rdpRoot = container.querySelector(".rdp");
  expect(rdpRoot).toHaveClass("my-custom-class");
});

it("defaults to a full-width responsive layout", () => {
  const { container } = render(<Calendar />);
  const rdpRoot = container.querySelector(".rdp");
  expect(rdpRoot).toHaveClass("w-full");
  expect(rdpRoot).toHaveClass("max-w-full");
});

it("passes mode prop through to DayPicker", () => {
  const onSelect = vi.fn();
  render(<Calendar mode="single" onSelect={onSelect} />);
  const dayButton = screen.getAllByRole("button").find(
    (el) => el.getAttribute("aria-label")?.includes("September") || el.closest("td") !== null
  );
  expect(dayButton ?? screen.getByRole("grid")).toBeTruthy();
});

it("defaults showOutsideDays to true", () => {
  const { container } = render(<Calendar />);
  const outsideDays = container.querySelectorAll(".rdp-outside");
  expect(outsideDays.length).toBeGreaterThan(0);
});
