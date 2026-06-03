// Package def defines the core types for flow definitions.
// It is a leaf package imported by translator, validator, evaluator, and builtin
// sub-packages to avoid import cycles with the parent flow package.
package def

// FlowTool is an open string type. The constants below are the built-in tools
// registered by Crowbar's MCP server. User-defined flows may declare additional
// tool names — they are valid as long as the MCP server registers them.
// The flow package does not validate tool names against this constant set.
type FlowTool = string

const (
	ToolCrowbarSignal           FlowTool = "crowbar.signal"
	ToolCrowbarCreateItem       FlowTool = "crowbar.create_item"
	ToolCrowbarUpdateItemStatus FlowTool = "crowbar.update_item_status"
	ToolCrowbarGetItems         FlowTool = "crowbar.get_items"
	ToolCrowbarOpenThread       FlowTool = "crowbar.open_thread"
	ToolCrowbarReplyThread      FlowTool = "crowbar.reply_thread"
	ToolCrowbarGetThreads       FlowTool = "crowbar.get_threads"
	ToolCrowbarResolveThread    FlowTool = "crowbar.resolve_thread"
	ToolFSRead                  FlowTool = "fs.read"
	ToolFSWrite                 FlowTool = "fs.write"
	ToolTerminal                FlowTool = "terminal"
)

type UIMode string

const (
	UIModeChat       UIMode = "chat"
	UIModeKanban     UIMode = "kanban"
	UIModeDiff       UIMode = "diff"
	UIModeBackground UIMode = "background"
)

type IntelligenceLevel string

const (
	IntelligenceLow       IntelligenceLevel = "low"
	IntelligenceMedium    IntelligenceLevel = "medium"
	IntelligenceHigh      IntelligenceLevel = "high"
	IntelligenceUltrahigh IntelligenceLevel = "ultrahigh"
)

type AgentDef struct {
	Intelligence IntelligenceLevel
	Tools        []FlowTool
	SystemPrompt string
}

type TransitionDef struct {
	To string // target state name
	On string // event name that triggers this transition
}

// EmitDef declares a side-effect trigger — an improvement agent fired when this
// state is entered or a specific event fires. Reserved for the improvement agent
// subsystem; v0 parsers read but do not act on emit definitions.
type EmitDef struct {
	Agent string // improvement agent name to spawn
	On    string // event name that triggers the spawn (empty = on state entry)
}

type StateDefinition struct {
	Name        string
	Terminal    bool
	UI          UIMode
	Items       bool      // if true, this state creates and manages KanbanItems
	Agent       *AgentDef // nil for human-only states (e.g. human_review)
	Transitions []TransitionDef
	Emits       []EmitDef // reserved; v0 parsers read but do not act on these
}

type FlowDefinition struct {
	Name         string
	Version      string
	Description  string
	ItemStatuses []string
	States       []StateDefinition
}
