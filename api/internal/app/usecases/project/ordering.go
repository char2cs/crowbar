package project

import (
	"slices"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

type slot struct {
	at        int
	id        string
	order     int
	rank      int
	createdAt time.Time
}

type move struct {
	at    int
	order int
}

func place(
	slots []slot,
	id string,
	target *int,
) []move {
	sortSlots(slots)
	if id != "" && target != nil {
		slots = reinsert(slots, id, *target)
	}
	moves := make([]move, 0, len(slots))
	for i, s := range slots {
		if s.order == i {
			continue
		}
		moves = append(moves, move{at: s.at, order: i})
	}
	return moves
}

func sortSlots(
	slots []slot,
) {
	slices.SortFunc(slots, func(a, b slot) int {
		if a.order != b.order {
			return a.order - b.order
		}
		if a.rank != b.rank {
			return a.rank - b.rank
		}
		if !a.createdAt.Equal(b.createdAt) {
			return a.createdAt.Compare(b.createdAt)
		}
		return strings.Compare(a.id, b.id)
	})
}

func finalIndexOf(
	slots []slot,
	id string,
	target *int,
) int {
	cp := make([]slot, len(slots))
	copy(cp, slots)
	sortSlots(cp)
	if id != "" && target != nil {
		cp = reinsert(cp, id, *target)
	}
	return slices.IndexFunc(cp, func(s slot) bool { return s.id == id })
}

func reinsert(
	slots []slot,
	id string,
	target int,
) []slot {
	at := slices.IndexFunc(slots, func(s slot) bool { return s.id == id })
	if at < 0 {
		return slots
	}
	moved := slots[at]
	slots = slices.Delete(slots, at, at+1)
	return slices.Insert(slots, min(max(target, 0), len(slots)), moved)
}

func containsID(
	slots []slot,
	id string,
) bool {
	return slices.ContainsFunc(slots, func(s slot) bool { return s.id == id })
}

func nodeIndex(
	rows []dtree.Node,
) []slot {
	slots := make([]slot, 0, len(rows))
	for i, row := range rows {
		slots = append(slots, slot{at: i, id: row.ID, order: row.Order, rank: rankOfKind(row.Kind)})
	}
	return slots
}

func rankOfKind(
	kind dtree.Kind,
) int {
	switch kind {
	case dtree.KindFolder:
		return tree.RankFolder
	case dtree.KindChat:
		return tree.RankChat
	case dtree.KindWorkspace:
		return tree.RankWorkspace
	}
	return tree.RankRepo
}

func projectIndex(
	rows []domain.Project,
) []slot {
	slots := make([]slot, 0, len(rows))
	for i, row := range rows {
		slots = append(slots, slot{at: i, id: row.ID, order: row.Order})
	}
	return slots
}
