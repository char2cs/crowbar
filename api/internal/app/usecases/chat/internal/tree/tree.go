// Package tree owns the sidebar forest's organisation layer: the FOLDER rows a
// user files chats into, where each row hangs, and the dense sibling order
// every row kind SHARES — they interleave at every level and sort on one Order
// field within one ParentID.
//
// A folder is a plain `domain.Folder` row (identity: id, name, repo scope)
// paired with a `domain.Node` row (position) — home-scoped (RepoID == "")
// since 2026-09-08 sidebar-placement-unification Task 5, repo-scoped
// (RepoID != "") too since Task 8. It is never a Chat row: `ChatTypeFolder`
// dropped out of `domain.ChatType`'s closed taxonomy the moment this
// migration finished (see chat_type.go). A CHAT's placement is Node-backed
// the same way, for the same two scopes — its identity (title, model,
// conversation) stays exactly where it always was, on `domain.Chat`.
//
// Its two deletes are deliberately opposite, and the asymmetry is the whole
// domain rule. Deleting a FOLDER promotes what it held to the folder's own
// parent: a folder holds no conversation, so the chats outlive it. Deleting a
// CHAT-typed row takes its entire subtree: a thread exists to CONTINUE its
// parent — it reads that parent's turns — so leaving it behind would strand it
// reading a context that no longer exists.
//
// Nothing here reasons about processes. A chat's runner, its PTY and its ledger
// belong to the agent usecase; this one moves rows and, for the cascade, asks
// that usecase to erase each CHAT-typed row it has decided must go.
//
// Every operation reads its rows ONCE, plans the whole change in memory, and
// then writes only the rows that actually moved. That is not merely an
// optimisation: the chat read model is an asynchronous projection, so a
// re-read taken between a write and the renumber that follows it can still be
// serving the pre-write list. Planning from a single snapshot removes the race
// rather than papering over it with a barrier.
package tree

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type chatFolderUsecase struct {
	chats      Chats
	agent      Agent
	work       *inflight.Work
	workspaces WorkspaceGitStatus
	reaper     WorkspaceReaper
	holders    WorkspaceHolders
	// folders and nodes are the folder/chat placement surface every scope
	// goes through now — home-scoped (RepoID == "") since Task 5, repo-scoped
	// (RepoID != "") too since Task 8 — see Folders/Nodes' own docs
	// (home_ports.go).
	folders Folders
	nodes   Nodes
	// announceRepo tells a live client about a repo header row a densify
	// shifted as collateral — a repo's Node write has no hub projection of
	// its own, unlike a chat's. nil announces nothing.
	announceRepo RepoAnnouncer
}

// RepoAnnouncer announces one repo's DECIDED placement, for the composition
// root to fan out as a RepoDTO.
type RepoAnnouncer func(ctx context.Context, repoID, parentID string, order int)

// Option configures New beyond its required ports.
type Option func(*chatFolderUsecase)

// WithRepoAnnouncer wires the collateral repo announce — see RepoAnnouncer.
func WithRepoAnnouncer(fn RepoAnnouncer) Option {
	return func(u *chatFolderUsecase) { u.announceRepo = fn }
}

// New builds the tree usecase over the chat row repository and the agent
// usecase behind everything a chat is besides a row.
//
// The agent handle is required in both directions — this usecase decides which
// chats a delete takes and which chat a create is born under, and the agent
// usecase is the only thing that knows how to erase one, mint one, or start a
// CLI on one.
//
// work is the SAME in-flight tracker the agent usecase's own turn and runner
// components observe, not a second one: a move or delete refuses over the
// subtree it takes by asking it directly, so the answer can never lag behind
// what a hook just announced.
//
// workspaces is DeletePreview's seam onto the workspace layer; reaper is
// DeleteChat's, and it is REQUIRED rather than optional for the reason
// ChatTreeUsecase itself is: a delete wired without it
// would erase a chat and silently strand the worktree it owned, which is the
// bug this port exists to close. Making it a parameter puts that mis-wire in
// front of the compiler instead of in front of a user.
//
// holders is required for the same reason and guards the same verb from the
// opposite failure: the reaper alone will tear down whatever it is handed, so
// without the holder census DeleteChat cascades a worktree that surviving
// sibling chats are still working in. One port says how a workspace is torn
// down; the other says whether it may be.
// folders and nodes are the home-scoped folder/chat placement surface (see
// Folders/Nodes' own docs, types.go) — Task 5 of the 2026-09-08
// sidebar-placement-unification plan.
func New(
	chats Chats,
	agent Agent,
	work *inflight.Work,
	workspaces WorkspaceGitStatus,
	reaper WorkspaceReaper,
	holders WorkspaceHolders,
	folders Folders,
	nodes Nodes,
	opts ...Option,
) Usecase {
	u := &chatFolderUsecase{
		chats:      chats,
		agent:      agent,
		work:       work,
		workspaces: workspaces,
		reaper:     reaper,
		holders:    holders,
		folders:    folders,
		nodes:      nodes,
	}
	for _, opt := range opts {
		opt(u)
	}
	return u
}

// ListInRepo returns repoID's own folder rows — "" for project home, a real
// repo id otherwise, the SAME convention Folder.RepoID is stored under. Every
// folder is Folder/Node-backed now (2026-09-08 sidebar-placement-unification
// Task 5 for home, Task 8 for repo-scoped), so this is one plain filter over
// FindAll — no Chat-typed fallback remains: ChatTypeFolder stopped being
// mintable the moment this task's Create landed (see Step 4, chat_type.go).
func (u *chatFolderUsecase) ListInRepo(
	ctx context.Context,
	repoID string,
) ([]domain.Chat, error) {
	return u.listFolders(ctx, func(f domain.Folder) bool { return f.RepoID == repoID })
}

// ListInHome implements Usecase. A home folder written before HomeID
// existed is adopted on this read: by the home its rows already attribute it
// to, else by the first home that lists it — never by every home at once.
func (u *chatFolderUsecase) ListInHome(
	ctx context.Context,
	homeID string,
) ([]domain.Chat, error) {
	return u.listFolders(ctx, func(f domain.Folder) bool {
		if f.RepoID == "" && f.HomeID == "" {
			f = u.adoptHomeFolder(ctx, f, homeID)
		}
		return f.InHome(homeID)
	})
}

// adoptHomeFolder stamps a legacy home folder with the home it belongs to:
// the one its ancestors or members already name, or homeID when nothing does.
func (u *chatFolderUsecase) adoptHomeFolder(
	ctx context.Context,
	f domain.Folder,
	homeID string,
) domain.Folder {
	home := u.homeOfFolder(ctx, f.ID, homeID, map[string]bool{})
	if home == "" {
		home = homeID
	}
	f.HomeID = home
	if err := u.folders.Save(ctx, f); err != nil {
		slog.WarnContext(ctx, "agent chat folder: adopt legacy home folder",
			"folder_id", f.ID, "home_id", home, "err", err)
	}
	return f
}

// homeOfFolder derives a home folder's home from the rows around it: a
// chat above or beneath it names its workspace, a folder names its home, a
// repo beneath it belongs to homeID's project or not.
func (u *chatFolderUsecase) homeOfFolder(
	ctx context.Context,
	id string,
	homeID string,
	seen map[string]bool,
) string {
	seen[id] = true
	if n, err := u.nodes.GetNode(ctx, id); err == nil && n.ParentID != "" {
		if home := u.homeOfRow(ctx, n.ParentID, homeID, seen); home != "" {
			return home
		}
	}
	children, err := u.nodes.ListByParent(ctx, id)
	if err != nil {
		return ""
	}
	for _, child := range children {
		if home := u.homeOfRow(ctx, child.ID, homeID, seen); home != "" {
			return home
		}
	}
	return ""
}

func (u *chatFolderUsecase) homeOfRow(
	ctx context.Context,
	id string,
	homeID string,
	seen map[string]bool,
) string {
	if seen[id] {
		return ""
	}
	if c, err := u.chats.Get(ctx, id); err == nil {
		return c.WorkspaceID
	}
	if f, err := u.folders.FindByKey(ctx, id); err == nil && f != nil {
		if f.HomeID != "" {
			return f.HomeID
		}
		return u.homeOfFolder(ctx, f.ID, homeID, seen)
	}
	if u.repoMemberIDsForHome(ctx, homeID)[id] {
		return homeID
	}
	return ""
}

func (u *chatFolderUsecase) listFolders(
	ctx context.Context,
	keep func(domain.Folder) bool,
) ([]domain.Chat, error) {
	all, err := u.folders.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: list folders: %w", err)
	}
	out := make([]domain.Chat, 0, len(all))
	for _, f := range all {
		if !keep(f) {
			continue
		}
		row := homeFolderView(f, domain.Node{})
		if n, nerr := u.nodes.GetNode(ctx, f.ID); nerr == nil {
			row = homeFolderView(f, n)
		}
		out = append(out, row)
	}
	return out, nil
}

// FolderScope implements Usecase.
func (u *chatFolderUsecase) FolderScope(
	ctx context.Context,
	id string,
) (domain.Folder, error) {
	f, err := u.folders.FindByKey(ctx, id)
	if err != nil {
		return domain.Folder{}, fmt.Errorf("agent chat folder: %s: %w", id, err)
	}
	if f == nil {
		return domain.Folder{}, fmt.Errorf("agent chat folder: %s: %w", id, apperr.ErrNotFound)
	}
	return *f, nil
}

// Create mints a new folder, home-scoped (RepoID == "") or repo-scoped
// (RepoID != "") alike since Task 8: identity is a plain domain.Folder row,
// position is a domain.Node row, minted through the SAME snapshot/plan/
// persist densify every other Create call runs — checkFolderContainer, guard
// walks and the write dispatch are entirely unchanged, only WHERE a row's
// identity/position come from differs (see writeRow's home dispatch,
// plan.go). Prior to Task 8 a repo-scoped folder was still a ChatTypeFolder
// Chat row; that path is gone now that Step 4 (chat_type.go) drops
// ChatTypeFolder from the closed taxonomy entirely.
func (u *chatFolderUsecase) Create(
	ctx context.Context,
	in CreateInput,
) (domain.Chat, []domain.Chat, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	folder := domain.Folder{Name: name, RepoID: in.RepoID, HomeID: in.HomeID}
	snapshot, err := u.globalSnapshotIn(ctx, domain.Chat{}, u.scopeForFolder(ctx, folder))
	if err != nil {
		return domain.Chat{}, nil, err
	}
	if cErr := u.checkFolderContainer(ctx, snapshot, in.RepoID, in.ParentID); cErr != nil {
		return domain.Chat{}, nil, cErr
	}
	id := in.ID
	if id == "" {
		id = uuid.NewString()
	}
	folder.ID = id
	if err := u.folders.Save(ctx, folder); err != nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: create %s: %w", id, err)
	}
	target := snapshot.plan.NextSlot(snapshot.canonical(in.ParentID))
	snapshot.add(domain.Chat{
		ID:       id,
		Type:     domain.ChatTypeFolder,
		Title:    name,
		RepoID:   in.RepoID,
		ParentID: in.ParentID,
		Order:    target,
	})
	snapshot.homeIDs[id] = true
	snapshot.freshIDs[id] = true
	snapshot.plan.Reorder(snapshot.canonical(in.ParentID), id, target)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Chat{}, nil, u.discardFolder(ctx, id, err)
	}
	return *snapshot.placedRow(id), without(written, id), nil
}

// discardFolder takes a just-minted folder back out when the create failed
// after minting it, and hands back the failure that caused it. The Folder row
// is always taken back out (Save already ran), and the Node row is a
// best-effort Forget — persist may have failed before this row's own Create
// ever ran, and Forget on an unknown id is a tolerated no-op (mirrors
// project.go's importOneRepo rollback). The purge is best-effort and NEVER
// replaces the cause: the user is told what actually failed.
func (u *chatFolderUsecase) discardFolder(
	ctx context.Context,
	id string,
	cause error,
) error {
	if err := u.folders.Delete(ctx, id); err != nil {
		return fmt.Errorf("%w (and cleanup failed: %v)", cause, err)
	}
	if err := u.nodes.Forget(ctx, id); err != nil {
		return fmt.Errorf("%w (and node cleanup failed: %v)", cause, err)
	}
	return cause
}

// Rename resolves id through the Folders store — the only place a folder's
// identity lives now, home-scoped or repo-scoped alike — and refuses an id
// that does not name one with apperr.ErrNotFound (a chat id, or a genuinely
// unknown one).
func (u *chatFolderUsecase) Rename(
	ctx context.Context,
	id string,
	name string,
) (domain.Chat, error) {
	clean, err := cleanName(name)
	if err != nil {
		return domain.Chat{}, err
	}
	f, ferr := u.folders.FindByKey(ctx, id)
	if ferr != nil {
		return domain.Chat{}, fmt.Errorf("agent chat folder: rename %s: %w", id, ferr)
	}
	if f == nil {
		return domain.Chat{}, fmt.Errorf("agent chat folder: %s: %w", id, apperr.ErrNotFound)
	}
	f.Name = clean
	if err := u.folders.Save(ctx, *f); err != nil {
		return domain.Chat{}, fmt.Errorf("agent chat folder: rename %s: save: %w", f.ID, err)
	}
	n, err := u.nodes.GetNode(ctx, f.ID)
	if err != nil {
		return homeFolderView(*f, domain.Node{}), nil
	}
	return homeFolderView(*f, n), nil
}

// Move resolves id through the Folders store the same way Rename does, then
// runs the SAME globalSnapshotAround/checkFolderMove/guardNotWorking/
// replace/persist chain every scope shares — see mergeForest (home_forest.go)
// for how that snapshot ends up seeing this row (and its repo/chat/folder
// siblings, wherever they sit) correctly.
func (u *chatFolderUsecase) Move(
	ctx context.Context,
	id string,
	in MoveInput,
) (domain.Chat, []domain.Chat, error) {
	f, ferr := u.folders.FindByKey(ctx, id)
	if ferr != nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: move %s: %w", id, ferr)
	}
	if f == nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: %s: %w", id, apperr.ErrNotFound)
	}
	n, err := u.nodes.GetNode(ctx, f.ID)
	if err != nil {
		return domain.Chat{}, nil, fmt.Errorf("agent chat folder: move %s: node: %w", f.ID, err)
	}
	current := homeFolderView(*f, n)
	snapshot, err := u.globalSnapshotAround(ctx, current)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	destination := current.ParentID
	if in.ParentID != nil {
		destination = *in.ParentID
	}
	if err := u.ensureWorkspaceAnchor(ctx, destination); err != nil {
		return domain.Chat{}, nil, err
	}
	if mErr := u.checkFolderMove(ctx, snapshot, f.RepoID, f.ID, destination); mErr != nil {
		return domain.Chat{}, nil, mErr
	}
	if wErr := guardNotWorking(snapshot.subtreeIDs(f.ID), u.work); wErr != nil {
		return domain.Chat{}, nil, wErr
	}
	u.replace(snapshot, f.ID, current.ParentID, destination, in.Order, false)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	return *snapshot.placedRow(f.ID), without(written, f.ID), nil
}

// Delete resolves id through the Folders store the same way Rename/Move do.
// What f held is PROMOTED to f's own parent (never cascaded — folders hold no
// conversation, see the package doc), f's Folder row and Node row are both
// erased, and the level it sat in is closed up.
func (u *chatFolderUsecase) Delete(
	ctx context.Context,
	id string,
) ([]domain.Chat, error) {
	f, ferr := u.folders.FindByKey(ctx, id)
	if ferr != nil {
		return nil, fmt.Errorf("agent chat folder: delete %s: %w", id, ferr)
	}
	if f == nil {
		return nil, fmt.Errorf("agent chat folder: %s: %w", id, apperr.ErrNotFound)
	}
	n, err := u.nodes.GetNode(ctx, f.ID)
	if err != nil {
		return nil, fmt.Errorf("agent chat folder: delete %s: node: %w", f.ID, err)
	}
	current := homeFolderView(*f, n)
	snapshot, err := u.globalSnapshotAround(ctx, current)
	if err != nil {
		return nil, err
	}
	if wErr := guardNotWorking(snapshot.subtreeIDs(f.ID), u.work); wErr != nil {
		return nil, wErr
	}
	if err := u.folders.Delete(ctx, f.ID); err != nil {
		return nil, fmt.Errorf("agent chat folder: delete %s: %w", f.ID, err)
	}
	if err := u.nodes.Forget(ctx, f.ID); err != nil {
		return nil, fmt.Errorf("agent chat folder: delete %s: node: %w", f.ID, err)
	}
	snapshot.plan.Reparent(f.ID, snapshot.canonical(current.ParentID))
	snapshot.drop(f.ID)
	snapshot.plan.Reorder(snapshot.canonical(current.ParentID), "", -1)
	return u.persist(ctx, snapshot)
}
