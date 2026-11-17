package handlers

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/app/browserbridge"
)

// NewWithKeepalive builds the handlers with their own ping and pong timing, so a test need not
// wait out the production intervals.
func NewWithKeepalive(
	bridge *browserbridge.Bridge,
	ping, pong time.Duration,
) *Handlers {
	return &Handlers{bridge: bridge, ping: ping, pong: pong}
}
