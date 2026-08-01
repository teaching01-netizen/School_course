import type { QueueURLState } from "./types";

export const impactQueryKeys = {
  all: ["schedule-impact"] as const,
  queue: (filters: QueueURLState) =>
    ["schedule-impact", "queue", filters] as const,
  queueSimple: (status: string, q: string, severity: string) =>
    ["schedule-impact", "queue", { status, q, severity }] as const,
  issue: (issueId: string) =>
    ["schedule-impact", "issue", issueId] as const,
  candidates: (issueId: string) =>
    ["schedule-impact", "candidates", issueId] as const,
  processing: ["schedule-impact", "processing"] as const,
  history: (filters: Record<string, string>) =>
    ["schedule-impact", "history", filters] as const,
  navSummary: ["schedule-impact", "nav-summary"] as const,
  allQueues: ["schedule-impact", "queue"] as const,
};
