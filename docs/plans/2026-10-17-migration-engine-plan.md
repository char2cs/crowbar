# Migration Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Crowbar's permanent migration engine skeleton: one home resolver, a `LAYOUT` version read before any store opens, a home lock, a step contract (plan / apply / verify, pre-open and post-open), a run ledger, a `state/` backup, a halt-loudly boot, and `crowbar migrate --plan`; port today's `upgrade.go` steps into it and delete the marker-file mechanism in the same change.

**Architecture:** `core/home` resolves the home once. `core/migrate` is the engine (no store, no domain knowledge): a validated `Catalog` of `Step`s (offline identity and plan) paired at run time with `Impl`s (apply and verify); a `Session` takes the lock, reads `LAYOUT`, runs the pre-open phase before any database opens and the post-open phase inside `app.New`, between the runner reconcile and the owning-chat reconcile. `adapter/migrations` holds the production catalog, the pre-open legacy step and `Begin`; `app` supplies the post-open legacy impls; `core/bootgate` serves `/v0/health` (`migrating` / `halted`) from the moment the socket is bound.

**Tech Stack:** Go 1.26 (`-tags noEmbed`), cobra, testify, existing SQLite adapter (GORM); Rust (Tauri sidecar) and one TS listener for the desktop tasks.

**Spec:** `docs/plans/2026-10-16-thread-branches-and-home-spike.md` Parts B, C, D (D2 shape and contract); `docs/plans/2026-10-16-crowbar-architecture.md` §9 and §11 (S8); `docs/plans/2026-09-24-upgrade-notes.md` (what the five legacy steps do). Rulings below amend two lines of those documents (Task 16 edits them).

## Global Constraints

Every task's requirements include these.

- Build and test: `cd api && go test -tags noEmbed ./<pkg>/ -count=1`; add `-race` for Tasks 8, 9, 10, 13. Lint: `cd api && make lint`. At the end of every task that adds an export: `cd api && make deadcode` (baseline `api/deadcode-baseline.txt` only shrinks; no export without a production caller).
- Disk is nearly full: run `df -h /System/Volumes/Data` before any build or test; under 8 GB free means stop and report.
- Tests run only against fixture homes under `t.TempDir()` (`testutil.FixtureHome`, Task 1). Nothing reads, writes or connects to `~/.crowbar`, `~/.crowbar/crowbar.sock`, `~/.claude` or `~/.codex`. The only live run (Tasks 14 and 15) uses `make dev-desktop` with an isolated `CROWBAR_HOME` under the worktree.
- No `time.Sleep`; wait on channels, `ctx`, `sync.WaitGroup`. Clocks, free-disk and git probes are injected through `migrate.Options`.
- One test file per source file, named `<source>_test.go` beside it; extend an existing file before adding one; no `*_extra_*`, `*_coverage_*`, `*_gaps_*` files. Files stay under 500 lines.
- Go: funlen 100 lines / 50 statements, gocyclo 15, nestif 2, early returns, errors wrapped `fmt.Errorf("migrate: <op>: %w", err)`, required dependencies are constructor arguments, no `...ForTest` in non-test files (use `export_test.go` or an injected option). Comments say why, in 3 lines or fewer.
- No provider vocabulary (claude/codex names, flags, event names) in Go.
- No feature flags, shims, or parallel old/new paths: the marker-file mechanism and the adapter's lock are both deleted in Task 11, the same task that first runs a session next to an adapter.
- Lint suppressions name the rule and a reason on the same line. Never lower the 92% coverage floor or any baseline.
- Each task ends with a commit on `fix/refactor-chats` (the commit IS a deliverable: `git status` clean, `git log` shows it, report the SHA). Never push, never open a PR. Commit messages end with the attribution line the executing session is configured with.
- Web (Task 15 only): targeted `bunx vitest run <file>` only, never the full suite; type-check with `bun tsc --noEmit` (not `bunx tsc`); `bunx eslint` and `bunx prettier --check` on the touched files.

## Review Focus

The five failure modes the spec implies that are most likely to bite, most likely first. Each has a pinning test in the owning task.

1. **Interrupted mid-step (SIGTERM during a long migration).** A cancelled context is outcome `interrupted`, not a failure: no halt, `LAYOUT` unchanged, the next boot resumes from checkpoints. Task 9.
2. **Torn final ledger line (SIGKILL mid-append).** The partial line is ignored and truncated; every earlier record survives; a corrupt line in the middle halts instead. Task 6.
3. **Unreadable `LAYOUT` (empty, `abc`, `-1`, `1.5`).** Never treated as legacy 0: the daemon halts with `layout_unreadable` and no step runs. Tasks 5 and 9.
4. **Not enough disk for the `state/` backup.** The run halts before any step touches anything, and no partial copy is left behind. Tasks 7 and 9.
5. **A production home that still carries the old `state/upgrades/*` markers.** The steps are adopted from the markers, not re-run (re-running `chat-providers-from-history` can change which provider a chat resumes), and no backup is taken. Task 11.

## Rulings

Decisions the plan makes so executors do not have to.

- **R1. The ledger is a file outside `state/`:** `<home>/migrate/ledger.jsonl`, append-only JSON lines, `fsync` per record, mode 0600. It must record the step that creates `crowbar.db`; a backed-up or restored `state/` must not rewind it (the old markers lived in `state/upgrades/` and did exactly that); `crowbar migrate --plan` and a too-old binary read it without opening a database. This amends the spike's "migration ledger in crowbar.db" and architecture §9's "recorded in the journal". Moving it into `crowbar.db` later is itself a step.
- **R2. Two owners, two facts.** The `LAYOUT` file (decimal integer plus `\n`, atomic replace, absent = 0 = legacy) owns the layout version. A *bump* step (`To == From+1`) is pending exactly while `LAYOUT == From`. The ledger owns completion of *in-layout* steps (`To == From`). Order inside a bump step: Verify, then write `LAYOUT`, then append `step.done` (history, never authority). This removes the crash window between two copies of "is it done".
- **R3. A step is a `Step` (offline) plus an `Impl` (online), paired by number.** `Step` carries number, name, phase, `From`, `To` and a `Plan` func that runs offline (read-only, no store open; counts it cannot know are `-1`). `Impl` carries `Apply` and `Verify`. Only pre-open steps may bump the layout. A post-open step must sit at the catalog's `Known` layout, so the change that adds a bump step is forced to retire or re-version every post-open step in the same commit.
- **R4. One resolver.** `core/home` is the only Go code that reads `CROWBAR_HOME` or builds `~/.crowbar` (an AST test enforces it). The Rust shell asks the Go binary (`crowbar paths`). The only Rust path logic left is the debug-build isolation injection (`<repo>/.crowbar` exported as `CROWBAR_HOME`), which is a guard, not a resolver. Moving dev homes outside the repo (spike Part C) is out of scope.
- **R5. Test-binary guard.** `home.Resolve` and `home.At` return `ErrOutsideTemp` inside any test binary (`testing.Testing()`) when the root is not under `os.TempDir()`. One line in the one resolver makes "nothing runs against the real home" hold for every test in the repo.
- **R6. The home lock stays at `state/.lock`.** It moves from `adapter` to `migrate`, is taken before `LAYOUT` is read and released after the databases close. Older binaries cannot read `LAYOUT` but they do `flock` this file, so a running new daemon still excludes them.
- **R7. One backup per run,** before the first step, at `<home>/migrate/backups/<run-id>/state/` (a copy, built as `<run-id>.partial` and renamed when complete). Taken iff some pending step's plan sets `RewritesState` and `state/` holds files; free space of at least size + 64 MiB is checked first. Crowbar never deletes backups; restoring is manual and its path is in the ledger and the halt message.
- **R8. Halt, do not exit.** A failure or refusal keeps the process up, holding the lock and the socket, serving only `GET /v0/health` (200, `status:"halted"`, the `Halt` object) and 503 `{code:"daemon_halted"}` for everything else. During a long migration health says `"migrating"` with progress. Exiting would make the desktop supervisor respawn into the same failure and leaves no channel for the reason. A cancelled context is the exception: outcome `interrupted`, process exits, next boot resumes.
- **R9. `app.New` takes the post-open runner as a required argument** (`app.New(ctx, eng, adapters, post)`), never an option: a constructor that could run without it would skip the post-open steps silently. Production (`internal.New`) passes the session; tests with nothing to migrate pass `migratetest.NoPostOpen()`, and a wiring test pins that every post-open step reaches the runner.
- **R10. Ported steps.** The five legacy fix-ups lose their fail-soft stance: a closure that reports "unfinished" now fails the step and halts boot (a half-applied step looking finished was the flaw D1 names). Each step consumes its own `state/upgrades/<name>` marker: marker present means record `adopted`, delete the marker, run nothing. That is the only reader of the old mechanism and it dies with the steps. A stage that deletes the code a ported step calls deletes the step in the same change.
- **R11. Records only for runs with work.** A boot with nothing pending writes nothing (no `migrate/` directory on a clean current home). Run id is the run's start time, `20060102T150405.000000Z`.

## Evidence carried (so no task needs the scratchpad)

- Six home resolvers today: `metadata.GetHomePath` (`core/metadata/metadata.go`, `resolve_home*.go`), `worktreepath.DefaultCrowbarHome` (`core/paths/worktreepath/worktreepath.go`), `icons.DefaultCrowbarHome` (`api/v0/endpoints/icons/icons.go`, used as the default by projects/repos handlers), the socket default (`core/gateway/transports/socket.go`), `adapter.resolveHome` (`adapter/container.go`), Rust `crowbar_home()` (`desktop/src-tauri/src/sidecar/mod.rs`). Also `project_import.go` defaults `CrowbarHome` to the worktreepath copy, and `config.Get` reads `metadata.GetConfigPath`.
- Socket rule (keep): `CROWBAR_HOME` set and different from the default means `$TMPDIR/crowbar-<fnv1a64(home) as %x>.sock` (macOS caps `sun_path` at 104 bytes); otherwise `<home>/crowbar.sock`. Golden: `/dev/crowbar-home` gives `crowbar-c13f09536446a88e.sock`. A `CROWBAR_HOME` that merely spells out the default is not an override.
- Today's boot: `internal.New` binds the gateway, builds `engine`, then `adapter.New` (takes `state/.lock`, opens 19 SQLite files), then `app.New`, which calls `runUpgrades` between `reconcileAgentRunners` and `startOwningChatReconcile`. `runServe` (`cmd/crowbar/main.go`) calls `internal.New` then `Run`.
- Legacy steps (`api/internal/app/upgrade.go`), each marked by an empty file `state/upgrades/<name>`: `project-homes`, `workspace-paths-table`, `chat-types`, `chat-providers-from-history`, `hook-deliveries`. Four of them need the opened usecases (`project-homes`, `chat-types`, `chat-providers-from-history`, `hook-deliveries`); `workspace-paths-table` only drops a table from `state/view.db`. The old closures return `bool` (finished or not).
- Desktop supervisor (`desktop/src-tauri/src/sidecar/`): `wait_for_health` gives a new daemon 30 attempts of 200 ms; the watchdog probes `/v0/projects` every 10 s and SIGQUITs after 3 failures; at most 3 respawns per 10 minutes. A halted or migrating daemon must not look wedged.
- Existing black-box harnesses wire the stack by hand (`api/tests/kit/env.go` `NewEnvWithHome`, `api/tests/harness_test.go` `newHarnessAt`); `api/cmd/crowbar-seed-chat/main.go` calls `adapter.New()` and relied on the adapter's lock to refuse a running daemon.

## Boot sequence (target)

1. `home.Resolve()` (or `home.At(dir)` for `WithHomeDir`).
2. Bind the socket; start serving through `bootgate` (health: `migrating`).
3. `migrations.Begin` (lock, `LAYOUT`, ledger, closing a dangling run), then `session.RunPreOpen` (plans, preflight, backup, pre-open steps). Refusal or failure: the gate reports `halted`, the container holds the lock and serves the gate only.
4. Engines, adapter (no lock), `app.New(ctx, engines, adapter, session)`, which calls `RunPostOpen` after the runner reconcile and before the owning-chat reconcile; then the API, the `crowbar` binary installed for hooks, and the hook server listening and published.
5. `gate.Open(api handler)`: the server already serving since step 2 starts passing requests to the API.
6. Close: servers, app and databases first, then the session (lock) last.

## File map

| Create | Responsibility |
|---|---|
| `api/internal/core/home/{home,socket,default_unix,default_windows}.go` (+ `home_test.go`, `socket_test.go`, `architecture_test.go`) | the single resolver and path accessors |
| `api/internal/testutil/fixturehome.go` | fixture homes for tests |
| `api/internal/core/migrate/{step,plan,catalog,layout,lock,lock_unix,lock_windows,ledger,checkpoints,backup,disk_unix,disk_windows,halt,session,apply,inspect,render}.go` (+ `_test.go` each, `helpers_test.go`) | the engine |
| `api/internal/core/bootgate/gate.go` (+ test) | pre-serve health and refusal handler |
| `api/internal/adapter/migrations/{catalog,workspace_paths,marker}.go` (+ tests) | production catalog, pre-open legacy step, `Begin` |
| `api/internal/app/legacy_steps.go` (+ test; replaces `upgrade.go`) | post-open legacy impls |
| `api/cmd/crowbar/{paths,migrate}.go` (+ tests) | `crowbar paths`, `crowbar migrate --plan` |
| `desktop/src-tauri/src/sidecar/paths.rs` | Rust side of `crowbar paths` |

Modified: `core/metadata`, `core/paths` (deleted), `core/config`, `core/gateway/transports`, `adapter/container.go` (+ lock files deleted), `worktreepath`, icons/projects/repos handlers, `usecases/project/project_import.go`, `app/container.go`, `internal/internal.go`, `cmd/crowbar/{main,descriptor}.go`, `cmd/crowbar-seed-chat/main.go`, `tests/kit/env.go`, `tests/harness_test.go`, `tests/regression_upgrade_boot_steps_test.go`, `tests/base_era_seed_test.go`, `desktop/src-tauri/src/{lib,ws_bridge,diagnostics}.rs`, `sidecar/{mod,supervisor}.rs`, `web/src/features/window/components/daemon-health-listener.tsx` (+ its test), `api/ARCHITECTURE.md`, three plan docs (`2026-09-24-upgrade-notes`, the spike, the architecture doc).

---

### Task 1: `core/home`, the one resolver, and fixture homes

**Goal:** One Go package decides where the Crowbar home and its socket are, and no test can resolve to a home outside `os.TempDir()`.

**Files:**
- Create: `api/internal/core/home/home.go`, `default_unix.go` (`//go:build !windows`), `default_windows.go` (`//go:build windows`), `socket.go`
- Create: `api/internal/testutil/fixturehome.go`
- Test: `api/internal/core/home/home_test.go`, `api/internal/core/home/socket_test.go` (both `package home`; do not import `testutil` here, it imports `home`)

**Interfaces:**
- Produces:
```go
package home // internal/core/home
const EnvVar = "CROWBAR_HOME"
var ErrRelative = errors.New("home: CROWBAR_HOME must be an absolute path")
var ErrOutsideTemp = errors.New("home: a test binary may only use a home under os.TempDir()")
type Home struct{ /* root string; overridden bool */ }
func Resolve() (Home, error)        // EnvVar if set (absolute), else OS default
func At(root string) (Home, error)  // explicit root; same validation
func (Home) Root() string           // cleaned
func (Home) State() string          // <root>/state
func (Home) Events() string         // <root>/state/events
func (Home) Store() string          // <root>/state/store
func (Home) Config() string         // <root>/config.yaml
func (Home) LayoutFile() string     // <root>/LAYOUT
func (Home) LockFile() string       // <root>/state/.lock
func (Home) MigrateDir() string     // <root>/migrate
func (Home) Socket() string         // see Evidence: socket rule
func socketName(root string) string // unexported: "crowbar-<fnv1a64 %x>.sock"

package testutil
func FixtureHome(tb testing.TB) home.Home                    // t.TempDir()/home, created, via home.At
func WriteFile(tb testing.TB, h home.Home, rel, content string) // parents 0o700, file 0o600
func TreeSnapshot(tb testing.TB, h home.Home) []string        // sorted "relpath size" for every file under Root()
```
Add no accessor without a caller in this plan.

- [ ] **Step 1: Write the failing tests**
  - `TestResolve_RootsEverythingAtCrowbarHome`: `root := t.TempDir(); t.Setenv(EnvVar, root)`; assert `Root()==root`, `State()==root/state`, `Events()==root/state/events`, `Store()==root/state/store`, `Config()==root/config.yaml`, `LayoutFile()==root/LAYOUT`, `LockFile()==root/state/.lock`, `MigrateDir()==root/migrate`.
  - `TestResolve_RejectsARelativeCrowbarHome`: `t.Setenv(EnvVar, "rel/dir")`; `errors.Is(err, ErrRelative)`.
  - `TestResolve_DefaultsToDotCrowbarUnderTheUserHome` (skip on windows): `t.Setenv(EnvVar, "")`, `t.Setenv("HOME", t.TempDir())`; `Root()` is `$HOME/.crowbar`.
  - `TestAt_RefusesAHomeOutsideTempInATestBinary`: `real, err := os.UserHomeDir()` (skip on error or if it is under `os.TempDir()`); `At(filepath.Join(real, ".crowbar"))` fails with `ErrOutsideTemp`. This pins R5.
  - `TestAt_AcceptsAHomeUnderTemp`.
  - `TestSocket_DefaultHomeBindsInsideTheHome`: `HOME=tmp`, `EnvVar=""`; `Socket()==tmp/.crowbar/crowbar.sock`.
  - `TestSocket_SpelledOutDefaultIsNotAnOverride`: `HOME=tmp`, `EnvVar=tmp/.crowbar`; same socket as above.
  - `TestSocket_OverrideLivesInTempKeyedByTheHome`: `EnvVar=t.TempDir()`; `Socket()==filepath.Join(os.TempDir(), socketName(root))`.
  - `TestSocketName_IsStable`: `socketName("/dev/crowbar-home")=="crowbar-c13f09536446a88e.sock"`.
- [ ] **Step 2: Run to verify failure**
  Run: `cd api && go test -tags noEmbed ./internal/core/home/ -count=1`
  Expected: build failure, package has no non-test files.
- [ ] **Step 3: Implement `home.go`, `default_*.go`, `socket.go`, `testutil/fixturehome.go`.**
  `Resolve`: read `EnvVar`; non-empty must be absolute (else `ErrRelative`); empty uses `defaultRoot()` (unix `UserHomeDir()/.crowbar`; windows `UserHomeDir()\Documents\.crowbar`, the old template's `{{USER}}` was never expanded). `overridden = Clean(root) != Clean(defaultRoot())` (a failing `defaultRoot` counts as overridden). One unexported `newHome(root, overridden)` is the only constructor and applies the R5 guard with `testing.Testing()` (comment: why, 3 lines max): reject when `filepath.Rel(os.TempDir(), root)` starts with `..`. `Socket()` per the Evidence rule; hash `Root()` with `hash/fnv` `New64a`.
- [ ] **Step 4: Run to verify pass** (same command). Expected: `ok`.
- [ ] **Step 5: Commit**
  `feat(api): add core/home as the single resolver of the Crowbar home`

**Acceptance:** `go test -tags noEmbed ./internal/core/home/ -count=1` passes; `go vet -tags noEmbed ./internal/core/home/ ./internal/testutil/` is clean; no accessor exists without a test.

---

### Task 2: Replace the Go resolvers; guard against a sixth

**Goal:** Every Go caller that used one of the five Go resolvers, the socket derivation included, goes through `core/home`, and an AST test fails if a new one appears.

**Files:**
- Test (write first): `api/internal/core/home/architecture_test.go`
- Modify: `api/internal/core/metadata/{metadata.go,metadata.yaml,metadata_test.go}`; delete `resolve_home.go`, `resolve_home_windows.go`, `osvalue.go`
- Delete: `api/internal/core/paths/{paths.go,paths_test.go}` (superseded by `Home.Events()/Store()`; keep `core/paths/worktreepath`)
- Modify: `api/internal/core/gateway/transports/{socket.go,socketpath.go,socket_test.go,socketpath_test.go}`, `api/internal/adapter/container.go` (+ `container_internal_test.go`, `container_test.go`), `api/internal/core/paths/worktreepath/{worktreepath.go,worktreepath_test.go}` (delete `DefaultCrowbarHome`), `api/internal/api/v0/endpoints/icons/{icons.go,serve_test.go}` (delete `DefaultCrowbarHome`), `.../projects/handlers/projects.go`, `.../repos/handlers/repos.go` (+ their tests), `api/internal/app/usecases/project/project_import.go`, `api/internal/core/config/{config.go,config_test.go}`, `api/cmd/crowbar/{main.go,descriptor.go}`, `api/internal/internal.go`, and every other user of `metadata.HomeEnvVar` (`cmd/crowbar-seed/main.go`, `tests/kit/suite.go`, tests) which switches to `home.EnvVar`.

**Interfaces:**
- Consumes: `home.Resolve`, `home.At`, `Home` accessors (Task 1).
- Produces: `adapter.WithHomeDir(dir string)` unchanged in signature (calls `home.At`); the icons/projects/repos handler constructors and `project.ImportDeps` take the crowbar home as a **required** constructor argument (`func() (string, error)` returning the container's resolved home); `WithIconStorage` and the nil defaults are deleted. `transports.SocketPath(host)` keeps its signature; with no explicit path it is `home.Resolve()` then `Home.Socket()`. `metadata` keeps only name, version and codename.

- [ ] **Step 1: Write the failing guard** `TestOnlyHomeResolvesTheCrowbarHome`: parse every non-test `.go` file under `api/` with `go/parser`; collect files containing the string literal `"CROWBAR_HOME"` or `".crowbar"`, or a selector named `HomeEnvVar`, `DefaultHomePath`, `DefaultCrowbarHome`. Allowed: `internal/core/home/*`, `cmd/crowbar/scope.go` (exports the env to child callbacks), `cmd/crowbar-seed/main.go` (dev tool that refuses to default). Fail listing every other file. Assert more than 100 files were scanned (a vacuous walk must fail). A genuinely non-daemon file under `cmd/` or `tests/kit` that needs the literal may be added to the allowlist with a one-line reason; never allow a file under `internal/` other than `core/home`. Reword the `CROWBAR_HOME` mention inside the error text of `cmd/crowbar-seed-chat/main.go` (it is a string literal).
- [ ] **Step 2: Run to verify failure**
  Run: `cd api && go test -tags noEmbed ./internal/core/home/ -run TestOnlyHomeResolvesTheCrowbarHome -count=1`
  Expected: FAIL naming `metadata.go`, `worktreepath.go`, `icons.go`, `socket.go`, `internal.go`, and the others.
- [ ] **Step 3: Update the existing tests first** so they speak the new API (`home.At(t.TempDir())` instead of `metadata.GetStateDirPathAt`; handler tests pass the home to the constructor; `socketpath_test.go` `TestSocketPath_CrowbarHomeHashed` expects `home.Home.Socket()` and the spelled-out-default and literal-default cases assert against `Home.Socket()`, the derivation itself being tested in Task 1). Add `TestNewSocket_CreatesTheSocketDirectory`: `NewSocket("unix://" + <short temp>/nested/s.sock)` succeeds, because the binder, not the resolver, creates the parent. They must fail to compile or fail until Step 4.
- [ ] **Step 4: Implement the sweep.**
  - `metadata`: delete `Paths`, the `paths:` block of `metadata.yaml`, `defaultMetadata`'s paths, `GetHomePath`, `DefaultHomePath`, `GetStateDirPath*`, `GetEventsPathAt`, `GetStorePathAt`, `GetConfigPath`, `resolvePath`, both `resolve_home*.go`, `osvalue.go` and their tests. `HomeEnvVar` moves to `home.EnvVar`.
  - `adapter.New`: resolve `h` via `home.At(cfg.homeDir)` or `home.Resolve()`; create `h.Events()` and `h.Store()` with `os.MkdirAll(..., 0o750)`; `acquireStateLock(h.State())` stays for now (Task 11 removes it).
  - `config.Get`: `home.Resolve()` then read `h.Config()`; any error keeps the embedded defaults.
  - `internal.New`: resolve `h` once at the top; `installHome = h.Root()` (the `metadata.GetHomePath` fallback goes). `cmd/crowbar/{main,descriptor}.go` use `home.Resolve()`.
  - `transports/socket.go`: delete `overrideSocketPath`, `literalSocketPath`, the `metadata` and `hash/fnv` imports and the resolver's `MkdirAll` side effects; `NewSocket` does `os.MkdirAll(filepath.Dir(path), 0o700)` before binding.
  - Handlers and `NewImport`: the home is a constructor argument, supplied from `adapters.CrowbarHome()` (plumb it through `app`/`v0` wiring; add `func (c *Container) CrowbarHome() string` to `app.Container` only if no accessor exists).
  - Delete anything this leaves unused (`worktreepath.GlobalStateDir` stays: Task 11 still uses it until `upgrade.go` goes).
- [ ] **Step 5: Run to verify pass**
  Run: `cd api && go build -tags noEmbed ./... && go vet -tags noEmbed ./... && go test -tags noEmbed ./internal/core/home/ ./internal/core/metadata/ ./internal/core/config/ ./internal/core/paths/... ./internal/core/gateway/... ./internal/adapter/... ./internal/api/v0/endpoints/icons/... ./internal/api/v0/endpoints/projects/... ./internal/api/v0/endpoints/repos/... ./internal/app/usecases/project/... ./cmd/... -count=1`
  Expected: `ok` everywhere, including the guard. Fix any test that now trips R5 by giving it a `t.TempDir()` home.
- [ ] **Step 6: Commit**
  `refactor(api): resolve the Crowbar home in one place`

**Acceptance:** the guard passes and scanned more than 100 files; `grep -rn "DefaultCrowbarHome\|DefaultHomePath\|GetHomePath\|GetStateDirPath\|GetEventsPathAt\|GetStorePathAt\|GetConfigPath" api --include='*.go'` prints nothing; the handlers, `NewImport` and `project.ImportDeps` have no nil-default for the home; `transports` imports neither `metadata` nor `hash/fnv`.

---

### Task 3: `crowbar paths`, the Go side of the desktop handshake

**Goal:** The desktop shell has a command that prints the resolved home and socket, so it never has to compute either.

**Files:**
- Create: `api/cmd/crowbar/paths.go`; Test: `api/cmd/crowbar/paths_test.go`
- Modify: `api/cmd/crowbar/main.go` (register `newPathsCmd()`)

**Interfaces:**
- Consumes: `home.Resolve`, `Home.Socket`.
- Produces: `crowbar paths` prints one JSON object `{"home":"<abs>","socket":"<abs>"}` and nothing else, resolved by `home.Resolve()`; exit 1 with the error on stderr if resolution fails.

- [ ] **Step 1: Write the failing tests**
  - `TestPathsCmd_PrintsHomeAndSocket`: `h := testutil.FixtureHome(t); t.Setenv(home.EnvVar, h.Root())`; run `newRootCmd()` with args `paths`, capture stdout; decode; assert `home==h.Root()` and `socket==h.Socket()`; stdout holds exactly one JSON value.
  - `TestPathsCmd_FailsOnARelativeHome`: `t.Setenv(home.EnvVar, "rel")`; the command returns an error mentioning `absolute`.
- [ ] **Step 2: Run to verify failure**
  Run: `cd api && go test -tags noEmbed ./cmd/crowbar/ -run 'PathsCmd' -count=1`
  Expected: FAIL (unknown command `paths`).
- [ ] **Step 3: Implement** `newPathsCmd` (~25 lines): cobra command, `json.NewEncoder(cmd.OutOrStdout()).Encode(...)`.
- [ ] **Step 4: Run to verify pass** (same command). Expected: `ok`.
- [ ] **Step 5: Commit**
  `feat(api): crowbar paths prints the resolved home and socket for the desktop shell`

**Acceptance:** the command's only output is the JSON object; it reads nothing but `home.Resolve()`.

---

### Task 4: `migrate` types and the validated catalog

**Goal:** The step contract exists as types, and a catalog of steps cannot be constructed unless its numbering, phases and layout chain are coherent.

**Files:**
- Create: `api/internal/core/migrate/{step.go,plan.go,catalog.go}`
- Test: `api/internal/core/migrate/{step_test.go,plan_test.go,catalog_test.go}`

**Interfaces:**
- Produces:
```go
package migrate
type Phase uint8
const ( PreOpen Phase = iota + 1; PostOpen )
func (Phase) String() string // "pre-open", "post-open"

type ID struct{ Number int `json:"number"`; Name string `json:"name"` }
type Env struct{ Home home.Home; Log *slog.Logger }

type Counts map[string]int64
func (c Counts) Add(o Counts) // c must be non-nil

type ActionKind string
const ( Move ActionKind = "move"; Convert ActionKind = "convert"; Delete ActionKind = "delete" )
const Unknown int64 = -1 // a count the offline plan cannot know
type Action struct{ Kind ActionKind; Subject, Dest string; Count int64; Note string }
type Plan struct{ Actions []Action; RewritesState bool; NeedsGit bool }
func (p Plan) Empty() bool // no actions

type Step struct {
	Number int; Name string; Phase Phase; From, To int
	Plan func(ctx context.Context, env Env) (Plan, error) // offline, read-only
}
func (s Step) ID() ID

type Catalog struct{ /* steps sorted by Number; known int */ }
func NewCatalog(steps ...Step) (*Catalog, error)
func (c *Catalog) Known() int          // highest To among bump steps, 0 if none
func (c *Catalog) Steps() []Step       // by Number
func (c *Catalog) order(layout int, applied func(Step) bool) []Step // unexported: execution order from layout (see Step 3)
```
`Impl` and `Impls` (the online half of a step) arrive in Task 6 beside `Checkpoints`, which they reference.

- [ ] **Step 1: Write the failing tests**
  - `TestNewCatalog_AcceptsAChain`: steps `{1,"a",PreOpen,0,0}`, `{2,"b",PreOpen,0,1}`, `{3,"c",PreOpen,1,1}`, `{4,"d",PostOpen,1,1}` (all with a non-nil `Plan`); `Known()==1`.
  - `TestNewCatalog_Rejects` (table; assert error text contains the quoted fragment): duplicate number `"duplicate step number"`; duplicate name `"duplicate step name"`; name `"Bad_Name"` not `^[a-z][a-z0-9-]*$` `"invalid step name"`; number `0` `"step number must be >= 1"`; `To==From+2` and `To<From` `"invalid layout range"`; bump step in `PostOpen` `"only pre-open steps may bump the layout"`; two bumps from layout 0 `"more than one bump from layout"`; bumps `0->1` and `2->3` `"gap in layout chain"`; post-open step at layout 0 when `Known==1` `"post-open step must sit at the known layout"`; in-layout pre-open step at layout 0 numbered after the `0->1` bump `"must precede the bump"`; nil `Plan` `"missing plan"`.
  - `TestCatalog_OrderRunsInLayoutOrderThenPostOpen`: with the accepted chain above and `layout=0`, nothing applied: order is `1,2,3` (pre-open, layout-walking) then `4`; with `layout=1`, `3` applied: only `4`; a bump step whose `From < layout` never appears.
  - `TestPhase_String`, `TestCounts_AddSums`, `TestPlan_Empty`.
- [ ] **Step 2: Run to verify failure**
  Run: `cd api && go test -tags noEmbed ./internal/core/migrate/ -count=1`
  Expected: build failure, undefined `NewCatalog`.
- [ ] **Step 3: Implement.** `order(layout, applied)`: for `L := layout; L <= known; L++` emit pre-open in-layout steps at `L` that are not `applied`, then (if `L < known`) the bump step from `L`; finally emit post-open steps (all at `known`) that are not `applied`. `applied(Step)` is supplied by the caller (Task 8 wires it to the ledger for in-layout steps only, per R2). Sort inputs by `Number` once in `NewCatalog`.
- [ ] **Step 4: Run to verify pass** (same command). Expected: `ok`.
- [ ] **Step 5: Commit**
  `feat(api): migrate step contract and validated catalog`

**Acceptance:** every rejection in R3 has a named test; `NewCatalog()` with no steps is valid and has `Known()==0`; the package imports `home` and nothing above `core`.

---

### Task 5: `LAYOUT` file and the home lock

**Goal:** The layout version can be read (absent means legacy, garbage is an error) and written atomically, and one process at a time can own the home.

**Files:**
- Create: `api/internal/core/migrate/{layout.go,lock.go,lock_unix.go,lock_windows.go}`; copy the platform bodies from `api/internal/adapter/lock_{unix,windows}.go` (the adapter copies stay until Task 11)
- Test: `api/internal/core/migrate/{layout_test.go,lock_test.go}`

**Interfaces:**
- Produces (unexported, used by `Session` and `Inspect`):
```go
var ErrLayoutUnreadable = errors.New("migrate: LAYOUT file is unreadable")
var ErrHomeLocked = errors.New("migrate: another crowbar instance is using this home")
func readLayout(h home.Home) (int, error)  // absent => 0; garbage => wraps ErrLayoutUnreadable
func writeLayout(h home.Home, v int) error // temp file in the same dir, fsync, rename, fsync dir
type homeLock struct{ /* *os.File */ }
func lockHome(h home.Home) (*homeLock, error) // MkdirAll(h.State(), 0o750), flock h.LockFile() non-blocking
func (l *homeLock) Close() error
func probeLock(h home.Home) bool // true iff another holder has the lock; opens WITHOUT O_CREATE, creates nothing, releases at once (Inspect, Task 12)
```

- [ ] **Step 1: Write the failing tests** (use `testutil.FixtureHome`)
  - `TestReadLayout`: table over file contents: absent `0`; `"0\n"` `0`; `"1\n"` `1`; `" 2 \n"` `2`; and errors for `""`, `"abc"`, `"-1"`, `"1.5"`, `"1 2"` (each `errors.Is(err, ErrLayoutUnreadable)`). This pins Review Focus 3 at the reader.
  - `TestWriteLayout_RoundTripsAtomically`: write `3`, read `3`, file bytes are exactly `"3\n"`, no `LAYOUT.tmp` left; a second write replaces it.
  - `TestWriteLayout_FailsWhenTheHomeDoesNotExist`.
  - `TestLockHome_SecondLockerIsRefused`: first lock ok; second `errors.Is(err, ErrHomeLocked)`; after `Close()` a third succeeds; `state/` is created with mode 0750 when missing.
  - `TestProbeLock`: no `state/.lock` file gives false and creates nothing (`TreeSnapshot` unchanged); held by `lockHome` gives true; after `Close` gives false.
- [ ] **Step 2: Run to verify failure**
  Run: `cd api && go test -tags noEmbed ./internal/core/migrate/ -run 'Layout|Lock' -count=1`
  Expected: undefined `readLayout` / `lockHome`.
- [ ] **Step 3: Implement** as specified. `readLayout` accepts only text that, after `strings.TrimSpace`, matches `^[0-9]+$`. The lock is a straight move of the adapter's `acquireStateLock` logic onto `h.LockFile()` with the new sentinel.
- [ ] **Step 4: Run to verify pass** (same command). Expected: `ok`.
- [ ] **Step 5: Commit**
  `feat(api): migrate reads LAYOUT atomically and owns the home lock`

**Acceptance:** the adapter's lock files are still present and untouched (Task 11 deletes them); `lock_windows.go` compiles (`GOOS=windows go vet -tags noEmbed ./internal/core/migrate/`).

---

### Task 6: The run ledger, checkpoints, and the online half of a step

**Goal:** Every run, step and checkpoint is durably recorded in an append-only file that survives a torn write, and a step's `Apply` can resume where it stopped.

**Files:**
- Create: `api/internal/core/migrate/ledger.go` (records, reader, appender, view queries), `api/internal/core/migrate/checkpoints.go` (`Checkpoints`, `Impl`, `Impls`)
- Test: `api/internal/core/migrate/{ledger_test.go,checkpoints_test.go}`

**Interfaces:**
- Produces:
```go
var ErrLedgerCorrupt = errors.New("migrate: ledger is corrupt")
const ledgerName = "ledger.jsonl" // under h.MigrateDir()

type record struct { // JSON field names are the on-disk format; pin them
	V       int       `json:"v"`      // 1
	Kind    string    `json:"kind"`   // run.start|backup|step.start|checkpoint|step.done|step.fail|run.end
	At      time.Time `json:"at"`
	Run     string    `json:"run"`
	Version string    `json:"version,omitempty"`     // run.start: binary version
	From    *int      `json:"layout_from,omitempty"` // run.start
	To      *int      `json:"layout_to,omitempty"`   // run.end
	Pending []int     `json:"pending,omitempty"`     // run.start
	Step    int       `json:"step,omitempty"`
	Name    string    `json:"name,omitempty"`
	Phase   string    `json:"phase,omitempty"`
	Key     string    `json:"key,omitempty"`         // checkpoint
	Counts  Counts    `json:"counts,omitempty"`
	Outcome string    `json:"outcome,omitempty"`     // step.done: applied; run.end: completed|failed|interrupted
	Error   string    `json:"error,omitempty"`
	Path    string    `json:"path,omitempty"`        // backup
	Bytes   int64     `json:"bytes,omitempty"`       // backup
}
type RunSummary struct { ID, Version, Outcome, Error string; Start, End time.Time; LayoutFrom, LayoutTo int; Counts Counts }

type ledgerView struct{ /* []record */ }
func readLedger(path string) (ledgerView, error) // read-only; the only tolerated bad line is a FINAL one with no trailing newline (a torn write); any other bad line => ErrLedgerCorrupt
func (v ledgerView) applied(n int, name string) bool         // a step.done with outcome applied for number AND name
func (v ledgerView) checkpoints(n int) map[string]Counts     // all checkpoint records for step n, every run
func (v ledgerView) danglingRun() (record, bool)             // the last run.start, when no run.end follows it
func (v ledgerView) lastRun() (RunSummary, bool)
type ledger struct{ view ledgerView; /* *os.File */ }
func openLedger(path string) (*ledger, error) // readLedger, truncate a torn tail, open O_APPEND|O_CREATE 0o600 (dir 0o700)
func (l *ledger) append(r record) error       // stamps V=1; one Write of json+"\n"; Sync; updates l.view
func (l *ledger) Close() error

type Checkpoints struct{ /* ledger, run id, step ID, done map[string]Counts */ }
func (c *Checkpoints) Done(key string) bool
func (c *Checkpoints) Mark(key string, n Counts) error // appends a checkpoint record (fsynced) before returning
func (c *Checkpoints) total() Counts                   // sum of every Mark for this step, across attempts

type Impl struct { // the online half of a Step, paired with it by number at run time
	Apply  func(ctx context.Context, env Env, cp *Checkpoints) (Counts, error) // idempotent; return ctx.Err() promptly when cancelled
	Verify func(ctx context.Context, env Env) error                            // postconditions; an error halts boot
}
type Impls map[int]Impl // keyed by Step.Number
```

- [ ] **Step 1: Write the failing tests** (`t.TempDir()` files; a fixed clock for `At`)
  - `TestLedger_AppendThenReadBack`: append `run.start`, `step.start`, `checkpoint`, `step.done`, `run.end`; `readLedger` returns them in order with `V==1`; `applied(2,"b")` true and `applied(2,"other")` false; `lastRun()` has outcome `completed` and summed counts.
  - `TestLedger_TornFinalLineIsIgnoredAndTruncated` (Review Focus 2): write two good lines then `{"v":1,"kind":"ste` with no newline; `readLedger` returns two records; `openLedger` then `append` produces a file whose every line parses and has three records.
  - `TestLedger_CorruptLinesAreAnError`: good, garbage, good gives `errors.Is(err, ErrLedgerCorrupt)`; so does a garbage final line that DOES end in a newline (a torn write never ends in one).
  - `TestLedger_DanglingRun`: `run.start` without `run.end` reports the run id; with `run.end` reports none.
  - `TestLedger_AbsentFileIsEmpty` (read-only path creates nothing).
  - `TestRecord_JSONKeysArePinned`: marshal a fully-populated record; the key set is exactly `v,kind,at,run,version,layout_from,layout_to,pending,step,name,phase,key,counts,outcome,error,path,bytes`.
  - `TestCheckpoints_MarkIsDurableAndDoneSurvivesReopen`: `Mark("a", {"files":2})` then reopen the ledger: a new `Checkpoints` for the same step reports `Done("a")`, `!Done("b")`, `total()=={"files":2}`.
- [ ] **Step 2: Run to verify failure**
  Run: `cd api && go test -tags noEmbed ./internal/core/migrate/ -run 'Ledger|Checkpoints' -count=1`
  Expected: undefined `readLedger`.
- [ ] **Step 3: Implement.** Reader splits on `\n`; a trailing empty element is the normal end of file; only a last non-empty element with no newline after it may fail to parse. `openLedger` records the byte offset of the last good newline and `Truncate`s to it before opening for append.
- [ ] **Step 4: Run to verify pass** (same command). Expected: `ok`.
- [ ] **Step 5: Commit**
  `feat(api): migrate run ledger with torn-tail repair and checkpoints`

**Acceptance:** `go vet` clean; the package now compiles with `Impl` (Task 4's catalog code needs no change); JSON field names match the record struct exactly (`TestRecord_JSONKeysArePinned`).

---

### Task 7: State backup and free-disk probe

**Goal:** A complete, atomic copy of `state/` can be taken before a rewriting step, refused cleanly when the disk cannot hold it.

**Files:**
- Create: `api/internal/core/migrate/{backup.go,disk_unix.go,disk_windows.go}`
- Test: `api/internal/core/migrate/backup_test.go`

**Interfaces:**
- Produces:
```go
var ErrInsufficientDisk = errors.New("migrate: not enough free disk space for the state backup")
type FreeBytesFunc func(path string) (uint64, error)
func freeBytes(path string) (uint64, error) // unix.Statfs / windows GetDiskFreeSpaceEx; the Options default
type backupResult struct{ Path string; Bytes int64 } // Path "" means nothing to back up
func backupState(h home.Home, runID string, free FreeBytesFunc) (backupResult, error)
func cleanPartialBackups(h home.Home) error // removes migrate/backups/*.partial left by a crashed run; an absent directory is not an error and is not created
func treeSize(dir string) (int64, error)    // total bytes of regular files, skipping ".lock" (shared with Inspect, Task 12)
```

- [ ] **Step 1: Write the failing tests** (fixture home; `free` is a closure returning a chosen number)
  - `TestBackupState_CopiesTheTreeByteForByte`: write `state/events/a.db`, `state/events/a.db-wal`, `state/content/aa/bb/blob`, `state/view.db`; result path is `<MigrateDir>/backups/<runID>/state`; every file's bytes and mode equal the source; `state/.lock` is NOT copied.
  - `TestBackupState_EmptyOrMissingStateMakesNoBackup`: no `state/` and an empty `state/`: `Path==""`, nothing created under `migrate/`.
  - `TestBackupState_InsufficientDiskLeavesNothingBehind` (Review Focus 4): `free` returns less than size+64 MiB; `errors.Is(err, ErrInsufficientDisk)`; `TreeSnapshot` of the home is identical before and after.
  - `TestBackupState_RejectsAnUnsupportedEntry`: a symlink inside `state/` returns an error naming it, and no partial directory remains.
  - `TestCleanPartialBackups_RemovesOnlyPartials`: a stale `<id>.partial` is removed; a finished `<id>` is kept; with no `migrate/` directory it returns nil and creates nothing.
  - `TestTreeSize_SumsFilesAndSkipsTheLock`.
- [ ] **Step 2: Run to verify failure**
  Run: `cd api && go test -tags noEmbed ./internal/core/migrate/ -run 'Backup' -count=1`
  Expected: undefined `backupState`.
- [ ] **Step 3: Implement.** Walk `h.State()` computing size (skip `.lock`); require `free(h.Root()) >= size + 64<<20` before creating anything; copy into `backups/<runID>.partial/state` (dirs 0o700, files keep their mode, `Sync` each file); only regular files and directories are legal; on any error `RemoveAll` the partial; on success `Rename` to `backups/<runID>` and fsync the parent.
- [ ] **Step 4: Run to verify pass** (same command). Expected: `ok`.
- [ ] **Step 5: Commit**
  `feat(api): migrate state backup with a free-space guard`

**Acceptance:** a failed backup of any kind leaves `migrate/backups/` free of both `.partial` and final directories for that run id; `GOOS=windows go vet -tags noEmbed ./internal/core/migrate/` compiles `disk_windows.go`.

---

### Task 8: `Session`: begin, order, and the apply loop

**Goal:** A session takes the lock, reads the layout, and runs the pending pre-open and post-open steps in order, recording every run, with resumable checkpoints and a halt on any step failure.

**Files:**
- Create: `api/internal/core/migrate/{halt.go,session.go,apply.go}`
- Test: `api/internal/core/migrate/{session_test.go,apply_test.go,helpers_test.go}` (`helpers_test.go` has no `Test*`: fake steps and impls)

**Interfaces:**
- Consumes: Tasks 4 to 7.
- Produces:
```go
type HaltReason string
const (
	HaltLayoutNewer      HaltReason = "layout_newer"
	HaltLayoutUnreadable HaltReason = "layout_unreadable"
	HaltLedgerCorrupt    HaltReason = "ledger_corrupt"
	HaltPlanFailed       HaltReason = "plan_failed"
	HaltPreflight        HaltReason = "preflight_failed"
	HaltStepFailed       HaltReason = "step_failed"
	HaltVerifyFailed     HaltReason = "verify_failed"
	HaltNoImpl           HaltReason = "no_implementation"
)
type Halt struct {
	Reason HaltReason `json:"reason"`; Message string `json:"message"`
	Step *ID `json:"step,omitempty"`; Phase string `json:"phase,omitempty"`
	Layout int `json:"layout"`; Known int `json:"known"`
	Run string `json:"run,omitempty"`; Ledger string `json:"ledger,omitempty"`; Backup string `json:"backup,omitempty"`
}
func (h *Halt) Error() string // Message

type Options struct {
	Version   string
	Clock     func() time.Time // default time.Now
	FreeBytes FreeBytesFunc    // default freeBytes
	HaveGit   func() bool      // default: exec.LookPath(binpath.Resolve("git")) succeeds
	Log       *slog.Logger     // default slog.Default()
}
type Progress struct{ Phase Phase; Step ID; Index, Total int }
type PostOpenRunner interface{ RunPostOpen(ctx context.Context, impls Impls) error }

type Session struct{ /* lock, layout, catalog, ledger, halt, progress (atomic) */ }
func Begin(ctx context.Context, h home.Home, cat *Catalog, opts Options) (*Session, error)
func (s *Session) RunPreOpen(ctx context.Context, impls Impls) error
func (s *Session) RunPostOpen(ctx context.Context, impls Impls) error // implements PostOpenRunner
func (s *Session) Halt() *Halt
func (s *Session) Progress() Progress
func (s *Session) Close() error // releases the lock; writes nothing
```
`Begin` returns an error only for `ErrHomeLocked` or an unexpected I/O failure (no session). Refusals are a non-nil `Halt()` on a live session (Task 9).

- [ ] **Step 1: Write the failing tests** (fixture home; `helpers_test.go`: `step(n, name, phase, from, to)` with an empty plan, `impl(fn)`, and a fake clock that advances one second per call)
  - `TestBegin_NothingPendingWritesNothing` (R11): (a) an empty catalog on a fresh home: after `Begin`, `RunPreOpen`, `RunPostOpen`, neither `LAYOUT` nor `migrate/` exists; (b) a catalog whose single step already ran in an earlier session: the `TreeSnapshot` taken right after the second `Begin` equals the one taken after both `Run*` calls (the ledger is not appended to).
  - `TestRun_InLayoutStepRunsOnceAcrossBoots`: one pre-open step `0->0`; boot twice (new `Session` each); `Apply` called once; the ledger has one `step.done` with outcome `applied`.
  - `TestRun_BumpStepWritesLayoutAndIsNotRerun`: steps `1:0->0`, `2:0->1`; inside step 2's `Verify`, `readLayout==0` (the bump happens only after Verify passes); after `RunPreOpen`, `readLayout==1` and the ledger has `step.done` for both; a second boot runs nothing.
  - `TestRun_PhasesAndOrder`: record the sequence of `Apply` calls for a chain `1 pre 0->0`, `2 pre 0->1`, `3 pre 1->1`, `4 post 1->1`, `5 post 1->1`: `RunPreOpen` yields `1,2,3`; `RunPostOpen` yields `4,5`; post-open never runs before `RunPostOpen` is called.
  - `TestRun_ResumesFromCheckpoints`: `Apply` marks `"a"` and `"b"` then returns `errors.New("boom")` on the first boot; second boot with a healthy impl observes `cp.Done("a")` and `cp.Done("b")` true and only works `"c"`; the final `step.done` counts equal the sum of all marks.
  - `TestRun_FailureHaltsWithTheStepAndKeepsLayout`: `Apply` error gives `Halt` reason `step_failed`, `Step` set, message contains the error; ledger ends `step.fail` then `run.end{failed}`; a bump step leaves `LAYOUT` unchanged; the next boot retries.
  - `TestRun_VerifyFailureHalts`: `Verify` error gives `verify_failed` and no `LAYOUT` bump even though `Apply` succeeded.
  - `TestRun_MissingImplHalts`: pending step with no entry in `impls` gives `no_implementation` and runs nothing; an `impls` key with no catalog step returns an error (`unknown implementation`).
  - `TestRun_AfterAHaltEverythingReturnsTheHalt`: `RunPostOpen` after a pre-open halt returns the same `*Halt` and runs no step (`errors.As`).
  - `TestProgress_ReportsThePhaseAndStep` (observe `Progress()` from inside `Apply`).
- [ ] **Step 2: Run to verify failure**
  Run: `cd api && go test -tags noEmbed -race ./internal/core/migrate/ -count=1`
  Expected: undefined `Begin`.
- [ ] **Step 3: Implement.**
  - `Begin`: `lockHome`; `readLayout`; `cleanPartialBackups`; `openLedger`; close a dangling run (Task 9). Compute the execution list with one helper, `func (c *Catalog) pending(layout int, v ledgerView) []Step` (in `session.go`): `c.order(layout, applied)` where `applied(s)` is `v.applied(s.Number, s.Name)` for in-layout steps and always false for bump steps (R2). `Inspect` (Task 12) calls the same helper.
  - `RunPreOpen`: no pending means return nil. Otherwise append `run.start` once (run id from the clock, `pending` numbers, `layout_from`, `Version`), then loop: recompute `order`, take the first step while its phase is `PreOpen`, find its `Impl` (else `HaltNoImpl`), append `step.start`, build `Checkpoints`, call `Apply`, then `Verify`; on success for a bump `writeLayout(To)`; append `step.done` with `cp.total()` plus the returned counts. After the last pre-open step, if nothing is pending post-open append `run.end{completed}`.
  - `RunPostOpen`: same loop for `PostOpen`, then `run.end{completed}` (`layout_to` = current layout, counts = sum of this run's `step.done`).
  - Any failure: append `step.fail` and `run.end{failed}`, set the session's `Halt` (Ledger path filled), return it. `apply.go` holds the loop so `session.go` stays under 500 lines.
- [ ] **Step 4: Run to verify pass** (same command). Expected: `ok`, no race reports.
- [ ] **Step 5: Commit**
  `feat(api): migrate session runs steps with checkpoints and records every run`

**Acceptance:** `session.go` and `apply.go` each under 500 lines; every exported identifier above has a caller in Tasks 11 to 13 (no speculative export); a step failure leaves `LAYOUT` and previously done steps untouched.

---

### Task 9: `Session`: refusals, interruption, preflight, backup, crash recovery

**Goal:** Every way a boot can go wrong ends in a named, recorded outcome (halt, interruption or recovery) before any step can damage the home.

**Files:**
- Modify: `api/internal/core/migrate/{session.go,apply.go,halt.go}`
- Test: extend `session_test.go`, `apply_test.go`, add `halt_test.go`

**Interfaces:**
- Consumes: Task 8's `Session`, Task 7's `backupState`.
- Produces: no new exports; behaviour below.

- [ ] **Step 1: Write the failing tests**
  - `TestBegin_RefusesANewerLayout`: `LAYOUT` contains `5`, catalog `Known()==1`; `Begin` returns a session with `Halt().Reason==HaltLayoutNewer`, message names both numbers; `RunPreOpen` returns it; no `Apply` call; **no `migrate/` directory is created and `LAYOUT` is untouched** (an older binary must not write into a newer home).
  - `TestBegin_UnreadableLayoutHaltsAndNeverRunsAnyStep` (Review Focus 3): table over `""`, `"abc"`, `"-1"`: reason `layout_unreadable`; assert a legacy-layout step's `Apply` is never called (garbage is not legacy).
  - `TestBegin_CorruptLedgerHalts`: garbage in the middle gives `ledger_corrupt`, no step runs.
  - `TestRun_CancelledContextIsInterruptedNotHalted` (Review Focus 1): `Apply` marks `"a"`, then blocks on `<-ctx.Done()` and returns `ctx.Err()`; test cancels via a channel handshake; `RunPreOpen` returns an error with `errors.Is(err, context.Canceled)` that is NOT a `*Halt`; `s.Halt()==nil`; ledger ends `run.end{interrupted}`; `LAYOUT` unchanged; a second `Begin` resumes and sees `Done("a")`.
  - `TestBegin_ClosesADanglingRunAsInterrupted`: pre-seed a ledger with `run.start` and `step.start` but no `run.end` (a SIGKILL); `Begin` appends `run.end{interrupted}` for that run id and the pending list is unchanged.
  - `TestBegin_ABumpWhoseLayoutLandedBeforeItsDoneRecordIsNotRerun` (R2): pre-seed `LAYOUT=1` and a ledger holding `run.start` and `step.start` for bump step `0->1` with no `step.done`; `Begin` plus both `Run*` calls never invoke that step's `Apply`; the dangling run is closed `completed` (every step its `run.start` listed is done).
  - `TestRun_PlanErrorHalts`: a step whose `Plan` errors gives `plan_failed` naming the step; no `Apply`.
  - `TestRun_PreflightGit`: plan `NeedsGit:true` with `HaveGit` returning false gives `preflight_failed` mentioning git; no backup, no `Apply`.
  - `TestRun_BacksUpStateBeforeTheFirstStepWhenAPlanRewritesIt`: plan `RewritesState:true`, `state/view.db` has content; inside `Apply`, `migrate/backups/<run>/state/view.db` already exists with identical bytes; the ledger has a `backup` record; after a failing step `Halt().Backup` is that path.
  - `TestRun_NoBackupWhenNoPlanRewritesStateOrStateIsEmpty`.
  - `TestRun_InsufficientDiskHaltsBeforeAnyStep` (Review Focus 4): `FreeBytes` tiny; `preflight_failed`, `Apply` never called, `TreeSnapshot(state/)` unchanged, no `.partial` directory.
  - `TestHalt_JSONShape`: marshal a `Halt` and assert the exact key set `reason,message,step,phase,layout,known,run,ledger,backup` (omitempty ones present when set).
- [ ] **Step 2: Run to verify failure**
  Run: `cd api && go test -tags noEmbed -race ./internal/core/migrate/ -count=1`
  Expected: the new tests FAIL (no refusal, no interruption handling, no backup).
- [ ] **Step 3: Implement.** In `Begin`: a layout greater than `Known()` sets `HaltLayoutNewer` and returns before the ledger is opened; `readLayout` error sets `HaltLayoutUnreadable`; `openLedger` corruption sets `HaltLedgerCorrupt`; a dangling run gets `run.end{completed}` when every step its `run.start` listed is no longer pending, else `run.end{interrupted}`. In `RunPreOpen`, after `run.start` and before the loop: call every pending step's `Plan` (offline, both phases), halt `plan_failed` on error, check `NeedsGit` against `HaveGit`, then if any plan has `RewritesState` call `backupState` (map `ErrInsufficientDisk` and other copy errors to `HaltPreflight`) and append a `backup` record. In the loop, an `Apply` error satisfying `errors.Is(err, ctx.Err())` with `ctx.Err()!=nil` appends `run.end{interrupted}` and is returned as is (never set `s.halt`).
- [ ] **Step 4: Run to verify pass** (same command). Expected: `ok`, no race reports.
- [ ] **Step 5: Commit**
  `feat(api): migrate refuses newer layouts, backs up state, and distinguishes interruption from failure`

**Acceptance:** a refused or unreadable home is byte-identical afterwards (`TreeSnapshot`); Review Focus 1, 3 and 4 each have a passing named test; the `Halt` JSON keys are pinned.

---

### Task 10: `bootgate`: health and refusal before the daemon serves

**Goal:** From the moment the socket is bound, the daemon answers `/v0/health` truthfully (`migrating` or `halted`) and refuses everything else with a 503 that says why.

**Files:**
- Create: `api/internal/core/bootgate/gate.go`
- Test: `api/internal/core/bootgate/gate_test.go`

**Interfaces:**
- Produces:
```go
package bootgate
type Report struct{ State string; Message string; Detail any } // State: "migrating" | "halted"
type Reporter func() Report
func New(version string, report Reporter) *Gate
func (g *Gate) Open(h http.Handler) // atomic; every later request goes to h
func (g *Gate) ServeHTTP(w http.ResponseWriter, r *http.Request)
```
Before `Open`: `GET /v0/health` answers 200 `{"success":true,"data":{"status":<State>,"version":<version>,"pid":<os.Getpid()>,"detail":<Detail>}}`; anything else answers 503 `{"success":false,"error":<Message>,"code":"daemon_<State>"}`. The envelope shape matches `api/internal/api/libs` (`success`, `error`, `code`, `data`); do not import `api` from `core`.

- [ ] **Step 1: Write the failing tests** (`httptest`)
  - `TestGate_ServesHealthBeforeOpen`: report `migrating`; `GET /v0/health` is 200, `data.status=="migrating"`, `data.pid==os.Getpid()`, `data.version` set, `data.detail` echoes `Detail`.
  - `TestGate_RefusesEverythingElseBeforeOpen`: `GET /v0/projects`, `POST /v0/chats`, a WebSocket upgrade request: all 503 with `code=="daemon_migrating"` and `error==Message`.
  - `TestGate_HaltedUsesItsOwnCode`: state `halted` gives `daemon_halted` and health `status:"halted"`.
  - `TestGate_ReporterIsCalledPerRequest` (progress changes between two health calls).
  - `TestGate_OpenHandsEveryRequestToTheRealHandler`: after `Open`, `/v0/health` and others reach the stub handler.
  - `TestGate_OpenIsSafeUnderConcurrentRequests` (`-race`): N goroutines request while `Open` runs; coordinate with a start channel and `sync.WaitGroup`, no sleeps; every response is either the gate's or the real handler's, never torn.
- [ ] **Step 2: Run to verify failure**
  Run: `cd api && go test -tags noEmbed -race ./internal/core/bootgate/ -count=1`
  Expected: package has no non-test files.
- [ ] **Step 3: Implement** with an `atomic.Pointer[http.Handler]`; JSON via `encoding/json`.
- [ ] **Step 4: Run to verify pass** (same command). Expected: `ok`.
- [ ] **Step 5: Commit**
  `feat(api): bootgate answers health and refuses traffic until the daemon is ready`

**Acceptance:** `core/bootgate` imports nothing from `api`, `app`, `adapter` or `engine`; no test sleeps; `-race` clean.

---

### Task 11: Production catalog, pre-open legacy step, post-open legacy impls; delete the marker mechanism and the adapter lock

**Goal:** Today's five upgrade steps run through the engine (pre-open or post-open), existing markers are adopted instead of re-run, and neither the marker files nor the adapter's lock exist any more.

**Files:**
- Create: `api/internal/adapter/migrations/{catalog.go,workspace_paths.go,marker.go,boot.go}` (+ `_test.go` each)
- Modify: `api/internal/app/container.go` (accept `WithPostOpen`; replace the `runUpgrades` call), rename `api/internal/app/upgrade.go` to `legacy_steps.go` and `upgrade_internal_test.go` to `legacy_steps_internal_test.go` (`git mv`), `api/tests/regression_upgrade_boot_steps_test.go`, `api/tests/base_era_seed_test.go`, `api/tests/harness_test.go`, `api/tests/kit/env.go`, `api/internal/adapter/container.go` (+ `container_test.go`, `container_internal_test.go`), `api/cmd/crowbar-seed-chat/main.go` (+ `fixture_test.go` if it constructs the adapter)
- Delete: `runUpgrades`, `upgradeStep`, `upgradeSteps`, `worktreepath.GlobalStateDir` if now unused; `api/internal/adapter/{lock.go,lock_unix.go,lock_windows.go}`, `adapter.ErrStateDirLocked`, the `lock` field and its release in `Container.Close` (its test coverage lives in Task 5; a session and an adapter lock on the same `state/.lock` would otherwise refuse each other in one process)
- Note: from this task until Task 13 the real daemon runs without a lock; use only isolated dev homes and do not start a daemon in between.

**Interfaces:**
- Consumes: `migrate.*` (Tasks 4 to 9), `storesqlite.OpenDB`.
- Produces:
```go
package migrations // internal/adapter/migrations
const ( StepProjectHomes = 1; StepWorkspacePathsTable = 2; StepChatTypes = 3; StepChatProviders = 4; StepHookDeliveries = 5 )
func Catalog() (*migrate.Catalog, error)          // names: project-homes (post), workspace-paths-table (pre), chat-types (post), chat-providers-from-history (post), hook-deliveries (post); all 0->0
func PreOpenImpls(h home.Home) migrate.Impls      // step 2 only
func Boot(ctx context.Context, h home.Home, opts migrate.Options) (*migrate.Session, error)
// Boot = Catalog + migrate.Begin + RunPreOpen. A *migrate.Halt error comes back WITH a non-nil session (the caller keeps the lock and serves the gate); any other error comes back with a nil session and the lock released.
func AdoptMarker(h home.Home, name string) (adopted bool, err error) // present: delete it and an empty state/upgrades, return true
func MarkerPresent(h home.Home, name string) bool

package app
type Option func(*appOpts)
func WithPostOpen(p migrate.PostOpenRunner) Option
func New(ctx context.Context, engines *engine.Container, adapters *adapter.Container, opts ...Option) (*Container, error)
type legacyFuncs struct { // the seams the old closures called; tests fill them with fakes
	createMissingHomes func(ctx context.Context, cp *migrate.Checkpoints) (created int, err error)
	missingHomes       func(ctx context.Context) ([]string, error)
	backfillChatTypes  func(ctx context.Context) bool
	restateProviders   func(ctx context.Context) bool
	removeHookJournals func(ctx context.Context, cp *migrate.Checkpoints) (removed int, err error)
}
func newLegacyFuncs(repos *repositories.Container, ucs *usecases.Container) legacyFuncs // the production wiring
func legacyImpls(h home.Home, f legacyFuncs) migrate.Impls // steps 1,3,4,5
```

- [ ] **Step 1: Write the failing tests**
  - `catalog_test.go` `TestCatalog_IsValid`: `Catalog()` returns no error and `Known()==0`.
  - Plans (`catalog_test.go`, fixture home): `TestPlan_MarkerPresentIsAnAdoptionNotAWork`: with `state/upgrades/chat-types` present, step 3's plan is one `Delete` action on that path with count 1 and `RewritesState:false`; without it, one `Convert` action with `Count: migrate.Unknown` and `RewritesState:true`. `hook-deliveries` never sets `RewritesState`. `workspace-paths-table` without `state/view.db` is an empty plan.
  - `workspace_paths_test.go`: `TestPreOpen_DropsTheRetiredTable`: create `state/view.db` with a `workspace_paths` table via `storesqlite.OpenDB`, close it; run `Boot` and close the session; table gone, a second `Boot` is a no-op; `TestPreOpen_NeverCreatesAViewDB`: no `state/view.db` before means none after; `TestPreOpen_CorruptViewDBHaltsNamingTheStep`: write garbage to `state/view.db`; `Boot` returns a `*migrate.Halt` with `Step.Name=="workspace-paths-table"` and a non-nil session (R8).
  - `marker_test.go`: `AdoptMarker` removes the file and the then-empty `state/upgrades`; leaves a non-empty `upgrades` dir's other files; absent returns false.
  - `legacy_steps_internal_test.go` (moved file): keep the `reconcileProjectHomes` tests, adapted to the new return shape (`reconcileProjectHomes` returns `(created int, err error)`); delete `TestRunUpgrades_*`; the `dropRetiredTable` helper and its test move to `adapter/migrations/workspace_paths{,_test}.go` and the step reuses it. Add `TestLegacyImpls_AdoptedStepRunsNothing` (marker present: `Apply` returns `Counts{"adopted":1}`, no `legacyFuncs` fake is called, marker gone) and `TestLegacyImpls_UnfinishedClosureFailsTheStep` (the old `bool false` becomes an error naming the step; R10).
  - Black-box (`tests/regression_upgrade_boot_steps_test.go`): update the three existing tests to reset by `os.RemoveAll(h.MigrateDir())` instead of the markers dir and to boot through `migrations.Boot` (harnesses, below). Add `TestRegression_Upgrade_AdoptsTheMarkersAnOlderDaemonLeft` (Review Focus 5): seed a base-era home containing an untyped legacy chat and all five `state/upgrades/*` markers, no ledger; boot; assert the chat is **still** untyped (`chat-types` was not re-run), all five markers are gone, the ledger shows steps 1 to 5 applied with `adopted` counts, and `migrate/backups` does not exist.
- [ ] **Step 2: Run to verify failure**
  Run: `cd api && go test -tags noEmbed ./internal/adapter/migrations/ ./internal/app/ -count=1`
  Expected: build failures (undefined `Catalog`, `WithPostOpen`).
- [ ] **Step 3: Implement.**
  - Plans (all offline): marker present gives the adoption plan above; otherwise `project-homes` Convert "projects without a home workspace" (`Unknown`, `RewritesState`), `workspace-paths-table` Convert "state/view.db: drop table workspace_paths" (`Unknown`, `RewritesState`) only if `state/view.db` exists, `chat-types` and `chat-providers-from-history` Convert (`Unknown`, `RewritesState`), `hook-deliveries` Delete ".hook-deliveries directories" (`Unknown`, no `RewritesState`).
  - Impls: every `Apply` first calls `AdoptMarker` (adopted gives `Counts{"adopted":1}`, nothing else runs). `workspace-paths-table`: if `state/view.db` exists, `storesqlite.OpenDB`, `Migrator().DropTable("workspace_paths")` if present, close the handle; `Verify` reopens and checks. `project-homes`: `cp.Done(projectID)`/`cp.Mark` per created home, counts `{"homes_created":n}`; `Verify` recomputes the missing-home set and requires it empty. `chat-types`, `chat-providers-from-history`: call `ucs.AgentChat.BackfillChatTypes` / `ucs.AgentRunner.RestateProvidersFromHistory`; `false` returns `fmt.Errorf("legacy step %s: unfinished", name)`; `Verify` checks the marker is absent. `hook-deliveries`: per workspace `cp.Done(ws.ID)`/`Mark` with `{"dirs_removed":n}`; keep `seen` dedup and `worktreepath.IsLiveCheckout`/`RemoveUnderHome` as they are.
  - `app.New`: parse options; where `runUpgrades` ran, call `post.RunPostOpen(ctx, legacyImpls(h, newLegacyFuncs(repos, ucs)))` (with `h` from `home.At(adapters.CrowbarHome())`) when `WithPostOpen` was given, wrapping its error `fmt.Errorf("app: migrate: %w", err)` so `errors.As(err, **migrate.Halt)` still works. Use `ctx` (not `context.WithoutCancel`): a cancel must interrupt, not be ignored.
  - Harnesses (`tests/kit/env.go`, `tests/harness_test.go`): `migrations.Boot(ctx, h, migrate.Options{Version: metadata.GetVersion()})` before `adapter.New`, `app.WithPostOpen(sess)`, and `sess.Close()` after the adapter closes in every teardown path (restart tests reopen the same home).
  - Drop the old nil-guard on `ucs.AgentChat`, `ucs.AgentRunner`, `ucs.AgentWorkspaceReader`: production always has them, and a missing usecase is a wiring bug that must halt (`no_implementation`), not silently skip steps.
  - `Boot`: `Catalog()`, `migrate.Begin`; if the session came back halted return `(sess, sess.Halt())`; else `sess.RunPreOpen(ctx, PreOpenImpls(h))` and on error return `(sess, err)` when it is a `*migrate.Halt`, otherwise `sess.Close()` and return `(nil, err)`.
  - Delete `upgrade.go`'s mechanism and the `state/upgrades` writes. `grep -rn "upgrades" api/ --include='*.go'` must show only `marker.go` and tests.
  - Delete the adapter lock as listed. `cmd/crowbar-seed-chat` resolves `h` with `home.Resolve()`, calls `migrations.Boot`, opens `adapter.New(adapter.WithHomeDir(h.Root()))`, and closes adapter then session; keep its "stop the dev daemon first" message (now backed by `ErrHomeLocked`).
- [ ] **Step 4: Run to verify pass**
  Run: `cd api && go test -tags noEmbed ./internal/adapter/... ./internal/app/ ./cmd/... -count=1 && go test -tags 'integration noEmbed' ./tests/ -run 'Upgrade|Health' -count=1`
  Expected: `ok`.
- [ ] **Step 5: Commit**
  `refactor(api): run the legacy upgrade steps through the migration engine and delete the marker mechanism`

**Acceptance:** `state/upgrades` is written nowhere; Review Focus 5 has a passing black-box test; no file under `api/internal/adapter/` mentions `.lock`; `go vet -tags noEmbed ./...` is clean; `make deadcode` does not grow.

---

### Task 12: `Inspect`, `Render`, and `crowbar migrate --plan`

**Goal:** `crowbar migrate --plan` prints every pending step with its actions and counts, the layout, the lock state and whether a backup will be taken, while changing nothing on disk.

**Files:**
- Create: `api/internal/core/migrate/{inspect.go,render.go}`, `api/cmd/crowbar/migrate.go`
- Test: `api/internal/core/migrate/{inspect_test.go,render_test.go}`, `api/cmd/crowbar/migrate_test.go`
- Modify: `api/cmd/crowbar/main.go` (register `newMigrateCmd()`)

**Interfaces:**
- Produces:
```go
type StepPlan struct{ Step Step; Plan Plan }
type BackupPlan struct{ Required bool; Bytes int64; FreeBytes uint64 }
type Report struct {
	Home string; Layout, Known int; LockHeld bool
	Steps []StepPlan // execution order: pre-open by number, then post-open by number
	Backup BackupPlan; Last *RunSummary
}
func Inspect(ctx context.Context, h home.Home, cat *Catalog, opts Options) (Report, error) // read-only: no lock, no ledger repair, creates nothing; a newer layout returns *Halt
func (r Report) Render(w io.Writer) error
```
`Inspect` shares `Catalog.order` and the `applied` rule with `Session` (do not duplicate the pending computation).

- [ ] **Step 1: Write the failing tests**
  - `TestInspect_IsReadOnly`: legacy fixture home with steps pending and a ledger; `TreeSnapshot` before and after `Inspect` is identical (no `migrate/`, no `LAYOUT`, no backups, no `.lock` side effects); works when `state/` does not exist.
  - `TestInspect_ReportsLockHeld`: a live `Begin` session holds the lock; `Inspect` returns `LockHeld==true` without error.
  - `TestInspect_NewerLayoutReturnsHalt`.
  - `TestInspect_BackupLine`: `RewritesState` plan + `state/` bytes: `Backup.Required` and `Bytes` equal the tree size, `FreeBytes` from the injected probe.
  - `TestRender_Format` (golden substrings, one per line asserted): `layout    0 (legacy)   this binary knows up to 1`; `lock      free`; `last run  none`; a step header `  #2 workspace-paths-table   pre-open   layout 0`; an action line containing `convert  state/view.db` and a count column that prints `?` for `Unknown` and the decimal otherwise; `state backup  required`/`not needed`; `2 pending steps, in execution order`; when nothing is pending the single line `nothing to do`.
  - `migrate_test.go`: `TestMigrateCmd_PlanPrintsAndChangesNothingOnDisk`: `testutil.FixtureHome`, `t.Setenv(home.EnvVar, ...)`, seed `state/upgrades/chat-types` and a `state/view.db` with arbitrary bytes (the plan must not open it); run `newRootCmd()` with `migrate --plan`; stdout mentions `chat-types` adoption and `workspace-paths-table`; `TreeSnapshot` unchanged. `TestMigrateCmd_RequiresPlan`: `migrate` alone errors with `use --plan`. `TestMigrateCmd_NewerLayoutExitsNonZero`: `LAYOUT` containing `9` returns an error naming layout 9.
- [ ] **Step 2: Run to verify failure**
  Run: `cd api && go test -tags noEmbed ./internal/core/migrate/ ./cmd/crowbar/ -run 'Inspect|Render|Migrate' -count=1`
  Expected: undefined `Inspect` / unknown command.
- [ ] **Step 3: Implement.** `Inspect`: `readLayout` (error maps to the same `Halt` reasons), `readLedger` (read-only), `probeLock` (Task 5) for `LockHeld`, `Catalog.pending` (Task 8), each step's `Plan` offline, `treeSize(h.State())` (Task 7) for the backup bytes, `opts.FreeBytes` for free space. `Render` uses `text/tabwriter`. The command: `migrate` has a `--plan` bool flag; without it, return `errors.New("applying happens when the daemon boots; use --plan to preview")`; with it, `home.Resolve()`, `migrations.Catalog()`, `Inspect`, `Render(cmd.OutOrStdout())`. There is deliberately no apply path.
- [ ] **Step 4: Run to verify pass** (same command). Expected: `ok`.
- [ ] **Step 5: Commit**
  `feat(api): crowbar migrate --plan previews the pending steps without touching the home`

**Acceptance:** `crowbar migrate --plan` against a fixture home exits 0 and prints the labelled lines; `crowbar migrate` alone exits 1; the pending computation exists once (Session and Inspect share it).

---

### Task 13: Wire the daemon: boot sequence and halted mode

**Goal:** `crowbar serve` boots through the engine: the socket answers health from the first moment, migrations run before any store opens, and a refusal or failure leaves a live daemon that holds the lock and says why.

**Files:**
- Modify: `api/internal/internal.go` (+ `internal_test.go`)

**Interfaces:**
- Consumes: `bootgate` (Task 10), `migrations.Boot` (Task 11), `migrate.Halt`, `app.WithPostOpen`.
- Produces: `internal.New` keeps its signature. `Container` gains `session *migrate.Session`, `gate *bootgate.Gate`, `serveErr chan error`. The HTTP server starts serving the gate inside `New`, right after the socket binds; `Run` no longer starts it (it waits on `serveErr` / `ctx.Done()`); the hook server starts in `Run` only on a healthy container. `Close` nil-guards every layer (a halted container has no engines, adapter, app or api) and releases the session last.

- [ ] **Step 1: Write the failing tests** (`internal/internal_test.go`; bind `unix://<short path under os.MkdirTemp("", "cb")>/s.sock` and dial it with an `http.Client` whose transport dials that socket)
  - `TestNew_RunsTheLegacyStepsBeforeServing` (also pins R9: production passes `WithPostOpen`): fixture home; after `New`, `<home>/migrate/ledger.jsonl` records steps 1 to 5 `applied` and a `completed` run; `/v0/health` reports `ok`.
  - `TestNew_HaltsOnANewerLayout`: write `LAYOUT` containing `9`; `New` returns a container and no error; `GET /v0/health` is 200 with `data.status=="halted"` and `data.detail.reason=="layout_newer"`; `GET /v0/projects` is 503 `daemon_halted`; `hookserver.Read(home)` fails (no hook server); no `migrate/` directory; `Run` with a cancelled context returns nil.
  - `TestNew_HaltsWhenAStepFails`: write garbage into `state/view.db`; `/v0/health` shows `halted` with `detail.step.name=="workspace-paths-table"`; the home lock is still held (a second `migrate.Begin`/`New` on the home gets `ErrHomeLocked`).
  - `TestNew_RefusesASecondDaemonOnTheSameHome`: second `New` on the same home returns an error satisfying `errors.Is(err, migrate.ErrHomeLocked)`.
  - `TestClose_ReleasesTheHomeLock`: after `Close`, a new `New` on the home succeeds.
  - `TestNew_CancelledDuringMigrationReturnsTheContextError`: a context cancelled before `New` while a pending step exists returns an error with `errors.Is(err, context.Canceled)` (not a halted container); the ledger's last record is `run.end{interrupted}`; the lock is released.
  - Update the existing `internal_test.go` cases to the same helpers; `TestNew_InvalidHomeDir_ReturnsAdapterError` keeps passing (the error now comes from `home.At`/`Begin`).
- [ ] **Step 2: Run to verify failure**
  Run: `cd api && go test -tags noEmbed -race ./internal/ -count=1`
  Expected: new tests FAIL (no gate, no halted mode).
- [ ] **Step 3: Implement `New`** in this order: resolve `h`; `gateway.New(host)`; `bootgate.New(metadata.GetVersion(), reporter)` where `reporter` returns `halted` (detail = the `*Halt`, message = `Halt.Message`) when the session has a halt, else `migrating` (detail = `Session.Progress()`, message `"Crowbar is upgrading its data (step <n> of <total>: <name>)"`); start `http.Server{Handler: gate}.Serve(listener)` in a goroutine feeding `serveErr` (buffered 2); `migrations.Boot`; on `errors.As(err, &halt)` return a halted container (session and gate only); on any other error stop the server, close the listener, return the error; then engines, adapter, `app.New(ctx, engines, adapters, app.WithPostOpen(sess))`; if `app.New` fails with a `*Halt`, close adapter and engines and return the halted container, otherwise unwind everything and return the error; then `crowbarapi.New`, `selfinstall`, hook server, `gate.Open(apiContainer.Handler())`. `runServe` in `cmd/crowbar/main.go` logs `slog.Error("daemon: halted", ...)` with the halt when `New` returns a halted container, still calls `Run`, and returns nil (exit 0) when `New` fails only because the context was cancelled.
- [ ] **Step 4: Run to verify pass**
  Run: `cd api && go build -tags noEmbed ./... && go test -tags noEmbed -race ./internal/ ./cmd/... -count=1 && go test -tags 'integration noEmbed' ./tests/ -run 'Health|Upgrade' -count=1`
  Expected: `ok`. Then `make deadcode` and `make lint`; the deadcode baseline must not grow.
- [ ] **Step 5: Commit**
  `feat(api): boot through the migration engine; halt loudly with a health report`

**Acceptance:** `Run` and `Close` work on both a healthy and a halted container; the production `Container` is the only place that wires `Boot`, the gate and the post-open runner together; a `LAYOUT` newer than known yields a daemon that answers `halted` and writes nothing.

---

### Task 14: Desktop shell asks the daemon for its paths (resolver 6 of 6)

**Goal:** The Tauri shell stops deriving the home and socket itself and asks `crowbar-api paths`, so Go's `core/home` is the only place either is decided.

Separable: dropping this task leaves a Rust resolver pinned to the Go one by the cross-language golden, which is the pre-plan state. It carries the highest verification cost, so do it after the Go tasks.

**Files:**
- Create: `desktop/src-tauri/src/sidecar/paths.rs`
- Modify: `desktop/src-tauri/src/sidecar/mod.rs` (delete `crowbar_home`, `override_socket_path`, `fnv1a64`, `DEFAULT_SOCKET_NAME`, their tests), `desktop/src-tauri/src/lib.rs` (the `sidecar::socket_path()` call at setup), `desktop/src-tauri/src/ws_bridge.rs` (`ws_open`), `desktop/src-tauri/src/diagnostics.rs` if it names a removed fn

**Interfaces:**
- Consumes: `crowbar-api paths` (Task 3).
- Produces (Rust):
```rust
pub struct ResolvedPaths { pub home: PathBuf, pub socket: PathBuf }
pub fn parse_paths(stdout: &[u8]) -> Result<ResolvedPaths, String> // JSON {"home","socket"}; both must be absolute
#[cfg(debug_assertions)] pub fn dev_home_override(repo_root: &Path) -> PathBuf // <repo_root>/.crowbar
pub async fn resolve<R: Runtime>(app: &AppHandle<R>) -> Result<(), Box<dyn Error + Send + Sync>> // runs the sidecar, caches in a OnceLock
pub fn socket_path() -> PathBuf        // reads the OnceLock; resolve() has completed before any window can call ws_open
pub fn daemon_log_path() -> PathBuf    // <home>/logs/daemon.log from the same cache
```

- [ ] **Step 1: Write the failing tests** (`paths.rs` `#[cfg(test)]`): `parse_paths_reads_home_and_socket`; `parse_paths_rejects_relative_or_missing_fields` (relative socket, missing home, non-JSON); `dev_home_override_is_dot_crowbar_in_the_repo_root` (debug builds only); `a_set_crowbar_home_is_not_replaced_by_the_dev_override` (the injection applies only when `CROWBAR_HOME` is unset).
- [ ] **Step 2: Run to verify failure**
  Run: `cd desktop/src-tauri && cargo test -p crowbar-desktop sidecar::paths -- --nocapture`
  Expected: unresolved module `paths`. (Check `df` first: a cold cargo build is large.)
- [ ] **Step 3: Implement.** `resolve`: in debug builds, if `CROWBAR_HOME` is unset set it for the child to `dev_home_override(<CARGO_MANIFEST_DIR>/../..)`; run `app.shell().sidecar("crowbar-api")?.args(["paths"]).output().await`; on non-zero exit return the stderr text; `parse_paths`; store in a `static PATHS: OnceLock<ResolvedPaths>`. `lib.rs`: call `tauri::async_runtime::block_on(sidecar::resolve(&handle))` in `setup` before the window can load; a failure is fatal with the reason logged. `spawn` receives the cached socket and, in debug builds, still exports the same `CROWBAR_HOME` to `serve`. Remove the cross-language golden test from Rust (the Go golden in Task 1 remains).
- [ ] **Step 4: Run to verify pass** (same command, then the whole `sidecar::` module). Expected: `ok`.
- [ ] **Step 5: Live verification (required).** From the worktree: `df -h /System/Volumes/Data`, then `make dev-desktop` (isolated `CROWBAR_HOME=<worktree>/.crowbar`). Confirm: the app window loads, the daemon log at `<worktree>/.crowbar/logs/daemon.log` shows `daemon: listening ... home=<worktree>/.crowbar`, and `ls $TMPDIR/crowbar-*.sock` shows one socket for this home. Do not point anything at `~/.crowbar`. Stop the dev app afterwards.
- [ ] **Step 6: Commit**
  `refactor(desktop): resolve the Crowbar home and socket through the daemon binary`

**Acceptance:** `grep -n "fnv1a64\|override_socket_path\|fn crowbar_home" desktop/src-tauri/src -r` prints nothing; a debug build without `CROWBAR_HOME` still lands in `<worktree>/.crowbar` (never `~/.crowbar`); the live check above passed.

---

### Task 15: Desktop treats a migrating or halted daemon as alive, and says why

**Goal:** A daemon that is upgrading its data or has halted is never mistaken for a wedge (no kill, no respawn loop), and a halt reaches the user as a persistent error.

**Files:**
- Modify: `desktop/src-tauri/src/sidecar/{mod.rs,supervisor.rs}`; `web/src/features/window/components/daemon-health-listener.tsx`
- Test: Rust unit tests in the same files; `web/src/__tests__/features/window/components/daemon-health-listener.test.tsx` (extend)

**Interfaces:**
- Consumes: `/v0/health` shapes from Task 10 (`status` of `ok`, `migrating` or `halted`; for `halted`, `detail.message` carries the halt text).
- Produces (Rust):
```rust
pub enum Health { Ready { pid: Option<i32> }, Migrating, Halted { message: String } }
pub fn parse_health(body: &[u8]) -> Health // absent status => Ready (older daemons); unknown status => Ready
// supervisor.rs: Probe gains `Held` (alive, deliberately not serving); probe_indicts_daemon(Held) == false
```
Tauri event `daemon:halted` with the halt message as payload, emitted once per halt.

- [ ] **Step 1: Write the failing tests**
  - Rust `parse_health`: ok body with pid gives `Ready{pid}`; `{"data":{"status":"migrating"}}` gives `Migrating`; `{"data":{"status":"halted","detail":{"message":"x"}}}` gives `Halted{message:"x"}`; body without `status` gives `Ready`; garbage gives `Ready{pid:None}` (a transport problem is the probe's business, not the parser's). Keep the existing `parse_health_pid` cases by adapting them to `parse_health`.
  - Rust `supervisor.rs`: `a_held_daemon_is_never_indicted` (`probe_indicts_daemon(Probe::Held)==false`).
  - Web (extend the existing file, still at most 3 `vi.mock`): the subscription count assertion goes from 3 to 4; `handlers.get('daemon:halted')!({ payload: 'layout 9 is newer than this Crowbar knows' })` calls `toast.show` with `type:'error'`, a title matching `/could not start/i`, the payload as description, and `duration: 0`.
- [ ] **Step 2: Run to verify failure**
  Run: `cd desktop/src-tauri && cargo test -p crowbar-desktop sidecar:: ` and `cd web && bunx vitest run src/__tests__/features/window/components/daemon-health-listener.test.tsx`
  Expected: unresolved `parse_health` / handler `daemon:halted` undefined.
- [ ] **Step 3: Implement.** `check_health` returns `Health`; `wait_for_health` keeps polling while `Migrating` (without consuming the 30-attempt budget; cap the wait at 30 minutes) and returns an error carrying the message on `Halted`, after which `spawn` emits `daemon:halted` and does not kill the child. `probe_ready` first asks `/v0/health`; `Migrating` or `Halted` returns `Probe::Held`; only `Ready` goes on to the `/v0/projects` deep probe. The watchdog resets its streak on `Held` and never kills. `handle_termination` is unchanged (a halted daemon does not exit). Web: add the fourth subscription using `toast.show({ type: 'error', message: 'Crowbar could not start', description: String(payload), duration: 0 })`.
- [ ] **Step 4: Run to verify pass** (same commands). Also `cd web && bun tsc --noEmit && bunx eslint src/features/window/components/daemon-health-listener.tsx src/__tests__/features/window/components/daemon-health-listener.test.tsx && bunx prettier --check` on both files. Expected: clean.
- [ ] **Step 5: Live verification (required, isolated home only).** In the worktree's dev home (`<worktree>/.crowbar`, never `~/.crowbar`) write `LAYOUT` containing `9`; run `make dev-desktop`. Confirm: the app stays open, the persistent error toast shows the layout message, `curl --unix-socket $TMPDIR/crowbar-<hash>.sock http://x/v0/health` reports `halted`, `daemon.log` shows no respawn after 40 seconds, and no `daemon:unhealthy` toast appears. Remove `LAYOUT`, restart, confirm a normal boot. Remove the test `LAYOUT` before committing.
- [ ] **Step 6: Commit**
  `feat(desktop): a migrating or halted daemon is held, not killed, and the app says why`

**Acceptance:** `cargo test -p crowbar-desktop sidecar::` and the targeted vitest file pass; the live check above passed; the web test file still has at most 3 `vi.mock`; no `LAYOUT` file is left in the dev home.

---

### Task 16: Documentation, spec amendments, final gates

**Goal:** The architecture doc, the upgrade notes and the two spec documents describe the engine as built, and every gate CI runs on the touched code is green.

**Files:**
- Modify: `api/ARCHITECTURE.md`, `docs/plans/2026-09-24-upgrade-notes.md`, `docs/plans/2026-10-16-thread-branches-and-home-spike.md`, `docs/plans/2026-10-16-crowbar-architecture.md`

- [ ] **Step 1: `api/ARCHITECTURE.md`.** Under `core/`: replace the `metadata (home dir …)` clause with `home` (the single resolver; test-binary guard), add `migrate` (engine: lock, `LAYOUT`, ledger, steps, backup), `bootgate`. Under `adapter/`: remove "takes the single-instance lock on the home dir"; add `adapter/migrations` (production catalog and pre-open legacy step). Add a short "Boot and migration" subsection with the six-line boot sequence from this plan. Add `crowbar migrate --plan` and `crowbar paths` to the binaries table.
- [ ] **Step 2: `2026-09-24-upgrade-notes.md`.** Replace "an empty marker per step under `<home>/state/upgrades/`" with the ledger at `<home>/migrate/ledger.jsonl`, state that a failed step now halts boot (health reports why), and that a pre-audit home's existing markers are adopted once.
- [ ] **Step 3: Spec amendments (two lines, each citing "migration engine plan R1").** Spike Part C: the `crowbar.db` node's "migration ledger" becomes a separate `migrate/ledger.jsonl` beside `LAYOUT`; add `migrate/` to the home tree. Architecture §9: "recorded in the journal" becomes "recorded in `migrate/ledger.jsonl`".
- [ ] **Step 4: Final gates** (cold cache; `df` first):
  `cd api && go build -tags noEmbed ./... && go vet -tags noEmbed ./... && make lint && make deadcode && go test -tags noEmbed -race ./internal/core/home/ ./internal/core/migrate/ ./internal/core/bootgate/ ./internal/core/gateway/... ./internal/adapter/... ./internal/app/ ./internal/ ./cmd/... -count=1`; `go test -tags 'integration noEmbed' ./tests/ -run 'Upgrade|Health|Restart' -count=1`. Per-package coverage of the new code, each at least 90%: `go test -tags noEmbed -cover ./internal/core/home/ ./internal/core/migrate/ ./internal/core/bootgate/ ./internal/adapter/migrations/` (the repo-wide 92% floor stays enforced by CI's `make test-coverage`; do not run the whole suite locally, and never lower the floor). `cd desktop/src-tauri && cargo test -p crowbar-desktop sidecar::`. Confirm `grep -rn "CROWBAR_HOME" api --include='*.go'` lists only the allowlisted files and tests.
- [ ] **Step 5: Commit**
  `docs: record the migration engine, the ledger location ruling, and the new boot sequence`

**Acceptance:** every command in Step 4 exits 0; `git status` is clean; `make deadcode` reports no new entries; the amended spec lines cite R1.

---

## Requirement coverage

| Requirement | Task |
|---|---|
| One place resolving the home (six resolvers) | 1, 2, 3, 14 |
| `LAYOUT` read before any store opens; absent = legacy; refuse a newer layout | 5, 9, 13 |
| Home lock | 5, 13 |
| Step contract: number, name, from/to; plan; apply (idempotent, checkpointed, resumable); verify; pre/post-open | 4, 6, 8 |
| Record of every run; decide where it lives | R1, 6, 8, 9 |
| Copy of `state/` before a rewriting step | R7, 7, 9 |
| Failure halts boot loudly; health says why; no continue-anyway, no flags | R8, 9, 10, 13, 15 |
| `crowbar migrate --plan` | 12 |
| Port `upgrade.go` steps; delete the marker mechanism | R10, 11 |
| Tests only on fixture homes | R5, 1 and every task's tests |

## Out of scope (explicit)

- The legacy to layout-1 conversion itself (store merge, worktree moves, per-chat files, blob refcounts, protocol stamp): later stages add steps; a bump step arriving forces the post-open legacy steps to be retired (R3).
- Restore tooling and backup pruning (R7); moving the ledger into `crowbar.db` (R1); relocating dev homes outside the repo (R4); `bin/crowbar` replacement by build hash; putting the ledger into the diagnostics zip.
