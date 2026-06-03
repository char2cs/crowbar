# Crowbar API Layer, WebSocket Channels & Query Cache Persistence — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire Crowbar's web UI to a mock-stubbed Go backend with typed REST + per-resource WebSocket channels, replace IS_MOCK with MSW, and persist the TanStack Query cache to IDB for instant startup.

**Architecture:** Go server exposes REST endpoints backed by `//go:embed` JSON fixtures and a chaos middleware. One WS endpoint per resource type (each carries one object shape). MSW intercepts all HTTP + WS in dev so query functions contain zero mock awareness. TanStack `PersistQueryClientProvider` restores the query cache from IDB on startup; cache buster is the git SHA injected by Vite.

**Tech Stack:** Go 1.25 + Gin + gorilla/websocket + google/uuid; MSW v2; @tanstack/react-query-persist-client; @tanstack/query-async-storage-persister; Vite define.

**⚡ Parallelism:** Tasks 1–8 (Go) and Tasks 9–15 (Frontend) are fully independent. Run them with separate agents simultaneously.

---

## Go Module: `github.com/char2cs/crowbar/api`

---

### Task 1: Add deps + Chaos middleware

**Files:**
- Create: `api/internal/api/middleware/chaos.go`
- Create: `api/internal/api/middleware/chaos_test.go`
- Modify: `api/go.mod` (new deps)
- Modify: `api/internal/api/container.go` (apply middleware)

- [ ] **Step 1: Add Go dependencies**

```bash
cd api
go get github.com/gorilla/websocket@v1.5.3
go get github.com/google/uuid@v1.6.0
```

Expected: go.mod and go.sum updated.

- [ ] **Step 2: Write failing tests**

Create `api/internal/api/middleware/chaos_test.go`:

```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/api/middleware"
)

func TestChaos_Latency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.Chaos())
	r.GET("/", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Crowbar-Latency", "50")
	w := httptest.NewRecorder()
	start := time.Now()
	r.ServeHTTP(w, req)
	if time.Since(start) < 50*time.Millisecond {
		t.Fatal("expected at least 50ms delay")
	}
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestChaos_ErrorRate_Always(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.Chaos())
	r.GET("/", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Crowbar-Error-Rate", "1.0")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestChaos_NoHeaders_PassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.Chaos())
	r.GET("/", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
```

- [ ] **Step 3: Run tests to confirm they fail**

```bash
cd api && go test ./internal/api/middleware/...
```

Expected: FAIL — `middleware.Chaos` undefined.

- [ ] **Step 4: Implement chaos middleware**

Create `api/internal/api/middleware/chaos.go`:

```go
package middleware

import (
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func Chaos() gin.HandlerFunc {
	return func(c *gin.Context) {
		if d := c.GetHeader("X-Crowbar-Latency"); d != "" {
			if ms, err := strconv.Atoi(d); err == nil && ms > 0 {
				time.Sleep(time.Duration(ms) * time.Millisecond)
			}
		}
		if r := c.GetHeader("X-Crowbar-Error-Rate"); r != "" {
			if rate, err := strconv.ParseFloat(r, 64); err == nil && rand.Float64() < rate {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "chaos injection"})
				return
			}
		}
		c.Next()
	}
}
```

- [ ] **Step 5: Run tests — confirm pass**

```bash
cd api && go test ./internal/api/middleware/...
```

Expected: PASS.

- [ ] **Step 6: Apply chaos middleware in API container**

Edit `api/internal/api/container.go` — add chaos to the v0 group:

```go
apiV0 := router.Group("/api/v0")
apiV0.Use(middleware.Chaos())
v0.Register(apiV0, appContainer)
```

- [ ] **Step 7: Commit**

```bash
cd api && go build ./...
git add api/
git commit -m "feat(api): add gorilla/websocket dep and chaos middleware"
```

---

### Task 2: Fixture types + Store

**Files:**
- Create: `api/internal/fixtures/types.go`
- Create: `api/internal/fixtures/store.go`
- Create: `api/internal/fixtures/store_test.go`

- [ ] **Step 1: Write failing store test**

Create `api/internal/fixtures/store_test.go`:

```go
package fixtures_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/fixtures"
)

func TestStore_GetWorkspace_Found(t *testing.T) {
	s := fixtures.NewStore()
	ws := fixtures.WorkspacePayload{ID: "ws1", RepoID: "crowbar", Branch: "main"}
	s.AddWorkspace(ws)

	got, ok := s.GetWorkspace("ws1")
	if !ok {
		t.Fatal("expected workspace to be found")
	}
	if got.ID != "ws1" {
		t.Fatalf("expected id ws1, got %s", got.ID)
	}
}

func TestStore_GetWorkspace_Missing(t *testing.T) {
	s := fixtures.NewStore()
	_, ok := s.GetWorkspace("missing")
	if ok {
		t.Fatal("expected workspace not found")
	}
}

func TestStore_AddProject(t *testing.T) {
	s := fixtures.NewStore()
	s.AddProject(fixtures.Project{ID: "p1", Name: "Crowbar"})
	projects := s.ListProjects()
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
}
```

- [ ] **Step 2: Run — confirm fail**

```bash
cd api && go test ./internal/fixtures/...
```

Expected: FAIL — package not found.

- [ ] **Step 3: Create types.go**

Create `api/internal/fixtures/types.go`:

```go
package fixtures

import "time"

type UIMode string

const (
	UIModeChat UIMode = "chat"
	UIModeDiff UIMode = "diff"
)

type FlowStateDefinition struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	UI    UIMode `json:"ui"`
}

type FlowDefinition struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	States      []FlowStateDefinition `json:"states"`
}

type WorkspacePayload struct {
	ID           string         `json:"id"`
	RepoID       string         `json:"repoId"`
	Branch       string         `json:"branch"`
	FlowName     string         `json:"flowName"`
	CurrentState string         `json:"currentState"`
	Flow         FlowDefinition `json:"flow"`
}

type ChatMessage struct {
	ID             string  `json:"id"`
	Role           string  `json:"role"`
	Content        string  `json:"content"`
	AuthorName     string  `json:"authorName"`
	AuthorInitials string  `json:"authorInitials"`
	ModelName      string  `json:"modelName,omitempty"`
	Timestamp      string  `json:"timestamp"`
	ToolCalls      int     `json:"toolCalls,omitempty"`
	DurationSec    float64 `json:"durationSec,omitempty"`
}

type Project struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	LastActivity time.Time `json:"lastActivity"`
}

type FileStatus string

const (
	FileStatusModified FileStatus = "modified"
	FileStatusAdded    FileStatus = "added"
	FileStatusDeleted  FileStatus = "deleted"
	FileStatusRenamed  FileStatus = "renamed"
)

type GitFile struct {
	Path   string     `json:"path"`
	Status FileStatus `json:"status"`
}

type GitStatus struct {
	Branch   string    `json:"branch"`
	Staged   []GitFile `json:"staged"`
	Unstaged []GitFile `json:"unstaged"`
}

type Commit struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"shortHash"`
	Message   string `json:"message"`
	Author    string `json:"author"`
	Date      string `json:"date"`
}

type Branch struct {
	Name       string `json:"name"`
	IsCurrent  bool   `json:"isCurrent"`
	IsRemote   bool   `json:"isRemote"`
	LastCommit string `json:"lastCommit,omitempty"`
}

type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Type     string     `json:"type"`
	Size     int64      `json:"size,omitempty"`
	Children []FileNode `json:"children,omitempty"`
}

// WS event shapes — one per channel

type WorkspaceEvent struct {
	WorkspaceID string `json:"workspaceId"`
	Action      string `json:"action"` // "created" | "updated" | "deleted"
}

type GitEvent struct {
	Repo    string `json:"repo"`
	Changed bool   `json:"changed"`
}

type FileEvent struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

type ChatChunk struct {
	ChatID  string `json:"chatId"`
	Content string `json:"content"`
	Done    bool   `json:"done"`
}

type TerminalFrame struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
	IsInput   bool   `json:"isInput"`
}

type DaemonStatus struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
}
```

- [ ] **Step 4: Create store.go**

Create `api/internal/fixtures/store.go`:

```go
package fixtures

import "sync"

type Store struct {
	// Read-only after Load() — no lock needed for reads
	Flows         []FlowDefinition
	FileTree      FileNode
	GitLog        []Commit
	GitBranches   []Branch
	GitStatus     GitStatus
	Conversations map[string][]ChatMessage

	mu         sync.RWMutex
	workspaces map[string]WorkspacePayload
	projects   []Project
}

func NewStore() *Store {
	return &Store{
		workspaces:    make(map[string]WorkspacePayload),
		Conversations: make(map[string][]ChatMessage),
	}
}

func (s *Store) GetWorkspace(id string) (WorkspacePayload, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ws, ok := s.workspaces[id]
	return ws, ok
}

func (s *Store) AddWorkspace(ws WorkspacePayload) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaces[ws.ID] = ws
}

func (s *Store) ListWorkspaces() []WorkspacePayload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]WorkspacePayload, 0, len(s.workspaces))
	for _, ws := range s.workspaces {
		out = append(out, ws)
	}
	return out
}

func (s *Store) ListProjects() []Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Project, len(s.projects))
	copy(out, s.projects)
	return out
}

func (s *Store) AddProject(p Project) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects = append(s.projects, p)
}
```

- [ ] **Step 5: Run tests — confirm pass**

```bash
cd api && go test ./internal/fixtures/...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/fixtures/
git commit -m "feat(api): add fixture types and Store"
```

---

### Task 3: Fixture JSON files + generator

**Files:**
- Create: `api/internal/fixtures/workspaces.json`
- Create: `api/internal/fixtures/flows.json`
- Create: `api/internal/fixtures/projects.json`
- Create: `api/internal/fixtures/conversations.json`
- Create: `api/internal/fixtures/git-branches.json`
- Create: `api/internal/fixtures/git-status.json`
- Create: `api/cmd/fixtures-gen/main.go` (generates git-log.json + file-tree.json)

- [ ] **Step 1: Create flows.json**

Create `api/internal/fixtures/flows.json`:

```json
[
  {
    "name": "feature-development",
    "description": "Standard feature development flow",
    "states": [
      { "name": "brainstorming", "label": "Brainstorm", "ui": "chat" },
      { "name": "spec", "label": "Spec", "ui": "chat" },
      { "name": "implementation", "label": "Implement", "ui": "diff" },
      { "name": "ai_review", "label": "AI Review", "ui": "chat" },
      { "name": "human_review", "label": "Human Review", "ui": "diff" }
    ]
  },
  {
    "name": "bugfix",
    "description": "Bug investigation and fix",
    "states": [
      { "name": "investigate", "label": "Investigate", "ui": "chat" },
      { "name": "fix", "label": "Fix", "ui": "diff" },
      { "name": "review", "label": "Review", "ui": "diff" }
    ]
  },
  {
    "name": "refactor",
    "description": "Code refactoring flow",
    "states": [
      { "name": "analyze", "label": "Analyze", "ui": "chat" },
      { "name": "refactor", "label": "Refactor", "ui": "diff" },
      { "name": "test", "label": "Test", "ui": "diff" },
      { "name": "review", "label": "Review", "ui": "diff" }
    ]
  },
  {
    "name": "hotfix",
    "description": "Production hotfix",
    "states": [
      { "name": "investigate", "label": "Investigate", "ui": "chat" },
      { "name": "patch", "label": "Patch", "ui": "diff" },
      { "name": "deploy", "label": "Deploy", "ui": "chat" }
    ]
  },
  {
    "name": "research",
    "description": "Research and prototype",
    "states": [
      { "name": "research", "label": "Research", "ui": "chat" },
      { "name": "prototype", "label": "Prototype", "ui": "diff" },
      { "name": "evaluate", "label": "Evaluate", "ui": "chat" }
    ]
  }
]
```

- [ ] **Step 2: Create workspaces.json**

Create `api/internal/fixtures/workspaces.json` (20 entries across 3 repos — use feature-development flow for all):

```json
[
  { "id": "ws-develop",  "repoId": "crowbar",        "branch": "develop",                  "flowName": "feature-development", "currentState": "brainstorming", "flow": {} },
  { "id": "ws3",         "repoId": "crowbar",        "branch": "feature/app-design",       "flowName": "feature-development", "currentState": "brainstorming", "flow": {} },
  { "id": "ws1",         "repoId": "crowbar",        "branch": "enhancement/scaffold",     "flowName": "feature-development", "currentState": "implementation","flow": {} },
  { "id": "ws-fix",      "repoId": "crowbar",        "branch": "fix/toolbar-crash",        "flowName": "bugfix",              "currentState": "investigate",   "flow": {} },
  { "id": "ws2",         "repoId": "crowbar",        "branch": "feature/api-backend",      "flowName": "feature-development", "currentState": "human_review",  "flow": {} },
  { "id": "ws4",         "repoId": "crowbar",        "branch": "feature/search",           "flowName": "feature-development", "currentState": "spec",          "flow": {} },
  { "id": "ws5",         "repoId": "crowbar",        "branch": "feature/notifications",    "flowName": "feature-development", "currentState": "brainstorming", "flow": {} },
  { "id": "ws6",         "repoId": "crowbar",        "branch": "fix/memory-leak",          "flowName": "bugfix",              "currentState": "fix",           "flow": {} },
  { "id": "ws7",         "repoId": "crowbar",        "branch": "feature/dashboard",        "flowName": "feature-development", "currentState": "ai_review",     "flow": {} },
  { "id": "qc-develop",  "repoId": "quiver-core",    "branch": "develop",                  "flowName": "feature-development", "currentState": "brainstorming", "flow": {} },
  { "id": "qc1",         "repoId": "quiver-core",    "branch": "feature/old-auth",         "flowName": "feature-development", "currentState": "human_review",  "flow": {} },
  { "id": "qc2",         "repoId": "quiver-core",    "branch": "feature/oauth2",           "flowName": "feature-development", "currentState": "implementation","flow": {} },
  { "id": "qc3",         "repoId": "quiver-core",    "branch": "fix/session-leak",         "flowName": "bugfix",              "currentState": "investigate",   "flow": {} },
  { "id": "qc4",         "repoId": "quiver-core",    "branch": "feature/mfa",              "flowName": "feature-development", "currentState": "spec",          "flow": {} },
  { "id": "qc5",         "repoId": "quiver-core",    "branch": "refactor/db-layer",        "flowName": "refactor",            "currentState": "analyze",       "flow": {} },
  { "id": "qd-develop",  "repoId": "quiver-desktop", "branch": "develop",                  "flowName": "feature-development", "currentState": "brainstorming", "flow": {} },
  { "id": "qd2",         "repoId": "quiver-desktop", "branch": "feature/quiver-shell",     "flowName": "feature-development", "currentState": "implementation","flow": {} },
  { "id": "qd3",         "repoId": "quiver-desktop", "branch": "feature/tray-menu",        "flowName": "feature-development", "currentState": "brainstorming", "flow": {} },
  { "id": "qd4",         "repoId": "quiver-desktop", "branch": "fix/crash-on-open",        "flowName": "bugfix",              "currentState": "fix",           "flow": {} },
  { "id": "qd5",         "repoId": "quiver-desktop", "branch": "feature/auto-update",      "flowName": "feature-development", "currentState": "ai_review",     "flow": {} }
]
```

Note: the `"flow": {}` placeholders are filled in by the loader (it joins workspace.flowName → flows array).

- [ ] **Step 3: Create projects.json**

Create `api/internal/fixtures/projects.json`:

```json
[
  { "id": "proj-1",  "name": "crowbar",         "path": "/Users/dev/crowbar",          "lastActivity": "2026-05-29T14:00:00Z" },
  { "id": "proj-2",  "name": "quiver-core",     "path": "/Users/dev/quiver-core",      "lastActivity": "2026-05-28T09:30:00Z" },
  { "id": "proj-3",  "name": "quiver-desktop",  "path": "/Users/dev/quiver-desktop",   "lastActivity": "2026-05-27T16:45:00Z" },
  { "id": "proj-4",  "name": "quiver-api",      "path": "/Users/dev/quiver-api",       "lastActivity": "2026-05-26T11:00:00Z" },
  { "id": "proj-5",  "name": "infra",           "path": "/Users/dev/infra",            "lastActivity": "2026-05-25T08:00:00Z" },
  { "id": "proj-6",  "name": "design-system",   "path": "/Users/dev/design-system",    "lastActivity": "2026-05-24T13:15:00Z" },
  { "id": "proj-7",  "name": "docs",            "path": "/Users/dev/docs",             "lastActivity": "2026-05-23T10:00:00Z" },
  { "id": "proj-8",  "name": "marketing-site",  "path": "/Users/dev/marketing-site",   "lastActivity": "2026-05-22T17:30:00Z" },
  { "id": "proj-9",  "name": "cli",             "path": "/Users/dev/cli",              "lastActivity": "2026-05-21T09:00:00Z" },
  { "id": "proj-10", "name": "analytics",       "path": "/Users/dev/analytics",        "lastActivity": "2026-05-20T14:00:00Z" }
]
```

- [ ] **Step 4: Create git-branches.json**

Create `api/internal/fixtures/git-branches.json` (50 branches: 20 active, 30 stale):

```json
[
  { "name": "develop",                     "isCurrent": false, "isRemote": false, "lastCommit": "chore: weekly release prep" },
  { "name": "enhancement/design-language", "isCurrent": true,  "isRemote": false, "lastCommit": "feat: add workspace tree redesign" },
  { "name": "feature/app-design",          "isCurrent": false, "isRemote": false, "lastCommit": "feat: scaffold IDE shell" },
  { "name": "enhancement/scaffold",        "isCurrent": false, "isRemote": false, "lastCommit": "feat: add pane system" },
  { "name": "fix/toolbar-crash",           "isCurrent": false, "isRemote": false, "lastCommit": "fix: null deref in toolbar" },
  { "name": "feature/api-backend",         "isCurrent": false, "isRemote": false, "lastCommit": "feat: add REST endpoints" },
  { "name": "feature/search",              "isCurrent": false, "isRemote": false, "lastCommit": "feat: fuzzy file search" },
  { "name": "feature/notifications",       "isCurrent": false, "isRemote": false, "lastCommit": "feat: toast notification system" },
  { "name": "fix/memory-leak",             "isCurrent": false, "isRemote": false, "lastCommit": "fix: clear tree cache on close" },
  { "name": "feature/dashboard",           "isCurrent": false, "isRemote": false, "lastCommit": "feat: activity dashboard" },
  { "name": "refactor/store-migration",    "isCurrent": false, "isRemote": false, "lastCommit": "refactor: migrate to zustand" },
  { "name": "feature/settings-v2",         "isCurrent": false, "isRemote": false, "lastCommit": "feat: settings redesign" },
  { "name": "fix/idb-schema",             "isCurrent": false, "isRemote": false, "lastCommit": "fix: add schema versioning" },
  { "name": "feature/tab-groups",          "isCurrent": false, "isRemote": false, "lastCommit": "feat: editor tab groups" },
  { "name": "feature/ai-review",           "isCurrent": false, "isRemote": false, "lastCommit": "feat: inline AI review" },
  { "name": "feature/git-diff",            "isCurrent": false, "isRemote": false, "lastCommit": "feat: split diff view" },
  { "name": "fix/ws-reconnect",            "isCurrent": false, "isRemote": false, "lastCommit": "fix: exponential backoff" },
  { "name": "feature/terminal-v2",         "isCurrent": false, "isRemote": false, "lastCommit": "feat: PTY terminal rewrite" },
  { "name": "chore/deps-update",           "isCurrent": false, "isRemote": false, "lastCommit": "chore: bump vite to 6.3" },
  { "name": "feature/collaboration",       "isCurrent": false, "isRemote": false, "lastCommit": "feat: multi-cursor stub" },
  { "name": "origin/develop",              "isCurrent": false, "isRemote": true,  "lastCommit": "chore: weekly release prep" },
  { "name": "origin/main",                 "isCurrent": false, "isRemote": true,  "lastCommit": "chore: v0.8.0 release" },
  { "name": "origin/feature/app-design",   "isCurrent": false, "isRemote": true,  "lastCommit": "feat: scaffold IDE shell" },
  { "name": "origin/feature/api-backend",  "isCurrent": false, "isRemote": true,  "lastCommit": "feat: add REST endpoints" },
  { "name": "origin/fix/toolbar-crash",    "isCurrent": false, "isRemote": true,  "lastCommit": "fix: null deref in toolbar" },
  { "name": "stale/feature/old-payments",  "isCurrent": false, "isRemote": false, "lastCommit": "wip: payment prototype" },
  { "name": "stale/feature/old-auth",      "isCurrent": false, "isRemote": false, "lastCommit": "wip: basic auth sketch" },
  { "name": "stale/fix/layout-bug",        "isCurrent": false, "isRemote": false, "lastCommit": "fix: attempt flex layout" },
  { "name": "stale/feature/import-export", "isCurrent": false, "isRemote": false, "lastCommit": "feat: csv import wip" },
  { "name": "stale/chore/cleanup",         "isCurrent": false, "isRemote": false, "lastCommit": "chore: remove old files" },
  { "name": "stale/feature/onboarding",    "isCurrent": false, "isRemote": false, "lastCommit": "feat: onboarding flow draft" },
  { "name": "stale/feature/billing",       "isCurrent": false, "isRemote": false, "lastCommit": "feat: stripe integration wip" },
  { "name": "stale/fix/perf-regression",   "isCurrent": false, "isRemote": false, "lastCommit": "perf: profile rendering" },
  { "name": "stale/feature/themes",        "isCurrent": false, "isRemote": false, "lastCommit": "feat: theme picker draft" },
  { "name": "stale/refactor/old-router",   "isCurrent": false, "isRemote": false, "lastCommit": "refactor: migrate react-router" },
  { "name": "stale/feature/mobile",        "isCurrent": false, "isRemote": false, "lastCommit": "feat: responsive layout wip" },
  { "name": "stale/feature/webhooks",      "isCurrent": false, "isRemote": false, "lastCommit": "feat: webhook delivery draft" },
  { "name": "stale/fix/safari-compat",     "isCurrent": false, "isRemote": false, "lastCommit": "fix: safari flexbox workaround" },
  { "name": "stale/feature/plugin-api",    "isCurrent": false, "isRemote": false, "lastCommit": "feat: plugin system sketch" },
  { "name": "stale/chore/remove-cra",      "isCurrent": false, "isRemote": false, "lastCommit": "chore: migrate off CRA" },
  { "name": "stale/feature/e2e-tests",     "isCurrent": false, "isRemote": false, "lastCommit": "test: playwright setup wip" },
  { "name": "stale/feature/analytics",     "isCurrent": false, "isRemote": false, "lastCommit": "feat: event tracking draft" },
  { "name": "stale/fix/idb-corruption",    "isCurrent": false, "isRemote": false, "lastCommit": "fix: handle corrupted idb" },
  { "name": "stale/feature/shortcuts",     "isCurrent": false, "isRemote": false, "lastCommit": "feat: keybinding system draft" },
  { "name": "stale/feature/search-v1",     "isCurrent": false, "isRemote": false, "lastCommit": "feat: basic text search" },
  { "name": "stale/refactor/css-vars",     "isCurrent": false, "isRemote": false, "lastCommit": "refactor: css custom properties" },
  { "name": "stale/feature/dark-mode",     "isCurrent": false, "isRemote": false, "lastCommit": "feat: dark mode toggle draft" },
  { "name": "stale/fix/scroll-jump",       "isCurrent": false, "isRemote": false, "lastCommit": "fix: scroll position reset" },
  { "name": "stale/feature/workspace-v1",  "isCurrent": false, "isRemote": false, "lastCommit": "feat: workspace concept draft" },
  { "name": "stale/chore/monorepo",        "isCurrent": false, "isRemote": false, "lastCommit": "chore: pnpm workspace setup" }
]
```

- [ ] **Step 5: Create git-status.json**

Create `api/internal/fixtures/git-status.json`:

```json
{
  "branch": "enhancement/design-language",
  "staged": [
    { "path": "web/src/components/layout/IDEShell.tsx",          "status": "modified" },
    { "path": "web/src/components/layout/workspace-branch-icon.tsx", "status": "modified" },
    { "path": "web/src/components/layout/workspace-inline-input.tsx","status": "modified" },
    { "path": "web/src/components/layout/workspace-tree-footer.tsx", "status": "modified" },
    { "path": "web/src/features/editor/components/monaco-editor.tsx","status": "modified" },
    { "path": "web/src/features/panes/components/pane-resize-handle.tsx","status": "modified" },
    { "path": "web/src/lib/persistence/idb.ts",                  "status": "modified" },
    { "path": "web/src/lib/persistence/schemas.ts",              "status": "modified" },
    { "path": "api/internal/fixtures/types.go",                  "status": "added" },
    { "path": "api/internal/fixtures/store.go",                  "status": "added" },
    { "path": "api/internal/api/middleware/chaos.go",            "status": "added" },
    { "path": "web/src/mocks/browser.ts",                        "status": "added" },
    { "path": "web/src/lib/ws/manager.ts",                       "status": "added" },
    { "path": "web/src/lib/ws/types.ts",                         "status": "added" }
  ],
  "unstaged": [
    { "path": "web/src/lib/api.ts",                              "status": "modified" },
    { "path": "web/src/lib/queries.ts",                          "status": "modified" },
    { "path": "web/src/main.tsx",                                "status": "modified" },
    { "path": "web/vite.config.ts",                              "status": "modified" },
    { "path": "web/src/components/dev/chaos-panel.tsx",          "status": "added" },
    { "path": "docs/superpowers/specs/2026-05-30-api-layer-ws-channels-query-cache-design.md", "status": "added" },
    { "path": "old-feature.ts",                                  "status": "deleted" },
    { "path": "src/legacy/payment.ts",                           "status": "deleted" },
    { "path": "src/auth/session.ts",                             "status": "modified" },
    { "path": "src/auth/middleware.ts",                          "status": "renamed" },
    { "path": "api/go.mod",                                      "status": "modified" },
    { "path": "api/go.sum",                                      "status": "modified" },
    { "path": "README.md",                                       "status": "modified" },
    { "path": "package.json",                                    "status": "modified" },
    { "path": "web/package-lock.json",                           "status": "modified" }
  ]
}
```

- [ ] **Step 6: Create conversations.json**

Create `api/internal/fixtures/conversations.json` — keyed by wsId, each with a mix of message types (the loader populates `Conversations["default"]`):

```json
{
  "default": [
    { "id": "msg-1",  "role": "user",      "content": "Let's build the payment module.",                                                                           "authorName": "Mateo Urrutia",  "authorInitials": "MU", "timestamp": "2026-05-29T10:00:00Z" },
    { "id": "msg-2",  "role": "assistant", "content": "I'll start by scaffolding the PaymentService class with typed error codes.",                                "authorName": "Claude",         "authorInitials": "C",  "timestamp": "2026-05-29T10:00:05Z", "modelName": "claude-sonnet-4-6", "durationSec": 4.2 },
    { "id": "msg-3",  "role": "assistant", "content": "Created `PaymentService.ts` and `PaymentError.ts`. Running tests now.",                                     "authorName": "Claude",         "authorInitials": "C",  "timestamp": "2026-05-29T10:00:10Z", "modelName": "claude-sonnet-4-6", "toolCalls": 5, "durationSec": 12.8 },
    { "id": "msg-4",  "role": "user",      "content": "Can you add retry logic with exponential backoff?",                                                         "authorName": "Mateo Urrutia",  "authorInitials": "MU", "timestamp": "2026-05-29T10:01:00Z" },
    { "id": "msg-5",  "role": "assistant", "content": "Added `RetryPolicy` class with configurable maxRetries and backoff multiplier. Tests pass.",                "authorName": "Claude",         "authorInitials": "C",  "timestamp": "2026-05-29T10:01:20Z", "modelName": "claude-sonnet-4-6", "toolCalls": 8, "durationSec": 18.3 },
    { "id": "msg-6",  "role": "user",      "content": "What about webhook delivery?",                                                                              "authorName": "Mateo Urrutia",  "authorInitials": "MU", "timestamp": "2026-05-29T10:02:00Z" },
    { "id": "msg-7",  "role": "assistant", "content": "Webhook delivery needs a queue. I'll use an in-memory queue for now with a TODO for Redis in production.",  "authorName": "Claude",         "authorInitials": "C",  "timestamp": "2026-05-29T10:02:15Z", "modelName": "claude-sonnet-4-6", "durationSec": 2.1 },
    { "id": "msg-8",  "role": "assistant", "content": "Implemented `WebhookQueue` with FIFO delivery and configurable concurrency. 12 tests added.",              "authorName": "Claude",         "authorInitials": "C",  "timestamp": "2026-05-29T10:02:45Z", "modelName": "claude-sonnet-4-6", "toolCalls": 15, "durationSec": 31.7 },
    { "id": "msg-9",  "role": "user",      "content": "Looks great. Let's move to the review phase.",                                                             "authorName": "Mateo Urrutia",  "authorInitials": "MU", "timestamp": "2026-05-29T10:03:30Z" },
    { "id": "msg-10", "role": "assistant", "content": "I'll do a security review of the payment module before we hand off to human review.",                      "authorName": "Claude",         "authorInitials": "C",  "timestamp": "2026-05-29T10:03:35Z", "modelName": "claude-sonnet-4-6", "durationSec": 1.2 }
  ]
}
```

Note: The real conversations.json should have ~500 messages. Copy the pattern above, generating entries with `msg-N` IDs, alternating user/assistant roles, varying toolCalls and durationSec.

- [ ] **Step 7: Write the large-fixture generator**

Create `api/cmd/fixtures-gen/main.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
)

type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Type     string     `json:"type"`
	Size     int64      `json:"size,omitempty"`
	Children []FileNode `json:"children,omitempty"`
}

type Commit struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"shortHash"`
	Message   string `json:"message"`
	Author    string `json:"author"`
	Date      string `json:"date"`
}

var commitMsgs = []string{
	"feat: add payment processing module",
	"fix: resolve null pointer in auth middleware",
	"refactor: extract database connection pool",
	"chore: update dependencies to latest versions",
	"docs: add API endpoint documentation",
	"test: add integration tests for user service",
	"perf: optimize query execution with indexes",
	"feat: implement real-time notification system",
	"fix: handle edge case in date parsing",
	"Merge branch 'feature/payments' into develop",
	"feat: add CSV export functionality",
	"fix: correct timezone handling in scheduler",
	"chore: add CI/CD pipeline configuration",
	"style: apply linting rules across codebase",
	"feat: add dashboard analytics widgets",
}

var authors = []string{"Mateo Urrutia", "Claude Agent", "Dependabot[bot]"}

var exts = []string{".ts", ".tsx", ".go", ".json", ".md", ".yaml", ".css", ".test.ts"}

var dirNames = []string{
	"components", "utils", "services", "models", "hooks",
	"api", "lib", "types", "store", "features", "tests", "docs",
}

func generateTree(path string, depth, maxDepth int, count, target *int) FileNode {
	node := FileNode{Name: filepath.Base(path), Path: path, Type: "directory"}
	if depth >= maxDepth || *count >= *target {
		return node
	}
	n := rand.Intn(6) + 2
	for i := 0; i < n && *count < *target; i++ {
		(*count)++
		if depth < maxDepth-1 && rand.Float32() < 0.35 {
			dirName := fmt.Sprintf("%s-%d", dirNames[rand.Intn(len(dirNames))], i)
			child := generateTree(path+"/"+dirName, depth+1, maxDepth, count, target)
			node.Children = append(node.Children, child)
		} else {
			ext := exts[rand.Intn(len(exts))]
			fname := fmt.Sprintf("file-%d%s", i, ext)
			node.Children = append(node.Children, FileNode{
				Name: fname,
				Path: path + "/" + fname,
				Type: "file",
				Size: int64(rand.Intn(200000) + 100),
			})
		}
	}
	return node
}

func generateLog(n int) []Commit {
	commits := make([]Commit, n)
	for i := range commits {
		h := fmt.Sprintf("%040x", rand.Int63()^int64(i))
		commits[i] = Commit{
			Hash:      h,
			ShortHash: h[:7],
			Message:   commitMsgs[rand.Intn(len(commitMsgs))],
			Author:    authors[rand.Intn(len(authors))],
			Date:      fmt.Sprintf("%d days ago", i/10+1),
		}
	}
	return commits
}

func writeJSON(path string, v any) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
}

func main() {
	outDir := "api/internal/fixtures"

	count, target := 0, 5000
	tree := generateTree("crowbar", 0, 7, &count, &target)
	writeJSON(filepath.Join(outDir, "file-tree.json"), tree)
	fmt.Printf("file-tree.json: %d nodes\n", count)

	log := generateLog(2000)
	writeJSON(filepath.Join(outDir, "git-log.json"), log)
	fmt.Printf("git-log.json: %d commits\n", len(log))
}
```

- [ ] **Step 8: Run the generator from repo root**

```bash
go run api/cmd/fixtures-gen/main.go
```

Expected output:
```
file-tree.json: ~5000 nodes
git-log.json: 2000 commits
```

Verify both files appear in `api/internal/fixtures/`.

- [ ] **Step 9: Commit**

```bash
git add api/internal/fixtures/ api/cmd/
git commit -m "feat(api): add fixture JSON files and large-fixture generator"
```

---

### Task 4: Fixture loader

**Files:**
- Create: `api/internal/fixtures/loader.go`
- Create: `api/internal/fixtures/loader_test.go`

- [ ] **Step 1: Write failing test**

Create `api/internal/fixtures/loader_test.go`:

```go
package fixtures_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/fixtures"
)

func TestLoad_ReturnsPopulatedStore(t *testing.T) {
	store, err := fixtures.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(store.Flows) == 0 {
		t.Fatal("expected flows to be loaded")
	}
	if len(store.ListWorkspaces()) == 0 {
		t.Fatal("expected workspaces to be loaded")
	}
	if len(store.ListProjects()) == 0 {
		t.Fatal("expected projects to be loaded")
	}
	if len(store.GitLog) == 0 {
		t.Fatal("expected git log to be loaded")
	}
	if len(store.GitBranches) == 0 {
		t.Fatal("expected git branches to be loaded")
	}
	if store.FileTree.Type != "directory" {
		t.Fatal("expected file tree root to be a directory")
	}
}

func TestLoad_WorkspacesHaveFlowsPopulated(t *testing.T) {
	store, err := fixtures.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	workspaces := store.ListWorkspaces()
	for _, ws := range workspaces {
		if ws.Flow.Name == "" {
			t.Fatalf("workspace %s has empty flow name", ws.ID)
		}
		if len(ws.Flow.States) == 0 {
			t.Fatalf("workspace %s has no flow states", ws.ID)
		}
	}
}
```

- [ ] **Step 2: Run — confirm fail**

```bash
cd api && go test ./internal/fixtures/... -run TestLoad
```

Expected: FAIL — `fixtures.Load` undefined.

- [ ] **Step 3: Implement loader**

Create `api/internal/fixtures/loader.go`:

```go
package fixtures

import (
	"encoding/json"
	_ "embed"
)

//go:embed workspaces.json
var workspacesJSON []byte

//go:embed flows.json
var flowsJSON []byte

//go:embed projects.json
var projectsJSON []byte

//go:embed conversations.json
var conversationsJSON []byte

//go:embed git-log.json
var gitLogJSON []byte

//go:embed git-branches.json
var gitBranchesJSON []byte

//go:embed git-status.json
var gitStatusJSON []byte

//go:embed file-tree.json
var fileTreeJSON []byte

// Load reads all embedded fixture files and returns a populated Store.
// Workspaces have their Flow field populated by joining on flowName.
func Load() (*Store, error) {
	s := NewStore()

	if err := json.Unmarshal(flowsJSON, &s.Flows); err != nil {
		return nil, err
	}

	flowByName := make(map[string]FlowDefinition, len(s.Flows))
	for _, f := range s.Flows {
		flowByName[f.Name] = f
	}

	var wsSlice []WorkspacePayload
	if err := json.Unmarshal(workspacesJSON, &wsSlice); err != nil {
		return nil, err
	}
	for _, ws := range wsSlice {
		ws.Flow = flowByName[ws.FlowName]
		s.workspaces[ws.ID] = ws
	}

	var projects []Project
	if err := json.Unmarshal(projectsJSON, &projects); err != nil {
		return nil, err
	}
	s.projects = projects

	if err := json.Unmarshal(conversationsJSON, &s.Conversations); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(gitLogJSON, &s.GitLog); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(gitBranchesJSON, &s.GitBranches); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(gitStatusJSON, &s.GitStatus); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(fileTreeJSON, &s.FileTree); err != nil {
		return nil, err
	}

	return s, nil
}
```

- [ ] **Step 4: Run tests — confirm pass**

```bash
cd api && go test ./internal/fixtures/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/fixtures/loader.go api/internal/fixtures/loader_test.go
git commit -m "feat(api): add fixture loader with go:embed"
```

---

### Task 5: REST handlers

**Files:**
- Create: `api/internal/api/v0/workspaces_handler.go`
- Create: `api/internal/api/v0/workspaces_handler_test.go`
- Create: `api/internal/api/v0/flows_handler.go`
- Create: `api/internal/api/v0/conversations_handler.go`
- Create: `api/internal/api/v0/projects_handler.go`
- Create: `api/internal/api/v0/fs_handler.go`
- Create: `api/internal/api/v0/git_handler.go`
- Create: `api/internal/api/v0/terminal_handler.go`

- [ ] **Step 1: Write workspaces handler test**

Create `api/internal/api/v0/workspaces_handler_test.go`:

```go
package v0_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

func newTestStore() *fixtures.Store {
	s := fixtures.NewStore()
	s.Flows = []fixtures.FlowDefinition{
		{
			Name: "feature-development",
			States: []fixtures.FlowStateDefinition{
				{Name: "brainstorming", Label: "Brainstorm", UI: "chat"},
			},
		},
	}
	s.AddWorkspace(fixtures.WorkspacePayload{
		ID: "ws1", RepoID: "crowbar", Branch: "main",
		FlowName: "feature-development", CurrentState: "brainstorming",
		Flow: s.Flows[0],
	})
	return s
}

func TestWorkspacesHandler_Get(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore()
	r := gin.New()
	h := v0.NewWorkspacesHandler(store)
	r.GET("/workspaces/:id", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/workspaces/ws1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp fixtures.WorkspacePayload
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.ID != "ws1" {
		t.Fatalf("expected id ws1, got %s", resp.ID)
	}
}

func TestWorkspacesHandler_Get_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := fixtures.NewStore()
	r := gin.New()
	h := v0.NewWorkspacesHandler(store)
	r.GET("/workspaces/:id", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/workspaces/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestWorkspacesHandler_Create(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore()
	r := gin.New()
	h := v0.NewWorkspacesHandler(store)
	r.POST("/workspaces", h.Create)

	body := `{"repoId":"crowbar","branch":"feature/new","flowName":"feature-development"}`
	req := httptest.NewRequest(http.MethodPost, "/workspaces", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d — body: %s", w.Code, w.Body.String())
	}
	var resp fixtures.WorkspacePayload
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ID == "" {
		t.Fatal("expected non-empty id")
	}
	if resp.Branch != "feature/new" {
		t.Fatalf("expected branch feature/new, got %s", resp.Branch)
	}
}
```

- [ ] **Step 2: Run — confirm fail**

```bash
cd api && go test ./internal/api/v0/... -run TestWorkspaces
```

Expected: FAIL — `v0.NewWorkspacesHandler` undefined.

- [ ] **Step 3: Implement workspaces handler**

Create `api/internal/api/v0/workspaces_handler.go`:

```go
package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

type WorkspacesHandler struct {
	store *fixtures.Store
}

func NewWorkspacesHandler(store *fixtures.Store) *WorkspacesHandler {
	return &WorkspacesHandler{store: store}
}

func (h *WorkspacesHandler) Get(c *gin.Context) {
	ws, ok := h.store.GetWorkspace(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}
	c.JSON(http.StatusOK, ws)
}

func (h *WorkspacesHandler) Create(c *gin.Context) {
	var req struct {
		RepoID   string `json:"repoId" binding:"required"`
		Branch   string `json:"branch" binding:"required"`
		FlowName string `json:"flowName" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var flow fixtures.FlowDefinition
	for _, f := range h.store.Flows {
		if f.Name == req.FlowName {
			flow = f
			break
		}
	}
	ws := fixtures.WorkspacePayload{
		ID:           uuid.New().String(),
		RepoID:       req.RepoID,
		Branch:       req.Branch,
		FlowName:     req.FlowName,
		CurrentState: func() string {
			if len(flow.States) > 0 {
				return flow.States[0].Name
			}
			return ""
		}(),
		Flow: flow,
	}
	h.store.AddWorkspace(ws)
	c.JSON(http.StatusCreated, ws)
}
```

- [ ] **Step 4: Run workspaces tests — confirm pass**

```bash
cd api && go test ./internal/api/v0/... -run TestWorkspaces
```

Expected: PASS.

- [ ] **Step 5: Implement remaining REST handlers**

Create `api/internal/api/v0/flows_handler.go`:

```go
package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

type FlowsHandler struct{ store *fixtures.Store }

func NewFlowsHandler(store *fixtures.Store) *FlowsHandler {
	return &FlowsHandler{store: store}
}

func (h *FlowsHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.Flows)
}
```

Create `api/internal/api/v0/conversations_handler.go`:

```go
package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

type ConversationsHandler struct{ store *fixtures.Store }

func NewConversationsHandler(store *fixtures.Store) *ConversationsHandler {
	return &ConversationsHandler{store: store}
}

func (h *ConversationsHandler) Get(c *gin.Context) {
	wsID := c.Param("wsId")
	msgs, ok := h.store.Conversations[wsID]
	if !ok {
		msgs = h.store.Conversations["default"]
	}
	c.JSON(http.StatusOK, gin.H{"messages": msgs})
}
```

Create `api/internal/api/v0/projects_handler.go`:

```go
package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/char2cs/crowbar/api/internal/fixtures"
	"time"
)

type ProjectsHandler struct{ store *fixtures.Store }

func NewProjectsHandler(store *fixtures.Store) *ProjectsHandler {
	return &ProjectsHandler{store: store}
}

func (h *ProjectsHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.ListProjects())
}

func (h *ProjectsHandler) Create(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p := fixtures.Project{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Path:         req.Path,
		LastActivity: time.Now(),
	}
	h.store.AddProject(p)
	c.JSON(http.StatusCreated, p)
}
```

Create `api/internal/api/v0/fs_handler.go`:

```go
package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

type FsHandler struct{ store *fixtures.Store }

func NewFsHandler(store *fixtures.Store) *FsHandler {
	return &FsHandler{store: store}
}

func (h *FsHandler) Tree(c *gin.Context) {
	// Returns the same stress-test tree regardless of root path
	c.JSON(http.StatusOK, h.store.FileTree)
}
```

Create `api/internal/api/v0/git_handler.go`:

```go
package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

type GitHandler struct{ store *fixtures.Store }

func NewGitHandler(store *fixtures.Store) *GitHandler {
	return &GitHandler{store: store}
}

func (h *GitHandler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.GitStatus)
}

func (h *GitHandler) Log(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.GitLog)
}

func (h *GitHandler) Branches(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.GitBranches)
}
```

Create `api/internal/api/v0/terminal_handler.go`:

```go
package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TerminalHandler struct{}

func NewTerminalHandler() *TerminalHandler { return &TerminalHandler{} }

func (h *TerminalHandler) CreateSession(c *gin.Context) {
	c.JSON(http.StatusCreated, gin.H{"sessionId": uuid.New().String()})
}
```

- [ ] **Step 6: Verify build**

```bash
cd api && go build ./...
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add api/internal/api/v0/
git commit -m "feat(api): add REST handlers for workspaces, flows, git, fs, terminal"
```

---

### Task 6: WS Hub + WS handlers

**Files:**
- Create: `api/internal/wshub/hub.go`
- Create: `api/internal/wshub/hub_test.go`
- Create: `api/internal/api/v0/ws_workspaces_handler.go`
- Create: `api/internal/api/v0/ws_git_handler.go`
- Create: `api/internal/api/v0/ws_files_handler.go`
- Create: `api/internal/api/v0/ws_chat_handler.go`
- Create: `api/internal/api/v0/ws_terminal_handler.go`
- Create: `api/internal/api/v0/ws_daemon_handler.go`

- [ ] **Step 1: Write hub test**

Create `api/internal/wshub/hub_test.go`:

```go
package wshub_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/char2cs/crowbar/api/internal/wshub"
	"github.com/gorilla/websocket"
)

func TestHub_BroadcastReachesClient(t *testing.T) {
	h := wshub.New()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		h.Register(conn)
		defer h.Unregister(conn)
		// keep connection open until test server closes
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	url := "ws" + srv.URL[4:]
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	type payload struct{ OK bool }
	h.Broadcast(payload{OK: true})

	_, msg, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(msg) != `{"OK":true}` {
		t.Fatalf("unexpected message: %s", msg)
	}
}
```

- [ ] **Step 2: Run — confirm fail**

```bash
cd api && go test ./internal/wshub/...
```

Expected: FAIL — package not found.

- [ ] **Step 3: Implement wshub**

Create `api/internal/wshub/hub.go`:

```go
package wshub

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
}

func New() *Hub {
	return &Hub{clients: make(map[*websocket.Conn]struct{})}
}

func (h *Hub) Register(conn *websocket.Conn) {
	h.mu.Lock()
	h.clients[conn] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) Unregister(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
}

func (h *Hub) Broadcast(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for conn := range h.clients {
		_ = conn.WriteMessage(websocket.TextMessage, data)
	}
}

func (h *Hub) Upgrader() *websocket.Upgrader { return &upgrader }
```

Fix the missing import in hub.go — add `"net/http"` to the import block.

- [ ] **Step 4: Run hub tests — confirm pass**

```bash
cd api && go test ./internal/wshub/...
```

Expected: PASS.

- [ ] **Step 5: Implement WS handlers**

Create `api/internal/api/v0/ws_workspaces_handler.go`:

```go
package v0

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/char2cs/crowbar/api/internal/fixtures"
	"github.com/char2cs/crowbar/api/internal/wshub"
)

var wsUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

type WSWorkspacesHandler struct {
	hub   *wshub.Hub
	store *fixtures.Store
}

func NewWSWorkspacesHandler(hub *wshub.Hub, store *fixtures.Store) *WSWorkspacesHandler {
	return &WSWorkspacesHandler{hub: hub, store: store}
}

func (h *WSWorkspacesHandler) Upgrade(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.hub.Register(conn)
	defer func() {
		h.hub.Unregister(conn)
		conn.Close()
	}()

	// Send initial snapshot then keep-alive pings
	_ = conn.WriteJSON(fixtures.WorkspaceEvent{WorkspaceID: "", Action: "snapshot"})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
```

Create `api/internal/api/v0/ws_git_handler.go`:

```go
package v0

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/fixtures"
	"github.com/char2cs/crowbar/api/internal/wshub"
)

type WSGitHandler struct {
	hub   *wshub.Hub
	store *fixtures.Store
}

func NewWSGitHandler(hub *wshub.Hub, store *fixtures.Store) *WSGitHandler {
	return &WSGitHandler{hub: hub, store: store}
}

func (h *WSGitHandler) Upgrade(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.hub.Register(conn)
	defer func() {
		h.hub.Unregister(conn)
		conn.Close()
	}()

	repo := c.Query("repo")
	_ = conn.WriteJSON(fixtures.GitEvent{Repo: repo, Changed: false})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
```

Create `api/internal/api/v0/ws_files_handler.go` — same pattern as ws_git_handler.go but sends `fixtures.FileEvent`.

Create `api/internal/api/v0/ws_chat_handler.go` — sends a mock `fixtures.ChatChunk` sequence on connection, then idles:

```go
package v0

import (
	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/fixtures"
	"github.com/char2cs/crowbar/api/internal/wshub"
)

type WSChatHandler struct {
	hub *wshub.Hub
}

func NewWSChatHandler(hub *wshub.Hub) *WSChatHandler {
	return &WSChatHandler{hub: hub}
}

func (h *WSChatHandler) Upgrade(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.hub.Register(conn)
	defer func() {
		h.hub.Unregister(conn)
		conn.Close()
	}()

	chatID := c.Param("chatId")
	words := []string{"Hello", " from", " the", " mock", " server", "!"}
	for _, w := range words {
		_ = conn.WriteJSON(fixtures.ChatChunk{ChatID: chatID, Content: w, Done: false})
	}
	_ = conn.WriteJSON(fixtures.ChatChunk{ChatID: chatID, Done: true})

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
```

Create `api/internal/api/v0/ws_terminal_handler.go` — echoes client input:

```go
package v0

import (
	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/fixtures"
	"github.com/char2cs/crowbar/api/internal/wshub"
)

type WSTerminalHandler struct {
	hub *wshub.Hub
}

func NewWSTerminalHandler(hub *wshub.Hub) *WSTerminalHandler {
	return &WSTerminalHandler{hub: hub}
}

func (h *WSTerminalHandler) Upgrade(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	sessionID := c.Param("sessionId")

	_ = conn.WriteJSON(fixtures.TerminalFrame{
		SessionID: sessionID,
		Data:      "crowbar mock terminal ready\r\n$ ",
		IsInput:   false,
	})

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		// Echo input back as output
		_ = conn.WriteJSON(fixtures.TerminalFrame{
			SessionID: sessionID,
			Data:      string(msg),
			IsInput:   false,
		})
	}
}
```

Create `api/internal/api/v0/ws_daemon_handler.go` — sends status on connect then idles:

```go
package v0

import (
	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/fixtures"
	"github.com/char2cs/crowbar/api/internal/wshub"
)

type WSDaemonHandler struct{ hub *wshub.Hub }

func NewWSDaemonHandler(hub *wshub.Hub) *WSDaemonHandler {
	return &WSDaemonHandler{hub: hub}
}

func (h *WSDaemonHandler) Upgrade(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.hub.Register(conn)
	defer func() {
		h.hub.Unregister(conn)
		conn.Close()
	}()

	_ = conn.WriteJSON(fixtures.DaemonStatus{Status: "running", Version: "0.1.0-mock"})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
```

- [ ] **Step 6: Build check**

```bash
cd api && go build ./...
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add api/internal/wshub/ api/internal/api/v0/ws_*.go
git commit -m "feat(api): add WS hub and per-channel WS handlers"
```

---

### Task 7: Wire everything — router + app container

**Files:**
- Modify: `api/internal/app/container.go`
- Modify: `api/internal/api/v0/router.go`
- Modify: `api/internal/api/container.go`

- [ ] **Step 1: Add Fixtures to app container**

Edit `api/internal/app/container.go`:

```go
package app

import (
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/fixtures"
	"github.com/char2cs/crowbar/api/internal/wshub"
)

type Container struct {
	Hub      *hub.Hub
	Health   *usecases.HealthUsecase
	Fixtures *fixtures.Store
	WSHubs   *WSHubSet
}

type WSHubSet struct {
	Workspaces *wshub.Hub
	Git        *wshub.Hub
	Files      *wshub.Hub
	Chat       *wshub.Hub
	Terminal   *wshub.Hub
	Daemon     *wshub.Hub
}

func New() (*Container, error) {
	store, err := fixtures.Load()
	if err != nil {
		return nil, err
	}
	return &Container{
		Hub:      hub.New(),
		Health:   usecases.NewHealth(),
		Fixtures: store,
		WSHubs: &WSHubSet{
			Workspaces: wshub.New(),
			Git:        wshub.New(),
			Files:      wshub.New(),
			Chat:       wshub.New(),
			Terminal:   wshub.New(),
			Daemon:     wshub.New(),
		},
	}, nil
}
```

- [ ] **Step 2: Register all handlers in router**

Replace the contents of `api/internal/api/v0/router.go`:

```go
package v0

import (
	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/app"
)

func Register(rg *gin.RouterGroup, c *app.Container) {
	store := c.Fixtures
	hubs := c.WSHubs

	// Existing
	rg.GET("/health", NewHealthHandler(c.Health).Check)
	rg.GET("/events", NewEventsHandler(c.Hub).Stream)

	// REST — workspaces
	ws := NewWorkspacesHandler(store)
	rg.GET("/workspaces/:id", ws.Get)
	rg.POST("/workspaces", ws.Create)

	// REST — flows
	rg.GET("/flows", NewFlowsHandler(store).List)

	// REST — conversations
	rg.GET("/conversations/:wsId/:step", NewConversationsHandler(store).Get)

	// REST — projects
	proj := NewProjectsHandler(store)
	rg.GET("/projects", proj.List)
	rg.POST("/projects", proj.Create)

	// REST — fs + git
	rg.GET("/fs/tree", NewFsHandler(store).Tree)
	git := NewGitHandler(store)
	rg.GET("/git/status", git.Status)
	rg.GET("/git/log", git.Log)
	rg.GET("/git/branches", git.Branches)

	// REST — terminal sessions
	rg.POST("/terminal/sessions", NewTerminalHandler().CreateSession)

	// WS channels
	rg.GET("/ws/workspaces", NewWSWorkspacesHandler(hubs.Workspaces, store).Upgrade)
	rg.GET("/ws/git", NewWSGitHandler(hubs.Git, store).Upgrade)
	rg.GET("/ws/files", NewWSFilesHandler(hubs.Files, store).Upgrade)
	rg.GET("/ws/chat/:chatId", NewWSChatHandler(hubs.Chat).Upgrade)
	rg.GET("/ws/terminal/:sessionId", NewWSTerminalHandler(hubs.Terminal).Upgrade)
	rg.GET("/ws/daemon", NewWSDaemonHandler(hubs.Daemon).Upgrade)
}
```

- [ ] **Step 3: Build and verify**

```bash
cd api && go build ./...
```

Expected: no errors.

- [ ] **Step 4: Smoke test the server**

```bash
cd api && go run ./cmd/crowbar serve &
sleep 1
curl -s http://localhost:8080/api/v0/health
curl -s http://localhost:8080/api/v0/flows | head -c 200
curl -s http://localhost:8080/api/v0/git/branches | head -c 200
kill %1
```

Expected: health returns `{"status":"ok"}`, flows returns JSON array, branches returns JSON array.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/container.go api/internal/api/v0/router.go
git commit -m "feat(api): wire all REST and WS handlers into router"
```

---

## Frontend Track (independent — run in parallel with Go tasks)

---

### Task 8: MSW install + REST handlers

**Files:**
- Create: `web/src/mocks/browser.ts`
- Create: `web/src/mocks/handlers/workspaces.ts`
- Create: `web/src/mocks/handlers/flows.ts`
- Create: `web/src/mocks/handlers/conversations.ts`
- Create: `web/src/mocks/handlers/projects.ts`
- Create: `web/src/mocks/handlers/git.ts`
- Create: `web/src/mocks/handlers/fs.ts`
- Create: `web/src/mocks/handlers/index.ts`

- [ ] **Step 1: Install MSW**

```bash
cd web && npm install --save-dev msw@^2
npx msw init public/ --save
```

Expected: `public/mockServiceWorker.js` created, `package.json` updated with `"msw": { "workerDirectory": ["public"] }`.

- [ ] **Step 2: Create browser.ts**

Create `web/src/mocks/browser.ts`:

```ts
import { setupWorker } from 'msw/browser'
import { handlers } from './handlers'

export const worker = setupWorker(...handlers)
```

- [ ] **Step 3: Create workspaces handler**

Create `web/src/mocks/handlers/workspaces.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { getMockWorkspace, createMockWorkspace } from '@/lib/mock/workspaces'

export const workspaceHandlers = [
  http.get('/api/v0/workspaces/:id', ({ params }) => {
    const ws = getMockWorkspace(params.id as string)
    if (!ws) return HttpResponse.json({ error: 'not found' }, { status: 404 })
    return HttpResponse.json(ws)
  }),

  http.post('/api/v0/workspaces', async ({ request }) => {
    const body = await request.json() as { repoId: string; branch: string; flowName: string }
    const ws = createMockWorkspace(body.repoId, body.branch, body.flowName)
    return HttpResponse.json(ws, { status: 201 })
  }),
]
```

- [ ] **Step 4: Create flows handler**

Create `web/src/mocks/handlers/flows.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { MOCK_FLOWS } from '@/lib/mock/flows'

export const flowHandlers = [
  http.get('/api/v0/flows', () => HttpResponse.json(MOCK_FLOWS)),
]
```

- [ ] **Step 5: Create conversations handler**

Create `web/src/mocks/handlers/conversations.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { getMockConversation } from '@/lib/mock/conversations'

export const conversationHandlers = [
  http.get('/api/v0/conversations/:wsId/:step', ({ params }) =>
    HttpResponse.json({ messages: getMockConversation(params.wsId as string, params.step as string) })
  ),
]
```

- [ ] **Step 6: Create projects handler**

Create `web/src/mocks/handlers/projects.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { getAllMockProjects, createMockProject } from '@/lib/mock/projects'

export const projectHandlers = [
  http.get('/api/v0/projects', () => HttpResponse.json(getAllMockProjects())),

  http.post('/api/v0/projects', async ({ request }) => {
    const body = await request.json() as { name: string; path: string }
    return HttpResponse.json(createMockProject(body), { status: 201 })
  }),
]
```

- [ ] **Step 7: Create git handler**

Create `web/src/mocks/handlers/git.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { getMockGitStatus, getMockCommitHistory, getMockBranches } from '@/lib/mock/git-data'

export const gitHandlers = [
  http.get('/api/v0/git/status',   ({ request }) => {
    const repo = new URL(request.url).searchParams.get('repo') ?? ''
    return HttpResponse.json(getMockGitStatus(repo))
  }),
  http.get('/api/v0/git/log',      ({ request }) => {
    const repo = new URL(request.url).searchParams.get('repo') ?? ''
    return HttpResponse.json(getMockCommitHistory(repo))
  }),
  http.get('/api/v0/git/branches', ({ request }) => {
    const repo = new URL(request.url).searchParams.get('repo') ?? ''
    return HttpResponse.json(getMockBranches(repo))
  }),
]
```

- [ ] **Step 8: Create fs handler**

Create `web/src/mocks/handlers/fs.ts`:

```ts
import { http, HttpResponse } from 'msw'
import { getMockFileTree } from '@/lib/mock/files'

export const fsHandlers = [
  http.get('/api/v0/fs/tree', ({ request }) => {
    const root = new URL(request.url).searchParams.get('root') ?? ''
    return HttpResponse.json(getMockFileTree(root))
  }),
]
```

- [ ] **Step 9: Create terminal session handler**

Create `web/src/mocks/handlers/terminal.ts`:

```ts
import { http, HttpResponse } from 'msw'

export const terminalHandlers = [
  http.post('/api/v0/terminal/sessions', () =>
    HttpResponse.json({ sessionId: crypto.randomUUID() }, { status: 201 })
  ),
]
```

- [ ] **Step 10: Create handlers index**

Create `web/src/mocks/handlers/index.ts`:

```ts
import { workspaceHandlers } from './workspaces'
import { flowHandlers } from './flows'
import { conversationHandlers } from './conversations'
import { projectHandlers } from './projects'
import { gitHandlers } from './git'
import { fsHandlers } from './fs'
import { terminalHandlers } from './terminal'

export const handlers = [
  ...workspaceHandlers,
  ...flowHandlers,
  ...conversationHandlers,
  ...projectHandlers,
  ...gitHandlers,
  ...fsHandlers,
  ...terminalHandlers,
]
```

- [ ] **Step 11: Commit**

```bash
cd web && git add src/mocks/ public/mockServiceWorker.js package.json
git commit -m "feat(web): add MSW v2 with REST mock handlers"
```

---

### Task 9: MSW WebSocket handlers

**Files:**
- Create: `web/src/mocks/handlers/ws/git.ts`
- Create: `web/src/mocks/handlers/ws/chat.ts`
- Create: `web/src/mocks/handlers/ws/terminal.ts`
- Modify: `web/src/mocks/handlers/index.ts`

- [ ] **Step 1: Create git WS handler**

Create `web/src/mocks/handlers/ws/git.ts`:

```ts
import { ws } from 'msw'

export const gitWsHandler = ws.link('/api/v0/ws/git').addEventListener('connection', ({ client }) => {
  client.send(JSON.stringify({ repo: 'mock', changed: false }))

  const interval = setInterval(() => {
    client.send(JSON.stringify({ repo: 'mock', changed: true }))
  }, 8000)

  client.addEventListener('close', () => clearInterval(interval))
})
```

- [ ] **Step 2: Create chat WS handler**

Create `web/src/mocks/handlers/ws/chat.ts`:

```ts
import { ws } from 'msw'

export const chatWsHandler = ws.link('/api/v0/ws/chat/:chatId').addEventListener('connection', ({ client, params }) => {
  const chatId = params.chatId as string
  const words = ['Hello', ' from', ' the', ' mock', ' AI', ' server', '!']
  let i = 0
  const interval = setInterval(() => {
    if (i < words.length) {
      client.send(JSON.stringify({ chatId, content: words[i], done: false }))
      i++
    } else {
      client.send(JSON.stringify({ chatId, content: '', done: true }))
      clearInterval(interval)
    }
  }, 150)

  client.addEventListener('close', () => clearInterval(interval))
})
```

- [ ] **Step 3: Create terminal WS handler**

Create `web/src/mocks/handlers/ws/terminal.ts`:

```ts
import { ws } from 'msw'

export const terminalWsHandler = ws.link('/api/v0/ws/terminal/:sessionId').addEventListener('connection', ({ client, params }) => {
  const sessionId = params.sessionId as string
  client.send(JSON.stringify({ sessionId, data: 'crowbar mock terminal ready\r\n$ ', isInput: false }))

  client.addEventListener('message', ({ data }) => {
    const frame = typeof data === 'string' ? JSON.parse(data) : data
    client.send(JSON.stringify({ sessionId, data: frame.data, isInput: false }))
  })
})
```

- [ ] **Step 4: Add WS handlers to index**

Edit `web/src/mocks/handlers/index.ts` — append to imports and export:

```ts
import { gitWsHandler } from './ws/git'
import { chatWsHandler } from './ws/chat'
import { terminalWsHandler } from './ws/terminal'

export const handlers = [
  ...workspaceHandlers,
  ...flowHandlers,
  ...conversationHandlers,
  ...projectHandlers,
  ...gitHandlers,
  ...fsHandlers,
  ...terminalHandlers,
  gitWsHandler,
  chatWsHandler,
  terminalWsHandler,
]
```

- [ ] **Step 5: Commit**

```bash
git add web/src/mocks/handlers/ws/
git commit -m "feat(web): add MSW WebSocket handlers for git, chat, terminal channels"
```

---

### Task 10: Remove IS_MOCK — wire apiFetch

**Files:**
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/lib/queries.ts`

- [ ] **Step 1: Rewrite api.ts**

Replace the full contents of `web/src/lib/api.ts`:

```ts
import type { WorkspacePayload, FlowDefinition, ChatMessage, Project } from './types'

const crowbar = (window as any).__CROWBAR__
export const API_BASE: string = crowbar?.api ?? import.meta.env.VITE_API_URL ?? ''

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, init)
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json() as Promise<T>
}

export function fetchWorkspace(wsId: string): Promise<WorkspacePayload> {
  return apiFetch(`/api/v0/workspaces/${wsId}`)
}

export function postWorkspace(repoId: string, branch: string, flowName: string): Promise<WorkspacePayload> {
  return apiFetch('/api/v0/workspaces', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ repoId, branch, flowName }),
  })
}

export function fetchFlows(): Promise<FlowDefinition[]> {
  return apiFetch('/api/v0/flows')
}

export function fetchConversation(wsId: string, step: string): Promise<{ messages: ChatMessage[] }> {
  return apiFetch(`/api/v0/conversations/${wsId}/${step}`)
}

export function fetchProjects(): Promise<Project[]> {
  return apiFetch('/api/v0/projects')
}

export function postProject(name: string, path: string): Promise<Project> {
  return apiFetch('/api/v0/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, path }),
  })
}
```

- [ ] **Step 2: Rewrite queries.ts**

Replace the full contents of `web/src/lib/queries.ts`:

```ts
import { queryOptions } from '@tanstack/react-query'
import { fetchWorkspace, fetchFlows, fetchConversation, apiFetch } from './api'
import type { GitStatus, Commit, Branch } from '@/lib/mock/git-data'
import type { FileNode } from '@/lib/mock/files'

export const workspaceQueryOptions = (wsId: string) =>
  queryOptions({
    queryKey: ['workspace', wsId],
    queryFn: () => fetchWorkspace(wsId),
  })

export const flowsQueryOptions = () =>
  queryOptions({
    queryKey: ['flows'],
    queryFn: fetchFlows,
  })

export const conversationQueryOptions = (wsId: string, step: string) =>
  queryOptions({
    queryKey: ['conversation', wsId, step],
    queryFn: () => fetchConversation(wsId, step),
  })

export const fileTreeQueryOptions = (rootPath: string) =>
  queryOptions({
    queryKey: ['file-tree', rootPath] as const,
    queryFn: () => apiFetch<FileNode>(`/api/v0/fs/tree?root=${encodeURIComponent(rootPath)}`),
  })

export const gitStatusQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-status', repoPath] as const,
    queryFn: () => apiFetch<GitStatus>(`/api/v0/git/status?repo=${encodeURIComponent(repoPath)}`),
  })

export const gitHistoryQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-history', repoPath] as const,
    queryFn: () => apiFetch<Commit[]>(`/api/v0/git/log?repo=${encodeURIComponent(repoPath)}`),
  })

export const gitBranchesQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-branches', repoPath] as const,
    queryFn: () => apiFetch<Branch[]>(`/api/v0/git/branches?repo=${encodeURIComponent(repoPath)}`),
  })
```

- [ ] **Step 3: Verify TypeScript**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors. If FileNode or GitStatus types are not exported from mock files, export them (add `export` keyword to the interface definitions in `web/src/lib/mock/files.ts` and `web/src/lib/mock/git-data.ts` — they're already exported per the file content).

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/api.ts web/src/lib/queries.ts
git commit -m "feat(web): remove IS_MOCK — all queries use apiFetch, MSW handles dev interception"
```

---

### Task 11: WS types + manager

**Files:**
- Create: `web/src/lib/ws/types.ts`
- Create: `web/src/lib/ws/manager.ts`
- Create: `web/src/__tests__/lib/ws/manager.test.ts`

- [ ] **Step 1: Write failing test**

Create `web/src/__tests__/lib/ws/manager.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'

// We test the manager's ref-counting behaviour using a mock WebSocket
class MockWebSocket {
  static instances: MockWebSocket[] = []
  onmessage: ((e: MessageEvent) => void) | null = null
  onclose: (() => void) | null = null
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  readyState = WebSocket.OPEN
  send = vi.fn()
  close = vi.fn()
  constructor(public url: string) { MockWebSocket.instances.push(this) }
  simulateMessage(data: string) { this.onmessage?.({ data } as MessageEvent) }
  simulateOpen() { this.onopen?.() }
}

beforeEach(() => { MockWebSocket.instances = [] })

vi.stubGlobal('WebSocket', MockWebSocket)

// Dynamic import so the global stub is in place before module loads
const { createWSManager } = await import('@/lib/ws/manager')

describe('WSManager', () => {
  it('opens one socket for two subscribers to the same endpoint', () => {
    const mgr = createWSManager()
    const cb1 = vi.fn()
    const cb2 = vi.fn()
    mgr.subscribe('/api/v0/ws/git', cb1)
    mgr.subscribe('/api/v0/ws/git', cb2)
    expect(MockWebSocket.instances).toHaveLength(1)
  })

  it('calls all subscribers when a message arrives', () => {
    const mgr = createWSManager()
    const cb1 = vi.fn()
    const cb2 = vi.fn()
    mgr.subscribe('/api/v0/ws/git', cb1)
    mgr.subscribe('/api/v0/ws/git', cb2)
    MockWebSocket.instances[0].simulateMessage('{"changed":true}')
    expect(cb1).toHaveBeenCalledWith({ changed: true })
    expect(cb2).toHaveBeenCalledWith({ changed: true })
  })

  it('closes socket when last subscriber unsubscribes', () => {
    const mgr = createWSManager()
    const unsub1 = mgr.subscribe('/api/v0/ws/git', vi.fn())
    const unsub2 = mgr.subscribe('/api/v0/ws/git', vi.fn())
    unsub1()
    expect(MockWebSocket.instances[0].close).not.toHaveBeenCalled()
    unsub2()
    expect(MockWebSocket.instances[0].close).toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run — confirm fail**

```bash
cd web && npx vitest run src/__tests__/lib/ws/manager.test.ts
```

Expected: FAIL — cannot find module `@/lib/ws/manager`.

- [ ] **Step 3: Create types.ts**

Create `web/src/lib/ws/types.ts`:

```ts
export interface WorkspaceEvent { workspaceId: string; action: string }
export interface GitEvent { repo: string; changed: boolean }
export interface FileEvent { workspaceId: string; path: string }
export interface ChatChunk { chatId: string; content: string; done: boolean }
export interface TerminalFrame { sessionId: string; data: string; isInput: boolean }
export interface DaemonStatus { status: string; version?: string }
```

- [ ] **Step 4: Create manager.ts**

Create `web/src/lib/ws/manager.ts`:

```ts
type Callback = (data: unknown) => void

interface Channel {
  socket: WebSocket
  callbacks: Set<Callback>
  reconnectDelay: number
  endpoint: string
}

export interface WSManager {
  subscribe(endpoint: string, cb: Callback): () => void
  send(endpoint: string, data: unknown): void
}

export function createWSManager(): WSManager {
  const channels = new Map<string, Channel>()

  function open(endpoint: string): Channel {
    const ch: Channel = {
      socket: new WebSocket(endpoint),
      callbacks: new Set(),
      reconnectDelay: 1000,
      endpoint,
    }

    ch.socket.onmessage = (e) => {
      let parsed: unknown
      try { parsed = JSON.parse(e.data as string) } catch { parsed = e.data }
      ch.callbacks.forEach(cb => cb(parsed))
    }

    ch.socket.onclose = () => {
      if (ch.callbacks.size === 0) return
      setTimeout(() => {
        const fresh = open(endpoint)
        fresh.callbacks = ch.callbacks
        channels.set(endpoint, fresh)
        ch.callbacks.forEach(cb => cb({ reconnected: true }))
      }, ch.reconnectDelay)
      ch.reconnectDelay = Math.min(ch.reconnectDelay * 2, 30_000)
    }

    channels.set(endpoint, ch)
    return ch
  }

  return {
    subscribe(endpoint, cb) {
      const ch = channels.get(endpoint) ?? open(endpoint)
      ch.callbacks.add(cb)
      return () => {
        ch.callbacks.delete(cb)
        if (ch.callbacks.size === 0) {
          ch.socket.close()
          channels.delete(endpoint)
        }
      }
    },

    send(endpoint, data) {
      const ch = channels.get(endpoint)
      if (ch?.socket.readyState === WebSocket.OPEN) {
        ch.socket.send(JSON.stringify(data))
      }
    },
  }
}

export const wsManager = createWSManager()
```

- [ ] **Step 5: Run tests — confirm pass**

```bash
cd web && npx vitest run src/__tests__/lib/ws/manager.test.ts
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/ws/ web/src/__tests__/lib/ws/
git commit -m "feat(web): add typed WS channel manager with ref-counting"
```

---

### Task 12: IDB query-cache store + TanStack persist client

**Files:**
- Modify: `web/src/lib/persistence/schemas.ts`
- Modify: `web/src/lib/persistence/idb.ts`
- Create: `web/src/lib/persistence/query-persister.ts`

- [ ] **Step 1: Install packages**

```bash
cd web && npm install @tanstack/react-query-persist-client @tanstack/query-async-storage-persister
```

Expected: packages added to node_modules and package.json.

- [ ] **Step 2: Add query-cache to schemas.ts**

Edit `web/src/lib/persistence/schemas.ts` — add to `CrowbarDB` interface:

```ts
'query-cache': {
  key: string
  value: string
}
```

Full updated `CrowbarDB`:

```ts
export interface CrowbarDB extends DBSchema {
  'workspace-layout': { key: string; value: WorkspaceLayout }
  'editor-state': { key: [string, string]; value: EditorState; indexes: { workspaceId: string } }
  'ui-preferences': { key: string; value: UIPreferences }
  'query-cache': { key: string; value: string }
}
```

- [ ] **Step 3: Add store to idb.ts**

Edit `web/src/lib/persistence/idb.ts` — inside the `if (oldVersion < 1)` block, add one line:

```ts
db.createObjectStore('query-cache')
```

The block should now look like:

```ts
if (oldVersion < 1) {
  db.createObjectStore('workspace-layout', { keyPath: 'workspaceId' })
  const editorStore = db.createObjectStore('editor-state', {
    keyPath: ['workspaceId', 'bufferId'],
  })
  editorStore.createIndex('workspaceId', 'workspaceId')
  db.createObjectStore('ui-preferences')
  db.createObjectStore('query-cache')
}
```

- [ ] **Step 4: Create query-persister.ts**

Create `web/src/lib/persistence/query-persister.ts`:

```ts
import { createAsyncStoragePersister } from '@tanstack/query-async-storage-persister'
import { getDB } from './idb'

const idbAsyncStorage = {
  getItem: async (key: string): Promise<string | null> =>
    (await getDB()).get('query-cache', key) ?? null,

  setItem: async (key: string, value: string): Promise<void> => {
    await (await getDB()).put('query-cache', value, key)
  },

  removeItem: async (key: string): Promise<void> => {
    await (await getDB()).delete('query-cache', key)
  },
}

export const persister = createAsyncStoragePersister({
  storage: idbAsyncStorage,
  key: 'crowbar-query-cache',
})
```

- [ ] **Step 5: Verify TypeScript**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/persistence/
git commit -m "feat(web): add IDB query-cache store and TanStack async persister"
```

---

### Task 13: Vite git SHA + PersistQueryClientProvider

**Files:**
- Modify: `web/vite.config.ts`
- Modify: `web/src/vite-env.d.ts` (or create if missing)
- Modify: `web/src/main.tsx`

- [ ] **Step 1: Inject git SHA in vite.config.ts**

Edit `web/vite.config.ts` — add import and define at the top:

```ts
import { execSync } from 'child_process'

const gitSHA = (() => {
  try { return execSync('git rev-parse --short HEAD').toString().trim() }
  catch { return 'dev' }
})()
```

Add `define` inside `defineConfig({...})`:

```ts
define: {
  __APP_VERSION__: JSON.stringify(gitSHA),
},
```

- [ ] **Step 2: Declare the global type**

Check if `web/src/vite-env.d.ts` exists. If yes, append; if no, create it:

```ts
/// <reference types="vite/client" />

declare const __APP_VERSION__: string
```

- [ ] **Step 3: Swap QueryClientProvider in main.tsx**

Edit `web/src/main.tsx`:

1. Add imports:

```ts
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client'
import { persister } from '@/lib/persistence/query-persister'
```

2. Remove the existing `import { QueryClientProvider } from '@tanstack/react-query'` line.

3. Replace `<QueryClientProvider client={queryClient}>` with:

```tsx
<PersistQueryClientProvider
  client={queryClient}
  persistOptions={{
    persister,
    maxAge: 7 * 24 * 60 * 60 * 1000,
    buster: __APP_VERSION__,
  }}
>
```

4. Replace `</QueryClientProvider>` with `</PersistQueryClientProvider>`.

5. Add MSW startup before `createRoot(...)`:

```ts
if (import.meta.env.VITE_USE_MOCK === 'true') {
  const { worker } = await import('./mocks/browser')
  await worker.start({ onUnhandledRequest: 'warn' })
}
```

Note: wrapping the `createRoot` call in an async IIFE is required for the top-level await. Wrap everything from the MSW block to the end of the file:

```ts
async function main() {
  if (import.meta.env.VITE_USE_MOCK === 'true') {
    const { worker } = await import('./mocks/browser')
    await worker.start({ onUnhandledRequest: 'warn' })
  }

  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <PersistQueryClientProvider
        client={queryClient}
        persistOptions={{ persister, maxAge: 7 * 24 * 60 * 60 * 1000, buster: __APP_VERSION__ }}
      >
        <TooltipProvider>
          <RouterProvider router={router} />
        </TooltipProvider>
      </PersistQueryClientProvider>
    </StrictMode>,
  )
}

void main()
```

- [ ] **Step 4: Verify TypeScript and build**

```bash
cd web && npx tsc --noEmit && npx vite build --mode development 2>&1 | tail -5
```

Expected: no TypeScript errors, build succeeds.

- [ ] **Step 5: Commit**

```bash
git add web/vite.config.ts web/src/vite-env.d.ts web/src/main.tsx
git commit -m "feat(web): add git SHA cache buster and PersistQueryClientProvider"
```

---

### Task 14: Chaos store + dev panel

**Files:**
- Create: `web/src/lib/store/chaos.ts`
- Create: `web/src/components/dev/chaos-panel.tsx`
- Modify: `web/src/lib/api.ts` (read chaos headers)
- Modify: `web/src/main.tsx` (render panel)

- [ ] **Step 1: Create chaos store**

Create `web/src/lib/store/chaos.ts`:

```ts
import { create } from 'zustand'

interface ChaosState {
  latency: number
  errorRate: number
  setLatency: (ms: number) => void
  setErrorRate: (rate: number) => void
  reset: () => void
}

export const useChaosStore = create<ChaosState>()((set) => ({
  latency: 0,
  errorRate: 0,
  setLatency: (latency) => set({ latency }),
  setErrorRate: (errorRate) => set({ errorRate }),
  reset: () => set({ latency: 0, errorRate: 0 }),
}))
```

- [ ] **Step 2: Update apiFetch to send chaos headers**

Edit `web/src/lib/api.ts` — update `apiFetch`:

```ts
import { useChaosStore } from '@/lib/store/chaos'

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const { latency, errorRate } = useChaosStore.getState()
  const chaosHeaders: Record<string, string> = {}
  if (latency > 0) chaosHeaders['X-Crowbar-Latency'] = String(latency)
  if (errorRate > 0) chaosHeaders['X-Crowbar-Error-Rate'] = String(errorRate)

  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: { ...init?.headers, ...chaosHeaders },
  })
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json() as Promise<T>
}
```

- [ ] **Step 3: Create chaos panel component**

Create `web/src/components/dev/chaos-panel.tsx`:

```tsx
import { useChaosStore } from '@/lib/store/chaos'

export function ChaosPanel() {
  const { latency, errorRate, setLatency, setErrorRate, reset } = useChaosStore((s) => ({
    latency: s.latency,
    errorRate: s.errorRate,
    setLatency: s.setLatency,
    setErrorRate: s.setErrorRate,
    reset: s.reset,
  }))

  return (
    <div className="fixed bottom-4 right-4 z-50 w-56 rounded-lg border border-border bg-background p-3 shadow-lg text-xs font-mono">
      <div className="mb-2 font-bold text-foreground">Chaos Panel</div>

      <label className="block mb-1 text-muted-foreground">
        Latency: {latency}ms
      </label>
      <input
        type="range"
        min={0}
        max={5000}
        step={100}
        value={latency}
        onChange={(e) => setLatency(Number(e.target.value))}
        className="w-full mb-2"
      />

      <label className="block mb-1 text-muted-foreground">
        Error rate: {(errorRate * 100).toFixed(0)}%
      </label>
      <input
        type="range"
        min={0}
        max={1}
        step={0.05}
        value={errorRate}
        onChange={(e) => setErrorRate(Number(e.target.value))}
        className="w-full mb-2"
      />

      <button
        onClick={reset}
        className="w-full rounded border border-border px-2 py-1 text-muted-foreground hover:bg-accent"
      >
        Reset
      </button>
    </div>
  )
}
```

- [ ] **Step 4: Render panel in main.tsx**

Edit `web/src/main.tsx` — add import and render conditionally:

```ts
import { ChaosPanel } from '@/components/dev/chaos-panel'
```

Inside the JSX tree, after `<RouterProvider>`:

```tsx
{import.meta.env.DEV && import.meta.env.VITE_USE_MOCK !== 'true' && <ChaosPanel />}
```

- [ ] **Step 5: Verify TypeScript**

```bash
cd web && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/store/chaos.ts web/src/components/dev/chaos-panel.tsx \
        web/src/lib/api.ts web/src/main.tsx
git commit -m "feat(web): add chaos dev panel and X-Crowbar-Latency/Error-Rate headers"
```

---

### Task 15: End-to-end smoke test

- [ ] **Step 1: Start Go server**

```bash
cd api && go run ./cmd/crowbar serve
```

Expected: server listening on :8080.

- [ ] **Step 2: Start frontend against Go server**

In a separate terminal:

```bash
cd web && VITE_API_URL=http://localhost:8080 npm run dev
```

Open http://localhost:5173 in a browser.

- [ ] **Step 3: Verify REST endpoints load**

- Navigate to a workspace route — workspace data should load (not 404)
- Open the git panel — branches and status should appear from the 50-branch fixture
- Open the file explorer — the 5000-node file tree should render (may need virtual scrolling)

- [ ] **Step 4: Verify WebSocket channels**

Open browser DevTools → Network → WS. Confirm:
- `/api/v0/ws/git` connection established, receives `{"repo":"mock","changed":false}`
- `/api/v0/ws/daemon` connection established, receives `{"status":"running","version":"0.1.0-mock"}`

- [ ] **Step 5: Test chaos panel**

With the Go server running and `VITE_API_URL` set:
- Chaos panel appears bottom-right
- Set latency to 2000ms — workspace fetch takes ~2s
- Set error rate to 1.0 — all fetches return 500

- [ ] **Step 6: Test mock mode**

```bash
cd web && VITE_USE_MOCK=true npm run dev
```

- Chaos panel should NOT appear
- All data loads from MSW mock handlers
- Network tab shows service worker intercepting requests

- [ ] **Step 7: Test cache persistence**

With `VITE_API_URL` set, reload the app. Open DevTools → Application → IndexedDB → crowbar → query-cache. Confirm entries are persisted. Reload — data should appear instantly before WS events fire.

- [ ] **Step 8: Final commit**

```bash
git add -A
git commit -m "feat: complete API layer, WS channels, MSW, and query cache persistence"
```
