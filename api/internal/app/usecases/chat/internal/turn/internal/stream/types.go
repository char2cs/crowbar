package stream

import "time"

// Message is a frozen copy of one streamed assistant message, computed while the
// stream lock is held. It is the ONLY shape Streams hands out: the termwait sweep
// goroutine reads a message at the same instant a hook goroutine appends
// increments to it, so a shared buffer pointer was a concurrent map read and map
// write on the buffer's chunks — a fatal throw, not a recoverable error.
//
// Freezing Text and RecordedText together is load-bearing a second time: a message
// recorded from one Text must be marked recorded with that same value, or a buffer
// that grew in between leaves RecordedText ahead of what was persisted and the tail
// is dropped as already-recorded.
type Message struct {
	ID           string
	TurnID       string
	Text         string
	RecordedText string
	Final        bool
	Complete     bool
	LastAt       time.Time
}
