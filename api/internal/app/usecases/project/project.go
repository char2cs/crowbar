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
// own Node row (NodePlacements), interleaved against real home chats/folders
// (HomeFolders) that are not yet Node-backed themselves — see
// placeRepoAmongHomeSiblings.
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

// HomeFolders is the narrow chat-tree surface a repo's OWN home placement
// needs: Get confirms a FolderID names a genuine project-home folder before
// writing it (the same golden rule tree/validate.go's checkFolderContainer
// already enforces for an ordinary chat folder — "" is always legal, nothing
// to inherit a scope from; anything else must resolve and be home-scoped).
// ListByWorkspace/ListChats/SetOrder are placeRepoAmongHomeSiblings's own
// chat-side surface: reading every chat/folder sharing a repo's exact
// container, and writing back whichever ones a repo's own move displaces —
// see that function's doc for why a repo's Order cannot be densified in
// isolation from them. This whole surface stays UNCHANGED by the repo's own
// migration onto Node (NodePlacements, above): home chats/folders are not yet
// Node-backed themselves — a later task in the plan. Satisfied structurally
// by the chat repository itself (repos.AgentChat), which already answers the
// full agentic Chats port (chat/internal/tree/types.go) this is a subset of.
type HomeFolders interface {
	Get(ctx context.Context, id string) (domain.Chat, error)
	ListByWorkspace(ctx context.Context, workspaceID string) ([]domain.Chat, error)
	ListChats(ctx context.Context) ([]domain.Chat, error)
	SetOrder(ctx context.Context, chatID string, order int) (domain.Chat, error)
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
// This is deliberately the REPO side only. Home chats/folders are not yet
// Node-backed (that is a later task in the plan) and keep reading/writing
// through HomeFolders above, unchanged — see placeRepoAmongHomeSiblings.
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
	projects    store.Store[domain.Project, string]
	repos       store.ScopedStore[domain.Repository, string]
	workspaces  WorkspaceRelocator
	homeFolders HomeFolders
	nodes       NodePlacements
}

// New builds a Usecase from the project and repository GORM stores, the
// workspace relocator a cross-project repo move needs, the chat-tree read port
// a repo's own home-folder placement needs to interleave against real
// chat/folder siblings, and the Node surface that now owns a repo's OWN
// position (see NodePlacements).
func New(
	projects store.Store[domain.Project, string],
	repos store.ScopedStore[domain.Repository, string],
	workspaces WorkspaceRelocator,
	homeFolders HomeFolders,
	nodes NodePlacements,
) Usecase {
	return &projectUsecase{
		projects:    projects,
		repos:       repos,
		workspaces:  workspaces,
		homeFolders: homeFolders,
		nodes:       nodes,
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
	subject, err := u.getRepoNode(ctx, repoID)
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
	if err := u.applyRepoPlacement(ctx, *repo, targetFolder, subject, in.Order); err != nil {
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
func (u *projectUsecase) getRepoNode(
	ctx context.Context,
	repoID string,
) (domain.Node, error) {
	n, err := u.nodes.GetNode(ctx, repoID)
	if err != nil {
		if errors.Is(err, noderepo.ErrNotFound) {
			return domain.Node{ID: repoID, Kind: domain.NodeKindRepo}, nil
		}
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
) error {
	if order != nil {
		return u.placeRepoAmongHomeSiblings(ctx, repo.ProjectID, targetFolder, subject, *order)
	}
	return u.densifyRepos(ctx, repo.ProjectID, targetFolder, &subject)
}

// validateRepoFolder confirms folderID names a genuine project-home folder —
// "" (the project's own home root) is always legal; anything else must
// resolve to a domain.Chat row that IS a folder and carries no repo scope of
// its own (RepoID == ""). A repo-internal folder (one that organises that
// repo's OWN branches, RepoID == the repo's id) is refused: a repo's entry is
// filed in some project's home, never inside its own tree — that would be a
// repo containing itself.
//
// This checks the SAME granularity checkFolderContainer (tree/validate.go)
// already accepts for an ordinary chat folder move, no finer: it does not
// confirm folderID belongs to THIS repo's own project specifically, because
// nothing in the chat tree currently anchors a root-level home folder to one
// project over another (every project's home folder scope reads "" alike).
// The frontend never offers a cross-project target; closing that gap for real
// would need a project anchor on the home tree itself, which is out of scope
// here.
//
// This still validates against domain.Chat, not domain.Node: home folders are
// not yet Node-backed (a later task in the plan). Once every kind shares one
// Node.ListByParent read, a folder id's containment becomes the SAME golden
// rule every other kind's ParentID is checked against — that generalisation is
// a different task's job, not this one's.
func (u *projectUsecase) validateRepoFolder(
	ctx context.Context,
	folderID string,
) error {
	if folderID == "" {
		return nil
	}
	folder, err := u.homeFolders.Get(ctx, folderID)
	if err != nil {
		return fmt.Errorf("project: update repo: folder %s: %w", folderID, err)
	}
	if folder.Type != domain.ChatTypeFolder || folder.RepoID != "" {
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
) error {
	return u.densifyReposScoped(ctx, projectID, folderID, subject, "")
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
	return u.densifyReposScoped(ctx, projectID, folderID, nil, exclude)
}

func (u *projectUsecase) densifyReposScoped(
	ctx context.Context,
	projectID string,
	folderID string,
	subject *domain.Node,
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
		reparenting := subject != nil && row.ID == subject.ID && subject.ParentID != folderID
		if err := u.writeRepoNode(ctx, row.ID, folderID, moved.order, reparenting); err != nil {
			return err
		}
	}
	if subject != nil && subject.ParentID != folderID {
		if err := u.forceReparentWrite(ctx, slots, subject.ID, folderID, nil, written); err != nil {
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

// writeRepoNode issues the one write a densify or placement pass owes a repo
// row: SetPlacement when it is reparenting, SetOrder otherwise — the same
// per-row dispatch plan.go's writeRow makes for chats.
func (u *projectUsecase) writeRepoNode(
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

// forceReparentWrite guarantees a reparenting subject's SetPlacement actually
// happens even when place() recorded no move for it — see finalIndexOf's own
// doc for the coincidence this defends against (a reparent that lands back on
// the same dense index it already held, invisible to place()'s numeric diff).
// slots is the ORIGINAL (pre-sort) slot list — finalIndexOf takes its own copy
// and never mutates the caller's.
func (u *projectUsecase) forceReparentWrite(
	ctx context.Context,
	slots []slot,
	subjectID string,
	folderID string,
	target *int,
	written map[string]bool,
) error {
	if written[subjectID] {
		return nil
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

// homeContainerSibling is one non-repo row sharing a repo's exact
// project-home container — a chat or a folder, {@link placeRepoAmongHomeSiblings}
// only ever needs its id, kind and current Order to seat a repo among them.
type homeContainerSibling struct {
	id    string
	order int
}

// homeContainerChats answers every chat/folder sharing ONE project-home
// container — the repo-agnostic half of {@link placeRepoAmongHomeSiblings}'s
// sibling space. Unchanged by the Node migration: home chats/folders are not
// yet Node-backed, and keep reading/writing through HomeFolders exactly as
// before — only the REPO side of the merge that follows moved onto Node.
//
// A nested folder (folderID != "") is scoped by ParentID alone: a folder id
// is globally unique, so filtering the daemon's WHOLE chat/folder set
// (ListChats) by it can never leak another project's row in — this is the
// SAME safety `checkFolderContainer`'s golden rule already leans on.
//
// The bare project-home ROOT (folderID == "") has no such anchor for a
// FOLDER row (a folder never carries a WorkspaceID — see domain.Chat's own
// doc — so "" repo scope is indistinguishable between projects there, the
// same pre-existing gap validateRepoFolder's own doc discloses). CHATS are
// still scoped correctly there via homeWorkspaceID (ListByWorkspace), which
// IS project-specific — so root-level folders are left out of the sibling
// space rather than risked against the wrong project, and only chats are
// returned. This narrows what a repo dropped at the bare root can correctly
// displace to its sibling CHATS, not root-level folders — a real, disclosed
// gap, not silently assumed correct.
func (u *projectUsecase) homeContainerChats(
	ctx context.Context,
	homeWorkspaceID string,
	folderID string,
) ([]homeContainerSibling, error) {
	if folderID != "" {
		all, err := u.homeFolders.ListChats(ctx)
		if err != nil {
			return nil, fmt.Errorf("project: reorder repos: list chats: %w", err)
		}
		out := make([]homeContainerSibling, 0, len(all))
		for _, c := range all {
			if c.ParentID == folderID {
				out = append(out, homeContainerSibling{id: c.ID, order: c.Order})
			}
		}
		return out, nil
	}
	if homeWorkspaceID == "" {
		return nil, nil
	}
	chats, err := u.homeFolders.ListByWorkspace(ctx, homeWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("project: reorder repos: list home chats: %w", err)
	}
	out := make([]homeContainerSibling, 0, len(chats))
	for _, c := range chats {
		// ParentID "" is the panel root; the home workspace's own owning
		// BRANCH chat is excluded — it draws no row of its own (rows-from-
		// home.ts's identical exclusion), so it is not a sibling to displace.
		if c.ParentID == "" && c.Type != domain.ChatTypeBranch {
			out = append(out, homeContainerSibling{id: c.ID, order: c.Order})
		}
	}
	return out, nil
}

// placeRepoAmongHomeSiblings gives subject exactly the Order the caller asked
// for within (projectID, folderID), shifting whatever ELSE shares that
// container — other repos AND home chats/folders alike — out of its way.
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
// This is now a merge of a Node-backed kind (repos, via Node.ListByParent)
// with a Chat-backed kind (home chats/folders, via homeContainerChats) rather
// than the two Repository/Chat reads the interim fix used — home chats/
// folders are not yet Node-backed (a later task in the plan), so that half of
// the merge is UNCHANGED. Reuses place()'s exact reinsert-and-renumber
// algorithm (ordering.go), unchanged — the only difference from densifyRepos
// is that home siblings are merged in alongside the repo siblings before
// place() ever runs, so its clamp and its dense renumbering are computed
// against the REAL sibling count, and every row that shifts is written back
// to whichever aggregate it actually belongs to (Node via SetOrder/
// SetPlacement, Chat via SetOrder).
//
// subject is included AUTHORITATIVELY (see densifyRepos's own doc for why) —
// UpdateRepo's pre-write read of the row being placed, not merely whatever
// Node.ListByParent(folderID) currently answers. When subject's own OLD
// parent differs from folderID it is reparenting as part of this call and its
// write is SetPlacement; every other row, subject included when its parent
// already matched, is SetOrder.
func (u *projectUsecase) placeRepoAmongHomeSiblings(
	ctx context.Context,
	projectID string,
	folderID string,
	subject domain.Node,
	target int,
) error {
	homeWorkspaceID := ""
	// A nil relocator (no cross-project move ever wired up for this
	// caller) or an unresolved home workspace still lets a project's repos
	// reorder among each other — homeContainerChats answers no chats for ""
	// either way, the same posture Get takes on an unresolved home elsewhere.
	if u.workspaces != nil {
		if ws, err := u.workspaces.GetHomeForProject(ctx, projectID); err == nil {
			homeWorkspaceID = ws.ID
		}
	}

	memberIDs, err := u.repoIDSet(ctx, projectID)
	if err != nil {
		return err
	}
	nodes, err := u.nodes.ListByParent(ctx, folderID)
	if err != nil {
		return fmt.Errorf("project: reorder repos: list nodes: %w", err)
	}
	chatSiblings, err := u.homeContainerChats(ctx, homeWorkspaceID, folderID)
	if err != nil {
		return err
	}
	slots, order := placementSlots(memberIDs, nodes, subject, chatSiblings)

	reparented := subject.ParentID != folderID
	written := make(map[string]bool, len(slots))
	for _, moved := range place(slots, subject.ID, &target) {
		sib := order[moved.at]
		written[sib.id] = true
		if sib.isRepo {
			reparenting := sib.id == subject.ID && reparented
			if err := u.writeRepoNode(ctx, sib.id, folderID, moved.order, reparenting); err != nil {
				return err
			}
			continue
		}
		if _, err := u.homeFolders.SetOrder(ctx, sib.id, moved.order); err != nil {
			return fmt.Errorf("project: reorder repos: save home row %s: %w", sib.id, err)
		}
	}
	if reparented {
		if err := u.forceReparentWrite(ctx, slots, subject.ID, folderID, &target, written); err != nil {
			return err
		}
	}
	return nil
}

// atID names one placementSlots row by its position in the parallel `order`
// slice: place() sorts its slots argument IN PLACE (slices.SortFunc mutates
// the backing array), so indexing back into slots itself after calling
// place() would read whatever the sort left at that position, not the row
// move.at actually named. A SEPARATE, untouched slice is how densifyRows'
// own []domain.Node avoids the same trap for a single-aggregate list; this is
// that same defence for a slot list holding two aggregates (repo Node rows
// and home chat/folder rows) at once.
type atID struct {
	id     string
	isRepo bool
}

// placementSlots builds placeRepoAmongHomeSiblings's merged slot list: every
// repo Node belonging to projectID and sitting in folderID (memberIDs,
// nodes), subject appended in its own place (authoritatively — see
// densifyRows' identical reasoning), and every home chat/folder sharing the
// same container (chatSiblings, UNCHANGED by this migration — see
// homeContainerChats' own doc).
func placementSlots(
	memberIDs map[string]bool,
	nodes []domain.Node,
	subject domain.Node,
	chatSiblings []homeContainerSibling,
) ([]slot, []atID) {
	order := make([]atID, 0, len(nodes)+len(chatSiblings)+1)
	slots := make([]slot, 0, len(nodes)+len(chatSiblings)+1)
	for _, n := range nodes {
		if !memberIDs[n.ID] || n.ID == subject.ID {
			continue // subject is added below, authoritatively
		}
		slots = append(slots, slot{at: len(slots), id: n.ID, order: n.Order})
		order = append(order, atID{id: n.ID, isRepo: true})
	}
	slots = append(slots, slot{at: len(slots), id: subject.ID, order: subject.Order})
	order = append(order, atID{id: subject.ID, isRepo: true})
	for _, c := range chatSiblings {
		slots = append(slots, slot{at: len(slots), id: c.id, order: c.order})
		order = append(order, atID{id: c.id, isRepo: false})
	}
	return slots, order
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
