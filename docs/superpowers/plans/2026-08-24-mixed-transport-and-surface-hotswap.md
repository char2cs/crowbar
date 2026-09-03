# Mixed Transport & Surface Hotswap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give codex the same interactive core claude already has — streaming, answerable permissions, telemetry, interrupt, compact — by running it over the `app-server` API for turns while keeping hooks for the five events the API doesn't carry, and let the descriptor declare whether a provider's terminal and chat surfaces can be live at the same instant so the frontend's dead `handoverBlocked` prop and dev-only split gate become real, declared facts.

**Architecture:** Two capabilities sharing one seam (the codex descriptor). Capability 2 (surface hotswap) is a small, independent, wire-and-frontend-only change: a declared `hotswap` bool plus a derived `hasTerminal` bool, both defaulting safely, feeding the already-built `ViewSwitcher`/`SelectionCluster` gating. Capability 1 (mixed transport) is new engine surface: a WebSocket-framed JSON-RPC2 client dials the socket codex's `app-server` listens on, a reverse dispatcher turns each inbound frame's wire method (plus `when:` discriminators for sum types) into the same canonical event names the hooks path already produces, and both wires feed the *same* `Turns.IngestHook` entrypoint keyed by the runner's own ID — so ingestion, activity, and the answer desk need no changes at all. The PTY still runs `attach`, so the terminal surface is unchanged; only the "who tells Crowbar what happened" wire is now plural.

**Tech Stack:** Go 1.x, `gorilla/websocket` (already a dependency), the existing `spec`/`descriptor`/`protocol`/`mapping` packages under `api/internal/engine/agents`, React/TypeScript on the frontend (`web/src/features/agent`).

**Spec:** `docs/superpowers/specs/2026-08-24-mixed-transport-and-surface-hotswap-design.md` — read it alongside this plan; §1 explains what already exists, §2/§3 are the two capabilities, §4 is the order of work this plan follows, §5/§6 are verification and risk.

## Corrections this plan makes to the spec

Live probing against the real `codex-cli 0.146.0` binary (§6 of the spec explicitly asks for this — "Verify `attach` early") surfaced two facts the spec did not have, because the fixture capture script only ever exercised `--listen stdio://`:

1. **`unix://` is WebSocket-framed, not raw NDJSON.** `codex app-server --listen unix://PATH` runs an HTTP-Upgrade WebSocket handshake over the socket (`tungstenite`); a client that writes bare `{"jsonrpc":...}\n` bytes gets `httparse error: invalid token` and the connection is closed with no response. `stdio://` (what the fixture script used) is plain NDJSON. The `map:` paths in `codex-api.yaml` are unaffected — verified byte-for-byte against a live turn below — but the client **must** speak WebSocket, and must not offer the `permessage-deflate` extension (`gorilla/websocket`'s zero-value `Dialer{}` already offers none, which is what makes Task 6 work with no special configuration).
2. **A live turn over the socket works end-to-end**, and — with the user's own `~/.codex/hooks.json` present — **hooks and the API fired concurrently on the same thread**, which is a live existence proof of "mixed transport" rather than a hypothesis. `thread/tokenUsage/updated`'s shape matched `codex-api.yaml`'s `tokenUsage.total.*` paths exactly.
3. **`codex --remote unix://PATH` requires a real TTY** (`Error: stdin is not a terminal` under a piped/null stdin) and renders a screen once given a PTY, but headless probing could not confirm which thread it attaches to — there is no `--thread` flag on `codex --remote`, and neither the descriptor's `attach:` nor codex's own CLI surface names one. **This plan's live-verification task (Task 15) must confirm empirically whether `--remote` attaches to the thread `serve` already opened, or opens its own** — if it opens its own, capability 1's "one thread, two wires" claim needs a follow-up (very likely a `--thread <id>` equivalent surfaces in a future codex build, since the whole feature is `[experimental]`); do not assume either answer.

Nothing else in the spec needs correction. The event table (§1.4), the `map:`/`send:`/`reply:` paths in `codex-api.yaml`, the answer-desk transport-agnosticism this plan leans on hard (see Task 10), and the three-state hotswap model (§3.1) all held up against the live probe and the code read for this plan.

## Global Constraints

- Provider/transport code lives under `api/internal/engine/agents/internal/...` (Go `internal/` visibility) — nothing outside that tree may import the new packages directly; callers go through `api/internal/engine/agents/internal/protocol/protocol.go`, the existing single façade.
- Test files mirror source under `web/src/__tests__/...` per this repo's CLAUDE.md. Go tests are co-located (`_test.go` beside the file), matching this codebase's existing Go convention (do not invent a mirrored Go test tree).
- Every new capability boolean on the wire defaults in the **safe direction for what it gates**, not uniformly `false` — Task 3 spells out why `hasTerminal` and `hotswap` default differently, and it is load-bearing; do not "fix" it to match the other capability keys.
- `descriptors-v3/experimental/codex-api.yaml` and its fixtures at `api/internal/engine/agents/internal/protocol/testdata/fixtures/codex-api/` are deleted only in Task 14, after the merged `codex.yaml` is proven against those same fixtures — never delete the experimental file before that proof exists.
- Live verification (Tasks 7 and 15) runs in the real desktop app via the Tauri MCP bridge against the dev daemon under the worktree's isolated `CROWBAR_HOME` — never the production socket, never a headless-only check standing in for it.
- `go build ./...`, `go vet ./...`, `golangci-lint run`, the full `bun test` web suite, and `bun tsc` must stay green after every task — run them at the end of each task, not just at the end of the plan.

---

## Part A — Capability 2: Surface Hotswap

Independent of Part B and ships first, per spec §4 step 2. Both shipped providers declare `hotswap: true` today (§3.5), so this part's live verification only exercises the permissive branch; the negative branch (Task 6) is proven with a fixture descriptor, per spec §5's explicit warning that a capability with no real `false` provider is untested if only ever exercised on its permissive value.

### Task 1: `RuntimeSpec.Hotswap` and derived `HasTerminal`/`Hotswap` capabilities

**Files:**
- Modify: `api/internal/engine/agents/internal/spec/v3.go:19-25` (`RuntimeSpec`)
- Modify: `api/internal/engine/agents/internal/models/spawn.go:17-38` (`Capabilities`)
- Modify: `api/internal/engine/agents/agents.go:139-155` (`Capabilities()`)
- Test: `api/internal/engine/agents/agents_test.go`
- Test: `api/internal/engine/agents/internal/protocol/internal/descriptor/v3_test.go`

**Interfaces:**
- Consumes: `spec.Descriptor.Runtime` (existing), `spec.APISpec.Attach` (existing, `[]string`).
- Produces: `models.Capabilities.Hotswap bool`, `models.Capabilities.HasTerminal bool` — read by Task 2's provider assembly as `caps.Hotswap`, `caps.HasTerminal`.

- [ ] **Step 1: Write the failing test for the derivation**

```go
// api/internal/engine/agents/agents_test.go — add near the other Capabilities() tests
func TestCapabilities_HasTerminalIsStructuralNotDeclared(t *testing.T) {
	hooksProvider := &agent{spec: &spec.Descriptor{
		ID:      "x",
		Runtime: spec.RuntimeSpec{Transport: "hooks", Spawn: spec.SpawnSpec{Cmd: "x"}},
	}}
	assert.True(t, hooksProvider.Capabilities().HasTerminal,
		"a hooks-transport provider's PTY IS its terminal")

	apiNoAttach := &agent{spec: &spec.Descriptor{
		ID:      "x",
		Runtime: spec.RuntimeSpec{Transport: "api", API: spec.APISpec{Serve: []string{"x"}}},
	}}
	assert.False(t, apiNoAttach.Capabilities().HasTerminal,
		"served with no attach: there is no terminal, not a disabled one")

	apiWithAttach := &agent{spec: &spec.Descriptor{
		ID: "x",
		Runtime: spec.RuntimeSpec{
			Transport: "api",
			API:       spec.APISpec{Serve: []string{"x"}, Attach: []string{"x", "--remote"}},
		},
	}}
	assert.True(t, apiWithAttach.Capabilities().HasTerminal)
}

func TestCapabilities_HotswapDefaultsFalse(t *testing.T) {
	undeclared := &agent{spec: &spec.Descriptor{ID: "x", Runtime: spec.RuntimeSpec{Transport: "hooks"}}}
	assert.False(t, undeclared.Capabilities().Hotswap,
		"a descriptor that has not thought about hotswap gets the conservative answer")

	declared := &agent{spec: &spec.Descriptor{
		ID:      "x",
		Runtime: spec.RuntimeSpec{Transport: "hooks", Hotswap: true},
	}}
	assert.True(t, declared.Capabilities().Hotswap)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/engine/agents/... -run TestCapabilities_HasTerminal -run TestCapabilities_Hotswap -v`
Expected: FAIL — `spec.RuntimeSpec` has no field `Hotswap`, `models.Capabilities` has no field `HasTerminal`/`Hotswap` (compile error).

- [ ] **Step 3: Add the field and derive the capabilities**

In `api/internal/engine/agents/internal/spec/v3.go`, add to `RuntimeSpec`:

```go
type RuntimeSpec struct {
	// Transport is the default for every event that declares none: hooks | api | oneshot.
	Transport string    `yaml:"transport"`
	API       APISpec   `yaml:"api"`
	Hooks     HooksWire `yaml:"hooks"`
	Spawn     SpawnSpec `yaml:"spawn"`

	// Hotswap is whether this provider's TWO faces — Crowbar's chat and the
	// provider's own terminal — can be live at the same instant, so a live turn
	// can hand over mid-flight instead of blocking the switch until it ends.
	// Defaults false on absence: a descriptor that has not thought about this
	// gets the conservative answer, matching every other capability key.
	Hotswap bool `yaml:"hotswap"`
}
```

In `api/internal/engine/agents/internal/models/spawn.go`, add to `Capabilities`:

```go
	// Hotswap is RuntimeSpec.Hotswap, carried through unchanged: whether both of
	// this provider's faces can be live at once.
	Hotswap bool

	// HasTerminal is STRUCTURAL, never declared: true when this descriptor's spawn
	// plan produces a real interactive PTY the user can look at. A hooks-transport
	// provider's PTY IS the vendor CLI, so it always has one; an api-transport
	// provider has one only if it declares `attach` (§3.2 of the design spec —
	// existence must be derived from what Crowbar was told to run, not declared,
	// because a separate boolean could contradict it).
	HasTerminal bool
```

In `api/internal/engine/agents/agents.go`, extend `Capabilities()`:

```go
func (a *agent) Capabilities() Capabilities {
	caps := Capabilities{
		SlashCatalog: a.spec.Presentation.SlashCatalog != nil,
		Telemetry:    a.spec.Telemetry != nil,
		ModelSelect:  a.spec.Model != nil,
		EffortSelect: a.spec.Effort != nil,

		TerminalPrompts: protocol.TerminalPrompts(a.spec),
		Compaction:      protocol.CanSend(a.spec, "compact_start"),
		Observes:        protocol.Observes(a.spec),

		Hotswap:     a.spec.Runtime.Hotswap,
		HasTerminal: a.spec.Runtime.Transport != "api" || len(a.spec.Runtime.API.Attach) > 0,
	}
	if ps := a.spec.Presentation.PromptSubmit; ps != nil {
		caps.PromptSubmit = true
		caps.Delivery = ps.Strategy
	}
	return caps
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd api && go test ./internal/engine/agents/... -run TestCapabilities_HasTerminal -run TestCapabilities_Hotswap -v`
Expected: PASS.

- [ ] **Step 5: Run the full descriptor test suite to confirm no regression**

Run: `cd api && go test ./internal/engine/agents/... ./internal/engine/agents/internal/protocol/...`
Expected: PASS — `claude.yaml`/`codex.yaml` parse fine with the new field absent (defaults to `false`/structural-true as designed).

- [ ] **Step 6: Commit**

```bash
git add api/internal/engine/agents/internal/spec/v3.go api/internal/engine/agents/internal/models/spawn.go api/internal/engine/agents/agents.go api/internal/engine/agents/agents_test.go
git commit -m "feat(agents): declare hotswap and derive hasTerminal on Capabilities"
```

---

### Task 2: Wire capabilities through `domain.AgentProvider` and the DTO

**Files:**
- Modify: `api/internal/domain/agent_provider.go:22-53` (`AgentProvider` struct)
- Modify: `api/internal/app/usecases/chat/internal/provider/provider.go:107-146` (`ResolveProviders`)
- Modify: `api/internal/api/v0/dto/agent.go:513-533` (`AgentProviderDTO`)
- Modify: `api/internal/api/v0/endpoints/chat/handlers/providers.go:85-103` (`providerDTOs`)
- Test: `api/internal/app/usecases/chat/providers_test.go`
- Test: `api/internal/api/v0/endpoints/chat/handlers/providers_test.go`

**Interfaces:**
- Consumes: `engineagents.Capabilities.Hotswap`, `.HasTerminal` (Task 1).
- Produces: `domain.AgentProvider.Hotswap bool`, `.HasTerminal bool`; `dto.AgentProviderDTO.Hotswap bool` (`json:"hotswap"`), `.HasTerminal bool` (`json:"hasTerminal"`) — read by Task 3's frontend `AgentProvider` type.

- [ ] **Step 1: Write the failing handler test**

In `api/internal/api/v0/endpoints/chat/handlers/providers_test.go`, extend the existing table-driven assertion (find the test that asserts `Compaction`/`ModelSelect` on the JSON response) to also assert:

```go
	assert.Contains(t, string(body), `"hotswap"`)
	assert.Contains(t, string(body), `"hasTerminal"`)
```

Use the exact style already in that file (it decodes into `struct{ Data []dto.AgentProviderDTO }` — add `Hotswap bool` / `HasTerminal bool` fields with json tags to that local struct and assert them directly instead of a raw string-contains check, matching how the file already tests `Compaction`).

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/api/v0/endpoints/chat/handlers/... -run TestProviders -v`
Expected: FAIL — fields absent from the JSON.

- [ ] **Step 3: Thread the two booleans through**

`api/internal/domain/agent_provider.go` — add beside `Compaction`:

```go
	// Hotswap and HasTerminal are engine.Capabilities.Hotswap/.HasTerminal,
	// carried through unchanged — see there for what each means.
	Hotswap     bool
	HasTerminal bool
```

`api/internal/app/usecases/chat/internal/provider/provider.go` — in `ResolveProviders`, extend the `domain.AgentProvider{...}` literal:

```go
		out = append(out, domain.AgentProvider{
			ID:           d.ID(),
			DisplayName:  display.Name,
			Icon:         display.Icon,
			Connected:    p.installed(d),
			Enabled:      !pref.Disabled,
			MCPEnabled:   !pref.MCPDisabled,
			Compaction:   caps.Compaction,
			ModelSelect:  caps.ModelSelect,
			EffortSelect: caps.EffortSelect,
			Hotswap:      caps.Hotswap,
			HasTerminal:  caps.HasTerminal,
			Models:       d.Models(),
			Efforts:      resolveEfforts(d),
		})
```

`api/internal/api/v0/dto/agent.go` — add to `AgentProviderDTO`:

```go
	Hotswap     bool `json:"hotswap"`
	HasTerminal bool `json:"hasTerminal"`
```

`api/internal/api/v0/endpoints/chat/handlers/providers.go` — extend `providerDTOs`:

```go
		out = append(out, dto.AgentProviderDTO{
			ID:           p.ID,
			DisplayName:  p.DisplayName,
			Icon:         p.Icon,
			Connected:    p.Connected,
			Enabled:      p.Enabled,
			MCPEnabled:   p.MCPEnabled,
			Compaction:   p.Compaction,
			ModelSelect:  p.ModelSelect,
			EffortSelect: p.EffortSelect,
			Hotswap:      p.Hotswap,
			HasTerminal:  p.HasTerminal,
			Models:       p.Models,
			Efforts:      p.Efforts,
		})
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd api && go test ./internal/api/v0/endpoints/chat/handlers/... ./internal/app/usecases/chat/... -run TestProviders -v`
Expected: PASS.

- [ ] **Step 5: Run the package build and full relevant tests**

Run: `cd api && go build ./... && go test ./internal/domain/... ./internal/app/usecases/chat/... ./internal/api/v0/...`
Expected: PASS, no compile errors elsewhere referencing the old struct literals (Go struct literals here are keyed, so no positional-literal breakage is possible, but confirm).

- [ ] **Step 6: Commit**

```bash
git add api/internal/domain/agent_provider.go api/internal/app/usecases/chat/internal/provider/provider.go api/internal/api/v0/dto/agent.go api/internal/api/v0/endpoints/chat/handlers/providers.go api/internal/api/v0/endpoints/chat/handlers/providers_test.go
git commit -m "feat(agents): carry hotswap/hasTerminal through domain, DTO, and handler"
```

---

### Task 3: Declare `hotswap: true` on both shipped descriptors

**Files:**
- Modify: `api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/claude.yaml` (add to `runtime:` block)
- Modify: `api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/codex.yaml:13-20` (add to `runtime:` block)
- Test: `api/internal/engine/agents/internal/protocol/internal/descriptor/v3_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: both descriptors' `Capabilities().Hotswap == true`, consumed by Task 2's assembly (already wired) with no further code change.

- [ ] **Step 1: Write the failing test**

```go
// v3_test.go — add near existing "declared descriptor" assertions
func TestShippedDescriptors_DeclareHotswapTrue(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		d, err := descriptor.Resolve(context.Background(), "", id)
		require.NoError(t, err)
		assert.True(t, d.Runtime.Hotswap, "%s must declare hotswap — both keep the PTY "+
			"attached for the whole session with hooks reporting alongside (design spec §3.5)", id)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/engine/agents/internal/protocol/internal/descriptor/... -run TestShippedDescriptors_DeclareHotswapTrue -v`
Expected: FAIL — both default to `false`.

- [ ] **Step 3: Add the field to both YAML files**

In `claude.yaml`, under the existing `runtime:` key, add a sibling line (do not restructure the block):

```yaml
runtime:
  transport: hooks
  hotswap: true
  hooks:
    ...
```

In `codex.yaml:13-20`, same:

```yaml
runtime:
  transport: hooks
  hotswap: true
  hooks:
    format: json
    delivery: http
    require_payload_fields: [transcript_path]
  spawn:
    cmd: codex
```

(Exact indentation must match the surrounding block — 2 spaces under `runtime:`.)

- [ ] **Step 4: Run to verify it passes**

Run: `cd api && go test ./internal/engine/agents/internal/protocol/internal/descriptor/... -v`
Expected: PASS, including the fixture-replay and reply-template tests already in this package (unaffected by this change).

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/claude.yaml api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/codex.yaml api/internal/engine/agents/internal/protocol/internal/descriptor/v3_test.go
git commit -m "feat(descriptors): declare hotswap:true on claude and codex"
```

---

### Task 4: Fixture descriptor proving the `false` branch (negative-case test)

Per spec §5: "the negative case needs a fixture provider... otherwise the capability is only ever exercised on its permissive value and the gate is untested" ([[project_vacuous_guard_tests]]).

**Files:**
- Create: `api/internal/engine/agents/internal/protocol/internal/descriptor/testdata/fixture-descriptors/no-hotswap.yaml`
- Test: `api/internal/engine/agents/internal/protocol/internal/descriptor/v3_test.go`

**Interfaces:**
- Consumes: `descriptor.ParseV3` (existing, takes raw bytes — does not require the embedded FS, so a testdata file works without touching the embed glob).
- Produces: nothing new — this is a test-only fixture proving Task 1's derivation on the conservative branch.

- [ ] **Step 1: Write the fixture descriptor**

```yaml
# A minimal, VALID v3 descriptor that declares api transport with no attach — the
# "no terminal" case Capabilities.HasTerminal must report false for, and no hotswap
# key — the "not declared" case Hotswap must default false for. Never shipped;
# read only by the test below.
id: no-hotswap-fixture
display_name: No Hotswap Fixture
icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"></svg>'

runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [true]
    handshake: { call: initialize }
  spawn:
    cmd: "true"

events:
  session_start:
    in: thread/started
    map: { session_id: thread.id }
```

- [ ] **Step 2: Write the failing test**

```go
func TestFixtureDescriptor_NoHotswapNoAttachYieldsConservativeCapabilities(t *testing.T) {
	raw, err := os.ReadFile("testdata/fixture-descriptors/no-hotswap.yaml")
	require.NoError(t, err)
	d, err := descriptor.ParseV3(raw)
	require.NoError(t, err)

	a := agents.NewForTest(d) // see Step 3 — a tiny exported test constructor
	caps := a.Capabilities()
	assert.False(t, caps.Hotswap, "undeclared hotswap must default false")
	assert.False(t, caps.HasTerminal, "api transport with no attach has no terminal at all")
}
```

- [ ] **Step 3: Add the minimal test seam if `agent` is not already constructible from outside its package**

Check first: `agent` in `api/internal/engine/agents/agents.go` is unexported. If `agents_test.go` (same package, `package agents`) already has direct access, move this test there instead of the descriptor package — prefer that over adding a new exported constructor. Rewrite Step 2's test as:

```go
// api/internal/engine/agents/agents_test.go
func TestFixtureDescriptor_NoHotswapNoAttachYieldsConservativeCapabilities(t *testing.T) {
	raw, err := os.ReadFile("internal/protocol/internal/descriptor/testdata/fixture-descriptors/no-hotswap.yaml")
	require.NoError(t, err)
	d, err := descriptorpkg.ParseV3(raw) // import descriptor package's ParseV3 directly
	require.NoError(t, err)

	a := &agent{spec: d}
	caps := a.Capabilities()
	assert.False(t, caps.Hotswap)
	assert.False(t, caps.HasTerminal)
}
```

(This package already imports nothing from `descriptor` today — add
`descriptorpkg "github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"`
to `agents_test.go`'s import block; it is legal since `agents_test.go` lives under the
same `api/internal/...` tree.)

- [ ] **Step 4: Run to verify it fails, then passes**

Run: `cd api && go test ./internal/engine/agents/... -run TestFixtureDescriptor_NoHotswap -v`
Expected: FAIL before Task 1/3 lands in a fresh checkout (it won't be — Tasks 1-3 are already committed by this point), so expected here is a straight PASS once the fixture file exists, proving the derivation genuinely branches both ways.

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/agents/internal/protocol/internal/descriptor/testdata/fixture-descriptors/no-hotswap.yaml api/internal/engine/agents/agents_test.go
git commit -m "test(agents): prove hotswap/hasTerminal on their conservative branch"
```

---

### Task 5: Frontend `AgentProvider` type and `mapProvider` defaults

**Files:**
- Modify: `web/src/features/agent/api/agent-api.ts:209-234` (`AgentProvider` interface), `:686-705` (`mapProvider`)
- Test: `web/src/__tests__/features/agent/api/agent-api.test.ts`

**Interfaces:**
- Consumes: wire fields `hotswap?: boolean`, `hasTerminal?: boolean` (Task 2).
- Produces: `AgentProvider.hotswap: boolean`, `.hasTerminal: boolean` (both always-present after mapping) — consumed by Task 6.

- [ ] **Step 1: Write the failing test**

In `web/src/__tests__/features/agent/api/agent-api.test.ts`, find the existing `mapProvider`/`listProviders` defaulting test (the one asserting `compaction` defaults to `false` when omitted) and add a sibling case:

```ts
it('defaults hasTerminal to true and hotswap to false when the daemon omits them', async () => {
  server.use(
    http.get('*/chats/providers', () =>
      HttpResponse.json({
        data: [{ id: 'claude', displayName: 'Claude', icon: '<svg/>', connected: true, enabled: true }],
      }),
    ),
  )
  const providers = await listProviders('ws-1')
  expect(providers[0].hasTerminal).toBe(true)
  expect(providers[0].hotswap).toBe(false)
})

it('carries hasTerminal:false and hotswap:true through unchanged', async () => {
  server.use(
    http.get('*/chats/providers', () =>
      HttpResponse.json({
        data: [{
          id: 'claude', displayName: 'Claude', icon: '<svg/>', connected: true, enabled: true,
          hasTerminal: false, hotswap: true,
        }],
      }),
    ),
  )
  const providers = await listProviders('ws-1')
  expect(providers[0].hasTerminal).toBe(false)
  expect(providers[0].hotswap).toBe(true)
})
```

(Match the exact MSW `server.use`/`http.get` pattern already used elsewhere in this file — the two snippets above follow the shape of the existing `compaction` defaulting test; copy its imports and setup verbatim rather than introducing a new mocking style.)

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && bun test src/__tests__/features/agent/api/agent-api.test.ts`
Expected: FAIL — TypeScript compile error (`hasTerminal`/`hotswap` do not exist on `AgentProvider`) or the fields are simply `undefined` at runtime.

- [ ] **Step 3: Add the fields**

In `web/src/features/agent/api/agent-api.ts`, extend `AgentProvider` (after the `effortSelect?` field, before `models`):

```ts
  /**
   * Whether this provider's terminal surface EXISTS AT ALL — structural, not a
   * capability the descriptor opts into. Defaults to `true` on omission: every
   * shipped provider today spawns a real PTY, so an OLDER daemon that predates
   * this field is describing exactly that reality, and defaulting `false` would
   * hide the view switcher for every existing install until the daemon catches
   * up. This is the opposite direction from every OTHER capability key on this
   * type, deliberately: those gate a control that does not exist yet, and
   * defaulting them on hides nothing that was already there.
   */
  hasTerminal?: boolean
  /**
   * Whether this provider's chat and terminal faces can be live at the same
   * instant. Defaults to `false` on omission, the same direction as
   * modelSelect/effortSelect/compaction: an older daemon or an undeclared
   * descriptor gets the conservative answer, and the user is asked to finish
   * the turn rather than being handed a swap nobody verified.
   */
  hotswap?: boolean
```

In `mapProvider` (around line 701, right after the `compaction` line):

```ts
    compaction: p.compaction ?? false,
    hasTerminal: p.hasTerminal ?? true,
    hotswap: p.hotswap ?? false,
    models: p.models ?? [],
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && bun test src/__tests__/features/agent/api/agent-api.test.ts`
Expected: PASS.

- [ ] **Step 5: Typecheck**

Run: `cd web && bun tsc --noEmit`
Expected: no new errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/agent/api/agent-api.ts web/src/__tests__/features/agent/api/agent-api.test.ts
git commit -m "feat(agent-api): map hasTerminal/hotswap with their correct defaults"
```

---

### Task 6: Wire `handoverBlocked`, hide the switcher, gate split on `hotswap`

**Files:**
- Modify: `web/src/features/agent/chat/agent-chat-view.tsx:361`, `:486` (`showSwitcher` computation)
- Modify: `web/src/features/agent/components/agent-chat-pane.tsx` (pass `handoverBlocked`)
- Modify: `web/src/features/agent/controls/view-switcher.tsx` — no change needed; `handoverBlocked` and hiding are both already-built props, confirm only.
- Test: `web/src/__tests__/features/agent/chat/agent-chat-view.test.tsx`
- Test: `web/src/__tests__/features/agent/components/agent-chat-pane.test.tsx`

**Interfaces:**
- Consumes: `AgentProvider.hasTerminal`, `.hotswap` (Task 5); `working` (already computed in `agent-chat-pane.tsx:191` from `s.agentChats.working[shownChatId]`, which — per `api/internal/domain/chat.go:76` (`Working = CurrentTurnStarted != nil || AsyncWork > 0`) — stays `true` through a permission wait, since that clears only on `turn_stop`/`turn_failed`, never on a `HookPermission`/`HookElicitation` observation. This is why `working` is the correct "a turn is open" signal for this gate with **no new wire field**: verified by reading `handleObservation` (`api/internal/app/usecases/chat/internal/turn/observation.go`), which never touches `CurrentTurnStarted`.
- Produces: `ViewSwitcher` receiving a real `handoverBlocked`; hidden entirely when `!provider.hasTerminal`; split only offered when `provider.hotswap` (in addition to the existing dev-flag gate, which stays as a second, independent gate per spec §3.4: "capability says *may*, the setting says *shown*").

- [ ] **Step 1: Write the failing tests**

In `web/src/__tests__/features/agent/chat/agent-chat-view.test.tsx`, add:

```tsx
it('hides the view switcher entirely when the provider has no terminal', () => {
  renderAgentChatView({ provider: { ...baseProvider, hasTerminal: false } })
  expect(screen.queryByRole('tablist', { name: 'View' })).not.toBeInTheDocument()
})

it('blocks handover to the terminal mid-turn when the provider cannot hotswap', () => {
  renderAgentChatView({ provider: { ...baseProvider, hotswap: false }, working: true })
  const terminalTab = screen.getByRole('tab', { name: 'Terminal' })
  expect(terminalTab).toBeDisabled()
})

it('never blocks handover when the provider can hotswap, even mid-turn', () => {
  renderAgentChatView({ provider: { ...baseProvider, hotswap: true }, working: true })
  const terminalTab = screen.getByRole('tab', { name: 'Terminal' })
  expect(terminalTab).not.toBeDisabled()
})
```

Adapt `renderAgentChatView`'s signature/props to whatever this test file's existing harness already exposes (it renders `AgentChatView` with a `provider` and store-seeded `working` state per the existing tests in this file — follow that exact pattern rather than introducing a new one).

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && bun test src/__tests__/features/agent/chat/agent-chat-view.test.tsx`
Expected: FAIL — switcher still renders when `hasTerminal:false`; terminal tab is never disabled today (dead prop, per spec §1.5).

- [ ] **Step 3: Wire the props**

In `web/src/features/agent/chat/agent-chat-view.tsx`, both `showSwitcher` sites (lines 361 and 486):

```tsx
      showSwitcher={presentation !== 'terminal' && provider?.hasTerminal !== false}
```

and both `SelectionCluster`/`ProviderBar` call sites gain:

```tsx
      handoverBlocked={!provider?.hotswap && working}
```

(`provider` and `working` are already in scope at both sites per the read in this plan's research — `provider` is passed a few lines above/below each `SelectionCluster`, `working` is a prop already threaded to the second call site at line ~483; confirm it is also in scope at the first, and lift it from the same source the second site uses if not.)

Confirm `splitEnabled` at both sites also ANDs `provider?.hotswap`:

```tsx
      splitEnabled={splitEnabled && provider?.hotswap === true}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && bun test src/__tests__/features/agent/chat/agent-chat-view.test.tsx src/__tests__/features/agent/components/agent-chat-pane.test.tsx`
Expected: PASS.

- [ ] **Step 5: Full web suite + typecheck**

Run: `cd web && bun test && bun tsc --noEmit`
Expected: PASS, no regressions in `view-switcher`/`selection-cluster` tests (both already have coverage for `handoverBlocked`'s disabled/tooltip rendering per the code read — this task only needs to prove something now PASSES it a real value).

- [ ] **Step 6: Commit**

```bash
git add web/src/features/agent/chat/agent-chat-view.tsx web/src/__tests__/features/agent/chat/agent-chat-view.test.tsx web/src/__tests__/features/agent/components/agent-chat-pane.test.tsx
git commit -m "feat(chat): feed handoverBlocked/hasTerminal/hotswap into the surface switcher"
```

---

### Task 7: Live-verify Capability 2 in the Tauri desktop app

**Files:** none (verification only).

- [ ] **Step 1: Build and launch the dev desktop app** against the worktree's isolated `CROWBAR_HOME` (see [[feedback_verify_via_dev_desktop_not_headless]] and [[project_dev_home_isolation]] — never the production socket).

Run: `make dev-desktop` (or this repo's equivalent — confirm the exact target in `Makefile`/`package.json` before running).

- [ ] **Step 2: Open a claude chat and a codex chat side by side (two tabs).** Using the Tauri MCP bridge (`mcp__tauri__*` tools), confirm via `webview_dom_snapshot`/`webview_find_element` that both show the Terminal segment enabled (not disabled) in the view switcher at all times, including mid-turn (send a prompt, and while `working` is true, confirm the terminal tab is still clickable).

- [ ] **Step 3: Confirm split is offered** (with the dev split setting on) for both providers, and that switching mid-turn does not interrupt the running turn (the PTY keeps running; re-selecting Chat shows the same, uninterrupted transcript).

- [ ] **Step 4: Report the verification result to the user before proceeding to Part B** — this is the natural checkpoint the spec calls "ships value immediately and is independently verifiable" (§4).

---

## Part B — Capability 1: Mixed Transport

Everything here builds toward one live-verified outcome (Task 15): a codex chat where `subagent_pre` (hooks) and `message_delta` (API) both reach the same chat aggregate, in order, with no duplicate turn, and a permission is answered from Crowbar's own chat UI for the first time ever for codex.

### Task 8: `wsrpc` — a transport-agnostic WebSocket-framed JSON-RPC2 client

This package knows nothing about Crowbar's descriptors or canonical events. It is a reusable client: dial a unix socket, speak WS-framed JSON-RPC2 over it, expose inbound frames on a channel, let the caller send requests/notifications/replies. Testable with zero real `codex` process — a real `httptest`-style unix-socket WS server standing in.

**Files:**
- Create: `api/internal/engine/agents/internal/protocol/internal/wsrpc/wsrpc.go`
- Create: `api/internal/engine/agents/internal/protocol/internal/wsrpc/wsrpc_test.go`

**Interfaces:**
- Consumes: `gorilla/websocket` (already a go.mod dependency).
- Produces:
  - `func Dial(ctx context.Context, socketPath string) (*Conn, error)`
  - `func (c *Conn) Call(ctx context.Context, method string, params any) (json.RawMessage, error)`
  - `func (c *Conn) Notify(method string, params any) error`
  - `func (c *Conn) Reply(id json.RawMessage, result json.RawMessage) error`
  - `func (c *Conn) Frames() <-chan Frame` where `type Frame struct { ID json.RawMessage; Method string; Params json.RawMessage }` — every inbound message that carries a `method` (both plain notifications and server-initiated asks that carry an `id`); pure request/response replies to *our own* `Call`s are consumed internally and never appear here.
  - `func (c *Conn) Close() error`

  Consumed by Task 10 (`apidriver`).

- [ ] **Step 1: Write the failing test — a fake unix-socket WS server, dial, initialize-style call round trip**

```go
package wsrpc_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/wsrpc"
)

// serveOnUnixSocket starts an httptest-style server listening on a unix socket
// (not TCP), upgrading every connection to a WebSocket and handing it to serve.
// codex's own app-server does exactly this over `--listen unix://PATH` — see the
// design spec's "Corrections" section for how this was confirmed live.
func serveOnUnixSocket(t *testing.T, serve func(*websocket.Conn)) string {
	t.Helper()
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sockPath)
	require.NoError(t, err)

	upgrader := websocket.Upgrader{}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		serve(conn)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close(); _ = os.Remove(sockPath) })
	return sockPath
}

func TestDialAndCall_RoundTrips(t *testing.T) {
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))
		require.Equal(t, "initialize", req.Method)
		resp, _ := json.Marshal(map[string]any{
			"id":     json.RawMessage(req.ID),
			"result": map[string]string{"codexHome": "/fake"},
		})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := wsrpc.Dial(ctx, sockPath)
	require.NoError(t, err)
	defer conn.Close()

	result, err := conn.Call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "crowbar"}})
	require.NoError(t, err)
	require.JSONEq(t, `{"codexHome":"/fake"}`, string(result))
}

func TestFrames_DeliversNotificationsAndAsksButNotOwnCallResponses(t *testing.T) {
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		// A notification (no id).
		note, _ := json.Marshal(map[string]any{"method": "thread/started", "params": map[string]string{"threadId": "t1"}})
		_ = conn.WriteMessage(websocket.TextMessage, note)
		// A server-initiated ask (has both id and method).
		ask, _ := json.Marshal(map[string]any{"id": 99, "method": "item/permissions/requestApproval", "params": map[string]string{"tool": "shell"}})
		_ = conn.WriteMessage(websocket.TextMessage, ask)
		// Read our reply to that ask and never send it back out as a Frame.
		_, _, _ = conn.ReadMessage()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := wsrpc.Dial(ctx, sockPath)
	require.NoError(t, err)
	defer conn.Close()

	frame1 := <-conn.Frames()
	require.Equal(t, "thread/started", frame1.Method)
	require.Nil(t, frame1.ID)

	frame2 := <-conn.Frames()
	require.Equal(t, "item/permissions/requestApproval", frame2.Method)
	require.NotNil(t, frame2.ID)

	require.NoError(t, conn.Reply(frame2.ID, json.RawMessage(`{"decision":"approved"}`)))
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/engine/agents/internal/protocol/internal/wsrpc/... -v`
Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Implement `wsrpc.go`**

```go
// Package wsrpc is a WebSocket-framed JSON-RPC2 client over a unix socket.
//
// codex's `app-server --listen unix://PATH` runs a plain HTTP-Upgrade WebSocket
// handshake over the socket (tungstenite), NOT raw newline-delimited JSON like its
// `--listen stdio://` mode does — confirmed live against codex-cli 0.146.0; a raw
// writer gets `httparse error: invalid token` and the connection is closed with no
// response. This package speaks the WebSocket layer so nothing above it has to.
//
// It knows nothing about Crowbar's descriptors, canonical events, or codex's own
// method names — that translation is protocol/internal/apidriver's job.
package wsrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Frame is one inbound message that carries a method: either a plain notification
// (ID nil) or a server-initiated request this connection's owner must Reply to
// (ID non-nil). A bare id+result/error frame — the response to OUR OWN Call — is
// never delivered here; Call consumes it directly.
type Frame struct {
	ID     json.RawMessage
	Method string
	Params json.RawMessage
}

type Conn struct {
	ws *websocket.Conn

	mu      sync.Mutex // guards WriteMessage — gorilla/websocket forbids concurrent writers
	nextID  int64
	pending map[int64]chan wireFrame

	frames chan Frame
	closed chan struct{}
	once   sync.Once
}

type wireFrame struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *wireError      `json:"error,omitempty"`
}

type wireError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Dial performs the WebSocket handshake over a unix socket at socketPath.
//
// EnableCompression is left at its zero value (false) DELIBERATELY: codex's
// server rejects a Sec-WebSocket-Extensions offer it does not recognise with
// "Missing, duplicated or incorrect header sec-websocket-extensions" and closes
// the connection — confirmed live. gorilla/websocket's default Dialer already
// offers no extensions, which is why this needs no explicit configuration.
func Dial(ctx context.Context, socketPath string) (*Conn, error) {
	dialer := websocket.Dialer{
		NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
		HandshakeTimeout: 10 * time.Second,
	}
	// The URL's host/scheme are ignored by NetDialContext; codex does not
	// validate them beyond requiring a well-formed request line.
	ws, _, err := dialer.DialContext(ctx, "ws://unix/", http.Header{})
	if err != nil {
		return nil, fmt.Errorf("wsrpc: dial %s: %w", socketPath, err)
	}
	c := &Conn{
		ws:      ws,
		pending: make(map[int64]chan wireFrame),
		frames:  make(chan Frame, 32),
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

func (c *Conn) readLoop() {
	defer close(c.frames)
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		var f wireFrame
		if err := json.Unmarshal(data, &f); err != nil {
			continue // a malformed frame is dropped, never fatal to the connection
		}
		if f.Method == "" && f.ID != nil {
			// A response to one of OUR calls.
			var id int64
			if err := json.Unmarshal(f.ID, &id); err != nil {
				continue
			}
			c.mu.Lock()
			ch, ok := c.pending[id]
			delete(c.pending, id)
			c.mu.Unlock()
			if ok {
				ch <- f
			}
			continue
		}
		select {
		case c.frames <- Frame{ID: f.ID, Method: f.Method, Params: f.Params}:
		case <-c.closed:
			return
		}
	}
}

// Frames delivers every inbound notification and server-initiated ask, in
// arrival order. Closed when the connection is closed or the server hangs up.
func (c *Conn) Frames() <-chan Frame { return c.frames }

// Call sends a JSON-RPC request and blocks for its matching response.
func (c *Conn) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("wsrpc: marshal params for %s: %w", method, err)
	}
	req, err := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int64           `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}{"2.0", id, method, paramsRaw})
	if err != nil {
		return nil, fmt.Errorf("wsrpc: marshal request %s: %w", method, err)
	}

	ch := make(chan wireFrame, 1)
	c.mu.Lock()
	c.pending[id] = ch
	writeErr := c.ws.WriteMessage(websocket.TextMessage, req)
	c.mu.Unlock()
	if writeErr != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("wsrpc: write %s: %w", method, writeErr)
	}

	select {
	case f := <-ch:
		if f.Error != nil {
			return nil, fmt.Errorf("wsrpc: %s: %s (code %d)", method, f.Error.Message, f.Error.Code)
		}
		return f.Result, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case <-c.closed:
		return nil, errors.New("wsrpc: connection closed")
	}
}

// Notify sends a JSON-RPC notification (no id, no reply expected).
func (c *Conn) Notify(method string, params any) error {
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("wsrpc: marshal params for %s: %w", method, err)
	}
	msg, err := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}{"2.0", method, paramsRaw})
	if err != nil {
		return fmt.Errorf("wsrpc: marshal notify %s: %w", method, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteMessage(websocket.TextMessage, msg)
}

// Reply answers a server-initiated ask (a Frame with a non-nil ID) with a
// JSON-RPC response frame carrying result verbatim.
func (c *Conn) Reply(id json.RawMessage, result json.RawMessage) error {
	msg, err := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
	}{"2.0", id, result})
	if err != nil {
		return fmt.Errorf("wsrpc: marshal reply: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteMessage(websocket.TextMessage, msg)
}

func (c *Conn) Close() error {
	var err error
	c.once.Do(func() {
		close(c.closed)
		err = c.ws.Close()
	})
	return err
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd api && go test ./internal/engine/agents/internal/protocol/internal/wsrpc/... -v -race`
Expected: PASS (run with `-race` — this package is concurrent by construction: the read loop, `Call`'s blocking wait, and `Frames()` consumers all run on different goroutines).

- [ ] **Step 5: `go vet` and lint**

Run: `cd api && go vet ./internal/engine/agents/internal/protocol/internal/wsrpc/... && golangci-lint run ./internal/engine/agents/internal/protocol/internal/wsrpc/...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add api/internal/engine/agents/internal/protocol/internal/wsrpc/
git commit -m "feat(agents): add wsrpc, a WebSocket-framed JSON-RPC2 client over a unix socket"
```

---

### Task 9: Reverse wire-event dispatch (`in:`/`ask:` + `when:` → canonical name)

The hooks path already knows the canonical name (the forwarder CLI is invoked as `crowbar hook session_start ...`). The API path only has the wire method (`"turn/completed"`) and must resolve it back to a canonical event, honoring `when:` for sum types (`item/started` serves `user_prompt`, `tool_pre`, and others depending on `item.type`).

**Files:**
- Create: `api/internal/engine/agents/internal/protocol/internal/dispatch/dispatch.go`
- Create: `api/internal/engine/agents/internal/protocol/internal/dispatch/dispatch_test.go`

**Interfaces:**
- Consumes: `spec.Descriptor.Events` (existing), `mapping.Match` (existing, `api/internal/engine/agents/internal/mapping/mapping.go:314`).
- Produces: `func Resolve(d *spec.Descriptor, wireMethod string, params map[string]any) (canonical string, ok bool)` — consumed by Task 10.

- [ ] **Step 1: Write the failing test using the real captured fixtures**

```go
package dispatch_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/dispatch"
)

func loadCodexAPIDescriptor(t *testing.T) *spec.Descriptor {
	t.Helper()
	raw, err := os.ReadFile("../descriptor/descriptors-v3/experimental/codex-api.yaml")
	require.NoError(t, err)
	d, err := descriptor.ParseV3(raw)
	require.NoError(t, err)
	return d
}

func loadParams(t *testing.T, fixture string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/fixtures/codex-api/" + fixture + ".json")
	require.NoError(t, err)
	var frame struct {
		Params map[string]any `json:"params"`
	}
	require.NoError(t, json.Unmarshal(raw, &frame))
	return frame.Params
}

func TestResolve_PlainEventNoSumType(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	canonical, ok := dispatch.Resolve(d, "turn/completed", loadParams(t, "turn_completed"))
	require.True(t, ok)
	assert.Equal(t, "turn_stop", canonical)
}

func TestResolve_SumTypeDisambiguatesByItemType(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	canonical, ok := dispatch.Resolve(d, "item/started", loadParams(t, "item_started"))
	require.True(t, ok)
	// item_started.json's captured item.type decides which of {user_prompt,
	// tool_pre} this resolves to — assert whichever the fixture actually is by
	// reading its item.type first (do not hard-code a guess).
	params := loadParams(t, "item_started")
	item, _ := params["item"].(map[string]any)
	if item["type"] == "userMessage" {
		assert.Equal(t, "user_prompt", canonical)
	} else {
		assert.Equal(t, "tool_pre", canonical)
	}
}

func TestResolve_UnknownWireMethodIsNotOK(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	_, ok := dispatch.Resolve(d, "some/method/nobody/declared", map[string]any{})
	assert.False(t, ok)
}

func TestResolve_OutboundEventsAreNeverCandidates(t *testing.T) {
	// "prompt" declares out: turn/start — Resolve must never match an inbound
	// wire method against an outbound event's Send templates.
	d := loadCodexAPIDescriptor(t)
	_, ok := dispatch.Resolve(d, "turn/start", map[string]any{})
	assert.False(t, ok, "turn/start is what WE send, not something codex reports")
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/engine/agents/internal/protocol/internal/dispatch/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement**

```go
// Package dispatch resolves an inbound API-transport wire frame — a method name
// plus its decoded params — back to the canonical event name the hooks transport
// already names explicitly (the forwarder CLI is invoked with the canonical name
// as an argument; the API transport carries only the provider's own method name).
//
// Sum types (one wire method serving several canonical events, selected by a
// discriminator field) are resolved via the same `when:` mechanism the
// descriptor already declares for exactly this purpose.
package dispatch

import (
	"sort"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Resolve finds the canonical event whose in: or ask: names wireMethod and whose
// when: (if any) matches params. Deterministic when more than one candidate's
// when: matches (should not happen for a well-formed descriptor, but iteration
// order over a map must not make a bug non-reproducible): candidates are tried in
// sorted-name order and the first match wins.
func Resolve(d *spec.Descriptor, wireMethod string, params map[string]any) (string, bool) {
	names := make([]string, 0, len(d.Events))
	for name := range d.Events {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		ev := d.Events[name]
		wire, direction := ev.WireEvent()
		if direction == "out" || wire != wireMethod {
			continue
		}
		if !mapping.Match(params, ev.When) {
			continue
		}
		return name, true
	}
	return "", false
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd api && go test ./internal/engine/agents/internal/protocol/internal/dispatch/... -v`
Expected: PASS.

- [ ] **Step 5: `go vet` and lint**

Run: `cd api && go vet ./internal/engine/agents/internal/protocol/internal/dispatch/... && golangci-lint run ./internal/engine/agents/internal/protocol/internal/dispatch/...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add api/internal/engine/agents/internal/protocol/internal/dispatch/
git commit -m "feat(agents): resolve API wire frames to canonical events via when: matching"
```

---

### Task 10: `apidriver` — the protocol-facing API transport driver

Owns dial+handshake+receive-loop+reply for one runner's API connection, and is the one new thing `protocol.go` (the existing single façade — see its own doc comment: "Everything under internal/ here is reachable only through this file") exposes. It does NOT know about `Turns`, `answerdesk`, or any usecase-layer type — it hands the caller `(canonical string, rawJSON []byte, ask *Ask)` per inbound frame and lets the caller (Task 11, in the `runner` usecase package, which already holds direct references to `rs.turns` and `rs.answers`) decide what to do.

**Files:**
- Create: `api/internal/engine/agents/internal/protocol/internal/apidriver/apidriver.go`
- Create: `api/internal/engine/agents/internal/protocol/internal/apidriver/apidriver_test.go`
- Modify: `api/internal/engine/agents/internal/protocol/protocol.go` (add the façade functions)

**Interfaces:**
- Consumes: `wsrpc.Dial`/`.Call`/`.Frames`/`.Reply` (Task 8), `dispatch.Resolve` (Task 9), `spec.Descriptor.Events[canonical].Map` via the existing `inbound.Parse`... — actually simpler: apidriver does NOT call `inbound.Parse` itself (that stays the ingest path's job, unchanged, fed with raw JSON bytes exactly as hooks are). apidriver only resolves the canonical name and re-marshals `params` to raw bytes.
- Produces:
  - `type Event struct { Canonical string; Raw []byte; AskID json.RawMessage }` — `AskID` is non-nil exactly when this event is an `ask:` the caller may reply to.
  - `func Start(ctx context.Context, d *spec.Descriptor, socketPath string) (*Driver, error)` — dials, runs `handshake.call` (from `d.Runtime.API.Handshake["call"]`), sends `initialized` if the handshake succeeded (mirrors the capture script's own sequence), and starts the receive loop.
  - `func (drv *Driver) Events() <-chan Event`
  - `func (drv *Driver) Reply(canonical string, raw []byte, decision models.AnswerDecision) error` — renders via the EXISTING `answer.Render` (already transport-agnostic — confirmed by reading `AnswerChoice` in `api/internal/app/usecases/chat/answers.go:165`, which calls `agent.RenderAnswer` and hands the bytes straight to `answerdesk.Resolve` with no transport-specific branching) and writes the JSON-RPC reply frame. Actually — simplify: since `RenderAnswer` already lives on `engineagents.Agent` and produces exactly the bytes to send, `apidriver.Driver.Reply` takes the ALREADY-RENDERED bytes (the caller renders; the driver only frames and writes), matching the separation of concerns in Task 11.
  - Corrected signature: `func (drv *Driver) Reply(askID json.RawMessage, rendered []byte) error` — wraps `rendered` as the JSON-RPC `result` and calls the underlying `wsrpc.Conn.Reply`.
  - `func (drv *Driver) Send(canonical string, values map[string]string) error` — for outbound events (`prompt`, `interrupt`, `compact_start`); resolves via the EXISTING `outbound.Resolve` (`protocol.Send`, already transport-agnostic) to get the wire method + payload, then `wsrpc.Conn.Notify` (all three of codex-api.yaml's outbound events are declared `out:`, i.e. notifications with no reply expected — confirm this holds; if a future outbound event is declared `ask:`-shaped from our side, `Send` would need `Call` instead, but nothing in scope here needs that).
  - `func (drv *Driver) Close() error`

- [ ] **Step 1: Write the failing test — full round trip against a fake server using the REAL codex-api.yaml descriptor and REAL fixtures**

```go
package apidriver_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/apidriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
)

func loadCodexAPIDescriptor(t *testing.T) *spec.Descriptor {
	t.Helper()
	raw, err := os.ReadFile("../descriptor/descriptors-v3/experimental/codex-api.yaml")
	require.NoError(t, err)
	d, err := descriptor.ParseV3(raw)
	require.NoError(t, err)
	return d
}

// fakeCodexServer replays a scripted sequence of frames after completing the
// initialize handshake exactly as the real app-server does (see wsrpc's own test
// for the base handshake plumbing; this adds the initialized notification step
// and a scripted frame sequence).
func fakeCodexServer(t *testing.T, afterInit func(*websocket.Conn)) string {
	t.Helper()
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sockPath)
	require.NoError(t, err)
	upgrader := websocket.Upgrader{}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		_, msg, err := conn.ReadMessage() // initialize
		require.NoError(t, err)
		var req struct{ ID json.RawMessage }
		require.NoError(t, json.Unmarshal(msg, &req))
		resp, _ := json.Marshal(map[string]any{"id": json.RawMessage(req.ID), "result": map[string]string{}})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
		_, _, _ = conn.ReadMessage() // initialized notification, discarded
		afterInit(conn)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return sockPath
}

func TestStart_HandshakeThenDeliversCanonicalEvents(t *testing.T) {
	turnCompleted, err := os.ReadFile("../../testdata/fixtures/codex-api/turn_completed.json")
	require.NoError(t, err)

	sockPath := fakeCodexServer(t, func(conn *websocket.Conn) {
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, turnCompleted))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	select {
	case ev := <-drv.Events():
		require.Equal(t, "turn_stop", ev.Canonical)
		require.Nil(t, ev.AskID)
		require.Contains(t, string(ev.Raw), "threadId")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the dispatched event")
	}
}

func TestStart_AsksCarryAReplyChannel(t *testing.T) {
	sockPath := fakeCodexServer(t, func(conn *websocket.Conn) {
		ask, _ := json.Marshal(map[string]any{
			"id": 7, "method": "item/permissions/requestApproval",
			"params": map[string]string{"tool": "shell"},
		})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, ask))
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		var reply struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		require.NoError(t, json.Unmarshal(msg, &reply))
		require.Equal(t, 7, reply.ID)
		require.JSONEq(t, `{"decision":"approved"}`, string(reply.Result))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	ev := <-drv.Events()
	require.Equal(t, "permission", ev.Canonical)
	require.NotNil(t, ev.AskID)
	require.NoError(t, drv.Reply(ev.AskID, []byte(`{"decision":"approved"}`)))
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/engine/agents/internal/protocol/internal/apidriver/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement**

```go
// Package apidriver drives one provider's API transport connection: dial,
// handshake, receive loop, and reply — the pieces protocol/protocol.go exposes so
// the runner usecase (which already holds the Turns/answerdesk references this
// needs to feed) never has to know wsrpc or dispatch exist.
package apidriver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/dispatch"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/wsrpc"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/outbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Event is one canonical event resolved from the wire. AskID is non-nil exactly
// when this is an ask: the caller may Reply to.
type Event struct {
	Canonical string
	Raw       []byte
	AskID     json.RawMessage
}

type Driver struct {
	conn *wsrpc.Conn
	d    *spec.Descriptor
	out  chan Event
}

// Start dials socketPath, runs the descriptor's declared handshake call, sends
// `initialized` (mirroring the sequence codex's own fixture-capture script uses),
// and starts translating inbound frames into canonical Events.
func Start(ctx context.Context, d *spec.Descriptor, socketPath string) (*Driver, error) {
	conn, err := wsrpc.Dial(ctx, socketPath)
	if err != nil {
		return nil, err
	}
	call := d.Runtime.API.Handshake["call"]
	if call == "" {
		conn.Close()
		return nil, fmt.Errorf("apidriver: descriptor %s declares no handshake call", d.ID)
	}
	if _, err := conn.Call(ctx, call, map[string]any{
		"clientInfo": map[string]string{"name": "crowbar", "title": "Crowbar", "version": "0.0.0"},
	}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("apidriver: %s handshake: %w", d.ID, err)
	}
	if err := conn.Notify("initialized", map[string]any{}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("apidriver: %s initialized: %w", d.ID, err)
	}

	drv := &Driver{conn: conn, d: d, out: make(chan Event, 64)}
	go drv.translateLoop()
	return drv, nil
}

func (drv *Driver) translateLoop() {
	defer close(drv.out)
	for frame := range drv.conn.Frames() {
		var params map[string]any
		if err := json.Unmarshal(frame.Params, &params); err != nil {
			continue // a malformed params object is dropped, never fatal
		}
		canonical, ok := dispatch.Resolve(drv.d, frame.Method, params)
		if !ok {
			continue // this provider's descriptor does not map this wire method
		}
		drv.out <- Event{Canonical: canonical, Raw: frame.Params, AskID: frame.ID}
	}
}

// Events delivers every canonical event this driver has resolved, in arrival
// order. Closed when the underlying connection closes.
func (drv *Driver) Events() <-chan Event { return drv.out }

// Reply answers an ask (an Event whose AskID is non-nil) with already-rendered
// bytes — rendering is the caller's job via the existing, transport-agnostic
// engineagents.Agent.RenderAnswer, so this package stays free of any per-event
// rendering knowledge.
func (drv *Driver) Reply(askID json.RawMessage, rendered []byte) error {
	return drv.conn.Reply(askID, json.RawMessage(rendered))
}

// Send drives an outbound canonical event (prompt, interrupt, compact_start) by
// resolving it through the SAME outbound.Resolve the hooks transport's
// injection-based sends use, then notifying over the socket.
func Send(drv *Driver, canonical string, values map[string]string) error {
	wire, payload, ok := outbound.Resolve(drv.d, canonical, values)
	if !ok {
		return fmt.Errorf("apidriver: %s does not declare outbound event %q", drv.d.ID, canonical)
	}
	params := make(map[string]any, len(payload))
	for k, v := range payload {
		params[k] = v
	}
	return drv.conn.Notify(wire, params)
}

func (drv *Driver) Close() error {
	return drv.conn.Close()
}
```

Note: `outbound.Resolve`'s existing signature returns `payload map[string]string` (confirmed by reading `protocol.go:96-100` — `Send(d, canonical, values) (wireEvent string, payload map[string]string, ok bool)`), so the `Send` helper above widens string values into `map[string]any` only to satisfy `Notify`'s `any` params — if `outbound.Resolve`'s payload is ever non-scalar, revisit this cast; for `prompt`/`interrupt`/`compact_start` in `codex-api.yaml` every value is a plain string template.

- [ ] **Step 4: Run to verify it passes**

Run: `cd api && go test ./internal/engine/agents/internal/protocol/internal/apidriver/... -v -race`
Expected: PASS.

- [ ] **Step 5: Expose through `protocol.go` — as a WRAPPER, not a bare re-export**

**Why a wrapper and not `type APIEvent = apidriver.Event` plus returning `*apidriver.Driver` directly:** Go's `internal/` visibility is per directory, and `apidriver` sits at `.../protocol/internal/apidriver` — importable only by code at-or-under `.../protocol/`. `protocol.go` itself qualifies, so it may hold and call `*apidriver.Driver` freely. But `agents.go` (Task 11 needs this) sits at `api/internal/engine/agents` — under the FIRST `internal` boundary (parent `api`, so it may import `protocol`, which is exactly what it already does today for `ParseHook`/`RenderAnswer`/etc.) but NOT under the SECOND one (parent `.../agents/internal/protocol`), so it cannot name `apidriver.Driver` or `apidriver.Event` even indirectly through an alias declared elsewhere — every layer must define its OWN alias/wrapper of the layer below, exactly the pattern `aliases.go` already uses for `models.Capabilities` → `engineagents.Capabilities`. Add to `api/internal/engine/agents/internal/protocol/protocol.go`:

```go
// --- api transport: a persistent connection instead of a per-hook payload -----

// APIEvent is one canonical event resolved from a provider's API-transport wire
// frame. AskID is non-nil exactly when a human decision must be sent back.
type APIEvent = apidriver.Event

// APIConn wraps *apidriver.Driver so a caller outside this package's own
// internal/ boundary (agents.go, one layer up) can hold and call it without
// ever naming the apidriver package — see this task's Step 5 note on why that
// indirection is required, not optional.
type APIConn struct{ drv *apidriver.Driver }

func (c *APIConn) Events() <-chan APIEvent           { return c.drv.Events() }
func (c *APIConn) Reply(askID json.RawMessage, rendered []byte) error {
	return c.drv.Reply(askID, rendered)
}
func (c *APIConn) Close() error { return c.drv.Close() }

// StartAPIDriver dials a provider's API socket, completes its declared
// handshake, and returns a connection translating inbound frames into canonical
// events — see apidriver's own doc comment for why this is the one new thing
// this façade exposes for capability 1.
func StartAPIDriver(ctx context.Context, d *spec.Descriptor, socketPath string) (*APIConn, error) {
	drv, err := apidriver.Start(ctx, d, socketPath)
	if err != nil {
		return nil, err
	}
	return &APIConn{drv: drv}, nil
}
```

(Add the `apidriver`, `context`, and `encoding/json` imports to `protocol.go`'s import block.)

- [ ] **Step 6: `go vet`, lint, full protocol package test**

Run: `cd api && go vet ./internal/engine/agents/internal/protocol/... && golangci-lint run ./internal/engine/agents/internal/protocol/... && go test ./internal/engine/agents/internal/protocol/...`
Expected: clean, all PASS.

- [ ] **Step 7: Commit**

```bash
git add api/internal/engine/agents/internal/protocol/internal/apidriver/ api/internal/engine/agents/internal/protocol/protocol.go
git commit -m "feat(agents): add apidriver, the API-transport connection driver, behind protocol.go"
```

---

### Task 11: Spawn `serve`, handshake, then `attach` in the PTY

The runner currently assumes one process, one PTY (`rs.term.CreateCommand` in `api/internal/app/usecases/chat/internal/runner/spawn.go:227`). For an api-transport descriptor, spawn must additionally start `serve` as a background (non-PTY) process, complete the handshake via `apidriver.Start`, and only then hand the PTY to `attach`'s argv. Failure of `attach` must not fail the session (spec §2.2b).

**Files:**
- Modify: `api/internal/app/usecases/chat/internal/runner/spawn.go`
- Modify: `api/internal/app/usecases/chat/internal/runner/runner.go` (new field on `Runners` for the per-runner driver registry)
- Create: `api/internal/app/usecases/chat/internal/runner/apiconn.go` (the serve+handshake+registry helper, kept out of `spawn.go` to keep that file's existing size in check)
- Test: `api/internal/app/usecases/chat/internal/runner/apiconn_test.go`
- Test: `api/internal/app/usecases/chat/runner_test.go` (integration-level, at the usecase façade)

**Interfaces:**
- Consumes: `engineagents.Agent.StartAPIConn` (new — see Step 0 below, wraps Task 10's `protocol.StartAPIDriver`), `descriptor.Spawn` (existing — resolves `serve`'s argv via the SAME `spawn.Plan`/template-expansion path `attach`'s argv already uses, since `{socket}` is descriptor-templated per spec §2.2b), the existing `binpath.Resolve`.
- Produces: `func (rs *Runners) startAPIConn(ctx context.Context, runnerID string, agent engineagents.Agent, tctx engineagents.TemplateCtx) (*apiconn, bool)` — a private helper `spawnRunner` calls before `forkCLI`, returning `ok=false` (never an error) when the descriptor is not api-transport, so the hooks-only path (claude, and codex until Task 14 lands) is untouched.
- The per-runner driver registry: a small `map[string]*apiconn` protected by a mutex, keyed by runnerID, so Task 12/13 (ingestion routing, answer replies) and Task 11's own teardown (`onRunnerExit`) can find and close it. `apiconn.driver` holds an `engineagents.APIConn` (the top-level alias, NOT `*apidriver.Driver` — `runner` sits outside `agents/internal/`'s visibility entirely, one boundary further out than `agents.go` itself; see Step 0).

- [ ] **Step 0: Add the `agents.go`-level wrapper `runner` package can actually import**

`runner` (`api/internal/app/usecases/chat/internal/runner`) is a sibling subtree of `agents` entirely — it already reaches every provider capability (`ParseHook`, `RenderAnswer`, `SpawnPlan`, ...) exclusively through the exported `engineagents.Agent` interface in `api/internal/engine/agents/agents.go`, never through `protocol` directly, because `protocol` lives under `agents/internal/` and is invisible outside the `agents` tree. The API driver must follow the identical pattern — write the test for this BEFORE Task 11's Step 1, since everything else in this task calls it:

```go
// api/internal/engine/agents/agents_test.go
func TestAgent_StartAPIConnIsANoopForHooksTransport(t *testing.T) {
	a := &agent{spec: hooksTransportTestDescriptor(t)} // e.g. resolve "claude"
	_, err := a.StartAPIConn(context.Background(), "/nonexistent.sock")
	assert.ErrorIs(t, err, engineagents.ErrAPITransportNotDeclared)
}
```

Run it, watch it fail to compile, then add to `api/internal/engine/agents/aliases.go` (beside the other `type X = models.X` / `type X = spec.X` lines):

```go
	APIEvent = protocol.APIEvent
	APIConn  = protocol.APIConn
```

(add `protocol "github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"` to `aliases.go`'s imports if not already present under that name — `agents.go` already imports it as `"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"` with no alias, i.e. package name `protocol`; reuse the same unaliased import in `aliases.go`).

Add `ErrAPITransportNotDeclared` to `api/internal/engine/agents/errors.go` beside the existing sentinel errors there, then extend the `Agent` interface in `agents.go`:

```go
	// StartAPIConn dials this provider's API socket and completes its declared
	// handshake. socketPath is ALREADY resolved (the caller expands {socket} via
	// the same TemplateCtx machinery SpawnPlan uses). Returns
	// ErrAPITransportNotDeclared for a hooks-only descriptor — never nil, nil.
	StartAPIConn(ctx context.Context, socketPath string) (APIConn, error)

	// APIServeArgv and APIAttachArgv are runtime.api.serve / .attach, already
	// template-expanded against ctx, mirroring PromptSteps's (x, bool) shape: ok
	// is false when this descriptor declares no such field.
	APIServeArgv(ctx TemplateCtx) ([]string, bool)
	APIAttachArgv(ctx TemplateCtx) ([]string, bool)

	// TransportFor is spec.Descriptor.TransportFor, exposed narrowly for exactly
	// the reason every other capability on this interface is narrow rather than
	// exposing the raw descriptor: the `runner` usecase package (Task 12/13) is
	// OUTSIDE agents/internal/'s visibility entirely and can only ever reach a
	// provider through this interface, never through *spec.Descriptor by name.
	TransportFor(canonical string) string
```

and implement on `*agent`:

```go
func (a *agent) StartAPIConn(ctx context.Context, socketPath string) (APIConn, error) {
	if a.spec.Runtime.Transport != "api" && !hasAPIEventOverride(a.spec) {
		return APIConn{}, ErrAPITransportNotDeclared
	}
	conn, err := protocol.StartAPIDriver(ctx, a.spec, socketPath)
	if err != nil {
		return APIConn{}, err
	}
	return *conn, nil
}

func (a *agent) APIServeArgv(ctx TemplateCtx) ([]string, bool) {
	if len(a.spec.Runtime.API.Serve) == 0 {
		return nil, false
	}
	return expandArgv(a.spec.Runtime.API.Serve, ctx), true
}

func (a *agent) APIAttachArgv(ctx TemplateCtx) ([]string, bool) {
	if len(a.spec.Runtime.API.Attach) == 0 {
		return nil, false
	}
	return expandArgv(a.spec.Runtime.API.Attach, ctx), true
}

func (a *agent) TransportFor(canonical string) string {
	return a.spec.TransportFor(canonical)
}
```

`expandArgv` and `hasAPIEventOverride` are small new unexported helpers in `agents.go`: the former maps `template.Expand(arg, ctx)` over the slice (mirroring `spawn.Plan`'s own `for _, a := range d.Spawn.Args { plan.Argv = append(plan.Argv, template.Expand(a, ctx)) }` at `spawn.go:30-32`); the latter checks whether ANY event declares `transport: api` even when `Runtime.Transport != "api"` (defensive — not reachable for either shipped descriptor today, but cheap to make correct rather than assuming `Runtime.Transport` is the only source of truth).

`APIConn` above is used **by value** (`APIConn{}`/`*conn`), not by pointer, because `protocol.APIConn`'s methods are already defined on `*protocol.APIConn` in Task 10 — reconcile this: either dereference once here (as shown) and keep `engineagents.APIConn` a plain struct alias whose zero value is a safe "invalid" sentinel distinguishable from a real connection (add an unexported `valid bool` field set on successful construction, checked by `Events`/`Reply`/`Close`), or change the interface method to return `*APIConn` and skip the dereference entirely. Prefer returning `*APIConn` (skip the dereference, return `conn` directly, change the interface signature to `(*APIConn, error)`) — it is simpler and matches every other pointer-returning method already on this interface (`SpawnPlan` returns `*SpawnPlan`).

Run: `cd api && go test ./internal/engine/agents/... -run TestAgent_StartAPIConn -v`
Expected: PASS once the above compiles.

- [ ] **Step 1: Understand exactly where `{socket}` must resolve — write the failing test for THAT first, in isolation**

Before touching the runner, confirm the template-expansion path used for `serve`/`attach` args resolves `{socket}` to a path that satisfies the sun_path 104-byte limit outside a Crowbar worktree (spec §2.2b references [[project_dev_home_isolation]] for exactly this). Add to `api/internal/engine/agents/internal/template/template_test.go` (or wherever `TemplateCtx`'s existing fields are tested):

```go
func TestTemplateCtx_SocketPathIsShortEnoughForAUnixSocket(t *testing.T) {
	ctx := models.TemplateCtx{ /* however this repo's other tests build one, plus */ Socket: "/some/short/path/s.sock" }
	rendered := template.Expand("unix://{socket}", ctx)
	assert.LessOrEqual(t, len(rendered)-len("unix://"), 104,
		"macOS sun_path is 104 bytes — see [[project_dev_home_isolation]]")
}
```

First check: does `models.TemplateCtx` already have a field a `{socket}` placeholder could bind to? Read `api/internal/engine/agents/internal/models/template.go` before writing this step for real — if `TemplateCtx` has no such field yet, add `Socket string` there (a short path under the daemon's runtime dir, NOT under `worktree`/`tmpDir` — those already 104-byte-risk per the referenced memory) and wire it exactly where `Cwd`/`Tmp` are already populated in `renderSpawnContext` (`api/internal/app/usecases/chat/internal/runner/spawn.go` — search for `spawnContext` and its render function).

- [ ] **Step 2: Run, fix, confirm green** (mechanical — this step exists to lock the socket-path decision down before Step 3 depends on it).

- [ ] **Step 3: Write the failing test for `startAPIConn`**

```go
// api/internal/app/usecases/chat/internal/runner/apiconn_test.go
func TestStartAPIConn_HooksTransportIsANoop(t *testing.T) {
	rs := newTestRunners(t) // this file's existing harness constructor — reuse it
	d := hooksTransportTestAgent(t) // e.g. resolve "claude" via the real descriptor loader
	_, ok := rs.startAPIConn(context.Background(), "runner-1", d, models.TemplateCtx{})
	assert.False(t, ok, "a hooks-transport provider starts no API connection at all")
}

func TestStartAPIConn_APITransportDialsAndHandshakes(t *testing.T) {
	sockPath := fakeAppServer(t) // a tiny helper in this test file, same shape as apidriver's own fake server
	rs := newTestRunners(t)
	d := apiTransportTestAgentPointingAt(t, sockPath) // a descriptor whose api.serve is a no-op that never actually starts a process — see Step 4's note
	conn, ok := rs.startAPIConn(context.Background(), "runner-1", d, models.TemplateCtx{Socket: sockPath})
	require.True(t, ok)
	defer conn.driver.Close()
}
```

- [ ] **Step 4: Run to verify it fails**

Run: `cd api && go test ./internal/app/usecases/chat/internal/runner/... -run TestStartAPIConn -v`
Expected: FAIL — `startAPIConn` does not exist.

Note for Step 3's second test: `rs.startAPIConn` must NOT itself spawn the `serve` process in this unit test's scope — that half belongs to `forkServeProcess` (Step 5, tested separately with a real short-lived process like `sleep 1` or `true`, never a real codex binary in a unit test). Split the test so `startAPIConn` is exercised against an ALREADY-LISTENING fake socket (as above), and `forkServeProcess` is exercised separately against a trivial real command.

- [ ] **Step 5: Implement `apiconn.go`**

```go
// Package runner (file apiconn.go) is the api-transport half of spawning a
// provider: start `serve` as a background process, complete the handshake, and
// hand the connection to whatever needs to feed it into ingestion (bound after
// construction the same way rs.turns already is — see runner.go's own comment on
// that field for why).
//
// This file reaches the API transport ONLY through engineagents.Agent
// (StartAPIConn/APIServeArgv/APIAttachArgv, added in Step 0) — never through
// .../agents/internal/protocol or .../apidriver, which this package's import
// path has no visibility into (see Step 0's note on Go's internal/ boundaries).
package runner

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type apiconn struct {
	serveCmd *exec.Cmd
	driver   *engineagents.APIConn
}

// apiConns is the per-runner registry Task 12/13 and onRunnerExit look up by
// runnerID. In memory only, like every other live-process registry in this
// package (answerdesk's Desk, pendingHooks) — it describes a live connection to
// a live process, so it cannot survive a restart and must not try to.
type apiConnRegistry struct {
	mu    sync.Mutex
	byRun map[string]*apiconn
}

func newAPIConnRegistry() *apiConnRegistry {
	return &apiConnRegistry{byRun: make(map[string]*apiconn)}
}

func (r *apiConnRegistry) set(runnerID string, c *apiconn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byRun[runnerID] = c
}

func (r *apiConnRegistry) get(runnerID string) (*apiconn, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byRun[runnerID]
	return c, ok
}

func (r *apiConnRegistry) drop(runnerID string) {
	r.mu.Lock()
	c, ok := r.byRun[runnerID]
	delete(r.byRun, runnerID)
	r.mu.Unlock()
	if ok {
		if c.driver != nil {
			_ = c.driver.Close()
		}
		if c.serveCmd != nil && c.serveCmd.Process != nil {
			_ = c.serveCmd.Process.Kill()
		}
	}
}

// startAPIConn starts nothing and returns ok=false for a hooks-transport
// descriptor. For an api-transport one, it forks `serve`, waits for the socket to
// exist, and hands the connection to protocol.StartAPIDriver.
func (rs *Runners) startAPIConn(
	ctx context.Context,
	runnerID string,
	agent engineagents.Agent,
	tctx engineagents.TemplateCtx,
) (*apiconn, bool) {
	serveArgv, ok := agent.APIServeArgv(tctx) // Step 0's accessor
	if !ok {
		return nil, false
	}
	cmd, err := forkServeProcess(ctx, serveArgv)
	if err != nil {
		return nil, false // logged by the caller; a failed serve degrades to no API conn, never fails the spawn
	}
	if err := waitForSocket(ctx, tctx.Socket); err != nil {
		_ = cmd.Process.Kill()
		return nil, false
	}
	driver, err := agent.StartAPIConn(ctx, tctx.Socket) // Step 0's accessor — the ONLY way this package reaches the API transport
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, false
	}
	conn := &apiconn{serveCmd: cmd, driver: driver}
	rs.apiConns.set(runnerID, conn)
	return conn, true
}
```

This step needs two small, ordinary helpers this plan must make explicit rather than leave implicit (the "No Placeholders" rule applies to helpers too): `forkServeProcess` and `waitForSocket`. Implement `forkServeProcess` with `exec.CommandContext` plus `cmd.Start()` (not `Run()` — this is a long-lived background process, argv built the same way `spawn.go:166` already does — `append([]string{argv[0]}, argv[1:]...)` with `binpath.Resolve(argv[0])` for the executable), and `waitForSocket` as a short poll loop (`os.Stat` in a `time.Tick`-driven loop bounded by `ctx`), matching the style already used elsewhere in this codebase for socket-readiness waits (search for an existing poll-for-socket helper before writing a new one — `internal/core/terminal` or the daemon's own startup code likely has one; reuse it if so).

- [ ] **Step 6: Wire `startAPIConn` into `spawnRunner`**

In `api/internal/app/usecases/chat/internal/runner/spawn.go`, `spawnRunner` currently goes straight to `forkCLI` with `argv := append([]string{plan.Executable}, plan.Argv...)` built from `descriptor.SpawnPlan`. Insert the api-transport branch before that:

```go
	plan, err := descriptor.SpawnPlan(tctx, os.Environ(), steps)
	if err != nil {
		rs.agents.ForgetRunner(runnerID)
		return "", fmt.Errorf("agent: spawn runner: build spawn plan: %w", err)
	}

	// api-transport: start `serve`, complete the handshake, THEN let the PTY
	// carry `attach`'s argv instead of the bare descriptor's spawn.cmd. Failure
	// here must never fail the session — the chat still works over hooks alone
	// if `serve` cannot start (design spec §2.2b).
	if _, ok := rs.startAPIConn(ctx, runnerID, descriptor, tctx); ok {
		if attachArgv, ok := descriptor.APIAttachArgv(tctx); ok {
			plan.Argv = attachArgv[1:] // argv[0] is the executable, matching plan.Executable's own convention
			plan.Executable = binpath.Resolve(attachArgv[0])
		}
		// no attach declared: serve is running, the PTY is absent for this
		// session (capability 2's third state, reached here rather than by
		// declaration alone — see design spec §3.1's own framing of this case).
	}

	argv := append([]string{plan.Executable}, plan.Argv...)
```

(`APIAttachArgv` is the mirror of `APIServeArgv` for the `attach:` field, same `(x, bool)` shape.)

- [ ] **Step 7: Teardown — extend `onRunnerExit`**

In `spawn.go`'s `onRunnerExit`, add `rs.apiConns.drop(runnerID)` alongside the existing cleanup (`worktreepath.RemoveUnderHome`), so a dead runner's serve process and driver are never leaked.

- [ ] **Step 8: Run to verify it passes**

Run: `cd api && go test ./internal/app/usecases/chat/internal/runner/... -v -race`
Expected: PASS.

- [ ] **Step 9: `go vet`, lint, build**

Run: `cd api && go build ./... && go vet ./... && golangci-lint run ./internal/app/usecases/chat/...`
Expected: clean.

- [ ] **Step 10: Commit**

```bash
git add api/internal/app/usecases/chat/internal/runner/ api/internal/engine/agents/
git commit -m "feat(agents): spawn serve, handshake, then attach in the PTY for api-transport providers"
```

---

### Task 12: Route API-driver events into `Turns.IngestHook` — no double-counting

Every `apiconn`'s driver events must reach the SAME `IngestHook(ctx, runnerID, provider, canonicalEvent, rawPayload)` entrypoint hooks already use (`api/internal/app/usecases/chat/internal/turn/ingest.go:17`). Since `runnerID` is already known here (the driver is registered per-runner, not resolved from any wire content), the cross-wire session-identity problem the spec worried about (§2.2c) mostly does not apply to Crowbar's own side — this task's real job is de-duplication: `turn_stop` must not double-fire once both wires could in principle report it (in practice, once Task 14 merges the descriptor, `turn_stop` is declared ONLY on the api transport per the per-event `transport:` override, so this is enforced by construction, not by a runtime check — but this task still needs a small guard for the transition period where `codex.yaml` is not yet merged, see Step 3).

**Files:**
- Modify: `api/internal/app/usecases/chat/internal/runner/apiconn.go` (add the pump goroutine)
- Test: `api/internal/app/usecases/chat/internal/runner/apiconn_test.go`

**Interfaces:**
- Consumes: `apidriver.Driver.Events()` (Task 10), `rs.turns.IngestHook` (existing, already a field on `Runners`).
- Produces: every event on `conn.driver.Events()` reaching `rs.turns.IngestHook(ctx, runnerID, providerID, ev.Canonical, ev.Raw)` — but only events for canonical names this descriptor declares with `transport: api` (or no override, defaulting to the runtime's `api`) — checked via `d.TransportFor(ev.Canonical) == "api"`, so if a future edit to a descriptor accidentally leaves an event dual-declared, the driver-fed copy is dropped rather than double-applied, and the guard is a single, obviously-correct line rather than a wire-content heuristic.

- [ ] **Step 1: Write the failing test**

```go
func TestStartAPIConn_PumpsDriverEventsIntoIngestHook(t *testing.T) {
	var got []string
	rs := newTestRunnersWithIngestSpy(t, func(runnerID, provider, canonical string, raw []byte) error {
		got = append(got, canonical)
		return nil
	})
	sockPath := fakeAppServerEmitting(t, turnCompletedFixture(t))
	d := apiTransportTestAgentPointingAt(t, sockPath)

	conn, ok := rs.startAPIConn(context.Background(), "runner-1", d, models.TemplateCtx{Socket: sockPath})
	require.True(t, ok)
	rs.pumpAPIConn(context.Background(), "runner-1", "codex-api", conn) // Step 3 adds this

	require.Eventually(t, func() bool { return len(got) == 1 }, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, "turn_stop", got[0])
}

func TestStartAPIConn_DropsAnEventTheDescriptorDoesNotRouteThroughAPI(t *testing.T) {
	var got []string
	rs := newTestRunnersWithIngestSpy(t, func(runnerID, provider, canonical string, raw []byte) error {
		got = append(got, canonical)
		return nil
	})
	// A descriptor identical to codex-api.yaml but with turn_stop's transport
	// forced to "hooks" — the guard must drop it even though the driver resolved it.
	d := apiTransportTestAgentWithEventForcedToHooksTransport(t, "turn_stop")
	sockPath := fakeAppServerEmitting(t, turnCompletedFixture(t))
	conn, _ := rs.startAPIConn(context.Background(), "runner-1", d, models.TemplateCtx{Socket: sockPath})
	rs.pumpAPIConn(context.Background(), "runner-1", "codex-api", conn)

	time.Sleep(200 * time.Millisecond) // no Eventually — proving an ABSENCE needs a bounded wait, not a poll-until
	assert.Empty(t, got)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/app/usecases/chat/internal/runner/... -run TestStartAPIConn_Pumps -run TestStartAPIConn_Drops -v`
Expected: FAIL — `pumpAPIConn` does not exist.

- [ ] **Step 3: Implement**

```go
// pumpAPIConn forwards every canonical event this connection's driver resolves
// into the SAME ingest entrypoint hooks use — ownership, activity, and the
// answer desk need no transport-specific branch because of this. Runs until the
// driver's Events() channel closes (the connection died) or ctx is cancelled.
//
// agent (an engineagents.Agent, the SAME value spawnRunner already holds) is how
// this reaches TransportFor — never a raw *spec.Descriptor, which this package
// cannot name (see Task 11 Step 0).
func (rs *Runners) pumpAPIConn(ctx context.Context, runnerID, providerID string, agent engineagents.Agent, conn *apiconn) {
	go func() {
		for ev := range conn.driver.Events() {
			if agent.TransportFor(ev.Canonical) != "api" {
				// Declared on hooks by this descriptor — the hooks wire already
				// carries it, or will; a driver-resolved copy here would double it.
				continue
			}
			if err := rs.turns.IngestHook(ctx, runnerID, providerID, ev.Canonical, ev.Raw); err != nil {
				slog.WarnContext(ctx, "agent: api transport: ingest", "err", err,
					"runner_id", runnerID, "event", ev.Canonical)
			}
		}
	}()
}
```

Update Task 11's `spawnRunner` wiring to call `rs.pumpAPIConn(ctx, runnerID, providerID, descriptor, conn)` right after `startAPIConn` succeeds — `descriptor` there is already the `engineagents.Agent` value `spawnRunner` resolved earlier in the same function (`descriptor, err := rs.agents.Get(ctx, crowbarHome, providerID)`), so no new accessor is needed at that call site.

- [ ] **Step 4: Run to verify it passes**

Run: `cd api && go test ./internal/app/usecases/chat/internal/runner/... -v -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/chat/internal/runner/apiconn.go api/internal/app/usecases/chat/internal/runner/apiconn_test.go
git commit -m "feat(agents): pump api-transport events into the same ingest path as hooks, guarded by TransportFor"
```

---

### Task 13: Answer a permission from Crowbar's chat over the API transport

This is the headline capability (spec §5: "the one that has never worked for codex"). Per this plan's research, `answerdesk`/`AnswerChoice` are ALREADY fully transport-agnostic (`api/internal/app/usecases/chat/answers.go:129-175` renders via `agent.RenderAnswer` and calls `u.answers.Resolve(slot, stdout)` with no branch on how the slot is held) — so this task only needs to (a) hold the slot with a synthetic delivery id when the ask arrives over the API driver instead of an HTTP hook delivery, and (b) write the rendered decision back over the socket instead of returning it to an HTTP long-poll.

**Files:**
- Modify: `api/internal/app/usecases/chat/internal/runner/apiconn.go` (extend `pumpAPIConn`'s per-event handling for `AskID != nil`)
- Test: `api/internal/app/usecases/chat/internal/runner/apiconn_test.go`

**Interfaces:**
- Consumes: `rs.answers` (`*answerdesk.Desk`, already a field on `Runners`), `inflight.WithDeliveryID` (existing, used today by the journalled hook path — `api/internal/app/usecases/chat/internal/shared/inflight`), `rs.turns.IngestHook` (unchanged — an ask event still needs to open its `ActivityChoice` through the normal observation path before anyone can decide it).
- Produces: a permission asked over the API driver appears as an answerable choice in Crowbar's chat, and picking an option in the UI causes `conn.driver.Reply(askID, rendered)` to be called with the SAME bytes `agent.RenderAnswer` already produces for any other transport.

- [ ] **Step 1: Write the failing test — full round trip, human answers, provider gets the reply**

```go
func TestPumpAPIConn_AskEventIsAnswerableFromCrowbarAndRepliesOverTheSocket(t *testing.T) {
	replySeen := make(chan string, 1)
	sockPath := fakeAppServerAskingThenCapturingReply(t, "item/permissions/requestApproval",
		map[string]string{"tool": "shell"}, replySeen)

	rs, answerAPI := newTestRunnersWithRealAnswerDesk(t) // exposes AnswerChoice etc. for this test to call
	d := apiTransportTestAgentPointingAt(t, sockPath)
	conn, ok := rs.startAPIConn(context.Background(), "runner-1", d, models.TemplateCtx{Socket: sockPath})
	require.True(t, ok)
	rs.pumpAPIConn(context.Background(), "runner-1", "codex-api", d.Spec(), conn)

	// Wait for the choice to become pending in the chat's activity ledger — the
	// same observability path a real chat UI polls.
	var choiceID string
	require.Eventually(t, func() bool {
		choices, _ := rs.activityForTest("chat-1")
		if len(choices) == 1 {
			choiceID = choices[0].ID
			return true
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	require.NoError(t, answerAPI.AnswerChoice(context.Background(), "chat-1", choiceID, []string{"allow"}, "", nil))

	select {
	case reply := <-replySeen:
		assert.JSONEq(t, `{"decision":"approved"}`, reply)
	case <-time.After(2 * time.Second):
		t.Fatal("codex never received a reply over the socket")
	}
}
```

Building `newTestRunnersWithRealAnswerDesk` and `activityForTest` may require reusing fixtures/helpers already present in `runner_test.go`/`commands_test.go` in this package — read those before inventing new test scaffolding; this package already has integration-style tests wiring `Runners` to a real (in-memory) event store per the files read during this plan's research (`event_store_test.go`, `store_test.go`), so follow that existing harness rather than building a second one.

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/app/usecases/chat/internal/runner/... -run TestPumpAPIConn_AskEvent -v`
Expected: FAIL.

- [ ] **Step 3: Implement — extend `pumpAPIConn`**

```go
func (rs *Runners) pumpAPIConn(
	ctx context.Context, runnerID, providerID string, agent engineagents.Agent, conn *apiconn,
) {
	go func() {
		for ev := range conn.driver.Events() {
			if agent.TransportFor(ev.Canonical) != "api" {
				continue
			}
			evCtx := ctx
			if ev.AskID != nil {
				// A synthetic delivery id scopes this ask's slot exactly the way an
				// HTTP hook delivery's id would — see holdForAnswer in
				// internal/turn/observation.go, which reads this straight out of the
				// context and needs no other change to hold the prompt open.
				deliveryID := runnerID + ":" + string(ev.AskID)
				evCtx = inflight.WithDeliveryID(ctx, deliveryID)
			}
			if err := rs.turns.IngestHook(evCtx, runnerID, providerID, ev.Canonical, ev.Raw); err != nil {
				slog.WarnContext(ctx, "agent: api transport: ingest", "err", err,
					"runner_id", runnerID, "event", ev.Canonical)
				continue
			}
			if ev.AskID != nil {
				rs.awaitAndReplyOverSocket(ctx, runnerID, string(ev.AskID), conn)
			}
		}
	}()
}

// awaitAndReplyOverSocket blocks on the SAME answerdesk.Await an HTTP hook relay
// would, on this goroutine instead of an HTTP request — there is no relay
// process for the API transport, so this goroutine IS the thing waiting. On a
// verdict (or an expired budget, which yields an empty stdout exactly as it
// would for a hook relay nobody answered in time — the provider then falls back
// to answering through its own attached TUI, per capability 2), it writes the
// reply back over the socket.
func (rs *Runners) awaitAndReplyOverSocket(ctx context.Context, runnerID, askID string, conn *apiconn) {
	deliveryID := runnerID + ":" + askID
	answer, err := rs.answers.Await(ctx, deliveryID)
	if err != nil || len(answer.Stdout) == 0 {
		return // nobody answered from Crowbar in time; codex's own attached TUI still has it
	}
	if err := conn.driver.Reply([]byte(askID), answer.Stdout); err != nil {
		slog.WarnContext(ctx, "agent: api transport: write reply", "err", err, "runner_id", runnerID)
	}
}
```

Note the `ev.AskID` (`json.RawMessage`, e.g. `7` or `"abc"`) is reused both as (part of) the delivery id string AND, re-cast via `[]byte(askID)`, as the argument to `conn.driver.Reply` — confirm `apidriver.Driver.Reply(askID json.RawMessage, rendered []byte)`'s first parameter is exactly `json.RawMessage` (Task 10's signature) so this round-trips byte-identically to what the server sent, not a re-serialization of a parsed-and-reprinted number that could change `7` to `7.0` or similar. Adjust the delivery-id string encoding if `json.RawMessage`'s raw bytes contain characters unsafe for the rest of this codebase's delivery-id handling (read `inflight`'s existing delivery-id validation, if any, before finalizing the encoding — prefer hex-encoding `ev.AskID` into the delivery id string if there is any such constraint).

- [ ] **Step 4: Run to verify it passes**

Run: `cd api && go test ./internal/app/usecases/chat/internal/runner/... -v -race`
Expected: PASS.

- [ ] **Step 5: `go vet`, lint, full build**

Run: `cd api && go build ./... && go vet ./... && golangci-lint run ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add api/internal/app/usecases/chat/internal/runner/apiconn.go api/internal/app/usecases/chat/internal/runner/apiconn_test.go
git commit -m "feat(agents): answer API-transport permissions from Crowbar's chat, replying over the socket"
```

---

### Task 14: Merge codex into one descriptor; delete the experimental file

**Files:**
- Modify: `api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/codex.yaml`
- Delete: `api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/experimental/codex-api.yaml`
- Test: `api/internal/engine/agents/internal/protocol/internal/descriptor/fixture_test.go` (already exists and walks `experimental/` too — confirm it still passes once that directory is gone and the merged file carries the same events)
- Test: `api/internal/engine/agents/internal/protocol/internal/descriptor/v3_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1-13.
- Produces: `descriptor.Resolve(ctx, "", "codex")` returns a descriptor with `Runtime.Transport: "api"`, both `Runtime.API` and `Runtime.Hooks` populated, and per-event `Transport: "hooks"` on exactly `subagent_pre`, `subagent_post`, `compact_pre`, `compact_post`, `session_end`.

- [ ] **Step 1: Write the failing test**

```go
func TestCodexDescriptor_IsMergedMixedTransport(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), "", "codex")
	require.NoError(t, err)

	assert.Equal(t, "api", d.Runtime.Transport)
	assert.NotEmpty(t, d.Runtime.API.Serve)
	assert.NotEmpty(t, d.Runtime.API.Attach)
	assert.NotEmpty(t, d.Runtime.Hooks.Format, "hooks stay declared — the attached TUI still fires them")

	hooksOnly := []string{"subagent_pre", "subagent_post", "compact_pre", "compact_post", "session_end"}
	for _, name := range hooksOnly {
		assert.Equal(t, "hooks", d.TransportFor(name), "event %q must stay on hooks — the API does not carry it", name)
	}
	apiOnly := []string{"turn_stop", "message_delta", "permission", "elicitation", "telemetry", "interrupt", "compact_start"}
	for _, name := range apiOnly {
		assert.Equal(t, "api", d.TransportFor(name), "event %q must be on the api default", name)
	}
}

func TestExperimentalCodexAPIDescriptorIsGone(t *testing.T) {
	_, err := os.Stat("descriptors-v3/experimental/codex-api.yaml")
	assert.True(t, os.IsNotExist(err), "codex-api.yaml is merged into codex.yaml — it must not exist alongside it")
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/engine/agents/internal/protocol/internal/descriptor/... -run TestCodexDescriptor_IsMergedMixedTransport -run TestExperimentalCodexAPIDescriptorIsGone -v`
Expected: FAIL.

- [ ] **Step 3: Merge the descriptor**

Rewrite `codex.yaml`'s `runtime:` and `events:` blocks per spec §2.1:

```yaml
runtime:
  transport: api
  hotswap: true
  api:
    protocol: jsonrpc2
    serve:  [codex, app-server, --listen, "unix://{socket}"]
    attach: [codex, --remote, "unix://{socket}"]
    handshake: { call: initialize }
  hooks:
    format: json
    delivery: http
    require_payload_fields: [transcript_path]
  spawn:
    cmd: codex

events:
  session_start:
    in: thread/started
    map: { session_id: thread.id }

  user_prompt:
    in: item/started
    when: { item.type: userMessage }
    map:
      session_id: threadId
      message:    item.content[type=text].text

  turn_stop:
    in: turn/completed
    map:
      session_id: threadId
      message:    turn.items[type=agentMessage].text

  tool_pre:
    in: item/started
    when: { item.type: commandExecution || fileChange || mcpToolCall }
    map:
      session_id: threadId
      tool_id:    item.id
      tool_name:  item.type
      tool_input: item

  tool_post:
    in: item/completed
    when: { item.type: commandExecution || fileChange || mcpToolCall }
    map:
      session_id:  threadId
      tool_id:     item.id
      tool_name:   item.type
      tool_result: item.content

  message_delta:
    in: item/agentMessage/delta
    map:
      session_id: threadId
      message_id: itemId
      text:       delta
      turn_id:    turnId

  telemetry:
    in: thread/tokenUsage/updated
    map:
      session_id:              threadId
      input_tokens:            tokenUsage.total.inputTokens
      output_tokens:           tokenUsage.total.outputTokens
      context.used_tokens:     tokenUsage.total.totalTokens
      context.capacity_tokens: tokenUsage.modelContextWindow

  permission:
    ask: item/permissions/requestApproval
    timeout_seconds: 270
    map:
      tool_name:  tool
      tool_input: params
    reply:
      allow: '{"decision":"approved"}'
      deny:  '{"decision":"denied","message":{reason_json}}'

  elicitation:
    ask: mcpServer/elicitation/request
    timeout_seconds: 270
    map:
      message: message
      schema:  requestedSchema
    reply:
      accept:  '{"action":"accept","content":{content_json}}'
      decline: '{"action":"decline"}'
      cancel:  '{"action":"cancel"}'

  prompt:
    out: turn/start
    send:
      threadId: "{session_id}"
      text:     "{text}"

  interrupt:
    out: turn/interrupt
    send: { threadId: "{session_id}" }

  compact_start:
    out: thread/compact/start
    send: { threadId: "{session_id}" }

  subagent_pre:
    transport: hooks
    in: SubagentStart
    map: { session_id: session_id, subagent_id: agent_id, agent_type: agent_type }

  subagent_post:
    transport: hooks
    in: SubagentStop
    map: { session_id: session_id, subagent_id: agent_id, agent_type: agent_type }

  compact_pre:
    transport: hooks
    in: PreCompact
    map: { session_id: session_id, trigger: trigger }

  compact_post:
    transport: hooks
    in: PostCompact
    map: { session_id: session_id, trigger: trigger }

  session_end:
    transport: hooks
    in: SessionEnd
    map: { session_id: session_id, reason: reason }

catalog:
  models:   { call: model/list }
  commands: { call: skills/list }
  limits:   { call: account/rateLimits/read }

inject:
  - at: mcp
    call: config/mcpServer/reload
  - at: context
    call: thread/inject_items
    send:
      threadId: "{session_id}"
      text:     "{context}"
```

Keep every line below the (former) `# --- carried across from v2 unchanged ---` marker in the current `codex.yaml` (spawn/session/presentation/model/effort/config_injection/mcp_injection/context_inject/resume_context_inject) EXACTLY as-is — none of that is transport-specific. Update the header comment (lines 1-8) to describe the merged state instead of "not used here", and keep the hook-injection `config_injection` entries (`hooks.SessionStart=[...]` etc.) exactly as they are today — the attached TUI still needs them for the five hooks-only events, per spec §2.1: "The hook injection codex.yaml already performs... stays: it is what makes the attached TUI report those five."

Per this plan's own live-verified correction, decide `terminal_prompts`/`terminal_notices` **conservatively**: keep them for now. Task 15's live verification includes an explicit check for whether the API transport actually reports the trust modal and usage-limit banner structurally; only remove `terminal_prompts`/`terminal_notices` in a follow-up once that is confirmed, per this plan's stated correction to spec §2.1 ("can be deleted", not "must be" — the spec itself already hedges this).

- [ ] **Step 4: Delete the experimental descriptor and its now-redundant fixture-walk dependency**

```bash
git rm api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/experimental/codex-api.yaml
```

Leave `api/internal/engine/agents/internal/protocol/testdata/fixtures/codex-api/` in place — Task 8/9/10's tests still read fixtures from there by relative path, and `fixture_test.go`'s replay walks `descriptors-v3/` (now just `claude.yaml` + `codex.yaml`, no `experimental/` subdirectory left to walk) and looks up fixtures by **provider id**, which is `codex`, not `codex-api` — so update `fixture_test.go`'s fixture lookup for the merged `codex` descriptor to also check `testdata/fixtures/codex-api/` (the directory name stays as the historical capture-script default) when `testdata/fixtures/codex/` has no matching file, OR simplest: rename the fixture directory itself to `testdata/fixtures/codex/` to match the merged descriptor's id and update every path this plan's Tasks 8-10 tests reference accordingly. Prefer the rename — it removes the id mismatch permanently rather than special-casing it in the loader.

```bash
git mv api/internal/engine/agents/internal/protocol/testdata/fixtures/codex-api api/internal/engine/agents/internal/protocol/testdata/fixtures/codex
```

Update the relative paths in Task 8/9/10's test files (`../../testdata/fixtures/codex-api/...` → `.../codex/...`) accordingly.

- [ ] **Step 5: Run every descriptor and protocol test**

Run: `cd api && go test ./internal/engine/agents/... -v`
Expected: PASS, including `TestV3Descriptors_ResolveAgainstRecordedTraffic` (now exercising `codex`'s merged event table against the renamed fixture directory) and `TestV3Descriptors_ReplyTemplatesAreValidJSON`.

- [ ] **Step 6: Full build/vet/lint**

Run: `cd api && go build ./... && go vet ./... && golangci-lint run ./...`
Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/codex.yaml
git rm api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/experimental/codex-api.yaml
git add api/internal/engine/agents/internal/protocol/testdata/fixtures/
git add api/internal/engine/agents/internal/protocol/internal/descriptor/fixture_test.go api/internal/engine/agents/internal/protocol/internal/descriptor/v3_test.go
git add api/internal/engine/agents/internal/protocol/internal/apidriver/ api/internal/engine/agents/internal/protocol/internal/dispatch/
git commit -m "feat(codex): merge codex into one mixed-transport descriptor; delete the experimental file"
```

---

### Task 15: Live-verify Capability 1 end-to-end in the Tauri desktop app

This is the task that resolves the one open empirical question this plan flags in its "Corrections" section: whether `codex --remote` attaches to the thread `serve` already opened. **Do not report Capability 1 complete until this task's Step 2 has an answer, whichever way it comes out.**

**Files:** none (verification only, plus a possible follow-up task filed against whatever Step 2 finds).

- [ ] **Step 1: Rebuild and launch the dev desktop app** against the worktree's isolated `CROWBAR_HOME`, per [[feedback_verify_via_dev_desktop_not_healless]] / [[project_dev_home_isolation]].

Run: `make dev-desktop` (confirm target name first).

- [ ] **Step 2: Start a codex chat, run one turn, and via the Tauri MCP bridge confirm — in this order:**
  1. The compact button appears (it did not before this plan's work).
  2. The context gauge fills and rate limits appear.
  3. The assistant's answer **streams** rather than landing whole.
  4. Switch to the Terminal surface mid-turn (send a second prompt first, then switch while `working` is true) — confirm the attached TUI shows **the same conversation**, not a fresh one. **This directly answers the open question from this plan's Corrections section — record the answer explicitly, and if it shows a fresh conversation instead, do not mark this plan's headline claim done; file the gap instead of asserting success.**
  5. Trigger a real permission prompt (ask codex to run a shell command in a fresh chat, before granting broad approval) and answer it **from Crowbar's chat UI** — assert the CLI actually proceeds (this is the capability that has never worked for codex before this plan).
  6. Send Stop while a turn is running and confirm it sends a real interrupt (the CLI's turn actually stops) rather than killing the runner.

- [ ] **Step 3: Prove the two wires really are concurrent, not accidentally serialized** — with `~/.codex/hooks.json` present (as it was during this plan's own live probe), trigger a subagent (or, if none is easy to trigger by hand, at minimum confirm via daemon logs that both a hooks-delivered event and an api-driver-delivered event reached the SAME chat's ledger during one turn, in order, with no duplicate `turn_stop`).

- [ ] **Step 4: Report results to the user, explicitly including the Step 2.4 answer and whether `terminal_prompts`/`terminal_notices` proved safe to delete** (per Task 14's deliberate conservatism) — this is the final checkpoint for the whole spec, matching its own §5 verification list almost line for line.

---

## Order of work

Follows spec §4 exactly: Part A (Tasks 1-7) ships first and independently. Within Part B, Tasks 8-9 (wsrpc, dispatch) have no dependency on each other and could run in parallel if using subagent-driven-development; Task 10 depends on both; Tasks 11-13 are sequential (spawn → route → answer); Task 14 depends on all of 8-13; Task 15 depends on 14.
