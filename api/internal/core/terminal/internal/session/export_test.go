package session

func (s *Session) FanOutForTest(
	chunk []byte,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fanOutFrameLocked(OutputFrame{SessionID: s.id, Data: chunk})
}

func (s *Session) PumpChunkForTest(chunk []byte) {
	s.pumpStep(chunk)
}

const ClientSendBufForTest = clientSendBuf
