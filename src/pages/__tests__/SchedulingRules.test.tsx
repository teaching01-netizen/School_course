import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import SchedulingRules from "../SchedulingRules";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

const rules = [
  { id: "room_overlap", label: "Room overlap", description: "Room", controlled: true },
  { id: "teacher_overlap", label: "Teacher overlap", description: "Teacher", controlled: true },
];

function fixture(overrides: Partial<{ system_enforced: boolean; legacy_sync_enforced: boolean }> = {}) {
  return {
    system_enforced: true,
    legacy_sync_enforced: true,
    updated_at: "2026-08-23T03:00:00Z",
    rules,
    history: [],
    history_retention: "3 days",
    ...overrides,
  };
}

function renderPage() {
  render(
    <MemoryRouter>
      <ToastProvider>
        <SchedulingRules />
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("SchedulingRules", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("requires confirmation before turning off system enforcement", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/admin/scheduling-rules" && !init) return Promise.resolve(fixture());
      if (path === "/api/v1/admin/scheduling-rules" && init?.method === "PUT") {
        return Promise.resolve(fixture({ system_enforced: false }));
      }
      throw new Error(`Unexpected API call: ${path}`);
    });

    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Turn off enforcement" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("Preflight and conflict checks will still run");
    expect(mockApiJson).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Confirm turn off" }));
    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith("/api/v1/admin/scheduling-rules", {
        method: "PUT",
        body: JSON.stringify({ system_enforced: false, legacy_sync_enforced: true }),
      });
    });
  });

  it("turns legacy enforcement on with the system control", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/admin/scheduling-rules" && !init) return Promise.resolve(fixture({ system_enforced: false, legacy_sync_enforced: false }));
      if (path === "/api/v1/admin/scheduling-rules" && init?.method === "PUT") {
        return Promise.resolve(fixture());
      }
      throw new Error(`Unexpected API call: ${path}`);
    });

    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Turn on enforcement" }));
    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith("/api/v1/admin/scheduling-rules", {
        method: "PUT",
        body: JSON.stringify({ system_enforced: true, legacy_sync_enforced: true }),
      });
    });
  });

  it("requires confirmation before turning off legacy enforcement", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/admin/scheduling-rules" && !init) return Promise.resolve(fixture());
      if (path === "/api/v1/admin/scheduling-rules" && init?.method === "PUT") {
        return Promise.resolve(fixture({ legacy_sync_enforced: false }));
      }
      throw new Error(`Unexpected API call: ${path}`);
    });

    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Allow legacy conflicts" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("Turn off legacy sync enforcement?");
    expect(mockApiJson).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Confirm turn off" }));
    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith("/api/v1/admin/scheduling-rules", {
        method: "PUT",
        body: JSON.stringify({ system_enforced: true, legacy_sync_enforced: false }),
      });
    });
  });
});
