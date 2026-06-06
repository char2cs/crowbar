package git

import "context"

// RevParse resolves rev to a full commit SHA (07 §1 / §3.1). It is the
// exported counterpart of the internal revParse helper and does not take the
// repo lock — rev-parse is a read-only operation.
func (e *engine) RevParse(
	ctx context.Context,
	repoPath string,
	rev string,
) (string, error) {
	return e.revParse(ctx, repoPath, rev)
}
