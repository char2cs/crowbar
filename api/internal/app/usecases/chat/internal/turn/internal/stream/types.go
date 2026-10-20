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
	ID string
	// Reply is the reply its turn closed on, when the message is that reply
	// streamed late; zero when the message is its own.
	Reply        Answer
	RunnerID     string
	TurnID       string
	Text         string
	RecordedText string
	Final        bool
	Complete     bool

	// FirstAt is when this message first appeared, before any later tool call
	// or reply item the provider may emit in the same turn.
	FirstAt time.Time
	LastAt  time.Time
}

// Answer is a turn's reply recorded from the hook that closed it, no message of
// it having streamed yet: a final message of that turn landing later joins it.
type Answer struct {
	MessageID, Text, Effort, Status, Diff string
	// TurnID is the chat's newest turn when the reply was recorded; HasTurn is
	// false when the chat had none.
	TurnID  string
	HasTurn bool
}
