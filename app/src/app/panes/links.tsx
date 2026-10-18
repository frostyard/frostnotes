// The Backlinks and Outgoing links panes' containers (docs/specs/ui.md,
// Right sidebar panes): the active file's backlinks from note.backlinks,
// each with its line; its unlinked mentions from a search for its title
// and aliases, leaving out the notes that link to it; and its links from
// note.links. Each reads again whenever the vault's files change.
import { useEffect, useState } from "react";

import { useFiles } from "../../data/files";
import { notify } from "../../data/overlays";
import { useClient } from "../../data/session";
import { useNotes } from "../../data/stores";
import { useDocuments } from "../../editor/registry";
import { BacklinksPane, type MentionGroup } from "../../features/links/BacklinksPane";
import { OutgoingPane } from "../../features/links/OutgoingPane";
import { type OutgoingLink, outgoingLinks } from "../../lib/outgoing";
import type { Backlink, Client } from "../../rpc/gen/api";
import { activeTab } from "../../workspace/ops";
import { useWorkspace } from "../../workspace/store";
import { openOrCreate } from "../commands";
import { openAtLine, openFile, whereOf } from "./open";

const titleOf = (p: string) => p.slice(p.lastIndexOf("/") + 1).replace(/\.md$/i, "");

/** useActivePath is the active tab's file, or null. */
export function useActivePath(): string | null {
  return useWorkspace((s) => {
    const v = activeTab(s.ws).view;
    return v.type === "note" || v.type === "file" ? v.path : null;
  });
}

/** linkedGroups groups backlinks by their source, in the order notesd lists them. */
export function linkedGroups(backlinks: readonly Backlink[]): MentionGroup[] {
  const groups = new Map<string, MentionGroup & { mentions: MentionGroup["mentions"][number][] }>();
  for (const b of backlinks) {
    let g = groups.get(b.source);
    if (!g) {
      g = { path: b.source, title: titleOf(b.source), mentions: [] };
      groups.set(b.source, g);
    }
    const parts = b.context.length > 0 ? b.context : [{ text: b.link.target, match: true }];
    g.mentions.push({ line: b.link.line, parts });
  }
  return [...groups.values()];
}

// unlinkedGroups searches for the note's title and aliases as phrases,
// keeping the notes that do not link to it.
async function unlinkedGroups(
  c: Client,
  vaultId: number,
  path: string,
  linked: ReadonlySet<string>,
): Promise<MentionGroup[]> {
  const terms = [titleOf(path)];
  if (path.toLowerCase().endsWith(".md")) {
    const note = await c.note.read({ vaultId, path });
    terms.push(...note.aliases);
  }
  const groups = new Map<string, MentionGroup>();
  for (const term of terms) {
    if (term.trim() === "") continue;
    const hits = await c.note.search({ vaultId, query: `"${term.replaceAll('"', "")}"`, limit: 100 });
    for (const h of hits) {
      if (h.path === path || linked.has(h.path) || groups.has(h.path)) continue;
      groups.set(h.path, { path: h.path, title: h.title, mentions: [{ line: null, parts: h.snippet }] });
    }
  }
  return [...groups.values()];
}

/** BacklinksContainer draws the Backlinks pane for the active file. */
export function BacklinksContainer() {
  const client = useClient();
  const docs = useDocuments();
  const vaultId = useNotes((s) => s.vaultId);
  const files = useFiles((s) => s.files);
  const path = useActivePath();
  // The last answers, for the file they answer: kept while the next loads, so the pane does not flicker.
  const [state, setState] = useState<{ path: string; linked: MentionGroup[]; unlinked: MentionGroup[] | null } | null>(
    null,
  );

  // biome-ignore lint/correctness/useExhaustiveDependencies: files is the trigger: any change in the vault may add or drop a mention
  useEffect(() => {
    if (vaultId === null || path === null) return;
    let stale = false;
    void (async () => {
      try {
        const linked = linkedGroups(await client.note.backlinks({ vaultId, path }));
        if (stale) return;
        setState((s) => ({ path, linked, unlinked: s?.path === path ? s.unlinked : null }));
        const unlinked = await unlinkedGroups(client, vaultId, path, new Set(linked.map((g) => g.path)));
        if (!stale) setState({ path, linked, unlinked });
      } catch (err) {
        if (!stale) setState((s) => ({ path, linked: s?.path === path ? s.linked : [], unlinked: [] }));
        console.error("backlinks:", err);
      }
    })();
    return () => {
      stale = true;
    };
  }, [client, vaultId, path, files]);

  const current = state !== null && state.path === path ? state : null;
  return (
    <BacklinksPane
      linked={path === null ? null : (current?.linked ?? [])}
      unlinked={current?.unlinked ?? null}
      onOpen={(source, line, click) => {
        if (line === null || vaultId === null) return openFile(source, click);
        openAtLine(client, docs, vaultId, source, line, click).catch((err: unknown) =>
          notify(err instanceof Error ? err.message : String(err)),
        );
      }}
    />
  );
}

/** OutgoingContainer draws the Outgoing links pane for the active note. */
export function OutgoingContainer() {
  const client = useClient();
  const vaultId = useNotes((s) => s.vaultId);
  const files = useFiles((s) => s.files);
  const path = useActivePath();
  const [state, setState] = useState<{ path: string; links: OutgoingLink[] } | null>(null);

  // biome-ignore lint/correctness/useExhaustiveDependencies: files is the trigger: a new or moved file resolves links
  useEffect(() => {
    if (vaultId === null || path === null) return;
    let stale = false;
    client.note
      .links({ vaultId, path })
      .then((l) => !stale && setState({ path, links: outgoingLinks(l, path) }))
      .catch(() => !stale && setState({ path, links: [] }));
    return () => {
      stale = true;
    };
  }, [client, vaultId, path, files]);

  return (
    <OutgoingPane
      links={path === null ? null : state?.path === path ? state.links : []}
      onOpen={(link, click) => {
        if (link.path !== null) return openFile(link.path, click);
        openOrCreate(client, link.target, whereOf(click)).catch((err: unknown) =>
          notify(err instanceof Error ? err.message : String(err)),
        );
      }}
    />
  );
}
