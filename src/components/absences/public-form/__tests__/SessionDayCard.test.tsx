import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import SessionDayCard, { type SessionDayGroup } from "../SessionDayCard";

const day: SessionDayGroup = {
  id: "day-1",
  date: "2026-08-19",
  start_at: "2026-08-19T03:00:00Z",
  end_at: "2026-08-19T05:00:00Z",
  items: [{ id: "session-1", date: "2026-08-19", start_at: "2026-08-19T03:00:00Z", end_at: "2026-08-19T05:00:00Z", already_absent: false }],
};

describe("SessionDayCard", () => {
  it("makes the session row the touch target while preserving a real checkbox", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(<SessionDayCard dayGroup={day} selected={false} alreadyAbsent={false} disabled={false} onToggle={onToggle} />);

    const checkbox = screen.getByRole("checkbox", { name: /19 Aug/i });
    expect(checkbox).toHaveAttribute("id", "session-day-1");
    await user.click(screen.getByText(/19 Aug/i));
    expect(onToggle).toHaveBeenCalledOnce();
  });

  it("reveals dependent make-up content only after selection", () => {
    render(
      <SessionDayCard dayGroup={day} selected alreadyAbsent={false} disabled={false} onToggle={vi.fn()}>
        <p>Choose a make-up class</p>
      </SessionDayCard>,
    );

    expect(screen.getByText("Choose a make-up class")).toBeInTheDocument();
  });
});
