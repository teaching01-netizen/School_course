import type { ReactNode } from "react";

export type ClassesStepProps = { children: ReactNode };

export default function ClassesStep({ children }: ClassesStepProps) {
  return <><header className="absence-step-heading"><h1 tabIndex={-1} className="absence-step-title text-2xl font-bold tracking-tight text-[var(--color-wi-text)]">Courses &amp; classes</h1><p className="absence-step-heading__help">Choose the courses and dates you’ll miss, then arrange any make-up classes.</p></header>{children}</>;
}
