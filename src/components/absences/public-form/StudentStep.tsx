import type { ReactNode } from "react";

export type StudentStepProps = { children: ReactNode };

export default function StudentStep({ children }: StudentStepProps) {
  return <><h1 className="text-2xl font-bold tracking-tight text-[var(--color-wi-text)]">Find your profile</h1>{children}</>;
}
