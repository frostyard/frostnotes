package api_test

import (
	"testing"

	"github.com/frostyard/frostnotes/api"
)

func TestParseScope(t *testing.T) {
	for in, want := range map[string]api.Scope{
		"vault=1":                   {Vault: 1},
		"vault=42;write":            {Vault: 42, Write: true},
		"vault=9223372036854775807": {Vault: 9223372036854775807},
	} {
		got, err := api.ParseScope(in)
		if err != nil || got != want || got.String() != in {
			t.Errorf("ParseScope(%q) = %+v (%s), %v; want %+v", in, got, got.String(), err, want)
		}
	}
	for _, in := range []string{"", "vault=", "vault=0", "vault=-1", "vault=01", "vault=+1", "vault=1;read",
		"vault=1;write;write", "vault=1 ", " vault=1", "vault=a", "vault=9223372036854775808", "VAULT=1", "vault=1;"} {
		if got, err := api.ParseScope(in); err == nil {
			t.Errorf("ParseScope(%q) = %+v, want an error", in, got)
		}
	}
}
