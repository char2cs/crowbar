package worktreepath

import "testing"

func BenchmarkStorageDir(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = StorageDir("/crow", "proj-1", "ws-abc")
	}
}
