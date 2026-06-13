import { useState } from "react";
import Button from "../ui/Button";

type Props = {
  onSearch: (wcode: string) => void;
  loading: boolean;
};

export default function CrossStudyStudentSearch({ onSearch, loading }: Props) {
  const [wcode, setWcode] = useState("");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = wcode.trim();
    if (trimmed) onSearch(trimmed);
  };

  return (
    <form onSubmit={handleSubmit} className="flex items-end gap-2">
      <div className="flex-1">
        <label className="block text-xs font-semibold text-gray-500 uppercase tracking-wider mb-1">
          Search WCode
        </label>
        <input
          type="text"
          value={wcode}
          onChange={(e) => setWcode(e.target.value)}
          placeholder="e.g. W12345"
          className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
        />
      </div>
      <Button type="submit" variant="primary" size="md" loading={loading} disabled={loading || !wcode.trim()}>
        {loading ? "Searching…" : "Search"}
      </Button>
    </form>
  );
}
