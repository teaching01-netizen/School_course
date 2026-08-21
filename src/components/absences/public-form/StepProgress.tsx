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
              <div className="flex items-center gap-2.5">
                <button
                  type="button"
                  tabIndex={isClickable ? 0 : -1}
                  onClick={() => { if (isClickable) onStepClick?.(index); }}
                  disabled={!isClickable && !isCurrent}
                  aria-current={isCurrent ? "step" : undefined}
                  aria-label={`${step.label}${isCompleted ? " - completed" : ""}${isCurrent ? " - current" : ""}`}
                  className={clsx(
                    "relative z-10 inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-semibold transition-colors motion-reduce:transition-none",
                    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2",
                    isCompleted && "bg-[var(--color-wi-primary)] text-white hover:bg-[var(--color-wi-primary-dark)]",
                    isCurrent && "border-2 border-[var(--color-wi-primary)] bg-white text-[var(--color-wi-primary)]",
                    !isCompleted && !isCurrent && "border-2 border-[var(--color-wi-line)] bg-white text-[var(--color-wi-text-light)]",
                    !isClickable && !isCurrent && "cursor-default",
                  )}
                >
                  {index + 1}
                </button>
                <div className="hidden min-w-0 flex-col sm:flex">
                  <span className={clsx(
                    "text-xs font-semibold leading-tight",
                    isCompleted && "text-[var(--color-wi-primary)]",
                    isCurrent && "text-[var(--color-wi-text)]",
                    !isCompleted && !isCurrent && "text-[var(--color-wi-text-light)]",
                  )}>
                    {step.label}
                  </span>
                  {step.description ? (
                    <span className="hidden text-[11px] leading-tight text-[var(--color-wi-text-light)] md:block">{step.description}</span>
                  ) : null}
                </div>
              </div>
            </li>
          );
        })}
      </ol>

      <p className="mt-3 text-center text-sm font-semibold text-[var(--color-wi-text)] sm:hidden">
        Step {currentStep + 1} of {steps.length}: {steps[currentStep]?.label}
      </p>
    </nav>
  );
}
