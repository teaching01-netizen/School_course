import { apiJson } from "@/api/client";
import type {
  AbsenceFormConfig,
  ManagedAbsence,
  SessionsInRangeResponse,
  StudentLookupResponse,
} from "../types";
import { DEFAULT_ADMIN_CONTACT, DEFAULT_CONFIG, DEFAULT_NOTIFICATIONS } from "../constants";
import type { AbsenceBatchCreateItem } from "../domain/submissionPayload";

export type AbsenceBatchCreateResponse = {
  items: ManagedAbsence[];
};

export type SessionsInRangeOptions = {
  courseIds?: string[];
  satVerbalAfterPriority?: number;
  bypassTiming?: boolean;
};

export function sessionsInRangePath(
  wcode: string,
  dateFrom?: string,
  dateTo?: string,
  options?: SessionsInRangeOptions,
): string {
  const params = new URLSearchParams({ wcode });
  if (dateFrom) params.set("date_from", dateFrom);
  if (dateTo) params.set("date_to", dateTo);
  if (options?.courseIds && options.courseIds.length > 0) {
    params.set("course_ids", options.courseIds.join(","));
  }
  if (options?.satVerbalAfterPriority !== undefined) {
    params.set("sat_verbal_after_priority", String(options.satVerbalAfterPriority));
  }
  if (options?.bypassTiming) {
    params.set("bypass_timing", "true");
  }
  return `/api/v1/absences/sessions-in-range?${params.toString()}`;
}

export function normalizeAbsenceFormConfig(data: AbsenceFormConfig): AbsenceFormConfig {
  const notifications = {
    sms_parent_enabled: data.notifications?.sms_parent_enabled ?? DEFAULT_NOTIFICATIONS.sms_parent_enabled,
    sms_parent_template: data.notifications?.sms_parent_template ?? DEFAULT_NOTIFICATIONS.sms_parent_template,
    sms_success_template: data.notifications?.sms_success_template ?? DEFAULT_NOTIFICATIONS.sms_success_template,
    allow_submit_without_otp: data.notifications?.allow_submit_without_otp ?? DEFAULT_NOTIFICATIONS.allow_submit_without_otp,
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
  const data = await apiJson<AbsenceFormConfig>("/api/v1/absence-form-config", { method: "GET" });
  return normalizeAbsenceFormConfig(data);
}

export function lookupStudentByWcode(wcode: string): Promise<StudentLookupResponse> {
  return apiJson<StudentLookupResponse>(
    `/api/v1/absences/student-lookup?wcode=${encodeURIComponent(wcode)}`,
    { method: "GET" },
  );
}

export function loadSessionsInRange(
  wcode: string,
  dateFrom?: string,
  dateTo?: string,
  init?: Pick<RequestInit, "signal">,
  options?: SessionsInRangeOptions,
): Promise<SessionsInRangeResponse> {
  return apiJson<SessionsInRangeResponse>(
    sessionsInRangePath(wcode, dateFrom, dateTo, options),
    { method: "GET", ...init },
  );
}

export function submitAbsenceBatch(input: {
  idempotencyKey: string;
  wcode: string;
  email?: string;
  reason: string;
  verificationToken?: string;
  items: AbsenceBatchCreateItem[];
}): Promise<AbsenceBatchCreateResponse> {
  return apiJson<AbsenceBatchCreateResponse>("/api/v1/absences/batch", {
    method: "POST",
    headers: { "Idempotency-Key": input.idempotencyKey },
    body: JSON.stringify({
      wcode: input.wcode,
      email: input.email,
      reason: input.reason,
      verification_token: input.verificationToken,
      items: input.items,
    }),
  });
}
