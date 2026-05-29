export type ApiError = {
  code: string;
  message: string;
  details?: unknown;
};

/**
 * Upload a file using multipart/form-data (for XLSX upload).
 */
export async function apiUpload<T>(path: string, file: File): Promise<T> {
  const form = new FormData();
  form.append("file", file);
  const res = await fetch(path, {
    method: "POST",
    credentials: "include",
    body: form,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: { message: res.statusText } }));
    throw new Error(err?.error?.message ?? res.statusText);
  }
  return res.json();
}

export class ApiRequestError extends Error {
  code?: string;
  status?: number;
  details?: unknown;
  constructor(message: string, opts?: { code?: string; status?: number }) {
    super(message);
    this.name = "ApiRequestError";
    this.code = opts?.code;
    this.status = opts?.status;
  }
}

async function readApiError(res: Response): Promise<ApiRequestError> {
  try {
    const body = (await res.json()) as Partial<ApiError>;
    const msg = body && typeof body.message === "string" && body.message ? body.message : res.statusText || "Request failed";
    const code = body && typeof body.code === "string" ? body.code : undefined;
    const err = new ApiRequestError(msg, { code, status: res.status });
    if (body && "details" in body) err.details = body.details;
    return err;
  } catch {
    // ignore
  }
  return new ApiRequestError(res.statusText || "Request failed", { status: res.status });
}

export async function findAvailableSlots(params: {
  student_id: string;
  course_id: string;
  start_date: string;
  end_date: string;
  slot_duration_minutes?: number;
  day_start_hour?: number;
  day_end_hour?: number;
}): Promise<{ slots: SlotFinderSlot[] }> {
  return apiJson<{ slots: SlotFinderSlot[] }>("/api/v1/scheduling/find-slots", {
    method: "POST",
    body: JSON.stringify(params),
  });
}

export type SlotFinderSlot = {
  date: string;
  start_time: string;
  end_time: string;
  status: "provisional" | "blocked";
  kind?: string;
  message?: string;
  conflicts?: Array<{
    session_id: string;
    series_id?: string | null;
    course_id: string;
    room_id: string | null;
    teacher_id: string;
    start_at: string;
    end_at: string;
  }>;
};

/**
 * Generate a new idempotency key (UUID v4) for mutation requests.
 */
export function newIdempotencyKey(): string {
  // crypto.randomUUID() is available in modern browsers and Node 19+.
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  // Fallback: Math.random-based UUID v4 (not cryptographically strong, but sufficient for dedup).
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    const v = c === "x" ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

/**
 * Check if the given path is exempt from requiring an Idempotency-Key header.
 */
export function isIdempotencyExempt(method: string, path: string): boolean {
  // Auth endpoints (login/logout) and preflight/find-slots are exempt.
  if (method === "GET") return true;
  if (path === "/api/v1/login" || path === "/api/v1/logout") return true;
  if (path.startsWith("/api/v1/scheduling/preflight") || path === "/api/v1/scheduling/find-slots") return true;
  if (path === "/api/v1/absences" || path === "/api/v1/absences/batch-status") return true;
  // POST /api/v1/scheduling/apply is mutative (but it's an actual modification endpoint)
  // — it should have an idempotency key. Only exempt preflight.
  return false;
}

/**
 * Wrap an apiJson call with automatic Idempotency-Key for mutating requests.
 * For mutation requests (POST/PUT/PATCH/DELETE) that are not exempt, a
 * stable key is generated and sent as an Idempotency-Key header.
 */
export async function apiJson<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method ?? "GET").toUpperCase();

  // Build headers, adding Idempotency-Key for mutating requests that require it.
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init?.headers as Record<string, string> ?? {}),
  };

  if (!isIdempotencyExempt(method, path)) {
    if (!headers["Idempotency-Key"]) {
      headers["Idempotency-Key"] = newIdempotencyKey();
    }
  }

  const res = await fetch(path, {
    ...init,
    credentials: "include",
    headers,
  });

  if (!res.ok) {
    throw await readApiError(res);
  }
  return (await res.json()) as T;
}

export async function downloadApiFile(path: string): Promise<void> {
  const res = await fetch(path, { method: "GET", credentials: "include" });
  if (!res.ok) {
    throw await readApiError(res);
  }
  const blob = await res.blob();
  const disposition = res.headers.get("Content-Disposition") ?? "";
  const filenameMatch = disposition.match(/filename="?([^"]+)"?/);
  const filename = filenameMatch?.[1] ?? "absence-report.csv";
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}
