import { forwardRef } from "react";
import SearchableSelect, {
  type SearchableSelectProps,
  type SearchableSelectSize,
} from "./SearchableSelect";

type SelectProps = SearchableSelectProps;
type SelectSize = SearchableSelectSize;

const Select = forwardRef<HTMLSelectElement, SelectProps>((props, ref) => (
  <SearchableSelect ref={ref} triggerMode="native" {...props} />
));

Select.displayName = "Select";
export default Select;
export type { SelectProps, SelectSize };
