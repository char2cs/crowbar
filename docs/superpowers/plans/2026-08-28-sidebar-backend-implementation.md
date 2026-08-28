# Unified Sidebar — Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the sidebar's backend so a `Chat` row is one of four kinds
(`chat`, `branch`, `folder`, `workflow`), placement is one tree, a chat's
workspace is optional and mutable, and the verbs (create, promote, move,
delete) enforce the new invariants server-side.

**Architecture:** `domain.Chat` gains a closed `Type` taxonomy and absorbs
`domain.Folder` + `domain.ChatFolder`. `Workspace.ParentID` becomes a derived
projection; `FolderID`/`Order` are deleted from it. The generic `app/tree.Tree`
planner (already built, already reused twice under different names) becomes
the one implementation the unified forest walk builds on. Routes move off
`/workspaces/:wsId/` onto `/repos/:rid/`.

**Tech Stack:** Go, GORM (SQLite via `adapter/store/sqlite`), asynx aggregates,
Chi router (`api/v0/endpoints`).

**Spec:** [`docs/superpowers/specs/2026-08-23-unified-sidebar-design.md`](../specs/2026-08-23-unified-sidebar-design.md)
(the model — read this first, this plan does not repeat its rationale) +
[`docs/superpowers/specs/2026-08-28-sidebar-backend-implementation.md`](../specs/2026-08-28-sidebar-backend-implementation.md)
(the two additions this plan folds into stages 4 and 7).

**A note on drift from the model spec.** The model spec's own "what is wrong
today" table (§1) is stale relative to the code on this branch. Where a task
below contradicts that table, the task is right and the table is not — this
was verified by reading the actual files, not by re-deriving from the spec's
prose. The concrete drifts, so nobody re-derives them:

- `usecases/agentchatfolder` doesn't exist as a package. It's already folded
  into `usecases/chat/internal/tree`, operating on `domain.ChatFolder` — the
  already-renamed `AgentChatFolder`. **The package name the model spec wants
  for the whole unified forest is already taken** by this old, chat-folder-only
  implementation. Stage 1/3 gut and rebuild it in place; they do not create it.
- `domain.Chat` has no `Type` field at all today (stage 1 adds one).
  `domain.ChatType` exists but is dead code — only `chat`/`workflow` values,
  zero references anywhere.
- `PATCH /chats/:id/placement` already exists (`SetPlacement` command,
  `ParentID`+`Order`, already cross-lineage by design). Stage 3 mostly extends
  this to branch/folder rows rather than building placement from zero.
- `DELETE /chats/:id` already exists, workspace-scoped. The wire-contract
  change is a rescope (drop `:wsId` from the path), not a new route. Only
  `delete-preview` (the addendum) is genuinely new.
- Two near-duplicate tree implementations already exist —
  `usecases/chat/internal/tree` (chat folders) and `usecases/folder` (repo
  folders) — both already wrapping the generic `app/tree.Tree`. This is the
  "same concept declared twice" problem the model spec names, just located
  differently than its table says.

## Global Constraints

- **One public file per package**; everything else under that package's
  `internal/`. (Model spec principle 1.)
- **Engines never cross-import.** (Principle 2.)
- **The usecase holds no machinery** — no mutex on any usecase type.
  (Principle 3.)
- **No migration.** This repo has no SQL migration files — schema changes are
  GORM struct-field changes picked up by `AutoMigrate` on next boot
  (confirmed: `adapter/store/sqlite/sqlite.go:108`,
  `repositories/chat/activity/internal/store/internal/storage/storage.go:20`).
  Every fork workspace today has no chat at all; that backfill is explicitly
  out of scope (model spec, "Not in scope").
- **Coverage floor is 92%, not lower** — `api/Makefile`'s `test-coverage`
  target enforces this exact number today (its own comment records that 95%
  was tried and rolled back: "a local gate that cries wolf is a gate people
  learn to ignore"). Every task's tests must clear this floor for the packages
  they touch; do not lower it.
- **Every CI command below is exact, copied from `api/Makefile` — run these,
  not invented equivalents:**
  ```
  cd api && go test -tags noEmbed -race ./...                                      # make test
  cd api && go test -tags 'integration noEmbed' -race -v -timeout 600s -p 1 ./...   # make test-integration
  cd api && go vet -tags noEmbed ./...                                              # make govet
  cd api && golangci-lint run --build-tags noEmbed ./...                            # make lint
  cd api && go test -tags noEmbed -race -count=1 -coverpkg=$(COVERPKG) -coverprofile=coverage.out ./... && go tool cover -func=coverage.out   # make test-coverage
  ```
  `-count=1` is load-bearing — cached test binaries misattribute coverage
  under a wide `-coverpkg`. Run `make -C api pr-checks` (lint, test,
  test-coverage) as the full local gate before each stage's commit.
- **Test package convention, confirmed repo-wide:** black-box by default
  (`package foo_test`), `_internal_test.go` suffix for white-box tests needing
  unexported access, `_export_test.go` to expose internals to the black-box
  package. Follow whichever a file you're modifying already uses.
- **Go style:** one parameter per line always; max 2 indentation levels; early
  exits, happy path last; no inline comments, no comments on unexported
  symbols; one type per file; singletons are package-level functions, not
  structs; constructors (`NewXxx`) only when there's real state; public APIs
  expose interfaces, never concrete types.

---

## Stage 1 — The row taxonomy

*Model spec §3.1, §7 stage 1. Independent of every other stage.*

### Task 1: `ChatType` gains `branch` and `folder`

**Files:**
- Modify: `api/internal/domain/chat_type.go`
- Test: `api/internal/domain/chat_type_test.go` (new — none exists today)

**Interfaces:**
- Produces: `domain.ChatTypeBranch`, `domain.ChatTypeFolder` — consumed by
  every task below and by stage 3's tree walk.

- [ ] **Step 1: Write the failing test**

```go
package domain_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestChatType_ClosedTaxonomy(t *testing.T) {
	want := []domain.ChatType{
		domain.ChatTypeChat,
		domain.ChatTypeBranch,
		domain.ChatTypeFolder,
		domain.ChatTypeWorkflow,
	}
	for _, tc := range want {
		if tc == "" {
			t.Fatalf("chat type constant is empty")
		}
	}
	if domain.ChatTypeBranch == domain.ChatTypeFolder {
		t.Fatalf("branch and folder must be distinct")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags noEmbed ./internal/domain/... -run TestChatType_ClosedTaxonomy -v`
Expected: FAIL — `domain.ChatTypeBranch` undefined.

- [ ] **Step 3: Add the two constants**

```go
const (
	ChatTypeChat     ChatType = "chat"
	ChatTypeBranch   ChatType = "branch"
	ChatTypeFolder   ChatType = "folder"
	ChatTypeWorkflow ChatType = "workflow"
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test -tags noEmbed ./internal/domain/... -run TestChatType_ClosedTaxonomy -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/domain/chat_type.go api/internal/domain/chat_type_test.go
git commit -m "feat(domain): close the ChatType taxonomy over branch and folder"
```

### Task 2: `domain.Chat` gains `Type`

**Files:**
- Modify: `api/internal/domain/chat.go`
- Test: `api/internal/domain/chat_test.go` (check if it exists; if not, create)

**Interfaces:**
- Consumes: `domain.ChatType` (Task 1).
- Produces: `Chat.Type` field — consumed by every repository/usecase task
  below.

- [ ] **Step 1: Add the field**

```go
// Type is the row's kind in the sidebar forest. Closed taxonomy: chat,
// branch, folder, workflow. A row's type never changes after creation —
// promotion fills WorkspaceID on a chat row, it does not retype the row.
Type ChatType `json:"type"`
```

Add directly under the `ID` field in the struct.

- [ ] **Step 2: Write a struct-shape test**

```go
package domain_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestChat_HasType(t *testing.T) {
	c := domain.Chat{Type: domain.ChatTypeBranch}
	if c.Type != domain.ChatTypeBranch {
		t.Fatalf("Type not settable/readable")
	}
}
```

- [ ] **Step 3: Run, confirm pass**

Run: `cd api && go test -tags noEmbed ./internal/domain/... -run TestChat_HasType -v`
Expected: PASS (this is a compile-gate test — its value is catching a future
accidental field rename).

- [ ] **Step 4: Commit**

```bash
git add api/internal/domain/chat.go api/internal/domain/chat_test.go
git commit -m "feat(domain): give Chat a Type field"
```

### Task 3: `Create` command carries `Type`, defaults enforced

**Files:**
- Modify: `api/internal/app/repositories/chat/internal/commands/create.go`
- Test: `api/internal/app/repositories/chat/internal/commands/create_test.go`

**Interfaces:**
- Consumes: `domain.ChatType` constants.
- Produces: `Create.Type domain.ChatType` — consumed by Task 4
  (`usecases/chat`'s `CreateChild`) and stage 5.

Today's `create.go` validates `c.ID == "" || c.WorkspaceID == ""`
(`internal/commands/create.go:32-34`) — `WorkspaceID` is still required here;
that requirement is removed in Stage 2, not this task. This task only adds
`Type` and validates it against the closed set.

- [ ] **Step 1: Write the failing test**

```go
func TestCreate_Validate_RejectsUnknownType(t *testing.T) {
	c := Create{ID: "chat-1", WorkspaceID: "ws-1", Type: domain.ChatType("bogus")}
	if err := c.Validate(); err == nil {
		t.Fatalf("expected error for unknown chat type")
	}
}

func TestCreate_Validate_AcceptsEachKnownType(t *testing.T) {
	for _, ct := range []domain.ChatType{
		domain.ChatTypeChat,
		domain.ChatTypeBranch,
		domain.ChatTypeFolder,
		domain.ChatTypeWorkflow,
	} {
		c := Create{ID: "chat-1", WorkspaceID: "ws-1", Type: ct}
		if err := c.Validate(); err != nil {
			t.Fatalf("type %s: unexpected error: %v", ct, err)
		}
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `cd api && go test -tags noEmbed ./internal/app/repositories/chat/internal/commands/... -run TestCreate_Validate -v`
Expected: FAIL — `Type` field undefined on `Create`.

- [ ] **Step 3: Implement**

Add `Type domain.ChatType` to the `Create` struct. In `Validate`, add a
closed-set check:

```go
func validChatType(t domain.ChatType) bool {
	switch t {
	case domain.ChatTypeChat, domain.ChatTypeBranch, domain.ChatTypeFolder, domain.ChatTypeWorkflow:
		return true
	default:
		return false
	}
}
```

called from `Validate` alongside the existing ID/WorkspaceID checks. In
`EmitEvent`, set `chat.Type = c.Type`.

- [ ] **Step 4: Run, verify pass**

Run: `cd api && go test -tags noEmbed ./internal/app/repositories/chat/internal/commands/... -v`
Expected: PASS, including every pre-existing test in the package.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/repositories/chat/internal/commands/create.go api/internal/app/repositories/chat/internal/commands/create_test.go
git commit -m "feat(chat): validate Type against the closed taxonomy on create"
```

### Task 4: Fold `domain.Folder` and `domain.ChatFolder` into `folder`-typed chats

**Files:**
- Delete: `api/internal/domain/folder.go`, `api/internal/domain/chat_folder.go`
- Delete: `api/internal/app/usecases/folder/` (whole package)
- Delete: `api/internal/api/v0/endpoints/folders/` (whole package)
- Delete: `api/internal/api/v0/dto/folder.go`, `folder_test.go`,
  `agent_chat_folder.go`
- Rewrite: `api/internal/app/usecases/chat/internal/tree/tree.go` — this file
  currently implements `chatFolderUsecase` over `domain.ChatFolder`. Replace
  its body so every operation targets `domain.Chat` rows where
  `Type == ChatTypeFolder` instead. Keep the package doc comment's
  asymmetric-delete rule (folder delete promotes children; a `chat`-typed row
  cascades) — it's the load-bearing rule this task must preserve, just over
  the new row shape.
- Test: `api/internal/app/usecases/chat/internal/tree/tree_test.go`

**Interfaces:**
- Consumes: `domain.ChatTypeFolder` (Task 1), `Chat.Type` (Task 2).
- Produces: `tree.Usecase` interface unchanged in shape
  (`ListInWorkspace`/`Create`/`Rename`/`Move`/`Delete`) but now operates over
  folder-typed `Chat` rows repo-wide, not workspace-scoped `ChatFolder` rows.
  This is a breaking signature change — `ListInWorkspace(ctx, workspaceID)`
  becomes repo-scoped (`ListInRepo(ctx, repoID)`), since a folder's scope is
  now the repo forest, not a single workspace. Stage 3 finishes the walk
  logic; this task only retypes the storage.

This is the task where the two duplicate implementations
(`usecases/chat/internal/tree` and `usecases/folder`) actually merge. Read
both fully before writing code — `usecases/folder/folder.go`'s
`resolvePlacement`/`checkMove`/`checkContainer`/`checkFolderTarget` logic is
the more general of the two (it already handles cross-container placement,
which the chat-folder version doesn't need today because it's workspace-local)
and should be the one that survives, adapted to the unified row.

- [ ] **Step 1: Write the failing test** — a folder created via the new API
  is a `Chat` row, not a separate table row:

```go
func TestTree_Create_MintsChatTypedFolder(t *testing.T) {
	uc := newTestUsecase(t)
	folder, err := uc.Create(ctx, CreateInput{RepoID: "repo-1", Name: "My Folder"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := uc.chats.Get(ctx, folder.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Type != domain.ChatTypeFolder {
		t.Fatalf("want folder type, got %s", got.Type)
	}
	if got.WorkspaceID != "" {
		t.Fatalf("a folder must not carry a workspace, got %q", got.WorkspaceID)
	}
}
```

- [ ] **Step 2: Run, verify fail** (compile failure — `CreateInput` and the
  new `Get` dependency don't line up with today's shape yet)

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/chat/internal/tree/... -v`

- [ ] **Step 3: Implement** — retype the storage:
  - `CreateInput` drops `WorkspaceID`, gains `RepoID`.
  - Every read/write goes through the `chat` repository's `Create`/`Get`/list
    calls (Task 3's `Type` field) instead of a `ChatFolder` store.
  - Delete `domain.ChatFolder`, its GORM table reference, and its DTO.
  - Delete `usecases/folder` and `endpoints/folders` outright — every route
    they served is superseded by the chat tree routes (stage 7 finishes the
    route-level cutover; this task only removes the now-dead Go packages so
    nothing references the deleted domain types).

- [ ] **Step 4: Run the full package + its former sibling's tests, verify
  pass, verify `usecases/folder`'s and `endpoints/folders`' test files are
  gone (not skipped)**

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/chat/... ./internal/domain/... -v`
Run: `cd api && go build -tags noEmbed ./... ` (confirms nothing still
imports the deleted packages)

- [ ] **Step 5: Commit**

```bash
git add -A api/internal/domain/ api/internal/app/usecases/chat/ api/internal/app/usecases/folder api/internal/api/v0/endpoints/folders api/internal/api/v0/dto/
git commit -m "feat(chat): fold Folder and ChatFolder into folder-typed chat rows"
```

---

## Stage 2 — `WorkspaceID` becomes optional and mutable

*Model spec §1.5, §7 stage 2. Depends on nothing; fixes a live bug before
anything else moves the field around.*

### Task 5: `Create` no longer requires `WorkspaceID`

**Files:**
- Modify: `api/internal/app/repositories/chat/internal/commands/create.go:32-34`
- Test: same package's `create_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestCreate_Validate_AllowsEmptyWorkspaceID(t *testing.T) {
	c := Create{ID: "chat-1", Type: domain.ChatTypeChat}
	if err := c.Validate(); err != nil {
		t.Fatalf("a bubble chat must be creatable with no workspace: %v", err)
	}
}
```

- [ ] **Step 2: Run, verify fail** — today's `Validate` rejects this.

- [ ] **Step 3: Implement** — change the guard from
  `if c.ID == "" || c.WorkspaceID == ""` to `if c.ID == ""`.

- [ ] **Step 4: Run full package, verify pass**

Run: `cd api && go test -tags noEmbed ./internal/app/repositories/chat/... -v`

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/repositories/chat/internal/commands/create.go api/internal/app/repositories/chat/internal/commands/create_test.go
git commit -m "fix(chat): allow a chat row to be created with no workspace"
```

### Task 6: New `SetWorkspace` command

**Files:**
- Create: `api/internal/app/repositories/chat/internal/commands/set_workspace.go`
- Test: `api/internal/app/repositories/chat/internal/commands/set_workspace_test.go`

**Interfaces:**
- Produces: `SetWorkspace{ID, WorkspaceID string}` — consumed by Stage 5's
  promotion task (fills the slot) and Stage 4's move task (a worktree row's
  reparent never changes `WorkspaceID`, but a bubble gaining worktree
  ownership does — this command is that write).

No such command exists today (confirmed by recon — zero hits for
`SetWorkspace` anywhere in `internal/commands/`). Model it on the shape of
`set_placement.go`, the sibling command already in this package.

- [ ] **Step 1: Write the failing test**

```go
func TestSetWorkspace_EmitEvent_SetsWorkspaceID(t *testing.T) {
	chat := domain.Chat{ID: "chat-1"}
	cmd := SetWorkspace{ID: "chat-1", WorkspaceID: "ws-1"}
	updated := cmd.EmitEvent(chat)
	if updated.WorkspaceID != "ws-1" {
		t.Fatalf("want ws-1, got %q", updated.WorkspaceID)
	}
}

func TestSetWorkspace_Validate_RejectsEmptyID(t *testing.T) {
	cmd := SetWorkspace{ID: "", WorkspaceID: "ws-1"}
	if err := cmd.Validate(); err == nil {
		t.Fatalf("expected error for empty chat id")
	}
}
```

- [ ] **Step 2: Run, verify fail** — file doesn't exist yet.

- [ ] **Step 3: Implement**, matching `set_placement.go`'s structure exactly
  (same package, same command-interface shape the aggregate already dispatches
  on):

```go
package commands

import "github.com/char2cs/crowbar/api/internal/domain"

type SetWorkspace struct {
	ID          string
	WorkspaceID string
}

func (c SetWorkspace) Validate() error {
	if c.ID == "" {
		return ErrEmptyID
	}
	return nil
}

func (c SetWorkspace) EmitEvent(chat domain.Chat) domain.Chat {
	chat.WorkspaceID = c.WorkspaceID
	return chat
}
```

(Match `ErrEmptyID` to whatever sentinel `set_placement.go` already uses —
reuse it, don't mint a second one.)

- [ ] **Step 4: Run, verify pass**

Run: `cd api && go test -tags noEmbed ./internal/app/repositories/chat/... -v`

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/repositories/chat/internal/commands/set_workspace.go api/internal/app/repositories/chat/internal/commands/set_workspace_test.go
git commit -m "feat(chat): add SetWorkspace, the write §1.5's bug needs"
```

### Task 7: Re-key ledger and content storage onto the chat id

**Files:**
- Investigate first (the exact call site was not fully pinned by recon —
  `spawn.go`, `aliases.go`, `internal/shared/answerdesk/answerdesk.go`,
  `internal/shared/tools/toolset.go`, and `internal/tree/types.go` all
  reference `ledger.` — grep each for the actual `ledger.Open(...)` call and
  confirm which one derives its path from `Chat.WorkspaceID` via
  `worktreepath.ChatsDir`).
- Modify: whichever call site(s) the investigation finds.
- Modify: `api/internal/app/repositories/chat/activity/internal/store/internal/content/content.go`
  (confirmed location of payload-blob content storage — not the flatter
  `repositories/chat/internal/content/` the model spec's prose describes;
  that path doesn't exist) if it keys on workspace anywhere.
- Test: alongside whichever files change.

**Interfaces:**
- Consumes: `worktreepath.ChatsDir(worktreePath string) string`,
  `worktreepath.RunnerDir(chatsDir, runnerID, provider string) string`
  (`api/internal/app/usecases/internal/worktreepath/worktreepath.go:100,148`
  — confirmed real, unlike the model spec's illustrative
  `AgentLedgerDir`, which does not exist under that name anywhere).

- [ ] **Step 1: Investigate** — `grep -n "ledger.Open" api/internal/app/usecases/chat/aliases.go api/internal/app/usecases/chat/internal/runner/spawn.go api/internal/app/usecases/chat/internal/shared/answerdesk/answerdesk.go api/internal/app/usecases/chat/internal/shared/tools/toolset.go api/internal/app/usecases/chat/internal/tree/types.go`
  and read every match to confirm which one, if any, derives its directory
  from `chat.WorkspaceID` rather than `chat.ID`. Write down the exact
  file:line before touching anything.

- [ ] **Step 2: Write the failing test** — a ledger opened for a chat with no
  workspace (`WorkspaceID == ""`) must not error and must not collide with a
  ledger opened for the same chat after it's promoted (`WorkspaceID` set).
  Shape the test around whatever the investigation in Step 1 finds; the
  invariant under test is fixed regardless:

```go
func TestLedgerPath_StableAcrossPromotion(t *testing.T) {
	chatID := "chat-1"
	before := ledgerPathFor(domain.Chat{ID: chatID, WorkspaceID: ""})
	after := ledgerPathFor(domain.Chat{ID: chatID, WorkspaceID: "ws-1"})
	if before != after {
		t.Fatalf("ledger path must be a function of chat id, not workspace: %q != %q", before, after)
	}
}
```

(Name `ledgerPathFor` to match whatever the real function is called once
Step 1 identifies it — this is the one placeholder name in this plan that
depends on that investigation; replace it with the real name before writing
the test for real.)

- [ ] **Step 3: Implement** — change the derivation to key on `chat.ID`, never
  `chat.WorkspaceID`. If the current call site needs a worktree path to build
  a directory under (some ledgers may still want to live under the worktree
  for locality), key the leaf directory name on the chat id while the parent
  directory can still come from the workspace *when one exists*, falling back
  to a home-scoped default (`worktreepath.HomeDefaultChatsDir`, confirmed to
  exist) when it doesn't.

- [ ] **Step 4: Run the full chat usecase suite**

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/chat/... -v`

- [ ] **Step 5: Commit**

```bash
git add -A api/internal/app/usecases/chat/
git commit -m "fix(chat): key the ledger path on chat id, never workspace id"
```

---

## Stage 3 — Three walks, one tree

*Model spec §3.2, §3.3, §7 stage 3. Depends on Stage 1 (the taxonomy) and
Stage 2 (WorkspaceID optional). Everything downstream reads these walks.*

### Task 8: Unified tree built directly on `app/tree.Tree`

**Files:**
- Modify: `api/internal/app/usecases/chat/internal/tree/tree.go` (further, on
  top of Task 4's retyping)
- Delete: any bespoke `treeSnapshot`-style wrapper left over from the old
  `usecases/folder` package (Task 4 already deleted the package; this task
  makes sure nothing recreates its pattern)
- Test: `api/internal/app/usecases/chat/internal/tree/tree_walk_test.go`

**Interfaces:**
- Consumes: `tree.Tree` — the existing, generic, already-built planner
  (`api/internal/app/tree/tree.go`):
  ```go
  type Node struct { ID, ParentID string; Order int; CreatedAt time.Time }
  type Tree interface {
      Node(id string) (Node, bool)
      Members(container string) []Node
      NextSlot(container string) int
      IndexOf(container, id string) int
      Add(node Node)
      Drop(id string)
      SetParent(id, parentID string)
      Touch(id string)
      Reorder(container, placed string, target int)
      Reparent(id, destination string)
      Reaches(container, ancestor string) bool
      Dirty() []string
      Reparented(id string) bool
  }
  func New(nodes []Node) Tree
  ```
- Produces: three walk functions, consumed by Stage 4 (drop legality), Stage
  5 (`CreateChild`'s parent resolution), and Stage 7 (each row shipping its
  walks pre-resolved):
  ```go
  func CwdWorkspaceID(t tree.Tree, chats map[string]domain.Chat, rowID string) (string, bool)
  func ForkParentID(t tree.Tree, chats map[string]domain.Chat, rowID string) (string, bool)
  func ChatLineage(t tree.Tree, chats map[string]domain.Chat, rowID string) []string
  ```

Every `Chat` row (`chat`, `branch`, `folder`) becomes one `tree.Node` — build
the `[]tree.Node` slice by mapping `Chat{ID, ParentID, Order, CreatedAt}`
directly; there is no snapshot type to reinvent, `tree.Tree` already is one.

- [ ] **Step 1: Write the failing tests** — one per walk, covering the
  qualifier that makes each interesting:

```go
func TestCwdWorkspaceID_SkipsUnprovisionedAncestor(t *testing.T) {
	chats := map[string]domain.Chat{
		"root":   {ID: "root", Type: domain.ChatTypeBranch, WorkspaceID: "ws-root"},
		"folder": {ID: "folder", Type: domain.ChatTypeFolder, ParentID: "root"},
		"blocked": {ID: "blocked", Type: domain.ChatTypeBranch, ParentID: "folder", WorkspaceID: ""},
		"leaf":   {ID: "leaf", Type: domain.ChatTypeChat, ParentID: "blocked"},
	}
	tr := treeFrom(chats)
	got, ok := CwdWorkspaceID(tr, chats, "leaf")
	if !ok || got != "ws-root" {
		t.Fatalf("want ws-root (walk past the unprovisioned blocked row), got %q ok=%v", got, ok)
	}
}

func TestForkParentID_ExcludesSelf(t *testing.T) {
	chats := map[string]domain.Chat{
		"root": {ID: "root", Type: domain.ChatTypeBranch, WorkspaceID: "ws-root"},
		"self": {ID: "self", Type: domain.ChatTypeBranch, ParentID: "root", WorkspaceID: "ws-self"},
	}
	tr := treeFrom(chats)
	got, ok := ForkParentID(tr, chats, "self")
	if !ok || got != "root" {
		t.Fatalf("fork parent must exclude self, want root, got %q ok=%v", got, ok)
	}
}

func TestChatLineage_StopsAtNonChatRow(t *testing.T) {
	chats := map[string]domain.Chat{
		"folder": {ID: "folder", Type: domain.ChatTypeFolder},
		"parent": {ID: "parent", Type: domain.ChatTypeChat, ParentID: "folder"},
		"child":  {ID: "child", Type: domain.ChatTypeChat, ParentID: "parent"},
	}
	tr := treeFrom(chats)
	got := ChatLineage(tr, chats, "child")
	want := []string{"parent"}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("lineage must only include Type==chat ancestors, got %v", got)
	}
}
```

Add a small test helper in the same test file:

```go
func treeFrom(chats map[string]domain.Chat) tree.Tree {
	nodes := make([]tree.Node, 0, len(chats))
	for _, c := range chats {
		nodes = append(nodes, tree.Node{ID: c.ID, ParentID: c.ParentID, Order: c.Order, CreatedAt: c.CreatedAt})
	}
	return tree.New(nodes)
}
```

- [ ] **Step 2: Run, verify fail** — functions don't exist yet.

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/chat/internal/tree/... -run 'TestCwdWorkspaceID|TestForkParentID|TestChatLineage' -v`

- [ ] **Step 3: Implement** each walk as a small, pure function over
  `tree.Tree` + the chat map — no IO, no context needed (matches principle 3,
  no machinery in the usecase):

```go
func CwdWorkspaceID(t tree.Tree, chats map[string]domain.Chat, rowID string) (string, bool) {
	for id := rowID; id != ""; {
		if c, ok := chats[id]; ok && c.WorkspaceID != "" {
			return c.WorkspaceID, true
		}
		node, ok := t.Node(id)
		if !ok {
			break
		}
		id = node.ParentID
	}
	return "", false
}

func ForkParentID(t tree.Tree, chats map[string]domain.Chat, rowID string) (string, bool) {
	node, ok := t.Node(rowID)
	if !ok {
		return "", false
	}
	return CwdWorkspaceIDExcludingSelf(t, chats, node.ParentID)
}
```

(`CwdWorkspaceIDExcludingSelf` is `CwdWorkspaceID` starting from the parent —
implement it as `CwdWorkspaceID(t, chats, node.ParentID)`, i.e. `ForkParentID`
is exactly `CwdWorkspaceID` run from one node up. Don't duplicate the walk.)

```go
func ChatLineage(t tree.Tree, chats map[string]domain.Chat, rowID string) []string {
	var out []string
	node, ok := t.Node(rowID)
	if !ok {
		return out
	}
	for id := node.ParentID; id != ""; {
		c, ok := chats[id]
		if !ok {
			break
		}
		if c.Type != domain.ChatTypeChat {
			nextNode, ok := t.Node(id)
			if !ok {
				break
			}
			id = nextNode.ParentID
			continue
		}
		out = append(out, id)
		nextNode, ok := t.Node(id)
		if !ok {
			break
		}
		id = nextNode.ParentID
	}
	return out
}
```

- [ ] **Step 4: Run, verify pass**

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/chat/internal/tree/... -v`

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/chat/internal/tree/
git commit -m "feat(chat): the three walks — cwd, fork parent, chat lineage"
```

### Task 9: `Workspace.ParentID` becomes a maintained projection; `FolderID`/`Order` deleted

**Files:**
- Modify: `api/internal/domain/workspace.go` — remove `FolderID`, `Order`
  fields; `ParentID` stays but its doc comment changes to state it is derived,
  never authored directly.
- Modify: `api/internal/app/repositories/workspace/internal/commands/set_placement.go`
  — delete this command; folder placement for a workspace-owning row now goes
  through the chat aggregate's placement command exclusively.
- Modify: `api/internal/app/repositories/workspace/internal/commands/reparent.go`
  — this command's *meaning* changes from "move the workspace's git lineage
  and its sidebar placement together" to "recompute the projection after the
  chat aggregate's placement changed" — it becomes internal machinery invoked
  by the chat-side move (Stage 4), not a directly authored write.
- Test: `api/internal/app/repositories/workspace/internal/commands/reparent_test.go`

**Interfaces:**
- Consumes: `tree.ForkParentID` (Task 8).
- Produces: `Workspace.ParentID` stays readable by the three consumers the
  model spec names (merge eligibility, diff base, reparent leaf guard) —
  their call sites do not need to change, only how the field gets written
  does.

- [ ] **Step 1: Write the failing test** — the workspace repository refuses a
  direct external write to `ParentID` outside the reconciliation path:

```go
func TestWorkspace_ParentID_NotDirectlySettable(t *testing.T) {
	// set_placement.go must no longer exist as an exported command.
	var _ = commands.SetPlacement{} // this line must fail to compile once removed
}
```

(This is a compile-gate test, deliberately — its purpose is to force the
command's deletion; once `SetPlacement` is removed from the `workspace`
commands package this file itself won't compile and must be deleted as part
of Step 3, leaving only `reparent_test.go`'s real behavioral tests.)

- [ ] **Step 2: Write the real behavioral test**, replacing the compile-gate
  one once `SetPlacement` is gone:

```go
func TestReparent_ProjectsForkParentFromChatTree(t *testing.T) {
	ws := domain.Workspace{ID: "ws-1", ParentID: "old-parent"}
	cmd := Reparent{ID: "ws-1", NewForkParentID: "new-parent"}
	updated := cmd.EmitEvent(ws)
	if updated.ParentID != "new-parent" {
		t.Fatalf("want new-parent, got %q", updated.ParentID)
	}
}
```

- [ ] **Step 3: Implement**
  - Remove `FolderID`, `Order` from `domain.Workspace`.
  - Delete `set_placement.go` from the workspace repository's commands.
  - Rename `Reparent`'s field from whatever it's called today to
    `NewForkParentID` (make explicit that this is a projection write, not an
    authored placement) and update its call site — the one place it's invoked
    from becomes Stage 4's move verb, which calls it *after* computing the new
    fork parent via `tree.ForkParentID`, not from a route handler directly.

- [ ] **Step 4: Run**

Run: `cd api && go test -tags noEmbed ./internal/app/repositories/workspace/... ./internal/domain/... -v`
Run: `cd api && go build -tags noEmbed ./...` (catches every now-stale
`FolderID`/`Order` reference across the codebase — fix each compile error by
deleting the reference, not by re-adding the field)

- [ ] **Step 5: Commit**

```bash
git add -A api/internal/domain/workspace.go api/internal/app/repositories/workspace/
git commit -m "feat(workspace): ParentID becomes a projection; delete FolderID and Order"
```

---

## Stage 4 — Drop rules server-side, and the working-guard (includes the addendum)

*Model spec §4.3, invariant 5, §7 stage 4 + the 2026-08-28 addendum's
addition 2. Depends on Stage 3.*

### Task 10: `guardNotWorking` — the shared guard

**Files:**
- Create: `api/internal/app/usecases/chat/internal/tree/guard_not_working.go`
- Test: `api/internal/app/usecases/chat/internal/tree/guard_not_working_test.go`

**Interfaces:**
- Consumes: `inflight.Work` — specifically
  `(*turnstate.Work).Observe(chatID string) (working, known bool, changed <-chan struct{})`
  (`api/internal/app/usecases/chat/internal/shared/inflight/internal/turnstate/turnstate.go:225`,
  aliased as `inflight.Work` in `chat.go:38`). The chat usecase already holds
  one instance of this (`chat.go:240`, field `work *inflight.Work`) — this
  guard is a method reachable from that same usecase, not a new subsystem.
- Produces: `ErrSubtreeWorking` sentinel, `guardNotWorking(subtreeIDs []string, work *inflight.Work) error`
  — consumed by Task 11 (move) and Stage 7's delete verb.

- [ ] **Step 1: Write the failing test**

```go
func TestGuardNotWorking_RefusesWhenAnyRowWorking(t *testing.T) {
	work := inflight.NewWork()
	work.Set("child-2", true)
	err := guardNotWorking([]string{"root", "child-1", "child-2"}, work)
	if !errors.Is(err, ErrSubtreeWorking) {
		t.Fatalf("want ErrSubtreeWorking, got %v", err)
	}
}

func TestGuardNotWorking_AllowsWhenSubtreeIdle(t *testing.T) {
	work := inflight.NewWork()
	err := guardNotWorking([]string{"root", "child-1"}, work)
	if err != nil {
		t.Fatalf("unexpected error on an idle subtree: %v", err)
	}
}
```

- [ ] **Step 2: Run, verify fail** — file doesn't exist.

- [ ] **Step 3: Implement**

```go
package tree

import (
	"errors"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
)

var ErrSubtreeWorking = errors.New("tree: a working chat refuses this verb")

func guardNotWorking(subtreeIDs []string, work *inflight.Work) error {
	for _, id := range subtreeIDs {
		if working, _, _ := work.Observe(id); working {
			return ErrSubtreeWorking
		}
	}
	return nil
}
```

- [ ] **Step 4: Run, verify pass**

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/chat/internal/tree/... -v`

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/chat/internal/tree/guard_not_working.go api/internal/app/usecases/chat/internal/tree/guard_not_working_test.go
git commit -m "feat(chat): guardNotWorking — the shared refusal for move and delete"
```

### Task 11: Move takes the subtree; working and cross-repo refusals; wired server-side

**Files:**
- Modify: `api/internal/app/usecases/chat/internal/tree/tree.go` (the placement
  verb, extended from Task 4/3.1) — becomes the one place placement legality
  is decided.
- Modify: `api/internal/app/usecases/worktree/worktree.go:1238` (`guardReparent`)
  — add the working-guard call and the cross-repo-only-if-no-worktree rule
  from model spec invariant 7.
- Test: `api/internal/app/usecases/chat/internal/tree/move_test.go`,
  `api/internal/app/usecases/worktree/worktree_test.go` (extend existing)

**Interfaces:**
- Consumes: `guardNotWorking` (Task 10), `tree.ChatLineage`/`ForkParentID`
  (Task 8), `cascade.Plan(rootID string, all []cascade.Node) []string`
  (`api/internal/app/usecases/internal/cascade/cascade.go:20` — confirmed
  existing, deepest-first post-order DFS, skip-locked-but-descend) reused
  directly to compute the subtree a move or delete takes, fed
  `cascade.Node{ID, Parent, Locked}` built from the unified `Chat` rows
  instead of `domain.Workspace` rows.

- [ ] **Step 1: Write the failing tests**

```go
func TestMove_RefusesWorkingSubtree(t *testing.T) {
	uc := newTestUsecase(t)
	uc.work.Set("child-1", true)
	_, err := uc.Move(ctx, "root", MoveInput{NewParentID: "other"})
	if !errors.Is(err, ErrSubtreeWorking) {
		t.Fatalf("want ErrSubtreeWorking, got %v", err)
	}
}

func TestMove_TakesWholeSubtree(t *testing.T) {
	uc := newTestUsecase(t)
	seedChats(t, uc, "root", "child-1", "child-2")
	_, err := uc.Move(ctx, "root", MoveInput{NewParentID: "other"})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	for _, id := range []string{"root", "child-1", "child-2"} {
		got, _ := uc.chats.Get(ctx, id)
		if !strings.HasPrefix(got.ParentID, "other") && id != "root" {
			continue // children keep pointing at root, root repoints to other — assert on root only below
		}
	}
	root, _ := uc.chats.Get(ctx, "root")
	if root.ParentID != "other" {
		t.Fatalf("root did not move")
	}
}

func TestGuardReparent_CrossRepoOnlyLegalWithoutWorktree(t *testing.T) {
	u := newTestWorktreeUsecase(t)
	child := domain.Workspace{ID: "child", RepoID: "repo-a", WorktreePath: "/some/path"}
	newParent := domain.Workspace{ID: "parent", RepoID: "repo-b", WorktreePath: "/other/path"}
	err := u.guardReparent(ctx, child, newParent)
	if err == nil {
		t.Fatalf("expected refusal: cross-repo move of a worktree-owning row")
	}
}
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**
  - In the tree package's move verb: compute the subtree via
    `cascade.Plan`, call `guardNotWorking` over it before any write.
  - In `guardReparent` (`worktree.go:1238`): add the same `guardNotWorking`
    call (this usecase reaches the chat package's `inflight.Work` through
    whatever shared dependency wiring `app/container.go` already uses to hand
    the chat usecase to sibling usecases — check `container.go` for the
    existing pattern before inventing a new one), and add:
    ```go
    if child.RepoID != newParent.RepoID && child.WorktreePath != "" {
        return ErrCrossRepoWorktreeMove
    }
    ```

- [ ] **Step 4: Run**

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/chat/... ./internal/app/usecases/worktree/... -v`

- [ ] **Step 5: Commit**

```bash
git add -A api/internal/app/usecases/chat/internal/tree/ api/internal/app/usecases/worktree/worktree.go
git commit -m "feat(chat,worktree): move takes the subtree, refuses working and illegal cross-repo"
```

---

## Stage 5 — One `CreateChild`, promotion, provisional branch names

*Model spec §4.1, §4.2, §7 stage 5. Depends on Stage 2 and Stage 3.*

### Task 12: `CreateChild` collapses onto the model spec's shape

**Files:**
- Modify: `api/internal/app/usecases/worktree/worktree.go:27-42`
  (`CreateChildInput`) and `:185` (`CreateChild`).

Today's `CreateChildInput` already has `ParentID`, but also `RepoID`,
`ProjectID`, `RepoPath`, `RemoteURL`, `Branch`, `ParentBranch`, `FolderID`,
`Order` — several of which the model spec's target shape
(`{ParentID, OwnWorktree, ProviderID}`) doesn't carry, because they're now
derivable from the walk (Task 8) or from the taxonomy default rule ("the
default is inherited from the parent") instead of being passed in. `FolderID`
in particular is dead per Stage 3.2 — placement is a tree operation now, not a
constructor argument.

- [ ] **Step 1: Write the failing test**

```go
func TestCreateChild_DefaultsOwnWorktreeFromParent(t *testing.T) {
	u := newTestWorktreeUsecase(t)
	seedLockedBranch(t, u, "locked-parent")
	child, err := u.CreateChild(ctx, CreateChildInput{ParentID: "locked-parent"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if child.WorktreePath == "" {
		t.Fatalf("a child of a locked row must default to owning a worktree")
	}
}

func TestCreateChild_BubbleUnderBubbleStaysBubble(t *testing.T) {
	u := newTestWorktreeUsecase(t)
	seedBubbleChat(t, u, "bubble-parent")
	child, err := u.CreateChild(ctx, CreateChildInput{ParentID: "bubble-parent"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if child.WorktreePath != "" {
		t.Fatalf("a child of a workspace-less chat must default to no workspace")
	}
}
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**
  - Reduce `CreateChildInput` to `{ParentID string; OwnWorktree *bool;
    ProviderID string}` (a pointer so "unset" is distinguishable from
    "explicitly false" — the default-inherited rule only applies when unset).
  - Resolve `RepoID`/`ProjectID`/`RepoPath`/`RemoteURL`/`ParentBranch` from the
    parent row via `tree.ForkParentID` (Task 8) instead of accepting them as
    caller-supplied.
  - When `OwnWorktree` is nil, inherit from the parent: a row under a
    workspace-owning ancestor (`ForkParentID` resolves to a real workspace)
    defaults `true`; otherwise `false`.

- [ ] **Step 4: Run**

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/worktree/... -v`

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/worktree/worktree.go
git commit -m "feat(worktree): CreateChild collapses to {ParentID, OwnWorktree, ProviderID}"
```

### Task 13: Generated provisional branch names, `set_branch_name`

**Files:**
- Create: `api/internal/app/usecases/chat/internal/shared/tools/tools_branch_name.go`
  (sibling to `tools_chat.go`, which already implements `set_chat_title` —
  mirror it exactly)
- Test: `api/internal/app/usecases/chat/internal/shared/tools/tools_branch_name_test.go`

**Interfaces:**
- Consumes: the same `ChatRenamer`-shaped port pattern
  `tools_chat.go` uses (`RenameByRunner(ctx, runnerID, title, source string) error`)
  — this task's port is the workspace-side equivalent, name it
  `WorkspaceBranchRenamer` per the model spec §4.1's own naming.
- Produces: the `set_branch_name` MCP tool, `RenameBranch` (already noted by
  the model spec as safe to call from inside the branch's own worktree — "a
  git ref rename and one record write, nothing on disk is touched"; confirm
  this function already exists under `usecases/worktree` — recon did not
  locate it explicitly, so Step 1 below starts with that check).

- [ ] **Step 1: Confirm `RenameBranch` exists**

Run: `grep -n "func.*RenameBranch" api/internal/app/usecases/worktree/rename_branch.go`

If it exists (the file `rename_branch.go` was confirmed present by recon),
read its signature and reuse it verbatim from the new tool. If its signature
doesn't match `(ctx, workspaceID, newBranch string) error`, adapt the tool
call to whatever it actually is — do not change `RenameBranch` itself, this
task only adds a caller.

- [ ] **Step 2: Write the failing test**, mirroring `tools_chat.go`'s own test
  file:

```go
func TestSetBranchName_RejectsEmptyName(t *testing.T) {
	tool := NewSetBranchNameTool(fakeRenamer{})
	_, err := tool.Handle(ctx, SetBranchNameInput{Name: ""})
	if err == nil {
		t.Fatalf("expected error: agenttools: set_branch_name: name must not be empty")
	}
}

func TestSetBranchName_CallsRenameByRunner(t *testing.T) {
	renamer := &fakeRenamer{}
	tool := NewSetBranchNameTool(renamer)
	_, err := tool.Handle(ctx, SetBranchNameInput{Name: "fix-the-thing"})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if renamer.calledWith != "fix-the-thing" {
		t.Fatalf("renamer not called with the given name")
	}
}
```

- [ ] **Step 3: Implement**, copying `tools_chat.go`'s structure field for
  field: tool name `"set_branch_name"`, description *"Rename this workspace's
  branch. Call once the task is achieved — the branch name should describe
  what shipped, not what was asked."*, input schema
  `{"properties":{"name":{"type":"string"}},"required":["name"]}`, guard on
  empty name with the matching error-message convention
  (`"agenttools: set_branch_name: name must not be empty"`).

- [ ] **Step 4: Run**

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/chat/internal/shared/tools/... -v`

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/chat/internal/shared/tools/tools_branch_name.go api/internal/app/usecases/chat/internal/shared/tools/tools_branch_name_test.go
git commit -m "feat(chat): set_branch_name, mirroring set_chat_title's precedence"
```

### Task 14: Promotion — fill the slot, respawn via the existing handoff machinery

**Files:**
- Create: `api/internal/app/usecases/chat/internal/tree/promote.go`
- Test: `api/internal/app/usecases/chat/internal/tree/promote_test.go`

**Interfaces:**
- Consumes:
  `SwitchProvider(ctx context.Context, chatID string, targetProviderID string) (string, error)`
  (`api/internal/app/usecases/chat/internal/runner/switch.go`, confirmed to
  take the spawn gate itself — call this, never `switchProviderLocked`
  directly, or a promotion invoked from inside `ResumeChat` deadlocks per that
  file's own doc comment) and
  `AssembleHandoff(ctx context.Context, chatID string) (string, error)`
  (`api/internal/app/usecases/chat/internal/conversation/handoff.go`,
  confirmed to exist with exactly this signature).
- Produces: `Promote(ctx, chatID string) (domain.Chat, error)` — the route
  handler stage 7 adds calls this directly.

- [ ] **Step 1: Write the failing test**

```go
func TestPromote_FillsWorkspaceKeepsIdentity(t *testing.T) {
	uc := newTestUsecase(t)
	chat := seedBubbleChat(t, uc, "bubble-1")
	promoted, err := uc.Promote(ctx, chat.ID)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if promoted.ID != chat.ID {
		t.Fatalf("promotion must keep the chat's id")
	}
	if promoted.WorkspaceID == "" {
		t.Fatalf("promotion must fill WorkspaceID")
	}
	if promoted.Title != chat.Title {
		t.Fatalf("promotion must keep the chat's title")
	}
}
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**

```go
func (u *usecase) Promote(ctx context.Context, chatID string) (domain.Chat, error) {
	chat, err := u.chats.Get(ctx, chatID)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("promote: get: %w", err)
	}
	forkParentID, ok := ForkParentID(u.tree, u.chatsByID(ctx), chatID)
	if !ok {
		return domain.Chat{}, ErrNoForkParent
	}
	ws, err := u.worktrees.CreateChild(ctx, worktree.CreateChildInput{ParentID: forkParentID})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("promote: create workspace: %w", err)
	}
	if err := u.chats.Dispatch(ctx, commands.SetWorkspace{ID: chatID, WorkspaceID: ws.ID}); err != nil {
		return domain.Chat{}, fmt.Errorf("promote: set workspace: %w", err)
	}
	handoff, err := u.conversations.AssembleHandoff(ctx, chatID)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("promote: assemble handoff: %w", err)
	}
	if _, err := u.runners.SwitchProvider(ctx, chatID, chat.ProviderID(handoff)); err != nil {
		return domain.Chat{}, fmt.Errorf("promote: respawn: %w", err)
	}
	return u.chats.Get(ctx, chatID)
}
```

(`chat.ProviderID(handoff)` above is illustrative of intent — the real
provider-selection call already exists somewhere in `SwitchProvider`'s
callers today; find and reuse the exact existing pattern for "which provider
does this chat resume as" rather than inventing a new one. Read
`internal/runner/switch.go`'s other call sites before finalizing this line.)

Append the `[Crowbar]` ledger note the model spec's §4.2 requires, matching
`lineageNoteText`'s existing convention for a lineage change (find and reuse
that function; it already exists per the model spec's own citation).

- [ ] **Step 4: Run**

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/chat/... -v`

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/chat/internal/tree/promote.go api/internal/app/usecases/chat/internal/tree/promote_test.go
git commit -m "feat(chat): promotion fills the workspace slot via the existing handoff respawn"
```

---

## Stage 6 — Merge `usecases/worktree` + `usecases/workspace`; promote `worktreepath`

*Model spec §7 stage 6. Independent of stages 1-5; can run in parallel with
them. Sequenced last here only because this plan is written to be executed
roughly in order — a subagent-driven executor may run this stage's tasks
concurrently with stages 1-5 if two workers aren't touching the same files at
once.*

### Task 15: Promote `worktreepath` to `core/paths`

**Files:**
- Move: `api/internal/app/usecases/internal/worktreepath/` →
  `api/internal/core/paths/worktreepath/`
- Modify: every import of the old path (`grep -rl
  "app/usecases/internal/worktreepath" api/internal`) to the new one.
- Test: the package's existing tests move with it, unchanged in content.

- [ ] **Step 1: Confirm the full list of importers**

Run: `grep -rl "app/usecases/internal/worktreepath" api/internal`

- [ ] **Step 2: Move the package** (file move, not a rewrite — content is
  unchanged)

```bash
mkdir -p api/internal/core/paths
git mv api/internal/app/usecases/internal/worktreepath api/internal/core/paths/worktreepath
```

- [ ] **Step 3: Update every import path found in Step 1** to
  `github.com/char2cs/crowbar/api/internal/core/paths/worktreepath`.

- [ ] **Step 4: Run**

Run: `cd api && go build -tags noEmbed ./... && go test -tags noEmbed ./... `

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor(paths): promote worktreepath to core/paths, the last filesystem package to leave usecases"
```

### Task 16: Merge `usecases/worktree` and `usecases/workspace`

**Files:**
- Move: `api/internal/app/usecases/worktree/worktree.go` (and every sibling
  file) into `api/internal/app/usecases/workspace/internal/hierarchy/`
  (fork point, merge eligibility, cascade — per the model spec's own target
  tree §5).
- Move: `api/internal/app/usecases/worktree/import.go`,
  `merge_result.go`, `rename_branch.go` alongside it.
- Modify: `api/internal/app/usecases/workspace/workspace.go` — becomes the
  one public face; its existing methods (`List, ListInRepo, Get,
  SetMergeStrategy, SetLock, SyncWorkingTreeState, ResolveConflicts,
  MergeEligibilityFor`) plus the moved worktree methods
  (`CreateChild, MergeIntoParent, Reparent, RebaseOntoParent, RetryProvision,
  DetachHolder, DeleteCascade, DeleteRepoWorkspaces`) are exposed from this
  one file; the `internal/hierarchy/` package holds the implementation.
- Delete: `api/internal/app/usecases/worktree/` once empty.
- Test: move every existing `worktree_*_test.go` alongside its source; keep
  them passing unmodified in content (this is a pure relocation task, not a
  behavior change — if a test needs to change, that's a sign this task's scope
  crept into another stage's work).

- [ ] **Step 1: List every `worktree` package importer**

Run: `grep -rl "app/usecases/worktree\"" api/internal`

- [ ] **Step 2: Move the files**

```bash
mkdir -p api/internal/app/usecases/workspace/internal/hierarchy
git mv api/internal/app/usecases/worktree/*.go api/internal/app/usecases/workspace/internal/hierarchy/
git mv api/internal/app/usecases/worktree/internal/* api/internal/app/usecases/workspace/internal/hierarchy/internal/ 2>/dev/null || true
```

- [ ] **Step 3: Update `workspace.go`** to embed/delegate to
  `hierarchy.New(...)`, exposing every moved method on `workspaceUsecase`
  directly (a thin delegation, not a re-implementation — the interface
  callers already use should not change shape, only which package answers
  it).

- [ ] **Step 4: Update every importer found in Step 1** to import
  `usecases/workspace` instead, and to call the method on the workspace
  usecase instance instead of a separate worktree one — this is the step
  where `container.go`'s wiring collapses two usecase instances into one.

- [ ] **Step 5: Run**

Run: `cd api && go build -tags noEmbed ./... && go test -tags noEmbed -race ./... `

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor(workspace): merge usecases/worktree into usecases/workspace/internal/hierarchy"
```

---

## Stage 7 — Routes, and the addendum's two additions

*Model spec §5.1, §7 stage 7. Depends on stages 1-6. Breaks every agent URL;
frontend clients update in the same commit (coordinate with the frontend
plan's final task before merging this stage).*

### Task 17: Rescope chat routes off `/workspaces/:wsId/`

**Files:**
- Modify: `api/internal/api/v0/endpoints/chat/routes.go` — every route in the
  confirmed list (`POST /chats`, `GET /chats`, ... through `GET /chats/ws`)
  moves from being mounted under `wsScoped` to being mounted under a
  repo-scoped router (`/repos/:rid/chats/...`), matching the model spec's
  wire contract.
- Modify: `api/internal/api/v0/endpoints/chat/internal/handlers/*.go` —
  every handler that reads `:wsId` from the route now reads `:rid`
  (repo id) and, where a specific chat's resolved workspace is needed,
  resolves it via `tree.CwdWorkspaceID` (Task 8) instead of trusting the
  URL.
- Test: `api/internal/api/v0/endpoints/chat/internal/handlers/*_test.go` —
  update route paths in every existing handler test; add one asserting the
  old `/workspaces/:wsId/chats` shape now 404s.

- [ ] **Step 1: Write the failing test**

```go
func TestChatRoutes_MountedUnderRepo(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v0/repos/repo-1/chats", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code == http.StatusNotFound {
		t.Fatalf("chats must be reachable under /repos/:rid/chats")
	}
}

func TestChatRoutes_OldWorkspaceScopedPathGone(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v0/workspaces/ws-1/chats", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("the old workspace-scoped chat path must be gone, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement** — change `Register`'s mount point and every
  handler's id-extraction call from `chi.URLParam(r, "wsId")` to
  `chi.URLParam(r, "rid")`, threading the resolved workspace through
  `tree.CwdWorkspaceID` wherever a handler needs one for a specific chat.

- [ ] **Step 4: Run the full endpoint test suite**

Run: `cd api && go test -tags noEmbed ./internal/api/... -v`

- [ ] **Step 5: Commit**

```bash
git add -A api/internal/api/v0/endpoints/chat/
git commit -m "feat(chat): rescope chat routes onto /repos/:rid/, off /workspaces/:wsId/"
```

### Task 18: `GET /repos/:rid/chats/:id/delete-preview`

**Files:**
- Create: `api/internal/api/v0/endpoints/chat/internal/handlers/delete_preview.go`
- Modify: `api/internal/api/v0/endpoints/chat/routes.go` (register the route)
- Create: `api/internal/app/usecases/chat/internal/tree/delete_preview.go`
- Test: alongside each new file.

**Interfaces:**
- Consumes: `cascade.Plan` (confirmed, Task 11), git status per workspace —
  find the existing per-workspace status call (`WorkingTreeSummary`,
  confirmed at `worktree.go:898` pre-move, now under
  `workspace/internal/hierarchy/` per Stage 6) and sum its `added`/`deleted`
  fields across every workspace-owning row in the subtree.
- Produces: `DeletePreview(ctx, chatID string) (chatCount, fileCount int, err error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestDeletePreview_SumsFileCountsAcrossSubtree(t *testing.T) {
	uc := newTestUsecase(t)
	seedFolderWithTwoWorktreeChats(t, uc, "folder-1", "chat-a", "chat-b")
	setWorkingTreeSummary(t, uc, "chat-a", 3, 1)
	setWorkingTreeSummary(t, uc, "chat-b", 2, 0)
	chatCount, fileCount, err := uc.DeletePreview(ctx, "folder-1")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if chatCount != 2 {
		t.Fatalf("want 2 chats, got %d", chatCount)
	}
	if fileCount != 6 {
		t.Fatalf("want 6 (3+1+2+0), got %d", fileCount)
	}
}
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**

```go
func (u *usecase) DeletePreview(ctx context.Context, chatID string) (int, int, error) {
	subtree, err := u.subtreeOf(ctx, chatID)
	if err != nil {
		return 0, 0, fmt.Errorf("delete preview: subtree: %w", err)
	}
	chatCount := 0
	fileCount := 0
	for _, row := range subtree {
		if row.Type == domain.ChatTypeChat {
			chatCount++
		}
		if row.WorkspaceID == "" {
			continue
		}
		added, deleted, err := u.workspaces.WorkingTreeSummary(ctx, row.WorkspaceID)
		if err != nil {
			return 0, 0, fmt.Errorf("delete preview: git status for %s: %w", row.WorkspaceID, err)
		}
		fileCount += added + deleted
	}
	return chatCount, fileCount, nil
}
```

- [ ] **Step 4: Add the HTTP handler**

```go
func (h *Handler) DeletePreview(w http.ResponseWriter, r *http.Request) {
	chatID := chi.URLParam(r, "id")
	chatCount, fileCount, err := h.usecase.DeletePreview(r.Context(), chatID)
	if err != nil {
		respondError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, deletePreviewResponse{ChatCount: chatCount, FileCount: fileCount})
}
```

(Match `respondError`/`respondJSON`'s real names to whatever helper this
handlers package already uses — every other handler in the package follows
one convention; copy it exactly rather than inventing a second one.)

- [ ] **Step 5: Register the route** in `routes.go`:
  `GET /chats/:id/delete-preview  h.DeletePreview` (mounted under the same
  repo-scoped router Task 17 set up).

- [ ] **Step 6: Run**

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/chat/... ./internal/api/... -v`

- [ ] **Step 7: Commit**

```bash
git add -A api/internal/app/usecases/chat/internal/tree/delete_preview.go api/internal/api/v0/endpoints/chat/
git commit -m "feat(chat): delete-preview aggregates chat and file counts across a subtree"
```

### Task 19: Delete verb wired to `guardNotWorking`, `cascade.Plan`

**Files:**
- Modify: `api/internal/api/v0/endpoints/chat/internal/handlers/delete.go`
  (today's `h.Delete`, confirmed to already exist — this task rewires it, it
  does not create it) and its backing usecase method.
- Test: alongside.

- [ ] **Step 1: Write the failing test**

```go
func TestDelete_RefusesWorkingSubtree_Unconditionally(t *testing.T) {
	uc := newTestUsecase(t)
	uc.work.Set("chat-1", true)
	err := uc.Delete(ctx, "chat-1")
	if !errors.Is(err, tree.ErrSubtreeWorking) {
		t.Fatalf("want ErrSubtreeWorking, got %v", err)
	}
}

func TestDelete_TakesTheSubtree(t *testing.T) {
	uc := newTestUsecase(t)
	seedFolderWithTwoWorktreeChats(t, uc, "folder-1", "chat-a", "chat-b")
	if err := uc.Delete(ctx, "folder-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	for _, id := range []string{"folder-1", "chat-a", "chat-b"} {
		if _, err := uc.chats.Get(ctx, id); err == nil {
			t.Fatalf("%s should be gone", id)
		}
	}
}
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**

```go
func (u *usecase) Delete(ctx context.Context, chatID string) error {
	subtree, err := u.subtreeOf(ctx, chatID)
	if err != nil {
		return fmt.Errorf("delete: subtree: %w", err)
	}
	ids := make([]string, len(subtree))
	for i, row := range subtree {
		ids[i] = row.ID
	}
	if err := guardNotWorking(ids, u.work); err != nil {
		return err
	}
	order := cascade.Plan(chatID, nodesFrom(subtree))
	for _, id := range order {
		if err := u.removeOne(ctx, id); err != nil {
			return fmt.Errorf("delete: remove %s: %w", id, err)
		}
	}
	return nil
}
```

`removeOne` handles both cases per row: a `folder`/`branch`-typed row with no
workspace is a bare aggregate delete; a row owning a workspace also calls
through to the (post-Stage-6) workspace usecase's teardown. There is no
confirm-and-override path — per the addendum, this refusal has no bypass.

- [ ] **Step 4: Run**

Run: `cd api && go test -tags noEmbed ./internal/app/usecases/chat/... ./internal/api/... -v`

- [ ] **Step 5: Commit**

```bash
git add -A api/internal/app/usecases/chat/ api/internal/api/v0/endpoints/chat/
git commit -m "feat(chat): delete takes the subtree, refuses a working row unconditionally"
```

### Task 20: `endpoints/workspaces` loses `/reparent`; resource-only `Detail`

**Files:**
- Modify: `api/internal/api/v0/endpoints/workspaces/routes.go` — remove
  `POST /workspaces/:wsId/reparent` (its git-lineage effect now happens
  inside the chat tree's move verb, Task 11, which calls the workspace
  repository's `Reparent` command internally rather than exposing it as its
  own route).
- Test: update the route-list test to assert the route is gone.

- [ ] **Step 1: Write the failing test**

```go
func TestWorkspaceRoutes_ReparentRemoved(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v0/repos/repo-1/workspaces/ws-1/reparent", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("reparent must no longer be a standalone workspace route, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run, verify fail** (route still exists today)

- [ ] **Step 3: Remove the route registration and its handler** from
  `endpoints/workspaces`.

- [ ] **Step 4: Run**

Run: `cd api && go test -tags noEmbed ./internal/api/... -v`

- [ ] **Step 5: Commit**

```bash
git add -A api/internal/api/v0/endpoints/workspaces/
git commit -m "feat(workspaces): remove the standalone reparent route, folded into chat placement"
```

---

## Stage 8 — Full-gate verification

*Not in the model spec's own stage table — added here because this plan's
Global Constraints require every stage to leave the full CI gate green, and a
final pass catches drift between stages.*

### Task 21: Full gate, coverage, integration suite

- [ ] **Step 1:** `cd api && make pr-checks` (lint, test, test-coverage) —
  must be clean.
- [ ] **Step 2:** `cd api && go vet -tags noEmbed ./...` — must be clean.
- [ ] **Step 3:** `cd api && go test -tags 'integration noEmbed' -race -v -timeout 600s -p 1 ./...`
  — must be clean. If no integration subpackage exists yet for the sidebar
  forest (recon found none under `api/tests/integration/` — the closest
  existing ones are `worktree`, `paths`, `threads`), add
  `api/tests/integration/tree/sidebar_forest_test.go` covering: create a
  folder, create two chats under it (one a worktree, one a bubble), move the
  folder, delete it, assert the whole subtree is gone and a working chat
  inside it refuses the delete first.
- [ ] **Step 4:** Confirm every §6 invariant from the model spec (1-8) and the
  addendum's invariant 9 has a test that fails when inverted — grep each
  invariant's keyword across the new test files written in this plan and
  confirm coverage; write the one missing if any stage above didn't produce
  it directly.
- [ ] **Step 5: Commit** (only if Step 3 required a new integration test file)

```bash
git add -A api/tests/integration/tree/
git commit -m "test(tree): integration coverage for the unified sidebar forest"
```

---

## Self-review notes (from writing this plan)

- **Spec coverage:** every §6/§7 item in the model spec has a task above
  except §7's frontend-only stage 8, which belongs to the companion frontend
  plan, not this one — see
  `docs/superpowers/plans/2026-08-28-sidebar-frontend-implementation.md`.
- **Known open item deliberately left for the executor, not this plan:** the
  model spec's §10 open question 1 ("is `chat` still the right name?") is not
  resolved here — renaming the aggregate is out of scope for an already-large
  plan and nothing above depends on the answer.
- **Type consistency check:** `ForkParentID`, `CwdWorkspaceID`, `ChatLineage`
  (Task 8) are used with identical signatures in Tasks 4.2, 5.1, 5.3, and
  7.1 — confirmed consistent throughout.
