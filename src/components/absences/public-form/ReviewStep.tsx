import type { ReactNode } from "react";

export type ReviewStepProps = { children: ReactNode };

export default function ReviewStep({ children }: ReviewStepProps) {
  return <><h1 className="text-2xl font-bold tracking-tight text-[var(--color-wi-text)]">Review your absence</h1>{children}</>;
}
