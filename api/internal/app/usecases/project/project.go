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
// workspace under it — see WorkspaceRelocator. FolderID re-files the repo's
// own entry within its project's home tree — see HomeFolders.
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
// (placeRepoInHomeContainer) can scope a ROOT-level container to THIS
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
// ListByWorkspace/ListChats/SetOrder are placeRepoInHomeContainer's own
// surface: reading every chat/folder sharing a repo's exact container, and
// writing back whichever ones a repo's own move displaces — see that
// function's doc for why a repo's Order cannot be densified in isolation
// from them. Satisfied structurally by the chat repository itself
// (repos.AgentChat), which already answers the full agentic Chats port
// (chat/internal/tree/types.go) this is a subset of.
type HomeFolders interface {
	Get(ctx context.Context, id string) (domain.Chat, error)
	ListByWorkspace(ctx context.Context, workspaceID string) ([]domain.Chat, error)
	ListChats(ctx context.Context) ([]domain.Chat, error)
	SetOrder(ctx context.Context, chatID string, order int) (domain.Chat, error)
}

type projectUsecase struct {
	projects    store.Store[domain.Project, string]
	repos       store.ScopedStore[domain.Repository, string]
	workspaces  WorkspaceRelocator
	homeFolders HomeFolders
}

// New builds a Usecase from the project and repository GORM stores, the
// workspace relocator a cross-project repo move needs, and the chat-tree read
// port a repo's own home-folder placement needs.
func New(
	projects store.Store[domain.Project, string],
	repos store.ScopedStore[domain.Repository, string],
	workspaces WorkspaceRelocator,
	homeFolders HomeFolders,
) Usecase {
	return &projectUsecase{
		projects:    projects,
		repos:       repos,
		workspaces:  workspaces,
		homeFolders: homeFolders,
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
	if in.Name != nil {
		repo.Name = *in.Name
		repo.AvatarLabel = avatar.Label(*in.Name)
		repo.AvatarColor = avatar.Color(*in.Name)
	}
	origin := repo.ProjectID
	originFolder := repo.FolderID
	if mErr := u.applyRepoProject(ctx, repo, in.ProjectID); mErr != nil {
		return domain.Repository{}, mErr
	}
	if in.FolderID != nil {
		if err := u.validateRepoFolder(ctx, *in.FolderID); err != nil {
			return domain.Repository{}, err
		}
		repo.FolderID = *in.FolderID
	}
	if err := u.repos.Save(ctx, *repo); err != nil {
		return domain.Repository{}, fmt.Errorf("project: update repo: save: %w", err)
	}
	// An explicit Order (a real drag) needs the CROSS-AGGREGATE placement —
	// see placeRepoInHomeContainer's own doc for why densifyRepos alone
	// clamps it wrong the instant a home chat/folder shares the container. A
	// move with no explicit order (e.g. a bare project/folder change from
	// some other caller) keeps the old repo-only resort, which merely
	// re-sorts by each row's own current (order, id) — nothing to place at a
	// specific position, so nothing needs the wider sibling view.
	if in.Order != nil {
		if err := u.placeRepoInHomeContainer(ctx, repo.ProjectID, repo.FolderID, repoID, *in.Order); err != nil {
			return domain.Repository{}, err
		}
	} else if err := u.densifyRepos(ctx, repo.ProjectID, repo.FolderID, repoID, nil); err != nil {
		return domain.Repository{}, err
	}
	// The container the repo LEFT — whether it moved project, folder, or both —
	// still has a gap where its row used to sit and needs closing. A plain
	// reorder within the same container is covered by the densify above; this
	// only fires for an actual move.
	if origin != repo.ProjectID || originFolder != repo.FolderID {
		if err := u.densifyRepos(ctx, origin, originFolder, "", nil); err != nil {
			return domain.Repository{}, err
		}
	}
	updated, err := u.repos.FindByKey(ctx, repoID)
	if err != nil || updated == nil {
		return *repo, nil
	}
	return *updated, nil
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

// densifyRepos renumbers one project's repo list 0..n-1, optionally placing
// repoID at target first, and writes back only the rows that moved.
//
// Scoped to folderID, not the whole project: a repo's Order is only dense
// among its OWN siblings — every other repo filed under that same
// project-home folder (or, for "", every other root-level repo) — mirroring
// how a workspace's Order densifies within its own folder rather than across
// a whole repo. FindWhere is queried by ProjectID alone and filtered here in
// Go rather than adding FolderID to the struct match: GORM's struct-as-where
// silently DROPS a zero-value field (the empty-string "root" case), which
// would have folded every folder's repos into one densify pass.
func (u *projectUsecase) densifyRepos(
	ctx context.Context,
	projectID string,
	folderID string,
	repoID string,
	target *int,
) error {
	all, err := u.repos.FindWhere(ctx, domain.Repository{ProjectID: projectID})
	if err != nil {
		return fmt.Errorf("project: reorder repos: list: %w", err)
	}
	rows := make([]domain.Repository, 0, len(all))
	for _, row := range all {
		if row.FolderID == folderID {
			rows = append(rows, row)
		}
	}
	for _, moved := range place(repoIndex(rows), repoID, target) {
		rows[moved.at].Order = moved.order
		if err := u.repos.Save(ctx, rows[moved.at]); err != nil {
			return fmt.Errorf("project: reorder repos: save %s: %w", rows[moved.at].ID, err)
		}
	}
	return nil
}

// homeContainerSibling is one non-repo row sharing a repo's exact
// project-home container — a chat or a folder, {@link placeRepoInHomeContainer}
// only ever needs its id, kind and current Order to seat a repo among them.
type homeContainerSibling struct {
	id    string
	order int
}

// homeContainerChats answers every chat/folder sharing ONE project-home
// container — the repo-agnostic half of {@link placeRepoInHomeContainer}'s
// sibling space.
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

// placeRepoInHomeContainer gives repoID exactly the Order the caller asked
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
// Reuses place()'s exact reinsert-and-renumber algorithm (ordering.go),
// unchanged — the ONLY difference from densifyRepos is what slot list gets
// built: home chats/folders sharing the container are merged in alongside
// the other repos before place() ever runs, so its clamp and its dense
// renumbering are computed against the REAL sibling count, and every
// row that shifts is written back to whichever aggregate it actually
// belongs to (Repository via GORM Save, Chat via SetOrder).
func (u *projectUsecase) placeRepoInHomeContainer(
	ctx context.Context,
	projectID string,
	folderID string,
	repoID string,
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

	repoRows, err := u.repos.FindWhere(ctx, domain.Repository{ProjectID: projectID})
	if err != nil {
		return fmt.Errorf("project: reorder repos: list: %w", err)
	}
	chatSiblings, err := u.homeContainerChats(ctx, homeWorkspaceID, folderID)
	if err != nil {
		return err
	}

	// atID is a SEPARATE slice from `slots`, deliberately: place() sorts its
	// slots argument IN PLACE (slices.SortFunc mutates the backing array),
	// so indexing back into `slots` itself after calling place() would read
	// whatever the sort left at that position, not the row `move.at`
	// actually named. densifyRepos avoids this the same way — indexing
	// `rows` ([]domain.Repository), never the []slot it hands to place() —
	// this mirrors that for a slot list holding two aggregates instead of
	// one.
	type atID struct {
		id     string
		isRepo bool
	}
	order := make([]atID, 0, len(repoRows)+len(chatSiblings))
	slots := make([]slot, 0, len(repoRows)+len(chatSiblings))
	for _, r := range repoRows {
		if r.FolderID != folderID {
			continue
		}
		slots = append(slots, slot{at: len(slots), id: r.ID, order: r.Order})
		order = append(order, atID{id: r.ID, isRepo: true})
	}
	for _, c := range chatSiblings {
		slots = append(slots, slot{at: len(slots), id: c.id, order: c.order})
		order = append(order, atID{id: c.id, isRepo: false})
	}

	byID := make(map[string]domain.Repository, len(repoRows))
	for _, r := range repoRows {
		byID[r.ID] = r
	}

	for _, moved := range place(slots, repoID, &target) {
		sib := order[moved.at]
		if sib.isRepo {
			repoRow, ok := byID[sib.id]
			if !ok {
				continue
			}
			repoRow.Order = moved.order
			if err := u.repos.Save(ctx, repoRow); err != nil {
				return fmt.Errorf("project: reorder repos: save %s: %w", sib.id, err)
			}
			continue
		}
		if _, err := u.homeFolders.SetOrder(ctx, sib.id, moved.order); err != nil {
			return fmt.Errorf("project: reorder repos: save home row %s: %w", sib.id, err)
		}
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
