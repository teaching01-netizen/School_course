import { useEffect, useState, useCallback } from "react";
import { useToast } from "../hooks/useToast";
import type { EmailTemplate, EmailWorkflow } from "../types";
import * as templateApi from "../api/emailTemplates";
import * as workflowApi from "../api/emailWorkflows";
import PageHeading from "../components/ui/PageHeading";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import Button from "../components/ui/Button";
import Modal from "../components/Modal";
import TemplateEditor from "../components/email/TemplateEditor";

// ─── Email chip input component ────────────────────────────────────

function EmailChipInput({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const [input, setInput] = useState("");

  const addEmail = (email: string) => {
    const trimmed = email.trim();
    if (!trimmed) return;
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmed)) return;
    if (value.includes(trimmed)) return;
    onChange([...value, trimmed]);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      addEmail(input);
      setInput("");
    }
  };

  const removeEmail = (email: string) => {
    onChange(value.filter((v) => v !== email));
  };

  return (
    <div className="flex flex-wrap items-center gap-1.5 rounded-sm border border-gray-300 p-2 min-h-[42px] focus-within:border-gray-500">
      {value.map((email) => (
        <span
          key={email}
          className="inline-flex items-center gap-1 rounded-sm bg-blue-50 px-2 py-0.5 text-xs text-blue-700"
        >
          {email}
          <button
            type="button"
            onClick={() => removeEmail(email)}
            className="text-blue-400 hover:text-blue-700 leading-none"
            aria-label={`Remove ${email}`}
          >
            &times;
          </button>
        </span>
      ))}
      <input
        type="email"
        value={input}
        onChange={(e) => setInput(e.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={() => { addEmail(input); setInput(""); }}
        className="flex-1 min-w-[140px] border-none outline-none text-sm p-0"
        placeholder={value.length === 0 ? "Type email and press Enter..." : "Add more..."}
      />
    </div>
  );
}

// ─── Template card ──────────────────────────────────────────────────

function TemplateCard({
  tmpl,
  workflowCounts,
  onEdit,
  onDelete,
}: {
  tmpl: EmailTemplate;
  workflowCounts: Record<string, number>;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="border border-gray-200 rounded-sm p-4 flex flex-col gap-2">
      <div className="flex items-start justify-between gap-2">
        <h3 className="text-sm font-semibold text-gray-900 truncate">{tmpl.name}</h3>
        <div className="flex gap-1 shrink-0">
          <button onClick={onEdit} className="text-xs text-gray-500 hover:text-gray-700 px-1 py-0.5">Edit</button>
          <button onClick={onDelete} className="text-xs text-red-500 hover:text-red-700 px-1 py-0.5">Delete</button>
        </div>
      </div>
      <p className="text-xs text-gray-500 truncate">{tmpl.subject}</p>
      <p className="text-xs text-gray-400 line-clamp-2">{tmpl.body.slice(0, 120)}</p>
      <div className="text-[10px] text-gray-400 mt-auto pt-1 border-t border-gray-100">
        {(workflowCounts[tmpl.id] ?? 0)} workflow(s)
      </div>
    </div>
  );
}

// ─── Workflow card ──────────────────────────────────────────────────

function WorkflowCard({
  wf,
  sending,
  disabled,
  onToggle,
  onSend,
  onEdit,
  onDelete,
}: {
  wf: EmailWorkflow;
  sending: boolean;
  disabled: boolean;
  onToggle: () => void;
  onSend: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="border border-gray-200 rounded-sm p-4 space-y-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <h3 className="text-sm font-semibold text-gray-900">{wf.name}</h3>
          <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded-sm ${wf.enabled ? "bg-green-100 text-green-700" : "bg-gray-100 text-gray-500"}`}>
            {wf.enabled ? "Enabled" : "Disabled"}
          </span>
        </div>
        <div className="flex gap-1">
          <button onClick={onEdit} className="text-xs text-gray-500 hover:text-gray-700 px-1 py-0.5">Edit</button>
          <button onClick={onDelete} className="text-xs text-red-500 hover:text-red-700 px-1 py-0.5">Delete</button>
        </div>
      </div>

      <div className="text-xs text-gray-600">
        <span className="font-medium">Template:</span> {wf.template_name}
      </div>

      <div>
        <span className="text-xs font-medium text-gray-600">Recipients:</span>
        <div className="flex flex-wrap gap-1 mt-1">
          {wf.recipients.length === 0 ? (
            <span className="text-xs text-gray-400">No recipients configured</span>
          ) : (
            wf.recipients.map((email) => (
              <span key={email} className="inline-block rounded-sm bg-blue-50 px-1.5 py-0.5 text-[11px] text-blue-700">
                {email}
              </span>
            ))
          )}
        </div>
      </div>

      <div className="flex items-center justify-between pt-1 border-t border-gray-100">
        <div className="text-[11px] text-gray-400">
          <p>{wf.trigger_description}</p>
          {wf.last_sent_at && (
            <p className="mt-0.5">Last sent: {new Date(wf.last_sent_at).toLocaleString()} &middot; {wf.last_sent_count} sent</p>
          )}
        </div>
        <div className="flex gap-2 items-center">
          <label className="flex items-center gap-1.5 cursor-pointer text-xs text-gray-600">
            <input
              type="checkbox"
              checked={wf.enabled}
              onChange={onToggle}
              className="w-3.5 h-3.5"
            />
            Auto
          </label>
          <Button variant="secondary" size="sm" onClick={onSend} loading={sending} disabled={disabled}>Send Now</Button>
        </div>
      </div>
    </div>
  );
}

// ─── Main page ──────────────────────────────────────────────────────

export default function EmailReminders() {
  const { addToast } = useToast();
  const [templates, setTemplates] = useState<EmailTemplate[]>([]);
  const [workflows, setWorkflows] = useState<EmailWorkflow[]>([]);
  const [loading, setLoading] = useState(true);
  const [sendingAll, setSendingAll] = useState(false);
  const [sendingWorkflows, setSendingWorkflows] = useState<Set<string>>(new Set());

  // Modal state
  const [templateModal, setTemplateModal] = useState<{ mode: "create"; showPresetPicker: boolean } | { mode: "edit"; id: string } | null>(null);
  const [workflowModal, setWorkflowModal] = useState<{ mode: "create"; defaultTemplateId?: string } | { mode: "edit"; id: string } | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<{ type: "template"; id: string; name: string } | { type: "workflow"; id: string; name: string } | null>(null);
  const [previewOpen, setPreviewOpen] = useState<{ subject: string; body: string } | null>(null);

  // ── Form state (template) ──
  const [formName, setFormName] = useState("");
  const [formSubject, setFormSubject] = useState("");
  const [formBody, setFormBody] = useState("");

  // ── Form state (workflow) ──
  const [wfName, setWfName] = useState("");
  const [wfTemplateId, setWfTemplateId] = useState("");
  const [wfRecipients, setWfRecipients] = useState<string[]>([]);
  const [wfTriggerDescription, setWfTriggerDescription] = useState("Daily at 08:00 (Asia/Bangkok)");
  const [wfEnabled, setWfEnabled] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const [templatesData, workflowsData] = await Promise.all([
        templateApi.listTemplates(),
        workflowApi.listWorkflows(),
      ]);
      setTemplates(templatesData);
      setWorkflows(workflowsData);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to load data");
    } finally {
      setLoading(false);
    }
  }, [addToast]);

  useEffect(() => { loadData(); }, [loadData]);

  // ── Workflow counts per template ──
  const workflowCounts: Record<string, number> = {};
  for (const wf of workflows) {
    workflowCounts[wf.template_id] = (workflowCounts[wf.template_id] ?? 0) + 1;
  }

  // ── Template actions ──
  const presets = templates.filter((t) => t.built_in);

  const openCreateTemplate = () => {
    setFormName("");
    setFormSubject("");
    setFormBody("");
    setTemplateModal({ mode: "create", showPresetPicker: true });
  };

  const selectPreset = (tmpl: EmailTemplate) => {
    setFormName("");
    setFormSubject(tmpl.subject);
    setFormBody(tmpl.body);
    setTemplateModal({ mode: "create", showPresetPicker: false });
  };

  const startFromScratch = () => {
    setFormName("");
    setFormSubject("");
    setFormBody("");
    setTemplateModal({ mode: "create", showPresetPicker: false });
  };

  const openEditTemplate = (tmpl: EmailTemplate) => {
    setFormName(tmpl.name);
    setFormSubject(tmpl.subject);
    setFormBody(tmpl.body);
    setTemplateModal({ mode: "edit", id: tmpl.id });
  };

  const saveTemplate = async () => {
    if (!formName.trim()) { addToast("error", "Template name is required"); return; }
    try {
      if (templateModal?.mode === "create") {
        await templateApi.createTemplate({ name: formName, subject: formSubject, body: formBody });
        addToast("success", "Template created");
      } else if (templateModal?.mode === "edit") {
        await templateApi.updateTemplate(templateModal.id, { name: formName, subject: formSubject, body: formBody });
        addToast("success", "Template updated");
      }
      setTemplateModal(null);
      await loadData();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Save failed");
    }
  };

  const deleteTemplate = async (id: string) => {
    try {
      await templateApi.deleteTemplate(id);
      addToast("success", "Template deleted");
      setDeleteConfirm(null);
      await loadData();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Delete failed");
    }
  };

  const openPreview = async () => {
    try {
      const result = await templateApi.previewTemplate(formSubject, formBody);
      setPreviewOpen(result);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Preview failed");
    }
  };

  // ── Workflow actions ──
  const openCreateWorkflow = (templateId?: string) => {
    setWfName("");
    setWfTemplateId(templateId ?? (templates[0]?.id ?? ""));
    setWfRecipients([]);
    setWfTriggerDescription("Daily at 08:00 (Asia/Bangkok)");
    setWfEnabled(false);
    setWorkflowModal({ mode: "create", defaultTemplateId: templateId });
  };

  const openEditWorkflow = (wf: EmailWorkflow) => {
    setWfName(wf.name);
    setWfTemplateId(wf.template_id);
    setWfRecipients([...wf.recipients]);
    setWfTriggerDescription(wf.trigger_description);
    setWfEnabled(wf.enabled);
    setWorkflowModal({ mode: "edit", id: wf.id });
  };

  const saveWorkflow = async () => {
    if (!wfName.trim()) { addToast("error", "Workflow name is required"); return; }
    if (!wfTemplateId) { addToast("error", "Please select a template"); return; }
    try {
      if (workflowModal?.mode === "create") {
        await workflowApi.createWorkflow({
          name: wfName,
          template_id: wfTemplateId,
          enabled: wfEnabled,
          trigger_description: wfTriggerDescription,
          recipients: wfRecipients,
        });
        addToast("success", "Workflow created");
      } else if (workflowModal?.mode === "edit") {
        await workflowApi.updateWorkflow(workflowModal.id, {
          name: wfName,
          template_id: wfTemplateId,
          enabled: wfEnabled,
          trigger_description: wfTriggerDescription,
          recipients: wfRecipients,
        });
        addToast("success", "Workflow updated");
      }
      setWorkflowModal(null);
      await loadData();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Save failed");
    }
  };

  const deleteWorkflow = async (id: string) => {
    try {
      await workflowApi.deleteWorkflow(id);
      addToast("success", "Workflow deleted");
      setDeleteConfirm(null);
      await loadData();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Delete failed");
    }
  };

  const toggleWorkflow = async (wf: EmailWorkflow) => {
    try {
      await workflowApi.updateWorkflow(wf.id, { enabled: !wf.enabled });
      await loadData();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Toggle failed");
    }
  };

  const sendWorkflow = async (id: string) => {
    setSendingWorkflows((prev) => new Set(prev).add(id));
    try {
      const result = await workflowApi.sendWorkflow(id);
      addToast("success", result.message);
      await loadData();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Send failed");
    } finally {
      setSendingWorkflows((prev) => { const next = new Set(prev); next.delete(id); return next; });
    }
  };

  const sendAll = async () => {
    setSendingAll(true);
    try {
      const result = await workflowApi.sendAllWorkflows();
      addToast("success", result.message);
      await loadData();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Send all failed");
    } finally {
      setSendingAll(false);
    }
  };

  if (loading) return <LoadingSkeleton type="text" lines={8} />;

  return (
    <div className="mx-auto max-w-4xl space-y-8">
      <div className="flex items-center justify-between">
        <div>
          <PageHeading>Email Reminders</PageHeading>
          <p className="text-sm text-gray-500 mt-1">
            Manage email templates and configure automated notification workflows.
          </p>
        </div>
        <Button variant="secondary" onClick={sendAll} loading={sendingAll}>
          Send All Workflows
        </Button>
      </div>

      {/* ── Templates section ── */}
      <section>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-base font-semibold text-gray-900">Templates</h2>
          <Button size="sm" onClick={openCreateTemplate}>+ New Template</Button>
        </div>
        {templates.length === 0 ? (
          <p className="text-sm text-gray-400">No templates yet. Create one to get started.</p>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
            {templates.map((tmpl) => (
              <TemplateCard
                key={tmpl.id}
                tmpl={tmpl}
                workflowCounts={workflowCounts}
                onEdit={() => openEditTemplate(tmpl)}
                onDelete={() => setDeleteConfirm({ type: "template", id: tmpl.id, name: tmpl.name })}
              />
            ))}
          </div>
        )}
      </section>

      {/* ── Workflows section ── */}
      <section>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-base font-semibold text-gray-900">Workflows</h2>
          <Button size="sm" onClick={() => openCreateWorkflow()}>+ New Workflow</Button>
        </div>
        {workflows.length === 0 ? (
          <p className="text-sm text-gray-400">No workflows yet. Create one to start sending emails.</p>
        ) : (
          <div className="space-y-3">
            {workflows.map((wf) => (
              <WorkflowCard
                key={wf.id}
                wf={wf}
                sending={sendingWorkflows.has(wf.id)}
                disabled={sendingAll || sendingWorkflows.size > 0}
                onToggle={() => toggleWorkflow(wf)}
                onSend={() => sendWorkflow(wf.id)}
                onEdit={() => openEditWorkflow(wf)}
                onDelete={() => setDeleteConfirm({ type: "workflow", id: wf.id, name: wf.name })}
              />
            ))}
          </div>
        )}
      </section>

      {/* ── Template create/edit modal ── */}
      {templateModal && (() => {
        if (templateModal.mode === "create" && templateModal.showPresetPicker) {
          return (
            <Modal title="New Template" onClose={() => setTemplateModal(null)} size="md">
              <div className="space-y-3">
                <p className="text-sm text-gray-600">Choose a starting point or start from scratch.</p>
                <button
                  onClick={startFromScratch}
                  className="w-full text-left rounded-sm border border-dashed border-gray-300 p-3 text-sm text-gray-500 hover:border-gray-500 hover:text-gray-700 transition-colors"
                >
                  <span className="font-medium text-gray-900">Start from scratch</span>
                  <p className="text-xs text-gray-400 mt-0.5">Blank template with no content</p>
                </button>
                {presets.map((tmpl) => (
                  <button
                    key={tmpl.id}
                    onClick={() => selectPreset(tmpl)}
                    className="w-full text-left rounded-sm border border-gray-200 p-3 hover:border-gray-400 transition-colors"
                  >
                    <span className="text-sm font-medium text-gray-900">{tmpl.name}</span>
                    <p className="text-xs text-gray-500 mt-0.5 line-clamp-1">{tmpl.subject}</p>
                    <p className="text-xs text-gray-400 mt-0.5 line-clamp-2">{tmpl.body.slice(0, 100)}</p>
                  </button>
                ))}
              </div>
            </Modal>
          );
        }

        return (
          <Modal
            title={templateModal.mode === "create" ? "New Template" : "Edit Template"}
            onClose={() => setTemplateModal(null)}
            footer={
              <div className="flex gap-2 w-full justify-between">
                <Button variant="secondary" size="sm" onClick={openPreview}>Preview</Button>
                <div className="flex gap-2">
                  <Button variant="secondary" size="sm" onClick={() => setTemplateModal(null)}>Cancel</Button>
                  <Button size="sm" onClick={saveTemplate}>
                    {templateModal.mode === "create" ? "Create" : "Save"}
                  </Button>
                </div>
              </div>
            }
            size="xl"
          >
            <TemplateEditor
              name={formName}
              onNameChange={setFormName}
              subject={formSubject}
              body={formBody}
              onChange={(s, b) => { setFormSubject(s); setFormBody(b); }}
            />
          </Modal>
        );
      })()}

      {/* ── Workflow create/edit modal ── */}
      {workflowModal && (
        <Modal
          title={workflowModal.mode === "create" ? "New Workflow" : "Edit Workflow"}
          onClose={() => setWorkflowModal(null)}
          footer={
            <div className="flex gap-2 justify-end w-full">
              <Button variant="secondary" size="sm" onClick={() => setWorkflowModal(null)}>Cancel</Button>
              <Button size="sm" onClick={saveWorkflow}>
                {workflowModal.mode === "create" ? "Create" : "Save"}
              </Button>
            </div>
          }
          size="lg"
        >
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Workflow name</label>
              <input
                type="text"
                value={wfName}
                onChange={(e) => setWfName(e.target.value)}
                className="w-full rounded-sm border border-gray-300 p-2 text-sm"
                placeholder="e.g. Sit-in Day Reminder"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Template</label>
              <select
                value={wfTemplateId}
                onChange={(e) => setWfTemplateId(e.target.value)}
                className="w-full rounded-sm border border-gray-300 p-2 text-sm"
              >
                <option value="">Select a template...</option>
                {templates.map((t) => (
                  <option key={t.id} value={t.id}>{t.name}</option>
                ))}
              </select>
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Recipients</label>
              <EmailChipInput value={wfRecipients} onChange={setWfRecipients} />
              <p className="text-xs text-gray-400 mt-1">Type email addresses and press Enter to add them.</p>
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Trigger description</label>
              <input
                type="text"
                value={wfTriggerDescription}
                onChange={(e) => setWfTriggerDescription(e.target.value)}
                className="w-full rounded-sm border border-gray-300 p-2 text-sm"
              />
              <p className="text-xs text-gray-400 mt-1">
                Describes when this workflow runs. Displayed to staff so they understand the schedule.
              </p>
            </div>

            <div className="flex items-center gap-3 pt-2">
              <label className="flex items-center gap-2 text-sm text-gray-700 cursor-pointer">
                <input
                  type="checkbox"
                  checked={wfEnabled}
                  onChange={(e) => setWfEnabled(e.target.checked)}
                  className="w-4 h-4"
                />
                Enable automatic execution
              </label>
            </div>

            {/* Trigger visual */}
            <div className="rounded-sm border border-gray-200 bg-gray-50 p-3">
              <p className="text-xs font-medium text-gray-700 mb-2">⏰ When this workflow runs</p>
              <div className="relative h-6">
                <div className="absolute inset-x-0 top-2.5 h-0.5 bg-gray-300" />
                <div className="absolute left-0 top-1.5 w-3 h-3 rounded-full bg-blue-500" />
                <div className="absolute right-0 top-1.5 w-3 h-3 rounded-full bg-gray-400" />
                <span className="absolute left-0 -bottom-4 text-[10px] text-gray-500">Now</span>
                <span className="absolute right-0 -bottom-4 text-[10px] text-gray-500">{wfTriggerDescription}</span>
              </div>
              <p className="text-[11px] text-gray-500 mt-5">
                This workflow triggers automatically at the schedule above. It sends the selected template
                to all configured recipients.
              </p>
            </div>
          </div>
        </Modal>
      )}

      {/* ── Preview modal ── */}
      {previewOpen && (
        <Modal
          title="Template Preview"
          onClose={() => setPreviewOpen(null)}
          size="lg"
        >
          <div className="space-y-3">
            <div>
              <span className="text-xs text-gray-400 font-medium">Subject:</span>
              <p className="text-sm font-medium text-gray-900 mt-0.5">{previewOpen.subject}</p>
            </div>
            <div>
              <span className="text-xs text-gray-400 font-medium">Body:</span>
              <div className="mt-0.5 text-sm text-gray-700 whitespace-pre-wrap rounded-sm border border-gray-100 bg-white p-3">
                {previewOpen.body}
              </div>
            </div>
          </div>
        </Modal>
      )}

      {/* ── Delete confirmation ── */}
      {deleteConfirm && (
        <Modal
          title={`Delete ${deleteConfirm.type}`}
          onClose={() => setDeleteConfirm(null)}
          footer={
            <div className="flex gap-2 justify-end w-full">
              <Button variant="secondary" size="sm" onClick={() => setDeleteConfirm(null)}>Cancel</Button>
              <Button
                variant="danger"
                size="sm"
                onClick={() => {
                  if (deleteConfirm.type === "template") deleteTemplate(deleteConfirm.id);
                  else deleteWorkflow(deleteConfirm.id);
                }}
              >
                Delete
              </Button>
            </div>
          }
          size="sm"
        >
          <p className="text-sm text-gray-600">
            Are you sure you want to delete <strong>{deleteConfirm.name}</strong>?
            {deleteConfirm.type === "template" && " Templates that are in use by workflows cannot be deleted."}
          </p>
        </Modal>
      )}
    </div>
  );
}
