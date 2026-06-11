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
}: StepCoverVerificationProps) {
  const [session, setSession] = useState<ParentVerificationResponse | null>(null);
  const [sendError, setSendError] = useState<string | null>(null);
  const [verifyError, setVerifyError] = useState<string | null>(null);
  const [sendCount, setSendCount] = useState(0);
  const [isSending, setIsSending] = useState(false);
  const [isVerifying, setIsVerifying] = useState(false);
  const [lastSentAt, setLastSentAt] = useState<number | null>(null);
  const autoVerifyCodeRef = useRef<string | null>(null);

  const verified = completed || session?.status === "verified" || session?.status === "consumed";

  useEffect(() => {
    if (verified || isSending || isVerifying || !verification.token) return;
    const normalized = verification.code.replace(/\D/g, "").slice(0, 6);
    if (normalized.length !== 6) { autoVerifyCodeRef.current = null; return; }
    if (autoVerifyCodeRef.current === normalized) return;
    autoVerifyCodeRef.current = normalized;
    void handleVerify();
  }, [verification.code, verification.token, verified, isSending, isVerifying]);

  async function handleSend() {
    if (!wcode || !parentPhone) return;
    setIsSending(true);
    setSendError(null);
    setVerifyError(null);
    try {
      const response = await apiJson<ParentVerificationResponse>("/api/v1/absences/parent-verification/send", {
        method: "POST",
        body: JSON.stringify({ wcode, ...(verification.token ? { token: verification.token } : {}) }),
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
    return <p className="text-xs text-green-600 font-medium">✓ Verified</p>;
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
      {verifyError ? (
        <p role="alert" className="text-xs text-red-600">{verifyError}</p>
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
