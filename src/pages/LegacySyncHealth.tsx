import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, AlertTriangle, CheckCircle2, ClipboardCheck, Pause, Play, RefreshCw } from "lucide-react";
import { Link } from "react-router-dom";
import { apiJson } from "../api/client";
import { cachePolicies, queryKeys } from "../query/cache";
import { useToast } from "../hooks/useToast";
import Button from "../components/ui/Button";
import PageHeading from "../components/ui/PageHeading";
import { entityOptions, errorMessage, formatFreshness, formatPayload, formatTime, isFreshnessStale, statusClass, statusCopy, type SyncConflict, type SyncControl, type SyncHealth, type SyncJob } from "./LegacySyncHealth.model";
import LegacySyncProgress from "./LegacySyncProgress";

export default function LegacySyncHealth() {
  const { addToast } = useToast();
  const queryClient = useQueryClient();
  const [entityType, setEntityType] = useState("course");
  const [externalID, setExternalID] = useState("");

  const healthQuery = useQuery({
    queryKey: queryKeys.legacySync.health,
    queryFn: () => apiJson<SyncHealth>("/api/v1/admin/legacy-sync/health"),
    ...cachePolicies.operational,
    refetchInterval: 2_000,
  });
  const jobsQuery = useQuery({
    queryKey: queryKeys.legacySync.jobs,
    queryFn: () => apiJson<SyncJob[]>("/api/v1/admin/legacy-sync/jobs?limit=12"),
    ...cachePolicies.operational,
    refetchInterval: 2_000,
  });
  const conflictsQuery = useQuery({
    queryKey: queryKeys.legacySync.conflicts,
    queryFn: () => apiJson<SyncConflict[]>("/api/v1/admin/legacy-sync/conflicts"),
    ...cachePolicies.operational,
  });

  const controlMutation = useMutation({
    mutationFn: (action: "pause" | "resume") => apiJson<SyncControl>(`/api/v1/admin/legacy-sync/${action}`, { method: "POST" }),
    onSuccess: (_, action) => {
      addToast("success", action === "pause" ? "Legacy synchronization paused" : "Legacy synchronization resumed");
      void queryClient.invalidateQueries({ queryKey: queryKeys.legacySync.all });
    },
    onError: (error) => addToast("error", errorMessage(error)),
  });

  const shadowMutation = useMutation({
    mutationFn: (enabled: boolean) => apiJson<SyncControl>("/api/v1/admin/legacy-sync/shadow", {
      method: "POST",
      body: JSON.stringify({ enabled }),
    }),
    onSuccess: (_, enabled) => {
      addToast("success", enabled ? "Shadow mode enabled — observations only, nothing is applied" : "Shadow mode disabled — sync applies to local data");
      void queryClient.invalidateQueries({ queryKey: queryKeys.legacySync.all });
    },
    onError: (error) => addToast("error", errorMessage(error)),
  });

  const studentImportMutation = useMutation({
    mutationFn: (enabled: boolean) => apiJson<SyncControl>("/api/v1/admin/legacy-sync/student-import", {
      method: "POST",
      body: JSON.stringify({ enabled }),
    }),
    onSuccess: (_, enabled) => {
      addToast("success", enabled ? "Legacy rosters will import students on the next reconcile" : "Legacy roster import disabled");
      void queryClient.invalidateQueries({ queryKey: queryKeys.legacySync.all });
    },
    onError: (error) => addToast("error", errorMessage(error)),
  });

  const conflictMutation = useMutation({
    mutationFn: ({ id, action }: { id: string; action: "resolve" | "ignore" }) =>
      apiJson<SyncConflict>(`/api/v1/admin/legacy-sync/conflicts/${id}/${action}`, { method: "POST" }),
    onSuccess: (_, { action }) => {
      addToast("success", action === "resolve" ? "Conflict resolved" : "Conflict ignored");
      void queryClient.invalidateQueries({ queryKey: queryKeys.legacySync.all });
    },
    onError: (error) => addToast("error", errorMessage(error)),
  });

  const refreshMutation = useMutation({
    mutationFn: () => apiJson<SyncJob>("/api/v1/admin/legacy-sync/refresh", {
      method: "POST",
      body: JSON.stringify({ entity_type: entityType, external_id: entityType === "full" ? "" : externalID }),
    }),
    onSuccess: () => {
      addToast("success", "Refresh queued");
      setExternalID("");
      void queryClient.invalidateQueries({ queryKey: queryKeys.legacySync.all });
    },
    onError: (error) => addToast("error", errorMessage(error)),
  });

  const health = healthQuery.data;
  const requiresExternalID = entityType !== "full";
  const refreshDisabled = refreshMutation.isPending || (requiresExternalID && externalID.trim() === "");
  const recentJobs = jobsQuery.data ?? [];
  const conflicts = conflictsQuery.data ?? [];
  const metricCards = useMemo(() => {
    if (!health) return [];
    return [
      { label: "Queued", value: health.queue.queued, tone: "text-[var(--color-wi-primary)]" },
      { label: "Running", value: health.queue.running, tone: "text-[var(--color-wi-amber)]" },
      { label: "Dead letters", value: health.queue.dead, tone: health.queue.dead ? "text-[var(--color-wi-red)]" : "text-[var(--color-wi-green)]" },
      { label: "Open conflicts", value: health.open_conflicts, tone: health.open_conflicts ? "text-[var(--color-wi-red)]" : "text-[var(--color-wi-green)]" },
    ];
  }, [health]);

  return (
    <div className="max-w-[1100px] space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <PageHeading>Legacy synchronization</PageHeading>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-[var(--color-wi-text-light)]">
            Monitor the local mirror of the Warwick legacy site. Refreshes are queued for the separate sync service and never block this application.
          </p>
        </div>
        <Button variant="secondary" size="md" onClick={() => void healthQuery.refetch()} loading={healthQuery.isFetching && !healthQuery.data}>
          <RefreshCw className="mr-2 h-4 w-4" aria-hidden="true" />
          Refresh status
        </Button>
        <Link to="/admin/legacy-sync/audit" className="inline-flex min-h-[34px] items-center rounded-sm border border-wi-line bg-white px-3 py-1.5 text-sm font-medium hover:bg-[var(--color-wi-row-alt)]">
          <ClipboardCheck className="mr-2 h-4 w-4" aria-hidden="true" />
          Migration audit
        </Link>
      </div>

      {healthQuery.isError ? (
        <section className="border border-[var(--color-wi-red)]/30 bg-red-50 p-4" role="alert">
          <div className="flex items-start gap-3">
            <AlertTriangle className="mt-0.5 h-5 w-5 text-[var(--color-wi-red)]" aria-hidden="true" />
            <div>
              <p className="font-semibold text-[var(--color-wi-text)]">Sync status unavailable</p>
              <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">{errorMessage(healthQuery.error)}</p>
              <Button className="mt-3" variant="secondary" size="sm" onClick={() => void healthQuery.refetch()}>Try again</Button>
            </div>
          </div>
        </section>
      ) : health ? (
        <>
          <section className="border border-[var(--color-wi-border)] bg-white p-5" aria-labelledby="sync-status-heading">
            <div className="flex flex-wrap items-center justify-between gap-4">
              <div className="flex items-center gap-3">
                <div className={`flex h-10 w-10 items-center justify-center rounded-full border ${statusClass(health.status)}`}>
                  <Activity className="h-5 w-5" aria-hidden="true" />
                </div>
                <div>
                  <h2 id="sync-status-heading" className="text-lg font-semibold text-[var(--color-wi-text)]">{statusCopy(health.status)}</h2>
                  <p className="text-sm text-[var(--color-wi-text-light)]">
                    {health.last_successful_at ? `Last successful run ${formatTime(health.last_successful_at)}` : "No successful run recorded yet"}
                  </p>
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                {health.paused ? (
                  <Button onClick={() => controlMutation.mutate("resume")} loading={controlMutation.isPending}>
                    <Play className="mr-2 h-4 w-4" aria-hidden="true" /> Resume sync
                  </Button>
                ) : (
                  <Button variant="secondary" onClick={() => controlMutation.mutate("pause")} loading={controlMutation.isPending}>
                    <Pause className="mr-2 h-4 w-4" aria-hidden="true" /> Pause sync
                  </Button>
                )}
              </div>
            </div>
            <div className="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              {metricCards.map((metric) => (
                <div key={metric.label} className="border border-wi-line bg-[var(--color-wi-row-alt)] px-4 py-3">
                  <p className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">{metric.label}</p>
                  <p className={`mt-1 text-2xl font-bold tabular-nums ${metric.tone}`}>{metric.value}</p>
                </div>
              ))}
            </div>
            {isFreshnessStale(health.freshness_seconds) ? (
              <div className="mt-4 flex items-start gap-2 border border-[var(--color-wi-amber)]/40 bg-[var(--color-wi-amber-bg)] px-3 py-2 text-sm text-[var(--color-wi-text)]" role="status">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-wi-amber)]" aria-hidden="true" />
                <span>Local mirror freshness is {formatFreshness(health.freshness_seconds)}. Check the source and queue before relying on current legacy-owned data.</span>
              </div>
            ) : null}
          </section>

          <LegacySyncProgress run={health.latest_run} queue={health.queue} />

          <div className="grid gap-6 lg:grid-cols-[minmax(0,1.2fr)_minmax(280px,0.8fr)]">
            <section className="border border-[var(--color-wi-border)] bg-white p-5" aria-labelledby="refresh-heading">
              <h2 id="refresh-heading" className="text-lg font-semibold text-[var(--color-wi-text)]">Queue a refresh</h2>
              <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">Use a targeted job for an operational change or start a bounded reconciliation.</p>
              <div className="mt-4 grid gap-4 sm:grid-cols-2">
                <label className="text-sm font-medium text-[var(--color-wi-text)]">
                  Entity
                  <select value={entityType} onChange={(event) => setEntityType(event.target.value)} className="mt-1 block min-h-10 w-full rounded-sm border border-wi-line bg-white px-3 text-sm focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-blue-100">
                    {entityOptions.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                  </select>
                </label>
                <label className="text-sm font-medium text-[var(--color-wi-text)]">
                  Legacy ID {requiresExternalID ? <span className="text-[var(--color-wi-red)]">*</span> : <span className="font-normal text-[var(--color-wi-text-light)]">(not needed)</span>}
                  <input value={externalID} onChange={(event) => setExternalID(event.target.value)} disabled={!requiresExternalID} placeholder={requiresExternalID ? "e.g. 7306" : "Full reconciliation"} className="mt-1 block min-h-10 w-full rounded-sm border border-wi-line px-3 text-sm focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-blue-100 disabled:bg-[var(--color-wi-row-alt)]" />
                </label>
              </div>
              <Button className="mt-4" onClick={() => refreshMutation.mutate()} loading={refreshMutation.isPending} disabled={refreshDisabled}>Queue refresh</Button>
            </section>

            <section className="border border-[var(--color-wi-border)] bg-white p-5" aria-labelledby="mode-heading">
              <h2 id="mode-heading" className="text-lg font-semibold text-[var(--color-wi-text)]">Operating mode</h2>
              <dl className="mt-4 space-y-3 text-sm">
                <div className="flex items-center justify-between gap-4"><dt className="text-[var(--color-wi-text-light)]">Apply to local domain</dt><dd className="font-medium text-[var(--color-wi-text)]">{health.control.apply_enabled ? "Enabled" : "Paused"}</dd></div>
                <div className="flex items-center justify-between gap-4"><dt className="text-[var(--color-wi-text-light)]">Shadow mode</dt><dd className="flex items-center gap-2 font-medium text-[var(--color-wi-text)]">{health.control.shadow_mode ? "On" : "Off"}
                  <Button variant="secondary" size="sm" loading={shadowMutation.isPending} onClick={() => shadowMutation.mutate(!health.control.shadow_mode)} aria-label={health.control.shadow_mode ? "Disable shadow mode" : "Enable shadow mode"}>
                    {health.control.shadow_mode ? "Turn off" : "Turn on"}
                  </Button>
                </dd></div>
                <div className="flex items-center justify-between gap-4"><dt className="text-[var(--color-wi-text-light)]">Import legacy students</dt><dd className="flex items-center gap-2 font-medium text-[var(--color-wi-text)]">{health.control.student_enabled ? "On" : "Off"}
                  <Button variant="secondary" size="sm" loading={studentImportMutation.isPending} onClick={() => studentImportMutation.mutate(!health.control.student_enabled)} aria-label={health.control.student_enabled ? "Disable student import" : "Enable student import"}>
                    {health.control.student_enabled ? "Turn off" : "Turn on"}
                  </Button>
                </dd></div>
                <div className="flex items-center justify-between gap-4"><dt className="text-[var(--color-wi-text-light)]">Realtime events</dt><dd className="font-medium text-[var(--color-wi-text)]">{health.control.realtime_enabled ? "Enabled" : "Disabled"}</dd></div>
                <div className="flex items-center justify-between gap-4"><dt className="text-[var(--color-wi-text-light)]">Tombstoning</dt><dd className="font-medium text-[var(--color-wi-text)]">{health.control.tombstone_enabled ? "Enabled" : "Guarded"}</dd></div>
              </dl>
              {health.control.shadow_mode ? (
                <p className="mt-4 border border-[var(--color-wi-amber)]/40 bg-[var(--color-wi-amber-bg)] px-3 py-2 text-xs text-[var(--color-wi-text)]" role="status">
                  Shadow mode is on: reconciles observe the legacy site but never create, link, or update local courses.
                </p>
              ) : null}
              <div className="mt-4 flex gap-2 border-t border-wi-line pt-4 text-xs text-[var(--color-wi-text-light)]"><CheckCircle2 className="h-4 w-4 text-[var(--color-wi-green)]" aria-hidden="true" /> Local application reads remain available during source outages.</div>
            </section>
          </div>
        </>
      ) : (
        <div className="border border-wi-line bg-white p-8 text-sm text-[var(--color-wi-text-light)]">Loading synchronization status…</div>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <section className="border border-[var(--color-wi-border)] bg-white" aria-labelledby="jobs-heading">
          <div className="flex items-center justify-between border-b border-wi-line px-5 py-4"><h2 id="jobs-heading" className="text-lg font-semibold text-[var(--color-wi-text)]">Recent jobs</h2><span className="text-xs text-[var(--color-wi-text-light)]">Last 12</span></div>
          <div className="overflow-x-auto">
            <table className="min-w-full text-left text-sm"><thead className="bg-[var(--color-wi-row-alt)] text-xs uppercase tracking-wide text-[var(--color-wi-text-light)]"><tr><th className="px-5 py-3 font-semibold">Job</th><th className="px-5 py-3 font-semibold">Status</th><th className="px-5 py-3 font-semibold">Attempts</th></tr></thead><tbody className="divide-y divide-wi-line">{recentJobs.map((job) => <tr key={job.id}><td className="px-5 py-3"><div className="font-medium text-[var(--color-wi-text)]">{job.entity_type ?? "full"}{job.external_id ? ` · ${job.external_id}` : ""}</div><div className="text-xs text-[var(--color-wi-text-light)]">{job.job_type}</div></td><td className="px-5 py-3"><span className={job.status === "dead" ? "font-medium text-[var(--color-wi-red)]" : job.status === "completed" ? "font-medium text-[var(--color-wi-green)]" : "text-[var(--color-wi-text)]"}>{job.status}</span>{job.last_error ? <div className="max-w-[220px] truncate text-xs text-[var(--color-wi-red)]" title={job.last_error}>{job.last_error}</div> : null}</td><td className="px-5 py-3 tabular-nums text-[var(--color-wi-text-light)]">{job.attempt}/{job.max_attempts}</td></tr>)}{recentJobs.length === 0 ? <tr><td colSpan={3} className="px-5 py-8 text-center text-[var(--color-wi-text-light)]">No jobs recorded.</td></tr> : null}</tbody></table>
          </div>
        </section>

        <section className="border border-[var(--color-wi-border)] bg-white" aria-labelledby="conflicts-heading">
          <div className="flex items-center justify-between border-b border-wi-line px-5 py-4"><h2 id="conflicts-heading" className="text-lg font-semibold text-[var(--color-wi-text)]">Open conflicts</h2><span className="text-xs text-[var(--color-wi-text-light)]">{conflicts.length} open</span></div>
          <div className="divide-y divide-wi-line">{conflicts.map((conflict) => <div key={conflict.id} className="px-5 py-4"><div className="flex items-start justify-between gap-4"><div><p className="font-medium text-[var(--color-wi-text)]">{conflict.entity_type} · {conflict.external_id}</p><p className="mt-1 text-sm text-[var(--color-wi-text-light)]">{conflict.message ?? conflict.conflict_type}</p></div><span className="shrink-0 text-xs font-medium text-[var(--color-wi-red)]">{conflict.category}</span></div>
            {(conflict.source_payload !== null || conflict.local_payload !== null) ? (
              <dl className="mt-3 grid gap-2 text-xs">
                {conflict.source_payload !== null ? <div><dt className="font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Source payload</dt><dd><pre className="mt-1 max-h-32 overflow-y-auto rounded-sm border border-wi-line bg-[var(--color-wi-row-alt)] p-2 font-mono text-[11px] leading-4 text-[var(--color-wi-text)]">{formatPayload(conflict.source_payload)}</pre></dd></div> : null}
                {conflict.local_payload !== null ? <div><dt className="font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Local payload</dt><dd><pre className="mt-1 max-h-32 overflow-y-auto rounded-sm border border-wi-line bg-[var(--color-wi-row-alt)] p-2 font-mono text-[11px] leading-4 text-[var(--color-wi-text)]">{formatPayload(conflict.local_payload)}</pre></dd></div> : null}
              </dl>
            ) : null}
            <div className="mt-3 flex gap-2">
              <Button variant="secondary" size="sm" loading={conflictMutation.isPending && conflictMutation.variables?.id === conflict.id && conflictMutation.variables?.action === "resolve"} onClick={() => conflictMutation.mutate({ id: conflict.id, action: "resolve" })}>Resolve</Button>
              <Button variant="secondary" size="sm" loading={conflictMutation.isPending && conflictMutation.variables?.id === conflict.id && conflictMutation.variables?.action === "ignore"} onClick={() => conflictMutation.mutate({ id: conflict.id, action: "ignore" })}>Ignore</Button>
            </div>
          </div>)}{conflicts.length === 0 ? <div className="flex items-center gap-2 px-5 py-8 text-sm text-[var(--color-wi-text-light)]"><CheckCircle2 className="h-4 w-4 text-[var(--color-wi-green)]" aria-hidden="true" /> No open conflicts.</div> : null}</div>
        </section>
      </div>

      {(jobsQuery.isError || conflictsQuery.isError) ? <p className="text-sm text-[var(--color-wi-red)]" role="status">Some secondary sync details could not be loaded. The health summary remains authoritative.</p> : null}
    </div>
  );
}
