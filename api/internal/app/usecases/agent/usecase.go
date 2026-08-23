package agent

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/app/chatlog"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Usecase is the agent surface the API layer holds: one object in front of five
// concerns — the chat aggregate, the activity record hooks write, the runner
// lifecycle, the human-in-the-loop answer desk and the provider table — each of
// which owns its own state and is reachable only through here.
//
// Every exported method on it is a delegate. The concerns call each other by
// ordinary field reference, which is why they are one package: the seams
// between them are real, but a call across one costs nothing and hides nothing.
type Usecase struct {
	chat      *chatUsecase
	turn      *turnUsecase
	runner    *runnerUsecase
	answers   *answerUsecase
	providers *providerUsecase
	// termWait detects the state no hook reports: a CLI parked on a modal Crowbar
	// cannot answer, which otherwise renders as an empty pane over a live process.
	// NIL when the terminal seam cannot render a screen, in which case every chat
	// reports the zero verdict — see newTerminalWaitDetector.
	termWait termwait.Detector
}

// New builds the agent usecase and every concern behind it.
//
// The shared machinery — the spawn gate, the turn registry, the work mirror,
// the turn-start interlock, the hook barrier, the message streams and the two
// journals — is constructed HERE, once, and the same value is handed to each
// concern that needs it. A second instance of any of them is a silent wedge: two
// turn registries means a switch parks on a turn nothing will ever complete, and
// two spawn gates means two CLIs on one chat.
func New(
	chats agentchat.EventStore,
	runners agentrunner.EventStore,
	activity agentactivity.EventStore,
	agents engineagents.Agents,
	term TerminalCommander,
	ws WorkspaceReader,
	lineage ChatLineage,
	providerPrefs store.Store[domain.AgentProviderPreference, string],
	home func() (string, error),
	installed func(a engineagents.Agent) bool,
	minter *agenttools.TokenMinter,
	tools agenttools.Deps,
) *Usecase {
	telemetry := newTelemetryStore()
	work := newChatWorkStates()
	spawns := newChatGate()
	turns := newTurnWaits()
	turnStarts := newChatGate()
	pendingHooks := newPendingRunnerHooks()
	answers := newAnswerUsecase(activity, chats, runners, agents, ws)

	u := &Usecase{
		chat: newChatUsecase(
			chats, runners, activity, telemetry, agents, ws, lineage, home, work, spawns,
		),
		turn: newTurnUsecase(
			chats, runners, activity, telemetry, agents, ws, home, work, turns, turnStarts,
			newMessageStreams(), pendingHooks, agentjournal.NewHookDeliveries(), newChatGate(),
			answers,
		),
		runner: newRunnerUsecase(
			chats, runners, activity, agents, term, ws, spawns, turns, turnStarts, work,
			agentjournal.NewPromptRequests(), pendingHooks, newCatalogRuns(), minter, answers,
		),
		answers: answers,
	}
	// The tool surface's two self-ports, filled in here because u does not exist
	// when the caller builds the Deps. newProviderUsecase adds the third,
	// ToolAccess, which it can only bind to a method of its own.
	tools.Chats = u
	tools.ChatLogs = u
	u.providers = newProviderUsecase(agents, home, installed, providerPrefs, minter, tools)

	// The concerns reach each other directly, and the graph has cycles in it (a
	// spawn discards its half-created chat; a purge retires the CLIs on it), so
	// the sibling ports are bound after all three exist rather than passed into
	// constructors that could not name each other.
	u.chat.runner = u.runner
	u.turn.chat = u.chat
	u.turn.runner = u.runner
	u.runner.chat = u.chat
	u.runner.turn = u.turn
	u.runner.providers = u.providers

	// Built LAST, from u rather than from the arguments: its ports are spread
	// across the concerns above and it binds them by value. It only observes —
	// nothing runs until StartTerminalWaitSweep is called.
	u.termWait = newTerminalWaitDetector(u)
	return u
}

// TerminalWait reports whether a chat's CLI is parked on a modal terminal
// prompt, and on which kind. A daemon whose terminal seam cannot render a
// screen always answers "not waiting".
func (u *Usecase) TerminalWait(chatID string) domain.AgentTerminalWait {
	if u.termWait == nil {
		return domain.AgentTerminalWait{}
	}
	return u.termWait.Wait(chatID)
}

// StartTerminalWaitSweep starts the screen sweep and wires the two publish
// callbacks the hub owns: promptSettled onto the runner concern, messageDelta
// onto the activity one.
//
// Both are assigned BEFORE the nil-detector return. A daemon with no detector
// still streams assistant messages to its chat UI, and dropping messageDelta on
// that path is invisible until a user watches a message that never grows.
func (u *Usecase) StartTerminalWaitSweep(
	ctx context.Context,
	publish func(chatID, workspaceID string, wait domain.AgentTerminalWait),
	promptSettled func(chatID, workspaceID, requestID string),
	messageDelta func(chatID, workspaceID, messageID, text string),
) {
	u.runner.promptSettled = promptSettled
	u.turn.messageDelta = messageDelta
	if u.termWait == nil {
		return
	}
	u.termWait.Run(ctx, publish)
}

// MintChat creates an empty chat in a workspace. It delegates to ChatUsecase.
func (u *Usecase) MintChat(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	return u.chat.MintChat(ctx, workspaceID)
}

// RenameChat retitles a chat according to where the title came from. It
// delegates to ChatUsecase.
func (u *Usecase) RenameChat(
	ctx context.Context,
	chatID, title, source string,
) error {
	return u.chat.RenameChat(ctx, chatID, title, source)
}

// RenameByRunner retitles whichever chat a runner is placed on. It delegates to
// ChatUsecase.
func (u *Usecase) RenameByRunner(
	ctx context.Context,
	runnerID, title, source string,
) error {
	return u.chat.RenameByRunner(ctx, runnerID, title, source)
}

// PurgeChat hard-deletes a chat and everything recorded under it. It delegates
// to ChatUsecase.
func (u *Usecase) PurgeChat(
	ctx context.Context,
	chatID string,
) error {
	return u.chat.PurgeChat(ctx, chatID)
}

// ListChats returns every chat the daemon knows. It delegates to ChatUsecase.
func (u *Usecase) ListChats(
	ctx context.Context,
) ([]domain.AgentChat, error) {
	return u.chat.ListChats(ctx)
}

// ListChatsByWorkspace returns one workspace's chats. It delegates to
// ChatUsecase.
func (u *Usecase) ListChatsByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.AgentChat, error) {
	return u.chat.ListChatsByWorkspace(ctx, workspaceID)
}

// GetChat reads one chat aggregate. It delegates to ChatUsecase.
func (u *Usecase) GetChat(
	ctx context.Context,
	id string,
) (domain.AgentChat, error) {
	return u.chat.GetChat(ctx, id)
}

// SetChatSelection records the model and effort the chat's next CLI is launched
// with. It delegates to ChatUsecase.
func (u *Usecase) SetChatSelection(
	ctx context.Context,
	chatID string,
	model string,
	effort string,
) error {
	return u.chat.SetChatSelection(ctx, chatID, model, effort)
}

// ReadChatLog renders a chat's whole conversation as speaker/body turns. It
// delegates to ChatUsecase.
func (u *Usecase) ReadChatLog(
	ctx context.Context,
	chatID string,
) ([]agenttools.ChatTurn, error) {
	return u.chat.ReadChatLog(ctx, chatID)
}

// ReadMessages pages a chat's conversation record. It delegates to ChatUsecase.
func (u *Usecase) ReadMessages(
	ctx context.Context,
	chatID string,
	after, before, limit int,
) (chatlog.Page, error) {
	return u.chat.ReadMessages(ctx, chatID, after, before, limit)
}

// NoteThreadLineage records a chat's move under new parents in its own
// conversation. It delegates to ChatUsecase.
func (u *Usecase) NoteThreadLineage(
	ctx context.Context,
	chatID string,
	ancestors []string,
) error {
	return u.chat.NoteThreadLineage(ctx, chatID, ancestors)
}

// AssembleHandoff renders the conversation an incoming CLI is spawned with. It
// delegates to ChatUsecase.
func (u *Usecase) AssembleHandoff(
	ctx context.Context,
	chatID string,
) (string, error) {
	return u.chat.AssembleHandoff(ctx, chatID)
}

// IngestHook applies one canonical hook event from a runner's CLI. It delegates
// to TurnUsecase.
func (u *Usecase) IngestHook(
	ctx context.Context,
	runnerID string,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	return u.turn.IngestHook(ctx, runnerID, provider, canonicalEvent, rawPayload)
}

// IngestHookDelivery is the exactly-once ingress for one relayed hook. It
// delegates to TurnUsecase.
func (u *Usecase) IngestHookDelivery(
	ctx context.Context,
	workspaceID, deliveryID, runnerID, provider, canonicalEvent string,
	rawPayload []byte,
) error {
	return u.turn.IngestHookDelivery(
		ctx, workspaceID, deliveryID, runnerID, provider, canonicalEvent, rawPayload,
	)
}

// ReadActivity pages a chat's tool calls, subagents, interruptions and choices.
// It delegates to TurnUsecase.
func (u *Usecase) ReadActivity(
	ctx context.Context,
	chatID string,
	after int64,
	limit int,
) (ChatActivity, error) {
	return u.turn.ReadActivity(ctx, chatID, after, limit)
}

// ReadPendingChoices lists the questions a chat's CLI is still blocked on. It
// delegates to TurnUsecase.
func (u *Usecase) ReadPendingChoices(
	ctx context.Context,
	chatID string,
) ([]domain.ActivityChoice, error) {
	return u.turn.ReadPendingChoices(ctx, chatID)
}

// ReadToolPayload returns one tool call's stored request or result body. It
// delegates to TurnUsecase.
func (u *Usecase) ReadToolPayload(
	ctx context.Context,
	chatID, toolID, side string,
) ([]byte, error) {
	return u.turn.ReadToolPayload(ctx, chatID, toolID, side)
}

// Telemetry reports the newest provider telemetry for a chat. It delegates to
// TurnUsecase.
func (u *Usecase) Telemetry(chatID string) (engineagents.Telemetry, bool) {
	return u.turn.Telemetry(chatID)
}

// OpenWork reports whether a chat has a tool call or subagent still running. It
// delegates to TurnUsecase.
func (u *Usecase) OpenWork(ctx context.Context, chatID string) (bool, error) {
	return u.turn.OpenWork(ctx, chatID)
}

// UnfinishedSince reports when a chat's assistant stream last grew. It
// delegates to TurnUsecase.
func (u *Usecase) UnfinishedSince(chatID string) (time.Time, bool) {
	return u.turn.UnfinishedSince(chatID)
}

// AbandonMessage records a cut-off assistant message and closes its turn. It
// delegates to TurnUsecase.
func (u *Usecase) AbandonMessage(ctx context.Context, chatID string) (bool, error) {
	return u.turn.AbandonMessage(ctx, chatID)
}

// MatchTerminalPrompt asks a provider's descriptor whether a screen is one of
// its modal prompts. It delegates to TurnUsecase.
func (u *Usecase) MatchTerminalPrompt(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalPrompt, bool) {
	return u.turn.MatchTerminalPrompt(ctx, providerID, screen)
}

// MatchTerminalNotice asks a provider's descriptor whether a screen carries a
// recordable notice. It delegates to TurnUsecase.
func (u *Usecase) MatchTerminalNotice(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalNotice, bool) {
	return u.turn.MatchTerminalNotice(ctx, providerID, screen)
}

// SpawnChat mints a chat and starts a CLI on it. It delegates to RunnerUsecase.
func (u *Usecase) SpawnChat(
	ctx context.Context,
	workspaceID string,
	providerID string,
) (chatID, runnerID string, err error) {
	return u.runner.SpawnChat(ctx, workspaceID, providerID)
}

// StartRunner starts a CLI on an existing chat. It delegates to RunnerUsecase.
func (u *Usecase) StartRunner(
	ctx context.Context,
	chatID string,
	providerID string,
) (string, error) {
	return u.runner.StartRunner(ctx, chatID, providerID)
}

// ResumeChat revives a dormant chat on the provider that last held it. It
// delegates to RunnerUsecase.
func (u *Usecase) ResumeChat(
	ctx context.Context,
	chatID string,
) (string, error) {
	return u.runner.ResumeChat(ctx, chatID)
}

// StopChat quits the CLI on a chat, leaving it dormant. It delegates to
// RunnerUsecase.
func (u *Usecase) StopChat(
	ctx context.Context,
	chatID string,
) error {
	return u.runner.StopChat(ctx, chatID)
}

// SwitchProvider replaces the chat's CLI with one from another provider. It
// delegates to RunnerUsecase.
func (u *Usecase) SwitchProvider(
	ctx context.Context,
	chatID string,
	targetProviderID string,
) (string, error) {
	return u.runner.SwitchProvider(ctx, chatID, targetProviderID)
}

// SubmitPrompt delivers a React-authored prompt to the chat's CLI. It delegates
// to RunnerUsecase.
func (u *Usecase) SubmitPrompt(
	ctx context.Context,
	chatID, text, clientRequestID string,
) (domain.AgentPromptSubmission, error) {
	return u.runner.SubmitPrompt(ctx, chatID, text, clientRequestID)
}

// SlashCatalog probes a chat's live CLI for the slash commands it declares. It
// delegates to RunnerUsecase.
func (u *Usecase) SlashCatalog(
	ctx context.Context,
	chatID string,
) (engineagents.SlashCatalog, error) {
	return u.runner.SlashCatalog(ctx, chatID)
}

// LiveRunnerForChat returns the runner currently placed on a chat. It delegates
// to RunnerUsecase.
func (u *Usecase) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (domain.AgentRunner, error) {
	return u.runner.LiveRunnerForChat(ctx, chatID)
}

// ConversationsForChat lists every provider conversation ever bound to a chat.
// It delegates to RunnerUsecase.
func (u *Usecase) ConversationsForChat(
	ctx context.Context,
	chatID string,
) ([]domain.ChatConversation, error) {
	return u.runner.ConversationsForChat(ctx, chatID)
}

// ReconcileRunnersOnBoot repairs the runner and prompt state a restart left
// behind. It delegates to RunnerUsecase.
func (u *Usecase) ReconcileRunnersOnBoot(
	ctx context.Context,
) error {
	return u.runner.ReconcileRunnersOnBoot(ctx)
}

// PendingDelivery reports the prompt delivery a chat is still waiting on a turn
// for. It delegates to RunnerUsecase.
func (u *Usecase) PendingDelivery(ctx context.Context, chatID string) (termwait.Delivery, bool) {
	return u.runner.PendingDelivery(ctx, chatID)
}

// SettleDelivery retires a prompt delivery that produced no turn. It delegates
// to RunnerUsecase.
func (u *Usecase) SettleDelivery(ctx context.Context, chatID, requestID string) (bool, error) {
	return u.runner.SettleDelivery(ctx, chatID, requestID)
}
