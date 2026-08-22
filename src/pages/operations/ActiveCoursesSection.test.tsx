import { beforeEach, describe, expect, it, vi } from "vitest";
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
      { course_id: "c-hidden", course_code: "MATH-102", course_name: "Math", cycle_id: "cy1", cycle_label: "Cycle 1", is_active: false, absence_form_visible: false },
    ],
    ...overrides,
  };
}

function renderSection() {
  return render(
    <ToastProvider>
      <ActiveCoursesSection />
    </ToastProvider>,
  );
}

describe("ActiveCoursesSection — absence form control center", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/admin/active-courses?") && init?.method !== "PUT") {
        return Promise.resolve({
          subjects: [subject()],
          total_subjects: 1,
          total_courses: 2,
          limit: 50,
          offset: 0,
        });
      }
      if (path === "/api/v1/admin/active-courses/visibility" && init?.method === "PUT") {
        return Promise.resolve({ status: "ok" });
      }
      return Promise.reject(new Error(`Unexpected API call: ${path}`));
    });
  });

  it("renders the explainer so the three concepts are understandable", async () => {
    renderSection();
    expect(await screen.findByText("How the student absence form uses these settings")).toBeInTheDocument();
    expect(screen.getByText(/auto-assigns when a student reports an absence/i)).toBeInTheDocument();
    expect(screen.getByText("sit-in")).toBeInTheDocument();
    expect(screen.getByText(/Hidden classes can.t be picked by students/i)).toBeInTheDocument();
  });

  it("shows switch state per course with the active course marked Active", async () => {
    renderSection();
    const shown = await screen.findByRole("switch", { name: "Show MATH-101 in the student absence form" });
    expect(shown).toHaveAttribute("aria-checked", "true");
    const hidden = screen.getByRole("switch", { name: "Show MATH-102 in the student absence form" });
    expect(hidden).toHaveAttribute("aria-checked", "false");
    expect(screen.getAllByText("In student form").length).toBeGreaterThan(0);
    expect(screen.getByText("Hidden from students")).toBeInTheDocument();
    expect(screen.getAllByText("Active").length).toBeGreaterThan(0);
  });

  it("warns when the active course itself is hidden from the form", async () => {
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

  it("toggles visibility through the dedicated endpoint and updates in place", async () => {
    renderSection();
    const hidden = await screen.findByRole("switch", { name: "Show MATH-102 in the student absence form" });
    await userEvent.setup().click(hidden);

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/active-courses/visibility",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ course_id: "c-hidden", absence_form_visible: true }),
        }),
      );
    });
    await waitFor(() =>
      expect(screen.getByRole("switch", { name: "Show MATH-102 in the student absence form" })).toHaveAttribute(
        "aria-checked",
        "true",
      ),
    );
  });
});
