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
  row_hash: "crm-row-hash-1",
  xlsx_row_number: 12,
  course_name: "SAT Verbal Beginner (Section 1) Cash Card",
  course_id: "",
  extra_note: "New Student - Summer Cash Card 2026 / เรียนไขว้ Sec.1&Sec.2 Tue Writing & Sat Reading",
  imported_at: "2026-05-31T13:06:08Z",
};

const currentAssignmentWithDestinations: AssignmentDTO = {
  id: "assignment-id",
  dest_course_a: courses[2],
  dest_course_b: courses[3],
  dest_course_a_weekdays: [2],
  dest_course_b_weekdays: [6],
  assigned_course_id: "course-a-id",
  status: "active",
  extra_note_snapshot: crmRowWithoutMappedSource.extra_note,
  source_valid: true,
  updated_at: "2026-06-14T03:21:34Z",
};

function wrapper({ children }: { children: React.ReactNode }) {
  return <ToastProvider>{children}</ToastProvider>;
}

describe("CrossStudyAssignmentForm", () => {
  it("does not render a source course selector because staff only chooses destination courses", () => {
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

    expect(screen.queryByText(/source course to treat this row as/i)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /save assignment/i })).toBeEnabled();
  });

  it("saves only destination courses even when the CRM source course is not mapped", async () => {
    const mockApiJson = vi.mocked(apiJson);
    mockApiJson.mockResolvedValueOnce({ ok: true });

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

    await userEvent.click(screen.getByRole("button", { name: /save assignment/i }));

    await waitFor(() =>
      expect(
        mockApiJson.mock.calls.some(
          ([url, options]) =>
            url === "/api/v1/cross-study/assignments" &&
            (options as { method?: string } | undefined)?.method === "PUT",
        ),
      ).toBe(true),
    );
    expect(mockApiJson).toHaveBeenCalledWith("/api/v1/cross-study/assignments", {
      method: "PUT",
      body: JSON.stringify({
        wcode: "W260032",
        snapshot_id: "b8cd8dcf-fb6b-4f51-b681-d9f9270eac74",
        crm_course_name: "SAT Verbal Beginner (Section 1) Cash Card",
        crm_row_hash: "crm-row-hash-1",
        crm_xlsx_row_number: 12,
        dest_course_a_id: "course-a-id",
        dest_course_b_id: "course-b-id",
        dest_course_a_weekdays: [2],
        dest_course_b_weekdays: [6],
        assigned_course_id: "course-a-id",
        extra_note_text: crmRowWithoutMappedSource.extra_note,
      }),
    });
  });

  it("lets staff scope each destination course to different weekdays", async () => {
    const mockApiJson = vi.mocked(apiJson);
    mockApiJson.mockResolvedValueOnce({ ok: true });

    render(
      <CrossStudyAssignmentForm
        student={student}
        crmRow={crmRowWithoutMappedSource}
        currentAssignment={null}
        courses={courses}
        onSaved={vi.fn()}
      />,
      { wrapper },
    );

    const [courseAInput, courseBInput] = screen.getAllByRole("combobox");
    await userEvent.click(courseAInput);
    await userEvent.type(courseAInput, "Writing");
    await userEvent.click(screen.getByRole("option", { name: /writing beginner/i }));
    await userEvent.click(courseBInput);
    await userEvent.type(courseBInput, "Reading");
    await userEvent.click(screen.getByRole("option", { name: /reading beginner/i }));

    await userEvent.click(screen.getByRole("checkbox", { name: /course a.*tue/i }));
    await userEvent.click(screen.getByRole("checkbox", { name: /course b.*sat/i }));
    await userEvent.click(screen.getByRole("button", { name: /save assignment/i }));

    await waitFor(() =>
      expect(
        mockApiJson.mock.calls.some(
          ([url, options]) =>
            url === "/api/v1/cross-study/assignments" &&
            (options as { method?: string } | undefined)?.method === "PUT",
        ),
      ).toBe(true),
    );
    expect(mockApiJson).toHaveBeenCalledWith("/api/v1/cross-study/assignments", {
      method: "PUT",
      body: JSON.stringify({
        wcode: "W260032",
        snapshot_id: "b8cd8dcf-fb6b-4f51-b681-d9f9270eac74",
        crm_course_name: "SAT Verbal Beginner (Section 1) Cash Card",
        crm_row_hash: "crm-row-hash-1",
        crm_xlsx_row_number: 12,
        dest_course_a_id: "course-a-id",
        dest_course_b_id: "course-b-id",
        dest_course_a_weekdays: [2],
        dest_course_b_weekdays: [6],
        assigned_course_id: "course-a-id",
        extra_note_text: crmRowWithoutMappedSource.extra_note,
      }),
    });
  });
});
