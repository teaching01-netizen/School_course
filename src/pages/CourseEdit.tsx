import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import { useFormValidation } from "../hooks/useFormValidation";
import { useDirtyForm } from "../hooks/useDirtyForm";
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import Input from "../components/ui/Input";
import FormField from "../components/ui/FormField";
import FormErrorSummary from "../components/ui/FormErrorSummary";

type Course = { id: string; code: string; name: string };

const schema = {
  code: [{ type: "required" as const, message: "Code is required" }],
  name: [{ type: "required" as const, message: "Name is required" }],
};

export default function CourseEdit() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { addToast } = useToast();

  const [course, setCourse] = useState<Course | null>(null);
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const formValues = { code, name };
  const { errors, validate, validateAll, touched, touch } = useFormValidation(schema, formValues);
  const { setInitialState } = useDirtyForm(null, formValues, { warnBeforeUnload: true });

  useEffect(() => {
    (async () => {
      if (!id) return;
      try {
        setLoading(true);
        const c = await apiJson<Course>(`/api/v1/courses/${id}`, { method: "GET" });
        setCourse(c);
        setCode(c.code);
        setName(c.name);
        setInitialState({ code: c.code, name: c.name });
      } catch (err) {
        addToast("error", err instanceof Error ? err.message : "Failed to load course");
      } finally {
        setLoading(false);
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [addToast, id]);

  const onSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!id) return;
    if (!validateAll()) return;
    try {
      setSaving(true);
      await apiJson(`/api/v1/courses/${id}`, { method: "PUT", body: JSON.stringify({ code, name }) });
      addToast("success", "Course updated");
      navigate(`/courses/${id}`);
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Update failed");
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <div className="text-sm text-gray-400">Loading…</div>;
  if (!course) return <div className="text-sm text-gray-400">Course not found</div>;

  return (
    <div className="max-w-xl">
      <PageHeading>Edit Course</PageHeading>
      <form onSubmit={onSave} className="space-y-3">
        <FormErrorSummary errors={errors} touched={touched} />

        <FormField name="code" label="Code" error={errors.code} touched={touched.code} required>
          <Input size="sm" value={code} onChange={(e) => setCode(e.target.value)} onBlur={() => { touch("code"); validate("code"); }} />
        </FormField>

        <FormField name="name" label="Name" error={errors.name} touched={touched.name} required>
          <Input size="sm" value={name} onChange={(e) => setName(e.target.value)} onBlur={() => { touch("name"); validate("name"); }} />
        </FormField>

        <div className="flex gap-2">
          <Button type="button" variant="secondary" size="md" onClick={() => navigate(`/courses/${id}`)}>Cancel</Button>
          <Button type="submit" variant="primary" size="md" loading={saving}>{saving ? "Saving…" : "Save"}</Button>
        </div>
      </form>
    </div>
  );
}
