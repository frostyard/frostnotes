// An empty tab's content (docs/specs/ui.md#empty-tab): four links that run
// commands.

/** EmptyTabAction is a link and the command it runs. */
export interface EmptyTabAction {
  command: string;
  label: string;
  keys: string;
  disabled: boolean;
}

export interface EmptyTabProps {
  actions: EmptyTabAction[];
  onRun: (command: string) => void;
}

/** EmptyTab draws an empty tab. */
export function EmptyTab({ actions, onRun }: EmptyTabProps) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2">
      {actions.map((a) => (
        <button
          key={a.label}
          type="button"
          disabled={a.disabled}
          onClick={() => onRun(a.command)}
          className="text-empty text-accent hover:underline disabled:text-tertiary disabled:no-underline"
        >
          {a.label} ({a.keys})
        </button>
      ))}
    </div>
  );
}
