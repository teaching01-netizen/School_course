import { vi } from "vitest";
import { render, type RenderOptions } from "@testing-library/react";
import { ToastProvider } from "../../../hooks/useToast";
import type { SessionsInRangeResponse, SubjectSessions } from "../../../types";

export { render };

export function renderWithProviders(ui: React.ReactElement, options?: RenderOptions) {
  return render(<ToastProvider>{ui}</ToastProvider>, options);
}

/**
 * Mock apiJson to return different responses based on URL pattern matching.
 * @param mockApiJson - The vi.fn() mock for apiJson
 * @param routes - Map of URL pattern → response data. First matching pattern wins.
 * @example
 *   mockApiByPattern(mockApiJson, {
 *     "absence-form-config": MOCK_CONFIG,
 *     "student-lookup": MOCK_STUDENT,
 *     "sessions-in-range": MOCK_SESSIONS,
 *   });
 */
export function mockApiByPattern(
  mockApiJson: ReturnType<typeof vi.fn>,
  routes: Record<string, unknown>,
) {
  mockApiJson.mockImplementation(async (url: string, _init?: RequestInit) => {
    for (const [pattern, data] of Object.entries(routes)) {
      if (String(url).includes(pattern)) return data;
    }
    throw new Error(`Unmocked API call: ${url}`);
  });
}

// === Fixtures ===

export function createMockSessionsInRange(subjects?: SubjectSessions[]): SessionsInRangeResponse {
  return {
    subjects: subjects ?? [
      {
        subject_id: "subj-1",
        subject_code: "MATH",
        subject_name: "Mathematics",
        course_id: "c-math201",
        course_code: "MATH201",
        sessions: [
          { id: "s1", start_at: "2026-06-01T09:00:00Z", end_at: "2026-06-01T10:30:00Z", date: "2026-06-01", already_absent: false },
          { id: "s2", start_at: "2026-06-03T09:00:00Z", end_at: "2026-06-03T10:30:00Z", date: "2026-06-03", already_absent: false },
        ],
      },
      {
        subject_id: "subj-2",
        subject_code: "PHYS",
        subject_name: "Physics",
        course_id: "c-phys301",
        course_code: "PHYS301",
        sessions: [
          { id: "s3", start_at: "2026-06-02T11:00:00Z", end_at: "2026-06-02T12:30:00Z", date: "2026-06-02", already_absent: false },
        ],
      },
    ],
  };
}

export function createMockSitInResult(method: "zoom" | "physical" | "pending" = "zoom"): Record<string, unknown> {
  if (method === "zoom") {
    return { sit_in_method: "zoom" as const, missed_count: 2 };
  }
  if (method === "pending") {
    return { sit_in_method: "pending" as const, missed_count: 0 };
  }
  return {
    sit_in_method: "physical" as const,
    sit_in_course: { id: "c-sit", code: "MATH-301", name: "Calculus III" },
    missed_count: 2,
    missed_sessions: [
      { id: "ms1", start_at: "2026-06-01T09:00:00Z", end_at: "2026-06-01T10:30:00Z" },
      { id: "ms2", start_at: "2026-06-03T09:00:00Z", end_at: "2026-06-03T10:30:00Z" },
    ],
    available_sessions: [
      { id: "as1", start_at: "2026-06-01T11:00:00Z", end_at: "2026-06-01T12:30:00Z" },
      { id: "as2", start_at: "2026-06-03T11:00:00Z", end_at: "2026-06-03T12:30:00Z" },
    ],
    pre_selected: [
      { id: "as1", start_at: "2026-06-01T11:00:00Z", end_at: "2026-06-01T12:30:00Z" },
    ],
  };
}
