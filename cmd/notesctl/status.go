package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/frostyard/clix"
	"github.com/spf13/cobra"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/flatpakapp"
)

// statusDial bounds the dial and rpc.hello of status: a notesd that
// accepts and never answers counts as not running.
const statusDial = 5 * time.Second

// errNotRunning is status's error when no notesd answers.
var errNotRunning = errors.New("notesd is not running")

// statusReport is what status found, and its --json document.
type statusReport struct {
	Socket  string  `json:"socket"`
	Running bool    `json:"running"`
	Error   *string `json:"error,omitzero"`
	Server  string  `json:"server,omitzero"`
	// Vaults, their sync and the hubs this device paired with, while
	// notesd answers.
	Vaults []api.Vault      `json:"vaults,omitzero"`
	Sync   []api.SyncStatus `json:"sync,omitzero"`
	Hubs   []api.SyncHub    `json:"hubs,omitzero"`
	// In the Flatpak: the background setting, and the end of notesd.log
	// when notesd does not answer.
	Background *flatpakapp.Background `json:"background,omitzero"`
	Log        *flatpakapp.NotesdLog  `json:"log,omitzero"`
}

// newStatusCmd builds status: what a window showed of notesd, as text.
func newStatusCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether notesd runs, its vaults and their sync, or why it does not run",
		Long: `Show whether notesd answers, and then its vaults, each vault's sync, the hubs
this device paired with and, in the Flatpak, the background setting. When
notesd does not answer, show why: in the Flatpak, the last lines of
notesd.log in the app's state directory, and the fix when the Flatpak portal
did not start it. It exits non-zero when notesd does not answer.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report := gatherStatus(cmd.Context(), opts, os.Getenv)
			if written, err := clix.OutputJSON(report); err != nil || written {
				if err == nil && !report.Running {
					err = errNotRunning
				}
				return err
			}
			if err := printStatus(cmd.OutOrStdout(), report); err != nil {
				return err
			}
			if !report.Running {
				return errNotRunning
			}
			return nil
		},
	}
}

// gatherStatus asks notesd, and reads the Flatpak's files.
func gatherStatus(ctx context.Context, opts *rootOptions, getenv flatpakapp.Getenv) *statusReport {
	r := &statusReport{}
	if b, ok := flatpakapp.GetBackground(getenv); ok {
		r.Background = &b
	}
	socket, err := opts.socketPath()
	if err != nil {
		msg := err.Error()
		r.Error = &msg
		return r
	}
	r.Socket = socket
	dctx, cancel := context.WithTimeout(ctx, statusDial)
	defer cancel()
	c, hello, err := opts.dial(dctx)
	if err != nil {
		msg := err.Error()
		r.Error = &msg
		if log, ok := flatpakapp.ReadNotesdLog(getenv); ok {
			r.Log = &log
		}
		return r
	}
	defer c.Close()
	r.Running, r.Server = true, hello.Server
	note := func(what string, err error) {
		if err != nil && r.Error == nil {
			msg := what + ": " + err.Error()
			r.Error = &msg
		}
	}
	var lerr error
	r.Vaults, lerr = c.Vault().List(ctx, &api.VaultListParams{})
	note("vault.list", lerr)
	// A notesd without sync answers these with unavailable or
	// methodNotFound: then there is nothing to show.
	var serr, herr error
	r.Sync, serr = c.Sync().Status(ctx, &api.SyncStatusParams{})
	r.Hubs, herr = c.Sync().Hubs(ctx, &api.SyncHubsParams{})
	if !noSync(serr) {
		note("sync.status", serr)
	}
	if !noSync(herr) {
		note("sync.hubs", herr)
	}
	return r
}

// noSync reports whether err says this notesd has no sync.
func noSync(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && (e.Code == api.CodeUnavailable || e.Code == api.CodeMethodNotFound)
}

func printStatus(w io.Writer, r *statusReport) error {
	p := &printer{w: w}
	if !r.Running {
		printNotRunning(p, r)
		return p.err
	}
	p.line("%s answers on %s", r.Server, r.Socket)
	if len(r.Vaults) == 0 {
		p.line("no vaults: register a folder with: notesctl vault add PATH")
	}
	for _, v := range r.Vaults {
		p.line("vault %d %q at %s: %s", v.ID, v.Name, v.Path, syncLine(v.ID, r.Sync))
	}
	if len(r.Hubs) == 0 {
		p.line("no hub: pair this device and sync a vault with: notesctl hub join URL NAME")
	}
	for _, h := range r.Hubs {
		p.line("paired with hub %s at %s as device %d, since %s", h.Hub, h.URL, h.Device, h.PairedAt.UTC().Format("2006-01-02"))
	}
	if r.Background != nil {
		p.line("%s", backgroundLine(r.Background))
	}
	if r.Error != nil {
		p.line("error: %s", *r.Error)
	}
	return p.err
}

// syncLine is a vault's sync, as status shows it.
func syncLine(vault int64, statuses []api.SyncStatus) string {
	for _, s := range statuses {
		if s.VaultID != vault {
			continue
		}
		text := fmt.Sprintf("syncs with %q at %s: %s, %d pending, %d conflicted", s.HubVault, s.URL, s.State, s.Pending, s.Conflicts)
		if s.Error != nil {
			text += " (" + *s.Error + ")"
		}
		return text
	}
	return "not synced"
}

func printNotRunning(p *printer, r *statusReport) {
	why := "notesd does not answer"
	if r.Socket != "" {
		why += " on " + r.Socket
	}
	if r.Error != nil {
		why += ": " + *r.Error
	}
	p.line("%s", why)
	if r.Log != nil {
		if len(r.Log.Lines) == 0 {
			p.line("notesd.log is empty or missing: %s", r.Log.Path)
		} else {
			p.line("the end of notesd.log (%s):", r.Log.Path)
			for _, l := range r.Log.Lines {
				p.line("  %s", l)
			}
		}
		if r.Log.PortalRefused {
			p.line("The Flatpak portal did not start notesd. To fix it:")
			p.line("  1. Run flatpak override --user --show org.frostyard.Frostnotes: it must not list a blocked org.freedesktop.portal.Flatpak.")
			p.line("  2. Remove the overrides with flatpak override --user --reset org.frostyard.Frostnotes, if it does.")
			p.line("  3. Check the Flatpak portal with systemctl --user status flatpak-portal.service.")
		}
	}
	if r.Background != nil {
		p.line("Start notesd with: flatpak run org.frostyard.Frostnotes")
		p.line("%s", backgroundLine(r.Background))
	}
}

// printer writes lines and keeps the first error.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) line(format string, args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format+"\n", args...)
	}
}
