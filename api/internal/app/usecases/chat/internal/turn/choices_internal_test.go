package turn

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type pendingActivity struct {
	agentactivity.EventStore
	pending map[string][]domain.ActivityChoice
}

func (a *pendingActivity) PendingChoices(_ context.Context, chatID string) ([]domain.ActivityChoice, error) {
	return a.pending[chatID], nil
}

func (a *pendingActivity) AllPendingChoices(context.Context) ([]domain.ActivityChoice, error) {
	var all []domain.ActivityChoice
	for _, choices := range a.pending {
		all = append(all, choices...)
	}
	return all, nil
}

type pushedChoices struct {
	chatID      string
	workspaceID string
	ids         []string
	answerable  []string
}

func choiceRig(activity *pendingActivity) (*Turns, *[]pushedChoices) {
	desk := answerdesk.New(answerdesk.DefaultRetention, nil)
	turns := New(Deps{Chats: raceChats{}, Activity: activity, Answers: desk})
	var pushed []pushedChoices
	turns.SetFeed(seam.ChatFeed{Choices: func(chatID, workspaceID string, choices []domain.ActivityChoice, answerable []string) {
		got := pushedChoices{chatID: chatID, workspaceID: workspaceID, answerable: answerable}
		for _, c := range choices {
			got.ids = append(got.ids, c.ID)
		}
		pushed = append(pushed, got)
	}})
	return turns, &pushed
}

func TestPublishChoices_PushesTheWholeSetOnceAndTheClearWhenItEmpties(t *testing.T) {
	activity := &pendingActivity{pending: map[string][]domain.ActivityChoice{
		"chat-1": {{ID: "choice-a", ChatID: "chat-1"}, {ID: "choice-b", ChatID: "chat-1"}},
	}}
	turns, pushed := choiceRig(activity)

	turns.PublishChoices("chat-1")
	turns.PublishChoices("chat-1")
	require.Len(t, *pushed, 1, "an unchanged set is not announced twice")
	assert.Equal(t, []string{"choice-a", "choice-b"}, (*pushed)[0].ids)

	activity.pending["chat-1"] = nil
	turns.PublishChoices("chat-1")
	require.Len(t, *pushed, 2)
	assert.Empty(t, (*pushed)[1].ids, "an empty set is the clear")

	turns.PublishChoices("chat-1")
	assert.Len(t, *pushed, 2, "a chat that never had a prompt is not announced")
}

func TestPublishChoices_AnAnswerabilityChangeAloneIsAnnounced(t *testing.T) {
	activity := &pendingActivity{pending: map[string][]domain.ActivityChoice{
		"chat-1": {{ID: "choice-a", ChatID: "chat-1"}},
	}}
	desk := answerdesk.New(answerdesk.DefaultRetention, nil)
	turns := New(Deps{Chats: raceChats{}, Activity: activity, Answers: desk})
	var answerable [][]string
	turns.SetFeed(seam.ChatFeed{Choices: func(_, _ string, _ []domain.ActivityChoice, held []string) {
		answerable = append(answerable, held)
	}})

	turns.PublishChoices("chat-1")
	desk.Hold("delivery-1", answerdesk.Prompt{ChoiceID: "choice-a", ChatID: "chat-1"})
	turns.PublishChoices("chat-1")

	assert.Equal(t, [][]string{{}, {"choice-a"}}, answerable)
}

func TestPendingChoiceSets_GroupsByChatAndSkipsChatsThatAreGone(t *testing.T) {
	activity := &pendingActivity{pending: map[string][]domain.ActivityChoice{
		"chat-1": {{ID: "choice-a", ChatID: "chat-1"}, {ID: "choice-b", ChatID: "chat-1"}},
	}}
	turns, _ := choiceRig(activity)

	sets, err := turns.PendingChoiceSets(t.Context())

	require.NoError(t, err)
	require.Len(t, sets, 1)
	assert.Equal(t, "chat-1", sets[0].ChatID)
	assert.Len(t, sets[0].Choices, 2)
}
