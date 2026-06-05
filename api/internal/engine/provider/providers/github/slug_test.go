package github

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlug_Success(t *testing.T) {
	dir := t.TempDir()
	s, err := slug(
		context.Background(),
		dir,
		fakeCmd("https://github.com/owner/repo.git", 0),
	)
	require.NoError(t, err)
	assert.Equal(t, "owner/repo", s)
}

func TestSlug_CommandError(t *testing.T) {
	dir := t.TempDir()
	_, err := slug(
		context.Background(),
		dir,
		fakeCmd("", 1),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote: get-url")
}

func TestSlug_InvalidURL(t *testing.T) {
	dir := t.TempDir()
	_, err := slug(
		context.Background(),
		dir,
		fakeCmd("totally-invalid-url", 0),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote: slug")
}

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
