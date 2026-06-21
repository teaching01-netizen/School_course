import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider } from "../../../hooks/useToast";
import OverrideSitInModal from "../OverrideSitInModal";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("OverrideSitInModal", () => {
  beforeEach(() => { mockApiJson.mockReset(); });

  it("does not show code - name fallback in course dropdown when subject_name is missing", async () => {
    mockApiJson
      .mockResolvedValueOnce([
        { id: "course-1", code: "MATH-201", name: "Algebra II", subject_name: null },
      ]);

    render(
      <ToastProvider>
        <OverrideSitInModal
          absenceId="abs-1"
          version={1}
          onClose={vi.fn()}
          onSaved={vi.fn()}
        />
      </ToastProvider>,
    );

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/courses/public",
        expect.objectContaining({ method: "GET" }),
      );
    });

    expect(screen.queryByRole("option", { name: "MATH-201 - Algebra II" })).not.toBeInTheDocument();
  });

  it("shows confirmation without SMS preview when preview fetch fails, still allows Send/Skip", async () => {
    mockApiJson
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new Error("Preview unavailable"));

    const onClose = vi.fn();
    const onSaved = vi.fn();

    render(
      <ToastProvider>
        <OverrideSitInModal
          absenceId="abs-1"
          version={1}
          currentMethod="zoom"
          onClose={onClose}
          onSaved={onSaved}
        />
      </ToastProvider>,
    );

    const user = userEvent.setup();

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/courses/public",
        expect.objectContaining({ method: "GET" }),
      );
    });

    await user.click(screen.getByRole("button", { name: /save override/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/abs-1/sit-in",
        expect.objectContaining({
          method: "PUT",
          body: expect.stringContaining("Override by admin"),
        }),
      );
    });

    await waitFor(() => {
      expect(screen.getByText(/override saved/i)).toBeInTheDocument();
    });

    expect(screen.getByText(/sms preview unavailable/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /send sms/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /skip/i })).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
    expect(onSaved).not.toHaveBeenCalled();
  });

  it("shows confirmation with phone numbers and SMS preview after save", async () => {
    mockApiJson
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce(undefined)
      .mockResolvedValueOnce({
        student_phone: "+66812345678",
        parent_phone: "+66812345679",
        student_sms: "Your sit-in has been updated to Zoom",
        parent_sms: "Your child's sit-in has been updated to Zoom",
      });

    const onClose = vi.fn();
    const onSaved = vi.fn();

    render(
      <ToastProvider>
        <OverrideSitInModal
          absenceId="abs-1"
          version={1}
          currentMethod="zoom"
          onClose={onClose}
          onSaved={onSaved}
        />
      </ToastProvider>,
    );

    const user = userEvent.setup();

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/courses/public",
        expect.objectContaining({ method: "GET" }),
      );
    });

    await user.click(screen.getByRole("button", { name: /save override/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/abs-1/sit-in",
        expect.objectContaining({
          method: "PUT",
          body: expect.stringContaining("Override by admin"),
        }),
      );
    });

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/abs-1/sms-preview",
        expect.objectContaining({ method: "GET" }),
      );
    });

    expect(screen.getByText(/override saved/i)).toBeInTheDocument();
    expect(screen.getByText(/\+66812345678/)).toBeInTheDocument();
    expect(screen.getByText(/\+66812345679/)).toBeInTheDocument();
    expect(screen.getByText(/your sit-in has been updated to zoom/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /send sms/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /skip/i })).toBeInTheDocument();

    expect(onClose).not.toHaveBeenCalled();
    expect(onSaved).not.toHaveBeenCalled();
  });

  it("calls sms-notify and fires onSaved + onClose when Send SMS is clicked", async () => {
    mockApiJson
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce(undefined)
      .mockResolvedValueOnce({
        student_phone: "+66812345678",
        parent_phone: "+66812345679",
        student_sms: "Your sit-in has been updated to Zoom",
        parent_sms: "Your child's sit-in has been updated to Zoom",
      })
      .mockResolvedValueOnce(undefined);

    const onClose = vi.fn();
    const onSaved = vi.fn();

    render(
      <ToastProvider>
        <OverrideSitInModal
          absenceId="abs-1"
          version={1}
          currentMethod="zoom"
          onClose={onClose}
          onSaved={onSaved}
        />
      </ToastProvider>,
    );

    const user = userEvent.setup();

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/courses/public",
        expect.objectContaining({ method: "GET" }),
      );
    });

    await user.click(screen.getByRole("button", { name: /save override/i }));

    await waitFor(() => {
      expect(screen.getByText(/send sms/i)).toBeInTheDocument();
    });

    expect(onClose).not.toHaveBeenCalled();
    expect(onSaved).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: /send sms/i }));

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/abs-1/sms-notify",
        expect.objectContaining({ method: "POST" }),
      );
    });

    await waitFor(() => {
      expect(onClose).toHaveBeenCalled();
    });

    expect(onSaved).toHaveBeenCalled();
  });

  it("fires onSaved + onClose without calling sms-notify when Skip is clicked", async () => {
    mockApiJson
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce(undefined)
      .mockResolvedValueOnce({
        student_phone: "+66812345678",
        parent_phone: "+66812345679",
        student_sms: "Your sit-in has been updated to Zoom",
        parent_sms: "Your child's sit-in has been updated to Zoom",
      });

    const onClose = vi.fn();
    const onSaved = vi.fn();

    render(
      <ToastProvider>
        <OverrideSitInModal
          absenceId="abs-1"
          version={1}
          currentMethod="zoom"
          onClose={onClose}
          onSaved={onSaved}
        />
      </ToastProvider>,
    );

    await waitFor(() => {
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/courses/public",
        expect.objectContaining({ method: "GET" }),
      );
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /save override/i }));

    await waitFor(() => {
      expect(screen.getByText(/skip/i)).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /skip/i }));

    await waitFor(() => {
      expect(onClose).toHaveBeenCalled();
    });

    expect(onSaved).toHaveBeenCalled();
    expect(mockApiJson).not.toHaveBeenCalledWith(
      "/api/v1/absences/abs-1/sms-notify",
      expect.anything(),
    );
  });
});
