import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import SessionActions from "@/components/SessionActions";
import type { Session } from "@/types";

const callbacks = {
  onAttendance: vi.fn(),
  onEdit: vi.fn(),
  onCancel: vi.fn(),
  onEditSeriesTandF: vi.fn(),
  onEditSeriesEntire: vi.fn(),
  onCancelSeries: vi.fn(),
};

function renderActions(startAt: string, seriesId: string | null = "series-1") {
  const session: Session = {
    id: "session-1",
    series_id: seriesId,
    course_id: "course-1",
    room_id: null,
    teacher_id: "teacher-1",
    start_at: startAt,
    end_at: "2026-08-03T12:00:00Z",
    version: 1,
  };

  render(<SessionActions session={session} cancelingId={null} {...callbacks} />);
}

describe("SessionActions historical series controls", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-03T10:00:00Z"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it.each([
    ["at the current instant", "2026-08-03T10:00:00Z"],
    ["before the current instant", "2026-08-03T09:59:59Z"],
  ])("hides This & Future for a series session %s", (_label, startAt) => {
    renderActions(startAt);
    expect(screen.queryByRole("button", { name: "This & Future" })).not.toBeInTheDocument();
  });

  it("shows This & Future for a future series session", () => {
    renderActions("2026-08-03T10:00:01Z");
    expect(screen.getByRole("button", { name: "This & Future" })).toBeInTheDocument();
  });

  it("hides This & Future for a future standalone session", () => {
    renderActions("2026-08-03T10:00:01Z", null);
    expect(screen.queryByRole("button", { name: "This & Future" })).not.toBeInTheDocument();
  });
});
