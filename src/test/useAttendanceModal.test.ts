import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useAttendanceModal } from "@/hooks/useAttendanceModal";
import type { AttendanceOverride, Session, Student } from "@/types";
import { queryClient } from "@/query/cache";

const mockApiJson = vi.hoisted(() => vi.fn());
const mockUseRealtime = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", () => ({
  apiJson: mockApiJson,
}));

vi.mock("@/hooks/useRealtime", () => ({ useRealtime: mockUseRealtime }));

describe("useAttendanceModal", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockUseRealtime.mockReset();
    queryClient.clear();
  });

  it("reloads an open attendance modal after realtime reconnect", async () => {
    const session = {
      id: "session-reconnect",
      course_id: "course-reconnect",
      room_id: null,
      teacher_id: "teacher-1",
      start_at: "2026-06-12T09:00:00.000Z",
      end_at: "2026-06-12T10:00:00.000Z",
      version: 1,
    } satisfies Session;
    const initial = { id: "student-1", wcode: "W0001", full_name: "Before", notes: "" } satisfies Student;
    const updated = { ...initial, full_name: "After" };
    mockApiJson
      .mockResolvedValueOnce([initial])
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([updated])
      .mockResolvedValueOnce([]);
    const { result } = renderHook(() => useAttendanceModal(vi.fn()));
    await act(async () => result.current.openAttendance(session));
    expect(result.current.roster).toEqual([initial]);

    const options = mockUseRealtime.mock.calls.at(-1)?.[2] as { onReconnect?: () => void } | undefined;
    expect(options?.onReconnect).toBeTypeOf("function");
    await act(async () => options?.onReconnect?.());

    await waitFor(() => expect(result.current.roster).toEqual([updated]));
    expect(mockApiJson).toHaveBeenCalledTimes(4);
  });

  it("refreshes the merged roster after including a student by W-code", async () => {
    const session: Session = {
      id: "session-1",
      course_id: "course-1",
      room_id: null,
      teacher_id: "teacher-1",
      start_at: "2026-06-12T09:00:00.000Z",
      end_at: "2026-06-12T10:00:00.000Z",
      version: 1,
    };
    const existingStudent: Student = {
      id: "student-1",
      wcode: "W0001",
      full_name: "Existing Student",
      notes: "",
    };
    const addedStudent: Student = {
      id: "student-2",
      wcode: "W0002",
      full_name: "Added Student",
      notes: "",
    };
    const overrides: AttendanceOverride[] = [
      { student_id: addedStudent.id, status: "included", created_at: "2026-06-12T00:00:00.000Z" },
    ];

    mockApiJson
      .mockResolvedValueOnce([existingStudent])
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce(addedStudent)
      .mockResolvedValueOnce({ ok: true })
      .mockResolvedValueOnce(overrides)
      .mockResolvedValueOnce(addedStudent);

    const { result } = renderHook(() => useAttendanceModal(vi.fn()));

    await act(async () => {
      await result.current.openAttendance(session);
    });

    expect(result.current.roster).toEqual([existingStudent]);

    act(() => {
      result.current.setIncludeWcode("W0002");
    });

    await act(async () => {
      await result.current.addIncludedByWcode();
    });

    await waitFor(() => {
      expect(result.current.roster.map((student) => student.id)).toEqual(["student-1", "student-2"]);
    });
    expect(result.current.overrides).toEqual(overrides);
  });

  it("reuses cached roster and overrides when the same session is reopened", async () => {
    const session = {
      id: "session-cache",
      course_id: "course-cache",
      room_id: null,
      teacher_id: "teacher-1",
      start_at: "2026-06-12T09:00:00.000Z",
      end_at: "2026-06-12T10:00:00.000Z",
      version: 1,
    } satisfies Session;
    mockApiJson.mockResolvedValueOnce([]).mockResolvedValueOnce([]).mockResolvedValueOnce([]);
    const { result } = renderHook(() => useAttendanceModal(vi.fn()));

    await act(async () => result.current.openAttendance(session));
    await waitFor(() => expect(result.current.loading).toBe(false));
    act(() => result.current.closeAttendance());
    await act(async () => result.current.openAttendance(session));
    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(mockApiJson.mock.calls.filter(([url]) => String(url).includes("/courses/")).length).toBe(1);
    expect(mockApiJson.mock.calls.filter(([url]) => String(url).includes("/attendance")).length).toBe(2);
  });

  it("invalidates session and dashboard projections immediately after an attendance mutation", async () => {
    const session = {
      id: "session-invalidate",
      course_id: "course-invalidate",
      room_id: null,
      teacher_id: "teacher-1",
      start_at: "2026-06-12T09:00:00.000Z",
      end_at: "2026-06-12T10:00:00.000Z",
      version: 1,
    } satisfies Session;
    queryClient.setQueryData(["sessions", "list", "today"], [{ id: session.id }]);
    queryClient.setQueryData(["teacher-dashboards", "today"], { sessions: [{ id: session.id }] });
    mockApiJson
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce({ ok: true })
      .mockResolvedValueOnce([]);
    const { result } = renderHook(() => useAttendanceModal(vi.fn()));

    await act(async () => result.current.openAttendance(session));
    await act(async () => result.current.upsertAttendance("student-1", "included"));

    expect(queryClient.getQueryState(["sessions", "list", "today"])?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(["teacher-dashboards", "today"])?.isInvalidated).toBe(true);
  });
});
