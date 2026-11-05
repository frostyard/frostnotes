//go:build linux

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/std/problems"

	"github.com/frostyard/frostnotes/internal/problem"
	"github.com/frostyard/frostnotes/internal/problem/problemtest"
	"github.com/frostyard/frostnotes/internal/store"
)

// The child processes of TestACrashOfNotesdIsRecordedAtTheNextStart: the
// test binary runs itself with childEnv set, and TestMain runs the child
// instead of the tests.
const (
	childEnv   = "FROSTNOTES_TEST_PROBLEMS_CHILD"
	journalEnv = "FROSTNOTES_TEST_JOURNAL"
	childPanic = "a bug in a goroutine of notesd"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(childEnv); mode != "" {
		os.Exit(runChild(mode))
	}
	os.Exit(m.Run())
}

// runChild runs notesd at release 1.2.3, recording to the journal socket
// in journalEnv. "crash" panics in a goroutine of its own once notesd
// serves, which ends the process; "start" stops as soon as notesd serves.
func runChild(mode string) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o := runOptions{version: "1.2.3", problems: []problems.Option{problems.WithSocket(os.Getenv(journalEnv))}}
	switch mode {
	case "crash":
		o.serving = func() { go func() { panic(childPanic) }() }
	case "start":
		o.serving = cancel
	default:
		fmt.Fprintln(os.Stderr, "unknown child mode", mode)
		return 1
	}
	if err := runWith(ctx, nil, o); err != nil {
		fmt.Fprintln(os.Stderr, "notesd:", err)
		return 1
	}
	return 0
}

// Version is the version a tag gives: without its v, a release semver;
// a development build's version makes the recorder inert.
func TestReleaseVersion(t *testing.T) {
	for in, want := range map[string]string{"v0.5.0": "0.5.0", "0.5.0": "0.5.0", "v1.2.3-rc.1": "1.2.3-rc.1",
		"vv1.0.0": "v1.0.0", "dev": "dev", "flatpak": "flatpak"} {
		if got := releaseVersion(in); got != want {
			t.Errorf("releaseVersion(%q) = %q; want %q", in, got, want)
		}
	}
	for _, v := range []string{"v0.5.0", "0.5.0", "v1.2.3-rc.1"} {
		if err := problems.New(problemtest.AppID, releaseVersion(v), problems.ComponentDaemon).Err(); err != nil {
			t.Errorf("a recorder at %s is inert: %v", v, err)
		}
	}
	for _, v := range []string{"dev", "flatpak", "v0.5.0-3-gabc1234", "0.0.0-abc1234-snapshot", "v0.5.0-dirty"} {
		if problems.New(problemtest.AppID, releaseVersion(v), problems.ComponentDaemon).Err() == nil {
			t.Errorf("a recorder at %s records", v)
		}
	}
}

// A database whose migration fails stops notesd, and is recorded in the
// journal as store.migrate_failed, with the tag's version without its v
// (ADR-0036). This test goes through a real journal socket end to end.
func TestAFailedMigrationIsRecorded(t *testing.T) {
	sb := newSandbox(t)
	j := problemtest.Listen(t)
	path := filepath.Join(sb.data, "notesd.db")
	if err := os.MkdirAll(sb.data, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO schema_migrations (version, name, applied_at, checksum)
		VALUES (999, 'future.sql', 'x', 'x')`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	err = runWith(t.Context(), nil, runOptions{version: "v1.2.3", problems: []problems.Option{problems.WithSocket(j.Path)}})
	if me := (*store.MigrateError)(nil); !errors.As(err, &me) {
		t.Fatalf("runWith = %v; want the failed migration", err)
	}
	problemtest.Check(t, j.Next(t), problemtest.Want{Version: "1.2.3", Class: "error", Code: problem.StoreMigrateFailed})
	j.None(t)
}

// A panic in notesd's main goroutine is recorded, with its frames, before
// it goes on (ADR-0036).
func TestAPanicOfNotesdIsRecorded(t *testing.T) {
	newSandbox(t)
	j := problemtest.Listen(t)
	o := runOptions{version: "1.2.3", problems: []problems.Option{problems.WithSocket(j.Path)},
		serving: func() { panic("a bug in notesd") }}
	func() {
		defer func() {
			if x := recover(); x != "a bug in notesd" {
				t.Fatalf("recovered %v; want notesd's panic", x)
			}
		}()
		_ = runWith(t.Context(), nil, o)
		t.Fatal("runWith returned after a panic")
	}()
	f := j.Next(t)
	problemtest.Check(t, f, problemtest.Want{Version: "1.2.3", Class: "panic"})
	// The frame is main.runWith in notesd, and has the import path in a test.
	frames := strings.Split(f["FROSTYARD_FRAMES"], "\n")
	if !slices.ContainsFunc(frames, func(fr string) bool {
		return fr == "main.runWith" || fr == "github.com/frostyard/frostnotes/cmd/notesd.runWith"
	}) {
		t.Errorf("frames %q lack runWith", frames)
	}
	j.None(t)
}

// A crash that no deferred recovery sees (a panic in another goroutine)
// ends notesd; the crash file that notesd keeps (CrashOutput) becomes 1
// record at its next start (ADR-0036).
func TestACrashOfNotesdIsRecordedAtTheNextStart(t *testing.T) {
	newSandbox(t)
	j := problemtest.Listen(t)
	child := func(mode string) (string, error) {
		cmd := exec.CommandContext(t.Context(), os.Args[0])
		cmd.Env = append(os.Environ(), childEnv+"="+mode, journalEnv+"="+j.Path)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := child("crash")
	if ee := (*exec.ExitError)(nil); !errors.As(err, &ee) || ee.ExitCode() != 2 || !strings.Contains(out, childPanic) {
		t.Fatalf("the crashing notesd = %v; want exit status 2 and its panic:\n%s", err, out)
	}
	j.None(t) // the crash itself records nothing
	if out, err := child("start"); err != nil {
		t.Fatalf("the next notesd = %v:\n%s", err, out)
	}
	f := j.Next(t)
	problemtest.Check(t, f, problemtest.Want{Version: "1.2.3", Class: "panic"})
	if !strings.Contains(f["MESSAGE"], childPanic) {
		t.Errorf("MESSAGE = %q; want the panic %q", f["MESSAGE"], childPanic)
	}
	j.None(t)
}
