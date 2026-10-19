import { EditorView } from "@codemirror/view";
import { describe, expect, it } from "vitest";

import { attachmentFolder, base64, embedFor, fileName, freePath, insertAt, pastedName, uploadFiles } from "./attach";

describe("attachments", () => {
  it("go where the vault's setting says", () => {
    expect(attachmentFolder("/", "Projects/Plan.md")).toBe("");
    expect(attachmentFolder("", "Plan.md")).toBe("");
    expect(attachmentFolder("./", "Projects/Plan.md")).toBe("Projects");
    expect(attachmentFolder("./", "Plan.md")).toBe("");
    expect(attachmentFolder("./assets", "Projects/Plan.md")).toBe("Projects/assets");
    expect(attachmentFolder("Assets/img/", "Projects/Plan.md")).toBe("Assets/img");
  });

  it("take a name no file has", () => {
    const taken = new Set(["Assets/photo.jpg", "Assets/photo 1.jpg"]);
    expect(freePath("Assets", "photo.jpg", (p) => taken.has(p))).toBe("Assets/photo 2.jpg");
    expect(freePath("", "new.png", (p) => taken.has(p))).toBe("new.png");
    expect(freePath("A", "README", () => false)).toBe("A/README");
  });

  it("name a pasted image as Obsidian does", () => {
    const at = new Date(2026, 9, 17, 9, 30, 5);
    expect(pastedName(at, "png")).toBe("Pasted image 20261017093005.png");
    expect(fileName({ name: "", type: "image/jpeg" }, at)).toBe("Pasted image 20261017093005.jpg");
    expect(fileName({ name: "image.png", type: "image/png" }, at)).toBe("Pasted image 20261017093005.png");
    expect(fileName({ name: "IMG_0001.HEIC", type: "image/heic" }, at)).toBe("IMG_0001.HEIC");
    expect(fileName({ name: "a/b:c.png", type: "image/png" }, at)).toBe("a-b-c.png");
  });

  it("embed by name, by path when the name is taken, or as markdown", () => {
    expect(embedFor("Assets/photo.jpg", true, true)).toBe("![[photo.jpg]]");
    expect(embedFor("Assets/photo.jpg", true, false)).toBe("![[Assets/photo.jpg]]");
    expect(embedFor("My Assets/a b.png", false, true)).toBe("![](My%20Assets/a%20b.png)");
  });

  it("encode bytes as base64", () => {
    expect(base64(new Uint8Array([0x89, 0x50, 0x4e, 0x47]))).toBe("iVBORw==");
    const big = new Uint8Array(100_000).fill(65);
    expect(atob(base64(big)).length).toBe(100_000);
  });

  it("upload through notesd and return the embeds", async () => {
    const calls: { vaultId: number; path: string; data: string }[] = [];
    const client = {
      note: { upload: async (p: { vaultId: number; path: string; data: string }) => void calls.push(p) },
    };
    const file = new File([new Uint8Array([1, 2, 3])], "photo.jpg", { type: "image/jpeg" });
    const text = await uploadFiles(
      {
        client: client as never,
        vaultId: 3,
        notePath: "Projects/Plan.md",
        settings: { attachmentFolder: "./assets", wikilinks: true },
        paths: () => ["Projects/assets/photo.jpg", "Other/photo 1.jpg"],
      },
      [file, file],
    );
    expect(calls.map((c) => c.path)).toEqual(["Projects/assets/photo 1.jpg", "Projects/assets/photo 2.jpg"]);
    expect(calls[0]?.data).toBe("AQID");
    expect(text).toBe("![[Projects/assets/photo 1.jpg]]\n![[photo 2.jpg]]");
    const big = new File([new Uint8Array(10 * 1024 * 1024 + 1)], "big.bin");
    await expect(
      uploadFiles({ client: client as never, vaultId: 3, notePath: "a.md", settings: null, paths: () => [] }, [big]),
    ).rejects.toThrow("big.bin is over 10 MB");
  });

  it("go into the note on a line of their own at a line's start, else at the cursor", () => {
    const view = new EditorView({ doc: "# Inbox\nsee " });
    insertAt(view, 0, "![[a.png]]");
    expect(view.state.doc.toString()).toBe("![[a.png]]\n# Inbox\nsee ");
    expect(view.state.selection.main.head).toBe(10);
    insertAt(view, view.state.doc.length, "![[b.png]]");
    expect(view.state.doc.toString()).toBe("![[a.png]]\n# Inbox\nsee ![[b.png]]");
    view.destroy();
  });
});
