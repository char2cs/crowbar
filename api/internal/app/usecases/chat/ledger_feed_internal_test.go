package chat

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/transcript"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// A chat the record erases takes what its clients were told of its questions
// with it: the same chat id announced again is told again, not deduped.
func TestForgetting_ErasesWhatTheChatsClientsWereTold(t *testing.T) {
	var choices choiceFeed
	told := 0
	feed := &ChatFeed{Choices: func(string, string, []domain.ActivityChoice, []string) { told++ }}
	asked := []domain.ActivityChoice{{ID: "a"}}
	none := func(string, []domain.ActivityChoice) []string { return nil }

	choices.publish(feed, "c1", "ws", asked, none)
	forgetting{Projector: transcript.New(nil), choices: &choices}.Forget("c1")
	choices.publish(feed, "c1", "ws", asked, none)

	assert.Equal(t, 2, told)
}
