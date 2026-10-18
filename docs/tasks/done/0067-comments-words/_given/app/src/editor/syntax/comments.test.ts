// T-0067's given tests: comments, found as notesd finds them, and marked in view.
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { afterEach, describe, expect, it } from "vitest";

import { commentStyle, findComments } from "./comments";
import { markdownSupport } from "./index";

function parsed(doc: string): EditorState {
  const state = EditorState.create({ doc, extensions: markdownSupport() });
  ensureSyntaxTree(state, state.doc.length, 10_000);
  return state;
}

/** comments lists the text of each comment of doc. */
function comments(doc: string): string[] {
  return findComments(parsed(doc)).map((c) => doc.slice(c.from, c.to));
}

describe("comments", () => {
  it("are %%…%% with their marks, in order", () => {
    expect(findComments(parsed("a %%hidden%% b %%two%%"))).toEqual([
      { from: 2, to: 12 },
      { from: 15, to: 22 },
    ]);
    expect(comments("%%%% and %%x%%")).toEqual(["%%%%", "%%x%%"]);
  });

  it("cross lines and blocks", () => {
    expect(comments("a %%x\n\n- y\n> z %% b")).toEqual(["%%x\n\n- y\n> z %%"]);
    expect(comments("%%\nA block comment with [[Hidden]]\n%%\nafter")).toEqual([
      "%%\nA block comment with [[Hidden]]\n%%",
    ]);
  });

  it("run to the end when left open", () => {
    expect(comments("a %%rest\n\nmore")).toEqual(["%%rest\n\nmore"]);
    expect(comments("a %%%")).toEqual(["%%%"]);
  });

  it("are not opened in code or HTML", () => {
    expect(comments("`%%` x %%c%%")).toEqual(["%%c%%"]);
    expect(comments("```\n%%\n```\n\n%%f%%")).toEqual(["%%f%%"]);
    expect(comments("    %% indented\n\npara %%i%%")).toEqual(["%%i%%"]);
    expect(comments('<span title="%%">x</span> %%h%%')).toEqual(["%%h%%"]);
    expect(comments("<div>\n%%\n</div>\n\n%%z%%")).toEqual(["%%z%%"]);
    expect(comments("<!-- %% -->\n\n%%w%%")).toEqual(["%%w%%"]);
  });

  it("are closed by the next %% wherever it is", () => {
    expect(comments("%%a `x%%` b")).toEqual(["%%a `x%%"]);
  });

  it("are the body's only", () => {
    expect(comments("---\nkey: '%%v%%'\n---\n%%c%%")).toEqual(["%%c%%"]);
  });
});

describe("the comments' style", () => {
  let view: EditorView | null = null;
  afterEach(() => {
    view?.destroy();
    view = null;
  });

  it("marks the comments in view", () => {
    view = new EditorView({
      parent: document.body,
      state: EditorState.create({ doc: "a %%b%% c\n`%%` d %%e\nf%%", extensions: markdownSupport() }),
    });
    const marked = [...view.contentDOM.querySelectorAll(".cm-fn-comment")].map((el) => el.textContent);
    expect(marked.join("|")).toBe("%%b%%|%%e|f%%");
  });

  it("follows edits", () => {
    view = new EditorView({
      parent: document.body,
      state: EditorState.create({ doc: "a b c", extensions: [markdownSupport(), commentStyle] }),
    });
    expect(view.contentDOM.querySelectorAll(".cm-fn-comment")).toHaveLength(0);
    view.dispatch({
      changes: [
        { from: 2, insert: "%%" },
        { from: 3, insert: "%%" },
      ],
    });
    expect([...view.contentDOM.querySelectorAll(".cm-fn-comment")].map((el) => el.textContent)).toEqual(["%%b%%"]);
  });
});
