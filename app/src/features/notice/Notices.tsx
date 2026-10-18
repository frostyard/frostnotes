// The window's notices (docs/specs/ui.md#notices): short messages stacked
// at the top right, each leaving on its own after a while or when
// clicked; one may carry an action. Task T-0048 replaces this stub.

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
export function Notices(_props: NoticesProps) {
  return null;
}
