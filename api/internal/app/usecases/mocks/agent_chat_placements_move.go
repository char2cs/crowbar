package mocks

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// MoveLedger is the move half of AgentChatPlacements: each chat's pending move, the
// moves done, and which chats are mid-turn. Every chat is in project "p1" unless
// Projects names another.
type MoveLedger struct {
	mu sync.Mutex
	// exec runs one move at a time: the fake's rows, unlike the real stores, are not
	// safe for concurrent writers.
	exec         sync.Mutex
	Pending      map[string]ledger.Move
	Done         map[string][]ledger.Move
	Busy         map[string]bool
	Projects     map[string]string // workspace id -> project id
	BranchOuts   []string
	BranchOutErr error
	// BranchNameErr refuses every name CheckBranchOutName is asked about.
	BranchNameErr error
	Discarded     []string
	// Finished is signalled with the chat id after each ExecuteMove that ran.
	Finished chan string
}

func (s *AgentChatPlacements) moveLedger() *MoveLedger {
	s.Moves.mu.Lock()
	if s.Moves.Pending == nil {
		s.Moves.Pending, s.Moves.Done = map[string]ledger.Move{}, map[string][]ledger.Move{}
	}
	s.Moves.mu.Unlock()
	return &s.Moves
}

func (s *AgentChatPlacements) RequestMove(_ context.Context, chatID string, m ledger.Move) (bool, error) {
	l := s.moveLedger()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Pending[chatID] = m
	return !l.Busy[chatID], nil
}

func (s *AgentChatPlacements) CancelMove(_ context.Context, chatID, _ string) error {
	l := s.moveLedger()
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.Pending, chatID)
	return nil
}

func (s *AgentChatPlacements) ExecuteMove(
	ctx context.Context, chatID string, apply func(context.Context, ledger.Move) (ledger.Move, error),
) (ledger.Move, bool, error) {
	l := s.moveLedger()
	l.mu.Lock()
	m, ok := l.Pending[chatID]
	busy := l.Busy[chatID]
	l.mu.Unlock()
	if !ok || busy {
		return ledger.Move{}, false, nil
	}
	l.exec.Lock()
	done, err := apply(ctx, m)
	l.exec.Unlock()
	if errors.Is(err, ledger.ErrMoveWaits) {
		return ledger.Move{}, false, nil
	}
	l.mu.Lock()
	delete(l.Pending, chatID)
	if err == nil {
		l.Done[chatID] = append(l.Done[chatID], done)
	}
	l.mu.Unlock()
	if l.Finished != nil {
		l.Finished <- chatID
	}
	return done, true, err
}

func (s *AgentChatPlacements) RunIn(_ context.Context, chatID, workspaceID string) error {
	for i := range s.Rows {
		if s.Rows[i].ID == chatID {
			s.Rows[i].WorkspaceID = workspaceID
			return nil
		}
	}
	return fmt.Errorf("run in: %s: not found", chatID)
}

func (s *AgentChatPlacements) MoveSite(_ context.Context, workspaceID string) (dir, projectID, repoID string, _ error) {
	l := s.moveLedger()
	l.mu.Lock()
	defer l.mu.Unlock()
	project := "p1"
	if p, ok := l.Projects[workspaceID]; ok {
		project = p
	}
	return "/dirs/" + workspaceID, project, "", nil
}

func (s *AgentChatPlacements) MovesWaiting(context.Context) ([]string, error) {
	l := s.moveLedger()
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for id := range l.Pending {
		if !l.Busy[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

// SetBusy marks chatID mid-turn or idle.
func (s *AgentChatPlacements) SetBusy(chatID string, busy bool) {
	l := s.moveLedger()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.Busy == nil {
		l.Busy = map[string]bool{}
	}
	l.Busy[chatID] = busy
}

// BranchOuts records each BranchOutWorkspace call's source workspace; BranchOutErr fails it,
// and Discarded records what DiscardWorkspace removed.
func (s *AgentChatPlacements) RequestBranchOut(ctx context.Context, chatID, workspaceID, branch, by string) (bool, error) {
	dir, _, _, err := s.MoveSite(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	return s.RequestMove(ctx, chatID, ledger.Move{
		RequestID: "branch-" + chatID, BranchOut: true, Branch: branch, From: workspaceID, FromDir: dir, By: by,
	})
}

func (s *AgentChatPlacements) CheckBranchOutName(context.Context, string, string) error {
	l := s.moveLedger()
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.BranchNameErr
}

func (s *AgentChatPlacements) BranchOutWorkspace(_ context.Context, workspaceID, branch string) (domain.Workspace, error) {
	l := s.moveLedger()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.BranchOuts = append(l.BranchOuts, workspaceID)
	if l.BranchOutErr != nil {
		return domain.Workspace{}, l.BranchOutErr
	}
	return domain.Workspace{ID: "ws-cut", Branch: branch, ForkPointSha: "abc1234", WorktreePath: "/dirs/ws-cut"}, nil
}

func (s *AgentChatPlacements) DiscardWorkspace(_ context.Context, workspaceID string) error {
	l := s.moveLedger()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Discarded = append(l.Discarded, workspaceID)
	return nil
}
