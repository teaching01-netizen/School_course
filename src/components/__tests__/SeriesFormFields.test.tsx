import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import SeriesFormFields from "@/components/SeriesFormFields";
import {
  MAX_SERIES_OCCURRENCES,
  MAX_SESSION_DURATION_MINUTES,
} from "@/features/scheduling/recurrenceLimits";
import { validateSeriesPreflight, type SeriesPreflightForm } from "@/utils/preflight";

function renderFields(useCount = true) {
  render(
    <SeriesFormFields
      weekdays={[false, true, false, false, false, false, false]}
      onWeekdayChange={vi.fn()}
      startLocalTime="09:00"
      onStartLocalTimeChange={vi.fn()}
      durationMinutes={60}
      onDurationMinutesChange={vi.fn()}
      useCount={useCount}
      onUseCountChange={vi.fn()}
      count={10}
      onCountChange={vi.fn()}
      endDate="2026-08-10"
      onEndDateChange={vi.fn()}
      startDate="2026-08-03"
      onStartDateChange={vi.fn()}
    />,
  );
}

const validForm: SeriesPreflightForm = {
  course_id: "course-1",
  room_id: "",
  teacher_id: "teacher-1",
  weekdays: [false, true, false, false, false, false, false],
  start_local_time: "09:00",
  duration_minutes: 60,
  start_date: "2026-08-03",
  end_date: "2031-08-03",
  count: 10,
};

describe("SeriesFormFields recurrence limits", () => {
  it("exposes native duration and count limits", () => {
    renderFields();

    expect(screen.getByLabelText("Duration (minutes)")).toHaveAttribute("min", "1");
    expect(screen.getByLabelText("Duration (minutes)")).toHaveAttribute(
      "max",
      String(MAX_SESSION_DURATION_MINUTES),
    );
    expect(screen.getByLabelText("Count (total occurrences)")).toHaveAttribute("min", "1");
    expect(screen.getByLabelText("Count (total occurrences)")).toHaveAttribute(
      "max",
      String(MAX_SERIES_OCCURRENCES),
    );
  });

  it("rejects duration and count values above the shared limits", () => {
    expect(
      validateSeriesPreflight(
        { ...validForm, duration_minutes: MAX_SESSION_DURATION_MINUTES + 1 },
        true,
      ),
    ).toBeNull();
    expect(
      validateSeriesPreflight({ ...validForm, count: MAX_SERIES_OCCURRENCES + 1 }, true),
    ).toBeNull();
  });

  it("accepts the five-year horizon boundary and rejects dates beyond it", () => {
    expect(validateSeriesPreflight(validForm, false)).not.toBeNull();
    expect(
      validateSeriesPreflight({ ...validForm, end_date: "2031-08-04" }, false),
    ).toBeNull();
  });
});
