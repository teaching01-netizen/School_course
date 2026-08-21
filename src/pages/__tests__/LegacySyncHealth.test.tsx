import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClientProvider } from "@tanstack/react-query";
import LegacySyncHealth from "../LegacySyncHealth";
import { ToastProvider } from "../../hooks/useToast";
import { queryClient } from "@/query/cache";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("../../hooks/useAuth", () => ({
  useAuth: () => ({ user: { id: "admin-1", username: "Admin", role: "Admin" }, loading: false }),
}));

const controlFixture = { detection_enabled: true, fetch_enabled: true, apply_enabled: true, student_enabled: false, tombstone_enabled: false, realtime_enabled: true, shadow_mode: true };

function renderPage() {
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ToastProvider>
          <LegacySyncHealth />
        </ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("Legacy sync health shadow toggle", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string) => {
      if (path === "/api/v1/admin/legacy-sync/health") {
        return Promise.resolve({
          status: "shadow",
          paused: false,
          shadow_mode: true,
          control: controlFixture,
          queue: { queued: 1, running: 0, completed: 0, dead: 0 },
          open_conflicts: 0,
          latest_run: null,
          last_successful_at: null,
          freshness_seconds: null,
        });
      }
      if (path === "/api/v1/admin/legacy-sync/jobs?limit=12") return Promise.resolve([]);
      if (path === "/api/v1/admin/legacy-sync/conflicts") return Promise.resolve([]);
      if (path === "/api/v1/admin/legacy-sync/shadow") {
        return Promise.resolve({ ...controlFixture, shadow_mode: false });
      }
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("offers a shadow mode toggle that posts the new state", async () => {
    renderPage();
    const toggle = await screen.findByRole("button", { name: "Disable shadow mode" });
    expect(screen.getByText("Shadow mode is on: reconciles observe the legacy site but never create, link, or update local courses.")).toBeInTheDocument();

    fireEvent.click(toggle);

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith("/api/v1/admin/legacy-sync/shadow", {
        method: "POST",
        body: JSON.stringify({ enabled: false }),
      });
    });
  });

  it("shows a turn-on control when shadow mode is off", async () => {
    mockApiJson.mockImplementation((path: string) => {
      if (path === "/api/v1/admin/legacy-sync/health") {
        return Promise.resolve({
          status: "healthy",
          paused: false,
          shadow_mode: false,
          control: { ...controlFixture, shadow_mode: false },
          queue: { queued: 0, running: 0, completed: 0, dead: 0 },
          open_conflicts: 0,
          latest_run: null,
          last_successful_at: null,
          freshness_seconds: null,
        });
      }
      if (path === "/api/v1/admin/legacy-sync/jobs?limit=12") return Promise.resolve([]);
      if (path === "/api/v1/admin/legacy-sync/conflicts") return Promise.resolve([]);
      throw new Error(`Unexpected API call: ${path}`);
    });
    renderPage();
    expect(await screen.findByRole("button", { name: "Enable shadow mode" })).toBeInTheDocument();
    expect(screen.queryByText(/Shadow mode is on/)).not.toBeInTheDocument();
  });
});

describe("Legacy sync health student import toggle", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string) => {
      if (path === "/api/v1/admin/legacy-sync/health") {
        return Promise.resolve({
          status: "shadow",
          paused: false,
          shadow_mode: true,
          control: controlFixture,
          queue: { queued: 0, running: 0, completed: 0, dead: 0 },
          open_conflicts: 0,
          latest_run: null,
          last_successful_at: null,
          freshness_seconds: null,
        });
      }
      if (path === "/api/v1/admin/legacy-sync/jobs?limit=12") return Promise.resolve([]);
      if (path === "/api/v1/admin/legacy-sync/conflicts") return Promise.resolve([]);
      if (path === "/api/v1/admin/legacy-sync/student-import") {
        return Promise.resolve({ ...controlFixture, student_enabled: true });
      }
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("offers a student import toggle that posts the new state", async () => {
    renderPage();
    const toggle = await screen.findByRole("button", { name: "Enable student import" });

    fireEvent.click(toggle);

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith("/api/v1/admin/legacy-sync/student-import", {
        method: "POST",
        body: JSON.stringify({ enabled: true }),
      });
    });
  });
});

describe("Legacy sync health conflict actions", () => {
  const openConflict = {
    id: "conflict-1",
    entity_type: "course",
    external_id: "7306",
    conflict_type: "code_claimed",
    category: "mapping_conflict",
    message: "Legacy course 7306 already claimed by local course CS101",
    source_payload: '{"code":"CS101"}',
    local_payload: '{"code":"CS101","title":"Computing"}',
    status: "open",
    created_at: null,
  };

  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string) => {
      if (path === "/api/v1/admin/legacy-sync/health") {
        return Promise.resolve({
          status: "shadow",
          paused: false,
          shadow_mode: true,
          control: controlFixture,
          queue: { queued: 0, running: 0, completed: 0, dead: 0 },
          open_conflicts: 1,
          latest_run: null,
          last_successful_at: null,
          freshness_seconds: null,
        });
      }
      if (path === "/api/v1/admin/legacy-sync/jobs?limit=12") return Promise.resolve([]);
      if (path === "/api/v1/admin/legacy-sync/conflicts") return Promise.resolve([openConflict]);
      if (path === "/api/v1/admin/legacy-sync/conflicts/conflict-1/resolve" || path === "/api/v1/admin/legacy-sync/conflicts/conflict-1/ignore") {
        return Promise.resolve({ ...openConflict, status: path.endsWith("/resolve") ? "resolved" : "ignored" });
      }
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("renders payloads for open conflicts", async () => {
    renderPage();
    expect(await screen.findByText(/Legacy course 7306 already claimed/)).toBeInTheDocument();
    expect(screen.getByText("Source payload")).toBeInTheDocument();
    expect(screen.getByText("Local payload")).toBeInTheDocument();
    expect(screen.getByText(/"title": "Computing"/)).toBeInTheDocument();
  });

  it("posts to the resolve endpoint when Resolve is clicked", async () => {
    renderPage();
    const resolveButton = await screen.findByRole("button", { name: "Resolve" });

    fireEvent.click(resolveButton);

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith("/api/v1/admin/legacy-sync/conflicts/conflict-1/resolve", { method: "POST" });
    });
  });

  it("posts to the ignore endpoint when Ignore is clicked", async () => {
    renderPage();
    const ignoreButton = await screen.findByRole("button", { name: "Ignore" });

    fireEvent.click(ignoreButton);

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith("/api/v1/admin/legacy-sync/conflicts/conflict-1/ignore", { method: "POST" });
    });
  });
});

describe("Legacy sync live progress", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string) => {
      if (path === "/api/v1/admin/legacy-sync/health") {
        return Promise.resolve({
          status: "syncing",
          paused: false,
          shadow_mode: false,
          control: { ...controlFixture, shadow_mode: false },
          queue: { queued: 5, running: 1, completed: 12, dead: 0 },
          open_conflicts: 2,
          latest_run: {
            id: "run-1",
            mode: "full_sweep",
            status: "running",
            started_at: "2026-08-20T06:48:27Z",
            completed_at: null,
            pages_requested: 0,
            entities_parsed: 0,
            entities_changed: 0,
            entities_applied: 0,
            parse_failures: 0,
            reconciliation_mismatches: 0,
            source_latency_ms: null,
            last_error: null,
            progress: {
              phase: "reconciling_courses",
              current_entity: "2101",
              processed_entities: 1234,
              total_entities: 7325,
              changed_entities: 456,
              applied_entities: 789,
              failures: 2,
              updated_at: "2026-08-20T06:48:58Z",
            },
          },
          last_successful_at: null,
          freshness_seconds: null,
        });
      }
      if (path === "/api/v1/admin/legacy-sync/jobs?limit=12") return Promise.resolve([]);
      if (path === "/api/v1/admin/legacy-sync/conflicts") return Promise.resolve([]);
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("shows live progress counts and the remaining course queue", async () => {
    renderPage();

    expect(await screen.findByRole("heading", { name: "Live import result" })).toBeInTheDocument();
    expect(screen.getByText("1,234 / 7,325 processed")).toBeInTheDocument();
    expect(screen.getByText("Current item:")).toBeInTheDocument();
    expect(screen.getByText("2101")).toBeInTheDocument();
    expect(screen.getByText(/course refreshes are still queued/)).toBeInTheDocument();
  });
});
