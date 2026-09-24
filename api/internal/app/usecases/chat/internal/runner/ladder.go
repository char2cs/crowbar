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

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// resumeProbeWindow is how soon after launch a resuming CLI must have
// announced a session: one that dies unannounced within it refused the resume
// (a vendor CLI exits within a second or two when it cannot load a session).
const resumeProbeWindow = 30 * time.Second

// redeliverBound caps the next-rung delivery of a refused prompt: the gate
// wait plus a fresh spawn.
const redeliverBound = 2 * time.Minute

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

// refusedDelivery fails the prompt runner's launch carried, which its CLI
// provably never read (it refused the resume before announcing a session), and
// returns it for the next rung.
func (rs *Runners) refusedDelivery(ctx context.Context, runner engineagents.Runner) (agentjournal.PromptRequest, bool) {
	dir, err := rs.promptJournalDirFor(runner.CurrentChatID)
	if err != nil || runner.CurrentChatID == "" {
		return agentjournal.PromptRequest{}, false
	}
	record, found, err := rs.prompts.ActiveForRunner(dir, runner.ID, runner.ProviderID)
	if err != nil || !found {
		return agentjournal.PromptRequest{}, false
	}
	if err := rs.prompts.MarkRefused(dir, record.RequestID, time.Now()); err != nil {
		slog.WarnContext(ctx, "agent: resume ladder: fail refused delivery", "chat_id", runner.CurrentChatID, "err", err)
		return agentjournal.PromptRequest{}, false
	}
	return record, true
}

// redeliverRefused sends a refused launch's prompt again under the same
// request id. The refused session is quarantined, so the revive this send
// performs lands on the transcript rung: the conversation continues.
func (rs *Runners) redeliverRefused(ctx context.Context, chatID string, record agentjournal.PromptRequest) {
	rs.background.run(ctx, func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, redeliverBound)
		defer cancel()
		park, release, err := rs.spawns.Acquire(ctx, chatID)
		if err != nil {
			return // a Stop or delete preempted it: the prompt stays failed, retryable
		}
		defer release()
		revive := func() error { return rs.reviveForDelivery(ctx, park, chatID) }
		if _, err := rs.submitPromptLocked(ctx, chatID, record.Text, record.RequestID, revive); err != nil {
			slog.WarnContext(ctx, "agent: resume ladder: redeliver refused prompt",
				"chat_id", chatID, "client_request_id", record.RequestID, "err", err)
		}
	})
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
