import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import SessionChip from "../SessionChip";

const defaultProps = {
  id: "sess-1",
  date: "2026-06-01",
  startTime: "09:00",
  endTime: "10:30",
  selected: false,
  alreadyAbsent: false,
  onToggle: vi.fn(),
  subjectCode: "MATH101",
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe("SessionChip", () => {
  it("renders with correct text", () => {
    render(<SessionChip {...defaultProps} />);
    expect(screen.getByText(/Mon.*09:00.*10:30/)).toBeInTheDocument();
  });

  it("selected state has green styling and aria-checked=true", () => {
    render(<SessionChip {...defaultProps} selected={true} />);
    const chip = screen.getByRole("checkbox");
    expect(chip).toHaveAttribute("aria-checked", "true");
    expect(chip.className).toContain("bg-green-100");
    expect(chip.className).toContain("border-green-300");
    expect(chip.className).toContain("text-green-800");
  });

  it("deselected state has gray styling and aria-checked=false", () => {
    render(<SessionChip {...defaultProps} selected={false} />);
    const chip = screen.getByRole("checkbox");
    expect(chip).toHaveAttribute("aria-checked", "false");
    expect(chip.className).toContain("bg-gray-100");
    expect(chip.className).toContain("border-gray-300");
    expect(chip.className).toContain("text-gray-600");
  });

  it("already absent has disabled styling and aria-disabled=true", () => {
    render(<SessionChip {...defaultProps} alreadyAbsent={true} />);
    const chip = screen.getByRole("checkbox");
    expect(chip).toHaveAttribute("aria-disabled", "true");
    expect(chip.className).toContain("cursor-not-allowed");
    expect(chip.className).toContain("bg-gray-50");
    expect(chip.className).toContain("text-gray-400");
  });

  it("clicking toggles selection", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(<SessionChip {...defaultProps} onToggle={onToggle} />);
    await user.click(screen.getByRole("checkbox"));
    expect(onToggle).toHaveBeenCalledWith("sess-1");
  });

  it("keyboard Enter/Space toggles selection", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(<SessionChip {...defaultProps} onToggle={onToggle} />);
    const chip = screen.getByRole("checkbox");
    chip.focus();
    await user.keyboard("{Enter}");
    expect(onToggle).toHaveBeenCalledWith("sess-1");

    onToggle.mockClear();
    await user.keyboard(" ");
    expect(onToggle).toHaveBeenCalledWith("sess-1");
  });

  it("disabled prevents toggle", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(<SessionChip {...defaultProps} disabled={true} onToggle={onToggle} />);
    await user.click(screen.getByRole("checkbox"));
    expect(onToggle).not.toHaveBeenCalled();
  });

  it("alreadyAbsent prevents toggle", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(<SessionChip {...defaultProps} alreadyAbsent={true} onToggle={onToggle} />);
    await user.click(screen.getByRole("checkbox"));
    expect(onToggle).not.toHaveBeenCalled();
  });

  it("aria-label includes date, time, and subject code", () => {
    render(<SessionChip {...defaultProps} />);
    const chip = screen.getByRole("checkbox");
    expect(chip).toHaveAttribute(
      "aria-label",
      "2026-06-01 09:00-10:30 MATH101"
    );
  });

  it("aria-label omits subject code when not provided", () => {
    const { subjectCode: _, ...propsWithoutSubject } = defaultProps;
    render(<SessionChip {...propsWithoutSubject} />);
    const chip = screen.getByRole("checkbox");
    expect(chip).toHaveAttribute("aria-label", "2026-06-01 09:00-10:30 ");
  });
});
