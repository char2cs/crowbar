package workspace

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store/projections"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// maxOCCAttempts bounds optimistic-concurrency retries on ErrPipelineFailed
// (decision 10): with writeMu deleted, concurrent Sends to one aggregate id can
// version-collide, so the losers retry — Send re-reads the current version each
// attempt (the shard's pre-assigned version is ignored by the event store), so a
// retry converges. ErrValidation is NEVER retried; ErrQueueFull is surfaced.
const maxOCCAttempts = 5

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
	Kind          domain.WorkspaceKind
	HeldByPath    string
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
	// ProvisionInPlace attaches a worktree to a placeholder row (spec §3.3): it
	// records worktreePath + forkPointSha and clears HeldByPath, keeping Status.
	ProvisionInPlace(
		ctx context.Context,
		id string,
		worktreePath string,
		forkPointSha string,
	) (domain.Workspace, error)
	// ClearBranch blanks an existing aggregate's Branch to "" (spec §4/B6),
	// leaving every other field untouched. Used by the Detach-holder op when the
	// holder is the repo home.
	ClearBranch(
		ctx context.Context,
		id string,
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
	// Delete tombstones the workspace: it fires the pure Delete command (folds
	// Status=deleted) via Send. The physical teardown (cascade Forgets + rm -rf +
	// axWorkspace.Forget) runs off the write path in the async delete reactor
	// (spec §3.6/§3.8, Task 8).
	Delete(
		ctx context.Context,
		id string,
	) error
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	// GetHomeForProject returns the home workspace for the given project.
	// Returns apperr.ErrNotFound if no home workspace exists yet.
	GetHomeForProject(
		ctx context.Context,
		projectID string,
	) (domain.Workspace, error)
	// CreateHome provisions the home workspace for a project. Callers use this
	// for lazy provisioning when GetHomeForProject returns ErrNotFound.
	CreateHome(
		ctx context.Context,
		projectID string,
		worktreePath string,
		now time.Time,
	) (domain.Workspace, error)
}

// workspace is the singleton-backed workspace aggregate repository. One
// axWorkspace routes every workspace id to a shard by hash; there is no per-entity
// Registry, no writeMu (per-aggregate safety is shard routing + (id,version)
// uniqueness + OCC retry), and no location index (the store read model carries
// project_id/repo_id and doubles as the location index — §3.7).
type workspace struct {
	ax         asynx.Asynx[domain.Workspace]
	readModel  store.Store
	pathsStore wspaths.WorkspacePaths
}

// New builds the singleton-backed Workspace repository over axWorkspace, the
// workspace read-model DB (state/store/workspace.db), and the view.db id↔path
// index. It registers the save-only store projection on axWorkspace via
// store.New; the hub projection is registered separately by repositories.Container
// (which owns the enrichment callback).
func New(
	ax asynx.Asynx[domain.Workspace],
	storeDB *gormdb.DB,
	pathsStore wspaths.WorkspacePaths,
) (Workspace, error) {
	if ax == nil {
		return nil, fmt.Errorf("workspace: nil asynx")
	}
	if storeDB == nil {
		return nil, fmt.Errorf("workspace: nil store db")
	}
	if pathsStore == nil {
		return nil, fmt.Errorf("workspace: nil paths store")
	}
	readModel, err := store.New(storeDB, ax)
	if err != nil {
		return nil, fmt.Errorf("workspace: store: %w", err)
	}
	return &workspace{ax: ax, readModel: readModel, pathsStore: pathsStore}, nil
}

// RegisterHubProjection registers the hub (WS fan-out) projection on the singleton
// axWorkspace: for every workspace event it builds the base frame from
// evt.Aggregate, runs enrich to attach the derived overlays the container owns
// (Working + merge eligibility), then broadcasts. It is generic over the frame
// type F so this package stays decoupled from the api-layer wire DTO the container
// supplies. Registered ONCE, by repositories.Container (which owns enrich +
// broadcast, and routes BeginWork/EndWork through the SAME pair); the save-only
// store projection is registered inside New. The projections subpackage lives
// under workspace/internal, so this forwarder is the seam the container reaches it
// through (spec §3.5 hub-frame enrichment, decision 5).
func RegisterHubProjection[F any](
	ax asynx.Asynx[domain.Workspace],
	enrich func(ctx context.Context, ws domain.Workspace) F,
	broadcast func(frame F),
) error {
	return projections.RegisterHub(ax, enrich, broadcast)
}

// sendFunc issues one command attempt against the aggregate.
type sendFunc func(
	ctx context.Context,
	cmd asynxModels.Command[domain.Workspace],
) (asynxModels.Event[domain.Workspace], error)

// occSend runs send with OCC retry and the terminal error disposition contract
// (spec §3.5, decision 10):
//
//   - success                → returned immediately.
//   - ErrValidation          → surfaced immediately, NEVER retried (→ 422).
//   - ErrQueueFull           → translated to apperr.ErrUnavailable (→ 503),
//     NEVER retried: a full shard queue is backpressure, not a version race.
//   - ErrPipelineFailed      → retried up to maxOCCAttempts; still failing after
//     the retries is an unrecoverable optimistic-concurrency collision, surfaced
//     as ErrPipelineFailed (→ 409).
//   - any other error        → surfaced as-is.
//
// All classification is via errors.Is, never string compare.
func occSend(
	ctx context.Context,
	send sendFunc,
	cmd asynxModels.Command[domain.Workspace],
) (asynxModels.Event[domain.Workspace], error) {
	var lastErr error
	for range maxOCCAttempts {
		evt, err := send(ctx, cmd)
		if err == nil {
			return evt, nil
		}
		switch {
		case errors.Is(err, asynxModels.ErrValidation):
			return asynxModels.Event[domain.Workspace]{}, err
		case errors.Is(err, asynxModels.ErrQueueFull):
			return asynxModels.Event[domain.Workspace]{}, fmt.Errorf("workspace: send: %w", apperr.ErrUnavailable)
		case errors.Is(err, asynxModels.ErrPipelineFailed):
			lastErr = err
		default:
			return asynxModels.Event[domain.Workspace]{}, err
		}
	}
	return asynxModels.Event[domain.Workspace]{}, lastErr
}

// sendWithOCC dispatches cmd to the singleton axWorkspace with OCC retry.
func (w *workspace) sendWithOCC(
	ctx context.Context,
	cmd asynxModels.Command[domain.Workspace],
) (asynxModels.Event[domain.Workspace], error) {
	return occSend(ctx, w.ax.Send, cmd)
}

func (w *workspace) Create(
	ctx context.Context,
	in CreateInput,
	now time.Time,
) (domain.Workspace, error) {
	// Record the id→path row before the aggregate exists (§3.9 write-point (a)):
	// it is the rename-resilience map, keyed by the workspace UUID. Roll it back
	// unless the create commits, so a failed create leaves no orphan path row.
	if err := w.pathsStore.Put(ctx, in.ID, in.WorktreePath); err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: create: paths: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = w.pathsStore.Delete(ctx, in.ID)
		}
	}()
	evt, err := w.sendWithOCC(ctx, commands.CreateWorkspace{
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
		Kind:          in.Kind,
		HeldByPath:    in.HeldByPath,
		Now:           now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: create: %w", err)
	}
	committed = true
	return evt.Aggregate, nil
}

func (w *workspace) SyncWorkingTreeState(
	ctx context.Context,
	in SyncInput,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.SyncWorkingTreeState{
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
	// Per-id reads fold the aggregate directly from the event log (§3.7), so Get
	// is always current and needs no read-model rebuild. asynx returns ErrNotFound
	// for an unknown id, which handlers map to 404.
	got, err := w.ax.Get(ctx, id)
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
	evt, err := w.sendWithOCC(ctx, commands.SyncProviderState{
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
	evt, err := w.sendWithOCC(ctx, commands.SetMergeStrategy{ID: id, Strategy: strategy})
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
	evt, err := w.sendWithOCC(ctx, commands.TouchActivity{ID: id, Now: now})
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
	evt, err := w.sendWithOCC(ctx, commands.Reparent{
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
	evt, err := w.sendWithOCC(ctx, commands.ResolveConflicts{
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
	evt, err := w.sendWithOCC(ctx, commands.UpdateForkPoint{ID: id, ForkPointSha: forkPointSha})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: update fork point: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) ProvisionInPlace(
	ctx context.Context,
	id string,
	worktreePath string,
	forkPointSha string,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.ProvisionInPlace{
		ID:           id,
		WorktreePath: worktreePath,
		ForkPointSha: forkPointSha,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: provision in place: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) ClearBranch(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.ClearBranch{ID: id})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: clear branch: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) SetParentFromPR(
	ctx context.Context,
	id string,
	parentID string,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.SetParentFromPR{ID: id, ParentID: parentID})
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
	evt, err := w.sendWithOCC(ctx, commands.SetLastError{ID: id, Message: message})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set last error: %w", err)
	}
	return evt.Aggregate, nil
}

// Delete tombstones the workspace via the pure Delete command (Send + OCC): it
// folds Status=deleted and does NO io. The store projection persists the deleted
// row (so the boot orphan-sweep still finds it) and the async delete reactor
// (topic "workspace.deleted.*", Task 8) performs the physical teardown off the
// write path — closing the old synchronous forget→rm crash gap (spec §3.6/§3.8).
func (w *workspace) Delete(
	ctx context.Context,
	id string,
) error {
	_, err := w.sendWithOCC(ctx, commands.Delete{ID: id})
	if err != nil {
		return fmt.Errorf("workspace: delete: %w", err)
	}
	return nil
}

// List returns every workspace row from the durable read model
// (state/store/workspace.db), which doubles as the location index (§3.7). It
// reads the projection directly and MUST NOT trigger any per-workspace reconcile.
func (w *workspace) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	rows, err := w.readModel.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace: list: %w", err)
	}
	return rows, nil
}

// GetHomeForProject scans all workspaces for the project and returns the one
// whose Kind is WorkspaceKindHome. Returns apperr.ErrNotFound when absent.
func (w *workspace) GetHomeForProject(ctx context.Context, projectID string) (domain.Workspace, error) {
	all, err := w.List(ctx)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("get home for project: list: %w", err)
	}
	for _, ws := range all {
		if ws.ProjectID == projectID && ws.Kind == domain.WorkspaceKindHome {
			return ws, nil
		}
	}
	return domain.Workspace{}, fmt.Errorf("get home for project %q: %w", projectID, apperr.ErrNotFound)
}

// CreateHome provisions the home workspace for a project, used for lazy
// provisioning when GetHomeForProject returns ErrNotFound.
func (w *workspace) CreateHome(ctx context.Context, projectID, worktreePath string, now time.Time) (domain.Workspace, error) {
	ws, err := w.Create(ctx, CreateInput{
		ID:           uuid.NewString(),
		ProjectID:    projectID,
		WorktreePath: worktreePath,
		Kind:         domain.WorkspaceKindHome,
	}, now)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create home workspace: %w", err)
	}
	return ws, nil
}
