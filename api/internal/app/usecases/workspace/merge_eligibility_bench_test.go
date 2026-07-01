package workspace_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func BenchmarkMergeEligibilityFor_LargeSiblingSet(b *testing.B) {
	uc := workspace.New(nil, nil, nil)

	const n = 1000
	siblings := make([]domain.Workspace, 0, n)
	for i := 0; i < n; i++ {
		siblings = append(
			siblings,
			domain.Workspace{
				ID:     "s" + strconv.Itoa(i),
				Branch: "branch-" + strconv.Itoa(i),
				Status: domain.WorkspaceStatusNew,
			},
		)
	}

	// Parent sits at the tail so the scan walks the whole slice.
	ws := domain.Workspace{ID: "child", ParentID: "s" + strconv.Itoa(n-1)}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uc.MergeEligibilityFor(context.Background(), ws, siblings)
	}
}
