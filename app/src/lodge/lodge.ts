// The web app's view of lodge's shell (docs/design/app.md#in-a-browser):
// the parts of @frostyard/lodge-client's Lodge the app uses, so the bridge
// and its tests take a fake, and the shell's scheme for the Theme
// preference's System.
import type { Lodge as LodgeClient } from "@frostyard/lodge-client";
import { create } from "zustand";

/** Lodge is what the app uses of lodge-client's connection to the shell. */
export type Lodge = Pick<
  LodgeClient,
  "theme" | "history" | "route" | "context" | "open" | "on" | "contentURL" | "apiURL"
>;

/** LodgeState is the shell the app runs in, when it runs in one. */
export interface LodgeState {
  lodge: Lodge | null;
  /** The shell's theme: what the Theme preference's System follows inside lodge. */
  scheme: "light" | "dark" | null;
}

export const useLodge = create<LodgeState>(() => ({ lodge: null, scheme: null }));
