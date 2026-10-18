// A notice in a tab (docs/specs/ui.md#notices-in-a-tab): a 32-high bar
// under the view header with its actions at the right.

/** TabNoticeAction is a notice's button. */
export interface TabNoticeAction {
  label: string;
  onClick: () => void;
}

export interface TabNoticeProps {
  text: string;
  actions: TabNoticeAction[];
}

/** TabNotice draws a notice in a tab. */
export function TabNotice({ text, actions }: TabNoticeProps) {
  return (
    <div role="status" className="flex h-8 shrink-0 items-center gap-3 bg-banner px-3 text-[12px] leading-4">
      <span className="min-w-0 flex-1 truncate" title={text}>
        {text}
      </span>
      {actions.map((a) => (
        <button key={a.label} type="button" className="shrink-0 text-accent hover:underline" onClick={a.onClick}>
          {a.label}
        </button>
      ))}
    </div>
  );
}
