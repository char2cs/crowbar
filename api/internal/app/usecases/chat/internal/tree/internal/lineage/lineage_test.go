package lineage_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree/internal/lineage"
)

// rows is a whole Chats-panel tree as two lookups: what each row hangs off, and
// which rows are chats. Anything not named as a chat is a folder, which is the
// only distinction the walk makes.
type rows struct {
	parents map[string]string
	chats   map[string]bool
}

func (r rows) walk(
	id string,
) []string {
	return lineage.Walk(
		id,
		func(at string) string { return r.parents[at] },
		func(at string) bool { return r.chats[at] },
	)
}

func TestWalk_AChatAtTheRootInheritsNothing(t *testing.T) {
	tree := rows{parents: map[string]string{"c1": ""}, chats: map[string]bool{"c1": true}}

	assert.Empty(t, tree.walk("c1"))
}

func TestWalk_AChatUnderAChatReadsIt(t *testing.T) {
	tree := rows{
		parents: map[string]string{"c1": "", "c2": "c1"},
		chats:   map[string]bool{"c1": true, "c2": true},
	}

	assert.Equal(t, []string{"c1"}, tree.walk("c2"))
}

// Nearest first, because that is the order a model should read them in: the
// chat this one was forked off says more about the task than its grandparent.
func TestWalk_ReturnsTheWholeChainNearestFirst(t *testing.T) {
	tree := rows{
		parents: map[string]string{"c1": "", "c2": "c1", "c3": "c2"},
		chats:   map[string]bool{"c1": true, "c2": true, "c3": true},
	}

	assert.Equal(t, []string{"c2", "c1"}, tree.walk("c3"))
}

// A folder holds no turns, so it cannot be inherited and cannot BLOCK
// inheritance. This is the property the whole feature turns on: filing a thread
// away is organisation, and organisation must never change what an agent reads.
func TestWalk_FoldersAreTransparent(t *testing.T) {
	tree := rows{
		parents: map[string]string{"c1": "", "f1": "c1", "f2": "f1", "c2": "f2"},
		chats:   map[string]bool{"c1": true, "c2": true},
	}

	assert.Equal(t, []string{"c1"}, tree.walk("c2"),
		"a thread two folders deep under a chat reads exactly what it would read sitting directly under it")
}

// The same tree with the folders taken out must answer identically — asserted
// against the OTHER shape rather than against a literal, so the two can never be
// updated apart.
func TestWalk_FilingAThreadChangesNothingAboutWhatItReads(t *testing.T) {
	filed := rows{
		parents: map[string]string{"c1": "", "f1": "c1", "f2": "f1", "c2": "f2"},
		chats:   map[string]bool{"c1": true, "c2": true},
	}
	direct := rows{
		parents: map[string]string{"c1": "", "c2": "c1"},
		chats:   map[string]bool{"c1": true, "c2": true},
	}

	assert.Equal(t, direct.walk("c2"), filed.walk("c2"))
}

// A folder at the root is not a chat ancestor, so a chat merely filed somewhere
// inherits nothing at all and must be spawned exactly like an unfiled one.
func TestWalk_AChatFiledInARootFolderInheritsNothing(t *testing.T) {
	tree := rows{
		parents: map[string]string{"f1": "", "c1": "f1"},
		chats:   map[string]bool{"c1": true},
	}

	assert.Empty(t, tree.walk("c1"))
}

// A parent id naming a row the tree does not hold — a deleted folder whose
// children were not renumbered, say — ends the walk rather than being invented.
func TestWalk_StopsAtAParentNothingHolds(t *testing.T) {
	tree := rows{
		parents: map[string]string{"c2": "gone"},
		chats:   map[string]bool{"c2": true},
	}

	assert.Empty(t, tree.walk("c2"))
}

// A cycle is unrepresentable through the usecase's guards. If one ever exists
// anyway, the answer is a bounded walk and not a hung daemon.
func TestWalk_ACycleTerminates(t *testing.T) {
	tree := rows{
		parents: map[string]string{"c1": "c2", "c2": "c1"},
		chats:   map[string]bool{"c1": true, "c2": true},
	}

	assert.Equal(t, []string{"c2"}, tree.walk("c1"),
		"the walk stops when it comes back round, and never collects the chat it started from")
}

// A chat parented to ITSELF is the degenerate cycle, and the seed of the visited
// set is what stops it: the walk never revisits the row it started from.
func TestWalk_AChatParentedToItselfInheritsNothing(t *testing.T) {
	tree := rows{parents: map[string]string{"c1": "c1"}, chats: map[string]bool{"c1": true}}

	assert.Empty(t, tree.walk("c1"))
}
