package agents

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type (
	// Runner is one live vendor-CLI process and ChatConversation is a conversation it
	// hosted. Both describe the PROCESS, not Crowbar's model of the chat.
	Runner           = models.Runner
	ChatConversation = models.ChatConversation

	// Channel is the wire a delivery arrived on (api connection or hook relay POST).
	// The caller decides it at receive time so ParseHook picks the channel-scoped
	// field map instead of guessing from the payload's shape.
	Channel = spec.Channel

	TemplateCtx    = models.TemplateCtx
	SpawnPlan      = models.SpawnPlan
	InjectStep     = spec.InjectStep
	Display        = models.Display
	Capabilities   = models.Capabilities
	CanonicalEvent = models.CanonicalEvent

	ToolEvent      = models.ToolEvent
	ToolLocation   = models.ToolLocation
	PatchHunk      = models.PatchHunk
	SubagentEvent  = models.SubagentEvent
	SubagentUsage  = models.SubagentUsage
	PlanStepEvent  = models.PlanStepEvent
	ShellEvent     = models.ShellEvent
	InterruptEvent = models.InterruptEvent
	ChoicePrompt   = models.ChoicePrompt
	PromptQuestion = models.PromptQuestion
	ChoiceOption   = models.ChoiceOption

	TerminalPrompt = models.TerminalPrompt

	AnswerCapability = models.AnswerCapability
	AnswerDecision   = models.AnswerDecision
	Selection        = models.Selection

	MessageDelta     = models.MessageDelta
	MessageComplete  = models.MessageComplete
	PlanStep         = models.PlanStep
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
	// APISessionOrigin is how the runner layer learns every session a
	// connection opens or enters — see protocol's own alias.
	APISessionOrigin = protocol.APISessionOrigin
	// APIStdio is serve's stdin and stdout, as StartServe opens them.
	APIStdio = protocol.APIStdio
)

const (
	ChannelAPI   = spec.ChannelAPI
	ChannelHooks = spec.ChannelHooks
)

// Surface is which VIEW an event is worth ingesting on.
const (
	SurfaceChat     = spec.SurfaceChat
	SurfaceTerminal = spec.SurfaceTerminal
)

const (
	HookSessionStart    = spec.HookSessionStart
	HookUserPrompt      = spec.HookUserPrompt
	HookTurnStop        = spec.HookTurnStop
	HookToolPre         = spec.HookToolPre
	HookToolPost        = spec.HookToolPost
	HookSubagentPre     = spec.HookSubagentPre
	HookSubagentPost    = spec.HookSubagentPost
	HookNotification    = spec.HookNotification
	HookWarning         = spec.HookWarning
	HookPermission      = spec.HookPermission
	HookCompactPre      = spec.HookCompactPre
	HookCompactPost     = spec.HookCompactPost
	HookSessionEnd      = spec.HookSessionEnd
	HookTelemetry       = spec.HookTelemetry
	HookToolFail        = spec.HookToolFail
	HookElicitation     = spec.HookElicitation
	HookMessageDelta    = spec.HookMessageDelta
	HookReasoningDelta  = spec.HookReasoningDelta
	HookTurnFailed      = spec.HookTurnFailed
	HookTerminalInput   = spec.HookTerminalInput
	FailureLimit        = spec.FailureLimit
	HookIdle            = spec.HookIdle
	HookToolOutputDelta = spec.HookToolOutputDelta
	HookPlanUpdate      = spec.HookPlanUpdate
	HookPlanStep        = spec.HookPlanStep
	HookSubagentResult  = spec.HookSubagentResult
	HookDiffUpdate      = spec.HookDiffUpdate
	HookMessageComplete = spec.HookMessageComplete
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
	PlanStepPending = models.PlanStepPending
	PlanStepActive  = models.PlanStepActive
	PlanStepDone    = models.PlanStepDone
)

const (
	TelemetrySourceCallback = models.TelemetrySourceCallback
	TelemetrySourceProbe    = models.TelemetrySourceProbe
)
