# Asynx-Alignment Backend Refactor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> **Companion spec (authoritative for design detail):** `docs/superpowers/specs/2026-07-07-asynx-alignment-refactor-design.md`. Each task cites the spec section (§) that carries the full design rationale and code-level delta; this plan sequences those deltas into ordered, testable, committable units. Where this plan and the spec disagree, the spec wins — flag it.

**Goal:** Make the Crowbar backend a quiver.core-faithful `asynx` consumer: one asynx instance per aggregate *type* (not per workspace instance), central per-type event/read stores, pure commands with `Send`+reactors+projections, graceful shutdown, reconcile-only crash recovery, and human-readable worktree paths — deleting the per-entity model entirely.

**Architecture:** CQRS + event sourcing via `asynx@v0.6.2`. Two persistence planes per aggregate: `state/events/<type>.db` (append-only log, truth) and `state/store/<type>.db` (durable read-model projection). One eager singleton `asynx.Asynx[T]` per type routes many aggregate ids by FNV shard hash. Writes: HTTP validates → `Send` (async) → 202 ack; results reach clients via a hub projection → WS. Cross-aggregate effects and all git/fs/network live in post-commit `Subscribe` reactors, never in commands.

**Tech Stack:** Go 1.26, `github.com/char2cs/asynx v0.6.2`, GORM + glebarez (pure-Go SQLite, WAL), gin, Tauri/React frontend (unchanged), `testify/suite` integration harness.

## Global Constraints

- **Big-bang, no dead code.** No migration code, no deprecated shims, no dead branches, no commented-out code. The old model is deleted. `go vet ./...`, lint, and `deadcode ./...` must be clean. (Spec §Approach, decision 15.)
- **No data migration.** The `.crowbar` layout changes incompatibly; prod `~/.crowbar` is wiped on ship. Do not write migration/compat code.
- **Aggregate types = {Workspace, ReviewThread}.** Chat is deleted (deferred). Projects/repos/terminals/settings stay non-event-sourced. (Decision 2.)
- **Commands are pure:** `Validate`/`EmitEvent` do NO io (no network/git/fs). All io lives in cancelable, timeout-bounded reactors/sweeps. (Decision 9.)
- **`Send` everywhere; `SendWait` only in crash recovery.** Results delivered via projection→WS hub, never the `Send` return. (Decision 4.)
- **No `writeMu`.** Per-aggregate safety = shard routing + `(aggregate_id, version)` uniqueness + OCC retry ≤5× on `ErrPipelineFailed` (never on `ErrValidation`). (Decision 10.)
- **Error handling via `errors.Is`** against asynx sentinels — never string compare. (Spec §G idiom.)
- **Home isolation:** every state path derives from the adapter's resolved `crowbarHome` via `paths.EventsAt(home)`/`StoreAt(home)`/`StateAt(home)` — never the home-agnostic `paths.Events()`/`Store()`. `WithHomeDir` must fully isolate (no leak into prod `~/.crowbar`). (Spec §4, decision 14.)
- **Frontend: zero functional regressions.** Backend-only. Verify WS-driven flows + Branch Review still render. (Decision: §1.3, §4 Frontend.)
- **Worktree path:** `<HOME>/projects/<project>/<host>/<owner>/<repo>/<branch>/`; repo = full slug; **Crowbar-managed** repo-home leaf = `.home`. (Decision 13, §3.9.) **Exception (Task 3b decision):** the ADOPTED repo-home / adopted-home workspace stays rooted at the user's actual checkout (`repo.Path`/`project.Path`), NOT a `.home` leaf — per the locked Crowbar workspace-model law (the repo home IS the user's real checkout). `.home` applies only to net-new Crowbar-managed home worktrees. **RATIFIED by spec author 2026-07-07 — §3.9 has been updated to match, so this is no longer a deviation (see Task 3b).**
- **TDD + frequent commits.** Each task: failing test → verify fail → implement → verify pass → commit. Run `-race` on concurrency-touching tests.

**Commands to know:**
- Unit + race: `cd api && go test -race ./internal/...`
- Integration: `cd api && go test -tags integration ./tests/...`
- Vet/deadcode: `cd api && go vet ./... && deadcode ./...`
- FE checks: `cd web && bun tsc --noEmit && bunx prettier --check .`
- Lint: `cd api && golangci-lint run`

---

## Task ordering & dependency map

```
A. Foundation:   1 (adapter/paths) → 2 (view.db workspace_paths)
B. Naming:       3 (path-derivation helper, additive) → 3b (rewire provisioner onto Derive; delete For)   [3 needs nothing; 3b needs 3, touches usecases/worktree + project_import]
C. Workspace:    4 (commands) → 5 (store proj, additive) → 6 (hub proj+enrichment, additive) → 7 (singleton wiring; land T5/T6 deletions; Delete→Send rewrite)
                 → 8 (delete purge reactor) → 9 (reconcile-on-open) → 10 (orphan-sweep) → 11 (lazy replay)
D. ReviewThread: 12 (full conversion)                 [pattern from 4-7,11]
E. Chat:         13 (remove aggregate + branchreview consumer)
F. Wiring:       14 (wireCallbacks) → 15 (graceful shutdown)
G. Tests:        16 (harness) → 17 (crash/recovery/rebuild/path matrix)
H. Cleanup:      18 (delete internal/locations + storages tree; vet/deadcode/lint green)
```
Each task ends green (compiles, its tests pass) and is committed. Because this is a big-bang refactor, the tree may not fully compile mid-phase; where a task leaves a temporary compile break that the next task closes, the task says so explicitly and the commit is a `wip:` checkpoint. Prefer ordering that keeps `go build ./...` green at each commit; only Task 13 carries a sanctioned interim break (called out inline). Task 7 now stays green — it rewrites `Delete` to a pure `Send` in the same commit that strips the `locations` field, so no reference to the deleted field survives.

---

### Task 1: Adapter — per-type stores, home-parameterized paths, read pools, close-all

**Spec:** §3.2, §3.3, §4 (adapter bullet), decisions 3, 11, 12, 14.

**Files:**
- Modify: `api/internal/adapter/container.go` (ADD fixed per-type event/read stores ALONGSIDE the still-live per-entity resolution + `Registry` — per-entity/Chat deletion is deferred to Tasks 7/13 so `go build ./...` stays green; see Interfaces)
- Modify: `api/internal/adapter/store/sqlite/sqlite.go` (read pool option)
- Modify: `api/internal/adapter/eventstore/sqlite/event_store.go` (confirm WAL+busy_timeout; keep single-writer)
- Test: `api/internal/adapter/container_test.go`

**Interfaces (additive — this task ADDS handles and removes NOTHING from the per-entity API; deletions land in Tasks 7 & 13 so `go build ./...` stays green throughout — ratification path (a)):**
- Produces (added this task):
  - `ReviewThreadES() asynxModels.Store` — already no-arg (`container.go:154`); re-point it to `state/events/review_thread.db`.
  - `ReviewThreadView() *gorm.DB` (new) → `state/store/review_thread.db` (opened via the read pool).
  - The new no-arg workspace handles for `state/events/workspace.db` and `state/store/workspace.db`. Go has **no method overloading**, so a no-arg `WorkspaceES()`/`WorkspaceView()` **cannot coexist** with the still-live 3-arg `WorkspaceES(p,r,w)` (`container.go:185`)/`WorkspaceView(p,r,w)` (`container.go:216`) — expose the new handles under **temporary distinct names** `WorkspaceEventStore()`/`WorkspaceStoreDB()` (or hold them as unexported fields). Task 7 deletes the 3-arg pair and **promotes** these to `WorkspaceES()`/`WorkspaceView()`.
  - `GlobalView() *gorm.DB` (unchanged) → `state/view.db`; `Close() error` (`errors.Join` over ALL planes — the surviving per-entity `Registry` `CloseAll`s AND the new per-type event/read/view handles — with WAL checkpoint); the read-pool helper `OpenReadPoolDB`; and the existing `WithHomeDir(dir string)` option (`container.go:63`), which this task hardens so the new per-type DBs land under the temp home, never prod `~/.crowbar`.
- KEEPS (do **NOT** remove this task): the per-entity `WorkspaceES(projectID, repoID, wsID)`/`WorkspaceView(projectID, repoID, wsID)` methods (`container.go:185,216`) + `workspaceES`/`workspaceView` `Registry` fields (`container.go:37-38`) + `storages/` dir logic — still called at `workspace.go:320,324`, deleted in **Task 7**; and `ChatES()` (`container.go:149`) + the `chatES` field (`container.go:34`) — still called at `app/container.go:39` and `repositories/container.go:59`, deleted in **Task 13**. Removing any of them now breaks `go build ./...` from this commit through Tasks 7/13.
- Consumes: `paths.EventsAt(home)`, `paths.StoreAt(home)`, `paths.StateAt(home)`.

- [ ] **Step 1: Write failing test — WithHomeDir isolates all state under the temp home**

```go
func TestContainer_WithHomeDir_IsolatesAllState(t *testing.T) {
    home := t.TempDir()
    c, err := adapter.New(adapter.WithHomeDir(home))
    require.NoError(t, err)
    t.Cleanup(func() { _ = c.Close() })

    // Every DB file must live under `home`, never under the real ~/.crowbar.
    for _, p := range []string{
        filepath.Join(home, "state", "events", "workspace.db"),
        filepath.Join(home, "state", "events", "review_thread.db"),
        filepath.Join(home, "state", "store", "workspace.db"),
        filepath.Join(home, "state", "store", "review_thread.db"),
        filepath.Join(home, "state", "view.db"),
    } {
        _, statErr := os.Stat(p)
        require.NoError(t, statErr, "expected %s to exist under the isolated home", p)
    }
}
```

- [ ] **Step 2: Run — expect FAIL** (`go test ./internal/adapter/ -run WithHomeDir`) — the new per-type DB files don't exist yet (the test `os.Stat`s them). No compile break: the old per-entity API is retained (additive path (a)), so the package still builds; the test fails only because the new `state/events/*` + `state/store/*` files aren't created until Step 3.

- [ ] **Step 3: Implement.** Rewrite `container.go` per spec §3.3/§4: resolve `crowbarHome` from `cfg.homeDir` (or `CROWBAR_HOME`/default). **`paths.EventsAt`/`StoreAt`/`StateAt` each return `(string, error)`** (they `MkdirAll` via `ensure` — `internal/core/paths/paths.go:59,71,34`), so you MUST resolve the dir first — a 2-value return can't be inlined into `filepath.Join`:

```go
eventsDir, err := paths.EventsAt(home) // likewise storeDir := paths.StoreAt(home), stateDir := paths.StateAt(home)
if err != nil { return nil, err }
wsES, err := eventsqlite.NewEventStore(filepath.Join(eventsDir, "workspace.db"))       // review_thread.db likewise
wsView, err := storesqlite.OpenDB(filepath.Join(storeDir, "workspace.db"))             // review_thread.db likewise
globalView, err := storesqlite.OpenDB(filepath.Join(stateDir, "view.db"))
```

**Do NOT delete** `chatES`, `workspaceES *Registry`, `workspaceView *Registry`, the per-entity `WorkspaceES(p,r,w)`/`WorkspaceView(p,r,w)` methods, or the `storages/` dir logic this task — they stay live (their callers are unconverted until Tasks 7/13; deleting now breaks `go build`). Instead ADD the new per-type handles alongside them: expose the workspace ones under the temp names `WorkspaceEventStore()`/`WorkspaceStoreDB()` (per Interfaces), re-point `ReviewThreadES()` to `state/events/review_thread.db`, and add `ReviewThreadView()` at `state/store/review_thread.db`. Add read pool: give `store/sqlite` an `OpenReadPoolDB(path)` that sets `SetMaxOpenConns(N)` (N=4) under WAL for read-model/view DBs (this is what `WorkspaceStoreDB`/`ReviewThreadView`/`GlobalView` open); event stores keep `SetMaxOpenConns(1)`. `Close()` = `errors.Join` over ALL closers — the surviving per-entity `workspaceES.CloseAll`/`workspaceView.CloseAll` AND the new per-type event/read/view handles — with WAL checkpoint.

- [ ] **Step 4: Run — expect PASS.** Also add/keep a test asserting `paths.Events()` (home-agnostic) is NOT used by the adapter (grep guard in test or a `homeDir`-set assertion).

- [ ] **Step 5: Commit** `git commit -am "refactor(adapter): add per-type event/read stores + read pools + WithHomeDir isolation (per-entity Registry/Chat kept live until T7/T13)"`

---

### Task 2: view.db `workspace_paths` id↔path store

**Spec:** §3.9 (view.db map bullet), §4 (gorm.go bullet).

**Files:**
- Create: `api/internal/adapter/store/wspaths/wspaths.go` — the `WorkspacePaths` type as a **custom** GORM CRUD store in a **new package named `wspaths`** (NOT `paths`). **The name `wspaths` is load-bearing:** the existing `core/paths` package (`internal/core/paths/paths.go`, `package paths`) owns `EventsAt`/`StoreAt`/`StateAt` and is imported as `paths` in Task 1; a second package literally named `paths` would shadow it and force an import alias wherever both are needed. Site it on the adapter/view.db side per spec §3.9 ("owned by the adapter (which owns view.db)"), alongside `adapter/store/sqlite` (imported as `storesqlite`). It does **NOT** use the generic `storesqlite.NewFromDB[T,string]` shape the existing four use — it needs `Put`/`Get`(→a **package-local `wspaths.ErrNotFound`**)/`Delete`, not the generic `Store[T,string]` surface. **Do NOT return `apperr.ErrNotFound` from inside `wspaths`:** no adapter-layer package imports `app/apperr` today (verified — `grep 'app/apperr' internal/adapter/` is empty), and the established pattern is a package-local sentinel translated at the repo boundary (the retired `locations` package defined its own `locations.ErrNotFound`, translated to `apperr.ErrNotFound` at `workspace.go:307`). So define `wspaths.ErrNotFound` here (mirroring `locations.ErrNotFound`) and let the workspace repo (Task 7) translate it at the boundary — using `apperr` here would invert the adapter→app layering.
- **This task builds the `wspaths` package + AutoMigrates its table; it does NOT wire it into `gorm.go`'s `GORMStores` set.** Construction/ownership belongs to `repositories.New` (Task 7), which builds `wspaths.NewWorkspacePaths(adapters.GlobalView())` locally and threads it into `workspace.New` (self-review 2nd-pass, item "pathsStore threading"). **Spec deviation — flagged per the plan's "spec wins — flag it" rule:** spec §3.9 says the store is "exposed as a **fifth `view.db` store** alongside the existing four (`api/internal/app/gorm.go:15-18`)". We honor the spec's **design decision** — the store is adapter-owned, backed by the adapter's `view.db` via `adapters.GlobalView()` — but deliberately DO NOT add a `GORMStores.WorkspacePaths` field, for two concrete reasons: (i) the four existing `GORMStores` fields are `store.Store[T,string]` built via `NewFromDB`, whereas `wspaths` exposes `Put`/`Get`/`Delete` — a different shape that cannot sit "alongside" them as the same type; and (ii) Task 7 constructs the instance locally in `repositories.New`, so a `GORMStores.WorkspacePaths` field would never be consumed and would fail Task 18's `deadcode ./...` gate. The spec's `gorm.go:15-18` mechanism is thus the one point where implementation reality overrides the spec's literal wording; the adapter-ownership intent is fully preserved.
- Test: alongside.

**Interfaces:**
- Produces: `WorkspacePaths` store with `Put(ctx, wsID, path) error`, `Get(ctx, wsID) (string, error)` (returns the **package-local `wspaths.ErrNotFound`**, mirroring `locations.ErrNotFound` — NOT `apperr.ErrNotFound`; the workspace repo translates it at its boundary in Task 7), `Delete(ctx, wsID) error`. Table `workspace_paths(workspace_id TEXT PRIMARY KEY, worktree_path TEXT NOT NULL)`.
- **Three write points — wired by later tasks, NOT here (spec §3.9 a/b/c). This task only builds the store; if none of them land the map stays empty and rename resilience is silently dead, so they are enumerated as explicit steps in their owning tasks:** (a) initial row on **Create** → `pathsStore.Put` in **Task 7**; (b) **rename** → `Move` in **Task 3**; (c) **delete** → `pathsStore.Delete` in the **Task 8** reactor.

- [ ] **Step 1: Failing test** — Put→Get round-trips; Get(missing)→`wspaths.ErrNotFound`; Delete removes.

```go
func TestWorkspacePaths_CRUD(t *testing.T) {
    db := openMemView(t) // WAL temp view.db
    s := wspaths.NewWorkspacePaths(db)
    require.NoError(t, s.Put(ctx, "ws-1", "/h/projects/p/github.com/o/r/main"))
    got, err := s.Get(ctx, "ws-1"); require.NoError(t, err)
    require.Equal(t, "/h/projects/p/github.com/o/r/main", got)
    require.NoError(t, s.Delete(ctx, "ws-1"))
    _, err = s.Get(ctx, "ws-1"); require.ErrorIs(t, err, wspaths.ErrNotFound)
}
```

- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement** the GORM model + custom `Put`/`Get`/`Delete` CRUD; **define the package-local `var ErrNotFound = errors.New("wspaths: not found")` and return it (wrapped) from `Get` on a missing row** — mirroring `locations.go:16`'s `ErrNotFound`; do NOT import `app/apperr` (adapter layer); the app-boundary translation to `apperr.ErrNotFound` is Task 7's job. AutoMigrate the `workspace_paths` table on the adapter's view.db (`adapters.GlobalView()`). **Do NOT** register it in `gorm.go`'s `GORMStores` set — its shape differs from the `NewFromDB[T,string]` stores (`Projects/Repositories/TerminalProfiles/TerminalSessions`) and its instance is constructed locally by `repositories.New` in Task 7, so a `GORMStores` field would be unused dead code (see Files, spec-deviation flag). Ownership stays with the adapter (it owns view.db), per §3.9.
- [ ] **Step 4: Run — PASS.**
- [ ] **Step 5: Commit** `git commit -am "feat(view): workspace_paths id↔path store for rename resilience"`

---

### Task 3: Human-readable worktree path-derivation helper

**Spec:** §3.9 (all), decision 13.

**CRITICAL — no second `worktreepath` package; extend the live one (resolves the reviewer's name-collision + importability blocker).** There is ALREADY a package literally named `worktreepath` at `api/internal/app/usecases/internal/worktreepath/` — the live path authority whose `For(home, projectID, repoID, wsID)` builds the current UUID worktree path and is called at `usecases/worktree/worktree.go:192,765` and `usecases/project/project_import.go:575`. Creating a **second** `worktreepath` under `repositories/workspace/internal/` (the earlier draft) both (i) collides on package name and (ii) is **un-importable by the provisioner** — a `repositories/workspace/internal/…` package cannot be imported from `usecases/worktree` or `usecases/project`, so `Derive` could never reach the call sites that need it. **Resolution:** put the new derivation into the EXISTING `usecases/internal/worktreepath` package (one package, no collision, importable by the whole `usecases/` provisioner). This task is **ADDITIVE** — it adds `Derive`/`HomeLeaf`/`DetectClash`/`Move` and KEEPS the UUID `For` (still called by three sites) so the tree stays green; **Task 3b** atomically rewires those three call sites onto `Derive` and deletes `For` in the same commit. The workspace REPO (Task 7) does NOT import this helper: its Create persists the already-derived `in.WorktreePath` string (a `workspace.CreateInput` field, `workspace.go:38`) that the usecase computed via `Derive`.

**Files:**
- Modify: `api/internal/app/usecases/internal/worktreepath/worktreepath.go` — ADD `Derive`/`HomeLeaf`/`DetectClash`/`Move`; keep `For` (removed in Task 3b). Leave the non-worktree helpers untouched (`StorageDir`, `ThreadsStorageDir`, `RepoDir`, `RepoStorageDir`, `RepoIconPath`, `ProjectDir`, `ProjectStorageDir`, `GlobalStateDir`, `DefaultCrowbarHome`) — they root repo icons / project dirs / terminal-session storage, NOT the git worktree, and §3.9 redefines only the worktree path (their §4 storages-tree disposition is handled in Task 3b + Task 18).
- Test: `api/internal/app/usecases/internal/worktreepath/worktreepath_test.go` (extend the existing suite; `package worktreepath`).

**Interfaces:**
- Produces:
  - `Derive(home, project, slug, branch string) (string, error)` → `<home>/projects/<project>/<host>/<owner>/<repo>/<branch>/`. `slug` = `host/owner/repo` (or a single `name` for no-remote). Branch `feature/login` → nested `feature/login`.
  - `HomeLeaf(home, project, slug string) string` → `.../<slug>/.home/`.
  - `DetectClash(existingPaths []string, candidate string) error` → returns a name-clash error when a case-insensitive-equal path already exists (macOS/Windows) — reject, do not disambiguate.
  - `Move(oldPath, newPath string, gitMove func(old, new string) error, updateMap func() error) error` → `git worktree move` + view.db map update; on move failure keep old map entry.

- [ ] **Step 1: Failing tests** — table-driven:

```go
func TestDerive(t *testing.T) {
    home := "/h"
    cases := []struct{ project, slug, branch, want string }{
        {"proj", "github.com/char2cs/crowbar", "main", "/h/projects/proj/github.com/char2cs/crowbar/main"},
        {"proj", "github.com/char2cs/crowbar", "feature/login", "/h/projects/proj/github.com/char2cs/crowbar/feature/login"},
        {"proj", "localrepo", "main", "/h/projects/proj/localrepo/main"}, // no-remote single-leaf name
    }
    for _, c := range cases {
        got, err := worktreepath.Derive(home, c.project, c.slug, c.branch)
        require.NoError(t, err); require.Equal(t, c.want, got)
    }
}

func TestHomeLeafIsDotHome(t *testing.T) {
    got := worktreepath.HomeLeaf("/h", "proj", "github.com/o/r")
    require.Equal(t, "/h/projects/proj/github.com/o/r/.home", got)
}

func TestDetectClash_CaseInsensitive(t *testing.T) {
    existing := []string{"/h/projects/p/github.com/o/Repo/main"}
    err := worktreepath.DetectClash(existing, "/h/projects/p/github.com/o/repo/main")
    require.Error(t, err) // rejected on a case-insensitive FS
}
```

- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement (additive — `For` stays live for its 3 callers; Task 3b removes it).** `Derive` splits the slug on `/` and joins; no sanitization (git already guards refnames); nested branch dirs kept. `HomeLeaf` appends `.home` (git rejects leading-dot refnames, so it can never collide with a branch leaf — spec §3.9). `DetectClash` compares `strings.EqualFold` on the full candidate vs existing paths. `Move` calls the injected `gitMove` then `updateMap`.
- [ ] **Step 4: Run — PASS** (`-race` not needed; pure). `go build ./...` stays green (additive; `For` + its callers untouched).
- [ ] **Step 5: Commit** `git commit -am "feat(worktree): human-readable path derivation (host/owner/repo/branch, .home, clash-reject)"`

---

### Task 3b: Rewire the worktree provisioner onto `Derive` (slug resolution, clash-reject, `.home` leaf) + delete `For`

**Spec:** §3.9 (all), §4 (worktree-naming / storages-tree deletion), decision 13. **This is the task the reviewer's blocker demands: without it, `Derive`/`DetectClash`/`HomeLeaf` are never called, `workspace_paths` rows record UUID paths, and the Task 17 "Friendly worktree path" test cannot pass.** The provisioner lives in the **usecase** layer (`usecases/worktree`, `usecases/project`) — the sole callers of `worktreepath.For` — so this task touches those packages (which §4's impact map and Tasks 1-18 otherwise never touch for path derivation). Task 3 made `Derive` importable from there by extending the live `usecases/internal/worktreepath`.

**Files:**
- Create: `api/internal/app/usecases/internal/worktreepath/slug.go` (+ test) — `RemoteSlug(repo domain.Repository) string` returning `host/owner/repo` parsed from `repo.RemoteURL` (e.g. `git@github.com:char2cs/crowbar.git` and `https://github.com/char2cs/crowbar` both → `github.com/char2cs/crowbar`), falling back to `repo.Name` as a single-leaf identity when `RemoteURL == ""` (the no-remote case, §3.9). Reuse/lift the existing remote parsing at `internal/engine/provider/providers/github/slug.go` rather than re-implementing; keep it pure.
- Modify: `api/internal/app/usecases/worktree/worktree.go` — replace `worktreepath.For(home, in.ProjectID, in.RepoID, wsID)` (`:192`) and `worktreepath.For(home, ws.ProjectID, ws.RepoID, ws.ID)` (`:765`) with `worktreepath.Derive(home, project, slug, branch)`. Neither call site has the slug in hand (only `RepoID`/`ProjectID` UUIDs), so **load the repo row (by `RepoID`) to reach `RemoteURL`** and resolve the slug via `RemoteSlug`; the branch is `in.Branch` (`:192`) / `ws.Branch` (`:765`). Call `worktreepath.DetectClash(existingSiblings, candidate)` **before** `u.addWorktree`/`materializeProtectedWorktree` and reject case-only collisions (surface as `apperr.ErrInvalidArgument` → 400, or a dedicated name-clash sentinel); gather `existingSiblings` by `os.ReadDir`-ing the repo's derived parent dir `<home>/projects/<project>/<slug>/` (authoritative for the case-insensitive-FS check).
- Modify: `api/internal/app/usecases/project/project_import.go` — replace `worktreepath.For(crowbarHome, repo.ProjectID, repo.ID, wsID)` (`:575`) with `Derive` (the `repo` row is already in scope, so `RemoteSlug(repo)` needs no extra load) + `DetectClash` before `addProtectedWorktree`. **DECISION — RATIFIED (spec author, 2026-07-07); §3.9 updated to match:** the **adopted repo-home / adopted-home workspace keeps the user's actual checkout** — leave `WorktreePath: repo.Path` (`project_import.go:478`) and `WorktreePath: project.Path` (`project_import.go:681`) exactly as the live code has them; do **NOT** relocate them to a `.home` leaf. This is mandated by the **locked Crowbar workspace-model law** (the repo home IS the user's real checkout, detached to HEAD when on a protected branch — never a Crowbar-managed worktree) and by the live provisioning code (`:478,681`); forcing `.home` here would physically move the user's clone. **`worktreepath.HomeLeaf`/`.home` is therefore restricted to any NET-NEW *Crowbar-managed* home worktree only** — a home worktree Crowbar itself materializes under `<HOME>/projects/...`, not an adopted checkout — where you derive `worktreepath.HomeLeaf(home, project, slug)` per §3.9. **Spec deviation flagged (spec §3.9 is the preferred rule; per the plan's "spec wins — flag it"):** §3.9 line 188 derives the repo-home worktree to the `.../<slug>/.home/` leaf without distinguishing adopted vs managed homes; this plan narrows that to managed homes only, because the adopted case is governed by the higher-authority locked workspace-model law. **RATIFIED (spec author, 2026-07-07): adopted repo-home stays `repo.Path`/`project.Path`; `.home` applies to net-new Crowbar-managed homes only. §3.9 has been updated to match, so this is no longer a spec deviation — execute Task 3b as written.**
- **Modify (delete `For`): `api/internal/app/usecases/internal/worktreepath/worktreepath.go`** — remove `For` (+ its now-unused private `workspaceDir` if `StorageDir`/`ThreadsStorageDir` no longer need it; keep `workspaceDir` if they do) now that all three callers are rewired. This closes the additive gap Task 3 left; the whole rewire+removal is ONE green commit (no sanctioned break needed).
- **Storages-tree consumers (§4 "delete the storages tree / UUID naming") — explicit disposition:**
  - `worktreepath.StorageDir` (`usecases/terminal/metastore.go:104`, doc at `engine/terminal/metastore.go:42`) produces the per-workspace `.../workspaces/<wsID>/storages` UUID tree that §4 marks deleted-outright. **The spec (§3.9) redefines ONLY the worktree path and provides NO replacement template for terminal-session storage under the human-readable layout.** Rather than invent one, this task **retains `StorageDir`** and FLAGS re-rooting terminal-session storage as a **spec follow-up** (tracked in Task 18's guard). Do not silently drop it.
  - `worktreepath.RepoIconPath` (`project_import.go:425` + `repos/handlers/repos.go:396,420,475` mirrors) and `worktreepath.ProjectDir` (`project_delete.go:161`) and `worktreepath.DefaultCrowbarHome` (`project_import.go:244`) are **repo-/project-/home-level metadata, NOT the per-workspace worktree** — §3.9 does not touch them; they are **retained unchanged**.
- Test: `usecases/worktree/…_test.go`, `usecases/project/…_test.go`, and `worktreepath/slug_test.go`.

**Interfaces:**
- Produces: `RemoteSlug(repo domain.Repository) string`. Consumes Task 3's `Derive`/`DetectClash`/`HomeLeaf`. After this task, the `worktreePath` threaded into `workspace.CreateInput.WorktreePath` (and thus persisted by Task 7's `pathsStore.Put(ctx, id, in.WorktreePath)`) is the human-readable `<home>/projects/<project>/<host>/<owner>/<repo>/<branch>/`.

- [ ] **Step 1: Failing tests** — (a) `RemoteSlug` maps ssh/https remotes → `host/owner/repo` and empty remote → `repo.Name`; (b) a create through `usecases/worktree` lands the worktree at `projects/<project>/<slug>/<branch>/` (assert via the returned `WorktreePath`); (c) a second create whose derived path is case-only-equal to an existing sibling is rejected (`DetectClash`); (d) `project_import` provisions the managed *branch* worktree at the derived path; (e) **the adopted repo-home/home workspace keeps the user's checkout** — assert `project_import` persists `WorktreePath == repo.Path` (the `:478` import row) and `WorktreePath == project.Path` (the `:681` home-create row), NOT a `.home` leaf under `<HOME>/projects/...` (guards the Task-3b decision + the locked workspace-model law).
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement** the slug resolver + the three call-site rewires + `DetectClash` guards + `HomeLeaf` for net-new Crowbar-managed home worktrees only (per the DECISION above — adopted repo-home keeps `repo.Path`/`project.Path`), then delete `For`.
- [ ] **Step 4: Run — PASS.** `go build ./... && go vet ./...` green (atomic rewire+removal — no interim break).
- [ ] **Step 5: Commit** `git commit -am "feat(worktree): derive human-readable worktree paths in the provisioner (slug+branch, clash-reject, .home); delete UUID For"`

---

### Task 4: Workspace commands (pure `Command[Workspace]`)

**Spec:** §3.5 (command shape), decision 9. The command set is the **12 existing** command files under `internal/app/repositories/workspace/internal/commands/` (verified live) — this task audits them for purity, it does **not** invent new mutations. `Delete` is the one genuinely-new command (§3.8).

The 12 live commands: `create.go`, `clear_branch.go`, `provision_in_place.go`, `reparent.go`, `resolve_conflicts.go`, `set_last_error.go`, `set_merge_strategy.go`, `set_parent_from_pr.go`, `sync_provider_state.go`, `sync_working_tree_state.go`, `touch_activity.go`, `update_fork_point.go`. There is **no** `set_status.go` (status is a folded field of `sync_*`/`reparent`/etc., not a standalone command), **no** `sync_working_tree.go`/`sync_provider.go` (the real files are `sync_working_tree_state.go`/`sync_provider_state.go`), and **no** `delete.go` yet — Delete today is a synchronous `entity.forget()` in `workspace.go`, not a command (Task 8 converts it to `Send(Delete{})`). The status set the commands fold into is `new/locked/pr-conflicts/pr-open/pr-merged/pr-closed/deleted`.

**Files:**
- Modify: the 12 existing `internal/commands/*.go` — **confirm** each `Validate`/`EmitEvent` is already pure (no move-out work is expected). This is a **verified-pure** package: every non-test file imports only `fmt`, `time`, `asynxModels` (`github.com/char2cs/asynx/models`), `domain`, and `gitdomain` (`.../domain/git`) — NO `os`/`net`/`exec`/`os/exec` and no engine-git/engine-provider packages, so there is no IO to relocate. Named-suspect check: `sync_provider_state.go` does all its mapping through pure helpers (`nextProviderStatus`/`prStatusToWorkspace`), and `sync_working_tree_state.go`/`resolve_conflicts.go`/`update_fork_point.go` likewise only fold caller-supplied derived values into state (decision 9 already holds). The only real work here is verifying that invariant still holds and adding the new pure `delete.go`.
- Create: `api/internal/app/repositories/workspace/internal/commands/delete.go` (new tombstone command; emits `workspace.deleted.<id>`, folds `Status = deleted`).
- Test: `api/internal/app/repositories/workspace/internal/commands/commands_test.go` (extend the existing suite — the file already exists — plus the per-command `*_test.go` already present, e.g. `provision_in_place_test.go`, `reparent_test.go`).

**Interfaces:**
- Produces: each command implements asynx `Command[domain.Workspace]`: `AggregateID() string` (ws UUID), `EventName() string` (`"workspace.<action>.<id>"`), `Validate(*domain.Workspace) error` (state guard → `asynxmodels.ErrValidation`), `EmitEvent(*domain.Workspace) domain.Workspace` (pure next-state), `ShouldSnapshot() bool`.
- **`Delete` emits `workspace.deleted.<id>` and its `EmitEvent` sets `Status = deleted`** (terminal tombstone; NOT a Forget). (Spec §3.8 delete lifecycle.)

- [ ] **Step 1: Failing tests** — for each command: `Validate` rejects illegal transitions with `ErrValidation`; `EmitEvent` produces the exact next-state; `EventName()` is `workspace.<action>.<id>`; commands do no io (assert by construction — no client fields). Example:

```go
func TestDelete_EmitsTombstone(t *testing.T) {
    cmd := commands.Delete{ID: "ws-1"}
    require.Equal(t, "workspace.deleted.ws-1", cmd.EventName())
    next := cmd.EmitEvent(&domain.Workspace{ID: "ws-1", Status: domain.WorkspaceStatusPROpen})
    require.Equal(t, domain.WorkspaceStatusDeleted, next.Status)
}
```

- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement.** Confirm the 12 existing command structs are already pure (verified: the package imports no IO packages — see Files; the state-folding invariant of decision 9 already holds, so expect nothing to move out) and add the new pure `Delete` struct. If — contrary to the verified import audit — a command is found doing git/provider IO, move that derivation OUT to the caller (caller computes the derived values, passes them as command fields); this is a safeguard, not expected work.
- [ ] **Step 4: Run — PASS.**
- [ ] **Step 5: Commit** `git commit -am "feat(workspace): pure Command[Workspace] set (create/sync/delete)"` (no `status` verb — status is a folded field of `sync_*`/`reparent`/etc., there is NO `set_status` command, see Task 4 header)

---

### Task 5: Workspace store projection (`store/workspace.db`)

**Spec:** §3.5 (delivery), §3.7, decision 5.

**Structure decision (resolves the projector-file ambiguity):** the two projections live in a NEW `projections/` **subpackage** under `store/` — `internal/store/projections/store.go` (Task 5) + `internal/store/projections/hub.go` (Task 6) — per spec §4 ("`projections/store.go`" / "`projections/hub.go`", lines 126-128) and the repo-layout note (spec: `.../internal/store/internal/projections/*`). This is the workspace design the spec prescribes; it is the same store+hub SPLIT as Task 12/reviewthread (spec §4 line 197), just landed in a `projections/` subpackage rather than same-package files (spec wins on the subpackage placement for workspace).

**This task is purely ADDITIVE — it does NOT delete the combined `projections.go` nor rework the 5-arg `store.New`, because `workspace.go:340` still calls `store.New(ctx, view, ax, w.broadcast, loc.ID)` and workspace.go is not modified until Task 7.** Removing `store.New`/`registerProjections` here would break `go test -race ./internal/...` at this commit — a non-sanctioned build break (lines 48/571 promise `go build ./...` stays green T1→T6). So this task creates the new save-only `projections/store.go` + `RegisterStore` and unit-tests them against a **test-built asynx**, leaving the combined `projections.go` and the 5-arg `store.New` intact. The **removal** of the combined `projections.go`, the **`store.New` signature rework** (drop the per-`aggregateID` param + eager reconcile), and the **production `RegisterStore` registration on the singleton `axWorkspace`** are all DEFERRED to **Task 7** (where workspace.go's per-entity code + its `store.New(...)` call site are deleted) — mirroring how Task 8 defers its production wiring to Task 14.

**Files:**
- Create: `api/internal/app/repositories/workspace/internal/store/projections/store.go` (the **save-only** projection — the durable read side) + its `RegisterStore` function.
- **Do NOT touch this task (deferred to Task 7):** the combined `api/internal/app/repositories/workspace/internal/store/projections.go` — today a **single combined** projector whose `projector.onEvent` (`projections.go:43-52`) BOTH saves (`saveWithRetry`) AND `broadcast`s under one `Subscribe(asynx.Topic("workspace.*"))` (`projections.go:29`), with `onForget` row-delete (`projections.go:76-82`); and `internal/store/store.go`'s `store.New`/`reconcile` (`store.go:44-85`) — today `New(ctx, db, ax, broadcast, aggregateID)` (`:44`) registers the projection AND runs a per-`aggregateID` `reconcile` (`:69-85`) via `registerProjections` (`store.go:55`). Task 7 removes the combined file (its save half + `onForget` are already re-expressed in this task's `projections/store.go`; its `broadcast` half in Task 6's `projections/hub.go`) and reworks `store.New` to drop the `aggregateID` param + eager reconcile (read-model repair becomes lazy — Tasks 9/11). The read-model rows carry `project_id`,`repo_id`,status,branch,counters,merge-state — the read model doubles as the location index (§3.7).
- Test: alongside — drive `RegisterStore` with a test-built asynx over temp `events` + `store` DBs (independent of the still-live combined `projections.go`).

**Interfaces:**
- Produces: `RegisterStore(store, axWorkspace)` subscribing `"workspace.*"` (designed to register ONCE on the singleton, not per aggregate); handler folds `evt.Aggregate` into `store/workspace.db`; `OnForget` deletes the row (cheap, synchronous — spec §3.6). Store exposes `List(ctx) ([]Row, error)`, `Get(ctx, id) (Row, error)`. **This task only PRODUCES + unit-tests `RegisterStore` (against a test asynx); it is not production-wired here.** The old per-entity `store.New(ctx, db, ax, broadcast, aggregateID)` signature and its eager `reconcile` remain live this task (workspace.go still calls them) and are removed/replaced in **Task 7**, which also performs the production `RegisterStore` registration on the singleton `axWorkspace` (central registration + lazy repair).

- [ ] **Step 1: Failing test** — publish a `workspace.created.*` event via a test asynx over a temp `events` + `store` DB; assert the row appears in `List`; publish `workspace.deleted.*`→ then `Forget`→ row gone.
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement** the save-only projection + `RegisterStore` in the new `projections/store.go` (mirror quiver `arrow/internal/store/internal/projections`), folding `evt.Aggregate`; `OnForget` row-delete only (no io). **Additive only:** leave the combined `internal/store/projections.go` and the 5-arg `store.New`/`reconcile` (`store.go:44-85`) untouched (workspace.go still uses them until Task 7). Their removal + the `store.New` rework + the production `RegisterStore` registration on the singleton land in **Task 7**.
- [ ] **Step 4: Run — PASS** (`-race`) — `go test ./internal/...` stays green because this task added a package and deleted nothing (workspace.go still builds against the live combined `projections.go` + 5-arg `store.New`).
- [ ] **Step 5: Commit** `git commit -am "feat(workspace): store projection → store/workspace.db (List doubles as index)"`

---

### Task 6: Workspace hub projection + enrichment callback

**Spec:** §3.5 (hub-frame enrichment — CRITICAL for FE), decision 5.

**This task is purely ADDITIVE, like Task 5.** `RegisterHub(axWorkspace, ...)` must ultimately subscribe on the singleton `axWorkspace`, which is not constructed until Task 7 — so Task 6 cannot production-wire the hub at its own commit without a non-sanctioned break. This task therefore only PRODUCES `projections/hub.go` + `RegisterHub` and unit-tests them against a **test-built asynx** with a **stub `enrich`**. Extracting the real enrichment (`broadcastWorkspace`) in `repositories/container.go` into the shared injected callback, and the **production `RegisterHub(axWorkspace, enrich, broadcast)` registration on the singleton**, are DEFERRED to **Task 7** (which builds `axWorkspace` and removes the combined `projections.go`'s broadcast half).

**Files:**
- Create: `api/internal/app/repositories/workspace/internal/store/projections/hub.go` — a distinct `Subscribe("workspace.*")` hub projection + its `RegisterHub` function (the **broadcast half** that Task 7 will carve out of the combined `internal/store/projections.go`, no longer welded to the save projection).
- **Do NOT modify `repositories/container.go` this task (deferred to Task 7):** the enrichment-callback extraction and the production registration on the singleton land in Task 7.
- Test: alongside — a unit test that `RegisterHub` (with a stub `enrich` + a test asynx) emits the expected frame; plus an assertion that the hub-projection frame and a `BeginWork`-style rebroadcast built from the SAME `enrich` are byte-identical.

**Interfaces:**
- Produces: `RegisterHub(axWorkspace, enrich func(domain.Workspace) WorkspaceFrame, broadcast func(WorkspaceFrame))` subscribing `"workspace.*"`; builds base frame from `evt.Aggregate`, calls `enrich` (in production: attaches `Working` via `IsWorking(id)` + `CanMergeLocally`/`ParentBranch` via sibling `List` + `ResolveMergeEligibility`), then `broadcast`. **Built + unit-tested here with a stub `enrich`; the real container-owned `enrich` is injected and registered on `axWorkspace` in Task 7.**
- **`BeginWork`/`EndWork` keep a direct rebroadcast** through the SAME `enrich`+`broadcast` (they fire on the 202 ack, not on an event) so the FE spinner is preserved (wired in Task 7). (Spec §3.5.)

- [ ] **Step 1: Failing test** — assert the frame emitted by `RegisterHub`'s hub projection for a given aggregate equals the frame produced by the same `enrich`+`broadcast` invoked directly (the `BeginWork` path), and that `Working`/`CanMergeLocally` are populated by the stub `enrich`. Drive it with a test-built asynx.
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement** the hub projection + `RegisterHub` in `projections/hub.go`. **Additive only:** do NOT touch `repositories/container.go` or the combined `projections.go` — the real-`enrich` extraction + production registration are Task 7.
- [ ] **Step 4: Run — PASS** — `go test ./internal/...` stays green (package added, nothing deleted).
- [ ] **Step 5: Commit** `git commit -am "feat(workspace): hub projection + shared enrichment (Working/merge overlays preserved)"`

---

### Task 7: Singleton `axWorkspace` wiring; delete Registry/LRU/writeMu/entityForLocation

**Spec:** §3.4, §4 (workspace/**, app/container.go, repositories/container.go bullets), decisions 1, 10. **No interim break** — this task also rewrites `Delete` to a pure `Send(commands.Delete{ID:id})` (the `commands.Delete` type already exists from Task 4), so the `locations` field is stripped cleanly with no dangling reference and the workspace package stays green (`go build`/`go test -race` for the package both succeed at this commit). The post-commit purge io moves to the Task 8 reactor; only the reactor registration is deferred (to Task 14 via `wireCallbacks`).

**Files:**
- Modify: `api/internal/app/repositories/workspace/workspace.go` — hold `axWorkspace`; delete `entities *Registry`, `entityFor`/`entityForLocation` (`workspace.go:293-356` — `entityFor` doc from `:293`/body `:297-313`, `entityForLocation` `:315-356`), `wsEntity`, `writeMu`, `send()`'s SendWait, **and the 5-arg `store.New(ctx, view, ax, w.broadcast, loc.ID)` call (`workspace.go:340`)**; **rewrite the whole synchronous `Delete` method (`workspace.go:646-685`) to the pure async command `func (w *workspace) Delete(ctx context.Context, id string) error { _, err := w.sendWithOCC(ctx, commands.Delete{ID: id}); return err }`** (the `commands.Delete` type already exists from Task 4) — this deletes the old body's `w.locations.Get` (`:650`), `entityForLocation` call (`:654`), `entity.forget` (`:665`), `w.entities.Evict` (`:668`), `w.locations.Delete` (`:671`), and the `removeWorkspaceDir` synchronous purge (`:674`) in one stroke; all of that post-commit purge io moves to the Task 8 reactor. **Strip the `locations locations.Store` field (`:224`), its construction `locations.New(adapters.GlobalView())` (`:267,275`), and the `locations` import (`:20`)**, re-homing every remaining `w.locations.*` call site (`:301,363,379,714`) off it (see Interfaces). Because `Delete` is now a pure `Send`, no `w.locations.*` reference survives the strip — the package compiles at this commit. Wire in the `pathsStore` (Task 2) and the store projection (Task 5).
- **Modify: `api/internal/app/repositories/workspace/internal/store/store.go` + delete `internal/store/projections.go` — this is the DEFERRED disposition from Tasks 5/6 (they were purely additive).** Delete the combined `internal/store/projections.go` (its `onEvent` save half is already re-expressed in Task 5's `projections/store.go`, its `broadcast` half in Task 6's `projections/hub.go`; no combined save+broadcast projector survives — decision 15). Rework `store.New`/`reconcile` (`store.go:44-85`): drop the per-`aggregateID` param and the eager per-entity `reconcile` (`:69-85`) + the `registerProjections` call (`store.go:55`); read-model repair is now lazy (Tasks 9/11) and reconcile-on-open is the bounded background task (Task 9). **Register `RegisterStore` (Task 5) and `RegisterHub` (Task 6) exactly ONCE on the singleton `axWorkspace`** (not per aggregate) — this is the production wiring both additive tasks deferred to here.
- Modify: `api/internal/adapter/container.go` — **delete the per-entity `WorkspaceES(p,r,w)` (`:185`)/`WorkspaceView(p,r,w)` (`:216`) methods, the `workspaceES`/`workspaceView` `Registry` fields (`:37-38`) + their `CloseAll` in `Close()` (`:251-259`), and the `storages/` dir logic (`workspaceStorageDir`)**, then **promote** the Task 1 temp-named handles to no-arg `WorkspaceES()`/`WorkspaceView()` (the names are free now that the 3-arg pair is gone). Keep `ChatES()` (deleted in Task 13).
- Modify: `api/internal/app/container.go` (construct `axWorkspace` + `axReviewThread` singletons; stop passing the per-entity `AsynxFactory`; **retain both singletons on `app.Container` for Task 15 shutdown — see Interfaces**)
- Modify: `api/internal/app/repositories/container.go` (wire singleton + read-model DB via `adapters.WorkspaceView()`; **construct `pathsStore := wspaths.NewWorkspacePaths(adapters.GlobalView())` here (Task 2's view.db store, package `wspaths` — NOT `paths`) and pass it into `workspace.New` as its third arg — `repositories.New` already receives `adapters`, so no new `repositories.New` param is needed; see Interfaces "pathsStore threading"**; **extract the existing `broadcastWorkspace` enrichment into the shared injected `enrich` callback (the production wiring DEFERRED from Task 6) and pass it into `RegisterHub(axWorkspace, enrich, broadcast)`; route `BeginWork`/`EndWork` through the SAME `enrich`+`broadcast` so the FE spinner + merge badges survive the store/hub split**; **retain `axWorkspace`/`axReviewThread` for the Task 15 drain — see Interfaces**)
- Test: **rewrite/delete every test hard-coupled to the per-entity model this task deletes — otherwise these packages fail to compile at this commit, breaking the "each task ends green" rule** (the deleted symbols are the 3-arg `WorkspaceES/WorkspaceView`, the `Registry`, `writeMu`, `entityFor`/`entityForLocation`, the `locations` field, `AsynxFactory`/`WithMaxOpenEntities`, and the `storages/` layout):
  - `api/internal/app/repositories/workspace/workspace_concurrency_test.go` (rewrite: no writeMu; OCC retry).
  - `api/internal/app/repositories/workspace/workspace_test.go` — rewrite for the new `workspace.New(axWorkspace, storeDB, pathsStore, broadcast, ...)` signature (call sites `:49,59,135,207,219,447` use the old 3-arg `workspace.New(adapters, broadcast, wsAsynxFactory)`); delete the `wsAsynxFactory` test factory (`:25`), the `workspace.WithMaxOpenEntities` bounded-cache test (`:468`), the `storages/` on-disk assertions (`:147-150`), and the `entityFor`/`entityForLocation` release-after-use comment/assertions (`:455`) — all reference the deleted `Registry`/`asynxFactory`/`WithMaxOpenEntities`/`storages` layout.
  - `api/internal/app/repositories/workspace/export_test.go` — **delete `CachedEntityCount`** (it type-asserts `*workspace` and calls `w.entities.Len()` on the now-deleted `entities *Registry`; nothing can reference the LRU once it's gone).
  - `api/internal/adapter/container_test.go` — **remove the per-entity tests that drive the deleted 3-arg `WorkspaceES(p,r,w)`/`WorkspaceView(p,r,w)` + `storages/` layout**: the `storagesDir` helper (`:16`), `TestWorkspaceES_LazyOpenCreatesEventStreamDB` (`:33`, call `:43`), `TestWorkspaceView_LazyOpenCreatesViewDB` (`:52`, call `:62`), `TestWorkspaceES_CachedSecondCall` (`:71`), `TestClose_ClosesAllAndLock` (`:110`, calls `:115,118`, re-open `:130`), `TestRegression_AccessorsRefuseReopenAfterClose` (`:138`, calls `:148,150`, dir assert `:154`), `TestWorkspaceES_MkdirError` (`:157`, calls `:168,171`); re-express `Close()` coverage as close-all over the new no-arg per-type handles. **Leave `TestChatAndReviewThreadES_Global` (`:100`) intact here** — its `c.ReviewThreadES()` assert survives, and its `c.ChatES()` assert at `:106` is trimmed by Task 13 (`ChatES()` is still live through Task 12).

**Interfaces:**
- Produces: `workspace.New(axWorkspace, storeDB, pathsStore, broadcast, ...) (Workspace, error)`; mutations call `sendWithOCC(ctx, cmd)` (retry ≤5× on `asynxmodels.ErrPipelineFailed`; **OCC-exhaustion terminal disposition — after the 5 retries are exhausted and `errors.Is(err, asynxmodels.ErrPipelineFailed)` still holds, map it to HTTP 409 Conflict** (per §3.5's `ErrPipelineFailed→OCC retry ≤5×→409/500`): a still-failing pipeline after retries is an unrecoverable optimistic-concurrency/version collision, so **409 is chosen over 500** and it slots into the **existing 409 Conflict category of `libs.StatusAndMessage`** (`internal/api/libs/status.go`, which already returns 409 for `apperr.ErrLocked` + the git conflict sentinels) by adding `asynxmodels.ErrPipelineFailed` to that guard — all via `errors.Is`, never string compare; never retry `asynxmodels.ErrValidation` → **HTTP 422** (Unprocessable Entity — per spec §3.5 pseudocode step 3 `ErrValidation→422` and the command-flow narrative; a state-machine guard rejection is 422, NOT 400 — 400 in §3.5 is reserved for the earlier decode/shape-validate "bad input, fail fast" step, which is a separate pre-`Send` check; there is no central `ErrValidation`→HTTP mapping in `internal/api` today, so this is net-new behavior where the spec's value is authoritative); **map `asynxmodels.ErrQueueFull` → HTTP 503** — a sentinel that becomes newly-reachable in this refactor **not** because of `Send` vs `SendWait` (in asynx v0.6.2 both route through `processor.sendAndWait` (`processor.go:180-201`), whose `default: return ErrQueueFull` applies either way), but because **`writeMu` is deleted and one shared `axWorkspace` instance now absorbs concurrent `Send`s under load**: today's per-entity instance + `writeMu` keeps at most one command in-flight per aggregate, so no shard queue ever fills. Once that serialization is gone, a shard queue can fill, so map it explicitly — add an `apperr.ErrUnavailable` sentinel mirroring `apperr.go:11/19/26` (`ErrNotFound`/`ErrLocked`/`ErrInvalidArgument`) and `libs.WriteErr(ctx, http.StatusServiceUnavailable, …)`; all mapping via `errors.Is`, never string compare).
- **§3.5 command-flow deviation (flagged per the plan's "spec wins — flag it" rule):** the HTTP mutation handlers are **intentionally left returning the full applied `evt.Aggregate` synchronously** (via `Send`'s applied-aggregate return), NOT §3.5's literal "return 202 + `{id}` ack-only". This is a deliberate NON-implementation of §3.5's 202-ack shape, chosen to honor the equally-binding "zero FE regressions" constraint (Global Constraints / §1.3 / §4 Frontend) — the frontend still reads the full entity off the mutation response. Recorded here as a conscious deviation, not an oversight; the projection→WS hub still delivers the authoritative post-commit frame.
- **pathsStore threading:** `repositories.New` builds `pathsStore := wspaths.NewWorkspacePaths(adapters.GlobalView())` (Task 2's view.db store in package `wspaths` — deliberately NOT named `paths` to avoid shadowing `core/paths`, backed by the adapter's `GlobalView()`) and passes it as the third arg of `workspace.New(axWorkspace, storeDB, pathsStore, broadcast, ...)`. **No new `repositories.New` parameter is required** — it already receives `adapters`, so the `app/container.go` → `repositories.New` call is unchanged for the paths wiring (only the `axChat`/`AsynxFactory` args change per the bullets below). Constructing it here from `adapters.GlobalView()` keeps view.db ownership with the adapter (spec §3.9). (The `GORMStores.WorkspacePaths`-field alternative is **foreclosed** — Task 2 deliberately does NOT add such a field: it would be an unused, wrong-shape `store.Store[T,string]` slot failing `deadcode`; see Task 2's spec-deviation flag.)
- Consumes: `adapters.WorkspaceES()` (Task 1, promoted from its temp name this task), the store projection (Task 5) as the id→{project_id,repo_id} authority, `pathsStore` (Task 2) as the id→path map, hub (Task 6).
- **Retire `internal/locations` from workspace.go — every `w.locations.*` site is re-homed so the package is genuinely unreferenced by Task 18 (§3.7/§3.9):**
  - `entityFor.Get` (`:301`) → the store projection `Get(id)` (the read model carries `{project_id, repo_id}` and doubles as the location index, §3.7). The store projection's `Get` translates its package-local not-found → `apperr.ErrNotFound` here, exactly as `workspace.go:307` did for `locations.ErrNotFound`, so a bogus workspace id still maps to HTTP 404.
  - **`wspaths.ErrNotFound` → `apperr.ErrNotFound` boundary translation (Fix, per Task 2):** wherever the workspace repo consumes `pathsStore.Get` (Task 2's `wspaths` store, which returns its own package-local `wspaths.ErrNotFound`, NOT `apperr.ErrNotFound` — the adapter layer does not import `app/apperr`) and surfaces the result to an HTTP handler, translate `wspaths.ErrNotFound` → `apperr.ErrNotFound` at this workspace-repo boundary, mirroring the `workspace.go:307` `locations.ErrNotFound`→`apperr.ErrNotFound` translation. (The Task 8 delete reactor, which also reads `pathsStore.Get`, instead treats not-found as an idempotent no-op — nothing to `rm` — rather than a 404.)
  - Create's `w.locations.Save` (`:363`) → **write the initial `workspace_paths` row `pathsStore.Put(ctx, id, in.WorktreePath)`** (§3.9 write-point (a); `in.WorktreePath` is the human-readable path the provisioner derived via `Derive` in Task 3b, `workspace.go:38`) while the store projection persists `project_id`/`repo_id` from the `workspace.created` event; the rollback `w.locations.Delete` (`:379`) → `pathsStore.Delete(ctx, in.ID)` on Create failure.
  - `List.List` (`:714`) → the store projection `List(ctx)` (Task 5).
  - Delete's `w.locations.Get` (`:650`) + `w.locations.Delete` (`:671`) → **removed outright in THIS task** when `Delete` becomes a pure `Send(commands.Delete{ID:id})` (see Files) — they are NOT re-homed. The equivalent post-commit path resolution + `pathsStore.Delete` are performed independently by the **Task 8 delete reactor** (which reads the store projection and calls `pathsStore.Delete`), re-deriving them fresh rather than inheriting a value off this now-deleted synchronous read path.
- **Singleton retention for shutdown (Task 15):** `app.New` stores `axWorkspace`/`axReviewThread` on `app.Container` (or `repositories.Container` exposes a drain accessor over them) rather than letting them fall out of scope after `repositories.New`, so Task 15's `Shutdown(ctx)` can `ax.Shutdown` each. (The shared drain `WaitGroup` itself is created/held by Task 14's `wireCallbacks`.)

- [ ] **Step 1: Failing test** — concurrent `Send`s to the same workspace id converge without `writeMu` (OCC): fire N goroutines issuing a **real** live command; assert final state is consistent and no `ErrPipelineFailed` escapes (retried). `-race`.

```go
func TestConcurrentSends_NoWriteMu_OCC(t *testing.T) {
    // spin real axWorkspace over temp stores; fire 20 concurrent TouchActivity
    // commands on ws-1 (a real live command — there is NO SetStatus command;
    // SyncWorkingTreeState works equally as the contended mutation).
    // assert: no error escapes, final row present, version monotonic
}
```

Also add the **error-mapping tests** for `sendWithOCC`'s disposition contract (asserting the exact `libs.StatusAndMessage` status per §3.5, via `errors.Is`):

```go
func TestSendWithOCC_ErrorDisposition(t *testing.T) {
    // ErrPipelineFailed that NEVER converges → retried exactly 5× then surfaced;
    // libs.StatusAndMessage(err) == http.StatusConflict (409) — OCC exhaustion.
    // ErrValidation                         → NOT retried; == 422 (Unprocessable Entity).
    // ErrQueueFull                          → NOT retried; == 503 (Service Unavailable).
}
```

- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement.** In `workspace.go`: delete the entity `Registry` + `writeMu` + `entityFor`/`entityForLocation`; hold `axWorkspace`; add `sendWithOCC`; **rewrite `Delete` (`:646-685`) to `func (w *workspace) Delete(ctx context.Context, id string) error { _, err := w.sendWithOCC(ctx, commands.Delete{ID: id}); return err }`** (removing the old `w.locations.Get`/`entityForLocation`/`entity.forget`/`w.entities.Evict`/`w.locations.Delete`/`removeWorkspaceDir` body wholesale — its io is the Task 8 reactor's job); re-home every remaining `w.locations.*` call per Interfaces (Create writes the initial `workspace_paths` row via `pathsStore.Put(ctx, id, worktreePath)` and relies on the store projection for `project_id`/`repo_id`; List reads the store projection; the `locations` field + import are removed — and because `Delete` no longer touches `w.locations`, nothing references the field after the strip). In `internal/store`: **delete the combined `projections.go`**, rework `store.New`/`reconcile` (`store.go:44-85`) to drop the `aggregateID` param + eager reconcile, and **register `RegisterStore` (Task 5) + `RegisterHub` (Task 6) exactly once on `axWorkspace`** (the production wiring both additive tasks deferred here); in `repositories/container.go` extract the `broadcastWorkspace` enrichment into the shared `enrich` callback and inject it into `RegisterHub`, routing `BeginWork`/`EndWork` through the same `enrich`. In `adapter/container.go`: delete the per-entity `WorkspaceES(p,r,w)`/`WorkspaceView(p,r,w)` methods + `Registry` fields + `storages/` logic and promote the Task 1 temp handles to `WorkspaceES()`/`WorkspaceView()`. In `app/container.go`: construct `axWorkspace` + `axReviewThread`, stop passing the `AsynxFactory`, and retain both singletons on `app.Container` for Task 15. **`axChat` stays live and wired** — chat's repo still consumes it until Task 13, so keep constructing it and passing it to `repositories.New` (no `_ = axChat` guard, no `wip:` — with `Delete` now a pure `Send`, `go build ./...` and the workspace package's `go test -race` are both green at this commit).
- [ ] **Step 4: Run — PASS** the concurrency test AND the error-disposition test; `go build ./... && go vet ./...` stay green — rewriting `Delete` to a pure `Send` in the same commit closes what used to be the delete-reactor seam, so there is no interim break (Task 8 only ADDS the purge reactor).
- [ ] **Step 5: Commit** `git commit -am "refactor(workspace): singleton axWorkspace; delete Registry/LRU/writeMu/entityForLocation; Delete via Send"`

---

### Task 8: Delete lifecycle (persist-then-purge) + delete reactor with ordering gate

**Spec:** §3.6 (reactor + ordering/idempotency contract), §3.8 (delete lifecycle). Closes the current `workspace.go:665-674` crash gap — the *persist* half (`Delete` → `Send(commands.Delete{})`) already landed in **Task 7**; this task adds the *purge* half (the gated post-commit reactor).

**Scope:** this task **produces and unit-tests** `RegisterDeleteReactor` in isolation (driving it with a **test-injected** `drainWG` and test doubles for `storeReader`/`pathsStore`/`reviewThreadForget`/`rmWorktree`). `workspace.go`'s `Delete` is **already** the pure `Send(commands.Delete{ID:id})` from Task 7 — this task does **NOT** touch `workspace.go`; it only adds the reactor package. The **production** wiring — registering the reactor through `wireCallbacks` and handing it the shared production `drainWG`/drain-context — does NOT exist yet: both `wireCallbacks` and the shared `drainWG` are first created in **Task 14**. So do NOT touch `repositories/container.go` here either; there is no seam to wire into at this commit.

**Files:**
- Create: `api/internal/app/repositories/workspace/internal/reactors/delete.go` (+ tests). **Do NOT modify `workspace.go` — `Delete` is already `Send(commands.Delete{ID:id})` from Task 7.**
- (production wiring deferred) `api/internal/app/repositories/container.go` — NOT modified here; the reactor is registered via `wireCallbacks` with the shared production `drainWG` in **Task 14**.
- Test: alongside (`-race`) with a test-injected `drainWG`, plus the ordering-gate test.

**Interfaces:**
- Produces: `RegisterDeleteReactor(axWorkspace, storeReader, pathsStore, reviewThreadForget func(ctx, wsID) error, rmWorktree func(path) error, drainWG)` subscribing `Topic("workspace.deleted.*")` (regex `^workspace\.deleted\..*$` — a bare `"workspace.deleted"` would never fire; spec §3.6). Reactor: gate on observing persisted `Status=deleted` row → resolve the worktree path (store projection `{project_id,repo_id}` + `pathsStore.Get`) → cascade `reviewThreadAx.Forget` per thread → `rm -rf` worktree → **`pathsStore.Delete(ctx, wsID)`** (§3.9 write-point (c) — removes the id↔path row) → `axWorkspace.Forget(wsID)` (terminal; its `OnForget` drops the store-projection row). `context.WithoutCancel`, timeout-bounded, on drain WG. This reactor performs — post-commit and idempotently — the same purge io the old synchronous `Delete` body did inline (`w.locations.Get`/`entity.forget`/`Evict`/`w.locations.Delete`/`removeWorkspaceDir`, `workspace.go:650-674`), all of which Task 7 deleted when `Delete` became a pure `Send`; the reactor re-derives the path fresh from the store projection + `pathsStore` (a not-found `pathsStore.Get` is an idempotent no-op, not a 404) rather than inheriting it from the deleted read path. **The `drainWG` param is satisfied by a test-injected `WaitGroup` in this task's unit tests; the shared PRODUCTION `drainWG` (+ its registration through `wireCallbacks`) is created and passed in by Task 14** — so `RegisterDeleteReactor` is built to accept an injected WG here but is not production-wired until then.

- [ ] **Step 1: Failing test** — `Delete` persists a `deleted` row (visible in `List`) and the worktree is removed only after the row is observed; killing between persist and purge leaves a `deleted` row + worktree that a re-drive reaps to (no row, no worktree, no `workspace_paths` row). Assert the ordering gate: reactor does not `rm` before the row is present; assert the `workspace_paths` id↔path row is deleted as part of the purge.
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement** per §3.6 contract (bounded wait for the tombstone; `pathsStore.Delete(ctx, wsID)` before the terminal `Forget`; `Forget` terminal; every step idempotent).
- [ ] **Step 4: Run — PASS** (`-race`). `go build ./...` stays green (it was already green after Task 7; this task only ADDS the `reactors/delete.go` package).
- [ ] **Step 5: Commit** `git commit -am "feat(workspace): gated delete reactor for persist-then-purge (closes crash-orphan gap)"`

---

### Task 9: Reconcile-on-open (lazy, background, bounded) + recovery SendWait

**Spec:** §3.8 (reconcile-on-open), decisions 4 (the sanctioned SendWait), 9.

**Files:**
- Create: `api/internal/app/repositories/workspace/internal/reconcile/reconcile.go`
- Modify: `workspace.go` (`Get`/detail triggers a deduped one-shot background reconcile task; `List` does NOT)
- Test: alongside.

**Interfaces:**
- Produces: `Reconciler.OnOpen(ctx, wsID)` — dedup (one task per id); the task does cancelable/timeout-bounded git+provider re-derivation, then `SendWait`s a **pure** sync command and broadcasts the corrected frame. `List` path never calls this.

- [ ] **Step 1: Failing test** — a `Get` after boot dispatches exactly one reconcile task per id (repeat opens don't stack); a `List` dispatches none; the reconcile updates the read model + broadcasts.
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement.** Background task with dedup map; IO bounded by `context.WithTimeout`; the command it fires is pure (io already done).
- [ ] **Step 4: Run — PASS** (`-race`).
- [ ] **Step 5: Commit** `git commit -am "feat(workspace): lazy reconcile-on-open (bounded IO off the read path)"`

---

### Task 10: Boot orphan-sweep

**Spec:** §3.8 (boot orphan-sweep).

**Files:**
- Create: `api/internal/app/repositories/workspace/internal/reconcile/sweep.go`
- Modify: `api/internal/app/container.go` — there is **no `app.Start` seam**. Boot work runs synchronously inside `app.New` (today `startProviderSweep`/`startRecoverySweep`/`startRestoreTerminalSessions` at `container.go:76-78`, before `app.New` returns and thus before `internal.Run` serves). Invoke the new `Sweep(ctx)` synchronously here, **in place of** `startRecoverySweep(ctx, ucs)` (`:77`). **Also remove the now-unused import `github.com/char2cs/crowbar/api/internal/core/safego` (`app/container.go:14`)** — its sole user is the deleted `startRecoverySweep` (the `safego.Go("app.recoverySweep", …)` call at `:143`); the new `Sweep(ctx)` runs **synchronously** (no `safego.Go`), so leaving the import in place is an "imported and not used" compile error. Task 10 MUST end green — it is NOT the sanctioned interim-break task (only T13 is).
- Delete: the broken recovery sweep this replaces — `startRecoverySweep` (definition `app/container.go:139-146` + call site `:77`) AND its sole production callee `worktree.Usecase.ReconcileAll` (interface `usecases/worktree/worktree.go:82`; impl `worktree.go:1010`, doc comment from `:1000`). Its only other references are that usecase's own tests (`worktree_test.go` `TestReconcileAll_*`, ~`:1664-1817`) — delete that whole block — **and, because those tests are the EXCLUSIVE consumers of two test-helper types, delete the now-orphaned helpers too**: `perPathSummaryGit` (doc comment + type + method, `worktree_test.go` ~`:479-498`; used only at `:1678` and `:1791`, both inside the deleted `:1664-1817` block) and `perRepoStore` (~`:500-515`; used only at `:1768`, also inside that block). Leaving them makes them unreferenced dead code (fails Task 18's `deadcode` gate) AND their doc comments still contain the string `ReconcileAll`. **Correcting the count: after deleting the `TestReconcileAll_*` block, FOUR `ReconcileAll` string sites survive in `api/internal`, split by disposition:**
  - **(i) `engine/fs/internal/watch/watcher.go:226`** and **(ii) the `resyncSummary` doc comment at `usecases/worktree/worktree.go:586`** ("…callers (e.g. ReconcileAll) can act on it…") — both sit on **surviving** code, so they are **refreshed** (reword to drop the `ReconcileAll` reference). The `worktree.go:586` comment is on the surviving `resyncSummary` function.
  - **(iii) `worktree_test.go:480`** (doc comment on `perPathSummaryGit`) and **(iv) `worktree_test.go:500`** (doc comment on `perRepoStore`) — these are **eliminated by deleting the two orphaned helpers** above (no separate reword needed; the comment goes with the type).

  All four matter because Task 18's guard `grep -rnE '…ReconcileAll…' api/internal` MUST return NOTHING; any of the four left in place fails that gate. Once `startRecoverySweep` is gone, `ReconcileAll` is unreferenced and would otherwise fail the Task 18 `deadcode` gate; leaving `startRecoverySweep` in place would run TWO sweeps.
- Test: alongside.

**Interfaces:**
- Produces: `Sweep(ctx)` — reads `store/workspace.db` directly (NO lazy Replay trigger); any residual `Status=deleted` row → re-drive the idempotent purge (cascade Forget + rm + `axWorkspace.Forget`); half-provisioned worktree → complete/clean. Invoked synchronously from `app.New` before it returns (replacing the old async `startRecoverySweep`).

- [ ] **Step 1: Failing test** — seed a `deleted` row + lingering worktree, run `Sweep`, assert row + worktree gone; seed a clean tree, assert `Sweep` is a no-op and triggers no replay.
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run — PASS.**
- [ ] **Step 5: Commit** `git commit -am "feat(workspace): boot orphan-sweep (reap deleted-but-lingering)"`

---

### Task 11: Lazy read-model Replay repair

**Spec:** §3.7 (lazy Replay), decision 7.

**Files:**
- Modify: workspace store (List detects empty-model + non-empty-log → rebuild)
- Create: `api/internal/app/repositories/workspace/internal/store/rebuild.go`
- Test: alongside.

**Interfaces:**
- Produces: on `List` when the read model is empty AND `es.(serialize.AggregateLister).AggregateIDs(ctx)` is non-empty → enumerate ids, `axWorkspace.Replay` each into `store/workspace.db`, then return the list. A per-id `Get` never triggers rebuild (folds from log directly).

- [ ] **Step 1: Failing test** — create workspaces, delete `store/workspace.db` file (event log intact), call `List` → rebuilt via Replay, list correct; assert boot (no List) did NOT replay.
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement** the empty-model/non-empty-log trigger + `AggregateLister` enumerate + per-id Replay.
- [ ] **Step 4: Run — PASS.**
- [ ] **Step 5: Commit** `git commit -am "feat(workspace): lazy read-model Replay repair on empty-model List"`

---

### Task 12: ReviewThread — full conversion (equal to Workspace)

**Spec:** §3.1, §4 (reviewthread/** bullet) — this is the big one for ReviewThread.

**Files:**
- Modify: `api/internal/app/repositories/reviewthread/**` (relocate stores; delete `serialize.KeyedMutex` writeMu **usages**; `SendWait`→`Send`+OCC; split combined projector into `store.go`+`hub.go`; drop eager reconcile-on-open → lazy Replay)
- Modify: `api/internal/app/repositories/container.go` — rewire the `reviewthread.New(...)` caller (`:63`) to reviewthread's new Task-12 signature: pass `adapters.ReviewThreadView()` (Task 1's `state/store/review_thread.db` read-model handle) as the read-model DB in place of the shared `db := adapters.GlobalView()`; keep `adapters.ReviewThreadES()` (still needed for the lazy `AggregateLister` Replay, §3.7). reviewthread's read model no longer lives in view.db. **Leave the `db := adapters.GlobalView()` declaration at `:58` in place for now** — chat still consumes it at `:59` until Task 13, which removes both `:59` and the then-orphaned `:58` (see Task 13).
- Test: `api/internal/app/repositories/reviewthread/reviewthread_test.go` — **rewrite it for the Send+OCC / no-`writeMu` / lazy-`Replay` shape** (it does not compile/pass as-is after the conversion): today it is a black-box suite that drives the synchronous `SendWait` API — e.g. `resolved, _ := repo.Resolve(ctx, "t1")` immediately asserts `resolved.Status` (`:45-47`), Open→Resolve→Reopen back-to-back (`:39-50`) — and constructs the pre-conversion `reviewthread.New(ctx, ax, es, db, broadcast)` (`:34`). Update every call to the new `reviewthread.New(axReviewThread, es, storeDB, broadcast)` signature and Send-based flow, and extend it with the four Step-1 cases below (concurrent-replies-OCC, store projection persist, lazy rebuild, unchanged hub frame) — mirroring Tasks 5/6/7/11 tests for reviewthread.

**Interfaces:**
- Produces: `reviewthread.New(axReviewThread, es, storeDB, broadcast)` where `storeDB = adapters.ReviewThreadView()` (`state/store/review_thread.db`) and `es = adapters.ReviewThreadES()` (`state/events/review_thread.db`, retained only to reach the lazy `AggregateLister` — matching how Workspace consumes `WorkspaceES()`; §3.7 wins over the reviewer's "drops es" shorthand since lazy Replay requires it); store+hub projections off `evt.Aggregate`; lazy Replay on empty-model List.

- [ ] **Step 1: Failing tests** — (a) concurrent replies to one thread converge without writeMu (OCC, `-race`); (b) store projection persists to `store/review_thread.db`; (c) lazy rebuild works; (d) hub broadcasts unchanged frame shape (no FE regression).
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement** per §4 reviewthread deltas: relocate DBs, delete `reviewthread.go:93` writeMu + the `writeMu`+`SendWait` mutation block at `145-264` (Open through Reopen — Reopen's `SendWait` is at `:259`, its closing brace at `:264`; `:258` is only Reopen's `defer writeMu.Unlock`), split `internal/store/projections.go` into store+hub, move `New`'s eager `aggregateIDs()`+reconcile behind the lazy trigger. **Remove reviewthread's `serialize.KeyedMutex` usages only (the `writeMu` field + Lock/Unlock calls); do NOT delete the shared `internal/serialize/keyed_mutex.go` file here** — chat (`chat.go:66`) still imports it until Task 13, so the now-unreferenced `keyed_mutex.go` + `keyed_mutex_test.go` are deleted in **Task 18**. **Keep `internal/serialize/aggregate_lister.go`** (workspace Task 11 + reviewthread lazy Replay still type-assert `serialize.AggregateLister`).
- [ ] **Step 4: Run — PASS** (`-race`).
- [ ] **Step 5: Commit** `git commit -am "refactor(reviewthread): full asynx-alignment (Send+OCC, split projections, central stores, lazy replay)"`

---

### Task 13: Remove Chat aggregate + live `branchreview` consumer

**Spec:** §4 (Chat-removal scope — exact files), decisions 2, 15. **FE-safe** because `review-api.ts:115` reads `conversations ?? []`.

**Files (all must change to compile the big-bang delete — decision 15 forbids leaving any orphaned referent; deleting `repositories/chat` + `usecases/chat` cascades far past the spec §4 §-listed files):**

_Branch Review consumer (spec §4 Chat-removal scope, lines 210-217):_
- Modify: `api/internal/app/usecases/branchreview/branch_review.go` — drop the `chat` import (`:14`) + `branchchat` import (`:17`), the `chats chat.Chat` field (`:71`), the ctor param (`:81`) + assignment (`:89`), the `u.chats.ListByWorkspace(ctx, ws.ID)` call (`:188-190`), and the `Conversations: branchchat.From(...)` assembly (`:197`).
- Modify: `api/internal/domain/branch_review.go:14` (drop `Conversations []BranchChat`).
- Modify: `api/internal/api/v0/dto/review.go` (drop the `Conversations` `[]domain.BranchChat` field `:44` + the nil-guard/assignment mapping `:103-113`).
- Delete: `api/internal/domain/branch_chat.go`, `api/internal/app/usecases/internal/branchchat/` (helper `branchchat.go` + `branchchat_test.go`).

_Chat aggregate delete (spec §4 line 217) — every referent that goes dead once `repositories/chat` + `usecases/chat` are removed:_
- Modify: `api/internal/app/usecases/container.go` — **FULL** edits, not just the branchreview arg: remove the `chat` import (`:7`), the `Chat chat.Usecase` field (`:38`), the `chatUsecase := chat.New(repos.Chat, repos.Workspace, projectUsecase, nowFunc)` construction (`:69-74`), the `Chat: chatUsecase` return field (`:138`), AND the `repos.Chat` argument to `branchreview.New` (`:128`).
- Modify: `api/internal/app/repositories/container.go` — remove the `chat` import (`:13`), the `Chat chat.Chat` field (`:23`), the `axChat asynx.Asynx[domain.Chat]` param (`:46`), the `ch, err := chat.New(...)` construction (`:59-62`), and the `c.Chat = ch` assignment (`:68`). **Remove the now-unused `db := adapters.GlobalView()` at `:58`** — reviewthread was migrated off it in **Task 12**, and chat (its last consumer, `:59`) is deleted here, so leaving `db` would yield a `declared and not used` compile error. (The `domain`/`asynx` imports stay for `axReviewThread`.)
- Modify: `api/internal/app/container.go` — delete the `axChat` construction (`:39-42`) and its argument to `repositories.New` (`:58`); **KEEP** the adjacent `axReviewThread` construction (`:43-46`) and argument (`:59`) — do NOT delete by an old `39-46`/`58-59` range or you remove `axReviewThread`.
- Modify: `api/internal/adapter/container.go` — delete the `ChatES()` method (`:149-151`), the `chatES asynxModels.Store` field (`:34`), its construction `eventsqlite.NewEventStore(filepath.Join(stateDir, "chat_"+eventStreamDBName))` (`:118`), and its inclusion in the close set (`closeIfCloser(chatES)` `:124,130` + `collectClosers(chatES, reviewThreadES)` `:142`). This is the `ChatES()` deletion **deferred from Task 1** (ratification path (a)); once `app/container.go:39` and `repositories/container.go:59` stop calling `ChatES()` (above), the method + field are unreferenced and decision 15 requires removing them.
- Modify: `api/internal/adapter/container_test.go` — **delete the `assert.NotNil(t, c.ChatES())` assertion at `:106`** inside `TestChatAndReviewThreadES_Global` (`:100`), reducing that test to the `ReviewThreadES()`-only assertion (`:107`) — otherwise the adapter test package fails to compile the moment `ChatES()` is removed above. (Task 7 already stripped this file's per-entity `WorkspaceES/WorkspaceView` tests; this is the one remaining `ChatES` referent, deferred here because `ChatES()` stays live through Task 12.)
- Delete: `api/internal/api/v0/dto/chat.go` (`ChatDTO` `:13`, `ChatDTOFrom` `:23`, `ChatDTOList` `:38`) + its `dto/chat_test.go`.
- Delete: `api/internal/api/v0/endpoints/chats/` entire tree (`routes.go`, `handlers/`, and their `_test.go`) — confirmed dormant: nothing outside the package imports it, the router never mounts it (stale TODO at `router.go:115-118` — the comment block is FOUR lines: `:115-117` is the prose and `:118` is the trailing `// /v0/projects/:p/repos/:r/workspaces/:w/chats` path line, part of the same TODO; removing only `:115-117` would strand `:118` as a dangling comment), both `routes.go` and `handlers/` reference `domain.Chat`.
- Delete: `api/internal/domain/chat.go` (the `domain.Chat` type — no referent survives once all of the above go).
- Modify: `api/internal/app/usecases/mocks/mocks.go` — surgically remove the chat fakes `ChatForkArgs`/`ChatRepo`/`ChatWorkspaceRepo` (~`:559-690`); this is a **shared** mocks package (keeps `ProjectStore`/`WorkspaceRepo`/`GitEngine`/etc.), so delete only the Chat structs, NOT the file.
- Modify: `api/internal/api/v0/container.go` — delete the stale chat comment (`:31-32`); also drop the sibling stale TODO at `router.go:115-118` (all four lines — both name the now-deleted "chat domain, repo CRUD, and usecase"; `:118` is the `chats` path comment that closes the block).
- Modify: `api/internal/app/usecases/container_test.go`, `api/internal/app/repositories/container_test.go`, `api/internal/app/usecases/branchreview/branch_review_test.go`, `branch_review_bench_test.go` — drop chat/`Conversations` references so those packages still compile + pass.
- Delete: `api/internal/app/repositories/chat` (repo + its `internal/mocks/mocks.go`) and `api/internal/app/usecases/chat` (usecase) — the whole packages.
- Test: `api/internal/api/v0/dto/review_test.go` (assert `/review` DTO has no `conversations` key), and a web-side check that Branch Review still renders.

- [ ] **Step 1: Failing/guard test** — `BranchReviewDTOFrom` produces JSON without a `conversations` key; and the dead-code guard `grep -rnE 'domain\.Chat|ChatDTO|ChatES|repos\.Chat|axChat|endpoints/chats|usecases/chat|repositories/chat|BranchChat|branchchat|\.Conversations' api/internal` returns nothing after the change (the earlier `BranchChat|branchchat|.Conversations` grep missed every file in this expanded list; `ChatES` catches the adapter method deleted this task).
- [ ] **Step 2: Run — FAIL** (still references exist).
- [ ] **Step 3: Implement** all deletions in the listed files. Verify `web/src/features/git/api/review-api.ts` still maps `raw.conversations ?? []` (no change needed — degrades to empty).
- [ ] **Step 4: Run — PASS.** `go build ./... && go vet ./...` clean; `cd web && bun tsc --noEmit` clean.
- [ ] **Step 5: Commit** `git commit -am "refactor: remove deferred Chat aggregate + branchreview consumer (no dead code)"`

---

### Task 14: `wireCallbacks` — cross-aggregate reactions

**Spec:** §3.6, §4 (repositories/container.go bullet).

**Files:**
- Modify: `api/internal/app/repositories/container.go` (add `wireCallbacks` invoked at construction; **add the shared `drainWG *sync.WaitGroup` + cancelable drain-context fields to `repositories.Container` and a drain accessor `app.Container` can reach for Task 15's `Shutdown`**)
- Test: cross-aggregate cascade test.

**Interfaces:**
- Produces: `wireCallbacks()` registering: the workspace delete reactor (Task 8), reviewthread forget-on-workspace-delete cascade, and the hub projections (Tasks 6/12). **`wireCallbacks` creates and stores the shared drain `WaitGroup` + drain context on `repositories.Container` (exposed to `app.Container` via a drain accessor); every reactor it registers joins that WG** (decisions 9 + 11), so Task 15's `Shutdown(ctx)` can wait on it.

- [ ] **Step 1: Failing test** — deleting a workspace forgets its review threads (their rows vanish) and removes the worktree; assert cascade.
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement** `wireCallbacks` (mirror quiver container `wireCallbacks`).
- [ ] **Step 4: Run — PASS** (`-race`).
- [ ] **Step 5: Commit** `git commit -am "feat(app): wireCallbacks cross-aggregate reactions (workspace delete → cascade)"`

---

### Task 15: Graceful shutdown — ordered, bounded drain-all + close-all

**Spec:** §3.8 (shutdown), decision 11.

**Files:**
- Modify: `api/internal/internal.go` — the real teardown seam (NOT a `Start`). **Signal handling already exists** upstream at `cmd/crowbar/main.go:53` (`signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`, with `container.Run(ctx)` blocking until cancel and a `defer container.Close()`) — **no new signal wiring**. Insert the new drain-all step into the existing ordered teardown: `Run` already stops the HTTP server (hardcoded 5s `shutdownCtx` at `:114` + `c.server.Shutdown(shutdownCtx)` at `:116`); `Close` then runs `app.Close → engines.Close → adapter.Close → listener.Close` (`:132-140`). The new drain must run **after** `server.Shutdown` and **before** `adapter.Close` (`:138`). Because `Close()` currently takes no ctx (the 5s `shutdownCtx` lives and is cancelled inside `Run`), thread a bounded shutdown ctx to the drain step (e.g. give `Close` a ctx, or drain at the tail of `Run` before it returns).
- Add: a new `app.Container.Shutdown(ctx)` method — it does **NOT** exist today. The app layer exposes only `Close()` (`app/container.go:105-107`, which does just `c.Realtime.Close()`), and there is **no** `ax.Shutdown`/drain anywhere in the tree. `Shutdown(ctx)` closes the drain gate, `drainWG.Wait()` **bounded by ctx**, then calls `ax.Shutdown` for EACH singleton (`axWorkspace`, `axReviewThread`). **This task only consumes state that earlier tasks establish:** the two `ax` singletons are retained on `app.Container` by **Task 7**, and the shared drain gate (`drainWG` + drain context) is created/held by **Task 14**'s `wireCallbacks` and reached here via the `repositories.Container` drain accessor. Keep the existing `app.Close()` (Realtime) — `Shutdown` drains asynx, `Close` still releases realtime resources.
- Test: `api/tests/integration/...` drain-integrity (Task 17 also covers).

**Interfaces:**
- Produces: on ctx cancel (from `cmd/crowbar/main.go:53`), `internal.Run` returns after `server.Shutdown`, then teardown runs `app.Shutdown(shutdownCtx)` (drain gate closes, `drainWG.Wait()` **bounded by ctx**, then `ax.Shutdown` per singleton) → `app.Close()` (Realtime) → `engines.Close()` → `adapter.Close()` → `listener.Close()`. Every wait honors `shutdownCtx`.

- [ ] **Step 1: Failing test** — under simulated in-flight reactor work, `Shutdown(ctx)` returns within the deadline (does not hang) and all DBs are closed; a hung reactor is bounded by ctx (not unbounded like quiver).
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement** the ordered, bounded teardown; drain gate + `select { case <-done: case <-ctx.Done(): }` around each wait.
- [ ] **Step 4: Run — PASS.**
- [ ] **Step 5: Commit** `git commit -am "feat(daemon): ordered, ctx-bounded graceful shutdown (drain-all + close-all)"`

---

### Task 16: Integration test harness (quiver's kit, adapted)

**Spec:** §5.

**Files:**
- Create/Modify: `api/tests/kit/env.go`, `api/tests/kit/suite.go`
- Test: harness self-test.

**Interfaces:**
- Produces: `IntegrationSuite`; `kit.Main(m)`; `BuildEnv(t, home)` (real adapter+app+api over a Unix socket, `WithHomeDir(home)` isolation); `NewEnvWithHome(home)` (restart primitive = second env over same home); `Close`/`CloseCrashing`/`CloseWithoutKilling` (one `sync.Once`); WS state-watchers `WaitForState`/`WaitForListLen`.

- [ ] **Step 1: Failing test** — `BuildEnv` boots + serves over a socket under the temp home; a second `NewEnvWithHome` reopens the same home and reads persisted state.
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement** (mirror quiver `tests/kit/*`; short socket path for macOS 104-byte `sun_path`).
- [ ] **Step 4: Run — PASS** (`-tags integration`).
- [ ] **Step 5: Commit** `git commit -am "test(kit): integration harness (WithHomeDir, restart, crash variants, WS watchers)"`

---

### Task 17: Crash / recovery / rebuild / friendly-path test matrix

**Spec:** §5 (the table).

**Files:**
- Create: `api/tests/integration/crash/crash_test.go`, `api/tests/integration/lifecycle/lifecycle_test.go`, `api/tests/integration/paths/paths_test.go`
- Build tag `integration`.

- [ ] **Step 1: Write the matrix tests** (each a real test with WS-watcher assertions):
  - Graceful restart persists (no replay ran).
  - Crash mid-provision → reconcile completes/cleans.
  - Crash mid-merge (`MERGE_HEAD`) → `pr-conflicts`/aborted.
  - Provider drift while down → reconcile re-fetches.
  - Deleted + lingering worktree → boot sweep reaps (row + worktree gone).
  - Graceful drain integrity (bounded, no lost writes, no hang).
  - Lazy read-model rebuild on empty-model List.
  - Friendly worktree path (full slug on disk; case-only clash rejected).
- [ ] **Step 2: Run — expect FAIL** where behavior not yet exact; fix implementation (loop back to the owning task) until green.
- [ ] **Step 3: Run full integration suite — PASS.**
- [ ] **Step 4: Commit** `git commit -am "test: crash/recovery/rebuild/friendly-path integration matrix"`

---

### Task 18: Cleanup — delete `internal/locations` + storages tree; vet/deadcode/lint green

**Spec:** §4 (Deleted outright), decision 15.

**Files:**
- Delete: `api/internal/app/repositories/workspace/internal/locations` (package + table). **UUID worktree naming (`worktreepath.For`) is already gone — deleted in Task 3b when the provisioner was rewired onto `Derive`** (not re-deleted here). **FLAGGED-RETAINED (spec follow-up, NOT deleted this task):** the per-workspace `worktreepath.StorageDir` `.../workspaces/<wsID>/storages` tree (`usecases/terminal/metastore.go:104`) — §4 marks it deleted-outright, but §3.9 gives no replacement template for terminal-session storage under the human-readable layout (see Task 3b's storages-tree disposition). Re-rooting terminal storage requires a spec decision; it is deliberately left live with the flag rather than invented here.
- Delete: `api/internal/app/repositories/internal/serialize/keyed_mutex.go` + `keyed_mutex_test.go` — `serialize.KeyedMutex` had exactly two consumers, chat (`chat.go:66`, package deleted in Task 13) and reviewthread (`reviewthread.go:93`, writeMu usages removed in Task 12); both are gone by now, so the file is unreferenced and would fail the `deadcode` gate. **KEEP `api/internal/app/repositories/internal/serialize/aggregate_lister.go`** in the same package — workspace (Task 11) and reviewthread (Task 12) lazy Replay still type-assert `serialize.AggregateLister`.
- Modify: any remaining references.

- [ ] **Step 1: Guard test / grep** — `grep -rn "locations\.\|Registry\[\|writeMu\|KeyedMutex\|storages/\|SendWait\|event_stream.db\|per-entity\|ReconcileAll\|startRecoverySweep\|worktreepath.For\b" api/internal` returns only sanctioned hits (the one recovery `SendWait`; the flagged-retained `worktreepath.StorageDir` `storages/` hits for terminal-session storage — spec follow-up, see Files; `worktreepath.For` / UUID worktree naming must return NOTHING — already deleted in Task 3b; `KeyedMutex`/`writeMu`/`ReconcileAll`/`startRecoverySweep` must return NOTHING — `KeyedMutex`'s file is deleted this task; `ReconcileAll`/`startRecoverySweep` are fully cleared by Task 10, which deletes the `TestReconcileAll_*` block AND its two exclusive orphaned helpers `perPathSummaryGit`/`perRepoStore` (whose doc comments at `worktree_test.go:480`/`:500` also carry the string), and refreshes the two surviving-code comments at `watcher.go:226` + `worktree.go:586`). `deadcode ./...` empty. (The added `KeyedMutex` term is needed because the old `writeMu` term no longer appears in `keyed_mutex.go` — the field/usages were renamed away in Tasks 12/13 — so a `writeMu`-only guard would miss the orphaned file.)
- [ ] **Step 2: Run — FAIL** (residual refs).
- [ ] **Step 3: Implement** deletions; fix references.
- [ ] **Step 4: Run — PASS.** Full gate: `go build ./... && go vet ./... && deadcode ./... && golangci-lint run && go test -race ./internal/... && go test -tags integration ./tests/... && (cd ../web && bun tsc --noEmit && bunx prettier --check .)` — all green.
- [ ] **Step 5: Commit** `git commit -am "chore: delete internal/locations + residual per-entity paths; vet/deadcode/lint green"`

---

## Self-review notes (coverage check vs spec)

- Decisions 1–15: 1(T7), 2(T13), 3(T1), 4(T4/T9), 5(T5/T6/T12), 6(T5/T12), 7(T11), 8(T8/T9/T10), 9(T4/T9), 10(T7), 11(T15), 12(T1), 13(T3/T3b), 14(T1/T16), 15(T13/T18). ✅
- §3.5 hub enrichment (FE-regression risk): built T6 (additive), production-registered on the singleton T7. §3.5 command flow: T7 (202-ack is a flagged deliberate deviation — handlers keep returning `evt.Aggregate` for zero FE regression; `sendWithOCC` maps `ErrValidation`→422 (per §3.5, not 400), OCC-exhausted `ErrPipelineFailed`→409 Conflict (§3.5's 409/500), and `ErrQueueFull`→503). §3.6 delete ordering: T8. §3.9: derivation helper T3 (additive) + provisioner rewire onto `Derive`/`DetectClash`/`HomeLeaf` T3b (deletes UUID `For`), `workspace_paths` store T2, its three write points Create→T7 / rename→T3 / delete→T8. ReviewThread full conversion: T12. Chat live consumer: T13. Testing: T16/T17. ✅
- **Live-verification (make dev-desktop + Tauri MCP, dev-isolated home) is NOT a plan task** — it is the separate §6 verification gate run after all tasks are green (tracked outside this plan).

## Open items — RESOLVED by the plan-hardening review (2026-07-07)
- **Workspace command/mutation set (Task 4):** RESOLVED — the live set is the **12 existing** command files (`create`, `clear_branch`, `provision_in_place`, `reparent`, `resolve_conflicts`, `set_last_error`, `set_merge_strategy`, `set_parent_from_pr`, `sync_provider_state`, `sync_working_tree_state`, `touch_activity`, `update_fork_point`). No `set_status.go`; the earlier `sync_working_tree.go`/`sync_provider.go` names were wrong (real: `*_state.go`). Task 4 now audits those 12 and ADDS only `delete.go`. Task 7's concurrency test uses a real command (`TouchActivity`), not the non-existent `SetStatus`.
- **Daemon entrypoint / boot-sweep + shutdown seams (Tasks 10, 15):** RESOLVED — there is **no `app.Start`**. Boot work runs synchronously in `app.New` (`app/container.go:76-78`); process lifecycle is `internal.New`→`internal.Run`→`internal.Close` (`internal.go`); the signal source already exists at `cmd/crowbar/main.go:53`. Task 10 invokes `Sweep` from `app.New` (replacing `startRecoverySweep`, which — with its callee `ReconcileAll` — is now explicitly deleted). Task 15 adds a new `app.Container.Shutdown(ctx)` and threads it into `internal.go`'s ordered teardown (after `server.Shutdown` `:116`, before `adapter.Close` `:138`); `app` currently exposes only `Close()` and has no `ax.Shutdown`.
- **ReviewThread projector/mutex/line refs (Task 12):** RESOLVED — `writeMu` field at `reviewthread.go:93`; the `writeMu`+`SendWait` mutation block spans `:145-264` (Open→Reopen, corrected from `145-258`); combined projector at `internal/store/projections.go`.
- **Task 1 no longer deletes still-consumed adapter APIs (blocker):** RESOLVED via **ratification path (a)** — Task 1 is now purely additive (adds the per-type handles + read pools + `ReviewThreadView()`, hardens `WithHomeDir`) and deletes NOTHING from the per-entity API. Because Go has no method overloading, the new no-arg workspace handles ride temporary names (`WorkspaceEventStore()`/`WorkspaceStoreDB()`) that Task 7 promotes to `WorkspaceES()`/`WorkspaceView()`. The 3-arg `WorkspaceES(p,r,w)`/`WorkspaceView(p,r,w)` + `Registry` (still called at `workspace.go:320,324`) are deleted in Task 7; `ChatES()` + `chatES` (called at `app/container.go:39`, `repositories/container.go:59`) in Task 13. `go build ./...` stays green T1→T6; line 48 now reads "only Task 13 carries a sanctioned interim break" (Task 7 no longer breaks — the fourth review moved the `Delete`→`Send` rewrite into it, see below).
- **`reviewthread.New` caller rewiring + orphaned `db` (Task 12/13):** RESOLVED — Task 12 adds `repositories/container.go` to its Files and rewires the `reviewthread.New(...)` caller at `:63` onto `adapters.ReviewThreadView()` (dropping the shared `GlobalView` db, keeping `ReviewThreadES()` for the lazy `AggregateLister`). The `db := adapters.GlobalView()` at `:58` stays alive in Task 12 (chat still consumes it at `:59`) and is removed in Task 13 once chat is gone (fixes the false "reviewthread uses it" note + the incoming `declared and not used` error).
- **`serialize.KeyedMutex` orphaned after chat+reviewthread (Task 18):** RESOLVED — Task 18 now deletes `internal/serialize/keyed_mutex.go` + `keyed_mutex_test.go` (last consumers removed in Tasks 12/13) and KEEPS `aggregate_lister.go` (still used by workspace + reviewthread lazy Replay). Task 18's grep guard gained a `KeyedMutex` term (the old `writeMu`-only guard missed the orphaned file).
- **`workspace_paths` write lifecycle (Tasks 2/7/8, spec §3.9):** RESOLVED — all three §3.9 writers are now wired: Create → `pathsStore.Put` (Task 7), rename → `Move` (Task 3), delete → `pathsStore.Delete` in the Task 8 reactor. Task 2 cross-references them so the map can't silently stay empty.
- **`w.locations.*` retirement before `internal/locations` delete (Tasks 7/8/18):** RESOLVED — Task 7 enumerates re-homing every live `w.locations.*` site (`:301,363,379,650,671,714`) onto the store projection + `pathsStore` and strips the `locations` field/import; Delete's sites (`:650,671`) move into the Task 8 reactor, so the package is genuinely unreferenced when Task 18 deletes it.
- **Shutdown reachability of ax singletons + drain gate (Tasks 7/14/15):** RESOLVED — Task 7 retains `axWorkspace`/`axReviewThread` on `app.Container` (rather than dropping them after `repositories.New`), Task 14's `wireCallbacks` creates/holds the shared drain `WaitGroup` + drain context on `repositories.Container` with an accessor, so Task 15's `Shutdown(ctx)` can `drainWG.Wait()` and `ax.Shutdown` each singleton.
- **Per-entity test files not listed at their delete commit (Tasks 7/13, major):** RESOLVED — the tests hard-coupled to the deleted per-entity model are now enumerated where the symbols die, so no package fails to compile mid-plan. Task 7's Files list `workspace_test.go` (rewrite for the new `workspace.New` signature; drop `wsAsynxFactory`/`WithMaxOpenEntities`/`storages` assertions), `export_test.go` (delete `CachedEntityCount`/`w.entities.Len()`), and `adapter/container_test.go` (remove the `storagesDir` helper `:16` + the 3-arg `WorkspaceES/WorkspaceView` lazy-open/cached/close/reopen-after-close/mkdir-error tests). Task 13's Files add `adapter/container_test.go` to delete the `c.ChatES()` assert at `:106` (reducing `TestChatAndReviewThreadES_Global` to `ReviewThreadES` only). Task 1 stays additive and keeps these tests live (it cannot delete methods it keeps).
- **Task 12's existing reviewthread test not listed (minor):** RESOLVED — Task 12's Files now name `reviewthread/reviewthread_test.go` with an explicit rewrite for the Send+OCC / no-`writeMu` / lazy-`Replay` shape (the suite drives the synchronous `SendWait` API — `Resolve` then asserts `resolved.Status` at `:45-47` — and the pre-conversion `New(ctx, ax, es, db, broadcast)` at `:34`).
- **`entityFor`/`entityForLocation` range under-cited (Task 7, minor):** RESOLVED — corrected `workspace.go:293-315` to `:293-356` (`entityFor` doc `:293`/body `:297-313`, `entityForLocation` `:315-356`).
- **pathsStore threading unspecified + package-name collision (Tasks 2/7, minor):** RESOLVED — Task 7 now states `repositories.New` builds `pathsStore := wspaths.NewWorkspacePaths(adapters.GlobalView())` and passes it as `workspace.New`'s third arg; no new `repositories.New` param (it already takes `adapters`), so `app/container.go`'s call is unchanged for paths. The store lives in a **new package `wspaths`** (`api/internal/adapter/store/wspaths/`) — deliberately NOT named `paths`, which would shadow the `core/paths` package (`EventsAt`/`StoreAt`/`StateAt`) Task 1 imports as `paths`. Ownership stays with the adapter that owns view.db (§3.9).
- **§3.5 202-ack + `ErrQueueFull` mapping unassigned (Tasks 4/5/6/7, minor):** RESOLVED — Task 7 flags that handlers intentionally keep returning the full applied `evt.Aggregate` (a deliberate NON-implementation of §3.5's literal 202-ack, chosen to honor "zero FE regressions"), and extends `sendWithOCC`'s error contract to map the NEW `asynxmodels.ErrQueueFull` → HTTP 503 (add an `apperr.ErrUnavailable` sentinel mirroring `apperr.go:11/19/26`) via `errors.Is` — a sentinel that becomes newly-reachable because `writeMu` is deleted and one shared `axWorkspace` absorbs concurrent `Send`s under load (not a `Send`-vs-`SendWait` distinction — both route through the same `processor.sendAndWait` `ErrQueueFull` branch).
- **Cosmetic: Task 4 commit + Task 2 gorm.go cite (minor):** RESOLVED — dropped the phantom `status` verb from Task 4's commit (`create/sync/delete`; there is no `set_status` command); normalized Task 2's citation from `gorm.go:14-18` to `gorm.go:15-18` (line 14 is the `GORMStores` struct header).

## Open items — RESOLVED by the second plan-hardening review (2026-07-07)
- **Combined `projections.go` + `store.New` disposition unnamed (Tasks 5/6, major):** RESOLVED (timing **SUPERSEDED by the third review below** — the *removal* of the combined file + the `store.New` rework + the singleton registration now land in **Task 7**, not Task 5, because Tasks 5/6 must stay additive to avoid a non-sanctioned build break; the split DESIGN below still stands). The two projections split the combined `internal/store/projections.go` (its `onEvent` at `projections.go:43-52` both `saveWithRetry`s AND `broadcast`s under one `Subscribe(asynx.Topic("workspace.*"))` at `:29`, plus `onForget` at `:76-82`) into a save-only `projections/store.go` + `onForget` (Task 5) and the broadcast half `projections/hub.go` (Task 6); the combined file is REMOVED and `store.go`'s `New`/`reconcile` (`store.go:44-85`) reworked to drop the per-`aggregateID` eager reconcile — the central store projection registers ONCE on the singleton (`RegisterStore`), read-model repair lazy (Tasks 9/11) — **all executed in Task 7**. **Architecture decision stated:** the two projections live in a `projections/` **subpackage** under `store/` (spec §4 lines 126-128 `projections/store.go`/`projections/hub.go` + repo-layout `.../internal/store/internal/projections/*`) — the same store+hub split as Task 12/reviewthread, placed in a subpackage per the spec (spec wins on placement).
- **Missed `ReconcileAll` doc refs — FOUR surviving sites, not two (Task 10, major):** RESOLVED — after Task 10 deletes the `TestReconcileAll_*` block, FOUR `ReconcileAll` string sites survive in `api/internal`. Two sit on surviving code and are **refreshed**: `watcher.go:226` AND the `resyncSummary` doc comment at `usecases/worktree/worktree.go:586` (a comment on the SURVIVING `resyncSummary` fn). Two more are doc comments on test helpers used EXCLUSIVELY by the deleted tests — `perPathSummaryGit` (`worktree_test.go:480`, used only at `:1678`/`:1791`) and `perRepoStore` (`:500`, used only at `:1768`) — and are **eliminated by deleting those now-orphaned helpers** (`~:479-498` and `~:500-515`), which also clears the `deadcode` gate they would otherwise trip. All four would otherwise be hit by Task 18's `ReconcileAll` grep guard (must return NOTHING).
- **Unused `core/safego` import at Task 10 (minor):** RESOLVED — Task 10's `app/container.go` edits now remove the `core/safego` import (`:14`), whose sole user is the deleted `startRecoverySweep` (`safego.Go` at `:143`); the new synchronous `Sweep(ctx)` uses no `safego.Go`, so leaving it would break the "Task 10 ends green" rule (T10 is not a sanctioned interim-break task).
- **Premature `repositories/container.go` wiring in Task 8 (minor):** RESOLVED — Task 8 is rescoped to only PRODUCE + unit-test `RegisterDeleteReactor` (test-injected `drainWG`); the `repositories/container.go` PRODUCTION wiring via `wireCallbacks` + the shared production `drainWG` is deferred to Task 14, where both are first created. The premature "Modify: repositories/container.go" line is annotated as deferred.
- **Task 4 "move IO out" / "Chief suspects" over-scoped (minor):** RESOLVED — Task 4 reframed to CONFIRM purity (verified: `internal/commands/*` imports only `fmt`/`time`/`asynxModels`/`domain`/`gitdomain` — no `os`/`net`/`exec`/engine-git/engine-provider; `sync_provider_state.go` maps via pure helpers `nextProviderStatus`/`prStatusToWorkspace`), with the move-out reduced to a safeguard, not expected work. The "Chief suspects" list is relabeled as already-verified-pure.
- **`ErrQueueFull` rationale mechanically wrong (Task 7, minor):** RESOLVED — the justification is corrected from "reachable only under async `Send` / could never fire under `SendWait`" (both route through the same `processor.sendAndWait` `ErrQueueFull` branch in asynx v0.6.2, `processor.go:180-201`) to the accurate reason: it becomes reachable once `writeMu` is deleted and one shared `axWorkspace` absorbs concurrent `Send`s under load (today's per-entity instance + `writeMu` keeps ≤1 command in-flight per aggregate, so no shard queue fills). The actionable `ErrQueueFull`→503-via-`apperr.ErrUnavailable` mapping is unchanged. Fixed in both Task 7 Interfaces and the first-pass self-review note.

## Open items — RESOLVED by the third plan-hardening review (2026-07-07)
- **Friendly worktree path (decision 13 / §3.9) was never wired into the provisioner (blocker):** RESOLVED — the plan added a dedicated **Task 3b** that rewires the real provisioning call sites. The blocker had two roots: (1) **package collision + un-importability** — Task 3 created a SECOND package named `worktreepath` under `repositories/workspace/internal/`, colliding with the live UUID authority at `usecases/internal/worktreepath/` (`For(home, projectID, repoID, wsID)`, called at `usecases/worktree/worktree.go:192,765` and `usecases/project/project_import.go:575`) AND un-importable by the `usecases/` provisioner. **Fix:** Task 3 now EXTENDS the live `usecases/internal/worktreepath` package (one package, no collision, importable everywhere in `usecases/`), adding `Derive`/`HomeLeaf`/`DetectClash`/`Move` additively while keeping `For`. (2) **No task called `Derive`/`DetectClash`/`HomeLeaf`, and slug resolution was unassigned.** **Fix:** Task 3b resolves the repo slug (`RemoteSlug(repo)` parses `repo.RemoteURL`→`host/owner/repo`, `repo.Name` fallback; the worktree.go sites load the repo row by `RepoID` for `RemoteURL`), rewires the three `For` call sites onto `Derive`, calls `DetectClash` before create (case-only reject), uses `HomeLeaf` for net-new Crowbar-managed home worktrees ONLY — the ADOPTED repo-home keeps the user's checkout (`repo.Path`/`project.Path` at `project_import.go:478,681`) per the locked workspace-model law (§3.9's `.home` rule narrowed to managed homes; spec-author ratification pending, DECISION recorded in Task 3b), then deletes `For` in the same green commit. The existing package's OTHER consumers are accounted for: `StorageDir` (terminal storages tree, `usecases/terminal/metastore.go:104`) is the sole §4-implicated consumer — **flagged-retained** (spec gives no replacement template; re-rooting is a spec follow-up tracked in Task 18); `RepoIconPath`/`ProjectDir`/`DefaultCrowbarHome` are repo/project/home metadata, retained unchanged.
- **Tasks 5 & 6 caused a non-sanctioned build break (blocker):** RESOLVED — both are now **purely additive**, mirroring how Task 8 defers its production wiring to Task 14. Task 5 CREATES `internal/store/projections/store.go` + `RegisterStore` and unit-tests them against a test asynx, LEAVING the combined `internal/store/projections.go` and the 5-arg `store.New(ctx, view, ax, broadcast, aggregateID)` (still called at `workspace.go:340`) intact. Task 6 CREATES `projections/hub.go` + `RegisterHub` with a stub `enrich`, touching neither `container.go` nor the combined projector. The DEFERRED disposition — delete the combined `projections.go`, rework `store.New`/`reconcile` (`store.go:44-85`, drop the `aggregateID` param + eager reconcile + `registerProjections`), extract `broadcastWorkspace` into the injected `enrich`, and register `RegisterStore`+`RegisterHub` ONCE on the singleton `axWorkspace` — is folded into **Task 7** (where `axWorkspace` and workspace.go's per-entity code are built/deleted). "`go build ./...` stays green T1→T6" holds unchanged; line 48's sanctioned-break list was later narrowed to "only Task 13" by the fourth review (Task 7 was fixed to stay green — see below).
- **Task 2 vs Task 7 `workspace_paths` wiring contradiction (major):** RESOLVED — Task 2 no longer instructs registering the store in `gorm.go`'s `GORMStores` set (its `gorm.go:15-18` citation is dropped). Task 7 (and 2nd-pass self-review 580) construct `wspaths.NewWorkspacePaths(adapters.GlobalView())` locally in `repositories.New`, so a `GORMStores.WorkspacePaths` field would be unused (fails `deadcode`) and can't share the `store.Store[T,string]` shape anyway (`wspaths` exposes `Put`/`Get`/`Delete`). **Spec deviation flagged:** §3.9's "fifth `view.db` store … (`gorm.go:15-18`)" mechanism is overridden by implementation reality; the adapter-ownership DESIGN (backed by `GlobalView()`) is preserved.
- **`sendWithOCC` OCC-exhaustion disposition unspecified (minor):** RESOLVED — Task 7's contract now maps a still-failing `asynxmodels.ErrPipelineFailed` AFTER the ≤5 OCC retries to **HTTP 409 Conflict** (per §3.5's `ErrPipelineFailed→OCC retry ≤5×→409/500`; 409 chosen as an unrecoverable OCC/version collision, slotting into `libs.StatusAndMessage`'s existing 409 category at `internal/api/libs/status.go` via `errors.Is`). A `TestSendWithOCC_ErrorDisposition` test asserting 409 (exhaustion) / 422 (`ErrValidation`) / 503 (`ErrQueueFull`) was added to Task 7 Step 1.
- **Task 13 stale `router.go` citation (minor):** RESOLVED — the chat TODO is a FOUR-line block; the two `router.go:115-117` citations in Task 13 are corrected to `router.go:115-118` (line `:118` is the trailing `// /v0/projects/:p/repos/:r/workspaces/:w/chats` path comment, part of the same TODO; removing only `:115-117` would strand it).

## Open items — RESOLVED by the fourth plan-hardening review (2026-07-07)
- **Task 7↔Task 8 `Delete`/`locations` internal contradiction (blocker):** RESOLVED — Task 7 stripped the `locations` field + `entities *Registry` + `wsEntity.forget` while leaving the synchronous `Delete` method (`workspace.go:646-685`) still referencing `w.locations.Get` (`:650`), `entity.forget` (`:665`), `w.entities.Evict` (`:668`), and `w.locations.Delete` (`:671`) — a hard compile error that also made Task 7 Step 4's "PASS the concurrency test" impossible (the `-race` binary couldn't build). **Fix:** the `Delete = w.sendWithOCC(ctx, commands.Delete{ID:id})` rewrite (the `commands.Delete` type already exists from Task 4) is **moved INTO Task 7**, so the `locations` field is stripped with no dangling reference and the workspace package compiles + the concurrency test builds/passes. Task 7's Interfaces carve-out ("Delete's `w.locations.Get`(:650)+`w.locations.Delete`(:671) → move into the Task 8 delete reactor, NOT this task") is deleted — those synchronous calls are simply removed when `Delete` becomes a pure `Send`. **Task 8 is reduced to ADD only `reactors/delete.go` + tests** (registered in Task 14); it no longer modifies `workspace.go`. The "sanctioned interim compile break / commit as `wip:`" framing for Task 7 (lines 48, 307, Step 3/4/5) is dropped — the package now stays green (line 48's sanctioned-break list is narrowed to "only Task 13").
- **Repo-home `.home` derivation vs the user's real checkout (major):** RESOLVED — Task 3b/§3.9 derived the repo-home worktree as the Crowbar-managed `.../<slug>/.home/` leaf, but the live code roots the repo-home/adopted-home workspace at the user's ACTUAL clone (`project_import.go:478 WorktreePath: repo.Path`, `:681 WorktreePath: project.Path`), and the locked Crowbar workspace-model law states the repo home IS the user's real checkout (never a Crowbar-managed worktree). The old plan resolved this non-actionably ("§3.9's `.home` derivation wins unless the author overrides"). **Fix:** Task 3b now carries an explicit DECISION (RATIFIED spec author 2026-07-07; §3.9 updated to match): the **adopted repo-home/adopted-home keeps `repo.Path`/`project.Path`** per the live code + locked law; **`HomeLeaf`/`.home` is restricted to net-new Crowbar-MANAGED home worktrees only**. Recorded as a flagged §3.9 narrowing (spec preferred, but overridden by the higher-authority locked law for the adopted case); the global worktree-path constraint (line 23) and a new Step-1 test case (e) — assert adopted home persists `repo.Path`/`project.Path`, not a `.home` leaf — enforce it.
- **`wspaths.Get` not-found sentinel inverted the adapter→app layering (minor):** RESOLVED — Task 2 sited `wspaths` in the adapter layer (`api/internal/adapter/store/wspaths/`) but had `Get` return `apperr.ErrNotFound` (Step-1 test asserted `apperr.ErrNotFound`), even though no adapter package imports `app/apperr` (verified). **Fix:** Task 2 now defines a **package-local `wspaths.ErrNotFound`** (mirroring `locations.ErrNotFound`), the Step-1 test asserts `require.ErrorIs(t, err, wspaths.ErrNotFound)`, and the workspace repo (Task 7) translates `wspaths.ErrNotFound` → `apperr.ErrNotFound` at its boundary exactly as `workspace.go:307` does for `locations.ErrNotFound` (the Task 8 reactor instead treats a not-found path as an idempotent no-op, not a 404).
