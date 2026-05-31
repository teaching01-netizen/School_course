import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import SummaryBar from "../SummaryBar";

const defaultProps = {
  absentCount: 5,
  coverCount: 3,
  dayBreakdown: [
    { day: "Mon", count: 3, date: "2026-06-01" },
    { day: "Tue", count: 2, date: "2026-06-02" },
  ],
  hasSelection: true,
  onBack: vi.fn(),
  onScrollToDate: vi.fn(),
  onSubmit: vi.fn(),
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe("SummaryBar", () => {
  describe("counts display", () => {
    it("renders absent and cover counts in summary text", () => {
      render(<SummaryBar {...defaultProps} />);
      expect(screen.getByText(/5 absent/)).toBeInTheDocument();
      expect(screen.getByText(/3 cover/)).toBeInTheDocument();
    });

    it("renders zero counts correctly", () => {
      render(
        <SummaryBar
          {...defaultProps}
          absentCount={0}
          coverCount={0}
          hasSelection={false}
        />,
      );
      expect(screen.getByText(/0 absent/)).toBeInTheDocument();
      expect(screen.getByText(/0 cover/)).toBeInTheDocument();
    });
  });

  describe("day breakdown chips", () => {
    it("renders a chip for each day with count", () => {
      render(<SummaryBar {...defaultProps} />);
      expect(screen.getByText("Mon: 3")).toBeInTheDocument();
      expect(screen.getByText("Tue: 2")).toBeInTheDocument();
    });

    it("clicking a day chip calls onScrollToDate with the date", async () => {
      const user = userEvent.setup();
      const onScrollToDate = vi.fn();
      render(
        <SummaryBar {...defaultProps} onScrollToDate={onScrollToDate} />,
      );
      await user.click(screen.getByText("Mon: 3"));
      expect(onScrollToDate).toHaveBeenCalledWith("2026-06-01");
    });

    it("renders no chips when dayBreakdown is empty", () => {
      render(<SummaryBar {...defaultProps} dayBreakdown={[]} />);
      expect(screen.queryByText(/Mon/)).not.toBeInTheDocument();
    });
  });

  describe("back button", () => {
    it("renders a back button", () => {
      render(<SummaryBar {...defaultProps} />);
      expect(
        screen.getByRole("button", { name: /back/i }),
      ).toBeInTheDocument();
    });

    it("clicking back calls onBack", async () => {
      const user = userEvent.setup();
      const onBack = vi.fn();
      render(<SummaryBar {...defaultProps} onBack={onBack} />);
      await user.click(screen.getByRole("button", { name: /back/i }));
      expect(onBack).toHaveBeenCalledTimes(1);
    });
  });

  describe("submit button", () => {
    it("renders a submit button", () => {
      render(<SummaryBar {...defaultProps} />);
      expect(
        screen.getByRole("button", { name: /submit/i }),
      ).toBeInTheDocument();
    });

    it("submit is disabled when hasSelection is false", () => {
      render(
        <SummaryBar {...defaultProps} hasSelection={false} />,
      );
      expect(
        screen.getByRole("button", { name: /submit/i }),
      ).toBeDisabled();
    });

    it("submit is enabled when hasSelection is true", () => {
      render(<SummaryBar {...defaultProps} hasSelection={true} />);
      expect(
        screen.getByRole("button", { name: /submit/i }),
      ).toBeEnabled();
    });

    it("clicking submit calls onSubmit", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();
      render(
        <SummaryBar {...defaultProps} onSubmit={onSubmit} />,
      );
      await user.click(screen.getByRole("button", { name: /submit/i }));
      expect(onSubmit).toHaveBeenCalledTimes(1);
    });
  });

  describe("live count updates", () => {
    it("updates displayed counts when props change", () => {
      const { rerender } = render(
        <SummaryBar {...defaultProps} absentCount={2} coverCount={1} />,
      );
      expect(screen.getByText(/2 absent/)).toBeInTheDocument();
      expect(screen.getByText(/1 cover/)).toBeInTheDocument();

      rerender(
        <SummaryBar {...defaultProps} absentCount={5} coverCount={3} />,
      );
      expect(screen.getByText(/5 absent/)).toBeInTheDocument();
      expect(screen.getByText(/3 cover/)).toBeInTheDocument();
    });
  });
});
