import { useEffect, useMemo, useState } from "react";
import Modal from "../components/Modal";
import { useToast } from "../hooks/useToast";
import { useApiQuery } from "@/hooks/useApiQuery";
import { useApiMutation } from "@/hooks/useApiMutation";
import { useFormValidation } from "../hooks/useFormValidation";
import PageHeading from "../components/ui/PageHeading";
import SearchInput from "../components/ui/SearchInput";
import Button from "../components/ui/Button";
import Input from "../components/ui/Input";
import EmptyState from "../components/ui/EmptyState";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import FormField from "../components/ui/FormField";
import FormErrorSummary from "../components/ui/FormErrorSummary";

type Room = { id: string; name: string; capacity: number | null };

const createSchema = {
  createName: [{ type: "required" as const, message: "Name is required" }],
  createCapacity: [{ type: "min" as const, value: 1, message: "Capacity must be at least 1" }],
};

const editSchema = {
  editName: [{ type: "required" as const, message: "Name is required" }],
  editCapacity: [{ type: "min" as const, value: 1, message: "Capacity must be at least 1" }],
};

export default function Classrooms() {
  const { addToast } = useToast();
  const [search, setSearch] = useState("");

  const [createModal, setCreateModal] = useState(false);
  const [editModal, setEditModal] = useState<Room | null>(null);
  const [createForm, setCreateForm] = useState({ name: "", capacity: "" });
  const [editForm, setEditForm] = useState({ name: "", capacity: "" });

  const { data: rooms, loading, error, refetch } = useApiQuery<Room[]>("/api/v1/rooms");
  const { mutate: createRoom, loading: creating } = useApiMutation<{ name: string; capacity: number | null }, unknown>("POST");
  const { mutate: updateRoom, loading: updating } = useApiMutation<{ name: string; capacity: number | null }, unknown>("PUT");

  const createFormValues = { createName: createForm.name, createCapacity: createForm.capacity };
  const createValidation = useFormValidation(createSchema, createFormValues);

  const editFormValues = { editName: editForm.name, editCapacity: editForm.capacity };
  const editValidation = useFormValidation(editSchema, editFormValues);

  useEffect(() => {
    if (error) addToast("error", error.message);
  }, [error]);

  const saving = creating || updating;

  const filtered = useMemo(() => {
    if (!search.trim()) return rooms ?? [];
    const q = search.toLowerCase();
    return (rooms ?? []).filter((c) => c.name.toLowerCase().includes(q) || c.id.toLowerCase().includes(q));
  }, [search, rooms]);

  const handleCreate = async () => {
    if (!createValidation.validateAll()) return;
    const cap = createForm.capacity.trim() ? Number(createForm.capacity) : null;
    if (cap != null && (!Number.isFinite(cap) || cap <= 0)) {
      addToast("error", "Capacity must be a positive number");
      return;
    }
    try {
      await createRoom({ name: createForm.name, capacity: cap }, "/api/v1/rooms");
      addToast("success", "Room created");
      setCreateModal(false);
      setCreateForm({ name: "", capacity: "" });
      refetch();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Create failed");
    }
  };

  const openEdit = (r: Room) => {
    setEditModal(r);
    setEditForm({ name: r.name, capacity: r.capacity == null ? "" : String(r.capacity) });
    editValidation.clearErrors();
  };

  const handleEdit = async () => {
    if (!editModal) return;
    if (!editValidation.validateAll()) return;
    const cap = editForm.capacity.trim() ? Number(editForm.capacity) : null;
    if (cap != null && (!Number.isFinite(cap) || cap <= 0)) {
      addToast("error", "Capacity must be a positive number");
      return;
    }
    try {
      await updateRoom({ name: editForm.name, capacity: cap }, `/api/v1/rooms/${editModal.id}`);
      addToast("success", "Room updated");
      setEditModal(null);
      refetch();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Update failed");
    }
  };

  return (
    <div>
      <PageHeading>Classroom</PageHeading>
      <div className="flex flex-wrap items-center gap-2 mb-4">
        <SearchInput value={search} onChange={setSearch} placeholder="Search id / name..." />
        <Button variant="primary" size="md" onClick={() => setCreateModal(true)}>
          Create
        </Button>
      </div>
      <table className="w-full text-[13px]">
        <thead>
          <tr className="border-b-2 border-gray-300">
            <th className="text-left py-2 px-2 font-semibold">Name</th>
            <th className="text-left py-2 px-2 font-semibold">Capacity</th>
            <th className="text-left py-2 px-2 font-semibold"></th>
          </tr>
        </thead>
        <tbody>
          {filtered.map((c, idx) => (
            <tr key={c.id} className={`border-b border-gray-200 hover:bg-gray-50 ${idx % 2 === 1 ? "bg-gray-50/50" : ""}`}>
              <td className="py-2 px-2">{c.name}</td>
              <td className="py-2 px-2">{c.capacity ?? "-"}</td>
              <td className="py-2 px-2">
                <Button variant="primary" size="sm" onClick={() => openEdit(c)}>edit</Button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {loading && <LoadingSkeleton type="table" lines={3} />}
      {!loading && filtered.length === 0 && <EmptyState message="No rooms found" />}

      {createModal && (
        <Modal
          title="Create Classroom"
          onClose={() => setCreateModal(false)}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setCreateModal(false)}>Cancel</Button>
              <Button variant="primary" size="sm" onClick={handleCreate} loading={saving}>
                {saving ? "Creating…" : "Create"}
              </Button>
            </>
          }
        >
          <div className="space-y-3">
            <FormErrorSummary errors={createValidation.errors} touched={createValidation.touched} />

            <FormField name="createName" label="Name" error={createValidation.errors.createName} touched={createValidation.touched.createName} required>
              <Input size="sm" value={createForm.name} onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })} onBlur={() => { createValidation.touch("createName"); createValidation.validate("createName"); }} />
            </FormField>

            <FormField name="createCapacity" label="Capacity" error={createValidation.errors.createCapacity} touched={createValidation.touched.createCapacity}>
              <Input size="sm" type="number" value={createForm.capacity} onChange={(e) => setCreateForm({ ...createForm, capacity: e.target.value })} onBlur={() => { createValidation.touch("createCapacity"); createValidation.validate("createCapacity"); }} />
            </FormField>
          </div>
        </Modal>
      )}

      {editModal && (
        <Modal
          title="Edit Classroom"
          onClose={() => setEditModal(null)}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setEditModal(null)}>Cancel</Button>
              <Button variant="primary" size="sm" onClick={handleEdit} loading={saving}>
                {saving ? "Saving…" : "Save"}
              </Button>
            </>
          }
        >
          <div className="space-y-3">
            <FormErrorSummary errors={editValidation.errors} touched={editValidation.touched} />

            <FormField name="editName" label="Name" error={editValidation.errors.editName} touched={editValidation.touched.editName} required>
              <Input size="sm" value={editForm.name} onChange={(e) => setEditForm({ ...editForm, name: e.target.value })} onBlur={() => { editValidation.touch("editName"); editValidation.validate("editName"); }} />
            </FormField>

            <FormField name="editCapacity" label="Capacity" error={editValidation.errors.editCapacity} touched={editValidation.touched.editCapacity}>
              <Input size="sm" type="number" value={editForm.capacity} onChange={(e) => setEditForm({ ...editForm, capacity: e.target.value })} onBlur={() => { editValidation.touch("editCapacity"); editValidation.validate("editCapacity"); }} />
            </FormField>
          </div>
        </Modal>
      )}
    </div>
  );
}
