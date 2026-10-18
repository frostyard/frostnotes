// T-0067's given tests: words and characters as notesd counts words.
import { ensureSyntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { describe, expect, it } from "vitest";

import { readFixtures } from "../testing/fixtures";
import { markdownSupport } from "./syntax/index";
import { countWords } from "./words";

function parsed(doc: string): EditorState {
  const state = EditorState.create({ doc, extensions: markdownSupport() });
  ensureSyntaxTree(state, state.doc.length, 10_000);
  return state;
}

const words = (doc: string) => countWords(parsed(doc)).words;
const characters = (doc: string) => countWords(parsed(doc)).characters;

describe("words", () => {
  for (const f of readFixtures()) {
    it(`count as Parse counts them: ${f.id} — ${f.title}`, () => {
      expect(words(f.note)).toBe(f.want.words ?? 0);
    });
  }

  it("are split by Go's white space", () => {
    expect(words("one two\tthree\nfour")).toBe(4);
    expect(words("a\u0085b a\u00a0b a\u3000b a\u2028b")).toBe(8);
    expect(words("a\ufeffb a\u200bb")).toBe(2);
  });

  it("need a letter or a digit", () => {
    expect(words("- * -- 42 (x) !! … ½")).toBe(2);
    expect(words("日本語 é ٣")).toBe(3);
  });

  it("count markup as written", () => {
    expect(words("[[Apple Butter]] **bold** `code`")).toBe(4);
  });

  it("leave out comments, which split words", () => {
    expect(words("a%%x y%%b")).toBe(2);
    expect(words("a %%x y z%% b %%open")).toBe(2);
  });

  it("leave out the frontmatter", () => {
    expect(words("---\ntitle: x y\n---\nbody text")).toBe(2);
  });
});

describe("characters", () => {
  it("count every character but line breaks", () => {
    expect(characters("ab c\nd")).toBe(5);
    expect(characters("a\r\nb")).toBe(2);
    expect(characters("𝒜b é")).toBe(4);
  });

  it("leave out comments and the frontmatter", () => {
    expect(characters("a %%bc%% d")).toBe(4);
    expect(characters("---\nk: v\n---\nxy")).toBe(2);
  });
});

describe("a range", () => {
  it("counts only its part of the body", () => {
    const state = parsed("---\nk: v\n---\nalpha beta gamma %%x%% delta");
    const at = (s: string) => state.doc.toString().indexOf(s);
    expect(countWords(state, at("beta"), at("beta") + 4)).toEqual({ words: 1, characters: 4 });
    expect(countWords(state, at("gam") + 1, at("delta") + 2)).toEqual({ words: 2, characters: 8 });
    expect(countWords(state, 0, at("alpha") + 2)).toEqual({ words: 1, characters: 2 });
    expect(countWords(state, at("x"), at("x") + 1)).toEqual({ words: 0, characters: 0 });
  });
});
