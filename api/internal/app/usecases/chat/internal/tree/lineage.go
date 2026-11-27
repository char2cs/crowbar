package tree

import (
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree/internal/lineage"
)

type Lineage = lineage.Resolver

func NewLineage(chats Chats) *Lineage {
	return lineage.New(chats)
}
