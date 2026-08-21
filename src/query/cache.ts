import { QueryClient } from "@tanstack/react-query";
import { ApiRequestError } from "@/api/client";

export const cachePolicies = {
  reference: {
    staleTime: 5 * 60_000,
    gcTime: 30 * 60_000,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
  },
  semiStatic: {
    staleTime: 60_000,
    gcTime: 10 * 60_000,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
  },
  operational: {
    staleTime: 0,
    gcTime: 5 * 60_000,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
  },
  sensitiveDetail: {
    staleTime: 15_000,
    gcTime: 60_000,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
  },
} as const;

export const queryKeys = {
  sessions: {
    all: ["sessions"] as const,
    list: (request: string) => ["sessions", "list", request] as const,
  },
  attendance: {
    all: ["attendance"] as const,
    detail: (sessionID: string) => ["attendance", sessionID] as const,
  },
  operationsCalendar: {
    all: ["operations-calendar"] as const,
    range: (request: string | null) => ["operations-calendar", request] as const,
  },
  absences: {
    all: ["absences"] as const,
    list: (request: string) => ["absences", "list", request] as const,
    detail: (absenceID: string) => ["absences", "detail", absenceID] as const,
    teacherDetail: (absenceID: string) => ["absences", "teacher-detail", absenceID] as const,
  },
  absenceStats: ["absence-stats"] as const,
  legacySync: {
    all: ["legacy-sync"] as const,
    health: ["legacy-sync", "health"] as const,
    audit: ["legacy-sync", "audit"] as const,
    auditSummary: ["legacy-sync", "audit-summary"] as const,
    auditSkippedSessions: (limit: number, offset: number) => ["legacy-sync", "audit-skipped-sessions", limit, offset] as const,
    auditSkippedCourses: (limit: number, offset: number) => ["legacy-sync", "audit-skipped-courses", limit, offset] as const,
    auditDeadLetters: (limit: number, offset: number) => ["legacy-sync", "audit-dead-letters", limit, offset] as const,
    runs: ["legacy-sync", "runs"] as const,
    run: (id: string) => ["legacy-sync", "run", id] as const,
    runProgress: (id: string) => ["legacy-sync", "run-progress", id] as const,
    jobs: ["legacy-sync", "jobs"] as const,
    jobsPaginated: (limit: number, offset: number) => ["legacy-sync", "jobs-paginated", limit, offset] as const,
    conflicts: ["legacy-sync", "conflicts"] as const,
    conflictsPaginated: (limit: number, offset: number) => ["legacy-sync", "conflicts-paginated", limit, offset] as const,
    conflictDetail: (id: string) => ["legacy-sync", "conflict", id] as const,
  },
  teacherDashboards: {
    all: ["teacher-dashboards"] as const,
    detail: (teacherID: string, monthStart: string) => ["teacher-dashboards", teacherID, monthStart] as const,
  },
  courses: {
    all: ["courses"] as const,
    list: (request = "") => ["courses", "list", request] as const,
  },
  courseRosters: {
    all: ["course-rosters"] as const,
    detail: (courseID: string) => ["course-rosters", courseID] as const,
  },
  api: (url: string, deps: unknown[] = []) => ["api", url, ...deps] as const,
};

export function queryKeyForURL(url: string, deps: unknown[] = []): readonly unknown[] {
  if (url.startsWith("/api/v1/teacher/dashboard")) {
    return [...queryKeys.teacherDashboards.all, url, ...deps];
  }
  const teacherAbsenceMatch = url.match(/^\/api\/v1\/teacher\/absences\/([^/?]+)/);
  if (teacherAbsenceMatch) {
    return [...queryKeys.absences.teacherDetail(teacherAbsenceMatch[1]), ...deps];
  }
  if (url.startsWith("/api/v1/courses")) return [...queryKeys.courses.all, "api", url, ...deps];
  if (url.startsWith("/api/v1/subjects") || url.startsWith("/api/v1/rooms") || url.startsWith("/api/v1/users")) {
    return ["reference", url, ...deps];
  }
  if (url.startsWith("/api/v1/students")) return ["students", url, ...deps];
  return queryKeys.api(url, deps);
}

export function cachePolicyForURL(url: string) {
  if (url.startsWith("/api/v1/teacher/dashboard")) return cachePolicies.operational;
  if (url.startsWith("/api/v1/teacher/absences/") || /^\/api\/v1\/absences\/[^/?]+$/.test(url)) {
    return cachePolicies.sensitiveDetail;
  }
  if (
    url.startsWith("/api/v1/subjects") ||
    url.startsWith("/api/v1/rooms") ||
    url.startsWith("/api/v1/users?role=Teacher")
  ) {
    return cachePolicies.reference;
  }
  return cachePolicies.semiStatic;
}

export function createAppQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: (failureCount, error) => {
          if (error instanceof ApiRequestError && error.status != null && error.status >= 400 && error.status < 500) {
            return false;
          }
          return failureCount < 1;
        },
      },
      mutations: { retry: false },
    },
  });
}

export function clearCacheForUserChange(
  client: QueryClient,
  previousUserID: string | null,
  nextUserID: string | null,
): void {
  if (previousUserID !== nextUserID) client.clear();
}

export const queryClient = createAppQueryClient();
