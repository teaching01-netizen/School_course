import { useEffect, useRef, useState } from "react";
import { apiJson, ApiRequestError } from "@/api/client";
import Button from "@/components/ui/Button";
import LoadingSkeleton from "@/components/ui/LoadingSkeleton";
import OtpInput from "./OtpInput";
import CountdownTimer from "./CountdownTimer";
import { CheckCircle, AlertCircle } from "lucide-react";
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
        setResumeError("Verification is optional — you can continue without a code.");
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
    <div className="space-y-4">
      <header className="space-y-2">
        <p className="text-xs text-gray-600">
          Verify the parent phone before continuing with the absence request.
        </p>
        {onWcodeChange ? (
          <div>
            <Button variant="ghost" size="sm" onClick={onWcodeChange}>
              Change W-code
            </Button>
          </div>
        ) : null}
      </header>

      {resumeLoading ? (
        <LoadingSkeleton type="text" lines={2} />
      ) : null}

      {resumeError ? (
        <div role="alert" className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-900 flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 flex-shrink-0" aria-hidden="true" />
          <span>{resumeError}</span>
        </div>
      ) : null}

      {parentMissing ? (
        <div role="alert" className="rounded-sm border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
          Your parent's phone number is not in our records.
          {allowSubmitWithoutOtp ? (
            <div className="mt-2 text-xs">You can continue without verification because the policy allows it.</div>
          ) : (
            <div className="mt-2 text-xs">
              Please contact the school office before continuing.
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
        session ? (
          <div className="rounded-sm border border-emerald-250 bg-emerald-50 p-4 text-sm text-emerald-900 animate-fade-in">
            <div className="font-semibold text-emerald-800">Verification complete</div>
            <p className="mt-1 text-xs text-emerald-800 font-medium">
              Parent confirmed via SMS code.
            </p>
            {session.verified_at ? (
              <p className="mt-1 text-xs text-emerald-700">Verified at {formatTime(session.verified_at) ?? "the recorded time"}.</p>
            ) : null}
          </div>
        ) : (
          <div className="rounded-sm border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 animate-fade-in">
            <div className="font-semibold text-amber-800">Verification complete</div>
            <p className="mt-1 text-xs text-amber-700 font-medium">
              Verification was skipped — parent was not contacted.
            </p>
          </div>
        )
      ) : (
        <div className="space-y-4">
          {sendError ? (
            <div role="alert" className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-900 flex items-start gap-2">
            <AlertCircle className="h-4 w-4 mt-0.5 flex-shrink-0" aria-hidden="true" />
            <span>{sendError}</span>
            </div>
          ) : null}

          {verifyError ? (
            <div role="alert" className="rounded-sm border border-red-200 bg-red-50 p-4 text-sm text-red-900 flex items-start gap-2">
            <AlertCircle className="h-4 w-4 mt-0.5 flex-shrink-0" aria-hidden="true" />
            <span>{verifyError}</span>
            </div>
          ) : null}

          <div className="flex flex-wrap items-center gap-3">
            <Button
              variant="primary"
              loading={isSending}
              onClick={() => void handleSend()}
              disabled={!canSend}
            >
              {sendCount > 0 ? "Resend code" : "Send code"}
            </Button>

            {canSkip ? (
              <Button variant="secondary" onClick={handleSkip}>
                Continue without confirming
              </Button>
            ) : null}
          </div>

          {verification.token ? (
            <div className="space-y-3">
              <OtpInput
                value={verification.code}
                onChange={verification.setCode}
                disabled={isSending || isVerifying}
                error={!!verifyError}
                autoFocus={sendCount > 0}
                label="Verification code"
              />

              <Button
                variant="primary"
                size="lg"
                onClick={() => void handleVerify()}
                loading={isVerifying}
                disabled={!canVerify}
                className="w-full"
              >
                <CheckCircle className="h-4 w-4 mr-1.5" />
                Verify code
              </Button>

              <div className="flex items-center justify-between text-xs text-gray-500">
                <CountdownTimer
                  secondsLeft={resendAvailableIn}
                  label={sendCount > 0 ? "Resend available in" : "Cooldown"}
                />
                {session?.otp_code_expires_at ? (
                  <span>
                    Code expires at {formatTime(session.otp_code_expires_at) ?? "the recorded time"}.
                  </span>
                ) : null}
              </div>
            </div>
          ) : (
            <p className="text-xs text-gray-600">
              Send the code once you're ready for your parent to confirm.
            </p>
          )}
        </div>
      )}
    </div>
  );
}
