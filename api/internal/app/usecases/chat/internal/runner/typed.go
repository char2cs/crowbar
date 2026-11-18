package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/promptsigil"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// errNotTypeable is the ledger refusing a typed prompt in the commit that would
// record it: the ordinary path delivers it instead.
var errNotTypeable = errors.New("agent: type: the chat is not idle for typing")

// submitPromptTyped types text into live's own composer when the descriptor
// declares typing and the CLI is idle at an empty box, keeping its background work.
// handled=false leaves the ordinary path to decide; a Stop cancels park, ending the typing.
func (rs *Runners) submitPromptTyped(
	ctx, park context.Context,
	chat domain.Chat,
	clientRequestID string,
	live engineagents.Runner,
	descriptor engineagents.Agent,
	text string,
) (domain.AgentPromptSubmission, bool, error) {
	screens, ok := rs.typeableRunner(live, descriptor)
	if !ok {
		return domain.AgentPromptSubmission{}, false, nil
	}
	restart, err := rs.selectionRequiresRestart(ctx, chat.ID, live, descriptor)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, fmt.Errorf("agent: submit prompt: check selection restart: %w", err)
	}
	if restart {
		return domain.AgentPromptSubmission{}, false, nil
	}
	sigils, escape := descriptor.PromptLeadingSigils()
	dispatch, err := rs.rewritePromptTextForDispatch(chat.ID, promptsigil.Guard(sigils, escape, chat.ID, text))
	if err != nil {
		return domain.AgentPromptSubmission{}, true, fmt.Errorf("agent: submit prompt: materialize attachments: %w", err)
	}
	// Judged on the dispatch: the sigil guard is what lets a leading attachment be typed.
	if !descriptor.CanType(dispatch) {
		return domain.AgentPromptSubmission{}, false, nil
	}
	in := typedPrompt{requestID: clientRequestID, text: text, dispatch: dispatch, live: live, descriptor: descriptor}
	var handled bool
	// In turn order from the gate to the typing: no turn opens on the CLI meanwhile.
	rs.live.inTurnOrder(chat.ID, func() {
		handled, err = rs.typeLocked(ctx, park, chat.ID, screens, in)
	})
	if !handled || err != nil {
		return domain.AgentPromptSubmission{}, handled, err
	}
	submission, err := rs.commitPromptSpawn(ctx, chat.ID, clientRequestID, live.ID)
	return submission, true, err
}

// typeableRunner is the screen reader for live's PTY when live is a CLI text
// may be typed into: a PTY of its own, no api connection, no attached view.
func (rs *Runners) typeableRunner(
	live engineagents.Runner, descriptor engineagents.Agent,
) (engineterminal.ScreenStateReader, bool) {
	if _, declared := descriptor.PromptType(); !declared || live.TerminalSession == "" {
		return nil, false
	}
	if _, viaAPI := rs.apiConns.get(live.ID); viaAPI {
		return nil, false
	}
	// An attached view is an attach client of the api session, not live's own CLI.
	if _, attached := rs.attached.get(live.ID); attached {
		return nil, false
	}
	screens, ok := rs.term.(engineterminal.ScreenStateReader)
	return screens, ok
}

// typedPrompt is one prompt the gate is deciding to type.
type typedPrompt struct {
	requestID, text, dispatch string
	live                      engineagents.Runner
	descriptor                engineagents.Agent
}

// typeLocked is the gate's turn-ordered half, then the record and the typing:
// handled=false records nothing.
func (rs *Runners) typeLocked(
	ctx, park context.Context, chatID string, screens engineterminal.ScreenStateReader, in typedPrompt,
) (bool, error) {
	if current, ok := rs.live.unending(in.live.ID); !ok || current.CurrentChatID != chatID {
		return false, nil
	}
	if !composerReady(screens, in.live.TerminalSession, in.descriptor) {
		return false, nil
	}
	var writes [][]byte
	note, err := rs.recordPrompt(ctx, chatID, in.requestID, in.text, in.dispatch, ledger.DeliveryType, in.live.ID,
		func(st ledger.State) error {
			var ok bool
			writes, ok = in.descriptor.PromptTypeWrites(st.MoveNote() + in.dispatch)
			return requireTypeable(st, ok)
		})
	if errors.Is(err, errNotTypeable) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if note != "" {
		rs.agents.RecordInjection(in.live.ID, strings.TrimSpace(note))
	}
	if err := rs.typeWrites(park, in, writes); err != nil {
		return true, rs.failPrompt(ctx, chatID, in.requestID, typeFailure(ctx, park, err))
	}
	return true, nil
}

// typeWrites sends writes in order, the descriptor's interval apart: a CLI reads a
// burst as a paste and rewrites it, so the words must arrive as keystrokes.
func (rs *Runners) typeWrites(park context.Context, in typedPrompt, writes [][]byte) error {
	typing, _ := in.descriptor.PromptType()
	for i, w := range writes {
		if i > 0 {
			if err := rs.pause(park, typing.ChunkInterval); err != nil {
				return err
			}
		}
		if err := rs.term.Write(park, in.live.TerminalSession, w); err != nil {
			return err
		}
	}
	return nil
}

func pauseFor(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("pause: %w", ctx.Err())
	}
}

// composerReady reports whether sessionID's screen takes text into an empty
// composer; a screen it cannot read does not.
func composerReady(screens engineterminal.ScreenStateReader, sessionID string, descriptor engineagents.Agent) bool {
	state, err := screens.ScreenState(sessionID)
	return err == nil && state.BracketedPaste &&
		descriptor.ComposerEmpty(engineagents.ComposerScreen{
			Text: state.Text, CursorRow: state.CursorRow, CursorCol: state.CursorCol, Faint: state.Faint,
		})
}

// requireTypeable admits a typed prompt only while no main-scope turn runs, no
// prompt or move is pending: a subagent or shell outliving its turn does not block it.
func requireTypeable(st ledger.State, typeable bool) error {
	_, running := st.RunningTurn()
	_, moving := st.PendingMove()
	if !typeable || running || moving || len(st.PendingPrompts()) > 0 {
		return errNotTypeable
	}
	return nil
}

// typeFailure is what a failed typing tells the client: a PTY that is gone took
// nothing; a write that failed part way may have left text in the box. A Stop
// that cut the typing short is the stop's: the CLI ends with it.
func typeFailure(ctx, park context.Context, err error) error {
	if stopped := parkErr(park, err); errors.Is(stopped, ErrStopped) {
		return stopped
	}
	if errors.Is(err, engineterminal.ErrSessionNotFound) {
		return fmt.Errorf("agent: submit prompt: type: %w", ErrPromptSessionUnavailable)
	}
	return uncertainPromptOutcome(ctx, "type into the composer", err)
}
