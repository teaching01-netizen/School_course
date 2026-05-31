export function formatDate(iso: string): string {
  if (!iso) return "";
  // Ensure the date is parsed in local timezone context without shifting
  const datePart = iso.split("T")[0];
  const d = new Date(`${datePart}T00:00:00`);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("en-GB", {
    weekday: "short",
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}

export function formatTime(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    // If it's a simple time format like "14:30:00", handle it
    const parts = iso.split(":");
    if (parts.length >= 2) return `${parts[0]}:${parts[1]}`;
    return iso;
  }
  return d.toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function formatDateTime(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("en-GB", {
    weekday: "short",
    day: "numeric",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
