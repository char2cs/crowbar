//go:build integration

package tests

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegression_TopLevelChatPastTiedRepos_LandsWhereIndicated(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	var home struct {
		OwningChatID string `json:"owningChatId"`
	}
	h.get("/v0/projects/"+p+"/home", &home)

	var repos []string
	inverted := -1
	for range 8 {
		addSecondRepo(t, h, p)
		var listed []struct {
			ID string `json:"id"`
		}
		h.get("/v0/projects/"+p+"/repos", &listed)
		repos = repos[:0]
		for _, r := range listed {
			repos = append(repos, r.ID)
		}
		inverted = slices.IndexFunc(repos[:len(repos)-1], func(id string) bool {
			return id > repos[slices.Index(repos, id)+1]
		})
		if inverted >= 0 {
			break
		}
	}
	require.GreaterOrEqual(t, inverted, 0, "need an import-order/id-order inversion: %v", repos)
	rA := repos[inverted]

	c1 := createHomeChatStub(t, h, p)
	c2 := createHomeChatStub(t, h, p)

	ctx := context.Background()
	for _, id := range append([]string{c1, c2}, repos...) {
		require.NoError(t, h.app.Repositories.Placement.SetOrder(ctx, id, 0))
	}
	h.Quiesce()

	var listed []struct {
		ID string `json:"id"`
	}
	h.get("/v0/projects/"+p+"/repos", &listed)
	displayed := make([]string, 0, len(listed))
	for _, r := range listed {
		displayed = append(displayed, r.ID)
	}
	require.NotEqual(t, repos, displayed, "the tie must have re-sequenced the listing by id")

	rest := append([]string{c1}, displayed...)
	at := slices.Index(rest, rA) + 1
	want := slices.Insert(slices.Clone(rest), at, c2)

	h.patch("/v0/projects/"+p+"/home/chats/"+c2+"/placement", map[string]any{"parentId": "", "order": at}, nil)
	h.Quiesce()
	rows := topLevel(t, h, p, home.OwningChatID)
	t.Logf("c2->%d: %+v", at, rows)
	assertDense(t, rows)
	assert.Equal(t, want, idsOf(rows), "chat must land after the repo the drop line was drawn on, and untouched repos must keep their drawn order")
}
