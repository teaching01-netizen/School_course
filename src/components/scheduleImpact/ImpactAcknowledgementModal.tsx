import { useState } from "react";
import Modal from "../Modal";
import Button from "../ui/Button";

export type ImpactSummary = {
  direct_sit_in_assignments?: number;
  missed_session_references?: number;
  predicted_student_overlaps?: number;
  potential_eligibility_changes?: number;
  short_notice?: boolean;
};

interface ImpactAcknowledgementModalProps {
  summary: ImpactSummary;
  saving?: boolean;
  onBack: () => void;
  onConfirm: () => void;
}

function impactRows(summary: ImpactSummary): string[] {
  const rows: string[] = [];
  if (summary.direct_sit_in_assignments) rows.push(`${summary.direct_sit_in_assignments} sit-in arrangement${summary.direct_sit_in_assignments === 1 ? "" : "s"} will be reviewed`);
  if (summary.missed_session_references) rows.push(`${summary.missed_session_references} missed-session reference${summary.missed_session_references === 1 ? "" : "s"} may change`);
  if (summary.predicted_student_overlaps) rows.push(`${summary.predicted_student_overlaps} possible student timetable overlap${summary.predicted_student_overlaps === 1 ? "" : "s"} need review`);
  if (summary.potential_eligibility_changes) rows.push(`${summary.potential_eligibility_changes} eligibility check${summary.potential_eligibility_changes === 1 ? "" : "s"} may change`);
  if (summary.short_notice) rows.push("This is a short-notice change and affected students may need prompt contact");
  return rows.length > 0 ? rows : ["Affected student arrangements will be checked after this change is saved"];
}

export default function ImpactAcknowledgementModal({ summary, saving = false, onBack, onConfirm }: ImpactAcknowledgementModalProps) {
  const [acknowledged, setAcknowledged] = useState(false);
  return (
    <Modal
      title="Review schedule impact"
      onClose={onBack}
      size="md"
      footer={<><Button variant="secondary" size="sm" onClick={onBack} disabled={saving}>Back to editing</Button><Button size="sm" loading={saving} disabled={!acknowledged} onClick={onConfirm}>Save and review impact</Button></>}
    >
      <p className="text-sm text-gray-700">This schedule change affects existing student arrangements. Saving it will create or refresh work in Schedule Impact.</p>
      <ul className="mt-4 space-y-2 rounded-sm border border-amber-200 bg-amber-50 p-3 text-sm text-amber-950">
        {impactRows(summary).map((row) => <li key={row} className="flex gap-2"><span aria-hidden="true">•</span><span>{row}</span></li>)}
      </ul>
      <label className="mt-4 flex cursor-pointer items-start gap-2 text-sm text-gray-800">
        <input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} className="mt-0.5" />
        <span>I understand that affected arrangements require review before students are contacted.</span>
      </label>
    </Modal>
  );
}
