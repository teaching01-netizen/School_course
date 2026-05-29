import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import AbsenceDetail from "../AbsenceDetail";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

const DETAIL = {
  id: "abs-1",
  wcode: "W250389",
  student_name: "John Smith",
  student_email: null,
  student_phone: null,
  course_id: "course-1",
  course_code: "MATH-201",
  course_name: "Algebra II",
  subject_id: "subj-1",
  subject_code: "MATH",
  subject_name: "Mathematics",
  date_from: "2026-06-02",
  date_to: "2026-06-06",
  reason_category: "medical",
  reason: "Appointment",
  sit_in_method: "physical",
  sit_in_course_id: "sit-1",
  sit_in_course_code: "MATH-301",
  sit_in_course_name: "Calculus III",
  status: "pending",
  admin_notes: "",
  version: 1,
  created_at: "2026-05-27T09:00:00Z",
  updated_at: "2026-05-27T09:00:00Z",
  sit_ins: [{ id: "si-1", session_id: "sess-1", course_id: "sit-1", course_code: "MATH-301", course_name: "Calculus III", room_name: "Room 201", start_at: "2026-06-02T04:00:00Z", end_at: "2026-06-02T05:30:00Z" }],
  timeline: [{ id: "tl-1", action: "submitted", actor_role: "student", details: {}, created_at: "2026-05-27T09:00:00Z" }],
};

function renderDetail() {
  render(
    <MemoryRouter initialEntries={["/absences/abs-1"]}>
      <ToastProvider>
        <Routes>
          <Route path="/absences/:id" element={<AbsenceDetail />} />
        </Routes>
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("Absence detail", () => {
  beforeEach(() => mockApiJson.mockReset());

  it("shows the action context and marks a pending record reviewed", async () => {
    mockApiJson
      .mockResolvedValueOnce(DETAIL)
      .mockResolvedValueOnce({ status: "reviewed", version: 2 })
      .mockResolvedValueOnce({ ...DETAIL, status: "reviewed", version: 2 });
    renderDetail();
    const user = userEvent.setup();

    expect(await screen.findByText("John Smith")).toBeInTheDocument();
    expect(screen.getByText(/MATH-301/)).toBeInTheDocument();
    expect(screen.getAllByText(/submitted/i).length).toBeGreaterThan(0);

    await user.click(screen.getByRole("button", { name: /mark reviewed/i }));
    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/abs-1/status",
        expect.objectContaining({ body: JSON.stringify({ status: "reviewed", expected_version: 1 }), method: "PUT" }),
      );
    });
  });

  it("saves internal notes using optimistic versioning", async () => {
    mockApiJson
      .mockResolvedValueOnce(DETAIL)
      .mockResolvedValueOnce({ version: 2, admin_notes: "Called guardian" })
      .mockResolvedValueOnce({ ...DETAIL, version: 2, admin_notes: "Called guardian" });
    renderDetail();
    const user = userEvent.setup();

    const note = await screen.findByLabelText(/internal note/i);
    await user.type(note, "Called guardian");
    await user.click(screen.getByRole("button", { name: /save note/i }));

    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absences/abs-1/notes",
      expect.objectContaining({ method: "PUT", body: JSON.stringify({ notes: "Called guardian", expected_version: 1 }) }),
    );
  });

  it("warns administrators when a manual sit-in session approaches room capacity", async () => {
    mockApiJson
      .mockResolvedValueOnce(DETAIL)
      .mockResolvedValueOnce([{ id: "sit-2", code: "MATH-201", name: "Calculus II" }])
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([{ id: "sess-2", start_at: "2026-06-03T04:00:00Z", end_at: "2026-06-03T05:30:00Z", room_name: "Room 105", capacity_warning: true }]);
    renderDetail();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /override sit-in/i }));
    await user.click(screen.getByRole("button", { name: /manual course/i }));
    await user.selectOptions(screen.getByLabelText("Course"), "sit-2");

    expect(await screen.findByText(/near capacity/i)).toBeInTheDocument();
  });
});
