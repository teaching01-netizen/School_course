export const scheduleImpactRoutes = {
  schedule: "/schedule",
  scheduleImpact: "/operations/schedule-impact",
  sessionChangeDetail: (id: string) => `/operations/session-changes/${id}`,
  absenceSettings: "/admin/absence-settings",
  notificationSettings: "/admin/absence-settings?section=notifications",
} as const;
