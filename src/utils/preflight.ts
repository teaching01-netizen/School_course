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
  if (form.duration_minutes <= 0) {
    return null;
  }
  if (useCount) {
    if (!Number.isFinite(form.count) || form.count <= 0) {
      return null;
    }
  } else {
    if (!form.end_date) {
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
