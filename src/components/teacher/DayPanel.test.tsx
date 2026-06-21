import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { StrictMode } from "react";
import DayPanel from "./DayPanel";
import type { TeacherAbsenceDetail, TeacherDashboardSession } from "../../types";
import { queryClient } from "../../query/cache";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("../../api/client", async () => {
  const actual = await vi.importActual<typeof import("../../api/client")>("../../api/client");
  return { ...actual, apiJson: mockApiJson };
});

function renderPanel(sessions: TeacherDashboardSession[]) {
  return render(
    <MemoryRouter>
      <DayPanel date={new Date("2026-06-20T00:00:00Z")} sessions={sessions} onClose={vi.fn()} />
    </MemoryRouter>,
  );
}

function renderStrictPanel(sessions: TeacherDashboardSession[]) {
  return render(
    <StrictMode>
      <MemoryRouter>
        <DayPanel date={new Date("2026-06-20T00:00:00Z")} sessions={sessions} onClose={vi.fn()} />
      </MemoryRouter>
    </StrictMode>,
  );
}

const baseSession: TeacherDashboardSession = {
  id: "s-1",
  course_id: "c-1",
  course_code: "SATM101",
  course_name: "SAT Math Beginner C2",
  subject_name: "Math",
  start_at: "2026-06-20T06:00:00Z",
  end_at: "2026-06-20T07:00:00Z",
  room_name: "Room 12",
  absent_count: 1,
  absent_students: [],
  sit_in_visitors: [],
};

function absenceDetail(overrides: Partial<TeacherAbsenceDetail> & Pick<TeacherAbsenceDetail, "id">): TeacherAbsenceDetail {
  return {
    student_name: "Titan",
    student_nickname: null,
    wcode: "W260114",
    course_code: "SATM101",
    course_name: "SAT Math Beginner C2",
    subject_name: "Math",
    date_from: "2026-06-20",
    date_to: "2026-06-20",
    reason_category: "medical",
    reason: "Doctor appointment",
    status: "pending",
    missed_sessions: [],
    sit_in_sessions: [],
    version: 1,
    ...overrides,
  };
}

describe("DayPanel", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    queryClient.clear();
  });

  it("loads and shows reason previews for absent students and sit-ins", async () => {
    const sessions: TeacherDashboardSession[] = [
      {
        ...baseSession,
        absent_students: [
          { wcode: "W260114", nickname: "Titan", student_name: null, absence_id: "ab-1", created_at: "2026-06-20T01:00:00Z" },
        ],
        sit_in_visitors: [
          {
            wcode: "W260207",
            nickname: "Nut",
            student_name: null,
            from_course_code: "ENG201",
            from_subject_name: "English",
            absence_id: "ab-2",
            session_start_at: "2026-06-20T08:00:00Z",
            session_end_at: "2026-06-20T09:00:00Z",
            absent_subject_name: "English",
            absence_date: "2026-06-19",
          },
        ],
      },
    ];

    mockApiJson.mockImplementation(async (path: string) => {
      if (path === "/api/v1/teacher/absences/ab-1") {
        return absenceDetail({ id: "ab-1", reason: "Recovering from flu" });
      }
      if (path === "/api/v1/teacher/absences/ab-2") {
        return absenceDetail({ id: "ab-2", reason: "Family trip" });
      }
      throw new Error(`Unexpected path: ${path}`);
    });

    renderPanel(sessions);

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledTimes(2));
    expect(await screen.findByText("Recovering from flu")).toBeInTheDocument();
    expect(screen.getByText("Family trip")).toBeInTheDocument();
    expect(screen.getAllByText("Reason:")).toHaveLength(2);
  });

  it("never falls back to a course code for a sit-in origin", async () => {
    mockApiJson.mockResolvedValue(absenceDetail({ id: "ab-2", reason: "Family trip" }));
    renderPanel([{
      ...baseSession,
      sit_in_visitors: [{
        wcode: "W260207",
        nickname: "Nut",
        student_name: null,
        from_course_code: "ENG201",
        from_subject_name: null,
        absence_id: "ab-2",
        session_start_at: "2026-06-20T08:00:00Z",
        session_end_at: "2026-06-20T09:00:00Z",
        absent_subject_name: null,
        absence_date: "2026-06-19",
      }],
    }]);

    expect(await screen.findByText(/sit-in from Subject unavailable/)).toBeInTheDocument();
    expect(screen.queryByText(/ENG201/)).not.toBeInTheDocument();
  });

  it("leaves loading state after a successful request in StrictMode", async () => {
    const sessions: TeacherDashboardSession[] = [
      {
        ...baseSession,
        absent_students: [
          { wcode: "W260114", nickname: "Titan", student_name: null, absence_id: "ab-strict", created_at: "2026-06-20T01:00:00Z" },
        ],
      },
    ];

    mockApiJson.mockResolvedValue(absenceDetail({ id: "ab-strict", reason: "Family travel" }));

    renderStrictPanel(sessions);

    expect(await screen.findByText("Family travel")).toBeInTheDocument();
    expect(screen.queryByText("Loading reason…")).not.toBeInTheDocument();
  });

  it("reuses a recent absence detail when the modal is reopened", async () => {
    const sessions: TeacherDashboardSession[] = [
      {
        ...baseSession,
        absent_students: [
          { wcode: "W260114", nickname: "Titan", student_name: null, absence_id: "ab-cached", created_at: "2026-06-20T01:00:00Z" },
        ],
      },
    ];
    mockApiJson.mockResolvedValue(absenceDetail({ id: "ab-cached", reason: "Medical appointment" }));

    const first = renderPanel(sessions);
    expect(await screen.findByText("Medical appointment")).toBeInTheDocument();
    first.unmount();
    renderPanel(sessions);
    expect(await screen.findByText("Medical appointment")).toBeInTheDocument();

    expect(mockApiJson).toHaveBeenCalledTimes(1);
  });

  it("shows a muted placeholder when no reason exists", async () => {
    const sessions: TeacherDashboardSession[] = [
      {
        ...baseSession,
        absent_students: [
          { wcode: "W260114", nickname: "Titan", student_name: null, absence_id: "ab-3", created_at: "2026-06-20T01:00:00Z" },
        ],
      },
    ];

    mockApiJson.mockResolvedValueOnce(absenceDetail({ id: "ab-3", reason: null }));

    renderPanel(sessions);

    expect(await screen.findByText("No reason provided")).toBeInTheDocument();
  });

  it("falls back quietly when the reason lookup fails", async () => {
    const sessions: TeacherDashboardSession[] = [
      {
        ...baseSession,
        absent_students: [
          { wcode: "W260114", nickname: "Titan", student_name: null, absence_id: "ab-4", created_at: "2026-06-20T01:00:00Z" },
        ],
      },
    ];

    mockApiJson.mockRejectedValueOnce(new Error("Network failed"));

    renderPanel(sessions);

    expect(await screen.findByText("Reason unavailable")).toBeInTheDocument();
  });

  it("closes from Escape and the mobile-sized close control", () => {
    const onClose = vi.fn();
    render(
      <MemoryRouter>
        <DayPanel date={new Date("2026-06-20T00:00:00Z")} sessions={[]} onClose={onClose} />
      </MemoryRouter>,
    );
    const close = screen.getByRole("button", { name: "Close panel" });
    expect(close.className).toContain("h-11");
    fireEvent.keyDown(document, { key: "Escape" });
    fireEvent.click(close);
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("shows a green indicator when a session has no absences and no sit-ins", () => {
    renderPanel([baseSession]);
    expect(screen.getByText("No absences — No sit-ins")).toBeInTheDocument();
  });

  it("does NOT show the green indicator when a session has absences", () => {
    renderPanel([{
      ...baseSession,
      absent_students: [
        { wcode: "W260114", nickname: "Titan", student_name: null, absence_id: "ab-1", created_at: "2026-06-20T01:00:00Z" },
      ],
    }]);
    mockApiJson.mockResolvedValue(absenceDetail({ id: "ab-1", reason: null }));
    expect(screen.queryByText("No absences — No sit-ins")).not.toBeInTheDocument();
  });

  it("does NOT show the green indicator when a session has sit-in visitors", () => {
    renderPanel([{
      ...baseSession,
      sit_in_visitors: [{
        wcode: "W260207",
        nickname: "Nut",
        student_name: null,
        from_course_code: "ENG201",
        from_subject_name: null,
        absence_id: "ab-2",
        session_start_at: baseSession.start_at,
        session_end_at: baseSession.end_at,
        absent_subject_name: null,
        absence_date: "2026-06-19",
      }],
    }]);
    mockApiJson.mockResolvedValue(absenceDetail({ id: "ab-2", reason: "Family trip" }));
    expect(screen.queryByText("No absences — No sit-ins")).not.toBeInTheDocument();
  });

  it("no longer renders View Course or Take Attendance buttons", () => {
    renderPanel([baseSession]);
    expect(screen.queryByText("View Course")).not.toBeInTheDocument();
    expect(screen.queryByText("Take Attendance")).not.toBeInTheDocument();
  });
});
