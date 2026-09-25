package domain

import "time"

// AgentSession is why a chat's vendor session is in the state it is, as the
// daemon's session supervisor recorded it — so a client renders it and never
// infers it.
type AgentSession struct {
	// Rung is how the live runner continued the conversation (AgentRung*);
	// empty while dormant.
	Rung string
	// ExitReason is how the chat's last runner ended (AgentExit*); empty while
	// a runner is live or when none has run since the daemon started.
	ExitReason string
	ExitedAt   time.Time
}

// Resume ladder rungs, in the order the supervisor tries them.
const (
	// AgentRungSession resumed the provider's own recorded session.
	AgentRungSession = "session"
	// AgentRungTranscript started a fresh provider session handed Crowbar's
	// own transcript, because the recorded one was unavailable.
	AgentRungTranscript = "transcript"
	// AgentRungFresh had no conversation to continue.
	AgentRungFresh = "fresh"
)

// Runner exit reasons.
const (
	AgentExitStopped           = "stopped"
	AgentExitExited            = "exited"
	AgentExitConnectionLost    = "connection_lost"
	AgentExitTransportOverflow = "transport_overflow"
	AgentExitDaemonRestart     = "daemon_restart"
	AgentExitResumeFailed      = "resume_failed"
	AgentExitSpawnFailed       = "spawn_failed"
	// AgentExitMoved: the CLI moved to another conversation (/clear, /resume).
	AgentExitMoved = "moved"
	// AgentExitDisplaced: Crowbar took the CLI off the chat (another runner
	// took its conversation, or a switch whose replacement never came up).
	AgentExitDisplaced = "displaced"
)
