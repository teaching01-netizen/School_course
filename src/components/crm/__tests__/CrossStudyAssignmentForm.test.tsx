import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider } from "@/hooks/useToast";
import { apiJson } from "@/api/client";
import CrossStudyAssignmentForm from "../CrossStudyAssignmentForm";
import type { AssignmentDTO, CrmRowInfo, StudentInfo } from "@/types/crossStudy";

vi.mock("@/api/client", () => ({
  apiJson: vi.fn(),
}));

const courses = [
  {
    id: "source-course-id",
    code: "0000000099",
    name: "SAT Verbal Beginner Section 1 C2/26",
    subject_name: "SAT Verbal",
  },
  {
    id: "manual-source-course-id",
    code: "0000000100",
    name: "Manual Trusted Source Course",
    subject_name: "SAT Verbal",
  },
  {
    id: "course-a-id",
    code: "0000000013",
    name: "SAT Verbal Writing Beginner Section 1 C2/26",
    subject_name: "SAT Writing",
  },
  {
    id: "course-b-id",
    code: "0000000018",
    name: "SAT Verbal Reading Beginner Section 2 C2/26",
    subject_name: "SAT Reading",
  },
];

const student: StudentInfo = {
  id: "student-id",
  wcode: "W260032",
  full_name: "Korboon Kanchanomai",
};

const crmRowWithoutMappedSource: CrmRowInfo = {
  snapshot_id: "b8cd8dcf-fb6b-4f51-b681-d9f9270eac74",
  course_name: "SAT Verbal Beginner (Section 1) Cash Card",
  course_id: "",
  extra_note: "New Student - Summer Cash Card 2026 / เรียนไขว้ Sec.1&Sec.2 Tue Writing & Sat Reading",
  imported_at: "2026-05-31T13:06:08Z",
};

const currentAssignmentWithDestinations: AssignmentDTO = {
  id: "assignment-id",
  source_course: null,
  dest_course_a: courses[2],
  dest_course_b: courses[3],
  assigned_course_id: "course-a-id",
  status: "active",
  extra_note_snapshot: crmRowWithoutMappedSource.extra_note,
  source_valid: true,
  updated_at: "2026-06-14T03:21:34Z",
};

const crmRowWithMappedSource: CrmRowInfo = {
  ...crmRowWithoutMappedSource,
  course_id: "source-course-id",
};

const currentAssignmentWithMappedSource: AssignmentDTO = {
  ...currentAssignmentWithDestinations,
  source_course: courses[0],
};

function wrapper({ children }: { children: React.ReactNode }) {
  return <ToastProvider>{children}</ToastProvider>;
}

describe("CrossStudyAssignmentForm", () => {
  it("requires an editable source course before saving even when destinations are selected", () => {
    render(
      <CrossStudyAssignmentForm
        student={student}
        crmRow={crmRowWithoutMappedSource}
        currentAssignment={currentAssignmentWithDestinations}
        courses={courses}
        onSaved={vi.fn()}
      />,
      { wrapper },
    );

    expect(screen.getByRole("button", { name: /save assignment/i })).toBeDisabled();
    expect(screen.getByText(/choose the source course before saving/i)).toBeInTheDocument();
  });

  it("saves the staff-edited source course instead of the auto-matched source", async () => {
    const user = userEvent.setup();
    const mockApiJson = vi.mocked(apiJson);
    mockApiJson.mockResolvedValueOnce({ ok: true });

    render(
      <CrossStudyAssignmentForm
        student={student}
        crmRow={crmRowWithMappedSource}
        currentAssignment={currentAssignmentWithMappedSource}
        courses={courses}
        onSaved={vi.fn()}
      />,
      { wrapper },
    );

    const sourceInput = screen.getAllByRole("combobox")[0];
    await user.click(sourceInput);
    await user.type(sourceInput, "Manual");
    await user.click(screen.getByRole("option", { name: /manual trusted source course/i }));
    await user.click(screen.getByRole("button", { name: /save assignment/i }));

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledTimes(1));
    expect(mockApiJson).toHaveBeenCalledWith("/api/v1/cross-study/assignments", {
      method: "PUT",
      body: JSON.stringify({
        wcode: "W260032",
        source_course_id: "manual-source-course-id",
        snapshot_id: "b8cd8dcf-fb6b-4f51-b681-d9f9270eac74",
        dest_course_a_id: "course-a-id",
        dest_course_b_id: "course-b-id",
        assigned_course_id: "course-a-id",
        extra_note_text: crmRowWithoutMappedSource.extra_note,
      }),
    });
  });
});
