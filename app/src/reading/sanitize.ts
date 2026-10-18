// The app's sanitizer (ADR-0007; docs/specs/render.md#sanitizer): notesd's
// allowlist again, with DOMPurify, so the reading view's HTML is safe even
// if notesd's were not. Elements and attributes off the list go; what runs
// script, embeds, submits or styles goes with its content; a URL keeps
// only the schemes the list names, and a vault file is named by data-blob,
// never by URL.
import DOMPurify from "dompurify";

const BLOCKS = ["p", "br", "hr", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre", "code", "div", "span"];
const INLINE = [
  "em",
  "i",
  "strong",
  "b",
  "del",
  "s",
  "u",
  "ins",
  "mark",
  "sup",
  "sub",
  "small",
  "kbd",
  "abbr",
  "q",
  "cite",
];
const LISTS = ["ul", "ol", "li", "dl", "dt", "dd"];
const TABLES = ["table", "thead", "tbody", "tfoot", "tr", "th", "td"];
const OTHER = ["details", "summary", "figure", "figcaption", "a", "img", "audio", "video", "input"];

/** ELEMENTS are the elements that stay. */
export const ELEMENTS = [...BLOCKS, ...INLINE, ...LISTS, ...TABLES, ...OTHER];

/** SHARED are the attributes any kept element may carry. */
export const SHARED = [
  "class",
  "title",
  "id",
  "role",
  "lang",
  "dir",
  "data-href",
  "data-path",
  "data-blob",
  "data-tag",
  "data-tex",
  "data-callout",
  "data-fold",
  "data-line",
  "data-block",
];

/**
 * CLASSES are the class names notesd's renderer draws (render.md's
 * sanitizer): a note's other classes go, so its HTML cannot use the app's
 * own styles. A code block's language-… stays too.
 */
const CLASSES = new Set([
  "internal-link",
  "external-link",
  "is-unresolved",
  "embed",
  "file-embed",
  "tag",
  "math",
  "callout",
  "callout-title",
  "callout-content",
  "footnotes",
  "footnote-ref",
  "footnote-backref",
]);

/** ROLES are the roles notesd's renderer draws: footnotes'. */
const ROLES = ["doc-noteref", "doc-endnotes", "doc-backlink"];

/** classes keeps the class names of value the renderer draws. */
export function classes(value: string): string {
  return value
    .split(/\s+/)
    .filter((c) => CLASSES.has(c) || (c.startsWith("language-") && c.length > "language-".length))
    .join(" ");
}

/** BY_ELEMENT are the attributes particular elements may carry besides the shared ones. */
const BY_ELEMENT: Record<string, readonly string[]> = {
  ol: ["start"],
  li: ["value"],
  th: ["align", "colspan", "rowspan"],
  td: ["align", "colspan", "rowspan"],
  a: ["href", "rel"],
  img: ["src", "alt", "width", "height"],
  audio: ["controls"],
  video: ["controls"],
  input: ["type", "checked", "disabled"],
};

/** DROPPED_WHOLE go with their content (render.md's list). */
const DROPPED_WHOLE = [
  "script",
  "style",
  "template",
  "iframe",
  "frame",
  "frameset",
  "object",
  "embed",
  "applet",
  "param",
  "portal",
  "form",
  "button",
  "select",
  "option",
  "optgroup",
  "datalist",
  "textarea",
  "link",
  "meta",
  "base",
  "head",
  "title",
  "svg",
  "math",
  "noscript",
  "noembed",
  "noframes",
  "xmp",
  "plaintext",
];

/**
 * safeURL reports whether a URL may stay: http and https, and for a link
 * mailto and a fragment of this page. White space and control characters
 * are taken out first, as a browser reads a scheme.
 */
export function safeURL(value: string, link: boolean): boolean {
  // biome-ignore lint/suspicious/noControlCharactersInRegex: control characters are what this strips
  const v = value.replace(/[\u0000- \u007f]/g, "");
  if (link && v.startsWith("#")) return true;
  const colon = v.indexOf(":");
  if (colon < 0 || /[/?#]/.test(v.slice(0, colon))) return false;
  const scheme = v.slice(0, colon).toLowerCase();
  return scheme === "http" || scheme === "https" || (link && scheme === "mailto");
}

/** createSanitizer makes the reading view's sanitizer over a window. */
export function createSanitizer(win: Window & typeof globalThis = window): (html: string) => string {
  const purify = DOMPurify(win);
  purify.addHook("uponSanitizeElement", (node, data) => {
    // An input stays only as a task's checkbox.
    if (data.tagName === "input" && (node as Element).getAttribute?.("type")?.toLowerCase() !== "checkbox") {
      node.parentNode?.removeChild(node);
    }
  });
  purify.addHook("uponSanitizeAttribute", (node, data) => {
    const tag = node.nodeName.toLowerCase();
    const name = data.attrName;
    if (name === "class") {
      data.attrValue = classes(data.attrValue);
      data.keepAttr = data.attrValue !== "";
      return;
    }
    // A footnote's id and role only: no other id can name a window property the app reads.
    if (name === "id") {
      data.keepAttr = data.attrValue.startsWith("fn:") || data.attrValue.startsWith("fnref");
      return;
    }
    if (name === "role") {
      data.keepAttr = ROLES.includes(data.attrValue);
      return;
    }
    if (SHARED.includes(name)) return;
    if (!BY_ELEMENT[tag]?.includes(name)) {
      data.keepAttr = false;
      return;
    }
    if (name === "href" || name === "src") {
      data.keepAttr = safeURL(data.attrValue, name === "href");
      data.forceKeepAttr = data.keepAttr || undefined;
    }
  });
  const config = {
    ALLOWED_TAGS: ELEMENTS,
    ALLOWED_ATTR: [...SHARED, ...new Set(Object.values(BY_ELEMENT).flat())] as string[],
    FORBID_CONTENTS: DROPPED_WHOLE,
    ALLOW_DATA_ATTR: false,
    ALLOW_ARIA_ATTR: false,
    ALLOW_UNKNOWN_PROTOCOLS: false,
    KEEP_CONTENT: true,
    RETURN_TRUSTED_TYPE: false as const,
  };
  const clean = (html: string) => purify.sanitize(html, config) as string;
  // An engine whose DOM DOMPurify misreads (happy-dom keeps <script>) gets
  // no HTML at all: the note's HTML shows as text, escaped.
  if (clean('<p>a<script>x</script><b onclick="y">b</b></p>') !== "<p>a<b>b</b></p>") return escapeHTML;
  return clean;
}

/** escapeHTML is html as text. */
export function escapeHTML(html: string): string {
  return html.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);
}
