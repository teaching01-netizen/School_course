import StepProgress, { type AbsenceStep } from "./StepProgress";

type AbsenceAppHeaderProps = {
  steps: AbsenceStep[];
  currentStep: number;
  onStepClick?: (step: number) => void;
};

export default function AbsenceAppHeader({ steps, currentStep, onStepClick }: AbsenceAppHeaderProps) {
  return (
    <div className="absence-app-header">
      <div className="absence-app-header__identity">
        <p className="text-xs font-semibold uppercase tracking-[0.16em] text-[var(--color-wi-primary)]">Warwick Institute</p>
        <p className="mt-1 text-sm font-medium text-[var(--color-wi-text-light)]">Student absence</p>
      </div>
      <StepProgress steps={steps} currentStep={currentStep} onStepClick={onStepClick} />
    </div>
  );
}
