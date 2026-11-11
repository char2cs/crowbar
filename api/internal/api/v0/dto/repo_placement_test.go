package dto_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// positions answers each repo's Node row; a repo in broken fails the read the
// way its project's unreadable tree does, one with no row is not found.
func positions(rows map[string]tree.Node, broken map[string]bool) func(context.Context, string) (tree.Node, error) {
	return func(_ context.Context, id string) (tree.Node, error) {
		if broken[id] {
			return tree.Node{}, fmt.Errorf("tree: read: %w", errors.New("disk gone"))
		}
		if n, ok := rows[id]; ok {
			return n, nil
		}
		return tree.Node{}, problem.New(problem.NotFound, "no node")
	}
}

// One project's unreadable tree lists that project's repos at the root and
// leaves every other project's positions alone; it is logged once per project.
func TestPlacements_AnUnreadableTreeOnlyUnplacesItsOwnProject(t *testing.T) {
	logged := map[string]int{}
	p := dto.NewPlacements(
		positions(map[string]tree.Node{"r1": {ParentID: "f1", Order: 2}, "r3": {ParentID: "f9", Order: 1}},
			map[string]bool{"r2": true}),
		func(_ context.Context, projectID string, _ error) { logged[projectID]++ })
	rows := []domain.Repository{{ID: "r1", ProjectID: "p1"}, {ID: "r2", ProjectID: "p1"}, {ID: "r3", ProjectID: "p2"}}

	got := p.Of(context.Background(), rows)
	again := p.Of(context.Background(), rows)

	want := map[string]dto.RepoPlacement{"r1": {}, "r2": {}, "r3": {FolderID: "f9", Order: 1}}
	assert.Equal(t, want, got)
	assert.Equal(t, want, again)
	assert.Equal(t, map[string]int{"p1": 1}, logged)
}

func TestPlacements_ARepoWithNoRowYetSitsAtTheRoot(t *testing.T) {
	p := dto.NewPlacements(positions(nil, nil), func(context.Context, string, error) { t.Fatal("not a failure") })

	got := p.Of(context.Background(), []domain.Repository{{ID: "r1", ProjectID: "p1"}})

	assert.Equal(t, map[string]dto.RepoPlacement{"r1": {}}, got)
}
