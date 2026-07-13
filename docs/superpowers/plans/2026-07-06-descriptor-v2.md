# Descriptor v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every per-CLI fact live in the YAML descriptor — write side (hook-config authored literally) and read side (hook payloads mapped declaratively) — and build the conversation log from hooks instead of reading vendor transcript files.

**Architecture:** Big-bang refactor of the agent engine. The hook-config file becomes literal `write_file` content authored in YAML (no Go renderer). `crowbar hook` forwards the raw payload verbatim; the daemon parses it per the descriptor's declared `format` and extracts fields per a per-event map. The ledger stores hook-derived conversation turns (`user`/`assistant`), and the switch handoff renders that clean conversation. No v1 fallback, no dead code.

**Tech Stack:** Go, gorm+glebarez/sqlite (single-conn), cobra, gin, yaml.v3, creack/pty, testify.

**Spec:** [`docs/superpowers/specs/2026-07-06-descriptor-v2-design.md`](../specs/2026-07-06-descriptor-v2-design.md)

## Global Constraints

- **Big bang, no dead code.** No v1/v2 coexistence, no feature flag, no compat shim, no fallback. Deleted symbols are deleted, not deprecated. A v1-format descriptor must fail to parse.
- **This is the only provider path.** There is exactly one code path through which Crowbar reads and uses providers.
- **Interactive PTY only.** Never spawn a headless CLI; the `forbid_flags` guard in `BuildSpawnPlan` stays load-bearing.
- **A hook must never break the vendor CLI.** `crowbar hook` swallows all errors to exit-0 (stderr only); the daemon drops malformed payloads (log + return nil), never 5xx-loops a turn.
- **Test file location:** Go tests are co-located (`_test.go` beside source), matching the existing repo layout. (The `web/src/__tests__` mirror rule in CLAUDE.md is frontend-only and does not apply here.)
- **Canonical event vocabulary (fixed, Crowbar-owned):** `session_start`, `user_prompt`, `turn_stop`.
- **Guaranteed template variables:** `{tmp}`, `{cwd}`, `{crowbar_hook}`, `{segid}`, `{provider}`, `{id}`, `{handoff}`.
- **Per-task green ≠ full build.** Because Go compiles a package under test with its imports (not its importers), each task is verified by its own package's tests even while a not-yet-updated downstream package won't build. Task 8 restores `go build ./...` + the full suite. Do not treat a downstream compile error from an as-yet-unmodified caller as task failure.

**Task order & why:** 1 Template (leaf) → 2 Engine core (descriptor/hooks/inject/YAML) → 3 Ledger → 4 Domain → 5 `crowbar hook` CLI → 6 Usecase → 7 API handler → 8 Integration + full build. Each depends only on lower-numbered tasks.

---

### Task 1: Template variables (`{segid}`, `{provider}`; drop unused)

**Files:**
- Modify: `api/internal/engine/agent/template.go`
- Test: `api/internal/engine/agent/template_test.go`

**Interfaces:**
- Produces: `agent.TemplateCtx{ Tmp, ID, Handoff, Cwd, CrowbarHook, Segid, Provider string }`; `agent.Expand(s string, ctx TemplateCtx) string` expanding `{tmp} {id} {handoff} {cwd} {crowbar_hook} {segid} {provider}`.

- [ ] **Step 1: Update the test to the new variable set**

Replace the body of `api/internal/engine/agent/template_test.go` with:

```go
package agent_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestExpand_ReplacesKnownTokens(t *testing.T) {
	ctx := agent.TemplateCtx{Tmp: "/t", ID: "s9", Cwd: "/w", CrowbarHook: "/bin/crowbar", Segid: "seg1", Provider: "claude"}
	require.Equal(t, "/t/settings.json", agent.Expand("{tmp}/settings.json", ctx))
	require.Equal(t, "resume s9", agent.Expand("resume {id}", ctx))
	require.Equal(t, "handoff=HI", agent.Expand("handoff={handoff}", agent.TemplateCtx{Handoff: "HI"}))
	require.Equal(t,
		"/bin/crowbar hook turn_stop --segment seg1 --provider claude",
		agent.Expand("{crowbar_hook} hook turn_stop --segment {segid} --provider {provider}", ctx))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/engine/agent/ -run TestExpand_ReplacesKnownTokens`
Expected: FAIL (compile error — `Segid`/`Provider` fields don't exist yet).

- [ ] **Step 3: Rewrite `template.go`**

Replace the whole file with:

```go
package agent

import "strings"

type TemplateCtx struct {
	Tmp         string
	ID          string
	Handoff     string
	Cwd         string
	CrowbarHook string
	Segid       string
	Provider    string
}

func Expand(s string, ctx TemplateCtx) string {
	r := strings.NewReplacer(
		"{tmp}", ctx.Tmp,
		"{id}", ctx.ID,
		"{handoff}", ctx.Handoff,
		"{cwd}", ctx.Cwd,
		"{crowbar_hook}", ctx.CrowbarHook,
		"{segid}", ctx.Segid,
		"{provider}", ctx.Provider,
	)
	return r.Replace(s)
}
```

This drops the unused `UUID`/`{uuid}`, `CwdSlug`/`{cwd_slug}`, and `SessionID`/`{session_id}` (verified: no descriptor or Go caller references them).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./internal/engine/agent/ -run TestExpand_ReplacesKnownTokens`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/agent/template.go api/internal/engine/agent/template_test.go
git commit -m "refactor(agent): add {segid}/{provider} template vars, drop unused {uuid}/{cwd_slug}/{session_id}"
```

---

### Task 2: Engine v2 core — descriptor types, read-side mapping, literal write side

This is the heart of the refactor. `descriptor.go`, `hooks.go`, `inject.go`, and both embedded YAML descriptors change together so the package parses its own descriptors and compiles.

**Files:**
- Modify: `api/internal/engine/agent/descriptor.go` (types, `Validate`, add `ParsePayload`)
- Modify: `api/internal/engine/agent/hooks.go` (`CanonicalEvent.Message`, `MapHook`, dotted-path `extract`)
- Modify: `api/internal/engine/agent/inject.go` (delete `render_hooks` verb + `renderHooks`)
- Modify: `api/internal/engine/agent/descriptors/claude.yaml`
- Modify: `api/internal/engine/agent/descriptors/codex.yaml`
- Test: `descriptor_test.go`, `descriptor_error_test.go`, `hooks_test.go`, `inject_test.go`, `inject_error_test.go`

**Interfaces:**
- Consumes: `agent.TemplateCtx` (Task 1).
- Produces:
  - `agent.HookSpec{ Format string; Events map[string]map[string]string }`; `Descriptor.Hooks HookSpec`.
  - `(*Descriptor).ParsePayload(raw []byte) (map[string]any, error)`.
  - `agent.CanonicalEvent{ Kind, SessionID, Message string; Raw map[string]any }`.
  - `(*Descriptor).MapHook(canonical string, payload map[string]any) (CanonicalEvent, error)`.
  - `HookMap` type and `provider_event`/`render_hooks` are **removed**.

- [ ] **Step 1: Rewrite `descriptor.go`**

```go
package agent

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Descriptor holds only fields the engine consumes. Every field here is
// load-bearing; provider-specific shapes (hook-config layout, native event
// names) live in the descriptor's literal write-side content, never here.
type Descriptor struct {
	ID    string `yaml:"id"`
	Spawn struct {
		Cmd                 string   `yaml:"cmd"`
		InteractiveRequired bool     `yaml:"interactive_required"`
		ForbidFlags         []string `yaml:"forbid_flags"`
		Args                []string `yaml:"args"`
		Env                 struct {
			Clear []string `yaml:"clear"`
		} `yaml:"env"`
	} `yaml:"spawn"`
	Session struct {
		Resume *ArgSpec `yaml:"resume"`
	} `yaml:"session"`
	ConfigInjection []InjectStep `yaml:"config_injection"`
	Hooks           HookSpec     `yaml:"hooks"`
	HandoffInject   []InjectStep `yaml:"handoff_inject"`
}

type ArgSpec struct {
	Arg string `yaml:"arg"`
}

// HookSpec is the read side: how to parse this CLI's hook payloads (format) and
// where each Crowbar vocabulary field sits inside each canonical event's
// payload (events[canonical][vocab] = dotted path).
type HookSpec struct {
	Format string                       `yaml:"format"`
	Events map[string]map[string]string `yaml:"events"`
}

// InjectStep is one declarative injection verb, e.g. `- pass_arg: {arg: --settings, value: x}`.
// The YAML is a single-key map; the key is the verb, the value is its args.
type InjectStep struct {
	Verb string
	Args map[string]any
}

func (s *InjectStep) UnmarshalYAML(value *yaml.Node) error {
	var m map[string]map[string]any
	if err := value.Decode(&m); err != nil {
		return fmt.Errorf("agent: inject step decode: %w", err)
	}
	if len(m) != 1 {
		return fmt.Errorf("agent: inject step must have exactly one verb, got %d", len(m))
	}
	for verb, args := range m {
		s.Verb = verb
		s.Args = args
	}
	return nil
}

func LoadDescriptor(data []byte) (*Descriptor, error) {
	var d Descriptor
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("agent: descriptor unmarshal: %w", err)
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return &d, nil
}

func (d *Descriptor) Validate() error {
	if d.ID == "" {
		return fmt.Errorf("agent: descriptor missing id")
	}
	if d.Spawn.Cmd == "" {
		return fmt.Errorf("agent: descriptor %q missing spawn.cmd", d.ID)
	}
	if !d.Spawn.InteractiveRequired {
		return fmt.Errorf("agent: descriptor %q must set spawn.interactive_required", d.ID)
	}
	if d.Hooks.Format == "" {
		return fmt.Errorf("agent: descriptor %q missing hooks.format", d.ID)
	}
	if ss := d.Hooks.Events["session_start"]; ss["session_id"] == "" {
		return fmt.Errorf("agent: descriptor %q hooks.events.session_start must map session_id", d.ID)
	}
	if ts := d.Hooks.Events["turn_stop"]; ts["message"] == "" {
		return fmt.Errorf("agent: descriptor %q hooks.events.turn_stop must map message", d.ID)
	}
	return nil
}

// ParsePayload decodes raw hook bytes into a map per the descriptor's declared
// format. An unsupported format is an explicit error (documented boundary).
func (d *Descriptor) ParsePayload(raw []byte) (map[string]any, error) {
	switch d.Hooks.Format {
	case "json":
		if len(raw) == 0 {
			return map[string]any{}, nil
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("agent: descriptor %q parse json payload: %w", d.ID, err)
		}
		return m, nil
	default:
		return nil, fmt.Errorf("agent: descriptor %q unsupported hooks.format %q", d.ID, d.Hooks.Format)
	}
}
```

- [ ] **Step 2: Rewrite `hooks.go`**

```go
package agent

import (
	"fmt"
	"strings"
)

type CanonicalEvent struct {
	Kind      string
	SessionID string
	Message   string
	Raw       map[string]any
}

func (d *Descriptor) MapHook(canonical string, payload map[string]any) (CanonicalEvent, error) {
	fields, ok := d.Hooks.Events[canonical]
	if !ok {
		return CanonicalEvent{}, fmt.Errorf("agent: descriptor %q has no hook %q", d.ID, canonical)
	}
	get := func(field string) string {
		path, ok := fields[field]
		if !ok {
			return ""
		}
		return extract(payload, path)
	}
	return CanonicalEvent{
		Kind:      canonical,
		SessionID: get("session_id"),
		Message:   get("message"),
		Raw:       payload,
	}, nil
}

// extract walks a dotted path ("a.b.c") into a decoded payload, returning "" for
// any missing segment or a non-string leaf. A bare key ("session_id") is a
// one-segment path.
func extract(payload map[string]any, path string) string {
	if path == "" {
		return ""
	}
	var cur any = payload
	for _, p := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur, ok = m[p]
		if !ok {
			return ""
		}
	}
	if s, ok := cur.(string); ok {
		return s
	}
	return ""
}
```

- [ ] **Step 3: Edit `inject.go` — delete the `render_hooks` verb and `renderHooks`**

Remove the `"encoding/json"` import (now unused). Change `runStep` to drop the `d *Descriptor` parameter and the `render_hooks` case; delete the entire `renderHooks` function. The new `runStep` and its call site:

```go
// in BuildSpawnPlan, change the call:
//   if err := runStep(d, st, ctx, plan); err != nil {
// to:
//   if err := runStep(st, ctx, plan); err != nil {

func runStep(st InjectStep, ctx TemplateCtx, plan *SpawnPlan) error {
	arg := func(k string) string { return Expand(asString(st.Args[k]), ctx) }
	switch st.Verb {
	case "set_env":
		plan.Env = append(plan.Env, arg("name")+"="+arg("value"))
	case "write_file":
		return writeFileStep(arg("path"), arg("content"), arg("from"))
	case "pass_arg":
		if pos, ok := st.Args["positional"]; ok {
			plan.Argv = append(plan.Argv, Expand(asString(pos), ctx))
			return nil
		}
		plan.Argv = append(plan.Argv, arg("arg"))
		if _, ok := st.Args["value"]; ok {
			plan.Argv = append(plan.Argv, arg("value"))
		}
	default:
		return fmt.Errorf("agent: unknown inject verb %q", st.Verb)
	}
	return nil
}
```

Leave `writeFileStep`, `clearEnv`, `asString`, `expandHome`, `copyFile`, and the `forbid_flags` guard in `BuildSpawnPlan` unchanged.

- [ ] **Step 4: Rewrite `descriptors/claude.yaml`**

```yaml
# Validated against claude 2.1.201 (Phase-0 spike). Documentation only.
id: claude
spawn:
  cmd: claude
  interactive_required: true
  forbid_flags: ["-p", "--print"]
  env:
    clear: ["CLAUDE_CODE_CHILD_SESSION", "CLAUDECODE"]
session:
  resume: { arg: "--resume {id}" }
config_injection:
  - write_file:
      path: "{tmp}/settings.json"
      content: |
        {"hooks":{
          "SessionStart":[{"hooks":[{"type":"command",
            "command":"{crowbar_hook} hook session_start --segment {segid} --provider {provider}"}]}],
          "UserPromptSubmit":[{"hooks":[{"type":"command",
            "command":"{crowbar_hook} hook user_prompt --segment {segid} --provider {provider}"}]}],
          "Stop":[{"hooks":[{"type":"command",
            "command":"{crowbar_hook} hook turn_stop --segment {segid} --provider {provider}"}]}]
        }}
  - pass_arg: { arg: "--settings", value: "{tmp}/settings.json" }
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt:   { message: prompt }
    turn_stop:     { session_id: session_id, message: last_assistant_message }
handoff_inject:
  - pass_arg: { arg: "--append-system-prompt", value: "{handoff}" }
```

- [ ] **Step 5: Rewrite `descriptors/codex.yaml`**

```yaml
# Validated against codex 0.139.0 (Phase-0 spike). Documentation only.
id: codex
spawn:
  cmd: codex
  interactive_required: true
  forbid_flags: ["exec"]
  args: ["--dangerously-bypass-hook-trust"]
session:
  resume: { arg: "resume {id}" }
config_injection:
  - set_env:    { name: CODEX_HOME, value: "{tmp}/codex-home" }
  - write_file: { path: "{tmp}/codex-home/auth.json", from: "~/.codex/auth.json" }
  - write_file:
      path: "{tmp}/codex-home/config.toml"
      content: |
        [projects."{cwd}"]
        trust_level = "trusted"
  - write_file:
      path: "{tmp}/codex-home/hooks.json"
      content: |
        {"hooks":{
          "SessionStart":[{"hooks":[{"type":"command",
            "command":"{crowbar_hook} hook session_start --segment {segid} --provider {provider}"}]}],
          "UserPromptSubmit":[{"hooks":[{"type":"command",
            "command":"{crowbar_hook} hook user_prompt --segment {segid} --provider {provider}"}]}],
          "Stop":[{"hooks":[{"type":"command",
            "command":"{crowbar_hook} hook turn_stop --segment {segid} --provider {provider}"}]}]
        }}
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt:   { message: prompt }
    turn_stop:     { session_id: session_id, message: last_assistant_message }
handoff_inject:
  - pass_arg: { positional: "{handoff}" }
```

- [ ] **Step 6: Update `descriptor_error_test.go` — v2 `validMinimalDescriptor` + validation cases**

Replace the `validMinimalDescriptor` const and the two hook-shape-dependent tests. New const:

```go
const validMinimalDescriptor = `
id: testprov
spawn:
  cmd: testcmd
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: last_assistant_message }
`
```

Rewrite `TestLoadDescriptor_RejectsSessionStartMissingSessionIDField` and add two new cases:

```go
func TestLoadDescriptor_RejectsSessionStartMissingSessionIDField(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte(`
id: testprov
spawn: { cmd: testcmd, interactive_required: true }
hooks:
  format: json
  events:
    session_start: { }
    turn_stop: { message: last_assistant_message }
`))
	require.Error(t, err)
}

func TestLoadDescriptor_RejectsMissingHooksFormat(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte(`
id: testprov
spawn: { cmd: testcmd, interactive_required: true }
hooks:
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: last_assistant_message }
`))
	require.Error(t, err)
}

func TestLoadDescriptor_RejectsTurnStopMissingMessage(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte(`
id: testprov
spawn: { cmd: testcmd, interactive_required: true }
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { }
`))
	require.Error(t, err)
}
```

Also update the inline `hooks:` blocks in `TestLoadDescriptor_RejectsMissingSpawnCmd` and `TestLoadDescriptor_RejectsInteractiveRequiredFalse` to the v2 shape (`format: json` + `events:`), so each isolates its intended failure. `TestLoadDescriptor_RejectsMissingSessionStartHook` (no `hooks:` at all) still errors — leave as is.

- [ ] **Step 7: Update `descriptor_test.go` — assert v2 shape**

Replace lines 17–18 (`ProviderEvent`/`Fields` asserts) with:

```go
	require.Equal(t, "json", d.Hooks.Format)
	require.Equal(t, "session_id", d.Hooks.Events["session_start"]["session_id"])
	require.Equal(t, "last_assistant_message", d.Hooks.Events["turn_stop"]["message"])
```

Add `ParsePayload` tests to this file:

```go
func TestParsePayload_JSON(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	m, err := d.ParsePayload([]byte(`{"session_id":"x"}`))
	require.NoError(t, err)
	require.Equal(t, "x", m["session_id"])
}

func TestParsePayload_UnknownFormatErrors(t *testing.T) {
	d, err := agent.LoadDescriptor([]byte(`
id: p
spawn: { cmd: x, interactive_required: true }
hooks:
  format: toml
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: msg }
`))
	require.NoError(t, err)
	_, err = d.ParsePayload([]byte("x=1"))
	require.Error(t, err)
}
```

- [ ] **Step 8: Update `hooks_test.go` — message extraction + dotted path**

Replace the whole file with:

```go
package agent_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestMapHook_ClaudeTurnStop(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	ev, err := d.MapHook("turn_stop", map[string]any{
		"session_id":             "abc-123",
		"last_assistant_message": "acknowledged",
	})
	require.NoError(t, err)
	require.Equal(t, "turn_stop", ev.Kind)
	require.Equal(t, "abc-123", ev.SessionID)
	require.Equal(t, "acknowledged", ev.Message)
}

func TestMapHook_ClaudeUserPrompt(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	ev, err := d.MapHook("user_prompt", map[string]any{"prompt": "hello there"})
	require.NoError(t, err)
	require.Equal(t, "hello there", ev.Message)
}

func TestMapHook_DottedPathDescent(t *testing.T) {
	d, err := agent.LoadDescriptor([]byte(`
id: nested
spawn: { cmd: x, interactive_required: true }
hooks:
  format: json
  events:
    session_start: { session_id: session.id }
    turn_stop: { message: result.text }
`))
	require.NoError(t, err)
	ev, err := d.MapHook("turn_stop", map[string]any{
		"result": map[string]any{"text": "deep"},
	})
	require.NoError(t, err)
	require.Equal(t, "deep", ev.Message)
}

func TestMapHook_MissingFieldYieldsEmpty(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	ev, err := d.MapHook("turn_stop", map[string]any{"session_id": "s"})
	require.NoError(t, err)
	require.Equal(t, "", ev.Message)
}

func TestMapHook_UnknownCanonicalErrors(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	_, err = d.MapHook("nope", map[string]any{})
	require.Error(t, err)
}
```

- [ ] **Step 9: Update `inject_test.go` — literal content, `{segid}`/`{provider}` substitution**

In `TestBuildSpawnPlan_ClaudeWritesSettingsAndArgs`, set `Segid`/`Provider` on the ctx and assert the literal command is present:

```go
	ctx := agent.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/bin/crowbar", Segid: "seg-9", Provider: "claude"}
	// ...
	require.Contains(t, string(data), "SessionStart")
	require.Contains(t, string(data), "/bin/crowbar hook turn_stop --segment seg-9 --provider claude")
```

In `TestBuildSpawnPlan_CodexSetsHomeAndBypassFlag`, set `Segid: "seg-c", Provider: "codex"` on the ctx; after the `os.Stat(... "/hooks.json")` check, assert its content:

```go
	hooksData, err := os.ReadFile(envValue(plan.Env, "CODEX_HOME") + "/hooks.json")
	require.NoError(t, err)
	require.Contains(t, string(hooksData), "--segment seg-c --provider codex")
```

Leave `TestBuildSpawnPlan_RejectsForbiddenFlag` unchanged.

- [ ] **Step 10: Update `inject_error_test.go` — delete the `render_hooks` test**

Delete `TestRenderHooksStep_MkdirFailsWhenIntoParentIsAFile` entirely (the verb no longer exists; `TestWriteFileStep_MkdirFailsWhenPathParentIsAFile` already covers the write_file mkdir-failure branch). All other tests use `validMinimalDescriptor` (now v2) and are unaffected.

- [ ] **Step 11: Run the whole engine/agent package**

Run: `cd api && go test ./internal/engine/agent/...`
Expected: PASS (all files compile; descriptors parse under v2; no `render_hooks`/`provider_event`/`Transcript` references remain).

- [ ] **Step 12: Commit**

```bash
git add api/internal/engine/agent/
git commit -m "refactor(agent): descriptor v2 core — literal write side, declarative read side, drop render_hooks/transcript"
```

---

### Task 3: Ledger stores conversation turns

**Files:**
- Modify: `api/internal/app/ledger/ledger.go`
- Test: `api/internal/app/ledger/ledger_test.go`

**Interfaces:**
- Produces:
  - `ledger.Turn{ Role, Provider, Text string; At time.Time }`.
  - `(*Ledger).AppendTurn(role, provider string, at time.Time, text string) (string, error)` — empty text is a no-op returning `("", nil)`.
  - `(*Ledger).RenderConversation() ([]byte, error)`.
  - `ledger.Open(dir)` unchanged.
  - `Append`/`ReadAll` are **removed**.

- [ ] **Step 1: Rewrite the tests**

Replace `TestLedger_AppendThenReadAllOrdered` with:

```go
func TestLedger_AppendTurnsThenRenderOrdered(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)
	at := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)

	_, err = l.AppendTurn("user", "claude", at, "FIRST")
	require.NoError(t, err)
	_, err = l.AppendTurn("assistant", "claude", at.Add(time.Minute), "SECOND")
	require.NoError(t, err)

	all, err := l.RenderConversation()
	require.NoError(t, err)
	s := string(all)
	require.Less(t, indexOf(s, "FIRST"), indexOf(s, "SECOND"))
	require.Contains(t, s, "user: FIRST")
	require.Contains(t, s, "assistant (claude): SECOND")
}

func TestLedger_AppendTurn_EmptyTextIsNoOp(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)
	name, err := l.AppendTurn("assistant", "claude", time.Now(), "")
	require.NoError(t, err)
	require.Empty(t, name)
	out, err := l.RenderConversation()
	require.NoError(t, err)
	require.Empty(t, out)
}
```

In the remaining error-path tests, replace every `l.Append("claude", <at>, []byte("x"))` with `l.AppendTurn("assistant", "claude", <at>, "x")`, and every `l.ReadAll()` with `l.RenderConversation()`. Keep their permission-manipulation logic and error-substring assertions (`"mkdir"`, `"write"`, `"read"`) unchanged. In `TestLedger_AppendSequenceIncrementsAcrossEntries`, keep the `00000001-`/`00000002-`/`00000003-` prefix assertions (unchanged — same `%08d` scheme).

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app/ledger/...`
Expected: FAIL (compile — `AppendTurn`/`RenderConversation` undefined).

- [ ] **Step 3: Rewrite `ledger.go`**

```go
// Package ledger implements the per-chat, append-only, provider-tagged store of
// conversation turns Crowbar derives from vendor-CLI hooks (agentic-engine
// descriptor-v2 §7). Crowbar builds this record itself; it never reads a file
// the vendor CLI wrote.
package ledger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Turn is one conversation turn recorded from a hook.
type Turn struct {
	Role     string    `json:"role"`     // "user" | "assistant"
	Provider string    `json:"provider"`
	Text     string    `json:"text"`
	At       time.Time `json:"at"`
}

// Ledger is a per-chat, append-only store of conversation turns.
type Ledger struct{ dir string }

// Open ensures the ledger directory exists and returns a handle to it.
func Open(dir string) (*Ledger, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("ledger: mkdir: %w", err)
	}
	return &Ledger{dir: dir}, nil
}

// AppendTurn records one conversation turn. Empty text is a no-op (returns
// ("", nil)) so a provider that fires turn_stop with no final message never
// writes a blank entry. The %08d prefix keeps lexical == chronological order.
func (l *Ledger) AppendTurn(role, provider string, at time.Time, text string) (string, error) {
	if text == "" {
		return "", nil
	}
	seq, err := l.nextSeq()
	if err != nil {
		return "", err
	}
	rec, err := json.Marshal(Turn{Role: role, Provider: provider, Text: text, At: at.UTC()})
	if err != nil {
		return "", fmt.Errorf("ledger: marshal turn: %w", err)
	}
	name := fmt.Sprintf("%08d-%s-%s-%s.turn", seq, at.UTC().Format("20060102T150405Z"), role, provider)
	if err := os.WriteFile(filepath.Join(l.dir, name), rec, 0o640); err != nil { //nolint:gosec // ledger entries are group-readable by design; name is ledger-generated
		return "", fmt.Errorf("ledger: write: %w", err)
	}
	return name, nil
}

func (l *Ledger) entries() ([]string, error) {
	des, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, fmt.Errorf("ledger: readdir: %w", err)
	}
	var names []string
	for _, de := range des {
		if !de.IsDir() && filepath.Ext(de.Name()) == ".turn" {
			names = append(names, de.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func (l *Ledger) nextSeq() (int, error) {
	names, err := l.entries()
	if err != nil {
		return 0, err
	}
	return len(names) + 1, nil
}

// RenderConversation reads every turn in order and renders a legible plain-text
// conversation for a receiving model.
func (l *Ledger) RenderConversation() ([]byte, error) {
	names, err := l.entries()
	if err != nil {
		return nil, err
	}
	var out []byte
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(l.dir, n)) //nolint:gosec // n comes from entries() listing l.dir, not external input
		if err != nil {
			return nil, fmt.Errorf("ledger: read %s: %w", n, err)
		}
		var tn Turn
		if err := json.Unmarshal(data, &tn); err != nil {
			return nil, fmt.Errorf("ledger: unmarshal %s: %w", n, err)
		}
		header := tn.Role
		if tn.Role == "assistant" && tn.Provider != "" {
			header = fmt.Sprintf("assistant (%s)", tn.Provider)
		}
		out = append(out, []byte(header+": "+tn.Text+"\n\n")...)
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/app/ledger/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/ledger/
git commit -m "refactor(ledger): store hook-derived conversation turns, render clean chat log"
```

---

### Task 4: Drop `TranscriptPath` from the domain segment

**Files:**
- Modify: `api/internal/domain/agent_segment.go`

**Interfaces:**
- Produces: `domain.AgentSegment` without the `TranscriptPath` field.

- [ ] **Step 1: Delete the field**

In `api/internal/domain/agent_segment.go`, delete the line:

```go
	TranscriptPath    string     `json:"transcriptPath"`
```

- [ ] **Step 2: Confirm no non-usecase consumers remain**

Run: `cd api && grep -rn "TranscriptPath" --include=*.go . | grep -v _test.go | grep -v internal/app/usecases/agent/agent.go`
Expected: no output. (The usecase references in `agent.go` are removed in Task 6; repository/adapter/web have none — pre-verified.)

- [ ] **Step 3: Build the domain package**

Run: `cd api && go build ./internal/domain/...`
Expected: success. (No AutoMigrate change needed — gorm leaves the now-unused column in place; pre-production, no migration per project convention.)

- [ ] **Step 4: Commit**

```bash
git add api/internal/domain/agent_segment.go
git commit -m "refactor(domain): drop AgentSegment.TranscriptPath (transcript reading removed)"
```

---

### Task 5: `crowbar hook` — raw byte forwarder with `--segment`/`--provider`

**Files:**
- Modify: `api/cmd/crowbar/hook.go`
- Test: `api/cmd/crowbar/hook_test.go`

**Interfaces:**
- Produces: `runHook(event, segment, provider string, payload []byte, host string) error`; POSTs `{segment_id, provider, event, payload_raw}` to `/v0/agent/hooks`. `resolvePayload(inline, file string, stdin io.Reader) ([]byte, error)` with precedence inline > file > stdin. Reads no env var.

- [ ] **Step 1: Rewrite the test**

Replace `api/cmd/crowbar/hook_test.go`'s test function with:

```go
func TestRunHook_ForwardsSegmentProviderAndRawPayload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "hook")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	sock := filepath.Join(tmpDir, "h.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	var mu sync.Mutex
	var got map[string]any
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		_ = json.NewDecoder(r.Body).Decode(&got)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	err = runHook("turn_stop", "seg-42", "claude", []byte(`{"session_id":"abc"}`), "unix://"+sock)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "seg-42", got["segment_id"])
	require.Equal(t, "claude", got["provider"])
	require.Equal(t, "turn_stop", got["event"])
	require.Equal(t, `{"session_id":"abc"}`, got["payload_raw"])
}

func TestResolvePayload_Precedence(t *testing.T) {
	f := filepath.Join(t.TempDir(), "p.json")
	require.NoError(t, os.WriteFile(f, []byte("FROMFILE"), 0o644))

	inline, err := resolvePayload("INLINE", f, strings.NewReader("FROMSTDIN"))
	require.NoError(t, err)
	require.Equal(t, "INLINE", string(inline))

	fromFile, err := resolvePayload("", f, strings.NewReader("FROMSTDIN"))
	require.NoError(t, err)
	require.Equal(t, "FROMFILE", string(fromFile))

	fromStdin, err := resolvePayload("", "", strings.NewReader("FROMSTDIN"))
	require.NoError(t, err)
	require.Equal(t, "FROMSTDIN", string(fromStdin))
}
```

Ensure the import block includes `"io"` and drops nothing still used. (`encoding/json`, `net`, `net/http`, `os`, `path/filepath`, `strings`, `sync`, `testing`, testify all stay.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./cmd/crowbar/ -run 'TestRunHook_ForwardsSegmentProviderAndRawPayload|TestResolvePayload_Precedence'`
Expected: FAIL (signature mismatch / `resolvePayload` undefined).

- [ ] **Step 3: Rewrite `hook.go`**

```go
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

func newHookCmd() *cobra.Command {
	var segment, provider, payloadFile, payloadInline string
	cmd := &cobra.Command{
		Use:    "hook <event>",
		Short:  "Forward a vendor-CLI hook payload to the Crowbar daemon",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(_ *cobra.Command, args []string) error {
			// A hook must never break the vendor CLI: swallow every error into
			// an exit-0 RunE, surfaced on stderr only (never stdout).
			payload, err := resolvePayload(payloadInline, payloadFile, os.Stdin)
			if err == nil {
				err = runHook(args[0], segment, provider, payload, "unix://")
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "crowbar hook %s: %v\n", args[0], err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&segment, "segment", "", "Crowbar segment id")
	cmd.Flags().StringVar(&provider, "provider", "", "provider id")
	cmd.Flags().StringVar(&payloadFile, "payload-file", "", "read the payload from this file instead of stdin")
	cmd.Flags().StringVar(&payloadInline, "payload", "", "inline payload instead of stdin")
	return cmd
}

// resolvePayload selects the payload source: inline > file > stdin.
func resolvePayload(inline, file string, stdin io.Reader) ([]byte, error) {
	switch {
	case inline != "":
		return []byte(inline), nil
	case file != "":
		return os.ReadFile(file) //nolint:gosec // path is authored in the descriptor's own hook command
	default:
		return io.ReadAll(io.LimitReader(stdin, 8<<20))
	}
}

// runHook forwards a raw hook payload verbatim to the daemon, which holds the
// descriptor and parses it per the provider's declared format.
func runHook(event, segment, provider string, payload []byte, host string) error {
	client, err := ipc.NewClient(host)
	if err != nil {
		return err
	}
	body := map[string]any{
		"segment_id":  segment,
		"provider":    provider,
		"event":       event,
		"payload_raw": string(payload),
	}
	_, _, err = client.PostJSON(context.Background(), "/v0/agent/hooks", body)
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./cmd/crowbar/ -run 'TestRunHook_ForwardsSegmentProviderAndRawPayload|TestResolvePayload_Precedence'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/cmd/crowbar/hook.go api/cmd/crowbar/hook_test.go
git commit -m "refactor(cmd): crowbar hook forwards raw payload with --segment/--provider (drop CROWBAR_SEGMENT_ID)"
```

---

### Task 6: Usecase — hook-derived turns, new `IngestHook` signature, arg-based spawn attribution

**Files:**
- Modify: `api/internal/app/usecases/agent/agent.go`
- Test: `api/internal/app/usecases/agent/agent_test.go` (and any sibling `*_test.go` in the package that calls `IngestHook` — `switch_test.go`, `handoff_test.go`, `race_test.go`)

**Interfaces:**
- Consumes: `engineagent.TemplateCtx{Segid,Provider}` (T1); `ParsePayload`/`MapHook`/`CanonicalEvent.Message` (T2); `ledger.AppendTurn`/`RenderConversation` (T3); `domain.AgentSegment` without `TranscriptPath` (T4).
- Produces: `(*Usecase).IngestHook(ctx, crowbarSegID, provider, canonicalEvent string, rawPayload []byte) error`.

- [ ] **Step 1: Change the `IngestHook` signature and switch**

Replace `IngestHook` (currently taking `payload map[string]any`) with:

```go
// IngestHook maps an incoming vendor hook to a canonical event, runs the
// context-move reducer on session_start, and appends a conversation turn to the
// chat's ledger on user_prompt / turn_stop. An unknown crowbarSegID (no active
// segment) or a malformed payload is ignored, never an error — a hook must
// never break the vendor CLI's turn.
func (u *Usecase) IngestHook(
	ctx context.Context,
	crowbarSegID string,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	u.segMu.Lock(crowbarSegID)
	defer u.segMu.Unlock(crowbarSegID)

	seg, err := u.repo.GetActiveSegmentByCrowbarID(ctx, crowbarSegID)
	if errors.Is(err, agentchat.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("agent: ingest hook: active segment: %w", err)
	}

	chat, err := u.repo.GetChat(ctx, seg.ChatID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: chat: %w", err)
	}

	crowbarHome, projectID, repoID, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: worktree dir: %w", err)
	}

	// The active segment is the source of truth for which provider spawned this
	// CLI. The hook's self-reported provider is only a guard against a
	// mis-authored descriptor.
	if provider != "" && provider != seg.ProviderID {
		slog.WarnContext(ctx, "agent: ingest hook: provider mismatch",
			"hook_provider", provider, "segment_provider", seg.ProviderID, "segment_id", crowbarSegID)
	}

	descriptor, err := engineagent.ResolveDescriptor(crowbarHome, seg.ProviderID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: resolve descriptor: %w", err)
	}

	payload, err := descriptor.ParsePayload(rawPayload)
	if err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: parse payload", "err", err, "segment_id", crowbarSegID)
		return nil
	}

	ev, _ := descriptor.MapHook(canonicalEvent, payload)

	switch ev.Kind {
	case "session_start":
		return u.handleSessionStart(ctx, crowbarSegID, seg, chat, ev)
	case "user_prompt":
		return u.appendTurn(ctx, seg, chat, crowbarHome, projectID, repoID, "user", ev.Message)
	case "turn_stop":
		return u.appendTurn(ctx, seg, chat, crowbarHome, projectID, repoID, "assistant", ev.Message)
	}
	return nil
}
```

- [ ] **Step 2: Replace `handleTurnStop` with `appendTurn`**

Delete `handleTurnStop` (the `os.ReadFile(ev.Transcript)` version) and add:

```go
// appendTurn records one conversation turn (user or assistant) into the chat's
// ledger and broadcasts the lifecycle event. Empty text is a no-op.
func (u *Usecase) appendTurn(
	ctx context.Context,
	seg domain.AgentSegment,
	chat domain.AgentChat,
	crowbarHome, projectID, repoID string,
	role, text string,
) error {
	if text == "" {
		return nil
	}
	dir := worktreepath.AgentLedgerDir(crowbarHome, projectID, repoID, chat.WorkspaceID, chat.ID)
	led, err := ledger.Open(dir)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: ledger open: %w", err)
	}
	if _, err := led.AppendTurn(role, seg.ProviderID, time.Now(), text); err != nil {
		return fmt.Errorf("agent: ingest hook: ledger append: %w", err)
	}
	kind := "turn_stopped"
	if role == "user" {
		kind = "user_prompt"
	}
	u.bc.BroadcastAgentChat(chat.ID, kind)
	return nil
}
```

- [ ] **Step 3: Drop `TranscriptPath` writes and switch handoff to `RenderConversation`**

- In `persistBound`, delete the line `seg.TranscriptPath = ev.Transcript`.
- In `persistRegistered`, delete the field `TranscriptPath: ev.Transcript,` from the `newSeg` literal.
- In `AssembleHandoff`, change `blob, err := led.ReadAll()` to `blob, err := led.RenderConversation()`.

- [ ] **Step 4: Arg-based spawn attribution**

In `spawnSegment`, set the new template fields and stop injecting the env var:

```go
	tctx := engineagent.TemplateCtx{
		Tmp:         tmpDir,
		Cwd:         worktree,
		CrowbarHook: u.crowbarHookPath(crowbarHome),
		Handoff:     handoff,
		Segid:       segID,
		Provider:    providerID,
	}
```

and replace:

```go
	argv := append([]string{descriptor.Spawn.Cmd}, plan.Argv...)
	env := append(plan.Env, "CROWBAR_SEGMENT_ID="+segID)

	termSessID, err := u.term.CreateCommand(ctx, chat.WorkspaceID, worktree, argv, env,
		func() { _ = os.RemoveAll(tmpDir) })
```

with:

```go
	argv := append([]string{descriptor.Spawn.Cmd}, plan.Argv...)

	termSessID, err := u.term.CreateCommand(ctx, chat.WorkspaceID, worktree, argv, plan.Env,
		func() { _ = os.RemoveAll(tmpDir) })
```

Confirm the `os` import is still used (it is: `os.MkdirAll`, `os.Environ`, `os.RemoveAll`, `os.Getenv`). The `slog` import is already present.

- [ ] **Step 5: Update the package's tests to the new signature and behavior**

Mechanical signature change — every `IngestHook` call in the package's test files becomes: insert the provider argument after the segment id, and pass the payload as raw JSON bytes instead of a `map[string]any`. Add this helper (once, in `agent_test.go`):

```go
func mustJSON(t *testing.T, m map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}
```

Transform rule (apply to `agent_test.go`, `switch_test.go`, `handoff_test.go`, `race_test.go`):
`u.IngestHook(ctx, seg, "session_start", map[string]any{...})`
→ `u.IngestHook(ctx, seg, "claude", "session_start", mustJSON(t, map[string]any{...}))`
(use the provider that spawned the segment under test — `"claude"` or `"codex"`).

Behavioral test replacements in `agent_test.go`:

- **Delete** every assertion of the form `assert.Equal(t, "/tmp/....jsonl", <seg>.TranscriptPath)` and any `session_start` payload key `"transcript_path"`.
- **Replace the "turn_stop persists a transcript" test** with a ledger assertion. After driving a `turn_stop`, assert the handoff renders the turn:

```go
func TestIngestHook_TurnStopAppendsAssistantTurn(t *testing.T) {
	u, deps := newTestUsecase(t) // existing helper that wires repo+fakes+temp home
	chatID, segID := mustSpawn(t, u, "claude")

	err := u.IngestHook(context.Background(), segID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"session_id": "s1", "last_assistant_message": "done thing"}))
	require.NoError(t, err)

	handoff, err := u.AssembleHandoff(context.Background(), chatID)
	require.NoError(t, err)
	require.Contains(t, handoff, "assistant (claude): done thing")
}

func TestIngestHook_UserPromptAppendsUserTurn(t *testing.T) {
	u, _ := newTestUsecase(t)
	chatID, segID := mustSpawn(t, u, "claude")

	err := u.IngestHook(context.Background(), segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "please do the thing"}))
	require.NoError(t, err)

	handoff, err := u.AssembleHandoff(context.Background(), chatID)
	require.NoError(t, err)
	require.Contains(t, handoff, "user: please do the thing")
}
```

> If `newTestUsecase`/`mustSpawn`/`newActive` helpers differ in name in the existing file, reuse whatever the file already uses to construct the usecase and spawn a chat — do not invent new fixtures; only the `IngestHook` argument shape and the assertion target (ledger/handoff instead of `TranscriptPath`) change.

- **Replace the spawn-env attribution test.** The old assertion `assert.Contains(t, call.env, "CROWBAR_SEGMENT_ID="+segID)` becomes: the segment id and provider now live in the written hook-config, not the env. Add a helper and rewrite:

```go
func argAfter(argv []string, flag string) string {
	for i, a := range argv {
		if a == flag && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	return ""
}

func TestSpawn_HookConfigCarriesSegmentAndProvider(t *testing.T) {
	u, deps := newTestUsecase(t)
	_, segID := mustSpawn(t, u, "claude")

	call := deps.commander.calls[0]
	settingsPath := argAfter(call.argv, "--settings")
	require.NotEmpty(t, settingsPath)
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "--segment "+segID+" --provider claude")

	for _, kv := range call.env {
		require.False(t, strings.HasPrefix(kv, "CROWBAR_SEGMENT_ID="))
	}
}
```

Add `"strings"` and `"encoding/json"` to the test imports if not already present.

- [ ] **Step 6: Run the usecase package tests**

Run: `cd api && go test ./internal/app/usecases/agent/...`
Expected: PASS. (The API handler, an importer, won't build yet — that's Task 7. Package-scoped `go test` compiles only this package + its imports.)

- [ ] **Step 7: Commit**

```bash
git add api/internal/app/usecases/agent/
git commit -m "refactor(agent-usecase): build conversation from hooks, arg-based attribution, drop transcript reads"
```

---

### Task 7: API handler — bind `payload_raw`/`provider`, call new `IngestHook`

**Files:**
- Modify: `api/internal/api/v0/endpoints/agent/handlers/hooks.go`
- Test: `api/internal/api/v0/endpoints/agent/handlers/hooks_test.go` (and `routes_test.go` if it posts hook bodies)

**Interfaces:**
- Consumes: `(*Usecase).IngestHook(ctx, segID, provider, event string, rawPayload []byte) error` (T6).

- [ ] **Step 1: Update the handler**

Replace the body binding and call in `hooks.go`:

```go
	var body struct {
		SegmentID  string `json:"segment_id"`
		Provider   string `json:"provider"`
		Event      string `json:"event"`
		PayloadRaw string `json:"payload_raw"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.usecase.IngestHook(rctx, body.SegmentID, body.Provider, body.Event, []byte(body.PayloadRaw)); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteAccepted(ctx)
```

- [ ] **Step 2: Update the handler tests**

In `hooks_test.go` (and `routes_test.go` if applicable), change every posted JSON body from `{"segment_id","event","payload":{...}}` to `{"segment_id","provider","event","payload_raw":"<json-string>"}`. Example replacement for the happy-path test body:

```go
	body := `{"segment_id":"seg-1","provider":"claude","event":"turn_stop","payload_raw":"{\"session_id\":\"s\",\"last_assistant_message\":\"hi\"}"}`
```

If a test uses a fake/stub usecase implementing `IngestHook`, update that stub's method signature to `IngestHook(ctx context.Context, segID, provider, event string, raw []byte) error` and assert on the new args (e.g. `provider == "claude"`, `string(raw)` contains the payload).

- [ ] **Step 3: Run the endpoint tests**

Run: `cd api && go test ./internal/api/v0/endpoints/agent/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add api/internal/api/v0/endpoints/agent/
git commit -m "refactor(api): agent hooks handler forwards raw payload + provider to IngestHook"
```

---

### Task 8: Integration rewire + full build + full suite

**Files:**
- Modify: `api/tests/integration/agent/agent_gaps_test.go`
- Verify: whole module builds and both suites pass.

**Interfaces:**
- Consumes: everything from Tasks 1–7.

- [ ] **Step 1: Full build to find every remaining break**

Run: `cd api && go build ./... 2>&1 | head -50`
Expected initially: compile errors only in `tests/integration/agent/agent_gaps_test.go` (and nowhere else). Any error outside that file means an earlier task missed a call site — fix it in that task's package before continuing.

- [ ] **Step 2: Rewire `agent_gaps_test.go` verification from transcript to ledger**

The cross-provider handoff tests drive real `claude`/`codex` PTYs; their hooks reach the daemon through `crowbar hook` → the HTTP handler, so the ingestion path itself needs no test change. Only the **assertions and helpers** change:

- **Delete** every `require.NotEmpty(t, <seg>.TranscriptPath, ...)` and every `require.Equal(t, <a>.TranscriptPath, <b>.TranscriptPath, ...)`.
- **Delete** the transcript-reading helpers used to assert Crowbar behavior (`codexLastAssistantText`, `claudeAssistantTexts`) **where they back a `TranscriptPath` assertion**. If a test still wants to peek at a vendor transcript purely as an *oracle* (to confirm the injected handoff reached the next CLI), that is allowed — but it reads a path it discovers itself (e.g. from the CLI's own SessionStart hook captured in-test), never from `seg.TranscriptPath`.
- **Replace** "the handoff carried the prior turn" checks with a ledger/handoff assertion:

```go
	handoff, err := uc.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	require.Contains(t, handoff, "OTTER-3304") // the codeword the prior turn established
```

- **Replace** the native-`--resume` continuity check (was transcript-path equality) with session-id continuity:

```go
	require.Equal(t, origClaudeSeg.ProviderSessionID, resumedSeg.ProviderSessionID,
		"a native --resume must reattach to the SAME provider session as before the switch")
```

- [ ] **Step 3: Build the integration package**

Run: `cd api && go build -tags integration ./tests/... && go vet ./...`
Expected: success. (If `go vet` flags a pre-existing unrelated issue such as the known `waitForOutput redeclared` under `-tags integration`, note it but do not expand scope; it predates this work.)

- [ ] **Step 4: Run the full unit suite (race)**

Run: `cd api && go test -race ./...`
Expected: PASS across all packages.

- [ ] **Step 5: Run the integration suite**

Run: `cd api && go test -tags integration ./tests/integration/agent/... -v 2>&1 | tail -40`
Expected: PASS (requires real `claude`/`codex` binaries on PATH, as before). If the binaries are unavailable in the run environment, record that the suite was skipped/blocked rather than claiming a pass.

- [ ] **Step 6: Lint**

Run: `cd api && golangci-lint run ./... 2>&1 | tail -30`
Expected: clean (no new findings; unused imports from deleted code would surface here).

- [ ] **Step 7: Commit**

```bash
git add api/tests/integration/agent/agent_gaps_test.go
git commit -m "test(agent): verify cross-provider handoff via ledger/handoff, not vendor transcripts"
```

- [ ] **Step 8: Live verification (per project rule — tests are not a live proof)**

Rebuild the sidecar and drive a real Claude↔Codex switch in the running app via `make dev-desktop` (isolated `CROWBAR_HOME`, never the production instance). Confirm: (a) a fresh chat records user+assistant turns as they happen; (b) a provider switch hands the target CLI a clean conversation (user/assistant turns, no tool-call noise); (c) the target CLI demonstrably read the handoff (it can answer about a fact from before the switch). Record what was observed.

---

## Self-Review

**1. Spec coverage** (each spec section → task):
- §3 descriptor format (write+read) → Tasks 2 (types/YAML), 1 (vars).
- §4 round trip → exercised end-to-end in Tasks 5–8.
- §5 `crowbar hook` raw forwarder → Task 5.
- §6 read-side extraction (`HookSpec`, `ParsePayload`, `MapHook`, dotted `extract`, `Validate`) → Task 2.
- §7 conversation ledger (`AppendTurn`/`RenderConversation`) + usecase rewiring (`user_prompt`, `appendTurn`, drop `TranscriptPath`, handoff render) → Tasks 3, 6.
- §8 spawn rewiring (`render_hooks` deleted, `{segid}`/`{provider}`, drop `CROWBAR_SEGMENT_ID`) → Tasks 1, 2, 6.
- §9 compatibility contract → documented in spec; `ParsePayload` unknown-format boundary → Task 2 test.
- §10 testing (unit + integration + live) → Tasks 1–8 test steps + Task 8 §8.
- §11 out of scope → nothing built (no storage reorg, no `run` verb, no multi-format).
- §12 flagged decisions → all three implemented as recommended (raw-forward T5; `--provider` guard T6; no `via` key — delivery via `crowbar hook` flags T5).

**2. Placeholder scan:** no "TBD"/"handle errors"/"similar to". The one soft reference (Task 6 fixture-helper names) is bounded with an explicit instruction to reuse the file's existing fixtures and change only the argument shape + assertion target.

**3. Type consistency:** `IngestHook(ctx, crowbarSegID, provider, canonicalEvent string, rawPayload []byte)` identical across Tasks 6 (def) and 7 (call). `AppendTurn(role, provider string, at time.Time, text string)` / `RenderConversation()` identical across Tasks 3 (def) and 6 (call). `HookSpec.Events map[string]map[string]string` / `MapHook`/`ParsePayload`/`CanonicalEvent.Message` consistent across Tasks 2 (def) and 6 (use). `TemplateCtx{Segid,Provider}` consistent across Tasks 1 (def), 2 (test), 6 (use).
