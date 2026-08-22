import { apiJson } from "@/api/client";
import { cachePolicyForURL, queryClient, queryKeyForURL } from "./cache";

/**
 * Intent prefetching: warm the query cache and the lazy route chunk while the
 * user is still deciding to click (hover/focus on a nav link). Landing on
 * cached data removes the navigation round trip; a miss simply falls back to
 * normal loading, so entries can never show wrong data.
 *
 * The request URLs must byte-match what the target page issues, because query
 * keys are URL-derived. Keep them in sync with the pages' default request
 * construction.
 */
const routeRequests: Record<string, readonly string[]> = {
  "/absences": ["/api/v1/absences?limit=25&offset=0&bucket=active"],
  "/courses": ["/api/v1/courses?limit=50&offset=0"],
  "/students": ["/api/v1/students?limit=50&offset=0"],
  "/teachers": ["/api/v1/users?role=Teacher"],
  "/subjects": ["/api/v1/subjects"],
  "/classrooms": ["/api/v1/rooms"],
};

/** Same dynamic-import specifiers as the lazy routes in App.tsx; Vite reuses the chunks. */
const routeChunks: Record<string, () => Promise<unknown>> = {
  "/absences": () => import("../pages/Absences"),
  "/courses": () => import("../pages/Courses"),
  "/students": () => import("../pages/Students"),
  "/teachers": () => import("../pages/Teachers"),
  "/subjects": () => import("../pages/Subjects"),
  "/classrooms": () => import("../pages/Classrooms"),
};

export function prefetchRoute(pathname: string): void {
  void routeChunks[pathname]?.();
  for (const url of routeRequests[pathname] ?? []) {
    void queryClient.prefetchQuery({
      queryKey: queryKeyForURL(url),
      queryFn: () => apiJson(url, { method: "GET" }),
      ...cachePolicyForURL(url),
    });
  }
}
