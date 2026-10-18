// How the history writes a version's time and size (docs/specs/ui.md,
// History), using local calendar days and binary size units.

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

/** whenText writes at as the history shows it, seen from now. */
export function whenText(at: Date, now: Date): string {
  const yesterday = new Date(now);
  yesterday.setDate(yesterday.getDate() - 1);
  const day = sameDay(at, now)
    ? "Today"
    : sameDay(at, yesterday)
      ? "Yesterday"
      : `${MONTHS[at.getMonth()]} ${at.getDate()}, ${at.getFullYear()}`;
  return `${day} at ${at.getHours()}:${String(at.getMinutes()).padStart(2, "0")}`;
}

/** sizeText writes a size: "512 bytes", "1 byte", "1.5 KB", "12 KB", "2.3 MB". */
export function sizeText(bytes: number): string {
  if (bytes < 1024) return `${bytes} ${bytes === 1 ? "byte" : "bytes"}`;
  const unit = bytes < 1024 * 1024 ? "KB" : "MB";
  const size = bytes / (unit === "KB" ? 1024 : 1024 * 1024);
  return `${Number(size.toFixed(size < 10 ? 1 : 0))} ${unit}`;
}
