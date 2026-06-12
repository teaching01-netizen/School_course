import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import CrmFilterPanel from "./CrmFilterPanel";
import { apiJson } from "../../api/client";
import { ToastProvider } from "../../hooks/useToast";

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, apiJson: vi.fn() };
});

const mockApiJson = vi.mocked(apiJson);

const defaultFilter = {
  cycle_labels: [],
  cycle_blank_mode: "any",
  course_name_values: [],
  course_name_blank_mode: "any",
  academic_level_values: [],
  academic_level_blank_mode: "any",
  secondary_school_values: [],
  secondary_school_blank_mode: "any",
  teachers_contains: "",
  teachers_blank_mode: "any",
};

function renderPanel(onRosterChanged = vi.fn()) {
  render(
    <ToastProvider>
      <CrmFilterPanel courseId="course-1" isAdmin={true} onRosterChanged={onRosterChanged} embeddedInModal />
    </ToastProvider>,
  );
  return { onRosterChanged };
}

function arrangeApi(jobStatus: "failed" | "succeeded" | "running") {
  mockApiJson.mockImplementation(async (path: string, init?: RequestInit) => {
    if (path === "/api/v1/courses/course-1/crm-filter" && init?.method === "GET") {
      return { enabled: true, locked: false, filter: defaultFilter };
    }
    if (path === "/api/v1/crm/options") {
      return { cycle_labels: [], course_names: [], academic_levels: [], secondary_schools: [] };
    }
    if (path === "/api/v1/courses/course-1/crm-filter/preview") {
      return { distinct_students: 18 };
    }
    if (path === "/api/v1/courses/course-1/crm-filter" && init?.method === "PUT") {
      return { ok: true, job_id: "job-1", status: "queued" };
    }
    if (path === "/api/v1/courses/course-1/crm-filter/jobs/job-1") {
      if (jobStatus === "failed") {
        return {
          job_id: "job-1",
          status: "failed",
          message: "Student schedule conflict: Jane cannot be added to SAT",
          details: {
            kind: "crm_student_schedule_conflict",
            student: { wcode: "W250001", full_name: "Jane" },
            target_course: { code: "SAT" },
            conflicts: [{ course: { code: "ALG" }, start_at: "2026-05-20T10:00:00Z", end_at: "2026-05-20T11:00:00Z" }],
          },
        };
      }
      return { job_id: "job-1", status: jobStatus, message: `Course CRM reconcile job is ${jobStatus}` };
    }
    throw new Error(`Unexpected API call: ${path}`);
  });
}

describe("CrmFilterPanel reconcile status", () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    mockApiJson.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("keeps failed reconcile conflicts in the modal without refreshing roster", async () => {
    arrangeApi("failed");
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onRosterChanged } = renderPanel();

    const saveButton = await screen.findByRole("button", { name: "Save filter" });
    await user.click(saveButton);

    expect(await screen.findByText("CRM reconcile failed")).toBeInTheDocument();
    expect(screen.getByText(/^Jane cannot be added to SAT\.$/)).toBeInTheDocument();
    expect(screen.getByText("Conflicts with ALG at 20 May, 17:00-18:00.")).toBeInTheDocument();
    expect(screen.getByText("Student schedule conflict: Jane (W250001) cannot be added to SAT because they already have ALG at 20 May, 17:00-18:00")).toBeInTheDocument();
    expect(onRosterChanged).not.toHaveBeenCalled();
  });

  it("refreshes roster only after a successful terminal reconcile", async () => {
    arrangeApi("succeeded");
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onRosterChanged } = renderPanel();

    await user.click(await screen.findByRole("button", { name: "Save filter" }));

    expect(await screen.findByText("CRM reconcile complete")).toBeInTheDocument();
    await waitFor(() => expect(onRosterChanged).toHaveBeenCalledTimes(1));
  });

  it("disables save while reconcile is active", async () => {
    arrangeApi("running");
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPanel();

    const saveButton = await screen.findByRole("button", { name: "Save filter" });
    await user.click(saveButton);

    await waitFor(() => expect(saveButton).toBeDisabled());
    expect(screen.getByText("CRM reconcile running")).toBeInTheDocument();
  });
});
