import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Schedule from "../Schedule";
import { ToastProvider } from "../../hooks/useToast";
import { ApiRequestError } from "@/api/client";

const mockApiJson = vi.hoisted(() => vi.fn());
const mockSessionsQuery = vi.hoisted(() => ({
  data: [] as never[],
  isPending: false,
  error: null,
  refetch: vi.fn(async () => ({ data: [] as never[] })),
}));
const mockLookups = vi.hoisted(() => {
  const courses = [{ id: "course-1", code: "MATH-101", name: "Math 101" }];
  const rooms = [{ id: "room-1", name: "Room 1", capacity: 20 }];
  const teachers = [{ id: "teacher-1", username: "teacher.one", role: "Teacher" as const }];

  return {
    courses,
    rooms,
    teachers,
    courseById: new Map(courses.map((item) => [item.id, item])),
    roomById: new Map(rooms.map((item) => [item.id, item])),
    teacherById: new Map(teachers.map((item) => [item.id, item])),
    courseOptions: courses.map((item) => ({ value: item.id, label: `${item.code} — ${item.name}`, keywords: `${item.code} ${item.name}` })),
    teacherOptions: teachers.map((item) => ({ value: item.id, label: item.username, keywords: item.username })),
    loading: false,
    reload: vi.fn(),
  };
});
let preflightAttempts = 0;

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("@/query/useOperationalQuery", () => ({
  useOperationalQuery: () => mockSessionsQuery,
}));

vi.mock("@/hooks/useInstituteMeta", () => ({
  default: () => ({ serverNow: "2026-05-29T10:00:00Z", instituteTZ: "Asia/Bangkok" }),
}));

const emptyResponse: never[] = [];

vi.mock("@/features/scheduling/hooks/useLookups", () => ({
  default: () => mockLookups,
}));

beforeEach(() => {
  preflightAttempts = 0;
  mockApiJson.mockReset();
  mockApiJson.mockImplementation(async (path: unknown, init?: RequestInit) => {
    if (path === "/api/v1/scheduling/preflight" && init?.method === "POST") {
      preflightAttempts += 1;
      if (preflightAttempts === 1) {
        throw new ApiRequestError("Schedule service unavailable", { status: 503, code: "internal" });
      }
      return { status: "available" };
    }
    return emptyResponse;
  });
});

describe("Schedule accessibility user stories", () => {
  it("announces a preflight error, exposes a disabled-save reason, and supports keyboard retry", async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <ToastProvider>
          <Schedule />
        </ToastProvider>
      </MemoryRouter>,
    );

    await user.click(screen.getByRole("button", { name: "Create Session" }));
    const dialog = screen.getByRole("dialog", { name: "Create Session" });
    fireEvent.change(within(dialog).getByLabelText(/Start \(local time\)/i), {
      target: { value: "2026-05-20T10:00" },
    });
    fireEvent.change(within(dialog).getByLabelText(/End \(local time\)/i), {
      target: { value: "2026-05-20T11:00" },
    });

    const alert = await screen.findByTestId("preflight-error");
    expect(alert).toHaveAttribute("role", "alert");
    expect(alert).toHaveTextContent("Could not check the schedule");
    const save = screen.getByRole("button", { name: "Unavailable — check schedule" });
    expect(save).toBeDisabled();

    const retry = within(alert).getByRole("button", { name: "Try checking availability again" });
    retry.focus();
    expect(retry).toHaveFocus();
    await user.keyboard("{Enter}");

    await waitFor(() => expect(screen.getByRole("button", { name: "Create" })).not.toBeDisabled());
    expect(preflightAttempts).toBe(2);
  });
});
