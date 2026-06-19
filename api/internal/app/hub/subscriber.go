package hub

import (
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Subscriber receives hub broadcasts. Implemented by the API WS handler set.
type Subscriber interface {
	PushWorkspace(
		ws domain.Workspace,
	)
	PushGit(
		wsID string,
		status gitdomain.GitStatus,
	)
	PushFile(
		evt domain.FileChangeEvent,
	)
}
