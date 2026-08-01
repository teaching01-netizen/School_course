import type { QueueURLState } from "../types";

/**
 * Parse URL search parameters into a typed QueueURLState.
 * Invalid values fall back to safe defaults.
 */
export function parseQueueParams(searchParams: URLSearchParams): QueueURLState {
  const viewRaw = searchParams.get("view");
  const view: QueueURLState["view"] =
    viewRaw === "processing" || viewRaw === "history" ? viewRaw : "queue";
  const query = searchParams.get("q") ?? "";
  const severityRaw = searchParams.get("severity");
  const severity: QueueURLState["severity"] = severityRaw === "critical" || severityRaw === "warning" ? severityRaw : "";
  const status = (searchParams.get("status") ?? "all") as QueueURLState["status"];
  const offset = parseInt(searchParams.get("offset") ?? "0", 10);
  const rawLimit = parseInt(searchParams.get("limit") ?? "25", 10);
  const limit = (rawLimit === 50 || rawLimit === 100 ? rawLimit : 25) as QueueURLState["limit"];

  return {
    view,
    query,
    severity,
    status,
    offset: isNaN(offset) ? 0 : offset,
    limit,
  };
}

/**
 * Serialize a QueueURLState into URL search parameters.
 * Omits default values to keep URLs clean.
 */
export function serializeQueueParams(state: QueueURLState): URLSearchParams {
  const params = new URLSearchParams();
  if (state.view !== "queue") params.set("view", state.view);
  if (state.query) params.set("q", state.query);
  if (state.severity) params.set("severity", state.severity);
  if (state.status !== "all") params.set("status", state.status);
  if (state.offset > 0) params.set("offset", String(state.offset));
  if (state.limit !== 25) params.set("limit", String(state.limit));
  return params;
}

/**
 * When a filter changes, reset offset to 0.
 */
export function resetOffsetOnFilterChange(): number {
  return 0;
}
