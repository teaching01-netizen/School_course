import type { ReactNode } from "react";
import StepCoverVerification from "@/components/absences/StepCoverVerification";

type VerificationStore = {
  code: string;
  setCode: (next: string) => void;
  token: string | null;
  persistToken: (nextToken: string, nextExpiresAt?: number | null) => void;
  clearStoredToken: () => void;
};

type ParentConfirmScreenProps = {
  studentName: string;
  wcode: string;
  lookupToken: string;
  hasPhone: boolean;
  phoneHint?: string;
  smsParentEnabled?: boolean;
  adminContact?: { email: string; phone: string; hours: string };
  verification: VerificationStore;
  completed: boolean;
  blocked?: boolean;
  onSatisfied: () => void;
  onRestart: () => void;
  onRestored: () => void;
};

function endingDigits(hint?: string): string {
  const digits = (hint ?? "").replace(/[•*]/g, "");
  return digits.length >= 2 ? digits.slice(-2) : "";
}

function Lead({ children }: { children: ReactNode }) {
  return <p className="text-[17px] leading-relaxed text-[var(--color-wi-text-light)]">{children}</p>;
}

export default function ParentConfirmScreen({
  studentName,
  wcode,
  lookupToken,
  hasPhone,
  phoneHint,
  smsParentEnabled = true,
  adminContact,
  verification,
  completed,
  blocked = false,
  onSatisfied,
  onRestart,
  onRestored,
}: ParentConfirmScreenProps) {
  const ending = endingDigits(phoneHint);
  const sessionStarted = Boolean(verification.token);

  return (
    <div className="mx-auto w-full max-w-xl">
      {completed ? (
        <div className="flex flex-col items-center py-10 text-center" role="status" aria-live="polite">
          <div className="flex h-14 w-14 items-center justify-center rounded-full bg-[var(--color-wi-green)]/10">
            <svg className="h-7 w-7 text-[var(--color-wi-green)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5} aria-hidden="true">
              <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
            </svg>
          </div>
          <h1 className="mt-5 text-[28px] font-bold leading-tight tracking-tight text-[var(--color-wi-text)]">
            Confirmed
          </h1>
        </div>
      ) : (
        <>
          <h1 className="text-[28px] font-bold leading-tight tracking-tight text-[var(--color-wi-text)]">
            {sessionStarted ? "Enter the code" : hasPhone ? "Confirm with a parent" : "Add a parent phone number"}
          </h1>

          {blocked ? (
            <div role="alert" className="mt-5 rounded-xl border border-[var(--color-wi-amber)]/30 bg-[var(--color-wi-amber-bg)] p-4 text-[15px] text-[var(--color-wi-amber)]">
              Your parent&apos;s confirmation expired. Please confirm again.
              <span className="mt-1 block">Your absence details are still saved.</span>
            </div>
          ) : null}

          <div className="mt-4 space-y-2">
            {hasPhone ? (
              <>
                <Lead>
                  {sessionStarted
                    ? `We sent it to the number ending in ${ending ? `••${ending}` : "your parent's phone"}.`
                    : ending
                      ? "We'll text a 6-digit code to the parent number ending in " + `••${ending}.`
                      : "We'll text a 6-digit code to your parent's phone."}
                </Lead>
                {!sessionStarted ? (
                  <p className="font-mono text-[22px] font-semibold tracking-widest text-[var(--color-wi-text)]">
                    {ending ? `••${ending}` : phoneHint}
                  </p>
                ) : null}
              </>
            ) : (
              <Lead>
                We need a parent phone number to confirm this absence.
                We&apos;ll only use it for school communication and absence confirmation.
              </Lead>
            )}
          </div>

          <div className="mt-6">
            <StepCoverVerification
              lookupToken={lookupToken}
              wcode={wcode}
              parentVerificationAvailable={hasPhone}
              smsParentEnabled={smsParentEnabled}
              adminContact={adminContact}
              verification={verification}
              completed={completed}
              onSatisfied={onSatisfied}
              onRestart={onRestart}
              onRestored={onRestored}
            />
          </div>
        </>
      )}

      <p className="sr-only">{studentName} · {wcode}</p>
    </div>
  );
}