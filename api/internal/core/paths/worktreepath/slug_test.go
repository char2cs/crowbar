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
		{
			name: "no remote prefers the immutable path slug over the display name",
			repo: domain.Repository{RemoteURL: "", PathSlug: "widget", Name: "Renamed Repo"},
			want: "widget",
		},
		{
			name: "unparseable remote prefers the immutable path slug over the display name",
			repo: domain.Repository{RemoteURL: "not-a-url", PathSlug: "widget", Name: "Renamed"},
			want: "widget",
		},
		{
			name: "a parseable remote still outranks the path slug",
			repo: domain.Repository{
				RemoteURL: "git@github.com:char2cs/crowbar.git",
				PathSlug:  "widget",
				Name:      "Renamed",
			},
			want: "github.com/char2cs/crowbar",
		},
		{
			name: "a row imported before path slugs existed still falls back to the name",
			repo: domain.Repository{RemoteURL: "", PathSlug: "", Name: "legacy"},
			want: "legacy",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, RemoteSlug(c.repo))
		})
	}
}

func TestSeedPathSlug(t *testing.T) {
	cases := []struct {
		name      string
		remoteURL string
		repoPath  string
		want      string
	}{
		{
			name:      "a parseable remote seeds the full host/owner/repo identity",
			remoteURL: "git@github.com:char2cs/crowbar.git",
			repoPath:  "/Users/me/Projects/crowbar",
			want:      "github.com/char2cs/crowbar",
		},
		{
			name:      "no remote seeds the repo directory's own base name",
			remoteURL: "",
			repoPath:  "/Users/me/Projects/widget",
			want:      "widget",
		},
		{
			name:      "an unparseable remote seeds the repo directory's own base name",
			remoteURL: "not-a-url",
			repoPath:  "/Users/me/Projects/widget",
			want:      "widget",
		},
		{
			name:      "a trailing separator still yields the leaf directory",
			remoteURL: "",
			repoPath:  "/Users/me/Projects/widget/",
			want:      "widget",
		},
		{
			name:      "a traversal component is resolved before the leaf is taken",
			remoteURL: "",
			repoPath:  "/Users/me/Projects/widget/..",
			want:      "Projects",
		},
		{
			name:      "a path with no usable leaf seeds nothing",
			remoteURL: "",
			repoPath:  "..",
			want:      "",
		},
		{
			name:      "a dot path seeds nothing",
			remoteURL: "",
			repoPath:  ".",
			want:      "",
		},
		{
			name:      "the filesystem root seeds nothing",
			remoteURL: "",
			repoPath:  "/",
			want:      "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, SeedPathSlug(c.remoteURL, c.repoPath))
		})
	}
}
