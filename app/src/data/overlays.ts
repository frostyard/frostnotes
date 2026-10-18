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

/** OpenMove is the folder chooser, open to move a file or a folder. */
export interface OpenMove {
  path: string;
  folder: boolean;
}

/** OpenConfirm is a question waiting for its answer. */
export interface OpenConfirm {
  title: string;
  text: string;
  confirmLabel: string;
  cancelLabel?: string;
  danger?: boolean;
  answer: (yes: boolean) => void;
}

export interface OverlayState {
  menu: OpenMenu | null;
  prompt: PromptKind | null;
  /** The folder chooser, open for a move. */
  move: OpenMove | null;
  /** The confirmation shown; one at a time. */
  confirm: OpenConfirm | null;
  /** The settings dialog, open at a section. */
  settings: string | null;
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
  move: null,
  confirm: null,
  settings: null,
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

/**
 * ask shows a confirmation and resolves to the answer: true when it is
 * confirmed. A question asked while another waits answers the other no.
 */
export function ask(q: Omit<OpenConfirm, "answer">): Promise<boolean> {
  return new Promise((resolve) => {
    useOverlays.getState().confirm?.answer(false);
    const answer = (yes: boolean) => {
      useOverlays.setState((s) => (s.confirm?.answer === answer ? { confirm: null } : {}));
      resolve(yes);
    };
    useOverlays.setState({ confirm: { ...q, answer }, menu: null });
  });
}

/** openMove opens the folder chooser to move path, a folder or a file. */
export function openMove(path: string, folder: boolean): void {
  useOverlays.setState({ move: { path, folder }, menu: null, prompt: null });
}

export function closeMove(): void {
  useOverlays.setState({ move: null });
}

/** openSettings opens the settings dialog at a section. */
export function openSettings(section = "editor"): void {
  useOverlays.setState({ settings: section, menu: null, prompt: null });
}

export function closeSettings(): void {
  useOverlays.setState({ settings: null });
}
