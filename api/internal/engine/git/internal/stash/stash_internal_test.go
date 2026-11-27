package stash

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func TestParseLine_TooFewParts(
	t *testing.T,
) {
	_, ok := parseLine("stash@{0}\x00only two parts")
	assert.False(t, ok)
}

func TestParseLine_EmptyID(
	t *testing.T,
) {
	_, ok := parseLine("\x00WIP on main\x002024-01-01 00:00:00 +0000\x00")
	assert.False(t, ok)
}

func TestParseLine_InvalidDate(
	t *testing.T,
) {
	_, ok := parseLine("stash@{0}\x00my stash\x00not-a-date\x00")
	assert.False(t, ok)
}

func TestParseLine_Valid(
	t *testing.T,
) {
	s, ok := parseLine("stash@{0}\x00WIP on main: abc123 msg\x002024-01-15 10:30:00 +0000\x00")
	assert.True(t, ok)
	assert.Equal(t, "stash@{0}", s.ID)
	assert.Equal(t, "WIP on main: abc123 msg", s.Message)
	assert.False(t, s.Date.IsZero())
}

func TestCountChangedFiles_RequireSuccessError(
	t *testing.T,
) {
	dir := t.TempDir()
	ctx := context.Background()
	r := exec.Git(ctx, dir, "init", "-b", "main")
	require.Equal(t, 0, r.ExitCode, r.Stderr)
	_ = exec.Git(ctx, dir, "config", "user.email", "t@t.com")
	_ = exec.Git(ctx, dir, "config", "user.name", "T")

	fp := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(fp, []byte("hello\n"), 0o600))
	_ = exec.Git(ctx, dir, "add", "f.txt")
	rr := exec.Git(ctx, dir, "commit", "-m", "init")
	require.Equal(t, 0, rr.ExitCode, rr.Stderr)

	_, err := countChangedFiles(ctx, dir, "stash@{99}")
	require.Error(t, err)
}

func listProcessRaw(
	raw string,
) []string {
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	var ids []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		s, ok := parseLine(line)
		if !ok {
			continue
		}
		ids = append(ids, s.ID)
	}
	return ids
}

func TestListProcessRaw_BlankLine(
	t *testing.T,
) {
	valid := "stash@{0}\x00msg\x002024-01-15 10:30:00 +0000\x00"
	raw := valid + "\n\n" + strings.Replace(valid, "stash@{0}", "stash@{1}", 1)
	ids := listProcessRaw(raw)
	assert.Equal(t, []string{"stash@{0}", "stash@{1}"}, ids)
}

func TestListProcessRaw_MalformedLine(
	t *testing.T,
) {
	valid := "stash@{0}\x00msg\x002024-01-15 10:30:00 +0000\x00"
	raw := valid + "\nBAD_LINE_NO_NULLS\n"
	ids := listProcessRaw(raw)
	assert.Equal(t, []string{"stash@{0}"}, ids)
}
