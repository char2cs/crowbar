package session

// FanOutForTest calls fanOut directly with chunk for use in unit tests.
func (s *Session) FanOutForTest(
	chunk []byte,
) {
	s.fanOut(chunk)
}

// RingWriteForTest writes p directly to the session's ring buffer for testing.
func (s *Session) RingWriteForTest(p []byte) {
	s.ring.Write(p)
}

// PumpChunkForTest delegates to pumpStep, the production critical section used by
// pump(). This means the regression test exercises the real code path: a future
// regression that removes the lock from pumpStep will be caught by the race detector.
func (s *Session) PumpChunkForTest(chunk []byte) {
	s.pumpStep(chunk)
}

// ClientSendBufForTest exposes the constant for test assertions.
const ClientSendBufForTest = clientSendBuf
