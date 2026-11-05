package logring

import (
	"log/slog"
	"time"
)

// Record is one captured log record in the JSON shape the console streams. Seq starts
// at 1 and is never reused, so a client resumes from the last one it saw; Fields are
// dotted keys, redacted, cut at 2 KiB and capped at 32; Type is always "log".
type Record struct {
	Type      string         `json:"type"`
	Seq       uint64         `json:"seq"`
	Time      time.Time      `json:"time"`
	Level     string         `json:"level"`
	Component string         `json:"component"`
	Msg       string         `json:"msg"`
	Fields    map[string]any `json:"fields"`

	level slog.Level
}
