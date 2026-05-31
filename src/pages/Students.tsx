import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import Modal from "../components/Modal";
import { useToast } from "../hooks/useToast";
import { useApiQuery } from "@/hooks/useApiQuery";
import { useApiMutation } from "@/hooks/useApiMutation";
import PageHeading from "../components/ui/PageHeading";
import SearchInput from "../components/ui/SearchInput";
import Button from "../components/ui/Button";
import Input from "../components/ui/Input";
import EmptyState from "../components/ui/EmptyState";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";

type Student = { id: string; wcode: string; full_name: string; notes: string };

export default function Students() {
  const { addToast } = useToast();
  const [search, setSearch] = useState("");
  const [createModal, setCreateModal] = useState(false);
  const [form, setForm] = useState({ wcode: "", full_name: "", notes: "" });

  const { data: students, loading, error, refetch } = useApiQuery<Student[]>("/api/v1/students");
  const { mutate: createStudent, loading: creating, error: createError } = useApiMutation<{ wcode: string; full_name: string; notes: string }, unknown>("POST");

  useEffect(() => {
    if (error) addToast("error", error.message);
  }, [error]);

  useEffect(() => {
    if (createError) addToast("error", createError.message);
  }, [createError]);

  const filtered = useMemo(() => {
    let data = [...(students ?? [])];
    const q = search.trim().toLowerCase();
    if (q) {
      data = data.filter((s) => s.wcode.toLowerCase().includes(q) || s.full_name.toLowerCase().includes(q));
    }
    return data;
  }, [search, students]);

  const handleCreate = async () => {
    if (!form.wcode.trim() || !form.full_name.trim()) {
      addToast("error", "W-Code and Name are required");
      return;
    }
    try {
      await createStudent(form, "/api/v1/students");
      addToast("success", "Student created");
      setCreateModal(false);
      setForm({ wcode: "", full_name: "", notes: "" });
      refetch();
    } catch {
      // error toast handled by useEffect on createError
    }
  };

  return (
    <div>
      <PageHeading>Student</PageHeading>
      <div className="flex flex-wrap items-center gap-2 mb-4">
        <SearchInput value={search} onChange={setSearch} placeholder="Search name / W-Code..." />
        <Button variant="secondary" size="sm" onClick={() => void refetch()}>Refresh</Button>
        <Button variant="primary" size="md" onClick={() => setCreateModal(true)}>
          Create
        </Button>
      </div>

      <div className="overflow-x-auto"><table className="w-full text-[13px]">
        <thead>
          <tr className="border-b-2 border-gray-300">
            <th className="text-left py-2 px-2 font-semibold">W-Code</th>
            <th className="text-left py-2 px-2 font-semibold">Name</th>
            <th className="text-left py-2 px-2 font-semibold"></th>
          </tr>
        </thead>
        <tbody>
          {filtered.map((s) => (
            <tr key={s.id} className="border-b border-gray-200 hover:bg-gray-50">
              <td className="py-2 px-2 font-mono text-xs text-gray-600">{s.wcode}</td>
              <td className="py-2 px-2">{s.full_name}</td>
              <td className="py-2 px-2">
                <Link
                  to={`/students/${encodeURIComponent(s.wcode)}`}
                  className="px-2 py-0.5 text-xs bg-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary-dark)] text-white rounded-sm inline-block"
                >
                  detail
                </Link>
              </td>
            </tr>
          ))}
        </tbody>
      </table></div>

      {loading && <LoadingSkeleton type="table" lines={3} />}
      {!loading && filtered.length === 0 && <EmptyState message="No students found" />}

      {createModal && (
        <Modal
          title="Create New Student"
          onClose={() => setCreateModal(false)}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setCreateModal(false)}>
                Cancel
              </Button>
              <Button variant="primary" size="sm" onClick={handleCreate} loading={creating}>
                {creating ? "Creating…" : "Create"}
              </Button>
            </>
          }
        >
          <div className="space-y-3">
            <div>
              <label className="block text-xs text-gray-500 mb-1">W-Code *</label>
              <Input size="sm" value={form.wcode} onChange={(e) => setForm({ ...form, wcode: e.target.value })} />
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Name *</label>
              <Input size="sm" value={form.full_name} onChange={(e) => setForm({ ...form, full_name: e.target.value })} />
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Notes</label>
              <textarea value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm" rows={3} />
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
