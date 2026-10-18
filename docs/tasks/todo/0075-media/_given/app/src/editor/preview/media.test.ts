// T-0075's given tests: embedded images, audio and video in live preview,
// drawn in an editor's view.
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorSelection, EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { afterEach, describe, expect, it } from "vitest";

import { markdownSupport } from "../syntax/index";
import { BASIC } from "./basic";
import { livePreview, previewEnv } from "./framework";
import { MEDIA } from "./media";

let view: EditorView | null = null;
afterEach(() => {
  view?.destroy();
  view = null;
});

const RESOLVE: Record<string, string | null> = {
  "map.png": "media/map.png",
  "song.mp3": "audio/song.mp3",
  "clip.mp4": "clip.mp4",
  Plan: "Plan.md",
  "doc.pdf": "doc.pdf",
  "media/map.png": "media/map.png",
  "my map.png": "media/my map.png",
  Gone: null,
};

function open(doc: string, cursor: number | string = doc.length, remote = false): EditorView {
  const anchor = typeof cursor === "number" ? cursor : doc.indexOf(cursor);
  const state = EditorState.create({
    doc,
    selection: EditorSelection.cursor(anchor),
    extensions: [
      markdownSupport(),
      livePreview([...BASIC, ...MEDIA]),
      previewEnv.of({
        resolve: (t) => RESOLVE[t],
        command: () => {},
        fileURL: (p) => `blob:${p}`,
        remoteImages: () => remote,
      }),
    ],
  });
  ensureSyntaxTree(state, state.doc.length, 10_000);
  view = new EditorView({ state, parent: document.body });
  return view;
}

const media = (v: EditorView) => [...v.contentDOM.querySelectorAll<HTMLElement>(".cm-pv-media")];
const attrs = (v: EditorView, ...names: string[]) => media(v).map((el) => names.map((n) => el.getAttribute(n)));

describe("embeds", () => {
  it("draw a vault image in its line, away from the selection's lines", () => {
    const v = open("See ![[map.png]] here\n\nend");
    expect(media(v).map((el) => [el.tagName, el.dataset.pv])).toEqual([["IMG", "media"]]);
    expect(attrs(v, "src", "alt", "width")).toEqual([["blob:media/map.png", "map.png", null]]);
    expect(v.contentDOM.querySelector(".cm-line")?.textContent).toBe("See  here");
  });

  it("size an image by an alias of digits, else take the alias as its text", () => {
    const v = open("![[map.png|300]] ![[map.png|300x200]] ![[map.png|A map]]\n\nend");
    expect(attrs(v, "alt", "width", "height")).toEqual([
      ["map.png", "300", null],
      ["map.png", "300", "200"],
      ["A map", null, null],
    ]);
  });

  it("draw audio and video with their controls", () => {
    const v = open("![[song.mp3]]\n\n![[clip.mp4]]\n\nend");
    expect(media(v).map((el) => [el.tagName, el.getAttribute("src"), el.hasAttribute("controls")])).toEqual([
      ["AUDIO", "blob:audio/song.mp3", true],
      ["VIDEO", "blob:clip.mp4", true],
    ]);
  });

  it("leave notes, other files and unresolved embeds to their links", () => {
    expect(media(open("![[Plan]] ![[doc.pdf]] ![[Gone]] ![[Asking]]\n\nend"))).toEqual([]);
  });

  it("show their source on the selection's lines", () => {
    const v = open("See ![[map.png]] here\n\nend", "here");
    expect(media(v)).toEqual([]);
    expect(v.contentDOM.querySelector(".cm-line")?.textContent).toBe("See ![[map.png]] here");
  });

  it("show an image's source when it is clicked", () => {
    const doc = "See ![[map.png]] here\n\nend";
    const v = open(doc);
    media(v)[0]?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 }));
    expect(v.state.selection.main.head).toBe(doc.indexOf("[[") + 1);
    expect(media(v)).toEqual([]);
  });
});

describe("markdown images", () => {
  const doc =
    "![a map](map.png) ![sized|250](media/map.png) ![spaced](my%20map.png) ![web](https://e.test/x.png)\n\nend";

  it("draw vault images, sized by digits after the text's last bar", () => {
    expect(attrs(open(doc), "src", "alt", "width")).toEqual([
      ["blob:media/map.png", "a map", null],
      ["blob:media/map.png", "sized", "250"],
      ["blob:media/my map.png", "spaced", null],
    ]);
  });

  it("draw web images only while remote images load", () => {
    expect(attrs(open(doc, doc.length, true), "src")).toEqual([
      ["blob:media/map.png"],
      ["blob:media/map.png"],
      ["blob:media/my map.png"],
      ["https://e.test/x.png"],
    ]);
  });

  it("leave other files to their links", () => {
    expect(media(open("![x](doc.pdf) ![y](Gone)\n\nend"))).toEqual([]);
  });
});
