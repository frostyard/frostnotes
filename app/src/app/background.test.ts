// The Background section of the settings (docs/specs/ui.md#settings-batch-2):
// the Flatpak's own switch for background activity, and what the desktop's
// Background portal last allowed (ADR-0033).
import { describe, expect, it } from "vitest";

import type { Background, PortalAnswer } from "../data/background";
import { DEFAULT_PREFS } from "../data/prefs";
import { backgroundSection, portalText, settingsSections } from "./Settings";

/** answer is the portal's answer, as the helper wrote it, with fields changed. */
const answer = (fields: Partial<PortalAnswer>): PortalAnswer => ({
  at: "2026-10-28T09:00:00Z",
  autostartRequested: true,
  response: 0,
  background: true,
  autostart: true,
  error: null,
  request: null,
  ...fields,
});

const background = (keepRunning: boolean, portal: PortalAnswer | null): Background => ({
  keepRunning,
  portal,
  request: null,
});

describe("the Background section", () => {
  it("has the switch, on with today's behaviour, and the desktop's last word", () => {
    const rows = backgroundSection(background(true, null)).rows;
    expect(rows[0]?.id).toBe("bg:keep");
    expect(rows[0]?.name).toBe("Keep syncing when no window is open");
    expect(rows[0]?.description).toContain("Off: notesd stops when the last window closes");
    expect(rows[0]?.control).toEqual({ kind: "toggle", value: true });
    expect(rows[1]?.id).toBe("bg:desktop");
    expect(rows[1]?.control).toEqual({ kind: "info", text: "Not asked yet." });
  });

  it("shows the switch off", () => {
    expect(backgroundSection(background(false, null)).rows[0]?.control).toEqual({ kind: "toggle", value: false });
  });

  it("says what the desktop allows, and what a refusal does", () => {
    expect(portalText(answer({}))).toBe("Allowed; notesd starts at login.");
    expect(portalText(answer({ autostart: false }))).toBe("Allowed; notesd does not start at login.");
    const refused = /^Refused\. .*GNOME Settings, Apps, Frostnotes, Run in Background/;
    expect(portalText(answer({ background: false }))).toMatch(refused);
    expect(portalText(answer({ response: 1 }))).toMatch(refused);
    expect(portalText(null)).toBe("Not asked yet.");
  });

  it("says the desktop did not answer when it did not", () => {
    const silent = { response: null, background: null, autostart: null };
    expect(portalText(answer(silent))).toBe("Not known: the desktop did not answer.");
    expect(portalText(answer({ ...silent, response: 2 }))).toBe("Not known: the desktop did not answer.");
    // Granted, but without the permission in the answer: no usable answer either.
    expect(portalText(answer({ background: null }))).toBe("Not known: the desktop did not answer.");
    const failed = { response: null, background: null, autostart: null, error: "no session bus" };
    expect(portalText(answer(failed))).toBe("Not known: the desktop did not answer (no session bus).");
  });

  it("is no section where the shell has no such setting", () => {
    expect(settingsSections(DEFAULT_PREFS, null, null).map((s) => s.id)).not.toContain("background");
  });

  it("is the last section where the shell has the setting", () => {
    const sections = settingsSections(DEFAULT_PREFS, null, null, null, background(true, null));
    expect(sections.at(-1)?.id).toBe("background");
    expect(sections.at(-1)?.rows).toHaveLength(2);
  });

  it("says that it asks the desktop while it waits for a new answer", () => {
    const control = backgroundSection(background(true, null), true).rows[1]?.control;
    expect(control).toEqual({ kind: "info", text: "Asking the desktop…" });
    const sections = settingsSections(DEFAULT_PREFS, null, null, null, background(true, null), true);
    expect(sections.at(-1)?.rows[1]?.control).toEqual({ kind: "info", text: "Asking the desktop…" });
  });
});
