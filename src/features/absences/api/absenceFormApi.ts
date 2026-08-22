import { apiJson } from "@/api/client";
import type {
  AbsenceFormConfig,
  ManagedAbsence,
  PublicStudentLookupResponse,
  SessionsInRangeResponse,
  StudentLookupResponse,
  VerifiedStudentProfile,
} from "../types";
import {
  DEFAULT_ADMIN_CONTACT,
  DEFAULT_CONFIG,
  DEFAULT_NOTIFICATIONS,
} from "../constants";
import type { AbsenceBatchCreateItem } from "../domain/submissionPayload";

export type AbsenceBatchCreateResponse = {
  items: ManagedAbsence[];
};

export type StaffSessionsInRangeOptions = {
  courseIds?: string[];
  subjectIds?: string[];
  includeAllSubjects?: boolean;
  bypassTiming?: boolean;
  satVerbalAfterPriority?: number;
};

export type StudentSessionsOptions = {
  courseIds?: string[];
  subjectIds?: string[];
  satVerbalAfterPriority?: number;
};

export function sessionsInRangePath(
  wcode: string,
  dateFrom?: string,
  dateTo?: string,
  options?: StaffSessionsInRangeOptions,
): string {
  const params = new URLSearchParams({ wcode });
  if (dateFrom) params.set("date_from", dateFrom);
  if (dateTo) params.set("date_to", dateTo);
  if (options?.courseIds && options.courseIds.length > 0) {
    params.set("course_ids", options.courseIds.join(","));
  }
  if (options?.subjectIds && options.subjectIds.length > 0) {
    params.set("subject_ids", options.subjectIds.join(","));
  }
  if (options?.bypassTiming) {
    params.set("bypass_timing", "true");
  }
  if (options?.includeAllSubjects) {
    params.set("include_all_subjects", "true");
  }
  if (options?.satVerbalAfterPriority !== undefined) {
    params.set(
      "sat_verbal_after_priority",
      String(options.satVerbalAfterPriority),
    );
  }
  return `/api/v1/absences/sessions-in-range?${params.toString()}`;
}

export function studentSessionsPath(
  dateFrom?: string,
  dateTo?: string,
  options?: StudentSessionsOptions,
): string {
  const params = new URLSearchParams();
  if (dateFrom) params.set("date_from", dateFrom);
  if (dateTo) params.set("date_to", dateTo);
  if (options?.courseIds && options.courseIds.length > 0) {
    params.set("course_ids", options.courseIds.join(","));
  }
  if (options?.subjectIds && options.subjectIds.length > 0) {
    params.set("subject_ids", options.subjectIds.join(","));
  }
  if (options?.satVerbalAfterPriority !== undefined) {
    params.set("sat_verbal_after_priority", String(options.satVerbalAfterPriority));
  }
  const query = params.toString();
  return query ? `/api/v1/absence-self-service/sessions?${query}` : "/api/v1/absence-self-service/sessions";
}

export function normalizeAbsenceFormConfig(
  data: AbsenceFormConfig,
): AbsenceFormConfig {
  const notifications = {
    sms_parent_enabled:
      data.notifications?.sms_parent_enabled ??
      DEFAULT_NOTIFICATIONS.sms_parent_enabled,
    sms_parent_template:
      data.notifications?.sms_parent_template ??
      DEFAULT_NOTIFICATIONS.sms_parent_template,
    sms_success_template:
      data.notifications?.sms_success_template ??
      DEFAULT_NOTIFICATIONS.sms_success_template,
    sms_special_approved_template:
      data.notifications?.sms_special_approved_template ??
      DEFAULT_NOTIFICATIONS.sms_special_approved_template,
    email_success_enabled:
      data.notifications?.email_success_enabled ??
      DEFAULT_NOTIFICATIONS.email_success_enabled,
    email_success_subject:
      data.notifications?.email_success_subject ??
      DEFAULT_NOTIFICATIONS.email_success_subject,
    email_success_body:
      data.notifications?.email_success_body ??
      DEFAULT_NOTIFICATIONS.email_success_body,
  };
  const adminContact = {
    email: data.admin_contact?.email ?? DEFAULT_ADMIN_CONTACT.email,
    phone: data.admin_contact?.phone ?? DEFAULT_ADMIN_CONTACT.phone,
    hours: data.admin_contact?.hours ?? DEFAULT_ADMIN_CONTACT.hours,
  };
  return {
    ...DEFAULT_CONFIG,
    ...data,
    form: { ...DEFAULT_CONFIG.form, ...data.form },
    sit_in: { ...DEFAULT_CONFIG.sit_in, ...data.sit_in },
    notifications,
    admin_contact: adminContact,
  };
}

export async function loadAbsenceFormConfig(): Promise<AbsenceFormConfig> {
  const data = await apiJson<AbsenceFormConfig>("/api/v1/absence-form-config", {
    method: "GET",
  });
  return normalizeAbsenceFormConfig(data);
}

export function lookupStudentByWcode(wcode: string): Promise<PublicStudentLookupResponse> {
  return apiJson<PublicStudentLookupResponse>(
    "/api/v1/absence-self-service/lookup",
    { method: "POST", body: JSON.stringify({ wcode }) },
  );
}

export function loadStudentProfile(): Promise<VerifiedStudentProfile> {
  return apiJson<VerifiedStudentProfile>("/api/v1/absence-self-service/me", {
    method: "GET",
  });
}

export function lookupStaffStudentByWcode(wcode: string): Promise<StudentLookupResponse> {
  return apiJson<StudentLookupResponse>(
    `/api/v1/admin/absences/student-lookup?wcode=${encodeURIComponent(wcode)}`,
    { method: "GET" },
  );
}

export function loadStudentSessions(
  dateFrom?: string,
  dateTo?: string,
  init?: Pick<RequestInit, "signal">,
  options?: StudentSessionsOptions,
): Promise<SessionsInRangeResponse> {
  return apiJson<SessionsInRangeResponse>(
    studentSessionsPath(dateFrom, dateTo, options),
    { method: "GET", ...init },
  );
}

export function loadSessionsInRange(
  wcode: string,
  dateFrom?: string,
  dateTo?: string,
  init?: Pick<RequestInit, "signal">,
  options?: StaffSessionsInRangeOptions,
): Promise<SessionsInRangeResponse> {
  return apiJson<SessionsInRangeResponse>(
    sessionsInRangePath(wcode, dateFrom, dateTo, options),
    { method: "GET", ...init },
  );
}

export async function submitAbsenceBatch(input: {
  idempotencyKey: string;
  email?: string;
  nickname?: string;
  reason: string;
  items: AbsenceBatchCreateItem[];
}): Promise<AbsenceBatchCreateResponse> {
  const request: RequestInit = {
    method: "POST",
    headers: { "Idempotency-Key": input.idempotencyKey },
    body: JSON.stringify({
      email: input.email,
      nickname: input.nickname,
      reason: input.reason,
      items: input.items,
    }),
  };

  try {
    return await apiJson<AbsenceBatchCreateResponse>(
      "/api/v1/absences/batch",
      request,
    );
  } catch (error) {
    if (!(error instanceof TypeError)) throw error;
    return apiJson<AbsenceBatchCreateResponse>(
      "/api/v1/absences/batch",
      request,
    );
  }
}
