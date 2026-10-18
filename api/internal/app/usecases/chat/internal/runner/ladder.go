// Package runner (file ladder.go) is the resume ladder: how a replacement CLI
// continues a chat's conversation. First rung, the provider's own recorded
// session — used only once the provider is known to still have it. Second,
// a fresh session handed Crowbar's own transcript. A chat with history
// therefore always continues; the user never has to fork a thread to escape
// a session the provider lost (sessions spec §2.2).
package runner

import (
	"context"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// resumeProbeWindow is how soon after launch a resuming CLI must have
// announced a session: one that dies unannounced within it refused the resume
// (a vendor CLI exits within a second or two when it cannot load a session).
const resumeProbeWindow = 30 * time.Second

// verifiedResume keeps sessionID only if the provider can still resume it:
// not quarantined by an earlier failed launch, and present where the
// descriptor says the provider keeps it (a descriptor declaring no location
// is trusted). "" drops the ladder to the transcript rung.
func (rs *Runners) verifiedResume(
	ctx context.Context, d engineagents.Agent, chatID, sessionID string,
) string {
	if sessionID == "" {
		return ""
	}
	if rs.sessions.isQuarantined(chatID, sessionID) {
		slog.InfoContext(ctx, "agent: resume ladder: session failed to resume before; continuing from Crowbar's transcript",
			"chat_id", chatID, "provider", d.ID(), "session_id", sessionID)
		return ""
	}
	if exists, declared := d.SessionExists(sessionID); declared && !exists {
		slog.InfoContext(ctx, "agent: resume ladder: provider no longer has the session; continuing from Crowbar's transcript",
			"chat_id", chatID, "provider", d.ID(), "session_id", sessionID)
		return ""
	}
	return sessionID
}

// ladderTarget is resumeTarget passed through the ladder: lost reports that
// the recorded session was dropped, so the restart must carry the transcript.
func (rs *Runners) ladderTarget(
	ctx context.Context, chatID string, live engineagents.Runner, d engineagents.Agent,
) (resuming bool, sessionID string, lost bool, err error) {
	resuming, sessionID, err = rs.resumeTarget(ctx, chatID, live)
	if err != nil || !resuming {
		return resuming, sessionID, false, err
	}
	if rs.verifiedResume(ctx, d, chatID, sessionID) == "" {
		return false, "", true, nil
	}
	return true, sessionID, false, nil
}

// transcriptFor is the transcript rung's hand-over: the conversation so far,
// as a provider new to the chat is given it. Best effort — "" on failure.
func (rs *Runners) transcriptFor(ctx context.Context, chatID string) string {
	doc, err := rs.conversations.AssembleConversation(ctx, chatID, false, time.Time{})
	if err != nil {
		slog.WarnContext(ctx, "agent: resume ladder: assemble transcript (continuing without it)",
			"chat_id", chatID, "err", err)
		return ""
	}
	return doc
}

// spawnRung is the rung spawnRunner's runner actually launched on: the api
// connection may have replaced the session it was asked to resume.
func (rs *Runners) spawnRung(runnerID string, resuming bool, conversation string) string {
	if conn, ok := rs.apiConns.get(runnerID); ok && conn.replacedSession {
		return domain.AgentRungTranscript
	}
	return launchRung(resuming, conversation)
}

// watchResume arms the probe that catches a resume the provider refuses at
// launch. Only a launch that carries a prompt is watched: its CLI must
// announce a session to take the prompt, so silence before an exit is proof.
func (rs *Runners) watchResume(runnerID, chatID, sessionID, promptMessage string) {
	if sessionID == "" || promptMessage == "" {
		return
	}
	rs.sessions.probe(runnerID, resumeProbe{chatID: chatID, sessionID: sessionID, at: time.Now()})
}
