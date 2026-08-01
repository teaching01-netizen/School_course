import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import {
  issueMessage,
  issueConsequence,
  formatBangkokDateTime,
  formatBangkokDate,
  formatBangkokDateShort,
  formatBangkokTime,
  urgencyFor,
  notificationMessage,
} from "../format";
import { buildImpactIssue } from "../test/builders";

/* ------------------------------------------------------------------ */
/*  issueMessage                                                       */
/* ------------------------------------------------------------------ */

describe("issueMessage", () => {
  it("returns known message for regular_session_overlap", () => {
    const issue = buildImpactIssue({ issue_type: "regular_session_overlap" });
    expect(issueMessage(issue)).toBe("This sit-in overlaps with the student's regular class.");
  });

  it("returns known message for sit_in_overlap", () => {
    const issue = buildImpactIssue({ issue_type: "sit_in_overlap" });
    expect(issueMessage(issue)).toBe("This sit-in overlaps with another sit-in.");
  });

  it("returns known message for sit_in_ineligible", () => {
    const issue = buildImpactIssue({ issue_type: "sit_in_ineligible" });
    expect(issueMessage(issue)).toBe("The student is no longer eligible for this session.");
  });

  it("returns known message for past_time_change", () => {
    const issue = buildImpactIssue({ issue_type: "past_time_change" });
    expect(issueMessage(issue)).toBe("This session has already started or passed.");
  });

  it("returns known message for short_notice_change", () => {
    const issue = buildImpactIssue({ issue_type: "short_notice_change" });
    expect(issueMessage(issue)).toBe("The student has limited notice of this change.");
  });

  it("returns known message for sit_in_session_changed", () => {
    const issue = buildImpactIssue({ issue_type: "sit_in_session_changed" });
    expect(issueMessage(issue)).toBe("The assigned sit-in session was changed.");
  });

  it("returns known message for missed_session_changed", () => {
    const issue = buildImpactIssue({ issue_type: "missed_session_changed" });
    expect(issueMessage(issue)).toBe("The missed session was changed.");
  });

  it("returns safe fallback for unknown issue type", () => {
    const issue = buildImpactIssue({ issue_type: "unknown_type_xyz" });
    expect(issueMessage(issue)).toBe("This student arrangement needs attention.");
  });
});

/* ------------------------------------------------------------------ */
/*  issueConsequence                                                   */
/* ------------------------------------------------------------------ */

describe("issueConsequence", () => {
  it("returns short_notice consequence", () => {
    const issue = buildImpactIssue({ issue_type: "short_notice_change" });
    expect(issueConsequence(issue)).toBe("The student needs a clear update before the session begins.");
  });

  it("returns past_time consequence", () => {
    const issue = buildImpactIssue({ issue_type: "past_time_change" });
    expect(issueConsequence(issue)).toBe("The original arrangement can no longer be used.");
  });

  it("returns default consequence for other types", () => {
    const issue = buildImpactIssue({ issue_type: "regular_session_overlap" });
    expect(issueConsequence(issue)).toBe("The student cannot safely attend the current arrangement.");
  });
});

/* ------------------------------------------------------------------ */
/*  formatBangkokDateTime                                              */
/* ------------------------------------------------------------------ */

describe("formatBangkokDateTime", () => {
  it("formats start and end time range", () => {
    const result = formatBangkokDateTime("2026-07-31T06:00:00Z", "2026-07-31T07:00:00Z");
    // Should contain a dash between times
    expect(result).toMatch(/\d{2}:\d{2}–\d{2}:\d{2}/);
  });

  it("formats start time only when end is null", () => {
    const result = formatBangkokDateTime("2026-07-31T06:00:00Z", null);
    expect(result).toMatch(/\d{2}:\d{2}/);
    expect(result).not.toMatch(/–/);
  });

  it("returns 'Session unavailable' for null start", () => {
    expect(formatBangkokDateTime(null)).toBe("Session unavailable");
    expect(formatBangkokDateTime(null, null)).toBe("Session unavailable");
  });

  it("formats date with weekday", () => {
    const result = formatBangkokDateTime("2026-07-31T06:00:00Z");
    expect(result).toMatch(/Thu|Fri|Sat|Sun|Mon|Tue|Wed/i);
  });
});

/* ------------------------------------------------------------------ */
/*  formatBangkokDate                                                  */
/* ------------------------------------------------------------------ */

describe("formatBangkokDate", () => {
  it("returns empty string for null", () => {
    expect(formatBangkokDate(null)).toBe("");
  });

  it("formats full date", () => {
    const result = formatBangkokDate("2026-07-31T06:00:00Z");
    expect(result).toMatch(/July/);
    expect(result).toMatch(/31/);
  });
});

/* ------------------------------------------------------------------ */
/*  formatBangkokDateShort                                             */
/* ------------------------------------------------------------------ */

describe("formatBangkokDateShort", () => {
  it("returns empty string for null", () => {
    expect(formatBangkokDateShort(null)).toBe("");
  });

  it("formats short date", () => {
    const result = formatBangkokDateShort("2026-07-31T06:00:00Z");
    expect(result).toMatch(/Jul/);
    expect(result).toMatch(/31/);
  });
});

/* ------------------------------------------------------------------ */
/*  formatBangkokTime                                                  */
/* ------------------------------------------------------------------ */

describe("formatBangkokTime", () => {
  it("returns 'Not set' for null start", () => {
    expect(formatBangkokTime(null)).toBe("Not set");
  });

  it("formats start and end range", () => {
    const result = formatBangkokTime("2026-07-31T06:00:00Z", "2026-07-31T07:00:00Z");
    expect(result).toMatch(/\d{2}:\d{2}–\d{2}:\d{2}/);
  });

  it("formats start only when end is undefined", () => {
    const result = formatBangkokTime("2026-07-31T06:00:00Z");
    expect(result).toMatch(/\d{2}:\d{2}/);
    expect(result).not.toMatch(/–/);
  });
});

/* ------------------------------------------------------------------ */
/*  urgencyFor                                                         */
/* ------------------------------------------------------------------ */

describe("urgencyFor", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-31T04:00:00Z"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("shows 'Starts within an hour' when notice_hours < 1", () => {
    const issue = buildImpactIssue({ details: { notice_hours: 0.5 } });
    expect(urgencyFor(issue)).toBe("Starts within an hour");
  });

  it("shows hours when notice_hours < 24", () => {
    const issue = buildImpactIssue({ details: { notice_hours: 5 } });
    expect(urgencyFor(issue)).toBe("Starts in 5h");
  });

  it("shows days when notice_hours >= 24", () => {
    const issue = buildImpactIssue({ details: { notice_hours: 48 } });
    expect(urgencyFor(issue)).toBe("Starts in 2 days");
  });

  it("rounds hours up in days", () => {
    const issue = buildImpactIssue({ details: { notice_hours: 50 } });
    expect(urgencyFor(issue)).toBe("Starts in 3 days");
  });

  it("falls back to snapshot start_at when notice_hours is absent", () => {
    const issue = buildImpactIssue({
      details: {},
      assignment_context: {
        assigned_at: "2026-07-30T03:00:00Z",
        original_session: {
          quality: "exact",
          source: "snapshot",
          snapshot: {
            start_at: "2026-07-31T05:00:00Z",
            end_at: "2026-07-31T06:00:00Z",
          },
        },
        current_session: null,
      },
    });
    expect(urgencyFor(issue)).toBe("Starts in 1h");
  });

  it("falls back to change_context.before when snapshot is absent", () => {
    const issue = buildImpactIssue({
      details: {},
      assignment_context: {
        assigned_at: "2026-07-30T03:00:00Z",
        original_session: {
          quality: "unavailable",
          source: "none",
          snapshot: null,
        },
        current_session: null,
      },
      change_context: {
        change_id: "change-1",
        before: { start_at: "2026-07-31T05:00:00Z", end_at: "2026-07-31T06:00:00Z" },
        after: {},
      },
    });
    expect(urgencyFor(issue)).toBe("Starts in 1h");
  });

  it("returns 'Needs urgent review' for critical with no start time", () => {
    const issue = buildImpactIssue({
      details: {},
      severity: "critical",
      assignment_context: {
        assigned_at: null,
        original_session: { quality: "unavailable", source: "none", snapshot: null },
        current_session: null,
      },
      change_context: { change_id: "change-1", before: null, after: null },
    });
    expect(urgencyFor(issue)).toBe("Needs urgent review");
  });

  it("returns 'Review soon' for warning with no start time", () => {
    const issue = buildImpactIssue({
      details: {},
      severity: "warning",
      assignment_context: {
        assigned_at: null,
        original_session: { quality: "unavailable", source: "none", snapshot: null },
        current_session: null,
      },
      change_context: { change_id: "change-1", before: null, after: null },
    });
    expect(urgencyFor(issue)).toBe("Review soon");
  });
});

/* ------------------------------------------------------------------ */
/*  notificationMessage                                                */
/* ------------------------------------------------------------------ */

describe("notificationMessage", () => {
  it("returns queued message", () => {
    expect(notificationMessage("queued")).toBe("Student notification queued");
  });

  it("returns not_configured message", () => {
    expect(notificationMessage("not_configured")).toBe(
      "Assignment updated, but SMS and email templates are not configured.",
    );
  });

  it("returns no_recipient message", () => {
    expect(notificationMessage("no_recipient")).toBe(
      "Assignment updated, but no contact method is available for this student.",
    );
  });

  it("returns generic fallback for unknown status", () => {
    expect(notificationMessage("unknown")).toBe("Issue updated");
    expect(notificationMessage("not_required")).toBe("Issue updated");
  });
});
