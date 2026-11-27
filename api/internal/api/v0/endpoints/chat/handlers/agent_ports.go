package handlers

import (
	"context"
	"io"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

type ChatUsecase interface {
	ChatSnapshot(
		ctx context.Context,
		chatID string,
	) (agentusecase.ChatSnapshot, error)

	ChatSnapshotsOf(
		ctx context.Context,
		chatIDs []string,
	) ([]agentusecase.ChatSnapshot, error)

	ListChatsByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)

	ListChatsInRepo(
		ctx context.Context,
		repoID string,
	) ([]domain.Chat, error)

	CwdWorkspaceID(
		ctx context.Context,
		chatID string,
	) (workspaceID string, ok bool, err error)

	GetChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)

	AssembleHandoff(
		ctx context.Context,
		chatID string,
	) (string, error)

	RenameChat(
		ctx context.Context,
		chatID, title, source string,
	) error

	SetChatSelection(
		ctx context.Context,
		chatID, model, effort string,
	) error

	PurgeChat(
		ctx context.Context,
		chatID string,
	) error
}

type TurnUsecase interface {
	Hook(
		ctx context.Context,
		runnerID, provider, event string,
		raw []byte,
		channel engineagents.Channel,
	) (agentusecase.HookReply, error)

	Telemetry(chatID string) (engineagents.Telemetry, bool)
	TelemetryOnChatSurface(ctx context.Context, chatID string) bool

	UploadAttachment(
		ctx context.Context,
		chatID string,
		in agentusecase.UploadAttachmentInput,
	) (agentusecase.StoredAttachment, error)

	ReadAttachment(
		ctx context.Context,
		chatID, fileName string,
	) ([]byte, string, error)

	BeginAttachmentUpload(ctx context.Context, chatID, id, fileName string, size int64) (int64, error)
	AppendAttachmentUpload(ctx context.Context, chatID, id string, offset int64, chunk io.Reader) (int64, error)
	CompleteAttachmentUpload(ctx context.Context, chatID, id, fileName string, size int64) (agentusecase.StoredAttachment, error)
	AbortAttachmentUpload(ctx context.Context, chatID, id string) error
}

type RunnerUsecase interface {
	SubmitPrompt(
		ctx context.Context,
		chatID, text, clientRequestID, provider string,
		selection *domain.ChatSelection,
	) (domain.AgentPromptSubmission, error)

	SlashCatalog(
		ctx context.Context,
		chatID string,
	) (engineagents.SlashCatalog, error)

	ConversationsForChat(
		ctx context.Context,
		chatID string,
	) ([]engineagents.ChatConversation, error)

	SwitchProvider(
		ctx context.Context,
		chatID string,
		targetProviderID string,
		surface string,
	) (newRunnerID string, err error)

	ResumeChat(
		ctx context.Context,
		chatID string,
	) (runnerID string, err error)

	StopChat(ctx context.Context, chatID string) error

	Retract(ctx context.Context, chatID, requestID string) (agentusecase.RetractResult, error)

	KillChat(ctx context.Context, chatID string) error

	Compact(
		ctx context.Context,
		chatID string,
	) error

	StopShell(ctx context.Context, chatID, shellID string) error

	SwitchToTerminal(
		ctx context.Context,
		chatID string,
	) (terminalSessionID string, err error)

	SwitchToNative(
		ctx context.Context,
		chatID string,
	) error

	AttachedTerminalSession(runnerID string) (string, bool)

	TerminalWait(
		chatID string,
	) domain.AgentTerminalWait
}

type AnswerUsecase interface {
	AnswerTerminalWait(ctx context.Context, chatID, answerID string) error

	AnswerChoice(
		ctx context.Context,
		chatID, choiceID string,
		optionIDs []string,
		reason string,
		content []byte,
	) error

	HoldsAnswer(askID string) bool

	AwaitAnswer(
		ctx context.Context,
		askID string,
	) (agentusecase.HookAnswer, error)

	SetChatPermissionLevel(
		ctx context.Context,
		chatID string,
		level string,
	) error
}

type ProviderUsecase interface {
	InstallDescriptor(context.Context, []byte) (descriptorcheck.Report, error)
	ResolveProviders(
		ctx context.Context,
	) ([]domain.AgentProvider, error)

	DescriptorReports(
		ctx context.Context,
	) ([]descriptorcheck.Report, error)

	ReplaceProviderPreferences(
		ctx context.Context,
		prefs []domain.AgentProviderPreference,
	) ([]domain.AgentProvider, error)

	DispatchMCP(
		ctx context.Context,
		runnerID string,
		token string,
		message []byte,
	) ([]byte, bool, error)

	DispatchRelayMCP(
		ctx context.Context,
		relayID string,
		token string,
		message []byte,
	) ([]byte, bool, error)

	VerifyRunnerToken(runnerID, token string) bool

	DefaultPermissionLevel(
		ctx context.Context,
	) (string, error)
	SetDefaultPermissionLevel(
		ctx context.Context,
		level string,
	) error

	ModelManifestFetchEnabled(
		ctx context.Context,
	) (bool, error)
	SetModelManifestFetchEnabled(
		ctx context.Context,
		enabled bool,
	) error
}
