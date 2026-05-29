import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import OperationsHub from "../operations/OperationsHub";
import { ToastProvider } from "../../hooks/useToast";

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: vi.fn().mockResolvedValue({}) };
});

function renderWithRouter(initialEntries: string[] = ["/operations"]) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <ToastProvider>
        <OperationsHub />
      </ToastProvider>
    </MemoryRouter>
  );
}

describe("OperationsHub - Cross-links", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders "Course Levels" link pointing to /course-levels', () => {
    renderWithRouter();

    const courseLevelsLink = screen.getByText("Course Levels");
    expect(courseLevelsLink).toBeInTheDocument();
    expect(courseLevelsLink.closest("a")).toHaveAttribute("href", "/course-levels");
  });
});
