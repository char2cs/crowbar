package agents_test

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestStartServe_SpeaksOnTheChildsStdinAndStdout(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "sh", "-c", `read line; echo "got $line"`)
	stdio, err := agents.StartServe(cmd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stdio.Close() })

	_, err = stdio.In.Write([]byte("hello\n"))
	require.NoError(t, err)
	line, err := bufio.NewReader(stdio.Out).ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "got hello\n", line)
	require.NoError(t, cmd.Wait())
}

// Serve's exit is EOF on its stdout only if nothing else holds the write
// end; a copy kept here would leave a dead serve looking alive.
func TestStartServe_AServeThatExitsIsEOF(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "sh", "-c", "exit 0")
	stdio, err := agents.StartServe(cmd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stdio.Close() })
	require.NoError(t, cmd.Wait())

	out, ok := stdio.Out.(*os.File)
	require.True(t, ok)
	require.NoError(t, out.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err = out.Read(make([]byte, 1))
	assert.ErrorIs(t, err, io.EOF)
}

func TestStartServe_AServeThatCannotStartIsAnError(t *testing.T) {
	_, err := agents.StartServe(exec.CommandContext(t.Context(), "/nonexistent/serve"))

	require.Error(t, err)
}
