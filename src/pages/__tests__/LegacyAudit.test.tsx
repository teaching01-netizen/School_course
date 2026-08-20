import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import LegacyAudit from "../LegacyAudit";
import { ToastProvider } from "../../hooks/useToast";
import type { LegacyAudit as LegacyAuditData } from "../LegacyAudit.model";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("../../hooks/useAuth", () => ({
  useAuth: () => ({ user: { id: "admin-1", username: "Admin", role: "Admin" }, loading: false }),
}));

const auditFixture: LegacyAuditData = {
  generated_at: "2026-08-20T09:00:00Z",
  totals: {
    linked_courses: 8,
    archived_courses: 1,
    synced_courses: 7,
    legacy_sessions: 42,
    active_sessions: 40,
    soft_deleted_sessions: 2,
    external_series: 6,
    students_imported: 5,
    mapped_rooms: 9,
    mapped_teachers: 7,
    mapped_subjects: 2,
  },
  runs: {
    completed_runs: 4,
    entities_parsed: 1200,
    entities_applied: 1180,
    parse_failures: 2,
    reconciliation_mismatches: 0,
    last_successful_at: "2026-08-20T08:30:00Z",
  },
  skips: {
    sessions_skipped_total: 3,
    sessions_skipped_open: 2,
    courses_skipped_total: 1,
    courses_skipped_open: 1,
    partial_snapshots: 1,
    by_cause: [
      { cause: "open_conflict", entity_type: "course", key: "room_overlap", count: 2 },
      { cause: "dead_letter", entity_type: "course", key: "database_constraint", count: 1 },
      { cause: "partial_snapshot", entity_type: "course", key: "partial", count: 1 },
    ],
  },
  skipped_sessions: [
    {
      legacy_schedule_id: "sched-101",
      date: "2026-08-15",
      begin: "18:00",
      end: "19:30",
      classroom: "A1",
      conflict_type: "room_overlap",
      category: "database_constraint",
      message: "legacy schedule sched-101 (2026-08-15 18:00-19:30) skipped: room overlap",
      status: "open",
      created_at: "2026-08-15T05:00:00Z",
      course_id: null,
      course_code: "ENG-1",
      course_name: "English 1",
      legacy_course_id: "7306",
    },
  ],
  skipped_courses: [
    {
      reason_kind: "conflict",
      external_id: "7310",
      conflict_type: "code_claimed",
      error_category: null,
      message: "local course code MATH-2 is already linked to legacy course 7309",
      status: "open",
      created_at: "2026-08-10T05:00:00Z",
      course_id: null,
      course_code: "MATH-2",
      course_name: null,
    },
  ],
  dead_letters: [
    {
      id: "dead-1",
      job_type: "legacy_refresh_course",
      entity_type: "course",
      external_id: "7320",
      error_category: "database_constraint",
      last_error: "insert or update on table courses violates constraint",
      attempts: 11,
      created_at: "2026-08-19T05:00:00Z",
    },
  ],
};

function renderPage(client = new QueryClient({ defaultOptions: { queries: { retry: false } } })) {
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ToastProvider>
          <LegacyAudit />
        </ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("Legacy data audit", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string) => {
      if (path === "/api/v1/admin/legacy-sync/audit") return Promise.resolve(auditFixture);
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("renders import totals from the audit endpoint", async () => {
    renderPage();
    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith("/api/v1/admin/legacy-sync/audit"));
    expect(await screen.findByText("8")).toBeTruthy();
    expect(screen.getByText("Linked courses")).toBeTruthy();
    expect(screen.getByText("40")).toBeTruthy();
    expect(screen.getByText("Legacy sessions")).toBeTruthy();
    expect(screen.getByText("5")).toBeTruthy();
    expect(screen.getByText("Students imported")).toBeTruthy();
  });

  it("shows skip totals, the by-cause breakdown, and skipped session/course rows", async () => {
    renderPage();
    expect(await screen.findByText("Skipped data")).toBeTruthy();
    expect(screen.getByText("3")).toBeTruthy(); // skipped sessions total
    expect(screen.getByText("2 open · recorded in conflicts")).toBeTruthy();
    // By-cause table
    expect(screen.getByText("Open conflicts")).toBeTruthy();
    expect((await screen.findAllByText("Room overlap")).length).toBeGreaterThan(0);
    expect((await screen.findAllByText("Skipped sessions")).length).toBeGreaterThan(0);
    expect(await screen.findByText("English 1")).toBeTruthy();
    expect(screen.getByText("sched-101")).toBeTruthy();
    // Skipped courses section shows the code-claimed conflict
    expect((await screen.findAllByText("Skipped courses")).length).toBeGreaterThan(0);
    expect(screen.getByText("Course code already linked")).toBeTruthy();
    // Dead letters
    expect((await screen.findAllByText("Dead letters")).length).toBeGreaterThan(0);
    expect(screen.getByText("legacy_refresh_course")).toBeTruthy();
  });

  it("shows run totals and the last successful run", async () => {
    renderPage();
    expect(await screen.findByText("Completed runs")).toBeTruthy();
    expect(screen.getByText("4")).toBeTruthy();
    expect(screen.getByText("1,200")).toBeTruthy(); // entities parsed formatted
    expect(screen.getByText(/Last successful run/)).toBeTruthy();
  });

  it("surfaces API errors with a retry action", async () => {
    mockApiJson.mockImplementation(() => Promise.reject(new Error("database unavailable")));
    renderPage();
    expect(await screen.findByText("Audit data unavailable")).toBeTruthy();
    expect(screen.getByText("database unavailable")).toBeTruthy();
    expect(screen.getByText("Try again")).toBeTruthy();
  });
});