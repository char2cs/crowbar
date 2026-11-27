package lineage_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree/internal/lineage"
)

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

func TestWalk_ReturnsTheWholeChainNearestFirst(t *testing.T) {
	tree := rows{
		parents: map[string]string{"c1": "", "c2": "c1", "c3": "c2"},
		chats:   map[string]bool{"c1": true, "c2": true, "c3": true},
	}

	assert.Equal(t, []string{"c2", "c1"}, tree.walk("c3"))
}

func TestWalk_FoldersAreTransparent(t *testing.T) {
	tree := rows{
		parents: map[string]string{"c1": "", "f1": "c1", "f2": "f1", "c2": "f2"},
		chats:   map[string]bool{"c1": true, "c2": true},
	}

	assert.Equal(t, []string{"c1"}, tree.walk("c2"),
		"a thread two folders deep under a chat reads exactly what it would read sitting directly under it")
}

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

func TestWalk_AChatFiledInARootFolderInheritsNothing(t *testing.T) {
	tree := rows{
		parents: map[string]string{"f1": "", "c1": "f1"},
		chats:   map[string]bool{"c1": true},
	}

	assert.Empty(t, tree.walk("c1"))
}

func TestWalk_StopsAtAParentNothingHolds(t *testing.T) {
	tree := rows{
		parents: map[string]string{"c2": "gone"},
		chats:   map[string]bool{"c2": true},
	}

	assert.Empty(t, tree.walk("c2"))
}

func TestWalk_ACycleTerminates(t *testing.T) {
	tree := rows{
		parents: map[string]string{"c1": "c2", "c2": "c1"},
		chats:   map[string]bool{"c1": true, "c2": true},
	}

	assert.Equal(t, []string{"c2"}, tree.walk("c1"),
		"the walk stops when it comes back round, and never collects the chat it started from")
}

func TestWalk_AChatParentedToItselfInheritsNothing(t *testing.T) {
	tree := rows{parents: map[string]string{"c1": "c1"}, chats: map[string]bool{"c1": true}}

	assert.Empty(t, tree.walk("c1"))
}
