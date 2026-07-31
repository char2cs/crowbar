package dto

import (
	"slices"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// FolderDTO is the wire shape of a Folder: the sidebar's organisation node. It
// carries no status, no branch and no counts, because a folder is not backed by
// anything on disk.
type FolderDTO struct {
	ID        string `json:"id"`
	RepoID    string `json:"repoId"`
	ProjectID string `json:"projectId"`
	ParentID  string `json:"parentId,omitempty"`
	Name      string `json:"name"`
	Order     int    `json:"order"`
	// Status carries the tombstone marker on the Folders broadcast channel: ""
	// for a live folder, "deleted" for a removal frame so the FE entity cache
	// drops it (00 §6, one-type-per-channel, mirroring RepoDTO). Read-path DTOs
	// leave it empty.
	Status string `json:"status,omitempty"`
}

// FolderDTOFrom converts a domain Folder into its wire DTO.
func FolderDTOFrom(
	f domain.Folder,
) FolderDTO {
	return FolderDTO{
		ID:        f.ID,
		RepoID:    f.RepoID,
		ProjectID: f.ProjectID,
		ParentID:  f.ParentID,
		Name:      f.Name,
		Order:     f.Order,
	}
}

// FolderDTOList converts a slice of domain Folders into wire DTOs in sidebar
// order, returning a non-nil empty slice when the input is empty so the envelope
// carries [].
//
// The sort lives HERE, in the converter both the REST list handler and the WS
// snapshot go through, because those are the two answers to the same question
// and a client that got different orders from them would watch its sidebar
// reshuffle on every reconnect.
func FolderDTOList(
	folders []domain.Folder,
) []FolderDTO {
	dtos := make([]FolderDTO, 0, len(folders))
	for _, f := range folders {
		dtos = append(dtos, FolderDTOFrom(f))
	}
	slices.SortFunc(dtos, compareFolderDTOs)
	return dtos
}

// compareFolderDTOs orders folders by their dense sibling index, then by id.
// Order is only meaningful WITHIN a parent, so the list is not a single ordered
// sequence — it is every level's sequence, interleaved; the client groups by
// parentId and reads each group in this order.
func compareFolderDTOs(
	a FolderDTO,
	b FolderDTO,
) int {
	if a.Order != b.Order {
		return a.Order - b.Order
	}
	return strings.Compare(a.ID, b.ID)
}
