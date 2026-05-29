import { expect, it, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ProvisionalBadge } from "../ProvisionalBadge";

const allTrue = { studentOk: true, teacherOk: true, roomOk: true };
const roomFalse = { studentOk: true, teacherOk: true, roomOk: false };
const teacherFalse = { studentOk: true, teacherOk: false, roomOk: false };
const studentFalse = { studentOk: false, teacherOk: false, roomOk: false };

beforeEach(() => {
  vi.clearAllMocks();
});

it("shows 'Available' green badge when all three are true", () => {
  render(<ProvisionalBadge {...allTrue} />);
  expect(screen.getByText("Available")).toBeInTheDocument();
  expect(screen.queryByText("Provisional")).not.toBeInTheDocument();
});

it("does not render checklist in Available mode", () => {
  render(<ProvisionalBadge {...allTrue} />);
  expect(screen.queryByText("Student")).not.toBeInTheDocument();
  expect(screen.queryByText("Teacher")).not.toBeInTheDocument();
  expect(screen.queryByText("Room")).not.toBeInTheDocument();
});

it("shows 'Provisional' amber badge when roomOk is false", () => {
  render(<ProvisionalBadge {...roomFalse} />);
  expect(screen.getByText("Provisional")).toBeInTheDocument();
  expect(screen.queryByText("Available")).not.toBeInTheDocument();
});

it("shows 'Provisional' amber badge when teacherOk is false", () => {
  render(<ProvisionalBadge {...teacherFalse} />);
  expect(screen.getByText("Provisional")).toBeInTheDocument();
});

it("shows 'Provisional' amber badge when studentOk is false", () => {
  render(<ProvisionalBadge {...studentFalse} />);
  expect(screen.getByText("Provisional")).toBeInTheDocument();
});

it("renders checklist with Student, Teacher, Room labels in Provisional mode", () => {
  render(<ProvisionalBadge {...roomFalse} />);
  expect(screen.getByText("Student")).toBeInTheDocument();
  expect(screen.getByText("Teacher")).toBeInTheDocument();
  expect(screen.getByText("Room")).toBeInTheDocument();
});

it("shows Check icon for Student when studentOk is true", () => {
  render(<ProvisionalBadge {...roomFalse} />);
  const studentItem = screen.getByText("Student").closest("span")!;
  expect(studentItem.querySelector("svg")).toHaveClass("text-green-600");
});

it("shows X icon for Student when studentOk is false", () => {
  render(<ProvisionalBadge {...studentFalse} />);
  const studentItem = screen.getByText("Student").closest("span")!;
  expect(studentItem.querySelector("svg")).toHaveClass("text-red-600");
});

it("shows Clock icon for Room when roomOk is false", () => {
  render(<ProvisionalBadge {...roomFalse} />);
  const roomItem = screen.getByText("Room").closest("span")!;
  expect(roomItem.querySelector("svg")).toHaveClass("text-amber-500");
});

it("shows X icon for Teacher when teacherOk is false", () => {
  render(<ProvisionalBadge {...teacherFalse} />);
  const teacherItem = screen.getByText("Teacher").closest("span")!;
  expect(teacherItem.querySelector("svg")).toHaveClass("text-red-600");
});

it("calls onClick when clicked and onClick provided", async () => {
  const user = userEvent.setup();
  const onClick = vi.fn();
  render(<ProvisionalBadge {...roomFalse} onClick={onClick} />);
  await user.click(screen.getByText("Provisional"));
  expect(onClick).toHaveBeenCalledTimes(1);
});

it("does not throw when clicked without onClick", async () => {
  const user = userEvent.setup();
  render(<ProvisionalBadge {...roomFalse} />);
  await user.click(screen.getByText("Provisional"));
});

it("has role='status' accessibility attribute", () => {
  render(<ProvisionalBadge {...roomFalse} />);
  expect(screen.getByRole("status")).toBeInTheDocument();
});

it("has descriptive aria-label", () => {
  render(<ProvisionalBadge {...roomFalse} />);
  const el = screen.getByRole("status");
  expect(el).toHaveAttribute("aria-label");
});

it("applies pointer cursor when onClick provided", () => {
  const { container } = render(
    <ProvisionalBadge {...roomFalse} onClick={() => {}} />
  );
  expect(container.firstElementChild!.className).toContain("cursor-pointer");
});

it("does not apply pointer cursor when onClick omitted", () => {
  const { container } = render(<ProvisionalBadge {...roomFalse} />);
  expect(container.firstElementChild!.className).not.toContain("cursor-pointer");
});

it("fires onClick when Available badge is clicked and onClick provided", async () => {
  const user = userEvent.setup();
  const onClick = vi.fn();
  render(<ProvisionalBadge {...allTrue} onClick={onClick} />);
  await user.click(screen.getByText("Available"));
  expect(onClick).toHaveBeenCalledTimes(1);
});

it("applies pointer cursor on Available badge when onClick provided", () => {
  const { container } = render(
    <ProvisionalBadge {...allTrue} onClick={() => {}} />
  );
  expect(container.firstElementChild!.className).toContain("cursor-pointer");
});
