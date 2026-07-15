import {
  MAX_SERIES_HORIZON_YEARS,
  MAX_SERIES_OCCURRENCES,
  MAX_SESSION_DURATION_MINUTES,
} from "@/features/scheduling/recurrenceLimits";

export type SeriesPreflightForm = {
  course_id: string;
  room_id: string;
  teacher_id: string;
  weekdays: boolean[];
  start_local_time: string;
  duration_minutes: number;
  start_date: string;
  end_date: string;
  count: number;
};

export type SeriesPreflightValidation = {
  weekdays: number[];
  end_date: string | null;
  count: number | null;
  room_id: string | null;
};

export function validateSeriesPreflight(
  form: SeriesPreflightForm,
  useCount: boolean
): SeriesPreflightValidation | null {
  const weekdays = form.weekdays
    .map((v, idx) => (v ? idx : null))
    .filter((v): v is number => v != null);

  if (!form.course_id || !form.teacher_id || weekdays.length === 0 || !form.start_local_time || !form.start_date) {
    return null;
  }
  if (
    !Number.isInteger(form.duration_minutes) ||
    form.duration_minutes <= 0 ||
    form.duration_minutes > MAX_SESSION_DURATION_MINUTES
  ) {
    return null;
  }
  if (useCount) {
    if (
      !Number.isInteger(form.count) ||
      form.count <= 0 ||
      form.count > MAX_SERIES_OCCURRENCES ||
      !countFitsHorizon(form.start_date, weekdays, form.count)
    ) {
      return null;
    }
  } else {
    if (!form.end_date || !endDateFitsHorizon(form.start_date, form.end_date)) {
      return null;
    }
  }

  return {
    weekdays,
    end_date: useCount ? null : form.end_date,
    count: useCount ? form.count : null,
    room_id: form.room_id ? form.room_id : null,
  };
}

function parseDateOnly(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return null;

  const date = new Date(Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3])));
  return date.getUTCFullYear() === Number(match[1]) &&
    date.getUTCMonth() === Number(match[2]) - 1 &&
    date.getUTCDate() === Number(match[3])
    ? date
    : null;
}

function horizonFrom(start: Date): Date {
  const horizon = new Date(start);
  horizon.setUTCFullYear(horizon.getUTCFullYear() + MAX_SERIES_HORIZON_YEARS);
  return horizon;
}

function endDateFitsHorizon(startDate: string, endDate: string): boolean {
  const start = parseDateOnly(startDate);
  const end = parseDateOnly(endDate);
  if (!start || !end) return false;
  return end >= start && end <= horizonFrom(start);
}

function countFitsHorizon(startDate: string, weekdays: number[], count: number): boolean {
  const cursor = parseDateOnly(startDate);
  if (!cursor) return false;
  const horizon = horizonFrom(cursor);
  const allowedWeekdays = new Set(weekdays);
  let occurrences = 0;

  while (cursor <= horizon) {
    if (allowedWeekdays.has(cursor.getUTCDay())) {
      occurrences += 1;
      if (occurrences === count) return true;
    }
    cursor.setUTCDate(cursor.getUTCDate() + 1);
  }

  return false;
}
