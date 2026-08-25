package apidriver

// THROWAWAY diagnostic test — dials a REAL codex app-server directly (no
// attach TUI involved at all) to determine whether item-level notifications
// (item/started, turn/completed, item/agentMessage/delta) require the
// `capabilities.experimentalApi` opt-in on `initialize`, per codex 0.149.1's
// InitializeCapabilities schema. Deleted after use — see the mixed-transport
// investigation in the current session.
//
// Run manually: CROWBAR_LIVE_CODEX_TEST=1 go test ./internal/engine/agents/internal/protocol/internal/apidriver/ -run TestLiveCapabilityNegotiation -v

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/wsrpc"
)

func TestLiveCapabilityNegotiation(t *testing.T) {
	if os.Getenv("CROWBAR_LIVE_CODEX_TEST") == "" {
		t.Skip("set CROWBAR_LIVE_CODEX_TEST=1 to run against a real codex app-server")
	}

	for _, experimental := range []bool{false, true} {
		experimental := experimental
		t.Run(map[bool]string{false: "no_experimentalApi", true: "with_experimentalApi"}[experimental], func(t *testing.T) {
			runOneNegotiation(t, experimental)
		})
	}
}

func runOneNegotiation(t *testing.T, experimental bool) {
	scratch := t.TempDir()
	tag := "a"
	if experimental {
		tag = "b"
	}
	sock := filepath.Join(os.TempDir(), "cbdiag-"+tag+".sock")
	_ = os.Remove(sock)

	codexPath := os.Getenv("CROWBAR_CODEX_BIN")
	if codexPath == "" {
		home, _ := os.UserHomeDir()
		codexPath = filepath.Join(home, ".local", "bin", "codex")
	}

	cmd := exec.Command(codexPath, "app-server", "--listen", "unix://"+sock)
	cmd.Dir = scratch
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start codex app-server: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

	// Poll for the socket file rather than sleeping a fixed duration.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("codex app-server never created %s", sock)
		}
		time.Sleep(50 * time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	conn, err := wsrpc.Dial(ctx, sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	initParams := map[string]any{
		"clientInfo": map[string]string{"name": "crowbar-diag", "title": "Crowbar Diag", "version": "0.0.0"},
	}
	if experimental {
		initParams["capabilities"] = map[string]any{"experimentalApi": true}
	}
	if _, err := conn.Call(ctx, "initialize", initParams); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := conn.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("initialized: %v", err)
	}

	startResult, err := conn.Call(ctx, "thread/start", map[string]any{
		"cwd":            scratch,
		"sandbox":        "workspace-write",
		"approvalPolicy": "on-request",
	})
	if err != nil {
		t.Fatalf("thread/start: %v", err)
	}
	var started struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(startResult, &started); err != nil {
		t.Fatalf("parse thread/start response: %v", err)
	}
	t.Logf("thread id: %s", started.Thread.ID)

	if _, err := conn.Call(ctx, "turn/start", map[string]any{
		"threadId": started.Thread.ID,
		"input":    []map[string]string{{"type": "text", "text": "Reply with exactly the single word PONG and nothing else."}},
	}); err != nil {
		t.Fatalf("turn/start: %v", err)
	}

	var seenMethods []string
	sawTurnCompleted := false
	for !sawTurnCompleted {
		select {
		case frame, ok := <-conn.Frames():
			if !ok {
				t.Logf("frames channel closed; methods seen: %v", seenMethods)
				return
			}
			seenMethods = append(seenMethods, frame.Method)
			t.Logf("frame: method=%s params=%s", frame.Method, truncate(string(frame.Params), 300))
			if frame.Method == "turn/completed" {
				sawTurnCompleted = true
			}
		case <-ctx.Done():
			t.Logf("timed out; methods seen so far: %v", seenMethods)
			return
		}
	}
	t.Logf("ALL methods seen (experimentalApi=%v): %v", experimental, seenMethods)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
