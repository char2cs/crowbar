# S3a: One error envelope, `report()`, daemon states. Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every error the daemon returns carries a code from one closed enum and the status that code maps to; the client reads it as one `Problem`, has one `report()`, reports every unhandled rejection, and shows a migrating or halted daemon as what it is.

**Architecture:** See the index, `docs/plans/2026-10-17-s3-operations-errors-plan.md` (Shared vocabulary, R1, R15, R17, R18, A4, A5, "Status and dispatch"). Requires S1 (merged); does not wait for S2 (OR1). Go commands use the index's `GOTEST`/`GOBOX`/`GOLINT` forms.

**Tech Stack:** Go, gin, testify; React 19, zustand, vitest + Testing Library.

**Spec:** architecture §6 bullets 2 and 4; the coordinator input in R18.

---

### Task 1: One error envelope

**Files:**
- Create: `api/internal/domain/problem/code.go` (the `Code` enum and `Status()`), `api/internal/domain/problem/problem.go` (`Problem`, `Error`, `New`, `Errorf`, `CodeOf`, `Of`, `DataOf`, the nine category sentinels), `api/internal/domain/problem/foreign.go` (the table for errors Crowbar does not own: `fs.ErrNotExist`→NotFound, `fs.ErrExist`→Conflict, `fs.ErrInvalid`→InvalidArgument, `asynxmodels.ErrNotFound`→NotFound, `asynxmodels.ErrValidation`→Unprocessable, `asynxmodels.ErrPipelineFailed`→Conflict, `context.DeadlineExceeded`→Timeout)
- Test: `api/internal/domain/problem/{code_test.go,problem_test.go,foreign_test.go}`
- Delete: `api/internal/app/apperr/` (71 lines, and its tests), `api/internal/api/libs/status.go` (327 lines), `api/internal/api/libs/status_test.go` (its cases move to `problem_test.go`/`foreign_test.go` as `CodeOf` cases with the same expectations)
- Modify: `api/internal/api/libs/response.go` (`Fail`, `WriteErr` sets `code`, delete `WriteErrCode`, `noteFailure` stays), `api/internal/api/libs/delete_consent.go` (`WriteDeleteErr` → `Fail`), `api/internal/api/v0/endpoints/chat/handlers/presentation.go` (`writeCodedErr` → `Fail`; the prompt/catalog code helpers `PromptErrorCode`/`CatalogErrorCode` in `app/usecases/chat/runner.go:237,254` become `*problem.Error` sentinels), `chat/handlers/switch.go:38` (its `writeCodedErr` call), `api/internal/api/v0/endpoints/documents/handlers.go` (A4: `documentError` → `Fail`; the two S5 code constants move to `problem`)
- Modify (sentinels, each re-declared `problem.New(<code status.go gives it today>, <same message>)`): every sentinel `libs/status.go` names today (its imports at refresh): `core/terminal` (`ErrCommandNotFound` → FailedDependency, `terminal.go:45`), `engine/git` (`ErrAuthFailed` → Forbidden, `errors.go:17`), `engine/search`, `engine/fs/safepath` (`ErrFileTooLarge` → TooLarge, `ErrPathEscapesWorkspace` → InvalidArgument; `engine/fs/internal/content` aliases the first), `app/usecases/workspace`, `app/usecases/project`, `app/usecases/chat` (agentusecase; `ErrToolUnauthorized` → Forbidden, an alias of `tools.ErrUnauthorized` at `aliases.go:222`), `app/repositories/{chat,node}` (`ErrNotFound`), `app/repositories/chat/attachments` (repoattachments), `app/usecases/branchreview`, `domain/work_at_risk.go:35` (`ErrWorkAtRisk` → `WorkAtRisk` code, its list in `Data`), and `asynxmodels` (foreign table)
- Modify (mechanical): every `apperr.X` → `problem.X` (62 non-test files, 75 test files at refresh); every `status, msg := libs.StatusAndMessage(err); libs.WriteErr(c, status, msg)` pair → `libs.Fail(c, err)` (106 sites at refresh)
- Modify (carry from T11b): `api/internal/core/gateway/transports/socket.go:73` returns the bare `ErrDaemonRunning` when `conn.Close()` succeeds and `errors.Join(ErrDaemonRunning, closeErr)` only when it fails, so the sentinel keeps its identity
- Modify: `api/internal/bootreport_test.go` (A5 test)
- Oversize (R15): `web/src/lib/api.ts` 714 (split below); `web/src/__tests__/lib/api.test.ts` is 497, so the moved cases leave it, none are added
- Test (black box): `api/tests/regression_refused_create_code_test.go`
- Web: create `web/src/lib/errors/problem.ts`, `web/src/lib/errors/api-error.ts` (`ApiError`, `workAtRiskOf`, `isNotFoundError`, `isDaemonUnavailableError`, moved out of `lib/api.ts`); split `web/src/lib/api.ts` (714 lines) under 500 by responsibility (R15): the fetch core (`apiFetch`, `apiFetchRaw`, `readEnvelope`, `sharedRead`, retry) to `web/src/lib/api/fetch.ts` (beside the existing `lib/api/{documents,sidebar-placement,workspace}.ts`), endpoint functions stay; update importers, no re-exports. The two `res.json().catch(() => null)` at `api.ts:169,190` (counted `web.swallowed-promise` sites) become the handled fallback of Step 6, so the counter drops by 2
- Test (web): create `web/src/__tests__/lib/errors/problem.test.ts`; extend `web/src/__tests__/lib/api.test.ts` (cases about the fetch core move to `web/src/__tests__/lib/api/fetch.test.ts`, named for the source they exercise)

**Interfaces:**
- Produces: the `problem` package exactly as in the index's Shared vocabulary; `func Fail(c *gin.Context, err error)` in `libs`; `WriteErr(c *gin.Context, status int, msg string)` unchanged in signature, now writing `code: categoryFor(status)` (400 invalid_argument, 403 forbidden, 404 not_found, 409 conflict, 413 too_large, 422 unprocessable, 424 failed_dependency, 502 bad_gateway, 503 unavailable, 504 timeout, else internal); TS `Problem`, `ProblemCode`, `problemOf`; `ApiError` gains `readonly problem: Problem` (its `code` field is deleted: callers read `problem.code`).
- Consumes: nothing from S3.

- [ ] **Step 1: List the specific codes.** `grep -rn 'ErrorCode(\|WriteErrCode\|Code: *"' api/internal --include='*.go' | grep -v _test` and `grep -rn "\.code ===\|code === '" web/src --include='*.ts*' | grep -v __tests__` (8 web files at refresh). Every code a handler emits and the client branches on becomes a `Code` constant (spelling unchanged) with the status it is written with today; at refresh that includes S5's `host_document_requires_local_transport` and `daemon_state_document` (A4) and `core/bootgate`'s `daemon_` prefix codes (A5, written by `gate.go:16,142`). Record the list in the commit body.

- [ ] **Step 2: Write the failing tests.**

`api/internal/domain/problem/code_test.go`:
```go
func TestCode_StatusTable(t *testing.T) // table: every constant → its status (index Shared vocabulary); Interrupted and Partial → 500
func TestCode_UnknownCodeIsInternal(t *testing.T) // Code("nope").Status() == 500
```
`problem_test.go`:
```go
func TestCodeOf_FindsTheFirstCodedErrorInTheChain(t *testing.T) // fmt.Errorf("x: %w", problem.New(Conflict,"held")) → Conflict
func TestCodeOf_SentinelIdentityIsKept(t *testing.T) // errors.Is(wrapped ErrNotFound, ErrNotFound) true; errors.Is(New(NotFound,"a"), ErrNotFound) false
func TestOf_MessageIsTheWholeChain(t *testing.T) // Of(fmt.Errorf("import: %w", New(Conflict,"held"))).Message == "import: held"
func TestDataOf_ReturnsTheCodedErrorsData(t *testing.T)
func TestCodeOf_NilAndPlainErrorsAreInternal(t *testing.T)
```
`foreign_test.go`: one case per foreign row above, plus every case moved from `libs/status_test.go` as `CodeOf(err).Status()` equal to the status that test asserted.

`api/internal/api/libs/response_test.go` (extend):
```go
func TestFail_WritesTheCodeItsStatusAndData(t *testing.T) // Fail(c, New(WorkAtRisk,...){Data:list}) → 409, body code "work_at_risk", data == list
func TestWriteErr_AlwaysSetsTheCategoryCode(t *testing.T) // WriteErr(c, 404, "x") → code "not_found"; 418 → "internal"
```
`api/internal/bootreport_test.go` (extend), A5:
```go
func TestBootReport_RefusalCodesAreProblemCodes(t *testing.T) // "daemon_"+string(bootgate.StateMigrating) == string(problem.DaemonMigrating); same for StateHalted
```
Black box, `api/tests/regression_refused_create_code_test.go`:
```go
// TestRegression_RefusedCreateCarriesItsCode: a create the daemon refuses says why in a code
// the client can branch on, not only in prose.
func TestRegression_RefusedCreateCarriesItsCode(t *testing.T)
```
Uses `newImportFixture(t, h)`, imports `feature/a` once (wait for its workspace on the repo's chats stream as `regression_import_branches_test.go` does), then creates a chat with its own worktree on branch `feature/a` again (`POST /v0/projects/:p/repos/:r/chats` with `ownWorktree: true, branch: "feature/a"`). Assert status 409, envelope `code == "conflict"`, `error` contains `feature/a`.

`web/src/__tests__/lib/errors/problem.test.ts`:
```ts
it('reads the code and message an ApiError carries')
it('an Error without a code is internal and keeps its message')
it('a thrown string becomes an internal problem with that text')
```
`web/src/__tests__/lib/api/fetch.test.ts` (moved cases plus):
```ts
it('a refusal envelope becomes ApiError.problem with the daemon code and message')  // 409 {error:'held', code:'conflict'}
it('a refusal body that is not JSON becomes an internal problem naming the status') // 502 text/html → problem {internal, '502 Bad Gateway'}
```

- [ ] **Step 3: Run them to verify they fail.**

```bash
GOTEST ./internal/domain/problem/... ./internal/api/libs/... ./internal/
GOBOX 'TestRegression_RefusedCreateCarriesItsCode'
cd web && bunx vitest run src/__tests__/lib/errors/problem.test.ts src/__tests__/lib/api/fetch.test.ts
```
Expected: compile failures (`problem` does not exist), and the black-box test fails on `code == ""`.

- [ ] **Step 4: Implement the `problem` package** (Shared vocabulary). `Status()` is one `switch` with a case per constant and `default: 500`. `CodeOf` walks with `errors.As` for `*Error`, then the foreign table in order, then `Internal`. `Error.Error()` is `Message` when `Err` is nil, else `Message + ": " + Err.Error()` (and `Err.Error()` when `Message` is empty).

- [ ] **Step 5: Re-declare the sentinels and move the mapping.** For each sentinel `status.go` names, change its declaration to `problem.New(<code>, <its current message>)` where `<code>` is the category whose status `status.go` gives it. Delete `status.go`. Replace every `StatusAndMessage`+`WriteErr` pair with `libs.Fail(c, err)`; `WriteDeleteErr` and `writeCodedErr` call `Fail`. Rewrite `apperr.X` → `problem.X` and delete `app/apperr`. `libs` now imports no usecase or engine package: `grep -n '"github.com/char2cs/crowbar/api/internal/\(engine\|app\)' api/internal/api/libs/*.go` prints nothing.

- [ ] **Step 6: Implement the web half.** `api-error.ts`: `class ApiError extends Error { readonly status: number; readonly problem: Problem; readonly data?: unknown }`. The fetch core builds it from the envelope (`code` and `error`), from the proxy header (`daemon_unavailable`), or, for a body that is not JSON, `{ code: 'internal', message: \`${status} ${statusText}\` }` read through `res.text()` and a `JSON.parse` whose `catch` returns that fallback (a handled branch, not a swallow). Callers that read `err.code` read `err.problem.code`. Split `lib/api.ts` as listed.

- [ ] **Step 7: Run the tests, then the checks.**

```bash
GOTEST ./internal/domain/... ./internal/api/... ./internal/app/... ./internal/engine/... ./internal/core/... ./internal/
GOBOX 'TestRegression_RefusedCreateCarriesItsCode|TestRegression_DeleteWorkAtRisk|TestRegression_Import'
GOLINT   # golangci-lint, scripts/deadcode-check.sh, go vet -tags noEmbed ./...
(cd api && go vet -tags 'integration noEmbed' ./tests/...)
cd web && bunx vitest run src/__tests__/lib src/__tests__/components/layout/removal-commit.test.ts && bun tsc --noEmit
node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs
```
Expected: PASS. `go vet` on `./tests/...` compiles the kit without running it. `web.swallowed-promise` 27 → 25.

- [ ] **Step 8: Move the budgets** (R1): `go.app-misc` −(apperr lines), `go.http` −(status.go lines); `go.domain` +(the same total minus the lines the collapse removed). Body states each number.

- [ ] **Step 9: Commit**

```bash
git add <each file in this task's Files list that git status --short shows changed, by path>   # git rm the deleted ones by path
git commit -m "refactor(api): one error envelope with a closed code enum in domain

Every error response carries a code from domain/problem and the status the
code maps to; libs.Fail replaces the status union over eight packages.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```

(Step 5 touches many files: stage each by path from `git status --short`, never a directory.)

---

### Task 2: `report()`, global reporting, and daemon states

**Files:**
- Create: `web/src/lib/errors/report.ts`, `web/src/lib/ws/daemon-probe.ts`
- Modify: `web/src/lib/ws/connection-store.ts` (52 lines), `web/src/lib/ws/manager.ts` (225; a socket that closes without having opened calls `probeDaemon()`), `web/src/lib/api/fetch.ts` *(S3 T1)* (a `daemon_migrating`/`daemon_halted` refusal calls `reportDaemonRefusal`), `web/src/components/layout/connection-indicator.tsx`, `web/src/features/window/components/daemon-health-listener.tsx` (drop its on-mount `bestEffort` health read at `:92`; its `daemon:halted` handler already calls `useDaemonHaltStore.getState().halt`, and stays), `web/src/main.tsx` (call `installGlobalReporting()` once at boot)
- Unchanged owner (F1): `web/src/features/window/stores/daemon-halt-store.ts` and `components/daemon-halt-gate.tsx` keep halted; T2 adds callers of `halt()`, not a second copy
- Test: create `web/src/__tests__/lib/errors/report.test.ts`, `web/src/__tests__/lib/ws/daemon-probe.test.ts`; extend `web/src/__tests__/lib/ws/connection-store.test.ts`, `lib/ws/manager.test.ts`, `components/layout/connection-indicator.test.tsx`, `features/window/components/daemon-health-listener.test.tsx`, `lib/api/fetch.test.ts` *(S3 T1)*

**Interfaces:**
- Consumes: T1 `Problem`, `problemOf`, `ApiError`.
- Produces:

```ts
// lib/errors/report.ts
export interface ReportContext { action: string; key?: string }
export function report(error: unknown, context: ReportContext): void
// toastManager.add({ id: key ?? action, type: 'error', title: action, description: problemOf(error).message })
// AbortError and Monaco Canceled → nothing; a daemon_migrating/daemon_halted problem → reportDaemonRefusal, no toast (F8); never throws
export function installGlobalReporting(target: Window = window): () => void // 'unhandledrejection' → report(reason, { action: 'Something went wrong' })
// lib/ws/connection-store.ts
export type ConnectionStatus = 'idle' | 'connected' | 'disconnected' | 'migrating'
export interface Migrating { message: string; step?: { index: number; total: number; name: string } }
interface ConnectionState { status: ConnectionStatus; migrating: Migrating | null }
export function reportMigrating(m: Migrating): void // ignored while any channel is open
export function reportDaemonRefusal(problem: Problem): void  // daemon_migrating → reportMigrating; daemon_halted → useDaemonHaltStore.getState().halt(message); else nothing
export type DaemonHealthState = { kind: 'migrating'; migrating: Migrating } | { kind: 'halted'; message: string } | null
export function daemonStateFromHealth(data: unknown): DaemonHealthState // health `data`: status 'migrating' (detail {index,total,step:{name}}) or 'halted' (detail {message}); else null
// lib/ws/daemon-probe.ts
export function probeDaemon(): Promise<void> // one GET /v0/health at a time; migrating → reportMigrating, halted → halt(); a failed read leaves the state as it is
```
Status derivation: an open channel → `connected` and `migrating` cleared; else `migrating` when set; else `disconnected` when channels exist; else `idle`. Halted is not a connection status (F1): `DaemonHaltGate` replaces every route while `useDaemonHaltStore.message` is set. `lib/ws/` may import the halt store (a store-to-store call, no `components/` import).

- [ ] **Step 1: Write the failing tests.**

`report.test.ts` (mock `@/lib/toast-manager` only):
```ts
it('toasts the action as the title and the problem message as the description')
it('keys the toast by context.key, else by the action')
it('ignores an AbortError and a Monaco Canceled')
it('a daemon_halted problem halts instead of toasting')            // F8
it('reports an unhandled rejection once installed, and nothing after dispose')
```
`connection-store.test.ts`:
```ts
it('a daemon_migrating refusal is a state, not a drop')          // reportChannelState('/a', false); reportDaemonRefusal({code:'daemon_migrating', message:'…'}) → status 'migrating'
it('a daemon_halted refusal halts the daemon-halt store with its reason') // F1: useDaemonHaltStore.message set; status unchanged
it('an open socket clears the migrating state')                    // Review Focus 5
it('a refusal while a socket is open does not change the status')  // Review Focus 5
```
`daemon-probe.test.ts` (stub `fetch` on `globalThis`):
```ts
it('a migrating health read sets the step from detail')            // detail {index:2,total:5,step:{number:7,name:'chat-ledger'}} → step {2,5,'chat-ledger'}
it('a halted health read halts the daemon-halt store with its reason')
it('one probe at a time: a second call while one is in flight shares it')
it('an unreachable daemon leaves the state unchanged')
```
`manager.test.ts`:
```ts
it('a socket that closes before it opened probes the daemon once')
it('a socket that closed after opening does not probe until a reconnect fails')
```
`connection-indicator.test.tsx`:
```ts
it('a migrating daemon shows its step instead of Reconnecting')    // toast {message:'Crowbar is upgrading its data', description:'Step 2 of 5: chat-ledger'}; no toast with 'Reconnecting…'
it('the upgrade toast gives way to Reconnected when a socket opens')
```
`daemon-health-listener.test.tsx`: delete the on-mount health-read cases (the read goes); the existing supervisor `daemon:halted` case stays as it is.
`fetch.test.ts`:
```ts
it('a daemon_migrating refusal reports the state before it throws')
it('a daemon_halted refusal halts the daemon before it throws')
```

- [ ] **Step 2: Run them to verify they fail.**

```bash
cd web && bunx vitest run src/__tests__/lib/errors/report.test.ts src/__tests__/lib/ws src/__tests__/components/layout/connection-indicator.test.tsx src/__tests__/features/window/components/daemon-health-listener.test.tsx src/__tests__/lib/api/fetch.test.ts
```
Expected: FAIL (missing modules; the indicator still shows "Reconnecting…").

- [ ] **Step 3: Implement** the interfaces above. `ConnectionIndicator` reads `status` and `migrating` with two narrow selectors. `migrating`: show at once (no `SHOW_DELAY_MS`), `{ type: 'info', message: 'Crowbar is upgrading its data', description: step ? \`Step ${index} of ${total}: ${name}\` : migrating.message, key: TOAST_KEY, duration: 0 }`, updated in place as steps advance. `disconnected` keeps today's delayed "Backend unavailable · Reconnecting…". Leaving either for `connected` shows "Reconnected" as today. `DaemonHealthListener` keeps its `unhealthy`/`restarted`/`dead` toasts and its `daemon:halted` → `halt()` handler, and loses the on-mount health read (and with it a `bestEffort` call).

- [ ] **Step 4: Run the tests.** Same command as Step 2, plus `src/__tests__/lib/api.test.ts`. Expected: PASS. Then `bun tsc --noEmit` and the per-commit web checks on the touched paths.

- [ ] **Step 5: Live check** (OR6: the noVNC container, isolated `CROWBAR_HOME` under the worktree's `/.crowbar/`; refuse to continue unless the socket path starts with the worktree path). Stop the dev daemon, start it again with a pending migration step if the engine's dev catalog offers one; otherwise drive the state from the browser console by calling `reportMigrating` and `reportDaemonRefusal({code:'daemon_halted', …})` from their modules, and screenshot the migrating toast and the halt screen. Record which path was used.

- [ ] **Step 6: Commit**

```bash
git add <each file in this task's Files list that git status --short shows changed, by path>   # git rm the deleted ones by path
git commit -m "feat(web): one report(), and a migrating or halted daemon is a state, not a drop

Unhandled rejections reach the user through report(); the connection store
learns daemon_migrating and the indicator shows the step; every halt source
reaches the existing halt store.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```
