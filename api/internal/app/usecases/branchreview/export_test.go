package branchreview

import (
	"golang.org/x/sync/singleflight"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// SharedScope exposes sharedScope to the external test package. Unwrapping a
// flight result for one waiter is a pure function, so the property that matters
// about it — that waiters never alias one backing array — is provable without
// racing several callers onto a live flight.
//
// It takes the flight's CONTENTS rather than the flight result itself because
// the value a flight carries is unexported (scopeRead: the scope plus the
// working-tree fact its status established), and an external test handing in a
// bare ReviewScope would type-assert to nothing and prove the opposite of what
// it claims.
func SharedScope(
	base string,
	files []gitdomain.ReviewFileSummary,
) (gitdomain.ReviewScope, error) {
	read, err := sharedScope(singleflight.Result{
		Val: scopeRead{scope: gitdomain.ReviewScope{Base: base, Files: files}},
	})
	return read.scope, err
}
