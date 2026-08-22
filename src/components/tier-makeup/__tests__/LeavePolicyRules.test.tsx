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

  it("shows subject names in production course dropdown labels", async () => {
    mockApiJson
      .mockResolvedValueOnce([
        {
          id: "course-reading",
          code: "0000000026",
          name: "",
          subject_code: "05",
          subject_name: "SAT Verbal Reading Beginner",
        },
      ])
      .mockResolvedValueOnce({
        active: false,
        mappings: [],
        warnings: [],
        matched_courses: [],
        unmatched_policy_rows: [],
      });

    renderWithProviders(<LeavePolicyRules />);

    const options = await screen.findAllByRole("option", {
      name: /0000000026 - \(SAT Verbal Reading Beginner\)/i,
    });

    expect(options).toHaveLength(LEAVE_POLICY_COURSE_RULES.length);
    expect(options.every((option) => option.getAttribute("value") === "course-reading")).toBe(true);
    expect(screen.queryAllByRole("option", { name: /0000000026 - \(05\)/i })).toHaveLength(0);
  });

  it("saves SAT Verbal policy as one production course mapping per course rule", async () => {
    mockApiJson
      .mockResolvedValueOnce([
        { id: "course-r3s3", code: "R3S3", name: "Custom Verbal Section C", subject_code: "SATV" },
        { id: "course-believe", code: "BEL", name: "Specific Believe Name", subject_code: "BELIEVE" },
      ])
      .mockResolvedValueOnce({
        active: false,
        mappings: [],
        warnings: [],
        matched_courses: [],
        unmatched_policy_rows: [],
      })
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce({
        active: true,
        mappings: [
          {
            active: true,
            rule_id: "rank3-sec3",
            course_id: "course-r3s3",
            course_name: "Custom Verbal Section C",
          },
        ],
        warnings: ["No course selected for SAT Verbal Believe"],
        matched_courses: [{ policy_course_name: "SAT Verbal Rank 3-Section 3", course_name: "Custom Verbal Section C" }],
        unmatched_policy_rows: ["SAT Verbal Believe"],
      });

    renderWithProviders(<LeavePolicyRules />);

    const user = userEvent.setup();
    const ruleSelect = await screen.findByRole("combobox", { name: /SAT Verbal Rank 3-Section 3 production course/i });
    await user.selectOptions(ruleSelect, "course-r3s3");
    await user.click(screen.getByRole("button", { name: /save sat verbal course rules/i }));

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
    expect(body.policy).toEqual(LEAVE_POLICY_COURSE_RULES);
    expect(body.mappings).toEqual([{ rule_id: "rank3-sec3", course_id: "course-r3s3" }]);
    expect(body.subject_id).toBeUndefined();
    expect(screen.getByText(/No course selected for SAT Verbal Believe/)).toBeInTheDocument();
  });

  it("shows merged courses as selectable Leave Policy targets", async () => {
    mockApiJson
      .mockResolvedValueOnce([
        { id: "course-reading", code: "READ", name: "Reading", subject_code: "SATV" },
      ])
      .mockResolvedValueOnce({ active: false, mappings: [], warnings: [], matched_courses: [], unmatched_policy_rows: [] })
      .mockResolvedValueOnce([
        {
          id: "merge-1",
          name: "SAT Verbal Combined",
          level: 2,
          cycle_id: "cycle-1",
          cycle_label: "Cycle 1",
          sit_in_rule_id: null,
          course_codes: ["READ", "WRITE"],
          course_names: ["Reading", "Writing"],
        },
      ]);

    renderWithProviders(<LeavePolicyRules />);

    const options = await screen.findAllByRole("option", { name: /SAT Verbal Combined - READ \+ WRITE \(merged course\)/i });
    expect(options).toHaveLength(LEAVE_POLICY_COURSE_RULES.length);
    expect(options.every((option) => option.getAttribute("value") === "merge:merge-1")).toBe(true);
  });

  it("submits a merged course target as a merge group mapping", async () => {
    mockApiJson
      .mockResolvedValueOnce([
        { id: "course-reading", code: "READ", name: "Reading", subject_code: "SATV" },
      ])
      .mockResolvedValueOnce({ active: false, mappings: [], warnings: [], matched_courses: [], unmatched_policy_rows: [] })
      .mockResolvedValueOnce([
        {
          id: "merge-1",
          name: "SAT Verbal Combined",
          level: 2,
          cycle_id: "cycle-1",
          cycle_label: "Cycle 1",
          sit_in_rule_id: null,
          course_codes: ["READ", "WRITE"],
          course_names: ["Reading", "Writing"],
        },
      ])
      .mockResolvedValueOnce({
        active: true,
        mappings: [{ active: true, rule_id: LEAVE_POLICY_COURSE_RULES[0].id, course_id: null, merge_group_id: "merge-1" }],
        warnings: [],
        matched_courses: [],
        unmatched_policy_rows: [],
      });

    renderWithProviders(<LeavePolicyRules />);

    const user = userEvent.setup();
    const ruleSelect = await screen.findByRole("combobox", { name: `${LEAVE_POLICY_COURSE_RULES[0].courseName} production course` });
    await user.selectOptions(ruleSelect, "merge:merge-1");
    await user.click(screen.getByRole("button", { name: /save sat verbal course rules/i }));

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/admin/sat-verbal-policy/apply",
      expect.objectContaining({ method: "POST", body: expect.any(String) }),
    ));
    const applyCall = mockApiJson.mock.calls.find(([path]) => path === "/api/v1/admin/sat-verbal-policy/apply");
    expect(JSON.parse(applyCall?.[1]?.body as string).mappings).toEqual([
      { rule_id: LEAVE_POLICY_COURSE_RULES[0].id, merge_group_id: "merge-1" },
    ]);
  });
});
