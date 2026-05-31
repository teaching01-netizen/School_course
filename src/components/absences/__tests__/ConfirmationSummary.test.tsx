import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ConfirmationSummary, { type ConfirmationSummaryProps } from "../ConfirmationSummary";

function baseProps(overrides?: Partial<ConfirmationSummaryProps>): ConfirmationSummaryProps {
  return {
    studentName: "John Smith",
    wcode: "W250389",
    dateFrom: "2026-06-02",
    dateTo: "2026-06-06",
    subjects: [
      {
        subjectCode: "MATH",
        subjectName: "Mathematics",
        sessionCount: 2,
        sitInMethod: "Zoom",
      },
      {
        subjectCode: "PHYS",
        subjectName: "Physics",
        sessionCount: 1,
        sitInMethod: "Physical",
      },
    ],
    mode: "review",
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("ConfirmationSummary", () => {
  describe("review mode", () => {
    it("shows student name and wcode", () => {
      render(<ConfirmationSummary {...baseProps()} />);
      expect(screen.getByText(/John Smith/)).toBeInTheDocument();
      expect(screen.getByText(/W250389/)).toBeInTheDocument();
    });

    it("shows date range", () => {
      render(<ConfirmationSummary {...baseProps()} />);
      expect(screen.getByText(/2 Jun 2026/)).toBeInTheDocument();
      expect(screen.getByText(/6 Jun 2026/)).toBeInTheDocument();
    });

    it("shows subjects with session count", () => {
      render(<ConfirmationSummary {...baseProps()} />);
      expect(screen.getByText(/MATH/)).toBeInTheDocument();
      expect(screen.getByText(/Mathematics/)).toBeInTheDocument();
      expect(screen.getByText(/2 session/)).toBeInTheDocument();
      expect(screen.getByText(/PHYS/)).toBeInTheDocument();
      expect(screen.getByText(/Physics/)).toBeInTheDocument();
      expect(screen.getByText(/1 session/)).toBeInTheDocument();
    });

    it("shows reason when provided", () => {
      render(
        <ConfirmationSummary
          {...baseProps()}
          reasonCategory="medical"
          reasonCategoryLabel="Medical"
          reason="Doctor appointment"
        />,
      );
      expect(screen.getByText(/Medical/)).toBeInTheDocument();
      expect(screen.getByText(/Doctor appointment/)).toBeInTheDocument();
    });

    it("shows sit-in method per subject", () => {
      render(<ConfirmationSummary {...baseProps()} />);
      expect(screen.getByText(/Zoom/)).toBeInTheDocument();
      expect(screen.getByText(/Physical/)).toBeInTheDocument();
    });

    it("does not show result mode elements in review mode", () => {
      render(<ConfirmationSummary {...baseProps()} />);
      expect(screen.queryByText(/Submitted/)).not.toBeInTheDocument();
      expect(screen.queryByText(/Submit Another/)).not.toBeInTheDocument();
      expect(screen.queryByText(/Done/)).not.toBeInTheDocument();
    });

    it("shows heading", () => {
      render(<ConfirmationSummary {...baseProps()} />);
      expect(screen.getByText(/Review your absence/)).toBeInTheDocument();
    });
  });

  describe("review mode with edit", () => {
    it("shows edit button", () => {
      render(<ConfirmationSummary {...baseProps({ onEdit: vi.fn() })} />);
      expect(screen.getByRole("button", { name: /edit/i })).toBeInTheDocument();
    });

    it("clicking edit calls onEdit", async () => {
      const user = userEvent.setup();
      const onEdit = vi.fn();
      render(<ConfirmationSummary {...baseProps({ onEdit })} />);
      await user.click(screen.getByRole("button", { name: /edit/i }));
      expect(onEdit).toHaveBeenCalledTimes(1);
    });

    it("does not show edit button when onEdit not provided", () => {
      render(<ConfirmationSummary {...baseProps()} />);
      expect(screen.queryByRole("button", { name: /edit/i })).not.toBeInTheDocument();
    });
  });

  describe("result mode success", () => {
    it("shows success checkmark heading", () => {
      render(
        <ConfirmationSummary
          {...baseProps({
            mode: "result",
            subjects: [
              {
                subjectCode: "MATH",
                subjectName: "Mathematics",
                sessionCount: 2,
                submitStatus: "success",
              },
            ],
          })}
        />,
      );
      expect(screen.getByText(/Absence submitted/)).toBeInTheDocument();
    });

    it("shows success indicator for submitted subject", () => {
      render(
        <ConfirmationSummary
          {...baseProps({
            mode: "result",
            subjects: [
              {
                subjectCode: "MATH",
                subjectName: "Mathematics",
                sessionCount: 2,
                submitStatus: "success",
              },
            ],
          })}
        />,
      );
      expect(screen.getByText(/MATH/)).toBeInTheDocument();
      expect(screen.getByText(/Mathematics/)).toBeInTheDocument();
      expect(screen.getByText(/Submitted/)).toBeInTheDocument();
    });

    it("shows pending indicator for pending subject", () => {
      render(
        <ConfirmationSummary
          {...baseProps({
            mode: "result",
            subjects: [
              {
                subjectCode: "MATH",
                subjectName: "Mathematics",
                sessionCount: 2,
                submitStatus: "pending",
              },
            ],
          })}
        />,
      );
      expect(screen.getByText(/Pending/)).toBeInTheDocument();
    });
  });

  describe("result mode partial failure", () => {
    it("shows retry button for failed subject", () => {
      render(
        <ConfirmationSummary
          {...baseProps({
            mode: "result",
            subjects: [
              {
                subjectCode: "PHYS",
                subjectName: "Physics",
                sessionCount: 1,
                submitStatus: "error",
                submitError: "network error",
                onRetry: vi.fn(),
              },
            ],
          })}
        />,
      );
      expect(screen.getByText(/Failed/)).toBeInTheDocument();
      expect(screen.getByText(/network error/)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /retry/i })).toBeInTheDocument();
    });

    it("does not show retry button for successful subject", () => {
      render(
        <ConfirmationSummary
          {...baseProps({
            mode: "result",
            subjects: [
              {
                subjectCode: "MATH",
                subjectName: "Mathematics",
                sessionCount: 2,
                submitStatus: "success",
              },
            ],
          })}
        />,
      );
      expect(screen.queryByRole("button", { name: /retry/i })).not.toBeInTheDocument();
    });
  });

  describe("result mode retry", () => {
    it("clicking retry calls subject onRetry", async () => {
      const user = userEvent.setup();
      const onRetry = vi.fn();
      render(
        <ConfirmationSummary
          {...baseProps({
            mode: "result",
            subjects: [
              {
                subjectCode: "PHYS",
                subjectName: "Physics",
                sessionCount: 1,
                submitStatus: "error",
                submitError: "network error",
                onRetry,
              },
            ],
          })}
        />,
      );
      await user.click(screen.getByRole("button", { name: /retry/i }));
      expect(onRetry).toHaveBeenCalledTimes(1);
    });
  });

  describe("result mode buttons", () => {
    it("does not render Submit Another and Done buttons (parent handles them)", () => {
      render(
        <ConfirmationSummary
          {...baseProps({
            mode: "result",
            subjects: [
              {
                subjectCode: "MATH",
                subjectName: "Mathematics",
                sessionCount: 2,
                submitStatus: "success",
              },
            ],
          })}
        />,
      );
      expect(screen.queryByRole("button", { name: /submit another/i })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /done/i })).not.toBeInTheDocument();
    });
  });

  describe("empty subjects", () => {
    it("handles empty subjects array in review mode", () => {
      render(<ConfirmationSummary {...baseProps({ subjects: [] })} />);
      expect(screen.getByText(/John Smith/)).toBeInTheDocument();
      expect(screen.getByText(/Review your absence/)).toBeInTheDocument();
    });

    it("handles empty subjects array in result mode", () => {
      render(
        <ConfirmationSummary
          {...baseProps({
            mode: "result",
            subjects: [],
          })}
        />,
      );
      expect(screen.getByText(/Absence submitted/)).toBeInTheDocument();
    });
  });

  describe("confirmation text", () => {
    it("shows confirmation text when provided in result mode", () => {
      render(
        <ConfirmationSummary
          {...baseProps({
            mode: "result",
            confirmationText: "Your absence has been recorded successfully.",
            subjects: [
              {
                subjectCode: "MATH",
                subjectName: "Mathematics",
                sessionCount: 2,
                submitStatus: "success",
              },
            ],
          })}
        />,
      );
      expect(screen.getByText(/Your absence has been recorded successfully/)).toBeInTheDocument();
    });

    it("does not show confirmation text when not provided", () => {
      render(
        <ConfirmationSummary
          {...baseProps({
            mode: "result",
            subjects: [
              {
                subjectCode: "MATH",
                subjectName: "Mathematics",
                sessionCount: 2,
                submitStatus: "success",
              },
            ],
          })}
        />,
      );
      expect(screen.queryByText(/Your absence has been recorded/)).not.toBeInTheDocument();
    });
  });
});
