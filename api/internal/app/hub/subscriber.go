package hub

import "github.com/char2cs/crowbar/api/internal/domain"

// Subscriber receives hub broadcasts. Implemented by the API WS handler set.
type Subscriber interface {
	PushWorkspace(
		ws domain.Workspace,
	)
	PushChat(
		evt ChatStatusEvent,
	)
	PushGit(
		wsID string,
		status domain.GitStatus,
	)
	PushFile(
		evt domain.FileChangeEvent,
	)
}
