import { useState } from "react";
import LoadingSkeleton from "../../components/ui/LoadingSkeleton";
import Button from "../../components/ui/Button";
import Modal from "../../components/Modal";
import RulePredicateForm from "../../components/RulePredicateForm";
import { RulePreviewPanel, RULE_TYPE_DESCRIPTIONS } from "../../components/RulePreviewPanel";
import { RuleExampleSection } from "../../components/RuleExampleSection";
import { useSitInRules } from "../../hooks/useSitInRules";
import type { SitInRuleType, SitInRuleCreateInput } from "../../types";

const RULE_TYPE_LABELS: Record<SitInRuleType, string> = {
  level_ladder: "Level Ladder",
  cross_section: "Cross-Section",
  any_day_except_last: "Any Day Except Last",
  rank_chain: "Rank Chain",
  teacher_case_by_case: "Teacher Case by Case",
};

const RULE_TYPE_OPTIONS: SitInRuleType[] = [
  "level_ladder",
  "cross_section",
  "any_day_except_last",
  "rank_chain",
  "teacher_case_by_case",
];

const EMPTY_FORM: SitInRuleCreateInput = {
  name: "",
  type: "level_ladder",
  predicate: {},
  description: "",
};

export function SitInRuleInventoryPage() {
  const { rules, loading, createRule, updateRule, deleteRule } = useSitInRules();
  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<SitInRuleCreateInput>(EMPTY_FORM);
  const [saving, setSaving] = useState(false);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);

  function openCreate() {
    setEditingId(null);
    setForm(EMPTY_FORM);
    setModalOpen(true);
  }

  function openEdit(id: string) {
    const rule = rules.find((r) => r.id === id);
    if (!rule) return;
    setEditingId(id);
    setForm({ name: rule.name, type: rule.type, predicate: rule.predicate, description: rule.description });
    setModalOpen(true);
  }

  async function handleSave() {
    setSaving(true);
    try {
      const input: SitInRuleCreateInput = {
        ...form,
      };
      if (editingId) {
        await updateRule(editingId, input);
      } else {
        await createRule(input);
      }
      setModalOpen(false);
    } catch {
      // toast handled by hook
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete() {
    if (!confirmDeleteId) return;
    setDeletingId(confirmDeleteId);
    try {
      await deleteRule(confirmDeleteId);
      setConfirmDeleteId(null);
    } catch {
      // toast handled by hook
    } finally {
      setDeletingId(null);
    }
  }

  if (loading) return <LoadingSkeleton type="card" lines={5} />;

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <p className="text-sm text-gray-500">Manage sit-in eligibility rules that determine which students can attend which sessions.</p>
        <Button variant="primary" size="sm" onClick={openCreate}>Create Rule</Button>
      </div>

      {rules.length === 0 ? (
        <p className="py-8 text-center text-sm text-gray-400">No rules configured.</p>
      ) : (
        <div className="rounded-sm border border-gray-200 bg-white shadow-sm overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200 bg-gray-50/70 text-left text-gray-500">
                <th className="px-4 py-2.5 font-medium">Name</th>
                <th className="px-4 py-2.5 font-medium">Type</th>
                <th className="px-4 py-2.5 font-medium">Description</th>
                <th className="px-4 py-2.5 font-medium text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {rules.map((rule) => (
                <tr key={rule.id} className="hover:bg-gray-50/50">
                  <td className="px-4 py-2.5 font-medium text-gray-800">{rule.name}</td>
                  <td className="px-4 py-2.5">
                    <span className="inline-block rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-600">
                      {RULE_TYPE_LABELS[rule.type]}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-gray-500 max-w-xs truncate">{rule.description}</td>
                  <td className="px-4 py-2.5 text-right">
                    <div className="flex items-center justify-end gap-2">
                      <Button variant="secondary" size="sm" onClick={() => openEdit(rule.id)}>Edit</Button>
                      <Button variant="secondary" size="sm" onClick={() => setConfirmDeleteId(rule.id)}>Delete</Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {modalOpen ? (
        <Modal
          title={editingId ? "Edit Rule" : "Create Rule"}
          onClose={() => setModalOpen(false)}
          size="xl"
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setModalOpen(false)}>Cancel</Button>
              <Button variant="primary" size="sm" loading={saving} onClick={handleSave}>
                {editingId ? "Save Changes" : "Create Rule"}
              </Button>
            </>
          }
        >
          <div className="space-y-5">
            <RulePreviewPanel form={form} />

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
              <input
                type="text"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="w-full rounded-sm border border-gray-300 px-3 py-2 text-sm focus:border-[var(--color-wi-primary)] focus:outline-none"
                placeholder="e.g. Level progression"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Type</label>
              <select
                value={form.type}
                onChange={(e) => setForm({ ...form, type: e.target.value as SitInRuleType })}
                className="w-full rounded-sm border border-gray-300 px-3 py-2 text-sm focus:border-[var(--color-wi-primary)] focus:outline-none"
              >
                {RULE_TYPE_OPTIONS.map((t) => (
                  <option key={t} value={t}>{RULE_TYPE_LABELS[t]} — {RULE_TYPE_DESCRIPTIONS[t].description}</option>
                ))}
              </select>
              <p className="mt-1 text-xs text-gray-500">
                {RULE_TYPE_DESCRIPTIONS[form.type].example}
              </p>
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Predicate</label>
              <RulePredicateForm
                ruleType={form.type}
                predicate={form.predicate}
                onChange={(predicate) => setForm({ ...form, predicate })}
              />
            </div>

            <RuleExampleSection form={form} />

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
              <textarea
                value={form.description}
                onChange={(e) => setForm({ ...form, description: e.target.value })}
                className="w-full rounded-sm border border-gray-300 px-3 py-2 text-sm focus:border-[var(--color-wi-primary)] focus:outline-none"
                rows={3}
                placeholder="Describe what this rule does"
              />
            </div>
          </div>
        </Modal>
      ) : null}

      {confirmDeleteId ? (
        <Modal
          title="Delete Rule"
          onClose={() => setConfirmDeleteId(null)}
          size="sm"
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setConfirmDeleteId(null)}>Cancel</Button>
              <Button variant="danger" size="sm" loading={deletingId === confirmDeleteId} onClick={handleDelete}>
                Delete
              </Button>
            </>
          }
        >
          <p className="text-sm text-gray-600">
            Are you sure you want to delete this rule? This action cannot be undone.
          </p>
        </Modal>
      ) : null}
    </div>
  );
}

export default SitInRuleInventoryPage;
