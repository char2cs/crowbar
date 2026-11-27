package session

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

func (s *Session) SetNowForTest(fn func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = fn
}

func (s *Session) PumpNotifyForTest() <-chan struct{} { return s.pumpNotify }

func (s *Session) SerializedForTest() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.serializeLocked()
}

func SetNewModelForTest(
	fn func(cols, rows, sb int) (model.TerminalModel, model.Serializer),
) (restore func()) {
	orig := newModel
	newModel = fn
	return func() { newModel = orig }
}
