package vaults

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/internal/desktop/desktoptest"
)

func TestACleanupFailureIsNotACancel(t *testing.T) {
	err := cleanupFailed(context.Canceled, "Notes", errors.New("gone"))
	if errors.Is(err, context.Canceled) {
		t.Fatal("the cleanup failure would be dropped as a cancel")
	}
	if !strings.Contains(err.Error(), "Notes") {
		t.Fatalf("%v", err)
	}
}

func TestACancelDoesNotCutTheAddShort(t *testing.T) {
	f := desktoptest.StartNotesd(t)
	dir := filepath.Join(t.TempDir(), "new")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := Join{Client: f.Other, Context: ctx, Folder: dir}
	v, err := r.addVault()
	if err != nil || v == nil {
		t.Fatalf("vault %v, err %v", v, err)
	}
}

func TestLostAddAnswerExplainsPossibleVault(t *testing.T) {
	f := desktoptest.StartNotesd(t)
	if err := f.Other.Close(); err != nil {
		t.Fatal(err)
	}
	r := Join{Client: f.Other, Folder: "/made-up/vault"}
	_, err := r.addVault()
	if err == nil || !strings.Contains(err.Error(), "Manage vaults") || !strings.Contains(err.Error(), r.Folder) {
		t.Fatalf("lost answer: %v", err)
	}
	if left := (*VaultLeft)(nil); !errors.As(err, &left) {
		t.Fatal("a cancelled join would drop the lost answer")
	}
}
