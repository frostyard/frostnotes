// A sidebar as a drawer in a narrow window (docs/specs/ui.md#narrow-windows):
// over the workspace from its own edge, 84% of the width and at most 450,
// the workspace behind dimmed. A tap on the dimmed workspace or Escape
// closes it.
import { type ReactNode, useEffect, useRef } from "react";

export interface DrawerProps {
  side: "left" | "right";
  label: string;
  onClose: () => void;
  children: ReactNode;
}

/** Drawer draws an open drawer and its backdrop. */
export function Drawer(props: DrawerProps) {
  const panel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const before = document.activeElement;
    panel.current?.focus();
    return () => {
      if (before instanceof HTMLElement && before.isConnected) before.focus();
    };
  }, []);
  return (
    <div
      className="fixed inset-0 z-40 flex"
      style={{ justifyContent: props.side === "left" ? "flex-start" : "flex-end" }}
    >
      <div
        data-testid="drawer-backdrop"
        aria-hidden="true"
        className="absolute inset-0 bg-backdrop"
        onPointerDown={props.onClose}
      />
      <div
        ref={panel}
        role="dialog"
        aria-label={props.label}
        aria-modal="true"
        tabIndex={-1}
        className="relative flex h-full flex-col bg-sidebar shadow-popover"
        style={{ width: "min(84vw, 450px)", paddingTop: "env(safe-area-inset-top)" }}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.stopPropagation();
            props.onClose();
          }
        }}
      >
        {props.children}
      </div>
    </div>
  );
}
