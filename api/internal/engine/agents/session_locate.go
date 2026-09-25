package agents

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/sessionstore"

// SessionExists implements Agent.
func (a *agent) SessionExists(sessionID string) (exists, declared bool) {
	finder := a.sessions
	if finder == nil {
		finder = sessionstore.New()
	}
	return finder.Exists(a.spec.ID, a.spec.Session.Locate, sessionID)
}
