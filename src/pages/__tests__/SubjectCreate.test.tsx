import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import SubjectCreate from "../SubjectCreate";
import { ToastProvider } from "../../hooks/useToast";
import { ApiRequestError } from "../../api/client";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("SubjectCreate", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("refreshes the generated id when create fails because the id already exists", async () => {
    mockApiJson
      .mockResolvedValueOnce([{ id: "s4", code: "04", name: "Existing" }])
      .mockRejectedValueOnce(new ApiRequestError("Already exists", { code: "conflict", status: 409 }))
      .mockResolvedValueOnce([
        { id: "s4", code: "04", name: "Existing" },
        { id: "s5", code: "05", name: "Newer existing" },
      ]);

    render(
      <MemoryRouter>
        <ToastProvider>
          <SubjectCreate />
        </ToastProvider>
      </MemoryRouter>,
    );
    const user = userEvent.setup();

    expect(await screen.findByDisplayValue("05")).toBeInTheDocument();
    await user.type(screen.getByLabelText(/name/i), "IELTS Writing");
    await user.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => {
      expect(screen.getByDisplayValue("06")).toBeInTheDocument();
    });
    expect(screen.getByText(/id was already used/i)).toBeInTheDocument();
  });
});
