// The editing toolbar of a narrow window (docs/specs/ui.md#narrow-windows):
// a row 52 high on the on-screen keyboard, scrolling sideways, with
// Obsidian's default phone commands in its order. A tap runs a command
// without taking the focus from the editor, so the keyboard stays.
import {
  Bold,
  Brackets,
  Code,
  Hash,
  Heading,
  Highlighter,
  Image,
  IndentDecrease,
  IndentIncrease,
  Italic,
  Link,
  List,
  ListChecks,
  ListOrdered,
  Paperclip,
  Percent,
  Redo2,
  Strikethrough,
  TextQuote,
  Undo2,
} from "lucide-react";
import type { ReactNode } from "react";

/** ToolbarItem is one of the toolbar's commands. */
export interface ToolbarItem {
  command: string;
  label: string;
  icon: ReactNode;
}

/** TOOLBAR is Obsidian's default phone toolbar, in its order. */
export const TOOLBAR: readonly ToolbarItem[] = [
  { command: "editor:undo", label: "Undo", icon: <Undo2 size={20} /> },
  { command: "editor:redo", label: "Redo", icon: <Redo2 size={20} /> },
  { command: "editor:insert-wikilink", label: "Add internal link", icon: <Brackets size={20} /> },
  { command: "editor:insert-embed", label: "Add embed", icon: <Image size={20} /> },
  { command: "editor:insert-tag", label: "Add tag", icon: <Hash size={20} /> },
  { command: "editor:attach-file", label: "Attach file", icon: <Paperclip size={20} /> },
  { command: "editor:set-heading", label: "Set heading", icon: <Heading size={20} /> },
  { command: "editor:toggle-bold", label: "Bold", icon: <Bold size={20} /> },
  { command: "editor:toggle-italics", label: "Italic", icon: <Italic size={20} /> },
  { command: "editor:toggle-strikethrough", label: "Strikethrough", icon: <Strikethrough size={20} /> },
  { command: "editor:toggle-highlight", label: "Highlight", icon: <Highlighter size={20} /> },
  { command: "editor:toggle-code", label: "Code", icon: <Code size={20} /> },
  { command: "editor:toggle-blockquote", label: "Blockquote", icon: <TextQuote size={20} /> },
  { command: "editor:toggle-comments", label: "Comment", icon: <Percent size={20} /> },
  { command: "editor:insert-link", label: "Insert markdown link", icon: <Link size={20} /> },
  { command: "editor:toggle-bullet-list", label: "Bullet list", icon: <List size={20} /> },
  { command: "editor:toggle-numbered-list", label: "Numbered list", icon: <ListOrdered size={20} /> },
  { command: "editor:toggle-checklist-status", label: "Toggle checklist", icon: <ListChecks size={20} /> },
  { command: "editor:indent-list", label: "Indent", icon: <IndentIncrease size={20} /> },
  { command: "editor:unindent-list", label: "Unindent", icon: <IndentDecrease size={20} /> },
];

export interface EditingToolbarProps {
  items?: readonly ToolbarItem[];
  /** How far above the layout viewport's bottom the keyboard reaches, in pixels. */
  bottom: number;
  onRun: (command: string) => void;
}

/** EditingToolbar draws the toolbar docked to the keyboard. */
export function EditingToolbar(props: EditingToolbarProps) {
  return (
    <div
      role="toolbar"
      aria-label="Editing toolbar"
      className="fixed inset-x-0 z-30 flex h-[52px] items-center gap-1 overflow-x-auto border-t border-separator bg-toolbar px-1"
      style={{ bottom: props.bottom }}
    >
      {(props.items ?? TOOLBAR).map((item) => (
        <button
          key={item.command}
          type="button"
          aria-label={item.label}
          title={item.label}
          className="flex size-11 shrink-0 items-center justify-center rounded-md text-secondary active:bg-selection-inactive"
          // Keep the editor focused, and the keyboard open.
          onPointerDown={(e) => e.preventDefault()}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => props.onRun(item.command)}
        >
          {item.icon}
        </button>
      ))}
    </div>
  );
}
