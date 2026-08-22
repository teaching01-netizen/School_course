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
      { course_id: "c-hidden", course_code: "MATH-102", course_name: "Math", cycle_id: "cy1", cycle_label: "Cycle 1", is_active: false, absence_form_visible: false },
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

describe("ActiveCoursesSection — absence form control center", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path.startsWith("/api/v1/admin/active-courses?") && init?.method !== "PUT") {
        return Promise.resolve(defaultResponse());
      }
      if (path === "/api/v1/admin/active-courses/visibility" && init?.method === "PUT") {
        return Promise.resolve({ status: "ok" });
      }
      if (path === "/api/v1/admin/active-courses" && init?.method === "PUT") {
        return Promise.resolve({ status: "ok" });
      }
      if (path === "/api/v1/admin/active-courses/visibility/bulk" && init?.method === "PUT") {
        return Promise.resolve({ updated: 1 });
      }
      return Promise.reject(new Error(`Unexpected API call: ${path}`));
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
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

  it("selects all classes on the page, bulk-hides them (skipping no-ops), and confirms before hiding an active course", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    renderSection();
    await screen.findByText("MATH — Mathematics");

    await userEvent.setup().click(screen.getByRole("checkbox", { name: "Select all classes on this page" }));
    expect(screen.getByRole("checkbox", { name: "Select MATH-101" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Select MATH-102" })).toBeChecked();
    expect(screen.getByText("2 classes selected")).toBeInTheDocument();
    // Hiding would trap the subject: the active course is in the selection.
    expect(screen.getByText(/1 active course in this selection/i)).toBeInTheDocument();

    await userEvent.setup().click(screen.getByRole("button", { name: "Hide from form" }));
    expect(confirmSpy).toHaveBeenCalledWith(expect.stringContaining("active courses"));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/active-courses/visibility/bulk",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ course_ids: ["c-active"], absence_form_visible: false }),
        }),
      );
    });
    await waitFor(() => {
      expect(screen.getByRole("switch", { name: "Show MATH-101 in the student absence form" })).toHaveAttribute(
        "aria-checked",
        "false",
      );
    });
    // Selection clears after a successful bulk apply.
    await waitFor(() => {
      expect(screen.getByRole("checkbox", { name: "Select MATH-101" })).not.toBeChecked();
    });
    expect(screen.queryByText("2 classes selected")).not.toBeInTheDocument();
  });

  it("bulk-shows a single selected hidden class through the bulk endpoint", async () => {
    renderSection();
    await screen.findByText("MATH — Mathematics");

    await userEvent.setup().click(screen.getByRole("checkbox", { name: "Select MATH-102" }));
    expect(screen.getByText("1 classes selected")).toBeInTheDocument();

    await userEvent.setup().click(screen.getByRole("button", { name: "Show in form" }));
    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/active-courses/visibility/bulk",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ course_ids: ["c-hidden"], absence_form_visible: true }),
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

  it("saves a new active course through the radio draft + Save flow", async () => {
    renderSection();
    await screen.findByText("MATH — Mathematics");

    const user = userEvent.setup();
    expect(screen.getByRole("radio", { name: /MATH-101/ })).toBeChecked();

    await user.click(screen.getByRole("radio", { name: /MATH-102/ }));
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/active-courses",
        expect.objectContaining({
          method: "PUT",
          body: expect.stringContaining("c-hidden"),
        }),
      );
    });
    await waitFor(() => {
      expect(screen.getByRole("radio", { name: /MATH-102/ })).toBeChecked();
      expect(screen.getByRole("radio", { name: /MATH-101/ })).not.toBeChecked();
    });
  });

  it("reverts the draft and toasts when saving the active course fails", async () => {
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
    await user.click(screen.getByRole("radio", { name: /MATH-102/ }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Failed to update MATH")).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
    });
    expect(screen.getByRole("radio", { name: /MATH-101/ })).toBeChecked();
    expect(screen.getByRole("radio", { name: /MATH-102/ })).not.toBeChecked();
  });

  it("shows a disabled radio and create link for subjects with no courses", async () => {
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
    expect(screen.getByRole("radio", { name: "No courses available" })).toBeDisabled();
    expect(screen.getByText("No courses — create one first")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Create Course" })).toHaveAttribute("href", "/courses/create");
  });
});
