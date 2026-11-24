// Package vaults shares the vault registration and device pairing rules of the desktop clients.
package vaults

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
)

func forbiddenName() *regexp.Regexp { return regexp.MustCompile(`[\\/:*?"<>|#^\[\]]`) }

// NameError says why name cannot name a new vault, "" when it can.
func NameError(name string) string {
	t := strings.TrimSpace(name)
	switch {
	case t == "":
		return "A vault needs a name."
	case strings.HasPrefix(t, "."):
		return "A vault's name cannot start with a dot."
	case forbiddenName().MatchString(t):
		return `A vault's name cannot hold any of these: \ / : * ? " < > | # ^ [ ]`
	}
	return ""
}

func Folder(location, name string) string {
	return strings.TrimRight(location, "/") + "/" + strings.TrimSpace(name)
}

func HubURL(url string) string { return strings.TrimRight(strings.TrimSpace(url), "/") }

// Join is one run of the flow, off the UI thread.
type Join struct {
	Client            *api.Client
	Context           context.Context
	URL, Name, Device string
	VaultID           int64
	PairOnly          bool
	Folder            string
	OnCode            func(string)
	OnJoining         func()
}

func (r *Join) stopped() error {
	if r.Context.Err() != nil {
		return context.Canceled
	}
	return nil
}

// awaitApproval shows the pairing's code and waits: notesd long-polls up to
// 30 seconds for each sync.paired.
func (r *Join) awaitApproval(p *api.Pairing) error {
	if err := r.stopped(); err != nil {
		return err
	}
	r.OnCode(p.Code)
	for {
		if err := r.stopped(); err != nil {
			return err
		}
		res, err := r.Client.Sync().Paired(r.Context, &api.SyncPairedParams{Pairing: p.Pairing})
		if err != nil {
			return err
		}
		switch res.State {
		case api.PairingStateApproved:
			return nil
		case api.PairingStateExpired:
			return errors.New("the pairing expired: the code was not approved within 10 minutes")
		}
	}
}

func (r *Join) pairNew() error {
	params := &api.SyncPairParams{URL: r.URL}
	if r.Device != "" {
		params.Name = &r.Device
	}
	p, err := r.Client.Sync().Pair(r.Context, params)
	if err != nil {
		return err
	}
	return r.awaitApproval(p)
}

func (r *Join) paired() (bool, error) {
	hubs, err := r.Client.Sync().Hubs(r.Context, &api.SyncHubsParams{})
	if err != nil {
		return false, err
	}
	for _, h := range hubs {
		if h.URL == r.URL {
			return true, nil
		}
	}
	return false, nil
}

func (r *Join) join(id int64, wasPaired bool) error {
	params := &api.SyncJoinParams{VaultID: id, URL: r.URL, Name: r.Name}
	_, err := r.Client.Sync().Join(r.Context, params)
	var e *api.Error
	if err == nil || !wasPaired || !errors.As(err, &e) || e.Code != api.CodeRevoked {
		return err
	}
	// The hub refuses a device it paired: pair again once, join again.
	if err := r.pairNew(); err != nil {
		return err
	}
	_, err = r.Client.Sync().Join(r.Context, params)
	return err
}

// addVault adds the folder to be made and waits for notesd's answer, even
// through a cancel: one that landed mid-call could leave a vault nothing knows
// of. The join after it sees the cancel, and removes the vault.
func (r *Join) addVault() (*api.Vault, error) {
	bg, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	create := true
	v, err := r.Client.Vault().Add(bg, &api.VaultAddParams{Path: r.Folder, Create: &create})
	if err != nil {
		var rpcErr *api.Error
		if !errors.As(err, &rpcErr) {
			return nil, &VaultLeft{fmt.Sprintf("adding the vault lost its answer: %v; the vault at %q may remain on the list. Check Manage vaults and remove it before retrying", err, r.Folder)}
		}
	}
	return v, err
}

// VaultLeft is a join's failure that may leave its vault on the list. The
// user hears of it even when the join was cancelled or replaced (#194).
type VaultLeft struct{ msg string }

func (e *VaultLeft) Error() string { return e.msg }

// cleanupFailed says a join failed and its vault stayed on the list. It
// does not wrap the join's error: a cancel would read as nothing to show.
func cleanupFailed(err error, name string, rerr error) error {
	why := err.Error()
	if errors.Is(err, context.Canceled) {
		why = "the join was cancelled"
	}
	return &VaultLeft{why + `; could not remove the new vault "` + name + `" from the list: ` + rerr.Error() + ". Remove it in Manage vaults."}
}

// Run returns the vault it made, nil when the vault existed.
func (r *Join) Run() (*api.Vault, error) {
	if r.PairOnly {
		return nil, r.pairNew()
	}
	wasPaired, err := r.paired()
	if err != nil {
		return nil, err
	}
	if !wasPaired {
		if err := r.pairNew(); err != nil {
			return nil, err
		}
	}
	r.OnJoining()
	if r.Folder == "" {
		return nil, r.join(r.VaultID, wasPaired)
	}
	v, err := r.addVault()
	if err != nil {
		return nil, err
	}
	if err := r.join(v.ID, wasPaired); err != nil {
		// Forget the vault it made and did not join, even when cancelled.
		bg, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if rerr := r.Client.Vault().Remove(bg, &api.VaultRemoveParams{ID: v.ID}); rerr != nil {
			return nil, cleanupFailed(err, r.Name, rerr)
		}
		return nil, err
	}
	return v, nil
}
