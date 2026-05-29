import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import Schedule from "../Schedule";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("@/hooks/useInstituteMeta", () => ({
  default: () => ({ serverNow: "2026-05-29T10:00:00Z", instituteTZ: "Asia/Bangkok" }),
}));

vi.mock("@/hooks/useLookups", () => ({
  default: () => ({
    courses: [], rooms: [], teachers: [],
    courseById: new Map(), roomById: new Map(), teacherById: new Map(),
    courseOptions: [], teacherOptions: [],
  }),
}));

describe("Schedule empty state", () => {
  it("shows enhanced empty state message when no sessions exist", async () => {
    mockApiJson.mockResolvedValueOnce([]);
    render(
      <MemoryRouter>
        <ToastProvider>
          <Schedule />
        </ToastProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByText(/No sessions for this date range/)).toBeInTheDocument();
    expect(screen.getByText(/Use the toolbar above to create a session or series/)).toBeInTheDocument();
  });
});
