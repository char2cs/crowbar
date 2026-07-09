package git_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// advanceOrigin pushes a fresh commit to origin/main from a separate clone of
// repo's remote, leaving repo's own refs untouched until it fetches.
func advanceOrigin(
	t *testing.T,
	repo string,
) {
	t.Helper()
	bare := gitOutput(t, repo, "remote", "get-url", "origin")
	other := initRepo(t)
	gitRun(t, other, "remote", "add", "origin", bare)
	gitRun(t, other, "fetch", "origin")
	gitRun(t, other, "checkout", "main")
	makeCommit(t, other, "remote.txt", "remote\n", "remote work")
	gitRun(t, other, "push", "origin", "main")
}

func TestPullIsFastForward(
	t *testing.T,
) {
	cases := []struct {
		name  string
		setup func(t *testing.T) (repo, branch string)
		want  bool
	}{
		{
			name: "strictly behind is fast-forwardable",
			setup: func(t *testing.T) (string, string) {
				repo := initRepoWithBareOrigin(t)
				advanceOrigin(t, repo)
				gitRun(t, repo, "fetch", "origin")
				return repo, "main"
			},
			want: true,
		},
		{
			name: "diverged is not fast-forwardable",
			setup: func(t *testing.T) (string, string) {
				repo := initRepoWithBareOrigin(t)
				advanceOrigin(t, repo)
				makeCommit(t, repo, "local.txt", "local\n", "local work")
				gitRun(t, repo, "fetch", "origin")
				return repo, "main"
			},
			want: false,
		},
		{
			name: "unknown upstream ref is not fast-forwardable",
			setup: func(t *testing.T) (string, string) {
				repo := initRepo(t)
				makeCommit(t, repo, "base.txt", "base\n", "base")
				return repo, "main"
			},
			want: false,
		},
	}

	e := git.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, branch := tc.setup(t)
			ok, err := e.PullIsFastForward(context.Background(), repo, branch)
			require.NoError(t, err)
			assert.Equal(t, tc.want, ok)
		})
	}
}
