import type { HistoryUpdate, LodgeEvents, LodgeLocation } from "@frostyard/lodge-client";
import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { App } from "../app/App";
import { openExternal } from "../app/system";
import { emptyFiles, useFiles } from "../data/files";
import { useOverlays } from "../data/overlays";
import { usePrefs } from "../data/prefs";
import { useNotes } from "../data/stores";
import { emptyVault, useVaultState } from "../data/vaultState";
import { MockTransport, mockVaults } from "../rpc/mock/mock";
import { emptyWorkspace } from "../workspace/model";
import { activeTab } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { LodgeBridge } from "./Bridge";
import { type Lodge, useLodge } from "./lodge";

afterEach(() => {
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [], vaultId: null });
  useFiles.setState(emptyFiles(null));
  useVaultState.setState(emptyVault(null));
  useWorkspace.setState({ vaultPath: null, ws: emptyWorkspace() });
  usePrefs.getState().setPrefs({ lastVault: null, theme: "system" });
  useLodge.setState({ lodge: null, scheme: null });
  useOverlays.setState({ prompt: null });
  localStorage.clear();
  delete document.documentElement.dataset.theme;
});

type Handlers = { [T in keyof LodgeEvents]: Set<(e: LodgeEvents[T]) => void> };

/** FakeLodge is lodge-client's connection as the shell would drive it. */
class FakeLodge implements Lodge {
  theme: "light" | "dark" = "dark";
  readonly routes: [string, string | undefined][] = [];
  readonly contexts: unknown[] = [];
  readonly opened: string[] = [];
  readonly handlers: Handlers = { navigate: new Set(), visible: new Set(), theme: new Set(), context: new Set() };
  history: Lodge["history"];

  constructor(start: string) {
    const location: LodgeLocation = { pathname: start, search: "", hash: "" };
    this.history = {
      location,
      push: () => {},
      replace: () => {},
      listen: (_fn: (u: HistoryUpdate) => void) => () => {},
    };
  }
  apiURL = () => "wss://lodge.example.ts.net/app/notes/api";
  contentURL = (path: string) => `https://lodge-content.example.ts.net/notes/${path}`;
  route = (path: string, title?: string) => void this.routes.push([path, title]);
  context = (c: unknown) => void this.contexts.push(c);
  open = (url: string) => void this.opened.push(url);
  on = <T extends keyof LodgeEvents>(type: T, fn: (e: LodgeEvents[T]) => void) => {
    const set = this.handlers[type] as Set<(e: LodgeEvents[T]) => void>;
    set.add(fn);
    return () => void set.delete(fn);
  };
  send<T extends keyof LodgeEvents>(type: T, e: LodgeEvents[T]) {
    for (const fn of this.handlers[type] as Set<(e: LodgeEvents[T]) => void>) fn(e);
  }
}

const files = { 3: { "a.md": "Alpha\n", "Deep/b c.md": "Beta\n", "paper.pdf": "%PDF-1.4" }, 1: { "n.md": "N\n" } };

async function start(route: string) {
  const lodge = new FakeLodge(route);
  const mock = new MockTransport(mockVaults(), { files, caller: "session", login: "e2e@example.com", node: "e2e" });
  render(
    <App connect={async () => mock}>
      <LodgeBridge lodge={lodge} />
    </App>,
  );
  await screen.findByRole("main", { name: "Workspace" });
  return { lodge, mock };
}

const activePath = () => {
  const v = activeTab(useWorkspace.getState().ws).view;
  return v.type === "empty" ? "" : v.path;
};

describe("LodgeBridge", () => {
  it("opens the frame's own route on load, then reports it with the note's context", async () => {
    const { lodge } = await start("/3/Deep/b%20c.md");
    await waitFor(() => expect(activePath()).toBe("Deep/b c.md"));
    await waitFor(() => expect(lodge.routes.at(-1)).toEqual(["/3/Deep/b%20c.md", "b c"]));
    expect(lodge.contexts.at(-1)).toEqual({ kind: "note", ref: "notes/3/Deep/b%20c.md", title: "b c" });
  });

  it("reports the vault's route for a note whose route would be too long", async () => {
    const long = `${"x".repeat(2100)}.md`;
    const lodge = new FakeLodge("/");
    const mock = new MockTransport(mockVaults(), { files: { 3: { [long]: "x\n" } }, caller: "session", node: "e2e" });
    render(
      <App connect={async () => mock}>
        <LodgeBridge lodge={lodge} />
      </App>,
    );
    await screen.findByRole("main", { name: "Workspace" });
    await act(async () => lodge.send("navigate", { route: `/3/${long}` }));
    await waitFor(() => expect(activePath()).toBe(long));
    await waitFor(() => expect(lodge.routes.at(-1)?.[0]).toBe("/3"));
  });

  it("opens another vault's note on navigate", async () => {
    const { lodge } = await start("/");
    await waitFor(() => expect(useNotes.getState().vaultId).toBe(3));
    await act(async () => lodge.send("navigate", { route: "/1/n.md?x=1#top" }));
    await waitFor(() => expect(useNotes.getState().vaultId).toBe(1));
    await waitFor(() => expect(activePath()).toBe("n.md"));
    await waitFor(() => expect(lodge.routes.at(-1)).toEqual(["/1/n.md", "n"]));
  });

  it("opens the quick switcher for a note that is gone", async () => {
    await start("/3/Gone.md");
    await waitFor(() => expect(useOverlays.getState().prompt).toBe("switcher"));
    expect(activePath()).toBe("");
  });

  it("follows the shell's theme while the preference is System", async () => {
    const { lodge } = await start("/");
    await waitFor(() => expect(document.documentElement.dataset.theme).toBe("dark"));
    act(() => lodge.send("theme", { scheme: "light" }));
    expect(document.documentElement.dataset.theme).toBe("light");
    act(() => usePrefs.getState().setPrefs({ theme: "dark" }));
    act(() => lodge.send("theme", { scheme: "light" }));
    expect(document.documentElement.dataset.theme).toBe("dark");
  });

  it("opens a PDF in a new tab through the shell, or as a download", async () => {
    const { lodge } = await start("/3/paper.pdf");
    const open = await screen.findByRole("button", { name: "Open in new tab" });
    act(() => open.click());
    act(() => screen.getByRole("button", { name: "Download" }).click());
    expect(lodge.opened).toHaveLength(2);
    expect(lodge.opened[1]).toBe(`${lodge.opened[0]}?download=1`);
  });

  it("asks the shell to open web links", async () => {
    const { lodge } = await start("/");
    await waitFor(() => expect(useLodge.getState().lodge).toBe(lodge));
    await openExternal("https://lodge.example.ts.net/mail/thread/42");
    expect(lodge.opened).toEqual(["https://lodge.example.ts.net/mail/thread/42"]);
  });
});
