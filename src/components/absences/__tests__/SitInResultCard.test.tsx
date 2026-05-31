import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import SitInResultCard, { type SitInResultCardProps } from "../SitInResultCard";
import type { SitInResult } from "../SitInResultCard";

const MOCK_ZOOM_RESULT: SitInResult = {
  sit_in_method: "zoom",
  missed_count: 2,
};

const MOCK_PHYSICAL_RESULT: SitInResult = {
  sit_in_method: "physical",
  sit_in_course: { id: "c-sit", code: "MATH-301", name: "Calculus III" },
  missed_count: 2,
  missed_sessions: [
    { id: "ms1", start_at: "2026-06-01T09:00:00Z", end_at: "2026-06-01T10:30:00Z" },
    { id: "ms2", start_at: "2026-06-03T09:00:00Z", end_at: "2026-06-03T10:30:00Z" },
  ],
  available_sessions: [
    { id: "as1", start_at: "2026-06-01T11:00:00Z", end_at: "2026-06-01T12:30:00Z" },
    { id: "as2", start_at: "2026-06-03T11:00:00Z", end_at: "2026-06-03T12:30:00Z" },
  ],
  pre_selected: [
    { id: "as1", start_at: "2026-06-01T11:00:00Z", end_at: "2026-06-01T12:30:00Z" },
  ],
};

const MOCK_PENDING_RESULT: SitInResult = {
  sit_in_method: "pending",
  missed_count: 0,
};

function baseProps(overrides?: Partial<SitInResultCardProps>): SitInResultCardProps {
  return {
    subjectCode: "MATH-301",
    subjectName: "Calculus III",
    result: MOCK_ZOOM_RESULT,
    selectedSessionIds: new Set<string>(),
    onToggleSession: vi.fn(),
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("SitInResultCard", () => {
  describe("zoom method", () => {
    it("renders blue banner with zoom description", () => {
      render(<SitInResultCard {...baseProps({ result: MOCK_ZOOM_RESULT, zoomDescription: "Zoom session - no physical class attendance required." })} />);
      expect(screen.getByText(/Zoom session/)).toBeInTheDocument();
    });

    it("renders missed count text", () => {
      render(<SitInResultCard {...baseProps({ result: MOCK_ZOOM_RESULT })} />);
      expect(screen.getByText(/You will miss 2 session/)).toBeInTheDocument();
    });

    it("renders no checkboxes", () => {
      render(<SitInResultCard {...baseProps({ result: MOCK_ZOOM_RESULT })} />);
      expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    });
  });

  describe("physical method", () => {
    it("renders green banner with course code", () => {
      render(<SitInResultCard {...baseProps({ result: MOCK_PHYSICAL_RESULT })} />);
      expect(screen.getByText(/MATH-301/)).toBeInTheDocument();
    });

    it("renders course name in banner", () => {
      render(<SitInResultCard {...baseProps({ result: MOCK_PHYSICAL_RESULT })} />);
      expect(screen.getByText(/Calculus III/)).toBeInTheDocument();
    });

    it("renders missed and available counts", () => {
      render(<SitInResultCard {...baseProps({ result: MOCK_PHYSICAL_RESULT })} />);
      expect(screen.getByText(/2 missed session/)).toBeInTheDocument();
      expect(screen.getByText(/2 sit-in session/)).toBeInTheDocument();
    });

    it("renders day headers for each unique date", () => {
      render(<SitInResultCard {...baseProps({ result: MOCK_PHYSICAL_RESULT })} />);
      // Two distinct dates → two day headers
      const dayHeaders = screen.getAllByText(/Jun$/);
      expect(dayHeaders.length).toBe(2);
      // First header is for June 1, second for June 3
      expect(dayHeaders[0].textContent).toContain("Mon");
      expect(dayHeaders[1].textContent).toContain("Wed");
    });

    it("renders missed session labels for each session", () => {
      render(<SitInResultCard {...baseProps({ result: MOCK_PHYSICAL_RESULT })} />);
      const missedTexts = screen.getAllByText(/Missed:/);
      expect(missedTexts.length).toBe(2);
    });

    it("renders sit-in checkboxes for paired sessions", () => {
      render(<SitInResultCard {...baseProps({ result: MOCK_PHYSICAL_RESULT })} />);
      const checkboxes = screen.getAllByRole("checkbox");
      expect(checkboxes.length).toBe(2);
    });
  });

  describe("pending method", () => {
    it("renders amber assigned by staff message", () => {
      render(<SitInResultCard {...baseProps({ result: MOCK_PENDING_RESULT })} />);
      expect(screen.getByText(/assigned by staff after review/)).toBeInTheDocument();
    });
  });

  describe("error state", () => {
    it("renders error message", () => {
      render(<SitInResultCard {...baseProps({ result: MOCK_ZOOM_RESULT, error: "Failed to resolve sit-in" })} />);
      expect(screen.getByText("Failed to resolve sit-in")).toBeInTheDocument();
    });

    it("renders retry button when onRetry provided", () => {
      const onRetry = vi.fn();
      render(<SitInResultCard {...baseProps({ result: MOCK_ZOOM_RESULT, error: "Failed", onRetry })} />);
      expect(screen.getByRole("button", { name: /retry/i })).toBeInTheDocument();
    });

    it("renders skip button when onSkip provided", () => {
      const onSkip = vi.fn();
      render(<SitInResultCard {...baseProps({ result: MOCK_ZOOM_RESULT, error: "Failed", onSkip })} />);
      expect(screen.getByRole("button", { name: /skip/i })).toBeInTheDocument();
    });
  });

  describe("physical pairing", () => {
    it("missed sessions paired with available sessions", () => {
      render(<SitInResultCard {...baseProps({ result: MOCK_PHYSICAL_RESULT })} />);
      const sitInLabels = screen.getAllByText(/Sit-in:/);
      expect(sitInLabels.length).toBe(2);
    });

    it("pre-selected sessions are checked", () => {
      render(<SitInResultCard {...baseProps({ result: MOCK_PHYSICAL_RESULT, selectedSessionIds: new Set(["as1"]) })} />);
      const checkboxes = screen.getAllByRole("checkbox");
      expect(checkboxes[0]).toBeChecked();
      expect(checkboxes[1]).not.toBeChecked();
    });
  });

  describe("checkbox toggle", () => {
    it("clicking checkbox calls onToggleSession with session id", async () => {
      const user = userEvent.setup();
      const onToggleSession = vi.fn();
      render(<SitInResultCard {...baseProps({ result: MOCK_PHYSICAL_RESULT, onToggleSession })} />);
      const checkboxes = screen.getAllByRole("checkbox");
      await user.click(checkboxes[0]);
      expect(onToggleSession).toHaveBeenCalledWith("as1");
    });
  });

  describe("empty sessions", () => {
    it("shows no sessions message when missed and available are empty", () => {
      const emptyResult: SitInResult = {
        sit_in_method: "physical",
        missed_count: 0,
        missed_sessions: [],
        available_sessions: [],
      };
      render(<SitInResultCard {...baseProps({ result: emptyResult })} />);
      expect(screen.getByText(/No sessions/)).toBeInTheDocument();
    });
  });

  describe("retry button", () => {
    it("clicking retry calls onRetry", async () => {
      const user = userEvent.setup();
      const onRetry = vi.fn();
      render(<SitInResultCard {...baseProps({ result: MOCK_ZOOM_RESULT, error: "Failed", onRetry })} />);
      await user.click(screen.getByRole("button", { name: /retry/i }));
      expect(onRetry).toHaveBeenCalledTimes(1);
    });
  });

  describe("skip button", () => {
    it("clicking skip calls onSkip", async () => {
      const user = userEvent.setup();
      const onSkip = vi.fn();
      render(<SitInResultCard {...baseProps({ result: MOCK_ZOOM_RESULT, error: "Failed", onSkip })} />);
      await user.click(screen.getByRole("button", { name: /skip/i }));
      expect(onSkip).toHaveBeenCalledTimes(1);
    });
  });
});
