# Conversation List, Chat Titles & Prompts Config — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every `AgentChat` a human title that the agent sets via a `crowbar chat rename` command (first-prompt fallback), move Crowbar's agent-facing prompts into `~/.crowbar/config.yaml`, and delete the dead "conversation" iterations (markdown-chat + mock runs; the unmounted chat REST + CRUD usecase).

**Architecture:** Backend-only for now (no new FE). Titles are agent-driven: Crowbar injects a title instruction (from config) into the agent's system prompt at spawn; the agent calls back via `crowbar chat rename <chatid> <title>` → `POST /v0/agent/chats/:id/rename` → the usecase sets `AgentChat.Title` under user>agent>derived precedence. Prompt text lives in config; the injection mechanism stays in the descriptor.

**Tech Stack:** Go (gin, gorm+glebarez/sqlite single-conn, cobra, yaml.v3, creack/pty, testify); unix-socket ipc; TS/React frontend (vitest, `bun tsc`).

**Spec:** [`docs/superpowers/specs/2026-07-06-conversation-list-title-config-design.md`](../specs/2026-07-06-conversation-list-title-config-design.md)

## Global Constraints

- **Config is prompts-only.** `~/.crowbar/config.yaml` (embedded `default.yaml` overlaid by the user file); the unused `intelligence`/`GetIntelligence`/`ModelForTier` are removed.
- **Title precedence: user > agent > derived.** `AgentChat.TitleLocked bool`. Derived sets only if empty; agent sets unless locked; user sets and locks. Empty title is always a no-op.
- **A `crowbar` command run by the agent must never break the agent's turn** — errors swallowed to exit-0, stderr only (same as `crowbar hook`).
- **Rename endpoint:** `POST /v0/agent/chats/:id/rename` `{title}` with `?source=agent` for the agent path (matches the `/switch` POST convention; the ipc client is POST-only).
- **Guaranteed template variables (additions):** `{crowbar}` (crowbar binary path, same value as `{crowbar_hook}`), `{chatid}` (chat id). Existing: `{tmp} {cwd} {crowbar_hook} {segid} {provider} {id} {handoff}`.
- **Keep live — do NOT delete:** `domain.Chat`, `app/repositories/chat/`, `app/usecases/branchreview`, `branchchat`, `domain.BranchChat` (Branch Review reads them). Only `endpoints/chats` + `usecases/chat` are dead.
- **`cmd/crowbar` builds only under `-tags noEmbed`** (empty `//go:embed all:web/dist`).
- **Frontend tests** live in `web/src/__tests__/` mirror; components kebab-case; `bun tsc --noEmit` + `prettier --check` gate CI.
- **Per-task green ≠ full build.** Package-scoped `go test` compiles a package + its imports, not its importers; a downstream compile break from a not-yet-updated caller is expected and closed by that caller's task. Task 8 restores the full build + suites.

**Task order:** 1 Config → 2 Prereqs (template vars + domain field) → 3 Usecase → 4 API → 5 CLI → 6 Backend cleanup → 7 Frontend cleanup → 8 Integration + live.

---

### Task 1: Config — prompts-only

**Files:**
- Modify: `api/internal/core/config/config.go`
- Modify: `api/internal/core/config/default.yaml`
- Test: `api/internal/core/config/config_test.go`

**Interfaces:**
- Produces: `config.Prompts{ TitleInstruction, HandoffWrapper string }`; `config.GetPrompts() Prompts`. Removes `Intelligence`, `ConfigData.Intelligence`, `GetIntelligence`, `ModelForTier`.

- [ ] **Step 1: Rewrite the test**

Replace `api/internal/core/config/config_test.go` with:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

func TestGetPrompts_FromEmbeddedDefaults(t *testing.T) {
	resetForTesting()
	p := GetPrompts()
	assert.Contains(t, p.TitleInstruction, "chat rename {chatid}")
	assert.Contains(t, p.HandoffWrapper, "{conversation}")
}

func TestGetPrompts_UserConfigOverlays(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CROWBAR_HOME", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("config:\n  prompts:\n    handoff_wrapper: \"CUSTOM {conversation}\"\n"), 0o644))
	metadata.ResetForTesting() // re-resolve paths under the new CROWBAR_HOME
	resetForTesting()

	p := GetPrompts()
	assert.Equal(t, "CUSTOM {conversation}", p.HandoffWrapper)
	// absent field keeps the embedded default
	assert.Contains(t, p.TitleInstruction, "chat rename {chatid}")
}
```

> If `metadata` has no `ResetForTesting`, use whatever the package already exposes to re-resolve paths after setting `CROWBAR_HOME`, or drop that line if `GetConfigPath()` reads the env live. Read `metadata.go` to confirm; adapt the one line, not the assertions.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd api && go test ./internal/core/config/ -run TestGetPrompts`
Expected: FAIL (compile — `GetPrompts`/`Prompts` undefined).

- [ ] **Step 3: Rewrite `config.go`**

Replace the `Intelligence`/`ConfigData`/`GetIntelligence`/`ModelForTier` declarations with:

```go
// Prompts holds Crowbar's agent-facing prompt templates. Placeholders
// ({crowbar}, {chatid}, {conversation}) are expanded by Crowbar at injection time.
type Prompts struct {
	TitleInstruction string `yaml:"title_instruction"`
	HandoffWrapper   string `yaml:"handoff_wrapper"`
}

// ConfigData is the top-level config section.
type ConfigData struct {
	Prompts Prompts `yaml:"prompts"`
}

// Config is the full config structure loaded from default.yaml and overlaid by
// the user's ~/.crowbar/config.yaml.
type Config struct {
	Config ConfigData `yaml:"config"`
}

// GetPrompts returns the configured agent-facing prompt templates.
func GetPrompts() Prompts {
	return Get().Config.Prompts
}
```

Leave `Get`, `getDefaultConfig`, `resetForTesting`, and the `var (…)` block unchanged. Delete `Intelligence`, `GetIntelligence`, and `ModelForTier` entirely.

- [ ] **Step 4: Rewrite `default.yaml`**

```yaml
config:
  prompts:
    title_instruction: |
      Give this conversation a short title, once, by running exactly this command (replace the placeholder with a concise 2-5 word Title-Case title of the task):
      {crowbar} chat rename {chatid} "<title>"
    handoff_wrapper: |
      === HANDED-OFF CONTEXT (Crowbar) ===
      {conversation}
      === END ===
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd api && go test ./internal/core/config/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/core/config/
git commit -m "refactor(config): config is prompts-only (title_instruction + handoff_wrapper); drop unused intelligence"
```

---

### Task 2: Prerequisites — template vars + domain field

**Files:**
- Modify: `api/internal/engine/agent/template.go`
- Test: `api/internal/engine/agent/template_test.go`
- Modify: `api/internal/domain/agent_chat.go`

**Interfaces:**
- Produces: `agent.TemplateCtx` gains `Crowbar` (unused-name-note: mapped from existing `CrowbarHook`) and `Chatid` — actually just add `Chatid`; `{crowbar}` aliases the existing `CrowbarHook` field. `Expand` also replaces `{crowbar}` and `{chatid}`. `domain.AgentChat` gains `TitleLocked bool`.

- [ ] **Step 1: Update `template_test.go`**

Add a case to `TestExpand_ReplacesKnownTokens` (keep the existing asserts):

```go
	require.Equal(t,
		"/bin/crowbar chat rename c-9 \"x\"",
		agent.Expand("{crowbar} chat rename {chatid} \"x\"",
			agent.TemplateCtx{CrowbarHook: "/bin/crowbar", Chatid: "c-9"}))
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd api && go test ./internal/engine/agent/ -run TestExpand_ReplacesKnownTokens`
Expected: FAIL (compile — `Chatid` field / `{chatid}` missing).

- [ ] **Step 3: Edit `template.go`**

Add `Chatid string` to `TemplateCtx`, and add two entries to the `strings.NewReplacer` in `Expand`:

```go
		"{crowbar}", ctx.CrowbarHook, // same binary as {crowbar_hook}; friendlier for non-hook commands
		"{chatid}", ctx.Chatid,
```

(Place them alongside the existing `{crowbar_hook}` / `{segid}` entries.)

- [ ] **Step 4: Run it to verify it passes**

Run: `cd api && go test ./internal/engine/agent/ -run TestExpand_ReplacesKnownTokens`
Expected: PASS.

- [ ] **Step 5: Add the domain field**

In `api/internal/domain/agent_chat.go`, add after `Title`:

```go
	TitleLocked     bool      `json:"titleLocked"`
```

- [ ] **Step 6: Build check + commit**

Run: `cd api && go build ./internal/engine/agent/... ./internal/domain/... && go test ./internal/engine/agent/ -run TestExpand`
Expected: success + PASS.

```bash
git add api/internal/engine/agent/template.go api/internal/engine/agent/template_test.go api/internal/domain/agent_chat.go
git commit -m "feat(agent): add {crowbar}/{chatid} template vars and AgentChat.TitleLocked"
```

---

### Task 3: Usecase — RenameChat, first-prompt fallback, spawn title injection, config-driven handoff

**Files:**
- Modify: `api/internal/app/usecases/agent/agent.go`
- Test: `api/internal/app/usecases/agent/agent_test.go` (+ sibling `*_test.go` calling `spawnSegment`/`IngestHook`)

**Interfaces:**
- Consumes: `config.GetPrompts()` (T1); `TemplateCtx{Chatid}` + `{crowbar}`/`{chatid}` (T2); `domain.AgentChat.TitleLocked` (T2).
- Produces: `(*Usecase).RenameChat(ctx context.Context, chatID, title, source string) error`.

- [ ] **Step 1: Add `RenameChat` + `deriveTitle`**

Add to `agent.go` (add `"github.com/char2cs/crowbar/api/internal/core/config"` to imports):

```go
// RenameChat sets a chat's title under user>agent>derived precedence:
//   source "derived": set only if the title is currently empty (first-prompt fallback).
//   source "agent":   set unless the title is user-locked (agent may upgrade a derived title).
//   source "user"/"": set unconditionally AND lock (a manual rename wins and sticks).
// An empty title is always a no-op. Broadcasts "titled" on a successful change.
func (u *Usecase) RenameChat(
	ctx context.Context,
	chatID, title, source string,
) error {
	if title == "" {
		return nil
	}
	chat, err := u.repo.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: rename chat: get: %w", err)
	}
	switch source {
	case "derived":
		if chat.Title != "" {
			return nil
		}
	case "agent":
		if chat.TitleLocked {
			return nil
		}
	default: // "user" / "" — manual rename wins and locks
		chat.TitleLocked = true
	}
	chat.Title = title
	if err := u.repo.SaveChat(ctx, chat); err != nil {
		return fmt.Errorf("agent: rename chat: save: %w", err)
	}
	u.bc.BroadcastAgentChat(chatID, "titled")
	return nil
}

// deriveTitle turns a user prompt into a short chat title: the first non-empty
// line, trimmed, capped to 60 runes.
func deriveTitle(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		r := []rune(line)
		if len(r) > 60 {
			return strings.TrimSpace(string(r[:60])) + "…"
		}
		return line
	}
	return ""
}
```

- [ ] **Step 2: Fire the derived fallback on `user_prompt`**

In `IngestHook`, change the `user_prompt` switch case to set the derived title before appending the turn:

```go
	case "user_prompt":
		if err := u.RenameChat(ctx, chat.ID, deriveTitle(ev.Message), "derived"); err != nil {
			slog.WarnContext(ctx, "agent: ingest hook: derived title", "err", err, "chat_id", chat.ID)
		}
		return u.appendTurn(ctx, seg, chat, crowbarHome, projectID, repoID, "user", ev.Message)
```

- [ ] **Step 3: Config-driven handoff wrapper**

In `AssembleHandoff`, replace the hardcoded wrapper return. The current tail reads:

```go
	if len(blob) == 0 {
		return "", nil
	}
	return "=== HANDED-OFF CONTEXT (Crowbar) ===\n" + string(blob) + "\n=== END ===", nil
```

Replace with:

```go
	if len(blob) == 0 {
		return "", nil
	}
	return strings.ReplaceAll(config.GetPrompts().HandoffWrapper, "{conversation}", string(blob)), nil
```

- [ ] **Step 4: Inject the title instruction on a fresh spawn**

Add an `injectTitle bool` parameter to `spawnSegment` and set the template context. Change the signature:

```go
func (u *Usecase) spawnSegment(
	ctx context.Context,
	chat domain.AgentChat,
	providerID string,
	extraSteps []engineagent.InjectStep,
	handoff string,
	injectTitle bool,
) (string, error) {
```

Inside, where `tctx` is built and the plan is created, replace that block with:

```go
	tctx := engineagent.TemplateCtx{
		Tmp:         tmpDir,
		Cwd:         worktree,
		CrowbarHook: u.crowbarHookPath(crowbarHome),
		Segid:       segID,
		Provider:    providerID,
		Chatid:      chat.ID,
	}
	steps := extraSteps
	if injectTitle {
		// Fresh chat: the injected system-prompt document is the title instruction
		// (from config), delivered through the descriptor's handoff_inject mechanism.
		tctx.Handoff = engineagent.Expand(config.GetPrompts().TitleInstruction, tctx)
		steps = descriptor.HandoffInject
	} else {
		tctx.Handoff = handoff
	}
	plan, err := engineagent.BuildSpawnPlan(descriptor, tctx, os.Environ(), steps)
```

Update the two callers:
- In `SpawnChat`: `segID, err = u.spawnSegment(ctx, chat, providerID, nil, "", true)`
- In `SwitchProvider`: `newSegID, err := u.spawnSegment(ctx, chat, targetProviderID, extraSteps, handoff, false)`

- [ ] **Step 5: Write the tests**

Add to `agent_test.go` (reuse the file's existing fixtures for constructing the usecase + spawning — do NOT invent new ones; only the assertions below are new). Add a `mustJSON` helper only if the file doesn't already have one from prior work.

```go
func TestRenameChat_Precedence(t *testing.T) {
	u, _ := newTestUsecase(t)                     // reuse existing fixture
	chatID, _ := mustSpawn(t, u, "claude")        // reuse existing fixture
	ctx := context.Background()

	// derived sets when empty, then does not overwrite
	require.NoError(t, u.RenameChat(ctx, chatID, "First Topic", "derived"))
	require.Equal(t, "First Topic", getTitle(t, u, chatID))
	require.NoError(t, u.RenameChat(ctx, chatID, "Second Topic", "derived"))
	require.Equal(t, "First Topic", getTitle(t, u, chatID))

	// agent upgrades a derived title
	require.NoError(t, u.RenameChat(ctx, chatID, "Agent Title", "agent"))
	require.Equal(t, "Agent Title", getTitle(t, u, chatID))

	// user rename wins and locks; agent can no longer clobber
	require.NoError(t, u.RenameChat(ctx, chatID, "User Title", "user"))
	require.Equal(t, "User Title", getTitle(t, u, chatID))
	require.NoError(t, u.RenameChat(ctx, chatID, "Agent Again", "agent"))
	require.Equal(t, "User Title", getTitle(t, u, chatID))

	// empty is a no-op
	require.NoError(t, u.RenameChat(ctx, chatID, "", "user"))
	require.Equal(t, "User Title", getTitle(t, u, chatID))
}

func TestIngestHook_UserPrompt_SetsDerivedTitle(t *testing.T) {
	u, _ := newTestUsecase(t)
	chatID, segID := mustSpawn(t, u, "claude")
	ctx := context.Background()

	require.NoError(t, u.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "Refactor the auth module to use JWT\nmore detail"})))
	require.Equal(t, "Refactor the auth module to use JWT", getTitle(t, u, chatID))

	// a later prompt does not overwrite
	require.NoError(t, u.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "now do something else"})))
	require.Equal(t, "Refactor the auth module to use JWT", getTitle(t, u, chatID))
}

func TestSpawnChat_InjectsTitleInstruction(t *testing.T) {
	u, deps := newTestUsecase(t)
	chatID, _ := mustSpawn(t, u, "claude")

	call := deps.commander.calls[0]
	doc := argAfter(call.argv, "--append-system-prompt") // claude's handoff_inject flag
	require.NotEmpty(t, doc)
	require.Contains(t, doc, "chat rename "+chatID)
}
```

Add a tiny `getTitle` helper if the file lacks one:

```go
func getTitle(t *testing.T, u *agentusecase.Usecase, chatID string) string {
	t.Helper()
	c, err := u.GetChat(context.Background(), chatID)
	require.NoError(t, err)
	return c.Title
}
```

(`argAfter` was added in prior agent-test work; if absent, add the 6-line helper that returns the argv element after a flag.)

Update **every** existing `u.spawnSegment(...)` call in the package's tests to the new 6-arg signature (append the `injectTitle` bool — `false` for switch-context tests, `true` only where a fresh spawn is intended). Update any test that asserted the switch path still injects the *handoff* (not the title) — a switch passes `injectTitle=false`.

- [ ] **Step 6: Run the package tests**

Run: `cd api && go test ./internal/app/usecases/agent/... -race`
Expected: PASS. (The API handler and cmd won't build yet — T4/T5 close them.)

- [ ] **Step 7: Commit**

```bash
git add api/internal/app/usecases/agent/
git commit -m "feat(agent): chat titles — RenameChat precedence, first-prompt fallback, spawn title injection, config handoff wrapper"
```

---

### Task 4: API — `POST /v0/agent/chats/:id/rename`

**Files:**
- Modify: `api/internal/api/v0/endpoints/agent/handlers/handlers.go` (add to `AgentUsecase`)
- Modify: `api/internal/api/v0/endpoints/agent/handlers/chats.go` (add `Rename` handler)
- Modify: `api/internal/api/v0/endpoints/agent/routes.go` (mount the route)
- Test: `api/internal/api/v0/endpoints/agent/handlers/chats_test.go` (+ any stub usecase in `routes_test.go`)

**Interfaces:**
- Consumes: `(*Usecase).RenameChat(ctx, chatID, title, source string) error` (T3).

- [ ] **Step 1: Extend the `AgentUsecase` interface**

In `handlers.go`, add to the `AgentUsecase` interface:

```go
	RenameChat(
		ctx context.Context,
		chatID, title, source string,
	) error
```

- [ ] **Step 2: Add the handler**

In `chats.go`:

```go
// Rename handles POST /v0/agent/chats/:id/rename: sets the chat's title.
// `?source=agent` applies the agent precedence rule (skip if user-locked); the
// default (a human/FE rename) sets unconditionally and locks.
func (h *Handlers) Rename(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")
	source := ctx.Query("source")

	var body struct {
		Title string `json:"title"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.usecase.RenameChat(rctx, id, body.Title, source); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteAccepted(ctx)
}
```

- [ ] **Step 3: Mount the route**

In `routes.go`, alongside the other `rg.POST("/agent/chats/...")` lines:

```go
	rg.POST("/agent/chats/:id/rename", h.Rename)
```

- [ ] **Step 4: Update tests + stubs**

Add a handler test in `chats_test.go`:

```go
func TestRename_PostsTitleAndSource(t *testing.T) {
	rec, ctx := newTestContext(t, http.MethodPost,
		"/v0/agent/chats/c-1/rename?source=agent",
		`{"title":"My Title"}`) // reuse the file's existing request helper
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	uc := &stubUsecase{} // reuse the package's stub
	New(uc).Rename(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Equal(t, "c-1", uc.renamedID)
	require.Equal(t, "My Title", uc.renamedTitle)
	require.Equal(t, "agent", uc.renamedSource)
}
```

> Adapt to the file's real test harness (request construction, stub name). Add `RenameChat` to whatever stub usecase(s) implement `AgentUsecase` (in `chats_test.go`/`routes_test.go`), recording the args:
> ```go
> func (s *stubUsecase) RenameChat(_ context.Context, id, title, source string) error {
> 	s.renamedID, s.renamedTitle, s.renamedSource = id, title, source
> 	return nil
> }
> ```

- [ ] **Step 5: Run the endpoint tests**

Run: `cd api && go test ./internal/api/v0/endpoints/agent/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/api/v0/endpoints/agent/
git commit -m "feat(api): POST /v0/agent/chats/:id/rename -> RenameChat (agent/user source precedence)"
```

---

### Task 5: CLI — `crowbar chat rename`

**Files:**
- Create: `api/cmd/crowbar/chat.go`
- Modify: `api/cmd/crowbar/main.go` (register the command)
- Test: `api/cmd/crowbar/chat_test.go`

**Interfaces:**
- Produces: `runChatRename(chatID, title, host string) error`; the `crowbar chat rename <chatid> <title>` command posting to `/v0/agent/chats/:id/rename?source=agent`.

- [ ] **Step 1: Write the test**

Create `api/cmd/crowbar/chat_test.go` (model it on `hook_test.go`'s unix-socket round trip):

```go
package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunChatRename_PostsTitleWithAgentSource(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "chat")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	sock := filepath.Join(tmpDir, "c.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	var mu sync.Mutex
	var gotPath string
	var gotBody map[string]any
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.RequestURI()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	err = runChatRename("c-42", "Fix Auth Flow", "unix://"+sock)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "/v0/agent/chats/c-42/rename?source=agent", gotPath)
	require.Equal(t, "Fix Auth Flow", gotBody["title"])
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd api && go test -tags noEmbed ./cmd/crowbar/ -run TestRunChatRename`
Expected: FAIL (compile — `runChatRename` undefined).

- [ ] **Step 3: Create `chat.go`**

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

func newChatCmd() *cobra.Command {
	chat := &cobra.Command{
		Use:    "chat",
		Short:  "Manage Crowbar agent chats",
		Hidden: true,
	}
	rename := &cobra.Command{
		Use:   "rename <chatid> <title>",
		Short: "Set an agent chat's title",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			// Run by the agent — must never break its turn: swallow to exit-0,
			// stderr only (never stdout).
			if err := runChatRename(args[0], args[1], "unix://"); err != nil {
				fmt.Fprintf(os.Stderr, "crowbar chat rename %s: %v\n", args[0], err)
			}
			return nil
		},
	}
	chat.AddCommand(rename)
	return chat
}

// runChatRename posts a title to the daemon with source=agent (agent precedence:
// upgrade a derived title, never clobber a user-locked one).
func runChatRename(chatID, title, host string) error {
	client, err := ipc.NewClient(host)
	if err != nil {
		return err
	}
	_, _, err = client.PostJSON(context.Background(),
		"/v0/agent/chats/"+chatID+"/rename?source=agent",
		map[string]any{"title": title})
	return err
}
```

- [ ] **Step 4: Register the command**

In `main.go`, add `newChatCmd()` to the `root.AddCommand(...)` call:

```go
	root.AddCommand(newServeCmd(), newVersionCmd(), newHookCmd(), newHandoffCmd(), newChatCmd())
```

- [ ] **Step 5: Run it to verify it passes**

Run: `cd api && go test -tags noEmbed ./cmd/crowbar/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/cmd/crowbar/chat.go api/cmd/crowbar/main.go api/cmd/crowbar/chat_test.go
git commit -m "feat(cmd): crowbar chat rename <chatid> <title> -> POST rename?source=agent"
```

---

### Task 6: Backend cleanup — delete dead chat REST + CRUD usecase

**Files:**
- Delete: `api/internal/api/v0/endpoints/chats/` (whole dir)
- Delete: `api/internal/app/usecases/chat/` (whole dir)
- Modify: `api/internal/app/usecases/container.go` (unwire the chat usecase)

**Interfaces:** none produced; this removes dead code. **Keep** `repos.Chat`, `domain.Chat`, `branchreview`, `branchchat`.

- [ ] **Step 1: Confirm the dead set (grep before deleting)**

Run:
```bash
cd api && grep -rn "chats.Register" . --include=*.go | grep -v "endpoints/chats/"
cd api && grep -rln "app/usecases/chat\"" . --include=*.go
```
Expected: first returns nothing (never mounted); second returns only `usecases/container.go` (+ the package's own test). If either shows another live consumer, STOP and report — the removal scope is wrong.

- [ ] **Step 2: Delete the two dead dirs**

```bash
cd api && git rm -r internal/api/v0/endpoints/chats internal/app/usecases/chat
```

- [ ] **Step 3: Unwire the chat usecase from `usecases/container.go`**

Read `api/internal/app/usecases/container.go`. Remove: the `usecases/chat` import; the `chatUsecase := chat.New(repos.Chat, …)` construction; and the `Chat: chatUsecase` field in the returned `Usecases` struct (and the `Chat` field on the `Usecases` struct type if it lives here). **Keep** the separate `repos.Chat` argument passed into the `branchreview.New(…)` construction — that is Branch Review and stays. If `repos.Chat` becomes referenced only by branchreview after this, that is correct.

- [ ] **Step 4: Build + Branch Review still compiles/tests**

Run:
```bash
cd api && go build ./... && go test ./internal/app/usecases/branchreview/... ./internal/api/v0/endpoints/review/...
```
Expected: build succeeds; Branch Review tests pass. (If `go build ./...` errors in `cmd/crowbar` on the `all:web/dist` embed, that's the known env quirk — use `go build -tags noEmbed ./...`.)

- [ ] **Step 5: Commit**

```bash
git add -A api/internal
git commit -m "refactor(chat): delete unmounted endpoints/chats + dead usecases/chat; keep live domain.Chat/repo (Branch Review)"
```

---

### Task 7: Frontend cleanup — remove markdown-chat, the `crowbarChat` seam, and the dead-wired sidebar-chats UI

**Files:** see the manifest below. All under `web/src`.

**Interfaces:** none produced; removes dead FE. Per spec §8.1 the sidebar-chats UI is **removed now, rebuilt fresh in the FE phase**.

- [ ] **Step 1: Delete wholesale**

```bash
cd web && git rm -r \
  src/features/markdown-chat \
  src/routes/_shell/chat/\$chatId.tsx \
  src/lib/api/run.ts \
  src/mocks/handlers/markdown-chat.ts \
  src/lib/mock/markdown-chat.ts \
  src/__tests__/features/markdown-chat \
  src/features/editor/stores/buffer-content-factory.ts
```
(`buffer-content-factory.ts` is already-orphaned dead code that references `crowbarChat`; confirmed zero importers.)

- [ ] **Step 2: Excise the `crowbarChat` buffer kind (surgical)**

Edit each, removing every `crowbarChat` branch/member (the discriminated union `PaneContent`/`OpenContentSpec` loses one arm):
- `src/features/panes/types/pane-content.ts` — remove `'crowbarChat'` from `PaneContentType`, the `CrowbarChatContent` interface, the `| CrowbarChatContent` union arm, `'crowbarChat'` from `VIRTUAL_TYPES`, and the `crowbarChat` variant of `OpenContentSpec`.
- `src/features/panes/components/pane-container.tsx` — remove the lazy `MarkdownChatView` import and the `case 'crowbarChat':` render branch; drop `CrowbarChatContent` from the type import.
- `src/features/workspace/stores/slices/buffer-slice.ts` — remove the `spec.type === 'crowbarChat'` dedup branch, the `else if (spec.type === 'crowbarChat')` construction branch, and the `closeBuffer` `crowbarChat` → `destroyConversationStore` block; drop the `CrowbarChatContent` import.
- `src/features/workspace/stores/workspace-store-registry.ts` — remove the `CrowbarChatContent` import and the `chatBuffers`/`destroyConversationStore` block in `destroyWorkspaceStore`.
- `src/features/tabs/components/tab-bar-item.tsx` — remove the `buffer.type === 'crowbarChat'` icon branch.
- `src/features/tabs/components/tab-bar.tsx` — remove the `openContent({ type: 'crowbarChat', … })` "New Conversation" action.
- `src/features/tabs/components/tab-new-button.tsx` — remove the `onNewConversation` prop, its `<Chat/>` menu item + separator, and the now-unused `Chat` icon import.
- `src/lib/mock/scenarios/{index,normal,extreme,empty}.ts` — remove the `markdownTurns` field + its `MarkdownTurn` import + generator functions.
- `src/mocks/handlers/index.ts` — remove the `markdownChatHandlers` import + spread.
- `src/lib/store/chaos.ts` — remove the `'markdown-chat'` entry from `FaultKey`/`FAULT_KEYS`/`FAULT_LABELS`/`DEFAULT_FAULTS`.
- Tests: `src/__tests__/features/workspace/stores/slices/buffer-slice.test.ts` (remove the two `crowbarChat` `it()` blocks + the `conversation-store` `vi.mock`), `src/__tests__/lib/store/chaos.test.ts` (drop the `'markdown-chat': 0` fixture line), `src/__tests__/features/tabs/components/tab-new-button.test.tsx` (remove the `onNewConversation` case).

- [ ] **Step 3: Remove the dead-wired sidebar-chats UI (spec §8.1)**

```bash
cd web && git rm \
  src/components/layout/chat-tree.tsx \
  src/components/layout/chat-tree-item.tsx \
  src/components/layout/chat-tree-context.tsx \
  src/lib/store/chat-list-store.ts \
  src/lib/api/chat.ts \
  src/mocks/handlers/chats.ts \
  src/__tests__/components/layout/chat-tree.test.tsx \
  src/__tests__/components/layout/chat-tree-context.test.ts \
  src/__tests__/lib/store/chat-list-store.test.ts
```
(Adjust to the real file set — grep `chat-tree` / `chat-list-store` / `lib/api/chat` first to catch exact names.) Then surgically remove the sidebar's `chats` wiring:
- `src/lib/store/sidebar.ts` — remove the `chats: ProjectChat[]` field, `ProjectChat`/`ChatStatus`/`ChatType` types, and `addChat`/`deleteChat`/`renameChat` reducers.
- `src/components/layout/sidebar-tab-bar.tsx` — remove the "Chats" tab (`'chats'` from `SidebarTab`, the `ChatsCircle` entry).
- `src/components/layout/ide-shell.tsx` — remove the `useSidebarStore(s => s.chats)` usage, `activeChatId`, `chatTabLabel`, and the `/chat/:id` header render branch.
- Fix any remaining `chats:[]`/`ProjectChat` references surfaced by `bun tsc` in `src/__tests__/lib/store/sidebar*.test.ts`, `ide-shell.test.tsx`, `hydrate.test.ts` (drop the incidental fixtures).

- [ ] **Step 4: Verify the FE is green**

Run:
```bash
cd web && bun tsc --noEmit && bunx prettier --check "src/**/*.{ts,tsx}" && bunx vitest run
```
Expected: tsc clean (no dangling `crowbarChat`/`markdown-chat`/`ProjectChat` refs), prettier clean, tests pass. Iterate on whatever `tsc` flags — it is the completeness check for a discriminated-union removal.

- [ ] **Step 5: Commit**

```bash
cd web && git add -A
git commit -m "refactor(web): remove dead markdown-chat feature, crowbarChat buffer kind, and dead-wired sidebar-chats UI"
```

---

### Task 8: Integration + live agent-rename verification

**Files:**
- Create/modify: `api/tests/integration/agent/agent_title_test.go` (new integration test)
- Verify: whole-module build + suites; live drive.

**Interfaces:** consumes everything above.

- [ ] **Step 1: Full build to catch stragglers**

Run: `cd api && go build -tags noEmbed ./... 2>&1 | head -30`
Expected: clean. Any error outside the integration test dir means an earlier task missed a call site — fix in that task's package.

- [ ] **Step 2: Integration — the `crowbar chat rename` binary drives the title end-to-end**

Add `api/tests/integration/agent/agent_title_test.go`. Model it on the existing `agent_gaps_test.go` harness (`h.app`, `requireCLI`, the daemon boot). The deterministic path exercises the real `crowbar` binary → daemon → title, plus the derived fallback:

```go
//go:build integration

package agent_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAgent_CrowbarChatRename_SetsTitle proves the agent-facing command path:
// invoking the real `crowbar chat rename` binary against the running daemon
// updates the AgentChat title with agent precedence.
func TestAgent_CrowbarChatRename_SetsTitle(t *testing.T) {
	requireCLI(t, "claude")
	h := newAgentHarness(t)            // reuse the file's harness constructor
	ctx := context.Background()

	chatID, _ := mustSpawnChat(t, h, "claude") // reuse existing spawn helper

	// The agent would run this; here we run the same binary the daemon installed.
	crowbar := h.crowbarBinPath()      // <home>/bin/crowbar (selfinstall path)
	out, err := exec.Command(crowbar, "chat", "rename", chatID, "Fix The Auth Flow").CombinedOutput()
	require.NoError(t, err, "crowbar chat rename failed: %s", out)

	waitUntil(t, 5*time.Second, func() bool {
		c, _ := h.app.Usecases.Agent.GetChat(ctx, chatID)
		return c.Title == "Fix The Auth Flow"
	})
}

// TestAgent_FirstPrompt_DerivesTitle proves the fallback: a real user_prompt
// hook round-trip sets a derived title when the agent hasn't named the chat.
func TestAgent_FirstPrompt_DerivesTitle(t *testing.T) {
	requireCLI(t, "claude")
	h := newAgentHarness(t)
	ctx := context.Background()
	chatID, segID := mustSpawnChat(t, h, "claude")

	postHook(t, h, segID, "claude", "user_prompt",
		`{"prompt":"Explain how bloom filters work"}`) // reuse the file's hook-post helper

	waitUntil(t, 5*time.Second, func() bool {
		c, _ := h.app.Usecases.Agent.GetChat(ctx, chatID)
		return c.Title == "Explain how bloom filters work"
	})
}
```

> Reuse the actual harness/helper names from `agent_gaps_test.go` (`h.app`, its spawn/hook-post/wait helpers, the installed crowbar path). If no `crowbarBinPath`/`waitUntil` exists, add small local helpers; do not weaken the assertions. The title-lands assertion must read back through `Usecases.Agent.GetChat`.

- [ ] **Step 3: Run the unit suite (race) + integration**

Run:
```bash
cd api && go test -race -tags noEmbed ./... 2>&1 | tail -20
cd api && go test -tags integration ./tests/integration/agent/... -timeout 600s -v 2>&1 | tail -40
```
Expected: unit PASS across packages; integration PASS against real claude (and codex where its tests run). If the CLIs are unavailable, report which tests skipped — never claim an unobserved pass.

- [ ] **Step 4: Lint**

Run: `cd api && golangci-lint run ./...` and `cd web && bun tsc --noEmit`
Expected: clean (no new findings).

- [ ] **Step 5: Commit**

```bash
git add api/tests/integration/agent/agent_title_test.go
git commit -m "test(agent): integration — crowbar chat rename end-to-end + first-prompt derived title"
```

- [ ] **Step 6: Live / manual verification (per project rule — the user explicitly wants this)**

Rebuild the sidecar and run `make dev-desktop` (isolated `CROWBAR_HOME`, never the production instance). Then:
1. **Agent-driven rename (the headline):** spawn a real Claude chat, send a first prompt on a clear topic, and confirm the agent — following the injected `title_instruction` — actually runs `crowbar chat rename <chatid> "…"`, and the chat's title updates (verify via `GET /v0/agent/chats/:id` or a `tail` of the daemon log showing the rename POST). This is the "be sure agents can interact with title renaming" check — observe it happening with a real agent, not just the binary.
2. **Fallback:** spawn a chat where the agent does *not* rename (or before it does) and confirm the first-prompt derived title appears.
3. **Config override:** put a custom `handoff_wrapper` in the dev `~/.crowbar/config.yaml` (under the dev `CROWBAR_HOME`), do a provider switch, and confirm the handoff uses the overridden wrapper.
Record exactly what was observed for each; if the agent doesn't reliably run the command, note it (the fallback still guarantees a title) and we tune the `title_instruction` wording.

---

## Self-Review

**1. Spec coverage:**
- §1.1 FE removal → Task 7. §1.2 backend removal (corrected: keep domain.Chat/repo) → Task 6. §1.3 sidebar-chats removal → Task 7 Step 3.
- §3.1 agent-names-chat via `crowbar chat rename` → Tasks 3 (usecase), 4 (endpoint), 5 (CLI). §3.2 precedence + `TitleLocked` → Tasks 2, 3. §3.3 derived fallback → Task 3. §3.4 broadcast `titled` → Task 3.
- §4 prompts-in-config (title_instruction + handoff_wrapper, intelligence removed) → Tasks 1, 3 (consumption). §4.3 injection via handoff_inject → Task 3 Step 4.
- §5 backend surface → Tasks 1–6. §6 testing → each task's tests + Task 8. §2 FE target → deferred (not built), noted.

**2. Placeholder scan:** no TBD/"handle errors"/"similar to". The soft spots (Task 3/8 fixture-helper names, Task 7 exact sidebar file set) are bounded with an explicit "reuse the file's existing fixtures / grep for exact names; do not weaken assertions" instruction.

**3. Type consistency:** `RenameChat(ctx, chatID, title, source string) error` identical across Task 3 (def), Task 4 (interface + stub), Task 5 (the CLI posts the matching body/query). `TitleLocked bool` (T2) used by `RenameChat` (T3). `config.GetPrompts() Prompts{TitleInstruction, HandoffWrapper}` (T1) consumed by Task 3 (`AssembleHandoff`, spawn injection). `{crowbar}`/`{chatid}`/`Chatid` (T2) used in Task 3's spawn injection and Task 1's `default.yaml`. Endpoint `POST /v0/agent/chats/:id/rename?source=agent` identical across Task 4 (route) and Task 5 (`runChatRename`).
