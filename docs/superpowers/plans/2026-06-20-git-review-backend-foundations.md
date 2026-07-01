# Git Review Backend Foundations — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the small backend pieces the GitHub-PR-style review UI needs — a working-tree-inclusive "blended" diff (committed + uncommitted vs the fork point), per-file `uncommitted` flagging, multi-line range anchors on comment threads, and human/agent authorship on the live `/threads` wire.

**Architecture:** Pure additive changes to the existing layered Go backend (`engine` → `app/usecases` → `api/v0`). The review diff switches from a committed-only 3-dot `RangeDiff` to a new `DiffAgainstRef` evaluated in the worktree against the workspace fork point, then annotated with per-file uncommitted state from `git status`. Thread anchors gain a start/end line; the `/threads` handlers + DTO gain author + `isAgent` (the domain, repo `OpenInput`, and asynx commands already support these — only the HTTP layer and `Reply` path drop them).

**Tech Stack:** Go, gin, the `git` engine (shells the system `git`), testify integration suite (`//go:build integration`) under `api/tests`.

## Global Constraints

- Test files are black-box integration tests under `api/tests/*_test.go` with the build tag `//go:build integration`, driven by the `harness` helper (`newHarness`, `importWritableWorkspace`, `wsBase`, `h.get/post/put/patch`). Run them with: `go test -tags integration ./api/tests/... -run <Name>`.
- Engine unit tests live in `package git_test` beside the engine and use the existing `initRepo`/`makeCommit`/`gitRun` helpers + `git.New()`. Run with: `go test ./api/internal/engine/git/... -run <Name>`.
- Match surrounding style: gofmt, doc comment on every exported symbol, errors wrapped as `fmt.Errorf("context: %w", err)`. Run `gofmt -l` / `go vet ./...` clean.
- All routes are entity-scoped: `/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/...`. Responses use the `{success,error,data}` envelope (handled by `libs.WriteQueryOK` etc.).
- The diff wire DTO (`FileDiffDTO`/`MultiFileDiffDTO`) **embeds** the domain structs, so a new domain `FileDiff` field surfaces on the wire automatically — no DTO edit needed for it.
- **Out of scope for this plan:** untracked (never-added) files do not appear in the review diff body yet — they show in the sidebar status; their synthesized diff is a follow-up. The GitHub-identity resolver (`gh api user`) lands in the comments plan, which consumes it.

---

### Task 1: Engine — `DiffAgainstRef` (working-tree-inclusive diff)

A new engine op: the full diff of the **working tree** (committed + uncommitted tracked changes) against an arbitrary ref. This is the "blended" diff source. It mirrors the existing `diff.Range` exactly, but uses two-dot-to-working-tree semantics (`git diff -M <ref> --`, no `...`).

**Files:**
- Create: `api/internal/engine/git/internal/diff/against_ref.go`
- Create: `api/internal/engine/git/diff_against_ref.go`
- Modify: `api/internal/engine/git/git.go` (add method to the `Engine` interface, after `RangeDiff` ~line 370)
- Test: `api/internal/engine/git/diff_against_ref_test.go`

**Interfaces:**
- Consumes: `exec.Git`, `parseFiles`, `totals` (existing in `package diff`, used by `range.go`).
- Produces:
  - `diff.AgainstRef(ctx context.Context, repoPath, ref string) (gitdomain.MultiFileDiff, error)`
  - `Engine.DiffAgainstRef(ctx context.Context, repoPath, ref string) (gitdomain.MultiFileDiff, error)`

- [ ] **Step 1: Write the failing engine test**

Create `api/internal/engine/git/diff_against_ref_test.go`:

```go
package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// TestDiffAgainstRef_IncludesCommittedAndUncommitted proves the blended diff:
// against the base ref it shows both a committed change on the branch AND an
// uncommitted working-tree edit.
func TestDiffAgainstRef_IncludesCommittedAndUncommitted(t *testing.T) {
	dir := initRepo(t)
	makeCommit(t, dir, "base.go", "package main\n", "base commit")

	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir, "committed.go", "package main\n\nfunc c() {}\n", "committed change")

	// uncommitted working-tree edit of a tracked file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.go"), []byte("package main\n// edit\n"), 0o644))

	ctx := context.Background()
	e := git.New()
	result, err := e.DiffAgainstRef(ctx, dir, "main")
	require.NoError(t, err)

	paths := map[string]bool{}
	for _, f := range result.Files {
		paths[f.FilePath] = true
	}
	assert.True(t, paths["committed.go"], "committed branch change must appear")
	assert.True(t, paths["base.go"], "uncommitted working-tree edit must appear")
	assert.GreaterOrEqual(t, result.TotalFiles, 2)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./api/internal/engine/git/... -run TestDiffAgainstRef_IncludesCommittedAndUncommitted`
Expected: FAIL — `e.DiffAgainstRef` undefined.

- [ ] **Step 3: Implement `diff.AgainstRef`**

Create `api/internal/engine/git/internal/diff/against_ref.go`:

```go
package diff

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// AgainstRef returns the diff of the working tree against ref, including both
// committed changes since ref and uncommitted tracked modifications. It runs
// `git diff -M <ref> --` (two-dot-to-working-tree) and returns a MultiFileDiff.
// Commit metadata fields are always zero-value. Untracked files are not
// included (they have no diff against the index); they surface via git status.
func AgainstRef(
	ctx context.Context,
	repoPath string,
	ref string,
) (gitdomain.MultiFileDiff, error) {
	r := exec.Git(ctx, repoPath, "diff", "-M", ref, "--")
	if err := exec.RequireSuccess("diff: against ref", r); err != nil {
		return gitdomain.MultiFileDiff{}, err
	}
	files := parseFiles(ctx, repoPath, r.Stdout)
	totalAdd, totalDel := totals(files)
	return gitdomain.MultiFileDiff{
		Files:          files,
		TotalFiles:     len(files),
		TotalAdditions: totalAdd,
		TotalDeletions: totalDel,
	}, nil
}
```

Create `api/internal/engine/git/diff_against_ref.go`:

```go
package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// DiffAgainstRef returns the working-tree-inclusive diff against ref (review §5).
func (e *engine) DiffAgainstRef(
	ctx context.Context,
	repoPath string,
	ref string,
) (gitdomain.MultiFileDiff, error) {
	return diff.AgainstRef(ctx, repoPath, ref)
}
```

Add to the `Engine` interface in `api/internal/engine/git/git.go` (right after the `RangeDiff` method, before the closing `}` of the interface ~line 370):

```go
	// DiffAgainstRef returns the working-tree-inclusive diff against ref:
	// committed changes since ref plus uncommitted tracked modifications
	// (`git diff -M <ref> --`). Used for the blended branch-review diff.
	DiffAgainstRef(
		ctx context.Context,
		repoPath string,
		ref string,
	) (gitdomain.MultiFileDiff, error)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./api/internal/engine/git/... -run TestDiffAgainstRef_IncludesCommittedAndUncommitted`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/git/internal/diff/against_ref.go api/internal/engine/git/diff_against_ref.go api/internal/engine/git/git.go api/internal/engine/git/diff_against_ref_test.go
git commit -m "feat(engine): add DiffAgainstRef for working-tree-inclusive diffs"
```

---

### Task 2: Blended review diff + per-file `uncommitted` flag

Add the `Uncommitted` field to the domain `FileDiff` (auto-surfaces on the wire via the embedded DTO), and switch `branchReviewUsecase.Get` to diff against the workspace fork point with `DiffAgainstRef`, then annotate each file's `Uncommitted` from `git status`.

**Files:**
- Modify: `api/internal/domain/git/diff.go` (add `Uncommitted` to `FileDiff`, ~line 40)
- Modify: `api/internal/app/usecases/branchreview/branch_review.go` (`Get` + new helpers)
- Test: `api/tests/review_test.go` (new integration test)

**Interfaces:**
- Consumes: `Engine.DiffAgainstRef` (Task 1), `Engine.MergeBase`, `Engine.Status`, `domain.Workspace.ForkPointSha`.
- Produces: `GET .../workspaces/:wsId/review` whose `diff.files[].uncommitted` is `true` for files with working-tree changes and `false` for committed-only files.

- [ ] **Step 1: Write the failing integration test**

Create `api/tests/review_test.go`:

```go
//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reviewDiffFile struct {
	FilePath    string `json:"file_path"`
	Uncommitted bool   `json:"uncommitted"`
}

type reviewDTO struct {
	Diff struct {
		Files []reviewDiffFile `json:"files"`
	} `json:"diff"`
}

// TestReview_BlendedDiffFlagsUncommitted proves the review diff is blended:
// a committed branch change appears with uncommitted=false, while a working-tree
// edit appears with uncommitted=true.
func TestReview_BlendedDiffFlagsUncommitted(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	// committed change on the branch
	var saved struct{ ID string `json:"id"` }
	h.put(base+"/files/content", map[string]string{"path": "committed.txt", "content": "one\n"}, &saved)
	h.post(base+"/git/stage", map[string]any{"paths": []string{"committed.txt"}}, 200, &saved)
	h.post(base+"/git/commit", map[string]string{"subject": "add committed.txt"}, 200, &saved)

	// uncommitted working-tree edit of a tracked file
	h.put(base+"/files/content", map[string]string{"path": "README.md", "content": "edited\n"}, &saved)

	var review reviewDTO
	h.get(base+"/review", &review)

	byPath := map[string]bool{}
	for _, f := range review.Diff.Files {
		byPath[f.FilePath] = f.Uncommitted
	}
	require.Contains(t, byPath, "committed.txt", "committed change must be in the review diff")
	require.Contains(t, byPath, "README.md", "uncommitted edit must be in the review diff")
	assert.False(t, byPath["committed.txt"], "committed file must be flagged uncommitted=false")
	assert.True(t, byPath["README.md"], "edited file must be flagged uncommitted=true")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -tags integration ./api/tests/... -run TestReview_BlendedDiffFlagsUncommitted`
Expected: FAIL — files missing or `uncommitted` always false (the diff is still committed-only 3-dot).

- [ ] **Step 3: Add the `Uncommitted` field to the domain `FileDiff`**

In `api/internal/domain/git/diff.go`, add the field to `FileDiff` (after `Hunks`, ~line 40):

```go
	Hunks         []Hunk     `json:"hunks"`
	// Uncommitted is true when the file has working-tree changes not yet
	// committed (staged or unstaged). Used by the blended branch-review diff to
	// mark files as committed vs uncommitted. Always false for commit/range diffs.
	Uncommitted bool `json:"uncommitted"`
```

- [ ] **Step 4: Switch `Get` to the blended diff + annotate uncommitted**

In `api/internal/app/usecases/branchreview/branch_review.go`, replace the body of `Get` (lines 96–113) and add two helpers. New `Get`:

```go
func (u *branchReviewUsecase) Get(
	ctx context.Context,
	wsID string,
) (domain.BranchReview, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: get workspace: %w", asNotFound(err))
	}
	ref, err := u.resolveDiffRef(ctx, ws)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: resolve ref: %w", err)
	}
	diff, err := u.git.DiffAgainstRef(ctx, ws.WorktreePath, ref)
	if err != nil {
		return domain.BranchReview{}, fmt.Errorf("branch review: diff: %w", err)
	}
	u.annotateUncommitted(ctx, ws, &diff)
	return u.assemble(ctx, ws, diff)
}

// resolveDiffRef returns the ref the review diffs against: the workspace's
// recorded fork point when known, else the merge-base of the parent/default
// branch and HEAD. This makes the diff show exactly this workspace's changes
// (committed + uncommitted) since it diverged.
func (u *branchReviewUsecase) resolveDiffRef(
	ctx context.Context,
	ws domain.Workspace,
) (string, error) {
	if ws.ForkPointSha != "" {
		return ws.ForkPointSha, nil
	}
	base, err := u.resolveBase(ctx, ws)
	if err != nil {
		return "", err
	}
	return u.git.MergeBase(ctx, ws.WorktreePath, base, "HEAD")
}

// annotateUncommitted marks each diff file as uncommitted when it has a matching
// entry in git status (staged or unstaged working-tree change). Status failures
// are non-fatal: the diff is still returned, just without the flags.
func (u *branchReviewUsecase) annotateUncommitted(
	ctx context.Context,
	ws domain.Workspace,
	diff *gitdomain.MultiFileDiff,
) {
	status, err := u.git.Status(ctx, ws.WorktreePath)
	if err != nil {
		return
	}
	dirty := make(map[string]bool, len(status.Files))
	for _, f := range status.Files {
		dirty[f.Path] = true
	}
	for i := range diff.Files {
		diff.Files[i].Uncommitted = dirty[diff.Files[i].FilePath]
	}
}
```

Keep the existing `resolveBase` helper (still used by `resolveDiffRef`). If `go vet` flags `gitdomain` already imported, it is (line 19) — no new import needed. If `domain/git` `GitStatus.Files[].Path` differs, confirm the field name in `api/internal/domain/git/status.go` and adjust.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test -tags integration ./api/tests/... -run TestReview_BlendedDiffFlagsUncommitted`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/domain/git/diff.go api/internal/app/usecases/branchreview/branch_review.go api/tests/review_test.go
git commit -m "feat(review): blended working-tree-inclusive diff with per-file uncommitted flag"
```

---

### Task 3: Multi-line range anchors on review threads

Add `StartLine`/`EndLine` to the thread anchor end-to-end. `LineNumber` stays as the primary anchor (the end line); single-line comments set `StartLine == EndLine == LineNumber`.

**Files:**
- Modify: `api/internal/domain/review_thread.go` (add `StartLine`, `EndLine`)
- Modify: `api/internal/app/repositories/reviewthread/reviewthread.go` (`OpenInput` + pass-through)
- Modify: `api/internal/app/repositories/reviewthread/internal/commands/open.go` (carry + emit)
- Modify: `api/internal/api/v0/endpoints/threads/handlers/threads.go` (`OpenThread` body)
- Modify: `api/internal/api/v0/dto/thread.go` (`ThreadDTO` + `ThreadDTOFrom`)
- Test: `api/tests/threads_test.go` (new integration test)

**Interfaces:**
- Produces: `POST .../threads` accepts `{filePath, line, startLine, endLine, side, body}` (startLine/endLine optional, default to `line`); `ThreadDTO` carries `startLine`, `endLine`.

- [ ] **Step 1: Write the failing integration test**

Create `api/tests/threads_test.go`:

```go
//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

type threadDTO struct {
	ID        string `json:"id"`
	FilePath  string `json:"filePath"`
	Line      int    `json:"line"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Side      string `json:"side"`
	Body      string `json:"body"`
	Author    string `json:"author"`
	IsAgent   bool   `json:"isAgent"`
}

// TestThreads_RangeAnchor proves a thread can be anchored to a multi-line range.
func TestThreads_RangeAnchor(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	var got threadDTO
	h.post(base+"/threads", map[string]any{
		"filePath":  "README.md",
		"line":      44,
		"startLine": 42,
		"endLine":   44,
		"side":      "right",
		"body":      "guard this range",
	}, http.StatusCreated, &got)

	assert.Equal(t, 42, got.StartLine)
	assert.Equal(t, 44, got.EndLine)
	assert.Equal(t, 44, got.Line)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -tags integration ./api/tests/... -run TestThreads_RangeAnchor`
Expected: FAIL — `startLine`/`endLine` decode to 0 (fields don't exist yet).

- [ ] **Step 3: Add range to the domain aggregate**

In `api/internal/domain/review_thread.go`, add the fields to `ReviewThread` (after `LineNumber`, line 11):

```go
	LineNumber int                `json:"lineNumber"`
	// StartLine and EndLine bound a multi-line comment range on the same Side.
	// For a single-line comment StartLine == EndLine == LineNumber.
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
```

- [ ] **Step 4: Carry range through the command + repo input**

In `api/internal/app/repositories/reviewthread/internal/commands/open.go`, add `StartLine`/`EndLine` to the `OpenReviewThread` struct (after `LineNumber`, line 18) and set them in `EmitEvent` (after `LineNumber: c.LineNumber`, line 58):

```go
	LineNumber int
	StartLine  int
	EndLine    int
```
```go
		FilePath:   c.FilePath,
		LineNumber: c.LineNumber,
		StartLine:  c.StartLine,
		EndLine:    c.EndLine,
		Side:       c.Side,
```

In `api/internal/app/repositories/reviewthread/reviewthread.go`, add `StartLine`/`EndLine` to `OpenInput` (after `LineNumber`, line 24) and pass them in `Open` (after `LineNumber: in.LineNumber`, line 96):

```go
	LineNumber int
	StartLine  int
	EndLine    int
```
```go
		FilePath:   in.FilePath,
		LineNumber: in.LineNumber,
		StartLine:  in.StartLine,
		EndLine:    in.EndLine,
		Side:       in.Side,
```

- [ ] **Step 5: Accept range in the HTTP handler**

In `api/internal/api/v0/endpoints/threads/handlers/threads.go`, extend the `OpenThread` request body and the `OpenInput` it builds (lines 65–83):

```go
	var body struct {
		FilePath  string            `json:"filePath"`
		Line      int               `json:"line"`
		StartLine int               `json:"startLine"`
		EndLine   int               `json:"endLine"`
		Side      domain.ReviewSide `json:"side"`
		Body      string            `json:"body"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.StartLine == 0 {
		body.StartLine = body.Line
	}
	if body.EndLine == 0 {
		body.EndLine = body.Line
	}
	thread, err := h.store.Open(ctx.Request.Context(), reviewthread.OpenInput{
		ID:         h.newID(),
		WsID:       wsID,
		FilePath:   body.FilePath,
		LineNumber: body.Line,
		StartLine:  body.StartLine,
		EndLine:    body.EndLine,
		Side:       body.Side,
		MessageID:  h.newID(),
		Body:       body.Body,
	}, h.now())
```

- [ ] **Step 6: Expose range on the DTO**

In `api/internal/api/v0/dto/thread.go`, add fields to `ThreadDTO` (after `Line`, line 29) and set them in `ThreadDTOFrom` (after `Line: rt.LineNumber`, line 72):

```go
	Line        int              `json:"line"`
	StartLine   int              `json:"startLine"`
	EndLine     int              `json:"endLine"`
```
```go
		FilePath:    rt.FilePath,
		Line:        rt.LineNumber,
		StartLine:   rt.StartLine,
		EndLine:     rt.EndLine,
		Side:        string(rt.Side),
```

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test -tags integration ./api/tests/... -run TestThreads_RangeAnchor`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add api/internal/domain/review_thread.go api/internal/app/repositories/reviewthread/ api/internal/api/v0/endpoints/threads/handlers/threads.go api/internal/api/v0/dto/thread.go api/tests/threads_test.go
git commit -m "feat(threads): multi-line range anchors (startLine/endLine)"
```

---

### Task 4: Author + `isAgent` on the live `/threads` wire

Surface authorship: the `OpenThread`/`Reply` handlers accept `author` + `isAgent`, thread them through (the `Reply` repo path currently drops them), and the `ThreadDTO`/`ThreadReplyDTO` expose `isAgent`.

**Files:**
- Modify: `api/internal/app/repositories/reviewthread/reviewthread.go` (`Reply` signature + interface)
- Modify: `api/internal/app/usecases/branchreview/branch_review.go` (`Reply` usecase call site)
- Modify: `api/internal/api/v0/endpoints/threads/handlers/threads.go` (`OpenThread` + `Reply` bodies)
- Modify: `api/internal/api/v0/dto/thread.go` (`ThreadDTO` + `ThreadReplyDTO` + `ThreadDTOFrom`)
- Test: extend `api/tests/threads_test.go`

**Interfaces:**
- Consumes: `commands.ReplyReviewThread{Author, IsAgent}` (already present).
- Produces: `ReviewThread.Reply(ctx, id, messageID, author string, isAgent bool, body string, now time.Time)`; `POST .../threads` + `.../replies` accept `{author, isAgent}`; `ThreadDTO`/`ThreadReplyDTO` carry `isAgent`.

- [ ] **Step 1: Write the failing integration test**

Append to `api/tests/threads_test.go`:

```go
type threadReplyDTO struct {
	ID      string `json:"id"`
	Body    string `json:"body"`
	Author  string `json:"author"`
	IsAgent bool   `json:"isAgent"`
}

type threadWithReplies struct {
	threadDTO
	Replies []threadReplyDTO `json:"replies"`
}

// TestThreads_AuthorAndIsAgent proves human vs agent authorship round-trips on
// open and reply.
func TestThreads_AuthorAndIsAgent(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	var opened threadWithReplies
	h.post(base+"/threads", map[string]any{
		"filePath": "README.md", "line": 10, "side": "right",
		"author": "mateourru", "isAgent": false, "body": "@claude take a look",
	}, http.StatusCreated, &opened)
	assert.Equal(t, "mateourru", opened.Author)
	assert.False(t, opened.IsAgent)

	var replied threadWithReplies
	h.post(base+"/threads/"+opened.ID+"/replies", map[string]any{
		"author": "claude", "isAgent": true, "body": "on it",
	}, http.StatusOK, &replied)

	require.Len(t, replied.Replies, 1)
	assert.Equal(t, "claude", replied.Replies[0].Author)
	assert.True(t, replied.Replies[0].IsAgent, "agent reply must carry isAgent")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -tags integration ./api/tests/... -run TestThreads_AuthorAndIsAgent`
Expected: FAIL — reply has no author/isAgent fields and the body is ignored on the wire.

- [ ] **Step 3: Thread author/isAgent through the repo `Reply`**

In `api/internal/app/repositories/reviewthread/reviewthread.go`, change the `Reply` method on the `ReviewThread` interface (lines 40–46) and its implementation (lines 110–127):

```go
	Reply(
		ctx context.Context,
		id string,
		messageID string,
		author string,
		isAgent bool,
		body string,
		now time.Time,
	) (domain.ReviewThread, error)
```
```go
func (r *reviewThread) Reply(
	ctx context.Context,
	id string,
	messageID string,
	author string,
	isAgent bool,
	body string,
	now time.Time,
) (domain.ReviewThread, error) {
	evt, err := r.ax.SendWait(ctx, commands.ReplyReviewThread{
		ID:        id,
		MessageID: messageID,
		Author:    author,
		IsAgent:   isAgent,
		Body:      body,
		Now:       now,
	})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: reply: %w", err)
	}
	return evt.Aggregate, nil
}
```

- [ ] **Step 4: Update the other `Reply` call site (branchreview usecase)**

In `api/internal/app/usecases/branchreview/branch_review.go`, the usecase `Reply` (lines 191–201) calls `u.threads.Reply`. Update the call to pass empty author/false (this usecase path is not author-aware; the live path is the handler):

```go
	thread, err := u.threads.Reply(ctx, threadID, uuid.NewString(), "", false, body, u.now())
```

If a generated mock exists at `api/internal/app/repositories/reviewthread/internal/mocks/mocks.go`, regenerate or hand-edit its `Reply` signature to match. Run `go build ./...` to surface any other call sites and fix them the same way.

- [ ] **Step 5: Accept author/isAgent in the handlers**

In `api/internal/api/v0/endpoints/threads/handlers/threads.go`:

`OpenThread` body — add `Author`/`IsAgent` and pass to `OpenInput`:

```go
	var body struct {
		FilePath  string            `json:"filePath"`
		Line      int               `json:"line"`
		StartLine int               `json:"startLine"`
		EndLine   int               `json:"endLine"`
		Side      domain.ReviewSide `json:"side"`
		Author    string            `json:"author"`
		IsAgent   bool              `json:"isAgent"`
		Body      string            `json:"body"`
	}
```
(in the `reviewthread.OpenInput{...}` literal add:) `Author: body.Author, IsAgent: body.IsAgent,`

`Reply` handler — add `Author`/`IsAgent` and pass to the new signature (lines 154–161):

```go
	var body struct {
		Author  string `json:"author"`
		IsAgent bool   `json:"isAgent"`
		Body    string `json:"body"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	thread, err := h.store.Reply(ctx.Request.Context(), threadID, h.newID(), body.Author, body.IsAgent, body.Body, h.now())
```

- [ ] **Step 6: Expose `isAgent` on the DTOs**

In `api/internal/api/v0/dto/thread.go`, add `IsAgent` to `ThreadReplyDTO` (after `Author`, line 15) and `ThreadDTO` (after `Author`, line 32), and populate both in `ThreadDTOFrom`:

```go
	Author    string    `json:"author"`
	IsAgent   bool      `json:"isAgent"`
```
(root, in the `ThreadDTO{...}` return add `IsAgent: rootIsAgent,`; reply, in the appended `ThreadReplyDTO{...}` add `IsAgent: msg.IsAgent,`). Track the root message's flag while looping:

```go
	body := ""
	author := ""
	rootIsAgent := false
	replies := make([]ThreadReplyDTO, 0, len(rt.Messages))
	for i, msg := range rt.Messages {
		if i == 0 {
			body = msg.Body
			author = msg.Author
			rootIsAgent = msg.IsAgent
			continue
		}
		replies = append(replies, ThreadReplyDTO{
			ID:        msg.ID,
			ThreadID:  rt.ID,
			Body:      msg.Body,
			Author:    msg.Author,
			IsAgent:   msg.IsAgent,
			CreatedAt: msg.CreatedAt,
		})
	}
```

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test -tags integration ./api/tests/... -run TestThreads_AuthorAndIsAgent`
Expected: PASS. Then run the whole thread/review suite to confirm no regressions: `go test -tags integration ./api/tests/... -run 'TestThreads|TestReview'`.

- [ ] **Step 8: Commit**

```bash
git add api/internal/app/repositories/reviewthread/ api/internal/app/usecases/branchreview/branch_review.go api/internal/api/v0/endpoints/threads/handlers/threads.go api/internal/api/v0/dto/thread.go api/tests/threads_test.go
git commit -m "feat(threads): author + isAgent on open and reply wire"
```

---

## Self-Review

**Spec coverage (backend items):**
- `DiffAgainstRef` + working-tree-inclusive review diff → Task 1 + Task 2. ✓
- Per-file `uncommitted` flag → Task 2. ✓
- Range anchors (`StartLine`/`EndLine`) → Task 3. ✓
- `isAgent` + author on `/threads` DTO + handlers → Task 4. ✓
- Identity resolver (`gh api user`) → **deferred to the comments plan** (consumes it). Noted in Global Constraints.
- Outdated snapshot field → deferred (FE-first per spec §5.6); revisit in the comments plan.
- Untracked-file diffs → deferred (noted), tracked changes only in v1.

**Placeholder scan:** No TBD/TODO/"handle errors appropriately" — every step shows real code and exact commands. ✓

**Type consistency:** `DiffAgainstRef(ctx, repoPath, ref)` is identical in the interface, the engine method, and the test. `Reply(ctx, id, messageID, author, isAgent, body, now)` is consistent across the interface, impl, both call sites, and the handler. `Uncommitted bool json:"uncommitted"` matches between the domain field and the test decode. `StartLine`/`EndLine` consistent across domain, command, `OpenInput`, handler, DTO. ✓

**Notes for the implementer:** confirm `domain/git` `GitStatus.Files[].Path` field name (Task 2 Step 4) and whether a generated reviewthread mock needs its `Reply` signature updated (Task 4 Step 4) — `go build ./...` surfaces both.
