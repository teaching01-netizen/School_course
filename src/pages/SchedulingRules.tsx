import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { AlertTriangle, CheckCircle2, Info, ShieldAlert, ShieldCheck } from "lucide-react";
import { apiJson } from "../api/client";
import ConfirmModal from "../components/ConfirmModal";
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import { useToast } from "../hooks/useToast";

type Rule = {
  id: string;
  label: string;
  description: string;
  controlled: boolean;
};

type PolicyHistoryItem = {
  id: number;
  created_at: string;
  actor: string;
  previous?: PolicyState;
  next?: PolicyState;
  legacy_forced_on?: boolean;
};

type PolicyState = {
  system_enforced: boolean;
  legacy_sync_enforced: boolean;
};

type SchedulingRulesResponse = PolicyState & {
  updated_at: string;
  rules: Rule[];
  history: PolicyHistoryItem[];
  history_retention: string;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function parsePolicyState(value: unknown): PolicyState {
  if (!isRecord(value) || typeof value.system_enforced !== "boolean" || typeof value.legacy_sync_enforced !== "boolean") {
    throw new Error("Invalid scheduling policy state");
  }
  return { system_enforced: value.system_enforced, legacy_sync_enforced: value.legacy_sync_enforced };
}

function parseSchedulingRulesResponse(value: unknown): SchedulingRulesResponse {
  if (!isRecord(value) || typeof value.updated_at !== "string" || typeof value.history_retention !== "string" || !Array.isArray(value.rules) || !Array.isArray(value.history)) {
    throw new Error("Invalid scheduling rules response");
  }
  const state = parsePolicyState(value);
  const rules: Rule[] = value.rules.map((rule): Rule => {
    if (!isRecord(rule) || typeof rule.id !== "string" || typeof rule.label !== "string" || typeof rule.description !== "string" || typeof rule.controlled !== "boolean") {
      throw new Error("Invalid scheduling rule response");
    }
    return { id: rule.id, label: rule.label, description: rule.description, controlled: rule.controlled };
  });
  const history: PolicyHistoryItem[] = value.history.map((entry): PolicyHistoryItem => {
    if (!isRecord(entry) || typeof entry.id !== "number" || !Number.isInteger(entry.id) || typeof entry.created_at !== "string" || typeof entry.actor !== "string") {
      throw new Error("Invalid scheduling policy history response");
    }
    const previous = entry.previous === undefined ? undefined : parsePolicyState(entry.previous);
    const next = entry.next === undefined ? undefined : parsePolicyState(entry.next);
    if (entry.legacy_forced_on !== undefined && typeof entry.legacy_forced_on !== "boolean") {
      throw new Error("Invalid scheduling policy transition response");
    }
    return { id: entry.id, created_at: entry.created_at, actor: entry.actor, previous, next, legacy_forced_on: entry.legacy_forced_on };
  });
  return { ...state, updated_at: value.updated_at, rules, history, history_retention: value.history_retention };
}

function formatTimestamp(value: string) {
  if (!value) return "—";
  return timestampFormatter.format(new Date(value));
}

const timestampFormatter = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });

function EnforcementBadge({ enforced }: { enforced: boolean }) {
  return (
    <span className={`inline-flex items-center gap-1 rounded-sm border px-2 py-1 text-xs font-semibold ${enforced
      ? "border-[var(--color-wi-green)] bg-[var(--color-wi-green-bg)] text-[var(--color-wi-green)]"
      : "border-[var(--color-wi-red)] bg-[var(--color-wi-danger-bg)] text-[var(--color-wi-red)]"
      }`}>
      {enforced ? <ShieldCheck className="h-3.5 w-3.5" aria-hidden="true" /> : <ShieldAlert className="h-3.5 w-3.5" aria-hidden="true" />}
      {enforced ? "Blocking" : "Warning only"}
    </span>
  );
}

export default function SchedulingRules() {
  const { addToast } = useToast();
  const [settings, setSettings] = useState<SchedulingRulesResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [confirmOff, setConfirmOff] = useState<"system" | "legacy" | null>(null);

  const loadSettings = useCallback(async () => {
    setLoading(true);
    try {
      const response = await apiJson<unknown>("/api/v1/admin/scheduling-rules");
      setSettings(parseSchedulingRulesResponse(response));
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load scheduling rules");
    } finally {
      setLoading(false);
    }
  }, [addToast]);

  useEffect(() => {
    void loadSettings();
  }, [loadSettings]);

  async function updatePolicy(next: PolicyState) {
    setSaving(true);
    try {
      const response = await apiJson<unknown>("/api/v1/admin/scheduling-rules", {
        method: "PUT",
        body: JSON.stringify(next),
      });
      setSettings(parseSchedulingRulesResponse(response));
      addToast("success", "Scheduling conflict policy updated");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Could not update scheduling conflict policy");
    } finally {
      setSaving(false);
      setConfirmOff(null);
    }
  }

  function toggleSystem() {
    if (!settings || saving) return;
    if (settings.system_enforced) {
      setConfirmOff("system");
      return;
    }
    void updatePolicy({ system_enforced: true, legacy_sync_enforced: true });
  }

  function toggleLegacy() {
    if (!settings || saving) return;
    if (settings.legacy_sync_enforced) {
      setConfirmOff("legacy");
      return;
    }
    void updatePolicy({
      system_enforced: settings.system_enforced,
      legacy_sync_enforced: !settings.legacy_sync_enforced,
    });
  }

  if (loading && !settings) return <LoadingSkeleton type="text" lines={8} />;
  if (!settings) {
    return (
      <div className="mx-auto max-w-4xl space-y-4">
        <PageHeading>Scheduling rules</PageHeading>
        <div className="rounded-sm border border-[var(--color-wi-red)] bg-[var(--color-wi-danger-bg)] p-4 text-sm text-[var(--color-wi-red)]">
          Scheduling rules could not be loaded. Use Reload to try again.
        </div>
        <Button variant="secondary" onClick={() => void loadSettings()}>Reload</Button>
      </div>
    );
  }

  const systemState = settings.system_enforced ? "Conflict checks block unsafe writes." : "Conflict checks still run, but writes continue with warnings.";
  const legacyState = settings.legacy_sync_enforced ? "Legacy sync conflict checks block the affected sync row." : "Legacy sync can materialize the row and retain a conflict marker.";

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <PageHeading>Scheduling rules</PageHeading>
          <p className="mt-1 max-w-2xl text-sm text-[var(--color-wi-text-light)]">
            Control whether the six known scheduling conflicts block writes or remain advisory across the entire system.
          </p>
        </div>
        <Button variant="secondary" size="sm" onClick={() => void loadSettings()} disabled={loading || saving}>Reload</Button>
      </div>

      <section className={`rounded-sm border p-4 ${settings.system_enforced
        ? "border-[var(--color-wi-green)] bg-[var(--color-wi-green-bg)]"
        : "border-[var(--color-wi-red)] bg-[var(--color-wi-danger-bg)]"
        }`} aria-live="polite">
        <div className="flex items-start gap-3">
          {settings.system_enforced
            ? <CheckCircle2 className="mt-0.5 h-5 w-5 shrink-0 text-[var(--color-wi-green)]" aria-hidden="true" />
            : <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-[var(--color-wi-red)]" aria-hidden="true" />}
          <div>
            <h2 className="text-base font-semibold text-[var(--color-wi-text)]">
              {settings.system_enforced ? "Conflict enforcement is active" : "Emergency mode: conflicts are allowed"}
            </h2>
            <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">{systemState}</p>
          </div>
        </div>
      </section>

      <section className="rounded-sm border border-wi-line bg-white">
        <div className="border-b border-wi-line px-5 py-4">
          <h2 className="text-base font-semibold text-[var(--color-wi-text)]">Enforcement controls</h2>
          <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">Changes apply globally to new writes and remain in effect until changed here.</p>
        </div>
        <div className="divide-y divide-[var(--color-wi-line)]">
          <div className="flex flex-wrap items-center justify-between gap-4 px-5 py-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <h3 className="font-medium text-[var(--color-wi-text)]">All system writes</h3>
                <EnforcementBadge enforced={settings.system_enforced} />
              </div>
              <p className="mt-1 max-w-2xl text-sm text-[var(--color-wi-text-light)]">Sessions, series, roster, attendance, and availability mutations use this policy.</p>
            </div>
            <Button variant={settings.system_enforced ? "danger" : "primary"} size="sm" onClick={toggleSystem} disabled={saving} aria-pressed={settings.system_enforced}>
              {settings.system_enforced ? "Turn off enforcement" : "Turn on enforcement"}
            </Button>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-4 px-5 py-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <h3 className="font-medium text-[var(--color-wi-text)]">Legacy sync</h3>
                <EnforcementBadge enforced={settings.legacy_sync_enforced} />
              </div>
              <p className="mt-1 max-w-2xl text-sm text-[var(--color-wi-text-light)]">{legacyState}</p>
              {settings.system_enforced && !settings.legacy_sync_enforced && (
                <p className="mt-2 text-xs font-medium text-[var(--color-wi-amber)]">Legacy sync is independently set to warning-only.</p>
              )}
            </div>
        <Button variant="secondary" size="sm" onClick={toggleLegacy} disabled={saving} aria-pressed={settings.legacy_sync_enforced}>
              {settings.legacy_sync_enforced ? "Allow legacy conflicts" : "Block legacy conflicts"}
            </Button>
          </div>
        </div>
        <div className="flex items-start gap-2 border-t border-wi-line bg-[var(--color-wi-callout)] px-5 py-3 text-xs text-[var(--color-wi-text-light)]">
          <Info className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-wi-primary)]" aria-hidden="true" />
          <p>Turning all system enforcement back on also turns legacy sync enforcement on. You can then turn legacy sync off separately.</p>
        </div>
      </section>

      <section className="rounded-sm border border-wi-line bg-white">
        <div className="border-b border-wi-line px-5 py-4">
          <h2 className="text-base font-semibold text-[var(--color-wi-text)]">Controlled rules</h2>
          <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">These rules can become advisory. Identity, reference, malformed-data, and security checks always remain blocking.</p>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[680px] text-sm">
            <caption className="sr-only">Scheduling rules and their current enforcement scope</caption>
            <thead className="bg-[var(--color-wi-callout)] text-left text-xs font-semibold text-[var(--color-wi-text-light)]">
              <tr>
                <th scope="col" className="px-5 py-3">Rule</th>
                <th scope="col" className="px-5 py-3">Description</th>
                <th scope="col" className="px-5 py-3">System writes</th>
                <th scope="col" className="px-5 py-3">Legacy sync</th>
              </tr>
            </thead>
            <tbody>
              {settings.rules.map((rule) => (
                <tr key={rule.id} className="border-t border-wi-line align-top">
                  <th scope="row" className="px-5 py-3 text-left font-medium text-[var(--color-wi-text)]">{rule.label}</th>
                  <td className="px-5 py-3 text-[var(--color-wi-text-light)]">{rule.description}</td>
                  <td className="px-5 py-3"><EnforcementBadge enforced={settings.system_enforced} /></td>
                  <td className="px-5 py-3"><EnforcementBadge enforced={settings.legacy_sync_enforced} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="rounded-sm border border-wi-line bg-white">
        <div className="border-b border-wi-line px-5 py-4">
          <h2 className="text-base font-semibold text-[var(--color-wi-text)]">Policy change history</h2>
          <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">Only setting changes are retained here for {settings.history_retention}. Conflict warnings are transient.</p>
        </div>
        {settings.history.length === 0 ? (
          <p className="px-5 py-6 text-sm text-[var(--color-wi-text-light)]">No policy changes in the last three days.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[640px] text-sm">
              <caption className="sr-only">Recent scheduling policy changes</caption>
              <thead className="bg-[var(--color-wi-callout)] text-left text-xs font-semibold text-[var(--color-wi-text-light)]">
                <tr><th scope="col" className="px-5 py-3">When</th><th scope="col" className="px-5 py-3">Actor</th><th scope="col" className="px-5 py-3">Change</th></tr>
              </thead>
              <tbody>
                {settings.history.map((item) => (
                  <tr key={item.id} className="border-t border-wi-line">
                    <td className="px-5 py-3 text-[var(--color-wi-text-light)]">{formatTimestamp(item.created_at)}</td>
                    <td className="px-5 py-3 text-[var(--color-wi-text)]">{item.actor || "system"}</td>
                    <td className="px-5 py-3 text-[var(--color-wi-text-light)]">
                      {item.next?.system_enforced ? "System blocking" : "System warning-only"}; {item.next?.legacy_sync_enforced ? "legacy blocking" : "legacy warning-only"}
                      {item.legacy_forced_on && <span className="ml-2 text-xs font-medium text-[var(--color-wi-amber)]">legacy forced on</span>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <div className="flex items-start gap-2 text-sm text-[var(--color-wi-text-light)]">
        <Info className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-wi-faint)]" aria-hidden="true" />
        <p>Existing legacy sync conflict review remains available in <Link className="text-[var(--color-wi-primary)] hover:underline" to="/admin/legacy-sync">Legacy Sync</Link>. This page does not create a separate violation record list.</p>
      </div>

      <ConfirmModal
        open={confirmOff !== null}
        title={`Turn off ${confirmOff === "legacy" ? "legacy sync" : "system"} enforcement?`}
        message="Preflight and conflict checks will still run and show red warnings, but affected writes will continue. This setting stays off until an admin turns it back on."
        confirmLabel="Confirm turn off"
        loading={saving}
        onCancel={() => setConfirmOff(null)}
        onConfirm={() => {
          if (confirmOff === "system") {
            void updatePolicy({ system_enforced: false, legacy_sync_enforced: settings.legacy_sync_enforced });
          } else if (confirmOff === "legacy") {
            void updatePolicy({ system_enforced: settings.system_enforced, legacy_sync_enforced: false });
          }
        }}
      />
    </div>
  );
}
