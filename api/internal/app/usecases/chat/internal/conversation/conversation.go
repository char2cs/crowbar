// Package conversation owns the chat record: the aggregate's identity, title and
// model selection, and every read served off the conversation it accumulates.
//
// It starts no processes. A chat exists, and is readable, whether or not a CLI
// has ever run on it — minting one, renaming it and reading its log are all
// answerable with no runner in sight, which is what makes them separable from the
// runner lifecycle at all.
//
// The one thing it does owe the processes on a chat is the hard delete: erasing
// the aggregate has to retire the CLIs pointed at it, which it asks the runner
// lifecycle to do through a port rather than reaching for it.
package conversation

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// Runners is the one thing this package needs from the runner lifecycle: the
// CLIs a hard delete owes retirement to.
//
// It is declared here, by the consumer, so the two never import each other. A
// chat's erasure decides that the processes on it must go; how they are torn
// down is not its business.
type Runners interface {
	// RetireChatRunners quits every CLI currently placed on the chat. It is
	// best-effort: a chat is erased whether or not its processes could be reached.
	RetireChatRunners(
		ctx context.Context,
		chatID string,
	)
}

// Conversations is the chat record.
type Conversations struct {
	chats       agentchat.EventStore
	runnerStore agentrunner.EventStore
	// activity is the conversation record: turns, tool calls, subagents and
	// interruptions. Every read this type serves comes off it.
	activity  agentactivity.EventStore
	telemetry *telemetry.Store
	agents    engineagents.Agents
	ws        seam.WorkspaceReader
	// lineage answers "what does this chat read" at spawn time. See ThreadContext.
	lineage seam.ChatLineage
	// home is the app-config crowbar-home resolver, NOT a wsId lookup: it resolves
	// the descriptor catalog a chat's model/effort selection is validated against.
	home func() (string, error)
	work *inflight.Work
	// permissionLevels is the per-chat trust dial a newly minted chat is seeded
	// into, from the global default at the moment of creation (see
	// defaultPermissionLevel).
	permissionLevels *permission.Store
	// defaultPermissionLevel resolves the current global default at mint time.
	// It's a closure, not a direct usecase reference, the same way home is a
	// closure — this package must not import the chat usecase package it lives
	// inside of.
	defaultPermissionLevel func(ctx context.Context) (permission.Level, error)
	// spawns is the per-chat gate the USER-INITIATED spawn paths share. PurgeChat
	// takes it for the same reason they do — a delete racing a switch would
	// otherwise start a CLI onto a chat that has just been forgotten.
	spawns *inflight.Gate
	// runners is the runner lifecycle, reached for the one thing a hard delete
	// owes the processes on the chat: retiring them. It is bound AFTER construction
	// because the two sides are built together and neither can name the other
	// first — see SetRunners.
	runners Runners
}

// Deps is everything the chat record is built over.
type Deps struct {
	Chats     agentchat.EventStore
	Runners   agentrunner.EventStore
	Activity  agentactivity.EventStore
	Telemetry *telemetry.Store
	Agents    engineagents.Agents
	Workspace seam.WorkspaceReader
	Lineage   seam.ChatLineage
	Home      func() (string, error)

	Work   *inflight.Work
	Spawns *inflight.Gate

	PermissionLevels *permission.Store
	// DefaultPermissionLevel resolves the current global default at the
	// moment a chat is minted. It's a closure, not a direct usecase
	// reference, the same way Home is a closure — conversation must not
	// import the chat usecase package it lives inside of.
	DefaultPermissionLevel func(ctx context.Context) (permission.Level, error)
}

// New builds the chat record. The runner port is bound separately, by SetRunners.
func New(d Deps) *Conversations {
	return &Conversations{
		chats:       d.Chats,
		runnerStore: d.Runners,
		activity:    d.Activity,
		telemetry:   d.Telemetry,
		agents:      d.Agents,
		ws:          d.Workspace,
		lineage:     d.Lineage,
		home:        d.Home,
		work:        d.Work,
		spawns:      d.Spawns,

		permissionLevels:       d.PermissionLevels,
		defaultPermissionLevel: d.DefaultPermissionLevel,
	}
}

// SetRunners binds the runner lifecycle a hard delete retires through.
//
// It is a setter and not a constructor argument because the erasure edge is a
// cycle: a purge retires the CLIs on a chat, and a spawn that fails discards the
// half-created chat it was for. Building both and then closing the edge is the
// honest expression of that; a constructor that could take it would be lying
// about which one exists first.
func (c *Conversations) SetRunners(runners Runners) { c.runners = runners }

func (c *Conversations) RenameChat(
	ctx context.Context,
	chatID, title, source string,
) error {
	if title == "" {
		return nil
	}
	chat, err := c.chats.GetChat(ctx, chatID)
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
		source = "user"
	}
	if _, err := c.chats.SetTitle(ctx, chatID, title, source); err != nil {
		return fmt.Errorf("agent: rename chat: save: %w", err)
	}
	return nil
}

func (c *Conversations) RenameByRunner(
	ctx context.Context,
	runnerID, title, source string,
) error {
	runner, err := c.runnerStore.Get(ctx, runnerID)
	if err != nil {
		return fmt.Errorf("agent: rename by runner: runner: %w", err)
	}
	if runner.CurrentChatID == "" {
		return nil
	}
	return c.RenameChat(ctx, runner.CurrentChatID, title, source)
}

func (c *Conversations) PurgeChat(
	ctx context.Context,
	chatID string,
) error {
	// The chat's spawn gate, for the same reason the spawn paths take it: a delete racing a
	// switch would otherwise Start a CLI onto a chat that has just been Forgotten. That
	// self-heals (the runner's first hook finds no chat and retires it), but only after
	// spawning a real process and leaving its tmp dir behind. Serialising is a line.
	defer c.spawns.Lock(chatID)()

	return c.PurgeLocked(ctx, chatID)
}

// The caller already holds chatID's spawn gate: PurgeChat above takes it, and
// the spawn path reaches this from INSIDE it (the runner lifecycle's discardSpawnedChat).
// inflight.Gate is not reentrant, so wiring either caller to PurgeChat instead
// compiles and deadlocks that goroutine on its own gate forever.
func (c *Conversations) PurgeLocked(
	ctx context.Context,
	chatID string,
) error {
	chat, err := c.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: purge chat: get: %w", err)
	}
	if err := c.chats.Forget(ctx, chatID); err != nil {
		return fmt.Errorf("agent: purge chat: forget: %w", err)
	}
	c.runners.RetireChatRunners(ctx, chatID)

	// Drop the conversation record and its telemetry. The record outlives the
	// process and nothing else removes it, so a hard delete that skipped it would
	// leave the chat's plaintext readable after the user asked for it to be gone.
	if err := c.activity.Forget(ctx, chatID); err != nil {
		slog.ErrorContext(ctx, "agent: purge chat: forget conversation record (best-effort, continuing)",
			"chat_id", chatID, "err", err)
	}
	c.telemetry.Forget(chatID)

	// Drop the chat's conversation history. It is append-only and outlives the
	// process, so nothing else ever removes it — and a conversation still pointing
	// at a hard-deleted chat is a live trap: a later /resume of that session id would
	// resolve (ChatForSession) to a chat that no longer exists.
	if err := c.runnerStore.ForgetChat(ctx, chatID); err != nil {
		slog.ErrorContext(ctx, "agent: purge chat: forget conversation history (best-effort, continuing)",
			"chat_id", chatID, "err", err)
	}

	// Reap the chat's on-disk footprint now the aggregate is gone. The conversation
	// itself no longer lives here — it is in the record dropped above — but a chat
	// directory may still hold whatever a spawn left in it.
	//
	// NOT the runner's tmp dir, which no longer lives under the chat (worktreepath.RunnerDir)
	// and is no business of the chat's: it belongs to the PROCESS lifecycle, and the process
	// we have just SIGTERM'd is still alive and still reading it. Removing a live CLI's
	// config out from under it was only ever an accident of the old layout. It goes when the
	// PTY does (onExit), or at the next boot if the daemon died first.
	//
	// The removal is routed through RemoveUnderHome, which re-asserts the target is strictly
	// under crowbar home, so even a poisoned chats dir can never reach the user's real
	// repository.
	chatsDir, err := c.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: purge chat: resolve chats dir for reap (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return nil
	}
	home, _, _, _, err := c.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: purge chat: resolve home for reap guard (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return nil
	}
	worktreepath.RemoveUnderHome(ctx, home, filepath.Join(chatsDir, chatID))
	return nil
}

func (c *Conversations) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	return c.chats.ListChats(ctx)
}

func (c *Conversations) GetChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	return c.chats.GetChat(ctx, id)
}

func (c *Conversations) MintChat(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	chatID := uuid.NewString()
	created, err := c.chats.Create(ctx, agentchat.CreateInput{
		ID:          chatID,
		WorkspaceID: workspaceID,
		Now:         time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("agent: mint chat: %w", err)
	}
	c.work.Set(chatID, created.Working)
	// A default-level read failure is swallowed here, not propagated: the chat
	// must still get created even if the lookup has trouble. permission.Store's
	// own Guarded fallback is the safety net for a chat never seeded here.
	if level, err := c.defaultPermissionLevel(ctx); err == nil {
		c.permissionLevels.Set(chatID, level)
	}
	return chatID, nil
}

func (c *Conversations) NoteThreadLineage(
	ctx context.Context,
	chatID string,
	ancestors []string,
) error {
	chat, err := c.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: note thread lineage: chat: %w", err)
	}
	turns, err := c.ChatTurns(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: note thread lineage: turns: %w", err)
	}
	if len(turns) == 0 {
		return nil
	}
	return c.appendTurn(ctx, chat, lineageNoteProvider, "user", lineageNoteText(ancestors))
}

const lineageNoteProvider = "crowbar"

func lineageNoteText(
	ancestors []string,
) string {
	return "[Crowbar] This chat was moved in the Chats panel and is a THREAD of " +
		strings.Join(ancestors, ", ") + " (nearest parent first) from this point on. " +
		"Read those chats with get_chat_log. Everything above this line was said BEFORE the move, " +
		"without any of that context: the move changes what this chat reads from now on and " +
		"rewrites nothing it has already read."
}

// Ancestors returns the CHAT ancestors of chatID, nearest parent first — what a
// thread inherits, and the same answer the spawn path composes prior context
// from. Folders are transparent to it: a thread filed two folders deep under a
// chat inherits exactly what it would sitting directly under it.
//
// Empty for a chat at the panel root and for one merely filed in a folder;
// neither reads anything.
func (c *Conversations) Ancestors(
	ctx context.Context,
	chatID string,
) ([]string, error) {
	return c.lineage.Ancestors(ctx, chatID)
}

func (c *Conversations) ListChatsByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.Chat, error) {
	return c.chats.ListByWorkspace(ctx, workspaceID)
}
