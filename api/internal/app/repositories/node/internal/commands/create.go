package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Create mints a Node: the position row for a new chat, folder, repo import, or
// workspace fork/lock. ID reuses the referenced entity's own id — no separate id
// space is minted for it.
type Create struct {
	ID       string
	Kind     domain.NodeKind
	ParentID string
	Order    int
}

func (c Create) AggregateID() string  { return c.ID }
func (c Create) EventName() string    { return "node.created." + c.ID }
func (c Create) ShouldSnapshot() bool { return false }

func (c Create) Validate(current *domain.Node) error {
	if current != nil {
		return fmt.Errorf("create node: exists: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" {
		return fmt.Errorf("create node: missing id: %w", asynxModels.ErrValidation)
	}
	if !validNodeKind(c.Kind) {
		return fmt.Errorf("create node: invalid kind: %w", asynxModels.ErrValidation)
	}
	if c.Order < 0 {
		return fmt.Errorf("create node: negative order: %w", asynxModels.ErrValidation)
	}
	return nil
}

func validNodeKind(k domain.NodeKind) bool {
	switch k {
	case domain.NodeKindChat, domain.NodeKindFolder, domain.NodeKindWorkspace, domain.NodeKindRepo:
		return true
	default:
		return false
	}
}

func (c Create) EmitEvent(_ *domain.Node) domain.Node {
	return domain.Node{
		ID:       c.ID,
		Kind:     c.Kind,
		ParentID: c.ParentID,
		Order:    c.Order,
	}
}
