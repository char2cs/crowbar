// Package agent hosts the agentic-chat usecase: it owns AgentChat/AgentSegment
// lifecycle, spawns vendor CLIs in a PTY, and runs the context-move reducer
// against incoming hooks, persisting outcomes and appending the ledger.
package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/ledger"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
)

// TerminalCommander is the terminal-engine seam the usecase spawns vendor CLIs
// through and tears them down with.
type TerminalCommander interface {
	CreateCommand(
		ctx context.Context,
		workspaceID string,
		cwd string,
		argv []string,
		env []string,
	) (string, error)
	Kill(
		ctx context.Context,
		sessionID string,
	) error
}

// Broadcaster is the hub seam the usecase pushes agent-chat lifecycle events
// through.
type Broadcaster interface {
	BroadcastAgentChat(chatID, kind string)
}

// WorkspaceReader resolves a workspace's Crowbar-managed identity and git
// worktree directory.
type WorkspaceReader interface {
	WorktreeDir(
		ctx context.Context,
		workspaceID string,
	) (crowbarHome, projectID, repoID, worktree string, err error)
}

// Usecase is the agentic-chat engine: spawning vendor CLI segments, ingesting
// their hooks through the context-move reducer, and persisting the result.
type Usecase struct {
	repo     agentchat.Store
	registry *engineagent.Registry
	term     TerminalCommander
	bc       Broadcaster
	ws       WorkspaceReader
}

// New builds a Usecase from its repository, reducer registry, and seams.
func New(
	repo agentchat.Store,
	registry *engineagent.Registry,
	term TerminalCommander,
	bc Broadcaster,
	ws WorkspaceReader,
) *Usecase {
	return &Usecase{
		repo:     repo,
		registry: registry,
		term:     term,
		bc:       bc,
		ws:       ws,
	}
}

// SpawnChat creates a fresh AgentChat and its first AgentSegment, launching the
// provider's vendor CLI in a PTY. The returned segID is also the segment's
// CrowbarSegmentID and terminal session id.
func (u *Usecase) SpawnChat(
	ctx context.Context,
	workspaceID string,
	providerID string,
) (chatID, segID string, err error) {
	chatID = uuid.NewString()
	chat := domain.AgentChat{
		ID:          chatID,
		WorkspaceID: workspaceID,
		CreatedAt:   time.Now(),
	}
	if err := u.repo.SaveChat(ctx, chat); err != nil {
		return "", "", fmt.Errorf("agent: spawn chat: save chat: %w", err)
	}

	segID, err = u.spawnSegment(ctx, chat, providerID, nil, "")
	if err != nil {
		return "", "", err
	}
	return chatID, segID, nil
}

// spawnSegment is the single owner of AgentChat.ActiveSegmentID: it persists a
// new active AgentSegment, binds the reducer, builds and launches the
// provider's spawn plan, and stamps the segment's terminal session id onto
// both the segment and the chat. Both SpawnChat and SwitchProvider go through
// it so ActiveSegmentID is never left unset.
func (u *Usecase) spawnSegment(
	ctx context.Context,
	chat domain.AgentChat,
	providerID string,
	extraSteps []engineagent.InjectStep,
	handoff string,
) (string, error) {
	segID := uuid.NewString()
	seg := domain.AgentSegment{
		ID:               segID,
		ChatID:           chat.ID,
		ProviderID:       providerID,
		CrowbarSegmentID: segID,
		Status:           "active",
		StartedAt:        time.Now(),
	}
	if err := u.repo.SaveSegment(ctx, seg); err != nil {
		return "", fmt.Errorf("agent: spawn segment: save segment: %w", err)
	}
	u.registry.BindSegment(segID, chat.ID)

	crowbarHome, _, _, worktree, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: spawn segment: worktree dir: %w", err)
	}

	descriptor, err := engineagent.ResolveDescriptor(crowbarHome, providerID)
	if err != nil {
		return "", fmt.Errorf("agent: spawn segment: resolve descriptor: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "crowbar-agent-")
	if err != nil {
		return "", fmt.Errorf("agent: spawn segment: mkdtemp: %w", err)
	}

	tctx := engineagent.TemplateCtx{
		Tmp:         tmpDir,
		Cwd:         worktree,
		CrowbarHook: u.crowbarHookPath(crowbarHome),
		Handoff:     handoff,
	}
	plan, err := engineagent.BuildSpawnPlan(descriptor, tctx, os.Environ(), extraSteps)
	if err != nil {
		return "", fmt.Errorf("agent: spawn segment: build spawn plan: %w", err)
	}

	argv := append([]string{descriptor.Spawn.Cmd}, plan.Argv...)
	env := append(plan.Env, "CROWBAR_SEGMENT_ID="+segID)

	termSessID, err := u.term.CreateCommand(ctx, chat.WorkspaceID, worktree, argv, env)
	if err != nil {
		return "", fmt.Errorf("agent: spawn segment: create command: %w", err)
	}

	seg.TerminalSessionID = termSessID
	if err := u.repo.SaveSegment(ctx, seg); err != nil {
		return "", fmt.Errorf("agent: spawn segment: save terminal session id: %w", err)
	}

	chat.ActiveSegmentID = segID
	if err := u.repo.SaveChat(ctx, chat); err != nil {
		return "", fmt.Errorf("agent: spawn segment: save chat active segment: %w", err)
	}
	return segID, nil
}

func (u *Usecase) crowbarHookPath(home string) string {
	if v := os.Getenv("CROWBAR_HOOK_BIN"); v != "" {
		return v
	}
	return filepath.Join(home, "bin", "crowbar")
}

// IngestHook maps an incoming vendor hook to a canonical event, runs the
// context-move reducer on session_start (persisting the outcome and emitting a
// WS event), and appends the transcript to the chat's ledger on turn_stop. An
// unknown crowbarSegID (no active segment) is ignored, not an error.
func (u *Usecase) IngestHook(
	ctx context.Context,
	crowbarSegID string,
	canonicalEvent string,
	payload map[string]any,
) error {
	seg, err := u.repo.GetActiveSegmentByCrowbarID(ctx, crowbarSegID)
	if errors.Is(err, agentchat.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("agent: ingest hook: active segment: %w", err)
	}

	chat, err := u.repo.GetChat(ctx, seg.ChatID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: chat: %w", err)
	}

	crowbarHome, projectID, repoID, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: worktree dir: %w", err)
	}

	descriptor, err := engineagent.ResolveDescriptor(crowbarHome, seg.ProviderID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: resolve descriptor: %w", err)
	}

	ev, _ := descriptor.MapHook(canonicalEvent, payload)

	switch ev.Kind {
	case "session_start":
		return u.handleSessionStart(ctx, crowbarSegID, seg, chat, ev)
	case "turn_stop":
		return u.handleTurnStop(ctx, seg, chat, crowbarHome, projectID, repoID, ev)
	}
	return nil
}

func (u *Usecase) handleSessionStart(
	ctx context.Context,
	crowbarSegID string,
	seg domain.AgentSegment,
	chat domain.AgentChat,
	ev engineagent.CanonicalEvent,
) error {
	out := u.registry.OnSessionStart(crowbarSegID, ev.SessionID, uuid.NewString)

	var err error
	switch out.Kind {
	case "bound":
		err = u.persistBound(ctx, seg, ev)
	case "registered":
		err = u.persistRegistered(ctx, crowbarSegID, seg, chat, out, ev)
	case "focus":
		err = u.persistFocus(ctx, crowbarSegID, seg, out, ev)
	}
	if err != nil {
		return err
	}

	u.bc.BroadcastAgentChat(out.ChatID, out.Kind)
	return nil
}

func (u *Usecase) persistBound(
	ctx context.Context,
	seg domain.AgentSegment,
	ev engineagent.CanonicalEvent,
) error {
	if seg.ProviderSessionID == "" {
		seg.ProviderSessionID = ev.SessionID
	}
	seg.TranscriptPath = ev.Transcript
	if err := u.repo.SaveSegment(ctx, seg); err != nil {
		return fmt.Errorf("agent: ingest hook: bound: save segment: %w", err)
	}
	return nil
}

func (u *Usecase) persistRegistered(
	ctx context.Context,
	crowbarSegID string,
	oldSeg domain.AgentSegment,
	priorChat domain.AgentChat,
	out engineagent.Outcome,
	ev engineagent.CanonicalEvent,
) error {
	now := time.Now()
	oldSeg.Status = "moved"
	oldSeg.EndedAt = &now
	if err := u.repo.SaveSegment(ctx, oldSeg); err != nil {
		return fmt.Errorf("agent: ingest hook: registered: save old segment: %w", err)
	}

	newSeg := domain.AgentSegment{
		ID:                uuid.NewString(),
		ChatID:            out.ChatID,
		ProviderID:        oldSeg.ProviderID,
		CrowbarSegmentID:  crowbarSegID,
		ProviderSessionID: ev.SessionID,
		TranscriptPath:    ev.Transcript,
		TerminalSessionID: oldSeg.TerminalSessionID,
		Status:            "active",
		StartedAt:         now,
	}
	if err := u.repo.SaveSegment(ctx, newSeg); err != nil {
		return fmt.Errorf("agent: ingest hook: registered: save new segment: %w", err)
	}

	newChat := domain.AgentChat{
		ID:              out.ChatID,
		WorkspaceID:     priorChat.WorkspaceID,
		CreatedAt:       now,
		ActiveSegmentID: newSeg.ID,
	}
	if err := u.repo.SaveChat(ctx, newChat); err != nil {
		return fmt.Errorf("agent: ingest hook: registered: save new chat: %w", err)
	}
	return nil
}

func (u *Usecase) persistFocus(
	ctx context.Context,
	crowbarSegID string,
	oldSeg domain.AgentSegment,
	out engineagent.Outcome,
	ev engineagent.CanonicalEvent,
) error {
	oldSeg.Status = "moved"
	if err := u.repo.SaveSegment(ctx, oldSeg); err != nil {
		return fmt.Errorf("agent: ingest hook: focus: save old segment: %w", err)
	}

	newSeg := domain.AgentSegment{
		ID:                uuid.NewString(),
		ChatID:            out.ChatID,
		ProviderID:        oldSeg.ProviderID,
		CrowbarSegmentID:  crowbarSegID,
		ProviderSessionID: ev.SessionID,
		TerminalSessionID: oldSeg.TerminalSessionID,
		Status:            "active",
		StartedAt:         time.Now(),
	}
	if err := u.repo.SaveSegment(ctx, newSeg); err != nil {
		return fmt.Errorf("agent: ingest hook: focus: save new segment: %w", err)
	}

	focusedChat, err := u.repo.GetChat(ctx, out.ChatID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: focus: load chat: %w", err)
	}
	focusedChat.ActiveSegmentID = newSeg.ID
	if err := u.repo.SaveChat(ctx, focusedChat); err != nil {
		return fmt.Errorf("agent: ingest hook: focus: save chat: %w", err)
	}
	return nil
}

func (u *Usecase) handleTurnStop(
	ctx context.Context,
	seg domain.AgentSegment,
	chat domain.AgentChat,
	crowbarHome, projectID, repoID string,
	ev engineagent.CanonicalEvent,
) error {
	blob, err := os.ReadFile(ev.Transcript)
	if err != nil {
		return nil
	}

	dir := worktreepath.AgentLedgerDir(crowbarHome, projectID, repoID, chat.WorkspaceID, chat.ID)
	led, err := ledger.Open(dir)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: turn stop: ledger open: %w", err)
	}
	if _, err := led.Append(seg.ProviderID, time.Now(), blob); err != nil {
		return fmt.Errorf("agent: ingest hook: turn stop: ledger append: %w", err)
	}

	u.bc.BroadcastAgentChat(chat.ID, "turn_stopped")
	return nil
}

// ListChats returns every persisted AgentChat.
func (u *Usecase) ListChats(
	ctx context.Context,
) ([]domain.AgentChat, error) {
	return u.repo.ListChats(ctx)
}

// GetChat returns a single AgentChat by id.
func (u *Usecase) GetChat(
	ctx context.Context,
	id string,
) (domain.AgentChat, error) {
	return u.repo.GetChat(ctx, id)
}

// SegmentsFor returns every AgentSegment belonging to a chat, oldest first.
func (u *Usecase) SegmentsFor(
	ctx context.Context,
	chatID string,
) ([]domain.AgentSegment, error) {
	return u.repo.ListSegmentsByChat(ctx, chatID)
}

// SeedRegistry rehydrates the reducer's known-session index from persisted
// segments at startup, so a resumed process that /resumes into a
// pre-restart chat is recognized as "focus" rather than "registered".
func (u *Usecase) SeedRegistry(
	ctx context.Context,
) error {
	segs, err := u.repo.AllSegments(ctx)
	if err != nil {
		return fmt.Errorf("agent: seed registry: all segments: %w", err)
	}
	for _, seg := range segs {
		if seg.ProviderSessionID == "" {
			continue
		}
		u.registry.Seed(seg.ProviderSessionID, seg.ChatID)
	}
	return nil
}
