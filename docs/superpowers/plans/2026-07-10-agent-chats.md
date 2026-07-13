# Agent Chats Implementation Plan

> For agentic workers: execute this plan with the `superpowers:subagent-driven-development` skill — one subagent per task, each task a self-contained TDD cycle ending in a green commit.

**Goal:** Ship Crowbar's agentic chats end-to-end: a workspace-scoped "Chats" sidebar tab that lists a workspace's agent chats (create-per-provider, select, rename, reorder, delete via drag-to-trash), a main-area chat pane that attaches the selected chat's live agent terminal with a provider-switch dropdown, plus the four backend additions that feed it (descriptor `icon`/`display_name`, `GET .../agent/providers`, `activeProviderId` on the chat DTO, re-derived workspace `working` overlay) and a centralized flip-dot spinner shared by the workspace icon and the chat row.

**Architecture:** The frontend mirrors the review-threads live list end-to-end. Because chats are per-workspace, state lives in the workspace store registry (`features/workspace/stores/`), not `lib/store/`. A REST client (`agent-api.ts`) wraps `apiFetch` + `workspaceBase(wsId)`; an immer slice (`agent-chats-slice.ts`) holds chats/working-map/order/providers; a WS hook (`use-workspace-agent-chats-stream.ts`) seeds via GET then react-then-refetches on bare lifecycle frames. The chat pane is a new pane content type (`'agentChat'`) that pre-seeds the terminal-store connection mapping so `resolveTerminalConnection` **attaches** the agent's live PTY instead of spawning a new one. Backend: descriptors gain display metadata (declarative YAML, no Go branching), an enumeration usecase backs a providers endpoint, the chat DTO derives its active provider from the active segment, and the workspace `Working` overlay is re-lit from an in-memory per-workspace turn set maintained by a third projection on `axAgentChat` (mirroring the `inflight` overlay).

**Tech Stack:** Backend Go (gin, char2cs/asynx event sourcing, YAML descriptors, embed.FS). Frontend React + TypeScript under **bun**, Zustand + immer stores, TanStack Router, xterm.js terminals, CossUI (`@/components/ui/*`) with shadcn/ui fallback, Tailwind CSS tokens, Vite (`import.meta.glob`).

---

## Global Constraints

Copied verbatim from the project-wide rules (root `CLAUDE.md` + the "no timing in tests" law). Every task must honour all of them.

- **Component file naming:** all component files are **kebab-case** (`agent-chat-row.tsx`, not `AgentChatRow.tsx`); the exported React component name stays **PascalCase** (`export function AgentChatRow()`).
- **Test location & imports:** all test files live in `web/src/__tests__/` **mirroring** the `web/src/` structure — a test for `web/src/features/X/lib/foo.ts` goes in `web/src/__tests__/features/X/lib/foo.test.ts`. Never create `features/X/tests/`. Use `@/` imports (never relative `../../`) inside test files.
- **Zustand:** use `useXxxStore((state) => state.specificField)` with a **narrow selector** — never `useXxxStore()` with no selector. Use `useXxxStore.getState()` **only** inside event handlers and `useEffect` bodies — never in the render path. Stores must **not** import from `components/`; move side effects (toasts, DOM) to components that watch store state via `useEffect`.
- **State placement:** per-workspace state lives in the workspace store registry (`features/workspace/stores/`); global app state in `features/window/stores/` or `features/settings/`; `lib/store/` is for server-state-adjacent structures.
- **UI components & tokens:** always reach for CossUI `@/components/ui/*` first, shadcn/ui as fallback; **CSS variable tokens only** — never hardcode colors.
- **Frontend runtime is bun:** run tests with `bun run test:coverage`, typecheck with `bun tsc --noEmit`. (pnpm is retired; `bun.lock` is the sole lockfile.)
- **Go tests are black-box:** `TestRegression_*` in `api/tests` with the `integration` build tag where relevant, driven through the real wired backend harness (`newHarness`).
- **NO timing in tests:** never sleep, never `Eventually/After/poll`. Block on real signals — asynx `WaitIdle`/`WaitPublish` (exposed as `h.Quiesce()` / `Repositories.WaitQuiescent()`), channels, and the harness `readUntil` frame loop (which advances on real WS frames, with `go test -timeout` / a read deadline as the only backstop).
- **Coverage & CI:** 100% coverage target on touched code; CI (`bun tsc --noEmit`, `prettier --check`, `go test`, lint) must be green before a task is considered done.

---

## Contract facts (verified — do not re-litigate)

- Agent routes are **workspace-scoped** under `.../projects/:projectId/repos/:repoId/workspaces/:wsId/agent/...`. The FE builds them with `workspaceBase(wsId)` (`web/src/lib/workspace-scope-url.ts`). Envelope `{success,error,data}` is unwrapped by `apiFetch`.
- WS frame `AgentChatEvent = {chatId, workspaceId, kind}`, **bare** (no snapshot). `kind ∈ created | segment_opened | segment_ended | session_bound | turn_started | turn_stopped | title_set | deleted`.
- `AgentSegment = {id, providerId, providerSessionId?, crowbarSegmentId, terminalSessionId, startedAt, endedAt?, status(active|ended)}`.
- `POST /agent/chats {provider}` → 201 `{id}`. `POST /agent/chats/:id/switch {provider}` → 200 `{id}`. `POST /agent/chats/:id/rename {title}` (+ `?source` default = user rename, locks) → 202. `DELETE /agent/chats/:id` → 202 (`PurgeChat`: PTY teardown + asynx Forget + on-disk `RemoveUnderHome`). **Delete needs no backend work.**
- Provider switch: the old segment ends, a new segment spawns with a **new** `terminalSessionId`; the FE pane must re-attach to the new session.
- A segment's `terminalSessionId` is a **live session in the same terminal-engine registry** the FE already attaches to (`engine/terminal` `CreateCommand` → `reg.Add`). No new bridge — pre-seed the terminal-store mapping so `resolveTerminalConnection` attaches instead of spawns.

---

## Task 1 — Descriptor `icon` + `display_name` (backend)

**Files**
- Modify: `api/internal/engine/agent/descriptor.go` (add `Icon`, `DisplayName` fields; relax `Validate`)
- Modify: `api/internal/engine/agent/descriptors/claude.yaml` (add `display_name` + real Claude SVG `icon`)
- Modify: `api/internal/engine/agent/descriptors/codex.yaml` (add `display_name` + real OpenAI/Codex SVG `icon`)
- Test: `api/internal/engine/agent/descriptor_test.go` (extend or create — same package, `package agent`)

**Interfaces**
- Produces: `Descriptor.Icon string`, `Descriptor.DisplayName string` (both `yaml:"icon"` / `yaml:"display_name"`), populated by `LoadDescriptor`.
- Consumes: nothing new (YAML only).

**Steps**

1. Write the failing test. Append to `descriptor_test.go`:

```go
func TestLoadDescriptor_ParsesDisplayMetadata(t *testing.T) {
	d, err := LoadDescriptor([]byte(`id: demo
display_name: Demo Provider
icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M1 1h1v1H1z"/></svg>'
spawn:
  cmd: "true"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: message }
`))
	require.NoError(t, err)
	require.Equal(t, "Demo Provider", d.DisplayName)
	require.Contains(t, d.Icon, "currentColor")
}

func TestValidate_DisplayFieldsAreOptional(t *testing.T) {
	// A descriptor with NO icon/display_name still validates: the display-only
	// carve-out must not break the "every engine field load-bearing" invariant.
	d, err := LoadDescriptor([]byte(`id: bare
spawn:
  cmd: "true"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: message }
`))
	require.NoError(t, err)
	require.Empty(t, d.Icon)
	require.Empty(t, d.DisplayName)
}
```

2. Run it — expected **FAIL** (compile error: `Icon`/`DisplayName` undefined):

```
cd api && go test ./internal/engine/agent/ -run 'TestLoadDescriptor_ParsesDisplayMetadata|TestValidate_DisplayFieldsAreOptional'
```

3. Minimal implementation. In `descriptor.go`, add the two fields to `Descriptor` (after `ID`) with a documented carve-out comment:

```go
type Descriptor struct {
	ID string `yaml:"id"`
	// DisplayName and Icon are the ONLY display-only fields on the descriptor
	// (the "every field is load-bearing" invariant's documented carve-out): they
	// are surfaced to the FE by GET .../agent/providers (dto.AgentProviderDTO) to
	// label and glyph the provider, and never influence spawn/hook behaviour.
	// Both are optional — Validate does not require them.
	DisplayName string `yaml:"display_name"`
	Icon        string `yaml:"icon"`
	Spawn       struct {
```

(`Validate` already only checks `ID`/`Spawn.Cmd`/`InteractiveRequired`/`Hooks`, so no change is needed there to keep them optional — the carve-out is that we deliberately add **no** requirement for the two new fields. Leave `Validate` as-is; the test `TestValidate_DisplayFieldsAreOptional` pins that.)

4. Source the **real** provider SVGs (do NOT invent them). Use `WebSearch`/`WebFetch` to fetch the official single-path logos — the Claude/Anthropic wordmark-or-glyph and the OpenAI/Codex glyph (e.g. from the official brand pages or the simple-icons slugs `anthropic` and `openai`). Strip each to one self-contained `<svg …><path …/></svg>`, set every `fill` to `currentColor` (so a theme token colors it), and inline it as a single-quoted YAML scalar. Add to `descriptors/claude.yaml` (after the `id: claude` line):

```yaml
display_name: Claude
icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><path d="REPLACE_WITH_REAL_SOURCED_ANTHROPIC_PATH"/></svg>'
```

and to `descriptors/codex.yaml` (after `id: codex`):

```yaml
display_name: Codex
icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><path d="REPLACE_WITH_REAL_SOURCED_OPENAI_PATH"/></svg>'
```

> The `REPLACE_WITH_REAL_SOURCED_*` `d` bytes MUST be the actually-fetched official path data — verify each renders as the real logo before committing. The test only asserts non-empty + `currentColor`, so getting the bytes right is a manual/live check (§Live verification).

5. Run — expected **PASS** for the unit tests. Also run the full engine package + the existing integration scope test to prove the YAML still loads:

```
cd api && go test ./internal/engine/agent/... && go test -tags integration ./tests/ -run TestAgentREST_Scope
```

6. **Commit:** `feat(agent): add display_name + icon to provider descriptors`

---

## Task 2 — Descriptor enumeration (`AllDescriptors`)

**Files**
- Modify: `api/internal/engine/agent/descriptors_embed.go` (add `AllDescriptors`)
- Test: `api/internal/engine/agent/descriptors_embed_test.go` (create — `package agent`)

**Interfaces**
- Consumes: the `embedded embed.FS` (`descriptors/*.yaml`) already declared in the file; on-disk `<homeDir>/descriptors/*.yaml`; existing `ResolveDescriptor(homeDir, id)`.
- Produces: `func AllDescriptors(homeDir string) ([]*Descriptor, error)` — every known provider descriptor, id-deduped (on-disk override wins per id, via `ResolveDescriptor`), sorted by `ID` (deterministic).

**Steps**

1. Failing test `descriptors_embed_test.go`:

```go
package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllDescriptors_EnumeratesEmbeddedProviders(t *testing.T) {
	got, err := AllDescriptors(t.TempDir()) // empty home → embedded only
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, d := range got {
		ids[d.ID] = true
	}
	require.True(t, ids["claude"], "claude descriptor must be enumerated")
	require.True(t, ids["codex"], "codex descriptor must be enumerated")
}

func TestAllDescriptors_OnDiskOverrideWinsAndAddsNewIds(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	// A brand-new on-disk-only provider id (future user-managed provider).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.yaml"), []byte(`id: extra
display_name: Extra
spawn:
  cmd: "true"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: message }
`), 0o644))

	got, err := AllDescriptors(home)
	require.NoError(t, err)
	byID := map[string]*Descriptor{}
	for _, d := range got {
		byID[d.ID] = d
	}
	require.Contains(t, byID, "extra", "an on-disk-only provider must appear in the enumeration")
	require.Equal(t, "Extra", byID["extra"].DisplayName)
	require.Contains(t, byID, "claude", "embedded providers still enumerate alongside on-disk ones")
	// Sorted by id, deterministic.
	for i := 1; i < len(got); i++ {
		require.Less(t, got[i-1].ID, got[i].ID, "AllDescriptors must be sorted by id")
	}
}
```

2. Run — expected **FAIL** (`AllDescriptors` undefined):

```
cd api && go test ./internal/engine/agent/ -run TestAllDescriptors
```

3. Implement in `descriptors_embed.go` (add imports `sort`, `strings`):

```go
// AllDescriptors enumerates every known provider descriptor: the ids embedded
// under descriptors/*.yaml PLUS any id present on disk at
// <homeDir>/descriptors/*.yaml (future user-managed providers), id-deduped with
// the on-disk override winning per id (each id is loaded through ResolveDescriptor,
// which already prefers the disk override), sorted by id for a deterministic feed.
// It backs GET .../agent/providers (the lazy-by-id ResolveDescriptor cannot list).
func AllDescriptors(homeDir string) ([]*Descriptor, error) {
	ids := map[string]struct{}{}

	entries, err := embedded.ReadDir("descriptors")
	if err != nil {
		return nil, fmt.Errorf("agent: list embedded descriptors: %w", err)
	}
	for _, e := range entries {
		if id, ok := descriptorID(e.Name()); ok {
			ids[id] = struct{}{}
		}
	}

	// On-disk overrides / additions. A missing dir is not an error (no overrides).
	if diskEntries, derr := os.ReadDir(filepath.Join(homeDir, "descriptors")); derr == nil {
		for _, e := range diskEntries {
			if e.IsDir() {
				continue
			}
			if id, ok := descriptorID(e.Name()); ok {
				ids[id] = struct{}{}
			}
		}
	}

	out := make([]*Descriptor, 0, len(ids))
	for id := range ids {
		d, err := ResolveDescriptor(homeDir, id)
		if err != nil {
			return nil, fmt.Errorf("agent: enumerate descriptor %q: %w", id, err)
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// descriptorID extracts the provider id from a "<id>.yaml" file name, or (,false)
// for a non-yaml entry.
func descriptorID(name string) (string, bool) {
	if !strings.HasSuffix(name, ".yaml") {
		return "", false
	}
	return strings.TrimSuffix(name, ".yaml"), true
}
```

4. Run — expected **PASS**:

```
cd api && go test ./internal/engine/agent/...
```

5. **Commit:** `feat(agent): enumerate provider descriptors (AllDescriptors)`

---

## Task 3 — `GET .../agent/providers` endpoint

**Files**
- Modify: `api/internal/api/v0/dto/agent.go` (add `AgentProviderDTO`)
- Modify: `api/internal/app/usecases/agent/agent.go` (add `ListProviders` usecase method)
- Create: `api/internal/api/v0/endpoints/agent/handlers/providers.go` (`Providers` handler)
- Modify: `api/internal/api/v0/endpoints/agent/handlers/handlers.go` (add `ListProviders` to `AgentUsecase` interface)
- Modify: `api/internal/api/v0/endpoints/agent/routes.go` (register `GET /agent/providers`)
- Modify: `api/internal/api/v0/endpoints/agent/handlers/hooks_test.go` (fake `fakeAgentUsecase` gains `ListProviders`)
- Test: `api/internal/api/v0/endpoints/agent/handlers/providers_test.go` (create — `package handlers_test`)
- Test: `api/tests/agent_providers_test.go` (create — integration black-box)

**Interfaces**
- Produces: `dto.AgentProviderDTO{ID, DisplayName, Icon string}` (`json:"id"|"displayName"|"icon"`).
- Produces: `(*agent.Usecase).ListProviders(ctx, workspaceID string) ([]engineagent.Descriptor, error)` — resolves crowbar home from the workspace, returns `engineagent.AllDescriptors(home)` (deref'd).
- Consumes (handler): `AgentUsecase.ListProviders(ctx, workspaceID string) ([]engineagent.Descriptor, error)`.

**Steps**

1. Add the DTO to `dto/agent.go`:

```go
// AgentProviderDTO is the wire shape of one registered agent provider (00
// agentic-engine spec §7.2): the id the FE passes back to create/switch, a
// human display name, and an inline SVG icon (fill="currentColor"). Backed by the
// descriptor enumeration; workspace-independent but served on the workspace-scoped
// route for surface consistency.
type AgentProviderDTO struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Icon        string `json:"icon"`
}
```

2. Failing handler test `providers_test.go`:

```go
package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agent/handlers"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
)

func TestProviders_Success(t *testing.T) {
	uc := &fakeAgentUsecase{providers: []engineagent.Descriptor{
		{ID: "claude", DisplayName: "Claude", Icon: "<svg/>"},
		{ID: "codex", DisplayName: "Codex", Icon: "<svg/>"},
	}}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws-1/agent/providers", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}}

	h.Providers(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)
	var env struct {
		Success bool `json:"success"`
		Data    []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Icon        string `json:"icon"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.True(t, env.Success)
	require.Len(t, env.Data, 2)
	assert.Equal(t, "claude", env.Data[0].ID)
	assert.Equal(t, "Claude", env.Data[0].DisplayName)
	assert.Equal(t, "ws-1", uc.listProvidersWorkspace)
}
```

Add the fake method + field to the shared `fakeAgentUsecase` in `hooks_test.go`:

```go
// (fields)
providers               []engineagent.Descriptor
providersErr            error
listProvidersWorkspace  string

func (f *fakeAgentUsecase) ListProviders(_ context.Context, workspaceID string) ([]engineagent.Descriptor, error) {
	f.listProvidersWorkspace = workspaceID
	return f.providers, f.providersErr
}
```

(import `engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"` in `hooks_test.go`.)

3. Run — expected **FAIL** (`h.Providers` undefined, `ListProviders` not on interface):

```
cd api && go test ./internal/api/v0/endpoints/agent/handlers/ -run TestProviders_Success
```

4. Implement. Add to the `AgentUsecase` interface in `handlers.go` (and its import block `engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"`):

```go
	// ListProviders enumerates the registered agent providers for the workspace
	// (the route ignores which workspace — the descriptor set is global — but the
	// usecase resolves crowbar home from it to read on-disk overrides).
	ListProviders(
		ctx context.Context,
		workspaceID string,
	) ([]engineagent.Descriptor, error)
```

Create `providers.go`:

```go
package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Providers handles GET .../workspaces/:wsId/agent/providers: the registered
// agent providers (id + display name + inline SVG icon) that back the chat row
// glyph, the New-chat rows, and the provider-switch menu. The :wsId path param is
// only used to resolve crowbar home for on-disk descriptor overrides — the
// provider set itself is workspace-independent (kept on the workspace group for
// surface consistency, 00 agentic-engine spec §7.2).
func (h *Handlers) Providers(
	ctx *gin.Context,
) {
	descs, err := h.usecase.ListProviders(ctx.Request.Context(), ctx.Param("wsId"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	out := make([]dto.AgentProviderDTO, 0, len(descs))
	for _, d := range descs {
		out = append(out, dto.AgentProviderDTO{ID: d.ID, DisplayName: d.DisplayName, Icon: d.Icon})
	}
	libs.WriteQueryOK(ctx, out)
}
```

Add the usecase method to `agent.go` (`engineagent` is already imported there):

```go
// ListProviders enumerates the registered agent providers for the workspace's
// crowbar home (embedded defaults + on-disk overrides), backing GET
// .../agent/providers. workspaceID is only used to resolve crowbar home — the
// descriptor set is global — so any workspace in the same home yields the same list.
func (u *Usecase) ListProviders(
	ctx context.Context,
	workspaceID string,
) ([]engineagent.Descriptor, error) {
	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("agent: list providers: worktree dir: %w", err)
	}
	descs, err := engineagent.AllDescriptors(crowbarHome)
	if err != nil {
		return nil, fmt.Errorf("agent: list providers: %w", err)
	}
	out := make([]engineagent.Descriptor, 0, len(descs))
	for _, d := range descs {
		out = append(out, *d)
	}
	return out, nil
}
```

Register the route in `routes.go` (add alongside the other GETs):

```go
	wsScoped.GET("/agent/providers", h.Providers)
```

5. Run the handler test — expected **PASS**. Then add the integration test `api/tests/agent_providers_test.go`:

```go
//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_AgentProvidersEndpoint proves GET .../agent/providers returns the
// enumerated providers (id/displayName/icon) so the FE can render the row glyph,
// the New-chat rows, and the switch menu without N per-chat fetches.
func TestRegression_AgentProvidersEndpoint(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h) // adds the "stub" provider on disk
	imported := importWritableWorkspace(t, h)

	var providers []struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		Icon        string `json:"icon"`
	}
	h.get(wsBase(imported)+"/agent/providers", &providers)

	ids := map[string]bool{}
	for _, p := range providers {
		ids[p.ID] = true
	}
	require.True(t, ids["claude"], "claude is an embedded provider")
	require.True(t, ids["codex"], "codex is an embedded provider")
	assert.True(t, ids["stub"], "an on-disk provider is also enumerated")
}
```

6. Run everything touched — expected **PASS**:

```
cd api && go test ./internal/api/v0/endpoints/agent/... ./internal/app/usecases/agent/... && go test -tags integration ./tests/ -run 'TestRegression_AgentProvidersEndpoint|TestAgentREST_Scope'
```

7. **Commit:** `feat(agent): GET workspace/:wsId/agent/providers`

---

## Task 4 — `activeProviderId` on `AgentChatDTO`

**Files**
- Modify: `api/internal/api/v0/dto/agent.go` (`ActiveProviderID` field + derive in `AgentChatDTOFrom`)
- Test: `api/internal/api/v0/dto/agent_test.go` (create or extend — `package dto`)
- Test: `api/tests/agent_active_provider_test.go` (create — integration)

**Interfaces**
- Produces: `AgentChatDTO.ActiveProviderID string` (`json:"activeProviderId"`), derived from the active segment; flows through `AgentChatDTOFrom` → `AgentChatDTOList` and `AgentChatDetailDTOFrom`.
- Consumes: `domain.AgentChat.Segments []AgentSegment` + `.ActiveSegmentID` + `AgentSegment.ProviderID`.

**Steps**

1. Failing unit test `agent_test.go`:

```go
package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestAgentChatDTOFrom_DerivesActiveProviderID(t *testing.T) {
	chat := domain.AgentChat{
		ID:              "c1",
		ActiveSegmentID: "s2",
		Segments: []domain.AgentSegment{
			{ID: "s1", ProviderID: "claude", Status: "ended"},
			{ID: "s2", ProviderID: "codex", Status: "active"},
		},
	}
	got := AgentChatDTOFrom(chat)
	assert.Equal(t, "codex", got.ActiveProviderID)
}

func TestAgentChatDTOFrom_EmptyWhenNoActiveSegment(t *testing.T) {
	got := AgentChatDTOFrom(domain.AgentChat{ID: "c1", ActiveSegmentID: ""})
	assert.Equal(t, "", got.ActiveProviderID)
}
```

2. Run — expected **FAIL** (`ActiveProviderID` undefined):

```
cd api && go test ./internal/api/v0/dto/ -run TestAgentChatDTOFrom
```

3. Implement. Add the field and a small helper in `dto/agent.go`:

```go
type AgentChatDTO struct {
	ID              string    `json:"id"`
	WorkspaceID     string    `json:"workspaceId"`
	Title           string    `json:"title"`
	ActiveSegmentID string    `json:"activeSegmentId"`
	// ActiveProviderID is the providerId of the currently active segment (00
	// agentic-engine spec §7.3), derived from the embedded segments so the chat
	// list can map a row to its provider glyph without N detail fetches. Empty
	// when the chat has no active segment.
	ActiveProviderID string    `json:"activeProviderId"`
	CreatedAt        time.Time `json:"createdAt"`
}

func AgentChatDTOFrom(
	c domain.AgentChat,
) AgentChatDTO {
	return AgentChatDTO{
		ID:               c.ID,
		WorkspaceID:      c.WorkspaceID,
		Title:            c.Title,
		ActiveSegmentID:  c.ActiveSegmentID,
		ActiveProviderID: activeProviderID(c),
		CreatedAt:        c.CreatedAt,
	}
}

// activeProviderID returns the providerId of c's active segment (the one whose id
// is c.ActiveSegmentID), or "" when there is none.
func activeProviderID(c domain.AgentChat) string {
	for _, s := range c.Segments {
		if s.ID == c.ActiveSegmentID {
			return s.ProviderID
		}
	}
	return ""
}
```

`AgentChatDTOList` and `AgentChatDetailDTOFrom` already delegate to `AgentChatDTOFrom`, so both surface the field automatically.

4. Run — expected **PASS**:

```
cd api && go test ./internal/api/v0/dto/
```

5. Integration test `agent_active_provider_test.go` (proves it flows over the real List + Detail routes):

```go
//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_AgentChatActiveProviderID proves both GET .../agent/chats (list)
// and GET .../agent/chats/:id (detail) carry activeProviderId derived from the
// active segment, so the FE row glyph resolves with no extra fetch.
func TestRegression_AgentChatActiveProviderID(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	chatID := createAgentChat(t, h, imported) // spawns provider "stub"

	var list []struct {
		ID               string `json:"id"`
		ActiveProviderID string `json:"activeProviderId"`
	}
	h.get(wsBase(imported)+"/agent/chats", &list)
	require.Len(t, list, 1)
	assert.Equal(t, chatID, list[0].ID)
	assert.Equal(t, "stub", list[0].ActiveProviderID)

	var detail struct {
		ActiveProviderID string `json:"activeProviderId"`
	}
	h.get(wsBase(imported)+"/agent/chats/"+chatID, &detail)
	assert.Equal(t, "stub", detail.ActiveProviderID)
}
```

6. Run — expected **PASS**:

```
cd api && go test ./internal/api/v0/dto/ && go test -tags integration ./tests/ -run TestRegression_AgentChatActiveProviderID
```

7. **Commit:** `feat(agent): expose activeProviderId on AgentChatDTO`

---

## Task 5 — Re-derive workspace `Working` from agent turns

**Files**
- Modify: `api/internal/app/repositories/container.go` (in-memory turn set, third `axAgentChat` projection, `enrichFrame` OR-in)
- Test: `api/tests/agent_working_overlay_test.go` (create — integration)

**Interfaces**
- Consumes: `axAgentChat` events (`asynxModels.Event[domain.AgentChat]`) — `evt.EventName` (`agentchat.<kind>.<id>`), `evt.AggregateID` (chatId), `evt.Aggregate.WorkspaceID`; `OnForget`.
- Produces: `enrichFrame` now sets `ws.Working = c.IsWorking(ws.ID) || c.agentWorkingFor(ws.ID)`; the affected workspace is re-broadcast on `turn_started`/`turn_stopped`/`deleted`.

**Design (concrete):** Mirror the existing `inflight` overlay. Keep an in-memory `agentWorking map[string]map[string]struct{}` (wsID → set of chatIds currently mid-turn), guarded by the existing `c.mu`. A third projection on `axAgentChat` (alongside the store + hub projections) folds `turn_started` (add) / `turn_stopped` (remove) and `OnForget` (remove — a chat deleted mid-turn must not wedge the spinner on), then re-broadcasts the workspace through the SAME `rebroadcast`→`enrichFrame` path the `inflight` overlay uses. The set is authoritative and race-free (no read-model query), so ordering between the store and working projections never matters.

> **Boot note (in-memory overlay, empty on restart).** `agentWorking` is in-memory only and starts **empty** on daemon boot — a chat that was mid-turn when the daemon stopped is shown idle until its next `turn_started`. This is safe because boot-reconcile (`agent.Usecase.ReconcileOnBoot`, run from `app/container.go`) ends stale segments/turns on restart (`segment_ended` / `turn_stopped → idle`), so no chat is genuinely mid-turn-but-shown-idle after boot — consistent with the FE chat-row working map's accepted default-idle-on-load.

**Steps**

1. Failing integration test `agent_working_overlay_test.go`. It needs a **long-lived** stub (so the segment stays active while hooks fire). Add a helper next to `writeStubProviderDescriptor`:

```go
//go:build integration

package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// liveStubProviderDescriptorYAML spawns `cat`, which stays alive on its PTY
// (no stdin EOF) so the segment remains active while user_prompt/turn_stop hooks
// fire — unlike the `true` stub, which exits instantly and ends its segment.
const liveStubProviderDescriptorYAML = `id: livestub
spawn:
  cmd: "cat"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt: { message: prompt }
    turn_stop: { session_id: session_id, message: last_assistant_message }
`

func writeLiveStubProviderDescriptor(t *testing.T, h *harness) {
	t.Helper()
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "livestub.yaml"), []byte(liveStubProviderDescriptorYAML), 0o644))
}

// TestRegression_WorkspaceWorkingReflectsAgentTurn proves the workspace `working`
// overlay (domain.Workspace.Working) is re-lit from live agent turns: a
// user_prompt hook (→ turn_started) re-broadcasts the workspace with working=true,
// and a turn_stop hook (→ turn_stopped) re-broadcasts it with working=false.
func TestRegression_WorkspaceWorkingReflectsAgentTurn(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	var created struct {
		ID string `json:"id"`
	}
	h.post(wsBase(imported)+"/agent/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	h.Quiesce()

	var detail struct {
		Segments []struct {
			CrowbarSegmentID string `json:"crowbarSegmentId"`
		} `json:"segments"`
	}
	h.get(wsBase(imported)+"/agent/chats/"+created.ID, &detail)
	require.NotEmpty(t, detail.Segments)
	segID := detail.Segments[0].CrowbarSegmentID

	conn := h.dial(repoBase + "/workspaces")

	// user_prompt opens the turn.
	_ = h.raw(http.MethodPost, wsBase(imported)+"/agent/hooks", map[string]string{
		"segment_id": segID, "provider": "livestub", "event": "user_prompt",
		"payload_raw": `{"prompt":"hi"}`,
	}, http.StatusAccepted).Body.Close()
	readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == imported.workspaceID && m["working"] == true
	})

	// turn_stop closes it.
	_ = h.raw(http.MethodPost, wsBase(imported)+"/agent/hooks", map[string]string{
		"segment_id": segID, "provider": "livestub", "event": "turn_stop",
		"payload_raw": `{"last_assistant_message":"done"}`,
	}, http.StatusAccepted).Body.Close()
	readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == imported.workspaceID && m["working"] == false
	})
}
```

2. Run — expected **FAIL** (the workspace frame never carries `working==true`; the overlay is always false today):

```
cd api && go test -tags integration ./tests/ -run TestRegression_WorkspaceWorkingReflectsAgentTurn
```

3. Implement in `container.go`. Add the import `asynxModels "github.com/char2cs/asynx/models"` (asynx + strings already imported). Add the field to `Container` (next to `inflight`):

```go
	// agentWorking maps a workspace id to the set of its agent chats currently
	// mid-turn (00 agentic-engine spec §7.4). It is the agent-turn counterpart to
	// inflight: enrichFrame ORs it into the derived Working overlay, and the
	// registerAgentWorkingProjection folds turn_started/turn_stopped/forget into
	// it and re-broadcasts the affected workspace. Guarded by mu.
	agentWorking map[string]map[string]struct{}
```

Initialise it in `New` (in the `c := &Container{...}` literal): `agentWorking: map[string]map[string]struct{}{},`.

Register the projection in `New` right after `c.AgentChat = agentChat`:

```go
	if err := c.registerAgentWorkingProjection(); err != nil {
		return nil, fmt.Errorf("repositories: agent working projection: %w", err)
	}
```

Add the methods:

```go
// registerAgentWorkingProjection subscribes a THIRD projection on axAgentChat
// (alongside the store + hub projections built in NewEventSourced): it re-derives
// the per-workspace Working overlay from agent turn events (00 §7.4). turn_started
// marks the chat working, turn_stopped clears it, and a Forget of a chat mid-turn
// clears it too (so a delete never wedges the spinner on); each transition
// re-broadcasts the affected workspace through the same enrichFrame path the
// inflight overlay uses, so the FE spinner on the workspace tree + context pill +
// tiles tracks live agent activity. The in-memory set is authoritative (not a
// read-model query), so it never races the store projection.
func (c *Container) registerAgentWorkingProjection() error {
	if _, err := c.axAgentChat.Subscribe(asynx.Topic("agentchat.*"),
		func(ctx context.Context, evt asynxModels.Event[domain.AgentChat]) {
			wsID := evt.Aggregate.WorkspaceID
			if wsID == "" {
				return
			}
			switch agentEventKind(evt.EventName) {
			case "turn_started":
				c.setAgentTurn(wsID, evt.AggregateID, true)
			case "turn_stopped":
				c.setAgentTurn(wsID, evt.AggregateID, false)
			default:
				return
			}
			c.rebroadcast(ctx, wsID)
		}); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	if _, err := c.axAgentChat.OnForget(
		func(ctx context.Context, evt asynxModels.Event[domain.AgentChat]) {
			wsID := evt.Aggregate.WorkspaceID
			if wsID == "" {
				return
			}
			c.setAgentTurn(wsID, evt.AggregateID, false)
			c.rebroadcast(ctx, wsID)
		}); err != nil {
		return fmt.Errorf("onforget: %w", err)
	}
	return nil
}

// setAgentTurn adds/removes chatID from the workspace's mid-turn set under mu.
func (c *Container) setAgentTurn(wsID, chatID string, working bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if working {
		set := c.agentWorking[wsID]
		if set == nil {
			set = map[string]struct{}{}
			c.agentWorking[wsID] = set
		}
		set[chatID] = struct{}{}
		return
	}
	if set := c.agentWorking[wsID]; set != nil {
		delete(set, chatID)
		if len(set) == 0 {
			delete(c.agentWorking, wsID)
		}
	}
}

// agentWorkingFor reports whether the workspace has any agent chat mid-turn.
func (c *Container) agentWorkingFor(wsID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.agentWorking[wsID]) > 0
}

// agentEventKind extracts <kind> from an agentchat EventName ("agentchat.<kind>.<id>").
func agentEventKind(eventName string) string {
	rest := strings.TrimPrefix(eventName, "agentchat.")
	kind, _, found := strings.Cut(rest, ".")
	if !found {
		return rest
	}
	return kind
}
```

OR the overlay into `enrichFrame`:

```go
func (c *Container) enrichFrame(
	ctx context.Context,
	ws domain.Workspace,
) dto.WorkspaceDTO {
	ws.Working = c.IsWorking(ws.ID) || c.agentWorkingFor(ws.ID)
	elig := c.eligibilityFor(ctx, ws)
	return dto.WorkspaceDTOFrom(ws, elig)
}
```

Also OR it into the two snapshot readers so a snapshot taken mid-turn agrees with the live frame — in `ListWorkspaces` and `ListWorkspacesInRepo` change `rows[i].Working = c.IsWorking(rows[i].ID)` to `rows[i].Working = c.IsWorking(rows[i].ID) || c.agentWorkingFor(rows[i].ID)`.

4. Run — expected **PASS**. Also run the repositories package tests (the new field/projection must not break existing container tests):

```
cd api && go test ./internal/app/repositories/... && go test -tags integration ./tests/ -run TestRegression_WorkspaceWorkingReflectsAgentTurn
```

5. **Commit:** `feat(workspace): re-derive working overlay from agent turns`

---

## Task 6 — Home-workspace agent chats end-to-end: mount agent REST + WS under the home group **and** build the home-scoped CLI callback URL

> **Gap discovered during planning — required for the spec's CRITICAL "Chats works for ALL workspace kinds" requirement (project-home specifically).** Two distinct breakages must both be fixed for a **project-home** workspace, and they are cured together here:
>
> 1. **Daemon side (mount).** Today `agent.Register` mounts only on `wsScoped` (`.../workspaces/:wsId/agent/...`). A **home** workspace resolves `workspaceBase(wsId)` → `/v0/projects/:p/home` (see `web/src/lib/workspace-scope-url.ts`), and `home.Register` mounts files/threads/terminals but **not** agent — so `${workspaceBase(homeWsId)}/agent/chats` would 404 for a project-home workspace. Mirror the threads/terminals pattern (`RequireHomeWorkspace` injects the resolved `:wsId`) to serve the agent surface under `/home` too.
> 2. **CLI-callback side (URL build).** The spawned vendor CLI's in-PTY callbacks (`crowbar hook …`, `crowbar chat rename …`, `crowbar handoff …`) build their daemon URL via `cmd/crowbar/scope.go`'s `scopedAgentPath(project, repo, workspace, suffix)`, interpolating the `{project_id}/{repo_id}/{workspace_id}` the descriptor commands carry (see `engine/agent/template.go` + `descriptors/*.yaml`). For a **project-home** workspace `agentWorkspaceReader.WorktreeDir` returns an **empty `RepoID`** (the project-level home has no repo id — see the `AgentChatsDir` doc: "the project-level home … has no repo id to resolve a slug from"), so `scopedAgentPath` emits `/v0/projects/p/repos//workspaces/ws/agent/hooks` → **404**. Repo-home is fine (Kind=git / IsDefault, carries a repo id) and worktrees are fine; **only project-home is broken** — its `user_prompt`→`turn_started`, its agent-derived titles, and its `session_start` binding never reach the daemon, breaking the working overlay + spinner + titles for project-home. Add a **home branch** to `scopedAgentPath`: when `repo == ""`, emit `/v0/projects/{project}/home/agent{suffix}`, exactly matching the home-group mount added in this task.
>
> These are complementary: (1) makes the daemon *serve* the home agent surface; (2) makes the CLI *address* it. Neither alone makes project-home chats work. Task 5's overlay test only exercises a worktree (`importWritableWorkspace`), so it cannot catch either gap; this task adds a project-home hook integration test that does. If a reviewer decides home chats are out of MVP scope, skip this task and instead gate the FE Chats tab off home routes — but that contradicts the spec, so implement it.

**Files**
- Modify: `api/internal/api/v0/endpoints/home/routes.go` (mount agent routes under the `home` group with `RequireHomeWorkspace`; widen `Register`'s signature to accept the agent usecase + WS handle)
- Modify: `api/internal/api/v0/router.go` (thread `c.app.Usecases.Agent` + `c.agentChats.Handle` into `homePkg.Register`)
- Modify: `api/cmd/crowbar/scope.go` (add the `repo == ""` home branch to `scopedAgentPath`)
- Test: `api/cmd/crowbar/scope_test.go` (extend — home-path unit tests; `package main`)
- Test: `api/tests/agent_home_scope_test.go` (create — integration: home REST reachable)
- Test: `api/tests/agent_home_hook_test.go` (create — integration: CLI callbacks reach the daemon for a project-home workspace)

**Interfaces**
- Consumes: the same `agenthandlers.AgentUsecase` + WS `gin.HandlerFunc` already passed to `agent.Register`; `cobra` (unchanged) for the CLI callback flags.
- Produces (daemon): `GET/POST/DELETE /v0/projects/:projectId/home/agent/chats[...]`, `POST /v0/projects/:projectId/home/agent/hooks`, `GET /v0/projects/:projectId/home/agent/providers`, and `GET /v0/projects/:projectId/home/agent/ws/chats`, each `RequireHomeWorkspace`-scoped so the injected `:wsId` is the project's home workspace.
- Produces (CLI): `scopedAgentPath(project, "", workspace, suffix)` now returns `/v0/projects/{project}/home/agent{suffix}` (the home mount above); a non-empty `repo` still returns the `.../repos/{repo}/workspaces/{workspace}/agent{suffix}` worktree/repo-home path. Same function backs all three callbacks (`/hooks`, `/chats/:id/rename?source=agent`, `/chats/:id/handoff`) in `hook.go`/`chat.go`/`handoff.go` — no per-callback change needed.

**Steps**

1. Failing integration test `agent_home_scope_test.go`:

```go
//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_AgentChatsWorkOnHomeWorkspace proves the agent chat surface is
// reachable under the project-home group (not only under .../workspaces/:wsId),
// so the FE Chats tab works for a home workspace exactly as the spec requires.
func TestRegression_AgentChatsWorkOnHomeWorkspace(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	homeBase := "/v0/projects/" + imported.projectID + "/home"

	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/agent/chats", map[string]string{"provider": "stub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.Quiesce()

	var list []struct {
		ID string `json:"id"`
	}
	h.get(homeBase+"/agent/chats", &list)
	require.Len(t, list, 1)
	assert.Equal(t, created.ID, list[0].ID)

	var providers []struct {
		ID string `json:"id"`
	}
	h.get(homeBase+"/agent/providers", &providers)
	require.NotEmpty(t, providers)
}
```

2. Run — expected **FAIL** (404: `POST /home/agent/chats` not mounted):

```
cd api && go test -tags integration ./tests/ -run TestRegression_AgentChatsWorkOnHomeWorkspace
```

3. Implement. In `home`'s `Register`, add the agent handler and routes mirroring the threads block (each route `RequireHomeWorkspace`-scoped so the injected `:wsId` resolves the home workspace). Construct the agent handlers with the injected usecase and mount:

```go
	ah := agenthandlers.New(agentUsecase) // agenthandlers "…/endpoints/agent/handlers"
	home.POST("/agent/chats", h.RequireHomeWorkspace, ah.Create)
	home.GET("/agent/chats", h.RequireHomeWorkspace, ah.List)
	home.GET("/agent/chats/:id", h.RequireHomeWorkspace, ah.Get)
	home.POST("/agent/chats/:id/switch", h.RequireHomeWorkspace, ah.Switch)
	home.POST("/agent/chats/:id/rename", h.RequireHomeWorkspace, ah.Rename)
	home.GET("/agent/chats/:id/handoff", h.RequireHomeWorkspace, ah.Handoff)
	home.DELETE("/agent/chats/:id", h.RequireHomeWorkspace, ah.Delete)
	home.POST("/agent/hooks", h.RequireHomeWorkspace, ah.Hooks)
	home.GET("/agent/providers", h.RequireHomeWorkspace, ah.Providers)
	home.GET("/agent/ws/chats", h.RequireHomeWorkspace, agentWS)
```

Widen `home.Register`'s signature to accept `agentUsecase agenthandlers.AgentUsecase` and `agentWS gin.HandlerFunc`, and pass them from `router.go`:

```go
	homePkg.Register(
		projectScoped,
		c.app.Repositories.Workspace,
		c.app.GORM.Projects,
		c.app.Usecases.File,
		c.eng.Terminal,
		c.files.Handle,
		c.app.Repositories.ReviewThread,
		c.threads,
		c.threads.Handle,
		c.app.Usecases.Agent,   // NEW
		c.agentChats.Handle,     // NEW
		ws.DualServe,
	)
```

> Note: the home group already registers a `.../home` route with no `:wsId` segment; `RequireHomeWorkspace` sets `ctx.Param("wsId")` to the resolved home workspace, which the agent handlers read exactly like every other home-scoped handler. The agent WS `agentChatDef` filter keys on the injected `:wsId`, so a home client only sees the home workspace's chats.

4. Run — expected **PASS**. Also re-run the workspace-scoped scope test to prove no regression:

```
cd api && go test -tags integration ./tests/ -run 'TestRegression_AgentChatsWorkOnHomeWorkspace|TestAgentREST_Scope'
```

5. **Commit (1 of 2):** `feat(agent): mount agent chat surface under the home group`

---

Now cure the CLI-callback side (breakage #2 in the task preamble). The daemon serves the home agent surface after commit 1, but the in-PTY CLI still builds a `/repos//workspaces/...` URL for a project-home workspace.

6. Failing unit test — extend `api/cmd/crowbar/scope_test.go` (same `package main`; it already has `TestScopedAgentPath` for the worktree case, which must stay green):

```go
func TestScopedAgentPath_HomeWorkspaceHasNoRepo(t *testing.T) {
	// A project-home workspace resolves an EMPTY repo id (WorktreeDir returns ""
	// for the project-level home — see agentWorkspaceReader.AgentChatsDir's doc).
	// The callback must target the home-group mount that home.Register serves
	// (added in commit 1), NOT /repos//workspaces/.../agent, which 404s. This is
	// the project-home half of the spec's CRITICAL "chats work for ALL workspace
	// kinds" requirement.
	got := scopedAgentPath("p1", "", "home-ws", "/hooks")
	want := "/v0/projects/p1/home/agent/hooks"
	if got != want {
		t.Fatalf("scopedAgentPath(home) = %q, want %q", got, want)
	}
}

func TestScopedAgentPath_HomeSuffixesCompose(t *testing.T) {
	// The home branch must compose with every callback suffix — the hook
	// (hook.go), the agent rename with its ?source=agent query (chat.go), and the
	// handoff dump (handoff.go) — each landing on the matching home-group route.
	cases := map[string]string{
		"/hooks":                        "/v0/projects/p1/home/agent/hooks",
		"/chats/c1/rename?source=agent": "/v0/projects/p1/home/agent/chats/c1/rename?source=agent",
		"/chats/c1/handoff":             "/v0/projects/p1/home/agent/chats/c1/handoff",
	}
	for suffix, want := range cases {
		if got := scopedAgentPath("p1", "", "home-ws", suffix); got != want {
			t.Fatalf("scopedAgentPath(home, %q) = %q, want %q", suffix, got, want)
		}
	}
}
```

Run — expected **FAIL** (the empty-repo path is still `/v0/projects/p1/repos//workspaces/home-ws/agent/hooks`):

```
cd api && go test ./cmd/crowbar/ -run 'TestScopedAgentPath'
```

7. Implement the home branch in `api/cmd/crowbar/scope.go`:

```go
// scopedAgentPath builds the agent API path for the given project/repo/workspace
// ids, appending suffix (which may carry its own path segments and query string)
// after the .../agent segment.
//
// A project-level HOME workspace has NO repo id: agentWorkspaceReader.WorktreeDir
// returns an empty RepoID for it (the project-level home "has no repo id to
// resolve a slug from"; see usecases/container.go AgentChatsDir). Its agent
// surface is mounted under the home group (/v0/projects/:projectId/home/agent/...)
// by home.Register — NOT under .../repos/:repoId/workspaces/:wsId — so with an
// empty repo we must emit the HOME path or the in-PTY callbacks (hook, chat
// rename, handoff dump) would 404 on /repos//workspaces/.../agent. Repo-home
// (Kind=git / IsDefault) and worktrees both carry a repo id and take the
// workspace-scoped branch below.
func scopedAgentPath(
	project, repo, workspace, suffix string,
) string {
	if repo == "" {
		return "/v0/projects/" + project + "/home/agent" + suffix
	}
	return "/v0/projects/" + project + "/repos/" + repo + "/workspaces/" + workspace + "/agent" + suffix
}
```

Run — expected **PASS** (both new cases and the pre-existing `TestScopedAgentPath` worktree case):

```
cd api && go test ./cmd/crowbar/
```

8. Prove the two halves meet end-to-end: a **project-home** hook integration test `api/tests/agent_home_hook_test.go`. It reuses `writeLiveStubProviderDescriptor` (the long-lived `cat` stub from Task 5, so the segment stays active while hooks fire) and the existing `dialAgentWS` / `waitForChatFrame` helpers (channel-fed, backstopped only by `t.Context().Done()` — no sleeps/polls). The hook body shape mirrors `cmd/crowbar/hook.go`'s `runHook` (`segment_id`/`provider`/`event`/`payload_raw`); the HTTP POST to `homeBase+"/agent/hooks"` is the exact URL `scopedAgentPath(project, "", workspace, "/hooks")` now builds for project-home:

```go
//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRegression_AgentHomeCallbacksReachDaemon proves the in-PTY CLI callbacks
// for a PROJECT-HOME workspace land on the home-group agent mount and are
// processed by the daemon. A project-home workspace resolves an EMPTY repo id, so
// scope.go builds its callback URLs as /v0/projects/:projectId/home/agent/...
// (the home branch), and home.Register serves them. This fires the hook + rename
// callbacks against that home path and asserts the agent lifecycle WS emits the
// resulting session_bound / turn_started / turn_stopped / title_set frames — the
// live signals the FE working overlay, chat-row spinner, and derived titles rely
// on. Without BOTH the scope.go home branch (commit 2) AND the home mount
// (commit 1) these 404, so this is the guard for the spec's CRITICAL "chats work
// for ALL workspace kinds" (project-home) requirement that Task 5's worktree-only
// overlay test cannot catch.
func TestRegression_AgentHomeCallbacksReachDaemon(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h) // cat stays alive so the segment is active for the turn
	imported := importProject(t, h)
	homeBase := "/v0/projects/" + imported.projectID + "/home"

	// Dial the HOME agent lifecycle WS BEFORE creating so no frame is missed. The
	// agentChatDef filter keys on the RequireHomeWorkspace-injected :wsId, so this
	// connection sees exactly the project-home workspace's chats.
	frames := dialAgentWS(t, h, homeBase+"/agent/ws/chats")

	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/agent/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	waitForChatFrame(t, frames, created.ID, "created")

	h.Quiesce()
	var detail struct {
		Segments []struct {
			CrowbarSegmentID string `json:"crowbarSegmentId"`
		} `json:"segments"`
	}
	h.get(homeBase+"/agent/chats/"+created.ID, &detail)
	require.NotEmpty(t, detail.Segments)
	segID := detail.Segments[0].CrowbarSegmentID

	// session_start hook → the provider session binds (session_bound). Proves the
	// project-home /agent/hooks callback (repo=="" ⇒ home path) reaches the daemon.
	postHomeHook(t, h, homeBase, segID, "session_start", `{"session_id":"sess-home-1"}`)
	waitForChatFrame(t, frames, created.ID, "session_bound")

	// user_prompt hook → the turn opens (turn_started): the working overlay + chat
	// spinner signal, now proven for a PROJECT-HOME workspace.
	postHomeHook(t, h, homeBase, segID, "user_prompt", `{"prompt":"hi"}`)
	waitForChatFrame(t, frames, created.ID, "turn_started")

	// turn_stop hook → the turn closes (turn_stopped).
	postHomeHook(t, h, homeBase, segID, "turn_stop", `{"last_assistant_message":"done"}`)
	waitForChatFrame(t, frames, created.ID, "turn_stopped")

	// Agent rename callback (?source=agent) → title_set: the derived-title path,
	// which for project-home builds /home/agent/chats/:id/rename via scope.go too.
	resp := h.raw(http.MethodPost, homeBase+"/agent/chats/"+created.ID+"/rename?source=agent",
		map[string]string{"title": "Derived home title"}, http.StatusAccepted)
	_ = resp.Body.Close()
	waitForChatFrame(t, frames, created.ID, "title_set")
}

// postHomeHook forwards a raw hook payload to the project-home agent hooks
// endpoint exactly as the in-PTY `crowbar hook` callback does (body shape from
// cmd/crowbar/hook.go's runHook), and asserts the daemon accepts it (202). It is
// the HTTP-level stand-in for the vendor CLI's callback, whose URL scope.go now
// builds as the home path when the workspace has no repo id.
func postHomeHook(
	t *testing.T,
	h *harness,
	homeBase, segID, event, payloadRaw string,
) {
	t.Helper()
	resp := h.raw(http.MethodPost, homeBase+"/agent/hooks", map[string]string{
		"segment_id":  segID,
		"provider":    "livestub",
		"event":       event,
		"payload_raw": payloadRaw,
	}, http.StatusAccepted)
	_ = resp.Body.Close()
}
```

Run — expected **PASS**, and re-run the workspace-scoped scope test + Task 5's worktree overlay test to prove no regression:

```
cd api && go test ./cmd/crowbar/ && go test -tags integration ./tests/ -run 'TestRegression_AgentHomeCallbacksReachDaemon|TestRegression_AgentChatsWorkOnHomeWorkspace|TestRegression_WorkspaceWorkingReflectsAgentTurn|TestAgentREST_Scope'
```

9. **Commit (2 of 2):** `fix(agent): build home-scoped agent callback URL when workspace has no repo id`

---

## Task 7 — New flip-dot `<FlickerSpinner>` component (flicker)

**Files**
- Create: `web/src/components/ui/flicker-spinner.tsx` (the new flip-dot component — a **distinct** file, NOT a change to `spinner.tsx`)
- Create: `web/src/components/ui/spinners/pacman.svg` (the template; plus the rest captured during implementation)
- Create: `web/src/components/ui/spinners/framer-loading.svg` (second real spinner)
- Test: `web/src/__tests__/components/ui/flicker-spinner.test.tsx`

**Interfaces**
- Produces: `export function FlickerSpinner(props: React.ComponentProps<'span'>): React.ReactElement` — renders a **random** flip-dot SVG (per instance) inlined so `currentColor` + the SVG's declarative `<animate>` both work. Sizes via `className` (default `size-4`); color inherits `currentColor`.
- Consumes: `import.meta.glob('./spinners/*.svg', { eager: true, query: '?raw', import: 'default' })` — resolved **relative to `flicker-spinner.tsx`** (both `flicker-spinner.tsx` and the `spinners/` folder live in `web/src/components/ui/`, so `./spinners/*.svg` → `web/src/components/ui/spinners/*.svg`).

> **Note (scope — deliberately NOT a blast radius):** approved spec §6 scopes the flicker spinner to the **workspace icon and the chat row ONLY**. The generic `components/ui/spinner.tsx` (the lucide `Loader2` spinner) is **left untouched** so its consumers `components/ui/button.tsx` and `components/ui/loading-spinner.tsx` keep their existing loading spinner. The flip-dot lives in a **distinctly-named** new component `FlickerSpinner` (`@/components/ui/flicker-spinner`) so no app button/loading state is affected. `FlickerSpinner` inherits `currentColor`; the two consumers (`WorkspaceBranchIcon` Task 8, `AgentChatRow` Task 14) wrap it in a `text-primary` span. Recorded in Self-Review.

**Steps**

1. Author the PACMAN template SVG at `web/src/components/ui/spinners/pacman.svg`. It is a **self-animating** 5×5 flip-dot: 25 `<circle r="2" fill="currentColor">` in a 30×30 grid (cx/cy ∈ {3,9,15,21,27}, row-major), each with a discrete `<animate>` over the 10 captured PACMAN frames (`1` → opacity 1, `0` → opacity 0.14 so the "off" dot stays a faint flip-dot). This exact file:

```xml
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 30 30" fill="none" aria-hidden="true">
  <circle cx="3" cy="3" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="1;1;0.14;0.14;0.14;0.14;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="9" cy="3" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="1;1;1;0.14;0.14;0.14;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="15" cy="3" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;1;1;1;0.14;0.14;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="21" cy="3" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;0.14;0.14;1;1;0.14;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="27" cy="3" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;0.14;0.14;1;1;1;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="3" cy="9" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="1;1;1;0.14;0.14;0.14;0.14;0.14;0.14;1"/></circle>
  <circle cx="9" cy="9" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;1;1;1;0.14;0.14;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="15" cy="9" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;0.14;1;1;1;0.14;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="21" cy="9" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;0.14;1;1;1;1;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="27" cy="9" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;0.14;0.14;0.14;1;1;1;0.14;0.14;0.14"/></circle>
  <circle cx="3" cy="15" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;1;1;0.14;0.14;0.14;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="9" cy="15" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;0.14;1;1;0.14;0.14;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="15" cy="15" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="1;1;1;1;1;0.14;0.14;0.14;1;1"/></circle>
  <circle cx="21" cy="15" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;0.14;0.14;0.14;1;1;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="27" cy="15" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;0.14;0.14;0.14;1;1;1;0.14;0.14;0.14"/></circle>
  <circle cx="3" cy="21" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="1;1;1;0.14;0.14;0.14;0.14;0.14;0.14;1"/></circle>
  <circle cx="9" cy="21" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;1;1;1;0.14;0.14;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="15" cy="21" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;0.14;1;1;1;0.14;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="21" cy="21" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;0.14;1;1;1;1;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="27" cy="21" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;0.14;0.14;0.14;1;1;1;0.14;0.14;0.14"/></circle>
  <circle cx="3" cy="27" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="1;1;0.14;0.14;0.14;0.14;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="9" cy="27" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="1;1;1;0.14;0.14;0.14;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="15" cy="27" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;1;1;1;0.14;0.14;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="21" cy="27" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;0.14;0.14;1;1;0.14;0.14;0.14;0.14;0.14"/></circle>
  <circle cx="27" cy="27" r="2" fill="currentColor"><animate attributeName="opacity" calcMode="discrete" dur="1s" repeatCount="indefinite" values="0.14;0.14;0.14;1;1;1;0.14;0.14;0.14;0.14"/></circle>
</svg>
```

**Construction rule (apply to every spinner):** for frames `F = [f0…fN-1]` (each a 25-char row-major on/off string), cell `i` (0..24) sits at `cx = {3,9,15,21,27}[i%5]`, `cy = {3,9,15,21,27}[floor(i/5)]`, and its `<animate values>` is the `i`-th char of each frame joined by `;`, mapping `1`→`1` and `0`→`0.14`, with `calcMode="discrete" dur="1s" repeatCount="indefinite"` (10 frames ⇒ 100 ms/frame; scale `dur` to keep ~100 ms/frame for other frame counts). Deriving the SVG from frame data with a **throwaway local one-liner is fine — the committed artifacts are static `.svg` files; there is NO build-time codegen or index to maintain** (the glob discovers them).

2. Author a second real spinner `framer-loading.svg` from its captured frames (same rule), so the random pick has ≥2 options and the glob is proven with more than one file. FRAMER LOADING frames (13 frames):

```
["0001100001000001000011000","0011000011000010000010000","0110000110000110000100000","1100001100001100001100001","1000011000011000011000011","0000110000110000110000110","0001100001100001100001100","0011000011000011000011000","0110000110000110000110000","1100001100001100001100001","1000011000011000011000011","0000010000110000110000110","0000100000100001100001100"]
```

> During implementation, capture the remaining ~29 spinners from `flicker.laurie.fyi/gallery` via browser eval: for each 5×5 grid, sample its `<circle>` fills (`on` ≈ `#F5F5F5` → `1`, `off` ≈ `#404040` → `0`; positions `cx`/`cy ∈ {3,9,15,21,27}`, `viewBox "0 0 30 30"`, `r=2`), collect the per-frame strings, and author one `.svg` per spinner with the construction rule above. Dropping a new `.svg` into the folder = a new spinner, zero code change.

3. Failing test `web/src/__tests__/components/ui/flicker-spinner.test.tsx`:

```tsx
import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'

describe('FlickerSpinner', () => {
  it('inlines a flip-dot SVG so currentColor + <animate> work', () => {
    const { container } = render(<FlickerSpinner />)
    const svg = container.querySelector('svg')
    expect(svg).not.toBeNull()
    // Inlined (not an <img>): currentColor dots + an <animate> element are present.
    expect(svg!.querySelector('animate')).not.toBeNull()
    expect(svg!.querySelector('[fill="currentColor"]')).not.toBeNull()
  })

  it('exposes a status role and honours className sizing', () => {
    const { getByRole } = render(<FlickerSpinner className="size-3.5" />)
    const el = getByRole('status')
    expect(el.className).toContain('size-3.5')
  })
})
```

4. Run — expected **FAIL** (module `@/components/ui/flicker-spinner` does not exist yet):

```
cd web && bun run test:coverage -- flicker-spinner.test.tsx
```

5. Implement `components/ui/flicker-spinner.tsx`:

```tsx
import { useState } from 'react'
import type React from 'react'
import { cn } from '@/lib/utils'

// Discover every flip-dot spinner SVG as raw markup at build time. Inlining the
// markup (not an <img src>) is load-bearing: it lets each SVG's fill="currentColor"
// dots inherit the Crowbar theme token and lets its declarative <animate> run.
// Adding a spinner = drop an .svg in ./spinners — no index, no codegen.
const SPINNERS = Object.values(
  import.meta.glob('./spinners/*.svg', { eager: true, query: '?raw', import: 'default' }),
) as string[]

export function FlickerSpinner({ className, ...props }: React.ComponentProps<'span'>): React.ReactElement {
  // Random-pick one spinner per instance (mirrors the retired WorkspaceAgentSpinner).
  const [markup] = useState(() => SPINNERS[Math.floor(Math.random() * SPINNERS.length)] ?? '')
  return (
    <span
      role="status"
      aria-label="Loading"
      // Size via className (default size-4); color inherits currentColor. The
      // inner <svg> fills the span. No hardcoded color — a theme token wraps it.
      className={cn('inline-flex size-4 items-center justify-center [&>svg]:size-full', className)}
      dangerouslySetInnerHTML={{ __html: markup }}
      {...props}
    />
  )
}
```

6. Run — expected **PASS**:

```
cd web && bun run test:coverage -- flicker-spinner.test.tsx && bun tsc --noEmit
```

7. **Commit:** `feat(ui): FlickerSpinner flip-dot component from flicker SVGs`

---

## Task 8 — `WorkspaceBranchIcon` uses `<FlickerSpinner>`; drop `@agilek/cli-loaders`

**Files**
- Modify: `web/src/components/layout/workspace-branch-icon.tsx`
- Modify: `package.json` (remove `@agilek/cli-loaders` dependency; run `bun install`)
- Test: `web/src/__tests__/components/layout/workspace-branch-icon.test.tsx` (create or extend)

**Interfaces**
- Consumes: `FlickerSpinner` from `@/components/ui/flicker-spinner` (Task 7).
- Produces: `WorkspaceBranchIcon` renders `<FlickerSpinner>` (in a `text-primary`, `size-4` wrapper) when `working`; `WorkspaceAgentSpinner` export is removed (or re-implemented as a thin wrapper).

**Steps**

1. Failing test `workspace-branch-icon.test.tsx`:

```tsx
import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { WorkspaceBranchIcon } from '@/components/layout/workspace-branch-icon'

describe('WorkspaceBranchIcon', () => {
  it('renders the centralized flip-dot spinner when working', () => {
    const { container, getByRole } = render(<WorkspaceBranchIcon status="new" working />)
    expect(getByRole('status')).toBeTruthy()
    // Flicker spinner, not the retired @agilek/cli-loaders Spinner.
    expect(container.querySelector('svg animate')).not.toBeNull()
  })

  it('renders the branch glyph when idle', () => {
    const { queryByRole } = render(<WorkspaceBranchIcon status="new" />)
    expect(queryByRole('status')).toBeNull()
  })
})
```

2. Run — expected **FAIL** (still imports `@agilek/cli-loaders`; no `animate`):

```
cd web && bun run test:coverage -- workspace-branch-icon.test.tsx
```

3. Implement — replace the top imports and the `WorkspaceAgentSpinner` internals:

```tsx
import { GitBranch, GitFork, GitMerge, GitPullRequest, Lock, Warning } from '@phosphor-icons/react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { WorkspaceStatus } from '@/lib/store/sidebar'
```

Keep the component body identical except the spinner branch and helper:

```tsx
  if (working) return <WorkspaceAgentSpinner />
```

```tsx
export function WorkspaceAgentSpinner() {
  // Theme-token colored (text-primary), never a provider/hardcoded color; the
  // <FlickerSpinner> random-picks a flicker spinner and animates it.
  return (
    <span className="flex size-4 shrink-0 items-center justify-center text-primary">
      <FlickerSpinner className="size-3.5" />
    </span>
  )
}
```

4. Remove `@agilek/cli-loaders` from `package.json` `dependencies` and run `bun install` to update `bun.lock`. Update the one other importer flagged in Task-7 notes if any test referenced it (`__tests__/components/layout/pending-create-row.test.tsx` — if it mocks `@agilek/cli-loaders`, drop the mock; the flicker spinner needs none).

5. Run — expected **PASS**, and confirm the package is gone:

```
cd web && bun install && bun run test:coverage -- workspace-branch-icon.test.tsx pending-create-row.test.tsx && bun tsc --noEmit
grep -r "@agilek/cli-loaders" web/src && echo "STILL PRESENT (fail)" || echo "clean"
```

6. **Commit:** `refactor(ui): WorkspaceBranchIcon uses FlickerSpinner; drop cli-loaders`

---

## Task 9 — Agent REST client (`agent-api.ts`)

**Files**
- Create: `web/src/features/agent/api/agent-api.ts`
- Test: `web/src/__tests__/features/agent/api/agent-api.test.ts`

**Interfaces**
- Consumes: `apiFetch` (`@/lib/api`), `workspaceBase` (`@/lib/workspace-scope-url`).
- Produces (types): `AgentSegment`, `AgentChat`, `AgentChatDetail`, `AgentProvider`.
- Produces (functions): `listChats(wsId) → AgentChat[]`, `getChat(wsId, id) → AgentChatDetail`, `createChat(wsId, provider) → string /*id*/`, `switchProvider(wsId, id, provider) → string /*new segId*/`, `renameChat(wsId, id, title) → void`, `deleteChat(wsId, id) → void`, `listProviders(wsId) → AgentProvider[]`.

**Steps**

1. Failing test `agent-api.test.ts`. Mock `@/lib/api` and `@/lib/workspace-scope-url`, assert URLs/methods/mapping:

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiFetch = vi.fn()
vi.mock('@/lib/api', () => ({ apiFetch: (...a: unknown[]) => apiFetch(...a) }))
vi.mock('@/lib/workspace-scope-url', () => ({ workspaceBase: (id: string) => `/v0/ws/${id}` }))

import * as api from '@/features/agent/api/agent-api'

describe('agent-api', () => {
  beforeEach(() => apiFetch.mockReset())

  it('listChats GETs the workspace-scoped chats list', async () => {
    apiFetch.mockResolvedValue([
      { id: 'c1', workspaceId: 'w1', title: 'T', activeSegmentId: 's1', activeProviderId: 'claude', createdAt: '2026-01-01T00:00:00Z' },
    ])
    const chats = await api.listChats('w1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/ws/w1/agent/chats')
    expect(chats[0]).toMatchObject({ id: 'c1', activeProviderId: 'claude' })
  })

  it('createChat POSTs the provider and returns the new id', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    const id = await api.createChat('w1', 'codex')
    expect(id).toBe('c9')
    const [url, init] = apiFetch.mock.calls[0]
    expect(url).toBe('/v0/ws/w1/agent/chats')
    expect(init).toMatchObject({ method: 'POST', body: JSON.stringify({ provider: 'codex' }) })
  })

  it('switchProvider POSTs to /switch and returns the new segment id', async () => {
    apiFetch.mockResolvedValue({ id: 'seg2' })
    const seg = await api.switchProvider('w1', 'c1', 'claude')
    expect(seg).toBe('seg2')
    expect(apiFetch.mock.calls[0][0]).toBe('/v0/ws/w1/agent/chats/c1/switch')
  })

  it('renameChat POSTs the title; deleteChat DELETEs; listProviders GETs', async () => {
    apiFetch.mockResolvedValue(undefined)
    await api.renameChat('w1', 'c1', 'New')
    expect(apiFetch.mock.calls[0][0]).toBe('/v0/ws/w1/agent/chats/c1/rename')
    await api.deleteChat('w1', 'c1')
    expect(apiFetch.mock.calls[1]).toEqual(['/v0/ws/w1/agent/chats/c1', { method: 'DELETE' }])
    apiFetch.mockResolvedValue([{ id: 'claude', displayName: 'Claude', icon: '<svg/>' }])
    const p = await api.listProviders('w1')
    expect(p[0]).toMatchObject({ id: 'claude', displayName: 'Claude' })
  })
})
```

2. Run — expected **FAIL** (module missing):

```
cd web && bun run test:coverage -- agent-api.test.ts
```

3. Implement `agent-api.ts`:

```ts
import { apiFetch } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'

// Workspace-scoped agentic-chat REST client. Routes nest under
// workspaceBase(wsId)/agent (00 agentic-engine spec §2); the {success,data}
// envelope is unwrapped by apiFetch. Modelled on features/git/api/review-api.ts.

function agentBase(wsId: string): string {
  return `${workspaceBase(wsId)}/agent`
}

// ── Wire shapes (identical to the backend DTOs; camelCase) ──────────
export type AgentSegmentStatus = 'active' | 'ended'

export interface AgentSegment {
  id: string
  providerId: string
  providerSessionId?: string
  crowbarSegmentId: string
  terminalSessionId: string
  startedAt: string
  endedAt?: string
  status: AgentSegmentStatus
}

export interface AgentChat {
  id: string
  workspaceId: string
  title: string
  activeSegmentId: string
  activeProviderId: string
  createdAt: string
}

export interface AgentChatDetail extends AgentChat {
  segments: AgentSegment[]
}

export interface AgentProvider {
  id: string
  displayName: string
  icon: string
}

// ── Mappers (wire → store types). Identity today, but kept explicit so a
//    future wire/store divergence changes one place (review-api idiom). ──
function mapChat(c: AgentChat): AgentChat {
  return {
    id: c.id,
    workspaceId: c.workspaceId,
    title: c.title,
    activeSegmentId: c.activeSegmentId,
    activeProviderId: c.activeProviderId,
    createdAt: c.createdAt,
  }
}

// ── Reads ───────────────────────────────────────────────────────────
export async function listChats(wsId: string): Promise<AgentChat[]> {
  const raw = await apiFetch<AgentChat[]>(`${agentBase(wsId)}/chats`)
  return (raw ?? []).map(mapChat)
}

export async function getChat(wsId: string, id: string): Promise<AgentChatDetail> {
  const raw = await apiFetch<AgentChatDetail>(`${agentBase(wsId)}/chats/${encodeURIComponent(id)}`)
  return { ...mapChat(raw), segments: raw.segments ?? [] }
}

export async function listProviders(wsId: string): Promise<AgentProvider[]> {
  const raw = await apiFetch<AgentProvider[]>(`${agentBase(wsId)}/providers`)
  return raw ?? []
}

// ── Writes ──────────────────────────────────────────────────────────
export async function createChat(wsId: string, provider: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(`${agentBase(wsId)}/chats`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ provider }),
  })
  return res.id
}

export async function switchProvider(wsId: string, id: string, provider: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(
    `${agentBase(wsId)}/chats/${encodeURIComponent(id)}/switch`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ provider }),
    },
  )
  return res.id
}

export async function renameChat(wsId: string, id: string, title: string): Promise<void> {
  await apiFetch<unknown>(`${agentBase(wsId)}/chats/${encodeURIComponent(id)}/rename`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  })
}

export async function deleteChat(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${agentBase(wsId)}/chats/${encodeURIComponent(id)}`, { method: 'DELETE' })
}
```

4. Run — expected **PASS**:

```
cd web && bun run test:coverage -- agent-api.test.ts && bun tsc --noEmit
```

5. **Commit:** `feat(agent): workspace-scoped agent REST client`

---

## Task 10 — Agent chats store slice (`agent-chats-slice.ts`)

**Files**
- Create: `web/src/features/workspace/stores/slices/agent-chats-slice.ts`
- Modify: `web/src/features/workspace/stores/workspace-store.types.ts` (add `AgentChatsSlice`)
- Modify: `web/src/features/workspace/stores/workspace-store.ts` (compose `createAgentChatsSlice`)
- Test: `web/src/__tests__/features/workspace/stores/slices/agent-chats-slice.test.ts`

**Interfaces**
- Consumes: `AgentChat`, `AgentProvider` (Task 9); `WorkspaceState` (immer StateCreator, like `branch-review-slice.ts`); `get().workspaceId` for per-workspace order persistence.
- Produces: `AgentChatsSlice` = `{ agentChats: AgentChatsState, upsertAgentChat, removeAgentChat, setAgentChatWorking, setAgentChatOrder, setActiveAgentChatId, setAgentProviders, hydrateAgentChatOrder }` + pure helper `orderedChats(chats, order)`.

**Steps**

1. Failing test `agent-chats-slice.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { orderedChats } from '@/features/workspace/stores/slices/agent-chats-slice'
import type { AgentChat } from '@/features/agent/api/agent-api'

const chat = (id: string, createdAt: string): AgentChat => ({
  id, workspaceId: 'w1', title: id, activeSegmentId: `${id}-s`, activeProviderId: 'claude', createdAt,
})

describe('agent-chats-slice', () => {
  it('upserts (insert then replace by id) and removes', () => {
    const s = createWorkspaceStore('w1')
    s.getState().upsertAgentChat(chat('c1', '2026-01-01T00:00:00Z'))
    s.getState().upsertAgentChat(chat('c1', '2026-01-02T00:00:00Z')) // replace
    expect(s.getState().agentChats.chats).toHaveLength(1)
    expect(s.getState().agentChats.chats[0].createdAt).toBe('2026-01-02T00:00:00Z')
    s.getState().removeAgentChat('c1')
    expect(s.getState().agentChats.chats).toHaveLength(0)
  })

  it('toggles the working map and stores providers/active id', () => {
    const s = createWorkspaceStore('w1')
    s.getState().setAgentChatWorking('c1', true)
    expect(s.getState().agentChats.working.c1).toBe(true)
    s.getState().setAgentChatWorking('c1', false)
    expect(s.getState().agentChats.working.c1).toBe(false)
    s.getState().setAgentProviders([{ id: 'claude', displayName: 'Claude', icon: '<svg/>' }])
    expect(s.getState().agentChats.providers).toHaveLength(1)
    s.getState().setActiveAgentChatId('c1')
    expect(s.getState().agentChats.activeChatId).toBe('c1')
  })

  it('orderedChats: saved order first, unknown chats appended by createdAt', () => {
    const chats = [chat('a', '2026-01-03T00:00:00Z'), chat('b', '2026-01-01T00:00:00Z'), chat('c', '2026-01-02T00:00:00Z')]
    // order pins b then a; c is absent → appended after, sorted by createdAt.
    expect(orderedChats(chats, ['b', 'a']).map((x) => x.id)).toEqual(['b', 'a', 'c'])
    // empty order → pure createdAt ascending.
    expect(orderedChats(chats, []).map((x) => x.id)).toEqual(['b', 'c', 'a'])
  })
})
```

2. Run — expected **FAIL** (slice not composed):

```
cd web && bun run test:coverage -- agent-chats-slice.test.ts
```

3. Implement `agent-chats-slice.ts`:

```ts
import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { AgentChat, AgentProvider } from '@/features/agent/api/agent-api'

const orderKey = (wsId: string) => `crowbar:agent-chat-order:${wsId}`

function loadOrder(wsId: string): string[] {
  try {
    const raw = localStorage.getItem(orderKey(wsId))
    return raw ? (JSON.parse(raw) as string[]) : []
  } catch {
    return []
  }
}

function saveOrder(wsId: string, order: string[]): void {
  try {
    localStorage.setItem(orderKey(wsId), JSON.stringify(order))
  } catch {
    /* quota / private mode — best effort */
  }
}

/** Order chats by the client-persisted order first (chats named in `order`, in
 *  that sequence), then append any chat absent from `order` sorted by createdAt
 *  ascending (creation order, newest last) — §4.6 default ordering. Pure/testable. */
export function orderedChats(chats: AgentChat[], order: string[]): AgentChat[] {
  const byId = new Map(chats.map((c) => [c.id, c]))
  const pinned = order.map((id) => byId.get(id)).filter((c): c is AgentChat => c !== undefined)
  const pinnedIds = new Set(pinned.map((c) => c.id))
  const rest = chats
    .filter((c) => !pinnedIds.has(c.id))
    .sort((a, b) => a.createdAt.localeCompare(b.createdAt))
  return [...pinned, ...rest]
}

export interface AgentChatsState {
  chats: AgentChat[]
  working: Record<string, boolean>
  order: string[]
  activeChatId: string | null
  providers: AgentProvider[]
}

export interface AgentChatsSlice {
  agentChats: AgentChatsState
  upsertAgentChat: (chat: AgentChat) => void
  removeAgentChat: (chatId: string) => void
  setAgentChatWorking: (chatId: string, working: boolean) => void
  setAgentChatOrder: (order: string[]) => void
  hydrateAgentChatOrder: () => void
  setActiveAgentChatId: (chatId: string | null) => void
  setAgentProviders: (providers: AgentProvider[]) => void
}

export const INITIAL_AGENT_CHATS_STATE: AgentChatsState = {
  chats: [],
  working: {},
  order: [],
  activeChatId: null,
  providers: [],
}

export const createAgentChatsSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  AgentChatsSlice
> = (set, get) => ({
  agentChats: { ...INITIAL_AGENT_CHATS_STATE },

  upsertAgentChat: (chat) =>
    set((s) => {
      const idx = s.agentChats.chats.findIndex((c) => c.id === chat.id)
      if (idx === -1) s.agentChats.chats.push(chat)
      else s.agentChats.chats[idx] = chat
    }),

  removeAgentChat: (chatId) =>
    set((s) => {
      s.agentChats.chats = s.agentChats.chats.filter((c) => c.id !== chatId)
      delete s.agentChats.working[chatId]
      s.agentChats.order = s.agentChats.order.filter((id) => id !== chatId)
      if (s.agentChats.activeChatId === chatId) s.agentChats.activeChatId = null
    }),

  setAgentChatWorking: (chatId, working) =>
    set((s) => {
      s.agentChats.working[chatId] = working
    }),

  setAgentChatOrder: (order) => {
    saveOrder(get().workspaceId, order)
    set((s) => {
      s.agentChats.order = order
    })
  },

  hydrateAgentChatOrder: () =>
    set((s) => {
      s.agentChats.order = loadOrder(get().workspaceId)
    }),

  setActiveAgentChatId: (chatId) =>
    set((s) => {
      s.agentChats.activeChatId = chatId
    }),

  setAgentProviders: (providers) =>
    set((s) => {
      s.agentChats.providers = providers
    }),
})
```

4. Compose it. In `workspace-store.types.ts` add the import + intersection member:

```ts
import type { AgentChatsSlice } from './slices/agent-chats-slice'
// …
export type WorkspaceState = WorkspaceBaseState &
  PaneSlice &
  BufferSlice &
  LspSlice &
  TerminalSlice &
  FileWatcherSlice &
  RecentFilesSlice &
  BranchReviewSlice &
  AgentChatsSlice
```

In `workspace-store.ts` add the import + spread (after `createBranchReviewSlice(...)`):

```ts
import { createAgentChatsSlice } from './slices/agent-chats-slice'
// …
        ...createBranchReviewSlice(set, get, api),
        ...createAgentChatsSlice(set, get, api),
```

5. Run — expected **PASS**:

```
cd web && bun run test:coverage -- agent-chats-slice.test.ts && bun tsc --noEmit
```

6. **Commit:** `feat(agent): agent-chats workspace store slice`

---

## Task 11 — `agentChat` pane content type + buffer builder

> Defined before the WS hook (Task 12) and the panel (Task 17) so both can reference the `agentChat` type and its `chatId` field with a green `tsc`. The pane-container **render case** is added later in Task 16 (it needs the `AgentChatPane` component to exist).

**Files**
- Modify: `web/src/features/panes/types/pane-content.ts` (type + interface + guard + OpenContentSpec + virtual set)
- Modify: `web/src/features/workspace/stores/slices/buffer-slice.ts` (dedup branch + builder branch)
- Test: `web/src/__tests__/features/workspace/stores/slices/buffer-slice-agent-chat.test.ts`

**Interfaces**
- Produces: `AgentChatContent extends PaneContentBase { type: 'agentChat'; chatId: string; wsId: string }`; `isAgentChatContent(c): c is AgentChatContent`; `OpenContentSpec` variant `{ type: 'agentChat'; chatId: string; wsId: string; name: string }`.
- Consumes: existing `openContent` dedup/builder machinery.

**Steps**

1. Failing test `buffer-slice-agent-chat.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

describe('buffer-slice agentChat', () => {
  it('opens an agentChat buffer and dedups/focuses by chatId', () => {
    const s = createWorkspaceStore('w1')
    const id1 = s.getState().bufferActions.openContent({ type: 'agentChat', chatId: 'c1', wsId: 'w1', name: 'Chat 1' })
    const id2 = s.getState().bufferActions.openContent({ type: 'agentChat', chatId: 'c1', wsId: 'w1', name: 'Chat 1' })
    expect(id2).toBe(id1) // same chatId → existing buffer focused, not duplicated
    const buf = s.getState().bufferActions.getBufferById(id1)
    expect(buf?.type).toBe('agentChat')
    expect((buf as { chatId?: string }).chatId).toBe('c1')
  })
})
```

2. Run — expected **FAIL** (`'agentChat'` not assignable to `OpenContentSpec`):

```
cd web && bun run test:coverage -- buffer-slice-agent-chat.test.ts
```

3. Implement. In `pane-content.ts`: add `'agentChat'` to `PaneContentType`; add the interface, guard, union member, `OpenContentSpec` variant, and virtual set entry:

```ts
export type PaneContentType =
  | 'editor'
  | 'terminal'
  | 'newTab'
  | 'diff'
  | 'markdownPreview'
  | 'htmlPreview'
  | 'csvPreview'
  | 'externalEditor'
  | 'branchReview'
  | 'agentChat'
```

```ts
export interface AgentChatContent extends PaneContentBase {
  type: 'agentChat'
  /** Stable chat id — the pane tab is keyed by it (survives provider switches). */
  chatId: string
  wsId: string
}
```

```ts
export type PaneContent =
  | EditorContent
  | TerminalContent
  | NewTabContent
  | DiffContent
  | MarkdownPreviewContent
  | HtmlPreviewContent
  | CsvPreviewContent
  | ExternalEditorContent
  | BranchReviewContent
  | AgentChatContent
```

```ts
export function isAgentChatContent(c: PaneContent): c is AgentChatContent {
  return c.type === 'agentChat'
}
```

```ts
// (append to OpenContentSpec union)
  | {
      type: 'agentChat'
      chatId: string
      wsId: string
      name: string
    }
```

```ts
const VIRTUAL_TYPES: ReadonlySet<PaneContentType> = new Set([
  'terminal',
  'newTab',
  'branchReview',
  'agentChat',
])
```

In `buffer-slice.ts`: import `AgentChatContent` in the `pane-content` type import block. Add a dedup branch inside the `existing` IIFE (next to the `branchReview` branch):

```ts
          if (spec.type === 'agentChat') {
            return get().buffers.find(
              (b) => b.type === 'agentChat' && (b as AgentChatContent).chatId === spec.chatId,
            )
          }
```

Add a builder branch (next to the `branchReview` builder branch):

```ts
        } else if (spec.type === 'agentChat') {
          buf = {
            id,
            type: 'agentChat',
            chatId: spec.chatId,
            wsId: spec.wsId,
            name: spec.name,
            path: `agent-chat://${spec.chatId}`,
            isPinned: false,
            isPreview: false,
            isActive: false,
          } satisfies AgentChatContent
```

4. Run — expected **PASS**:

```
cd web && bun run test:coverage -- buffer-slice-agent-chat.test.ts && bun tsc --noEmit
```

5. **Commit:** `feat(agent): agentChat pane content type + buffer builder`

---

## Task 12 — Workspace agent-chats WS hook

**Files**
- Create: `web/src/features/workspace/stores/hooks/use-workspace-agent-chats-stream.ts`
- Test: `web/src/__tests__/features/workspace/stores/hooks/use-workspace-agent-chats-stream.test.ts`

**Interfaces**
- Consumes: `wsManager` (`@/lib/ws/manager`), `workspaceBase`, `listChats`/`getChat`/`listProviders` (Task 9), `getOrCreateWorkspaceStore` (slice actions from Task 10), `AgentChatEvent` type (defined here).
- Produces: `useWorkspaceAgentChatsStream(wsId: string): void` — seeds chats + order + providers, subscribes `${workspaceBase(wsId)}/agent/ws/chats`, handles `{reconnected}` reseed and per-kind frames, closes a deleted chat's pane tab.

**Steps**

1. Failing test `use-workspace-agent-chats-stream.test.ts`. Model on `use-workspace-threads-stream.test.ts` (mock `@/lib/ws/manager` to capture the callback + return an unsubscribe, mock `@/features/agent/api/agent-api`, render the hook, then invoke frames and block on the mocked promises — no timers/sleeps):

```ts
import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

let capturedCb: ((f: unknown) => void) | null = null
const unsub = vi.fn()
vi.mock('@/lib/ws/manager', () => ({
  wsManager: { subscribe: (_e: string, cb: (f: unknown) => void) => { capturedCb = cb; return unsub } },
}))
vi.mock('@/lib/workspace-scope-url', () => ({ workspaceBase: (id: string) => `/v0/ws/${id}` }))
const listChats = vi.fn()
const getChat = vi.fn()
const listProviders = vi.fn()
vi.mock('@/features/agent/api/agent-api', () => ({
  listChats: (...a: unknown[]) => listChats(...a),
  getChat: (...a: unknown[]) => getChat(...a),
  listProviders: (...a: unknown[]) => listProviders(...a),
}))

import { useWorkspaceAgentChatsStream } from '@/features/workspace/stores/hooks/use-workspace-agent-chats-stream'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'

const chat = (id: string) => ({ id, workspaceId: 'w1', title: id, activeSegmentId: `${id}-s`, activeProviderId: 'claude', createdAt: '2026-01-01T00:00:00Z' })

describe('useWorkspaceAgentChatsStream', () => {
  beforeEach(() => {
    capturedCb = null
    listChats.mockResolvedValue([chat('c1')])
    getChat.mockResolvedValue(chat('c1'))
    listProviders.mockResolvedValue([{ id: 'claude', displayName: 'Claude', icon: '<svg/>' }])
  })

  it('seeds chats + providers on mount', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await waitFor(() => expect(getOrCreateWorkspaceStore('w1').getState().agentChats.chats).toHaveLength(1))
    expect(getOrCreateWorkspaceStore('w1').getState().agentChats.providers).toHaveLength(1)
  })

  it('turn_started/turn_stopped toggle the working map without a refetch', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await waitFor(() => expect(capturedCb).not.toBeNull())
    getChat.mockClear()
    capturedCb!({ chatId: 'c1', workspaceId: 'w1', kind: 'turn_started' })
    expect(getOrCreateWorkspaceStore('w1').getState().agentChats.working.c1).toBe(true)
    capturedCb!({ chatId: 'c1', workspaceId: 'w1', kind: 'turn_stopped' })
    expect(getOrCreateWorkspaceStore('w1').getState().agentChats.working.c1).toBe(false)
    expect(getChat).not.toHaveBeenCalled() // turn events never refetch
  })

  it('deleted removes the chat from the store', async () => {
    const store = getOrCreateWorkspaceStore('w1')
    store.getState().upsertAgentChat(chat('c1'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await waitFor(() => expect(capturedCb).not.toBeNull())
    capturedCb!({ chatId: 'c1', workspaceId: 'w1', kind: 'deleted' })
    expect(store.getState().agentChats.chats.find((c) => c.id === 'c1')).toBeUndefined()
  })

  it('reconnect sentinel reseeds', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await waitFor(() => expect(capturedCb).not.toBeNull())
    listChats.mockClear()
    capturedCb!({ reconnected: true })
    await waitFor(() => expect(listChats).toHaveBeenCalled())
  })
})
```

2. Run — expected **FAIL** (module missing):

```
cd web && bun run test:coverage -- use-workspace-agent-chats-stream.test.ts
```

3. Implement `use-workspace-agent-chats-stream.ts` (structural twin of `use-workspace-threads-stream.ts`):

```ts
import { useEffect } from 'react'
import { wsManager } from '@/lib/ws/manager'
import { workspaceBase } from '@/lib/workspace-scope-url'
import { listChats, getChat, listProviders } from '@/features/agent/api/agent-api'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'

// Bare lifecycle frame (00 agentic-engine spec §7): no snapshot, so most kinds
// react-then-refetch; only turn_started/turn_stopped carry enough in the kind.
interface AgentChatEvent {
  chatId: string
  workspaceId: string
  kind:
    | 'created'
    | 'segment_opened'
    | 'segment_ended'
    | 'session_bound'
    | 'turn_started'
    | 'turn_stopped'
    | 'title_set'
    | 'deleted'
}

/** Subscribe to the workspace-scoped agent-chat lifecycle WS while `wsId` is
 *  active. Mirrors useWorkspaceThreadsStream: seed via GET, subscribe, handle the
 *  {reconnected} sentinel by reseeding, and per-kind react-then-refetch vs
 *  working-map toggle. Providers are seeded once (workspace-independent data). */
export function useWorkspaceAgentChatsStream(wsId: string): void {
  useEffect(() => {
    let cancelled = false

    const seedChats = async () => {
      try {
        const chats = await listChats(wsId)
        if (cancelled) return
        const st = getOrCreateWorkspaceStore(wsId).getState()
        st.hydrateAgentChatOrder()
        for (const c of chats) st.upsertAgentChat(c)
      } catch {
        /* seed failure is non-fatal — the WS stream still pushes */
      }
    }

    const seedProviders = async () => {
      try {
        const providers = await listProviders(wsId)
        if (cancelled) return
        getOrCreateWorkspaceStore(wsId).getState().setAgentProviders(providers)
      } catch {
        /* non-fatal */
      }
    }

    const refetchOne = async (chatId: string) => {
      try {
        const chat = await getChat(wsId, chatId)
        if (cancelled) return
        getOrCreateWorkspaceStore(wsId).getState().upsertAgentChat(chat)
      } catch {
        /* a not-found here is handled by the deleted frame path */
      }
    }

    void seedChats()
    void seedProviders()

    const unsubscribe = wsManager.subscribe(`${workspaceBase(wsId)}/agent/ws/chats`, (frame) => {
      if (cancelled) return
      if (frame && typeof frame === 'object' && 'reconnected' in frame) {
        void seedChats()
        return
      }
      const ev = frame as AgentChatEvent
      if (!ev.chatId) return
      const st = getOrCreateWorkspaceStore(wsId).getState()
      switch (ev.kind) {
        case 'turn_started':
          st.setAgentChatWorking(ev.chatId, true)
          return
        case 'turn_stopped':
          st.setAgentChatWorking(ev.chatId, false)
          return
        case 'deleted': {
          st.removeAgentChat(ev.chatId)
          // Close the deleted chat's pane tab if open (§4.5).
          const buf = st.buffers.find((b) => b.type === 'agentChat' && b.chatId === ev.chatId)
          if (buf) st.bufferActions.closeBuffer(buf.id)
          return
        }
        case 'created':
          void seedChats() // new chat + ordering — reseed the whole list
          return
        default:
          void refetchOne(ev.chatId) // title_set / segment_* / session_bound
      }
    })

    return () => {
      cancelled = true
      unsubscribe()
    }
  }, [wsId])
}
```

> `b.type === 'agentChat'` and `b.chatId` are valid because the `AgentChatContent` pane type is defined in Task 11 (executed before this task). When no chat pane is open the `.find` is a harmless no-op.

4. Run — expected **PASS**:

```
cd web && bun run test:coverage -- use-workspace-agent-chats-stream.test.ts && bun tsc --noEmit
```

5. **Commit:** `feat(agent): workspace agent-chats WS stream hook`

---

## Task 13 — Provider-switch dropdown

**Files**
- Create: `web/src/features/agent/components/provider-switch-dropdown.tsx`
- Test: `web/src/__tests__/features/agent/components/provider-switch-dropdown.test.tsx`

**Interfaces**
- Consumes: `Dropdown`, `DROPDOWN_TRIGGER_BASE` (`@/components/ui/dropdown`); `AgentProvider` (Task 9).
- Produces: `ProviderSwitchDropdown({ providers, currentProviderId, onSwitch }: { providers: AgentProvider[]; currentProviderId: string; onSwitch: (providerId: string) => void })` — a trigger showing the current provider (icon + name) that opens a menu of the OTHER providers.

**Steps**

1. Failing test `provider-switch-dropdown.test.tsx`:

```tsx
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ProviderSwitchDropdown } from '@/features/agent/components/provider-switch-dropdown'

const providers = [
  { id: 'claude', displayName: 'Claude', icon: '<svg data-p="claude"/>' },
  { id: 'codex', displayName: 'Codex', icon: '<svg data-p="codex"/>' },
]

describe('ProviderSwitchDropdown', () => {
  it('shows the current provider on the trigger', () => {
    render(<ProviderSwitchDropdown providers={providers} currentProviderId="claude" onSwitch={() => {}} />)
    expect(screen.getByRole('button', { name: /claude/i })).toBeTruthy()
  })

  it('opens and switches to another provider', () => {
    const onSwitch = vi.fn()
    render(<ProviderSwitchDropdown providers={providers} currentProviderId="claude" onSwitch={onSwitch} />)
    fireEvent.click(screen.getByRole('button', { name: /claude/i }))
    fireEvent.click(screen.getByRole('menuitem', { name: /codex/i }))
    expect(onSwitch).toHaveBeenCalledWith('codex')
  })
})
```

2. Run — expected **FAIL** (module missing):

```
cd web && bun run test:coverage -- provider-switch-dropdown.test.tsx
```

3. Implement `provider-switch-dropdown.tsx`:

```tsx
import { useRef, useState } from 'react'
import { CaretUpDown } from '@phosphor-icons/react'
import { Dropdown, DROPDOWN_TRIGGER_BASE } from '@/components/ui/dropdown'
import { cn } from '@/lib/utils'
import type { AgentProvider } from '@/features/agent/api/agent-api'

// Inline an SVG icon string so its fill="currentColor" inherits the local text token.
function ProviderIcon({ svg }: { svg: string }) {
  return (
    <span
      aria-hidden="true"
      className="inline-flex size-3.5 items-center justify-center [&>svg]:size-full"
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  )
}

interface ProviderSwitchDropdownProps {
  providers: AgentProvider[]
  currentProviderId: string
  onSwitch: (providerId: string) => void
}

export function ProviderSwitchDropdown({
  providers,
  currentProviderId,
  onSwitch,
}: ProviderSwitchDropdownProps) {
  const [open, setOpen] = useState(false)
  const anchorRef = useRef<HTMLButtonElement>(null)
  const current = providers.find((p) => p.id === currentProviderId)
  const others = providers.filter((p) => p.id !== currentProviderId)

  return (
    <>
      <button
        ref={anchorRef}
        type="button"
        onClick={() => setOpen((v) => !v)}
        className={cn(DROPDOWN_TRIGGER_BASE, 'h-7 text-foreground')}
      >
        {current && <ProviderIcon svg={current.icon} />}
        <span className="truncate">{current?.displayName ?? 'Provider'}</span>
        <CaretUpDown size={12} className="text-muted-foreground" />
      </button>
      <Dropdown
        isOpen={open}
        onClose={() => setOpen(false)}
        anchorRef={anchorRef}
        anchorSide="top"
        items={others.map((p) => ({
          id: p.id,
          label: p.displayName,
          icon: <ProviderIcon svg={p.icon} />,
          onClick: () => onSwitch(p.id),
        }))}
      />
    </>
  )
}
```

4. Run — expected **PASS**:

```
cd web && bun run test:coverage -- provider-switch-dropdown.test.tsx && bun tsc --noEmit
```

5. **Commit:** `feat(agent): provider-switch dropdown`

---

## Task 14 — Agent chat row

**Files**
- Create: `web/src/features/agent/components/agent-chat-row.tsx`
- Test: `web/src/__tests__/features/agent/components/agent-chat-row.test.tsx`

**Interfaces**
- Consumes: `ROW_BASE`, `ROW_ACTIVE`, `ROW_INACTIVE` (`@/components/layout/workspace-row-base`); `FlickerSpinner` (`@/components/ui/flicker-spinner`); `WorkspaceInlineInput` (`@/components/layout/workspace-inline-input`).
- Produces: `AgentChatRow(props)` — leading provider-icon glyph that swaps to `<FlickerSpinner>` while working; single-click selects, double-click renames inline; no menu, no ×; exposes `data-agent-chat-drop={chatId}` + a drag pointer-down.

```ts
interface AgentChatRowProps {
  chatId: string
  title: string
  providerIcon: string
  working: boolean
  active: boolean
  renaming: boolean
  onSelect: () => void
  onStartRename: () => void
  onConfirmRename: (title: string) => void
  onCancelRename: () => void
  onPointerDownDrag: (e: React.PointerEvent) => void
}
```

**Steps**

1. Failing test `agent-chat-row.test.tsx`:

```tsx
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AgentChatRow } from '@/features/agent/components/agent-chat-row'

const base = {
  chatId: 'c1', title: 'My chat', providerIcon: '<svg data-icon="claude"/>',
  working: false, active: false, renaming: false,
  onSelect: vi.fn(), onStartRename: vi.fn(), onConfirmRename: vi.fn(),
  onCancelRename: vi.fn(), onPointerDownDrag: vi.fn(),
}

describe('AgentChatRow', () => {
  it('renders the provider icon when idle and the spinner when working', () => {
    const { rerender, container } = render(<AgentChatRow {...base} />)
    expect(container.querySelector('[data-icon="claude"]')).not.toBeNull()
    rerender(<AgentChatRow {...base} working />)
    expect(screen.getByRole('status')).toBeTruthy()
  })

  it('single-click selects, double-click starts rename', () => {
    const onSelect = vi.fn()
    const onStartRename = vi.fn()
    render(<AgentChatRow {...base} onSelect={onSelect} onStartRename={onStartRename} />)
    const row = screen.getByText('My chat')
    fireEvent.click(row)
    expect(onSelect).toHaveBeenCalled()
    fireEvent.doubleClick(row)
    expect(onStartRename).toHaveBeenCalled()
  })

  it('renaming renders the inline input seeded with the title', () => {
    render(<AgentChatRow {...base} renaming />)
    expect(screen.getByDisplayValue('My chat')).toBeTruthy()
  })
})
```

2. Run — expected **FAIL** (module missing):

```
cd web && bun run test:coverage -- agent-chat-row.test.tsx
```

3. Implement `agent-chat-row.tsx`:

```tsx
import { cn } from '@/lib/utils'
import { ROW_BASE, ROW_ACTIVE, ROW_INACTIVE } from '@/components/layout/workspace-row-base'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { WorkspaceInlineInput } from '@/components/layout/workspace-inline-input'

interface AgentChatRowProps {
  chatId: string
  title: string
  providerIcon: string
  working: boolean
  active: boolean
  renaming: boolean
  onSelect: () => void
  onStartRename: () => void
  onConfirmRename: (title: string) => void
  onCancelRename: () => void
  onPointerDownDrag: (e: React.PointerEvent) => void
}

export function AgentChatRow({
  chatId,
  title,
  providerIcon,
  working,
  active,
  renaming,
  onSelect,
  onStartRename,
  onConfirmRename,
  onCancelRename,
  onPointerDownDrag,
}: AgentChatRowProps) {
  return (
    <div
      data-agent-chat-drop={chatId}
      className={cn(ROW_BASE, active ? ROW_ACTIVE : ROW_INACTIVE)}
      onPointerDown={onPointerDownDrag}
      onClick={renaming ? undefined : onSelect}
      onDoubleClick={renaming ? undefined : onStartRename}
      role="button"
      tabIndex={0}
    >
      {/* Leading glyph: provider icon → flip-dot spinner while working. Spinner
          uses a theme token (text-primary), never a provider color. */}
      {working ? (
        <span className="flex size-4 shrink-0 items-center justify-center text-primary">
          <FlickerSpinner className="size-3.5" />
        </span>
      ) : (
        <span
          aria-hidden="true"
          className="flex size-4 shrink-0 items-center justify-center text-foreground [&>svg]:size-full"
          dangerouslySetInnerHTML={{ __html: providerIcon }}
        />
      )}

      {renaming ? (
        <WorkspaceInlineInput
          defaultValue={title}
          placeholder="chat title"
          onConfirm={onConfirmRename}
          onCancel={onCancelRename}
        />
      ) : (
        <span className="min-w-0 flex-1 truncate">{title}</span>
      )}
    </div>
  )
}
```

4. Run — expected **PASS**:

```
cd web && bun run test:coverage -- agent-chat-row.test.tsx && bun tsc --noEmit
```

5. **Commit:** `feat(agent): agent chat row`

---

## Task 15 — Agent chat pane (terminal attach) + pane-container render case

**Files**
- Create: `web/src/features/agent/components/agent-chat-pane.tsx`
- Modify: `web/src/features/panes/components/pane-container.tsx` (lazy import + `case 'agentChat'`)
- Test: `web/src/__tests__/features/agent/components/agent-chat-pane.test.ts` (unit-tests the attach seam)

**Interfaces**
- Consumes: `Frame`, `FramePanel`, `FrameFooter` (`@/components/ui/frame`); `XtermTerminal` (`@/features/terminal/components/terminal`); `useTerminalStore` (`@/features/terminal/stores/terminal-store`); `saveReconnect` (`@/features/terminal/lib/terminal-reconnect-map`); `getChat`, `switchProvider` (Task 9); `ProviderSwitchDropdown` (Task 13); `useWorkspaceStore` (`@/features/workspace/stores/workspace-context`) + `useStore` (`zustand`); `AgentChatContent` (Task 11).
- Produces: `AgentChatPane({ chatId, wsId, isActivePane })`; exported seam `attachAgentSegment(wsId, chatId): Promise<string | null>` (resolves the active segment's `terminalSessionId` and **pre-seeds** the terminal-store mapping so `resolveTerminalConnection` attaches instead of spawns).

**Terminal-attach mechanism (the one real risk):** a segment's `terminalSessionId` is a live terminal-engine session. `XtermTerminal` resolves its connection via `resolveTerminalConnection`, whose highest-priority reuse branch is the terminal-store session's `connectionId`. So before mounting `XtermTerminal` with `sessionId = terminalSessionId`, seed `useTerminalStore.updateSession(terminalSessionId, { connectionId: terminalSessionId })` and `saveReconnect(wsId, terminalSessionId, terminalSessionId)`. `XtermTerminal` then sees a store `connectionId`, finds the PTY live in `listLiveSessions`, and **attaches** (no spawn). Keying `XtermTerminal` by `terminalSessionId` means a provider switch (new segment → new `terminalSessionId`, delivered as an `activeSegmentId` change via the WS refetch) **remounts and re-attaches in place** inside the stable, chatId-keyed pane tab.

> **Critical caveat — `XtermTerminal` derives its own `wsId`/`base`, it does NOT take them from `AgentChatPane` props.** `XtermTerminal`'s props are `sessionId`/`isActive`/`isVisible` only; internally it calls `getActiveWorkspaceId()` and builds `base = ${workspaceBase(wsId)}/terminals`, then runs its own `resolveTerminalConnection({ workspaceId: wsId, listLiveSessions: () => terminalListLive(base), … })` and `saveReconnect(wsId, …)` / `terminalAttach(connectionId, base)`. So the attach only succeeds when the pane's workspace (which created the agent PTY via `term.CreateCommand(ctx, workspaceID, …)`) **is** the active workspace whose live-session list (`terminalListLive(base)`) actually contains that PTY, and whose `base` matches. This holds because the Chats panel derives its `wsId` from the active route (the pane is always the active workspace's). The pre-seed `saveReconnect` in `attachAgentSegment` (keyed by the pane's `wsId`) and `XtermTerminal`'s own `saveReconnect` (keyed by `getActiveWorkspaceId()`) must therefore agree — they do only while the pane's workspace is active. **Home-workspace subtlety:** for a HOME workspace `workspaceBase(wsId)` → `/v0/projects/:p/home`, so `base = /v0/projects/:p/home/terminals` and the attach hits the **home terminal routes** (`GET /home/terminals`, `GET /home/terminals/:sessionId/ws`) that `home.Register` mounts — a different route surface than a worktree's `.../workspaces/:wsId/terminals`. Live verification MUST explicitly confirm attach works for a **home-workspace** agent chat, not only a worktree (§Live verification).

**Steps**

1. Failing test `agent-chat-pane.test.ts` (covers the load-bearing attach seam):

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'

const getChat = vi.fn()
vi.mock('@/features/agent/api/agent-api', () => ({
  getChat: (...a: unknown[]) => getChat(...a),
  switchProvider: vi.fn(),
}))
const saveReconnect = vi.fn()
vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  saveReconnect: (...a: unknown[]) => saveReconnect(...a),
}))

import { attachAgentSegment } from '@/features/agent/components/agent-chat-pane'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'

describe('attachAgentSegment', () => {
  beforeEach(() => {
    getChat.mockReset()
    saveReconnect.mockReset()
    useTerminalStore.setState({ sessions: new Map() })
  })

  it('pre-seeds the terminal-store mapping to the active segment PTY so resolve ATTACHES', async () => {
    getChat.mockResolvedValue({
      id: 'c1', activeSegmentId: 's2', activeProviderId: 'codex',
      segments: [
        { id: 's1', terminalSessionId: 'term-1', status: 'ended', providerId: 'claude', crowbarSegmentId: 's1', startedAt: '' },
        { id: 's2', terminalSessionId: 'term-2', status: 'active', providerId: 'codex', crowbarSegmentId: 's2', startedAt: '' },
      ],
    })
    const sid = await attachAgentSegment('w1', 'c1')
    expect(sid).toBe('term-2')
    expect(useTerminalStore.getState().getSession('term-2')?.connectionId).toBe('term-2')
    expect(saveReconnect).toHaveBeenCalledWith('w1', 'term-2', 'term-2')
  })

  it('returns null when the active segment has no terminal session', async () => {
    getChat.mockResolvedValue({ id: 'c1', activeSegmentId: '', segments: [] })
    expect(await attachAgentSegment('w1', 'c1')).toBeNull()
  })
})
```

2. Run — expected **FAIL** (module missing):

```
cd web && bun run test:coverage -- agent-chat-pane.test.ts
```

3. Implement `agent-chat-pane.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { useStore } from 'zustand'
import { Frame, FramePanel, FrameFooter } from '@/components/ui/frame'
import { XtermTerminal } from '@/features/terminal/components/terminal'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { saveReconnect } from '@/features/terminal/lib/terminal-reconnect-map'
import { getChat, switchProvider } from '@/features/agent/api/agent-api'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { ProviderSwitchDropdown } from './provider-switch-dropdown'

// attachAgentSegment resolves chatId's ACTIVE segment's live terminal session and
// pre-seeds the terminal-store mapping (connectionId = terminalSessionId) + the
// reconnect backstop, so XtermTerminal's resolveTerminalConnection ATTACHES the
// agent's running PTY instead of spawning a fresh one. Returns the session id to
// mount (null when there is no live segment). The single testable seam.
export async function attachAgentSegment(wsId: string, chatId: string): Promise<string | null> {
  const detail = await getChat(wsId, chatId)
  const seg = detail.segments.find((s) => s.id === detail.activeSegmentId)
  const sessionId = seg?.terminalSessionId
  if (!sessionId) return null
  useTerminalStore.getState().updateSession(sessionId, { connectionId: sessionId })
  saveReconnect(wsId, sessionId, sessionId)
  return sessionId
}

interface AgentChatPaneProps {
  chatId: string
  wsId: string
  isActivePane: boolean
}

export function AgentChatPane({ chatId, wsId, isActivePane }: AgentChatPaneProps) {
  const store = useWorkspaceStore()
  // Narrow selectors on the workspace store's agent-chats slice.
  const activeSegmentId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.activeSegmentId ?? null,
  )
  const activeProviderId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.activeProviderId ?? '',
  )
  const providers = useStore(store, (s) => s.agentChats.providers)

  const [sessionId, setSessionId] = useState<string | null>(null)

  // (Re-)attach when the chat opens or its active segment changes (provider
  // switch). Keying XtermTerminal by sessionId below remounts it on a switch so
  // the new PTY is attached in place.
  useEffect(() => {
    let cancelled = false
    void attachAgentSegment(wsId, chatId).then((sid) => {
      if (!cancelled) setSessionId(sid)
    })
    return () => {
      cancelled = true
    }
  }, [wsId, chatId, activeSegmentId])

  return (
    <Frame className="h-full w-full rounded-none bg-transparent p-0">
      <FramePanel className="min-h-0 flex-1 rounded-none border-0 bg-transparent p-0 before:hidden">
        {sessionId && (
          <XtermTerminal key={sessionId} sessionId={sessionId} isActive={isActivePane} isVisible />
        )}
      </FramePanel>
      <FrameFooter className="flex items-center justify-start px-2 py-1.5">
        <ProviderSwitchDropdown
          providers={providers}
          currentProviderId={activeProviderId}
          onSwitch={(providerId) => void switchProvider(wsId, chatId, providerId)}
        />
      </FrameFooter>
    </Frame>
  )
}
```

4. Wire the render case in `pane-container.tsx`. Add the lazy import next to the other lazies, import the `AgentChatContent` type, and a `case`:

```tsx
const AgentChatPane = lazy(() =>
  import('@/features/agent/components/agent-chat-pane').then((m) => ({ default: m.AgentChatPane })),
)
```

```tsx
        case 'agentChat': {
          const c = buffer as import('../types/pane-content').AgentChatContent
          return <AgentChatPane chatId={c.chatId} wsId={c.wsId} isActivePane={isActivePane} />
        }
```

> The agent chat pane is NOT in the always-mounted `terminal` fast-path (it renders through `renderActiveBuffer`), so tab-switching away and back remounts its inner `XtermTerminal` and re-attaches via the pre-seed — acceptable for MVP (attach replays the live PTY). Verify the replay looks clean in the Tauri app (§Live verification).

5. Run — expected **PASS**:

```
cd web && bun run test:coverage -- agent-chat-pane.test.ts && bun tsc --noEmit
```

6. **Commit:** `feat(agent): agent chat pane attaches the live agent terminal`

---

## Task 16 — Agent chats panel (list + New rows + trash footer + drag)

**Files**
- Create: `web/src/features/agent/components/agent-chats-panel.tsx`
- Test: `web/src/__tests__/features/agent/components/agent-chats-panel.test.tsx`

**Interfaces**
- Consumes: `useWorkspaceAgentChatsStream` (Task 12); `orderedChats` (Task 10); `AgentChatRow` (Task 14); `createChat`, `deleteChat`, `renameChat` (Task 9); `getOrCreateWorkspaceStore` (registry); `parseWorkspaceScopeFromPath` (`@/lib/workspace-scope`); `useRouterState` (`@tanstack/react-router`); `useStore` (`zustand`); `ScrollArea` (`@/components/ui/scroll-area`).
- Produces: `AgentChatsPanel()`; pure helper `reorderIds(orderedIds, draggedId, targetId)`.

**Steps**

1. Failing test `agent-chats-panel.test.tsx`:

```tsx
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@tanstack/react-router', () => ({ useRouterState: () => '/ide/p1/r1/w1' }))
vi.mock('@/lib/workspace-scope', () => ({ parseWorkspaceScopeFromPath: () => ({ wsId: 'w1' }) }))
vi.mock('@/features/workspace/stores/hooks/use-workspace-agent-chats-stream', () => ({
  useWorkspaceAgentChatsStream: () => {},
}))
const createChat = vi.fn().mockResolvedValue('c-new')
const deleteChat = vi.fn().mockResolvedValue(undefined)
const renameChat = vi.fn().mockResolvedValue(undefined)
vi.mock('@/features/agent/api/agent-api', () => ({
  createChat: (...a: unknown[]) => createChat(...a),
  deleteChat: (...a: unknown[]) => deleteChat(...a),
  renameChat: (...a: unknown[]) => renameChat(...a),
}))

import { AgentChatsPanel, reorderIds } from '@/features/agent/components/agent-chats-panel'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'

const seed = () => {
  const st = getOrCreateWorkspaceStore('w1').getState()
  st.setAgentProviders([
    { id: 'claude', displayName: 'Claude', icon: '<svg data-p="claude"/>' },
    { id: 'codex', displayName: 'Codex', icon: '<svg data-p="codex"/>' },
  ])
  st.upsertAgentChat({ id: 'c1', workspaceId: 'w1', title: 'First', activeSegmentId: 's1', activeProviderId: 'claude', createdAt: '2026-01-01T00:00:00Z' })
}

describe('AgentChatsPanel', () => {
  beforeEach(() => {
    createChat.mockClear(); deleteChat.mockClear()
    seed()
  })

  it('renders real chats then one New row per provider', () => {
    render(<AgentChatsPanel />)
    expect(screen.getByText('First')).toBeTruthy()
    expect(screen.getByText(/New Claude chat/i)).toBeTruthy()
    expect(screen.getByText(/New Codex chat/i)).toBeTruthy()
  })

  it('clicking a New row creates a chat for that provider', () => {
    render(<AgentChatsPanel />)
    fireEvent.click(screen.getByText(/New Codex chat/i))
    expect(createChat).toHaveBeenCalledWith('w1', 'codex')
  })

  it('clicking a chat row opens its pane tab', () => {
    render(<AgentChatsPanel />)
    fireEvent.click(screen.getByText('First'))
    const st = getOrCreateWorkspaceStore('w1').getState()
    expect(st.agentChats.activeChatId).toBe('c1')
    expect(st.buffers.some((b) => b.type === 'agentChat')).toBe(true)
  })
})

describe('reorderIds', () => {
  it('moves the dragged id in front of the target', () => {
    expect(reorderIds(['a', 'b', 'c'], 'c', 'a')).toEqual(['c', 'a', 'b'])
    expect(reorderIds(['a', 'b', 'c'], 'a', 'a')).toEqual(['a', 'b', 'c'])
  })
})
```

2. Run — expected **FAIL** (module missing):

```
cd web && bun run test:coverage -- agent-chats-panel.test.tsx
```

3. Implement `agent-chats-panel.tsx`:

```tsx
import { useCallback, useEffect, useRef, useState } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { useStore } from 'zustand'
import { cn } from '@/lib/utils'
import { ScrollArea } from '@/components/ui/scroll-area'
import { parseWorkspaceScopeFromPath } from '@/lib/workspace-scope'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { useWorkspaceAgentChatsStream } from '@/features/workspace/stores/hooks/use-workspace-agent-chats-stream'
import { orderedChats } from '@/features/workspace/stores/slices/agent-chats-slice'
import { createChat, deleteChat, renameChat } from '@/features/agent/api/agent-api'
import { AgentChatRow } from './agent-chat-row'
import type { AgentChat, AgentProvider } from '@/features/agent/api/agent-api'

/** Place draggedId immediately before targetId in the full ordered id list. */
export function reorderIds(orderedIds: string[], draggedId: string, targetId: string): string[] {
  if (draggedId === targetId) return orderedIds
  const without = orderedIds.filter((id) => id !== draggedId)
  const idx = without.indexOf(targetId)
  if (idx === -1) return orderedIds
  return [...without.slice(0, idx), draggedId, ...without.slice(idx)]
}

function findDropTarget(x: number, y: number, draggingId: string): string | null {
  for (const el of document.elementsFromPoint(x, y)) {
    if (!(el instanceof Element)) continue
    if (el.getAttribute('data-trash-drop') !== null) return 'trash'
    const id = el.getAttribute('data-agent-chat-drop')
    if (id !== null && id !== draggingId) return id
  }
  return null
}

export function AgentChatsPanel() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const wsId = parseWorkspaceScopeFromPath(pathname)?.wsId ?? null

  return wsId ? <AgentChatsPanelInner wsId={wsId} /> : null
}

function AgentChatsPanelInner({ wsId }: { wsId: string }) {
  useWorkspaceAgentChatsStream(wsId)
  const store = getOrCreateWorkspaceStore(wsId)

  const chats = useStore(store, (s) => s.agentChats.chats)
  const order = useStore(store, (s) => s.agentChats.order)
  const working = useStore(store, (s) => s.agentChats.working)
  const providers = useStore(store, (s) => s.agentChats.providers)
  const activeChatId = useStore(store, (s) => s.agentChats.activeChatId)

  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [draggingId, setDraggingId] = useState<string | null>(null)
  const [hoverTarget, setHoverTarget] = useState<string | null>(null)
  const dragRef = useRef<{ id: string; startX: number; startY: number; active: boolean } | null>(null)

  const ordered = orderedChats(chats, order)
  const providerById = new Map(providers.map((p) => [p.id, p]))

  const openChat = useCallback(
    (chat: AgentChat) => {
      const st = store.getState()
      st.setActiveAgentChatId(chat.id)
      st.bufferActions.openContent({ type: 'agentChat', chatId: chat.id, wsId, name: chat.title || 'Agent chat' })
    },
    [store, wsId],
  )

  // Pointer drag: reorder within the list, or drop on the trash footer to delete.
  useEffect(() => {
    const onMove = (e: PointerEvent) => {
      const d = dragRef.current
      if (!d) return
      if (!d.active) {
        if (Math.hypot(e.clientX - d.startX, e.clientY - d.startY) <= 5) return
        d.active = true
        setDraggingId(d.id)
      }
      setHoverTarget(findDropTarget(e.clientX, e.clientY, d.id))
    }
    const onUp = (e: PointerEvent) => {
      const d = dragRef.current
      dragRef.current = null
      if (!d || !d.active) {
        setDraggingId(null)
        setHoverTarget(null)
        return
      }
      const target = findDropTarget(e.clientX, e.clientY, d.id)
      if (target === 'trash') {
        void deleteChat(wsId, d.id) // WS 'deleted' removes it + closes the pane
      } else if (target) {
        store.getState().setAgentChatOrder(reorderIds(ordered.map((c) => c.id), d.id, target))
      }
      setDraggingId(null)
      setHoverTarget(null)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
  }, [ordered, store, wsId])

  const onPointerDownDrag = useCallback((id: string, e: React.PointerEvent) => {
    if (e.button !== 0) return
    dragRef.current = { id, startX: e.clientX, startY: e.clientY, active: false }
  }, [])

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <ScrollArea className="min-h-0 flex-1">
        <div className="py-1">
          {ordered.map((chat) => (
            <AgentChatRow
              key={chat.id}
              chatId={chat.id}
              title={chat.title || 'Untitled chat'}
              providerIcon={providerById.get(chat.activeProviderId)?.icon ?? ''}
              working={working[chat.id] ?? false}
              active={activeChatId === chat.id}
              renaming={renamingId === chat.id}
              onSelect={() => openChat(chat)}
              onStartRename={() => setRenamingId(chat.id)}
              onConfirmRename={(title) => {
                setRenamingId(null)
                store.getState().upsertAgentChat({ ...chat, title })
                void renameChat(wsId, chat.id, title)
              }}
              onCancelRename={() => setRenamingId(null)}
              onPointerDownDrag={(e) => onPointerDownDrag(chat.id, e)}
            />
          ))}

          {/* New-row per provider, below real chats, thin separator, + on the right. */}
          {providers.length > 0 && <div className="my-1 border-t border-border/60" />}
          {providers.map((p) => (
            <NewChatRow key={p.id} provider={p} onClick={() => void createChat(wsId, p.id)} />
          ))}
        </div>
      </ScrollArea>

      <TrashFooter dragging={draggingId !== null} isOver={hoverTarget === 'trash'} />
    </div>
  )
}

function NewChatRow({ provider, onClick }: { provider: AgentProvider; onClick: () => void }) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onClick}
      className="mx-1.5 my-0.5 flex h-9 cursor-pointer select-none items-center gap-2 rounded-lg px-2 text-[13px] text-muted-foreground/70 hover:bg-accent"
    >
      <span
        aria-hidden="true"
        className="flex size-4 shrink-0 items-center justify-center [&>svg]:size-full"
        dangerouslySetInnerHTML={{ __html: provider.icon }}
      />
      <span className="min-w-0 flex-1 truncate">New {provider.displayName} chat</span>
      <span className="text-[15px] leading-none">+</span>
    </div>
  )
}

// Mirrors components/layout/workspace-tree-footer.tsx: slides in on drag, "Drop to delete".
function TrashFooter({ dragging, isOver }: { dragging: boolean; isOver: boolean }) {
  return (
    <div
      className={cn(
        'shrink-0 overflow-hidden transition-[max-height] duration-150 ease-out',
        dragging ? 'max-h-16' : 'max-h-0',
      )}
    >
      <div className="flex items-center justify-center border-t border-border bg-background p-2">
        <div
          data-trash-drop="true"
          className={cn(
            'flex h-10 w-full items-center justify-center gap-2 rounded-lg border border-dashed text-[13px] font-medium transition-colors',
            isOver
              ? 'border-destructive bg-destructive/10 text-destructive'
              : 'border-destructive/40 text-destructive/40',
          )}
        >
          Drop to delete
        </div>
      </div>
    </div>
  )
}
```

4. Run — expected **PASS**:

```
cd web && bun run test:coverage -- agent-chats-panel.test.tsx && bun tsc --noEmit
```

5. **Commit:** `feat(agent): agent chats panel (list, new rows, drag, delete)`

---

## Task 17 — Register the "Chats" sidebar tab (all workspace kinds) + remove the mock ChatRow

**Files**
- Modify: `web/src/lib/store/sidebar.ts` (`SidebarTab` union)
- Modify: `web/src/components/layout/sidebar-tab-bar.tsx` (`TABS` insert Chats 2nd; keep it for home)
- Modify: `web/src/components/layout/sidebar-carousel.tsx` (`TABS` array + Chats panel → `AgentChatsPanel`)
- Delete: `web/src/components/layout/sidebar-row.tsx` and `web/src/__tests__/components/layout/sidebar-row.test.tsx` (the stray mock `ChatRow`/`NewRow` — nothing else imports them)
- Test: `web/src/__tests__/components/layout/sidebar-tab-bar-chats.test.tsx`

**Interfaces**
- Consumes: `AgentChatsPanel` (Task 16).
- Produces: a 2nd sidebar tab (order: Workspaces · **Chats** · Files · Git) present for **every** workspace kind (home + repo-home + worktree — nothing gates it by kind; only `git` is filtered on home routes).

**Steps**

1. **CRITICAL check** — confirm nothing gates sidebar tabs by workspace kind. Verified during planning: `sidebar-tab-bar.tsx` filters only `git` on the home route (`visibleTabs = isHomeRoute ? TABS.filter((t) => t.tab !== 'git') : TABS`); `sidebar-carousel.tsx` always renders all panels; the panel derives its wsId reactively from the route and shows the ACTIVE workspace's chats regardless of kind. So Chats must be added to `TABS`/union/carousel and **must not** be added to any home filter.

2. Failing test `sidebar-tab-bar-chats.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const useMatch = vi.fn()
vi.mock('@tanstack/react-router', () => ({ useMatch: (...a: unknown[]) => useMatch(...a) }))

import { SidebarTabBar } from '@/components/layout/sidebar-tab-bar'
import { useSidebarStore } from '@/lib/store/sidebar'

describe('SidebarTabBar Chats tab', () => {
  beforeEach(() => useSidebarStore.setState(useSidebarStore.getInitialState()))

  it('shows Chats as the 2nd tab on a non-home route', () => {
    useMatch.mockReturnValue(null) // not a home route
    render(<SidebarTabBar />)
    expect(screen.getByText('Chats')).toBeTruthy()
    expect(screen.getByText('Git')).toBeTruthy()
  })

  it('shows Chats on a home route (only Git is filtered)', () => {
    useMatch.mockReturnValue({}) // home route match
    render(<SidebarTabBar />)
    expect(screen.getByText('Chats')).toBeTruthy()
    expect(screen.queryByText('Git')).toBeNull()
  })
})
```

3. Run — expected **FAIL** (no Chats tab):

```
cd web && bun run test:coverage -- sidebar-tab-bar-chats.test.tsx
```

4. Implement.

`lib/store/sidebar.ts`:

```ts
export type SidebarTab = 'workspaces' | 'chats' | 'files' | 'git'
```

(Also change `getInitialState`'s `activeTab: 'workspaces' as SidebarTab` — unchanged value, still valid.)

`sidebar-tab-bar.tsx` — import a chat icon and insert Chats 2nd (the `visibleTabs` home filter already only removes `git`, so Chats survives on home automatically):

```tsx
import { SquaresFour, ChatCircle, FolderOpen, GitBranch } from '@phosphor-icons/react'
// …
const TABS: {
  tab: SidebarTab
  label: string
  Icon: React.ComponentType<{ size: number; weight: 'fill' | 'regular' }>
}[] = [
  { tab: 'workspaces', label: 'Workspaces', Icon: SquaresFour },
  { tab: 'chats', label: 'Chats', Icon: ChatCircle },
  { tab: 'files', label: 'Files', Icon: FolderOpen },
  { tab: 'git', label: 'Git', Icon: GitBranch },
]
```

`sidebar-carousel.tsx` — add `chats` to the `TABS` order array (keeps the scroll-index math aligned) and insert the Chats panel as the 2nd child so panel order matches:

```tsx
import { AgentChatsPanel } from '@/features/agent/components/agent-chats-panel'
// …
const TABS: SidebarTab[] = ['workspaces', 'chats', 'files', 'git']
```

```tsx
        {/* Workspaces panel */}
        <div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden h-full">
          <WorkspaceTree />
        </div>

        {/* Chats panel */}
        <div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden h-full">
          <AgentChatsPanel />
        </div>

        {/* Files panel */}
        {/* …existing… */}
```

5. Delete the stray mock: remove `web/src/components/layout/sidebar-row.tsx` and `web/src/__tests__/components/layout/sidebar-row.test.tsx` (confirmed via grep that only that test imports it).

6. Run — expected **PASS**, and confirm the mock is gone + full typecheck:

```
cd web && bun run test:coverage -- sidebar-tab-bar-chats.test.tsx && bun tsc --noEmit
grep -r "sidebar-row" web/src && echo "STILL REFERENCED (fail)" || echo "clean"
```

7. **Commit:** `feat(agent): register Chats sidebar tab for all workspace kinds`

---

## Live verification (Tauri) — do before claiming done

Per the project rules, tests/tsc/review ≠ a visible result. Run `make dev-desktop` (never touch prod `~/.crowbar`) and exercise:

1. Open the **Chats** tab on a worktree workspace, a **repo-home**, and a **project-home** — the list renders in all three (Task 6 + 17).
2. Click **New Claude chat** and **New Codex chat** — each spawns a chat row; selecting one opens a pane tab whose terminal **attaches** the live agent CLI (not a fresh shell) and replays cleanly. **Do this on a project-home workspace too, not only a worktree:** `XtermTerminal` derives its own `wsId`/`base` (`getActiveWorkspaceId()` → `/v0/projects/:p/home/terminals` for home), so home attach exercises a different route surface (`/home/terminals/...`) — confirm the home agent chat attaches its live PTY and that the hook-driven `turn_started`/title callbacks arrive (Task 6 scope.go home branch + home mount).
3. Use the footer **provider-switch** dropdown to switch Claude↔Codex — the pane re-attaches **in place** to the new segment's PTY (verify the full-screen TUI redraws).
4. While the agent is mid-turn, confirm the **flip-dot spinner** shows on the chat row's leading glyph AND on the workspace tree row / context pill / tile (Task 5 overlay + Task 7 spinner).
5. **Double-click** a row → inline rename; **drag** a row to reorder (order persists across a reload); **drag** a row onto the **trash footer** → it deletes immediately and closes its pane tab.
6. Confirm the real provider **logos** render correctly (Task 1 sourced SVGs) and are theme-token colored in light and dark.

---

## Self-Review

### Spec-section → task coverage

| Spec section | Task(s) |
|---|---|
| §2 Delete needs no backend work | Confirmed — no task (uses existing `DELETE`/`PurgeChat`) |
| §2 / CRITICAL: Chats for ALL kinds (home + repo-home + worktree) | **Task 6** (mount agent under home **+** home-scoped CLI callback URL in `scope.go` so project-home hooks/rename/handoff reach the daemon) + **Task 17** (no kind-gating on the tab) |
| §3 API client | **Task 9** |
| §3 Store slice (chats/working/order/activeChatId/providers) | **Task 10** |
| §3 WS hook (seed, per-kind, `{reconnected}`, cleanup) | **Task 12** |
| §3 Providers fetched once, keyed | **Task 12** (seedProviders) + **Task 10** (providers state) |
| §4.1 Sidebar Chats tab (2nd, three registration sites) | **Task 17** |
| §4.2 Chat rows = workspace-row siblings (icon→spinner, click/dblclick, no menu/×) | **Task 14** |
| §4.3 New rows per provider, `+` on right, bottom | **Task 16** |
| §4.4 Header-less `Frame` pane, `FramePanel`=terminal, `FrameFooter`=left switch dropdown | **Task 15** (Frame/pane) + **Task 13** (dropdown) |
| §4.5 Drag → trash drop-zone, immediate delete, close pane tab | **Task 16** (trash/drag/delete) + **Task 12** (close pane on `deleted`) |
| §4.6 Reorder + client-persisted order, createdAt fallback | **Task 10** (`orderedChats`, order persistence) + **Task 16** (`reorderIds`, drag) |
| §5 Terminal attach (pre-seed mapping) + re-attach on switch | **Task 15** (`attachAgentSegment`, sessionId-keyed remount) |
| §6 Flicker spinner scoped to workspace icon + chat row ONLY (glob SVGs, currentColor, random pick) + replace cli-loaders; generic `Spinner` untouched | **Task 7** (`FlickerSpinner`, new `flicker-spinner.tsx`) + **Task 8** (icon) + **Task 14** (row) |
| §7.1 Descriptor `icon` + `display_name` (real SVGs, relaxed Validate) | **Task 1** |
| §7.2 `GET .../agent/providers` + descriptor enumeration | **Task 2** (enumeration) + **Task 3** (endpoint/DTO/usecase/handler) |
| §7.3 `activeProviderId` on `AgentChatDTO` (+ list) | **Task 4** |
| §7.4 Re-derive workspace `working` from agent turns | **Task 5** |
| §8 Component/file inventory (new + modified) | Distributed across Tasks 1–17 (all listed files covered) |
| §9 Testing (store/hook/row/panel/backend/live, no timing) | Every task's TDD steps + Live verification |
| §10 Out of scope (nesting, user-managed providers UI, durable order, handoff viewer) | Not implemented — rows built flat (nesting layerable), providers endpoint accommodates future providers, order client-side |

### Type / symbol consistency (every referenced symbol is defined in a task)

- **Wire/store types** `AgentChat`, `AgentSegment`, `AgentChatDetail`, `AgentProvider` → defined **Task 9**; consumed by Tasks 10, 12, 13, 14, 15, 16.
- **Pane type** `AgentChatContent`, `isAgentChatContent`, `OpenContentSpec['agentChat']` → defined **Task 11**; consumed by Tasks 12 (delete-close), 15 (render case), 16 (openContent).
- **Slice** `AgentChatsSlice`, `orderedChats`, order persistence → defined **Task 10**; composed into `WorkspaceState` (Task 10); consumed by Tasks 12, 15, 16.
- **WS hook** `useWorkspaceAgentChatsStream` → **Task 12**; consumed by **Task 16**.
- **Components** `ProviderSwitchDropdown` (Task 13), `AgentChatRow` (Task 14), `AgentChatPane` + `attachAgentSegment` (Task 15), `AgentChatsPanel` + `reorderIds` (Task 16) — each consumed only by later tasks.
- **FlickerSpinner** `FlickerSpinner` (`@/components/ui/flicker-spinner`, a **new** file) → **Task 7**; consumed by Task 8 (`WorkspaceBranchIcon`) and Task 14 (`AgentChatRow`). The generic lucide `Spinner` (`@/components/ui/spinner`) is **left untouched**, so `components/ui/button.tsx` and `components/ui/loading-spinner.tsx` keep their existing loading spinner — the flip-dot is scoped to the workspace icon + chat row only (spec §6). No task imports the flip-dot from `@/components/ui/spinner`.
- **Backend** `Descriptor.Icon/DisplayName` (Task 1) → `AllDescriptors` (Task 2) → `ListProviders`/`AgentProviderDTO`/`Providers` (Task 3); `ActiveProviderID` (Task 4); `agentWorking`/`registerAgentWorkingProjection`/`agentEventKind` (Task 5); home mounts reuse `agenthandlers.New` + all handlers, and `scopedAgentPath`'s new `repo == ""` home branch reuses the existing `hook.go`/`chat.go`/`handoff.go` callers unchanged (Task 6). Every backend/CLI symbol referenced is defined in an earlier or same task; `scopedAgentPath` already exists (Task 6 only adds its home branch), and the home hook integration test reuses `writeLiveStubProviderDescriptor` (Task 5) + `dialAgentWS`/`waitForChatFrame` (existing harness helpers).

### No placeholders

Every code step contains real, compilable code. The **one** intentionally deferred value is the provider logo `d`-path bytes in Task 1's YAML (`REPLACE_WITH_REAL_SOURCED_*`): this is not a "TBD" but a **sourcing procedure** — fetch the official Anthropic/OpenAI SVG at implementation time (WebSearch/WebFetch, simple-icons slugs `anthropic`/`openai`), set `fill="currentColor"`, and the Task-1 test + Live verification confirm shape and appearance. You cannot hardcode a logo you must source; the mechanism is fully specified.

### Risks / open items flagged

1. **Project-home is broken on TWO fronts, both cured by Task 6 — the biggest deviation.** The spec's §7 enumerates four backend additions and §2 asserts routes are workspace-scoped, but planning found a project-home workspace fails twice: (a) the agent surface is mounted only on `wsScoped`, **not** under `/home`, so `${workspaceBase(homeWsId)}/agent/...` 404s (fixed: home mount, Task 6 commit 1); and (b) a project-home workspace resolves an **empty `RepoID`**, so the in-PTY CLI callbacks build `/v0/projects/p/repos//workspaces/ws/agent/...` and 404 — its `turn_started`, agent-derived titles, and `session_start` binding never reach the daemon (fixed: `scopedAgentPath` home branch, Task 6 commit 2). Repo-home and worktrees are unaffected (both carry a repo id). Task 5's overlay test only exercises a worktree (`importWritableWorkspace`), so a **new project-home hook integration test** (`agent_home_hook_test.go`, Task 6) asserts `session_bound`/`turn_started`/`turn_stopped`/`title_set` actually reach the daemon over the home path. If a reviewer decides home chats are out of MVP scope, drop Task 6 and gate the FE Chats tab off home routes — but that contradicts the spec, so Task 6 is included.
2. **Flicker spinner is scoped, NOT a blast radius (Task 7/8/14).** An earlier draft repurposed `components/ui/spinner.tsx`, which would have flipped every `button.tsx` / `loading-spinner.tsx` loading state to a flip-dot grid — outside spec §6, which scopes the flicker spinner to the workspace icon and the chat row only. The plan now leaves the generic lucide `Spinner` untouched and puts the flip-dot in a distinctly-named new component `FlickerSpinner` (`@/components/ui/flicker-spinner`), consumed only by `WorkspaceBranchIcon` (Task 8) and `AgentChatRow` (Task 14). No app button/loading state is affected; nothing to re-confirm beyond the two intended consumers.
3. **Terminal attach is the one real runtime risk (Task 15).** The pre-seed → attach path is unit-tested at the seam, but attaching to a full-screen TUI must be verified live (§Live verification) — replay/redraw on both initial attach and provider-switch re-attach. **Two attach subtleties (see the Task 15 mechanism caveat):** (a) `XtermTerminal` derives its own `wsId`/`base` from `getActiveWorkspaceId()` — it does NOT take them from `AgentChatPane` props — so the pane's workspace must be the active one for `terminalListLive(base)` to contain the agent PTY and for `terminalAttach(connectionId, base)` to hit the right route; this holds because the pane is always the active workspace's. (b) For a HOME workspace the attach route surface is `/home/terminals/...` (not `.../workspaces/:wsId/terminals`), so live verification must confirm attach on a **project-home** agent chat, not only a worktree.
4. **`axAgentChat` third subscriber (Task 5).** Assumes asynx supports multiple `Subscribe`/`OnForget` handlers on one instance — already true (the store projection + hub projection both subscribe). The turn-set is in-memory and authoritative, so no read-model ordering race.
5. **Integration-test PTY lifetime (Task 5).** The `livestub` (`cat`) PTY stays alive for the turn assertions and is reaped by `appContainer.Close` on harness cleanup — no leak, no sleep.

---

## Execution order & rationale

Backend endpoints land before the FE that consumes them; within the FE, types/clients precede the components that import them.

1. **Task 1–6 (backend)** — descriptor metadata → enumeration → providers endpoint → `activeProviderId` → workspace `working` → home mounts **+ home-scoped CLI callback URL** (Task 6 is a two-commit task: mount, then `scope.go` home branch). Each ends with a passing Go test (unit + integration).
2. **Task 7–8 (spinner)** — new `<FlickerSpinner>` component (leaving the generic `Spinner` untouched) then swap `WorkspaceBranchIcon` / drop `cli-loaders`. Independent of the agent feature; the chat row (Task 14) also depends on Task 7.
3. **Task 9–12 (FE foundation)** — API client → slice → `agentChat` pane type → WS hook. Ordered so each references only already-defined symbols (pane type before the hook/panel that use it).
4. **Task 13–16 (FE components)** — dropdown → row → pane (with pane-container render case) → panel. The panel is the integration point (mounts the hook, opens the pane).
5. **Task 17 (wiring)** — register the Chats tab for all kinds and delete the mock row. Last, so `AgentChatsPanel` exists and the tab is never wired to a missing panel.
6. **Live verification (Tauri)** — the final gate before claiming done.

Run the full suites at the end: `cd api && go test ./... && go test -tags integration ./tests/...` and `cd web && bun run test:coverage && bun tsc --noEmit && bunx prettier --check .`.

