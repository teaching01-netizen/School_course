import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import SessionGrid from "../SessionGrid";
import type { SubjectSessions } from "@/types";

const MOCK_SUBJECTS: SubjectSessions[] = [
  {
    subject_id: "subj-1",
    subject_code: "MATH",
    subject_name: "Mathematics",
    course_id: "c-math201",
    course_code: "MATH201",
    course_name: "Mathematics 201",
    sessions: [
      { id: "s1", start_at: "2026-06-01T09:00:00Z", end_at: "2026-06-01T10:30:00Z", date: "2026-06-01", already_absent: false },
      { id: "s2", start_at: "2026-06-03T09:00:00Z", end_at: "2026-06-03T10:30:00Z", date: "2026-06-03", already_absent: false },
    ],
  },
  {
    subject_id: "subj-2",
    subject_code: "PHYS",
    subject_name: "Physics",
    course_id: "c-phys301",
    course_code: "PHYS301",
    course_name: "Physics 301",
    sessions: [
      { id: "s3", start_at: "2026-06-02T11:00:00Z", end_at: "2026-06-02T12:30:00Z", date: "2026-06-02", already_absent: true },
    ],
  },
];

const ALL_SESSION_IDS = new Set(["s1", "s2", "s3"]);

function defaultProps(overrides?: Partial<React.ComponentProps<typeof SessionGrid>>) {
  return {
    subjects: MOCK_SUBJECTS,
    selectedSessionIds: ALL_SESSION_IDS,
    onToggleSession: vi.fn(),
    onToggleAll: vi.fn(),
    onToggleSubject: vi.fn(),
    allSelected: true,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("SessionGrid", () => {
  it("renders sessions grouped by subject", () => {
    render(<SessionGrid {...defaultProps()} />);

    // Subject code and name are in separate spans
    expect(screen.getByText("MATH")).toBeInTheDocument();
    expect(screen.getByText("(Mathematics)")).toBeInTheDocument();
    expect(screen.getByText("PHYS")).toBeInTheDocument();
    expect(screen.getByText("(Physics)")).toBeInTheDocument();

    // 2 MATH chips + 1 PHYS chip + 1 master toggle + 2 subject toggles = 6
    const checkboxes = screen.getAllByRole("checkbox");
    expect(checkboxes.length).toBe(6);
  });

  it("shows header with date range derived from sessions", () => {
    render(<SessionGrid {...defaultProps()} />);
    expect(screen.getByText(/Classes in range/)).toBeInTheDocument();
  });

  it("all sessions selected by default show aria-checked=true", () => {
    render(<SessionGrid {...defaultProps()} />);
    const subjectChips = screen.getAllByRole("checkbox").filter(
      (el) => el.getAttribute("aria-label")?.includes("2026-")
    );
    subjectChips.forEach((chip) => {
      expect(chip).toHaveAttribute("aria-checked", "true");
    });
  });

  it("master toggle deselects all — calls onToggleAll", async () => {
    const user = userEvent.setup();
    const onToggleAll = vi.fn();
    render(<SessionGrid {...defaultProps({ onToggleAll, allSelected: true })} />);

    const masterToggle = screen.getByRole("checkbox", { name: /select all/i });
    await user.click(masterToggle);

    expect(onToggleAll).toHaveBeenCalledTimes(1);
  });

  it("master toggle re-selects all — calls onToggleAll when not all selected", async () => {
    const user = userEvent.setup();
    const onToggleAll = vi.fn();
    render(<SessionGrid {...defaultProps({ onToggleAll, allSelected: false })} />);

    const masterToggle = screen.getByRole("checkbox", { name: /select all/i });
    await user.click(masterToggle);

    expect(onToggleAll).toHaveBeenCalledTimes(1);
  });

  it("per-subject toggle calls onToggleSubject with subjectId", async () => {
    const user = userEvent.setup();
    const onToggleSubject = vi.fn();
    render(<SessionGrid {...defaultProps({ onToggleSubject })} />);

    const mathToggle = screen.getByRole("checkbox", { name: /toggle all MATH/i });
    await user.click(mathToggle);

    expect(onToggleSubject).toHaveBeenCalledWith("subj-1");
  });

  it("empty state shows 'No classes found' message", () => {
    render(<SessionGrid {...defaultProps({ subjects: [] })} />);
    expect(screen.getByText(/No classes found/i)).toBeInTheDocument();
  });

  it("subject groups have role='group' with correct aria-label", () => {
    render(<SessionGrid {...defaultProps()} />);
    const mathGroup = screen.getByRole("group", { name: /MATH sessions/i });
    expect(mathGroup).toBeInTheDocument();
    const physGroup = screen.getByRole("group", { name: /PHYS sessions/i });
    expect(physGroup).toBeInTheDocument();
  });

  it("master toggle has role='checkbox' with aria-checked and aria-label", () => {
    render(<SessionGrid {...defaultProps({ allSelected: true })} />);
    const masterToggle = screen.getByRole("checkbox", { name: /select all/i });
    expect(masterToggle).toHaveAttribute("role", "checkbox");
    expect(masterToggle).toHaveAttribute("aria-checked", "true");
  });

  it("subject toggle has role='checkbox' with aria-checked", () => {
    render(<SessionGrid {...defaultProps({ allSelected: true })} />);
    const mathToggle = screen.getByRole("checkbox", { name: /toggle all MATH/i });
    expect(mathToggle).toHaveAttribute("role", "checkbox");
    expect(mathToggle).toHaveAttribute("aria-checked", "true");
  });

  it("each session renders a chip with correct date and time props", () => {
    render(<SessionGrid {...defaultProps()} />);

    // s1 and s2 both have 09:00–10:30
    const morningChips = screen.getAllByText(/09:00/);
    expect(morningChips.length).toBe(2);
    // s3 has 11:00–12:30
    expect(screen.getByText(/11:00/)).toBeInTheDocument();
  });

  it("live region announces selected count", () => {
    render(<SessionGrid {...defaultProps({ selectedSessionIds: new Set(["s1", "s3"]) })} />);
    const liveRegion = screen.getByRole("status");
    expect(liveRegion).toHaveTextContent(/2 of 3/);
  });
});
