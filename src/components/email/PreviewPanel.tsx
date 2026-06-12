import { useEffect, useState } from "react";
import { apiJson } from "../../api/client";

type PreviewResult = {
  subject: string;
  body: string;
};

type Props = {
  subject: string;
  body: string;
};

export default function PreviewPanel({ subject, body }: Props) {
  const [preview, setPreview] = useState<PreviewResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!subject && !body) {
      setPreview(null);
      return;
    }
    const timer = setTimeout(() => {
      setLoading(true);
      setError(null);
      apiJson<PreviewResult>("/api/v1/admin/sit-in-email-preview", {
        method: "POST",
        body: JSON.stringify({ template_subject: subject, template_body: body }),
      })
        .then(setPreview)
        .catch((err) => setError(err instanceof Error ? err.message : "Preview failed"))
        .finally(() => setLoading(false));
    }, 500);
    return () => clearTimeout(timer);
  }, [subject, body]);

  return (
    <div className="border border-gray-200 rounded-sm">
      <div className="px-3 py-2 text-sm font-medium text-gray-700 bg-gray-50 border-b border-gray-200">
        Preview
      </div>
      <div className="p-3">
        {loading && <p className="text-sm text-gray-400">Rendering preview...</p>}
        {error && <p className="text-sm text-red-500">{error}</p>}
        {preview && (
          <div className="space-y-3">
            <div>
              <span className="text-xs text-gray-400 font-medium">Subject:</span>
              <p className="text-sm font-medium text-gray-900 mt-0.5">{preview.subject}</p>
            </div>
            <div>
              <span className="text-xs text-gray-400 font-medium">Body:</span>
              <div className="mt-0.5 text-sm text-gray-700 whitespace-pre-wrap rounded-sm border border-gray-100 bg-white p-3">
                {preview.body}
              </div>
            </div>
          </div>
        )}
        {!preview && !loading && !error && (
          <p className="text-sm text-gray-400">Edit the template above to see a preview.</p>
        )}
      </div>
    </div>
  );
}
