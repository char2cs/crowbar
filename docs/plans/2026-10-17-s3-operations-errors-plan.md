# S3: Operations and error discipline. Implementation Plan (index)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks are sequential: each one assumes every earlier task and all of S1 and S2 are committed. T8 (asynx) has an external precondition and nothing after it depends on its code, so it may run last if the precondition is not yet met.

**Goal:** Every piece of work that outlives its request is an operation with an id and a terminal state the user sees. Every HTTP error carries one envelope with a code from one closed enum. The client has one `report()`. Both lossy error channels, `pending-creates`, every landing watcher, `reparent-settle` and `bestEffort` are deleted. `_ =`, empty `catch` and swallowed promises reach 0.

**Architecture:** The daemon gains an `operations` table (in `view.db`), a small service that runs detached work, records its outcome and publishes every change on a global `/v0/operations` stream, and a boot pass that fails what a restart interrupted. Each of the 16 routes that answer 202 and keep working afterwards today returns the operation. The client keeps one operations store, fed by that stream and by its own in-flight requests (local operations). Sidebar rows, panes and toasts read outcomes from it. A create is drawn from its operation until the entity it names arrives; nothing waits for a row to land. `domain/problem` holds the closed code enum and the code→status table; `libs.Fail(c, err)` is the only way a handler turns an error into a response.

**Tech Stack:** Go (`api/`, `-tags noEmbed`), gin, gorm + SQLite, gorilla/websocket, asynx (v1 release, T8), testify, the `api/tests` black-box harness. React 19, zustand, TanStack Router, `@/components/ui/*` (base-ui toast), phosphor icons, vitest + Testing Library (`web/src/__tests__/`), ESLint flat config.

**Spec:** `docs/plans/2026-10-16-crowbar-architecture.md` §0 principle 3 ("Operations are entities"), §3 (Operations in the journal and client entity store), §5 (operation lifecycle diagram), §6 (error-surfacing discipline), §7 row "Branch import/create fails with no reason", §8 bullet "Operations are rows", §11 row S3. The S1 and S2 plans (`2026-10-17-s1-chat-ledger-plan.md`, `2026-10-17-s2-chatstore-plan.md` and their sub-plans) for what exists when S3 starts. `docs/plans/2026-10-17-migration-engine-plan.md` for the boot gate. Executors read the sections a task cites.

## Plan files

| File | Tasks |
|---|---|
| `docs/plans/2026-10-17-s3a-envelope-and-report-plan.md` | T1 one error envelope · T2 `report()`, global reporting, and a migrating or halted daemon is a state, not a drop |
| `docs/plans/2026-10-17-s3b-operations-core-plan.md` | T3 operations: table, service, stream, client store; git verbs move onto it · T4 workspace verbs are operations; the working overlay and `reparent-settle` go |
| `docs/plans/2026-10-17-s3c-creates-and-imports-plan.md` | T5 creates and imports are drawn from operations (the user's bug) · T6 repos and projects are operations; the last lossy channel goes |
| `docs/plans/2026-10-17-s3d-discipline-and-gate-plan.md` | T7 web: no swallowed failure · T8 asynx v1 and durable read models · T9–T11 Go: no discarded error · T12 gate |

12 tasks, one commit each. T5 is the task the user's complaint names: "Sometimes Crowbar cannot import/create a branch nor says why it failed."

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
- Commits stage only the files the task touched, by path (`git status --short`, then `git add <path>…`, `git rm <path>…` for deletions). Another session may hold uncommitted edits in the same directories; a directory-wide `git add` would commit them.
- Running tests: targeted only, never a full suite. Web: `cd web && bunx vitest run <paths>`; types `cd web && bun tsc --noEmit` (never `bunx tsc`, a different package). Before each web commit: `bunx eslint <touched> --max-warnings 0`, `bunx prettier --check <touched>`, `bun run knip`, `bun run lint:baselines`, `bun run lint:doctor`. Go, always in this form (no inherited `HOME`, no inherited `CROWBAR_HOME`):

```bash
cd api && T=$(mktemp -d) && env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin GOPATH=/Users/char2cs/go GOMODCACHE=/Users/char2cs/go/pkg/mod GOCACHE=/Users/char2cs/Library/Caches/go-build HOME=$T/home CROWBAR_HOME= go test -tags noEmbed <pkgs>; rm -rf $T
```

  Black box: the same env prefix with `go test -tags 'integration noEmbed' -count=1 -run '<regex>' ./tests/` (the top-level `./tests/` package only). NEVER run `-tags integration` tests in `api/tests/integration/agent`, `api/tests/integration/scripted` or `api/tests/kit` (the kit reads the real `~/.claude.json` and links the real `~/.codex/auth.json`); for those, compile only: `go vet -tags 'integration noEmbed' ./tests/...`. Never `go test ./...`. Then `make lint`, `make deadcode`, `go vet -tags noEmbed ./...`. Below, "`GOTEST <pkgs>`" means the guarded command above.
- Safety: never connect to `~/.crowbar/crowbar.sock` or open anything under `~/.crowbar/state`; dev runs use the worktree's `CROWBAR_HOME` (`make dev`, `make dev-desktop`). Never read or write `~/.claude` or `~/.codex`. Check `df -h /System/Volumes/Data` before any build or `bun install`; under 8 GB free, stop and report. Never bare `git stash`. Never push. No PRs. No scratch files inside the worktree (sibling sessions commit with `git add -A`); `git add` names paths, never `-A`.
- On-disk changes go through the migration engine's catalog (`api/internal/adapter/migrations`, engine `api/internal/core/migrate`). S3 creates new tables only (R12); it rewrites no existing data.

## Rulings (open decisions resolved for S3)

| # | Decision | Ruling |
|---|---|---|
| R1 | `make ratchet` per task | As S2 R1: tasks do not run it. Each task checks the 500-line cap with `wc -l` on what it touched and states, in its commit body, the net line change per module key it touched. A task that moves a responsibility between modules (T1: `app/apperr` and `libs/status.go` → `domain/problem`; T3: detached work from `api` into `app/usecases/operation`) edits those two keys in `scripts/ratchet/loc-budgets-baseline.json` by the moved amount in the same commit, as `loc-budgets.mjs` prescribes; `go.total` and `web.total` are never raised. T12 runs `make ratchet ARGS=--update` once. |
| R2 | Which work is an operation | Work that continues after the response: the 16 routes that run detached work today (worktree `…/chats/:id/{sync,merge-into-parent,rebase-onto-parent,reparent,retry-provision,detach-holder}` and `…/chats/import-batch`; `/v0/chats/:chatId/git/{push,fetch,pull,merge,rebase}`; `POST`/`DELETE` of projects and of repos; `sync`, `git/fetch`, `git/merge` and `git/rebase` are called only by tests today, and whether they stay is S7's), plus daemon-started jobs (boot resume of a repo or project delete, a workspace purge that gives up). Prompt submission and compact keep their 202: their outcome channel is ledger rows (architecture §5 outbox), not an operation. A handler that finishes its work inside the request answers `204` (`libs.WriteNoContent`), not 202 (T6). |
| R3 | Operation id | Client-minted UUID sent as `Idempotency-Key`; the daemon mints one when the header is absent (CLI and MCP callers). Same key with the same kind and subject returns the existing operation in whatever state it is in. Same key with a different kind or subject is 409 `operation_id_conflict`. A malformed key is 400. |
| R4 | Validation | Anything a route can check before 202 stays a synchronous 4xx and creates no operation (today's `CheckRepoImportable`, `branchesMissingFromRemote`, default-branch refusal, body binding). |
| R5 | Terminal states | `accepted → running → succeeded \| failed` (architecture §5). An operation with items fails if any item failed, with code `partial` and a message counting them; each failed item carries its own `Problem`. Shutdown cancels running operations; a cancelled one is `failed{interrupted}`. At boot, every `accepted`/`running` row is set to `failed{interrupted, "Crowbar stopped before this finished"}` before the API opens. |
| R6 | Retention | Operations are kept across restarts. At boot, after R5's pass, settled operations beyond the newest 500 by `updatedAt` are deleted. No timer prunes anything. Dismissing (`DELETE /v0/operations/:id`) sets `dismissed`; it hides the toast and chip, never the entity's last error (R9). |
| R7 | Stream | A dedicated global stream `GET /v0/operations` (WebSocket) on the existing `ws.StreamDef`/`Broadcaster` machinery: on connect a snapshot of every retained operation, then one bare `Operation` JSON frame per change. Frames carry `version` (incremented on each change); the client ignores a frame whose version is not newer. S4 folds this stream into the one stream (D3). |
| R8 | "Succeeded: subject version advances" while entity frames ride other sockets | The daemon publishes an operation's terminal frame only after the work's entity writes returned (projections and hub broadcasts already ran). Sockets are not ordered against each other, so the client never treats "succeeded" alone as "the rows are here": a create's row is drawn from its operation item until the entity with that id is in its store (`isEntityPresent`), and `whenSettled(opId)` resolves only when the operation succeeded and every created item is present (every deleted item absent). This is a join of two server facts, not a watcher: no timeout, and a failure carries its reason. S4's single ordered stream removes the join. |
| R9 | `lastError` | Deleted from the Workspace aggregate, `Repository`, `Project` and every DTO (`WorkspaceDTO`, `ChatWorktreeDTO`, `RepoDTO`, `ProjectDTO`) and the TS types. The client projects it: `selectEntityProblem(state, id)` returns the `Problem` of the newest operation naming `id` (as subject or item) when that operation failed, else null. A later operation on the same entity that succeeds clears it. The Workspace event log keeps old `/lastError` patches; they are stripped at read by the existing upcaster (R13). `Repository.Deleting`/`Project.Deleting` stay: they are entity lifecycle state, not error channels. |
| R10 | Who names a created row | The client mints the ids of the rows it draws while a create is in flight: the chat id of a thread or branch create, and one chat id per branch of an import (`branches: [{name, chatId}]`). The daemon uses them (409 `id_conflict` if taken, 400 if not a UUID). Ancestor branches an import adds from the PR graph keep daemon-minted ids and arrive as items. Repo and project creates do not mint ids: their modal waits on `whenSettled`, as it waits on `awaitEntity` today. |
| R11 | Local operations | A request the client starts that the daemon finishes inside the response (thread create, branch create, single-branch import) is a local operation in the same store: `accepted` when sent, `succeeded` with the reply's items, or `failed` with the reply's `Problem`. A server frame with the same id always replaces a local entry. Local operations live in memory; a reload drops them (the entity, if created, is in the stores by then). |
| R12 | Schema | `operations` is a new gorm table in `view.db` created by `AutoMigrate` at open, as every `view.db` table is. It changes no existing row, so it needs no catalog step. The `last_error` columns of `projects` and `repositories` stay on disk unread (gorm never drops a column); removing them belongs to S8's layout step (amendment A6). |
| R13 | Workspace `LastError` in the event log | `workspace.SchemaVersion` goes 2 → 3 and `StripRetiredPlacementFields` (renamed `StripRetiredFields`) also drops `/lastError` patches, registered as the 2 → 3 upcaster. This is the mechanism v1 → v2 already uses; it reads old events and writes nothing (D6). |
| R14 | Working overlay | `BeginWork`/`EndWork`/`IsWorking` and the `inflight` map in `app/repositories/container.go` are deleted (T4): "this workspace has work in flight" is an operation in `accepted`/`running` naming it. The DTO `working` keeps only the agent half (`WorkingFor`). Rows show `working || selectEntityBusy(id)`. |
| R15 | 500-line cap on files S3 edits | A task that edits a production file over 500 lines splits it by responsibility in that task, each part named for what it exports, so the module does not grow. Known: `lib/api.ts` 714 (T1), `components/sidebar/sidebar-row.tsx` 680 (T3), `drop-actions.ts` 999 (T4), `components/sidebar/lib/row-actions.ts` 671 (T5), `rows-from-repo.ts` 702 (T5), `repos/handlers/repos.go` 1051 (T6), `usecases/project/project_import.go` 926 (T6), `project_delete.go` 560 (T6), `repositories/workspace/workspace.go` 874 (T4), `repositories/container.go` 852 (T4), `api/v0/container.go` 904 (T3), `app/container.go` 633 (T3), `git/handlers/write.go` 664 (T3). An oversize test file is never grown: cases for deleted behaviour go, new cases go in the test file named for the source they exercise (S1 R17). |
| R16 | Where client code lives | `report()` and the `Problem` mirror in `web/src/lib/errors/` (module `web.core`); the operations store and its helpers in `web/src/lib/store/` ("server-state-adjacent structures", `CLAUDE.md`; module `web.sidebar`, which also loses `pending-creates`). S4/S9 re-home them under `core/`. |
| R17 | What `report()` routes | `report(error, context)` is the toast route; it ignores `AbortError` and `OperationFailedError` (the operations watcher already showed that failure) and never throws. Inline errors render `problemOf(error).message`; operation chips and row badges render from the operations store. All three read the same `Problem`, so no failure has two wordings. `installGlobalReporting()` sends every `unhandledrejection` to `report` (D1). |
| R18 | Migrating or halted daemon (coordinator input, from the migration engine review) | The connection store learns these as states, not drops. Sources: (a) any API response whose envelope code is `daemon_migrating` or `daemon_halted`; (b) when a channel's socket fails to open, one `GET /v0/health` per failed attempt (deduped while one is in flight), riding `wsManager`'s existing reconnect backoff, so no new timer; (c) the desktop supervisor's `daemon:halted` event. Any socket opening clears it. `ConnectionIndicator` is the one owner of the toast: migrating shows "Crowbar is upgrading its data" with "Step i of n: <name>" from health `detail`; halted shows "Crowbar could not start" with the reason. `DaemonHealthListener` keeps the supervisor's `unhealthy`/`restarted`/`dead` toasts and stops reading health itself. |
| R19 | Go discards that cannot fail usefully | Hash writes (`hash.Hash.Write` never returns an error, and errcheck already excludes it) are written as bare calls, not `_, _ =`. `Close` on a read-only file or a response body is joined into the returned error with `errors.Join`. A deadline set on a connection that is then written returns its error. A cleanup after a failure (`os.Remove(tmp)`) is joined into the failure. A boot or background job with no caller logs with `slog.Warn` and the error; one the user can act on is an operation (R2). `_ = ctx`-style silencers are deleted. T11 turns on errcheck `check-blank`. |
| R20 | Durable read models (T8) | The read-model projections of Workspace, Chat, ReviewThread and Node become asynx durable subscribers (`WithSubscriber`) that return their save error, so a failed save is retried and parks instead of being logged and dropped. Their row delete moves to the `models.ForgetEventName` marker. Hubs stay live `Subscribe` handlers (clients re-snapshot on reconnect). The OCC retry loops on `ErrPipelineFailed` are deleted (v1 serializes same-aggregate commands). Reconcile-on-open stays until S8 merges the databases (it is the upgrade path's guarantee for events committed before T8). Sync projections are not used: they must write through the event store's transaction, and the read models live in other files until S8. |
| R21 | asynx release | S3 owns the bump. Precondition: the user names an asynx release after v1.0.0 that fixes the verification's three Important findings: NF1 (a `Get` racing `Forget` must not resurrect the aggregate through an unlocked auto-snapshot), NF2 (one busy aggregate must not stall the other aggregates of its shard) and NF7 (recovery must be linear per aggregate; `Build` must not take quadratic time on a backlog), and whose release passes those probes. Crowbar carries no workaround for a library defect. |

## Decisions that change what the user sees

Ruled so the plan can proceed; the user may overturn any before its task.

| # | Change | Task |
|---|---|---|
| V1 | A failed background job (push, pull, merge into parent, rebase, reparent, retry, detach, import, repo/project add or remove) shows a toast naming the action and the reason, once, when it fails. The row it concerns keeps the reason (icon + tooltip, as placeholders do today) until a later operation on it succeeds. Today most of these fail silently or only in the daemon log. | T3–T6 |
| V2 | An import shows one row per requested branch while it runs. A branch that cannot be imported keeps its row with the reason and a Dismiss button; the others land normally. | T5 |
| V3 | A creating row never times out ("The new row never arrived from the daemon" after 30 s is gone). It spins until the daemon says it succeeded or why it failed. A daemon restart turns it into "Crowbar stopped before this finished". | T5 |
| V4 | A daemon that is upgrading its data shows "Crowbar is upgrading its data · Step 2 of 5: <step>" instead of "Backend unavailable · Reconnecting…". A halted daemon shows one "Crowbar could not start" toast with the reason, never next to "Reconnecting…". | T2 |
| V5 | The workspace spinner during a git or hierarchy job comes from the operation, so it also shows after a reload while the job still runs. | T4 |
| V6 | Adding a repository or project waits in its dialog as today, without the 30 s "timed out awaiting entity" failure; a failure shows the daemon's reason (toast) and re-enables the dialog. | T6 |

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
	DaemonReportUnencodable Code = "daemon_report_unencodable" // 503
	// Operation-only (never an HTTP status):
	Interrupted Code = "interrupted"
	Partial     Code = "partial"
)
func (c Code) Status() int                         // the table; Interrupted/Partial → 500 if ever written
type Problem struct { Code Code `json:"code"`; Message string `json:"message"` }
type Error struct { Code Code; Message string; Data any; Err error } // Error(), Unwrap()
func New(code Code, message string) *Error          // sentinels: var ErrBranchHeld = problem.New(problem.Conflict, "...")
func Errorf(code Code, format string, args ...any) error // %w wraps
func CodeOf(err error) Code                         // first *Error in the chain; else the foreign table (fs, context, asynx models); else Internal
func Of(err error) Problem                          // {CodeOf(err), err.Error()}
func DataOf(err error) any                          // the first *Error's Data
var ErrNotFound, ErrLocked, ErrInvalidArgument, ErrUnavailable, ErrConflict,
	ErrUnprocessable, ErrTimeout, ErrBadGateway, ErrFailedDependency *Error // replace app/apperr 1:1
```

`errors.Is` keeps identity semantics (a sentinel matches only itself); code checks use `problem.CodeOf(err) == problem.X`.

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
type Spec struct { ID string; Kind domain.OperationKind; Subject domain.EntityRef; ProjectID, Label string; Items []domain.OperationItem }
type Progress interface { Item(item domain.OperationItem) } // replaces the item with the same Label (labels are unique per operation), else appends; publishes
type Work func(ctx context.Context, p Progress) error
func New(store store.ScopedStore[domain.Operation, string], publish func(domain.Operation), clock func() time.Time) *Service
func (s *Service) Start(ctx context.Context, spec Spec, work Work) (domain.Operation, error) // R3 idempotency; returns the accepted op; work runs detached
func (s *Service) Fail(ctx context.Context, spec Spec, cause error) (domain.Operation, error) // records daemon-started work that already failed (T4: a purge that gives up)
func (s *Service) Dismiss(ctx context.Context, id string) (domain.Operation, error)          // NotFound, OperationRunning
func (s *Service) Recover(ctx context.Context) error // R5 boot pass then R6 prune; called once before the API opens
func (s *Service) Snapshot(ctx context.Context) ([]domain.Operation, error)
func (s *Service) Shutdown(ctx context.Context) error // cancels running work, waits for it
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
export function isEntityPresent(ref: EntityRef): boolean // reads the stores that hold each kind after S2
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
| Why the daemon is not serving | connection store (`daemon_migrating`/`daemon_halted`) | read as a dropped connection; health read once in `DaemonHealthListener` |
| The text a sidebar create row shows before the user confirms | `create-draft` store (client UI state) | `pending-creates` `naming` |

## Deletion table (S3 "Deletes" → task)

| Deleted | Task |
|---|---|
| `libs/status.go` (`StatusAndMessage`, `isNotFound`, `isBadRequest`, `isConflict`, `isPlacementConflict`, `isGitConflict`), `libs.WriteErrCode`, `api/internal/app/apperr/` | T1 |
| `web/src/lib/best-effort.ts` call in `daemon-health-listener.tsx`; the "Reconnecting…" toast during a migration or halt | T2 |
| `git/handlers/async.go` (`runAsync`, `Shutdown`), `SetLastError` from git handlers | T3 |
| `worktree/handlers/async.go` (`runAsync`, `broadcastLastError`, `WaitAsync`, `Shutdown`); `BeginWork`/`EndWork`/`IsWorking`/`inflight`; `components/sidebar/lib/reparent-settle.ts`; the purge reactor's "deferring to boot sweep" log-and-drop | T4 |
| `lib/store/pending-creates.ts`, `components/sidebar/lib/rows-from-pending.ts`, `rowRepoScope`, `panelRowsAtClick`, `failCreate`, `untilLanded`, `ROW_ARRIVAL_TIMEOUT_MS`, every `*HasLanded`/`waitFor*` watcher, `armImportLockWatch`, `IMPORT_LOCK_SETTLE_TIMEOUT_MS`, `placeholder-toast-watcher.tsx`; Workspace `LastError`, `set_last_error` command, `SetLastError` on the repository; `WorkspaceDTO.LastError`, `ChatWorktreeDTO.LastError` | T5 |
| `projects/handlers/async.go`, `repos/handlers/async.go` (`WaitAsync` everywhere), `lib/ws/await-entity.ts`, `releaseWhenGone`, `Repository.LastError`, `Project.LastError`, `RepoDTO.LastError`, `ProjectDTO.LastError`, `endpoints/internal/detached/`, `ShutdownDetached`, `libs.WriteAccepted` on every route that finishes in the request (it stays only where a ledger row is the outcome), `createRequest.ID` (ignored today) | T6 |
| `web/src/lib/best-effort.ts` (whole file and its test); every empty `catch` and `.catch(() => …)` swallow | T7 |
| OCC retry loops on `ErrPipelineFailed` (chat, workspace, reviewthread, node repositories), `saveWithRetry`/`isTransientIOError`, `WithPublishErrorHandler` log-and-drop on read models, the adapter's unused `Count` | T8 |
| Every `_ =` / `_, _ =` in non-test Go | T9–T11 |

## Module keys and counters

- New Go code: `api/internal/domain/problem/` and `domain/operation.go` (`go.domain`), `api/internal/app/usecases/operation/` (`go.service`), the operations stream def in `api/internal/api/v0/` (`go.http`; the stream def lives beside `projectsDef`), the `Broadcast/PushOperation` pair in `api/internal/app/hub` (`go.stream`). Shrinks: `go.http` (status.go, four `async.go`, coded-error helpers), `go.app-misc` (`app/apperr`), `go.store` (OCC loops, overlay, `set_last_error`), `go.dto` (four `LastError` fields).
- New web code: `web/src/lib/errors/` (`web.core`), `web/src/lib/store/{operations,run-operation,entity-presence,create-draft}.ts` and `components/sidebar/lib/rows-from-operations.ts` (`web.sidebar`). Shrinks: `web.sidebar` (pending-creates, rows-from-pending, reparent-settle, landing watchers, placeholder toast watcher, removal watcher), `web.core` (best-effort, await-entity, the error half of `lib/api.ts`).
- Counters at the end of S3: `go.discarded-errors` 0, `web.empty-catch` 0, `web.swallowed-promise` 0. `go.timers` and `web.timers` fall (the 30 s watcher timeouts, the purge reactor backoff, the OCC jitter); T12 writes whatever they measure.

## Gate baseline (measured at `74e0c7775`, before S1 and S2)

| Counter / budget | Baseline file | Measured now | After S3 |
|---|---|---|---|
| `go.discarded-errors` | 181 | 172 (S1 deletes ≈ 17 in `usecases/chat/internal/runner`, `cmd/crowbar/hook_answer.go`, `chat/handlers/hooks.go`) | 0 |
| `web.empty-catch` | 34 | 34 (S2 deletes 7: `use-prompt-queue.ts` ×2, `prompt-queue-persistence.ts` ×2, `use-workspace-agent-chats-stream.ts` ×2, `agent-chats-slice.ts`) | 0 |
| `web.swallowed-promise` | 32 | 32 (S2 deletes `use-prompt-queue.ts:374`) | 0 |
| `go.timers` / `web.timers` | 29 / 87 | 29 / 87 (S2 T13 takes `web.timers` to ≤ 75) | lower; written by T12 |
| LOC `go.total` / `web.total` | post-S2 baseline | — | not raised |

Lists at planning time came from `node scripts/ratchet/counters.mjs --list <id>`; each sweep task re-runs it, because S1 and S2 change them.

## Amendments to earlier plans

The earlier plan files are not edited; these notes override them where they meet S3.

- A1 (S2 R9): "Sidebar and tabs read the entity's server-folded `working`." After T4 that field is the agent half only; a workspace row reads `working || selectEntityBusy(id)`.
- A2 (S2 outbox, `s2c` T5): the prompt submission 202 is not an operation (R2) and keeps S2's contract. `report()` (T2) is the outbox's toast route for a send that fails before its `user_message` row.
- A3 (S2 T13): its `web.timers ≤ 75` check stands; T12 lowers the baseline further.
- A4 (S5 `2026-10-17-s5-editor-host-plan.md`): "When S3's closed code enum exists, the code is added there": S5 adds `HostDocumentRequiresLocalTransport Code = "host_document_requires_local_transport"` to `api/internal/domain/problem/code.go` with its status, and returns it with `problem.New`; "S3's `report()`" is `report` from `@/lib/errors/report`.
- A5 (migration engine): `core/bootgate` keeps writing its refusal codes itself (core does not import domain); T1 adds `TestCode_BootgateRefusalsAreInTheEnum` in `domain/problem` so the spellings cannot drift, and T2 makes the client treat them as states (R18).
- A6 (S8, not yet planned): the unread `last_error` columns of `projects`/`repositories`, the R13 upcaster, the read-model reconcile-on-open (R20) and the operations table's home all move with S8's database merge.
- A7 (S4, not yet planned): the `/v0/operations` stream joins the one stream; R8's presence join becomes ordering; rows drawn from operation items take their placement from the Tree.

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
5. §11's S3 gate "`_ =` → 0" counts `api/tests/kit` (non-test files), which this machine may compile but never run under the integration tag. T11 changes the kit compile-only.
6. The asynx v1.0.0 verification (2026-10-18) found NF1, NF2 and NF7 (R21). T8 cannot start until a fixed release exists; no other task depends on it.

## Open decisions for the user

- **D1. Floating promises.** Recommendation: `installGlobalReporting()` reports every unhandled rejection now (T2), and the type-aware `@typescript-eslint/no-floating-promises` rule waits for S9's lint pass. Cost if wrong: S3 would also have to switch ESLint to typed linting (`parserOptions.projectService`), which slows every lint run and touches every `void promise` site; doing it in S9 costs nothing extra.
- **D2. A generic Retry on failed operations.** Recommendation: no; the existing per-entity actions stay (Retry provisioning, Detach holder, re-run the git action). Cost if wrong: a Retry button later is additive (the operation already records kind and subject).
- **D3. A dedicated `/v0/operations` stream** rather than adding operation frames to the projects stream, whose reader treats any object with a string `id` as a project. Recommendation: dedicated. Cost if wrong: none lasting; S4 merges both.
- **D4. Retention of 500 settled operations across restarts.** Recommendation: as ruled (R6). Cost if wrong: one constant.
- **D5. A branch create opens its chat once the branch exists** (`whenSettled`), as today, rather than opening at once on a "Creating…" pane. Recommendation: keep today's behaviour. Cost if wrong: opening at once needs the IDE route and panes to render an operation for an id that has no workspace yet, about one task.
- **D6. Workspace `lastError` leaves the event log through the existing upcaster** (R13) rather than a catalog step that rewrites `workspace.db`. Recommendation: upcaster; it writes nothing and has precedent. Cost if wrong: S8 adds a catalog step that compacts the stripped patches.
