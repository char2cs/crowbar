# S1 — Chat ledger and chat-owned conversation: Implementation Plan (index)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks are strictly sequential: each one assumes every earlier task is committed.

**Goal:** Every fact about a chat — prompt, turn, session binding, runner start/exit, text, tool call, subagent, ask, marker — becomes a row in one append-only per-chat ledger with a server-assigned `seq`, and the daemon's HTTP/WS chat contract that today's web client reads is served from that ledger.

**Architecture:** A pure `domain/ledger` package holds the row vocabulary and the state machines (turn, conversation, prompt) as a fold. An `adapter/store/ledger` SQLite store is the only writer: one in-process lock per chat, `seq` assigned under it, projections (heads, session index) committed in the same transaction, subscribers told after commit in `seq` order. Hook ingress maps each provider event to rows and replies only after the append; the runner becomes an in-memory supervisor whose durable facts are rows; a per-chat WebSocket feed pushes rows. The old read endpoints are projections of the ledger until S2 replaces the client. The legacy records become ledger rows through migration step 6 `chat-ledger`, run by the migration engine at boot.

**Tech Stack:** Go (`api/`, always `-tags noEmbed`), SQLite through `gorm.io/gorm` + raw SQL, gin, `gorilla/websocket` (as `api/internal/api/v0/ws` already uses), testify, the black-box suite in `api/tests` (`//go:build integration`) over `api/tests/kit`, the migration engine (`api/internal/core/migrate`, catalog in `api/internal/adapter/migrations`).

**Spec:** `docs/plans/2026-10-16-crowbar-architecture.md` (§3, §4, §5, §6, §11 row S1) · `docs/plans/2026-10-16-chat-surface-report.md` (§2 symptoms 2–4, §4 target, §4.3 ingress contract, §5 decisions) · `docs/plans/2026-10-16-thread-branches-and-home-spike.md` (§A8, Part D) · `docs/plans/2026-10-16-chat-surface-additions-design.md` (§3.1 canonical fields, §3.2 rows). Executors read the spec sections a task cites.

## Refresh 2026-10-18

Rewritten after the migration engine landed (HEAD `ec50d1548`) so each task's text stands alone. Changes:
- Folded the S1 pre-flight rulings (R1–R25, R28, R30–R34, R8 amended, R7/R29 provisional) and carries C1–C4 into the tasks they name. Each task now carries its own commands, oversize-file rule and safety lines.
- The engine API is used as built: `migrations.Begin`, `Session.RunPreOpen`, positional `app.New(ctx, engines, adapters, post)`, `migrate.Step`/`Impl`/`Checkpoints`, `home.Home` accessors, and `kit.BootHome` in tests. The plan text named a `Boot` and a `WithPostOpen` that were never built.
- T12 is now engine step 6 `chat-ledger` (pre-open, 0→0) in `adapter/migrations/chatledger`, with `ledgerstore.Store.Import`. The `crowbar chat-ledger` CLI is gone.
- T4 keeps step 4 `chat-providers-from-history`, over a read-only legacy reader. The T2 boot pass moves to `ReconcileRunnersOnBoot`.
- From the S2 pre-flight: T10 writes the final `FeedDelta` plus the post-ready snapshot, T7 writes `ask.answerable`, and T5 has no `retracted` status.
- Citations fixed: `app/upgrade.go` → `app/legacy_steps.go` + `adapter/migrations/catalog.go`; the boot-pass sites; the T13 web-diff base; the gate baseline, re-measured at S1 start.
- Banned commands replaced: `make test-integration`, `make pr-checks` and `make test-coverage` all run `go test ./...` or read provider homes.

## Plan files

| File | Tasks |
|---|---|
| `docs/plans/2026-10-17-s1a-ledger-foundation-plan.md` | T1 behaviour harness · T2 ledger store + prompts as rows |
| `docs/plans/2026-10-17-s1b-conversation-and-runner-plan.md` | T3 chat-owned conversation (symptom 2) · T4 runner as supervisor |
| `docs/plans/2026-10-17-s1c-turns-and-rows-plan.md` | T5 turn lifecycle (symptom 4) · T6 one hook ingress · T7 the transcript is the ledger (symptom 3) · T8 tool and MCP detail |
| `docs/plans/2026-10-17-s1d-feed-migration-gate-plan.md` | T9 subagents and plans as entities · T10 chat feed · T11 retire the relay transport · T12 migration step 6 `chat-ledger` · T13 gate |

13 tasks, one commit each (T1 adds one chore commit before its own). T7 is the one large cutover: text and tool rows cannot move separately, because the old wire attaches each tool call to the assistant message that follows it.

---

## Global Constraints

Every task's requirements include these. Each task also restates the ones it most needs, in its own text.

- "Keep the hook channel and YAML descriptors. No reading provider transcripts, no SDK swap."
- "No provider vocabulary (claude/codex names, event names, flags) in Go code: it lives in descriptors." Tests, test support under `api/tests` and descriptors may name providers; non-test Go under `api/internal` and `api/cmd` may not.
- The chat ledger is its own append-only table with a per-chat single writer, not an asynx aggregate.
- "no feature flags, no parallel old/new code paths, no compatibility shims between stages. Each stage deletes what it replaces in the same change." Every task deletes what it replaces in the same commit.
- "Ingress never discards." "never acknowledge a hook before its row is written, never drop one without a dead-letter row, and never let silence or a clock substitute for a fact." (report §3)
- "the reply waits only for the append to the ledger; projection and publication follow on the chat's single writer. Only asks and steers hold the reply open" (architecture §4).
- "Ids are minted at command time. A turn has an id before the provider names one" (architecture §3). "Append-only means never rewritten … every close names its turn." "A late event for a closed turn is recorded as a late row. It never reopens the turn."
- "Resume is always by the stored session id … never by 'last' or 'continue'." "Only the CLI's explicit refusal may move the chat to `lost`." "'delivered' only after the provider has acknowledged the turn."
- "Server-canonical time and order. Timestamps are stamped on write; order is `seq`."
- A fix starts with a failing test. Never fix with a timer, poll, retry, flag or resync effect. Every backend bug gets a black-box `TestRegression_*`.
- Dead code: delete what you make unused in the same change. `make deadcode` analyses from `./cmd/...` only, so every task wires what it adds. Baselines (`api/deadcode-baseline.txt`, `scripts/ratchet/*-baseline.json`, golangci-lint `new-from-rev`) only shrink. Never loosen a lint config, coverage floor or budget.
- Coverage: each task runs `go test -cover` on the packages it touched, and a new package ships at ≥ 92% through its public API. The module-wide floor (`make test-coverage`, 92%) runs `go test ./...`, which the standing rules forbid locally, so CI runs it. A per-package drop is reported to the orchestrator.
- Go: funlen 100 lines/50 statements, gocyclo 15, nestif 2, early returns, wrapped errors (`fmt.Errorf("op: ctx: %w", err)`), required dependencies as constructor arguments, no `...ForTest` in non-test files, one test file per source file.
- **500-line rule (pre-flight R8, amended).**
  - A net-shrinking edit to a file over 500 lines needs no split.
  - A task that adds lines to an oversize file, or rewrites most of it, splits it below 500 in that task. Trimming narrative comments to ≤3-line whys counts toward the offset; squeezing blank lines does not.
  - A split wiring file's new path is added to `scripts/ratchet/modules.json` `wiring`.
  - No file may cross 500 lines because of an S1 task.
- Comments explain why in ≤3 lines.
- Web: S1 changes no file under `web/`. A task that finds it must stops and reports.
- Safety:
  - Never connect to `~/.crowbar/crowbar.sock` or open anything under `~/.crowbar`.
  - Never read or write `~/.claude`, `~/.claude.json` or `~/.codex`.
  - Real provider CLIs are not run in S1.
  - Check `df -h /System/Volumes/Data` before any build; under 8 GB free, stop and report.
  - Never bare `git stash`. Never push.
- Commands: every task uses the standing form it carries, a `gox`/`bbx` shell block. Never `go test ./...`, `make test-integration`, `make pr-checks`, `make test-coverage`, `-tags integration` on `./tests/integration/...` or `./tests/kit/...`, or the full vitest suite.
- Ratchet: each task runs `node scripts/ratchet/file-size.mjs` and `node scripts/ratchet/counters.mjs` read-only; both must exit 0. `loc-budgets.mjs` already fails on the engine's growth; T13 measures it and reports it (R7, provisional: no budget rises).
- Lock order: ingress per-chat lock (T6) → ledger writer → snapshot owner / transcript projector. Code holding the snapshot owner's or the projector's lock never calls the ledger store. The snapshot learns ledger-derived facts only from publication.

## Rulings (open decisions resolved for S1)

Each follows the spec's own recommendation where it gives one. The pre-flight rulings are folded into the tasks they name; this table holds the design rulings. Tasks cite the two sets as "pre-flight Rn" and "design Rn".

| # | Decision | Ruling for S1 |
|---|---|---|
| R1 | Prompt delivery: restart per message vs live channel (report §5.1) | Keep `restart_tui` (and the codex api push) exactly as today. Delivery is a row either way, so a later spike can switch it without touching the ledger. |
| R2 | A provider that goes silent, e.g. Claude's Escape (report §5.2) | No timer decides anything. The 120 s stall, 30 s message-quiet, 30 s delivery-quiet, 5 s idle latch and 2 h staleness nets are deleted. Crowbar writes `turn_ended{interrupted}` itself when it sends the interrupt. A provider's own idle hook is a fact: it closes a running turn at once (`turn_ended{done, reason: provider_idle}`). A turn the provider silently abandons stays running until the user's Stop, the next idle hook, or the runner's exit. |
| R3 | A prompt the provider never acknowledges | It stays `pending`; the next prompt is refused with `ErrPromptBusy` meanwhile. It settles only on a fact: acknowledgement → delivered; `compact_pre`/`compact_post` → consumed; idle hook with no running turn, user Stop, runner exit, daemon restart or refused resume → not consumed, with that reason. No automatic re-drive. |
| R4 | "delivered" vs "row written" | `user_message` = pending (written before the CLI is touched). `turn_started` = delivered = running: written only when the provider acknowledges. |
| R5 | Lost session | The transcript rung is deleted. A refused resume writes `session_lost` and `runner_exited{resume_failed}`; the next prompt starts a fresh session (`session_bound{cause: fresh}`) with no transcript handed over. S2 adds the explicit rebind/start-fresh affordance. |
| R6 | A live runner announces a session id the chat did not bind (`/clear` in the TUI) | It becomes the chat's binding (`session_bound{cause: announced, previous}`) plus `marker{session_changed}`. No runner moves between chats and no chat is minted from a hook. An id bound to a different chat is refused: `marker{session_conflict}`, binding unchanged. |
| R7 | Streamed text | Provider-numbered chunks are durable `assistant_text` rows keyed by (message id, index). Unnumbered deltas, reasoning, tool-output and diff deltas are an ephemeral side channel, superseded by a complete row. `seq` advances only on rows. |
| R8 | Where rows are pushed | A per-chat WebSocket `GET /v0/chats/:chatId/feed?since=N`. The lifecycle feed (`…/chats/ws`) keeps today's frame kinds, emitted from ledger publication; its `message_delta` stays cumulative. |
| R9 | The relay hook transport | Retired in T11: no shipped descriptor uses it. HTTP hooks (`{hook_url}`, `{hook_curl}`) are the single ingress. |
| R10 | Switch or Stop while a turn runs | A switch while a turn runs is refused with `ErrPromptBusy` (409), with no force-kill. Stop writes `turn_ended{interrupted, reason: stopped}` first, then signals the provider, and never waits. |
| R11 | Busy | `Busy = an open main-scope turn ∨ (T5–T8) the last turn_ended carried async_work > 0 and a runner is live ∨ (T9 on) a subagent is starting or running`. T9 deletes the `async_work` clause. |
| R12 | The legacy conversion vs the deadcode gate | Migration step 6 `chat-ledger`, pre-open, layout 0→0, registered in `api/internal/adapter/migrations/catalog.go`. Its code is in `api/internal/adapter/migrations/chatledger/`. It is reachable from the daemon's boot; the dry run is `crowbar migrate --plan`. It runs on dev homes until the release. |
| R13 | Old read shapes (`/messages`, `/activity`, `/choices`, `/pending-prompt`, chat DTO `working`/`session`) | Projections of the ledger: an in-memory LRU of 64 chats fed by publication, rebuilt from rows on a miss under the chat's writer. This is a stage adapter that S2 deletes. |
| R14 | Dead letters | Unknown runner → 404 plus a `ledger_dead_letters` entry (payload truncated at 65536 bytes). A bad token stays 401 with no record. Known chat, unmappable payload → 422 plus `marker{dead_letter}` plus an entry. A foreign-conversation hook → 200 plus an entry (`foreign_conversation`). Surface gating stays a declared filter. |
| R15 | Codex child frames before their subagent is bound | A bounded per-chat buffer (256 frames), drained on bind; at the parent turn's close, the rest are dead-lettered (`unclaimed_scope`). |
| R16 | Thread read scope | Out of S1. |
| R17 | The 500-line cap | Replaced by the amended 500-line rule (pre-flight R8) in Global Constraints. |

## Migration engine as built (what S1 tasks use)

- `migrate.Step{Number, Name, Phase (migrate.PreOpen|PostOpen), From, To, Plan func(ctx, migrate.Env) (migrate.Plan, error)}`.
- `migrate.Env{Home home.Home; Log *slog.Logger; Earlier Marks}`.
- `migrate.Impl{Apply func(ctx, migrate.Env, *migrate.Checkpoints) (migrate.Counts, error); Verify func(ctx, migrate.Env) error}`; `migrate.Impls map[int]Impl`.
- `(*Checkpoints).Done(key) bool`, `Mark(key, migrate.Counts) error`; `migrate.Counts map[string]int64`. The legacy steps count skipped rows as `skipped_rows`.
- `migrate.Plan{Actions []Action; RewritesState, NeedsGit bool}`, `Action{Kind (migrate.Convert|Delete), Subject, Count}`, `migrate.Unknown = -1`. A Plan opens no store and writes nothing.
- `api/internal/adapter/migrations`:
  - `Catalog()`, `Begin(ctx, h, opts)`, `PreOpenImpls()` (in `workspace_paths.go`), `Adopting(number, impl)`.
  - Constants `StepProjectHomes=1`, `StepWorkspacePathsTable=2` (pre-open), `StepChatTypes=3`, `StepChatProviders=4`, `StepHookDeliveries=5` (post-open, in `api/internal/app/legacy_steps.go` `legacyImpls(newLegacyFuncs(repos, ucs))`).
  - Pre-open steps run in number order, then post-open.
- Boot (`api/internal/internal.go`): `migrations.Begin` → `session.RunPreOpen(ctx, migrations.PreOpenImpls())` → `adapter.New(adapter.WithHomeDir(h.Root()))` → `app.New(ctx, engines, adapters, post migrate.PostOpenRunner)`.
  - `app.New` order: `startBootSweep`, `ProjectDelete.Resume`, terminal restore, `reconcileAgentRunners` (→ `ucs.AgentRunner.ReconcileRunnersOnBoot`), then `post.RunPostOpen(...)`, then the loops.
  - S1 boot passes live in `ReconcileRunnersOnBoot`, before `RunPostOpen`.
- `home.Home`: `Root`, `State`, `Events`, `Content`, `Store`, `Config`, `MigrateDir`, `Socket`, …; the adapter exposes `Home()`. `api/internal/core/home/architecture_test.go` bans hand-built home resolvers. Under `go test` a home must sit under `os.TempDir()`.
- Halt reasons: `layout_newer`, `layout_unreadable`, `ledger_corrupt`, `io_failed`, `plan_failed`, `preflight_failed`, `step_failed`, `verify_failed`, `no_implementation`. Health while migrating: "Crowbar is upgrading its data (step i of n: <name>)".
- Test support:
  - `kit.BootHome(ctx, dir)`.
  - `tests/harness_test.go`: `newHarness`, `newHarnessAt(t, home)`, `h.shutdown()`, `h.crash()` (releases the home lock), `h.Quiesce()`.
  - `internal/testutil/migratetest`: `NoPostOpen()`, `Applied(tb, h)`.
  - `testutil.FixtureHome`.

## Shared vocabulary — `api/internal/domain/ledger`

Defined here once; each task adds only the kinds and fields it writes ("Added in"). Each task restates the parts it needs.

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
| `user_message` | `requestId`, `text`, `origin` (`crowbar`\|`provider`\|`harness`), `delivery` (`restart`\|`steer`\|`api`, crowbar origin only), `runnerId` (the carrier: the runner that delivers a crowbar prompt; only it acknowledges or strands the prompt) | T2 |
| `turn_started` | `requestId` (crowbar origin only), `providerTurnId`, `runnerId` | T2 |
| `prompt_settled` | `requestId`, `consumed`, `reason` | T2 |
| `turn_ended` | `status` (`done`\|`interrupted`), `reason`, `asyncWork` (deleted in T9), `effort` | T5 |
| `turn_failed` | `reason`, `detail` | T5 |
| `session_bound` | `provider`, `sessionId`, `cause` (`assigned`\|`announced`\|`fresh`), `previous` | T3 |
| `session_lost` | `provider`, `sessionId`, `reason` | T3 |
| `runner_started` | `runnerId`, `provider`, `terminalSession`, `launchModel`, `launchEffort`, `launchPermissionLevel`, `resumeSessionId` | T4 |
| `runner_exited` | `runnerId`, `reason` (a `domain.AgentExit*` value), `exitCode` | T4 |
| `assistant_text` | `messageId`, `index` (`-1` = the provider's complete text), `final`, `text`, `phase` | T7 |
| `reasoning` | `messageId`, `text` | T7 |
| `marker` | `kind`, `detail` | T3, T6, T7 |
| `tool_call` | `toolId`, `phase` (`pre`\|`post`\|`fail`), `name`, `kind`, `target`, `locations`, `inputRef`, `resultRef`, `status`, `error`, `durationMs`, `diff`, `spawnsScope`; T8 adds `title`, `mcpServer`, `mcpTool`, `exitCode`, `interrupted`; T9 adds `addressesScope` | T7 |
| `ask` | `askId`, `kind`, `promptId`, `toolId`, `toolName`, `title`, `question`, `mode`, `multi`, `options`, `questions`, `schema`, `answerable`; T8 adds `toolTarget`, `toolInputRef`, `mcpServer` | T7 |
| `ask_resolved` | `askId`, `resolution` (`answered`\|`proceeded`\|`abandoned`), `optionIds` | T7 |
| `subagent` | `phase` (`start`\|`stop`; T9 adds `bound`\|`result`), `spawnToolId`, `type`; T9 adds `title`, `status`, `result`, `usage` {`tokens`,`toolUses`,`durationMs`} | T7 |
| `plan` | `op` (`replace`\|`upsert`), `items` [{`id`,`text`,`detail`,`status`}] | T7, T9 (`upsert`) |

Marker kinds (closed set, constants in `marker.go`):
- `session_changed`, `session_conflict` (T3; T12 also writes `session_conflict`);
- `dead_letter` (T6);
- `compaction_started`, `compaction_ended`, `stopped`, `provider_switched`, `model_changed`, `effort_changed`, `notification` (T7).

The fold:
- `type State struct{…}`, `func Fold(chatID string, rows []Row) State`, `func (s *State) Apply(r Row)`, `func Check(s State, r Row) error`.
- `Apply` never fails: a transition the architecture §5 diagrams do not allow leaves the state unchanged (a late row).
- `Check` refuses only malformed rows: an unknown kind; a turn-scoped kind without `TurnID`; an undecodable body.
- Rows in a subagent scope carry the turn that spawned the subagent.
- `State` holds lifecycle facts, open work and the natural keys ingress dedupes on (T7). Closed transcript content is read from rows.
- `TurnStatus` is `pending`, `running`, `done`, `failed`, `interrupted`. There is no `retracted`.

## Ownership after S1

| Fact | Owner after S1 | Owner today (deleted) |
|---|---|---|
| Pending/queued prompt | `user_message` + `prompt_settled` rows | `agentjournal` fsync'd files, 6-state machine |
| Session id per provider | `session_bound`/`session_lost` rows; `chat_ledger_sessions` index in the same tx | runner `CurrentSession*`, `LaunchSessionID`, `agent_chat_conversations`, per-turn tags, quarantine |
| Live runner | in-memory supervisor map keyed by chat; `runner_started`/`runner_exited` rows | `engine/agents/runner` aggregate + live read model + placements + heal |
| Turn and busy | `turn_started`/`turn_ended`/`turn_failed` rows; `State.Busy()` | chat aggregate `Working`/`CurrentTurnStarted`/`AsyncWork`, `inflight.Turns`/`Work`, activity turn pointer, stall closer |
| Transcript | rows; old DTOs projected from them (R13) | `agent_activity` aggregate + read model |
| Hook ordering | ingress's per-chat lock (T6), also taken by user-initiated spawns | per-runner hook gate, startup barrier, delivery dedup, 3 s step-out, `inflight` spawn gate |
| Legacy records | migration step 6 `chat-ledger` (T12) and step 4's legacy reader (T4) | nothing reads them after T4/T7 |

## Deletion table (S1 "Deletes" column → task → files)

| Deleted | Task | Files (src and their tests) |
|---|---|---|
| Session inference | T3 | `runner/{ladder.go,session.go,sessionorigin.go}`; `resumeTarget`/`resumableConversation`; `engine/agents/internal/move/`; `models/move.go`; `Decide`; runner commands `bind_session.go`, `move.go`; the timestamp half of `turn/conversation.go`'s identity filter |
| Quarantine | T3 | `sessionBook` probe/quarantine in `runner/supervisor.go`, `resumeProbeWindow` |
| Transcript fallback | T3 | `transcriptFor`, `spawnRung`, `refusedDelivery`, `redeliverRefused`, `watchResume`; `domain.AgentRungTranscript`; codex api resume-refusal replacement; `engine/agents/session_locate.go`, `internal/sessionstore/`, `session.locate` blocks, `SessionLocateSpec` |
| `agentjournal` | T2 | `api/internal/adapter/store/agentjournal/`; `runner/{promptrecovery.go,pendingprompt.go,settle.go}`; termwait delivery-quiet |
| Runner aggregate | T4 | `api/internal/engine/agents/runner/`; its wiring and `agent_runner` DBs; eviction (`retireOthersOn`, `evictHolderOf`, `retireOlderThan`, `requirePlacement`); `provider_backfill.go`'s runner-store and activity reads (step 4 keeps the rule over a legacy reader); `fanout/` runner half; `models.ChatPlacement` |
| Activity/runner/conversation split | T3, T4, T7 | `agent_chat_conversations` + heal markers (T3); runner store (T4); `repositories/chat/activity/`, `agent_activity` DBs, `axAgentActivity`, `ChoiceWatch` (T7) |
| Stall-as-owner | T5 | `turn/{stall.go,idle.go,subagent_staleness.go,tool_staleness.go}`; termwait stall/message/idle halves; `runner/switchwait.go`; `stop.go` interrupt bound; `inflight/internal/turnstate`; chat aggregate turn state |
| Hook-side drops and gates | T6, T7, T9 | `turn/delivery.go`, `turn/internal/dedup/`, the whole `inflight` package incl. the spawn gate (T6); the `turn` package (T7); nested-session routing rewritten as scope rows (T9) |
| Relay transport | T11 | `api/cmd/crowbar/{hook.go,hook_delivery.go,hook_answer.go}`; handlers `Hooks`, `AwaitHookAnswer`, `AbandonHookAnswer`; routes `/chats/hooks*`; `{crowbar_hook} hook` wiring |

## Review Focus

The inputs most likely to bite first; each has a test in the owning task.

1. **Concurrent hooks for one chat.** Parallel PreToolUse hooks 20–100 ms apart. Expect contiguous `seq`, no lost row, publication in order, each reply after its own commit. T2 `TestStore_ConcurrentAppendsGetContiguousSeqsPublishedInOrder`; T6 `TestRegression_ConcurrentHooksForOneChatAllLand`.
2. **Daemon crash mid-turn with a pending prompt.** T5 `TestRegression_Symptom4_CrashMidTurnLeavesTheChatUsable`.
3. **A first hook that precedes the session announcement.** T3 `TestRegression_Symptom2_HooksBeforeTheAnnouncementNeverUnbindTheSession`.
4. **A chat with thousands of rows.** T2 `TestStore_OpenChatsListsOnlyChatsWithOpenWork`, `TestStore_StateSurvivesReopenWithManyRows`.
5. **Hooks after the chat was deleted.** T6 `TestRegression_HooksAfterChatDeleteAreRefusedNotResurrected`.
6. **A duplicated hook delivery.** T7 natural-key dedup tests.
7. **An upgrading home.** T12 converts a legacy home at boot, and a second boot is a no-op.

## Gate baseline (for T13)

The chat subtree is counted with plain `wc -l` over `.go` files (tests = `*_test.go`).
- Source and unit tests under:
  - `api/internal/app/usecases/chat`, `api/internal/app/repositories/chat`;
  - `api/internal/engine/agents/runner`, `api/internal/engine/agents/internal/move`, `api/internal/engine/agents/internal/sessionstore`, `api/internal/engine/agents/session_locate.go`;
  - `api/internal/adapter/store/agentjournal`, `api/internal/adapter/store/ledger`, `api/internal/domain/ledger`, `api/internal/adapter/migrations/chatledger`;
  - `api/internal/api/v0/endpoints/chat`;
  - `api/internal/domain/{chat.go,chat_activity.go,agent_session.go,agent_pending_prompt.go,chat_log.go}`;
  - `api/cmd/crowbar/{hook.go,hook_delivery.go,hook_answer.go}`.
- Plus black-box tests: `api/tests/**/*_test.go` whose name contains `agent` or `chat`, everything under `api/tests/integration/agent/`, and `api/tests/kit/fakeprovider.go`.

Baseline at S1 start (`ec50d1548`, the migration engine merged): **source 41,482 · tests 72,643**. Gate: **source ≤ 31,482 · tests ≤ 52,643**, plus the `TestRegression_Symptom2_*`, `Symptom3_*` and `Symptom4_*` tests green. The original count at `9641256b5` was 41,356 / 72,596; the engine added 126 / 47 inside these paths. T13 carries the measuring command.

## Spec notes found while planning

1. Architecture §4 makes streamed deltas ephemeral and `seq` row-only. Architecture §3 and report §8.3 keep chunks durable. Settled by R7.
2. "Delivered" means two things in the specs. Settled by R4.
3. Spike §A3 keeps a visible transcript fallback; architecture §6 deletes it. Settled by R5.
4. `api/ARCHITECTURE.md` says hooks arrive through `crowbar hook`; both shipped descriptors post HTTP hooks. Settled by R9.
5. The architecture's S1 gate does not define "chat subtree"; the list above does.
6. Black-box tests do not count toward the 92% floor, and `make deadcode` forbids a function that only tests reach. Every new package has unit tests, and the conversion is wired as an engine step (R12).
7. The additions design names the failure row `turn_error`; the plan uses the architecture's `turn_failed`.
8. `termwait`'s terminal-prompt and terminal-notice detection is a 2 s screen poll. S1 keeps it. It decides nothing except a notice declared `ends_turn`. Pre-flight R26 gives its replacement, a screen-change signal from the terminal model, to S6: amend the S6 plan before S6 runs.
