import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import LeavePolicyRules from "../LeavePolicyRules";
import { LEAVE_POLICY_COURSE_RULES } from "../leavePolicyData";
import { ToastProvider } from "../../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function renderWithProviders(ui: React.ReactElement) {
  return render(<ToastProvider>{ui}</ToastProvider>);
}

describe("LeavePolicyRules production mapping", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("applies the full SAT Verbal policy to one selected subject", async () => {
    mockApiJson
      .mockResolvedValueOnce([
        { id: "subject-satv", code: "SATV", name: "SAT Verbal" },
        { id: "subject-math", code: "MATH", name: "Math" },
      ])
      .mockResolvedValueOnce({
        active: false,
        warnings: [],
        matched_courses: [],
        unmatched_policy_rows: [],
        unmatched_courses: [],
      })
      .mockResolvedValueOnce({
        active: true,
        subject_id: "subject-satv",
        warnings: ["No course found for SAT Verbal Believe"],
        matched_courses: [{ policy_course_name: "SAT Verbal Rank 3-Section 3", course_name: "SAT Verbal Rank 3 Section 3" }],
        unmatched_policy_rows: ["SAT Verbal Believe"],
        unmatched_courses: [],
      });

    renderWithProviders(<LeavePolicyRules />);

    const user = userEvent.setup();
    const subjectSelect = await screen.findByRole("combobox", { name: /sat verbal subject/i });
    await user.selectOptions(subjectSelect, "subject-satv");
    await user.click(screen.getByRole("button", { name: /apply sat verbal policy/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/admin/sat-verbal-policy/apply",
        expect.objectContaining({
          method: "POST",
          body: expect.any(String),
        }),
      );
    });

    const applyCall = mockApiJson.mock.calls.find(([path]) => path === "/api/v1/admin/sat-verbal-policy/apply");
    expect(applyCall).toBeTruthy();
    const body = JSON.parse(applyCall?.[1]?.body as string);
    expect(body.subject_id).toBe("subject-satv");
    expect(body.policy).toEqual(LEAVE_POLICY_COURSE_RULES);
    expect(screen.getByText(/No course found for SAT Verbal Believe/)).toBeInTheDocument();
  });
});
