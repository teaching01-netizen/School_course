import { useState } from "react";
import { useNavigate } from "react-router-dom";
import WILogo from '../components/WILogo';
import { useAuth } from "../hooks/useAuth";
import { useFormValidation } from "../hooks/useFormValidation";
import FormField from "../components/ui/FormField";
import FormErrorSummary from "../components/ui/FormErrorSummary";

const schema = {
  username: [{ type: "required" as const, message: "Username is required" }],
  password: [{ type: "required" as const, message: "Password is required" }],
};

export default function Login() {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const navigate = useNavigate();
  const { login } = useAuth();

  const formValues = { username, password };
  const { errors, validate, validateAll, touched, touch } = useFormValidation(schema, formValues);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!validateAll()) return;
    setError("");
    setSubmitting(true);
    try {
      await login(username, password);
      navigate('/');
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setSubmitting(false);
    }
  };

  const allErrors = { ...errors };
  if (error) allErrors._server = error;

  return (
    <div className="min-h-screen bg-gray-100 flex items-center justify-center px-4">
      <div className="w-full max-w-sm bg-white p-6 rounded-sm shadow-sm border border-gray-200">
        <div className="flex justify-center mb-6">
          <WILogo />
        </div>
        <h2 className="text-center text-lg font-semibold text-gray-800 mb-4">Sign In</h2>
        <form onSubmit={handleSubmit} className="space-y-3">
          <FormErrorSummary errors={allErrors} touched={{ ...touched, _server: true }} />

          <FormField name="username" label="Username" error={errors.username} touched={touched.username} required>
            <input
              type="text"
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="Enter username..."
              className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
              onBlur={() => { touch("username"); validate("username"); }}
            />
          </FormField>

          <FormField name="password" label="Password" error={errors.password} touched={touched.password} required>
            <input
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
              onBlur={() => { touch("password"); validate("password"); }}
            />
          </FormField>

          <button
            type="submit"
            disabled={submitting}
            className="w-full py-1.5 text-sm bg-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary-dark)] text-white rounded-sm disabled:opacity-60 disabled:cursor-not-allowed"
          >
            {submitting ? "Signing in..." : "Sign In"}
          </button>
        </form>
        <p className="text-center text-xs text-gray-400 mt-4">Sign in with your provisioned account</p>
      </div>
    </div>
  );
}
