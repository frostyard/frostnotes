// Finding the Files pane's rows in the window's tests: a file's row by its
// path, its folders opened first, as a user would.
import { fireEvent, screen, waitFor } from "@testing-library/react";

function rowAt(tree: HTMLElement, path: string): HTMLElement | undefined {
  return [...tree.querySelectorAll<HTMLElement>("[data-path]")].find((el) => el.dataset.path === path);
}

/** fileRow returns the Files pane's row for path, opening the folders that hold it. */
export function fileRow(path: string): HTMLElement {
  const tree = screen.getByRole("tree", { name: "Files" });
  const parts = path.split("/");
  for (let i = 1; i < parts.length; i++) {
    const folder = rowAt(tree, parts.slice(0, i).join("/"));
    if (folder?.getAttribute("aria-expanded") === "false") fireEvent.click(folder);
  }
  const row = rowAt(tree, path);
  if (!row) throw new Error(`the Files pane has no row for ${path}`);
  return row;
}

/** findFileRow waits for path's row to show, then returns it. */
export async function findFileRow(path: string): Promise<HTMLElement> {
  await waitFor(() => fileRow(path));
  return fileRow(path);
}
