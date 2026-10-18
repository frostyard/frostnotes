// How the history writes a version's time and size (docs/specs/ui.md,
// History). Task T-0061 replaces this stub.

/** whenText writes at as the history shows it, seen from now. */
export function whenText(at: Date, _now: Date): string {
  return at.toISOString();
}

/** sizeText writes a size: "512 bytes", "1 byte", "1.5 KB", "12 KB", "2.3 MB". */
export function sizeText(bytes: number): string {
  return String(bytes);
}
