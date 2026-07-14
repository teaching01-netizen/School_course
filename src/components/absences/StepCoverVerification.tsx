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
  smsParentEnabled?: boolean;
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
  smsParentEnabled = true,
  verification,
  completed,
  onSatisfied,
  onRestart,
  onRestored,
}: StepCoverVerificationProps) {
  const [session, setSession] = useState<ParentVerificationResponse | null>(null);
  const [sendError, setSendError] = useState<string | null>(null);
  const [verifyError, setVerifyError] = useState<string | null>(null);
  const [verifyRetryable, setVerifyRetryable] = useState(false);
  const [sendCount, setSendCount] = useState(0);
  const [isSending, setIsSending] = useState(false);
  const [isVerifying, setIsVerifying] = useState(false);
  const [lastSentAt, setLastSentAt] = useState<number | null>(null);
  const [restoreError, setRestoreError] = useState<string | null>(null);
  const [validationAttempt, setValidationAttempt] = useState(0);
  const autoVerifyCodeRef = useRef<string | null>(null);

  const verified = completed || session?.status === "verified" || session?.status === "consumed";
  const deliveryStatus = session?.delivery_status;
  const deliveryPending = deliveryStatus === "queued"
    || deliveryStatus === "preparing"
    || deliveryStatus === "submitting"
    || deliveryStatus === "retryable";
  const deliveryAccepted = deliveryStatus === "accepted" || (!deliveryStatus && lastSentAt !== null);

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
    const token = session?.token ?? verification.token;
    if (!token || !deliveryPending) return;
    const controller = new AbortController();
    const poll = window.setInterval(() => {
      void apiJson<ParentVerificationResponse>(
        `/api/v1/absences/parent-verification/${encodeURIComponent(token)}`,
        { method: "GET", signal: controller.signal },
      ).then((response) => {
        if (controller.signal.aborted) return;
        setSession(response);
        if (response.delivery_status === "accepted") setLastSentAt(Date.now());
      }).catch(() => {
        // A later poll can recover from a transient status-read failure.
      });
    }, 1000);
    return () => {
      controller.abort();
      window.clearInterval(poll);
    };
  }, [session?.token, verification.token, deliveryPending]);

  useEffect(() => {
    if (verified || isSending || isVerifying || !verification.token) return;
    const normalized = verification.code.replace(/\D/g, "").slice(0, 6);
    if (normalized.length !== 6) { autoVerifyCodeRef.current = null; return; }
    if (autoVerifyCodeRef.current === normalized) return;
    autoVerifyCodeRef.current = normalized;
    void handleVerify();
  }, [verification.code, verification.token, verified, isSending, isVerifying]);

  async function handleSend(startNewSession = false) {
    if (!smsParentEnabled || !wcode || !parentPhone) return;
    if (startNewSession) {
      onRestart();
      setSession(null);
    }
    setIsSending(true);
    setSendError(null);
    setVerifyError(null);
    setVerifyRetryable(false);
    try {
      const response = await apiJson<ParentVerificationResponse>("/api/v1/absences/parent-verification/send", {
        method: "POST",
        body: JSON.stringify({ wcode, ...(!startNewSession && verification.token ? { token: verification.token } : {}) }),
      });
      setSession(response);
      verification.persistToken(response.token, response.expires_at ? Date.parse(response.expires_at) : null);
      verification.setCode("");
      if (!response.delivery_status || response.delivery_status === "accepted") {
        setLastSentAt(Date.now());
      } else {
        setLastSentAt(null);
      }
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
    setVerifyRetryable(false);
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
      const retryable = isRetryable(err);
      setVerifyError(message);
      setVerifyRetryable(retryable);
      if (!retryable) verification.setCode("");
    } finally {
      setIsVerifying(false);
    }
  }

  const parentMissing = !parentPhone || parentPhone.trim() === "";
  const canSend = smsParentEnabled && !isSending && !isVerifying && !parentMissing && !verified;

  if (verified) {
    return (
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-green-600 font-medium">✓ Verified</p>
        {smsParentEnabled && !parentMissing ? (
          <button
            type="button"
            onClick={() => void handleSend(true)}
            className="text-xs font-semibold text-[var(--color-wi-primary)] hover:text-[var(--color-wi-primary-dark)]"
          >
            Send new code
          </button>
        ) : null}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {!smsParentEnabled ? (
        <p role="alert" className="text-xs text-amber-600">
          Parent verification codes are currently unavailable.
          Contact admin before continuing.
        </p>
      ) : parentMissing ? (
        <p role="alert" className="text-xs text-amber-600">
          Your parent's phone number is not in our records.
          Contact admin before continuing.
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
        <div className="space-y-1 text-xs text-red-600">
          <p role="alert">{verifyError}</p>
          {verifyRetryable && verification.code.length === 6 ? (
            <button
              type="button"
              onClick={() => void handleVerify()}
              disabled={isVerifying}
              className="font-semibold underline underline-offset-2 disabled:opacity-50"
            >
              Retry verification
            </button>
          ) : null}
        </div>
      ) : null}

      {smsParentEnabled && !parentMissing && !verified ? (
        <p className="text-xs text-[var(--color-wi-text-light)]">
          We'll send a code to your parent's phone for consent.
        </p>
      ) : null}

      {smsParentEnabled ? (
        <SmsSendButton
          isSending={isSending || deliveryPending}
          sendCount={sendCount}
          disabled={!canSend}
          onClick={() => void handleSend()}
          parentPhoneMissing={parentMissing}
        />
      ) : null}

      {deliveryPending ? (
        <p className="text-xs font-medium text-blue-600">Sending code…</p>
      ) : null}

      {deliveryStatus === "uncertain" ? (
        <p role="status" className="text-xs font-medium text-amber-700">
          The SMS may have been sent. Enter the code if it arrives, or resend after the cooldown.
        </p>
      ) : null}

      {deliveryStatus === "failed" ? (
        <p role="alert" className="text-xs font-medium text-red-600">
          We couldn't send the code. Please try again.
        </p>
      ) : null}

      {deliveryStatus === "expired" ? (
        <p role="alert" className="text-xs font-medium text-red-600">
          The verification code expired. Request a new code.
        </p>
      ) : null}

      {deliveryAccepted && lastSentAt && !isSending && (
        <motion.p
          initial={{ opacity: 0, y: -4 }}
          animate={{ opacity: 1, y: 0 }}
          className="flex items-center gap-1.5 text-xs text-emerald-700 font-medium"
        >
          <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
          </svg>
          Code sent to {parentPhone ? `${parentPhone.replace(/\D/g, "").slice(0, 3)} *** ${parentPhone.replace(/\D/g, "").slice(-3)}` : "parent"}
        </motion.p>
      )}

      {(session?.token || verification.token) ? (
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
        </motion.div>
      ) : null}

    </div>
  );
}
