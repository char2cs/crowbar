// Package flow is the flow engine: it parses, validates, and evaluates flow
// YAML definitions. All state transitions in Crowbar go through this layer.
//
// Types are defined in the def sub-package to avoid import cycles with the
// translator, validator, evaluator, and builtin sub-packages; this file
// re-exports them as type aliases so all callers use the flow.* namespace.
package flow

import "github.com/char2cs/crowbar/api/internal/engine/flow/def"

// FlowTool is an open type alias — no closed validation.
type FlowTool = def.FlowTool

const (
	ToolCrowbarSignal           = def.ToolCrowbarSignal
	ToolCrowbarCreateItem       = def.ToolCrowbarCreateItem
	ToolCrowbarUpdateItemStatus = def.ToolCrowbarUpdateItemStatus
	ToolCrowbarGetItems         = def.ToolCrowbarGetItems
	ToolCrowbarOpenThread       = def.ToolCrowbarOpenThread
	ToolCrowbarReplyThread      = def.ToolCrowbarReplyThread
	ToolCrowbarGetThreads       = def.ToolCrowbarGetThreads
	ToolCrowbarResolveThread    = def.ToolCrowbarResolveThread
	ToolFSRead                  = def.ToolFSRead
	ToolFSWrite                 = def.ToolFSWrite
	ToolTerminal                = def.ToolTerminal
)

type UIMode = def.UIMode

const (
	UIModeChat       = def.UIModeChat
	UIModeKanban     = def.UIModeKanban
	UIModeDiff       = def.UIModeDiff
	UIModeBackground = def.UIModeBackground
)

type IntelligenceLevel = def.IntelligenceLevel

const (
	IntelligenceLow       = def.IntelligenceLow
	IntelligenceMedium    = def.IntelligenceMedium
	IntelligenceHigh      = def.IntelligenceHigh
	IntelligenceUltrahigh = def.IntelligenceUltrahigh
)

type AgentDef = def.AgentDef
type TransitionDef = def.TransitionDef
type EmitDef = def.EmitDef
type StateDefinition = def.StateDefinition
type FlowDefinition = def.FlowDefinition

// Loader loads a FlowDefinition from a path or returns the builtin flow.
type Loader interface {
	Load(flowPath string) (FlowDefinition, error)
}
