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
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [], vaultId: null });
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
    const m = await manageVaults("garden");
    const hubRow = within(m.getByRole("region", { name: "Join a vault on a hub" }));
    await user.click(hubRow.getByRole("button", { name: "Join" }));
    const panel = within(screen.getByRole("region", { name: "Join a vault on a hub" }));
    await user.type(panel.getByRole("textbox", { name: "Hub URL" }), "http://127.0.0.1:7788/");
    await user.type(panel.getByRole("textbox", { name: "Hub vault name" }), "shared");
    fireEvent.change(panel.getByRole("textbox", { name: "Location" }), { target: { value: "/home/demo" } });
    await user.click(panel.getByRole("button", { name: "Join" }));

    // The manager closes once the vault is made, joined and open.
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Manage vaults" })).toBeNull());
    expect(mock.calls.find((c) => c.method === "vault.add")?.params).toEqual({
      path: "/home/demo/shared",
      create: true,
    });
    expect(mock.calls.find((c) => c.method === "sync.join")?.params).toEqual({
      vaultId: 4,
      url: "http://127.0.0.1:7788",
      name: "shared",
    });
    await waitFor(() => expect(document.title).toBe("shared - Frostnotes"));
  });

  it("forgets the new vault when the hub refuses it, and says why in the panel", async () => {
    const mock = new MockTransport(mockVaults(), { joinRefusals: 1 });
    render(<App connect={async () => mock} />);
    await screen.findByRole("main", { name: "Workspace" });
    const user = userEvent.setup();
    const m = await manageVaults("garden");
    const hubRow = within(m.getByRole("region", { name: "Join a vault on a hub" }));
    await user.click(hubRow.getByRole("button", { name: "Join" }));
    const panel = within(screen.getByRole("region", { name: "Join a vault on a hub" }));
    await user.type(panel.getByRole("textbox", { name: "Hub URL" }), "http://127.0.0.1:7788");
    await user.type(panel.getByRole("textbox", { name: "Hub vault name" }), "shared");
    fireEvent.change(panel.getByRole("textbox", { name: "Location" }), { target: { value: "/home/demo" } });
    await user.click(panel.getByRole("button", { name: "Join" }));

    expect(await screen.findByText(/refused this device's token/)).toBeTruthy();
    expect(mock.calls.find((c) => c.method === "vault.remove")?.params).toEqual({ id: 4 });
    await waitFor(() => expect(within(manager()).getAllByRole("listitem")).toHaveLength(3));
    expect(within(manager()).getByRole("region", { name: "Join a vault on a hub" })).toBeTruthy();
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
