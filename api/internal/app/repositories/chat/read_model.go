package chat

import (
	"fmt"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// WatchFunc and ChatEvent alias the store-layer watch seam so callers need not
// import the internal store package. The repository announces what happened; the
// usecase decides what the frontend is told.
type (
	WatchFunc = store.WatchFunc
	ChatEvent = store.ChatEvent
)

// ReadModel is the read model's write side (state/store/agent_chat.db). It is opened before the
// asynx instance so it can be that instance's durable subscriber.
type ReadModel = store.Projection

// OpenReadModel opens the read model over storeDB.
func OpenReadModel(
	storeDB *gormdb.DB,
) (*ReadModel, error) {
	rm, err := store.Open(storeDB)
	if err != nil {
		return nil, fmt.Errorf("agentchat: read model: %w", err)
	}
	return rm, nil
}

// Subscriber is the builder option that makes rm the instance's durable subscriber.
func Subscriber(
	rm *ReadModel,
) func(*asynx.Builder[domain.Chat]) {
	return store.Subscriber(rm)
}
