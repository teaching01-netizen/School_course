import { expect, it, vi, beforeEach } from "vitest";
import { render, screen, within } from "@testing-library/react";
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

function getRowByText(text: string) {
  return screen.getByText(text).closest("tr")!;
}

it("renders field diff table with local and server values", () => {
  render(<StaleEditBanner {...defaultProps} />);

  expect(screen.getByText("My Title")).toBeInTheDocument();
  expect(screen.getByText("Server Title")).toBeInTheDocument();
  expect(screen.getAllByText("09:00").length).toBeGreaterThanOrEqual(1);
});

it("highlights changed fields differently from unchanged fields", () => {
  render(<StaleEditBanner {...defaultProps} />);

  const table = screen.getByRole("table");
  const rows = within(table).getAllByRole("row");
  const dataRows = rows.filter((r) => r.querySelector("td"));
  const changedRow = dataRows.find((r) => r.getAttribute("data-changed") === "true");
  const unchangedRow = dataRows.find((r) => !r.getAttribute("data-changed"));

  expect(changedRow).toBeDefined();
  expect(changedRow).toHaveClass("bg-red-50");
  expect(unchangedRow).toBeDefined();
  expect(unchangedRow).not.toHaveClass("bg-red-50");
});

it("shows 'changed' text indicator for changed fields (WCAG 1.4.1)", () => {
  render(<StaleEditBanner {...defaultProps} />);

  const changedSpans = screen.getAllByText("changed");
  expect(changedSpans).toHaveLength(1);
  expect(changedSpans[0]).toHaveClass("text-red-600");
});

it("shows 'changed' text indicator for multiple changed fields", () => {
  render(
    <StaleEditBanner
      {...defaultProps}
      serverCopy={{ id: 1, title: "Server Title", start_at: "10:00" }}
      localCopy={{ id: 1, title: "My Title", start_at: "09:00" }}
    />
  );

  expect(screen.getAllByText("changed")).toHaveLength(2);
});

it("uses fieldLabels when provided", () => {
  const fieldLabels = { title: "Title", start_at: "Start Time" };
  render(<StaleEditBanner {...defaultProps} fieldLabels={fieldLabels} />);

  expect(screen.getByText("Title")).toBeInTheDocument();
  expect(screen.getByText("Start Time")).toBeInTheDocument();
});

it("falls back to raw field names when fieldLabels not provided", () => {
  render(<StaleEditBanner {...defaultProps} />);

  const startAtRow = getRowByText("start_at");
  expect(startAtRow).toBeInTheDocument();
  // Verify the raw field name is rendered, not a label
  expect(within(startAtRow).getByText("start_at")).toBeInTheDocument();
});

it("falls back to raw field name when fieldLabels missing a mapping", () => {
  const fieldLabels = { title: "Title" };
  render(<StaleEditBanner {...defaultProps} fieldLabels={fieldLabels} />);

  expect(screen.getByText("Title")).toBeInTheDocument();
  expect(screen.getByText("start_at")).toBeInTheDocument();
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
