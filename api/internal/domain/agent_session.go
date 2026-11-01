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
	// AgentExitMoved: the CLI moved the chat to another session (/clear, /resume).
	AgentExitMoved = "moved"
	// AgentExitDisplaced: Crowbar ended the CLI to replace it (a switch, a
	// restart) or because its chat was erased.
	AgentExitDisplaced = "displaced"
	// AgentExitRetracted: the prompt the CLI was launched to carry was withdrawn.
	AgentExitRetracted = "retracted"
)
