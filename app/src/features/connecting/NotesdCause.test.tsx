import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { NotesdCause } from "./NotesdCause";

const section = () => screen.getByRole("region", { name: "Why notesd is not running" });

describe("NotesdCause", () => {
  it("says the portal did not start notesd, how to check, and where the log is", () => {
    render(
      <NotesdCause
        log={{
          path: "/home/ana/.var/app/org.frostyard.Frostnotes/state/frostnotes/notesd.log",
          lastLine: "frostnotes: notesd did not start through the Flatpak portal (flatpak-spawn); see x",
          portalRefused: true,
        }}
      />,
    );
    const text = section().textContent ?? "";
    expect(text).toContain("notesd did not start, because the Flatpak portal did not start it.");
    expect(text).toContain("flatpak override --user --show org.frostyard.Frostnotes");
    expect(text).toContain("org.freedesktop.portal.Flatpak");
    expect(text).toContain("flatpak override --user --reset org.frostyard.Frostnotes");
    expect(text).toContain("xdg-desktop-portal");
    expect(text).toContain("The log: /home/ana/.var/app/org.frostyard.Frostnotes/state/frostnotes/notesd.log");
    // The launcher's line is the cause above, not repeated.
    expect(text).not.toContain("The last line");
  });

  it("shows another last line, and the log", () => {
    const lastLine = "notesd: another notesd uses /d";
    render(<NotesdCause log={{ path: "/s/notesd.log", lastLine, portalRefused: false }} />);
    expect(section().textContent).toBe(`The last line of notesd's log: ${lastLine}The log: /s/notesd.log`);
  });

  it("names only the log when it holds nothing", () => {
    render(<NotesdCause log={{ path: "/s/notesd.log", lastLine: null, portalRefused: false }} />);
    expect(section().textContent).toBe("The log: /s/notesd.log");
  });
});
