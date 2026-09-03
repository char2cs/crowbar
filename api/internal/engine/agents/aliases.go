package agents

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type (
	// Runner is one live vendor-CLI process and ChatConversation is a conversation it
	// hosted. The engine owns both: they describe the PROCESS, not Crowbar's model of
	// the chat (design spec 3.1).
	Runner           = models.Runner
	ChatConversation = models.ChatConversation

	TemplateCtx    = models.TemplateCtx
	SpawnPlan      = models.SpawnPlan
	InjectStep     = spec.InjectStep
	Display        = models.Display
	Capabilities   = models.Capabilities
	CanonicalEvent = models.CanonicalEvent

	ToolEvent      = models.ToolEvent
	SubagentEvent  = models.SubagentEvent
	InterruptEvent = models.InterruptEvent
	ChoicePrompt   = models.ChoicePrompt
	PromptQuestion = models.PromptQuestion
	ChoiceOption   = models.ChoiceOption

	TerminalPrompt = models.TerminalPrompt

	AnswerCapability = models.AnswerCapability
	AnswerDecision   = models.AnswerDecision
	Decision         = models.Decision
	Selection        = models.Selection
	MoveKind         = models.MoveKind

	MessageDelta     = models.MessageDelta
	TurnFailure      = models.TurnFailure
	SlashCatalog     = models.SlashCatalog
	SlashCatalogItem = models.SlashCatalogItem
	ProbeOptions     = models.ProbeOptions

	Telemetry       = models.Telemetry
	ContextUsage    = models.ContextUsage
	RateLimitWindow = models.RateLimitWindow
	SessionCost     = models.SessionCost
	ModelIdentity   = models.ModelIdentity

	Acquire = exec.Acquire

	// APIEvent and APIConn are protocol's own wrappers around the api-transport
	// connection (see protocol.go's own comment on why they exist as a second
	// layer of indirection rather than re-exporting apidriver's types directly).
	APIEvent = protocol.APIEvent
	APIConn  = protocol.APIConn
)

const (
	MoveNoop    = models.MoveNoop
	MoveBind    = models.MoveBind
	MoveToNew   = models.MoveToNew
	MoveToKnown = models.MoveToKnown
)

const (
	HookSessionStart = spec.HookSessionStart
	HookUserPrompt   = spec.HookUserPrompt
	HookTurnStop     = spec.HookTurnStop
	HookToolPre      = spec.HookToolPre
	HookToolPost     = spec.HookToolPost
	HookSubagentPre  = spec.HookSubagentPre
	HookSubagentPost = spec.HookSubagentPost
	HookNotification = spec.HookNotification
	HookPermission   = spec.HookPermission
	HookCompactPre   = spec.HookCompactPre
	HookCompactPost  = spec.HookCompactPost
	HookSessionEnd   = spec.HookSessionEnd
	HookTelemetry    = spec.HookTelemetry
	HookToolFail     = spec.HookToolFail
	HookElicitation  = spec.HookElicitation
	HookMessageDelta = spec.HookMessageDelta
	HookTurnFailed   = spec.HookTurnFailed
)

const (
	InterruptPermission       = models.InterruptPermission
	InterruptNotification     = models.InterruptNotification
	InterruptElicitation      = models.InterruptElicitation
	InterruptCompaction       = models.InterruptCompaction
	InterruptStopped          = models.InterruptStopped
	InterruptProviderSwitched = models.InterruptProviderSwitched
	InterruptModelChanged     = models.InterruptModelChanged
	InterruptEffortChanged    = models.InterruptEffortChanged
)

const (
	ChoiceToolPermission = models.ChoiceToolPermission
	ChoiceQuestion       = models.ChoiceQuestion
	ChoiceElicitation    = models.ChoiceElicitation
)

const (
	ChoiceOptionAnswer     = models.ChoiceOptionAnswer
	ChoiceOptionAllow      = models.ChoiceOptionAllow
	ChoiceOptionDeny       = models.ChoiceOptionDeny
	ChoiceOptionSuggestion = models.ChoiceOptionSuggestion
)

const (
	TerminalPromptTrust = spec.TerminalPromptTrust
)

const (
	DeliveryRestartTUI = spec.DeliveryRestartTUI
)

const (
	CatalogCompletenessComplete     = string(spec.CatalogCompletenessComplete)
	CatalogCompletenessModelVisible = string(spec.CatalogCompletenessModelVisible)
	CatalogCompletenessPluginOnly   = string(spec.CatalogCompletenessPluginOnly)

	CatalogItemKindSkill = models.CatalogItemKindSkill
)

const (
	TelemetrySourceCallback = models.TelemetrySourceCallback
	TelemetrySourceProbe    = models.TelemetrySourceProbe
)
