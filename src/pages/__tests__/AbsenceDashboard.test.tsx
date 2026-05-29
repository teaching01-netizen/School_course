import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import AbsenceDashboard from "../AbsenceDashboard";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());
const mockDownloadApiFile = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson, downloadApiFile: mockDownloadApiFile };
});

it("renders monthly workflow totals and breakdowns", async () => {
  mockApiJson.mockResolvedValueOnce({
    period: "month",
    stats: { total_count: 47, pending_count: 12, reviewed_count: 21, actioned_count: 11, cancelled_count: 3, today_count: 5 },
    subjects: [{ label: "MATH", count: 8 }],
    reasons: [{ label: "medical", count: 20 }],
  });
  render(<MemoryRouter><ToastProvider><AbsenceDashboard /></ToastProvider></MemoryRouter>);

  expect(await screen.findByText("47")).toBeInTheDocument();
  expect(screen.getByText("MATH")).toBeInTheDocument();
  expect(screen.getByText("medical")).toBeInTheDocument();
});

it("exports the selected dashboard month as a report", async () => {
  mockApiJson.mockResolvedValue({
    period: "month",
    stats: { total_count: 1, pending_count: 1, reviewed_count: 0, actioned_count: 0, cancelled_count: 0, today_count: 0 },
    subjects: [],
    reasons: [],
  });
  mockDownloadApiFile.mockResolvedValueOnce(undefined);
  render(<MemoryRouter><ToastProvider><AbsenceDashboard /></ToastProvider></MemoryRouter>);
  const user = userEvent.setup();

  const exportBtn = await screen.findByRole("button", { name: /^export$/i });
  await user.click(exportBtn);

  expect(mockDownloadApiFile).toHaveBeenCalledWith(expect.stringMatching(/date_from=.*date_to=/));
});
