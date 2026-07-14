import type { Ref } from "react";

export type FormAlertProps = { message: string; alertRef?: Ref<HTMLDivElement> };

export default function FormAlert({ message, alertRef }: FormAlertProps) {
  return <div ref={alertRef} tabIndex={-1} role="alert" className="mb-6 rounded-xl border border-[var(--color-wi-red)]/20 bg-[var(--color-wi-danger-bg)] p-4 text-base text-[var(--color-wi-red)] outline-none focus:ring-2 focus:ring-[var(--color-wi-red)]/30">{message}</div>;
}
