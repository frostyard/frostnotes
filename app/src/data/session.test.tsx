import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { Client, type Event, PROTOCOL } from "../rpc/gen/api";
import { MockTransport, mockVaults } from "../rpc/mock/mock";
import { feed } from "./events";
import { emptyFiles, useFiles } from "./files";
import { usePrefs } from "./prefs";
import { Session } from "./session";
import { useNotes } from "./stores";
import { emptyVault, useVaultState } from "./vaultState";

afterEach(() => {
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [], vaultId: null });
  useFiles.setState(emptyFiles(null));
  useVaultState.setState(emptyVault(null));
  usePrefs.getState().setPrefs({ lastVault: null });
});

/** Probe shows the stores as text. */
function Probe() {
  const vaultId = useNotes((s) => s.vaultId);
  const files = useFiles((s) => (s.loaded ? [...s.files.keys()].join(",") : "loading"));
  const settings = useVaultState((s) => (s.settings ? `wikilinks=${s.settings.wikilinks}` : "no settings"));
  const sync = useVaultState((s) => `sync=${s.syncAvailable}`);
  return <p data-testid="probe">{`vault=${vaultId} files=${files} ${settings} ${sync}`}</p>;
}

const probe = () => screen.getByTestId("probe").textContent;

function start(mock: MockTransport) {
  render(
    <Session connect={async () => mock} fallback={<p>connecting</p>}>
      <Probe />
    </Session>,
  );
}

async function notesctl(mock: MockTransport): Promise<Client> {
  const c = new Client(mock);
  await c.rpc.hello({ protocol: PROTOCOL, client: "notesctl" });
  return c;
}

describe("Session", () => {
  it("opens the vault the window showed last, with its files and settings", async () => {
    usePrefs.getState().setPrefs({ lastVault: "/home/demo/Documents/Work" });
    start(new MockTransport(mockVaults(), { files: { 2: { "Standup.md": "s\n" } } }));
    await waitFor(() => expect(probe()).toBe("vault=2 files=Standup.md wikilinks=true sync=true"));
  });

  it("opens the first vault when the last one is gone, and remembers it", async () => {
    usePrefs.getState().setPrefs({ lastVault: "/gone" });
    start(new MockTransport(mockVaults()));
    await waitFor(() => expect(probe()).toMatch(/^vault=3 /)); // "garden" sorts first
    expect(usePrefs.getState().lastVault).toBe("/home/demo/garden");
  });

  it("keeps the files and settings current from events", async () => {
    const mock = new MockTransport(mockVaults(), { files: { 3: { "a.md": "a\n" } } });
    start(mock);
    await waitFor(() => expect(probe()).toMatch(/files=a.md /));
    const other = await notesctl(mock);
    await act(async () => {
      await other.note.create({ vaultId: 3, path: "b.md", content: "b\n" });
      await other.note.move({ vaultId: 3, path: "a.md", to: "Old/a.md" });
      await other.vault.configure({ id: 3, wikilinks: false });
    });
    await waitFor(() => expect(probe()).toBe("vault=3 files=b.md,Old/a.md wikilinks=false sync=true"));
    expect(useFiles.getState().folders).toEqual(["Old"]);
  });

  it("switches to another vault when the window's is removed", async () => {
    const mock = new MockTransport(mockVaults(), { files: { 1: { "n.md": "n\n" } } });
    start(mock);
    await waitFor(() => expect(probe()).toMatch(/^vault=3 /));
    const other = await notesctl(mock);
    await act(async () => {
      await other.vault.remove({ id: 3 });
    });
    await waitFor(() => expect(probe()).toMatch(/^vault=1 files=n.md /));
  });

  it("replays what happened while it was away to the feed, and lists again", async () => {
    const mock = new MockTransport(mockVaults(), { files: { 3: { "a.md": "a\n" } } });
    start(mock);
    await waitFor(() => expect(probe()).toMatch(/files=a.md /));
    const seen: Event[] = [];
    const off = feed.listen((e) => seen.push(e));
    act(() => mock.close("notesd restarted"));
    mock.reopen();
    mock.vault(3).move("a.md", "Moved/a.md");
    await waitFor(() => expect(probe()).toMatch(/files=Moved\/a.md /), { timeout: 3000 });
    off();
    expect(seen.map((e) => e.event)).toEqual(["note.moved"]);
    expect(feed.since()).toBe(seen[0]?.seq);
  });
});
