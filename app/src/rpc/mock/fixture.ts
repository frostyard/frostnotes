// The made-up vault VITE_MOCK=1 shows: notes, folders, links, tags, a
// daily note and an image, so every pane has something to show. Nothing
// here comes from a real vault.

// A 4 × 4 PNG in the accent's blue.
const PIXEL = Uint8Array.from(
  atob("iVBORw0KGgoAAAANSUhEUgAAAAQAAAAECAIAAAAmkwkpAAAAEElEQVR4nGOQz78BRwzEcQA6ghZhQn5iTgAAAABJRU5ErkJggg=="),
  (c) => c.charCodeAt(0),
);

/** mockFiles are the made-up vaults' files, by vault ID. */
export function mockFiles(): Record<number, Record<string, string | Uint8Array>> {
  return {
    1: {
      "Welcome.md": [
        "# Welcome",
        "",
        "This vault is made up, so the app has something to show without notesd.",
        "",
        "- Plans live in [[Projects/Orchard plan|the orchard plan]].",
        "- Today's note is [[2026-10-17]].",
        "- Tags: #demo #garden/fruit",
        "",
        "![[Attachments/pixel.png]]",
        "",
      ].join("\n"),
      "Projects/Orchard plan.md": [
        "---",
        "status: active",
        "tags: [garden, planning]",
        "aliases: [Orchard]",
        "---",
        "# Orchard plan",
        "",
        "## Goals",
        "",
        "- [x] Order quince saplings",
        "- [ ] Prune the old pear",
        "",
        "## Later",
        "",
        "See [[Projects/Seed list]] and [[Welcome]].",
        "",
      ].join("\n"),
      "Projects/Seed list.md": "# Seed list\n\nBeans, peas, chard. #garden\n",
      "Projects/Archive/": "",
      "Daily/2026-10-17.md": "# 2026-10-17\n\nWalked the [[Projects/Orchard plan|orchard]]. ==Remember== the pears.\n",
      "Attachments/pixel.png": PIXEL,
    },
    2: {
      "Meetings/Standup.md": "# Standup\n\nNotes from the standup. #work\n",
    },
  };
}
