# Agent 04 — Flow Engine

**Working directory:** `/Users/char2cs/Projects/Rabbyte/crowbar/api`
**Module:** `github.com/char2cs/crowbar/api`

## Context

The flow engine parses, validates, and evaluates flow YAML definitions. It is the core of Crowbar's state machine. All state transitions in the system go through this layer.

## Files to read before starting

- `docs/superpowers/specs/2026-05-19-flow-engine-design.md` — complete; this is your primary spec
- `api/ARCHITECTURE.md` §"engine/flow/" — package layout
- Quiver reference for embedded file patterns: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/core/metadata/`

## What already exists

Agents 01–03 are complete. `internal/domain/` has all entity types.

## Package layout

```
internal/engine/flow/
├── flow.go
├── loader.go
├── translator/
│   ├── translator.go
│   ├── raw.go
│   ├── mapper.go
│   └── schema/
│       └── flow.json
├── validator/
│   └── validator.go
├── evaluator/
│   └── evaluator.go
└── builtin/
    └── feature_development.go
```

## Tasks

### `flow.go` — exported types

```go
package flow

// FlowTool is an open type alias — no closed validation.
type FlowTool = string

type UIMode string
const (
    UIModeDiff       UIMode = "diff"
    UIModeKanban     UIMode = "kanban"
    UIModeChat       UIMode = "chat"
    UIModeBackground UIMode = "background"
)

type IntelligenceLevel string
const (
    IntelligenceLow    IntelligenceLevel = "low"
    IntelligenceMedium IntelligenceLevel = "medium"
    IntelligenceHigh   IntelligenceLevel = "high"
)

type AgentDef struct {
    Intelligence IntelligenceLevel
    SystemPrompt string
    Tools        []FlowTool
}

type TransitionDef struct {
    On   string // event name
    To   string // target state name
}

type EmitDef struct {
    Agent string
    On    string
}

type StateDefinition struct {
    Name        string
    Agent       *AgentDef     // nil = human-only state
    UI          UIMode
    Items       bool          // crowbar_create_item allowed
    Terminal    bool          // terminal state; flow ends here
    Transitions []TransitionDef
    Emits       []EmitDef     // reserved for v1; parsed but not acted on in v0
}

type FlowDefinition struct {
    Name         string
    Description  string
    ItemStatuses []string
    States       []StateDefinition
}

type Loader interface {
    Load(flowPath string) (FlowDefinition, error)
}
```

### `loader.go`

`Load(flowPath string)`:
- If `flowPath == ""` → return `builtin.FeatureDevelopmentFlow` (the package-level var)
- Otherwise: read file from disk, call `translator.Parse(bytes)`, then `validator.Validate(def)`
- Return combined errors from validator as a single error

### `translator/raw.go`

Flat YAML structs matching the YAML shape exactly:

```go
type rawAgent struct {
    Intelligence string   `yaml:"intelligence"`
    SystemPrompt string   `yaml:"system_prompt"`
    Tools        []string `yaml:"tools"`
}

type rawTransition struct {
    On string `yaml:"on"`
    To string `yaml:"to"`
}

type rawEmit struct {
    Agent string `yaml:"agent"`
    On    string `yaml:"on"`
}

type rawState struct {
    Name        string          `yaml:"name"`
    Agent       *rawAgent       `yaml:"agent"`
    UI          string          `yaml:"ui"`
    Items       bool            `yaml:"items"`
    Terminal    bool            `yaml:"terminal"`
    Transitions []rawTransition `yaml:"transitions"`
    Emits       []rawEmit       `yaml:"emits"`
}

type rawFlow struct {
    Name         string     `yaml:"name"`
    Description  string     `yaml:"description"`
    ItemStatuses []string   `yaml:"item_statuses"`
    States       []rawState `yaml:"states"`
}
```

### `translator/translator.go`

```go
func Parse(data []byte) (flow.FlowDefinition, error)
```

1. Validate `data` against the embedded `schema/flow.json` JSON schema (structural validation)
2. Unmarshal YAML into `rawFlow`
3. Call `mapper.Map(raw)` → `FlowDefinition`
4. Return

Use `embed` to include `schema/flow.json`. Use a JSON schema validator (e.g. `github.com/santhosh-tekuri/jsonschema/v5` or similar available in the module cache — check `go.sum` in Quiver for what's available). If no JSON schema library is available, do structural validation manually (required fields present).

### `translator/mapper.go`

Maps `rawFlow` → `FlowDefinition`. Straightforward field-by-field mapping. `FlowTool = string` so no casting needed. `IntelligenceLevel` and `UIMode` cast from string.

### `translator/schema/flow.json`

Minimal JSON schema validating required top-level fields:
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["name", "states"],
  "properties": {
    "name": {"type": "string"},
    "states": {"type": "array", "minItems": 1}
  }
}
```

### `validator/validator.go`

```go
type ValidationError struct {
    Rule    string
    Message string
}

func Validate(def flow.FlowDefinition) []ValidationError
```

Five concurrent rules (run with `sync.WaitGroup`, collect errors into a mutex-guarded slice):

| Rule | Check |
|------|-------|
| `StateNamesUnique` | no duplicate state names |
| `TransitionTargetsExist` | every `transition.To` names an existing state |
| `AtLeastOneTerminal` | at least one state has `Terminal: true` |
| `UIModeValid` | every state UI value is one of: `diff`, `kanban`, `chat`, `background` |
| `IntelligenceLevelValid` | every agent intelligence is one of: `low`, `medium`, `high` |

Note: `AgentToolsKnown` is **not** a rule — FlowTool is an open type.

### `evaluator/evaluator.go`

```go
func Evaluate(def flow.FlowDefinition, currentState string, event string) (nextState string, ok bool)
```

Finds the `StateDefinition` matching `currentState`, then iterates `Transitions` looking for `On == event`. Returns `(transition.To, true)` on match, `("", false)` if no match.

### `builtin/feature_development.go`

Package-level `var FeatureDevelopmentFlow flow.FlowDefinition` with the builtin feature development flow. This defines 7 states: `brainstorming`, `spec`, `implementation`, `implementation_complete`, `ai_review`, `human_review`, `complete`.

Implement all 7 states with appropriate `AgentDef`, `UI`, `Items`, `Terminal`, and `Transitions` fields matching the domain spec. `complete` is the only terminal state. At minimum:

```
brainstorming → (user_approved) → spec
spec          → (user_approved) → implementation
implementation → (implementation_complete) → implementation_complete
implementation_complete → (ai_review_ready) → ai_review
ai_review     → (review_passed) → human_review
              → (review_failed) → implementation
human_review  → (approved) → complete
              → (changes_requested) → implementation
complete      — terminal
```

## Verification

```bash
cd /Users/char2cs/Projects/Rabbyte/crowbar/api && go build ./internal/engine/flow/...
go vet ./internal/engine/flow/...
```
