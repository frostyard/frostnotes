//go:build !windows

package config

import "testing"

func TestResolveDefaults(t *testing.T) {
	p, err := Resolve(env(map[string]string{"HOME": "/home/u", "XDG_RUNTIME_DIR": "/run/user/1000"}))
	if err != nil {
		t.Fatal(err)
	}
	want := Paths{
		DataDir: "/home/u/.local/share/frostnotes",
		Socket:  "/run/user/1000/frostnotes/notesd.sock",
		DB:      "/home/u/.local/share/frostnotes/notesd.db", Blobs: "/home/u/.local/share/frostnotes/blobs",
	}
	if p != want {
		t.Fatalf("Resolve = %+v\nwant %+v", p, want)
	}
}

func TestResolveOverridesAndRelativeXDG(t *testing.T) {
	p, err := Resolve(env(map[string]string{
		"HOME": "/home/u", "XDG_DATA_HOME": "relative/ignored",
		"FROSTNOTES_SOCKET": "/s/notesd.sock",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if p.DataDir != "/home/u/.local/share/frostnotes" || p.Socket != "/s/notesd.sock" {
		t.Fatalf("Resolve = %+v", p)
	}
	p, err = Resolve(env(map[string]string{"FROSTNOTES_DATA_DIR": "/d", "FROSTNOTES_SOCKET": "/s"}))
	if err != nil || p.DB != "/d/notesd.db" {
		t.Fatalf("Resolve with FROSTNOTES_DATA_DIR = %+v, %v", p, err)
	}
}

func TestResolveNeedsRuntimeDir(t *testing.T) {
	if _, err := Resolve(env(map[string]string{"HOME": "/home/u"})); err == nil {
		t.Fatal("want an error without XDG_RUNTIME_DIR or FROSTNOTES_SOCKET")
	}
}

func TestDefaultSocketInTheFlatpak(t *testing.T) {
	run := "/run/user/1000"
	for _, c := range []struct {
		name, flatpakID, want string
	}{
		{"flatpak", AppID, "/run/user/1000/app/org.frostyard.Frostnotes/frostnotes/notesd.sock"},
		{"another flatpak", "org.example.Other", "/run/user/1000/frostnotes/notesd.sock"},
		{"no flatpak", "", "/run/user/1000/frostnotes/notesd.sock"},
	} {
		got, err := DefaultSocket(env(map[string]string{"XDG_RUNTIME_DIR": run, "FLATPAK_ID": c.flatpakID}))
		if err != nil || got != c.want {
			t.Errorf("%s: DefaultSocket = %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

func TestResolveSocketOverrideBeatsTheFlatpak(t *testing.T) {
	p, err := Resolve(env(map[string]string{
		"HOME": "/home/u", "XDG_RUNTIME_DIR": "/run/user/1000", "FLATPAK_ID": AppID,
		"FROSTNOTES_SOCKET": "/s/notesd.sock",
	}))
	if err != nil || p.Socket != "/s/notesd.sock" {
		t.Fatalf("Resolve = %+v, %v", p, err)
	}
}
