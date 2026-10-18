import clsx from "clsx";

export type AbsenceStep = {
  label: string;
  description?: string;
};

type StepProgressProps = {
  steps: AbsenceStep[];
  currentStep: number;
  onStepClick?: (step: number) => void;
};

export default function StepProgress({ steps, currentStep, onStepClick }: StepProgressProps) {
  return (
    <nav aria-label="Progress" className="absence-step-progress">

      <ol role="list" className="absence-step-progress__steps">
        {steps.map((step, index) => {
          const isCompleted = index < currentStep;
          const isCurrent = index === currentStep;
          const isClickable = isCompleted && Boolean(onStepClick);

          return (
            <li key={step.label} className="absence-step-progress__item">
                <button
                  type="button"
                  tabIndex={isClickable ? 0 : -1}
                  onClick={() => { if (isClickable) onStepClick?.(index); }}
                  disabled={!isClickable && !isCurrent}
                  aria-current={isCurrent ? "step" : undefined}
                  aria-label={`${step.label}${isCompleted ? " - completed" : ""}${isCurrent ? " - current" : ""}`}
                  data-step-state={isCurrent ? "current" : isCompleted ? "completed" : "upcoming"}
                  className={clsx(
                    "absence-step-progress__step relative z-10 inline-flex min-h-11 items-center gap-2.5 rounded-xl text-sm font-semibold transition-colors motion-reduce:transition-none",
                    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2",
                    isCompleted && "text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-row-alt)]",
                    isCurrent && "text-[var(--color-wi-text)]",
                    !isCompleted && !isCurrent && "text-[var(--color-wi-text-light)]",
                    !isClickable && !isCurrent && "cursor-default",
                  )}
                >
                  <span className="absence-step-progress__number" aria-hidden="true">{index + 1}</span>
                  <span className="absence-step-progress__label">
                    <span>{step.label}</span>
                    {step.description ? <span className="absence-step-progress__description">{step.description}</span> : null}
                  </span>
                </button>
            </li>
          );
        })}
      </ol>

      <p className="absence-step-progress__position mt-3 text-center text-sm font-semibold text-[var(--color-wi-text)] sm:hidden">
        Step {currentStep + 1} of {steps.length}<span className="absence-step-progress__name">: {steps[currentStep]?.label}</span>
      </p>
    </nav>
  );
}
