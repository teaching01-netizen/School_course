import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { StudentConflictPopover } from "./StudentConflictPopover";

const conflict = {
  kind: "student_overlap" as const,
  current_session_id: "session-1",
  current_start_at: "2026-05-27T02:00:00Z",
  current_end_at: "2026-05-27T04:00:00Z",
  conflicting_session_id: "session-2",
  conflicting_course_id: "course-2",
  conflicting_course_code: "ENG-202",
  conflicting_course_name: "English 202",
  conflicting_start_at: "2026-05-27T02:30:00Z",
  conflicting_end_at: "2026-05-27T04:00:00Z",
};

describe("StudentConflictPopover", () => {
  it("shows a clear state without an interactive conflict control", () => {
    render(<StudentConflictPopover conflicts={[]} zone="Asia/Bangkok" />);

    expect(screen.getByText("Clear")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /student conflict/i })).not.toBeInTheDocument();
  });

  it("opens student overlap details and links to the conflicting course", async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <StudentConflictPopover conflicts={[conflict]} zone="Asia/Bangkok" />
      </MemoryRouter>,
    );

    await user.click(screen.getByRole("button", { name: "Show 1 student conflict" }));

    expect(screen.getByText("Student schedule conflict")).toBeInTheDocument();
    expect(screen.getByText(/ENG-202 · English 202/)).toHaveAttribute("href", "/courses/course-2");
  });
});
