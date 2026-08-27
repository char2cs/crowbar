# Crowbar Permission Levels Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace "Crowbar always holds every tool-call permission for a human" with a generic, provider-blind `guarded`/`trusted`/`full-auto` policy — defaulting to `full-auto`, configurable in Settings → Agents — and fix a structural stall where Crowbar's own MCP tool calls have no one present to answer them.

**Architecture:** A `RiskTier` (`read-only`/`standard`/`sensitive`/`internal`) is computed per tool call from a classification table each provider's descriptor declares in YAML. A `PermissionLevel` (`guarded`/`trusted`/`full-auto`), scoped per chat and seeded from a global backend-persisted default, is compared against the tier by one pure policy function. When the level clears the tier, the engine auto-resolves the prompt using the exact same render-and-resolve sequence a human's click uses today, so the transcript still shows what happened.

**Tech Stack:** Go (backend engine + usecases + GORM store), YAML (provider descriptors), React + Zustand (frontend settings + composer UI), Vitest (frontend tests), Go's `testing` + `testify` (backend tests).

**Spec:** `docs/superpowers/specs/2026-08-26-crowbar-permission-levels-design.md`

## Global Constraints

- No provider name or provider tool-vocabulary literal (`"claude"`, `"codex"`, `"Bash"`, `"Read"`, etc.) may appear in any `.go` file added or modified by this plan — that vocabulary lives only in the YAML descriptors (`claude.yaml`, `codex.yaml`) and in Go test fixtures that construct a descriptor in-memory. This is a repository-wide law (see `feedback_no_provider_specific_code` in project memory), not new to this feature.
- Test files go in `web/src/__tests__/` mirroring `web/src/`, using `@/` imports — per this repo's `CLAUDE.md`. Go tests are named `<source>_test.go` beside the file they test, following the two existing conventions already used in the touched packages: black-box `package x_test` with `testify/assert`+`require` (`translate/inbound`, `answerdesk`) and white-box `package spec_test`/`package x` with plain stdlib `testing` (`internal/spec`) — match whichever convention the file you're editing already uses; do not convert one style to the other.
- Component files are kebab-case (`permission-level-select.tsx`, not `PermissionLevelSelect.tsx`); the exported component name stays PascalCase.
- Frontend stores: narrow selectors only (`useXStore((s) => s.field)`), `.getState()` only inside handlers/effects, stores never import from `components/`.
- **You are working in a git worktree shared with a concurrent session actively editing files under `web/src/features/agent/`** (confirmed at plan-writing time: `agent-chat-view.tsx`, `composer-choice.tsx`, `provider-bar.tsx`, `composer.css`, `transcript.css`, `working-line.tsx` were all mid-edit). Before Task 8 (the only task touching that directory), re-run `git status --porcelain` and treat any file it names as a moving target — read it fresh immediately before editing, keep your diff to the smallest possible isolated addition, and if the exact lines you need to touch are already modified, stop and flag it rather than force a merge. Every other task (1-7) touches files outside `web/src/features/agent/` and carries no such risk.
- Every task ends green on its own package's test suite (`go test ./<package>/...` or the relevant `vitest` file) before its commit — no task hands off a red suite to the next.
- Follow the `go-style` skill's conventions for every `.go` file this plan touches or creates.

---

## Task 1: `RiskTier` type, `ChoicePrompt.Risk`, descriptor `risk:` schema, and classification in `permissionChoice`

**Files:**
- Create: `api/internal/engine/agents/internal/models/risk.go`
- Modify: `api/internal/engine/agents/internal/models/event.go` (the `ChoicePrompt` struct, currently at `event.go:100-120`)
- Modify: `api/internal/engine/agents/internal/spec/v3.go` (the `EventSpec` struct, currently at `v3.go:53-103`)
- Modify: `api/internal/engine/agents/internal/spec/unified.go` (new `EventRisk` accessor, alongside `EventFields` at `unified.go:10-16`)
- Modify: `api/internal/engine/agents/aliases.go` (re-export block, alongside the `ChoiceToolPermission` block at `aliases.go:94-98`)
- Modify: `api/internal/engine/agents/internal/protocol/internal/translate/inbound/choice.go` (`permissionChoice`, `choice.go:34-58`)
- Modify: `api/internal/engine/agents/internal/protocol/internal/translate/inbound/hooks.go` (`Parse`/`build`, `hooks.go:32-58` and `83-124`)
- Test: `api/internal/engine/agents/internal/protocol/internal/translate/inbound/choice_test.go` (append)
- Test: `api/internal/engine/agents/internal/spec/unified_test.go` (append)

**Interfaces:**
- Produces: `models.RiskTier` (type `string`) and constants `models.RiskReadOnly`, `models.RiskStandard`, `models.RiskSensitive`, `models.RiskInternal`, re-exported as `engineagents.RiskTier` / `engineagents.RiskReadOnly` / `engineagents.RiskStandard` / `engineagents.RiskSensitive` / `engineagents.RiskInternal`. `models.ChoicePrompt` gains a `Risk RiskTier` field (and thus `engineagents.ChoicePrompt.Risk` / `ev.Choice.Risk` wherever a `CanonicalEvent` is read). `spec.EventSpec` gains `Risk map[string][]string` (yaml key `risk`). `spec.Descriptor.EventRisk(canonical string) (map[string][]string, bool)`.
- Consumes: nothing new — this task only adds to existing types.

- [ ] **Step 1: Write the failing tests**

In `api/internal/engine/agents/internal/protocol/internal/translate/inbound/choice_test.go`, append:

```go
func TestParse_PermissionClassifiesRiskFromTheDescriptorsTable(t *testing.T) {
	risk := map[string][]string{
		"read-only": {"Read", "Grep"},
		"standard":  {"Bash", "Edit"},
		"internal":  {"mcp__crowbar__*"},
	}
	d := descriptorWithRisk(permissionMap(), risk)

	cases := []struct {
		name     string
		toolName string
		want     models.RiskTier
	}{
		{"read-only tool", "Read", models.RiskReadOnly},
		{"standard tool", "Bash", models.RiskStandard},
		{"crowbar's own mcp tool matches the wildcard", "mcp__crowbar__post_review_comment", models.RiskInternal},
		{"unclassified tool defaults to sensitive", "WebFetch", models.RiskSensitive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(`{"session_id":"s1","prompt_id":"p1","tool_name":"` + tc.toolName + `"}`)
			ev, err := inbound.Parse(d, spec.HookPermission, payload)
			require.NoError(t, err)
			require.NotNil(t, ev.Choice)
			assert.Equal(t, tc.want, ev.Choice.Risk)
		})
	}
}

func TestParse_PermissionWithNoRiskTableDefaultsEverythingToSensitive(t *testing.T) {
	d := descriptorWithRisk(permissionMap(), nil)

	ev, err := inbound.Parse(d, spec.HookPermission, []byte(`{"prompt_id":"p1","tool_name":"Bash"}`))

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	assert.Equal(t, models.RiskSensitive, ev.Choice.Risk,
		"a descriptor that declares no risk: table must never grant an implicit auto-approve")
}

func TestParse_QuestionChoiceCarriesNoRiskTier(t *testing.T) {
	d := descriptorWithRisk(permissionMap(), map[string][]string{"standard": {"AskUserQuestion"}})

	ev, err := inbound.Parse(d, spec.HookPermission, []byte(threeQuestionPayload))

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	assert.Equal(t, models.ChoiceQuestion, ev.Choice.Kind)
	assert.Empty(t, ev.Choice.Risk,
		"a question prompt has no allow/deny to auto-resolve, so it must never carry a risk tier")
}

func descriptorWithRisk(fields map[string]string, risk map[string][]string) *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe", Events: map[string]spec.EventSpec{
		spec.HookPermission: {In: spec.HookPermission, Map: fields, Risk: risk},
	}}
	d.Runtime.Hooks.Format = "json"
	return d
}
```

In `api/internal/engine/agents/internal/spec/unified_test.go`, add a `Risk` entry to the shared `v3()` fixture's `"permission"` event and a new test:

```go
			"permission": {
				Ask: "PermissionRequest", TimeoutSeconds: 270, AnswersInto: "answers",
				Reply: map[string]string{"allow": "{}"},
				Risk:  map[string][]string{"standard": {"Bash"}},
			},
```

```go
func TestEventRisk_ReadsTheEventTable(t *testing.T) {
	d := v3()
	r, ok := d.EventRisk("permission")
	if !ok || len(r["standard"]) != 1 || r["standard"][0] != "Bash" {
		t.Errorf("EventRisk = (%v,%v)", r, ok)
	}
	if _, ok := d.EventRisk("never_declared"); ok {
		t.Error("an undeclared event must report false, same as EventFields")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/engine/agents/... 2>&1 | head -50`
Expected: compile failures — `models.RiskTier`, `models.RiskReadOnly` etc. undefined; `spec.EventSpec` has no field `Risk`; `Descriptor.EventRisk` undefined.

- [ ] **Step 3: Implement**

Create `api/internal/engine/agents/internal/models/risk.go`:

```go
package models

// RiskTier classifies how much latitude a tool call needs, independent of
// which provider raised it. Each provider's descriptor maps its own tool
// vocabulary into this fixed, provider-blind scale (see EventSpec.Risk); Go
// never inspects a raw tool name to decide it.
type RiskTier string

const (
	// RiskReadOnly inspects, changes nothing.
	RiskReadOnly RiskTier = "read-only"
	// RiskStandard is an ordinary edit or command inside the workspace.
	RiskStandard RiskTier = "standard"
	// RiskSensitive is destructive, external-facing, or — critically — simply
	// unclassified. It is the safe default: a tool the descriptor's risk table
	// doesn't name gets the most conservative tier, never the most permissive.
	RiskSensitive RiskTier = "sensitive"
	// RiskInternal marks a call to Crowbar's own injected tool surface. It is
	// not part of the guarded/trusted/full-auto scale at all — see
	// permission.AutoApprove — because no human is present in a Crowbar-driven
	// pane to answer for it; holding it would only ever stall it.
	RiskInternal RiskTier = "internal"
)
```

In `api/internal/engine/agents/internal/models/event.go`, add `Risk` to `ChoicePrompt` (after `ToolName`, before `Title`, so a permission prompt's provider-facing fields and Crowbar's own classification field sit next to each other):

```go
type ChoicePrompt struct {
	Kind     string
	PromptID string
	ToolName string
	Risk     RiskTier
	Title    string
	Question string
	Mode     string
	Multi    bool

	Options   []ChoiceOption
	Questions []PromptQuestion
	Schema    []byte
}
```

In `api/internal/engine/agents/internal/spec/v3.go`, add `Risk` to `EventSpec` (after `Reply`):

```go
	Reply map[string]string `yaml:"reply"`
	// Risk classifies this event's tool calls into engineagents.RiskTier
	// buckets, keyed by tier name ("read-only", "standard", "sensitive",
	// "internal") to a list of tool-name patterns (a trailing "*" matches a
	// prefix; anything else must match exactly). Only meaningful for the
	// permission event today. A tool matching no pattern in any bucket, or a
	// descriptor declaring no risk: block at all, classifies as "sensitive" —
	// see models.RiskSensitive.
	Risk map[string][]string `yaml:"risk"`
```

In `api/internal/engine/agents/internal/spec/unified.go`, add the accessor after `EventFields`:

```go
// EventRisk returns the risk-tier classification table for an event, and
// whether the provider declares the event at all — same key-presence
// contract as EventFields. A declared event with no risk: block returns a
// nil map, which classifyRisk (translate/inbound/choice.go) treats as
// "everything is sensitive", never as "everything is allowed".
func (d *Descriptor) EventRisk(canonical string) (map[string][]string, bool) {
	e, ok := d.Events[canonical]
	if !ok {
		return nil, false
	}
	return e.Risk, true
}
```

In `api/internal/engine/agents/aliases.go`, add `RiskTier` to the `type (...)` block (next to `ChoicePrompt` at line 27):

```go
	ChoicePrompt   = models.ChoicePrompt
	RiskTier       = models.RiskTier
```

and a new const block after the `ChoiceOptionAllow` block (after line 105):

```go
const (
	RiskReadOnly  = models.RiskReadOnly
	RiskStandard  = models.RiskStandard
	RiskSensitive = models.RiskSensitive
	RiskInternal  = models.RiskInternal
)
```

In `api/internal/engine/agents/internal/protocol/internal/translate/inbound/choice.go`, add `"strings"` to imports, thread a `risk` parameter through `permissionChoice`, and add the classifier:

```go
func permissionChoice(
	fields map[string]string,
	risk map[string][]string,
	decoded map[string]any,
) *models.ChoicePrompt {
	if !declaresChoice(fields) {
		return nil
	}
	promptID := firstNonEmpty(decoded, fields["prompt_id"])
	toolName := firstNonEmpty(decoded, fields["tool_name"])

	if questions := mapping.Objects(decoded, fields["questions"]); len(questions) > 0 {
		return questionChoice(fields, questions, promptID, toolName)
	}

	prompt := &models.ChoicePrompt{
		Kind:     models.ChoiceToolPermission,
		PromptID: promptID,
		ToolName: toolName,
		Risk:     classifyRisk(risk, toolName),
		Title:    toolName,

		Options: []models.ChoiceOption{
			{ID: models.ChoiceOptionAllow, Kind: models.ChoiceOptionAllow, Label: "Allow"},
			{ID: models.ChoiceOptionDeny, Kind: models.ChoiceOptionDeny, Label: "Deny"},
		},
	}
	prompt.Options = append(prompt.Options, suggestionOptions(fields, decoded)...)
	return prompt
}

// classifyRisk maps toolName to the tier the descriptor's own risk: table
// declares for it. Unmatched — including every name when the descriptor
// declares no risk: block — is models.RiskSensitive: the table is a safe
// allowlist, never a denylist.
func classifyRisk(risk map[string][]string, toolName string) models.RiskTier {
	switch {
	case matchesAny(risk[string(models.RiskInternal)], toolName):
		return models.RiskInternal
	case matchesAny(risk[string(models.RiskReadOnly)], toolName):
		return models.RiskReadOnly
	case matchesAny(risk[string(models.RiskStandard)], toolName):
		return models.RiskStandard
	default:
		return models.RiskSensitive
	}
}

func matchesAny(patterns []string, toolName string) bool {
	for _, pattern := range patterns {
		if matchesPattern(pattern, toolName) {
			return true
		}
	}
	return false
}

// matchesPattern supports one wildcard shape — a trailing "*" — which is all
// Crowbar's own MCP tool names need (they share one "mcp__crowbar__" prefix).
// Anything else must match the provider's tool name exactly.
func matchesPattern(pattern, toolName string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(toolName, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == toolName
}
```

`questionChoice` is untouched — it never sets `Risk`, which is exactly `TestParse_QuestionChoiceCarriesNoRiskTier`'s point: a question prompt's `Risk` stays the zero value, and Task 4's engine policy must gate on `Kind == ChoiceToolPermission` before consulting it (never on `Risk` being non-empty).

In `api/internal/engine/agents/internal/protocol/internal/translate/inbound/hooks.go`, thread `risk` through `Parse` and `build`:

```go
	fields, declared := d.EventFields(canonical)
	if !declared {
		return models.CanonicalEvent{}, fmt.Errorf("%w: %q on %q", ErrUndeclaredEvent, canonical, d.ID)
	}
	risk, _ := d.EventRisk(canonical)
	return build(canonical, fields, risk, decoded), nil
}
```

```go
func build(canonical string, fields map[string]string, risk map[string][]string, decoded map[string]any) models.CanonicalEvent {
	...
	case spec.HookPermission:
		ev.Interrupt = &models.InterruptEvent{Kind: models.InterruptPermission, Detail: ev.Message}
		ev.Choice = permissionChoice(fields, risk, decoded)
	case spec.HookElicitation:
		ev.Interrupt = &models.InterruptEvent{Kind: models.InterruptElicitation, Detail: ev.Message}
		ev.Choice = elicitationChoice(fields, decoded, ev.Message)
```

(`elicitationChoice`'s call is unchanged — elicitation stays out of scope for risk, per the spec.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/engine/agents/... -run 'Risk|EventRisk' -v`
Expected: PASS for every new test, plus the full `./internal/engine/agents/...` suite still green (`go test ./internal/engine/agents/...`).

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/agents/internal/models/risk.go \
        api/internal/engine/agents/internal/models/event.go \
        api/internal/engine/agents/internal/spec/v3.go \
        api/internal/engine/agents/internal/spec/unified.go \
        api/internal/engine/agents/internal/spec/unified_test.go \
        api/internal/engine/agents/aliases.go \
        api/internal/engine/agents/internal/protocol/internal/translate/inbound/choice.go \
        api/internal/engine/agents/internal/protocol/internal/translate/inbound/hooks.go \
        api/internal/engine/agents/internal/protocol/internal/translate/inbound/choice_test.go
git commit -m "feat(agents): classify permission events into a provider-blind RiskTier"
```

---

## Task 2: Descriptor YAML — `risk:` classification tables for Claude and Codex

**Files:**
- Modify: `api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/claude.yaml` (the `permission:` block, `claude.yaml:119-153`)
- Modify: `api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/codex.yaml` (the `permission:` block, `codex.yaml:136-145`)
- Test: `api/internal/engine/agents/internal/protocol/internal/descriptor/v3_test.go` (append — this is the suite that loads the real embedded YAML, per its existing role validating `claude.yaml`/`codex.yaml`)

**Interfaces:**
- Consumes: `EventSpec.Risk` (Task 1).
- Produces: nothing new in Go — this task only supplies the data Task 1's classifier reads for the two real providers.

- [ ] **Step 1: Write the failing tests**

Read `api/internal/engine/agents/internal/protocol/internal/descriptor/v3_test.go` first to find how it currently loads a real descriptor by ID (it will call whatever the package's loader entry point is — reuse that exact call, don't invent a new one). Append:

```go
func TestClaudeDescriptor_ClassifiesKnownToolsIntoRiskTiers(t *testing.T) {
	d := loadDescriptor(t, "claude") // use this file's existing loader helper — see its own top for the real name

	risk, ok := d.EventRisk("permission")
	require.True(t, ok)
	assert.Contains(t, risk["read-only"], "Read")
	assert.Contains(t, risk["standard"], "Bash")
	assert.Contains(t, risk["internal"], "mcp__crowbar__*")
}

func TestCodexDescriptor_ClassifiesKnownToolsIntoRiskTiers(t *testing.T) {
	d := loadDescriptor(t, "codex")

	risk, ok := d.EventRisk("permission")
	require.True(t, ok)
	assert.Contains(t, risk["standard"], "shell")
	assert.Contains(t, risk["internal"], "mcp__crowbar__*")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/engine/agents/internal/protocol/internal/descriptor/... -run RiskTiers -v`
Expected: FAIL — `risk["read-only"]`/`risk["standard"]` etc. are empty (the real YAML declares no `risk:` block yet).

- [ ] **Step 3: Implement**

In `claude.yaml`, inside the existing `permission:` event (after its `reply:` block, `claude.yaml:150-153`):

```yaml
    risk:
      read-only: [Read, Grep, Glob, NotebookRead]
      standard:  [Edit, Write, MultiEdit, NotebookEdit, Bash]
      internal:  ["mcp__crowbar__*"]
      # anything else (WebFetch, WebSearch, unrecognized future tools, ...) is
      # sensitive by default — see models.RiskSensitive.
```

In `codex.yaml`, inside the existing `permission:` event (after its `reply:` block, `codex.yaml:142-145`):

```yaml
    risk:
      standard: [shell, apply_patch]
      internal: ["mcp__crowbar__*"]
      # anything else is sensitive by default.
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/engine/agents/internal/protocol/internal/descriptor/... -v`
Expected: PASS, including every pre-existing test in the package (confirms the new `risk:` key doesn't trip descriptor validation).

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/claude.yaml \
        api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/codex.yaml \
        api/internal/engine/agents/internal/protocol/internal/descriptor/v3_test.go
git commit -m "feat(agents): declare risk-tier classification tables for claude and codex"
```

---

## Task 3: `permission` package — `Level`, `AutoApprove` policy, per-chat `Store`

**Files:**
- Create: `api/internal/app/usecases/chat/internal/shared/permission/permission.go`
- Create: `api/internal/app/usecases/chat/internal/shared/permission/store.go`
- Test: `api/internal/app/usecases/chat/internal/shared/permission/permission_test.go`
- Test: `api/internal/app/usecases/chat/internal/shared/permission/store_test.go`

**Interfaces:**
- Consumes: `engineagents.RiskTier` and its four constants (Task 1).
- Produces: `permission.Level` (`string`) and constants `permission.Guarded`, `permission.Trusted`, `permission.FullAuto`; `permission.AutoApprove(level Level, risk engineagents.RiskTier) bool`; `permission.Store` with `New() *Store`, `(*Store) Set(chatID string, level Level)`, `(*Store) Get(chatID string) Level`, `(*Store) Forget(chatID string)`.

- [ ] **Step 1: Write the failing tests**

`permission_test.go`:

```go
package permission_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestAutoApprove(t *testing.T) {
	cases := []struct {
		name  string
		level permission.Level
		risk  engineagents.RiskTier
		want  bool
	}{
		{"guarded holds a read-only tier", permission.Guarded, engineagents.RiskReadOnly, false},
		{"guarded holds a standard tier", permission.Guarded, engineagents.RiskStandard, false},
		{"guarded holds a sensitive tier", permission.Guarded, engineagents.RiskSensitive, false},
		{"guarded still auto-approves internal", permission.Guarded, engineagents.RiskInternal, true},

		{"trusted auto-approves read-only", permission.Trusted, engineagents.RiskReadOnly, true},
		{"trusted auto-approves standard", permission.Trusted, engineagents.RiskStandard, true},
		{"trusted still holds sensitive", permission.Trusted, engineagents.RiskSensitive, false},
		{"trusted auto-approves internal", permission.Trusted, engineagents.RiskInternal, true},

		{"full-auto auto-approves read-only", permission.FullAuto, engineagents.RiskReadOnly, true},
		{"full-auto auto-approves standard", permission.FullAuto, engineagents.RiskStandard, true},
		{"full-auto auto-approves sensitive", permission.FullAuto, engineagents.RiskSensitive, true},
		{"full-auto auto-approves internal", permission.FullAuto, engineagents.RiskInternal, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, permission.AutoApprove(tc.level, tc.risk))
		})
	}
}
```

`store_test.go`:

```go
package permission_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
)

func TestStore_GetDefaultsToGuardedForAnUnseenChat(t *testing.T) {
	s := permission.New()
	assert.Equal(t, permission.Guarded, s.Get("never-set"))
}

func TestStore_SetThenGetRoundTrips(t *testing.T) {
	s := permission.New()
	s.Set("chat-1", permission.FullAuto)
	assert.Equal(t, permission.FullAuto, s.Get("chat-1"))
}

func TestStore_ForgetReturnsToTheDefault(t *testing.T) {
	s := permission.New()
	s.Set("chat-1", permission.Trusted)
	s.Forget("chat-1")
	assert.Equal(t, permission.Guarded, s.Get("chat-1"))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app/usecases/chat/internal/shared/permission/... -v`
Expected: FAIL to compile — package `permission` does not exist yet.

- [ ] **Step 3: Implement**

`permission.go`:

```go
// Package permission owns the per-chat trust dial that decides how much of a
// provider's own tool-call approval Crowbar answers automatically instead of
// holding for a human.
package permission

import engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"

// Level is how far up the RiskTier scale a chat auto-resolves prompts.
type Level string

const (
	// Guarded holds every prompt for a human — today's behavior, unchanged.
	Guarded Level = "guarded"
	// Trusted auto-resolves read-only and standard tiers; sensitive still holds.
	Trusted Level = "trusted"
	// FullAuto auto-resolves every tier, no exceptions.
	FullAuto Level = "full-auto"
)

// AutoApprove reports whether a chat at level should auto-resolve a prompt of
// risk, instead of holding it for a human.
//
// RiskInternal always auto-resolves, independent of level — it marks a call
// to Crowbar's own injected tool surface, which runs with no human present in
// the pane to answer for it. Holding it would not make it safer, only stall
// it forever, so it sits outside the guarded/trusted/full-auto dial entirely.
func AutoApprove(level Level, risk engineagents.RiskTier) bool {
	if risk == engineagents.RiskInternal {
		return true
	}
	switch level {
	case FullAuto:
		return true
	case Trusted:
		return risk == engineagents.RiskReadOnly || risk == engineagents.RiskStandard
	default:
		return false
	}
}
```

`store.go`:

```go
package permission

import "sync"

// Store is the level a chat was seeded with at creation (from the global
// default, see the conversation package) or later switched to. It is
// in-memory only and deliberately not durable — same shape and same reasoning
// as telemetry.Store: a slot describes a live chat's current dial, not a fact
// a daemon restart should resurrect. A chat this store has never seen reports
// Guarded, the safe fallback.
type Store struct {
	mu     sync.RWMutex
	levels map[string]Level
}

func New() *Store {
	return &Store{levels: map[string]Level{}}
}

func (s *Store) Set(chatID string, level Level) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.levels[chatID] = level
}

func (s *Store) Get(chatID string) Level {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if level, ok := s.levels[chatID]; ok {
		return level
	}
	return Guarded
}

func (s *Store) Forget(chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.levels, chatID)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/app/usecases/chat/internal/shared/permission/... -v`
Expected: PASS, all cases.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/chat/internal/shared/permission/
git commit -m "feat(chat): add the permission package — Level, AutoApprove policy, per-chat Store"
```

---

## Task 4: Engine wiring — auto-resolve in `openChoice`, and the Claude-side internal-tool regression fix

**Files:**
- Modify: `api/internal/app/usecases/chat/internal/turn/turns.go` (`Turns` struct at `turns.go:26-72`, `Deps` at `turns.go:78-94`, `New` at `turns.go:98-120`)
- Modify: `api/internal/app/usecases/chat/internal/turn/observation.go` (`openChoice` at `observation.go:67-99`)
- Modify: `api/internal/app/usecases/chat/chat.go` (wherever `turn.New(turn.Deps{...})` is called, `chat.go:311-325`)
- Test: `api/internal/app/usecases/chat/internal/turn/turns_internal_test.go` (append — this is the existing white-box test file for this package's internals)

**Interfaces:**
- Consumes: `permission.Store`, `permission.AutoApprove`, `permission.Guarded` (Task 3); `engineagents.ChoiceToolPermission`, `ev.Choice.Risk` (Task 1); the existing `agent.RenderAnswer(canonical string, raw []byte, decision AnswerDecision) ([]byte, error)`, `answerdesk.Decide(choice, optionIDs, reason, content)`, `t.answers.Resolve(slot, stdout)`, `t.answers.ByChoiceID(choiceID)`, `t.activity.AnswerChoice(ctx, chatID, choiceID, optionIDs, now)`, `domain.ChoiceOptionAllow`.
- Produces: `Turns.permissionLevels *permission.Store` (new field), reachable so Task 6 can call `t.permissionLevels.Set(chatID, level)` when seeding a new chat or handling a level-change request. `turn.Deps` gains `PermissionLevels *permission.Store`.

**No existing test in this package or `answerdesk` exercises the human-hold path end to end** (confirmed: `grep -rln "WithDeliveryID" api/internal --include="*_test.go"` returns nothing) — `holdForAnswer` requires a non-empty `inflight.DeliveryID(ctx)`, and every existing fixture in this repo calls hook-handling code with a bare `context.Background()`. This task is the first to test that path, using `inflight.WithDeliveryID` (`api/internal/app/usecases/chat/internal/shared/inflight/inflight.go:65`) directly, a REAL `engineagents.Agent` from `engineagents.New()` (never a hand-rolled fake — `Agent` is a wide interface and the real claude/codex descriptors already implement it correctly, exactly how `harness_test.go`'s fixture does it elsewhere in this codebase), and a REAL `answerdesk.Desk` (`answerdesk.New(answerdesk.DefaultRetention, nil)` — a nil ledger is explicitly documented as legal for a test that only exercises the desk).

- [ ] **Step 1: Write the failing tests**

Append to `turns_internal_test.go` (same `package turn`, reusing the file's existing `raceChats` fake for `Chats` — it already returns `domain.Chat{ID: id}` for any id, which is exactly what `chatForRunner` needs):

```go
// fakeChoiceActivity is a minimal agentactivity.EventStore recording only the
// three calls the permission/auto-approve path makes — Interrupt, OpenChoice,
// AnswerChoice — mirroring the embed-and-override shape harness_test.go's own
// faultWriteActivity uses one package over, but scoped to this package's own
// narrower needs rather than importing that internal test type across
// packages.
type fakeChoiceActivity struct {
	agentactivity.EventStore
	mu       sync.Mutex
	answered []answeredChoice
}

type answeredChoice struct {
	chatID, choiceID string
	optionIDs        []string
}

func (f *fakeChoiceActivity) Interrupt(
	context.Context, string, string, string, string, time.Time,
) error {
	return nil
}

func (f *fakeChoiceActivity) OpenChoice(context.Context, agentactivity.ChoiceInput) error {
	return nil
}

func (f *fakeChoiceActivity) AnswerChoice(
	_ context.Context, chatID, choiceID string, optionIDs []string, _ time.Time,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answered = append(f.answered, answeredChoice{chatID: chatID, choiceID: choiceID, optionIDs: optionIDs})
	return nil
}

// newChoiceTestTurns builds a real *Turns over the real claude/codex agent
// registry (never a hand-rolled Agent fake — see this task's own note above),
// a real in-memory answerdesk.Desk, and the two narrow fakes this file
// already has a pattern for. Every field New(Deps{}) doesn't set here is left
// zero-value deliberately: the permission-event path never touches Telemetry,
// Workspace, or the inflight gates, so a nil there must never be reached — if
// a future change makes it reach one, that is this test correctly catching a
// widened blast radius, not a fixture gap to paper over.
func newChoiceTestTurns(t *testing.T) (*Turns, *fakeChoiceActivity) {
	t.Helper()
	activity := &fakeChoiceActivity{}
	turns := New(Deps{
		Chats:            raceChats{},
		Activity:         activity,
		Agents:           agents.New(),
		Answers:          answerdesk.New(answerdesk.DefaultRetention, nil),
		PermissionLevels: permission.New(),
		Home:             func() (string, error) { return t.TempDir(), nil },
	})
	return turns, activity
}

// permissionChoiceEvent sets PromptID deliberately: choiceID() (observation.go)
// falls back to an unpredictable, timestamp-based id whenever PromptID is
// empty, which would make a test's expected id nondeterministic. Every test
// below computes its expected id by calling the real choiceID() function
// rather than hand-duplicating its concatenation rule, so a future change to
// that rule cannot silently desync a hand-written literal from the code
// actually running.
func permissionChoiceEvent(toolName string, risk agents.RiskTier) agents.CanonicalEvent {
	return agents.CanonicalEvent{
		Kind: agents.HookPermission,
		Choice: &agents.ChoicePrompt{
			Kind:     agents.ChoiceToolPermission,
			PromptID: "p1",
			ToolName: toolName,
			Risk:     risk,
			Options: []agents.ChoiceOption{
				{ID: domain.ChoiceOptionAllow, Kind: domain.ChoiceOptionAllow, Label: "Allow"},
				{ID: domain.ChoiceOptionDeny, Kind: domain.ChoiceOptionDeny, Label: "Deny"},
			},
		},
		Interrupt: &agents.InterruptEvent{Kind: agents.InterruptPermission},
	}
}

func TestOpenChoice_TrustedLevelAutoApprovesAStandardTierPromptWithNoHumanHold(t *testing.T) {
	turns, activity := newChoiceTestTurns(t)
	turns.permissionLevels.Set("chat-1", permission.Trusted)
	ctx := inflight.WithDeliveryID(context.Background(), "delivery-1")
	agent, err := turns.agents.Get(ctx, t.TempDir(), "claude")
	require.NoError(t, err)
	runner := agents.Runner{ID: "r1", ProviderID: "claude", CurrentChatID: "chat-1"}
	ev := permissionChoiceEvent("Bash", agents.RiskStandard)

	err = turns.handleObservation(ctx, runner, agent, ev, []byte(`{}`))

	require.NoError(t, err)
	id := choiceID("chat-1", ev.Choice)
	_, held := turns.answers.ByChoiceID(id)
	assert.False(t, held, "a trusted-level standard-tier prompt must auto-resolve, not wait for a human")
	require.Len(t, activity.answered, 1)
	assert.Equal(t, []string{domain.ChoiceOptionAllow}, activity.answered[0].optionIDs,
		"the recorded decision must be indistinguishable from a human's own Allow click")
}

func TestOpenChoice_GuardedLevelHoldsASensitiveTierPromptForAHuman(t *testing.T) {
	turns, activity := newChoiceTestTurns(t)
	// No Set call: the chat is unseen, so permission.Store.Get defaults to Guarded.
	ctx := inflight.WithDeliveryID(context.Background(), "delivery-2")
	agent, err := turns.agents.Get(ctx, t.TempDir(), "claude")
	require.NoError(t, err)
	runner := agents.Runner{ID: "r1", ProviderID: "claude", CurrentChatID: "chat-2"}
	ev := permissionChoiceEvent("WebFetch", agents.RiskSensitive)

	err = turns.handleObservation(ctx, runner, agent, ev, []byte(`{}`))

	require.NoError(t, err)
	id := choiceID("chat-2", ev.Choice)
	_, held := turns.answers.ByChoiceID(id)
	assert.True(t, held, "guarded must still hold a sensitive-tier prompt for a human, unchanged")
	assert.Empty(t, activity.answered, "nothing must be auto-decided while the prompt is held")
}

// TestRegression_CrowbarsOwnMCPToolCallNeverHoldsForAHumanOnAnyProvider is the
// parity fix: Crowbar's own injected tool calls run in a pane with no human
// present to answer a modal (see codex.yaml's default_tools_approval_mode
// comment) — RiskInternal must bypass the level dial entirely, even at the
// strictest level, on EITHER provider, since Risk is a canonical field the
// engine reasons about directly rather than a per-provider CLI workaround.
func TestRegression_CrowbarsOwnMCPToolCallNeverHoldsForAHumanOnAnyProvider(t *testing.T) {
	for _, providerID := range []string{"claude", "codex"} {
		t.Run(providerID, func(t *testing.T) {
			turns, activity := newChoiceTestTurns(t)
			// Guarded, the strictest level — proving RiskInternal bypasses the dial
			// entirely rather than merely being permitted at a lenient one.
			ctx := inflight.WithDeliveryID(context.Background(), "delivery-"+providerID)
			agent, err := turns.agents.Get(ctx, t.TempDir(), providerID)
			require.NoError(t, err)
			chatID := "chat-" + providerID
			runner := agents.Runner{ID: "r1", ProviderID: providerID, CurrentChatID: chatID}
			ev := permissionChoiceEvent("mcp__crowbar__resolve_review_thread", agents.RiskInternal)

			err = turns.handleObservation(ctx, runner, agent, ev, []byte(`{}`))

			require.NoError(t, err)
			id := choiceID(chatID, ev.Choice)
			_, held := turns.answers.ByChoiceID(id)
			assert.False(t, held, "an internal-tier call must never stall waiting for a human")
		})
	}
}
```

Add `"sync"`, `agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"` (already imported), `"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"`, `"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"`, `"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"`, and `"github.com/stretchr/testify/require"` to this file's imports if not already present (it currently imports `assert` but not `require`, per the file as read).

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app/usecases/chat/internal/turn/... -run 'AutoApproves|GuardedLevelHolds|Regression_CrowbarsOwn' -v`
Expected: FAIL — either a compile error (`t.permissionLevels` undefined, `Deps.PermissionLevels` undefined) or the assertion that nothing was held, because today `openChoice` always calls `holdForAnswer` with no auto-resolve path.

- [ ] **Step 3: Implement**

In `turns.go`, add the field to the struct (next to `answers *answerdesk.Desk`) and to `Deps`:

```go
	answers          *answerdesk.Desk
	permissionLevels *permission.Store
```

```go
type Deps struct {
	...
	Answers          *answerdesk.Desk
	PermissionLevels *permission.Store
	...
}
```

In `New(d Deps) *Turns`, set it alongside `answers: d.Answers,`:

```go
		answers:          d.Answers,
		permissionLevels: d.PermissionLevels,
```

Add the import `"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"`.

In `chat.go`, wherever `turn.New(turn.Deps{..., Answers: sh.answers, ...})` is called (`chat.go:311-325`), add a sibling `shared` field and pass it — mirroring exactly how `sh.answers` itself is built and threaded:

```go
	sh := shared{
		...
		answers:          answerdesk.New(answerdesk.DefaultRetention, d.Activity),
		permissionLevels: permission.New(),
	}
	...
	u.turns = turn.New(turn.Deps{
		...
		Answers:          sh.answers,
		PermissionLevels: sh.permissionLevels,
		...
	})
```

(Add `permissionLevels *permission.Store` to the `shared` struct alongside its existing `answers *answerdesk.Desk` field, and the `permission` import.) Task 6 reuses this same `sh.permissionLevels` pointer when it wires `conversation.Deps`.

In `observation.go`, add the auto-resolve call after `holdForAnswer` in `openChoice`:

```go
	t.holdForAnswer(ctx, chat, runner, agent, ev, id, raw)
	t.autoApproveIfPolicy(ctx, chatID, id, ev, agent)
}

// autoApproveIfPolicy resolves a just-opened choice immediately when the
// chat's permission level clears the prompt's risk tier, using the exact
// render-and-resolve sequence a human's Allow click uses — so even under
// full-auto, the transcript shows a decision was made, not silence.
//
// Only a plain tool-permission prompt (Kind == ChoiceToolPermission) is ever
// eligible: a question prompt (AskUserQuestion) has no Allow option to pick,
// and an elicitation is out of scope by design (see the spec) — both carry no
// meaningful Risk tier and must fall through to the human hold untouched.
func (t *Turns) autoApproveIfPolicy(
	ctx context.Context,
	chatID string,
	choiceID string,
	ev engineagents.CanonicalEvent,
	agent engineagents.Agent,
) {
	if ev.Choice == nil || ev.Choice.Kind != engineagents.ChoiceToolPermission {
		return
	}
	level := t.permissionLevels.Get(chatID)
	if !permission.AutoApprove(level, ev.Choice.Risk) {
		return
	}
	slot, held := t.answers.ByChoiceID(choiceID)
	if !held {
		return
	}
	// Same safety property the human path enforces in AnswerChoice
	// (chat/answers.go): a provider that cannot express "allow" for this
	// event must never have one manufactured for it.
	if !slot.Keys.Accepts(domain.ChoiceOptionAllow) {
		return
	}
	decision := engineagents.AnswerDecision{Key: domain.ChoiceOptionAllow}
	stdout, err := agent.RenderAnswer(slot.Event, slot.Raw, decision)
	if err != nil {
		slog.WarnContext(ctx, "agent: permission: auto-approve: render", "err", err, "choice_id", choiceID)
		return
	}
	if err := t.activity.AnswerChoice(
		ctx, chatID, choiceID, []string{domain.ChoiceOptionAllow}, time.Now(),
	); err != nil {
		slog.WarnContext(ctx, "agent: permission: auto-approve: ledger", "err", err, "choice_id", choiceID)
		return
	}
	t.answers.Resolve(slot, stdout)
}
```

`slot.Event`/`slot.Raw` are the same fields `holdForAnswer` populated on the `answerdesk.Prompt` a moment earlier (`answerdesk.go`'s `Slot` embeds `Prompt`, which has `Event`/`Raw` — confirm the exact field names against `answerdesk/answerdesk.go`'s `Prompt`/`Slot` definitions before compiling; they are used identically in `chat/answers.go`'s `AnswerChoice` as `slot.Event, slot.Raw`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/app/usecases/chat/... -v`
Expected: PASS across the whole `chat` usecase tree — this is the first task that changes real request-handling behavior, so run the full tree, not just the `turn` package.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/chat/internal/turn/turns.go \
        api/internal/app/usecases/chat/internal/turn/observation.go \
        api/internal/app/usecases/chat/chat.go \
        api/internal/app/usecases/chat/internal/turn/turns_internal_test.go
git commit -m "feat(chat): auto-resolve tool-permission prompts the chat's level clears"
```

---

## Task 5: Global default level — domain type, `defaultlevel` sub-component, HTTP endpoint

This mirrors how `provider.Providers` is already structured: a small, independently-testable sub-component over a GORM-backed `store.Store`, tested directly with a real in-memory sqlite store (`provider_test.go`'s own pattern) rather than through the full `chat.Usecase` fixture — `Usecase.DefaultPermissionLevel`/`SetDefaultPermissionLevel` end up as one-line delegates, exactly like `Usecase.ReplaceProviderPreferences` (`providers.go:80-84`) already is.

**Files:**
- Create: `api/internal/domain/agent_permission_default.go`
- Create: `api/internal/app/usecases/chat/internal/defaultlevel/defaultlevel.go`
- Test: `api/internal/app/usecases/chat/internal/defaultlevel/defaultlevel_test.go`
- Modify: `api/internal/app/usecases/container.go` (`GORMStores` struct at `container.go:29-37`, wiring at `container.go:265` area)
- Find and modify the app-layer `GORMStores` construction site: run `grep -rn "AgentProviderPreferences" api/internal --include="*.go"` first — it leads to (a) the container.go sites above, and (b) at least one more site higher up (the app-layer struct that actually builds the GORM-backed store and the GORM auto-migration list). Mirror every site you find for the new type.
- Modify: `api/internal/app/usecases/chat/chat.go` (new `Deps.PermissionPrefs` field, `u.defaultLevel` built in `buildComponents`)
- Create: `api/internal/app/usecases/chat/permission_default.go` (two one-line delegate methods)
- Modify: `api/internal/api/v0/endpoints/chat/routes.go` (new route, alongside `settingsRG.PUT("/settings/chat/providers", ...)`)
- Modify: `api/internal/api/v0/endpoints/chat/handlers/handlers.go` (interface methods, alongside the `ReplaceProviderPreferences` block at `handlers.go:292-296`)
- Modify: `api/internal/api/v0/endpoints/chat/handlers/hooks_test.go` (add fields/methods to the shared `fakeAgentUsecase`)
- Create: `api/internal/api/v0/endpoints/chat/handlers/permission_default.go`
- Test: `api/internal/api/v0/endpoints/chat/handlers/permission_default_test.go`

**Interfaces:**
- Consumes: `permission.Level`, `permission.Guarded`/`Trusted`/`FullAuto` (Task 3); `store.Store[T, K]` (`api/internal/adapter/store`).
- Produces: `domain.AgentPermissionDefault{ID, Level string}` + `domain.DefaultPermissionLevelKey` (GORM row, fixed key). `defaultlevel.DefaultLevel` with `New(Deps{Prefs store.Store[domain.AgentPermissionDefault, string]}) *DefaultLevel`, `(*DefaultLevel) Get(ctx) (permission.Level, error)`, `(*DefaultLevel) Set(ctx, permission.Level) error`. `Usecase.DefaultPermissionLevel(ctx) (permission.Level, error)` / `Usecase.SetDefaultPermissionLevel(ctx, permission.Level) error`. `PUT`/`GET /v0/settings/chat/permission-level`.

- [ ] **Step 1: Write the failing tests**

`api/internal/app/usecases/chat/internal/defaultlevel/defaultlevel_test.go` (mirrors `provider_test.go`'s `newTable` pattern exactly — a real in-memory sqlite store, never a hand-rolled fake `store.Store`):

```go
package defaultlevel_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/defaultlevel"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newTable(t *testing.T) *defaultlevel.DefaultLevel {
	t.Helper()
	prefs, err := storesqlite.New[domain.AgentPermissionDefault, string](":memory:")
	require.NoError(t, err)
	return defaultlevel.New(defaultlevel.Deps{Prefs: prefs})
}

func TestGet_UnsetFallsBackToFullAuto(t *testing.T) {
	t.Parallel()
	table := newTable(t)

	level, err := table.Get(t.Context())

	require.NoError(t, err)
	assert.Equal(t, permission.FullAuto, level,
		"the shipped default is full-auto until a user has ever changed it in Settings")
}

func TestSet_ThenGetRoundTrips(t *testing.T) {
	t.Parallel()
	table := newTable(t)

	require.NoError(t, table.Set(t.Context(), permission.Guarded))
	level, err := table.Get(t.Context())

	require.NoError(t, err)
	assert.Equal(t, permission.Guarded, level)
}

func TestSet_RejectsAnUnknownLevel(t *testing.T) {
	t.Parallel()
	table := newTable(t)

	err := table.Set(t.Context(), permission.Level("yolo"))

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}
```

`api/internal/api/v0/endpoints/chat/handlers/permission_default_test.go` (using the real harness confirmed in `answers_test.go`: `newTestContext`, `newChatHandlers`, `inWorkspace`, `libs.WriteQueryOK`'s `{"data": ...}` envelope shape):

```go
package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
)

func TestGetDefaultPermissionLevel_ReturnsTheCurrentLevel(t *testing.T) {
	uc := &fakeAgentUsecase{defaultLevel: permission.Trusted}
	ctx, rec := newTestContext(t, http.MethodGet, "/v0/settings/chat/permission-level", nil)

	newChatHandlers(uc).GetDefaultPermissionLevel(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data struct {
			Level string `json:"level"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.Equal(t, "trusted", envelope.Data.Level)
}

func TestPutDefaultPermissionLevel_ForwardsTheLevel(t *testing.T) {
	uc := &fakeAgentUsecase{}
	ctx, rec := newTestContext(t, http.MethodPut, "/v0/settings/chat/permission-level",
		[]byte(`{"level":"full-auto"}`))

	newChatHandlers(uc).PutDefaultPermissionLevel(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.setDefaultLevelCalls, 1)
	assert.Equal(t, permission.FullAuto, uc.setDefaultLevelCalls[0])
}

func TestPutDefaultPermissionLevel_RejectsAMalformedBody(t *testing.T) {
	uc := &fakeAgentUsecase{}
	ctx, rec := newTestContext(t, http.MethodPut, "/v0/settings/chat/permission-level", []byte(`{`))

	newChatHandlers(uc).PutDefaultPermissionLevel(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.setDefaultLevelCalls)
}

func TestPutDefaultPermissionLevel_SurfacesAnUnknownLevel(t *testing.T) {
	uc := &fakeAgentUsecase{setDefaultLevelErr: apperr.ErrInvalidArgument}
	ctx, rec := newTestContext(t, http.MethodPut, "/v0/settings/chat/permission-level",
		[]byte(`{"level":"yolo"}`))

	newChatHandlers(uc).PutDefaultPermissionLevel(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
```

In `hooks_test.go`, add to the shared `fakeAgentUsecase` struct (wherever its other per-method fields like `answerErr`/`answerCalls` are declared) and its method set:

```go
	defaultLevel         permission.Level
	defaultLevelErr      error
	setDefaultLevelCalls []permission.Level
	setDefaultLevelErr   error
```

```go
func (f *fakeAgentUsecase) DefaultPermissionLevel(context.Context) (permission.Level, error) {
	return f.defaultLevel, f.defaultLevelErr
}

func (f *fakeAgentUsecase) SetDefaultPermissionLevel(_ context.Context, level permission.Level) error {
	if f.setDefaultLevelErr != nil {
		return f.setDefaultLevelErr
	}
	f.setDefaultLevelCalls = append(f.setDefaultLevelCalls, level)
	return nil
}
```

(Add the `permission` import to `hooks_test.go` if not already present.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app/usecases/chat/internal/defaultlevel/... ./internal/api/v0/endpoints/chat/... -run 'DefaultPermissionLevel|Get_Unset|Set_' -v`
Expected: compile failures — none of the new types/methods exist yet.

- [ ] **Step 3: Implement**

`api/internal/domain/agent_permission_default.go`:

```go
package domain

// AgentPermissionDefault is the one global row holding the permission level a
// new chat is seeded with. It is a singleton by convention (see
// DefaultPermissionLevelKey), not a table with one row per anything.
type AgentPermissionDefault struct {
	ID    string `gorm:"primaryKey"`
	Level string
}

// DefaultPermissionLevelKey is the fixed primary key AgentPermissionDefault is
// always saved and loaded under.
const DefaultPermissionLevelKey = "default"
```

`api/internal/app/usecases/chat/internal/defaultlevel/defaultlevel.go`:

```go
// Package defaultlevel owns the one global row holding the permission level a
// newly created chat is seeded with (see conversation.Deps.DefaultPermissionLevel,
// Task 6).
package defaultlevel

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// valid is the closed set Set accepts. Anything else — a typo, a future level
// not yet shipped — is rejected rather than silently stored, so Get can never
// hand a chat-seeding caller a Level permission.AutoApprove doesn't know how
// to interpret.
var valid = map[permission.Level]bool{
	permission.Guarded:  true,
	permission.Trusted:  true,
	permission.FullAuto: true,
}

type DefaultLevel struct {
	prefs store.Store[domain.AgentPermissionDefault, string]
}

type Deps struct {
	Prefs store.Store[domain.AgentPermissionDefault, string]
}

func New(d Deps) *DefaultLevel {
	return &DefaultLevel{prefs: d.Prefs}
}

// Get is the level a newly created chat is seeded with. Unset (no row saved
// yet) reports permission.FullAuto — the shipped out-of-the-box default —
// not an error.
func (d *DefaultLevel) Get(ctx context.Context) (permission.Level, error) {
	row, err := d.prefs.FindByKey(ctx, domain.DefaultPermissionLevelKey)
	if err != nil {
		return "", fmt.Errorf("agent: default permission level: %w", err)
	}
	if row == nil {
		return permission.FullAuto, nil
	}
	return permission.Level(row.Level), nil
}

// Set overwrites the global default. It rejects any level outside the closed
// set before writing anything.
func (d *DefaultLevel) Set(ctx context.Context, level permission.Level) error {
	if !valid[level] {
		return fmt.Errorf("%w: unknown permission level %q", apperr.ErrInvalidArgument, level)
	}
	if err := d.prefs.Save(ctx, domain.AgentPermissionDefault{
		ID: domain.DefaultPermissionLevelKey, Level: string(level),
	}); err != nil {
		return fmt.Errorf("agent: set default permission level: %w", err)
	}
	return nil
}
```

In `container.go`, add to `GORMStores` (next to `AgentProviderPreferences`):

```go
	AgentProviderPreferences store.Store[domain.AgentProviderPreference, string]
	AgentPermissionDefault   store.Store[domain.AgentPermissionDefault, string]
```

Then run the grep from the Files section, and for every other site it surfaces (the app-layer `GORMStores` struct that actually constructs the GORM store, the GORM auto-migration list), add a directly analogous `AgentPermissionDefault` line next to the existing `AgentProviderPreferences` one.

**Critical, not optional — read before moving on:** `Usecase.DefaultPermissionLevel`/`SetDefaultPermissionLevel` call `u.defaultLevel.Get`/`.Set`, which dereference `d.prefs` (the `store.Store` this task threads through `Deps.PermissionPrefs`). `MintChat` (Task 6) calls `DefaultPermissionLevel` on **every single chat creation**, including in production. If `Deps.PermissionPrefs` is left unwired anywhere `agentusecase.New(agentusecase.Deps{...})` is actually called with a real, non-empty `Deps` literal, chat creation panics on a nil-interface method call the first time it runs. A repo-wide check (`grep -rn "agentusecase\.New(agentusecase\.Deps{" api/internal --include="*.go"`) found exactly three such call sites, and this task must fix the two that build a working chat surface:

1. **`api/internal/app/usecases/container.go:257-271` — the PRODUCTION composition root.** Add one line next to the existing `ProviderPrefs: gormStores.AgentProviderPreferences,` (line 265):

   ```go
   		ProviderPrefs:   gormStores.AgentProviderPreferences,
   		PermissionPrefs: gormStores.AgentPermissionDefault,
   ```

   Missing this means every real user's first new chat after this feature ships panics the daemon. This is not hypothetical — trace it yourself before moving on: `MintChat` → `defaultPermissionLevel(ctx)` → `u.DefaultPermissionLevel` → `u.defaultLevel.Get` → `d.prefs.FindByKey` on a nil interface.

2. **`api/internal/app/usecases/chat/harness_test.go`'s `newFixtureUsing`** (the shared fixture dozens of tests across this package use via `newFixture(t)`/`f.spawn(...)`) — add a real in-memory store next to the existing `providerPrefs` construction (`harness_test.go:1026-1027`):

   ```go
   	permissionPrefs, err := storesqlite.New[domain.AgentPermissionDefault, string](":memory:")
   	require.NoError(t, err)
   ```

   and add `PermissionPrefs: permissionPrefs,` to the `agentusecase.New(agentusecase.Deps{...})` literal (`harness_test.go:1063-1084`), next to the existing `ProviderPrefs: providerPrefs,`. Without this, every existing test in `providers_test.go`/`mcp_test.go`/etc. that calls `f.spawn(...)` starts panicking the moment Task 6 lands, even though none of those tests are about permission levels at all — this is the single highest-blast-radius omission in this whole plan, precisely because it looks unrelated to what those tests are testing.

The third call site, `providers_test.go:327`'s `agentusecase.New(agentusecase.Deps{})` and `providers_integration_test.go:39`'s `agentusecase.Deps{...}` (no `Chats`/`Runners` at all), never call `MintChat`/`SpawnChat` — confirmed by reading what they exercise (`DispatchMCP` on an unconfigured surface, and the provider-preference HTTP routes only) — so they are safe unmodified. Do not add `PermissionPrefs` to them; doing so would be unrelated noise on a test that was never at risk.

In `chat/chat.go`, add to `Deps` (next to `ProviderPrefs`):

```go
	ProviderPrefs   store.Store[domain.AgentProviderPreference, string]
	PermissionPrefs store.Store[domain.AgentPermissionDefault, string]
```

and in `buildComponents`, alongside `u.providers = provider.New(...)`:

```go
	u.defaultLevel = defaultlevel.New(defaultlevel.Deps{Prefs: d.PermissionPrefs})
```

(Add a `defaultLevel *defaultlevel.DefaultLevel` field to `Usecase` and the `defaultlevel` import.)

`api/internal/app/usecases/chat/permission_default.go`:

```go
package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
)

// DefaultPermissionLevel is the level a newly created chat is seeded with.
func (u *Usecase) DefaultPermissionLevel(ctx context.Context) (permission.Level, error) {
	return u.defaultLevel.Get(ctx)
}

// SetDefaultPermissionLevel overwrites the global default.
func (u *Usecase) SetDefaultPermissionLevel(ctx context.Context, level permission.Level) error {
	return u.defaultLevel.Set(ctx, level)
}
```

Add both signatures to whichever usecase interface `providers.go`'s `ReplaceProviderPreferences` is declared on (same interface, same doc-comment style, `providers.go:23-28`).

In `api/internal/api/v0/endpoints/chat/handlers/handlers.go`, add to the interface (mirroring `ReplaceProviderPreferences` at `handlers.go:292-296`):

```go
	// DefaultPermissionLevel and SetDefaultPermissionLevel back
	// GET/PUT /v0/settings/chat/permission-level.
	DefaultPermissionLevel(ctx context.Context) (permission.Level, error)
	SetDefaultPermissionLevel(ctx context.Context, level permission.Level) error
```

`api/internal/api/v0/endpoints/chat/handlers/permission_default.go` — find the exact `Handlers` field name `providers.go`'s own methods call through (e.g. `h.providers.ResolveProviders(...)` — read that file first) and use the identical field here, since both interfaces are satisfied by the same underlying `chat.Usecase`:

```go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
)

// GetDefaultPermissionLevel handles GET /v0/settings/chat/permission-level.
func (h *Handlers) GetDefaultPermissionLevel(ctx *gin.Context) {
	level, err := h.providers.DefaultPermissionLevel(ctx.Request.Context())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryOK(ctx, gin.H{"level": string(level)})
}

// PutDefaultPermissionLevel handles PUT /v0/settings/chat/permission-level.
func (h *Handlers) PutDefaultPermissionLevel(ctx *gin.Context) {
	var body struct {
		Level string `json:"level"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	err := h.providers.SetDefaultPermissionLevel(ctx.Request.Context(), permission.Level(body.Level))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryOK(ctx, gin.H{"level": body.Level})
}
```

In `routes.go`, alongside `settingsRG.PUT("/settings/chat/providers", h.UpdateProviderPreferences)`:

```go
	settingsRG.GET("/settings/chat/permission-level", h.GetDefaultPermissionLevel)
	settingsRG.PUT("/settings/chat/permission-level", h.PutDefaultPermissionLevel)
```

- [ ] **Step 4: Run tests to verify they pass**

Run first: `cd api && go test ./internal/app/usecases/chat/... ./internal/api/v0/endpoints/chat/... -v`
Then, because this task's `PermissionPrefs` wiring has repo-wide blast radius (see the critical note in Step 3): `cd api && go test ./...`
Expected: PASS everywhere. Any panic anywhere in the tree at this step means a construction site was missed — go back to Step 3's grep, not forward.

- [ ] **Step 5: Commit**

```bash
git add api/internal/domain/agent_permission_default.go \
        api/internal/app/usecases/chat/internal/defaultlevel/ \
        api/internal/app/usecases/container.go \
        api/internal/app/usecases/chat/chat.go \
        api/internal/app/usecases/chat/permission_default.go \
        api/internal/api/v0/endpoints/chat/routes.go \
        api/internal/api/v0/endpoints/chat/handlers/handlers.go \
        api/internal/api/v0/endpoints/chat/handlers/permission_default.go \
        api/internal/api/v0/endpoints/chat/handlers/permission_default_test.go \
        api/internal/api/v0/endpoints/chat/handlers/hooks_test.go \
        api/internal/app/usecases/chat/harness_test.go
# add any additional files the AgentProviderPreferences grep surfaced in Step 3
git commit -m "feat(chat): global default permission level, GET/PUT /v0/settings/chat/permission-level"
```

---

## Task 6: Seed a new chat's level from the global default; per-chat level-change endpoint

**Files:**
- Modify: `api/internal/app/usecases/chat/internal/conversation/conversation.go` (`MintChat` at `conversation.go:250-265`, its `Deps`)
- Modify: `api/internal/app/usecases/chat/chat.go` (thread `sh.permissionLevels` and `u.DefaultPermissionLevel` into `conversation.Deps`, alongside Task 4's `turn.Deps` wiring)
- Create: `api/internal/app/usecases/chat/permission_level.go` (`SetChatPermissionLevel` usecase method)
- Modify: `api/internal/api/v0/endpoints/chat/routes.go` (new per-chat route, alongside `wsScoped.POST("/chats/:id/choices/:choiceId/answer", h.AnswerChoice)`)
- Modify: `api/internal/api/v0/endpoints/chat/handlers/handlers.go` (interface method)
- Modify: `api/internal/api/v0/endpoints/chat/handlers/hooks_test.go` (add `SetChatPermissionLevel` to `fakeAgentUsecase`)
- Create: `api/internal/api/v0/endpoints/chat/handlers/permission_level.go`
- Test: `api/internal/app/usecases/chat/internal/conversation/conversation_test.go` (append)
- Test: `api/internal/app/usecases/chat/permission_level_test.go` (new, using the real `chat_test` fixture from `harness_test.go`)
- Test: `api/internal/api/v0/endpoints/chat/handlers/permission_level_test.go`

**Interfaces:**
- Consumes: `permission.Store` (Task 3, the same `sh.permissionLevels` instance Task 4 wired into `turn.Deps`); `Usecase.DefaultPermissionLevel` (Task 5).
- Produces: every new chat has an entry in `permission.Store` immediately on creation. `Usecase.SetChatPermissionLevel(ctx, chatID string, level permission.Level) error`. `PUT /v0/workspaces/:wsId/chats/:id/permission-level`.

- [ ] **Step 1: Write the failing tests**

In `conversation_test.go`, append — reusing the file's own `newChatStore`/`newRunnerStore`/`newActivityStore`/`stubWorkspace`/`stubLineage` helpers directly (not the shared `newFixture(t, lineage)`, since these two tests need a non-default `DefaultPermissionLevel` closure `newFixture` doesn't take a parameter for):

```go
func TestMintChat_SeedsThePermissionLevelFromTheCurrentGlobalDefault(t *testing.T) {
	t.Parallel()
	chats, _ := newChatStore(t)
	runners, _ := newRunnerStore(t)
	activity, _ := newActivityStore(t)
	home := t.TempDir()
	levels := permission.New()

	conversations := conversation.New(conversation.Deps{
		Chats:     chats,
		Runners:   runners,
		Activity:  activity,
		Telemetry: telemetry.New(),
		Agents:    engineagents.New(),
		Workspace: stubWorkspace{home: home, worktree: filepath.Join(home, "projects", "p1", "slug", "branch", "worktree")},
		Lineage:   stubLineage{},
		Home:      func() (string, error) { return home, nil },
		Work:      inflight.NewWork(),
		Spawns:    inflight.NewGate(),

		PermissionLevels:       levels,
		DefaultPermissionLevel: func(context.Context) (permission.Level, error) { return permission.Trusted, nil },
	})

	chatID, err := conversations.MintChat(t.Context(), "ws-1")

	require.NoError(t, err)
	assert.Equal(t, permission.Trusted, levels.Get(chatID))
}

func TestMintChat_ChangingTheGlobalDefaultDoesNotRetroactivelyChangeAnAlreadyOpenChat(t *testing.T) {
	t.Parallel()
	chats, _ := newChatStore(t)
	runners, _ := newRunnerStore(t)
	activity, _ := newActivityStore(t)
	home := t.TempDir()
	levels := permission.New()
	current := permission.Guarded

	conversations := conversation.New(conversation.Deps{
		Chats:     chats,
		Runners:   runners,
		Activity:  activity,
		Telemetry: telemetry.New(),
		Agents:    engineagents.New(),
		Workspace: stubWorkspace{home: home, worktree: filepath.Join(home, "projects", "p1", "slug", "branch", "worktree")},
		Lineage:   stubLineage{},
		Home:      func() (string, error) { return home, nil },
		Work:      inflight.NewWork(),
		Spawns:    inflight.NewGate(),

		PermissionLevels:       levels,
		DefaultPermissionLevel: func(context.Context) (permission.Level, error) { return current, nil },
	})

	chatID, err := conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)
	require.Equal(t, permission.Guarded, levels.Get(chatID))

	current = permission.FullAuto // the global default changes AFTER this chat was minted

	assert.Equal(t, permission.Guarded, levels.Get(chatID),
		"an already-open chat's level must not drift when the global default later changes")
}
```

Add `"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"` to this file's imports.

`api/internal/app/usecases/chat/permission_level_test.go` (using the REAL `chat_test` fixture from `harness_test.go` — `newFixture(t)`, `f.usecase`, `f.spawn` — the same harness `providers_test.go` uses; do not build a second construction path):

```go
package chat_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
)

func TestSetChatPermissionLevel_RejectsAnUnknownLevel(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	err := f.usecase.SetChatPermissionLevel(f.ctx, chatID, permission.Level("yolo"))

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestSetChatPermissionLevel_RejectsAnUnknownChat(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.SetChatPermissionLevel(f.ctx, "never-created", permission.Guarded)

	require.Error(t, err)
}

func TestSetChatPermissionLevel_Succeeds(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.SetChatPermissionLevel(f.ctx, chatID, permission.FullAuto))
}
```

(A third assertion — that the level change actually took effect — is exactly what Task 4's `TestOpenChoice_TrustedLevelAutoApprovesAStandardTierPromptWithNoHumanHold`-style test already proves at the `permission.Store` level; this file only needs to prove the usecase method validates and writes, not re-prove the auto-approve policy a second time.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app/usecases/chat/... -run 'MintChat_Seeds|MintChat_Changing|SetChatPermissionLevel' -v`
Expected: compile failures / FAIL — `MintChat` doesn't seed anything yet, `SetChatPermissionLevel` doesn't exist.

- [ ] **Step 3: Implement**

In `conversation.go`, add to `Deps`:

```go
type Deps struct {
	...
	PermissionLevels *permission.Store
	// DefaultPermissionLevel resolves the current global default at the
	// moment a chat is minted. It's a closure, not a direct usecase
	// reference, the same way Home is a closure — conversation must not
	// import the chat usecase package it lives inside of.
	DefaultPermissionLevel func(ctx context.Context) (permission.Level, error)
}
```

and seed it in `MintChat`, right after the chat is created:

```go
func (c *Conversations) MintChat(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	chatID := uuid.NewString()
	created, err := c.chats.Create(ctx, agentchat.CreateInput{
		ID:          chatID,
		WorkspaceID: workspaceID,
		Now:         time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("agent: mint chat: %w", err)
	}
	c.work.Set(chatID, created.Working)
	if level, err := c.defaultPermissionLevel(ctx); err == nil {
		c.permissionLevels.Set(chatID, level)
	}
	return chatID, nil
}
```

(Store `permissionLevels *permission.Store` and `defaultPermissionLevel func(ctx context.Context) (permission.Level, error)` on `Conversations` from `Deps` in its constructor, next to its existing `work *inflight.Work` field. A default-level read failure is swallowed here — not propagated — because a chat must still get created even if the default-level lookup has trouble; `permission.Store.Get`'s own `Guarded` fallback (Task 3) is the safety net for a chat that was never seeded.)

In `chat.go`, thread the two new deps into `conversation.Deps` next to the existing `Work: sh.work,` line:

```go
	u.conversations = conversation.New(conversation.Deps{
		...
		Work:                   sh.work,
		PermissionLevels:       sh.permissionLevels,
		DefaultPermissionLevel: u.DefaultPermissionLevel,
		...
	})
```

`api/internal/app/usecases/chat/permission_level.go`:

```go
package chat

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
)

// SetChatPermissionLevel overrides one chat's level for the rest of its
// lifetime, independent of the global default (see DefaultPermissionLevel).
// The override is in-memory only, exactly like the level itself.
func (u *Usecase) SetChatPermissionLevel(
	ctx context.Context,
	chatID string,
	level permission.Level,
) error {
	if !validPermissionLevels[level] {
		return fmt.Errorf("%w: unknown permission level %q", apperr.ErrInvalidArgument, level)
	}
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return fmt.Errorf("agent: set chat permission level: chat: %w", err)
	}
	u.permissionLevels.Set(chatID, level)
	return nil
}
```

(`u.permissionLevels` is the same `sh.permissionLevels` pointer Task 4 already wired onto `Usecase`/`Turns` — add the field to `Usecase` in `chat.go` next to `answers *answerdesk.Desk` if it isn't already reachable there.)

Handler (`permission_level.go`), routed as `wsScoped.PUT("/chats/:id/permission-level", h.SetChatPermissionLevel)` in `routes.go`, following `answers.go`'s `AnswerChoice` handler shape exactly (`h.requireChatInWorkspace`, `ctx.ShouldBindJSON`, `libs.WriteErr`/`libs.WriteMutationOK`):

```go
func (h *Handlers) SetChatPermissionLevel(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	var body struct {
		Level string `json:"level"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	err := h.answers.SetChatPermissionLevel(ctx.Request.Context(), chat.ID, permission.Level(body.Level))
	if err != nil {
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}
	libs.WriteMutationOK(ctx, http.StatusOK, chat.ID)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/app/usecases/chat/... ./internal/api/v0/endpoints/chat/... -v`

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/chat/internal/conversation/conversation.go \
        api/internal/app/usecases/chat/internal/conversation/conversation_test.go \
        api/internal/app/usecases/chat/chat.go \
        api/internal/app/usecases/chat/permission_level.go \
        api/internal/app/usecases/chat/permission_level_test.go \
        api/internal/api/v0/endpoints/chat/routes.go \
        api/internal/api/v0/endpoints/chat/handlers/handlers.go \
        api/internal/api/v0/endpoints/chat/handlers/permission_level.go \
        api/internal/api/v0/endpoints/chat/handlers/permission_level_test.go
git commit -m "feat(chat): seed new chats from the global default level; per-chat override endpoint"
```

---

## Task 7: Frontend — "Default permission level" in Settings → Agents

**Files:**
- Modify: `web/src/features/agent/api/agent-api.ts` (two new calls, alongside `updateProviderPreferences` at `agent-api.ts:742-751`)
- Modify: `web/src/features/settings/stores/agent-providers-store.ts` (or a small sibling — read the file first; if it's scoped tightly to the provider list, add a parallel small store rather than overloading it, per "one clear responsibility per file")
- Modify: `web/src/features/settings/components/tabs/providers-settings.tsx` (new row inside the existing `Section title="Agents"` at `providers-settings.tsx:185`)
- Test: `web/src/__tests__/features/settings/components/tabs/providers-settings.test.tsx` (append, or the equivalent file this component already has — confirm the exact path first; §4 of the earlier research confirmed the mirrored-path convention with `chat-presentation-setting.test.tsx` as the real example to follow)

**Interfaces:**
- Consumes: `GET`/`PUT /v0/settings/chat/permission-level` (Task 5).
- Produces: nothing new consumed by later tasks (Task 8 reads the per-chat endpoint from Task 6, not this one).

- [ ] **Step 1: Write the failing test**

Read `chat-presentation-setting.test.tsx` first for the exact render/assert idiom this codebase uses for a settings control, and the real path/import style. Write a test for the new control at whatever path Step 3 creates it, asserting:
- it renders three options (Guarded, Trusted, Full Auto),
- selecting "Guarded" calls the PUT endpoint with `{ level: "guarded" }`,
- on mount it reflects whatever `GET /v0/settings/chat/permission-level` returned.

Do not guess the exact mocking helper (`vi.fn()` wiring, MSW, or a store mock) — copy the real one `chat-presentation-setting.test.tsx` uses, since `providers-settings.tsx`'s own tests already mock its backend calls the same way `updateProviderPreferences` is mocked there.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bun vitest run src/__tests__/features/settings/components/tabs/providers-settings.test.tsx`
Expected: FAIL — the control doesn't exist yet.

- [ ] **Step 3: Implement**

In `agent-api.ts`, alongside `updateProviderPreferences`:

```ts
export type PermissionLevel = 'guarded' | 'trusted' | 'full-auto'

export async function getDefaultPermissionLevel(): Promise<PermissionLevel> {
  const res = await apiFetch<{ level: PermissionLevel }>(`/v0/settings/chat/permission-level`)
  return res.level
}

export async function updateDefaultPermissionLevel(level: PermissionLevel): Promise<PermissionLevel> {
  const res = await apiFetch<{ level: PermissionLevel }>(`/v0/settings/chat/permission-level`, {
    method: 'PUT',
    body: JSON.stringify({ level }),
  })
  return res.level
}
```

(Match `apiFetch`'s real call signature exactly as `updateProviderPreferences` uses it — headers/body-serialization conventions in that function must not be reinvented here.)

Add the `Select` row inside `providers-settings.tsx`'s existing `Section title="Agents"`, following the exact `SettingRow` + `Select`/`SelectTrigger`/`SelectValue`/`SelectContent`/`SelectItem` pattern already used at `editor-settings.tsx:122-146`:

```tsx
<SettingRow label="Default permission level" description="How much of a new chat's tool-call approval is answered automatically.">
  <Select value={defaultPermissionLevel} onValueChange={handlePermissionLevelChange}>
    <SelectTrigger size="sm" className={SETTINGS_CONTROL_WIDTHS.default}>
      <SelectValue />
    </SelectTrigger>
    <SelectContent>
      <SelectItem value="guarded">Guarded</SelectItem>
      <SelectItem value="trusted">Trusted</SelectItem>
      <SelectItem value="full-auto">Full Auto</SelectItem>
    </SelectContent>
  </Select>
</SettingRow>
```

Load `defaultPermissionLevel` on mount via `getDefaultPermissionLevel()` (component-local `useState` + `useEffect`, or the small sibling store from the Files note — decide based on what you find `chat-presentation-setting.tsx` doing for its own single boolean, and match it), and call `updateDefaultPermissionLevel` from `handlePermissionLevelChange`, optimistically setting local state first the same way `providers-settings.tsx:103-131`'s `commit()` does for provider rows.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bun vitest run src/__tests__/features/settings/components/tabs/providers-settings.test.tsx`
Then: `cd web && bun tsc --noEmit` (per project memory, `bunx tsc` is a different package than `bun tsc` — always use `bun tsc`).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/api/agent-api.ts \
        web/src/features/settings/components/tabs/providers-settings.tsx \
        web/src/__tests__/features/settings/components/tabs/providers-settings.test.tsx
# add the sibling store file from Step 3 if you created one
git commit -m "feat(settings): default permission level control in Settings > Agents"
```

---

## Task 8: Frontend — per-chat permission-level switcher (conflict-aware, do this last)

**Before touching anything in this task:** run `git status --porcelain` and re-read `web/src/features/agent/composer/composer-choice.tsx` fresh, regardless of what it looked like earlier in this plan's research. If the concurrent session's edits now touch the exact lines you need, stop and say so rather than forcing a merge — this file was confirmed mid-edit by another session while this plan was being written.

**Files:**
- Modify: `web/src/features/agent/api/agent-api.ts` (one new call, using `PermissionLevel` from Task 7)
- Modify: `web/src/features/agent/composer/composer-choice.tsx` (smallest possible isolated addition — a single new small component, not a restructure)
- Test: `web/src/__tests__/features/agent/composer/composer-choice.test.tsx` (append — confirm exact existing path first)

**Interfaces:**
- Consumes: `PUT /v0/workspaces/:wsId/chats/:id/permission-level` (Task 6); `PermissionLevel` type (Task 7).

- [ ] **Step 1: Write the failing test**

Read the existing `composer-choice.test.tsx` (or find its real path if renamed by the concurrent session) for its render harness first. Add a test asserting a level switcher renders near the Allow/Deny buttons and that changing it calls the new API function with the active chat id and the selected level.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bun vitest run <the real test file path>`

- [ ] **Step 3: Implement**

In `agent-api.ts`:

```ts
export async function setChatPermissionLevel(
  wsId: string,
  chatId: string,
  level: PermissionLevel,
): Promise<void> {
  await apiFetch(`/v0/workspaces/${wsId}/chats/${chatId}/permission-level`, {
    method: 'PUT',
    body: JSON.stringify({ level }),
  })
}
```

Add one new small component, e.g. `web/src/features/agent/composer/permission-level-switcher.tsx` (kebab-case file, `PermissionLevelSwitcher` export), reusing the same `Select` primitives as Task 7's settings control. Mount it inside `composer-choice.tsx` with the smallest possible diff — a single import line and a single render call near the existing `ChoiceButtons`, not a restructure of the file's existing layout.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bun vitest run <the real test file path>` then `cd web && bun tsc --noEmit`.

- [ ] **Step 5: Manual verification, then commit**

Per project convention, tests passing is not sufficient proof for a UI change: start the app (`make dev-desktop`) and drive a real chat through Guarded → Trusted → Full Auto **against both a Claude session and a Codex session** (per the spec's own §9 testing plan — this task is the first point in the whole plan where a live CLI is actually exercised end to end), confirming the transcript distinguishes an auto-approved entry from a human-approved one (Task 4's ledger write) on both providers.

```bash
git add web/src/features/agent/api/agent-api.ts \
        web/src/features/agent/composer/permission-level-switcher.tsx \
        web/src/features/agent/composer/composer-choice.tsx \
        web/src/__tests__/features/agent/composer/composer-choice.test.tsx
git commit -m "feat(chat): per-chat permission-level switcher next to the approve/deny prompt"
```

---

## Final check

Run the full backend and frontend suites once more end to end before considering this plan done: `cd api && go test ./...` and `cd web && bun vitest run && bun tsc --noEmit`. Then re-read the spec (`docs/superpowers/specs/2026-08-26-crowbar-permission-levels-design.md`) section by section against what was actually built, and confirm every section has a corresponding task above.
