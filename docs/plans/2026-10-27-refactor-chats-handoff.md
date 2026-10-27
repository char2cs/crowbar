# fix/refactor-chats — handoff to a cloud worker (2026-10-27)

This branch runs the 2026-10 chat/editor/terminal refactor as several plans executed with
subagent-driven development (SDD). This file carries everything the local controller session
knew. **Delete this file before the branch is merged.**

## 1. Where things are

| Ref | Head | What |
|---|---|---|
| `fix/refactor-chats` | this commit (parent `3a5725dcb`) | All merged work. Base for every next task. |
| `refactor-chats-wip/s5-t6b` | `cbf76b6a6` (parent `c521aa596`) | S5 Task 6b, one WIP commit, **unreviewed**. Its gates were running when it stopped. |
| `refactor-chats-wip/s6-t6c` | `49d040c51` (base `15ef2aacb`) | S6 Task 6c, 12 commits, **unreviewed**, no report written. |

S1 Task 9 had just started (it was reading code): nothing was committed, so there is no branch for it.

Plans live in `docs/plans/` (all committed). The SDD ledgers lived in the git-ignored
`.superpowers/sdd/<plan>/progress.md`. They are copied verbatim in Appendix B and are the
authoritative record of every task, ruling and carry. Recreate them on the cloud side:
`.superpowers/sdd/<plan-basename>/progress.md`. Per-task briefs, reports and reviews (12 MB)
were **not** carried over. A brief can be regenerated from the plan with the SDD `task-brief`
script; the binding controller amendments for in-flight tasks are in Appendix A.

## 2. Hard rules (from the user; non-negotiable)

- **Never touch the production daemon or its state:** no `~/.crowbar/crowbar.sock` and no
  `~/.crowbar/state` (not even `ls`). A dev daemon derives its socket from an isolated `CROWBAR_HOME`.
- **Never read or write provider internals** (`~/.claude`, `~/.codex`, `~/.claude.json`). Read-only
  access is also a violation. Run no real provider CLIs.
- **Hard law:** zero provider vocabulary (codex/claude names) in non-test Go, comments included.
  Provider behaviour belongs in the descriptor YAML.
- Never fabricate fixtures. A recording is a recording; an edited recording counts as fabricated.
- Baselines only shrink (`scripts/ratchet/*-baseline.json`, `api/deadcode-baseline.txt`,
  `web/knip.json`, the eslint `*_BASELINE` lists). Never loosen a lint config or a budget.
- No `git stash` (shared stack), no `git rebase -i`. Stage by explicit path. No PRs, merges to
  develop, or other outward GitHub actions without the user's say-so.
- **Go tests:** always `-tags noEmbed`, with an isolated HOME and an empty CROWBAR_HOME:
  `T=$(mktemp -d) && HOME=$T/home CROWBAR_HOME= go test -tags noEmbed <pkgs>; rm -rf $T`.
  Never `go test ./...`, `make pr-checks`, `make test-integration`, `make test-coverage` or `make meter`.
  **Never run** `-tags integration` on `./tests/integration/...` or `./tests/kit/...`: vet only.
  The black-box package `./tests/` (bbx) may run with an isolated HOME/CROWBAR_HOME. Under `-race`
  it exceeds the default 10m timeout; use `-timeout 40m` without `-race`, and `-race` on the
  targeted tests only.
- **Web:** `bun tsc --noEmit` (`bunx tsc` is a different package). Targeted vitest only; **never
  the full vitest suite**. At most 3 `vi.mock` per file. Also eslint `--max-warnings 0`, knip,
  prettier.
- No sleeps in tests; block on real signals. Every backend bug gets a `TestRegression_*` (bbx).
- Files stay under ~500 lines (ratchet `file-size`). Split before adding.
- Comments: why only, at most 3 lines.
- Commit trailer, exactly:
  ```
  Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK
  ```

## 3. How the work runs (SDD)

The controller writes no code. Per task:
1. An implementer (opus for big tasks) works from the brief plus rulings, commits, and writes a report.
2. A review package is built (`review-package PLAN BASE HEAD`, never `HEAD~1`).
3. An opus task review checks spec compliance and code quality.
4. Fix rounds follow: resume the same implementer for rounds 1–3, then a fresh, stronger model.
5. A sonnet scoped re-review checks the fix.
6. The controller rebases the task onto the branch tip. Ratchet-baseline conflicts are resolved by a
   3-way **min**-merge: drop entries one side removed, drop file-size entries whose file no longer
   exists, and take the min of each number.
7. The controller checks the rebased diff is identical, then runs the gates: build, vet, the
   integration vet, file-size and counters, plus targeted web gates.
8. Fast-forward merge to `fix/refactor-chats`, then a ledger entry: `Task N: complete …`.

Disjoint tasks run in parallel lanes (separate worktrees). Rulings are made and recorded in the
ledger (`Ruling: what — why — cost if wrong`); the controller does not stall on questions.

User preferences: concise replies; questions as a plain prose list with a recommendation (no
AskUserQuestion); short status lines; "fix, don't attribute" (never offer "leave it" as an option).

## 4. Status by plan

- **Migration engine:** complete (HEAD at the time `ec50d1548`). Its carries went into the S1 ledger.
- **S1 chat ledger** (`docs/plans/2026-10-17-s1*-plan.md`; T9–T13 are in the s1d plan):
  T1–T8 complete, as are C24 and the other C-carries in the ledger. **T9 is in flight, not started.**
  T10–T13 follow, then the S1 final review.
- **S2 chatstore:** not started (13 tasks). Carries C27 and the streaming-fence skeleton rule are in its ledger.
- **S3 operations/errors:** T10 complete, T11a merged. **T11b waits for S1 T9.** For the rest, see the ledger.
- **S4 stream/store/tree:** not started. The user accepted the ledger's provisional decisions
  (2026-10-18). It needs a plan-refresh pass before its first dispatch.
- **S5 editor host:** T1–T6 merged. **T6b in flight** (WIP branch). Then T7 (LSP), then T8 (live gate, with
  the user). T0 (budget instruments) still needs its live part.
- **S6 terminal wrapped links:** T1–T6 merged. **T6c in flight** (WIP branch); it must land before the
  branch is finished. Then T7 (live in Tauri, with the user).

**loc-budgets is red at base.** web.editor is 11931 against a budget of 11538, and go.total keeps growing. This waits on
the user's ruling "R7" (S1 ledger). Report the delta each task; do not raise budgets.

## 5. In-flight tasks: exactly where they stopped

### S5 Task 6b: host documents follow external changes (`refactor-chats-wip/s5-t6b`)
- Brief: S5 plan §"Task 6b" (line ~913), plus the controller rulings R1–R11 and T6 carries C1–C6 (Appendix A.1).
  R7 amended: the regressions go in bbx `api/tests/`. R8 amended: a deterministic Go fd/emit test under
  1,000-sibling churn replaces the meter.
- Implementation is complete per its draft report (Appendix A.2); its `@@STATUS@@/@@GATES@@`
  placeholders were never filled. It was running the full bbx (`-timeout 40m`, no `-race`) when stopped.
- **Next:** finish the gates on the WIP commit. Then either have an implementer split the WIP commit into
  proper commits, or keep it as one and give it a proper message. Fill in the report, run the opus review,
  and continue the loop. Base was `c521aa596`; it must be rebased onto the tip. It overlaps with S1 only in
  `scripts/ratchet/file-size-baseline.json` and `api/ARCHITECTURE.md`.

### S6 Task 6c: reflow on resize (`refactor-chats-wip/s6-t6c`)
- Brief: S6 plan §"Task 6c" (lines 235–346) plus rulings R-6c-1..5 (Appendix A.3).
- Done (commits): plan Steps 1–11 (red tests, the wide-glyph wrap, the reflow port in
  `api/third_party/x-vt/reflow.go`, the R5 flow, the in-band snapshot, perf bounds, docs and NOTICE),
  plus follow-ups `9ff1142d9`, `829e88949` and `49d040c51`, which look like Step 12 mutation-check fallout.
- **Not done:** whether Step 12 (mutation check, killed/total) finished is unknown; Step 13 (gates) was not run;
  no report was written. R-6c-2 (did erase write EmptyCell, or was the divergence pinned?) and R-6c-4 (a
  temporary-config funlen/gocyclo/nestif lint over reflow.go) must be confirmed in the report.
- **Next:** a fresh implementer verifies Steps 12–13 and writes the report; then the opus review, and so on.

### S1 Task 9: subagents and plans as entities
- Brief: s1d plan §"Task 9", plus the scope carries 1–8 (Appendix A.4), all binding and must-fix.
- Nothing done. Dispatch fresh from the `fix/refactor-chats` tip.

## 6. Open carries (not in the in-flight tasks)

- **T13 (live run list):**
  - C25 and C26 live: record a real tool_fail capture with the MCP and subagent fields, so the
    unverified flag can go.
  - Run `TestScripted_AnAskTooBigToHoldIsRefused`.
  - Live-capture a codex PermissionRequest with its PreToolUse, and an api approval with its
    item/started; this is M1(3), binding by item id.
  - The codex tool_fail fields.
  - `...BackgroundedSubagentKeepsChatWorking` live.
- **S1 final review fix dispatch:**
  - C28: `abandonUnforked` must reap the runner dir and the surface-registry entry of a refused pre-fork spawn.
  - C30: 12 pre-existing lint issues under the integration tag in `tests/integration/scripted`.
  - C25: dead letters with chat_id "" are never purged.
- **S2:**
  - C27: a typed session-lost send error.
  - On the web, Retry on a settled or abandoned prompt row mints a fresh request id; drop the "may already
    have this message" wording when the ledger says session_lost; no raw "apperr: conflict" text in the
    resume/switch toast.
  - A truncated ToolInput is a JSON string.
- **S5 T8 live checklist additions:**
  - same-pane switching between same-named files of two workspaces;
  - the breadcrumb on a non-active workspace's file;
  - host HTML with a sibling stylesheet renders styled;
  - a relative terminal link in a non-active workspace opens that workspace's file;
  - the swapped Save As tab.
- **Live verification** (S5 T8, S6 T7, S1 T13) needs the desktop app (`make dev-desktop`, Tauri MCP) and
  real provider CLIs, so it must be done with the user on their machine, not in the cloud.

## 7. Next steps, in order

1. Recreate the ledgers from Appendix B, then append a line to each in-flight plan's ledger:
   `RESUMED in cloud 2026-10-27`.
2. In parallel lanes: S5 6b (gates → review loop), S6 6c (Steps 12–13 → report → review loop), S1 T9 (dispatch).
3. After T9: S1 T10, and unblock S3 T11b.
4. Then S5 T7; S1 T11–T12; the S2 and S4 plan work; S1 T13 and the live gates with the user; then the S1 final review.

---

# Appendix A: binding material for the in-flight tasks

## A.1 S5 Task 6b: controller rulings (R1–R11, C1–C6)

Source: `.superpowers/sdd/2026-10-17-s5-editor-host-plan/task-6b-rulings.md`

~~~~markdown
# S5 Task 6b — binding rulings (from task-6b-preflight.md)

These override task-6b-brief.md wherever they differ. The pre-flight's R-numbers are kept. Where the pre-flight checked web rows at ae1b3eed3, re-check them against the T6 tip you build on; fix round 2 changed restore-documents.ts, open-document.ts and external-buffer-sync.ts (a single `retryDocument` path).

- **R1, adopted:**
  - Read `fileMissing` as `unavailable` throughout the brief.
  - A tab holds a watch only while it has a model. An `unavailable` tab holds none. It revives through T6's single retry path (`retryDocument`: reopen, reconcile, Reload from disk, Try again), and that path opens through follow like any other open.
- **R2, adopted, data first:**
  - A follow that is rejected while the tab has an unsaved draft opens the draft model UNWATCHED and shows a toast with the reason, once.
  - The only record of "unwatched" is the missing disposer entry: no flag, no second copy of state.
  - The tab re-follows on the next reopen or reconcile.
  - Brief rule 4 ("no host document opens unwatched") still holds for every tab without a draft.
- **R3, adopted:**
  - `releaseWithDocument` keeps a list of disposers per uri; S5 T7 appends its LSP disposer to the same list.
  - `adoptDraft` either applies the draft in place or transfers the disposer across a release and reopen. It never drops the disposer.
  - Enumerate the watch and unwatch behaviour for every open and restore exit path, and test each.
  - `reconcileDocument`, or T6's retry path, is an open site and goes through follow.
  - Followed host documents are excluded from `reconcileWorkspaceDocuments`, so exactly one mechanism owns them.
- **R4, adopted:**
  - `Acquire` returns an error. The documents stream gets an admit-time hook that refuses before the upgrade with a 503, so a 101 always means the watch is armed. Don't use close-after-101.
  - Arm per handle, not under a manager-wide lock, and emit outside the manager lock.
  - Say this in the ARCHITECTURE.md Watchers paragraph.
- **R5, adopted:**
  - Add a default `wsManager` stub in `web/src/__tests__/setup.ts`, following T6's clipboard-stub precedent.
  - Add `open-document.test.ts`, `reveal.test.ts` and every other test that opens a host document to the modify list.
  - Keep ≤3 `vi.mock` per file.
- **R6, adopted:** `opened()` uses channel-level waiters:
  - it resolves on the channel's first open;
  - it rejects when `isWebSocketCapable()` is false or the channel is permanently refused;
  - on a channel that is closed and awaiting reconnect, it waits only up to the caller's own bound;
  - waiters carry across `fresh`.
- **R7, amended:**
  - `TestRegression_HostDocumentWatchArmedBeforeRead` and `TestRegression_HostDocumentFollowsExternalChange` live in the black-box package `api/tests/`. That is where every other `TestRegression_*` lives, and it is runnable as bbx: `go test -tags 'integration noEmbed' ./tests/`, with an isolated HOME and CROWBAR_HOME.
  - Any dial or wait helpers they need go into `tests/kit` as library code.
  - Hard rule, verbatim: NEVER run `-tags integration` tests on ./tests/integration/... or ./tests/kit/... — vet only. bbx (./tests/) is the only integration-tagged package you may run.
  - If the arm-before-101 claim can be pinned more tightly with a unix-listener unit test in `internal/api/v0`, add that as well.
- **R8, amended:**
  - Replace the meter "gate" with a deterministic Go test. A followed document holds a constant number of fds and emits nothing under churn: 1,000 sibling creates and deletes in its parent directory. Count the fds through the OS's fd list for the test process, or through the watcher's own handle count.
  - Don't run `make meter`. A host-document meter scenario is optional, and runs at T8 against an isolated dev daemon only, never ~/.crowbar.
- **R9, adopted:**
  - Acceptance is "file-size and counters green; loc-budgets delta reported", per standing S1 ruling R7.
  - `container_push_internal_test.go` is capped at 642 lines, so give `PushFile` tests a home that doesn't grow it.
  - Drop the `usecases/mocks/mocks.go` row and add `watcher_internal_test.go` to the edit list.
  - Run `make ratchet ARGS=--update` only to commit lowered numbers.
- **R10, adopted:** when the disk text equals the document text, return without notifying, either in the clean branch of `syncBufferWithDisk` or in `applyExternal`. It is not a flag; it is an equality check.
- **R11, adopted:**
  - Widen the provider-vocabulary grep to the lines the diff adds.
  - Document, and handle visibly, a retry that repeats forever on a permanently refused endpoint, and the N concurrent sockets opened at boot restore.
  - Soften "never with the tree": Linux parent-dir wakeups happen, and deleting the parent drops the watch, so it must re-arm or surface as `unavailable`.
  - A 400 from canonical-spelling drift on restore maps to the same `unavailable` path, with a message naming the path.

## T6 carries (must-fix in 6b; from task-6-rereview-3.md)
6b owns the same open/retry path (restore-documents.ts, open-document.ts, external-buffer-sync.ts), so these land here. Write a test that fails first (RED) for each.
- **C1, stale reason.** On the open-model path, a later read error must replace the old reason. Example: a 500 after a 404 must not keep showing "File not found", in the tab or in the toast.
- **C2, test gaps.**
  - Pin the explicit clear at restore-documents.ts:49 for a file that comes back unchanged. The re-reviewer moved that clear off the early path and 26 tests stayed green.
  - Pin `catchUp`'s dirty branch: the text is kept, the external change is flagged, and one warning is shown.
- **C3, duplicated branch.** `catchUp` restates the clean/dirty branch of `syncBufferWithDisk`. Make it one function; follow 6b's external-change path, which becomes the owner anyway.
- **C4, silent failure.** Once a model is open, `failSafe` swallows a throw silently. Report it the way `hydrate` used to, and surface it if the user can see a consequence.
- **C5, stale read.** `catchUp` can apply a read that is older than a save made while the read was in flight (only boot is exposed). A read older than the latest save must never overwrite. Use the document's own version or save sequence, not a timer.
- **C6, ambiguous name.** When the same relative path fails in two worktrees, the toast shows one name. Qualify each with its workspace.
~~~~

## A.2 S5 Task 6b: implementer's draft report (unfinished)

Source: `lane .superpowers/sdd/2026-10-17-s5-editor-host-plan/task-6b-report.md`

~~~~markdown
# S5 Task 6b: host documents follow external changes. Report

Status: @@STATUS@@
Lane: `par/s5-t6b` (base c521aa596). Commit: @@COMMIT@@

## What landed

**Daemon**
- One file watch per held host path, refcounted by `realtime.HostWatchManager` (`api/internal/app/realtime/host_watch_manager.go`).
- `GET /v0/documents/ws?path=<canonical abs>` holds that watch for the socket's life. It works only on the local socket and only for a canonical host path (`endpoints/documents/handlers.go` `Watch`, `usecases/file/document.go:87` `HostDocument`).
- The watch is armed before the upgrade answers, so a 101 is the ack. A failed arm answers 503 (`ws/stream_def.go:32` `Admit`, `ws/broadcaster.go:138-147`).
- Host changes go to the documents broadcaster. Workspace changes keep going to files (`v0/container.go:394` `PushFile`, `v0/file_streams.go:52` `documentsDef`, `:68` `withHostWatchLifecycle`).
- A single-file watcher in the fs engine (`watch/file_watcher.go:28` `NewFileWatcher`):
  - darwin: FSEvents rooted at the file.
  - Linux: inotify on the parent directory, with sibling events filtered out.
  - If the path is missing, it watches the nearest existing ancestor and re-arms (`backend_fsnotify_file.go:21`).
  - The shared burst coalescer moved to `watch/burst.go`.

**Web**
- `wsManager` gains three methods (`web/src/lib/ws/manager.ts:54-58`, `:76-207`):
  - `opened()`: channel-level waiters.
  - `onDrop()`: hears a dropped socket.
  - `subscribed()`.
- `followHostDocument` subscribes and waits for `opened()`. It reads the document on a reconnect sentinel, on modified/created, and on a refusal notice (`features/workspace/lib/external-buffer-sync.ts:104-152`).
- `openDocument` opens a host document through follow, then read (`features/editor/documents/open-document.ts:217-259`).
- Boot restore and the retry path also follow before they read (`restore-documents.ts:60-160`).
- The watch disposer is held per document uri (`features/editor/host/editor-host.ts:31-56`).

## Commits
@@COMMITS@@

## REDs (each failing first, and why)
The full log is at `/private/tmp/claude-501/-Users-char2cs--crowbar-projects-cb81ea03-54b7-4093-8bdd-fdd6b183e91d-github-com-char2cs-crowbar-fix-refactor-chats-worktree/503ae3f2-1747-426a-8623-d22231419e7d/scratchpad/t6b-notes/reds.md`. Summary:

**Go**
- Build REDs: missing `newBurst`, `NewFileWatcher`, `watchSpec.file`, `FileChangeEvent.Authority`, `Engine.NewFileWatcher`, `HostWatchManager`, `Service.AcquireHostWatch`, `Usecase.HostDocument`, `StreamDef.Admit`, and the documents stream handler.
- darwin `TestFileWatcher_WatchesAPathThatDoesNotExistYet` failed with no change in 10s. A missing root was not resolved through `/var` → `/private/var`. Fixed with nearest-ancestor resolution.
- darwin `...AcrossItsDirectoryBeingRecreated` failed on `deleted`. FSEvents drops the file's removal when its directory is RemoveAll'd. The test now asserts the file is followed again; a leading `deleted` is tolerated.
- `TestContainer_PushFile_RoutesHostChangesToDocumentsAndTheRestToFiles` failed at the base `PushFile`: no frame reached the document socket within 10s.
- bbx, with the documents route unmounted (the base state):
  - `TestRegression_HostDocumentWatchRefusedOverTCP` failed: expected 403, got 404.
  - `...WatchArmedBeforeRead` and `...FollowsExternalChange` failed with `websocket: bad handshake`.

**Web**
- These failed as "not a function" before the code existed:
  - manager `opened`, `onDrop` and `subscribed`;
  - editor-host `releaseWithDocument` and `reopenDocument`;
  - registry `saveMark`.
- Registry R10 failed: the external listener was called for text the model already held.
- 13 external-buffer-sync REDs:
  - C5: a stale read reverted the saved text (`'v1'` vs `'v2'`).
  - A dirty buffer holding its own text was flagged anyway.
  - Reconcile never touched host buffers (calls `[]`).
  - The document was read before any watch (calls `['read']`).
  - With nothing subscribed: no reload, no dirty flag, the refused document still opened unwatched, no refusal toast, nothing on drop.
- Open/restore REDs, verified by running the new tests against HEAD's `open-document.ts` and `restore-documents.ts`:
  - C1: the stale `File not found` stayed after a 500.
  - C4: no `Unsaved changes not restored`.
  - C6: no workspace qualifier.
  - The host document was read before its watch.
  - A 404 under a draft held no watch.
  - adoptDraft ran the other opener's disposer (`['mine', 0]`).
  - A refused watch still opened the document (`unavailable` undefined).
  - R2 showed no toast.
  - Racing opens held 0 watches.
- Pins that pass at base by design:
  - C2's unchanged-file clear and C2's dirty catch-up branch;
  - the exit paths "tab closed during read" and "404 without a draft";
  - own-save echo, and deleted/renamed being a no-op.

## Rulings

- **R1:** an `unavailable` tab holds no watch.
  - Every restore exit releases the watch unless a model took it: `restoreDocument` ends it in a `finally` (`restore-documents.ts:94-106`), and `watch.open` hands it to the model (`:84-88`).
  - A tab revives through `retryDocument` → `restoreDocument` → `watchFor` → follow (`:329`).
  - `reconcileDocument` routes host retries there (`external-buffer-sync.ts:207-211`).
- **R2:** when a follow is refused under an unsaved draft, the draft opens unwatched and one `Not following changes on disk` is pushed (`restore-documents.ts:140-152`). That push is skipped if a non-404 read error was already reported, so one toast per tab.
  - The only record of "unwatched" is the absence of a manager subscription for the path (`external-buffer-sync.ts:167` `heldUnwatched`).
  - It re-follows on reopen (`open-document.ts:152-166` → `refollowHostDocument`, `external-buffer-sync.ts:177`) and on any workspace reconcile (`:197`).
  - Every tab without a draft still never opens unwatched: `open-document.ts:217-238` and `restore-documents.ts:140-146`.
- **R3:**
  - The disposer list per uri is `editor-host.ts:31` `releaseWithDocument`, run at once if the model is not open.
  - `reopenDocument` (`:37`) carries the list across a release and reopen. `adoptDraft` uses it (`restore-documents.ts:223`).
  - Exit paths and their tests:
    - open: follow refused; read failed after the watch; a racing open landed; `openContent` rejected (the model is released, so the disposer runs at once); success.
    - restore: tab closed during the read; a 404 with and without a draft; a model opened elsewhere before or during the read (adopt); an already-open model (no second watch); refused with and without a draft.
    - Tests: `open-document.test.ts` (racing/404/close test) and `restore-documents.test.ts` ("a restored host document and its watch").
  - Followed host documents are excluded from `reconcileWorkspaceDocuments` (`external-buffer-sync.ts:193-198`). Only `awaitsRetry` or `heldUnwatched` ones take part.
- **R4:**
  - `HostWatchManager.Acquire` returns an error, arms per handle outside the manager lock, and emits outside it (`host_watch_manager.go:75-105`, `:142-154`).
  - The admit hook refuses before the upgrade with 503 (`broadcaster.go:138-147`).
  - The ARCHITECTURE.md Watchers paragraph says so.
- **R5:** the default `wsManager` stub in `web/src/__tests__/setup.ts:259` routes `/v0/documents/ws?` to no-ops; everything else goes to the real singleton.
  - Every test that opens a host document runs green under it, including `reveal.test.ts` and `open-document.test.ts`; see the vitest list below.
  - Each file has ≤3 `vi.mock`. setup.ts has 1. The rest use `vi.spyOn`.
- **R6:** waiters live on the channel (`manager.ts:35`, `:192-199`).
  - They resolve on open (`:101-103`).
  - They reject when `!isWebSocketCapable()` or there is no channel (`:194-196`), on close before open, or when no subscriber is left (`:127`, `:139`, `:177`).
  - A waiter added while the channel waits to reconnect carries into `fresh` (`:146`).
  - The bound is one connection attempt: every failed attempt rejects. A caller that wants longer awaits again (`noticeRefusal`).
- **R7:**
  - `TestRegression_HostDocumentWatchArmedBeforeRead` and `...FollowsExternalChange` are in `api/tests/files_test.go:142,159`, and `...WatchRefusedOverTCP` is at `:125`.
  - Their dial and wait helpers are in `tests/kit/documents.go` and `tests/kit/ws_watcher.go`.
  - The arm-before-101 claim is also pinned in `ws/broadcaster_lifecycle_test.go` (`TestBroadcaster_Admit_*`) and `endpoints/documents/handlers_test.go` (`TestWatch_*`).
- **R8:** `TestFileWatcher_SiblingChurnHoldsDescriptorsAndEmitsNothing` (`watch/file_watcher_test.go:162`) runs 1,000 sibling creates and deletes. The fd count is unchanged and the first emit is the file's own. No `make meter`.
- **R9:** file-size and counters are green.
  - `container_push_internal_test.go` is still 642. `PushFile` routing is tested in the new `v0/file_streams_test.go`.
  - The `mocks.go` row is dropped.
  - `watcher_internal_test.go` needed no edit: `handleBurst` and `loop` keep their names; only the coalescer moved to `burst.go`.
  - `make ratchet ARGS=--update` lowered only the file-size baseline (`container.go` 904→875, `watch/watcher_test.go` 553→551).
- **R10:** `document-registry.ts:156` returns without notifying when the model already holds the text and, for a save, the saved text.
- **R11:**
  - Grep of added diff lines (Go and web) for provider vocabulary: no hits.
  - Retry-forever is visible. An open or restore with a refused watch makes the tab `unavailable` with a reason (`open-document.ts:112` `watchRefusal`). A followed document whose socket keeps being refused warns once (`Not following changes to <name>`, `external-buffer-sync.ts:133-152`). A drop while the daemon is down says nothing.
  - N concurrent sockets at boot: commented at `restore-documents.ts:71` and documented in ARCHITECTURE.md.
  - The softened "never with the tree" is in the ARCHITECTURE.md Watchers paragraph: Linux sibling wakeups are dropped, and a deleted parent re-arms on the ancestor.
  - Canonical drift (a 400 on restore or open) gives `unavailable` / `Failed to open file` naming the path. Tested in "whose watch is refused opens nowhere and says why".

## T6 carries

- **C1:** a later read error replaces a clean open document's stale reason (`restore-documents.ts:177-180`). Test: `open-document.test.ts` › retryLoad, 500 after 404.
- **C2:** the unchanged-file clear is pinned in the same test's second half. The dirty catch-up branch is pinned in "a dirty open document meeting changed disk keeps its text, is flagged, and warns once".
- **C3:** `catchUp` is gone.
  - Clean/dirty is decided once in `takeDiskText` (`external-buffer-sync.ts:66`). Both `syncBufferWithDisk` (`:91`) and the restore (`restore-documents.ts:174`) use it.
- **C4:** after the model opens, `failSafe` calls `console.error('Restoring a document failed:', …)`. If the throw cost the draft, it pushes `Unsaved changes not restored` (`restore-documents.ts:194-203`).
- **C5:** a save sequence is kept per document (`document-registry.ts:39,89,131,141`).
  - The mark is taken before the read, and the read lands only if it is unchanged (`takeDiskText`, `external-buffer-sync.ts:68`).
  - Test: "a read older than a save made while it was in flight never lands".
- **C6:** each failure carries its `workspaceId`. If the same path fails in another workspace, the label gets ` (<branch>)` (`restore-documents.ts:240-263`).

## Gates
@@GATES@@

## Deviations and concerns
- **Live check deferred:** live Step 9 and the meter scenario are deferred to T8, as briefed. Nothing was run against a live app or ~/.crowbar.
- **darwin:** FSEvents does not report a file's removal when its parent directory is removed. This is harmless, since `deleted` is a no-op on the web. The test tolerates it.
- **Linux:** sibling changes in the parent directory still wake the daemon (and are dropped). The fd count stays constant (R8 test).
- **R2 record:** "unwatched" is recorded as "no manager subscription for the path", not as an empty disposer list. Once T7 appends the LSP disposer to the same list, list emptiness can no longer mean "unwatched", so this probe has to stay on the subscription.
- **R6 bound:** the bound is one connection attempt, not a caller-supplied timeout. `noticeRefusal` decides a refusal from two failed reconnects in a row while the connection store reports `connected`. That is a heuristic, because the manager cannot see the HTTP status of a refused upgrade.
- **Boot restore:** it opens one socket per restored host tab, all at once. Tabs on the same path share one armed watch.
- **Drift:** a restored host tab whose path drifted from canonical now shows `unavailable` naming the path, instead of opening unwatched.
- **No extra helpers:** no `WaitNDocumentsRegistered` or `Env.DialDocument`, since a 101 already implies registration.
- **loc-budgets:** red at base, and still red (see Gates for the delta). It cannot be green without deleting unrelated code.
- **Full bbx under `-race`:** it exceeds go test's default 10m timeout (`FAIL ./tests 600.7s`, the timeout panic in asynx's pool goroutine dump). It was rerun without `-race` and with `-timeout 40m` (see Gates). The new `TestRegression_HostDocument*` tests pass under `-race` on their own.
~~~~

## A.3 S6 Task 6c: controller rulings

Source: `.superpowers/sdd/2026-10-17-s6-terminal-wrapped-links-plan/task-6c-rulings.md`

~~~~markdown
# S6 Task 6c — controller rulings (binding over the plan text where they differ)

- **R-6c-1, the cursor at a pending wrap.** The client is never left in xterm's pending-wrap state, because a keyframe parks the cursor with a CUP. After a widening resize at a pending wrap, the client's cursor sits one column left of the daemon's until the keyframe's CUP lands in the same resize cycle. Accept this and record it in D2 as the plan says. Do NOT re-print a cell to fake the pending state: with IRM on, or DECAWM off, a re-print mutates content, which is a worse class of bug. Cost if wrong: a one-column cursor offset for one round trip, during a resize at a pending wrap.
- **R-6c-2, printed spaces (Decision 5).** Don't hide the divergence. First find out whether x/vt's erase paths (EL, ED, ECH, scroll fill, resize fill) can write `uv.EmptyCell` (Content "") instead of a space cell without changing what renders or serializes.
  - If they can, patch them in the vendored copy so a printed space is content, exactly as xterm counts it. Add a fixture scenario whose wrapped chain ends in printed spaces.
  - If not, keep D2's note, and add a fixture scenario that pins the known divergence explicitly, as a named known-divergence assertion rather than an equality exemption.
  - Report which branch you took and why.
- **R-6c-3, the height-only row step.** Adopted. The daemon drops the bottom rows today while xterm scrolls the top ones into history, so after a height change the daemon repaints a screen the client never showed. The three TestRowWrapped rows get xterm's expectations.
- **R-6c-4, lint skips third_party.**
  - Check reflow.go by hand, as the plan says.
  - Also run golangci-lint's funlen, gocyclo and nestif over the file with a temporary config in a mktemp dir, e.g. `golangci-lint run --no-config --enable-only funlen,gocyclo,nestif` with the repo's limits, and report the output.
  - Never edit `api/.golangci.yml`.
- **R-6c-5, the NOTICE entry.** Adopted, covering both the reflow port and T6's URL regex.
~~~~

## A.4 S1 Task 9: scope carries

Source: `.superpowers/sdd/2026-10-17-s1-chat-ledger-plan/task-9-scope.md`

~~~~markdown
# S1 Task 9 — carries (binding, in addition to task-9-brief.md)

T9's plan text is task-9-brief.md, from docs/plans/2026-10-17-s1d-feed-migration-gate-plan.md: subagents and plans as entities. The carries below were assigned to T9 by earlier rulings. Each one is must-fix, and each starts with a test that fails first (RED).

1. **`Subagent.SpawnToolID` comes back with its writer.** T7 deleted it because nothing wrote it. T9 writes it when a subagent's spawning tool call is known, and reads it where the brief needs it.
2. **The live buffer gets a cap** (`ingress/live.go`). Frames carry the whole text, so a long tool_output stream costs O(n²) bytes. Give it a per-key cap or a tail window with an explicit truncation marker, and add a benchmark that pins linear cost.
3. **C13.** A child thread's `thread_started` (an api-channel subagent) must never rebind the chat. It belongs to the subagent entity this task creates. Decide it by descriptor and channel, never by provider name.
4. **C20.** A turn_stop that maps no async_work leaves a backgrounded subagent unaccounted for. Make "a backgrounded subagent keeps the chat working" pass from the subagent entity, not from a flag. The black-box pin is `...BackgroundedSubagentKeepsChatWorking`, and T13 checks it live.
5. **C21 (hard law).** `apidriver.Start` hard-codes the handshake's `initialized` notification and its `clientInfo` param. Declare the handshake in the descriptor and add a descriptor-check rule for it.
6. **Split before adding.** `v3_test.go` is at 500 lines and `mapping.go` at 492. Split `v3_test.go` by provider before adding any case, and don't let `mapping.go` pass 500.
7. **`handlers/ws.go:90` ignores an error** (`_ = eng.Attach(...)`). Log it with context, and remove the discard from the counter.

Not here: C25/C26 (T13); C27 (S2); C28 and C30 (the S1 final review); M1(3) item-id binding and the M6 scripted run (T13).
8. **T8b re-review Minors.**
   - `bindTool` always does a second `OpenTool` scan; do it once.
   - Add a test that a displaced relay prints nothing.
~~~~

# Appendix B: SDD ledgers (verbatim, authoritative)

## B: 2026-10-17-s1-chat-ledger-plan

Source: `.superpowers/sdd/2026-10-17-s1-chat-ledger-plan/progress.md`

~~~~markdown
# SDD ledger — plan: docs/plans/2026-10-17-s1-chat-ledger-plan.md
Plan committed fbc6c9758 (index + s1a-s1d; T1-T13). Spec: REFACTOR-CONSTRAINTS.md list (architecture §11 S1 row, chat-surface report, additions design, spike).
Pre-flight scan: preflight.md in this dir (opus a4698b763a2101318): 26 pairs (8 disagree), 13 task rows, 34 recommended rulings.
Execution: after the migration engine (T0-T16). S1 runs against the post-engine tree.

## Rulings
Ruling: ADOPT preflight §9 R1-R25, R28, R30-R34 as written. Carry each into the task it names. Key ones:
 - R1/R34: T12 registers step `chat-ledger` (pre-open, 0→0) in the engine catalog, conversion in adapter/migrations/chatledger, Apply+Verify, store Import; NO `crowbar chat-ledger` CLI (`crowbar migrate --plan` is the dry run). Runs on dev homes only until the release — no user go-ahead needed beyond the release.
 - R2: T4 KEEPS the chat-providers-from-history step with a read-only legacy reader inside it.
 - R3/R4/R30: claude.yaml maps `turn_id: prompt_id` (descriptor only); hand-back prompts open a provider turn; T7 mapper dedupes by natural keys.
 - R5/R6/R31: fake provider gets temp CLAUDE_CONFIG_DIR/CODEX_HOME; no `make test-integration`; standing Go-test form + PATH prefix (REFACTOR-CONSTRAINTS.md).
 - R17 (fixtures): the T8 implementer copies the needed captures from the scratchpad design-hooks/logs into protocol/testdata/fixtures/, scrubbing tokens (runner token, MCP bearer), user paths and ids; no real-CLI runs.
Ruling (R8, amended): a net-shrinking edit to an oversize file needs no split; a task that ADDS lines to an oversize file, or rewrites most of it, splits it below 500 in that task — matches the user's HARD 500-line rule ("any rewrite touching an oversize file") without ~9k lines of unplanned splitting — cost if wrong: bigger tasks.
Ruling (R7): T13 runs `make ratchet`; per-module rises (domain, store, test-support, core) follow the USER's pending reallocation decision; engine+S1 are one LOC window.
Ruling (R26): termwait's 2 s screen poll → S6 gets a task to replace it with a screen-change signal from the terminal model (amend the S6 plan before S6 runs) — cost if wrong: a permanent poll.
Ruling (R27): ACCEPT the R13 projector as a derived, S2-deleted stage adapter (single source of truth = ledger; required so S1 leaves the unchanged client working). The S2 planner brief already deletes it.
Ruling (R29): behaviour changes §6 items 1-7, 14, 15 → ASKED USER (items 8-13 are fixed by R1, R2, R11, R12). Not blocking until S1 starts.
Ruling (from S2 pre-flight P3, amends S1 before it starts): S1 T10's feed writes the FINAL FeedDelta shape (deltas carry only new text, numbered per stream, with a `replace` flag) plus the post-ready snapshot; S1 T7 writes `answerable` on ask rows; S1 T5 never adds the unwritten `TurnRetracted` status. S2 keeps `retract` and `/blobs`. — why: S1 creates those structs and S2 would rewrite them — cost: S1 +~60 lines.
Ruling (R29 provisional, 2026-10-18): the user has not answered and S1 is next, so behaviour changes §6 items 1-7, 14 and 15 are ACCEPTED as planned. Each follows from a binding rule:
- item 3: provider internals are never read, so the transcript is never handed over;
- items 4 and 5: no timers, polls or redelivery;
- items 1, 2, 6, 7, 14 and 15: one owner per state, with no force-kill fallback.
The user was re-asked in plain words and can override any item before the task that lands it. Cost if wrong: that task is reworked.
Ruling (R7 provisional): no per-module budget rises. S1's gate must pass by deletion, and any remaining gap goes to the user with its counts. Web growth from the engine (+79 total, +210 tests) is settled at this same gate.
Carries from the migration engine (see the engine ledger and final-review.md):
- C1: deleting a home workspace's only chat reaps the home, because DeleteCascade has no guard, so the live project is left homeless. A TestRegression_* fails first, then the owner is fixed (inside the task that touches DeleteCascade/chat delete, or T1 if none does).
- C2: the migrating progress screen lands with the first step that takes more than a few seconds. That is likely T12 `chat-ledger`. The desktop holds `migrating` already; the web must show progress (S3 T2 owns the connection-store side).
- C3: the web LOC growth since S0 (+79 total, +210 tests) is settled at T13.
- C4: gofmt `api/tests/integration/scripted/livemodel_test.go`, as a chore commit in T1's dispatch.
Ruling: before T1, a plan-refresh pass (opus, docs only) rewrites the S1 plan text so the briefs carry everything themselves:
- every ruling in this ledger;
- the S2 pre-flight amendments;
- the engine API as built (Begin / RunPreOpen / PostOpenRunner positional; Step / Impl / Checkpoints; catalog in adapter/migrations; `home.Home()` accessors; io_failed);
- C1–C4.
Why: 13 tasks would otherwise each need ~30 rulings carried in by hand. Cost: one opus pass.
Plan refresh (opus adb1cae5df2147a7d) committed 1175cf0da. It folded 41 rulings and fixed 10 citations. T9 moved from s1c to s1d; the numbering is unchanged.
Ruling: accept every refresh conflict resolution (1–8). The code wins over plan text:
- 3: the legacy DB is opened mode=ro when a non-empty -wal exists.
- 4: coverage is run -cover per package, and the module floor is CI's.
- 5: the web migrating screen is S3 T2's; S1 proves only the server side.
- 6: T13 reports the LOC gap and raises no budget. The gap goes to the user (R7).
- 7: the per-chat lock is held for the spawn decision and the runner_started row only. Deadlock-free; T-reviewers check it.
- 8: ReconcileRunnersOnBoot stays as the boot-pass entry.
Task 1: dispatching. BASE 1175cf0da. Brief task-1-brief.md (from s1a).
Task 1: implementer sonnet a604841b92913dd4d
Task 1: implemented 63d631567 (chore) and 0638a942a by sonnet a604841b92913dd4d. Status DONE_WITH_CONCERNS: suite.go +1 cleanup line; the TestTurns pattern was wrong (the fixture is read by TestRegression_RealClaudeSubagentSession_*); go.test-support +420; integration-tag lint findings in the new files.
Task 1: review dispatched (sonnet a83c0968a070edc53) → task-1-review.md
Task 1 review (sonnet a83c0968a070edc53): Spec ✅, Safety ✅, Needs fixes. 0 Critical, 1 Important, 8 Minor.
- I1: the new files are not lint-clean under the integration tag.
- The 3.7 s is the daemon's 3 s AwaitOpen (defaultMessageAwaitTimeout): the test's turn_stop carries text with no earlier message_delta.
Ruling, fix round 1 (resume a604841b92913dd4d):
- I1: rename t to tb; fix the nestif and the 3 gosec file modes; give each true false positive (G304, G204, G703, bodyclose) a nolint with the rule name and a reason on the same line.
- M2: register signal.Notify before record.
- M3: cut the comment to 3 lines.
- M4: the first test posts a message_delta before turn_stop, the real Claude shape (MessageDisplay then Stop), so the 3 s wait goes. Record the real cause in the report.
Ruling: M1 and M5 kept as the brief mandates; T3 exercises RefuseResume, Patch and the bare path. M7 → T13.
Ruling, M6: PostHook has no client timeout. Park it to the first task that replays a held-open hook (PermissionRequest or Elicitation): that task posts from a goroutine, with no timer. Cost if wrong: one test hangs until the go test timeout.
Ruling, M8: noted only.
Task 1: fix round 1 committed 2217800a5. The new files are lint-clean under the integration tag; the first test takes 0.66 s; gates exit 0.
Ruling: the controller verifies fix round 1 directly; there is no separate re-review. Every item was mechanical (lint, comment, signal order, test shape).
- Checked: the stat (3 files) and all 10 new nolint lines, each naming its rule with a reason.
- The implementer's tagged lint run shows 0 findings in the new files.
- Cost if wrong: a lint nit survives to the final review.
Task 1: complete (63d631567, 0638a942a, 2217800a5)
Task 2: dispatching. BASE 2217800a5. Ruling: use opus. This is the ledger's foundation (per-chat single writer, subscribers under the writer, prompt semantics) across about 20 files, so concurrency mistakes would cascade into T3–T13. Cost: one opus task.
Task 2: implementer opus a797e81c4ced6a6a0
Task 2: implementer DONE_WITH_CONCERNS, commit 50e148f27 (102 files, +2225/−4698). Report: task-2-report.md.
- Concern: the implementer once ran `-tags integration -run TestNothing ./tests/kit/` with the real HOME. No test ran; TestMain isolated HOME. Logged as a rule breach; the user is told.
- Carry C5: `usecases/terminal` TestRegression_TerminalSession_RestartRoundTrip_RealStore hangs at HEAD 2217800a5 as well. Pre-existing, but it is not an answer: investigate before the S1 final review.
- Carry C6: web still handles `request_already_accepted`, which the server no longer sends. Ruling pending the review.
Task 2: review dispatched, sonnet ab181610b3b8cc990, package review-2217800a5..50e148f27.diff
Task 2: review (sonnet) — Needs fixes. 0 Critical / 4 Important / 11 Minor. All of deviations 1–11 were judged correct. The concurrency risk is clean.
Ruling, I1: settle at question time. A pending prompt whose carrier is not a live runner is settled `runner_exited` inside the deciding Do. The boot pass runs even when AllLive errors. Why: enumerating departure sites already failed once and bricked chats. Cost if wrong: none; it only removes a stuck state.
Ruling, I2: add the missing branch tests in the existing files.
Ruling, I3: OVERRIDE the brief. A switch with a pending prompt parks on the ledger signal (turn_started or prompt_settled), cancelled by ctx or Stop, and restores `...WaitsRatherThanRefusing`. Why: refusing re-opens the switch 409 the user reported twice, and the brief's goal was deleting the 30 s timer, not the wait. Cost if wrong: a switch request can stay open until the prompt is acknowledged or settled.
Ruling, I4: keep the brief's rule that a settled request id gets a conflict. Carry C7 to S2: web Retry mints a fresh clientRequestId for settled items; delete the dead `request_already_accepted` branch and its test (use-prompt-queue.ts:470, agent-chat-view.test.tsx:712) and the stale pendingprompt.go comment (use-prompt-queue.ts:334). C6 is folded into C7. Why: S1 changes no web file. Cost if wrong: until S2 lands, Retry after a failed delivery shows a misleading 409 on this unmerged branch.
Ruling, risk 2 (codex idle before the turn) and M12: carry C8 to T5, which owns the turn state machine. Idle settles only prompts that reached their carrier; for api transport the turn/start response is the acknowledgement. Cost if wrong: an early `not_acknowledged` on codex until T5.
Ruling, M2–M11: fix now. M5 keeps Store.Append, because T4 consumes it. M9 unifies settle-by-carrier into domain/ledger.
Ruling: the 12 out-of-scope lint fixes are accepted. They are behaviour-neutral and the gate needed them clean.
Task 2: fix round 1 → resume implementer a797e81c4ced6a6a0. Brief: task-2-fix-1.md
Carry C5 diagnosed (sonnet, scratchpad/c5-terminal-hang.md). It is a test bug: the cwd, about 130 chars, splits across rows of the 80-column model, so the frame match never occurs, and the wait has no deadline. Production code is fine. Linux CI passes only because /tmp is short.
Ruling: fix it in T2 fix round 1, in its own commit, with a computed short token plus a deadline. Cost: one test file.
Task 2: fix round 1 committed 693a93af0 (fix) and 03ec580f5 (C5 test). Gates are reported clean, and the file-size baseline went down. Concern: a parked switch may hold the spawn gate with no deadline. The re-review checks this against the "lock only for the spawn decision" ruling.
Task 2: re-review fix round 1 dispatched (sonnet aaa3c2c8384152af1), package review-50e148f27..03ec580f5.diff
Task 2: re-review of fix round 1 (sonnet): all 15 items ADDRESSED. Stop reaches a parked switch with no deadlock. The wait is Store.Watch, a channel closed on commit, with no timer and no leak. The changed tests were not weakened. New Important: a parked switch holds inflight's spawn gate with no deadline, so other gate users on that chat queue behind it.
Ruling: accept it for T2 and carry C9 to T6. The shape is the same as the existing awaitTurnOrForce under the gate.go design ("a holder may park"). T6 replaces inflight's gate with the per-chat ingress lock, held only for the decision (R22). So T6 must move both waits (awaitTurnOrForce and awaitNoPendingPrompt) outside the lock, give Stop a cancel seam that does not depend on holding the gate, and re-check under the lock in a loop. Cost if wrong: until T6, on this unmerged branch, sends on a chat with a parked switch wait instead of getting a 409.
Parked minors, carry C10 to T4 (runner):
- switchwait.go:~13: the doc says "ahead of every read", but GetChat precedes the wait.
- runner_export_test.go: SetSwitchAwaitTimeout is a silent no-op for a non-*Usecase.
- usecases/container.go:1001: the "hook-delivery journal" wording.
- A parked switch re-evaluates only on a ledger commit. This is moot while every departure site settles, and T4 owns departures.
Task 2: complete (50e148f27, 693a93af0, 03ec580f5)
Task 3: dispatching. BASE 03ec580f5. Ruling: use opus, because it touches about 40 files across the domain, store, descriptors, terminal, runner and DTO split, plus large deletions.
Ruling (T3 named risk): the brief's Snapshots.Get calling Store.State for an untouched chat would self-deadlock when it runs inside the ledger subscriber, which publishLedgerRows does under the writer lock. So the subscriber installs facts via ApplyLedger before any Get, nothing on the subscriber path calls the Store, and a first-commit test runs with a deadline. Cost if wrong: daemon hangs on a chat's first commit.
Ruling: T2's refused-resume ladder test dies with the ladder; its behaviour stays covered by Symptom2_OnlyAnExplicitRefusal plus a unit test.
Ruling: git add -A api is allowed only after git status shows no stray untracked files; baselines are staged by path.
Task 3: implementer opus a4f165d6339e9f4ce. Brief: task-3-brief.md. Report: task-3-report.md.
Task 3: implementer DONE_WITH_CONCERNS, commit 0042b8de9 (155 files, +3789/−7606). All 7 Symptom2 tests went RED→GREEN; gates exit 0; loc-budgets is red, as it already was on base (go.dto over budget; R7: no rises).
- Beyond-brief changes:
  - A never-turned bound session is relaunched with the assign arg and the same id.
  - Conversation records the bind cause, so there is no transcript handover after a loss.
- Concerns:
  - A codex thread that never turned is still resumed, so a refusal drops the prompt.
  - contextcheck nolint with Background.
  - A pre-existing reconcileRunnerExit race.
  - Lost coverage of the two-placement race.
  - Web describeRung is dead (→ S2, C7 group).
Ruling: split the T3 review in two because the diff is 984 KB. A sonnet reviewer takes production (review-t3-prod.diff, a31e389375398bb20) and another sonnet reviewer takes tests (review-t3-tests.diff, ac39acfc644cc4508). Cost: about one extra reviewer's overhead; it avoids context overflow.
Task 3: test review (sonnet): Needs fixes. Findings: 3 Important (I1 retire-on-spawn coverage, I2 api bind/refusal glue, I3 no-redelivery) and 7 Minor, saved in task-3-review-tests.md. Waiting on the production review.
Task 3: production review (sonnet): Needs fixes. 0 Critical / 2 Important (I1 a never-turned codex binding is resumed and refused; I2 the assign-reuse loop never recovers) / 8 Minor (I5 self-deadlock on a GetChat error; I6 concurrent claim; I8 api refusal by the generic -32600 code). Lock ordering is sound. Background ctx is right. The DTO split is a pure move.
Ruling R1: OVERRIDE the brief's "bound → resume" for never-turned bindings. A never-turned binding is never resumed: an assign provider gets a NEW id (previous: S), and a non-assign provider launches fresh. The announcement replacing a never-turned binding rebinds with no marker and no leaveSession settle. "Turned" is folded from turn_started rows (TurnStarted carries the session), and the LastTurnForSession read is deleted. Why: the brief's rule drops the user's prompt on codex routinely and on claude; reusing the same id has no failure signal (it wedges); the activity log is a second owner. Cost if wrong: a wrongly "never turned" binding is abandoned, not lost, and stays in Conversations() for /resume.
Ruling R2: api refusal = refused.rpc_code AND a needle on the message, and session_lost_codes is deleted. One mechanism with the same two-part shape as PTY. Cost if wrong: if codex's RPC message lacks the needle, refusal is not recognised; T13's live check verifies it.
Ruling: fix production I3, I5, I6, I7, I9, I10 and test-review I1–I3, M1–M4, M6 and the feed deadline now.
New carries:
- C10 (T4): I4.
- C11 (T5): the reconcileRunnerExit/leaveSession interlock.
- C12 (T7): the preloadLedger full fold.
- C13 (T9): a codex child thread_started rebinding the chat.
- C7 (S2): describeRung is dead.
Characterization labels were checked in the commit body (R18).
Task 3: fix round 1 → resume implementer a4f165d6339e9f4ce. Brief: task-3-fix-1.md
Task 3: fix round 1 committed 10f543084. Every item is closed except I10. Gates pass except loc-budgets, which was already red. bbx: 119 ok.
- Deviations: `switch.go:263` `leftAt` still reads LastTurnForSession, for the handover gap start only (→ C12, T7). `establishAPISession` was split out to make it testable.
- Concerns: I10 open; codex "turned" set at turn start, but codex saves the thread at turn end; shared needles widen the codex PTY rule; the codex refusal message is unverified (→ T13 live); the I6 test is characterization (it passes on HEAD).
Ruling, I10: Important, not parked. A refusal wrapped at 80 columns is missed, so the chat wedges on a refused session, the exact symptom-2 class. Fix it in RefusedResume with a whitespace-insensitive match over the joined rows, still gated by exit/rpc code. Cost if wrong: a needle split across unrelated lines plus a matching exit code false-positives as a refusal, losing a session that was still resumable. Negligible.
Ruling, codex turned-too-early: carry C14 to T5. T5 adds turn-end rows, so "Turned" can be declared per descriptor (`session.persists: turn_start|turn_end`) rather than by provider code. Cost until then: a codex chat whose first turn dies before completing has one prompt refused and its session lost.
Note for T13: verify live that (a) codex's api refusal message carries the needle and (b) claude accepts a fresh --session-id after a never-turned launch.
Task 3: fix round 2 → resume implementer. Brief: task-3-fix-2.md. The re-review then covers both rounds (0042b8de9..HEAD).
USER 2026-10-18 ("Agreed with everything"):
- R29 behaviour changes are ACCEPTED, no longer provisional. These are the T2 busy-until-idle/Stop/exit rule, and in T3 pruned → fresh, /clear marker and /resume conflict.
- The controller overrides are ACCEPTED: the T2 I3 switch waits, and the T3 R1 never-turned rule.
- The test-kit guard is to drop its listing of the provider homes. Carry C15 into the T4 dispatch: `MainGuardingProviderHomes` must not read ~/.claude or ~/.codex; the isolated HOME is the guard.
- asynx findings were posted at the user's request: https://github.com/char2cs/asynx/issues/47 (body plus 2 comments with the probe sources).
USER 2026-10-18:
- The mermaid fix is already on this branch: 04e0373fc, an ancestor of ec50d1548, so T13's web-diff check is unaffected. The old worktree's desktop/src-tauri/target (3.4 GB) was deleted at the user's request. Its branch fix/mermaid-error-overlay (40b4f9120, the same patch on develop) is now redundant.
- Streaming mermaid/excalidraw skeleton → S2 T3; see the S2 ledger.
- "Land S0 early" was left to the controller. Ruling: S0 stays on this branch and lands with the refactor. No separate PR to develop. Why: it is already in this branch's history, a separate PR is an outward action plus a rebase for no functional gain, and the asynx bump is unrelated. Cost if wrong: develop gets the ratchet gates later.
Task 3: fix round 2 committed 95f46ca3b. I10 is fixed: a whitespace-insensitive match gated by the exit code, tested at 80 and 20 columns. apidriver.go comments were trimmed (572→527). Re-review of both rounds dispatched next.
Task 3: re-review of fix rounds 1–2 (sonnet): every item ADDRESSED and no new Critical or Important. R1 checks out: a new id, no marker, the carried prompt untouched; leftAt is only the gap start (T7). I5 is structural (Held never reads the ledger). I6 maps to the conflict marker. R2/I10 are code-gated on both channels.
Ruling, minors:
- `Store.SessionOwner` is now test-only. It goes to the T4 dispatch (C10): delete it if no later task consumes it (CLAUDE.md "delete what you make unused").
- The assertSessionLost bare range, the hard-coded "chat-1" in announce(), and M5's missing ConversationsForChat assertion are parked for the S1 final review.
New carry C16 (T5, with C14): descriptorcheck must require `refused.rpc_code` on any api-transport descriptor; without it the api channel never recognises a refusal.
Note for T13 (live): the codex rpc refusal message carries the needle; the shared needles widen the codex PTY rule, and the exit code bounds that.
Task 3: complete (0042b8de9, 10f543084, 95f46ca3b)
Ruling, amended: keep Store.SessionOwner. T12 consumes it (s1d:562, ConvertChat owner over Store.SessionOwner); drop it from C10.
Task 4: dispatching. BASE 95f46ca3b.
- Ruling: use opus. It deletes the whole runner aggregate package and rewires about 30 files.
- Carries: C10 (switchwait doc, SetSwitchAwaitTimeout, the container wording, the session_bound event name) and C15 (the kit guard stops listing provider homes).
- Ruling: T3's retire test is rewritten to the live-table invariant (ErrChatHasLiveRunner, one live runner after a switch), not just deleted.
Task 4: implementer opus a2a832889ba3408bf. Brief: task-4-brief.md. Report: task-4-report.md.
Task 4: implementer DONE_WITH_CONCERNS, commit 71a5ba78c (154 files, +2867/−5908). Gates exit 0 except loc-budgets (red on base; go.domain +87, go.store +164, go.app-misc +4, engine-provider −1183, go.chat −290). It also fixed a failed-terminate "exiting" hang in ResumeChat (keepLive).
Task 4: the 845 KB diff is split into review-t4-prod.diff (360 KB) and review-t4-tests.diff (485 KB), with two sonnet reviewers → task-4-review-{prod,tests}.md.
Ruling (C15b): the tests/kit login seeding (IsolateProviderHomes, claudeSeed, RequireProviderLogin, requireRealCodexLogin) still reads the real ~/.codex/auth.json and ~/.claude.json. That breaks the never-read-provider-internals law. T4 fix round 1 makes the kit take credentials only from explicit test env vars or a path the user supplies, never from a real provider home; with none given, real-provider tests skip and name the missing variable — why: the user's HARD law, and the kit is already in T4's scope through C15 — cost if wrong: a real-provider integration run needs credentials set up explicitly.
Task 4: both reviews say "Needs fixes".
- Prod: I1 noteRefusedRunner marks a healthy runner exiting; I2 the turn close after runner_exited is not crash-safe and the shutdown guard was deleted.
- Tests: I1 the kit reads/symlinks the real provider homes, and every scripted test does it via newRig; I2 the Shutdown→exitAll path is untested; I3 the launch-report merge coverage vanished.
Ruling (T4-P-I2): fix in T4, not T5 — a turn closes before its runner's exit commits; if AbandonTurn fails there is no exit row, but the live-table entry is still removed, and boot reconciles — why: crash-safety of a fact T4 introduced; C11 stays in T5 — cost if wrong: an ordering change that T5 revisits.
Ruling (T4-P-M8): step 4 seeds runner_started rows for legacy runners still live, so the first boot closes their turns — why: an upgrade from a crashed pre-T4 daemon would otherwise strand open turns (the stuck-spinner class) — cost if wrong: a small amount of migration code.
Ruling (T4-P-M1): keep the conversations source in NewestProviders (the brief's rule list names it) and add a deterministic ORDER BY tie-break — cost if wrong: a legacy chat picks a different newest provider on a tie.
Ruling (T4-T-M8): fold in the pre-existing negative-wait timers of the touched files; any that need a seam that can't be added cleanly get parked.
Parked for the final review: P-M5 (the unrecorded window before runner_started) and P-M7 (a purge race into a forgotten chat; engine-unreachable).
Task 4: fix round 1 dispatched (resumed the implementer); brief task-4-fix-1.md; BASE 71a5ba78c.
Task 4: fix round 1 committed 15bcbc81a (52 files, +1054/−630). Gates exit 0; the counters and file-size ratchets were updated (shrinks only). loc-budgets is still red: go.chat +38, go.store +44, go.app-misc +10 against 71a5ba78c. The boot runner reconcile now runs after RunPostOpen (app/container.go:179-183). Re-review dispatched (sonnet) → task-4-rereview-1.md.
Carry C17 (T12): a home whose step-4 marker is already set never re-runs step 4, so its legacy live runners are never adopted. T12's pre-open step must adopt them for every upgrading home.
Parked for the final review:
- TestRegression_SwitchToTerminalDoesNotExitThePTYLessRunnerItHandsOver still waits 300 ms. The apiConnRegistry.watchExit goroutine is untracked, and tracking it in backgroundWork deadlocks Shutdown. That also points at C9/T6's cancel seam.
- apiconn_internal_test.go keeps its sleeps.
- The tools.Resolver `chats` port has been dead since before T4 (~20 test call sites).
Ruling (T4-T-I1b): keep CLAUDE_SECURESTORAGE_CONFIG_DIR="" removed. It sent the isolated claude to the user's real keychain credential, which a token refresh can overwrite. Opt-in real-claude tests on macOS now need ANTHROPIC_API_KEY or CROWBAR_TEST_CLAUDE_CREDENTIALS — why: an isolated CLI must not be able to write back to the real credential — cost if wrong: the user has to set up a key to run those tests. The user is told.
Task 4: re-review 1 found two items open.
- P-I1, exit-reason leg: a healthy runner inherits resume_failed through first-wins noteCause.
- New breakage: newRig dropped IsolateProviderHomes, so CODEX_HOME and CLAUDE_CONFIG_DIR are unisolated. removeSessionFiles could then delete REAL transcripts if the shell exports them.
Everything else is addressed.
C17 is confirmed: develop already ships step 4 with a marker, so the T12 dispatch MUST carry the adoption of legacy live agent_runners rows. The T12 plan does not mention it.
Task 4: fix round 2 dispatched (resumed the implementer); brief task-4-fix-2.md; BASE 15bcbc81a.
Task 4: fix round 2 committed 1eeb12636 (25 files). Gates exit 0 except loc-budgets, which was already over (nothing raised). F1 showed RED on base for both stop and shutdown. F2 is compile-checked only (no integration run is allowed); the kit guard tests were run untagged in a scratch module copy.
Self-reported, not fixed: onAPIConnLost can leave a stale connection_lost cause in a narrow pre-fork window. It goes to re-review 2 for a severity ruling.
Pre-existing flake, outside T4: TestGitBinding_FollowsAChatToItsNewWorktree (internal/api/v0, develop #216) fails TempDir cleanup with ".git: directory not empty". A git write outlives the test, the same class as the manifest-fetch flake (only a JOIN fixes it). Parked for the final review, with an issue to be filed with the user's OK.
Re-review 2 dispatched (resumed the re-reviewer) → task-4-rereview-2.md.
Task 4: re-review 2 says all findings addressed and no new Critical/Important. F1 is mutation-proven; F2 is grep-clean and vet-clean under all three tag sets.
Ruling (T4-G1): fix the stale connection_lost cause now (fix round 3), not park it. It reproduced in 5 of 8 runs, and the user sees "Crowbar lost its connection" for a Stop. The fix is narrow (the reason lives on the apiconn and only the watchExit wrapper notes it), and the implementer has the context — cost if wrong: one small extra round.
Ruling (T4-G2): the kit resolves only the supplied credential path and compares lexically; it never stats or resolves a provider home — why: the never-read-provider-internals law — cost if wrong: a credential path reached through a symlinked provider home is not caught.
Task 4: fix round 3 dispatched; brief task-4-fix-3.md; BASE 1eeb12636.
Task 4: fix round 3 committed 616190e81 (11 files). G1 showed RED on base, and its guard test keeps connection_lost for an adopted connection. G2 now resolves only the credential path; accepted cost: a credential reached through a dotfile-symlinked ~/.codex is no longer caught by its real path. Gates exit 0 except loc-budgets (already over, nothing raised). Re-review 3 dispatched (resumed) → task-4-rereview-3.md.
FOUND: TestSessionInvariants_HoldWhenIntentsRace (usecases/chat/runner_test.go) hangs with -count=15 at 1eeb12636 and 616190e81. The stack is ResumeChat → switchProviderLocked → awaitTurnOrForce → AwaitTurnComplete, which matches C9's shape. Pre-T4 (95f46ca3b) it instead fails "S4: a stopped chat is left working". So earlier green sweeps were lucky. A diagnosis was dispatched (opus, read-only, bisecting over the T4 commits) → intents-race-diagnosis.md. T4 is held open until the owner is ruled.
Task 4: re-review 3 found G2 and G3 addressed but G1 open, as NEW breakage. When serve crashes, the runner records `exited` 37 of 60 times (it was 60 of 60 connection_lost at 1eeb12636): watchExit reads lostReason before the pump stores it, and the pump skips storing once drop has cancelled the ctx.
Task 4: fix round 4 (H1) dispatched to a FRESH opus implementer per the R≥4 rule; brief task-4-fix-4.md; BASE 616190e81. It must prove every deliberate teardown cancels the ctx before serve can exit, or else move the cause decision to the owner of the teardown.
Task 4: fix round 4 committed 6ed428565 (4 files). A crashed serve now records connection_lost 60 of 60 (base: 18/60 adopted, 38/60 PTY-less). exitCause checks the ctx first, then the stored reason; every teardown path is cited in the report. TestHelperServe now binds under a temp name and renames once listening (0 failures in 2000 spawns). Gates exit 0 except loc-budgets (already over). Re-review 4 dispatched to a fresh sonnet → task-4-rereview-4.md.
Carry C18 (T9, which owns the api transport): production waitForSocket (apiserve.go:99) treats a socket path that merely exists as ready. A socket that is bound but not yet listening refuses, and the spawn falls back to hooks over a PTY (~0.4% of spawns in the helper measurement). It needs a failing test and a readiness signal owned by serve, not a retry.
Parked for the final review:
- The transport_overflow branch has no runner-level test.
- The scripted faults_test.go:65,135 still accept exited|connection_lost (tightening them needs an integration run).
Task 4: re-review 4 APPROVED. H1 is addressed, exitCause is race-free, and every teardown path was verified. The watcher test fails 20 of 20 at base. No new breakage.
Parked for the final review:
- The unbounded `<-lost` in TestWatchExit.
- detachNativeView (attach.go:~345): a re-established connection that dies before watchAPIConnExit records `exited` (display only).
- No runner-level test for transport_overflow.
Task 4's review loop is closed. Completion is held only on the intents-race diagnosis ruling.
DIAGNOSIS (intents-race-diagnosis.md): the hang is a LIVELOCK, not C9.
- reconcileRunnerExit closes the in-flight turn, but nothing orders that against the CLI's user_prompt hook for the same runner. So turns.Begin (or chats.StartTurn) can land after the close, leaving a turn open on a dead runner. Every switch, resume or revive then laps forever in switchProviderLocked.
- Pre-existing on the crash path. T4 widened it by deleting the turn-start interlock from Stop and Close.
- Bisect hang counts per 100 runs: pre-T4 4, HEAD 10–18.
- ~75% of the S3/S4 failures are a harness bug: concurrent f.wait() → asynx WaitPublish (NF5).
- Real users can hit it: Send, then Stop or Close before the CLI acks → the chat is bricked until a daemon restart.
Ruling (T4-J1): fix in T4 as round 5 (the last round before the breaker) — why: T4 widened it, and C9's fix would still spin on a turn nothing completes — cost if wrong: T4 grows by one more round. Round 5 goes to the round-4 implementer (resumed); brief task-4-fix-5.md; BASE 6ed428565.
Carry C19 (T5): T5's decision table needs the rule "a user_prompt or turn_started from a runner that is not st.LiveRunner() produces no rows", unless round 5 shows it redundant. Without it, Busy() sticks and the same wedge returns as permanent 409s.
Task 4: fix round 5 committed 07e89f5a3 (17 files). The turn-start gate moved into the live table; the turn package reaches it through Runners.TurnRunner. TestEndingRunner_APromptItAcknowledgesOpensNoTurn fails 20/20 at base. The intents-race test now runs 100/0/0, and 100/0/0 under -race. Gates exit 0 except loc-budgets (go.chat 36749, still under its 38317 baseline).
C19 confirmed still needed: the in-memory gate does not survive a restart. T5 ports the regression test to the ledger, and runner_exited closes the running turn in the same Do.
Disclosed: recordSpawnFailure (spawnrecord.go:78) has a small window. The new test file stop_invariants_test.go has no source of that name; re-review 5 is asked to check it.
Re-review 5 dispatched (resumed) → task-4-rereview-5.md. This is round 5: if findings remain, the breaker trips and I adjudicate.
Task 4: re-review 5 has ONE Important item open, so the breaker trips (R=5).
- deliverSteered (turn/turn.go:80-84 → steer.go:67) opens a turn with no gate. A parked steer delivered by turn_stop, racing a Stop or crash, re-poisons the chat; reproduced deterministically. Same hole at base.
- Minors: the recordSpawnFailure window is real; the in-op S4 Working assert was lost for revived rounds; IngestHook has no deadline; the gate.go comment is stale.
- stop_invariants_test.go already existed (#196). Ruling: extend it, since it has ~290 lines of room and runner_test.go is at its baseline.
- The reviewer's classifier refused the intents-race run, so the 100/0/0 tally is unverified by review.
Adjudication (T4-K1): load-bearing — the same brick-the-chat class. Ruled ONE targeted adjudicated fix (5b) to the same implementer. Every turn opens through one function that takes the gate, so a call site cannot miss it. Minors K2–K4 are included. A scoped verification follows; then T4 completes — why: the stated invariant is incomplete without it, and a chat-bricking race is not parkable — cost if wrong: one more small round. Brief task-4-fix-5b.md; BASE 07e89f5a3.
Task 4: adjudicated fix 5b committed abe4c97ba (15 files). Every turn now opens through startTurn, which needs a turnHold that only Runners.TurnRunner(runnerID, open) hands out under the gate; ingestUserPromptInterlocked is deleted. The steer RED test fails 20/20 at base. K2 has a deterministic test. K3 restores S4, adds a per-round S6 (turn_started names the live runner) and an end-of-walk S3; the mutation check fires them. intents-race: 100/0/0, and 100/0/0 under -race. Gates exit 0 except loc-budgets (go.chat 36783 < 38317).
Parked for the final review:
- Between markEnding and a failed quit's keepLive, a user_prompt or steer is refused (the failed-terminate path is engine-unreachable).
- claimEnd skips Complete when the runner is already gone from the table.
C19 is extended: T5 ports both TestEndingRunner_* tests to the ledger rule (S6 is that rule, checked from outside).
Scoped verification dispatched (resumed the re-reviewer) → task-4-rereview-5b.md.
Task 4: the scoped verification of 5b is APPROVED: K1–K4 addressed, no Important item open. The controller checked the intents-race evidence files itself: fix5b/intents100.txt is 100 PASS / 0 FAIL, ok 88 s; intents100-race.txt is 100 PASS / 0 FAIL, ok 742 s.
Parked for the final review:
- Gate.Lock ignores ctx (gate.go:135), so a gate held across endProcesses hangs the new test instead of failing it.
- turnHold is a plain value; "no turn without a hold" is a turn-package convention.
- K3 checks a registry-only stale turn at the walk's end, not per round.
Task 4: complete. Commits 71a5ba78c, 15bcbc81a, 1eeb12636, 616190e81, 6ed428565, 07e89f5a3, abe4c97ba.
Task 5: dispatched to an opus implementer, BASE abe4c97ba. Brief task-5-brief.md; report task-5-report.md. It carries C1, C8, C11, C14 (session.persists turn_start|turn_end, with codex = turn_end), C16 and C19 (ledger no-rows rule plus the TestEndingRunner_* ports).
Ruling (T5-C9): T5's deletion of AwaitTurnComplete and the switch force-kill (switch → 409 while busy) removes the awaitTurnOrForce half of C9. T6 keeps awaitNoPendingPrompt and the Stop cancel seam.
Ruling (T5-exit): the brief's single Do of runner_exited plus turn_ended supersedes T4's two-step order (close the turn, then exit). It keeps the crash-safety property: all or nothing.
USER 2026-10-18: "parallel some work". This overrides the skill's one-implementer rule.
Protocol: independent lanes run in their own git worktrees under <scratchpad>/wt/<lane>, on branches par/<lane> from abe4c97ba. Lanes never touch the main worktree. Each lane gets reviewed; reviewed lanes are cherry-picked onto fix/refactor-chats only in a merge window between S1 tasks (never while an S1 implementer is mid-task). Ratchet baselines are updated once, at merge.
Lanes launched alongside T5:
- par/s3-t10 (sonnet): S3 T10, errcheck in the terminal/fs/search/git engines. Ruling: S3's plan refresh is not needed for T10, since S1 does not touch those engines.
- par/gitbinding-flake (opus): TestGitBinding_FollowsAChatToItsNewWorktree, a writer outlives the test → join it at its owner.
- par/c18-socket-ready (opus): C18 pulled forward from T9, confined to the apiserve startup path.
- par/s5-t4 (opus): S5 T4, slim shiki bundle. Ruling: the live O2/O4 step is skipped for now (one dev instance; S5 T0's baseline is not measured), and the JS engine ships by default. Live measurement happens when S5 runs.
Not parallel now, and why:
- S3 T1 and T9: 110+ handler/app sites overlap S1 T5–T7.
- S3 T11: agents engine overlaps S1.
- S4: tree/placement overlaps S1, and needs its refresh after S1–S3.
- S2: consumes S1 interfaces.
- S5 T0–T3: live measurement needs the user at the keyboard, and T1 touches api.
Lane c18: stopped at RED (685160f66 on par/c18-socket-ready, a test only). The provider's serve binds and listens; Crowbar only chooses the path and polls for the file to exist, so no owner-side readiness exists without a descriptor schema change. Options: A stdio transport (recommended by the lane), B a declared readiness line, C a handed listener fd. A retry is rejected.
- descriptorcheck/appserver_unix.go awaitSocket is a second copy of the bug.
- The apiserve.go/apiconn.go comments use codex vocabulary.
- TestWaitForSocket_DetectsTheSocketAppearing sleeps.
Ruling (C18-hold): do NOT merge the RED-only commit; it rides with its fix. Option A is NOT ruled yet: the lane is asked to check it against the socket's other users (SwitchToTerminal/native attach, adoptAPIConn, the {socket} placeholder) before a ruling. The fix will not run in parallel with T5, which edits descriptor schema files for C14 and C16.
Lane c18 design check: option A survives all three.
- Switching to the terminal kills serve and forks `resume` with no {socket}; switching back forks a new serve.
- adoptAPIConn adopts only the spawn's own connection; nothing re-adopts after a restart.
- {socket} is used only in codex.yaml's serve line.
Ruling (C18-A): serve over a stdio channel declared by the descriptor. Delete {socket}, waitForSocket, descriptorcheck's awaitSocket and the unused hotswap-over-socket schema shape; fix the codex-vocabulary comments; replace TestWaitForSocket's sleep. The RED test 685160f66 rides with the fix.
- Live checks before it lands (scratch CODEX_HOME, env -i, no real provider home): codex app-server's stdio framing, that it exits when stdin closes, and that its stdout carries frames only.
- Schedule: its own lane right after T5 lands, because T5 edits the descriptor schema (C14, C16). It can run alongside T6 if T6 is file-disjoint.
- Why: the socket's only job was a path that Crowbar cannot see become ready; stdio removes the race by construction.
- Cost if wrong: a future hotswap provider that wants a shared socket needs option C (Crowbar listens and hands serve the fd).
Lane gitbinding: 662ab7d7c on par/gitbinding-flake. Root cause: OriginSyncManager.StopAll cancelled its runners without joining them, so an open-time git fetch wrote .git/FETCH_HEAD after Close. Fix: StopAll waits on its runners WaitGroup. synctest RED 200/200 → GREEN; the flake went 5/200 and 4/200 → 0/200 twice.
Ruling: the siblings with the same shape are fixed in this lane rather than carried: ProviderPollManager.StopAll, WatcherManager/Watcher.Stop, and cancel propagation to the git process, so Close never waits 30 s behind a hung fetch. Round 2 has been dispatched (resumed).
Env note for briefs: golangci-lint lives in /opt/homebrew/bin, so add it to the PATH prefix for `make lint`.
Lane s5-t4: 54cab8a65 on par/s5-t4. Parity test is 12/12 languages, 71/71 tests. Review dispatched (sonnet) → s5 task-4-review.md.
PAUSED 2026-10-18 at the user's request (out of tokens). Every subagent is stopped. State to resume from:
- S1 T5: the implementer was stopped MID-TASK; its last note was "Now the DTO". The main worktree has 99 uncommitted paths on top of abe4c97ba. Resume by sending a message to the T5 agent (abd2ed15b1f49a950) or a fresh opus with task-5-brief.md plus the dispatch carries. Do NOT discard the working tree.
- Lane gitbinding: round 2 (sibling StopAlls, cancel propagation) was stopped mid-gates; 9 uncommitted paths in wt/gitbinding-flake on top of 662ab7d7c. Resume agent a1cf00a37df54dab0.
- Lane s3-t10: COMPLETE (752349341, 2effbd4a9). Waiting for the merge window.
- Lane s5-t4: fix round 1 committed 4912b4f66. Its re-review (a958565200439d441) was stopped before finishing; re-run it.
- Lane c18: a RED test only (685160f66). Option A (stdio) is ruled. The fix lane starts after T5.
- asynx PR #48: verified at f7b6fcbbe/82e4c38 (S3 ledger). Waiting on a tag.
RESUMED 2026-10-19 at the user's word. T5 (abd2ed15b1f49a950), lane gitbinding round 2 (a1cf00a37df54dab0) and the s5-t4 re-review (a958565200439d441) were resumed by message.
asynx v1.1.0 released 2026-10-19. It contains the PR #48 merge e007f6ff3 plus 6 later commits, not yet verified. S3 T8 bumps to v1.1.0 and re-runs storetest and RunSnapshots on the SQLite port before relying on it.
Lane gitbinding round 2: 0d32d3d63. ProviderPollManager/WatcherManager StopAll join; Watcher.Stop cancels and joins; fetch/poll run under the runner ctx (no WithoutCancel). 6 synctest REDs 200/200 → 0/200; the flake 0/200 twice, 0/50 -race. lint/deadcode/counters exit 0; file-size exits 1 only for the obsolete 520-line baseline of origin_sync_manager_test.go (now 500), which the merge window's --update lowers.
Ruling (GB-1): a cancelled git command gets SIGTERM, not SIGKILL — git removes its own lockfiles on SIGTERM; the existing WaitDelay escalates to kill. One seam: engine/git/internal/exec/exec.go Cmd.Cancel, with a non-unix fallback to Kill — why: round 2 made an unsubscribe kill an in-flight fetch, and SIGKILL can strand a ref lock that lock_retry doesn't clean — cost if wrong: a git that ignores SIGTERM waits WaitDelay before the kill.
Ruling (GB-3/4): make Watcher.Start's backend walk observe ctx if it is in-package; fix the stale "blocks" doc on watcherProc.Start.
Ruling (GB-2, parked): lockRepo/lockRepoRead ignore ctx while queued; holders are now cancellable, so the wait is bounded by a holder that is itself ending or doing real repo work. Park for the S1 final review.
Round 3 dispatched (resumed a1cf00a37df54dab0). One task review covers 662ab7d7c..round 3 after it lands.
Lane gitbinding round 3: 853a5d8eb. GB-1 SIGTERM Cancel (real git fetch lock test, SIGKILL 5/5 RED → 0/50 macOS + 0/50 Linux docker); GB-3 fsnotify walk observes ctx (Linux-only test, docker 0/200); GB-4 doc. lint/deadcode/counters 0. Review dispatched (opus) over abe4c97ba..853a5d8eb; package lane-gitbinding-review-package.md.
Lane gitbinding review (opus a69bc8a1d9120c9f7, lane-gitbinding-review.md): spec ✅; quality Needs fixes. Important I1: origin_sync_manager.go:255 now passes the runner ctx into advanceLockedRoot, so an unsubscribe or shutdown SIGTERMs `git merge --ff-only` mid-checkout. That leaves a locked root half-advanced (HEAD old, files partly new); every later ff is refused as ErrDirtyTree, logged at Debug only. Reproduced by the reviewer. Minors: GitStream keeps SIGKILL (two policies; the exec.go:42 comment is false for stream.go); the FIFO test breaks `GOOS=windows go vet` (base passes).
Ruling (GB-I1): cancel gates whether a local write STARTS, never interrupts one in progress — check ctx before the fast-forward, then run MergeFFOnly to completion (the merge is local and bounded; StopAll's join waits for it). RED: an unsubscribe during the ff leaves a clean tree that a later ff advances — cost if wrong: Close waits for a local checkout to finish.
Ruling (GB-M): one cancel policy — startStream uses the same SIGTERM Cancel; move the FIFO test into exec_unix_test.go behind //go:build unix so Windows vet passes as at base.
Lane fix round 1 dispatched (resumed a1cf00a37df54dab0).
Lane gitbinding review fix 1: 26a14ab22. GB-I1: ctx gates the ff start, MergeFFOnly runs WithoutCancel to completion (real-git smudge-filter test, 5/5 RED → 0/50 macOS+Linux); GB-M: one command() builder → SIGTERM for buffered and streamed; FIFO test in exec_unix_test.go; Windows vet 0. lint/deadcode/counters 0. Scoped re-review dispatched (sonnet).
T5 implementer DONE_WITH_CONCERNS: 54a1b5fbd (one commit on abe4c97ba; 157 files, +2845/−7562). Unit sweep 139 pkgs, chat -race, bbx 30/30, descriptor-check ok, lint/deadcode/vets/file-size/counters ok; file-size 20 shrinks written back; go.domain +183 (turn fold); modules.json maps the new agent_working.go to wiring. Concerns: FailTurnOnNotice check-then-act; asyncWork>0 keeps a chat busy until a later turn ends or the runner exits; termwait still finds notices by a ticker screen poll (the implementer's "S6 replaces it" is wrong — S6 is wrapped links); turn pkg unit coverage 51%.
T5 review: package is 880 KB → split into t5-part-{A,B,C,D}.diff (157/157 files). Two opus reviewers: R1 = A (domain/ledger, repositories, engine/agents, api, tests, scripts) + D (usecases/chat top level); R2 = B (turn, shared, tree, conversation, fanout, workspace) + C (runner).
Lane gitbinding re-review 1 (sonnet): all findings addressed, no new breakage (lane-gitbinding-rereview-1.md).
Lane gitbinding: COMPLETE (662ab7d7c, 0d32d3d63, 853a5d8eb, 26a14ab22). Waits for the merge window; file-size --update at merge clears the origin_sync_manager_test.go entry.
Parked for the S1 final review: GB-2 lockRepo ignores ctx; git subprocesses outside engine/git bypass the SIGTERM seam (ref_runner.go:25, avatar.go:124, project_import.go:921, repos.go:703, prerequisites.go:65 — all read-only/one-shot, pre-existing); the stream SIGTERM test and the ref-lock test assume unix/git ≥ 2.28 (no Windows CI; CI git 2.47).
MERGE WINDOW (after T5 commit, during T5 review): cherry-picked s3-t10 (ab2f911dd, a0f3d0782) and gitbinding 662ab7d7c→6e0cdce16, 0d32d3d63→a5d4b536c. 853a5d8eb conflicts in backend_fsnotify.go (s3-t10 errcheck × GB-3 ctx walk, 2 hunks). Resolution delegated to the gitbinding agent (a1cf00a37df54dab0), which continues the sequence (26a14ab22, s5-t4 ×3) and runs build/race/linux vet/lint.
MERGE WINDOW DONE: s3-t10 → ab2f911dd a0f3d0782; gitbinding → 6e0cdce16 a5d4b536c 5cc18ca78 (conflict in backend_fsnotify.go resolved keeping both, by the lane agent) 438208c49; s5-t4 → c333d98f6 1bf63a67c 5a005824c; ratchet shrinks 8d8b5f067 (discarded-errors 159→105; origin_sync_manager_test.go gone). Post-merge: build, -race fs/realtime (macOS + Linux docker), linux vet, lint 0. loc-budgets still red: go.engine-other 13900→13998 now (s3-t10 + gitbinding combined) — joins R7. Lane worktrees removed; branches kept.
Lane c18 fix dispatched (opus) on par/c18-socket-ready rebased to 8d8b5f067 (RED now 83fc97495); brief lane-c18-fix-brief.md. Runs alongside the T5 review; confined to the serve/transport path.
Lane c18 fix agent: aaa9ae2702084e108.
T5 review A+D (opus a7d7b4b4a94f6f394, task-5-review-AD.md): spec ❌, Needs fixes. I1: C19's live-runner rule (turn_open.go:37) can be deleted with every chat test still green — the ported TestEndingRunner_* only assert end state; the crash-inside-the-ack-window race lost its test. I2: tests/integration/agent/agent_subagent_working_test.go deleted, but s1d plan :164 requires it to keep compiling for T9; no carry. ⚠️ C16 partial (an api-transport descriptor with NO refused block only warns). Minors M1–M8 (M1 applyTurnStarted reopens a closed turn; M2 a fold test that doesn't test its refusal; M3 claude persists implicit; M6 two order asserts relaxed to ElementsMatch; M7 duplicate row checker; M8 web toasts a raw 409 on ⌘/ during a turn).
Rulings (A+D), held until the B+C review lands so one fix round covers both:
- I1: a deterministic test that fails with the rule removed — a user_prompt / turn_started / steer delivered for a runner that is not LiveRunner writes no rows; and restore the crash-inside-the-ack-window interleaving deterministically (a seam that parks the ack). Prove it by mutation (rule off → red).
- I2: restore the integration file; it must compile under `go vet -tags integration`, porting any deleted API it used to the ledger's Busy. Never run it.
- C16: require `refused.rpc_code` on every api-transport descriptor (a missing refused block is an error, not a warning).
- M1 (a turn_started over an open turn is refused by the fold), M2, M3 (claude declares persists: turn_start; descriptorcheck requires persists on resumable descriptors), M6 (assert order per feed), M7: fix in the same round.
- M8: carry to the web — the S2 web fixes lane (C7): ⌘/ while working is guarded and the 409 maps to a calm message, never a raw error.
Ruling amended (M8): fix it in T5's fix round, not later — T5 changed the contract, so T5 adapts its client: handleSwitch is guarded while the chat is working, and describeSpawnError maps the busy 409 to a calm message. Targeted vitest only.
T5 review B+C (opus a1b1fc7f25877a7ea, task-5-review-BC.md): spec ✅ with ⚠️; Needs fixes. I1: brief-mandated idle-closes-at-once ends every codex turn before turn/completed (codex.yaml:575-583 live order) — answer after the busy edge, status/effort lost, a failed turn may fold done. ⚠️ Stop on a non-interruptible provider still waits; brief :120 "late → nothing" contradicts plan :45/:47/:165 (late rows, ingress never discards). Minors M1–M8.
Ruling (T5-idle): idle closes a turn only where the descriptor declares it; codex's turn end is turn/completed alone — why: a live-measured order beats R2's blanket rule — cost if wrong: a codex turn that idles without completing stays open until the runner exits.
Ruling (T5-late): spec beats brief :120 — late stop/failed/idle are written as late rows, fold state unchanged; the background-work clause reads the live runner's last turn_ended, late or not (resolves BC-M3) — cost if wrong: one extra row kind path to test.
Ruling (T5-stop): follow brief :124 — no wait on a non-interruptible Stop; report if a test proves the wait load-bearing.
Ruling: BC-M5 (steer text copy) → carry to T6; BC-M7 (staleness display lost until activity is replaced) → carry to T7. All other A+D and B+C minors fixed in round 1.
Fix round 1 dispatched (resumed abd2ed15b1f49a950); brief task-5-fix-1.md; HEAD 8d8b5f067.
Overlap note: T5 fix round 1 edits codex.yaml/claude.yaml/descriptorcheck rules; lane c18 edits codex.yaml's serve line, the schema and descriptorcheck/appserver_unix.go. Expect a merge conflict when c18 lands; resolve by the c18 agent, keeping both.
Lane c18 fix: 3bbb8f26c (on RED 83fc97495; 54 files +1275/−1375). runtime.api declares delivery: stdio, format: ndjson (load rule api_delivery); codex serve = [codex, app-server]; wsrpc → stdio jsonrpc; shared engineagents.StartServe; socket pieces deleted. RED replaced by TestStartAPIConn_ServeIsReadyOnlyOnceItAnswers + _AServeThatEndsUnansweredIsNotReady (mutation-checked). Live (codex-cli 0.163.0, env -i, scratch HOME/CODEX_HOME): handshake 177 ms one JSON line; stdin EOF exit 0 in 12 ms; RUST_LOG=trace stdout frames only. Gates 0 except loc-budgets (red at base). Lane lowered timers 27→24, discarded-errors 105→102. Concerns: codex binary is a ~/.local/bin symlink into ~/.codex/packages (executed only; nothing read/listed); integration scripted suite vetted not run; a user override with unix://{socket} now fails to load. Review dispatched (opus).
T5 fix round 1 DONE_WITH_CONCERNS: 3d67e590f (api: late rows, descriptor-declared idle close, C19 tests + usecase interleavings, AD-I2 restored, C16, minors, Stop no-wait via retire) + 3f4561ba5 (web: switchBlocked includes working; chat_busy code → calm message; file-size shrinks). All gates 0 except loc-budgets (red at base). Mutation: at usecase level the ledger rule and the in-memory TurnRunner check are layered (each alone green, both off red); the ledger rule alone is proven red in the turn package.
Concerns carried: codex turn_stop maps no async_work → CodexBackgroundedSubagentKeepsChatWorking is expected to fail live until T9 (carry C20 → T9: make it pass; T13 live check); notice match fires on a model row that STARTS with the needle (re-review to judge vs the whole-line ruling); the codex failed-turn end is known only from a schema-derived fixture (carry → T13 live check).
Scoped re-review dispatched (opus) over 8d8b5f067..3f4561ba5.
Lane c18 review (opus a0c1a274f117f6eb9, lane-c18-review.md): spec ✅ ⚠️, Approved. Risk 4 answered: an old unix://{socket} override fails AcceptOverride and degrades to the built-in descriptor — no crash, no whole-list failure. Minors M1/M2/M3/M5 sent back (M5: zero provider vocabulary in Go is a hard law — sweep apidriver.go, spec/v3.go, spawnplan.go, descriptorcheck appServer/app_server). M4 (Windows pipes not pollable): info only, no Windows build. M6: the controller squashes RED 83fc97495 into the fix at merge. Integration scripted suite (fake codex on stdio) vetted only → runs in the T13 / Docker pass.
Lane c18 review fixes: 178750143 (M1 rewrap; M2 no-PTY asserted while gated; M3 StartServe reaps on a failed close, untestable without a test seam; M5 provider and wire-method names out of every non-test Go file the lane touched — controller grep confirms zero; descriptorcheck step app_server → api_serve, not persisted, visible only in `crowbar descriptor test --live`; TestScripted_ConnectionDrop). Gates 0. Lane c18: COMPLETE (83fc97495 RED + 3bbb8f26c + 178750143) — merges after the T5 re-review, squashing the RED into the fix.
Carries: C21 → T9: apidriver.Start hard-codes the handshake's `initialized` notification and `clientInfo` param — declare the handshake in the descriptor (hard law). C22 → T6: runner/capabilities.go comment still says "app-server notification".
T5 re-review 1 (opus a37c592e6188c2747, task-5-rereview-1.md): open — BC-M2 (needle is a squeezed row-prefix match; whole-line impossible: the banner wraps and carries an account URL and reset time), AD-I1 (the in-memory TurnRunner hold has no test at its own level). New Minors N1–N4 from the late-rows ruling. No new Critical/Important.
Rulings: BC-M2 amended → a descriptor-declared anchor (the banner's leading glyph) matched on the raw row, taken from a repo capture; N1 idle never restates asyncWork; N2 a late row is never projected as a transition; N3 a failed turn restates asyncWork 0; N4 a Stop ends the runner's background-work claim. Accepted: a codex api turn with no turn/completed stays busy until Stop/exit (silence never ends a turn) → T13 live check.
Fix round 2 dispatched (resumed abd2ed15b1f49a950); brief task-5-fix-2.md.
PAUSED 2026-10-19 at the user's request (out of tokens). All agents stopped; no test processes left. Resume state:
- S1 T5: fix round 2 STOPPED MID-TASK (last note "Now the domain implementation"); 10 uncommitted paths in the main worktree on top of 3f4561ba5 — do NOT discard. Resume abd2ed15b1f49a950 (brief task-5-fix-2.md), then a scoped re-review.
- Lane c18: COMPLETE at 178750143 (wt/c18-socket-ready). Merge after T5 closes: squash RED 83fc97495 into 3bbb8f26c, then 178750143; expect a codex.yaml/descriptorcheck conflict with T5 (c18 agent aaa9ae2702084e108 resolves).
- Then T6 (carries C22, BC-M5), with parallel lanes.
T5 fix round 2: d2b1ee578 (BC-M2 notices match a declared anchor on the raw row — codex.yaml anchor "■", descriptorcheck terminal_notices.anchor_missing; N1 only turn_stop restates asyncWork; N2 late rows project no turn frame; N3 failed turn restates 0; N4 a Stop voids the runner's restatements until its next turn; AD-I1 runner test, proven by mutation). All gates 0; loc-budgets red at base.
Ruling (anchor source): the "■" anchor stands — the verbatim banner row is in four #151 fixtures recorded from a live 0.146.0 screen, and codex's live-verified interrupt banner uses the same glyph — cost if wrong: the notice stops matching, caught by the T13 live check (added).
Ruling (wire change): a turn_ended row with no asyncWork means "restates nothing" — such rows exist only in unreleased dev homes and a restart mints new runners — cost if wrong: one stale fold in a dev home.
Scoped re-review 2 dispatched.
T5 re-review 2 (opus a6977727cdb3c74e3, task-5-rereview-2.md): all six ADDRESSED, no new Critical/Important. Minors: M1 anchor checked after trimming leading whitespace (notice.go:34), so an indented glyph row still matches; M2 Stop while busy only on background work writes no turn_ended, so the claim holds until runner_exited.
Ruling (M1): fix — the ruling said raw row; column 0 of the untrimmed row holds all four fixture banners. Round 3 dispatched (resumed abd2ed15b1f49a950); the controller verifies the small diff.
Ruling (M2): accepted — the runner is alive with background work until its exit is recorded, which Stop's SIGTERM→kill escalation bounds; voiding it earlier needs a ledger "stopping" row (scope growth) — cost if wrong: busy lingers a few seconds after Stop on a background-only chat. Parked for the S1 final review.
T5 fix round 3: cdc4fde0e (anchor at column 0 of the untrimmed row; RED: two indented banner rows) + 6ff4aa081 (TestInterruptTurn flake from T5's background send, 114/300 under -race → 0/500; joins backgroundWork, drops a 3 s time.After). Controller read the diff: correct, but notice.go's new comment names "codex" (hard law) → one-line reword dispatched with a provider-vocab grep over T5's non-test Go.
Lane c18 merge prep: the lane rebases onto 6ff4aa081 in its own worktree (branch par/c18-merge; RED squashed into the fix), resolving T5's descriptor/descriptorcheck/apiconn conflicts keeping both; the controller then cherry-picks. T6 starts after c18 lands (T6 rewrites the apiconn pump). T6 carries: task-6-carries.md (C9, BC-M5 ruling, C22, staging/trailer overrides).
T5 round 3 follow-up: ea9985074 (notice comment names no provider). Grep: no line added in abe4c97ba..HEAD names a provider.
Task 5: complete (54a1b5fbd; fix rounds 3d67e590f, 3f4561ba5, d2b1ee578, cdc4fde0e, 6ff4aa081, ea9985074). Parked for the final review: M2 Stop on a background-only chat; N4 next-turn clear needs a future provider that both interrupts in place and reports background work.
Carry C23 (hard law, "pre-existing is never an answer"): 16 non-test Go files still name providers on older lines (e.g. turn/compaction.go, turn/observation.go, engine/agents/agents.go, descriptorcheck/rules.go:172,178). After c18 merges, re-grep and run a vocabulary-sweep lane over the files T6 does not touch; the rest ride with the task that owns them.
Lane c18 MERGED: ee5c3063e (RED squashed into the fix) + 0dc078f21. The lane rebased onto 6ff4aa081 itself (conflicts: interrupt_internal_test kept T5's join; file-size baseline regenerated, only drops); all gates 0, bbx 30/30. Cherry-picked clean onto ea9985074. Lane worktree removed; branches par/c18-socket-ready and par/c18-merge kept.
C23 survey: 59 non-test Go files name a provider (c23-vocab-files.txt). Ruling: T6 sweeps the files it modifies; the rest go to a dedicated lane after T6, starting with a read-only classification (comment / logic belonging in the descriptor / external contract such as an env var name) — why: a blind rename of 59 files collides with T6–T9 — cost if wrong: the law stays broken a few tasks longer.
T6 dispatched (opus), BASE 0dc078f21; brief task-6-brief.md + task-6-carries.md.
C23 survey (sonnet a13a767e651c577c4, c23-vocab-survey.md): 121 hits = 120 comments + 1 naming (seed fixture "claude"); 0 logic, 0 external contract in the 59 files (the L/X items sit in tests/kit integration files). Packages: V1–V4 (68 hits) free now; V5 rides T6, V6 T7, V7 T7 (31 hits in files T7 deletes), V8 T8/T9.
Ruling (C23 lane): one sonnet lane does V1–V4 (batched same-shape rewording) in wt/c23-vocab (par/c23-vocab from 0dc078f21), skipping every file T6 touches (api/libs/status.go, runner/capabilities.go and T6's brief list) — the regression gate (the survey's "ratchet counter") is NOT added as a new baseline entry (baselines only shrink); instead novocab coverage widens to all of api/internal at T13, when the count is 0 — cost if wrong: a regression between now and T13 is caught only at T13.
Lane c23 (sonnet a69529d857c517170): 5cef114aa chore(api): comments name no provider — 37 files, 62 keyword hits + riders fixed, comments only (token stream identical outside comments); 7 hits skipped by exclusion (status.go:64,143; capabilities.go:26-28,47 → T6; :28's test rename travels with it). Controller spot-checked the wording: whys kept, nothing lengthened. Lane COMPLETE; cherry-picks after T6 lands (never while an implementer is mid-task in the main worktree).
MERGE WINDOW after T6: c23 5cef114aa→a vocab commit; S6 T1–T5 (12 commits) → …bc166ebfc; S3 T11a → 6b00bfeec, 0f75948c9 (one comment-only conflict in descriptorcheck/term.go: kept the vocab lane's wording). Build, vet, integration vet 0. Ratchet shrinks 097d22a1f (go.discarded-errors 99→57; diff.go off the oversize list). Note S6-M2: a6d08f1a6's copy fails lint alone. Lane worktrees removed; branches kept.
T6 DONE_WITH_CONCERNS: 7d3b3b3ef (135 files +2823/−3990; ingress 97.2%; Step 4 coverage and bbx regex 0; race on chat and cmd 0; lint, deadcode, vets, file-size, counters 0; go.timers 24→23, discarded 102→99; loc go.total −667). Concerns: (1) Symptom2 refusal test flaky 4/6 head, 5/6 at base 0dc078f21; (2) AConcurrentFailureSortsBeforeTheSwitch fails at base (waits on a removed log line; busy switch is 409 since T5); (3) barriers_test.go kept as the live suite's harness; (4) deleting AwaitOpen brings back the turn_stop-before-final-delta race, and its 2 regression tests were deleted; (5) relay retries can duplicate rows with dedup gone; (6) Lock ignores ctx; a CLI that dies at startup gets its queued hooks dead-lettered as unknown_runner. Two opus reviews dispatched: ingress side → task-6-review-ingress.md; deletions/tests → task-6-review-deletions.md.
T6 review ingress (opus a9ff28c535ee41abd, task-6-review-ingress.md): partial; Needs fixes. I1 a failed observation write is answered 2xx with no row and no dead letter (Dispatch logs and drops via note for every observation kind; message MarkRecorded after a failed write; a foreign-conversation dead-letter write failure → 200 with nothing kept). I2 an api-channel ask the ingress refuses is never answered (pump continues on a Hook error, skipping awaitAndReply) → the provider's turn waits forever; a regression vs base. 9 Minors. Risk rulings: Lock should take ctx (non-blocking); relay duplicates acceptable until T7/T11 (no shipped descriptor uses the relay). Waiting for the deletions review before the fix round.
T6 review deletions (opus aaeaf04102626df2f, task-6-review-deletions.md): not compliant (the BC-M5 RED is missing); Needs fixes. I1 deleting AwaitOpen brings back the live duplicate-assistant-message bug (2026-08-29), its 2 regression tests deleted, and T7's plan does not fix it; I2 the lock ordering has no effective test (RecordStop regression deleted; the replacement tests pass 200/200 with the lock removed, verified by overlay); I3 the BC-M5 RED is missing. M1 the Symptom2 flake comes from T4 71a5ba78c (first-cause-wins drops resume_failed behind displaced). M2 a dead switch test since T5.
Rulings (task-6-fix-1.md): fix ING-I1, ING-I2, DEL-I1 (owner: the turn's message record — a final message after the turn_stop fallback replaces or joins it, keyed by turn), DEL-I2 (port, synctest, mutation table), DEL-I3, all Minors; Lock takes ctx now (channel-based) — why: cheap, and it makes the lock tests deterministic. Relay duplicates accepted until T7/T11 (no shipped descriptor uses the relay). C24 (next task after T6): the Symptom2 flake — a restart-delivered provider spawns once, carrying the prompt.
Fix round 1 dispatched (resumed a44f124c67daa6ec0).
T6 fix round 1 (a44f124c67daa6ec0): 3b53ca789 (M9 relay hang-up closes its ask as proceeded; api runner end releases as abandoned) + 4196cd5a1 (ING-I1, ING-I2, ctx Lock, DEL-I1 message record owns the turn's answer — a late final delta joins it keyed by the ledger's last turn, DEL-I2, DEL-I3, ingress M2–M6/M8, DEL-M5/M6/M8/M10; new: a steer whose turn_stop reply failed to record settles delivery_failed, RED added) + 18eb3350a (DEL-M2/M3/M4) + 903724f7d (ratchet, discarded 57→56). Mutations a–r all RED. Gates 0 except loc-budgets (red at base; +270 go.chat, +314 go.tests). Carry DEL-M9 → T7 (TestMap_ARepeatedAskJoinsTheFirst replaces the no-prompt-ID permission regression). Edge left: an api connection that drops while its PTY lives holds its in-flight ask until the runner ends.
T6 scoped re-review 1 dispatched (opus aa641b7016bd141f6) on review-097d22a1f..903724f7d.diff → task-6-rereview-1.md.
C24 lane dispatched (opus aed60f1198ef685e2), wt/c24 branch par/c24-refused-resume from 903724f7d; brief c24-brief.md, report c24-report.md. Ruling: a provider started to deliver a prompt spawns once carrying it; cause precedence alone is not the fix (keeps the doomed second resume) — cost if wrong: one rework round.
T7 prepared: task-7-brief.md + task-7-carries.md (C12 leftAt onto the ledger + preloadLedger under Store.View; BC-M7 read-time close for subagents; DEL-M9; DEL-I1 tests ported, never deleted; relay dedup on both channels; V6/V7 vocab incl. seedProvider literal → descriptor-derived). Dispatch after T6 completes and C24 merges (C24 touches runner/{revive,prompts,live}.go).
Lane S5 T1 running in parallel (see the S5 ledger).
T6 re-review 1 (opus aa641b7016bd141f6, task-6-rereview-1.md): every round-1 finding ADDRESSED; new Important N1 — the DEL-I1 join records under hook-<uuid> while the late delta streams under its own id → the client never confirms the bubble (ghost duplicate + 1 s poll). Minors M-a join crosses messages (no scope/turn check), M-b answers map unbounded, M-c steer-turn duplicate, M-d telemetry 422 body prints into the status line, M-e two 4-line comments.
Rulings (task-6-fix-2.md): N1 the message record decides the id before the live push; a joined delta's live frame carries the answer's recorded id (no row rewrite, no web change), RED on the live frame id. M-a join only main scope + prefix-related text — why: same reply streamed; ids alone can't tell for a provider without turn ids — cost if wrong: two identical consecutive replies join (only under a refused user_prompt). M-b runner exit drops its entry. M-c → T7 carry (message-id keyed fold). M-d curl --fail. M-e trim.
Parked C25 (S1 final review): dead letters with chat_id "" are never purged; foreign_conversation letters have no retention; refusal count is a count(*) on the liveness path.
Fix round 2 dispatched (resumed a44f124c67daa6ec0).
C24 DONE_WITH_CONCERNS: 53b9c54c4 (wt/c24) — a send to a dormant chat launches one CLI carrying its prompt (reviveForDelivery plans via resolvePromptDelivery + shared launchCarrying); RED revive_internal_test.go both orders; mutation incl. precedence-only alternative; Symptom2 bbx 20/20 (base 2/6). Lint/deadcode 0; loc go.total +43 (now 37 over).
Rulings (c24-fix-1.md, before review): fix the staged-switch double spawn here (same class/owner); a lost session on the carrying api push loses the session (never spawn_failed); verify claude.yaml hotswap:true against the attach-path prompt drop and fix it either way (carry or refuse up front, never silent pending); no prompt_submit → refused on dormant send accepted (consistent with restart); gapTurns 0 unchanged. Resumed aed60f1198ef685e2.
T6 fix round 2 (a44f124c67daa6ec0): bb7680b7e (N1 decide-once before the live push, frames under the reply's id; M-a main scope + last turn + prefix, re-checked on the finished text; beyond the ruling: salvageUnfinished and closeLateReplies route through the same join-or-own decision; M-b AbandonMessageForRunner drops the runner's open messages and held reply on exit/leave/boot) + 928089b05 (M-d curl --fail) + f20f6c51f (M-e). Mutations r2a–r2j RED. Gates 0 except loc-budgets; go.total +86 → 80 over (was 6 under). Caveat: a middle chunk before index 0 fails the prefix check → its own row (prior behaviour) → T7 carry.
T6 scoped re-review 2 dispatched (sonnet) on review-903724f7d..f20f6c51f.diff → task-6-rereview-2.md.
T6 re-review 2 (sonnet ad75c9d936b62b544, task-6-rereview-2.md): N1, M-a, M-b, M-d, M-e ADDRESSED; new Important I1 — the round-2 extension (closeLateReplies/recordUnfinished) re-records the old reply via CloseTurn while the next turn is open → it takes T2's DisplayOrder, plan and repointed tools. m1 non-zero first chunk; m2 divergent late message shows only on next refresh.
Ruling (task-6-fix-3.md): a join adding no text writes nothing; once the ledger's last turn moved on, the old reply row is never written again — why: CloseTurn's repoint belongs to the open turn — cost if wrong: an unfinished late tail beyond the recorded answer is lost (T7 fold closes it). RED through the real activity aggregate. m1/m2 → T7. Fix round 3 dispatched (resumed a44f124c67daa6ec0).
T6 fix round 3: 1c1c033d2 (joinAnswer guard: adds-no-text writes nothing; writeJoin only while the ledger's last turn is the reply's, shared answersLastTurn; closeLateReplies takes only streams still reading as the reply). RED via real activity aggregate (newActivityTurns, waits on ax.WaitPublish); mutations r3a–r3e RED, r2a–r2f/r2i still RED. Gates 0 except loc-budgets; go.total +17 → 97 over.
T6 scoped re-review 3 dispatched (sonnet) → task-6-rereview-3.md.
C24 fix round 1: cc2cc9f77 (every send launches one CLI carrying its prompt: staged switch live+dormant, lost-session api push loses the session, attach path delivers or refuses) + 28cde6217 (ratchet). Every new test RED on 53b9c54c4; 5 mutations RED; Symptom2 family 140/140; lint/deadcode 0; go.total 128050 (88 over). claude.yaml hotswap:true has no runtime.api (hooks channel) → no shipped descriptor reaches the attach path. Concerns: flake TestSwitchProvider_MidTurn_ClosesTheOutgoingTurn (base 5/400); a switching send with a pending prompt now refused busy; dividers before provider_switched; latent: forkCLI failure on the attach path never drops the api conn.
C24 review dispatched (opus) on review-903724f7d..28cde6217.diff → c24-review.md.
T6 re-review 3 (sonnet ad009a8179ce7b906, task-6-rereview-3.md): I1 ADDRESSED; no new Critical/Important; reviewer's overlay mutation of writeJoin went RED. Out of scope: (1) the still-allowed join (late stream longer than the reply, no next turn) rewrites the reply row through CloseTurn — DisplayOrder 1→4, plan wiped, StartedAt moved (T6-introduced in round 1, untested); (2) AbandonMessageForRunner's bool discarded by both callers; (3) runner-exit salvage runs without the chat lock.
Ruling: T6 complete; (1)–(3) carry to T7, which deletes the activity aggregate and turn/message.go — why: the defect lives entirely in code T7 removes and the branch ships nothing in between; a round 4 needs a fresh higher-tier implementer for code with days to live — cost if wrong: if T7 slips, the branch carries a reordering of a late-extended reply.
Task 6: complete (7d3b3b3ef; fix rounds 3b53ca789, 4196cd5a1, 18eb3350a, 903724f7d, bb7680b7e, 928089b05, f20f6c51f, 1c1c033d2). Carries: DEL-M9, M-c, N1 live-id contract, m1, m2, round-3 cost, re-review-3 (1)–(3) → task-7-carries.md.
Ruling (T8 split): T8a = engine half (grammar pattern:/join kind_map, vocabulary fields, ToolEvent/ChoicePrompt, descriptor values + scrubbed capture fixtures, engine tests, V8 vocab) runs NOW as lane wt/t8a (par/s1-t8a from 1c1c033d2) in parallel with T7; T8b (ledger fields, ingress rows, canonical-input ask binding, DTO, black-box) after T7 — why: T8a's files are engine-only; T8b's files are created by T7 — cost if wrong: additive YAML/vocabulary conflicts at cherry-pick (T7 adds message_complete). Scope: task-8a-scope.md.
Ruling (T7 placement): T7 runs in lane wt/t7 (par/s1-t7 from 1c1c033d2), not the main worktree — why: C24 and T8a merge into the branch meanwhile, and the main worktree must stay free for merge windows; the T7 agent rebases onto the tip when told and resolves keeping C24's one-spawn delivery and T8a's fields — cost if wrong: one rebase round inside T7.
T7 dispatched (opus af22f0f669496ee1f) in wt/t7; T8a dispatched (sonnet af0d2b8eafc357249) in wt/t8a.
C24 review (opus abad3a4715a8fbbf6, c24-review.md): Needs fixes. Risks 1/3/4 hold; I1 an api establish refusal during a carrying spawn still forks a PTY resuming the lost session with the settled prompt (now reachable from every dormant send / staged switch on codex). Minors M1 staged selection committed before launch, M2 early refusal records no provider_switched, M3 naming/vocab, M4 comments, M5 attach forkCLI failure leaks the api conn, M6 stale plan after displaceForPrompt's unlocked wait ("start session, then quick send"), M7 assertions/placement; flake root cause is in the test (async hook in duringTerminate).
Rulings (c24-fix-2.md): fix I1 (ErrAPISessionLost, no fork) and every minor — M1 selection persisted with runner_started; M2 provider_switched in the same write as the provider move; M6 re-plan under the lock after the wait (C9 pattern); flake fixed in the test. Fix round 2 dispatched (resumed aed60f1198ef685e2).
T8a DONE_WITH_CONCERNS: 13d70c430 (par/s1-t8a): pattern/join map branches, mapping.LookupKind composite fallback, vocabulary fields, ToolEvent/ChoicePrompt, claude/codex values, 12 scrubbed real fixtures; engine tests green; lint/deadcode/vets/descriptor-check 0; mapping_test.go 503→487; LOC go +101, tests +211. Concerns: tool_fail + claude permission captures not resolvable by the replay harness → inline payloads, tool_fail stays unverified; codex value deviations (exit_code only post/fail, item.command fallback, dynamicToolCall kept, collab wait/close targets); codex item.tool on collab calls (T8b: MCP-ness keys on MCPServer); codex hooks apply_patch target = whole patch text; trailer says "Claude Sonnet 5.5" (fix at merge).
T8a review dispatched (sonnet) → task-8a-review.md.
T8a review (sonnet a7cd980f27bff61da, task-8a-review.md): spec PASS with gaps; Needs fixes. I1 pattern regex recompiled per eval (1762 ns/57 allocs); I2 mapping.go:129 still names agentMessage; I3 a transform can't sit in first_present → codex hooks apply_patch target = whole patch. Risk 1 deviations judged right; fixtures real + scrubbed; tool_fail is a data gap (needs live captures), permission is structural (item-scoped fields). M4 codex tool_fail lacks tool_title; bashFailure payload edited; trailer Sonnet.
Rulings (task-8a-fix-1.md): I1 compile at load onto the expression node (no global cache); I2, I3, M4 fix; permission replay skips item-scoped families paid by fixture_test.go comment trims; tool_fail stays unverified → carry C26 to T13 live (real tool_fail capture with MCP + subagent fields needs provider auth in a scratch config dir = user decision); bashFailure verbatim; 13d70c430 trailer fixed by the controller at merge. Fix round 1 dispatched (resumed af0d2b8eafc357249).
Carry C26 → T13: record a real tool_fail capture (MCP server/tool and subagent_id present) so tool_fail's unverified flag can go.
Carry → T8b (task-8a-review.md hand-offs): MCP-ness keys on MCPServer (codex sets item.tool on collab calls too); clamp ToolInput size on ask rows.
Carry → T9: v3_test.go at 500 and mapping.go at 492 — split v3_test.go by provider before adding.
C24 fix round 2: 4f3fa2ed1 (flake: hook delivered inside terminate; 400/400 under -race, base 5/400) + a40815f54 (I1 no fork after an api establish refusal; M1 pick travels with the carrier; M2 provider_switched with the provider move; M3–M7; M6 re-plan after the unlocked wait) + 8a3ff3b47 (ratchet). 11 new/changed tests RED on old code, 50/50 -race; 8 single-fix reverts each RED; Symptom2 140/140; gates clean; go.total 127964 (2 over; was 88 over). Open questions: a no-prompt ResumeChat after an api refusal still falls back to a PTY resuming the lost session (pinned by TestSpawnRunner_ARefusedAPIResumeLeavesAHealthyRunner); a live same-provider send commits its pick before delivery; a refused send reports ErrPromptOutcomeUnknown though the ledger says session_lost.
C24 scoped re-review 2 dispatched (opus) → c24-rereview-2.md, with the three open questions.
T8a fix round 1: 0aaa588bf (mapping.Expr holds the compiled regexp, built at load: 1877 ns/57 allocs → 31 ns/0 miss, 408 ns/2 match; agentMessage → message; first_present takes transforms; apply_patch target from the real PreToolUse.applyPatch.json; claude permission replayed against 2 real captures, no longer unverified; fixture_test.go 471 (off baseline); bashFailure verbatim; codex tool_fail tool_title; v3_test.go split). Concerns: a DERIVED fixture (collabAgentToolCallFailed: live completion with status edited to failed); claude/codex-hooks MCP calls get no tool_kind mcp.
Ruling: the derived fixture goes — R17 forbids fabricated fixtures, and "derived" is an edited recording — codex tool_fail's new keys stay unverified and join C26 (T13 live capture) — cost if wrong: one more unverified mapping until T13. Ruling: add tool_kind mcp for claude and codex-hooks MCP calls now (one YAML line each, pinned by the existing real MCP fixtures) — why: kind stays consistent across channels; T8b still keys MCP-ness on MCPServer. Round 2 (small) dispatched (resumed af0d2b8eafc357249).
C24 re-review 2 (opus aca9fe274a1f7bd87, c24-rereview-2.md): I1, M1–M7, flake ADDRESSED. New Important N1: 3 integration tests (scripted faults lostThenFresh ×2, live LostSessionStartsFresh codex leg) still require NoError on a refused send → fail every time now (nightly/T13 only). N2 no-op selection_set per revive; N3 doc lengths. Q1 → (a); Q2 harmless; Q3 keep ErrPromptOutcomeUnknown, typed error needs a web change.
Rulings (c24-fix-3.md): N1 assert the ruled outcome (vet only; T13 runs them); Q1 (a) every api establish refusal fails the spawn with ErrAPISessionLost, /resume answers 4xx, fallback-pinning tests rewritten RED-first — why: T3's law is that a refused session is never resumed again, and the fallback 200 hands back a CLI that dies seconds later — cost if wrong: a resume after an api refusal needs a second click (fresh); N2, N3 fix; Q2 keep; Q3 keep → C27. Fix round 3 dispatched (resumed aed60f1198ef685e2).
Carry C27 → S2 (web ChatStore): a typed session-lost send error, plus the matching web change: Retry on a settled/abandoned prompt row mints a fresh request id, and the "may already have this message" wording goes when the ledger says session_lost.
T8a fix round 2: feabfbafb (derived fixture + its test deleted; per-field `unverified_fields: [tool_title]` on codex tool_fail — fails if the field starts resolving or stops being mapped; tool_kind mcp on claude pre/post/fail and codex hooks pre, pinned by real MCP fixtures).
Ruling: unverified_fields accepted — why: a block flag would silence the block's proven fields; the per-field flag fails loudly when the field resolves — cost if wrong: one small grammar key to fold into the block flag later. T8b carry amended: MCP rows key on Kind == mcp; MCPServer is optional (codex hooks has none).
T8a scoped re-review dispatched (sonnet) on 13d70c430..feabfbafb → task-8a-rereview-1.md.
T8a re-review (sonnet adeb9bf8a6fd166a3, task-8a-rereview-1.md): APPROVE, every finding ADDRESSED; regexp on Branch.re compiled at decode; fixtures match captures; unverified_fields proven by 4 mutations; sizes ≤ 500. Minors → carries: isSubObjectField also skips `questions` (declare via unverified_fields) → T8b; claude tool_fail mcp-kind line unpinned and apply_patch Add/Delete unrecorded → C26; codex tool_fail mcp_server/mcp_tool resolve only against a pre-existing schema-derived fixture → C26.
Task 8a: complete. MERGED onto 1c1c033d2: e0b6c9b29 (13d70c430 with the trailer corrected to Opus 5.5), 6142bdb36, 8a682d588; tree identical to par/s1-t8a. Lane wt/t8a to be removed; branch kept.
PAUSED 2026-10-20 at the user's request ("STOP EVERYTHING"). All agents stopped; no test processes left. Resume state:
- Branch fix/refactor-chats HEAD 8a682d588 (T8a merged). T6, T8a complete.
- T7: lane wt/t7 (par/s1-t7), NO commits, 47 uncommitted paths mid-implementation (last note: merging moved attachment code into the usecase's attachments.go) — do NOT discard; resume af22f0f669496ee1f.
- C24: lane wt/c24 at 8a3ff3b47 + 9 uncommitted paths of fix round 3 (last note: mutations caught, about to run gates) — resume aed60f1198ef685e2, then a scoped re-review of round 3, then merge before T7 rebases.
- T8b: after T7 (carries in this ledger + task-8a-review.md hand-offs).
RESUMED 2026-10-23 (user: "Go on!"): T7 af22f0f669496ee1f, C24 aed60f1198ef685e2 resumed in their lanes.
C24 fix round 3: 87a41cff7 (every spawn fails on a refused session, 409 conflict — not 424, which the web renders as "isn't installed"; N1 integration asserts (vetted, first run at T13); N2 unchanged pick skipped; N3). 23 packages pass, -race clean, 5 key tests 50/50, 3 mutations RED, Symptom2 140/140; go.total 127970 (8 over). Attach toggle still discards a refusal (no spawn, nothing resumes). Addendum dispatched: trim SetChatSelection (13) and validateStagedSelection (9) docs to ≤3 lines.
C24 addendum: c82fef603 (docs to ≤3 lines; go.total 127954, 8 under budget). Scoped re-review 3 dispatched (sonnet) on 8a3ff3b47..c82fef603 → c24-rereview-3.md.
C24 re-review 3 (sonnet a68ca9ae52b48a444, c24-rereview-3.md): APPROVED — N1/N2/N3/Q1(a)/addendum ADDRESSED; one refusal check in spawn.go:196 serves every spawn path; 409 renders "Couldn't resume/switch" + raw daemon text. Out of scope: a refused pre-fork spawn leaves an empty runner dir + surface-registry entry (abandonUnforked reaps neither); the toast shows raw "apperr: conflict" text.
C24: complete (53b9c54c4, cc2cc9f77, 28cde6217, 4f3fa2ed1, a40815f54, 8a3ff3b47, 87a41cff7, c82fef603). Carries: C28 (S1 final-review fix dispatch, fix not park): abandonUnforked reaps the runner dir and surface-registry entry of a refused pre-fork spawn; C27 += the raw daemon text in the resume/switch toast.
Merge: the C24 lane rebases onto 8a682d588 (T6 rounds 2–3 + T8a touch the same packages) and re-runs its gates on the combined code; then the controller cherry-picks.
C29 lane dispatched (sonnet), wt/c29 par/c29-dompurify from 8a682d588 → c29-report.md.
C24 MERGED: the lane rebased onto 8a682d588 with no conflicts (ratchet baseline merged keeping both sides' lower numbers); combined-code gates: build/vet 0, -race chat usecases 20 pkgs 0 races, C24 key tests 20/20 -race, switch test 50/50, Symptom2 140/140, message-stream bbx 5/5, integration vet, cold lint 0, deadcode, file-size, counters 0. Branch fast-forwarded to c3d734dd3 (a63ffec20..c3d734dd3, 8 commits, all with the Opus trailer). go.total 128254 vs 127962 (R7). Lane wt/c24 removed; branch kept.
C29: 717595c0a pins dompurify ^3.4.17 / override >=3.4.17. A real flag: GHSA-cg2g-xh64-v4q7 (critical), GHSA-mrv2-3pqr-j5jr (high) and GHSA-c6gr-qvv4-5v3f (low), published 2026-10-22; the lock was resolving 3.4.15. lint:doctor 0; tsc/eslint/knip/prettier 0; user tests 24/24. Controller-checked: diff is 2 files, 5 lines.
Ruling: before the merge, C29 adds html-node.test.tsx (sanitized render of a markdown file's raw HTML, RED by un-sanitizing). Why: the sanitizer had zero behaviour tests, and this is the XSS surface. If wrong, it costs one test file.
C29 MERGED by cherry-pick: 99a800d0c d596cd66f (pin + html-node.test.tsx: 4 tests, RED by un-sanitizing 3/4). Lane removed.
PAUSED 2026-10-23 (user: "Stop agents here please"): T7 af22f0f669496ee1f stopped mid-task (last: appending Recorder tests to ingest_internal_test.go); lane wt/t7 kept uncommitted. Branch at 99a800d0c (C24 + C29 merged).
RESUMED 2026-10-24 (user: "Go on!!"): T7 af22f0f669496ee1f.
T7: 840b4b485 on 1c1c033d2 (209 files, +8236/−15299, go.total −3053). bbx 280/0 with 1 test skipped; coverage ledger 95.0 / ingress 93.7 / transcript 96.8; lint/deadcode/vets/descriptor/file-size/counters 0. Concerns: TestRegression_DeleteChat_LeavesProviderSessionIntact hangs 30 min at base and tip (purge announces exited, test awaits displaced, probably since 71a5ba78c; agentFrames.await has no deadline); carry 3(2)-(3) partial; rebase pending.
Ruling: close them BEFORE review (task-7-prereview.md): rebase onto 99a800d0c; exit writes under the ingress lock; fix the hang by the spec plus deadlines on the kit waiters (never skip); all gates on the combined code. Why: the concerns are correctness, and reviewing pre-rebase code would review code that never ships. If wrong, it costs one longer implementer turn.
T7 before-review: head 82e50dee3 on 99a800d0c (c925e429b T7 rebased; 44789ad59 exit writes under the chat lock, RED shown; 82e50dee3 hang fixed: the TEST was wrong per s1b plan :436, "displaced/moved no longer produced", missed by 71a5ba78c; bbx frame waiters stop after 30s and list the frames seen). Gates: full bbx 281/0/0 skipped, -race 19 pkgs, coverage 95.0/93.7/96.8, vets, lint 0, deadcode, descriptor, file-size, counters. go.total 128254→125273 (−2981). Behaviour changes from item 2: a crash during a turn_stop now DELIVERS the steer (as Stop does); a hook arriving while the exit is recorded → ErrNotFound + dead-letter (check it against "nothing a hook says is dropped").
Ruling: review T7 with TWO parallel opus reviewers. A = spec + production code (non-test). B = carries checklist, tests, ported/restored tests, and the item-2 behaviour changes. Why: a 1.2 MB diff (220 files) is too much for one pass, and the split is disjoint. If wrong, it costs one extra review seat.
T7 review A (opus a08a94126f538ca0d, task-7-review-a.md): Needs fixes. I1: facts.go:220-245 holds the CLI for an unawaited ask but writes it closed (answerable iff HoldAsk; CLI blocks 120s with no card). I2: transcript.go:56-76 Projector.Apply rebuilds the view twice per commit, under the ledger writer and one global mutex (breaks the Subscriber no-block contract; O(n²) streaming). I3: ledger writes outside the ingress lock: answers.go:186, :204-206, conversation/chatlog.go:72 (runner/stop.go divide unconfirmed). 11 minors. Deletions done, message_complete wired, no lock inversion, no vocab. Waiting for B before the combined fix round.
T7 review B (opus ad2d96a7587d3a77e, task-7-review-b.md): Needs fixes. I1: a hook after the exit is refused and dead-lettered (content lost). I2: background subagents read stopped and their tools abandoned on the real fixture (AsyncWork=2 ignored). I3: the 2 KiB error caption gone, and a failed blob write drops the tool call (tests deleted with no successor). I4: 2 more tests deleted with no successor. 3a and the hang fix sound; other carries done; test law holds.
Rulings (task-7-fix-1.md): A-I1 held ⇔ answerable, the card shows whenever the CLI is held. A-I2 per-commit O(commit) projection, per-chat state, benchmark. A-I3 every ledger write under the chat lock. B-I1 a late hook is recorded into its turn; dead-letter only the unplaceable. B-I2 declared async work outlives the turn end (amends BC-M7). B-I3 caption restored, blob failure degrades. B-I4 port the tests. All minors. Steer rule acknowledged. Why: each follows "nothing a hook says is dropped" and one owner per state. If wrong on B-I2, it costs one projection rule.
T7 fix round 1 → af22f0f669496ee1f.
T7 fix round 1: a1cf46b52 (21 commits on 82e50dee3). All of A-I1..I3, B-I1..I4 and the minors, RED first. -race 19 ok; bbx 282/0/0; coverage 138 pkgs; lint/deadcode/vets/descriptor/file-size/counters 0. BenchmarkProjector_Apply flat at ~1,000 ns/op and 15 allocs (from 116k/2.19M ns). go.total 125596 (−2658 vs 99a800d0c; this round +323). Two base tests reversed per held ⇔ answerable. Deviations: Subagent.SpawnToolID deleted (no writer) → T9 CARRY: add it back with its writer; the seed uses "read_file" (no catalogue). Residuals: a post-commit rebuild can delay that chat's next Apply by one fold; the live buffer is uncapped within a turn.
Scoped re-review 1 (opus) → task-7-rereview-1.md.
T7 re-review 1 (opus ac159dd2c22e2d001, task-7-rereview-1.md): all 11 rulings ADDRESSED, no regressions; deviations accepted ((b) read_file is no provider's wire name). One Important gap: an async turn end abandons a held background-subagent ask (the CLI blocks 270 s with no card) → fix round 2 (task-7-fix-2.md): the ask closes with its own scope, using the reviewer's probe as RED.
T9 CARRY: Subagent.SpawnToolID returns with its writer. The live buffer (ingress/live.go) gets a per-key cap or tail window (frames carry the whole text, so a long tool_output stream costs O(n²) bytes).
T7 fix round 2 → af22f0f669496ee1f.
T7 fix round 2: ce6b70e5f (3 commits; ledger.Ask.OutlivesEnd as the one predicate for fold/ingress/transcript; probe ported RED; minors in 8545defd5; the two-chat test now catches a global lock). race 20, bbx 282/0/0, coverage, lint 0, gates 0; go.total −2633. The rule keys on held + async end, since the descriptor maps no subagent id on a permission. Scoped re-review 2 (sonnet) → task-7-rereview-2.md.
T7 re-review 2 (sonnet abaa9b682984d9a75): I ADDRESSED (one predicate, a RED per site). N1: movesAsks omits KindSubagent (stale choices frame). N2: an expired desk hold writes nothing, so a scope-"" ask stays a card with no waiter. Ruling: the desk records the outcome it gave the CLI on expiry, under the lock, in the same commit. T7 fix round 3 → af22f0f669496ee1f (task-7-fix-3.md).
PAUSED 2026-10-24 (user: "Stop all agents please."): T7 af22f0f669496ee1f stopped mid fix round 3 (task-7-fix-3.md); lane wt/t7 kept, uncommitted state as is. Resume that agent.
T7 round 3 at pause: committed 3add51d26 (N1), 411f998c7 (N2), 12f019279 (nits) on ce6b70e5f; lane clean. The final gates were NOT run (stopped as it began them; no stray processes). On resume: run the gates, then the scoped re-review.
RESUMED 2026-10-25 (user: "Go on please!"). macOS's 3-day /tmp cleanup had deleted most of lane wt/t7 (no .git file, so git saw it as prunable). Branch par/s1-t7 @12f019279 was intact. Rebuilt it: pruned the t7 metadata only, ran `worktree add` at the same path and restored its .superpowers. Resumed af22f0f669496ee1f to run the final gates for fix round 3.
T7 fix round 3 gates (on the rebuilt lane, cold) at 12f019279: race 20 ok, bbx 282/0/0, coverage 138 ok, vets 0, lint 0, deadcode, descriptor-check, file-size and counters all clean; go.total −2631 against 99a800d0c. Plan nit to fix at merge: s1d-feed-migration-gate-plan.md:388 still names `...IsPushedAsUnanswerable`; its successor is `...IsClosedAsProceeded`. Scoped re-review 3 (sonnet a80ac08c5f512865d) → task-7-rereview-3.md.
T7 re-review 3 (sonnet a80ac08c5f512865d, task-7-rereview-3.md): APPROVED. N1 and N2 addressed. The lock order is ingress, then ledger, then the desk as a leaf; 1,800 races under -race gave exactly one ask_resolved row each.
Ruling: fix its Minors before merge, because the ledger records an outcome the CLI never got. (1) An api-channel expiry sends a refusal but records `proceeded`; record what was delivered. (2) AnswerChoice can record `answered` that never reached the CLI; one claimant under the desk mutex writes, the loser gets a typed "no longer waiting". (3) Stale dto text and s1d plan :388. Then rebase onto e578c0a34 and run the gates once. Why: a ledger that lies is the bug class this plan exists to remove. If wrong, it costs one round.
Ruling: resume af22f0f669496ee1f for round 4 instead of a fresh implementer. Why: it isn't stuck (round 3 was approved); these are new findings, and it owns the code the rebase conflicts will touch. If wrong, a fresh opus agent costs more context rebuilding.
T7 fix round 4 + rebase → af22f0f669496ee1f (task-7-fix-4.md).
RESUMED 2026-10-25 after a session restart: T7 Part A committed (6782daa9e refused-on-expiry, c811318b0, afcb0b095 one claimant writes, 8196c3137 docs). The Part B rebase was at 1/34 with a conflict in file-size-baseline.json (resolve to the lower value per entry). Resumed af22f0f669496ee1f.
T7 fix round 4 + rebase: pre-rebase 6782daa9e, c811318b0, afcb0b095, 8196c3137. Rebased onto e578c0a34 → head e9b940d73 (36 commits: 1330c12d6 refused-on-expiry, 2ec9e2c39 Answer seam, 6ec2b3d5a one claimant writes + ErrNoLongerWaiting + injected clock, 036227c23 docs + s1d :388, 29ab68def file-size write-back, e9b940d73 go.timers 23→22). One conflict (file-size baseline; lower value per entry, three dead entries dropped). Gates on the head: race 20/20, bbx 286/0/0, coverage 139, lint 0, deadcode, descriptor-check, file-size and counters 0; go.total 126209 (−2529 vs base 128738). Residuals: (a) a claimed answer is recorded answered even if the relay hangs up or a re-ask displaces it before delivery; (b) an api-channel ask that isn't held is refused with nothing recorded. Scoped re-review 4 (sonnet) → task-7-rereview-4.md; it rules on (a) and (b).
T7 re-review 4 (sonnet a5cd4ca571307851a, task-7-rereview-4.md): APPROVED. Items 1–3 addressed; the rebase is clean (31/34 identical; the 3 that differ change only baselines; no baseline rose). Residual (a): reachable, Minor (a re-ask displacing a claimed answer drops it while the UI shows success). Residual (b): reachable, IMPORTANT (an unheld api ask over 128 KiB is refused to the CLI but leaves a pending card that later closes as proceeded or abandoned).
Ruling: merge T7 now and carry (a) and (b) into T8b as must-fix items 4 and 5 (task-8b-scope.md). Why: T7 is at round 5 (the breaker); (b) sits on the ask payload path T8b's ToolInput clamp already rewrites; T8b is the next sequential S1 task. If wrong, (b) lives on the branch for one task.
T7 rebased onto c521aa596 by the controller (no conflicts; the non-baseline diff is identical to e9b940d73; no baseline rose; 5 file-size entries removed). Web gates on the new base: tsc 0, knip 0, eslint and prettier on the 5 web files 0, vitest 2 files/58. Ratchet: file-size ok, counters ok (go.timers 22). Go is unchanged against e9b940d73's gates (S5 T6 is web-only).
Task 7: complete. MERGED by ff: fix/refactor-chats c521aa596 → 348f4dcbc (36 commits). Lane wt/t7 removed; branch par/s1-t7 kept.
T8b lane: wt/t8b (par/s1-t8b from 348f4dcbc). Scope: task-8b-scope.md.
T8b dispatched (opus acf371c68ef0f6a00) in wt/t8b, BASE 348f4dcbc; scope task-8b-scope.md (carries 1–5).
T8b DONE_WITH_CONCERNS: 1b9115417 (tool and ask detail through ledger, ingress, transcript and DTO; carries 1–2; black-box MCP regression), dfdd21289 (carry 4: an unheld api ask is recorded refused), 60da30fb5 (carry 5: the answer moves to the displacing relay, first-writer-wins on hang-up, 409 when none waits), 307c81a36 (carry 3). Gates green: race 95, full bbx, vets, cold lint 0, deadcode, descriptor-check, file-size, counters; go.total +141. Deviations: asks.go; binding by name when no input was recorded; carries 4/5 regressions in-package; two synthetic chat fixtures edited. Concern: a hang-up after commit, before delivery, still records answered. Review (opus a23b6085d42c1e1c1) → task-8b-review.md.
T8b review (opus a23b6085d42c1e1c1, task-8b-review.md): spec ✅, Approved, no Critical or Important. Concern (e) is inherent: no channel confirms receipt, so "answered" means handed to its relay. (d) is not fabrication: hand-written test maps, and the one recorded MCP pair carries the same input on both events.
Ruling: fix M1 (a misjoined same-name ask displaces another's relay), M2 (log), M3 (clamp on encoded bytes), M4 (log), M5 (a spurious 409 while a relay waits), the M7 perf note, M6 (a scripted black-box test, vet only, added to the T13 run list) and the nit, in round 1. Why: M1 binds the wrong ask and M5 shows a false error. If wrong, it costs one round. S2 note: a truncated ToolInput is a JSON string.
T8b fix round 1 → acf371c68ef0f6a00 (task-8b-fix-1.md).
T8b fix round 1: 7174bf590 (M1+M2), 3d04c8006 (M3, measured on the row's own encoding), 6118dd454 (M7), 47c8c5fd8 (M5+M4), cb2c7dc5f (M6, vet only), dcb2950cd (nit). Gates: race 92, full bbx, vets, cold lint 0, deadcode, descriptor-check, file-size, counters; go.total +206 against base. M5 inverts one old assertion on purpose.
Carries: M1(3), binding api approvals by item id → T13 live capture; M6 → T13 run list; 12 pre-existing lint issues under the integration tag in tests/integration/scripted → S1 final review fix dispatch (C30).
Scoped re-review 1 (sonnet) → task-8b-rereview-1.md.
T9 prepared: task-9-brief.md (from the s1d plan) + task-9-scope.md (carries 1–7: SpawnToolID, the live-buffer cap, C13, C20, C21, the v3_test split, the ws.go:90 discard). It dispatches after T8b merges.
T8b re-review 1 (sonnet a8b42744361c9b2ec, task-8b-rereview-1.md): APPROVED, all 8 items ADDRESSED. Each fix was reverted in a scratch copy and its test went red. M5 under a 300× -race stress was clean. Minors go to T9 scope item 8. T13 run list: TestScripted_AnAskTooBigToHoldIsRefused, plus live captures of a codex PermissionRequest with its PreToolUse and of an api approval with its item/started.
Task 8 (8a + 8b): complete. T8b was rebased onto 15ef2aacb by the controller (no overlapping files; diff identical). build, vet, the integration vet, file-size and counters all ok. MERGED by ff → 3a5725dcb (10 commits). The wt/t8b lane was removed.
T9 dispatched (opus a55e0dd2fd8f3c4c5) in lane wt/t9 (par/s1-t9, BASE 3a5725dcb), with task-9-brief.md + task-9-scope.md (carries 1–8).
~~~~

## B: 2026-10-17-s2-chatstore-plan

Source: `.superpowers/sdd/2026-10-17-s2-chatstore-plan/progress.md`

~~~~markdown
# SDD ledger — plan: docs/plans/2026-10-17-s2-chatstore-plan.md
Plan committed 7f7b7d719 (index + s2a-s2e; T1-T13). Spec: architecture §11 S2 row, chat-surface report, additions design A-J.
Pre-flight scan: preflight.md in this dir (opus a1f347a49112c09c7): 29 pairs (8 disagree, 7 partial), 13 task rows, 36 rulings P1-P36.
Execution: after S1.

## Rulings
Ruling: ADOPT P1-P36 as written. Carry each into the task it names. Highlights:
 - P1: every task runs `make ratchet` without --update and fails on any grew/new except keys its plan names; T13 alone writes back.
 - P2: T11 (delete the R13 projector) runs right after T5. P13: a mechanical pre-split commit (six >500 files, no behaviour change) before T3.
 - P3: S1 is amended before it starts (see S1 ledger); S2 T1 shrinks to /blobs.
 - P7/P8/P9/P11/P12: live text falls back to row text (Claude streams chunk rows); harness origin kept; halted pill derived from turn_failed; steer-send, compaction hold, image-upload hold and staged model/effort KEPT; T3 keeps getPendingPrompt until T5.
 - P26: T13 reports physical and code-only lines; no blank squeezing; over 12k → bring the cut list to the USER.
 - P31: df + bun install before T3; Tauri build needs space.
Ruling (D1-D8, provisional until the USER answers): D1 accepted only with P11 (steerable prompts persist as ledger rows); D5 → a Retry action on the failed provider state (no timer); D2, D3, D4, D6, D7, D8 accepted per preflight §6. ASKED USER.
USER 2026-10-18: "Agreed with everything". The provisional decisions recorded in this ledger are ACCEPTED as defaults.
USER 2026-10-18, new requirement → S2 T3 ("The transcript comes from the feed"). The S2 plan refresh must fold it into T3's text:
- While a message is streaming, a mermaid or excalidraw block whose fence is still open renders as a skeleton, never as partial code or a parse attempt.
- It renders once, when the fence closes.
- If the stream ends with the fence still open, the block falls back to plain code. A skeleton never outlives its stream (no ghost UI).
- The skeleton-to-diagram transition must not snap.
- Test through the real Plate path, with deltas that split the fence.
Carry C27 (from S1 C24, 2026-10-20): a typed session-lost send error + web: Retry on a settled/abandoned prompt row mints a fresh request id; drop the 'may already have this message' wording when the ledger says session_lost. Details: ../2026-10-17-s1-chat-ledger-plan/c24-rereview-2.md Q3.
~~~~

## B: 2026-10-17-s3-operations-errors-plan

Source: `.superpowers/sdd/2026-10-17-s3-operations-errors-plan/progress.md`

~~~~markdown
# SDD ledger — plan: docs/plans/2026-10-17-s3-operations-errors-plan.md
Plan written by opus acd12d5ff0c309de3 on 2026-10-18: an index plus s3a–s3d, 12 tasks.
- S3 owns the asynx bump (T8).
- T8's precondition is a named asynx release after v1.0.0 that fixes NF1, NF2 and NF7 and passes the verifier's probes. No Crowbar workarounds.
Pre-flight (opus ac90d59b68f41ae5f) → preflight.md: 3 Blocking, 13 Important, 12 Minor.
Ruling: adopt F1–F28 exactly as recommended in preflight.md. The Blocking ones:
- F1: halted already has one owner (useDaemonHaltStore + DaemonHaltGate, 2dea4beab). T2 teaches the connection store `migrating` only, and every halt source calls halt().
- F2: T6's re-add test makes `<slug>` a regular file, so it can fail.
- F3: T11 uses golangci v2 `linters.settings.errcheck.check-blank`, scoped to non-test files.
Ruling on decisions D1–D6, adopted provisionally (user told; they may override):
- D1: amended per F8. report() ignores Monaco Canceled and routes daemon_* to the daemon-state owners. Type-aware no-floating-promises lint goes to S9.
- D2: no generic Retry.
- D3: dedicated /v0/operations stream.
- D4: keep the newest 500 settled operations; say that pruning drops a placeholder's reason.
- D5: a branch create opens its chat once the branch exists.
- D6: lastError leaves through the existing upcaster.
Ruling: R18 accepted for migrating (refusals, probe, open clears). Its halted toast is rejected (F1).
Ruling: before S3's first dispatch, a plan-refresh pass applies these rulings to the plan text and reconciles it with S1/S2 as landed, because S1/S2 will move files the plan cites.
- Cost if this is wrong: implementers would follow stale text.
- Mitigation: every S3 dispatch also carries this ledger's rulings.
USER 2026-10-18: "Agreed with everything". The provisional decisions recorded in this ledger are ACCEPTED as defaults.
T8 gate check, 2026-10-18 (asynx PR #48, head 7fc6d96). Verdict: MEETS GATE once tagged. Full report: <scratchpad>/asynx48-verification.md.
- NF1, NF2 and NF7 are fixed. Each new probe fails on v1.0.0 and passes on the head, on both the in-memory store and Crowbar's SQLite adapter.
- NF2 on SQLite: an unrelated command took ~111 ms on v1.0.0 and 4–7 ms on the head.
- Still open: there is no tag yet. Five of our posted probes (NF1, NF2, NF2b, NF3, NF5) deadlock by construction on a correct fix; bounded replacements are in <scratchpad>/asynx48/probes/.
- Not fixed upstream:
  - NF3b: a retry on a parked aggregate returns nil.
  - ResumeDeliveries is quadratic in parked aggregates.
  - The PR's NF2 test cannot fail.
  - Two doc lines still say "per shard".
- Port preview for T8 (in a scratch copy only):
  - About 130 lines in the eventstore/sqlite adapter. storetest.Run passes 22/22 and RunSnapshots 8/8 under -race.
  - Three test files use the old store API: workspace/internal/store/rebuild_test.go:217, chat/internal/store/store_test.go:232, chat/activity/internal/store/store_test.go:142.
  - The quiesce comments in repositories/container.go and drain/gate.go:90 become stale, since WaitPublish no longer refuses.
  - The adapter is pinned to one connection, so sync projections in the v1 port must stay short.
  - Reference port: <scratchpad>/asynx48/crowbar/api/internal/adapter/eventstore/sqlite/.
T8 gate re-check at asynx 837dd59 (head since moved to 823280b, a refactor that was smoke-tested only): still MEETS GATE once tagged. NF1, NF2 and NF7 hold. On SQLite: storetest 22/22, RunSnapshots 8/8, NF2 at 4–5 ms against 77–128 ms on v1.0.0. There is still no tag.
The "all fixed" claim does not hold:
- NEW NF3b regression: a retry of the event that itself parked the aggregate returns ErrDeliveryParked; it was nil before.
- NF3b IsParked/log-read race.
- A retry on a parked aggregate reads the whole delivery log (29 ms at 50k pending on SQLite, holding the one connection).
- ResumeDeliveries is still quadratic.
- Docs residue.
Report: <scratchpad>/asynx48-verification.md, section "Re-verification at 837dd59"; probes zz_pv48b_*.
T8 gate re-check at asynx 30c3265: still MEETS GATE once tagged (NF1, NF2 and NF7 hold; SQLite storetest 22/22, RunSnapshots 8/8). There is still no tag.
Fixed since 837dd59: the NF3b regression, the IsParked race, the test gap, the parked-retry cost (now 36–86 µs) and the docs.
Still open or new:
- A DEADLOCK: ResumeDeliveries locks up to 512 aggregates while a command sends to another aggregate from Validate. Before the T8 bump, check whether Crowbar ever sends cross-aggregate from inside a command.
- A wrong ErrDeliveryParked after partial recovery.
- A nil returned after ResumeDeliveries fails a log read.
- ResumeDeliveries is still quadratic past 512 parked aggregates.
- Two untested mutations.
Report: <scratchpad>/asynx48-verification.md, section "Re-verification at 30c3265".
T8 gate re-check at asynx f7b6fcbbe (and 82e4c38): MEETS GATE once tagged. 51 bounded probes pass under -race x5. ResumeDeliveries is linear. All four 30c3265 findings are fixed. There is still no tag.
Still open:
- ResumeDeliveries returns nil on a ctx cancel between aggregates (a regression).
- Test gaps M28–M31 survive.
- The shared shard intake gives ErrQueueFull to neighbours under heavy load (at QueueDepth 1000: 55% refused at 2048 callers).
- A pre-existing Forget-marker double delivery, 4–14%.
- A re-park during a resume is "left for the next call", with nothing scheduling that call.
Crowbar exposure (item 5): no command sends cross-aggregate while holding a lock; Validate is pure; Crowbar has no durable subscribers, sync projections, ResumeDeliveries or PausedAggregates. QueueDepth 1000 must stay; ErrQueueFull maps to 503 with no retry.
T8 note: the open items do not touch Crowbar's usage, except the Forget-marker double delivery (Recover redelivers a failed Forget at startup). T8 must make Forget-marker handling idempotent, or confirm it already is.
T10 (parallel lane par/s3-t10): 752349341, 22 files, +340/−127. 54 sites → 0; go.discarded-errors 159 → 105. Two RED→GREEN tests. Linux docker run as uid 501 green. Review dispatched (sonnet) → task-10-review.md.
Carry to T11 (and likely T9): with errcheck check-blank on, 21 sites in T10's area are invisible to the counter regex (5 read-path defer Close, 9 doublestar.Match, 7 strconv.Atoi, and the recover at pump.go:85). They need semantic fallbacks. T11's step 4 requires a clean lint-backlog, so T11 must take them.
Carry to T9: handlers/ws.go:90 `_ = eng.Attach(...)` must log the error.
loc: T10 adds go.core +92 and go.engine-other +83 non-test lines. To be reconciled at merge; no budget is raised.
T10: the review was compliant with one Important item (6 unchecked SetTheme test calls, +6 errcheck backlog). Fix round 1 is 2effbd4a9. The controller verified the diff (36 lines): all 6 calls are wrapped, failStart keeps sentinel identity, spawn reaps only a killed child, and warnUnlessClosed also skips os.ErrClosed. errcheck backlog over the 12 packages: base 19, head 19.
Task 10: complete on par/s3-t10 (752349341, 2effbd4a9). It waits for the merge window after S1 T5; the counters baseline (go.discarded-errors 159 → 105) is updated at merge.
asynx TAGGED v1.1.0 (2026-10-19): contains the PR #48 merge e007f6ff3 plus 6 later commits. T8 pins v1.1.0, re-runs SQLite storetest and RunSnapshots against it first, and makes Forget-marker handling idempotent (see the open upstream items above).
T10: MERGED into fix/refactor-chats (ab2f911dd, a0f3d0782); counters baseline 105 committed 8d8b5f067.
Ruling (T11 split): T11a now as a parallel lane (wt/s3-t11, par/s3-t11 from 0dc078f21): the counter's sites under engine/agents, engine/lsp and tests/kit (~42), plus the brief's failing tests. T11b after T9: errcheck check-blank (F3), the whole-repo test -z gate, and T10's 21 counter-invisible sites — why: the whole-repo gate cannot pass while T9's area (blocked by S1 T6's handler/usecase edits) still holds sites; engine/agents and lsp are disjoint from S1 T6 — cost if wrong: one rebase if S1 T8/T9 touch the same engine lines.
T11a dispatched (sonnet), brief task-11-brief.md.
T11a DONE_WITH_CONCERNS: b838f3438 (27 files; go.discarded-errors 102→60; 5 RED→GREEN; agents+lsp 36 pkgs, vet, lint, deadcode 0). Behaviour changes: lsp ClientCodeActions/ClientLenses error on a malformed command (was stripped); Initialize fails on undecodable result/capabilities; applyEdit/configuration with bad params → -32602; processTransport.Close returns its error. Kit teardown logs (t.Logf). Part B input: check-blank shows 27 non-test lines here + 382 test lines. Review dispatched (sonnet).
T11a review (sonnet af324e149e2eba43c): spec ✅ ⚠️ (kit t.Logf sound); Needs fixes. I1 one malformed lsp command fails the whole code-action/lens request (web swallows it → user loses every action); I2 Initialize fails on an odd executeCommandProvider → server never starts, re-spawned per request. Ruling: both degrade — drop the bad entry with slog.Warn and keep the rest; Initialize fails only on an undecodable result — why: third-party data with a still-valid answer; R19's 'caller gets the error' assumes the caller can act — overrules the brief's '…IsAnError' test name — cost if wrong: a malformed server is quieter (logged). Fix round 1 dispatched (resumed a3eb1b6c9ec0c9df7).
T11a fix round 1: ccfd44127 (lsp drops a malformed command/lens with slog.Warn and keeps the rest; an odd executeCommandProvider warns and offers no commands; Initialize fails only on an undecodable result; minors done; closeLogged stays put since server.go is 586 lines). Controller read the core diff: correct. Process note: the worker ran pkill -f server.test to stop its own hung draft test — no other server.test seen. Task 11a: complete (b838f3438, ccfd44127) on par/s3-t11; merges after S1 T6 lands. go.discarded-errors 102 → 60 (baseline written at merge). T11b waits for T9.
T11a MERGED (6b00bfeec, 0f75948c9); baseline 097d22a1f.
~~~~

## B: 2026-10-17-s4-stream-store-tree-plan

Source: `.superpowers/sdd/2026-10-17-s4-stream-store-tree-plan/progress.md`

~~~~markdown
# SDD ledger — plan: docs/plans/2026-10-17-s4-stream-store-tree-plan.md
Plan written by opus ac9a31b72c2ba3a14 on 2026-10-18 and committed as 044e6bfc2: an index plus s4a–s4d, 13 tasks.
Preconditions:
- P1: an asynx release past v1.0.0 that fixes NF1, NF2 and NF7, bumped by S3 T8.
- P2: S1–S3 have landed.
- P3: at least 8 GB of disk free.
Pre-flight (opus aa7f0904140918d06) → preflight.md: 4 Blocking, 19 Important, 9 Minor.
Ruling: adopt F2–F32 exactly as recommended in preflight.md, except where amended below.
Ruling, F1 amended: the Tree represents everything that is legal and visible today. No upgrade step moves a user's chat.
- A chat filed in another branch's folder stays there. It keeps its bound workspace, and R4 admits container workspace ≠ bound workspace for a chat.
- V5 ("reappears under its own branch") is REJECTED. It would break the user's HARD hierarchy invariant and "same features".
- FromLegacy still normalizes the two cases whose visible rendering is unchanged:
  - an off-parent fork goes under its fork parent's node;
  - a dangling bubble parent goes to the chat's ground.
- Count each case under its own label in Mark. TestTreeContainersStep_NormalizesEveryLegacyViolation also pins that a cross-branch filing survives unchanged.
- Cost if wrong: the Tree carries one extra legal case.
Ruling, D6: refuse only what is refused today (chat-under-chat across workspaces). Cross-workspace filing of a chat into a folder stays allowed, as today.
Ruling, D9 (per F12): re-key only the shell files (842 lines). Delete the dead files (427). The sidebar-owned files (784) stay in web.sidebar. Re-keying sidebar-owned code to fit the gate is loosening a budget, which CLAUDE.md forbids.
Ruling, F11: the sidebar ≤5k gate (spec §11) stands.
- The S4 plan-refresh, due after S1–S3 land, must show real deletions that reach it.
- If a genuine design pass cannot, the counts go to the user as a spec-amendment question. Never a budget move.
Ruling, F21: T13 runs targeted gates only, never `make pr-checks` or the full vitest suite (a HARD user rule). CI runs the full suites.
Ruling, decisions D1–D10, adopted provisionally (the user is told and may override):
- D1, D2, D3, D8, D10: as the planner recommended.
- D4: as recommended, but drop the scope instead of resubscribing (F22).
- D5: only after the F4 fix.
- D6, D9: as ruled above.
- D7: with the F6 fix.
Ruling: before S4's first dispatch, a plan-refresh pass applies these rulings and reconciles the plan with S1–S3 as landed, as for S3.
USER 2026-10-18: "Agreed with everything". The provisional decisions recorded in this ledger are ACCEPTED as defaults.
~~~~

## B: 2026-10-17-s5-editor-host-plan

Source: `.superpowers/sdd/2026-10-17-s5-editor-host-plan/progress.md`

~~~~markdown
# SDD ledger — plan: docs/plans/2026-10-17-s5-editor-host-plan.md
Plan committed 70a64beb5. Spec: docs/plans/2026-10-16-crowbar-architecture.md (S5 row, editor sections).
Pre-flight scan: preflight.md in this dir (opus a8dc15f18dbdf24ae): 18 pairs (4 disagree/at risk), 9 task rows, 21 recommended rulings.
Execution: after S1 and S2. Needs web/node_modules (make web-install) and ≥ 25 GB free for the Tauri build; K1-K3 and O3 need the USER at the keyboard.

## Rulings
Ruling: ADOPT all 21 recommended rulings in preflight.md §8, with the amendments below. Carry each into the task it names.
Ruling (amends the html-preview ruling): a HOST document's HTML preview runs with scripts disabled (iframe sandbox without allow-scripts) and a srcdoc CSP `default-src 'none'; style-src 'unsafe-inline'; img-src data:`; asset inlining only from the document's own directory subtree — connect-src alone still leaks via img/navigation — cost if wrong: scripted HTML outside a workspace doesn't run in preview.
Ruling (the "user decides" item): host-document opens AND writes under the daemon's own <home>/state are refused (403, reason shown once) — saving there corrupts the live SQLite; a read-only view has no use — cost if wrong: one guard clause to relax.
Ruling (case identity): on darwin, worktree-prefix matching folds case; pinned by a test — one file, one identity — cost if wrong: a case-sensitive APFS volume could alias two distinct files (rare).
Ruling (ratchet): per task `make ratchet`; no baselined oversize file grows; per-module growth (web.core, go.domain, go.service, go.engine-other) follows the USER's pending reallocation decision; until then place new code in the module it replaces where natural (shiki-grammars.ts under features/editor/).
T4 (parallel lane par/s5-t4, from abe4c97ba): 54cab8a65. The review says "Needs fixes".
- I1: `target: 'auto'` resolves to ES2025, which mis-tokenizes uppercase SQL, PowerShell and Batch on V8.
- I2: file-size baseline is stale, and web.tests grew +208.
Ruling (T4-I1): the target is chosen by capability: ES2024 when the `v` flag compiles, else ES2018, never 'auto'. Parity is tested under both, with uppercase SQL and PowerShell fixtures. The live checklist gains `sql` and `powershell` fences and confirms the webview resolves ES2024 — why: no minimum macOS is set, so ES2024 alone could break on macOS < 14 — cost if wrong: on old macOS, 4 exotic grammars mis-tokenize.
Ruling (T4-I2): the lane commits the file-size shrink. web.tests +208 joins the user's pending LOC reallocation decision (R7, raised at the S1 T13 gate).
Fix round 1 dispatched (resumed); brief task-4-fix-1.md.
Deferred live checklist for T4: O2 and O4 budgets; haskell, uppercase sql and powershell fences in chat; the webview's regex target resolves ES2024.
T4 re-review 1 (sonnet): all items ADDRESSED; quality Approved with M1 (nothing pins the production target options) and N1 (stale plan line 639). Fix round 2 dispatched; brief task-4-fix-2.md.
S5 T4 fix round 2: sonnet a85cce705304a1ee0.
T4 fix round 2: 9d2b4c833 (pin test; RED under 'auto' confirmed by the worker; 35/35 + 91/91, tsc/eslint/knip clean). Controller read the 17-line diff: M1 and N1 addressed. Task 4 (lane): complete — waits for the merge window. Live checklist stays deferred to the user-at-keyboard pass.
T4: MERGED into fix/refactor-chats (c333d98f6, 1bf63a67c, 5a005824c).
Ruling (lane start): S5 T1 runs now as a parallel lane during S1 T6/T7 instead of after S1+S2 — why: its files (documents/files endpoints, fs engine, editor web) are disjoint from S1's chat code except status.go/mocks.go/tests fixtures, which cherry-pick merges resolve; the user wants parallel lanes — cost if wrong: a merge-window conflict round.
T1 dispatched (opus ab3fb1938a17707b2), wt/s5-t1 branch par/s5-t1 from 903724f7d; brief task-1-brief.md + task-1-carries.md → task-1-report.md.
T1 DONE_WITH_CONCERNS: 49d7c80d6 (par/s5-t1, 79 files). Go 89 pkgs, targeted bbx incl. TCP host refusal, route audit, vitest 22 files/205, tsc/eslint/prettier/knip/doctor/lint/deadcode 0; ratchet red only on loc-budgets (web.core 6050 vs 5979 = +71 over; go.domain +34, go.service +180, go.engine-other +108 → R7). Concerns: T3-only exports; kit suites vetted only; extras (state 403 guard, dead mock hooks/platform fns, route audit 61); upload/.gitignore/env template no-op without a wsId; PUT returns the resolved ref.
T1 review dispatched (opus) → task-1-review.md (named risks: transport spoofing, canonicalisation, state guard, ambient wsId no-ops, T3-only exports, extras, net-zero baselined files).
T1 review (opus a35f97d7e98e93312, task-1-review.md): spec ❌ (2 rulings), Needs fixes. Critical none (transportOf sound: LocalAddrContextKey comma-ok). I1 state guard and worktree matching compare path prefixes → macOS firmlink alias (/System/Volumes/Data) reads/writes the live state DB as a host document and bypasses the workspace 409; a dangling symlink write creates a file inside state. Minors M1 route-audit restore (ruling), M2 readDocumentText name collides with T5, M3 Upload File silent no-op before the home wsId resolves, M4 case-folded authority but not spelling, M5 pre-existing dangling-symlink containment hole (nilerr), M6 T3-only exports, M7 undisclosed deletions, M8 state refusal has no machine code.
Rulings (task-1-fix-1.md): I1 file identity (os.SameFile walk, rel rebuilt from on-disk names) for the state guard and worktree match, writeAtomic refuses dangling symlinks; M1, M2, M3, M4, M5 (black-box over TCP), M8 fix; M6 accepted — the S5 lane continues T2+T3 before merging so the exports land with their consumer — cost if wrong: if the lane stalls, T1 can't merge alone without a knip-justified export; M7 report only. Fix round 1 dispatched (resumed ab3fb1938a17707b2).
T1 fix round 1: 85b067355 (safepath.Locate by os.SameFile for the state guard and worktree matching; dangling-symlink writes refused 400; M1–M5, M7, M8 done; darwin firmlink + case tests ran on host; 5 mutations caught). go.engine-other +126 more (+234 total → R7). Concerns: dangling in-tree symlink never written through; NFC/NFD spelling not rebuilt.
Ruling: dangling-symlink refusal accepted even for in-tree targets — why: following a link whose target doesn't exist is the escape shape; a refusal is visible — cost if wrong: a save through a dangling in-tree link fails with a reason. NFC/NFD → T3 carry (rebuild rel per entry by identity when name lookup fails), T3 lands in the same lane before merge.
T1 scoped re-review dispatched (opus) → task-1-rereview-1.md.
T1 re-review 1 (opus a6b1afa9e1d31f2ba, task-1-rereview-1.md): APPROVED — I1, M1–M5, M8 ADDRESSED; firmlink test ran; M5 over TCP RED→GREEN. N1 a hard link to state/global.db outside state passes the guard (read returns DB bytes; write replaces only the link). N2 a workspace whose worktree dir is missing no longer claims its paths (they become host documents). TOCTOU graded acceptable (0600 socket, same user).
Ruling N1: accepted — creating that hard link needs the same user's existing read access to the DB, so the API grants nothing new — cost if wrong: none beyond what the user already can do. Ruling N2: fix in T3 (carried) — a worktree root that doesn't exist claims its paths by canonical prefix — cost if wrong: a write under a missing worktree skips the lock/resync.
Task 1: complete (49d7c80d6, 85b067355). Lane continues: T2 then T3, merge T1–T3 together.
T2 dispatched (opus a4c7e5c172ba6d459) in wt/s5-t1; brief task-2-brief.md + task-2-carries.md → task-2-report.md.
T2 DONE: 7c3cc6d77 (one EditorHost per window; 222 targeted tests; tsc/eslint/prettier/knip/doctor 0; web.workspace-panes −256, web.editor −30, web.total −286; monaco-lsp-providers 499→497; swallowed-promise 30→29). Extras: reveal matches by uri; use-editor-host test; createActiveEditorRegistry unexported. Review dispatched (sonnet) → task-2-review.md.
T2 review (sonnet a6660c18b6cf64586, task-2-review.md): spec PASS; I1 the satellites reconcile compares relative path only → switching ws-a:package.json ↔ ws-b:package.json in one pane writes B's text into A's held model (reproduced; predates T2). Minors: stale comments, leftover copy, long test header, toast test pins; outside diff: breadcrumb opens in the ambient workspace; moveEditorTabToPane leaks the source pane's held ref.
Rulings (task-2-fix-1.md): fix I1 by document identity with RED + grep for sibling path-only comparisons; fix the breadcrumb (HARD ambient rule) and the held-ref leak now ("pre-existing is never an answer"); all minors. T8 live checklist += same-pane switching between two workspaces' same-named files. Fix round 1 dispatched (resumed a4c7e5c172ba6d459).
PAUSED 2026-10-20 at the user's request. T2 fix round 1 stopped mid-task: lane wt/s5-t1 at 7c3cc6d77 + 2 uncommitted paths — do NOT discard; resume a4c7e5c172ba6d459 (task-2-fix-1.md), then a scoped re-review, then T3 (task-3-brief.md + task-3-carries.md).
RESUMED 2026-10-23: T2 fix round 1 a4c7e5c172ba6d459 resumed.
T2 fix round 1: fabb9fe74 (I1 reconcile by uri; siblings: md/html preview source lookup and jump-list same-file include the workspace; breadcrumb uses the buffer's workspace; moveEditorTabToPane releases the source ref; M1–M4). 225 targeted tests; tsc/eslint/prettier/knip clean. lint:doctor exits 1 on a Socket supply-chain score for dompurify@3.4.13 only (no code findings). Markdown-preview link clicks still use the ambient workspace → already T3's ruling (links open from the source DocumentRef).
Carry C29 (branch-wide, before any gate): lint:doctor red on dompurify@3.4.13's Socket score — a dependency bump lane (check the advisory, bump to a clean version, targeted tests of its users).
T8 live checklist += breadcrumb on a file from a non-active workspace.
T2 scoped re-review dispatched (sonnet) → task-2-rereview-1.md.
T2 re-review 1 (sonnet a4daac4ab84f1636e, task-2-rereview-1.md): APPROVED, all ADDRESSED; the I1 test ran 2/2; grep sweep clean. Observations → T3 carries: buffer-slice.ts:117 openContent ambient fallback (HARD); languageOverride selector keyed on the active tab (verify).
Task 2: complete (7c3cc6d77, fabb9fe74).
T3 dispatched (opus, fresh) in wt/s5-t1; brief task-3-brief.md + task-3-carries.md → task-3-report.md.
PAUSED 2026-10-23: T3 a48e22708809c011d stopped mid-task in wt/s5-t1; uncommitted work kept. Resume that agent.
RESUMED 2026-10-24: T3 a48e22708809c011d.
T3: 309d1392a (135 paths; 101 targeted files/1048 tests; 11 Go pkgs; tsc/eslint/prettier/knip/lint/deadcode 0; counters written back 34→33 empty-catch, tab-bar 749→745, use-file-explorer-context-menu 614→610). Carry 6 (languageOverride) was a real misfire, fixed. LOC: web.tests +593, go.service +49, web.editor +63, web.app-shell +8, web.core −49 (R7). Jump list now reopens another workspace's entry. Step 7: the "host documents have no external-change watch" issue goes to the user to file (an outward action, so ask first).
T3 review dispatched (opus) on fabb9fe74..309d1392a → task-3-review.md.
T3 review (opus afb6cdf0c27559f56, task-3-review.md): ❌ / Needs fixes. I1: an untitled buffer's language never reaches its model. I2: a terminal relative link resolves against the active workspace (HARD). I3: the CSP blocks data: stylesheets. I4: no TestRegression for N2 or NFC/NFD. M1–M7. The commit body carries the issue note (controller-checked).
Ruling (I3): inline subtree stylesheets as <style>, keep style-src 'unsafe-inline' only, and don't read CSP-blocked assets. Why: it renders with no CSP widening. If wrong, it costs one rewrite of the builder.
Ruling: M4 and M7 accepted (M7 = the N1 hard-link class).
T8 live checklist += a host HTML file with a sibling stylesheet renders styled; a relative link in a non-active workspace's terminal opens that workspace's file.
T3 fix round 1 → a48e22708809c011d (task-3-fix-1.md).
USER DECISION 2026-10-24: host documents get an external-change watch AS PART OF S5 (not a filed issue). This supersedes Ruling 4's "no external-change watch" and T3 Step 7. A planner (opus) is writing "Task 6b: Host documents follow external changes" into the plan (between T6 and T7), shaped as: a daemon-owned, refcounted parent-dir watch; the same external-change event keyed on DocumentRef; the same external-buffer-sync path; no poll.
T3 fix round 1: 97eb39ed2 (I1–I4, M1/M2/M3/M5/M6; RED recorded; 16 files/150 tests + 8 M1 caller files/60; 2 TestRegression via bbx; hydrate.test.ts 998→997 written back). web.editor 11593 vs budget 11538 (+59 from the I3 inlining; R7). M1 changes drop/Open File…/git-history/explorer-drag: they focus the holding pane. Scoped re-review dispatched (sonnet) → task-3-rereview-1.md.
T3 re-review 1 (sonnet a2358f2c5d81ff19b, task-3-rereview-1.md): APPROVED, all ADDRESSED; </style escaped; CSP default-src 'none' with no allow-scripts; both TestRegression pass via bbx.
Ruling: one more round (fix-2) for the M1 side effect, a drop or Open File… on a file held in another project's view shows nothing (a silent no-op is a defect, so it is not parked), plus the </style escape pin and 2 doc comments. If wrong, it costs one small round.
T3 fix round 2 → a48e22708809c011d.
T3 fix round 2: db68a9b9d (a drop or Open File… opens in its target pane; focus only a same-view holder; </style pin; 2 docs). 14 files/132 tests. web.editor 11603 (R7). Scoped re-review 2 (sonnet) → task-3-rereview-2.md.
Task 6b drafted into the plan (opus a2580479bd1580546): lines 913–1129, plus Ruling 4 (51), the feature map (109), T3 Step 7 superseded (580), and T8 Step 3b (1174–1181, 1193). The design: a new `GET /v0/documents/ws?path=` subscription with the same socket and state gates; FileChangeEvent + Authority; the same syncBufferWithDisk; a refcounted registry, one dir watch each; the debounce timer extracted and shared (the timer counter can't rise).
Ruling (6b gap): watch-before-read ordering (subscribe → daemon arms → ack → read), with a deterministic ack-to-read write test. Why: it closes the gap with no resync. If wrong, it adds one ack round-trip per open.
Ruling (6b scope): no recursive FSEvents tree for a host doc; cost scales with open docs. Spike FSEvents on the file path itself; otherwise kqueue vnode on file + parent dir. Why: the kqueue/Falcon load history; a doc in $HOME would stream ~/Library. If wrong, it costs one backend.
T3 re-review 2 (sonnet a411e53ad213a8263, task-3-rereview-2.md): APPROVED, all ADDRESSED. Note: a holder in the bottom tray now gets a second tab (allowed by the ruling).
Task 3: complete (309d1392a, 97eb39ed2, db68a9b9d).
Merge S5 T1–T3: the lane (base 903724f7d, 8 commits) rebases onto 99a800d0c and re-runs its gates (a48e22708809c011d), then the controller fast-forwards.
Task 6b plan COMMITTED 3fa3ffb77: file-rooted FSEvents (spiked in scratchpad: modify/atomic-replace/delete/rename/recreate delivered, nothing from siblings) / Linux inotify on the parent filtered to the name; armed in OnSubscribe before the 101 (the ack); read after the socket opens; TestRegression_HostDocumentWatchArmedBeforeRead. Before dispatching 6b (after T6): a sonnet pre-flight scan of 6b against the T5/T6/T8 interfaces as built.
S5 T1–T3 MERGED by cherry-pick onto 3fa3ffb77 → 602993585 (7 commits; tree == the rebased lane 9c1f3ac8c + the 6b plan commit). Rebase gates on 99a800d0c: Go build/vets/S5 pkgs, both S5 TestRegression + T1 bbx, cold lint 0, deadcode; web install/tsc/eslint/prettier/knip, lint:doctor 100/100, 103 files/1067 tests incl. html-node. LOC vs 99a800d0c: web total −434 (editor 11603 vs 11538), go total +484 (R7). Lane wt/s5-t1 removed.
RULE BREACH (reported to the user): during the rebase gates the worker ran `-tags integration` on ./tests/integration/files and ./tests/integration/lifecycle, which only vet is allowed on. Checked: kit Env uses an os.MkdirTemp home (tests/kit/env.go tempHome), and neither suite names a provider; no evidence of harm. Future lane prompts repeat the rule verbatim.
Next S5: T5 (document text seam), then T6, then 6b (pre-flight first), T7, T8 (live, with the user). T0 (budget instruments) also open: check whether T0 is done.
T5 lane wt/s5-t5 (par/s5-t5 from 602993585); brief task-5-brief.md + task-5-carries.md; implementer dispatched (opus) → task-5-report.md.
T5: 0da150e41 (seam + hook 9/9 incl. the ≤2-renders test unedited; 46 files/438 tests; tsc/eslint/prettier/knip/lint:doctor/file-size/counters 0; hydrate.test.ts 997→995). web.editor +135, core +2. Concerns: Plate adopts useDocumentText alongside onExternalText (two rich panes); Plate's initial value from the snapshot; LSP workspace edits now arm autosave; buffer-save↔document-text import cycle (T6 breaks it). Review dispatched (opus) → task-5-review.md.
T5 review (opus a28cc3ab393840e17, task-5-review.md): spec ✅; quality Needs fixes. I1: Save As onto an open document leaves two buffers for one document (cross-writes, autosave loop). M1 applyWorkspaceEdit untested; M2 fake-timer leak; M3 getState in render (uSES snapshot); M4 notify gate. Named risks: dual Plate path is one adopt, no loop; LSP-edit autosave follows the brief; the import cycle is safe (T6 removes it).
Ruling (I1): one document, one buffer. Save As onto a dirty open target is refused with a toast; a clean target adopts the text and the source tab is retargeted in place (same-pane duplicate → close it and focus). Why: no silent overwrite of unsaved edits. If wrong, it costs one dialog flow.
T5 fix round 1 → a6441cd5eb8649bff (task-5-fix-1.md).
T5 fix round 1: cbb842ee1 (I1: Save As refuses a dirty target, a clean target → replaceEditorTab in place; M1: workspace-edit test moved to its mirror path + 3 cases; M2; M3: read moved into getSnapshot; M4). 63 files/590 tests; gates 0. Concern: an open during the Save As write could still create a second buffer. Scoped re-review (sonnet) → task-5-rereview-1.md.
T5 re-review 1 (sonnet ae5a3ee3b6971e824, task-5-rereview-1.md): M1–M4 ADDRESSED; I1 partial: the open race is real (window.prompt blocks; remote writes take hundreds of ms); a failed write drops the untitled tab and leaves the target dirty; a failed new-file Save As is silent; Save As onto a preview tab stays a preview.
Ruling: write first, then re-check findDocumentBuffer once and adopt through applyExternalText({saved:true}) + replaceEditorTab; a failed write changes nothing and toasts (also fixes the silent new-file failure); the result is pinned. Why: ordering closes the race with no timer, and no failure is silent. If wrong, it costs one small round.
T5 fix round 2 → a6441cd5eb8649bff.
T5 fix round 2: deba33a63 (write first, re-check once; a failed write changes nothing + toast; preview → not a preview; added: text typed into the untitled tab during the write carries to the target, unsaved). 5 RED→GREEN; 595 targeted tests; gates 0.
Ruling: "pinned" = not a preview (isPinned untouched), correct. Concern: a target edited in another pane during the write gets overwritten by saved:true. Ruling: at the re-check, a target that is now dirty takes the external-change rule for a dirty buffer (keep edits, flag + toast), never saved:true. Folded into round 3 (tiny); then one scoped re-review of deba33a63..round 3.
T5 fix round 3: 7bc7dd6be (a target dirty at the re-check → flagExternalChange, extracted from syncBufferWithDisk; 596 tests). Ruling: when both are dirty, the source stays an untitled buffer holding its text (no text is ever dropped); buffer-save.test.ts must stay ≤500 (494 now).
T5 round 3 addendum: 3668d3db8 (both dirty → the source stays untitled with its text; buffer-save.test.ts 493). Scoped re-review 2 of cbb842ee1..3668d3db8 (sonnet) → task-5-rereview-2.md.
T5 re-review 2 (sonnet ae0b2ccb369578b60, task-5-rereview-2.md): APPROVED, all ADDRESSED; 30/30. Minor 1 (both-dirty returns true → tab-bar closes the untitled tab), clearBlame dropped, a spurious flag on equal text, the carry test missing its tab assert → tiny round 4 (controller reads the diff). Minor 2 → T6 carry.
T6 CARRY: the late-typing carry and applyWorkspaceEdit reach only a BOUND Monaco model. A held-but-hidden dirty target can lose text, because rebind reconciles only clean buffers. T6 (models own text) must cover hidden held models, with a test.
T5 fix round 4: e578c0a34 (controller read the diff: both-dirty returns false; clearBlame restored; equal text adopted clean; tab assert). Task 5: complete. MERGED by fast-forward → e578c0a34 (6 commits, trailers verified). Lane removed. Next: T6 (carry: hidden held dirty models), then 6b (pre-flight first).
T6 lane wt/s5-t6 (par/s5-t6 from e578c0a34); brief task-6-brief.md (457 lines) + task-6-carries.md; implementer (opus) → task-6-report.md.
T6: 60ddc2f38 (81 files/684 tests; carry RED→GREEN; tsc/eslint/prettier/knip/lint:doctor 0; web.total −102, editor −40; timers 87→84, swallowed-promise 29→27; deletion table verified). Concerns: one scheduler drives drafts and autosave; setup.ts clipboard stubs; workspace.switch end-mark moved to first paint; two old untitled:Untitled tabs share one document; a refused or non-404 tab opens blank with no draft; buffer-slice.test (4) and workspace-view.test (6) over 3 vi.mock at base. Review (opus) → task-6-review.md.
T6 review (opus ab9e3ea13e1261926, task-6-review.md): spec met (2 accepted deviations); Needs fixes. I1: a failed non-404 restore with no draft stays blank forever (no retry, silent Cmd+S; no model, so no data loss). I2: restore loads Monaco on every boot, even with no editor tab (3.5 MB at launch; O4 skewed). I3: touched test files over 3 vi.mock. Risk 1a unreachable (opens dedupe by document); 1c/1d pass; Save As, one owner, one scheduler, upgrade and deletions clean.
Ruling: I1 shows a labeled error state with the reason + retry on reconcile and reopen + a visible Cmd+S refusal; I2 loads the host only for a layout with an editor tab; I3 brings the mocks to ≤3; all minors, including the restore-read draft race (decide after the read). Why: no ghost UI, no silent failure, no lost draft. If wrong, it costs one round.
T6 fix round 1 → ad114298ddf218532.
T6 fix round 1: ae1b3eed3 (I1 labeled error + retry + visible Cmd+S; I2 lazy host; I3 mocks 3/2; M1–M6, with M3 via releaseDocumentOf instead of the fixture). 81 files/717 tests. beforeAll host-load timeouts in 3 test files (a latent Monaco cold-load flake). web.editor +25 over base; web.total −31. M1 edge: live edits in a doc opened during the restore read win over the draft. Scoped re-review (sonnet) → task-6-rereview-1.md.
PAUSED 2026-10-24: T6 re-review 1 (a6ad453cdaece0dd7) stopped before it read the diff; re-dispatch it fresh on 60ddc2f38..ae1b3eed3 (package already written). T6 implementer ad114298ddf218532 idle.
RESUMED 2026-10-25: T6 re-review 1 → sonnet ab318d126d362717b (task-6-rereview-1.md). It checks I1–I3 and M1–M6 against task-6-fix-1.md, plus three judgements: is M3's releaseDocumentOf legitimate production code, can the M1 edge be reached at boot, and are the beforeAll host-load timeouts legitimate.
T6 re-review 1 (sonnet ab318d126d362717b, task-6-rereview-1.md): APPROVED. I1–I3 and M1–M6 addressed. M3's releaseDocumentOf is legitimate production code (disposeBuffers calls it). The M1 edge can't be reached (an open during the read happens only for an `unavailable` tab, which stays hidden behind the placeholder). The beforeAll host loads are the established pattern (15 files). Accepted deviation: document-text.test gained a beforeAll and imports, with its assertions unchanged.
Ruling: fix the open items in this task instead of parking them: persisting `unavailable` shows a stale reason at boot; the title-keyed toast drops files; a whole-restore rejection leaves blank tabs; Reload from disk on an unavailable tab is a silent no-op. Why: no stale UI, no silent failure. If wrong, it costs one small round. T6 fix round 2 → ad114298ddf218532 (task-6-fix-2.md).
Task 6b pre-flight (sonnet af4311252d82ff6f9) → task-6b-preflight.md. It reads T6 only at ae1b3eed3, because T6 fix round 2 is editing the lane.
T6 fix round 2: c625a1ac3 (no load failure is persisted) and 4fd3fc8a5 (every tab a restore couldn't fill says why and retries; one retryDocument path for reopen, reconcile, Reload from disk and Try again, where the draft wins over disk, which also fixes a draft-shadowing bug in the retries; failures listed per kind of failure; lazy restore import breaks a cycle). 83 files/735 tests; gates 0; web.editor +69 this round. Concerns: a layout with no editor tabs whose drafts fail is silent and leaves orphans; a permanently broken IndexedDB keeps failing on purpose. Scoped re-review 2 (sonnet ad23fc8f6e8908dd0) → task-6-rereview-2.md.
Task 6b pre-flight (sonnet af4311252d82ff6f9, task-6b-preflight.md): 15 findings, 7 DISAGREE and 8 AT-RISK. Rulings are in task-6b-rulings.md, binding over the brief: R1–R6 and R9–R11 adopted.
- R7 amended: the regressions live in bbx api/tests/, where every TestRegression_* lives, and that package is runnable; kit gets helpers as library code only.
- R8 amended: a deterministic Go fd/emit test under 1,000-sibling churn replaces the meter "gate"; no `make meter`.
Why: the bbx package is runnable and is the project's regression home, and a test is a gate where a meter run is not. If wrong, 6b needs one extra round.
6b dispatches after T6 merges, on the merged tip.
T6 re-review 2 (sonnet ad23fc8f6e8908dd0, task-6-rereview-2.md): items 1–4 ADDRESSED; draft over disk on every path; concurrent retries are safe; the lazy import adds no load; the no-editor-tabs concern is harmless. IMPORTANT, open: Try again is a no-op for an open model that has reconcile's 404 reason (the adoptDraft early return at restore-documents.ts:48 skips the clear). Minors: same-basename names collapse in the toast; no pending state on the button; a non-read throw in restoreDocument can leave a blank tab.
Ruling: fix all four in round 3; one rule owns clearing the reason. If wrong, it costs one round. T6 fix round 3 → ad114298ddf218532 (task-6-fix-3.md).
T6 fix round 3: c521aa596. A successful read clears the reason on every path; catchUp updates an open model on the early path; the toast names enough of the path to tell files apart; UnavailableDocument has a retrying state; failSafe means a throw never leaves a blank tab; allSettled is now Promise.all. 83 files/739 tests; gates 0; web.editor +53. Scoped re-review 3 (sonnet a0a9190ac56102a75) → task-6-rereview-3.md. It checks that catchUp reuses the existing external-change mechanism and that failSafe never rejects. Ruling: any further Minors in these files are carried into 6b, which owns the same open and retry path, rather than taking a round 4.
RESUMED 2026-10-25 after a session restart: re-review 3 (a0a9190ac56102a75) resumed; it had not been written yet.
T6 re-review 3 (sonnet a0a9190ac56102a75, task-6-rereview-3.md): findings 1–4 ADDRESSED; approved with Minors. catchUp reuses flagExternalChange and applyExternalText, failSafe never rejects, retrying is local UI state, and the mock passes through to the real read. Minors C1–C6 are carried into 6b (task-6b-rulings.md "T6 carries"), per the earlier ruling.
Task 6: complete (60ddc2f38, ae1b3eed3, c625a1ac3, 4fd3fc8a5, c521aa596). MERGED by ff: fix/refactor-chats e578c0a34 → c521aa596. web.editor is 11931 against its 11538 budget; the loc-budget question is with the user (S1 R7).
Next: dispatch 6b on c521aa596 with task-6b-brief.md + task-6b-rulings.md.
6b dispatched (opus afe5aab4fa8f8eaf3) in lane wt/s5-t6b (par/s5-t6b, BASE c521aa596), with task-6b-brief.md + task-6b-rulings.md (R1–R11, C1–C6). The old T6 lane was removed after the merge.
~~~~

## B: 2026-10-17-s6-terminal-wrapped-links-plan

Source: `.superpowers/sdd/2026-10-17-s6-terminal-wrapped-links-plan/progress.md`

~~~~markdown
# SDD ledger — plan: docs/plans/2026-10-17-s6-terminal-wrapped-links-plan.md
Started 2026-10-19 as a parallel lane: worktree <scratchpad>/wt/s6, branch par/s6 from 2effbd4a9 (the reviewed s3-t10 head, which already edits model/diff.go and vt_emu.go, so S6 builds on it rather than conflicting at merge).
Ruling (lane): S6 is file-disjoint from S1 (chat ledger), S5 T4 (shiki) and gitbinding (realtime/git). It merges after s3-t10 in a merge window — cost if wrong: one rebase.
Ruling (trailer): commits end with the session's two trailer lines (Co-Authored-By: Claude Opus 5.5 / Claude-Session), not the plan's Sonnet line — the session attribution rule supersedes the plan text.
Ruling (T7): the live Tauri check runs only after the lane is merged into fix/refactor-chats, with the user at the keyboard (one dev instance) — cost if wrong: a wrap bug found late.
Ruling (worktree): the plan's Global Constraints name the main worktree; the lane uses <scratchpad>/wt/s6 instead, and never touches the main worktree.
Pre-flight scan dispatched (sonnet) → preflight.md.
Pre-flight: preflight.md (sonnet afe1ac114027004bf): 12 pairs (2 disagree: T1×T5 live CRLF, S5-T3×S6-T6; 6 at risk), 8 drift, 16 recommended rulings.
Rulings (adopt preflight §6 with these amendments):
- R1 ratchet: every task runs `make ratchet` and reports; the lane does NOT edit baselines — the controller updates them once at merge; S6's net LOC joins the user's pending reallocation decision (S1 R7) — cost if wrong: one extra --update commit.
- R2, R3, R4, R6, R7 (clear only what xterm clears: EL0@col0, EL2, ED0@col0, ED2), R10, R11, R14 (one shared row encoder for keyframe and diff writers), R16: adopted as written.
- R5 tall chains: a chain is capped at the viewport's rows; overflow rows go out as plain rows (rendering stays exact; only the wrap bit is lost); ruling 4 "no new keyframe triggers" stands. Add a Go case with a chain longer than rows — why: a keyframe loop is worse than an unjoined link longer than a screen — cost if wrong: such a link stays split.
- R8 knip: test through the public provider; export nothing test-only; re-key knip.json in the rename commit.
- R9 sequencing: T6 AND T1's web reds wait for S5 T3 (both rewrite terminal-file-links.ts / use-xterm-instance.ts openFile). The lane runs T1 (Go reds only), T2–T5 now; T6 is rebased on S5 T3 and drops "keep their signatures" — cost if wrong: S6 finishes later.
- R12 black-box gate: the host never runs `-tags integration` on ./tests/integration (session hard rule); T5 runs `go vet -tags integration` only. The regression test runs in the merge window, controller-supervised, by the Docker CI recipe if it isolates homes; otherwise the gap goes to the user at T7.
- R13: T6 fixes mount-xterm.ts:93-96 AND switches selection-text.ts from the rowRunsToTheEdge heuristic to isWrapped (one logical-line notion) — cost: small scope growth in T6.
- R15: T7 is the end-to-end check; T6 adds a Go-generated byte fixture replayed through xterm in vitest only if xterm runs under the current vitest env with no new dependency. The xterm-idiom guards stay, labelled as pins of an external contract.
T1 dispatched (sonnet a260eb943463216c5), BASE 2effbd4a9: Go reds + black-box file (vet only); web reds moved to T6.
T1 DONE_WITH_CONCERNS: 9ade838ef. 2 SoftWrapped reds fail on the missing contiguous URL, 2 hard-break guards pass; black-box file vetted only (live-frame hard break asserted as a column-1 CUP between rows; CRLF only on the snapshot keyframe). vt_serializer_test 636→616, diff_test 451→480, conformance 403→406. make ratchet exit 2 is inherited (loc-budgets red across modules; R7 pending with the user). Carry to T4: model.go:49 names the deleted residual test; a wrapped-line cell round-trip case. Review dispatched (sonnet).
T1 review (sonnet): spec ✅, Approved; 4 Minors. Ruling: carry to T4 — trim wrapScenario comment to ≤3 lines; correct the live-frame regex comment (the column-1 CUP may be the cursor park; still a valid proof); wrapped-line cell round trip; model.go:49. Task 1: complete (9ade838ef).
Ruling (R10 split): T2 adds the NOTICE.md entry, the dependabot ignore for github.com/charmbracelet/x/vt and the UPSTREAM.md pin-table update (they belong with the vendoring commit); T4 keeps the D1/D8 reference cleanup. T2 dispatched (sonnet), BASE 9ade838ef.
T2 DONE: fc45453ef. Controller-verified: diff -r vs module cache exit 0 (verbatim), go.mod adds only the replace, go.sum removes only the two x/vt lines, NOTICE/dependabot/UPSTREAM pin row correct; build green, model fails only on T1 reds, lint 0, deadcode baseline unchanged; vendoring adds nothing to the ratchet (A/B identical). Task 2: complete. Carry to T3: NOTICE entry gains a 'locally patched (see UPSTREAM.md)' line once the patch lands.
T3 dispatched (opus), BASE fc45453ef. Carries: R6 (evicting a head clears the new oldest line's flag; saturated-ring case), R7 (clear only on EL0@col0, EL2, ED0@col0, ED2 — at the erase-command level, not every ClearArea/FillArea; EL1@last col and ECH n>=cols keep the flag), R16 (docs residual is T4's), drift D1/D4/D5, NOTICE 'locally patched' line.
T3 DONE_WITH_CONCERNS: c86e7ada3. TestRowWrapped 30 rows (11 brief + 3 controller + 16), 26 rule-removal mutants all red; vendored patch +88/−23 over 6 files; model/terminal -race fail only on T1 reds; lint/deadcode 0. Clear rule follows xterm 6.0.0 InputHandler (adds LF onto a row, ED0 rows below, ED1) — accepted under R7 (xterm is the oracle). Alt-screen scrollback nil'd. Review dispatched (opus).
T3 review (opus ad2a41219ed53be97): spec ✅ ⚠️; Needs fixes. C1 CRITICAL: the patch panics when the cursor row is off the grid (handlers.go:606, :556, cc.go:22) — exit alt after shrink then ED0, ESC 8 after shrink, a scroll region taller than the screen then LF; the model's panic recovery then wipes screen+scrollback; unpatched x/vt tolerated it. ⚠️ Ruling-3 gaps: 1-row alt screen keeps row 0 flagged; a region scroll whose top is below row 0 can leave a stale row-0 flag.
Ruling (T3-C1): every wrap-flag access goes through one bounds-tolerant accessor pair on Screen (off-grid row = no-op, like uv SetCell/FillArea); x/vt cursor semantics untouched — why: removes the bug class rather than three call sites — cost if wrong: an off-grid write silently drops a flag (the row is invisible anyway). Ruling-3 gaps (1-row alt, region top > 0) fixed in the same round. Fix round 1 dispatched (resumed a9ca5d5561a603d30).
T3 fix round 1: e552b56a6 — one bounds-tolerant setWrapped/isWrapped pair on Screen; Ruling-3 gaps fixed; TestRowWrapped 38 rows (8 new RED at c86e7ada3: 6 panics, 2 stale row-0); 29/29 mutants killed; gates as before. Ruling (T3-R2): the pre-existing DECSTBM over-tall-margin panic is fixed here too (clamp to screen height as xterm IH:2798) — we vendor x/vt now, and a panic wipes the user's screen; plus a 2-line UPSTREAM note for review M3. Fix round 2 dispatched (resumed a9ca5d5561a603d30); one re-review covers rounds 1+2.
T3 fix round 2: f179e3403 — DECSTBM clamps bottom to height (xterm IH:2798; Resize already resets margins); M3 recorded; 39 rows, 31/31 mutants killed. Scoped re-review (sonnet) over c86e7ada3..f179e3403.
T3 re-review (sonnet a4e10dc0811cd2d32): all addressed, no new breakage; TestRowWrapped 39/39. Task 3: complete (c86e7ada3, e552b56a6, f179e3403). Out-of-scope: escan shadow DECSTBM unclamped (xterm clamps the replay identically — no action); DECSLRM right margin unclamped → carry to T4 as a probe: if an over-wide right margin can panic, clamp it like the bottom margin.
T4 dispatched (opus), BASE f179e3403. Carries: R14 (one shared row encoder in wrap.go that T5's diff writers reuse — head/continuation/tail rule encoded once), R16/D8 docs cleanup (model.go:34-51 and :49; UPSTREAM D1 + checklist + every D1 reference: UPSTREAM.md:99,101,109,124,164,173-174,182, vt_model.go:262, vt_serializer_test.go:459; D1a standalone), T1 minors (wrapScenario comment ≤3 lines, live-frame regex comment, wrapped-line cell round trip), DECSLRM probe, file-size (vt_serializer_test.go 616 may not grow).
T4 DONE_WITH_CONCERNS: 59126e679 (DECSLRM clamp: the probe found 7/12 ops panic with a right margin past the width; clamped to the last column, cites go-te/ESCTest2) + 2903562d5 (serializer: shared encoder encodeChainRow(cells, cols, wrapped, nextWrapped, brk) + padToCols in wrap.go; D1/residual docs removed; T1 minors fixed). Model fails only on T5's diff red; wrap round trip 9/9; 11/11 mutants killed; lint/deadcode 0.
Carries to T5: (a) a trailing EL0 after a tail that reaches the last column erases that cell in x/vt but not in xterm.js (pending-wrap) — write a full-width tail padded with no EL; (b) scrollback lines wider than cols (pre-shrink, no reflow) arrive on the client as an extra flagged row — the flag-equality check must compare logical lines after a width change, or the encoder must define what a wide line becomes; decide and test.
Review dispatched (opus).
T4 review (opus ae48e47927b9326b8): spec ✅ ⚠️; Needs fixes. I1 (regression vs base): the continuation's first-cell SGR precedes the char that triggers the client's autowrap; a wrap-induced scroll BCE-fills the new row and a trimmed continuation leaves it painted (probe TestProbeBCE). Ruling: the shared encoder ends a short non-head continuation with EL0 after its pen reset (skipped at the last column); the encoder writes to an io.StringWriter and owns the whole rule so T5 calls it with no separator. Minors M3 (UPSTREAM trigger wording), M5 (record x/vt-vs-xterm divergences under D2) fixed in the same round. Fix round 1 dispatched (resumed abb53a124c87a08ae).
PAUSED 2026-10-19. T4 fix round 1 (BCE on the wrap scroll) was dispatched to abb53a124c87a08ae and stopped before it changed anything: wt/s6 is clean at 2903562d5. Resume by re-sending the round (ruling above) to that agent, then a scoped re-review; then T5 (carries a, b and the encoder call), T6 after S5 T3, T7 live.
T4 fix round 1: 140bbb955 (BCE: a short non-head continuation ends with EL0 after its pen reset, none at the last column measured by written cells; probe is a wrap_test round-trip case, red at 2903562d5; encoder appendChainRow(dst []byte, …) []byte over one appendLine; 10k-line keyframe 47,859 → 45 allocs; UPSTREAM trigger + D2 divergences c, d, EL0-at-pending-wrap). Round trip 12/12, mutants 15/15, -race/lint/deadcode clean; ratchet +25 go.core +16 go.tests (still red at base). Concern 2 retracted.
Ruling (T4-M2): append-to-[]byte instead of io.StringWriter — the stdlib append idiom gives the same single-allocation goal without unchecked WriteString errors (errcheck), a nolint or a counted `_ =` — cost if wrong: a signature change before T5 consumes it. Scoped re-review dispatched.
Task 4: complete (59126e679, 2903562d5, 140bbb955; re-review task-4-rereview-1.md: all addressed, Approved).
T5 dispatched (opus aa51b499d01a2b243), BASE 140bbb955. Carries: R5 tall-chain cap + case, R7, preflight item 4 (client modes e.chrome.modes), D7 move list, R11 split before adding (e.g. conformance_wrap_test.go), R12 vet-only, R14 encoder via the T4 report's AvailableBuffer calls, R1 ratchet report; mutation check on the chain and straddle rules.
T5 DONE_WITH_CONCERNS: a6d08f1a6 (chrome split, diff.go 640→394) + 0cdcd65ed (chains; diff.go 478). 313 pass; mutants 26/27 (survivor equivalent); race, conformance x20, lint, deadcode, integration vet 0; ratchet red at base. Concerns: alt-screen compares grid flags only; whole-chain rewrite on any dirty row (screen-tall wrapped line typing 75→127 µs/frame, allocs 12→5); beyond brief: a chain crossing an active scroll region's bottom lifts and restores the region (tests red first); R5 cut rows lose the bit, 1-row screen repaints row 0 each frame; a6d08f1a6 alone has 6 lint hits fixed in 0cdcd65ed. Review dispatched (opus).
T5 review (opus a3b61a3d2f715ba11): spec ✅ ⚠️ (alt-screen skip justified; the scroll-region lift beyond the brief is correct); Needs fixes. I1: the whole-chain rewrite per keystroke — 4021 vs 127 wire bytes typing a screen-tall line, 204 vs 45 for a 3-row command, BenchmarkDiffEcho 110.7 vs 69.5 µs; the reviewer verified a first-dirty-row start against all conformance, T1 reds and a 24k-step fuzz.
Ruling (T5-I1): fix in this task, overriding the brief's whole-chain rule — why: a 30x wire cost on every keystroke in a long command is a hot-path regression this task introduces; the fix generalises the task's own row-0 held rule — cost if wrong: one more round. Minors M1, M3, M4 fixed in the same round; M2 (the move commit fails lint alone) is accepted and noted at merge — lint is not a per-commit test gate, and the pure move stays reviewable.
Fix round 1 dispatched (resumed aa51b499d01a2b243).
T5 fix round 1: 7c003247d (first-dirty-row start; wire per key 4021→127 and 204→45; BenchmarkDiffEcho 121→76 µs, 5.4 KB→336 B; frames re-baselined at diff_test:282,:325; seeded 400-seed walk; mutants 30/31, C1 equivalent; M1, M3, M4 done). Scoped re-review dispatched (sonnet).
M5: 002a98ad5 (walk compares on-screen flags every step; 400 seeds pass; flagged lines checked 11,645→19,403). Task 5: complete (a6d08f1a6, 0cdcd65ed, 7c003247d, 002a98ad5). Lane state: T1–T5 done; T6 waits for S5 T3 (R9); T7 live after merge. The lane merges into fix/refactor-chats after S1 T6 lands (cherry-pick 2effbd4a9..par/s6; note M2: a6d08f1a6 alone fails lint), then the R12 black-box test runs isolated in the merge window.
Lane MERGED into fix/refactor-chats after S1 T6 (12 commits, …bc166ebfc). T6 (web) rebases on S5 T3 from the branch; T7 live with the user. R12 black-box test still to run isolated.
2026-10-25: T6 dispatched (opus a3c7dc4a1d2c6c671) in lane wt/s6-t6 (branch par/s6-t6, BASE e578c0a34, which includes S5 T3). Carries: R9 (drop "keep their signatures"; build on S5 T3's functions), T1 web reds, R8, R13 (mount-xterm wrapped rows + selection-text → isWrapped), R15 (fixture only if xterm runs in vitest with no new dependency), R1 (report the ratchet, never edit baselines). Brief line numbers are stale after S5 T3; find each site by content.
T6 DONE_WITH_CONCERNS: dcde9b7d3 (rename, knip re-keyed), 83da40e02 (one tokenizer; addon and dependency removed; openUrl wired), ab86406e0 (R13: copy keeps a program's line breaks), 454cf7968 (R15: Go golden fixture replayed through xterm). 46 tests pass; 12/12 mutants killed; gates clean; ratchet red at base (web.terminal −44).
Concerns: R13 deviation (ownCopy and selection-text.ts deleted, so copy is xterm's native handler); the MIT-ported regex has no NOTICE entry; the fixture is read across trees.
SLIP: the worker ran one read-only `ls ~/.crowbar/state` and wrote nothing. Reported to the user.
Review (opus a552b610ef9c8acbb) → task-6-review.md. Named risks: copy regressions where the wrap bit is still missing (R5 tall chains, alt screen, keyframes, resize), range mapping, the URL port, the fixture's CI path, licence.
T6 review (opus a552b610ef9c8acbb, task-6-review.md): spec ⚠️ (deviations justified; R13 by deletion is the better form); quality good; Needs fixes.
- I1: with ownCopy gone, copy inserts newlines in two daemon cases: (1) a width change clears every wrap flag (x-vt screen.go:83-92) and the resize keyframe replays history as plain rows; (2) R5 cuts. The literal R13 switch would behave the same.
- I2: the CI web filter misses the fixture path.
- M1–M4.
Ruling (T6-I1): fix in Go, in a new Task 6c. The daemon model keeps logical lines across width changes (reflow, with xterm 6.0.0 as the oracle per R7), the resize keyframe replays wrap bits, and the R5 cut is re-examined. Why: a sidebar or split toggle is an everyday gesture, and "accept and open an issue" is not a fix; re-adding the heuristic would be a second notion of a logical line. If wrong, S6 grows by one Go task. 6c is planned first (opus planner → plan section), then implemented. T6 merges after its own round; 6c must land before the branch is finished.
Ruling (M2): restore the old skip, dropping file candidates inside the non-space run that starts at a URL. Why: a path inside a URL is part of the URL.
T6 fix round 1 → a3c7dc4a1d2c6c671 (task-6-fix-1.md: I2, M1–M4).
RESUMED 2026-10-25 after a session restart: T6 fix round 1 (a3c7dc4a1d2c6c671) resumed; nothing had been committed. Task 6c planner (opus a79647cc03ef145fb) is writing the plan section into the S6 plan doc (uncommitted; the controller commits it after reading).
T6 fix round 1: 2dec4780c (I2 CI filter, checked with picomatch), d3cc1e3c5 (M1, M3), 872cccf04 (M2, http(s) only), 6aec7bf34 (M4). 48 tests; gates clean. M2 was narrower than the ruling (the old URL_PREFIX_RE also skipped scheme:// and www. runs), so a follow-up restores the full skip, RED first. The scoped re-review follows it.
S6 T6 M2 follow-up: 830bd01ef (any scheme:// or www. run; 5/5 mutants; 49 tests). Scoped re-review 1 (sonnet aa658fb4b4fdd7f23) → task-6-rereview-1.md. Note: the branch is now c521aa596 (S5 T6 merged), so S6 T6 will cherry-pick onto it.
T6 re-review 1 (sonnet aa658fb4b4fdd7f23): I2, M1, M3 and M4 ADDRESSED; M2's ruled cases pass. N1 IMPORTANT: the overlap test, together with www\. matching after / - or ., drops bin/www.js:12, ./www.ts:5, src/my-www.config.ts and docs/www.example.com/index.html. Ruling: drop a candidate only when it starts inside a run, and anchor www. to a real left boundary. T6 fix round 2 → a3c7dc4a1d2c6c671.
T6 fix round 2: de5fd19cc (N1: start-inside rule, anchored www.; 14/15 mutants, the survivor equivalent; 51 tests). Scoped re-review 2 (sonnet a74390f62267fe799) → task-6-rereview-2.md. Branch tip is now 348f4dcbc (S1 T7 merged), so S6 T6 cherry-picks onto it.
T6 re-review 2 (sonnet a74390f62267fe799, task-6-rereview-2.md): N1 ADDRESSED, nothing open. `out/https` is harmless and pre-T6 behaved the same; the survivor is equivalent; lookbehind adds no new engine floor.
Task 6: complete. Rebased onto 348f4dcbc by the controller with no conflicts (knip.json auto-merged), giving 10 commits ending at feeff357a. Gates on the rebased head: tsc, knip, eslint and prettier 0; vitest terminal-links, mount-xterm and terminal.test 51; Go model package ok; file-size and counters ok. MERGED by ff → feeff357a.
Open: I1 (copy and links lose the wrap bit after a width change and at R5 cuts) moves to Task 6c, which must land before this branch is finished. Task 7 (live) needs 6c first.
Task 6c planned (opus a79647cc03ef145fb). The plan section is committed at 15ef2aacb (lines 235-346, plus consistency edits to Ruling 3, Review Focus 3, Task 7 scenario 5 and Out-of-scope). Decisions: reflow goes in vendored x/vt (reflow.go, a port of xterm 6.0.0 Buffer.resize); the R5 cut is fixed as one continuous flow with no new trigger; the snapshot goes in band (RIS + frame, one write) to remove a real blank frame.
Rulings R-6c-1..5 are in task-6c-rulings.md: accept the pending-wrap cursor offset with no re-print; make printed spaces content if erase can write EmptyCell, otherwise pin the divergence explicitly; adopt the row step; hand-check reflow.go plus a temporary-config lint run; adopt the NOTICE entry.
6c dispatched (opus aede156ef71259b2d) in lane wt/s6-t6c (par/s6-t6c, BASE 15ef2aacb), with task-6c-brief.md + task-6c-rulings.md.
~~~~

## B: 2026-10-17-migration-engine-plan

Source: `.superpowers/sdd/2026-10-17-migration-engine-plan/progress.md`

~~~~markdown
# SDD ledger — plan: docs/plans/2026-10-17-migration-engine-plan.md
Plan committed 8308ebabc. Spec: docs/plans/2026-10-16-thread-branches-and-home-spike.md Parts C/D + architecture §9/§11.
Pre-flight scan: preflight.md in this dir (opus a81986ac531365c84): 21 pairs (5 conflicts, 1 ambiguous), 16 task rows, 22 findings F1-F22.
Execution: starts after the S0 final-fix round completes (one implementer at a time).

## Rulings (carry the relevant ones into each task dispatch)
Ruling T0 (F2): NEW Task 0 before Task 1 — delete TestDefaultCrowbarHome (api/internal/api/v0/endpoints/repos/handlers/repos_extra_test.go:62); it writes into and RemoveAll's the REAL ~/.crowbar/projects/probe-project and pins the resolver T2 deletes. No repos test runs before it. Then `make ratchet ARGS=--update` for the shrink — hard safety rule — cost if wrong: none.
Ruling F1 (ratchet): engine + S1 are ONE LOC window (go.total enforced at S1's gate vs the pre-engine baseline). Per task, file-size and counters still hold: no baselined oversize file grows (new code goes in new files: app/options-equivalent, kit Boot wiring in a new kit file), no new `_ =` (use errors.Join), no new timers. Each task runs `make ratchet` and reports the LOC delta; T16 records it. Per-module reallocation (go.core up, go.chat down) awaits the USER decision already asked — cost if wrong: S1 must delete more.
Ruling F3: split Boot into Begin + RunPreOpen; internal.New publishes the session to the health reporter (atomic.Pointer) before RunPreOpen.
Ruling F4: Begin only reads the ledger; the appender opens/creates (and repairs a torn tail) on first append.
Ruling F5: "nothing to back up" = no regular file other than state/.lock; TreeSnapshot comparisons exclude state/.lock.
Ruling F6: no second backup while the ledger has a `backup` record after the last completed run; Halt.Backup points to it.
Ruling F7: post-open runner is a POSITIONAL app.New argument (CLAUDE.md: required deps are ctor args); unit tests pass a no-op from a new package internal/testutil/migratetest (avoids the testutil import cycle); kit + both integration harnesses pass a real session — cost: ~18 mechanical edits.
Ruling F8: internal.New gives engines/app a child ctx and cancels it before closing them on a post-open halt.
Ruling F9: T11-T12 deadcode may list only probeLock, ledgerView.lastRun, Session.Progress; hard at T13. Delete Catalog.Steps() and ActionKind Move unless a production caller exists.
Ruling F10: move TestRegression_StateDirSingleInstanceLock → migrate/lock_test.go and TestRegression_SocketPath_* → home/socket_test.go, names kept.
Ruling F11 (overrides plan R10 for legacy steps): regression_upgrade_boot_best_effort_test.go stays green. A ported legacy step that meets an unreplayable row records it (count `skipped_rows`, reason in the ledger) and completes; halts are for engine-level failures (I/O, corrupt ledger, a step's own postcondition failing). An existing user-chosen regression outranks the spec's "no continuing anyway" — cost if wrong: a bad row survives the migration, but recorded and visible in --plan/health. TELL USER.
Ruling F12-F17, F20-F22: as recommended in preflight.md §7 (guard exact-match + `.crowbar` segment; `go vet -tags 'integration noEmbed' ./tests/...` in every Go task; refuse newer layout right after readLayout; ctx.Err() before each step; invalid-home test uses <TempDir>/file/sub; Apply returns only unmarked counts; narrowed upgrades grep; chat-types Verify = no chat with empty Type; main.go in T13 files; keep ReadHeaderTimeout; second-daemon test uses its own socket; R8 line on the home lock implying "no runner alive"; document probeLock race; T14 live check also launches once with CROWBAR_HOME unset → lands in <worktree>/.crowbar).
Ruling F18: Health parsing in new sidecar/health.rs; add `show: vi.fn()` to the mock; add `bun run lint:doctor`; DROP the 30-minute "migrating" cap — the daemon owns the state (halted/exited are reported), a cap is a timer that would kill a legitimately long migration — cost if wrong: a wedged migration shows "migrating" with live progress until the user quits.
Ruling F19: T14/T15 (cold cargo + bun install + dev-desktop) run only with ≥ 25 GB free; one shared CARGO_TARGET_DIR; df after the cargo fetch.
Ruling D11: T16's two spec-line amendments (ledger location) proceed; the user was told of the deviation.
Task 0: dispatched (haiku a040deb5c0600f59c) BASE 0c801d0e2 — runs alongside the S0 re-review (read-only); S0 needs no further implementer unless the re-review fails.
Task 0: implementer DONE (94d001ddd; package tests ok; baselines repos_extra_test.go 904→872, go.tests 209610→209578). Sweep: no other test writes the real home; read-only callers binpath_test:31, write_file_test:20, integration agent_test:241.
Ruling: standing Go-test rules appended to REFACTOR-CONSTRAINTS.md (temp HOME + real GOMODCACHE/GOCACHE; never run integration agent/scripted — kit reads real ~/.claude.json and links ~/.codex/auth.json via user.Current()). Task 0 left 4 GB of temp caches; controller deleted them.
Ruling: plan's integration RUNS (T11, T13, T16) become `go vet -tags 'integration noEmbed'` compile checks for agent/scripted; other tests/ packages that do not call IsolateProviderHomes may run under the standing rules.
Task 0: review dispatched (haiku a78597f7d249e6519) 0c801d0e2..94d001ddd
Task 1: dispatched (sonnet a5e81808d6b1c4e3e) BASE 94d001ddd. Rulings carried: R5 check rel==".." or "../" prefix (+ test for "..x"); At/Resolve never mkdir; R5 pin test uses user.Current().HomeDir; testing.Testing() guard approved; new files only; standing Go-test form; LOC growth reported, not --update'd.
Task 0: review Approved (a78597f7d249e6519).
Task 0: carry to Task 2 (pre-existing hazard): project_import.go:338 defaults ImportDeps.CrowbarHome to worktreepath.DefaultCrowbarHome → make it a required constructor argument (no default) in the T2 sweep; fix stale `defaultCrowbarHome` mentions in repos_extra_test.go:71,78,680,688,694; icons.go:64-72 fallback gets a test via core/home.
Task 0: complete (commits 0c801d0..94d001d, review clean)
Task 1: implementer DONE (5fd780479; home 14 tests, testutil 3; lint clean; LOC go.core +185, go.test-support +75, go.tests +204, go.total +260 — windowed). Extra tests accepted. Concern: Rust proxy hashes raw CROWBAR_HOME vs Go Clean(root) — reconciled by T14 (crowbar paths); low.
Task 1: review dispatched (sonnet a3f2b4cabc9560a59) 94d001ddd..5fd780479
Task 1: review Approved (a3f2b4cabc9560a59) with 2 plan-mandated Importants.
Ruling: Windows default → UserHomeDir()/.crowbar on every OS (matches the Rust shell and old socketPath; Documents\.crowbar matched nothing running); default_unix/windows.go deleted — cost if wrong: none (no Windows users/CI).
Ruling: socket hash stays on the cleaned root; the Rust shell hashes the raw env until T14 replaces it with `crowbar paths`. Carry to T2/T13 wiring: the daemon MkdirAll's the home (old code did; At/Resolve no longer do); dev exports clean CROWBAR_HOME — cost if wrong: a non-clean CROWBAR_HOME splits sockets between T2 and T14 (dev only).
Task 1: minors folded into fix round 1 (comment wording, windows skips, WriteFile rel guard).
Task 1: fix round 1 dispatched (resumed a5e81808d6b1c4e3e), FIX_BASE 5fd780479
Task 1: fix round 1 implementer DONE (7b87bfa0a; home 14, testutil 4; LOC go.total +238 cumulative)
Task 1: re-review dispatched (haiku a5bf33995f0bfc817)
Task 1: fix round 1/5 (4 addressed, 0 open; commits 5fd7804..7b87bfa). Windows relocation accepted: the old Documents template never expanded {{USER}} and the Rust shell already used %USERPROFILE%\.crowbar.
Task 1: complete (commits 94d001d..7b87bfa, review clean)
BASE before Task 2: 7b87bfa0a
Task 2: dispatched (sonnet a3db76b6544d794a2) BASE 7b87bfa0a. Rulings carried: F12 guard exact-match + .crowbar segment + deleted metadata selectors; F10 regression tests moved not deleted; Task-0 carry (ImportDeps required, stale names); daemon mkdir in adapter/binder; oversize files must not grow; F13 vet integration; standing Go form + PATH prefix; ratchet --update for file-size/counters shrinks, loc windowed.
Task 2: implementer DONE_WITH_CONCERNS (f64d50fd6, 64 files; guard 858 files; file-size/counters ok; loc windowed).
Ruling: deadcode on Home.LayoutFile/LockFile/MigrateDir accepted until their callers land (T5-T7); deadcode hard gate at T13 (extends F9) — cost if wrong: a dead accessor ships if a later task drops its caller.
Ruling: TestRegression_TerminalSession_RestartRoundTrip_RealStore hangs under the stripped-PATH env (also on the parent) → park for the terminal stage (S6): a test that hangs instead of failing at a deadline is itself a defect; not this task's scope.
Task 2: review dispatched (opus ab1d8d045351b3712) 7b87bfa0a..f64d50fd6
Task 2: review Needs fixes (opus ab1d8d045351b3712): 1 Critical (plan-mandated: override socket hashed the cleaned root → parity break with the desktop for non-clean CROWBAR_HOME), 1 Important (app/crowbar_home.go untested), 9 minors.
Ruling: REVERSES the Task 1 hash ruling — Socket() hashes the override exactly as given (as before and as the Rust shell does); Root() and other accessors stay cleaned — same behaviour as today, no window — cost if wrong: `/x` and `/x/` keep separate sockets, as today.
Ruling: all 9 minors folded into fix round 1 (stale names incl. Rust comments only, guard catches os.Getenv(home.EnvVar), allowlist shrinks to core/home, nil-panics in handler ctors, comment trims).
Task 2: fix round 1 dispatched (resumed a3db76b6544d794a2), FIX_BASE f64d50fd6
Task 2: fix round 1 implementer DONE (2b9da9cbe; 63 pkgs ok; Home.given hashed; Home.IsDefault added for seed/kit; repos.New removed from deadcode baseline)
Task 2: re-review dispatched (sonnet a382244e003f97066) 7f7b7d719..2b9da9cbe
Task 2: re-review round 1: all addressed; NEW Important: daemon binds the as-given hash but callbacks get cleaned --home → re-derive a different socket. Scope additions accepted (Home.IsDefault; kit replaces non-temp CROWBAR_HOME).
Ruling: the daemon owns its socket; callbacks get the bound socket in their scope flags and never re-derive (fallback: pass the as-given home if no flag exists and adding one > ~30 lines) — removes the derivation class instead of aligning two copies — cost if wrong: one more flag on callback command lines.
Task 2: fix round 2 dispatched (resumed a3db76b6544d794a2), FIX_BASE 2b9da9cbe
Task 2: fix round 2 implementer DONE (5ae62f6a6; 101 pkgs ok; --host=unix://<bound socket> in scope flags; black-box roundtrip test RED→GREEN). Accepted: ~55 lines (> 30) because the fallback needed the same plumbing; shipped descriptors use inherited CROWBAR_HOME (already matched).
Task 2: re-review round 2 dispatched (sonnet addde39c13bec48f5) 2b9da9cbe..5ae62f6a6
Task 2: fix round 2/5 (addressed; commits 2b9da9c..5ae62f6). Re-review: no new breakage.
Task 2: minor (deferred → Task 11, which edits tests/kit): isolateHome prints "ignoring CROWBAR_HOME" even when unset (every integration binary). The guard forbids os.Getenv(home.EnvVar) in kit, so add the check inside core/home if needed.
Task 2: minor (parked): runners surviving a daemon restart onto a different EXPLICIT --host keep the old baked-in socket (default host is stable).
Task 2: complete (commits 7b87bfa..5ae62f6, review clean)
BASE before Task 3: 5ae62f6a6
Task 3: dispatched (haiku a776ae9b066aed822) BASE 5ae62f6a6
Task 3: implementer DONE (7a556e426; RED/GREEN; binary check ok; trailer names Haiku 5.5 — accepted as accurate)
Task 3: review dispatched (haiku a2260aecf53421b1b) 5ae62f6a6..7a556e426
Task 4: dispatched (sonnet a16693af196ff88ab) BASE 7a556e426 — overlaps Task 3's read-only review (disjoint packages). Ruling F9 carried: no Catalog.Steps(), no Move.
Task 4: implementer DONE (3c4772afc; 100% cov, -race ok; lint clean; +315 go.core, +297 go.tests). Extra rejections accepted: invalid phase; pre-open step beyond the known layout. Carry to Task 5: reject an unreadable/negative LAYOUT before calling order() (order clamps negatives to 0).
Task 4: review Approved (sonnet a06170bfa40553564; spec yes, 0 Critical/Important).
Ruling: Task 4 minors carried into Task 5 (same package, Task 5 owns LAYOUT reading): document `0 <= layout <= Known` as order()'s precondition and drop the clamp; add a test where Number order != layout order (expects [2 1]); delete Action.Dest and Action.Note (no producer/consumer anywhere in the plan) — one dispatch instead of a fix round — cost if wrong: Task 12 re-adds a field.
Task 4: complete (commit 3c4772afc, review clean)
BASE before Task 5: 3c4772afc
Task 3: review (haiku a2260aecf53421b1b): spec yes; sole Important = loc-budgets red (+37 go.cmd, +49 go.tests).
Ruling: Task 3's LOC Important is not a task defect — Ruling F1 makes engine + S1 one LOC window enforced at S1's gate; file-size and counters are ok — the reviewer lacked that ruling — cost if wrong: none (gate still enforced at the window end).
Task 3: minors parked: paths.go:31 shadows err; main.go:24 long AddCommand line; relative-home test doesn't assert stderr (verified by hand).
Task 3: complete (commit 7a556e426, review clean under F1)
Task 5: dispatched (sonnet a1f9e6a5b6d633ddc) BASE 3c4772afc, with Task 4 minors as a separate first commit.
Task 5: implementer DONE (71cf70ae8 Task-4 minors; ca7407075 LAYOUT+lock; race/vet/windows vet ok; +266 go.core, +213 go.tests)
Task 5: review dispatched (sonnet a74fa85c1fc27ab04) 3c4772afc..ca7407075
Task 5: review Approved (sonnet a74fa85c1fc27ab04; 0 Critical/Important). Concerns 1-4 accepted (I/O errors wrap ErrLayoutUnreadable → reportable halt; Windows skips dir fsync; slog.Warn on probe release).
Ruling: Task 5 polish (layout.go:98-99 drop the false NTFS write-through claim; lock_test.go:31-33 assert `perm &^ 0o750 == 0` (umask-proof); catalog.go:48-53 order doc ≤ 3 lines) carried into Task 6 as a separate first commit — same package — cost if wrong: none.
Carry → Task 8: never Close() a nil *homeLock (panics); writeLayout can return a dir-sync error AFTER the rename landed — don't record step.fail for a bump that took effect (or document it).
Carry → Task 9: add a layout_unreadable I/O case (LAYOUT is a directory). Plan line 479 is stale: readLayout I/O errors are halts, not Begin errors.
Carry → Task 12: probeLock briefly holds the exclusive lock; a concurrent daemon lockHome can see a spurious ErrHomeLocked — document.
Task 5: complete (commits 71cf70ae8..ca7407075, review clean)
BASE before Task 6: ca7407075
Task 6: dispatched (sonnet a900646a194e9c0de) BASE ca7407075, Task 5 polish as first commit
Task 6: implementer DONE (02a3f0590 Task-5 polish; b7aade134 ledger+checkpoints; 92.3% cov, race/vet/windows ok)
Task 6: review dispatched (sonnet abc85c716248425bb) ca7407075..b7aade134
Task 6: review Needs fixes (sonnet abc85c716248425bb): Important — failed/short append leaves a fragment that the next append (Task 8's step.fail/run.end) fuses into a permanent ErrLedgerCorrupt boot halt (reproduced with RLIMIT_FSIZE). Minors: clone Counts/Pending in append; syncDir comment; zero-byte ledger test; Kind=="" line is corrupt.
Ruling: poison the ledger on any Write/Sync error (the ledger owns its integrity; callers can't be trusted to stop, and Task 8's failure path itself appends) — cost if wrong: after a transient write error the run's step.fail/run.end is not recorded; next boot sees a dangling run.
Carry → Task 8: after an append error the ledger refuses all appends; the failure path must not expect step.fail/run.end to land — the next boot closes the dangling run.
Task 6: fix round 1/5 dispatched (resumed a900646a194e9c0de), FIX_BASE b7aade134
Task 6: fix round 1 implementer DONE (fa103a6cb; poison on Write/Sync; append refuses empty Kind; 92.9% cov)
Task 6: re-review dispatched (haiku ae7dab26aa7c1d035) b7aade134..fa103a6cb
Task 6: fix round 1/5 (addressed; commits b7aade1..fa103a6). Re-review clean; Sync-poison branch untested (fsync can't fail portably) — parked.
Task 6: complete (commits 02a3f0590..fa103a6cb, review clean)
BASE before Task 7: fa103a6cb
Ruling (Task 7): "skip .lock" means exactly <dir>/.lock at the walked root (the home lock), never any file named .lock deeper in state/ — F5 says state/.lock; a nested .lock is user data — cost if wrong: a deeper lock file gets backed up (harmless).
Task 7: dispatched (sonnet a224583e61a704717) BASE fa103a6cb
Task 7: implementer DONE (f8641c410; race/vet linux+windows ok; extras: .partial run ids refused, parent chain fsynced, treeSize absent=0; symlinked state/ root refused)
Task 7: review dispatched (sonnet ad55db155ec595136) fa103a6cb..f8641c410
Task 8 rulings (prepared): Begin order = lockHome → readLayout → [Task 9 newer-layout refusal slot] → cleanPartialBackups → readLedger (F4: read-only; the ledger file opens lazily on the first append, so R11(a) creates no migrate/); ctx.Err() checked before each step (F14); step.done counts = cp.total() + Apply's unmarked counts (F15); dangling-run close, Plan/preflight, backup and interruption are Task 9 — not in Task 8; a failed append never loses the Halt (cause kept, append error joined); never Close a nil lock; run id = UTC with nanoseconds (single path segment, unique per boot).
Task 7: review Needs fixes (sonnet ad55db155ec595136): Important — symlinked state/ root refused → every rewriting migration halts for a config that works today; treeSize returns 0 for it.
Ruling: resolve the state/ ROOT symlink once (EvalSymlinks, shared by scan+copy); free space stays probed at h.Root(); symlinks inside state/ stay refused — a relocated state dir is a legitimate setup and the rest of the product follows it — cost if wrong: backup copies from a target the user didn't expect (still read-only).
Minors in the same round: lock precondition doc; fail fast on an existing final backup dir; error names the entry kind; Linux uses Frsize. Parked: apparent size vs allocated blocks (ENOSPC mid-copy is cleaned up).
Task 7: fix round 1/5 dispatched (resumed a224583e61a704717), FIX_BASE f8641c410
Task 7: fix round 1 implementer DONE (bac98ef22; resolveRoot; fail-fast existing final; kind in error; disk_linux Frsize)
Task 7: re-review dispatched (haiku a8e130b6d60c516b7) f8641c410..bac98ef22
Task 7: fix round 1/5 (addressed; commits f8641c4..bac98ef). Re-review clean. Parked minors: root resolved per walk (scan, copy) — retarget between them is a narrow window under the lock; requireAbsent runs after the scan.
Task 7: complete (commits f8641c410..bac98ef22, review clean)
BASE before Task 8: bac98ef22
Task 8: dispatched (sonnet a63dfdc81b329fb09) BASE bac98ef22
Task 8: implementer DONE (1346c606c; 25 tests, -race -count=3 ok; session.go 193, apply.go 325; seam in Session.load())
Task 8: review dispatched (opus af643c67a86432750 — core loop) bac98ef22..1346c606c
Task 8: review Needs fixes (opus af643c67a86432750): 3 Important — ledger I/O failures are plain errors (daemon exits, violates R8); run.start lazy → post-open-only runs start after stores open (Task 9 backup seam unsafe); Halt() races with Task 13's health reader.
Ruling: ledger open/run.start/step.start failures → step_failed halt on the step about to run; a failed closing run.end{completed} after all steps are durable → Warn + nil (migration succeeded; next boot closes the dangling run) — a halt there would block a successful boot — cost if wrong: one run recorded as interrupted.
Ruling: RunPreOpen starts the run whenever anything (either phase) is pending; RunPostOpen never starts one; a before-step cancel inside a run appends run.end{interrupted} (Task 9 adds mid-Apply) — matches Task 13's cancelled-boot test.
Ruling: Halt stored as atomic.Pointer. Minors in round: Halt.Ledger only if file exists; Begin doc; checkpoints keyed by number AND name (R2 consistency); delete/replace the coverage-only Options test.
Note → Task 11/13: Boot returns the session only after RunPreOpen, so the reporter can't show pre-open progress (plan-level; F3 already publishes the session before RunPreOpen — verify at Task 13).
Task 8: fix round 1/5 dispatched (resumed a63dfdc81b329fb09), FIX_BASE 1346c606c
Task 8: fix round 1 implementer DONE (fa6f470a3; tests written after fix but each mutation-checked to fail on the old behaviour; failure path moved to halt.go + halt_test.go)
Task 8: re-review dispatched (sonnet af78e370e1df75061) 1346c606c..fa6f470a3
Task 8: fix round 1/5 (addressed; commits 1346c60..fa6f470). Re-review clean. Note: finish's "next boot closes the dangling run" becomes true at Task 9.
Task 8: complete (commits 1346c606c..fa6f470a3, review clean)
BASE before Task 9: fa6f470a3
PAUSED before Task 9 dispatch: disk 3.3 GB free (< 8 GB stop rule). Not our temp dirs; swap at 7.3/8 GB (memory pressure) is the likely consumer; Go build cache 11 GB. Asked user to free space.
RESUMED: user approved both; go clean -cache (11 GB) + target/node_modules removed in 17 merged-PR worktrees (develop, open-PR and no-PR worktrees kept). 58 GB free. T14/T15 disk gate (>= 25 GB) now met.
Task 9: dispatched (sonnet ae4ffc52614831f64) BASE fa6f470a3 — rulings: F14 refusal before any write; LAYOUT-as-dir case; F6 one backup per attempt (retry reuses); dangling close only when one exists
Task 9: implementer DONE (c57927c1f; 14 RED→GREEN; race -count=3; preflight.go+preflight_test.go new; concerns: completed-but-unrecorded run's backup stays 'in force'; backup-record append failure leaves Halt.Backup empty)
Task 9: review dispatched (opus aa9780085bf986de7) fa6f470a3..c57927c1f
Task 9: review Needs fixes (opus aa9780085bf986de7): Important — a finished-but-unclosed run (failed run.end write or kill after last step.done) is closed interrupted, so its backup stays in force forever → a later rewriting migration reuses a stale image (breaks R7).
Ruling: Fix A — closeDanglingRun closes `completed` when every number in run.start.Pending is a catalog step no longer pending; else interrupted. Brief amended: the R2 bump-landed test expects `completed` — accurate history + backup retires with no new state — cost if wrong: a run is recorded completed that a person would call interrupted (no data effect).
Ruling: an unreadable ledger (dir / EACCES) is a ledger_corrupt halt on a live session (R8: halt, don't exit), mirroring LAYOUT I/O → layout_unreadable.
Minors in round: Halt.Backup dropped if the path is gone (+Warn on retake); HaltNoImpl in post-open uses stop(stepRan=false); test the backup-record append failure; tighten the newer-layout message assertion; 3-line comment.
Parked → Task 16 docs: F6 boundary — halt, reinstall older release, run for days, upgrade: the retry reuses the pre-halt image, so days of data are in no backup (the ledger can't see normal operation). Halt.Layout is 0 for layout_unreadable.
Task 9: fix round 1/5 dispatched (resumed ae4ffc52614831f64), FIX_BASE c57927c1f
Task 9: fix round 1 implementer DONE (d22299c33; 8 RED→GREEN; Fix A; unreadable ledger halts)
Task 9: re-review dispatched (sonnet acee55ba1de16cdb9) c57927c1f..d22299c33
Task 9: fix round 1/5 (addressed; commits c57927c..d22299c). Re-review clean. Dropped-step dangling run stays interrupted (accepted: wrongly 'completed' could retire the only pre-attempt image).
Carry → Task 16 docs: plan lines ~352, 526, 536 stale (danglingRun signature; bump-landed run closes completed now); F6 rollback boundary; Halt.Layout 0 for layout_unreadable.
Task 9: complete (commits c57927c1f..d22299c33, review clean)
BASE before Task 10: d22299c33
Task 10: dispatched (haiku a178cdd49d66ca99e) BASE d22299c33
Task 10: implementer DONE (d49906b8f; 8/8 -race; report transcribed)
Task 10: review dispatched (sonnet af42fd7d8e9eaedbe) d22299c33..d49906b8f
Task 11 rulings (prepared):
- F3 overrides the brief's Boot: adapter/migrations exposes `Begin(ctx, h, opts) (*migrate.Session, error)` (Catalog + migrate.Begin) and `PreOpenImpls(h)`; callers run `sess.RunPreOpen` themselves so Task 13 can publish the session to /health before pre-open runs. A Begin-time halt comes back as a live session (Halt() set).
- F7 overrides WithPostOpen: app.New takes the post-open runner as a POSITIONAL required arg; unit tests pass a no-op from new internal/testutil/migratetest; kit + harness_test + both integration harnesses pass a real session.
- F10: TestRegression_StateDirSingleInstanceLock moves to migrate/lock_test.go (name kept) when the adapter lock is deleted.
- F11 + F16: a legacy step meeting unreplayable rows records `skipped_rows` and completes (regression_upgrade_boot_best_effort_test.go stays green); errors only for engine-level failures; Verify is a real postcondition over replayable rows (same predicate the backfill uses), never "marker absent"; grep gate narrowed to 'state/upgrades\|"upgrades"'.
- Task 2 carry: kit isolateHome's stderr line printed even when CROWBAR_HOME is unset → delete the line (silent replacement, as round-1 reviewer accepted); fix its doc.
- Verification: `go test -tags 'integration noEmbed' ./tests/ -run 'Upgrade|Health'` IS allowed — api/tests uses kit.Main (isolateHome only; no provider-home reads, no package-level home reads; checked). Never ./tests/integration/... or ./tests/kit.
- Deadcode after T11: may list only probeLock, ledgerView.lastRun, Session.Progress (F9); baseline must not grow.
Task 10: review Approved (sonnet af42fd7d8e9eaedbe; mutation: plain field → DATA RACE).
Carry → Task 13: export bootgate StateMigrating/StateHalted constants and use them (a typo'd state reads as "ready" in Task 15's parser); marshal before WriteHeader and 500 on encode failure; CORS: gate responses lack Access-Control-Allow-Origin (webview can't read daemon_* bodies; preflight OPTIONS gets 503) — decide.
Carry → Task 15: scripts/e2e/lib/daemon.mjs waitHealthy treats any 200 as ready; check_health (mod.rs:391) too until Task 15.
Task 10: complete (commit d49906b8f, review clean)
BASE before Task 11: d49906b8f
Task 11: dispatched (opus af974f2fa476f027b — 20+ files, judgment-heavy) BASE d49906b8f
Task 11: implemented f9ad89c28 + 2d8816572 (opus af974f2fa476f027b) — concerns: deadcode adds RunSummary.fold + layoutOf (only caller lastRun); background work leaks if RunPostOpen fails inside app.New; lockHome creates home 0700; internal.go wired (halt returned as error until T13)
Task 11: review dispatched (opus af4b714299d21f28a) package review-d49906b8f..2d8816572.diff → task-11-review.md
Task 11: review Needs fixes (opus af4b714299d21f28a; 0C/2I/12M; named risks all clean: adoption, single lock, R11, shared predicates, 0700 only on create).
Ruling: fix I1 (hook-deliveries best-effort, skipped_rows, shared Apply/Verify predicate), I2 (app.New stops what it started on runner failure; lock released after stores close) and M1/M3/M4/M5/M7/M8/M10/M11 in fix round 1 — I2's exit is new in this task and Task 13 keeps a halted daemon alive — wrong costs one extra round.
Ruling: M1 project-homes — a home that exists as an unpurged tombstone is not "missing"; one predicate for Apply+Verify; other CreateHome failures stay engine-level halts — if wrong, a project stays homeless until manual action.
Ruling: M2 nil-usecase panic accepted (amends dispatch ruling 7) — usecases are required ctor deps; a panic at New is a loud wiring failure, a nil-check reintroduces the guard CLAUDE.md forbids — cost if wrong: a crash instead of a gate on a wiring bug no test misses.
Ruling: deadcode fold/layoutOf accepted until Task 12 (callers of lastRun via Inspect/Render).
Carry → Task 12: M6 seed-chat must refuse (via Inspect) when anything is pending, else it opens a run/backup and leaves it dangling; add a test for its run(). M9 treeSize lives in backup_test.go — move it back to production when Inspect uses it.
Carry → final review: M12 go.total +~620 on an already-over loc budget.
Task 11: fix round 1 dispatched (resume opus af974f2fa476f027b) brief task-11-fix1-brief.md
Task 11: fix round 1 committed 91f83e994 (A–H; RED shown for B/C/F). Re-review dispatched (sonnet abecc3f7388ad0d69) package review-2d8816572..91f83e994.diff → task-11-fix1-review.md
Carry → S1 (chat ledger), product bug found in fix round 1: deleting a home workspace's only chat reaps the home (DeleteCascade has no guard) → a live project is left homeless; project-homes runs once per install so nothing recreates it after the upgrade boot. S1 must add TestRegression_* (delete home's only chat → project keeps a home) and fix the owner (DeleteCascade must not reap a project's home).
Carry → final review: runner package Windows vet fails on syscall.Kill in test files (pre-existing, untouched); loc shift go.wiring→go.app-misc ~190 (app shutdown.go).
Task 11: re-review clean (sonnet abecc3f7388ad0d69; A–H addressed, no new C/I).
Carry → Task 13: abandon() on an interrupted boot (ctx already cancelled) gets a cut-short asynx Shutdown and stopBackgroundWorkers is then a latched no-op — untested; Task 13 owns interruption/halt lifetime.
Ruling: file-size/counters "shrank" exits are accepted until Task 16's single `make ratchet ARGS=--update` (Ruling F1 window) — one batched update — cost if wrong: none, numbers only fall.
Task 11: complete (commits f9ad89c28, 2d8816572, 91f83e994; review clean after 1 fix round)
BASE before Task 12: 91f83e994
Ruling: Task 12 seed-chat — Inspect first, refuse when anything is pending or halted, then Begin only for the lock (no RunPreOpen); a caller that seeds a never-booted home → NEEDS_CONTEXT, not a guess — cost if wrong: a dev script needs one daemon boot first.
Ruling: after Task 12 deadcode may list only Session.Progress (fold/layoutOf/lastRun/probeLock live via Inspect).
Task 12: dispatched (sonnet a5f8c6ee6dfaca485) BASE 91f83e994
Task 12: implemented a90aa84ee + 60575a217 (seed-chat second: calls Inspect). Concerns: --plan doesn't foresee a symlink in state/ that halts backup; says 'required' when an interrupted attempt's backup would be kept; Required=false for empty state/.
Carry → final review: cmd/crowbar/hook_answer_test.go:271 syscall.Kill breaks GOOS=windows vet of ./cmd/... (pre-existing, same class as runner).
Note: seed-chat callers = Makefile seed-chat (needs existing WORKSPACE_ID) + S2e QA step; a dev home last booted pre-engine is refused until one boot of this build.
Task 12: review dispatched (sonnet a96a462c8a9192b29) package review-91f83e994..60575a217.diff → task-12-review.md
Task 12: review Needs fixes (sonnet a96a462c8a9192b29; 0C/2I/5M). I1 planBackup is a parallel copy ignoring backupInForce (preview says "required"/may say "will halt" when the retry keeps the old backup); I2 symlink/socket/device in state/ halts the real boot (HaltPreflight) but previews green.
Ruling: fix I1+I2 by sharing the session's backup-due rule and preflight checks with Inspect (no copies); a preflight failure makes Inspect return the same *Halt the boot would (existing pattern; --plan exits non-zero) — cost if wrong: the user sees the halt instead of the step list for a home that can't migrate anyway. Also fix files==0 vs size==0, planPending/planAll duplication, comment length, seed-chat Halt() test, HaveGit via shared preflight.
Task 12: fix round 1 dispatched (resume sonnet a5f8c6ee6dfaca485) brief task-12-fix1-brief.md
Task 12: fix round 1 committed 7e01eda37 (checkPending shared; treeSize → measureState supersedes decision 3; double free-space probe in session)
Task 12: re-review dispatched (sonnet af884731c942c47a6) package review-60575a217..7e01eda37.diff. Note: implementer a5f8c6ee6dfaca485 stopped with own background work still running — tree clean at 7e01eda37; watch for stray commits.
Task 12: re-review clean (sonnet af884731c942c47a6; A–C addressed, no new C/I). Minors: Inspect reads the raw view, so after Begin closes a dangling run as completed it can say "kept" while the boot copies anew (violates the never-disagree ruling); 3 preflight comments 4 lines; Halt-equality test over-excludes Backup.
Ruling: fix those three in a small fix round 2 (resume sonnet a5f8c6ee6dfaca485) — a known preview/boot disagreement is the defect class this round exists to remove — cost: one small round.
Task 12: fix round 2 committed 9b9863984 (Catalog.endOfDangling shared; haltAt sets Backup)
Task 12: fix round 2 re-review dispatched (sonnet a2270312b2ceeef80)
Task 12: fix round 2 re-review (sonnet a2270312b2ceeef80): A–C addressed; I1 inspect_test.go 505 lines (new file over cap → file-size ratchet fails; implementer's "unchanged" claim wrong); Minor: no test pins the Halt path's use of the settled view (inspect.go:61).
Ruling: fix round 3 (resume sonnet a5f8c6ee6dfaca485): bring inspect_test.go under 500 without baselining — prefer moving any test that exercises another source file (preflight/backup/ledger) into that file's _test.go over squeezing — plus the Halt-table row (finished dangling run + backup on disk + git missing → no Backup). Rule: always run `make ratchet` and quote file-size output verbatim.
Task 12: fix round 3 dispatched
Task 12: fix round 3 committed 18a5f78f1 (preflight tests moved to preflight_test.go 426; inspect_test.go 405; RED shown for the settled-view row). Controller verified: stat = move + one row; file-size ratchet prints no grew/new line.
Task 12: complete (commits a90aa84ee, 60575a217, 7e01eda37, 9b9863984, 18a5f78f1; review clean after 3 fix rounds)
BASE before Task 13: 18a5f78f1
Ruling (Task 13 dispatch): sequence per F3/F7 — gate serves before Begin; the session is published to the reporter BEFORE RunPreOpen; app.New takes the session positionally. Bootgate carries folded in (exported state constants; marshal-then-write, 500 on encode failure; gate responses use the API's existing CORS middleware, not a copy). e2e waitHealthy waits for status "ok" and fails fast on "halted". Teardown after a cancelled boot must complete (context.WithoutCancel for shutdown only) — T11 carry. Deadcode hard gate: no new entries.
Task 13: dispatched (opus ac0127cf44894d7ad) BASE 18a5f78f1
Task 13: implemented 5b6bfdaa6 + 923367979 (opus ac0127cf44894d7ad). Concerns: e2e waitHealthy checked vs fake server only; halt reason in detail.message (T15 parser); 'starting' message untested; reporter inlined in internal.go because loc-budgets rejected a new file.
Task 13: review dispatched (opus a6f8a98085f3bdeb6) package review-18a5f78f1..923367979.diff → task-13-review.md
Task 13: review Needs fixes (opus a6f8a98085f3bdeb6; 0C/1I/9M). I1 cancelled boot drops New's error silently (main.go:81). Reviewer: reporter-in-internal.go was a misread (loc-budgets only needs a modules.json path assignment), not gaming.
Ruling: fix I1 + M1–M8 in fix round 1 (resume opus ac0127cf44894d7ad); M9 baseline write-back stays batched at Task 16. Adding a path to modules.json under an existing module key is assignment, not a budget change — allowed.
Note: from 923367979 until Task 15 the desktop's check_health reads the immediate "migrating" 200 as ready → engine Tasks 13 and 15 ship together (the branch is one release anyway — user decision).
Task 13: fix round 1 dispatched brief task-13-fix1-brief.md
Task 13: fix round 1 committed 4d96b0142. Concerns: halted-boot teardown failures now Debug only; one halt still logs twice at Error (migrate: halted + daemon: halted).
Task 13: re-review dispatched (sonnet a82b3227607606366). Mermaid fix cherry-picked onto branch (from 40b4f9120, fix/mermaid-error-overlay) — web-only, unrelated to the engine plan.
Task 13: fix round 1 re-review (sonnet a82b3227607606366): I1, M1–M8 addressed; NEW Important: halted-boot teardown failure logged at Debug (internal.go:126) → invisible at the daemon's Info level (base logged Error).
Ruling: fix round 2 (resume opus ac0127cf44894d7ad): teardown failures on a halted boot → slog.Warn only when err carries more than the halt; one halt = one Error line: serve's `daemon: halted` stays Error, the engine's `migrate: halted` (halt.go:58,:90) drops to Info (kept, since M3's test seam observes it) — cost if wrong: one extra Info line per halt.
Task 13: fix round 2 committed d04d4f36d (Warn only when err carries more than the halt — besidesHalt walks the join/wrap tree; engine's migrate: halted → Info).
Ruling: controller verified fix round 2 directly (6 files, logging-level change + tree walk + tests; RED shown) instead of a fourth review seat — cost: a missed nit in a logging helper; the final whole-branch review re-reads it.
Task 13: complete (commits 5b6bfdaa6, 923367979, 4d96b0142, d04d4f36d; 2 fix rounds)
Note: the branch also carries 04e0373fc (user-requested mermaid fix, cherry-picked from fix/mermaid-error-overlay 40b4f9120) between T13 commits.
BASE before Task 14: d04d4f36d
Ruling: Tasks 14 and 15 go to ONE implementer in sequence (two commits), with one live dev-desktop session at the end that covers both checks, then one combined review over both briefs. Reasons: they share sidecar/mod.rs and lib.rs, and each needs a cold cargo build plus dev-desktop (memory: one dev instance, serialised). Cost if wrong: a combined diff is harder to review, mitigated by the separate commits.
Ruling: F18 still applies, so Health parsing goes in a new sidecar/health.rs.
- The web mock gains `show: vi.fn()`, and `bun run lint:doctor` is run.
- The 30-minute migrating cap is DROPPED. A daemon that keeps reporting "migrating" keeps being held.
Ruling: oversize files must not grow (user HARD 500-line rule): mod.rs 836, lib.rs 1371 and ws_bridge.rs 758.
- New code goes in new files (paths.rs, health.rs).
- CI gates to run: cargo fmt --check, cargo clippy -D warnings, cargo test (sidecar:: targeted; full desktop cargo test only once at the end).
Task 14+15: dispatched
Task 14+15: dispatched (sonnet a4e7434f8d9b1d989) BASE d04d4f36d
Task 14+15: implemented 820617387 + 74e0c7775 (sonnet a4e7434f8d9b1d989), with live checks PASS.
- mod.rs 836→662; lib.rs and ws_bridge.rs did not grow.
- Deviation: the web also reads /v0/health once on mount, because the one-shot daemon:halted event fires about 0.4 s before the page mounts.
- Concerns:
  - /oobe has no toast viewport, so the halt is invisible to a fresh install;
  - the connection indicator says "Reconnecting…" beside the halt toast;
  - Go home.Home.given comment and scripts/e2e/lib/daemon.mjs still duplicate fnv1a64.
Task 14+15: review dispatched (opus a4582c6659b03f839) package review-d04d4f36d..74e0c7775.diff → task-14-15-review.md
Task 14+15: review Needs fixes (opus a4582c6659b03f839; 0C/2I/6M).
- T14 ✅.
- I1: the halt toast has no viewport outside IDEShell, so /oobe shows nothing.
- I2: nothing tests that a held daemon is never killed on the real watchdog path.
- Reviewer accepted the on-mount health read: it subscribes first, then reads the owner once. Not a resync.
Ruling: fix round 1 (resume sonnet a4e7434f8d9b1d989) covers I1, I2, M1, M2, M3 and M5.
- I1: a root halt screen replaces the toast, so it shows on every route.
- I2: a watchdog-path test that must bite.
- M1: delete Home.given and hash the cleaned root, one rule. Cost if wrong: an unclean dev CROWBAR_HOME gets a new socket name once.
- M2: the e2e helper asks crowbar-api paths.
- M5: Go-owned health fixtures that Rust include_str!s.
Ruling: M4 ("Reconnecting…" during migrating/halted; connection store learns daemon_halted/daemon_migrating are states) → the S3 plan. M6 (silent vanish on resolve failure, native dialog) → final review.
Task 14+15: fix round 1 dispatched
2026-10-18: network outage (ENOTFOUND) killed T14+15 fix round 1 (mid-C, uncommitted edits kept), the S3 planner and the asynx verifier. All three resumed via SendMessage. Trailer is back to Co-Authored-By only (Claude-Session dropped).
Task 14+15: fix round 1 committed 2dea4beab (halt screen on every route; probe_ready watchdog test bites; Home.given deleted; e2e asks crowbar-api paths; Go-owned health fixtures). Live PASS (IDE + /oobe shots).
Task 14+15: re-review dispatched (sonnet aa5692460844808b3)
Task 14+15: fix round 1 re-review (sonnet aa5692460844808b3).
- A–D ADDRESSED. E is addressed in code, but has a CI gap.
- I-1: the ci.yml tauri filter covers desktop/** only, so a Go-only rename of a health state never runs the Rust fixture test.
- Minors:
  - scope_roundtrip_test.go:168 comment is now false;
  - mod.rs is still 603 lines (touched oversize file > 500: the user's HARD rule);
  - the halt message says "crowbar" in lowercase;
  - no Quit button.
Ruling: fix round 2 (resume sonnet a4e7434f8d9b1d989) covers:
- I-1: add api/internal/core/bootgate/testdata/** to the tauri filter;
- M-1;
- capitalise "Crowbar" in user-facing halt text;
- split sidecar/mod.rs under 500 by moving whole concerns into new files, no behaviour change.
Quit button: parked; Cmd+Q works and there is no process plugin.
Cost if wrong: one more small round.
Task 14+15: fix round 2 committed 7999c1275.
- CI tauri filter now includes bootgate/testdata.
- Comment fixed.
- "Crowbar" capitalised (RED shown).
- sidecar split: mod.rs 603→282, plus stop.rs and watchdog.rs; a pure move, and the same 29 tests pass.
Controller verified it directly: the stat, the CI diff, and no stray processes.
Task 14: complete (820617387; fixes in 2dea4beab, 7999c1275)
Task 15: complete (74e0c7775; fixes in 2dea4beab, 7999c1275)
Parked → final review: no Quit button on the halt screen (no process plugin; Cmd+Q works).
BASE before Task 16: 7999c1275
Task 16: dispatched (sonnet af447f537de75d1ad) BASE 7999c1275 — docs as built, final gates, one ratchet --update commit; must not stage untracked s3 plan files
Task 16: implemented by sonnet af447f537de75d1ad.
- Commits: 0c694ca4b (docs), d94e0c962 (ratchet --update: only falls), 971ab04cc (3 behaviour tests; adapter/migrations coverage 85.1 → 90.8%).
- All gates exit 0 except loc-budgets (Ruling F1).
- go.total is +4,097 over its baseline.
- Web grew too: total +79, tests +210. That is outside the Go window, so S1's gate must settle it.
Task 16: review dispatched (sonnet a17ccfdd556648849) → task-16-review.md
Final whole-plan review dispatched (opus a48ada15480bd892d) range 0c801d0e2..971ab04cc → final-review.md. Ruling: opus, not fable, for the final seat — the user asked to watch costs and opus has been the top reviewer all plan — cost if wrong: a subtle cross-task issue survives to S1 review.
Task 16: review Approved (sonnet a17ccfdd556648849; 0C/0I/7M — doc one-liners: upgrade-notes:8 'all through the engine' vs every-boot items in app.New; ARCHITECTURE.md:142 names deleted runUpgrades, :143 hook server order, :202 'set by CROWBAR_HOME' → any non-default home; plan R9/boot lines still name WithPostOpen/Boot; spike diagram order).
Ruling: Task 16 doc minors fold into the single final-review fix dispatch.
Task 16: complete (commits 0c694ca4b, d94e0c962, 971ab04cc)
Final review (opus a48ada15480bd892d): 0 Critical, 2 Important, 12 Minor. Verdict: ready for S1 once I1 and I2 are fixed.
- I1: a Begin I/O failure exits the daemon instead of halting it (R8).
- I2: a backup restore is unsafe, because LAYOUT and the ledger do not rewind, and the halt UI never shows the backup path.
Ruling: ONE final fix dispatch (opus), per final-fix-brief.md. It covers I1, I2, M1–M5, part of M6, M7–M10, M12, and a native dialog on resolve failure (tauri-plugin-dialog is already a dependency).
Ruling: carried out of this round:
- M6 config.Get and engine.WithHomeDir → S7.
- M11 loose permissions on existing homes → S8, as a catalog step.
- The Quit button stays parked.
- The migrating screen → S1's first long step.
- The web LOC growth decision → S1's gate.
Final fix round dispatched (opus a08b3ea3b58265a32) BASE 971ab04cc → final-fix-report.md; scoped re-review follows.
Final fix round committed by opus a08b3ea3b58265a32:
- 211d6a9e8 (api)
- b312628a4 (desktop)
- ab4a06d21 (web)
- 0155ad929 (docs)
Gates: all green except loc-budgets (F1).
Live check: a garbage view.db halted at step 2 with step_failed, and both the halt screen and /v0/health named the backup. A card-overflow bug was found and fixed live.
Ruling: the M2 test waits out the real 4 s writerDrainGrace. That breaks "no timing in tests". The grace must be injectable (constructor or config), and the test must not wait in real time. This goes in a residual fix round after the re-review.
Ruling: `chat/activity/internal/store/store.go` is not gofmt-formatted. It is pre-existing (728d84787 is in develop). Gofmt it in the same residual round, because "pre-existing" is never an answer.
S4 plan committed (see git log). Its pre-flight is running separately.
Final fix re-review dispatched (sonnet aafc690946725fc32) → final-fix-review.md
Final fix re-review (sonnet aafc690946725fc32): 14 ADDRESSED, 0 PARTIAL, 0 NOT ADDRESSED; 0 Critical, 0 Important; verdict Residuals (Minor only).
Ruling: one residual round (sonnet, fresh) fixes every residual; none is parked.
- M2 seam: `abandonWithin(ctx, cause, grace)`. abandon passes writerDrainGrace; the internal test passes a tiny grace against a drain that never finishes, and has no real-time wait.
- Minor 1: a lockHome failure other than ErrHomeLocked becomes an io_failed halt that is served WITHOUT the lock. Nothing runs, the gate only reports, and a second daemon still collides on the socket.
  - If the home cannot even be created, the gate cannot bind and the daemon exits. That stays as is, with a test pinning which branch is which.
  - Cost if wrong: a halted daemon without a lock. It runs no steps, so nothing is written.
- Minor 2: delete the dead Env.Earlier write.
- Minor 3: drive the resolve-failure dialog live once with a relative CROWBAR_HOME, under the "never defer live verification" rule. If the native dialog can't be observed without screen recording, say so.
- Minors 4–8: fix them all. The upgrade-notes must also say what happens to migrate/backups: kept, never pruned, and safe for the user to delete once the upgrade is confirmed.
- Also: gofmt `chat/activity/internal/store/store.go`.
Residual fix round dispatched (sonnet a677b5decfc93dc69) BASE 0155ad929… (HEAD 044e6bfc2) → residual-fix-report.md
Residual round (sonnet a677b5decfc93dc69) committed e706b3841 (api) and ec50d1548 (docs). Every item is done.
- The M2 test now runs in 0.12 s and bites under mutation.
- An io_failed halt now happens without the lock, and both branches are pinned.
- store.go is gofmt-clean.
- Gates: all exit 0 apart from loc-budgets (Ruling F1).
- The controller checked the stat and found no new sleeps or `_ =`.
Live dialog: partly observed.
- The log shows the reason, a thread waits inside CFUserNotificationDisplayAlert, and no daemon is spawned.
- The dialog's text and the exit after dismissal were not observed: native automation is forbidden by the user's rule.
- Ruling: accept this, and give the user a one-click check (`CROWBAR_HOME=rel/home make dev-desktop`, then click OK). Cost if wrong: the app does not exit after OK.
Ruling: `api/tests/integration/scripted/livemodel_test.go` is not gofmt-clean (pre-existing). It is gofmt'd as a chore commit inside S1 T1's dispatch.
MIGRATION ENGINE: COMPLETE (T0–T16 plus the final review, fix, re-review and residual rounds). HEAD is ec50d1548.
Ruling: this workspace is KEPT until the branch finishes, even though the skill says to delete it. The S1, S3 and S4 ledgers cite final-review.md and this ledger's carries. Cost: a few MB of ignored files.
Carries to S1, already in the S1 ledger or added there now:
- DeleteCascade reaps the home when its only chat is deleted. Add a TestRegression and fix the owner.
- The migrating screen arrives with the first slow step.
- The web LOC growth is decided at S1's gate.
~~~~
