export function extractLegacyCourseId(url: string): string | null {
  const trimmed = url.trim();
  try {
    const parsed = new URL(trimmed);
    const id = parsed.searchParams.get("id");
    if (id && /^\d+$/.test(id)) return id;
  } catch {
    // Not a full URL; fall through to raw query string/id parsing.
  }
  const match = trimmed.match(/[?&]id=(\d+)/);
  if (match) return match[1];
  if (/^\d+$/.test(trimmed)) return trimmed;
  return null;
}

export function formatLegacySyncTime(value: string | null | undefined): string {
  if (!value) return "Not synced yet";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Unavailable";
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(date);
}
