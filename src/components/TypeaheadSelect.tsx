export type TypeaheadOption = { value: string; label: string; keywords?: string };

import SearchableSelect from "./ui/SearchableSelect";

export default function TypeaheadSelect(props: {
  id?: string;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
  value: string;
  onChange: (value: string) => void;
  options: TypeaheadOption[];
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}) {
  return (
    <SearchableSelect
      id={props.id}
      value={props.value}
      onValueChange={props.onChange}
      options={props.options}
      placeholder={props.placeholder}
      disabled={props.disabled}
      className={props.className}
      aria-invalid={props["aria-invalid"]}
      aria-describedby={props["aria-describedby"]}
      triggerMode="input"
      searchPlaceholder={props.placeholder ?? "Search options…"}
    />
  );
}
