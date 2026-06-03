package chat

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	appHub "github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type stubConvWriter struct{}

func (s *stubConvWriter) Create(
	_ context.Context,
	_ string,
	_ domain.ConversationMessageRole,
	_ domain.ConversationMessageType,
	_ string,
) (domain.ConversationMessage, error) {
	return domain.ConversationMessage{}, nil
}

func TestNewHandler_returnsNonNil(t *testing.T) {
	h := NewHandler(appHub.NewChatHub(), &stubConvWriter{})
	assert.NotNil(t, h)
}
