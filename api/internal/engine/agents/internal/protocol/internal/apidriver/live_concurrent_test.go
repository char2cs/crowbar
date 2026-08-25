package apidriver

// THROWAWAY diagnostic test — does a SECOND client connecting to the same
// running codex app-server (simulating the `attach` TUI reconnecting to the
// same thread `serve` already owns) degrade what the FIRST connection (our
// own API driver) receives? Deleted after use — mixed-transport investigation.
//
// Run manually: CROWBAR_LIVE_CODEX_TEST=1 go test ./internal/engine/agents/internal/protocol/internal/apidriver/ -run TestLiveConcurrentAttach -v

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

func TestLiveConcurrentAttach(t *testing.T) {
	if os.Getenv("CROWBAR_LIVE_CODEX_TEST") == "" {
		t.Skip("set CROWBAR_LIVE_CODEX_TEST=1 to run against a real codex app-server")
	}

	scratch := t.TempDir()
	sock := filepath.Join(os.TempDir(), "cbdiag-concurrent.sock")
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

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Connection A: our own API driver, dialed and started FIRST — the same
	// role apidriver.Start plays in production.
	connA, err := wsrpc.Dial(ctx, sock)
	if err != nil {
		t.Fatalf("dial A: %v", err)
	}
	defer func() { _ = connA.Close() }()
	if _, err := connA.Call(ctx, "initialize", map[string]any{
		"clientInfo": map[string]string{"name": "crowbar-diag-a", "title": "Crowbar Diag A", "version": "0.0.0"},
	}); err != nil {
		t.Fatalf("initialize A: %v", err)
	}
	if err := connA.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("initialized A: %v", err)
	}

	startResult, err := connA.Call(ctx, "thread/start", map[string]any{
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

	// Connection B: a SECOND client dialing the SAME socket/thread, mimicking
	// what `attach` does when a chat's terminal view is opened on top of an
	// already-running api transport connection.
	connB, err := wsrpc.Dial(ctx, sock)
	if err != nil {
		t.Fatalf("dial B: %v", err)
	}
	defer func() { _ = connB.Close() }()
	if _, err := connB.Call(ctx, "initialize", map[string]any{
		"clientInfo": map[string]string{"name": "crowbar-diag-b-attach", "title": "Crowbar Diag B (attach)", "version": "0.0.0"},
	}); err != nil {
		t.Fatalf("initialize B: %v", err)
	}
	if err := connB.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("initialized B: %v", err)
	}
	// Mirror what a real `--remote unix://socket` attach does: it points at
	// the SAME already-existing thread rather than starting a new one. Try
	// thread/resume; if unavailable, thread/read is enough to "attach" a
	// second observer to the same thread for this test's purposes.
	if _, err := connB.Call(ctx, "thread/resume", map[string]any{"threadId": started.Thread.ID}); err != nil {
		t.Logf("thread/resume on B failed (may not exist): %v — trying thread/read instead", err)
		if _, err := connB.Call(ctx, "thread/read", map[string]any{"threadId": started.Thread.ID}); err != nil {
			t.Logf("thread/read on B also failed: %v", err)
		}
	}

	// Now drain connA's frames in the background while we send a SECOND
	// prompt on connA, WITH connB attached — this is the exact production
	// scenario the user described.
	var aMethods []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case frame, ok := <-connA.Frames():
				if !ok {
					return
				}
				aMethods = append(aMethods, frame.Method)
				t.Logf("[A] method=%s params=%s", frame.Method, truncate(string(frame.Params), 200))
				if frame.Method == "turn/completed" {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	if _, err := connA.Call(ctx, "turn/start", map[string]any{
		"threadId": started.Thread.ID,
		"input":    []map[string]string{{"type": "text", "text": "Reply with exactly the single word PONG2 and nothing else."}},
	}); err != nil {
		t.Fatalf("turn/start (with B attached): %v", err)
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for connA's turn to complete with B attached")
	}

	t.Logf("connA methods seen WITH connB attached: %v", aMethods)

	sawItemStarted := false
	sawTurnCompleted := false
	for _, m := range aMethods {
		if m == "item/started" {
			sawItemStarted = true
		}
		if m == "turn/completed" {
			sawTurnCompleted = true
		}
	}
	if !sawItemStarted || !sawTurnCompleted {
		t.Errorf("REPRODUCED: with a second client attached to the same thread, connA (our api driver) did NOT see item/started=%v turn/completed=%v — degraded to coarse events only", sawItemStarted, sawTurnCompleted)
	} else {
		t.Logf("NOT reproduced this way: connA still saw full item-level events even with connB attached")
	}
}
