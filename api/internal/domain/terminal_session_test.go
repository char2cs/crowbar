package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestTerminalSession_TableName(t *testing.T) {
	assert.Equal(t, "terminal_sessions", domain.TerminalSession{}.TableName())
}
