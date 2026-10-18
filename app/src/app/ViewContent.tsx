// What a tab shows under its view header. An empty tab's links run
// commands; a file shows by its hash; a note shows its text read-only
// until the editor (Phase 5's editor core) takes its place, and the
// history and merge views come with Phase 5's last part.
import { useEffect, useState } from "react";

import { useCommands } from "../commands/registry";
import { useFiles } from "../data/files";
import { useClient } from "../data/session";
import { useNotes } from "../data/stores";
import { EmptyTab } from "../features/workspace/EmptyTab";
import { extensionOf, fileType } from "../lib/fileTree";
import { tabTitle, type ViewState } from "../workspace/model";
import type { OpenWhere } from "../workspace/ops";
import { whereFor } from "./open";

export interface ViewContentProps {
  view: ViewState;
  /** The file the view shows is in the vault. */
  exists: boolean;
  /** Open a note from this tab. */
  onOpen: (path: string, where: OpenWhere) => void;
}

/** ViewContent draws a tab's view. */
export function ViewContent({ view, exists, onOpen }: ViewContentProps) {
  const commands = useCommands();
  if (view.type === "empty") {
    const action = (command: string, label: string, keys: string) => ({
      command,
      label,
      keys,
      disabled: !commands.applies(command),
    });
    return (
      <EmptyTab
        actions={[
          action("file-explorer:new-file", "Create new note", "Ctrl + N"),
          action("switcher:open", "Go to file", "Ctrl + O"),
          action("switcher:open", "See recent files", "Ctrl + O"),
          action("workspace:close", "Close", "Ctrl + W"),
        ]}
        onRun={(c) => commands.run(c)}
      />
    );
  }
  if (!exists) {
    return <Message text={`${view.path} is not in the vault: it was deleted or moved outside the app.`} />;
  }
  switch (view.type) {
    case "note":
      return <NoteText path={view.path} title={tabTitle(view)} onOpen={onOpen} />;
    case "file":
      return <FileView path={view.path} />;
    case "history":
    case "merge":
      return <Message text={`${tabTitle(view)} opens with the history and merge views.`} />;
  }
}

function Message({ text }: { text: string }) {
  return <p className="text-empty mx-auto max-w-[700px] px-8 py-8 text-secondary">{text}</p>;
}

// NoteText shows a note's text as notesd reads it, again whenever its hash
// changes; wikilinks open their targets as a click on a link would.
function NoteText({ path, title, onOpen }: { path: string; title: string; onOpen: ViewContentProps["onOpen"] }) {
  const c = useClient();
  const vaultId = useNotes((s) => s.vaultId);
  const hash = useFiles((s) => s.files.get(path)?.hash);
  const [text, setText] = useState<string | null>(null);
  useEffect(() => {
    if (vaultId === null || hash === undefined) return;
    let stale = false;
    c.note
      .read({ vaultId, path })
      .then((n) => !stale && setText(n.content))
      .catch((err: unknown) => !stale && setText(`Cannot read ${path}: ${err instanceof Error ? err.message : err}`));
    return () => {
      stale = true;
    };
  }, [c, vaultId, path, hash]);
  const links = text === null ? [] : [...new Set([...text.matchAll(/\[\[([^\]|#]+)/g)].map((m) => m[1] ?? ""))];
  const files = useFiles((s) => s.files);
  const resolve = (target: string) =>
    [...files.keys()].find(
      (p) => p === target || p === `${target}.md` || p.endsWith(`/${target}.md`) || p.endsWith(`/${target}`),
    );
  return (
    <article className="mx-auto max-w-[700px] px-8 pt-8 pb-[50vh]">
      <h1 className="mb-[0.5em] text-[1.618em] leading-[1.2] font-bold">{title}</h1>
      <pre className="font-[inherit] text-[16px] leading-normal whitespace-pre-wrap select-text">{text ?? ""}</pre>
      {links.length > 0 && (
        <nav aria-label="Links" className="mt-6 flex flex-wrap gap-2 border-t border-separator pt-3 text-[13px]">
          {links.map((target) => {
            const to = resolve(target);
            return (
              <button
                key={target}
                type="button"
                disabled={to === undefined}
                className="text-accent hover:underline disabled:text-tertiary"
                onClick={(e) => to && onOpen(to, whereFor(e))}
                onAuxClick={(e) => to && e.button === 1 && onOpen(to, "tab")}
              >
                {target}
              </button>
            );
          })}
        </nav>
      )}
    </article>
  );
}

// FileView shows an attachment by its hash: an image, audio or video
// inline, anything else by name.
function FileView({ path }: { path: string }) {
  const c = useClient();
  const hash = useFiles((s) => s.files.get(path)?.hash);
  const url = hash === undefined ? "" : (c.transport.blobURL?.(hash, extensionOf(path)) ?? "");
  const name = path.slice(path.lastIndexOf("/") + 1);
  switch (fileType(path)) {
    case "image":
      return (
        <div className="flex justify-center p-8">
          <img src={url} alt={name} className="max-w-full" />
        </div>
      );
    case "audio":
      // biome-ignore lint/a11y/useMediaCaption: a vault's audio has no captions to offer
      return <audio src={url} controls className="m-8" />;
    case "video":
      // biome-ignore lint/a11y/useMediaCaption: a vault's video has no captions to offer
      return <video src={url} controls className="mx-auto max-w-full p-8" />;
    default:
      return <Message text={`${name} has no view in the app yet.`} />;
  }
}
