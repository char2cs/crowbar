package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	store "github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	noderepo "github.com/char2cs/crowbar/api/internal/app/repositories/node"
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
	UpdateRepo(
		ctx context.Context,
		repoID string,
		in RepoUpdate,
	) (domain.Repository, error)
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
// ProjectID moves the repo to another project, which also carries every
// workspace under it — see WorkspaceRelocator. FolderID/Order re-file the
// repo's own entry within its project's home tree — written to the repo's
// own Node row (NodePlacements), interleaved against its real home chat and
// folder siblings, which are Node-backed too now (2026-09-08
// sidebar-placement-unification Task 5) — see placeRepoAmongHomeSiblings.
type RepoUpdate struct {
	Name      *string
	ProjectID *string
	Order     *int
	FolderID  *string
}

// WorkspaceRelocator is the narrow workspace surface a repo move needs. Every
// workspace carries a denormalised ProjectID that the hierarchical routes and
// the WS namespace are keyed on, so a repo that changed projects while its
// workspaces did not would keep them and stop showing them.
//
// GetHomeForProject answers the one other thing a repo's OWN home placement
// needs: which workspace IS project home, so a repo's sibling search
// (placeRepoAmongHomeSiblings) can scope a ROOT-level container to THIS
// project specifically — home chats otherwise carry no project id of their
// own to filter by (see repoScopeOf's doc elsewhere: every project's home
// resolves to the same "" repo scope). Satisfied structurally by the
// workspace REPOSITORY (not the usecase — container.go builds this one
// before the workspace usecase exists, which itself depends on this
// package), the same adapter home.Register's own HomeWorkspaces port uses.
type WorkspaceRelocator interface {
	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Workspace, error)
	SetProject(
		ctx context.Context,
		id string,
		projectID string,
	) (domain.Workspace, error)
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
	workspaces WorkspaceRelocator
	folders    Folders
	nodes      NodePlacements
	homeChats  HomeChats
	// broadcastChat announces a collaterally-shifted CHAT/FOLDER-kind Node
	// row on the same chats WS a chat/folder placement already uses — see
	// placeRepoAmongHomeSiblings' own doc on why this exists at all. Nilable,
	// degrading to silence like every other optional dependency here: a repo
	// reorder still WRITES correctly with it nil, it just leaves a live
	// client's chat siblings stale until their next reseed.
	broadcastChat func(id, workspaceID, kind string)
}

// New builds a Usecase from the project and repository GORM stores, the
// workspace relocator a cross-project repo move needs, the home-folder
// identity store validateRepoFolder checks a FolderID against, the Node
// surface that now owns every home-scope sibling's OWN position (see
// NodePlacements), and the chat-membership read that keeps a bare-root
// placement from leaking another project's chats into this one's densify
// (see HomeChats). A nil homeChats degrades the same way a nil workspaces
// already does elsewhere in this file: the bare-root densify simply cannot
// narrow CHAT-kind siblings to this project and falls back to including
// them all, rather than failing the request outright.
//
// broadcastChat is the chats-WS announce callback (Hub.BroadcastAgentChatFolder
// in production) a repo reorder needs for exactly the same reason PlaceChat
// does (see that handler's own comment): every home-scope sibling's write now
// rides Node, which has no aggregate-command hub projection of its own, so
// nothing tells a live client a CHAT/FOLDER row it did not drag also moved as
// collateral of the repo it did drag.
func New(
	projects store.Store[domain.Project, string],
	repos store.ScopedStore[domain.Repository, string],
	workspaces WorkspaceRelocator,
	folders Folders,
	nodes NodePlacements,
	homeChats HomeChats,
	broadcastChat func(id, workspaceID, kind string),
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
//
// A project move carries the repo's workspaces with it and renumbers BOTH
// projects' repo lists. It moves nothing on disk: worktree paths were derived
// once and are stored absolute, so they keep resolving from where they are.
func (u *projectUsecase) UpdateRepo(
	ctx context.Context,
	repoID string,
	in RepoUpdate,
) (domain.Repository, error) {
	repo, err := u.repos.FindByKey(ctx, repoID)
	if err != nil {
		return domain.Repository{}, fmt.Errorf("project: update repo: %w", err)
	}
	if repo == nil {
		return domain.Repository{}, fmt.Errorf("project: update repo: id %s: %w", repoID, apperr.ErrNotFound)
	}
	subject, subjectExists, err := u.getRepoNode(ctx, repoID)
	if err != nil {
		return domain.Repository{}, fmt.Errorf("project: update repo: node: %w", err)
	}
	if in.Name != nil {
		repo.Name = *in.Name
		repo.AvatarLabel = avatar.Label(*in.Name)
		repo.AvatarColor = avatar.Color(*in.Name)
	}
	origin := repo.ProjectID
	originFolder := subject.ParentID
	if mErr := u.applyRepoProject(ctx, repo, in.ProjectID); mErr != nil {
		return domain.Repository{}, mErr
	}
	targetFolder, err := u.resolveTargetFolder(ctx, in, originFolder)
	if err != nil {
		return domain.Repository{}, err
	}
	if err := u.repos.Save(ctx, *repo); err != nil {
		return domain.Repository{}, fmt.Errorf("project: update repo: save: %w", err)
	}
	if err := u.applyRepoPlacement(ctx, *repo, targetFolder, subject, subjectExists, in.Order); err != nil {
		return domain.Repository{}, err
	}
	// The container the repo LEFT — whether it moved project, folder, or both —
	// still has a gap where its row used to sit and needs closing. A plain
	// reorder within the same container is covered by the densify above; this
	// only fires for an actual move.
	if origin != repo.ProjectID || originFolder != targetFolder {
		if err := u.densifyReposExcluding(ctx, origin, originFolder, repoID); err != nil {
			return domain.Repository{}, err
		}
	}
	updated, err := u.repos.FindByKey(ctx, repoID)
	if err != nil || updated == nil {
		return *repo, nil
	}
	return *updated, nil
}

// getRepoNode reads repoID's own Node row, degrading to a fresh zero-value
// (ParentID "", Order 0 — the project-home root) when none exists yet rather
// than failing the update: a repo seeded directly (a test fixture, or a row
// written before this migration/through the bare buildRepo+Save fallback with
// no importer wired) has no Node row, and every UpdateRepo call — even a bare
// rename — must still work.
//
// The second return value is the one thing the zero-value degrade loses on
// its own: whether that row is real. writeNode/forceReparentWrite need this —
// a row that has never been Created must be Created on its first write, not
// handed to SetOrder/SetPlacement (which correctly refuse a row that doesn't
// exist yet) — this is the mint-on-first-touch half of "best effort, no
// backfill" that only degrading the READ side (this function, before this
// fix) never actually delivered for the write.
func (u *projectUsecase) getRepoNode(
	ctx context.Context,
	repoID string,
) (domain.Node, bool, error) {
	n, err := u.nodes.GetNode(ctx, repoID)
	if err != nil {
		if errors.Is(err, noderepo.ErrNotFound) {
			return domain.Node{ID: repoID, Kind: domain.NodeKindRepo}, false, nil
		}
		return domain.Node{}, false, err
	}
	return n, true, nil
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
	subjectExists bool,
	order *int,
) error {
	if order != nil {
		return u.placeRepoAmongHomeSiblings(ctx, repo.ProjectID, targetFolder, subject, subjectExists, *order)
	}
	return u.densifyRepos(ctx, repo.ProjectID, targetFolder, &subject, subjectExists)
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

// applyRepoProject moves repo to another project, relocating every workspace
// under it. The workspace relocation runs BEFORE the repo row is saved so a
// failure leaves the repo where its workspaces still are, rather than the other
// way round.
func (u *projectUsecase) applyRepoProject(
	ctx context.Context,
	repo *domain.Repository,
	projectID *string,
) error {
	if projectID == nil || *projectID == repo.ProjectID {
		return nil
	}
	target, err := u.projects.FindByKey(ctx, *projectID)
	if err != nil {
		return fmt.Errorf("project: update repo: resolve project: %w", err)
	}
	if target == nil {
		return fmt.Errorf("project: update repo: project %s: %w", *projectID, apperr.ErrNotFound)
	}
	if u.workspaces == nil {
		return fmt.Errorf("project: update repo: no workspace relocator wired")
	}
	rows, err := u.workspaces.ListInRepo(ctx, repo.ProjectID, repo.ID)
	if err != nil {
		return fmt.Errorf("project: update repo: list workspaces: %w", err)
	}
	for _, ws := range rows {
		if _, err := u.workspaces.SetProject(ctx, ws.ID, *projectID); err != nil {
			return fmt.Errorf("project: update repo: relocate workspace %s: %w", ws.ID, err)
		}
	}
	repo.ProjectID = *projectID
	return nil
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

// homeChatIDSet answers the set of chat ids belonging to projectID's own
// home workspace, scoped ONLY at the bare project-home root (folderID == "")
// — placeRepoAmongHomeSiblings' own doc explains why: a real folder id is
// globally unique and safely scopes its own children by construction, but
// the bare root has no such anchor for a CHAT-kind Node row (unlike a repo,
// scoped via Repository.ProjectID — repoIDSet). A nil return (not an empty,
// non-nil map) means "do not filter" — folderID != "", u.homeChats being
// unwired (test fixtures that don't care about this scoping), or an
// unresolved home workspace all degrade to the OLD (Task 5's first version)
// posture of including every chat sharing the container, matching
// homeContainerChats' own identical degrade before this task deleted it.
func (u *projectUsecase) homeChatIDSet(
	ctx context.Context,
	projectID string,
	folderID string,
) (map[string]bool, error) {
	if folderID != "" || u.homeChats == nil || u.workspaces == nil {
		return nil, nil
	}
	ws, err := u.workspaces.GetHomeForProject(ctx, projectID)
	if err != nil {
		return nil, nil
	}
	chats, err := u.homeChats.ListByWorkspace(ctx, ws.ID)
	if err != nil {
		return nil, fmt.Errorf("project: reorder repos: list home chats: %w", err)
	}
	ids := make(map[string]bool, len(chats))
	for _, c := range chats {
		ids[c.ID] = true
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
	subjectExists bool,
) error {
	return u.densifyReposScoped(ctx, projectID, folderID, subject, subjectExists, "")
}

// densifyReposExcluding is densifyRepos with no subject to include, but a
// specific id to drop even if Node's read model has not yet folded its
// departure — used to close the gap in the container a repo just LEFT, where
// there is nothing to place, only a stray reference to defend against.
func (u *projectUsecase) densifyReposExcluding(
	ctx context.Context,
	projectID string,
	folderID string,
	exclude string,
) error {
	return u.densifyReposScoped(ctx, projectID, folderID, nil, false, exclude)
}

func (u *projectUsecase) densifyReposScoped(
	ctx context.Context,
	projectID string,
	folderID string,
	subject *domain.Node,
	subjectExists bool,
	exclude string,
) error {
	memberIDs, err := u.repoIDSet(ctx, projectID)
	if err != nil {
		return err
	}
	nodes, err := u.nodes.ListByParent(ctx, folderID)
	if err != nil {
		return fmt.Errorf("project: reorder repos: list nodes: %w", err)
	}
	rows := densifyRows(memberIDs, nodes, subject, exclude)
	subjectID := ""
	if subject != nil {
		subjectID = subject.ID
	}
	slots := nodeIndex(rows)
	written := make(map[string]bool, len(rows))
	for _, moved := range place(slots, subjectID, nil) {
		row := rows[moved.at]
		written[row.ID] = true
		if subject != nil && row.ID == subject.ID && !subjectExists {
			if err := u.mintNode(ctx, row.ID, folderID, moved.order); err != nil {
				return err
			}
			continue
		}
		reparenting := subject != nil && row.ID == subject.ID && subject.ParentID != folderID
		if err := u.writeNode(ctx, row.ID, folderID, moved.order, reparenting); err != nil {
			return err
		}
	}
	if subject != nil {
		reparented := subject.ParentID != folderID
		if err := u.ensureSubjectWritten(ctx, slots, subject.ID, subjectExists, reparented, folderID, nil, written); err != nil {
			return err
		}
	}
	return nil
}

// densifyRows builds densifyReposScoped's candidate row list: every node
// belonging to projectID (memberIDs) and sitting in folderID's sibling space
// (nodes, from Node.ListByParent), minus exclude (a row known to have LEFT
// even if the read model has not yet folded that) and minus subject's own
// stale copy — subject, when given, is appended in its place as the
// authoritative one (see densifyReposScoped's own doc for the race this
// defends against).
func densifyRows(
	memberIDs map[string]bool,
	nodes []domain.Node,
	subject *domain.Node,
	exclude string,
) []domain.Node {
	rows := make([]domain.Node, 0, len(nodes)+1)
	for _, n := range nodes {
		if !memberIDs[n.ID] || n.ID == exclude {
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
	folderID string,
	order int,
) error {
	if _, err := u.nodes.Create(ctx, id, domain.NodeKindRepo, folderID, order); err != nil {
		return fmt.Errorf("project: reorder repos: mint %s: %w", id, err)
	}
	return nil
}

// ensureSubjectWritten guarantees the subject ends up in the state this call
// actually asked for, even when place()'s numeric diff saw no move to make —
// two DIFFERENT coincidences collapse to the same blind spot, and both need
// the SAME unconditional (not "only if reparenting") check here, not just the
// main densify loop's per-row dispatch:
//
//   - A genuinely NEW row (subjectExists false — every real pre-existing
//     repo in production, since this migration ships with no backfill) whose
//     first-ever placement happens to land on the exact index its zero-value
//     degrade already reads as (dragging a lone repo to "the front" is
//     already order 0 before it has ever been Created) — invisible to
//     place()'s diff, which only compares ORDER values, not existence. Caught
//     live: "node: set order: no node: asynx: validation failed" on the very
//     first drag of a pre-existing repo, reproduced in
//     TestRegression_UpdateRepo_PreExistingRepoWithNoNodeRowStillReorders.
//   - A REPARENTING existing row that lands back on the same dense index it
//     already held (the original, narrower case this function used to be
//     named for, before the Node-less case above showed the SAME gap needed
//     the SAME unconditional check).
//
// slots is the ORIGINAL (pre-sort) slot list — finalIndexOf takes its own
// copy and never mutates the caller's. Called after EVERY densify/place pass,
// not gated behind "if reparented": a Node-less subject may need minting
// regardless of whether its resolved parent happens to differ from its
// (meaningless, zero-value) current one.
func (u *projectUsecase) ensureSubjectWritten(
	ctx context.Context,
	slots []slot,
	subjectID string,
	subjectExists bool,
	reparented bool,
	folderID string,
	target *int,
	written map[string]bool,
) error {
	if written[subjectID] {
		return nil
	}
	if !subjectExists {
		i := finalIndexOf(slots, subjectID, target)
		if i < 0 {
			i = 0
		}
		return u.mintNode(ctx, subjectID, folderID, i)
	}
	if !reparented {
		return nil // a real, existing row place() correctly found no write needed for
	}
	i := finalIndexOf(slots, subjectID, target)
	if i < 0 {
		return nil
	}
	if err := u.nodes.SetPlacement(ctx, subjectID, folderID, i); err != nil {
		return fmt.Errorf("project: reorder repos: place %s: %w", subjectID, err)
	}
	return nil
}

// placeRepoAmongHomeSiblings gives subject exactly the Order the caller asked
// for within (projectID, folderID), shifting whatever ELSE shares that
// container — other repos, home chats, AND home folders alike — out of its
// way.
//
// This is the fix for a real, caught-live bug: densifyRepos (above) only
// ever renumbers a repo among its OWN kind, clamping any requested target to
// "however many OTHER REPOS share this folder" — reinsert (ordering.go)
// clamps to len(slots) after removing the moved one, and a project with a
// single repo has zero OTHER repos to clamp against, so EVERY move of that
// repo silently snapped back to order 0 no matter what position the drag
// asked for. A repo's row renders in the SAME sibling sort as every home
// chat/folder around it (SidebarTree's roots.sort(byOrder), fed by
// rowsFromHome's own repo-interleave — see that file's doc), so its Order
// has to be placed against ALL of them, not a repo-only subset.
//
// Every home-scope sibling is Node-backed now (2026-09-08
// sidebar-placement-unification Task 5 made chats/folders Node-backed too,
// same as repos already were), so this reads ONE Node.ListByParent(folderID)
// call — no more cross-aggregate merge with the chat package for FOLDER
// CRUD/reads at all. Repos are narrowed to this project via memberIDs
// (Node carries no project id of its own — see repoIDSet's doc). CHATS are
// narrowed the same way via homeChatIDSet — a fix-round-2 correction: the
// FIRST version of this function let them through unconditionally, which
// looked safe (folder ids are globally unique, so a real folderID's own
// children can't leak) but was WRONG at the bare root: folderID == "" has no
// per-project anchor for a chat any more than it does for a repo, and every
// project's home-scoped chat Node rows literally share that one container —
// an unfiltered pass renumbered (and WROTE) another project's chats as a
// side effect of THIS project's own repo reorder. FOLDER-kind rows sharing
// the bare root are still included unconditionally: a home Folder has no
// project field at all to filter by, the SAME pre-existing, disclosed gap
// validateRepoFolder's own doc already carried before this task (see this
// task's report for the full account).
//
// subject is included AUTHORITATIVELY (see densifyRepos's own doc for why) —
// UpdateRepo's pre-write read of the row being placed, not merely whatever
// Node.ListByParent(folderID) currently answers. When subject's own OLD
// parent differs from folderID it is reparenting as part of this call and its
// write is SetPlacement; every other row, subject included when its parent
// already matched, is SetOrder.
//
// homeSiblingRows is the filter itself, factored out of
// placeRepoAmongHomeSiblings so the scope rules for each kind read as one
// small pass rather than adding a branch to an already-long function: repos
// are dropped when memberIDs excludes them (repoIDSet, this project's own
// repos only), chats are dropped when chatMemberIDs is non-nil and excludes
// them (homeChatIDSet — nil means "do not filter," see its own doc), and
// folder-kind rows pass through unconditionally (the disclosed, pre-existing
// gap this task does not attempt to close).
func homeSiblingRows(
	memberIDs map[string]bool,
	chatMemberIDs map[string]bool,
	nodes []domain.Node,
	subjectID string,
) []domain.Node {
	rows := make([]domain.Node, 0, len(nodes))
	for _, n := range nodes {
		if n.ID == subjectID {
			continue // subject is appended by the caller, authoritatively
		}
		if n.Kind == domain.NodeKindRepo && !memberIDs[n.ID] {
			continue
		}
		if n.Kind == domain.NodeKindChat && chatMemberIDs != nil && !chatMemberIDs[n.ID] {
			continue
		}
		rows = append(rows, n)
	}
	return rows
}

func (u *projectUsecase) placeRepoAmongHomeSiblings(
	ctx context.Context,
	projectID string,
	folderID string,
	subject domain.Node,
	subjectExists bool,
	target int,
) error {
	memberIDs, err := u.repoIDSet(ctx, projectID)
	if err != nil {
		return err
	}
	chatMemberIDs, err := u.homeChatIDSet(ctx, projectID, folderID)
	if err != nil {
		return err
	}
	nodes, err := u.nodes.ListByParent(ctx, folderID)
	if err != nil {
		return fmt.Errorf("project: reorder repos: list nodes: %w", err)
	}
	rows := append(homeSiblingRows(memberIDs, chatMemberIDs, nodes, subject.ID), subject)
	slots := nodeIndex(rows)

	// Resolved once, best-effort: a repo's OWN broadcast is the caller's job
	// (UpdateRepo's handler already re-fetches and broadcasts the repo DTO);
	// this is only for announcing a COLLATERAL chat/folder sibling this densify
	// renumbers but never returns to that caller at all. An error or a nil
	// workspaces dependency degrades to "announce nothing" — the write above
	// already committed either way — not a failed reorder.
	var homeWorkspaceID string
	if u.broadcastChat != nil && u.workspaces != nil {
		if ws, err := u.workspaces.GetHomeForProject(ctx, projectID); err == nil {
			homeWorkspaceID = ws.ID
		}
	}

	reparented := subject.ParentID != folderID
	written := make(map[string]bool, len(slots))
	for _, moved := range place(slots, subject.ID, &target) {
		row := rows[moved.at]
		written[row.ID] = true
		if row.ID == subject.ID && !subjectExists {
			if err := u.mintNode(ctx, row.ID, folderID, moved.order); err != nil {
				return err
			}
			continue
		}
		reparenting := row.ID == subject.ID && reparented
		if err := u.writeNode(ctx, row.ID, folderID, moved.order, reparenting); err != nil {
			return err
		}
		// The moved chat/folder rows only, not the repo subject: PlaceChat's
		// own announce covers a chat/folder's OWN drag, this covers the same
		// rows moving as collateral of a REPO drag instead (caught live: a
		// repo dragged above a chat left that chat's stale order tied
		// against the repo's new one, so the repo never visibly passed it).
		if row.ID != subject.ID && homeWorkspaceID != "" &&
			(row.Kind == domain.NodeKindChat || row.Kind == domain.NodeKindFolder) {
			u.broadcastChat(row.ID, homeWorkspaceID, "order_set")
		}
	}
	if err := u.ensureSubjectWritten(ctx, slots, subject.ID, subjectExists, reparented, folderID, &target, written); err != nil {
		return err
	}
	return nil
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
