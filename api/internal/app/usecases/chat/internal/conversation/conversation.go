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

type Runners interface {
	RunnerByID(runnerID string) (engineagents.Runner, bool)
	LiveRunnerForChat(ctx context.Context, chatID string) (engineagents.Runner, error)
	RetireChatRunners(
		ctx context.Context,
		chatID string,
	) (end func())
	HoldChat(ctx context.Context, chatID string) (release func(), err error)
}

type Writes interface {
	Do(ctx context.Context, chatID string, fn func(ledger.State) ([]ledger.Row, error)) ([]ledger.Row, error)
}

type Conversations struct {
	chats                  agentchat.EventStore
	blobs                  *ledgerstore.Blobs
	telemetry              *telemetry.Store
	agents                 engineagents.Agents
	ws                     seam.WorkspaceReader
	lineage                seam.ChatLineage
	home                   func() (string, error)
	ledger                 *ledgerstore.Store
	writes                 Writes
	defaultPermissionLevel func(ctx context.Context) (string, error)
	runners                Runners
}

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

	DefaultPermissionLevel func(ctx context.Context) (string, error)
}

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
	default:
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
	release, err := c.runners.HoldChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: purge chat: %w", err)
	}
	if _, err := c.chats.GetChat(ctx, chatID); err != nil {
		release()
		return fmt.Errorf("agent: purge chat: get: %w", err)
	}
	end := c.runners.RetireChatRunners(ctx, chatID)
	err = c.forget(ctx, chatID)
	release()
	end()
	return err
}

func (c *Conversations) forgetLedger(ctx context.Context, chatID string) {
	refs, err := c.ledger.BlobRefs(ctx, chatID)
	if err != nil {
		slog.ErrorContext(ctx, "agent: purge chat: list payloads (best-effort, continuing)", "chat_id", chatID, "err", err)
	}
	if err := c.ledger.Forget(ctx, chatID); err != nil {
		slog.ErrorContext(ctx, "agent: purge chat: forget ledger (best-effort, continuing)", "chat_id", chatID, "err", err)
	}
	stillUsed, err := c.ledger.RefsInUse(ctx, refs)
	if err != nil {
		slog.ErrorContext(ctx, "agent: purge chat: scan refs in use (best-effort, continuing)", "chat_id", chatID, "err", err)
	}
	inUse := func(ref string) (bool, error) { return stillUsed[ref], nil }
	for _, ref := range refs {
		if ctx.Err() != nil {
			slog.WarnContext(ctx, "agent: purge chat: payload sweep aborted, context canceled", "chat_id", chatID)
			break
		}
		if err := c.blobs.Collect(ref, inUse); err != nil {
			slog.WarnContext(ctx, "agent: purge chat: delete payload (best-effort, continuing)", "chat_id", chatID, "err", err)
		}
	}
}

func (c *Conversations) forget(ctx context.Context, chatID string) error {
	if err := c.chats.Forget(ctx, chatID); err != nil {
		return fmt.Errorf("agent: purge chat: forget: %w", err)
	}

	c.telemetry.Forget(ctx, chatID)
	c.forgetLedger(ctx, chatID)

	home, err := c.home()
	if err != nil {
		slog.WarnContext(ctx, "agent: purge chat: resolve home for reap (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return nil
	}
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
