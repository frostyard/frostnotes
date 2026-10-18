// The Search pane's container (docs/specs/ui.md, Search pane): it holds
// the query in the panes store, so the Tags and All properties panes and
// Ctrl+Shift+F can set it, and searches 250 ms after it last changed:
// note.search (at most 100 notes), or, for Obsidian's "[key]", note.list
// of the notes with that property.
import { useEffect, useState } from "react";

import { setSearch, usePanes } from "../../data/panes";
import { useClient } from "../../data/session";
import { useNotes } from "../../data/stores";
import { SearchPane } from "../../features/search/SearchPane";
import type { SearchHit } from "../../rpc/gen/api";
import { openFile } from "./open";

/** SEARCH_DELAY is how long after the last change the pane searches. */
export const SEARCH_DELAY = 250;

const PROPERTY = /^\[([^\]:]+)\]$/;

/** SearchContainer draws the Search pane over the window's vault. */
export function SearchContainer() {
  const client = useClient();
  const vaultId = useNotes((s) => s.vaultId);
  const query = usePanes((s) => s.searchQuery);
  const focus = usePanes((s) => s.searchFocus);
  const [answer, setAnswer] = useState<{ query: string; hits: SearchHit[] | null; error: string | null }>({
    query: "",
    hits: null,
    error: null,
  });

  useEffect(() => {
    const q = query.trim();
    if (vaultId === null || q === "") return;
    let stale = false;
    const timer = setTimeout(() => {
      const key = PROPERTY.exec(q)?.[1]?.trim();
      const run: Promise<SearchHit[]> = key
        ? client.note
            .list({ vaultId, property: key })
            .then((notes) => notes.map((n) => ({ path: n.path, title: n.title, snippet: [] })))
        : client.note.search({ vaultId, query: q, limit: 100 });
      run
        .then((hits) => !stale && setAnswer({ query: q, hits, error: null }))
        .catch(
          (err: unknown) =>
            !stale && setAnswer({ query: q, hits: null, error: err instanceof Error ? err.message : String(err) }),
        );
    }, SEARCH_DELAY);
    return () => {
      stale = true;
      clearTimeout(timer);
    };
  }, [client, vaultId, query]);

  // An answer for another query is not shown: the results follow the query.
  const current = answer.query === query.trim();
  return (
    <SearchPane
      query={query}
      onQuery={(q) => setSearch(q)}
      results={current ? answer.hits : null}
      error={current ? answer.error : null}
      focusRequest={focus}
      onOpen={(path, click) => openFile(path, click)}
    />
  );
}
