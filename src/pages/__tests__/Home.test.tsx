import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import Home from "../Home";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("Home empty state", () => {
  it("shows EmptyState when no sessions exist for the selected date", async () => {
    mockApiJson.mockResolvedValue([]);
    render(
      <MemoryRouter>
        <ToastProvider>
          <Home />
        </ToastProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.queryByText("Loading…")).not.toBeInTheDocument();
    });
    expect(screen.getByText(/No sessions found for/)).toBeInTheDocument();
  });
});

describe("Home sessions without a room", () => {
  it("groups roomless sessions under Unassigned instead of crashing", async () => {
    mockApiJson.mockImplementation((url: string) => {
      if (url.startsWith("/api/v1/sessions")) {
        return Promise.resolve([
          {
            id: "s1",
            course_id: "c1",
            room_id: null,
            teacher_id: "t1",
            start_at: "2026-08-16T09:00:00+07:00",
            end_at: "2026-08-16T10:00:00+07:00",
          },
        ]);
      }
      return Promise.resolve([]);
    });
    render(
      <MemoryRouter>
        <ToastProvider>
          <Home />
        </ToastProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText(/Unassigned on/)).toBeInTheDocument();
    });
  });
});

describe("Home assign rooms entry", () => {
  it("offers an Assign Rooms button linking to the assign page", async () => {
    mockApiJson.mockResolvedValue([]);
    render(
      <MemoryRouter>
        <ToastProvider>
          <Home />
        </ToastProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.queryByText("Loading…")).not.toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: /assign rooms/i })).toBeInTheDocument();
  });
});

describe("Home session course display", () => {
  it("shows only the subject name, without the course code or session column", async () => {
    mockApiJson.mockImplementation((url: string) => {
      if (url.startsWith("/api/v1/sessions")) {
        return Promise.resolve([
          {
            id: "s1",
            course_id: "c1",
            room_id: "r1",
            teacher_id: "t1",
            start_at: "2026-08-16T09:00:00+07:00",
            end_at: "2026-08-16T10:00:00+07:00",
          },
        ]);
      }
      if (url.startsWith("/api/v1/courses")) {
        return Promise.resolve([
          { id: "c1", code: "COURSE-abc", name: "Course abc", subject_name: "Mathematics" },
        ]);
      }
      if (url.startsWith("/api/v1/rooms")) {
        return Promise.resolve([{ id: "r1", name: "Room 1", capacity: 20 }]);
      }
      return Promise.resolve([]);
    });
    render(
      <MemoryRouter>
        <ToastProvider>
          <Home />
        </ToastProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("Mathematics")).toBeInTheDocument();
    });
    expect(screen.queryByText("COURSE-abc")).not.toBeInTheDocument();
    expect(screen.queryByText(/Course abc/)).not.toBeInTheDocument();
    expect(screen.queryByText("Session")).not.toBeInTheDocument();
  });

  it("falls back to the course name when the course has no subject", async () => {
    mockApiJson.mockImplementation((url: string) => {
      if (url.startsWith("/api/v1/sessions")) {
        return Promise.resolve([
          {
            id: "s1",
            course_id: "c1",
            room_id: null,
            teacher_id: "t1",
            start_at: "2026-08-16T09:00:00+07:00",
            end_at: "2026-08-16T10:00:00+07:00",
          },
        ]);
      }
      if (url.startsWith("/api/v1/courses")) {
        return Promise.resolve([
          { id: "c1", code: "COURSE-abc", name: "Course abc", subject_name: "" },
        ]);
      }
      return Promise.resolve([]);
    });
    render(
      <MemoryRouter>
        <ToastProvider>
          <Home />
        </ToastProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("Course abc")).toBeInTheDocument();
    });
    expect(screen.queryByText("COURSE-abc")).not.toBeInTheDocument();
  });
});
