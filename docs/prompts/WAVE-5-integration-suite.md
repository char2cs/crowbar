# WAVE 5 — Integration suite to quiver parity (single agent)

You are building Crowbar's **integration test suite** to the same standard and
structure as `quiver.core`'s — a robust, event-driven, deterministic suite that
exercises the whole backend against real git repos, real PTYs, and real
WebSocket streams. The backend (Waves 0–4) is green; now prove it end-to-end and
lock it down against regressions.

**Study first and mirror exactly:** `/Users/char2cs/Projects/Rabbyte/quiver.core/tests/`
— both `tests/kit/` and `tests/integration/`, plus its `Makefile` test targets
and `.golangci.yml`.

## ⛔ Rabbyte standards — NON-NEGOTIABLE (violating ANY = task failure)

A reviewer checks each one.
1. **Replicate quiver.core's `tests/` structure** — `tests/kit/` (suite + env +
   clients + watchers + fixtures + oracle + bench helpers) and
   `tests/integration/{concern}/` (one package per concern). Mirror it.
2. **One concern per integration package**; **one `_test.go` per source file**
   in the kit (except struct-only files); **source files < 500 LOC**.
3. **One parameter per line, ALWAYS** — signatures AND multi-arg calls; closing
   paren on its own line.
4. **Early returns ALWAYS** — `else` is a smell. **Max 3 indentation levels per
   function** — level 3 must NEVER exist; abstract into kit helpers instead.
5. **No flaky tests, anywhere.** Deterministic every run, every order, every
   machine. **NO `time.Sleep`, EVER** — synchronize ONLY via the kit's
   event-driven WebSocket watchers + channels (mirror quiver's `WaitForState` /
   `WaitForCount` / `WaitFor…`). A condition-poll with a deadline is acceptable
   for state with no WS topic; a bare sleep is not.
6. **Integration benchmarks** for the latency-critical paths (WS push latency,
   snapshot-on-subscribe, git status recompute) with a `baseline.json` +
   regression check, mirroring quiver's `bench` target.
7. **CLEAN**: guard clauses, composition, `fmt.Errorf("op: ctx: %w", err)`,
   gofumpt + goimports. Full statement: `docs/prompts/README.md`.

**Project basics:** module `github.com/char2cs/crowbar/api`; Go 1.26.2; **invoke
`go-style` first**; build tag `//go:build integration` on all kit + integration
files; run sequentially (`-p 1`); reference every `api/docs/specs/v0/` spec for
the behaviors under test.

## Build — `tests/kit/`
Mirror quiver's kit, adapted to Crowbar:
- `suite.go` — `IntegrationSuite` (testify/suite), `Main(m)` (silences logs,
  `gin.TestMode`), `NewEnv()`.
- `env.go` — `BuildEnv()` spins up a **real** server on an ephemeral port with an
  **isolated temp `~/.crowbar`** (`WithHomeDir`) and **temp git repos**; opens the
  WS watchers and exposes `WaitFor…` helpers (NO sleeps):
  `WaitForWorkspaceStatus`, `WaitForDiffStats`, `WaitForConflicts`,
  `WaitForGitStatus`, `WaitForFileEvent`, `WaitForChatStatus`, `WaitForPRBadge`.
- `client.go` + `typed_client.go` — raw HTTP + type-safe API client (projects,
  repos, workspaces, hierarchy ops, files, git read/write, terminal, search,
  review, provider).
- `ws_client.go` — WS dial helpers per topic (`DialWorkspaces`, `DialGit`,
  `DialFiles`, `DialChats`, `DialTerminal`) with channel-fed watchers.
- `repos.go` — fixture **git repo builders** (a repo with branches/worktrees,
  conflicting branches, a repo with a hosted remote stub, dirty trees).
- `oracle.go` — **consistency oracle**: asserts API state ⇄ real git state ⇄ WS
  pushes all agree (e.g. a committed file shows in status, the diff, and the
  workspace `+N/-N`).
- `helpers.go`, `bench.go` (baseline mgmt + regression detection).

## Build — `tests/integration/{concern}/` (one package each)
At minimum: `lifecycle/` (import project → repos → workspace → file edit →
commit), `git/` (status/diff/commit/hunk-stage/stash/reset), `worktree/`
(child create, all 3 merge strategies, re-parent leaf + has_children rejection,
cascade delete skip-locked), `conflicts/` (detect → resolve → operation/continue
& abort), `files/` (tree, content, mutations, watcher fan-out), `terminal/`
(create, multi-attach, ring-buffer replay, resize), `search/` (toggles, gitignore,
cap), `provider/` (gh-backed PR badge, graceful disable), `review/` (threads,
read model), `websocket/` (**snapshot-on-subscribe** for the snapshot topics —
Workspaces/Chats/Git/LSP: connect mid-state, assert the snapshot arrives; Files is
**change-only** (no snapshot); Terminal **replays its ring buffer** — `03` §1a),
`concurrency/` (parallel git ops under the per-repo lock), `crash/` (AgentRun
recovery + chat reconcile on restart).

## Definition of done
- `make test-integration` green, **fully deterministic** (run it 3× and with
  `-shuffle=on` — zero flakes), **zero `time.Sleep`** anywhere in `tests/`.
- `make bench` produces a baseline and passes the regression check.
- The oracle passes for every lifecycle path.
- `make missing-tests` reports nothing uncovered that should be covered.

Report the concern list implemented, the flake-check result (3× + shuffle), and
any spec behavior that was hard to make deterministic.
