package domain

const (
	AgentExitStopped           = "stopped"
	AgentExitExited            = "exited"
	AgentExitConnectionLost    = "connection_lost"
	AgentExitTransportOverflow = "transport_overflow"
	AgentExitDaemonRestart     = "daemon_restart"
	AgentExitResumeFailed      = "resume_failed"
	AgentExitSpawnFailed       = "spawn_failed"
	AgentExitDisplaced         = "displaced"
	AgentExitRetracted         = "retracted"
	AgentExitMoved             = "moved"
	AgentExitRebound           = "rebound"
)
