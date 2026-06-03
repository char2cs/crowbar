//go:build integration || manual

package kit

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/api"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/stretchr/testify/require"
)

// Env is a fully wired in-process test server. All HTTP and WebSocket traffic
// flows over a Unix socket so tests are isolated from real network ports.
type Env struct {
	Client    *Client
	WSClient  *WSClient
	MCPClient *MCPClient
	Stub      *AgentStub
	// HomeDir is the server home directory. Agent run artifacts are written
	// under HomeDir/runs/<runID>/. Empty for non-real envs.
	HomeDir string
	close   func()
}

// Close shuts down the in-process server and releases all resources.
// It is safe to call multiple times; subsequent calls are no-ops.
func (e *Env) Close() {
	if e.close != nil {
		e.close()
		e.close = nil
	}
}

// NewEnv creates an Env with a fresh temp directory as its home.
func NewEnv(
	t *testing.T,
) *Env {
	return NewEnvWithHome(t, t.TempDir())
}

// NewEnvWithHome creates an Env using an explicit home directory, useful for
// crash-recovery tests that need two successive envs pointing at the same
// underlying SQLite data.
func NewEnvWithHome(
	t *testing.T,
	homeDir string,
) *Env {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	stub := NewAgentStub()

	engines, err := engine.New(ctx, nil, engine.WithHomeDir(homeDir), engine.WithAgentRuntime(stub))
	require.NoError(t, err)

	adapters, err := adapter.New(adapter.WithHomeDir(homeDir))
	require.NoError(t, err)

	appContainer, err := app.New(ctx, engines, adapters, app.WithHomeDir(homeDir))
	require.NoError(t, err)

	// Wire the chat hub into the stub so StreamChunks delivers frames to WS clients.
	stub.SetChatHub(appContainer.ChatHub)

	apiContainer, err := api.New(appContainer, nil)
	require.NoError(t, err)

	// macOS enforces UNIX_PATH_MAX = 104 chars. Use a short-named temp dir so
	// the socket path stays well under the limit regardless of test name length.
	sockDir, err := os.MkdirTemp("", "cb")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "crowbar.sock")

	ln, err := net.Listen("unix", sockPath)
	require.NoError(t, err)

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_ = apiContainer.Run(ln)
	}()

	client := newClient(t, sockPath)
	wsClient := newWSClient(t, sockPath)
	mcpClient := newMCPClient(sockPath)

	closeFn := func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = apiContainer.Shutdown(shutdownCtx)
		<-runDone
		cancel()
		_ = adapters.Close()
	}

	t.Cleanup(func() {
		closeFn()
	})

	return &Env{
		Client:    client,
		WSClient:  wsClient,
		MCPClient: mcpClient,
		Stub:      stub,
		close:     closeFn,
	}
}
