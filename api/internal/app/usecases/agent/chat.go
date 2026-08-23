package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func (u *Usecase) RenameChat(
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

func (u *Usecase) RenameByRunner(
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

func (u *Usecase) PurgeChat(
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

func (u *Usecase) purgeChatLocked(
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
	u.retireChatRunners(ctx, chatID)

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

func (u *Usecase) ResumeChat(
	ctx context.Context,
	chatID string,
) (string, error) {
	defer u.spawns.lock(chatID)()

	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if err == nil {
		return live.ID, nil
	}
	if !errors.Is(err, agentrunner.ErrNotFound) {
		return "", fmt.Errorf("agent: resume chat: live runner: %w", err)
	}
	last, err := u.runners.LastConversation(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: resume chat: no conversation to resume: %w", err)
	}
	// The gate is already held: call the inner body, never SwitchProvider itself.
	return u.switchProviderLocked(ctx, chatID, last.ProviderID)
}

func (u *Usecase) StopChat(
	ctx context.Context,
	chatID string,
) error {
	// The chat's spawn gate, for the same reason every teardown path takes it: a stop
	// racing a switch or resume must not terminate a runner the other path is mid-way
	// through placing. It is never taken on the hook path, so a CLI still talking as it
	// dies can always reach us.
	defer u.spawns.lock(chatID)()

	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return nil // already dormant: there is no live CLI to stop
	}
	if err != nil {
		return fmt.Errorf("agent: stop chat: live runner: %w", err)
	}
	u.retire(ctx, live)
	return nil
}

func (u *Usecase) ListChats(
	ctx context.Context,
) ([]domain.AgentChat, error) {
	return u.chats.ListChats(ctx)
}

func (u *Usecase) GetChat(
	ctx context.Context,
	id string,
) (domain.AgentChat, error) {
	return u.chats.GetChat(ctx, id)
}
