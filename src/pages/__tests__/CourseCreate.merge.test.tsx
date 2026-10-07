import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import CourseCreate from "../CourseCreate";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function renderCourseCreate(entry = "/courses/create") {
  render(
    <MemoryRouter initialEntries={[entry]}>
      <ToastProvider>
        <Routes>
          <Route path="/courses/create" element={<CourseCreate />} />
          <Route path="/course-groups/:id" element={<div>Created merged course</div>} />
        </Routes>
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("CourseCreate merge mode", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/users?role=Teacher") return Promise.resolve([]);
      if (path === "/api/v1/subjects") return Promise.resolve([]);
      if (path === "/api/v1/crm/cycles") return Promise.resolve([]);
      if (path === "/api/v1/courses?limit=200&offset=0") {
        return Promise.resolve({
          items: [
            { id: "course-reading", code: "SAT-R", name: "Reading", subject_code: "SAT", subject_name: "Verbal", teacher_name: "AJ. NICE" },
            { id: "course-writing", code: "SAT-W", name: "Writing", subject_code: "SAT", subject_name: "Verbal", teacher_name: "AJ. RYU" },
          ],
        });
      }
      if (path === "/api/v1/courses/course-outside-page") {
        return Promise.resolve({ id: "course-outside-page", code: "SAT-X", name: "Reading", subject_code: "SAT", subject_name: "Verbal", teacher_name: "AJ. NEW" });
      }
      if (path === "/api/v1/course-groups" && init?.method === "POST") return Promise.resolve({ id: "group-1", name: "SAT Verbal", course_ids: ["course-reading", "course-writing"] });
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("creates a named merge from two searchable source courses", async () => {
    const user = userEvent.setup();
    renderCourseCreate();

    await user.click(screen.getByRole("tab", { name: "Merge existing courses" }));
    const name = await screen.findByRole("textbox", { name: /Merged course name/ });
    await user.type(name, "SAT Verbal");

    const selectors = screen.getAllByRole("combobox");
    await user.click(selectors[0]);
    await user.click(await screen.findByRole("option", { name: /SAT-R — Reading/ }));
    await user.click(selectors[1]);
    await user.click(await screen.findByRole("option", { name: /SAT-W — Writing/ }));

    expect(screen.getByLabelText("Merge preview")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Create merged course" }));

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith("/api/v1/course-groups", expect.objectContaining({ method: "POST" })));
    const post = mockApiJson.mock.calls.find(([path, init]) => path === "/api/v1/course-groups" && init?.method === "POST");
    if (!post) throw new Error("Expected merged course POST");
    expect(JSON.parse(post[1].body as string)).toEqual({ name: "SAT Verbal", course_ids: ["course-reading", "course-writing"] });
    expect(await screen.findByText("Created merged course")).toBeInTheDocument();
    expect(screen.queryByText("New Course")).not.toBeInTheDocument();
  });

  it("links a split course as a continuation that keeps the chosen course's settings", async () => {
    const user = userEvent.setup();
    renderCourseCreate();

    await user.click(screen.getByRole("tab", { name: "Merge existing courses" }));
    await user.type(await screen.findByRole("textbox", { name: /Merged course name/ }), "SAT Math Rank 1 C3");
    await user.click(screen.getByRole("checkbox", { name: /Same course \/ continuation/ }));

    const selectors = screen.getAllByRole("combobox");
    await user.click(selectors[0]);
    await user.click(await screen.findByRole("option", { name: /SAT-R — Reading · AJ\. NICE/ }));
    await user.click(selectors[1]);
    await user.click(await screen.findByRole("option", { name: /SAT-W — Writing · AJ\. RYU/ }));

    await user.click(screen.getByRole("button", { name: "Create merged course" }));
    expect(await screen.findByText("Choose which course's settings to keep")).toBeInTheDocument();
    expect(mockApiJson).not.toHaveBeenCalledWith("/api/v1/course-groups", expect.anything());

    await user.click(screen.getByRole("radio", { name: /SAT-R — Reading/ }));
    expect(screen.getByText(/Both IDs will use SAT-R's level/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Create merged course" }));

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith("/api/v1/course-groups", expect.objectContaining({ method: "POST" })));
    const post = mockApiJson.mock.calls.find(([path, init]) => path === "/api/v1/course-groups" && init?.method === "POST");
    if (!post) throw new Error("Expected continuation POST");
    expect(JSON.parse(post[1].body as string)).toEqual({
      name: "SAT Math Rank 1 C3",
      course_ids: ["course-reading", "course-writing"],
      kind: "continuation",
      rule_source_course_id: "course-reading",
    });
  });

  it("prefills a continuation from a course-link suggestion, including courses outside the first page", async () => {
    const user = userEvent.setup();
    const params = new URLSearchParams({
      mode: "merge",
      continuation: "1",
      courses: "course-reading,course-outside-page",
      source: "course-reading",
      name: "Same course: SAT-R + SAT-X",
    });
    renderCourseCreate(`/courses/create?${params.toString()}`);

    expect(await screen.findByRole("textbox", { name: /Merged course name/ })).toHaveValue("Same course: SAT-R + SAT-X");
    expect(screen.getByRole("checkbox", { name: /Same course \/ continuation/ })).toBeChecked();
    expect(await screen.findByLabelText("Merge preview")).toHaveTextContent("SAT-X");
    expect(screen.getByRole("radio", { name: /SAT-R — Reading/ })).toBeChecked();

    await user.click(screen.getByRole("button", { name: "Create merged course" }));
    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith("/api/v1/course-groups", expect.objectContaining({ method: "POST" })));
    const post = mockApiJson.mock.calls.find(([path, init]) => path === "/api/v1/course-groups" && init?.method === "POST");
    if (!post) throw new Error("Expected continuation POST");
    expect(JSON.parse(post[1].body as string)).toEqual({
      name: "Same course: SAT-R + SAT-X",
      course_ids: ["course-reading", "course-outside-page"],
      kind: "continuation",
      rule_source_course_id: "course-reading",
    });
  });
});
