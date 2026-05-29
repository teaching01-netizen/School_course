import { expect, it, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StaleEditBanner } from "../StaleEditBanner";

const defaultProps = {
  entityType: "session" as const,
  serverCopy: { id: 1, title: "Server Title", start_at: "09:00" },
  localCopy: { id: 1, title: "My Title", start_at: "09:00" },
  fields: ["title", "start_at"],
  onAcceptServer: vi.fn(),
  onRetry: vi.fn(),
  onCancel: vi.fn(),
};

beforeEach(() => {
  vi.clearAllMocks();
});

it("renders field diff table with local and server values", () => {
  render(<StaleEditBanner {...defaultProps} />);

  expect(screen.getByText("title")).toBeInTheDocument();
  expect(screen.getByText("My Title")).toBeInTheDocument();
  expect(screen.getByText("Server Title")).toBeInTheDocument();

  expect(screen.getByText("start_at")).toBeInTheDocument();
  expect(screen.getAllByText("09:00").length).toBeGreaterThanOrEqual(1);
});

it("highlights changed fields differently from unchanged fields", () => {
  render(<StaleEditBanner {...defaultProps} />);

  const titleRow = screen.getByText("title").closest("tr");
  const startAtRow = screen.getByText("start_at").closest("tr");

  expect(titleRow).toHaveClass("bg-red-50");
  expect(startAtRow).not.toHaveClass("bg-red-50");
});

it("shows 'Session' in title for entityType session", () => {
  render(<StaleEditBanner {...defaultProps} entityType="session" />);
  expect(screen.getByRole("heading")).toHaveTextContent("Session");
});

it("shows 'Series' in title for entityType series", () => {
  render(<StaleEditBanner {...defaultProps} entityType="series" />);
  expect(screen.getByRole("heading")).toHaveTextContent("Series");
});

it("shows 'Absence' in title for entityType absence", () => {
  render(<StaleEditBanner {...defaultProps} entityType="absence" />);
  expect(screen.getByRole("heading")).toHaveTextContent("Absence");
});

it("calls onAcceptServer when 'Accept server version' clicked", async () => {
  const user = userEvent.setup();
  render(<StaleEditBanner {...defaultProps} />);

  await user.click(screen.getByText("Accept server version"));
  expect(defaultProps.onAcceptServer).toHaveBeenCalledTimes(1);
});

it("calls onRetry when 'Retry with my changes' clicked", async () => {
  const user = userEvent.setup();
  render(<StaleEditBanner {...defaultProps} />);

  await user.click(screen.getByText("Retry with my changes"));
  expect(defaultProps.onRetry).toHaveBeenCalledTimes(1);
});

it("calls onCancel when 'Cancel' clicked", async () => {
  const user = userEvent.setup();
  render(<StaleEditBanner {...defaultProps} />);

  await user.click(screen.getByText("Cancel"));
  expect(defaultProps.onCancel).toHaveBeenCalledTimes(1);
});

it("has role='alert' for accessibility", () => {
  render(<StaleEditBanner {...defaultProps} />);
  expect(screen.getByRole("alert")).toBeInTheDocument();
});
