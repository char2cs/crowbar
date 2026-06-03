//go:build manual

package kit

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/api"
	"github.com/char2cs/crowbar/api/internal/app"
	appHub "github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/stretchr/testify/require"
)

// NewRealEnv creates an Env that uses the real ACP subprocess agent runtime
// instead of AgentStub. Use only in manual tests; requires the claude binary
// in PATH and ANTHROPIC_API_KEY set.
func NewRealEnv(t *testing.T) *Env {
	t.Helper()
	homeDir := t.TempDir()

	// The engine runs agent subprocesses with CLAUDE_CONFIG_DIR pointed at
	// homeDir/agent-claude so they do not inherit the host user's skills,
	// settings, or memory. A fresh config dir, however, has no OAuth token, so
	// seed one from the host's Claude Code credentials. Without this the agent
	// subprocess fails every prompt with "Authentication required".
	seedAgentCredentials(t, filepath.Join(homeDir, "agent-claude"))

	ctx, cancel := context.WithCancel(context.Background())

	// Allocate a random TCP port for the MCP server. The agent subprocess is a
	// separate OS process and cannot use the kit's Unix-socket dialer, so it
	// needs a real TCP URL to reach the MCP endpoint.
	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	mcpURL := fmt.Sprintf("http://%s/mcp", tcpLn.Addr().String())

	// Pre-create the chat hub so it can be shared between engine and app.
	chatHub := appHub.NewChatHub()

	engines, err := engine.New(ctx, chatHub,
		engine.WithHomeDir(homeDir),
		engine.WithMCPURL(mcpURL),
	)
	require.NoError(t, err)

	adapters, err := adapter.New(adapter.WithHomeDir(homeDir))
	require.NoError(t, err)

	appContainer, err := app.New(ctx, engines, adapters,
		app.WithHomeDir(homeDir),
		app.WithChatHub(chatHub),
	)
	require.NoError(t, err)

	apiContainer, err := api.New(appContainer, nil)
	require.NoError(t, err)

	// Unix socket for test kit clients.
	sockDir, err := os.MkdirTemp("", "cb")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "crowbar.sock")
	unixLn, err := net.Listen("unix", sockPath)
	require.NoError(t, err)

	// Serve on both listeners simultaneously. http.Server.Serve can be called
	// multiple times; Shutdown stops all of them.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = apiContainer.Run(unixLn)
	}()
	go func() {
		defer wg.Done()
		_ = apiContainer.Run(tcpLn)
	}()

	runDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(runDone)
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

	t.Cleanup(func() { closeFn() })

	return &Env{
		Client:    client,
		WSClient:  wsClient,
		MCPClient: mcpClient,
		Stub:      nil,
		HomeDir:   homeDir,
		close:     closeFn,
	}
}

// seedAgentCredentials copies the host's Claude Code OAuth credentials into
// cfgDir/.credentials.json so an agent subprocess running with an isolated
// CLAUDE_CONFIG_DIR can still authenticate.
//
// When ANTHROPIC_API_KEY is set the agent authenticates via the API key and no
// seeding is needed. Otherwise the credentials are read from the host's
// ~/.claude/.credentials.json, or — on macOS — from the login keychain.
func seedAgentCredentials(t *testing.T, cfgDir string) {
	t.Helper()
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return
	}

	creds := hostCredentials(t)
	if len(creds) == 0 {
		t.Log("no host Claude credentials found to seed; agent auth may fail")
		return
	}

	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("seedAgentCredentials: mkdir %s: %v", cfgDir, err)
	}
	dst := filepath.Join(cfgDir, ".credentials.json")
	if err := os.WriteFile(dst, creds, 0o600); err != nil {
		t.Fatalf("seedAgentCredentials: write %s: %v", dst, err)
	}
}

// hostCredentials returns the host's Claude Code credentials JSON, or nil if
// none can be located.
func hostCredentials(t *testing.T) []byte {
	t.Helper()

	home, err := os.UserHomeDir()
	if err == nil {
		path := filepath.Join(home, ".claude", ".credentials.json")
		if data, readErr := os.ReadFile(path); readErr == nil && len(data) > 0 {
			return data
		}
	}

	if runtime.GOOS == "darwin" {
		out, kcErr := exec.Command("security", "find-generic-password",
			"-s", "Claude Code-credentials", "-w").Output()
		if kcErr == nil && len(out) > 0 {
			// security appends a trailing newline; trim it.
			for len(out) > 0 && (out[len(out)-1] == '\n' || out[len(out)-1] == '\r') {
				out = out[:len(out)-1]
			}
			return out
		}
	}

	return nil
}
