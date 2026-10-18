// The Search pane (docs/specs/ui.md, Search pane): a controlled query
// field and matching notes with foldable, highlighted snippets.
import { ChevronRight, Search, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { SearchHit } from "../../rpc/gen/api";
import { type PaneClick, paneClick, paneEmpty, paneRow } from "../panes/common";

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
export function SearchPane({ query, onQuery, results, error, focusRequest, onOpen }: SearchPaneProps) {
  const field = useRef<HTMLInputElement>(null);
  const [folded, setFolded] = useState<ReadonlySet<string>>(() => new Set());

  useEffect(() => {
    if (focusRequest !== 0) {
      field.current?.focus();
      field.current?.select();
    }
  }, [focusRequest]);

  function toggle(path: string) {
    setFolded((previous) => {
      const next = new Set(previous);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  }

  return (
    <div>
      <div className="relative m-2 flex h-7 items-center rounded-md border border-separator bg-window">
        <Search size={14} aria-hidden="true" className="pointer-events-none absolute left-2 text-secondary" />
        <input
          ref={field}
          type="search"
          aria-label="Search"
          placeholder="Search…"
          spellCheck={false}
          value={query}
          onChange={(event) => onQuery(event.currentTarget.value)}
          className="text-sidebar-row h-full w-full min-w-0 bg-transparent px-7 outline-none [&::-webkit-search-cancel-button]:appearance-none"
        />
        {query !== "" && (
          <button
            type="button"
            aria-label="Clear search"
            className="absolute right-1 flex size-5 items-center justify-center text-secondary"
            onClick={() => {
              onQuery("");
              field.current?.focus();
            }}
          >
            <X size={12} aria-hidden="true" />
          </button>
        )}
      </div>
      {query.trim() !== "" &&
        (error !== null ? (
          <p className={paneEmpty}>{error}</p>
        ) : results === null ? null : (
          <>
            <p className="text-status px-4 text-secondary">
              {results.length} {results.length === 1 ? "result" : "results"}
            </p>
            <ul aria-label="Results">
              {results.map((hit) => {
                const open = !folded.has(hit.path);
                const slash = hit.path.lastIndexOf("/");
                const folder = slash < 0 ? "" : hit.path.slice(0, slash);
                return (
                  <li key={hit.path}>
                    <div className={paneRow}>
                      <button
                        type="button"
                        tabIndex={-1}
                        aria-label={`${open ? "Fold" : "Unfold"} ${hit.title}`}
                        aria-expanded={open}
                        className="flex h-full shrink-0 items-center justify-center text-secondary"
                        onClick={() => toggle(hit.path)}
                      >
                        <ChevronRight size={14} aria-hidden="true" className={open ? "rotate-90" : ""} />
                      </button>
                      <button
                        type="button"
                        title={hit.path}
                        className="flex h-full min-w-0 flex-1 items-center gap-2 text-left"
                        onClick={(event) => onOpen(hit.path, paneClick(event))}
                        onAuxClick={(event) => {
                          if (event.button === 1) onOpen(hit.path, paneClick(event));
                        }}
                      >
                        <span className="truncate font-semibold">{hit.title}</span>
                        {folder && <span className="truncate text-xs leading-4 text-secondary">{folder}</span>}
                      </button>
                    </div>
                    {open && hit.snippet.length > 0 && (
                      <button
                        type="button"
                        className="mx-2 block w-[calc(100%-1rem)] rounded-md pl-6 text-left text-xs leading-4 hover:bg-selection-inactive"
                        onClick={(event) => onOpen(hit.path, paneClick(event))}
                        onAuxClick={(event) => {
                          if (event.button === 1) onOpen(hit.path, paneClick(event));
                        }}
                      >
                        {hit.snippet.map((part, index) =>
                          part.match ? (
                            // biome-ignore lint/suspicious/noArrayIndexKey: Snippet parts are known by their place.
                            <mark key={index} className="bg-highlight text-inherit">
                              {part.text}
                            </mark>
                          ) : (
                            // biome-ignore lint/suspicious/noArrayIndexKey: Snippet parts are known by their place.
                            <span key={index}>{part.text}</span>
                          ),
                        )}
                      </button>
                    )}
                  </li>
                );
              })}
            </ul>
          </>
        ))}
    </div>
  );
}
