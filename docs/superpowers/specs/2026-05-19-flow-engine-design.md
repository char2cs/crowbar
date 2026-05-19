# Crowbar — Flow Engine Spec

**Date:** 2026-05-19
**Status:** Approved
**Sprint:** v0 — Initial Backend

---

## Overview

The Flow Engine parses Flow YAML files into typed Go structs, validates them in two phases, evaluates state machine transitions, and provides the hardcoded feature-development flow for v0. It is a pure logic layer — no database, no HTTP, no goroutines. Every other subsystem that needs to know "what state comes next?" goes through the evaluator.

**Out of scope this sprint:** ImprovementAgent definitions, YAML flow authoring UI, custom flow hot-reload, `emit` transitions (concurrent improvement agent triggers).

---

## 1. Package Layout

```
internal/engine/flow/
├── flow.go              // FlowDefinition + all enum types
├── loader.go            // Load(flowPath) → FlowDefinition
├── translator/
│   ├── translator.go    // Parse([]byte) → FlowDefinition (structural validation + mapping)
│   ├── raw.go           // raw YAML structs (one-to-one with YAML shape)
│   ├── mapper.go        // rawFlow → FlowDefinition
│   └── schema/
│       └── flow.json    // embedded JSON schema for structural validation
├── validator/
│   └── validator.go     // Validate(FlowDefinition) → []ValidationError (semantic phase)
├── evaluator/
│   └── evaluator.go     // Evaluate(flow, currentState, event) → (string, bool)
└── builtin/
    └── feature_development.go  // package-level FlowDefinition var
```

---

## 2. Domain Structs (`flow.go`)

Pure Go — no database tags, no serialization tags. These are the in-memory representation of a loaded flow.

### Enums

```go
type IntelligenceLevel string

const (
    IntelligenceLow       IntelligenceLevel = "low"
    IntelligenceMedium    IntelligenceLevel = "medium"
    IntelligenceHigh      IntelligenceLevel = "high"
    IntelligenceUltrahigh IntelligenceLevel = "ultrahigh"
)

type UIMode string

const (
    UIModeChat       UIMode = "chat"
    UIModeKanban     UIMode = "kanban"
    UIModeDiff       UIMode = "diff"
    UIModeBackground UIMode = "background"
)

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
```

### FlowDefinition

```go
type FlowDefinition struct {
    Name         string
    Version      string
    Description  string
    ItemStatuses []string
    States       []StateDefinition
}

type StateDefinition struct {
    Name        string
    Terminal    bool
    UI          UIMode
    Items       bool       // if true, this state creates and manages KanbanItems
    Agent       *AgentDef  // nil for human-only states (e.g. human_review)
    Transitions []TransitionDef
    Emits       []EmitDef  // reserved; v0 parsers read but do not act on these
}

type AgentDef struct {
    Intelligence IntelligenceLevel
    Tools        []FlowTool
    SystemPrompt string
}

type TransitionDef struct {
    To string  // target state name
    On string  // event name that triggers this transition
}

// EmitDef declares a side-effect trigger — an improvement agent fired when this
// state is entered or a specific event fires. Reserved for the improvement agent
// subsystem; v0 parsers read but do not act on emit definitions.
type EmitDef struct {
    Agent string // improvement agent name to spawn
    On    string // event name that triggers the spawn (empty = on state entry)
}
```

---

## 3. YAML Parser + Translator

### Entry point

```go
// translator.Parse reads raw YAML bytes, validates the structure against the
// embedded JSON schema, and maps to a FlowDefinition.
func Parse(data []byte) (flow.FlowDefinition, error)
```

### Parse flow

1. Unmarshal YAML into `flowRaw` struct
2. Convert to JSON, validate against embedded `schema/flow.json` (field types, required fields, enum membership for `ui` and `intelligence`)
3. `mapper.toFlowDefinition(raw)` → typed `FlowDefinition`

### Raw structs (`raw.go`)

Intentionally flat — one-to-one with YAML shape. No business logic here.

```go
type flowRaw struct {
    Name         string     `yaml:"name"`
    Version      string     `yaml:"version"`
    Description  string     `yaml:"description"`
    ItemStatuses []string   `yaml:"item_statuses"`
    States       []stateRaw `yaml:"states"`
}

type stateRaw struct {
    Name        string          `yaml:"name"`
    Terminal    bool            `yaml:"terminal"`
    UI          string          `yaml:"ui"`
    Items       bool            `yaml:"items"`
    Agent       *agentRaw       `yaml:"agent"`
    Transitions []transitionRaw `yaml:"transitions"`
    Emits       []emitRaw       `yaml:"emits"`
}

type agentRaw struct {
    Intelligence string   `yaml:"intelligence"`
    Tools        []string `yaml:"tools"`
    SystemPrompt string   `yaml:"system_prompt"`
}

type transitionRaw struct {
    To string `yaml:"to"`
    On string `yaml:"on"`
}

type emitRaw struct {
    Agent string `yaml:"agent"`
    On    string `yaml:"on"`
}
```

### Mapper (`mapper.go`)

Converts `flowRaw` → `flow.FlowDefinition`. Responsible for casting string fields to their enum types (`string` → `IntelligenceLevel`, `string` → `UIMode`). `FlowTool` is an open string type — no casting required. `emitRaw` entries are mapped directly to `EmitDef` values in `StateDefinition.Emits`; agent name validation is deferred to a future sprint when the improvement agent subsystem is defined.

---

## 4. Two-Phase Validator

### Phase 1 — Structural (inside `translator.Parse`)

JSON schema validation against `schema/flow.json`. Catches:
- Missing required fields (`name`, `version`, `states`)
- Invalid enum strings for `ui` and `intelligence`
- Wrong field types

### Phase 2 — Semantic (`validator.Validate`)

```go
func Validate(f flow.FlowDefinition) []ValidationError

type ValidationError struct {
    Rule    string
    Message string
}
```

All rules run concurrently via `sync.WaitGroup`. Errors are collected under a mutex and returned together — same pattern as Quiver's ruleset. Caller wraps the slice into a single error for the HTTP layer.

**Rules:**

| Rule | Check |
|------|-------|
| `UniqueStateNames` | No two states share a name |
| `AtLeastOneTerminal` | At least one terminal state exists |
| `TransitionTargetsExist` | Every `TransitionDef.To` references a defined state name |
| `NoTransitionToTerminal` | No state has a transition *into* a terminal state — terminal states are sinks |
| `NonTerminalHasTransitions` | Every non-terminal state has at least one transition defined — a state with no transitions and no terminal flag is a dead end |

---

## 5. State Machine Evaluator

Pure function. No side effects, no I/O, no database.

```go
// Evaluate returns the target state name for the given event fired in currentState.
// Returns ("", false) if no matching transition exists for this event.
func Evaluate(f flow.FlowDefinition, currentState string, event string) (string, bool)
```

**Implementation:** find the `StateDefinition` where `Name == currentState`, iterate its `Transitions`, return `TransitionDef.To` for the first entry where `TransitionDef.On == event`. Pure table lookup — no switch statements, no if chains.

**Callers:**
- **Agent Runtime** — when `crowbar_signal(event)` arrives via MCP, passes the loaded flow + task's current state to get the next state, then issues `AdvanceState` on the Task aggregate
- **Task usecase** — when the user triggers a force transition via `POST /api/v0/tasks/:id/transition`, validates the requested state exists in the flow before advancing

---

## 6. Flow Loader

```go
// Load returns the FlowDefinition for the given path.
// An empty path or "builtin:feature-development" returns the hardcoded builtin flow.
// Any other path reads the file from disk, parses, and validates it.
func Load(flowPath string) (flow.FlowDefinition, error)
```

**Builtin shortcut:** `flowPath == "" || flowPath == "builtin:feature-development"` → returns `builtin.FeatureDevelopment` directly. No disk I/O, no parsing, no validation (it is always valid by construction).

**Disk path:** reads file → `translator.Parse()` → `validator.Validate()` → returns `FlowDefinition` or aggregated error. Validation errors are surfaced to the caller (HTTP layer returns them to the frontend at Task creation time).

---

## 7. Hardcoded Feature Development Flow (`builtin/feature_development.go`)

A single package-level `var` — the literal Go translation of the Reference Flow from the domain spec. All states, transitions, tools, and system prompts defined as struct literals.

```go
var FeatureDevelopment = flow.FlowDefinition{
    Name:         "feature-development",
    Version:      "1.0",
    Description:  "Full feature development — brainstorm to reviewed implementation",
    ItemStatuses: []string{"todo", "implementing", "done"},
    States: []flow.StateDefinition{
        {
            Name: "brainstorming",
            UI:   flow.UIModeChat,
            Agent: &flow.AgentDef{
                Intelligence: flow.IntelligenceMedium,
                Tools:        []flow.FlowTool{flow.ToolCrowbarSignal, flow.ToolFSRead, flow.ToolTerminal},
                SystemPrompt: `...`, // verbatim from domain spec §Reference Flow // verbatim from domain spec §Reference Flow
            },
            Transitions: []flow.TransitionDef{
                {To: "spec",      On: "user_approved"},
                {To: "debugging", On: "bug_identified"},
            },
        },
        {
            Name: "spec",
            UI:   flow.UIModeChat,
            Agent: &flow.AgentDef{
                Intelligence: flow.IntelligenceMedium,
                Tools:        []flow.FlowTool{flow.ToolCrowbarSignal, flow.ToolFSRead},
                SystemPrompt: `...`, // verbatim from domain spec §Reference Flow
            },
            Transitions: []flow.TransitionDef{
                {To: "implementation",  On: "user_approved"},
                {To: "brainstorming",   On: "revision_requested"},
            },
        },
        {
            Name:  "implementation",
            UI:    flow.UIModeKanban,
            Items: true,
            Agent: &flow.AgentDef{
                Intelligence: flow.IntelligenceHigh,
                Tools: []flow.FlowTool{
                    flow.ToolCrowbarSignal, flow.ToolCrowbarCreateItem,
                    flow.ToolCrowbarUpdateItemStatus, flow.ToolCrowbarGetItems,
                    flow.ToolCrowbarGetThreads, flow.ToolCrowbarReplyThread,
                    flow.ToolFSRead, flow.ToolFSWrite, flow.ToolTerminal,
                },
                SystemPrompt: `...`, // verbatim from domain spec §Reference Flow
            },
            Transitions: []flow.TransitionDef{
                {To: "ai_review", On: "implementation_complete"},
                {To: "spec",      On: "scope_changed"},
            },
        },
        {
            Name: "ai_review",
            UI:   flow.UIModeDiff,
            Agent: &flow.AgentDef{
                Intelligence: flow.IntelligenceHigh,
                Tools: []flow.FlowTool{
                    flow.ToolCrowbarSignal, flow.ToolCrowbarOpenThread,
                    flow.ToolCrowbarGetThreads, flow.ToolCrowbarResolveThread,
                    flow.ToolCrowbarReplyThread, flow.ToolFSRead,
                },
                SystemPrompt: `...`, // verbatim from domain spec §Reference Flow
            },
            Transitions: []flow.TransitionDef{
                {To: "implementation", On: "review_failed"},
                {To: "human_review",   On: "review_passed"},
            },
        },
        {
            Name:  "human_review",
            UI:    flow.UIModeDiff,
            Agent: nil, // human-only state
            Transitions: []flow.TransitionDef{
                {To: "implementation", On: "changes_requested"},
                {To: "complete",       On: "approved"},
            },
        },
        {
            Name: "debugging",
            UI:   flow.UIModeChat,
            Agent: &flow.AgentDef{
                Intelligence: flow.IntelligenceHigh,
                Tools:        []flow.FlowTool{flow.ToolCrowbarSignal, flow.ToolFSRead, flow.ToolFSWrite, flow.ToolTerminal},
                SystemPrompt: `...`, // verbatim from domain spec §Reference Flow
            },
            Transitions: []flow.TransitionDef{
                {To: "implementation", On: "bug_resolved"},
                {To: "brainstorming",  On: "scope_expanded"},
            },
        },
        {
            Name:     "complete",
            Terminal: true,
        },
    },
}
```

**Migration guarantee:** when the YAML engine is added, `translator.Parse()` on the feature-development YAML will produce a struct identical to this var. The migration is a parser addition, not a rewrite.
