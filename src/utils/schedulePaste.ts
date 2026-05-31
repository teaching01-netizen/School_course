export type ParsedSchedulePasteRow = {
  rowNumber: number;
  date: string;
  begin: string;
  end: string;
  duration: string;
  classroom: string;
};

export type SchedulePasteError = {
  rowNumber: number;
  message: string;
};

const MONTHS: Record<string, number> = {
  jan: 1,
  feb: 2,
  mar: 3,
  apr: 4,
  may: 5,
  jun: 6,
  jul: 7,
  aug: 8,
  sep: 9,
  oct: 10,
  nov: 11,
  dec: 12,
};

function pad2(value: number): string {
  return String(value).padStart(2, "0");
}

function parseScheduleDate(value: string): string | null {
  const parts = value.trim().split(/\s+/);
  const dateParts = parts.length === 4 ? parts.slice(1) : parts;
  if (dateParts.length !== 3) return null;

  const day = Number(dateParts[0]);
  const month = MONTHS[dateParts[1].slice(0, 3).toLowerCase()];
  const rawYear = Number(dateParts[2]);
  if (!Number.isInteger(day) || !month || !Number.isInteger(rawYear)) return null;

  const year = rawYear < 100 ? 2000 + rawYear : rawYear;
  const dt = new Date(Date.UTC(year, month - 1, day));
  if (dt.getUTCFullYear() !== year || dt.getUTCMonth() !== month - 1 || dt.getUTCDate() !== day) {
    return null;
  }

  return `${year}-${pad2(month)}-${pad2(day)}`;
}

function isTime(value: string): boolean {
  const match = value.match(/^(\d{1,2}):([0-5]\d)$/);
  if (!match) return false;
  const hour = Number(match[1]);
  return hour >= 0 && hour <= 23;
}

function isHeader(columns: string[]): boolean {
  return columns[0]?.trim().toLowerCase() === "date" && columns[1]?.trim().toLowerCase() === "begin";
}

function splitColumns(line: string): string[] {
  if (line.includes("\t")) return line.split("\t");
  return line.trim().split(/\s{2,}/);
}

export function parseSchedulePaste(input: string): { rows: ParsedSchedulePasteRow[]; errors: SchedulePasteError[] } {
  const rows: ParsedSchedulePasteRow[] = [];
  const errors: SchedulePasteError[] = [];

  input
    .replace(/\r\n/g, "\n")
    .split("\n")
    .forEach((line, index) => {
      const rowNumber = index + 1;
      if (!line.trim()) return;

      const columns = splitColumns(line);
      if (isHeader(columns)) return;

      const date = parseScheduleDate(columns[0] ?? "");
      const begin = (columns[1] ?? "").trim();
      const end = (columns[2] ?? "").trim();
      const duration = (columns[3] ?? "").trim();
      const classroom = (columns[4] ?? "").trim();

      if (!date) {
        errors.push({ rowNumber, message: "Invalid date" });
        return;
      }
      if (!isTime(begin)) {
        errors.push({ rowNumber, message: "Invalid begin time" });
        return;
      }
      if (!isTime(end)) {
        errors.push({ rowNumber, message: "Invalid end time" });
        return;
      }

      rows.push({ rowNumber, date, begin, end, duration, classroom });
    });

  return { rows, errors };
}
