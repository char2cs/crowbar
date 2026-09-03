package domain

type AgentTerminalWait struct {
	Waiting bool

	Kind string
}

const (
	AgentTerminalWaitTrust = "workspace_trust"
)
