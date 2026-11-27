package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func (e *engine) CheckBranchName(ctx context.Context, repoPath, name string) error {
	if !strings.HasPrefix(name, "-") {
		r := e.exec(ctx, repoPath, "check-ref-format", "--branch", name)
		if r.ExitCode == 0 && strings.TrimSpace(r.Stdout) == name {
			return nil
		}
	}
	return &problem.Error{
		Code:    problem.InvalidArgument,
		Message: fmt.Sprintf("%q is not a valid branch name", name),
		Err:     problem.ErrInvalidArgument,
	}
}
