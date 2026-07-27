package diff_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/status"
)

// unsplitReference is the equivalence oracle for the committed/dirty split: the
// pre-split FileSummaries algorithm, kept alive here verbatim. One
// `git diff --name-status -M -z <ref> --` for the file set joined by path with
// one `git diff --numstat -M -z <ref> --` for the counts, both against the
// working tree. Its parsing is written independently of the production parsers
// so agreement between the two is evidence rather than tautology.
func unsplitReference(
	t *testing.T,
	repo string,
	ref string,
) []gitdomain.ReviewFileSummary {
	t.Helper()
	ctx := context.Background()

	nameStatus := exec.Git(ctx, repo, "diff", "--name-status", "-M", "-z", ref, "--")
	require.Equal(t, 0, nameStatus.ExitCode, nameStatus.Stderr)
	numstat := exec.Git(ctx, repo, "diff", "--numstat", "-M", "-z", ref, "--")
	require.Equal(t, 0, numstat.ExitCode, numstat.Stderr)

	entries := referenceEntries(nameStatus.Stdout)
	counts := referenceCounts(numstat.Stdout)
	for i := range entries {
		count, ok := counts[entries[i].Path]
		if !ok {
			continue
		}
		entries[i].Additions = count[0]
		entries[i].Deletions = count[1]
	}
	return entries
}

func referenceEntries(
	text string,
) []gitdomain.ReviewFileSummary {
	tokens := strings.Split(text, "\x00")
	var out []gitdomain.ReviewFileSummary
	for i := 0; i < len(tokens); i++ {
		if tokens[i] == "" {
			continue
		}
		code := tokens[i]
		if code[0] == 'R' || code[0] == 'C' {
			out = append(out, gitdomain.ReviewFileSummary{
				OldPath: tokens[i+1],
				Path:    tokens[i+2],
				Status:  gitdomain.GitFileStatusRenamed,
			})
			i += 2
			continue
		}
		out = append(out, gitdomain.ReviewFileSummary{
			Path:   tokens[i+1],
			Status: referenceStatus(code[0]),
		})
		i++
	}
	return out
}

func referenceStatus(
	code byte,
) gitdomain.GitFileStatus {
	switch code {
	case 'A':
		return gitdomain.GitFileStatusAdded
	case 'D':
		return gitdomain.GitFileStatusDeleted
	default:
		return gitdomain.GitFileStatusModified
	}
}

func referenceCounts(
	text string,
) map[string][2]int {
	tokens := strings.Split(text, "\x00")
	out := make(map[string][2]int, len(tokens))
	for i := 0; i < len(tokens); i++ {
		fields := strings.SplitN(tokens[i], "\t", 3)
		if len(fields) < 3 {
			continue
		}
		count := [2]int{referenceCount(fields[0]), referenceCount(fields[1])}
		if fields[2] != "" {
			out[fields[2]] = count
			continue
		}
		out[tokens[i+2]] = count
		i += 2
	}
	return out
}

func referenceCount(
	s string,
) int {
	if s == "-" {
		return -1
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// dirtyPaths is exactly what the branch-review usecase threads down: the paths
// of the git status it already fetches for the uncommitted/staged flags.
func dirtyPaths(
	t *testing.T,
	repo string,
) []string {
	t.Helper()
	s, err := status.Parse(context.Background(), repo)
	require.NoError(t, err)
	out := make([]string, 0, len(s.Files))
	for _, f := range s.Files {
		out = append(out, f.Path)
	}
	return out
}

func seedEquivalenceBase(
	t *testing.T,
	repo string,
) {
	t.Helper()
	writeFile(t, repo, "keep.txt", "a\nb\nc\nd\ne\n")
	writeFile(t, repo, "mod.txt", "1\n2\n3\n4\n5\n6\n")
	writeFile(t, repo, "del.txt", "x\ny\nz\n")
	writeFile(t, repo, "ren.txt", "r1\nr2\nr3\nr4\nr5\nr6\nr7\nr8\n")
	writeFile(t, repo, "logo.bin", "\x00\x01\x02\x03\xff")
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-m", "base")
}

func commitAll(
	t *testing.T,
	repo string,
	msg string,
) {
	t.Helper()
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-m", msg)
}

// TestFileSummaries_MatchesUnsplitReference asserts the committed/dirty split
// produces summaries identical to a single ref->worktree numstat, which is what
// the implementation did before the split. Any divergence here is a
// user-visible wrong +/- count in the sidebar, so every repo shape that can
// separate the two queries is enumerated — above all the file that is both
// committed-changed AND dirty, whose true count is neither diff alone nor the
// sum of the two.
func TestFileSummaries_MatchesUnsplitReference(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, repo string)
	}{
		{
			name:  "clean tree",
			setup: func(t *testing.T, repo string) { t.Helper() },
		},
		{
			name: "committed change only",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, repo, "mod.txt", "1\n2\n3\n4\n5\n6\n7\n8\n")
				commitAll(t, repo, "committed change")
			},
		},
		{
			name: "dirty tracked file",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, repo, "mod.txt", "1\n2\nchanged\n4\n5\n6\n")
			},
		},
		{
			name: "staged file",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, repo, "mod.txt", "1\n2\nstaged\n4\n5\n6\n")
				mustGit(t, repo, "add", "mod.txt")
			},
		},
		{
			name: "untracked file",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, repo, "brand-new.txt", "n1\nn2\n")
			},
		},
		{
			name: "committed-changed AND dirty",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, repo, "mod.txt", "1\n2\n3\n4\n5\n6\n7\n8\n")
				commitAll(t, repo, "committed change")
				writeFile(t, repo, "mod.txt", "1\n2\nX\n4\n5\n6\n7\n8\n9\n10\n")
			},
		},
		{
			name: "committed-changed AND dirty AND staged",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, repo, "mod.txt", "1\n2\n3\n4\n5\n6\n7\n8\n")
				commitAll(t, repo, "committed change")
				writeFile(t, repo, "mod.txt", "1\n2\nstaged\n4\n5\n6\n7\n8\n")
				mustGit(t, repo, "add", "mod.txt")
				writeFile(t, repo, "mod.txt", "1\n2\nstaged\nthen\ndirty\n6\n7\n8\n")
			},
		},
		{
			name: "rename",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				mustGit(t, repo, "mv", "ren.txt", "renamed.txt")
				commitAll(t, repo, "rename")
			},
		},
		{
			name: "committed rename then dirty",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				mustGit(t, repo, "mv", "ren.txt", "renamed.txt")
				commitAll(t, repo, "rename")
				writeFile(t, repo, "renamed.txt", "r1\nr2\nr3\nr4\nr5\nr6\nr7\nr8\nr9\n")
			},
		},
		{
			name: "rename split across commit and worktree",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				mustGit(t, repo, "rm", "ren.txt")
				commitAll(t, repo, "delete the source")
				writeFile(t, repo, "resurrected.txt", "r1\nr2\nr3\nr4\nr5\nr6\nr7\nr8\n")
				mustGit(t, repo, "add", "resurrected.txt")
			},
		},
		// ref->HEAD detects the rename, ref->worktree does not: the rewrite
		// drops the pair below the similarity threshold. The committed half
		// therefore holds no count at all for the deleted source, and nothing
		// about the source is dirty, so only "no committed count" can flag it.
		{
			name: "committed rename undone by a worktree rewrite",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				mustGit(t, repo, "mv", "ren.txt", "renamed.txt")
				commitAll(t, repo, "rename")
				writeFile(t, repo, "renamed.txt", scaleLines("nothing in common", 30))
			},
		},
		{
			name: "binary file",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, repo, "logo.bin", "\xff\xfe\x00\x11\x22\x33")
				commitAll(t, repo, "binary change")
			},
		},
		{
			name: "binary file dirty",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, repo, "logo.bin", "\xff\xfe\x00\x11\x22\x33")
				commitAll(t, repo, "binary change")
				writeFile(t, repo, "logo.bin", "\x01\x02\x03\x04\x05\x06\x07")
			},
		},
		{
			name: "deleted file",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				mustGit(t, repo, "rm", "del.txt")
				commitAll(t, repo, "delete")
			},
		},
		{
			name: "file deleted in worktree only",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(repo, "del.txt")))
			},
		},
		{
			name: "committed change then deleted in worktree",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, repo, "del.txt", "x\ny\nz\nw\n")
				commitAll(t, repo, "extend before deleting")
				require.NoError(t, os.Remove(filepath.Join(repo, "del.txt")))
			},
		},
		{
			name: "mode change only",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				require.NoError(t, os.Chmod(filepath.Join(repo, "keep.txt"), 0o755))
			},
		},
		// git status --porcelain=v2 is parsed field-wise, so a path containing a
		// space or a byte git C-quotes does not round-trip through the dirty set
		// the split is keyed on. Such a file must still get its working-tree
		// count, not the committed one.
		{
			name: "path with a space, committed and dirty",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, repo, "with space.txt", "s1\ns2\n")
				commitAll(t, repo, "add spaced path")
				writeFile(t, repo, "with space.txt", "s1\ns2\ns3\ns4\n")
				commitAll(t, repo, "commit spaced change")
				writeFile(t, repo, "with space.txt", "s1\ns2\ns3\ns4\ns5\ns6\n")
			},
		},
		{
			name: "non-ASCII path, committed and dirty",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, repo, "café.txt", "u1\nu2\n")
				commitAll(t, repo, "add unicode path")
				writeFile(t, repo, "café.txt", "u1\nu2\nu3\nu4\n")
				commitAll(t, repo, "commit unicode change")
				writeFile(t, repo, "café.txt", "u1\nu2\nu3\nu4\nu5\nu6\n")
			},
		},
		{
			name: "everything at once",
			setup: func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, repo, "mod.txt", "1\n2\n3\n4\n5\n6\n7\n8\n")
				mustGit(t, repo, "mv", "ren.txt", "renamed.txt")
				mustGit(t, repo, "rm", "del.txt")
				writeFile(t, repo, "added.txt", "a1\na2\na3\n")
				writeFile(t, repo, "logo.bin", "\x09\x08\x07")
				commitAll(t, repo, "branch work")

				writeFile(t, repo, "mod.txt", "1\n2\nX\n4\n5\n6\n7\n8\n9\n")
				writeFile(t, repo, "renamed.txt", "r1\nr2\nr3\nr4\nr5\nr6\nr7\nr8\nr9\n")
				writeFile(t, repo, "added.txt", "a1\na2\na3\na4\n")
				mustGit(t, repo, "add", "added.txt")
				writeFile(t, repo, "untracked.txt", "u\n")
				require.NoError(t, os.Remove(filepath.Join(repo, "keep.txt")))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := initRepo(t)
			seedEquivalenceBase(t, repo)
			ref := headSHA(t, repo)
			tc.setup(t, repo)

			want := unsplitReference(t, repo, ref)

			// The oracle parses git's wire format itself, so pin it against the
			// production parser on the very query it reproduces before trusting
			// it to judge the split.
			unsplit, err := diff.FileSummaries(context.Background(), repo, ref, nil)
			require.NoError(t, err)
			require.Equal(t, want, unsplit, "oracle disagrees with the unsplit implementation")

			got, err := diff.FileSummaries(context.Background(), repo, ref, dirtyPaths(t, repo))
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

// TestFileSummaries_MatchesUnsplitReferenceWhenChunked forces the dirty
// pathspec across several git invocations, so the chunking that keeps a large
// dirty set under ARG_MAX is proven not to change a single count. Rename
// candidates must survive chunking: the source of a rename has to travel in the
// same invocation as its destination or git reports an unrelated add.
func TestFileSummaries_MatchesUnsplitReferenceWhenChunked(t *testing.T) {
	repo := initRepo(t)
	seedEquivalenceBase(t, repo)
	for i := range 40 {
		writeFile(t, repo, fmt.Sprintf("src/pkg%d/file%d.txt", i%5, i), scaleLines("base", 20))
	}
	commitAll(t, repo, "seed many files")
	ref := headSHA(t, repo)

	for i := range 40 {
		writeFile(t, repo, fmt.Sprintf("src/pkg%d/file%d.txt", i%5, i), scaleLines("head", 24))
	}
	mustGit(t, repo, "mv", "ren.txt", "renamed.txt")
	commitAll(t, repo, "committed churn")
	for i := range 40 {
		writeFile(t, repo, fmt.Sprintf("src/pkg%d/file%d.txt", i%5, i), scaleLines("dirty", 28))
	}
	writeFile(t, repo, "renamed.txt", "r1\nr2\nr3\nr4\nr5\nr6\nr7\nr8\nr9\nr10\n")

	restore := diff.SetPathspecChunkBytes(64)
	defer restore()

	want := unsplitReference(t, repo, ref)
	got, err := diff.FileSummaries(context.Background(), repo, ref, dirtyPaths(t, repo))
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.NotEmpty(t, got)
}

// TestFileSummaries_UnknownDirtySetRecomputesEverything covers the caller that
// could not produce a status — the split has no way to tell which counts are
// stale, so it must fall back to the single ref->worktree numstat rather than
// serve committed counts for a dirty tree.
func TestFileSummaries_UnknownDirtySetRecomputesEverything(t *testing.T) {
	repo := initRepo(t)
	seedEquivalenceBase(t, repo)
	ref := headSHA(t, repo)
	writeFile(t, repo, "mod.txt", "1\n2\n3\n4\n5\n6\n7\n8\n")
	commitAll(t, repo, "committed change")
	writeFile(t, repo, "mod.txt", "1\n2\nX\n4\n5\n6\n7\n8\n9\n10\n")

	want := unsplitReference(t, repo, ref)
	got, err := diff.FileSummaries(context.Background(), repo, ref, nil)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func scaleLines(
	salt string,
	n int,
) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "%s line %d\n", salt, i)
	}
	return b.String()
}
