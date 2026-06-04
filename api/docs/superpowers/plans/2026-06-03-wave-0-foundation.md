# Wave 0 — Backend Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the complete skeleton of the Crowbar Go backend (core/domain/adapter/app/api layers, Asynx event-sourcing, the generic `Broadcaster[T]`, and 4-container DI wiring) so every later wave has a solid, spec-conformant foundation.

**Architecture:** Layered `engine → adapter → app → api` containers wired by `internal/internal.go`, mirroring `quiver.core`. Aggregates with state machines (Workspace, Chat, AgentRun, ReviewThread) are event-sourced via Asynx (one SQLite event-store file each); plain CRUD entities (Project, Repository, TerminalProfile) use a single GORM DB. Live data reaches clients through a typed `WebSocketHub` fan-out to generic `Broadcaster[T]` instances.

**Tech Stack:** Go 1.26.2, `github.com/char2cs/asynx` v0.6.2, `gorilla/websocket`, `gin-gonic/gin`, `glebarez/sqlite` + `gorm.io/gorm`, `creack/pty`, `stretchr/testify`, cobra.

---

## Reference Implementation

Every infrastructure pattern mirrors `quiver.core` at `/Users/char2cs/Projects/Rabbyte/quiver.core`. The exact files to copy-adapt are cited per task. **Read the cited quiver file before writing each Crowbar equivalent.**

## Non-Negotiable Standards (a reviewer checks every one)

1. **One type per file**, named `snake_case` after the type. One `_test.go` per source file (except struct-only files). Source files < 500 LOC.
2. **One parameter per line** — signatures AND multi-arg calls; closing paren on its own line.
3. **Early returns always; no `else`.** Max 2 indentation levels per function (level 3 must never exist — extract a method).
4. **Coverage ≥95%** on new packages. **No `time.Sleep` in tests, ever** — commands take an injected `Now time.Time` so they stay pure and tests stay deterministic. Synchronize WS tests with `Broadcaster.WaitRegistered()`.
5. **Errors:** `fmt.Errorf("op: ctx: %w", err)`. gofumpt + goimports clean. `.golangci.yml` enforces funlen 100/50, gocyclo 15, nestif ≤2, revive early-return.
6. **Layering:** lower layers never import higher. `api → app → {adapter, engine}`; `domain` and `core` import nothing internal above them.
7. **No inline comments. No doc comments on unexported symbols** (names must be self-evident). Exported symbols get detailed doc comments.

## File Structure (what gets created)

```
api/
  .golangci.yml                                  enforce standards from line 1
  Makefile                                       test/lint/bench/coverage targets
  go.mod / go.sum                                + asynx, websocket, pty, testify; go 1.26.2
  cmd/crowbar/main.go                            (adjust — WithHomeDir plumb, drop fixtures)
  internal/internal.go                           root container: engine→adapter→app→api
  internal/core/
    metadata/{metadata.go,metadata.yaml,osvalue.go,resolve_home.go,resolve_home_windows.go}
    paths/paths.go                               Events/Store/Runs/Logs (+ ...At for tests)
    config/{config.go,default.yaml}              embedded defaults + intelligence→model map
  internal/domain/
    project.go repository.go terminal_profile.go                  (GORM)
    workspace.go workspace_status.go merge_strategy.go pending_merge.go   (Asynx aggregate)
    chat.go chat_status.go                                        (Asynx aggregate)
    agent_run.go agent_run_status.go                              (Asynx aggregate)
    review_thread.go review_thread_status.go                      (Asynx aggregate)
  internal/adapter/
    container.go                                 event stores (4) + GORM DB
    eventstore/sqlite/event_store.go             one SQLite file per aggregate
    store/sqlite/sqlite.go                       generic GORM Store[T,K] + OpenDB
    store/store.go                               Store[T,K] interface
  internal/app/
    container.go                                 build asynx + db + hub + repos + recovery
    asynx.go                                      newAsynx[T] helper (8 shards/queue 1000)
    hub/hub.go hub/web_socket_hub.go hub/subscriber.go hub/chat_status_event.go
    repositories/
      container.go                               build 4 repos, recovery, RegisterHubProjections stub
      recovery.go                                AgentRun running→error + idempotent chat reconcile
      workspace/workspace.go workspace/internal/commands/{create.go,sync_working_tree_state.go}
      agentrun/agentrun.go  agentrun/internal/commands/{create.go,mark_running.go,complete.go,fail.go}
      chat/chat.go          chat/internal/commands/{create.go,reset_idle.go}
      reviewthread/reviewthread.go reviewthread/internal/commands/{open.go,resolve.go,reopen.go}
  internal/api/
    container.go                                 gin engine, middleware, mount v0
    middleware/{logging.go,recovery.go}          (keep; delete chaos.go)
    static.go                                    (keep)
    v0/
      container.go                               holds broadcasters + svc refs
      router.go                                  REST routes + dispatch() wiring
      health.go                                  GET /v0/health
      dispatch.go                                REST+WS dual-serve helper
      ws/broadcaster.go ws/client.go ws/filter.go ws/stream_def.go
```

**Deleted in Phase 0:** `internal/fixtures/`, `internal/wshub/`, `cmd/fixtures-gen/`, all `internal/api/v0/*_handler.go` + `events.go` + old `router.go`, `internal/api/middleware/chaos.go`, `internal/app/hub/hub.go` (old), `internal/app/usecases/`, old `internal/domain/{project,workspace,domain_test}.go`, old `internal/adapter/store/sqlite/store.go`, `internal/api/v0/health_test.go` (regenerated), `coverage.html`, `coverage.out`, `crowbar` binary.

---

## Phase 0 — Demolition & Tooling

Stand up the standards-enforcement tooling *first*, then delete the old scaffold, so every new file is linted from line one.

### Task 1: go.mod dependencies & Go version

**Files:**
- Modify: `api/go.mod`

- [ ] **Step 1: Add dependencies and bump the Go version**

Run these from `api/`:

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-resonant-niobium-4734/api
go mod edit -go=1.26.2
go get github.com/char2cs/asynx@v0.6.2
go get github.com/gorilla/websocket@v1.5.3
go get github.com/creack/pty@v1.1.21
go get github.com/stretchr/testify@v1.9.0
go mod tidy
```

(Skip `acp-go-sdk` and `mcp-go` — bridge/chat, deferred to later waves.)

- [ ] **Step 2: Verify the module resolves**

Run: `go build ./... 2>&1 | head -20`
Expected: compiles (old scaffold still present — that's fine; we delete it next). `gorilla/websocket` moves from `// indirect` to a direct require.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "build: add asynx, websocket, pty, testify; go 1.26.2"
```

### Task 2: golangci config & Makefile

**Files:**
- Create: `api/.golangci.yml`
- Modify: `api/Makefile`

- [ ] **Step 1: Write `.golangci.yml`** (mirror quiver's, with crowbar local-prefix)

```yaml
version: "2"
run:
  modules-download-mode: readonly
  issues-exit-code: 1
  tests: true
output:
  formats:
    text:
      path: stdout
      print-linter-name: true
      print-issued-lines: true
linters:
  default: none
  enable:
    - bodyclose
    - errcheck
    - exhaustive
    - funlen
    - gochecknoinits
    - gocyclo
    - goprintffuncname
    - gosec
    - govet
    - ineffassign
    - nakedret
    - nestif
    - nolintlint
    - revive
    - rowserrcheck
    - staticcheck
    - unconvert
    - unused
    - whitespace
  settings:
    exhaustive:
      default-signifies-exhaustive: false
    funlen:
      lines: 100
      statements: 50
    gocyclo:
      min-complexity: 15
    nestif:
      min-complexity: 2
    nolintlint:
      require-explanation: false
      require-specific: false
      allow-unused: false
    revive:
      enable-all-rules: false
      rules:
        - name: early-return
          severity: warning
          disabled: false
  exclusions:
    generated: lax
    rules:
      - linters:
          - funlen
          - gocyclo
        path: _test\.go
      - linters:
          - errcheck
          - gosec
        path: _test\.go
      - linters:
          - gosec
        text: 'G404:'
    paths:
      - third_party$
      - builtin$
      - examples$
issues:
  max-issues-per-linter: 0
  max-same-issues: 0
  new: false
  fix: false
severity:
  default: error
formatters:
  enable:
    - gofumpt
    - goimports
  settings:
    gofumpt:
      extra-rules: true
    goimports:
      local-prefixes:
        - github.com/char2cs/crowbar
  exclusions:
    generated: lax
    paths:
      - third_party$
      - builtin$
      - examples$
```

- [ ] **Step 2: Rewrite the `Makefile`** (keep `embed-web`/`dev`/`build`; add standards targets)

```makefile
.PHONY: dev build test test-integration bench test-coverage lint pr-checks missing-tests embed-web

embed-web:
	@test -d ../web/dist || (echo "ERROR: web/dist not found. Run 'cd ../web && bun run build' first." && exit 1)
	rm -rf cmd/crowbar/web/dist
	mkdir -p cmd/crowbar/web
	cp -r ../web/dist cmd/crowbar/web/dist

dev:
	go run -tags noEmbed ./cmd/crowbar serve

build: embed-web
	go build -o bin/crowbar ./cmd/crowbar

test:
	go test -race ./...

test-integration:
	go test -tags integration -race ./tests/...

bench:
	go test -run=^$$ -bench=. -benchmem ./...

test-coverage:
	@go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | awk '/^total:/{gsub("%",""); if ($$3+0 < 95) {print "Coverage " $$3 "% is below 95% threshold"; exit 1} else {print "Coverage " $$3 "% OK"}}'
	@rm -f coverage.out

lint:
	golangci-lint run ./...

pr-checks: lint test test-coverage
	go vet ./...

missing-tests:
	@for f in $$(find internal cmd -name '*.go' ! -name '*_test.go'); do \
		t=$${f%.go}_test.go; \
		if [ ! -f "$$t" ] && ! grep -qL 'type .* struct' "$$f" 2>/dev/null; then \
			echo "missing test: $$f"; \
		fi; \
	done
```

- [ ] **Step 3: Verify lint runs** (golangci-lint must be installed: `brew install golangci-lint`)

Run: `cd api && make lint 2>&1 | head -20`
Expected: runs (may report issues on the soon-to-be-deleted old scaffold — acceptable for now).

- [ ] **Step 4: Commit**

```bash
git add .golangci.yml Makefile
git commit -m "build: golangci config + standards Makefile targets"
```

### Task 3: Delete the old scaffold

**Files:**
- Delete: see command below.

- [ ] **Step 1: Remove non-conforming scaffold**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-resonant-niobium-4734/api
git rm -r internal/fixtures internal/wshub cmd/fixtures-gen
git rm internal/api/v0/branch_review_handler.go internal/api/v0/branch_review_handler_test.go \
       internal/api/v0/conversations_handler.go internal/api/v0/events.go \
       internal/api/v0/flows_handler.go internal/api/v0/fs_file_handler.go \
       internal/api/v0/fs_handler.go internal/api/v0/git_handler.go \
       internal/api/v0/markdown_chat_handler.go internal/api/v0/projects_handler.go \
       internal/api/v0/terminal_handler.go internal/api/v0/workspaces_handler.go \
       internal/api/v0/workspaces_handler_test.go internal/api/v0/router.go \
       internal/api/v0/health_test.go \
       internal/api/v0/ws_chat_handler.go internal/api/v0/ws_daemon_handler.go \
       internal/api/v0/ws_files_handler.go internal/api/v0/ws_git_handler.go \
       internal/api/v0/ws_terminal_handler.go internal/api/v0/ws_workspaces_handler.go
git rm internal/api/middleware/chaos.go internal/api/middleware/chaos_test.go
git rm internal/app/hub/hub.go internal/app/hub/hub_test.go
git rm -r internal/app/usecases
git rm internal/domain/project.go internal/domain/workspace.go internal/domain/domain_test.go
git rm internal/adapter/store/sqlite/store.go internal/adapter/store/sqlite/store_test.go
git rm internal/api/v0/health.go
git rm -f coverage.html coverage.out crowbar 2>/dev/null || true
```

> Keep: `internal/core/gateway/`, `internal/api/static.go`, `internal/api/middleware/{logging,recovery}.go`, `internal/engine/container.go`, `cmd/crowbar/{main,web_embed,web_noembed}.go`.

- [ ] **Step 2: Confirm no `rabbytesoftware/*` imports remain** (already migrated, this is a guard)

Run: `grep -rl "rabbytesoftware" --include="*.go" . || echo "clean"`
Expected: `clean`

- [ ] **Step 3: Expect a broken build** (intentional — we rebuild in later phases)

Run: `go build ./... 2>&1 | head`
Expected: errors referencing deleted packages from `internal/api`, `internal/app`, `internal/internal.go`. These are fixed in Phases 1–7. Do **not** try to patch them now.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: demolish non-spec scaffold (fixtures, SSE, flow/task handlers)"
```

---

## Phase 1 — `core/` (metadata, paths, config)

Lowest layer, imports nothing internal. Build and test in isolation. Mirrors `quiver.core/internal/core/{metadata,paths,config}`.

### Task 4: metadata package

**Files:**
- Create: `api/internal/core/metadata/osvalue.go`
- Create: `api/internal/core/metadata/resolve_home.go`
- Create: `api/internal/core/metadata/resolve_home_windows.go`
- Create: `api/internal/core/metadata/metadata.yaml`
- Create: `api/internal/core/metadata/metadata.go`
- Test: `api/internal/core/metadata/metadata_test.go`

Reference: `quiver.core/internal/core/metadata/{metadata.go,osvalue.go,resolve_home.go,metadata.yaml}`.

- [ ] **Step 1: Write `osvalue.go`** (OS-keyed value with default)

```go
package metadata

import "runtime"

type OsValue[T any] struct {
	Default T            `yaml:"default"`
	OS      map[string]T `yaml:"os"`
}

func (v OsValue[T]) Resolve() T {
	if v.OS == nil {
		return v.Default
	}
	if val, ok := v.OS[runtime.GOOS]; ok {
		return val
	}
	return v.Default
}
```

- [ ] **Step 2: Write `resolve_home.go`** (non-Windows home expansion)

```go
//go:build !windows

package metadata

import (
	"os"
	"path/filepath"
	"strings"
)

func resolveHome() string {
	home := Get().Paths.Home.Resolve()
	if !strings.HasPrefix(home, "~") {
		return home
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return home
	}
	return filepath.Join(userHome, strings.TrimPrefix(home, "~"))
}
```

- [ ] **Step 3: Write `resolve_home_windows.go`**

```go
//go:build windows

package metadata

import "os"

func resolveHome() string {
	home := Get().Paths.Home.Resolve()
	return os.ExpandEnv(home)
}
```

- [ ] **Step 4: Write `metadata.yaml`** (crowbar identity + path templates incl. `runs`)

```yaml
version:
  number: 0.1.0
  codename: "Crowbar"

metadata:
  name: Crowbar
  description: Local, single-user agentic development platform.
  author: char2cs
  url: https://char2cs.net
  license: proprietary
  copyright: Copyright 2026 char2cs.net

paths:
  home:
    default: "~/.crowbar"
    os:
      windows: 'C:\Users\{{USER}}\Documents\.crowbar'
  events: "{{home}}/state/events"
  store: "{{home}}/state/store"
  runs: "{{home}}/runs"
  config: "{{home}}/config.yaml"
  logs: "{{home}}/logs"
```

- [ ] **Step 5: Write `metadata.go`** (embedded singleton + path getters with `...At` variants)

```go
package metadata

import (
	_ "embed"
	"path/filepath"
	"strings"
	"sync"

	yaml "gopkg.in/yaml.v3"
)

var (
	//go:embed metadata.yaml
	metadataByte []byte
	metadata     *Metadata
	once         sync.Once
)

type MetadataInfo struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Author      string `yaml:"author"`
	URL         string `yaml:"url"`
	License     string `yaml:"license"`
	Copyright   string `yaml:"copyright"`
}

type Version struct {
	Number   string `yaml:"number"`
	Codename string `yaml:"codename"`
}

type Paths struct {
	Home   OsValue[string] `yaml:"home"`
	Events string          `yaml:"events"`
	Store  string          `yaml:"store"`
	Runs   string          `yaml:"runs"`
	Config string          `yaml:"config"`
	Logs   string          `yaml:"logs"`
}

type Metadata struct {
	Version  Version      `yaml:"version"`
	Metadata MetadataInfo `yaml:"metadata"`
	Paths    Paths        `yaml:"paths"`
}

func Get() *Metadata {
	once.Do(func() {
		metadata = &Metadata{}
		if err := yaml.Unmarshal(metadataByte, metadata); err != nil {
			metadata = defaultMetadata()
		}
	})
	return metadata
}

func GetVersion() string {
	return Get().Version.Number
}

func GetEventsPath() string {
	return resolvePath(Get().Paths.Events, resolveHome())
}

func GetEventsPathAt(homeDir string) string {
	return resolvePath(Get().Paths.Events, homeDir)
}

func GetStorePath() string {
	return resolvePath(Get().Paths.Store, resolveHome())
}

func GetStorePathAt(homeDir string) string {
	return resolvePath(Get().Paths.Store, homeDir)
}

func GetRunsPath() string {
	return resolvePath(Get().Paths.Runs, resolveHome())
}

func GetRunsPathAt(homeDir string) string {
	return resolvePath(Get().Paths.Runs, homeDir)
}

func GetConfigPath() string {
	return resolvePath(Get().Paths.Config, resolveHome())
}

func GetLogsPath() string {
	return resolvePath(Get().Paths.Logs, resolveHome())
}

func GetLogsPathAt(homeDir string) string {
	return resolvePath(Get().Paths.Logs, homeDir)
}

func resolvePath(
	tmpl string,
	home string,
) string {
	return filepath.FromSlash(strings.ReplaceAll(tmpl, "{{home}}", home))
}

func resetForTesting() {
	metadata = nil
	once = sync.Once{}
}

func defaultMetadata() *Metadata {
	return &Metadata{
		Version: Version{Number: "0.1.0", Codename: "Crowbar"},
		Metadata: MetadataInfo{
			Name:        "Crowbar",
			Description: "Local, single-user agentic development platform.",
			Author:      "char2cs",
		},
		Paths: Paths{
			Home:   OsValue[string]{Default: "~/.crowbar"},
			Events: "{{home}}/state/events",
			Store:  "{{home}}/state/store",
			Runs:   "{{home}}/runs",
			Config: "{{home}}/config.yaml",
			Logs:   "{{home}}/logs",
		},
	}
}
```

> `gopkg.in/yaml.v3` is already an indirect dep (via goccy? no — add explicitly). Run `go get gopkg.in/yaml.v3` if `go build` reports it missing.

- [ ] **Step 6: Write `metadata_test.go`**

```go
package metadata

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGet_LoadsEmbeddedMetadata(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	m := Get()
	assert.Equal(t, "Crowbar", m.Metadata.Name)
	assert.Equal(t, "0.1.0", m.Version.Number)
}

func TestGetEventsPathAt_RootsAtHomeDir(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	got := GetEventsPathAt("/tmp/crowtest")
	assert.True(t, strings.HasPrefix(got, "/tmp/crowtest"))
	assert.True(t, strings.HasSuffix(got, "state/events"))
}

func TestGetRunsPathAt_RootsAtHomeDir(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	got := GetRunsPathAt("/tmp/crowtest")
	assert.Equal(t, "/tmp/crowtest/runs", got)
}

func TestOsValue_Resolve_FallsBackToDefault(t *testing.T) {
	v := OsValue[string]{Default: "d", OS: map[string]string{"plan9": "x"}}
	assert.Equal(t, "d", v.Resolve())
}

func TestGetVersion_NonEmpty(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	assert.NotEmpty(t, GetVersion())
}
```

- [ ] **Step 7: Run tests**

Run: `go test ./internal/core/metadata/...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/core/metadata
git commit -m "feat(core): metadata package with crowbar identity + path templates"
```

### Task 5: paths package

**Files:**
- Create: `api/internal/core/paths/paths.go`
- Test: `api/internal/core/paths/paths_test.go`

Reference: `quiver.core/internal/core/paths/paths.go`.

- [ ] **Step 1: Write `paths.go`** (lazy mkdir with per-path mutex; `Events/Store/Runs/Logs` + `...At`)

```go
// Package paths resolves named Crowbar directories from metadata, creates them
// on demand with a per-path mutex, and returns their absolute paths.
package paths

import (
	"fmt"
	"os"
	"sync"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

var mu sync.Map

func ensure(
	path string,
) (string, error) {
	v, _ := mu.LoadOrStore(path, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	defer m.Unlock()
	if err := os.MkdirAll(path, 0o750); err != nil {
		return "", fmt.Errorf("paths: create %q: %w", path, err)
	}
	return path, nil
}

// Events returns the event-store directory, creating it if absent.
func Events() (string, error) {
	return ensure(metadata.GetEventsPath())
}

// EventsAt returns the event-store directory rooted at homeDir, creating it if absent.
func EventsAt(
	homeDir string,
) (string, error) {
	return ensure(metadata.GetEventsPathAt(homeDir))
}

// Store returns the GORM read-model directory, creating it if absent.
func Store() (string, error) {
	return ensure(metadata.GetStorePath())
}

// StoreAt returns the GORM read-model directory rooted at homeDir, creating it if absent.
func StoreAt(
	homeDir string,
) (string, error) {
	return ensure(metadata.GetStorePathAt(homeDir))
}

// Runs returns the agent-run artifacts directory, creating it if absent.
func Runs() (string, error) {
	return ensure(metadata.GetRunsPath())
}

// RunsAt returns the agent-run artifacts directory rooted at homeDir, creating it if absent.
func RunsAt(
	homeDir string,
) (string, error) {
	return ensure(metadata.GetRunsPathAt(homeDir))
}

// Logs returns the logs directory, creating it if absent.
func Logs() (string, error) {
	return ensure(metadata.GetLogsPath())
}

// LogsAt returns the logs directory rooted at homeDir, creating it if absent.
func LogsAt(
	homeDir string,
) (string, error) {
	return ensure(metadata.GetLogsPathAt(homeDir))
}
```

- [ ] **Step 2: Write `paths_test.go`** (use `t.TempDir()` for isolation)

```go
package paths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventsAt_CreatesDir(t *testing.T) {
	home := t.TempDir()
	got, err := EventsAt(home)
	require.NoError(t, err)
	info, statErr := os.Stat(got)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
	assert.Equal(t, filepath.Join(home, "state", "events"), got)
}

func TestStoreAt_CreatesDir(t *testing.T) {
	home := t.TempDir()
	got, err := StoreAt(home)
	require.NoError(t, err)
	_, statErr := os.Stat(got)
	require.NoError(t, statErr)
}

func TestRunsAt_CreatesDir(t *testing.T) {
	home := t.TempDir()
	got, err := RunsAt(home)
	require.NoError(t, err)
	_, statErr := os.Stat(got)
	require.NoError(t, statErr)
}

func TestLogsAt_CreatesDir(t *testing.T) {
	home := t.TempDir()
	got, err := LogsAt(home)
	require.NoError(t, err)
	_, statErr := os.Stat(got)
	require.NoError(t, statErr)
}

func TestEnsure_RejectsUncreatablePath(t *testing.T) {
	_, err := ensure("/dev/null/cannot")
	assert.Error(t, err)
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/core/paths/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/core/paths
git commit -m "feat(core): paths package with lazy mkdir + test isolation"
```

### Task 6: config package

**Files:**
- Create: `api/internal/core/config/default.yaml`
- Create: `api/internal/core/config/config.go`
- Test: `api/internal/core/config/config_test.go`

Reference: `quiver.core/internal/core/config/config.go`.

- [ ] **Step 1: Write `default.yaml`** (intelligence-tier → model map)

```yaml
config:
  intelligence:
    light: "claude-haiku-4-5"
    medium: "claude-sonnet-4-6"
    heavy: "claude-opus-4-8"
```

- [ ] **Step 2: Write `config.go`** (embedded defaults overlaid by `~/.crowbar/config.yaml`; `ModelForTier`)

```go
package config

import (
	_ "embed"
	"os"
	"path/filepath"
	"sync"

	yaml "gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

var (
	//go:embed default.yaml
	defaultConfigByte []byte
	config            *Config
	once              sync.Once
)

type Intelligence struct {
	Light  string `yaml:"light"`
	Medium string `yaml:"medium"`
	Heavy  string `yaml:"heavy"`
}

type ConfigData struct {
	Intelligence Intelligence `yaml:"intelligence"`
}

type Config struct {
	Config ConfigData `yaml:"config"`
}

// Get returns the singleton config: embedded defaults overlaid by the user's
// config.yaml at metadata.GetConfigPath(). Absent user fields keep defaults.
func Get() *Config {
	once.Do(func() {
		config = getDefaultConfig()
		configBytes, err := os.ReadFile(filepath.Clean(metadata.GetConfigPath()))
		if err != nil {
			return
		}
		if err := yaml.Unmarshal(configBytes, config); err != nil {
			config = getDefaultConfig()
		}
	})
	return config
}

// GetIntelligence returns the configured intelligence-tier → model mapping.
func GetIntelligence() Intelligence {
	return Get().Config.Intelligence
}

// ModelForTier maps an intelligence tier name to its model id, "" if unknown.
func ModelForTier(
	tier string,
) string {
	i := GetIntelligence()
	switch tier {
	case "light":
		return i.Light
	case "medium":
		return i.Medium
	case "heavy":
		return i.Heavy
	default:
		return ""
	}
}

func getDefaultConfig() *Config {
	cfg := &Config{}
	if err := yaml.Unmarshal(defaultConfigByte, cfg); err != nil {
		panic("config: failed to parse embedded default.yaml: " + err.Error())
	}
	return cfg
}

func resetForTesting() {
	config = nil
	once = sync.Once{}
}
```

- [ ] **Step 3: Write `config_test.go`**

```go
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelForTier_KnownTiers(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	assert.Equal(t, "claude-haiku-4-5", ModelForTier("light"))
	assert.Equal(t, "claude-sonnet-4-6", ModelForTier("medium"))
	assert.Equal(t, "claude-opus-4-8", ModelForTier("heavy"))
}

func TestModelForTier_UnknownTier_ReturnsEmpty(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	assert.Equal(t, "", ModelForTier("nonexistent"))
}

func TestGetIntelligence_FromEmbeddedDefaults(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	i := GetIntelligence()
	assert.NotEmpty(t, i.Medium)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/core/config/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/config
git commit -m "feat(core): config with embedded defaults + intelligence→model map"
```

---

## Phase 2 — `domain/` (aggregate types + enums)

One type per file. GORM structs get `gorm` tags + `TableName()`. Asynx aggregates are plain JSON-serializable structs (asynx marshals them). All IDs are UUID strings (`00` §3). Struct-only files need **no** `_test.go`; the one behavioral helper (`Workspace.HasLiveAgentRun` predicate is computed in the projection layer, not here) means these files are pure data — tests live with the commands that mutate them (Phase 4).

### Task 7: GORM entity types + shared enums

**Files:**
- Create: `api/internal/domain/project.go`
- Create: `api/internal/domain/repository.go`
- Create: `api/internal/domain/terminal_profile.go`
- Create: `api/internal/domain/merge_strategy.go`
- Create: `api/internal/domain/workspace_status.go`
- Create: `api/internal/domain/chat_status.go`
- Create: `api/internal/domain/agent_run_status.go`
- Create: `api/internal/domain/review_thread_status.go`

- [ ] **Step 1: `project.go`** (GORM — `00` §5.1)

```go
package domain

import "time"

// Project is the org-level node grouping repositories (00 §5.1).
type Project struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	LastActivity time.Time `json:"lastActivity"`
}

func (Project) TableName() string {
	return "projects"
}
```

- [ ] **Step 2: `repository.go`** (GORM — `00` §5.2)

```go
package domain

// Repository is a git repo imported under a Project (00 §5.2).
type Repository struct {
	ID            string `gorm:"primaryKey" json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultBranch string `json:"defaultBranch"`
	AvatarLabel   string `json:"avatarLabel"`
	AvatarColor   string `json:"avatarColor"`
}

func (Repository) TableName() string {
	return "repositories"
}
```

- [ ] **Step 3: `terminal_profile.go`** (GORM, `[]string` via json serializer — `00` §5.6)

```go
package domain

// TerminalProfile is a server-stored PTY launch profile (00 §5.6).
type TerminalProfile struct {
	ID               string   `gorm:"primaryKey" json:"id"`
	Name             string   `json:"name"`
	Shell            string   `json:"shell,omitempty"`
	StartupDirectory string   `json:"startupDirectory,omitempty"`
	StartupCommands  []string `gorm:"serializer:json" json:"startupCommands"`
	Icon             string   `json:"icon,omitempty"`
	Color            string   `json:"color,omitempty"`
}

func (TerminalProfile) TableName() string {
	return "terminal_profiles"
}
```

- [ ] **Step 4: `merge_strategy.go`** (`00` §5.3)

```go
package domain

// MergeStrategy is the branch-review merge selector (09 §4).
type MergeStrategy string

const (
	MergeStrategyMerge  MergeStrategy = "merge"
	MergeStrategySquash MergeStrategy = "squash"
	MergeStrategyRebase MergeStrategy = "rebase"
)
```

- [ ] **Step 5: `workspace_status.go`** (`00` §6.1 — empty string == null/no-badge)

```go
package domain

// WorkspaceStatus is the base lifecycle badge; "" means "has commits, no PR" (00 §6.1).
type WorkspaceStatus string

const (
	WorkspaceStatusNew      WorkspaceStatus = "new"
	WorkspaceStatusPROpen   WorkspaceStatus = "pr-open"
	WorkspaceStatusPRMerged WorkspaceStatus = "pr-merged"
	WorkspaceStatusPRClosed WorkspaceStatus = "pr-closed"
)
```

- [ ] **Step 6: `chat_status.go`** (`00` §5.4 / §6.1)

```go
package domain

// ChatStatus drives the chat sidebar spinner (00 §5.4).
type ChatStatus string

const (
	ChatStatusIdle         ChatStatus = "idle"
	ChatStatusAgentRunning ChatStatus = "agent-running"
)
```

- [ ] **Step 7: `agent_run_status.go`** (`00` §6.2)

```go
package domain

// AgentRunStatus is the AgentRun lifecycle (00 §6.2).
type AgentRunStatus string

const (
	AgentRunStatusPending     AgentRunStatus = "pending"
	AgentRunStatusRunning     AgentRunStatus = "running"
	AgentRunStatusDone        AgentRunStatus = "done"
	AgentRunStatusError       AgentRunStatus = "error"
	AgentRunStatusInterrupted AgentRunStatus = "interrupted"
)
```

- [ ] **Step 8: `review_thread_status.go`** (`00` §6.3)

```go
package domain

// ReviewThreadStatus is the open↔resolved lifecycle (00 §6.3).
type ReviewThreadStatus string

const (
	ReviewThreadStatusOpen     ReviewThreadStatus = "open"
	ReviewThreadStatusResolved ReviewThreadStatus = "resolved"
)
```

- [ ] **Step 9: Build the package**

Run: `go build ./internal/domain/...`
Expected: compiles.

- [ ] **Step 10: Commit**

```bash
git add internal/domain
git commit -m "feat(domain): GORM entities (Project, Repository, TerminalProfile) + status enums"
```

### Task 8: Workspace aggregate type

**Files:**
- Create: `api/internal/domain/pending_merge.go`
- Create: `api/internal/domain/workspace.go`

- [ ] **Step 1: `pending_merge.go`** (`00` §5.3)

```go
package domain

// PendingMerge records a conflicted merge-into-parent awaiting resolution (07 §3.1).
type PendingMerge struct {
	Strategy       MergeStrategy `json:"strategy"`
	TargetParentID string        `json:"targetParentId"`
}
```

- [ ] **Step 2: `workspace.go`** (full aggregate — every field from `00` §5.3)

```go
package domain

import "time"

// Workspace is the git-worktree aggregate; the single source of truth for the
// sidebar row (00 §5.3). Mutated only through Asynx commands.
type Workspace struct {
	ID             string          `json:"id"`
	RepoID         string          `json:"repoId"`
	ProjectID      string          `json:"projectId"`
	Branch         string          `json:"branch"`
	WorktreePath   string          `json:"worktreePath"`
	ForkPointSha   string          `json:"forkPointSha"`
	ParentID       string          `json:"parentId,omitempty"`
	Status         WorkspaceStatus `json:"status,omitempty"`
	Locked         bool            `json:"locked"`
	HasConflicts   bool            `json:"hasConflicts"`
	MergeStrategy  MergeStrategy   `json:"mergeStrategy"`
	PendingMerge   *PendingMerge   `json:"pendingMerge,omitempty"`
	Added          int             `json:"added"`
	Deleted        int             `json:"deleted"`
	PRUrl          string          `json:"prUrl,omitempty"`
	PRTitle        string          `json:"prTitle,omitempty"`
	PRTargetBranch string          `json:"prTargetBranch,omitempty"`
	LastActivity   time.Time       `json:"lastActivity"`
	CreatedAt      time.Time       `json:"createdAt"`
}
```

- [ ] **Step 3: Build**

Run: `go build ./internal/domain/...`
Expected: compiles.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/pending_merge.go internal/domain/workspace.go
git commit -m "feat(domain): Workspace aggregate with full spec field set"
```

### Task 9: Chat, AgentRun, ReviewThread aggregate types

**Files:**
- Create: `api/internal/domain/chat.go`
- Create: `api/internal/domain/agent_run.go`
- Create: `api/internal/domain/review_thread.go`

- [ ] **Step 1: `chat.go`** (`00` §5.4 — content deferred per spec)

```go
package domain

import "time"

// Chat is the conversation aggregate; only its status lifecycle is event-sourced
// here — turn content is deferred to the Agentic Bridge spike (00 §5.4).
type Chat struct {
	ID        string     `json:"id"`
	WsID      string     `json:"wsId"`
	Status    ChatStatus `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
}
```

- [ ] **Step 2: `agent_run.go`** (`00` §5.5)

```go
package domain

import "time"

// AgentRun is the agent execution aggregate (00 §5.5). Mutation commands beyond
// crash recovery are bridge-owned (post-spike).
type AgentRun struct {
	ID        string         `json:"id"`
	WsID      string         `json:"wsId"`
	ChatID    string         `json:"chatId"`
	Status    AgentRunStatus `json:"status"`
	CreatedAt time.Time      `json:"createdAt"`
}
```

- [ ] **Step 3: `review_thread.go`** (`00` §6.3)

```go
package domain

import "time"

// ReviewThread is the branch-review comment-thread aggregate (00 §6.3).
type ReviewThread struct {
	ID        string             `json:"id"`
	WsID      string             `json:"wsId"`
	Status    ReviewThreadStatus `json:"status"`
	CreatedAt time.Time          `json:"createdAt"`
}
```

- [ ] **Step 4: Build the whole domain package**

Run: `go build ./internal/domain/... && go vet ./internal/domain/...`
Expected: compiles, vet clean.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/chat.go internal/domain/agent_run.go internal/domain/review_thread.go
git commit -m "feat(domain): Chat, AgentRun, ReviewThread aggregate types"
```

---

## Phase 3 — `adapter/` (SQLite event stores + GORM)

Persistence layer. Imports `core/` only. Mirrors `quiver.core/internal/adapter/{eventstore/sqlite,store/sqlite,container.go}`.

### Task 10: SQLite Asynx event store

**Files:**
- Create: `api/internal/adapter/eventstore/sqlite/event_store.go`
- Test: `api/internal/adapter/eventstore/sqlite/event_store_test.go`
- Bench: `api/internal/adapter/eventstore/sqlite/event_store_bench_test.go`

Reference: `quiver.core/internal/adapter/eventstore/sqlite/event_store.go` (copy near-verbatim; implements `asynx/models.Store`).

- [ ] **Step 1: Write `event_store.go`**

```go
package sqlite

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type eventEntry struct {
	AggregateID string `gorm:"primaryKey;column:aggregate_id"`
	Version     int64  `gorm:"primaryKey;column:version"`
	Data        []byte `gorm:"not null"`
}

func (eventEntry) TableName() string {
	return "events"
}

type eventStore struct {
	db *gorm.DB
}

// NewEventStore returns a GORM-backed asynx event store at path. Pins to a single
// connection (serialized writes) and checkpoints the WAL on Close.
func NewEventStore(
	path string,
) (models.Store, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("eventstore: open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("eventstore: db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(&eventEntry{}); err != nil {
		return nil, fmt.Errorf("eventstore: migrate: %w", err)
	}

	return &eventStore{db: db}, nil
}

func (s *eventStore) Append(
	ctx context.Context,
	aggregateID string,
	version int64,
	data []byte,
) error {
	result := s.db.WithContext(ctx).Create(&eventEntry{
		AggregateID: aggregateID,
		Version:     version,
		Data:        data,
	})
	if result.Error != nil {
		return fmt.Errorf("%w: version conflict (%s, v%d)", models.ErrPipelineFailed, aggregateID, version)
	}
	return nil
}

func (s *eventStore) ReadFrom(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
) ([][]byte, error) {
	var entries []eventEntry
	err := s.db.WithContext(ctx).
		Where("aggregate_id = ? AND version >= ?", aggregateID, fromVersion).
		Order("version ASC").
		Find(&entries).Error
	if err != nil {
		return nil, fmt.Errorf("eventstore: read from: %w", err)
	}
	return toBlobs(entries), nil
}

func (s *eventStore) ReadRange(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
	count int64,
) ([][]byte, error) {
	var entries []eventEntry
	err := s.db.WithContext(ctx).
		Where("aggregate_id = ? AND version >= ?", aggregateID, fromVersion).
		Order("version ASC").
		Limit(int(count)).
		Find(&entries).Error
	if err != nil {
		return nil, fmt.Errorf("eventstore: read range: %w", err)
	}
	return toBlobs(entries), nil
}

func (s *eventStore) Count(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&eventEntry{}).
		Where("aggregate_id = ? AND version >= ?", aggregateID, fromVersion).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("eventstore: count: %w", err)
	}
	return count, nil
}

func (s *eventStore) Delete(
	ctx context.Context,
	aggregateID string,
) error {
	result := s.db.WithContext(ctx).
		Where("aggregate_id = ?", aggregateID).
		Delete(&eventEntry{})
	if result.Error != nil {
		return fmt.Errorf("eventstore: delete: %w", result.Error)
	}
	return nil
}

// Close checkpoints the WAL and releases the file handle.
func (s *eventStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("eventstore: close: %w", err)
	}
	return sqlDB.Close()
}

func toBlobs(
	entries []eventEntry,
) [][]byte {
	result := make([][]byte, len(entries))
	for i, e := range entries {
		result[i] = e.Data
	}
	return result
}
```

> **Verify the `models.Store` interface signature** before writing: open `/Users/char2cs/go/pkg/mod/github.com/char2cs/asynx@v0.6.2/models/store.go`. If `Close() error` is not part of the interface, keep it as an extra method (the adapter container type-asserts `io.Closer`).

- [ ] **Step 2: Write `event_store_test.go`** (adapt quiver's; `:memory:` DB)

```go
package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEventStore(t *testing.T) models.Store {
	t.Helper()
	s, err := NewEventStore(":memory:")
	require.NoError(t, err)
	return s
}

func cancelledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestNewEventStore_InvalidPath_ReturnsError(t *testing.T) {
	_, err := NewEventStore("/nonexistent-dir-crowbar/db.sqlite")
	assert.Error(t, err)
}

func TestEventStore_Append_ThenReadFrom(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("e1")))
	blobs, err := s.ReadFrom(ctx, "agg-1", 1)
	require.NoError(t, err)
	require.Len(t, blobs, 1)
	assert.Equal(t, []byte("e1"), blobs[0])
}

func TestEventStore_Append_VersionConflict(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("first")))
	err := s.Append(ctx, "agg-1", 1, []byte("dup"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrPipelineFailed))
}

func TestEventStore_ReadFrom_Offset(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("e1")))
	require.NoError(t, s.Append(ctx, "agg-1", 2, []byte("e2")))
	blobs, err := s.ReadFrom(ctx, "agg-1", 2)
	require.NoError(t, err)
	require.Len(t, blobs, 1)
	assert.Equal(t, []byte("e2"), blobs[0])
}

func TestEventStore_ReadRange_TruncatesCount(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("e1")))
	require.NoError(t, s.Append(ctx, "agg-1", 2, []byte("e2")))
	blobs, err := s.ReadRange(ctx, "agg-1", 1, 100)
	require.NoError(t, err)
	assert.Len(t, blobs, 2)
}

func TestEventStore_Count(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("e1")))
	require.NoError(t, s.Append(ctx, "agg-1", 2, []byte("e2")))
	count, err := s.Count(ctx, "agg-1", 1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestEventStore_Delete_RemovesAggregate(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()
	require.NoError(t, s.Append(ctx, "agg-1", 1, []byte("e1")))
	require.NoError(t, s.Delete(ctx, "agg-1"))
	blobs, err := s.ReadFrom(ctx, "agg-1", 1)
	require.NoError(t, err)
	assert.Empty(t, blobs)
}

func TestEventStore_Append_ContextCancelled(t *testing.T) {
	s := newTestEventStore(t)
	err := s.Append(cancelledCtx(), "agg-1", 1, []byte("d"))
	assert.Error(t, err)
}

func TestEventStore_ReadFrom_ContextCancelled(t *testing.T) {
	s := newTestEventStore(t)
	_, err := s.ReadFrom(cancelledCtx(), "agg-1", 1)
	assert.Error(t, err)
}

func TestEventStore_ReadRange_ContextCancelled(t *testing.T) {
	s := newTestEventStore(t)
	_, err := s.ReadRange(cancelledCtx(), "agg-1", 1, 10)
	assert.Error(t, err)
}

func TestEventStore_Count_ContextCancelled(t *testing.T) {
	s := newTestEventStore(t)
	_, err := s.Count(cancelledCtx(), "agg-1", 1)
	assert.Error(t, err)
}
```

- [ ] **Step 3: Write `event_store_bench_test.go`** (perf-critical write path — standards rule 6)

```go
package sqlite

import (
	"context"
	"testing"
)

func BenchmarkEventStore_Append(b *testing.B) {
	s, err := NewEventStore(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Append(ctx, "agg-bench", int64(i+1), []byte("payload"))
	}
}

func BenchmarkEventStore_ReadFrom(b *testing.B) {
	s, err := NewEventStore(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		_ = s.Append(ctx, "agg-bench", int64(i+1), []byte("payload"))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.ReadFrom(ctx, "agg-bench", 1)
	}
}
```

- [ ] **Step 4: Run tests + bench smoke**

Run: `go test ./internal/adapter/eventstore/sqlite/... && go test -run=^$ -bench=. ./internal/adapter/eventstore/sqlite/...`
Expected: tests PASS; benchmarks report ns/op.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/eventstore
git commit -m "feat(adapter): SQLite asynx event store + benchmarks"
```

### Task 11: Generic GORM store + adapter container

**Files:**
- Create: `api/internal/adapter/store/store.go`
- Create: `api/internal/adapter/store/sqlite/sqlite.go`
- Test: `api/internal/adapter/store/sqlite/sqlite_test.go`
- Create: `api/internal/adapter/container.go`
- Test: `api/internal/adapter/container_test.go`

Reference: `quiver.core/internal/adapter/store/sqlite/sqlite.go` + `quiver.core/internal/adapter/container.go`.

- [ ] **Step 1: `store/store.go`** (the interface — go-style rule 8: expose interface)

```go
package store

import "context"

// Store is a generic CRUD repository over a GORM-mapped entity T keyed by K.
type Store[T any, K comparable] interface {
	Save(
		ctx context.Context,
		item T,
	) error
	Delete(
		ctx context.Context,
		id K,
	) error
	FindByKey(
		ctx context.Context,
		id K,
	) (*T, error)
	FindAll(
		ctx context.Context,
	) ([]T, error)
}
```

- [ ] **Step 2: `store/sqlite/sqlite.go`** (copy quiver near-verbatim; generic `New`, `OpenDB`, `NewFromDB`)

```go
// Package sqlite provides a GORM-backed Store[T, K] implementation.
package sqlite

import (
	"context"
	"errors"
	"fmt"

	glebarez "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
)

type gormStore[T any, K comparable] struct {
	db    *gorm.DB
	pkCol string
}

// New opens (or creates) a SQLite-backed Store[T, K] at path, auto-migrating T.
func New[T any, K comparable](
	path string,
) (store.Store[T, K], error) {
	db, err := OpenDB(path)
	if err != nil {
		return nil, err
	}
	return NewFromDB[T, K](db)
}

// OpenDB opens (or creates) a single-connection SQLite database at path.
func OpenDB(
	path string,
) (*gorm.DB, error) {
	db, err := gorm.Open(glebarez.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("sqlite: db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db, nil
}

// NewFromDB builds a Store[T, K] over an already-open DB, auto-migrating T.
func NewFromDB[T any, K comparable](
	db *gorm.DB,
) (store.Store[T, K], error) {
	var zero T
	if err := db.AutoMigrate(&zero); err != nil {
		return nil, fmt.Errorf("sqlite: migrate: %w", err)
	}
	pkCol, err := primaryKeyColumn[T](db)
	if err != nil {
		return nil, fmt.Errorf("sqlite: schema: %w", err)
	}
	return &gormStore[T, K]{db: db, pkCol: pkCol}, nil
}

func primaryKeyColumn[T any](
	db *gorm.DB,
) (string, error) {
	var zero T
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&zero); err != nil {
		return "", err
	}
	namer := schema.NamingStrategy{}
	for _, field := range stmt.Schema.Fields {
		if field.PrimaryKey {
			return namer.ColumnName("", field.Name), nil
		}
	}
	return "", fmt.Errorf("no primary key field found")
}

func (s *gormStore[T, K]) Save(
	ctx context.Context,
	item T,
) error {
	return s.db.WithContext(ctx).Save(&item).Error
}

func (s *gormStore[T, K]) Delete(
	ctx context.Context,
	id K,
) error {
	var zero T
	return s.db.WithContext(ctx).Where(s.pkCol+" = ?", id).Delete(&zero).Error
}

func (s *gormStore[T, K]) FindByKey(
	ctx context.Context,
	id K,
) (*T, error) {
	var item T
	err := s.db.WithContext(ctx).Where(s.pkCol+" = ?", id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *gormStore[T, K]) FindAll(
	ctx context.Context,
) ([]T, error) {
	var items []T
	if err := s.db.WithContext(ctx).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
```

- [ ] **Step 3: `store/sqlite/sqlite_test.go`** (use `domain.Project` as the test entity)

```go
package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newProjectStore(t *testing.T) (context.Context, interface {
	Save(context.Context, domain.Project) error
	Delete(context.Context, string) error
	FindByKey(context.Context, string) (*domain.Project, error)
	FindAll(context.Context) ([]domain.Project, error)
}) {
	t.Helper()
	s, err := sqlite.New[domain.Project, string](":memory:")
	require.NoError(t, err)
	return context.Background(), s
}

func TestGormStore_SaveAndFindByKey(t *testing.T) {
	ctx, s := newProjectStore(t)
	require.NoError(t, s.Save(ctx, domain.Project{ID: "p1", Name: "Alpha"}))
	got, err := s.FindByKey(ctx, "p1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Alpha", got.Name)
}

func TestGormStore_FindByKey_NotFound_ReturnsNil(t *testing.T) {
	ctx, s := newProjectStore(t)
	got, err := s.FindByKey(ctx, "missing")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestGormStore_FindAll(t *testing.T) {
	ctx, s := newProjectStore(t)
	require.NoError(t, s.Save(ctx, domain.Project{ID: "p1"}))
	require.NoError(t, s.Save(ctx, domain.Project{ID: "p2"}))
	all, err := s.FindAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestGormStore_Delete(t *testing.T) {
	ctx, s := newProjectStore(t)
	require.NoError(t, s.Save(ctx, domain.Project{ID: "p1"}))
	require.NoError(t, s.Delete(ctx, "p1"))
	got, err := s.FindByKey(ctx, "p1")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestNew_InvalidPath_ReturnsError(t *testing.T) {
	_, err := sqlite.New[domain.Project, string]("/nonexistent-dir-crowbar/x.db")
	assert.Error(t, err)
}
```

- [ ] **Step 4: `container.go`** (4 event stores + GORM DB; `WithHomeDir`; `io.Closer` aggregation)

```go
package adapter

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/paths"
)

// Container holds the persistence layer: one event store per Asynx aggregate and
// the shared GORM database.
type Container struct {
	WorkspaceES    asynxModels.Store
	ChatES         asynxModels.Store
	AgentRunES     asynxModels.Store
	ReviewThreadES asynxModels.Store
	DB             *gormdb.DB
	closers        []io.Closer
}

type adapterOpts struct {
	homeDir string
}

// Option configures adapter.New.
type Option func(*adapterOpts)

// WithHomeDir overrides the home directory used for path resolution.
func WithHomeDir(
	dir string,
) Option {
	return func(o *adapterOpts) {
		o.homeDir = dir
	}
}

// New constructs all event stores and the GORM database.
func New(
	opts ...Option,
) (*Container, error) {
	cfg := adapterOpts{}
	for _, o := range opts {
		o(&cfg)
	}

	eventsPath, storePath, err := resolveDirs(cfg.homeDir)
	if err != nil {
		return nil, err
	}

	stores, closers, err := openEventStores(eventsPath)
	if err != nil {
		return nil, err
	}

	db, err := storesqlite.OpenDB(filepath.Join(storePath, "crowbar.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: open db: %w", err)
	}

	return &Container{
		WorkspaceES:    stores[0],
		ChatES:         stores[1],
		AgentRunES:     stores[2],
		ReviewThreadES: stores[3],
		DB:             db,
		closers:        closers,
	}, nil
}

func resolveDirs(
	homeDir string,
) (string, string, error) {
	if homeDir != "" {
		eventsPath, err := paths.EventsAt(homeDir)
		if err != nil {
			return "", "", fmt.Errorf("adapter: events: %w", err)
		}
		storePath, err := paths.StoreAt(homeDir)
		if err != nil {
			return "", "", fmt.Errorf("adapter: store: %w", err)
		}
		return eventsPath, storePath, nil
	}
	eventsPath, err := paths.Events()
	if err != nil {
		return "", "", fmt.Errorf("adapter: events: %w", err)
	}
	storePath, err := paths.Store()
	if err != nil {
		return "", "", fmt.Errorf("adapter: store: %w", err)
	}
	return eventsPath, storePath, nil
}

func openEventStores(
	eventsPath string,
) ([]asynxModels.Store, []io.Closer, error) {
	names := []string{"workspace.db", "chat.db", "agent_run.db", "review_thread.db"}
	stores := make([]asynxModels.Store, 0, len(names))
	closers := make([]io.Closer, 0, len(names))
	for _, name := range names {
		es, err := eventsqlite.NewEventStore(filepath.Join(eventsPath, name))
		if err != nil {
			return nil, nil, fmt.Errorf("adapter: event store %s: %w", name, err)
		}
		stores = append(stores, es)
		if cl, ok := es.(io.Closer); ok {
			closers = append(closers, cl)
		}
	}
	return stores, closers, nil
}

// Close checkpoints and closes every event store.
func (c *Container) Close() error {
	var errs []error
	for _, cl := range c.closers {
		if err := cl.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
```

- [ ] **Step 5: `container_test.go`** (boots all stores under a temp home)

```go
package adapter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
)

func TestNew_BootsAllStores(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	assert.NotNil(t, c.WorkspaceES)
	assert.NotNil(t, c.ChatES)
	assert.NotNil(t, c.AgentRunES)
	assert.NotNil(t, c.ReviewThreadES)
	assert.NotNil(t, c.DB)
}

func TestClose_Idempotentish(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	assert.NoError(t, c.Close())
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/adapter/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter
git commit -m "feat(adapter): generic GORM store + container wiring 4 event stores + DB"
```

---

## Phase 4 — `app/hub/` (typed WebSocketHub fan-out)

The hub is the only thing producers call; it knows nothing about WebSockets (`03` §1). Defined in the app layer so app builders depend on it without importing `api`. Mirrors `quiver.core/internal/app/hub/hub.go`. For Wave 0 the interface carries the two **Class A** producers that exist now — `BroadcastWorkspace` and `BroadcastChat`. Class-B topic methods (Git/Files/LSP/Terminal) are added alongside their concrete broadcasters in later waves.

### Task 12: Hub interface, events, and fan-out

**Files:**
- Create: `api/internal/app/hub/chat_status_event.go`
- Create: `api/internal/app/hub/web_socket_hub.go`
- Create: `api/internal/app/hub/subscriber.go`
- Create: `api/internal/app/hub/hub.go`
- Test: `api/internal/app/hub/hub_test.go`

- [ ] **Step 1: `chat_status_event.go`** (`03` §3/§4.2 payload)

```go
package hub

import "github.com/char2cs/crowbar/api/internal/domain"

// ChatStatusEvent is the Chats-topic payload; chatId identifies the row (03 §4.2).
type ChatStatusEvent struct {
	ChatID string            `json:"chatId"`
	WsID   string            `json:"wsId"`
	Status domain.ChatStatus `json:"status"`
}
```

- [ ] **Step 2: `web_socket_hub.go`** (the producer-facing interface)

```go
package hub

import "github.com/char2cs/crowbar/api/internal/domain"

// WebSocketHub is the version-agnostic broadcast interface domain producers call.
// Class-B topic methods (Git, Files, LSP, Terminal) are added with their
// broadcasters in later waves.
type WebSocketHub interface {
	BroadcastWorkspace(
		ws domain.Workspace,
	)
	BroadcastChat(
		evt ChatStatusEvent,
	)
}
```

- [ ] **Step 3: `subscriber.go`** (the API-side receiver interface)

```go
package hub

import "github.com/char2cs/crowbar/api/internal/domain"

// Subscriber receives hub broadcasts. Implemented by the API WS handler set.
type Subscriber interface {
	PushWorkspace(
		ws domain.Workspace,
	)
	PushChat(
		evt ChatStatusEvent,
	)
}
```

- [ ] **Step 4: `hub.go`** (fan-out; implements `WebSocketHub`)

```go
package hub

import (
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Hub fans out domain broadcasts to all registered Subscribers. It implements
// WebSocketHub so the app layer can broadcast through it.
type Hub struct {
	mu          sync.RWMutex
	subscribers []Subscriber
}

// NewHub constructs an empty Hub.
func NewHub() *Hub {
	return &Hub{}
}

// Register adds a Subscriber to the fan-out set.
func (h *Hub) Register(
	s Subscriber,
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subscribers = append(h.subscribers, s)
}

// BroadcastWorkspace fans a Workspace row out to every subscriber.
func (h *Hub) BroadcastWorkspace(
	ws domain.Workspace,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushWorkspace(ws)
	}
}

// BroadcastChat fans a ChatStatusEvent out to every subscriber.
func (h *Hub) BroadcastChat(
	evt ChatStatusEvent,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushChat(evt)
	}
}

var _ WebSocketHub = (*Hub)(nil)

var _ = domain.Workspace{}
```

> Remove the trailing `var _ = domain.Workspace{}` line — it's only there to remind you `domain` is imported; if `goimports` flags an unused import, the real method bodies already reference `domain` via signatures, so just delete that line.

- [ ] **Step 5: `hub_test.go`** (a fake subscriber records pushes)

```go
package hub_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakeSubscriber struct {
	workspaces []domain.Workspace
	chats      []hub.ChatStatusEvent
}

func (f *fakeSubscriber) PushWorkspace(
	ws domain.Workspace,
) {
	f.workspaces = append(f.workspaces, ws)
}

func (f *fakeSubscriber) PushChat(
	evt hub.ChatStatusEvent,
) {
	f.chats = append(f.chats, evt)
}

func TestHub_BroadcastWorkspace_ReachesSubscribers(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastWorkspace(domain.Workspace{ID: "w1"})

	assert.Len(t, a.workspaces, 1)
	assert.Len(t, b.workspaces, 1)
	assert.Equal(t, "w1", a.workspaces[0].ID)
}

func TestHub_BroadcastChat_ReachesSubscribers(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	h.Register(a)

	h.BroadcastChat(hub.ChatStatusEvent{ChatID: "c1", WsID: "w1", Status: domain.ChatStatusAgentRunning})

	assert.Len(t, a.chats, 1)
	assert.Equal(t, "c1", a.chats[0].ChatID)
}

func TestHub_NoSubscribers_DoesNotPanic(t *testing.T) {
	h := hub.NewHub()
	assert.NotPanics(t, func() {
		h.BroadcastWorkspace(domain.Workspace{ID: "w1"})
	})
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/app/hub/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/app/hub
git commit -m "feat(app): typed WebSocketHub fan-out (Workspace + Chat producers)"
```

---

## Phase 5 — `app/repositories/` (Asynx commands + repos)

Each aggregate gets a repository package exposing an **interface** (go-style rule 8); the implementation wraps `asynx.Asynx[T]` and its commands live in `internal/commands/`. Commands are **pure** (`asynx/models.Command` contract: no IO, no `time.Now()`) — timestamps are injected via a `Now time.Time` field set by the caller. This is what makes the **GATE round-trip** (send command → event persisted → reload reconstructs state) demonstrable and deterministic.

> **Asynx command contract** (`asynx@v0.6.2/models/command.go`): `AggregateID() string`, `EventName() string`, `ShouldSnapshot() bool`, `Validate(current *T) error` (nil current = never existed), `EmitEvent(current *T) T` (nil current = never existed). `models.ErrValidation` is the canonical rejection error.

### Task 13: Workspace repository + commands (GATE round-trip + state machine)

**Files:**
- Create: `api/internal/app/repositories/workspace/internal/commands/create.go`
- Create: `api/internal/app/repositories/workspace/internal/commands/sync_working_tree_state.go`
- Test: `api/internal/app/repositories/workspace/internal/commands/commands_test.go`
- Create: `api/internal/app/repositories/workspace/workspace.go`
- Test: `api/internal/app/repositories/workspace/workspace_test.go`

Reference: `quiver.core/internal/app/repositories/arrow/internal/commands/add.go` (command shape).

- [ ] **Step 1: `create.go`** (`status = new` at create — `00` §5.3)

```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// CreateWorkspace creates a new workspace aggregate seeded to status "new".
type CreateWorkspace struct {
	ID            string
	RepoID        string
	ProjectID     string
	Branch        string
	WorktreePath  string
	ForkPointSha  string
	ParentID      string
	Locked        bool
	MergeStrategy domain.MergeStrategy
	Now           time.Time
}

func (c CreateWorkspace) AggregateID() string {
	return c.ID
}

func (c CreateWorkspace) EventName() string {
	return "workspace.created." + c.ID
}

func (c CreateWorkspace) ShouldSnapshot() bool {
	return true
}

func (c CreateWorkspace) Validate(
	current *domain.Workspace,
) error {
	if current != nil {
		return fmt.Errorf("create workspace: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.RepoID == "" || c.ProjectID == "" {
		return fmt.Errorf("create workspace: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c CreateWorkspace) EmitEvent(
	_ *domain.Workspace,
) domain.Workspace {
	strategy := c.MergeStrategy
	if strategy == "" {
		strategy = domain.MergeStrategyMerge
	}
	return domain.Workspace{
		ID:            c.ID,
		RepoID:        c.RepoID,
		ProjectID:     c.ProjectID,
		Branch:        c.Branch,
		WorktreePath:  c.WorktreePath,
		ForkPointSha:  c.ForkPointSha,
		ParentID:      c.ParentID,
		Status:        domain.WorkspaceStatusNew,
		Locked:        c.Locked,
		MergeStrategy: strategy,
		LastActivity:  c.Now,
		CreatedAt:     c.Now,
	}
}
```

- [ ] **Step 2: `sync_working_tree_state.go`** (the `new→null` guard — `00` §6.1; clamps `≥0`)

```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SyncWorkingTreeState recomputes diff/conflict summary from git and clears the
// "new" status once the branch has commits (00 §5.3, §6.1). HasCommits is a
// transient input, not a stored field.
type SyncWorkingTreeState struct {
	ID           string
	Added        int
	Deleted      int
	HasConflicts bool
	HasCommits   bool
	Now          time.Time
}

func (c SyncWorkingTreeState) AggregateID() string {
	return c.ID
}

func (c SyncWorkingTreeState) EventName() string {
	return "workspace.working_tree_synced." + c.ID
}

func (c SyncWorkingTreeState) ShouldSnapshot() bool {
	return true
}

func (c SyncWorkingTreeState) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("sync working tree: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SyncWorkingTreeState) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.Added = clampZero(c.Added)
	ws.Deleted = clampZero(c.Deleted)
	ws.HasConflicts = c.HasConflicts
	ws.LastActivity = c.Now
	if ws.Status == domain.WorkspaceStatusNew && c.HasCommits {
		ws.Status = ""
	}
	return ws
}

func clampZero(
	n int,
) int {
	if n < 0 {
		return 0
	}
	return n
}
```

- [ ] **Step 3: `commands_test.go`** (pure unit tests of Validate/EmitEvent — no asynx needed)

```go
package commands

import (
	"errors"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestCreateWorkspace_Validate_RejectsExisting(t *testing.T) {
	cmd := CreateWorkspace{ID: "w1", RepoID: "r1", ProjectID: "p1"}
	err := cmd.Validate(&domain.Workspace{ID: "w1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestCreateWorkspace_Validate_RejectsMissingIDs(t *testing.T) {
	cmd := CreateWorkspace{ID: "w1"}
	err := cmd.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestCreateWorkspace_EmitEvent_SeedsNewStatusAndDefaultStrategy(t *testing.T) {
	now := time.Unix(1000, 0)
	cmd := CreateWorkspace{ID: "w1", RepoID: "r1", ProjectID: "p1", Now: now}
	ws := cmd.EmitEvent(nil)
	assert.Equal(t, domain.WorkspaceStatusNew, ws.Status)
	assert.Equal(t, domain.MergeStrategyMerge, ws.MergeStrategy)
	assert.Equal(t, now, ws.CreatedAt)
}

func TestSyncWorkingTreeState_Validate_RejectsMissing(t *testing.T) {
	err := SyncWorkingTreeState{ID: "w1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestSyncWorkingTreeState_ClearsNewWhenHasCommits(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusNew}
	ws := SyncWorkingTreeState{ID: "w1", HasCommits: true}.EmitEvent(cur)
	assert.Equal(t, domain.WorkspaceStatus(""), ws.Status)
}

func TestSyncWorkingTreeState_DoesNotStompPROpen(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusPROpen}
	ws := SyncWorkingTreeState{ID: "w1", HasCommits: true}.EmitEvent(cur)
	assert.Equal(t, domain.WorkspaceStatusPROpen, ws.Status)
}

func TestSyncWorkingTreeState_ClampsNegativeCounts(t *testing.T) {
	cur := &domain.Workspace{ID: "w1"}
	ws := SyncWorkingTreeState{ID: "w1", Added: -5, Deleted: -2}.EmitEvent(cur)
	assert.Equal(t, 0, ws.Added)
	assert.Equal(t, 0, ws.Deleted)
}

func TestCommands_Metadata(t *testing.T) {
	c := CreateWorkspace{ID: "w1"}
	require.Equal(t, "w1", c.AggregateID())
	assert.Contains(t, c.EventName(), "workspace.created")
	assert.True(t, c.ShouldSnapshot())
	s := SyncWorkingTreeState{ID: "w1"}
	assert.Equal(t, "w1", s.AggregateID())
	assert.Contains(t, s.EventName(), "working_tree_synced")
	assert.True(t, s.ShouldSnapshot())
}
```

- [ ] **Step 4: `workspace.go`** (the repository interface + impl wrapping asynx)

```go
package workspace

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// CreateInput carries the fields needed to create a workspace.
type CreateInput struct {
	ID            string
	RepoID        string
	ProjectID     string
	Branch        string
	WorktreePath  string
	ForkPointSha  string
	ParentID      string
	Locked        bool
	MergeStrategy domain.MergeStrategy
}

// SyncInput carries a recomputed working-tree summary.
type SyncInput struct {
	ID           string
	Added        int
	Deleted      int
	HasConflicts bool
	HasCommits   bool
}

// Workspace is the workspace aggregate repository.
type Workspace interface {
	Create(
		ctx context.Context,
		in CreateInput,
		now time.Time,
	) (domain.Workspace, error)
	SyncWorkingTreeState(
		ctx context.Context,
		in SyncInput,
		now time.Time,
	) (domain.Workspace, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
}

type workspace struct {
	ax asynx.Asynx[domain.Workspace]
}

// New builds a Workspace repository over the given asynx instance.
func New(
	ax asynx.Asynx[domain.Workspace],
) Workspace {
	return &workspace{ax: ax}
}

func (w *workspace) Create(
	ctx context.Context,
	in CreateInput,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.CreateWorkspace{
		ID:            in.ID,
		RepoID:        in.RepoID,
		ProjectID:     in.ProjectID,
		Branch:        in.Branch,
		WorktreePath:  in.WorktreePath,
		ForkPointSha:  in.ForkPointSha,
		ParentID:      in.ParentID,
		Locked:        in.Locked,
		MergeStrategy: in.MergeStrategy,
		Now:           now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: create: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) SyncWorkingTreeState(
	ctx context.Context,
	in SyncInput,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.SyncWorkingTreeState{
		ID:           in.ID,
		Added:        in.Added,
		Deleted:      in.Deleted,
		HasConflicts: in.HasConflicts,
		HasCommits:   in.HasCommits,
		Now:          now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	got, err := w.ax.Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: get: %w", err)
	}
	return got, nil
}
```

> **Confirm `models.Event[T]` exposes `.Aggregate`** before writing — open `asynx@v0.6.2/models/event.go`. Quiver uses `evt.Aggregate` (see ARCHITECTURE.md Asynx reference). If the field differs, adjust the three return sites.

- [ ] **Step 5: `workspace_test.go`** (the GATE round-trip: send → persist → reload reconstructs)

```go
package workspace_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/stretchr/testify/assert"
)

func newRepo(t *testing.T) (context.Context, workspace.Workspace) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return context.Background(), workspace.New(ax)
}

func TestWorkspace_Create_RoundTrips(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()

	created, err := repo.Create(ctx, workspace.CreateInput{
		ID:        "w1",
		RepoID:    "r1",
		ProjectID: "p1",
		Branch:    "feature/x",
	}, now)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatusNew, created.Status)

	reloaded, err := repo.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "feature/x", reloaded.Branch)
	assert.Equal(t, domain.WorkspaceStatusNew, reloaded.Status)
	assert.Equal(t, "p1", reloaded.ProjectID)
}

func TestWorkspace_SyncClearsNewStatus(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)

	synced, err := repo.SyncWorkingTreeState(ctx, workspace.SyncInput{
		ID:         "w1",
		Added:      10,
		Deleted:    2,
		HasCommits: true,
	}, now)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatus(""), synced.Status)
	assert.Equal(t, 10, synced.Added)

	reloaded, err := repo.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatus(""), reloaded.Status)
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/app/repositories/workspace/...`
Expected: PASS — this proves the GATE's Asynx round-trip requirement.

- [ ] **Step 7: Commit**

```bash
git add internal/app/repositories/workspace
git commit -m "feat(app): Workspace repository + commands (round-trip + new→null guard)"
```

> **Verified against `asynx@v0.6.2`:** `models.Event[T]` has field `Aggregate T` (use `evt.Aggregate`). `asynx.New[T]().WithEventStore(es).WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).Build()` is correct. `models.Store` does **not** include `Close()`, so the event store's `Close()` is an extra method on the concrete type — the adapter container's `io.Closer` type assertion (Task 11) is the right way to reach it.

### Task 14: AgentRun repository + commands (needed by crash recovery)

**Files:**
- Create: `api/internal/app/repositories/agentrun/internal/commands/create.go`
- Create: `api/internal/app/repositories/agentrun/internal/commands/mark_running.go`
- Create: `api/internal/app/repositories/agentrun/internal/commands/complete.go`
- Create: `api/internal/app/repositories/agentrun/internal/commands/fail.go`
- Test: `api/internal/app/repositories/agentrun/internal/commands/commands_test.go`
- Create: `api/internal/app/repositories/agentrun/agentrun.go`
- Test: `api/internal/app/repositories/agentrun/agentrun_test.go`

- [ ] **Step 1: `create.go`** (`pending` — `00` §6.2)

```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// CreateAgentRun creates an AgentRun aggregate in the pending state.
type CreateAgentRun struct {
	ID     string
	WsID   string
	ChatID string
	Now    time.Time
}

func (c CreateAgentRun) AggregateID() string {
	return c.ID
}

func (c CreateAgentRun) EventName() string {
	return "agent_run.created." + c.ID
}

func (c CreateAgentRun) ShouldSnapshot() bool {
	return false
}

func (c CreateAgentRun) Validate(
	current *domain.AgentRun,
) error {
	if current != nil {
		return fmt.Errorf("create agent run: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.WsID == "" || c.ChatID == "" {
		return fmt.Errorf("create agent run: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c CreateAgentRun) EmitEvent(
	_ *domain.AgentRun,
) domain.AgentRun {
	return domain.AgentRun{
		ID:        c.ID,
		WsID:      c.WsID,
		ChatID:    c.ChatID,
		Status:    domain.AgentRunStatusPending,
		CreatedAt: c.Now,
	}
}
```

- [ ] **Step 2: `mark_running.go`** (`pending → running`)

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// MarkAgentRunRunning advances a pending AgentRun to running.
type MarkAgentRunRunning struct {
	ID string
}

func (c MarkAgentRunRunning) AggregateID() string {
	return c.ID
}

func (c MarkAgentRunRunning) EventName() string {
	return "agent_run.running." + c.ID
}

func (c MarkAgentRunRunning) ShouldSnapshot() bool {
	return false
}

func (c MarkAgentRunRunning) Validate(
	current *domain.AgentRun,
) error {
	if current == nil {
		return fmt.Errorf("mark running: %w", asynxModels.ErrValidation)
	}
	if current.Status != domain.AgentRunStatusPending {
		return fmt.Errorf("mark running: not pending: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c MarkAgentRunRunning) EmitEvent(
	current *domain.AgentRun,
) domain.AgentRun {
	run := *current
	run.Status = domain.AgentRunStatusRunning
	return run
}
```

- [ ] **Step 3: `complete.go`** (`running → done`)

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// CompleteAgentRun advances a running AgentRun to done.
type CompleteAgentRun struct {
	ID string
}

func (c CompleteAgentRun) AggregateID() string {
	return c.ID
}

func (c CompleteAgentRun) EventName() string {
	return "agent_run.done." + c.ID
}

func (c CompleteAgentRun) ShouldSnapshot() bool {
	return false
}

func (c CompleteAgentRun) Validate(
	current *domain.AgentRun,
) error {
	if current == nil {
		return fmt.Errorf("complete agent run: %w", asynxModels.ErrValidation)
	}
	if current.Status != domain.AgentRunStatusRunning {
		return fmt.Errorf("complete agent run: not running: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c CompleteAgentRun) EmitEvent(
	current *domain.AgentRun,
) domain.AgentRun {
	run := *current
	run.Status = domain.AgentRunStatusDone
	return run
}
```

- [ ] **Step 4: `fail.go`** (`running → error`; the crash-recovery command — `00` §6.2)

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// FailAgentRun advances a running AgentRun to error. Issued by crash recovery.
type FailAgentRun struct {
	ID string
}

func (c FailAgentRun) AggregateID() string {
	return c.ID
}

func (c FailAgentRun) EventName() string {
	return "agent_run.error." + c.ID
}

func (c FailAgentRun) ShouldSnapshot() bool {
	return false
}

func (c FailAgentRun) Validate(
	current *domain.AgentRun,
) error {
	if current == nil {
		return fmt.Errorf("fail agent run: %w", asynxModels.ErrValidation)
	}
	if current.Status != domain.AgentRunStatusRunning {
		return fmt.Errorf("fail agent run: not running: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c FailAgentRun) EmitEvent(
	current *domain.AgentRun,
) domain.AgentRun {
	run := *current
	run.Status = domain.AgentRunStatusError
	return run
}
```

- [ ] **Step 5: `commands_test.go`**

```go
package commands

import (
	"errors"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestCreateAgentRun_EmitsPending(t *testing.T) {
	run := CreateAgentRun{ID: "a1", WsID: "w1", ChatID: "c1", Now: time.Unix(1, 0)}.EmitEvent(nil)
	assert.Equal(t, domain.AgentRunStatusPending, run.Status)
}

func TestCreateAgentRun_Validate_RejectsExisting(t *testing.T) {
	err := CreateAgentRun{ID: "a1", WsID: "w1", ChatID: "c1"}.Validate(&domain.AgentRun{ID: "a1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestMarkRunning_RequiresPending(t *testing.T) {
	err := MarkAgentRunRunning{ID: "a1"}.Validate(&domain.AgentRun{Status: domain.AgentRunStatusRunning})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
	run := MarkAgentRunRunning{ID: "a1"}.EmitEvent(&domain.AgentRun{Status: domain.AgentRunStatusPending})
	assert.Equal(t, domain.AgentRunStatusRunning, run.Status)
}

func TestComplete_RequiresRunning(t *testing.T) {
	err := CompleteAgentRun{ID: "a1"}.Validate(&domain.AgentRun{Status: domain.AgentRunStatusPending})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
	run := CompleteAgentRun{ID: "a1"}.EmitEvent(&domain.AgentRun{Status: domain.AgentRunStatusRunning})
	assert.Equal(t, domain.AgentRunStatusDone, run.Status)
}

func TestFail_RequiresRunning_ThenErrors(t *testing.T) {
	err := FailAgentRun{ID: "a1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
	run := FailAgentRun{ID: "a1"}.EmitEvent(&domain.AgentRun{Status: domain.AgentRunStatusRunning})
	assert.Equal(t, domain.AgentRunStatusError, run.Status)
}

func TestAgentRun_CommandMetadata(t *testing.T) {
	assert.Equal(t, "a1", CreateAgentRun{ID: "a1"}.AggregateID())
	assert.False(t, CreateAgentRun{ID: "a1"}.ShouldSnapshot())
	assert.Contains(t, FailAgentRun{ID: "a1"}.EventName(), "error")
	assert.Contains(t, MarkAgentRunRunning{ID: "a1"}.EventName(), "running")
	assert.Contains(t, CompleteAgentRun{ID: "a1"}.EventName(), "done")
}
```

- [ ] **Step 6: `agentrun.go`** (repository interface + impl)

```go
package agentrun

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// AgentRun is the agent-run aggregate repository.
type AgentRun interface {
	Create(
		ctx context.Context,
		id string,
		wsID string,
		chatID string,
		now time.Time,
	) (domain.AgentRun, error)
	MarkRunning(
		ctx context.Context,
		id string,
	) (domain.AgentRun, error)
	Complete(
		ctx context.Context,
		id string,
	) (domain.AgentRun, error)
	Fail(
		ctx context.Context,
		id string,
	) (domain.AgentRun, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.AgentRun, error)
}

type agentRun struct {
	ax asynx.Asynx[domain.AgentRun]
}

// New builds an AgentRun repository over the given asynx instance.
func New(
	ax asynx.Asynx[domain.AgentRun],
) AgentRun {
	return &agentRun{ax: ax}
}

func (r *agentRun) Create(
	ctx context.Context,
	id string,
	wsID string,
	chatID string,
	now time.Time,
) (domain.AgentRun, error) {
	evt, err := r.ax.SendWait(ctx, commands.CreateAgentRun{ID: id, WsID: wsID, ChatID: chatID, Now: now})
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("agentrun: create: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *agentRun) MarkRunning(
	ctx context.Context,
	id string,
) (domain.AgentRun, error) {
	evt, err := r.ax.SendWait(ctx, commands.MarkAgentRunRunning{ID: id})
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("agentrun: mark running: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *agentRun) Complete(
	ctx context.Context,
	id string,
) (domain.AgentRun, error) {
	evt, err := r.ax.SendWait(ctx, commands.CompleteAgentRun{ID: id})
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("agentrun: complete: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *agentRun) Fail(
	ctx context.Context,
	id string,
) (domain.AgentRun, error) {
	evt, err := r.ax.SendWait(ctx, commands.FailAgentRun{ID: id})
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("agentrun: fail: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *agentRun) Get(
	ctx context.Context,
	id string,
) (domain.AgentRun, error) {
	got, err := r.ax.Get(ctx, id)
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("agentrun: get: %w", err)
	}
	return got, nil
}
```

- [ ] **Step 7: `agentrun_test.go`** (full lifecycle round-trip)

```go
package agentrun_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newRepo(t *testing.T) (context.Context, agentrun.AgentRun) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRun]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return context.Background(), agentrun.New(ax)
}

func TestAgentRun_Lifecycle_PendingRunningDone(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	done, err := repo.Complete(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusDone, done.Status)
}

func TestAgentRun_Fail_FromRunning(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	failed, err := repo.Fail(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusError, failed.Status)
}

func TestAgentRun_MarkRunning_RejectedFromDone(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	_, err = repo.Complete(ctx, "a1")
	require.NoError(t, err)
	_, err = repo.MarkRunning(ctx, "a1")
	assert.Error(t, err)
}
```

- [ ] **Step 8: Run tests**

Run: `go test ./internal/app/repositories/agentrun/...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/app/repositories/agentrun
git commit -m "feat(app): AgentRun repository + lifecycle commands (incl. Fail for recovery)"
```

### Task 15: Chat repository + commands (Create + idempotent ResetIdle for reconcile)

**Files:**
- Create: `api/internal/app/repositories/chat/internal/commands/create.go`
- Create: `api/internal/app/repositories/chat/internal/commands/reset_idle.go`
- Test: `api/internal/app/repositories/chat/internal/commands/commands_test.go`
- Create: `api/internal/app/repositories/chat/chat.go`
- Test: `api/internal/app/repositories/chat/chat_test.go`

- [ ] **Step 1: `create.go`** (`idle`)

```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// CreateChat creates a Chat aggregate in the idle state.
type CreateChat struct {
	ID   string
	WsID string
	Now  time.Time
}

func (c CreateChat) AggregateID() string {
	return c.ID
}

func (c CreateChat) EventName() string {
	return "chat.created." + c.ID
}

func (c CreateChat) ShouldSnapshot() bool {
	return false
}

func (c CreateChat) Validate(
	current *domain.Chat,
) error {
	if current != nil {
		return fmt.Errorf("create chat: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.WsID == "" {
		return fmt.Errorf("create chat: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c CreateChat) EmitEvent(
	_ *domain.Chat,
) domain.Chat {
	return domain.Chat{
		ID:        c.ID,
		WsID:      c.WsID,
		Status:    domain.ChatStatusIdle,
		CreatedAt: c.Now,
	}
}
```

- [ ] **Step 2: `reset_idle.go`** (**idempotent** — `00` §6.2; resetting an already-idle chat is a no-op, never an error)

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ResetChatIdle forces a Chat to idle. Idempotent: resetting an already-idle chat
// emits the same idle state without error, so concurrent crash-recovery passes
// cannot conflict (00 §6.2).
type ResetChatIdle struct {
	ID string
}

func (c ResetChatIdle) AggregateID() string {
	return c.ID
}

func (c ResetChatIdle) EventName() string {
	return "chat.idle_reset." + c.ID
}

func (c ResetChatIdle) ShouldSnapshot() bool {
	return false
}

func (c ResetChatIdle) Validate(
	current *domain.Chat,
) error {
	if current == nil {
		return fmt.Errorf("reset chat idle: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ResetChatIdle) EmitEvent(
	current *domain.Chat,
) domain.Chat {
	chat := *current
	chat.Status = domain.ChatStatusIdle
	return chat
}
```

- [ ] **Step 3: `commands_test.go`**

```go
package commands

import (
	"errors"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestCreateChat_EmitsIdle(t *testing.T) {
	chat := CreateChat{ID: "c1", WsID: "w1", Now: time.Unix(1, 0)}.EmitEvent(nil)
	assert.Equal(t, domain.ChatStatusIdle, chat.Status)
}

func TestCreateChat_Validate_RejectsExisting(t *testing.T) {
	err := CreateChat{ID: "c1", WsID: "w1"}.Validate(&domain.Chat{ID: "c1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestResetIdle_FromAgentRunning(t *testing.T) {
	chat := ResetChatIdle{ID: "c1"}.EmitEvent(&domain.Chat{Status: domain.ChatStatusAgentRunning})
	assert.Equal(t, domain.ChatStatusIdle, chat.Status)
}

func TestResetIdle_IdempotentFromIdle(t *testing.T) {
	chat := ResetChatIdle{ID: "c1"}.EmitEvent(&domain.Chat{Status: domain.ChatStatusIdle})
	assert.Equal(t, domain.ChatStatusIdle, chat.Status)
}

func TestResetIdle_Validate_RejectsMissing(t *testing.T) {
	err := ResetChatIdle{ID: "c1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}
```

- [ ] **Step 4: `chat.go`** (repository)

```go
package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Chat is the chat aggregate repository.
type Chat interface {
	Create(
		ctx context.Context,
		id string,
		wsID string,
		now time.Time,
	) (domain.Chat, error)
	ResetIdle(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
}

type chat struct {
	ax asynx.Asynx[domain.Chat]
}

// New builds a Chat repository over the given asynx instance.
func New(
	ax asynx.Asynx[domain.Chat],
) Chat {
	return &chat{ax: ax}
}

func (c *chat) Create(
	ctx context.Context,
	id string,
	wsID string,
	now time.Time,
) (domain.Chat, error) {
	evt, err := c.ax.SendWait(ctx, commands.CreateChat{ID: id, WsID: wsID, Now: now})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: create: %w", err)
	}
	return evt.Aggregate, nil
}

func (c *chat) ResetIdle(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	evt, err := c.ax.SendWait(ctx, commands.ResetChatIdle{ID: id})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: reset idle: %w", err)
	}
	return evt.Aggregate, nil
}

func (c *chat) Get(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	got, err := c.ax.Get(ctx, id)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: get: %w", err)
	}
	return got, nil
}
```

- [ ] **Step 5: `chat_test.go`** (round-trip + idempotent double-reset)

```go
package chat_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newRepo(t *testing.T) (context.Context, chat.Chat) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return context.Background(), chat.New(ax)
}

func TestChat_Create_RoundTrips(t *testing.T) {
	ctx, repo := newRepo(t)
	created, err := repo.Create(ctx, "c1", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusIdle, created.Status)
	reloaded, err := repo.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "w1", reloaded.WsID)
}

func TestChat_ResetIdle_Idempotent(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, "c1", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.ResetIdle(ctx, "c1")
	require.NoError(t, err)
	second, err := repo.ResetIdle(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusIdle, second.Status)
}
```

- [ ] **Step 6: Run tests + commit**

Run: `go test ./internal/app/repositories/chat/...`
Expected: PASS.

```bash
git add internal/app/repositories/chat
git commit -m "feat(app): Chat repository + Create/ResetIdle (idempotent reconcile)"
```

### Task 16: ReviewThread repository + commands (open↔resolved — `00` §6.3)

**Files:**
- Create: `api/internal/app/repositories/reviewthread/internal/commands/open.go`
- Create: `api/internal/app/repositories/reviewthread/internal/commands/resolve.go`
- Create: `api/internal/app/repositories/reviewthread/internal/commands/reopen.go`
- Test: `api/internal/app/repositories/reviewthread/internal/commands/commands_test.go`
- Create: `api/internal/app/repositories/reviewthread/reviewthread.go`
- Test: `api/internal/app/repositories/reviewthread/reviewthread_test.go`

- [ ] **Step 1: `open.go`** (create in `open`)

```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// OpenReviewThread creates a ReviewThread aggregate in the open state.
type OpenReviewThread struct {
	ID   string
	WsID string
	Now  time.Time
}

func (c OpenReviewThread) AggregateID() string {
	return c.ID
}

func (c OpenReviewThread) EventName() string {
	return "review_thread.opened." + c.ID
}

func (c OpenReviewThread) ShouldSnapshot() bool {
	return false
}

func (c OpenReviewThread) Validate(
	current *domain.ReviewThread,
) error {
	if current != nil {
		return fmt.Errorf("open review thread: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.WsID == "" {
		return fmt.Errorf("open review thread: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c OpenReviewThread) EmitEvent(
	_ *domain.ReviewThread,
) domain.ReviewThread {
	return domain.ReviewThread{
		ID:        c.ID,
		WsID:      c.WsID,
		Status:    domain.ReviewThreadStatusOpen,
		CreatedAt: c.Now,
	}
}
```

- [ ] **Step 2: `resolve.go`** (`open → resolved`)

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ResolveReviewThread marks an open thread resolved.
type ResolveReviewThread struct {
	ID string
}

func (c ResolveReviewThread) AggregateID() string {
	return c.ID
}

func (c ResolveReviewThread) EventName() string {
	return "review_thread.resolved." + c.ID
}

func (c ResolveReviewThread) ShouldSnapshot() bool {
	return false
}

func (c ResolveReviewThread) Validate(
	current *domain.ReviewThread,
) error {
	if current == nil {
		return fmt.Errorf("resolve review thread: %w", asynxModels.ErrValidation)
	}
	if current.Status != domain.ReviewThreadStatusOpen {
		return fmt.Errorf("resolve review thread: not open: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ResolveReviewThread) EmitEvent(
	current *domain.ReviewThread,
) domain.ReviewThread {
	thread := *current
	thread.Status = domain.ReviewThreadStatusResolved
	return thread
}
```

- [ ] **Step 3: `reopen.go`** (`resolved → open`)

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ReopenReviewThread re-opens a resolved thread.
type ReopenReviewThread struct {
	ID string
}

func (c ReopenReviewThread) AggregateID() string {
	return c.ID
}

func (c ReopenReviewThread) EventName() string {
	return "review_thread.reopened." + c.ID
}

func (c ReopenReviewThread) ShouldSnapshot() bool {
	return false
}

func (c ReopenReviewThread) Validate(
	current *domain.ReviewThread,
) error {
	if current == nil {
		return fmt.Errorf("reopen review thread: %w", asynxModels.ErrValidation)
	}
	if current.Status != domain.ReviewThreadStatusResolved {
		return fmt.Errorf("reopen review thread: not resolved: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ReopenReviewThread) EmitEvent(
	current *domain.ReviewThread,
) domain.ReviewThread {
	thread := *current
	thread.Status = domain.ReviewThreadStatusOpen
	return thread
}
```

- [ ] **Step 4: `commands_test.go`**

```go
package commands

import (
	"errors"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestOpen_EmitsOpen(t *testing.T) {
	th := OpenReviewThread{ID: "t1", WsID: "w1", Now: time.Unix(1, 0)}.EmitEvent(nil)
	assert.Equal(t, domain.ReviewThreadStatusOpen, th.Status)
}

func TestResolve_RequiresOpen(t *testing.T) {
	err := ResolveReviewThread{ID: "t1"}.Validate(&domain.ReviewThread{Status: domain.ReviewThreadStatusResolved})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
	th := ResolveReviewThread{ID: "t1"}.EmitEvent(&domain.ReviewThread{Status: domain.ReviewThreadStatusOpen})
	assert.Equal(t, domain.ReviewThreadStatusResolved, th.Status)
}

func TestReopen_RequiresResolved(t *testing.T) {
	err := ReopenReviewThread{ID: "t1"}.Validate(&domain.ReviewThread{Status: domain.ReviewThreadStatusOpen})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
	th := ReopenReviewThread{ID: "t1"}.EmitEvent(&domain.ReviewThread{Status: domain.ReviewThreadStatusResolved})
	assert.Equal(t, domain.ReviewThreadStatusOpen, th.Status)
}

func TestReviewThread_Metadata(t *testing.T) {
	assert.Equal(t, "t1", OpenReviewThread{ID: "t1"}.AggregateID())
	assert.Contains(t, ResolveReviewThread{ID: "t1"}.EventName(), "resolved")
	assert.Contains(t, ReopenReviewThread{ID: "t1"}.EventName(), "reopened")
}
```

- [ ] **Step 5: `reviewthread.go`** (repository)

```go
package reviewthread

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ReviewThread is the review-thread aggregate repository.
type ReviewThread interface {
	Open(
		ctx context.Context,
		id string,
		wsID string,
		now time.Time,
	) (domain.ReviewThread, error)
	Resolve(
		ctx context.Context,
		id string,
	) (domain.ReviewThread, error)
	Reopen(
		ctx context.Context,
		id string,
	) (domain.ReviewThread, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.ReviewThread, error)
}

type reviewThread struct {
	ax asynx.Asynx[domain.ReviewThread]
}

// New builds a ReviewThread repository over the given asynx instance.
func New(
	ax asynx.Asynx[domain.ReviewThread],
) ReviewThread {
	return &reviewThread{ax: ax}
}

func (r *reviewThread) Open(
	ctx context.Context,
	id string,
	wsID string,
	now time.Time,
) (domain.ReviewThread, error) {
	evt, err := r.ax.SendWait(ctx, commands.OpenReviewThread{ID: id, WsID: wsID, Now: now})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: open: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *reviewThread) Resolve(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	evt, err := r.ax.SendWait(ctx, commands.ResolveReviewThread{ID: id})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: resolve: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *reviewThread) Reopen(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	evt, err := r.ax.SendWait(ctx, commands.ReopenReviewThread{ID: id})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: reopen: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *reviewThread) Get(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	got, err := r.ax.Get(ctx, id)
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: get: %w", err)
	}
	return got, nil
}
```

- [ ] **Step 6: `reviewthread_test.go`** (open→resolve→reopen round-trip)

```go
package reviewthread_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newRepo(t *testing.T) (context.Context, reviewthread.ReviewThread) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ReviewThread]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return context.Background(), reviewthread.New(ax)
}

func TestReviewThread_OpenResolveReopen(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Open(ctx, "t1", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	resolved, err := repo.Resolve(ctx, "t1")
	require.NoError(t, err)
	assert.Equal(t, domain.ReviewThreadStatusResolved, resolved.Status)
	reopened, err := repo.Reopen(ctx, "t1")
	require.NoError(t, err)
	assert.Equal(t, domain.ReviewThreadStatusOpen, reopened.Status)
}
```

- [ ] **Step 7: Run tests + commit**

Run: `go test ./internal/app/repositories/reviewthread/...`
Expected: PASS.

```bash
git add internal/app/repositories/reviewthread
git commit -m "feat(app): ReviewThread repository + open/resolve/reopen commands"
```

### Task 17: Crash-recovery scaffold + repositories container

First extend Chat with the `idle → agent-running` transition (the other half of its `00` §6.1 state machine) so the reconcile path is exercisable, then build recovery and the container.

**Files:**
- Create: `api/internal/app/repositories/chat/internal/commands/set_agent_running.go`
- Modify: `api/internal/app/repositories/chat/internal/commands/commands_test.go` (add a case)
- Modify: `api/internal/app/repositories/chat/chat.go` (add `SetAgentRunning`)
- Modify: `api/internal/app/repositories/chat/chat_test.go` (add a case)
- Create: `api/internal/app/repositories/recovery.go`
- Test: `api/internal/app/repositories/recovery_test.go`
- Create: `api/internal/app/repositories/container.go`
- Test: `api/internal/app/repositories/container_test.go`

- [ ] **Step 1: `set_agent_running.go`** (`idle → agent-running`)

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetChatAgentRunning advances an idle Chat to agent-running.
type SetChatAgentRunning struct {
	ID string
}

func (c SetChatAgentRunning) AggregateID() string {
	return c.ID
}

func (c SetChatAgentRunning) EventName() string {
	return "chat.agent_running." + c.ID
}

func (c SetChatAgentRunning) ShouldSnapshot() bool {
	return false
}

func (c SetChatAgentRunning) Validate(
	current *domain.Chat,
) error {
	if current == nil {
		return fmt.Errorf("set chat agent running: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetChatAgentRunning) EmitEvent(
	current *domain.Chat,
) domain.Chat {
	chat := *current
	chat.Status = domain.ChatStatusAgentRunning
	return chat
}
```

- [ ] **Step 2: Append a command test** to `chat/internal/commands/commands_test.go`

```go
func TestSetAgentRunning_FromIdle(t *testing.T) {
	chat := SetChatAgentRunning{ID: "c1"}.EmitEvent(&domain.Chat{Status: domain.ChatStatusIdle})
	assert.Equal(t, domain.ChatStatusAgentRunning, chat.Status)
}
```

- [ ] **Step 3: Extend the Chat repository** — add to the `Chat` interface in `chat.go` (after `ResetIdle`):

```go
	SetAgentRunning(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
```

and the implementation method:

```go
func (c *chat) SetAgentRunning(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	evt, err := c.ax.SendWait(ctx, commands.SetChatAgentRunning{ID: id})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: set agent running: %w", err)
	}
	return evt.Aggregate, nil
}
```

- [ ] **Step 4: Append a repo test** to `chat/chat_test.go`

```go
func TestChat_SetAgentRunning_RoundTrips(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, "c1", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	running, err := repo.SetAgentRunning(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusAgentRunning, running.Status)
}
```

Run: `go test ./internal/app/repositories/chat/...` → PASS.

- [ ] **Step 5: `recovery.go`** (running→error, then idempotent chat reconcile; per-item helpers keep ≤2 levels — `00` §6.2)

```go
package repositories

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// RecoverAgentRuns drives any candidate AgentRun still in running to error. Uses
// SendWait so all recovery events drain before the caller proceeds (00 §6.2).
func RecoverAgentRuns(
	ctx context.Context,
	candidateIDs []string,
	runs agentrun.AgentRun,
) {
	for _, id := range candidateIDs {
		recoverOneRun(ctx, id, runs)
	}
}

func recoverOneRun(
	ctx context.Context,
	id string,
	runs agentrun.AgentRun,
) {
	run, err := runs.Get(ctx, id)
	if err != nil {
		slog.WarnContext(ctx, "crash recovery: get agent run", "id", id, "err", err)
		return
	}
	if run.Status != domain.AgentRunStatusRunning {
		return
	}
	if _, failErr := runs.Fail(ctx, id); failErr != nil {
		slog.WarnContext(ctx, "crash recovery: fail agent run", "id", id, "err", failErr)
	}
}

// ReconcileChats resets any candidate Chat stuck in agent-running with no live run
// back to idle. ResetIdle is idempotent, so this is safe to run after AgentRun
// recovery regardless of which pass clears a given chat first (00 §6.2).
func ReconcileChats(
	ctx context.Context,
	candidateIDs []string,
	hasLiveRun func(chatID string) bool,
	chats chat.Chat,
) {
	for _, id := range candidateIDs {
		reconcileOneChat(ctx, id, hasLiveRun, chats)
	}
}

func reconcileOneChat(
	ctx context.Context,
	id string,
	hasLiveRun func(chatID string) bool,
	chats chat.Chat,
) {
	c, err := chats.Get(ctx, id)
	if err != nil {
		slog.WarnContext(ctx, "crash recovery: get chat", "id", id, "err", err)
		return
	}
	if c.Status != domain.ChatStatusAgentRunning || hasLiveRun(id) {
		return
	}
	if _, resetErr := chats.ResetIdle(ctx, id); resetErr != nil {
		slog.WarnContext(ctx, "crash recovery: reset chat", "id", id, "err", resetErr)
	}
}
```

- [ ] **Step 6: `recovery_test.go`** (seed running run + agent-running chat; assert recovered; assert idempotent)

```go
package repositories_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newAgentRunRepo(t *testing.T) agentrun.AgentRun {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRun]().WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return agentrun.New(ax)
}

func newChatRepo(t *testing.T) chat.Chat {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return chat.New(ax)
}

func TestRecoverAgentRuns_RunningBecomesError(t *testing.T) {
	ctx := context.Background()
	runs := newAgentRunRepo(t)
	_, err := runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = runs.MarkRunning(ctx, "a1")
	require.NoError(t, err)

	repositories.RecoverAgentRuns(ctx, []string{"a1"}, runs)

	got, err := runs.Get(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusError, got.Status)
}

func TestRecoverAgentRuns_NonRunningUntouched(t *testing.T) {
	ctx := context.Background()
	runs := newAgentRunRepo(t)
	_, err := runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)

	repositories.RecoverAgentRuns(ctx, []string{"a1"}, runs)

	got, err := runs.Get(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusPending, got.Status)
}

func TestReconcileChats_AgentRunningWithNoLiveRun_ResetToIdle(t *testing.T) {
	ctx := context.Background()
	chats := newChatRepo(t)
	_, err := chats.Create(ctx, "c1", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = chats.SetAgentRunning(ctx, "c1")
	require.NoError(t, err)

	repositories.ReconcileChats(ctx, []string{"c1"}, func(string) bool { return false }, chats)

	got, err := chats.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusIdle, got.Status)
}

func TestReconcileChats_LiveRunKeepsAgentRunning(t *testing.T) {
	ctx := context.Background()
	chats := newChatRepo(t)
	_, err := chats.Create(ctx, "c1", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = chats.SetAgentRunning(ctx, "c1")
	require.NoError(t, err)

	repositories.ReconcileChats(ctx, []string{"c1"}, func(string) bool { return true }, chats)

	got, err := chats.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusAgentRunning, got.Status)
}

func TestReconcileChats_SecondPassIsIdempotent(t *testing.T) {
	ctx := context.Background()
	chats := newChatRepo(t)
	_, err := chats.Create(ctx, "c1", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = chats.SetAgentRunning(ctx, "c1")
	require.NoError(t, err)

	repositories.ReconcileChats(ctx, []string{"c1"}, func(string) bool { return false }, chats)
	repositories.ReconcileChats(ctx, []string{"c1"}, func(string) bool { return false }, chats)

	got, err := chats.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusIdle, got.Status)
}
```

- [ ] **Step 7: `container.go`** (build 4 repos; `RegisterHubProjections` stub; `RecoverOrphans`)

```go
package repositories

import (
	"context"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Container holds the four aggregate repositories.
type Container struct {
	Workspace    workspace.Workspace
	Chat         chat.Chat
	AgentRun     agentrun.AgentRun
	ReviewThread reviewthread.ReviewThread
}

// New builds all aggregate repositories from their asynx instances.
func New(
	axWorkspace asynx.Asynx[domain.Workspace],
	axChat asynx.Asynx[domain.Chat],
	axAgentRun asynx.Asynx[domain.AgentRun],
	axReviewThread asynx.Asynx[domain.ReviewThread],
) *Container {
	return &Container{
		Workspace:    workspace.New(axWorkspace),
		Chat:         chat.New(axChat),
		AgentRun:     agentrun.New(axAgentRun),
		ReviewThread: reviewthread.New(axReviewThread),
	}
}

// RegisterHubProjections is the Wave-0 stub. Asynx subscriptions → hub.BroadcastX
// are wired fully in Wave 3 (03 §7).
func (c *Container) RegisterHubProjections(
	_ hub.WebSocketHub,
) error {
	return nil
}

// RecoverOrphans runs AgentRun crash recovery (running→error) and then the
// idempotent chat reconcile. Wave 0 has no read model to enumerate candidates, so
// the ID lists are empty; the enumerators are wired in a later wave (00 §6.2).
func (c *Container) RecoverOrphans(
	ctx context.Context,
) {
	RecoverAgentRuns(ctx, nil, c.AgentRun)
	ReconcileChats(ctx, nil, func(string) bool { return false }, c.Chat)
}
```

- [ ] **Step 8: `container_test.go`** (constructs the four repos + stub/recover no-op)

```go
package repositories_test

import (
	"context"
	"testing"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func ax[T any](t *testing.T) asynx.Asynx[T] {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	a, err := asynx.New[T]().WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })
	return a
}

func TestContainer_New_BuildsRepos(t *testing.T) {
	c := repositories.New(
		ax[domain.Workspace](t),
		ax[domain.Chat](t),
		ax[domain.AgentRun](t),
		ax[domain.ReviewThread](t),
	)
	assert.NotNil(t, c.Workspace)
	assert.NotNil(t, c.Chat)
	assert.NotNil(t, c.AgentRun)
	assert.NotNil(t, c.ReviewThread)
}

func TestContainer_RegisterHubProjections_StubNoError(t *testing.T) {
	c := repositories.New(
		ax[domain.Workspace](t),
		ax[domain.Chat](t),
		ax[domain.AgentRun](t),
		ax[domain.ReviewThread](t),
	)
	assert.NoError(t, c.RegisterHubProjections(hub.NewHub()))
}

func TestContainer_RecoverOrphans_EmptyIsNoOp(t *testing.T) {
	c := repositories.New(
		ax[domain.Workspace](t),
		ax[domain.Chat](t),
		ax[domain.AgentRun](t),
		ax[domain.ReviewThread](t),
	)
	assert.NotPanics(t, func() { c.RecoverOrphans(context.Background()) })
}
```

- [ ] **Step 9: Run + commit**

Run: `go test ./internal/app/repositories/...`
Expected: PASS.

```bash
git add internal/app/repositories
git commit -m "feat(app): crash-recovery scaffold + repositories container + hub-projection stub"
```

### Task 18: app container (asynx helper + GORM stores + wiring + recovery)

Mirrors `quiver.core/internal/app/container.go`: build asynx instances from the adapter event stores, build the GORM CRUD stores (migrating their tables), build the hub, build repositories, call `RegisterHubProjections`, run crash recovery synchronously, return.

**Files:**
- Create: `api/internal/app/asynx.go`
- Test: `api/internal/app/asynx_test.go`
- Create: `api/internal/app/gorm.go`
- Test: `api/internal/app/gorm_test.go`
- Create: `api/internal/app/container.go`
- Test: `api/internal/app/container_test.go`

- [ ] **Step 1: `asynx.go`** (the 8-shard/queue-1000 helper)

```go
package app

import (
	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
)

func newAsynx[T any](
	es asynxModels.Store,
) (asynx.Asynx[T], error) {
	return asynx.New[T]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}
```

- [ ] **Step 2: `asynx_test.go`**

```go
package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestNewAsynx_BuildsInstance(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := newAsynx[domain.Workspace](es)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	assert.NotNil(t, ax)
}
```

- [ ] **Step 3: `gorm.go`** (the three CRUD stores; migrates their tables)

```go
package app

import (
	"fmt"

	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// GORMStores holds the plain-CRUD repositories backed by the shared GORM DB.
type GORMStores struct {
	Projects         store.Store[domain.Project, string]
	Repositories     store.Store[domain.Repository, string]
	TerminalProfiles store.Store[domain.TerminalProfile, string]
}

func newGORMStores(
	db *gormdb.DB,
) (*GORMStores, error) {
	projects, err := storesqlite.NewFromDB[domain.Project, string](db)
	if err != nil {
		return nil, fmt.Errorf("app: project store: %w", err)
	}
	repos, err := storesqlite.NewFromDB[domain.Repository, string](db)
	if err != nil {
		return nil, fmt.Errorf("app: repository store: %w", err)
	}
	profiles, err := storesqlite.NewFromDB[domain.TerminalProfile, string](db)
	if err != nil {
		return nil, fmt.Errorf("app: terminal profile store: %w", err)
	}
	return &GORMStores{
		Projects:         projects,
		Repositories:     repos,
		TerminalProfiles: profiles,
	}, nil
}
```

- [ ] **Step 4: `gorm_test.go`** (Project round-trips through the GORM store)

```go
package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestNewGORMStores_ProjectRoundTrips(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	stores, err := newGORMStores(db)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, stores.Projects.Save(ctx, domain.Project{ID: "p1", Name: "Alpha"}))
	got, err := stores.Projects.FindByKey(ctx, "p1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Alpha", got.Name)
}
```

- [ ] **Step 5: `container.go`** (the full app wiring — recovery before return)

```go
package app

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the application layer: the hub, the aggregate repositories, and
// the GORM CRUD stores.
type Container struct {
	Hub          *hub.Hub
	Repositories *repositories.Container
	GORM         *GORMStores
}

// New constructs the application layer from the engine and adapter containers,
// wires hub projections, and runs AgentRun crash recovery synchronously before
// returning (00 §6.2, §7).
func New(
	ctx context.Context,
	_ *engine.Container,
	adapters *adapter.Container,
) (*Container, error) {
	axWorkspace, err := newAsynx[domain.Workspace](adapters.WorkspaceES)
	if err != nil {
		return nil, fmt.Errorf("app: asynx workspace: %w", err)
	}
	axChat, err := newAsynx[domain.Chat](adapters.ChatES)
	if err != nil {
		return nil, fmt.Errorf("app: asynx chat: %w", err)
	}
	axAgentRun, err := newAsynx[domain.AgentRun](adapters.AgentRunES)
	if err != nil {
		return nil, fmt.Errorf("app: asynx agent run: %w", err)
	}
	axReviewThread, err := newAsynx[domain.ReviewThread](adapters.ReviewThreadES)
	if err != nil {
		return nil, fmt.Errorf("app: asynx review thread: %w", err)
	}

	gormStores, err := newGORMStores(adapters.DB)
	if err != nil {
		return nil, err
	}

	h := hub.NewHub()
	repos := repositories.New(axWorkspace, axChat, axAgentRun, axReviewThread)
	if err := repos.RegisterHubProjections(h); err != nil {
		return nil, fmt.Errorf("app: hub projections: %w", err)
	}
	repos.RecoverOrphans(ctx)

	return &Container{
		Hub:          h,
		Repositories: repos,
		GORM:         gormStores,
	}, nil
}
```

- [ ] **Step 6: `container_test.go`** (boots the full app layer over a temp adapter)

```go
package app_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/engine"
)

func TestApp_New_BootsFullLayer(t *testing.T) {
	ctx := context.Background()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)
	assert.NotNil(t, c.Hub)
	assert.NotNil(t, c.Repositories)
	assert.NotNil(t, c.GORM)
	assert.NotNil(t, c.Repositories.Workspace)
}
```

- [ ] **Step 7: Run + commit**

Run: `go test ./internal/app/...`
Expected: PASS.

```bash
git add internal/app/asynx.go internal/app/asynx_test.go internal/app/gorm.go internal/app/gorm_test.go internal/app/container.go internal/app/container_test.go
git commit -m "feat(app): container wiring asynx instances, GORM stores, hub, recovery"
```

---

## Phase 6 — `api/` (Broadcaster framework, dispatch, health, routing)

The delivery layer. Imports `app`. We build the **generic** `Broadcaster[T]` framework, the `dispatch()` REST/WS dual-serve helper, the snapshot-on-subscribe contract hook, middleware, and `/v0/health`. Per scope we wire only the two **Class-A** topics whose hub methods already exist (Workspaces, Chats) as generic broadcasters — the Class-B/bridge topics (Git, Files, LSP, Terminal, ChatStream) and full overlay-computing snapshots are **deferred** to later waves.

### Task 19: generic Broadcaster[T] + StreamDef + filter + client (GATE WS-push)

**Files:**
- Create: `api/internal/api/v0/ws/client.go`
- Create: `api/internal/api/v0/ws/stream_def.go`
- Create: `api/internal/api/v0/ws/filter.go`
- Test: `api/internal/api/v0/ws/filter_test.go`
- Create: `api/internal/api/v0/ws/broadcaster.go`
- Test: `api/internal/api/v0/ws/broadcaster_test.go`

Reference: `quiver.core/internal/api/ws/{client.go,filter.go,broadcaster.go,broadcaster_test.go}` (copy; add the `Snapshot` hook + keep ≤2 indentation via extracted helpers).

- [ ] **Step 1: `client.go`** (copy quiver verbatim)

```go
package ws

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	pingInterval = 30 * time.Second
	pongTimeout  = 60 * time.Second
	writeTimeout = 10 * time.Second
	sendBuffer   = 64
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type client struct {
	send chan []byte
	done chan struct{}
}

func newClient() *client {
	return &client{
		send: make(chan []byte, sendBuffer),
		done: make(chan struct{}),
	}
}

func readPump(
	conn *websocket.Conn,
) {
	_ = conn.SetReadDeadline(time.Now().Add(pongTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongTimeout))
	})
	for {
		if _, _, err := conn.NextReader(); err != nil {
			return
		}
	}
}

func writePump(
	conn *websocket.Conn,
	cl *client,
) {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()
	for {
		if !writeNext(conn, cl, ticker) {
			return
		}
	}
}

func writeNext(
	conn *websocket.Conn,
	cl *client,
	ticker *time.Ticker,
) bool {
	select {
	case msg := <-cl.send:
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		return conn.WriteMessage(websocket.TextMessage, msg) == nil
	case <-ticker.C:
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		return conn.WriteMessage(websocket.PingMessage, nil) == nil
	case <-cl.done:
		return false
	}
}
```

> The `writeNext` extraction (vs quiver's inline `select`) keeps `writePump` within the 2-level indentation rule.

- [ ] **Step 2: `stream_def.go`** (StreamDef + FilterDef + the `Snapshot` contract hook)

```go
package ws

// StreamDef declares how a Broadcaster routes, serializes, filters, and
// snapshots a stream of T (03 §1, §1a).
type StreamDef[T any] struct {
	Namespace func(T) string
	Serialize func(T) ([]byte, error)
	Filters   []FilterDef[T]
	Snapshot  func() []T
}

// FilterDef is an optional query-param predicate over a stream value.
type FilterDef[T any] struct {
	Param   string
	Extract func(T) string
	Match   func(param, value string) bool
	Default string
}
```

- [ ] **Step 3: `filter.go`** (copy quiver; `ExactMatch`, `GlobMatch`, `BuildPredicate`)

```go
package ws

import (
	"path"

	"github.com/gin-gonic/gin"
)

// ExactMatch reports whether param equals value.
func ExactMatch(
	param string,
	value string,
) bool {
	return param == value
}

// GlobMatch reports whether value matches the glob pattern ("" matches all).
func GlobMatch(
	pattern string,
	value string,
) bool {
	if pattern == "" {
		return true
	}
	matched, err := path.Match(pattern, value)
	return err == nil && matched
}

type activeFilter[T any] struct {
	param string
	fd    FilterDef[T]
}

// BuildPredicate compiles the namespace glob and active query-param filters from
// the request into a single predicate over T.
func BuildPredicate[T any](
	c *gin.Context,
	def StreamDef[T],
) func(T) bool {
	nsPattern := c.Param("ns")
	active := collectFilters(c, def)
	return func(event T) bool {
		if nsPattern != "" && !GlobMatch(nsPattern, def.Namespace(event)) {
			return false
		}
		return matchesAll(active, event)
	}
}

func collectFilters[T any](
	c *gin.Context,
	def StreamDef[T],
) []activeFilter[T] {
	var active []activeFilter[T]
	for _, f := range def.Filters {
		v := c.Query(f.Param)
		if v == "" {
			v = f.Default
		}
		if v != "" {
			active = append(active, activeFilter[T]{param: v, fd: f})
		}
	}
	return active
}

func matchesAll[T any](
	active []activeFilter[T],
	event T,
) bool {
	for _, af := range active {
		if !af.fd.Match(af.param, af.fd.Extract(event)) {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: `filter_test.go`**

```go
package ws

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type row struct {
	name string
	kind string
}

func ctxWith(
	query string,
	ns string,
) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/x?"+query, nil)
	if ns != "" {
		c.Params = gin.Params{{Key: "ns", Value: ns}}
	}
	return c
}

func def() StreamDef[row] {
	return StreamDef[row]{
		Namespace: func(r row) string { return r.name },
		Filters: []FilterDef[row]{
			{Param: "kind", Extract: func(r row) string { return r.kind }, Match: ExactMatch},
		},
	}
}

func TestBuildPredicate_FilterMatches(t *testing.T) {
	p := BuildPredicate(ctxWith("kind=fruit", ""), def())
	assert.True(t, p(row{name: "apple", kind: "fruit"}))
	assert.False(t, p(row{name: "carrot", kind: "veg"}))
}

func TestBuildPredicate_NamespaceGlob(t *testing.T) {
	p := BuildPredicate(ctxWith("", "al*"), def())
	assert.True(t, p(row{name: "alpha"}))
	assert.False(t, p(row{name: "beta"}))
}

func TestBuildPredicate_NoFiltersMatchesAll(t *testing.T) {
	p := BuildPredicate(ctxWith("", ""), StreamDef[row]{Namespace: func(r row) string { return r.name }})
	assert.True(t, p(row{name: "anything"}))
}

func TestGlobMatch_EmptyPatternMatchesAll(t *testing.T) {
	assert.True(t, GlobMatch("", "x"))
}

func TestExactMatch(t *testing.T) {
	assert.True(t, ExactMatch("a", "a"))
	assert.False(t, ExactMatch("a", "b"))
}
```

- [ ] **Step 5: `broadcaster.go`** (generic; snapshot-on-subscribe under the registration lock; ≤2 levels via helpers)

```go
package ws

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

type filteredClient[T any] struct {
	*client
	predicate func(T) bool
}

// Broadcaster fans a stream of T out to filtered WebSocket clients (03 §1).
type Broadcaster[T any] struct {
	def        StreamDef[T]
	mu         sync.RWMutex
	clients    map[*filteredClient[T]]struct{}
	registered chan struct{}
	once       sync.Once
}

// NewBroadcaster builds a Broadcaster from a StreamDef.
func NewBroadcaster[T any](
	def StreamDef[T],
) *Broadcaster[T] {
	return &Broadcaster[T]{
		def:        def,
		clients:    make(map[*filteredClient[T]]struct{}),
		registered: make(chan struct{}),
	}
}

// WaitRegistered blocks until at least one client has registered. Test-only.
func (b *Broadcaster[T]) WaitRegistered() {
	<-b.registered
}

// Handle upgrades the request to a WebSocket, registers the client (delivering
// the snapshot atomically under the registration lock), then streams live events.
func (b *Broadcaster[T]) Handle(
	c *gin.Context,
) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	predicate := BuildPredicate(c, b.def)
	cl := &filteredClient[T]{client: newClient(), predicate: predicate}

	b.register(cl)
	go writePump(conn, cl.client)
	readPump(conn)
	b.remove(cl)
}

func (b *Broadcaster[T]) register(
	cl *filteredClient[T],
) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[cl] = struct{}{}
	b.pushSnapshot(cl)
	b.once.Do(func() { close(b.registered) })
}

func (b *Broadcaster[T]) remove(
	cl *filteredClient[T],
) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, cl)
	close(cl.done)
}

func (b *Broadcaster[T]) pushSnapshot(
	cl *filteredClient[T],
) {
	if b.def.Snapshot == nil {
		return
	}
	for _, item := range b.def.Snapshot() {
		b.serializeAndSend(cl, item)
	}
}

func (b *Broadcaster[T]) serializeAndSend(
	cl *filteredClient[T],
	item T,
) {
	data, err := b.def.Serialize(item)
	if err != nil {
		return
	}
	sendIfMatch(cl, item, data)
}

// Push serializes the event once and delivers it to every matching client.
func (b *Broadcaster[T]) Push(
	event T,
) {
	data, err := b.def.Serialize(event)
	if err != nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for cl := range b.clients {
		sendIfMatch(cl, event, data)
	}
}

func sendIfMatch[T any](
	cl *filteredClient[T],
	event T,
	data []byte,
) {
	if !cl.predicate(event) {
		return
	}
	select {
	case cl.send <- data:
	default:
	}
}
```

- [ ] **Step 6: `broadcaster_test.go`** (GATE: trivial Broadcaster pushes to a connected WS client; adapt quiver)

```go
package ws_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ws "github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

type item struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

func itemDef() ws.StreamDef[item] {
	return ws.StreamDef[item]{
		Namespace: func(i item) string { return i.Name },
		Serialize: func(i item) ([]byte, error) { return json.Marshal(i) },
		Filters: []ws.FilterDef[item]{
			{Param: "kind", Extract: func(i item) string { return i.Kind }, Match: ws.ExactMatch},
		},
	}
}

func setup(t *testing.T, def ws.StreamDef[item]) (*ws.Broadcaster[item], *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	b := ws.NewBroadcaster(def)
	r := gin.New()
	r.GET("/items", b.Handle)
	r.GET("/items/:ns", b.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return b, srv
}

func dial(t *testing.T, srv *httptest.Server, path string) *websocket.Conn {
	t.Helper()
	url := "ws" + srv.URL[len("http"):] + path
	conn, _, err := websocket.DefaultDialer.Dial(url, nil) //nolint:bodyclose
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func read(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(msg, v))
}

func TestBroadcaster_Push_DeliversToClient(t *testing.T) {
	b, srv := setup(t, itemDef())
	conn := dial(t, srv, "/items")
	b.WaitRegistered()

	b.Push(item{Name: "alpha", Kind: "fruit"})

	var got item
	read(t, conn, &got)
	assert.Equal(t, "alpha", got.Name)
}

func TestBroadcaster_Push_FieldFilter(t *testing.T) {
	b, srv := setup(t, itemDef())
	conn := dial(t, srv, "/items?kind=fruit")
	b.WaitRegistered()

	b.Push(item{Name: "carrot", Kind: "veg"})
	b.Push(item{Name: "apple", Kind: "fruit"})

	var got item
	read(t, conn, &got)
	assert.Equal(t, "apple", got.Name)
}

func TestBroadcaster_NamespaceGlob(t *testing.T) {
	b, srv := setup(t, itemDef())
	conn := dial(t, srv, "/items/alpha")
	b.WaitRegistered()

	b.Push(item{Name: "beta", Kind: "fruit"})
	b.Push(item{Name: "alpha", Kind: "fruit"})

	var got item
	read(t, conn, &got)
	assert.Equal(t, "alpha", got.Name)
}

func TestBroadcaster_SnapshotOnSubscribe(t *testing.T) {
	def := itemDef()
	def.Snapshot = func() []item {
		return []item{{Name: "seed", Kind: "fruit"}}
	}
	b, srv := setup(t, def)
	conn := dial(t, srv, "/items")
	b.WaitRegistered()

	var got item
	read(t, conn, &got)
	assert.Equal(t, "seed", got.Name)
}

func TestBroadcaster_UpgradeRejectsNonWS(t *testing.T) {
	_, srv := setup(t, itemDef())
	resp, err := http.Get(srv.URL + "/items") //nolint:bodyclose
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBroadcaster_SlowConsumer_DoesNotBlock(t *testing.T) {
	b, srv := setup(t, itemDef())
	_ = dial(t, srv, "/items")
	b.WaitRegistered()
	for i := 0; i < 65; i++ {
		b.Push(item{Name: fmt.Sprintf("i%d", i), Kind: "fruit"})
	}
}

func TestBroadcaster_SerializationError_SkipsDelivery(t *testing.T) {
	def := ws.StreamDef[item]{
		Namespace: func(i item) string { return i.Name },
		Serialize: func(i item) ([]byte, error) { return nil, fmt.Errorf("boom") },
	}
	b, srv := setup(t, def)
	conn := dial(t, srv, "/items")
	b.WaitRegistered()
	b.Push(item{Name: "x"})
	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	assert.Error(t, err)
}
```

- [ ] **Step 7: Run tests (race detector on — this is the gate's concurrency-sensitive test)**

Run: `go test -race ./internal/api/v0/ws/...`
Expected: PASS — proves the GATE's "Broadcaster[T] pushes to a connected WS client" requirement.

- [ ] **Step 8: Commit**

```bash
git add internal/api/v0/ws
git commit -m "feat(api): generic Broadcaster[T] + StreamDef snapshot hook + filters"
```

### Task 20: dispatch() REST/WS dual-serve helper + health route

**Files:**
- Create: `api/internal/api/v0/ws/dispatch.go`
- Test: `api/internal/api/v0/ws/dispatch_test.go`
- Create: `api/internal/api/v0/health.go`
- Test: `api/internal/api/v0/health_test.go`

- [ ] **Step 1: `dispatch.go`** (WS upgrade → `Handle`; else REST snapshot body — `03` §1a)

```go
package ws

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Dispatch serves the same URL as both a REST snapshot and a live WebSocket: a
// WebSocket upgrade request streams via the Broadcaster, any other request gets
// the JSON snapshot body (03 §1a).
func Dispatch[T any](
	b *Broadcaster[T],
	snapshot func(*gin.Context) (any, error),
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if websocket.IsWebSocketUpgrade(c.Request) {
			b.Handle(c)
			return
		}
		writeSnapshot(c, snapshot)
	}
}

func writeSnapshot(
	c *gin.Context,
	snapshot func(*gin.Context) (any, error),
) {
	body, err := snapshot(c)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, body)
}
```

- [ ] **Step 2: `dispatch_test.go`** (REST path returns snapshot; error path 500)

```go
package ws_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ws "github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

func TestDispatch_RESTReturnsSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	b := ws.NewBroadcaster(ws.StreamDef[item]{
		Namespace: func(i item) string { return i.Name },
		Serialize: func(i item) ([]byte, error) { return json.Marshal(i) },
	})
	r := gin.New()
	r.GET("/items", ws.Dispatch(b, func(*gin.Context) (any, error) {
		return []item{{Name: "snap"}}, nil
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/items", nil)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "snap")
}

func TestDispatch_SnapshotError_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	b := ws.NewBroadcaster(ws.StreamDef[item]{Namespace: func(i item) string { return i.Name }})
	r := gin.New()
	r.GET("/items", ws.Dispatch(b, func(*gin.Context) (any, error) {
		return nil, assertErr{}
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/items", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

type assertErr struct{}

func (assertErr) Error() string { return "snapshot failed" }
```

- [ ] **Step 3: `health.go`** (`GET /v0/health`)

```go
package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

func registerHealth(
	rg *gin.RouterGroup,
) {
	rg.GET("/health", healthHandler)
}

func healthHandler(
	c *gin.Context,
) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"version": metadata.GetVersion(),
	})
}
```

- [ ] **Step 4: `health_test.go`**

```go
package v0

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestHealthHandler_ReturnsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerHealth(r.Group("/v0"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v0/health", nil)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ok")
}
```

- [ ] **Step 5: Run + commit**

Run: `go test ./internal/api/v0/...`
Expected: PASS.

```bash
git add internal/api/v0/ws/dispatch.go internal/api/v0/ws/dispatch_test.go internal/api/v0/health.go internal/api/v0/health_test.go
git commit -m "feat(api): dispatch() REST/WS dual-serve helper + /v0/health"
```

### Task 21: v0 container (Workspaces + Chats broadcasters, hub Subscriber) + api container

The v0 container holds the two Class-A broadcasters, implements `hub.Subscriber` (so the app hub fans out to it), and registers its routes. The api container builds gin, mounts `/v0`, and serves static assets.

**Files:**
- Create: `api/internal/api/v0/container.go`
- Test: `api/internal/api/v0/container_test.go`
- Modify: `api/internal/api/container.go` (rewrite to mount v0 + register hub subscriber)
- Test: `api/internal/api/container_test.go`
- Modify: `api/internal/api/static.go` (treat `/v0/` as API for the SPA fallback)

- [ ] **Step 1: `v0/container.go`** (broadcasters + Subscriber + routes)

```go
package v0

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	ws "github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Container is the v0 delivery surface: the Class-A broadcasters plus REST routes.
// It implements hub.Subscriber so app-layer broadcasts reach connected clients.
type Container struct {
	workspaces *ws.Broadcaster[domain.Workspace]
	chats      *ws.Broadcaster[hub.ChatStatusEvent]
}

// New builds the v0 container and registers it as a hub subscriber.
func New(
	appContainer *app.Container,
) *Container {
	c := &Container{
		workspaces: ws.NewBroadcaster(workspacesDef()),
		chats:      ws.NewBroadcaster(chatsDef()),
	}
	appContainer.Hub.Register(c)
	return c
}

// Register mounts the v0 REST and WebSocket routes.
func (c *Container) Register(
	rg *gin.RouterGroup,
) {
	registerHealth(rg)
	rg.GET("/ws/workspaces", c.workspaces.Handle)
	rg.GET("/ws/chats", c.chats.Handle)
}

// PushWorkspace implements hub.Subscriber.
func (c *Container) PushWorkspace(
	wsRow domain.Workspace,
) {
	c.workspaces.Push(wsRow)
}

// PushChat implements hub.Subscriber.
func (c *Container) PushChat(
	evt hub.ChatStatusEvent,
) {
	c.chats.Push(evt)
}

func workspacesDef() ws.StreamDef[domain.Workspace] {
	return ws.StreamDef[domain.Workspace]{
		Namespace: func(w domain.Workspace) string { return w.ID },
		Serialize: func(w domain.Workspace) ([]byte, error) { return json.Marshal(w) },
		Filters: []ws.FilterDef[domain.Workspace]{
			{Param: "projectId", Extract: func(w domain.Workspace) string { return w.ProjectID }, Match: ws.ExactMatch},
			{Param: "repoId", Extract: func(w domain.Workspace) string { return w.RepoID }, Match: ws.ExactMatch},
		},
	}
}

func chatsDef() ws.StreamDef[hub.ChatStatusEvent] {
	return ws.StreamDef[hub.ChatStatusEvent]{
		Namespace: func(e hub.ChatStatusEvent) string { return e.ChatID },
		Serialize: func(e hub.ChatStatusEvent) ([]byte, error) { return json.Marshal(e) },
		Filters: []ws.FilterDef[hub.ChatStatusEvent]{
			{Param: "wsId", Extract: func(e hub.ChatStatusEvent) string { return e.WsID }, Match: ws.ExactMatch},
		},
	}
}

var _ hub.Subscriber = (*Container)(nil)
```

- [ ] **Step 2: `v0/container_test.go`** (subscriber registered → hub broadcast reaches a connected WS client)

```go
package v0_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/engine"
)

func newApp(t *testing.T) *app.Container {
	t.Helper()
	ctx := context.Background()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	a, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)
	return a
}

func TestV0_HubBroadcastReachesWSClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newApp(t)
	c := v0.New(a)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/v0/ws/workspaces"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil) //nolint:bodyclose
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	c.WaitWorkspacesRegistered()

	a.Hub.BroadcastWorkspace(workspaceFixture())

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(msg, &got))
	assert.Equal(t, "w1", got["id"])
}
```

> This test needs a `WaitWorkspacesRegistered()` accessor and a `workspaceFixture()` helper. Add to `v0/container.go`:
> ```go
> // WaitWorkspacesRegistered blocks until a workspaces client registers. Test-only.
> func (c *Container) WaitWorkspacesRegistered() {
> 	c.workspaces.WaitRegistered()
> }
> ```
> and define `workspaceFixture()` in the test file:
> ```go
> func workspaceFixture() domain.Workspace {
> 	return domain.Workspace{ID: "w1", RepoID: "r1", ProjectID: "p1"}
> }
> ```
> (add the `domain` import to the test).

- [ ] **Step 3: Rewrite `internal/api/container.go`** (mount `/v0`, keep middleware + static)

```go
package api

import (
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"

	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/api/middleware"
	"github.com/char2cs/crowbar/api/internal/app"
)

// Container owns the configured gin engine.
type Container struct {
	router *gin.Engine
}

// New builds the HTTP layer: middleware, the v0 surface (mounted at /v0 and
// registered as a hub subscriber), and optional embedded static assets.
func New(
	appContainer *app.Container,
	staticFS fs.FS,
) (*Container, error) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.UseRawPath = true
	router.UnescapePathValues = true
	router.Use(middleware.Logger(), middleware.Recovery())

	v0Container := v0.New(appContainer)
	v0Container.Register(router.Group("/v0"))

	if staticFS != nil {
		RegisterStatic(router, staticFS)
	}

	return &Container{router: router}, nil
}

// Handler returns the underlying http.Handler.
func (c *Container) Handler() http.Handler {
	return c.router
}
```

- [ ] **Step 4: `internal/api/container_test.go`** (health reachable end-to-end through the gin engine)

```go
package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	crowbarapi "github.com/char2cs/crowbar/api/internal/api"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/engine"
)

func TestAPI_New_HealthRoute(t *testing.T) {
	ctx := context.Background()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	a, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)

	c, err := crowbarapi.New(a, nil)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v0/health", nil)
	c.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ok")
}
```

- [ ] **Step 5: Update `static.go`** — change the API-prefix guard so `/v0/...` 404s as JSON instead of falling back to `index.html`:

Replace the line `if strings.HasPrefix(path, "/api/") {` with:

```go
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/v0/") {
```

- [ ] **Step 6: Run + commit**

Run: `go test ./internal/api/...`
Expected: PASS.

```bash
git add internal/api
git commit -m "feat(api): v0 container (Workspaces+Chats broadcasters) + container + static guard"
```

---

## Phase 7 — Root wiring, CLI, and GATE verification

### Task 22: engine WithHomeDir + root `internal.go` wiring

**Files:**
- Modify: `api/internal/engine/container.go`
- Test: `api/internal/engine/engine_test.go` (verify/adjust)
- Modify: `api/internal/internal.go`
- Test: `api/internal/internal_test.go`

- [ ] **Step 1: Add a `WithHomeDir` option to engine** (no-op now; parity for later waves)

Replace `internal/engine/container.go` with:

```go
package engine

import "context"

// Container holds engine-layer dependencies. The AI Bridge engine and addon
// registry are added in later waves.
type Container struct{}

type engineOpts struct {
	homeDir string
}

// Option configures engine.New.
type Option func(*engineOpts)

// WithHomeDir overrides the home directory used for path resolution.
func WithHomeDir(
	dir string,
) Option {
	return func(o *engineOpts) {
		o.homeDir = dir
	}
}

// New constructs the engine container.
func New(
	_ context.Context,
	opts ...Option,
) (*Container, error) {
	cfg := engineOpts{}
	for _, o := range opts {
		o(&cfg)
	}
	_ = cfg
	return &Container{}, nil
}
```

- [ ] **Step 2: Verify `engine_test.go`** still passes (adjust if it referenced removed symbols)

Run: `go test ./internal/engine/...`
Expected: PASS. If the existing test fails to compile, replace it with:

```go
package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_ReturnsContainer(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir("/tmp/x"))
	require.NoError(t, err)
	assert.NotNil(t, c)
}
```

- [ ] **Step 3: Rewrite `internal/internal.go`** (engine→adapter→app→api order; homeDir → adapter; close listener)

```go
package internal

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter"
	crowbarapi "github.com/char2cs/crowbar/api/internal/api"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/core/gateway"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the root container owning the HTTP server, its listener, and the
// adapter layer (closed on shutdown).
type Container struct {
	server   *http.Server
	listener net.Listener
	adapter  *adapter.Container
}

type rootOpts struct {
	homeDir string
}

// Option configures internal.New.
type Option func(*rootOpts)

// WithHomeDir roots all on-disk state under dir (test isolation).
func WithHomeDir(
	dir string,
) Option {
	return func(o *rootOpts) {
		o.homeDir = dir
	}
}

// New wires engine → adapter → app → api in order and returns the root container.
func New(
	ctx context.Context,
	host string,
	staticFS fs.FS,
	options ...Option,
) (*Container, error) {
	cfg := rootOpts{}
	for _, o := range options {
		o(&cfg)
	}

	listener, err := gateway.New(host)
	if err != nil {
		return nil, fmt.Errorf("internal: gateway: %w", err)
	}

	engines, err := engine.New(ctx, engine.WithHomeDir(cfg.homeDir))
	if err != nil {
		return nil, fmt.Errorf("internal: engine: %w", err)
	}

	adapters, err := adapter.New(adapterOptions(cfg.homeDir)...)
	if err != nil {
		return nil, fmt.Errorf("internal: adapter: %w", err)
	}

	appContainer, err := app.New(ctx, engines, adapters)
	if err != nil {
		return nil, fmt.Errorf("internal: app: %w", err)
	}

	apiContainer, err := crowbarapi.New(appContainer, staticFS)
	if err != nil {
		return nil, fmt.Errorf("internal: api: %w", err)
	}

	return &Container{
		server:   &http.Server{Handler: apiContainer.Handler(), ReadHeaderTimeout: 30 * time.Second},
		listener: listener,
		adapter:  adapters,
	}, nil
}

func adapterOptions(
	homeDir string,
) []adapter.Option {
	if homeDir == "" {
		return nil
	}
	return []adapter.Option{adapter.WithHomeDir(homeDir)}
}

// Run serves until ctx is cancelled, then gracefully shuts down.
func (c *Container) Run(
	ctx context.Context,
) error {
	go c.server.Serve(c.listener) //nolint:errcheck
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.server.Shutdown(shutdownCtx)
}

// Close releases the adapter layer and the listener.
func (c *Container) Close() {
	_ = c.adapter.Close()
	_ = c.listener.Close()
}
```

- [ ] **Step 4: `internal/internal_test.go`** (boots the full container over an ephemeral TCP port; no Run/Sleep)

```go
package internal_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal"
)

func TestNew_BootsFullStack(t *testing.T) {
	c, err := internal.New(
		context.Background(),
		"tcp://127.0.0.1:0",
		nil,
		internal.WithHomeDir(t.TempDir()),
	)
	require.NoError(t, err)
	t.Cleanup(c.Close)
	assert.NotNil(t, c)
}

func TestNew_InvalidHost_ReturnsError(t *testing.T) {
	_, err := internal.New(context.Background(), "bogus://x", nil, internal.WithHomeDir(t.TempDir()))
	assert.Error(t, err)
}
```

- [ ] **Step 5: Run + commit**

Run: `go test ./internal/...`
Expected: PASS.

```bash
git add internal/engine internal/internal.go internal/internal_test.go
git commit -m "feat(internal): root container wiring engine→adapter→app→api with WithHomeDir"
```

### Task 23: CLI adjustments

**Files:**
- Modify: `api/cmd/crowbar/main.go`
- Test: `api/cmd/crowbar/main_test.go` (verify/adjust)

- [ ] **Step 1: Point `version` at metadata** — replace the `versionCmd.Run` body in `main.go`:

```go
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("crowbar " + metadata.GetVersion())
	},
```

and add the import `"github.com/char2cs/crowbar/api/internal/core/metadata"`. The rest of `main.go` (the `serve` command calling `internal.New(ctx, host, staticFS)`) is unchanged — the new variadic `Option` parameter is backward compatible.

- [ ] **Step 2: Verify `main_test.go`** compiles & passes

Run: `go test ./cmd/...`
Expected: PASS. If `main_test.go` references deleted scaffold, reduce it to:

```go
package main

import "testing"

func TestRootCommand_HasServe(t *testing.T) {
	if _, _, err := rootCmd.Find([]string{"serve"}); err != nil {
		t.Fatalf("serve command missing: %v", err)
	}
}
```

- [ ] **Step 3: Confirm the binary builds & boots** (dev mode, no embedded web)

Run:
```bash
go build -tags noEmbed -o /tmp/crowbar-smoke ./cmd/crowbar && /tmp/crowbar-smoke version
```
Expected: prints `crowbar 0.1.0`.

- [ ] **Step 4: Commit**

```bash
git add cmd/crowbar
git commit -m "feat(cmd): version reads metadata; CLI boots on new wiring"
```

### Task 24: GATE 0 verification

No new code — prove every Definition-of-Done item, capture the output, and report.

- [ ] **Step 1: Build, vet, lint all clean**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-resonant-niobium-4734/api
go build ./...
go vet ./...
make lint
```
Expected: all exit 0, no findings.

- [ ] **Step 2: Full race-enabled test suite + coverage ≥95%**

```bash
make test
make test-coverage
```
Expected: all PASS; coverage line prints `Coverage NN% OK` with NN ≥ 95.

- [ ] **Step 3: Confirm test-file discipline (one `_test.go` per source file)**

```bash
make missing-tests
```
Expected: no output (struct-only domain files are exempt — confirm any listed file is struct-only).

- [ ] **Step 4: Confirm zero `time.Sleep` in tests and no `rabbytesoftware` imports**

```bash
grep -rn "time.Sleep" --include="*_test.go" . && echo "FAIL: sleep found" || echo "OK: no sleeps"
grep -rl "rabbytesoftware" --include="*.go" . && echo "FAIL: legacy import" || echo "OK: clean"
```
Expected: `OK: no sleeps` and `OK: clean`.

- [ ] **Step 5: Demonstrate the Asynx round-trip explicitly** (gate item)

```bash
go test -run 'TestWorkspace_Create_RoundTrips|TestWorkspace_SyncClearsNewStatus' -v ./internal/app/repositories/workspace/...
```
Expected: both PASS — command sent → event persisted → reload reconstructs state, with the `new→null` guard.

- [ ] **Step 6: Demonstrate the Broadcaster→WS push** (gate item)

```bash
go test -race -run 'TestBroadcaster_Push_DeliversToClient|TestV0_HubBroadcastReachesWSClient' -v ./internal/api/...
```
Expected: both PASS — a `Broadcaster[T]` (and the full hub→subscriber→broadcaster path) delivers to a connected WS client.

- [ ] **Step 7: Demonstrate `GET /v0/health`** (gate item; via the api container test)

```bash
go test -run TestAPI_New_HealthRoute -v ./internal/api/...
```
Expected: PASS (HTTP 200, body contains `ok`).

- [ ] **Step 8: Confirm containers boot from `cmd/crowbar`** (gate item)

```bash
go test -run TestNew_BootsFullStack -v ./internal/...
```
Expected: PASS — full engine→adapter→app→api stack constructs.

- [ ] **Step 9: Final commit + report**

```bash
git add -A
git commit -m "test(wave-0): GATE 0 verification green"
```

Then write the GATE report (below).

## GATE 0 — Report Template

Fill this in after Task 24 and hand it back:

```
WAVE 0 — FOUNDATION: GATE REPORT

Built:
- core/{metadata,paths,config} — embedded identity, lazy-mkdir paths, intelligence→model config
- domain/ — all 00 §5 aggregates + GORM entities + status/merge enums + state machines
- adapter/ — 4 SQLite event stores + generic GORM Store[T,K] + container (WithHomeDir)
- app/ — 4 Asynx instances, typed WebSocketHub fan-out, 4 aggregate repositories with
  pure commands, crash-recovery scaffold (running→error + idempotent chat reconcile),
  RegisterHubProjections stub, GORM CRUD stores
- api/ — generic Broadcaster[T] + StreamDef snapshot hook + filters + dispatch() dual-serve,
  middleware, /v0/health, v0 container wiring Workspaces+Chats as hub subscribers
- internal.go — engine→adapter→app→api wiring with WithHomeDir; cmd/crowbar boots

Reproduce the gate:
  cd api && go build ./... && go vet ./... && make lint && make test && make test-coverage
  (individual gate proofs: Task 24 steps 5–8)

Coverage: <paste `make test-coverage` line>

Ambiguities encountered in 00 / 03:
- <e.g. AgentRun read-model for recovery enumeration is undefined in Wave 0 — recovery
  is built as a tested scaffold taking explicit candidate IDs; production enumerator
  deferred to the wave that introduces an AgentRun read model.>
- <e.g. "seven concrete topics" vs needing a hub Subscriber: wired only the two Class-A
  topics (Workspaces, Chats) whose hub methods exist; Class-B/bridge topics deferred.>
- <add any others discovered during implementation.>
```

---

## Self-Review (performed against `00` + `03`)

**Spec coverage:**
- `00` §2 layer order → internal.go (Task 22), per-layer containers. ✓
- `00` §3 UUID IDs → all aggregate/entity IDs are `string` UUIDs (callers generate). ✓
- `00` §4 storage tiers → Asynx event stores (Task 10/11) + GORM (Task 11/18). ✓
- `00` §5 entities → all types (Tasks 7–9); TerminalProfile `[]string` via json serializer. ✓
- `00` §5.3 Workspace single-source-of-truth + SyncWorkingTreeState semantics → Task 13. ✓
- `00` §6.1 Workspace state machine (new→null guard, no status stomp, clamp ≥0) → Task 13. ✓
- `00` §6.2 AgentRun crash recovery (running→error, idempotent chat reconcile, drain order) → Tasks 14, 17. ✓
- `00` §6.3 ReviewThread open↔resolved↔open → Task 16. ✓
- `00` §7 DI wiring + RegisterHubProjections stub + recovery before app.New returns → Tasks 17, 18. ✓
- `03` §1 Hub/Broadcaster/projection pattern → Tasks 12, 19. ✓
- `03` §1a snapshot-on-subscribe contract hook under registration lock → Task 19 (`pushSnapshot` in `register`). ✓
- `03` §2 two producer classes (framework only; Class-A wired) → Task 21. ✓
- `03` §6 lazy per-subscription resources → deferred (needs concrete watcher/LSP; out of Wave 0 scope, noted). ✓ (documented deferral)

**Deferred (explicitly out of Wave 0 scope, per the prompt):** the seven concrete broadcasters' full StreamDefs with overlay-computing snapshots, Class-B producers (FileWatcher/LSP/PTY), the Git Provider engine `SyncProviderState`, and full `RegisterHubProjections` (Wave 3).

**Placeholder scan:** none — every code step contains complete, compilable source.

**Type consistency:** repository method names (`Create`, `SyncWorkingTreeState`, `Get`, `MarkRunning`, `Complete`, `Fail`, `ResetIdle`, `SetAgentRunning`, `Open`, `Resolve`, `Reopen`), command struct names, `hub.ChatStatusEvent`, `ws.StreamDef.Snapshot`, and `evt.Aggregate` (verified against asynx v0.6.2) are used consistently across tasks.
