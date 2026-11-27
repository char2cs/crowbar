package branchreview

import (
	"golang.org/x/sync/singleflight"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func SharedScope(
	base string,
	files []gitdomain.ReviewFileSummary,
) (gitdomain.ReviewScope, error) {
	read, err := sharedScope(singleflight.Result{
		Val: scopeRead{scope: gitdomain.ReviewScope{Base: base, Files: files}},
	})
	return read.scope, err
}
