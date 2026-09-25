# Agent sessions that never break

Status: AUDIT + SPEC + PLAN. Date: 2026-09-24. Area: stabilization §7-A.
Owner's report: sessions that cannot be resumed (users fork a thread to
continue), Codex cannot start in TUI mode, runners lost mid-operation, chats
that "bug out" mid-turn, and the native (api) interface occasionally blocking
the whole conversation. Plus: descriptors are the main source of breakage and
must be validated like untrusted input.

Everything below marked **(live)** was reproduced against the real binaries in
this container (codex-cli 0.156.1, Claude Code 2.1.281) driven through a PTY,
with a local fake model server standing in for the vendor API (no credentials
needed — see §6.3). Everything else is from code, with file:line at the base
commit `70aaae7`.

## 1. Failure catalogue

### 1.1 Resume fails → the user forks a thread

A chat with `presentation.prompt_submit.strategy: restart_tui` (both built-ins)
re-launches its CLI on **every** prompt, and every launch after the first is a
`--resume <id>` / `resume <id>` (runner/promptdelivery.go `resolvePromptDelivery`,
runner/resume_injection.go:12). Resume is therefore on the hot path of every
message, not an edge case.

| # | Symptom | Mechanism | Missing invariant |
|---|---|---|---|
| R1 | "No conversation found" / chat drops dormant on send | The vendor session file is gone: claude prunes transcripts (`cleanupPeriodDays`), a CLI killed before its first turn completes never writes one (codex writes no rollout until a turn completes), the user deleted it. Crowbar forwards the recorded id blindly: `resumableConversation` (runner/switch.go:352) decides from Crowbar's own activity table and **age heuristics** (`sessionAnnounceCrashWindow`, `sessionLegacyMinAge`), never from whether the file exists. **(live)** claude `--resume <missing>` prints `No conversation found with session ID …` and exits 1; codex `resume <missing>` prints `No saved session found with ID …` and exits 1. | A session id is only used after the vendor's own store is probed for it. |
| R2 | Revive fails forever | The failed resume leaves the chat dormant. The client spends its one revive (`attemptedRef`, agent-chat-pane.tsx) on the same doomed id; `ResumeChat` → `switchProviderLocked` recomputes the same id and fails again. There is no next rung: the only escape is "new thread" — which is exactly what users do. | Resume is a ladder that always ends in a continued conversation (Crowbar's own transcript), never in a refusal. |
| R3 | Codex resume hangs on "Working directory · resume" | **(live)** `codex resume <id>` from a cwd other than the one recorded in the rollout paints an interactive modal ("Use session directory / Use current directory") before any hook fires. Worktrees are re-created and chats move between workspaces, so the recorded cwd routinely differs. No `terminal_prompts` needle matches it, so termwait never surfaces it: an empty pane over a blocked process. | Every launch is non-interactive up to the composer: the descriptor must pre-answer every boot modal (`-c tui.resume_cwd="current"`, verified to suppress it). |
| R4 | Wrong rung silently | When a resume does not happen (fresh spawn), nothing tells the user their provider started a new conversation with Crowbar's hand-off document; when it does, nothing records which id was used. | The rung used is recorded on the chat and rendered when it is not the first. |

### 1.2 Codex cannot start in TUI mode

| # | Symptom | Mechanism | Missing invariant |
|---|---|---|---|
| T1 | Codex terminal chat never starts | **(live)** codex 0.156 paints **"Trust this folder?"** (`enter continue · esc quit`) for any directory not in `[projects]` of `~/.codex/config.toml`. Every Crowbar worktree is a new directory, so every TUI launch (terminal surface `start_here`, `SwitchToTerminal`'s `attach`, every restart_tui prompt) parks there before `SessionStart`. The app-server path does not ask, which is why the chat surface works. codex.yaml's `terminal_prompts` (codex.yaml:1066) only knows the login screen's "Press enter to continue", so termwait never reports it. | Same as R3: descriptors pre-answer boot modals; the validator checks the TUI reaches its composer in a sandbox. Fix verified live: `-c projects={"<cwd>"={trust_level="trusted"}}` (session-layer, nothing persisted). |
| T2 | Codex terminal chat shows no session until typed into | **(live)** codex 0.156 fires `SessionStart` lazily — on the first turn, not at TUI boot. A prompt-less TUI launch (terminal surface, `ResumeChat`) has no session id until the user types. | "Live" is placement + process, never "announced a session". (Already true in Go; recorded so no one "fixes" it into a wait.) |
| T3 | Two codex processes per chat | `APIServeArgv` (agents.go:624) applies the descriptor's whole `config_injection`, including all 11 `hooks.*` entries, to `codex app-server`. The api channel therefore fires **every event twice** — once as a JSON-RPC notification, once as a `crowbar hook` relay — which is the whole reason for `owner:`, `ownerDropsThisDelivery` (turn/ingest.go:313), `HasDispatchedOverAPI`, the ctx channel marker, and the "companion PTY" folklore. It also double-asks permissions (api `requestApproval` + hooks `PermissionRequest`). | One channel per process: hook wiring is declared separately (`hooks_injection`) and applied only to a process whose surface channel is `hooks`. |

### 1.3 Runner lost mid-operation

| # | Mechanism | Missing invariant |
|---|---|---|
| L1 | restart_tui delivery kills the live CLI (`displaceForPrompt`, prompts.go) **before** it knows the replacement can start. A replacement that fails (R1/R3/T1, CLI missing) leaves the chat with no runner and the prompt journal `uncertain`. | The supervisor never tears down the outgoing process for a replacement it has not planned (ladder resolved, descriptor valid, binary resolvable). |
| L2 | Api connection loss (`onAPIConnLost`, connloss.go) reconciles the turn but the runner row is only exited through `watchExit` if the serve process also dies; a dropped socket over a live `serve` leaves a runner with no channel. | A runner is exactly one process + one channel; losing the channel kills the process and exits the runner with a recorded reason. |
| L3 | Exit reasons are not recorded: a crash, a failed resume, a connection loss, a displacement and a Stop all end as "dormant". The client guesses (`handleSessionGone`, `isDisplacing`) and sometimes revives, sometimes shows Resume. | Every runner exit records `{reason, at}` on the chat snapshot; the UI renders it, never infers it. |
| L4 | Daemon restart: PTYs do not survive; `ReconcileRunnersOnBoot` exits the rows and closes turns (correct). Nothing resumes on the next send: `SubmitPrompt` on a dormant chat returns `ErrPromptSessionUnavailable` (prompts.go:248); the client's revive budget decides. | Send on a dormant chat revives it server-side through the ladder, in the same gate hold as the delivery. |
| L5 | A hook for an unknown runner is dropped (turn/ingest.go `ingestHookNow`) — correct, but combined with L1 a relaunched CLI's early hooks can land before `recordRunner` (handled by the startup barrier) or after the runner was already reconciled (dropped). | Unchanged; covered by the startup barrier. Recorded as verified. |

### 1.4 Mid-operation "bug outs"

| # | Mechanism | Missing invariant |
|---|---|---|
| B1 | Duplicate api+hooks delivery (T3) guarded by `owner:` + "has dispatched" liveness — any window where the flag is wrong either doubles a turn or drops its only copy (both happened, see the comments in ingest.go). | Structural: one channel ⇒ no dedup at all. |
| B2 | An api permission ask whose answer budget expires is never replied to (`awaitAndReplyOverSocket`, apiconn.go:448, "codex's own attached TUI still has it" — false since codex no longer forks one). codex waits on `waitingOnApproval` forever; the turn never closes. | Every ask is answered: on expiry the desk replies the descriptor's deny/decline. |
| B3 | A spawn whose resume silently falls back to a fresh conversation still gets the gap-only hand-off (it believed it was resuming). | The hand-off content follows the rung actually used. |

### 1.5 Native interface blocks the whole conversation

| # | Mechanism | Missing invariant |
|---|---|---|
| N1 | `wsrpc.readLoop` blocks on `c.frames <-` (wsrpc.go:134, cap 32) and `apidriver.translateLoop` blocks on `drv.out <-` (apidriver.go:146, cap 64). A slow ingest (one DB write per event) back-pressures the **read loop**, and the read loop is also what delivers **responses** to our own calls — so `turn/interrupt` (Stop), `turn/start` (send) and `thread/resume` wait behind a stalled notification queue. | The read loop never blocks: responses are routed inline, notifications go to an unbounded-by-count but capped mailbox; overflow closes the connection (recorded reason `transport_overflow`) instead of wedging. |
| N2 | All writes hold `Conn.mu` across `ws.WriteMessage` with no deadline (wsrpc.go:197/258/274): one stuck socket write blocks every Call/Reply forever. | Every write has a deadline. |
| N3 | `interruptTurn` (stop.go:145) sends with Stop's request ctx: codex defers the reply until the turn ends, so a wedged turn keeps Stop (and the preempted spawn gate) waiting on the client's patience. | Every outbound call has a per-request timeout; Stop falls back to a kill after it. |
| N4 | `Driver.Send/Dispatch/EstablishSession` inherit caller ctx; the spawn path uses the request ctx, so an app-server that never answers `thread/start` holds the spawn gate until the HTTP client gives up. | Same as N3. |

### 1.6 A7 audit — per-chat state cleared on delete

`PurgeChat` retires runners, which clears `apiConns`, `attached`, `surfaces`
(per runner, on exit), the answer desk (`ReleaseRunner`), `inflight.Turns`;
`phases` is cleared by the operation's own defer; the snapshot owner deletes
the chat on the `forgotten` event. Remaining leaks found: none new beyond the
§7-A list (`inflight.Work.states` is keyed by chat and cleared only by
`Set(false)`); the new supervisor state (`sessions`) is deleted on purge and
tested (§5).

### 1.7 Found while proving it (Phase 4)

Each has a failing test first; **(live)** marks ones only the real CLIs showed.

| # | Mechanism | Fix |
|---|---|---|
| P1 | A native (api) codex turn whose `turn/completed` never came was never closed by the provider's idle report: termwait skipped runners without a PTY. | termwait consults the idle report for every runner. |
| P2 | The idle-report close left `inflight.Turns` open, so the chat stayed busy for every later send. | `AbandonMessage` completes the in-flight turn. |
| P3 | A refused resume's prompt was written off. | Redelivered on the transcript rung (same request id). |
| P4 | A replacement that could not start left a silent dormancy. | `spawn_failed` exit reason. |
| P5 | codex's thread announcement at startup was dropped before the runner row existed, so codex never resumed natively. | The startup barrier also buffers api events. |
| P6 | Switching back from the terminal / Stop while in the terminal deadlocked on the spawn gate (exit callback ran inside the gated teardown). | The exit callback runs as tracked background work taking the gate itself. |
| P7 **(live)** | SIGKILLing `codex app-server` leaves its thread's writer lease, so the next one was refused the thread and every stop silently forked a new codex session. | SIGTERM, bounded wait for the reap, then kill. |
| P8 **(live)** | A resumed CLI that Crowbar stopped, or that had already reported hooks, was read as refusing its resume and the session quarantined. | Any ingested hook confirms the launch; an exit Crowbar caused never counts. |
| P9 **(live)** | A turn_stop waiting for its final `message_delta` held the runner's hook gate the delta needed: always a 3 s stall and a duplicate assistant row. | The wait steps out of the gate. |
| P10 **(live)** | claude's first-run screens (theme, login method) were not recognised as blocking; a parent Claude Code session's identity env leaked into spawned CLIs; hook commands broke under a home path with a space. | Needles, `env.clear`, shell quoting. |
| P11 | The web queue confirmed a delivered prompt by comparing text. | The user turn is recorded under the prompt's clientRequestId; the queue matches by id. |

## 2. Target design

### 2.1 One supervisor per chat, on the daemon

`runner.Runners` already has the right skeleton — a preemptible per-chat spawn
gate, `phases`, a versioned snapshot owner. What it lacked is **one place that
decides** and **a record of why**. The supervisor is that place:

- **State** (in memory, per chat): `phase` (dormant|starting|live|switching|
  stopping — existing), `rung` of the current runner's launch (`session`,
  `transcript`, `fresh`), and `lastExit {reason, detail, at}`. Joined onto every
  snapshot (`dto.AgentChat.session`), so the client renders, never infers.
- **Intents** (the only client verbs): send prompt, stop, switch provider,
  resume (explicit — the terminal surface's "start session" button), open in
  terminal / back to chat. Everything else — resume-on-send, revive after a
  crash or daemon restart, fallback between rungs — is the daemon's.
- **Transitions** all run under the chat's spawn gate; every wait parks on the
  gate's preemptible park context (Stop cancels it); every call to a vendor
  process has a timeout; no lock is held across process or network I/O
  except the spawn gate itself, which Stop can always preempt.
- **Exit reasons**: `stopped`, `displaced`, `crashed` (process exited on its
  own), `resume_failed`, `connection_lost`, `transport_overflow`,
  `daemon_restart`, `spawn_failed`. Recorded by the one exit path
  (`reconcileRunnerExit`) from what the caller or the process told it.

### 2.2 Resume ladder (declared per provider)

`session.resume` gains a `locate:` list — where the vendor keeps a session,
relative to an env-overridable root:

```yaml
session:
  resume: { arg: "--resume {id}" }
  locate:
    root: { env: CLAUDE_CONFIG_DIR, default: "~/.claude" }
    glob: ["projects/*/{id}.jsonl"]
```

The supervisor resolves a launch in this order and records the rung:

1. **session** — the chat's last recorded vendor session for this provider
   (`ConversationsForChat`) *and* `locate` finds it. A descriptor without
   `locate` keeps today's behaviour (trust the id), reported as a validator
   warning.
2. **transcript** — a fresh vendor session handed Crowbar's own ledger
   (`AssembleConversation(resuming=false)`) as its context. Always available.
3. **fresh** — nothing to continue (a new chat).

"Resume latest in cwd" (`--continue`, `resume --last`) is deliberately **not**
a rung: several chats share one worktree, so "latest in cwd" is another chat's
conversation — the chat-theft class. The ladder is monotonic: a launch that
fails on rung *n* (the process exits before any hook, or the api `resume` call
is refused) is retried once on rung *n+1* with the same pending prompt, and the
failing id is quarantined for the chat so no later path picks it again.

### 2.3 One channel per surface

A runner is **one process with one channel**: an api-channel runner is its
`serve` process (no PTY); a hooks-channel runner is its PTY. Hook wiring moves
out of `config_injection` into `hooks_injection`, applied only to PTY plans.
Consequences, all deleted: `owner:` (descriptor key and spec), 
`ownerDropsThisDelivery`, `HasDispatchedOverAPI`/`dispatchedOverAPI`, the
companion-PTY commentary, connloss's own turn reconcile (a lost connection
kills the serve process and exits the runner through the one exit path). The
explicit channel marker on the delivery stays — it is a fact about the
delivery, used to pick the channel block.

"Open in terminal" for an api provider keeps the existing idle-only handover
(`SwitchToTerminal`: api connection closed, `attach` PTY forked with hooks) —
still one channel at a time.

### 2.4 Backpressure-safe native transport

- `wsrpc`: responses dispatched inline in the read loop; notifications appended
  to a mailbox (slice + signal, never a blocking channel send); cap 16 384
  frames, overflow closes the connection. Writes carry a 10 s deadline.
- `apidriver`: `translateLoop` forwards through the same non-blocking mailbox.
- Every outbound call (`Send`, `Dispatch`, `EstablishSession`, `InjectAt`) is
  bounded by a per-request timeout (30 s; `turn/interrupt` 10 s, then kill).
- Answer desk: an api ask always gets a reply — the user's verdict, or the
  descriptor's `deny`/`decline` template on expiry or runner exit.

### 2.5 Descriptor validation & conformance (`engine/agents/descriptorcheck`)

Static: schema + semantic rules, each finding `{rule, severity, path, line,
message, hint}`; errors block enabling. Live: sandboxed probes against the real
binary (version, flags, TUI boot to composer/login, app-server `initialize`,
hook round trip, resume acceptance). Scripted: a fake CLI emulator replaying
recorded transcripts through the real hook/api paths. Surfaced as
`crowbar descriptor validate|test`, `GET /v0/providers/:id/check`, boot logs,
and the provider settings row.

## 3. Invariants

- S1 At most one live runner per chat and per (workspace, vendor session).
- S2 A chat is never left with no runner and no actionable status: dormant
  always carries `lastExit` (or is new), and send on dormant revives.
- S3 Every open turn closes (completed / interrupted / failed) within the
  process's lifetime or at its exit.
- S4 Stop returns within a bound regardless of what the vendor does.
- S5 A send to a chat with history always continues the conversation, on some
  rung; the rung is visible.
- S6 A runner has exactly one channel; no event is ingested twice.
- S7 The api read loop never blocks; every outbound call is bounded; every ask
  is answered.
- S8 Deleting a chat removes every per-chat entry (A7), including supervisor
  state; no goroutine outlives its runner.

## 4. What the client stops doing

Deleted from `agent-chat-pane.tsx`: the revive effect and budget
(`attemptedRef`, `reviveInFlightByChatId`, `revivesInFlight`,
`REVIVE_REQUEST_BOUND_MS`, `adopt`-after-resume). The pane renders
`phase`/`session` and posts intents; a dormant chat on the chat surface shows
the composer (send revives), on the terminal surface a "Start session" button.

## 5. Test plan

- Go property test over random interleavings of send / stop / switch / resume /
  crash / hang / conn-drop / daemon-restart / delete against the real usecase
  with a scripted terminal: S1–S5, S8 checked after every step (`-race`).
- Fault injection with the scripted CLI (both descriptors): kill mid-turn,
  hang, slow hook, socket drop, daemon restart mid-turn, unanswered ask,
  missing session file → ladder.
- `wsrpc`/`apidriver`: a consumer that never drains still gets call
  responses; overflow closes; write deadline.
- Descriptor rules: one test per rule id, plus the shipped descriptors pass
  clean.
- Live (this container, fake model server): codex TUI boot → composer, hook
  round trip, full turn, resume by id from another cwd, missing-id exit;
  claude same; codex app-server `initialize` + `thread/start` + `turn/start`.
- Web: reducer/rendering of `phase`+`session`, no revive calls on mount.

## 6. Notes

### 6.1 Deliberate non-changes
- claude's own trust dialog is left to termwait (it lives in the user's
  `~/.claude.json`, which Crowbar must not write). codex's is answered per
  launch with a session-layer `-c`, nothing persisted.

### 6.2 Out of scope here
- Server-side prompt queue. `samePrompt` now matches by request id (P11).
  The busy-barrier recheck and the ledger poll remain: "busy" has sources the
  pushed snapshot does not carry (a pending journal delivery, in-flight turn
  lag, a runner being replaced), and ledger rows are not pushed. Removing
  either needs those facts on the snapshot first.

### 6.3 Offline real-binary harness
Both CLIs accept a custom endpoint: codex via a `config.toml`
`[model_providers.<id>]` with `base_url` and `env_key`, claude via
`ANTHROPIC_BASE_URL` + `ANTHROPIC_API_KEY` (the key pre-approved and
onboarding marked done in its `.claude.json`). The stand-in server
(`api/tests/integration/scripted/livemodel_test.go`) speaks the Responses and
Messages SSE shapes and drives complete real turns with hooks firing. It
backs the scripted suite's live mode (`SCRIPTED_LIVE=1`), not
`crowbar descriptor test --live`: that one runs the CLI as the owner has it
configured, so `--turn` needs real credentials and otherwise stops at the
login screen, which it reports.
