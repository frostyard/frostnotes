// Every live-preview provider (docs/design/app.md#live-preview), in the
// order they draw: the first ones (basic.ts), then batch 3's, a file each.
import { BASIC } from "./basic";
import { CODE } from "./code";
import type { Provider } from "./framework";
import { INLINE } from "./inline";
import { LISTS } from "./lists";
import { MATH } from "./math";
import { MEDIA } from "./media";
import { RENDERED } from "./rendered";

/** PROVIDERS are live preview's providers. */
export const PROVIDERS: readonly Provider[] = [...BASIC, ...INLINE, ...CODE, ...LISTS, ...MATH, ...RENDERED, ...MEDIA];
