package api_test

import (
	"reflect"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

// porter is Porter's scope (ADR-0028).
const porter = "vault=7;write=Porter/;except=Porter/Routines/;daily"

func TestParseScope(t *testing.T) {
	for in, want := range map[string]api.Scope{
		"vault=1":                               {Vault: 1},
		"vault=42;write":                        {Vault: 42, Write: true},
		"vault=9223372036854775807":             {Vault: 9223372036854775807},
		porter:                                  {Vault: 7, WriteTo: []string{"Porter/"}, Except: []string{"Porter/Routines/"}, Daily: true},
		"vault=7;write=Porter/,Inbox/Clips/":    {Vault: 7, WriteTo: []string{"Porter/", "Inbox/Clips/"}},
		"vault=7;write;except=Private/":         {Vault: 7, Write: true, Except: []string{"Private/"}},
		"vault=7;daily":                         {Vault: 7, Daily: true},
		"vault=7;except=Journal/Private/;daily": {Vault: 7, Except: []string{"Journal/Private/"}, Daily: true},
		"vault=7;write;daily":                   {Vault: 7, Write: true, Daily: true},
		// ",", ";" and "%" are always encoded, as are spaces and every
		// byte outside ASCII.
		"vault=7;write=My%20Notes/,a%2Cb/,c%3Bd/,e%25f/,R%C3%A9sum%C3%A9s/": {Vault: 7,
			WriteTo: []string{"My Notes/", "a,b/", "c;d/", "e%f/", "Résumés/"}},
	} {
		got, err := api.ParseScope(in)
		if err != nil || !reflect.DeepEqual(got, want) || got.String() != in {
			t.Errorf("ParseScope(%q) = %+v (%s), %v; want %+v", in, got, got.String(), err, want)
		}
	}
	for _, in := range []string{
		"", "vault=", "vault=0", "vault=-1", "vault=01", "vault=+1", "vault=1;read",
		"vault=1 ", " vault=1", "vault=a", "vault=9223372036854775808", "VAULT=1", "vault=1;",
		// Each part at most once, in order.
		"vault=1;write;write", "vault=1;write;write=A/", "vault=1;write=A/;write", "vault=1;write=A/;write=B/",
		"vault=1;daily;write", "vault=1;except=A/;write", "vault=1;daily;except=A/;write", "vault=1;write;daily;daily",
		"vault=1;write;except=A/;except=B/", "vault=1;Write", "vault=1;daily=", "vault=1;daily=x", "vault=1;except",
		// except takes from a write right.
		"vault=1;except=A/",
		// Folders: not empty, ending in "/", passing the path guard.
		"vault=1;write=", "vault=1;except=", "vault=1;write=A", "vault=1;write=/", "vault=1;write=A//",
		"vault=1;write=/A/", "vault=1;write=../", "vault=1;write=A/../", "vault=1;write=./", "vault=1;write=A/./",
		"vault=1;write=.obsidian/", "vault=1;write=A/.git/", "vault=1;write=A/,", "vault=1;write=,A/",
		"vault=1;write=A/,,B/", "vault=1;write=A%2FB/", "vault=1;write=A%00/", "vault=1;write=%ZZ/",
		"vault=1;write=%FF/", "vault=1;write=%2E%2E/", "vault=1;write=%2Ehidden/",
		// One spelling: canonical encoding, NFC.
		"vault=1;write=R%c3%a9sum%c3%a9s/", "vault=1;write=Résumés/", "vault=1;write=My Notes/",
		"vault=1;write=%50orter/", "vault=1;write=Re%CC%81sume%CC%81s/",
	} {
		if got, err := api.ParseScope(in); err == nil {
			t.Errorf("ParseScope(%q) = %+v, want an error", in, got)
		}
	}
}

// A scope that parses reads one way: its String is the input.
func FuzzParseScope(f *testing.F) {
	for _, s := range []string{porter, "vault=1", "vault=1;write", "vault=2;write=a%2Cb/;daily", "vault=3;write=A/;except=A/B/"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		sc, err := api.ParseScope(in)
		if err != nil {
			return
		}
		if sc.String() != in {
			t.Fatalf("ParseScope(%q).String() = %q", in, sc.String())
		}
	})
}

func mustScope(t *testing.T, s string) api.Scope {
	t.Helper()
	sc, err := api.ParseScope(s)
	if err != nil {
		t.Fatal(err)
	}
	return sc
}

// Paths are judged segment by segment, after NFC normalization and case
// folding, and except wins: no spelling of Porter/Routines/ is writable.
func TestScopeWritable(t *testing.T) {
	sc := mustScope(t, porter)
	for p, want := range map[string]bool{
		"Porter/Memory.md":            true,
		"Porter/Briefs/2026-10-19.md": true,
		"Porter/Reviews/2026-W42.md":  true,
		"porter/Memory.md":            true,
		"PORTER/briefs/x.md":          true,
		"Porter/Routines.md":          true,
		"Porter/RoutinesOld/x.md":     true,
		"Porter/Routine/x.md":         true,
		"Porter/Routines/Brief.md":    false,
		"Porter/Routines/Sub/x.md":    false,
		"Porter/Routines":             false,
		"porter/routines/Brief.md":    false,
		"PORTER/ROUTINES/Brief.md":    false,
		"Porter/routines/x.md":        false,
		"pOrTeR/rOuTiNeS/x.md":        false,
		"Porter/Routineſ/x.md":        false, // ſ, long s, folds to s
		"Porter":                      false,
		"Porter.md":                   false,
		"Porterhouse.md":              false,
		"Porterhouse/x.md":            false,
		"Notes/Porter/x.md":           false,
		"Plan.md":                     false,
		"2026-10-19.md":               false,
	} {
		if got := sc.Writable(p); got != want {
			t.Errorf("Writable(%q) = %t, want %t", p, got, want)
		}
	}
	// Unicode spellings: NFC and NFD are the same name.
	acc := mustScope(t, "vault=7;write=Porter/;except=Porter/R%C3%A9sum%C3%A9s/")
	for p, want := range map[string]bool{
		"Porter/Résumés/x.md":   false,
		"Porter/Résumés/x.md": false,
		"Porter/RÉSUMÉS/x.md":   false,
		"PORTER/RÉSUMÉS/x.md": false,
		"Porter/Resumes/x.md":   true,
		"Pörter/x.md":           false,
	} {
		if got := acc.Writable(p); got != want {
			t.Errorf("Writable(%q) = %t, want %t", p, got, want)
		}
	}
	whole := mustScope(t, "vault=7;write;except=Private/Keys/")
	for p, want := range map[string]bool{
		"Plan.md": true, "Private/x.md": true, "Private/Keys/x.md": false, "private/keys/x.md": false, "Private/Keys2/x.md": true,
	} {
		if got := whole.Writable(p); got != want {
			t.Errorf("bare write: Writable(%q) = %t, want %t", p, got, want)
		}
	}
	for _, s := range []string{"vault=7", "vault=7;daily"} {
		if sc := mustScope(t, s); sc.Writable("Plan.md") || sc.Writable("Porter/x.md") {
			t.Errorf("%s writes a path", s)
		}
	}
}

// A folder is made where its files could be written, and a write= folder
// itself; it is moved or deleted only when it is writable whole.
func TestScopeFolders(t *testing.T) {
	sc := mustScope(t, porter)
	for p, want := range map[string]bool{
		"Porter": true, "Porter/": true, "porter": true, "Porter/Briefs": true, "Porter/Briefs/2026": true,
		"Porter/Routines": false, "porter/routines": false, "Porter/Routines/Sub": false,
		"Other": false, "Porterhouse": false,
	} {
		if got := sc.MayMakeFolder(p); got != want {
			t.Errorf("MayMakeFolder(%q) = %t, want %t", p, got, want)
		}
	}
	for p, want := range map[string]bool{
		"Porter/Briefs": true, "Porter/Briefs/2026": true, "porter/briefs": true, "Porter/Routines2": true,
		"Porter": false, "porter": false, "Porter/Routines": false, "PORTER/ROUTINES": false,
		"Porter/Routines/Sub": false, "Other": false, "Porterhouse": false,
	} {
		if got := sc.WritableFolder(p); got != want {
			t.Errorf("WritableFolder(%q) = %t, want %t", p, got, want)
		}
	}
	whole := mustScope(t, "vault=7;write;except=Private/Keys/")
	for p, want := range map[string]bool{
		"Notes": true, "Private/Other": true, "Private": false, "PRIVATE": false, "Private/Keys": false, "Private/Keys/Old": false,
	} {
		if got := whole.WritableFolder(p); got != want {
			t.Errorf("bare write: WritableFolder(%q) = %t, want %t", p, got, want)
		}
	}
}

func TestSameName(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"Routines", "routines", true},
		{"Routines", "ROUTINES", true},
		{"Routines", "Routineſ", true},
		{"Résumés", "Résumés", true},
		{"Résumés", "RÉSUMÉS", true},
		{"Routines", "Routines ", false},
		{"Routines", "Routines.", false},
		// Dotless ı folds to no i; a share that upper-cases it to I is
		// caught by the vault's spelling check.
		{"Routines", "Routınes", false},
	} {
		if got := api.SameName(tc.a, tc.b); got != tc.want {
			t.Errorf("SameName(%q, %q) = %t, want %t", tc.a, tc.b, got, tc.want)
		}
	}
}
