package logring

import (
	"log/slog"
	"regexp"
)

const (
	capacity     = 5000
	liveBuffer   = 256
	replayLast   = 500
	maxValue     = 2048
	maxAttrs     = 32
	redactedText = "[redacted]"
	sensitiveKey = `(?i)token|secret|passw(or)?d|authorization|api[-_]?key|private[-_]?key|bearer|cookie|credential|pairing`
)

func New() Ring {
	return &ringBuffer{
		records:     make([]Record, capacity),
		subscribers: make(map[*subscriber]struct{}),
		redact:      regexp.MustCompile(sensitiveKey),
	}
}

var _ slog.Handler = (*handler)(nil)
