import { INSTITUTE_TIME_ZONE } from "@/features/absences/domain/sessionGrouping";
import { formatZoneDateKey } from "@/utils/timezone";
import type { MakeUpOption } from "./MakeUpPicker";

/** One selectable make-up day for the sheet's date strip. Days come from the
 *  institute-local `dateKey` supplied by `normalizeSitInDisplayModel`, never
 *  from parsing display labels. */
export type MakeUpDay = {
  key: string;
  weekday: string;
  dayNumber: string;
  monthShort: string;
  monthLabel: string;
  ariaLabel: string;
  total: number;
  availableCount: number;
  hasAvailable: boolean;
};

function formatDayPart(key: string, format: string, fallback: string): string {
  return formatZoneDateKey(key, INSTITUTE_TIME_ZONE, format) ?? fallback;
}

/** Distinct institute days in the option list, chronological. Options without a
 *  day key (fallback callers/tests) are ignored here and stay in the flat list. */
export function makeUpDays(options: MakeUpOption[]): MakeUpDay[] {
  const byDay = new Map<string, MakeUpOption[]>();
  for (const option of options) {
    const key = option.display?.dateKey;
    if (!key) continue;
    const bucket = byDay.get(key);
    if (bucket) bucket.push(option);
    else byDay.set(key, [option]);
  }
  return [...byDay.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, dayOptions]) => {
      const availableCount = dayOptions.filter((option) => !option.disabled).length;
      return {
        key,
        weekday: formatDayPart(key, "ccc", key),
        dayNumber: formatDayPart(key, "d", ""),
        monthShort: formatDayPart(key, "LLL", ""),
        monthLabel: formatDayPart(key, "LLLL yyyy", ""),
        ariaLabel: formatDayPart(key, "cccc d LLLL", key),
        total: dayOptions.length,
        availableCount,
        hasAvailable: availableCount > 0,
      };
    });
}

/** The day the sheet opens on: the day of the staged/committed choice when that
 *  option still exists (even when it became unavailable, so its reason is
 *  visible), otherwise the first day with a selectable time, otherwise the
 *  first day. Empty string means "no day model" (flat list). */
export function defaultMakeUpDay(options: MakeUpOption[], days: MakeUpDay[], stagedValue: string): string {
  if (days.length === 0) return "";
  const staged = stagedValue ? options.find((option) => option.value === stagedValue) : undefined;
  const stagedDay = staged?.display?.dateKey;
  if (stagedDay && days.some((day) => day.key === stagedDay)) return stagedDay;
  return days.find((day) => day.hasAvailable)?.key ?? days[0].key;
}

function sortByAvailability(options: MakeUpOption[]): MakeUpOption[] {
  return options.slice().sort((a, b) => Number(Boolean(a.disabled)) - Number(Boolean(b.disabled)));
}

/** Options shown for a day. Key-less options stay visible on every day so no
 *  option can become unreachable behind the filter. */
export function optionsForDay(options: MakeUpOption[], day: string, publicForm: boolean): MakeUpOption[] {
  if (!day) return publicForm ? sortByAvailability(options) : options;
  const dayOptions = options.filter((option) => !option.display?.dateKey || option.display.dateKey === day);
  return publicForm ? sortByAvailability(dayOptions) : dayOptions;
}

/** Institute day of the first option that appeared after a "Show more times"
 *  request, or null when nothing was added. */
export function firstAddedOptionDay(
  options: MakeUpOption[],
  previousValues: ReadonlySet<string>,
): string | null {
  const added = options.find((option) => !previousValues.has(option.value));
  return added?.display?.dateKey || null;
}
