import type {
  ParentVerificationResponse,
  PublicStudentLookupResponse,
  SessionsInRangeResponse,
  StudentLookupResponse,
  VerifiedStudentProfile,
} from "@/types";

/** Local-time date key N days from today, e.g. relativeDateKey(0) -> "2026-09-03". */
export function relativeDateKey(dayOffset: number): string {
  const d = new Date();
  d.setDate(d.getDate() + dayOffset);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

/** ISO timestamp for a local time on a day N days from today. */
export function relativeISO(hour: number, minute: number, dayOffset: number): string {
  const d = new Date();
  d.setDate(d.getDate() + dayOffset);
  d.setHours(hour, minute, 0, 0);
  return d.toISOString();
}

export const PUBLIC_FORM_CONFIG = {
  form: {
    max_date_range_days: 30,
    min_hours_before_session: 0,
    max_hours_after_session: 0,
    require_reason: true,
    reason_categories: [],
    allow_free_text_reason: true,
    intro_text: "",
    confirmation_text: "",
  },
  sit_in: {
    auto_resolve_enabled: true,
    zoom_description: "Zoom session.",
    max_sessions_per_absence: 10,
  },
  notifications: {
    sms_parent_enabled: true,
    sms_parent_template: "template",
    sms_success_template: "success template",
  },
  admin_contact: {
    email: "office@example.edu",
    phone: "+66 2123 4567",
    hours: "Mon-Fri 08:00-16:00",
  },
};

export const MANUAL_EMAIL_STUDENT: StudentLookupResponse = {
  student_id: "student-a",
  wcode: "W250389",
  full_name: "Alex Student",
  parent_phone: "+66812345678",
  email_crm: null,
  email_system: null,
  subjects: [{ id: "subject-math", code: "MATH", name: "Mathematics" }],
};

export const CRM_EMAIL_STUDENT: StudentLookupResponse = {
  ...MANUAL_EMAIL_STUDENT,
  email_crm: "alex.crm@example.edu",
  email_system: "alex.system@example.edu",
};

export const SYSTEM_EMAIL_STUDENT: StudentLookupResponse = {
  ...MANUAL_EMAIL_STUDENT,
  email_system: "alex.system@example.edu",
};

export const SECOND_STUDENT: StudentLookupResponse = {
  student_id: "student-b",
  wcode: "W250400",
  full_name: "Bailey Student",
  parent_phone: "+66899999999",
  email_crm: "bailey@example.edu",
  email_system: null,

  subjects: [{ id: "subject-physics", code: "PHYS", name: "Physics" }],
};
export function publicStudentLookup(
  student: StudentLookupResponse,
  lookupToken = `lookup-${student.wcode}`,
): PublicStudentLookupResponse {
  const hintSource = student.nickname?.trim() || student.full_name?.trim() || "";
  const phoneDigits = student.parent_phone?.replace(/\D/g, "") ?? "";
  return {
    wcode: student.wcode,
    lookup_token: lookupToken,
    email_input_required: !Boolean(
      student.email_crm?.trim() || student.email_system?.trim() || student.email?.trim(),
    ),
    parent_verification_available: Boolean(student.parent_phone?.trim()),
    ...(hintSource ? { nickname_hint: `${Array.from(hintSource)[0]}***` } : {}),
    ...(phoneDigits.length >= 4 ? { parent_phone_hint: `••••${phoneDigits.slice(-4)}` } : {}),
  };
}

export function verifiedStudentProfile(
  student: StudentLookupResponse,
): VerifiedStudentProfile {
  return {
    wcode: student.wcode,
    display_name: student.display_name?.trim() || student.full_name,
    email_on_file: Boolean(
      student.email_crm?.trim() || student.email_system?.trim() || student.email?.trim(),
    ),
    subjects: student.subjects.map(({ id, code, name }) => ({ id, code, name })),
  };
}

export const PUBLIC_FORM_SESSIONS: SessionsInRangeResponse = {
  subjects: [
    {
      subject_id: "subject-math",
      subject_code: "MATH",
      subject_name: "Mathematics",
      course_id: "course-math",
      course_code: "MATH201",
      course_name: "Mathematics",
      teacher_name: "Ms. Jane",
      sessions: [
        {
          id: "session-math-1",
          start_at: relativeISO(17, 0, 0),
          end_at: relativeISO(19, 0, 0),
          date: relativeDateKey(0),
          already_absent: false,
        },
      ],
    },
    {
      subject_id: "subject-physics",
      subject_code: "PHYS",
      subject_name: "Physics",
      course_id: "course-physics",
      course_code: "PHYS201",
      course_name: "Physics",
      teacher_name: "Mr. Long",
      sessions: [
        {
          id: "session-physics-1",
          start_at: relativeISO(17, 0, 1),
          end_at: relativeISO(19, 0, 1),
          date: relativeDateKey(1),
          already_absent: false,
        },
      ],
    },
  ],
};

export function parentVerification(
  status: ParentVerificationResponse["status"],
  wcode = MANUAL_EMAIL_STUDENT.wcode,
): ParentVerificationResponse {
  return {
    token: "opaque-verification-token",
    status,
    wcode,
    parent_phone: "+66812345678",
    expires_at: new Date(Date.now() + 60 * 60_000).toISOString(),
    otp_code_expires_at: new Date(Date.now() + 10 * 60_000).toISOString(),
  };
}

export function sessionsWithAlreadyAbsent(): SessionsInRangeResponse {
  return {
    subjects: [{
      ...PUBLIC_FORM_SESSIONS.subjects[0],
      sessions: [{
        ...PUBLIC_FORM_SESSIONS.subjects[0].sessions[0],
        already_absent: true,
      }],
    }],
  };
}
