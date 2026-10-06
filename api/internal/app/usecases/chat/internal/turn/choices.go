package turn

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChoiceSet is one chat's pending prompts and which of them a relay is holding.
type ChoiceSet struct {
	ChatID      string
	WorkspaceID string
	Choices     []domain.ActivityChoice
	Answerable  []string
}

// choiceFeed remembers what each chat was last told, so a change that moves
// nothing a client can see is not re-announced.
type choiceFeed struct {
	mu   sync.Mutex
	sent map[string]string
}

func newChoiceFeed() *choiceFeed { return &choiceFeed{sent: map[string]string{}} }

// PublishChoices pushes chatID's pending prompts to the chat feed when they
// differ from what it last pushed. It reads and pushes under one lock, so two
// racing announcements cannot arrive in the opposite order to their reads and
// leave a client holding the older set.
func (t *Turns) PublishChoices(chatID string) {
	if t.feed.Choices == nil {
		return
	}
	t.choiceFeed.mu.Lock()
	defer t.choiceFeed.mu.Unlock()

	ctx := context.Background()
	chat, err := t.chats.GetChat(ctx, chatID)
	if err != nil {
		delete(t.choiceFeed.sent, chatID)
		return
	}
	choices, err := t.activity.PendingChoices(ctx, chatID)
	if err != nil {
		return
	}
	answerable := t.answers.AnswerableIDs(chatID, choices)
	previous, announced := t.choiceFeed.sent[chatID]
	if len(choices) == 0 {
		// The clear is owed only to a client that was told of a prompt.
		if !announced {
			return
		}
		delete(t.choiceFeed.sent, chatID)
		t.feed.Choices(chatID, chat.WorkspaceID, nil, nil)
		return
	}
	fingerprint := choiceFingerprint(choices, answerable)
	if announced && previous == fingerprint {
		return
	}
	t.choiceFeed.sent[chatID] = fingerprint
	t.feed.Choices(chatID, chat.WorkspaceID, choices, answerable)
}

func choiceFingerprint(choices []domain.ActivityChoice, answerable []string) string {
	ids := make([]string, 0, len(choices))
	for _, c := range choices {
		ids = append(ids, c.ID)
	}
	slices.Sort(answerable)
	return strings.Join(ids, ",") + "|" + strings.Join(answerable, ",")
}

// PendingChoiceSets is every chat's pending prompts, for a client that has just
// connected and so heard none of the frames that announced them.
func (t *Turns) PendingChoiceSets(ctx context.Context) ([]ChoiceSet, error) {
	all, err := t.activity.AllPendingChoices(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: pending choice sets: %w", err)
	}
	var sets []ChoiceSet
	for _, choice := range all {
		if n := len(sets); n > 0 && sets[n-1].ChatID == choice.ChatID {
			sets[n-1].Choices = append(sets[n-1].Choices, choice)
			continue
		}
		sets = append(sets, ChoiceSet{ChatID: choice.ChatID, Choices: []domain.ActivityChoice{choice}})
	}
	out := sets[:0]
	for _, set := range sets {
		chat, err := t.chats.GetChat(ctx, set.ChatID)
		if err != nil {
			continue
		}
		set.WorkspaceID = chat.WorkspaceID
		set.Answerable = t.answers.AnswerableIDs(set.ChatID, set.Choices)
		out = append(out, set)
	}
	return out, nil
}
