// The command registry (docs/design/app.md#commands-and-keys): every
// command once, by Obsidian's ID where Obsidian has the command, with when
// it applies and what it does. The keys, the menus and the command palette
// all run commands through it.
import { createContext, useContext } from "react";

/** Command is a thing the app can do. */
export interface Command {
  /** Obsidian's command ID where Obsidian has the command ("workspace:new-tab"). */
  id: string;
  /** The name the palette and menus show ("New tab"). */
  name: string;
  /** Whether the command applies now (an editor is active, a note is open); always when absent. */
  applies?: () => boolean;
  run: () => void | Promise<void>;
}

/** CommandRegistry holds the window's commands. */
export class CommandRegistry {
  private readonly commands = new Map<string, Command>();

  /** register adds commands, replacing any with the same ID; it returns their removal. */
  register(...commands: Command[]): () => void {
    for (const c of commands) this.commands.set(c.id, c);
    return () => {
      for (const c of commands) if (this.commands.get(c.id) === c) this.commands.delete(c.id);
    };
  }

  get(id: string): Command | undefined {
    return this.commands.get(id);
  }

  /** applies reports whether a command exists and applies now. */
  applies(id: string): boolean {
    const c = this.commands.get(id);
    return c !== undefined && (c.applies?.() ?? true);
  }

  /** list returns the commands that apply now, by name. */
  list(): Command[] {
    return [...this.commands.values()]
      .filter((c) => c.applies?.() ?? true)
      .sort((a, b) => a.name.localeCompare(b.name));
  }

  /** run runs a command when it applies, and reports whether it ran. */
  run(id: string): boolean {
    const c = this.commands.get(id);
    if (!c || !(c.applies?.() ?? true)) return false;
    void Promise.resolve(c.run()).catch((err: unknown) => console.error(`command ${id}:`, err));
    return true;
  }
}

/** CommandsContext gives components the window's registry. */
export const CommandsContext = createContext<CommandRegistry | null>(null);

/** useCommands returns the window's command registry. */
export function useCommands(): CommandRegistry {
  const r = useContext(CommandsContext);
  if (!r) throw new Error("useCommands outside a CommandsContext");
  return r;
}
