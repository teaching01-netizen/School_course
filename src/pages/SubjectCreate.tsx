import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ApiRequestError, apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import { useFormValidation } from "../hooks/useFormValidation";
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import Input from "../components/ui/Input";
import FormField from "../components/ui/FormField";
import FormErrorSummary from "../components/ui/FormErrorSummary";

type Subject = { id: string; code: string; name: string };

const schema = {
  name: [{ type: "required" as const, message: "Name is required" }],
};

function nextSubjectCode(subjects: Subject[]) {
  const numericCodes = subjects
    .map((s) => s.code.trim())
    .filter((c) => /^\d+$/.test(c))
    .map((c) => Number.parseInt(c, 10))
    .filter((n) => Number.isFinite(n) && n >= 0);
  return (numericCodes.length ? Math.max(...numericCodes) + 1 : 0).toString().padStart(2, "0");
}

export default function SubjectCreate() {
  const navigate = useNavigate();
  const { addToast } = useToast();

  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [generating, setGenerating] = useState(true);

  const formValues = { name };
  const { errors, validate, validateAll, touched, touch } = useFormValidation(schema, formValues);

  const refreshGeneratedCode = useCallback(async () => {
    setGenerating(true);
    try {
      const subjects = await apiJson<Subject[]>("/api/v1/subjects", { method: "GET" });
      setCode(nextSubjectCode(subjects));
    } catch {
      const fallback = (Date.now() % 100).toString().padStart(2, "0");
      setCode(fallback);
    } finally {
      setGenerating(false);
    }
  }, []);

  useEffect(() => {
    void refreshGeneratedCode();
  }, [refreshGeneratedCode]);

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!validateAll()) return;
    if (!code.trim()) {
      addToast("error", "Id is not ready yet");
      return;
    }
    try {
      setSubmitting(true);
      await apiJson("/api/v1/subjects", { method: "POST", body: JSON.stringify({ code: code.trim(), name: name.trim() }) });
      addToast("success", "Subject created");
      navigate("/subjects");
    } catch (err) {
      if (err instanceof ApiRequestError && (err.status === 409 || err.code === "conflict")) {
        await refreshGeneratedCode();
        addToast("warning", "Id was already used. Generated a new id; review and save again.");
        return;
      }
      addToast("error", err instanceof Error ? err.message : "Create failed");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="w-full">
      <PageHeading>New Subject</PageHeading>

      <form onSubmit={onSubmit} className="max-w-xl space-y-4">
        <FormErrorSummary errors={errors} touched={touched} />

        <FormField name="code" label="Id">
          <Input size="md" value={generating ? "Generating…" : code} readOnly className="bg-gray-50" />
        </FormField>

        <FormField name="name" label="Name" error={errors.name} touched={touched.name} required>
          <Input size="md" value={name} onChange={(e) => setName(e.target.value)} onBlur={() => { touch("name"); validate("name"); }} />
        </FormField>

        <div className="flex gap-3 mt-6">
          <Button type="submit" variant="primary" size="lg" loading={submitting}>
            {submitting ? "Saving…" : "Save"}
          </Button>
          <Button type="button" variant="secondary" size="lg" onClick={() => navigate("/subjects")}>
            Back
          </Button>
        </div>
      </form>
    </div>
  );
}
