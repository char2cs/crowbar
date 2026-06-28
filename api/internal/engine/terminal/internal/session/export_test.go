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

// PumpChunkForTest simulates one fixed pump cycle: holds s.mu across ring.Write
// and fanOutLocked, matching the post-fix pump behavior. This prevents Attach()
// from interleaving between the ring write and the fan-out delivery.
func (s *Session) PumpChunkForTest(chunk []byte) {
	s.mu.Lock()
	s.ring.Write(chunk)
	s.fanOutLocked(chunk)
	s.mu.Unlock()
}

// ClientSendBufForTest exposes the constant for test assertions.
const ClientSendBufForTest = clientSendBuf
