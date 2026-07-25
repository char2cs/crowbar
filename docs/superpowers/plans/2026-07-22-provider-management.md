# Provider Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give Crowbar a first-class provider model — connected (installed) status, user-controlled priority, and enable/disable — surfaced in a "Providers" Settings group and consumed by a single unified "New chat" action.

**Architecture:** The backend owns the resolved provider list: `GET .../agent/providers` returns the descriptor catalog enriched with `connected` + `enabled` and already in priority order, driven by a new global `AgentProviderPreference` table (mirrors the `TerminalProfile` global-settings stack). A global `PUT /v0/settings/agent/providers` rewrites the preference set. The frontend stays dumb — every "New chat" surface takes the first *enabled* provider, and a new backend-persisted Settings tab reorders (via `@dnd-kit/sortable`) and toggles providers.

**Tech Stack:** Go (Gin, GORM/sqlite generic `store.Store[T,K]`), React 19 + TypeScript + Zustand (immer), `@dnd-kit/sortable`, Vitest, `react-doctor`.

## Global Constraints

- **No new dependencies.** `@dnd-kit/sortable` is already installed. Do **not** edit `web/package.json` or `web/bun.lock`; do **not** run `bun install` (another agent has them uncommitted for the Plate editor).
- **FE component files kebab-case**; exported component name PascalCase.
- **FE tests** live in `web/src/__tests__/` mirroring `web/src/` structure; use `@/` imports, never relative `../../`.
- **Zustand:** narrow selectors only (`useStore(store, (s) => s.field)`); `getState()` only in handlers/effects; stores must not import from `components/`.
- **No timing in tests** (no sleeps/polls) — block on real signals.
- **Wire contract is fixed** (spec §3): `AgentProviderDTO` = `{id, displayName, icon, connected, enabled}`; `PUT` body = `{providers:[{id, disabled}]}`, response = resolved `AgentProviderDTO[]`.
- **Gates (all must pass):** `go -C api test ./internal/...` · `bun --cwd web run test` · `bun --cwd web x tsc --noEmit` · `bun --cwd web run lint` · `bun --cwd web run doctor` (must stay **100/100**).
- **Commit messages** end with the repo's `Co-Authored-By` trailer. Do not push; do not open PRs.

## File Structure

**Backend (`api/`):**
- Create `api/internal/domain/agent_provider_preference.go` — domain row.
- Modify `api/internal/app/gorm.go` — register the store.
- Create `api/internal/engine/agent/connected.go` — install probe (or add to an existing file in the package).
- Modify agent usecase (`api/internal/app/usecases/agent/*.go`) — `ResolveProviders` + inject the preference store.
- Modify `api/internal/api/v0/dto/agent.go` — extend `AgentProviderDTO`.
- Modify `api/internal/api/v0/endpoints/agent/handlers/providers.go` — enriched GET + new PUT.
- Modify `api/internal/api/v0/endpoints/agent/routes.go` (+ wiring in `router.go`) — mount the global PUT.
- Tests: co-located `_test.go` for domain/probe/resolution; black-box integration test alongside the existing agent endpoint tests.

**Frontend (`web/`):**
- Modify `web/src/features/agent/api/agent-api.ts` — `AgentProvider` type + `listProviders` mapping + `updateProviderPreferences`.
- Modify `web/src/features/workspace/stores/slices/agent-chats-slice.ts` — `selectEnabledProviders`.
- Modify `web/src/features/agent/components/agent-chats-panel.tsx` — unified New-chat row.
- Modify `web/src/features/panes/components/new-tab-view.tsx` + `web/src/features/panes/hooks/use-pane-keyboard.ts` — first-enabled pick.
- Modify `web/src/features/window/stores/ui-state-store.ts` + `web/src/features/settings/components/settings-tab-items.ts` + `web/src/features/settings/components/settings-dialog.tsx` — register the tab.
- Create `web/src/features/settings/components/tabs/providers-settings.tsx` — the tab.
- Tests mirrored under `web/src/__tests__/...`.

---

## Execution grouping (for the orchestrator)

- **Wave 1 (parallel, no shared files):** Tasks **B1–B4** (backend subagent) ‖ Tasks **F1–F3** (frontend-plumbing subagent). Both code against the fixed contract; FE tests mock provider objects, so they don't need the backend.
- **Wave 2 (after F1):** Tasks **F4–F5** (frontend-settings subagent) — consumes F1's `updateProviderPreferences` + type + selector.
- **Wave 3 (orchestrator):** full gates + integration fixes.

---

## Task B1: `AgentProviderPreference` domain + store

**Files:**
- Create: `api/internal/domain/agent_provider_preference.go`
- Modify: `api/internal/app/gorm.go` (add store field + `NewFromDB` + assignment — mirror `TerminalProfiles`)
- Test: `api/internal/app/agent_provider_preference_store_test.go` (or co-located; follow how `TerminalProfiles` is tested — if there is no store test, add one here)

**Interfaces:**
- Produces: `domain.AgentProviderPreference{ProviderID string; Priority int; Disabled bool}` and `GORMStores.AgentProviderPreferences store.Store[domain.AgentProviderPreference, string]` (methods `Save`/`FindAll`/`FindByKey`/`Delete`).

- [ ] **Step 1: Write the failing test** — a round-trip through the real sqlite store (use the same in-memory/temp-db setup the existing store tests use; read a neighboring `*_store_test.go` for the harness).

```go
func TestAgentProviderPreferenceStore_RoundTrip(t *testing.T) {
	stores := newTestGORMStores(t) // same helper the other store tests use
	ctx := context.Background()
	s := stores.AgentProviderPreferences

	require.NoError(t, s.Save(ctx, domain.AgentProviderPreference{ProviderID: "codex", Priority: 0, Disabled: false}))
	require.NoError(t, s.Save(ctx, domain.AgentProviderPreference{ProviderID: "claude", Priority: 1, Disabled: true}))

	all, err := s.FindAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)

	got, err := s.FindByKey(ctx, "claude")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.True(t, got.Disabled)
	require.Equal(t, 1, got.Priority)
}
```

- [ ] **Step 2: Run it — expect FAIL** (`AgentProviderPreferences` undefined).

Run: `go -C api test ./internal/app/... -run TestAgentProviderPreferenceStore_RoundTrip`
Expected: compile error / FAIL.

- [ ] **Step 3: Create the domain file.**

```go
package domain

// AgentProviderPreference is a global user preference for one agent provider: its
// position in the priority order (lower = higher priority) and whether it is
// disabled. Persisted as a row keyed by provider id; a provider with no row uses
// defaults (enabled, ordered after every preferenced provider by descriptor id).
type AgentProviderPreference struct {
	ProviderID string `gorm:"primaryKey" json:"providerId"`
	Priority   int    `json:"priority"`
	Disabled   bool   `json:"disabled"`
}

func (AgentProviderPreference) TableName() string { return "agent_provider_preferences" }
```

- [ ] **Step 4: Wire the store in `gorm.go`** — add the struct field, a `storesqlite.NewFromDB[domain.AgentProviderPreference, string](db)` construction (with the same error-wrap style as `TerminalProfiles`), and the assignment into the returned `GORMStores`. Read the current `TerminalProfiles` lines and copy the shape exactly.

- [ ] **Step 5: Run test — expect PASS.**

Run: `go -C api test ./internal/app/... -run TestAgentProviderPreferenceStore_RoundTrip`
Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
git add api/internal/domain/agent_provider_preference.go api/internal/app/gorm.go api/internal/app/agent_provider_preference_store_test.go
git commit -m "feat(agent): global AgentProviderPreference store"
```

---

## Task B2: Connected (install) probe

**Files:**
- Create: `api/internal/engine/agent/connected.go`
- Test: `api/internal/engine/agent/connected_test.go`

**Interfaces:**
- Produces: `func Connected(cmd string) bool` in package `agent` — true iff `cmd` resolves to an executable file on PATH.

- [ ] **Step 1: Write the failing test** — put a fake executable in a temp dir that `binpath.Resolve` will find. `binpath.Resolve` probes `exec.LookPath` (respects `PATH`) first, so prepend the temp dir to `PATH`.

```go
func TestConnected(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fakecli")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	require.True(t, Connected("fakecli"), "an on-PATH executable is connected")
	require.False(t, Connected("definitely-not-a-real-cli-xyz"), "a missing cli is not connected")
}
```

- [ ] **Step 2: Run it — expect FAIL** (`Connected` undefined).

Run: `go -C api test ./internal/engine/agent/ -run TestConnected`
Expected: FAIL.

- [ ] **Step 3: Implement.**

```go
package agent

import (
	"os"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
)

// Connected reports whether a provider's CLI is installed: its spawn.cmd resolves
// to an executable file on PATH. binpath.Resolve also probes well-known bin dirs so
// the daemon's minimal PATH doesn't produce false negatives. Install-only — there is
// deliberately no auth check (the claude/codex CLIs expose no reliable one).
func Connected(cmd string) bool {
	info, err := os.Stat(binpath.Resolve(cmd))
	return err == nil && !info.IsDir()
}
```

- [ ] **Step 4: Run test — expect PASS.**

Run: `go -C api test ./internal/engine/agent/ -run TestConnected`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add api/internal/engine/agent/connected.go api/internal/engine/agent/connected_test.go
git commit -m "feat(agent): install-only Connected probe"
```

---

## Task B3: DTO extension + `ResolveProviders`

**Files:**
- Modify: `api/internal/api/v0/dto/agent.go` (extend `AgentProviderDTO`)
- Modify: the agent usecase file that holds `ListProviders` (`api/internal/app/usecases/agent/agent.go` around the existing `ListProviders`) — add `ResolveProviders` and give the usecase access to the preference store.
- Test: co-located usecase test (follow the existing usecase test harness in `api/internal/app/usecases/agent/`).

**Interfaces:**
- Consumes: `domain.AgentProviderPreference` (B1), `agent.Connected` (B2), `engineagent.AllDescriptors(home)` (existing).
- Produces: `AgentProviderDTO{ID, DisplayName, Icon string; Connected, Enabled bool}` and a usecase method that returns the enriched, priority-ordered `[]dto.AgentProviderDTO` from (descriptors + preferences + probe), independent of `wsId`.

- [ ] **Step 1: Extend the DTO** in `dto/agent.go` — add `Connected bool json:"connected"` and `Enabled bool json:"enabled"` to `AgentProviderDTO`. Keep the existing doc comment; add one line noting connected = installed, enabled = !disabled, order = priority.

- [ ] **Step 2: Write the failing resolution test.** Drive it with a fake descriptor set + a stub preference store + a stub probe (inject the probe as a func field or package var you can override in the test — pick whichever matches the usecase's existing testability; if the usecase already takes injected deps, add the probe there). The assertion pins ordering + enabled + connected:

```go
func TestResolveProviders_OrdersByPreferenceThenAppendsNewEnabled(t *testing.T) {
	// descriptors: claude, codex (ids). prefs: codex priority 0 enabled, claude priority 1 DISABLED.
	// a third descriptor "gemini" has NO pref row.
	uc := newTestAgentUsecase(t, /* descriptors */ []string{"claude", "codex", "gemini"})
	uc.setPrefs(t,
		domain.AgentProviderPreference{ProviderID: "codex", Priority: 0, Disabled: false},
		domain.AgentProviderPreference{ProviderID: "claude", Priority: 1, Disabled: true},
	)
	uc.setConnected(map[string]bool{"codex": true, "claude": false, "gemini": true})

	got, err := uc.ResolveProviders(context.Background())
	require.NoError(t, err)

	ids := providerIDs(got) // helper: []string of .ID in order
	require.Equal(t, []string{"codex", "claude", "gemini"}, ids, "preferenced first in priority order, unpreferenced appended by id")
	require.True(t, got[0].Enabled)                // codex enabled
	require.True(t, got[0].Connected)              // codex installed
	require.False(t, got[1].Enabled)               // claude disabled
	require.False(t, got[1].Connected)             // claude not installed
	require.True(t, got[2].Enabled)                // gemini defaults to enabled
}
```

- [ ] **Step 3: Run it — expect FAIL** (`ResolveProviders` undefined).

Run: `go -C api test ./internal/app/usecases/agent/ -run TestResolveProviders`
Expected: FAIL.

- [ ] **Step 4: Implement `ResolveProviders`.** Load descriptors via `AllDescriptors(home)` (get `home` from the usecase's app config, not `wsId`), load prefs via the injected store into a `map[string]domain.AgentProviderPreference`, and build + sort:

```go
func (u *Usecase) ResolveProviders(ctx context.Context) ([]dto.AgentProviderDTO, error) {
	descs, err := engineagent.AllDescriptors(u.home())
	if err != nil {
		return nil, err
	}
	prefs, err := u.providerPrefs.FindAll(ctx) // injected store.Store
	if err != nil {
		return nil, err
	}
	byID := make(map[string]domain.AgentProviderPreference, len(prefs))
	for _, p := range prefs {
		byID[p.ProviderID] = p
	}
	out := make([]dto.AgentProviderDTO, 0, len(descs))
	for _, d := range descs {
		p, has := byID[d.ID]
		out = append(out, dto.AgentProviderDTO{
			ID:          d.ID,
			DisplayName: d.DisplayName,
			Icon:        d.Icon,
			Connected:   u.connected(d.Spawn.Cmd), // injected probe; defaults to agent.Connected
			Enabled:     !p.Disabled,               // zero value (no row) => enabled
		})
	}
	// preferenced providers by Priority; unpreferenced (no row) after them, by id.
	rank := func(id string) (int, bool) { p, ok := byID[id]; return p.Priority, ok }
	sort.SliceStable(out, func(i, j int) bool {
		ri, oki := rank(out[i].ID)
		rj, okj := rank(out[j].ID)
		if oki != okj {
			return oki // preferenced (true) sorts before unpreferenced (false)
		}
		if oki && ri != rj {
			return ri < rj
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
```

Inject the preference store and the probe into the usecase constructor (default `u.connected = agent.Connected`). Update the constructor call site(s) in `app/` wiring to pass `GORMStores.AgentProviderPreferences`.

- [ ] **Step 5: Run test — expect PASS.**

Run: `go -C api test ./internal/app/usecases/agent/ -run TestResolveProviders`
Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
git add api/internal/api/v0/dto/agent.go api/internal/app/usecases/agent/
git commit -m "feat(agent): resolve providers with connected+enabled+priority"
```

---

## Task B4: Enriched GET + global PUT endpoint

**Files:**
- Modify: `api/internal/api/v0/endpoints/agent/handlers/providers.go` (GET uses `ResolveProviders`; add `UpdateProviderPreferences`)
- Modify: `api/internal/api/v0/endpoints/agent/routes.go` + the mount in `api/internal/api/v0/router.go` (add the global `settingsRG.PUT`)
- Test: black-box integration test alongside the existing agent-endpoint integration tests (find the pattern — an existing `*_test.go` that spins the router and hits `/v0/...`).

**Interfaces:**
- Consumes: `usecase.ResolveProviders(ctx)`, `usecase.ReplaceProviderPreferences(ctx, []domain.AgentProviderPreference)` (add this usecase method — validates ids against the catalog, upserts the submitted set, deletes stored rows absent from it).
- Produces: `GET .../agent/providers` → enriched ordered list; `PUT /v0/settings/agent/providers` (body `{providers:[{id,disabled}]}`) → resolved list; `400` on unknown id.

- [ ] **Step 1: Write the failing integration test.**

```go
func TestAgentProviders_EnrichedAndPreferences(t *testing.T) {
	srv := newTestServer(t) // existing harness

	// GET returns enriched fields in default (id) order.
	var list []dto.AgentProviderDTO
	srv.GET(t, ".../agent/providers", &list)
	require.NotEmpty(t, list)
	require.Contains(t, []string{"claude", "codex"}, list[0].ID)
	// connected/enabled present (enabled defaults true).
	require.True(t, list[0].Enabled)

	// PUT reorders + disables.
	var resolved []dto.AgentProviderDTO
	srv.PUT(t, "/v0/settings/agent/providers",
		map[string]any{"providers": []map[string]any{
			{"id": "codex", "disabled": false},
			{"id": "claude", "disabled": true},
		}}, &resolved)
	require.Equal(t, "codex", resolved[0].ID)
	require.True(t, resolved[0].Enabled)
	require.Equal(t, "claude", resolved[1].ID)
	require.False(t, resolved[1].Enabled)

	// Unknown id => 400.
	code := srv.PUTStatus(t, "/v0/settings/agent/providers",
		map[string]any{"providers": []map[string]any{{"id": "nope", "disabled": false}}})
	require.Equal(t, http.StatusBadRequest, code)
}
```

- [ ] **Step 2: Run it — expect FAIL** (route 404 / handler missing).

Run: `go -C api test ./... -run TestAgentProviders_EnrichedAndPreferences`
Expected: FAIL.

- [ ] **Step 3: Implement the GET change** — replace the descriptor loop in `Providers` with `descs, err := h.usecase.ResolveProviders(ctx.Request.Context())` and `libs.WriteQueryOK(ctx, descs)` (it already returns DTOs).

- [ ] **Step 4: Implement the PUT handler + usecase method.**

```go
// providers.go
func (h *Handlers) UpdateProviderPreferences(ctx *gin.Context) {
	var body struct {
		Providers []struct {
			ID       string `json:"id"`
			Disabled bool   `json:"disabled"`
		} `json:"providers"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	prefs := make([]domain.AgentProviderPreference, len(body.Providers))
	for i, p := range body.Providers {
		prefs[i] = domain.AgentProviderPreference{ProviderID: p.ID, Priority: i, Disabled: p.Disabled}
	}
	resolved, err := h.usecase.ReplaceProviderPreferences(ctx.Request.Context(), prefs)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryOK(ctx, resolved)
}
```

`ReplaceProviderPreferences`: validate every id against `AllDescriptors` (return a `libs`-mapped bad-request error on unknown), `Save` each submitted row, `Delete` any stored row whose id is not in the submission, then return `ResolveProviders(ctx)`.

- [ ] **Step 5: Mount the route** — in the agent routes/registration, add `settingsRG.PUT("/settings/agent/providers", h.UpdateProviderPreferences)` on the top-level settings group (the same group `/settings/terminal/profiles` uses). If the agent endpoint's `Register` doesn't currently receive `settingsRG`, thread it through from `router.go` (mirror how `terminal.Register` receives `rg`).

- [ ] **Step 6: Run test — expect PASS.** Also run the whole agent package tests to catch regressions.

Run: `go -C api test ./... -run TestAgentProviders_EnrichedAndPreferences` then `go -C api test ./internal/...`
Expected: PASS, no regressions.

- [ ] **Step 7: Commit.**

```bash
git add api/internal/api/v0/
git commit -m "feat(agent): enriched providers GET + global preferences PUT"
```

---

## Task F1: FE types, api client, enabled selector

**Files:**
- Modify: `web/src/features/agent/api/agent-api.ts`
- Modify: `web/src/features/workspace/stores/slices/agent-chats-slice.ts`
- Test: `web/src/__tests__/features/agent/api/agent-api.test.ts` (extend), `web/src/__tests__/features/workspace/stores/slices/agent-chats-slice.test.ts` (or the nearest existing slice test — create mirrored if none)

**Interfaces:**
- Produces: `AgentProvider` now has `connected: boolean` + `enabled: boolean`; `updateProviderPreferences(prefs: ProviderPreference[]): Promise<AgentProvider[]>`; `selectEnabledProviders(state): AgentProvider[]`.

- [ ] **Step 1: Write failing api tests** (mirror the existing `createChat`/`listProviders` test style; mock `apiFetch`).

```ts
it('listProviders maps connected + enabled', async () => {
  mockApiFetch.mockResolvedValueOnce([
    { id: 'codex', displayName: 'Codex', icon: '<svg/>', connected: true, enabled: true },
  ])
  const out = await listProviders('w1')
  expect(out[0]).toMatchObject({ id: 'codex', connected: true, enabled: true })
})

it('updateProviderPreferences PUTs the ordered list and returns providers', async () => {
  mockApiFetch.mockResolvedValueOnce([{ id: 'codex', displayName: 'Codex', icon: '', connected: true, enabled: true }])
  const out = await updateProviderPreferences([{ id: 'codex', disabled: false }, { id: 'claude', disabled: true }])
  expect(mockApiFetch).toHaveBeenCalledWith('/v0/settings/agent/providers', expect.objectContaining({
    method: 'PUT',
    body: JSON.stringify({ providers: [{ id: 'codex', disabled: false }, { id: 'claude', disabled: true }] }),
  }))
  expect(out[0].id).toBe('codex')
})
```

- [ ] **Step 2: Run — expect FAIL.** `bun --cwd web run test -- agent-api`

- [ ] **Step 3: Implement** — extend `AgentProvider` with `connected: boolean; enabled: boolean` (map them in `listProviders`; if `listProviders` currently passes provider objects straight through, ensure the two new fields are carried), add `ProviderPreference` + `updateProviderPreferences` (spec §5.1).

- [ ] **Step 4: Write failing selector test.**

```ts
it('selectEnabledProviders drops disabled, preserves order', () => {
  const state = { agentChats: { providers: [
    { id: 'codex', enabled: true }, { id: 'claude', enabled: false },
  ] } } as any
  expect(selectEnabledProviders(state).map((p) => p.id)).toEqual(['codex'])
})
```

- [ ] **Step 5: Run — expect FAIL**, then add `export const selectEnabledProviders = (s) => s.agentChats.providers.filter((p) => p.enabled)` to the slice.

- [ ] **Step 6: Run — expect PASS.** `bun --cwd web run test -- agent-api agent-chats-slice`

- [ ] **Step 7: Commit.**

```bash
git add web/src/features/agent/api/agent-api.ts web/src/features/workspace/stores/slices/agent-chats-slice.ts web/src/__tests__/features/agent/
git commit -m "feat(agent): FE provider connected/enabled types + preferences client"
```

---

## Task F2: Unified "New chat" row in the sidebar

**Files:**
- Modify: `web/src/features/agent/components/agent-chats-panel.tsx` (replace the `providers.map(NewChatRow)` loop, ~lines 256-267, with one row)
- Test: `web/src/__tests__/features/agent/components/agent-chats-panel.test.tsx` (extend; the file already mocks `@tanstack/react-virtual` to render all items)

**Interfaces:**
- Consumes: `selectEnabledProviders` (F1), existing `createChat`.
- Produces: exactly one "New chat" row that opens the first enabled provider (rendered nothing when there are no enabled providers).

- [ ] **Step 1: Write failing tests** (read the current panel test to reuse its render harness + fixtures).

```ts
it('renders exactly one New chat row and opens the first enabled provider', async () => {
  // seed providers: claude(enabled, priority 0), codex(enabled) — claude is first enabled
  renderPanel(/* providers with enabled flags */)
  const rows = screen.getAllByTestId ... // however the panel tests query New rows
  expect(rows).toHaveLength(1)
  await userEvent.click(rows[0])
  expect(createChat).toHaveBeenCalledWith('w1', 'claude')
})

it('does not render a New chat row when no provider is enabled', () => {
  renderPanel(/* all providers enabled:false */)
  expect(screen.queryBy...('new-chat')).toBeNull()
})
```

- [ ] **Step 2: Run — expect FAIL.** `bun --cwd web run test -- agent-chats-panel`

- [ ] **Step 3: Implement** — replace the per-provider map with a single row. Read `enabled` providers via a narrow selector (`useStore(store, selectEnabledProviders)`); compute `const primary = enabledProviders[0]`; render one `NewChatRow` labelled just "New chat" that calls `newChat(primary)` when `primary` exists, else render nothing. Keep the hairline separator logic (show separator when there are chats *and* a primary provider). Simplify `NewChatRow`'s label to a constant "New chat" (drop the per-provider `New {displayName} chat`), keeping the leading glyph as the primary provider's icon (or a neutral "＋"/new-chat glyph — match the New Tab surface's New-chat row for consistency; read `new-tab-view.tsx` to reuse its label/glyph).

- [ ] **Step 4: Run — expect PASS.** `bun --cwd web run test -- agent-chats-panel`

- [ ] **Step 5: Commit.**

```bash
git add web/src/features/agent/components/agent-chats-panel.tsx web/src/__tests__/features/agent/components/agent-chats-panel.test.tsx
git commit -m "feat(agent): unify sidebar into a single New chat row"
```

---

## Task F3: New-tab + ⌘⇧N pick the first enabled provider

**Files:**
- Modify: `web/src/features/panes/components/new-tab-view.tsx` (its `createNewChat` — read current post-"ASCII backdrop" state first)
- Modify: `web/src/features/panes/hooks/use-pane-keyboard.ts` (the ⌘⇧N handler)
- Test: extend the mirrored tests for both (`web/src/__tests__/features/panes/...`)

**Interfaces:**
- Consumes: `selectEnabledProviders` (F1).
- Produces: both surfaces open the first *enabled* provider; no-op when none enabled.

- [ ] **Step 1: Write failing tests** — assert that with `[claude(disabled), codex(enabled)]`, the surface opens `codex`, not `claude`.

```ts
it('new-tab New chat opens the first ENABLED provider', async () => {
  // providers: claude disabled, codex enabled
  renderNewTab(...)
  await userEvent.click(screen.getBy...('new-chat'))
  expect(createChat).toHaveBeenCalledWith('w1', 'codex')
})
```

- [ ] **Step 2: Run — expect FAIL.** `bun --cwd web run test -- new-tab use-pane-keyboard`

- [ ] **Step 3: Implement** — in both files replace `const provider = providers[0]` with the first enabled provider: read `providers` from the store then `.find((p) => p.enabled)` (or use `selectEnabledProviders(state)[0]`). Keep the existing `if (!provider) return` guard.

- [ ] **Step 4: Run — expect PASS.** `bun --cwd web run test -- new-tab use-pane-keyboard`

- [ ] **Step 5: Commit.**

```bash
git add web/src/features/panes/ web/src/__tests__/features/panes/
git commit -m "feat(panes): New chat surfaces pick the first enabled provider"
```

---

## Task F4: Register the "Providers" settings tab

**Files:**
- Modify: `web/src/features/window/stores/ui-state-store.ts` (add `'providers'` to the `SettingsTab` union)
- Modify: `web/src/features/settings/components/settings-tab-items.ts` (add the tab item — use a Phosphor icon; the app reverted to Phosphor)
- Modify: `web/src/features/settings/components/settings-dialog.tsx` (import + `case 'providers'`)
- Create: `web/src/features/settings/components/tabs/providers-settings.tsx` (stub in this task — a titled `Section` with the provider list placeholder; F5 fills it in)
- Test: extend the settings-dialog test (or create mirrored) asserting the Providers tab renders.

**Interfaces:**
- Produces: `SettingsTab` includes `'providers'`; `SETTINGS_TAB_ITEMS` includes it; `settings-dialog` routes to `<ProvidersSettings />`.

- [ ] **Step 1: Write failing test** — selecting the `'providers'` tab renders a "Providers" section heading.

- [ ] **Step 2: Run — expect FAIL.** `bun --cwd web run test -- settings-dialog`

- [ ] **Step 3: Implement** the three registration touchpoints + a minimal `ProvidersSettings` that renders `<Section title="Providers" …>` (follow `git-settings.tsx`).

- [ ] **Step 4: Run — expect PASS.**

- [ ] **Step 5: Commit.**

```bash
git add web/src/features/window/stores/ui-state-store.ts web/src/features/settings/
git commit -m "feat(settings): register the Providers tab"
```

---

## Task F5: Providers tab — status, toggle, drag-reorder, persist

**Files:**
- Modify: `web/src/features/settings/components/tabs/providers-settings.tsx`
- Create (optional): `web/src/features/settings/components/tabs/sortable-provider-row.tsx` (the `useSortable` row, if it keeps the tab file focused — react-doctor `no-giant-component`)
- Test: `web/src/__tests__/features/settings/components/tabs/providers-settings.test.tsx`

**Interfaces:**
- Consumes: `updateProviderPreferences` + `AgentProvider` (F1); the active workspace store's `providers` + `setAgentProviders`; `@dnd-kit/sortable` (pattern: `tab-bar.tsx` / `sortable-editor-tab.tsx`).
- Produces: the working Providers tab.

- [ ] **Step 1: Write failing tests.** Mock `updateProviderPreferences`; seed the active workspace store with two providers.

```ts
it('renders one row per provider with connected + enabled state', () => {
  seedProviders([
    { id: 'codex', displayName: 'Codex', icon: '', connected: true, enabled: true },
    { id: 'claude', displayName: 'Claude', icon: '', connected: false, enabled: false },
  ])
  renderTab()
  expect(screen.getByText('Codex')).toBeInTheDocument()
  expect(screen.getByTestId('provider-connected-codex')).toHaveAttribute('data-connected', 'true')
  expect(screen.getByTestId('provider-connected-claude')).toHaveAttribute('data-connected', 'false')
})

it('toggling enable sends the full ordered preference list and reconciles the store', async () => {
  seedProviders([
    { id: 'codex', displayName: 'Codex', icon: '', connected: true, enabled: true },
    { id: 'claude', displayName: 'Claude', icon: '', connected: true, enabled: true },
  ])
  updateProviderPreferences.mockResolvedValueOnce([
    { id: 'codex', displayName: 'Codex', icon: '', connected: true, enabled: false },
    { id: 'claude', displayName: 'Claude', icon: '', connected: true, enabled: true },
  ])
  renderTab()
  await userEvent.click(screen.getByTestId('provider-toggle-codex'))
  expect(updateProviderPreferences).toHaveBeenCalledWith([
    { id: 'codex', disabled: true },
    { id: 'claude', disabled: false },
  ])
  // store reconciled from the response:
  expect(activeStore.getState().agentChats.providers[0].enabled).toBe(false)
})
```

(Reorder is exercised by calling the same commit path the dnd `onDragEnd` uses — factor the "build prefs from ordered ids + toggles, PUT, reconcile" into a plain function and test it directly; a full pointer-drag through `@dnd-kit` in jsdom is brittle, so unit-test the reorder→payload mapping, mirroring how `use-tab-drag` is tested.)

- [ ] **Step 2: Run — expect FAIL.** `bun --cwd web run test -- providers-settings`

- [ ] **Step 3: Implement** — read providers from the active workspace store (narrow selector); render `DndContext` + `SortableContext` (vertical) over the provider rows. Each row: provider glyph + `displayName`, a connected dot (`data-testid="provider-connected-<id>"`, `data-connected`), an enable `Switch` (`data-testid="provider-toggle-<id>"`), a drag handle. A single `commit(orderedIds, disabledMap)` builds `ProviderPreference[]` in order, calls `updateProviderPreferences`, and on resolve calls `setAgentProviders(result)` on the active store. `onDragEnd` reorders local ids then `commit`s; the toggle flips one id's disabled then `commit`s. Keep the tab file within `no-giant-component` — extract `SortableProviderRow` if needed.

- [ ] **Step 4: Run — expect PASS.** `bun --cwd web run test -- providers-settings`

- [ ] **Step 5: Run react-doctor to confirm 100/100.** `bun --cwd web run doctor`

- [ ] **Step 6: Commit.**

```bash
git add web/src/features/settings/components/tabs/ web/src/__tests__/features/settings/
git commit -m "feat(settings): Providers tab — status, toggle, drag-reorder, persist"
```

---

## Task G: Full-suite verification (orchestrator)

- [ ] `go -C api test ./internal/...` — all backend green.
- [ ] `bun --cwd web run test` — full vitest suite green (no regressions vs the prior 2032-test baseline plus the new tests).
- [ ] `bun --cwd web x tsc --noEmit` — clean.
- [ ] `bun --cwd web run lint` — eslint + prettier clean (run `bun --cwd web run format` first if needed, but never touch `package.json`/`bun.lock`).
- [ ] `bun --cwd web run doctor` — react-doctor 100/100.
- [ ] Manual/live consideration: note that jsdom can't exercise the real `@dnd-kit` pointer drag or live PATH probing; flag live Tauri verification as the honest remaining step (do not claim the UI works live until sampled in the running dev app).

---

## Self-Review (against spec)

- **§3.1 enriched GET** → B3 (DTO) + B4 (handler). **§3.2 PUT** → B4. ✓
- **§4.1 domain** → B1. **§4.2 store** → B1. **§4.3 probe** → B2. **§4.4 resolution** → B3. **§4.5 DTO** → B3. **§4.6 handlers/routes** → B4. ✓
- **§5.1 types/api** → F1. **§5.2 selector** → F1. **§5.3 unified New chat (panel/new-tab/keyboard)** → F2 + F3. **§5.4 settings tab** → F4 + F5. ✓
- **§6 tests** — every listed test has a home in B1–B4 / F1–F5. ✓
- **Type consistency** — `AgentProviderDTO`/`AgentProvider` fields (`connected`,`enabled`), `updateProviderPreferences`, `selectEnabledProviders`, `ResolveProviders`, `ReplaceProviderPreferences` names used identically across tasks. ✓
- **No placeholders** — every code step shows code; test bodies concrete (the few `screen.getBy...` ellipses defer to the panel/settings test harness the implementer reads first, by design, not as a TODO). ✓
