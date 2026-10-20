// Package conversation owns the chat record: the aggregate's identity, title and
// model selection, and every read served off the conversation it accumulates.
//
// It starts no processes. A chat exists, and is readable, whether or not a CLI
// has ever run on it — minting one, renaming it and reading its log are all
// answerable with no runner in sight, which is what makes them separable from the
// runner lifecycle at all.
//
// The one thing it does owe the processes on a chat is the hard delete: erasing
// the aggregate has to retire the CLIs pointed at it, which it asks the runner
// lifecycle to do through a port rather than reaching for it.
package conversation

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Runners is what this package needs from the runner lifecycle: the live
// runner a hook or a provider read names, and the CLI a hard delete retires.
//
// It is declared here, by the consumer, so the two never import each other. A
// chat's erasure decides that the processes on it must go; how they are torn
// down is not its business.
type Runners interface {
	// RunnerByID is the live runner runnerID, also while it is exiting.
	RunnerByID(runnerID string) (engineagents.Runner, bool)
	// LiveRunnerForChat is the chat's live runner; an error when it is dormant.
	LiveRunnerForChat(ctx context.Context, chatID string) (engineagents.Runner, error)
	// RetireChatRunners records the end of the CLI on the chat, under its lock,
	// and returns the end of its processes, run once the lock is let go. It is
	// best-effort: a chat is erased whether or not its processes are reached.
	RetireChatRunners(
		ctx context.Context,
		chatID string,
	) (end func())
	// HoldChat cancels the chat's parked user-initiated waits and takes its
	// lock: a purge never queues behind a wait on a chat that is going away.
	HoldChat(ctx context.Context, chatID string) (release func(), err error)
}

// Conversations is the chat record.
type Conversations struct {
	chats agentchat.EventStore
	// activity is the conversation record: turns, tool calls, subagents and
	// interruptions. Every read this type serves comes off it.
	activity  agentactivity.EventStore
	telemetry *telemetry.Store
	agents    engineagents.Agents
	ws        seam.WorkspaceReader
	// lineage answers "what does this chat read" at spawn time. See ThreadContext.
	lineage seam.ChatLineage
	// home is the app-config crowbar-home resolver, NOT a wsId lookup: it resolves
	// the descriptor catalog a chat's model/effort selection is validated against.
	home   func() (string, error)
	ledger *ledgerstore.Store
	// defaultPermissionLevel resolves the current global default at mint time
	// — Crowbar's own guarded/trusted/full-auto name, not yet checked against
	// any provider (a bare-minted chat has none). It's a closure, not a
	// direct usecase reference, the same way home is a closure — this
	// package must not import the chat usecase package it lives inside of.
	defaultPermissionLevel func(ctx context.Context) (string, error)
	// runners is the runner lifecycle: the live runner a read names, and the
	// retirement a hard delete owes the processes on the chat. It is bound AFTER construction
	// because the two sides are built together and neither can name the other
	// first — see SetRunners.
	runners Runners
}

// Deps is everything the chat record is built over.
type Deps struct {
	Chats     agentchat.EventStore
	Activity  agentactivity.EventStore
	Telemetry *telemetry.Store
	Agents    engineagents.Agents
	Workspace seam.WorkspaceReader
	Lineage   seam.ChatLineage
	Home      func() (string, error)
	Ledger    *ledgerstore.Store

	// DefaultPermissionLevel resolves the current global default at the
	// moment a chat is minted. It's a closure, not a direct usecase
	// reference, the same way Home is a closure — conversation must not
	// import the chat usecase package it lives inside of.
	DefaultPermissionLevel func(ctx context.Context) (string, error)
}

// New builds the chat record. The runner port is bound separately, by SetRunners.
func New(d Deps) *Conversations {
	return &Conversations{
		chats:     d.Chats,
		activity:  d.Activity,
		telemetry: d.Telemetry,
		agents:    d.Agents,
		ws:        d.Workspace,
		lineage:   d.Lineage,
		home:      d.Home,
		ledger:    d.Ledger,

		defaultPermissionLevel: d.DefaultPermissionLevel,
	}
}

// SetRunners binds the runner lifecycle a hard delete retires through.
//
// It is a setter and not a constructor argument because the erasure edge is a
// cycle: a purge retires the CLIs on a chat, and a spawn that fails discards the
// half-created chat it was for. Building both and then closing the edge is the
// honest expression of that; a constructor that could take it would be lying
// about which one exists first.
func (c *Conversations) SetRunners(runners Runners) { c.runners = runners }

func (c *Conversations) RenameChat(
	ctx context.Context,
	chatID, title, source string,
) error {
	if title == "" {
		return nil
	}
	chat, err := c.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: rename chat: get: %w", err)
	}
	switch source {
	case "derived":
		if chat.Title != "" {
			return nil
		}
	case "agent":
		if chat.TitleLocked {
			return nil
		}
	default: // "user" / "" — manual rename wins and locks
		source = "user"
	}
	if _, err := c.chats.SetTitle(ctx, chatID, title, source); err != nil {
		return fmt.Errorf("agent: rename chat: save: %w", err)
	}
	return nil
}

func (c *Conversations) RenameByRunner(
	ctx context.Context,
	runnerID, title, source string,
) error {
	runner, ok := c.runners.RunnerByID(runnerID)
	if !ok {
		return fmt.Errorf("agent: rename by runner: runner %s is not live: %w", runnerID, apperr.ErrNotFound)
	}
	return c.RenameChat(ctx, runner.CurrentChatID, title, source)
}

func (c *Conversations) PurgeChat(
	ctx context.Context,
	chatID string,
) error {
	// The chat's lock, as the spawn paths take it: a delete racing a switch would
	// otherwise start a CLI onto a chat that has just been forgotten, and a hook
	// would record onto it.
	release, err := c.runners.HoldChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: purge chat: %w", err)
	}
	end, err := c.PurgeLocked(ctx, chatID)
	release()
	end()
	return err
}

// The caller already holds chatID's lock: PurgeChat above takes it, and the
// spawn path reaches this from INSIDE its hold (discardSpawnedChat). The lock
// is not reentrant, so wiring either caller to PurgeChat instead deadlocks.
// end ends the chat's processes; the caller runs it once it lets the lock go.
func (c *Conversations) PurgeLocked(
	ctx context.Context,
	chatID string,
) (end func(), err error) {
	if _, err := c.chats.GetChat(ctx, chatID); err != nil {
		return func() {}, fmt.Errorf("agent: purge chat: get: %w", err)
	}
	// Retire first, so the chat's feed still carries its runner's exit.
	end = c.runners.RetireChatRunners(ctx, chatID)
	return end, c.forget(ctx, chatID)
}

// forget erases chatID's record, conversation, ledger and directory.
func (c *Conversations) forget(ctx context.Context, chatID string) error {
	if err := c.chats.Forget(ctx, chatID); err != nil {
		return fmt.Errorf("agent: purge chat: forget: %w", err)
	}

	// Drop the conversation record and its telemetry. The record outlives the
	// process and nothing else removes it, so a hard delete that skipped it would
	// leave the chat's plaintext readable after the user asked for it to be gone.
	if err := c.activity.Forget(ctx, chatID); err != nil {
		slog.ErrorContext(ctx, "agent: purge chat: forget conversation record (best-effort, continuing)",
			"chat_id", chatID, "err", err)
	}
	c.telemetry.Forget(ctx, chatID)

	if err := c.ledger.Forget(ctx, chatID); err != nil {
		slog.ErrorContext(ctx, "agent: purge chat: forget ledger (best-effort, continuing)",
			"chat_id", chatID, "err", err)
	}

	// Reap the chat's on-disk footprint now the aggregate is gone. The conversation
	// itself no longer lives here — it is in the record dropped above — but a chat
	// directory may still hold whatever a spawn left in it.
	//
	// NOT the runner's tmp dir, which no longer lives under the chat (worktreepath.RunnerDir)
	// and is no business of the chat's: it belongs to the PROCESS lifecycle, and the process
	// we have just SIGTERM'd is still alive and still reading it. Removing a live CLI's
	// config out from under it was only ever an accident of the old layout. It goes when the
	// PTY does (onExit), or at the next boot if the daemon died first.
	//
	// Resolved via c.home, NOT c.ws.AgentChatsDir(chat.WorkspaceID): this is the chat's
	// own directory (worktreepath.LedgerChatsDir, not chat_ledger.db, forgotten above),
	// keyed by the chat's id alone because WorkspaceID is optional and mutable (spec §1.5) —
	// a workspace lookup would error for a bubble and could target the wrong directory for a
	// chat that has since been promoted. RemoveUnderHome re-asserts the target is strictly
	// under crowbar home regardless.
	home, err := c.home()
	if err != nil {
		slog.WarnContext(ctx, "agent: purge chat: resolve home for reap (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return nil
	}
	// Best-effort: a reap that cannot happen leaves disk behind; failing the purge
	// would leave a chat the user asked to erase still listed.
	if err := worktreepath.RemoveUnderHome(home, filepath.Join(worktreepath.LedgerChatsDir(home), chatID)); err != nil {
		slog.WarnContext(ctx, "agent: purge chat: reap its chat directory (best-effort)", "chat_id", chatID, "err", err)
	}
	return nil
}

func (c *Conversations) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	return c.chats.ListChats(ctx)
}

func (c *Conversations) GetChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	return c.chats.GetChat(ctx, id)
}

// providerID is the vendor the chat is BORN on, written durably here rather than
// waiting for the spawn that follows. It is the difference between a chat that
// can always say what it runs and one that can only say so once a CLI has
// actually started on it: the spawn-time SetProvider (runner.recordChatProvider)
// is best-effort and runs after a fork that may never happen, so a create that
// failed at the spawn — or predated that write entirely — left the field empty
// forever, and a dormant chat with no conversation row and no switch marker then
// had nothing anywhere that named its provider. Writing it at birth closes that
// at the source; the spawn-time write remains correct, and is what records a
// later provider SWITCH.
//
// "" is the honest value for a chat minted with no vendor in mind at all — an
// owning/placeholder row, which never gets a CLI.
//
// surface is the VIEW this chat is born on (design spec 2.5) — "" for the
// provider's own default landing. See domain.Chat.Surface for why it is
// durable rather than a one-shot spawn argument.
func (c *Conversations) MintChat(
	ctx context.Context,
	workspaceID string,
	providerID string,
	surface string,
) (string, error) {
	chatID := uuid.NewString()
	_, err := c.chats.Create(ctx, agentchat.CreateInput{
		ID:          chatID,
		WorkspaceID: workspaceID,
		Type:        domain.ChatTypeChat,
		ProviderID:  providerID,
		Surface:     surface,
		Now:         time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("agent: mint chat: %w", err)
	}
	c.SeedPermissionLevel(ctx, chatID)
	return chatID, nil
}

// SeedPermissionLevel durably writes the CURRENT global default onto a
// freshly created chat — the RAW, provider-blind choice, never a
// provider-clamped one, and NON-explicit: it seeds a display value for a
// chat that has not opted out of the dial, it does not pin one. ChatSelection
// re-resolves the LIVE global default for this chat on every future spawn
// until (if ever) SetChatPermissionLevel makes an explicit choice — so a
// chat that later switches providers still has its own real, CURRENT intent
// to resolve against, not whatever an earlier provider happened to clamp it
// to, nor whatever the default was on the day it was minted. A read failure
// is swallowed, not propagated: the chat must still get created even if the
// lookup has trouble, and an unseeded chat still answers something sane —
// see the write failure's own best-effort logging for why the write side is
// the same story.
func (c *Conversations) SeedPermissionLevel(
	ctx context.Context,
	chatID string,
) {
	level, err := c.defaultPermissionLevel(ctx)
	if err != nil {
		return
	}
	if _, err := c.chats.SeedPermissionLevel(ctx, chatID, level); err != nil {
		slog.WarnContext(ctx, "agent: seed permission level (best-effort, continuing)",
			"chat_id", chatID, "err", err)
	}
}

func (c *Conversations) NoteThreadLineage(
	ctx context.Context,
	chatID string,
	ancestors []string,
) error {
	chat, err := c.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: note thread lineage: chat: %w", err)
	}
	turns, err := c.ChatTurns(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: note thread lineage: turns: %w", err)
	}
	if len(turns) == 0 {
		return nil
	}
	return c.appendTurn(ctx, chat, lineageNoteProvider, "user", lineageNoteText(ancestors))
}

const lineageNoteProvider = "crowbar"

func lineageNoteText(
	ancestors []string,
) string {
	return "[Crowbar] This chat was moved in the Chats panel and is a THREAD of " +
		strings.Join(ancestors, ", ") + " (nearest parent first) from this point on. " +
		"Read those chats with get_chat_log. Everything above this line was said BEFORE the move, " +
		"without any of that context: the move changes what this chat reads from now on and " +
		"rewrites nothing it has already read."
}

// NotePromotion records, in a chat's own conversation, that it has just been
// promoted from a bubble into its own worktree — the model spec §4.2 ledger
// note, following lineageNoteText's own convention: a "[Crowbar] ..." system
// turn appended AFTER the respawn it describes, so the incoming CLI's own
// handoff (assembled from the ledger BEFORE this call) never sees it.
//
// Like NoteThreadLineage, it writes nothing into a chat that has said nothing
// yet — a chat with no ledger has nothing for the note to distinguish a
// "before" from.
func (c *Conversations) NotePromotion(
	ctx context.Context,
	chatID string,
) error {
	chat, err := c.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: note promotion: chat: %w", err)
	}
	turns, err := c.ChatTurns(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: note promotion: turns: %w", err)
	}
	if len(turns) == 0 {
		return nil
	}
	return c.appendTurn(ctx, chat, lineageNoteProvider, "user", promotionNoteText())
}

func promotionNoteText() string {
	return "[Crowbar] This chat was just promoted to its own git worktree. " +
		"Everything above this line was said BEFORE the promotion, running with no worktree of its own; " +
		"from this point on it runs in the new worktree the promotion created."
}

// Ancestors returns the CHAT ancestors of chatID, nearest parent first — what a
// thread inherits, and the same answer the spawn path composes prior context
// from. Folders are transparent to it: a thread filed two folders deep under a
// chat inherits exactly what it would sitting directly under it.
//
// Empty for a chat at the panel root and for one merely filed in a folder;
// neither reads anything.
func (c *Conversations) Ancestors(
	ctx context.Context,
	chatID string,
) ([]string, error) {
	return c.lineage.Ancestors(ctx, chatID)
}

func (c *Conversations) ListChatsByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.Chat, error) {
	return c.chats.ListByWorkspace(ctx, workspaceID)
}
