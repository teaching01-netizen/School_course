import type { ReactNode } from "react";

export type ReviewStepProps = { children: ReactNode };

export default function ReviewStep({ children }: ReviewStepProps) {
  return <><header className="absence-step-heading"><h1 tabIndex={-1} className="absence-step-title text-2xl font-bold tracking-tight text-[var(--color-wi-text)]">Review your absence</h1><p className="absence-step-heading__help">Check the dates and make-up details before sending your request.</p></header>{children}</>;
}
