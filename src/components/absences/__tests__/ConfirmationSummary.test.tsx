import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import ConfirmationSummary from "../ConfirmationSummary";

function baseProps() {
  return {
    studentName: "John Smith",
    wcode: "W250389",
    dateFrom: "2026-06-02",
    dateTo: "2026-06-06",
    subjects: [
      { code: "MATH", name: "Mathematics", dates: ["2 Jun 2026", "4 Jun 2026"], sitInLabel: "Zoom" },
      { code: "PHYS", name: "Physics", dates: ["3 Jun 2026"], sitInLabel: "Physical" },
    ],
    reason: "Doctor appointment",
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("ConfirmationSummary", () => {
  it("shows student name and wcode", () => {
    render(<ConfirmationSummary {...baseProps()} />);
    expect(screen.getByText(/John Smith/)).toBeInTheDocument();
    expect(screen.getByText(/W250389/)).toBeInTheDocument();
  });

  it("shows date range", () => {
    render(<ConfirmationSummary {...baseProps()} />);
    expect(screen.getAllByText(/2 Jun 2026/).length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText(/6 Jun 2026/)).toBeInTheDocument();
  });

  it("shows subjects", () => {
    render(<ConfirmationSummary {...baseProps()} />);
    expect(screen.getByText(/MATH/)).toBeInTheDocument();
    expect(screen.getByText(/Mathematics/)).toBeInTheDocument();
    expect(screen.getByText(/PHYS/)).toBeInTheDocument();
    expect(screen.getByText(/Physics/)).toBeInTheDocument();
  });

  it("shows reason when provided", () => {
    render(<ConfirmationSummary {...baseProps()} reason="Doctor appointment" />);
    expect(screen.getByText(/Doctor appointment/)).toBeInTheDocument();
  });

  it("shows sit-in label per subject", () => {
    render(<ConfirmationSummary {...baseProps()} />);
    expect(screen.getByText(/Zoom/)).toBeInTheDocument();
    expect(screen.getByText(/Physical/)).toBeInTheDocument();
  });
});
