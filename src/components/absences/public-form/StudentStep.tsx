import type { ReactNode } from "react";

export type StudentStepProps = { children: ReactNode; publicForm?: boolean };

export default function StudentStep({ children, publicForm = false }: StudentStepProps) {
  return <><h1 tabIndex={-1} className="absence-step-title text-2xl font-bold tracking-tight text-[var(--color-wi-text)]">{publicForm ? "Who will be absent?" : "Find your profile"}</h1>{children}</>;
}
