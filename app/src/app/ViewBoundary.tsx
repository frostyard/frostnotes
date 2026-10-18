// A tab's error boundary: a view that fails while it draws shows why in
// its tab, and the rest of the window works on (plan 0005, Phase 7).
import { Component, type ReactNode } from "react";

export interface ViewBoundaryProps {
  /** What the view shows, for the message. */
  label: string;
  /** Names the view: a new one (another tab, note or kind of view) is drawn afresh. */
  view: string;
  children: ReactNode;
}

interface ViewBoundaryState {
  error: Error | null;
  view: string;
}

/** ViewBoundary keeps one view's failure inside its tab. */
export class ViewBoundary extends Component<ViewBoundaryProps, ViewBoundaryState> {
  override state: ViewBoundaryState = { error: null, view: this.props.view };

  static getDerivedStateFromError(error: unknown): Partial<ViewBoundaryState> {
    return { error: error instanceof Error ? error : new Error(String(error)) };
  }

  static getDerivedStateFromProps(
    props: ViewBoundaryProps,
    state: ViewBoundaryState,
  ): Partial<ViewBoundaryState> | null {
    return props.view === state.view ? null : { error: null, view: props.view };
  }

  override componentDidCatch(error: unknown): void {
    console.error(`${this.props.label} failed:`, error);
  }

  override render(): ReactNode {
    if (this.state.error) {
      return (
        <p role="alert" className="text-empty px-8 py-4 text-secondary">
          Cannot show {this.props.label}: {this.state.error.message}
        </p>
      );
    }
    return this.props.children;
  }
}
