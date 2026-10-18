package domain

import "time"

// AgentSession is why a chat's vendor session is in the state it is, as the
// daemon's session supervisor recorded it — so a client renders it and never
// infers it.
type AgentSession struct {
	// ExitReason is how the chat's last runner ended (AgentExit*); empty while
	// a runner is live or when none has run since the daemon started.
	ExitReason string
	ExitedAt   time.Time
}

// Runner exit reasons.
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
	// AgentExitDisplaced: Crowbar took the CLI off the chat (another runner
	// was placed on it, or a switch whose replacement never came up).
	AgentExitDisplaced = "displaced"
)
