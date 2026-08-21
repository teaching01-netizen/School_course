import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { SessionConflictPopover } from "./SessionConflictPopover";

const conflict = {
  kind: "room_overlap" as const,
  resource: "room" as const,
  conflicting_session_id: "session-2",
  conflicting_course_id: "course-2",
  conflicting_course_code: "ENG-202",
  conflicting_course_name: "English 202",
  conflicting_start_at: "2026-05-27T02:30:00Z",
  conflicting_end_at: "2026-05-27T04:00:00Z",
};

describe("SessionConflictPopover", () => {
  it("shows a clear state without an interactive conflict control", () => {
    render(<SessionConflictPopover conflicts={[]} currentCourseId="course-1" zone="Asia/Bangkok" />);

    expect(screen.getByText("Clear")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /session conflict/i })).not.toBeInTheDocument();
  });

  it("opens conflict details and links to the conflicting course", async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <SessionConflictPopover conflicts={[conflict]} currentCourseId="course-1" zone="Asia/Bangkok" />
      </MemoryRouter>,
    );

    await user.click(screen.getByRole("button", { name: "Show 1 session conflict" }));

    expect(screen.getByText("Room overlap")).toBeInTheDocument();
    expect(screen.getByText(/ENG-202 · English 202/)).toHaveAttribute("href", "/courses/course-2");
  });
});
