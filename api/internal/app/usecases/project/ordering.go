package project

import (
	"slices"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// slot is one row of an ordered sidebar list: where it sits in the caller's
// slice, its id, and the order it currently holds.
type slot struct {
	at    int
	id    string
	order int
}

// move names a row that has to be written back, by its position in the caller's
// slice and the order it must now carry.
type move struct {
	at    int
	order int
}

// place renumbers slots into a dense 0..n-1 sequence, optionally lifting id out
// and re-inserting it at target first, and returns only the rows whose order
// actually changed — a reorder writes the moved row and the tail it displaced,
// never the whole list.
//
// Rows are read in (order, id) order. The id tiebreak is what keeps a list whose
// rows all still carry the migration default of 0 from re-shuffling itself
// between two identical requests; the first reorder replaces it with a real
// dense sequence. target is clamped, so an index computed against a stale list
// lands at an end rather than failing the request.
func place(
	slots []slot,
	id string,
	target *int,
) []move {
	slices.SortFunc(slots, func(a, b slot) int {
		if a.order != b.order {
			return a.order - b.order
		}
		return strings.Compare(a.id, b.id)
	})
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

func repoIndex(
	rows []domain.Repository,
) []slot {
	slots := make([]slot, 0, len(rows))
	for i, row := range rows {
		slots = append(slots, slot{at: i, id: row.ID, order: row.Order})
	}
	return slots
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
