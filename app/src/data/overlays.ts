// What the window shows over its layout (docs/specs/ui.md, Menus,
// Prompts, Notices): the one open menu, the open prompt, and the notices.
// Commands and containers open them through this store; app/Overlays.tsx
// draws them. The commands run from the palette are kept on this device
// (frostnotes.recentCommands), newest first.
import { create } from "zustand";

import type { MenuItem } from "../features/menu/Menu";
import type { NoticeItem } from "../features/notice/Notices";
import { PALETTE_RECENT } from "../lib/palette";
import { type KeyValue, localStore } from "./storage";

/** OpenMenu is a menu open at (x, y) and what choosing an item does. */
export interface OpenMenu {
  label: string;
  items: MenuItem[];
  x: number;
  y: number;
  onChoose: (id: string) => void;
}

/** PromptKind is a prompt the window can show. */
export type PromptKind = "switcher" | "palette";

export interface OverlayState {
  menu: OpenMenu | null;
  prompt: PromptKind | null;
  notices: NoticeItem[];
  /** The IDs of the commands run from the palette, newest first. */
  recentCommands: string[];
  /** The last notice's number, for the next one's ID. */
  noticeSeq: number;
}

const RECENT_KEY = "frostnotes.recentCommands";

function loadRecent(store: KeyValue): string[] {
  try {
    const v: unknown = JSON.parse(store.get(RECENT_KEY) ?? "[]");
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string").slice(0, PALETTE_RECENT) : [];
  } catch {
    return [];
  }
}

const storage = localStore();

export const useOverlays = create<OverlayState>(() => ({
  menu: null,
  prompt: null,
  notices: [],
  recentCommands: loadRecent(storage),
  noticeSeq: 0,
}));

/** openMenu shows a menu, closing any other. */
export function openMenu(menu: OpenMenu): void {
  useOverlays.setState({ menu });
}

export function closeMenu(): void {
  useOverlays.setState({ menu: null });
}

/** openPrompt shows a prompt, closing any menu. */
export function openPrompt(prompt: PromptKind): void {
  useOverlays.setState({ prompt, menu: null });
}

export function closePrompt(): void {
  useOverlays.setState({ prompt: null });
}

/** notify shows a notice; it returns its ID. */
export function notify(text: string, action?: NoticeItem["action"]): string {
  const seq = useOverlays.getState().noticeSeq + 1;
  const id = `n${seq}`;
  useOverlays.setState((s) => ({ noticeSeq: seq, notices: [...s.notices, { id, text, action }] }));
  return id;
}

export function dismissNotice(id: string): void {
  useOverlays.setState((s) => ({ notices: s.notices.filter((n) => n.id !== id) }));
}

/** ranCommand records a command run from the palette. */
export function ranCommand(id: string): void {
  const recent = [id, ...useOverlays.getState().recentCommands.filter((r) => r !== id)].slice(0, PALETTE_RECENT);
  useOverlays.setState({ recentCommands: recent });
  storage.set(RECENT_KEY, JSON.stringify(recent));
}
