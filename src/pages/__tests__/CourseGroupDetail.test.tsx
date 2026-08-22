import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import CourseGroupDetail from "../CourseGroupDetail";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function renderGroupDetail() {
  render(
    <MemoryRouter initialEntries={["/course-groups/group-1"]}>
      <ToastProvider>
        <Routes>
          <Route path="/course-groups/:id" element={<CourseGroupDetail />} />
        </Routes>
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("CourseGroupDetail", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/api/v1/course-groups/group-1" && init?.method === "PATCH") {
        const body = JSON.parse(String(init.body ?? "{}")) as { name?: string };
        return Promise.resolve({ id: "group-1", name: body.name ?? "" });
      }
      if (path === "/api/v1/course-groups/group-1" && init?.method === "DELETE") return Promise.resolve();
      if (path === "/api/v1/course-groups/group-1") {
        return Promise.resolve({
          id: "group-1",
          name: "SAT Verbal Reading + Writing",
          members: [
            { id: "course-reading", code: "SAT-R", name: "Reading", subject_code: "SAT", subject_name: "Verbal", legacy_course_id: "legacy-reading", teachers: [{ id: "teacher-1", username: "nice", full_name: "AJ. NICE", is_primary: true }] },
            { id: "course-writing", code: "SAT-W", name: "Writing", subject_code: "SAT", subject_name: "Verbal", legacy_course_id: null, teachers: [{ id: "teacher-2", username: "ryu", full_name: "AJ. RYU", is_primary: true }] },
          ],
          teachers: [
            { id: "teacher-1", username: "nice", full_name: "AJ. NICE", course_ids: ["course-reading"], course_codes: ["SAT-R"] },
            { id: "teacher-2", username: "ryu", full_name: "AJ. RYU", course_ids: ["course-writing"], course_codes: ["SAT-W"] },
          ],
        });
      }
      if (path === "/api/v1/course-groups/group-1/sessions") {
        return Promise.resolve([
          { id: "session-reading", course_id: "course-reading", course_code: "SAT-R", course_name: "Reading", room_id: "room-1", teacher_id: "teacher-1", teacher_name: "AJ. NICE", start_at: "2026-08-29T06:00:00Z", end_at: "2026-08-29T07:40:00Z", version: 1 },
          { id: "session-writing", course_id: "course-writing", course_code: "SAT-W", course_name: "Writing", room_id: null, teacher_id: "teacher-2", teacher_name: "AJ. RYU", start_at: "2026-08-29T07:40:00Z", end_at: "2026-08-29T09:20:00Z", version: 1 },
        ]);
      }
      if (path === "/api/v1/rooms") return Promise.resolve([{ id: "room-1", name: "Room 101", capacity: 20 }]);
      if (path === "/api/v1/meta/time") return Promise.resolve({ institute_tz: "Asia/Bangkok", server_now: "2026-08-21T12:00:00Z" });
      throw new Error(`Unexpected API call: ${path}`);
    });
  });

  it("shows source courses, merged teachers, and sessions without changing source links", async () => {
    renderGroupDetail();

    expect(await screen.findByRole("heading", { name: "SAT Verbal Reading + Writing" })).toBeInTheDocument();
    expect(screen.getByText("Legacy sync")).toBeInTheDocument();
    expect(screen.getAllByText("AJ. NICE").length).toBeGreaterThan(1);
    expect(screen.getAllByText("AJ. RYU").length).toBeGreaterThan(1);
    expect(screen.getAllByText("SAT-R").length).toBeGreaterThan(1);
    expect(screen.getAllByText("SAT-W").length).toBeGreaterThan(1);
    expect(screen.getByText("Room 101")).toBeInTheDocument();
    expect(screen.getByText("Not set")).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: "Open source course" })[0]).toHaveAttribute("href", "/courses/course-reading");
  });

  it("confirms and submits an unmerge without touching source links", async () => {
    const user = userEvent.setup();
    renderGroupDetail();

    expect(await screen.findByRole("heading", { name: "SAT Verbal Reading + Writing" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Unmerge course" }));

    expect(screen.getByRole("dialog", { name: "Unmerge course?" })).toBeInTheDocument();
    await user.click(within(screen.getByRole("dialog", { name: "Unmerge course?" })).getByRole("button", { name: "Unmerge course" }));

    expect(mockApiJson).toHaveBeenCalledWith("/api/v1/course-groups/group-1", { method: "DELETE" });
  });

  it("edits the merged view name without changing source courses", async () => {
    const user = userEvent.setup();
    renderGroupDetail();

    expect(await screen.findByRole("heading", { name: "SAT Verbal Reading + Writing" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Edit name" }));

    const dialog = screen.getByRole("dialog", { name: "Edit merged course" });
    const input = within(dialog).getByRole("textbox", { name: "Merged course name" });
    await user.clear(input);
    await user.type(input, "SAT Verbal Core");
    await user.click(within(dialog).getByRole("button", { name: "Save changes" }));

    expect(mockApiJson).toHaveBeenCalledWith("/api/v1/course-groups/group-1", {
      method: "PATCH",
      body: JSON.stringify({ name: "SAT Verbal Core" }),
    });
    expect(await screen.findByRole("heading", { name: "SAT Verbal Core" })).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: "Open source course" })[0]).toHaveAttribute("href", "/courses/course-reading");
    expect(screen.getAllByText("SAT-R").length).toBeGreaterThan(1);
    expect(screen.getAllByText("SAT-W").length).toBeGreaterThan(1);
  });

  it("keeps the edit dialog open when the name is blank", async () => {
    const user = userEvent.setup();
    renderGroupDetail();

    await screen.findByRole("heading", { name: "SAT Verbal Reading + Writing" });
    await user.click(screen.getByRole("button", { name: "Edit name" }));
    const dialog = screen.getByRole("dialog", { name: "Edit merged course" });
    await user.clear(within(dialog).getByRole("textbox", { name: "Merged course name" }));
    await user.click(within(dialog).getByRole("button", { name: "Save changes" }));

    expect(screen.getByRole("dialog", { name: "Edit merged course" })).toBeInTheDocument();
    expect(mockApiJson).not.toHaveBeenCalledWith("/api/v1/course-groups/group-1", expect.objectContaining({ method: "PATCH" }));
  });
});
