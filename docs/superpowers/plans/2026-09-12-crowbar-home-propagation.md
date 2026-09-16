# CROWBAR_HOME Propagation & Hook-Spool Resilience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every in-PTY callback the daemon spawns for a provider CLI (`crowbar hook`, `crowbar mcp`, `crowbar handoff dump`) must talk to the SAME crowbar instance (home + daemon) that spawned it, even when the provider CLI's own hook/MCP-subprocess mechanism does not forward the daemon process's environment — and a single undeliverable hook event must never block every other event behind it.

**Architecture:** The daemon already resolves the correct `crowbarHome` for a spawn (`spawnPaths.crowbarHome` in `spawnplan.go`) but currently only uses it to locate the `crowbar` binary on disk (`crowbarHookPath`), never to tell that binary which home to operate against once it runs. Fix: thread `crowbarHome` through `TemplateCtx` into the SAME `--project/--repo/--workspace` flag mechanism the hook/mcp/handoff commands already parse (`{scope_flags}` → `bindScopeFlags`), so the callback's own environment is irrelevant — its home is baked into its command line at spawn time, exactly like its scope already is. Separately, fix `hook_spool.go`'s drain loop so one envelope's permanent delivery failure (e.g. it references a since-deleted project) no longer wedges every envelope queued behind it.

**Tech Stack:** Go, cobra, testify.

**Spec:** This plan's own Goal/Architecture sections above — informal findings from a live diagnostic session, not a separate spec document. See the "Global Constraints" section for the exact facts the diagnosis established.

## Global Constraints

- `TemplateCtx.ScopeFlags()` (`api/internal/engine/agents/internal/models/template.go`) is the ONLY place that renders `{scope_flags}`, consumed verbatim inside `claude.yaml`/`codex.yaml`'s hook `command:` templates — any new flag added here reaches the spawned CLI's hook config with no descriptor-file change required (keeps this generic, not provider-specific).
- `bindScopeFlags` (`api/cmd/crowbar/scope.go`) is shared by `hook.go`, `mcp.go`, and `handoff.go` — all three in-PTY callbacks reach the daemon via `ipc.NewClient("unix://...")`, whose socket resolution (`transports.SocketPath`) and the hook spool's file location (`metadata.GetHomePath()`) both fall back to `os.Getenv("CROWBAR_HOME")`, then to `~/.crowbar`, when nothing else says otherwise.
- Every one of these callback processes is a fresh, short-lived OS process invoked once per event/relay-session — never long-running and shared across events — so `os.Setenv` at the very top of a command's `RunE` is race-free.
- The persistent drain loop lives in the DAEMON process (`api/cmd/crowbar/main.go:74`, `go drainHookSpoolLoop(ctx, host)`), ticking every second for the whole life of the daemon — it is the one place a failing envelope gets retried forever today, and the one place safe to hold in-memory per-envelope failure counts.
- Never rename/delete a spooled envelope except via the drain loop's own success/dead-letter paths — a hook CLI's one-shot ack fetch (`runHook`'s call to `drainHookSpoolFor`) must keep behaving exactly as before on the happy path (it still returns early on the FIRST error, since it only cares about its own delivery id and must never delay the vendor CLI).

---

### Task 1: `TemplateCtx.CrowbarHome` + `ScopeFlags()`

**Files:**
- Modify: `api/internal/engine/agents/internal/models/template.go`
- Test: `api/internal/engine/agents/internal/models/template_test.go` (new)

**Interfaces:**
- Produces: `TemplateCtx.CrowbarHome string` field; `ScopeFlags()` now also emits `--home=<value>`; `{crowbar_home}` becomes a valid token in `Replacer()`.

- [ ] **Step 1: Write the failing test**

```go
package models

import "testing"

func TestScopeFlags_IncludesHome(t *testing.T) {
	c := TemplateCtx{ProjectID: "p1", WorkspaceID: "w1", CrowbarHome: "/Users/x/.crowbar"}
	got := c.ScopeFlags()
	want := "--project=p1 --workspace=w1 --home=/Users/x/.crowbar"
	if got != want {
		t.Fatalf("ScopeFlags() = %q, want %q", got, want)
	}
}

func TestScopeFlags_OmitsHomeWhenEmpty(t *testing.T) {
	c := TemplateCtx{ProjectID: "p1", WorkspaceID: "w1"}
	got := c.ScopeFlags()
	want := "--project=p1 --workspace=w1"
	if got != want {
		t.Fatalf("ScopeFlags() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/engine/agents/internal/models/... -run TestScopeFlags -v`
Expected: FAIL — `ScopeFlags()` does not yet emit `--home=`.

- [ ] **Step 3: Write minimal implementation**

In `template.go`, add the field next to the other scope ids and extend `ScopeFlags()`:

```go
	Provider    string
	ProjectID   string
	RepoID      string
	WorkspaceID string

	// CrowbarHome is the crowbar home this spawn was resolved against
	// (spawnPaths.crowbarHome in the runner package). Every in-PTY callback
	// (hook, mcp, handoff) must operate against THIS home, not whatever
	// CROWBAR_HOME the vendor CLI's own hook/subprocess mechanism happens
	// to forward — which is not guaranteed, and silently defaults to the
	// user's real ~/.crowbar when absent. Baking it into the command line
	// here, exactly like project/repo/workspace already are, makes delivery
	// correct regardless of what environment the callback inherits.
	CrowbarHome string
```

```go
func (c TemplateCtx) ScopeFlags() string {
	flags := "--project=" + c.ProjectID + " --workspace=" + c.WorkspaceID
	if c.RepoID != "" {
		flags += " --repo=" + c.RepoID
	}
	if c.CrowbarHome != "" {
		flags += " --home=" + c.CrowbarHome
	}
	return flags
}
```

Add `"{crowbar_home}", c.CrowbarHome,` to the `pairs` slice in `Replacer()`, alongside the other scope tokens.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/engine/agents/internal/models/... -run TestScopeFlags -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/agents/internal/models/template.go api/internal/engine/agents/internal/models/template_test.go
git commit -m "feat(agents): carry crowbar home through ScopeFlags"
```

---

### Task 2: Wire `spawnPaths.crowbarHome` into `renderSpawnContext`

**Files:**
- Modify: `api/internal/app/usecases/chat/internal/runner/spawnplan.go`
- Test: `api/internal/app/usecases/chat/internal/runner/spawnplan_test.go` (new)

**Interfaces:**
- Consumes: `TemplateCtx.CrowbarHome` (Task 1); `spawnContext.crowbarHome` (already exists, `spawn.go:340`).
- Produces: `renderSpawnContext` now populates `TemplateCtx.CrowbarHome` from `in.crowbarHome`.

- [ ] **Step 1: Write the failing test**

```go
package runner

import "testing"

func TestRenderSpawnContext_CarriesCrowbarHome(t *testing.T) {
	rs := &Runners{}
	in := spawnContext{
		crowbarHome: "/tmp/some-home",
		projectID:   "p1",
		workspaceID: "w1",
		runnerID:    "run1",
		providerID:  "claude",
	}
	tctx, ok := rs.renderSpawnContext(in)
	if !ok {
		t.Fatalf("renderSpawnContext returned ok=false")
	}
	if tctx.CrowbarHome != "/tmp/some-home" {
		t.Fatalf("TemplateCtx.CrowbarHome = %q, want %q", tctx.CrowbarHome, "/tmp/some-home")
	}
}
```

(Check `renderSpawnContext`'s real second return value — if it is not a plain `bool` today, match its actual signature; the point of this step is only the `CrowbarHome` assertion.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/app/usecases/chat/internal/runner/... -run TestRenderSpawnContext_CarriesCrowbarHome -v`
Expected: FAIL — `tctx.CrowbarHome` is empty.

- [ ] **Step 3: Write minimal implementation**

In `spawnplan.go`'s `renderSpawnContext`, add one field to the `TemplateCtx{...}` literal:

```go
	tctx := engineagents.TemplateCtx{
		Tmp:         in.tmpDir,
		Cwd:         in.worktree,
		CrowbarHook: rs.crowbarHookPath(in.crowbarHome),
		CrowbarHome: in.crowbarHome,
		Segid:       in.runnerID,
		...
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/app/usecases/chat/internal/runner/... -run TestRenderSpawnContext_CarriesCrowbarHome -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/chat/internal/runner/spawnplan.go api/internal/app/usecases/chat/internal/runner/spawnplan_test.go
git commit -m "feat(runner): thread crowbarHome into the spawn template context"
```

---

### Task 3: `--home` flag + shared override helper in `scope.go`

**Files:**
- Modify: `api/cmd/crowbar/scope.go`
- Test: `api/cmd/crowbar/scope_test.go`

**Interfaces:**
- Produces: `bindScopeFlags(cmd, project, repo, workspace, home *string)` (signature grows by one parameter — every existing caller must be updated, done in Task 4); `applyHomeOverride(home string)` which sets `CROWBAR_HOME` in the process environment when `home != ""`.

- [ ] **Step 1: Write the failing test**

```go
func TestApplyHomeOverride_SetsEnvWhenNonEmpty(t *testing.T) {
	t.Setenv("CROWBAR_HOME", "")
	applyHomeOverride("/tmp/x")
	if got := os.Getenv("CROWBAR_HOME"); got != "/tmp/x" {
		t.Fatalf("CROWBAR_HOME = %q, want %q", got, "/tmp/x")
	}
}

func TestApplyHomeOverride_LeavesEnvAloneWhenEmpty(t *testing.T) {
	t.Setenv("CROWBAR_HOME", "/already/set")
	applyHomeOverride("")
	if got := os.Getenv("CROWBAR_HOME"); got != "/already/set" {
		t.Fatalf("CROWBAR_HOME = %q, want unchanged %q", got, "/already/set")
	}
}
```

(Add `"os"` to `scope_test.go`'s imports.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/cmd/crowbar/... -run TestApplyHomeOverride -v`
Expected: FAIL — `applyHomeOverride` does not exist.

- [ ] **Step 3: Write minimal implementation**

In `scope.go`:

```go
import "os"

// applyHomeOverride sets CROWBAR_HOME for this process when home is non-empty.
// Every in-PTY callback (hook, mcp, handoff dump) is a fresh, short-lived
// process invoked once per event/session, so mutating process-global state
// here at startup is race-free — there is no concurrent second invocation in
// the same process to race against.
//
// Without this, a callback resolves its home from whatever CROWBAR_HOME the
// vendor CLI's own hook/subprocess mechanism forwards, which is not
// guaranteed, and silently falls back to the real ~/.crowbar when absent —
// the exact mechanism that let an isolated dev instance's hook events land in
// production. home is now baked into the command line at spawn time
// (TemplateCtx.CrowbarHome / ScopeFlags, see spawnplan.go), so this override
// always wins over ambient environment.
func applyHomeOverride(home string) {
	if home != "" {
		_ = os.Setenv("CROWBAR_HOME", home)
	}
}
```

Extend `bindScopeFlags` with the fourth flag:

```go
func bindScopeFlags(
	cmd *cobra.Command,
	project, repo, workspace, home *string,
) {
	cmd.Flags().StringVar(project, "project", "", "project id")
	cmd.Flags().StringVar(repo, "repo", "", "repo id")
	cmd.Flags().StringVar(workspace, "workspace", "", "workspace id")
	cmd.Flags().StringVar(home, "home", "", "crowbar home this callback must operate against")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/cmd/crowbar/... -run TestApplyHomeOverride -v`
Expected: PASS (this step alone will not yet compile the package — `bindScopeFlags`'s new signature breaks `hook.go`/`mcp.go`/`handoff.go`/`scope_test.go`'s other callers; Task 4 fixes those callers. If your toolchain requires the package to compile before running any test in it, do Task 4's Step 3 edits first, then return here to confirm both tests pass together.)

- [ ] **Step 5: Commit**

```bash
git add api/cmd/crowbar/scope.go api/cmd/crowbar/scope_test.go
git commit -m "feat(crowbar): add --home flag and applyHomeOverride helper"
```

---

### Task 4: Apply `--home` in `hook`, `mcp`, and `handoff dump`

**Files:**
- Modify: `api/cmd/crowbar/hook.go`
- Modify: `api/cmd/crowbar/mcp.go`
- Modify: `api/cmd/crowbar/handoff.go`
- Test: `api/cmd/crowbar/hook_test.go`

**Interfaces:**
- Consumes: `bindScopeFlags(cmd, project, repo, workspace, home *string)` and `applyHomeOverride(home string)` (Task 3).

- [ ] **Step 1: Write the failing test**

Add to `hook_test.go` (same file/pattern as `TestRunHook_ForwardsSegmentProviderAndRawPayload`):

```go
func TestNewHookCmd_HomeFlagOverridesEnv(t *testing.T) {
	t.Setenv("CROWBAR_HOME", "/wrong/home")
	cmd := newHookCmd()
	cmd.SetArgs([]string{"session_start", "--home", "/tmp/right-home", "--payload", "{}"})
	// The command swallows all errors (must never break the vendor CLI), so
	// Execute always returns nil; what we assert is the env var it left behind.
	_ = cmd.Execute()
	if got := os.Getenv("CROWBAR_HOME"); got != "/tmp/right-home" {
		t.Fatalf("CROWBAR_HOME = %q, want %q", got, "/tmp/right-home")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/cmd/crowbar/... -run TestNewHookCmd_HomeFlagOverridesEnv -v`
Expected: FAIL — package does not compile (`bindScopeFlags` call site in `hook.go` still passes 4 args) until Step 3 lands, then FAIL on the assertion until the RunE calls `applyHomeOverride`.

- [ ] **Step 3: Write minimal implementation**

`hook.go` — add `home` and call the override before anything else in `RunE`:

```go
func newHookCmd() *cobra.Command {
	var segment, provider, payloadFile, payloadInline, project, repo, workspace, home string
	cmd := &cobra.Command{
		Use:    "hook <event>",
		Short:  "Forward a vendor-CLI hook payload to the Crowbar daemon",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(_ *cobra.Command, args []string) error {
			applyHomeOverride(home)
			// A hook must never break the vendor CLI: swallow every error into
			// an exit-0 RunE, surfaced on stderr only (never stdout).
			payload, err := resolvePayload(payloadInline, payloadFile, os.Stdin)
			if err == nil {
				err = runHook(hookRun{
					Event: args[0], Segment: segment, Provider: provider,
					Project: project, Repo: repo, Workspace: workspace,
					Payload: payload, Host: "unix://", Out: os.Stdout,
				})
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "crowbar hook %s: %v\n", args[0], err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&segment, "segment", "", "Crowbar segment id")
	cmd.Flags().StringVar(&provider, "provider", "", "provider id")
	cmd.Flags().StringVar(&payloadFile, "payload-file", "", "read the payload from this file instead of stdin")
	cmd.Flags().StringVar(&payloadInline, "payload", "", "inline payload instead of stdin")
	bindScopeFlags(cmd, &project, &repo, &workspace, &home)
	return cmd
}
```

`mcp.go` — same shape:

```go
func newMCPCmd() *cobra.Command {
	var project, repo, workspace, segment, token, home string
	cmd := &cobra.Command{
		Use:    "mcp",
		Short:  "Relay MCP stdio traffic to the Crowbar daemon",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			applyHomeOverride(home)
			client, err := ipc.NewClientWithTimeout("unix://", mcpRelayTimeout)
			if err != nil {
				return err
			}
			post := func(path string, body any) (int, []byte, error) {
				return client.PostJSON(context.Background(), path, body)
			}
			return runMCPRelay(os.Stdin, os.Stdout, post, segment, project, repo, workspace, token)
		},
	}
	cmd.Flags().StringVar(&segment, "segment", "", "Crowbar segment id")
	cmd.Flags().StringVar(&token, "token", "", "runner token minted at spawn")
	bindScopeFlags(cmd, &project, &repo, &workspace, &home)
	return cmd
}
```

`handoff.go` — same shape:

```go
func newHandoffDumpCmd() *cobra.Command {
	var project, repo, workspace, home string
	cmd := &cobra.Command{
		Use:   "dump <chatId>",
		Short: "Print a chat's assembled handoff to stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			applyHomeOverride(home)
			return runHandoffDump(args[0], project, repo, workspace, "unix://", os.Stdout)
		},
	}
	bindScopeFlags(cmd, &project, &repo, &workspace, &home)
	return cmd
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/cmd/crowbar/... -v`
Expected: PASS — including the full existing suite (`hook_test.go`, `scope_test.go`, `scope_roundtrip_test.go`, `handoff_test.go`, `mcp_test.go`), since only the call sites changed, not `bindScopeFlags`'s existing three flags.

- [ ] **Step 5: Commit**

```bash
git add api/cmd/crowbar/hook.go api/cmd/crowbar/mcp.go api/cmd/crowbar/handoff.go api/cmd/crowbar/hook_test.go
git commit -m "feat(crowbar): apply --home in hook, mcp, and handoff dump"
```

---

### Task 5: Stop one dead envelope from blocking the whole hook-spool

**Files:**
- Modify: `api/cmd/crowbar/hook_spool.go`
- Test: `api/cmd/crowbar/hook_spool_test.go` (new — this package currently has none)

**Interfaces:**
- Produces: `drainHookSpoolFor` no longer aborts its whole pass on the first delivery error; an envelope that fails delivery `maxDeliveryAttempts` times in a row is moved to `<hookSpoolDir>/dead-letter/` instead of being retried forever. Attempt count is tracked durably in the filename (survives daemon restarts), not in memory.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDrainHookSpool_SkipsPastAPermanentlyFailingEnvelope writes two spooled
// envelopes: one scoped to a project the stub daemon 404s on every request
// (simulating a deleted project), one scoped to a project it accepts. Before
// this fix, the failing envelope — first in FIFO order — aborted the whole
// pass and the second envelope was NEVER attempted, no matter how many times
// the loop ticked.
func TestDrainHookSpool_SkipsPastAPermanentlyFailingEnvelope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CROWBAR_HOME", home)
	sock := filepath.Join(shortSocketDir(t), "h.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	var delivered []string
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/projects/dead-project/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		delivered = append(delivered, r.URL.Path)
		w.WriteHeader(http.StatusAccepted)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	writeSpooledEnvelope(t, home, "0000000000000000001", hookEnvelope{
		DeliveryID: "dead", Project: "dead-project", Workspace: "w1",
		Event: "turn_stop", CreatedAt: "2026-09-10T00:00:00Z",
	})
	writeSpooledEnvelope(t, home, "0000000000000000002", hookEnvelope{
		DeliveryID: "live", Project: "live-project", Workspace: "w1",
		Event: "turn_stop", CreatedAt: "2026-09-10T00:00:01Z",
	})

	for i := 0; i < maxDeliveryAttempts+1; i++ {
		_ = drainHookSpool(context.Background(), "unix://"+sock)
	}

	require.Contains(t, delivered, "/v0/projects/live-project/home/chats/hooks",
		"the live envelope must be delivered even though the dead one, queued first, never succeeds")

	deadLetterDir := filepath.Join(hookSpoolDir(), "dead-letter")
	entries, err := os.ReadDir(deadLetterDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the permanently-failing envelope must be moved to dead-letter, not retried forever")
}

// writeSpooledEnvelope writes envelope straight into home's hook-spool with
// the given sort-order prefix, bypassing persistHookEnvelope's own filename
// scheme so the test controls FIFO order directly.
func writeSpooledEnvelope(t *testing.T, home, prefix string, envelope hookEnvelope) {
	t.Helper()
	dir := filepath.Join(home, "hook-spool")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	data, err := json.Marshal(envelope)
	require.NoError(t, err)
	name := prefix + "-" + envelope.DeliveryID + ".json"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o600))
}
```

(Add `"net"`, `"net/http"`, `"strings"`, `"encoding/json"` to this new test file's imports as needed; `shortSocketDir` already exists in `hook_test.go` in the same package.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/cmd/crowbar/... -run TestDrainHookSpool_SkipsPastAPermanentlyFailingEnvelope -v`
Expected: FAIL — `maxDeliveryAttempts` does not exist yet, and even once it's stubbed in, the live envelope is never delivered because `drainHookSpoolFor` returns on the dead envelope's first failure.

- [ ] **Step 3: Write minimal implementation**

In `hook_spool.go`:

```go
const maxDeliveryAttempts = 5

func drainHookSpoolFor(
	ctx context.Context,
	host string,
	deliveryID string,
) (mine []byte, err error) {
	dir := hookSpoolDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("hook spool: read: %w", err)
	}
	release, acquired, err := acquireHookDrain(dir)
	if err != nil || !acquired {
		return nil, err
	}
	defer release()

	for _, name := range spooledNames(entries) {
		if err := os.Chtimes(filepath.Join(dir, hookDrainLockName), time.Now(), time.Now()); err != nil {
			return mine, fmt.Errorf("hook spool: renew drain lease: %w", err)
		}
		envelope, body, deliverErr := deliverSpooled(ctx, host, dir, name)
		if deliverErr != nil {
			name, moveErr := recordFailedAttempt(dir, name)
			if moveErr != nil {
				slog.WarnContext(ctx, "hook spool: record failed attempt", "name", name, "err", moveErr)
			}
			slog.WarnContext(ctx, "hook spool: delivery deferred", "name", name, "err", deliverErr)
			continue
		}
		if deliveryID != "" && envelope.DeliveryID == deliveryID {
			mine = body
		}
	}
	return mine, nil
}

// recordFailedAttempt bumps name's durable attempt count (encoded in the
// filename as ".attemptN" before ".json") and, once it reaches
// maxDeliveryAttempts, moves the envelope to dir/dead-letter instead of
// retrying it forever. Durable rather than in-memory: the persistent drain
// loop runs for the whole life of the daemon, but a daemon restart must not
// forget how many times an envelope has already failed and re-grant it a
// fresh budget.
func recordFailedAttempt(dir, name string) (string, error) {
	base := strings.TrimSuffix(name, ".json")
	attempts := 1
	if idx := strings.LastIndex(base, ".attempt"); idx >= 0 {
		if n, err := strconv.Atoi(base[idx+len(".attempt"):]); err == nil {
			attempts = n + 1
			base = base[:idx]
		}
	}
	if attempts >= maxDeliveryAttempts {
		deadDir := filepath.Join(dir, "dead-letter")
		if err := os.MkdirAll(deadDir, 0o700); err != nil {
			return name, fmt.Errorf("hook spool: mkdir dead-letter: %w", err)
		}
		dest := filepath.Join(deadDir, base+".json")
		if err := os.Rename(filepath.Join(dir, name), dest); err != nil {
			return name, fmt.Errorf("hook spool: move to dead-letter: %w", err)
		}
		return dest, nil
	}
	newName := fmt.Sprintf("%s.attempt%d.json", base, attempts)
	if err := os.Rename(filepath.Join(dir, name), filepath.Join(dir, newName)); err != nil {
		return name, fmt.Errorf("hook spool: rename attempt: %w", err)
	}
	return newName, nil
}
```

Update `spooledNames` if needed so a `.attemptN.json` file still sorts and is still picked up (it already matches `filepath.Ext(entry.Name()) == ".json"`, and lexical sort on the leading timestamp prefix is unaffected by the `.attemptN` suffix, so no change should be required there — confirm with the test).

Add `"strconv"` to `hook_spool.go`'s imports.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/cmd/crowbar/... -v`
Expected: PASS, including every pre-existing test in the package (`TestRunHook_ForwardsSegmentProviderAndRawPayload` and friends) — this change only affects the failure path.

- [ ] **Step 5: Commit**

```bash
git add api/cmd/crowbar/hook_spool.go api/cmd/crowbar/hook_spool_test.go
git commit -m "fix(crowbar): dead-letter a permanently-failing hook envelope instead of blocking the spool"
```

---

## Self-Review Notes

- **Spec coverage:** Task 1–2 give every spawn a correctly-scoped `CrowbarHome`; Task 3–4 make `hook`/`mcp`/`handoff dump` apply it regardless of ambient environment; Task 5 fixes the independent head-of-line-blocking bug so the fix is not undone by any FUTURE cause of a stuck envelope (a legitimately deleted project mid-flight, a daemon bug, etc.).
- **Not in scope:** hardening `crowbar-seed` itself (separate plan, `2026-09-12-seed-home-safety.md`, since it shares no files with this one) and reconciling the orphaned `projects/*` directories already on disk (root cause not yet confirmed on the second, non-dev machine — held open per the user's own instruction).
