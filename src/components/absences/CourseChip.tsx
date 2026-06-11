import ToggleSwitch from "./ToggleSwitch";

type CourseChipProps = {
  id?: string;
  name: string;
  code: string;
  selected: boolean;
  onToggle: () => void;
  disabled?: boolean;
  tabIndex?: number;
};

export default function CourseChip({
  id,
  name,
  code,
  selected,
  onToggle,
  disabled = false,
}: CourseChipProps) {
  return (
    <ToggleSwitch
      id={id ?? `course-${code}`}
      checked={selected}
      onChange={onToggle}
      label={name}
      description={code}
      disabled={disabled}
    />
  );
}
