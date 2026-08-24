package agent

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func (u *chatUsecase) RenameChat(
	ctx context.Context,
	chatID, title, source string,
) error {
	if title == "" {
		return nil
	}
	chat, err := u.chats.GetChat(ctx, chatID)
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
	if _, err := u.chats.SetTitle(ctx, chatID, title, source); err != nil {
		return fmt.Errorf("agent: rename chat: save: %w", err)
	}
	return nil
}

func (u *chatUsecase) RenameByRunner(
	ctx context.Context,
	runnerID, title, source string,
) error {
	runner, err := u.runners.Get(ctx, runnerID)
	if err != nil {
		return fmt.Errorf("agent: rename by runner: runner: %w", err)
	}
	if runner.CurrentChatID == "" {
		return nil
	}
	return u.RenameChat(ctx, runner.CurrentChatID, title, source)
}

func (u *chatUsecase) PurgeChat(
	ctx context.Context,
	chatID string,
) error {
	// The chat's spawn gate, for the same reason the spawn paths take it: a delete racing a
	// switch would otherwise Start a CLI onto a chat that has just been Forgotten. That
	// self-heals (the runner's first hook finds no chat and retires it), but only after
	// spawning a real process and leaving its tmp dir behind. Serialising is a line.
	defer u.spawns.lock(chatID)()

	return u.purgeChatLocked(ctx, chatID)
}

// The caller already holds chatID's spawn gate: PurgeChat above takes it, and
// the spawn path reaches this from INSIDE it (runnerUsecase.discardSpawnedChat).
// chatGate is not reentrant, so wiring either caller to PurgeChat instead
// compiles and deadlocks that goroutine on its own gate forever.
func (u *chatUsecase) purgeChatLocked(
	ctx context.Context,
	chatID string,
) error {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: purge chat: get: %w", err)
	}
	if err := u.chats.Forget(ctx, chatID); err != nil {
		return fmt.Errorf("agent: purge chat: forget: %w", err)
	}
	u.runner.retireChatRunners(ctx, chatID)

	// Drop the conversation record and its telemetry. The record outlives the
	// process and nothing else removes it, so a hard delete that skipped it would
	// leave the chat's plaintext readable after the user asked for it to be gone.
	if err := u.activity.Forget(ctx, chatID); err != nil {
		slog.ErrorContext(ctx, "agent: purge chat: forget conversation record (best-effort, continuing)",
			"chat_id", chatID, "err", err)
	}
	u.telemetry.forget(chatID)

	// Drop the chat's conversation history. It is append-only and outlives the
	// process, so nothing else ever removes it — and a conversation still pointing
	// at a hard-deleted chat is a live trap: a later /resume of that session id would
	// resolve (ChatForSession) to a chat that no longer exists.
	if err := u.runners.ForgetChat(ctx, chatID); err != nil {
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
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: purge chat: resolve chats dir for reap (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return nil
	}
	home, _, _, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: purge chat: resolve home for reap guard (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return nil
	}
	RemoveUnderHome(ctx, home, filepath.Join(chatsDir, chatID))
	return nil
}

func (u *chatUsecase) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	return u.chats.ListChats(ctx)
}

func (u *chatUsecase) GetChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	return u.chats.GetChat(ctx, id)
}

func (u *chatUsecase) MintChat(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	chatID := uuid.NewString()
	created, err := u.chats.Create(ctx, agentchat.CreateInput{
		ID:          chatID,
		WorkspaceID: workspaceID,
		Now:         time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("agent: mint chat: %w", err)
	}
	u.work.set(chatID, created.Working)
	return chatID, nil
}

func (u *chatUsecase) NoteThreadLineage(
	ctx context.Context,
	chatID string,
	ancestors []string,
) error {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: note thread lineage: chat: %w", err)
	}
	turns, err := u.chatTurns(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: note thread lineage: turns: %w", err)
	}
	if len(turns) == 0 {
		return nil
	}
	return u.appendTurn(ctx, chat, lineageNoteProvider, "user", lineageNoteText(ancestors))
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

func (u *chatUsecase) ListChatsByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.Chat, error) {
	return u.chats.ListByWorkspace(ctx, workspaceID)
}
