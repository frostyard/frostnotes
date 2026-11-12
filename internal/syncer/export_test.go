package syncer

// CancelLoop ends vault's loop, as a stop of its notesd would, and does
// not wait for it: a crash hook calls it on the loop's own goroutine, where
// waiting would deadlock.
func (s *Syncer) CancelLoop(vaultID int64) {
	if l := s.loop(vaultID); l != nil {
		l.cancel()
	}
}

// WaitLoop waits for vault's loop to return, after CancelLoop or the end
// of the syncer's context.
func (s *Syncer) WaitLoop(vaultID int64) {
	if l := s.loop(vaultID); l != nil {
		<-l.done
	}
}
