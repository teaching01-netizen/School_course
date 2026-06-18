import { addDays, startOfWeek } from 'date-fns';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import Button from '../ui/Button';

type WeekNavigatorProps = {
  weekStart: Date;
  onChange: (weekStart: Date) => void;
};

export default function WeekNavigator({ weekStart, onChange }: WeekNavigatorProps) {
  return (
    <div className="flex items-center gap-1.5">
      <Button
        variant="ghost"
        size="sm"
        aria-label="Previous week"
        onClick={() => onChange(addDays(weekStart, -7))}
      >
        <ChevronLeft className="h-4 w-4" />
      </Button>
      <Button
        variant="ghost"
        size="sm"
        onClick={() => onChange(startOfWeek(new Date(), { weekStartsOn: 1 }))}
      >
        Today
      </Button>
      <Button
        variant="ghost"
        size="sm"
        aria-label="Next week"
        onClick={() => onChange(addDays(weekStart, 7))}
      >
        <ChevronRight className="h-4 w-4" />
      </Button>
    </div>
  );
}
