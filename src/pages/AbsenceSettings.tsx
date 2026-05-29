import { useEffect, useState } from "react";
import { apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import type { AbsenceSettings as AbsenceSettingsModel } from "../types";
import PageHeading from "../components/ui/PageHeading";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import Button from "../components/ui/Button";
import { AbsenceFormEditor } from "../components/absences/AbsenceFormEditor";

function PreviewContent({ settings }: { settings: AbsenceSettingsModel }) {
  return (
    <div className="space-y-4 text-sm">
      <div className="rounded-sm border border-gray-200 bg-gray-50 p-4">
        <p className="text-gray-700 whitespace-pre-wrap">{settings.form.intro_text || "No intro text set."}</p>
      </div>
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">Reason for absence</label>
        <select className="w-full rounded-sm border border-gray-300 p-2 text-sm" disabled>
          <option value="">Select a reason...</option>
          {settings.form.reason_categories.map((cat) => (
            <option key={cat.value} value={cat.value}>{cat.label || cat.value}</option>
          ))}
        </select>
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">From date</label>
          <input className="w-full rounded-sm border border-gray-300 p-2 text-sm" type="date" disabled />
        </div>
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">To date</label>
          <input className="w-full rounded-sm border border-gray-300 p-2 text-sm" type="date" disabled />
        </div>
      </div>
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">
          <input type="checkbox" disabled className="mr-2" />I will sit in on a physical class
        </label>
        <label className="block text-sm font-medium text-gray-700 mb-1">
          <input type="checkbox" disabled className="mr-2" />I will attend via Zoom
        </label>
      </div>
      <div className="rounded-sm border border-gray-200 bg-gray-50 p-4">
        <p className="text-gray-700 whitespace-pre-wrap">{settings.form.confirmation_text || "No confirmation text set."}</p>
      </div>
      <p className="text-xs text-gray-400">Max date range: {settings.form.max_date_range_days} days. {settings.form.require_reason ? "Reason is required." : "Reason is optional."}</p>
    </div>
  );
}

export default function AbsenceSettings() {
  const { addToast } = useToast();
  const [settings, setSettings] = useState<AbsenceSettingsModel | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    void apiJson<AbsenceSettingsModel>("/api/v1/admin/absence-settings", { method: "GET" })
      .then(setSettings)
      .catch((err: unknown) => addToast("error", err instanceof Error ? err.message : "Failed to load settings"));
  }, [addToast]);

  async function save() {
    if (!settings) return;
    setSaving(true);
    try {
      const updated = await apiJson<AbsenceSettingsModel>("/api/v1/admin/absence-settings", { method: "PUT", body: JSON.stringify(settings) });
      setSettings(updated);
      addToast("success", "Absence settings saved");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Settings save failed");
    } finally {
      setSaving(false);
    }
  }

  if (!settings) return <LoadingSkeleton type="text" lines={6} />;

  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <div>
        <PageHeading>Absence Form Settings</PageHeading>
        <p className="text-sm text-gray-500">Configure student form behavior and sit-in resolution without a deployment.</p>
      </div>

      <AbsenceFormEditor
        settings={settings}
        onChange={setSettings}
        onSave={save}
        saving={saving}
        showPreview={true}
        showReasonCategories={true}
        showTextEditors={true}
        showSitInSection={true}
        showFormBehavior={true}
        renderDefaultFooter={false}
        previewContent={<PreviewContent settings={settings} />}
      />

      <section className="rounded-sm border border-gray-200 bg-slate-50 p-5 text-gray-500">
        <h2 className="text-base font-semibold">Student Self-Service</h2>
        <p className="mt-1 text-sm">Viewing and cancelling submitted absences will be enabled in a later student portal release.</p>
        <div className="mt-4 space-y-3 text-sm">
          <label className="flex gap-2"><input type="checkbox" disabled checked={false} /> Allow students to view their submitted absences</label>
          <label className="flex gap-2"><input type="checkbox" disabled checked={false} /> Allow students to cancel their submitted absences</label>
        </div>
      </section>

      <div className="flex justify-end gap-2">
        <Button variant="secondary" onClick={() => window.location.reload()}>Cancel</Button>
        <Button loading={saving} onClick={() => void save()}>Save Settings</Button>
      </div>
    </div>
  );
}
