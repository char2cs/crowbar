package conflicts_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/conflicts"
)

func BenchmarkConflictParse(b *testing.B) {
	dir := b.TempDir()

	git := func(args ...string) {
		b.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			b.Fatalf("git %v: %s", args, out)
		}
	}

	git("init", "-b", "main")
	git("config", "user.email", "bench@test.com")
	git("config", "user.name", "Bench")

	conflictFile := filepath.Join(dir, "file.txt")
	writeFile := func(content string) {
		b.Helper()
		if err := os.WriteFile(conflictFile, []byte(content), 0o644); err != nil {
			b.Fatal(err)
		}
	}

	writeFile("line1\nline2\nline3\n")
	git("add", "file.txt")
	git("commit", "-m", "base")

	git("checkout", "-b", "branch-a")
	writeFile("line1\nfrom-a\nline3\n")
	git("add", "file.txt")
	git("commit", "-m", "a")

	git("checkout", "main")
	git("checkout", "-b", "branch-b")
	writeFile("line1\nfrom-b\nline3\n")
	git("add", "file.txt")
	git("commit", "-m", "b")

	cmd := exec.Command("git", "merge", "branch-a")
	cmd.Dir = dir
	_ = cmd.Run()

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = conflicts.ParseFile(ctx, dir, "file.txt")
	}
}
