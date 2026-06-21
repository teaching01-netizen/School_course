import { queryKeys } from "@/query/cache";

type RealtimeEventLike = {
  type: string;
  channel: string;
  id?: string;
  payload?: unknown;
};

export function invalidationKeysForEvent(event: RealtimeEventLike): readonly (readonly unknown[])[] {
  switch (event.channel) {
    case "sessions:all":
      return [
        queryKeys.sessions.all,
        queryKeys.attendance.all,
        queryKeys.operationsCalendar.all,
        queryKeys.teacherDashboards.all,
      ];
    case "absent:all":
      return [
        queryKeys.absences.all,
        ...(event.id ? [queryKeys.absences.detail(event.id)] : []),
        queryKeys.absenceStats,
        queryKeys.operationsCalendar.all,
        queryKeys.teacherDashboards.all,
      ];
    case "absent:stats":
      return [queryKeys.absenceStats];
    case "courses:all":
      return [queryKeys.courses.all, ...(event.id ? [queryKeys.courseRosters.detail(event.id)] : [queryKeys.courseRosters.all])];
    default:
      return [];
  }
}
