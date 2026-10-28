package engine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/sandbox"
)

// flatpak returns the vault service of a Flatpak whose file access is
// filesystems, with home as the home directory.
func flatpak(t *testing.T, home, filesystems string) vaults {
	t.Helper()
	g, err := sandbox.Parse(strings.NewReader("[Context]\nfilesystems="+filesystems+"\n"), sandbox.Env{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	return vaults{sandbox: g}
}

// A vault outside the Flatpak's file access says so; one inside it, and
// any vault outside a Flatpak, keep the plain file error.
func TestVaultPathExplainsARefusedFlatpakPath(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "Notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	narrow := flatpak(t, home, "~/Notes;")
	outside := filepath.Join(home, "Elsewhere", "Vault")
	inside := filepath.Join(home, "Notes", "Missing", "Vault") // its parent is missing too

	for _, create := range []bool{false, true} {
		_, err := narrow.vaultPath(outside, create)
		var rpc *api.Error
		if !errors.As(err, &rpc) || rpc.Code != api.CodeInvalidParams {
			t.Fatalf("create=%t: vaultPath(%s) = %v, want an invalidParams error", create, outside, err)
		}
		want := "Frostnotes cannot reach " + outside + " because the Flatpak's file access does not include it; " +
			"grant it with: flatpak override --user --filesystem=" + outside + " org.frostyard.Frostnotes, " +
			"then log out and in (notesd keeps its sandbox until then)."
		if rpc.Message != want {
			t.Errorf("create=%t: message =\n%s\nwant\n%s", create, rpc.Message, want)
		}

		_, err = narrow.vaultPath(inside, create)
		if err == nil || strings.Contains(err.Error(), "Frostnotes cannot") {
			t.Errorf("create=%t: vaultPath(%s) = %v, want a plain error: the grants cover it", create, inside, err)
		}
		_, err = vaults{}.vaultPath(outside, create)
		if err == nil || strings.Contains(err.Error(), "Frostnotes cannot") {
			t.Errorf("create=%t: outside a Flatpak, vaultPath(%s) = %v, want a plain error", create, outside, err)
		}
	}
}

// A path the grants cover opens, in a Flatpak as outside one.
func TestVaultPathOpensACoveredFlatpakPath(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "Notes")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := flatpak(t, home, "~/Notes;").vaultPath(dir, false)
	if err != nil || got != real {
		t.Fatalf("vaultPath(%s) = %q, %v; want %q", dir, got, err, real)
	}
}
