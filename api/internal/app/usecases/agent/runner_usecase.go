package agent

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// RunnerUsecase owns the vendor CLI: starting one on a chat, replacing it,
// resuming it, stopping it, and delivering a React-authored prompt to it.
//
// Every path in here that starts or ends a CLI on a chat holds that chat's
// spawn gate, which is the only thing that can stop two concurrent switches
// putting two CLIs on one chat (see chatGate). The gate is NEVER taken on the
// hook path, and the destructive paths park on the in-flight turn instead of
// decapitating it.
type RunnerUsecase interface {
	// SpawnChat mints a chat and starts a CLI on it in one act, returning both
	// ids. A refused spawn takes the half-created chat with it.
	SpawnChat(
		ctx context.Context,
		workspaceID string,
		providerID string,
	) (chatID, runnerID string, err error)

	// StartRunner starts a CLI on a chat that already exists.
	StartRunner(
		ctx context.Context,
		chatID string,
		providerID string,
	) (string, error)

	// ResumeChat revives a dormant chat on the provider that last held it,
	// returning the id of the runner now on it. A chat that already has a live
	// runner is returned unchanged.
	ResumeChat(
		ctx context.Context,
		chatID string,
	) (string, error)

	// StopChat quits the CLI on a chat, leaving the chat dormant. A chat with no
	// live runner is already stopped.
	StopChat(
		ctx context.Context,
		chatID string,
	) error

	// SwitchProvider replaces the chat's CLI with one from another provider,
	// handing the incoming CLI the conversation so far. It WAITS for any in-flight
	// turn rather than quitting a CLI mid-answer, and refuses a disabled target
	// before anything is torn down.
	SwitchProvider(
		ctx context.Context,
		chatID string,
		targetProviderID string,
	) (string, error)

	// SubmitPrompt delivers a React-authored prompt to the chat's CLI. Delivery is
	// at-most-once against clientRequestID: a retry replays the original outcome
	// rather than prompting twice.
	SubmitPrompt(
		ctx context.Context,
		chatID, text, clientRequestID string,
	) (domain.AgentPromptSubmission, error)

	// SlashCatalog probes the chat's live CLI for the slash commands it declares.
	// A probe superseded by a newer one on the same chat is refused rather than
	// answered late.
	SlashCatalog(
		ctx context.Context,
		chatID string,
	) (engineagents.SlashCatalog, error)

	// LiveRunnerForChat returns the runner currently placed on a chat.
	LiveRunnerForChat(
		ctx context.Context,
		chatID string,
	) (domain.AgentRunner, error)

	// ConversationsForChat lists every provider conversation ever bound to a chat,
	// oldest first.
	ConversationsForChat(
		ctx context.Context,
		chatID string,
	) ([]domain.ChatConversation, error)

	// ReconcileRunnersOnBoot Exits every recorded runner whose PTY did not survive
	// the restart, closes the turns they died in, and recovers their prompt
	// journals.
	ReconcileRunnersOnBoot(
		ctx context.Context,
	) error

	// PendingDelivery reports the prompt delivery a chat is still waiting on a
	// turn for.
	PendingDelivery(
		ctx context.Context,
		chatID string,
	) (termwait.Delivery, bool)

	// SettleDelivery retires a prompt delivery that produced no turn, after one
	// last look for ledger evidence that it was in fact accepted. The bool reports
	// whether anything was retired.
	SettleDelivery(
		ctx context.Context,
		chatID, requestID string,
	) (bool, error)
}

var _ RunnerUsecase = (*runnerUsecase)(nil)

type runnerUsecase struct {
	chats    agentchat.EventStore
	runners  agentrunner.EventStore
	activity agentactivity.EventStore
	agents   engineagents.Agents
	term     TerminalCommander
	ws       WorkspaceReader
	// spawns serialises the USER-INITIATED spawn paths per chat (SpawnChat,
	// SwitchProvider, ResumeChat, StopChat, SubmitPrompt). See chatGate: it is the
	// only thing that can stop two concurrent switches putting two CLIs on one
	// chat, and it is NEVER taken on the hook path.
	spawns *chatGate
	// turns is the in-flight-turn registry this type BLOCKS on before it quits a
	// CLI. See turnWaits.
	turns *turnWaits
	// turnStarts makes a hook's durable turn start atomic with the final
	// idle-check-and-displace section of destructive TUI replacement.
	turnStarts *chatGate
	work       *chatWorkStates
	// prompts is the durable at-most-once React-submission journal and the
	// process-local transition lock shared with user_prompt hook confirmation.
	prompts agentjournal.PromptRequests
	// pendingHooks is the fork-before-runner-persistence barrier this type
	// installs before the fork and finishes after the runner row exists.
	pendingHooks *pendingRunnerHooks
	// catalogs owns only cancellation for in-flight deterministic probes. Results
	// are deliberately never cached.
	catalogs *catalogRuns
	// minter issues the per-runner token an MCP call is authenticated by. It is
	// the SAME instance providers holds: a runner's token must be minted by the
	// same secret DispatchMCP verifies against.
	minter    *agenttools.TokenMinter
	chat      *chatUsecase
	turn      *turnUsecase
	answers   *answerUsecase
	providers *providerUsecase
	// promptSettled fans out the edge where a delivery is retired without ever
	// having produced a turn. Wired at sweep start rather than at construction,
	// because the thing it publishes through is the hub — a layer above this one.
	// Nil until then, and nil forever in a daemon with no detector.
	promptSettled func(chatID, workspaceID, requestID string)
}

func newRunnerUsecase(
	chats agentchat.EventStore,
	runners agentrunner.EventStore,
	activity agentactivity.EventStore,
	agents engineagents.Agents,
	term TerminalCommander,
	ws WorkspaceReader,
	spawns *chatGate,
	turns *turnWaits,
	turnStarts *chatGate,
	work *chatWorkStates,
	prompts agentjournal.PromptRequests,
	pendingHooks *pendingRunnerHooks,
	catalogs *catalogRuns,
	minter *agenttools.TokenMinter,
	answers *answerUsecase,
) *runnerUsecase {
	return &runnerUsecase{
		chats:        chats,
		runners:      runners,
		activity:     activity,
		agents:       agents,
		term:         term,
		ws:           ws,
		spawns:       spawns,
		turns:        turns,
		turnStarts:   turnStarts,
		work:         work,
		prompts:      prompts,
		pendingHooks: pendingHooks,
		catalogs:     catalogs,
		minter:       minter,
		answers:      answers,
	}
}
