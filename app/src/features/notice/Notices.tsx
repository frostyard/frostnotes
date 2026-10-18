// The window's notices (docs/specs/ui.md#notices): short messages stacked
// at the top right, each leaving on its own after a while or when
// clicked; one may carry an action. Timers follow each notice's first appearance.

import { useEffect, useRef } from "react";

/** NOTICE_MS and NOTICE_ACTION_MS are how long a notice stays without and with an action. */
export const NOTICE_MS = 4000;
export const NOTICE_ACTION_MS = 10000;

/** NoticeItem is a notice to show. */
export interface NoticeItem {
  /** Stable while the notice shows. */
  id: string;
  text: string;
  action?: { label: string; run: () => void };
}

export interface NoticesProps {
  /** The notices, oldest first: the top one is the oldest. */
  notices: NoticeItem[];
  /** The notice should go: its time is up, it was clicked, or its action ran. */
  onDismiss: (id: string) => void;
}

/** Notices draws the notice stack. */
export function Notices({ notices, onDismiss }: NoticesProps) {
  const timers = useRef(new Map<string, ReturnType<typeof setTimeout>>());
  const dismiss = useRef(onDismiss);

  useEffect(() => {
    dismiss.current = onDismiss;
  }, [onDismiss]);

  useEffect(() => {
    const visible = new Set(notices.map((notice) => notice.id));
    for (const [id, timer] of timers.current) {
      if (!visible.has(id)) {
        clearTimeout(timer);
        timers.current.delete(id);
      }
    }
    for (const notice of notices) {
      if (timers.current.has(notice.id)) continue;
      timers.current.set(
        notice.id,
        setTimeout(() => dismiss.current(notice.id), notice.action ? NOTICE_ACTION_MS : NOTICE_MS),
      );
    }
  }, [notices]);

  useEffect(() => {
    const active = timers.current;
    return () => {
      for (const timer of active.values()) clearTimeout(timer);
      active.clear();
    };
  }, []);

  function dismissNotice(id: string) {
    // Keep the entry until the notice leaves so later renders cannot restart it.
    clearTimeout(timers.current.get(id));
    onDismiss(id);
  }

  return (
    <div className="pointer-events-none fixed top-[52px] right-2 z-40 flex w-80 max-w-[calc(100vw-16px)] flex-col gap-2">
      {notices.map((notice) => (
        // biome-ignore lint/a11y/useKeyWithClickEvents: Clicking only hastens the timer; the action is a keyboard-accessible button.
        <div
          key={notice.id}
          role="status"
          className="pointer-events-auto flex items-center gap-3 rounded-lg border border-separator bg-toolbar px-3 py-2.5 text-xs leading-4 shadow-popover"
          onClick={() => dismissNotice(notice.id)}
        >
          <span className="min-w-0 flex-1 select-text wrap-break-word">{notice.text}</span>
          {notice.action && (
            <button
              type="button"
              className="shrink-0 bg-transparent text-xs leading-4 text-accent"
              onClick={(event) => {
                event.stopPropagation();
                notice.action?.run();
                dismissNotice(notice.id);
              }}
            >
              {notice.action.label}
            </button>
          )}
        </div>
      ))}
    </div>
  );
}
