import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import Summary from "../Summary";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("Summary session course display", () => {
  it("shows the subject name instead of the course name", async () => {
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
          <Summary />
        </ToastProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("Mathematics")).toBeInTheDocument();
    });
    expect(screen.queryByText("Course abc")).not.toBeInTheDocument();
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
          <Summary />
        </ToastProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("Course abc")).toBeInTheDocument();
    });
  });
});