import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { ToastProvider } from "../../hooks/useToast";
import { SitInRulesSection } from "../operations/SitInRulesSection";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("SitInRulesSection merged course scope", () => {
  it("shows a merged course in the policy configuration surface", async () => {
    mockApiJson
      .mockResolvedValueOnce([{ id: "rule-1", name: "Level Ladder", type: "level_ladder" }])
      .mockResolvedValueOnce({ items: [], total: 0, limit: 100, offset: 0 })
      .mockResolvedValueOnce({ absence_policies: { root_course_groups: {}, merge_groups: {} } })
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([
        {
          id: "merge-1",
          name: "SAT Verbal Reading + Writing",
          level: 2,
          cycle_id: "cy2025a",
          cycle_label: "Cycle 2025-01",
          sit_in_rule_id: "rule-1",
          course_codes: ["READ-R2", "WRITE-R2"],
          course_names: ["Reading Rank 2", "Writing Rank 2"],
        },
      ]);

    render(<ToastProvider><SitInRulesSection /></ToastProvider>);

    await waitFor(() => {
      expect(screen.getByText("SAT Verbal Reading + Writing")).toBeInTheDocument();
    });
    expect(screen.getByText("Merged course policies")).toBeInTheDocument();
    expect(screen.getByText("READ-R2 + WRITE-R2")).toBeInTheDocument();
  });
});
