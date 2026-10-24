package runner

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Conversations is what starting, replacing and prompting a CLI needs from the
// chat record: what the chat has chosen, what it has said, and — when a spawn
// fails — the ability to erase the chat it was for.
//
// It is declared here, by the consumer, so the two never import each other.
type Conversations interface {
	// ChatProviderID is the provider the chat's next CLI should be, resolved from
	// its selection and the live provider order.
	ChatProviderID(
		ctx context.Context,
		chatID string,
	) (string, error)
	// ChatSelection is the model and reasoning effort pinned to the chat. create
	// says the chat is being minted right now, so there is nothing to read.
	ChatSelection(
		ctx context.Context,
		chatID string,
		create bool,
	) (engineagents.Selection, error)
	// SetChatSelection pins the model and effort the chat's next CLI launches
	// with, refusing a value the resolved provider does not declare.
	SetChatSelection(
		ctx context.Context,
		chatID string,
		model, effort string,
	) error
	// ChatTurns is the chat's turns, read to decide whether a resumed CLI has
	// anything to be told about.
	ChatTurns(
		ctx context.Context,
		chatID string,
	) ([]domain.LedgerTurn, error)
	// AssembleConversation renders the prior exchange a replacement CLI is handed.
	// resuming says the incoming CLI will re-read the provider's own transcript, so
	// only what happened after leftAt needs restating.
	AssembleConversation(
		ctx context.Context,
		chatID string,
		resuming bool,
		leftAt time.Time,
	) (string, error)
	// ThreadContext is the prompt fragment naming the chats this one reads.
	// minting says the chat is being created now and can have no lineage yet.
	ThreadContext(
		ctx context.Context,
		chatID string,
		minting bool,
	) (string, error)
	// PurgeLocked erases a chat whose caller ALREADY holds its lock, and
	// returns the end of its processes. A spawn that fails discards the chat it
	// minted, from inside that hold.
	PurgeLocked(
		ctx context.Context,
		chatID string,
	) (end func(), err error)
	// SeedPermissionLevel durably writes the current global default onto a
	// freshly created chat — see Conversations.MintChat's own use of this,
	// which SpawnChat and moveToNewChat mirror since neither can call
	// MintChat itself (both pre-generate chatID before the row exists).
	SeedPermissionLevel(
		ctx context.Context,
		chatID string,
	)
}

// AncestorCwd answers where a workspace-less chat's CLI runs: the nearest
// workspace-owning ancestor's id, or false when the row has none — the tree
// usecase's own tree.CwdWorkspaceID ancestor walk (internal/tree, a peer this
// package may not import; see aliases_test.go's layering rule).
//
// Declared here, by the consumer, so the two never import each other.
type AncestorCwd interface {
	ResolveCwdWorkspaceID(
		ctx context.Context,
		chatID string,
	) (string, bool, error)
}

// Turns is what the CLI lifecycle needs from the hook side: an api connection's
// door in, the record of a runner's streamed text at its exit, and the screen
// classifiers. Only Hook takes the chat's lock, never called under it.
type Turns interface {
	// Hook places one event an api connection resolved, as the hook ingress
	// places every hook, and returns the ask its answer is held on.
	Hook(ctx context.Context, runnerID, provider, event string, raw []byte) (ask string, err error)
	// ForgetChat drops every piece of in-memory turn state held for chatID.
	ForgetChat(chatID string)
	// ExitRows are the rows a runner's end records of the answers it streamed
	// that no row holds; they commit with the end, and Retire follows.
	ExitRows(st ledger.State, chatID, runnerID string) ([]ledger.Row, error)
	// Retire drops runnerID's streamed text on chatID once its end is recorded.
	Retire(chatID, runnerID string)

	// The seams the terminal-wait detector reads through: they belong to the
	// hook side.
	termwait.Prompts
	termwait.Notices
	// FailTurnOnNotice records runner's running turn failed for a notice on
	// its screen that ends a turn, which no hook reports.
	FailTurnOnNotice(ctx context.Context, runner engineagents.Runner, notice engineagents.TerminalNotice)
}

// Providers is the provider table as the spawn and switch paths consult it: may
// this provider be used at all, and may its CLI call back into Crowbar.
type Providers interface {
	// RequireProviderEnabled refuses a spawn onto a provider the user switched off.
	RequireProviderEnabled(
		ctx context.Context,
		providerID string,
	) error
	// ProviderMCPEnabled reports whether the provider may use the tool surface, so
	// a spawn knows whether to render the MCP config at all.
	ProviderMCPEnabled(
		ctx context.Context,
		providerID string,
	) (bool, error)
}
