package adapter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
)

func TestChatLedgerDB_IsOneWriterConnectionUnderTheStore(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)

	sqlDB, err := c.ChatLedgerDB().DB()
	require.NoError(t, err)
	assert.Equal(t, 1, sqlDB.Stats().MaxOpenConnections, "the ledger has a single writer")
	_, statErr := os.Stat(filepath.Join(home, "state", "store", "chat_ledger.db"))
	require.NoError(t, statErr)

	require.NoError(t, c.Close())
	assert.Error(t, sqlDB.Ping(), "Close closes the ledger database")
}
