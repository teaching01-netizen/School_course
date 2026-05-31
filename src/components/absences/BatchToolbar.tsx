import Button from "@/components/ui/Button";

type BatchToolbarProps = {
  onSelectAll: () => void;
  onDeselectAll: () => void;
  onSelectAllCovers: () => void;
  onDeselectAllCovers: () => void;
  onSelectMornings: () => void;
  onSelectAfternoons: () => void;
  disabled?: boolean;
};

export default function BatchToolbar({
  onSelectAll,
  onDeselectAll,
  onSelectAllCovers,
  onDeselectAllCovers,
  onSelectMornings,
  onSelectAfternoons,
  disabled = false,
}: BatchToolbarProps) {
  return (
    <div className="flex flex-wrap gap-2" role="group" aria-label="Batch actions">
      <Button
        variant="secondary"
        size="sm"
        disabled={disabled}
        onClick={onSelectAll}
        aria-label="All absent"
      >
        All absent
      </Button>
      <Button
        variant="secondary"
        size="sm"
        disabled={disabled}
        onClick={onDeselectAll}
        aria-label="None absent"
      >
        None absent
      </Button>
      <Button
        variant="secondary"
        size="sm"
        disabled={disabled}
        onClick={onSelectAllCovers}
        aria-label="All cover"
      >
        All cover
      </Button>
      <Button
        variant="secondary"
        size="sm"
        disabled={disabled}
        onClick={onDeselectAllCovers}
        aria-label="None cover"
      >
        None cover
      </Button>
      <Button
        variant="secondary"
        size="sm"
        disabled={disabled}
        onClick={onSelectMornings}
        aria-label="All mornings"
      >
        All mornings
      </Button>
      <Button
        variant="secondary"
        size="sm"
        disabled={disabled}
        onClick={onSelectAfternoons}
        aria-label="All afternoons"
      >
        All afternoons
      </Button>
    </div>
  );
}
