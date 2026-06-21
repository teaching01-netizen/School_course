import { useEffect, useRef, useState } from "react";
import { motion } from "framer-motion";
import { apiJson, ApiRequestError } from "@/api/client";
import OtpInput from "./OtpInput";
import SmsSendButton from "./SmsSendButton";
import type { ParentVerificationResponse } from "@/types";

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
  adminContact?: { email: string; phone: string; hours: string };
  verification: VerificationStore;
  completed: boolean;
  onSatisfied: () => void;
  onRestart: () => void;
  onRestored: () => void;
};

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
  verification,
  completed,
  onSatisfied,
  onRestart,
  onRestored,
}: StepCoverVerificationProps) {
  const [session, setSession] = useState<ParentVerificationResponse | null>(null);
  const [sendError, setSendError] = useState<string | null>(null);
  const [verifyError, setVerifyError] = useState<string | null>(null);
  const [sendCount, setSendCount] = useState(0);
  const [isSending, setIsSending] = useState(false);
  const [isVerifying, setIsVerifying] = useState(false);
  const [lastSentAt, setLastSentAt] = useState<number | null>(null);
  const [restoreError, setRestoreError] = useState<string | null>(null);
  const [validationAttempt, setValidationAttempt] = useState(0);
  const autoVerifyCodeRef = useRef<string | null>(null);

  const verified = completed || session?.status === "verified" || session?.status === "consumed";

  useEffect(() => {
    if (!verification.token || !wcode) {
      setRestoreError(null);
      return;
    }
    const controller = new AbortController();
    setRestoreError(null);
    void apiJson<ParentVerificationResponse>(
      `/api/v1/absences/parent-verification/${encodeURIComponent(verification.token)}`,
      { method: "GET", signal: controller.signal },
    )
      .then((response) => {
        if (controller.signal.aborted) return;
        if (response.wcode !== wcode || response.status === "consumed") {
          setSession(null);
          onRestart();
          return;
        }
        setSession(response);
        if (response.status === "verified") onRestored();
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        if (error instanceof ApiRequestError && (error.status === 400 || error.status === 410)) {
          setSession(null);
          onRestart();
          return;
        }
        setRestoreError("Could not validate saved verification. Check your connection and try again.");
      });
    return () => controller.abort();
  }, [verification.token, wcode, validationAttempt, onRestart, onRestored]);

  useEffect(() => {
    if (verified || isSending || isVerifying || !verification.token) return;
    const normalized = verification.code.replace(/\D/g, "").slice(0, 6);
    if (normalized.length !== 6) { autoVerifyCodeRef.current = null; return; }
    if (autoVerifyCodeRef.current === normalized) return;
    autoVerifyCodeRef.current = normalized;
    void handleVerify();
  }, [verification.code, verification.token, verified, isSending, isVerifying]);

  async function handleSend(startNewSession = false) {
    if (!wcode || !parentPhone) return;
    if (startNewSession) {
      onRestart();
      setSession(null);
    }
    setIsSending(true);
    setSendError(null);
    setVerifyError(null);
    try {
      const response = await apiJson<ParentVerificationResponse>("/api/v1/absences/parent-verification/send", {
        method: "POST",
        body: JSON.stringify({ wcode, ...(!startNewSession && verification.token ? { token: verification.token } : {}) }),
      });
      setSession(response);
      verification.persistToken(response.token, response.expires_at ? Date.parse(response.expires_at) : null);
      verification.setCode("");
      setLastSentAt(Date.now());
      setSendCount((c) => c + 1);
    } catch (err) {
      setSendCount((c) => c + 1);
      setSendError(err instanceof Error ? err.message : "Could not send verification code");
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
        body: JSON.stringify({ token: verification.token, code: verification.code }),
      });
      setSession(response);
      verification.persistToken(response.token, response.expires_at ? Date.parse(response.expires_at) : null);
      onSatisfied();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Verification failed";
      setVerifyError(message);
      if (!isRetryable(err)) verification.setCode("");
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
    onSatisfied();
  }

  const parentMissing = !parentPhone || parentPhone.trim() === "";
  const canSend = !isSending && !isVerifying && !parentMissing && !verified;

  if (verified) {
    return (
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-green-600 font-medium">✓ Verified</p>
        <button
          type="button"
          onClick={() => void handleSend(true)}
          className="text-xs font-semibold text-[var(--color-wi-primary)] hover:text-[var(--color-wi-primary-dark)]"
        >
          Send new code
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {parentMissing ? (
        <p role="alert" className="text-xs text-amber-600">
          Your parent's phone number is not in our records.
          {!allowSubmitWithoutOtp ? " Contact admin before continuing." : null}
        </p>
      ) : null}

      {sendError ? (
        <p role="alert" className="text-xs text-red-600">{sendError}</p>
      ) : null}
      {restoreError ? (
        <div role="alert" className="space-y-1 text-xs text-red-600">
          <p>{restoreError}</p>
          <button
            type="button"
            onClick={() => setValidationAttempt((attempt) => attempt + 1)}
            className="font-semibold underline underline-offset-2"
          >
            Retry verification check
          </button>
        </div>
      ) : null}
      {verifyError ? (
        <p role="alert" className="text-xs text-red-600">{verifyError}</p>
      ) : null}

      {!parentMissing && !verified ? (
        <p className="text-xs text-[var(--color-wi-text-light)]">
          We'll send a code to your parent's phone for consent.
        </p>
      ) : null}

      <SmsSendButton
        isSending={isSending}
        sendCount={sendCount}
        disabled={!canSend}
        onClick={() => void handleSend()}
        parentPhoneMissing={parentMissing}
      />

      {lastSentAt && !isSending && (
        <motion.p
          initial={{ opacity: 0, y: -4 }}
          animate={{ opacity: 1, y: 0 }}
          className="flex items-center gap-1.5 text-xs text-emerald-600 font-medium"
        >
          <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
          </svg>
          Code sent to {parentPhone ? `${parentPhone.replace(/\D/g, "").slice(0, 3)} *** ${parentPhone.replace(/\D/g, "").slice(-3)}` : "parent"}
        </motion.p>
      )}

      {verification.token ? (
        <motion.div
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.25, ease: "easeOut" }}
          className="space-y-3"
        >
          <OtpInput
            value={verification.code}
            onChange={verification.setCode}
            disabled={isSending || isVerifying}
            error={!!verifyError}
            autoFocus={sendCount > 0}
            label="Verification code"
          />
          <p className="text-xs text-gray-500">Enter the 6-digit code sent to your parent's phone.</p>
          {allowSubmitWithoutOtp ? (
            <button
              type="button"
              onClick={handleSkip}
              className="text-xs font-medium text-gray-500 hover:text-gray-700 transition-colors"
            >
              Continue without verifying
            </button>
          ) : null}
        </motion.div>
      ) : null}
    </div>
  );
}
