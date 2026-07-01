package domain_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestTerminalSession_TableName(t *testing.T) {
	assert.Equal(t, "terminal_sessions", domain.TerminalSession{}.TableName())
}
