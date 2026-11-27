package v0

import "github.com/char2cs/crowbar/api/internal/core/logring"

type Option func(*Container)

func WithLogs(
	ring logring.Ring,
) Option {
	return func(c *Container) {
		c.logs = ring
	}
}
