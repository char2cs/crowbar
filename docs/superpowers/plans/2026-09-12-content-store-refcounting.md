# Content-Store Ownership Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a chat is forgotten, the tool-call payload blobs it wrote to `state/content` must be deleted too — deterministically, in the same call, by the same code that owns them — unless another chat still references the identical blob (content is deduplicated by hash, so that does happen).

**Architecture:** No background GC. `content.Store` gains a `Delete(ref) error`. `eventSourced.Forget` (the only place a chat's activity rows are erased) collects the chat's own `RequestRef`/`ResultRef`s *before* deleting its rows, deletes the rows, then for each collected ref checks — via one indexed SQL query against the `tool_call_rows` table, which still holds every OTHER chat's rows — whether any surviving row still points at it; if none does, it deletes the blob. This is a liveness check computed synchronously at the one moment it's needed, not a persisted counter (nothing to drift) and not a periodic sweep (nothing runs in the background).

**Tech Stack:** Go, GORM, testify.

**Spec:** This plan's own Goal/Architecture above. Supersedes the earlier "mark-and-sweep GC" framing — the user's explicit direction was that each module must take responsibility for its own children, proven by tests, not rely on a periodic reconciliation pass.

## Global Constraints

- `content.Store` (`.../activity/internal/store/internal/content/content.go`) is a pure hash-addressed blob store with no knowledge of who references what — ownership/liveness lives one layer up, in `internal/storage` (the GORM-backed `ToolCallRow` table), which already has `chat_id`, `request_ref`, `result_ref` columns.
- `ToolCallRow.RequestRef`/`ResultRef` are the ONLY fields anywhere that reference a content blob (confirmed: `grep -n "Ref " api/internal/domain/chat_activity.go` finds no other `*Ref` field on any other activity type).
- Never delete a blob without first confirming, via the SQL table (the durable source of truth), that no other row anywhere still points at it — the store dedupes identical payloads across chats.

---

### Task 1: `content.Store.Delete`

**Files:**
- Modify: `api/internal/app/repositories/chat/activity/internal/store/internal/content/content.go`
- Test: `api/internal/app/repositories/chat/activity/internal/store/internal/content/content_test.go`

**Interfaces:**
- Produces: `(s *Store) Delete(ref string) error` — removes the blob file for ref; a no-op (nil error) when ref is empty, malformed, or already gone.

- [ ] **Step 1: Write the failing test**

```go
func TestDelete_RemovesTheBlob(t *testing.T) {
	s := newStore(t)
	ref, err := s.Put([]byte("tool output"))
	require.NoError(t, err)

	require.NoError(t, s.Delete(ref))

	_, err = s.Get(ref)
	assert.ErrorIs(t, err, content.ErrNotFound)
}

func TestDelete_IsANoOpOnAnAlreadyMissingRef(t *testing.T) {
	s := newStore(t)
	ref, err := s.Put([]byte("tool output"))
	require.NoError(t, err)
	require.NoError(t, s.Delete(ref))

	assert.NoError(t, s.Delete(ref)) // second delete of the same ref
}

func TestDelete_IsANoOpOnAnEmptyOrMalformedRef(t *testing.T) {
	s := newStore(t)
	assert.NoError(t, s.Delete(""))
	assert.NoError(t, s.Delete("not-a-ref"))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/app/repositories/chat/activity/internal/store/internal/content/... -run TestDelete -v`
Expected: FAIL — `Delete` does not exist (compile error).

- [ ] **Step 3: Write minimal implementation**

```go
// Delete removes the blob behind ref, if any. A ref that is empty,
// malformed, or already gone is a no-op — Forget calls this after confirming
// no other chat's tool call still references it, and must not fail merely
// because two chats happened to race to delete the same already-shared,
// already-cleaned-up blob.
func (s *Store) Delete(ref string) error {
	path, _ := s.pathFor(ref)
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("agentactivity content: delete: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/app/repositories/chat/activity/internal/store/internal/content/... -v`
Expected: PASS, including every existing test in the package.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/repositories/chat/activity/internal/store/internal/content/content.go api/internal/app/repositories/chat/activity/internal/store/internal/content/content_test.go
git commit -m "feat(content): add Delete for a single blob"
```

---

### Task 2: `storage.Store.ToolCallRefs` + `RefInUse`

**Files:**
- Modify: `api/internal/app/repositories/chat/activity/internal/store/internal/storage/storage.go`
- Test: `api/internal/app/repositories/chat/activity/internal/store/internal/storage/storage_test.go` (check for an existing file first; extend it if present, create it following the package's existing test file's setup helper if one exists)

**Interfaces:**
- Consumes: `ToolCallRow` (existing, `rows.go`), `s.db *gorm.DB` (existing).
- Produces: `(s *Store) ToolCallRefs(ctx, chatID string) ([]string, error)` — every non-empty `RequestRef`/`ResultRef` any tool call row for chatID carries, BEFORE that chat's rows are deleted. `(s *Store) RefInUse(ctx, ref string) (bool, error)` — true iff some `ToolCallRow` anywhere still carries ref in `request_ref` or `result_ref`.

- [ ] **Step 1: Write the failing test**

```go
func TestToolCallRefs_ReturnsRequestAndResultRefsForOneChat(t *testing.T) {
	s := newTestStore(t) // use this package's existing test DB helper
	require.NoError(t, s.SaveToolCall(ctx, domain.ActivityToolCall{
		ID: "t1", ChatID: "c1", TurnID: "tu1", Seq: 1,
		RequestRef: "sha256:aaa", ResultRef: "sha256:bbb",
	}))
	require.NoError(t, s.SaveToolCall(ctx, domain.ActivityToolCall{
		ID: "t2", ChatID: "c1", TurnID: "tu1", Seq: 2,
		RequestRef: "sha256:ccc",
	}))

	refs, err := s.ToolCallRefs(ctx, "c1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"sha256:aaa", "sha256:bbb", "sha256:ccc"}, refs)
}

func TestRefInUse_TrueWhenAnotherChatStillReferencesIt(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.SaveToolCall(ctx, domain.ActivityToolCall{
		ID: "t1", ChatID: "c1", TurnID: "tu1", Seq: 1, RequestRef: "sha256:shared",
	}))
	require.NoError(t, s.SaveToolCall(ctx, domain.ActivityToolCall{
		ID: "t2", ChatID: "c2", TurnID: "tu2", Seq: 1, RequestRef: "sha256:shared",
	}))

	inUse, err := s.RefInUse(ctx, "sha256:shared")
	require.NoError(t, err)
	assert.True(t, inUse)
}

func TestRefInUse_FalseWhenNothingReferencesIt(t *testing.T) {
	s := newTestStore(t)
	inUse, err := s.RefInUse(ctx, "sha256:nobody-points-here")
	require.NoError(t, err)
	assert.False(t, inUse)
}
```

(Match this package's actual existing test helper name/signature for constructing a `*Store` over a real sqlite file or in-memory DB — check `storage_test.go` if it exists, or the sibling `queries_test.go`/whatever file already tests `ToolCalls`/`SaveTurn`, and use whatever `SaveToolCall`-equivalent method already exists rather than the name guessed here if it differs.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/app/repositories/chat/activity/internal/store/internal/storage/... -run "TestToolCallRefs|TestRefInUse" -v`
Expected: FAIL — both methods do not exist yet.

- [ ] **Step 3: Write minimal implementation**

Add to `storage.go`:

```go
// ToolCallRefs returns every non-empty RequestRef/ResultRef chatID's tool
// calls carry. Forget calls this BEFORE DeleteChat erases those rows, so it
// knows what to check for continued use afterward.
func (s *Store) ToolCallRefs(ctx context.Context, chatID string) ([]string, error) {
	var rows []ToolCallRow
	if err := s.db.WithContext(ctx).Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("agentactivity storage: tool call refs: %w", err)
	}
	refs := make([]string, 0, len(rows)*2)
	for _, r := range rows {
		if r.RequestRef != "" {
			refs = append(refs, r.RequestRef)
		}
		if r.ResultRef != "" {
			refs = append(refs, r.ResultRef)
		}
	}
	return refs, nil
}

// RefInUse reports whether any tool call row anywhere still references ref —
// the liveness check Forget runs, per ref, AFTER deleting the forgotten
// chat's own rows, so a shared (deduplicated) blob is never deleted out from
// under a chat that still legitimately points at it.
func (s *Store) RefInUse(ctx context.Context, ref string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&ToolCallRow{}).
		Where("request_ref = ? OR result_ref = ?", ref, ref).
		Limit(1).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("agentactivity storage: ref in use: %w", err)
	}
	return count > 0, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/app/repositories/chat/activity/internal/store/internal/storage/... -v`
Expected: PASS, full package.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/repositories/chat/activity/internal/store/internal/storage/storage.go api/internal/app/repositories/chat/activity/internal/store/internal/storage/storage_test.go
git commit -m "feat(activity storage): add ToolCallRefs and RefInUse for content cleanup"
```

---

### Task 3: Pass-throughs on the mid-level `store.Store`

**Files:**
- Modify: `api/internal/app/repositories/chat/activity/internal/store/store.go`

**Interfaces:**
- Consumes: `storage.Store.ToolCallRefs`/`RefInUse` (Task 2).
- Produces: `(s *Store) ToolCallRefs(ctx, chatID string) ([]string, error)` and `(s *Store) RefInUse(ctx, ref string) (bool, error)`, delegating to `s.storage` — mirroring how every other method on this `Store` (`Turns`, `ToolCalls`, `DeleteChat`, ...) already delegates.

- [ ] **Step 1: Write the failing test**

No new test file — this is a one-line pass-through mirroring an existing pattern (e.g. `Subagents`); Task 4's test on `eventSourced.Forget` exercises it end-to-end. Skip straight to implementation.

- [ ] **Step 2: N/A**

- [ ] **Step 3: Write minimal implementation**

Add next to the other delegating methods in `store.go`:

```go
func (s *Store) ToolCallRefs(ctx context.Context, chatID string) ([]string, error) {
	return s.storage.ToolCallRefs(ctx, chatID)
}

func (s *Store) RefInUse(ctx context.Context, ref string) (bool, error) {
	return s.storage.RefInUse(ctx, ref)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./api/internal/app/repositories/chat/activity/...`
Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/repositories/chat/activity/internal/store/store.go
git commit -m "feat(activity store): expose ToolCallRefs and RefInUse"
```

---

### Task 4: `Forget` deletes orphaned blobs

**Files:**
- Modify: `api/internal/app/repositories/chat/activity/activity.go`
- Test: `api/internal/app/repositories/chat/activity/activity_test.go` (check for an existing file first; extend if present)

**Interfaces:**
- Consumes: `s.store.ToolCallRefs`, `s.store.RefInUse`, `s.store.Content().Delete` (Tasks 1–3).
- Produces: `eventSourced.Forget` unchanged signature, new behavior: deletes every ref the forgotten chat owned that nothing else still references.

- [ ] **Step 1: Write the failing test**

```go
func TestForget_DeletesABlobNothingElseReferences(t *testing.T) {
	r := newTestEventSourced(t) // this package's existing constructor/fixture helper
	require.NoError(t, r.InvokeTool(ctx, ToolInput{ChatID: "c1", ToolID: "t1", Request: []byte("only c1 uses this")}))

	// Recover the ref Forget must clean up: read it back off the folded row.
	calls, err := r.ToolCalls(ctx, "c1", 0, 10)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	ref := calls[0].RequestRef
	require.NotEmpty(t, ref)

	require.NoError(t, r.Forget(ctx, "c1"))

	_, err = r.Payload(ctx, ref)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestForget_KeepsABlobAnotherChatStillReferences(t *testing.T) {
	r := newTestEventSourced(t)
	require.NoError(t, r.InvokeTool(ctx, ToolInput{ChatID: "c1", ToolID: "t1", Request: []byte("shared payload")}))
	calls, err := r.ToolCalls(ctx, "c1", 0, 10)
	require.NoError(t, err)
	ref := calls[0].RequestRef

	// A second chat's tool call happens to produce the identical payload, so
	// content-store dedup gives it the SAME ref.
	require.NoError(t, r.InvokeTool(ctx, ToolInput{ChatID: "c2", ToolID: "t2", Request: []byte("shared payload")}))

	require.NoError(t, r.Forget(ctx, "c1"))

	got, err := r.Payload(ctx, ref)
	require.NoError(t, err, "c2 still references this ref; Forget(c1) must not have deleted it")
	assert.Equal(t, "shared payload", string(got))
}
```

(Match whatever this package's actual test fixture/constructor helper is named — check `activity_test.go` if it exists, else the closest existing `_test.go` in this package for how an `eventSourced` gets built for a unit test, and use its real `ToolInput`/`InvokeTool` field names as already defined earlier in `activity.go`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/app/repositories/chat/activity/... -run TestForget -v`
Expected: `TestForget_DeletesABlobNothingElseReferences` FAILS — today's `Forget` never touches content, so the blob is still there after forgetting.

- [ ] **Step 3: Write minimal implementation**

```go
func (r *eventSourced) Forget(ctx context.Context, chatID string) error {
	refs, err := r.store.ToolCallRefs(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agentactivity: forget: collect refs: %w", err)
	}
	if err := r.store.DeleteChat(ctx, chatID); err != nil {
		return fmt.Errorf("agentactivity: forget rows: %w", err)
	}
	r.deleteOrphanedRefs(ctx, refs)
	if err := r.ax.Forget(ctx, chatID); err != nil {
		return fmt.Errorf("agentactivity: forget: %w", err)
	}
	return nil
}

// deleteOrphanedRefs removes each of chatID's former blobs that no OTHER
// chat's tool call still references. Called after DeleteChat, so chatID's
// own rows are already gone and RefInUse only sees rows that belong to
// somebody else — content is deduplicated by hash, so two unrelated chats
// can legitimately share one blob. Best-effort: a check or delete failure is
// logged, never fatal — Forget must still complete the row/event erasure the
// caller is waiting on.
func (r *eventSourced) deleteOrphanedRefs(ctx context.Context, refs []string) {
	for _, ref := range refs {
		inUse, err := r.store.RefInUse(ctx, ref)
		if err != nil {
			slog.WarnContext(ctx, "agentactivity: forget: ref liveness check failed; leaving blob", "ref", ref, "err", err)
			continue
		}
		if inUse {
			continue
		}
		if err := r.store.Content().Delete(ref); err != nil {
			slog.WarnContext(ctx, "agentactivity: forget: delete blob failed", "ref", ref, "err", err)
		}
	}
}
```

Add `"log/slog"` to `activity.go`'s imports if not already present (it is — `TouchProjectActivity`-style logging is common in this codebase; confirm with `goimports`/`go build`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/app/repositories/chat/activity/... -v`
Expected: PASS, full package including pre-existing tests.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/repositories/chat/activity/activity.go api/internal/app/repositories/chat/activity/activity_test.go
git commit -m "feat(activity): delete orphaned content blobs on Forget"
```

---

## Self-Review Notes

- **Spec coverage:** Task 1 gives content a delete primitive; Task 2 gives the SQL layer (the actual source of truth for "who points at this ref") a liveness query; Task 3 exposes both up the existing delegation chain; Task 4 wires them into the one place chats are actually erased. No new persisted counter, no background process.
- **Known risk, called out rather than hidden:** the dedup-sharing scenario (Task 4's second test) is the entire reason this can't be "the chat that wrote it just deletes it" — confirmed real by `content.Store.Put`'s own doc comment (`content.go`: `if _, err := os.Stat(path); err == nil { return ref, nil }`, i.e. an identical payload from anyone reuses the same file).
