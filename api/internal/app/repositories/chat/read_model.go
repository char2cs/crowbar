package chat

import (
	"fmt"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// WatchFunc and ChatEvent are aliases for the store-layer watch seam, exposed so
// callers wire it without importing the internal store package directly.
//
// The repository ANNOUNCES what happened; it does not decide what the frontend is
// told. That decision lives in usecases/chat/internal/fanout.
type (
	WatchFunc = store.WatchFunc
	ChatEvent = store.ChatEvent
	Option    = store.Option
)

// WithPlacementWatch is told, once the read model holds it, of every chat whose
// parent, workspace or type changed, or that was deleted.
func WithPlacementWatch(watch func(chatID string)) Option {
	return store.WithPlacementWatch(watch)
}

// ReadModel is the read model's write side (state/store/agent_chat.db). It is opened before the
// asynx instance so it can be that instance's durable subscriber.
type ReadModel = store.Projection

// OpenReadModel opens the read model over storeDB.
func OpenReadModel(
	storeDB *gormdb.DB,
	opts ...Option,
) (*ReadModel, error) {
	rm, err := store.Open(storeDB, opts...)
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
