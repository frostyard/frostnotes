// What a tab shows under its view header. An empty tab's links run
// commands; a note is its editor or its reading view; a file shows by its
// hash; history and merge tabs are their views.

import { useCommands } from "../commands/registry";
import { useFiles } from "../data/files";
import { usePrefs } from "../data/prefs";
import { useClient } from "../data/session";
import { useNotes } from "../data/stores";
import type { Notice } from "../editor/document";
import { MergePane } from "../editor/MergePane";
import { NoteEditor } from "../editor/NoteEditor";
import type { PreviewCommand } from "../editor/preview/framework";
import { EmptyTab } from "../features/workspace/EmptyTab";
import { extensionOf, fileType } from "../lib/fileTree";
import { ReadingView } from "../reading/ReadingView";
import { type MergeSide, tabTitle, type ViewState } from "../workspace/model";
import type { OpenWhere } from "../workspace/ops";
import { HistoryView } from "./HistoryView";
import { renameNote } from "./panes/files";

export interface ViewContentProps {
  tabId: string;
  view: ViewState;
  /** The file the view shows is in the vault. */
  exists: boolean;
  /** The tab is the window's active one: its editor takes focus. */
  active: boolean;
  /** Open a note from this tab. */
  onOpen: (path: string, where: OpenWhere) => void;
  /** An editor followed a link. */
  onFollow: (c: PreviewCommand) => void;
  /** A notice's Compare: open the merge view. */
  onCompare: (notice: Notice) => void;
  onClose: () => void;
  /** Keep where the cursor was in the note at path, for the tab. */
  onCursor: (pos: number, path: string) => void;
  /** Compare the note with another version: open the merge view. */
  onCompareWith: (path: string, other: MergeSide) => void;
  /** A merge view's Done. */
  onMergeDone: (path: string) => void;
}

/** ViewContent draws a tab's view. */
export function ViewContent(props: ViewContentProps) {
  const { view, exists } = props;
  const live = usePrefs((s) => s.editingMode === "live");
  const vaultId = useNotes((s) => s.vaultId);
  const commands = useCommands();
  const client = useClient();
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
  if (view.type === "note" && view.mode === "edit" && vaultId !== null) {
    return (
      <NoteEditor
        key={`${vaultId}:${props.tabId}`}
        tabId={props.tabId}
        vaultId={vaultId}
        path={view.path}
        live={live}
        cursor={view.cursor}
        autoFocus={props.active}
        onCommand={props.onFollow}
        onCompare={props.onCompare}
        onClose={props.onClose}
        onCursor={props.onCursor}
        onRename={(title) => void renameNote(client, vaultId, view.path, title)}
      />
    );
  }
  if (vaultId !== null) {
    switch (view.type) {
      case "note":
        return (
          <ReadingView
            key={`${vaultId}:${props.tabId}`}
            vaultId={vaultId}
            path={view.path}
            onFollow={props.onFollow}
            onCompare={props.onCompare}
            onClose={props.onClose}
            onRename={(title) => void renameNote(client, vaultId, view.path, title)}
          />
        );
      case "merge":
        return (
          <MergePane
            vaultId={vaultId}
            path={view.path}
            other={view.other}
            onDone={() => props.onMergeDone(view.path)}
          />
        );
      case "history":
        return (
          <HistoryView
            vaultId={vaultId}
            path={view.path}
            onCompare={(other) => props.onCompareWith(view.path, other)}
          />
        );
    }
  }
  if (!exists) {
    return <Message text={`${view.path} is not in the vault: it was deleted or moved outside the app.`} />;
  }
  return view.type === "file" ? <FileView path={view.path} /> : <Message text={`${tabTitle(view)} needs a vault.`} />;
}

function Message({ text }: { text: string }) {
  return <p className="text-empty mx-auto max-w-[700px] px-8 py-8 text-secondary">{text}</p>;
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
