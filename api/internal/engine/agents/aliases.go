package agents

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// The engine's public value types. They are defined in internal packages and
// re-exported here so this file is the whole of what a caller may depend on: the
// YAML shape behind them can change without touching a single call site.
type (
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

	// TerminalPrompt is a CLI caught blocking on a modal that reaches Crowbar
	// through no hook, and which Crowbar therefore cannot answer.
	TerminalPrompt = models.TerminalPrompt

	// AnswerCapability and AnswerDecision are the write side of a prompt: what a
	// provider can be told, and what a human decided.
	AnswerCapability = models.AnswerCapability
	AnswerDecision   = models.AnswerDecision
	Decision         = models.Decision
	Selection        = models.Selection
	MoveKind         = models.MoveKind

	SlashCatalog     = models.SlashCatalog
	SlashCatalogItem = models.SlashCatalogItem
	ProbeOptions     = models.ProbeOptions

	Telemetry       = models.Telemetry
	ContextUsage    = models.ContextUsage
	RateLimitWindow = models.RateLimitWindow
	SessionCost     = models.SessionCost
	ModelIdentity   = models.ModelIdentity

	// Acquire is the caller-owned concurrency permit every provider command is
	// taken under, so all concurrent probes share one daemon-wide budget.
	Acquire = exec.Acquire
)

// Move outcomes.
const (
	MoveNoop    = models.MoveNoop
	MoveBind    = models.MoveBind
	MoveToNew   = models.MoveToNew
	MoveToKnown = models.MoveToKnown
)

// Canonical hook kinds. A caller switches on these; it never learns a provider's
// own event names.
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
)

// Interruption kinds.
const (
	InterruptPermission   = models.InterruptPermission
	InterruptNotification = models.InterruptNotification
	InterruptElicitation  = models.InterruptElicitation
	InterruptCompaction   = models.InterruptCompaction
)

// Choice kinds — what kind of answer an agent is blocked waiting for.
const (
	ChoiceToolPermission = models.ChoiceToolPermission
	ChoiceQuestion       = models.ChoiceQuestion
	ChoiceElicitation    = models.ChoiceElicitation
)

// Choice option kinds. A client renders by kind, because the labels of the two
// synthetic ones are Crowbar's words rather than a provider's.
const (
	ChoiceOptionAnswer     = models.ChoiceOptionAnswer
	ChoiceOptionAllow      = models.ChoiceOptionAllow
	ChoiceOptionDeny       = models.ChoiceOptionDeny
	ChoiceOptionSuggestion = models.ChoiceOptionSuggestion
)

// Terminal-prompt kinds — Crowbar's own name for a blocking modal it recognises
// specifically. A match carrying none is reported generically; see TerminalPrompt.
const (
	TerminalPromptTrust = spec.TerminalPromptTrust
)

// Prompt-delivery strategies.
const (
	DeliveryRestartTUI = spec.DeliveryRestartTUI
	DeliveryRewakeHook = spec.DeliveryRewakeHook
)

// Catalogue completeness labels. They say exactly which provider-owned surface a
// probe can account for, so a partial inventory is never presented as complete.
const (
	CatalogCompletenessComplete     = string(spec.CatalogCompletenessComplete)
	CatalogCompletenessModelVisible = string(spec.CatalogCompletenessModelVisible)
	CatalogCompletenessPluginOnly   = string(spec.CatalogCompletenessPluginOnly)

	CatalogItemKindSkill = models.CatalogItemKindSkill
)

// Telemetry ingress sources.
const (
	TelemetrySourceCallback = models.TelemetrySourceCallback
	TelemetrySourceProbe    = models.TelemetrySourceProbe
)
