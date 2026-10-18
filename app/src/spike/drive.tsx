// The page's half of M3's exit evidence 2 and 3
// (docs/plans/0005-m3-desktop-app.md, Phase 7). The built app runs the
// real app with FROSTNOTES_SPIKE=hotkeys or writers, and a driver here
// works it as a person would. What it cannot do from the page, pressing
// keys the system delivers and writing the note from outside, it asks of
// the harness (internal/appevidence) in lines the shell prints
// (spike_say); the harness answers through the window and the vault.
// Its report is the last line, after which the app exits.
import { EditorView } from "@codemirror/view";
import { invoke } from "@tauri-apps/api/core";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "../app/App";
import type { Connect } from "../data/session";

/** say prints one line for the harness: what the driver needs done. */
export async function say(line: Record<string, unknown>): Promise<void> {
  await invoke("spike_say", { line: JSON.stringify(line) });
}

/** report prints the driver's results; the app exits. */
export async function report(r: Record<string, unknown>): Promise<void> {
  await invoke("spike_report", { report: JSON.stringify(r) });
}

/** until resolves once ready() holds, checked every 10 ms, or rejects after ms. */
export function until(ready: () => boolean, ms: number, what: string): Promise<void> {
  const end = performance.now() + ms;
  return new Promise((resolve, reject) => {
    const check = () => {
      let ok = false;
      try {
        ok = ready();
      } catch {
        ok = false;
      }
      if (ok) resolve();
      else if (performance.now() > end) reject(new Error(`gave up waiting for ${what} after ${ms} ms`));
      else setTimeout(check, 10);
    };
    check();
  });
}

/** frames resolves after n animation frames: React has committed and the browser drawn. */
export async function frames(n: number): Promise<void> {
  for (let i = 0; i < n; i++) await new Promise((r) => requestAnimationFrame(r));
}

/** editors are the editor views on the page, in the order the page draws them. */
export function editors(): EditorView[] {
  return [...document.querySelectorAll<HTMLElement>(".cm-editor")]
    .map((el) => EditorView.findFromDOM(el))
    .filter((v): v is EditorView => v !== null && v.dom.isConnected);
}

/** showApp draws the app as main.tsx does. */
export function showApp(root: HTMLElement, connect: Connect): void {
  createRoot(root).render(
    <StrictMode>
      <App connect={connect} />
    </StrictMode>,
  );
}

/** CLOSE_WAIT is how long the app may take to exit once close has closed its window. */
const CLOSE_WAIT = 10_000;

/**
 * drive runs a driver and reports what it found, or why it stopped. With
 * close, the report is said instead, then close closes the window as a
 * person would: the app's exit ends the run, and an app that lives on
 * reports that its window did not close.
 */
export async function drive(
  name: string,
  run: () => Promise<Record<string, unknown>>,
  close?: () => void,
): Promise<void> {
  let found: Record<string, unknown>;
  try {
    found = { spike: name, ...(await run()) };
  } catch (err) {
    await report({ spike: name, ok: false, error: err instanceof Error ? err.message : String(err) });
    return;
  }
  if (!close) {
    await report(found);
    return;
  }
  await say({ say: "report", report: found });
  close();
  await new Promise((resolve) => setTimeout(resolve, CLOSE_WAIT));
  await report({ spike: name, ok: false, error: `the window did not close within ${CLOSE_WAIT} ms` });
}
