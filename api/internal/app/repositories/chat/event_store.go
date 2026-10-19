// This file holds the asynx-backed AgentChat repository (EventStore) — the sole
// AgentChat repository since the Task 10 cutover retired the bespoke gorm-backed
// Store/gormStore/New. The agent usecase sends every mutation through it.
package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// maxOCCAttempts bounds optimistic-concurrency retries on ErrPipelineFailed:
// with no per-aggregate writeMu, concurrent Sends to one chat aggregate can
// version-collide, so the losers retry — Send re-reads the current version
// each attempt, so a retry converges. ErrValidation is NEVER retried;
// ErrQueueFull is surfaced as apperr.ErrUnavailable. Mirrors reviewthread's OCC
// contract (spec decision 10).
const maxOCCAttempts = 8

// WatchFunc and ChatEvent are aliases for the store-layer watch seam, exposed so
// callers wire it without importing the internal store package directly.
//
// The repository ANNOUNCES what happened; it does not decide what the frontend is
// told. That decision lives in usecases/chat/internal/fanout.
type (
	WatchFunc = store.WatchFunc
	ChatEvent = store.ChatEvent
	Option    = store.Option
)

// WithPlacementWatch is told, once the read model holds it, of every chat whose
// parent, workspace or type changed, or that was deleted.
func WithPlacementWatch(watch func(chatID string)) Option {
	return store.WithPlacementWatch(watch)
}

// CreateInput seeds a new AgentChat: identity, kind, workspace, clock, and the
// two durable CHOICES a chat is born with — its landing view and its vendor. It
// still carries no segment and no terminal, because the chat does not own the
// CLI talking to it — that is the runner (runner.Start), a separate aggregate,
// and ProviderID no more starts one than Surface does.
type CreateInput struct {
	ID          string
	WorkspaceID string
	// RepoID is meaningful only when Type is ChatTypeFolder — see
	// domain.Chat.RepoID's own doc. Left "" for every other type.
	RepoID string
	Type   domain.ChatType
	// Surface is the chat's landing VIEW — see domain.Chat.Surface.
	Surface string
	// ProviderID is the vendor the chat is born on — see domain.Chat.ProviderID.
	ProviderID string
	Now        time.Time
}

// EventStore is the asynx-backed AgentChat aggregate repository: mutations
// dispatch the command layer with optimistic-concurrency retry (sendWithOCC),
// reads delegate to the store package's read-model projection. It is the sole
// AgentChat repository — the agent usecase sends every mutation through it. The
// chat holds no process fact: the CLI talking to it is the runner aggregate's.
type EventStore interface {
	// Create mints a chat. It alone blocks until its projections fold (SendWait):
	// the first hook of the CLI spawned onto it reads the chat from the read model,
	// which a Send could leave lagging. Every other write validates against the
	// synchronous event log.
	Create(
		ctx context.Context,
		in CreateInput,
	) (domain.Chat, error)
	SetTitle(
		ctx context.Context,
		chatID string,
		title string,
		source string,
	) (domain.Chat, error)
	// SetSelection writes the chat's sticky model and reasoning-effort choice —
	// durable config beside the title, and the answer to "what should the next
	// message run as", never a claim about what any live process is running.
	//
	// It is on the ordinary async Send path, and it can be: the prompt path that
	// decides whether the selection changed reads it through LoadChat, which folds
	// the EVENT LOG. A user who picks a model and immediately sends a message
	// therefore cannot race the projection into delivering that message under the
	// model they just changed away from — the read that decides never consults the
	// read model at all.
	SetSelection(
		ctx context.Context,
		chatID string,
		model string,
		effort string,
	) (domain.Chat, error)
	// SetPermissionLevel writes an EXPLICIT per-chat guarded/trusted/full-auto
	// pick — the same durable, next-spawn-reads-it fact SetSelection writes
	// for Model/Effort, on the same ordinary async Send path for the same
	// reason: the prompt path's restart decision reads it through LoadChat,
	// never the read model. Marks the chat PermissionLevelExplicit, so it
	// keeps winning over the global default from here on — see
	// SeedPermissionLevel for the non-explicit counterpart.
	SetPermissionLevel(
		ctx context.Context,
		chatID string,
		level string,
	) (domain.Chat, error)
	// SetSurface moves the chat to the VIEW the user is on now. Create seeds
	// the same field, so this is what makes domain.Chat.Surface a CURRENT
	// fact rather than a birth record — its only callers are the two moments
	// Crowbar is actually told the user moved (SwitchToTerminal and
	// SwitchToNative).
	//
	// On the SendWait path, like SetWorkspace: a move that relaunches the CLI
	// spawns it next, reading the surface from the READ MODEL.
	SetSurface(
		ctx context.Context,
		chatID string,
		surface string,
	) (domain.Chat, error)
	// SetProvider restates the VENDOR the chat runs as. Create seeds the same
	// field, so this is what keeps domain.Chat.ProviderID a CURRENT fact
	// rather than a birth record — its caller is the moment a CLI is
	// actually placed on the chat (recordRunner).
	//
	// It may be called UNCONDITIONALLY: "the provider did not change" is
	// decided by the command against the authoritative fold, never out here
	// against the read model the projection lags. ErrValidation from this
	// call is therefore the ordinary no-op answer — every ordinary respawn
	// produces one — not a failure.
	SetProvider(
		ctx context.Context,
		chatID string,
		providerID string,
	) (domain.Chat, error)
	// SeedPermissionLevel writes the level a chat merely INHERITS from the
	// global default — at mint, or re-seeding a legacy chat that predates
	// this field entirely. It leaves PermissionLevelExplicit false, so
	// ChatSelection keeps re-resolving the CURRENT global default for this
	// chat on every future spawn rather than freezing at this value.
	SeedPermissionLevel(
		ctx context.Context,
		chatID string,
		level string,
	) (domain.Chat, error)
	// SetPlacement writes where the chat sits in the Chats tree: the row it hangs
	// off and its dense index within that sibling space.
	//
	// It is on the ordinary async Send path, and it has to be: a single drag
	// renumbers a whole level, so one gesture is N of these, and blocking each on
	// its projection would serialise the drag behind the read model. Nothing reads
	// back through the read model to decide the next write — the caller planned the
	// whole move from ONE snapshot before the first command went out — so there is
	// no barrier to owe.
	// LoadChat folds the chat directly from the EVENT LOG, so it is always
	// current — the same source every command validates against.
	//
	// It exists beside GetChat because the two answer different questions and one
	// of them is not safe for every caller. GetChat serves the read-model
	// projection, which is asynchronous: SetPlacement is deliberately on the async
	// Send path (see its doc), so a chat read back straight after a placement can
	// still be serving the placement it had BEFORE. That is fine for rendering a
	// list and fatal for a DECISION taken on it — a spawn resolving what a new
	// thread inherits read ParentID as "" and injected nothing, so the thread's
	// first session came up not knowing it was a thread. Anything deciding on a
	// chat's placement must read it here.
	//
	// It mirrors workspace.Get and reviewthread.Get, which fold per-id reads from
	// the log for exactly this reason (§3.7). Unlike those, this aggregate's list
	// is hot enough (every hook resolves its chat) that the projected read is kept
	// as the default rather than replaced.
	LoadChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	SetPlacement(
		ctx context.Context,
		chatID string,
		parentID string,
		order int,
	) (domain.Chat, error)
	// SetOrder writes a chat's index within the sibling space it is already in
	// and leaves its parent alone — the write a DENSIFY owes, as against a move.
	//
	// A drag renumbers a whole level and re-parents one row. Writing the rest of
	// that level through SetPlacement made every renumber restate a parent the
	// caller had read rather than decided, and the caller reads the asynchronous
	// projection: a second drag landing inside the first one's projection window
	// wrote a stale parent back and returned a just-filed thread to the panel
	// root. The parent this preserves comes from the log fold the command
	// validates against, so it is current rather than merely untouched.
	SetOrder(
		ctx context.Context,
		chatID string,
		order int,
	) (domain.Chat, error)
	// SetWorkspace fills a chat's WorkspaceID slot — a bubble gaining worktree
	// ownership, or a thread being promoted (Promote, usecases/chat).
	//
	// It is on the SAME SendWait path as Create, not the ordinary async Send
	// path every other setter above uses: Promote's very next act after this
	// write is a provider switch that resolves the respawn cwd from GetChat's
	// READ MODEL, and a Send here would leave a live window where that read
	// still serves the OLD (empty) WorkspaceID — spawning the incoming CLI into
	// the wrong directory. See Create's own doc for the same reasoning.
	SetWorkspace(
		ctx context.Context,
		chatID string,
		workspaceID string,
	) (domain.Chat, error)
	// SetType rewrites which KIND of row a chat is, leaving its id, placement,
	// workspace and conversation untouched. It is how a row survives its
	// workspace changing character — an ordinary worktree becoming a locked
	// branch — without a second row being minted to claim the same workspace.
	// See commands.SetType, which refuses a retype across the folder boundary.
	//
	// It is on the SendWait path, like Create above and for the same reason: a
	// retype happens because the workspace JUST became locked, and the client
	// that locked it re-reads the chat list off the READ MODEL as soon as the
	// call returns — expecting the branch row the lock entitles it to. On the
	// ordinary async Send path that read can still serve the pre-retype row, so
	// the guarantee "a locked workspace has a branch row" would hold in the
	// event log and not on the wire. Retypes are rare (a lock, not a keystroke),
	// so the barrier costs nothing worth saving.
	SetType(
		ctx context.Context,
		chatID string,
		chatType domain.ChatType,
	) (domain.Chat, error)
	// Forget purges the chat aggregate outright via ax.Forget: its synchronous
	// OnForget drops the read-model row AND the underlying event log is
	// erased, so a subsequent GetChat/ListByWorkspace genuinely reports not
	// found. Its one caller is the hard delete (agent.ChatUsecase.PurgeChat),
	// reached both on user request and from the workspace-delete cascade.
	// Mirrors reviewthread's DeleteThread.
	Forget(
		ctx context.Context,
		id string,
	) error
	GetChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	// Get is GetChat under the name the unified sidebar forest's tree usecase
	// asks by: a row is a row, folder or chat, and that usecase reads by id
	// alone.
	Get(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	ListChats(
		ctx context.Context,
	) ([]domain.Chat, error)
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.Chat, error)
}

// eventSourced is the asynx-backed EventStore implementation. There is no
// writeMu: per-aggregate safety comes from asynx shard routing plus
// (id,version) optimistic concurrency and the sendWithOCC retry below.
type eventSourced struct {
	ax    asynx.Asynx[domain.Chat]
	store store.Store
}

// NewEventSourced builds the asynx-backed AgentChat EventStore over ax (the
// singleton axAgentChat), es (the same per-type event log ax wraps, retained
// so the read model can self-heal via whole-model lazy Replay), and storeDB
// (state/store/agent_chat.db). It delegates to store.New to register the
// read-model and hub projections on ax, once, for the singleton.
func NewEventSourced(
	ax asynx.Asynx[domain.Chat],
	es asynxModels.Store,
	storeDB *gormdb.DB,
	watch WatchFunc,
	opts ...Option,
) (EventStore, error) {
	st, err := store.New(storeDB, es, ax, watch, opts...)
	if err != nil {
		return nil, fmt.Errorf("agentchat: store: %w", err)
	}
	return &eventSourced{ax: ax, store: st}, nil
}

// sendFunc issues one command attempt against the aggregate.
type sendFunc func(
	ctx context.Context,
	cmd asynxModels.Command[domain.Chat],
) (asynxModels.Event[domain.Chat], error)

// occSend runs send with OCC retry and the terminal error disposition
// contract (mirrors reviewthread's occSend):
//
//   - success                → returned immediately.
//   - ErrValidation          → surfaced immediately, NEVER retried (→ 422).
//   - ErrQueueFull           → translated to apperr.ErrUnavailable (→ 503),
//     NEVER retried: a full shard queue is backpressure, not a version race.
//   - ErrPipelineFailed      → retried up to maxOCCAttempts; still failing
//     after the retries is an unrecoverable optimistic-concurrency collision,
//     surfaced as ErrPipelineFailed (→ 409).
//   - any other error        → surfaced as-is.
//
// All classification is via errors.Is, never string compare.
func occSend(
	ctx context.Context,
	send sendFunc,
	cmd asynxModels.Command[domain.Chat],
) (asynxModels.Event[domain.Chat], error) {
	var lastErr error
	for range maxOCCAttempts {
		evt, err := send(ctx, cmd)
		if err == nil {
			return evt, nil
		}
		switch {
		case errors.Is(err, asynxModels.ErrValidation):
			return asynxModels.Event[domain.Chat]{}, err
		case errors.Is(err, asynxModels.ErrQueueFull):
			return asynxModels.Event[domain.Chat]{}, fmt.Errorf("agentchat: send: %w", apperr.ErrUnavailable)
		case errors.Is(err, asynxModels.ErrPipelineFailed):
			lastErr = err
		default:
			return asynxModels.Event[domain.Chat]{}, err
		}
	}
	return asynxModels.Event[domain.Chat]{}, lastErr
}

// sendWithOCC dispatches cmd to the singleton axAgentChat with OCC retry.
func (r *eventSourced) sendWithOCC(
	ctx context.Context,
	cmd asynxModels.Command[domain.Chat],
) (asynxModels.Event[domain.Chat], error) {
	return occSend(ctx, r.ax.Send, cmd)
}

// Create is the only command on the SendWait path; see the EventStore interface doc.
func (r *eventSourced) Create(
	ctx context.Context,
	in CreateInput,
) (domain.Chat, error) {
	evt, err := occSend(ctx, r.ax.SendWait, commands.Create{
		ID:          in.ID,
		WorkspaceID: in.WorkspaceID,
		RepoID:      in.RepoID,
		Type:        in.Type,
		Surface:     in.Surface,
		ProviderID:  in.ProviderID,
		Now:         in.Now,
	})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: create: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) SetTitle(
	ctx context.Context,
	chatID string,
	title string,
	source string,
) (domain.Chat, error) {
	evt, err := r.sendWithOCC(ctx, commands.SetTitle{ChatID: chatID, Title: title, Source: source})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set title: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) SetSelection(
	ctx context.Context,
	chatID string,
	model string,
	effort string,
) (domain.Chat, error) {
	evt, err := r.sendWithOCC(ctx, commands.SetSelection{ChatID: chatID, Model: model, Effort: effort})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set selection: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) SetPermissionLevel(
	ctx context.Context,
	chatID string,
	level string,
) (domain.Chat, error) {
	evt, err := r.sendWithOCC(ctx, commands.SetPermissionLevel{ChatID: chatID, Level: level, Explicit: true})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set permission level: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) SetSurface(
	ctx context.Context,
	chatID string,
	surface string,
) (domain.Chat, error) {
	evt, err := occSend(ctx, r.ax.SendWait, commands.SetSurface{ChatID: chatID, Surface: surface})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set surface: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) SetProvider(
	ctx context.Context,
	chatID string,
	providerID string,
) (domain.Chat, error) {
	evt, err := r.sendWithOCC(ctx, commands.SetProvider{ChatID: chatID, ProviderID: providerID})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set provider: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) SeedPermissionLevel(
	ctx context.Context,
	chatID string,
	level string,
) (domain.Chat, error) {
	evt, err := r.sendWithOCC(ctx, commands.SetPermissionLevel{ChatID: chatID, Level: level, Explicit: false})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: seed permission level: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *eventSourced) LoadChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	chat, err := r.ax.Get(ctx, id)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: load chat: %w", mapNotFound(err))
	}
	return chat, nil
}

func (r *eventSourced) SetPlacement(
	ctx context.Context,
	chatID string,
	parentID string,
	order int,
) (domain.Chat, error) {
	evt, err := r.sendWithOCC(ctx, commands.SetPlacement{ID: chatID, ParentID: parentID, Order: order})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set placement: %w", err)
	}
	return evt.Aggregate, nil
}

// SetOrder reports a target that no longer exists as apperr.ErrNotFound rather
// than the command's raw validation failure: a densify plans a whole level from
// one snapshot, and a chat a concurrent delete purged out from under it needs
// no order any more, which its caller can only tell apart from a genuine
// validation bug (a negative order) via this sentinel.
func (r *eventSourced) SetOrder(
	ctx context.Context,
	chatID string,
	order int,
) (domain.Chat, error) {
	evt, err := r.sendWithOCC(ctx, commands.SetOrder{ID: chatID, Order: order})
	if err != nil {
		if errors.Is(err, commands.ErrNoSuchChat) {
			return domain.Chat{}, fmt.Errorf("agentchat: set order: %w", apperr.ErrNotFound)
		}
		return domain.Chat{}, fmt.Errorf("agentchat: set order: %w", err)
	}
	return evt.Aggregate, nil
}

// SetWorkspace is deliberately on the SendWait path — see the interface doc.
func (r *eventSourced) SetWorkspace(
	ctx context.Context,
	chatID string,
	workspaceID string,
) (domain.Chat, error) {
	evt, err := occSend(ctx, r.ax.SendWait, commands.SetWorkspace{ID: chatID, WorkspaceID: workspaceID})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set workspace: %w", err)
	}
	return evt.Aggregate, nil
}

// SetType is deliberately on the SendWait path — see the interface doc.
func (r *eventSourced) SetType(
	ctx context.Context,
	chatID string,
	chatType domain.ChatType,
) (domain.Chat, error) {
	evt, err := occSend(ctx, r.ax.SendWait, commands.SetType{ID: chatID, Type: chatType})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set type: %w", err)
	}
	return evt.Aggregate, nil
}

// Forget purges the chat aggregate via ax.Forget (hard delete), mirroring
// reviewthread's DeleteThread. See the EventStore interface doc for details.
func (r *eventSourced) Forget(
	ctx context.Context,
	id string,
) error {
	if err := r.ax.Forget(ctx, id); err != nil {
		return fmt.Errorf("agentchat: forget: %w", err)
	}
	return nil
}

func (r *eventSourced) GetChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	chat, err := r.store.GetChat(ctx, id)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: get chat: %w", mapNotFound(err))
	}
	return chat, nil
}

func (r *eventSourced) Get(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	return r.GetChat(ctx, id)
}

func (r *eventSourced) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	rows, err := r.store.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agentchat: list chats: %w", err)
	}
	return rows, nil
}

func (r *eventSourced) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.Chat, error) {
	rows, err := r.store.ListByWorkspace(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("agentchat: list by workspace: %w", err)
	}
	return rows, nil
}

// mapNotFound bridges the store package's local ErrNotFound sentinel (kept
// local there to avoid an import cycle back into this package) AND the log-fold
// sentinel asynx raises for an unknown aggregate to this package's own
// ErrNotFound, so every EventStore caller sees one sentinel regardless of which
// of the two read paths served the miss.
func mapNotFound(
	err error,
) error {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, asynxModels.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
