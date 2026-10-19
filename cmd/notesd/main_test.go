package main

import "testing"

func TestHubListensOnLoopbackOnly(t *testing.T) {
	for _, ok := range []string{"127.0.0.1:7880", "[::1]:7880", "localhost:0", "127.0.0.2:1"} {
		if err := checkLoopback(ok); err != nil {
			t.Errorf("checkLoopback(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{":7880", "0.0.0.0:7880", "[::]:7880", "100.64.0.1:7880", "hub.example:7880", "127.0.0.1"} {
		if err := checkLoopback(bad); err == nil {
			t.Errorf("checkLoopback(%q) accepted a non-loopback address", bad)
		}
	}
}
