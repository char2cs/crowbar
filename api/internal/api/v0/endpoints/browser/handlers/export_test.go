package handlers

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/app/browserbridge"
)

func NewWithKeepalive(
	bridge *browserbridge.Bridge,
	ping, pong time.Duration,
) *Handlers {
	return &Handlers{bridge: bridge, ping: ping, pong: pong}
}
