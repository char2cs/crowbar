# Manual End-to-End Flow Test Design

**Date:** 2026-05-20  
**Scope:** A `//go:build manual` Go test that fires up Crowbar in-process with a real AI agent and drives a task through all five states of the feature-development flow.

---

## Goal

Verify the full agent dispatch loop works end-to-end: real HTTP API, real SQLite event stores, real dispatcher, real MCP server, and a real `claude` subprocess communicating over ACP. The stub-based integration tests already validate all the plumbing; this test validates that the AI actually drives the flow.

---

## File Location

```
api/tests/manual/e2e_flow_test.go
```

Build tag `//go:build manual` — never compiled or run by `go test ./...` or CI.

Run manually with:
```bash
go test -v -tags=manual -timeout=30m -run TestFullFlowWithRealAgent ./api/tests/manual/
```

---

## Skip Conditions

The test calls `t.Skip` (not `t.Fatal`) if either precondition is absent:

1. `claude` binary not found in `$PATH` — checked via `exec.LookPath("claude")`
2. `ANTHROPIC_API_KEY` environment variable not set

This allows running `go test -tags=manual ./tests/manual/...` on any machine without crashing the run.

---

## Server Wiring: `kit.NewRealEnv`

The existing `kit.NewEnv` passes `nil` as the hub to `engine.New`, which forces the stub. A new `kit.NewRealEnv(t)` function mirrors production wiring:

1. Create a `hub.New()` chat hub independently
2. Pass it to `engine.New(ctx, chatHub, engine.WithHomeDir(homeDir))` — no `WithAgentRuntime`
3. Pass engines + adapters to `app.New(ctx, engines, adapters, app.WithHomeDir(homeDir))`
4. Wire the resulting `appContainer.ChatHub` through normally

`NewRealEnv` lives in a new file `api/tests/kit/real_env.go` also tagged `//go:build manual`.

---

## Scratch Repository

The test creates a minimal git repo in `t.TempDir()` using `kit.NewGitRepo`:

```
initial commit (empty) — establishes HEAD and main branch
```

No files are pre-committed. The agent will create `hello.go` and its test during the `implementation` state.

---

## Test Scenario

**Task description:** `"Add a hello.go file with a Hello() function and a test for it"`  
**Branch name:** `feature/hello`

### State Walk

| # | State | Agent behaviour | Test waits for |
|---|---|---|---|
| 1 | `brainstorming` | reads repo, calls `crowbar_signal(user_approved, …)` | WS task event: `CurrentState == "spec"` |
| 2 | `spec` | writes spec in chat, calls `crowbar_signal(user_approved, …)` | WS task event: `CurrentState == "implementation"` |
| 3 | `implementation` | creates `hello.go` + `hello_test.go`, calls `crowbar_signal(implementation_complete, …)` | WS task event: `CurrentState == "ai_review"` |
| 4 | `ai_review` | reviews code, calls `crowbar_signal(review_passed, …)` | WS task event: `CurrentState == "human_review"` |
| 5 | `human_review` | no agent — **test calls `ForceTransition("complete")`** | HTTP 200, task state `"complete"` |

Per-state timeout: **3 minutes**. Total test timeout passed to `go test -timeout`: **30 minutes**.

### Final Assertions

- `task.CurrentState == "complete"`
- `hello.go` exists under `task.WorktreePath`
- At least 4 agent runs recorded (`GET /tasks/:id/agent-runs`, `len >= 4`)
- No agent run has `Status == "failed"`

---

## Error Handling & Diagnostics

- On any state-wait timeout: `t.Fatalf` with the last observed state and elapsed time
- On HTTP error: print response body before failing
- On test completion (pass or fail): print all chat frames received on WebSocket (for debugging what the agent actually said/did)

---

## Out of Scope

- Asserting the exact content of `hello.go` — the agent's output is non-deterministic
- Running multiple tasks concurrently
- Testing crash recovery with a real agent
- Cost tracking or token limits
