# S3: Operations and error discipline. Implementation Plan (index)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Dispatch follows "Status and dispatch" below (refresh 2026-10-28): T9–T11 are done, T1–T6 run in order on one lane, T8 runs beside T1 in its own lane, T7 and T12 close. S3 does not wait for S2 (OR1). Before each task, `grep` every symbol its **Files** block names; when an earlier stage moved one, follow the responsibility, not the path. Paths marked *(S3 Tn)* do not exist at refresh; that task creates them.

**Goal:** Every piece of work that outlives its request is an operation with an id and a terminal state the user sees. Every HTTP error carries one envelope with a code from one closed enum. The client has one `report()`. Both lossy error channels, `pending-creates`, every landing watcher, `reparent-settle` and `bestEffort` are deleted. `_ =`, empty `catch` and swallowed promises reach 0.

**Architecture:** The daemon gains an `operations` table (in `view.db`), a small service that runs detached work, records its outcome and publishes every change on a global `/v0/operations` stream, and a boot pass that fails what a restart interrupted. Each of the 16 routes that answer 202 and keep working afterwards today returns the operation. The client keeps one operations store, fed by that stream and by its own in-flight requests (local operations). Sidebar rows, panes and toasts read outcomes from it. A create is drawn from its operation until the entity it names arrives; nothing waits for a row to land. `domain/problem` holds the closed code enum and the code→status table; `libs.Fail(c, err)` is the only way a handler turns an error into a response.

**Tech Stack:** Go (`api/`, `-tags noEmbed`), gin, gorm + SQLite, gorilla/websocket, asynx (v1.1.0, T8; `go.mod` pins v0.8.1 at refresh), testify, the `api/tests` black-box harness. React 19, zustand, TanStack Router, `@/components/ui/*` (base-ui toast), phosphor icons, vitest + Testing Library (`web/src/__tests__/`), ESLint flat config.

**Spec:** `docs/plans/2026-10-16-crowbar-architecture.md` §0 principle 3 ("Operations are entities"), §3 (Operations in the journal and client entity store), §5 (operation lifecycle diagram), §6 (error-surfacing discipline), §7 row "Branch import/create fails with no reason", §8 bullet "Operations are rows", §11 row S3. The S1 plan (`2026-10-17-s1-chat-ledger-plan.md` and sub-plans; S1 T1–T12 merged) for what exists when S3 starts; the S2 plan (`2026-10-17-s2-chatstore-plan.md`; not started at refresh) for what S2 will move after S3; the S4 plan (refreshed 2026-10-28) for what it takes from S3. `docs/plans/2026-10-17-migration-engine-plan.md` for the boot gate. Executors read the sections a task cites.

## Plan files

| File | Tasks |
|---|---|
| `docs/plans/2026-10-17-s3a-envelope-and-report-plan.md` | T1 one error envelope · T2 `report()`, global reporting, and a migrating or halted daemon is a state, not a drop |
| `docs/plans/2026-10-17-s3b-operations-core-plan.md` | T3 operations: table, service, stream, client store; git verbs move onto it · T4 workspace verbs are operations; the working overlay and `reparent-settle` go |
| `docs/plans/2026-10-17-s3c-creates-and-imports-plan.md` | T5 creates and imports are drawn from operations (the user's bug) · T6 repos and projects are operations; the last lossy channel goes |
| `docs/plans/2026-10-17-s3d-discipline-and-gate-plan.md` | T7 web: no swallowed failure · T8 asynx v1 and durable read models · T9–T11 Go: no discarded error (**done**) · T12 gate |

12 tasks, one commit each. T5 is the task the user's complaint names: "Sometimes Crowbar cannot import/create a branch nor says why it failed."

## Status and dispatch (refresh 2026-10-28, against `bd758724`)

| Task | State | Notes |
|---|---|---|
| T1 | remaining | first on lane A |
| T2 | remaining | after T1 (needs `Problem`, `ApiError`, `lib/api/fetch.ts`) |
| T3 | remaining | after T2 (shares `main.tsx`, `fetch.ts`, `report.ts`); after T8 merged (OR2) |
| T4 | remaining | after T3 |
| T5 | remaining | after T4 |
| T6 | remaining | after T5 |
| T7 | remaining | after T6 (its site list moves under T1, T5 and T6) |
| T8 | remaining, precondition met | lane B, beside T1; merges before T3 starts (OR2) |
| T9 | **absorbed by T11b** | its REDs (`TestScope_ASetenvFailureIsReturned`, `TestStashPush_AMalformedBodyIs400AndAnEmptyOneKeepsTheDefaults`) landed in T11b; `app/shutdown.go` already returns `errors.Join` of the four asynx `Shutdown`s; `handlers/ws.go`'s attach is logged (`4547a851`). Mark done at the S3 final review. |
| T10 | **done** | `ab2f911dd`, `a0f3d0782` |
| T11 | **done** | T11a `6b00bfeec`, `0f75948c9`; T11b `cd0c77a9` (errcheck `check-blank` on, `go.discarded-errors` 0, baseline 0) |
| T12 | remaining | last, after T7 and T8 |

**Lanes.** Lane A: T1 → T2 → T3 → T4 → T5 → T6 → T7 → T12, one at a time (T3–T6 all edit `sidebar-row.tsx`, `drop-actions.ts`, `row-actions.ts`, `lib/api.ts`, `app/container.go`, `api/v0/container.go`). Lane B: T8 now. T8 overlaps T1 only in import lines of files T1 rewrites mechanically (`apperr` → `problem` in `repositories/{chat,node}`, `libs/status.go`'s `ErrPipelineFailed` row), so the controller's min-merge rebase covers it; it overlaps T3 (`app/container.go`) and T4 (`repositories/container.go`, `repositories/workspace/workspace.go`, `reactors/delete.go`) for real, so T8 merges before T3 is dispatched. Nothing else can run in parallel without a shared file. S4 T1 (s4a Task 1, which needs only S1's final review) also splits `sidebar-row.tsx` "in this commit"; whichever of S4 T1 and S3 T3 lands second follows the parts the first created.

**What S3 needs from S2 / S4:** nothing (OR1). S4 T3 owns the T11b tree lookup carry (`tree.go:387`, `home_forest.go:98`, `legacy_rows.go:71`; s4a T3 Step 3).

**What S2 needs from S3:** S2 T5's outbox toasts through `report()` (A2), so S3 T2 lands before S2 T5. S2 T2's refusals (`apperr` conflict / not found in s2a) are written as `problem` codes once T1 has landed, and S2's C27 typed session-lost error is a `problem.Code`. S2 T4 repoints `isEntityPresent`'s chat kind at the ChatStore (A8). T1 edits one S2-owned line: `use-prompt-queue.ts:467-469` reads `error.problem.code === 'chat_busy'` and loses its "no code" regex branch. S2 T0 performs the split of `agent-api.ts` (A9 as amended 2026-10-28) and T5 follows its parts; T5 splits `chat/handlers/chats.go` into `chats_create.go`, `chats_read.go`, `chats_mutate.go` (S2 T11 follows them). `report` (`lib/errors/report.ts`) is not exported at `b79be6f2`: the first task, S3 or S2 T5, that imports it outside its module adds the `export`. Shared files to rebase: the `agent-api.ts` parts (S2 T0; S3 T5; S2 T3–T5), `lib/ws/manager.ts` (S3 T2; S2 T3), the `chat/handlers/chats_*.go` parts (S3 T5–T6; S2 T11).

**What S4 needs from S3:** S4 T2 needs T8 (P1); S4 T3 needs T1; S4 T5 needs T3; S4 T8/T9 need T5; S4 T10 needs T3's `entity-presence.ts`. S4 deletes T3's `operationsDef`, `operations-stream.ts` and `hub.BroadcastOperation/PushOperation` in its T5, so T3 keeps them small and self-contained.

## Carries folded in (from T10, T11a, T11b)

- `internal/app/usecases/internal/provision/provision.go:77`: any `RevParse` failure becomes `localTip = ""`, not only "no local branch". → T5 (import/create path).
- `api/internal/api/v0/endpoints/repos/handlers/repos.go:852`: a struct literal squashed to 131 columns to stay at the file-size baseline. → T6 (it splits `repos.go`).
- `api/internal/core/gateway/transports/socket.go:73`: `errors.Join(ErrDaemonRunning, conn.Close())` is never `==` the sentinel (callers use `errors.Is`, so it works). → T1 (sentinel identity): return the bare sentinel when `Close` succeeds.
- `api/.golangci.yml:110`: a stray blank line before `paths:`. → T12.
- `tree.go:387`, `home_forest.go:98`, `legacy_rows.go:71` (lookup failure as a zero value) → S4 T3, already in its plan.
- T11a: `engine/lsp/internal/server/server.go` is 586 lines (`closeLogged` stayed put); S3 does not edit it.

---

## Global Constraints

Every task's requirements include these. Quoted from the specs, `CLAUDE.md` and the user's standing rules.

- "No feature flags, no parallel old/new code paths, no compatibility shims between stages. Each stage deletes what it replaces in the same change." A task that moves a route onto operations deletes that route's old outcome channel in the same commit, server and client together.
- "Same features as today, fewer lines." Nothing a user can do today is lost. Visible changes are listed under "Decisions that change what the user sees".
- "A fix starts with a failing test that reproduces it. Never fix a symptom with a timer, poll, retry, extra flag, or resync effect. Find the owner of the state." The owner of an outcome is its operation. Every backend bug gets a black-box `TestRegression_*`.
- §6: "A Go error is returned to the caller, attached to an operation, or a bug. No discarded errors, no log-and-return-nil in `service` or `store`; `slog` is for engines with no caller." "One HTTP error envelope with a closed code enum in `domain`; the mapping table lives next to the enum." "The client has one `report(error, context)`." "Empty `catch`, `.catch(() => {})`, floating promises and `bestEffort` are lint errors." (Floating promises: D1.)
- Go: required dependencies are constructor arguments, never nil-checked setters; no `...ForTest` methods or test-only package vars in non-test files; wrap errors `fmt.Errorf("op: ctx: %w", err)`; early returns; funlen 100/50, gocyclo 15, nestif 2; one test file per source file (`foo.go` → `foo_test.go`; CI rejects `*_extra_test.go`, `*_coverage_test.go`, `*_gaps_test.go`); no `time.Sleep` in tests (channels, `require.Eventually`, the harness's stream readers).
- Web: kebab-case files; narrow store selectors (`useXStore((s) => s.field)`), never a bare `useXStore()`; `getState()` only in handlers and effect bodies; stores never import `components/` or any `*.tsx` (toasts fire from components that watch store state); tests in `web/src/__tests__/` mirroring `src`, `@/` imports, extend the unit's test file, ≤3 `vi.mock` per file, no Tailwind-class assertions, no reading `src/` text, no sleeps (`waitFor`, fake timers, resolved promises); phosphor icons, base-ui, dnd-kit, shiki only; preserve every animation ("a snap is a defect").
- Files: every production file a task creates or edits is ≤500 lines when the task commits (R15). Comments explain *why* in ≤3 lines; a task that edits a file brings that file's comments to that rule.
- Lint suppressions name the rule and give a reason on the same line. Never loosen a lint config, coverage floor or budget. Baselines (`api/deadcode-baseline.txt`, `web/knip.json` `ignoreIssues`, `*_BASELINE` in `web/eslint.config.js`, golangci `new-from-rev`, `scripts/ratchet/*-baseline.json`) only shrink: fix an entry, delete it in the same commit; never add one.
- (X2) A task that splits or deletes a file listed in `web/eslint.config.js` `MAX_LINES_BASELINE` (cap 600) deletes its entry in the same commit, or `bun run lint:baselines` fails on the stale entry; likewise `api/deadcode-baseline.txt` for a deleted Go symbol (`scripts/deadcode-check.sh` fails on "no longer dead"). Affected: T1 `lib/api.ts`, T3 `sidebar-row.tsx`, T4 `drop-actions.ts` and the worktree `WaitAsync` line, T5 `row-actions.ts`, `rows-from-repo.ts` (and their tests if they drop under 600; `agent-api.ts` and its test are S2 T0's, A9), T6 the projects/repos `WaitAsync` lines, T7 `use-file-explorer-context-menu.tsx`.
- (X3) Many test files T1–T6 extend already have 3 or more `vi.mock` (`merge-popover` 5, `home-actions` 5, `removal-commit` 5, `row-actions` 5, `thread-create` 4; `branch-section`, `create-actions`, `add-repository-modal` 3). New cases inject `send`/`runOperation` or seed the operations store rather than add a mock; a rewritten file ends at ≤3.
- (X4) Black-box, kit and integration helpers read the working overlay, `LastError` and `runAsync` (`tests/harness_test.go waitForWorkComplete`, `tests/kit/env.go WaitForWorkComplete`/`WaitForWorkspaceLastError`, `tests/fixtures_test.go`, `tests/regressions_test.go`, `tests/integration/{conflicts,worktree,lifecycle}`). T3, T4 and T5 migrate the ones their deletion breaks to `awaitOperation` (and the kit twin `kit.AwaitOperation`) in the same commit; T12's grep only checks. T1, T3, T4, T5 and T6 also run `go vet -tags 'integration noEmbed' ./internal/...`, because the `//go:build integration` files under `internal/api/v0/` are otherwise never compiled.
- Commits stage only the files the task touched, by path (`git status --short`, then `git add <path>…`, `git rm <path>…` for deletions). Another session may hold uncommitted edits in the same directories; a directory-wide `git add` would commit them. Every commit message ends with exactly these two trailer lines (the handoff's form):

```
Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK
```

- Running tests: targeted only, never a full suite. Web: `cd web && bunx vitest run <paths>`; types `cd web && bun tsc --noEmit` (never `bunx tsc`, a different package). Before each web commit: `bunx eslint <touched> --max-warnings 0`, `bunx prettier --check <touched>`, `bun run knip`, `bun run lint:baselines`, `bun run lint:doctor`. Go, always in this form (below, "`GOTEST <pkgs>`"):

```bash
cd api && T=$(mktemp -d) && HOME=$T/home CROWBAR_HOME= go test -tags noEmbed <pkgs>; rm -rf $T
```

  (the handoff's form; `<scratchpad>/tools/env.sh` sets `PATH` when the toolchain was rebuilt). Black box ("`GOBOX <regex>`"): the same prefix with `go test -tags 'integration noEmbed' -count=1 -run '<regex>' ./tests/` (the top-level `./tests/` package only; no `-race` except on the targeted tests; a full run needs `-timeout 40m`). Never run `-tags integration` tests in `api/tests/integration/...` or `api/tests/kit` (the kit touches provider homes); compile them only: `go vet -tags 'integration noEmbed' ./tests/...`. Never `go test ./...`, `make pr-checks`, `make test-integration`, `make test-coverage` or `make meter`. Then, from `api/` ("`GOLINT`"): `golangci-lint run --build-tags noEmbed ./...` (new code), `golangci-lint run --build-tags noEmbed --new-from-rev= ./...` (the backlog, where a task says so), `scripts/deadcode-check.sh`, `go vet -tags noEmbed ./...`. Ratchets from the repo root: `node scripts/ratchet/file-size.mjs`, `node scripts/ratchet/counters.mjs` (`--list <id>` names the sites), `node scripts/ratchet/loc-budgets.mjs`.
- Safety: never connect to `~/.crowbar/crowbar.sock` or open anything under `~/.crowbar/state` (not even `ls`). Never read or write `~/.claude`, `~/.codex` or `~/.claude.json`; run no real provider CLI. Live checks run in the isolated noVNC container the S1/S5/S6 live gates used, with a `CROWBAR_HOME` under the worktree's git-ignored `/.crowbar/`, one dev instance at a time (OR6). Check `df -h "$(git rev-parse --show-toplevel)"` before any build or `bun install`; under 8 GB free, stop and report. Never bare `git stash`. Never push. No PRs. No scratch files inside the worktree; `git add` names paths, never `-A`.
- On-disk changes go through the migration engine's catalog (`api/internal/adapter/migrations`, engine `api/internal/core/migrate`). S3 creates new tables only (R12); it rewrites no existing data.

## Rulings (open decisions resolved for S3)

| # | Decision | Ruling |
|---|---|---|
| R1 | Ratchets per task | As S4 R1: each task checks the 500-line cap with `wc -l` on what it touched, runs `node scripts/ratchet/file-size.mjs` and `node scripts/ratchet/counters.mjs`, and states, in its commit body, the net line change per module key it touched. `loc-budgets` is red at base (S1 ledger R7, awaiting the user; at refresh `go.total` 129700 > 127962, `web.total` 106191 > 105828): report the delta, never raise a budget. A task that moves a responsibility between modules (T1: `app/apperr` and `libs/status.go` → `domain/problem`; T3: detached work from `api` into `app/usecases/operation`) edits those two keys in `scripts/ratchet/loc-budgets-baseline.json` by the moved amount in the same commit, as `loc-budgets.mjs` prescribes; `go.total` and `web.total` are never raised. Lower numbers are written back at merge (controller `chore(ratchet)` commits) and once more in T12 (`node scripts/ratchet/<file>.mjs --update`). |
| R2 | Which work is an operation | Work that continues after the response: the 16 routes that run detached work today (worktree `…/chats/:id/{sync,merge-into-parent,rebase-onto-parent,reparent,retry-provision,detach-holder}` and `…/chats/import-batch`; `/v0/chats/:chatId/git/{push,fetch,pull,merge,rebase}`; `POST`/`DELETE` of projects and of repos; `sync`, `git/fetch`, `git/merge` and `git/rebase` are called only by tests today, and whether they stay is S7's), plus daemon-started jobs (boot resume of a repo or project delete, a workspace purge that gives up). Prompt submission (200 with `PromptSubmissionDTO` since S1) and compact (202 via `WriteMutationOK`) are not operations: their outcome channel is ledger rows (architecture §5 outbox). A handler that finishes its work inside the request answers `204` (`libs.WriteNoContent`), not 202 (T6); after T6 `libs.WriteAccepted` has no caller and is deleted. |
| R3 | Operation id | Client-minted UUID sent as `Idempotency-Key`; the daemon mints one when the header is absent (CLI and MCP callers). Same key with the same kind and subject returns the existing operation in whatever state it is in. Same key with a different kind or subject is 409 `operation_id_conflict`. A malformed key is 400. The known-key check runs before any validation that depends on world state (`Spec.Validate`), so a retry is never refused by what the first attempt created. |
| R4 | Validation | Anything a route can check before 202 stays a synchronous 4xx and creates no operation (today's `CheckRepoImportable`, `branchesMissingFromRemote`, default-branch refusal, body binding). A check whose answer the first attempt changes (an import's "chat id unused") is `Spec.Validate`, which `Start` runs only for a new key. |
| R5 | Terminal states | `accepted → running → succeeded \| failed` (architecture §5). An operation with items fails if any item failed, with code `partial` and a message counting them; each failed item carries its own `Problem`. Shutdown waits for running operations within its deadline, then cancels what is left; a cancelled one is `failed{interrupted}` (a graceful stop does not kill an in-flight push); `Start` after Shutdown has begun is `unavailable`. At boot, every `accepted`/`running` row is set to `failed{interrupted, "Crowbar stopped before this finished"}` before the API opens. |
| R6 | Retention | Operations are kept across restarts. At boot, after R5's pass, settled operations beyond the newest 500 by `updatedAt` are deleted. No timer prunes anything. Dismissing (`DELETE /v0/operations/:id`) sets `dismissed`; it hides the toast and chip, never the entity's last error (R9). |
| R7 | Stream | A dedicated global stream `GET /v0/operations` (WebSocket) on the existing `ws.StreamDef`/`Broadcaster` machinery: on connect a snapshot of every retained operation, then one bare `Operation` JSON frame per change. Frames carry `version` (incremented on each change); the client ignores a frame whose version is not newer. S4 folds this stream into the one stream (D3). |
| R8 | "Succeeded: subject version advances" while entity frames ride other sockets | The daemon publishes an operation's terminal frame only after the work's entity writes returned (projections and hub broadcasts already ran). Sockets are not ordered against each other, so the client never treats "succeeded" alone as "the rows are here": a create's row is drawn from its operation item until the entity with that id is in its store (`isEntityPresent`), and `whenSettled(opId)` resolves only when the operation succeeded and every created item is present (every deleted item absent). This is a join of two server facts, not a watcher: no timeout, and a failure carries its reason. S4's single ordered stream removes the join. |
| R9 | `lastError` | Deleted from the Workspace aggregate, `Repository`, `Project` and every DTO (`WorkspaceDTO`, `ChatWorktreeDTO`, `RepoDTO`, `ProjectDTO`) and the TS types. The client projects it: `selectEntityProblem(state, id)` returns the `Problem` of the newest operation naming `id` (as subject or item) when that operation failed, else null. A later operation on the same entity that succeeds clears it. The Workspace event log keeps old `/lastError` patches; they are stripped at read by the existing upcaster (R13). `Repository.Deleting`/`Project.Deleting` stay: they are entity lifecycle state, not error channels. |
| R10 | Who names a created row | The client mints the ids of the rows it draws while a create is in flight: the chat id of a thread or branch create, and one chat id per branch of an import (`branches: [{name, chatId}]`). The daemon uses them (409 `id_conflict` if taken, refused by the chat aggregate's create command, never by a handler lookup; 400 if not a UUID). A failed import branch keeps the chat its attempt minted as its placeholder's owner, so no id is purged and re-minted. Ancestor branches an import adds from the PR graph keep daemon-minted ids and arrive as items. Repo and project creates do not mint ids: their modal waits on `whenSettled`, as it waits on `awaitEntity` today. |
| R11 | Local operations | A request the client starts that the daemon finishes inside the response (thread create, branch create, single-branch import) is a local operation in the same store: `accepted` when sent, `succeeded` with the reply's items, or `failed` with the reply's `Problem`. A server frame with the same id always replaces a local entry. Local operations live in memory; a reload drops them (the entity, if created, is in the stores by then). |
| R12 | Schema | `operations` is a new gorm table in `view.db` created by `AutoMigrate` at open, as every `view.db` table is. It changes no existing row, so it needs no catalog step. The `last_error` columns of `projects` and `repositories` stay on disk unread (gorm never drops a column); removing them belongs to S8's layout step (amendment A6). |
| R13 | Workspace `LastError` in the event log | `workspace.SchemaVersion` goes 2 → 3 and `StripRetiredPlacementFields` (renamed `StripRetiredFields`) also drops `/lastError` patches, registered as the 2 → 3 upcaster. This is the mechanism v1 → v2 already uses; it reads old events and writes nothing (D6). |
| R14 | Working overlay | `BeginWork`/`EndWork`/`IsWorking` and the `inflight` map in `app/repositories/container.go` are deleted (T4): "this workspace has work in flight" is an operation in `accepted`/`running` naming it. The DTO `working` keeps only the agent half (`WorkingFor`). Rows show `working || selectEntityBusy(id)`. |
| R15 | 500-line cap on files S3 edits | A task that edits a production file over 500 lines splits it by responsibility in that task, each part named for what it exports, so the module does not grow. Measured at refresh (`file-size-baseline.json`): `lib/api.ts` 714 (T1), `components/sidebar/sidebar-row.tsx` 680 (T3), `api/v0/container.go` 875 (T3), `app/container.go` 509 (T3), `git/handlers/write.go` 543 (T3), `drop-actions.ts` 999 (T4), `repositories/workspace/workspace.go` 874 (T4; T8 if it lands first), `repositories/container.go` 658 (T4), `components/sidebar/lib/row-actions.ts` 671 (T5), `rows-from-repo.ts` 702 (T5), `features/agent/api/agent-api.ts` 1381 (split by S2 T0 before T5, A9; T5 edits its `agent-chat-creates.ts` part), `endpoints/chat/handlers/chats.go` 605 (T5), `usecases/workspace/workspace.go` 530 (T5), `usecases/container.go` 1052 (T5, only if `hierarchyOwningChats` changes), `repos/handlers/repos.go` 1051 (T6), `usecases/project/project_import.go` 926 (T6), `project_delete.go` 560 (T6), `repositories/chat/event_store.go` 572 (T8), `use-file-explorer-context-menu.tsx` 610 (T7). An oversize test file is never grown: cases for deleted behaviour go, new cases go in the test file named for the source they exercise (S1 R17). Oversize test files S3 meets: `drop-actions.test.ts` 2584, `sidebar-row.test.tsx` 1114, `row-actions.test.ts` 857, `rows-from-repo.test.ts` 1065, `space-scroller.test.tsx` 1145, `create-actions.test.ts` 525, `thread-create.test.ts` 571, `agent-api.test.ts` 1075, `ide-shell.test.tsx` 581, `repos_extra_test.go` 835, `repos/handlers/create_test.go` 592, `repos_test.go` 588, `project_import_test.go` 1510, `project_import_coverage_test.go` 643, `project_delete_test.go` 503, `repositories/container_test.go` 645, `workspace_test.go` 937, `chat/event_store_test.go` 632, `node/event_store_test.go` 501, `reviewthread_test.go` 566. |
| R16 | Where client code lives | `report()` and the `Problem` mirror in `web/src/lib/errors/` (module `web.core`); the operations store and its helpers in `web/src/lib/store/` ("server-state-adjacent structures", `CLAUDE.md`; module `web.sidebar`, which also loses `pending-creates`). S4/S9 re-home them under `core/`. |
| R17 | What `report()` routes | `report(error, context)` is the toast route; it ignores `AbortError`, Monaco's `Canceled` and `OperationFailedError` (the operations watcher already showed that failure), routes a `daemon_migrating`/`daemon_halted` problem to the daemon-state owners (R18) instead of toasting it (ledger F8), and never throws. Inline errors render `problemOf(error).message`; operation chips and row badges render from the operations store. All three read the same `Problem`, so no failure has two wordings. `installGlobalReporting()` sends every `unhandledrejection` to `report` (D1). |
| R18 | Migrating or halted daemon (coordinator input, from the migration engine review; amended by ledger F1 and R18-accepted) | Halted already has one owner: `useDaemonHaltStore` (`features/window/stores/daemon-halt-store.ts`) rendered by `DaemonHaltGate` in place of every route (`2dea4beab`). Every halt source calls its `halt(message)`: (a) any API response whose envelope code is `daemon_halted`; (b) a health probe that reads `halted`; (c) the desktop supervisor's `daemon:halted` event (wired today). The connection store learns only `migrating` as a state, not a drop. Sources: (a) any API response whose envelope code is `daemon_migrating`; (b) when a channel's socket fails to open, one `GET /v0/health` per failed attempt (deduped while one is in flight), riding `wsManager`'s existing reconnect backoff, so no new timer. Any socket opening clears `migrating`. `ConnectionIndicator` owns the migrating toast: "Crowbar is upgrading its data" with "Step i of n: <name>" from health `detail`. It shows no halted toast (rejected, F1): the gate already says "Crowbar could not start" with the reason. `DaemonHealthListener` keeps the supervisor's `unhealthy`/`restarted`/`dead` toasts and stops reading health itself (its `bestEffort` health read goes). |
| R19 | Go discards that cannot fail usefully | Hash writes (`hash.Hash.Write` never returns an error, and errcheck already excludes it) are written as bare calls, not `_, _ =`. `Close` on a read-only file or a response body is joined into the returned error with `errors.Join`. A deadline set on a connection that is then written returns its error. A cleanup after a failure (`os.Remove(tmp)`) is joined into the failure. A boot or background job with no caller logs with `slog.Warn` and the error; one the user can act on is an operation (R2). `_ = ctx`-style silencers are deleted. T11 turns on errcheck `check-blank`. |
| R20 | Durable read models (T8) | The read-model projections of Workspace, Chat, ReviewThread and Node become asynx durable subscribers (`WithSubscriber`) that return their save error, so a failed save is retried and parks instead of being logged and dropped. Their row delete moves to the `models.ForgetEventName` marker. Hubs stay live `Subscribe` handlers (clients re-snapshot on reconnect). The OCC retry loops on `ErrPipelineFailed` are deleted (v1 serializes same-aggregate commands). Reconcile-on-open stays until S8 merges the databases (it is the upgrade path's guarantee for events committed before T8). Sync projections are not used: they must write through the event store's transaction, and the read models live in other files until S8. |
| R21 | asynx release | S3 owns the bump. Precondition: the user names an asynx release after v1.0.0 that fixes the verification's three Important findings: NF1 (a `Get` racing `Forget` must not resurrect the aggregate through an unlocked auto-snapshot), NF2 (one busy aggregate must not stall the other aggregates of its shard) and NF7 (recovery must be linear per aggregate; `Build` must not take quadratic time on a backlog), and whose release passes those probes. Crowbar carries no workaround for a library defect. **Met:** asynx v1.1.0 was tagged 2026-10-19 (PR #48 merge `e007f6ff3` plus 6 later commits; the gate checks ran on `f7b6fcbbe`/`82e4c38`). T8 pins v1.1.0, re-runs the SQLite `storetest.Run` and `RunSnapshots` against it before relying on it, and makes Forget-marker handling idempotent (upstream's Forget-marker double delivery, 4–14%, reaches Crowbar through `Recover` at startup). Upstream items that do not touch Crowbar's usage (ledger, item 5): no command sends cross-aggregate from `Validate`; Crowbar uses no `ResumeDeliveries`/`PausedAggregates`/sync projections; `QueueDepth` 1000 stays; `ErrQueueFull` maps to 503 with no retry. |

## Decisions that change what the user sees

Ruled so the plan can proceed; the user may overturn any before its task.

| # | Change | Task |
|---|---|---|
| V1 | A failed background job (push, pull, merge into parent, rebase, reparent, retry, detach, import, repo/project add or remove) shows a toast naming the action and the reason, once, when it fails. The row it concerns keeps the reason (icon + tooltip, as placeholders do today) until a later operation on it succeeds. Today most of these fail silently or only in the daemon log. | T3–T6 |
| V2 | An import shows one row per requested branch while it runs. A branch that cannot be imported keeps its row with the reason and a Dismiss button; the others land normally. | T5 |
| V3 | A creating row never times out ("The new row never arrived from the daemon" after 30 s is gone). It spins until the daemon says it succeeded or why it failed. A daemon restart turns it into "Crowbar stopped before this finished". | T5 |
| V4 | A daemon that is upgrading its data shows "Crowbar is upgrading its data · Step 2 of 5: <step>" instead of "Backend unavailable · Reconnecting…". A halted daemon learned from a refusal or a health probe (not only the supervisor event) shows the existing `DaemonHaltGate` "Crowbar could not start" screen with the reason (F1: no halted toast). | T2 |
| V5 | The workspace spinner during a git or hierarchy job comes from the operation, so it also shows after a reload while the job still runs. | T4 |
| V6 | Adding a repository or project waits in its dialog as today, without the 30 s "timed out awaiting entity" failure; a failure shows the daemon's reason (toast) and re-enables the dialog. A repo that was added but whose protected branch could not be set up (today a daemon-log warning only) opens its repo home; the failed branch is toasted once and drawn as a failed row with its reason and Dismiss. | T6 |

## Shared vocabulary

**Go: `api/internal/domain/problem`** (T1). Package `problem` (imports stdlib and `asynx/models` only):

```go
type Code string // closed: every Code is one of the constants below
const (
	InvalidArgument Code = "invalid_argument"   // 400
	Forbidden       Code = "forbidden"          // 403
	NotFound        Code = "not_found"          // 404
	Conflict        Code = "conflict"           // 409
	Locked          Code = "locked"             // 409, as today's ErrLocked
	TooLarge        Code = "too_large"          // 413
	Unprocessable   Code = "unprocessable"      // 422
	FailedDependency Code = "failed_dependency" // 424
	Internal        Code = "internal"           // 500
	BadGateway      Code = "bad_gateway"        // 502
	Unavailable     Code = "unavailable"        // 503
	Timeout         Code = "timeout"            // 504
	// Specific codes: every code a handler emits today with WriteErrCode or a
	// coded refusal (T1 Step 1 lists them by grep), plus:
	WorkAtRisk          Code = "work_at_risk"          // 409
	IDConflict          Code = "id_conflict"           // 409
	OperationIDConflict Code = "operation_id_conflict" // 409
	OperationRunning    Code = "operation_running"     // 409
	DaemonMigrating     Code = "daemon_migrating"      // 503, written by core/bootgate
	DaemonHalted        Code = "daemon_halted"         // 503, written by core/bootgate
	DaemonReportUnencodable Code = "daemon_report_unencodable" // 500, as gate.go:154 writes it
	// Operation-only (never an HTTP status):
	Interrupted Code = "interrupted"
	Partial     Code = "partial"
)
func (c Code) Status() int                         // the table; Interrupted/Partial → 500 if ever written
type Problem struct { Code Code `json:"code"`; Message string `json:"message"` }
type Error struct { Code Code; Message string; Data any; Err error } // Error(), Unwrap()
func New(code Code, message string) *Error          // sentinels: var ErrBranchHeld = problem.New(problem.Conflict, "...")
func Errorf(code Code, format string, args ...any) error // %w wraps
func CodeOf(err error) Code                         // every code in the chain (*Error via errors.As over the whole tree, plus the foreign table: fs, context, asynx models); the one whose category ranks first in status.go's order (NotFound > InvalidArgument > TooLarge > Forbidden > FailedDependency > Unprocessable > Timeout > BadGateway > Unavailable > Conflict); none → Internal
func Of(err error) Problem                          // {CodeOf(err), err.Error()}
func DataOf(err error) any                          // the first *Error in the chain that has Data
var ErrNotFound, ErrLocked, ErrInvalidArgument, ErrUnavailable, ErrConflict,
	ErrUnprocessable, ErrTimeout, ErrBadGateway, ErrFailedDependency *Error // replace app/apperr 1:1
```

`errors.Is` keeps identity semantics (a sentinel matches only itself); code checks use `problem.CodeOf(err) == problem.X`. A sentinel that wraps a category today keeps wrapping its `problem` twin, and a specifically-coded one sets `Err: problem.ErrConflict` (or its category), so every `errors.Is(err, problem.ErrX)` that holds on `apperr` today still holds (s3a T1 Step 5). `domain.WorkAtRisked(list) error` builds the `work_at_risk` error with `Data: {"workAtRisk": list}`.

**Go: operations** (T3).

```go
// api/internal/domain/operation.go
type OperationKind string // closed; the constant name and its value:
// OpWorkspaceSync "workspace.sync"                     OpGitPush "git.push"
// OpWorkspaceMergeIntoParent "workspace.merge_into_parent"  OpGitFetch "git.fetch"
// OpWorkspaceRebaseOntoParent "workspace.rebase_onto_parent" OpGitPull "git.pull"
// OpWorkspaceReparent "workspace.reparent"             OpGitMerge "git.merge"
// OpWorkspaceRetryProvision "workspace.retry_provision" OpGitRebase "git.rebase"
// OpWorkspaceDetachHolder "workspace.detach_holder"    OpBranchImport "branch.import"
// OpWorkspacePurge "workspace.purge"                   OpRepoCreate "repo.create"  OpRepoDelete "repo.delete"
//                                                      OpProjectCreate "project.create"  OpProjectDelete "project.delete"
// Client-only kinds for local operations (R11): "chat.create", "branch.create", "branch.import_one".
type OperationState string // "accepted" | "running" | "succeeded" | "failed"
type EntityKind string     // "project" | "repo" | "workspace" | "chat"
type EntityRef struct { Kind EntityKind `json:"kind"`; ID string `json:"id"` }
type OperationItem struct {
	Ref   EntityRef         `json:"ref"`
	Label string            `json:"label"`
	State OperationState    `json:"state"`
	Error *problem.Problem  `json:"error,omitempty"`
}
type Operation struct {
	ID        string            `json:"id" gorm:"primaryKey"`
	Kind      OperationKind     `json:"kind"`
	Subject   EntityRef         `json:"subject" gorm:"embedded;embeddedPrefix:subject_"`
	ProjectID string            `json:"projectId" gorm:"index"`
	Label     string            `json:"label"`
	State     OperationState    `json:"state"`
	Items     []OperationItem   `json:"items" gorm:"serializer:json"`
	Error     *problem.Problem  `json:"error,omitempty" gorm:"serializer:json"`
	Dismissed bool              `json:"dismissed"`
	Version   int               `json:"version"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt" gorm:"index"`
}

// api/internal/app/usecases/operation (package operation)
type Spec struct { ID string; Kind domain.OperationKind; Subject domain.EntityRef; ProjectID, Label string; Items []domain.OperationItem; Validate func(context.Context) error } // Validate: optional, run only for a new key (R3/R4)
type Progress interface { Item(item domain.OperationItem) } // replaces the item with the same Label (labels are unique per operation), else appends; publishes
type Work func(ctx context.Context, p Progress) error
func New(store store.ScopedStore[domain.Operation, string], publish func(domain.Operation), clock func() time.Time) *Service
func (s *Service) Start(ctx context.Context, spec Spec, work Work) (domain.Operation, error) // R3 idempotency (FindByKey under the service mutex; Save is an upsert); returns the accepted op; work runs detached under safego.RecoverFn
func (s *Service) Run(ctx context.Context, spec Spec, work Work) (domain.Operation, error)   // boot only: as Start, on the caller's goroutine; returns the settled op (T6 resumed deletes, invariant D5)
func (s *Service) Fail(ctx context.Context, spec Spec, cause error) (domain.Operation, error) // records daemon-started work that already failed (T4: a purge that gives up)
func (s *Service) Dismiss(ctx context.Context, id string) (domain.Operation, error)          // NotFound, OperationRunning
func (s *Service) Recover(ctx context.Context) error // R5 boot pass then R6 prune; called once before the API opens, before any boot Run
func (s *Service) Snapshot(ctx context.Context) ([]domain.Operation, error)
func (s *Service) Shutdown(ctx context.Context) error // waits for running work; when ctx ends, cancels the rest and records it failed{interrupted}; later Starts are Unavailable
```

HTTP: `libs.WriteOperation(c, op)` → `202 {"success":true,"data":{"operation":<Operation>}}`. `libs.Fail(c, err)` → `problem.CodeOf(err).Status()`, `{"success":false,"error":<message>,"code":<code>,"data":<DataOf>}`. `WriteErr(c, status, msg)` keeps its signature and always sets `code` (the category code for `status`). `DELETE /v0/operations/:id` → 200 with the operation. Stream `GET /v0/operations` (WebSocket).

**Web** (T1–T3):

```ts
// web/src/lib/errors/problem.ts
export type ProblemCode = 'invalid_argument' | 'forbidden' | 'not_found' | /* …the Go enum, same spelling… */ | 'daemon_unavailable' // the Tauri proxy's own code, client-only
export interface Problem { code: ProblemCode; message: string }
export function problemOf(error: unknown): Problem // ApiError → its problem; Error → {code:'internal', message}; else {internal, String(error)}
// web/src/lib/errors/report.ts
export interface ReportContext { action: string; key?: string }
export function report(error: unknown, context: ReportContext): void
export function installGlobalReporting(target?: Window): () => void
// web/src/lib/errors/operation-failed.ts (T3); report() ignores it, because the operations watcher already showed it
export class OperationFailedError extends Error { readonly problem: Problem; readonly operationId: string }
// web/src/lib/store/operations.ts
export type OperationState = 'accepted' | 'running' | 'succeeded' | 'failed'
export interface EntityRef { kind: 'project' | 'repo' | 'workspace' | 'chat'; id: string }
export interface OperationItem { ref: EntityRef; label: string; state: OperationState; error?: Problem; parentId?: string } // parentId: client-side placement of a local item
export interface Operation { id: string; kind: string; subject: EntityRef; projectId: string; label: string; state: OperationState; items: OperationItem[]; error?: Problem; dismissed: boolean; version: number; createdAt: string; updatedAt: string; local?: true }
export const useOperationsStore // { ops: Record<string, Operation>; latestFor: Record<string, string> /* entity id → newest op id naming it */ }
export function applyOperationFrame(op: Operation): void // upsert by id; ignored unless op.version is newer (a local entry is always replaced); snapshot frames are applied the same way
export function selectEntityProblem(s: OperationsState, entityId: string): Problem | null
export function selectEntityBusy(s: OperationsState, entityId: string): boolean
// web/src/lib/store/run-operation.ts
export type OperationReply = { operation: Operation } | { items: OperationItem[] }
export interface OperationStart { kind: string; subject: EntityRef; projectId: string; label: string; items?: OperationItem[] }
export function runOperation(start: OperationStart, send: (opId: string) => Promise<OperationReply>): Operation
export function whenSettled(opId: string): Promise<Operation> // R8; rejects with OperationFailedError. Awaits presence of the items of the kind the operation creates:
// branch.import, chat.create → chat; branch.create → chat and workspace; repo.create → repo; project.create → project;
// *.delete → its items absent; every other kind → nothing beyond `succeeded`
export function dismissOperation(opId: string): Promise<void>
// web/src/lib/store/entity-presence.ts
export function isEntityPresent(ref: EntityRef): boolean // at S3: every kind from useSidebarStore (projects, repos, repos[].workspaces + defaultWorkspaceId, repos[].chats + home chats); S2 T4 repoints chat
```

## Ownership after S3

| Fact | Owner after S3 | Today (deleted) |
|---|---|---|
| Outcome of background work | `operations` row, its stream frames | `SetLastError` on a workspace, `slog` in project/repo/import handlers, `Deleting`+`LastError` re-broadcast |
| "Work in flight on this workspace" | operation `accepted`/`running` naming it | `BeginWork`/`EndWork` in-memory overlay |
| An entity's last error | `selectEntityProblem` over operations | `LastError` on Workspace, Repository, Project and four DTOs |
| A row that is being created | its operation item, until the entity arrives | `pending-creates` `creating`/`error`, `realId`, `rowIdsAtClick`, `hideRowsForInFlightCreates` |
| "Did my create/reparent/remove land" | `whenSettled(opId)` | `untilLanded`, `waitForRow`, `chatHasLanded`, `forkHasLanded`, `waitForHomeTree`, `waitForHomeChat`, `waitForRootHomeChat`, `waitForImportedBranch`, `importedBranchHasLanded`, `armImportLockWatch`, `watchReparent`, `awaitEntity`, `releaseWhenGone` |
| HTTP status of an error | `problem.Code.Status()` | `libs/status.go`'s union of sentinels from 8+ packages |
| Surfacing a client failure | `report()` / `problemOf()` | 76 `toast.error` call sites with ad-hoc messages, `failCreate`, `bestEffort`, empty catches |
| Why the daemon is not serving | connection store (`migrating`); `useDaemonHaltStore` (halted, already its owner) | migrating read as a dropped connection; health read once in `DaemonHealthListener` |
| The text a sidebar create row shows before the user confirms | `create-draft` store (client UI state) | `pending-creates` `naming` |

## Deletion table (S3 "Deletes" → task)

| Deleted | Task |
|---|---|
| `libs/status.go` (`StatusAndMessage`, `isNotFound`, `isBadRequest`, `isConflict`, `isPlacementConflict`, `isGitConflict`), `libs.WriteErrCode`, `api/internal/app/apperr/`, `PromptErrorCode`/`CatalogErrorCode`, `domain.WorkAtRiskError`, the two "not found" message sniffs (`homeFileError`, `files/handlers/files.go:178`) | T1 |
| `web/src/lib/best-effort.ts` call in `daemon-health-listener.tsx` (its health read); the "Reconnecting…" toast during a migration | T2 |
| `git/handlers/async.go` (`runAsync`, `Shutdown`), `SetLastError` from git handlers | T3 |
| `worktree/handlers/async.go` (`runAsync`, `broadcastLastError`, `WaitAsync`, `Shutdown`); `BeginWork`/`EndWork`/`IsWorking`/`inflight`; `components/sidebar/lib/reparent-settle.ts`; the purge reactor's "deferring to boot sweep" log-and-drop; `tests/harness_test.go waitForWorkComplete`, `kit.WaitForWorkComplete` (→ `awaitOperation`, `kit.AwaitOperation`) | T4 |
| `lib/store/pending-creates.ts`, `components/sidebar/lib/rows-from-pending.ts`, `rowRepoScope`, `panelRowsAtClick`, `failCreate`, `untilLanded`, `ROW_ARRIVAL_TIMEOUT_MS`, every `*HasLanded`/`waitFor*` watcher, `armImportLockWatch`, `IMPORT_LOCK_SETTLE_TIMEOUT_MS`; `placeholder-toast-watcher.tsx`'s `lastError` text (the watcher stays); the import path's purge-and-re-mint of a failed branch's chat; Workspace `LastError`, `set_last_error` command, `SetLastError` on the repository; `WorkspaceDTO.LastError`, `ChatWorktreeDTO.LastError`; `kit.WaitForWorkspaceLastError` | T5 |
| `projects/handlers/async.go`, `repos/handlers/async.go` (`WaitAsync` everywhere), `lib/ws/await-entity.ts`, `releaseWhenGone`, `Repository.LastError`, `Project.LastError`, `RepoDTO.LastError`, `ProjectDTO.LastError`, `endpoints/internal/detached/`, `ShutdownDetached`, `libs.WriteAccepted` (every remaining caller finishes in the request; prompt submission and compact no longer use it), `createRequest.ID` (ignored today), `provisionProtectedWorktrees`' warn-and-continue | T6 |
| `web/src/lib/best-effort.ts` (whole file and its test); every empty `catch` and `.catch(() => …)` swallow | T7 |
| OCC retry loops on `ErrPipelineFailed` (chat, workspace, reviewthread, node repositories), `saveWithRetry`/`isTransientIOError`, `WithPublishErrorHandler` log-and-drop on read models, the adapter's unused `Count` | T8 |
| Every `_ =` / `_, _ =` in non-test Go | T9–T11 (**done**: T10, T11a, T11b) |

## Module keys and counters

- New Go code: `api/internal/domain/problem/` and `domain/operation.go` (`go.domain`), `api/internal/app/usecases/operation/` (`go.service`), the operations stream def in `api/internal/api/v0/` (`go.http`; the stream def lives beside `projectsDef`), the `Broadcast/PushOperation` pair in `api/internal/app/hub` (`go.stream`). Shrinks: `go.http` (status.go, four `async.go`, coded-error helpers), `go.app-misc` (`app/apperr`), `go.store` (OCC loops, overlay, `set_last_error`), `go.dto` (four `LastError` fields).
- New web code: `web/src/lib/errors/` (`web.core`), `web/src/lib/store/{operations,run-operation,entity-presence,create-draft}.ts` and `components/sidebar/lib/rows-from-operations.ts` (`web.sidebar`). Shrinks: `web.sidebar` (pending-creates, rows-from-pending, reparent-settle, landing watchers, placeholder toast watcher, removal watcher), `web.core` (best-effort, await-entity, the error half of `lib/api.ts`).
- Counters at the end of S3: `go.discarded-errors` 0, `web.empty-catch` 0, `web.swallowed-promise` 0. `go.timers` and `web.timers` fall (the 30 s watcher timeouts, the purge reactor backoff, the OCC jitter); T12 writes whatever they measure.

## Gate baseline (re-measured 2026-10-28 at `bd758724`; S1 T1–T12 merged, S2 not started)

| Counter / budget | Baseline file | Measured now | After S3 |
|---|---|---|---|
| `go.discarded-errors` | 0 | 0 (T10, T11a, T11b; errcheck `check-blank` keeps it there) | 0 |
| `web.empty-catch` | 33 | 33 (7 in S2-owned files: `use-prompt-queue.ts:275,460`, `prompt-queue-persistence.ts:178,206`, `use-workspace-agent-chats-stream.ts:156,197`, `agent-chats-slice.ts:39`; OR3) | 0 |
| `web.swallowed-promise` | 27 | 27 (S2-owned: `use-prompt-queue.ts:374`, `agent-chat-pane.tsx:478`; T6-owned: `removal-commit.ts:280`, `await-entity.ts:148`) | 0 |
| `go.timers` / `web.timers` | 21 / 84 | 21 / 84 (S2 T13 takes `web.timers` to ≤ 72) | lower; written by T12 |
| LOC `go.total` / `web.total` | 127962 / 105828 | 129700 / 106191 (red at base, S1 R7) | not raised by S3 |

Lists come from `node scripts/ratchet/counters.mjs --list <id>`; each sweep task re-runs it, because S2 and the earlier S3 tasks change them.

## Amendments to earlier plans

The earlier plan files are not edited; these notes override them where they meet S3.

- A1 (S2 R9): "Sidebar and tabs read the entity's server-folded `working`." After T4 that field is the agent half only; a workspace row reads `working || selectEntityBusy(id)`.
- A2 (S2 outbox, `s2c` T5): the prompt submission 202 is not an operation (R2) and keeps S2's contract. `report()` (T2) is the outbox's toast route for a send that fails before its `user_message` row.
- A3 (S2 T13): its `web.timers ≤ 72` check stands (S2 index, module keys; aligned 2026-10-28 from ≤ 75); T12 lowers the baseline further.
- A4 (S5 `2026-10-17-s5-editor-host-plan.md`): S5 landed before S3 T1, so its codes live in `api/internal/api/v0/endpoints/documents/handlers.go:21,23` (`HostDocumentRequiresLocalTransport = "host_document_requires_local_transport"`, `DaemonStateDocument = "daemon_state_document"`, written with `libs.WriteErrCode` from `apperr.ErrHostDocumentNeedsLocalTransport`/`ErrDaemonStateDocument`). T1 moves both into `domain/problem/code.go` with the status they are written with today, re-declares the two sentinels with `problem.New`, and deletes `documentError`'s switch; "S3's `report()`" is `report` from `@/lib/errors/report`.
- A5 (migration engine): `core/bootgate` keeps writing its refusal codes itself (bootgate stays dependency-free; other `core` packages do import `domain`); T1 adds `TestBootReport_RefusalCodesAreProblemCodes` in `api/internal/bootreport_test.go`, the one place that imports both without a test-only dependency, so the spellings cannot drift, and T2 makes the client treat them as states (R18).
- A6 (S8, not yet planned): the unread `last_error` columns of `projects`/`repositories`, the R13 upcaster, the read-model reconcile-on-open (R20) and the operations table's home all move with S8's database merge.
- A7 (S4, planned and refreshed 2026-10-28; its A3/A4): the `/v0/operations` stream joins the one stream in S4 T5; R8's presence join becomes ordering in S4 T10; `operations.ts`/`run-operation.ts` move to `web/src/core/entities/` and `entity-presence.ts` is deleted in S4 T10; `rows-from-operations.ts` and `create-draft.ts` fold into the Tree UI in S4 T8/T9; rows drawn from operation items take their placement from the Tree in S4 T12.
- A8 (S2, not started at refresh): S3 runs before S2 (OR1). S2 T4 repoints `isEntityPresent`'s chat kind from the workspace store registry's agent-chats slice to the ChatStore; S2 T2/T5 and C27 write `problem` codes, not `apperr`; S2 T5's toast route is `report()`.
- A9 (S2 T0, `s2b` Task 0; amended 2026-10-28 by the S2 pre-flight F3 ruling): S2 T0 performs the split of `features/agent/api/agent-api.ts` now, moves only, with A9's part names: `agent-chat-reads.ts` (chat reads), `agent-chat-commands.ts` (chat commands), `agent-chat-creates.ts` (chat creates: `createChat`, `createChatWithOwnWorktree`), `agent-providers-api.ts` (providers/settings), plus `agent-chat-activity.ts` (the activity read and choice types), because chat reads with the activity read measure ~760 lines and four parts cannot all be under 500 by moves alone. `agent-api.ts` and its `MAX_LINES_BASELINE` entry go; `agent-api.test.ts` splits along the same parts. S3 T5 follows those parts: it edits `agent-chat-creates.ts` (R15 is then already met) and does not split. If S3 T5 runs before S2 T0 has landed, T5 performs exactly this split with these names and S2 T0 skips the file.

## Review Focus

The five failure modes most likely to bite a user that no task's main tests exercise, each pinned by a test in its owning task:

1. The same import clicked twice, or retried after a dropped response: the daemon must answer with the first operation, never start a second import (T3 `TestService_StartIsIdempotentOnTheKey`; T5 `TestRegression_BranchImportRetryReturnsTheSameOperation`).
2. A daemon restart while an import runs: the row must turn into "Crowbar stopped before this finished", not spin (T3 `TestService_RecoverFailsWhatARestartInterrupted`; T5 web `rows-from-operations` case "an interrupted import row says so").
3. The operations socket reconnects in the middle of a create: the snapshot must not drop a local operation whose request is still in flight, and an older frame must not roll a settled operation back (T3 `operations.test.ts` "a reconnect never drops a local operation still in flight" and "an older frame never replaces a newer one").
4. A failed create whose entity still arrives (the daemon made a placeholder): the row must show the entity with its reason, once, not the entity plus an error row (T5 `rows-from-operations` case "a failed item whose entity exists draws only the entity").
5. A migration that ends while the window is open: the "upgrading" toast must give way to "Reconnected" when the first socket opens, and a refusal arriving after that must not resurrect it (T2 `connection-store.test.ts` "an open socket clears the daemon state").

## Spec issues found while planning

1. §6 says `report()` routes to "a toast, an inline row badge or an operation chip". A function cannot render a badge; R17 splits it: `report` toasts, the other two render the same `Problem` from the store.
2. §5 "running → succeeded: subject version advances" assumes one ordered stream, which is S4's. R8 is the S3 bridge.
3. §8 "`lastError` on an entity is a projection of its latest failed operation" is implemented on the client (R9). A daemon-side projection would put the same fact on two sockets.
4. §6 lists floating promises as a lint error; the web ESLint config is not type-aware, and `no-floating-promises` needs type information (D1).
5. §11's S3 gate "`_ =` → 0" counts `api/tests/kit` (non-test files), which this machine may compile but never run under the integration tag. T11 changed the kit compile-only (done).
6. The asynx v1.0.0 verification (2026-10-18) found NF1, NF2 and NF7 (R21). Resolved: v1.1.0 fixes them (R21 "Met").

## Open decisions for the user

All six were **accepted by the user on 2026-10-18** ("Agreed with everything"), D1 as amended by F8 (R17). They stay here for their reasoning.

- **D1. Floating promises.** Recommendation: `installGlobalReporting()` reports every unhandled rejection now (T2), and the type-aware `@typescript-eslint/no-floating-promises` rule waits for S9's lint pass. Cost if wrong: S3 would also have to switch ESLint to typed linting (`parserOptions.projectService`), which slows every lint run and touches every `void promise` site; doing it in S9 costs nothing extra.
- **D2. A generic Retry on failed operations.** Recommendation: no; the existing per-entity actions stay (Retry provisioning, Detach holder, re-run the git action). Cost if wrong: a Retry button later is additive (the operation already records kind and subject).
- **D3. A dedicated `/v0/operations` stream** rather than adding operation frames to the projects stream, whose reader treats any object with a string `id` as a project. Recommendation: dedicated. Cost if wrong: none lasting; S4 merges both.
- **D4. Retention of 500 settled operations across restarts.** Recommendation: as ruled (R6). Cost if wrong: one constant.
- **D5. A branch create opens its chat once the branch exists** (`whenSettled`), as today, rather than opening at once on a "Creating…" pane. Recommendation: keep today's behaviour. Cost if wrong: opening at once needs the IDE route and panes to render an operation for an id that has no workspace yet, about one task.
- **D6. Workspace `lastError` leaves the event log through the existing upcaster** (R13) rather than a catalog step that rewrites `workspace.db`. Recommendation: upcaster; it writes nothing and has precedent. Cost if wrong: S8 adds a catalog step that compacts the stripped patches.

## Open rulings (refresh 2026-10-28)

Each needs the controller's or the user's ruling before the task it names is dispatched.

- **OR1. S3 does not wait for S2.** The plan assumed S1 and S2 were both committed; S2 has not started. Nothing in T1–T6 or T8 needs an S2 artefact (T3's `isEntityPresent` reads today's chat store; A8). Recommendation: run S3 now and let S2 build on `problem`, `report()` and the operations store (A2, A8). Cost if wrong: S2 rebases over S3 in `agent-api.ts`, `lib/ws/manager.ts` and `chat/handlers/chats.go`, and S2 T4 repoints one presence reader.
- **OR2. T8 in lane B beside T1, merged before T3.** T8 is S4's precondition P1 and touches T3/T4's files (`app/container.go`, `repositories/container.go`, `repositories/workspace/workspace.go`, `reactors/delete.go`). Recommendation: dispatch T8 now; hold T3 until it merges. Cost if wrong: if T8 stalls (an upstream defect), T3 waits; then let T3 go first and rebase T8, at the cost of one real conflict in `app/container.go`.
- **OR3. T7 and the S2-owned sites.** Nine counter sites sit in files S2 deletes or rewrites (`use-prompt-queue.ts` ×3, `prompt-queue-persistence.ts` ×2, `use-workspace-agent-chats-stream.ts` ×2, `agent-chats-slice.ts`, `agent-chat-pane.tsx:478`). T7's lint rule cannot land while any site is left. Recommendation: under OR1, T7 treats them by the same rules (≈30 lines that S2 later deletes), so the rule guards S2's new code from the start. Cost if wrong: about 30 lines of throwaway churn; the alternative holds T7 and T12 until S2 T13.
- **OR4. The S3 pre-flight is lost.** The ledger adopts F1–F28 of `preflight.md`, which was not carried to the cloud; only F1, F2, F3 and F8 survive through the ledger's text (applied here: R17, R18, T2, T6, T11). Recommendation: before T1 is dispatched, an opus pre-flight of this refreshed plan regenerates the findings, as S4 OR3; the controller rules on any that contradict the user-accepted decisions. Cost if wrong: the plan ships without up to 24 findings the user already accepted. **Done 2026-10-28:** 62 findings, applied as listed in the "Pre-flight log"; none contradicts D1–D6.
- **OR5. The T8 reference port is gone.** The ledger's port preview and bounded probes lived in the old scratchpad (`<scratchpad>/asynx48/`), wiped before 2026-10-28. Recommendation: T8 ports from v1.1.0's `store/storetest` contract and writes its own Forget-idempotency test; it does not try to recover the preview. Cost if wrong: about 130 adapter lines written again.
- **OR6. Live checks run in the noVNC container.** T2 Step 5, T5 Step 8 and T12 Step 5 said `make dev-desktop` with `mcp__tauri__*`, which needs the user's machine. Recommendation: run them in the isolated noVNC container (isolated `CROWBAR_HOME`, no provider CLI; the S2 R25 fake provider if a chat is needed), as the S1/S5/S6 live gates did; the user re-runs T12 Step 5 on the desktop at the end of the branch. Cost if wrong: a Tauri-only difference (the `daemon:halted` supervisor event) goes unseen until then.

## Refresh log (2026-10-28, against `bd758724`)

Stale items fixed in place across the index and s3a–s3d (about 60): task status (T9 absorbed by T11b, T10 and T11 done, T8's precondition met by asynx v1.1.0 with its two obligations); the "all of S2 committed" assumption (OR1) and a per-task dispatch order with lanes; the macOS Go test command, `make lint`/`make lint-backlog`/`make deadcode`/`make ratchet`/`make dev-desktop` replaced by the handoff's direct commands; disk check path; the commit trailer gains `Claude-Session`; ledger rulings F1 (halted has one owner), F2 (T6's re-add test), F3 (done in T11b) and F8 (`report()` routing) applied; D1–D6 marked accepted; measured counters, LOC budgets and every oversize file a task edits (R15, with sizes); sentinel package paths (`core/safepath` → `engine/fs/safepath`, `engine/terminal` → `core/terminal`, `usecases/internal/repoattachments` → `repositories/chat/attachments`); `apperr` and `StatusAndMessage` counts (62/75 files, 106 sites); S5's document codes already written outside the enum (A4); prompt submission now 200, so `WriteAccepted` goes entirely (R2, T6); line citations that moved (`router.go` detached appends 122/132/190/227, `container.go:155` `Resume`, `ShutdownDetached` sites, `delete.go:220`, `kit/env.go:262,312`); `LastError` writers T5 missed (`commands/{set_lock,sync_working_tree_state,sync_provider_state}.go`, `worktree/handlers/lock.go`'s comment) and `PendingSidebarRow` in `workspace-row-base.ts`; `watchReparent` in `row-actions.ts` (T4); `awaitEntity` in `thread-create.ts` (T5); read-model registration (`RegisterStore` only in the workspace projection; chat/node/reviewthread register through `New`); old store-API test sites (`rebuild_test.go:217`, `chat/internal/store/store_test.go:229,292`; the activity store is gone); T7's site lists re-derived; T11b carries folded in.

Pre-flight rulings (2026-10-28, applied with the pre-flight below; each is the smallest change that keeps the plan's goals):

- Ruling: `CodeOf` ranks the codes in a chain by `status.go`'s category order, not by chain position (A2). Why: the eight multi-`%w` sites order their sentinels arbitrarily, and this keeps every status they answer today with no per-site edit. Cost if wrong: a wrap that means "the outer code wins" needs an explicit coded `*problem.Error`; one function changes.
- Ruling: `ErrSelfParent`, `ErrBranchStillHeld` and `ErrBranchHeldByManagedWorkspace` are Conflict (A4, B1, X1). Why: they are refusals of the same family as the reparent and branch sentinels in `conflictSentinels`; today they answer 500. Cost if wrong: one constant each, and T4's assertion.
- Ruling: `WorkAtRiskError` becomes the constructor `domain.WorkAtRisked(list)` that keeps the wire shape `data.workAtRisk` (A5). Why: `workAtRiskOf` reads that shape; `DataOf` on the sentinel would be nil. Cost if wrong: none on the wire.
- Ruling: `operations.Shutdown` is the first line of `app.Shutdown`, and it waits before it cancels (B4). Why: that is the point and the `drainCtx` that `ShutdownDetached` uses today (`internal.go:370-371`), before the reactor gate closes, and it keeps the call inside `app`; cancelling at once would kill an in-flight push on every graceful stop. Cost if wrong: one call moves.
- Ruling: `onGiveUp(ctx, wsID, tomb *domain.Workspace, err)` reaches the reactor through `repositories.New`, and is called with `context.WithoutCancel(ctx)` (B6). Why: the give-up fires exactly when `ctx` is done, and the never-observed path has no tomb. Cost if wrong: a signature.
- Ruling: `Spec.Validate`, run by `Start` only for a new key, rather than a `Lookup` the handler calls first (C2). Why: the idempotency order then lives in one place, under the service mutex. Cost if wrong: one optional field.
- Ruling: a failed import branch keeps the chat its attempt minted as its placeholder's owner; nothing is purged and re-minted (C3). Why: no forgotten id is reused, so a redelivered Forget marker (R21) cannot delete a live row, and R10 ("the daemon uses the client's ids") holds. Cost if wrong: the placeholder's owner is the attempt's chat rather than a fresh one; same visible result.
- Ruling: a repo whose protected-branch worktree fails is kept, and the operation is `failed{partial}`; V6 says what the dialog does (C1). Why: the repo is committed before provisioning today (`project_import.go:533-536`); a rollback would be a behaviour change V6 does not list. Cost if wrong: a rollback can be added inside the same work later.
- Ruling: boot resumes deletes with `Service.Run`, which settles before it returns, after `Recover` (C5). Why: it keeps invariant D5 ("finished before anything is served") instead of retiring it. Cost if wrong: one method; the alternative is a recorded retirement of D5.
- Ruling: the unload-path removal at `removal-commit.ts:280` reports through `report()` and carries the visible removal's `Idempotency-Key` (C7). Why: no (F) exception is needed, and a delete sent by both paths is one operation. Cost if wrong: a report that may never render.
- Ruling: `placeholder-toast-watcher.tsx` stays, without its `lastError` text, deduped against failed operations (C8). Why: deleting it loses the toast and the Fix… entry point for placeholders that repo add and boot create, which are not operations ("Same features as today"). Cost if wrong: the watcher lives until S4 folds the Tree.
- Ruling: S3 T5 performs S2 T0's split of `agent-api.ts` (C13, amendment A9). Why: R15 forces T5 to bring it under 500, and two different splits of one file would collide. Cost if wrong: S2 T0 renames parts. **Superseded 2026-10-28** by the S2 pre-flight F3 ruling (A9 as amended): S2 T0 splits it now with A9's names, S3 T5 follows the parts.
- Ruling: the engine's `revParse` runs `rev-parse --verify --quiet` and wraps `git.ErrBranchNotFound` on exit 1 (C16). Why: it gives provision a discriminator without an extra git call. Cost if wrong: a caller that relied on rev-parse's stderr for an absent revision loses that text (it still gets an error).
- Ruling: the review summary and outline background reads (`use-review-files-summary.ts:83`, `use-review-outline.ts:74`) are (F), not (R) (D4). Why: they are polled, keep the last value, and a report would toast on every tick. Cost if wrong: a persistent failure there shows only the pane's existing fallback.
- Ruling: the v1 event store keeps one write connection and adds a read pool, rather than several `BEGIN IMMEDIATE` writers (D9). Why: SQLite admits one writer; this is today's arrangement (`event_store.go:46`, `sqlite.go:56-59`) plus non-blocking reads. Cost if wrong: the T8 lane may choose `_txlock=immediate` instead.
- Ruling: the T7 lint rules go in a dedicated `src/**` object that ignores `src/__tests__/**` (D2). Why: there is no `src/**` object, and the general one lints tests, where three patterns are legitimate. Cost if wrong: three test lines would have to change.
- Ruling: T3 migrates only the `/git/pull` black-box wait; T4 deletes `waitForWorkComplete` and the kit twin (B2). Why: sync, retry and detach still raise the overlay until T4. Cost if wrong: none.
- Ruling: D7, D10, D11 and D15 are not written into T8's contract, because T8 is in flight in its own lane; they are listed at the top of T8 for that lane to rule on.

## Pre-flight log (2026-10-28, against `621d3147`)

Source: the opus pre-flight of the refreshed plan (62 findings, 13 blockers: A1–A4, B1–B2, C1–C3, D1, D6, D9, D12; cross-cutting X1–X5 deduplicate A4=B1, A7/B9/C12, A8/B10/C11, B2/C10/D13/B11, A12=D8). Every finding's evidence was re-checked against the code at `621d3147` before it was applied. Only line numbers had drifted, for example C1's `committed = true` (now `:533`), B2's `conflicts_test.go` wait (`:166`) and D13's `regressions_test.go:1115` (inside the test at `:1114`). D1's selector claim was confirmed in esquery's source: literal attribute matching compares `"" + getPath(node)`.

- Applied (58): X1–X5; A1–A15; B1–B15; C1–C17; D1–D6, D8, D9, D12–D14. D8 is applied as T1's `foreign.go` row, plus a clarification in T8's "Keep" line.
- Held for the T8 lane (4): D7, D10, D11 and D15. They are not applied because T8 is in flight and its contract is changed only by D6 and D9. They are listed in s3d T8.
- Rejected: none.
- Found while verifying (applied): `daemon_report_unencodable` is written with 500, not 503 (`core/bootgate/gate.go:154`). The vocabulary comment and T1 Step 1 are corrected.
- Rulings: the 18 "Ruling:" lines above.
- S2 pre-flight coordination (2026-10-28, S2 index "Pre-flight log"): A9 amended (S2 T0 splits `agent-api.ts` now; T5 follows; a fifth part `agent-chat-activity.ts`), its Ruling marked superseded, s3c T5's first step and baselines line follow it; A3 and the counters table align `web.timers` to ≤ 72; "What S2 needs from S3" names who exports `report`. A1, A2, A8 and the `chats_*.go` split stand; S2 b/c/e now cite them.
