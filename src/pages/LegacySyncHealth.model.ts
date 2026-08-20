export type SyncControl = {
  detection_enabled: boolean;
  fetch_enabled: boolean;
  apply_enabled: boolean;
  student_enabled: boolean;
  tombstone_enabled: boolean;
  realtime_enabled: boolean;
  shadow_mode: boolean;
  updated_at: string | null;
};

export type SyncRun = {
  id: string;
  mode: string;
  status: string;
  started_at: string | null;
  completed_at: string | null;
  pages_requested: number;
  entities_parsed: number;
  entities_changed: number;
  entities_applied: number;
  parse_failures: number;
  reconciliation_mismatches: number;
	source_latency_ms: number | null;
	last_error: string | null;
	progress: SyncRunProgress | null;
};

export type SyncRunProgress = {
	phase: string;
	current_entity: string | null;
	processed_entities: number;
	total_entities: number;
	changed_entities: number;
	applied_entities: number;
	failures: number;
	updated_at: string | null;
};

export type SyncHealth = {
	status: "healthy" | "paused" | "shadow" | "waiting" | "syncing" | "error";
  paused: boolean;
  shadow_mode: boolean;
  control: SyncControl;
  queue: { queued: number; running: number; completed: number; dead: number };
  open_conflicts: number;
  latest_run: SyncRun | null;
  last_successful_at: string | null;
  freshness_seconds: number | null;
};

export type SyncJob = {
  id: string;
  job_type: string;
  entity_type: string | null;
  external_id: string | null;
  priority: number;
  status: string;
  attempt: number;
  max_attempts: number;
  last_error: string | null;
  created_at: string | null;
  updated_at: string | null;
};

export type SyncConflict = {
  id: string;
  entity_type: string;
  external_id: string;
  conflict_type: string;
  category: string;
  message: string | null;
  source_payload: string | null;
  local_payload: string | null;
  status: string;
  created_at: string | null;
};

export function formatPayload(raw: string | null): string {
  if (raw === null) return "";
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

export const entityOptions = [
  ["course", "Course"],
  ["schedule", "Schedule"],
  ["checkin", "Check-in"],
  ["teacher", "Teachers"],
  ["subject", "Subjects"],
  ["classroom", "Classrooms"],
  ["student", "Student"],
  ["full", "Full reconciliation"],
] as const;

export function isFreshnessStale(seconds: number | null): boolean {
  return seconds !== null && seconds > 10 * 60;
}

export function formatFreshness(seconds: number | null): string {
  if (seconds === null) return "Freshness not available";
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m ago`;
}

export function formatTime(value: string | null): string {
  if (!value) return "Not available";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Not available";
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(date);
}

export function statusCopy(status: SyncHealth["status"]): string {
	if (status === "shadow") return "Shadow mode";
	if (status === "paused") return "Paused";
	if (status === "syncing") return "Sync in progress";
	if (status === "error") return "Sync stopped with an error";
	if (status === "waiting") return "Waiting for first run";
	return "Healthy";
}

export function statusClass(status: SyncHealth["status"]): string {
	if (status === "paused") return "border-[var(--color-wi-amber)] bg-[var(--color-wi-amber-bg)] text-[var(--color-wi-amber)]";
	if (status === "syncing") return "border-[var(--color-wi-amber)] bg-[var(--color-wi-amber-bg)] text-[var(--color-wi-amber)]";
	if (status === "error") return "border-[var(--color-wi-red)] bg-[var(--color-wi-danger-bg)] text-[var(--color-wi-red)]";
	if (status === "shadow") return "border-blue-200 bg-blue-50 text-[var(--color-wi-primary)]";
	if (status === "healthy") return "border-emerald-200 bg-emerald-50 text-[var(--color-wi-green)]";
	return "border-wi-line bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]";
}

export function syncPhaseCopy(phase: string): string {
	if (phase === "fetching_course_index") return "Fetching the latest legacy course index";
	if (phase === "course_index_loaded") return "Course index loaded; preparing reconciliation";
	if (phase === "applying_master_data") return "Applying teachers and subjects";
	if (phase === "reconciling_courses") return "Reconciling and linking legacy courses";
	if (phase === "observing_legacy_courses") return "Checking legacy courses in shadow mode";
	if (phase === "importing_student_profiles") return "Importing student directory profiles";
	if (phase === "completed") return "Full reconciliation completed";
	if (phase === "failed") return "Full reconciliation stopped with an error";
	return "Starting legacy synchronization";
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "The sync status could not be loaded.";
}
