// The Search pane (docs/specs/ui.md, Search pane). Task T-0058 replaces
// this stub.
import type { SearchHit } from "../../rpc/gen/api";
import type { PaneClick } from "../panes/common";

export interface SearchPaneProps {
  query: string;
  onQuery: (query: string) => void;
  /** The hits for the query; null before the first answer for it. */
  results: readonly SearchHit[] | null;
  /** Why the search failed. */
  error: string | null;
  /** Each new value focuses the field and selects its text. */
  focusRequest: number;
  onOpen: (path: string, click: PaneClick) => void;
}

/** SearchPane draws the Search pane. */
export function SearchPane(_props: SearchPaneProps) {
  return null;
}
