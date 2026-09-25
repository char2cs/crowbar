package session

// FanOutForTest fans chunk out to every attached client as one frame.
func (s *Session) FanOutForTest(
	chunk []byte,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fanOutFrameLocked(OutputFrame{SessionID: s.id, Data: chunk})
}

// PumpChunkForTest delegates to pumpStep, the production critical section used by
// pump(). This means the regression test exercises the real code path: a future
// regression that removes the lock from pumpStep will be caught by the race detector.
func (s *Session) PumpChunkForTest(chunk []byte) {
	s.pumpStep(chunk)
}

// ClientSendBufForTest exposes the constant for test assertions.
const ClientSendBufForTest = clientSendBuf
