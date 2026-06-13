import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { ToastProvider } from "@/hooks/useToast";
import { CRMConflictResolutionPanel } from "../CRMConflictResolutionPanel";
import type { BusyRangeConflict } from "../CRMConflictResolutionPanel";

vi.mock("@/api/client", () => ({
  apiJson: vi.fn(),
}));

function wrapper({ children }: { children: React.ReactNode }) {
  return <ToastProvider>{children}</ToastProvider>;
}

function makeConflict(overrides: Partial<BusyRangeConflict> = {}): BusyRangeConflict {
  return {
    studentWCode: "W260032",
    studentName: "Korboon Kanchanomai",
    targetCourse: "SAT Verbal Reading Beginner Section 1 C2/26",
    targetCourseID: "course-abc",
    conflictingCourse: "SAT Math Scholar C2",
    conflictTime: "13 Jun 2026, 09:00-10:00",
    targetSessions: [],
    detail: "Korboon Kanchanomai (W260032) conflicts with 1 session(s)",
    ...overrides,
  };
}

describe("CRMConflictResolutionPanel", () => {
  it("renders conflict details when targetSessions is empty instead of returning null", () => {
    const conflict = makeConflict({ targetSessions: [] });
    render(<CRMConflictResolutionPanel conflict={conflict} onResolved={vi.fn()} />, { wrapper });

    expect(screen.getByText(/resolve as cross-enrollment/i)).toBeInTheDocument();
    expect(screen.getByText(/SAT Verbal Reading Beginner Section 1 C2\/26/)).toBeInTheDocument();
  });

  it("renders info message when no target sessions are available for exclusion", () => {
    const conflict = makeConflict({ targetSessions: [] });
    render(<CRMConflictResolutionPanel conflict={conflict} onResolved={vi.fn()} />, { wrapper });

    expect(screen.getByText(/no sessions available/i)).toBeInTheDocument();
  });

  it("disables submit button when targetSessions is empty", () => {
    const conflict = makeConflict({ targetSessions: [] });
    render(<CRMConflictResolutionPanel conflict={conflict} onResolved={vi.fn()} />, { wrapper });

    const submitButton = screen.getByRole("button", { name: /confirm/i });
    expect(submitButton).toBeDisabled();
  });

  it("renders session checkboxes when targetSessions is populated", () => {
    const conflict = makeConflict({
      targetSessions: [
        { session_id: "s1", start_at: "2026-06-13T02:00:00Z", end_at: "2026-06-13T03:00:00Z" },
        { session_id: "s2", start_at: "2026-06-13T04:00:00Z", end_at: "2026-06-13T05:00:00Z" },
      ],
    });
    render(<CRMConflictResolutionPanel conflict={conflict} onResolved={vi.fn()} />, { wrapper });

    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes).toHaveLength(2);
  });

  it("renders nothing when studentWCode is missing", () => {
    const conflict = makeConflict({ studentWCode: null });
    render(<CRMConflictResolutionPanel conflict={conflict} onResolved={vi.fn()} />, { wrapper });
    expect(screen.queryByText(/resolve as cross-enrollment/i)).not.toBeInTheDocument();
  });

  it("renders nothing when targetCourseID is missing", () => {
    const conflict = makeConflict({ targetCourseID: null });
    render(<CRMConflictResolutionPanel conflict={conflict} onResolved={vi.fn()} />, { wrapper });
    expect(screen.queryByText(/resolve as cross-enrollment/i)).not.toBeInTheDocument();
  });
});
