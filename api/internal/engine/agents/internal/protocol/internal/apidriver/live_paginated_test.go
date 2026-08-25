package apidriver

// THROWAWAY diagnostic test — reproduces historyMode:"paginated" by pointing
// thread/start's cwd at a directory codex has already run many threads
// against (confirmed live in the dev daemon log to trigger paginated mode),
// then captures a real thread/read(includeTurns:true) response so the
// pull-based turn_stop fallback can be shaped correctly. Deleted after use.
//
// Run manually: CROWBAR_LIVE_CODEX_TEST=1 go test ./internal/engine/agents/internal/protocol/internal/apidriver/ -run TestLivePaginatedThreadRead -v

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

func TestLivePaginatedThreadRead(t *testing.T) {
	if os.Getenv("CROWBAR_LIVE_CODEX_TEST") == "" {
		t.Skip("set CROWBAR_LIVE_CODEX_TEST=1 to run against a real codex app-server")
	}

	busyCwd := os.Getenv("CROWBAR_LIVE_CODEX_CWD")
	if busyCwd == "" {
		t.Skip("set CROWBAR_LIVE_CODEX_CWD to a directory codex has run many threads against")
	}

	sock := filepath.Join(os.TempDir(), "cbdiag-paginated.sock")
	_ = os.Remove(sock)

	codexPath := os.Getenv("CROWBAR_CODEX_BIN")
	if codexPath == "" {
		home, _ := os.UserHomeDir()
		codexPath = filepath.Join(home, ".local", "bin", "codex")
	}

	cmd := exec.Command(codexPath, "app-server", "--listen", "unix://"+sock)
	cmd.Dir = busyCwd
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start codex app-server: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

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

	if _, err := conn.Call(ctx, "initialize", map[string]any{
		"clientInfo": map[string]string{"name": "crowbar-diag", "title": "Crowbar Diag", "version": "0.0.0"},
	}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := conn.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("initialized: %v", err)
	}

	startResult, err := conn.Call(ctx, "thread/start", map[string]any{
		"cwd":            busyCwd,
		"sandbox":        "workspace-write",
		"approvalPolicy": "on-request",
	})
	if err != nil {
		t.Fatalf("thread/start: %v", err)
	}
	var started struct {
		Thread struct {
			ID          string `json:"id"`
			HistoryMode string `json:"historyMode"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(startResult, &started); err != nil {
		t.Fatalf("parse thread/start response: %v", err)
	}
	t.Logf("thread id: %s historyMode: %s", started.Thread.ID, started.Thread.HistoryMode)

	if _, err := conn.Call(ctx, "turn/start", map[string]any{
		"threadId": started.Thread.ID,
		"input":    []map[string]string{{"type": "text", "text": "Reply with exactly the single word PONG3 and nothing else."}},
	}); err != nil {
		t.Fatalf("turn/start: %v", err)
	}

	// Drain frames until we see thread/status/changed go idle (or turn/completed,
	// if this cwd happens to be legacy mode this run).
	sawIdle := false
	sawTurnCompleted := false
	for !sawIdle && !sawTurnCompleted {
		select {
		case frame, ok := <-conn.Frames():
			if !ok {
				t.Fatalf("frames closed before idle/turn-completed")
			}
			t.Logf("frame: method=%s params=%s", frame.Method, truncate(string(frame.Params), 300))
			if frame.Method == "thread/status/changed" {
				var p struct {
					Status struct {
						Type string `json:"type"`
					} `json:"status"`
				}
				_ = json.Unmarshal(frame.Params, &p)
				if p.Status.Type == "idle" {
					sawIdle = true
				}
			}
			if frame.Method == "turn/completed" {
				sawTurnCompleted = true
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for idle/turn-completed")
		}
	}

	if sawTurnCompleted {
		t.Logf("this cwd ran in LEGACY mode this time — turn/completed carried the content natively, no thread/read fallback needed")
		return
	}

	// Now the pull-based fallback: fetch the thread WITH its turns.
	readResult, err := conn.Call(ctx, "thread/read", map[string]any{
		"threadId":     started.Thread.ID,
		"includeTurns": true,
	})
	if err != nil {
		t.Fatalf("thread/read: %v", err)
	}
	t.Logf("thread/read response: %s", truncate(string(readResult), 4000))
}
