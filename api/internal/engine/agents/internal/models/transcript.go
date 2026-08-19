package models

import "time"

// TranscriptMessage is one thing the agent SAID, read from the provider's own
// transcript rather than from a hook.
//
// It carries no identity of its own on purpose. A transcript entry's id is the
// provider's, and Crowbar's record is keyed by its own hook delivery ids; adding
// a provider id here would invite a join between two identity spaces that are
// only accidentally aligned.
type TranscriptMessage struct {
	Text string
	// At is when the provider says it was said. Zero when the provider reported
	// nothing usable, which the caller reads as "date it now".
	At time.Time
	// Truncated reports that Text is a prefix of what was said, cut at the
	// declared ceiling and marked in the text itself.
	Truncated bool
}

// TranscriptRead is one incremental read of a provider transcript.
type TranscriptRead struct {
	// Messages are what the agent said in the bytes this read consumed, in order.
	Messages []TranscriptMessage
	// Offset is where the NEXT read starts. It never moves backwards and never
	// lands mid-line, so a file being appended to while it is read is safe: the
	// partial tail is simply left for next time.
	Offset int64
	// Truncated reports that the window was larger than one read is allowed to
	// be, so Offset stopped short of the end. The remainder is read next time.
	Truncated bool
}
