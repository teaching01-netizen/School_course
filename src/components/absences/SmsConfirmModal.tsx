import { useCallback } from "react";
import { MessageSquare, Phone, Send, SkipForward } from "lucide-react";
import Button from "../ui/Button";
import Modal from "../Modal";

type Props = {
  phones: string[];
  message: string;
  onSend: () => void;
  onSkip: () => void;
  sending: boolean;
};

export default function SmsConfirmModal({ phones, message, onSend, onSkip, sending }: Props) {
  const handleClose = useCallback(() => {
    if (!sending) onSkip();
  }, [sending, onSkip]);

  return (
    <Modal
      title="Send Absence Notification"
      onClose={handleClose}
      size="md"
      footer={
        <>
          <div className="flex-1" />
          <Button variant="secondary" onClick={onSkip} disabled={sending}>
            <SkipForward className="mr-1.5 inline h-4 w-4" />
            Skip
          </Button>
          <Button onClick={onSend} loading={sending} disabled={sending}>
            <Send className="mr-1.5 inline h-4 w-4" />
            Send SMS & Email
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <p className="text-sm text-[var(--color-wi-text-light)]">
          The absence has been created. Would you like to send SMS and email notifications to the student and parent?
        </p>

        <div>
          <h4 className="mb-2 text-sm font-medium text-[var(--color-wi-text-light)]">Recipients</h4>
          <div className="space-y-1.5">
            {phones.map((phone, idx) => (
              <div key={`${phone}-${idx}`} className="flex items-center gap-2 rounded-lg border border-wi-line bg-[var(--color-wi-row-alt)] px-3 py-2">
                <Phone className="h-4 w-4 text-[var(--color-wi-text-light)]" />
                <span className="text-sm font-mono text-[var(--color-wi-text-light)]">{phone}</span>
              </div>
            ))}
          </div>
        </div>

        <div>
          <h4 className="mb-2 text-sm font-medium text-[var(--color-wi-text-light)]">Message Preview</h4>
          <div className="rounded-lg border border-amber-200 bg-amber-50/50 p-3">
            <div className="flex items-start gap-2">
              <MessageSquare className="mt-0.5 h-4 w-4 flex-shrink-0 text-amber-600" />
              <p className="whitespace-pre-wrap text-sm text-[var(--color-wi-text-light)]">{message}</p>
            </div>
          </div>
        </div>
      </div>
    </Modal>
  );
}
