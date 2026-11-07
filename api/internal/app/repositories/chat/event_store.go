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

// CreateInput seeds a new AgentChat: identity, kind, workspace, clock, landing
// view and vendor. It carries no process state: the CLI talking to the chat is
// the runner aggregate's, and ProviderID does not start one.
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

// EventStore is the asynx-backed AgentChat repository: mutations dispatch the
// command layer (asynx serializes each aggregate's commands) and reads use the
// read-model projection. The chat holds no process fact; that is the runner's.
type EventStore interface {
	// Create mints a chat and blocks until its projections fold (SendWait): the
	// first hook of the CLI spawned onto it reads the chat from the read model.
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
	// SetSelection writes the sticky model and effort the next message runs as,
	// never a claim about a live process. Async Send is safe: the prompt path reads
	// the selection via LoadChat (the event log), so it cannot race the projection.
	SetSelection(
		ctx context.Context,
		chatID string,
		model string,
		effort string,
	) (domain.Chat, error)
	// SetPermissionLevel writes an explicit per-chat permission level and marks
	// PermissionLevelExplicit, so the pick wins over the global default;
	// SeedPermissionLevel is the inherited counterpart.
	SetPermissionLevel(
		ctx context.Context,
		chatID string,
		level string,
	) (domain.Chat, error)
	// SetSurface moves the chat to the view the user is on now, keeping
	// domain.Chat.Surface current. It uses SendWait: a move that relaunches the CLI
	// spawns it next, reading the surface from the read model.
	SetSurface(
		ctx context.Context,
		chatID string,
		surface string,
	) (domain.Chat, error)
	// SetProvider restates the vendor the chat runs as, keeping ProviderID current.
	// Call it unconditionally: the command decides "unchanged" against the event-log
	// fold, so ErrValidation is the ordinary no-op answer, not a failure.
	SetProvider(
		ctx context.Context,
		chatID string,
		providerID string,
	) (domain.Chat, error)
	// SeedPermissionLevel writes the level a chat inherits from the global default.
	// It leaves PermissionLevelExplicit false, so every spawn re-resolves the
	// current default rather than freezing at this value.
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
	// SetWorkspace fills a chat's WorkspaceID. It uses SendWait: the next launch
	// resolves its cwd from the read model, and a lagging row would spawn the CLI
	// in the wrong directory.
	SetWorkspace(
		ctx context.Context,
		chatID string,
		workspaceID string,
	) (domain.Chat, error)
	// RunIn points a chat at a workspace another row owns, SendWait for the same
	// reason: the next launch resolves its folder from the read model.
	RunIn(
		ctx context.Context,
		chatID string,
		workspaceID string,
	) (domain.Chat, error)
	// SetType rewrites a chat's kind in place (chat or workflow), leaving its id,
	// workspace and conversation untouched; it backfills rows minted untyped.
	SetType(
		ctx context.Context,
		chatID string,
		chatType domain.ChatType,
	) (domain.Chat, error)
	// Forget purges the chat aggregate: OnForget drops the read-model row
	// synchronously and the event log is erased, so later reads report not found.
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

// Create is on the SendWait path; see the EventStore interface doc.
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
	evt, err := r.ax.SendWait(ctx, commands.SetPermissionLevel{ChatID: chatID, Level: level, Explicit: false})
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

// RunIn is SetWorkspace for a chat that shares the workspace it joins.
func (r *eventSourced) RunIn(
	ctx context.Context,
	chatID string,
	workspaceID string,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.SetWorkspace{ID: chatID, WorkspaceID: workspaceID, Shares: true})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: run in: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

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

// mapNotFound maps the read model's and the event-log fold's not-found sentinels
// to ErrNotFound, so callers see one sentinel whichever path served the miss.
func mapNotFound(
	err error,
) error {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, asynxModels.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
