package tree

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// ErrOwnerMoves refuses moving a branch's own thread: it is the branch, and a
// branch moves by reparenting its workspace.
var ErrOwnerMoves = problem.New(problem.Conflict, "A branch's own thread moves with its branch")

// moveSite is where a workspace's chats run: the resolved folder and what it belongs to.
type moveSite struct {
	Dir, ProjectID, RepoID string
}

func (u *chatFolderUsecase) siteOf(ctx context.Context, workspaceID string) (moveSite, error) {
	dir, projectID, repoID, err := u.agent.MoveSite(ctx, workspaceID)
	return moveSite{Dir: dir, ProjectID: projectID, RepoID: repoID}, err
}

// requestMove records a move of chatID, and of every thread under it still in workspaceID, to
// in.WorkspaceID; each runs at its own turn boundary. The row stays where it is until then.
func (u *chatFolderUsecase) requestMove(
	ctx context.Context, workspaceID, chatID string, in PlaceInput,
) (domain.Chat, []domain.Chat, error) {
	current, err := u.loadChat(ctx, workspaceID, chatID)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	if !current.IsChat() || current.OwnsWorkspace {
		return domain.Chat{}, nil, ErrOwnerMoves
	}
	destination := current.ParentID
	if in.ParentID != nil {
		destination = *in.ParentID
	}
	snapshot, err := u.workspaceSnapshotAround(ctx, workspaceID, current, destination)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	if err := u.checkMoveTarget(ctx, snapshot, chatID, in.WorkspaceID, destination); err != nil {
		return domain.Chat{}, nil, err
	}
	from, err := u.siteOf(ctx, workspaceID)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	repo, err := u.crossedInto(ctx, from, in.WorkspaceID)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	base := ledger.Move{To: in.WorkspaceID, From: workspaceID, FromDir: from.Dir, Label: in.Label, Repo: repo, By: "user"}
	for _, id := range snapshot.subtreeIDs(chatID) {
		row := snapshot.row(id)
		if row == nil || !row.IsChat() || row.WorkspaceID != workspaceID || row.OwnsWorkspace {
			continue
		}
		m := base
		m.RequestID, m.ParentID = uuid.NewString(), row.ParentID
		if id == chatID {
			m.ParentID, m.Order = destination, in.Order
		}
		if err := u.askMove(ctx, id, m); err != nil {
			return domain.Chat{}, nil, err
		}
	}
	return current, nil, nil
}

// checkMoveTarget refuses what no move could make: a drop into the chat's own subtree, a container
// the target workspace's chats cannot hang off, or a workspace with no folder to run in.
func (u *chatFolderUsecase) checkMoveTarget(
	ctx context.Context, snapshot *treeSnapshot, chatID, workspaceID, destination string,
) error {
	if destination == chatID || snapshot.plan.Reaches(snapshot.canonical(destination), chatID) {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", chatID, destination, ErrCycle)
	}
	if err := u.checkChatContainer(ctx, snapshot, workspaceID, destination, false); err != nil {
		return err
	}
	ok, err := u.workspaces.Provisioned(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("agent chat folder: move target %s: %w", workspaceID, err)
	}
	if !ok {
		return fmt.Errorf("agent chat folder: move target %s has no folder yet: %w", workspaceID, problem.ErrConflict)
	}
	return nil
}

// crossedInto names the repository workspaceID is in when it is another project than from's, "" otherwise.
func (u *chatFolderUsecase) crossedInto(ctx context.Context, from moveSite, workspaceID string) (string, error) {
	to, err := u.siteOf(ctx, workspaceID)
	if err != nil || to.ProjectID == from.ProjectID {
		return "", err
	}
	if to.RepoID == "" {
		return "the project home", nil
	}
	root, err := u.workspaces.DefaultWorkspaceOf(ctx, to.RepoID)
	if err != nil {
		return "", fmt.Errorf("agent chat folder: move: repository of %s: %w", workspaceID, err)
	}
	site, err := u.siteOf(ctx, root)
	if err != nil {
		return "", err
	}
	return filepath.Base(site.Dir), nil
}

func (u *chatFolderUsecase) askMove(ctx context.Context, chatID string, m ledger.Move) error {
	ready, err := u.agent.RequestMove(ctx, chatID, m)
	if err != nil {
		return fmt.Errorf("agent chat folder: move %s: %w", chatID, err)
	}
	if ready {
		u.moves.kick(context.WithoutCancel(ctx), chatID, u.runMove)
	}
	return nil
}

// MoveWhenReady runs chatID's move if its turn boundary has come; the ledger's subscriber calls it
// after every append, so a burst of rows starts one run.
func (u *chatFolderUsecase) MoveWhenReady(ctx context.Context, chatID string) {
	u.moves.kick(context.WithoutCancel(ctx), chatID, u.runMove)
}

// ResumeMoves runs every move a previous daemon left asked for and not finished.
func (u *chatFolderUsecase) ResumeMoves(ctx context.Context) error {
	ids, err := u.agent.MovesWaiting(ctx)
	if err != nil {
		return fmt.Errorf("agent chat folder: resume moves: %w", err)
	}
	for _, id := range ids {
		u.MoveWhenReady(ctx, id)
	}
	return nil
}

// runMove makes chatID's move under its lock, then records the thread it now reads, which needs the lock.
func (u *chatFolderUsecase) runMove(ctx context.Context, chatID string) {
	var inherited, lineage []string
	var left string
	done, ran, err := u.agent.ExecuteMove(ctx, chatID, func(ctx context.Context, m ledger.Move) (ledger.Move, error) {
		var err error
		left = m.From
		if m.BranchOut {
			return u.applyBranchOut(ctx, chatID, m)
		}
		inherited, lineage, err = u.applyMove(ctx, chatID, m)
		if err != nil {
			return ledger.Move{}, err
		}
		to, err := u.siteOf(ctx, m.To)
		return ledger.Move{From: m.From, To: m.To, FromDir: m.FromDir, ToDir: to.Dir, Repo: m.Repo,
			ParentID: m.ParentID, By: m.By}, err
	})
	if err != nil {
		slog.WarnContext(ctx, "agent chat folder: move failed", "chat_id", chatID, "err", err)
		return
	}
	if !ran {
		return
	}
	u.noteNewAncestors(ctx, chatID, inherited, lineage)
	stayed, err := u.chats.ListByWorkspace(ctx, left)
	if err != nil {
		slog.WarnContext(ctx, "agent chat folder: move: threads under the moved chat", "chat_id", chatID, "err", err)
		return
	}
	for _, c := range stayed {
		if c.ParentID != chatID {
			continue
		}
		if !done.BranchOut {
			u.MoveWhenReady(ctx, c.ID)
			continue
		}
		follow := ledger.Move{RequestID: uuid.NewString(), To: done.To, From: left, FromDir: done.FromDir,
			ParentID: chatID, Label: done.Branch, By: done.By}
		if err := u.askMove(ctx, c.ID, follow); err != nil {
			slog.WarnContext(ctx, "agent chat folder: branch out: a thread under it", "chat_id", c.ID, "err", err)
		}
	}
}

// applyMove points chatID at m.To and files it under m.ParentID there; a failed placement points it back.
func (u *chatFolderUsecase) applyMove(ctx context.Context, chatID string, m ledger.Move) ([]string, []string, error) {
	current, err := u.chats.LoadChat(ctx, chatID)
	if err != nil {
		return nil, nil, fmt.Errorf("agent chat folder: move %s: %w", chatID, err)
	}
	inherited, err := u.lineageOf(ctx, current)
	if err != nil {
		return nil, nil, err
	}
	if parent, err := u.chats.LoadChat(ctx, m.ParentID); err == nil && parent.IsChat() && parent.WorkspaceID != m.To {
		return nil, nil, ledger.ErrMoveWaits
	}
	if err := u.agent.RunIn(ctx, chatID, m.To); err != nil {
		return nil, nil, fmt.Errorf("agent chat folder: move %s: %w", chatID, err)
	}
	parent := m.ParentID
	if _, _, err := u.placeChat(ctx, m.To, chatID, PlaceInput{ParentID: &parent, Order: m.Order},
		placeMode{rebind: true}); err != nil {
		return nil, nil, errors.Join(err, u.agent.RunIn(ctx, chatID, current.WorkspaceID))
	}
	moved, err := u.chats.LoadChat(ctx, chatID)
	if err != nil {
		return inherited, nil, nil
	}
	lineage, _ := u.lineageOf(ctx, moved)
	return inherited, lineage, nil
}

func (u *chatFolderUsecase) lineageOf(ctx context.Context, chat domain.Chat) ([]string, error) {
	snapshot, err := u.workspaceSnapshotAround(ctx, chat.WorkspaceID, chat, chat.ParentID)
	if err != nil {
		return nil, err
	}
	return snapshot.chatLineage(chat.ID), nil
}

// CancelMove withdraws chatID's pending move.
func (u *chatFolderUsecase) CancelMove(ctx context.Context, chatID string) error {
	if err := u.agent.CancelMove(ctx, chatID, "cancelled"); err != nil {
		return fmt.Errorf("agent chat folder: cancel move: %w", err)
	}
	return nil
}

// moveRuns runs at most one move pass per chat; a kick during a pass runs one more after it, so
// a boundary reached while a pass was deciding is never missed.
type moveRuns struct {
	mu    sync.Mutex
	state map[string]bool // chat id -> another pass owed
}

func (r *moveRuns) kick(ctx context.Context, chatID string, run func(context.Context, string)) {
	r.mu.Lock()
	if r.state == nil {
		r.state = map[string]bool{}
	}
	if _, running := r.state[chatID]; running {
		r.state[chatID] = true
		r.mu.Unlock()
		return
	}
	r.state[chatID] = false
	r.mu.Unlock()
	go func() {
		for {
			run(ctx, chatID)
			r.mu.Lock()
			if !r.state[chatID] {
				delete(r.state, chatID)
				r.mu.Unlock()
				return
			}
			r.state[chatID] = false
			r.mu.Unlock()
		}
	}()
}
