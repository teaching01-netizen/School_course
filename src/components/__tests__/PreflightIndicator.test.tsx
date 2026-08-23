import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import {
  getSaveButtonLabel,
  isSaveDisabled,
  PreflightIndicator,
} from "@/components/PreflightIndicator";
import type { UsePreflightReturn, PreflightParams } from "@/features/scheduling/hooks/usePreflight";
import type { ConflictDetails } from "@/features/scheduling/types";

function makePreflight(overrides: Partial<UsePreflightReturn>): UsePreflightReturn {
  return {
    status: "idle",
    loading: false,
    details: null,
    warnings: [],
    error: null,
    occurrencesPlanned: null,
    lastParams: null,
    check: vi.fn(async (_params: PreflightParams): Promise<void> => undefined),
    reset: vi.fn(),
    ...overrides,
  };
}

const coursesById = new Map([[
  "course-1",
  { id: "course-1", version: 1, code: "MATH-101", name: "Math", primary_teacher_id: null },
]]);
const teachersById = new Map([[
  "teacher-1",
  { id: "teacher-1", username: "teacher.one", role: "Teacher" as const },
]]);

const roomConflict: ConflictDetails = {
  kind: "room_overlap",
  requested: {
    start_at: "2026-08-05T03:00:00.000Z",
    end_at: "2026-08-05T04:00:00.000Z",
    course_id: "course-1",
    room_id: "room-1",
    teacher_id: "teacher-1",
  },
  conflicts: [],
};

const lastParams: PreflightParams = {
  course_id: "course-1",
  teacher_id: "teacher-1",
  room_id: "room-1",
  start_at: "2026-08-05T03:00:00.000Z",
  end_at: "2026-08-05T04:00:00.000Z",
};

function renderIndicator(preflight: UsePreflightReturn) {
  return render(
    <MemoryRouter>
      <PreflightIndicator
        preflight={preflight}
        coursesById={coursesById}
        teachersById={teachersById}
      />
    </MemoryRouter>,
  );
}

describe("PreflightIndicator user stories", () => {
  it("distinguishes a system error from a blocked conflict and keeps save disabled", () => {
    const systemError = makePreflight({ status: "error" });
    const rendered = renderIndicator(systemError);

    expect(screen.getByRole("alert")).toHaveTextContent("Could not check the schedule");
    expect(screen.queryByText("Blocked")).not.toBeInTheDocument();
    expect(getSaveButtonLabel(systemError, "Save")).toBe("Unavailable — check schedule");
    expect(isSaveDisabled(systemError)).toBe(true);

    rendered.rerender(
      <MemoryRouter>
        <PreflightIndicator
          preflight={makePreflight({ status: "blocked", details: roomConflict })}
          coursesById={coursesById}
          teachersById={teachersById}
        />
      </MemoryRouter>,
    );

    expect(screen.getByText("Blocked")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(getSaveButtonLabel({ status: "blocked", loading: false }, "Save")).toBe("Blocked — fix conflicts");
    expect(isSaveDisabled({ status: "blocked", loading: false })).toBe(true);
  });

  it("retries with the current last preflight parameters", () => {
    const check = vi.fn(async (_params: PreflightParams): Promise<void> => undefined);
    const preflight = makePreflight({ status: "error", lastParams, check });
    renderIndicator(preflight);

    fireEvent.click(screen.getByRole("button", { name: "Try checking availability again" }));

    expect(check).toHaveBeenCalledTimes(1);
    expect(check).toHaveBeenCalledWith(lastParams);
  });

  it("preserves the exact HTTP conflict total when the visible list is truncated", () => {
    const conflicts = Array.from({ length: 25 }, (_, index) => ({
      session_id: `session-${index}`,
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      start_at: "2026-08-05T03:00:00.000Z",
      end_at: "2026-08-05T04:00:00.000Z",
    }));
    const details: ConflictDetails = {
      ...roomConflict,
      conflicts,
      total_conflicts: 30,
      conflicts_truncated: true,
    };

    renderIndicator(makePreflight({ status: "blocked", details }));

    expect(screen.getByText("Showing 25 of 30 conflicts")).toBeInTheDocument();
    const toggle = screen.getByRole("button", { name: /Showing 25 of 30/ });
    fireEvent.click(toggle);
    expect(screen.getByText("+27 more")).toBeInTheDocument();
  });

  it("gives a blocked save an accessible reason from the conflict kind", () => {
    const preflight = makePreflight({ status: "blocked", details: roomConflict });

    expect(getSaveButtonLabel(preflight, "Save", roomConflict)).toBe(
      "Blocked — try a different room or time slot",
    );
    expect(isSaveDisabled(preflight)).toBe(true);
  });

  it("shows allowed conflicts in red while keeping the save enabled", () => {
    const warning = {
      rule: "room_overlap" as const,
      code: "room_overlap",
      message: "Room 101 is already in use.",
      details: roomConflict,
    };
    const preflight = makePreflight({ status: "warning", details: roomConflict, warnings: [warning] });
    renderIndicator(preflight);

    expect(screen.getByTestId("preflight-warning")).toHaveTextContent("Room 101 is already in use.");
    expect(getSaveButtonLabel(preflight, "Save")).toBe("Save with warnings");
    expect(isSaveDisabled(preflight)).toBe(false);
  });
});
