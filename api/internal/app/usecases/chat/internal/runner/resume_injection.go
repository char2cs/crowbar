package runner

import (
	"strings"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// resumeInjectionSteps turns a native session id into the argv/inject steps
// that ask a freshly-spawned CLI to resume it — used by both a prompt's
// restart-to-deliver path (prompts.go) and a provider switch (switch.go).
func resumeInjectionSteps(d engineagents.Agent, sessionID string) []engineagents.InjectStep {
	arg, ok := d.ResumeArg()
	if sessionID == "" || !ok {
		return nil
	}
	ctx := engineagents.TemplateCtx{ID: sessionID}
	parts := strings.Fields(engineagents.Expand(arg, ctx))
	steps := make([]engineagents.InjectStep, 0, len(parts))
	for _, part := range parts {
		steps = append(steps, engineagents.InjectStep{
			Verb: "pass_arg",
			Args: map[string]any{"positional": part},
		})
	}
	return steps
}

// apiOwnsResume reports whether d DECLARES that resuming its session is
// applyAPITransport's job (codex.yaml's prompt.resume: thread/resume), never the
// redundant hooks-only PTY's (see codex.yaml's own comment on subagent_pre for
// why that PTY exists at all). Handing that SAME session id to the PTY too — as a
// native `resume {id}` argv, or as the resume context pointer, which for a
// provider whose only resume channel is a user message IS a prompt the PTY
// will act on — makes it a SECOND writer on a thread the api connection
// already holds. codex enforces one writer per thread (a thread-writer-lock
// file, confirmed on disk — the same constraint the header comment documents
// for --remote attach): confirmed live, the native `resume {id}` case exits
// immediately (exit code 1), onRunnerExit reconciles the whole runner as
// dead, and the switch that looked like it succeeded silently reverts.
// Confirmed live also: WITHOUT the id but WITH the pointer, the PTY no longer
// collides — it just answers the pointer itself, as its own genuine first
// turn, and that reply lands on this chat as a second, disconnected "codex"
// conversation the api connection knows nothing about. So both must be
// withheld from this PTY, not just the id: with neither, it starts idle (like
// today's already-accepted fresh-spawn "own unrelated conversation" gap) and
// never announces anything for resumableConversation to trip over.
//
// The api connection resuming silently, with no gap handed to it either, is
// the SAME known gap this leaves in place: codex.yaml declares an inject: at:
// context step (thread/inject_items) for exactly this, and nothing calls it
// yet. Recorded, not silently papered over — see the mixed-transport design
// spec.
//
// DECLARED CAPABILITY ONLY — never evidence that a connection exists. Every
// hazard above needs a FIRST writer to collide with, and there is none when the
// connection is absent: `serve` never forked, its socket never appeared, the
// handshake or thread/resume was refused (design spec §2.2b: the session then
// runs over hooks alone), or the transport is switched off for this process.
// The PTY is then the only codex there is, and withholding `resume {id}` from
// it makes it mint a BRAND NEW thread every restart and every switch back —
// the chat's own conversation silently abandoned. So this answers only half the
// question; apiResumes asks the other half.
func apiOwnsResume(d engineagents.Agent) bool {
	return d.TransportFor("prompt") == "api" && !d.Capabilities().Hotswap
}

// apiResumes is the whole question: this descriptor declares the api transport
// owns resuming AND runnerID's connection is live RIGHT NOW
// (HasLiveAPIConnection, apiconn.go — the same runtime check turn/ingest.go
// already pairs with its own TransportFor read). Only then is there a second
// writer for the PTY to collide with.
//
// Answerable only AFTER applyAPITransport has run, which is why spawnRunner
// establishes the connection before it renders the spawn argv rather than after.
func (rs *Runners) apiResumes(d engineagents.Agent, runnerID string) bool {
	return apiOwnsResume(d) && rs.HasLiveAPIConnection(runnerID)
}
