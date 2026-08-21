import { Activity, AlertTriangle, CheckCircle2 } from "lucide-react";
import { formatTime, syncPhaseCopy, type SyncHealth, type SyncRun } from "./LegacySyncHealth.model";

type LegacySyncProgressProps = {
  run: SyncRun | null;
  queue: SyncHealth["queue"];
};

function count(value: number): string {
  return value.toLocaleString();
}

export default function LegacySyncProgress({ run, queue }: LegacySyncProgressProps) {
  const progress = run?.progress;
  if (!run || !progress) return null;

  const hasTotal = progress.total_entities > 0;
  const percent = hasTotal ? Math.min(100, Math.round((progress.processed_entities / progress.total_entities) * 100)) : null;
  const isFailed = run.status === "failed";
  const isComplete = run.status === "completed";
  const hasQueuedRefreshes = queue.queued > 0 || queue.running > 0;

  return (
    <section className="border border-[var(--color-wi-primary)]/30 bg-white p-5" aria-labelledby="live-import-heading" aria-live="polite">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex items-start gap-3">
          <div className={`mt-0.5 flex h-9 w-9 items-center justify-center rounded-full border ${isFailed ? "border-[var(--color-wi-red)] bg-[var(--color-wi-danger-bg)] text-[var(--color-wi-red)]" : isComplete ? "border-[var(--color-wi-green)] bg-[var(--color-wi-row-alt)] text-[var(--color-wi-green)]" : "border-[var(--color-wi-primary)] bg-[var(--color-wi-row-alt)] text-[var(--color-wi-primary)]"}`}>
            {isFailed ? <AlertTriangle className="h-4 w-4" aria-hidden="true" /> : isComplete ? <CheckCircle2 className="h-4 w-4" aria-hidden="true" /> : <Activity className="h-4 w-4" aria-hidden="true" />}
          </div>
          <div>
            <h2 id="live-import-heading" className="text-lg font-semibold text-[var(--color-wi-text)]">Live import result</h2>
            <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">{syncPhaseCopy(progress.phase)}</p>
          </div>
        </div>
        <p className="text-xs font-medium text-[var(--color-wi-text-light)]">Updates automatically every 2 seconds</p>
      </div>

      {percent !== null ? (
        <div className="mt-5">
          <div className="flex items-center justify-between text-xs font-semibold text-[var(--color-wi-text-light)]">
            <span>{count(progress.processed_entities)} / {count(progress.total_entities)} processed</span>
            <span className="tabular-nums">{percent}%</span>
          </div>
          <div className="mt-2 h-2 overflow-hidden rounded-full border border-wi-line bg-[var(--color-wi-row-alt)]" role="progressbar" aria-label="Legacy import progress" aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent}>
            <div className="h-full bg-[var(--color-wi-primary)] transition-[width] duration-200 motion-reduce:transition-none" style={{ width: `${percent}%` }} />
          </div>
        </div>
      ) : null}

      <dl className="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div className="border border-wi-line bg-[var(--color-wi-row-alt)] px-3 py-2">
          <dt className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Processed</dt>
          <dd className="mt-1 text-lg font-semibold tabular-nums text-[var(--color-wi-text)]">{count(progress.processed_entities)}{hasTotal ? ` / ${count(progress.total_entities)}` : ""}</dd>
        </div>
        <div className="border border-wi-line bg-[var(--color-wi-row-alt)] px-3 py-2">
          <dt className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Changed / found</dt>
          <dd className="mt-1 text-lg font-semibold tabular-nums text-[var(--color-wi-text)]">{count(progress.changed_entities)}</dd>
        </div>
        <div className="border border-wi-line bg-[var(--color-wi-row-alt)] px-3 py-2">
          <dt className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Applied / queued</dt>
          <dd className="mt-1 text-lg font-semibold tabular-nums text-[var(--color-wi-text)]">{count(progress.applied_entities)}</dd>
        </div>
        <div className="border border-wi-line bg-[var(--color-wi-row-alt)] px-3 py-2">
          <dt className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Failures / conflicts</dt>
          <dd className={`mt-1 text-lg font-semibold tabular-nums ${progress.failures > 0 ? "text-[var(--color-wi-red)]" : "text-[var(--color-wi-green)]"}`}>{count(progress.failures)}</dd>
        </div>
      </dl>

      <div className="mt-4 flex flex-wrap justify-between gap-2 border-t border-wi-line pt-3 text-xs text-[var(--color-wi-text-light)]">
        <span>Current item: <strong className="font-medium text-[var(--color-wi-text)]">{progress.current_entity ?? "Preparing next stage"}</strong></span>
        <span>Last update: {formatTime(progress.updated_at)}</span>
      </div>

      {isFailed && run.last_error ? (
        <p className="mt-4 border border-[var(--color-wi-red)]/30 bg-[var(--color-wi-danger-bg)] px-3 py-2 text-sm text-[var(--color-wi-text)]" role="alert">{run.last_error}</p>
      ) : null}
      {hasQueuedRefreshes ? (
        <p className="mt-4 border border-[var(--color-wi-amber)]/40 bg-[var(--color-wi-amber-bg)] px-3 py-2 text-sm text-[var(--color-wi-text)]" role="status">
          {isComplete ? "The full reconciliation finished." : "The full reconciliation is still running."} {count(queue.queued)} course refreshes are still queued{queue.running > 0 ? ` and ${count(queue.running)} running` : ""}; the worker will import those course details next.
        </p>
      ) : null}
    </section>
  );
}
