# Wave 3B — Branch Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax. **Invoke the `go-style` skill before writing any Go.**
>
> **DEPENDS ON Plan 3A** (`2026-06-05-wave3a-app-core.md`) — the `ReviewThread` aggregate (full `OpenThread`/`ReplyThread` commands + read model + facade `Open`/`Reply`/`List`/`ListByWorkspace`), the `Workspace` repo (`Get`, `SetMergeStrategy`), and the `Chat` read model (`ListByWorkspace`) must already exist. Can run **alongside/after 3A**, independently of 3D.

**Goal:** Build the Branch Review surface — the `BranchReview` composite read-model usecase (`GET …/review`), the merge-strategy `PATCH`, and the review/thread REST handlers — assembling description (placeholder, bridge-deferred), `mergeStrategy` (from the Workspace aggregate), the branch diff (`git diff <base>...<branch>`), threads (ReviewThread aggregate), and `BranchChat[]` (Chat projection). Re-fetch on mutation; **no PR-create endpoint, no dedicated broadcaster.**

**Architecture:** A read-only composite usecase fans out to four sources (git engine diff, ReviewThread read model, Chat read model, Workspace row) and assembles a `BranchReview` DTO. Thread CRUD flows through the 3A `ReviewThread` aggregate. The REST handlers live in `internal/api/v0/` following the existing `provider_handlers.go`/`search_handlers.go` pattern. No new WS topic (09 §7).

**Tech Stack:** Go 1.26.2, gin, the git engine (`Diff`/three-dot), the ReviewThread + Chat + Workspace repos, testify. Module `github.com/char2cs/crowbar/api`.

---

## ⛔ Rabbyte standards gate (reviewer checks EACH)

Same seven rules as 3A. For this plan: handlers stay thin (delegate to the usecase); **one param per line**; **early returns** (404/400/500 guard clauses first); **no `time.Sleep`** (handler tests use `httptest` + the real in-memory stack); coverage **≥95%** including every error branch; benchmark the composite assembly (it fans out to git + two read models).

**Verification after every task:** `cd api && gofumpt -l -w . && goimports -w . && go build ./... && go vet ./... && go test ./internal/app/usecases/... ./internal/api/v0/...`

---

## Reference — what exists

- `internal/engine/git/engine.go`: `Diff(ctx, repoPath, staged)` (working tree), `CommitDiff(ctx, repoPath, sha)`. **Missing:** a three-dot range diff `git diff <base>...<branch>`. Add a `RangeDiff` primitive (Task 1).
- `gitdomain.MultiFileDiff` / `FileDiff` in `internal/domain/git/` (the diff shape `04` §3 — verify exact type names).
- 3A `reviewthread.ReviewThread`: `Open(ctx, OpenInput, now)`, `Reply(ctx, id, messageID, body, now)`, `Resolve`, `Reopen`, `List`, `ListByWorkspace`, `Get`.
- 3A `chat.Chat`: `ListByWorkspace(ctx, wsID)` returning `[]domain.Chat` (with `Status`, `Title`, `CreatedAt`).
- 3A `workspace.Workspace`: `Get`, `SetMergeStrategy`.
- `repositories.Container` and `usecases.Container` (extend).
- API handler registration pattern: `internal/api/v0/container.go` `Register` + `registerXHandlers(rg, c)`; handlers read `c.app.Usecases` / `c.app.Repositories`.

---

## File Structure

All under `api/`:

**Git primitive:**
- `internal/engine/git/range_diff.go` (+ test) — `RangeDiff(ctx, repoPath, base, branch)`.
- Extend `internal/engine/git/git.go` interface.

**Domain DTO (struct-only, no `_test.go`):**
- `internal/domain/branch_review.go` — `BranchReview`, `BranchChat`.

**Usecase:**
- `internal/app/usecases/branch_review.go` (+ test + bench) — `BranchReviewUsecase`: `Get`, `SetMergeStrategy`, `OpenThread`, `Reply`, `SetThreadResolved`.
- `internal/app/usecases/internal/branchchat/branchchat.go` (+ test) — `domain.Chat` → `BranchChat` projection (age + isActive).
- Modify `internal/app/usecases/container.go` — add `BranchReview`.

**API handlers:**
- `internal/api/v0/review_handlers.go` (+ test) — the five routes.
- Modify `internal/api/v0/container.go` — `registerReviewHandlers(rg, c)`.

---

## Phase 0 — Git range-diff primitive

### Task 1: `RangeDiff` (three-dot `git diff base...branch`)

**Files:**
- Create: `internal/engine/git/range_diff.go` (+ `range_diff_test.go`)
- Modify: `internal/engine/git/git.go`

- [ ] **Step 1: Grep for an existing range/three-dot diff**

Run: `cd api && grep -rn "\.\.\.\|RangeDiff\|three-dot\|MultiFileDiff" internal/engine/git/`
If a suitable primitive exists, reuse it and skip to Task 2. Otherwise continue.

- [ ] **Step 2: Inspect the diff internal package + return type**

Run: `cd api && sed -n '1,60p' internal/engine/git/internal/diff/diff.go && grep -rn "MultiFileDiff\|func Commit\|func WorkingTree" internal/engine/git/internal/diff/`
Confirm how `CommitDiff` builds a `MultiFileDiff` so `RangeDiff` reuses the same parser (`diff.Range(ctx, repoPath, base, branch)` mirroring `diff.Commit`).

- [ ] **Step 3: Write the failing test** (real temp repo: base branch + a branch with one added file → one `FileDiff`):

```go
func TestRangeDiff_ReturnsBranchChangesVsBase(t *testing.T) {
	repo := initRepoWithCommit(t) // default branch e.g. "main"
	e := New()
	ctx := context.Background()
	// create branch feature off main, add file, commit (use gitRun helper)
	d, err := e.RangeDiff(ctx, repo, "main", "feature")
	require.NoError(t, err)
	require.NotEmpty(t, d.Files)
	assert.Equal(t, "new.txt", d.Files[0].Path) // adjust to MultiFileDiff field names
}
```

- [ ] **Step 4: Run red.**

- [ ] **Step 5: Implement `range_diff.go`**

```go
package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

func (e *engine) RangeDiff(
	ctx context.Context,
	repoPath string,
	base string,
	branch string,
) (gitdomain.MultiFileDiff, error) {
	return diff.Range(ctx, repoPath, base, branch)
}
```

Add `diff.Range` in `internal/engine/git/internal/diff/` mirroring `diff.Commit` but invoking `git diff <base>...<branch>` (three-dot). Write its own `_test.go` covering parse + the hunk-id stamping already done for `Commit` (09 §2 reuses the `MultiFileDiff` shape, hunks carry stable `hunkId` per `04` §4 — reuse the existing hunk-id logic; do not duplicate).

- [ ] **Step 6: Add the interface method** to `git.go`:

```go
	// RangeDiff returns the three-dot diff base...branch — the review diff (09 §2).
	RangeDiff(
		ctx context.Context,
		repoPath string,
		base string,
		branch string,
	) (gitdomain.MultiFileDiff, error)
```

- [ ] **Step 7: Run green. Commit.**

```bash
git add api/internal/engine/git/
git commit -m "feat(git): RangeDiff three-dot base...branch for review (09 §2)"
```

---

## Phase 1 — DTO + BranchChat projection

### Task 2: `BranchReview` + `BranchChat` DTOs

**Files:**
- Create: `internal/domain/branch_review.go`

- [ ] **Step 1: Implement the DTO (struct-only file — no test)**

```go
package domain

import gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"

// BranchReview is the composite read model for the branch-review panel (09 §2).
// It is assembled per-request, never stored.
type BranchReview struct {
	Description   string                  `json:"description"`
	MergeStrategy gitdomain.MergeStrategy  `json:"mergeStrategy"`
	Diff          gitdomain.MultiFileDiff  `json:"diff"`
	Threads       []ReviewThread          `json:"threads"`
	Conversations []BranchChat            `json:"conversations"`
}

// BranchChat is a Chat surfaced read-only in the review panel (09 §2).
type BranchChat struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Age      string `json:"age"`
	IsActive bool   `json:"isActive"`
}
```

- [ ] **Step 2: Build + commit**

Run: `cd api && go build ./internal/domain/...`
```bash
git add api/internal/domain/branch_review.go
git commit -m "feat(domain): BranchReview + BranchChat composite DTOs (09 §2)"
```

### Task 3: `BranchChat` projection (age + isActive)

**Files:**
- Create: `internal/app/usecases/internal/branchchat/branchchat.go` (+ test)

> `isActive` = the Chat's status is `agent-running` (09 §2, purely derived). `age` is a relative-time string from `createdAt`. Reuse any existing age helper — **grep first** (`grep -rn "func.*[Aa]ge\|relative" internal/`); if none, implement a small `relativeAge(now, then)`.

- [ ] **Step 1: Write failing tests**

```go
func TestFrom_MapsActiveAndAge(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	chats := []domain.Chat{
		{ID: "c1", Title: "a", Status: domain.ChatStatusAgentRunning, CreatedAt: now.Add(-90 * time.Second)},
		{ID: "c2", Title: "b", Status: domain.ChatStatusIdle, CreatedAt: now.Add(-2 * time.Hour)},
	}
	out := From(chats, now)
	require.Len(t, out, 2)
	assert.True(t, out[0].IsActive)
	assert.False(t, out[1].IsActive)
	assert.Equal(t, "c1", out[0].ID)
	assert.NotEmpty(t, out[0].Age)
}

func TestFrom_EmptyInput(t *testing.T) {
	assert.Empty(t, From(nil, time.Unix(1, 0)))
}
```

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `branchchat.go`** — `From(chats []domain.Chat, now time.Time) []domain.BranchChat`; `isActive = c.Status == domain.ChatStatusAgentRunning`; `age = relativeAge(now, c.CreatedAt)`. Pure.

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/internal/branchchat/
git commit -m "feat(usecases): BranchChat projection — age + isActive (09 §2)"
```

---

## Phase 2 — BranchReview usecase

### Task 4: `BranchReviewUsecase.Get` (composite assembly)

**Files:**
- Create: `internal/app/usecases/branch_review.go`
- Create: `internal/app/usecases/branch_review_test.go`

> `Get(ctx, wsID)` assembles: load workspace; resolve `base` = parent workspace's branch (`parentId` → `workspace.Get(parentId).Branch`) or, for a root workspace (no `parentId`), the repo's `defaultBranch` (via Repository store by `workspace.RepoID`); `diff = git.RangeDiff(repoPath, base, ws.Branch)`; `threads = reviewThreads.ListByWorkspace(wsID)`; `conversations = branchchat.From(chats.ListByWorkspace(wsID), now)`; `mergeStrategy = ws.MergeStrategy`; `description = ""` (bridge-deferred, 09 §5). `repoPath` = the workspace's `WorktreePath` (the review diff runs in the branch's worktree).

- [ ] **Step 1: Write failing unit tests** (mock workspace repo, reviewthread repo, chat repo, git engine, repository store):

```go
func TestBranchReview_Get_RootUsesDefaultBranch(t *testing.T) {
	// ws has no parentId; repo.defaultBranch = "main"
	// expect git.RangeDiff(ws.WorktreePath, "main", ws.Branch)
	// assert review.MergeStrategy == ws.MergeStrategy, threads + conversations wired
}

func TestBranchReview_Get_ChildUsesParentBranch(t *testing.T) {
	// ws.parentId = "p"; parent.Branch = "develop"
	// expect git.RangeDiff(ws.WorktreePath, "develop", ws.Branch)
}

func TestBranchReview_Get_WorkspaceNotFound(t *testing.T) {
	// workspace.Get errors -> usecase returns wrapped error
}
```

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `branch_review.go`** — interface + impl, decomposed (`resolveBase` is its own method ≤2 indent):

```go
package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/branchchat"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// BranchReviewUsecase assembles and mutates the branch-review surface (09).
type BranchReviewUsecase interface {
	Get(
		ctx context.Context,
		wsID string,
	) (domain.BranchReview, error)
	SetMergeStrategy(
		ctx context.Context,
		wsID string,
		strategy gitdomain.MergeStrategy,
	) error
	OpenThread(
		ctx context.Context,
		in OpenThreadInput,
	) (domain.ReviewThread, error)
	Reply(
		ctx context.Context,
		threadID string,
		body string,
	) (domain.ReviewThread, error)
	SetThreadResolved(
		ctx context.Context,
		threadID string,
		resolved bool,
	) (domain.ReviewThread, error)
}

type branchReviewUsecase struct {
	workspaces workspace.Workspace
	threads    reviewthread.ReviewThread
	chats      chat.Chat
	repos      store.Store[domain.Repository, string]
	git        enginegit.Engine
	now        func() time.Time
}

// NewBranchReviewUsecase builds the branch-review usecase.
func NewBranchReviewUsecase(
	workspaces workspace.Workspace,
	threads reviewthread.ReviewThread,
	chats chat.Chat,
	repos store.Store[domain.Repository, string],
	git enginegit.Engine,
	now func() time.Time,
) BranchReviewUsecase {
	return &branchReviewUsecase{
		workspaces: workspaces,
		threads:    threads,
		chats:      chats,
		repos:      repos,
		git:        git,
		now:        now,
	}
}

func (u *branchReviewUsecase) Get(
	ctx context.Context,
	wsID string,
) (domain.BranchReview, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: get workspace: %w", err)
	}
	base, err := u.resolveBase(ctx, ws)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: resolve base: %w", err)
	}
	diff, err := u.git.RangeDiff(ctx, ws.WorktreePath, base, ws.Branch)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: diff: %w", err)
	}
	return u.assemble(ctx, ws, diff)
}

func (u *branchReviewUsecase) resolveBase(
	ctx context.Context,
	ws domain.Workspace,
) (string, error) {
	if ws.ParentID != "" {
		parent, err := u.workspaces.Get(ctx, ws.ParentID)
		if err != nil {
			return "", err
		}
		return parent.Branch, nil
	}
	repo, err := u.repos.FindByKey(ctx, ws.RepoID)
	if err != nil {
		return "", err
	}
	if repo == nil {
		return "", fmt.Errorf("branch review: repo %s not found", ws.RepoID)
	}
	return repo.DefaultBranch, nil
}

func (u *branchReviewUsecase) assemble(
	ctx context.Context,
	ws domain.Workspace,
	diff gitdomain.MultiFileDiff,
) (domain.BranchReview, error) {
	threads, err := u.threads.ListByWorkspace(ctx, ws.ID)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: threads: %w", err)
	}
	chats, err := u.chats.ListByWorkspace(ctx, ws.ID)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: chats: %w", err)
	}
	return domain.BranchReview{
		Description:   "",
		MergeStrategy: ws.MergeStrategy,
		Diff:          diff,
		Threads:       threads,
		Conversations: branchchat.From(chats, u.now()),
	}, nil
}
```

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/branch_review.go api/internal/app/usecases/branch_review_test.go
git commit -m "feat(usecases): BranchReview.Get composite assembly (09 §2)"
```

### Task 5: Mutations — `SetMergeStrategy`, `OpenThread`, `Reply`, `SetThreadResolved`

**Files:**
- Modify: `internal/app/usecases/branch_review.go`
- Create: `internal/app/usecases/open_thread_input.go`
- Test: append to `branch_review_test.go`

- [ ] **Step 1: Write `open_thread_input.go`**

```go
package usecases

import "github.com/char2cs/crowbar/api/internal/domain"

// OpenThreadInput carries a new review thread's anchor + first message (09 §3).
type OpenThreadInput struct {
	WsID       string
	FilePath   string
	LineNumber int
	Side       domain.ReviewSide
	Body       string
}
```

- [ ] **Step 2: Write failing tests** — `SetMergeStrategy` delegates to `workspace.SetMergeStrategy`; `OpenThread` mints thread+message IDs and calls `threads.Open`; `Reply` mints a message id and calls `threads.Reply`; `SetThreadResolved(true)`→`Resolve`, `(false)`→`Reopen`.

```go
func TestSetThreadResolved_TrueResolvesFalseReopens(t *testing.T) {
	// resolved=true -> threads.Resolve(id); resolved=false -> threads.Reopen(id)
}
func TestOpenThread_MintsIdsAndOpens(t *testing.T) {
	// asserts threads.Open called with OpenInput carrying the anchor + non-empty MessageID
}
```

- [ ] **Step 3: Run red.**

- [ ] **Step 4: Implement the four mutation methods**

```go
func (u *branchReviewUsecase) SetMergeStrategy(
	ctx context.Context,
	wsID string,
	strategy gitdomain.MergeStrategy,
) error {
	if _, err := u.workspaces.SetMergeStrategy(ctx, wsID, strategy); err != nil {
		return fmt.Errorf("branch review: set merge strategy: %w", err)
	}
	return nil
}

func (u *branchReviewUsecase) OpenThread(
	ctx context.Context,
	in OpenThreadInput,
) (domain.ReviewThread, error) {
	thread, err := u.threads.Open(ctx, reviewthread.OpenInput{
		ID:         uuid.NewString(),
		WsID:       in.WsID,
		FilePath:   in.FilePath,
		LineNumber: in.LineNumber,
		Side:       in.Side,
		MessageID:  uuid.NewString(),
		Body:       in.Body,
	}, u.now())
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("branch review: open thread: %w", err)
	}
	return thread, nil
}

func (u *branchReviewUsecase) Reply(
	ctx context.Context,
	threadID string,
	body string,
) (domain.ReviewThread, error) {
	thread, err := u.threads.Reply(ctx, threadID, uuid.NewString(), body, u.now())
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("branch review: reply: %w", err)
	}
	return thread, nil
}

func (u *branchReviewUsecase) SetThreadResolved(
	ctx context.Context,
	threadID string,
	resolved bool,
) (domain.ReviewThread, error) {
	if resolved {
		return u.resolveThread(ctx, threadID)
	}
	return u.reopenThread(ctx, threadID)
}

func (u *branchReviewUsecase) resolveThread(
	ctx context.Context,
	threadID string,
) (domain.ReviewThread, error) {
	thread, err := u.threads.Resolve(ctx, threadID)
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("branch review: resolve: %w", err)
	}
	return thread, nil
}

func (u *branchReviewUsecase) reopenThread(
	ctx context.Context,
	threadID string,
) (domain.ReviewThread, error) {
	thread, err := u.threads.Reopen(ctx, threadID)
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("branch review: reopen: %w", err)
	}
	return thread, nil
}
```

Add the `uuid` import (match the project's id minter).

- [ ] **Step 5: Run green. Commit.**

```bash
git add api/internal/app/usecases/
git commit -m "feat(usecases): BranchReview mutations — strategy + thread CRUD (09 §3,§4)"
```

### Task 6: Composite-assembly benchmark

**Files:**
- Create: `internal/app/usecases/branch_review_bench_test.go`

- [ ] **Step 1: Implement `BenchmarkBranchReview_Get`** over a real small repo + populated read models (reuse 3A test helpers or build a minimal real stack). The composite fans out to git + two read models — a hot path on every panel open.

- [ ] **Step 2: Run** `cd api && go test ./internal/app/usecases/ -bench BranchReview -benchtime 10x -run '^$'`
Expected: runs clean.

- [ ] **Step 3: Commit**

```bash
git add api/internal/app/usecases/branch_review_bench_test.go
git commit -m "test(usecases): BranchReview.Get benchmark"
```

### Task 7: Mount `BranchReview` in the usecases container

**Files:**
- Modify: `internal/app/usecases/container.go`
- Test: modify `internal/app/usecases/container_test.go`

- [ ] **Step 1: Write the failing test** — `container.BranchReview` non-nil.
- [ ] **Step 2: Run red.**
- [ ] **Step 3: Add the field + construction** (`NewBranchReviewUsecase(repos.Workspace, repos.ReviewThread, repos.Chat, gormStores.Repositories, engines.Git, time.Now)`).
- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/container.go api/internal/app/usecases/container_test.go
git commit -m "feat(usecases): mount BranchReview in container"
```

---

## Phase 3 — REST handlers

### Task 8: Review REST handlers (5 routes) + registration

**Files:**
- Create: `internal/api/v0/review_handlers.go`
- Create: `internal/api/v0/review_handlers_test.go`
- Modify: `internal/api/v0/container.go`

> Routes (02 §2.9): `GET /v0/workspaces/:wsId/review`; `PATCH /v0/workspaces/:wsId/review { mergeStrategy }`; `POST /v0/workspaces/:wsId/review/threads { filePath, lineNumber, side, body }`; `POST /v0/workspaces/:wsId/review/threads/:id/reply { body }`; `PATCH /v0/workspaces/:wsId/review/threads/:id { isResolved }`. Re-fetch on mutation: mutation handlers return the affected entity id (or the updated thread), and the frontend re-fetches `GET …/review` (09 §7) — no broadcaster. Follow the response envelope `{success,error,data}` (02 §1) — **check `internal/api/v0`’s existing envelope helper** (`grep -rn "success\|gin.H{" internal/api/v0/*.go`) and reuse it.

- [ ] **Step 1: Write failing handler tests** — spin up the real in-memory stack (mirror `terminal_handlers_test.go`/`provider_handlers_test.go` setup: build `app.Container` with `WithHomeDir(t.TempDir())` or in-memory, mount routes, use `httptest`). For each route assert status + body:
  - `GET review` for a created workspace → 200, body has `mergeStrategy`, `diff`, `threads`, `conversations`.
  - `PATCH review` `{mergeStrategy:"squash"}` → 200; subsequent `GET` shows `squash`.
  - `POST threads` → 201/200 with thread id; `GET` shows the thread with one message.
  - `POST reply` → thread now has two messages.
  - `PATCH thread {isResolved:true}` → thread resolved; `{isResolved:false}` → open.
  - Error paths: unknown `wsId` → 404; malformed body → 400.

> **No `time.Sleep`** — commands are `SendWait` (synchronous through the projection), so the read model is consistent immediately after the mutation returns. If a read races the projection, use `require.Eventually`.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `review_handlers.go`** — thin handlers delegating to `c.app.Usecases.BranchReview`. Pattern per handler (guard clauses, one concern each):

```go
package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// registerReviewHandlers mounts the branch-review routes (02 §2.9, 09).
func registerReviewHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.GET("/workspaces/:wsId/review", c.handleGetReview)
	rg.PATCH("/workspaces/:wsId/review", c.handleSetMergeStrategy)
	rg.POST("/workspaces/:wsId/review/threads", c.handleOpenThread)
	rg.POST("/workspaces/:wsId/review/threads/:id/reply", c.handleReplyThread)
	rg.PATCH("/workspaces/:wsId/review/threads/:id", c.handleSetThreadResolved)
}

func (c *Container) handleGetReview(
	ctx *gin.Context,
) {
	review, err := c.app.Usecases.BranchReview.Get(ctx.Request.Context(), ctx.Param("wsId"))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": review})
}
```

The remaining handlers bind their JSON bodies into small request structs (`mergeStrategyReq{MergeStrategy gitdomain.MergeStrategy}`, `openThreadReq{FilePath string; LineNumber int; Side domain.ReviewSide; Body string}`, `replyReq{Body string}`, `resolvedReq{IsResolved bool}`), validate (400 on `ShouldBindJSON` error), call the usecase, and return the envelope. Each handler ≤2 indent levels.

- [ ] **Step 4: Register in `container.go`** — add `registerReviewHandlers(rg, c)` to `Register`.

- [ ] **Step 5: Run green.** Run: `cd api && go test ./internal/api/v0/ -run Review`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/api/v0/
git commit -m "feat(api): branch-review REST handlers (02 §2.9, 09)"
```

---

## Phase 4 — Full verification

### Task 9: Build, vet, coverage, lint, race

- [ ] **Step 1:** `cd api && gofumpt -l -w . && goimports -w . && go build ./... && go vet ./...` → clean.
- [ ] **Step 2:** `cd api && go test -coverpkg=./internal/app/usecases/...,./internal/api/v0/...,./internal/engine/git/... -coverprofile=cover.out ./internal/app/usecases/... ./internal/api/v0/... ./internal/engine/git/... && go tool cover -func=cover.out | tail -1` → **≥95%**. Add tests for uncovered handler error branches (bad body, missing ws) and usecase error paths (repo nil, diff error).
- [ ] **Step 3:** `cd api && golangci-lint run ./internal/app/usecases/... ./internal/api/v0/... ./internal/engine/git/...` → no findings.
- [ ] **Step 4:** `cd api && go test -race ./internal/api/v0/... ./internal/app/usecases/...` → PASS.
- [ ] **Step 5: Final commit**

```bash
git add -A api/
git commit -m "test(wave3b): branch review build/vet/coverage/lint/race green"
```

---

## Self-Review checklist

- **Spec coverage (09):** composite `GET …/review` assembling description (empty placeholder, §5) + mergeStrategy + three-dot diff + threads + BranchChat ✓ Task 4; base = parent branch or repo defaultBranch ✓ `resolveBase`; `PATCH …/review` → `SetMergeStrategy` ✓ Task 5; thread CRUD via the aggregate (Open/Reply/Resolve/Reopen) ✓ Task 5; re-fetch-on-mutation, no broadcaster, no PR-create endpoint ✓ Task 8 (only the five routes; mutations return ids/threads, frontend re-fetches).
- **Deferred correctly:** AI description generation (bridge spike) — `description` is empty string, no backend generation. PR creation — absent.
- **Consumes 3A:** ReviewThread `Open`/`Reply`/`Resolve`/`Reopen`/`ListByWorkspace`, Chat `ListByWorkspace`, Workspace `Get`/`SetMergeStrategy` — all from 3A; no aggregate logic duplicated here.
- **Type consistency:** `OpenThreadInput` ↔ `reviewthread.OpenInput` field names; `BranchReview`/`BranchChat` JSON tags match UX read model.
- **No placeholders:** every handler + usecase method specified with full code or exact behavior + test assertions.

---

## Execution Handoff

Depends on **3A** (independent of **3D**). Recommended: **Subagent-Driven** execution. Sibling plans: `2026-06-05-wave3a-app-core.md` (prerequisite), `2026-06-05-wave3d-worktree-hierarchy.md`, `2026-06-05-wave3c-lsp.md`.
