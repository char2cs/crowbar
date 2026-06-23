package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/locations"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// AsynxFactory builds a workspace Asynx instance over a per-entity event store.
// It is injected by the app layer to avoid an import cycle on newAsynx.
type AsynxFactory func(
	es asynxModels.Store,
) (asynx.Asynx[domain.Workspace], error)

// CreateInput carries the fields needed to create a workspace.
type CreateInput struct {
	ID            string
	RepoID        string
	ProjectID     string
	Branch        string
	WorktreePath  string
	ForkPointSha  string
	ParentID      string
	Protected     bool
	MergeStrategy gitdomain.MergeStrategy
	IsDefault     bool
}

// SyncInput carries a recomputed working-tree summary.
type SyncInput struct {
	ID           string
	Added        int
	Deleted      int
	HasConflicts bool
	HasCommits   bool
}

// ProviderInput carries a provider poll result (08 §5).
type ProviderInput struct {
	ID             string
	Protected      bool
	HasPR          bool
	PRStatus       string
	PRUrl          string
	PRTitle        string
	PRTargetBranch string
}

// Workspace is the workspace aggregate repository.
type Workspace interface {
	Create(
		ctx context.Context,
		in CreateInput,
		now time.Time,
	) (domain.Workspace, error)
	SyncWorkingTreeState(
		ctx context.Context,
		in SyncInput,
		now time.Time,
	) (domain.Workspace, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	SyncProviderState(
		ctx context.Context,
		in ProviderInput,
		now time.Time,
	) (domain.Workspace, error)
	SetMergeStrategy(
		ctx context.Context,
		id string,
		strategy gitdomain.MergeStrategy,
	) (domain.Workspace, error)
	TouchActivity(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
	Reparent(
		ctx context.Context,
		id string,
		parentID string,
		forkPointSha string,
		now time.Time,
	) (domain.Workspace, error)
	ResolveConflicts(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
	UpdateForkPoint(
		ctx context.Context,
		id string,
		forkPointSha string,
	) (domain.Workspace, error)
	// SetParentFromPR sets ParentID from an open PR's target branch without
	// recomputing ForkPointSha.
	SetParentFromPR(
		ctx context.Context,
		id string,
		parentID string,
	) (domain.Workspace, error)
	// SetLastError records the message from a failed background operation on the
	// workspace; the failure surfaces on the entity, never a separate WS frame
	// (00 §4). The next successful mutating command clears it.
	SetLastError(
		ctx context.Context,
		id string,
		message string,
	) (domain.Workspace, error)
	Delete(
		ctx context.Context,
		id string,
	) error
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
}

// wsEntity is the per-workspace resolved Asynx instance plus its read-model
// store, cached in the entity registry.
type wsEntity struct {
	ax    asynx.Asynx[domain.Workspace]
	store store.Store
	// esRelease and viewRelease release the adapter's per-entity event-store and
	// view DB handles. The asynx (ax) is built over the ES handle, so they MUST be
	// released only after ax.Shutdown — see the registry closeFn in New. The entity
	// owns them so the wsEntity and its underlying ES/View handles are evicted
	// together.
	esRelease   func()
	viewRelease func()
	// writeMu serializes commands to this aggregate: single-writer-per-aggregate.
	writeMu sync.Mutex
}

// send dispatches a command to this entity's aggregate, serialized so only one
// command per workspace is in flight at a time.
//
// The optimistic event store reads nextVersion then Appends; asynx's default
// shard runs 8 workers, and that worker count is not settable through its public
// builder. So concurrent same-aggregate commands (the fs-watcher sync, the
// provider sweep, chat activity, and a user git op all firing on one workspace)
// each read the same nextVersion and Append at it — the losers fail with a
// version/PK conflict and the read model silently goes stale. Serializing here
// is the single-writer-per-aggregate model the event store assumes, without
// forking asynx. Cross-aggregate writes still run fully in parallel (one mutex
// per entity).
func (e *wsEntity) send(
	ctx context.Context,
	cmd asynxModels.Command[domain.Workspace],
) (asynxModels.Event[domain.Workspace], error) {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	return e.ax.SendWait(ctx, cmd)
}

// forget tombstones and erases the aggregate, serialized under the same
// per-aggregate write mutex as send so it never races a concurrent send (the
// provider sweep / fs-watcher firing SyncProviderState/SyncWorkingTreeState on
// the same workspace) — that race version-conflicts, the exact failure writeMu
// exists to eliminate.
func (e *wsEntity) forget(
	ctx context.Context,
	id string,
) error {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	return e.ax.Forget(ctx, id)
}

type workspace struct {
	adapters     *adapter.Container
	broadcast    store.BroadcastFunc
	asynxFactory AsynxFactory
	locations    locations.Store
	entities     *adapter.Registry[*wsEntity]
}

// Option configures the Workspace repository.
type Option func(*repoOpts)

type repoOpts struct {
	maxOpenEntities int
}

// WithMaxOpenEntities bounds the per-entity LRU cache to n open entities. A
// non-positive n falls back to the registry default. Used by tests to exercise
// eviction without opening 64+ entities.
func WithMaxOpenEntities(
	n int,
) Option {
	return func(o *repoOpts) {
		o.maxOpenEntities = n
	}
}

// New builds a per-entity Workspace repository. Each workspace's Asynx instance
// and read-model view are resolved lazily from the adapter container by ID via
// the location index (held in the global view DB), and cached in a ref-counted
// LRU registry. The broadcast func is the hub fan-out for projected rows.
// asynxFactory is injected by the app layer to avoid an import cycle on newAsynx.
func New(
	adapters *adapter.Container,
	broadcast store.BroadcastFunc,
	asynxFactory AsynxFactory,
	opts ...Option,
) (Workspace, error) {
	if adapters == nil {
		return nil, fmt.Errorf("workspace: nil adapters")
	}
	if asynxFactory == nil {
		return nil, fmt.Errorf("workspace: nil asynx factory")
	}
	cfg := repoOpts{}
	for _, o := range opts {
		o(&cfg)
	}
	locationIndex, err := locations.New(adapters.GlobalView())
	if err != nil {
		return nil, fmt.Errorf("workspace: location index: %w", err)
	}
	w := &workspace{
		adapters:     adapters,
		broadcast:    broadcast,
		asynxFactory: asynxFactory,
		locations:    locationIndex,
	}
	// closeFn shuts down asynx FIRST (it references the ES handle), then releases
	// the adapter's ES/View handles, so the underlying SQLite DB is never closed
	// out from under a live asynx — that ordering is the use-after-free guard.
	w.entities = adapter.NewRegistry[*wsEntity](cfg.maxOpenEntities, func(e *wsEntity) error {
		err := e.ax.Shutdown(context.Background())
		if e.esRelease != nil {
			e.esRelease()
		}
		if e.viewRelease != nil {
			e.viewRelease()
		}
		return err
	})
	return w, nil
}

// entityFor resolves the per-workspace Asynx + store for id, building it on a
// cache miss from the location index and the adapter's per-entity DB handles.
// The returned release func decrements the registry refcount; the caller MUST
// call it (typically via defer) when done so the LRU can evict the entity.
func (w *workspace) entityFor(
	ctx context.Context,
	id string,
) (*wsEntity, func(), error) {
	loc, err := w.locations.Get(ctx, id)
	if err != nil {
		// Translate the package-local not-found sentinel to the app-wide
		// apperr.ErrNotFound so HTTP handlers map a bogus workspace id to 404
		// (not 500): the location index is the authority for "does this
		// workspace exist?" across all per-entity reads.
		if errors.Is(err, locations.ErrNotFound) {
			return nil, nil, fmt.Errorf("workspace: locate %q: %w", id, apperr.ErrNotFound)
		}
		return nil, nil, fmt.Errorf("workspace: locate %q: %w", id, err)
	}
	return w.entityForLocation(ctx, loc)
}

func (w *workspace) entityForLocation(
	ctx context.Context,
	loc locations.Location,
) (*wsEntity, func(), error) {
	entity, release, err := w.entities.Acquire(loc.ID, func() (*wsEntity, error) {
		es, esRelease, esErr := w.adapters.WorkspaceES(loc.ProjectID, loc.RepoID, loc.ID)
		if esErr != nil {
			return nil, fmt.Errorf("workspace: event store: %w", esErr)
		}
		view, viewRelease, viewErr := w.adapters.WorkspaceView(loc.ProjectID, loc.RepoID, loc.ID)
		if viewErr != nil {
			// Release the ES handle acquired above before bailing, or it leaks
			// pinned in the registry forever (the wsEntity that would own it is
			// never built).
			esRelease()
			return nil, fmt.Errorf("workspace: view: %w", viewErr)
		}
		ax, axErr := w.asynxFactory(es)
		if axErr != nil {
			esRelease()
			viewRelease()
			return nil, fmt.Errorf("workspace: asynx: %w", axErr)
		}
		// store.New reconciles the read model from the event store on open, so the
		// first List after a crash mid-projection self-corrects (see store.New).
		st, stErr := store.New(ctx, view, ax, w.broadcast, loc.ID)
		if stErr != nil {
			// ax was built but never registered with the entity, so shut it down
			// before releasing the ES handle it sits on — same ordering as the
			// registry closeFn to avoid a use-after-free.
			_ = ax.Shutdown(context.Background())
			esRelease()
			viewRelease()
			return nil, fmt.Errorf("workspace: store: %w", stErr)
		}
		return &wsEntity{ax: ax, store: st, esRelease: esRelease, viewRelease: viewRelease}, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return entity, release, nil
}

func (w *workspace) Create(
	ctx context.Context,
	in CreateInput,
	now time.Time,
) (domain.Workspace, error) {
	if err := w.locations.Save(ctx, locations.Location{
		ID:        in.ID,
		ProjectID: in.ProjectID,
		RepoID:    in.RepoID,
	}); err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: create: %w", err)
	}
	entity, release, err := w.entityForLocation(ctx, locations.Location{
		ID:        in.ID,
		ProjectID: in.ProjectID,
		RepoID:    in.RepoID,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: create: %w", err)
	}
	defer release()
	evt, err := entity.send(ctx, commands.CreateWorkspace{
		ID:            in.ID,
		RepoID:        in.RepoID,
		ProjectID:     in.ProjectID,
		Branch:        in.Branch,
		WorktreePath:  in.WorktreePath,
		ForkPointSha:  in.ForkPointSha,
		ParentID:      in.ParentID,
		Protected:     in.Protected,
		IsDefault:     in.IsDefault,
		MergeStrategy: in.MergeStrategy,
		Now:           now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: create: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) SyncWorkingTreeState(
	ctx context.Context,
	in SyncInput,
	now time.Time,
) (domain.Workspace, error) {
	entity, release, err := w.entityFor(ctx, in.ID)
	if err != nil {
		return domain.Workspace{}, err
	}
	defer release()
	evt, err := entity.send(ctx, commands.SyncWorkingTreeState{
		ID:           in.ID,
		Added:        in.Added,
		Deleted:      in.Deleted,
		HasConflicts: in.HasConflicts,
		HasCommits:   in.HasCommits,
		Now:          now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	entity, release, err := w.entityFor(ctx, id)
	if err != nil {
		return domain.Workspace{}, err
	}
	defer release()
	got, err := entity.ax.Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: get: %w", err)
	}
	return got, nil
}

func (w *workspace) SyncProviderState(
	ctx context.Context,
	in ProviderInput,
	now time.Time,
) (domain.Workspace, error) {
	entity, release, err := w.entityFor(ctx, in.ID)
	if err != nil {
		return domain.Workspace{}, err
	}
	defer release()
	evt, err := entity.send(ctx, commands.SyncProviderState{
		ID:             in.ID,
		Protected:      in.Protected,
		HasPR:          in.HasPR,
		PRStatus:       in.PRStatus,
		PRUrl:          in.PRUrl,
		PRTitle:        in.PRTitle,
		PRTargetBranch: in.PRTargetBranch,
		Now:            now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync provider: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) SetMergeStrategy(
	ctx context.Context,
	id string,
	strategy gitdomain.MergeStrategy,
) (domain.Workspace, error) {
	entity, release, err := w.entityFor(ctx, id)
	if err != nil {
		return domain.Workspace{}, err
	}
	defer release()
	evt, err := entity.send(ctx, commands.SetMergeStrategy{ID: id, Strategy: strategy})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set merge strategy: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) TouchActivity(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	entity, release, err := w.entityFor(ctx, id)
	if err != nil {
		return domain.Workspace{}, err
	}
	defer release()
	evt, err := entity.send(ctx, commands.TouchActivity{ID: id, Now: now})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: touch activity: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) Reparent(
	ctx context.Context,
	id string,
	parentID string,
	forkPointSha string,
	now time.Time,
) (domain.Workspace, error) {
	entity, release, err := w.entityFor(ctx, id)
	if err != nil {
		return domain.Workspace{}, err
	}
	defer release()
	evt, err := entity.send(ctx, commands.Reparent{
		ID:           id,
		ParentID:     parentID,
		ForkPointSha: forkPointSha,
		Now:          now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: reparent: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) ResolveConflicts(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	entity, release, err := w.entityFor(ctx, id)
	if err != nil {
		return domain.Workspace{}, err
	}
	defer release()
	evt, err := entity.send(ctx, commands.ResolveConflicts{
		ID:  id,
		Now: now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: resolve conflicts: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) UpdateForkPoint(
	ctx context.Context,
	id string,
	forkPointSha string,
) (domain.Workspace, error) {
	entity, release, err := w.entityFor(ctx, id)
	if err != nil {
		return domain.Workspace{}, err
	}
	defer release()
	evt, err := entity.send(ctx, commands.UpdateForkPoint{ID: id, ForkPointSha: forkPointSha})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: update fork point: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) SetParentFromPR(
	ctx context.Context,
	id string,
	parentID string,
) (domain.Workspace, error) {
	entity, release, err := w.entityFor(ctx, id)
	if err != nil {
		return domain.Workspace{}, err
	}
	defer release()
	evt, err := entity.send(ctx, commands.SetParentFromPR{ID: id, ParentID: parentID})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set parent from pr: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) SetLastError(
	ctx context.Context,
	id string,
	message string,
) (domain.Workspace, error) {
	entity, release, err := w.entityFor(ctx, id)
	if err != nil {
		return domain.Workspace{}, err
	}
	defer release()
	evt, err := entity.send(ctx, commands.SetLastError{ID: id, Message: message})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set last error: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) Delete(
	ctx context.Context,
	id string,
) error {
	loc, err := w.locations.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("workspace: delete: locate %q: %w", id, err)
	}
	entity, release, err := w.entityForLocation(ctx, loc)
	if err != nil {
		return err
	}
	// Safe to release even after the Evict below: Evict detaches the entry, so the
	// deferred release just decrements a detached refcount (a no-op for eviction).
	defer release()
	// Tear the read model down first (Forget needs the per-entity view.db), then
	// close the handles, drop the location index, and rm -rf the on-disk tree.
	// forget serializes under writeMu so it never version-conflicts with a
	// concurrent sweep/watcher sync on the same aggregate.
	if err := entity.forget(ctx, id); err != nil {
		return fmt.Errorf("workspace: delete: %w", err)
	}
	if err := w.entities.Evict(id); err != nil {
		return fmt.Errorf("workspace: evict entity: %w", err)
	}
	if err := w.locations.Delete(ctx, id); err != nil {
		return fmt.Errorf("workspace: delete location: %w", err)
	}
	w.removeWorkspaceDir(loc.ProjectID, loc.RepoID, id)
	// Broadcast the deleted tombstone LAST — after the read model and the on-disk
	// storage tree are gone — so a client that receives status:"deleted" knows the
	// workspace is fully removed (00 §4/§6: lifecycle lives on the entity).
	w.broadcast(ctx, domain.Workspace{
		ID:        id,
		ProjectID: loc.ProjectID,
		RepoID:    loc.RepoID,
		Status:    domain.WorkspaceStatusDeleted,
	})
	return nil
}

// removeWorkspaceDir rm -rf's the whole per-workspace directory
// (<home>/projects/<P>/<R>/workspaces/<W>: worktree + storages + threads +
// terminals). It is guarded to ONLY remove paths under <home>/projects so an
// adopted/external worktree (the user's real repository, which lives outside the
// crowbar home) is never touched.
func (w *workspace) removeWorkspaceDir(
	projectID string,
	repoID string,
	wsID string,
) {
	home := w.adapters.CrowbarHome()
	if home == "" {
		return
	}
	dir := filepath.Join(home, "projects", projectID, repoID, "workspaces", wsID)
	root := filepath.Join(home, "projects") + string(os.PathSeparator)
	if !strings.HasPrefix(dir, root) {
		return
	}
	_ = os.RemoveAll(dir)
}

// List returns every workspace row across all entities. It enumerates the
// location index and reads each workspace's read-model row.
func (w *workspace) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	locs, err := w.locations.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace: list locations: %w", err)
	}
	rows := make([]domain.Workspace, 0, len(locs))
	for _, loc := range locs {
		// readRow acquires the entity, reads its row, and releases the registry
		// refcount via defer — so a per-iteration acquire/release lets the LRU evict
		// between reads instead of pinning every entity for the whole List.
		ws, err := w.readRow(ctx, loc)
		if err != nil {
			return nil, fmt.Errorf("workspace: list: %w", err)
		}
		if ws == nil {
			continue
		}
		rows = append(rows, *ws)
	}
	return rows, nil
}

// readRow resolves the entity for loc, reads its read-model row, and releases the
// registry refcount before returning (including on error), so List does not pin
// every entity at once.
func (w *workspace) readRow(
	ctx context.Context,
	loc locations.Location,
) (*domain.Workspace, error) {
	entity, release, err := w.entityForLocation(ctx, loc)
	if err != nil {
		return nil, err
	}
	defer release()
	return entity.store.Get(ctx, loc.ID)
}
