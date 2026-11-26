package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/frostyard/clix"
	"github.com/spf13/cobra"

	"github.com/frostyard/frostnotes/internal/flatpakapp"
)

// newBackgroundCmd builds background: the Flatpak's setting for background
// activity (docs/adr/0033-a-setting-keeps-notesd-running-without-a-window.md),
// which a window's Settings held before the Flatpak had no window.
func newBackgroundCmd(_ *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:       "background [on|off]",
		Short:     "Show or change whether the desktop starts notesd at login (Flatpak)",
		ValidArgs: []string{"on", "off"},
		Long: `In the Flatpak org.frostyard.Frostnotes, show whether notesd starts at login
and what the desktop's Background portal last answered. On is the default.

"background off" keeps the desktop from starting notesd at login: the
portal removes its autostart entry, and an entry that is left starts no
notesd. The notesd that runs keeps running until you log out or run
"notesctl shutdown". "background on" asks the portal to start notesd at
login again. Either waits for the desktop's answer: it may ask you first.

Outside the Flatpak notesd is a service its own manager runs, and this
command fails.`,
		Args: cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			getenv := os.Getenv
			out := cmd.OutOrStdout()
			if len(args) == 0 {
				b, ok := flatpakapp.GetBackground(getenv)
				if !ok {
					return errors.New("the background setting belongs to the Flatpak org.frostyard.Frostnotes; here notesd runs as its service manager starts it")
				}
				if written, err := clix.OutputJSON(b); err != nil || written {
					return err
				}
				_, err := fmt.Fprintln(out, backgroundLine(&b))
				return err
			}
			on := args[0] == "on"
			gen, err := flatpakapp.Change(getenv, on)
			if err != nil {
				return err
			}
			if !clix.JSONOutput {
				if _, err := fmt.Fprintln(out, "asking the desktop's Background portal…"); err != nil {
					return err
				}
			}
			if err := flatpakapp.Ask(cmd.Context(), getenv); err != nil {
				return err
			}
			b, _ := flatpakapp.GetBackground(getenv)
			b.Generation = gen
			if written, err := clix.OutputJSON(b); err != nil || written {
				return err
			}
			p := &printer{w: out}
			if b.Portal == nil || b.Portal.Generation == nil || *b.Portal.Generation < gen {
				// Another request held the helpers' lock, or the helper
				// wrote nothing: its answer comes later.
				p.line("%s; the desktop's answer is not in yet: notesctl background shows it later", settingText(b.KeepRunning))
			} else {
				p.line("%s", backgroundLine(&b))
			}
			if !on {
				p.line("the notesd that runs keeps running until you log out, or run: notesctl shutdown")
			}
			return p.err
		},
	}
}

// settingText is the setting alone.
func settingText(keepRunning bool) string {
	if keepRunning {
		return "background: on, notesd starts at login when the desktop allows it"
	}
	return "background: off, notesd does not start at login"
}

// backgroundLine is the setting and the desktop's last answer.
func backgroundLine(b *flatpakapp.Background) string {
	return settingText(b.KeepRunning) + "; " + portalText(b.Portal)
}

// portalText is the Background portal's last answer, as the helper wrote
// it. Without a window, GNOME Settings does not list the app, so a refusal
// names the permission store's command.
func portalText(p *flatpakapp.PortalAnswer) string {
	if p == nil {
		return "the desktop was not asked yet: run flatpak run org.frostyard.Frostnotes"
	}
	granted := p.Response != nil && *p.Response == 0
	if granted && p.Background != nil && *p.Background {
		if p.Autostart != nil && *p.Autostart {
			return "the desktop allows it to run in the background and starts it at login"
		}
		return "the desktop allows it to run in the background, and does not start it at login"
	}
	if (p.Response != nil && *p.Response == 1) || (granted && p.Background != nil && !*p.Background) {
		return "the desktop refused: it stops a notesd with no window, and does not start it at login. " +
			"Allow it with: flatpak permission-set background background org.frostyard.Frostnotes yes; " +
			"then run flatpak run org.frostyard.Frostnotes"
	}
	if p.Error != nil {
		return "the desktop did not answer (" + *p.Error + ")"
	}
	return "the desktop did not answer"
}
