import { useRef } from "react";
import PlaceholderGuide from "./PlaceholderGuide";

type Props = {
  subject: string;
  body: string;
  onChange: (subject: string, body: string) => void;
  name?: string;
  onNameChange?: (name: string) => void;
};

export default function TemplateEditor({ subject, body, onChange, name, onNameChange }: Props) {
  const bodyRef = useRef<HTMLTextAreaElement>(null);
  const subjectRef = useRef<HTMLInputElement>(null);

  function insertAt(element: HTMLInputElement | HTMLTextAreaElement | null, token: string) {
    if (!element) return;
    const start = element.selectionStart ?? element.value.length;
    const end = element.selectionEnd ?? start;
    const before = element.value.slice(0, start);
    const after = element.value.slice(end);
    const updated = before + token + after;
    if (element === subjectRef.current) {
      onChange(updated, body);
    } else {
      onChange(subject, updated);
    }
    requestAnimationFrame(() => {
      const pos = start + token.length;
      element.setSelectionRange(pos, pos);
      element.focus();
    });
  }

  return (
    <div className="space-y-4">
      {onNameChange && (
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Template name</label>
          <input
            type="text"
            value={name ?? ""}
            onChange={(e) => onNameChange(e.target.value)}
            className="w-full rounded-sm border border-gray-300 p-2 text-sm"
            placeholder="e.g. Sit-in Day Reminder"
          />
        </div>
      )}
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">Subject line</label>
        <div className="flex gap-1">
          <input
            ref={subjectRef}
            type="text"
            value={subject}
            onChange={(e) => onChange(e.target.value, body)}
            className="flex-1 rounded-sm border border-gray-300 p-2 text-sm font-mono"
            placeholder="Enter email subject..."
          />
          <PlaceholderGuide onInsert={(t) => insertAt(subjectRef.current, t)} />
        </div>
      </div>
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">Email body</label>
        <div className="space-y-2">
          <textarea
            ref={bodyRef}
            value={body}
            onChange={(e) => onChange(subject, e.target.value)}
            rows={12}
            className="w-full rounded-sm border border-gray-300 p-2 text-sm font-mono resize-y"
            placeholder="Enter email body..."
          />
          <PlaceholderGuide onInsert={(t) => insertAt(bodyRef.current, t)} />
        </div>
      </div>
    </div>
  );
}
