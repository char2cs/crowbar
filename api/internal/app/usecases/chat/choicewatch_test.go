package chat

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChoiceWatch_DropsNotificationsUntilAPublisherIsBound(t *testing.T) {
	var none *ChoiceWatch
	none.Notify("chat-1")

	w := NewChoiceWatch()
	w.Notify("chat-1")

	var got []string
	w.bind(func(chatID string) { got = append(got, chatID) })
	w.Notify("chat-2")

	assert.Equal(t, []string{"chat-2"}, got)
}
