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

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

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
// dispatch the command layer (asynx serializes each aggregate's commands),
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
	// LoadChat folds the chat directly from the EVENT LOG, so it is always
	// current, where GetChat serves the asynchronous read model: anything deciding
	// on a chat (what a new thread inherits, a placement plan) reads it here.
	LoadChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	// SetPlacement writes where the chat sits in the Chats tree: the row it hangs
	// off and its dense index there. Placement lives in the Tree aggregate; every
	// chat this repository returns carries it.
	SetPlacement(
		ctx context.Context,
		chatID string,
		parentID string,
		order int,
	) (domain.Chat, error)
	// SetOrder writes a chat's index and keeps its parent as the Tree holds it: a
	// densify renumbers a level and must never restate a parent it only read.
	// A chat already gone is problem.ErrNotFound.
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
	// GetChats is every chat of ids that exists, placed in one placement read.
	GetChats(
		ctx context.Context,
		ids []string,
	) ([]domain.Chat, error)
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
// writeMu: asynx serializes each aggregate's commands.
type eventSourced struct {
	ax         asynx.Asynx[domain.Chat]
	store      store.Store
	placements Placements
}

// NewEventSourced builds the repository over ax, which must have been built with
// Subscriber(rm). es is the log ax wraps, kept for the lazy rebuild; placements
// is where each chat's placement is kept; ctx's values reach the watch's reads.
func NewEventSourced(
	ctx context.Context,
	ax asynx.Asynx[domain.Chat],
	es asynxModels.Store,
	rm *ReadModel,
	watch WatchFunc,
	placements Placements,
) (EventStore, error) {
	r := &eventSourced{ax: ax, placements: placements}
	st, err := store.New(rm, es, ax, r.placedWatch(ctx, watch))
	if err != nil {
		return nil, fmt.Errorf("agentchat: store: %w", err)
	}
	r.store = st
	return r, nil
}

// Create is the only command on the SendWait path; see the EventStore interface doc.
func (r *eventSourced) Create(
	ctx context.Context,
	in CreateInput,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.Create{
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
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) SetTitle(
	ctx context.Context,
	chatID string,
	title string,
	source string,
) (domain.Chat, error) {
	evt, err := r.ax.Send(ctx, commands.SetTitle{ChatID: chatID, Title: title, Source: source})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set title: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) SetSelection(
	ctx context.Context,
	chatID string,
	model string,
	effort string,
) (domain.Chat, error) {
	evt, err := r.ax.Send(ctx, commands.SetSelection{ChatID: chatID, Model: model, Effort: effort})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set selection: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) SetPermissionLevel(
	ctx context.Context,
	chatID string,
	level string,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.SetPermissionLevel{ChatID: chatID, Level: level, Explicit: true})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set permission level: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) SetSurface(
	ctx context.Context,
	chatID string,
	surface string,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.SetSurface{ChatID: chatID, Surface: surface})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set surface: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) SetProvider(
	ctx context.Context,
	chatID string,
	providerID string,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.SetProvider{ChatID: chatID, ProviderID: providerID})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set provider: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) SeedPermissionLevel(
	ctx context.Context,
	chatID string,
	level string,
) (domain.Chat, error) {
	evt, err := r.ax.Send(ctx, commands.SetPermissionLevel{ChatID: chatID, Level: level, Explicit: false})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: seed permission level: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) LoadChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	chat, err := r.ax.Get(ctx, id)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: load chat: %w", mapNotFound(err))
	}
	return r.placed(ctx, chat)
}

// SetWorkspace is deliberately on the SendWait path — see the interface doc.
func (r *eventSourced) SetWorkspace(
	ctx context.Context,
	chatID string,
	workspaceID string,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.SetWorkspace{ID: chatID, WorkspaceID: workspaceID})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set workspace: %w", err)
	}
	if err := r.placements.AdoptChat(ctx, chatID, workspaceID); err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set workspace: placement: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

// SetType is deliberately on the SendWait path — see the interface doc.
func (r *eventSourced) SetType(
	ctx context.Context,
	chatID string,
	chatType domain.ChatType,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.SetType{ID: chatID, Type: chatType})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set type: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
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
	if err := r.placements.DropChat(ctx, id); err != nil {
		return fmt.Errorf("agentchat: forget: placement: %w", err)
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
	return r.placed(ctx, chat)
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
	return r.placedAll(ctx, rows)
}

func (r *eventSourced) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.Chat, error) {
	rows, err := r.store.ListByWorkspace(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("agentchat: list by workspace: %w", err)
	}
	return r.placedAll(ctx, rows)
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
