// Package dbustest runs a private D-Bus daemon for tests of code that talks
// to session services (the Secret Service, notifications).
package dbustest

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

// Bus runs a dbus-daemon for one test and returns its address. The test is
// skipped where dbus-daemon is not installed, unless FROSTNOTES_REQUIRE_DBUS
// is set (CI sets it), which makes a missing daemon a failure
// (frostyard/core ADR-0022).
func Bus(t testing.TB) string {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		if os.Getenv("FROSTNOTES_REQUIRE_DBUS") != "" {
			t.Fatal("FROSTNOTES_REQUIRE_DBUS is set and dbus-daemon is not installed")
		}
		t.Skip("no dbus-daemon")
	}
	dir := t.TempDir()
	cfg := filepath.Join(dir, "bus.conf")
	conf := `<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig><type>session</type><listen>unix:path=` + filepath.Join(dir, "bus") + `</listen>
<auth>EXTERNAL</auth><policy context="default"><allow send_destination="*" eavesdrop="true"/>
<allow eavesdrop="true"/><allow own="*"/></policy></busconfig>`
	if err := os.WriteFile(cfg, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(daemon, "--config-file="+cfg, "--nofork", "--print-address")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("dbus-daemon: %v", err)
	}
	return strings.TrimSpace(line)
}

// Connect opens a connection to the bus at addr, closed when the test ends.
func Connect(t testing.TB, addr string) *dbus.Conn {
	t.Helper()
	conn, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
