## Storage & adapter container (per-entity lazy-open DBs, LRU, event_stream.db + view.db split, UUID-based filesystem layout)

### Key signatures
- `api/internal/adapter/container.go`
  - type Container struct { WorkspaceES asynxModels.Store; ChatES; AgentRunES; ReviewThreadES asynxModels.Store; DB *gormdb.DB; closers []io.Closer; lock *instanceLock } (container.go:19-27)
  - func New(opts ...Option) (*Container, error) (container.go:46-70)
  - func newLocked(homeDir string, lock *instanceLock) (*Container, error) (container.go:72-103)
  - func openEventStores(eventsPath string) ([]asynxModels.Store, []io.Closer, error) — names := []string{"workspace.db","chat.db","agent_run.db","review_thread.db"} (container.go:157-174)
  - func (c *Container) Close() error (container.go:177-199)
  - func resolveDirs / resolveDefaultDirs / resolveHomeDirs (container.go:122-155)
- `api/internal/adapter/eventstore/sqlite/event_store.go`
  - func NewEventStore(path string) (models.Store, error) (event_store.go:30-58)
  - type eventEntry struct { AggregateID string; Version int64; Data []byte } TableName()="events" (event_store.go:13-21)
  - func (s *eventStore) Append/ReadFrom/ReadRange/Count/Delete (event_store.go:60-138)
  - func (s *eventStore) Close() error — checkpoints WAL, closes handle (event_store.go:141-147)
- `api/internal/adapter/store/sqlite/sqlite.go`
  - func New[T any, K comparable](path string) (store.Store[T,K], error) (sqlite.go:23-31)
  - func OpenDB(path string) (*gorm.DB, error) (sqlite.go:36-57)
  - func NewFromDB[T any, K comparable](db *gorm.DB) (store.Store[T,K], error) (sqlite.go:60-72)
  - func (s *gormStore[T,K]) Save/Delete/FindByKey/FindAll (sqlite.go:91-129)
- `api/internal/adapter/store/store.go`
  - type Store[T any, K comparable] interface { Save; Delete; FindByKey; FindAll } (store.go:8-24)
- `api/internal/adapter/lock.go`
  - var ErrStateDirLocked (lock.go:12)
  - const lockFileName = ".lock" (lock.go:14)
  - func acquireStateLock(stateDir string) (*instanceLock, error) (lock.go:27-47)
  - func (l *instanceLock) Close() error (lock.go:50-58)
- `api/internal/adapter/lock_unix.go`
  - func lockFile(file *os.File) error (lock_unix.go:12-20)
  - func unlockFile(file *os.File) error (lock_unix.go:22-26)
- `api/internal/adapter/lock_windows.go`
  - func lockFile / unlockFile (lock_windows.go:12-37)
- `api/internal/core/paths/paths.go`
  - var mu sync.Map (paths.go:13)
  - func ensure(path string) (string, error) (paths.go:15-26)
  - func State/StateAt/Events/EventsAt/Store/StoreAt/Runs/RunsAt/Logs/LogsAt (paths.go:29-86)
- `api/internal/core/metadata/metadata.go`
  - type Paths struct { Home OsValue[string]; Events; Store; Runs; Config; Logs string } (metadata.go:36-43)
  - func GetStateDirPath() / GetStateDirPathAt(homeDir) (metadata.go:71-80)
  - func GetEventsPath/GetStorePath + ...At variants (metadata.go:82-133)
  - func defaultMetadata() — Events:"{{home}}/state/events", Store:"{{home}}/state/store" (metadata.go:147-164)
- `api/internal/core/metadata/metadata.yaml`
  - events: "{{home}}/state/events"; store: "{{home}}/state/store"
- `api/internal/app/asynx.go`
  - func newAsynx[T any](es asynxModels.Store) (asynx.Asynx[T], error) — Shards:8, QueueDepth:1000 (asynx.go:8-15)
- `api/internal/app/gorm.go`
  - type GORMStores struct { Projects; Repositories store.Store[...]; TerminalProfiles store.Store[domain.TerminalProfile,string] } (gorm.go:14-18)
  - func newGORMStores(db *gormdb.DB) (*GORMStores, error) — all via NewFromDB over the one shared db (gorm.go:20-40)
- `api/internal/app/container.go`
  - func New(ctx, engines *engine.Container, adapters *adapter.Container) (*Container, error) (container.go:34-95)
  - axWorkspace := newAsynx[domain.Workspace](adapters.WorkspaceES) ... axChat/axAgentRun/axReviewThread (container.go:39-54)
  - repos := repositories.New(adapters.DB, h, axWorkspace, axChat, axAgentRun, axReviewThread) (container.go:62)
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go`
  - func For(crowbarHome, remoteURL, workspaceID string) (string, error) (worktreepath.go:20-26)
  - func RepoDir(crowbarHome, remoteURL string) (string, error) (worktreepath.go:33-39)
  - func DefaultCrowbarHome() (string, error) (worktreepath.go:43-49)
  - func repoRelPath(rawURL string) (string, error) — parses HTTPS/SSH (worktreepath.go:53-79)
- `api/internal/app/usecases/worktree/worktree.go`
  - type CreateChildInput struct { RepoID; ProjectID; RepoPath; RemoteURL; Branch; ParentID; ParentBranch string; ForceLocked bool } (worktree.go:24-37)
  - path, err := worktreepath.For(home, in.RemoteURL, wsID) (worktree.go:121)
- `api/internal/app/usecases/container.go`
  - CrowbarHome: worktreepath.DefaultCrowbarHome (container.go:101)
  - worktree.New(repos.Workspace, engines.Git, engines.Provider, gormStores.Repositories, nowFunc, worktreepath.DefaultCrowbarHome) (container.go:103-110)
- `api/internal/app/repositories/container.go`
  - func New(db *gormdb.DB, h hub.WebSocketHub, axWorkspace asynx.Asynx[domain.Workspace], axChat, axAgentRun, axReviewThread) (*Container, error) (container.go:27-62)
  - workspace.New(axWorkspace, db, broadcastFn) (container.go:37)
- `api/internal/adapter/container_test.go`
  - TestRegression_StateDirSingleInstanceLock (container_test.go:13)
  - TestNew_BootsAllStores — asserts WorkspaceES/ChatES/AgentRunES/ReviewThreadES/DB non-nil (container_test.go:32)
- `api/internal/app/usecases/internal/worktreepath/worktreepath_test.go`
  - TestFor_HTTPSRemote / SSHRemote / EmptyRemoteURLErrors / UnrecognisedURLErrors (worktreepath_test.go:12-52)
  - TestRepoDir_HTTPS / SSH / EmptyErrors (worktreepath_test.go:54-69)

### Must change
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go` — §8 rewrite: change For to `For(crowbarHome, projectID, repoID, workspaceID string) string` returning <home>/projects/<P>/<R>/workspaces/<W>/worktree (note new /worktree suffix, no error). Add StorageDir, RepoDir(projectID,repoID), ProjectDir(projectID) helpers. Delete repoRelPath and all URL parsing. Keep DefaultCrowbarHome. This is the canonical layout source consumed by the new container.
- `api/internal/app/usecases/worktree/worktree.go` — §8: replace worktreepath.For(home, in.RemoteURL, wsID) at line 121 with worktreepath.For(home, in.ProjectID, in.RepoID, wsID) (no error). Stop relying on RemoteURL for path derivation. §1: DeleteCascade/removeOne must rm -rf <ProjectDir/RepoDir/workspaces/W> (worktree + storages + threads + terminals) atomically after git worktree remove.
- `api/internal/adapter/container.go` — §2/§9: replace the 4 fixed global event-store fields + single DB with a lazy per-entity registry. Add LRU-capped caches keyed by entity id-path for event_stream.db and view.db handles; per-path mutex (mirror paths.ensure). Provide resolver methods (WorkspaceES/WorkspaceView/RepoES/RepoView/ProjectES/ProjectView + GlobalView/GlobalES). MkdirAll the storages/ dir on first open. Evicted LRU entries Close() their handle. Keep the global .lock over the whole crowbar tree. Global state now holds state/event_stream.db + state/view.db (terminal_profiles, settings) only.
- `api/internal/adapter/eventstore/sqlite/event_store.go` — §1: filename changes from <name>.db to event_stream.db per storages dir — NewEventStore(path) is unchanged in signature but callers pass per-entity paths. §14 (deferred): no required change now, but note the lack of a created_at/sequence column blocks WS replay if that open question is later closed.
- `api/internal/adapter/store/sqlite/sqlite.go` — §2: per-entity view.db files mean callers use New[T,K](path) (own DB per entity) instead of NewFromDB[T,K](sharedDB). No change to OpenDB/New/NewFromDB themselves, but the container/app must stop co-locating Project+Repository+Workspace in one DB. Each entity's view.db migrates only its own table set.
- `api/internal/core/metadata/metadata.go` — §1: state layout becomes state/event_stream.db + state/view.db (drop the events/ and store/ subdirectories). GetStateDirPath currently derives from filepath.Dir(GetEventsPath()) — give State its own template/accessor so removing Events doesn't break it. Add a projects-root accessor (or delegate the projects tree entirely to worktreepath).
- `api/internal/core/metadata/metadata.yaml` — §1: replace events:"{{home}}/state/events" and store:"{{home}}/state/store" templates with state:"{{home}}/state" (and projects:"{{home}}/projects" if not delegated to worktreepath). The global DBs become state/event_stream.db and state/view.db, not subdir crowbar.db.
- `api/internal/core/paths/paths.go` — §9: extend with entity-scoped storage-dir resolvers (project/repo/workspace storages) reusing ensure()+sync.Map, OR have the container call worktreepath.StorageDir + os.MkdirAll directly. Drop Events()/Store() (events/store subdirs no longer exist); keep State() pointing at state/.
- `api/internal/app/asynx.go` — §9: newAsynx[T] is invoked lazily per entity (inside the container's WorkspaceES resolver path) rather than once at startup. Keep the factory signature; change WHO calls it and WHEN (per workspace/repo/project).
- `api/internal/app/gorm.go` — §2: split GORMStores. TerminalProfiles (+ settings) stays in the global state/view.db (its own New(path)). Projects and Repositories move to per-entity view.db files resolved through the container, so they can no longer be plain NewFromDB(sharedDB) singletons — they become per-id resolvers.
- `api/internal/app/container.go` — §9: stop building 4 global Asynx instances from adapters.WorkspaceES/ChatES/... and stop passing adapters.DB. Wire repositories.New to a per-entity resolver/factory from the new adapter container. ChatES/AgentRunES go away (chat out of scope; agent_run eliminated per §3 open question).
- `api/internal/app/repositories/container.go` — §5/§9: repositories.New must accept a per-entity Asynx+view resolver instead of one global Asynx[domain.Workspace] + shared *gorm.DB. The workspace aggregate repo resolves its Asynx and view.db by (projectID,repoID,wsID). Drop Chat/AgentRun/ReviewThread global Asynx params (chat out of scope; agent_run removed; review_thread becomes the per-workspace thread scope per §1).
- `api/internal/app/usecases/container.go` — §8: worktree.New call site is compatible (still passes DefaultCrowbarHome), but verify CreateChildInput no longer needs RemoteURL for paths; ProjectID/RepoID already present on the input. No other change required here for storage.

### New contracts
- // worktreepath.go — §8 (no error returns; UUID-based)
func For(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
) string
- func StorageDir(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
) string
- func RepoDir(
	crowbarHome string,
	projectID string,
	repoID string,
) string
- func RepoStorageDir(
	crowbarHome string,
	projectID string,
	repoID string,
) string
- func ProjectDir(
	crowbarHome string,
	projectID string,
) string
- func ProjectStorageDir(
	crowbarHome string,
	projectID string,
) string
- func GlobalStateDir(
	crowbarHome string,
) string
- // adapter/container.go — §9 lazy per-entity registry
const maxOpenWorkspaceDBs = 64
- type Container struct {
	crowbarHome string
	mu          sync.RWMutex
	workspaceES *lru.Cache[string, asynxModels.Store] // key: projectID/repoID/wsID
	workspaceView *lru.Cache[string, *gormdb.DB]
	repoES      *lru.Cache[string, asynxModels.Store] // key: projectID/repoID
	repoView    *lru.Cache[string, *gormdb.DB]
	projectES   *lru.Cache[string, asynxModels.Store] // key: projectID
	projectView *lru.Cache[string, *gormdb.DB]
	globalES    asynxModels.Store
	globalView  *gormdb.DB
	lock        *instanceLock
}
- func (c *Container) WorkspaceES(
	projectID string,
	repoID string,
	wsID string,
) (asynxModels.Store, error)
- func (c *Container) WorkspaceView(
	projectID string,
	repoID string,
	wsID string,
) (*gormdb.DB, error)
- func (c *Container) RepoES(
	projectID string,
	repoID string,
) (asynxModels.Store, error)
- func (c *Container) RepoView(
	projectID string,
	repoID string,
) (*gormdb.DB, error)
- func (c *Container) ProjectES(
	projectID string,
) (asynxModels.Store, error)
- func (c *Container) ProjectView(
	projectID string,
) (*gormdb.DB, error)
- func (c *Container) GlobalView() *gormdb.DB
- func (c *Container) Close() error
- // per-entity DB file names (siblings inside storages/)
const eventStreamDBName = "event_stream.db"
const viewDBName = "view.db"
- // metadata.go — §1 global layout accessors
func GetStateDirPath() string // <home>/state
func GetStateDirPathAt(
	homeDir string,
) string
func GetProjectsPath() string // <home>/projects
func GetProjectsPathAt(
	homeDir string,
) string

### Risks
- repositories.New + app.New are the load-bearing consumers of the global container fields (adapters.WorkspaceES/ChatES/AgentRunES/ReviewThreadES + adapters.DB). The storage refactor cannot land in isolation — every one of these call sites breaks at compile time. The whole aggregate-repository layer must move from 'one global Asynx[T] + one shared DB' to 'per-entity Asynx+view resolved by id'. This is the single largest coupling hazard and dwarfs the adapter file itself.
- No LRU dependency exists in go.mod (grep for lru/hashicorp returned nothing). The spec's lru.Cache[string, T] is assumed-but-absent: either add github.com/hashicorp/golang-lru/v2 or hand-roll an LRU. State this explicitly — it is a new external dep decision.
- LRU eviction closes the evicted *eventStore/*gorm.DB. If an in-flight Asynx command or projection still holds that handle, eviction races a live writer → 'database is closed' panics. Eviction must be ref-counted or guarded so an entity currently being mutated is pinned, not evicted. The spec's naive Add-then-Close sketch (§9) does not address this.
- Asynx instances are per-DB and stateful (8 shards, queue depth 1000, projection handlers). Lazily creating an Asynx[domain.Workspace] per workspace means projection wiring (the broadcast callback in repositories.New) must be re-registered on every lazy open — and torn down on eviction. Re-opening an evicted workspace must rebuild its full Asynx+projection graph, not just the raw DB handle.
- worktreepath.For currently returns (path, error) and is consumed at worktree.go:121 with error handling; the §8 signature drops the error. The two-value→one-value change ripples through the call site and through usecases.container wiring of DefaultCrowbarHome.
- worktreepath_test.go (10 tests) and adapter/container_test.go (TestNew_BootsAllStores) assert the OLD contracts (URL-derived paths, 4 global stores). They WILL fail to compile after the rewrite and must be rewritten in lockstep, not deleted-and-forgotten — they are part of the §13 coverage gate (≥95%, must not regress).
- GetStateDirPath() = filepath.Dir(GetEventsPath()) couples the state-dir to the events template. Removing the events template (it merges into state/event_stream.db) silently changes GetStateDirPath's value unless State gets its own accessor. The single-instance lock writes <stateDir>/.lock, so a wrong stateDir means the lock and the global DBs land in different places.
- Cascade delete semantics change: §1 says deleting a workspace is rm -rf of the whole <...>/workspaces/<W> tree (worktree + storages + threads + terminals). The current removeOne does git worktree remove + branch delete + workspace.Delete (read-model row) but does NOT rm the storages/threads/terminals dirs. Stale event_stream.db/view.db dirs would leak. And the deleting entity's own event_stream.db cannot be open (LRU-cached) at rm time or the unlink races an open handle on some platforms.
- ChatES and AgentRunES are removed by scope (§3: agent_run eliminated, chat out of scope). But repositories.New, app.New, RegisterAgentRunProjection, RecoverOrphans, and ReconcileChats all reference these aggregates. Ripping out agent_run/chat is a prerequisite of the storage split, not an independent cleanup — partial removal leaves orphaned Asynx wiring that won't compile.
- Pre-production wipe (§Open-Q1): old layout (state/events/*.db, state/store/crowbar.db) is abandoned with NO migration. Any dev with existing ~/.crowbar/state must wipe it; the daemon should detect the old layout (or version bump) and refuse/clear rather than silently half-open. No version-gate code exists today.
- Per-entity SetMaxOpenConns(1) WAL DBs: with 64 workspaces × (event_stream.db + view.db) = up to 128 open SQLite connections plus WAL/SHM files. The LRU cap is per-registry (workspaceES) — the spec's maxOpenWorkspaceDBs=64 must be applied consistently across ALL registries or the fd count multiplies past the cap.

### Test targets
- api/internal/app/usecases/internal/worktreepath/worktreepath_test.go — REPLACE all URL-based cases. New: TestFor_UUIDPath (asserts <home>/projects/<P>/<R>/workspaces/<W>/worktree), TestFor_Deterministic, TestStorageDir, TestRepoDir, TestRepoStorageDir, TestProjectDir, TestProjectStorageDir, TestGlobalStateDir. No error returns to test.
- api/internal/app/usecases/internal/worktreepath/worktreepath_bench_test.go — BenchmarkFor, BenchmarkStorageDir (path construction is a §13 perf-sensitive path).
- api/internal/adapter/container_test.go — REWRITE. TestWorkspaceES_LazyOpenCreatesDBFile (asserts storages/event_stream.db created on first call, dir MkdirAll'd), TestWorkspaceES_ReturnsCachedHandleSecondCall, TestWorkspaceView_LazyOpen, TestRepoES/RepoView/ProjectES/ProjectView lazy-open, TestLRUEvictionClosesEvictedHandle (exceed maxOpenWorkspaceDBs+1, assert evicted handle is closed and a re-open succeeds), TestConcurrentWorkspaceES_NoDoubleOpen (parallel goroutines on same key get one handle — verifies per-path mutex), TestRegression_StateDirSingleInstanceLock (KEEP, still valid against the global lock), TestGlobalView_HoldsProfilesAndSettings.
- api/internal/adapter/container_bench_test.go — BenchmarkWorkspaceES_CacheHit, BenchmarkWorkspaceES_LazyOpenMiss (DB registry lookup is a §13 perf path).
- api/internal/adapter/eventstore/sqlite/event_store_test.go — add TestNewEventStore_CreatesEventStreamDBAtPath confirming the renamed file convention works under a per-entity storages dir (existing append/read tests stay).
- api/tests/integration/storage/storage_layout_test.go (build tag integration) — TestRegression_WorkspaceCreateWritesEntityScopedDBs: POST .../workspaces → 202, then assert on disk that projects/<P>/<R>/workspaces/<W>/storages/event_stream.db + view.db + worktree/ exist. Synchronize via WS WorkspaceDTO arrival (block on WS message with context deadline) — NO time.Sleep. TestRegression_WorkspaceDeleteRemovesEntireDir: DELETE .../workspaces/<W> → 202 + WS WorkspaceDTO{status:"deleted"}, then assert the whole workspaces/<W> dir is gone (worktree+storages+threads+terminals).
- api/tests/integration/storage/lazy_open_test.go (integration) — TestRegression_SecondDaemonOnSameHomeFailsLock (single-instance lock honored under new layout); TestRegression_ProjectDeleteRemovesProjectDir (DELETE project → projects/<P> gone).
- api/internal/app/usecases/worktree/worktree_test.go — update CreateChild assertions to expect UUID worktree paths (projectID/repoID/wsID, /worktree suffix) and verify DeleteCascade rm -rf's the per-workspace storages dir. Use SendWait / projection-complete synchronization (Asynx exposes Send vs SendWait — SendWait blocks until projections complete), never time.Sleep.

---

## worktreepath + worktree usecase + git worktree/branch engine

### Key signatures
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go`
  - worktreepath.go:20  func For(crowbarHome, remoteURL, workspaceID string) (string, error)
  - worktreepath.go:33  func RepoDir(crowbarHome, remoteURL string) (string, error)
  - worktreepath.go:43  func DefaultCrowbarHome() (string, error)
  - worktreepath.go:53  func repoRelPath(rawURL string) (string, error)  // SSH+HTTPS URL parser, unexported
- `api/internal/app/usecases/worktree/worktree.go`
  - worktree.go:24  type CreateChildInput struct { RepoID, ProjectID, RepoPath, RemoteURL, Branch, ParentID, ParentBranch string; ForceLocked bool }
  - worktree.go:43  type Usecase interface { CreateChild; MergeIntoParent; Reparent; DeleteCascade }
  - worktree.go:76  func New(workspaces workspace.Workspace, git enginegit.Engine, provider engineprovider.Engine, repos store.Store[domain.Repository,string], now func() time.Time, crowbarHome func() (string, error)) Usecase
  - worktree.go:94  func (u *worktreeUsecase) CreateChild(ctx, in CreateChildInput) (domain.Workspace, error)
  - worktree.go:121  path, err := worktreepath.For(home, in.RemoteURL, wsID)
  - worktree.go:125  startSha, err := u.git.WorktreeAddBranch(ctx, in.RepoPath, path, in.Branch, in.ParentBranch)
  - worktree.go:148  func (u *worktreeUsecase) adoptMainWorktree(ctx, in) (domain.Workspace, error)
  - worktree.go:172  func (u *worktreeUsecase) resolveLocked(ctx, repoPath, branch string) (bool, error)
  - worktree.go:416  func (u *worktreeUsecase) removeOne(ctx, ws) error  // git WorktreeRemove + ForceDeleteBranch + workspaces.Delete
- `api/internal/app/usecases/worktree/merge_result.go`
  - merge_result.go:8  type MergeResult struct { ConflictsPending bool `json:"conflictsPending"`; ParentTipSha string `json:"parentTipSha"` }
- `api/internal/app/usecases/worktree/worktree_errors.go`
  - worktree_errors.go:7   ErrParentLocked
  - worktree_errors.go:12  ErrRebaseNonLeaf
  - worktree_errors.go:16  ErrChildHasChildren
  - worktree_errors.go:20  ErrNewParentLocked
  - worktree_errors.go:26  ErrWorkspaceLocked
- `api/internal/app/usecases/internal/cascade/cascade.go`
  - cascade.go:10  type Node struct { ID, Parent string; Locked bool }
  - cascade.go:20  func Plan(rootID string, all []Node) []string
- `api/internal/app/usecases/internal/discover/discover.go`
  - discover.go:17  func Repos(root string, maxDepth int) ([]string, error)
  - discover.go:52  visit(): only a .git DIRECTORY marks a repo; .git FILE (gitdir pointer) is skipped
- `api/internal/app/usecases/internal/defaultbranch/defaultbranch.go`
  - defaultbranch.go:9   type RefRunner func(args ...string) (string, bool)
  - defaultbranch.go:16  func Resolve(runner RefRunner, configList []string) string
- `api/internal/engine/git/worktree.go`
  - worktree.go:10  func (e *engine) WorktreeAdd(ctx, repoPath, worktreePath, branch string) error  // `worktree add <path> <branch>` — checks out EXISTING branch
  - worktree.go:21  func (e *engine) WorktreeRemove(ctx, repoPath, worktreePath string) error  // `worktree remove --force`
  - worktree.go:31  func (e *engine) WorktreeList(ctx, repoPath string) ([]WorktreeEntry, error)
  - worktree.go:42  func (e *engine) RebaseOnto(ctx, repoPath, newTip, forkPoint, branch string) error
  - worktree.go:54  func (e *engine) MergeFFOnly(ctx, repoPath, branch string) error
- `api/internal/engine/git/worktree_add_branch.go`
  - worktree_add_branch.go:15  func (e *engine) WorktreeAddBranch(ctx, repoPath, worktreePath, branch, startPoint string) (string, error)  // `worktree add <path> -b <branch> <startSha>`
  - worktree_add_branch.go:35  func (e *engine) revParse(ctx, repoPath, rev string) (string, error)  // unexported
- `api/internal/engine/git/merge_base.go`
  - merge_base.go:10  func (e *engine) MergeBase(ctx, repoPath, a, b string) (string, error)
- `api/internal/engine/git/rebase_then_ff_merge.go`
  - rebase_then_ff_merge.go:11  func (e *engine) RebaseThenFFMerge(ctx, childWorktree, parentBranch, parentWorktree, childBranch string) error
- `api/internal/engine/git/ops.go`
  - ops.go:16  func resolveGitDir(repoPath string) string  // `rev-parse --git-dir`
  - ops.go:33  func detectInProgressOp(repoPath string) string
  - ops.go:59  func (e *engine) operationContinue(ctx, repoPath string) error
  - ops.go:75  func (e *engine) operationAbort(ctx, repoPath string) error
- `api/internal/engine/git/rev_parse.go`
  - rev_parse.go:8  func (e *engine) RevParse(ctx, repoPath, rev string) (string, error)
- `api/internal/engine/git/internal/branches/branches.go`
  - branches.go:22  func List(ctx, repoPath string) ([]gitdomain.Branch, error)  // `branch -a --format=...`
  - branches.go:36  func Create(ctx, repoPath, name, source string, switchTo bool) error
  - branches.go:72  func Delete(ctx, repoPath, name string) error
  - branches.go:83  func ForceDelete(ctx, repoPath, name string) error
  - branches.go:93  func Switch(ctx, repoPath, name string) error
  - branches.go:19  var gitRunner = exec.Git  // overridable in tests via export_test.go
- `api/internal/api/v0/endpoints/workspaces/handlers/crud.go`
  - crud.go:19  type createRequest struct { RepoID, Branch, ParentID string; Locked bool }
  - crud.go:28  func (h *Handlers) Create(c *gin.Context)  // returns 201 + created.ID synchronously
  - crud.go:59  func (h *Handlers) buildCreateInput(ctx, body, locked) (worktree.CreateChildInput, error)  // sets RemoteURL: repo.RemoteURL
  - crud.go:104  func (h *Handlers) Delete(c *gin.Context)  // returns 200

### Must change
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go` — Spec §8 + §1: Rewrite For() to the UUID signature `For(crowbarHome, projectID, repoID, workspaceID string) string` returning <crowbarHome>/projects/<P>/<R>/workspaces/<W>/worktree (note the trailing 'worktree' segment) with NO error return and NO url parsing. Change RepoDir to `RepoDir(crowbarHome, projectID, repoID string) string`. Add `StorageDir(crowbarHome, projectID, repoID, workspaceID string) string` → .../workspaces/<W>/storages and `ProjectDir(crowbarHome, projectID string) string` → projects/<P> (per §8/§9 — StorageDir is consumed by the adapter Container). Delete repoRelPath and the net/url, strings, fmt imports it needed. Keep DefaultCrowbarHome unchanged.
- `api/internal/app/usecases/worktree/worktree.go` — Spec §8: replace the `path, err := worktreepath.For(home, in.RemoteURL, wsID)` (line 121-124) call with the new errorless 4-arg form `worktreepath.For(home, in.ProjectID, in.RepoID, wsID)` and drop the error handling. Spec §3: split the single WorktreeAddBranch branch (line 125) into a remote-existence decision — if the requested branch already exists on the remote, fetch it and call git.WorktreeAdd (checkout existing branch); if not, call git.WorktreeAddBranch (create from ParentBranch). This decision MUST live in the usecase layer (CreateChild), driven by a new injected remote-branch-exists capability, not in the handler. Spec §8: remove the RemoteURL field from CreateChildInput (line 28) since path no longer needs it. Spec §5/§10: if Locked is migrated to a Status enum, update resolveLocked / Create wiring and the cascade Node mapping (nodesFrom) to derive Locked from Status instead of the bool.
- `api/internal/api/v0/endpoints/workspaces/handlers/crud.go` — Spec §3/§4: move this handler to the hierarchical route POST /v0/projects/:projectId/repos/:repoId/workspaces, validate synchronously, return 202 (empty body) and run CreateChild in a background goroutine that broadcasts the WorkspaceDTO via Broadcaster[WorkspaceDTO] on completion (success → status transition; failure → LastError set). Spec §8: stop setting CreateChildInput.RemoteURL (remove line 82); pass ProjectID/RepoID instead (already present via repo row). The remote-branch-exists vs create-local decision is delegated to the usecase, so the handler only validates input shape, repo existence, branch-name conflict, and parent state. Delete should likewise become 202 emitting WorkspaceDTO{status:"deleted"}.
- `api/internal/engine/git/internal/branches/branches.go` — Spec §3 requires resolving whether a branch exists ON THE REMOTE before deciding checkout-vs-create. branches.List only reflects already-fetched remote-tracking refs, so this subsystem needs a live remote primitive. Either add a `RemoteBranchExists(ctx, repoPath, branch string) (bool, error)` (git ls-remote --heads origin <branch>) here and expose it on the git Engine, OR promote provider.branchHasUpstream to the provider Engine interface. No change to existing List/Create/ForceDelete behavior; ForceDelete remains the teardown primitive used by removeOne.
- `api/internal/app/usecases/internal/worktreepath/worktreepath_test.go` — Spec §8: rewrite every test — the URL-based cases (HTTPS/SSH/empty/unrecognised, RepoDir(url)) no longer apply. Replace with UUID-path assertions for For/RepoDir/StorageDir/ProjectDir including the trailing 'worktree'/'storages' segments, determinism, and divergence by wsID. Remove require.NoError on For (no error returned now).

### New contracts
- // worktreepath.go (spec §8)
func For(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
) string
- func StorageDir(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
) string
- func RepoDir(
	crowbarHome string,
	projectID string,
	repoID string,
) string
- func ProjectDir(
	crowbarHome string,
	projectID string,
) string
- // For returns <crowbarHome>/projects/<projectID>/<repoID>/workspaces/<workspaceID>/worktree
// StorageDir returns .../workspaces/<workspaceID>/storages
// RepoDir returns <crowbarHome>/projects/<projectID>/<repoID>
// ProjectDir returns <crowbarHome>/projects/<projectID>
- // CreateChildInput after RemoteURL removal (spec §8)
type CreateChildInput struct {
	RepoID       string
	ProjectID    string
	RepoPath     string
	Branch       string
	ParentID     string
	ParentBranch string
	ForceLocked  bool
}
- // New remote-existence capability the usecase needs for spec §3 checkout-vs-create.
// Option A — on the git Engine (api/internal/engine/git):
RemoteBranchExists(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error)
- // CreateChild remote-vs-local decision (spec §3), in worktreeUsecase.CreateChild:
// exists, err := u.git.RemoteBranchExists(ctx, in.RepoPath, in.Branch)
// if exists { /* git fetch origin <branch>; startSha := RevParse(origin/<branch>); WorktreeAdd checkout */ }
// else      { startSha, _ := u.git.WorktreeAddBranch(ctx, in.RepoPath, path, in.Branch, in.ParentBranch) }
- // Hierarchical workspace routes replacing flat /v0/workspaces (spec §3)
POST   /v0/projects/:projectId/repos/:repoId/workspaces            // 202, body {branch:string, parentId?:string}
DELETE /v0/projects/:projectId/repos/:repoId/workspaces/:wsId      // 202, emits WorkspaceDTO{status:"deleted"}
- // New domain status constants the spec §5/§10 assume but that DO NOT yet exist:
const (
	WorkspaceStatusLocked  WorkspaceStatus = "locked"
	WorkspaceStatusDeleted WorkspaceStatus = "deleted"
	WorkspaceStatusPRConflicts WorkspaceStatus = "pr-conflicts"
)
- // Merge eligibility helper (spec §10) — belongs in the workspace usecase layer
func MergeEligibilityFor(
	ws domain.Workspace,
	siblings []domain.Workspace,
) (canMerge bool, parentBranch string)

### Risks
- SPEC GAP — missing status constants: domain/workspace_status.go defines ONLY new/pr-open/pr-merged/pr-closed (workspace_status.go:7-10). Spec §5 §10 reference WorkspaceStatusLocked, WorkspaceStatusDeleted, and pr-conflicts which DO NOT EXIST. MergeEligibilityFor as written in spec §10 will not compile until these are added. Adding 'locked'/'deleted' as Status while the codebase still uses domain.Workspace.Locked bool (workspace.go:20) and cascade.Node.Locked / nodesFrom() is a dual-source-of-truth hazard — the cascade planner, merge guards (ErrParentLocked), and resolveLocked all key off the bool today.
- SPEC GAP — no live remote-branch primitive: the spec §3 'API resolves internally whether branch exists on remote (checkout) or not' has no exported symbol to call. branches.List (branches.go:22) only sees already-fetched remote-tracking refs; the only live check is provider.branchHasUpstream (github.go:183, gitlab.go) which is UNEXPORTED and not on the provider Engine interface. A new RemoteBranchExists (git ls-remote) must be added and wired through the git Engine interface (git.go) AND the worktree.New constructor — touching the shared Engine interface used by many usecases.
- Checkout path needs a fetch + a different worktree-add: WorktreeAdd (worktree.go:10) checks out an EXISTING local-or-remote-tracking branch but does NOT fetch. To check out a branch that exists on the remote but not locally, CreateChild must `git fetch origin <branch>` first (no Fetch-for-ref primitive currently scoped to a single ref) then `worktree add <path> -b <branch> --track origin/<branch>` or `worktree add <path> origin/<branch>`. The forkPointSha semantics differ between the create-local path (startSha returned by WorktreeAddBranch) and the checkout path (must RevParse the resolved ref) — getting this wrong corrupts merge/reparent forkPoint math (finalizeMerge/replayAndReparent at worktree.go:290,360).
- Worktree path now has a 'worktree/' trailing segment (spec §1 §8). Anything that previously treated the workspace dir AS the worktree (e.g. WorktreeRemove given ws.WorktreePath, removeOne at worktree.go:424) must store and pass the .../worktree/ path; the rm -rf delete (spec §1) must target the PARENT .../workspaces/<W>/ to also remove storages/threads/terminals. Mixing the two paths risks leaving orphaned storages dirs or, worse, rm-rf-ing the wrong level.
- discover.Repos (discover.go) must keep skipping the new worktree/ checkouts (their .git is a pointer FILE). This already works, but if a future change makes a worktree a full checkout it would be re-imported as a repo. Keep the .git-FILE skip invariant.
- Broadcaster snapshot race (spec §4 §5): CreateChild now runs in a background goroutine after a 202. If the WS Broadcaster[WorkspaceDTO] snapshot is taken before the goroutine's Asynx command commits, the client could see status:new with no follow-up. The status:new → ready transition must be a single Asynx-committed push, and tests must block on the WS frame (context deadline), never time.Sleep.
- container.go (line 103-110) constructs worktree.New with crowbarHome func and the git/provider engines. Adding RemoteBranchExists to the git Engine and removing RemoteURL changes both the New wiring and the CreateChildInput built in crud.go:78 — these are compiled together; a partial migration breaks the build across handler+usecase+container simultaneously.
- worktreepath.For losing its error return is a signature breakage propagated to its single non-test caller (worktree.go:121) and to every URL-based test (worktreepath_test.go). project_import.go currently derives RemoteURL (project_import.go:229) and repos handler stores it; those stay for provider/icon use but must no longer feed worktree path derivation.

### Test targets
- api/internal/app/usecases/internal/worktreepath/worktreepath_test.go — rewrite: TestFor_UUIDPath (asserts projects/<P>/<R>/workspaces/<W>/worktree), TestFor_Deterministic, TestFor_DivergesByWorkspace, TestStorageDir_Path (.../storages), TestRepoDir_Path, TestProjectDir_Path, TestFor_NoErrorReturn. Delete all URL-based cases (HTTPS/SSH/empty/unrecognised).
- api/internal/app/usecases/internal/worktreepath/worktreepath_bench_test.go — NEW: BenchmarkFor (spec §13 path construction is a perf-sensitive path).
- api/internal/app/usecases/worktree/worktree_test.go — extend the fakeWorkspace/fake-git harness: TestCreateChild_RemoteBranchExists_ChecksOut (git.RemoteBranchExists→true ⇒ WorktreeAdd called, WorktreeAddBranch NOT called, forkPoint from resolved ref), TestCreateChild_RemoteBranchAbsent_CreatesLocal (→false ⇒ WorktreeAddBranch called from ParentBranch), TestCreateChild_UsesUUIDPathNotRemoteURL (assert worktreepath.For receives projectID/repoID, RemoteURL field gone), TestCreateChild_AdoptMainWorktreeUnchanged, TestMergeEligibilityFor_* (parent locked/deleted/missing/eligible per spec §10).
- api/internal/app/usecases/worktree/worktree_bench_test.go — extend/add: BenchmarkMergeEligibilityFor sibling scan (spec §13).
- api/internal/engine/git/worktree_test.go (or new branches/remote test) — TestRemoteBranchExists_True/False against a real temp repo+bare remote (kit fixtures), and TestWorktreeAdd_CheckoutExistingBranch verifying the checkout path produces the right HEAD.
- api/tests/ (build tag integration, TestRegression_*): TestRegression_CreateWorkspace_RemoteBranchExists_Checkout (POST .../workspaces with a branch pushed to the bare remote → 202 → WS WorkspaceDTO with that branch, no new local branch created), TestRegression_CreateWorkspace_RemoteBranchAbsent_CreateFromParent (epoch/first-pr style → 202 → WS WorkspaceDTO{status:new}→ready), TestRegression_DeleteWorkspace_RemovesWorktreeAndStorages (DELETE → 202 → WS WorkspaceDTO{status:deleted}; assert .../workspaces/<W>/ incl storages is gone), TestRegression_WorktreePathIsUUIDBased (worktree on disk at projects/<P>/<R>/workspaces/<W>/worktree). All WS assertions block on a context-deadline message read — NO time.Sleep; synchronise via the Asynx test API / kit WS waiter.

---

## HTTP + WebSocket route table: flat /v0 endpoints → hierarchical re-nesting under /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/... (spec §3), including dual-serve usage, param renaming, agentrun removal, chat deferral, and the worktreepath/storage-layout dependencies that the route handlers consume.

### Key signatures
- `api/internal/api/v0/router.go`
  - func (c *Container) Register(rg *gin.RouterGroup) — router.go:24
  - health.Register(rg) — :33
  - projects.Register(rg, Project, ProjectImport, ProjectDelete) — :35
  - repos.Register(rg, GORM.Repositories, eng.Provider, Repositories.Workspace) — :41
  - workspaces.Register(rg, Workspace, Worktree, GORM.Repositories, c.workspaces.Handle, ws.DualServe) — :47
  - git.Register(rg, Git, c.git.Handle, ws.DualServe) — :68
  - terminal.Register(rg, eng.Terminal, GORM.TerminalProfiles, Repositories.Workspace) — :74
  - editor.Register(rg, eng.LSP, eng.Git, Repositories.Workspace, c.lsp.Handle) — :94
  - agentrun.Register(rg, Repositories.AgentRun) — :101
  - chats.Register(...) — :55
- `api/internal/api/v0/container.go`
  - type Container struct { workspaces *ws.Broadcaster[domain.Workspace]; git ...; files ...; lsp ...; chats ...; chatStream ... } — container.go:27
  - func New(appContainer, engContainer) *Container — :49
  - func workspacesDef(appContainer) ws.StreamDef[domain.Workspace] — :145 (Filters: projectId, repoId via ExactMatch)
  - func gitDef(...) — :178 (Namespace=e.WsID, Filter wsId)
  - func scopeWsID(c *gin.Context) string — :101 (param wsId else query wsId)
  - PushWorkspace/PushGit/PushFile/PushChat — :111-143
- `api/internal/api/v0/middleware.go`
  - func rejectEmptyPathParams() gin.HandlerFunc — middleware.go:20
- `api/internal/api/container.go`
  - func New(appContainer, engContainer, staticFS) (*Container, error) — container.go:25
  - v0Container.Register(router.Group("/v0")) — :37
- `api/internal/api/v0/endpoints/projects/routes.go`
  - func Register(rg, reader ListGetter, importer Importer, deleter Deleter) — routes.go:13
  - rg.GET/POST "/projects"; rg.GET/DELETE "/projects/:id" — :20-23
- `api/internal/api/v0/endpoints/projects/handlers/projects.go`
  - func (h *Handlers) Detail — projects.go:101 (c.Param("id"))
  - func (h *Handlers) Import — :115 (WriteMutationOK 201)
  - func (h *Handlers) Delete — :152 (c.Param("id"), WriteMutationOK 200)
- `api/internal/api/v0/endpoints/repos/routes.go`
  - func Register(rg, store Store, prov BranchProviderEngine, wsReader WorkspaceReader) — routes.go:12
  - rg.POST/GET "/repos"; rg.GET "/repos/:id"; icon routes :22-26; rg.GET "/repos/:id/branches" :27
- `api/internal/api/v0/endpoints/repos/handlers/repos.go`
  - func (h *Handlers) Create — repos.go:159 (body {id,projectId,name,path,defaultBranch}; WriteQueryWithStatus 201 RepoDTOFrom)
  - func (h *Handlers) Icon — :223 (c.Param("id"); HTTPS redirect or local file serve)
  - func (h *Handlers) Branches — :269
  - No DeleteRepo handler present
- `api/internal/api/v0/endpoints/workspaces/routes.go`
  - func Register(rg, reader Reader, hierarchy Hierarchy, repos Repos, wsHandle gin.HandlerFunc, dispatch func) — routes.go:18
  - rg.GET "/workspaces" dispatch(h.List, wsHandle) — :27
  - rg.GET/DELETE "/workspaces/:wsId"; POST "/workspaces"; sync/merge/reparent :28-33
  - rg.GET "/ws/workspaces" wsHandle — :34
- `api/internal/api/v0/endpoints/workspaces/handlers/crud.go`
  - type createRequest struct {RepoID,Branch,ParentID,Locked} — crud.go:19
  - func (h *Handlers) Create — :28 (WriteMutationOK 201)
  - func (h *Handlers) buildCreateInput — :59 (repos.FindByKey, sets RemoteURL)
  - func (h *Handlers) Delete — :104 (c.Param("wsId"), 200)
- `api/internal/api/v0/endpoints/workspaces/handlers/list.go`
  - func (h *Handlers) List — list.go:14 (c.Query projectId/repoId)
  - func (h *Handlers) Detail — :32 (c.Param("wsId"))
  - func filterWorkspaces — :44
- `api/internal/api/v0/endpoints/git/routes.go`
  - func Register(rg, gitSvc Git, gitWS gin.HandlerFunc, dispatch func) — routes.go:18
  - rg.GET "/workspaces/:wsId/git/status" dispatch(h.Status, gitWS) — :26
  - 27 git routes total :26-58; rg.GET "/ws/git" :60
- `api/internal/api/v0/endpoints/files/routes.go`
  - func Register(rg, files Files, filesWS gin.HandlerFunc) — routes.go:13
  - rg.GET "/workspaces/:wsId/files/content"|/tree; PUT content; POST/PATCH/DELETE /files :19-24
  - rg.GET "/ws/files" filesWS :25
- `api/internal/api/v0/endpoints/editor/routes.go`
  - func Register(rg, lsp LSPEngine, git GitEngine, wsReader WorkspaceReader, lspWSHandle gin.HandlerFunc) — routes.go:19
  - rg.GET "/workspaces/:wsId/blame" :27; 11 lsp routes :28-38; rg.GET "/ws/lsp" :39
- `api/internal/api/v0/endpoints/terminal/routes.go`
  - func Register(rg, termEng TerminalEngine, profileStore ProfileStore, wsReader WorkspaceReader) — routes.go:12
  - rg.POST "/workspaces/:wsId/terminals" h.CreateSession :20; rg.DELETE "/terminals/:sessionId" :21
  - profiles CRUD :23-27; rg.GET "/ws/terminals/:sessionId" h.WS :29
- `api/internal/api/v0/endpoints/search/routes.go`
  - func Register(rg, searchEng SearchEngine, wsReader WorkspaceReader) — routes.go:11
  - rg.POST "/workspaces/:wsId/search"|/search/replace :17-18
- `api/internal/api/v0/endpoints/provider/routes.go`
  - func Register(rg, provEng ProviderEngine, wsReader WorkspaceReader) — routes.go:12
  - rg.GET "/workspaces/:wsId/provider" :18; rg.GET "/repos/:id/protected-branches" :19
- `api/internal/api/v0/endpoints/review/routes.go`
  - func Register(rg, reviewUsecase ReviewUsecase) — routes.go:14
  - rg.GET/PATCH "/workspaces/:wsId/review" :19-20
  - rg.POST "/workspaces/:wsId/review/threads"; reply; PATCH .../threads/:id :21-23
- `api/internal/api/v0/endpoints/chats/routes.go`
  - func Register(rg, chatUsecase, chatRepo, wsReader, chatsWS, chatStreamWS) — routes.go:12
  - rg.POST/GET "/workspaces/:wsId/chats"; fork/rename/delete; /ws/chats; /ws/chats/:chatId/stream :22-28
- `api/internal/api/v0/endpoints/agentrun/routes.go`
  - func Register(rg, repo AgentRunRepo) — routes.go:13
  - rg.POST "/workspaces/:wsId/runs"; /runs/running; /runs/:id/* :16-21
- `api/internal/api/v0/endpoints/system/routes.go`
  - func Register(rg) — routes.go:16; rg.GET "/system/prerequisites" :21
- `api/internal/api/v0/endpoints/health/routes.go`
  - func Register(rg) — routes.go:11; rg.GET "/health" :14
- `api/internal/api/v0/ws/dual_serve.go`
  - func DualServe(rest, wsHandler gin.HandlerFunc) gin.HandlerFunc — dual_serve.go:14 (websocket.IsWebSocketUpgrade)
- `api/internal/api/v0/ws/dispatch.go`
  - func Dispatch[T any](b *Broadcaster[T], snapshot func(*gin.Context)(any,error)) gin.HandlerFunc — dispatch.go:13
- `api/internal/api/v0/ws/filter.go`
  - func BuildPredicate[T any](c, def StreamDef[T]) func(T) bool — filter.go:36
  - func resolveFilterValue[T any](c, f FilterDef[T]) string — :69 (param→query→default)
  - ExactMatch :10; GlobMatch :18
- `api/internal/api/v0/ws/broadcaster.go`
  - type Broadcaster[T any] struct — broadcaster.go:16
  - func (b *Broadcaster[T]) Handle(c) — :59 (register→snapshotFor→onSubscribe→writePump→readPump→remove→onUnsubscribe)
  - func (b *Broadcaster[T]) Push(event T) — :165
- `api/internal/api/v0/route_audit_test.go`
  - func specRoutes() []string — route_audit_test.go:34
  - func extraRoutes() []string — :152
  - TestRouteAudit_AllSpecRoutesRegistered — :180
  - TestRouteAudit_DualServe_RestMode — :199 (paths /v0/workspaces, /v0/workspaces/w1/git/status)
  - TestRouteAudit_DualServe_WsMode — :227 (/v0/ws/workspaces)
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go`
  - func For(crowbarHome, remoteURL, workspaceID string) (string, error) — worktreepath.go:20
  - func RepoDir(crowbarHome, remoteURL string) (string, error) — :33
  - func repoRelPath(rawURL) (string,error) — :53
- `api/internal/adapter/container.go`
  - type Container struct {WorkspaceES, ChatES, AgentRunES, ReviewThreadES asynxModels.Store; DB *gormdb.DB} — container.go:19
  - func newLocked(homeDir, lock) (*Container, error) — :72
  - openEventStores(eventsPath) — :81

### Must change
- `api/internal/api/v0/router.go` — Per spec §3: stop mounting all endpoints flat on the single /v0 group. Build nested gin.RouterGroup chain projects := rg.Group("/projects"); projectScoped := projects.Group("/:projectId"); repos := projectScoped.Group("/repos"); repoScoped := repos.Group("/:repoId"); workspaces := repoScoped.Group("/workspaces"); wsScoped := workspaces.Group("/:wsId"). Pass the correct sub-group to each endpoint's Register. Mount health/system/settings on the top-level rg. Remove agentrun.Register entirely (spec §12 Open-Q3). Pass ws.DualServe to projects, repos, workspaces, terminal, review(threads). Add provider-poll lifecycle wiring (§11).
- `api/internal/api/v0/endpoints/projects/routes.go` — Per spec §3: GET/WS /projects dual-served via DualServe(h.List, projectsWS) where projectsWS is Broadcaster[ProjectDTO].Handle; GET/WS /projects/:projectId dual-served; POST /projects → 202; DELETE /projects/:projectId → 202. Add the Broadcaster handle + dispatch params to Register signature.
- `api/internal/api/v0/endpoints/projects/handlers/projects.go` — Per spec §4: Import and Delete return 202 (empty body) after synchronous validation; perform the actual work in a background goroutine and broadcast ProjectDTO. Keep c.Param("id") but note the registered name is now :projectId — update to c.Param("projectId") to stay consistent with the nested group.
- `api/internal/api/v0/endpoints/repos/routes.go` — Per spec §3: register on the projectScoped group so the prefix is /projects/:projectId/repos. GET/WS /repos dual-served (Broadcaster[RepoDTO]); GET/WS /repos/:repoId dual-served; POST → 202; add DELETE /repos/:repoId → 202. Rename param :id → :repoId. Move icon routes to /repos/:repoId/icon[/emoji|/github] (serve bytes from <home>/projects/<P>/<R>/icon, §1). branches + protected-branches under /repos/:repoId.
- `api/internal/api/v0/endpoints/repos/handlers/repos.go` — Per spec §1/§3/§4: read c.Param("projectId")+c.Param("repoId"). Icon handler reads bytes from worktreepath.RepoDir(home,p,r)+"/icon" instead of redirecting AvatarURL to GitHub. Add a DeleteRepo handler (202+WS, rm -rf repo dir). Create returns 202. PutIconGithub fetches+stores avatar bytes to disk.
- `api/internal/api/v0/endpoints/workspaces/routes.go` — Per spec §3: register on repoScoped group → prefix /projects/:projectId/repos/:repoId/workspaces. GET/WS /workspaces dual-served (already), GET/WS /workspaces/:wsId dual-served (NEW — detail must upgrade). POST /workspaces → 202; DELETE/sync/merge-into-parent/reparent → 202. Remove the dedicated rg.GET("/ws/workspaces") route (dual-serve covers it).
- `api/internal/api/v0/endpoints/workspaces/handlers/crud.go` — Per spec §4/§8/§10: Create reads repoId+projectId from PATH (not body), validates synchronously, returns 202; runs worktree add in a goroutine then broadcasts WorkspaceDTO (status new→ready). buildCreateInput drops RemoteURL dependence (worktreepath now UUID-based). Resolve branch-exists-on-remote vs create-from-parent internally (§3, E2E Step 4). Delete returns 202 and broadcasts WorkspaceDTO{status:"deleted"}.
- `api/internal/api/v0/endpoints/workspaces/handlers/list.go` — Per spec §3/§10: List reads projectId+repoId from PATH params; populate CanMergeLocally+ParentBranch via MergeEligibilityFor over the returned sibling set. Detail likewise computes merge eligibility. filterWorkspaces switches from query to path params (or list is already repo-scoped at the DB level).
- `api/internal/api/v0/endpoints/git/routes.go` — Per spec §3: register on wsScoped group → prefix /projects/:projectId/repos/:repoId/workspaces/:wsId/git. Remove rg.GET("/ws/git") (status dual-serve only). Reads stay GET 200; stage/unstage/discard/commit stay 200; push/fetch/pull/merge/rebase become 202+WS WorkspaceDTO (set lastError on failure, §4). Keep superset write routes (stage-hunk, switch, stash*, reset, resolve-hunk, operation*) but reclassify push/fetch/pull/merge/rebase as 202.
- `api/internal/api/v0/endpoints/files/routes.go` — Per spec §3: register on wsScoped group. Replace rg.GET("/ws/files") with rg.GET(".../files/ws", filesWS) co-located WS. Keep content/tree GET + POST/PATCH/DELETE/PUT mutations sync 200.
- `api/internal/api/v0/endpoints/editor/routes.go` — Per spec §3: register on wsScoped group. Replace rg.GET("/ws/lsp") with rg.GET(".../lsp/ws", lspWSHandle). LSP feature/sync routes stay POST/GET 200. blame stays (superset).
- `api/internal/api/v0/endpoints/terminal/routes.go` — Per spec §3: split registration. Mount POST+GET/WS .../workspaces/:wsId/terminals (list/lifecycle Broadcaster[TerminalSessionDTO]) and DELETE .../terminals/:sessionId and PTY .../terminals/:sessionId/ws on wsScoped group. Keep /settings/terminal/profiles CRUD on the top-level rg (global). POST → 201 {sessionId}; DELETE → 202.
- `api/internal/api/v0/endpoints/search/routes.go` — Per spec §3: register on wsScoped group → .../workspaces/:wsId/search[/replace]. search stays 200; replace may return 202 for large ops.
- `api/internal/api/v0/endpoints/provider/routes.go` — Per spec §3: GET .../workspaces/:wsId/provider on wsScoped group; GET .../projects/:projectId/repos/:repoId/protected-branches on repoScoped group. Rename :id→:repoId. Both stay GET 200.
- `api/internal/api/v0/endpoints/review/routes.go` — Per spec §1/§3/§4: keep GET/PATCH .../workspaces/:wsId/review on wsScoped. EXTRACT threads into a new threads endpoint: GET/WS+POST .../workspaces/:wsId/threads (Broadcaster[ThreadDTO]), GET .../threads/:threadId, PATCH .../threads/:threadId, POST .../threads/:threadId/replies. Thread mutations are Asynx-backed sync→broadcast ThreadDTO.
- `api/internal/api/v0/endpoints/chats/routes.go` — Per spec §3/§12 Open-Q3: chats are out-of-scope/TODO. Remove from the new hierarchical mount (commented in spec route tree) or leave dormant pending decision. Do NOT carry the flat /workspaces/:wsId/chats + /ws/chats routes into the new tree.
- `api/internal/api/v0/endpoints/agentrun/routes.go` — Per spec §12 Open-Q3: DELETE the entire agentrun endpoint package and its router.go wiring (agent-run concept eliminated).
- `api/internal/api/v0/container.go` — Per spec §5: add Broadcaster[ProjectDTO], Broadcaster[RepoDTO], Broadcaster[ThreadDTO], Broadcaster[TerminalSessionDTO] instances and their StreamDefs. workspacesDef Namespace becomes ProjectID+"/"+RepoID+"/"+ID (per §5) with projectId/repoId/wsId resolved as PATH params (filter.go resolveFilterValue already path-first). Remove chats/chatStream/agentrun broadcasters from scope (chat is TODO). Wire per-connection provider poll (§11) into the workspace broadcaster's OnSubscribe/OnUnsubscribe scoped by wsId.
- `api/internal/api/v0/middleware.go` — No signature change, but verify rejectEmptyPathParams() still aborts correctly when :projectId/:repoId/:wsId are all present — it iterates all c.Params so deeper nesting is handled. Add an integration test that //projects//repos//workspaces with empty segments 400s.
- `api/internal/api/v0/route_audit_test.go` — Per spec §3/§13: rewrite specRoutes()/extraRoutes() to the full hierarchical tree under /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/... Remove all /ws/* dedicated routes (folded into dual-serve + .../files/ws, .../lsp/ws, .../terminals/:sessionId/ws). Remove agentrun + chats routes. Update TestRouteAudit_DualServe_* hard-coded paths to the nested equivalents.
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go` — Per spec §8: replace For(crowbarHome, remoteURL, workspaceID)(string,error) with For(crowbarHome, projectID, repoID, workspaceID) string (no error). Add StorageDir, RepoDir(crowbarHome,projectID,repoID), ProjectDir(crowbarHome,projectID). Delete repoRelPath/URL parsing. This unblocks the route handlers to construct paths from the path params.

### New contracts
- // router.go — nested group construction
func (c *Container) Register(
	rg *gin.RouterGroup,
)
- // router.go group chain (illustrative target)
projects := rg.Group("/projects")
projectScoped := projects.Group("/:projectId")
repos := projectScoped.Group("/repos")
repoScoped := repos.Group("/:repoId")
workspaces := repoScoped.Group("/workspaces")
wsScoped := workspaces.Group("/:wsId")
- // projects/routes.go
func Register(
	rg *gin.RouterGroup,
	reader projecthandlers.ListGetter,
	importer projecthandlers.Importer,
	deleter projecthandlers.Deleter,
	projectsWS gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
)
- // projects routes (target paths, mounted on rg=/v0)
GET/WS  /v0/projects                 -> dispatch(h.List, projectsWS)
POST    /v0/projects                 -> h.Import (202)
GET/WS  /v0/projects/:projectId       -> dispatch(h.Detail, projectsWS)
DELETE  /v0/projects/:projectId       -> h.Delete (202)
- // repos/routes.go (mounted on projectScoped group)
func Register(
	rg *gin.RouterGroup,
	store repohandlers.Store,
	prov repohandlers.BranchProviderEngine,
	wsReader repohandlers.WorkspaceReader,
	reposWS gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
)
- // repos routes (target)
GET/WS  /v0/projects/:projectId/repos                          -> dispatch(h.List, reposWS)
POST    /v0/projects/:projectId/repos                          -> h.Create (202)
GET/WS  /v0/projects/:projectId/repos/:repoId                  -> dispatch(h.Detail, reposWS)
DELETE  /v0/projects/:projectId/repos/:repoId                  -> h.Delete (202)
GET     /v0/projects/:projectId/repos/:repoId/icon             -> h.Icon
PUT     /v0/projects/:projectId/repos/:repoId/icon             -> h.PutIcon
DELETE  /v0/projects/:projectId/repos/:repoId/icon             -> h.DeleteIcon
PUT     /v0/projects/:projectId/repos/:repoId/icon/emoji       -> h.PutIconEmoji
PUT     /v0/projects/:projectId/repos/:repoId/icon/github      -> h.PutIconGithub
GET     /v0/projects/:projectId/repos/:repoId/branches         -> h.Branches
GET     /v0/projects/:projectId/repos/:repoId/protected-branches -> provider.ProtectedBranches
- // workspaces/routes.go (mounted on repoScoped group)
func Register(
	rg *gin.RouterGroup,
	reader workspacehandlers.Reader,
	hierarchy workspacehandlers.Hierarchy,
	repos workspacehandlers.Repos,
	wsHandle gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
)
- // workspaces routes (target)
GET/WS  /v0/projects/:projectId/repos/:repoId/workspaces                    -> dispatch(h.List, wsHandle)
POST    /v0/projects/:projectId/repos/:repoId/workspaces                    -> h.Create (202)
GET/WS  /v0/projects/:projectId/repos/:repoId/workspaces/:wsId              -> dispatch(h.Detail, wsHandle)
DELETE  /v0/projects/:projectId/repos/:repoId/workspaces/:wsId              -> h.Delete (202)
POST    /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/sync         -> h.Sync (202)
POST    /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/merge-into-parent -> h.MergeIntoParent (202)
POST    /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/reparent     -> h.Reparent (202)
- // workspaces/handlers/crud.go — new create body (repoId/projectId from PATH)
type createRequest struct {
	Branch   string `json:"branch"`
	ParentID string `json:"parentId"`
	Locked   bool   `json:"locked"`
}
// repoId, projectId read via c.Param("repoId"), c.Param("projectId")
- // git/routes.go (mounted on wsScoped group; prefix .../workspaces/:wsId/git)
func Register(
	rg *gin.RouterGroup,
	gitSvc githandlers.Git,
	gitWS gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
)
// GET/WS .../git/status -> dispatch(h.Status, gitWS); push/fetch/pull/merge/rebase POST -> 202; no /ws/git route
- // files/routes.go (wsScoped) — co-located WS
GET   .../workspaces/:wsId/files/content -> h.ReadContent
GET   .../workspaces/:wsId/files/tree    -> h.Tree
PUT   .../workspaces/:wsId/files/content -> h.SaveContent
POST  .../workspaces/:wsId/files          -> h.Create
PATCH .../workspaces/:wsId/files          -> h.Rename
DELETE .../workspaces/:wsId/files         -> h.Delete
WS    .../workspaces/:wsId/files/ws       -> filesWS
- // editor/routes.go (wsScoped) — co-located WS
WS .../workspaces/:wsId/lsp/ws -> lspWSHandle  (replaces /ws/lsp)
- // terminal/routes.go — split
// wsScoped group:
GET/WS .../workspaces/:wsId/terminals               -> dispatch(h.List, terminalsWS)
POST   .../workspaces/:wsId/terminals               -> h.CreateSession (201 {sessionId})
DELETE .../workspaces/:wsId/terminals/:sessionId    -> h.KillSession (202)
WS     .../workspaces/:wsId/terminals/:sessionId/ws -> h.WS  (raw PTY pipe)
// top-level rg:
GET/POST/PUT/DELETE /v0/settings/terminal/profiles[/:id]
- // review + threads (wsScoped)
GET   .../workspaces/:wsId/review                       -> h.Get
PATCH .../workspaces/:wsId/review                       -> h.SetMergeStrategy
GET/WS .../workspaces/:wsId/threads                      -> dispatch(threads.List, threadsWS)
POST  .../workspaces/:wsId/threads                       -> threads.Open (sync->broadcast)
GET   .../workspaces/:wsId/threads/:threadId             -> threads.Detail
PATCH .../workspaces/:wsId/threads/:threadId             -> threads.SetResolved
POST  .../workspaces/:wsId/threads/:threadId/replies     -> threads.Reply
- // worktreepath.go (spec §8 — exact)
func For(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
) string

func StorageDir(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
) string

func RepoDir(
	crowbarHome string,
	projectID string,
	repoID string,
) string

func ProjectDir(
	crowbarHome string,
	projectID string,
) string
- // container.go — new broadcaster fields (spec §5)
type Container struct {
	projects   *ws.Broadcaster[ProjectDTO]
	repos      *ws.Broadcaster[RepoDTO]
	workspaces *ws.Broadcaster[domain.Workspace]
	threads    *ws.Broadcaster[ThreadDTO]
	terminals  *ws.Broadcaster[TerminalSessionDTO]
	git        *ws.Broadcaster[gitdomain.GitStatusEvent]
	files      *ws.Broadcaster[domain.FileChangeEvent]
	lsp        *ws.Broadcaster[lspdomain.DiagnosticsEvent]
	app        *app.Container
	eng        *engine.Container
}
- // container.go — workspace namespace (spec §5)
Namespace: func(d domain.Workspace) string {
	return d.ProjectID + "/" + d.RepoID + "/" + d.ID
}
// Filters: {Param:"projectId",Extract:ProjectID}, {Param:"repoId",Extract:RepoID}, {Param:"wsId",Extract:ID} — all path-first via resolveFilterValue

### Risks
- Param-name collision: today repos/provider use :id while workspaces use :wsId. gin forbids two different param names at the same tree position. Re-nesting to /:projectId/.../:repoId/.../:wsId must rename every :id to :projectId/:repoId consistently across projects, repos, provider handlers AND their c.Param() reads, or gin panics at registration (panic: ':projectId' in new path conflicts with existing wildcard).
- gin RouterGroup nesting depth: building groups projects/:projectId/repos/:repoId/workspaces/:wsId means every leaf route shares the wildcard chain. Any sibling static segment that conflicts with a wildcard at the same depth (e.g. /repos/:repoId vs a future /repos/search) will panic. The /settings/terminal/profiles and /system + /health routes must stay OUTSIDE the /projects group.
- rejectEmptyPathParams now guards 3 params per request — a request like /v0/projects//repos//workspaces/x 400s, which is correct, but the route_audit and any client constructing URLs with optional missing IDs (e.g. list scope at repo level hitting the workspace route) will newly 400 instead of matching a shorter route. Frontend must always build the full prefix.
- Removing /ws/workspaces, /ws/git, /ws/files, /ws/lsp, /ws/chats, /ws/terminals dedicated routes breaks the frontend WS client (web/src/lib/api.ts references /v0/ws/workspaces, /v0/ws/chats, /v0/ws/terminals/:sessionId, plus /ws/git in stores). Dual-serve on the nested GET URLs replaces them; the FE must switch from query-param scoping (?wsId=) to path scoping. resolveFilterValue is path-first so backend filters still bind, but the FE URL builders are all flat today.
- Broadcaster snapshot race is preserved (snapshot computed outside lock) but the workspace Namespace change to projectID/repoID/ID alters the prefix-match semantics: a client at .../repos/:r/workspaces (no wsId) must match prefix p/r/ — the current FilterDef ExactMatch on projectId+repoId already achieves this WITHOUT relying on the Namespace string, so the Namespace change is cosmetic for filtering; verify no code prefix-matches the Namespace directly. The spec §5 prose implies prefix matching on the namespace string, but the actual implementation filters via FilterDef per-field — a mismatch to reconcile.
- Workspace POST moving repoId/projectId from body to path is a breaking contract change. Existing crud_test.go, hierarchy_test.go, list_test.go and the integration suite (wave3_integration_test.go, route_audit_test.go) assert the flat shape and WILL fail; they must be migrated in lockstep or the build is red.
- 202-async conversion (Create/Delete/push/merge/reparent) requires a background goroutine + Asynx command + Broadcaster push. The current handlers block and return 201/200 synchronously. Splitting validation from execution risks losing the error path: spec §4 says failures set WorkspaceDTO.LastError (no error WS frame). If the goroutine panics or the entity DB open fails after 202, there is no HTTP channel left to report it — must ensure lastError is always broadcast.
- agentrun removal: agentrun.Register is wired in router.go:101 and the package + handlers + AgentRunES event store (adapter/container.go:23) + frontend /v0/runs/* calls all reference it. Deleting it cascades through adapter.Container, app.Repositories.AgentRun, and FE. go mod / build will break until every reference is removed (per MEMORY: go mod tidy prunes unused deps).
- chats marked TODO but currently fully wired (router.go:55, chats broadcaster, chatStream broadcaster, snapshots.go chatsSnapshot, container.go chatsDef/chatStreamDef). Decision needed: leave dormant (keeps flat /workspaces/:wsId/chats which violates the hierarchical tree) or remove. Either way the route_audit_test set must reflect the choice.
- worktreepath.For signature change from (home,remoteURL,wsId) to (home,projectID,repoID,wsId) ripples into every caller (worktree usecase, crud.buildCreateInput which currently passes RemoteURL). The RemoteURL field on CreateChildInput becomes dead for path construction but may still be needed for git remote ops — must distinguish path-derivation use from git-remote use to avoid silently breaking checkout/push.
- Icon-on-disk (spec §1) depends on the new worktreepath.RepoDir(home,p,r) existing AND the import flow writing the avatar bytes to <P>/<R>/icon. The current Create handler stores AvatarURL (an HTTPS string or local path) in the GORM row and the Icon handler redirects/serves it. The disk path does not exist yet — flagged: the spec assumes a storage layout (storages/, icon file) that adapter.New does not currently create.
- DualServe on workspace DETAIL (GET/WS /workspaces/:wsId) is new — today only the LIST and git/status are dual-served. The detail Broadcaster must filter to a single workspace by the :wsId path param; the existing workspacesDef has no wsId FilterDef (only projectId/repoId), so a wsId FilterDef must be added or the namespace-glob :ns mechanism used, otherwise a detail-scoped client receives all repo workspaces.
- Per-connection provider polling (§11) is a NEW goroutine started in the WS upgrade handler and cancelled on close. The Broadcaster.Handle lifecycle hooks (OnSubscribe/OnUnsubscribe, scoped by scopeWsID) are the natural attach point, but they are currently bound to watcher/LSP refcounts via withWatcherLifecycle/withLSPLifecycle — adding a third concern (provider poll) to the same StreamDef hooks risks coupling three independent lifecycles to one refcount.
- Storage layout / lazy per-entity DB registry (spec §1/§2/§9) and the LRU cache type (lru.Cache) do not exist in adapter/container.go today — it opens 4 global stores eagerly. The spec's AdapterContainer.WorkspaceES(projectID,repoID,wsID) method and maxOpenWorkspaceDBs cap are entirely new; flagged as assumed-not-present. Handlers passing projectId/repoId/wsId to resolve the right DB depend on this not-yet-existing API.

### Test targets
- api/internal/api/v0/route_audit_test.go (integration): rewrite TestRouteAudit_AllSpecRoutesRegistered with the full §3 hierarchical specRoutes()/extraRoutes(); assert exact set equality. Add cases for every nested path incl. /projects/:projectId, /repos/:repoId, /workspaces/:wsId/git/status, /threads/:threadId/replies, /terminals/:sessionId/ws, /files/ws, /lsp/ws.
- api/internal/api/v0/route_audit_test.go: update TestRouteAudit_DualServe_RestMode + TestRouteAudit_DualServe_WsMode hard-coded paths to /v0/projects/p1/repos/r1/workspaces and /v0/projects/p1/repos/r1/workspaces/w1/git/status; add the new dual-served detail routes (projects/:projectId, repos/:repoId, workspaces/:wsId) to both modes.
- api/internal/api/v0/middleware_test.go (or existing): TestRejectEmptyPathParams_NestedSegments — GET /v0/projects//repos/r1/workspaces/w1 and /v0/projects/p1/repos//workspaces/w1 each return 400 envelope; full-path request 2xx/404 (not 400).
- api/internal/api/v0/endpoints/workspaces/handlers/crud_test.go: TestCreate_ReadsRepoIdFromPath (repoId/projectId from c.Param, not body); TestCreate_Returns202; TestCreate_ValidationFailsSync_4xx (missing branch, repo not found); TestDelete_Returns202.
- api/internal/api/v0/endpoints/workspaces/handlers/list_test.go: TestList_ScopedByPathParams (projectId/repoId path); TestList_PopulatesMergeEligibility (CanMergeLocally true when parent sibling not locked/deleted, false otherwise) — exercises spec §10 MergeEligibilityFor.
- api/internal/app/usecases/internal/worktreepath/worktreepath_test.go: rewrite for new signatures — TestFor_UUIDPath (For(home,p,r,w) == home/projects/p/r/workspaces/w/worktree, no error); TestStorageDir; TestRepoDir; TestProjectDir. Plus worktreepath_bench_test.go BenchmarkFor (path construction, spec §13).
- api/internal/api/v0/endpoints/repos/handlers/repos_test.go: TestIcon_ServesBytesFromDisk (reads <home>/projects/p/r/icon, content-type sniff); TestCreate_Returns202; TestDeleteRepo_Returns202; TestParamRename_repoId (c.Param("repoId")).
- api/tests/TestRegression_WorkspaceCreate_202_then_WS_DTO (integration): POST .../workspaces → 202 empty body; open WS .../workspaces; assert WorkspaceDTO{status:"new"} then ready DTO. Block on WS message via context deadline — NO time.Sleep.
- api/tests/TestRegression_WorkspaceDelete_202_then_WS_Deleted (integration): DELETE → 202; WS emits WorkspaceDTO{status:"deleted"}.
- api/tests/TestRegression_GitPush_202_then_WS_LastError (integration): POST .../git/push → 202; WS WorkspaceDTO carries updated status or non-empty LastError on failure (mock remote denies).
- api/tests/TestRegression_WorkspaceNamespaceFiltering (integration): subscribe at project scope, repo scope, ws scope; assert each client only receives DTOs whose projectId/repoId/wsId match the path prefix (spec §5).
- api/tests/TestRegression_MergeEligibility_SiblingState (integration): GET workspace with parent locked → CanMergeLocally=false; parent active → true + ParentBranch set.
- api/tests/TestRegression_WorkspaceCreate_BranchExistsVsNot (integration): remote branch exists → checkout path; absent → create-from-parent path (spec §3, E2E Step 4).
- api/internal/api/v0/ws/broadcaster_bench_test.go: add BenchmarkBroadcaster_FanOut for the 3-level namespace filter (spec §13 broadcaster fan-out); api/internal/api/v0/ws/filter benchmark for resolveFilterValue path-first scan.

---

## WebSocket broadcaster, dual-serve, dispatch, hub fan-out, snapshots (api/internal/api/v0/ws + app/hub + v0 container/snapshots)

### Key signatures
- `api/internal/api/v0/ws/broadcaster.go`
  - type Broadcaster[T any] struct{ def StreamDef[T]; mu sync.RWMutex; clients map[*filteredClient[T]]struct{}; registered chan struct{}; once sync.Once; regCount chan struct{} } (broadcaster.go:16)
  - type filteredClient[T any] struct{ *client; predicate func(T) bool } (broadcaster.go:10)
  - func NewBroadcaster[T any](def StreamDef[T]) *Broadcaster[T] (broadcaster.go:28)
  - func (b *Broadcaster[T]) Handle(c *gin.Context) (broadcaster.go:59)
  - func (b *Broadcaster[T]) scopeKey(c *gin.Context) string (broadcaster.go:81)
  - func (b *Broadcaster[T]) register/remove/snapshotFor/Push (broadcaster.go:115,129,143,165)
  - func (b *Broadcaster[T]) WaitRegistered()/WaitNRegistered(n int) test-only sync (broadcaster.go:40,47)
- `api/internal/api/v0/ws/stream_def.go`
  - type StreamDef[T any] struct{ Namespace func(T) string; Serialize func(T)([]byte,error); Filters []FilterDef[T]; Snapshot func()[]T; ScopeKey func(*gin.Context) string; OnSubscribe func(scope string); OnUnsubscribe func(scope string) } (stream_def.go:14)
  - type FilterDef[T any] struct{ Param string; Extract func(T) string; Match func(param,value string) bool; Default string } (stream_def.go:25)
- `api/internal/api/v0/ws/filter.go`
  - func BuildPredicate[T any](c *gin.Context, def StreamDef[T]) func(T) bool (filter.go:36)
  - func GlobMatch(pattern, value string) bool (filter.go:18)
  - func ExactMatch(param, value string) bool (filter.go:10)
  - func resolveFilterValue[T any](c *gin.Context, f FilterDef[T]) string path>query>Default (filter.go:69)
- `api/internal/api/v0/ws/dispatch.go`
  - func Dispatch[T any](b *Broadcaster[T], snapshot func(*gin.Context)(any,error)) gin.HandlerFunc (dispatch.go:13)
  - func writeSnapshot(c *gin.Context, snapshot func(*gin.Context)(any,error)) (dispatch.go:26)
- `api/internal/api/v0/ws/dual_serve.go`
  - func DualServe(rest gin.HandlerFunc, wsHandler gin.HandlerFunc) gin.HandlerFunc (dual_serve.go:14)
- `api/internal/api/v0/ws/client.go`
  - type client struct{ send chan []byte; done chan struct{} } (client.go:21)
  - const sendBuffer=64; pingInterval=30s; pongTimeout=60s; writeTimeout=10s (client.go:10)
  - func writePump(conn *websocket.Conn, cl *client, snapshot [][]byte) (client.go:51)
  - func flushSnapshot/writeNext/readPump (client.go:74,93,33)
- `api/internal/api/v0/container.go`
  - type Container struct{ workspaces *ws.Broadcaster[domain.Workspace]; chats *ws.Broadcaster[hub.ChatStatusEvent]; git *ws.Broadcaster[gitdomain.GitStatusEvent]; files *ws.Broadcaster[domain.FileChangeEvent]; lsp *ws.Broadcaster[lspdomain.DiagnosticsEvent]; chatStream *ws.Broadcaster[ChatFrame]; app; eng } (container.go:27)
  - func New(appContainer *app.Container, engContainer *engine.Container) *Container (container.go:49)
  - func (c *Container) PushWorkspace/PushChat/PushGit/PushFile (container.go:111,118,126,139)
  - func workspacesDef — Namespace=w.ID; Filters projectId,repoId ExactMatch (container.go:145)
  - func gitDef/filesDef/lspDef — Namespace=e.WsID; Filter wsId ExactMatch (container.go:178,191,201)
  - func scopeWsID(c *gin.Context) string (container.go:101)
- `api/internal/api/v0/snapshots.go`
  - func workspacesSnapshot(appContainer) func() []domain.Workspace (snapshots.go:18)
  - func chatsSnapshot(appContainer) func() []hub.ChatStatusEvent (snapshots.go:33)
  - func gitSnapshot(appContainer) func() []gitdomain.GitStatusEvent (snapshots.go:52)
  - func lspSnapshot(appContainer, engContainer) func() []lspdomain.DiagnosticsEvent (snapshots.go:91)
- `api/internal/api/v0/router.go`
  - func (c *Container) Register(rg *gin.RouterGroup) (router.go:24)
  - workspaces.Register(..., c.workspaces.Handle, ws.DualServe) (router.go:47)
  - git.Register(..., c.git.Handle, ws.DualServe) (router.go:68)
  - chats.Register(..., c.chats.Handle, c.chatStream.Handle) (router.go:55); agentrun.Register(...) (router.go:101)
- `api/internal/app/hub/hub.go`
  - type Hub struct{ mu sync.RWMutex; subscribers []Subscriber } (hub.go:12)
  - func (h *Hub) Register(s Subscriber) (hub.go:23)
  - func (h *Hub) BroadcastWorkspace(ws domain.Workspace) (hub.go:32)
  - func (h *Hub) BroadcastChat(evt ChatStatusEvent) (hub.go:43)
  - func (h *Hub) BroadcastGit(wsID string, status gitdomain.GitStatus) (hub.go:54)
  - func (h *Hub) BroadcastFile(evt domain.FileChangeEvent) (hub.go:66)
- `api/internal/app/hub/subscriber.go`
  - type Subscriber interface{ PushWorkspace(ws domain.Workspace); PushChat(evt ChatStatusEvent); PushGit(wsID string, status gitdomain.GitStatus); PushFile(evt domain.FileChangeEvent) } (subscriber.go:9)
- `api/internal/app/hub/web_socket_hub.go`
  - type WebSocketHub interface{ BroadcastWorkspace(ws domain.Workspace); BroadcastChat(evt ChatStatusEvent); BroadcastGit(wsID string, status gitdomain.GitStatus); BroadcastFile(evt domain.FileChangeEvent) } (web_socket_hub.go:9)
- `api/internal/app/hub/chat_status_event.go`
  - type ChatStatusEvent struct{ ChatID string `json:"chatId"`; WsID string `json:"wsId"`; Status domain.ChatStatus `json:"status"` } (chat_status_event.go:6)
- `api/internal/api/v0/chat_frame.go`
  - type ChatFrame struct{ ChatID string `json:"chatId"` } (chat_frame.go:8)
- `api/internal/api/v0/dto/workspace.go`
  - type WorkspaceDTO struct{ ID; WorktreePath; RepoID; ProjectID; Branch; ParentID; ForkPointSha; Status; Locked bool; HasConflicts bool; Added; Deleted; MergeStrategy; PRUrl; PRTitle; PRTargetBranch; AgentRunning bool; PendingMerge *gitdomain.PendingMerge } (workspace.go:12)
  - func WorkspaceDTOFrom(w domain.Workspace) WorkspaceDTO (workspace.go:34)
- `api/internal/api/v0/dto/repo.go`
  - type RepoDTO struct{ ID; ProjectID; Name; Path; DefaultBranch; AvatarLabel; AvatarColor; AvatarURL string `json:"avatarUrl,omitempty"` } (repo.go:9)
  - func RepoDTOFrom(r domain.Repository) RepoDTO builds avatarURL='/v0/repos/'+r.ID+'/icon' (repo.go:20)

### Must change
- `api/internal/api/v0/ws/filter.go` — Spec §5 requires PREFIX namespace matching (client at .../repos/:r receives every event whose namespace begins with p/r), which GlobMatch (path.Match) cannot express (path.Match treats '/' as a segment separator and does not prefix-match). Add a prefix-aware matcher (PrefixMatch) and have BuildPredicate derive the client's scope prefix from path params (projectId/repoId/wsId) and use it. Keep ExactMatch/GlobMatch for back-compat filters.
- `api/internal/api/v0/ws/stream_def.go` — Spec §5: standardize Namespace to return the slash-joined hierarchical key (p/r/w). Spec §9: Snapshot func()[]T has no scope arg, so a scoped snapshot must enumerate globally then filter — incompatible with lazy per-entity storage. Change Snapshot to receive the connecting client's scope/context (e.g. func(scope string) []T or func(*gin.Context) []T) so it reads only the subscribed subtree.
- `api/internal/api/v0/ws/broadcaster.go` — Spec §9: snapshotFor (line 143) calls b.def.Snapshot() with no scope; update to pass the client's scope/context so per-entity lazy storage is not forced to enumerate every workspace. Spec §11: extend Handle (line 59) to start a per-connection provider-poll goroutine bound to a context cancelled when the WS closes (after readPump returns), distinct from the existing OnSubscribe/OnUnsubscribe scope hooks. Preserve the register-then-snapshot-outside-lock idempotent-replace invariant.
- `api/internal/api/v0/container.go` — Spec §5: replace flat domain-type broadcasters with per-entity typed DTO broadcasters — add Broadcaster[dto.ProjectDTO], Broadcaster[dto.RepoDTO], Broadcaster[dto.ThreadDTO], Broadcaster[dto.TerminalSessionDTO]; change Broadcaster[domain.Workspace] to Broadcaster[dto.WorkspaceDTO]. Rewrite each *Def so Namespace returns the composite prefix (workspace projectID/repoID/ID; repo projectID/ID; project ID; thread+terminal projectID/repoID/wsID/ID). Drop projectId/repoId/wsId query Filters in favor of prefix matching. Serialize marshals the DTO; CanMergeLocally/ParentBranch (§10) computed in the usecase before Push, not in Serialize. Remove chats/chatStream broadcasters and PushChat (§12).
- `api/internal/api/v0/snapshots.go` — Spec §6/§9/§10: re-scope snapshots to the subtree. workspacesSnapshot takes projectID+repoID and returns only that repo's workspaces as DTOs with CanMergeLocally/ParentBranch from siblings (MergeEligibilityFor). gitSnapshot/lspSnapshot scope to a single workspace (no cross-project Workspace.List). Add repoSnapshot/projectSnapshot/threadSnapshot/terminalSessionSnapshot. Remove chatsSnapshot.
- `api/internal/api/v0/router.go` — Spec §3/§7: rewrite every mount to hierarchical paths (/v0/projects/:projectId/repos/:repoId/workspaces[/:wsId], .../threads, .../terminals, .../git/status). Pass the new entity-broadcaster Handles into projects/repos/workspaces/threads/terminal Register. Remove agentrun.Register and chats.Register. Wire the per-connection provider poll (§11) into the workspace WS upgrade path.
- `api/internal/app/hub/hub.go` — Spec §5: add BroadcastProject(dto.ProjectDTO), BroadcastRepo(dto.RepoDTO), BroadcastThread(dto.ThreadDTO), BroadcastTerminalSession(dto.TerminalSessionDTO); switch BroadcastWorkspace to carry dto.WorkspaceDTO (or convert in v0.Container, kept consistent with subscriber.go). Remove BroadcastChat (§12).
- `api/internal/app/hub/subscriber.go` — Spec §5: widen Subscriber with PushProject/PushRepo/PushThread/PushTerminalSession; remove PushChat. v0.Container and every test fake implementing Subscriber must be updated in lockstep or compilation breaks across packages.
- `api/internal/app/hub/web_socket_hub.go` — Spec §5: mirror subscriber.go — add BroadcastProject/BroadcastRepo/BroadcastThread/BroadcastTerminalSession, remove BroadcastChat. Producers (app/repositories/container.go, app/realtime) depend on this interface.
- `api/internal/app/hub/chat_status_event.go` — Spec §12 (chat out of scope) + agent-run eliminated: delete ChatStatusEvent and its broadcast/subscriber wiring, or leave a TODO stub. No producer remains once BroadcastChat is removed.
- `api/internal/api/v0/chat_frame.go` — Spec §12: remove ChatFrame + chatStream broadcaster + its route (deferred chat feature).
- `api/internal/api/v0/dto/workspace.go` — Spec §5: add Working bool, LastError string, CanMergeLocally bool, ParentBranch string; remove Locked, HasConflicts, AgentRunning, PendingMerge, WorktreePath (and any PendingOp). WorkspaceDTOFrom accepts the sibling list to populate CanMergeLocally/ParentBranch via MergeEligibilityFor (§10). The broadcaster now emits THIS DTO (not domain.Workspace).
- `api/internal/api/v0/dto/repo.go` — Spec §3: change RepoDTOFrom avatarURL to the hierarchical /v0/projects/:projectId/repos/:repoId/icon path. This DTO becomes the Broadcaster[RepoDTO] payload.
- `api/internal/api/v0/ws/dual_serve.go` — No mechanism change, but every caller moves to hierarchical paths; consolidate DualServe vs Dispatch to one helper to avoid two parallel upgrade-detection paths (both exist; only DualServe is wired).

### New contracts
- // ws/filter.go — hierarchical prefix matcher (spec §5)
func PrefixMatch(
	prefix string,
	value string,
) bool
- // ws StreamDef namespace conventions (spec §5) — funcs, no struct change:
Namespace: func(d dto.WorkspaceDTO) string { return d.ProjectID + "/" + d.RepoID + "/" + d.ID }
Namespace: func(d dto.RepoDTO) string { return d.ProjectID + "/" + d.ID }
Namespace: func(d dto.ProjectDTO) string { return d.ID }
Namespace: func(d dto.ThreadDTO) string { return d.ProjectID + "/" + d.RepoID + "/" + d.WorkspaceID + "/" + d.ID }
Namespace: func(d dto.TerminalSessionDTO) string { return d.ProjectID + "/" + d.RepoID + "/" + d.WorkspaceID + "/" + d.ID }
- // ws/stream_def.go — scoped snapshot (spec §9)
type StreamDef[T any] struct {
	Namespace func(T) string
	Serialize func(T) ([]byte, error)
	Filters   []FilterDef[T]
	Snapshot  func(scope string) []T
	ScopeKey  func(*gin.Context) string
	OnSubscribe   func(scope string)
	OnUnsubscribe func(scope string)
}
- // v0/container.go — new per-entity typed broadcasters (spec §5)
type Container struct {
	projects   *ws.Broadcaster[dto.ProjectDTO]
	repos      *ws.Broadcaster[dto.RepoDTO]
	workspaces *ws.Broadcaster[dto.WorkspaceDTO]
	threads    *ws.Broadcaster[dto.ThreadDTO]
	terminals  *ws.Broadcaster[dto.TerminalSessionDTO]
	git        *ws.Broadcaster[gitdomain.GitStatusEvent]
	files      *ws.Broadcaster[domain.FileChangeEvent]
	lsp        *ws.Broadcaster[lspdomain.DiagnosticsEvent]
	app        *app.Container
	eng        *engine.Container
}
- // app/hub/subscriber.go — widened Subscriber (spec §5)
type Subscriber interface {
	PushProject(
		p dto.ProjectDTO,
	)
	PushRepo(
		r dto.RepoDTO,
	)
	PushWorkspace(
		w dto.WorkspaceDTO,
	)
	PushThread(
		t dto.ThreadDTO,
	)
	PushTerminalSession(
		s dto.TerminalSessionDTO,
	)
	PushGit(
		wsID string,
		status gitdomain.GitStatus,
	)
	PushFile(
		evt domain.FileChangeEvent,
	)
}
- // app/hub/web_socket_hub.go — widened producer interface (spec §5)
type WebSocketHub interface {
	BroadcastProject(
		p dto.ProjectDTO,
	)
	BroadcastRepo(
		r dto.RepoDTO,
	)
	BroadcastWorkspace(
		w dto.WorkspaceDTO,
	)
	BroadcastThread(
		t dto.ThreadDTO,
	)
	BroadcastTerminalSession(
		s dto.TerminalSessionDTO,
	)
	BroadcastGit(
		wsID string,
		status gitdomain.GitStatus,
	)
	BroadcastFile(
		evt domain.FileChangeEvent,
	)
}
- // dto/workspace.go — new wire shape (spec §5)
type WorkspaceDTO struct {
	ID              string                  `json:"id"`
	RepoID          string                  `json:"repoId"`
	ProjectID       string                  `json:"projectId"`
	Branch          string                  `json:"branch"`
	ParentID        string                  `json:"parentId,omitempty"`
	ForkPointSha    string                  `json:"forkPointSha,omitempty"`
	Status          domain.WorkspaceStatus  `json:"status"`
	Working         bool                    `json:"working"`
	LastError       string                  `json:"lastError,omitempty"`
	Added           int                     `json:"added"`
	Deleted         int                     `json:"deleted"`
	MergeStrategy   gitdomain.MergeStrategy `json:"mergeStrategy"`
	CanMergeLocally bool                    `json:"canMergeLocally"`
	ParentBranch    string                  `json:"parentBranch,omitempty"`
	PRUrl           string                  `json:"prUrl,omitempty"`
	PRTitle         string                  `json:"prTitle,omitempty"`
	PRTargetBranch  string                  `json:"prTargetBranch,omitempty"`
}
- // dto/thread.go — new (spec §5)
type ThreadReplyDTO struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"threadId"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
}
type ThreadDTO struct {
	ID          string           `json:"id"`
	ProjectID   string           `json:"projectId"`
	RepoID      string           `json:"repoId"`
	WorkspaceID string           `json:"workspaceId"`
	FilePath    string           `json:"filePath"`
	Line        int              `json:"line"`
	Side        string           `json:"side"`
	Body        string           `json:"body"`
	Author      string           `json:"author"`
	Resolved    bool             `json:"resolved"`
	CreatedAt   time.Time        `json:"createdAt"`
	Replies     []ThreadReplyDTO `json:"replies"`
}
- // dto/terminal.go — TerminalSessionDTO lifecycle payload (spec §5; PTY stream is separate, NOT a Broadcaster[T])
type TerminalSessionDTO struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"projectId"`
	RepoID      string     `json:"repoId"`
	WorkspaceID string     `json:"workspaceId"`
	ProfileID   string     `json:"profileId"`
	CreatedAt   time.Time  `json:"createdAt"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
}
- // usecase merge eligibility (spec §10), called wherever a WorkspaceDTO is produced
func MergeEligibilityFor(
	ws domain.Workspace,
	siblings []domain.Workspace,
) (canMerge bool, parentBranch string)
- // Route paths the broadcasters serve (spec §3):
GET/WS /v0/projects
GET/WS /v0/projects/:projectId
GET/WS /v0/projects/:projectId/repos
GET/WS /v0/projects/:projectId/repos/:repoId
GET/WS /v0/projects/:projectId/repos/:repoId/workspaces
GET/WS /v0/projects/:projectId/repos/:repoId/workspaces/:wsId
GET/WS /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/status
WS     /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/files/ws
WS     /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/lsp/ws
GET/WS /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads
GET/WS /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals
WS     /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals/:sessionId/ws (raw PTY, NOT a Broadcaster[T])

### Risks
- PREFIX MATCH IS NOT EXPRESSIBLE TODAY: filter.go GlobMatch uses path.Match, which does NOT prefix-match and treats '/' as a segment separator. A repo-scoped client subscribing to prefix 'p/r' cannot match 'p/r/w'. Spec §5 prefix model REQUIRES a new matcher; reusing the :ns glob will silently drop workspace events to repo-scoped subscribers. Highest-risk gap.
- SHARED INTERFACE WIDENING (hub.Subscriber + hub.WebSocketHub): adding PushProject/PushRepo/PushThread/PushTerminalSession and removing PushChat forces lockstep edits to v0.Container AND every fake/mock implementing Subscriber (hub_test.go, snapshots_internal_test fakes, integration kit oracles). Any missed implementer breaks compilation across packages.
- BROADCASTER EMITS DOMAIN TYPE, NOT DTO: workspacesDef serializes domain.Workspace while REST serializes dto.WorkspaceDTO — two divergent wire shapes for the same entity. Switching the broadcaster to dto.WorkspaceDTO (correct per §5) changes every WS consumer/test asserting the domain shape (broadcaster_test, integration websocket tests, frontend ws stores) and REMOVES fields (Locked/HasConflicts/AgentRunning/PendingMerge) the frontend currently reads.
- SNAPSHOT SIGNATURE LACKS SCOPE: StreamDef.Snapshot is func()[]T and snapshotFor (broadcaster.go:143) calls it with no scope; the per-client predicate filters the FULL set. Under spec §9 lazy per-entity storage a global enumeration opens every view.db (LRU cap 64) — risks handle thrashing and slow first-connect. Snapshot must take the connecting client's scope, a signature change rippling through every *Def and every snapshot test.
- SNAPSHOT VS LAZY STORAGE: gitSnapshot/lspSnapshot/workspacesSnapshot enumerate ALL workspaces via Workspace.List(ctx) / ListWorkspacesWithOverlay. These global reads are incompatible with the new hierarchical lazy-open storage and must be rescoped to the subscribed subtree.
- SNAPSHOT RACE INVARIANT must be preserved: broadcaster.go register-then-snapshot-outside-lock relies on idempotent full-state replace (double-delivery harmless). New DTOs with computed CanMergeLocally must stay idempotent full replaces; status:'deleted' tombstones (§6) must be full-DTO frames, not a separate delete type, to keep one-type-per-channel.
- CASCADE DELETE / TOMBSTONE ORDERING: deleting a workspace (rm -rf, §1) must emit WorkspaceDTO{status:'deleted'} BEFORE storage teardown; if the lazy DB handle is closed/evicted before the broadcast, a late subscriber's snapshot and the delete frame can race. Deleting a repo/project must cascade tombstones for child workspaces/threads/terminals — no cascade broadcast exists today.
- CanMergeLocally/ParentBranch (§10) are computed from the SIBLING set, but Push receives ONE DTO without the sibling list. Computing them in the broadcaster Serialize means a DB sibling read inside the Push hot path under RLock (perf + lock hazard). Must be resolved in the usecase before Push, never in the broadcaster.
- CHAT/AGENTRUN REMOVAL FANOUT: chatsDef/chatStreamDef/chatsSnapshot/BroadcastChat/PushChat/ChatStatusEvent/ChatFrame + chats.Register + agentrun.Register are interlinked across container.go, snapshots.go, router.go, hub/*, and frontend (chat-list-store.ts, run.ts). Frontend still references flat /v0/ws/chats and /v0/ws/workspaces — these break the moment routes go hierarchical.
- DUAL HELPER DRIFT: ws.Dispatch (Broadcaster+snapshot func) and ws.DualServe (two handlers) both exist with duplicate upgrade detection; only DualServe is wired. Consolidating to one avoids divergence but each has its own test that must be updated/removed.
- PER-CONNECTION PROVIDER POLL (§11) must start in the workspace WS upgrade handler and cancel on disconnect. Broadcaster.Handle only fires OnSubscribe/OnUnsubscribe scope hooks; there is no per-connection cancelable context, so a naive poll leaks on disconnect. Wiring it requires extending the lifecycle to carry a cancelable context tied to readPump returning.
- NET-NEW ENTITIES MISSING: spec §5 assumes Broadcaster[ProjectDTO]/[ThreadDTO]/[TerminalSessionDTO] but there is no ProjectDTO broadcaster wiring, no threads endpoint package, and terminal sessions are not a persisted GORM table today (engine-created, no view.db). The spec storage tables (threads, thread_replies, terminal_sessions §2) do not exist yet — these are dependencies, not just transport changes.

### Test targets
- api/internal/api/v0/ws/filter_test.go — add TestPrefixMatch_* (exact p/r/w; parent prefix p/r matches p/r/w; p/r does NOT match p/r2/w; project prefix p matches p/r/w; empty prefix matches all) and TestBuildPredicate_HierarchicalScope_* for project/repo/workspace-scoped clients.
- api/internal/api/v0/ws/broadcaster_test.go — add TestBroadcaster_PrefixNamespace_RepoScopedReceivesChildren (p/r prefix receives p/r/w1+p/r/w2 but not p/r2/w); TestBroadcaster_WorkspaceDTO_OneTypePerChannel; keep snapshot-before-live / no-truncation / concurrent-push-no-block against the new scoped Snapshot signature. Synchronize via b.WaitNRegistered(n), never time.Sleep.
- api/internal/api/v0/ws/broadcaster_bench_test.go + snapshot_bench_test.go — keep BenchmarkBroadcaster_PushFanOut; add BenchmarkBroadcaster_PrefixNamespaceMatch (prefix predicate cost at fan-out) and rescope BenchmarkBroadcaster_SnapshotOnSubscribe to the scoped Snapshot (§13 fan-out + DB registry benches).
- api/internal/api/v0/container_test.go + container_test_hooks.go — update for new broadcaster fields/Push* methods; add TestContainer_PushWorkspaceDTO_RoutesByPrefix, TestContainer_PushRepo/PushProject/PushThread/PushTerminalSession route to correct broadcaster. Remove chat/agentrun container tests.
- api/internal/api/v0/snapshots_internal_test.go + snapshots_test.go — rescope to per-entity: TestWorkspacesSnapshot_ScopedToRepo, TestWorkspacesSnapshot_ComputesCanMergeLocally (parent present/absent/locked/deleted per §10), TestRepoSnapshot/ProjectSnapshot/ThreadSnapshot; keep list-error-returns-nil pattern. Remove chatsSnapshot tests.
- api/internal/app/hub/hub_test.go — TestHub_BroadcastProject/Repo/Workspace/Thread/TerminalSession fan out to registered subscribers; remove BroadcastChat test; assert var _ WebSocketHub conformance with widened interface.
- Unit (usecase) MergeEligibilityFor _test.go — TestMergeEligibilityFor: no parent->false; parent locked->false; parent deleted->false; parent active->true + parentBranch (§10).
- api/tests/integration/websocket/websocket_test.go (build tag integration) — TestRegression_WorkspaceWS_NamespaceFiltering_ProjectRepoWsScopes: open WS at /v0/projects/:p/repos/:r/workspaces and assert only that repo's WorkspaceDTOs arrive; ws-scoped sees only :wsId; block on conn.ReadMessage with context deadline, NO time.Sleep.
- api/tests/regressions_test.go — TestRegression_CreateWorkspace_202_ThenWorkspaceDTOOnWS (202 then WS WorkspaceDTO{status:new} then ready); TestRegression_DeleteWorkspace_202_ThenDeletedTombstone (WS status:deleted); TestRegression_GitPush_202_ThenLastErrorOrStatus; TestRegression_RepoIcon_HierarchicalProxyURL; TestRegression_ProviderPoll_PrOpenToPrMerged (mock provider). Use harness.dial + registration waiters.
- api/tests/integration/terminal/ — TestRegression_TerminalSessionLifecycleWS: POST .../terminals 201 -> Broadcaster[TerminalSessionDTO] emits created; DELETE -> emits ended tombstone; distinct from the raw PTY /terminals/:sessionId/ws byte stream.
- Frontend web/src/__tests__ — update workspace-list / chat-list store tests for hierarchical WS endpoints; remove flat /v0/ws/workspaces and /v0/ws/chats assertions (crowbar-bridge.ts, workspace-list.ts, chat-list-store.ts).

---

## Workspace domain / status / DTO + workspace usecase + merge eligibility + provider sync

### Key signatures
- `api/internal/domain/workspace.go`
  - type Workspace struct (workspace.go:11)
  - Status WorkspaceStatus `json:"status,omitempty"` (workspace.go:19)
  - Locked bool `json:"locked"` (workspace.go:20)
  - HasConflicts bool `json:"hasConflicts"` (workspace.go:21)
  - PendingMerge *gitdomain.PendingMerge `json:"pendingMerge,omitempty"` (workspace.go:23)
  - AgentRunning bool `json:"agentRunning"` (workspace.go:34)
  - ParentID string `json:"parentId,omitempty"` (workspace.go:18)
  - LastActivity time.Time / CreatedAt time.Time (workspace.go:29-30)
- `api/internal/domain/workspace_status.go`
  - type WorkspaceStatus string (workspace_status.go:4)
  - WorkspaceStatusNew = "new" (workspace_status.go:7)
  - WorkspaceStatusPROpen = "pr-open" (workspace_status.go:8)
  - WorkspaceStatusPRMerged = "pr-merged" (workspace_status.go:9)
  - WorkspaceStatusPRClosed = "pr-closed" (workspace_status.go:10)
- `api/internal/api/v0/dto/workspace.go`
  - type WorkspaceDTO struct (workspace.go:12)
  - Locked bool / HasConflicts bool / AgentRunning bool / PendingMerge *gitdomain.PendingMerge (workspace.go:21,22,29,30)
  - func WorkspaceDTOFrom(w domain.Workspace) WorkspaceDTO (workspace.go:34)
  - func WorkspaceDTOList(workspaces []domain.Workspace) []WorkspaceDTO (workspace.go:62)
- `api/internal/app/usecases/workspace/workspace.go`
  - type Usecase interface (workspace.go:55) — List, Get, SetMergeStrategy, SyncWorkingTreeState (NO MergeEligibilityFor)
  - func (u *workspaceUsecase) MergeEligibilityFor(ws domain.Workspace, siblings []domain.Workspace) MergeEligibility (workspace.go:165)
  - CanMergeLocally: !s.Locked (workspace.go:175)
  - type WorkspaceLifecycleRepo interface (workspace.go:14)
  - func (u *workspaceUsecase) summarize(...) (worksp.SyncInput, error) (workspace.go:183)
- `api/internal/app/usecases/provider/provider_sync.go`
  - type WorkspaceRepo interface (provider_sync.go:14) — Get, SyncProviderState, List, SetParentFromPR
  - func (u *providerSyncUsecase) PollWorkspace(ctx, wsID string) error (provider_sync.go:76)
  - func (u *providerSyncUsecase) SyncFromState(ctx, wsID string, state engineprovider.ProviderState, now time.Time) error (provider_sync.go:94)
  - in := workspace.ProviderInput{ID: wsID, Protected: state.Protected} (provider_sync.go:106)
  - func (u *providerSyncUsecase) maybeReparentFromPR(...) (provider_sync.go:131)
- `api/internal/domain/git/merge.go`
  - type MergeStrategy string (merge.go:4) — merge|squash|rebase
  - type PendingMerge struct { Strategy MergeStrategy; TargetParentID string } (merge.go:13)
- `api/internal/app/repositories/workspace/workspace.go`
  - type CreateInput struct { ...; Locked bool; ... } (workspace.go:18)
  - type SyncInput struct { ID; Added; Deleted; HasConflicts bool; HasCommits bool } (workspace.go:31)
  - type ProviderInput struct { ID; Protected bool; HasPR; PRStatus; PRUrl; PRTitle; PRTargetBranch } (workspace.go:40)
  - SetPendingMerge / ClearPendingMerge methods (workspace.go:267,284)
  - SetParentFromPR (workspace.go:295)
- `api/internal/app/repositories/workspace/internal/commands/create.go`
  - type CreateWorkspace struct { ...; Locked bool; ... } (create.go:14)
  - EmitEvent sets Status: domain.WorkspaceStatusNew, Locked: c.Locked (create.go:66-67)
- `api/internal/app/repositories/workspace/internal/commands/sync_working_tree_state.go`
  - type SyncWorkingTreeState struct { ...; HasConflicts bool; HasCommits bool; ... } (sync_working_tree_state.go:15)
  - EmitEvent: ws.HasConflicts = c.HasConflicts (line 51); if ws.Status==new && HasCommits -> ws.Status="" (line 53-55)
- `api/internal/app/repositories/workspace/internal/commands/sync_provider_state.go`
  - EmitEvent: ws.Locked = c.Protected (sync_provider_state.go ~line 50)
  - func prStatusToWorkspace(status string) domain.WorkspaceStatus — open/merged/closed -> pr-* (default pr-open)
- `api/internal/app/repositories/workspace/internal/store/storage.go`
  - type workspaceRow struct { ID string; Data []byte } (storage.go:14)
  - func (s *storageStore) Save(ctx, ws domain.Workspace) error — json.Marshal(ws) (storage.go:60)
- `api/internal/app/usecases/worktree/worktree.go`
  - path, err := worktreepath.For(home, in.RemoteURL, wsID) (worktree.go:121)
  - func (u *worktreeUsecase) guardMerge — if parent.Locked { return ErrParentLocked } (worktree.go:228)
  - handleMergeError -> SetPendingMerge(ctx, child.ID, strategy, parent.ID) on ErrConflict (worktree.go:284)
  - guardReparent — if newParent.Locked { return ErrNewParentLocked } (worktree.go:378)
  - DeleteCascade — if root.Locked { return ErrWorkspaceLocked } (worktree.go:404)
  - nodesFrom -> cascade.Node{Locked: ws.Locked} (worktree.go:472)
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go`
  - func For(crowbarHome, remoteURL, workspaceID string) (string, error) (worktreepath.go:20)
  - func RepoDir(crowbarHome, remoteURL string) (string, error) (worktreepath.go:33)
  - func repoRelPath(rawURL string) (string, error) (worktreepath.go:53)
- `api/internal/api/v0/endpoints/workspaces/handlers/list.go`
  - func (h *Handlers) List(c *gin.Context) — dto.WorkspaceDTOList(filtered) (list.go:14,28)
  - func (h *Handlers) Detail — dto.WorkspaceDTOFrom(ws) (list.go:32,41)
  - func filterWorkspaces(rows, projectID, repoID) (list.go:44)
- `api/internal/api/v0/container.go`
  - func (c *Container) PushWorkspace(wsRow domain.Workspace) — c.workspaces.Push(wsRow) (container.go:111)

### Must change
- `api/internal/domain/workspace_status.go` — §5: add WorkspaceStatusLocked="locked", WorkspaceStatusPRConflicts="pr-conflicts", WorkspaceStatusDeleted="deleted". Decide the non-new/non-PR idle state (spec drops the empty-string status; either introduce an explicit idle status or document that status stays at its last value). MergeEligibilityFor (§10) references WorkspaceStatusLocked and WorkspaceStatusDeleted which must exist here.
- `api/internal/domain/workspace.go` — §5: remove Locked, HasConflicts, PendingMerge, AgentRunning. Add LastError string and Working bool (Working always false until chat exists). These become the only non-status mutation/error carriers; status now encodes locked/conflicts/deleted.
- `api/internal/api/v0/dto/workspace.go` — §5: redefine WorkspaceDTO to the spec field list (add Working, LastError, CanMergeLocally, ParentBranch; drop Locked, HasConflicts, AgentRunning, PendingMerge; drop WorktreePath from wire — not in spec list). Change WorkspaceDTOFrom signature so it can populate CanMergeLocally/ParentBranch — it must take the computed eligibility (or take siblings) rather than a lone Workspace. WorkspaceDTOList must compute eligibility per row against the full slice.
- `api/internal/app/usecases/workspace/workspace.go` — §10: define the MergeEligibility type (currently undefined — package does not compile) and add MergeEligibilityFor to the Usecase interface. Fix the rule to exclude BOTH locked and deleted parents: eligible := s.Status != WorkspaceStatusLocked && s.Status != WorkspaceStatusDeleted (currently only !s.Locked). Update all sibling/parent guards that read .Locked to read Status==locked.
- `api/internal/app/usecases/provider/provider_sync.go` — §11/§5: ProviderInput no longer carries Locked semantics via Protected->Locked; Protected must drive Status=locked. Provider remains the only place Status transitions to pr-merged/pr-closed. SyncFromState/PollWorkspace otherwise unchanged but must set LastError on poll failure if the failure should surface (per §4 errors belong to the entity).
- `api/internal/domain/git/merge.go` — §5: remove PendingMerge struct (subsumed by status:pr-conflicts). Keep MergeStrategy. Removing PendingMerge cascades to set_pending_merge.go/clear_pending_merge.go and the worktree merge-error path.
- `api/internal/app/repositories/workspace/workspace.go` — §5: drop Locked from CreateInput, HasConflicts from SyncInput, and the Protected->Locked mapping in ProviderInput handling; remove SetPendingMerge/ClearPendingMerge methods. Add a command/method to set Status=locked and to set LastError (or extend an existing command).
- `api/internal/app/repositories/workspace/internal/commands/create.go` — §5: replace Locked input with Status seeding (protected -> WorkspaceStatusLocked, else WorkspaceStatusNew). Remove the Locked field from CreateWorkspace.
- `api/internal/app/repositories/workspace/internal/commands/sync_working_tree_state.go` — §5: remove HasConflicts assignment; when local conflicts are detected transition Status to pr-conflicts instead of setting a bool. Remove the new->"" empty-string transition (empty status removed).
- `api/internal/app/repositories/workspace/internal/commands/sync_provider_state.go` — §11/§5: stop writing ws.Locked; map Protected to Status=locked. prStatusToWorkspace stays (open/merged/closed -> pr-*).
- `api/internal/app/usecases/worktree/worktree.go` — §8: switch worktreepath.For to the 4-arg UUID signature (home, projectID, repoID, wsID) with no error. §5: replace parent.Locked/newParent.Locked/root.Locked guards with Status==locked checks; replace handleMergeError SetPendingMerge with a Status=pr-conflicts transition; update nodesFrom cascade.Node.Locked to derive from Status==locked. RemoteURL no longer needed for path derivation.
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go` — §8: rewrite For(crowbarHome, projectID, repoID, workspaceID) string (no error) returning .../projects/<P>/<R>/workspaces/<W>/worktree; add StorageDir, RepoDir(P,R), ProjectDir(P) helpers; remove remoteURL/host parsing (repoRelPath).
- `api/internal/api/v0/endpoints/workspaces/handlers/list.go` — §10: List must compute MergeEligibilityFor per row against the filtered slice and feed it into the DTO; Detail must load siblings (same repo) to compute eligibility for the single row. WorkspaceDTOFrom/List call sites change to the new eligibility-aware converters.
- `api/internal/api/v0/container.go` — §5/§10: PushWorkspace must convert domain.Workspace to WorkspaceDTO with CanMergeLocally/ParentBranch resolved against the current workspace set (requires a sibling lookup at broadcast time, e.g. via the workspace usecase List) before c.workspaces.Push — the broadcaster now emits WorkspaceDTO, not raw domain.Workspace.

### New contracts
- // domain/workspace_status.go
const (
    WorkspaceStatusNew         WorkspaceStatus = "new"
    WorkspaceStatusLocked      WorkspaceStatus = "locked"
    WorkspaceStatusPRConflicts WorkspaceStatus = "pr-conflicts"
    WorkspaceStatusDeleted     WorkspaceStatus = "deleted"
    WorkspaceStatusPROpen      WorkspaceStatus = "pr-open"
    WorkspaceStatusPRMerged    WorkspaceStatus = "pr-merged"
    WorkspaceStatusPRClosed    WorkspaceStatus = "pr-closed"
)
- // domain/workspace.go — fields removed: Locked, HasConflicts, PendingMerge, AgentRunning; added:
    Status    WorkspaceStatus `json:"status,omitempty"`
    Working   bool            `json:"working"`
    LastError string          `json:"lastError,omitempty"`
- // api/v0/dto/workspace.go
type WorkspaceDTO struct {
    ID              string                  `json:"id"`
    RepoID          string                  `json:"repoId"`
    ProjectID       string                  `json:"projectId"`
    Branch          string                  `json:"branch"`
    ParentID        string                  `json:"parentId,omitempty"`
    ForkPointSha    string                  `json:"forkPointSha,omitempty"`
    Status          domain.WorkspaceStatus  `json:"status,omitempty"`
    Working         bool                    `json:"working"`
    LastError       string                  `json:"lastError,omitempty"`
    Added           int                     `json:"added"`
    Deleted         int                     `json:"deleted"`
    MergeStrategy   gitdomain.MergeStrategy `json:"mergeStrategy"`
    CanMergeLocally bool                    `json:"canMergeLocally"`
    ParentBranch    string                  `json:"parentBranch,omitempty"`
    PRUrl           string                  `json:"prUrl,omitempty"`
    PRTitle         string                  `json:"prTitle,omitempty"`
    PRTargetBranch  string                  `json:"prTargetBranch,omitempty"`
}
- // api/v0/dto/workspace.go — eligibility-aware converters
func WorkspaceDTOFrom(
    w domain.Workspace,
    elig workspace.MergeEligibility,
) WorkspaceDTO

func WorkspaceDTOList(
    workspaces []domain.Workspace,
    elig func(domain.Workspace) workspace.MergeEligibility,
) []WorkspaceDTO
- // app/usecases/workspace/workspace.go — currently undefined; must be declared
type MergeEligibility struct {
    CanMergeLocally bool
    ParentBranch    string
}

// added to the Usecase interface
MergeEligibilityFor(
    ws domain.Workspace,
    siblings []domain.Workspace,
) MergeEligibility
- // app/usecases/workspace/workspace.go — corrected rule body (§10)
func (u *workspaceUsecase) MergeEligibilityFor(
    ws domain.Workspace,
    siblings []domain.Workspace,
) MergeEligibility {
    if ws.ParentID == "" {
        return MergeEligibility{}
    }
    for _, s := range siblings {
        if s.ID == ws.ParentID {
            eligible := s.Status != domain.WorkspaceStatusLocked &&
                s.Status != domain.WorkspaceStatusDeleted
            return MergeEligibility{CanMergeLocally: eligible, ParentBranch: s.Branch}
        }
    }
    return MergeEligibility{}
}
- // app/usecases/internal/worktreepath/worktreepath.go (§8)
func For(
    crowbarHome string,
    projectID string,
    repoID string,
    workspaceID string,
) string

func StorageDir(
    crowbarHome string,
    projectID string,
    repoID string,
    workspaceID string,
) string

func RepoDir(
    crowbarHome string,
    projectID string,
    repoID string,
) string

func ProjectDir(
    crowbarHome string,
    projectID string,
) string
- // app/repositories/workspace/workspace.go — input struct changes
type CreateInput struct { // Locked removed; status seeded from Protected
    ID, RepoID, ProjectID, Branch, WorktreePath, ForkPointSha, ParentID string
    Protected     bool
    MergeStrategy gitdomain.MergeStrategy
}
type SyncInput struct { // HasConflicts removed
    ID string; Added int; Deleted int; HasCommits bool
}
- // Concrete hierarchical route paths this subsystem serves (§3)
GET  /v0/projects/:projectId/repos/:repoId/workspaces
POST /v0/projects/:projectId/repos/:repoId/workspaces            // body { branch, parentId? } -> 202
GET  /v0/projects/:projectId/repos/:repoId/workspaces/:wsId
WS   /v0/projects/:projectId/repos/:repoId/workspaces           // Broadcaster[WorkspaceDTO], prefix p/r/
WS   /v0/projects/:projectId/repos/:repoId/workspaces/:wsId      // Broadcaster[WorkspaceDTO], key p/r/w
DELETE /v0/projects/:projectId/repos/:repoId/workspaces/:wsId    // 202 -> WS WorkspaceDTO{status:deleted}
POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/merge-into-parent  // 202
POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/reparent          // 202

### Risks
- COMPILE BLOCKER (pre-existing): api/internal/app/usecases/workspace/workspace.go does not build — MergeEligibility is undefined and MergeEligibilityFor is not in the Usecase interface. The refactor must fix this first or the whole api module fails to build; any test run is currently red.
- MergeEligibility computation needs siblings, but the broadcast path (container.go PushWorkspace) only carries a single domain.Workspace. To populate CanMergeLocally/ParentBranch on the WS DTO, the broadcaster must fetch the current workspace set per push — risk of a snapshot race (a sibling's status changes between the trigger and the List, or two concurrent pushes see different sets). Spec §5 says fan-out emits WorkspaceDTO; decide where eligibility is resolved (usecase List at GET time is easy; broadcast-time resolution needs a consistent siblings read).
- WorkspaceDTOFrom is called from 3 handler sites (list.go:28,41; sync.go:24; hierarchy.go:87). Changing its signature to be eligibility-aware touches all of them plus the broadcast converter; the snapshots_test.go and dto/workspace_test.go encode the old field set and will break.
- domain.Workspace is JSON-serialized whole into read_workspaces (storage.go). Removing Locked/HasConflicts/PendingMerge/AgentRunning and adding LastError/Working changes every stored blob; pre-production wipe (Open Q1) is required — stale dev IndexedDB and ~/.crowbar/state must be cleared or unmarshal silently yields zero-value fields.
- worktreepath.For losing its error return and gaining projectID/repoID changes the worktree usecase call site (worktree.go:121) AND drops RemoteURL usage in CreateChildInput — but RemoteURL is still threaded through buildCreateInput (crud.go:83); the create path must stop requiring RemoteURL. The adoptMainWorktree branch (worktree.go:113) uses RepoPath directly and is unaffected by the path-derivation change but still sets Locked.
- Removing PendingMerge cascades to set_pending_merge.go, clear_pending_merge.go, their commands_test.go, branch_review_test.go, worktree handleMergeError, and the worktree_integration/_test files. The merge-conflict flow must transition Status=pr-conflicts and be resolvable back out — there is no existing command to clear pr-conflicts, so a new transition is needed (spec does not specify the un-conflict command).
- Removing Locked touches a wide surface: cascade.Node.Locked (delete-plan exclusion), worktree guards (merge/reparent/delete), git.go, file.go, search.go, project_import/project_delete. All must switch to Status==locked; cascade delete behavior (locked rows skipped) must be preserved via status.
- Provider sync currently maps Protected->Locked. Spec moves Protected to Status=locked, but Status is also the PR-status field — a protected branch WITH an open PR creates a conflict: should it be locked or pr-open? Spec §5 lists them as mutually exclusive enum values; the precedence rule is unspecified (risk: provider poll overwriting locked with pr-open or vice versa).
- The empty-string 'has commits, no PR' status is removed by spec but sync_working_tree_state.go transitions new->"". No replacement idle status is defined in the spec enum, so a workspace with commits but no PR has no clear status value — needs an explicit decision (e.g. keep new, or add an idle constant) or downstream UI badge logic breaks.
- Spec WorkspaceDTO omits WorktreePath, but the frontend/editor likely needs the worktree path for file ops; dropping it from the DTO (current field workspace.go:14) may break editor file routing — verify the frontend does not read worktreePath off the workspace DTO before removing it.

### Test targets
- api/internal/domain/workspace_status_test.go (new): assert all 7 status constants equal their wire strings (new, locked, pr-conflicts, deleted, pr-open, pr-merged, pr-closed).
- api/internal/app/usecases/workspace/workspace_test.go (exists, must extend): TestMergeEligibilityFor_NoParent (empty ParentID -> {}), _ParentLocked (parent Status=locked -> CanMergeLocally false), _ParentDeleted (parent Status=deleted -> false), _ParentIdle (-> true + ParentBranch), _ParentMissing (parent id not in siblings -> {}). Remove the old !s.Locked-based assertions.
- api/internal/api/v0/dto/workspace_test.go (exists, must rewrite): assert new DTO field set (Working, LastError, CanMergeLocally, ParentBranch present; Locked/HasConflicts/AgentRunning/PendingMerge absent) and JSON tag names; WorkspaceDTOFrom maps eligibility into the DTO.
- api/internal/app/usecases/internal/worktreepath/worktreepath_test.go (exists): For(home,p,r,w) returns .../projects/p/r/workspaces/w/worktree with no error; StorageDir/RepoDir/ProjectDir formulas; drop the remoteURL-parsing cases. worktreepath_bench_test.go: benchmark the pure-Join For (§13 path-construction bench).
- api/internal/app/repositories/workspace/internal/commands/commands_test.go (exists): CreateWorkspace seeds Status=locked when Protected, else new; SyncProviderState sets Status=locked from Protected and pr-* from PRStatus; SyncWorkingTreeState no longer sets HasConflicts and sets pr-conflicts on local conflict. Remove set_pending_merge/clear_pending_merge cases.
- api/internal/app/usecases/provider/provider_sync_test.go (mirror, if missing add): SyncFromState with Protected -> Status=locked; with PR open/merged/closed -> pr-*; auto-reparent unchanged; poll error path. Synchronize via Asynx WaitForState, no time.Sleep.
- api/internal/app/usecases/worktree/worktree_test.go + worktree_integration_test.go (exist): guards use Status==locked not .Locked; merge conflict transitions to pr-conflicts (not SetPendingMerge); reparent guard on locked parent; cascade delete skips locked-status rows; CreateChild uses new worktreepath.For (no RemoteURL).
- api/tests (integration tag, new TestRegression_*): TestRegression_WorkspaceCreate_202_ThenWsReadyDTO (POST workspaces -> 202, WS WorkspaceDTO{status:new} then ready); TestRegression_WorkspaceDelete_202_TombstoneDTO (DELETE -> 202, WS WorkspaceDTO{status:deleted}); TestRegression_WorkspacePush_202_LastErrorOrStatus; TestRegression_WorkspaceWS_NamespaceFiltering (project/repo/ws-scoped prefix match); TestRegression_ProviderPoll_PROpenToMerged (mock provider transition); TestRegression_MergeEligibility_TrueFalse (sibling locked/deleted vs idle); TestRegression_WorkspaceCreate_RemoteBranchExistsCheckoutVsCreate. All block on WS messages with a context deadline — no time.Sleep.

---

## Provider engine + polling (5-min global cron + 1-min per-active-WS-connection polling), spec §11

### Key signatures
- `api/internal/engine/provider/poll/poll.go`
  - poll.go:12 type SweepTarget struct { WSID string; RepoPath string; Branch string; HasOpenPR bool }
  - poll.go:20 type PRInfoSnapshot struct { Number int; Status string; URL string; Title string; TargetBranch string }
  - poll.go:29 type ProviderStateSnapshot struct { Protected bool; PR *PRInfoSnapshot }
  - poll.go:36 type PollFn func(ctx context.Context, wsID string, repoPath string, branch string) (ProviderStateSnapshot, error)
  - poll.go:45 type OnStateChangeFn func(wsID string, state ProviderStateSnapshot)
  - poll.go:60 type Sweeper interface { Start(ctx, workspacesFn func() []SweepTarget) }
  - poll.go:70 func NewSweeper(pollFn PollFn, onStateChange OnStateChangeFn) Sweeper // 60s
  - poll.go:79 func newSweeperWithInterval(pollFn, onStateChange, interval time.Duration) Sweeper
  - poll.go:119 func (s *sweeper) sweepOnce(ctx, targets []SweepTarget)
  - poll.go:129 func (s *sweeper) sweepTarget(ctx, t SweepTarget)
  - poll.go:155 func statesEqual(a, b ProviderStateSnapshot) bool
- `api/internal/engine/provider/engine.go`
  - engine.go:15 type providerEngine struct { detectFn ...; providerFac func(kind string) GitProvider }
  - engine.go:20 func newEngine() *providerEngine
  - engine.go:88 func (e *providerEngine) PollOnView(ctx, _ string, repoPath, branch string) (ProviderState, error)
  - engine.go:108 func (e *providerEngine) StartBackgroundSweep(ctx, workspacesFn func() []poll.SweepTarget, onStateChange func(wsID string, state ProviderState))
  - engine.go:127 func (e *providerEngine) sweepPollFn() poll.PollFn
  - engine.go:146 func (e *providerEngine) providerFor(kind string) GitProvider
  - engine.go:156 func defaultProviderFor(kind string) GitProvider // github.New()/gitlab.New()
  - engine.go:172 func pollOnce(ctx, prov GitProvider, repoPath, branch string) (ProviderState, error)
- `api/internal/engine/provider/provider.go`
  - provider.go:14 type PRInfo = providertypes.PRInfo
  - provider.go:16 type ProviderState = providertypes.ProviderState
  - provider.go:18 type ProviderCapability = providertypes.ProviderCapability
  - provider.go:23 type GitProvider interface { ProtectedBranches; PullRequestForBranch; OwnerAvatarURL }
  - provider.go:47 type Engine interface { Capability; ProtectedBranches; PollOnView; StartBackgroundSweep; OwnerAvatarURL }
  - provider.go:84 func New() Engine
- `api/internal/engine/provider/detect.go`
  - detect.go:12 type DetectExecFn func(ctx, name string, args ...string) *exec.Cmd
  - detect.go:15 type DetectResult struct { Kind string; Enabled bool }
  - detect.go:21 var DefaultProtectedBranches = []string{"main","develop","master"}
  - detect.go:34 func Detect(ctx, repoPath string) (DetectResult, error)
  - detect.go:43 func DetectWithExec(ctx, repoPath string, execFn DetectExecFn) (DetectResult, error)
- `api/internal/engine/provider/providers/github/github.go`
  - github.go:18 type ghProvider struct { execFn ExecFn }
  - github.go:23 func New() *ghProvider
  - github.go:72 func (g *ghProvider) PullRequestForBranch(ctx, repoPath, branch string) (*providertypes.PRInfo, error)
  - github.go:169 func mapState(state string) string // OPEN->open, MERGED->merged, else closed
  - github.go:223 func (g *ghProvider) OwnerAvatarURL(ctx, repoPath string) (string, error)
- `api/internal/engine/provider/providers/github/slug.go`
  - slug.go:11 func slug(ctx, repoPath string, execFn ExecFn) (string, error)
  - slug.go:35 func slugFromURL(url string) (string, error)
- `api/internal/engine/provider/providers/gitlab/gitlab.go`
  - gitlab.go:18 type glabProvider struct { execFn ExecFn }
  - gitlab.go:23 func New() *glabProvider
  - gitlab.go:65 func (g *glabProvider) PullRequestForBranch(ctx, repoPath, branch string) (*providertypes.PRInfo, error)
  - gitlab.go:162 func mapState(state string) string // opened->open, merged->merged, else closed
- `api/internal/engine/provider/types/types.go`
  - types.go:7 type PRInfo struct { Number int `json:"number"`; Status string `json:"status"`; URL string `json:"url"`; Title string `json:"title"`; TargetBranch string `json:"targetBranch"` }
  - types.go:16 type ProviderState struct { Protected bool `json:"protected"`; PR *PRInfo `json:"pr,omitempty"` }
  - types.go:22 type ProviderCapability struct { Kind string; Enabled bool }
- `api/internal/api/v0/endpoints/provider/handlers/handlers.go`
  - handlers.go:13 type ProviderEngine interface { PollOnView(ctx, wsID, repoPath, branch string) (provider.ProviderState, error); ProtectedBranches(ctx, repoPath string) ([]string, error) }
  - handlers.go:30 type WorkspaceReader interface { Get(ctx, id string) (domain.Workspace, error) }
  - handlers.go:46 func New(eng ProviderEngine, wsReader WorkspaceReader) *Handlers
- `api/internal/api/v0/endpoints/provider/handlers/provider.go`
  - provider.go:18 func (h *Handlers) State(ctx *gin.Context) // GET .../provider
  - provider.go:53 func (h *Handlers) ProtectedBranches(ctx *gin.Context) // GET /v0/repos/:id/protected-branches
- `api/internal/api/v0/endpoints/provider/routes.go`
  - routes.go:12 func Register(rg *gin.RouterGroup, provEng provhandlers.ProviderEngine, wsReader provhandlers.WorkspaceReader)
- `api/internal/api/v0/dto/provider.go`
  - provider.go:10 type PRInfoDTO struct { Number int; Status string; URL string; Title string; TargetBranch string }
  - provider.go:21 type ProviderStateDTO struct { Protected bool; PR *PRInfoDTO }
  - provider.go:28 func ProviderStateDTOFrom(state providertypes.ProviderState) ProviderStateDTO
- `api/internal/app/container.go`
  - container.go:115 func startProviderSweep(ctx, engines *engine.Container, repos *repositories.Container, ucs *usecases.Container)
  - container.go:128 func sweepCallback(ctx, ucs *usecases.Container) func(wsID string, state provider.ProviderState)
  - container.go:148 func sweepTargets(repo workspace.Workspace) func() []poll.SweepTarget
- `api/internal/app/usecases/provider/provider_sync.go`
  - provider_sync.go:41 type Usecase interface { PollWorkspace(ctx, wsID string) error; SyncFromState(ctx, wsID string, state engineprovider.ProviderState, now time.Time) error }
  - provider_sync.go:76 func (u *providerSyncUsecase) PollWorkspace(ctx, wsID string) error
  - provider_sync.go:94 func (u *providerSyncUsecase) SyncFromState(ctx, wsID, state, now) error
- `api/internal/app/repositories/workspace/internal/commands/sync_provider_state.go`
  - sync_provider_state.go:15 type SyncProviderState struct { ID, Protected, HasPR, PRStatus, PRUrl, PRTitle, PRTargetBranch, Now }
  - sync_provider_state.go:47 func (c SyncProviderState) EmitEvent(current *domain.Workspace) domain.Workspace
  - sync_provider_state.go:62 func prStatusToWorkspace(status string) domain.WorkspaceStatus
- `api/internal/domain/workspace_status.go`
  - workspace_status.go:6 const ( WorkspaceStatusNew="new"; WorkspaceStatusPROpen="pr-open"; WorkspaceStatusPRMerged="pr-merged"; WorkspaceStatusPRClosed="pr-closed" )
- `api/internal/api/v0/container.go`
  - container.go:76 func withWatcherLifecycle[T any](def ws.StreamDef[T], appContainer *app.Container) ws.StreamDef[T]
  - container.go:101 func scopeWsID(c *gin.Context) string
  - container.go:145 func workspacesDef(appContainer *app.Container) ws.StreamDef[domain.Workspace]
- `api/internal/app/realtime/watcher_manager.go`
  - watcher_manager.go:29 type WatcherManager struct { root; factory; mu; handles map[string]*watcherHandle; closed bool }
  - watcher_manager.go:77 func (m *WatcherManager) Acquire(wsID string)
  - watcher_manager.go:109 func (m *WatcherManager) Release(wsID string)
  - watcher_manager.go:137 func (m *WatcherManager) StopAll()
- `api/internal/app/realtime/service.go`
  - service.go:38 func New(ctx, h *hub.Hub, workspace workspacerepo.Workspace, gitEngine, fsEngine, lspLifecycle LSPLifecycle, now func() time.Time) *Service
  - service.go:57 func (s *Service) AcquireWatcher(wsID string)
  - service.go:91 func (s *Service) Close()

### Must change
- `api/internal/engine/provider/poll/poll.go` — §11: introduce an explicit two-tier interval. The global cron must tick every 5min (replace the 60s NewSweeper default, or add NewCron(interval=5*time.Minute)). Keep newSweeperWithInterval as the test seam. The per-connection tier needs a single-workspace 1-min ticking primitive (a thin loop calling PollFn for one fixed SweepTarget) OR can live entirely in the app-layer ProviderPollManager without touching this file. Recommended: rename/retune so the cron interval is 5min; do NOT delete the configurable-interval constructor (tests depend on it). Sweep dedup (lastState/statesEqual) and HasOpenPR gating stay as-is (spec §12: sweep logic unchanged).
- `api/internal/engine/provider/engine.go` — §11: StartBackgroundSweep currently fixes 60s via poll.NewSweeper. Change it (or its caller) to use the 5-minute global cron interval. PollOnView already supports the single-workspace immediate poll the per-connection tier needs — keep it; its unused wsID param (engine.go:88, named _) is fine. Update the doc comment that says '60s background sweep' to '5-minute global cron'.
- `api/internal/engine/provider/provider.go` — §11: update the Engine interface doc for StartBackgroundSweep to reflect 5-minute cron semantics. If the per-connection poller calls the engine directly, no new engine method is strictly required (PollOnView suffices). If the design prefers a typed start method, add StartGlobalCron with an explicit interval — but prefer reusing StartBackgroundSweep to minimize churn (spec §12 keeps engine surface stable).
- `api/internal/app/container.go` — §11: rename/retarget startProviderSweep so the global sweep ticks every 5 minutes instead of 60s (the interval is decided in engine.StartBackgroundSweep / poll). sweepTargets must continue to enumerate workspaces with a PR; under spec §11 the cron scopes to 'all workspaces with a PR URL', so widen the filter from Status==pr-open to 'PRUrl != ""' (or Status in pr-open|pr-conflicts) so the cron can observe pr-open->pr-merged/pr-closed transitions on workspaces nobody is watching. RepoPath currently = ws.WorktreePath; after §1/§8 the worktree path becomes <home>/projects/<P>/<R>/workspaces/<W>/worktree, so confirm SweepTarget.RepoPath still points at a valid git dir for gh/glab CLI invocation.
- `api/internal/app/realtime/service.go` — §11: add AcquireProviderPoll(wsID)/ReleaseProviderPoll(wsID) backed by a new ProviderPollManager (mirroring WatcherManager) that, on 0->1 for a wsId, starts a 1-min ticker calling ucs.ProviderSync.PollWorkspace(ctx, wsID), and on 1->0 cancels it. Thread the ProviderSync usecase + a now/clock into New(). Wire StopAll into Close().
- `api/internal/api/v0/container.go` — §11: add withProviderPollLifecycle wrapper (analogous to withWatcherLifecycle) and apply it to the workspace-scoped broadcaster def used for the /workspaces/:wsId WS route, with ScopeKey=scopeWsID, OnSubscribe=Realtime.AcquireProviderPoll, OnUnsubscribe=Realtime.ReleaseProviderPoll. CAUTION: only the single-workspace (:wsId) subscription should start a poll — the list-scope (/workspaces with projectId/repoId filter) must NOT, since scopeWsID returns "" there and the manager already no-ops on blank wsId.
- `api/internal/api/v0/endpoints/provider/routes.go` — §3: replace the flat routes /workspaces/:wsId/provider and /repos/:id/protected-branches with hierarchical /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/provider and /v0/projects/:projectId/repos/:repoId/protected-branches.
- `api/internal/api/v0/endpoints/provider/handlers/provider.go` — §3: State must resolve the workspace by the hierarchical (projectId,repoId,wsId) identity and emit dto.ProviderStateDTOFrom(state) rather than the raw engine type. ProtectedBranches must derive the repo root from the repo aggregate path (RepoDir = <home>/projects/<P>/<R>), not from a workspace worktree path — remove the Wave-3 'reuse workspace lookup' hack.
- `api/internal/api/v0/endpoints/provider/handlers/handlers.go` — §3: WorkspaceReader.Get and the ProviderEngine port may need project/repo context once IDs are UUID-hierarchical; at minimum the handler must pass the repo path (not worktree path) into ProtectedBranches.
- `api/internal/domain/workspace_status.go` — §5: add the missing status constants required by the new enum — WorkspaceStatusLocked="locked", WorkspaceStatusPRConflicts="pr-conflicts", WorkspaceStatusDeleted="deleted". The provider poll path needs a target to map provider-reported unresolvable conflicts to pr-conflicts.
- `api/internal/app/repositories/workspace/internal/commands/sync_provider_state.go` — §5/§11: reconcile EmitEvent with the new enum. Today it writes ws.Locked (bool) + a pr-* Status separately; spec §5 folds locked into Status and adds pr-conflicts. prStatusToWorkspace must additionally surface pr-conflicts when the provider reports unresolvable conflicts (requires a 'conflicts' signal threaded through PRInfo/ProviderInput, which does not exist yet). Decide whether Protected continues to set a separate Locked field or transitions Status='locked'.

### New contracts
- // api/internal/domain/workspace_status.go — add constants
const (
	WorkspaceStatusLocked       WorkspaceStatus = "locked"
	WorkspaceStatusPRConflicts  WorkspaceStatus = "pr-conflicts"
	WorkspaceStatusDeleted      WorkspaceStatus = "deleted"
)
- // api/internal/app/realtime/provider_poll_manager.go (new) — mirror of WatcherManager
type ProviderPollManager struct {
	root     context.Context
	interval time.Duration
	poll     ProviderPoller
	mu       sync.Mutex
	handles  map[string]*providerPollHandle
	closed   bool
}
- // ProviderPoller is the usecase surface the per-connection poll needs (satisfied by usecases/provider.Usecase)
type ProviderPoller interface {
	PollWorkspace(
		ctx context.Context,
		wsID string,
	) error
}
- func NewProviderPollManager(
	root context.Context,
	interval time.Duration,
	poll ProviderPoller,
) *ProviderPollManager
- func (m *ProviderPollManager) Acquire(
	wsID string,
)
- func (m *ProviderPollManager) Release(
	wsID string,
)
- func (m *ProviderPollManager) StopAll()
- // api/internal/app/realtime/service.go — add to Service
func (s *Service) AcquireProviderPoll(
	wsID string,
)
- func (s *Service) ReleaseProviderPoll(
	wsID string,
)
- // api/internal/app/realtime/service.go — New gains the provider-poll usecase
func New(
	ctx context.Context,
	h *hub.Hub,
	workspace workspacerepo.Workspace,
	gitEngine enginegit.Engine,
	fsEngine enginefs.Engine,
	lspLifecycle LSPLifecycle,
	providerPoll ProviderPoller,
	perConnPollInterval time.Duration,
	now func() time.Time,
) *Service
- // api/internal/api/v0/container.go — new lifecycle wrapper
func withProviderPollLifecycle[T any](
	def ws.StreamDef[T],
	appContainer *app.Container,
) ws.StreamDef[T]
- // api/internal/engine/provider/poll/poll.go — global cron interval constant
const GlobalCronInterval = 5 * time.Minute
const PerConnectionInterval = 1 * time.Minute
- // api/internal/engine/provider/poll/poll.go — explicit 5-min constructor (keeps configurable seam for tests)
func NewSweeper(
	pollFn PollFn,
	onStateChange OnStateChangeFn,
) Sweeper // now defaults to GlobalCronInterval (5m)
- // New hierarchical provider routes (spec §3)
GET /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/provider
GET /v0/projects/:projectId/repos/:repoId/protected-branches
GET/WS /v0/projects/:projectId/repos/:repoId/workspaces/:wsId  // workspace-scoped WS; subscribing here starts the 1-min poll
- // api/internal/api/v0/endpoints/provider/routes.go — updated mount
func Register(
	rg *gin.RouterGroup,
	provEng provhandlers.ProviderEngine,
	wsReader provhandlers.WorkspaceReader,
)

### Risks
- Missing domain status constants: spec §5/§11 assume WorkspaceStatusLocked, WorkspaceStatusPRConflicts, WorkspaceStatusDeleted exist, but workspace_status.go only defines new/pr-open/pr-merged/pr-closed. The provider poll's pr-conflicts transition has no domain target until these are added — flagged explicitly as a spec assumption that does not yet exist in code.
- No 'conflicts' signal flows through the provider pipeline: PRInfo (types.go) carries only Status open|merged|closed. The spec says pr-conflicts can come from the provider reporting unresolvable PR conflicts, but neither gh pr list (--json number,state,url,title,baseRefName) nor the PRInfo struct/ProviderInput carries a mergeable/conflicts field. Mapping provider conflicts to pr-conflicts requires extending PRInfo + the gh/glab JSON fields + ProviderInput + SyncProviderState — a cross-cut beyond §12's 'sweep logic unchanged'.
- Sweep filter is Status==pr-open only (container.go:158). pr-merged/pr-closed are terminal and never re-swept, and pr-conflicts workspaces would be skipped. Widening the cron to 'PRUrl != ""' risks repeatedly polling closed/merged PRs forever; needs a deliberate terminal-state stop rule.
- Per-connection poll must key off the SINGLE-workspace WS only. scopeWsID returns the :wsId path param OR ?wsId query; the list-scope workspace broadcaster (filtered by projectId/repoId) has no wsId, so it correctly no-ops — but if a future route passes ?wsId on a list connection, a poll could start unexpectedly. The blank-wsID no-op in the manager is the only guard.
- Refcount double-counting across topics: the same wsId may already be acquired by the watcher/LSP lifecycles. The provider-poll manager must use its OWN handle map (independent refcount), or a single OnSubscribe firing multiple Acquire* across managers could leak. Mirror WatcherManager's per-manager isolation exactly.
- Ctx lifetime mismatch: the global cron uses context.WithoutCancel(ctx) in sweepCallback so SyncFromState survives shutdown mid-tick; the per-connection poll goroutine is rooted on the realtime Service ctx and cancelled on WS close. PollWorkspace -> SyncFromState writes via Asynx; if the manager cancels the ctx mid-command an Asynx write could be aborted. Use a derived non-cancel ctx for the actual command issue, like the existing sweep callback does.
- Broadcaster snapshot vs poll race: connecting to /workspaces/:wsId triggers OnSubscribe AFTER register but the snapshot is computed outside the lock (broadcaster.go snapshotFor). If AcquireProviderPoll fires an immediate poll that mutates status before the snapshot serialises, the client may receive snapshot+poll frames out of order. Both are idempotent full-DTO replaces (upsert by id), so this is harmless given the §6 cache merge rule — but the poll should NOT fire synchronously inside OnSubscribe; tick first after the interval (or run the initial poll in the manager goroutine, not the registration path).
- Hierarchical route migration is shared with the workspaces/repos subsystems: provider routes.go Register and the workspace WS def both move under /v0/projects/:p/repos/:r/... Coordinating param names (:projectId, :repoId, :wsId) across endpoints is a cross-subsystem ordering hazard; provider handlers depend on the repo aggregate path (RepoDir) which is owned by the storage/worktreepath refactor (§8) that must land first.
- ProtectedBranches currently derives the repo root from a workspace worktree (Wave-3 hack); after §1 the worktree is .../workspaces/<W>/worktree and the repo's own path is .../projects/<P>/<R>. gh/glab run `git remote get-url origin` in cmd.Dir — a worktree dir still resolves origin, but switching to RepoDir requires that the repo path is itself a git working dir (it is the bare/primary clone location per §1); verify the repo path actually contains .git before assuming CLI calls succeed.
- maybeReparentFromPR uses a GLOBAL List() (provider_sync.go:136) to find a sibling branch; under entity-scoped storage (§2/§9) a global cross-workspace List may become expensive or unavailable (per-entity lazy DBs). The auto-reparent scan needs a repo-scoped sibling query after the storage refactor.
- Test interval coupling: poll_test.go asserts NewSweeper default == 60s (TestNewSweeper_DefaultInterval, poll_test.go:183). Changing the default to 5m will break that test and must be updated in lockstep; do not silently flip the constant.

### Test targets
- api/internal/engine/provider/poll/poll_test.go — UPDATE TestNewSweeper_DefaultInterval to assert the new 5-minute GlobalCronInterval default (was 60s); keep TestNewSweeperWithInterval for the configurable seam. No time.Sleep — use the existing channel-notify pattern (notified <- struct{}{}).
- api/internal/engine/provider/poll/poll_test.go — ADD TestSweeper_FiltersByPRUrl (cron sweeps all workspaces with PRUrl != "", not just pr-open) if the cron filter widens; assert pr-merged/pr-closed terminal workspaces are excluded to prevent infinite re-poll.
- api/internal/app/realtime/provider_poll_manager_test.go (new) — TestProviderPollManager_Acquire_StartsPoll (0->1 starts a goroutine that calls PollWorkspace; assert via a fake ProviderPoller signalling on a channel), TestProviderPollManager_Release_StopsPoll (1->0 cancels; no further calls after a context-done waiter), TestProviderPollManager_Refcount (two Acquire + one Release keeps polling), TestProviderPollManager_BlankWsID_NoOp, TestProviderPollManager_StopAll_Idempotent, TestProviderPollManager_AcquireAfterClose_NoOp. Inject a tiny interval; synchronise via a fake poller channel, never time.Sleep.
- api/internal/app/realtime/service_test.go — ADD TestService_AcquireProviderPoll / ReleaseProviderPoll delegate to the manager; TestService_Close_StopsProviderPoll.
- api/internal/app/usecases/provider/provider_sync_test.go — ADD TestSyncFromState_MapsPRConflicts once a conflicts signal exists; TestPollWorkspace_PerConnectionPath asserts PollWorkspace -> SyncProviderState issues exactly one command (use a fake WorkspaceRepo capturing ProviderInput).
- api/internal/app/repositories/workspace/internal/commands/sync_provider_state_test.go — ADD cases for prStatusToWorkspace mapping pr-conflicts and for the locked-as-status decision (§5); assert open->pr-open, merged->pr-merged, closed->pr-closed, conflicts->pr-conflicts.
- api/internal/api/v0/endpoints/provider/handlers/provider_test.go (handlers_test.go) — UPDATE for hierarchical params (projectId/repoId/wsId); assert State writes dto.ProviderStateDTOFrom shape (envelope) and ProtectedBranches uses repo path not worktree path.
- api/tests/regressions_test.go (build tag integration) — TestRegression_ProviderPoll_PROpenToMerged: spin real daemon against temp ~/.crowbar, create a workspace with a PR via a MOCK provider (inject a fake gh/glab exec or provider factory), open the real WS to /v0/projects/:p/repos/:r/workspaces/:wsId, drive a status change, block on the WS frame (context deadline, no time.Sleep) and assert WorkspaceDTO.Status transitions pr-open -> pr-merged and PRUrl/PRTitle/PRTargetBranch populate.
- api/tests/ (integration) — TestRegression_PerConnectionPoll_StartsOnSubscribe: assert that connecting the workspace WS starts the 1-min poller (observe a provider call via the mock provider within a deadline using a short test interval) and that disconnecting stops it (no further provider calls after WS close).
- api/tests/ (integration) — TestRegression_GlobalCron_PollsUnwatchedPRWorkspaces: with no WS connected, the 5-min cron (run with an injected short interval in the test daemon) transitions an unwatched pr-open workspace; assert the WorkspaceDTO arrives on a freshly-opened WS seeded by snapshot.
- api/tests/ (integration) — TestRegression_ProviderState_HierarchicalRoute: GET /v0/projects/:p/repos/:r/workspaces/:w/provider returns 200 + ProviderStateDTO envelope; GET /v0/projects/:p/repos/:r/protected-branches returns the protected list (fallback when CLI absent).

---

## Repo + Project domain/usecase/handlers, icon-on-disk storage, GitHub owner-avatar default on import, GORM projections

### Key signatures
- `api/internal/domain/repository.go`
  - type Repository struct {ID,ProjectID,Name,Path,DefaultBranch,AvatarLabel,AvatarColor,AvatarURL,RemoteURL} (repository.go:3)
  - func (Repository) TableName() string -> "repositories" (repository.go:15)
- `api/internal/domain/project.go`
  - type Project struct {ID,Name,Path,LastActivity} (project.go:6)
  - func (Project) TableName() string -> "projects" (project.go:13)
- `api/internal/domain/node.go`
  - type FileNode struct {Name,Path,Type,Children,GitStatus} (node.go:24)
- `api/internal/app/usecases/project/project.go`
  - type Usecase interface {List, Get, TouchProjectActivity} (project.go:15)
  - type projectUsecase struct {projects store.Store[domain.Project,string]; repos store.Store[domain.Repository,string]} (project.go:37)
  - func New(projects, repos) Usecase (project.go:43)
  - func (u *projectUsecase) List/Get/TouchProjectActivity (project.go:54,65,82)
- `api/internal/app/usecases/project/project_import.go`
  - const importMaxDepth = 3 (project_import.go:27)
  - var defaultBranchCandidates = []string{"main","develop","master"} (project_import.go:31)
  - type ImportProviderEngine interface {ProtectedBranches; OwnerAvatarURL(ctx,repoPath)(string,error)} (project_import.go:73)
  - type ImportDeps struct {Projects,Repos,Workspaces,Git,Provider,Discover,RefRunner,Now,Stat} (project_import.go:97)
  - func (u *projectImport) Import(ctx,name,path)(domain.Project,error) (project_import.go:165)
  - func (u *projectImport) Create(ctx,name,path)(domain.Project,error) (project_import.go:145)
  - func (u *projectImport) importOneRepo(ctx,project,repoPath) error (project_import.go:209)
  - func gitRemoteURL(repoPath string) string (project_import.go:367)
- `api/internal/app/usecases/project/project_delete.go`
  - type DeleteDeps struct {Projects,Repos,Workspaces,Git,CrowbarHome func()(string,error)} (project_delete.go:68)
  - func (u *projectDelete) Delete(ctx,id) error (project_delete.go:108)
  - func (u *projectDelete) removeWorktreeIfCrowbarManaged(ctx,ws,repos) (project_delete.go:190)
- `api/internal/app/usecases/internal/avatar/avatar.go`
  - func Label(name string) string (avatar.go:35)
  - func Color(name string) string (avatar.go:47)
  - func Palette() []string (avatar.go:29)
  - var iconCandidates []string (avatar.go:58)
  - func ScanRepoIcon(repoPath string) string (avatar.go:75)
- `api/internal/api/v0/endpoints/repos/handlers/repos.go`
  - type Store interface {FindAll,FindByKey,Save} (repos.go:70)
  - type Handlers struct {store,provider,wsReader} (repos.go:102)
  - func NewWithDeps(store,prov,wsReader) *Handlers (repos.go:117)
  - func (h *Handlers) List/Detail/Create (repos.go:123,139,159)
  - func (h *Handlers) Icon(c) (repos.go:223)
  - func (h *Handlers) Branches(c) (repos.go:269)
  - func (h *Handlers) PutIcon/DeleteIcon/PutIconEmoji/PutIconGithub (repos.go:402,381,353,465)
  - func repoIconDir(remoteURL string)(string,error) (repos.go:489)
  - func repoRelPathFromURL(rawURL string)(string,error) (repos.go:503)
  - func fetchGithubAvatar(ctx,repoPath string) string (repos.go:555)
  - func gitDefaultBranch/gitRemoteURL/parseRemoteBranches/repoAvatar (repos.go:211,60,318,33)
- `api/internal/api/v0/endpoints/repos/routes.go`
  - func Register(rg *gin.RouterGroup, store, prov, wsReader) (routes.go:12)
- `api/internal/api/v0/endpoints/projects/handlers/projects.go`
  - type ListGetter interface {List,Get} (projects.go:17)
  - type Importer interface {Import,Create} (projects.go:29)
  - type Deleter interface {Delete} (projects.go:48)
  - type importRequest struct {Name,Path,Quick} (projects.go:79)
  - func (h *Handlers) List/Detail/Import/Delete (projects.go:88,101,115,152)
- `api/internal/api/v0/endpoints/projects/routes.go`
  - func Register(rg *gin.RouterGroup, reader, importer, deleter) (routes.go:13)
- `api/internal/api/v0/dto/repo.go`
  - type RepoDTO struct {ID,ProjectID,Name,Path,DefaultBranch,AvatarLabel,AvatarColor,AvatarURL} (repo.go:9)
  - func RepoDTOFrom(r domain.Repository) RepoDTO (repo.go:20)
  - func RepoDTOList([]domain.Repository) []RepoDTO (repo.go:44)
- `api/internal/api/v0/dto/project.go`
  - type ProjectDTO struct {ID,Name,Path,LastActivity} (project.go:14)
  - func ProjectDTOFrom/ProjectDTOList (project.go:22,35)
- `api/internal/app/gorm.go`
  - type GORMStores struct {Projects,Repositories,TerminalProfiles} (gorm.go:14)
  - func newGORMStores(db *gormdb.DB) (*GORMStores,error) (gorm.go:20)
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go`
  - func For(crowbarHome, remoteURL, workspaceID string) (string, error) (worktreepath.go:20)
  - func RepoDir(crowbarHome, remoteURL string) (string, error) (worktreepath.go:33)
  - func DefaultCrowbarHome() (string, error) (worktreepath.go:43)
  - func repoRelPath(rawURL string) (string, error) (worktreepath.go:53)
- `api/internal/adapter/container.go`
  - type Container struct {WorkspaceES,ChatES,AgentRunES,ReviewThreadES asynxModels.Store; DB *gorm.DB; closers; lock} (container.go:19)
  - func New(opts...) (*Container,error) (container.go:46)
  - func openEventStores(eventsPath) ([]Store,[]Closer,error) — names workspace.db/chat.db/agent_run.db/review_thread.db (container.go:157)

### Must change
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go` — §8: Rewrite For to `For(crowbarHome, projectID, repoID, workspaceID string) string` (no error, append `/worktree`). Add `StorageDir(crowbarHome,projectID,repoID,workspaceID) string`, `RepoDir(crowbarHome,projectID,repoID) string`, `ProjectDir(crowbarHome,projectID) string`, and a repo-icon helper `RepoIconPath(crowbarHome,projectID,repoID) string` = RepoDir(...)/icon. Delete repoRelPath/URL parsing — no longer URL-derived. Keep DefaultCrowbarHome. Update the sole caller worktree.go:121 to pass (home, in.ProjectID, in.RepoID, wsID) and drop the error branch.
- `api/internal/domain/repository.go` — §2/§5: Replace the overloaded AvatarURL string with explicit projection columns to match the spec's repositories view: keep AvatarLabel, AvatarColor; add `AvatarHasIcon bool` (icon-on-disk flag). The spec repositories table (§2) lists (id, project_id, name, path, default_branch, remote_url, avatar_label, avatar_color, avatar_has_icon). Decide whether emoji/github-url icon kinds remain — if kept, add an `IconKind`/`IconValue` pair rather than reusing AvatarURL; otherwise AvatarHasIcon + on-disk bytes only. RemoteURL stays (still needed for github-avatar fetch + provider), but is no longer used for path derivation.
- `api/internal/app/usecases/internal/avatar/avatar.go` — §1/§3: ScanRepoIcon must stop being the source for AvatarURL pointing into the user's repo. On import, if a local icon is found it should be COPIED into <crowbarHome>/projects/<P>/<R>/icon (entity-scoped, no extension) — not referenced in place. Either move the copy into the import usecase and keep ScanRepoIcon returning the source path, or add a helper that returns the bytes+content-type. Default-on-import behavior (§3/E2E Step 3): when no local icon, fetch GitHub owner avatar bytes and write them to the on-disk icon path.
- `api/internal/app/usecases/project/project_import.go` — §1/§3: importOneRepo must (1) use worktreepath UUID paths (no RemoteURL-derived worktree), (2) write the repo icon to <crowbarHome>/projects/<P>/<R>/icon instead of storing an absolute scan path in AvatarURL, (3) default the icon to the GitHub owner avatar bytes on import (OwnerAvatarURL → download → write to icon file) per §3 'default GitHub owner avatar on import', setting AvatarHasIcon=true. The Repository must be persisted into the PER-REPO view.db (§2), not the global crowbar.db. Project row goes to the per-project view.db. ImportProviderEngine.OwnerAvatarURL stays but the import now needs to fetch the bytes too (or a new dep).
- `api/internal/app/usecases/project/project_delete.go` — §1: Delete must `rm -rf <crowbarHome>/projects/<P>` for the project and per-repo dirs, which now atomically removes worktrees, icon, event_stream.db, view.db. The current per-workspace worktree-only teardown + record cascade against the global DB must become directory-tree removal scoped to the project UUID dir. CrowbarHome dep stays; add a recursive-remove step keyed on worktreepath.ProjectDir. Preserve the 'never touch the user's real repo Path' invariant — only the ~/.crowbar/projects/<P> subtree is removed.
- `api/internal/api/v0/endpoints/repos/handlers/repos.go` — §1/§3: Replace repoIconDir(remoteURL)+repoRelPathFromURL URL derivation with worktreepath.RepoIconPath(crowbarHome, projectId, repoId) = ~/.crowbar/projects/<P>/<R>/icon (no extension). PutIcon writes raw bytes to that single file; record content-type via AvatarHasIcon + (if kept) a stored mime. Icon() reads the on-disk icon bytes (no http proxy fetch for the github case — github avatar is fetched-and-stored at PutIconGithub time, then served from disk). PutIconGithub downloads the avatar bytes and writes them to the icon file (sets AvatarHasIcon=true) instead of storing an HTTPS URL. DeleteIcon removes the on-disk icon file and sets AvatarHasIcon=false (reset to generated avatar). Handlers must receive projectId+repoId from path params (`:projectId`/`:repoId`), and the Store must be the per-repo-scoped view store, not the global one. Drop the duplicate repoAvatar() in favor of avatar.Label/Color, or keep but unify.
- `api/internal/api/v0/endpoints/repos/routes.go` — §3: Re-mount under hierarchical paths: /v0/projects/:projectId/repos (GET/WS/POST), /v0/projects/:projectId/repos/:repoId (GET/WS/DELETE), /v0/projects/:projectId/repos/:repoId/icon (GET/PUT/DELETE), /icon/emoji (PUT), /icon/github (PUT), /branches (GET), /protected-branches (GET). Register must take projectId/repoId-aware handlers + the Broadcaster[RepoDTO] for the GET/WS dispatch.
- `api/internal/api/v0/endpoints/projects/handlers/projects.go` — §3/§4: POST /v0/projects and DELETE /v0/projects/:projectId become 202 + WS (domain-entity mutations): validate synchronously, return 202 empty body, run import/delete in a background goroutine, then broadcast the ProjectDTO (status/deleted) via Broadcaster[ProjectDTO]. List/Detail add a WS branch via dispatch(). importRequest.Quick / Create vs Import behavior stays but the response contract changes from 201-with-id to 202.
- `api/internal/api/v0/endpoints/projects/routes.go` — §3: Add WS dispatch on GET /v0/projects and GET /v0/projects/:projectId. POST returns 202. DELETE returns 202. Wire Broadcaster[ProjectDTO].
- `api/internal/api/v0/dto/repo.go` — §5: RepoDTOFrom must emit AvatarURL as the hierarchical proxy `/v0/projects/<projectId>/repos/<id>/icon` (currently `/v0/repos/<id>/icon`). Drive the proxy off AvatarHasIcon (and emoji passthrough if retained) rather than parsing the AvatarURL string. RepoDTO is now also the Broadcaster[RepoDTO] payload.
- `api/internal/api/v0/dto/project.go` — §5/§4: ProjectDTO becomes the broadcaster type. Add a `Status string` field if the 'deleted' tombstone pattern (§6 cache merge: status:"deleted") is applied to projects so a DELETE can broadcast removal on the single typed channel. (Spec §5 names Broadcaster[ProjectDTO]; the deleted-tombstone rule in §6 is defined for workspaces but the one-type-per-channel rule implies projects need the same.)
- `api/internal/app/gorm.go` — §2: Stop building Projects and Repositories against one shared crowbar.db. Projects must be persisted into the per-project view.db (~/.crowbar/projects/<P>/storages/view.db) and Repositories into the per-repo view.db (~/.crowbar/projects/<P>/<R>/storages/view.db). Either replace GORMStores' Projects/Repositories with a lazy per-entity view-store provider (mirroring §9's AdapterContainer LRU registry) or move these stores out of the global GORMStores entirely. TerminalProfiles stays global (§2 global view.db).
- `api/internal/adapter/container.go` — §9: Replace eager openEventStores(4 named DBs) + single crowbar.db with lazy per-entity open + LRU registries keyed by projectID / projectID/repoID / projectID/repoID/wsID, plus per-entity view.db handles. Provide WorkspaceES/RepoES/ProjectES accessors that MkdirAll the storages/ dir and open event_stream.db/view.db on demand, evicting+closing the LRU when maxOpenWorkspaceDBs (64) is exceeded. Global state DBs become ~/.crowbar/state/event_stream.db + view.db (renamed from events/*.db and store/crowbar.db).

### New contracts
- // worktreepath.go (§8) — all UUID-based, no error
func For(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
) string
- func StorageDir(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
) string
- func RepoDir(
	crowbarHome string,
	projectID string,
	repoID string,
) string
- func ProjectDir(
	crowbarHome string,
	projectID string,
) string
- // new icon-on-disk path helper (§1)
func RepoIconPath(
	crowbarHome string,
	projectID string,
	repoID string,
) string // = filepath.Join(RepoDir(...), "icon")
- // domain.Repository (§2 repositories view) — icon-on-disk
type Repository struct {
	ID            string `gorm:"primaryKey" json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultBranch string `json:"defaultBranch"`
	RemoteURL     string `json:"remoteUrl,omitempty"`
	AvatarLabel   string `json:"avatarLabel"`
	AvatarColor   string `json:"avatarColor"`
	AvatarHasIcon bool   `json:"avatarHasIcon"`
}
- // dto.RepoDTO (§5) — AvatarURL is the hierarchical proxy
type RepoDTO struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultBranch string `json:"defaultBranch"`
	AvatarLabel   string `json:"avatarLabel"`
	AvatarColor   string `json:"avatarColor"`
	AvatarURL     string `json:"avatarUrl,omitempty"` // "/v0/projects/<p>/repos/<id>/icon" when AvatarHasIcon
}
- // dto.RepoDTOFrom now needs the project scope to build the proxy URL
func RepoDTOFrom(
	r domain.Repository,
) RepoDTO // builds AvatarURL = "/v0/projects/"+r.ProjectID+"/repos/"+r.ID+"/icon" when r.AvatarHasIcon
- // dto.ProjectDTO (§5/§6) — tombstone-capable broadcaster payload
type ProjectDTO struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	Status       string    `json:"status"` // "" | "deleted"
	LastActivity time.Time `json:"lastActivity"`
}
- // import provider surface — fetch avatar BYTES, not URL (§3 default-on-import)
type ImportProviderEngine interface {
	ProtectedBranches(
		ctx context.Context,
		repoPath string,
	) ([]string, error)
	OwnerAvatarURL(
		ctx context.Context,
		repoPath string,
	) (string, error)
}
- // avatar helper to fetch + return downloadable bytes for on-disk write (§3)
func FetchOwnerAvatarBytes(
	ctx context.Context,
	repoPath string,
) ([]byte, string, error) // bytes, contentType, err
- // adapter.AdapterContainer accessors (§9) — lazy per-entity
func (c *AdapterContainer) ProjectView(
	projectID string,
) (*gorm.DB, error)
- func (c *AdapterContainer) RepoView(
	projectID string,
	repoID string,
) (*gorm.DB, error)
- func (c *AdapterContainer) RepoES(
	projectID string,
	repoID string,
) (asynxModels.Store, error)
- func (c *AdapterContainer) ProjectES(
	projectID string,
) (asynxModels.Store, error)
- // repos handler routes (§3)
GET    /v0/projects/:projectId/repos
- POST   /v0/projects/:projectId/repos
- GET    /v0/projects/:projectId/repos/:repoId
- DELETE /v0/projects/:projectId/repos/:repoId
- GET    /v0/projects/:projectId/repos/:repoId/icon
- PUT    /v0/projects/:projectId/repos/:repoId/icon
- DELETE /v0/projects/:projectId/repos/:repoId/icon
- PUT    /v0/projects/:projectId/repos/:repoId/icon/emoji
- PUT    /v0/projects/:projectId/repos/:repoId/icon/github
- GET    /v0/projects/:projectId/repos/:repoId/branches
- GET    /v0/projects/:projectId/repos/:repoId/protected-branches
- // project handler routes (§3)
GET    /v0/projects        // GET + WS dispatch -> Broadcaster[ProjectDTO]
- POST   /v0/projects        // 202 Accepted, ProjectDTO arrives via WS
- GET    /v0/projects/:projectId   // GET + WS dispatch
- DELETE /v0/projects/:projectId   // 202 Accepted, ProjectDTO{status:"deleted"} via WS

### Risks
- SHARED FILE / cross-subsystem: api/internal/app/gorm.go and api/internal/adapter/container.go are touched by EVERY entity subsystem (project, repo, workspace, thread, terminal). The §9 lazy AdapterContainer + LRU registry is a single rewrite that all subsystems depend on; this map's icon/repo/project changes cannot land until those accessors exist. Coordinate ordering: container.go + worktreepath.go first, then project/repo usecases.
- AvatarURL is overloaded across FOUR layers (domain.Repository, repos.go handlers, dto/repo.go, frontend). Collapsing it to AvatarHasIcon + on-disk bytes drops the emoji and HTTPS-URL representations. The spec §3 route /icon/emoji (PUT) still exists, so emoji must be preserved somewhere (an additional IconKind/IconValue column or an emoji sidecar). The current dto/repo.go `emoji:` passthrough and Icon() http-proxy branch will both break if AvatarURL is removed without a replacement representation — call this out as an unresolved spec gap.
- TWO independent avatar implementations: repos.go has its own repoAvatar() + avatarColors palette (bg-*-700 Tailwind classes) while avatar.go has Label/Color (avatar-* tokens). Import uses avatar.go; the POST /v0/repos Create handler uses repoAvatar. If repos move to per-repo view.db and import is the only creator, the handler's repoAvatar may become dead or produce inconsistent colors. Unify on avatar.Label/Color.
- TWO independent URL parsers: worktreepath.repoRelPath, repos.go repoRelPathFromURL, and repos.go githubSlugFromURL all parse git remote URLs. §8 deletes worktreepath's parser (UUID-based) but repos.go still needs slug parsing for github-avatar fetch. Don't delete the wrong one.
- Cascade delete semantics change from record-cascade (delete rows in global DB) to directory-tree rm -rf of ~/.crowbar/projects/<P>. The current invariant 'never delete the user's real repo Path' is enforced by a crowbarHome-prefix check on WorktreePath; under entity-scoped storage the worktree is always inside <P>/<R>/workspaces/<W>/worktree, so the check becomes structural — but the adopted-main-worktree case (worktree.go adoptMainWorktree with WorktreePath==repo.Path OUTSIDE ~/.crowbar) means the user's real checkout is still referenced by a workspace row living in an in-tree view.db. Deleting the project dir must NOT touch repo.Path. Verify adoptMainWorktree's path is still external and excluded.
- Import currently fetches OwnerAvatarURL (a URL string). §3 default-on-import requires downloading the bytes and writing them to the on-disk icon file. This adds a network fetch INSIDE the import path (currently import only resolves the URL). Under the §4 202 contract, project import becomes async (background goroutine) — a slow/failing avatar download must not fail the whole import; it should leave AvatarHasIcon=false and broadcast the repo anyway. Ordering: repo row + worktree first, avatar best-effort after.
- gorm AutoMigrate per-entity: today storesqlite.NewFromDB AutoMigrates the repositories/projects tables on the single crowbar.db. With per-project/per-repo view.db, AutoMigrate runs lazily on first open of each new DB file — verify NewFromDB is idempotent and cheap on already-migrated files, and that the LRU eviction (closing a *sqlite handle) does not corrupt an in-flight AutoMigrate.
- Broadcaster snapshot race (§4/§5): POST returns 202 then a background goroutine does the work and Push()es the DTO. A WS client must subscribe BEFORE the work completes or it misses the event (no replay — Open Question 4 deferred). Integration tests must dial the WS before issuing the POST, then block on the message; otherwise flaky. The namespace function for RepoDTO must key on ProjectID+"/"+ID so a repo-scoped client and a project-scoped (prefix) client both match.
- Frontend coupling: dto/repo.go AvatarURL changes from /v0/repos/<id>/icon to /v0/projects/<p>/repos/<id>/icon. Every <img src> and the WKWebView crowbar:// proxy in web/ that consumes avatarUrl must update in lockstep, or icons 404. The frontend route-migration (§7) is a separate subsystem but shares this contract.
- Provider OwnerAvatarURL/fetchGithubAvatar shell out to `gh` and `git`. In integration tests (temp ~/.crowbar, fixture repos with no real origin/no gh auth) these return "" — the default-on-import path must degrade gracefully to AvatarHasIcon=false. Tests must not assume a GitHub avatar is fetchable.
- domain.Repository is listed under §12 'What Stays Unchanged' (domain model structs). But §2/§5 require an avatar_has_icon column and removal of AvatarURL overload — this is a direct contradiction with §12. Flag as a spec inconsistency the implementers must resolve before touching domain/repository.go.
- TerminalProfiles remains in the global GORMStores while Projects/Repositories move out. gorm.go's newGORMStores returns all three from one db; splitting it changes the struct shape and every caller of newGORMStores / GORMStores.Projects / .Repositories across the app wiring (container, usecase construction). Audit all references before changing the struct.

### Test targets
- api/internal/app/usecases/internal/worktreepath/worktreepath_test.go — rewrite: TestFor_UUIDPath (For returns <home>/projects/<p>/<r>/workspaces/<w>/worktree), TestStorageDir, TestRepoDir, TestProjectDir, TestRepoIconPath (<home>/projects/<p>/<r>/icon). Remove URL-parsing cases. Add worktreepath_bench_test.go BenchmarkFor (§13 path-construction benchmark).
- api/internal/app/usecases/internal/avatar/avatar_test.go — keep Label/Color/Palette cases; add TestFetchOwnerAvatarBytes_DegradesToEmptyWhenNoGh (no gh/origin -> nil bytes, no error swallowed wrong) and TestScanRepoIcon_ReturnsSourcePath (still returns in-repo path so import can copy bytes).
- api/internal/app/usecases/project/project_import_test.go — TestImport_WritesRepoIconToEntityDir (icon written to <home>/projects/<P>/<R>/icon, AvatarHasIcon=true), TestImport_DefaultsToGithubAvatarWhenNoLocalIcon, TestImport_AvatarFetchFailureLeavesRepoWithGeneratedAvatar (AvatarHasIcon=false, repo still saved), TestImport_RepoPersistedToPerRepoView (repo row in per-repo view.db, not global). Stub Provider + a fake avatar-bytes fetcher; use ImportDeps.Stat + fakes (no real fs/network).
- api/internal/app/usecases/project/project_delete_test.go — TestDelete_RemovesProjectDirTree (rm -rf <home>/projects/<P> incl icon/worktree/storages), TestDelete_NeverTouchesRealRepoPath (adopted main worktree at repo.Path outside ~/.crowbar survives), TestDelete_BestEffortContinuesOnRemoveFailure.
- api/internal/api/v0/dto/repo_test.go — TestRepoDTOFrom_ProxyURLHierarchical (AvatarHasIcon true -> /v0/projects/<p>/repos/<id>/icon), TestRepoDTOFrom_NoIconEmptyAvatarURL, and emoji passthrough case IF emoji representation retained.
- api/internal/api/v0/endpoints/repos/handlers/repos_test.go — TestPutIcon_WritesToEntityIconPath (bytes land at worktreepath.RepoIconPath, AvatarHasIcon set), TestIcon_ServesOnDiskBytes (correct content-type, no http proxy), TestPutIconGithub_DownloadsAndStoresBytes, TestDeleteIcon_RemovesFileResetsFlag, TestBranches_AnnotatesProtectedAndHasWorkspace. Handlers now take projectId+repoId params — set them via gin test context.
- api/tests/projects_test.go (integration build tag) — update TestProjects_ImportThenList to the hierarchical GET /v0/projects/:projectId/repos and the 202+WS contract: dial WS /v0/projects before POST, assert 202, block on ProjectDTO message (context deadline, NO time.Sleep). Update TestProjects_DeleteCascadesRecordsKeepsRealRepoOnDisk to assert 202 + WS ProjectDTO{status:"deleted"} and that ~/.crowbar/projects/<P> is gone while the real repo dir on disk remains.
- api/tests/regressions_test.go (integration) — TestRegression_RepoIconStoredOnDiskAtEntityPath (PUT icon then GET /v0/projects/:p/repos/:r/icon returns the bytes from <P>/<R>/icon), TestRegression_DefaultGithubAvatarFetchedOnImport (import a repo with a github origin/mock provider -> AvatarHasIcon true), TestRegression_RepoDTOAvatarURLIsHierarchical (GET repos -> avatarUrl == /v0/projects/.../repos/.../icon), TestRegression_DeleteProjectRemovesEntityDirTree. Use the existing harness dial() WS helper (api/tests/harness_test.go) and SetReadDeadline-based blocking — no time.Sleep.
- api/internal/adapter/container_test.go — TestAdapterContainer_LazyOpensRepoView, TestAdapterContainer_LRUEvictsAndClosesOldestES (open >64 workspace ES, assert LRU eviction closes the handle), TestAdapterContainer_MkdirAllsStoragesDir. Add container_bench_test.go BenchmarkRepoESLookup (§13 DB-registry-lookup benchmark).
- api/internal/app/gorm_test.go — update for split stores: TestNewGORMStores_GlobalOnlyHoldsTerminalProfiles (Projects/Repositories no longer on the shared db), and a per-entity view-store open/migrate test if the provider lives here.

---

## Terminal engine + sessions + ws + profiles → workspace-scoped (move session creation under workspace scope, add Broadcaster[TerminalSessionDTO] lifecycle topic, PTY stream stays raw, working dir = worktree path)

### Key signatures
- `api/internal/engine/terminal/terminal.go`
  - Engine interface (terminal.go:35) Create(ctx, workspaceID string, workspaceDir string, prof *domain.TerminalProfile) (sessionID string, err error) (terminal.go:37-42)
  - Attach(ctx, sessionID string, conn WSConn) error (terminal.go:47-51)
  - Write/Resize/Kill/ListSessions()/SessionExists/Shutdown (terminal.go:53-85)
  - func New() Engine (terminal.go:92)
  - terminalEngine.Create(_, _, workspaceDir, prof) (terminal.go:113) — workspaceID param currently ignored (named _ at :117)
  - outputMsg{SessionID,Data,IsInput} (terminal.go:99); inputMsg{Type,Data,Cols,Rows} (terminal.go:106)
  - reapOnDone(id, s) removes from registry on PTY exit (terminal.go:146)
- `api/internal/engine/terminal/internal/session/session.go`
  - func New(id, shell, cwd string, env []string) (*Session, error) (session.go:42) — cmd.Dir=cwd
  - Attach() (<-chan OutputFrame, error) (session.go:83)
  - Done() <-chan struct{} (session.go:76)
  - Write/Resize/Kill (session.go:128/139/154)
  - OutputFrame{SessionID string, Data []byte} (session.go:18)
- `api/internal/engine/terminal/internal/session/ring.go`
  - const defaultRingSize = 64*1024 (ring.go:6)
  - newRingBuffer(capacity int) *RingBuffer (ring.go:18)
  - Write(p []byte) (ring.go:25); Snapshot() []byte (ring.go:60)
- `api/internal/engine/terminal/internal/registry/registry.go`
  - type Registry (registry.go:12); New() *Registry (registry.go:18)
  - Add(id, *session.Session)/Get(id)(*Session,bool)/Remove(id)/List() []string (registry.go:23-59)
  - var ErrSessionNotFound (registry.go:63)
- `api/internal/engine/terminal/internal/profile/profile.go`
  - Resolved{Shell,CWD,Startup} (profile.go:11)
  - Resolve(p *domain.TerminalProfile, workspaceDir string) Resolved (profile.go:20)
  - resolveCWD: p.StartupDirectory else workspaceDir (profile.go:45)
- `api/internal/app/usecases/terminal/terminal.go`
  - Usecase-local Engine.Create(ctx, workspaceID, workspaceDir, prof)+Kill (terminal.go:14)
  - WorkspaceRepo.Get(ctx, id) (domain.Workspace, error) (terminal.go:29)
  - Usecase.CreateSession(ctx, wsID string, prof *domain.TerminalProfile) (string, error) (terminal.go:39)
  - New(engine, profiles store.Store[domain.TerminalProfile,string], workspaces WorkspaceRepo) Usecase (terminal.go:83)
  - CreateSession: ws=workspaces.Get; engine.Create(ctx, wsID, ws.WorktreePath, prof) (terminal.go:96-109)
- `api/internal/api/v0/endpoints/terminal/handlers/handlers.go`
  - TerminalEngine: Create/Kill/SessionExists/Attach (handlers.go:17)
  - ProfileStore: FindAll/FindByKey/Save/Delete (handlers.go:40)
  - WorkspaceReader.Get(ctx, id) (domain.Workspace, error) (handlers.go:48)
  - Handlers{termEng,profileStore,wsReader} (handlers.go:53); New(...) (handlers.go:60)
- `api/internal/api/v0/endpoints/terminal/handlers/sessions.go`
  - CreateSession(ctx): ctx.Param("wsId"); wsReader.Get; resolveProfile; eng.Create(ctx, wsID, ws.WorktreePath, prof); 201 {sessionId} (sessions.go:18-54)
  - KillSession(ctx): ctx.Param("sessionId"); eng.Kill; 200 WriteMutationOK(sid) (sessions.go:57-74)
  - resolveProfile(ctx, profileID) *domain.TerminalProfile (sessions.go:86)
- `api/internal/api/v0/endpoints/terminal/handlers/ws.go`
  - WS(ctx): ctx.Param("sessionId"); eng.SessionExists; terminalUpgrader.Upgrade; ping ticker; eng.Attach (ws.go:28-72)
  - terminalUpgrader (ws.go:19); wsWriteWait/wsPongWait/wsPingPeriod (ws.go:13)
- `api/internal/api/v0/endpoints/terminal/handlers/profiles.go`
  - ListProfiles/GetProfile/CreateProfile/UpdateProfile/DeleteProfile (profiles.go:15-110)
  - Routes GET/POST /v0/settings/terminal/profiles; GET/PUT/DELETE /:id
- `api/internal/api/v0/endpoints/terminal/routes.go`
  - Register(rg, termEng, profileStore, wsReader) (routes.go:12)
  - POST /workspaces/:wsId/terminals (routes.go:20); DELETE /terminals/:sessionId (routes.go:21)
  - GET /ws/terminals/:sessionId (routes.go:29); settings/terminal/profiles CRUD (routes.go:23-27)
- `api/internal/api/v0/dto/terminal.go`
  - TerminalProfileDTO{ID,Name,Shell,StartupDirectory,StartupCommands,Icon,Color} (terminal.go:11)
  - TerminalSessionDTO{SessionID string `json:"sessionId"`} (terminal.go:23) — only ONE field today
  - TerminalProfileDTOFrom/List (terminal.go:29/48)
- `api/internal/domain/terminal_profile.go`
  - TerminalProfile{ID gorm primaryKey, Name, Shell, StartupDirectory, StartupCommands serializer:json, Icon, Color} (terminal_profile.go:4)
  - TableName()="terminal_profiles" (terminal_profile.go:14)
- `api/internal/api/v0/router.go`
  - terminal.Register(...) (router.go:74)
  - workspaces.Register passes c.workspaces.Handle + ws.DualServe (router.go:47-54) — model to copy for the terminal lifecycle broadcaster
- `api/internal/api/v0/container.go`
  - Container fields workspaces/chats/git/files/lsp/chatStream broadcasters (container.go:25-32) — NO terminals field
  - workspacesDef with Filters projectId+repoId ExactMatch (container.go ~141) — template for terminalsDef
  - PushWorkspace/PushChat/PushGit/PushFile (container.go) implement hub.Subscriber
- `api/internal/app/hub/subscriber.go`
  - Subscriber{ PushWorkspace, PushChat, PushGit, PushFile } (subscriber.go:9-23)
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go`
  - For(crowbarHome, remoteURL, workspaceID string) (string, error) (worktreepath.go:20)
  - RepoDir(crowbarHome, remoteURL string) (string, error)
  - DefaultCrowbarHome() (string, error)

### Must change
- `api/internal/api/v0/endpoints/terminal/routes.go` — Re-mount session routes under the hierarchical prefix (spec §3): POST /projects/:projectId/repos/:repoId/workspaces/:wsId/terminals (create); GET/WS .../workspaces/:wsId/terminals (list sessions / lifecycle stream, dual-served via ws.DualServe + the new Broadcaster[TerminalSessionDTO].Handle); DELETE .../workspaces/:wsId/terminals/:sessionId (kill); WS .../workspaces/:wsId/terminals/:sessionId/ws (raw PTY). Grow Register to accept the lifecycle broadcaster Handle + dispatch func. Keep /settings/terminal/profiles global/unchanged.
- `api/internal/api/v0/endpoints/terminal/handlers/sessions.go` — CreateSession: read projectId+repoId+wsId path params (§3). Per §4 terminal-session creation is an Asynx-backed domain mutation → the §4 table says 202; the §3 route-tree note says 201 {sessionId}. Resolve the contradiction (see risks). Whatever the code returns, also broadcast a 'created'/active TerminalSessionDTO via the broadcaster. KillSession: §4 lists DELETE terminals/:id as 202; broadcast an 'ended' TerminalSessionDTO regardless. Currently 201/200 sync with no broadcast — both must change.
- `api/internal/api/v0/endpoints/terminal/handlers/ws.go` — Re-path the raw PTY WS to GET /projects/:projectId/repos/:repoId/workspaces/:wsId/terminals/:sessionId/ws (§3, §5). Logic unchanged (raw pipe, ring snapshot). Optionally validate the session belongs to the path's wsId before upgrading (requires workspace-scoped registry).
- `api/internal/api/v0/endpoints/terminal/handlers/handlers.go` — Add a TerminalBroadcaster dependency (Push(dto.TerminalSessionDTO)) to Handlers + New(). Add a ListSessions dependency (workspace-scoped engine listing) for the new GET .../terminals route. WorkspaceReader.Get still resolves by wsId (domain.Workspace already carries ProjectID/RepoID/WorktreePath, so PTY cwd needs no extra params).
- `api/internal/api/v0/dto/terminal.go` — Expand TerminalSessionDTO from {SessionID} to the full lifecycle DTO (id, workspaceId, projectId, repoId, profileId, status active|ended, createdAt, endedAt) so Broadcaster[TerminalSessionDTO] can namespace/filter and the FE can cache by id (spec §5, §1, §6). Add TerminalSessionDTOFrom + TerminalSessionDTOList converters. Note FE-breaking rename sessionId→id (see risks).
- `api/internal/app/usecases/terminal/terminal.go` — Wire this usecase into router (currently bypassed). CreateSession: resolve ws (has ProjectID/RepoID/WorktreePath), spawn with workspaceDir=ws.WorktreePath (§5/OQ#2 already correct), then broadcast TerminalSessionDTO{status:active}. KillSession: broadcast {status:ended,endedAt}. Add ListSessions(ctx, wsID) returning per-workspace sessions. Start passing workspaceID through engine.Create (currently ignored).
- `api/internal/engine/terminal/terminal.go` — Use the workspaceID param in Create (drop the _ at :117) so sessions can be keyed by workspace. Add ListSessionsForWorkspace(workspaceID) []string. Add a callback (mirroring LSP OnDiagnostics, container.go:64) e.g. OnSessionEnded(fn) so reapOnDone (terminal.go:146) can emit the 'ended' lifecycle DTO when a PTY exits on its own — the engine currently has no broadcaster/hub reference.
- `api/internal/engine/terminal/internal/registry/registry.go` — Add a workspaceID dimension so listing can be scoped per workspace (§3 GET .../workspaces/:wsId/terminals). Either store workspaceID per entry + ListByWorkspace(workspaceID) []string, or use map[workspaceID]map[sessionID]*Session. Update Add/Get/Remove/List and all engine callers.
- `api/internal/api/v0/container.go` — Add a terminals *ws.Broadcaster[dto.TerminalSessionDTO] field + terminalsDef StreamDef (Namespace=projectId/repoId/wsId per §5; Filters projectId+repoId+wsId ExactMatch mirroring workspacesDef; Snapshot=engine ListSessionsForWorkspace→DTOs; Serialize=json.Marshal). Add a PushTerminalSession path. Update the container doc comment (container.go:16-22): it asserts Terminal is NOT a Broadcaster[T] — true for the PTY stream, now false for the lifecycle topic; clarify the split.
- `api/internal/api/v0/router.go` — Update the terminal.Register call to pass c.terminals.Handle + ws.DualServe (mirroring workspaces.Register at router.go:47-54) so GET/WS .../workspaces/:wsId/terminals is wired.
- `api/internal/app/hub/subscriber.go` — If lifecycle events route through the hub: add PushTerminalSession(d dto.TerminalSessionDTO) to Subscriber + implement on the v0 Container. Optional if the usecase calls broadcaster.Push directly — choose the path consistent with how workspace mutations broadcast, and update all Subscriber mocks if the interface grows.
- `api/internal/api/v0/endpoints/terminal/handlers/profiles.go` — No scope change (profiles stay global per §3). Confirm FE still hits /v0/settings/terminal/profiles and whether the code's /:id sub-routes (GET/PUT/DELETE) remain — §3 route tree lists only collection-level verbs.
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go` — Rewrite per §8 to UUID-based For(crowbarHome, projectID, repoID, workspaceID) string (no error; returns .../workspaces/<W>/worktree) + StorageDir/RepoDir/ProjectDir. The terminal PTY cwd flows from ws.WorktreePath computed here; add/expect a TerminalsStorageDir helper (.../workspaces/<W>/terminals/storages) for §1. Owned by another subagent; the terminal create path must READ the new ws.WorktreePath, not reconstruct paths itself.

### New contracts
- // api/internal/api/v0/dto/terminal.go — expanded lifecycle DTO
type TerminalSessionDTO struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	ProjectID   string    `json:"projectId"`
	RepoID      string    `json:"repoId"`
	ProfileID   string    `json:"profileId,omitempty"`
	Status      string    `json:"status"` // "active" | "ended"
	CreatedAt   time.Time `json:"createdAt"`
	EndedAt     time.Time `json:"endedAt,omitempty"`
}
- // api/internal/domain/terminal_session.go — NEW domain struct + GORM table (spec §1/§2 terminal_sessions)
type TerminalSession struct {
	ID          string     `gorm:"primaryKey" json:"id"`
	WorkspaceID string     `json:"workspaceId"`
	ProfileID   string     `json:"profileId"`
	CreatedAt   time.Time  `json:"createdAt"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
}
func (TerminalSession) TableName() string { return "terminal_sessions" }
- // api/internal/api/v0/dto/terminal.go — converters
func TerminalSessionDTOFrom(
	s domain.TerminalSession,
	projectID string,
	repoID string,
) TerminalSessionDTO

func TerminalSessionDTOList(
	sessions []domain.TerminalSession,
	projectID string,
	repoID string,
) []TerminalSessionDTO
- // api/internal/api/v0/container.go — new broadcaster field + StreamDef
terminals *ws.Broadcaster[dto.TerminalSessionDTO]

func terminalsDef(
	appContainer *app.Container,
	engContainer *engine.Container,
) ws.StreamDef[dto.TerminalSessionDTO] {
	return ws.StreamDef[dto.TerminalSessionDTO]{
		Namespace: func(d dto.TerminalSessionDTO) string { return d.ProjectID + "/" + d.RepoID + "/" + d.WorkspaceID },
		Serialize: func(d dto.TerminalSessionDTO) ([]byte, error) { return json.Marshal(d) },
		Snapshot:  terminalsSnapshot(appContainer, engContainer),
		Filters: []ws.FilterDef[dto.TerminalSessionDTO]{
			{Param: "projectId", Extract: func(d dto.TerminalSessionDTO) string { return d.ProjectID }, Match: ws.ExactMatch},
			{Param: "repoId", Extract: func(d dto.TerminalSessionDTO) string { return d.RepoID }, Match: ws.ExactMatch},
			{Param: "wsId", Extract: func(d dto.TerminalSessionDTO) string { return d.WorkspaceID }, Match: ws.ExactMatch},
		},
	}
}
- // api/internal/api/v0/container.go — push entry (and/or hub.Subscriber method)
func (c *Container) PushTerminalSession(
	d dto.TerminalSessionDTO,
)
- // api/internal/engine/terminal/terminal.go — Engine surface additions
ListSessionsForWorkspace(
	workspaceID string,
) []string

OnSessionEnded(
	fn func(workspaceID string, sessionID string),
)
// and terminalEngine.Create must consume workspaceID (drop the _ at terminal.go:117)
- // api/internal/engine/terminal/internal/registry/registry.go — scoped listing
func (r *Registry) Add(
	id string,
	workspaceID string,
	s *session.Session,
)

func (r *Registry) ListByWorkspace(
	workspaceID string,
) []string
- // api/internal/app/usecases/terminal/terminal.go — usecase surface additions
CreateSession(
	ctx context.Context,
	wsID string,
	prof *domain.TerminalProfile,
) (dto.TerminalSessionDTO, error) // now returns the broadcast DTO, not bare id

ListSessions(
	ctx context.Context,
	wsID string,
) ([]dto.TerminalSessionDTO, error)
- // api/internal/api/v0/endpoints/terminal/routes.go — new Register signature
func Register(
	rg *gin.RouterGroup,
	termEng termhandlers.TerminalEngine,
	profileStore termhandlers.ProfileStore,
	wsReader termhandlers.WorkspaceReader,
	termBroadcast termhandlers.TerminalBroadcaster,
	wsHandle gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
)
- // api/internal/api/v0/endpoints/terminal/handlers/handlers.go — new dependency interface
type TerminalBroadcaster interface {
	Push(d dto.TerminalSessionDTO)
}
- // Concrete hierarchical route paths (spec §3)
POST   /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals
GET/WS /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals            // list / lifecycle stream (dual-served)
DELETE /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals/:sessionId
WS     /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals/:sessionId/ws // raw PTY (NOT Broadcaster[T])
GET    /v0/settings/terminal/profiles (POST/PUT/DELETE)  // unchanged, global

### Risks
- SPEC CONTRADICTION on POST .../terminals status code: §3 route tree says 'create terminal session → 201 {sessionId}' but §4 table classifies POST .../terminals as a 202 Asynx-backed domain mutation. FE needs the sessionId to open the PTY WS; a 202 empty body forces the FE to wait for the lifecycle WS DTO before connecting (an ordering change). Resolve before implementing — safest is 201 {sessionId} sync PLUS a lifecycle broadcast (deviates from the §4 table).
- domain.TerminalSession, the terminal_sessions GORM table, and a per-workspace view.db store do NOT exist yet — §1/§2 assume all three. The current code has zero session persistence (registry is in-memory only). Building Broadcaster[TerminalSessionDTO].Snapshot from persisted rows requires net-new storage; alternatively derive the snapshot purely from the in-memory engine registry (no DB). Decide, because §1 explicitly lists a terminal_sessions table.
- The flat registry keys sessions by sessionID ONLY. Adding a workspace dimension touches Add/Get/Remove/List and every engine caller (Create/Kill/Shutdown/reapOnDone). reapOnDone (terminal.go:146) currently only removes from the registry with no way to emit an 'ended' DTO; wiring an ended-broadcast there crosses an engine→broadcaster boundary that does not exist today — the engine has no hub/broadcaster reference and must gain a callback (like LSP OnDiagnostics, container.go:64).
- The terminal Usecase (app/usecases/terminal/terminal.go) is currently DEAD CODE — router.go wires handlers straight to the engine + GORM store + workspace repo, bypassing it. Moving lifecycle broadcasting into the usecase means either re-plumbing router.go to route handlers through the usecase or duplicating broadcast logic in handlers. Pick one to avoid two divergent create paths.
- Broadcaster snapshot race: the lifecycle broadcaster's Snapshot() runs OUTSIDE b.mu (broadcaster.go:143) while reading the engine registry under its own lock; a session can end (reapOnDone) between snapshot computation and the next live Push. Full-state-replace semantics make this mostly safe, but an 'ended' DTO pushed before a client's snapshot is taken could be overwritten by a stale 'active' snapshot. Mirror the workspace topic's idempotent full-replace and ensure 'ended' is reflected in the snapshot until consumed.
- worktreepath.go rewrite (§8, owned by another subagent) changes how ws.WorktreePath is computed. The PTY cwd flows from ws.WorktreePath; if the workspace repo starts returning .../worktree (new layout) vs the old .../workspaces/<wsId>, PTYs spawned mid-migration could target the wrong dir. Pre-production wipe (OQ#1) mitigates, but the terminal create path must READ the new WorktreePath, never reconstruct the path.
- Profile /:id sub-routes exist in code (routes.go:24,26,27) but §3 route tree lists only collection-level GET/POST/PUT/DELETE. If the FE migration drops /:id, GetProfile/UpdateProfile/DeleteProfile-by-id handlers may be orphaned or break — confirm the FE contract.
- Filter param resolution (filter.go resolveFilterValue) reads path param FIRST then query. The dual-served GET .../workspaces/:wsId/terminals uses :projectId/:repoId/:wsId PATH params; the namespace uses projectId/repoId/wsId. The Filters MUST use the same param names as the route :params (projectId, repoId, wsId) or path-first resolution silently broadcasts ALL sessions to every client.
- hub.Subscriber has 4 methods; adding PushTerminalSession changes the interface and every implementer/test double. Search all hub.Subscriber implementations and mocks before extending — an unimplemented method breaks compilation across the app layer. (Avoidable if the usecase calls broadcaster.Push directly instead of going through the hub.)
- Two TerminalSessionDTO consumers will exist: the HTTP create response and the WS frame. They must be byte-identical so the FE cache-merge (§6 'every WS message is a complete DTO, cache.set(dto.id)') treats them as one entity. The current DTO key is 'sessionId'; §6 IndexedDB keys by 'id'. Renaming sessionId→id is an FE-breaking change to coordinate with the frontend subagent.

### Test targets
- api/internal/engine/terminal/internal/registry/registry_test.go — TestRegistry_AddWithWorkspace, TestRegistry_ListByWorkspace_ScopesToWorkspace, TestRegistry_ListByWorkspace_EmptyForUnknownWs, TestRegistry_Remove_DropsFromWorkspaceIndex (target 100% coverage, no sleep).
- api/internal/engine/terminal/terminal_test.go — TestEngine_Create_KeysSessionByWorkspace, TestEngine_ListSessionsForWorkspace, TestEngine_OnSessionEnded_FiresOnReap (drive PTY exit by Kill and select on a callback-signalled channel — NOT time.Sleep).
- api/internal/api/v0/dto/terminal_test.go — TestTerminalSessionDTOFrom_PopulatesProjectRepoWsIDs, TestTerminalSessionDTOFrom_ActiveAndEndedStatus, TestTerminalSessionDTOList_NonNilEmptySlice (envelope carries [] not null).
- api/internal/app/usecases/terminal/terminal_test.go — TestCreateSession_UsesWorktreePathAsCWD, TestCreateSession_BroadcastsActiveDTO, TestKillSession_BroadcastsEndedDTO, TestListSessions_ScopedToWorkspace (mock engine + mock broadcaster recording pushes synchronously; no sleep).
- api/internal/api/v0/endpoints/terminal/handlers/handlers_test.go + a sessions handler test — TestCreateSession_ParsesProjectRepoWsParams, TestCreateSession_ReturnsExpectedStatus (per resolved §3/§4 decision), TestCreateSession_404OnUnknownWorkspace, TestKillSession_404OnUnknownSession, TestListSessions_ReturnsScopedList.
- api/internal/api/v0/endpoints/terminal/routes_test.go — assert the new hierarchical routes are registered (POST/GET/WS/DELETE under /projects/:projectId/repos/:repoId/workspaces/:wsId/terminals and /:sessionId/ws) and profiles stay at /settings/terminal/profiles.
- api/internal/api/v0/container_terminals_test.go (new) — TestTerminalsDef_NamespaceIsProjectRepoWs, TestTerminalsDef_FiltersScopeByProjectRepoWs, TestTerminalsDef_SnapshotFromEngine (table-driven via BuildPredicate/ExactMatch).
- api/internal/engine/terminal/internal/registry/registry_bench_test.go (new) — BenchmarkListByWorkspace (DB-registry-style lookup bench per §13 perf-sensitive paths).
- api/tests/terminal_test.go (build tag integration) — UPDATE TestTerminal_CreateStreamKill to hierarchical paths; ADD TestRegression_TerminalSession_LifecycleBroadcast: POST .../workspaces/:wsId/terminals → open WS .../workspaces/:wsId/terminals → block on a context deadline (no sleep) for TerminalSessionDTO{status:active,id,workspaceId,projectId,repoId} → DELETE → assert WS pushes {status:ended}.
- api/tests/terminal_test.go — ADD TestRegression_TerminalSession_NamespaceFiltering: create sessions in two workspaces, connect a workspace-scoped WS, assert only that workspace's DTOs arrive (mirror the workspaces namespace-filtering regression).
- api/tests/terminal_test.go — ADD TestRegression_Terminal_CWDIsWorktree: create a session, run `pwd` over the PTY WS, assert the worktree path is echoed (proves §5/OQ#2 working-dir contract) using the deadline-bounded readTerminalUntil helper (no sleep).
- api/tests/integration/terminal/terminal_test.go — extend TerminalSuite to the hierarchical routes + the lifecycle WS list/snapshot, blocking on WS frames via kit Env helpers (no time.Sleep).

---

## Agent-run removal + chat TODO boundary

### Key signatures
- `api/internal/domain/agent_run.go`
  - type AgentRun struct { ID, WsID, ChatID string; Status AgentRunStatus; CreatedAt time.Time } (agent_run.go:7-13)
- `api/internal/domain/agent_run_status.go`
  - type AgentRunStatus string (agent_run_status.go:4)
  - AgentRunStatusPending/Running/Done/Error/Interrupted (agent_run_status.go:7-11)
- `api/internal/domain/chat.go`
  - type Chat struct { ID, WsID, Title, ParentID string; Status ChatStatus; Type ChatType; CreatedAt time.Time; DeletedAt *time.Time } (chat.go:7-16)
- `api/internal/domain/chat_status.go`
  - type ChatStatus string (chat_status.go:4)
  - ChatStatusIdle, ChatStatusAgentRunning (chat_status.go:7-8)
- `api/internal/domain/chat_type.go`
  - ChatTypeChat, ChatTypeWorkflow (chat_type.go:8-9)
- `api/internal/domain/branch_chat.go`
  - type BranchChat struct { ID, Title, Age string; IsActive bool } (branch_chat.go:6-11)
- `api/internal/domain/workspace.go`
  - Workspace.AgentRunning bool `json:"agentRunning"` (workspace.go:34)
- `api/internal/api/v0/dto/workspace.go`
  - WorkspaceDTO.AgentRunning bool `json:"agentRunning"` (workspace.go:29, mapped at :54)
  - WorkspaceDTOFrom(w domain.Workspace) WorkspaceDTO (workspace.go:34)
- `api/internal/api/v0/endpoints/agentrun/routes.go`
  - Register(rg *gin.RouterGroup, repo agentrunhandlers.AgentRunRepo) (routes.go:13)
  - POST /workspaces/:wsId/runs, GET /runs/running, POST /runs/:id/start|complete|fail, GET /runs/:id (routes.go:16-21)
- `api/internal/api/v0/endpoints/agentrun/handlers/handlers.go`
  - type AgentRunRepo interface { Create/ListRunning/MarkRunning/Complete/Fail/Get } (handlers.go:12-39)
  - New(repo AgentRunRepo) *Handlers (handlers.go:47)
- `api/internal/api/v0/endpoints/agentrun/handlers/runs.go`
  - Create/ListRunning/Start/Complete/Fail/Get (runs.go:14-108)
- `api/internal/app/repositories/agentrun/agentrun.go`
  - type AgentRun interface { Create/MarkRunning/Complete/Fail/Get/List/ListRunning/ListByChat } (agentrun.go:21-55)
  - New(ax, db, broadcast) (AgentRun, error) (agentrun.go:64)
- `api/internal/app/repositories/agentrun/internal/store/storage.go`
  - agentRunRow.TableName() = read_agent_runs (storage.go:19-21)
  - newStorageStore(db) (storage.go:50)
- `api/internal/app/repositories/agentrun/internal/store/projections.go`
  - registerProjections(st, ax, broadcast) (projections.go:22)
  - ax.Subscribe(Topic("agent_run.*"), p.onEvent) (projections.go:28)
- `api/internal/app/repositories/agent_run_projection.go`
  - RegisterAgentRunProjection(ax, chats chat.Chat, runs agentrun.AgentRun) error (agent_run_projection.go:20)
  - agentRunProjector.applyChatStatus / hasOtherLiveRun (agent_run_projection.go:44, :65)
  - isTerminal(status) (agent_run_projection.go:82)
- `api/internal/app/repositories/recovery.go`
  - RecoverAgentRuns(ctx, runs agentrun.AgentRun) (recovery.go:14)
  - recoverOneRun (recovery.go:28)
  - ReconcileChats(ctx, chats chat.Chat, runs agentrun.AgentRun) (recovery.go:48)
  - liveChatSet / reconcileOneChat (recovery.go:67, :83)
- `api/internal/app/repositories/container.go`
  - Container.AgentRun field (container.go:22)
  - New(... axAgentRun asynx.Asynx[domain.AgentRun] ...) (container.go:29-36)
  - agentrun.New broadcast → c.refreshWorkspace (container.go:48-52)
  - broadcastWorkspace sets ws.AgentRunning = c.hasLiveAgentRun (container.go:99)
  - ListWorkspacesWithOverlay (container.go:108)
  - hasLiveAgentRun / runningWorkspaceIDs (container.go:122, :129)
  - RegisterHubProjections(axAgentRun) (container.go:78)
  - RecoverOrphans (container.go:148)
- `api/internal/app/repositories/chat/chat.go`
  - Chat interface incl. ResetIdle(ctx,id) and SetAgentRunning(ctx,id) (chat.go:47-54)
  - uses commands.ResetChatIdle / commands.SetChatAgentRunning (chat.go:150, :161)
- `api/internal/app/repositories/chat/internal/commands/set_agent_running.go`
  - SetChatAgentRunning{ID} EventName "chat.agent_running."+ID (set_agent_running.go:12-22)
- `api/internal/app/repositories/chat/internal/commands/reset_idle.go`
  - ResetChatIdle{ID} EventName "chat.idle_reset."+ID (reset_idle.go:14-23)
- `api/internal/app/usecases/chat/chat.go`
  - Usecase interface ListChatsByWorkspace/CreateChat/ForkChat/RenameChat/DeleteChat (chat.go:77-116)
- `api/internal/app/usecases/internal/branchchat/branchchat.go`
  - From(chats []domain.Chat, now time.Time) []domain.BranchChat (branchchat.go:14)
  - IsActive: c.Status == domain.ChatStatusAgentRunning (branchchat.go:25)
- `api/internal/app/usecases/branchreview/branch_review.go`
  - assemble(...) Conversations: branchchat.From(chats, u.now()) (branch_review.go:157)
- `api/internal/app/hub/chat_status_event.go`
  - ChatStatusEvent{ChatID, WsID string; Status domain.ChatStatus} (chat_status_event.go:6-10)
- `api/internal/app/container.go`
  - axAgentRun, err := newAsynx[domain.AgentRun](adapters.AgentRunES) (container.go:47)
  - repositories.New(... axAgentRun ...) (container.go:62)
  - repos.RegisterHubProjections(axAgentRun) (container.go:66)
  - repos.RecoverOrphans(ctx) (container.go:69)
- `api/internal/adapter/container.go`
  - Container.AgentRunES asynxModels.Store (container.go:22)
  - AgentRunES: stores[2] (container.go:97)
  - openEventStores names = {workspace.db, chat.db, agent_run.db, review_thread.db} (container.go:160)
- `api/internal/api/v0/router.go`
  - import endpoints/agentrun (router.go:6)
  - agentrun.Register(rg, c.app.Repositories.AgentRun) (router.go:101-104)
- `api/internal/api/v0/snapshots.go`
  - workspacesSnapshot → appContainer.Repositories.ListWorkspacesWithOverlay (snapshots.go:18-22)
  - chatsSnapshot → Repositories.Chat.List → ChatStatusEvent (snapshots.go:33-47)
- `api/internal/api/v0/container.go`
  - chats *ws.Broadcaster[hub.ChatStatusEvent] (container.go:29)
  - chatStream *ws.Broadcaster[ChatFrame] (container.go:33)
  - PushChat (container.go:118)
  - chatsDef / chatStreamDef (container.go:159, :218)
- `api/tests/integration/agentrun/agentrun_test.go`
  - AgentRunSuite: TestAgentRun_CreateStartComplete/CreateStartFail/ListRunning/GetUnknownReturns404 (agentrun_test.go:35-258)
- `api/tests/integration/lifecycle/lifecycle_test.go`
  - TestLifecycle_ChatAgentRunDrivesChatStatusOverWS (lifecycle_test.go:144) uses POST /v0/workspaces/:wsId/runs + /runs/:id/start (lifecycle_test.go:175-185)
- `api/tests/integration/crash/crash_test.go`
  - TestCrash_AgentRunRecoveryMarksOrphansError (crash_test.go:41) creates a stuck AgentRun and asserts running→error
- `api/tests/kit/env.go`
  - DialChats (env.go:245)
  - WaitForChat (env.go:355)
  - WaitForWorkspace (env.go:333)
  - MutationID (env.go:522)

### Must change
- `api/internal/domain/agent_run.go` — DELETE file. Spec §3 Open-Q 3 / §12: agent-run concept eliminated entirely.
- `api/internal/domain/agent_run_status.go` — DELETE file. No remaining referencer once recovery + projection are removed (spec §3, §5).
- `api/internal/api/v0/endpoints/agentrun` — DELETE entire directory (routes.go, handlers/handlers.go, handlers/runs.go, handlers/handlers_test.go, routes_test.go). No /runs routes in spec §3 route tree.
- `api/internal/app/repositories/agentrun` — DELETE entire directory tree (agentrun.go + internal/commands, internal/store, internal/mocks + all *_test.go + store_bench_test.go). Spec §12 keeps only chat domain; agent-run repo is gone.
- `api/internal/app/repositories/agent_run_projection.go` — DELETE file (+ agent_run_projection_test.go). The agent_run.* → chat status projection no longer exists (spec §3 chat TODO, §5 Working always false).
- `api/internal/app/repositories/recovery.go` — DELETE RecoverAgentRuns + recoverOneRun + ReconcileChats + liveChatSet + reconcileOneChat (the whole file becomes empty of behavior). Update container.RecoverOrphans accordingly. Spec §3/§5: no agent-running producer means nothing to recover/reconcile.
- `api/internal/app/repositories/container.go` — Remove AgentRun field, axAgentRun param, agentrun.New construction, refreshWorkspace, hasLiveAgentRun, runningWorkspaceIDs, RegisterHubProjections, and RecoverOrphans body. In broadcastWorkspace + ListWorkspacesWithOverlay: drop the AgentRunning overlay (set ws.Working = false per spec §5). Keep chat repo wiring (broadcastChat) as TODO.
- `api/internal/app/repositories/chat/chat.go` — Remove SetAgentRunning + ResetIdle from the Chat interface and impl (no remaining caller after projection/recovery deletion). Spec §3 chat TODO: chat keeps create/fork/rename/delete/list only.
- `api/internal/app/repositories/chat/internal/commands/set_agent_running.go` — DELETE file (+ its case in commands_test.go). Orphaned command; ChatStatusAgentRunning has no producer (spec §5).
- `api/internal/app/repositories/chat/internal/commands/reset_idle.go` — DELETE file (+ its case in commands_test.go). Orphaned once recovery + projection are gone.
- `api/internal/domain/chat_status.go` — Remove ChatStatusAgentRunning (or leave enum but document it as never-produced). Recommended: drop AgentRunning, keep only ChatStatusIdle, since spec §5 says Working is always false until chat is implemented. Coordinate with branch_chat.go IsActive removal.
- `api/internal/domain/branch_chat.go` — Remove IsActive (always false after agent_run removal) OR hardcode false. Spec §5: no agent/chat activity indicator exists in scope. Update branchchat.From accordingly.
- `api/internal/app/usecases/internal/branchchat/branchchat.go` — Drop the `IsActive: c.Status == domain.ChatStatusAgentRunning` line (set false or remove field). Spec §5 removes the agent-running concept.
- `api/internal/domain/workspace.go` — Rename AgentRunning → Working bool `json:"working"` per spec §5 (Working = active chat/agent session, always false until chat implemented). Keep the comment that it is a non-persisted derived overlay (now always false in scope).
- `api/internal/api/v0/dto/workspace.go` — Rename AgentRunning → Working bool `json:"working"` in WorkspaceDTO + WorkspaceDTOFrom mapping. Spec §5 WorkspaceDTO uses `Working bool` (TODO: always false until chat).
- `api/internal/app/container.go` — Remove axAgentRun construction (line 47), the axAgentRun arg to repositories.New (line 62), the RegisterHubProjections call (line 66). Update or drop RecoverOrphans (line 69) — keep only if any chat reconcile remains (it does not). Spec §3/§9/§12.
- `api/internal/adapter/container.go` — Remove AgentRunES field (line 22), stores[2] assignment (line 97), and "agent_run.db" from openEventStores names (line 160). Re-index ReviewThreadES. (Broader §9 lazy-open refactor is a separate scope; agent-run scope only removes the agent_run.db store.)
- `api/internal/api/v0/router.go` — Remove the endpoints/agentrun import (line 6) and the agentrun.Register block (lines 101-104). Spec §3 has no /runs routes.
- `api/internal/api/v0/snapshots.go` — Change workspacesSnapshot to call Repositories.Workspace.List (overlay removed). Update chatsSnapshot only if chat WS is kept (it is, as TODO). Spec §5 (no agent overlay), §3 (chat TODO).
- `api/tests/integration/agentrun/agentrun_test.go` — DELETE file. /runs routes removed (spec §13: only routes that exist get TestRegression coverage).
- `api/tests/integration/lifecycle/lifecycle_test.go` — DELETE TestLifecycle_ChatAgentRunDrivesChatStatusOverWS (lines ~142-200) — depends on /runs and agent-running. Keep git lifecycle cases.
- `api/tests/integration/crash/crash_test.go` — DELETE TestCrash_AgentRunRecoveryMarksOrphansError (and any chat-reconcile-via-run case). RecoverAgentRuns no longer exists (spec §12 keeps only what's in scope).
- `api/internal/api/v0/snapshots_test.go` — Update/remove assertions referencing the agent-running overlay (uses AgentRunning per earlier grep).
- `api/internal/api/v0/wave3_integration_test.go` — Update/remove agent-run related assertions (grep hit on AgentRun/agentrun).
- `api/internal/app/container_test.go` — Remove agent-run wiring assertions (axAgentRun / RegisterHubProjections / RecoverOrphans).
- `api/internal/app/repositories/container_test.go` — Remove AgentRun field, overlay, RegisterHubProjections, RecoverOrphans test cases.
- `api/internal/adapter/container_test.go` — Remove AgentRunES assertions and the agent_run.db open expectation.

### New contracts
- // internal/domain/workspace.go — replace AgentRunning with Working (spec §5)
// Working is a derived, non-persisted overlay: true when a chat/agent session is
// active. TODO: always false until chat is implemented.
Working bool `json:"working"`
- // internal/api/v0/dto/workspace.go — WorkspaceDTO field rename (spec §5)
Working bool `json:"working"`
- // internal/api/v0/dto/workspace.go — mapping in WorkspaceDTOFrom
Working: w.Working,
- // internal/domain/chat_status.go — agent-running removed (spec §5)
type ChatStatus string
const (
    ChatStatusIdle ChatStatus = "idle"
)
- // internal/domain/branch_chat.go — IsActive dropped (no agent-running producer in scope)
type BranchChat struct {
    ID    string `json:"id"`
    Title string `json:"title"`
    Age   string `json:"age"`
}
- // internal/app/usecases/internal/branchchat/branchchat.go — From no longer reads agent status
func From(
    chats []domain.Chat,
    now time.Time,
) []domain.BranchChat
- // internal/app/repositories/chat/chat.go — Chat interface loses SetAgentRunning + ResetIdle
type Chat interface {
    Create(ctx context.Context, id string, wsID string, title string, now time.Time) (domain.Chat, error)
    Fork(ctx context.Context, id string, wsID string, parentID string, title string, now time.Time) (domain.Chat, error)
    Rename(ctx context.Context, id string, title string) (domain.Chat, error)
    Delete(ctx context.Context, id string, now time.Time) (domain.Chat, error)
    Get(ctx context.Context, id string) (domain.Chat, error)
    List(ctx context.Context) ([]domain.Chat, error)
    ListByWorkspace(ctx context.Context, wsID string) ([]domain.Chat, error)
}
- // internal/app/repositories/container.go — New loses the agent-run Asynx param
func New(
    db *gormdb.DB,
    h hub.WebSocketHub,
    axWorkspace asynx.Asynx[domain.Workspace],
    axChat asynx.Asynx[domain.Chat],
    axReviewThread asynx.Asynx[domain.ReviewThread],
) (*Container, error)
- // internal/app/repositories/container.go — Container struct loses AgentRun
type Container struct {
    Workspace    workspace.Workspace
    Chat         chat.Chat
    ReviewThread reviewthread.ReviewThread
    hub          hub.WebSocketHub
}
- // internal/app/repositories/container.go — overlay removed; broadcast pushes Working=false
func (c *Container) broadcastWorkspace(
    ctx context.Context,
    ws domain.Workspace,
) {
    ws.Working = false
    c.hub.BroadcastWorkspace(ws)
}
- // internal/app/repositories/container.go — list without overlay (rename or simplify)
func (c *Container) ListWorkspaces(
    ctx context.Context,
) ([]domain.Workspace, error)
- // internal/adapter/container.go — Container struct without AgentRunES
type Container struct {
    WorkspaceES    asynxModels.Store
    ChatES         asynxModels.Store
    ReviewThreadES asynxModels.Store
    DB             *gormdb.DB
    closers        []io.Closer
    lock           *instanceLock
}
- // internal/adapter/container.go — openEventStores name list without agent_run.db
names := []string{"workspace.db", "chat.db", "review_thread.db"}
- // internal/app/container.go — repositories.New call site loses axAgentRun; remove RegisterHubProjections + RecoverOrphans
repos, err := repositories.New(adapters.DB, h, axWorkspace, axChat, axReviewThread)
- // internal/api/v0/router.go — agentrun.Register block removed entirely (no replacement route)
- // Routes removed (spec §3 route tree contains none of these):
// POST   /v0/workspaces/:wsId/runs
// GET    /v0/runs/running
// POST   /v0/runs/:id/start
// POST   /v0/runs/:id/complete
// POST   /v0/runs/:id/fail
// GET    /v0/runs/:id
- // Chat routes remain marked TODO per spec §3 (NOT deleted, NOT remounted in this PR):
// // TODO: /v0/projects/:p/repos/:r/workspaces/:w/chats — multi-agent conversation, future feature

### Risks
- SHARED FILE — repositories/container.go is the densest coupling point: it owns the AgentRun field, the agent-run→chat projection registration, the workspace AgentRunning overlay (broadcast + snapshot), and RecoverOrphans. Other subsystems (workspace broadcaster, snapshots.go) call broadcastWorkspace/ListWorkspacesWithOverlay. Renaming/removing these must be coordinated with the workspace-broadcaster subsystem owner to avoid a compile break or a silent loss of the Working field.
- SHARED FILE — app/container.go, adapter/container.go, api/v0/router.go, api/v0/snapshots.go are each touched by multiple subsystems in this refactor (lazy DB open §9, hierarchical routes §3, broadcaster §5). Removing agent-run lines must not clobber concurrent edits; agent-run scope should make the minimal targeted deletions and leave the §9 lazy-open rewrite to the adapter subsystem owner.
- ChatStatusAgentRunning is referenced by chat_status.go, branch_chat.go, branchchat.go, the chat commands set_agent_running.go/reset_idle.go, agent_run_projection.go, recovery.go, AND chat repo SetAgentRunning/ResetIdle. Removing the enum value will not compile until ALL of these are removed/updated in the same change — order: delete projection + recovery body first, then chat commands + repo methods, then enum, then branch_chat/branchchat.
- The chat domain is explicitly KEPT as TODO (spec §3, Open-Q 3) — do NOT delete domain/chat.go, chat repo store, chat usecase, chat WS broadcasters, or chat integration helpers. The risk is over-deletion: only the agent-run linkage and the agent-running status producer are removed, not chat itself.
- AgentRunning→Working JSON rename is a wire-contract break. The frontend currently reads `agentRunning`; after this change it must read `working` (spec §5/§6). FE workspace cache + sidebar derived-view code (hasWorking = workspaces.some(w => w.working)) must land in the same release or the indicator silently breaks.
- Removing /runs collapses the only path that ever set Chat.Status to agent-running. After removal, chatsSnapshot + the chats WS broadcaster will only ever emit idle. Keeping the chat WS surface alive is harmless but be aware its agent-running frames are now dead — tests asserting agent-running over WS (lifecycle, agentrun suites) MUST be deleted, not merely skipped.
- Crash recovery: RecoverOrphans currently runs synchronously at startup (app/container.go:69) via SendWait. If RecoverOrphans is reduced to a no-op or removed, confirm no startup ordering assumption depends on it (the provider sweep + realtime start after it). Removing it should be safe but verify container_test.go startup-order assertions.
- Asynx event-store re-index: adapter/container.go assigns stores by positional index (stores[0..3]). Dropping agent_run.db (index 2) shifts ReviewThreadES from stores[3] to stores[2]. An off-by-one here silently wires the review-thread aggregate to the wrong DB. Must update both the names slice AND the field assignments together.
- SPEC ASSUMES NOT-YET-EXISTING: spec §5 WorkspaceDTO also drops Locked/HasConflicts/PendingMerge and adds LastError/CanMergeLocally/ParentBranch/MergeStrategy/PRTitle/Status semantics. Those are OWNED BY OTHER SUBSYSTEMS (workspace status/DTO). The agent-run scope must ONLY do the AgentRunning→Working rename and must not unilaterally remove Locked/HasConflicts or it will collide with the workspace-status subsystem.
- GORM read_agent_runs table: deleting the agentrun store means the table is no longer auto-migrated. Per project memory (No Legacy Migration) this is fine pre-production — no migration/drop code needed; dev state is wiped (spec §0, Open-Q 1). Do NOT write a DROP TABLE migration.
- agentrun store has a *_bench_test.go (store_bench_test.go). Deleting the package removes the benchmark; spec §13 requires benchmarks for performance-sensitive paths — confirm no remaining path needs the bench (none do; agent-run is gone).
- internal/api/v0/defs_test.go, watcher_lifecycle_integration_test.go, realtime/watcher_dispatcher_internal_test.go appeared in the broad grep — verify they only reference Chat (kept) and not AgentRun before assuming no change; the grep matched on substring 'chat'/'Chat'.

### Test targets
- DELETE api/tests/integration/agentrun/agentrun_test.go (entire /runs suite — routes removed).
- EDIT api/tests/integration/lifecycle/lifecycle_test.go — remove TestLifecycle_ChatAgentRunDrivesChatStatusOverWS; keep git lifecycle cases (TestLifecycle_*Sync, *Git).
- EDIT api/tests/integration/crash/crash_test.go — remove TestCrash_AgentRunRecoveryMarksOrphansError and any chat-reconcile-via-live-run case.
- DELETE unit tests co-located with deleted source: api/internal/app/repositories/agentrun/**/*_test.go (agentrun_test.go, internal/commands/commands_test.go, internal/store/{projections,storage,store,store_bench}_test.go), api/internal/app/repositories/agent_run_projection_test.go, api/internal/app/repositories/recovery_test.go (agent-run half), api/internal/api/v0/endpoints/agentrun/**/*_test.go.
- EDIT api/internal/app/repositories/chat/internal/commands/commands_test.go — remove SetChatAgentRunning + ResetChatIdle cases.
- EDIT api/internal/app/repositories/chat/chat_test.go — remove SetAgentRunning + ResetIdle method tests.
- EDIT api/internal/app/usecases/internal/branchchat/branchchat_test.go — remove IsActive/agent-running assertions; assert From no longer sets active.
- EDIT api/internal/api/v0/dto/workspace_test.go — assert json key is `working` (not `agentRunning`); update WorkspaceDTOFrom mapping test.
- NEW/EDIT api/internal/app/repositories/container_test.go — assert Container has no AgentRun field, New takes no axAgentRun, broadcastWorkspace pushes Working=false, ListWorkspaces returns rows without overlay.
- EDIT api/internal/adapter/container_test.go — assert only 3 event stores opened (workspace.db, chat.db, review_thread.db), no AgentRunES, ReviewThreadES correctly wired post re-index.
- EDIT api/internal/app/container_test.go — assert startup wires no agent-run Asynx and does not call RegisterHubProjections/RecoverOrphans.
- EDIT api/internal/api/v0/snapshots_test.go — workspacesSnapshot returns plain Workspace.List (no agent overlay); assert Working=false on snapshot rows.
- EDIT api/internal/api/v0/wave3_integration_test.go — remove /runs and agent-running assertions.
- NEW blackbox TestRegression in api/tests/integration (integration tag) — TestRegression_WorkspaceDTOWorkingFieldDefaultsFalse: create workspace, GET/WS, assert dto.working == false and key present (synchronize via kit.WaitForWorkspace / WS context deadline, NO time.Sleep).
- NEW blackbox TestRegression — TestRegression_RunsRoutesGone: POST /v0/workspaces/:wsId/runs and GET /v0/runs/running return 404 (routes removed).
- NEW unit test — branchchat From: given chats with any status, all BranchChat have no active/agent-running indicator (IsActive removed).

---

## Frontend data layer (api client + stores + types + WS/IndexedDB cache)

### Key signatures
- `web/src/lib/api.ts`
  - api.ts:5 export const API_BASE: string
  - api.ts:22 export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T>
  - api.ts:18 export function isNotFoundError(err: unknown): boolean
  - api.ts:53 export function fetchWorkspace(wsId: string): Promise<WorkspacePayload>  -> GET /v0/workspaces/${wsId}
  - api.ts:59 export function postWorkspace(repoId, branch, parentId?): Promise<{id}>  -> POST /v0/workspaces body {repoId,branch,parentId?}
  - api.ts:71 export function deleteWorkspace(wsId): Promise<void>  -> DELETE /v0/workspaces/${wsId}
  - api.ts:75 export function fetchProjects(): Promise<Project[]>  -> GET /v0/projects
  - api.ts:79 export function fetchProject(id): Promise<Project>  -> GET /v0/projects/${id}
  - api.ts:83 export function postRepo(projectId, name, path): Promise<{id,projectId,name,path,defaultBranch}>  -> POST /v0/repos body {id,projectId,name,path}
  - api.ts:100 export async function fetchLandingWorkspaceId(): Promise<{id,projectId,repoId}|null>  -> GET /v0/workspaces (flat, cross-project)
  - api.ts:114 export function fetchPrerequisites(): Promise<Prerequisites>  -> GET /v0/system/prerequisites
  - api.ts:119 export function postProject(name, path, quick?): Promise<{id}>  -> POST /v0/projects
- `web/src/lib/types.ts`
  - types.ts:2 interface WorkspacePayload { id; projectId; repoId; branch }
  - types.ts:21 interface Project { id; name; path; lastActivity: Date }
  - types.ts:28 interface Prerequisites { git; gh; glab }
- `web/src/lib/loadable.ts`
  - loadable.ts:1 type Loadable<T>
  - loadable.ts:15 success<T>(data, at?)
  - loadable.ts:28 dataOf<T>(l?): T | undefined
- `web/src/lib/store/loadable-slice.ts`
  - loadable-slice.ts:5 interface LoadableSlice<T,K>
  - loadable-slice.ts:28 createLoadableSlice<T,K>(cfg)
  - loadable-slice.ts:60 applyDelta -> setTimeout(DELTA_DEBOUNCE_MS=120) then get().fetch(...)
- `web/src/lib/store/workspace-list.ts`
  - workspace-list.ts:9 async function fetchRepoTree(): Promise<Repo[]>  -> GET /v0/repos + GET /v0/workspaces
  - workspace-list.ts:17 useWorkspaceListStore = createLoadableSlice({store:'workspaces-data', wsEndpoint:()=>'/v0/ws/workspaces'})
- `web/src/lib/store/build-repo-tree.ts`
  - build-repo-tree.ts:22 interface RepoDTO { id; projectId; name; path; defaultBranch; avatarLabel; avatarColor; avatarUrl? }
  - build-repo-tree.ts:33 interface WorkspaceDTO { id; repoId; projectId; parentId?; branch; status; locked; hasConflicts; added; deleted; mergeStrategy; agentRunning }
  - build-repo-tree.ts:48 toSidebarStatus(ws) — maps agentRunning/locked/status onto sidebar status
  - build-repo-tree.ts:70 buildRepoTree(repos, workspaces): Repo[]
- `web/src/lib/store/sidebar.ts`
  - sidebar.ts:17 type WorkspaceStatus = 'locked'|'new'|'pr-open'|'pr-closed'|'pr-merged'|'agent-running'
  - sidebar.ts:25 interface Workspace { id; branch; parentId?; status?; added?; deleted?; age; hasConflicts? }
  - sidebar.ts:36 interface Repo { id; projectId?; name; avatarLabel; avatarColor; avatarURL?; workspaces }
  - sidebar.ts:147 addWorkspace(repoId,wsId,branch,parentId?)
  - sidebar.ts:168 deleteWorkspace(wsId) — BFS skipping locked
  - sidebar.ts:250 mergeRepos(incoming) — additive merge
- `web/src/lib/store/projects.ts`
  - projects.ts:18 useProjectDataStore = createLoadableSlice({store:'projects-data', fetcher: fetchProjects})
  - projects.ts:26 useProjectStore (persist activeProjectId)
  - projects.ts:43 export function importProjectAndSync(project): void
- `web/src/lib/store/workspace-route-guard.ts`
  - workspace-route-guard.ts:14 shouldRedirectUnknownWorkspace(listStatus, repos, wsId): boolean
- `web/src/lib/ws/manager.ts`
  - manager.ts:13 interface WSManager { subscribe(endpoint, cb): ()=>void; send(endpoint, data) }
  - manager.ts:31 createWSManager(): WSManager
  - manager.ts:127 export const wsManager
- `web/src/lib/ws/url.ts`
  - url.ts:9 export function wsUrl(path: string): string
  - url.ts:26 export function isWebSocketCapable(): boolean
- `web/src/lib/ws/types.ts`
  - types.ts:1 interface WorkspaceEvent { workspaceId; action }
  - types.ts:9 interface FileEvent { workspaceId; path }
- `web/src/lib/ws/connection-store.ts`
  - connection-store.ts:37 reportChannelState(endpoint, open)
  - connection-store.ts:43 reportChannelGone(endpoint)
- `web/src/lib/persistence/cache-store.ts`
  - cache-store.ts:4 type CacheStoreName = 'workspaces-data'|'git-data'|'file-tree-data'|'branch-review-data'|'chat-history'|'projects-data'|'chats-data'
  - cache-store.ts:19 saveCache<T>(store, key, data, fetchedAt?)
  - cache-store.ts:33 loadCache<T>(store, key): Promise<CachedRecord<T>|undefined>
- `web/src/lib/persistence/idb.ts`
  - idb.ts:9 openDB<CrowbarDB>('crowbar', 6, {upgrade})
  - idb.ts:56 export function resetDB(): void
- `web/src/lib/persistence/schemas.ts`
  - schemas.ts:64 interface CachedRecord<T> { key; data; fetchedAt }
  - schemas.ts:70 interface CrowbarDB extends DBSchema { ... 'workspaces-data': {key:string; value:CachedRecord<unknown>} ... }
- `web/src/components/app-sync-provider.tsx`
  - app-sync-provider.tsx:7 AppSyncProvider({children})
- `web/src/components/layout/workspace-tree-context.tsx`
  - workspace-tree-context.tsx:21 performCreateWorkspace(repoId, branch, parentId?)
  - workspace-tree-context.tsx:40 performDeleteWorkspace(wsId)
  - workspace-tree-context.tsx:60 performReparentWorkspace(wsId, newParentId, repoId)
- `web/src/lib/api/workspace.ts`
  - workspace.ts:13 reparentWorkspace(wsId, newParentId, repoId): Promise<void>  -> POST /v0/workspaces/${wsId}/reparent
  - workspace.ts:32 handleWorkspaceReparented(wsId, newParentId, repoId)
- `web/src/components/projects/add-repository-modal.tsx`
  - add-repository-modal.tsx:59 const repo = await postRepo(activeProjectId, repoName, trimmedPath)
  - add-repository-modal.tsx:64 const { id: wsId } = await postWorkspace(repoId, branch)
- `web/src/components/projects/import-project-modal.tsx`
  - import-project-modal.tsx:55 const { id } = await postProject(...)
  - import-project-modal.tsx:56 const project = await fetchProject(id)
- `web/src/components/workspace/new-workspace-page.tsx`
  - new-workspace-page.tsx:22 const ws = await postWorkspace(data.repoId, data.branch)
- `web/src/components/layout/repo-settings-panel.tsx`
  - repo-settings-panel.tsx:42 apiFetch(`/v0/repos/${repoId}/branches`)
  - repo-settings-panel.tsx:57 apiFetch('/v0/workspaces', POST {repoId,branch})
  - repo-settings-panel.tsx:100 PUT /v0/repos/${repoId}/icon (multipart)
  - repo-settings-panel.tsx:115 PUT /v0/repos/${repoId}/icon/emoji
  - repo-settings-panel.tsx:133 PUT /v0/repos/${repoId}/icon/github
  - repo-settings-panel.tsx:145 DELETE /v0/repos/${repoId}/icon
- `web/src/routes/_shell/index.tsx`
  - _shell/index.tsx:39 Promise.allSettled([fetchProjects(), fetchLandingWorkspaceId()])
- `web/src/features/files/lib/file-tree-api.ts`
  - file-tree-api.ts:32 fetchFileTree(wsId, path?)  -> GET /v0/workspaces/${wsId}/files/tree
  - file-tree-api.ts:40 filesWsEndpoint(wsId): string  -> /v0/ws/files?wsId=
- `web/src/features/workspace/stores/hooks/use-workspace-effects.ts`
  - use-workspace-effects.ts:49 useWorkspaceEffects(wsId)
  - use-workspace-effects.ts:199 wsManager.subscribe(`/v0/ws/git?wsId=${wsId}`, ...)
- `web/src/features/git/stores/git-store.ts`
  - git-store.ts:39 fetchAllGitData(wsId): Promise<GitData>
  - git-store.ts:106 wsEndpoint: (wsId) => `/v0/ws/git?wsId=${wsId}`
- `web/src/features/git/api/git-status-api.ts`
  - git-status-api.ts:11 POST /v0/workspaces/${wsId}/git/${action}
  - git-status-api.ts:31 GET /v0/workspaces/${wsId}/git/status
- `web/src/lib/crowbar-bridge.ts`
  - crowbar-bridge.ts:40 terminalCreate(wsId, profileId?)  -> POST /v0/workspaces/${wsId}/terminals
  - crowbar-bridge.ts:64 WebSocket(/v0/ws/terminals/${sessionId})
  - crowbar-bridge.ts:117 DELETE /v0/terminals/${id}
- `web/src/lib/store/chat-list-store.ts`
  - chat-list-store.ts:6 useChatListStore = createLoadableSlice({store:'chats-data', fetcher GET .../chats, wsEndpoint /v0/ws/chats?wsId=})

### Must change
- `web/src/lib/api.ts` — Rewrite all entity functions to hierarchical routes (§3,§7): postProject/deleteProject stay /v0/projects but become 202-empty-body; postRepo→POST /v0/projects/:p/repos (202); postWorkspace→POST /v0/projects/:p/repos/:r/workspaces body {branch,parentId?} (202, no {id} return); deleteWorkspace→DELETE /v0/projects/:p/repos/:r/workspaces/:w (202). fetchWorkspace→GET /v0/projects/:p/repos/:r/workspaces/:w. Replace fetchLandingWorkspaceId (no cross-project list exists; §7) with project-scoped landing resolution. Update apiFetch to tolerate 202 (already treats empty body as success — verify 202 falls through res.ok path). Add fetchRepos(projectId) and fetchWorkspaces(projectId,repoId).
- `web/src/lib/types.ts` — Add the canonical WorkspaceDTO/RepoDTO/ThreadDTO/ThreadReplyDTO/TerminalSessionDTO TS interfaces matching §5 exactly (camelCase). Expand WorkspacePayload or replace usages with WorkspaceDTO. These are the single source of truth the cache stores.
- `web/src/lib/store/build-repo-tree.ts` — Update WorkspaceDTO to §5 shape: remove `locked`,`hasConflicts`,`agentRunning`; add status union ('new'|'locked'|'pr-conflicts'|'deleted'|'pr-merged'|'pr-open'|'pr-closed'), `working`, `lastError`, `forkPointSha`, `parentId`, `mergeStrategy`, `canMergeLocally`, `parentBranch`, `prUrl`, `prTitle`, `prTargetBranch`. Rewrite toSidebarStatus to pass status through (working/locked/conflicts now ARE statuses). buildRepoTree stays a grouping helper but the new model may build the sidebar from the entity cache instead.
- `web/src/lib/store/loadable-slice.ts` — Core §6 change: applyDelta must MERGE the incoming complete DTO into the entity cache (upsert by id; status:'deleted' removes after animation) — NOT schedule a debounced refetch. Remove DELTA_DEBOUNCE_MS refetch path. fetch becomes the one-time seed (read IDB → GET → merge). Likely superseded by a new per-entity cache store; if kept, repurpose for list-seed only.
- `web/src/lib/store/workspace-list.ts` — Rewrite to per-project scope (§7): K becomes [projectId] (or [projectId,repoId] fan-out). fetchRepoTree→GET /v0/projects/:p/repos; per repo subscribe WS /v0/projects/:p/repos/:r/workspaces and seed via GET. Drop the flat /v0/repos + /v0/workspaces parallel fetch and /v0/ws/workspaces endpoint.
- `web/src/lib/store/sidebar.ts` — Extend WorkspaceStatus with 'pr-conflicts' and 'deleted'; remove 'agent-running' (use `working` flag). Drop hasConflicts boolean (derive from status). deleteWorkspace must become WS-driven (handle status:'deleted' DTO: keep briefly for animation then remove) rather than optimistic-on-HTTP. Add fields needed by §6 derived indicators (prUrl, working, canMergeLocally, parentBranch) to Workspace. addWorkspace optimistic insert may be removed in favor of WS-seeded cache.
- `web/src/lib/store/projects.ts` — Wire a project WS stream (wsEndpoint /v0/projects) so imported/deleted projects arrive live (§6). Remove importProjectAndSync's post-mutation double-refetch — the new ProjectDTO/RepoDTO arrive on WS and merge into cache. Keep useProjectStore.activeProjectId (needed as projectId source for hierarchical URLs).
- `web/src/lib/store/workspace-route-guard.ts` — Adjust the 'success' gate to reference the per-project/per-repo workspace cache load state instead of a single global list status (§7), so a redirect only fires once that repo's workspaces have actually seeded.
- `web/src/lib/ws/manager.ts` — On reconnect ({reconnected:true}) trigger a full GET re-seed of the affected entity cache rather than a loadable refetch (§6 reconnect recovery / Open Q4). No URL change needed (endpoint-keyed already supports hierarchical paths).
- `web/src/lib/ws/types.ts` — Delete/replace the thin notification envelopes (WorkspaceEvent{workspaceId,action}, FileEvent{workspaceId,path}). WS frames are now complete DTOs (§5). Keep only TerminalFrame (raw PTY pipe) and any genuinely non-DTO frames.
- `web/src/lib/persistence/cache-store.ts + new entity-cache module` — Add an entity-cache layer (§6): per-entity object stores crowbar_projects/crowbar_repos/crowbar_workspaces/crowbar_threads keyed by id, with upsert(dto)/getAll()/remove(id). cache.set(dto.id,dto) write-through. Keep the existing list-cache for non-entity data (git/file-tree).
- `web/src/lib/persistence/idb.ts + schemas.ts` — Bump DB version to 7; create the four entity object stores keyed by 'id'. Add daemon-version-change wipe-and-reseed logic (§6: 'On daemon version change: wipe all object stores and re-seed from GET'). Declare the new stores in CrowbarDB DBSchema with their DTO value types.
- `web/src/components/app-sync-provider.tsx` — Replace global fetch+startSync with the §7 startup sequence driven by the active projectId: GET /v0/projects/:p/repos → for each repo open WS .../workspaces + GET seed. Open WS /v0/projects for the project list. Remove the workspace-list→mergeRepos refetch bridge (cache is WS-live).
- `web/src/components/layout/workspace-tree-context.tsx` — performCreateWorkspace/performDeleteWorkspace: thread projectId+repoId; treat 202 as accepted, disable the action, and stop optimistically mutating the sidebar — let the WS DTO (status:'new'→idle, or status:'deleted') drive the cache. Read lastError from the updated DTO for inline error rendering. performReparentWorkspace uses hierarchical reparent route.
- `web/src/lib/api/workspace.ts` — reparentWorkspace→POST /v0/projects/:p/repos/:r/workspaces/:w/reparent (202+WS). Remove local handleWorkspaceReparented hierarchy persistence — parentId now lives on the WorkspaceDTO delivered by WS.
- `web/src/components/projects/add-repository-modal.tsx` — Use hierarchical postRepo(activeProjectId)/postWorkspace(projectId,repoId). Since 202 returns no ids, resolve repoId/wsId/defaultBranch from the WS RepoDTO/WorkspaceDTO events (subscribe before POST) then navigate. Remove post-mutation refetch+mergeRepos.
- `web/src/components/projects/import-project-modal.tsx` — postProject returns 202 empty body — replace the synchronous fetchProject(id) with resolution of the imported ProjectDTO from the /v0/projects WS stream (subscribe before POST), then onImport.
- `web/src/components/workspace/new-workspace-page.tsx` — Remove hardcoded REPOS mock; source repos from the project's repo cache. postWorkspace 202 → navigate after the WorkspaceDTO arrives on WS. Thread projectId+repoId.
- `web/src/components/layout/repo-settings-panel.tsx` — Migrate to hierarchical routes: GET /v0/projects/:p/repos/:r/branches; branch import POST .../workspaces (202+WS, drop refetch); icon PUT/DELETE/emoji/github under /v0/projects/:p/repos/:r/icon[/...] (sync 200). Accept projectId as a prop (currently only repoId/repoName).
- `web/src/routes/_shell/index.tsx` — Replace fetchLandingWorkspaceId (no cross-project endpoint under §7) with: GET /v0/projects → pick a project → GET its repos/workspaces (or use persisted last-active /ide route) to compute the landing redirect.
- `web/src/features/files/lib/file-tree-api.ts + use-workspace-effects.ts + features/git/api/*.ts + git-store.ts + lsp-client.ts + crowbar-bridge.ts` — Thread projectId+repoId into every workspace-scoped URL/WS topic and migrate to §3 hierarchical paths: files/tree, files/ws, git/status (DTO), git ops (stage/commit 200; push/fetch/pull/merge/rebase 202+WS), lsp/*, terminals + terminals/:id/ws. These consumers currently only receive wsId — they must receive projectId+repoId from the TanStack route params.

### New contracts
- // web/src/lib/types.ts — §5 canonical DTOs (camelCase mirror of Go)
- export interface WorkspaceDTO {
  id: string
  repoId: string
  projectId: string
  branch: string
  parentId: string
  forkPointSha: string
  status: 'new' | 'locked' | 'pr-conflicts' | 'deleted' | 'pr-merged' | 'pr-open' | 'pr-closed'
  working: boolean
  lastError: string
  added: number
  deleted: number
  mergeStrategy: string
  canMergeLocally: boolean
  parentBranch: string
  prUrl: string
  prTitle: string
  prTargetBranch: string
}
- export interface RepoDTO {
  id: string
  projectId: string
  name: string
  path: string
  defaultBranch: string
  avatarLabel: string
  avatarColor: string
  avatarUrl: string // proxied /v0/projects/:p/repos/:r/icon endpoint
}
- export interface ThreadReplyDTO {
  id: string
  threadId: string
  body: string
  author: string
  createdAt: string
}
- export interface ThreadDTO {
  id: string
  workspaceId: string
  filePath: string
  line: number
  side: 'old' | 'new'
  body: string
  author: string
  resolved: boolean
  createdAt: string
  replies: ThreadReplyDTO[]
}
- export interface TerminalSessionDTO {
  id: string
  workspaceId: string
  profileId: string
  createdAt: string
  endedAt: string | null
}
- // web/src/lib/api.ts — hierarchical, 202-aware
- export function fetchRepos(projectId: string): Promise<RepoDTO[]>  // GET /v0/projects/${projectId}/repos
- export function fetchWorkspaces(projectId: string, repoId: string): Promise<WorkspaceDTO[]>  // GET /v0/projects/${projectId}/repos/${repoId}/workspaces
- export function fetchWorkspace(projectId: string, repoId: string, wsId: string): Promise<WorkspaceDTO>  // GET .../workspaces/${wsId}
- export function postWorkspace(
  projectId: string,
  repoId: string,
  branch: string,
  parentId?: string,
): Promise<void>  // POST /v0/projects/${projectId}/repos/${repoId}/workspaces body {branch, parentId?} -> 202
- export function deleteWorkspace(projectId: string, repoId: string, wsId: string): Promise<void>  // DELETE .../workspaces/${wsId} -> 202
- export function postRepo(projectId: string, name: string, path: string): Promise<void>  // POST /v0/projects/${projectId}/repos -> 202
- export function postProject(name: string, path: string, quick?: boolean): Promise<void>  // POST /v0/projects -> 202
- // web/src/lib/api/workspace.ts
- export async function reparentWorkspace(
  projectId: string,
  repoId: string,
  wsId: string,
  newParentId: string,
): Promise<void>  // POST .../workspaces/${wsId}/reparent -> 202
- // Concrete route paths the frontend dials (§3)
- GET  /v0/projects
- POST /v0/projects
- GET  /v0/projects/:projectId/repos
- POST /v0/projects/:projectId/repos
- GET  /v0/projects/:projectId/repos/:repoId/branches
- PUT  /v0/projects/:projectId/repos/:repoId/icon | /icon/emoji | /icon/github
- DELETE /v0/projects/:projectId/repos/:repoId/icon
- GET  /v0/projects/:projectId/repos/:repoId/workspaces
- POST /v0/projects/:projectId/repos/:repoId/workspaces
- DELETE /v0/projects/:projectId/repos/:repoId/workspaces/:wsId
- GET  /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/files/tree
- WS   /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/files/ws
- GET  /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/status
- POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals
- WS   /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals/:sessionId/ws
- // WebSocket subscription endpoints (per §3 dispatch() upgrade)
- WS   /v0/projects
- WS   /v0/projects/:projectId/repos          (repo-scoped, prefix p/)
- WS   /v0/projects/:projectId/repos/:repoId/workspaces (ws-list scoped, prefix p/r/)
- WS   /v0/projects/:projectId/repos/:repoId/workspaces/:wsId (single ws)
- // New entity-cache module: web/src/lib/persistence/entity-cache.ts
- type EntityStoreName = 'crowbar_projects' | 'crowbar_repos' | 'crowbar_workspaces' | 'crowbar_threads'
- export function upsertEntity<T extends { id: string }>(store: EntityStoreName, dto: T): Promise<void>
- export function getAllEntities<T>(store: EntityStoreName): Promise<T[]>
- export function removeEntity(store: EntityStoreName, id: string): Promise<void>
- // idb.ts v7 upgrade adds stores keyed by 'id'
- for (const name of ['crowbar_projects','crowbar_repos','crowbar_workspaces','crowbar_threads'] as const) db.createObjectStore(name, { keyPath: 'id' })
- // loadable-slice.ts applyDelta new contract (merge, not refetch)
- applyDelta: (dto: WorkspaceDTO, ...args: K) => Promise<void>  // upsert dto by id into entity cache; if dto.status==='deleted' schedule removal

### Risks
- SHARED loadable-slice.ts: workspace-list, projects, git-store, and chat-list-store ALL consume createLoadableSlice. Changing applyDelta from 'refetch' to 'merge DTO' affects git-store (which gets non-DTO trigger frames) and chat-list-store (out of scope). The refactor must either special-case per store or fork a new entity-cache slice for DTO channels while leaving git/chat on the legacy refetch slice.
- 202 with EMPTY body removes the synchronous {id}/{defaultBranch}/{sessionId} return that add-repository-modal, import-project-modal, new-workspace-page, and crowbar-bridge.terminalCreate all depend on for immediate navigation. Spec §3 says terminals POST returns 201 {sessionId} (exception) but repo/workspace/project POSTs return 202 empty — every caller that navigates on the returned id must be re-architected to await the WS DTO, OR the daemon must echo the id in the 202 (spec text conflicts: §3 'returns 202' vs callers needing the id). This ambiguity is a blocker to resolve.
- isWebSocketCapable() returns FALSE on the desktop Tauri crowbar:// unix-socket transport (ws/url.ts). The entire §6 model assumes a live WS per scope; on desktop ALL DTO streaming is silently skipped (manager.subscribe early-returns), so the cache would only ever be seeded by GET and never updated — mutations would appear stuck. A native Tauri WS bridge (like the existing terminal Channel bridge in crowbar-bridge.ts) is REQUIRED for the desktop build and is not yet present (per MEMORY: Unix Socket Transport). This is a hard dependency the spec does not address.
- fetchLandingWorkspaceId relies on a flat cross-project GET /v0/workspaces that §7 eliminates ('sidebar never needs a cross-project workspace fetch'). routes/_shell/index.tsx beforeLoad has no projectId at landing time — landing logic must be redesigned (pick first project, or persist last-active /ide route) or it breaks cold start.
- Optimistic sidebar mutations (sidebar.ts addWorkspace/deleteWorkspace + workspace-tree-context) race the WS DTO. If both run, a delete could remove a node the WS later re-adds, or a create could duplicate. The spec §6 mandates WS-driven cache with optimistic state removed; partial migration (some optimistic, some WS) will cause flicker/duplication — must convert atomically.
- Cascade delete: sidebar deleteWorkspace BFS-removes non-locked descendants locally; the backend now owns rm -rf and emits a status:'deleted' DTO PER deleted workspace. The frontend must remove each deleted id as its DTO arrives (not BFS locally), or the tree desyncs when the backend's deletion set differs from the client's BFS.
- WorkspaceStatus enum is referenced widely (sidebar.ts, build-repo-tree.ts, and every consumer rendering badges). Adding 'pr-conflicts'/'deleted' and removing 'agent-running'/hasConflicts is a breaking type change across many components; exhaustive switch statements will need updating and may currently silently fall through.
- IndexedDB version bump to 7 + 'wipe on daemon version change' has no current mechanism; existing idb.ts upgrade ladder only ADDS stores. A wipe path that drops the new entity stores must coexist with the additive ladder, and the daemon version must be exposed to the client (no such field exists today).
- ws/manager.ts reconnect emits {reconnected:true} as a callback payload — if applyDelta naively merges it as a DTO it will corrupt the cache (no id). The merge path must distinguish the reconnect sentinel from real DTOs and route it to a full re-seed.
- review-api.ts threads currently live under /v0/workspaces/:wsId/review/threads, but §3 introduces a first-class /v0/projects/:p/repos/:r/workspaces/:w/threads with ThreadDTO+Broadcaster. There are now two thread models (the persisted branch-review-slice ReviewThread in IDB vs backend ThreadDTO) — they must be reconciled or the review panel will show two divergent thread sources.
- build-repo-tree.ts avatarURL prefixes API_BASE for the proxied icon; the icon route moves to /v0/projects/:p/repos/:r/icon. RepoDTO.avatarUrl from the backend must already be the hierarchical path or the sidebar avatar 404s.

### Test targets
- web/src/__tests__/lib/ws/contract-paths.test.ts — UPDATE: pin new hierarchical WS endpoints (.../workspaces, .../workspaces/:w, .../git/status, .../files/ws, .../terminals/:id/ws) instead of /v0/ws/git?wsId / /v0/ws/files?wsId; assert per-repo workspace subscription fan-out.
- web/src/__tests__/lib/api/mutation-contract.test.ts — UPDATE: assert postProject/postRepo/postWorkspace/deleteWorkspace hit hierarchical URLs and treat 202 (no body) as success without throwing; assert no synchronous entity is returned.
- web/src/__tests__/lib/api/workspace.test.ts — UPDATE: reparentWorkspace dials POST /v0/projects/:p/repos/:r/workspaces/:w/reparent.
- web/src/__tests__/lib/store/workspace-list.test.ts — UPDATE: fetch with projectId scope; mock GET /v0/projects/:p/repos and per-repo GET workspaces.
- web/src/__tests__/lib/store/loadable-slice.test.ts — UPDATE/ADD: applyDelta upserts a complete DTO into the entity cache by id (NO refetch); status:'deleted' removes after animation; reconnect sentinel triggers full re-seed not a corrupt upsert. (Event-driven: resolve a deferred when cache.set fires; no time.Sleep.)
- web/src/__tests__/lib/store/build-repo-tree.test.ts — UPDATE: WorkspaceDTO §5 shape; status passthrough for 'pr-conflicts'/'locked'/'deleted'; `working` replaces agentRunning; canMergeLocally/parentBranch present.
- web/src/__tests__/lib/store/sidebar.test.ts — UPDATE: deleted-status handling, pr-conflicts status, removal of hasConflicts boolean; WS-driven delete instead of optimistic BFS.
- web/src/__tests__/lib/persistence/entity-cache.test.ts — NEW: upsertEntity/getAllEntities/removeEntity round-trip across crowbar_projects/repos/workspaces/threads; best-effort degradation on missing store.
- web/src/__tests__/lib/persistence/idb-schema.test.ts — UPDATE: v7 creates the four entity stores keyed by 'id'; daemon-version-change wipe-and-reseed path.
- web/src/__tests__/lib/store/projects.test.ts + projects-loadable.test.ts — UPDATE: project WS stream wiring; importProjectAndSync no longer double-refetches (ProjectDTO arrives via WS).
- web/src/__tests__/lib/store/workspace-route-guard.test.ts — UPDATE: redirect gates on the per-repo workspace cache seed state, not a single global list.
- web/src/__tests__/lib/ws/manager.test.ts — UPDATE: reconnect emits a re-seed signal that callers route to GET, not a DTO merge.
- Backend blackbox-integration (api/tests, build tag integration, no time.Sleep — block on WS with context deadline via Asynx WaitForState/WaitForCount): TestRegression_WorkspaceCreate_202_then_WS_new_then_ready; TestRegression_WorkspaceDelete_202_then_WS_deleted; TestRegression_GitPush_202_then_WS_lastError_or_status; TestRegression_WorkspaceListNamespaceFiltering_project_repo_ws; TestRegression_ProviderPoll_pr_open_to_pr_merged_mockProvider; TestRegression_MergeEligibility_CanMergeLocally_true_false; TestRegression_WorkspaceCreate_remoteBranchExists_checkout_vs_createFromParent; TestRegression_Icon_serve_upload_emoji_github_reset_sync_200.
- Component tests (vitest, mock wsManager): add-repository-modal/import-project-modal/new-workspace-page — assert navigation happens AFTER the WS DTO (subscribe-before-POST), not after the HTTP response; repo-settings-panel — hierarchical icon/branch URLs and projectId prop threading.

---

## Frontend UI flows for §14 E2E (OOBE, sidebar, terminal, editor, git) — migration to hierarchical /v0/projects/:p/repos/:r/workspaces/:w API + scoped WebSocket broadcasters and IndexedDB-backed entity cache

### Key signatures
- `web/src/lib/api.ts`
  - apiFetch<T>(path, init) (api.ts:22) — envelope {success,error,data} unwrap; treats 204/null body as undefined
  - fetchWorkspace(wsId) -> /v0/workspaces/${wsId} (api.ts:53)
  - postWorkspace(repoId, branch, parentId?) -> POST /v0/workspaces body {repoId,branch,parentId?} returns {id} (api.ts:59)
  - deleteWorkspace(wsId) -> DELETE /v0/workspaces/${wsId} (api.ts:71)
  - fetchProjects() -> /v0/projects (api.ts:75)
  - fetchProject(id) -> /v0/projects/${id} (api.ts:79)
  - postRepo(projectId,name,path) -> POST /v0/repos body {id,projectId,name,path} returns {id,projectId,name,path,defaultBranch} (api.ts:83)
  - fetchLandingWorkspaceId() -> GET /v0/workspaces, picks first !locked (api.ts:100)
  - postProject(name,path,quick?) -> POST /v0/projects returns {id} (api.ts:119)
- `web/src/lib/store/workspace-list.ts`
  - fetchRepoTree() -> Promise.all([GET /v0/repos, GET /v0/workspaces]) then buildRepoTree (workspace-list.ts:9)
  - useWorkspaceListStore = createLoadableSlice({store:'workspaces-data', cacheKey:'workspaces', wsEndpoint:()=>'/v0/ws/workspaces'}) (workspace-list.ts:17)
- `web/src/lib/store/build-repo-tree.ts`
  - interface RepoDTO {id,projectId,name,path,defaultBranch,avatarLabel,avatarColor,avatarUrl?} (build-repo-tree.ts:22)
  - interface WorkspaceDTO {id,repoId,projectId,parentId?,branch,status,locked,hasConflicts,added,deleted,mergeStrategy,agentRunning} (build-repo-tree.ts:33)
  - toSidebarStatus(ws) maps agentRunning->'agent-running', locked->'locked' (build-repo-tree.ts:48)
  - buildRepoTree(repos,workspaces) groups workspaces under repos, prefixes avatarUrl with API_BASE (build-repo-tree.ts:70)
- `web/src/lib/store/sidebar.ts`
  - WorkspaceStatus union includes 'agent-running' (sidebar.ts:17)
  - interface Workspace {id,branch,parentId?,status?,added?,deleted?,age,hasConflicts?} (sidebar.ts:25)
  - interface Repo {id,projectId?,name,avatarLabel,avatarColor,avatarURL?,workspaces[]} (sidebar.ts:36)
  - addWorkspace(repoId,wsId,branch,parentId) sets status:'new' (sidebar.ts:147)
  - deleteWorkspace(wsId) BFS removes subtree, skips locked (sidebar.ts:168)
  - mergeRepos(incoming) additive merge by id (sidebar.ts:250)
  - getPostDeleteNavigationTarget(repos,wsId) (sidebar.ts:104)
- `web/src/components/layout/workspace-tree-context.tsx`
  - performCreateWorkspace(repoId,branch,parentId?) -> postWorkspace then sidebarStore.addWorkspace (workspace-tree-context.tsx:21)
  - performDeleteWorkspace(wsId) -> apiDeleteWorkspace then sidebarStore.deleteWorkspace (workspace-tree-context.tsx:40)
  - performReparentWorkspace(wsId,newParentId,repoId) -> reparentWorkspace (workspace-tree-context.tsx:60)
  - confirmCreate(branch) calls performCreateWorkspace with creatingChildOf.{repoId,parentId} (workspace-tree-context.tsx:172)
- `web/src/components/layout/workspace-tree.tsx`
  - WorkspaceTreeInner reads useSidebarStore((s)=>s.repos) (workspace-tree.tsx:62)
  - handleWorkspaceClick(wsId,projectId,repoId) navigate /ide/$projectId/$repoId/$wsId (workspace-tree.tsx:71)
  - passes projectId={repo.projectId ?? ''} to WorkspaceTreeItem (workspace-tree.tsx:171)
  - repo icon rendering: emoji:/img/avatarURL fallback (workspace-tree.tsx:112)
- `web/src/components/layout/workspace-tree-item.tsx`
  - WorkspaceTreeItem({node,depth,repoId,projectId,activeWorkspaceId,onWorkspaceClick}) (workspace-tree-item.tsx:19)
  - isLocked = workspace.status === 'locked' (workspace-tree-item.tsx:29)
  - WorkspaceBranchIcon status={workspace.status ?? 'new'} (workspace-tree-item.tsx:86)
- `web/src/lib/api/workspace.ts`
  - reparentWorkspace(wsId,newParentId,repoId) -> POST /v0/workspaces/${wsId}/reparent (workspace.ts:13)
  - handleWorkspaceReparented(...) updates sidebar + saveWorkspaceHierarchy (workspace.ts:32)
- `web/src/components/oobe/oobe-screen.tsx`
  - OobeStep = 'presentation'|'prerequisites'|'add-project' (oobe-screen.tsx:14)
  - fetchPrerequisites() drives system check (oobe-screen.tsx:281)
  - handleImport(project) -> importProjectAndSync + setActiveProject + navigate('/') (oobe-screen.tsx:304)
- `web/src/components/projects/import-project-modal.tsx`
  - handleImport -> postProject(name,path,quick) then fetchProject(id) then onImport(project) (import-project-modal.tsx:44)
- `web/src/components/projects/add-repository-modal.tsx`
  - handleAdd -> postRepo(activeProjectId,name,path) -> postWorkspace(repoId,defaultBranch) -> useWorkspaceListStore.fetch() -> mergeRepos -> navigate (add-repository-modal.tsx:46)
- `web/src/components/layout/repo-settings-panel.tsx`
  - GET /v0/repos/${repoId}/branches -> BranchEntry{name,isProtected,hasWorkspace} (repo-settings-panel.tsx:42)
  - handleImport -> POST /v0/workspaces {repoId,branch} per selected branch then refetch+merge (repo-settings-panel.tsx:51)
  - handleFileChange -> PUT /v0/repos/${repoId}/icon (multipart) (repo-settings-panel.tsx:93)
  - handleEmojiSubmit -> PUT /v0/repos/${repoId}/icon/emoji (repo-settings-panel.tsx:110)
  - handleGithubAvatar -> PUT /v0/repos/${repoId}/icon/github (repo-settings-panel.tsx:130)
  - handleResetIcon -> DELETE /v0/repos/${repoId}/icon (repo-settings-panel.tsx:142)
- `web/src/components/workspace/new-workspace-page.tsx`
  - handleSubmit({repoId,branch}) -> postWorkspace -> addWorkspace -> navigate (new-workspace-page.tsx:19)
  - REPOS hardcoded mock list (new-workspace-page.tsx:8)
- `web/src/components/layout/ide-shell.tsx`
  - activeWorkspaceId = pathname.match(/\/ide\/[^/]+\/[^/]+\/([^/]+)/)?.[1] (ide-shell.tsx:48)
  - activeRepo = repos.find(r => r.workspaces.some(ws => ws.id===activeWorkspaceId)) (ide-shell.tsx:50)
  - activeWorkspaceRepoPath = `/repos/${activeRepo.id}` synthetic mock-era root (ide-shell.tsx:57)
  - syncs active project from owning repo (ide-shell.tsx:66)
- `web/src/routes/_shell/index.tsx`
  - beforeLoad: Promise.allSettled([fetchProjects(), fetchLandingWorkspaceId()]) (index.tsx:37)
  - redirect to /ide/$projectId/$repoId/$wsId from landing ws (index.tsx:56)
  - NoReposScreen renders AddRepositoryModal (index.tsx:15)
- `web/src/routes/_shell/ide/$projectId/$repoId/$wsId.tsx`
  - Route params {projectId,repoId,wsId} (ide/$wsId.tsx:7)
  - shouldRedirectUnknownWorkspace(listStatus,repos,wsId) (ide/$wsId.tsx:17)
- `web/src/components/app-sync-provider.tsx`
  - useWorkspaceListStore.getState().fetch()/startSync() (app-sync-provider.tsx:9)
  - useProjectDataStore.getState().startSync() (app-sync-provider.tsx:13)
  - useWorkspaceListStore.subscribe -> mergeRepos (app-sync-provider.tsx:20)
- `web/src/components/hydration-gate.tsx`
  - Promise.all([useWorkspaceListStore.fetch(), useProjectDataStore.fetch()]) (hydration-gate.tsx:20)
  - setRepos(repos) + setProjects(projects) (hydration-gate.tsx:28)
- `web/src/lib/store/loadable-slice.ts`
  - applyDelta(event,...args) debounces a full re-fetch (loadable-slice.ts:60, DELTA_DEBOUNCE_MS=120)
  - startSync(...args) -> wsManager.subscribe(wsEndpoint(...args)) (loadable-slice.ts:53)
- `web/src/lib/ws/manager.ts`
  - subscribe(endpoint,cb) returns unsubscribe; skips when !isWebSocketCapable (manager.ts:90)
  - reconnect emits {reconnected:true} sentinel to subscribers (manager.ts:81)
  - send(endpoint,data) (manager.ts:118)
- `web/src/lib/crowbar-bridge.ts`
  - terminalCreate(wsId,profileId?) -> POST /v0/workspaces/${wsId}/terminals returns {sessionId} (crowbar-bridge.ts:40)
  - browser PTY WS: /v0/ws/terminals/${sessionId} (crowbar-bridge.ts:64)
  - terminalClose(id) -> DELETE /v0/terminals/${id} (crowbar-bridge.ts:117/125)
  - isTauri() (crowbar-bridge.ts:226)
- `web/src/features/terminal/components/terminal.tsx`
  - createTerminal(config) uses getActiveWorkspaceId() then terminalCreate(wsId,profileId) (terminal.tsx:1)
  - working_directory defaults to rootFolderPath/workingDirectory (terminal.tsx:306)
- `web/src/features/editor/components/editor-surface.tsx`
  - EditorSurface({paneId,bufferId,...}) (editor-surface.tsx:66)
  - writeContent -> handleContentChange (editor-app-store) (editor-surface.tsx:239)
- `web/src/features/file-system/controllers/platform.ts`
  - filesBaseFor(wsId) -> /v0/workspaces/${wsId}/files (platform.ts:13)
  - writeFile(path,content) -> PUT ${base}/content (platform.ts:25)
  - readFile/readWorkspaceFile -> GET ${base}/content?path= (platform.ts:33/48)
  - moveFile/deleteFile/createDirectory -> PATCH/DELETE/POST base (platform.ts:75/85/93)
- `web/src/features/files/lib/file-tree-api.ts`
  - fetchFileTree(wsId,path?) -> GET /v0/workspaces/${wsId}/files/tree (file-tree-api.ts:32)
  - filesWsEndpoint(wsId) -> /v0/ws/files?wsId= (file-tree-api.ts:40)
- `web/src/features/git/api/git-status-api.ts`
  - gitPost(wsId,action,body) -> POST /v0/workspaces/${wsId}/git/${action} (git-status-api.ts:5)
  - getGitStatus(wsId) -> GET .../git/status (git-status-api.ts:29)
  - stageFile/unstageFile/discardFileChanges (git-status-api.ts:42-74)
- `web/src/features/git/api/git-commits-api.ts`
  - commitChanges(wsId,message) -> POST /v0/workspaces/${wsId}/git/commit {subject,body} (git-commits-api.ts:6)
  - getGitLog(wsId,limit,skip) -> GET .../git/log (git-commits-api.ts:21)
- `web/src/features/git/components/git-commit-panel.tsx`
  - GitCommitPanel({stagedFilesCount,repoPath,ahead,behind,onCommitSuccess}) (git-commit-panel.tsx:23)
  - handleCommit -> commitChanges(repoPath,message) where repoPath is actually wsId (git-commit-panel.tsx:50)
  - handleRemoteAction push/pull -> pushChanges/pullChanges (git-commit-panel.tsx:71)
- `web/src/lib/store/projects.ts`
  - importProjectAndSync(project) -> addProject + projectDataStore.fetch() + workspaceListStore.fetch()+mergeRepos (projects.ts:43)
  - useProjectDataStore = LoadableSlice(fetchProjects) (projects.ts:18)

### Must change
- `web/src/lib/api.ts` — §3/§7: replace every flat route with hierarchical ones. fetchProjects/postProject/fetchProject keep /v0/projects[/:id]. postWorkspace must become POST /v0/projects/:p/repos/:r/workspaces returning 202 (empty body) — drop the {id} return contract; the ready WorkspaceDTO arrives on WS. deleteWorkspace -> DELETE .../workspaces/:w (202). postRepo -> POST /v0/projects/:p/repos (202). Remove fetchLandingWorkspaceId's reliance on the global /v0/workspaces+`locked`; resolve landing from the per-project repos+workspaces GETs and WorkspaceDTO.status.
- `web/src/lib/store/workspace-list.ts` — §7: this single global store must be replaced (or re-parameterized) by a per-(projectId,repoId) workspace fetch. fetchRepoTree's GET /v0/repos + GET /v0/workspaces become GET /v0/projects/:p/repos and, per repo, GET /v0/projects/:p/repos/:r/workspaces. wsEndpoint '/v0/ws/workspaces' becomes per-repo WS /v0/projects/:p/repos/:r/workspaces.
- `web/src/lib/store/build-repo-tree.ts` — §5: rewrite WorkspaceDTO to the spec shape — drop locked/hasConflicts/agentRunning; add working,lastError,canMergeLocally,parentBranch,prUrl,prTitle,prTargetBranch,forkPointSha,mergeStrategy. Rewrite toSidebarStatus to derive from Status only (no agentRunning/locked overlays; 'locked' and 'pr-conflicts' are first-class statuses). RepoDTO: add avatarURL (proxied /icon) and drop avatarUrl ambiguity; remove client-side workspace aggregation entirely (derive hasPR/hasConflicts/hasWorking from the workspace cache per §6).
- `web/src/lib/store/sidebar.ts` — §5/§6: WorkspaceStatus union must match spec (new|locked|pr-conflicts|deleted|pr-merged|pr-open|pr-closed) — remove 'agent-running'. Workspace interface: replace hasConflicts? with status-derived; add working?,lastError? so the row can render inline errors (§4). Stop being the source of truth for mutations; become a projection of the WS-fed entity cache. addWorkspace/deleteWorkspace optimistic writes should be replaced by cache-merge of incoming WorkspaceDTO (status:'new' then ready; status:'deleted' tombstone+animation).
- `web/src/components/layout/workspace-tree-context.tsx` — §4/§7: performCreateWorkspace must take projectId+repoId, POST the hierarchical 202 endpoint, and NOT write an optimistic node — instead rely on the WorkspaceDTO{status:'new'} WS push to insert the row, and the subsequent ready DTO to transition it. performDeleteWorkspace -> hierarchical 202; removal is driven by the status:'deleted' WS tombstone, not an immediate store delete. performReparentWorkspace -> hierarchical 202+WS.
- `web/src/components/layout/workspace-tree.tsx` — §7: the projectId fed to rows must come from the route/repo entity (RepoDTO.projectId) consistently; ensure repo rows render avatarURL from the proxied /v0/projects/:p/repos/:r/icon. Derive repo-level PR/conflict badges from the workspace cache (§6), not a repo aggregate.
- `web/src/components/layout/workspace-tree-item.tsx` — §4/§5: render workspace.lastError inline alongside the row when non-empty; show status:'new' transient then idle. Status icon must handle pr-conflicts/pr-open/pr-merged/pr-closed/locked/deleted from the new union.
- `web/src/lib/api/workspace.ts` — §3: reparentWorkspace -> POST /v0/projects/:p/repos/:r/workspaces/:w/reparent (needs projectId+repoId params), 202 + WS outcome; remove the local-only reparent-to-root TODO once the backend accepts an empty parent.
- `web/src/components/oobe/oobe-screen.tsx` — §14 Step 2: OOBE must include an 'Add a repository' step after add-project (the test treats repo-add as part of OOBE). Wire it to the new hierarchical repo POST and advance only after the RepoDTO arrives on WS. Replace navigate('/') seam so OOBE owns the add-repo flow rather than deferring to NoReposScreen.
- `web/src/components/projects/import-project-modal.tsx` — §4/§6: project creation is 202+WS. Remove the post-POST fetchProject(id) race; after a 202, read the ProjectDTO from the project entity cache populated by the /v0/projects WS push. Disable the Import button after 202 (call-site disable, §4).
- `web/src/components/projects/add-repository-modal.tsx` — §3/§4: postRepo -> hierarchical 202; REMOVE the explicit postWorkspace(defaultBranch) call and the useWorkspaceListStore.fetch()+mergeRepos refetch — the backend auto-imports the default branch on repo registration and both RepoDTO and the default WorkspaceDTO arrive via WS. Navigate into the IDE only once the workspace DTO is in cache.
- `web/src/components/layout/repo-settings-panel.tsx` — §3: thread projectId into the panel (props currently only carry repoId/repoName). Migrate all icon routes to /v0/projects/:p/repos/:r/icon[/emoji|/github] and branches to /v0/projects/:p/repos/:r/branches. Replace the post-icon useWorkspaceListStore.fetch() with reliance on the RepoDTO WS push (avatar update merges into repo cache). Branch import POST -> hierarchical workspaces 202; rows update via WS, not refetch.
- `web/src/components/workspace/new-workspace-page.tsx` — §7: remove the hardcoded REPOS mock list; source repos from the per-project repo cache. postWorkspace -> hierarchical 202; navigate after the WorkspaceDTO arrives. (Likely dead for the E2E — confirm whether route should be retired.)
- `web/src/components/layout/ide-shell.tsx` — §7: resolve activeProjectId/activeRepoId from the TanStack route params (the canonical source) rather than scanning the sidebar store. Replace the synthetic '/repos/<id>' rootFolderPath with the workspace-relative root (flagged tech debt) so terminal/file/git calls use the real worktree.
- `web/src/routes/_shell/index.tsx` — §7: beforeLoad must migrate fetchProjects (unchanged path) and replace fetchLandingWorkspaceId with a project-scoped landing resolution (per-project repos+workspaces). NoReposScreen's AddRepositoryModal must use the new hierarchical repo POST.
- `web/src/components/app-sync-provider.tsx` — §6: replace the single global '/v0/ws/workspaces' subscription + full-list refetch with: per-project /v0/projects WS, and for the active project, per-repo /v0/projects/:p/repos/:r/repos + .../workspaces WS subscriptions. Each WS message is a full DTO merged into the entity cache by id — drop the mergeRepos-on-every-push refetch.
- `web/src/components/hydration-gate.tsx` — §6 startup sequence: (1) open WS for needed scopes, (2) read IndexedDB entity cache and render immediately, (3) HTTP GET to seed/overwrite by id. Re-order so the IDB cache renders before the network seed, and seed per-project/per-repo rather than two global flat lists.
- `web/src/lib/store/loadable-slice.ts` — §5/§6: change applyDelta from 'refetch-on-event' to 'merge the full DTO carried by the WS event into the cache by id, write-through to IndexedDB'. Eliminate DELTA_DEBOUNCE refetch for entity channels (keep only for non-DTO triggers if any). This is the central virtualization change.
- `web/src/lib/crowbar-bridge.ts` — §3: terminalCreate -> POST /v0/projects/:p/repos/:r/workspaces/:w/terminals (needs projectId+repoId). PTY WS -> /v0/projects/:p/repos/:r/workspaces/:w/terminals/:sessionId/ws. terminalClose DELETE -> .../terminals/:sessionId. Drop the flat /v0/ws/terminals and /v0/terminals paths.
- `web/src/features/terminal/components/terminal.tsx` — §Open-Q2/§7: resolve projectId+repoId+wsId from the route (not just getActiveWorkspaceId) for terminalCreate; stop passing the synthetic rootFolderPath as working_directory — let the daemon default cwd to the workspace worktree.
- `web/src/features/file-system/controllers/platform.ts` — §3: filesBaseFor must become /v0/projects/:p/repos/:r/workspaces/:w/files — thread projectId+repoId alongside the active wsId. writeFile/readFile/moveFile/deleteFile/createDirectory keep their methods (sync, §4) but gain the scoped prefix.
- `web/src/features/files/lib/file-tree-api.ts` — §3: fetchFileTree -> /v0/projects/:p/repos/:r/workspaces/:w/files/tree; filesWsEndpoint -> .../workspaces/:w/files/ws (drop /v0/ws/files?wsId).
- `web/src/features/git/api/git-status-api.ts` — §3: gitPost/getGitStatus route prefix -> /v0/projects/:p/repos/:r/workspaces/:w/git/... ; thread projectId+repoId. Behavior (sync 200) unchanged per §4.
- `web/src/features/git/api/git-commits-api.ts` — §3: commitChanges/getGitLog route prefix -> hierarchical. Commit stays sync 200 (§4).
- `web/src/features/git/components/git-commit-panel.tsx` — §4: rename the misleading `repoPath` prop to `wsId` (it is already the wsId); push/pull are async (202+WS) — surface workspace.lastError from the WorkspaceDTO cache instead of synchronous toast-only error handling.
- `web/src/lib/store/projects.ts` — §6: importProjectAndSync must stop doing post-mutation refetches (projectDataStore.fetch + workspaceListStore.fetch + mergeRepos). The new project arrives via the /v0/projects WS push and merges into the entity cache; the sidebar derives from cache.

### New contracts
- // web/src/lib/api.ts — hierarchical project/repo/workspace mutations (202, empty body)
- export function postProject(name: string, path: string): Promise<{ id: string }>  // POST /v0/projects (id echoed for cache pre-seed; ready ProjectDTO via WS)
- export function deleteProject(projectId: string): Promise<void>  // DELETE /v0/projects/:projectId  -> 202
- export function postRepo(projectId: string, name: string, path: string): Promise<{ id: string }>  // POST /v0/projects/:projectId/repos -> 202
- export function deleteRepo(projectId: string, repoId: string): Promise<void>  // DELETE /v0/projects/:projectId/repos/:repoId -> 202
- export function postWorkspace(projectId: string, repoId: string, branch: string, parentId?: string): Promise<{ id: string }>  // POST /v0/projects/:projectId/repos/:repoId/workspaces  body { branch, parentId? } -> 202
- export function deleteWorkspace(projectId: string, repoId: string, wsId: string): Promise<void>  // DELETE /v0/projects/:projectId/repos/:repoId/workspaces/:wsId -> 202
- export function fetchRepos(projectId: string): Promise<RepoDTO[]>  // GET /v0/projects/:projectId/repos
- export function fetchWorkspaces(projectId: string, repoId: string): Promise<WorkspaceDTO[]>  // GET /v0/projects/:projectId/repos/:repoId/workspaces
- export function fetchWorkspace(projectId: string, repoId: string, wsId: string): Promise<WorkspaceDTO>  // GET .../workspaces/:wsId
- // web/src/lib/store/build-repo-tree.ts — spec §5 DTOs
- export interface WorkspaceDTO { id: string; repoId: string; projectId: string; branch: string; parentId: string; forkPointSha: string; status: WorkspaceStatus; working: boolean; lastError: string; added: number; deleted: number; mergeStrategy: string; canMergeLocally: boolean; parentBranch: string; prUrl: string; prTitle: string; prTargetBranch: string }
- export interface RepoDTO { id: string; projectId: string; name: string; path: string; defaultBranch: string; avatarLabel: string; avatarColor: string; avatarURL: string }
- export interface ProjectDTO { id: string; name: string; path: string; createdAt: string }
- export interface ThreadDTO { id: string; workspaceId: string; filePath: string; line: number; side: 'old' | 'new'; body: string; author: string; resolved: boolean; createdAt: string; replies: ThreadReplyDTO[] }
- // web/src/lib/store/sidebar.ts — spec §5 status union
- export type WorkspaceStatus = 'new' | 'locked' | 'pr-conflicts' | 'deleted' | 'pr-merged' | 'pr-open' | 'pr-closed'
- // web/src/lib/api/workspace.ts
- export function reparentWorkspace(projectId: string, repoId: string, wsId: string, newParentId: string | undefined): Promise<void>  // POST .../workspaces/:wsId/reparent -> 202
- // web/src/lib/crowbar-bridge.ts — terminal (§3, §Open-Q2)
- export function terminalCreate(projectId: string, repoId: string, wsId: string, profileId?: string): Promise<string>  // POST /v0/projects/:p/repos/:r/workspaces/:w/terminals -> 201 { sessionId }; PTY WS = .../workspaces/:w/terminals/:sessionId/ws
- export function terminalClose(projectId: string, repoId: string, wsId: string, sessionId: string): Promise<void>  // DELETE .../workspaces/:w/terminals/:sessionId
- // web/src/features/file-system/controllers/platform.ts (§3)
- function filesBaseFor(projectId: string, repoId: string, wsId: string): string  // returns `/v0/projects/${projectId}/repos/${repoId}/workspaces/${wsId}/files`
- // web/src/features/files/lib/file-tree-api.ts (§3)
- export function filesWsEndpoint(projectId: string, repoId: string, wsId: string): string  // `/v0/projects/${p}/repos/${r}/workspaces/${w}/files/ws`
- // web/src/lib/ws scoped broadcaster endpoints the client subscribes to (§5/§6)
- WS /v0/projects                                        // Broadcaster[ProjectDTO]
- WS /v0/projects/:projectId/repos                       // Broadcaster[RepoDTO]
- WS /v0/projects/:projectId/repos/:repoId/workspaces    // Broadcaster[WorkspaceDTO] (repo-scoped fan-out)
- WS /v0/projects/:projectId/repos/:repoId/workspaces/:wsId        // Broadcaster[WorkspaceDTO] (ws-scoped, triggers per-connection provider poll)
- WS /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/status   // Broadcaster[GitStatusDTO]
- // web/src/lib/store/loadable-slice.ts merge contract (§6)
- applyDelta(dto: T & { id: string; status?: string }, ...args): merge -> cache.set(dto.id, dto); write-through IndexedDB; if dto.status === 'deleted' remove after animation

### Risks
- OOBE/repo-add coupling: §14 Step 2 treats 'add repo' as an OOBE step, but the current UI puts repo-add in NoReposScreen/AddRepositoryModal at the '/' route, not inside oobe-screen.tsx. Adding a real OOBE repo step changes the OobeStep state machine and the navigate('/') seam — verify the acceptance test against where the repo modal actually lives, or the test fails at Step 2 even though repo-add works.
- AddRepositoryModal double-create: it currently does postRepo THEN postWorkspace(defaultBranch). Spec §14 Step 3 expects the default branch ('develop') to be auto-imported by the repo POST server-side. If both the client and the backend create the default-branch workspace, Step 3 sees a duplicate 'develop' or a name conflict (the fail-fast 4xx on 'branch already taken'). Must remove the client-side postWorkspace.
- Optimistic-write vs WS-merge ordering: sidebar.ts addWorkspace/deleteWorkspace and workspace-tree-context.performCreateWorkspace write the store immediately on HTTP return. Under 202+WS there is no {id} body and no synchronous entity — if the optimistic write is kept it will create a phantom node with a client-guessed id that never reconciles with the server's WorkspaceDTO id. Removing optimism risks a visible gap between the 202 and the WS 'new' push; the call-site-disable (§4) must cover that window.
- loadable-slice refetch-on-event is load-bearing: workspace-list, projects, AND git-store (git-store.ts:106 wsEndpoint) all share createLoadableSlice. Changing applyDelta from refetch to DTO-merge touches every consumer. git-store, settings-sync, chat-list use the same factory — a merge-by-id assumption may not hold for the git status payload shape. Scope the merge change carefully or branch behavior per store.
- projectId availability gap: repo-settings-panel.tsx receives only {repoId, repoName} (workspace-tree.tsx:143 push). The hierarchical icon/branch routes need projectId. The gear button must pass repo.projectId through, and RepoDTO.projectId must be populated — if a repo row was seeded from a stale flat fetch without projectId, the icon PUT 404s.
- Synthetic rootFolderPath ('/repos/<id>', ide-shell.tsx:57) threads into 40+ files (file-explorer, path-helpers, gitignore). Spec §3 file/git/terminal routes are workspace-relative; removing the synthetic root is explicitly NOT a contained change (already caused a 404). Terminal cwd and editor open-path resolution both depend on it — high regression surface for Steps 5/6/7.
- git-commit-panel `repoPath` is actually wsId (git-changes-panel.tsx:182). Any reader assuming it's a filesystem path will break when routes change. Push/pull are 202+WS per §4 but the panel handles them as synchronous toasts — converting to WS-driven lastError needs the WorkspaceDTO cache wired into the panel, which it currently does not read.
- Cache-derived repo badges: §6 says hasPR/hasConflicts/hasWorking must derive from the workspace cache, but build-repo-tree.ts currently nests workspaces under repos and the sidebar Repo aggregate is the source. Moving to a flat workspace entity cache + derived selectors changes how WorkspaceTree renders repo headers — risk of empty/incorrect badges during the transition.
- PTY WS path migration: crowbar-bridge.ts opens the PTY over /v0/ws/terminals/:sessionId; the desktop Tauri path (terminal_open) bridges the same URL in Rust (desktop/src-tauri/src/terminal.rs). Changing the WS path requires a coordinated Rust change — a frontend-only edit silently breaks the desktop terminal (Step 5/8) while browser keeps working.
- fetchLandingWorkspaceId relies on a global cross-project /v0/workspaces list with a `locked` boolean (api.ts:100). Spec removes both the global list AND the `locked` field (subsumed by status:'locked'). The cold-start route guard (routes/_shell/index.tsx beforeLoad) will break unless landing resolution is reworked to be project-scoped and status-based.
- WS message shape assumption: lib/ws/manager.ts fans out raw parsed JSON and injects a {reconnected:true} sentinel on reconnect. The new merge-by-id applyDelta must distinguish a real DTO from the reconnect sentinel, or it will try to cache.set(undefined.id) on every reconnect.
- Spec assumes a frontend IndexedDB *entity* cache (crowbar_projects/repos/workspaces/threads object stores, §6). This does not exist yet — current persistence is LoadableSlice's stale-while-revalidate cache-store keyed by list, not per-entity. The entity cache + write-through merge is net-new client infrastructure, not a migration of existing code.

### Test targets
- web/src/__tests__/lib/api.test.ts — postWorkspace/deleteWorkspace/postRepo/deleteProject hit the hierarchical paths and treat 202 (empty body) as success returning undefined; fetchWorkspaces(projectId,repoId) builds the scoped URL
- web/src/__tests__/lib/store/build-repo-tree.test.ts — WorkspaceDTO->Workspace mapping for the new fields (lastError, working, canMergeLocally, parentBranch, prUrl) and status passthrough for pr-conflicts/pr-open/pr-merged/pr-closed/locked/deleted; RepoDTO.avatarURL proxied form
- web/src/__tests__/lib/store/sidebar.test.ts — WS-driven merge: incoming WorkspaceDTO{status:'new'} inserts a row; subsequent ready DTO transitions it; status:'deleted' tombstone removes after animation; lastError surfaces on the row
- web/src/__tests__/lib/store/loadable-slice.test.ts — applyDelta merges a full DTO by id into cache + IndexedDB (no refetch); ignores the {reconnected:true} reconnect sentinel; deletion by status:'deleted'
- web/src/__tests__/components/layout/workspace-tree-context.test.tsx — confirmCreate POSTs hierarchical 202 with projectId+repoId+parentId and does NOT optimistically write a phantom id; delete relies on the WS tombstone
- web/src/__tests__/components/projects/add-repository-modal.test.tsx — handleAdd POSTs the repo only (no client-side default-branch workspace POST) and navigates after the WorkspaceDTO is in cache
- web/src/__tests__/components/projects/import-project-modal.test.tsx — handleImport pre-seeds from the 202 id and does NOT call fetchProject(id) (no post-mutation refetch race)
- web/src/__tests__/components/layout/repo-settings-panel.test.tsx — icon PUT/DELETE and branch import use /v0/projects/:p/repos/:r/... with threaded projectId; avatar update applied from RepoDTO WS push not refetch
- web/src/__tests__/lib/crowbar-bridge.test.ts — terminalCreate POSTs .../workspaces/:w/terminals and opens .../terminals/:sessionId/ws; terminalClose DELETEs the scoped path
- web/src/__tests__/features/file-system/platform.test.ts — filesBaseFor(p,r,w) builds the hierarchical files URL; writeFile/readFile PUT/GET the scoped /files/content
- web/src/__tests__/features/git/git-status-api.test.ts + git-commits-api.test.ts — stage/unstage/discard/commit/log use the hierarchical git prefix and stay synchronous 200
- api/tests/regressions_test.go (integration) TestRegression_CreateWorkspace_RemoteBranchAbsent_CreatesFromParent — POST .../workspaces {branch:'epoch/first-pr', parentId:develop} -> 202; open WS .../workspaces, block on WorkspaceDTO{status:'new'} then ready DTO (context deadline, no time.Sleep) [§14 Step 4]
- api/tests/regressions_test.go (integration) TestRegression_RepoRegister_AutoImportsDefaultBranch — POST .../repos -> 202; WS RepoDTO arrives AND a WorkspaceDTO for the default branch ('develop') arrives without a separate workspace POST [§14 Step 3]
- api/tests/regressions_test.go (integration) TestRegression_DeleteWorkspace_EmitsDeletedTombstone — DELETE .../workspaces/:w -> 202 then WS WorkspaceDTO{status:'deleted'}
- api/tests/regressions_test.go (integration) TestRegression_Workspaces_NamespaceFiltering — a client on .../repos/:r/workspaces receives DTOs prefixed p/r/, a client on .../workspaces/:w receives only p/r/w [§5]
- api/tests/regressions_test.go (integration) TestRegression_RepoIcon_GithubAvatarServedFromDisk — PUT .../icon/github stores bytes; GET .../icon serves them; DELETE .../icon reverts to generated avatar [§14 Step 3]
- api/tests/regressions_test.go (integration) TestRegression_TerminalSession_ScopedCreateAndPtyWS — POST .../workspaces/:w/terminals -> 201 {sessionId}; PTY WS .../terminals/:sessionId/ws echoes input; DELETE kills it [§14 Step 5]
- api/tests/git_test.go (integration) TestRegression_StageCommit_ResetsCounters — stage README.md, commit, assert WorkspaceDTO added/deleted reset to 0 and commit shows in /git/log [§14 Step 7]
- api/tests/workspaces_test.go (integration) TestRegression_GitPush_PrOpenStatusViaPoll — push then mock-provider poll transitions status to pr-open on the WS WorkspaceDTO; failure path sets lastError, status unchanged [§14 Step 8, §4, §11]

---

## Existing test infrastructure and blackbox harness (api/tests + web vitest) — the patterns new unit + blackbox tests must follow for the backend API/WS/storage refactor (spec §13, no time.Sleep)

### Key signatures
- `api/tests/kit/env.go`
  - env.go:39 type Env struct { URL string; app *app.Container; engine *engine.Container; v0c *v0.Container; homeDir string; adapters *adapter.Container }
  - env.go:55 func BuildEnv(t *testing.T) *Env — wraps BuildEnvAt(t, t.TempDir())
  - env.go:67 func BuildEnvAt(t *testing.T, homeDir string) *Env — calls engine.New(ctx, engine.WithHomeDir(homeDir)), adapter.New(adapter.WithHomeDir(homeDir)), app.New(ctx,eng,adapters), v0.New(appContainer, eng), apiContainer.Register(router.Group("/v0")); registers t.Cleanup for adapters.Close + srv.Close
  - env.go:127 func (e *Env) Close(t *testing.T) — eager adapter flush for shared-homeDir restart tests
  - env.go:142 func (e *Env) PushLSP(wsID string, diags []LSPDiagnostic) — injects via e.v0c.PushLSP
  - env.go:168 func (e *Env) PushFile(evt FileEvent) — injects via e.v0c.PushFile
  - env.go:198 func (e *Env) PushGit(evt GitStatusEvent) — injects via e.v0c.PushGit
  - env.go:222 func (e *Env) DialWorkspaces(t,queryParams) *WSWatcher — dials /v0/ws/workspaces+params, then e.v0c.WaitNWorkspacesRegistered(1)
  - env.go:245/268/291/314 DialChats / DialGit / DialFiles / DialLSP (all /v0/ws/<topic>, then WaitN<topic>Registered(1))
  - env.go:333 func WaitForWorkspace(t,w,wsID,timeout,pred) map[string]any — w.ReadUntil filtering msg["id"]==wsID && pred
  - env.go:355 func WaitForChat(t,w,chatID,wsID,status,timeout) map[string]any
  - env.go:370/384/408/438/468/491 GET/POST/PUT/PATCH/DELETE/DELETEJ(t,path,body) *http.Response
  - env.go:522 func MutationID(t,resp) string — decodes {data:{id}} envelope
  - env.go:542 func DecodeEnvData(t,resp,dest) — decodes {data:...} envelope
  - env.go:560 func (e *Env) RegisterRepo(t,id,path) — POST /v0/repos {id,projectId:p1,name,path} expect 201
  - env.go:578 func (e *Env) CreateWorkspace(t,repoID,branch) string — POST /v0/workspaces {repoId,branch} expect 201 -> MutationID
  - env.go:593 func (e *Env) CreateChildWorkspace(t,repoID,branch,parentID) string
  - env.go:634 func RequireStatus(t,resp,want)
  - env.go:652 func wsURL(base,path) string — "ws"+TrimPrefix(base,"http")+path
- `api/tests/kit/ws_watcher.go`
  - ws_watcher.go:18 type WSWatcher struct { conn *websocket.Conn }
  - ws_watcher.go:24 func Dial(t,rawURL) *WSWatcher — websocket.DefaultDialer.Dial, t.Cleanup closes
  - ws_watcher.go:72 func (w *WSWatcher) ReadUntil(t,timeout,match func(map[string]any) bool) map[string]any — SetReadDeadline then loop ReadMessage until match; fails on deadline
  - ws_watcher.go:105 func (w *WSWatcher) ReadMsg(t,timeout) map[string]any
  - ws_watcher.go:117 func (w *WSWatcher) AssertNoMessage(t,timeout) bool — timeout is the SUCCESS condition (proves topic quiet)
  - ws_watcher.go:146 func (w *WSWatcher) ReadRawMsg(t,timeout) []byte — for PTY/binary frames
- `api/tests/kit/suite.go`
  - suite.go:1 //go:build integration
  - suite.go:17 type IntegrationSuite struct { suite.Suite; Env *Env }
  - suite.go:25 func (s *IntegrationSuite) SetupTest() { s.Env = BuildEnv(s.T()) }
  - suite.go:31 func Main(m *testing.M) — silences logs to ERROR, gin.SetMode(TestMode), os.Exit(m.Run())
- `api/tests/kit/repos.go`
  - repos.go:26 func InitRepo(t) string — git init -b main + empty commit
  - repos.go:64 func InitRepoWithFile(t,filename,content) string
  - repos.go:82 func CommitFile(t,repoPath,filename,content,msg)
  - repos.go:112 func WriteRepoFile(t,repoPath,filename,content)
  - repos.go:141 func BranchName(t,repoPath) string
  - repos.go:157 func RevParse(t,repoPath,rev) string
  - repos.go:174 func GitRun(t testing.TB,dir,args...) string
  - repos.go:209 func BranchExists(t,repoPath,branch) bool
  - repos.go:245 func DirExists(t,path) bool / repos.go:263 FileExists(t,path) bool
- `api/tests/kit/oracle.go`
  - oracle.go:23 func AssertWorkspaceConsistency(t,env,wsID) — Repositories.Workspace.Get + List, DirExists(WorktreePath), BranchName match
  - oracle.go:93 func AssertGitStateMatchesReadModel(t,env,wsID) — engine.Git.Status vs ws.HasConflicts
- `api/tests/kit/bench.go`
  - bench.go:17 type BenchmarkResult struct { Name string; P50,P99 time.Duration }
  - bench.go:34 func RunBenchmark(t,name,n,fn) BenchmarkResult
  - bench.go:79 func AssertNoRegression(t,result) — threshold 1.25x p99 vs baseline.json; UPDATE_BASELINE=1 to refresh
  - baseline path: api/tests/integration/bench/baseline.json
- `api/tests/harness_test.go`
  - harness_test.go:1 //go:build integration; package tests
  - harness_test.go:33 type harness struct { t *testing.T; server *httptest.Server; url string }
  - harness_test.go:41 func newHarness(t) *harness — engine.New/adapter.New/app.New + crowbarapi.New(appContainer,engines,fstest.MapFS{}) + httptest.NewServer(apiContainer.Handler())
  - harness_test.go:73 (h) get(path,out) — asserts {success,error,data} envelope, decodes data
  - harness_test.go:82 post(path,body,wantStatus,out) / put / patch / del(path,body,wantStatus,out)
  - harness_test.go:128 postError(path,body,wantStatus) — asserts success=false + non-empty error
  - harness_test.go:194 (h) dial(path) *websocket.Conn — dials ws+/v0 path
  - harness_test.go:211 func readUntil(t,conn,match) map[string]any — 10s deadline, skips control frames, no sleep
  - harness_test.go:239 func gitRepoWithCommit(t) string
- `api/tests/regressions_test.go`
  - TestRegression_FilesTreeServedAtTreePath (line 31)
  - TestRegression_AllReadEndpointsUseEnvelope (line 55) — paths incl /v0/projects,/v0/repos,/v0/workspaces,base+/git/status,base+/chats,/v0/runs/running
  - TestRegression_StageUnstageDiscardAcceptPathsArray (line 83)
  - TestRegression_CommitAcceptsSubjectAndBody (line 105)
  - TestRegression_GitStatusFilesNeverNull (line 132) — RAW body assert files:[] not null
  - TestRegression_GitTopicQuietWhenIdle (line 152) — AssertNoMessage idiom inline
  - TestRegression_ImportNonexistentPathLeavesNoProject (line 384)
  - TestRegression_LinkedWorktreeImportsAsOneRepo (line 404)
  - TestRegression_BogusWorkspaceReadsAre404 (line 447)
  - TestRegression_DeleteLockedWorkspaceRejected (line 475) — asserts ws.Locked bool
  - TestRegression_EmptyPathParamsRejected (line 511) — rejectEmptyPathParams middleware on /v0 group
- `api/tests/fixtures_test.go`
  - fixtures_test.go:27 func importProject(t,h) importedRepo — POST /v0/projects {name,path} 201, listRepos, firstWorkspaceForRepo
  - fixtures_test.go:58 func importWritableWorkspace(t,h) importedRepo — child ws on feature/write
  - fixtures_test.go:80 type repoDTO struct { ID,ProjectID,Path string `json` }
  - fixtures_test.go:86 func listRepos(t,h,projectID) []repoDTO — GET /v0/repos?projectId=
  - fixtures_test.go:97 type workspaceDTO struct { ID,RepoID,ProjectID,Branch,Status string; Locked bool; Added,Deleted int }
  - fixtures_test.go:108 func firstWorkspaceForRepo(t,h,repoID) string — GET /v0/workspaces?repoId=
- `api/tests/integration/lifecycle/lifecycle_test.go`
  - TestLifecycle_WorkspaceCreateBroadcastsOverWS (line 36) — POST /v0/repos, DialWorkspaces(?projectId=p1), POST /v0/workspaces -> 201 id, WaitForWorkspace status==new
  - TestLifecycle_WorkingTreeSyncUpdatesReadModelAndBroadcasts (line 70) — stage/commit then POST /sync, WaitForWorkspace on status-absent (omitempty)
  - TestLifecycle_ChatAgentRunDrivesChatStatusOverWS (line 144) — agent-run flow (removed by spec)
  - TestLifecycle_WorkspaceList (line 205) / TestLifecycle_GitStageCommitChangesStatus (line 245)
- `api/tests/integration/websocket/websocket_test.go`
  - TestWS_WorkspacesFilter_ProjectID (line 39) / TestWS_WorkspacesFilter_RepoID (line 90) — two repos different projectIds, DialWorkspaces(?projectId=/?repoId=)
  - TestWS_ChatsFilter_WsID (line 121) — chats topic (removed)
  - TestWS_LSP_FilteredByWsID (line 181) — PushLSP injection + DialLSP(?wsId=)
  - TestWS_MultiClientFanOut (line 211) / TestWS_GitTopic_StatusBroadcast (line 253) — PushGit injection / TestWS_FilesTopicIsChangeOnly (line 329) — PushFile injection
- `api/tests/integration/crash/crash_test.go`
  - crash_test.go:28 func (s *CrashSuite) SetupTest() {} — no-op override; tests build Env locally
  - Uses BuildEnvAt(t, homeDir) twice on the SAME homeDir with env1.Close(t) between to simulate restart
  - Asserts read-model survives reopen of the same SQLite files
- `api/tests/integration/terminal/terminal_test.go`
  - TestTerminal_CreateAndKill (line 67) — POST /v0/workspaces/:wsId/terminals -> 201 {sessionId}, DELETE /v0/terminals/:sessionId
  - TestTerminal_ProfileCRUD (line 138) — /v0/settings/terminal/profiles CRUD
  - TestTerminal_WSConnectionEstablishes (line 247) — dial /v0/ws/terminals/:sessionId, ReadMsg frame {sessionId,data,isInput}
- `api/internal/api/v0/ws/broadcaster.go`
  - broadcaster.go:16 type Broadcaster[T any] struct { def StreamDef[T]; clients map; registered chan; once sync.Once; regCount chan struct{} (buffered 1024) }
  - broadcaster.go:28 func NewBroadcaster[T](def StreamDef[T]) *Broadcaster[T]
  - broadcaster.go:40 func (b) WaitRegistered() — <-b.registered (sync.Once, first client)
  - broadcaster.go:47 func (b) WaitNRegistered(n int) — drains n regCount tokens (multi-Dial safe)
  - broadcaster.go:59 func (b) Handle(c *gin.Context) — upgrade, register, snapshotFor, onSubscribe, writePump/readPump, remove, onUnsubscribe
  - broadcaster.go:165 func (b) Push(event T)
- `api/internal/api/v0/container_test_hooks.go`
  - container_test_hooks.go:13 func (c *Container) WaitNWorkspacesRegistered(n) { c.workspaces.WaitNRegistered(n) }
  - lines 23/33/43/53 WaitNChats/Files/Git/LSPRegistered
  - container_test_hooks.go:61 func (c *Container) PushLSP(evt lspdomain.DiagnosticsEvent)
  - (container.go:126/139 PushGit / PushFile are non-test-tagged)
- `api/internal/app/repositories/workspace/workspace.go`
  - workspace.go:143 ax.SendWait(ctx, commands.CreateWorkspace{...}) returns evt.Aggregate (synchronous, projection applied)
  - workspace.go:184 ax.Get(ctx,id) — read projected aggregate
  - workspace.go:311 ax.Forget(ctx,id) — delete
  - workspace.go:318 List(ctx) reads store projection
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go`
  - worktreepath.go:20 func For(crowbarHome, remoteURL, workspaceID string) (string, error)
  - worktreepath.go:33 func RepoDir(crowbarHome, remoteURL string) (string, error)
  - worktreepath.go:43 func DefaultCrowbarHome() (string, error)
  - worktreepath.go:53 func repoRelPath(rawURL string) (string, error)
  - worktreepath_test.go: TestFor_*/TestRepoDir_* table-style unit tests, no build tag, plain testify
- `web/vite.config.ts`
  - test.environment: 'jsdom'; globals: true; setupFiles: ['./src/__tests__/setup.ts']
  - alias '@tauri-apps/api/core' -> __tests__/__mocks__/tauri-api-core.ts
  - coverage thresholds { lines:28, functions:50, branches:70, statements:28 } (ratchet floor)
- `web/src/__tests__/setup.ts`
  - import 'fake-indexeddb/auto' (line 2) — real IndexedDB API in-memory
  - makeLocalStorage() polyfill (line 6)
- `web/src/__tests__/lib/ws/connection-store.test.ts`
  - class MockWebSocket with static CONNECTING/OPEN/CLOSING/CLOSED, instances[], simulateOpen/simulateClose
  - vi.stubGlobal('WebSocket', MockWebSocket)
  - await import('@/lib/ws/manager') / connection-store AFTER stubbing
  - vi.useFakeTimers()/advanceTimersByTime for reconnect — no real waits
- `web/src/__tests__/lib/persistence/cache-store.test.ts`
  - beforeEach(() => { resetDB(); vi.restoreAllMocks() })
  - saveCache(store,key,data,fetchedAt) / loadCache<T>(store,key)
  - vi.spyOn(idb,'getDB').mockRejectedValueOnce(...) — degrade-to-miss assertions

### Must change
- `api/tests/kit/env.go` — Spec §3/§7: migrate all helper URLs from flat to hierarchical. RegisterRepo -> POST /v0/projects/:p/repos. CreateWorkspace/CreateChildWorkspace -> POST /v0/projects/:p/repos/:r/workspaces, and (spec §4) expect 202 + block on a WS WorkspaceDTO instead of decoding a 201 body id. Add DialProjects/DialRepos/DialThreads/DialTerminals scoped to hierarchical paths with matching WaitN<X>Registered. Remove DialChats + WaitForChat (spec §12). Add a WaitForWorkspaceState(t,w,wsID,status,timeout) convenience over ReadUntil. Add a mock-provider injection helper for spec §11 PR-status transitions.
- `api/tests/kit/oracle.go` — Spec §5: AssertGitStateMatchesReadModel reads ws.HasConflicts which is removed; assert ws.Status == domain.WorkspaceStatusPRConflicts instead. AssertWorkspaceConsistency.WorktreePath must equal worktreepath.For(home,projectID,repoID,wsID) (new UUID layout, spec §1/§8) — add a path-shape assertion.
- `api/tests/fixtures_test.go` — Spec §5/§7: workspaceDTO drop Locked bool, rely on Status; add ParentID, LastError, CanMergeLocally, ParentBranch, PRUrl, PRTitle, PRTargetBranch. listRepos -> GET /v0/projects/:p/repos. firstWorkspaceForRepo -> GET /v0/projects/:p/repos/:r/workspaces. importProject: workspace creation is now 202+WS, so wait on the adopted-workspace WS DTO rather than reading a sync body.
- `api/tests/regressions_test.go` — Spec §3/§5/§12: migrate path lists in TestRegression_AllReadEndpointsUseEnvelope and TestRegression_BogusWorkspaceReadsAre404 to hierarchical routes; drop /v0/chats and /v0/runs/running entries. TestRegression_DeleteLockedWorkspaceRejected: replace ws.Locked assertion with status=='locked'. Add the new §13-mandated TestRegression_* cases here (see testTargets).
- `api/tests/integration/lifecycle/lifecycle_test.go` — Spec §4/§5: TestLifecycle_WorkspaceCreateBroadcastsOverWS must expect 202 (not 201) and dial the hierarchical workspaces WS. Delete TestLifecycle_ChatAgentRunDrivesChatStatusOverWS (spec §12). Update DialWorkspaces calls to hierarchical namespace routes.
- `api/tests/integration/websocket/websocket_test.go` — Spec §5: replace ?projectId=/?repoId=/?wsId= query-param filter tests with hierarchical-route prefix-matching tests (client at .../workspaces sees p/r/ prefix; at .../workspaces/:w sees exact p/r/w). Remove TestWS_ChatsFilter_WsID. Keep PushGit/PushFile/PushLSP injection idioms.
- `api/tests/integration/crash/crash_test.go` — Spec §1/§9/§12: replace agent-run RecoverOrphans content with per-entity DB persistence-across-restart tests (workspace/repo/project storages survive BuildEnvAt reopen; AdapterContainer LRU lazily re-opens). Remove chat/run-specific assertions.
- `api/tests/integration/terminal/terminal_test.go` — Spec §3/§4: DELETE -> /v0/projects/:p/repos/:r/workspaces/:w/terminals/:sessionId; PTY WS -> .../terminals/:sessionId/ws; POST terminals expects 202 + WS TerminalSessionDTO (currently 201 sync body). Add a Broadcaster[TerminalSessionDTO] lifecycle assertion.
- `api/tests/integration/provider/provider_test.go` — Spec §11: routes move to /v0/projects/:p/repos/:r/workspaces/:w/provider and .../protected-branches (the test currently hits /v0/repos/:wsId/protected-branches — a pre-existing mis-scoped path). Add a mock-provider PR-status transition test (pr-open -> pr-merged) asserting the WorkspaceDTO arrives via WS.
- `api/internal/api/v0/container_test_hooks.go` — Spec §5: add WaitNProjectsRegistered / WaitNReposRegistered / WaitNThreadsRegistered / WaitNTerminalsRegistered hooks for the new scoped broadcasters; remove WaitN(Chats)Registered. Add a Push hook for any new directly-injected DTO channel (e.g. ThreadDTO) if a test injection path is required.
- `api/internal/app/usecases/internal/worktreepath/worktreepath_test.go` — Spec §8: rewrite entirely to the new signatures For(home,projectID,repoID,wsID) string / StorageDir / RepoDir / ProjectDir (no error returns). New expected outputs are UUID-segment paths ending in /worktree and /storages. Add a *_bench_test.go for path construction (spec §13).

### New contracts
- // api/tests/kit/env.go — hierarchical helpers
- func (e *Env) RegisterProject(
	t *testing.T,
	name string,
	path string,
) string // POST /v0/projects {name,path} -> 202; returns projectID after WS ProjectDTO
- func (e *Env) RegisterRepo(
	t *testing.T,
	projectID string,
	name string,
	path string,
) string // POST /v0/projects/:projectId/repos -> 202; returns repoID after WS RepoDTO
- func (e *Env) CreateWorkspace(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
) string // POST /v0/projects/:p/repos/:r/workspaces -> 202; returns wsID after WS WorkspaceDTO{status:"new"}
- func (e *Env) CreateChildWorkspace(
	t *testing.T,
	projectID string,
	repoID string,
	branch string,
	parentID string,
) string
- func (e *Env) DialProjects(
	t *testing.T,
) *WSWatcher // WS /v0/projects
- func (e *Env) DialRepos(
	t *testing.T,
	projectID string,
) *WSWatcher // WS /v0/projects/:p/repos
- func (e *Env) DialWorkspaces(
	t *testing.T,
	projectID string,
	repoID string,
) *WSWatcher // WS /v0/projects/:p/repos/:r/workspaces (repo-scoped prefix)
- func (e *Env) DialWorkspace(
	t *testing.T,
	projectID string,
	repoID string,
	wsID string,
) *WSWatcher // WS /v0/projects/:p/repos/:r/workspaces/:wsId (exact)
- func (e *Env) DialThreads(
	t *testing.T,
	projectID string,
	repoID string,
	wsID string,
) *WSWatcher
- func (e *Env) DialTerminals(
	t *testing.T,
	projectID string,
	repoID string,
	wsID string,
) *WSWatcher
- func WaitForWorkspaceState(
	t *testing.T,
	w *WSWatcher,
	wsID string,
	status string,
	timeout time.Duration,
) map[string]any // ReadUntil msg["id"]==wsID && msg["status"]==status
- func WaitForWorkspaceLastError(
	t *testing.T,
	w *WSWatcher,
	wsID string,
	timeout time.Duration,
) string // ReadUntil msg["id"]==wsID && msg["lastError"]!=""; returns lastError
- // api/internal/api/v0/container_test_hooks.go (//go:build integration)
- func (c *Container) WaitNProjectsRegistered(
	n int,
) // c.projects.WaitNRegistered(n)
- func (c *Container) WaitNReposRegistered(
	n int,
)
- func (c *Container) WaitNThreadsRegistered(
	n int,
)
- func (c *Container) WaitNTerminalsRegistered(
	n int,
)
- // api/internal/app/usecases/internal/worktreepath/worktreepath.go (spec §8 — rewritten, no error)
- func For(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
) string
- func StorageDir(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
) string
- func RepoDir(
	crowbarHome string,
	projectID string,
	repoID string,
) string
- func ProjectDir(
	crowbarHome string,
	projectID string,
) string
- // api/tests workspaceDTO test struct (spec §5)
- type workspaceDTO struct {
	ID              string `json:"id"`
	RepoID          string `json:"repoId"`
	ProjectID       string `json:"projectId"`
	Branch          string `json:"branch"`
	ParentID        string `json:"parentId"`
	Status          string `json:"status"`
	Working         bool   `json:"working"`
	LastError       string `json:"lastError"`
	Added           int    `json:"added"`
	Deleted         int    `json:"deleted"`
	MergeStrategy   string `json:"mergeStrategy"`
	CanMergeLocally bool   `json:"canMergeLocally"`
	ParentBranch    string `json:"parentBranch"`
	PRUrl           string `json:"prUrl"`
	PRTitle         string `json:"prTitle"`
	PRTargetBranch  string `json:"prTargetBranch"`
}
- // Concrete blackbox route assertions added by §13:
- POST /v0/projects/:projectId/repos/:repoId/workspaces            -> 202 then WS WorkspaceDTO{status:"new"} then WS WorkspaceDTO ready
- DELETE /v0/projects/:projectId/repos/:repoId/workspaces/:wsId    -> 202 then WS WorkspaceDTO{status:"deleted"}
- POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/push -> 202 then WS WorkspaceDTO (status update OR lastError set)
- GET  /v0/projects/:projectId/repos/:repoId/icon                  -> 200 image bytes (from disk)

### Risks
- TWO independent blackbox harnesses exist and BOTH must be migrated in lockstep: kit.Env (api/tests/kit, wires v0.New directly, used by api/tests/integration/*) and the package-tests harness (api/tests/harness_test.go, wires top-level crowbarapi.New with web embed, used by all api/tests/*_test.go regression files). They register routes differently (router.Group("/v0") vs apiContainer.Handler()). A route change must be reflected in both; missing one leaves half the suite red or, worse, green against a stale route.
- Workspace/repo/project/terminal creation flips from synchronous 201+body-id to 202+empty-body+WS-DTO (spec §4). EVERY helper that does kit.MutationID(resp) or reads {data:{id}} (env.go CreateWorkspace/CreateChildWorkspace/RegisterRepo, fixtures_test.go importProject, websocket_test.go, lifecycle_test.go, crash_test.go, terminal_test.go) breaks: there is no id in a 202 body. The id must now be learned from the WS DTO. This is a pervasive ordering hazard — the WS watcher must be dialled BEFORE the POST or the create event is missed (WaitNRegistered gates this today for the workspaces topic only).
- Namespace filtering changes from query-param (?projectId=/?repoId=/?wsId=) to route-prefix derivation (spec §5). The Broadcaster currently filters via FilterDef query params (filter.go/BuildPredicate). If prefix matching is implemented but tests still pass query params (or vice versa) the filter silently matches everything or nothing — a subscriber at .../workspaces accidentally receiving cross-repo events would pass a naive 'received the event' assertion. Tests must explicitly assert NEGATIVE filtering (AssertNoMessage for out-of-prefix events), as TestWS_WorkspacesFilter does today.
- Removed DTO fields (Locked bool, HasConflicts bool, HasPR bool, PendingOp) are asserted directly in fixtures_test.go workspaceDTO, regressions_test.go (TestRegression_DeleteLockedWorkspaceRejected reads ws.Locked), and oracle.go (AssertGitStateMatchesReadModel reads ws.HasConflicts). These compile/fail silently if the JSON tag is dropped (zero-value bool) — the assertions would pass vacuously (Locked=false) and mask a real status regression. Each must be rewritten to status-string assertions, not just deleted.
- Chat/agent-run elimination (spec §12, Open-Q3) deletes kit.Env.DialChats/WaitForChat/WaitNChatsRegistered, TestWS_ChatsFilter_WsID, TestLifecycle_ChatAgentRun*, the entire crash_test.go RecoverOrphans content, and the /v0/chats + /v0/runs/running entries in TestRegression_AllReadEndpointsUseEnvelope. agentrun_test.go (api/tests/integration/agentrun) becomes dead. Leaving any reference compiles against removed container fields (c.chats) and fails the whole package build.
- Lazy per-entity DB open + LRU cap of 64 (spec §9) introduces eviction-closes-handle semantics. A blackbox test that holds >64 open workspaces and then reads an evicted-then-reopened workspace could race the close/reopen. The crash_test.go BuildEnvAt(shared homeDir)+Close pattern assumes a single global DB handle released by adapters.Close(); with per-entity handles in an LRU, Close must drain/close ALL cached handles or the second Env hits SQLITE_BUSY (the exact failure crash_test.go's comment already documents for WAL DDL).
- Broadcaster snapshot vs live-event race is already documented (broadcaster.go:108 register-before-snapshot, double-delivery harmless). With the new 202 flow, the create goroutine pushes the ready DTO asynchronously; if snapshotFor for a late-connecting client reads the projection between the 'new' and 'ready' transitions, the client may see only one of the two frames. Tests must assert on the TERMINAL state (ReadUntil status==X), never assume two distinct frames arrive, matching the omitempty pattern in TestLifecycle_WorkingTreeSync.
- The spec §13 demands 100% statement coverage / ≥95% CI gate for Go unit tests, but there is NO existing automated statement-coverage gate visible in the test kit (only the FE vitest ratchet and the bench p99 baseline). The Makefile coverpkg gate referenced in MEMORY (Wave 0) is the actual gate; the spec assumes it covers the new packages — verify the coverpkg list includes new adapter/registry, worktreepath, broadcaster, and merge-eligibility packages or coverage silently won't gate them.
- MergeEligibilityFor (spec §10) is computed per-list-call from siblings, never persisted. Blackbox tests asserting CanMergeLocally/ParentBranch depend on the FULL sibling set being in the list response. If list pagination or repo-scoping changes, the sibling scan input changes and the field flips — tests must construct an explicit parent+child pair in one repo and assert both true and false (locked/deleted parent) branches.
- Spec §11 per-connection 1-min provider poll is a goroutine started on WS upgrade and cancelled on close. A blackbox test cannot wait 1 minute (and must not time.Sleep). It needs an injectable/mock provider clock or an event-driven trigger; provider_test.go currently only tests the disabled-local-repo path. Without a mock-provider injection seam (analogous to PushGit/PushLSP), the pr-open->pr-merged transition test (§13) is untestable deterministically — flag as a missing test seam to add.
- worktreepath.For losing its error return (spec §8) changes call sites that currently do `path, err := For(...)`. Any test or production caller ignoring this compiles to a tuple-assignment error. The co-located worktreepath_test.go uses `path, err := For(...)` throughout and must be fully rewritten, not patched.

### Test targets
- UNIT api/internal/app/usecases/internal/worktreepath/worktreepath_test.go (rewrite): TestFor_UUIDLayout (.../projects/<P>/<R>/workspaces/<W>/worktree), TestStorageDir_EndsInStorages, TestRepoDir, TestProjectDir, TestFor_Deterministic, TestFor_DistinctWorkspacesDiverge — plain testify, no build tag, 100% coverage of the 4 funcs.
- BENCH api/internal/app/usecases/internal/worktreepath/worktreepath_bench_test.go: BenchmarkFor / BenchmarkStorageDir (spec §13 path-construction perf path).
- UNIT api/internal/adapter/<registry>_test.go: TestWorkspaceES_LazyOpenCreatesDir, TestWorkspaceES_ReturnsCachedHandle, TestWorkspaceES_LRUEvictsAndClosesOldest (maxOpenWorkspaceDBs=64), TestWorkspaceES_ConcurrentSamePath (per-path mutex) — synchronise via sync.WaitGroup, no sleep.
- BENCH api/internal/adapter/<registry>_bench_test.go: BenchmarkWorkspaceES_CachedLookup (DB registry lookup, spec §13).
- UNIT api/internal/app/usecases/workspace/<merge>_test.go: TestMergeEligibilityFor_NoParent_False, TestMergeEligibilityFor_ParentLocked_False, TestMergeEligibilityFor_ParentDeleted_False, TestMergeEligibilityFor_ParentIdle_TrueReturnsBranch, TestMergeEligibilityFor_ParentMissingFromSiblings_False.
- BENCH api/internal/app/usecases/workspace/<merge>_bench_test.go: BenchmarkMergeEligibilityFor_LargeSiblingSet (eligibility scan, spec §13).
- UNIT api/internal/api/v0/ws/broadcaster_test.go (extend): TestBroadcaster_NamespacePrefixMatch (client at p/r/ receives p/r/w; client at p/r/w rejects p/r2/w), reusing the existing WaitRegistered + itemDef pattern.
- BLACKBOX api/tests/regressions_test.go (add, package-tests harness): TestRegression_WorkspaceCreateReturns202ThenWS (POST .../workspaces -> 202 empty body, WS WorkspaceDTO{status:new} then ready), TestRegression_WorkspaceDeleteBroadcastsDeletedStatus (DELETE -> 202, WS WorkspaceDTO{status:deleted}), TestRegression_GitPushFailureSetsLastError (202 -> WS WorkspaceDTO.lastError non-empty), TestRegression_WorkspaceCreateBranchMissingCreatesFromParent vs TestRegression_WorkspaceCreateBranchExistsChecksOut, TestRegression_IconServedFromDiskNotGitHub (GET .../icon), TestRegression_DeleteWorkspaceRemovesStoragesDir (DirExists false after delete).
- BLACKBOX api/tests/integration/websocket/websocket_test.go (rewrite): TestWS_HierarchicalPrefix_RepoScopeReceivesAllWorkspaces, TestWS_HierarchicalPrefix_WsScopeRejectsSibling (AssertNoMessage), TestWS_ProjectScopeReceivesRepoEvents — using DialWorkspaces/DialWorkspace + WaitNRegistered, no query params.
- BLACKBOX api/tests/integration/lifecycle/lifecycle_test.go (update): TestLifecycle_WorkspaceCreate202ThenWS, TestLifecycle_MergeEligibilityTrueWhenParentIdle / FalseWhenParentLocked (CanMergeLocally + ParentBranch in the WS DTO), TestLifecycle_SyncClearsLastError.
- BLACKBOX api/tests/integration/provider/provider_test.go (add): TestProvider_PRStatusTransitionBroadcastsWorkspaceDTO (mock provider pr-open -> pr-merged, assert WS WorkspaceDTO{status:pr-merged}) — requires a new mock-provider injection seam (flagged in risks); routes rescoped to .../workspaces/:w/provider.
- BLACKBOX api/tests/integration/terminal/terminal_test.go (update): TestTerminal_Create202ThenSessionDTOOverWS, TestTerminal_DeleteScopedRoute, TestTerminal_PTYWSAtScopedPath (.../terminals/:sessionId/ws).
- BLACKBOX api/tests/integration/threads (new package threads_test.go): TestThreads_OpenBroadcastsThreadDTO, TestThreads_ReplyAppendsAndBroadcasts, TestThreads_ResolveTogglesAndBroadcasts — Broadcaster[ThreadDTO], embedding kit.IntegrationSuite + DialThreads.
- BLACKBOX api/tests/integration/crash/crash_test.go (rewrite): TestRestart_WorkspaceStoragesPersistAcrossReopen, TestRestart_AdapterLRUReopensEvictedDB — BuildEnvAt(shared homeDir)+Close, assert DirExists(storages/event_stream.db) and read-model survives.
- FE web/src/__tests__/lib/persistence/entity-cache.test.ts (new): cache merge upsert by id (cache.set), write-through to IndexedDB object store, status:'deleted' removal-after-transition, wipe-and-reseed on daemon version change — fake-indexeddb + resetDB idiom.
- FE web/src/__tests__/lib/ws/entity-stream.test.ts (new): MockWebSocket DTO frame -> cache merge -> re-render, dial-before-fetch startup sequence (spec §6/§7), hierarchical subscription URL construction (/v0/projects/:p/repos/:r/workspaces) — vi.stubGlobal('WebSocket') + dynamic import idiom.
- FE web/src/__tests__/lib/api/workspace.test.ts (update): hierarchical URL assertions for create (POST .../workspaces returns 202, no body id consumed) and the projectId+repoId-from-route plumbing (spec §7).

---
