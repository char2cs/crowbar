package runner

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// PendingPrompt returns the chat's most recent prompt submission, if the
// journal has not yet confirmed the provider accepted it. A record already
// in PromptStateAccepted means the ledger has the reply's own request — the
// client's own transcript read already shows it, so there is nothing to
// recover. A record with no stored text is a pre-migration record, written
// before PromptRequest gained a Text field: also nothing to recover.
func (rs *Runners) PendingPrompt(
	ctx context.Context,
	chatID string,
) (domain.PendingPrompt, bool, error) {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return domain.PendingPrompt{}, false, fmt.Errorf("agent: pending prompt: chat: %w", err)
	}
	chatsDir, err := rs.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return domain.PendingPrompt{}, false, fmt.Errorf("agent: pending prompt: chats dir: %w", err)
	}
	dir := rs.prompts.Dir(chatsDir, chat.ID)
	record, found, err := rs.prompts.LatestRequest(dir)
	if err != nil || !found {
		return domain.PendingPrompt{}, false, err
	}
	if record.State == agentjournal.PromptStateAccepted || record.Text == "" {
		return domain.PendingPrompt{}, false, nil
	}
	return domain.PendingPrompt{Text: record.Text, State: record.State}, true, nil
}
