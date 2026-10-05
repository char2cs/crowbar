package v0

import "github.com/char2cs/crowbar/api/internal/core/logring"

// Option configures New.
type Option func(*Container)

// WithLogs serves ring's records on GET /console/logs; a nil ring mounts nothing.
func WithLogs(
	ring logring.Ring,
) Option {
	return func(c *Container) {
		c.logs = ring
	}
}
