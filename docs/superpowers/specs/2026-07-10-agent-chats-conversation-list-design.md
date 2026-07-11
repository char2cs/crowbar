# Agent Chats — Conversation List & Chat Pane (design)

**Date:** 2026-07-10
**Branch:** `feature/agentic-bridge` (backend live as of `2fe783bb`; sidebar shell merged in `6a4f2557`)
**Status:** approved design, ready for implementation planning
**Scope:** full-stack (frontend + four backend additions)

---

## 1. Summary

Build the frontend for Crowbar's agentic chats: a **workspace-scoped "Chats" tab** in the
left sidebar that lists a workspace's agent chats, and a **main-area chat pane** that shows
the selected chat's live agent terminal. Chats can be created (one starter per registered
provider), selected, renamed, reordered, provider-switched (Claude ↔ Codex), and deleted.

No agent-chat UI exists today — the agent only surfaces as a raw xterm pane. This is built
from scratch, mirroring the existing **review-threads live list** (its structural twin) and
the **workspace tree rows** (their visual/interaction twin).

Four small backend additions are in scope (§7). Delete needs **no** backend work — the
existing `DELETE /agent/chats/:id` → `PurgeChat` already erases everything Crowbar-managed.

---

## 2. Corrected backend contract (verified against live code)

The original handoff was stale. Verified against the code on `feature/agentic-bridge`:

- **Workspace-scoped, not top-level.** Routes are under the workspace group
  (`.../projects/:projectId/repos/:repoId/workspaces/:wsId/agent/...`,
  `endpoints/agent/routes.go:30-38`). The FE uses `workspaceBase(wsId)`, exactly like
  review-threads — **not** a literal `/v0/agent/...`.
- **Delete exists and is complete.** `DELETE /agent/chats/:id` → `PurgeChat`
  (`usecases/agent/agent.go:186`) kills the active PTY (best-effort), `Forget`s the aggregate
  (event log + read-model row), **and** removes the on-disk chat dir (ledger + per-segment
  tmp) via home-guarded `RemoveUnderHome`. Nothing to add.
- **WS is workspace-scoped.** `.../agent/ws/chats` filters frames by `workspaceId`
  (`store/hub.go:24-26`); a client only sees its own workspace's chats. The frame now
  carries `workspaceId`.

**Endpoints consumed** (envelope `{success,error?,data?}`, unwrapped by `apiFetch`):

| Method | Path (relative to `workspaceBase(wsId)`) | Purpose |
|---|---|---|
| GET  | `/agent/chats` | list → `AgentChatDTO[]` |
| GET  | `/agent/chats/:id` | detail → `AgentChatDetailDTO` (chat + ordered `segments`) |
| POST | `/agent/chats` | `{provider}` → 201 `{id}` (spawns CLI in a PTY) |
| POST | `/agent/chats/:id/switch` | `{provider}` → 200 `{id}` (ends old segment, hands off, spawns target) |
| POST | `/agent/chats/:id/rename` | `{title}` (+ optional `?source=agent`) → 202; default = user rename, **locks** title |
| DELETE | `/agent/chats/:id` | → 202 (hard delete, full teardown — see above) |
| GET (WS) | `/agent/ws/chats` | lifecycle broadcast |
| GET  | `/agent/providers` | **NEW (§7.2)** → `AgentProviderDTO[]` |

**DTOs** (`api/internal/api/v0/dto/agent.go`, `domain/agent_segment.go`):
- `AgentChatDTO`: `{ id, workspaceId, title, activeSegmentId, createdAt }` **+ `activeProviderId` (§7.3)**
- `AgentChatDetailDTO`: `AgentChatDTO` + `segments: AgentSegment[]`
- `AgentSegment`: `{ id, providerId, providerSessionId?, crowbarSegmentId, terminalSessionId, startedAt, endedAt?, status }`, `status ∈ "active"|"ended"`
- `AgentChatEvent` (WS frame): `{ chatId, workspaceId, kind }` — **bare, no snapshot**

**Broadcast `kind`s** (`store/hub.go:31-40`): `created`, `segment_opened`, `segment_ended`,
`session_bound`, `turn_started`, `turn_stopped`, `title_set`, `deleted`.

**Two facts to design around:**
1. **Working/idle is not in any REST payload.** Derive it client-side: `Map<chatId, boolean>`,
   `true` on `turn_started`, `false` on `turn_stopped`. On fresh load / reconnect a chat's
   working state is unknown until its next turn event → **default idle**.
2. **WS frames carry no data.** React-then-refetch: on `created`/`title_set`/`segment_*`/
   `deleted` → refetch (`GET /agent/chats` or `/:id`); on `turn_started`/`turn_stopped` →
   toggle the working map; on the reconnect sentinel `{reconnected:true}` → reseed via GET.

---

## 3. Frontend architecture

Mirror the review-threads live list end-to-end. Because chats are **per-workspace**, state
lives in the **workspace store registry** (per `CLAUDE.md`), not `lib/store/`.

- **API client** — `web/src/features/agent/api/agent-api.ts`. `apiFetch` + `workspaceBase(wsId)`
  + wire→store mappers. Model on `features/git/api/review-api.ts`.
  Functions: `listChats`, `getChat`, `createChat(provider)`, `switchProvider(id, provider)`,
  `renameChat(id, title)`, `deleteChat(id)`, `listProviders()`.
- **Store slice** — `web/src/features/workspace/stores/slices/agent-chats-slice.ts`.
  Immer `upsert`/`remove` idiom from `branch-review-slice.ts`. Holds:
  `chats: AgentChat[]`, `working: Record<chatId, boolean>`, `order: chatId[]` (client-persisted,
  §5), `activeChatId: string | null`, `providers: AgentProvider[]`.
- **WS hook** — `web/src/features/workspace/stores/hooks/use-workspace-agent-chats-stream.ts`.
  Structural match to `use-workspace-threads-stream.ts`: `seed()` → `wsManager.subscribe(
  \`${workspaceBase(wsId)}/agent/ws/chats\`, cb)` → handle `{reconnected}` (reseed) → per-kind
  handling (refetch vs working-toggle) → cleanup. Reuse `wsManager` (`lib/ws/manager.ts`) and
  `lib/ws/url.ts` as-is. **Not** `lib/ws/entity-stream.ts` (that expects full-DTO frames).
- **Providers** — fetched once per workspace (data is workspace-independent but the route is
  workspace-scoped for consistency, §7.2); cached in the slice; keyed `providerId → {displayName, icon}`.

---

## 4. UI decisions (approved)

### 4.1 Sidebar "Chats" tab
A **4th sidebar tab**, inserted 2nd (order: Workspaces · **Chats** · Files · Git), per the
user's screenshot. Register in the three existing places:
`lib/store/sidebar.ts:68` (`SidebarTab` union), `components/layout/sidebar-tab-bar.tsx:8`
(`TABS`), `components/layout/sidebar-carousel.tsx:83` (carousel panel + order array).
Panel component: `web/src/features/agent/components/agent-chats-panel.tsx`.

### 4.2 Chat rows = workspace-row siblings
Rows reuse the workspace-row kit: `ROW_BASE` / `ROW_ACTIVE` / `ROW_INACTIVE`
(`components/layout/workspace-row-base.ts`). Anatomy (provider-forward):
- **Leading glyph = the provider icon** (from `providers[activeProviderId].icon`), which
  **swaps to the centralized spinner** while the chat is working (§6). No provider color on
  the spinner — it uses a theme token.
- **Title** (prose), single-click **selects** (opens/focuses its pane tab), **double-click**
  renames inline via the existing `WorkspaceInlineInput`.
- **No `⋯` menu, no `×` button.** (The stray `components/layout/sidebar-row.tsx` `ChatRow` —
  a mock with an `age` field and `×` — is **not** reused; delete it or leave it unused.)
- **Drag** = reorder within the list **and** drop-to-delete (§4.5). Nothing else.

### 4.3 "New" rows — one per provider, at the bottom
Below all real chats (thin separator above them), render one row **per registered provider**
from `GET /agent/providers`: `[providerIcon] New {displayName} chat` with the **`+` on the
right edge**. Click → `POST /agent/chats {provider}`. Adding a provider (future user-managed
providers) adds a row automatically — zero code change.

### 4.4 Chat pane = header-less CossUI `Frame`
Selecting a chat opens/focuses a **main-area pane tab** (new pane content type `'agentChat'`,
§4.6) — same mechanism as `branchReview`. The pane renders the vendored CossUI **`Frame`**
(`components/ui/frame.tsx`), **header omitted**, padding stripped (`p-0`, no outer margin):
- **`FramePanel`** (flush, fills the pane) = the live agent terminal.
- **`FrameFooter`** = a single control: the **provider-switch dropdown on the left**. Nothing
  on the right. Switching calls `POST /agent/chats/:id/switch {provider}` and lists providers
  minus the current one (driven by the providers list).

### 4.5 Delete — immediate, full erase (no dialog)
Drag a chat row; a **trash drop-zone** appears in the panel footer (mirror
`components/layout/workspace-tree-footer.tsx` — slides in on drag, "Drop to delete"). Dropping
calls `DELETE /agent/chats/:id` **immediately** (no confirmation, same as workspaces). Backend
`PurgeChat` erases everything Crowbar-managed. On the resulting `deleted` WS frame (or the 202),
remove from the store and **close the chat's pane tab** if open.

### 4.6 Reorder & ordering
Default order = **creation order, newest last** (new chats append at the bottom, above the New
rows). The user can **drag to reorder**; order is **persisted client-side** per workspace
(`order: chatId[]` in the slice, mirrored to local storage) — MVP scope, no backend order
field. Chats absent from the saved order fall back to `createdAt`. (Durable/cross-device order
via a backend field is a future upgrade, out of scope.)

---

## 5. Terminal integration (the one real risk)

A segment's `terminalSessionId` is a **live session in the same terminal-engine registry** the
FE already attaches to (`engine/terminal/terminal.go:487` `CreateCommand` → `reg.Add`; the FE's
`terminalAttach`/`terminalListen` read that registry). So **no new bridge** — but the FE
terminal `sessionId` is a *tab identifier*, not the daemon PTY id, and naively passing it
spawns a **new** PTY. To attach the agent's running PTY, pre-seed the mapping before the
terminal mounts (`terminal-store` `updateSession(tabId, {connectionId: terminalSessionId})` or
`saveReconnect(wsId, tabId, terminalSessionId)`), so `resolveTerminalConnection`
(`features/terminal/components/resolve-terminal-connection.ts`) **attaches** instead of creating.

**Provider switch re-attach:** the pane tab is keyed by the stable `chatId`. When the chat's
`activeSegmentId` changes (switch → `segment_opened` → refetch detail), the pane re-resolves to
the new segment's `terminalSessionId` and re-attaches **in place** (same tab, new session
underneath). This is the spot to **verify live** in the Tauri app (attaching to a full-screen
TUI like Claude Code must replay cleanly).

---

## 6. Centralized spinner (flicker)

One spinner "framework" for **both** the workspace icon and the chat row, using the **actual
flicker library** (flicker.laurie.fyi) — a flip-dot generator (each 5×5 spinner = per-frame
25-cell on/off data), 31 spinners in the 5×5 catalog. **Invent nothing.**

- **Component** — `web/src/components/ui/spinner.tsx` exposes a centralized `<Spinner>` that
  inlines a spinner definition, animates its frames, and **random-picks** one of the catalog
  per instance (like today's `WorkspaceAgentSpinner` random-picks a name).
- **File structure (no codegen script)** — `web/src/components/ui/spinners/` (sibling of
  `spinner.tsx`) holds **one self-contained file per spinner** (the flicker **SVG** export,
  authored with `currentColor` for on-dots and `currentColor` + low opacity for off-dots so a
  **theme token** colors it). The component discovers them via
  `import.meta.glob('./spinners/*.svg', { eager: true, query: '?raw' })` and inlines the SVG
  (so `currentColor` and the SVG's declarative animation both work — an `<img>` would break
  `currentColor`). **Adding a spinner = drop an SVG in the folder.** No index to edit, no
  generation script, no giant file. (If a given flicker export is static-per-frame rather than
  self-animating, that one file stores the frames and the component animates them — same folder,
  same glob.)
- **Theming** — the spinner is colored by a Crowbar theme token (`text-primary` / `currentColor`),
  never a hardcoded or provider color; respects light/dark.
- **Removal** — `@agilek/cli-loaders` and the bespoke `WorkspaceAgentSpinner` internals are
  replaced by `<Spinner>`. `WorkspaceBranchIcon` renders `<Spinner>` when `working`.

---

## 7. Backend additions (full-stack scope)

Four small additions. **Delete is already complete** (§2).

### 7.1 Provider descriptor: `icon` + `display_name`
Add `Icon string \`yaml:"icon"\`` and `DisplayName string \`yaml:"display_name"\`` to
`Descriptor` (`api/internal/engine/agent/descriptor.go:13`) and to both
`descriptors/claude.yaml` and `descriptors/codex.yaml`. The `icon` is an inline SVG string.
Relax `Validate` (`descriptor.go:83`) for these display-only fields (the file's "every field is
load-bearing" invariant gets a documented carve-out).

### 7.2 `GET /agent/providers`
New workspace-scoped route (`endpoints/agent/routes.go`, ignores `:wsId`; kept there for
surface consistency) → `AgentProviderDTO[]` = `[{ id, displayName, icon }]`. Backed by a new
**enumeration** of descriptors (embedded `descriptors/*.yaml` via `ReadDir` over the embed FS +
on-disk `<home>/descriptors/*.yaml` overrides) — none exists today (`ResolveDescriptor` is
lazy-by-id, `descriptors_embed.go`). Feeds the row icon, the New-chat rows, and the switch menu.

### 7.3 `activeProviderId` on `AgentChatDTO`
Add `ActiveProviderID string \`json:"activeProviderId"\`` to `AgentChatDTO`
(`dto/agent.go:12`), derived in `AgentChatDTOFrom` from the active segment (the domain
`AgentChat` already embeds `Segments` + `ActiveSegmentID` — no extra query). Without this the
list can't map a row to its provider without N detail fetches.

### 7.4 Re-derive workspace `working` from agent turns
`domain.Workspace.Working` (`domain/workspace.go:37`) already exists as a "derived,
non-persisted overlay computed at broadcast time" that is **currently always false** ("with the
agent-run concept removed"). **Re-light it:** `Working = true` iff the workspace has any agent
chat with an active turn, computed at broadcast time; re-broadcast the affected workspace on
agent `turn_started` / `turn_stopped`. The FE already renders `workspace.working` on the
context pill and workspace tiles via `WorkspaceBranchIcon` — so once the overlay is populated,
the same centralized spinner (§6) appears there automatically. No new FE wiring for the
workspace side beyond the shared `<Spinner>`.

---

## 8. Component & file inventory

**New (frontend):**
- `features/agent/api/agent-api.ts`
- `features/agent/components/agent-chats-panel.tsx` (list + New rows + trash footer + drag)
- `features/agent/components/agent-chat-row.tsx`
- `features/agent/components/agent-chat-pane.tsx` (Frame + terminal + footer dropdown)
- `features/agent/components/provider-switch-dropdown.tsx`
- `features/workspace/stores/slices/agent-chats-slice.ts`
- `features/workspace/stores/hooks/use-workspace-agent-chats-stream.ts`
- `components/ui/spinners/*.svg` (+ updated `components/ui/spinner.tsx`)

**Modified (frontend):**
- `lib/store/sidebar.ts`, `components/layout/sidebar-tab-bar.tsx`,
  `components/layout/sidebar-carousel.tsx` (register Chats tab)
- `features/panes/types/pane-content.ts` (add `'agentChat'` content type + guard + open spec)
- `features/workspace/stores/slices/buffer-slice.ts` (builder branch for `'agentChat'`)
- `features/panes/components/pane-container.tsx` (render case for `'agentChat'`)
- `components/layout/workspace-branch-icon.tsx` (use `<Spinner>`); remove `@agilek/cli-loaders`
- delete/ignore `components/layout/sidebar-row.tsx` mock `ChatRow`

**Modified (backend):** `engine/agent/descriptor.go`, `descriptors/{claude,codex}.yaml`,
`engine/agent/descriptors_embed.go` (enumeration), `endpoints/agent/routes.go` +
`endpoints/agent/handlers/` (providers handler), `dto/agent.go` (`activeProviderId`), the
workspace broadcast path (`working` re-derivation).

---

## 9. Testing

Per `CLAUDE.md`: tests mirror source under `web/src/__tests__/`, `@/` imports, run under bun
(`bun run test:coverage`), typecheck `bun tsc --noEmit`.

- **Store slice**: upsert/remove/reorder, working-map toggling, provider join.
- **WS hook**: seed, per-kind handling (refetch vs toggle), `{reconnected}` reseed, cleanup —
  drive with real signals, **no timing** (per the project rule).
- **Row/panel**: provider icon → spinner swap on working; New-row-per-provider; drag-to-delete
  closes the pane tab; double-click rename.
- **Backend**: providers enumeration + endpoint; `activeProviderId` mapping; `working`
  re-derivation on turn events (black-box `TestRegression_*` in `api/tests`, integration tag).
- **Live (Tauri)**: create a chat per provider, select → terminal attaches, provider switch
  re-attaches in place, working spinner on row + context pill + tile, drag-reorder, drop-to-delete.
  Verify live before claiming done.

---

## 10. Out of scope / future

- **Parent↔child chat nesting** — the recursive `depth`-based tree (like `WorkspaceTreeItem`)
  the user mentioned "later." MVP list is flat; build rows so nesting can be layered on.
- **User-managed providers** — the per-provider New rows and providers endpoint already
  accommodate it (more descriptors → more rows), but the management UI is later.
- **Durable/cross-device chat order** — MVP order is client-persisted (§4.6).
- **Handoff viewer** (`GET /agent/chats/:id/handoff`) — not surfaced in this iteration.

---

## 11. Open decision captured

**Reorder persistence = client-side for MVP** (§4.6). Flagged for review: if durable order is
wanted now, add a backend order field + reorder command (mutates the event-sourced aggregate).
Default recommendation is client-side to keep backend scope to the four additions above.
