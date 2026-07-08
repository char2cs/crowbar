package worktreepath

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestRemoteSlug(t *testing.T) {
	cases := []struct {
		name string
		repo domain.Repository
		want string
	}{
		{
			name: "ssh remote",
			repo: domain.Repository{RemoteURL: "git@github.com:char2cs/crowbar.git"},
			want: "github.com/char2cs/crowbar",
		},
		{
			name: "https remote",
			repo: domain.Repository{RemoteURL: "https://github.com/char2cs/crowbar"},
			want: "github.com/char2cs/crowbar",
		},
		{
			name: "https remote with .git suffix",
			repo: domain.Repository{RemoteURL: "https://github.com/char2cs/crowbar.git"},
			want: "github.com/char2cs/crowbar",
		},
		{
			name: "ssh url scheme with nested group",
			repo: domain.Repository{RemoteURL: "ssh://git@gitlab.com/group/sub/repo.git"},
			want: "gitlab.com/group/sub/repo",
		},
		{
			name: "host with port",
			repo: domain.Repository{RemoteURL: "https://git.example.com:8443/team/app.git"},
			want: "git.example.com/team/app",
		},
		{
			name: "no remote falls back to repo name",
			repo: domain.Repository{RemoteURL: "", Name: "localrepo"},
			want: "localrepo",
		},
		{
			name: "unparseable remote falls back to repo name",
			repo: domain.Repository{RemoteURL: "not-a-url", Name: "localrepo"},
			want: "localrepo",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, RemoteSlug(c.repo))
		})
	}
}
