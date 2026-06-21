import { describe, expect, it } from "vitest";
import { cachePolicies, clearCacheForUserChange, createAppQueryClient, queryKeyForURL, queryKeys } from "./cache";

describe("cache policies", () => {
  it("keeps operational data visible but immediately stale with visible safety polling", () => {
    expect(cachePolicies.operational).toMatchObject({
      staleTime: 0,
      gcTime: 5 * 60_000,
      refetchInterval: 30_000,
      refetchIntervalInBackground: false,
      refetchOnWindowFocus: true,
      refetchOnReconnect: true,
    });
  });

  it("uses longer memory-only lifetimes for reference and semi-static data", () => {
    expect(cachePolicies.reference).toMatchObject({ staleTime: 5 * 60_000, gcTime: 30 * 60_000 });
    expect(cachePolicies.semiStatic).toMatchObject({ staleTime: 60_000, gcTime: 10 * 60_000 });
    expect(cachePolicies.sensitiveDetail).toMatchObject({ staleTime: 15_000, gcTime: 60_000 });
  });
});

describe("query keys", () => {
  it("groups operational projections under stable domain prefixes", () => {
    expect(queryKeys.sessions.all).toEqual(["sessions"]);
    expect(queryKeys.absences.all).toEqual(["absences"]);
    expect(queryKeys.teacherDashboards.all).toEqual(["teacher-dashboards"]);
    expect(queryKeys.courses.all).toEqual(["courses"]);
  });

  it("includes request identity in list and detail keys", () => {
    expect(queryKeys.sessions.list("a=1")).toEqual(["sessions", "list", "a=1"]);
    expect(queryKeys.absences.detail("absence-1")).toEqual(["absences", "detail", "absence-1"]);
    expect(queryKeys.teacherDashboards.detail("teacher-1", "2026-06-01")).toEqual([
      "teacher-dashboards",
      "teacher-1",
      "2026-06-01",
    ]);
  });

  it("routes compatibility-hook URLs into invalidatable domain families", () => {
    expect(queryKeyForURL("/api/v1/teacher/dashboard?month_start=2026-06-01", [])).toEqual([
      "teacher-dashboards",
      "/api/v1/teacher/dashboard?month_start=2026-06-01",
    ]);
    expect(queryKeyForURL("/api/v1/teacher/absences/absence-1", [])).toEqual([
      "absences",
      "teacher-detail",
      "absence-1",
    ]);
    expect(queryKeyForURL("/api/v1/courses", [])).toEqual(["courses", "api", "/api/v1/courses"]);
  });
});

describe("authenticated cache isolation", () => {
  it("clears memory cache whenever the authenticated user identity changes", () => {
    const client = createAppQueryClient();
    client.setQueryData(["private"], { student: "sensitive" });
    clearCacheForUserChange(client, "user-a", "user-b");
    expect(client.getQueryData(["private"])).toBeUndefined();
  });

  it("preserves cache when the same authenticated identity rerenders", () => {
    const client = createAppQueryClient();
    client.setQueryData(["private"], { stable: true });
    clearCacheForUserChange(client, "user-a", "user-a");
    expect(client.getQueryData(["private"])).toEqual({ stable: true });
  });
});
