import type { ReactNode } from "react";

export type ClassesStepProps = { children: ReactNode };

export default function ClassesStep({ children }: ClassesStepProps) {
  return <><h1 className="text-2xl font-bold tracking-tight text-[var(--color-wi-text)]">Courses &amp; classes</h1>{children}</>;
}
