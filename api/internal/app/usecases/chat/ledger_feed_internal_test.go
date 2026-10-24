package chat

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/transcript"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// A chat the record erases takes what its clients were told of its questions
// with it: the same chat id announced again is told again, not deduped.
func TestFeedTranscript_ForgetErasesWhatTheChatsClientsWereTold(t *testing.T) {
	fed := &feedTranscript{Projector: transcript.New(nil)}
	told := 0
	feed := &ChatFeed{Choices: func(string, string, []domain.ActivityChoice, []string) { told++ }}
	asked := []domain.ActivityChoice{{ID: "a"}}
	none := func(string, []domain.ActivityChoice) []string { return nil }

	fed.choices.publish(feed, "c1", "ws", asked, none)
	fed.Forget("c1")
	fed.choices.publish(feed, "c1", "ws", asked, none)

	assert.Equal(t, 2, told)
}
