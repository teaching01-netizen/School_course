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
  const totalAffected = (summary.direct_sit_in_assignments ?? 0) + (summary.missed_session_references ?? 0) + (summary.predicted_student_overlaps ?? 0) + (summary.potential_eligibility_changes ?? 0);
  return (
    <Modal
      title="Review student impact"
      onClose={onBack}
      size="md"
      footer={<><Button variant="secondary" size="sm" onClick={onBack} disabled={saving}>Back to editing</Button><Button size="sm" loading={saving} onClick={onConfirm}>Save change and review {totalAffected}</Button></>}
    >
      <p className="text-sm text-gray-700">This change may affect {totalAffected} student arrangement{totalAffected === 1 ? "" : "s"}.</p>
      <ul className="mt-4 space-y-2 rounded-sm border border-amber-200 bg-amber-50 p-3 text-sm text-amber-950">
        {impactRows(summary).map((row) => <li key={row} className="flex gap-2"><span aria-hidden="true">•</span><span>{row}</span></li>)}
      </ul>
      <p className="mt-4 text-sm text-gray-600">Students will not be contacted until an administrator reviews the results.</p>
    </Modal>
  );
}
