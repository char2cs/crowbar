package handlers

import "time"

func NewHandlerWithProbeTimeout(d time.Duration) *Handler {
	return &Handler{probeTimeout: d}
}
