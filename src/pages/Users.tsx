import { useEffect, useMemo, useState } from "react";
import Modal from "../components/Modal";
import { ApiRequestError, apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import { useFormValidation } from "../hooks/useFormValidation";
import PageHeading from "../components/ui/PageHeading";
import SearchInput from "../components/ui/SearchInput";
import Button from "../components/ui/Button";
import Input from "../components/ui/Input";
import Select from "../components/ui/Select";
import EmptyState from "../components/ui/EmptyState";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import FormField from "../components/ui/FormField";
import FormErrorSummary from "../components/ui/FormErrorSummary";

type AdminUser = {
  id: string;
  username: string;
  full_name?: string | null;
  role: "Admin" | "Teacher";
  deleted_at: string | null;
  created_at: string;
};

const createSchema = {
  username: [{ type: "required" as const, message: "Username is required" }],
  role: [{ type: "required" as const, message: "Role is required" }],
  password: [
    { type: "required" as const, message: "Password is required" },
    { type: "minLength" as const, value: 6, message: "Password must be at least 6 characters" },
  ],
};

const resetSchema = {
  newPassword: [
    { type: "required" as const, message: "New password is required" },
    { type: "minLength" as const, value: 6, message: "Password must be at least 6 characters" },
  ],
};

export default function Users() {
  const { addToast } = useToast();
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [includeDeleted, setIncludeDeleted] = useState(false);
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [roleFilter, setRoleFilter] = useState<"All" | "Admin" | "Teacher">("All");

  const [createOpen, setCreateOpen] = useState(false);
  const [createSaving, setCreateSaving] = useState(false);
  const [createForm, setCreateForm] = useState({ username: "", role: "Teacher" as "Admin" | "Teacher", password: "" });

  const [resetUser, setResetUser] = useState<AdminUser | null>(null);
  const [resetSaving, setResetSaving] = useState(false);
  const [resetPassword, setResetPassword] = useState("");

  const [deactivateUser, setDeactivateUser] = useState<AdminUser | null>(null);
  const [deactivateSaving, setDeactivateSaving] = useState(false);

  const createFormValues = { username: createForm.username, role: createForm.role, password: createForm.password };
  const createValidation = useFormValidation(createSchema, createFormValues);

  const resetFormValues = { newPassword: resetPassword };
  const resetValidation = useFormValidation(resetSchema, resetFormValues);

  // The input stays immediate; the server request waits for typing to settle.
  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(search), 300);
    return () => clearTimeout(t);
  }, [search]);

  const load = async () => {
    try {
      setLoading(true);
      setLoadError(null);
      const params = new URLSearchParams();
      if (includeDeleted) params.set("include_deleted", "true");
      // Search server-side so users outside the initial page can be found;
      // 2+ characters prevents a broad re-fetch on the first keystroke.
      if (debouncedSearch.trim().length >= 2) params.set("q", debouncedSearch.trim());
      const qs = params.toString();
      const res = await apiJson<AdminUser[]>(`/api/v1/admin/users${qs ? `?${qs}` : ""}`, { method: "GET" });
      setUsers(res);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "Failed to load users");
      addToast("error", err instanceof Error ? err.message : "Failed to load users");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [includeDeleted, debouncedSearch]);

  const filtered = useMemo(() => {
    let data = [...users];
    if (roleFilter !== "All") data = data.filter((u) => u.role === roleFilter);
    if (search.trim()) {
      const q = search.trim().toLowerCase();
      data = data.filter((u) => u.username.toLowerCase().includes(q) || u.id.toLowerCase().includes(q));
    }
    return data;
  }, [users, roleFilter, search]);

  const submitCreate = async () => {
    if (!createValidation.validateAll()) return;
    try {
      setCreateSaving(true);
      await apiJson("/api/v1/admin/users", {
        method: "POST",
        body: JSON.stringify({
          username: createForm.username.trim(),
          role: createForm.role,
          password: createForm.password,
        }),
      });
      addToast("success", "User created");
      setCreateOpen(false);
      setCreateForm({ username: "", role: "Teacher", password: "" });
      await load();
    } catch (err) {
      if (err instanceof ApiRequestError && err.code) addToast("error", `${err.code}: ${err.message}`);
      else addToast("error", err instanceof Error ? err.message : "Create failed");
    } finally {
      setCreateSaving(false);
    }
  };

  const submitReset = async () => {
    if (!resetUser) return;
    if (!resetValidation.validateAll()) return;
    try {
      setResetSaving(true);
      await apiJson(`/api/v1/admin/users/${resetUser.id}/reset_password`, {
        method: "POST",
        body: JSON.stringify({ password: resetPassword }),
      });
      addToast("success", "Password reset");
      setResetUser(null);
      setResetPassword("");
      await load();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Reset failed");
    } finally {
      setResetSaving(false);
    }
  };

  const submitDeactivate = async () => {
    if (!deactivateUser) return;
    try {
      setDeactivateSaving(true);
      await apiJson(`/api/v1/admin/users/${deactivateUser.id}`, { method: "DELETE" });
      addToast("success", "User deactivated");
      setDeactivateUser(null);
      await load();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Deactivate failed");
    } finally {
      setDeactivateSaving(false);
    }
  };

  return (
    <div>
      <PageHeading>User</PageHeading>

      <div className="flex flex-wrap items-center gap-2 mb-4">
        <SearchInput value={search} onChange={setSearch} placeholder="Search username or id..." />
        <Select value={roleFilter} onChange={(e) => setRoleFilter(e.target.value as any)} size="sm">
          <option value="All">All Roles</option>
          <option value="Admin">Admin</option>
          <option value="Teacher">Teacher</option>
        </Select>
        <label className="flex items-center gap-1 text-sm text-[var(--color-wi-text-light)] select-none">
          <input type="checkbox" checked={includeDeleted} onChange={(e) => setIncludeDeleted(e.target.checked)} />
          Show deactivated
        </label>
        <Button variant="secondary" size="sm" onClick={() => void load()}>Refresh</Button>
        <Button variant="primary" size="sm" onClick={() => setCreateOpen(true)}>Create</Button>
      </div>

      <div className="border border-wi-line rounded-sm overflow-x-auto">
        <table className="w-full text-[13px]">
          <caption className="sr-only">List of users</caption>
          <thead>
            <tr className="border-b border-wi-line bg-[var(--color-wi-row-alt)]">
              <th scope="col" className="text-left py-2 px-2 font-semibold">Username</th>
              <th scope="col" className="text-left py-2 px-2 font-semibold">Role</th>
              <th scope="col" className="text-left py-2 px-2 font-semibold">Status</th>
              <th scope="col" className="text-left py-2 px-2 font-semibold">Created</th>
              <th scope="col" className="text-left py-2 px-2 font-semibold"></th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((u) => (
              <tr key={u.id} className="border-b border-wi-line-soft last:border-b-0 hover:bg-[var(--color-wi-row-alt)]">
                <td className="py-2 px-2">
                  <div className="text-[var(--color-wi-text)]">{u.full_name || u.username}</div>
                  <div className="font-mono text-[11px] text-[var(--color-wi-text-light)]">{u.username} · {u.id}</div>
                </td>
                <td className="py-2 px-2">
                  <span className={`text-xs px-2 py-0.5 rounded-sm ${u.role === "Admin" ? "bg-purple-100 text-purple-700" : "bg-blue-100 text-blue-700"}`}>
                    {u.role}
                  </span>
                </td>
                <td className="py-2 px-2">
                  {u.deleted_at ? <span className="text-xs text-[var(--color-wi-text-light)]">Deactivated</span> : <span className="text-xs text-green-700">Active</span>}
                </td>
                <td className="py-2 px-2 font-mono text-xs text-[var(--color-wi-text-light)]">{u.created_at}</td>
                <td className="py-2 px-2 text-right whitespace-nowrap">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setResetUser(u);
                      setResetPassword("");
                    }}
                    className="mr-2"
                  >
                    Reset password
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!!u.deleted_at}
                    onClick={() => setDeactivateUser(u)}
                  >
                    Deactivate
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {loading && <LoadingSkeleton type="table" lines={3} />}
        {!loading && loadError && <EmptyState message={`Failed to load users: ${loadError}`} />}
        {!loading && !loadError && filtered.length === 0 && <EmptyState message="No users found" />}
      </div>

      {createOpen && (
        <Modal
          title="Create User"
          onClose={() => setCreateOpen(false)}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setCreateOpen(false)}>Cancel</Button>
              <Button variant="primary" size="sm" onClick={() => void submitCreate()} loading={createSaving}>
                {createSaving ? "Creating…" : "Create"}
              </Button>
            </>
          }
        >
          <div className="space-y-3">
            <FormErrorSummary errors={createValidation.errors} touched={createValidation.touched} />

            <FormField name="username" label="Username" error={createValidation.errors.username} touched={createValidation.touched.username} required>
              <Input size="sm" value={createForm.username} onChange={(e) => setCreateForm({ ...createForm, username: e.target.value })} onBlur={() => { createValidation.touch("username"); createValidation.validate("username"); }} />
            </FormField>

            <FormField name="role" label="Role" error={createValidation.errors.role} touched={createValidation.touched.role} required>
              <Select size="sm" value={createForm.role} onChange={(e) => setCreateForm({ ...createForm, role: e.target.value as any })} onBlur={() => { createValidation.touch("role"); createValidation.validate("role"); }}>
                <option value="Teacher">Teacher</option>
                <option value="Admin">Admin</option>
              </Select>
            </FormField>

            <FormField name="password" label="Initial password" error={createValidation.errors.password} touched={createValidation.touched.password} required>
              <Input size="sm" type="password" value={createForm.password} onChange={(e) => setCreateForm({ ...createForm, password: e.target.value })} onBlur={() => { createValidation.touch("password"); createValidation.validate("password"); }} />
            </FormField>
          </div>
        </Modal>
      )}

      {resetUser && (
        <Modal
          title={`Reset password: ${resetUser.full_name || resetUser.username}`}
          onClose={() => setResetUser(null)}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setResetUser(null)}>Cancel</Button>
              <Button variant="primary" size="sm" onClick={() => void submitReset()} loading={resetSaving}>
                {resetSaving ? "Saving…" : "Reset"}
              </Button>
            </>
          }
        >
          <div className="space-y-3">
            <FormErrorSummary errors={resetValidation.errors} touched={resetValidation.touched} />

            <FormField name="newPassword" label="New password" error={resetValidation.errors.newPassword} touched={resetValidation.touched.newPassword} required>
              <Input size="sm" type="password" value={resetPassword} onChange={(e) => setResetPassword(e.target.value)} onBlur={() => { resetValidation.touch("newPassword"); resetValidation.validate("newPassword"); }} />
            </FormField>
            <div className="text-xs text-[var(--color-wi-text-light)]">This forces logout of all existing sessions for this user.</div>
          </div>
        </Modal>
      )}

      {deactivateUser && (
        <Modal
          title="Deactivate user?"
          onClose={() => setDeactivateUser(null)}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setDeactivateUser(null)}>Cancel</Button>
              <Button variant="danger" size="sm" onClick={() => void submitDeactivate()} loading={deactivateSaving}>
                {deactivateSaving ? "Deactivating…" : "Deactivate"}
              </Button>
            </>
          }
        >
          <p className="text-sm text-[var(--color-wi-text-light)]">
            Deactivate <span className="font-semibold">{deactivateUser.full_name || deactivateUser.username}</span>? They will no longer be able to sign in.
          </p>
        </Modal>
      )}
    </div>
  );
}
