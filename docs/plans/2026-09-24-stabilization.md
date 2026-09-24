# Stabilization: one owner per state, then delete the workarounds

Crowbar works, but it does not *stay* working. Of the last ~30 merged commits,
28 are fixes, and their titles keep naming the same thing: desync, leak,
bleed-through, stuck spinner, ghost rows, chat theft, stale thread, "Stop not
halting", orphaned blobs, wedging. This spec is the plan to end that — not by
fixing more instances, but by removing the conditions that generate them, and
deleting the code that exists only to paper over them.

It is built from a six-area read-only audit (agent lifecycle, terminal/PTY,
layout stores, storage/worktrees, editor/git/explorer, tests/CI/hygiene) plus a
tooling baseline (`go vet`, `deadcode`, `go test -race`, `knip`, `tsc`).
Findings marked **✔** were re-verified by reading the code while writing this;
the rest are audit findings with file:line evidence, to be confirmed by a
failing test before they are fixed (see §2).

## 1. Diagnosis

The trash is not random. Almost all of it is one pattern:

> A piece of state has **no single owner**, so it is stored in several places.
> The copies drift. Each drift is fixed where it was noticed — with a timer, a
> poll, a flag maintained by hand at N sites, a resync effect, a read-side
> "staleness net", or a defensive guard. Each fix works. None removes the drift.

`docs/plans/2026-09-23-views-are-records.md` already says it best about
`dormantArrangements`: *"This is not a bug with a fix. It is a generator."*
The audit found the same generator in every area:

| Area | State with no owner | Copies |
| --- | --- | --- |
| Agent | chat "working" flag | **11** (7 server, 4 web) |
| Agent | active provider | 6 sources, resolved in 5 places (one in the DTO layer) |
| Agent | pending prompts | 5 layers, confirmed by *prompt text equality* |
| Terminal | per-session lifecycle | 8 maps, cleaned by 5 inconsistent paths; "placeholder" inferred from `ptmx == nil` |
| Terminal | tab → connection | 4 copies; PTY size has 5+ writers |
| Layout | chat → workspace | **8 resolvers**, no source of truth |
| Layout | active workspace | 3 copies set in different React effect phases |
| Layout | sidebar tree | 7 write paths into `repos` |
| Storage | workspace kind / placeholder | encoded 3 ways, one of them "empty path" (ambiguous) |
| Storage | delete | 3 logical deleters, 2 physical purgers (diverged) |
| Editor | editor state | Monaco *and* a leftover textarea-era editor model |
| Editor | settings | 3 owners, 2 persistence paths (one write-only) |

What vibecoding got **right**, and what this plan keeps: layered structure,
hand-written fakes over mock frameworks, a real HTTP+SQLite integration kit,
~1.7× test-to-code ratio in Go, no snapshot spam, `tsc` clean, **zero data
races** under `go test -race`. The foundations are fine; the ownership is not.

## 2. Method — the loop every area goes through

1. **Owner.** For each piece of state: list every copy, name the one source of
   truth, name who creates and destroys it. Everything else becomes a derived,
   read-only projection or is deleted.
2. **Invariants.** Write the rules the owner guarantees as tests *before*
   refactoring (listed per area below). A property/fuzz test where the
   invariant is about sequences of operations.
3. **Move.** Route all writes through the owner.
4. **Delete, one workaround at a time.** Remove the timer/poll/flag/guard, run
   the invariant tests, and try to reproduce the bug it hid. If something
   breaks, fix the cause — never restore the hack.
5. **Split** the giant files along the seams the ownership now gives.

Rules for every PR in this plan:

- A bug fix starts with a failing test that reproduces it.
- Deletion PRs are net-negative in lines and carry their tests.
- No PR both changes behaviour and moves large amounts of code.
- One area per PR; reviewers review the invariants and tests, not every line.

## 3. Phase 0 — stop the data loss (do first, before any refactor)

These can destroy user data or leave the user's own git repo damaged. Each is a
small, targeted fix with a regression test; none waits for the refactor.

| # | Bug | Where | Fix |
| --- | --- | --- | --- |
| P0-1 ✔ | **Deleting a repo force-deletes the user's local branches**, including the default/protected branch and locked worktrees with uncommitted work. The repo row is deleted first, so `removeOne` runs with `defaultBranch=""` and takes the `ForceDeleteBranch` path for every managed workspace. | `usecases/workspace/internal/hierarchy/worktree.go:1580-1690`, `api/v0/endpoints/repos/handlers/repos.go:760` | Pass the repo's default branch into the cascade; never delete a branch Crowbar did not create (record `CreatedBranch bool`); never `--force` a protected or locked worktree. |
| P0-2 ✔ | **Boot sweep `rm -rf`s the whole workspace root**, including foreign checkouts; for a legacy pre-leaf `<slug>/<branch>` path that is every sibling workspace of the repo. It is an older copy of the delete reactor's remover that never got the "4.5 GB of hand-made worktrees" hardening. | `app/container.go:726-762` vs `repositories/container.go:982-1212` | Export the hardened `worktreeRemover` and call it from the boot sweep; delete `removeWorkspaceRoot`/`underHome`. |
| P0-3 | **Move repo to project B, delete project A → B's worktrees and chats are wiped.** `applyRepoProject` changes `ProjectID` but leaves worktrees under `projects/A/`; `projectDelete` skips them by ID then `RemoveAll(projects/A)`. | `usecases/project/project.go:529-557`, `project_delete.go` | Refuse project delete while any live row's path is under it (invariant D6), or move the worktrees on reassign. |
| P0-4 | **Boot sweep leaks provisioned placeholders** (reads path only from `wspaths`, which is never updated by `ProvisionInPlace`), then forgets the row. | `workspace.go:455,664` | Purge from the tombstone's `WorktreePath`; then delete `wspaths` (§7-D). |
| P0-5 | **Dead shell resurrected under the same id.** An `Attach` between shell exit and `reapOnDone` sees `!IsLive()`, "restores" a new shell, and the reaper then skips `onExit`/ended because `cur != s`. The web's reconnect-on-drop makes this likely. | `core/terminal/terminal.go:1039,1268,805-848` | Explicit session state (`Live/Exited/Suspended/Removed`); only `Suspended` may be restored. |
| P0-6 | **Typing `exit` anywhere kills the local PTY** — inside ssh, a Python REPL, or an agent TUI — via a keystroke sniffer. | `features/terminal/hooks/use-terminal-connection.ts` (`onData`) | Delete the sniffer; daemon sends an `exit` frame (§7-B). |
| P0-7 | **Stop blocks up to ~150 s** behind a concurrent switch/resume holding the non-cancellable spawn gate. | `runner/lifecycle.go:385`, `inflight/internal/gate` | Context-aware gate; Stop cancels an in-flight switch/resume wait. |
| P0-8 ✔ | **Stop records no "Interrupted" divider** on the retire path: `displace` completes the turn, then `RecordStop` returns early on "no inflight turn". The test's `spyStopTurns` hides it. | `runner/lifecycle.go:30,43`, `turn/turn.go:285`, `switchwait.go:143` | Record the interruption before completing the turn; test against the real `Turns`. |
| P0-9 | **Reload tab silently marks unsaved edits as saved** (reopens from the in-memory buffer, sets `savedContent = content`). | `features/tabs/components/tab-bar.tsx:528-553`, `buffer-slice.ts:256` | Reload reads from disk; prompt if dirty. |
| P0-10 | **Autosave loses/mislabels edits**: one timer shared by every buffer; marks clean even if edits landed during the write. | `features/editor/stores/editor-app-store.ts:287-330` | Per-buffer debounce; compare a content version before clearing dirty. |
| P0-11 | **Closing a view leaks its tabs' buffers and their terminal PTYs** forever (persisted). | `features/panes/lib/view-ops.ts:38-42,169` | `removeView` closes/kills unreferenced buffers in the same `set` (invariant C2). |

## 4. Phase 1 — guardrails, so trash stops coming in

Without this, agents add workarounds back as fast as they are removed.

**CI (`.github/workflows/ci.yml`)**
- Run `golangci-lint` in CI (`golangci/golangci-lint-action`, a version built
  with Go 1.26 — the installed 2.5.0 can't lint this module, so nobody runs
  `make lint` today). Expect a backlog; land with `--new-from-rev` first.
- `.golangci.yml`: `nolintlint.require-explanation/require-specific: true`;
  stop excluding `errcheck` for tests; add `errorlint`, `nilerr`, `noctx`,
  `contextcheck`, `forcetypeassert`, `unparam`, `dupl`, `godox`, `testifylint`,
  `thelper`, `forbidigo` (`time.Sleep` outside tests, `fmt.Print*` in
  `internal/`). Fix the 27 `//nolint` directives that name disabled linters.
- Dead-code gates: `deadcode -tags noEmbed ./cmd/...` (allowlist `mocks/`) and
  `bunx knip --production`. **Coverage without a dead-code gate rewards testing
  dead code** — the repo has `*_coverage_test.go` files that say exactly that
  (`engine/git/internal/exec/exec_coverage_test.go:16`).
- ESLint: `--max-warnings 0`; `no-explicit-any: error`;
  `ban-ts-comment` (allow-with-description); `eslint-comments/require-description`;
  `no-restricted-imports` so `**/stores/**` can't import `@/components/*`;
  `max-lines: 600` (baseline existing offenders); `@vitest/eslint-plugin`.
- A grep gate rejecting new files named `*_coverage_test.go`, `*_extra_test.go`,
  `*_gaps_test.go`.
- Nightly job running `api/tests/integration/*` (minus paid `agent`) — it
  exists but nothing runs it.

**Agent rules — additions to `CLAUDE.md`** (short, each one enforceable):

```md
## Fixing bugs
- A fix starts with a failing test that reproduces it.
- Never fix a symptom with a timer, poll, retry, extra flag, or resync effect.
  Find the owner of the state. If the fix needs a second copy of some state,
  stop and write a plan in docs/plans/ instead.
- A known bug is an issue, not a `t.Skip`.

## Hygiene
- Delete code you make unused in the same change. No exports kept "for API
  completeness"; deadcode/knip must stay clean.
- No stubs that ship: a UI must not be wired to a method that returns [] or false.
- Comments explain *why* in ≤3 lines — no incident narratives or PR history.
- Don't commit plans-as-scratch, QA logs, spikes, test output, placeholders.
- No second library for a job already covered (icons: phosphor; DnD: dnd-kit;
  headless UI: base-ui; highlighting: shiki).

## Tests
- Test behaviour through the public API; never add a test only for coverage.
- Extend the unit's existing test file instead of `<unit>-<scenario>.test.ts`.
- ≤3 `vi.mock` per file; no asserting Tailwind class names or reading src/ text.
- No sleeps: channels, `require.Eventually`, `waitFor`, fake timers.

## Lint suppressions
- Every nolint / eslint-disable / react-doctor-disable / ts-expect-error names
  the rule and gives a reason on the same line. Never loosen a lint config,
  coverage floor, or budget to get green.

## Go
- No `...ForTest` methods or test-only package vars in non-test files; use
  export_test.go or an injected config/clock.
- Required dependencies are constructor arguments, not nil-checked setters.
```

## 5. Phase 2 — mechanical deletion (low risk, tool-proven)

Tools prove these dead; each bullet is one small PR.

**Go** (`deadcode` baseline: 129 non-mock functions reachable only from tests)
- `internal/adapter/registry.go` (whole file) + tests; `internal/api/v0/ws/dispatch.go`;
  `sqlite.New`/`OpenDB`/`OpenDBWithPool`; `SweepDanglingAliases`;
  `exec.GitWithEnv` + `exec_coverage_test.go`; ~21 unused `api/v0/dto` funcs;
  `adapter/container.go:522 closeIfCloser`; `tree/subtree.go subtreeIDsOf`,
  `tree/plan.go globalSnapshot`; `paths.State`, `worktreepath.SlugDir`;
  `Session.fanOut`, `Session.SerializedLen`; unused mock methods in
  `repositories/{reviewthread,workspace}/internal/mocks`.
- Dead fallback in `repos.go:508-575` (`buildRepo`, `gitDefaultBranch`, the
  non-importer `persistRepo` branch — the importer is always wired).
- Test hooks in production files: `driveCyclesForTest`, `waitRunnersForTest`,
  `DefaultLocaleForTest`.

**Web** (`knip`: 5 unused files, 144 unused exports, 1 unused devDependency,
1 unlisted dependency `@platejs/slate`)
- Terminal: `terminal-host.tsx` + `terminal-slots-store.ts` + the
  `crowbar-terminal-refit` listener and park div (nothing ever registers a
  slot); `remoteConnectionId` (threaded through ~8 files, never set);
  `onTerminalExit` / `explicitExitRequestedRef` / `lastExitInfoRef` (never
  called/read); the `terminal-ready` event (no listeners).
- Layout: `agent-chat-tab-icon.tsx`; `sidebar.ts` `addWorkspace`,
  `deleteWorkspace`, `reparentWorkspace`, `openChatRow`, `capturePlacement`;
  `lib/persistence/workspace-hierarchy.ts` and its overlay in `hydrate.ts:205`
  (stale local `parentId` laid over daemon truth at boot — a ghost-row source);
  the buffer-session persistence pipeline (`buffer-session-persistence.ts`,
  `window/stores/session-store.ts`, `window/stores/project-store.ts`) that
  runs on every buffer write and always returns early; persisted
  `sidebarWidth`/`rightSidebarWidth` never read.
- Editor: `external-editor-terminal.tsx` + `externalEditor` content type and
  setting; all `remote://` branches (7 sites, nothing creates such paths);
  `gitDiffCache` (only `.invalidate` is ever called); git-blame store/API stub;
  `git-repo-api` / `getRemotes` / `fetchChanges` stubs; `view-store.ts`,
  `use-center-cursor.ts`, the textarea-ref API; test-only files
  `lsp-surface.ts`, `diff-review-header.tsx`, `diff-search.ts`,
  `normalize-diff.ts`, `diff-buffer-path.ts`; dead listeners
  `menu-go-to-line`, `editor-trigger-suggest`.
- Agent: "older daemon" compatibility defaults in `agent-api.ts:305-345` and
  `mapProvider` (`:885-925`) — the web bundle ships embedded in its own
  daemon, so an older one cannot occur; make the fields required. Duplicate
  selector `chatProviderId` (`agent-chat-pane.tsx:384` = `:314`).

**Repo hygiene**
- Delete: `web/test-results.log` (and fix `.gitignore`'s `_.log` typo →
  `*.log`), `web/index.ts` (bun-init hello world), `web/pnpm-workspace.yaml`
  (unfilled placeholder; pnpm retired), broken symlink
  `web/.cursor/rules/*.mdc`, `api/web/dist/index.html` (nothing references
  `api/web`), `flows/qa-findings.md`, the `pierre-diffs-spike.test.tsx` spike.
- Archive (branch or tag, then delete from `develop`): `docs/superpowers/`
  (252 files, 7.6 MB of historical agent plans/specs, incl. a Python spike),
  `docs/prompts/` (build-wave prompts — lift the standards into CLAUDE.md
  first), `docs/v0/*.png`, `api/docs/specs/v0/`, the duplicate
  `docs/plans/2026-05-18-scaffolding.md`.
- Rewrite `api/ARCHITECTURE.md` — it describes a flow engine, kanban, MCP and
  wiring that no longer exist, and it misleads every agent that reads it.

## 6. Phase 3 — owner decisions (resolved 2026-09-24)

1. **Stub-backed editor features → connect them.** Completion, hover, rename,
   references, code actions, document symbols, signature help, code lens,
   format and blame are implemented as Monaco providers over the daemon's
   existing `/lsp/*`, `/blame` and `/search` routes. The custom React overlays
   that re-implement Monaco widgets are deleted. No UI may remain bound to a
   method that returns a constant.
2. **Mock mode → delete** (`dev:mock`, `lib/mock`, `src/mocks`, chaos store,
   chaos headers in `lib/api.ts` and the CORS allow-list).
3. **Legacy data shims → delete.** All data is already on the current model.
   Remove every mint-on-read / upcaster / legacy tie-break / pre-leaf path
   guard and the `regression_legacy_*` tests that pin them. Make the
   owning-chat creation saga crash-safe first, since `EnsureOwner` was its
   only repair.
4. **Duplicate libraries → consolidate now.** dnd-kit (drop react-dnd),
   phosphor (drop lucide), base-ui (drop radix + ariakit where possible),
   shiki (drop lowlight), one read-only markdown renderer, drop
   `usehooks-ts`/`use-debounce` for local helpers, one geist-mono package.
5. **Hook delivery → both in-memory.** The relay's short retry and a bounded
   in-memory TTL dedup set replace the fsync'd exactly-once journal. No fsync
   on the hook hot path; memory bounded by TTL and a size cap.

## 6a. Performance mandate

Crowbar must be fast and frugal. Every area's work is judged on:

- **Idle cost ≈ 0.** No client polls (every poll in §7-A goes); no timers
  that fire while nothing changes; the daemon does no periodic work that an
  event could trigger. Idle CPU of daemon + webview is measured before/after.
- **Hot paths do no redundant work.** No per-frame full-store scans
  (`useViewWorkspaceIds`), no per-keystroke `lines[]` split (`view-store`), no
  fsync per hook delivery, no JSON-string encoding of PTY bytes (binary
  frames), narrow store selectors everywhere.
- **Bounded memory.** Every per-chat / per-session / per-buffer map has an
  owner that deletes its entries (invariants A7, B3, C2); caches have caps.
- **Smaller bundle.** Duplicate libraries removed (§6.4), heavy features
  lazily loaded, bundle budget ratcheted down after each removal.
- Numbers (bundle size, idle CPU, RSS after opening N chats/terminals) are
  recorded in each area's final PR.

## 6b. Designed for what comes next

New features land in weeks, so the target designs are extension points, not
just cleanups: a chat is a versioned snapshot with an explicit `phase` (new
providers and new chat states plug into one reducer); a terminal session is an
explicit state machine (new session kinds add states, not maps); a view is a
record with typed members (new pane content types register in one content
registry); workspace lifecycle is one service with one saga (new workspace
kinds add a state, not a fourth encoding); the editor is Monaco plus providers
(new language features are one provider each).

## 7. Phase 4 — per-area structural work

Order is by bug history: **A → B → C → D → E**. D's Phase-0 items are already
out of the way, so D can run in parallel with A/B if there are two people.

### A. Agent session & chat lifecycle

**Target design**
1. **One versioned chat snapshot on the wire.** Every chat WS frame *and* every
   GET carries the full chat DTO plus the aggregate version, including
   `working` (from the command-side fold, not the lagging projection),
   `liveRunnerId`, `providerId`, and a new
   `phase: dormant|starting|live|switching|stopping`. The client applies a
   snapshot only if its version is newer — whatever its source.
2. **Server-owned lifecycle.** Queueing, busy barriers, resume-on-send and
   revive move into the daemon; the client posts intents and renders `phase`.
   `chat.ProviderID` (already written on every spawn) becomes the only
   provider owner after a one-time backfill.
3. **One channel per surface.** Spawn a PTY for api-transport providers only on
   `SwitchToTerminal`. The "companion PTY" — self-described as "a known gap" in
   14 places — is what forces the dual-channel dedup machinery.

**Invariants**
- A1 After `StopChat` returns, no provider process, API connection or attached
  PTY for the chat is alive, `Working=false`, and exactly one `stopped`
  interruption exists if a turn was open.
- A2 `StopChat` completes within a bound while a switch/resume is in flight.
- A3 `inflight.Work`, the caught-up projection and workspace `agentWorking`
  agree with the aggregate fold; `Turns` is non-empty only if the turn is open.
- A4 At most one live runner per chat, and per (workspace, session).
- A5 Every hook event delivered on either channel is written to the ledger
  exactly once — including "owner live, nothing dispatched".
- A6 A client state write applies only if its version > the held version.
- A7 Deleting a chat removes every per-chat entry (`inflight`, `apiConns`,
  `attached`, answer desk, web slice maps, `streamingMessages`).

**Delete once the design lands**
- `web/src/features/agent/lib/chat-read-order.ts` (its header: "not fixable
  from the client… needs the server to stamp each chat row with its own
  revision"), plus `listSeq`, `needsReconnectReconcile`, `keepWorking`,
  `bornLive` and the 3-attempt reseed loop in the stream hook.
- The five polls: 5 s working poll (`agent-chat-pane.tsx:686`), prompt-queue
  busy recheck (`use-prompt-queue.ts:277`), activity poll + falling-edge retry
  (`use-agent-activity.ts:84-127`), 1 s orphan message poll
  (`use-chat-messages.ts:316`), compaction timeout backstop
  (`use-workspace-agent-chats-stream.ts:858`).
- Client revive orchestration: `attemptedRef`, `switchingRef`, module-global
  `reviveInFlightByChatId`/`displacingByChatId`, `escortRef`,
  `REVIVE_REQUEST_BOUND_MS`.
- `turnRevision` idle-edge inference; prompt confirmation by text equality
  (`samePrompt`) → match by `clientRequestId` on the ledger row.
- Companion-PTY machinery: `WithAPITransport`/`FromAPITransport`,
  `ownerDropsThisDelivery`, `HasDispatchedOverAPI`, `connloss.go`, the
  `retire` third-PTY cleanup, the static `TransportFor` skip in the pump
  (`apiconn.go:376`).
- `ActiveProviderID` three-source scan; provider logic in `api/v0/dto/agent.go:607`.
- 3 s `AwaitOpen` delta-vs-turn_stop wait (`turn/message.go:84-100`) → close on
  turn_stop, upsert late deltas by message id.
- Nil-registry "for test doubles" guards in `apiconn.go:79-100`.
- Read-side staleness nets (`withStaleSubagentsClosed` 2 h,
  `withStaleToolCallsClosed`) → a counted, logged dead-letter only.

**Other bugs to fix in this area** (each with a test first)
- The web working poll overwrites newer frame state and triggers a prompt
  re-submit into a live turn (`agent-chats-slice.ts:612` describes it).
- API pump blocks all later codex events on an unanswered permission prompt
  (`apiconn.go:398`); when buffers (64/32) fill, `interruptTurn` hangs holding
  the spawn gate.
- `guardNotWorking`/`Usecase.Working` ignore `known=false`, reading an
  untouched-since-boot chat as idle.
- Unbounded `inflight.Work.states`, `Turns.changed`, web `streamingMessages`.
- Stale docs: `gate.go` ("nothing else takes it" — 13 sites do),
  `hook_deliveries.go:54`, `turn/turns.go:67`.

**Splits:** `agent-chat-pane.tsx` (1 855 lines, 21 effects) →
`use-chat-lifecycle.ts`, `use-provider-selection.ts`,
`chat-terminal-presentation.tsx`, thin pane (most of the lifecycle hook
disappears under target 1). `use-workspace-agent-chats-stream.ts` → a pure
`reduceChatFrame(state, frame)` with table tests + a subscription shell.
`agent-api.ts` → per resource. Go `runner/` → `lifecycle/`, `apitransport/`,
`promptdelivery/`. Reconcile with the decomposition plan (2026-08-20), which
the code diverged from (an `aliases.go` re-export layer; `os` imports in 7
runner files).

### B. Terminal / PTY

**Target design**
1. **One lifecycle owner per session.** Replace the eight maps (`reg`,
   `sessionMu` sync.Map, `cmdCleanups`, `lastActive`, `endedOnce`, `.buf`
   files, meta rows, `reaps`) with one
   `sessionEntry{state, sess, chatID, onExit, lastActive, mu}` that is never
   deleted while referenced. Every transition (restore, suspend, kill, exit,
   evict) goes through one `transition()` that runs that state's cleanup set.
2. **Daemon is authoritative about exit.** An `{"type":"exit","code":N}` frame
   before close (or consume the existing, currently unconsumed, terminals
   lifecycle topic). On the web, a transport drop reconnects only if no exit
   frame arrived.
3. **One transport.** One Go WS handler (delete the copy in
   `home/handlers/terminal.go` and the `[]string`-vs-DTO dual shape), binary
   frames (removes the UTF-8 holdback), one Rust bridge (`ws_bridge.rs`;
   delete the ~500-line `terminal.rs` duplicate), one JS connection class with
   a listener *set*, and one PTY-size owner (`syncPtySize(connId)` driven by one
   ResizeObserver).

**Invariants**
- B1 Every session reaching `done` yields exactly one ended event and one
  `onExit`, under any interleaving of Kill / self-exit / Attach / Shutdown.
- B2 No resurrection: once a process exits, nothing spawns under that id;
  only `Suspended` is restorable.
- B3 Registry, `.buf` and meta row agree with the session's state after every
  engine operation (fuzz the op sequence).
- B4 Every PTY exit reaches the web as an `ended`/`exit` frame; no keystroke
  parsing.
- B5 Every Attach delivers exactly one snapshot before any delta; model and PTY
  dimensions match after every Resize.
- B6 Attach returns within a bound after its client stalls or the session dies.
- B7 A shell tab never spawns a PTY because an existing one exited.

**Delete once the design lands:** the `exit` sniffer; `endedOnce`; the
`sessionMu.Delete` pattern (it breaks mutual exclusion — a waiter on the old
mutex and a new `lockSession` caller hold different locks); raw-output
fallback machinery reachable only via test fakes (`pumpStep` else-branch,
`Resync` raw branch, `modelDrivenFellBack`, `emitForTest`) → on panic,
re-create the model and send a keyframe; `attach-refcount.ts` (guards a
two-views scenario the single-listener bridge can't support anyway); duplicated
`spawn`/`spawnCmd`; the timing hacks (400 ms list retry, 100 ms font settle,
200 ms init timer + parallel rAF init poll, 10 ms theme tick, 300 ms resync
debounce, 300 ms initial-command sleep, 20-frame dimension poll, 8-try focus
retry); probably the resync round-trip (`Resize` already invalidates to a
keyframe — prove with a test first). Test seams (`newModel`, `startupWrite`,
mutable package limits, clock fields) → one injected config/clock.

**Other bugs:** `evictPlaceholder` never fires ended; `writePump` has no write
deadline so a stuck peer blocks `Attach` forever; `Kill` SIGKILLs only the
shell pid while `Terminate` signals the process group (background jobs survive
Kill/Shutdown); callbacks run while holding `sessionMu` (re-entrant deadlock
risk; `TerminateGraceful` holds it up to 3 s); `Snapshot` reads `s.model`
without `s.mu`; ignored meta-store/startup-write errors
(`terminal.go:700,852,990,1005,2119`). Desktop: `shutdown_sidecar` signals a
pid read from `/v0/health` (could be a stale daemon), SIGKILLs unconditionally,
and runs on `CloseRequested` even if the close is later prevented.

**Splits:** `terminal.go` → `lifecycle.go`, `persist.go`, `transport.go`,
`env.go`. `session.go` → `birth.go`, `pump.go`, `snapshot.go`, `control.go`
(+ `session_testseams.go`). `terminal.tsx` (1 573 lines, 29 effects) →
`useXtermInstance`, `useTerminalAttachment` (init + reconnect as one path),
`usePtySizeSync`, `useTerminalInputHandlers`. `lib.rs`: move
`shutdown_sidecar` next to `kill_wedged` in `sidecar/` (they duplicate
SIGTERM/SIGKILL logic).

### C. Layout state — panes, views, tabs, workspace stores, sidebar

Finish `views-are-records` (stages 1, 2, 4 are done; 3 and the membership fork
are not) and give workspace stores and tabs a single owner each.

**Target design**
1. **Record-carried identity.** `ViewRecord { id, projectId, members:
   {chatId, workspaceId}[], layout }`, filled at open (where
   `openChatInOwnPane` already knows both — `drop-actions.ts:998` currently
   computes the workspace and throws it away). Panes become a projection;
   "empty view ⇒ delete" goes. This retires 6–7 of the 8 chat→workspace
   resolvers, `useViewWorkspaceIds`' all-store scan on every stream frame,
   and the Recents network fallback.
2. **One lifecycle owner for workspace stores.** `WorkspaceHost` owns the
   registry (`mount(ids)` / `canEvict(id)` / `destroy(id)`); nothing else
   creates stores (today `getOrCreateWorkspaceStore` is called *in render* at
   6+ sites). One active-workspace id, in the registry; delete
   `workspace-store-ref.ts` (microtask-deferred null) and the scope duplicate.
   Compute force-mounts from the retention plan, not from every pane.
3. **Tabs owned by panes.** `pane.tabs: BufferRef[]` (or ref-counted buffers).
   `openContent` takes an explicit `paneId` — no more "call `setActivePane`
   first". `removeView`/`removePane` close or kill unreferenced buffers.
4. **One placement writer for the sidebar**, and no optimistic writes (the
   code's own stated convention, violated by repo-reorder).

**Invariants**
- C1 Views change only on gestures (open, close, reorder, detach, remove-member)
  — never on runner moves, teardown, eviction, hydrate or project switch.
- C2 No orphaned buffers: every buffer is referenced by a pane or is in
  `closedBuffersHistory`, every tab id names a buffer, and no terminal session
  outlives its buffer.
- C3 `views[v].projectId` and each member's `workspaceId` are set once at open.
- C4 At the end of each commit, all active-workspace readers agree (or are null).
- C5 Registry keys equal the mounted set; `|mounted| ≤ RETENTION_CAP` plus the
  active workspace.
- C6 Rendering any component never changes `registry.size`.
- C7 After any sidebar drop, order equals the daemon's answer or is reverted.
- C8 `openContent(spec, {paneId})` lands in that pane regardless of focus.

**Bugs fixed by the design** (write each as a failing test first): keep-alive
cap defeated → stores destroyed under mounted views, re-minted in render,
WebGL cap breached (`workspace-host.tsx:203,286,315`); zombie store after a
vetoed destroy → permanent spinner (`registry.ts:340`); repo-reorder failure
never reverted (`drop-actions.ts:758-769`); views minted in the wrong project
(`view-actions.ts:52,142`); Recents fallback `forgetChat`s a live chat on a
misattributed project (`use-recents-chat-fallback.ts:64-100`); runner move
silently drops views (`runner-actions.ts:327`); active-workspace copies
disagree within a commit (`workspace-view.tsx:76` vs `:84`).

**Delete:** `resolveWorkspaceIdForChat`/`resolveChatOwnerWorkspaceId`/
`isKnownChatId` pair (each with a "NOT interchangeable" doc); `stripNewTabs`
repair-on-save; `release-closed-chat.ts:55` stale guard; the 100 ms
close/reopen in `tab-bar.tsx:540`; `waitForRow`/`waitForHomeTree` (promises
that never settle) → daemon returns the placement in the create response;
Go-error-string matching `"parent branch is not yet provisioned"`
(`drop-actions.ts:916`) → a `provisioned` field on the DTO;
`window.__fileDragData` + custom window events as a drag bus → one drag-state
store; the 7 hand-placed `healActivePane` calls (focus becomes a derived invariant).

**Splits:** `pane-container.tsx` → `usePaneWorkspace`, `usePaneDropHandlers`,
`usePanePresentation`, `PaneAccentRing`, content-type registry.
`space-content-actions.ts` → `open-`, `trash-`, `create-`, `home-actions.ts`,
with the repo and home create paths made one implementation.
`lib/store/sidebar.ts` → `sidebar-ui` + `repo-tree` (DTO appliers).

### D. Storage, worktrees, domain lifecycle (Go)

**Target design**
1. **One lifecycle service** owns workspace / repo / project
   create-provision-delete. Creation is one idempotent, resumable saga (intent
   record/outbox) with a **boot reconciler** that checks invariants D1, D3, D4,
   D5 across DB, git and filesystem. Deletion: the usecase writes a tombstone;
   the reactor is the *only* physical purger and the boot sweep calls the same
   function. This deletes `wspaths`, `purgeRemainingWorkspaceRows`, the
   orchestration in the repo HTTP handler, and `projectDelete`'s own teardown.
2. **Explicit state instead of an overloaded `Status`:** `Protected bool`,
   `Provisioning{provisioned, placeholder, shared}`, `Lifecycle{live, deleting}`,
   `PR{…}`. No more inferring from `IsDefault`, `Kind` or an empty path. Then
   the migration of §6.3.
3. **Required dependencies.** Break the chat↔workspace cycle with an interface
   passed at construction; delete `SetChatObserver`/`SetOwningChats` and the ~99
   nil-guards that silently disable things (e.g. `guardNotWorking` returns nil
   with no observer — a safety guard that can switch itself off).

**Invariants**
- D1 Every live workspace under home has its directory and a matching
  `git worktree` registration on its branch; every registration under
  `<home>/projects` maps to a live row.
- D2 Each repo has exactly one `IsDefault` workspace; each project exactly one
  home workspace (`homeWorkspaceID(P)`); at most one live non-default workspace
  per (repo, branch). *(The deterministic-id fix used for project homes was
  never applied to repo homes → duplicates under concurrency.)*
- D3 Every live repo/workspace/folder/chat has exactly one Node row and every
  Node row references a live entity.
- D4 Every live workspace has exactly one owning chat, surviving a crash at any
  step of creation.
- D5 After any delete settles, nothing remains for the deleted ids (events,
  rows, Node, activity, telemetry, chat dir, worktree root); blobs remain only
  if referenced; pre-existing user branches and foreign directory entries are
  untouched.
- D6 A workspace's path never lies under another project's directory.
- D7 Every referenced content blob exists; every blob is referenced or pending GC.
- D8 Placeholder status is explicit; retry/detach keep the id.

**Consolidate duplicates:** two protected-branch provisioners
(`project_import.go:752-919` vs `worktree.go:1305-1374`, which disagree on
whether the placeholder is locked); two owning-chat sagas
(`project/owning_chats.go` vs `hierarchy/owning_chats.go`; lazy `CreateHome`
skips it entirely); three delete orchestrations with different guards; copies
justified by a stale "cannot import worktreepath" comment (`repoDir`,
`pathSlug`, `underHome`, `managedWorktreePath`, `siblingWorktreePaths` ×2,
`gitRemoteURL` ×2, `samePath`/`resolvePath` ×2); folders rebuilt as chats in
4 places. Route `Branches` (`repos.go:1041`, shells out to `git branch -r`)
through the git engine and its per-repo lock.

**Other bugs:** workspace/repo/project delete leaves activity, telemetry, Node
rows and blobs (`forgetAgentChats` skips them, unlike `PurgeLocked`);
`DeleteRepo` deletes the row then cascades in a goroutine — a crash between
leaves orphans no sweep finds, and a failed `store.Delete` after the 202 is
silent (`repos.go:785`); project delete skips `git worktree remove` for locked
worktrees then `RemoveAll`s them, *manufacturing* the dangling registrations
the protected-branch spec works around; check-then-act on the lagging read
model in import guards (a `FindAll` error means "importable"); blob ref check
races dedup (`activity.go:545-580`); `CreateChild` still silently detaches the
repo home, against that spec's consent rule; discarded errors on real I/O
paths — list-errors-become-"not protected" (`repos.go:1053,1066,1108`),
rollback deletes (`project_import.go:348,372,481,483`), the LastError sink
itself (`git/handlers/async.go:33,38`), `WorktreePrune` (`holder.go:58`),
empty fork point on RevParse error, the prompt-durability journal
(`runner/prompts.go:113,272,277`, `promptrecovery.go:263,267`).

**Tests:** replace the workspace/repo/node fakes in `usecases/mocks` with real
SQLite stores in a temp dir — the fakes model neither read-model lag nor OCC,
so invariants "tested" against them don't hold against the real store.

**Splits:** `hierarchy/worktree.go` → `create.go`, `provision.go` (absorbing
`project_import.go:709-919`), `merge.go`, `delete.go`. `repos.go` → HTTP
binding only. `repositories/container.go` → extract `workspace/purge` and the
working overlay.

### E. Editor, git, explorer, settings

1. **Monaco is the only owner of editor state.** Delete `extensions/api.ts`
   (884 lines re-implementing comment toggle, bracket jump, multi-cursor, line
   moves, undo — all Monaco-native) and the six utils + tests only it uses,
   the editor `history-store`, and `use-lsp-integration.ts` (its effects can
   never run). Per §6.1, implement kept LSP features as Monaco providers over
   the existing `/lsp/*` routes. `reveal()` returns an "editor ready for
   bufferId" promise, removing the 100–150 ms jump hacks
   (`use-go-to-definition.ts:181`, `jump-navigation.ts:130`) and the wrong-offset
   bug they cause on large files. Use Monaco's find widget instead of the
   re-implemented `find-bar.tsx`.
2. **One settings store, one persistence path.** Delete the editor-settings copy
   and its 50 ms sync, the write-only IndexedDB ui-preferences, and the
   Tauri-store-shaped localStorage shim.
3. **One read-only markdown renderer** (today: a hand-written regex parser, a
   react-markdown component and Plate — two of them both named
   `MarkdownPreview`).
4. **Git refresh keyed by workspace**, not the global `rootFolderPath`: today
   every save reloads git in every mounted workspace via the
   `git-status-updated` window event. Replace both `git-status-*` window buses
   with a store action.
5. Small fixes: HTML preview relative assets (`convertFileSrc` is identity);
   `diagnostics-export.ts:16` uses `window.__TAURI_INTERNALS__` directly
   instead of `crowbar-bridge`; `attachment-drag-handle.tsx:190` calls a hook
   conditionally with the lint rule disabled.

**Invariants:** one `didOpen` per path per ref; the cursor lands on the right
line after a cross-file reveal; no UI control is bound to a method that returns
a constant.

**Splits:** `file-explorer-tree.tsx` (1 478) → `useVisibleTree`,
`useTreeSearch`, `lib/open-all.ts`, `useTreeContainerEvents`, ~500-line shell.
`review-code-view.tsx` (1 198) → pure `lib/placeholder-hunks.ts`,
`usePatchLoader`, binary/image rows, view. Leave vendored Plate UI
(`table-node.tsx` etc.) alone.

## 8. Tests — what to keep, what to change

- **Keep:** rule-per-test style (`tree/move_test.go`), security regressions
  against real git (`branches/arginjection_test.go`), the integration kit.
- **Stop:** coverage-named tests of dead code; 17-mock component tests that
  assert "renders SidebarCarousel" (`ide-shell.test.tsx`); 254 class-name
  assertions; 9 tests that `readFileSync` source files; one-file-per-bug
  siblings (186 web test files don't match their source path).
- **Skipped tests as a bug tracker** — five `t.Skip("product bug: …")` in
  `api/tests/regression_agent_chat_threads_test.go:311,455,611`,
  `agent_runner_moves_test.go:655`, `integration/crash/crash_test.go:273`.
  Turn each into an issue and fix it inside its area.
- **Environment-sensitive tests.** Under root (as in cloud CI containers) 9
  permission tests fail because root ignores read-only dirs; guard them with a
  "running as root" skip. `TestRegisterDeleteReactor_GatedPurge_*` failed once
  under full-suite load and passed 3/3 alone — flaky, root-cause it.
  `TestStart_WatchesResolvedRefPathsOfLinkedWorktree` failed on this Linux box
  (worktree private `refs/` dir not watched) — check against the git version
  the product supports.
- The CI comment says two tests in `api/tests/repo_import_test.go` hang on
  Linux, but the file has no skip: fix or delete the comment.

## 9. Sequencing

| Step | Content | Size | Risk |
| --- | --- | --- | --- |
| 1 | Phase 0: P0-1…P0-11, one PR each, test first | ~11 small PRs | low |
| 2 | Phase 1 guardrails (CI, lint, CLAUDE.md), baseline existing offenders | 2–3 PRs | low |
| 3 | Phase 2 mechanical deletion + hygiene | ~8 PRs, net −10k+ lines | low |
| 4 | Phase 3 owner decisions | a conversation | — |
| 5 | A — agent lifecycle (versioned snapshot → server lifecycle → companion PTY) | largest | high |
| 6 | B — terminal (state machine → exit frame → one transport) | large | medium |
| 7 | C — layout (records → registry owner → tabs) | large | medium |
| 8 | D — storage (lifecycle service → explicit state → migration) | large | high |
| 9 | E — editor (Monaco owner → settings → markdown) | medium | low |

Within each area: invariant tests land first (they may fail — mark them as the
area's exit criteria, not skipped), then the owner, then one workaround
deletion per PR.

## 10. How we know it worked

Tracked per area, reported in each area's final PR:

- Share of `fix:` commits touching the area over the following month (target:
  it drops; if it doesn't, the area isn't done).
- Net lines of code (target: down, substantially).
- Counts: `setTimeout` in `web/src` (92 today), `useEffect` in the three
  largest components (21 / 29 / 5), `//nolint` (136), `eslint-disable` (37),
  `deadcode` hits (129), `knip` unused exports (144), `if x == nil` optional-dep
  guards (~99), copies-of-state from §1's table.
- `go test -race` stays at zero races; `golangci-lint`, `deadcode`, `knip`,
  ESLint `--max-warnings 0` green in CI.

## Appendix — tooling baseline (2026-09-24)

- `go vet ./...`: clean apart from the missing `web/dist` embed in a fresh
  checkout (build with `-tags noEmbed`).
- `go test -race ./internal/...`: **0 data races.** Failures: 9 permission
  tests that can't fail as root, 1 load-flaky reactor test, 1 fs-watch test
  (see §8).
- `deadcode`: 129 non-mock functions reachable only from tests.
- `knip` (web): 5 unused files, 144 unused exports, 1 unused devDependency
  (`shadcn`), 1 unlisted dependency (`@platejs/slate`).
- `tsc --noEmit`: clean.
