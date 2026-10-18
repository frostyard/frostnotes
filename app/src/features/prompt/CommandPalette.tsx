// The command palette (docs/specs/ui.md, Prompts): Ctrl+P's prompt over
// the commands that apply now. Task T-0052 replaces this stub.
import type { PaletteCommand } from "../../lib/palette";

export interface CommandPaletteProps {
  /** The commands that apply now. */
  commands: readonly PaletteCommand[];
  /** The IDs of the commands run, newest first. */
  recent: readonly string[];
  onRun: (id: string) => void;
  onClose: () => void;
}

/** CommandPalette draws the command palette. */
export function CommandPalette(_props: CommandPaletteProps) {
  return null;
}
