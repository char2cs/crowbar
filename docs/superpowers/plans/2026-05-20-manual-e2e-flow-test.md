# Manual E2E Flow Test Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `//go:build manual` Go test that spins up Crowbar in-process with a real `claude` subprocess and drives a task through all five states of the feature-development flow.

**Architecture:** Three steps — (1) widen the kit build tag from `integration` to `integration || manual` so kit helpers are available to manual tests, (2) add `kit.NewRealEnv` which pre-creates a `ChatHub` and passes it to `engine.New` so the real ACP subprocess runtime is used, and (3) the test itself does the full 5-state walk with 3-minute per-state timeouts.

**Tech Stack:** Go test framework (`testing`), existing `kit` helpers, `hub.NewChatHub()`, `engine.New`, `app.WithChatHub`

---

## File Map

| Action | Path | Purpose |
|--------|------|---------|
| Modify (tag only) | `api/tests/kit/suite.go` | `integration` → `integration \|\| manual` |
| Modify (tag only) | `api/tests/kit/env.go` | same |
| Modify (tag only) | `api/tests/kit/fixtures.go` | same |
| Modify (tag only) | `api/tests/kit/client.go` | same |
| Modify (tag only) | `api/tests/kit/ws_client.go` | same |
| Modify (tag only) | `api/tests/kit/mcp_client.go` | same |
| Modify (tag only) | `api/tests/kit/git.go` | same |
| Modify (tag only) | `api/tests/kit/agent_stub.go` | same |
| Create | `api/tests/kit/real_env.go` | `NewRealEnv(t)` — same `*Env` but wired with real agent |
| Create | `api/tests/manual/e2e_flow_test.go` | The manual test |

---

### Task 1: Widen kit build tags

The kit files currently have `//go:build integration`. A `//go:build manual` test can't import them. Widen each to `//go:build integration || manual`.

**Files:** `api/tests/kit/suite.go`, `env.go`, `fixtures.go`, `client.go`, `ws_client.go`, `mcp_client.go`, `git.go`, `agent_stub.go`

- [ ] **Step 1: Update all 8 kit file build tags**

```bash
cd api
for f in tests/kit/suite.go tests/kit/env.go tests/kit/fixtures.go tests/kit/client.go tests/kit/ws_client.go tests/kit/mcp_client.go tests/kit/git.go tests/kit/agent_stub.go; do
  sed -i '' 's|//go:build integration$|//go:build integration || manual|' "$f"
done
```

- [ ] **Step 2: Verify the tag change compiled correctly for both tags**

```bash
go build -tags=integration ./tests/kit/... && go build -tags=manual ./tests/kit/...
```

Expected: no output for both (clean compile).

- [ ] **Step 3: Confirm integration tests still pass**

```bash
go test -tags=integration ./tests/integration/... 2>&1 | grep -E "ok|FAIL"
```

Expected: all lines show `ok`, no `FAIL`.

- [ ] **Step 4: Commit**

```bash
git add api/tests/kit/
git commit -m "test: widen kit build tag to integration || manual"
```

---

### Task 2: `kit.NewRealEnv` — real-agent Env wiring

**Files:**
- Create: `api/tests/kit/real_env.go`

The existing `kit.NewEnv` passes `nil` as hub to `engine.New`, which disables the real runtime. `NewRealEnv` pre-creates a `hub.ChatHub` and shares it between `engine.New` and `app.New(…, app.WithChatHub(chatHub))` — mirroring production wiring.

- [ ] **Step 1: Create `api/tests/kit/real_env.go`**

```go
//go:build manual

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

	ctx, cancel := context.WithCancel(context.Background())

	// Pre-create the chat hub so it can be shared between engine and app.
	chatHub := appHub.NewChatHub()

	engines, err := engine.New(ctx, chatHub, engine.WithHomeDir(homeDir))
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

	t.Cleanup(func() { closeFn() })

	return &Env{
		Client:    client,
		WSClient:  wsClient,
		MCPClient: mcpClient,
		Stub:      nil, // no stub — real agent runtime
		close:     closeFn,
	}
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd api && go build -tags=manual ./tests/kit/...
```

Expected: no output (clean compile).

- [ ] **Step 3: Commit**

```bash
git add api/tests/kit/real_env.go
git commit -m "test: add NewRealEnv kit helper for manual e2e tests"
```

---

### Task 3: Manual e2e test — full 5-state flow walk

**Files:**
- Create: `api/tests/manual/e2e_flow_test.go`

- [ ] **Step 1: Create `api/tests/manual/e2e_flow_test.go`**

```go
//go:build manual

package manual

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullFlowWithRealAgent drives a task through all five states of the
// feature-development flow using a real claude subprocess.
//
// Run with:
//
//	go test -v -tags=manual -timeout=30m -run TestFullFlowWithRealAgent ./tests/manual/
func TestFullFlowWithRealAgent(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude binary not found in PATH; skipping manual e2e test")
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set; skipping manual e2e test")
	}

	env := kit.NewRealEnv(t)

	gitRepo := kit.NewGitRepo(t)

	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := env.Client.CreateTask(repo.ID, kit.CreateTaskRequest{
		Title:      "Add a hello.go file with a Hello() function and a test for it",
		BranchName: "feature/hello",
		BaseBranch: "main",
		FlowPath:   "",
	})
	t.Logf("task created: id=%s state=%s worktree=%s", task.ID, task.CurrentState, task.WorktreePath)

	taskCh, closeTask := env.WSClient.SubscribeTasks(task.ID)
	defer closeTask()

	// Drain the initial sync frame (server sends current state on connect).
	waitManualTaskState(t, taskCh, task.CurrentState, 10*time.Second)

	const stateTimeout = 3 * time.Minute

	t.Log("waiting for agent to complete brainstorming…")
	waitManualTaskState(t, taskCh, "spec", stateTimeout)
	t.Log("✓ spec")

	t.Log("waiting for agent to complete spec…")
	waitManualTaskState(t, taskCh, "implementation", stateTimeout)
	t.Log("✓ implementation")

	t.Log("waiting for agent to complete implementation…")
	waitManualTaskState(t, taskCh, "ai_review", stateTimeout)
	t.Log("✓ ai_review")

	t.Log("waiting for agent to complete ai_review…")
	waitManualTaskState(t, taskCh, "human_review", stateTimeout)
	t.Log("✓ human_review")

	t.Log("force-transitioning to complete…")
	final := env.Client.ForceTransition(task.ID, "complete")
	require.Equal(t, "complete", final.CurrentState)
	t.Log("✓ complete")

	// hello.go must exist in the worktree after implementation.
	helloPath := filepath.Join(task.WorktreePath, "hello.go")
	_, statErr := os.Stat(helloPath)
	assert.NoError(t, statErr, "hello.go should exist in worktree at %s", helloPath)

	// At least 4 agent runs (brainstorming, spec, implementation, ai_review).
	runs := env.Client.ListAgentRuns(task.ID)
	assert.GreaterOrEqual(t, len(runs), 4, "expected ≥4 agent runs, got %d", len(runs))

	// No run should be failed.
	for _, r := range runs {
		assert.NotEqual(t, domain.AgentRunStatusFailed, r.Status,
			"agent run %s should not be failed", r.ID)
	}
}

// waitManualTaskState blocks until a Task with CurrentState == want arrives on ch
// or the timeout elapses.
func waitManualTaskState(
	t *testing.T,
	ch <-chan domain.Task,
	want string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("timeout waiting for task state %q", want)
			return
		}
		select {
		case <-time.After(remaining):
			t.Fatalf("timeout waiting for task state %q", want)
			return
		case task, ok := <-ch:
			if !ok {
				t.Fatalf("WS channel closed before state %q", want)
				return
			}
			t.Logf("  state update: %s", task.CurrentState)
			if task.CurrentState == want {
				return
			}
		}
	}
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd api && go build -tags=manual ./tests/manual/...
```

Expected: no output (clean compile).

- [ ] **Step 3: Verify skip behaviour (run without credentials)**

```bash
cd api && ANTHROPIC_API_KEY="" go test -v -tags=manual -run TestFullFlowWithRealAgent ./tests/manual/
```

Expected output contains:
```
--- SKIP: TestFullFlowWithRealAgent
```

- [ ] **Step 4: Commit**

```bash
git add api/tests/manual/e2e_flow_test.go
git commit -m "test: manual e2e full-flow test with real claude agent"
```

---

### Task 4: Smoke-run

- [ ] **Step 1: Run the test**

```bash
cd api && go test -v -tags=manual -timeout=30m -run TestFullFlowWithRealAgent ./tests/manual/ 2>&1 | tee /tmp/e2e-run.log
```

Expected: each state transition logged; final line is `PASS`.

- [ ] **Step 2: On failure — read the log**

```bash
cat /tmp/e2e-run.log
```

The test logs every task state update from WebSocket. The last logged state before a timeout line identifies exactly where the flow broke.

- [ ] **Step 3: Commit any fixes**

Fix root causes identified in the log, then:

```bash
git add -p
git commit -m "fix: <description>"
```
