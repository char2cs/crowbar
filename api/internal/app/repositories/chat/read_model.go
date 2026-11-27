package chat

import (
	"fmt"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type (
	WatchFunc = store.WatchFunc
	ChatEvent = store.ChatEvent
)

type ReadModel = store.Projection

func OpenReadModel(
	storeDB *gormdb.DB,
) (*ReadModel, error) {
	rm, err := store.Open(storeDB)
	if err != nil {
		return nil, fmt.Errorf("agentchat: read model: %w", err)
	}
	return rm, nil
}

func Subscriber(
	rm *ReadModel,
) func(*asynx.Builder[domain.Chat]) {
	return store.Subscriber(rm)
}
