package project

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	store "github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/avatar"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Usecase is the read + roll-up surface for the Project entity.
type Usecase interface {
	// List returns every project stored in the GORM table.
	List(
		ctx context.Context,
	) ([]domain.Project, error)

	// Get returns the project with the given id, or an error when not found.
	Get(
		ctx context.Context,
		id string,
	) (domain.Project, error)

	// TouchProjectActivity is a best-effort lastActivity roll-up: it looks up
	// the repo's projectId, updates the project row, and logs any failure
	// without returning it (00 §5.1).
	TouchProjectActivity(
		ctx context.Context,
		repoID string,
		now time.Time,
	)

	// UpdateRepo applies a partial repository update — display name, sidebar
	// order, owning project — in one load-mutate-save, returning the updated
	// Repository so the caller can broadcast its DTO. Returns apperr.ErrNotFound
	// when no repo has the given id, or when a requested project does not exist.
	//
	// A rename touches the LABEL only. Repository.PathSlug — the repo's on-disk
	// identity, seeded once at import — is deliberately left as loaded.
	// UpdateRepo answers the updated repo together with the placement it
	// DECIDED and every other Node the write shifted, so a caller announces
	// the settled state rather than re-reading an async projection.
	UpdateRepo(
		ctx context.Context,
		repoID string,
		in RepoUpdate,
	) (RepoUpdated, error)
	// Reorder sets a project's sidebar index and densifies the whole list, so
	// every project ends up holding a distinct 0..n-1 slot.
	Reorder(
		ctx context.Context,
		projectID string,
		order int,
	) (domain.Project, error)

	// Update applies a partial project update — display name, icon — in one
	// load-mutate-save, returning the updated Project so the caller can broadcast
	// its DTO. Returns apperr.ErrNotFound when no project has the given id.
	//
	// A rename touches the LABEL only. Project.Path — where the project actually
	// lives on disk, chosen once at import — is deliberately left as loaded, for
	// the same reason a repo rename leaves its PathSlug alone: the name is what
	// the sidebar shows, not what anything on disk is keyed on.
	Update(
		ctx context.Context,
		projectID string,
		in Update,
	) (domain.Project, error)
	// NextHomeSlot answers the first free index at projectID's home root —
	// where a freshly imported repo's Node is minted, so the level stays
	// dense from birth instead of every new row tying at 0.
	NextHomeSlot(
		ctx context.Context,
		projectID string,
	) (int, error)
}

// Update is a partial project update: a nil field is left as it is.
//
// The icon fields travel together because they are one three-state choice, not
// three independent ones (emoji > on-disk image > the sidebar's default glyph),
// and setting one without clearing the others is how a project ends up showing
// an emoji it was told to replace with an image.
type Update struct {
	Name          *string
	AvatarEmoji   *string
	AvatarHasIcon *bool
	// BumpAvatarVersion moves the DTO's cache-busting ?v= param. Set it whenever
	// new bytes have been written behind the stable icon URL.
	BumpAvatarVersion bool
}

// RepoUpdate is a partial repository update: a nil field is left as it is.
// ProjectID may only name the repo's own project — a repo's project is fixed
// at import (see refuseProjectMove). FolderID/Order re-file the
// repo's own entry within its project's home tree — written to the repo's
// own Node row (NodePlacements), interleaved against its real home chat and
// folder siblings, which are Node-backed too now (2026-09-08
// sidebar-placement-unification Task 5) — see placeRepoAmongHomeSiblings.
// RepoUpdated is UpdateRepo's answer: the repo, its own decided Node row and
// every OTHER row (any kind) the placement renumbered.
type RepoUpdated struct {
	Repo    domain.Repository
	Node    domain.Node
	Shifted []domain.Node
}

type RepoUpdate struct {
	Name      *string
	ProjectID *string
	Order     *int
	FolderID  *string
}

// HomeWorkspaces answers which workspace IS project home, so a repo's sibling
// search (placeRepoAmongHomeSiblings) can scope a ROOT-level container to THIS
// project specifically — home chats otherwise carry no project id of their own
// to filter by (every project's home resolves to the same "" repo scope).
// Satisfied structurally by the workspace REPOSITORY (not the usecase —
// container.go builds this one before the workspace usecase exists, which
// itself depends on this package).
type HomeWorkspaces interface {
	GetHomeForProject(
		ctx context.Context,
		projectID string,
	) (domain.Workspace, error)
}

// Folders is the plain-GORM home-folder identity surface validateRepoFolder
// needs: confirming a FolderID names a genuine project-home folder (RepoID ==
// "") before writing it — the same golden rule tree/validate.go's
// checkFolderContainer enforces for an ordinary chat folder. Home folders are
// Node/Folder-backed now (2026-09-08 sidebar-placement-unification Task 5),
// so this is the SAME domain.Folder store the tree package's own home-scoped
// CRUD reads/writes — satisfied structurally by the GORM store
// (gormStores.Folders).
type Folders interface {
	FindByKey(ctx context.Context, id string) (*domain.Folder, error)
}

// HomeChats is the narrow chat-membership read placeRepoAmongHomeSiblings
// needs to restore, for CHAT-kind Node rows only, the per-project scoping
// the pre-Task-5 merge (homeContainerChats, deleted) used to guarantee via
// this exact same ListByWorkspace(homeWorkspaceID) call. domain.Node carries
// no project id of its own — unlike a repo (Repository.ProjectID, see
// repoIDSet) or a home folder (which has NO project field at all, a
// pre-existing, disclosed gap this interface does NOT attempt to close, see
// placeRepoAmongHomeSiblings' own doc) — so a CHAT-kind sibling's project
// membership can only be answered by asking the chat aggregate directly.
// Satisfied structurally by the chat repository itself (repos.AgentChat).
type HomeChats interface {
	ListByWorkspace(ctx context.Context, workspaceID string) ([]domain.Chat, error)
}

// NodePlacements is the narrow Node surface a repo's OWN placement needs
// (2026-09-08 sidebar-placement-unification design §2, Task 3): minting the
// position row at repo creation (Create), the repo-kind sibling-space read a
// densify or a placement pass renumbers (ListByParent), one repo's own row for
// UpdateRepo's before/after read and the DTO's folderId/order fields (GetNode),
// the two writes a densify or a move ends in — SetOrder when a row's
// container does not change, SetPlacement when it does (mirrors
// chat/internal/tree/plan.go's writeRow dispatch) — and Forget, which undoes
// a Create a repo import's own rollback takes back out (see importOneRepo).
// Satisfied structurally by the node repository itself
// (repositories.Container.Node).
//
// Every home-scope sibling a repo interleaves with — chat, folder, or
// another repo — now shares this ONE surface (2026-09-08
// sidebar-placement-unification Task 5 made chats/folders Node-backed too),
// which is what let placeRepoAmongHomeSiblings drop its cross-aggregate merge
// with the chat package entirely.
type NodePlacements interface {
	Create(
		ctx context.Context,
		id string,
		kind domain.NodeKind,
		parentID string,
		order int,
	) (domain.Node, error)
	GetNode(
		ctx context.Context,
		id string,
	) (domain.Node, error)
	ListByParent(
		ctx context.Context,
		parentID string,
	) ([]domain.Node, error)
	SetOrder(
		ctx context.Context,
		id string,
		order int,
	) error
	SetPlacement(
		ctx context.Context,
		id string,
		parentID string,
		order int,
	) error
	// Forget purges a Node row outright. Used ONLY to unwind a Create that a
	// repo import's own rollback is taking back out — never to delete a
	// live, in-use row (a repo delete leaving its Node row behind is a
	// separate, harmless gap tracked elsewhere, not this method's job here).
	Forget(
		ctx context.Context,
		id string,
	) error
}

type projectUsecase struct {
	projects   store.Store[domain.Project, string]
	repos      store.ScopedStore[domain.Repository, string]
	workspaces HomeWorkspaces
	folders    Folders
	nodes      NodePlacements
	homeChats  HomeChats
	// broadcastChat announces a collaterally-shifted CHAT/FOLDER-kind Node
	// row on the same chats WS a chat/folder placement already uses — see
	// placeRepoAmongHomeSiblings' own doc on why this exists at all. Nilable,
	// degrading to silence like every other optional dependency here: a repo
	// reorder still WRITES correctly with it nil, it just leaves a live
	// client's chat siblings stale until their next reseed.
	broadcastChat HomeRowAnnouncer
}

// HomeRowAnnouncer announces one shifted home row on the chats WS by its
// own kind: a chat frame for a chat, a folder frame for a folder.
type HomeRowAnnouncer func(id, workspaceID string, kind domain.NodeKind, event string)

// New builds a Usecase from the project and repository GORM stores, the
// project-home lookup a root-level repo placement needs, the home-folder
// identity store validateRepoFolder checks a FolderID against, the Node
// surface that now owns every home-scope sibling's OWN position (see
// NodePlacements), and the chat-membership read that keeps a bare-root
// placement from leaking another project's chats into this one's densify
// (see HomeChats). A nil homeChats degrades the same way a nil workspaces
// already does elsewhere in this file: the bare-root densify simply cannot
// narrow CHAT-kind siblings to this project and falls back to including
// them all, rather than failing the request outright.
//
// broadcastChat is the chats-WS announce callback a repo reorder needs for
// exactly the same reason PlaceChat does (see that handler's own comment):
// every home-scope sibling's write now rides Node, which has no
// aggregate-command hub projection of its own, so nothing tells a live
// client a CHAT/FOLDER row it did not drag also moved as collateral of the
// repo it did drag.
func New(
	projects store.Store[domain.Project, string],
	repos store.ScopedStore[domain.Repository, string],
	workspaces HomeWorkspaces,
	folders Folders,
	nodes NodePlacements,
	homeChats HomeChats,
	broadcastChat HomeRowAnnouncer,
) Usecase {
	return &projectUsecase{
		projects:      projects,
		repos:         repos,
		workspaces:    workspaces,
		folders:       folders,
		nodes:         nodes,
		homeChats:     homeChats,
		broadcastChat: broadcastChat,
	}
}

// List returns every project stored in the GORM table.
func (u *projectUsecase) List(
	ctx context.Context,
) ([]domain.Project, error) {
	list, err := u.projects.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("project: list: %w", err)
	}
	return list, nil
}

// Get returns the project with the given id, or an error when not found.
func (u *projectUsecase) Get(
	ctx context.Context,
	id string,
) (domain.Project, error) {
	p, err := u.projects.FindByKey(ctx, id)
	if err != nil {
		return domain.Project{}, fmt.Errorf("project: get: %w", err)
	}
	if p == nil {
		return domain.Project{}, fmt.Errorf("project: get: id %s: %w", id, apperr.ErrNotFound)
	}
	return *p, nil
}

// TouchProjectActivity is a best-effort lastActivity roll-up: it looks up the
// repo's projectId, updates the project row, and logs any failure without
// returning it (00 §5.1).
func (u *projectUsecase) TouchProjectActivity(
	ctx context.Context,
	repoID string,
	now time.Time,
) {
	repo, err := u.repos.FindByKey(ctx, repoID)
	if err != nil || repo == nil {
		slog.WarnContext(ctx, "project: touch activity: repo not found", "repoID", repoID)
		return
	}
	project, err := u.projects.FindByKey(ctx, repo.ProjectID)
	if err != nil || project == nil {
		slog.WarnContext(ctx, "project: touch activity: project not found", "projectID", repo.ProjectID)
		return
	}
	project.LastActivity = now
	if err := u.projects.Save(ctx, *project); err != nil {
		slog.ErrorContext(ctx, "project: touch activity: save failed", "projectID", project.ID, "err", err)
	}
}

// UpdateRepo applies a partial repository update in one load-mutate-save.
//
// A rename refreshes the generated avatar: the label + color derive from the
// name and surface only as the fallback avatar (when the repo has no custom
// icon/emoji), but must still track the name so a renamed repo's letter badge is
// correct.
//
// The row is loaded and saved whole precisely so the fields an update must NOT
// disturb travel through untouched. Repository.PathSlug is the load-bearing one:
// it is the repo's on-disk identity, seeded once at import, and every managed
// worktree already lives under it. Assigning it here would fork the repo's tree
// in two — new workspaces under the new name, the existing ones stranded under
// the old — and blind the sibling scan that rejects case-only path clashes.
func (u *projectUsecase) UpdateRepo(
	ctx context.Context,
	repoID string,
	in RepoUpdate,
) (RepoUpdated, error) {
	repo, err := u.repos.FindByKey(ctx, repoID)
	if err != nil {
		return RepoUpdated{}, fmt.Errorf("project: update repo: %w", err)
	}
	if repo == nil {
		return RepoUpdated{}, fmt.Errorf("project: update repo: id %s: %w", repoID, apperr.ErrNotFound)
	}
	if err := refuseProjectMove(*repo, in.ProjectID); err != nil {
		return RepoUpdated{}, err
	}
	subject, err := u.getRepoNode(ctx, repoID)
	if err != nil {
		return RepoUpdated{}, fmt.Errorf("project: update repo: node: %w", err)
	}
	if in.Name != nil {
		repo.Name = *in.Name
		repo.AvatarLabel = avatar.Label(*in.Name)
		repo.AvatarColor = avatar.Color(*in.Name)
	}
	originFolder := subject.ParentID
	targetFolder, err := u.resolveTargetFolder(ctx, in, originFolder)
	if err != nil {
		return RepoUpdated{}, err
	}
	if err := u.repos.Save(ctx, *repo); err != nil {
		return RepoUpdated{}, fmt.Errorf("project: update repo: save: %w", err)
	}
	written, err := u.applyRepoPlacement(ctx, *repo, targetFolder, subject, in.Order)
	if err != nil {
		return RepoUpdated{}, err
	}
	// The folder the repo LEFT still has a gap where its row used to sit and
	// needs closing. A plain reorder within the same container is covered by
	// the densify above; this only fires for an actual move.
	if originFolder != targetFolder {
		left, err := u.densifyHomeLevel(ctx, repo.ProjectID, originFolder, repoID)
		if err != nil {
			return RepoUpdated{}, err
		}
		written = append(written, left...)
	}
	out := RepoUpdated{Repo: *repo, Node: subject}
	for _, n := range written {
		if n.ID == repoID {
			out.Node = n
			continue
		}
		out.Shifted = append(out.Shifted, n)
	}
	if updated, err := u.repos.FindByKey(ctx, repoID); err == nil && updated != nil {
		out.Repo = *updated
	}
	return out, nil
}

// getRepoNode reads repoID's own Node row. Every repo mints one at import
// (importOneRepo), so a missing row is an error, never a zero-value to mint
// on the first drag — all data is on the Node model (spec §6.3).
func (u *projectUsecase) getRepoNode(
	ctx context.Context,
	repoID string,
) (domain.Node, error) {
	n, err := u.nodes.GetNode(ctx, repoID)
	if err != nil {
		return domain.Node{}, err
	}
	return n, nil
}

// resolveTargetFolder validates and resolves the repo's target folder from
// the RepoUpdate: originFolder unchanged when in.FolderID is nil (nothing
// asked to move it), else the validated new value.
func (u *projectUsecase) resolveTargetFolder(
	ctx context.Context,
	in RepoUpdate,
	originFolder string,
) (string, error) {
	if in.FolderID == nil {
		return originFolder, nil
	}
	if err := u.validateRepoFolder(ctx, *in.FolderID); err != nil {
		return "", err
	}
	return *in.FolderID, nil
}

// applyRepoPlacement runs whichever of the two densify/place passes an
// update needs. An explicit order (a real drag) needs the CROSS-AGGREGATE
// placement — see placeRepoAmongHomeSiblings's own doc for why densifyRepos
// alone clamps it wrong the instant a home chat/folder shares the container.
// A move with no explicit order (e.g. a bare project/folder change from some
// other caller) keeps the old repo-only resort, which merely re-sorts by
// each row's own current (order, id) — nothing to place at a specific
// position, so nothing needs the wider sibling view.
func (u *projectUsecase) applyRepoPlacement(
	ctx context.Context,
	repo domain.Repository,
	targetFolder string,
	subject domain.Node,
	order *int,
) ([]domain.Node, error) {
	if order != nil {
		return u.placeRepoAmongHomeSiblings(ctx, repo.ProjectID, targetFolder, subject, *order)
	}
	return u.densifyRepos(ctx, repo.ProjectID, targetFolder, &subject)
}

// validateRepoFolder confirms folderID names a genuine project-home folder —
// "" (the project's own home root) is always legal; anything else must
// resolve to a domain.Folder row with no repo scope of its own (RepoID ==
// ""). A repo-internal folder (one that organises that repo's OWN branches,
// RepoID == the repo's id) is refused: a repo's entry is filed in some
// project's home, never inside its own tree — that would be a repo
// containing itself.
//
// This checks the SAME granularity checkFolderContainer (tree/validate.go)
// already accepts for an ordinary chat folder move, no finer: it does not
// confirm folderID belongs to THIS repo's own project specifically, because
// nothing in the chat tree currently anchors a root-level home folder to one
// project over another (every project's home folder scope reads "" alike).
// The frontend never offers a cross-project target; closing that gap for real
// would need a project anchor on the home tree itself, which is out of scope
// here (Task 8's own golden-rule Node port is where this would land, per this
// task's SDD ledger).
//
// This now validates against domain.Folder, not domain.Chat: home folders
// are Node/Folder-backed (2026-09-08 sidebar-placement-unification Task 5).
func (u *projectUsecase) validateRepoFolder(
	ctx context.Context,
	folderID string,
) error {
	if folderID == "" {
		return nil
	}
	folder, err := u.folders.FindByKey(ctx, folderID)
	if err != nil {
		return fmt.Errorf("project: update repo: folder %s: %w", folderID, err)
	}
	if folder == nil || folder.RepoID != "" {
		return fmt.Errorf(
			"project: update repo: %s is not a project-home folder: %w", folderID, apperr.ErrInvalidArgument,
		)
	}
	return nil
}

// refuseProjectMove answers ErrConflict for an update that names a project
// other than the repo's own. A repo's project is fixed at import (spec §3
// P0-3, invariant D6): everything it owns on disk — its managed worktrees
// (<home>/projects/<P>/<slug>/<branch>), its home checkout's chats tree, its
// workspaces' storage dirs, its entity dir — is keyed by that project, and
// live agents and terminals run inside those worktrees. Re-pointing the rows
// moved none of it (the worktrees stayed where another project's delete could
// reach them) and re-pointed the workspaces one by one, non-atomically. A repo
// that belongs elsewhere is removed and imported there instead.
func refuseProjectMove(
	repo domain.Repository,
	projectID *string,
) error {
	if projectID == nil || *projectID == repo.ProjectID {
		return nil
	}
	return fmt.Errorf(
		"project: update repo: a repository cannot change projects; remove it and import it into %s: %w",
		*projectID, apperr.ErrConflict,
	)
}

// repoIDSet answers the set of repo ids belonging to projectID — the project
// scope every repo-kind Node read below needs, since domain.Node carries no
// project id of its own (only ParentID/Order): membership comes from
// domain.Repository.ProjectID, unchanged by this migration, intersected
// against whatever Node.ListByParent returns.
func (u *projectUsecase) repoIDSet(
	ctx context.Context,
	projectID string,
) (map[string]bool, error) {
	rows, err := u.repos.FindWhere(ctx, domain.Repository{ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("project: reorder repos: list: %w", err)
	}
	ids := make(map[string]bool, len(rows))
	for _, r := range rows {
		ids[r.ID] = true
	}
	return ids, nil
}

// densifyRepos renumbers folderID's repo-kind sibling space within project
// projectID, 0..n-1, and writes back only the rows that moved. It never
// targets an explicit index (that is placeRepoAmongHomeSiblings's job) — it
// only closes gaps, sorting by each row's own current (order, id).
//
// subject, when given, is included AUTHORITATIVELY even when Node's read
// model has not yet folded a just-decided reparent for it: UpdateRepo passes
// its own pre-write read of the row (see getRepoNode) rather than trusting
// ListByParent(folderID) to already show it there, the same defence
// chat/internal/tree/plan.go's corrected() gives its own densify passes
// against the identical async-projection race SetOrder's own doc describes.
// When subject's own OLD parent differs from folderID, its write is
// SetPlacement (it is reparenting as part of this pass); every other row,
// subject included when its parent already matched, is SetOrder — the same
// per-row dispatch plan.go's writeRow makes.
//
// Scoped to folderID, not the whole project: a repo's Order is only dense
// among its OWN siblings — every other repo filed under that same
// project-home folder (or, for "", every other root-level repo) — mirroring
// how a workspace's Order densifies within its own folder rather than across
// a whole repo.
func (u *projectUsecase) densifyRepos(
	ctx context.Context,
	projectID string,
	folderID string,
	subject *domain.Node,
) ([]domain.Node, error) {
	memberIDs, err := u.repoIDSet(ctx, projectID)
	if err != nil {
		return nil, err
	}
	nodes, err := u.nodes.ListByParent(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("project: reorder repos: list nodes: %w", err)
	}
	rows := densifyRows(memberIDs, nodes, subject)
	subjectID := ""
	if subject != nil {
		subjectID = subject.ID
	}
	slots := nodeIndex(rows)
	written := make(map[string]bool, len(rows))
	var decided []domain.Node
	for _, moved := range place(slots, subjectID, nil) {
		row := rows[moved.at]
		written[row.ID] = true
		decided = append(decided, domain.Node{ID: row.ID, Kind: row.Kind, ParentID: folderID, Order: moved.order})
		reparenting := subject != nil && row.ID == subject.ID && subject.ParentID != folderID
		if err := u.writeNode(ctx, row.ID, folderID, moved.order, reparenting); err != nil {
			return nil, err
		}
	}
	if subject != nil {
		reparented := subject.ParentID != folderID
		n, err := u.ensureSubjectWritten(ctx, slots, subject.ID, reparented, folderID, nil, written)
		if err != nil {
			return nil, err
		}
		if n != nil {
			decided = append(decided, *n)
		}
	}
	return decided, nil
}

// densifyRows builds densifyRepos' candidate row list: every node belonging
// to projectID (memberIDs) and sitting in folderID's sibling space (nodes,
// from Node.ListByParent), minus subject's own stale copy — subject, when
// given, is appended in its place as the authoritative one (see
// densifyRepos' own doc for the race this defends against).
func densifyRows(
	memberIDs map[string]bool,
	nodes []domain.Node,
	subject *domain.Node,
) []domain.Node {
	rows := make([]domain.Node, 0, len(nodes)+1)
	for _, n := range nodes {
		if !memberIDs[n.ID] {
			continue
		}
		if subject != nil && n.ID == subject.ID {
			continue // superseded by subject below, which is authoritative
		}
		rows = append(rows, n)
	}
	if subject != nil {
		rows = append(rows, *subject)
	}
	return rows
}

// writeNode issues the one write a densify or placement pass owes a Node
// row — repo, chat, or folder alike, now that every home-scope sibling
// shares this one surface (2026-09-08 sidebar-placement-unification Task 5):
// SetPlacement when it is reparenting, SetOrder otherwise — the same per-row
// dispatch chat/internal/tree's own writeHomeNode makes.
func (u *projectUsecase) writeNode(
	ctx context.Context,
	id string,
	folderID string,
	order int,
	reparenting bool,
) error {
	if reparenting {
		if err := u.nodes.SetPlacement(ctx, id, folderID, order); err != nil {
			return fmt.Errorf("project: reorder repos: place %s: %w", id, err)
		}
		return nil
	}
	if err := u.nodes.SetOrder(ctx, id, order); err != nil {
		return fmt.Errorf("project: reorder repos: save %s: %w", id, err)
	}
	return nil
}

// mintNode is the "best effort, no backfill" half of degrading a Node-less
// row: a repo that predates this migration entirely (every real pre-existing
// repo in production — this migration ships with no backfill by design) has
// no Node row yet, and its FIRST reorder must Create one at exactly the
// resolved (folderID, order) rather than hand it to SetOrder/SetPlacement,
// which correctly refuse a row that was never Created (caught live: "node:
// set order: no node: asynx: validation failed" on the very first drag of a
// pre-existing repo). One Create call sets both fields the placement needs,
// so there is no separate reparenting branch to consider here.
func (u *projectUsecase) mintNode(
	ctx context.Context,
	id string,
	kind domain.NodeKind,
	folderID string,
	order int,
) error {
	if _, err := u.nodes.Create(ctx, id, kind, folderID, order); err != nil {
		return fmt.Errorf("project: reorder repos: mint %s: %w", id, err)
	}
	return nil
}

// ensureSubjectWritten guarantees a REPARENTING subject lands in the container
// it was asked for even when place()'s numeric diff saw no move to make — it
// landed back on the same dense index it already held, which only compares
// ORDER values, not parents. slots is the ORIGINAL (pre-sort) slot list.
func (u *projectUsecase) ensureSubjectWritten(
	ctx context.Context,
	slots []slot,
	subjectID string,
	reparented bool,
	folderID string,
	target *int,
	written map[string]bool,
) (*domain.Node, error) {
	if written[subjectID] {
		return nil, nil
	}
	i := finalIndexOf(slots, subjectID, target)
	if !reparented || i < 0 {
		return nil, nil // place() correctly found no write needed
	}
	if err := u.nodes.SetPlacement(ctx, subjectID, folderID, i); err != nil {
		return nil, fmt.Errorf("project: reorder repos: place %s: %w", subjectID, err)
	}
	return &domain.Node{ID: subjectID, Kind: domain.NodeKindRepo, ParentID: folderID, Order: i}, nil
}

// placeRepoAmongHomeSiblings gives subject exactly the Order the caller asked
// for within (projectID, folderID), shifting whatever ELSE shares that
// container — other repos, home chats, AND home folders alike — out of its
// way. The member set is homeLevel's: the one the sidebar renders.
//
// subject is included AUTHORITATIVELY — UpdateRepo's pre-write read of the
// row being placed, not merely whatever Node.ListByParent(folderID) currently
// answers. When subject's own OLD parent differs from folderID it is
// reparenting as part of this call and its write is SetPlacement; every other
// row, subject included when its parent already matched, is SetOrder. A row
// with no Node yet (the subject before its first drag, or a legacy root chat)
// is minted at the index it lands on.
func (u *projectUsecase) placeRepoAmongHomeSiblings(
	ctx context.Context,
	projectID string,
	folderID string,
	subject domain.Node,
	target int,
) ([]domain.Node, error) {
	siblings, homeWorkspaceID, err := u.homeLevel(ctx, projectID, folderID, subject.ID)
	if err != nil {
		return nil, err
	}
	rows := append(siblings, homeRow{Node: subject})
	slots := homeIndex(rows)
	reparented := subject.ParentID != folderID
	decided, written, err := u.writeHomeLevel(ctx, rows, place(slots, subject.ID, &target),
		folderID, homeWorkspaceID, subject.ID, reparented)
	if err != nil {
		return nil, err
	}
	n, err := u.ensureSubjectWritten(ctx, slots, subject.ID, reparented, folderID, &target, written)
	if err != nil {
		return nil, err
	}
	if n != nil {
		decided = append(decided, *n)
	}
	return decided, nil
}

// densifyHomeLevel closes the gap a repo left in (projectID, folderID) over
// the level's full member set — repos, home chats and home folders alike —
// so the rows that stay keep their drawn order. exclude is the row known to
// have LEFT even if Node's read model has not yet folded its departure.
func (u *projectUsecase) densifyHomeLevel(
	ctx context.Context,
	projectID string,
	folderID string,
	exclude string,
) ([]domain.Node, error) {
	rows, homeWorkspaceID, err := u.homeLevel(ctx, projectID, folderID, exclude)
	if err != nil {
		return nil, err
	}
	decided, _, err := u.writeHomeLevel(ctx, rows, place(homeIndex(rows), "", nil),
		folderID, homeWorkspaceID, "", false)
	return decided, err
}

// writeHomeLevel writes every move a home-level pass decided, mints the
// fresh rows the level touched and announces the chats/folders it shifted.
func (u *projectUsecase) writeHomeLevel(
	ctx context.Context,
	rows []homeRow,
	moves []move,
	folderID string,
	homeWorkspaceID string,
	subjectID string,
	reparented bool,
) ([]domain.Node, map[string]bool, error) {
	written := make(map[string]bool, len(rows))
	var decided []domain.Node
	for _, moved := range moves {
		row := rows[moved.at]
		written[row.ID] = true
		decided = append(decided, domain.Node{ID: row.ID, Kind: row.Kind, ParentID: folderID, Order: moved.order})
		if row.fresh {
			if err := u.mintNode(ctx, row.ID, row.Kind, folderID, moved.order); err != nil {
				return nil, nil, err
			}
		} else {
			reparenting := row.ID == subjectID && reparented
			if err := u.writeNode(ctx, row.ID, folderID, moved.order, reparenting); err != nil {
				return nil, nil, err
			}
		}
		// The moved chat/folder rows only, not the repo subject: PlaceChat's
		// own announce covers a chat/folder's OWN drag, this covers the same
		// rows moving as collateral of a REPO drag instead.
		if row.ID != subjectID && u.broadcastChat != nil && homeWorkspaceID != "" &&
			(row.Kind == domain.NodeKindChat || row.Kind == domain.NodeKindFolder) {
			u.broadcastChat(row.ID, homeWorkspaceID, row.Kind, "order_set")
		}
	}
	// A legacy root chat whose slot did not move (place reports no change, so
	// its order IS its final index) still becomes Node-backed: the level was
	// touched, and the next read must find every row on one surface.
	for _, row := range rows {
		if !row.fresh || written[row.ID] || row.ID == subjectID {
			continue
		}
		written[row.ID] = true
		if err := u.mintNode(ctx, row.ID, row.Kind, folderID, row.Order); err != nil {
			return nil, nil, err
		}
	}
	return decided, written, nil
}

// Reorder sets a project's sidebar index and densifies the whole list. Projects
// are the sidebar's top level, so the full set IS the sibling space and reading
// all of them is the scope, not an over-read.
func (u *projectUsecase) Reorder(
	ctx context.Context,
	projectID string,
	order int,
) (domain.Project, error) {
	rows, err := u.projects.FindAll(ctx)
	if err != nil {
		return domain.Project{}, fmt.Errorf("project: reorder: list: %w", err)
	}
	if !containsID(projectIndex(rows), projectID) {
		return domain.Project{}, fmt.Errorf("project: reorder: id %s: %w", projectID, apperr.ErrNotFound)
	}
	for _, moved := range place(projectIndex(rows), projectID, &order) {
		rows[moved.at].Order = moved.order
		if err := u.projects.Save(ctx, rows[moved.at]); err != nil {
			return domain.Project{}, fmt.Errorf("project: reorder: save %s: %w", rows[moved.at].ID, err)
		}
	}
	updated, err := u.projects.FindByKey(ctx, projectID)
	if err != nil || updated == nil {
		return domain.Project{}, fmt.Errorf("project: reorder: id %s: %w", projectID, apperr.ErrNotFound)
	}
	return *updated, nil
}

// Update applies a partial project update in one load-mutate-save.
func (u *projectUsecase) Update(
	ctx context.Context,
	projectID string,
	in Update,
) (domain.Project, error) {
	row, err := u.projects.FindByKey(ctx, projectID)
	if err != nil {
		return domain.Project{}, fmt.Errorf("project: update: load %s: %w", projectID, err)
	}
	if row == nil {
		return domain.Project{}, fmt.Errorf("project: update: id %s: %w", projectID, apperr.ErrNotFound)
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return domain.Project{}, fmt.Errorf("project: update: name is empty: %w", apperr.ErrInvalidArgument)
		}
		row.Name = name
	}
	if in.AvatarEmoji != nil {
		row.AvatarEmoji = *in.AvatarEmoji
	}
	if in.AvatarHasIcon != nil {
		row.AvatarHasIcon = *in.AvatarHasIcon
	}
	if in.BumpAvatarVersion {
		row.AvatarVersion++
	}
	if err := u.projects.Save(ctx, *row); err != nil {
		return domain.Project{}, fmt.Errorf("project: update: save %s: %w", projectID, err)
	}
	return *row, nil
}
