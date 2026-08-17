// This file holds the asynx-backed AgentRunner repository (EventStore) — the
// sole AgentRunner repository, and the only way anything outside this package
// touches a runner. Mutations dispatch the command layer with
// optimistic-concurrency retry; reads delegate to the store package's two
// projections (live runners, append-only conversation history).
package agentrunner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// maxOCCAttempts bounds optimistic-concurrency retries on ErrPipelineFailed:
// with no per-aggregate writeMu, concurrent Sends to one runner aggregate can
// version-collide, so the losers retry — Send re-reads the current version each
// attempt, so a retry converges. ErrValidation is NEVER retried; ErrQueueFull is
// surfaced as apperr.ErrUnavailable. Mirrors agentchat's/reviewthread's OCC
// contract.
const maxOCCAttempts = 8

// BroadcastFunc is an alias for the store-layer broadcast type, exposed so
// callers can wire the hub fan-out (hub.BroadcastAgentRunner) without importing
// the internal store package directly. It carries the CHAT id alongside the
// runner and workspace, because a `moved` frame's whole point is telling the
// frontend which chat the runner moved INTO.
type BroadcastFunc = store.BroadcastFunc

// StartInput seeds a freshly-spawned CLI. No conversation is bound yet — the
// provider announces one later, which lands as BindSession (or, if it announces
// a conversation belonging to another chat, as Move).
type StartInput struct {
	RunnerID        string
	WorkspaceID     string
	ProviderID      string
	TerminalSession string
	ChatID          string
	LaunchSessionID string
	Now             time.Time
}

// EventStore is the asynx-backed AgentRunner aggregate repository.
//
// The write side is four commands, and the important one is Move: repointing a
// runner at another chat/conversation is ONE write to ONE aggregate. It does not
// touch the chat being left or the chat being entered, so it cannot half-succeed
// across them.
//
// The read side answers placement questions ONLY, from two projections:
//   - live runners — a row exists exactly while its CLI runs, so ErrNotFound on
//     LiveRunnerForChat/Get means "dormant", not "broken".
//   - append-only history — ChatForSession/LastConversation keep answering long
//     after the runner that opened the conversation has died, which is what makes
//     a re-announced session id on /resume recognisable instead of looking new.
//
// Nothing here reports liveness as STATE: ask the live model (row exists?) or the
// PTY. ExitedAt is an audit tombstone and is never a liveness check.
type EventStore interface {
	// Start records a freshly-spawned CLI, pointed at the chat it was spawned into.
	Start(
		ctx context.Context,
		in StartInput,
	) (domain.AgentRunner, error)
	// BindSession records the provider's FIRST conversation id for a runner that is
	// staying put. now is when the conversation OPENED and is required (a zero one
	// is rejected): the history projection stamps FirstSeenAt from it, and the
	// runner's spawn time cannot stand in — a long-lived CLI opens conversations
	// hours after it started.
	BindSession(
		ctx context.Context,
		runnerID string,
		sessionID string,
		resumable bool,
		now time.Time,
	) (domain.AgentRunner, error)
	// Move repoints a runner at a different chat and conversation — the /clear and
	// /resume path. One write, one aggregate: the torn cross-aggregate write that
	// bricked a chat in production has no way to happen here. The PTY, the provider
	// and the runner id all travel unchanged, which is why the terminal never
	// remounts on a /clear. now is required, for the same reason as BindSession.
	Move(
		ctx context.Context,
		runnerID string,
		toChatID string,
		sessionID string,
		resumable bool,
		now time.Time,
	) (domain.AgentRunner, error)
	// Displace takes a runner OFF its chat and conversation, leaving its row — and
	// saying NOTHING about whether the process is alive. It is a PLACEMENT fact, the
	// one kind of fact Crowbar solely owns, and it is what makes "at most one runner
	// per chat" true at every instant instead of eventually: a SIGTERM'd CLI does not
	// die synchronously, so the CLI we are taking off a chat has to be taken off it
	// NOW, not whenever it gets around to falling over. Issued (before the kill) by
	// every path that removes a CLI from a chat, and issued even when that kill fails.
	Displace(
		ctx context.Context,
		runnerID string,
	) (domain.AgentRunner, error)
	// Exit tombstones a runner whose PTY has died. It is emitted ONLY because the
	// PTY died (the terminal engine's exit callback, or boot reconciliation asking
	// the PTY) — never from an independent opinion about liveness. The projection
	// DROPS the live row, and that deletion is how the chat goes dormant.
	Exit(
		ctx context.Context,
		runnerID string,
		now time.Time,
	) (domain.AgentRunner, error)
	// Get returns the live runner, or ErrNotFound once it has exited.
	Get(
		ctx context.Context,
		runnerID string,
	) (domain.AgentRunner, error)
	// LiveRunnerForChat returns the runner currently pointed at chatID, or
	// ErrNotFound when the chat is dormant. Row-existence IS the liveness answer.
	LiveRunnerForChat(
		ctx context.Context,
		chatID string,
	) (domain.AgentRunner, error)
	// LiveRunnersForChat returns EVERY live runner placed on chatID, newest arrival first.
	// It is what the two PLACEMENT paths (a Move onto a chat, a Start onto a chat) ask
	// immediately after their write commits, so they can retire everyone but themselves:
	// their job is to leave exactly ONE runner on the chat, and a single-row read cannot
	// tell "nobody else" from "somebody else, and maybe more". Serving paths want
	// LiveRunnerForChat instead — "who holds this chat".
	LiveRunnersForChat(
		ctx context.Context,
		chatID string,
	) ([]domain.AgentRunner, error)
	// LiveRunnersForSession returns EVERY live runner holding a conversation, newest arrival
	// first — the I3 twin of LiveRunnersForChat, and the read a placement onto a conversation
	// (a bind, a move) must use. The single-row read cannot serve that: once the write has
	// committed the caller IS the newest holder, so it would get its own row back and evict
	// nobody.
	LiveRunnersForSession(
		ctx context.Context,
		wsID string,
		sessionID string,
	) ([]domain.AgentRunner, error)
	// LiveRunnerForSession returns the runner currently HOLDING a conversation —
	// the incumbent an eviction must displace. ErrNotFound means nobody is running
	// that conversation right now, which is NOT "it never existed" (that question
	// is ChatForSession's, and it is answered from history).
	LiveRunnerForSession(
		ctx context.Context,
		wsID string,
		sessionID string,
	) (domain.AgentRunner, error)
	// ChatForSession resolves which chat a provider conversation belongs to, from
	// APPEND-ONLY history — so it still answers after the runner that opened it is
	// long gone.
	ChatForSession(
		ctx context.Context,
		wsID string,
		sessionID string,
	) (string, error)
	// LastConversation returns the most recent conversation a chat has hosted: the
	// one a Resume picks up from.
	LastConversation(
		ctx context.Context,
		chatID string,
	) (domain.ChatConversation, error)
	// ConversationsForChat returns every conversation the chat has hosted, OLDEST
	// FIRST. It is what a provider switch reads to find the conversation the
	// INCOMING provider left behind here (LastConversation only answers for the
	// chat as a whole, and after a handoff the newest conversation belongs to the
	// provider being switched AWAY from). Order is by when each conversation
	// opened, so "the newest one for provider X" is simply the last match.
	ConversationsForChat(
		ctx context.Context,
		chatID string,
	) ([]domain.ChatConversation, error)
	// AllLive returns every runner the read model believes is still running — the
	// input to boot reconciliation, which asks the PTY (the sole authority) whether
	// each one really is and Exits the ones that are not. On an idle machine an
	// empty result is the normal, truthful answer.
	AllLive(
		ctx context.Context,
	) ([]domain.AgentRunner, error)
	// ForgetChat drops a chat's conversation history — the chat-delete cascade, and
	// the ONLY thing permitted to remove append-only history. It deliberately does
	// NOT delete the chat's live runner row: that row belongs to the PTY's
	// lifecycle, so the cascade terminates the process and lets the row follow.
	// Hand-deleting it here would be a second authority on liveness — the exact
	// mistake this aggregate exists to delete.
	ForgetChat(
		ctx context.Context,
		chatID string,
	) error
}

// eventSourced is the asynx-backed EventStore implementation. There is no
// writeMu: per-aggregate safety comes from asynx shard routing plus (id,version)
// optimistic concurrency and the sendWithOCC retry below.
type eventSourced struct {
	ax    asynx.Asynx[domain.AgentRunner]
	store *store.Store
}

// NewEventSourced builds the asynx-backed AgentRunner EventStore over ax (the
// singleton axAgentRunner), es (the same per-type event log ax wraps, retained so
// the conversation history can be healed from it once at construction), and
// storeDB (state/store/agent_runner.db). It delegates to store.New to register
// the read-model and hub projections on ax, once, for the singleton.
//
// broadcast must not be nil (store.New refuses it): the hub projection calls it
// on every event, so a nil one would panic inside a projection goroutine far from
// whoever built the repository.
func NewEventSourced(
	ax asynx.Asynx[domain.AgentRunner],
	es asynxModels.Store,
	storeDB *gormdb.DB,
	broadcast BroadcastFunc,
) (EventStore, error) {
	st, err := store.New(storeDB, es, ax, broadcast)
	if err != nil {
		return nil, fmt.Errorf("agentrunner: store: %w", err)
	}
	return &eventSourced{ax: ax, store: st}, nil
}

// sendFunc issues one command attempt against the aggregate.
type sendFunc func(
	ctx context.Context,
	cmd asynxModels.Command[domain.AgentRunner],
) (asynxModels.Event[domain.AgentRunner], error)

// occSend runs send with OCC retry and the terminal error disposition contract
// (mirrors agentchat's occSend):
//
//   - success                → returned immediately.
//   - ErrValidation          → surfaced immediately, NEVER retried (→ 422).
//   - ErrQueueFull           → translated to apperr.ErrUnavailable (→ 503),
//     NEVER retried: a full shard queue is backpressure, not a version race.
//   - ErrPipelineFailed      → retried up to maxOCCAttempts; still failing after
//     the retries is an unrecoverable optimistic-concurrency collision, surfaced
//     as ErrPipelineFailed (→ 409).
//   - any other error        → surfaced as-is.
//
// All classification is via errors.Is, never string compare.
func occSend(
	ctx context.Context,
	send sendFunc,
	cmd asynxModels.Command[domain.AgentRunner],
) (asynxModels.Event[domain.AgentRunner], error) {
	var lastErr error
	for range maxOCCAttempts {
		evt, err := send(ctx, cmd)
		if err == nil {
			return evt, nil
		}
		switch {
		case errors.Is(err, asynxModels.ErrValidation):
			return asynxModels.Event[domain.AgentRunner]{}, err
		case errors.Is(err, asynxModels.ErrQueueFull):
			return asynxModels.Event[domain.AgentRunner]{}, fmt.Errorf("agentrunner: send: %w", apperr.ErrUnavailable)
		case errors.Is(err, asynxModels.ErrPipelineFailed):
			lastErr = err
		default:
			return asynxModels.Event[domain.AgentRunner]{}, err
		}
	}
	return asynxModels.Event[domain.AgentRunner]{}, lastErr
}

// sendWithOCC dispatches cmd to the singleton axAgentRunner with OCC retry.
//
// It uses SendWait, not Send: the command returns only once every projection has
// folded it, so the runner read model is CAUSALLY CONSISTENT with the write that
// just happened. That is load-bearing on the hook path, not a nicety. A vendor CLI
// fires session_start and then, milliseconds later, the user_prompt for the very
// turn that follows it; the second hook resolves the chat to write into by READING
// the runner (Get → CurrentChatID). With the async Send, a Move could still be in
// flight when that read happens, and the turn would be filed into the chat the
// runner had just LEFT — precisely the "hook lands in the wrong chat" bug this
// refactor deletes. Making the placement write synchronous with its own read model
// is what lets the usecase drop the in-memory registry entirely instead of
// replacing it with another shadow of durable state.
//
// The cost is bounded and paid in the right place: runner commands fire on spawn,
// bind, move and exit — never per turn — and their projections are two local sqlite
// writes plus a non-blocking hub push.
func (r *eventSourced) sendWithOCC(
	ctx context.Context,
	cmd asynxModels.Command[domain.AgentRunner],
) (asynxModels.Event[domain.AgentRunner], error) {
	return occSend(ctx, r.ax.SendWait, cmd)
}

func (r *eventSourced) Start(
	ctx context.Context,
	in StartInput,
) (domain.AgentRunner, error) {
	evt, err := r.sendWithOCC(ctx, commands.Start{
		RunnerID:        in.RunnerID,
		WorkspaceID:     in.WorkspaceID,
		ProviderID:      in.ProviderID,
		TerminalSession: in.TerminalSession,
		ChatID:          in.ChatID,
		LaunchSessionID: in.LaunchSessionID,
		Now:             in.Now,
	})
	if err != nil {
		return domain.AgentRunner{}, fmt.Errorf("agentrunner: start: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) BindSession(
	ctx context.Context,
	runnerID string,
	sessionID string,
	resumable bool,
	now time.Time,
) (domain.AgentRunner, error) {
	evt, err := r.sendWithOCC(ctx, commands.BindSession{
		RunnerID:  runnerID,
		SessionID: sessionID,
		Resumable: resumable,
		Now:       now,
	})
	if err != nil {
		return domain.AgentRunner{}, fmt.Errorf("agentrunner: bind session: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) Move(
	ctx context.Context,
	runnerID string,
	toChatID string,
	sessionID string,
	resumable bool,
	now time.Time,
) (domain.AgentRunner, error) {
	evt, err := r.sendWithOCC(ctx, commands.Move{
		RunnerID:  runnerID,
		ToChatID:  toChatID,
		SessionID: sessionID,
		Resumable: resumable,
		Now:       now,
	})
	if err != nil {
		return domain.AgentRunner{}, fmt.Errorf("agentrunner: move: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) Displace(
	ctx context.Context,
	runnerID string,
) (domain.AgentRunner, error) {
	evt, err := r.sendWithOCC(ctx, commands.Displace{RunnerID: runnerID})
	if err != nil {
		return domain.AgentRunner{}, fmt.Errorf("agentrunner: displace: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) Exit(
	ctx context.Context,
	runnerID string,
	now time.Time,
) (domain.AgentRunner, error) {
	evt, err := r.sendWithOCC(ctx, commands.Exit{RunnerID: runnerID, Now: now})
	if err != nil {
		return domain.AgentRunner{}, fmt.Errorf("agentrunner: exit: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) Get(
	ctx context.Context,
	runnerID string,
) (domain.AgentRunner, error) {
	runner, err := r.store.Get(ctx, runnerID)
	if err != nil {
		return domain.AgentRunner{}, fmt.Errorf("agentrunner: get: %w", mapNotFound(err))
	}
	return runner, nil
}

func (r *eventSourced) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (domain.AgentRunner, error) {
	runner, err := r.store.LiveRunnerForChat(ctx, chatID)
	if err != nil {
		return domain.AgentRunner{}, fmt.Errorf("agentrunner: live runner for chat: %w", mapNotFound(err))
	}
	return runner, nil
}

func (r *eventSourced) LiveRunnersForChat(
	ctx context.Context,
	chatID string,
) ([]domain.AgentRunner, error) {
	runners, err := r.store.LiveRunnersForChat(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agentrunner: live runners for chat: %w", err)
	}
	return runners, nil
}

func (r *eventSourced) LiveRunnersForSession(
	ctx context.Context,
	wsID string,
	sessionID string,
) ([]domain.AgentRunner, error) {
	runners, err := r.store.LiveRunnersForSession(ctx, wsID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("agentrunner: live runners for session: %w", err)
	}
	return runners, nil
}

func (r *eventSourced) LiveRunnerForSession(
	ctx context.Context,
	wsID string,
	sessionID string,
) (domain.AgentRunner, error) {
	runner, err := r.store.LiveRunnerForSession(ctx, wsID, sessionID)
	if err != nil {
		return domain.AgentRunner{}, fmt.Errorf("agentrunner: live runner for session: %w", mapNotFound(err))
	}
	return runner, nil
}

func (r *eventSourced) ChatForSession(
	ctx context.Context,
	wsID string,
	sessionID string,
) (string, error) {
	chatID, err := r.store.ChatForSession(ctx, wsID, sessionID)
	if err != nil {
		return "", fmt.Errorf("agentrunner: chat for session: %w", mapNotFound(err))
	}
	return chatID, nil
}

func (r *eventSourced) LastConversation(
	ctx context.Context,
	chatID string,
) (domain.ChatConversation, error) {
	conv, err := r.store.LastConversation(ctx, chatID)
	if err != nil {
		return domain.ChatConversation{}, fmt.Errorf("agentrunner: last conversation: %w", mapNotFound(err))
	}
	return conv, nil
}

func (r *eventSourced) ConversationsForChat(
	ctx context.Context,
	chatID string,
) ([]domain.ChatConversation, error) {
	convs, err := r.store.ConversationsForChat(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agentrunner: conversations for chat: %w", err)
	}
	return convs, nil
}

func (r *eventSourced) AllLive(
	ctx context.Context,
) ([]domain.AgentRunner, error) {
	rows, err := r.store.AllLive(ctx)
	if err != nil {
		return nil, fmt.Errorf("agentrunner: all live: %w", err)
	}
	return rows, nil
}

func (r *eventSourced) ForgetChat(
	ctx context.Context,
	chatID string,
) error {
	if err := r.store.ForgetChat(ctx, chatID); err != nil {
		return fmt.Errorf("agentrunner: forget chat: %w", err)
	}
	return nil
}

// mapNotFound bridges the store package's local ErrNotFound sentinel (kept local
// there to avoid an import cycle back into this package) to this package's own
// ErrNotFound, so every EventStore caller sees one sentinel regardless of which
// read path served the miss.
func mapNotFound(
	err error,
) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
