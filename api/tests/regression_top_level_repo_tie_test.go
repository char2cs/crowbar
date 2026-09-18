//go:build integration

package tests

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// K3, legacy data with TWO repos: a tied (all order 0) level is drawn by the
// sidebar's compareSidebarRows (row-order.ts) and indexed against that
// sequence, so the backend's tie-break over the rows that stay behind must be
// that same sequence. A repo Node carries no CreatedAt, so both writers fall
// through to the REPO id — and the sidebar ties a repo header on
// `repoIcon.repoId` too, never on the owning-chat id the row is drawn by.
// RepoDTOList's (order, id) sort is therefore exactly the displayed sequence,
// and this test derives `displayed` from it. Before the fix the chat landed
// beside the wrong repo and the two repos swapped.
func TestRegression_TopLevelChatPastTiedRepos_LandsWhereIndicated(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	var home struct {
		OwningChatID string `json:"owningChatId"`
	}
	h.get("/v0/projects/"+p+"/home", &home)

	// Import repos until GET .../repos lists two adjacent repos whose ids sort
	// the OTHER way round — the half of real installs where the tie-break bites.
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
	rA := repos[inverted] // listed first, larger id

	c1 := createHomeChatStub(t, h, p)
	c2 := createHomeChatStub(t, h, p)

	ctx := context.Background()
	for _, id := range append([]string{c1, c2}, repos...) {
		require.NoError(t, h.app.Repositories.Node.SetOrder(ctx, id, 0))
	}
	h.Quiesce()

	// The sidebar draws tied repos in repo-id order (compareSidebarRows ties a
	// header on repoIcon.repoId), which is GET .../repos' (order, id) sort
	// AFTER the tie — so that read is the display.
	var listed []struct {
		ID string `json:"id"`
	}
	h.get("/v0/projects/"+p+"/repos", &listed)
	displayed := make([]string, 0, len(listed))
	for _, r := range listed {
		displayed = append(displayed, r.ID)
	}
	require.NotEqual(t, repos, displayed, "the tie must have re-sequenced the listing by id")

	// Displayed: [c1, c2, displayed repos...]. Drag c2 directly AFTER rA —
	// drop-actions.ts's index over the rendered rows minus the lifted one.
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
