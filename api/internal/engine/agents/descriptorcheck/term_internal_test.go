//go:build unix

package descriptorcheck

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerm_AnExitIsReportedWithTheOutputDrained(t *testing.T) {
	script := `i=0; while [ $i -lt 2000 ]; do echo "line $i of boot noise"; i=$((i+1)); done; echo "config is corrupt"; exit 3`
	term, err := startTerm(context.Background(), []string{"/bin/sh", "-c", script}, nil, t.TempDir())
	require.NoError(t, err)
	defer term.close()

	ended, code := term.exited(10 * time.Second)

	shown := term.text()
	require.True(t, ended)
	assert.Equal(t, 3, code)
	assert.True(t, strings.HasSuffix(strings.TrimSpace(shown), "config is corrupt"),
		"the exit is reported before the CLI's last output was read: %q", tail(shown, 80))
}
