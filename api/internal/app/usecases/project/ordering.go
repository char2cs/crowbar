package project

import (
	"slices"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

// slot is one row of an ordered sidebar list: where it sits in the caller's
// slice, its id, and the order it currently holds.
type slot struct {
	at    int
	id    string
	order int
	// rank and createdAt break an order tie the same way the chat writer
	// does (tree.compareNodes): row kind, then creation time, then id.
	rank      int
	createdAt time.Time
}

// move names a row that has to be written back, by its position in the caller's
// slice and the order it must now carry.
type move struct {
	at    int
	order int
}

// place renumbers slots densely, optionally reinserting id at target, and returns only rows whose order
// changed. The (order, id) read order keeps all-zero migrated rows stable between identical requests;
// target is clamped so a stale index lands at an end.
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

// sortSlots is place()'s (order, id) rule, shared with finalIndexOf so both sort identically. Mutates
// slots in place.
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

// finalIndexOf returns id's dense index under place()'s sort and reinsert, or -1. place() reports only
// order changes, so a reparented row landing on its old index (0 in a new container) needs this to get
// its parent written. Works on a copy.
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

// nodeIndex builds place()'s []slot input from a repo-kind sibling space read off Node.
func nodeIndex(
	rows []dtree.Node,
) []slot {
	slots := make([]slot, 0, len(rows))
	for i, row := range rows {
		slots = append(slots, slot{at: i, id: row.ID, order: row.Order, rank: rankOfKind(row.Kind)})
	}
	return slots
}

// rankOfKind is tree.Node.Rank for a Node kind.
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
