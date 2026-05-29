import Button from "./ui/Button";

interface AutoSitInToggleProps {
  label: string;
  enabled: boolean;
  dirty: boolean;
  saving: boolean;
  onToggle: (enabled: boolean) => void;
  onSave: () => void;
}

export default function AutoSitInToggle({
  label,
  enabled,
  dirty,
  saving,
  onToggle,
  onSave,
}: AutoSitInToggleProps) {
  return (
    <div className="flex items-center gap-3">
      <label className="flex items-center gap-1.5 text-xs text-gray-600">
        <input
          type="checkbox"
          aria-label={`Auto sit-in for ${label}`}
          checked={enabled}
          onChange={(e) => onToggle(e.target.checked)}
        />
        Auto sit-in
      </label>
      <Button
        variant="primary"
        size="sm"
        aria-label={`Save auto sit-in for ${label}`}
        disabled={!dirty}
        loading={saving}
        onClick={onSave}
      >
        Save
      </Button>
    </div>
  );
}
