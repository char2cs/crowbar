package handlers

import "time"

// NewHandlerWithProbeTimeout is NewHandler with each probe bounded by d instead.
func NewHandlerWithProbeTimeout(d time.Duration) *Handler {
	return &Handler{probeTimeout: d}
}
