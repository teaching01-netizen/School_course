import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider } from "../../hooks/useToast";
import { ActiveCoursesSection } from "./ActiveCoursesSection";
import type { ActiveCourseSubject } from "../../types";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, apiJson: mockApiJson };
});

function subject(overrides: Partial<ActiveCourseSubject> = {}): ActiveCourseSubject {
  return {
    subject_id: "subj-1",
    subject_code: "MATH",
    subject_name: "Mathematics",
    courses: [
      { course_id: "c-active", course_code: "MATH-101", course_name: "Math", cycle_id: "cy1", cycle_label: "Cycle 1", is_active: true, absence_form_visible: true },
      { course_id: "c-off", course_code: "MATH-102", course_name: "Math", cycle_id: "cy1", cycle_label: "Cycle 1", is_active: false, absence_form_visible: false },
    ],
    ...overrides,
  };
}

function defaultResponse() {
  return {
    subjects: [subject()],
    total_subjects: 1,
    total_courses: 2,
    limit: 50,
    offset: 0,
    stats: { total_subjects: 1, missing_active: 0, hidden_active: 0 },
  };
}

function renderSection() {
  return render(
    <ToastProvider>
      <ActiveCoursesSection />
    </ToastProvider>,
  );
}

function getCalls() {
  return mockApiJson.mock.calls.filter(([path]) => typeof path === "string" && path.includes("/admin/active-courses?"));
}

describe("ActiveCoursesSection — one Active switch per class", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/admin/active-courses?") && init?.method !== "PUT") {
        return Promise.resolve(defaultResponse());
      }
      if (path === "/api/v1/admin/active-courses/set-active" && init?.method === "PUT") {
        return Promise.resolve({ status: "ok" });
      }
      if (path === "/api/v1/admin/active-courses/set-active/bulk" && init?.method === "PUT") {
        return Promise.resolve({ updated: 1 });
      }
      return Promise.reject(new Error(`Unexpected API call: ${path}`));
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the explainer with the multi-active model", async () => {
    renderSection();
    expect(await screen.findByText("How the student absence form uses these settings")).toBeInTheDocument();
    expect(screen.getByText(/A subject can run several active classes at once/i)).toBeInTheDocument();
    expect(screen.getAllByText(/sit-ins?/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/Hidden classes can.t be picked by students/i)).toBeInTheDocument();
  });

  it("shows the active class on and the others off", async () => {
    renderSection();
    const active = await screen.findByRole("switch", { name: "MATH-101 active" });
    expect(active).toHaveAttribute("aria-checked", "true");
    const off = screen.getByRole("switch", { name: "MATH-102 active" });
    expect(off).toHaveAttribute("aria-checked", "false");
    expect(screen.getAllByText("Active").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Off").length).toBeGreaterThan(0);
  });

  it("flags legacy hidden active classes so they can be healed", async () => {
    mockApiJson.mockImplementation(() =>
      Promise.resolve({
        subjects: [
          subject({
            courses: [
              { course_id: "c-active", course_code: "MATH-101", course_name: "Math", cycle_id: "cy1", cycle_label: "Cycle 1", is_active: true, absence_form_visible: false },
            ],
          }),
        ],
        total_subjects: 1,
      }),
    );
    renderSection();
    expect(await screen.findByText("Active — hidden from form")).toBeInTheDocument();
    expect(screen.getByText(/active course that is hidden/i)).toBeInTheDocument();
  });

  it("activates a class with one click and leaves siblings untouched", async () => {
    renderSection();
    await screen.findByText("MATH — Mathematics");

    const user = userEvent.setup();
    await user.click(screen.getByRole("switch", { name: "MATH-102 active" }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/active-courses/set-active",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ course_id: "c-off", active: true }),
        }),
      );
    });
    await waitFor(() => {
      expect(screen.getByRole("switch", { name: "MATH-102 active" })).toHaveAttribute("aria-checked", "true");
    });
    // Multi-active: the previously active class keeps its state.
    expect(screen.getByRole("switch", { name: "MATH-101 active" })).toHaveAttribute("aria-checked", "true");
  });

  it("shows the active count on the subject header when several classes are active", async () => {
    mockApiJson.mockImplementation(() =>
      Promise.resolve({
        subjects: [
          subject({
            courses: [
              { course_id: "c-active", course_code: "MATH-101", course_name: "Math", cycle_id: "cy1", cycle_label: "Cycle 1", is_active: true, absence_form_visible: true },
              { course_id: "c-active2", course_code: "MATH-102", course_name: "Math", cycle_id: "cy1", cycle_label: "Cycle 1", is_active: true, absence_form_visible: true },
            ],
          }),
        ],
        total_subjects: 1,
      }),
    );
    renderSection();
    expect(await screen.findByText("Active (2)")).toBeInTheDocument();
    // Both switches render on independently.
    expect(screen.getByRole("switch", { name: "MATH-101 active" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("switch", { name: "MATH-102 active" })).toHaveAttribute("aria-checked", "true");
  });

  it("leaves the switch unchanged and toasts when activation fails", async () => {
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/admin/active-courses?") && init?.method !== "PUT") {
        return Promise.resolve(defaultResponse());
      }
      if (init?.method === "PUT") {
        return Promise.reject(new Error("Network error"));
      }
      return Promise.reject(new Error(`Unexpected API call: ${path}`));
    });
    renderSection();
    await screen.findByText("MATH — Mathematics");

    const user = userEvent.setup();
    await user.click(screen.getByRole("switch", { name: "MATH-102 active" }));

    expect(await screen.findByText("Network error")).toBeInTheDocument();
    expect(screen.getByRole("switch", { name: "MATH-102 active" })).toHaveAttribute("aria-checked", "false");
    expect(screen.getByRole("switch", { name: "MATH-101 active" })).toHaveAttribute("aria-checked", "true");
  });

  it("filters server-side: status chips refetch with the status parameter", async () => {
    renderSection();
    await screen.findByText("MATH — Mathematics");
    expect(getCalls()).toHaveLength(1);

    await userEvent.setup().click(screen.getByRole("button", { name: /no active/i }));
    await waitFor(() => {
      expect(getCalls().at(-1)?.[0]).toContain("status=missing_active");
    });
    expect(screen.getByRole("button", { name: /no active/i })).toHaveAttribute("aria-pressed", "true");
  });

  it("searches server-side after the debounce settles", async () => {
    renderSection();
    await screen.findByText("MATH — Mathematics");

    await userEvent.setup().type(
      screen.getByPlaceholderText("Search subjects across all pages..."),
      "math",
    );
    await waitFor(() => {
      expect(getCalls().at(-1)?.[0]).toContain("search=math");
    }, { timeout: 2000 });
  });

  it("bulk turns off the selection through the bulk endpoint and refetches", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    renderSection();
    await screen.findByText("MATH — Mathematics");

    await userEvent.setup().click(screen.getByRole("checkbox", { name: "Select all classes on this page" }));
    expect(screen.getByRole("checkbox", { name: "Select MATH-101" })).toBeChecked();
    expect(screen.getByText("2 classes selected")).toBeInTheDocument();
    // The active class is in the selection: turning off must warn first.
    expect(screen.getByText(/1 active class in this selection/i)).toBeInTheDocument();

    await userEvent.setup().click(screen.getByRole("button", { name: "Turn off" }));
    expect(confirmSpy).toHaveBeenCalledWith(expect.stringContaining("active"));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/active-courses/set-active/bulk",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ course_ids: ["c-active", "c-off"], active: false }),
        }),
      );
    });
    // Selection clears and the page refetches server truth.
    await waitFor(() => {
      expect(screen.getByRole("checkbox", { name: "Select MATH-101" })).not.toBeChecked();
    });
    expect(screen.queryByText("2 classes selected")).not.toBeInTheDocument();
  });

  it("bulk activates a single selected class", async () => {
    renderSection();
    await screen.findByText("MATH — Mathematics");

    await userEvent.setup().click(screen.getByRole("checkbox", { name: "Select MATH-102" }));
    expect(screen.getByText("1 classes selected")).toBeInTheDocument();

    await userEvent.setup().click(screen.getByRole("button", { name: "Activate" }));
    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/active-courses/set-active/bulk",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ course_ids: ["c-off"], active: true }),
        }),
      );
    });
    await waitFor(() => {
      expect(screen.queryByText("1 classes selected")).not.toBeInTheDocument();
    });
  });

  it("shows a create link for subjects with no courses", async () => {
    mockApiJson.mockImplementation(() =>
      Promise.resolve({
        subjects: [
          {
            subject_id: "subj-empty",
            subject_code: "CHEM",
            subject_name: "Chemistry",
            courses: [],
          },
        ],
        total_subjects: 1,
      }),
    );
    renderSection();

    expect(await screen.findByText("CHEM — Chemistry")).toBeInTheDocument();
    expect(screen.getByText("No courses — create one first")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Create Course" })).toHaveAttribute("href", "/courses/create");
  });
});
