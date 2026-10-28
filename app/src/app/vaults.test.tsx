// Vaults from the app (ADR-0024; plan 0005 Phase 8), in the window over
// the mock: the window is the vault manager without a vault; Create new
// vault and Open folder as vault register a folder and show it, or say
// why not; Manage vaults… switches in place and removes a vault after
// asking; the window's title names the note and the vault. In a browser
// there is no folder picker, so paths are typed.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import { emptyFiles, useFiles } from "../data/files";
import { useOverlays } from "../data/overlays";
import { usePrefs } from "../data/prefs";
import { useNotes } from "../data/stores";
import { emptyVault, useVaultState } from "../data/vaultState";
import { MockTransport, mockVaults } from "../rpc/mock/mock";
import { findFileRow } from "../testing/files";
import { emptyWorkspace } from "../workspace/model";
import { useWorkspace } from "../workspace/store";
import { App } from "./App";

afterEach(() => {
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [], vaultId: null, joinHolds: 0 });
  useFiles.setState(emptyFiles(null));
  useVaultState.setState(emptyVault(null));
  useWorkspace.setState({ vaultPath: null, ws: emptyWorkspace() });
  useOverlays.setState({ menu: null, prompt: null, confirm: null, vaults: false, notices: [] });
  usePrefs.getState().setPrefs({ lastVault: null });
  localStorage.clear();
  document.title = "";
});

const manager = () => screen.getByRole("dialog", { name: "Manage vaults" });
const vaultRow = (name: string) => within(manager()).getByRole("button", { name: new RegExp(`^${name}`) });

async function manageVaults(current: string) {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: new RegExp(current) }));
  await user.click(screen.getByRole("menuitem", { name: "Manage vaults…" }));
  return within(manager());
}

/** HUB is the hub the join tests reach. */
const HUB = "http://127.0.0.1:7788";

/** joinButton waits for Join, which comes once the manager hears that notesd syncs. */
async function joinButton() {
  return waitFor(() => {
    const row = screen.getByRole("region", { name: "Join a vault on a hub" });
    return within(row).getByRole("button", { name: "Join" });
  });
}

/** joinPanel is the open join panel's fields. */
const joinPanel = () => within(screen.getByRole("region", { name: "Join a vault on a hub" }));

/** fillJoin fills the open join panel and joins. */
async function fillJoin(user: ReturnType<typeof userEvent.setup>, location: string) {
  const panel = joinPanel();
  await user.type(panel.getByRole("textbox", { name: "Hub URL" }), HUB);
  await user.type(panel.getByRole("textbox", { name: "Hub vault name" }), "shared");
  fireEvent.change(panel.getByRole("textbox", { name: "Location" }), { target: { value: location } });
  await user.click(panel.getByRole("button", { name: "Join" }));
}

describe("vaults from the app", () => {
  it("is the vault manager without a vault, and creates one", async () => {
    const mock = new MockTransport([], { folders: ["/home/demo/Documents"] });
    render(<App connect={async () => mock} />);
    const m = within(await screen.findByRole("dialog", { name: "Manage vaults" }));
    expect(m.getByText("No vaults yet")).toBeTruthy();
    expect(screen.queryByRole("main", { name: "Workspace" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
    await waitFor(() => expect(document.title).toBe("Frostnotes"));

    const user = userEvent.setup();
    await user.click(
      within(m.getByRole("region", { name: "Create new vault" })).getByRole("button", { name: "Create" }),
    );
    await user.type(screen.getByRole("textbox", { name: "Vault name" }), "Garden");
    fireEvent.change(screen.getByRole("textbox", { name: "Location" }), { target: { value: "/home/demo/Documents/" } });
    await user.click(screen.getByRole("button", { name: "Create" }));

    await screen.findByRole("main", { name: "Workspace" });
    expect(mock.calls.find((c) => c.method === "vault.add")?.params).toEqual({
      path: "/home/demo/Documents/Garden",
      create: true,
    });
    expect(screen.queryByRole("dialog", { name: "Manage vaults" })).toBeNull();
    await waitFor(() => expect(document.title).toBe("Garden - Frostnotes"));
  });

  it("opens a folder as a vault, and says why it will not", async () => {
    const mock = new MockTransport([]);
    render(<App connect={async () => mock} />);
    const m = within(await screen.findByRole("dialog", { name: "Manage vaults" }));
    const user = userEvent.setup();
    await user.type(m.getByRole("textbox", { name: "Folder path" }), "/home/demo/Notes{Enter}");
    await screen.findByRole("main", { name: "Workspace" });

    const again = await manageVaults("Notes");
    const path = again.getByRole("textbox", { name: "Folder path" });
    await user.type(path, "/home/demo/Notes/daily{Enter}");
    expect(await screen.findByText("/home/demo/Notes/daily lies inside the vault Notes.")).toBeTruthy();
    await user.clear(path);
    await user.type(path, "/home/demo{Enter}");
    expect(await screen.findByText("/home/demo holds the vault Notes.")).toBeTruthy();
    expect(manager()).toBeTruthy();
    expect(mock.calls.filter((c) => c.method === "vault.add")).toHaveLength(3);

    await user.click(again.getByRole("region", { name: "Create new vault" }).querySelector("button") as HTMLElement);
    await user.type(screen.getByRole("textbox", { name: "Vault name" }), "Recipes");
    fireEvent.change(screen.getByRole("textbox", { name: "Location" }), { target: { value: "/elsewhere" } });
    await user.click(screen.getByRole("button", { name: "Create" }));
    expect(await screen.findByText("/elsewhere is not a folder.")).toBeTruthy();
  });

  it("joins a vault on a hub, from the manager, with no terminal", async () => {
    const mock = new MockTransport(mockVaults());
    render(<App connect={async () => mock} />);
    await screen.findByRole("main", { name: "Workspace" });
    const user = userEvent.setup();
    await manageVaults("garden");
    await user.click(await joinButton());
    await fillJoin(user, "/home/demo");

    // The manager closes once the vault is made, joined and open.
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Manage vaults" })).toBeNull());
    expect(mock.calls.find((c) => c.method === "vault.add")?.params).toEqual({
      path: "/home/demo/shared",
      create: true,
    });
    expect(mock.calls.find((c) => c.method === "sync.join")?.params).toEqual({
      vaultId: 4,
      url: HUB,
      name: "shared",
    });
    await waitFor(() => expect(document.title).toBe("shared - Frostnotes"));
    expect(useNotes.getState().joinHolds).toBe(0);
  });

  it("keeps the panel while a first vault pairs again and joins", async () => {
    // The hub refuses the token once: the panel pairs again, shows its code,
    // and joins, with the window still on the manager (data/session.tsx, the
    // hold) even though notesd sends vault.added before vault.add answers.
    const mock = new MockTransport([], {
      folders: ["/home/demo/Documents"],
      paired: [HUB],
      addEventFirst: true,
      latency: 5,
      pendingPairings: 3,
    });
    mock.refuseJoins(1);
    render(<App connect={async () => mock} />);
    const user = userEvent.setup();
    await screen.findByRole("dialog", { name: "Manage vaults" });
    await user.click(await joinButton());
    await fillJoin(user, "/home/demo/Documents");

    // The refused join pairs again: the panel is still there to show the code.
    expect(await screen.findByText("notesctl hub approve ABCD-EFGH")).toBeTruthy();
    await waitFor(() => expect(mock.calls.filter((c) => c.method === "sync.join")).toHaveLength(2));
    expect(mock.calls.find((c) => c.method === "sync.join")?.params).toEqual({
      vaultId: 1,
      url: HUB,
      name: "shared",
    });
    await waitFor(() => expect(document.title).toBe("shared - Frostnotes"));
    expect(mock.calls.some((c) => c.method === "vault.remove")).toBe(false);
    expect(useNotes.getState().joinHolds).toBe(0);
  });

  it("checks whether notesd syncs before it offers Join", async () => {
    const mock = new MockTransport([], { folders: ["/home/demo/Documents"], latency: 200 });
    render(<App connect={async () => mock} />);
    const m = within(await screen.findByRole("dialog", { name: "Manage vaults" }));
    expect(m.getByText("Checking whether this notesd syncs…")).toBeTruthy();
    expect(m.queryByRole("button", { name: "Join" })).toBeNull();
    await waitFor(() => expect(m.getByRole("button", { name: "Join" })).toBeTruthy());
  });

  it("checks again with a new connection", async () => {
    const first = new MockTransport([], { folders: ["/home/demo/Documents"] });
    const second = new MockTransport([], { folders: ["/home/demo/Documents"], latency: 200 });
    const view = render(<App connect={async () => first} />);
    const m = within(await screen.findByRole("dialog", { name: "Manage vaults" }));
    await waitFor(() => expect(m.getByRole("button", { name: "Join" })).toBeTruthy());
    view.rerender(<App connect={async () => second} />);
    expect(await m.findByText("Checking whether this notesd syncs…")).toBeTruthy();
    await waitFor(() => expect(m.getByRole("button", { name: "Join" })).toBeTruthy());
  });

  it("hides Join where notesd does not sync", async () => {
    const mock = new MockTransport([], { folders: ["/home/demo/Documents"], noSync: true });
    render(<App connect={async () => mock} />);
    const m = within(await screen.findByRole("dialog", { name: "Manage vaults" }));
    expect(await m.findByText("This notesd does not sync.")).toBeTruthy();
    expect(m.queryByRole("button", { name: "Join" })).toBeNull();
  });

  it("goes back from the form and makes no vault", async () => {
    const mock = new MockTransport([], { folders: ["/home/demo/Documents"] });
    render(<App connect={async () => mock} />);
    const user = userEvent.setup();
    await screen.findByRole("dialog", { name: "Manage vaults" });
    await user.click(await joinButton());
    await user.type(screen.getByRole("textbox", { name: "Hub vault name" }), "shared");
    await user.click(screen.getByRole("button", { name: "Back" }));
    expect(within(manager()).getByRole("button", { name: "Create" })).toBeTruthy();
    expect(mock.calls.some((c) => c.method === "vault.add")).toBe(false);
    expect(mock.calls.some((c) => c.method === "sync.join")).toBe(false);
  });

  it("removes the vault it made when the person cancels the pairing again", async () => {
    const mock = new MockTransport(mockVaults(), { paired: [HUB], latency: 2, pendingPairings: 1000 });
    mock.refuseJoins(1);
    render(<App connect={async () => mock} />);
    await screen.findByRole("main", { name: "Workspace" });
    const user = userEvent.setup();
    await manageVaults("garden");
    await user.click(await joinButton());
    await fillJoin(user, "/home/demo");

    await screen.findByText("notesctl hub approve ABCD-EFGH");
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(useNotes.getState().joinHolds).toBe(0));
    expect(mock.calls.find((c) => c.method === "vault.remove")?.params).toEqual({ id: 4 });
    expect(mock.calls.filter((c) => c.method === "sync.join")).toHaveLength(1);
    await waitFor(() => expect(within(manager()).getAllByRole("listitem")).toHaveLength(3));
  });

  it("keeps a new flow's hold when a cancelled flow ends late", async () => {
    // Flow A waits for its approval when the person cancels it, and its reply
    // comes after flow B takes its own hold: A ending must not release B's,
    // or B's first vault would open and take B's panel away. Each flow makes
    // its vault in a folder of its own: a removed vault keeps its folder.
    const mock = new MockTransport([], {
      folders: ["/home/demo/Documents", "/home/demo/Notes"],
      paired: [HUB],
      addEventFirst: true,
      latency: 20,
    });
    mock.refuseJoins(1);
    mock.holdPairings();
    render(<App connect={async () => mock} />);
    const user = userEvent.setup();
    await screen.findByRole("dialog", { name: "Manage vaults" });
    await user.click(await joinButton());
    await fillJoin(user, "/home/demo/Documents");

    // A's join is refused once, so A pairs again and waits on its code.
    await screen.findByText("notesctl hub approve ABCD-EFGH");
    // Only now, so that the gate holds B's join and not A's refused one.
    mock.holdJoins();
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    // Flow B starts in another folder while A still waits for its reply.
    await user.click(await joinButton());
    await fillJoin(user, "/home/demo/Notes");
    mock.releasePairings();

    // A ends and releases its own hold; B's stands, so B's first vault opens
    // nothing and B's panel stays.
    await waitFor(() => expect(useNotes.getState().joinHolds).toBe(1));
    await waitFor(() => expect(useNotes.getState().vaults.map((v) => v.id)).toEqual([2]));
    expect(useNotes.getState().vaultId).toBeNull();
    expect(screen.getByRole("region", { name: "Join a vault on a hub" })).toBeTruthy();

    // B's join goes through, and B's vault opens.
    mock.releaseJoins();
    await waitFor(() => expect(useNotes.getState().vaultId).toBe(2));
    await waitFor(() => expect(document.title).toBe("shared - Frostnotes"));
    expect(mock.calls.filter((c) => c.method === "sync.join")).toHaveLength(2);
    expect(mock.calls.filter((c) => c.method === "vault.remove")).toHaveLength(1);
    expect(useNotes.getState().joinHolds).toBe(0);
  });

  it("makes no vault when the manager closes while it waits for the approval", async () => {
    const mock = new MockTransport(mockVaults(), { latency: 2, pendingPairings: 1000 });
    render(<App connect={async () => mock} />);
    await screen.findByRole("main", { name: "Workspace" });
    const user = userEvent.setup();
    await manageVaults("garden");
    await user.click(await joinButton());
    await fillJoin(user, "/home/demo");

    await screen.findByText("notesctl hub approve ABCD-EFGH");
    fireEvent.keyDown(manager(), { key: "Escape" });

    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Manage vaults" })).toBeNull());
    await waitFor(() => expect(useNotes.getState().joinHolds).toBe(0));
    expect(mock.calls.some((c) => c.method === "vault.add")).toBe(false);
    expect(mock.calls.some((c) => c.method === "sync.join")).toBe(false);
  });

  it("forgets the new vault when the hub refuses it, and says why in the panel", async () => {
    const mock = new MockTransport(mockVaults(), { joinRefusals: 1 });
    render(<App connect={async () => mock} />);
    await screen.findByRole("main", { name: "Workspace" });
    const user = userEvent.setup();
    await manageVaults("garden");
    await user.click(await joinButton());
    await fillJoin(user, "/home/demo");

    expect(await screen.findByText(/refused this device's token/)).toBeTruthy();
    expect(mock.calls.find((c) => c.method === "vault.remove")?.params).toEqual({ id: 4 });
    await waitFor(() => expect(within(manager()).getAllByRole("listitem")).toHaveLength(3));
    expect(within(manager()).getByRole("region", { name: "Join a vault on a hub" })).toBeTruthy();
  });

  it("says both when the join fails and the new vault cannot go", async () => {
    const mock = new MockTransport(mockVaults(), { joinRefusals: 1, removeRefusals: 1 });
    render(<App connect={async () => mock} />);
    await screen.findByRole("main", { name: "Workspace" });
    const user = userEvent.setup();
    await manageVaults("garden");
    await user.click(await joinButton());
    await fillJoin(user, "/home/demo");

    expect(await screen.findByText(/could not remove the new vault "shared" from the list/)).toBeTruthy();
    expect(screen.getByText(/Remove it in Manage vaults\./)).toBeTruthy();
    await waitFor(() => expect(within(manager()).getAllByRole("listitem")).toHaveLength(4));
  });

  it("switches vaults in place and names them in the title", async () => {
    const mock = new MockTransport(mockVaults(), { files: { 3: { "Plan.md": "Plan\n" } } });
    render(<App connect={async () => mock} />);
    await screen.findByRole("main", { name: "Workspace" });
    await waitFor(() => expect(document.title).toBe("garden - Frostnotes"));
    const user = userEvent.setup();
    await user.click(await findFileRow("Plan.md"));
    await waitFor(() => expect(document.title).toBe("Plan - garden - Frostnotes"));

    const m = await manageVaults("garden");
    expect(m.getAllByRole("listitem").map((r) => r.textContent)).toEqual([
      "garden/home/demo/garden",
      "Notes/home/demo/Notes",
      "Work/home/demo/Documents/Work",
    ]);
    expect(vaultRow("garden").getAttribute("aria-current")).toBe("true");
    await user.click(vaultRow("Work"));
    expect(screen.queryByRole("dialog", { name: "Manage vaults" })).toBeNull();
    await waitFor(() => expect(document.title).toBe("Work - Frostnotes"));
    expect(screen.getByRole("button", { name: /^Work/ })).toBeTruthy();
    expect(await screen.findByText("No notes yet")).toBeTruthy(); // Work's Files pane
  });

  it("removes a vault from the list after asking, and shows another", async () => {
    const mock = new MockTransport(mockVaults());
    render(<App connect={async () => mock} />);
    await screen.findByRole("main", { name: "Workspace" });
    const user = userEvent.setup();
    const m = await manageVaults("garden");
    await user.click(m.getByRole("button", { name: "Options for garden" }));
    await user.click(screen.getByRole("menuitem", { name: "Remove from list" }));
    const ask = await screen.findByRole("alertdialog", { name: 'Remove "garden" from the list?' });
    expect(ask.textContent).toContain(
      "Its folder and files stay where they are. Frostnotes forgets its index and version history on this device.",
    );
    await user.click(within(ask).getByRole("button", { name: "Cancel" }));
    expect(mock.calls.some((c) => c.method === "vault.remove")).toBe(false);

    await user.click(m.getByRole("button", { name: "Options for garden" }));
    await user.click(screen.getByRole("menuitem", { name: "Remove from list" }));
    await user.click(
      within(await screen.findByRole("alertdialog", { name: 'Remove "garden" from the list?' })).getByRole("button", {
        name: "Remove",
      }),
    );
    await waitFor(() => expect(mock.calls.find((c) => c.method === "vault.remove")?.params).toEqual({ id: 3 }));
    await waitFor(() => expect(document.title).toBe("Notes - Frostnotes"));
    expect(m.getAllByRole("listitem").map((r) => r.textContent?.split("/")[0])).toEqual(["Notes", "Work"]);
  });

  it("opens the manager with Open another vault, and closes it", async () => {
    const mock = new MockTransport(mockVaults());
    render(<App connect={async () => mock} />);
    await screen.findByRole("main", { name: "Workspace" });
    const user = userEvent.setup();
    await user.keyboard("{Control>}p{/Control}");
    await user.type(await screen.findByRole("combobox"), "Open another vault");
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("dialog", { name: "Manage vaults" })).toBeTruthy();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Manage vaults" })).toBeNull());
  });
});
