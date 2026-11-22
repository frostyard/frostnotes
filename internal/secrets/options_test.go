package secrets

import "testing"

func TestOptionsFor(t *testing.T) {
	if got := OptionsFor(true); got != FlatpakOptions() {
		t.Errorf("OptionsFor(true) = %+v, want %+v", got, FlatpakOptions())
	}
	if got := OptionsFor(false); got != HostOptions() {
		t.Errorf("OptionsFor(false) = %+v, want %+v", got, HostOptions())
	}
}
