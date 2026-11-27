package git

import "context"

func (e *engine) RevParse(
	ctx context.Context,
	repoPath string,
	rev string,
) (string, error) {
	return e.revParse(ctx, repoPath, rev)
}
