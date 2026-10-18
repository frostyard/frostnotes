// Obsidian's %%comments%% (docs/specs/markdown.md#comments): ranges of the
// text, found as notesd finds them, not syntax nodes, since one may cross
// blocks. Cached ranges also drive the visible comment decorations.
import { syntaxTree } from "@codemirror/language";
import { type EditorState, type Extension, RangeSetBuilder, type Text } from "@codemirror/state";
import { Decoration, type DecorationSet, type EditorView, ViewPlugin } from "@codemirror/view";
import type { SyntaxNode, Tree } from "@lezer/common";

import { frontmatterEnd } from "./extract";

/** Span is a range of the document, from inclusive, to exclusive. */
export interface Span {
  from: number;
  to: number;
}

/** findComments lists the body's comments, each with its %% marks, in order. */
export function findComments(state: EditorState): Span[] {
  const tree = syntaxTree(state);
  let documents = commentsByTree.get(tree);
  const cached = documents?.get(state.doc);
  if (cached) return cached;
  const text = state.doc.toString();
  const comments: Span[] = [];
  for (let pos = frontmatterEnd(text); pos < text.length; ) {
    const from = text.indexOf("%%", pos);
    if (from < 0) break;
    let literalEnd = from;
    for (let node: SyntaxNode | null = tree.resolveInner(from, 1); node; node = node.parent) {
      if (LITERAL_NODES.has(node.name)) literalEnd = Math.max(literalEnd, node.to);
    }
    if (literalEnd > from) {
      pos = literalEnd;
      continue;
    }
    const close = text.indexOf("%%", from + 2);
    const to = close < 0 ? text.length : close + 2;
    comments.push({ from, to });
    pos = to;
  }
  if (!documents) {
    documents = new WeakMap<Text, Span[]>();
    commentsByTree.set(tree, documents);
  }
  documents.set(state.doc, comments);
  return comments;
}

// A tree can be shared by different documents (notably the empty tree).
const commentsByTree = new WeakMap<Tree, WeakMap<Text, Span[]>>();
const LITERAL_NODES = new Set([
  "FencedCode",
  "CodeBlock",
  "InlineCode",
  "HTMLBlock",
  "CommentBlock",
  "ProcessingInstructionBlock",
  "HTMLTag",
  "Comment",
  "ProcessingInstruction",
  "InlineMath",
  "BlockMath",
]);
const commentMark = Decoration.mark({ class: "cm-fn-comment" });

function decorations(view: EditorView): DecorationSet {
  const builder = new RangeSetBuilder<Decoration>();
  const comments = findComments(view.state);
  const iterator = comments[Symbol.iterator]();
  let comment = iterator.next().value;
  for (const visible of view.visibleRanges) {
    while (comment && comment.to <= visible.from) comment = iterator.next().value;
    while (comment && comment.from < visible.to) {
      const from = Math.max(visible.from, comment.from);
      const to = Math.min(visible.to, comment.to);
      if (from < to) builder.add(from, to, commentMark);
      if (comment.to > visible.to) break;
      comment = iterator.next().value;
    }
  }
  return builder.finish();
}

/** commentStyle marks the comments in view with the cm-fn-comment class. */
export const commentStyle: Extension = ViewPlugin.define(
  (view) => ({
    decorations: decorations(view),
    update(update) {
      if (update.docChanged || update.viewportChanged || syntaxTree(update.startState) !== syntaxTree(update.state)) {
        this.decorations = decorations(update.view);
      }
    },
  }),
  { decorations: (plugin) => plugin.decorations },
);
