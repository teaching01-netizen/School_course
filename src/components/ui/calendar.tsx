import {
  DayPicker,
  type DayPickerProps,
  getDefaultClassNames,
} from "react-day-picker";
import { cn } from "@/utils/cn";

type CalendarProps = DayPickerProps;

function Calendar({
  className,
  classNames,
  showOutsideDays = true,
  fixedWeeks = true,
  ...props
}: CalendarProps) {
  const defaultClassNames = getDefaultClassNames();
  const { root: rootClassName, ...restClassNames } = classNames ?? {};
  return (
    <DayPicker
      showOutsideDays={showOutsideDays}
      fixedWeeks={fixedWeeks}
      className={cn("w-full max-w-full p-2 sm:p-3", className)}
      classNames={{
        ...defaultClassNames,
        ...restClassNames,
        root: cn(defaultClassNames.root, "rdp w-full max-w-full", rootClassName),
      }}
      {...props}
    />
  );
}

export default Calendar;
export type { CalendarProps };
