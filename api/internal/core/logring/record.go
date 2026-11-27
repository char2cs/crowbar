package logring

import (
	"log/slog"
	"time"
)

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
