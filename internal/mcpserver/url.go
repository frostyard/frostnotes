package mcpserver

import (
	"fmt"
	"strconv"
	"strings"
)

// noteURL is a note's canonical address in lodge, as notes' web app
// routes it and lodge.open takes it: /notes/<vault id>/<path>, each
// segment percent-encoded as JavaScript's encodeURIComponent encodes it
// (app/src/lodge/routes.ts; docs/design/app.md#in-a-browser).
func noteURL(vaultID int64, p string) string {
	segs := strings.Split(p, "/")
	for i, seg := range segs {
		segs[i] = uriComponent(seg)
	}
	return "/notes/" + strconv.FormatInt(vaultID, 10) + "/" + strings.Join(segs, "/")
}

// uriComponent is encodeURIComponent: every byte of s but ASCII letters,
// digits and -_.!~*'() percent-encoded, in upper-case hex.
func uriComponent(s string) string {
	var b strings.Builder
	for i := range len(s) {
		c := s[i]
		if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte("-_.!~*'()", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// url is the note's address in lodge, for a result: "" but through lodge,
// where the vault's ID is the one lodge's routes name.
func (t *tools) url(p string) string {
	if !t.o.URLs {
		return ""
	}
	return noteURL(t.o.VaultID, p)
}
