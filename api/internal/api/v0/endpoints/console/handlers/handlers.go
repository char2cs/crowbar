package handlers

import "github.com/char2cs/crowbar/api/internal/core/logring"

type Handlers struct {
	logs logring.Ring
}

func New(
	logs logring.Ring,
) *Handlers {
	return &Handlers{logs: logs}
}
