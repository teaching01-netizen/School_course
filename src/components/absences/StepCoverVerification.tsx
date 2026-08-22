import { useEffect, useRef, useState } from "react";
import { motion } from "framer-motion";
import { apiJson, ApiRequestError } from "@/api/client";
import { loadStudentProfile } from "@/features/absences/api/absenceFormApi";
import {
  clearStudentSessionHint,
  hasStudentSessionHint,
  markStudentSessionHint,
} from "@/features/absences/storage/studentResumeStorage";
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
  lookupToken: string;
  parentVerificationAvailable?: boolean;
  parentPhone?: string | null;
  online?: boolean;
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
  lookupToken,
  parentVerificationAvailable,
  parentPhone,
  online = true,
  smsParentEnabled = true,
  verification,
  completed,
  onSatisfied,
  onRestart,
  onRestored,
}: StepCoverVerificationProps) {
  const verificationAvailable = parentVerificationAvailable ?? Boolean(parentPhone);
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
  const [enrollPhone, setEnrollPhone] = useState("");
  const autoVerifyCodeRef = useRef<string | null>(null);

  const verified = completed || session?.status === "verified" || session?.status === "consumed";
  const deliveryStatus = session?.delivery_status;
  const deliveryPending = deliveryStatus === "queued"
    || deliveryStatus === "preparing"
    || deliveryStatus === "submitting"
    || deliveryStatus === "retryable";
  const deliveryAccepted = deliveryStatus === "accepted" || (!deliveryStatus && lastSentAt !== null);
  // The number lives in React state only: it is sent once to start the OTP
  // and never written to local storage.
  const enrollPhoneDigits = enrollPhone.replace(/\D/g, "");
  const enrollPhoneValid = enrollPhoneDigits.length >= 9 && enrollPhoneDigits.length <= 12;

  useEffect(() => {
    if (!verification.token || !wcode) {
      setRestoreError(null);
      return;
    }
    if (!online) {
      setRestoreError("You're offline. Reconnect to validate saved verification.");
      return;
    }
    const controller = new AbortController();
    setRestoreError(null);
    void apiJson<ParentVerificationResponse>(
      "/api/v1/absences/parent-verification/status",
      { method: "POST", body: JSON.stringify({ token: verification.token }), signal: controller.signal },
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
  }, [verification.token, wcode, online, validationAttempt, onRestart, onRestored]);

  useEffect(() => {
    if (
      verification.token
      || completed
      || !wcode
      || !online
      || !hasStudentSessionHint()
    ) {
      return;
    }
    let active = true;
    setRestoreError(null);
    void loadStudentProfile()
      .then((profile) => {
        if (!active) return;
        if (profile.wcode !== wcode) {
          clearStudentSessionHint();
          setRestoreError("Your verified session belongs to a different Student ID. Verify this Student ID again.");
          onRestart();
          return;
        }
        onRestored();
      })
      .catch((error: unknown) => {
        if (!active) return;
        if (error instanceof ApiRequestError && (error.status === 401 || error.status === 403)) {
          clearStudentSessionHint();
          return;
        }
        setRestoreError("Could not restore your verified session. Check your connection and try again.");
      });
    return () => {
      active = false;
    };
  }, [verification.token, completed, wcode, online, onRestart, onRestored]);

  useEffect(() => {
    const token = session?.token ?? verification.token;
    if (!online || !token || !deliveryPending) return;
    const controller = new AbortController();
    const poll = window.setInterval(() => {
      void apiJson<ParentVerificationResponse>(
        "/api/v1/absences/parent-verification/status",
        { method: "POST", body: JSON.stringify({ token }), signal: controller.signal },
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
  }, [session?.token, verification.token, deliveryPending, online]);

  useEffect(() => {
    if (!online || verified || isSending || isVerifying || !verification.token) return;
    const normalized = verification.code.replace(/\D/g, "").slice(0, 6);
    if (normalized.length !== 6) { autoVerifyCodeRef.current = null; return; }
    if (autoVerifyCodeRef.current === normalized) return;
    autoVerifyCodeRef.current = normalized;
    void handleVerify();
  }, [verification.code, verification.token, verified, isSending, isVerifying, online]);

  async function handleSend(startNewSession = false) {
    // With no phone on file, sending is only possible through enrollment.
    const enrolling = !verificationAvailable;
    if (!online || !smsParentEnabled || !lookupToken) return;
    if (enrolling && !enrollPhoneValid) return;
    if (startNewSession) {
      clearStudentSessionHint();
      onRestart();
      setSession(null);
    }
    setIsSending(true);
    setSendError(null);
    setVerifyError(null);
    setVerifyRetryable(false);
    try {
      const startingNewSession = startNewSession || !verification.token;
      // A family with no phone on file enrolls theirs here; the OTP proves
      // the number is reachable before the server persists it.
      const newSessionBody: Record<string, string> = { lookup_token: lookupToken };
      if (enrolling && enrollPhone.trim()) newSessionBody.parent_phone = enrollPhone.trim();
      const response = await apiJson<ParentVerificationResponse>("/api/v1/absences/parent-verification/send", {
        method: "POST",
        body: JSON.stringify(startingNewSession ? newSessionBody : { token: verification.token }),
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
    if (!online || !verification.token || verification.code.length !== 6) return;
    setIsVerifying(true);
    setVerifyError(null);
    setVerifyRetryable(false);
    try {
      const response = await apiJson<ParentVerificationResponse>("/api/v1/absences/parent-verification/verify", {
        method: "POST",
        body: JSON.stringify({ token: verification.token, code: verification.code }),
      });
      setSession(response);
      markStudentSessionHint();
      verification.clearStoredToken();
      verification.setCode("");
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
  const parentMissing = !verificationAvailable;
  const canSend = online
    && smsParentEnabled
    && !isSending
    && !isVerifying
    && !verified
    && (!parentMissing || enrollPhoneValid);

  if (verified) {
    return (
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-green-600 font-medium">✓ Verified</p>
        {smsParentEnabled && !parentMissing ? (
          <button
            type="button"
            onClick={() => void handleSend(true)}
            disabled={!online}
            className="text-xs font-semibold text-[var(--color-wi-primary)] hover:text-[var(--color-wi-primary-dark)] disabled:cursor-not-allowed disabled:opacity-50"
          >
            Send new code
          </button>
        ) : null}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {!online ? (
        <p role="status" aria-live="polite" className="rounded-lg bg-[var(--color-wi-amber-bg)] px-3 py-2 text-xs font-medium text-[var(--color-wi-amber)]">
          You're offline. Reconnect to send or verify the parent code.
        </p>
      ) : null}
      {!smsParentEnabled ? (
        <p role="alert" className="text-xs text-amber-600">
          Parent verification codes are currently unavailable.
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

      {smsParentEnabled && parentMissing && !verified ? (
        <div className="space-y-1.5">
          <label htmlFor="parent-phone-input" className="block text-xs font-medium text-[var(--color-wi-text-light)]">
            Parent's phone number <span className="text-[var(--color-wi-red)]">*</span>
          </label>
          <input
            id="parent-phone-input"
            type="tel"
            inputMode="tel"
            autoComplete="tel"
            className="min-h-[48px] w-full rounded-xl border border-[var(--color-wi-border)] bg-white px-4 text-base text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
            placeholder="e.g. 0812345678"
            value={enrollPhone}
            onChange={(e) => setEnrollPhone(e.target.value)}
          />
          <p className="text-xs text-[var(--color-wi-text-light)]">
            No parent phone is on file. We'll text a one-time code to this number to verify it, then save it for future absence confirmations.
          </p>
          {!enrollPhoneValid && enrollPhone.trim() ? (
            <p className="text-xs text-[var(--color-wi-amber)]">Enter a valid phone number (9–12 digits).</p>
          ) : null}
        </div>
      ) : null}

      {smsParentEnabled ? (
        <SmsSendButton
          isSending={isSending || deliveryPending}
          sendCount={sendCount}
          disabled={!canSend}
          onClick={() => void handleSend()}
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
          {/* parent_phone arrives pre-masked from the server (e.g. ••••5678) */}
          Code sent to {session?.parent_phone || "parent"}
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
          <p className="text-xs text-[var(--color-wi-text-light)]">Enter the 6-digit code sent to your parent's phone.</p>
        </motion.div>
      ) : null}

    </div>
  );
}
