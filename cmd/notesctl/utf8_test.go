package main

import (
	"strings"
	"testing"
)

// notesd's API carries text (ADR-0014): notesctl refuses bytes that are not
// UTF-8 before it sends them.
func TestWriteRefusesInvalidUTF8(t *testing.T) {
	fake := &fakeNotes{}
	srv := notesServer(t, fake, "garden")
	_, err := notesIn(t, srv, "caf\xe9\n", "write", "--create", "Menu.md")
	if err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("write of invalid UTF-8 = %v, want a UTF-8 error", err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("notesctl sent %d calls", len(fake.calls))
	}
}
