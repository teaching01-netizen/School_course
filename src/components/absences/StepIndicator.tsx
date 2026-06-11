import clsx from "clsx";

type Step = {
  label: string;
  description?: string;
};

type StepIndicatorProps = {
  steps: Step[];
  currentStep: number;
  onStepClick?: (step: number) => void;
};

function CheckIcon() {
  return (
    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}

export default function StepIndicator({ steps, currentStep, onStepClick }: StepIndicatorProps) {
  return (
    <nav aria-label="Progress" className="mb-8">
      <ol role="list" className="flex items-center">
        {steps.map((step, index) => {
          const isCompleted = index < currentStep;
          const isCurrent = index === currentStep;
          const isClickable = isCompleted && onStepClick;

          return (
            <li
              key={step.label}
              className={clsx(
                "relative flex items-center",
                index < steps.length - 1 && "flex-1",
              )}
            >
              <div className="flex items-center gap-2.5">
                {/* Circle */}
                <button
                  type="button"
                  tabIndex={isClickable ? 0 : -1}
                  onClick={() => isClickable && onStepClick(index)}
                  disabled={!isClickable}
                  className={clsx(
                    "relative z-10 flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm font-semibold transition-all duration-200",
                    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2",
                    isCompleted && "bg-[var(--color-wi-primary)] text-white",
                    isCurrent && "border-2 border-[var(--color-wi-primary)] bg-white text-[var(--color-wi-primary)]",
                    !isCompleted && !isCurrent && "border-2 border-gray-300 bg-white text-gray-400",
                    isClickable && "cursor-pointer hover:shadow-sm",
                    !isClickable && "cursor-default",
                  )}
                  aria-current={isCurrent ? "step" : undefined}
                  aria-label={`${step.label}${isCompleted ? " - completed" : ""}${isCurrent ? " - current" : ""}`}
                >
                  {isCompleted ? <CheckIcon /> : <span>{index + 1}</span>}
                </button>

                {/* Label - visible on md+, sr-only on mobile */}
                <div className="hidden flex-col sm:flex">
                  <span
                    className={clsx(
                      "text-xs font-semibold leading-tight",
                      isCompleted && "text-[var(--color-wi-primary)]",
                      isCurrent && "text-[var(--color-wi-text)]",
                      !isCompleted && !isCurrent && "text-gray-400",
                    )}
                  >
                    {step.label}
                  </span>
                  {step.description && (
                    <span className="text-[11px] leading-tight text-gray-500">{step.description}</span>
                  )}
                </div>
              </div>

              {/* Connector line */}
              {index < steps.length - 1 && (
                <div
                  className={clsx(
                    "mx-3 h-px flex-1 transition-colors duration-200 sm:mx-4",
                    isCompleted ? "bg-[var(--color-wi-primary)]" : "bg-gray-300",
                  )}
                  aria-hidden="true"
                />
              )}
            </li>
          );
        })}
      </ol>

      {/* Mobile: current step label */}
      <p className="mt-2 text-center text-sm font-semibold text-[var(--color-wi-text)] sm:hidden">
        {steps[currentStep]?.label}
      </p>
    </nav>
  );
}
