import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { useEditSession } from "./useEditSession";
import { ApiRequestError } from "@/api/client";
import type { AttendanceOverride, Session } from "../types";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

const SESSION: Session = {
  id: "sess-1",
  course_id: "course-1",
  room_id: "room-1",
  teacher_id: "teacher-1",
  start_at: "2025-08-14T01:00:00.000Z",
  end_at: "2025-08-14T02:30:00.000Z",
  version: 3,
};

type MockOptions = {
  attendance?: AttendanceOverride[] | Error;
  preview?: Record<string, unknown>;
  patch?: unknown;
  updatedSession?: Session;
};

// Dispatches on URL + method; per-test overrides via opts.
function mockDefaultDispatch(opts: MockOptions = {}) {
  mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
    const method = (init?.method ?? "GET").toUpperCase();
    if (url === "/api/v1/sessions/sess-1/attendance" && method === "GET") {
      if (opts.attendance instanceof Error) throw opts.attendance;
      return opts.attendance ?? [];
    }
    if (url === "/api/v1/scheduling/preflight" && method === "POST") return { status: "available" };
    if (url === "/api/v1/sessions/sess-1/change-preview" && method === "POST") {
      return opts.preview ?? { requires_acknowledgement: false };
    }
    if (url === "/api/v1/sessions/sess-1" && method === "PATCH") {
      if (opts.patch instanceof Error) throw opts.patch;
      return opts.patch ?? { change_id: "chg-1" };
    }
    if (url === "/api/v1/sessions?ids=sess-1" && method === "GET") return [opts.updatedSession ?? SESSION];
    throw new Error(`Unexpected API call: ${method} ${url}`);
  });
}

function renderEditSession() {
  const onSuccess = vi.fn();
  const addToast = vi.fn();
  const { result } = renderHook(
    ({ onSuccess, addToast }: { onSuccess: () => void; addToast: (type: string, msg: string) => void }) =>
      useEditSession(onSuccess, addToast, "Asia/Bangkok"),
    { initialProps: { onSuccess, addToast } },
  );
  return { result, onSuccess, addToast };
}

describe("useEditSession", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("openModal populates the form from the session and opens", async () => {
    mockDefaultDispatch();
    const { result } = renderEditSession();

    await act(async () => {
      result.current.openModal(SESSION);
    });

    expect(result.current.open).toBe(true);
    expect(result.current.form.course_id).toBe("course-1");
    expect(result.current.form.room_id).toBe("room-1");
    expect(result.current.form.teacher_id).toBe("teacher-1");
    expect(result.current.form.start_local).toBe("2025-08-14T08:00");
    expect(result.current.form.end_local).toBe("2025-08-14T09:30");
  });

  it("runs the change-preview before the PATCH and closes the modal on success", async () => {
    mockDefaultDispatch({
      preview: { requires_acknowledgement: false },
      patch: { change_id: "chg-1" },
    });
    const { result, onSuccess, addToast } = renderEditSession();

    await act(async () => {
      result.current.openModal(SESSION);
    });
    await waitFor(() => expect(result.current.gate.canSave).toBe(true));

    await act(async () => {
      await result.current.submit();
    });

    const calls = mockApiJson.mock.calls;
    const previewIdx = calls.findIndex(
      ([url, init]) => url === "/api/v1/sessions/sess-1/change-preview" && init?.method === "POST",
    );
    const patchIdx = calls.findIndex(
      ([url, init]) => url === "/api/v1/sessions/sess-1" && init?.method === "PATCH",
    );
    expect(previewIdx).toBeGreaterThanOrEqual(0);
    expect(patchIdx).toBeGreaterThan(previewIdx);
    const patchBody = JSON.parse((calls[patchIdx][1] as RequestInit).body as string);
    expect(patchBody.expected_version).toBe(3);
    expect(patchBody.start_at).toBe("2025-08-14T01:00:00.000Z");
    expect(patchBody.end_at).toBe("2025-08-14T02:30:00.000Z");
    expect(addToast).toHaveBeenCalledWith("success", expect.stringMatching(/Impact review queued/));
    expect(result.current.open).toBe(false);
    expect(onSuccess).toHaveBeenCalledTimes(1);
  });

  it("holds the PATCH when the preview requires acknowledgement", async () => {
    const impactSummary = { predicted_student_overlaps: 2, short_notice: true };
    mockDefaultDispatch({
      preview: { requires_acknowledgement: true, impact_summary: impactSummary },
    });
    const { result, onSuccess } = renderEditSession();

    await act(async () => {
      result.current.openModal(SESSION);
    });
    await waitFor(() => expect(result.current.gate.canSave).toBe(true));

    await act(async () => {
      await result.current.submit();
    });

    expect(result.current.pendingImpact).toEqual(impactSummary);
    const patchCalls = mockApiJson.mock.calls.filter(
      ([url, init]) => url === "/api/v1/sessions/sess-1" && init?.method === "PATCH",
    );
    expect(patchCalls).toHaveLength(0);
    expect(onSuccess).not.toHaveBeenCalled();
  });

  it("confirmImpact acknowledges the impact and sends the PATCH", async () => {
    const impactSummary = { predicted_student_overlaps: 2, short_notice: true };
    mockDefaultDispatch({
      preview: { requires_acknowledgement: true, impact_summary: impactSummary },
      patch: { change_id: "chg-1" },
    });
    const { result, onSuccess } = renderEditSession();

    await act(async () => {
      result.current.openModal(SESSION);
    });
    await waitFor(() => expect(result.current.gate.canSave).toBe(true));

    await act(async () => {
      await result.current.submit();
    });
    expect(result.current.pendingImpact).toEqual(impactSummary);

    await act(async () => {
      await result.current.confirmImpact();
    });

    expect(result.current.pendingImpact).toBeNull();
    const patchCall = mockApiJson.mock.calls.find(
      ([url, init]) => url === "/api/v1/sessions/sess-1" && init?.method === "PATCH",
    );
    expect(patchCall).toBeDefined();
    const patchBody = JSON.parse((patchCall![1] as RequestInit).body as string);
    expect(patchBody.acknowledge_impact).toBe(true);
    expect(onSuccess).toHaveBeenCalledTimes(1);
  });

  it("toasts plain \"Updated session\" when no change_id is returned", async () => {
    mockDefaultDispatch({ preview: { requires_acknowledgement: false }, patch: {} });
    const { result, addToast } = renderEditSession();

    await act(async () => {
      result.current.openModal(SESSION);
    });
    await waitFor(() => expect(result.current.gate.canSave).toBe(true));

    await act(async () => {
      await result.current.submit();
    });

    expect(addToast).toHaveBeenCalledWith("success", "Updated session");
  });

  it("does not send attendance overrides before the attendance response is available", async () => {
    let resolveAttendance: ((overrides: AttendanceOverride[]) => void) | undefined;
    mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
      const method = (init?.method ?? "GET").toUpperCase();
      if (url === "/api/v1/sessions/sess-1/attendance" && method === "GET") {
        return new Promise<AttendanceOverride[]>((resolve) => {
          resolveAttendance = resolve;
        });
      }
      if (url === "/api/v1/scheduling/preflight" && method === "POST") return { status: "available" };
      return undefined;
    });
    const { result } = renderEditSession();

    await act(async () => {
      result.current.openModal(SESSION);
    });

    await waitFor(() => {
      const preflightCall = mockApiJson.mock.calls.find(
        ([url, init]) => url === "/api/v1/scheduling/preflight" && init?.method === "POST",
      );
      expect(preflightCall).toBeDefined();
      const body = JSON.parse((preflightCall?.[1] as RequestInit).body as string);
      expect(body).not.toHaveProperty("included_student_ids");
      expect(body).not.toHaveProperty("excluded_student_ids");
    });

    await act(async () => {
      resolveAttendance?.([]);
    });
  });

  it("omits unverified attendance overrides when attendance loading fails", async () => {
    mockDefaultDispatch({ attendance: new Error("attendance unavailable") });
    const { result } = renderEditSession();

    await act(async () => {
      result.current.openModal(SESSION);
    });

    await waitFor(() => {
      const preflightCall = mockApiJson.mock.calls.find(
        ([url, init]) => url === "/api/v1/scheduling/preflight" && init?.method === "POST",
      );
      expect(preflightCall).toBeDefined();
      const body = JSON.parse((preflightCall?.[1] as RequestInit).body as string);
      expect(body).not.toHaveProperty("included_student_ids");
      expect(body).not.toHaveProperty("excluded_student_ids");
    });
    expect(result.current.attendanceOverridesLoaded).toBe(false);
  });

  it("reloads the latest session on stale_edit", async () => {
    const updatedSession: Session = {
      id: "sess-1",
      course_id: "course-2",
      room_id: null,
      teacher_id: "teacher-2",
      start_at: "2025-08-15T01:00:00.000Z",
      end_at: "2025-08-15T02:00:00.000Z",
      version: 4,
    };
    mockDefaultDispatch({
      preview: { requires_acknowledgement: false },
      patch: new ApiRequestError("Stale edit", { code: "stale_edit" }),
      updatedSession,
    });
    const { result, onSuccess, addToast } = renderEditSession();

    await act(async () => {
      result.current.openModal(SESSION);
    });
    await waitFor(() => expect(result.current.gate.canSave).toBe(true));

    await act(async () => {
      await result.current.submit();
    });

    expect(addToast).toHaveBeenCalledWith("error", expect.stringMatching(/Stale edit: reloaded latest session/));
    expect(result.current.session?.version).toBe(4);
    expect(result.current.form.course_id).toBe("course-2");
    expect(result.current.form.room_id).toBe("");
    expect(result.current.form.teacher_id).toBe("teacher-2");
    expect(result.current.form.start_local).toBe("2025-08-15T08:00");
    expect(result.current.form.end_local).toBe("2025-08-15T09:00");
    expect(onSuccess).toHaveBeenCalledTimes(1);
    expect(result.current.saving).toBe(false);
  });

  it("surfaces code and message for other ApiRequestErrors", async () => {
    mockDefaultDispatch({
      preview: { requires_acknowledgement: false },
      patch: new ApiRequestError("Conflict", { code: "conflict" }),
    });
    const { result, onSuccess, addToast } = renderEditSession();

    await act(async () => {
      result.current.openModal(SESSION);
    });
    await waitFor(() => expect(result.current.gate.canSave).toBe(true));

    await act(async () => {
      await result.current.submit();
    });

    expect(addToast).toHaveBeenCalledWith("error", "conflict: Conflict");
    expect(onSuccess).not.toHaveBeenCalled();
  });

  it("skips preview and further preflight calls when the date range is invalid", async () => {
    mockDefaultDispatch();
    const { result } = renderEditSession();

    await act(async () => {
      result.current.openModal(SESSION);
    });
    await waitFor(() => expect(result.current.gate.canSave).toBe(true));

    const isPreflightCall = (url: string, init?: RequestInit) =>
      url === "/api/v1/scheduling/preflight" && init?.method === "POST";
    expect(mockApiJson.mock.calls.filter(([url, init]) => isPreflightCall(url as string, init as RequestInit))).toHaveLength(1);

    act(() => {
      result.current.setForm({
        ...result.current.form,
        start_local: "2025-08-14T03:00",
        end_local: "2025-08-14T02:00",
      });
    });

    expect(result.current.gate.canSave).toBe(false);
    expect(mockApiJson.mock.calls.filter(([url, init]) => isPreflightCall(url as string, init as RequestInit))).toHaveLength(1);

    await act(async () => {
      await result.current.submit();
    });

    expect(
      mockApiJson.mock.calls.filter(
        ([url, init]) => url === "/api/v1/sessions/sess-1/change-preview" && init?.method === "POST",
      ),
    ).toHaveLength(0);
    expect(result.current.open).toBe(true);
  });

  it("an older preflight result does not overwrite a newer one", async () => {
    let resolveA!: (v: { status: string }) => void;
    let resolveB!: (v: { status: string }) => void;
    let bCallStarted = false;
    mockApiJson.mockImplementation(async (url: string, _init?: RequestInit) => {
      if (url.endsWith("/attendance")) return [];
      if (url.endsWith("/preflight")) {
        if (!resolveA) return new Promise((r) => { resolveA = r; });
        bCallStarted = true;
        return new Promise((r) => { resolveB = r; });
      }
      return undefined;
    });

    const { result } = renderEditSession();

    await act(async () => {
      result.current.openModal(SESSION);
    });

    // Debounced preflight A fires after 300ms — wait for it to start loading
    await waitFor(() => {
      expect(result.current.preflight.loading).toBe(true);
    });

    act(() => {
      result.current.setForm((f) => ({ ...f, start_local: "2025-08-14T10:00", end_local: "2025-08-14T11:00" }));
    });

    // Wait for preflight B API call to be made (debounced after form change)
    await waitFor(() => {
      expect(bCallStarted).toBe(true);
    });

    // Resolve the newer check (B) first with available...
    await act(async () => {
      resolveB({ status: "available" });
    });
    await waitFor(() => {
      expect(result.current.preflight.status).toBe("available");
    });
    expect(result.current.preflight.loading).toBe(false);

    // ...then the older one (A) resolves later with provisional — must be ignored.
    await act(async () => {
      resolveA({ status: "provisional" });
    });
    expect(result.current.preflight.status).toBe("available");
    expect(result.current.preflight.loading).toBe(false);
  });

  it("closeModal resets all modal state", async () => {
    mockDefaultDispatch({
      preview: { requires_acknowledgement: true, impact_summary: { unresolved: 1, critical: 0, warning: 0 } },
    });
    const { result } = renderEditSession();

    await act(async () => {
      result.current.openModal(SESSION);
    });
    await waitFor(() => expect(result.current.gate.canSave).toBe(true));

    await act(async () => {
      await result.current.submit();
    });
    expect(result.current.pendingImpact).not.toBeNull();

    act(() => {
      result.current.closeModal();
    });

    expect(result.current.open).toBe(false);
    expect(result.current.session).toBeNull();
    expect(result.current.form).toEqual({ course_id: "", room_id: "", teacher_id: "", start_local: "", end_local: "" });
    expect(result.current.pendingImpact).toBeNull();
    expect(result.current.attendanceOverrides).toEqual([]);
    expect(result.current.attendanceOverridesLoaded).toBe(false);
  });
});
