package session

// FanOutForTest calls fanOut directly with chunk for use in unit tests.
func (s *Session) FanOutForTest(
	chunk []byte,
) {
	s.fanOut(chunk)
}

// ClientSendBufForTest exposes the constant for test assertions.
const ClientSendBufForTest = clientSendBuf
