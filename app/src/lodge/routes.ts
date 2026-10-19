// Notes' routes inside lodge (docs/design/app.md#in-a-browser): a note's
// canonical URL is /notes/<vault id>/<path>, each segment percent-encoded,
// and its route, relative to the app's root, /<vault id>/<path>.

/** NoteRoute is what a route names: a vault, and a file in it ("" for none). */
export interface NoteRoute {
  vaultId: number;
  path: string;
}

/** noteRoute is the route of a vault's file, or of the vault alone when path is "". */
export function noteRoute(vaultId: number, path: string): string {
  const rest = path === "" ? "" : `/${path.split("/").map(encodeURIComponent).join("/")}`;
  return `/${vaultId}${rest}`;
}

/** noteRef is the context's ref for a note: notes/<vault id>/<path>, as the route has it. */
export function noteRef(vaultId: number, path: string): string {
  return `notes${noteRoute(vaultId, path)}`;
}

/** parseNoteRoute reads a route's pathname; null when it names no vault, or a segment does not decode. */
export function parseNoteRoute(pathname: string): NoteRoute | null {
  const [, first, ...rest] = pathname.split("/");
  if (first === undefined || !/^[1-9][0-9]{0,15}$/.test(first)) return null;
  const segments = rest.filter((s, i) => s !== "" || i < rest.length - 1);
  try {
    const path = segments.map(decodeURIComponent).join("/");
    if (segments.some((s) => s === "" || s === "." || s === "..")) return null;
    return { vaultId: Number(first), path };
  } catch {
    return null;
  }
}

/** noteTitle is how the shell titles a note: its name without .md. */
export function noteTitle(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1).replace(/\.md$/, "");
}
