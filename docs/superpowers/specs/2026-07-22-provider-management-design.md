# Provider Management — Design

**Date:** 2026-07-22
**Status:** Approved, ready for implementation
**Scope:** Give Crowbar a first-class model of its agentic providers (Claude, Codex, …): whether each is *connected* (installed), a user-controlled *priority* order, and a per-provider *enabled/disabled* toggle — surfaced in a new Settings group, and consumed by a single unified "New chat" action.

---

## 1. Problem

Today the frontend renders one "New <provider> chat" row per provider and every caller that "just opens a chat" independently guesses `providers[0]` as the best provider. There is no notion of:

- **Connected status** — Crowbar never checks whether a provider's CLI is installed; a missing CLI only surfaces as a `424` at spawn time.
- **Priority** — provider order is a hardcoded `id` sort in the backend descriptor enumeration.
- **Enable/disable** — every known provider is always offered.

The user wants: one "New chat" row, and a Settings group where providers can be seen (with connection status), reordered by priority, and disabled.

## 2. Guiding decisions (locked with the user)

1. **Connected = installed only.** The CLI binary resolves on `PATH`. No auth probe (the `claude`/`codex` CLIs expose no reliable machine-readable auth check). A logged-out-but-installed CLI reads *connected* and fails at spawn — acceptable; the existing `424` toast explains it.
2. **Disabled = hidden entirely.** A disabled provider drops out of every "New chat" surface. Already-running chats on a disabled provider keep running, untouched.
3. **Priority + enabled state is global** (per user/machine), not per-workspace — the CLIs are machine-level, so a per-workspace order would surprise.
4. **The backend owns the resolved list.** `GET .../agent/providers` returns the catalog already enriched (`connected`, `enabled`) and already in priority order. The frontend stays dumb: every surface consumes that one ordered list.
5. **Reorder UI = drag-and-drop**, via `@dnd-kit/sortable` (the codebase's existing sortable-list pattern — `tab-bar.tsx`).
6. **New-chat pick = first enabled provider in priority order.** `connected` is informational only for MVP and does not reorder the pick; an uninstalled top provider still gets tried and surfaces the existing `424`.

## 3. Wire contract

Two endpoints. The **read** enriches the existing workspace-scoped route (no change to the FE fetch path — the store already calls it). The **write** is a new *global* settings route (priority/enabled are global), mirroring `/v0/settings/terminal/profiles`.

### 3.1 Read — enriched, existing route

`GET .../workspaces/:wsId/agent/providers` → `data: AgentProviderDTO[]`, **in priority order**, including disabled providers:

```jsonc
[
  { "id": "codex",  "displayName": "Codex",  "icon": "<svg…>", "connected": true,  "enabled": true  },
  { "id": "claude", "displayName": "Claude", "icon": "<svg…>", "connected": false, "enabled": false }
]
```

- `connected` — the provider's `spawn.cmd` resolves to a real executable on `PATH`.
- `enabled` — `!disabled` from the global preference (default `true` for a provider with no stored preference).
- Priority is **implicit in array order**.
- `wsId` is retained for surface compatibility but the list is workspace-independent (it already is today).

### 3.2 Write — new global route

`PUT /v0/settings/agent/providers` — body is the full ordered preference set:

```jsonc
{ "providers": [ { "id": "codex", "disabled": false }, { "id": "claude", "disabled": true } ] }
```

- Array order defines the new priority (index → `Priority`).
- The submission is the **complete** ordered set of known providers; the handler replaces the whole preference table (upsert submitted rows, delete any stored row whose id is absent from the submission → reverts to default).
- Unknown provider ids → `400`.
- **Response:** `data: AgentProviderDTO[]` — the freshly resolved list (same enrichment/order as §3.1), so the client reconciles from server truth with no second fetch.

## 4. Backend

Module root `api/`. Mirrors the `TerminalProfile` global-settings stack.

### 4.1 Domain — `api/internal/domain/agent_provider_preference.go`

```go
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

### 4.2 Persistence — `api/internal/app/gorm.go`

Add a field `AgentProviderPreferences store.Store[domain.AgentProviderPreference, string]`, construct it with `storesqlite.NewFromDB[...](db)` (auto-migrates the table), and assign it into the returned `GORMStores`. No adapter code — the generic `Store[T,K]` (`Save`/`Delete`/`FindByKey`/`FindAll`) is reused.

### 4.3 Connected probe — `api/internal/engine/agent` (or a small `providerstatus` helper)

```go
// connected reports whether a provider's CLI is installed: its spawn.cmd resolves
// to an executable file on PATH (binpath.Resolve handles the daemon's minimal PATH
// by also probing well-known bin dirs). Install-only — no auth check.
func connected(cmd string) bool {
	resolved := binpath.Resolve(cmd)          // core/binpath
	info, err := os.Stat(resolved)
	return err == nil && !info.IsDir()
}
```

### 4.4 Resolution — agent usecase (`api/internal/app/usecases/agent/…`)

A method that produces the enriched, ordered list from three inputs — the descriptor catalog, the stored preferences, and the connected probe — with **no `wsId` dependence** (providers are global; crowbar home comes from app config):

1. `descs := engineagent.AllDescriptors(home)` — catalog `{id, displayName, icon, spawn.cmd}`.
2. `prefs := store.FindAll()` → `map[id]AgentProviderPreference`.
3. Per descriptor: `connected = connected(d.Spawn.Cmd)`; `enabled = !prefs[id].Disabled`; sort key = `prefs[id].Priority` if a row exists, else `+∞`.
4. Sort by `(hasPref ? priority : +∞)`, tie-break by descriptor id → preferenced providers in saved order, new/unpreferenced ones appended by id, all enabled by default.
5. Return `[]AgentProviderDTO` enriched.

The existing workspace-scoped `ListProviders(ctx, wsId)` delegates to this (ignoring `wsId` for ordering, which it already may). The global `PUT` handler calls the same resolver for its response.

### 4.5 DTO — `api/internal/api/v0/dto/agent.go`

Extend `AgentProviderDTO`:

```go
type AgentProviderDTO struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Icon        string `json:"icon"`
	Connected   bool   `json:"connected"`
	Enabled     bool   `json:"enabled"`
}
```

### 4.6 Handlers + routes

- **Read:** the existing `GET .../agent/providers` handler now returns the resolved (enriched, ordered) list from §4.4 instead of raw descriptors.
- **Write:** new handler for `PUT /v0/settings/agent/providers`, mounted on the top-level settings group (`settingsRG`, like `/settings/terminal/profiles`). Binds `{ providers: [{id, disabled}] }`, validates ids against the descriptor catalog (`400` on unknown), replaces the preference table, returns the resolved list. Envelope via `libs.WriteQueryOK` / `libs.WriteErr`.

## 5. Frontend

Module root `web/`.

### 5.1 Types + api — `web/src/features/agent/api/agent-api.ts`

- Extend `AgentProvider` with `connected: boolean` and `enabled: boolean`; map them in `listProviders`.
- Add:

```ts
export interface ProviderPreference { id: string; disabled: boolean }

export async function updateProviderPreferences(
  prefs: ProviderPreference[],
): Promise<AgentProvider[]> {
  const raw = await apiFetch<AgentProvider[]>(`/v0/settings/agent/providers`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ providers: prefs }),
  })
  return raw ?? []
}
```

(Global path, no `wsId` — mirrors the global helpers in `web/src/lib/api.ts`.)

### 5.2 Store selector — `web/src/features/workspace/stores/slices/agent-chats-slice.ts`

The `providers` array already carries the enriched, ordered list once §4 lands. Add a small pure selector for the enabled subset (used by every New-chat surface):

```ts
export const selectEnabledProviders = (s) => s.agentChats.providers.filter((p) => p.enabled)
```

### 5.3 Unified "New chat" — three surfaces

Replace each `providers[0]` / `providers.map(NewChatRow)` guess with **first enabled provider**:

- **`agent-chats-panel.tsx`** — replace the `providers.map(...)` loop (one row per provider) with a **single** "New chat" row that opens the first enabled provider; render nothing when no provider is enabled. The row lives in the same static footer position (below the virtualized list, behind the hairline separator).
- **`new-tab-view.tsx`** — `createNewChat` uses the first enabled provider (read current post-"ASCII backdrop" state before editing).
- **`use-pane-keyboard.ts`** — the ⌘⇧N handler uses the first enabled provider.

`createChat(wsId, provider.id)` is unchanged (still takes an explicit id).

### 5.4 Settings group — "Providers"

Register a new tab (same three-touchpoint pattern as every existing tab):

1. `web/src/features/window/stores/ui-state-store.ts` — add `'providers'` to the `SettingsTab` union.
2. `web/src/features/settings/components/settings-tab-items.ts` — add `{ id: 'providers', label: 'Providers', icon: <Phosphor icon> }`.
3. `web/src/features/settings/components/settings-dialog.tsx` — import + `case 'providers'`.

New component `web/src/features/settings/components/tabs/providers-settings.tsx`:

- Reads the enriched provider list from the **active workspace store** (there is always an active workspace when settings is open).
- One row per provider (in priority order): provider glyph + name, a **connected indicator** (filled dot = connected, hollow/muted = not), an **enable `Switch`**, and a **drag handle** for reorder.
- **Reorder** via `@dnd-kit/sortable` (`DndContext` + `SortableContext` + `useSortable`, following `tab-bar.tsx` / `sortable-editor-tab.tsx`).
- On any change (reorder or toggle): build the ordered `ProviderPreference[]` from current row order + toggles, call `updateProviderPreferences(prefs)`, and push the returned resolved list into the active workspace store via `setAgentProviders` (server-truth reconciliation, the codebase's lean). This is the **first backend-persisted settings tab** on the FE — it deliberately does *not* use the localStorage `updateSetting` path, because provider state is server-owned.

## 6. Testing (TDD throughout)

### Backend
- **Preference store round-trip** — save/list/replace via the generic store (table auto-migrates).
- **Resolution ordering** — preferenced providers in saved order; new/unpreferenced descriptors appended by id; `enabled` reflects `disabled`; a provider with no row defaults to enabled + appended.
- **Connected probe** — `connected("codex")` true when a stub executable is on the probe path, false when absent (drive via a temp dir on `PATH` / the well-known dirs `binpath` probes). Isolate so it never depends on the host having `claude`/`codex`.
- **PUT handler** — replaces the table (upsert submitted, delete omitted), `400` on unknown id, response is the resolved list. Black-box integration test under `api/…/tests` per repo convention (`TestRegression_`/integration tag as appropriate).
- **Enriched GET** — returns `connected`/`enabled` and priority order.

### Frontend
- **`listProviders`/`updateProviderPreferences`** — parse the new fields; PUT sends `{providers:[{id,disabled}]}` in order and returns the mapped list.
- **`selectEnabledProviders`** — filters disabled out, preserves order.
- **Unified New-chat row** — renders exactly one row; opens the first *enabled* provider; renders nothing when none enabled; disabled provider never offered. (Reuse the panel test's `@tanstack/react-virtual` all-items mock.)
- **New-tab + ⌘⇧N** — open the first enabled provider (not a disabled one).
- **Providers settings tab** — renders one row per provider with the right connected indicator + toggle state; toggling a provider and reordering both produce the correct ordered `ProviderPreference[]` PUT payload and reconcile the store from the response.

### Gates (run at the end, full)
`bun --cwd web run test` (vitest, full suite) · `bun --cwd web x tsc --noEmit` · `bun --cwd web run lint` (eslint + prettier --check) · `bun --cwd web run doctor` (react-doctor 100/100) · `go -C api test ./internal/...`. Never run `bun install` (another agent has `package.json`/`bun.lock` uncommitted for the Plate editor).

## 7. Non-goals (YAGNI)

- No auth/login status (install-only, decided).
- No auto-reprobe/polling of connected status — resolved on each `GET` (cheap PATH stat); a manual "recheck" is not needed for MVP.
- No per-provider `status_check` descriptor hook (a future extension if a CLI ever grows a real auth check).
- No cross-workspace live push of preference changes — the active workspace store is reconciled from the PUT response; other workspaces refresh their provider list on next open. (Documented, acceptable.)
- No server-side refusal to spawn a disabled provider — the FE simply never offers it (belt-and-suspenders hardening deferred).

## 8. Conflict awareness (shared workspace)

- Backend work is entirely under `api/` — no overlap with concurrent FE work.
- Another agent has `web/package.json` + `web/bun.lock` uncommitted (Plate editor deps). This feature adds **no dependencies** (`@dnd-kit/sortable` is already installed and used). Do not touch those files; do not run `bun install`.
- `new-tab-view.tsx` was recently changed (ASCII backdrop) — read its current state before editing.
