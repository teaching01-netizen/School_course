import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useAttendanceModal } from "@/hooks/useAttendanceModal";
import type { AttendanceOverride, Session, Student } from "@/types";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", () => ({
  apiJson: mockApiJson,
}));

describe("useAttendanceModal", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
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
      .mockResolvedValueOnce([existingStudent])
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
});
