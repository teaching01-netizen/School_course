import type { Page } from "@playwright/test";

export type QueueItem = {
  id: string;
  absence_id: string;
  issue_type: string;
  severity: "critical" | "warning";
  status: string;
  issue_version: number;
  wcode: string;
  student_name: string;
  start_at: string;
  end_at: string;
  details: { reasons: string[]; notice_hours: number };
  suggested_resolutions: unknown[];
  resolution_action: unknown;
  assignment_context: {
    assigned_at: string;
    original_session: {
      quality: string;
      source: string;
      snapshot: {
        start_at: string;
        end_at: string;
        room_name: string;
        teacher_name: string;
        course_code: string;
        course_name: string;
      };
    };
    current_session: {
      status: string;
      session_id: string;
      version: number;
      start_at: string;
      end_at: string;
      course_code: string;
      course_name: string;
      subject_name: string;
      room_name: string;
      teacher_name: string;
    };
  };
  change_context: {
    change_id: string;
    before: { start_at: string; end_at: string };
    after: { start_at: string; end_at: string };
  };
  impact_context: {
    issue_type: string;
    severity: string;
    reasons: { code: string; message: string }[];
  };
};

export type Candidate = {
  session_id: string;
  session_version: number;
  start_at: string;
  end_at: string;
  course_code: string;
  course_name: string;
  room_name: string;
  teacher_name: string;
  available_capacity: number;
  eligible: boolean;
  student_conflicts: boolean;
  generated_at: string;
};

export type ProcessingItem = {
  id: string;
  course_code: string;
  course_name: string;
  subject_name: string;
  created_at: string;
  status: "pending" | "processing" | "failed" | "delayed_by_batch";
  last_error: string | null;
};

export type HistoryItem = {
  id: string;
  new_course_code: string;
  new_course_name: string;
  new_course_subject: string;
  old_start_at: string;
  old_end_at: string;
  new_start_at: string;
  new_end_at: string;
  created_at: string;
  open_issue_count: number;
  critical_issue_count: number;
};

const defaultQueueItems: QueueItem[] = [
  {
    id: "issue-1",
    absence_id: "abs-1",
    issue_type: "regular_session_overlap",
    severity: "critical",
    status: "open",
    issue_version: 1,
    wcode: "W250001",
    student_name: "Alice Johnson",
    start_at: "2026-07-31T10:00:00Z",
    end_at: "2026-07-31T11:00:00Z",
    details: { reasons: ["Overlaps with regular class"], notice_hours: 2 },
    suggested_resolutions: [],
    resolution_action: null,
    assignment_context: {
      assigned_at: "2026-07-25T09:00:00Z",
      original_session: {
        quality: "exact",
        source: "database",
        snapshot: {
          start_at: "2026-07-31T10:00:00Z",
          end_at: "2026-07-31T11:00:00Z",
          room_name: "Room 4",
          teacher_name: "Mr Smith",
          course_code: "MATH101",
          course_name: "Mathematics",
        },
      },
      current_session: {
        status: "active",
        session_id: "session-1",
        version: 2,
        start_at: "2026-07-31T13:00:00Z",
        end_at: "2026-07-31T14:00:00Z",
        course_code: "MATH101",
        course_name: "Mathematics",
        subject_name: "Mathematics",
        room_name: "Room 7",
        teacher_name: "Mr Smith",
      },
    },
    change_context: {
      change_id: "change-1",
      before: { start_at: "2026-07-31T10:00:00Z", end_at: "2026-07-31T11:00:00Z" },
      after: { start_at: "2026-07-31T13:00:00Z", end_at: "2026-07-31T14:00:00Z" },
    },
    impact_context: {
      issue_type: "regular_session_overlap",
      severity: "critical",
      reasons: [
        {
          code: "overlap",
          message:
            "This sit-in overlaps with the student's regular class.",
        },
      ],
    },
  },
  {
    id: "issue-2",
    absence_id: "abs-2",
    issue_type: "short_notice_change",
    severity: "warning",
    status: "needs_review",
    issue_version: 1,
    wcode: "W250002",
    student_name: "Bob Wilson",
    start_at: "2026-08-01T09:00:00Z",
    end_at: "2026-08-01T10:00:00Z",
    details: { reasons: ["Teacher has a conflicting meeting"], notice_hours: 4 },
    suggested_resolutions: [],
    resolution_action: null,
    assignment_context: {
      assigned_at: "2026-07-26T10:00:00Z",
      original_session: {
        quality: "exact",
        source: "database",
        snapshot: {
          start_at: "2026-08-01T09:00:00Z",
          end_at: "2026-08-01T10:00:00Z",
          room_name: "Room 1",
          teacher_name: "Ms Green",
          course_code: "ENG201",
          course_name: "English",
        },
      },
      current_session: {
        status: "active",
        session_id: "session-2",
        version: 1,
        start_at: "2026-08-01T09:00:00Z",
        end_at: "2026-08-01T10:00:00Z",
        course_code: "ENG201",
        course_name: "English",
        subject_name: "English",
        room_name: "Room 1",
        teacher_name: "Ms Green",
      },
    },
    change_context: {
      change_id: "change-2",
      before: { start_at: "2026-08-01T09:00:00Z", end_at: "2026-08-01T10:00:00Z" },
      after: { start_at: "2026-08-01T09:00:00Z", end_at: "2026-08-01T10:00:00Z" },
    },
    impact_context: {
      issue_type: "short_notice_change",
      severity: "warning",
      reasons: [
        {
          code: "teacher_conflict",
          message: "The student has limited notice of this change.",
        },
      ],
    },
  },
  {
    id: "issue-3",
    absence_id: "abs-3",
    issue_type: "sit_in_session_changed",
    severity: "critical",
    status: "open",
    issue_version: 1,
    wcode: "W250003",
    student_name: "Carol Davis",
    start_at: "2026-08-02T14:00:00Z",
    end_at: "2026-08-02T15:00:00Z",
    details: { reasons: ["Room already booked"], notice_hours: 1 },
    suggested_resolutions: [],
    resolution_action: null,
    assignment_context: {
      assigned_at: "2026-07-27T11:00:00Z",
      original_session: {
        quality: "exact",
        source: "database",
        snapshot: {
          start_at: "2026-08-02T14:00:00Z",
          end_at: "2026-08-02T15:00:00Z",
          room_name: "Room 9",
          teacher_name: "Dr Brown",
          course_code: "SCI301",
          course_name: "Science",
        },
      },
      current_session: {
        status: "active",
        session_id: "session-3",
        version: 1,
        start_at: "2026-08-02T14:00:00Z",
        end_at: "2026-08-02T15:00:00Z",
        course_code: "SCI301",
        course_name: "Science",
        subject_name: "Science",
        room_name: "Room 9",
        teacher_name: "Dr Brown",
      },
    },
    change_context: {
      change_id: "change-3",
      before: { start_at: "2026-08-02T14:00:00Z", end_at: "2026-08-02T15:00:00Z" },
      after: { start_at: "2026-08-02T14:00:00Z", end_at: "2026-08-02T15:00:00Z" },
    },
    impact_context: {
      issue_type: "sit_in_session_changed",
      severity: "critical",
      reasons: [
        {
          code: "room_conflict",
          message: "The assigned sit-in session was changed.",
        },
      ],
    },
  },
];

const defaultCandidates: Candidate[] = [
  {
    session_id: "session-a",
    session_version: 1,
    start_at: "2026-08-01T10:00:00Z",
    end_at: "2026-08-01T11:00:00Z",
    course_code: "MATH101",
    course_name: "Mathematics",
    room_name: "Room 5",
    teacher_name: "Dr Jones",
    available_capacity: 4,
    eligible: true,
    student_conflicts: false,
    generated_at: "2026-07-31T09:00:00Z",
  },
  {
    session_id: "session-b",
    session_version: 1,
    start_at: "2026-08-01T13:00:00Z",
    end_at: "2026-08-01T14:00:00Z",
    course_code: "ENG201",
    course_name: "English",
    room_name: "Room 2",
    teacher_name: "Ms Green",
    available_capacity: 2,
    eligible: false,
    student_conflicts: true,
    generated_at: "2026-07-31T09:00:00Z",
  },
];

const defaultProcessing: ProcessingItem[] = [
  {
    id: "change-4",
    course_code: "CHEM101",
    course_name: "Chemistry",
    subject_name: "Chemistry",
    created_at: "2026-07-30T08:00:00Z",
    status: "processing",
    last_error: null,
  },
  {
    id: "change-5",
    course_code: "PHYS201",
    course_name: "Physics",
    subject_name: "Physics",
    created_at: "2026-07-29T14:00:00Z",
    status: "failed",
    last_error: "Network timeout",
  },
];

const defaultHistory: HistoryItem[] = [
  {
    id: "change-6",
    new_course_code: "HIST101",
    new_course_name: "History",
    new_course_subject: "History",
    old_start_at: "2026-07-20T09:00:00Z",
    old_end_at: "2026-07-20T10:00:00Z",
    new_start_at: "2026-07-20T11:00:00Z",
    new_end_at: "2026-07-20T12:00:00Z",
    created_at: "2026-07-20T10:30:00Z",
    open_issue_count: 0,
    critical_issue_count: 0,
  },
  {
    id: "change-7",
    new_course_code: "ART201",
    new_course_name: "Art",
    new_course_subject: "Art",
    old_start_at: "2026-07-21T14:00:00Z",
    old_end_at: "2026-07-21T15:00:00Z",
    new_start_at: "2026-07-21T14:00:00Z",
    new_end_at: "2026-07-21T15:00:00Z",
    created_at: "2026-07-21T15:00:00Z",
    open_issue_count: 1,
    critical_issue_count: 0,
  },
];

export type ScheduleImpactRoutesOptions = {
  queueItems?: QueueItem[];
  queueSummary?: {
    need_attention: number;
    critical: number;
    warnings: number;
    notification_failures: number;
    notifications_configured: boolean;
  };
  processingItems?: ProcessingItem[];
  historyItems?: HistoryItem[];
  candidates?: Candidate[];
  queueTotal?: number;
  queueOffset?: number;
  queueLimit?: number;
};

function filterQueueItems(
  items: QueueItem[],
  params: URLSearchParams,
): QueueItem[] {
  let result = items;
  const q = params.get("q");
  if (q) {
    const lower = q.toLowerCase();
    result = result.filter(
      (item) =>
        item.student_name.toLowerCase().includes(lower) ||
        item.wcode.toLowerCase().includes(lower) ||
        item.assignment_context.original_session.snapshot.course_code.toLowerCase().includes(lower) ||
        item.assignment_context.original_session.snapshot.course_name.toLowerCase().includes(lower),
    );
  }
  const severity = params.get("severity");
  if (severity) {
    result = result.filter((item) => item.severity === severity);
  }
  const status = params.get("status");
  if (status && status !== "all") {
    result = result.filter((item) => item.status === status);
  }
  return result;
}

export async function installScheduleImpactRoutes(
  page: Page,
  options: ScheduleImpactRoutesOptions = {},
) {
  const {
    queueItems = defaultQueueItems,
    queueSummary = {
      need_attention: 11,
      critical: 3,
      warnings: 6,
      notification_failures: 2,
      notifications_configured: true,
    },
    processingItems = defaultProcessing,
    historyItems = defaultHistory,
    candidates = defaultCandidates,
    queueTotal: initialTotal,
    queueOffset = 0,
    queueLimit = 25,
  } = options;

  await page.route("**/api/v1/me", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ id: "user-1", username: "admin", role: "Admin" }),
    }),
  );

  await page.route("**/api/v1/absences/stats", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ pending_count: 4 }),
    }),
  );

  await page.route("**/api/v1/operations/schedule-impact/processing", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ items: processingItems }),
    }),
  );

  await page.route("**/api/v1/operations/session-changes**", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ items: historyItems }),
    }),
  );

  await page.route("**/api/v1/operations/schedule-issues/*/candidates", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ items: candidates }),
    }),
  );

  await page.route("**/api/v1/operations/schedule-issues/*/activity", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        items: [
          {
            activity_id: "act-1",
            timestamp: "2026-07-31T09:00:00Z",
            actor: "System",
            action: "issue_detected",
            detail: "Schedule issue detected automatically.",
          },
        ],
      }),
    }),
  );

  await page.route("**/api/v1/operations/schedule-issues/*/resolve", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        id: "issue-1",
        status: "resolved",
        action: "reassign",
        notification_status: "queued",
      }),
    }),
  );

  await page.route("**/api/v1/operations/schedule-impact**", (route) => {
    const url = new URL(route.request().url());
    // Only handle the queue endpoint — let /processing and other sub-paths pass through
    if (url.pathname !== "/api/v1/operations/schedule-impact") {
      route.fallback();
      return;
    }
    const offset = Number(url.searchParams.get("offset")) || 0;
    const limit = Number(url.searchParams.get("limit")) || 25;

    const filtered = filterQueueItems(queueItems, url.searchParams);
    const total = initialTotal ?? filtered.length;
    const pageItems = filtered.slice(offset, offset + limit);

    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        items: pageItems,
        summary: queueSummary,
        pagination: {
          limit,
          offset,
          total,
          has_more: offset + limit < total,
          next_offset: offset + limit < total ? offset + limit : null,
        },
        limit,
        offset,
      }),
    });
  });
}
