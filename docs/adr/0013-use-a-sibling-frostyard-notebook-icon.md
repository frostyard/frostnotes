# 0013 — Use a sibling Frostyard notebook icon

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

Frostnotes needs an application icon that looks like Frostmail's sibling
while remaining distinguishable in a launcher. The initial icon is a plain
white page on a blue square. Frostmail already has a faceted envelope with
a snowflake-marked letter on a chamfered midnight-blue tile.

## Decision

Use an ice-blue notebook with a faceted spine, snowflake cover, ruled lines
and a ribbon bookmark. Reuse Frostmail's outer tile geometry, border,
snowflake construction and cold gradients. Colors come from core's
`frostyard-design/tokens/colors.css`; the SVG records their names and values.
This extends the sibling identity in ADR-0011 to the application icon; it
does not change the working UI's tokens.

Keep `app/src-tauri/icons/icon.svg` as the editable master. Copy Frostmail's
`scripts/render-icons.py` workflow to render 32, 128 and 512 pixel RGBA PNGs
at the paths consumed by the Tauri configuration. Check the two applications
side by side at small sizes on light and dark surfaces.

## Consequences

The applications share a recognizable visual identity but have different
central silhouettes. Builds use checked-in PNGs without extra dependencies.
Regeneration requires Python, PyGObject, GdkPixbuf and its librsvg SVG loader.
The copied artwork and rendering script must be maintained locally.

## Alternatives considered

- **Reuse Frostmail's envelope:** indistinguishable in the launcher.
- **Only recolor the initial page:** misses the sibling's facets and frame.
- **Painted raster artwork:** harder to edit and less predictable at small sizes.

## References

- Shapes: [overview — application icon](../design/overview.md#application-icon)
- Builds on: [ADR-0011](0011-look-like-frostmails-sibling-with-obsidians-layout.md)
- Built in: [M0 app shell](../plans/0001-m0-foundations.md)
