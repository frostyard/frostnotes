package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/config"
)

const (
	// shutdownWait is how long shutdown waits for notesd to stop answering.
	shutdownWait = 30 * time.Second
	// shutdownPoll is how often it asks again.
	shutdownPoll = 100 * time.Millisecond
)

// newShutdownCmd builds shutdown: stop notesd and wait until it has stopped.
func newShutdownCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "shutdown",
		Short: "Stop notesd and wait until it no longer answers",
		Long: `Call rpc.shutdown, which makes notesd finish the work in hand, release its
data directory's lock and exit, then wait for it, 30 seconds in all (the
calls and probes too): until the socket no longer answers rpc.hello, and until
shutdown can take the data directory's lock itself, which it releases at once.
That also holds when nothing answers at first: a notesd that is starting or
stopping holds the lock, and shutdown waits for it, and asks it to stop again
if it begins to answer. It exits 0 once the socket is silent and the lock is
free: with "notesd is not running" when nothing answered and the lock was free
at once, else "notesd stopped". It exits non-zero when 30 seconds pass first.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			socket, err := opts.socketPath()
			if err != nil {
				return err
			}
			// The data directory, as notesd resolves it; the socket is the one
			// this command dials, so no default socket is needed.
			paths, err := config.Resolve(func(k string) string {
				if k == "FROSTNOTES_SOCKET" {
					return socket
				}
				return os.Getenv(k)
			})
			if err != nil {
				return err
			}
			return runShutdown(cmd.Context(), cmd.OutOrStdout(), socket, paths.DataDir, shutdownWait)
		},
	}
}

// runShutdown stops the notesd on socket, within wait in all: one deadline
// covers the dial, rpc.shutdown, every probe and the wait for the lock. It
// repeats until the socket does not answer rpc.hello and dataDir's lock is
// free, and then exits 0. Each round:
//   - a notesd that answers is asked to stop (again, when it still answers
//     after an earlier ask);
//   - when nothing answers, the lock is tried: a notesd that has closed its
//     socket can still be finishing its last transaction, and one that is
//     starting holds the lock before it listens. A held lock is waited for.
//
// "notesd is not running" means nothing answered and the lock was free at
// once.
func runShutdown(parent context.Context, w io.Writer, socket, dataDir string, wait time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, wait)
	defer cancel()
	var (
		asked    bool // a notesd answered and was told to stop
		lockHeld bool // the lock was held at some round
		asking   bool // inside the rpc.shutdown call
	)
	// stuck is the error for the deadline: what was still true when it came.
	stuck := func() error {
		switch {
		case parent.Err() != nil:
			return parent.Err()
		case asking:
			return fmt.Errorf("notesd did not answer rpc.shutdown on %s within %s: %w", socket, wait, context.DeadlineExceeded)
		case lockHeld:
			return fmt.Errorf("notesd still holds the lock of %s after %s: %w", dataDir, wait, context.DeadlineExceeded)
		case asked:
			return fmt.Errorf("notesd still answers on %s after %s: %w", socket, wait, context.DeadlineExceeded)
		}
		return fmt.Errorf("notesd did not answer on %s within %s: %w", socket, wait, context.DeadlineExceeded)
	}
	// timedOut is true once the deadline has come. A dial or a read stops at
	// the deadline a moment before the context reports it, and that error
	// must not count as "nothing answers".
	limit, _ := ctx.Deadline()
	timedOut := func() bool { return ctx.Err() != nil || !time.Now().Before(limit) }
	tick := time.NewTicker(shutdownPoll)
	defer tick.Stop()
	for {
		c, _, err := api.Dial(ctx, socket, "notesctl shutdown "+version)
		var rpcErr *api.Error
		switch {
		case err == nil:
			asking = true
			err = c.RPC().Shutdown(ctx, &api.RPCShutdownParams{})
			_ = c.Close()
			if timedOut() {
				return stuck() // still asking
			}
			if err != nil {
				return fmt.Errorf("rpc.shutdown: %w", err)
			}
			asking, asked = false, true
		case errors.As(err, &rpcErr):
			return err // something answers, but not as notesd does
		case timedOut():
			return stuck()
		default: // nothing answers: only the lock is left to wait for
			free, err := lockFree(dataDir)
			if err != nil {
				return err
			}
			if free {
				if asked || lockHeld {
					fmt.Fprintln(w, "notesd stopped")
				} else {
					fmt.Fprintln(w, "notesd is not running")
				}
				return nil
			}
			lockHeld = true
		}
		select {
		case <-ctx.Done():
			return stuck()
		case <-tick.C:
		}
	}
}

// lockFree takes dataDir's lock (notesd's flock on config.LockFile) without
// waiting and releases it at once. A data directory or a lock file that does
// not exist has no holder.
func lockFree(dataDir string) (bool, error) {
	f, err := os.OpenFile(filepath.Join(dataDir, config.LockFile), os.O_RDWR, 0)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, nil
	case err != nil:
		return false, err
	}
	defer func() { _ = f.Close() }() // closing releases the lock
	switch err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.EWOULDBLOCK):
		return false, nil
	default:
		return false, fmt.Errorf("lock %s: %w", dataDir, err)
	}
}
