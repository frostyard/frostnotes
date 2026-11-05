package syncer

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/problem"
	"github.com/frostyard/frostnotes/internal/problem/problemtest"
)

// decodeErr is a real error of the JSON decoder for data into v.
func decodeErr(t *testing.T, data string, v any) error {
	t.Helper()
	err := json.Unmarshal([]byte(data), v)
	if err == nil {
		t.Fatalf("%s decoded", data)
	}
	return err
}

// Which ends of a session are protocol faults (ADR-0036).
func TestProtocolFaultClassifiesTheEndOfASession(t *testing.T) {
	var page api.LogPage
	for _, tc := range []struct {
		name  string
		err   error
		fault bool
	}{
		{"nil", nil, false},
		{"parse error", api.Errorf(api.CodeParseError, "bad json"), true},
		{"invalid request", api.Errorf(api.CodeInvalidRequest, "no method"), true},
		{"method not found", fmt.Errorf("log.pull: %w", api.Errorf(api.CodeMethodNotFound, "no log.pull")), true},
		{"invalid params", api.InvalidParams("cursor is negative"), true},
		{"protocol mismatch", api.ProtocolMismatch("the hub speaks protocol 2"), true},
		{"a result that does not decode", fmt.Errorf("decode log.pull result: %w", decodeErr(t, `{"files": 3}`, &page)), true},
		{"a message that is not JSON", fmt.Errorf("hub: malformed message: %w", decodeErr(t, `{"jsonrpc":`, &page)), true},
		{"internal", api.Errorf(api.CodeInternal, "log.pull failed"), false},
		{"not found", api.NotFound("no such vault"), false},
		{"conflict", api.Conflict("busy"), false},
		{"not ready", api.Errorf(api.CodeNotReady, "say hello first"), false},
		{"unavailable", api.Unavailable("the hub is stopping"), false},
		{"revoked by code", api.Errorf(api.CodeRevoked, "token revoked"), false},
		{"revoked", errRevoked, false},
		{"reset", errReset, false},
		{"canceled", fmt.Errorf("pull: %w", context.Canceled), false},
		{"deadline", context.DeadlineExceeded, false},
		{"connection refused", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, false},
		{"connection closed", errors.New("hub: connection closed"), false},
		{"eof", fmt.Errorf("read: %w", io.ErrUnexpectedEOF), false},
	} {
		if got := protocolFault(tc.err); got != tc.fault {
			t.Errorf("%s: protocolFault(%v) = %v; want %v", tc.name, tc.err, got, tc.fault)
		}
	}
}

// A run of sessions that end on a protocol fault gives 1 record; a session
// that ends otherwise starts a new run.
func TestAProtocolFaultIsRecordedOncePerRun(t *testing.T) {
	fake := &problemtest.Fake{}
	var f faultRecord
	fault := api.InvalidParams("bad cursor")
	for _, err := range []error{
		errors.New("hub: connection closed"), // 0
		fault,                                // 1: recorded
		fault,                                // the same run
		api.ProtocolMismatch("protocol 2"),   // the same run
		errors.New("hub: connection closed"), // the run ends
		fault,                                // 2: recorded
	} {
		f.ended(fake, err)
	}
	want := []string{problem.SyncProtocolError, problem.SyncProtocolError}
	if got := fake.Codes(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("recorded %q; want %q", got, want)
	}
	f.ended(nil, fault) // a nil recorder records nothing, and does not panic
}
