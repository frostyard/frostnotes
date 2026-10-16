import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import { useNotes, useUI } from "../data/stores";
import { Client, PROTOCOL } from "../rpc/gen/api";
import { MockTransport, mockVaults } from "../rpc/mock/mock";
import { App } from "./App";

afterEach(() => {
  useNotes.setState({ connection: { state: "connecting" }, hello: null, vaults: [] });
  useUI.setState({ sidebarVisible: true });
});

function vaultNames(): string[] {
  const list = screen.queryByRole("list", { name: "Vault list" });
  if (!list) return [];
  return within(list)
    .getAllByRole("listitem")
    .map((li) => li.querySelector("span")?.textContent ?? "");
}

/** start renders the app over mock and returns a second client of the same mock, standing in for notesctl. */
async function start(mock: MockTransport) {
  render(<App connect={async () => mock} />);
  await screen.findByTestId("hello-server");
  const other = new Client(mock);
  await other.rpc.hello({ protocol: PROTOCOL, client: "notesctl" });
  return other;
}

describe("App over the mock transport", () => {
  it("shows rpc.hello and the vaults", async () => {
    await start(new MockTransport(mockVaults(), { server: "notesd test" }));
    expect(screen.getByTestId("hello-server").textContent).toBe("notesd test");
    expect(screen.getByTestId("hello-protocol").textContent).toBe("1");
    expect(vaultNames()).toEqual(["garden", "Notes", "Work"]);
    expect(screen.getByRole("status").textContent).toContain("Connected to notesd test");
    expect(screen.getByRole("status").textContent).toContain("3 vaults");
    expect(within(screen.getByRole("navigation", { name: "Vaults" })).getAllByRole("listitem")).toHaveLength(3);
  });

  it("keeps the vault list live from vault.added and vault.removed", async () => {
    const mock = new MockTransport(mockVaults());
    const notesctl = await start(mock);
    await act(async () => {
      await notesctl.vault.add({ path: "/home/demo/Inbox" });
    });
    await waitFor(() => expect(vaultNames()).toEqual(["garden", "Inbox", "Notes", "Work"]));
    await act(async () => {
      await notesctl.vault.remove({ id: 1 });
    });
    await waitFor(() => expect(vaultNames()).toEqual(["garden", "Inbox", "Work"]));
    expect(screen.getByRole("status").textContent).toContain("3 vaults");
  });

  it("says how to add a vault when there is none", async () => {
    await start(new MockTransport());
    expect(screen.getByText(/No vaults yet/)).toBeTruthy();
  });

  it("toggles the sidebar from the title bar", async () => {
    await start(new MockTransport(mockVaults()));
    await userEvent.setup().click(screen.getByRole("button", { name: "Toggle Sidebar" }));
    expect(screen.queryByRole("navigation", { name: "Vaults" })).toBeNull();
  });

  it("shows the reason and reconnects when the connection is lost", async () => {
    const first = new MockTransport(mockVaults());
    let mock = first;
    render(<App connect={async () => mock} />);
    await screen.findByTestId("hello-server");
    mock = new MockTransport([], { server: "notesd again" });
    act(() => first.close("notesd stopped"));
    expect(await screen.findByText("Reconnecting to notesd… (notesd stopped)", { selector: "div" })).toBeTruthy();
    await waitFor(() => expect(screen.getByTestId("hello-server").textContent).toBe("notesd again"), {
      timeout: 3000,
    });
    expect(screen.getByText(/No vaults yet/)).toBeTruthy();
  });
});
