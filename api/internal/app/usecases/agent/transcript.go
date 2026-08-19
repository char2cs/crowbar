package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Reading what the agent ACTUALLY said, as opposed to the one message its
// terminating hook reports.
//
// THE DEFECT. claude's Stop hook carries `last_assistant_message`, and that is
// literally what it says. Asked to "send me a message, wait 30 seconds, then send
// another", claude did exactly that and Crowbar's record held ONE assistant turn:
// the second message. The first was never ingested, because no hook ever carried
// it. That record is what get_chat_log serves to sibling agents, so they were
// reading a version of the conversation with the middle of every turn missing.
//
// THE SOURCE. Every hook payload of both shipped providers names the CLI's own
// transcript, and that file has one entry PER MESSAGE. Crowbar had never opened
// it. It is opened READ-ONLY and nothing whatsoever is written into the
// provider's home — see the transcript engine package, which owns that promise.
//
// THE SHAPE OF THE RECORD. Each message becomes its own assistant turn, and each
// turn keeps the tool calls that ran in the segment ending with it. That is not a
// new convention, it is today's generalised: a turn's activity has always
// rendered under that turn's message, and with one message per turn "the turn's
// tools" and "the tools that produced this message" were the same set. Splitting
// at each message keeps every tool call attached to exactly one turn that exists,
// which is the property the UI's turnId join depends on.
//
// WHAT MAKES IT SAFE. Three rules, each of them a test:
//
//   - A provider that declares no transcript reaches none of this. Its turn
//     records exactly what it recorded before — one message, from the hook.
//
//   - A transcript that is missing, unreadable, mid-write or shaped unexpectedly
//     yields no messages, and the turn again closes on the hook's own message. The
//     record degrades to yesterday and never below it.
//
//   - A session Crowbar has not been watching starts at the file's END. Anything
//     else would replay a resumed conversation's entire history into the record as
//     though the agent had just said all of it.

// maxTranscriptMarks bounds how many transcripts a daemon remembers a position
// in. Each entry is a path and three small fields, and a long-lived daemon sees
// one per provider session, so the cap exists to make the growth bounded rather
// than because the memory matters. Evicting the oldest costs at worst one
// session's next drain starting from the file's end — which is the same safe
// degradation a daemon restart already produces.
const maxTranscriptMarks = 512

// transcriptMark is Crowbar's position in ONE provider transcript, plus what it
// last recorded from it.
//
// LastText and LastTurn are not bookkeeping for its own sake. LastText is what
// stops a message being recorded twice when the terminating hook restates a
// message the file had already given us (or gives us one the file had not yet
// flushed, which then arrives on the next read). LastTurn is the row that message
// landed on, so a turn whose every word was already recorded can still be closed
// ONTO that row — otherwise the tool calls that ran after the agent's last word
// would be left attached to a turn that was never written down.
type transcriptMark struct {
	Offset   int64
	LastText string
	LastTurn string
	// LastFromFile says the last recorded message came out of the TRANSCRIPT
	// rather than off a hook, and it is the difference between two facts that look
	// identical in the record.
	//
	// A message the file gave us and a terminating hook then restates is ONE
	// message described twice, and recording it again would double every reply. A
	// message that only ever came from a hook is not: two deliveries carrying the
	// same words are two things that happened, and Crowbar has always recorded
	// them as two — a user really can say the same thing twice, and so can an
	// agent. Without this distinction the safe fallback would silently swallow
	// every repeated reply on any provider with no transcript at all.
	LastFromFile bool
}

// transcriptMarks is the per-process registry of those positions, keyed by the
// transcript's own path.
//
// Keyed by PATH rather than by session or runner because the offset is a fact
// about a FILE. A runner can change session (a /clear mints a new transcript) and
// a session can outlive the runner that opened it (a resume), but a byte offset
// only ever means something relative to one path.
//
// In memory, deliberately. A position is only ever an optimisation over "start at
// the end": losing it costs the messages of at most one in-flight turn, which
// then fall back to the hook's own message — exactly today's behaviour. Persisting
// it would buy that one turn at the price of a durable cursor that can be wrong,
// and a wrong cursor replays somebody else's words into a conversation.
type transcriptMarks struct {
	mu    sync.Mutex
	marks map[string]transcriptMark
	order []string
}

func newTranscriptMarks() *transcriptMarks {
	return &transcriptMarks{marks: map[string]transcriptMark{}}
}

func (t *transcriptMarks) get(path string) (transcriptMark, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	mark, ok := t.marks[path]
	return mark, ok
}

// record notes what was last written to the conversation record from this
// transcript, WITHOUT touching the read position. The two are separate facts: a
// read has earned its offset the moment the bytes were consumed, whether or not
// the turn it produced was written successfully.
func (t *transcriptMarks) record(path, text, turnID string, fromFile bool) {
	if path == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	mark, ok := t.marks[path]
	if !ok {
		return
	}
	mark.LastText, mark.LastTurn, mark.LastFromFile = text, turnID, fromFile
	t.marks[path] = mark
}

func (t *transcriptMarks) set(path string, mark transcriptMark) {
	if path == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, known := t.marks[path]; !known {
		t.order = append(t.order, path)
		for len(t.order) > maxTranscriptMarks {
			delete(t.marks, t.order[0])
			t.order = t.order[1:]
		}
	}
	t.marks[path] = mark
}

// seedTranscript establishes a starting position for a transcript nobody has
// been watching, WITHOUT reading anything out of it.
//
// It runs on every hook rather than on the announcement alone, because a daemon
// that restarts mid-session never sees that session's announcement. Its cost on
// an already-watched transcript is one map read.
func (u *Usecase) seedTranscript(agent engineagents.Agent, ev engineagents.CanonicalEvent) {
	path, ok := agent.TranscriptPath(ev)
	if !ok {
		return
	}
	if _, watching := u.transcripts.get(path); watching {
		return
	}
	u.transcripts.set(path, transcriptMark{Offset: agent.TranscriptEnd(path)})
}

// beginTranscriptTurn is beginTurn addressed by hook rather than by path, so the
// turn handler never has to know a provider names its transcript at all.
func (u *Usecase) beginTranscriptTurn(agent engineagents.Agent, ev engineagents.CanonicalEvent) {
	if path, ok := agent.TranscriptPath(ev); ok {
		u.transcripts.beginTurn(path)
	}
}

// beginTurn forgets which RECORD ROW the last message landed on, without
// forgetting the message itself.
//
// The row is only meaningful within the turn that wrote it: it is what a turn
// whose every word was already recorded is closed onto, so the tools that ran
// after the agent's last word attach to the message that preceded them. Carried
// into the NEXT turn it would mean the opposite — a turn that said nothing at all
// re-closing onto a message from a turn that is already over.
func (t *transcriptMarks) beginTurn(path string) {
	if path == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	mark, ok := t.marks[path]
	if !ok {
		return
	}
	mark.LastTurn = ""
	t.marks[path] = mark
}

// drainTranscript returns everything the agent has said since Crowbar last read
// this conversation's transcript, and advances the position past it.
//
// It returns nothing at all — with no error and no log line worth reading — for a
// provider that declares no transcript, a payload that names none, a file that
// does not exist yet, and a session this process has not been watching. Each of
// those is an ordinary state, and each leaves the caller doing exactly what it
// did before this existed.
func (u *Usecase) drainTranscript(
	ctx context.Context,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) (string, []engineagents.TranscriptMessage) {
	path, ok := agent.TranscriptPath(ev)
	if !ok {
		return "", nil
	}
	mark, watching := u.transcripts.get(path)
	if !watching {
		// First sighting. Start where the file currently ENDS: a resumed session's
		// transcript already holds a whole conversation, and replaying it would
		// record the agent as having just said all of it.
		u.transcripts.set(path, transcriptMark{Offset: agent.TranscriptEnd(path)})
		return path, nil
	}

	read, err := agent.ReadTranscript(path, mark.Offset)
	if err != nil {
		// The position is deliberately NOT advanced: a transient read failure must
		// cost at most a late message, never a skipped one.
		slog.DebugContext(ctx, "agent: transcript: read", "err", err, "path", path)
		return path, nil
	}
	mark.Offset = read.Offset
	messages := read.Messages
	// The terminating hook can hand us a message the file had not yet flushed, and
	// the file then hands it back on the next read. Dropping a leading repeat of
	// the last thing we recorded is what keeps that from becoming two identical
	// turns; an agent that genuinely repeats itself word for word across two reads
	// loses the repetition, which is a far better failure than duplicating every
	// message whose hook beat its own transcript.
	if len(messages) > 0 && mark.LastText != "" && messages[0].Text == mark.LastText {
		messages = messages[1:]
	}
	u.transcripts.set(path, mark)
	return path, messages
}

// turnMessage is one message on its way into the record, and where it came from.
//
// The provenance is not decoration: it decides whether a later hook restating the
// same words is the same message or a second one. See transcriptMark.LastFromFile.
type turnMessage struct {
	text     string
	at       time.Time
	fromFile bool
}

func fileMessages(in []engineagents.TranscriptMessage) []turnMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]turnMessage, 0, len(in))
	for _, m := range in {
		out = append(out, turnMessage{text: m.Text, at: m.At, fromFile: true})
	}
	return out
}

// recordTranscriptSegments writes one assistant turn per message, closing the
// open turn onto each in order and re-opening a successor so the activity that
// follows has somewhere to attach.
//
// deliveryTurnID is the id the LAST message is recorded under — the hook's
// delivery id at a turn stop, so a redelivered hook rewrites one row instead of
// appending a second copy of the reply. The earlier messages take derived ids for
// the same reason: they must be stable across a redelivery of the same hook.
//
// Empty when there is nothing to record, so every caller can hand it whatever the
// drain produced without asking first.
func (u *Usecase) recordTranscriptSegments(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	path string,
	messages []turnMessage,
	deliveryTurnID, effort string,
	reopen bool,
) error {
	if len(messages) == 0 {
		return nil
	}
	base := deliveryTurnID
	var firstErr error
	for i, msg := range messages {
		last := i == len(messages)-1
		id := base + "-m" + itoa(int64(i))
		turnEffort := ""
		if last {
			id, turnEffort = deliveryTurnID, effort
		}
		at := msg.at
		if at.IsZero() {
			at = time.Now()
		}
		if err := u.activity.CloseTurn(ctx, agentactivity.TurnInput{
			ChatID:     chat.ID,
			TurnID:     id,
			ProviderID: runner.ProviderID,
			RunnerID:   runner.ID,
			SessionID:  runner.CurrentSession,
			Text:       msg.text,
			Effort:     turnEffort,
			Now:        at,
		}); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		u.transcripts.record(path, msg.text, id, msg.fromFile)
		if !last || reopen {
			// A closed turn leaves nothing open, and everything the agent does next —
			// the tool calls of the following segment, the reply still to come — needs
			// a turn to attach to. Re-opening immediately is what keeps that activity
			// from minting an anonymous turn of its own.
			u.openAssistantTurn(ctx, chat, runner)
		}
	}
	return firstErr
}

// finalMessage appends the terminating hook's own message to what the transcript
// gave us, unless it is already the last thing we have.
//
// This is the fallback that makes the whole feature safe. A provider with no
// transcript, a file that could not be read, a turn whose last message had not
// been flushed when the hook fired — all of them arrive here with the hook's
// message and nothing else, and all of them record exactly what Crowbar recorded
// before any of this existed.
func finalMessage(
	marks *transcriptMarks,
	path string,
	messages []turnMessage,
	final string,
) []turnMessage {
	if final == "" {
		return messages
	}
	switch {
	case len(messages) > 0:
		if messages[len(messages)-1].text == final {
			return messages
		}
	case path != "":
		// Nothing new in the file. The hook's message is a REPEAT only if the file is
		// where we last got it from: a message that has only ever arrived on a hook
		// is a fact in its own right however often the same words come back.
		if mark, ok := marks.get(path); ok && mark.LastFromFile && mark.LastText == final {
			return messages
		}
	}
	return append(messages, turnMessage{text: final})
}

// closeSilentTurn ends a turn that produced no new words.
//
// There are two ways to get here and they are not the same. A turn that genuinely
// said nothing — the provider ended it with an empty message and its transcript
// offered none — is ABANDONED, which is what Crowbar has always done: the turn
// closes so its in-flight tools stop reading as running, and no blank row appears
// in the conversation.
//
// A turn whose every word was ALREADY recorded from the transcript is a different
// thing, and new: it is closed onto the row that last message landed on. Without
// that, the tool calls that ran after the agent's final word would be left
// attached to a turn placeholder no message was ever written under, and would
// render nowhere at all.
func (u *Usecase) closeSilentTurn(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	path string,
) error {
	mark, recorded := u.transcripts.get(path)
	if path == "" || !recorded || mark.LastTurn == "" {
		if err := u.activity.Abandon(ctx, chat.ID, time.Now()); err != nil {
			return fmt.Errorf("agent: ingest hook: close empty turn: %w", err)
		}
		return nil
	}
	if err := u.activity.CloseTurn(ctx, agentactivity.TurnInput{
		ChatID:     chat.ID,
		TurnID:     mark.LastTurn,
		ProviderID: runner.ProviderID,
		RunnerID:   runner.ID,
		SessionID:  runner.CurrentSession,
		Text:       mark.LastText,
		Now:        time.Now(),
	}); err != nil {
		return fmt.Errorf("agent: ingest hook: close turn on its last message: %w", err)
	}
	return nil
}

// recordSaidSoFar writes down whatever the agent has said since the last time
// anybody looked, mid-turn.
//
// It runs on a TOOL INVOCATION and nowhere else in the observation path. That is
// the point where a message has just been said and work is about to follow it, so
// closing the segment here is what puts each message ahead of the tools it went
// on to run — and it is the only observation hook where the split buys anything,
// since every other one would record the same message a moment later at the same
// place in the record.
//
// It is best-effort in the strongest sense: a failure is logged and the turn goes
// on, because the terminating hook still records the reply and a hook must never
// break the vendor CLI's turn.
func (u *Usecase) recordSaidSoFar(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) {
	path, messages := u.drainTranscript(ctx, agent, ev)
	if len(messages) == 0 {
		return
	}
	if err := u.recordTranscriptSegments(
		ctx, chat, runner, path, fileMessages(messages), turnID(ctx), "", true,
	); err != nil {
		slog.WarnContext(ctx, "agent: transcript: record mid-turn messages",
			"err", err, "chat_id", chat.ID, "runner_id", runner.ID, "count", len(messages))
	}
}
