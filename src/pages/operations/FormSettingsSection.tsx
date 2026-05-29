import { useEffect, useState } from "react";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import LoadingSkeleton from "../../components/ui/LoadingSkeleton";
import { AbsenceFormEditor } from "../../components/absences/AbsenceFormEditor";
import type { AbsenceSettings } from "../../types";

export function FormSettingsSection() {
  const { addToast } = useToast();
  const [settings, setSettings] = useState<AbsenceSettings | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let active = true;
    (async () => {
      try {
        const data = await apiJson<AbsenceSettings>("/api/v1/admin/absence-settings", { method: "GET" });
        if (active) setSettings(data);
      } catch (err) {
        if (active) addToast("error", err instanceof Error ? err.message : "Failed to load settings");
      }
    })();
    return () => {
      active = false;
    };
  }, [addToast]);

  async function save() {
    if (!settings) return;
    setSaving(true);
    try {
      const updated = await apiJson<AbsenceSettings>("/api/v1/admin/absence-settings", {
        method: "PUT",
        body: JSON.stringify(settings),
      });
      setSettings(updated);
      addToast("success", "Settings saved");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  if (!settings) return <LoadingSkeleton type="text" lines={6} />;

  return (
    <AbsenceFormEditor
      settings={settings}
      onChange={setSettings}
      onSave={save}
      saving={saving}
      showPreview={false}
      showReasonCategories={false}
      renderDefaultFooter={true}
    />
  );
}
