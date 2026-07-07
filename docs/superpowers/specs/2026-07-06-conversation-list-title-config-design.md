# Iteration 3 — Conversation List, Chat Titles & Crowbar Prompts Config

**Date:** 2026-07-06
**Status:** design, pending approval
**Builds on:** [`2026-07-06-descriptor-v2-design.md`](./2026-07-06-descriptor-v2-design.md) (the agentic engine + hook-derived ledger). This spec adds the *conversation-list* surface, chat *titles*, and moves Crowbar's agent-facing *prompts* into config.

---

## 0. In plain terms

There have been three attempts at "Crowbar conversations." Two are dead and get deleted; the third is the keeper and gets rebuilt on the real engine:

1. **`markdown-chat`** — a rich chat-bubble UI driven by a **mock** `/v0/runs` API that has no backend. Dead brainstorming code → **delete**.
2. **The dead chat REST + CRUD usecase** — `endpoints/chats` (never mounted, D11) + `usecases/chat` (only that endpoint used it). → **delete**. (Note: `domain.Chat` + its repo are NOT dead — Branch Review reads them — so they stay; see §1.2.)
3. **The sidebar "Chats" list** — this is where Crowbar shows the user their conversations. Terminal-first: clicking a conversation opens/focuses the real CLI's terminal. It gets **repointed onto the agentic engine (`/v0/agent/*`)**. (FE build is a **later phase** — this spec prepares the backend for it.)

Two new capabilities land now, both backend:
- **Chat titles.** Every `AgentChat` gets a human title. We don't read it out of the providers' private stores — instead we **tell the agent to name the chat** by running a `crowbar chat … rename` command; a derived first-prompt title is the fallback.
- **Prompts in config.** The agent-facing prose Crowbar injects (the title instruction, the hand-off wrapper) moves out of Go and into `~/.crowbar/config.yaml`.

**Scope of *this* spec's implementation:** cleanup (1 & 2) + titles + prompts config. The terminal-first sidebar FE is designed here but **not built yet** ("no frontend code yet — cleanup and prep").

---

## 1. Removals (cleanup)

### 1.1 Frontend — `markdown-chat` + mock runs (iteration 1)
Delete `web/src/features/markdown-chat/` (whole feature), `web/src/routes/_shell/chat/$chatId.tsx`, `web/src/lib/api/run.ts`, `web/src/mocks/handlers/markdown-chat.ts`, `web/src/lib/mock/markdown-chat.ts`, and the mirrored tests under `web/src/__tests__/features/markdown-chat/`. Surgically excise the `crowbarChat` buffer kind from the shared pane/buffer system (`pane-content.ts` union member, `pane-container.tsx` render case, `buffer-slice.ts` open/close branches, `workspace-store-registry.ts`, tab bar/new-button, mock scenarios' `markdownTurns`, chaos `FaultKey`), and delete the already-orphaned `editor/stores/buffer-content-factory.ts` (zero importers, references `crowbarChat`). Per project convention (pre-production, no legacy migration) do **not** add stale-snapshot guards; dev clears its own persisted state.

### 1.2 Backend — dead chat REST + CRUD usecase (iteration 2)
**Correction from code inspection:** `domain.Chat` and `app/repositories/chat/` are **NOT dormant** — they are live infrastructure for Branch Review (`app/usecases/branchreview` reads `repos.Chat` → `domain.Chat` and projects it via `branchchat`; the `review` endpoint is mounted; `container.go:42` builds the chat asynx store). Deleting them would break Branch Review.

So iteration-2 removal is only the genuinely-dead surface:
- **Delete** `api/internal/api/v0/endpoints/chats/` — never mounted (`router.go` never calls `chats.Register`; D11).
- **Delete** `api/internal/app/usecases/chat/` — the CRUD usecase (create/fork/rename/delete/list). Consumed only by the never-mounted `endpoints/chats` and its own container wiring; branchreview uses the *repo* directly, not this usecase.
- **Unwire** the chat usecase from `app/usecases/container.go` (drop the `usecases/chat` import, the `chatUsecase := chat.New(repos.Chat, …)` construction, and the `Chat: chatUsecase` field), while **keeping** the `repos.Chat` argument passed to branchreview construction on the same file.

**Keep (live — do NOT touch):** `domain.Chat`, `app/repositories/chat/`, `app/usecases/branchreview`, `app/usecases/internal/branchchat/`, `domain.BranchChat`, the chat asynx store wiring. `domain.Chat`'s AutoMigrate stays.

### 1.3 The sidebar-chats FE seam
The current sidebar chats UI (`components/layout/chat-tree*.tsx`, `lib/store/chat-list-store.ts`, `lib/api/chat.ts`, the "Chats" tab, `ide-shell.tsx`'s `/chat/:id` branch) is wired entirely to the dead system (dead `/v0/workspaces/:wsId/chats` routes, opens `crowbarChat`). Its *concept* is the keeper; its *code* is dead-wired.

**Decision (flagged for review):** because it is entirely dead-wired and will be rebuilt terminal-first on `/v0/agent/*`, this spec treats the current sidebar-chats FE as **removed during cleanup and rebuilt fresh in the FE phase**, rather than left dormant/broken. If you'd rather keep and rewire the existing components, say so — it changes the cleanup surface but not the backend prep below.

---

## 2. Iteration-3 target (FE phase — designed, not built here)

Terminal-first conversation list, for context:
- The sidebar lists `AgentChat`s (`GET /v0/agent/chats`, scoped per workspace). Each row shows the **title** (§3) and a live **running/idle** state derived from the lifecycle WS (`user_prompt` → running, `turn_stop` → idle).
- Clicking a conversation **opens/focuses its live CLI terminal** — the active segment's `TerminalSessionID`, rendered via the existing terminal WS. "New conversation" = `POST /v0/agent/chats` (spawn a provider CLI) → open its terminal.
- Provider switch and rename act on the chat (`POST /chats/:id/switch`, `PATCH /chats/:id`).
- **Out of scope for iteration 3:** fork/`parentId` and `workflow` chat types (present in the old `ProjectChat` model, absent from `AgentChat`; deferred until there's a real need).

This section is the north star for §3–§4's backend prep; no FE is written now.

## 3. Chat titles

### 3.1 Source: the agent names its own chat
No CLI hook carries a title, and reading each provider's private title store (claude's transcript `ai-title`, codex's `state_5.sqlite` `threads.title`) is fragile and per-provider. Instead we invert it: **Crowbar instructs the agent to name the chat**, via an injected system-prompt line telling it to run a `crowbar` command. This is provider-agnostic (works for any CLI that runs shell commands) and needs zero per-provider title logic.

- **Command:** `crowbar chat <chatid> rename "<title>"` — a new hidden CLI subcommand (sibling of `crowbar hook`). It POSTs to the daemon and, like `crowbar hook`, **never breaks the CLI** (errors swallowed to exit-0, stderr only). `<chatid>` is Crowbar's real chat id, injected into the instruction at spawn (see §4).
- **Endpoint:** `POST /v0/agent/chats/:id/rename` `{ "title": "..." }`. Mutation-as-POST matches the existing `/chats/:id/switch` convention, and the ipc client is POST-only. This same endpoint serves the agent, a human via CLI, and the future FE rename — one surface.
- **Routing by chat id (not segment):** the title belongs to the chat. `{chatid}` is a guaranteed spawn variable (`chat.ID` is known at spawn). Accepted tradeoff: it's pinned to the chat live at spawn, so if a user `/clear`s mid-session and the agent re-titles, it renames the *original* chat and the new chat falls back to its derived title (§3.3) — graceful, not broken, and `/clear`-then-agent-retitle is rare.

### 3.2 Precedence & set-once
`AgentChat` gains a `TitleLocked bool` alongside `Title`. Precedence **user > agent > derived**:
- **Derived fallback** sets `Title` only when it is empty.
- **Agent rename** (`crowbar chat … rename`) sets `Title` unless `TitleLocked` — it may upgrade a derived title, never clobber a user title.
- **User/FE rename** sets `Title` **and** `TitleLocked = true`.

The rename endpoint distinguishes these with a query flag: `?source=agent` applies the agent rule (skip if locked); the default (FE/user) sets unconditionally and locks. The agent's injected command uses `?source=agent`.

### 3.3 Derived fallback (first prompt)
On the **first `user_prompt` hook for a chat**, if `Title == ""`, set `Title` to a short derivation of the prompt (first non-empty line, trimmed, capped ~60 chars). This runs in the usecase's existing `user_prompt` ingest path (which already appends the ledger turn). It guarantees every chat has a title even if the agent skips the rename; the agent's later rename upgrades it (§3.2). Empty/whitespace prompt → leave empty (FE shows a placeholder).

### 3.4 Broadcast
On any title change (`derived`, `agent`, `user`), broadcast an `AgentChatEvent{chatId, kind:"titled"}` on the existing `/v0/agent/ws/chats` feed so the (future) list refetches.

## 4. Crowbar prompts in config

### 4.1 What moves to config
Two agent-facing prompts, both provider-agnostic prose, move out of Go into the existing config package (`api/internal/core/config`, loaded from embedded `default.yaml` overlaid by `~/.crowbar/config.yaml`):

1. **`title_instruction`** — the line telling the agent to name the chat.
2. **`handoff_wrapper`** — the wrapper around the handed-off conversation, currently **hardcoded** in `AssembleHandoff` as `"=== HANDED-OFF CONTEXT (Crowbar) ===\n" + blob + "\n=== END ==="`. That prose moves to config.

### 4.2 Config shape
Replace `config.ConfigData`'s contents with a single `prompts` section (the `config:` root already exists and is designed to hold more). The **currently unused `intelligence` section is removed** (verified: `Intelligence`/`GetIntelligence`/`ModelForTier` are consumed only by the config package's own tests — nothing in the app uses them — so they go, per no-dead-code). Config now holds *only* the prompts; future config lands as new sibling sections. Defaults live in the package's embedded `default.yaml`; a user's `~/.crowbar/config.yaml` overlays them.

```yaml
# default.yaml (embedded) — overlaidable by ~/.crowbar/config.yaml
config:
  prompts:
    title_instruction: |
      Give this conversation a short title, once, by running exactly this command:
      {crowbar} chat {chatid} rename "<a concise 2-5 word title of the task>"
    handoff_wrapper: |
      === HANDED-OFF CONTEXT (Crowbar) ===
      {conversation}
      === END ===
```

Placeholders in the prompt templates are expanded by Crowbar at injection time via the engine's existing `Expand` mechanism: `{crowbar}` (the crowbar binary path), `{chatid}` (the chat id), `{conversation}` (the rendered ledger from `RenderConversation`). These join the guaranteed-variable set.

New accessor: `config.GetPrompts() Prompts`. Delete `Intelligence`, `ConfigData.Intelligence`, `GetIntelligence`, `ModelForTier`, and their tests; `ConfigData` becomes `{ Prompts Prompts }`.

### 4.3 How the prompts are injected (descriptor = mechanism, config = text)
The descriptor's `handoff_inject` steps are the single "inject a system-prompt document" mechanism — `--append-system-prompt {handoff}` for claude, positional for codex. Nothing about the mechanism changes; only *what document* fills `{handoff}` differs by situation, and its text comes from config:
- **On a switch (`SwitchProvider`, as today):** `{handoff}` = `AssembleHandoff` output, which now expands `config.prompts.handoff_wrapper` with `{conversation}` = `RenderConversation()` instead of hardcoding the wrapper.
- **On a fresh spawn (`SpawnChat`):** `{handoff}` = the expanded `config.prompts.title_instruction` (`{crowbar}`/`{chatid}` filled), and the descriptor's `handoff_inject` steps are applied (today `SpawnChat` passes an empty handoff and skips them). A fresh chat has no prior context, so the "system-prompt document" it receives is the title instruction; a switch enters an already-titled chat, so it receives the handoff. The two are mutually exclusive per spawn.

The split stays clean:
- **Descriptor (per-CLI):** how to inject a system-prompt document.
- **Config (global prose):** the title instruction and handoff-wrapper text.
- **Go:** only Crowbar's own contract — the `crowbar chat rename` command name, canonical events, `{…}` variable names.

## 5. Backend changes (implementation surface for this spec)

**Engine / usecase (`app/usecases/agent`, `engine/agent`):**
- `IngestHook` `user_prompt` path: derived-title fallback (§3.3).
- `SpawnChat`: compose + inject the `title_instruction` doc (§4.3); add `{crowbar}`/`{chatid}` to the template context.
- `AssembleHandoff`: config-driven wrapper (§4.3).
- New usecase method `RenameChat(ctx, chatID, title string, source string) error` applying §3.2 precedence.

**Domain:** `AgentChat.TitleLocked bool` (`Title` already exists).

**API (`api/v0/endpoints/agent`):** `POST /v0/agent/chats/:id/rename` → `RenameChat`; `?source=agent` flag; broadcast `titled`.

**CLI (`cmd/crowbar`):** `crowbar chat <chatid> rename <title>` subcommand → `PATCH …` via the ipc client; swallow-errors-to-exit-0 like `crowbar hook`.

**Config (`core/config`):** add `Prompts` struct + `GetPrompts`; `default.yaml` becomes only the `prompts` block; **remove** `Intelligence`, `ConfigData.Intelligence`, `GetIntelligence`, `ModelForTier`, and their tests (dead — verified unused).

## 6. Testing

- **Config:** defaults load; `~/.crowbar/config.yaml` overlays `prompts`; absent prompts keep embedded defaults. The old `intelligence`/`ModelForTier` tests are removed with the section.
- **Title fallback:** first `user_prompt` sets a derived title; second prompt does not overwrite; empty prompt leaves it empty.
- **Precedence:** derived → agent upgrade works; agent rename skips when `TitleLocked`; user rename sets + locks; agent cannot clobber locked.
- **Endpoint:** `POST …/rename` sets/locks correctly per `source`; broadcasts `titled`.
- **CLI:** `crowbar chat <id> rename <t>` posts the right body; a daemon-down error exits 0 with stderr.
- **Handoff:** `AssembleHandoff` uses the config wrapper; expanding `{conversation}` yields the rendered ledger; a custom `~/.crowbar/config.yaml` wrapper is honored.
- **Spawn injection:** a fresh `SpawnChat` injects the expanded title instruction (`{crowbar}`/`{chatid}` filled); a `SwitchProvider` does not.
- **Removals:** whole-module build green; no dangling refs to `crowbarChat`, `markdown-chat`, `endpoints/chats`, `usecases/chat`; **Branch Review still compiles and works** (it keeps using `repos.Chat`/`domain.Chat`); integration suite still green.
- **Live (per project rule):** after green tests, sample a real chat via `make dev-desktop` — confirm the agent runs `crowbar chat … rename` and the title lands, and the fallback fires when it doesn't.

## 7. Out of scope / deferred
- The terminal-first sidebar **FE** (§2) — later phase.
- Reading providers' native titles (transcript `ai-title` / codex sqlite) — rejected in favor of §3.1.
- Fork/`parentId`, `workflow` chat types.

## 8. Flagged decisions for review
1. **Sidebar-chats FE fate (§1.3):** remove-and-rebuild-fresh (spec's default) vs. keep-and-rewire the existing dead-wired components.
2. **Title fallback vs. agent race:** the derived fallback fires on the first `user_prompt`; the agent's rename (usually moments later) upgrades it via precedence (§3.2). If you'd prefer the fallback to *wait* briefly for the agent instead of upgrading, that's a variant — the spec chooses upgrade-always for simplicity.
