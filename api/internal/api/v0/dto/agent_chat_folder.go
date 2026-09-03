package dto

import (
	"slices"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// AgentChatFolderDTO is the wire shape of an AgentChatFolder: the Chats panel's
// organisation node. It carries no runner, no provider and no turn state,
// because a folder is not backed by anything a process owns.
//
// There is no status/tombstone field, unlike the sidebar's FolderDTO. This shape
// never rides a resource stream — the Chats socket is a bare EVENT feed — so a
// removal is announced as a folder frame on that feed and the client re-reads
// this list, rather than being handed a row whose only content is that it is
// gone.
type AgentChatFolderDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	ParentID    string `json:"parentId,omitempty"`
	Name        string `json:"name"`
	Order       int    `json:"order"`
}

// AgentChatFolderDTOFrom converts a domain AgentChatFolder into its wire DTO.
func AgentChatFolderDTOFrom(
	f domain.ChatFolder,
) AgentChatFolderDTO {
	return AgentChatFolderDTO{
		ID:          f.ID,
		WorkspaceID: f.WorkspaceID,
		ParentID:    f.ParentID,
		Name:        f.Name,
		Order:       f.Order,
	}
}

// AgentChatFolderDTOList converts a slice of domain AgentChatFolders into wire
// DTOs in panel order, returning a non-nil empty slice when the input is empty
// so the envelope carries [].
//
// The sort lives HERE, in the converter every read goes through, because a
// client that got one order from the list and another from a reseed would watch
// its panel reshuffle on every reconnect.
func AgentChatFolderDTOList(
	folders []domain.ChatFolder,
) []AgentChatFolderDTO {
	dtos := make([]AgentChatFolderDTO, 0, len(folders))
	for _, f := range folders {
		dtos = append(dtos, AgentChatFolderDTOFrom(f))
	}
	slices.SortFunc(dtos, compareAgentChatFolderDTOs)
	return dtos
}

// compareAgentChatFolderDTOs orders folders by their dense sibling index, then
// by id. Order is only meaningful WITHIN a parent, so the list is not a single
// ordered sequence — it is every level's sequence, interleaved; the client groups
// by parentId and reads each group in this order, merging the chats that share
// those levels.
func compareAgentChatFolderDTOs(
	a AgentChatFolderDTO,
	b AgentChatFolderDTO,
) int {
	if a.Order != b.Order {
		return a.Order - b.Order
	}
	return strings.Compare(a.ID, b.ID)
}
