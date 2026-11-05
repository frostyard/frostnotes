package syncer

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/problem"
)

// protocolFault reports whether err, the error that ended a sync session,
// shows a fault of the protocol between the device and the hub, which
// notesd records as sync.protocol_error (ADR-0036): the hub answered that
// the device's request was malformed, named no method it has, had bad
// params, or spoke another protocol; or the hub's reply could not be
// decoded. The network, a revoked token, a reset vault, a hub that is not
// ready or unavailable, and the end of the loop are not protocol faults.
func protocolFault(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, errRevoked) || errors.Is(err, errReset) {
		return false
	}
	var ae *api.Error
	if errors.As(err, &ae) {
		switch ae.Code {
		case api.CodeParseError, api.CodeInvalidRequest, api.CodeMethodNotFound, api.CodeInvalidParams,
			api.CodeProtocolMismatch:
			return true
		}
		return false
	}
	// A reply or a message of the hub that does not decode.
	var se *json.SemanticError
	var ye *jsontext.SyntacticError
	return errors.As(err, &se) || errors.As(err, &ye)
}

// faultRecord records sync.protocol_error for a loop: once for a run of
// sessions that end on a protocol fault, so a hub that keeps answering
// with one gives 1 record, not 1 at each reconnection.
type faultRecord struct{ last bool }

// ended takes the error that ended a session, and records it when it is a
// protocol fault and the session before did not end on one.
func (f *faultRecord) ended(rec problem.Recorder, err error) {
	fault := protocolFault(err)
	if fault && !f.last {
		problem.Record(rec, problem.SyncProtocolError, err)
	}
	f.last = fault
}
