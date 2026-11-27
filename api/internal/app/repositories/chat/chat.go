package chat

import (
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

var ErrNotFound = problem.New(problem.NotFound, "agentchat: not found")
