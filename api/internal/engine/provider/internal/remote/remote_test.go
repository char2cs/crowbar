package remote

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHelperProcess is the subprocess used by fakeExecFn.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	fmt.Fprint(os.Stdout, args[len(args)-2])
	if args[len(args)-1] != "0" {
		os.Exit(1)
	}
	os.Exit(0)
}

func fakeExecFn(
	output string,
	exitCode int,
) ExecCommandFn {
	return func(
		ctx context.Context,
		name string,
		args ...string,
	) *exec.Cmd {
		exitStr := "0"
		if exitCode != 0 {
			exitStr = fmt.Sprintf("%d", exitCode)
		}
		cs := []string{"-test.run=TestHelperProcess", "--", output, exitStr}
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	}
}

func TestSlug_Success(t *testing.T) {
	dir := t.TempDir()
	slug, err := Slug(
		context.Background(),
		dir,
		fakeExecFn("https://github.com/owner/repo.git", 0),
	)
	require.NoError(t, err)
	assert.Equal(t, "owner/repo", slug)
}

func TestSlug_CommandError(t *testing.T) {
	dir := t.TempDir()
	_, err := Slug(
		context.Background(),
		dir,
		fakeExecFn("", 1),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote: get-url")
}

func TestSlug_InvalidURL(t *testing.T) {
	dir := t.TempDir()
	_, err := Slug(
		context.Background(),
		dir,
		fakeExecFn("totally-invalid-url", 0),
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
