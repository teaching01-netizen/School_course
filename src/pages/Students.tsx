import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import Modal from "../components/Modal";
import { useToast } from "../hooks/useToast";
import { useApiQuery } from "@/hooks/useApiQuery";
import { useApiMutation } from "@/hooks/useApiMutation";
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import Input from "../components/ui/Input";
import EmptyState from "../components/ui/EmptyState";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";

type Student = { id: string; wcode: string; full_name: string; notes: string };
type StudentPage = { items: Student[]; total_count: number; offset: number; limit: number };
const PAGE_SIZE = 50;

export default function Students() {
  const { addToast } = useToast();
  const [searchInput, setSearchInput] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [offset, setOffset] = useState(0);
  const [createModal, setCreateModal] = useState(false);
  const [form, setForm] = useState({ wcode: "", full_name: "", notes: "" });
  const searchRef = useRef<HTMLInputElement>(null);

  const apiUrl = useMemo(() => {
    const params = new URLSearchParams();
    params.set("limit", String(PAGE_SIZE));
    params.set("offset", String(offset));
    if (searchQuery) params.set("q", searchQuery);
    return `/api/v1/students?${params.toString()}`;
  }, [offset, searchQuery]);

  const { data: page, loading, error, refetch } = useApiQuery<StudentPage>(apiUrl);
  const { mutate: createStudent, loading: creating, error: createError } = useApiMutation<{ wcode: string; full_name: string; notes: string }, unknown>("POST");

  useEffect(() => {
    if (error) addToast("error", error.message);
  }, [error]);

  useEffect(() => {
    if (createError) addToast("error", createError.message);
  }, [createError]);

  const items = page?.items ?? [];
  const hasPrevious = offset > 0;
  const hasNext = offset + PAGE_SIZE < (page?.total_count ?? 0);
  const totalPages = Math.ceil((page?.total_count ?? 0) / PAGE_SIZE);
  const currentPage = Math.floor(offset / PAGE_SIZE) + 1;

  function handleSearch() {
    setSearchQuery(searchInput.trim());
    setOffset(0);
  }

  function jumpToPage(event: React.ChangeEvent<HTMLInputElement>) {
    const next = Math.max(1, Math.min(totalPages, Number(event.target.value) || 1));
    setOffset((next - 1) * PAGE_SIZE);
  }

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
      setOffset(0);
      refetch();
    } catch {
      // error toast handled by useEffect on createError
    }
  };

  return (
    <div>
      <PageHeading>Student</PageHeading>
      <div className="flex flex-wrap items-center gap-2 mb-4">
        <div className="relative flex-1 min-w-[200px] max-w-xs">
          <Input
            ref={searchRef}
            type="search"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") handleSearch(); }}
            placeholder="Search name / W-Code..."
            size="sm"
            className="pl-8"
            aria-label="Search students"
          />
          <svg className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400 pointer-events-none" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" /></svg>
        </div>
        <Button variant="secondary" size="sm" onClick={handleSearch}>Search</Button>
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
          {items.map((s) => (
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
      {!loading && items.length === 0 && <EmptyState message="No students found" />}

      <div className="mt-3 flex items-center justify-between text-sm text-gray-500">
        <span>{page?.total_count ?? 0} records</span>
        <div className="flex items-center gap-2">
          <Button variant="secondary" size="sm" disabled={!hasPrevious} onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}>Previous</Button>
          <div className="flex items-center gap-1">
            <input aria-label="Go to page" type="number" min={1} max={totalPages} value={currentPage} onChange={jumpToPage} className="w-14 rounded-sm border border-gray-300 px-2 py-1 text-sm text-center" />
            <span>of {totalPages}</span>
          </div>
          <Button variant="secondary" size="sm" disabled={!hasNext} onClick={() => setOffset(offset + PAGE_SIZE)}>Next</Button>
        </div>
      </div>

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
