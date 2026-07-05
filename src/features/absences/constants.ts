import type {
  AbsenceFormConfig,
  AbsenceNotificationsSettings,
  AdminContactSettings,
} from "./types";

export const LEGACY_SESSION_STORAGE_KEY = "warwick-absence-form-state-v3";
export const LEGACY_VERIFICATION_STORAGE_KEY = `${LEGACY_SESSION_STORAGE_KEY}:parent-verification`;
export const STUDENT_RESUME_STORAGE_KEY = "warwick-absence-form-student-v1";
export const VERIFICATION_STORAGE_KEY = "warwick-absence-parent-verification-v1";

export const DEFAULT_NOTIFICATIONS: AbsenceNotificationsSettings = {
  sms_parent_enabled: true,
  sms_parent_template: "",
  sms_success_template: "",
  sms_special_approved_template: "",
  allow_submit_without_otp: false,
};

export const DEFAULT_ADMIN_CONTACT: AdminContactSettings = {
  email: "",
  phone: "",
  hours: "",
};

export const DEFAULT_CONFIG: AbsenceFormConfig = {
  form: {
    max_date_range_days: 30,
    min_hours_before_session: 0,
    max_hours_after_session: 0,
    require_reason: false,
    reason_categories: [],
    allow_free_text_reason: true,
    intro_text: "",
    confirmation_text: "",
  },
  sit_in: {
    auto_resolve_enabled: true,
    zoom_description: "Zoom session - no physical class attendance required.",
    max_sessions_per_absence: 10,
  },
  notifications: DEFAULT_NOTIFICATIONS,
  admin_contact: DEFAULT_ADMIN_CONTACT,
};
