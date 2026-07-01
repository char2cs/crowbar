# Review Thread Comment Menu — Implementation Plan

> Per-comment three-dots menu on review threads (GitHub-style): Delete, Edit (own), Copy as Markdown. Plus: wire up the bypassed auto-outdated UI.

**Goal:** Give each review comment a hover three-dots menu with Delete / Edit (own) / Copy-as-Markdown, and surface auto-outdated threads collapsed instead of dropping them.

**Architecture:** Review threads are 100% local (daemon SQLite, event-sourced via Asynx; provider is read-only). All mutations are local; the HTTP handler pushes the resulting DTO over the `threads` WS topic and the FE store converges via `upsertReviewThread` / `removeReviewThread`.

**Tech Stack:** Go (Asynx event sourcing + Gin + GORM/SQLite), React 19 + zustand + Tailwind v4 + base-ui (Monaco view-zone hosted threads).

## Global Constraints
- `@/components/ui/*` + CSS variable tokens only; never hardcode colors. kebab-case files.
- zustand narrow selectors; `getState()` only in handlers/effects; stores never import components.
- Tests live in `web/src/__tests__/` mirroring `web/src/`; `cd web && ./node_modules/.bin/vitest run <path>`.
- Go: `cd api && go test ./...`; black-box/integration coverage under `api/tests`.
- pnpm only. `tsc --noEmit` must have no NEW errors (pre-existing `otp-field.tsx` error excepted).
- Verify LIVE in the running Tauri app before claiming done.

---

## Key contract facts (discovered)
- Domain `ReviewThread.Messages[]`: `Messages[0]` is the root comment, rest are replies; **all have real UUIDs**.
- `ThreadDTOFrom` (api/internal/api/v0/dto/thread.go) currently **drops the root's real id** — surfaces body/author/isAgent at top level; FE synthesizes `${id}:root`.
- HTTP mutation broadcast happens at the **handler layer** (`h.push(d)` → v0 container `threads` broadcaster). The repository projection broadcast is a no-op (`repositories/container.go:52`).
- Asynx `Forget(ctx, id)` (v0.4.0) erases an aggregate; `OnForget` is already wired (`store/projections.go:31`) to delete the read-model row. It does NOT broadcast — the handler must push a tombstone.

---

## Task 1 — Backend: DTO root id + edit/delete-message commands + delete-thread (Forget) + routes/handlers + tests

**Files**
- Modify: `api/internal/api/v0/dto/thread.go` (add `MessageID`, `Deleted` fields; set `MessageID` from `Messages[0].ID`)
- Create: `api/internal/app/repositories/reviewthread/internal/commands/edit_message.go`
- Create: `api/internal/app/repositories/reviewthread/internal/commands/delete_message.go`
- Modify: `api/internal/app/repositories/reviewthread/reviewthread.go` (interface + impl: `EditMessage`, `DeleteMessage`, `DeleteThread`)
- Modify: `api/internal/api/v0/endpoints/threads/handlers/handlers.go` (ThreadStore interface: add the 3 methods)
- Modify: `api/internal/api/v0/endpoints/threads/handlers/threads.go` (`EditMessage`, `DeleteMessage`, `DeleteThread` handlers)
- Modify: `api/internal/api/v0/endpoints/threads/routes.go` (PATCH/DELETE `/threads/:threadId/messages/:messageId`, DELETE `/threads/:threadId`)
- Tests: `commands_test.go`, `handlers_test.go`, `routes_test.go`, `dto/thread_test.go`, integration `api/tests/integration/threads/threads_test.go`

**Command shapes** (mirror reply.go/resolve.go):
- `EditReviewMessage{ID, MessageID, Body string; Now time.Time}` — EventName `review_thread.message_edited.{ID}`; Validate: current!=nil, MessageID!="", message exists, Body!=""; EmitEvent copies thread and replaces matching `.Body`.
- `DeleteReviewMessage{ID, MessageID string}` — EventName `review_thread.message_deleted.{ID}`; Validate: current!=nil, MessageID!="", message exists AND `current.Messages[0].ID != MessageID` (root not deletable here → return ErrValidation); EmitEvent copies thread and filters the message out.

**Repository impl**: `EditMessage`/`DeleteMessage` → `ax.SendWait`; `DeleteThread(ctx,id)` → `r.ax.Forget(ctx, id)`.

**Handlers**:
- `EditMessage` (PATCH): body `{body}`; `store.EditMessage(...)`; `push(dto)`; 200.
- `DeleteMessage` (DELETE): `store.DeleteMessage(...)` returns updated thread; `push(dto)`; 200.
- `DeleteThread` (DELETE): `store.DeleteThread(threadId)`; push tombstone `dto.ThreadDTO{ID, ProjectID, RepoID, WorkspaceID:wsID, Deleted:true}` (must carry p/r/w for the broadcaster routing); 200.

**Verify**: `cd api && go test ./...` green; new integration tests cover edit (root+reply), delete reply (ok) + delete root via message route (400), delete thread (gone + tombstone), DTO `messageId` present.

---

## Task 2 — Frontend data layer: types, api fns, mapThread root id, WS tombstone

**Files**
- Modify: `web/src/lib/types.ts` (`ThreadDTO`: add `messageId: string`, `deleted?: boolean`)
- Modify: `web/src/features/git/api/review-api.ts` (`mapThread` root id from `t.messageId`; add `deleteThread`, `deleteMessage`, `editMessage`)
- Modify: `web/src/features/workspace/stores/hooks/use-workspace-threads-stream.ts` (tombstone branch → `removeReviewThread`)
- Tests: `__tests__/features/git/api/review-api.test.ts`, `__tests__/.../use-workspace-threads-stream.test.ts`

**api fns**
- `deleteThread(wsId, threadId): Promise<void>` → `DELETE workspaceBase/threads/:id`
- `deleteMessage(wsId, threadId, messageId): Promise<ReviewThread>` → `DELETE .../threads/:id/messages/:mid` → mapThread
- `editMessage(wsId, threadId, messageId, body): Promise<ReviewThread>` → `PATCH .../threads/:id/messages/:mid {body}` → mapThread
- `mapThread`: `rootMessage.id = t.messageId || ` `${t.id}:root` (fallback)

**WS stream**: before `mapThread(frame)`, if `frame.deleted && frame.id` → `removeReviewThread(frame.id)`; return.

---

## Task 3 — Frontend UI: per-message three-dots menu + inline edit + outdated fix

**Files**
- Modify: `web/src/features/panes/components/comment-composer.tsx` (optional `initialValue?: string`)
- Modify: `web/src/features/git/components/review-thread-item.tsx` (per-message `DropdownMenu`; inline edit; props `onDeleteThread`/`onDeleteMessage`/`onEditMessage`; `isOutdated` collapsed render already present)
- Modify: `web/src/features/git/utils/diff-editor-content.ts` (`findNearestUnifiedModelLine`)
- Modify: `web/src/features/git/components/diff/use-review-comment-layer.tsx` (wire handlers; outdated fallback anchor + pass `isOutdated`)
- Tests: `__tests__/features/git/components/review-thread-item.test.tsx`, `__tests__/features/git/utils/diff-editor-content.test.ts`

**Menu** (per MessageRow, hover-revealed trigger, base-ui `DropdownMenu`):
- Edit — only if `!message.isAgent && currentIdentity?.login === message.author`; enters inline edit (CommentComposer seeded w/ body) → `onEditMessage(thread.id, message.id, body)`.
- Copy as Markdown — `navigator.clipboard.writeText(message.body)`.
- Delete (destructive) — root (index 0) → `onDeleteThread(thread.id)`; reply → `onDeleteMessage(thread.id, message.id)`. Confirm via existing AlertDialog/Dialog if available, else immediate (local tool).

**Outdated**: `findNearestUnifiedModelLine(anchors, side, line)` = nearest entry on `side` with line ≤ target (else first after; else last non-spacer model line); returns model line or null. In use-review-comment-layer: when `findUnifiedModelLine` is null, fall back to nearest and render the thread with `isOutdated` (collapsed badge) instead of `continue`.

---

## Verify (orchestrator, live Tauri)
Rebuild daemon + reload app. In Branch Review: hover a comment → three-dots; Copy markdown; Edit own comment inline; Delete a reply; Delete a thread (junk `qadasdasda`/`test` threads gone); confirm an outdated thread renders collapsed. Then adversarial multi-dimension review of the branch diff + fix.
