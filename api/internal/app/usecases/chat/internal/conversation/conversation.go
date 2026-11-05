// Package conversation owns the chat record: the aggregate's identity, title and
// model selection, and every read served off the conversation it accumulates.
//
// It starts no processes: a chat is readable whether or not a CLI ever ran on it.
// The one thing it owes the processes is the hard delete, which retires a chat's
// CLIs through the Runners port rather than reaching for the runner lifecycle.
package conversation

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Runners is what this package needs from the runner lifecycle: live runner reads
// and the retirement a hard delete owes. Declared by the consumer so the two never
// import each other.
type Runners interface {
	// RunnerByID is the live runner runnerID, also while it is exiting.
	RunnerByID(runnerID string) (engineagents.Runner, bool)
	// LiveRunnerForChat is the chat's live runner; an error when it is dormant.
	LiveRunnerForChat(ctx context.Context, chatID string) (engineagents.Runner, error)
	// RetireChatRunners records the end of the CLI on the chat, under its lock,
	// and returns the end of its processes, run once the lock is let go. It is
	// best-effort: a chat is erased whether or not its processes are reached.
	RetireChatRunners(
		ctx context.Context,
		chatID string,
	) (end func())
	// HoldChat cancels the chat's parked user-initiated waits and takes its
	// lock: a purge never queues behind a wait on a chat that is going away.
	HoldChat(ctx context.Context, chatID string) (release func(), err error)
}

// Writes commits rows to a chat's ledger under the chat's lock.
type Writes interface {
	Do(ctx context.Context, chatID string, fn func(ledger.State) ([]ledger.Row, error)) ([]ledger.Row, error)
}

// Conversations is the chat record.
type Conversations struct {
	chats agentchat.EventStore
	// blobs are the tool payloads the chat's rows name.
	blobs     *ledgerstore.Blobs
	telemetry *telemetry.Store
	agents    engineagents.Agents
	ws        seam.WorkspaceReader
	// lineage answers "what does this chat read" at spawn time. See ThreadContext.
	lineage seam.ChatLineage
	// home is the app-config crowbar-home resolver, NOT a wsId lookup: it resolves
	// the descriptor catalog a chat's model/effort selection is validated against.
	home   func() (string, error)
	ledger *ledgerstore.Store
	// writes commits the chat's own notes, under the chat's lock.
	writes Writes
	// defaultPermissionLevel resolves the global default at mint time, unchecked
	// against any provider. A closure because this package may not import its parent.
	defaultPermissionLevel func(ctx context.Context) (string, error)
	// runners is the runner lifecycle, bound after construction by SetRunners because
	// the two sides are built together and neither can name the other first.
	runners Runners
}

// Deps is everything the chat record is built over.
type Deps struct {
	Chats     agentchat.EventStore
	Blobs     *ledgerstore.Blobs
	Telemetry *telemetry.Store
	Agents    engineagents.Agents
	Workspace seam.WorkspaceReader
	Lineage   seam.ChatLineage
	Home      func() (string, error)
	Ledger    *ledgerstore.Store
	Writes    Writes

	// DefaultPermissionLevel resolves the global default at mint time; a closure
	// because conversation may not import the chat usecase package above it.
	DefaultPermissionLevel func(ctx context.Context) (string, error)
}

// New builds the chat record. The runner port is bound separately, by SetRunners.
func New(d Deps) *Conversations {
	return &Conversations{
		chats:     d.Chats,
		blobs:     d.Blobs,
		telemetry: d.Telemetry,
		agents:    d.Agents,
		ws:        d.Workspace,
		lineage:   d.Lineage,
		home:      d.Home,
		ledger:    d.Ledger,
		writes:    d.Writes,

		defaultPermissionLevel: d.DefaultPermissionLevel,
	}
}

// SetRunners binds the runner lifecycle a hard delete retires through. A setter
// because the edge is a cycle: a purge retires a chat's CLIs, and a failed spawn
// discards the chat it was for.
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
	runner, ok := c.runners.RunnerByID(runnerID)
	if !ok {
		return fmt.Errorf("agent: rename by runner: runner %s is not live: %w", runnerID, problem.ErrNotFound)
	}
	return c.RenameChat(ctx, runner.CurrentChatID, title, source)
}

func (c *Conversations) PurgeChat(
	ctx context.Context,
	chatID string,
) error {
	// The chat's lock, as the spawn paths take it: a delete racing a switch would
	// otherwise start a CLI onto a chat that has just been forgotten, and a hook
	// would record onto it.
	release, err := c.runners.HoldChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: purge chat: %w", err)
	}
	if _, err := c.chats.GetChat(ctx, chatID); err != nil {
		release()
		return fmt.Errorf("agent: purge chat: get: %w", err)
	}
	// Retire first, so the chat's feed still carries its runner's exit; the
	// processes end once the lock is let go.
	end := c.runners.RetireChatRunners(ctx, chatID)
	err = c.forget(ctx, chatID)
	release()
	end()
	return err
}

// forgetLedger erases chatID's rows, then every payload they named that no
// other chat's rows name. Best-effort: a payload left behind is disk, not text
// anyone can still reach.
func (c *Conversations) forgetLedger(ctx context.Context, chatID string) {
	refs, err := c.ledger.BlobRefs(ctx, chatID)
	if err != nil {
		slog.ErrorContext(ctx, "agent: purge chat: list payloads (best-effort, continuing)", "chat_id", chatID, "err", err)
	}
	if err := c.ledger.Forget(ctx, chatID); err != nil {
		slog.ErrorContext(ctx, "agent: purge chat: forget ledger (best-effort, continuing)", "chat_id", chatID, "err", err)
	}
	inUse := func(ref string) (bool, error) { return c.ledger.RefInUse(ctx, ref) }
	for _, ref := range refs {
		if err := c.blobs.Collect(ref, inUse); err != nil {
			slog.WarnContext(ctx, "agent: purge chat: delete payload (best-effort, continuing)", "chat_id", chatID, "err", err)
		}
	}
}

// forget erases chatID's record, conversation, ledger and directory.
func (c *Conversations) forget(ctx context.Context, chatID string) error {
	if err := c.chats.Forget(ctx, chatID); err != nil {
		return fmt.Errorf("agent: purge chat: forget: %w", err)
	}

	// Drop the conversation and its telemetry. The ledger outlives the process
	// and nothing else removes it, so a hard delete that skipped it would leave
	// the chat's plaintext readable after the user asked for it to be gone.
	c.telemetry.Forget(ctx, chatID)
	c.forgetLedger(ctx, chatID)

	// Reap the chat's own directory, keyed by id under home because WorkspaceID is optional
	// and mutable. The runner's tmp dir is not touched: it belongs to the process lifecycle,
	// and the CLI just signalled may still be reading it.
	home, err := c.home()
	if err != nil {
		slog.WarnContext(ctx, "agent: purge chat: resolve home for reap (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return nil
	}
	// Best-effort: a reap that cannot happen leaves disk behind; failing the purge
	// would leave a chat the user asked to erase still listed.
	if err := worktreepath.RemoveUnderHome(home, filepath.Join(worktreepath.LedgerChatsDir(home), chatID)); err != nil {
		slog.WarnContext(ctx, "agent: purge chat: reap its chat directory (best-effort)", "chat_id", chatID, "err", err)
	}
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
	return c.appendNote(ctx, chat.ID, lineageNoteText(ancestors))
}

func lineageNoteText(
	ancestors []string,
) string {
	return "[Crowbar] This chat was moved in the Chats panel and is a THREAD of " +
		strings.Join(ancestors, ", ") + " (nearest parent first) from this point on. " +
		"Read those chats with get_chat_log. Everything above this line was said BEFORE the move, " +
		"without any of that context: the move changes what this chat reads from now on and " +
		"rewrites nothing it has already read."
}

// NotePromotion records, in a chat's own conversation, that it was just promoted into
// its own worktree. Appended after the respawn, so the new CLI's handoff never sees it;
// a chat that has said nothing gets no note.
func (c *Conversations) NotePromotion(
	ctx context.Context,
	chatID string,
) error {
	chat, err := c.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: note promotion: chat: %w", err)
	}
	turns, err := c.ChatTurns(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: note promotion: turns: %w", err)
	}
	if len(turns) == 0 {
		return nil
	}
	return c.appendNote(ctx, chat.ID, promotionNoteText())
}

func promotionNoteText() string {
	return "[Crowbar] This chat was just promoted to its own git worktree. " +
		"Everything above this line was said BEFORE the promotion, running with no worktree of its own; " +
		"from this point on it runs in the new worktree the promotion created."
}

// Ancestors returns the CHAT ancestors of chatID, nearest parent first — what a
// thread inherits and the spawn path composes prior context from. Folders are
// transparent; empty at the panel root or merely filed in a folder.
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
