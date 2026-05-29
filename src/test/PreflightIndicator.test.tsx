import { describe, it, expect, vi, beforeEach } from "vitest";
import { render as rtlRender, screen, fireEvent, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
function render(ui: React.ReactElement) { return rtlRender(ui, { wrapper: MemoryRouter }); }
import { PreflightIndicator, getSaveButtonLabel, isSaveDisabled } from "@/components/PreflightIndicator";
import { usePreflight } from "@/hooks/usePreflight";
import type { Course, Room, User, ConflictDetails } from "@/types";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function makePreflight(overrides: Partial<ReturnType<typeof usePreflight>>): ReturnType<typeof usePreflight> {
  return {
    status: "idle" as const,
    loading: false,
    details: null,
    error: null,
    occurrencesPlanned: null,
    check: vi.fn(),
    reset: vi.fn(),
    ...overrides,
  };
}

const coursesById = new Map<string, Course>([["c1", { id: "c1", code: "MATH101", name: "Math" }]]);
const teachersById = new Map<string, User>([["t1", { id: "t1", username: "jdoe", role: "Admin" }]]);
const roomsById = new Map<string, Room>([["r1", { id: "r1", name: "Room 101", capacity: 30 }]]);

const sampleConflictDetails: ConflictDetails = {
  kind: "room_overlap",
  requested: {
    start_at: "2024-01-15T03:00:00.000Z",
    end_at: "2024-01-15T05:00:00.000Z",
    course_id: "c1",
    room_id: null,
    teacher_id: "t1",
  },
  conflicts: [
    {
      session_id: "sess1",
      course_id: "c1",
      teacher_id: "t1",
      room_id: null,
      start_at: "2024-01-15T03:00:00.000Z",
      end_at: "2024-01-15T04:00:00.000Z",
    },
  ],
  conflicting_students: [
    { student_id: "st1", full_name: "Alice", status: "draft" },
  ],
};

describe("PreflightIndicator", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("renders actionable guidance text when idle", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "idle" })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText(/Fill in/i)).toBeInTheDocument();
    expect(screen.getByText(/course/i)).toBeInTheDocument();
    expect(screen.getByText(/teacher/i)).toBeInTheDocument();
    expect(screen.getByText(/time/i)).toBeInTheDocument();
  });

  it("renders an animated spinner when loading", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "idle", loading: true })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    const spinner = screen.getByTestId("preflight-spinner");
    expect(spinner).toBeInTheDocument();
    expect(spinner.className).toContain("animate-spin");
  });

  it("renders a green check icon and 'Available' when status is available", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "available" })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText("Available")).toBeInTheDocument();
    expect(screen.getByTestId("preflight-check-icon")).toBeInTheDocument();
  });

  it("renders resource checklist when provisional", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "provisional" })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText("Provisional")).toBeInTheDocument();
    expect(screen.getByTestId("provisional-checklist")).toBeInTheDocument();
    expect(screen.getByTestId("checklist-student")).toHaveTextContent("Student ✅");
    expect(screen.getByTestId("checklist-teacher")).toHaveTextContent("Teacher ✅");
    expect(screen.getByTestId("checklist-room")).toHaveTextContent("Room ⏳");
  });

  it("renders 'Blocked' in red when status is blocked", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: sampleConflictDetails })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    const badge = screen.getByText("Blocked");
    expect(badge).toBeInTheDocument();
    expect(badge.closest(".text-red-700")).toBeInTheDocument();
  });

  it("renders 'No conflicts found' when available with details", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "available", details: sampleConflictDetails })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText("No conflicts found")).toBeInTheDocument();
  });

  it("renders conflict kind label when blocked with details", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: sampleConflictDetails })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText("Room already booked")).toBeInTheDocument();
  });

  it("renders conflicting sessions list", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: sampleConflictDetails })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText(/10:00–11:00/)).toBeInTheDocument();
    expect(screen.getAllByText(/jdoe – MATH101/).length).toBeGreaterThanOrEqual(1);
  });

  it("renders conflict session as a link to course page", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: sampleConflictDetails })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    const link = screen.getByRole("link", { name: /jdoe/ });
    expect(link).toHaveAttribute("href", "/courses/c1");
  });

  it("shows conflict count within the conflict group header", () => {
    const twoConflicts = {
      ...sampleConflictDetails,
      kind: "teacher_overlap",
      conflicts: [
        { session_id: "s1", course_id: "c1", teacher_id: "t1", room_id: null, start_at: "2024-01-15T03:00:00.000Z", end_at: "2024-01-15T04:00:00.000Z" },
        { session_id: "s2", course_id: "c1", teacher_id: "t1", room_id: null, start_at: "2024-01-15T05:00:00.000Z", end_at: "2024-01-15T06:00:00.000Z" },
      ],
    };
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: twoConflicts })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText("Teacher has another session")).toBeInTheDocument();
    const countElements = screen.getAllByText("2 conflicts");
    expect(countElements.length).toBeGreaterThanOrEqual(1);
  });

  it("renders conflicting students list when present", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: sampleConflictDetails })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText("Affected students (1)")).toBeInTheDocument();
    expect(screen.getByText("Alice")).toBeInTheDocument();
  });

  it("shows specific missing fields when requiredFields provided with empty values", () => {
    render(
        <PreflightIndicator
          preflight={makePreflight({ status: "idle" })}
          coursesById={coursesById}
          teachersById={teachersById}
          roomsById={roomsById}
          requiredFields={[
            { label: "Course", value: "" },
            { label: "Teacher", value: "t1" },
            { label: "Start", value: "" },
          ]}
        />
    );
    expect(screen.getByText(/Fill in:/)).toBeInTheDocument();
    expect(screen.getByText(/Course/)).toBeInTheDocument();
    expect(screen.getByText(/Start/)).toBeInTheDocument();
    expect(screen.queryByText(/Teacher/)).not.toBeInTheDocument();
  });

  it("shows 'Checking required fields...' when all requiredFields have values but still idle", () => {
    render(
        <PreflightIndicator
          preflight={makePreflight({ status: "idle" })}
          coursesById={coursesById}
          teachersById={teachersById}
          roomsById={roomsById}
          requiredFields={[
            { label: "Course", value: "c1" },
            { label: "Teacher", value: "t1" },
          ]}
        />
    );
    expect(screen.getByText(/Checking required fields/)).toBeInTheDocument();
  });

  it("shows generic message when no requiredFields provided", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "idle" })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText(/Fill in course, teacher, and time/)).toBeInTheDocument();
  });

  it("renders '+N more' when more than 3 conflicts", () => {
    const manyConflicts = {
      ...sampleConflictDetails,
      conflicts: Array.from({ length: 5 }, (_, i) => ({
        session_id: `s${i}`,
        course_id: "c1",
        teacher_id: "t1",
        room_id: null,
        start_at: "2024-01-15T03:00:00.000Z",
        end_at: "2024-01-15T04:00:00.000Z",
      })),
    };
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: manyConflicts })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    const group = screen.getByTestId("conflict-group");
    fireEvent.click(within(group).getByRole("button", { name: /5 conflicts/ }));
    expect(screen.getByText("+2 more conflicts")).toBeInTheDocument();
  });

  it("auto-expands conflict list when 2 or fewer conflicts", () => {
    const twoConflicts = {
      ...sampleConflictDetails,
      kind: "teacher_overlap",
      conflicts: [
        { session_id: "s1", course_id: "c1", teacher_id: "t1", room_id: null, start_at: "2024-01-15T03:00:00.000Z", end_at: "2024-01-15T04:00:00.000Z" },
        { session_id: "s2", course_id: "c1", teacher_id: "t1", room_id: null, start_at: "2024-01-15T05:00:00.000Z", end_at: "2024-01-15T06:00:00.000Z" },
      ],
    };
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: twoConflicts })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText(/12:00–13:00/)).toBeInTheDocument();
    expect(screen.getByText(/10:00–11:00/)).toBeInTheDocument();
  });

  it("collapses and expands conflict list on toggle", () => {
    const manyConflicts = {
      ...sampleConflictDetails,
      conflicts: Array.from({ length: 5 }, (_, i) => ({
        session_id: `s${i}`,
        course_id: "c1",
        teacher_id: "t1",
        room_id: null,
        start_at: "2024-01-15T03:00:00.000Z",
        end_at: "2024-01-15T04:00:00.000Z",
      })),
    };
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: manyConflicts })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.queryAllByText(/10:00–11:00/).length).toBe(0);
    const group = screen.getByTestId("conflict-group");
    fireEvent.click(within(group).getByRole("button", { name: /5 conflicts/ }));
    expect(screen.queryAllByText(/10:00–11:00/).length).toBeGreaterThan(0);
    fireEvent.click(within(group).getByRole("button", { name: /5 conflicts/ }));
    expect(screen.queryAllByText(/10:00–11:00/).length).toBe(0);
  });

  it("renders occurrences planned when provided", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "available", occurrencesPlanned: 8 })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText("Occurrences planned: 8")).toBeInTheDocument();
  });

  it("shows 'First blocked occurrence' for series (when occurrencesPlanned is set)", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", occurrencesPlanned: 5, details: sampleConflictDetails })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText("First blocked occurrence")).toBeInTheDocument();
  });

  it("shows 'Your requested time' when no occurrencesPlanned", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: sampleConflictDetails })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText("Your requested time")).toBeInTheDocument();
  });

  it("shows room name in requested section when room_id is set", () => {
    const detailsWithRoom: ConflictDetails = {
      ...sampleConflictDetails,
      requested: { ...sampleConflictDetails.requested, room_id: "r1" },
    };
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: detailsWithRoom })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText(/Room 101/)).toBeInTheDocument();
  });

  it("shows room name on conflict items for room_overlap", () => {
    const detailsWithRoomConflict: ConflictDetails = {
      ...sampleConflictDetails,
      kind: "room_overlap",
      conflicts: [{
        session_id: "sess1",
        course_id: "c1",
        teacher_id: "t1",
        room_id: "r1",
        start_at: "2024-01-15T03:00:00.000Z",
        end_at: "2024-01-15T04:00:00.000Z",
      }],
    };
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: detailsWithRoomConflict })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText(/Room 101/)).toBeInTheDocument();
  });

  it("renders suggestion text when blocked with details", () => {
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: sampleConflictDetails })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText("Try a different room or time slot")).toBeInTheDocument();
  });

  it("renders kind/detail/suggestion/requested when blocked with zero conflicts (availability violation)", () => {
    const availabilityDetails: ConflictDetails = {
      kind: "teacher_availability",
      requested: {
        start_at: "2024-01-15T03:00:00.000Z",
        end_at: "2024-01-15T05:00:00.000Z",
        course_id: "c1",
        room_id: null,
        teacher_id: "t1",
      },
      conflicts: [],
    };
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: availabilityDetails })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText("Teacher not available")).toBeInTheDocument();
    expect(screen.getByText("Select a time within the teacher's working hours")).toBeInTheDocument();
    expect(screen.getByText("Your requested time")).toBeInTheDocument();
    expect(screen.getByText(/10:00–12:00/)).toBeInTheDocument();
    expect(screen.queryByText(/conflicts?/i)).toBeNull();
  });

  it("renders kind/detail/suggestion/requested when blocked with null conflicts", () => {
    const nullConflictsDetails: ConflictDetails = {
      kind: "teacher_availability",
      requested: {
        start_at: "2024-01-15T03:00:00.000Z",
        end_at: "2024-01-15T05:00:00.000Z",
        course_id: "c1",
        room_id: null,
        teacher_id: "t1",
      },
      conflicts: null as unknown as Array<{ session_id: string; series_id?: string | null; course_id: string; room_id: string | null; teacher_id: string; start_at: string; end_at: string }>,
    };
    render(<PreflightIndicator preflight={makePreflight({ status: "blocked", details: nullConflictsDetails })} coursesById={coursesById} teachersById={teachersById} roomsById={roomsById} />);
    expect(screen.getByText("Teacher not available")).toBeInTheDocument();
    expect(screen.getByText("Select a time within the teacher's working hours")).toBeInTheDocument();
    expect(screen.getByText("Your requested time")).toBeInTheDocument();
  });
});

describe("getSaveButtonLabel", () => {
  it("returns 'Checking…' when loading", () => {
    expect(getSaveButtonLabel({ status: "idle", loading: true }, "Create")).toBe("Checking…");
  });

  it("returns 'Blocked — fix conflicts' when blocked without details", () => {
    expect(getSaveButtonLabel({ status: "blocked", loading: false }, "Save")).toBe("Blocked — fix conflicts");
  });

  it("returns kind-specific blocked text when details passed", () => {
    expect(getSaveButtonLabel({ status: "blocked", loading: false }, "Save", { kind: "teacher_overlap" })).toBe("Blocked — choose a different teacher or adjust the time");
    expect(getSaveButtonLabel({ status: "blocked", loading: false }, "Save", { kind: "room_overlap" })).toBe("Blocked — try a different room or time slot");
    expect(getSaveButtonLabel({ status: "blocked", loading: false }, "Save", { kind: "student_overlap" })).toBe("Blocked — reschedule or manage student attendance overrides");
    expect(getSaveButtonLabel({ status: "blocked", loading: false }, "Save", { kind: "unknown" })).toBe("Blocked — adjust the schedule to resolve conflicts");
  });

  it("returns submit label when available", () => {
    expect(getSaveButtonLabel({ status: "available", loading: false }, "Create")).toBe("Create");
  });

  it("returns submit label when provisional", () => {
    expect(getSaveButtonLabel({ status: "provisional", loading: false }, "Save")).toBe("Save");
  });
});

describe("isSaveDisabled", () => {
  it("disabled when loading", () => {
    expect(isSaveDisabled({ status: "idle", loading: true })).toBe(true);
  });

  it("disabled when blocked", () => {
    expect(isSaveDisabled({ status: "blocked", loading: false })).toBe(true);
  });

  it("enabled when available", () => {
    expect(isSaveDisabled({ status: "available", loading: false })).toBe(false);
  });

  it("enabled when provisional", () => {
    expect(isSaveDisabled({ status: "provisional", loading: false })).toBe(false);
  });

  it("disabled when idle", () => {
    expect(isSaveDisabled({ status: "idle", loading: false })).toBe(true);
  });
});
