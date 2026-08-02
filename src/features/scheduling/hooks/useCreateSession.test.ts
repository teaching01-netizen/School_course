import { describe, expect, it, beforeEach, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { ApiRequestError } from "@/api/client";
import { useCreateSession } from "./useCreateSession";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function renderCreateSession() {
  const onSuccess = vi.fn();
  const addToast = vi.fn();
  const rendered = renderHook(() => useCreateSession(onSuccess, addToast, "Asia/Bangkok"));
  return { result: rendered.result, onSuccess, addToast };
}

function fillForm(rendered: { current: ReturnType<typeof useCreateSession> }): void {
  act(() => {
    rendered.current.openModal({ course_id: "course-1", teacher_id: "teacher-1" });
    rendered.current.setForm((form) => ({
      ...form,
      room_id: "room-1",
      start_local: "2026-08-05T10:00",
      end_local: "2026-08-05T11:30",
    }));
  });
}

describe("useCreateSession", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("preflights an available standalone session before creating it in UTC", async () => {
    const callKinds: string[] = [];
    const createBodies: string[] = [];
    mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
      if (url === "/api/v1/scheduling/preflight" && init?.method === "POST") {
        callKinds.push("preflight");
        return { status: "available" };
      }
      if (url === "/api/v1/sessions" && init?.method === "POST") {
        callKinds.push("session");
        createBodies.push(String(init.body ?? ""));
        return { id: "session-1" };
      }
      throw new Error(`Unexpected API call: ${init?.method ?? "GET"} ${url}`);
    });

    const { result, onSuccess, addToast } = renderCreateSession();
    fillForm(result);

    await waitFor(() => expect(result.current.gate.canSave).toBe(true));

    await act(async () => {
      await result.current.submit();
    });

    expect(callKinds).toEqual(["preflight", "session"]);
    expect(createBodies).toHaveLength(1);
    expect(JSON.parse(createBodies[0])).toEqual({
      course_id: "course-1",
      room_id: "room-1",
      teacher_id: "teacher-1",
      start_at: "2026-08-05T03:00:00.000Z",
      end_at: "2026-08-05T04:30:00.000Z",
    });
    expect(addToast).toHaveBeenCalledWith("success", "Session created");
    expect(result.current.open).toBe(false);
    expect(onSuccess).toHaveBeenCalledTimes(1);
  });

  it("keeps the form open when the final write reports a room overlap", async () => {
    const conflict = new ApiRequestError(
      "Room overlaps an existing session. Choose a different room or time.",
      { status: 409, code: "room_overlap" },
    );
    const callKinds: string[] = [];
    mockApiJson.mockImplementation(async (url: string, init?: RequestInit) => {
      if (url === "/api/v1/scheduling/preflight" && init?.method === "POST") {
        callKinds.push("preflight");
        return { status: "available" };
      }
      if (url === "/api/v1/sessions" && init?.method === "POST") {
        callKinds.push("session");
        throw conflict;
      }
      throw new Error(`Unexpected API call: ${init?.method ?? "GET"} ${url}`);
    });

    const { result, onSuccess, addToast } = renderCreateSession();
    fillForm(result);

    await waitFor(() => expect(result.current.gate.canSave).toBe(true));
    const formBeforeSubmit = result.current.form;

    await act(async () => {
      await result.current.submit();
    });

    expect(callKinds).toEqual(["preflight", "session"]);
    expect(result.current.form).toEqual(formBeforeSubmit);
    expect(result.current.open).toBe(true);
    expect(addToast).toHaveBeenCalledWith(
      "error",
      "room_overlap: Room overlaps an existing session. Choose a different room or time.",
    );
    expect(addToast).not.toHaveBeenCalledWith("success", expect.anything());
    expect(onSuccess).not.toHaveBeenCalled();
  });
});
