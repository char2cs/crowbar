// Package agent hosts the agentic-chat usecase: it owns AgentChat/AgentSegment
// lifecycle, spawns vendor CLIs in a PTY, and runs the context-move reducer
// against incoming hooks, persisting outcomes and appending the ledger.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/ledger"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
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
		onExit func(),
	) (string, error)
	// TerminateGraceful gracefully quits the outgoing CLI on a provider switch
	// (spec §8): a clean-exit SIGTERM, not SIGKILL — Claude flushes its native
	// transcript on a clean exit, and a hard kill can lose the outgoing CLI's
	// last pre-switch turn. Applies uniformly to every provider (Codex
	// tolerates it too); no provider branching here.
	TerminateGraceful(
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
	// segMu serializes IngestHook per crowbarSegID; see keyed_mutex.go for why
	// this exists (the read/reduce/persist sequence is not atomic on its own).
	segMu segmentMutex
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

	segID, err = u.spawnSegment(ctx, chat, providerID, nil, "", true)
	if err != nil {
		return "", "", err
	}
	return chatID, segID, nil
}

// RenameChat sets a chat's title under user>agent>derived precedence:
//
//	source "derived": set only if the title is currently empty (first-prompt fallback).
//	source "agent":   set unless the title is user-locked (agent may upgrade a derived title).
//	source "user"/"": set unconditionally AND lock (a manual rename wins and sticks).
//
// An empty title is always a no-op. Broadcasts "titled" on a successful change.
func (u *Usecase) RenameChat(
	ctx context.Context,
	chatID, title, source string,
) error {
	if title == "" {
		return nil
	}
	chat, err := u.repo.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: rename chat: get: %w", err)
	}
	switch source {
	case "derived":
		if chat.Title != "" {
			return nil
		}
	case "agent":
		if chat.TitleLocked {
			return nil
		}
	default: // "user" / "" — manual rename wins and locks
		chat.TitleLocked = true
	}
	chat.Title = title
	if err := u.repo.SaveChat(ctx, chat); err != nil {
		return fmt.Errorf("agent: rename chat: save: %w", err)
	}
	u.bc.BroadcastAgentChat(chatID, "titled")
	return nil
}

// deriveTitle turns a user prompt into a short chat title: the first non-empty
// line, trimmed, capped to 60 runes.
func deriveTitle(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		r := []rune(line)
		if len(r) > 60 {
			return strings.TrimSpace(string(r[:60])) + "…"
		}
		return line
	}
	return ""
}

// spawnSegment is the single owner of AgentChat.ActiveSegmentID: it persists a
// new active AgentSegment, binds the reducer, builds and launches the
// provider's spawn plan, and stamps the segment's terminal session id onto
// both the segment and the chat. Both SpawnChat and SwitchProvider go through
// it so ActiveSegmentID is never left unset. injectTitle is true only for a
// genuine fresh-chat spawn (SpawnChat): it injects the configured title
// instruction as a true system-prompt document via the descriptor's
// system_prompt_inject steps, instead of the (here empty) handoff.
func (u *Usecase) spawnSegment(
	ctx context.Context,
	chat domain.AgentChat,
	providerID string,
	extraSteps []engineagent.InjectStep,
	handoff string,
	injectTitle bool,
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

	// Home-scoped (not system-tmp) and keyed by segID so it is deterministic and
	// so sweepStaleAgentTmp can reliably find every leftover on daemon startup.
	// This dir holds the rendered hook config and, for codex, a COPY of
	// ~/.codex/auth.json (a credential) — it must survive for the whole life of
	// the spawned CLI, so it is removed via onExit below (on PTY session end),
	// never eagerly after spawn.
	tmpDir := filepath.Join(crowbarHome, "agent-tmp", segID)
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		return "", fmt.Errorf("agent: spawn segment: mkdir tmp: %w", err)
	}

	tctx := engineagent.TemplateCtx{
		Tmp:         tmpDir,
		Cwd:         worktree,
		CrowbarHook: u.crowbarHookPath(crowbarHome),
		Segid:       segID,
		Provider:    providerID,
		Chatid:      chat.ID,
	}
	steps := extraSteps
	if injectTitle {
		// Fresh chat: the injected document is the title instruction (from
		// config), delivered through the descriptor's system_prompt_inject
		// mechanism — a true per-invocation system prompt, NOT handoff_inject
		// (which for codex is a positional arg that would otherwise hijack its
		// initial user turn; see descriptor.go's SystemPromptInject doc).
		tctx.SystemPrompt = engineagent.Expand(config.GetPrompts().TitleInstruction, tctx)
		steps = append(steps, descriptor.SystemPromptInject...)
	} else {
		tctx.Handoff = handoff
	}
	plan, err := engineagent.BuildSpawnPlan(descriptor, tctx, os.Environ(), steps)
	if err != nil {
		return "", fmt.Errorf("agent: spawn segment: build spawn plan: %w", err)
	}

	argv := append([]string{descriptor.Spawn.Cmd}, plan.Argv...)

	termSessID, err := u.term.CreateCommand(ctx, chat.WorkspaceID, worktree, argv, plan.Env,
		func() { _ = os.RemoveAll(tmpDir) })
	if err != nil {
		// CreateCommand never got far enough to register onExit — clean up here
		// so a spawn failure doesn't leak the tmp dir until the next restart sweep.
		_ = os.RemoveAll(tmpDir)
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
// context-move reducer on session_start, and appends a conversation turn to the
// chat's ledger on user_prompt / turn_stop. An unknown crowbarSegID (no active
// segment) or a malformed payload is ignored, never an error — a hook must
// never break the vendor CLI's turn.
func (u *Usecase) IngestHook(
	ctx context.Context,
	crowbarSegID string,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	// Serialize the whole read -> reduce -> persist sequence per crowbarSegID
	// (see keyed_mutex.go). Unrelated segments proceed fully concurrently.
	u.segMu.Lock(crowbarSegID)
	defer u.segMu.Unlock(crowbarSegID)

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

	// The active segment is the source of truth for which provider spawned this
	// CLI. The hook's self-reported provider is only a guard against a
	// mis-authored descriptor.
	if provider != "" && provider != seg.ProviderID {
		slog.WarnContext(ctx, "agent: ingest hook: provider mismatch",
			"hook_provider", provider, "segment_provider", seg.ProviderID, "segment_id", crowbarSegID)
	}

	descriptor, err := engineagent.ResolveDescriptor(crowbarHome, seg.ProviderID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: resolve descriptor: %w", err)
	}

	payload, err := descriptor.ParsePayload(rawPayload)
	if err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: parse payload", "err", err, "segment_id", crowbarSegID)
		return nil
	}

	ev, _ := descriptor.MapHook(canonicalEvent, payload)

	switch ev.Kind {
	case "session_start":
		return u.handleSessionStart(ctx, crowbarSegID, seg, chat, ev)
	case "user_prompt":
		if err := u.RenameChat(ctx, chat.ID, deriveTitle(ev.Message), "derived"); err != nil {
			slog.WarnContext(ctx, "agent: ingest hook: derived title", "err", err, "chat_id", chat.ID)
		}
		return u.appendTurn(ctx, seg, chat, crowbarHome, projectID, repoID, "user", ev.Message)
	case "turn_stop":
		return u.appendTurn(ctx, seg, chat, crowbarHome, projectID, repoID, "assistant", ev.Message)
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

	// oldSeg just vacated priorChat (the process moved to a brand-new chat);
	// left alone, priorChat.ActiveSegmentID would keep pointing at a segment
	// that is now "moved", not active. Harmless in practice (callers resolve
	// "the active segment for a process" via GetActiveSegmentByCrowbarID,
	// never chat.ActiveSegmentID directly) but stale/misleading to inspect.
	if priorChat.ActiveSegmentID == oldSeg.ID {
		priorChat.ActiveSegmentID = ""
		if err := u.repo.SaveChat(ctx, priorChat); err != nil {
			return fmt.Errorf("agent: ingest hook: registered: clear vacated chat: %w", err)
		}
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

	// oldSeg was the active segment of ITS OWN chat, which may differ from the
	// chat we just focused into (that is the whole point of "focus": moving
	// into a DIFFERENT known chat). Clear that vacated chat's ActiveSegmentID
	// so it doesn't keep pointing at a now-"moved" segment. Guarded on
	// oldSeg.ChatID != out.ChatID so a same-chat edge case never clobbers the
	// focusedChat update just made above.
	if oldSeg.ChatID != out.ChatID {
		if err := u.clearVacatedChatActiveSegment(ctx, oldSeg.ChatID, oldSeg.ID); err != nil {
			return fmt.Errorf("agent: ingest hook: focus: clear vacated chat: %w", err)
		}
	}
	return nil
}

// clearVacatedChatActiveSegment nulls chatID's ActiveSegmentID when it still
// points at vacatedSegID (the segment a session_start move just carried the
// live process away from), so a chat's ActiveSegmentID never outlives the
// segment it names. A mismatch is left untouched (defensive: some other
// change may already have updated it).
func (u *Usecase) clearVacatedChatActiveSegment(
	ctx context.Context,
	chatID string,
	vacatedSegID string,
) error {
	c, err := u.repo.GetChat(ctx, chatID)
	if err != nil {
		return err
	}
	if c.ActiveSegmentID != vacatedSegID {
		return nil
	}
	c.ActiveSegmentID = ""
	return u.repo.SaveChat(ctx, c)
}

// appendTurn records one conversation turn (user or assistant) into the chat's
// ledger and broadcasts the lifecycle event. Empty text is a no-op.
func (u *Usecase) appendTurn(
	ctx context.Context,
	seg domain.AgentSegment,
	chat domain.AgentChat,
	crowbarHome, projectID, repoID string,
	role, text string,
) error {
	if text == "" {
		return nil
	}
	dir := worktreepath.AgentLedgerDir(crowbarHome, projectID, repoID, chat.WorkspaceID, chat.ID)
	led, err := ledger.Open(dir)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: ledger open: %w", err)
	}
	if _, err := led.AppendTurn(role, seg.ProviderID, time.Now(), text); err != nil {
		return fmt.Errorf("agent: ingest hook: ledger append: %w", err)
	}
	kind := "turn_stopped"
	if role == "user" {
		kind = "user_prompt"
	}
	u.bc.BroadcastAgentChat(chat.ID, kind)
	return nil
}

// AssembleHandoff resolves chatID's ledger directory, reads every entry, and
// wraps them in a legible preamble/footer so a freshly spawned provider CLI
// can be handed the prior context. Returns "" (not an error) when the ledger
// has no entries yet.
func (u *Usecase) AssembleHandoff(
	ctx context.Context,
	chatID string,
) (string, error) {
	chat, err := u.repo.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: assemble handoff: chat: %w", err)
	}

	crowbarHome, projectID, repoID, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: assemble handoff: worktree dir: %w", err)
	}

	dir := worktreepath.AgentLedgerDir(crowbarHome, projectID, repoID, chat.WorkspaceID, chat.ID)
	led, err := ledger.Open(dir)
	if err != nil {
		return "", fmt.Errorf("agent: assemble handoff: ledger open: %w", err)
	}

	blob, err := led.RenderConversation()
	if err != nil {
		return "", fmt.Errorf("agent: assemble handoff: ledger read all: %w", err)
	}
	if len(blob) == 0 {
		return "", nil
	}

	return strings.ReplaceAll(config.GetPrompts().HandoffWrapper, "{conversation}", string(blob)), nil
}

// SwitchProvider is the headline provider-switch: it terminates chatID's
// active provider CLI, assembles a handoff from the ledger, and spawns
// targetProviderID as a NEW segment in the SAME chat with the handoff
// injected. If a prior segment for targetProviderID already carries a native
// ProviderSessionID (a switch-back), the target CLI is also resumed into that
// session via the descriptor's session.resume; otherwise (a forward switch)
// it receives only the handoff. Reuses spawnSegment (Task 14), the single
// owner of ActiveSegmentID, so segment creation is never duplicated here.
func (u *Usecase) SwitchProvider(
	ctx context.Context,
	chatID string,
	targetProviderID string,
) (string, error) {
	chat, err := u.repo.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: chat: %w", err)
	}

	oldSeg, err := u.repo.GetSegment(ctx, chat.ActiveSegmentID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: active segment: %w", err)
	}

	// Read-before-terminate: the ledger is built from hooks (appended on each
	// turn_stop/user_prompt hook) and is already on disk, so assembling the
	// handoff does not depend on the outgoing CLI still being alive. A
	// failure here must abort the switch (return before terminate) rather
	// than silently proceed with an EMPTY handoff — nothing destructive has
	// happened yet, so aborting leaves the chat exactly as it was.
	handoff, err := u.AssembleHandoff(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: assemble handoff: %w", err)
	}

	// The outgoing CLI is quit gracefully (spec §8: clean-exit SIGTERM, never
	// SIGKILL) so a well-behaved CLI — Claude Code in particular — gets the
	// chance to flush its native transcript before it dies; a hard kill can
	// lose the outgoing CLI's last pre-switch turn. Applies uniformly to
	// every provider (Codex tolerates it too), no per-provider branching.
	//
	// A failed terminate is surfaced, not swallowed: if the outgoing CLI's
	// terminal session still exists but could not be terminated, proceeding
	// would spawn a SECOND live CLI into the same worktree while the DB marks
	// only the new one active — the exact "two live CLIs" hazard this guards
	// against. The one error TerminateGraceful can return today
	// (registry.ErrSessionNotFound, exported as terminal.ErrSessionNotFound)
	// means the session is already gone (process previously exited/reaped) —
	// safe, even correct, to continue: the alternative would trap a chat
	// unable to ever switch again once its terminal session ends on its own.
	if err := u.term.TerminateGraceful(ctx, oldSeg.TerminalSessionID); err != nil {
		if !errors.Is(err, engineterminal.ErrSessionNotFound) {
			return "", fmt.Errorf("agent: switch provider: terminate outgoing terminal: %w", err)
		}
		slog.WarnContext(ctx, "agent: switch provider: outgoing terminal session already gone before terminate; continuing switch",
			"chat_id", chatID, "segment_id", oldSeg.ID, "terminal_session_id", oldSeg.TerminalSessionID, "err", err)
	}
	now := time.Now()
	oldSeg.Status = "ended"
	oldSeg.EndedAt = &now
	if err := u.repo.SaveSegment(ctx, oldSeg); err != nil {
		return "", fmt.Errorf("agent: switch provider: save old segment: %w", err)
	}

	segs, err := u.repo.ListSegmentsByChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: list segments: %w", err)
	}
	// ListSegmentsByChat is ordered by started_at asc, so the last match
	// found while scanning forward is the most recent prior segment for the
	// target provider.
	var priorSessionID string
	for _, s := range segs {
		if s.ProviderID == targetProviderID && s.ProviderSessionID != "" {
			priorSessionID = s.ProviderSessionID
		}
	}

	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: worktree dir: %w", err)
	}
	d, err := engineagent.ResolveDescriptor(crowbarHome, targetProviderID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: resolve descriptor: %w", err)
	}

	// Resume arg must be split into separate argv tokens: exec.Command does
	// NOT split a string on whitespace, so a whole "--resume {id}" template
	// handed to a single pass_arg would become one literal argument.
	var resumeSteps []engineagent.InjectStep
	if priorSessionID != "" && d.Session.Resume != nil && d.Session.Resume.Arg != "" {
		resumeCtx := engineagent.TemplateCtx{ID: priorSessionID}
		for _, tok := range strings.Fields(engineagent.Expand(d.Session.Resume.Arg, resumeCtx)) {
			resumeSteps = append(resumeSteps, engineagent.InjectStep{
				Verb: "pass_arg",
				Args: map[string]any{"positional": tok},
			})
		}
	}
	// Resume goes first so codex's `resume <id>` subcommand precedes the
	// positional handoff; order is irrelevant for claude's flag pair.
	extraSteps := append(resumeSteps, d.HandoffInject...)

	newSegID, err := u.spawnSegment(ctx, chat, targetProviderID, extraSteps, handoff, false)
	if err != nil {
		return "", err
	}

	u.bc.BroadcastAgentChat(chatID, "switched")
	return newSegID, nil
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
