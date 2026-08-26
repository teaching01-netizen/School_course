import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { apiJson } from "@/api/client";
import CrossStudyPage from "../CrossStudyPage";

vi.mock("@/api/client", () => ({ apiJson: vi.fn() }));
vi.mock("@/components/crm/CrossStudyAssignmentList", () => ({
  default: () => null,
}));
vi.mock("@/components/crm/CrossStudyStudentSearch", () => ({
  default: ({ onSearch }: { onSearch: (wcode: string) => void }) => (
    <button type="button" onClick={() => onSearch("W260001")}>Lookup student</button>
  ),
}));
vi.mock("@/components/crm/CrossStudyAssignmentForm", () => ({
  default: ({ crmRow }: { crmRow: { course_name: string } }) => (
    <div>Assignment source: {crmRow.course_name}</div>
  ),
}));

describe("CrossStudyPage", () => {
  beforeEach(() => {
    vi.mocked(apiJson).mockImplementation(async (url: string) => {
      if (url === "/api/v1/courses") return [];
      return {
        student: { id: "student-id", wcode: "w260001", full_name: "Test Student" },
        crm_rows: [
          {
            snapshot_id: "snapshot-id",
            row_hash: "row-a",
            xlsx_row_number: 1,
            course_name: "Course A",
            course_id: "course-a-id",
            extra_note: "First note",
            imported_at: "2026-08-26T00:00:00Z",
          },
          {
            snapshot_id: "snapshot-id",
            row_hash: "row-b",
            xlsx_row_number: 2,
            course_name: "Course B",
            course_id: "course-b-id",
            extra_note: "Second note",
            imported_at: "2026-08-26T00:00:00Z",
          },
        ],
        current_assignment: null,
      };
    });
  });

  it("shows every CRM row and lets staff choose the assignment source", async () => {
    // Given: a student with two CRM rows.
    render(<CrossStudyPage />);

    // When: staff looks up the student and selects the second row.
    await userEvent.click(screen.getByRole("button", { name: "Lookup student" }));
    expect(await screen.findByText("Course A")).toBeInTheDocument();
    expect(screen.getByText("Course B")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /select course b/i }));

    // Then: the assignment form receives the selected CRM row.
    expect(screen.getByText("Assignment source: Course B")).toBeInTheDocument();
  });
});
