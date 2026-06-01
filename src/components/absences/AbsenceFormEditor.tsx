import { useState, type ReactNode } from "react";
import { Plus, Trash2, Eye } from "lucide-react";
import Button from "../ui/Button";
import Modal from "../Modal";
import type { AbsenceSettings } from "../../types";

type AbsenceFormEditorProps = {
  settings: AbsenceSettings;
  onChange: (settings: AbsenceSettings) => void;
  onSave: () => void;
  saving: boolean;
  showTextEditors?: boolean;
  showReasonCategories?: boolean;
  showSitInSection?: boolean;
  showPreview?: boolean;
  showFormBehavior?: boolean;
  renderDefaultFooter?: boolean;
  previewContent?: ReactNode;
};

export function AbsenceFormEditor({
  settings,
  onChange,
  onSave,
  saving,
  showTextEditors = true,
  showReasonCategories = true,
  showSitInSection = true,
  showPreview = false,
  showFormBehavior = true,
  renderDefaultFooter = true,
  previewContent,
}: AbsenceFormEditorProps) {
  const [previewOpen, setPreviewOpen] = useState(false);

  const inputClass = "mt-1 block w-full rounded-sm border border-gray-300 p-2 text-sm";

  return (
    <div className="space-y-4">
      {showFormBehavior && (
        <section className="rounded-sm border border-gray-200 bg-white p-5">
          <h2 className="mb-4 text-base font-semibold">Form Behavior</h2>
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="text-sm text-gray-700">
              Maximum date range (days)
              <input
                aria-label="Maximum date range"
                className={inputClass}
                min={1}
                max={365}
                type="number"
                value={settings.form.max_date_range_days}
                onChange={(e) =>
                  onChange({ ...settings, form: { ...settings.form, max_date_range_days: Number(e.target.value) } })
                }
              />
            </label>
            <div className="space-y-3 pt-6 text-sm">
              <label className="flex gap-2">
                <input
                  type="checkbox"
                  checked={settings.form.require_reason}
                  onChange={(e) => onChange({ ...settings, form: { ...settings.form, require_reason: e.target.checked } })}
                />{" "}
                Require reason category
              </label>
              <label className="flex gap-2">
                <input
                  type="checkbox"
                  checked={settings.form.allow_free_text_reason}
                  onChange={(e) => onChange({ ...settings, form: { ...settings.form, allow_free_text_reason: e.target.checked } })}
                />{" "}
                Allow free-text details
              </label>
            </div>
          </div>

          {showReasonCategories && (
            <div className="mt-5">
              <div className="mb-2 flex items-center justify-between">
                <h3 className="text-sm font-medium">Reason categories</h3>
                <Button size="sm" variant="secondary" onClick={() => onChange({ ...settings, form: { ...settings.form, reason_categories: [...settings.form.reason_categories, { value: "", label: "" }] } })}>
                  <Plus className="mr-1 h-3.5 w-3.5" />Add Category
                </Button>
              </div>
              <div className="space-y-2">
                {settings.form.reason_categories.map((category, index) => (
                  <div className="flex gap-2" key={`${category.value}-${index}`}>
                    <input
                      aria-label={`Reason value ${index + 1}`}
                      className="w-1/3 rounded-sm border border-gray-300 p-2 text-sm"
                      placeholder="value"
                      value={category.value}
                      onChange={(e) => {
                        const categories = [...settings.form.reason_categories];
                        categories[index] = { ...category, value: e.target.value };
                        onChange({ ...settings, form: { ...settings.form, reason_categories: categories } });
                      }}
                    />
                    <input
                      aria-label={`Reason label ${index + 1}`}
                      className="flex-1 rounded-sm border border-gray-300 p-2 text-sm"
                      placeholder="Student-facing label"
                      value={category.label}
                      onChange={(e) => {
                        const categories = [...settings.form.reason_categories];
                        categories[index] = { ...category, label: e.target.value };
                        onChange({ ...settings, form: { ...settings.form, reason_categories: categories } });
                      }}
                    />
                    <Button variant="ghost" size="sm" aria-label={`Remove reason ${index + 1}`} disabled={settings.form.reason_categories.length === 1} onClick={() => onChange({ ...settings, form: { ...settings.form, reason_categories: settings.form.reason_categories.filter((_, row) => row !== index) } })}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                ))}
              </div>
            </div>
          )}

          {showTextEditors && (
            <>
              <label className="mt-5 block text-sm">
                Intro text shown to students
                <textarea className="mt-1 block w-full rounded-sm border border-gray-300 p-2 text-sm" maxLength={500} rows={3} value={settings.form.intro_text} onChange={(e) => onChange({ ...settings, form: { ...settings.form, intro_text: e.target.value } })} />
              </label>
              <label className="mt-4 block text-sm">
                Confirmation text shown after submission
                <textarea className="mt-1 block w-full rounded-sm border border-gray-300 p-2 text-sm" maxLength={500} rows={2} value={settings.form.confirmation_text} onChange={(e) => onChange({ ...settings, form: { ...settings.form, confirmation_text: e.target.value } })} />
              </label>
            </>
          )}
        </section>
      )}

      {showSitInSection && (
        <section className="rounded-sm border border-gray-200 bg-white p-5">
          <h2 className="mb-4 text-base font-semibold">Sit-in Resolution</h2>
          <label className="mb-4 flex gap-2 text-sm">
            <input
              type="checkbox"
              checked={settings.sit_in.auto_resolve_enabled}
              onChange={(e) => onChange({ ...settings, sit_in: { ...settings.sit_in, auto_resolve_enabled: e.target.checked } })}
            />{" "}
            Auto-resolve sit-in plans for students
          </label>
          <label className="block text-sm">
            Zoom description
            <input className={inputClass} value={settings.sit_in.zoom_description} onChange={(e) => onChange({ ...settings, sit_in: { ...settings.sit_in, zoom_description: e.target.value } })} />
          </label>
          <label className="mt-4 block text-sm">
            Maximum sit-in sessions per absence
            <input className="mt-1 block w-32 rounded-sm border border-gray-300 p-2 text-sm" min={1} max={100} type="number" value={settings.sit_in.max_sessions_per_absence} onChange={(e) => onChange({ ...settings, sit_in: { ...settings.sit_in, max_sessions_per_absence: Number(e.target.value) } })} />
          </label>
        </section>
      )}

      {showTextEditors && (
        <section className="rounded-sm border border-gray-200 bg-white p-5">
          <h2 className="mb-4 text-base font-semibold">SMS Notifications</h2>
          <div className="space-y-4">
            <label className="flex gap-2 text-sm">
              <input
                type="checkbox"
                checked={settings.notifications?.sms_parent_enabled ?? true}
                onChange={(e) => onChange({ ...settings, notifications: { ...settings.notifications, sms_parent_enabled: e.target.checked, sms_parent_template: settings.notifications?.sms_parent_template ?? "", sms_success_template: settings.notifications?.sms_success_template ?? "", allow_submit_without_otp: settings.notifications?.allow_submit_without_otp ?? false } })}
              />{" "}
              Enable parent SMS notifications
            </label>
            <label className="block text-sm">
              Verification SMS template (sent to parent)
              <textarea className="mt-1 block w-full rounded-sm border border-gray-300 p-2 text-sm" maxLength={500} rows={3} value={settings.notifications?.sms_parent_template ?? ""}               onChange={(e) => onChange({ ...settings, notifications: { ...settings.notifications, sms_parent_template: e.target.value, sms_parent_enabled: settings.notifications?.sms_parent_enabled ?? true, sms_success_template: settings.notifications?.sms_success_template ?? "", allow_submit_without_otp: settings.notifications?.allow_submit_without_otp ?? false } })} />
              <span className="mt-1 text-xs text-gray-500">Placeholders: {"{{student_name}}"}, {"{{code}}"}</span>
            </label>
            <label className="block text-sm">
              Success SMS template (sent to parent and student after submission)
              <textarea className="mt-1 block w-full rounded-sm border border-gray-300 p-2 text-sm" maxLength={500} rows={3} value={settings.notifications?.sms_success_template ?? ""}               onChange={(e) => onChange({ ...settings, notifications: { ...settings.notifications, sms_success_template: e.target.value, sms_parent_enabled: settings.notifications?.sms_parent_enabled ?? true, sms_parent_template: settings.notifications?.sms_parent_template ?? "", allow_submit_without_otp: settings.notifications?.allow_submit_without_otp ?? false } })} />
              <span className="mt-1 text-xs text-gray-500">Placeholders: {"{{nickname}}"}, {"{{class_name}}"}, {"{{absence_date}}"}, {"{{sit_in_class}}"}, {"{{sit_in_date_time}}"}</span>
            </label>
          </div>
        </section>
      )}

      {showPreview && previewContent && (
        <>
          <div className="flex justify-end">
            <Button variant="secondary" onClick={() => setPreviewOpen(true)}>
              <Eye className="mr-1 h-4 w-4" />Preview Form
            </Button>
          </div>
          {previewOpen && (
            <Modal title="Preview Absence Form" size="lg" onClose={() => setPreviewOpen(false)} footer={<Button variant="secondary" onClick={() => setPreviewOpen(false)}>Close</Button>}>
              {previewContent}
            </Modal>
          )}
        </>
      )}

      {renderDefaultFooter && (
        <div className="flex justify-end">
          <Button loading={saving} onClick={onSave}>Save Settings</Button>
        </div>
      )}
    </div>
  );
}
