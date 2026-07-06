package ledger_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/ledger"
)

func indexOf(s, substr string) int {
	return strings.Index(s, substr)
}

func TestLedger_AppendThenReadAllOrdered(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)
	at := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)

	f1, err := l.Append("claude", at, []byte("FIRST"))
	require.NoError(t, err)
	require.Contains(t, f1, "claude")
	_, err = l.Append("codex", at.Add(time.Minute), []byte("SECOND"))
	require.NoError(t, err)

	all, err := l.ReadAll()
	require.NoError(t, err)
	require.Less(t, indexOf(string(all), "FIRST"), indexOf(string(all), "SECOND"))
	require.Contains(t, string(all), "claude")
	require.Contains(t, string(all), "codex")
}
