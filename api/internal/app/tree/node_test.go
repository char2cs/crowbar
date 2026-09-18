package tree_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/tree"
)

func at(
	sec int64,
) time.Time {
	return time.Unix(sec, 0).UTC()
}

func ids(
	nodes []tree.Node,
) []string {
	out := make([]string, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, node.ID)
	}
	return out
}

func TestMembers_OrdersOnTheDenseIndexFirst(t *testing.T) {
	plan := tree.New([]tree.Node{
		{ID: "c", Order: 2, CreatedAt: at(1)},
		{ID: "a", Order: 0, CreatedAt: at(3)},
		{ID: "b", Order: 1, CreatedAt: at(2)},
	})

	assert.Equal(t, []string{"a", "b", "c"}, ids(plan.Members("")))
}

// Every row a user has never dragged carries 0, so creation time is what keeps
// an untouched level in a fixed sequence instead of reshuffling per request.
func TestMembers_FallsBackToCreationTimeWhenIndicesTie(t *testing.T) {
	plan := tree.New([]tree.Node{
		{ID: "late", CreatedAt: at(9)},
		{ID: "early", CreatedAt: at(1)},
	})

	assert.Equal(t, []string{"early", "late"}, ids(plan.Members("")))
}

func TestMembers_FallsBackToTheIDWhenTimestampsTie(t *testing.T) {
	same := at(42)
	plan := tree.New([]tree.Node{
		{ID: "b", CreatedAt: same},
		{ID: "a", CreatedAt: same},
	})

	assert.Equal(t, []string{"a", "b"}, ids(plan.Members("")))
}

// A caller with no creation time at all still gets a deterministic level: the
// zero value ties among itself and falls straight through to the id.
func TestMembers_SortsUndatedRowsAheadOfDatedOnes(t *testing.T) {
	plan := tree.New([]tree.Node{
		{ID: "dated", CreatedAt: at(1)},
		{ID: "undated"},
	})

	assert.Equal(t, []string{"undated", "dated"}, ids(plan.Members("")))
}

// The sidebar draws a tied level folders, then branches, then chats, with a
// project's repo headers after its home rows — a densify that counted chats
// ahead of branches (or repos ahead of chats) put the first drop slots off.
func TestMembers_BreaksATieByKindInTheSidebarsDrawnSequence(t *testing.T) {
	plan := tree.New([]tree.Node{
		{ID: "chat", Rank: tree.RankChat, CreatedAt: at(1)},
		{ID: "workspace", Rank: tree.RankWorkspace, CreatedAt: at(2)},
		{ID: "folder", Rank: tree.RankFolder, CreatedAt: at(3)},
		{ID: "repo", Rank: tree.RankRepo, CreatedAt: at(4)},
	})

	assert.Equal(t, []string{"folder", "workspace", "chat", "repo"}, ids(plan.Members("")))
}
