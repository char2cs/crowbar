# Agentic Engine & Provider-Switching Core — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a provider-agnostic Go engine that drives real interactive agentic CLIs (Claude Code, Codex) in a PTY, owns a "chat" tracked across provider segments, detects conversation moves via hooks, and switches provider mid-conversation with a context handoff — all provider knowledge confined to one YAML descriptor per CLI.

**Architecture:** A new `engine/agent` package interprets a per-provider YAML **descriptor** (spawn/inject/hooks/transcript/handoff) with a generic step-runner; agentic segments are spawned as **terminal sessions** (reusing the battle-tested `engine/terminal` PTY host) with custom argv+env. Vendor CLIs call back via a new `crowbar hook` subcommand (a thin unix-socket client) → `/v0/agent/hooks` → a single serialized **detection reducer** that owns chat/segment state (gorm) + an append-only file **ledger**. A **switch** reads the outgoing transcript, snapshots the ledger, gracefully quits the CLI, and re-spawns the target with the handoff injected as a spawn-time arg.

**Tech Stack:** Go 1.x, gin (HTTP), gorilla/websocket, gorm + glebarez/sqlite (single-conn), creack/pty, charmbracelet/x/vt (screen model), cobra (CLI), testify (tests). Module path `github.com/char2cs/crowbar/api`.

---

## Global Constraints

- **Interactive PTY only, never headless.** The engine MUST refuse `-p`/`--print` (Claude) and `exec` (Codex). Enforced from the descriptor `spawn.forbid_flags`.
- **Crowbar never writes to the PTY.** Orchestration is spawn-time args + read-only side channels (hooks + transcript) + process lifecycle only. No "send a message" path anywhere.
- **All CLI-specific knowledge lives in the YAML descriptor.** Zero `if provider == "claude"` branches in engine code. Litmus: `grep -rE "claude|codex" api/internal/engine/agent --include=*.go | grep -v _test.go | grep -v descriptors/` returns nothing.
- **Detection branches on FACTS, not lifecycle labels.** The reducer keys on (1) did the session id under this segment change, (2) is the new id known. The `source`/`move_signal` label is optional metadata, never control-flow.
- **Spawn env clears `CLAUDE_CODE_CHILD_SESSION` and `CLAUDECODE`** (nested-Claude-Code markers suppress child transcript persistence — Phase-0 finding).
- **Transcript path is READ from the hook payload (`$.transcript_path`), never computed.**
- **Go tests:** unit tests build with `-tags noEmbed`; integration tests add `//go:build integration` and run with `-tags 'integration noEmbed' -p 1`. Unit coverage gate is **92%** (CI) on all packages except `/cmd/`. Assertions use `github.com/stretchr/testify`. Run everything from the `api/` directory.
- **Every commit** ends its message with the trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.

---

## Design Decisions (deviations from the spec, forced by the real codebase)

These were decided after mapping the code; a reviewer should challenge them first.

- **D1 — New models, not the dormant event-sourced `Chat`.** `domain.Chat` already exists as an event-sourced (asynx) aggregate with a `read_chats` view, currently unmounted ("removed per D11"). The agentic chat is a different concept (segments + ledger). We introduce **plain-gorm** `domain.AgentChat` + `domain.AgentSegment` (tables `agent_chats`, `agent_segments`), following the `domain.Project` CRUD pattern — NOT the event-sourced path. No entanglement with the existing chat aggregate.
- **D2 — Ledger lives under Crowbar-managed per-workspace storage, not inside the git worktree.** The spec's `<workspace>/.crowbar/chats/` has no codebase equivalent and would pollute the user's checkout. We add `worktreepath.AgentLedgerDir(home, projectID, repoID, wsID, chatID)` → `.../workspaces/<wsID>/agent-ledger/<chatID>/`. Same "Crowbar owns the saving, one place per chat" intent.
- **D3 — Agentic HTTP surface is a top-level `/v0/agent/` group**, not entity-scoped and not the dormant `/v0/chats`. Chat creation carries `workspaceId` in the body. Routes: `POST/GET /v0/agent/chats`, `GET /v0/agent/chats/:id`, `POST /v0/agent/chats/:id/switch`, `GET /v0/agent/chats/:id/handoff`, `POST /v0/agent/hooks` (unscoped, segment-keyed), WS `GET /v0/agent/ws/chats`.
- **D4 — Export the socket-path derivation.** `socketPath`/`overrideSocketPath` are unexported in `core/gateway/transports/socket.go`. We add an exported `transports.SocketPath(host string) (string, error)` (wrapping the existing logic) so both the daemon and the `crowbar hook` client resolve the identical path.
- **D5 — Binary self-install.** No `os.Executable()`/`$CROWBAR_HOME/bin` machinery exists. The daemon, at startup, copies its own executable to `$CROWBAR_HOME/bin/crowbar` (0755); descriptors invoke the hook by that absolute path.
- **D6 — Agentic segments are terminal sessions.** They spawn through `engine/terminal` (so the user sees/types via the existing terminal PTY WS) but with explicit argv+env via a new low-level `session.NewCommand` + engine `CreateCommand`. Agentic-session *restore across daemon restart* is OUT OF SCOPE this iteration (a dead segment is re-spawnable from the ledger).
- **D7 — Descriptors are `go:embed`ed** from `engine/agent/descriptors/*.yaml`, with an on-disk `$CROWBAR_HOME/descriptors/` override checked first.

---

## File Structure

**New files (created by this plan):**
```
api/internal/core/gateway/transports/socketpath.go      # D4: exported SocketPath()
api/internal/core/ipc/client.go                         # unix-socket http.Client + PostJSON
api/internal/core/selfinstall/selfinstall.go            # D5: copy exe -> $CROWBAR_HOME/bin/crowbar
api/internal/engine/agent/
  descriptor.go            # Descriptor struct + Load/Validate
  descriptors_embed.go     # go:embed claude.yaml, codex.yaml + resolver (disk override)
  descriptors/claude.yaml
  descriptors/codex.yaml
  template.go              # token resolution ({tmp},{uuid},{id},{handoff},{cwd_slug})
  inject.go                # generic step-runner -> Spawn{Argv,Env,Cwd,Cleanup}
  hooks.go                 # field-map: raw provider payload -> canonical event
  engine.go                # AgentEngine facade: LoadDescriptor, BuildSpawn, MapHook
  registry.go              # segment->session-id registry (serialized reducer state)
  reducer.go               # the detection reducer (facts, not labels)
api/internal/domain/agent_chat.go                       # D1: AgentChat gorm model
api/internal/domain/agent_segment.go                    # D1: AgentSegment gorm model
api/internal/app/repositories/agentchat/agentchat.go    # bespoke gorm repo (Chat+Segment)
api/internal/app/ledger/ledger.go                       # append-only file ledger writer
api/internal/app/usecases/agent/agent.go                # spawn/switch/handoff usecases + reducer host
api/internal/api/v0/endpoints/agent/routes.go
api/internal/api/v0/endpoints/agent/handlers/handlers.go
api/internal/api/v0/endpoints/agent/handlers/chats.go
api/internal/api/v0/endpoints/agent/handlers/hooks.go
api/internal/api/v0/endpoints/agent/handlers/switch.go
api/internal/api/v0/dto/agent.go
api/cmd/crowbar/hook.go                                  # `crowbar hook <event>` subcommand
api/cmd/crowbar/handoff.go                               # `crowbar handoff dump <chatId>` subcommand
```

**Modified files:**
```
api/internal/core/gateway/transports/socket.go          # NewSocket calls exported SocketPath
api/internal/engine/terminal/internal/session/session.go # + NewCommand + spawnCmd argv path
api/internal/engine/terminal/terminal.go                 # + CreateCommand(argv,env,cwd)
api/internal/app/usecases/internal/worktreepath/worktreepath.go  # + AgentLedgerDir
api/internal/app/gorm.go                                 # register AgentChat/AgentSegment stores (or repo)
api/internal/app/container.go                            # wire ledger + agent usecase
api/internal/app/usecases/container.go                   # hold Agent usecase
api/internal/api/v0/router.go                            # mount /v0/agent group
api/internal/api/v0/container.go                          # agent-chat WS broadcaster + Push
api/internal/app/hub/{hub.go,web_socket_hub.go,subscriber.go}  # BroadcastAgentChat/PushAgentChat
api/cmd/crowbar/main.go                                   # root.AddCommand(newHookCmd(), newHandoffCmd())
api/internal/internal.go                                  # call selfinstall at startup
```

---

## Phase A — Foundations (socket client, self-install)

### Task 1: Export the socket-path derivation (D4)

**Files:**
- Create: `api/internal/core/gateway/transports/socketpath.go`
- Modify: `api/internal/core/gateway/transports/socket.go` (make `NewSocket` call the exported fn)
- Test: `api/internal/core/gateway/transports/socketpath_test.go`

**Interfaces:**
- Produces: `func SocketPath(host string) (string, error)` — resolves the daemon socket path from a `unix://[path]` host string (or `""`), honoring `CROWBAR_HOME` exactly as the daemon does.

Context: `socket.go` currently has unexported `socketPath(path string)` and `overrideSocketPath(home string)` using `fnv.New64a()` over `CROWBAR_HOME`, placing the socket at `os.TempDir()/crowbar-<fnv1a64>.sock`, else `~/.crowbar/crowbar.sock`. We expose it without changing behavior.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/core/gateway/transports/socketpath_test.go
package transports

import (
	"hash/fnv"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSocketPath_ExplicitPathPassthrough(t *testing.T) {
	got, err := SocketPath("unix:///tmp/explicit.sock")
	require.NoError(t, err)
	require.Equal(t, "/tmp/explicit.sock", got)
}

func TestSocketPath_CrowbarHomeHashed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CROWBAR_HOME", home)
	got, err := SocketPath("unix://")
	require.NoError(t, err)

	h := fnv.New64a()
	_, _ = h.Write([]byte(home))
	want := filepath.Join(os.TempDir(), "crowbar-"+hex64(h.Sum64())+".sock")
	require.Equal(t, want, got)
}

func hex64(v uint64) string { return sprintfx(v) }
```

(Add a tiny `sprintfx` helper in the test using `fmt.Sprintf("%x", v)`, or inline `fmt.Sprintf`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/core/gateway/transports/ -run TestSocketPath -v`
Expected: FAIL — `SocketPath` undefined.

- [ ] **Step 3: Write the implementation**

```go
// api/internal/core/gateway/transports/socketpath.go
package transports

import "strings"

// SocketPath resolves the daemon's unix socket path from a host string
// ("unix://" or "unix:///abs/path"). With no explicit path it derives the
// same location the daemon binds: $TMPDIR/crowbar-<fnv1a64(CROWBAR_HOME)>.sock
// when CROWBAR_HOME is set, else ~/.crowbar/crowbar.sock. Shared by the daemon
// (NewSocket) and unix-socket clients (e.g. `crowbar hook`).
func SocketPath(host string) (string, error) {
	return socketPath(strings.TrimPrefix(host, "unix://"))
}
```

- [ ] **Step 4: Point `NewSocket` at the exported wrapper**

In `api/internal/core/gateway/transports/socket.go`, replace the body line `path, err := socketPath(strings.TrimPrefix(host, "unix://"))` with `path, err := SocketPath(host)`. (Behavior identical; single source of truth.)

This drops the only `strings.TrimPrefix` call in `socket.go`, so `"strings"` becomes an unused import → compile error. Remove `"strings"` from `socket.go`'s import block as part of this step. Verify first with `grep -n 'strings\.' api/internal/core/gateway/transports/socket.go` — keep the import only if another `strings.*` use remains, otherwise delete it.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./internal/core/gateway/transports/ -v`
Expected: PASS (all, including existing socket tests).

- [ ] **Step 6: Commit**

```bash
cd api && git add internal/core/gateway/transports/
git commit -m "feat(gateway): export SocketPath for unix-socket clients

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Unix-socket HTTP client helper

**Files:**
- Create: `api/internal/core/ipc/client.go`
- Test: `api/internal/core/ipc/client_test.go`

**Interfaces:**
- Consumes: `transports.SocketPath` (Task 1).
- Produces:
  - `func NewClient(host string) (*Client, error)` — a client bound to the daemon socket.
  - `func (c *Client) PostJSON(ctx context.Context, path string, body any) (status int, respBody []byte, err error)`

Context: There is NO existing Go unix-socket client. We build a minimal one: an `http.Client` whose `Transport.DialContext` dials the resolved unix socket; the URL host is a dummy (`http://unix`).

- [ ] **Step 1: Write the failing test**

```go
// api/internal/core/ipc/client_test.go
package ipc_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
	"github.com/stretchr/testify/require"
)

func TestClient_PostJSON_OverUnixSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "t.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v0/agent/hooks", r.URL.Path)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"success":true}`))
	})}
	go srv.Serve(ln)
	defer srv.Close()

	c, err := ipc.NewClient("unix://" + sock)
	require.NoError(t, err)
	status, body, err := c.PostJSON(context.Background(), "/v0/agent/hooks", map[string]string{"event": "session_start"})
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, status)
	require.Contains(t, string(body), "success")
	_ = os.Remove(sock)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/core/ipc/ -v`
Expected: FAIL — package `ipc` does not exist.

- [ ] **Step 3: Write the implementation**

```go
// api/internal/core/ipc/client.go
package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
)

// Client is a thin HTTP client that speaks to the Crowbar daemon over its unix
// socket. The HTTP host is a placeholder; DialContext always dials the socket.
type Client struct {
	http *http.Client
}

func NewClient(host string) (*Client, error) {
	sock, err := transports.SocketPath(host)
	if err != nil {
		return nil, fmt.Errorf("ipc: socket path: %w", err)
	}
	return &Client{http: &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sock)
			},
		},
	}}, nil
}

func (c *Client) PostJSON(ctx context.Context, path string, body any) (int, []byte, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return 0, nil, fmt.Errorf("ipc: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix"+path, bytes.NewReader(buf))
	if err != nil {
		return 0, nil, fmt.Errorf("ipc: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("ipc: do: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test -tags noEmbed ./internal/core/ipc/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/core/ipc/
git commit -m "feat(ipc): unix-socket http client for CLI subcommands

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Binary self-install (D5)

**Files:**
- Create: `api/internal/core/selfinstall/selfinstall.go`
- Test: `api/internal/core/selfinstall/selfinstall_test.go`
- Modify: `api/internal/internal.go` (call at startup, best-effort)

**Interfaces:**
- Produces: `func Install(homeDir string) (installedPath string, err error)` — copies `os.Executable()` to `<homeDir>/bin/crowbar` (0755), idempotent (skips if same size+mtime). Returns the absolute installed path (used later by descriptors as the hook command).

- [ ] **Step 1: Write the failing test**

```go
// api/internal/core/selfinstall/selfinstall_test.go
package selfinstall_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/selfinstall"
	"github.com/stretchr/testify/require"
)

func TestInstall_CopiesExecutableIntoBin(t *testing.T) {
	home := t.TempDir()
	got, err := selfinstall.Install(home)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, "bin", "crowbar"), got)

	info, err := os.Stat(got)
	require.NoError(t, err)
	require.NotZero(t, info.Size())
	require.NotZero(t, info.Mode()&0o100, "installed binary must be executable")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/core/selfinstall/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the implementation**

```go
// api/internal/core/selfinstall/selfinstall.go
package selfinstall

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Install copies the running executable to <homeDir>/bin/crowbar (0755) so the
// vendor CLIs' hooks can invoke `crowbar hook ...` by absolute path. Idempotent:
// re-copies only when the destination is missing or a different size. Best-effort
// at the call site (a failure must not stop the daemon).
func Install(homeDir string) (string, error) {
	src, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("selfinstall: executable: %w", err)
	}
	binDir := filepath.Join(homeDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("selfinstall: mkdir: %w", err)
	}
	dst := filepath.Join(binDir, "crowbar")

	si, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("selfinstall: stat src: %w", err)
	}
	if di, derr := os.Stat(dst); derr == nil && di.Size() == si.Size() {
		return dst, nil // already installed, same size
	}
	if err := copyFile(src, dst); err != nil {
		return "", err
	}
	if err := os.Chmod(dst, 0o755); err != nil {
		return "", fmt.Errorf("selfinstall: chmod: %w", err)
	}
	return dst, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("selfinstall: open src: %w", err)
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("selfinstall: create: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("selfinstall: copy: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("selfinstall: close: %w", err)
	}
	return os.Rename(tmp, dst) // atomic swap
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test -tags noEmbed ./internal/core/selfinstall/ -v`
Expected: PASS.

- [ ] **Step 5: Wire into daemon startup (best-effort)**

In `api/internal/internal.go`, inside `New(...)` after the container chain is built (near where `homeDir`/`cfg.homeDir` is known), add:

```go
// Install the crowbar binary into $CROWBAR_HOME/bin so vendor CLI hooks can
// invoke `crowbar hook` by absolute path. Best-effort: never block startup.
if p, err := selfinstall.Install(metadata.GetHomePath()); err == nil {
	_ = p // path is re-derived by the agent engine when rendering descriptors
}
```

Add the import `"github.com/char2cs/crowbar/api/internal/core/selfinstall"` and ensure `metadata` is imported. (If `internal.New` does not currently know the home dir, use `metadata.GetHomePath()`.)

- [ ] **Step 6: Run the build + tests**

Run: `cd api && go build -tags noEmbed ./cmd/crowbar && go test -tags noEmbed ./internal/core/selfinstall/ -v`
Expected: build OK, tests PASS.

- [ ] **Step 7: Commit**

```bash
cd api && git add internal/core/selfinstall/ internal/internal.go
git commit -m "feat(selfinstall): copy crowbar into \$CROWBAR_HOME/bin at startup

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase B — Component 1: descriptor, injection, PTY spawn, hooks

### Task 4: Descriptor types + loader + embedded descriptors (D6, D7)

**Files:**
- Create: `api/internal/engine/agent/descriptor.go`, `api/internal/engine/agent/descriptors_embed.go`, `api/internal/engine/agent/descriptors/claude.yaml`, `api/internal/engine/agent/descriptors/codex.yaml`
- Test: `api/internal/engine/agent/descriptor_test.go`

**Interfaces:**
- Produces:
  - `type Descriptor struct { ... }` (full shape below), `type InjectStep struct { Verb string; Args map[string]any }`, `type HookMap struct { ProviderEvent string; Fields map[string]string }`.
  - `func LoadDescriptor(data []byte) (*Descriptor, error)` — unmarshal + `Validate`.
  - `func (d *Descriptor) Validate() error`.
  - `func ResolveDescriptor(homeDir, providerID string) (*Descriptor, error)` — disk override `<home>/descriptors/<id>.yaml` else embedded.

Note: uses `gopkg.in/yaml.v3`. If absent from `go.mod`, run `cd api && go get gopkg.in/yaml.v3` first (it is a common transitive dep of gin/testify; verify with `go list -m gopkg.in/yaml.v3`).

- [ ] **Step 1: Write the failing test**

```go
// api/internal/engine/agent/descriptor_test.go
package agent_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestResolveDescriptor_EmbeddedClaudeValid(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	require.Equal(t, "claude", d.ID)
	require.Equal(t, "claude", d.Spawn.Cmd)
	require.True(t, d.Spawn.InteractiveRequired)
	require.Contains(t, d.Spawn.ForbidFlags, "-p")
	require.Equal(t, "SessionStart", d.Hooks["session_start"].ProviderEvent)
	require.Equal(t, "$.session_id", d.Hooks["session_start"].Fields["session_id"])
}

func TestResolveDescriptor_EmbeddedCodexValid(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	require.Equal(t, "codex", d.ID)
	require.Contains(t, d.Spawn.ForbidFlags, "exec")
	require.Contains(t, d.Spawn.Args, "--dangerously-bypass-hook-trust")
}

func TestLoadDescriptor_RejectsMissingID(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte("spawn:\n  cmd: x\n"))
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/engine/agent/ -run TestResolveDescriptor -v`
Expected: FAIL — package `agent` does not exist.

- [ ] **Step 3: Write the descriptor types + loader**

```go
// api/internal/engine/agent/descriptor.go
package agent

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type Descriptor struct {
	ID      string `yaml:"id"`
	Version struct {
		Pinned      string `yaml:"pinned"`
		CompatCheck string `yaml:"compat_check"`
	} `yaml:"version"`
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
		Assign *ArgSpec `yaml:"assign"`
		Resume *ArgSpec `yaml:"resume"`
	} `yaml:"session"`
	ConfigInjection []InjectStep       `yaml:"config_injection"`
	Hooks           map[string]HookMap `yaml:"hooks"`
	Transcript      struct {
		FromHook string `yaml:"from_hook"`
		Locate   string `yaml:"locate"`
		Content  string `yaml:"content"`
	} `yaml:"transcript"`
	HandoffInject []InjectStep `yaml:"handoff_inject"`
}

type ArgSpec struct {
	Arg string `yaml:"arg"`
}

type HookMap struct {
	ProviderEvent string            `yaml:"provider_event"`
	Fields        map[string]string `yaml:"fields"`
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
	if _, ok := d.Hooks["session_start"]; !ok {
		return fmt.Errorf("agent: descriptor %q missing hooks.session_start", d.ID)
	}
	if d.Hooks["session_start"].Fields["session_id"] == "" {
		return fmt.Errorf("agent: descriptor %q session_start must map session_id", d.ID)
	}
	return nil
}
```

- [ ] **Step 4: Write the embed resolver + the two descriptor YAMLs**

```go
// api/internal/engine/agent/descriptors_embed.go
package agent

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed descriptors/*.yaml
var embedded embed.FS

// ResolveDescriptor loads provider descriptor by id: a disk override at
// <homeDir>/descriptors/<id>.yaml wins, else the embedded default.
func ResolveDescriptor(homeDir, providerID string) (*Descriptor, error) {
	override := filepath.Join(homeDir, "descriptors", providerID+".yaml")
	if data, err := os.ReadFile(override); err == nil {
		return LoadDescriptor(data)
	}
	data, err := embedded.ReadFile("descriptors/" + providerID + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("agent: unknown provider %q: %w", providerID, err)
	}
	return LoadDescriptor(data)
}
```

Create `api/internal/engine/agent/descriptors/claude.yaml` — copy the corrected descriptor from the spec §4.1 verbatim (the version with `spawn.env.clear`, `session.assign` present, `config_injection` render_hooks+pass_arg, `hooks.session_start/turn_stop`, `transcript.from_hook`, `handoff_inject` append-system-prompt). Set the hook command to the placeholder `{crowbar_hook}` which the engine resolves at render time (see Task 6). Key lines:

```yaml
id: claude
version: { pinned: "2.1.201", compat_check: "claude --version" }
spawn:
  cmd: claude
  interactive_required: true
  forbid_flags: ["-p", "--print"]
  env:
    clear: ["CLAUDE_CODE_CHILD_SESSION", "CLAUDECODE"]
session:
  assign: { arg: "--session-id {uuid}" }
  resume: { arg: "--resume {id}" }
config_injection:
  - render_hooks: { format: claude_settings_json, into: "{tmp}/settings.json" }
  - pass_arg:     { arg: "--settings", value: "{tmp}/settings.json" }
hooks:
  session_start:
    provider_event: SessionStart
    fields: { session_id: "$.session_id", transcript: "$.transcript_path", move_signal: "$.source" }
  turn_stop:
    provider_event: Stop
    fields: { session_id: "$.session_id", transcript: "$.transcript_path" }
transcript:
  from_hook: "$.transcript_path"
  locate: "~/.claude/projects/{cwd_slug}/{session_id}.jsonl"
  content: opaque
handoff_inject:
  - pass_arg: { arg: "--append-system-prompt", value: "{handoff}" }
```

Create `api/internal/engine/agent/descriptors/codex.yaml` — copy the corrected descriptor from spec §4.2 verbatim:

```yaml
id: codex
version: { pinned: "0.139.0", compat_check: "codex --version" }
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
  - write_file: { path: "{tmp}/codex-home/config.toml", content: "[projects.\"{cwd}\"]\ntrust_level = \"trusted\"\n" }
  - render_hooks: { format: codex_hooks_json, into: "{tmp}/codex-home/hooks.json" }
hooks:
  session_start:
    provider_event: SessionStart
    fields: { session_id: "$.session_id", transcript: "$.transcript_path", move_signal: "$.source" }
  turn_stop:
    provider_event: Stop
    fields: { session_id: "$.session_id", transcript: "$.transcript_path" }
transcript:
  from_hook: "$.transcript_path"
  locate: "~/.codex/sessions/{yyyy}/{mm}/{dd}/rollout-*-{session_id}.jsonl"
  content: opaque
handoff_inject:
  - pass_arg: { positional: "{handoff}" }
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./internal/engine/agent/ -run 'TestResolveDescriptor|TestLoadDescriptor' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd api && git add internal/engine/agent/
git commit -m "feat(agent): provider descriptor types, loader, embedded claude/codex

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Template token resolution

**Files:**
- Create: `api/internal/engine/agent/template.go`
- Test: `api/internal/engine/agent/template_test.go`

**Interfaces:**
- Produces:
  - `type TemplateCtx struct { Tmp, UUID, ID, Handoff, Cwd, CwdSlug, CrowbarHook, SessionID string }`
  - `func Expand(s string, ctx TemplateCtx) string` — replaces `{tmp} {uuid} {id} {handoff} {cwd} {cwd_slug} {crowbar_hook} {session_id}` tokens.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/engine/agent/template_test.go
package agent_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestExpand_ReplacesKnownTokens(t *testing.T) {
	ctx := agent.TemplateCtx{Tmp: "/t", UUID: "u1", ID: "s9", Cwd: "/w", CrowbarHook: "/bin/crowbar"}
	require.Equal(t, "--session-id u1", agent.Expand("--session-id {uuid}", ctx))
	require.Equal(t, "/t/settings.json", agent.Expand("{tmp}/settings.json", ctx))
	require.Equal(t, "resume s9", agent.Expand("resume {id}", ctx))
	require.Equal(t, "/bin/crowbar hook session_start", agent.Expand("{crowbar_hook} hook session_start", ctx))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/engine/agent/ -run TestExpand -v`
Expected: FAIL — `Expand` undefined.

- [ ] **Step 3: Write the implementation**

```go
// api/internal/engine/agent/template.go
package agent

import "strings"

type TemplateCtx struct {
	Tmp         string
	UUID        string
	ID          string
	Handoff     string
	Cwd         string
	CwdSlug     string
	CrowbarHook string
	SessionID   string
}

func Expand(s string, ctx TemplateCtx) string {
	r := strings.NewReplacer(
		"{tmp}", ctx.Tmp,
		"{uuid}", ctx.UUID,
		"{id}", ctx.ID,
		"{handoff}", ctx.Handoff,
		"{cwd}", ctx.Cwd,
		"{cwd_slug}", ctx.CwdSlug,
		"{crowbar_hook}", ctx.CrowbarHook,
		"{session_id}", ctx.SessionID,
	)
	return r.Replace(s)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test -tags noEmbed ./internal/engine/agent/ -run TestExpand -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/engine/agent/template.go internal/engine/agent/template_test.go
git commit -m "feat(agent): descriptor template token expansion

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Injection step-runner → spawn plan

**Files:**
- Create: `api/internal/engine/agent/inject.go`
- Test: `api/internal/engine/agent/inject_test.go`

**Interfaces:**
- Consumes: `Descriptor`, `InjectStep`, `TemplateCtx`, `Expand`.
- Produces:
  - `type SpawnPlan struct { Argv []string; Env []string; Cwd string; TmpDir string; Cleanup func() }`
  - `func BuildSpawnPlan(d *Descriptor, ctx TemplateCtx, baseEnv []string, extraSteps []InjectStep) (*SpawnPlan, error)` — runs `spawn.args` + `config_injection` (+ optional `extraSteps` for handoff/session), rendering hook config files and assembling argv/env. The `render_hooks` verb writes a settings/hooks JSON whose command is `{crowbar_hook} hook <canonical>` for each descriptor hook, stamped so `CROWBAR_SEGMENT_ID` is inherited from env.

Behavior of each verb (all args pre-`Expand`ed against `ctx`):
- `set_env{name,value}` → append `name=value` to Env (also export into the child so nested renders see it).
- `write_file{path, content|from}` → mkdir-all parent, write `content` (or copy the file at `from`, expanding `~`).
- `render_hooks{format, into}` → write the provider hooks JSON to `into` (see `renderHooks`).
- `pass_arg{arg, value}` → append `arg` then (if present) `value` to Argv; `pass_arg{positional}` → append the positional value only.
- `copy_tree{from,into,except}` → recursive copy (used by an alternate Codex strategy; implement a simple version).
- `seed_trust{file}` → NO-OP placeholder in this iteration (Codex uses `--dangerously-bypass-hook-trust`); log-and-skip.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/engine/agent/inject_test.go
package agent_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestBuildSpawnPlan_ClaudeWritesSettingsAndArgs(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	ctx := agent.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/bin/crowbar"}

	plan, err := agent.BuildSpawnPlan(d, ctx, os.Environ(), nil)
	require.NoError(t, err)
	defer plan.Cleanup()

	// --settings <file> present and the file exists with a hook command
	require.Contains(t, plan.Argv, "--settings")
	idx := indexOf(plan.Argv, "--settings")
	settingsPath := plan.Argv[idx+1]
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "SessionStart")
	require.Contains(t, string(data), "/bin/crowbar")

	// nested-CC markers are cleared from Env
	for _, kv := range plan.Env {
		require.False(t, strings.HasPrefix(kv, "CLAUDE_CODE_CHILD_SESSION="))
		require.False(t, strings.HasPrefix(kv, "CLAUDECODE="))
	}
}

func TestBuildSpawnPlan_CodexSetsHomeAndBypassFlag(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	ctx := agent.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/bin/crowbar"}
	plan, err := agent.BuildSpawnPlan(d, ctx, os.Environ(), nil)
	require.NoError(t, err)
	defer plan.Cleanup()

	require.Contains(t, plan.Argv, "--dangerously-bypass-hook-trust")
	require.Contains(t, envValue(plan.Env, "CODEX_HOME"), filepath.Base(ctx.Tmp)) // CODEX_HOME under tmp
	_, err = os.Stat(envValue(plan.Env, "CODEX_HOME") + "/hooks.json")
	require.NoError(t, err)
}

func TestBuildSpawnPlan_RejectsForbiddenFlag(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	ctx := agent.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/bin/crowbar"}
	// A handoff/positional step that smuggles a headless flag must be rejected.
	_, err = agent.BuildSpawnPlan(d, ctx, os.Environ(), []agent.InjectStep{
		{Verb: "pass_arg", Args: map[string]any{"positional": "-p"}},
	})
	require.Error(t, err)
}
```

(Provide tiny `indexOf`, `envValue` helpers in the test file.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/engine/agent/ -run TestBuildSpawnPlan -v`
Expected: FAIL — `BuildSpawnPlan` undefined.

- [ ] **Step 3: Write the implementation**

```go
// api/internal/engine/agent/inject.go
package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type SpawnPlan struct {
	Argv    []string
	Env     []string
	Cwd     string
	TmpDir  string
	Cleanup func()
}

// BuildSpawnPlan renders a descriptor's spawn.args + config_injection (+ any
// extraSteps such as session/handoff args) into a concrete argv/env/cwd, writing
// any hook-config files under ctx.Tmp. baseEnv is the process env to start from.
func BuildSpawnPlan(d *Descriptor, ctx TemplateCtx, baseEnv []string, extraSteps []InjectStep) (*SpawnPlan, error) {
	env := clearEnv(baseEnv, d.Spawn.Env.Clear)
	plan := &SpawnPlan{
		Cwd:     ctx.Cwd,
		TmpDir:  ctx.Tmp,
		Env:     env,
		Cleanup: func() { _ = os.RemoveAll(ctx.Tmp) },
	}
	// static spawn.args first (e.g. codex --dangerously-bypass-hook-trust)
	for _, a := range d.Spawn.Args {
		plan.Argv = append(plan.Argv, Expand(a, ctx))
	}
	steps := append([]InjectStep{}, d.ConfigInjection...)
	steps = append(steps, extraSteps...)
	for _, st := range steps {
		if err := runStep(d, st, ctx, plan); err != nil {
			plan.Cleanup()
			return nil, err
		}
	}
	// Hard guard (Global Constraints): the engine must never spawn a headless CLI.
	// Reject if any assembled argv token exactly equals a descriptor forbid_flag.
	for _, tok := range plan.Argv {
		for _, forbidden := range d.Spawn.ForbidFlags {
			if tok == forbidden {
				plan.Cleanup()
				return nil, fmt.Errorf("agent: forbidden flag %q for provider %q", tok, d.ID)
			}
		}
	}
	return plan, nil
}

func runStep(d *Descriptor, st InjectStep, ctx TemplateCtx, plan *SpawnPlan) error {
	arg := func(k string) string { return Expand(asString(st.Args[k]), ctx) }
	switch st.Verb {
	case "set_env":
		kv := arg("name") + "=" + arg("value")
		plan.Env = append(plan.Env, kv)
	case "write_file":
		return writeFileStep(arg("path"), arg("content"), arg("from"))
	case "render_hooks":
		return renderHooks(d, arg("format"), arg("into"), ctx)
	case "pass_arg":
		if pos, ok := st.Args["positional"]; ok {
			plan.Argv = append(plan.Argv, Expand(asString(pos), ctx))
			return nil
		}
		plan.Argv = append(plan.Argv, arg("arg"))
		if _, ok := st.Args["value"]; ok {
			plan.Argv = append(plan.Argv, arg("value"))
		}
	case "copy_tree":
		return copyTree(arg("from"), arg("into"))
	case "seed_trust":
		return nil // no-op this iteration; codex uses --dangerously-bypass-hook-trust
	default:
		return fmt.Errorf("agent: unknown inject verb %q", st.Verb)
	}
	return nil
}

// renderHooks writes the provider hook config that maps each descriptor hook to
// `<crowbar_hook> hook <canonical-event>`. Both Claude settings.json and Codex
// hooks.json share the same nested shape {hooks:{Event:[{hooks:[{type,command}]}]}}.
func renderHooks(d *Descriptor, format, into string, ctx TemplateCtx) error {
	type cmd struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}
	type group struct {
		Hooks []cmd `json:"hooks"`
	}
	events := map[string][]group{}
	for canonical, hm := range d.Hooks {
		command := ctx.CrowbarHook + " hook " + canonical
		events[hm.ProviderEvent] = []group{{Hooks: []cmd{{Type: "command", Command: command}}}}
	}
	payload := map[string]any{"hooks": events}
	buf, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("agent: render hooks: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(into), 0o700); err != nil {
		return fmt.Errorf("agent: render hooks mkdir: %w", err)
	}
	return os.WriteFile(into, buf, 0o600)
}

func writeFileStep(path, content, from string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("agent: write_file mkdir: %w", err)
	}
	if from != "" {
		return copyFile(expandHome(from), path)
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

func clearEnv(env, clear []string) []string {
	if len(clear) == 0 {
		return append([]string{}, env...)
	}
	drop := map[string]struct{}{}
	for _, k := range clear {
		drop[k] = struct{}{}
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if _, skip := drop[name]; skip {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("agent: copy open %s: %w", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("agent: copy create %s: %w", dst, err)
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func copyTree(from, into string) error {
	return filepath.Walk(from, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(into, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0o700)
		}
		return copyFile(p, dst)
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./internal/engine/agent/ -run TestBuildSpawnPlan -v`
Expected: PASS. (The Codex test's `write_file from ~/.codex/auth.json` may not exist on the test machine — guard it: in the test, skip the auth assertion if `~/.codex/auth.json` is absent, OR make `write_file{from}` tolerate a missing source by writing an empty file with a logged warning. Choose the tolerant `write_file` so the plan build never fails on a missing optional source.)

Refinement to `writeFileStep`: when `from != ""` and the source is missing, create an empty destination instead of erroring:
```go
if from != "" {
	src := expandHome(from)
	if _, err := os.Stat(src); err != nil {
		return os.WriteFile(path, nil, 0o600) // tolerate missing optional source
	}
	return copyFile(src, path)
}
```

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/engine/agent/inject.go internal/engine/agent/inject_test.go
git commit -m "feat(agent): generic injection step-runner -> spawn plan

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: PTY spawn extension — `session.NewCommand`

**Files:**
- Modify: `api/internal/engine/terminal/internal/session/session.go`
- Test: `api/internal/engine/terminal/internal/session/session_command_test.go`

**Interfaces:**
- Produces: `func NewCommand(id string, argv []string, cwd string, env []string, cols, rows, scrollbackLines int) (*Session, error)` — spawns `argv[0]` with `argv[1:]` under a PTY, storing the joined command as `shell` for display. Requires `len(argv) >= 1`.

Context: the existing `spawn` (session.go:237-272) does `exec.Command(s.shell)`. Add a sibling that builds the command from an argv slice. Reuse everything else (sizing, model, pump).

- [ ] **Step 1: Write the failing test**

```go
// api/internal/engine/terminal/internal/session/session_command_test.go
package session

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewCommand_RunsArgv(t *testing.T) {
	dir := t.TempDir()
	// `sh -c 'printf MARKER'` — a real argv with args, not a bare shell.
	s, err := NewCommand("cmd-id", []string{"/bin/sh", "-c", "printf CMDMARKER; sleep 0.2"}, dir, os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	require.Equal(t, "cmd-id", s.ID())

	ch, err := s.Attach()
	require.NoError(t, err)
	deadline := time.After(3 * time.Second)
	var seen bool
	for !seen {
		select {
		case f := <-ch:
			if containsBytes(f.Data, "CMDMARKER") {
				seen = true
			}
		case <-deadline:
			t.Fatal("did not observe command output")
		}
	}
	require.True(t, seen)
	s.Kill()
}

func TestNewCommand_EmptyArgvErrors(t *testing.T) {
	_, err := NewCommand("x", nil, t.TempDir(), os.Environ(), 80, 24, 0)
	require.Error(t, err)
}

func containsBytes(b []byte, sub string) bool { return len(b) > 0 && (string(b) == sub || indexOfBytes(b, sub) >= 0) }
```

(Provide `indexOfBytes` via `strings.Contains(string(b), sub)`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/engine/terminal/internal/session/ -run TestNewCommand -v`
Expected: FAIL — `NewCommand` undefined.

- [ ] **Step 3: Write the implementation**

Add to `session.go` a `NewCommand` constructor and a `spawnCmd` that mirrors `spawn` but takes argv. Reuse `resolveBirth`, `newModel`, `startResponseSink`, `pump`.

```go
// NewCommand spawns an explicit argv (not a login shell) under a PTY. Used by the
// agentic engine to launch vendor CLIs (claude/codex) with descriptor-built args
// and env. The joined argv is stored as the display "shell".
func NewCommand(
	id string,
	argv []string,
	cwd string,
	env []string,
	cols int,
	rows int,
	scrollbackLines int,
) (*Session, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("session: NewCommand requires non-empty argv")
	}
	s := newBareSession(id, strings.Join(argv, " "), cwd, "")
	if err := s.spawnCmd(argv, env, spawnParams{Cols: cols, Rows: rows, ScrollbackLines: scrollbackLines}); err != nil {
		return nil, err
	}
	return s, nil
}

// spawnCmd is spawn() with an explicit argv instead of a bare shell.
func (s *Session) spawnCmd(argv []string, env []string, p spawnParams) error {
	cols, rows, sbLines, redraw := s.resolveBirth(p)

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = s.cwd
	cmd.Env = env

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("session: pty start: %w", err)
	}
	_ = pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})

	m, ser := newModel(cols, rows, sbLines)
	if len(redraw) > 0 {
		m.Write(redraw)
	}
	s.ptmx = ptmx
	s.cmd = cmd
	s.model = m
	s.serializer = ser
	s.emitter = model.NewDiffEmitter()
	if s.model != nil {
		s.startResponseSink(s.ptmx)
	}
	go s.pump()
	return nil
}
```

Add `"strings"` to the session.go imports if not present.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./internal/engine/terminal/internal/session/ -run TestNewCommand -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/engine/terminal/internal/session/
git commit -m "feat(terminal): NewCommand spawns explicit argv+env under a PTY

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Terminal engine `CreateCommand`

**Files:**
- Modify: `api/internal/engine/terminal/terminal.go`
- Test: `api/internal/engine/terminal/terminal_command_test.go`

**Interfaces:**
- Produces (on the `Engine` interface and `terminalEngine`): `CreateCommand(ctx context.Context, workspaceID, cwd string, argv []string, env []string) (string, error)` — spawns a command session via `session.NewCommand`, registers it (so it streams over the existing terminal WS), returns the session id.

Context: mirror `Create` (terminal.go:372) but skip profile resolution and use an explicit argv+env. Add `CreateCommand` to the `Engine` interface (find the interface declaration in terminal.go; add the method signature there too).

- [ ] **Step 1: Write the failing test**

```go
// api/internal/engine/terminal/terminal_command_test.go
package terminal

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateCommand_RegistersSession(t *testing.T) {
	e := New()
	defer e.Shutdown()
	id, err := e.CreateCommand(context.Background(), "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "sleep 1"}, os.Environ())
	require.NoError(t, err)
	require.NotEmpty(t, id)
	require.True(t, e.SessionExists(context.Background(), id))
	require.Contains(t, e.ListSessionsForWorkspace("ws1"), id)
}

func TestWithTerminalDefaults_InjectsTERMWhenAbsent(t *testing.T) {
	// A command session started with an env lacking TERM ends up with the default.
	got := withTerminalDefaults([]string{"PATH=/usr/bin"})
	require.Contains(t, got, "TERM=xterm-256color")
	require.Contains(t, got, "COLORTERM=truecolor")
	// Does not override a caller-provided TERM.
	got = withTerminalDefaults([]string{"TERM=screen"})
	require.Contains(t, got, "TERM=screen")
	require.NotContains(t, got, "TERM=xterm-256color")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/engine/terminal/ -run TestCreateCommand -v`
Expected: FAIL — `CreateCommand` undefined.

- [ ] **Step 3: Write the implementation**

```go
// CreateCommand spawns an explicit argv+env as a registered session (streamable
// over the terminal WS). Used by the agentic engine for vendor-CLI segments.
func (e *terminalEngine) CreateCommand(
	_ context.Context,
	workspaceID string,
	cwd string,
	argv []string,
	env []string,
) (string, error) {
	id := uuid.NewString()
	// The real terminal engine seeds ptyEnv() (TERM/COLORTERM); CreateCommand takes
	// the caller's env verbatim, so under launchd TERM is absent and Ink TUIs
	// misrender. Backfill the terminal defaults for any keys not already set.
	env = withTerminalDefaults(env)
	s, err := session.NewCommand(id, argv, cwd, env, 80, 24, 0)
	if err != nil {
		return "", fmt.Errorf("terminal: create command: %w", err)
	}
	e.reg.Add(id, workspaceID, s)
	go e.reapOnDone(id, workspaceID, s)
	return id, nil
}

// withTerminalDefaults appends TERM=xterm-256color / COLORTERM=truecolor only for
// keys the caller did not already provide, matching the real terminal engine's
// ptyEnv() seeding.
func withTerminalDefaults(env []string) []string {
	has := func(key string) bool {
		for _, kv := range env {
			if strings.HasPrefix(kv, key+"=") {
				return true
			}
		}
		return false
	}
	if !has("TERM") {
		env = append(env, "TERM=xterm-256color")
	}
	if !has("COLORTERM") {
		env = append(env, "COLORTERM=truecolor")
	}
	return env
}
```

Add `CreateCommand(ctx context.Context, workspaceID, cwd string, argv, env []string) (string, error)` to the `Engine` interface in terminal.go. Add `"strings"` to terminal.go's imports if not already present (used by `withTerminalDefaults`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./internal/engine/terminal/ -run TestCreateCommand -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/engine/terminal/
git commit -m "feat(terminal): CreateCommand registers an argv session on the engine

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: `crowbar hook <event>` subcommand

**Files:**
- Create: `api/cmd/crowbar/hook.go`
- Modify: `api/cmd/crowbar/main.go` (register `newHookCmd()`)
- Test: `api/cmd/crowbar/hook_test.go`

**Interfaces:**
- Consumes: `ipc.NewClient` (Task 2).
- Produces: `func newHookCmd() *cobra.Command` — reads stdin (the provider JSON payload), reads `$CROWBAR_SEGMENT_ID`, POSTs `{segment_id, event, payload}` to `/v0/agent/hooks`. Exits 0 always (a hook must never break the vendor CLI). Factor the core into `func runHook(event string, stdin io.Reader, host string) error` for testing.

- [ ] **Step 1: Write the failing test**

```go
// api/cmd/crowbar/hook_test.go
package main

import (
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunHook_ForwardsSegmentAndPayload(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "h.sock")
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

	t.Setenv("CROWBAR_SEGMENT_ID", "seg-42")
	err = runHook("session_start", strings.NewReader(`{"session_id":"abc","source":"startup"}`), "unix://"+sock)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "seg-42", got["segment_id"])
	require.Equal(t, "session_start", got["event"])
	payload := got["payload"].(map[string]any)
	require.Equal(t, "abc", payload["session_id"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./cmd/crowbar/ -run TestRunHook -v`
Expected: FAIL — `runHook` undefined.

- [ ] **Step 3: Write the implementation**

```go
// api/cmd/crowbar/hook.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

func newHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "hook <event>",
		Short:  "Forward a vendor-CLI hook payload to the Crowbar daemon",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(_ *cobra.Command, args []string) error {
			// A hook must never break the vendor CLI: swallow errors, exit 0.
			_ = runHook(args[0], os.Stdin, "unix://")
			return nil
		},
	}
}

func runHook(event string, stdin io.Reader, host string) error {
	raw, err := io.ReadAll(io.LimitReader(stdin, 8<<20))
	if err != nil {
		return fmt.Errorf("hook: read stdin: %w", err)
	}
	var payload any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			payload = map[string]any{"_raw": string(raw)} // tolerate non-JSON (argv-delivered variants)
		}
	}
	client, err := ipc.NewClient(host)
	if err != nil {
		return err
	}
	body := map[string]any{
		"segment_id": os.Getenv("CROWBAR_SEGMENT_ID"),
		"event":      event,
		"payload":    payload,
	}
	_, _, err = client.PostJSON(context.Background(), "/v0/agent/hooks", body)
	return err
}
```

Register in `main.go`: change `root.AddCommand(newServeCmd(), newVersionCmd())` to `root.AddCommand(newServeCmd(), newVersionCmd(), newHookCmd())`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./cmd/crowbar/ -run TestRunHook -v && go build -tags noEmbed ./cmd/crowbar`
Expected: PASS + build OK.

- [ ] **Step 5: Commit**

```bash
cd api && git add cmd/crowbar/
git commit -m "feat(cli): crowbar hook subcommand forwards payloads over the socket

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Canonical-event field-mapping (hooks.go)

**Files:**
- Create: `api/internal/engine/agent/hooks.go`
- Test: `api/internal/engine/agent/hooks_test.go`

**Interfaces:**
- Produces:
  - `type CanonicalEvent struct { Kind string; SessionID string; Transcript string; MoveSignal string; Raw map[string]any }` (`Kind` ∈ `"session_start"`, `"turn_stop"`).
  - `func (d *Descriptor) MapHook(canonical string, payload map[string]any) (CanonicalEvent, error)` — applies the descriptor `hooks[canonical].Fields` JSONPath-ish (`$.field`) map to the raw payload.

Only shallow `$.field` extraction is needed (no nested paths in the descriptors). Implement a tiny extractor.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/engine/agent/hooks_test.go
package agent_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestMapHook_ClaudeSessionStart(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	ev, err := d.MapHook("session_start", map[string]any{
		"session_id":      "abc-123",
		"transcript_path": "/x/abc-123.jsonl",
		"source":          "clear",
	})
	require.NoError(t, err)
	require.Equal(t, "session_start", ev.Kind)
	require.Equal(t, "abc-123", ev.SessionID)
	require.Equal(t, "/x/abc-123.jsonl", ev.Transcript)
	require.Equal(t, "clear", ev.MoveSignal)
}

func TestMapHook_UnknownCanonicalErrors(t *testing.T) {
	d, _ := agent.ResolveDescriptor(t.TempDir(), "claude")
	_, err := d.MapHook("nope", map[string]any{})
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/engine/agent/ -run TestMapHook -v`
Expected: FAIL — `MapHook` undefined.

- [ ] **Step 3: Write the implementation**

```go
// api/internal/engine/agent/hooks.go
package agent

import (
	"fmt"
	"strings"
)

type CanonicalEvent struct {
	Kind       string
	SessionID  string
	Transcript string
	MoveSignal string
	Raw        map[string]any
}

func (d *Descriptor) MapHook(canonical string, payload map[string]any) (CanonicalEvent, error) {
	hm, ok := d.Hooks[canonical]
	if !ok {
		return CanonicalEvent{}, fmt.Errorf("agent: descriptor %q has no hook %q", d.ID, canonical)
	}
	get := func(field string) string {
		path, ok := hm.Fields[field]
		if !ok {
			return ""
		}
		return extract(payload, path)
	}
	return CanonicalEvent{
		Kind:       canonical,
		SessionID:  get("session_id"),
		Transcript: get("transcript"),
		MoveSignal: get("move_signal"),
		Raw:        payload,
	}, nil
}

// extract reads a shallow `$.field` path from the payload (the only shape the
// descriptors use). Returns "" for a missing/non-string value.
func extract(payload map[string]any, path string) string {
	key := strings.TrimPrefix(path, "$.")
	if v, ok := payload[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./internal/engine/agent/ -run TestMapHook -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/engine/agent/hooks.go internal/engine/agent/hooks_test.go
git commit -m "feat(agent): map raw provider hook payloads to canonical events

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase C — Component 2: chat/segment model, ledger, detection

### Task 11: AgentChat + AgentSegment gorm models + repository (D1)

**Files:**
- Create: `api/internal/domain/agent_chat.go`, `api/internal/domain/agent_segment.go`, `api/internal/app/repositories/agentchat/agentchat.go`
- Test: `api/internal/app/repositories/agentchat/agentchat_test.go`

**Interfaces:**
- Produces:
  - `domain.AgentChat{ ID, WorkspaceID, Title, ActiveSegmentID string; CreatedAt time.Time }` (table `agent_chats`).
  - `domain.AgentSegment{ ID, ChatID, ProviderID, ProviderSessionID, CrowbarSegmentID string; StartedAt time.Time; EndedAt *time.Time; Status string }` (table `agent_segments`, `ChatID gorm:"index"`).
  - `agentchat.Store` interface + `agentchat.New(db *gorm.DB) (Store, error)` with: `SaveChat/GetChat/ListChats/SaveSegment/GetSegment/GetActiveSegmentByCrowbarID/ListSegmentsByChat/AllSegments`.

**Invariant (relied on by the reducer/persistence in Task 14):** at most one `AgentSegment` with `status="active"` exists per `CrowbarSegmentID` (one live process = one active segment; a chat move ends the old active segment and opens a new one), and a given process's `ProviderID` is stable for its whole life. "The current segment for a process" is therefore always resolved via `GetActiveSegmentByCrowbarID`, never a bare `.First` on `crowbar_segment_id` (multiple rows can share it once moves have happened).

Mirror the bespoke gorm repo template at `app/repositories/workspace/internal/locations/locations.go:60-121` (self-`AutoMigrate` in `New`, `db.WithContext(ctx).Save`, `.First(&row,"id = ?",id)`, `.Find`, translate `gorm.ErrRecordNotFound` → package `ErrNotFound`). Models follow `domain/project.go` conventions (string PK, `TableName()`, json tags).

- [ ] **Step 1: Write the failing test**

```go
// api/internal/app/repositories/agentchat/agentchat_test.go
package agentchat_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestAgentChat_SaveAndListSegments(t *testing.T) {
	db, err := storesqlite.OpenDB(filepath.Join(t.TempDir(), "v.db"))
	require.NoError(t, err)
	repo, err := agentchat.New(db)
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, repo.SaveChat(ctx, domain.AgentChat{ID: "c1", WorkspaceID: "w1", CreatedAt: time.Now()}))
	require.NoError(t, repo.SaveSegment(ctx, domain.AgentSegment{ID: "s1", ChatID: "c1", ProviderID: "claude", ProviderSessionID: "sid-1", Status: "active"}))
	require.NoError(t, repo.SaveSegment(ctx, domain.AgentSegment{ID: "s2", ChatID: "c1", ProviderID: "codex", ProviderSessionID: "sid-2", Status: "active"}))

	got, err := repo.ListSegmentsByChat(ctx, "c1")
	require.NoError(t, err)
	require.Len(t, got, 2)

	chat, err := repo.GetChat(ctx, "c1")
	require.NoError(t, err)
	require.Equal(t, "w1", chat.WorkspaceID)

	_, err = repo.GetChat(ctx, "missing")
	require.ErrorIs(t, err, agentchat.ErrNotFound)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/app/repositories/agentchat/ -v`
Expected: FAIL — package/models do not exist.

- [ ] **Step 3: Write the models**

```go
// api/internal/domain/agent_chat.go
package domain

import "time"

// AgentChat is a Crowbar-owned agentic conversation tracked across provider
// segments (00 agentic-engine spec §6). Distinct from the event-sourced Chat.
type AgentChat struct {
	ID              string    `gorm:"primaryKey" json:"id"`
	WorkspaceID     string    `gorm:"index"      json:"workspaceId"`
	Title           string    `json:"title"`
	ActiveSegmentID string    `json:"activeSegmentId"`
	CreatedAt       time.Time `json:"createdAt"`
}

func (AgentChat) TableName() string { return "agent_chats" }
```

```go
// api/internal/domain/agent_segment.go
package domain

import "time"

// AgentSegment is one provider stint within an AgentChat.
type AgentSegment struct {
	ID                string     `gorm:"primaryKey" json:"id"`
	ChatID            string     `gorm:"index"      json:"chatId"`
	ProviderID        string     `json:"providerId"`
	ProviderSessionID string     `gorm:"index"      json:"providerSessionId"`
	CrowbarSegmentID  string     `gorm:"index"      json:"crowbarSegmentId"`
	TerminalSessionID string     `json:"terminalSessionId"`
	TranscriptPath    string     `json:"transcriptPath"`
	StartedAt         time.Time  `json:"startedAt"`
	EndedAt           *time.Time `json:"endedAt,omitempty"`
	Status            string     `json:"status"`
}

func (AgentSegment) TableName() string { return "agent_segments" }
```

- [ ] **Step 4: Write the repository** (mirror `locations.go`)

```go
// api/internal/app/repositories/agentchat/agentchat.go
package agentchat

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	gormdb "gorm.io/gorm"
)

var ErrNotFound = errors.New("agentchat: not found")

type Store interface {
	SaveChat(ctx context.Context, c domain.AgentChat) error
	GetChat(ctx context.Context, id string) (domain.AgentChat, error)
	ListChats(ctx context.Context) ([]domain.AgentChat, error)
	SaveSegment(ctx context.Context, s domain.AgentSegment) error
	GetSegment(ctx context.Context, id string) (domain.AgentSegment, error)
	GetActiveSegmentByCrowbarID(ctx context.Context, crowbarSegID string) (domain.AgentSegment, error)
	ListSegmentsByChat(ctx context.Context, chatID string) ([]domain.AgentSegment, error)
	AllSegments(ctx context.Context) ([]domain.AgentSegment, error)
}

type gormStore struct{ db *gormdb.DB }

func New(db *gormdb.DB) (Store, error) {
	if err := db.AutoMigrate(&domain.AgentChat{}, &domain.AgentSegment{}); err != nil {
		return nil, fmt.Errorf("agentchat: migrate: %w", err)
	}
	return &gormStore{db: db}, nil
}

func (s *gormStore) SaveChat(ctx context.Context, c domain.AgentChat) error {
	return s.db.WithContext(ctx).Save(&c).Error
}

func (s *gormStore) GetChat(ctx context.Context, id string) (domain.AgentChat, error) {
	var c domain.AgentChat
	if err := s.db.WithContext(ctx).First(&c, "id = ?", id).Error; err != nil {
		if errors.Is(err, gormdb.ErrRecordNotFound) {
			return domain.AgentChat{}, ErrNotFound
		}
		return domain.AgentChat{}, err
	}
	return c, nil
}

func (s *gormStore) ListChats(ctx context.Context) ([]domain.AgentChat, error) {
	var out []domain.AgentChat
	return out, s.db.WithContext(ctx).Find(&out).Error
}

func (s *gormStore) SaveSegment(ctx context.Context, seg domain.AgentSegment) error {
	return s.db.WithContext(ctx).Save(&seg).Error
}

func (s *gormStore) GetSegment(ctx context.Context, id string) (domain.AgentSegment, error) {
	var seg domain.AgentSegment
	if err := s.db.WithContext(ctx).First(&seg, "id = ?", id).Error; err != nil {
		if errors.Is(err, gormdb.ErrRecordNotFound) {
			return domain.AgentSegment{}, ErrNotFound
		}
		return domain.AgentSegment{}, err
	}
	return seg, nil
}

// GetActiveSegmentByCrowbarID returns the single active segment for a live process
// (its CrowbarSegmentID). Multiple rows may share a crowbar_segment_id after chat
// moves, but the invariant guarantees at most one is active — so callers resolving
// "the current segment for a process" MUST use this, never a bare .First on the id.
func (s *gormStore) GetActiveSegmentByCrowbarID(ctx context.Context, crowbarSegID string) (domain.AgentSegment, error) {
	var seg domain.AgentSegment
	err := s.db.WithContext(ctx).
		Where("crowbar_segment_id = ? AND status = ?", crowbarSegID, "active").
		Order("started_at desc").
		First(&seg).Error
	if err != nil {
		if errors.Is(err, gormdb.ErrRecordNotFound) {
			return domain.AgentSegment{}, ErrNotFound
		}
		return domain.AgentSegment{}, err
	}
	return seg, nil
}

func (s *gormStore) ListSegmentsByChat(ctx context.Context, chatID string) ([]domain.AgentSegment, error) {
	var out []domain.AgentSegment
	return out, s.db.WithContext(ctx).Where("chat_id = ?", chatID).Order("started_at asc").Find(&out).Error
}

func (s *gormStore) AllSegments(ctx context.Context) ([]domain.AgentSegment, error) {
	var out []domain.AgentSegment
	return out, s.db.WithContext(ctx).Find(&out).Error
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./internal/app/repositories/agentchat/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd api && git add internal/domain/agent_chat.go internal/domain/agent_segment.go internal/app/repositories/agentchat/
git commit -m "feat(agent): AgentChat/AgentSegment models + gorm repository

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 12: Ledger writer + per-workspace path (D2)

**Files:**
- Modify: `api/internal/app/usecases/internal/worktreepath/worktreepath.go` (add `AgentLedgerDir`)
- Create: `api/internal/app/ledger/ledger.go`
- Test: `api/internal/app/ledger/ledger_test.go`, plus a case in `worktreepath_test.go`

**Interfaces:**
- Produces:
  - `func worktreepath.AgentLedgerDir(crowbarHome, projectID, repoID, wsID, chatID string) string` → `.../workspaces/<wsID>/agent-ledger/<chatID>`.
  - `type Ledger struct{ dir string }`, `func Open(dir string) (*Ledger, error)`, `func (l *Ledger) Append(providerID string, at time.Time, blob []byte) (string, error)` (writes `NNNN-<rfc3339>-<provider>.blob`, returns the filename), `func (l *Ledger) ReadAll() ([]byte, error)` (concatenates entries in order with provider/time separators).

Mirror `worktreepath.StorageDir` (worktreepath.go:32-42) for the new path helper.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/app/ledger/ledger_test.go
package ledger_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/ledger"
	"github.com/stretchr/testify/require"
)

func TestLedger_AppendThenReadAllOrdered(t *testing.T) {
	l, err := ledger.Open(filepath.Join(t.TempDir(), "c1"))
	require.NoError(t, err)
	at := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)

	f1, err := l.Append("claude", at, []byte("FIRST"))
	require.NoError(t, err)
	require.Contains(t, f1, "claude")
	_, err = l.Append("codex", at.Add(time.Minute), []byte("SECOND"))
	require.NoError(t, err)

	all, err := l.ReadAll()
	require.NoError(t, err)
	require.Less(t, indexOf(string(all), "FIRST"), indexOf(string(all), "SECOND"))
	require.Contains(t, string(all), "claude")
	require.Contains(t, string(all), "codex")
}
```

(Provide `indexOf` via `strings.Index`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/app/ledger/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the path helper**

Add to `worktreepath.go` (mirroring `StorageDir`):

```go
// AgentLedgerDir is the per-chat agentic ledger directory under the workspace's
// Crowbar-managed storage (NOT inside the git worktree).
func AgentLedgerDir(crowbarHome, projectID, repoID, wsID, chatID string) string {
	return filepath.Join(workspaceDir(crowbarHome, projectID, repoID, wsID), "agent-ledger", chatID)
}
```

(Use whatever the existing internal helper is named — `StorageDir` builds `.../workspaces/<wsID>/storages`; reuse the same base. If there is no `workspaceDir` helper, inline `filepath.Join(crowbarHome, "projects", projectID, repoID, "workspaces", wsID, "agent-ledger", chatID)` to match `StorageDir`'s construction exactly.)

- [ ] **Step 4: Write the ledger**

```go
// api/internal/app/ledger/ledger.go
package ledger

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Ledger is a per-chat, append-only, provider-tagged store of opaque transcript
// snapshots. Crowbar owns it; it is never parsed (agentic-engine spec §6).
type Ledger struct{ dir string }

func Open(dir string) (*Ledger, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("ledger: mkdir: %w", err)
	}
	return &Ledger{dir: dir}, nil
}

// Append writes the next opaque snapshot, prefixed with a zero-padded sequence
// so lexical order == chronological order. Returns the written filename.
func (l *Ledger) Append(providerID string, at time.Time, blob []byte) (string, error) {
	seq, err := l.nextSeq()
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%04d-%s-%s.blob", seq, at.UTC().Format("20060102T150405Z"), providerID)
	if err := os.WriteFile(filepath.Join(l.dir, name), blob, 0o640); err != nil {
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
		if !de.IsDir() && filepath.Ext(de.Name()) == ".blob" {
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

// ReadAll concatenates every entry in order, separated by a legible header so a
// receiving model can tell segments apart.
func (l *Ledger) ReadAll() ([]byte, error) {
	names, err := l.entries()
	if err != nil {
		return nil, err
	}
	var out []byte
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(l.dir, n))
		if err != nil {
			return nil, fmt.Errorf("ledger: read %s: %w", n, err)
		}
		out = append(out, []byte("\n===== LEDGER ENTRY "+n+" =====\n")...)
		out = append(out, data...)
	}
	return out, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./internal/app/ledger/ ./internal/app/usecases/internal/worktreepath/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd api && git add internal/app/ledger/ internal/app/usecases/internal/worktreepath/
git commit -m "feat(agent): append-only per-chat ledger + workspace ledger path

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 13: Detection reducer + segment registry (the crown jewel)

**Files:**
- Create: `api/internal/engine/agent/registry.go`
- Test: `api/internal/engine/agent/registry_test.go`

**Interfaces:**
- Produces:
  - `type Outcome struct { Kind string; ChatID, SessionID, SegmentID string }` (`Kind` ∈ `noop|bound|focus|registered`).
  - `type Registry struct { ... }`, `func NewRegistry() *Registry`.
  - `func (r *Registry) BindSegment(segmentID, chatID string)` — record which chat a freshly-spawned segment belongs to (called before its first hook).
  - `func (r *Registry) Seed(sessionID, chatID string)` — mark a session id as known (rehydration from DB at startup).
  - `func (r *Registry) OnSessionStart(segmentID, sessionID string, newChatID func() string) Outcome` — the serialized reducer.

This is the spec §7 reducer. Everything runs under one mutex (single-writer → the registry never corrupts, an acceptance criterion). Branches ONLY on facts: did the id change, is the new id known.

- [ ] **Step 1: Write the failing test (the full reducer table)**

```go
// api/internal/engine/agent/registry_test.go
package agent_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestReducer_SpawnBindsFirstSession(t *testing.T) {
	r := agent.NewRegistry()
	r.BindSegment("seg1", "chatA")
	out := r.OnSessionStart("seg1", "sid-0", newID("chatX"))
	require.Equal(t, "bound", out.Kind)
	require.Equal(t, "chatA", out.ChatID)
	require.Equal(t, "sid-0", out.SessionID)
}

func TestReducer_SameSessionIsNoop(t *testing.T) {
	r := agent.NewRegistry()
	r.BindSegment("seg1", "chatA")
	r.OnSessionStart("seg1", "sid-0", newID("x"))
	out := r.OnSessionStart("seg1", "sid-0", newID("x"))
	require.Equal(t, "noop", out.Kind)
}

func TestReducer_UnknownNewIdRegistersNewChat_Case2(t *testing.T) {
	r := agent.NewRegistry()
	r.BindSegment("seg1", "chatA")
	r.OnSessionStart("seg1", "sid-0", newID("x")) // bound to chatA
	out := r.OnSessionStart("seg1", "sid-1", newID("chatB")) // /clear -> new unknown id
	require.Equal(t, "registered", out.Kind)
	require.Equal(t, "chatB", out.ChatID)
}

func TestReducer_KnownIdMovesFocus_Case1(t *testing.T) {
	r := agent.NewRegistry()
	r.Seed("sid-known", "chatK") // some other chat's session, already known
	r.BindSegment("seg1", "chatA")
	r.OnSessionStart("seg1", "sid-0", newID("x")) // bound to chatA
	out := r.OnSessionStart("seg1", "sid-known", newID("nope")) // /resume of a known chat
	require.Equal(t, "focus", out.Kind)
	require.Equal(t, "chatK", out.ChatID)
}

func TestReducer_ClearThenResumeSequence(t *testing.T) {
	r := agent.NewRegistry()
	r.BindSegment("seg1", "chatA")
	require.Equal(t, "bound", r.OnSessionStart("seg1", "s0", newID("x")).Kind)
	reg := r.OnSessionStart("seg1", "s1", newID("chatB")) // clear
	require.Equal(t, "registered", reg.Kind)
	// resuming s0 (now known as chatA) => focus chatA
	back := r.OnSessionStart("seg1", "s0", newID("nope"))
	require.Equal(t, "focus", back.Kind)
	require.Equal(t, "chatA", back.ChatID)
}

func newID(id string) func() string { return func() string { return id } }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/engine/agent/ -run TestReducer -v`
Expected: FAIL — `NewRegistry` undefined.

- [ ] **Step 3: Write the reducer**

```go
// api/internal/engine/agent/registry.go
package agent

import "sync"

type Outcome struct {
	Kind      string // noop | bound | focus | registered
	ChatID    string
	SessionID string
	SegmentID string
}

// Registry is the single serialized owner of context-move detection state. All
// mutation goes through one mutex so the registry can never corrupt.
type Registry struct {
	mu            sync.Mutex
	segToSession  map[string]string // segment id -> last session id seen
	segToChat     map[string]string // segment id -> chat it currently hosts
	sessionToChat map[string]string // known session id -> chat id
}

func NewRegistry() *Registry {
	return &Registry{
		segToSession:  map[string]string{},
		segToChat:     map[string]string{},
		sessionToChat: map[string]string{},
	}
}

// BindSegment records the chat a freshly-spawned segment belongs to, before its
// first SessionStart hook arrives.
func (r *Registry) BindSegment(segmentID, chatID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.segToChat[segmentID] = chatID
}

// Seed marks a session id as known (used to rehydrate from the DB at startup).
func (r *Registry) Seed(sessionID, chatID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionToChat[sessionID] = chatID
}

// OnSessionStart is the spec §7 reducer. It branches ONLY on facts: (1) did the
// session id under this segment change, (2) is the new id known.
func (r *Registry) OnSessionStart(segmentID, sessionID string, newChatID func() string) Outcome {
	r.mu.Lock()
	defer r.mu.Unlock()

	prev := r.segToSession[segmentID]
	switch {
	case sessionID == prev:
		return Outcome{Kind: "noop", SegmentID: segmentID, SessionID: sessionID}

	case prev == "":
		// First id for this segment: bind it to the segment's chat (spawn / switch-continuation).
		chatID := r.segToChat[segmentID]
		r.sessionToChat[sessionID] = chatID
		r.segToSession[segmentID] = sessionID
		return Outcome{Kind: "bound", ChatID: chatID, SessionID: sessionID, SegmentID: segmentID}

	default:
		if chatID, known := r.sessionToChat[sessionID]; known {
			// CASE 1: moved into a chat we know -> focus it.
			r.segToChat[segmentID] = chatID
			r.segToSession[segmentID] = sessionID
			return Outcome{Kind: "focus", ChatID: chatID, SessionID: sessionID, SegmentID: segmentID}
		}
		// CASE 2: an unknown id appeared -> register a new chat.
		chatID := newChatID()
		r.sessionToChat[sessionID] = chatID
		r.segToChat[segmentID] = chatID
		r.segToSession[segmentID] = sessionID
		return Outcome{Kind: "registered", ChatID: chatID, SessionID: sessionID, SegmentID: segmentID}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./internal/engine/agent/ -run TestReducer -v`
Expected: PASS (all five).

- [ ] **Step 5: Add a concurrency test (single-writer guarantee)**

```go
func TestReducer_ConcurrentNoRace(t *testing.T) {
	r := agent.NewRegistry()
	r.BindSegment("seg", "chatA")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r.OnSessionStart("seg", "s0", newID("x")) }()
	}
	wg.Wait()
}
```
Run: `cd api && go test -tags noEmbed -race ./internal/engine/agent/ -run TestReducer_ConcurrentNoRace -v`
Expected: PASS, no race.

- [ ] **Step 6: Commit**

```bash
cd api && git add internal/engine/agent/registry.go internal/engine/agent/registry_test.go
git commit -m "feat(agent): serialized context-move detection reducer

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 14: Agent usecase — spawn segment, wire hooks→reducer→ledger, WS events

**Files:**
- Create: `api/internal/app/usecases/agent/agent.go`
- Modify: `api/internal/app/hub/{hub.go,web_socket_hub.go,subscriber.go}` (add `BroadcastAgentChat`/`PushAgentChat`), `api/internal/app/container.go` + `api/internal/app/usecases/container.go` (wire it)
- Test: `api/internal/app/usecases/agent/agent_test.go`

**Interfaces:**
- Consumes: `agent.Registry`, `agent.ResolveDescriptor`/`BuildSpawnPlan`/`MapHook`, `agentchat.Store`, `ledger`, a `TerminalCommander` seam (`CreateCommand`), a `WorkspaceReader` (to resolve the workspace's worktree dir), a `Broadcaster` seam, `selfinstall`/`metadata` for the crowbar-hook path.
- Produces:
  - `type Usecase struct { ... }`, `func New(...) *Usecase`.
  - `func (u *Usecase) SpawnChat(ctx, workspaceID, providerID string) (chatID, segmentID string, err error)` — creates AgentChat + AgentSegment, binds the reducer, builds the spawn plan (with `CROWBAR_SEGMENT_ID`), spawns via `CreateCommand`.
  - `func (u *Usecase) IngestHook(ctx, segmentID, canonicalEvent string, payload map[string]any) error` — resolves the segment's provider descriptor, maps the hook, runs the reducer on `session_start` (persist outcome + emit WS), and on `turn_stop` reads the transcript and appends to the ledger.
  - `func (u *Usecase) ListChats(ctx) ([]domain.AgentChat, error)`, `GetChat`, `SegmentsFor`.

Define the seam interfaces locally (like the endpoint handlers do) so the usecase is unit-testable with fakes:
```go
type TerminalCommander interface {
	CreateCommand(ctx context.Context, workspaceID, cwd string, argv, env []string) (string, error)
	Kill(ctx context.Context, sessionID string) error
}
type Broadcaster interface{ BroadcastAgentChat(chatID, kind string) }
type WorkspaceReader interface{ WorktreeDir(ctx context.Context, workspaceID string) (crowbarHome, projectID, repoID, worktree string, err error) }
```

**Shared helper `spawnSegment` — the single owner of `AgentChat.ActiveSegmentID`.** Both `SpawnChat` and `SwitchProvider` (Task 17) go through it, so `ActiveSegmentID` is always set by exactly one code path (never left `""`, which would break `SwitchProvider`'s `GetSegment(chat.ActiveSegmentID)` lookup):

```go
func (u *Usecase) spawnSegment(ctx context.Context, chat domain.AgentChat, providerID string, extraSteps []agent.InjectStep, handoff string) (segID string, err error)
```

The `handoff` param is the assembled handoff blob (empty for a fresh spawn); it binds the descriptor's `{handoff}` token via `TemplateCtx.Handoff` so `handoff_inject` steps in `extraSteps` resolve.

Contract (every step is normative):
1. `segID := uuid.NewString()` (also the `CrowbarSegmentID`).
2. Persist `AgentSegment{ID: segID, ChatID: chat.ID, ProviderID: providerID, CrowbarSegmentID: segID, Status: "active", StartedAt: now}`.
3. `registry.BindSegment(segID, chat.ID)`.
4. Resolve descriptor(providerID); build `TemplateCtx{Tmp: mkdtemp, Cwd: worktree(chat.WorkspaceID), CrowbarHook, Handoff: handoff}`; `plan, err := BuildSpawnPlan(d, ctx, os.Environ(), extraSteps)`; `argv = append([]string{d.Spawn.Cmd}, plan.Argv...)`; `env = append(plan.Env, "CROWBAR_SEGMENT_ID="+segID)`.
5. `termSessID := term.CreateCommand(ctx, chat.WorkspaceID, worktree, argv, env)`; set `seg.TerminalSessionID = termSessID`; save the segment.
6. **`chat.ActiveSegmentID = segID`; `SaveChat(chat)`.**
7. return `segID`.

**Design of `SpawnChat`:**
1. `chatID := uuid.NewString()`; create + persist `AgentChat{ID: chatID, WorkspaceID, CreatedAt: now}` (no `ActiveSegmentID` yet — the helper sets it).
2. `segID, err := u.spawnSegment(ctx, chat, providerID, nil, "")` — this creates + spawns the first segment and, per its contract, sets the chat's `ActiveSegmentID`. (Empty handoff: a fresh chat has no prior context.)
3. Return `chatID, segID`.

**Invariant (see Task 11):** one live process (`CrowbarSegmentID`) has **at most one** `AgentSegment` with `status="active"`, and its `ProviderID` is stable for life. A single process can host different chats over time (`/clear` registers a new chat; `/resume`-to-known focuses another), so the DB must *follow* the reducer — a move **ends the current active segment and opens a new active segment** rather than mutating a static `ChatID`. Therefore every "current segment for this process" lookup resolves via `GetActiveSegmentByCrowbarID(crowbarSegID)`, never a bare `.First` on `crowbar_segment_id` (which now matches multiple rows).

**Design of `IngestHook`** (`segmentID` here is the process's `CrowbarSegmentID`, i.e. `crowbarSegID`):
- Resolve the **active** segment for `crowbarSegID` via `agentchat.GetActiveSegmentByCrowbarID(ctx, crowbarSegID)` → its `ProviderID` → `ResolveDescriptor` → `ev, _ := d.MapHook(canonicalEvent, payload)`.
- If `ev.Kind == "session_start"`: `out := u.registry.OnSessionStart(crowbarSegID, ev.SessionID, func() string { return uuid.NewString() })`, then persist per outcome:
  - `bound`: set the active segment's `ProviderSessionID = ev.SessionID` and `TranscriptPath = ev.Transcript`; save. **Never overwrite a non-empty `ProviderSessionID`** (guard before assigning).
  - `registered`: mark the current active segment `Status="moved"`, `EndedAt=now`; save — **KEEP its `ProviderSessionID`** (switch-back resume to the old id must still work). Create a new `AgentChat{ID: out.ChatID, WorkspaceID: <the prior chat's WorkspaceID>, CreatedAt: now}`, and a **new** `AgentSegment{ID: uuid.NewString(), ChatID: out.ChatID, ProviderID: <same>, CrowbarSegmentID: crowbarSegID, ProviderSessionID: ev.SessionID, TranscriptPath: ev.Transcript, Status: "active", StartedAt: now}`; set the new chat's `ActiveSegmentID` to that segment; save.
  - `focus`: mark the current active segment `Status="moved"`, save; create a **new** `AgentSegment{ID: newSegID (=uuid.NewString()), ChatID: out.ChatID (the known chat), ProviderID: <same>, CrowbarSegmentID: crowbarSegID, ProviderSessionID: ev.SessionID, Status: "active", StartedAt: now}`; save. This path does **not** call `spawnSegment` — no new process is started; the *same* live process now hosts an already-known chat. So after saving the new active row, explicitly load the focused chat (`out.ChatID`), set its **`ActiveSegmentID = newSegID`**, and `SaveChat` (keeping that chat's `ActiveSegmentID` correct for its next switch). Broadcast focus. (`ProviderSessionID` may repeat across rows — that is expected; the active-status invariant keeps the "current segment" unambiguous.)
  - `noop`: nothing.
  - Always `u.bc.BroadcastAgentChat(out.ChatID, out.Kind)`.
- If `ev.Kind == "turn_stop"`: resolve `seg, err := agentchat.GetActiveSegmentByCrowbarID(ctx, crowbarSegID)` — the DB active row is the **single source of truth** for which chat the process currently hosts, so read the chat id directly as `seg.ChatID` (this is why `Registry.ChatForSegment` is unnecessary). Read the transcript inline as an opaque slurp: `blob, rerr := os.ReadFile(ev.Transcript)`. **If `rerr != nil`** (path missing / not yet written on disk), **SKIP the ledger append** — log at debug and return `nil`; do **not** error the hook. On a successful read: `AgentLedgerDir(...)` → `ledger.Open(...).Append(seg.ProviderID, now, blob)`; `BroadcastAgentChat(seg.ChatID, "turn_stopped")`.

- [ ] **Step 1: Write the failing test** (fakes for terminal/ws/workspace)

```go
// api/internal/app/usecases/agent/agent_test.go — table/behavior tests:
// (a) SpawnChat persists a chat+segment, binds the reducer, and calls CreateCommand
//     with CROWBAR_SEGMENT_ID in env and the descriptor cmd as argv[0]; it also
//     leaves the chat's ActiveSegmentID NON-EMPTY and EQUAL to the created segment
//     id, so a later SwitchProvider resolves the outgoing active segment via
//     GetSegment(chat.ActiveSegmentID).
// (b) IngestHook(session_start) with a NEW id under a bound segment records the
//     provider session id (and TranscriptPath) on the active segment.
// (c) IngestHook(turn_stop) with a REAL transcript file on disk appends a ledger
//     entry; with a MISSING/unwritten transcript path it no-ops (no ledger entry,
//     no error returned).
// (d) IngestHook(session_start) yielding "registered" (a second, unknown id under
//     the same crowbarSegID) marks the old segment Status="moved" (keeping its
//     ProviderSessionID intact — NOT overwritten) and creates a NEW active segment
//     + AgentChat carrying the prior chat's WorkspaceID; the old chat's active
//     segment is no longer active.
// (e) IngestHook(turn_stop) AFTER a move attributes the ledger entry to the NEW
//     chat (GetActiveSegmentByCrowbarID resolves the current, not the moved, row).
// Use a fakeCommander capturing argv/env, a fakeWorkspace returning a temp worktree,
// and a fakeBroadcaster recording (chatID,kind).
```
(Write the three tests concretely using testify, constructing `agent.New(...)` with the fakes + a real `agentchat.New(OpenDB(tmp))` + real `agent.NewRegistry()`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/agent/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the usecase** (implement `New`, `SpawnChat`, `IngestHook`, `ListChats`, `GetChat`, `SegmentsFor` per the design above). Segment lookups use `agentchat.GetActiveSegmentByCrowbarID` (added in Task 11) — do **not** add a plain `GetSegmentByCrowbarID`, since a bare `.First` on `crowbar_segment_id` mis-attributes once a process has hosted multiple chats.

- [ ] **Step 4: Add the hub plumbing** — mirror an existing topic exactly:
  - `hub/subscriber.go`: add `PushAgentChat(chatID, kind string)` to the `Subscriber` interface.
  - `hub/web_socket_hub.go`: add `BroadcastAgentChat(chatID, kind string)` to the interface.
  - `hub/hub.go`: add `func (h *Hub) BroadcastAgentChat(chatID, kind string) { ... fan to subscribers ... }` (mirror `BroadcastGit`, hub.go:90-99).

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/agent/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd api && git add internal/app/usecases/agent/ internal/app/hub/
git commit -m "feat(agent): usecase wiring hooks->reducer->ledger + spawn

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 15: `/v0/agent` endpoints + WS + container wiring (D3)

**Files:**
- Create: `api/internal/api/v0/endpoints/agent/{routes.go,handlers/handlers.go,handlers/chats.go,handlers/hooks.go}`, `api/internal/api/v0/dto/agent.go`
- Modify: `api/internal/api/v0/router.go` (mount `/v0/agent`), `api/internal/api/v0/container.go` (agent-chat broadcaster + `PushAgentChat`), `api/internal/app/container.go` + `usecases/container.go` (construct + expose the agent usecase)
- Test: `api/internal/api/v0/endpoints/agent/handlers/hooks_test.go` (unit) + integration test in Phase E

**Interfaces:**
- Routes (all on the top-level `/v0` group `rg` in `router.go`, mirroring how `health`/`system` register there):
  - `POST /v0/agent/chats` (body `{workspaceId, provider}`) → `SpawnChat` → `WriteMutationOK(chatID)`.
  - `GET  /v0/agent/chats` → list.
  - `GET  /v0/agent/chats/:id` → detail (chat + segments).
  - `POST /v0/agent/hooks` (body `{segment_id, event, payload}`) → `IngestHook` → `WriteAccepted`.
  - `GET  /v0/agent/ws/chats` → `c.agentChats.Handle` (the broadcaster).

Mirror the dormant `chats` endpoint package (`endpoints/chats/`) for structure, and the workspaces `Register` signature. For the WS broadcaster, add a `agentChats *ws.Broadcaster[dto.AgentChatDTO]` field to the v0 `Container` (container.go:33-44), construct it with an `agentChatDef(appContainer)` `StreamDef` that is **UNSCOPED** — `GET /v0/agent/ws/chats` is a global route (no `:wsId` path param), so do **NOT** mirror `gitDef` (which is wsId-scoped via a `wsId` Filter + `withWatcherLifecycle`, container.go:70/366-378). Mirror the flat topic defs instead (the shape of `filesDef`/`lspDef`, container.go:380-405) but **omit the `:wsId` Filter and any per-workspace lifecycle wrapper** — the closest fully-unscoped exemplars are `projectsDef`/`reposDef` (container.go:270-292): registered bare in `NewBroadcaster` with no Filters and no lifecycle wrapper. Namespace by `chatID`. Implement `PushAgentChat` mirroring `PushGit` (container.go:248-258) but pushing to all subscribers (no wsId scoping), and register the v0 container as the hub subscriber already does (`appContainer.Hub.Register(c)`).

- [ ] **Step 1: Write the failing unit test (hooks handler decodes + dispatches)**

```go
// api/internal/api/v0/endpoints/agent/handlers/hooks_test.go
// Build a gin test context with a JSON body {segment_id,event,payload}, call
// Hooks handler with a fake usecase, assert it called IngestHook with the decoded
// values and wrote 202. Use httptest + gin.CreateTestContext.
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/api/v0/endpoints/agent/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the DTO + handlers + routes** (mirror `endpoints/chats/` and `endpoints/workspaces/`). `handlers.go` declares the `AgentUsecase` seam interface (`SpawnChat`, `IngestHook`, `ListChats`, `GetChat`, `SegmentsFor`) and the `Handlers` struct + `New`. `chats.go` implements `Create`/`List`/`Get`. `hooks.go` implements `Hooks` (decode `{segment_id,event,payload}` → `IngestHook` → `WriteAccepted`). `routes.go` `Register(rg, usecase, wsHandle)`.

- [ ] **Step 4: Mount + wire**
  - `router.go`: after the `system.Register(rg)` line (router.go:50-51), add `agent.Register(rg, c.app.Usecases.Agent, c.agentChats.Handle)`.
  - `container.go` (v0): add the `agentChats` broadcaster field + construct (UNSCOPED `agentChatDef` — no `:wsId` Filter, no lifecycle wrapper) + `PushAgentChat` + `agentChatDef`.
  - `app/container.go` + `usecases/container.go`: construct `agent.New(...)` with the terminal engine, `agentchat` repo (built off `adapters.GlobalView()` in `repositories.New` or a new `GORMStores` entry), ledger factory, registry, the hub as broadcaster, and a workspace reader. Expose as `Usecases.Agent`.
    - **Concrete `WorkspaceReader.WorktreeDir` resolver** (the seam declared in Task 14): given `workspaceID`, resolve `projectID`/`repoID` from the workspace-location index — `c.app.Repositories.Workspace.Get(ctx, workspaceID)` returns a `domain.Workspace` carrying `ProjectID`/`RepoID` (it looks up the `locations` store at `repositories/workspace/internal/locations`, the authority for "where does this workspace live"). Then `crowbarHome := metadata.GetHomePath()` (`core/metadata`), and `worktree := worktreepath.For(crowbarHome, projectID, repoID, workspaceID)` (`app/usecases/internal/worktreepath`, the same helper `worktree.go` uses). Return `(crowbarHome, projectID, repoID, worktree, nil)`.
  - Seed the registry from persisted segments at startup (rehydration): in `app/container.go` after building the agent usecase, call `ucs.Agent.SeedRegistry(ctx)` which loads `AllSegments` and `registry.Seed(seg.ProviderSessionID, seg.ChatID)` for each non-empty session id.

- [ ] **Step 5: Run tests + build**

Run: `cd api && go test -tags noEmbed ./internal/api/v0/... -v && go build -tags noEmbed ./cmd/crowbar`
Expected: PASS + build OK.

- [ ] **Step 6: Commit**

```bash
cd api && git add internal/api/v0/ internal/app/container.go internal/app/usecases/container.go
git commit -m "feat(agent): /v0/agent endpoints, WS topic, container wiring

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase D — Component 3: provider switch

> **Key reconciliation (from the Phase-0 spike):** the vendor transcript is written **incrementally while the CLI is alive** (once the nested-CC env markers are cleared — Task 6). So by switch time the outgoing content is already on disk and in the ledger (populated on every `turn_stop`). The switch therefore **reads first, then terminates** — it does not depend on a flush-on-exit. Termination is a PID signal via the terminal engine's `Kill` (SIGTERM→grace→SIGKILL); Crowbar never writes to the PTY. The spec's "gracefully quit" is satisfied by read-before-kill + a terminate grace window.

### Task 16: Handoff assembly from the ledger

**Files:**
- Modify: `api/internal/app/usecases/agent/agent.go` (add `AssembleHandoff`)
- Test: `api/internal/app/usecases/agent/handoff_test.go`

**Interfaces:**
- Produces: `func (u *Usecase) AssembleHandoff(ctx context.Context, chatID string) (string, error)` — resolves the chat's ledger dir, `ReadAll()`, and wraps it in a legible preamble (`"=== HANDED-OFF CONTEXT (Crowbar) ===\n" + ledger + "\n=== END ==="`). Returns `""` (not an error) for an empty ledger.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/app/usecases/agent/handoff_test.go
// Given a usecase whose workspace reader points at a temp home, and a chat with
// two ledger entries appended (via IngestHook turn_stop, or directly), assert
// AssembleHandoff returns a string containing both entries and the preamble.
```

- [ ] **Step 2: Run test → fails.** Run: `cd api && go test -tags noEmbed ./internal/app/usecases/agent/ -run Handoff -v`

- [ ] **Step 3: Implement `AssembleHandoff`** (resolve `AgentLedgerDir` via the workspace reader → `ledger.Open` → `ReadAll` → wrap).

- [ ] **Step 4: Run test → passes.**

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/app/usecases/agent/
git commit -m "feat(agent): assemble handoff blob from the chat ledger

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 17: Switch orchestration

**Files:**
- Modify: `api/internal/app/usecases/agent/agent.go` (add `SwitchProvider`)
- Test: `api/internal/app/usecases/agent/switch_test.go`

**Interfaces:**
- Produces: `func (u *Usecase) SwitchProvider(ctx context.Context, chatID, targetProviderID string) (newSegmentID string, err error)`.

**Design:**
1. Load chat; load its active segment (`GetSegment(chat.ActiveSegmentID)`), which carries `TerminalSessionID` + `ProviderID` + `ProviderSessionID`.
2. `handoff, _ := u.AssembleHandoff(ctx, chatID)` (ledger already holds prior turns).
3. Terminate the outgoing CLI: `u.term.Kill(ctx, oldSeg.TerminalSessionID)`; mark `oldSeg.Status="ended"`, `oldSeg.EndedAt=now`, save.
4. Determine switch-back: if a prior segment in this chat exists with `ProviderID==targetProviderID` and a non-empty `ProviderSessionID`, this is a **switch-back** — build `session.resume` extraSteps so native context is restored; still also inject the handoff delta via `handoff_inject`. Otherwise, a **forward switch** — inject only the handoff (`resumeSteps` stays empty).
   - **Resume arg must be split into separate argv tokens.** `descriptor.Session.Resume.Arg` is one string like `"--resume {id}"` / `"resume {id}"`; passing it whole as a single `pass_arg{arg}`/`pass_arg{positional}` hands `exec.Command` one literal argument (e.g. `"--resume abc123"`) and resume fails. Expand the resume arg with `{id}=priorSessionID`, then split on whitespace and append each token as its own argv element:
     ```go
     var resumeSteps []agent.InjectStep
     if switchBack {
         resumeCtx := agent.TemplateCtx{ID: priorSessionID} // only {id} matters here
         for _, tok := range strings.Fields(agent.Expand(d.Session.Resume.Arg, resumeCtx)) {
             resumeSteps = append(resumeSteps, agent.InjectStep{Verb: "pass_arg", Args: map[string]any{"positional": tok}})
         }
     }
     ```
   - **Resume steps must precede the handoff positional.** Codex resume is the subcommand `resume <id>`, which must come *before* any positional handoff arg; assemble the extra steps with the resume steps FIRST:
     ```go
     // resume goes first so codex's `resume <id>` subcommand precedes the
     // positional handoff. The {handoff} token inside handoffSteps is bound by
     // spawnSegment's TemplateCtx.Handoff (the `handoff` arg passed below).
     handoffSteps := d.HandoffInject
     extraSteps := append(resumeSteps, handoffSteps...)
     ```
     (For claude the handoff is a flag pair `--append-system-prompt {handoff}`, so order is irrelevant there; prepending resume keeps the subcommand-style codex resume correct.)
5. Build the new segment through the shared helper (Task 14), the single owner of `ActiveSegmentID`: `newSegID, err := u.spawnSegment(ctx, chat, targetProviderID, extraSteps, handoff)` — passing the assembled `handoff` so `{handoff}` in `handoffSteps` resolves. The helper persists the new `AgentSegment` (`ProviderID:target`, `CrowbarSegmentID:newSegID`, `Status:"active"`), binds the reducer, builds the spawn plan with `extraSteps`, stamps `CROWBAR_SEGMENT_ID`, calls `CreateCommand`, stores `TerminalSessionID`, **and sets `chat.ActiveSegmentID = newSegID` + `SaveChat`** — so no inline segment-creation or `ActiveSegmentID` assignment is duplicated here.
6. `BroadcastAgentChat(chatID, "switched")`. Return `newSegID`.

- [ ] **Step 1: Write the failing test** (fakes): assert (a) old terminal session killed, (b) `CreateCommand` argv for the new segment contains the target descriptor cmd and the handoff value (forward), (c) a new active segment is persisted with the target provider, (d) switch-back path adds the resume flag and the prior session id as **separate** argv tokens (e.g. `--resume` and `priorSessionID` are two distinct elements of the new segment's argv, not one `"--resume priorSessionID"` token).

- [ ] **Step 2: Run test → fails.** Run: `cd api && go test -tags noEmbed ./internal/app/usecases/agent/ -run Switch -v`

- [ ] **Step 3: Implement `SwitchProvider`** per the design, reusing the shared private `spawnSegment(ctx, chat, providerID, extraSteps, handoff)` helper defined in Task 14 (it owns `ActiveSegmentID`) — do not re-implement segment creation inline.

- [ ] **Step 4: Run test → passes.**

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/app/usecases/agent/
git commit -m "feat(agent): provider switch — read ledger, kill, respawn with handoff

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 18: Switch endpoints + `crowbar handoff dump`

**Files:**
- Create: `api/internal/api/v0/endpoints/agent/handlers/switch.go`, `api/cmd/crowbar/handoff.go`
- Modify: `api/internal/api/v0/endpoints/agent/routes.go` (add switch + handoff routes), `api/cmd/crowbar/main.go` (register `newHandoffCmd()`)
- Test: `api/cmd/crowbar/handoff_test.go`

**Interfaces:**
- `POST /v0/agent/chats/:id/switch` (body `{provider}`) → `SwitchProvider` → `WriteMutationOK(newSegID)`.
- `GET  /v0/agent/chats/:id/handoff` → `AssembleHandoff` → `WriteQueryOK({handoff string})`.
- `func newHandoffCmd() *cobra.Command` — `crowbar handoff dump <chatId>`: GETs `/v0/agent/chats/<id>/handoff` via `ipc.Client`, prints the `data.handoff` to stdout. Factor `runHandoffDump(chatID, host string, out io.Writer) error`.

- [ ] **Step 1: Write the failing test** for `runHandoffDump` (stub unix server returns an envelope with `data.handoff="X"`, assert it's printed). Add `Get(ctx, path)` to `ipc.Client` (mirror `PostJSON` with `http.MethodGet`).

- [ ] **Step 2: Run test → fails.** Run: `cd api && go test -tags noEmbed ./cmd/crowbar/ -run Handoff -v`

- [ ] **Step 3: Implement** the switch handler, the `/handoff` GET handler, the routes, `ipc.Client.Get`, and the `handoff dump` subcommand. Register `newHandoffCmd()` in `main.go`.

- [ ] **Step 4: Run tests + build → pass.** Run: `cd api && go test -tags noEmbed ./cmd/crowbar/ ./internal/api/v0/... -v && go build -tags noEmbed ./cmd/crowbar`

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/api/v0/endpoints/agent/ cmd/crowbar/ internal/core/ipc/
git commit -m "feat(agent): switch endpoint + crowbar handoff dump

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase E — End-to-end verification against the real CLIs

### Task 19: Full-stack integration test (skip-if-absent)

**Files:**
- Create: `api/tests/integration/agent/agent_test.go`

**Interfaces:** none produced; this is the acceptance test. It mirrors the Phase-0 spike (`docs/superpowers/specs/spike-2026-07-05-agentic/orchestrator.py`) but drives the flow through the **real Go stack** over `httptest` — the one surface the Python harness didn't exercise.

Follows the `kit` style (`api/tests/kit/`) — `//go:build integration`, `kit.BuildEnv`, testify suite. Guard every test with `t.Skip` if `claude`/`codex` are absent (mirror `provider/integration_test.go`'s `exec.LookPath` guard).

**Test A — `TestAgent_ClaudeSpawnAndDetect` (needs `claude`):**
1. `env := kit.BuildEnv(t)`; import a repo + create a workspace (kit helpers).
2. `POST /v0/agent/chats {workspaceId, provider:"claude"}` → 200, `chatId`.
3. Dial `/v0/agent/ws/chats`; also dial the segment's terminal PTY WS (segment → terminalSessionId in `GET /v0/agent/chats/:id`).
4. Wait for a `bound` event (the SessionStart hook fired and the reducer bound the chat). Assert the segment now has a non-empty `providerSessionId`.
5. This proves: spawn → real claude in PTY → hook → `crowbar hook` → `/v0/agent/hooks` → reducer → WS, end-to-end through Go.

> Note: the test binary must be able to run `crowbar hook`. Build the binary into `$CROWBAR_HOME/bin/crowbar` in test setup (call `selfinstall.Install(home)` pointing at a built `crowbar` — or set the descriptor's `{crowbar_hook}` to a `go run`/prebuilt path via a test-only env override `CROWBAR_HOOK_BIN`). Add that override read in the agent usecase when computing `TemplateCtx.CrowbarHook` (`if v:=os.Getenv("CROWBAR_HOOK_BIN"); v!=""`). Document this seam.

**Test B — `TestAgent_SwitchClaudeToCodex` (needs both, the headline):**
1. Spawn claude chat; drive one real turn into the PTY (SendJSON a prompt with a codeword) and wait for a `turn_stopped` WS event (ledger got a snapshot).
2. `POST /v0/agent/chats/:id/switch {provider:"codex"}` → new segment.
3. Dial the new segment's PTY; assert (via `GET /v0/agent/chats/:id/handoff`) the handoff contains the codeword. Optionally drive a codex turn asking for the codeword and assert the reply — but the deterministic assertion is: **the new active segment is codex, the ledger/handoff carries the prior turn, and a new SessionStart was detected for the codex segment.**

- [ ] **Step 1: Write Test A** (skip-if-absent) per above.
- [ ] **Step 2: Run it.** Run: `cd api && go test -tags 'integration noEmbed' -race -p 1 -run TestAgent_ClaudeSpawnAndDetect ./tests/integration/agent/... -v`
  Expected: PASS when `claude` present; SKIP otherwise.
- [ ] **Step 3: Write Test B** (the switch) per above.
- [ ] **Step 4: Run it.** Run: `cd api && go test -tags 'integration noEmbed' -race -p 1 -run TestAgent_SwitchClaudeToCodex ./tests/integration/agent/... -v`
  Expected: PASS when both present; SKIP otherwise.
- [ ] **Step 5: Full suite + coverage gate.** Run:
  ```bash
  cd api && go test -tags noEmbed -race ./... \
    && go vet -tags noEmbed ./... \
    && COVERPKG=$(go list -tags noEmbed ./... | grep -v '/cmd/' | paste -sd,) \
       go test -tags noEmbed -race -coverpkg="$COVERPKG" -coverprofile=coverage.out ./... \
    && go tool cover -func=coverage.out | awk '/^total:/{gsub("%","");if($3+0<92){print "FAIL "$3"%";exit 1}else{print "OK "$3"%"}}'
  ```
  Expected: unit tests green, vet clean, coverage ≥ 92%. Fix any package below the gate by adding unit tests before proceeding.
- [ ] **Step 6: Commit**

```bash
cd api && git add tests/integration/agent/
git commit -m "test(agent): end-to-end spawn/detect/switch against real CLIs

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Acceptance (Definition of Done)

- One engine code path spawns and receives hooks from **both** claude and codex; no provider branch in `engine/agent` (litmus grep is clean).
- The detection reducer is accurate across bound/noop/focus/registered and never corrupts under concurrency (`-race`).
- A provider switch continues the conversation coherently; switch-back resumes native context; `crowbar handoff dump <chat>` prints the assembled ledger.
- `go test -tags noEmbed -race ./...` green; coverage ≥ 92%; `go vet` clean; `go build -tags noEmbed ./cmd/crowbar` OK.
- Integration Tests A + B pass when the CLIs are present (skip cleanly when absent).

---

## Self-Review (completed by the plan author)

- **Spec coverage:** §2 interaction model → Global Constraints + no message endpoint anywhere. §3 architecture → file map. §4 descriptor → Tasks 4–6 (+ the two YAMLs). §5 hooks → Tasks 9–10 + render_hooks. §6 chat/ledger → Tasks 11–12. §7 detection → Task 13. §8 switch → Tasks 16–18. §9 HTTP/WS → Task 15 + 18 (D3 routing note). §10 invariants → Global Constraints + litmus. §11 verification → Task 19. §12 build order → Phases B→C→D. All covered.
- **Deviations flagged:** D1–D7 documented up top; a reviewer should pressure-test D1 (new models vs the dormant event-sourced Chat), D2 (ledger path), D3 (routing) first.
- **Known thin spots for the review loop to harden:** Task 14 & 15 lean on "mirror the existing X" rather than full code for pure boilerplate (endpoint/WS/container wiring) — acceptable because the scouts quoted the exact patterns, but the reviewer should confirm each mirror target still exists. Task 19's `CROWBAR_HOOK_BIN` test seam must be wired in Task 14's `TemplateCtx.CrowbarHook` computation.
- **Type consistency:** `Outcome.Kind` values (`noop|bound|focus|registered`) are used identically in Tasks 13–15. `CanonicalEvent.Kind` (`session_start|turn_stop`) consistent Tasks 10/14. `SpawnPlan`/`TemplateCtx` fields consistent Tasks 5–6, 14, 17.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-05-agentic-engine.md`. Per the user's directive, execution proceeds **subagent-driven** (superpowers:subagent-driven-development): a fresh subagent per task with a two-stage review gate between tasks — but first the plan itself goes through a reviewer↔implementer loop (below) until a clean approval.
