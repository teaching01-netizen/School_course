import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import OperationsCalendar from "../OperationsCalendar";
import { ToastProvider } from "../../hooks/useToast";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function renderPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <OperationsCalendar />
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("OperationsCalendar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiJson.mockResolvedValueOnce({
      sessions: [],
      absence_days: [],
    });
  });

  it("renders week grid header", async () => {
    renderPage();
    expect(await screen.findByText("Operations Calendar")).toBeInTheDocument();
    expect(screen.getByText("Today")).toBeInTheDocument();
  });
});
