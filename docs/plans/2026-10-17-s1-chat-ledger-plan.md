# S1 — Chat ledger and chat-owned conversation: Implementation Plan (index)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks are strictly sequential: each one assumes every earlier task is committed.

**Goal:** Every fact about a chat — prompt, turn, session binding, runner start/exit, text, tool call, subagent, ask, marker — becomes a row in one append-only per-chat ledger with a server-assigned `seq`, and the daemon's HTTP/WS chat contract that today's web client reads is served from that ledger.

**Architecture:** A pure `domain/ledger` package holds the row vocabulary and the state machines (turn, conversation, prompt) as a fold. An `adapter/store/ledger` SQLite store is the only writer: one in-process lock per chat, `seq` assigned under it, projections (heads, session index) committed in the same transaction, subscribers told after commit in `seq` order. Hook ingress maps each provider event to rows and replies only after the append; the runner becomes an in-memory supervisor whose durable facts are rows; a per-chat WebSocket feed pushes rows. The old read endpoints are projections of the ledger until S2 replaces the client.

**Tech Stack:** Go (`api/`, always `-tags noEmbed`), SQLite through `gorm.io/gorm` + raw SQL, gin, `gorilla/websocket` (as `api/internal/api/v0/ws` already uses), testify, the black-box suite in `api/tests` (`//go:build integration`) over `api/tests/kit`.

**Spec:** `docs/plans/2026-10-16-crowbar-architecture.md` (§3, §4, §5, §6, §11 row S1) · `docs/plans/2026-10-16-chat-surface-report.md` (§2 symptoms 2–4, §4 target, §4.3 ingress contract, §5 decisions) · `docs/plans/2026-10-16-thread-branches-and-home-spike.md` (§A8, Part D) · `docs/plans/2026-10-16-chat-surface-additions-design.md` (§3.1 canonical fields, §3.2 rows). Executors read the spec sections a task cites.

## Plan files

| File | Tasks |
|---|---|
| `docs/plans/2026-10-17-s1a-ledger-foundation-plan.md` | T1 behaviour harness · T2 ledger store + prompts as rows |
| `docs/plans/2026-10-17-s1b-conversation-and-runner-plan.md` | T3 chat-owned conversation (symptom 2) · T4 runner as supervisor |
| `docs/plans/2026-10-17-s1c-turns-and-rows-plan.md` | T5 turn lifecycle (symptom 4) · T6 one hook ingress · T7 the transcript is the ledger (symptom 3) · T8 tool and MCP detail · T9 subagents and plans as entities |
| `docs/plans/2026-10-17-s1d-feed-migration-gate-plan.md` | T10 chat feed · T11 retire the relay transport · T12 legacy → layout-1 conversion · T13 gate |

13 tasks, one commit each. T7 is the one large cutover: text and tool rows cannot move separately, because the old wire attaches each tool call to the assistant message that follows it.

---

## Global Constraints

Every task's requirements include these. Quoted from the specs, `CLAUDE.md` and the user's standing rules.

- "Keep the hook channel and YAML descriptors. No reading provider transcripts, no SDK swap."
- "No provider vocabulary (claude/codex names, event names, flags) in Go code: it lives in descriptors." Tests and descriptors may name providers; non-test Go may not.
- "asynx STAYS … Until the user says it landed: do NOT depend on those. The chat ledger is its OWN append-only table with a per-chat single writer — not an asynx aggregate."
- "no feature flags, no parallel old/new code paths, no compatibility shims between stages. Each stage deletes what it replaces in the same change."
- "Same features as today, fewer lines." Every task deletes what it replaces in the same commit.
- "Ingress never discards." "never acknowledge a hook before its row is written, never drop one without a dead-letter row, and never let silence or a clock substitute for a fact." (report §3)
- "the reply waits only for the append to the ledger; projection and publication follow on the chat's single writer. Only asks and steers hold the reply open" (architecture §4).
- "Ids are minted at command time. A turn has an id before the provider names one" (architecture §3).
- "Append-only means never rewritten … every close names its turn" (architecture §3). "A late event for a closed turn is recorded as a late row. It never reopens the turn." (report §4.2)
- "Resume is always by the stored session id … never by 'last' or 'continue'." "Only the CLI's explicit refusal may move the chat to `lost`." "'delivered' only after the provider has acknowledged the turn." (architecture §5, spike §A8)
- "Server-canonical time and order. Timestamps are stamped on write; order is `seq`."
- "A fix starts with a failing test that reproduces it. Never fix a symptom with a timer, poll, retry, extra flag, or resync effect." Every backend bug gets a black-box `TestRegression_*`.
- "Delete code you make unused in the same change … deadcode/knip must stay clean." `make deadcode` analyses from `./cmd/...` only: a function reachable only from tests fails the gate, so every task wires what it adds.
- "Baselines (`api/deadcode-baseline.txt`, …, golangci-lint's `new-from-rev`) only shrink." "Never loosen a lint config, coverage floor, or budget to get green." The Go coverage floor is 92% and counts unit tests only (`make test-coverage`); integration-tagged tests do not count, so every new package ships unit tests through its public API.
- Go: "funlen 100 lines/50 statements, gocyclo 15, nestif 2, … early returns … Wrap errors with context: `fmt.Errorf("op: ctx: %w", err)`; check every error." "Required dependencies are constructor arguments, not nil-checked setters." "No `...ForTest` methods or test-only package vars in non-test files." "source files under 500 lines" — a task that edits a file over 500 lines brings it under 500. "One test file per source file (`foo.go` → `foo_test.go`)."
- Tests: "Test behaviour through the public API; never add a test only for coverage." "Extend the unit's existing test file." "No sleeps: channels, `require.Eventually`, `waitFor`, fake timers." CI rejects new `*_coverage_test.go`, `*_extra_test.go`, `*_gaps_test.go`.
- Comments: "explain *why* in ≤3 lines — no incident narratives or PR history."
- Web: "Do not touch web/ beyond what is strictly needed to keep it working." No task in S1 needs a web change; a task that finds it does stops and reports.
- Safety: never connect to `~/.crowbar/crowbar.sock` or open anything under `~/.crowbar/state`; never read or write `~/.claude` or `~/.codex`; real provider CLIs only in scratch dirs with `env -i`. Check `df -h /System/Volumes/Data` before any build; under 8 GB free, stop and report. Never bare `git stash`. Never push.
- Running tests: targeted only. Unit: `cd api && go test -tags noEmbed ./internal/<pkg>/...`. Black-box: `cd api && go test -tags 'integration noEmbed' -count=1 -run '<regex>' ./tests/`. Before each commit also run `make lint`, `make deadcode`, and `go vet -tags noEmbed ./...`. T13 runs the full Go gates once.
- Lock order: a chat's ledger writer may call into the snapshot owner and the hub (subscribers run under it); code holding the snapshot owner's lock never calls the ledger store. The snapshot learns ledger-derived facts (busy, conversation, last exit) only from publication.

## Rulings (open decisions resolved for S1)

Each follows the spec's own recommendation where it gives one; S1 keeps today's prompt delivery mechanism.

| # | Decision | Ruling for S1 |
|---|---|---|
| R1 | Prompt delivery: restart per message vs live channel (report §5.1) | Keep `restart_tui` (and the codex api push) exactly as today. Delivery is a row either way, so the later spike can switch it without touching the ledger. |
| R2 | A provider that goes silent, e.g. Claude's Escape (report §5.2) | No timer decides anything. The 120 s stall, 30 s message-quiet, 30 s delivery-quiet, 5 s idle latch and 2 h staleness nets are deleted. Crowbar writes `turn_ended{interrupted}` itself when it sends the interrupt. A provider's own idle hook is a fact: it closes a running turn at once (`turn_ended{done, reason: provider_idle}`). A turn the provider silently abandons stays running until the user's Stop, the next idle hook, or the runner's exit. |
| R3 | A prompt the provider never acknowledges | It stays `pending`, and the next prompt is refused with `ErrPromptBusy` meanwhile. It settles only on a fact: the provider's acknowledgement makes it delivered; `compact_pre`/`compact_post` settle it as consumed; a provider idle hook with no running turn, the user's Stop, the runner's exit, a daemon restart, or a refused resume settle it not-consumed with that reason. No automatic re-drive in S1. |
| R4 | "delivered" vs "row written" (report §4.2 and spike §A8 use the word for different things) | `user_message` = pending (written before the CLI is touched). `turn_started` = delivered = running: written only when the provider acknowledges (UserPromptSubmit / user message item). |
| R5 | Lost session (architecture §6 vs spike §A3's visible transcript fallback) | The transcript rung is deleted. A refused resume writes `session_lost` and `runner_exited{resume_failed}`; the next prompt the user sends starts a fresh session (`session_bound{cause: fresh}`) with no transcript handed over. S2 adds the explicit rebind/start-fresh affordance. |
| R6 | A live runner announces a session id the chat did not bind (`/clear` in the TUI) | It becomes the chat's binding for that provider (`session_bound{cause: announced, previous}`) and a visible `marker{session_changed}`. No runner ever moves between chats and no chat is minted from a hook. An id already bound to a different chat is refused: `marker{session_conflict}`, binding unchanged. This deletes "/clear mints a chat" and "/resume moves the runner to the chat that owns the id". |
| R7 | Streamed text: durable rows (architecture §3, report §8.3) vs ephemeral side channel (architecture §4) | Provider-numbered chunks (the descriptor maps `index`) are durable `assistant_text` rows keyed by (message id, index). Unnumbered token deltas, reasoning deltas, tool-output and diff deltas are an ephemeral side channel, superseded by a complete row. `seq` advances only on rows. |
| R8 | Where rows are pushed in S1 | A per-chat WebSocket `GET /v0/chats/:chatId/feed?since=N`. S4 folds it into the one stream. The existing lifecycle feed (`…/chats/ws`) keeps today's frame kinds, now emitted from ledger publication. |
| R9 | The relay hook transport (`/chats/hooks`, `…/await`, `…/abandon`, `crowbar hook`) | Retired in its own task (T11): no shipped descriptor uses it, and its delivery-id dedup is inert (a fresh uuid per request). HTTP hooks (`{hook_url}`, `{hook_curl}`) are the single ingress. T11 can be rejected independently without affecting any other task. |
| R10 | Switch or Stop while a turn runs | Switching provider while a turn is running is refused with `ErrPromptBusy` (409) — no 120 s force-kill. Stop writes `turn_ended{interrupted, reason: stopped}` first, then signals the provider (api interrupt call, or graceful terminate for a hooks-channel CLI), and never waits for confirmation. |
| R11 | Busy | `Busy = an open main-scope turn exists ∨ (T5–T8) the last turn_ended carried async_work > 0 and a runner is live ∨ (T9 on) a subagent is starting or running`. T9 deletes the `async_work` clause. |
| R12 | The legacy → layout-1 conversion "the migration engine wires later" vs the deadcode gate | The conversion lives at `api/internal/migrate/layout1/` and is reachable from a read-only `crowbar chat-ledger plan --home <dir>` dry run that prints per-chat row counts. It deliberately does not use the `migrate` command name, which `docs/plans/2026-10-17-migration-engine-plan.md` owns; when the engine adds its chat-ledger step, that step calls the conversion and the `chat-ledger` subcommand is deleted. |
| R13 | Old read shapes (`/messages`, `/activity`, `/choices`, `/pending-prompt`, chat DTO `working`/`session`) | Projections of the ledger held in an in-memory LRU of 64 chats fed by publication; rebuilt from rows on a miss. S2 deletes them. |
| R14 | Dead letters | Unknown runner: 404 and a row in the `ledger_dead_letters` table (payload kept, truncated at 65536 bytes). A bad runner token stays 401 with no record: an unauthenticated body is not a fact. Known chat, unmappable payload: 422, a `marker{dead_letter}` row in the chat, and a dead-letter table entry. A foreign-conversation hook (a provider-internal session): 200 and a dead-letter table entry with reason `foreign_conversation`, no chat row. Descriptor-declared surface gating stays a declared filter, not a discard. |
| R15 | Codex child frames that arrive before their subagent is bound (additions design §C) | Held in a bounded per-chat buffer (256 frames), drained on bind; whatever is left when the parent turn closes goes to the dead-letter table with reason `unclaimed_scope`. |
| R16 | Thread read scope (report §5.3) | Out of S1. No change. |
| R17 | The 500-line cap vs the oversize mock-based suites (`usecases/chat/runner_test.go` 5,482 lines, `turn_test.go`, `chat_test.go`, `harness_test.go`, `termwait_test.go`, `shared/tools/toolset_test.go`) | A task may delete cases from an oversize test file without splitting it; it never adds lines to one. New cases go in the unit's file when that file is under 500 lines, else in a new file named for the source file it tests (`foo.go` → `foo_test.go`). Production files a task edits are brought under 500 lines in that task. |

## Coordination with the migration-engine plan

`docs/plans/2026-10-17-migration-engine-plan.md` (written separately) also edits `api/internal/app/container.go`, `api/internal/adapter/container.go`, `api/internal/app/upgrade.go` (renamed `legacy_steps.go` there), `api/tests/harness_test.go`, `api/tests/kit/env.go` and `api/ARCHITECTURE.md`, and introduces `core/home` path accessors. Whichever plan executes second rebases onto the first: S1 opens `chat_ledger.db` through the engine's `home` accessors if they exist, and T4 retires the `chat-providers-from-history` legacy step in whichever file holds it. Neither plan changes the other's interfaces.

## Shared vocabulary — `api/internal/domain/ledger`

Defined here once; each task adds only the kinds and fields it writes (column "Added in").

```go
type Kind string

type Row struct {
	ChatID string          `json:"chatId"`
	Seq    int64           `json:"seq"`    // assigned by the store, 1-based, contiguous per chat
	Kind   Kind            `json:"kind"`
	TurnID string          `json:"turnId,omitempty"` // Crowbar-minted (uuid v4); "" for chat-level rows
	Scope  string          `json:"scope,omitempty"`  // "" = main agent, else the subagent's scope id
	At     time.Time       `json:"at"`              // stamped by the store's clock
	Body   json.RawMessage `json:"body"`
}

func NewRow[B any](kind Kind, turnID, scope string, body B) (Row, error)
func DecodeBody[B any](r Row) (B, error)
```

| Kind | Body (JSON field names) | Added in |
|---|---|---|
| `user_message` | `requestId`, `text`, `origin` (`crowbar`\|`provider`), `delivery` (`restart`\|`steer`\|`api`, crowbar origin only) | T2 |
| `turn_started` | `requestId` (empty for provider origin), `providerTurnId`, `runnerId` | T2 |
| `prompt_settled` | `requestId`, `consumed` (bool), `reason` | T2 |
| `turn_ended` | `status` (`done`\|`interrupted`), `reason`, `asyncWork` (removed in T9), `effort` | T5 |
| `turn_failed` | `reason`, `detail` | T5 |
| `session_bound` | `provider`, `sessionId`, `cause` (`assigned`\|`announced`\|`fresh`), `previous` | T3 |
| `session_lost` | `provider`, `sessionId`, `reason` | T3 |
| `runner_started` | `runnerId`, `provider`, `terminalSession`, `launchModel`, `launchEffort`, `launchPermissionLevel`, `resumeSessionId` | T4 |
| `runner_exited` | `runnerId`, `reason` (a `domain.AgentExit*` value), `exitCode` | T4 |
| `assistant_text` | `messageId`, `index` (chunk index; `-1` = the provider's complete text), `final` (the provider marked this chunk its message's last), `text`, `phase` (`""`\|`commentary`) | T7 |
| `reasoning` | `messageId`, `text` (the reasoning streamed during a turn, persisted once when the turn ends) | T7 |
| `marker` | `kind`, `detail` | T3 (session markers), T6 (`dead_letter`), T7 (the rest) |
| `tool_call` | `toolId`, `phase` (`pre`\|`post`\|`fail`), `name`, `kind`, `target`, `locations`, `inputRef`, `resultRef`, `status`, `error`, `durationMs`, `diff`, `spawnsScope`; T8 adds `title`, `mcpServer`, `mcpTool`, `exitCode`, `interrupted`; T9 adds `addressesScope` | T7 |
| `ask` | `askId`, `kind`, `promptId`, `toolId`, `toolName`, `title`, `question`, `mode`, `multi`, `options`, `questions`, `schema`; T8 adds `toolTarget`, `toolInputRef`, `mcpServer` | T7 |
| `ask_resolved` | `askId`, `resolution` (`answered`\|`proceeded`\|`abandoned`), `optionIds` | T7 |
| `subagent` | `phase` (`start`\|`stop`; T9 adds `bound`\|`result`), `spawnToolId`, `type`; T9 adds `title`, `status`, `result`, `usage` {`tokens`,`toolUses`,`durationMs`} | T7 |
| `plan` | `op` (`replace`\|`upsert`), `items` [{`id`,`text`,`detail`,`status`}] | T7 (`replace`), T9 (`upsert`) |

Marker kinds (closed set, constants in `marker.go`): `session_changed`, `session_conflict` (T3); `dead_letter` (T6); `compaction_started`, `compaction_ended`, `stopped`, `provider_switched`, `model_changed`, `effort_changed`, `notification` (T7). `inferred` is gone with R2.

The fold: `type State struct{…}`, `func Fold(chatID string, rows []Row) State`, `func (s *State) Apply(r Row)`, `func Check(s State, r Row) error`. `Apply` never fails: a transition the diagrams in architecture §5 do not allow (a `turn_ended` for a turn that is not running, a `tool_call post` for an unknown tool) leaves the state unchanged — the row is a late row. `Check` refuses only malformed rows (unknown kind; a `user_message`, `turn_started`, `turn_ended`, `turn_failed`, `assistant_text` or `reasoning` row without `TurnID`; an undecodable body); the store calls it before commit. Rows in a subagent scope carry the turn that spawned the subagent. `State` holds lifecycle facts and open work only (conversations, live runner, last exit, the running turn, pending prompts, request receipts, open tools, pending asks, live subagents, plans); closed transcript content is read from rows.

## Ownership after S1

| Fact | Owner after S1 | Owner today (deleted) |
|---|---|---|
| Pending/queued prompt | `user_message` + `prompt_settled` rows | `agentjournal` fsync'd files, 6-state machine |
| Session id per provider | `session_bound`/`session_lost` rows; `chat_ledger_sessions` index in the same tx | runner `CurrentSession*`, `LaunchSessionID`, `agent_chat_conversations`, per-turn tags, quarantine |
| Live runner | in-memory supervisor map keyed by chat; `runner_started`/`runner_exited` rows | `engine/agents/runner` aggregate + live read model + placements + heal |
| Turn and busy | `turn_started`/`turn_ended`/`turn_failed` rows; `State.Busy()` | chat aggregate `Working`/`CurrentTurnStarted`/`AsyncWork`, `inflight.Turns`/`Work`, activity turn pointer, stall closer |
| Transcript (text, tools, subagents, asks, plan, markers) | rows; old DTOs projected from them (R13) | `agent_activity` aggregate + read model |
| Hook ordering | the chat's ingest lock (T6), inside which rows commit under the chat's ledger writer | per-runner hook gate, startup barrier, delivery dedup, 3 s step-out |

## Deletion table (S1 "Deletes" column → task → files)

| Deleted | Task | Files (src and their tests) |
|---|---|---|
| Session inference | T3 | `api/internal/app/usecases/chat/internal/runner/{ladder.go,session.go,sessionorigin.go}`; `resumeTarget`/`resumableConversation` in `runner/{promptdelivery.go,switch.go}`; `api/internal/engine/agents/internal/move/`; `api/internal/engine/agents/internal/models/move.go`; `Decide` in `api/internal/engine/agents/agents.go`; runner commands `bind_session.go`, `move.go` and their store paths; the timestamp/runner-field half of `turn/conversation.go`'s identity filter (it becomes an equality with the ledger binding) |
| Quarantine | T3 | `sessionBook` probe/quarantine in `runner/supervisor.go` (`probe`, `confirm`, `failedProbe`, `isQuarantined`, `ConfirmLaunch`) and `resumeProbeWindow` |
| Transcript fallback | T3 | `transcriptFor`, `spawnRung`, `refusedDelivery`, `redeliverRefused`, `watchResume` (`runner/ladder.go`); `domain.AgentRungTranscript`; codex api resume-refusal replacement in `runner/apiconn.go`; `api/internal/engine/agents/{session_locate.go}`, `api/internal/engine/agents/internal/sessionstore/`, `session.locate` blocks in both shipped descriptors and `SessionLocateSpec` |
| `agentjournal` | T2 | `api/internal/adapter/store/agentjournal/` (whole package); `runner/{promptrecovery.go,pendingprompt.go,settle.go}`; journal paths in `runner/{prompts.go,promptswitch.go,switchwait.go,errors.go}`; `termwait` delivery-quiet (`Deliveries`, `DefaultDeliveryQuiet`) |
| Runner aggregate | T4 | `api/internal/engine/agents/runner/` (whole package); runner wiring in `api/internal/app/repositories/container.go`, `api/internal/app/container.go`, `api/internal/adapter/container.go` (`agent_runner` event/snapshot/read DBs); `runner/lifecycle.go` eviction (`retireOthersOn`, `evictHolderOf`, `retireOlderThan`, `requirePlacement`); `api/internal/app/usecases/chat/internal/conversation/provider_backfill.go`; `api/internal/app/usecases/chat/internal/fanout/` runner half; `models.ChatPlacement` |
| Activity/runner/conversation split | T3 (conversations), T4 (runner), T7 (activity) | `agent_chat_conversations` projection + heal markers (T3); runner store (T4); `api/internal/app/repositories/chat/activity/` whole package + `agent_activity` DBs in `api/internal/adapter/container.go` + `axAgentActivity` in `api/internal/app/container.go` + `ChoiceWatch` (T7) |
| Stall-as-owner | T5 | `api/internal/app/usecases/chat/internal/turn/{stall.go,idle.go,subagent_staleness.go,tool_staleness.go}`; `termwait` stall, message-quiet and idle halves (`DefaultStallQuiet`, `DefaultMessageQuiet`, `DefaultIdleQuiet`, `Stalled`, `Messages`, `Idle`, `Work`); `runner/switchwait.go` force-kill; `stop.go` interrupt bound; `api/internal/app/usecases/chat/internal/shared/inflight/` (whole package: `turnstate`, `gate` use on the hook path, `pending`) across T5 and T6; chat aggregate `start_turn.go`, `stop_turn.go`, `working.go` and the `Working`/`CurrentTurnStarted`/`AsyncWork`/`LastActivityAt`/`LedgerCursor` fields |
| Hook-side drops and gates (supporting "hook ingress writes rows") | T6, T7, T9 | `turn/delivery.go`, `turn/internal/dedup/`, startup barrier (`inflight/internal/pending`) (T6); the whole `api/internal/app/usecases/chat/internal/turn/` package incl. `internal/stream/`, `message.go`, `livetext.go`, `observation.go`, `compaction.go`, `routeSubagentTool` (T7); the nested-session router (`nested.go`, moved into `ingress` by T7) rewritten as scope rows with the R15 buffer (T9) |
| Relay transport (R9) | T11 | `api/cmd/crowbar/{hook.go,hook_delivery.go,hook_answer.go}` + tests; handlers `Hooks`, `AwaitHookAnswer`, `AbandonHookAnswer`; routes `/chats/hooks*`; `{crowbar_hook} hook` wiring in `descriptorcheck` |

## Review Focus

The five inputs the spec implies but no feature test exercises, most likely to bite first. Each has a test added to the owning task.

1. **Concurrent hooks for one chat.** Claude fires a parallel tool batch's PreToolUse hooks 20–100 ms apart on separate HTTP requests. Expect contiguous `seq`, no lost row, publication in `seq` order, and every reply only after its own row committed. Tests: T2 `TestStore_ConcurrentAppendsGetContiguousSeqsPublishedInOrder`; T6 `TestRegression_ConcurrentHooksForOneChatAllLand`.
2. **Daemon crash mid-turn with a pending prompt.** Expect the boot pass to write `runner_exited{daemon_restart}`, close the running turn as interrupted and settle the pending prompt, and the chat to accept the next prompt. Test: T5 `TestRegression_Symptom4_CrashMidTurnLeavesTheChatUsable`.
3. **A first hook that precedes the session announcement** (a resumed CLI's UserPromptSubmit or MessageDisplay before SessionStart). Expect the binding untouched and the turn to open normally. Test: T3 `TestRegression_Symptom2_HooksBeforeTheAnnouncementNeverUnbindTheSession`.
4. **A chat with thousands of rows.** Expect boot to touch only chats with open work and a read to replay the chat once, then serve from the projection. Tests: T2 `TestStore_OpenChatsListsOnlyChatsWithOpenWork` and `TestStore_StateSurvivesReopenWithManyRows` (5000 rows).
5. **Hooks after the chat was deleted**, from a CLI still dying. Expect 404 plus a dead-letter entry, and no resurrected ledger. Test: T6 `TestRegression_HooksAfterChatDeleteAreRefusedNotResurrected`.

## Gate baseline (for T13)

Chat subtree, measured at `9641256b5` with plain `wc -l` over `.go` files (tests = `*_test.go`):

- Source and unit tests under: `api/internal/app/usecases/chat`, `api/internal/app/repositories/chat`, `api/internal/engine/agents/runner`, `api/internal/engine/agents/internal/move`, `api/internal/engine/agents/internal/sessionstore`, `api/internal/engine/agents/session_locate.go`, `api/internal/adapter/store/agentjournal`, `api/internal/adapter/store/ledger`, `api/internal/domain/ledger`, `api/internal/migrate`, `api/internal/api/v0/endpoints/chat`, `api/internal/domain/{chat.go,chat_activity.go,agent_session.go,agent_pending_prompt.go,chat_log.go}`, `api/cmd/crowbar/{hook.go,hook_delivery.go,hook_answer.go}`.
- Plus black-box tests: `api/tests/**/*_test.go` whose name contains `agent` or `chat`, and everything under `api/tests/integration/agent/`.

Baseline: **source 41,356 · tests 72,596**. Gate: **source ≤ 31,356 · tests ≤ 52,596**, and the `TestRegression_Symptom2_*`, `TestRegression_Symptom3_*`, `TestRegression_Symptom4_*` tests green. T13 carries the measuring command.

## Spec notes found while planning

These are recorded so a reviewer does not re-discover them; the rulings above settle each for S1.

1. Architecture §4 makes streamed deltas ephemeral and `seq` row-only; architecture §3 and report §8.3 keep chunks durable keyed by (message id, index). Settled by R7.
2. Report §4.2 uses "delivered" for "the `user_message` row exists"; spike §A8 and architecture §9 use it for "the provider acknowledged". Settled by R4.
3. Spike §A3 keeps a visible transcript fallback; architecture §6 and the S1 row delete it. Settled by R5.
4. `api/ARCHITECTURE.md` says hooks arrive through `crowbar hook`; both shipped descriptors post HTTP hooks (`claude.yaml` native HTTP, `codex.yaml` curl), and `HTTPHook` mints a fresh delivery id per request, so the delivery-id dedup never dedups. Settled by R9.
5. The architecture's S1 gate does not define "chat subtree"; the list above does.
6. The architecture's "behaviour tests over HTTP" are integration-tagged and excluded from the 92% unit-coverage floor, and `make deadcode` forbids a function only tests reach. The plan keeps unit tests on every new package and wires everything it adds (R12).
7. The additions design's class diagram names the failure row `turn_error`; the architecture names it `turn_failed`. The plan uses `turn_failed`.
8. `termwait`'s terminal-prompt and terminal-notice detection is a 2 s screen poll; S1's delete list does not include it, so S1 keeps it (it decides no turn or session state except a notice declared `ends_turn`, which writes `turn_failed{reason: <notice kind>}`). Replacing the poll with a screen-change signal belongs with the runner state machine's `prompting` state in a later stage.
