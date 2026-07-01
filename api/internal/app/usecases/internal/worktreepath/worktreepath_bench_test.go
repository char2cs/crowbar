package worktreepath

import "testing"

func BenchmarkFor(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = For("/crow", "proj-1", "repo-1", "ws-abc")
	}
}

func BenchmarkStorageDir(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = StorageDir("/crow", "proj-1", "repo-1", "ws-abc")
	}
}
