import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import CourseLevels from "../CourseLevels";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("CourseLevels merged course scope", () => {
  it("shows a merged course as one configurable level scope", async () => {
    mockApiJson
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce({ absence_policies: { root_course_groups: {} } })
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce({ subjects: [] })
      .mockResolvedValueOnce([
        {
          id: "merge-1",
          name: "SAT Verbal Reading + Writing",
          level: 2,
          cycle_id: "cy2025a",
          cycle_label: "Cycle 2025-01",
          sit_in_rule_id: null,
          course_codes: ["READ-R2", "WRITE-R2"],
          course_names: ["Reading Rank 2", "Writing Rank 2"],
        },
      ]);

    render(<MemoryRouter><ToastProvider><CourseLevels /></ToastProvider></MemoryRouter>);

    await waitFor(() => {
      expect(screen.getByText("SAT Verbal Reading + Writing")).toBeInTheDocument();
    });
    expect(screen.getByText("READ-R2 + WRITE-R2")).toBeInTheDocument();
    expect(screen.getByText("Merged course level")).toBeInTheDocument();
  });
});
