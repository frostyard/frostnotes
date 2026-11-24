package secrets

import (
	"github.com/frostyard/frostnotes/internal/config"
	"strings"
	"testing"
)

func TestOptionsFor(t *testing.T) {
	if got := OptionsFor(config.AppID); got != FlatpakOptions() {
		t.Errorf("OptionsFor(config.AppID) = %+v, want %+v", got, FlatpakOptions())
	}
	if got := OptionsFor(""); got != HostOptions() {
		t.Errorf("host policy = %+v, want %+v", got, HostOptions())
	}
	preview := OptionsFor(config.PreviewAppID)
	if !preview.NoFallback || preview.application != config.PreviewAppID || !strings.Contains(preview.Hint, config.PreviewAppID) {
		t.Fatalf("preview storage policy = %+v", preview)
	}
}
