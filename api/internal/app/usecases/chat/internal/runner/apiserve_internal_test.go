package runner

import (
	"bufio"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForkServeProcess_ItsStdioIsTheConnection(t *testing.T) {
	serve, stdio, err := forkServeProcess([]string{"sh", "-c", `read line; echo "got $line"`})
	require.NoError(t, err)
	t.Cleanup(func() { _ = stdio.Close() })
	defer serve.kill()

	_, err = stdio.In.Write([]byte("hello\n"))
	require.NoError(t, err)
	line, err := bufio.NewReader(stdio.Out).ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "got hello\n", line)
}

func TestForkServeProcess_EmptyArgvIsAnError(t *testing.T) {
	_, _, err := forkServeProcess(nil)
	assert.Error(t, err)
}
