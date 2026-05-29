import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import { useFormValidation } from "../hooks/useFormValidation";
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import Input from "../components/ui/Input";
import FormField from "../components/ui/FormField";
import FormErrorSummary from "../components/ui/FormErrorSummary";

const schema = {
  username: [
    { type: "required" as const, message: "Username is required" },
    { type: "minLength" as const, value: 2, message: "Username must be at least 2 characters" },
  ],
  password: [
    { type: "required" as const, message: "Password is required" },
    { type: "minLength" as const, value: 6, message: "Password must be at least 6 characters" },
  ],
};

export default function TeacherCreate() {
  const navigate = useNavigate();
  const { addToast } = useToast();

  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const formValues = { username, password };
  const { errors, validate, validateAll, touched, touch } = useFormValidation(schema, formValues);

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!validateAll()) return;
    try {
      setSubmitting(true);
      await apiJson("/api/v1/admin/users", {
        method: "POST",
        body: JSON.stringify({
          username: username.trim(),
          role: "Teacher",
          password: password,
        }),
      });
      addToast("success", "Teacher created");
      navigate("/teachers");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Create failed");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="w-full">
      <PageHeading className="text-center">New Teacher</PageHeading>

      <form onSubmit={onSubmit} className="max-w-xl mx-auto space-y-4">
        <FormErrorSummary errors={errors} touched={touched} />

        <FormField name="username" label="Username" error={errors.username} touched={touched.username} required>
          <Input size="md" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" onBlur={() => { touch("username"); validate("username"); }} />
        </FormField>

        <FormField name="password" label="Password" error={errors.password} touched={touched.password} required>
          <Input size="md" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" onBlur={() => { touch("password"); validate("password"); }} />
        </FormField>

        <div className="flex gap-3 mt-6">
          <Button type="submit" variant="primary" size="lg" loading={submitting}>
            {submitting ? "Saving…" : "Save"}
          </Button>
          <Button type="button" variant="secondary" size="lg" onClick={() => navigate("/teachers")}>
            Back
          </Button>
        </div>
      </form>
    </div>
  );
}
