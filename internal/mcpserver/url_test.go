package mcpserver

import (
	"encoding/json/v2"
	"net/url"
	"os"
	"strings"
	"testing"
)

// A note's url is the web app's route under /notes: the cases its own
// route tests hold app/src/lodge/routes.ts to (spaces, #, %, reserved
// characters, NFC and NFD, CJK, emoji) give the same address here.
func TestNoteURLIsTheWebAppsRoute(t *testing.T) {
	data, err := os.ReadFile("../../app/src/lodge/note-routes.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		VaultID int64  `json:"vaultId"`
		Path    string `json:"path"`
		Route   string `json:"route"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 10 {
		t.Fatalf("%d cases", len(cases))
	}
	for _, c := range cases {
		got := noteURL(c.VaultID, c.Path)
		if got != "/notes"+c.Route {
			t.Errorf("noteURL(%d, %q) = %q, want /notes%s", c.VaultID, c.Path, got, c.Route)
		}
		// Each segment decodes back to the path's, as a browser reads it.
		segs := strings.Split(strings.TrimPrefix(got, "/notes/"), "/")[1:]
		for i, seg := range segs {
			segs[i], err = url.PathUnescape(seg)
			if err != nil {
				t.Fatal(err)
			}
		}
		if p := strings.Join(segs, "/"); p != c.Path {
			t.Errorf("%s decodes to %q, want %q", got, p, c.Path)
		}
	}
}

// uriComponent leaves exactly encodeURIComponent's characters as they are.
func TestURIComponentKeepsOnlyUnreserved(t *testing.T) {
	for c := range 256 {
		s := string([]byte{byte(c)})
		got := uriComponent(s)
		keep := 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.ContainsRune("-_.!~*'()", rune(c))
		if keep != (got == s) || !keep && len(got) != 3 {
			t.Errorf("uriComponent(%q) = %q", s, got)
		}
	}
}
