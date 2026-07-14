import type { ReactNode } from "react";
import clsx from "clsx";

export type VerificationStepProps = {
  studentName: string;
  wcode: string;
  phoneLabel: string;
  hasPhone: boolean;
  children: ReactNode;
};

export default function VerificationStep({ studentName, wcode, phoneLabel, hasPhone, children }: VerificationStepProps) {
  return <>
    <h1 className="text-2xl font-bold tracking-tight text-[var(--color-wi-text)]">Parent verification</h1>
    <div className="space-y-4">
      <div className="rounded-xl border border-[var(--color-wi-border)] bg-white p-5 shadow-sm">
        <p className="text-sm text-[var(--color-wi-text-light)]">Student</p>
        <p className="mt-1 text-base font-semibold text-[var(--color-wi-text)]">{studentName}</p>
        <p className="text-sm font-mono text-[var(--color-wi-text-light)]">{wcode}</p>
        <p className={clsx("mt-3 text-sm", hasPhone ? "text-[var(--color-wi-text-light)]" : "text-[var(--color-wi-amber)]")}>{phoneLabel}</p>
      </div>
      {children}
    </div>
  </>;
}
