package domain

// Runner exit reasons: how a chat's last runner ended, as its ledger records it.
const (
	AgentExitStopped           = "stopped"
	AgentExitExited            = "exited"
	AgentExitConnectionLost    = "connection_lost"
	AgentExitTransportOverflow = "transport_overflow"
	AgentExitDaemonRestart     = "daemon_restart"
	AgentExitResumeFailed      = "resume_failed"
	AgentExitSpawnFailed       = "spawn_failed"
	// AgentExitDisplaced: Crowbar ended the CLI to replace it (a switch, a
	// restart) or because its chat was erased.
	AgentExitDisplaced = "displaced"
	// AgentExitRetracted: the prompt the CLI was launched to carry was withdrawn.
	AgentExitRetracted = "retracted"
	// AgentExitMoved: its conversation was resumed in another chat, which
	// took the session.
	AgentExitMoved = "moved"
	// AgentExitRebound: its chat moved to another folder; the next launch
	// resumes the same session there.
	AgentExitRebound = "rebound"
)
