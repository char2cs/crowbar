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
	Decision       = models.Decision
	MoveKind       = models.MoveKind

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
)

// Interruption kinds.
const (
	InterruptPermission   = models.InterruptPermission
	InterruptNotification = models.InterruptNotification
	InterruptElicitation  = models.InterruptElicitation
	InterruptCompaction   = models.InterruptCompaction
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
