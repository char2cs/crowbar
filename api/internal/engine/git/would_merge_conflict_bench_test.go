package git_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// BenchmarkWouldMergeConflicts is the shape a chat list asks: many branches
// each dry-run against one parent, repeatedly, while no branch moves and the
// refs are no longer fresh.
func BenchmarkWouldMergeConflicts(b *testing.B) {
	ctx := context.Background()
	dir := initRepo(b)
	makeCommit(b, dir, "base.txt", "base\n", "base")
	var pairs []git.MergePair
	for i := range 10 {
		name := fmt.Sprintf("feature-%d", i)
		gitRun(b, dir, "checkout", "-b", name, "main")
		makeCommit(b, dir, name+".txt", "x\n", name)
		pairs = append(pairs, git.MergePair{Ours: "main", Theirs: name})
	}
	gitRun(b, dir, "checkout", "main")
	agedRefs(b, dir)
	e := git.New()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		e.WouldMergeConflicts(ctx, dir, pairs)
	}
}
