package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlugFromURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "HTTPS with .git suffix",
			url:  "https://github.com/owner/repo.git",
			want: "owner/repo",
		},
		{
			name: "HTTPS without .git suffix",
			url:  "https://github.com/owner/repo",
			want: "owner/repo",
		},
		{
			name: "SSH with .git suffix",
			url:  "git@github.com:owner/repo.git",
			want: "owner/repo",
		},
		{
			name: "SSH without .git suffix",
			url:  "git@github.com:owner/repo",
			want: "owner/repo",
		},
		{
			name: "GitLab HTTPS",
			url:  "https://gitlab.com/group/subgroup/repo.git",
			want: "group/subgroup/repo",
		},
		{
			name:    "Unrecognised format",
			url:     "totally-invalid",
			wantErr: true,
		},
		{
			name:    "Local path remote",
			url:     "/srv/remotes/github.com/owner/repo.git",
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := slugFromURL(c.url)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}
