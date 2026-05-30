import { useEffect, useMemo, useRef, useState } from "react";
import { apiJson, ApiRequestError } from "@/api/client";
import Button from "@/components/ui/Button";
import LoadingSkeleton from "@/components/ui/LoadingSkeleton";
import OtpInput from "./OtpInput";
import CountdownTimer from "./CountdownTimer";
import type { AdminContactSettings, ParentVerificationResponse } from "@/types";

type VerificationStore = {
  code: string;
  setCode: (next: string) => void;
  token: string | null;
  persistToken: (nextToken: string, nextExpiresAt?: number | null) => void;
  clearStoredToken: () => void;
};

type StepCoverVerificationProps = {
  wcode: string;
  parentPhone?: string | null;
  allowSubmitWithoutOtp: boolean;
  adminContact?: AdminContactSettings;
  verification: VerificationStore;
  completed: boolean;
  onSatisfied: () => void;
  onWcodeChange?: () => void;
};

function maskPhone(phone?: string | null): string {
  if (!phone) return "";
  const digits = phone.replace(/\D/g, "");
  if (digits.length <= 4) return phone;
  return `${digits.slice(0, 3)} *** ${digits.slice(-3)}`;
}

function formatTime(iso?: string | null): string | null {
  if (!iso) return null;
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return null;
  return date.toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function isRetryable(err: unknown): boolean {
  if (err instanceof ApiRequestError) {
    return !err.status || err.status >= 500;
  }
  return err instanceof TypeError;
}

export default function StepCoverVerification({
  wcode,
  parentPhone,
  allowSubmitWithoutOtp,
  adminContact,
  verification,
  completed,
  onSatisfied,
  onWcodeChange,
}: StepCoverVerificationProps) {
  const [session, setSession] = useState<ParentVerificationResponse | null>(null);
  const [resumeLoading, setResumeLoading] = useState(false);
  const [resumeError, setResumeError] = useState<string | null>(null);
  const [sendError, setSendError] = useState<string | null>(null);
  const [verifyError, setVerifyError] = useState<string | null>(null);
  const [sendCount, setSendCount] = useState(0);
  const [isSending, setIsSending] = useState(false);
  const [isVerifying, setIsVerifying] = useState(false);
  const [resendAvailableIn, setResendAvailableIn] = useState(0);
  const autoVerifyCodeRef = useRef<string | null>(null);

  const phoneLabel = useMemo(() => maskPhone(parentPhone), [parentPhone]);
  const verified = completed || session?.status === "verified" || session?.status === "consumed";

  useEffect(() => {
    let cancelled = false;

    if (!verification.token) {
      setSession(null);
      setResumeError(null);
      setResumeLoading(false);
      return () => {
        cancelled = true;
      };
    }

    setResumeLoading(true);
    setResumeError(null);
    void apiJson<ParentVerificationResponse>(`/api/v1/absences/parent-verification/${encodeURIComponent(verification.token)}`, {
      method: "GET",
    })
      .then((response) => {
        if (cancelled) return;
        setSession(response);
        if (response.status === "verified" || response.status === "consumed") {
          onSatisfied();
        }
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setResumeError(err instanceof Error ? err.message : "Saved verification could not be restored");
        verification.clearStoredToken();
        verification.setCode("");
      })
      .finally(() => {
        if (!cancelled) setResumeLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [onSatisfied, verification.clearStoredToken, verification.setCode, verification.token]);

  useEffect(() => {
    if (resendAvailableIn <= 0) return;
    const timer = window.setInterval(() => {
      setResendAvailableIn((current) => Math.max(0, current - 1));
    }, 1000);
    return () => window.clearInterval(timer);
  }, [resendAvailableIn]);

  useEffect(() => {
    if (verified || isSending || isVerifying || !verification.token) {
      return;
    }

    const normalized = verification.code.replace(/\D/g, "").slice(0, 6);
    if (normalized.length !== 6) {
      autoVerifyCodeRef.current = null;
      return;
    }
    if (autoVerifyCodeRef.current === normalized) {
      return;
    }
    autoVerifyCodeRef.current = normalized;
    void handleVerify();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [verification.code, verification.token, verified, isSending, isVerifying]);

  async function handleSend() {
    if (!wcode || !parentPhone) return;
    setIsSending(true);
    setSendError(null);
    setVerifyError(null);
    try {
      const response = await apiJson<ParentVerificationResponse>("/api/v1/absences/parent-verification/send", {
        method: "POST",
        body: JSON.stringify({
          wcode,
          ...(verification.token ? { token: verification.token } : {}),
        }),
      });
      setSession(response);
      verification.persistToken(response.token, response.expires_at ? Date.parse(response.expires_at) : null);
      verification.setCode("");
      setResendAvailableIn(60);
      setSendCount((current) => current + 1);
    } catch (err) {
      setSendCount((current) => current + 1);
      setSendError(err instanceof Error ? err.message : "Could not send verification code");
      if (allowSubmitWithoutOtp) {
        setResumeError("Verification is optional for this submission, so you can continue without a code.");
      }
    } finally {
      setIsSending(false);
    }
  }

  async function handleVerify() {
    if (!verification.token || verification.code.length !== 6) return;
    setIsVerifying(true);
    setVerifyError(null);
    try {
      const response = await apiJson<ParentVerificationResponse>("/api/v1/absences/parent-verification/verify", {
        method: "POST",
        body: JSON.stringify({
          token: verification.token,
          code: verification.code,
        }),
      });
      setSession(response);
      verification.persistToken(response.token, response.expires_at ? Date.parse(response.expires_at) : null);
      onSatisfied();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Verification failed";
      setVerifyError(message);
      if (!isRetryable(err)) {
        verification.setCode("");
      }
    } finally {
      setIsVerifying(false);
    }
  }

  function handleSkip() {
    verification.clearStoredToken();
    verification.setCode("");
    setSession(null);
    setSendError(null);
    setVerifyError(null);
    setResumeError(null);
    onSatisfied();
  }

  const parentMissing = !parentPhone || parentPhone.trim() === "";
  const canSend = !isSending && !isVerifying && !parentMissing && !verified;
  const canVerify = !isSending && !isVerifying && verification.code.length === 6 && !!verification.token && !verified;
  const canSkip = allowSubmitWithoutOtp && !verified;

  return (
    <section className="space-y-5 rounded-sm border border-gray-200 bg-white p-5 shadow-sm">
      <header className="space-y-2">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 className="text-lg font-semibold text-[var(--color-wi-text)]">Parent verification</h3>
            <p className="text-sm text-gray-600">
              Enter the W-code first, then verify the parent phone before continuing with the absence details.
            </p>
          </div>
          {onWcodeChange ? (
            <Button variant="secondary" size="sm" onClick={onWcodeChange}>
              Change W-code
            </Button>
          ) : null}
        </div>
      </header>

      {resumeLoading ? (
        <LoadingSkeleton type="text" lines={2} />
      ) : null}

      {resumeError ? (
        <div role="alert" className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-900">
          {resumeError}
        </div>
      ) : null}

      <div className="rounded-sm border border-gray-200 bg-gray-50 p-4 text-sm text-gray-700">
        <div className="grid gap-2 sm:grid-cols-2">
          <div>
            <div className="text-xs uppercase tracking-wide text-gray-500">W-code</div>
            <div className="font-mono text-sm">{wcode}</div>
          </div>
          <div>
            <div className="text-xs uppercase tracking-wide text-gray-500">Parent phone</div>
            <div>{phoneLabel || "No parent phone on file"}</div>
          </div>
        </div>
      </div>

      {parentMissing ? (
        <div role="alert" className="rounded-sm border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
          No parent phone number is on file for this student.
          {allowSubmitWithoutOtp ? (
            <div className="mt-2">You can continue without verification because the policy allows it.</div>
          ) : (
            <div className="mt-2">
              Contact the school office before submitting.
              {adminContact?.email ? (
                <div className="mt-1">
                  <a className="font-medium underline" href={`mailto:${adminContact.email}`}>
                    {adminContact.email}
                  </a>
                </div>
              ) : null}
            </div>
          )}
        </div>
      ) : null}

      {verified ? (
        <div className="rounded-sm border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-900">
          <div className="font-semibold">Verification complete</div>
          <p className="mt-1">
            {session?.status === "consumed"
              ? "This verification was already used for a previous submission."
              : session
                ? "The parent has successfully confirmed this absence request."
                : "Verification was skipped, so you can continue with the form."}
          </p>
          {session?.verified_at ? (
            <p className="mt-1 text-emerald-800">Verified at {formatTime(session.verified_at) ?? "the recorded time"}.</p>
          ) : null}
        </div>
      ) : (
        <div className="space-y-4">
          {sendError ? (
            <div role="alert" className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-900">
              {sendError}
            </div>
          ) : null}

          {verifyError ? (
            <div role="alert" className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-900">
              {verifyError}
            </div>
          ) : null}

          <div className="flex flex-wrap items-center gap-3">
            <Button
              variant="primary"
              loading={isSending}
              onClick={() => void handleSend()}
              disabled={!canSend}
            >
              {sendCount > 0 ? "Resend verification code" : "Send verification code"}
            </Button>

            {canSkip ? (
              <Button variant="secondary" onClick={handleSkip}>
                Continue without verification
              </Button>
            ) : null}
          </div>

          {verification.token ? (
            <div className="space-y-4 rounded-sm border border-gray-200 bg-white p-4">
              <OtpInput
                value={verification.code}
                onChange={verification.setCode}
                disabled={isSending || isVerifying}
                error={!!verifyError}
                autoFocus={sendCount > 0}
                label="Verification code"
              />

              <div className="flex flex-wrap items-center justify-between gap-3">
                <CountdownTimer
                  secondsLeft={resendAvailableIn}
                  label={sendCount > 0 ? "Resend available in" : "Cooldown"}
                />
                <Button
                  variant="primary"
                  onClick={() => void handleVerify()}
                  loading={isVerifying}
                  disabled={!canVerify}
                >
                  Verify code
                </Button>
              </div>

              {session?.otp_code_expires_at ? (
                <p className="text-xs text-gray-500">
                  The current code expires at {formatTime(session.otp_code_expires_at) ?? "the recorded time"}.
                </p>
              ) : null}
            </div>
          ) : (
            <p className="text-xs text-gray-500">
              Send the verification code once you are ready to confirm the parent phone.
            </p>
          )}
        </div>
      )}
    </section>
  );
}
