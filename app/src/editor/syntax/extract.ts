// What the editor's parse tree holds that notesd's parse also finds
// (docs/specs/markdown.md#parse-output): a note's links and tags, by line.
// The editor resolves the links it finds, and the fixture tests compare
// both parsers through this.
import { syntaxTree } from "@codemirror/language";
import type { EditorState } from "@codemirror/state";
import type { SyntaxNode } from "@lezer/common";

/** FoundLink is a link the tree holds, as Parse reports one. */
export interface FoundLink {
  kind: "wiki" | "markdown";
  target: string;
  subpath: string;
  alias: string;
  embed: boolean;
  line: number;
  from: number;
  to: number;
}

/** FoundTag is a tag the tree holds. */
export interface FoundTag {
  name: string;
  line: number;
}

const SCHEME = /^[a-zA-Z][a-zA-Z0-9+.-]*:/;

function decode(s: string): string {
  try {
    return decodeURIComponent(s);
  } catch {
    return s;
  }
}

/** frontmatterEnd is where a note's frontmatter ends: 0 when it has none. */
export function frontmatterEnd(text: string): number {
  const m = /^---\r?\n(?:[\s\S]*?\r?\n)?---(?:\r?\n|$)/.exec(text);
  return m ? m[0].length : 0;
}

/** findLinks lists the body's links, in order: wikilinks, and markdown links and images that name vault files. */
export function findLinks(state: EditorState, from = 0, to = state.doc.length): FoundLink[] {
  const doc = state.doc;
  const text = (n: SyntaxNode | null) => (n ? doc.sliceString(n.from, n.to) : "");
  const line = (pos: number) => doc.lineAt(pos).number;
  const refs = new Map<string, string>();
  const out: FoundLink[] = [];
  const pending: { node: SyntaxNode; label: string; embed: boolean; alias: string }[] = [];

  syntaxTree(state).iterate({
    from,
    to,
    enter: (ref) => {
      switch (ref.name) {
        case "WikiLink":
          wiki(ref.node, ref.node, false);
          return false;
        case "Embed": {
          const link = ref.node.getChild("WikiLink");
          if (link) wiki(link, ref.node, true);
          return false;
        }
        case "LinkReference": {
          const label = text(ref.node.getChild("LinkLabel")).slice(1, -1).toLowerCase();
          refs.set(label, unangle(text(ref.node.getChild("URL"))));
          return false;
        }
        case "Link":
        case "Image": {
          const n = ref.node;
          const embed = ref.name === "Image";
          const marks = n.getChildren("LinkMark");
          const textEnd = marks.find((m) => doc.sliceString(m.from, m.to) === "]")?.from ?? n.to;
          const alias = doc.sliceString(n.from + (embed ? 2 : 1), textEnd);
          const url = n.getChild("URL");
          if (url) push(n, unangle(text(url)), embed, alias);
          else {
            const label = n.getChild("LinkLabel");
            pending.push({ node: n, label: label ? text(label).slice(1, -1) : alias, embed, alias });
          }
          return false;
        }
        default:
          return;
      }
    },
  });
  for (const p of pending) {
    const dest = refs.get(p.label.toLowerCase());
    if (dest !== undefined) push(p.node, dest, p.embed, p.alias);
  }
  return out.sort((a, b) => a.from - b.from);

  // wiki adds the WikiLink n; at is n itself, or the Embed around it.
  function wiki(n: SyntaxNode, at: SyntaxNode, embed: boolean) {
    out.push({
      kind: "wiki",
      target: text(n.getChild("WikiLinkTarget")).trim(),
      subpath: text(n.getChild("WikiLinkSubpath")).trim(),
      alias: text(n.getChild("WikiLinkAlias")),
      embed,
      line: line(at.from),
      from: at.from,
      to: at.to,
    });
  }

  function push(n: SyntaxNode, dest: string, embed: boolean, alias: string) {
    if (SCHEME.test(dest) || dest.startsWith("//")) return;
    const hash = dest.indexOf("#");
    out.push({
      kind: "markdown",
      target: decode(hash < 0 ? dest : dest.slice(0, hash)),
      subpath: hash < 0 ? "" : decode(dest.slice(hash)),
      alias,
      embed,
      line: line(n.from),
      from: n.from,
      to: n.to,
    });
  }
}

function unangle(url: string): string {
  return url.startsWith("<") && url.endsWith(">") ? url.slice(1, -1) : url;
}

/** findTags lists the body's tags: the Hashtag nodes (tags.ts), except those in a link's text. */
export function findTags(state: EditorState): FoundTag[] {
  const out: FoundTag[] = [];
  syntaxTree(state).iterate({
    enter: (ref) => {
      if (ref.name === "Link" || ref.name === "Image") return false;
      if (ref.name !== "Hashtag") return;
      out.push({ name: state.doc.sliceString(ref.from + 1, ref.to), line: state.doc.lineAt(ref.from).number });
      return false;
    },
  });
  return out;
}

/** FoundBlock is a block ID the tree holds, as Parse reports one. */
export interface FoundBlock {
  /** Without its "^". */
  id: string;
  line: number;
  from: number;
  to: number;
}

// AFTER_LONE are the blocks a block ID may follow as a paragraph of its own.
const AFTER_LONE = new Set(["BulletList", "OrderedList", "Blockquote", "Table"]);

/**
 * findBlocks lists the body's block IDs: a BlockId node (blockIds.ts) that
 * ends a paragraph, or that is a paragraph of its own right after a list,
 * a quote or a table (docs/specs/markdown.md#block-ids).
 */
export function findBlocks(state: EditorState): FoundBlock[] {
  const doc = state.doc;
  const out: FoundBlock[] = [];
  syntaxTree(state).iterate({
    enter: (ref) => {
      if (ref.name !== "BlockId") return;
      const para = ref.node.parent;
      if (para?.name !== "Paragraph") return false;
      const lone = doc.sliceString(para.from, para.to).trim() === doc.sliceString(ref.from, ref.to);
      if (lone && !AFTER_LONE.has(para.prevSibling?.name ?? "")) return false;
      out.push({
        id: doc.sliceString(ref.from + 1, ref.to),
        line: doc.lineAt(ref.from).number,
        from: ref.from,
        to: ref.to,
      });
      return false;
    },
  });
  return out;
}
