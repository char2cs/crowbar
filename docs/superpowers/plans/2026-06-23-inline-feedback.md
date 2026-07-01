# Inline Feedback — Replace Toasts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace all non-editor toast notifications with inline, contextual feedback tied to the element that triggered the action.

**Architecture:** Three independent surface areas — git async ops (push/pull/rebase/merge), workspace/chat CRUD (optimistic creates and moves), and file-explorer clipboard ops — each gets its own local error/loading state rendered inline with no toast calls. The WS stream already drives final state; we just stop intercepting it with toasts.

**Tech Stack:** React, TypeScript, Tailwind CSS, Zustand (`useSidebarStore`), `@agilek/cli-loaders` (workspace spinner)

## Global Constraints

- No new dependencies. Use existing `WorkspaceAgentSpinner` from `workspace-branch-icon.tsx` for all spinners.
- All Tailwind: `text-destructive`, `text-muted-foreground`, `bg-destructive/10`, `border-destructive/30` for error states.
- Test files go in `web/src/__tests__/` mirroring source structure. Use `@/` imports.
- Remove every `toast.info`, `toast.success`, `toast.warning` call in-scope. Keep `toast.error` ONLY in editor/LSP files (out of scope).
- Run `cd web && npx tsc --noEmit` after each task to verify no type errors.

---

## Parallelism Map

**Wave 1 — all independent, run in parallel:**
- Task 1: `branch-section.tsx`
- Task 2: `merge-popover.tsx`
- Task 3: `git-actions-menu.tsx`
- Task 4: `git-tag-manager.tsx`
- Task 5: `use-review-comment-layer.tsx`
- Task 6: File explorer clipboard
- Task 7: Chat CRUD

**Wave 2 — sequential (same files), after Wave 1:**
- Task 8: Workspace create — optimistic spinner
- Task 9: Workspace move — optimistic snap-back (depends on Task 8)

---

### Task 1: branch-section.tsx — Inline push/pull/rebase errors

**Files:**
- Modify: `web/src/features/git/components/branch-section.tsx`

**Interfaces:**
- Produces: no API changes — local state only

- [ ] **Step 1: Add error state and wire status text during in-flight**

Replace the top of `BranchSection` component body. The existing `remoteBusy` and `rebasing` states stay; add two error states and derive in-flight status text:

```tsx
const [remoteBusy, setRemoteBusy] = useState(false)
const [rebasing, setRebasing] = useState(false)
const [remoteError, setRemoteError] = useState<string | null>(null)
const [rebaseError, setRebaseError] = useState<string | null>(null)

const refresh = () => window.dispatchEvent(new Event('git-status-changed'))

const handleRebaseOntoParent = async () => {
  setRebasing(true)
  setRebaseError(null)
  try {
    await rebaseOntoParent(wsId)
    refresh()
  } catch (e) {
    setRebaseError(e instanceof Error ? e.message : 'Rebase failed')
  } finally {
    setRebasing(false)
  }
}

const runRemote = async (kind: 'push' | 'pull') => {
  setRemoteBusy(true)
  setRemoteError(null)
  try {
    const res = kind === 'push' ? await pushChanges(wsId) : await pullChanges(wsId)
    if (res.success) {
      refresh()
    } else {
      setRemoteError(res.error || `Failed to ${kind}`)
    }
  } catch (e) {
    setRemoteError(e instanceof Error ? e.message : `Failed to ${kind}`)
  } finally {
    setRemoteBusy(false)
  }
}
```

- [ ] **Step 2: Update statusLine to reflect in-flight state**

Replace the `statusLine` computation:

```tsx
const statusLine = (() => {
  if (remoteBusy) return kind === 'push' ? 'Pushing to remote…' : 'Pulling from remote…'
  if (rebasing) return `Rebasing onto ${parentBranch}…`
  if (action.kind === 'commit') {
    return `${files.length} uncommitted change${files.length !== 1 ? 's' : ''}`
  }
  if (action.kind === 'resolve') return `Conflicts with ${parentBranch}`
  if (action.kind === 'pull-request') return `${parentBranch} is protected`
  if (ahead > 0 && behind > 0) return `Diverged · ${ahead} ahead, ${behind} behind`
  if (behind > 0) return `Clean · ${behind} behind`
  if (ahead > 0) return `Clean · ${ahead} to push`
  return 'Up to date'
})()
```

Note: the `kind` variable inside `statusLine` doesn't exist at that scope. Replace with:

```tsx
const statusLine = (() => {
  if (remoteBusy && action.remote === 'push') return 'Pushing to remote…'
  if (remoteBusy && action.remote === 'pull') return 'Pulling from remote…'
  if (remoteBusy) return 'Syncing…'
  if (rebasing) return `Rebasing onto ${parentBranch}…`
  if (action.kind === 'commit') {
    return `${files.length} uncommitted change${files.length !== 1 ? 's' : ''}`
  }
  if (action.kind === 'resolve') return `Conflicts with ${parentBranch}`
  if (action.kind === 'pull-request') return `${parentBranch} is protected`
  if (ahead > 0 && behind > 0) return `Diverged · ${ahead} ahead, ${behind} behind`
  if (behind > 0) return `Clean · ${behind} behind`
  if (ahead > 0) return `Clean · ${ahead} to push`
  return 'Up to date'
})()
```

- [ ] **Step 3: Remove toast import and add inline error rendering**

1. Remove `import { toast } from '@/features/window/stores/toast-store'` (if it's only used for these calls).
2. Add a spinner import at top: `import { ArrowUp, ArrowDown, GitBranch } from '@phosphor-icons/react'` — already present.
3. In the remote button, add `disabled={remoteBusy}` and a spinner:

```tsx
{action.remote && (
  <Button
    variant="outline"
    size="sm"
    disabled={remoteBusy}
    onClick={() => void runRemote(action.remote as 'push' | 'pull')}
  >
    {remoteBusy ? (
      <span className="size-3.5 animate-spin rounded-full border border-transparent border-t-current" />
    ) : action.remote === 'push' ? (
      <ArrowUp className="size-3.5" />
    ) : (
      <ArrowDown className="size-3.5" />
    )}
    {remoteBusy
      ? action.remote === 'push' ? 'Pushing…' : 'Pulling…'
      : action.remote === 'push'
        ? `Push${ahead ? ` ${ahead}` : ''}`
        : `Pull${behind ? ` ${behind}` : ''}`}
  </Button>
)}
```

4. After the action button row `</div>`, add error rendering:

```tsx
{(remoteError || rebaseError) && (
  <p className="ui-text-xs text-destructive mt-1">
    {remoteError ?? rebaseError}
    {' · '}
    <button
      type="button"
      className="underline"
      onClick={() => {
        if (remoteError) { setRemoteError(null); void runRemote(action.remote as 'push' | 'pull') }
        if (rebaseError) { setRebaseError(null); void handleRebaseOntoParent() }
      }}
    >
      Retry
    </button>
  </p>
)}

{action.kind === 'resolve' && (
  <p className="ui-text-xs text-muted-foreground">
    This branch conflicts with {parentBranch} and isn't integrated yet. Rebase onto{' '}
    {parentBranch} to resolve it — or drag it back to undo.
  </p>
)}
```

- [ ] **Step 4: Verify types**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-levitated-helium-84aa/web && npx tsc --noEmit 2>&1 | grep "branch-section"
```

Expected: no output (no errors in this file).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/git/components/branch-section.tsx
git commit -m "feat(git): replace push/pull/rebase toasts with inline error + spinner"
```

---

### Task 2: merge-popover.tsx — Inline merge error

**Files:**
- Modify: `web/src/features/git/components/merge-popover.tsx`

- [ ] **Step 1: Replace toast calls with local error state**

Add state at top of `MergePopover`:

```tsx
const [mergeError, setMergeError] = useState<string | null>(null)
const [strategyError, setStrategyError] = useState<string | null>(null)
const [merging, setMerging] = useState(false)
```

Replace `selectStrategy`:

```tsx
const selectStrategy = async (next: MergeStrategy) => {
  if (next === strategy) return
  const previous = strategy
  setStrategyError(null)
  getOrCreateWorkspaceStore(wsId).getState().setBranchReviewMergeStrategy(next)
  try {
    await patchMergeStrategy(wsId, next)
  } catch {
    getOrCreateWorkspaceStore(wsId).getState().setBranchReviewMergeStrategy(previous)
    setStrategyError('Failed to save strategy — try again')
  }
}
```

Replace `handleMerge`:

```tsx
const handleMerge = async () => {
  setMergeError(null)
  setMerging(true)
  let redirect: { projectId: string; repoId: string; wsId: string } | null = null
  if (deleteAfterMerge) {
    const repos = useSidebarStore.getState().repos
    const targetWsId = getPostDeleteNavigationTarget(repos, wsId)
    const repo = targetWsId
      ? repos.find((r) => r.workspaces.some((w) => w.id === targetWsId))
      : undefined
    if (targetWsId && repo) {
      redirect = { projectId: repo.projectId ?? '', repoId: repo.id, wsId: targetWsId }
    }
  }
  try {
    await mergeIntoParent(wsId, strategy, deleteAfterMerge)
    setOpen(false)
    if (redirect) void navigate({ to: '/ide/$projectId/$repoId/$wsId', params: redirect })
  } catch {
    setMergeError('Merge failed — check the logs for details')
  } finally {
    setMerging(false)
  }
}
```

- [ ] **Step 2: Remove toast import, render errors inline in popover**

Remove `import { toast } from '@/features/window/stores/toast-store'`.

In `PopoverContent`, after the `<RadioGroup>` add strategy error:

```tsx
{strategyError && (
  <p className="ui-text-xs text-destructive mb-2">{strategyError}</p>
)}
```

Replace the confirm button:

```tsx
<Button
  variant="default"
  size="sm"
  className="w-full"
  disabled={merging}
  onClick={() => void handleMerge()}
>
  {merging ? 'Merging…' : active.confirm}
</Button>
{mergeError && (
  <p className="ui-text-xs text-destructive mt-2">{mergeError}</p>
)}
```

- [ ] **Step 3: Verify types**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-levitated-helium-84aa/web && npx tsc --noEmit 2>&1 | grep "merge-popover"
```

Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/git/components/merge-popover.tsx
git commit -m "feat(git): replace merge toasts with inline error in popover"
```

---

### Task 3: git-actions-menu.tsx — Spinner on action + inline error

**Files:**
- Modify: `web/src/features/git/components/git-actions-menu.tsx`

- [ ] **Step 1: Read the full file to understand the toast pattern**

```bash
cat web/src/features/git/components/git-actions-menu.tsx
```

The file uses a `toast.show` for in-progress, `toast.success` on done, `toast.error` on failure. It has a `toastId` variable pattern.

- [ ] **Step 2: Replace the toast pattern with local state**

Add state:

```tsx
const [runningAction, setRunningAction] = useState<string | null>(null)
const [actionError, setActionError] = useState<string | null>(null)
```

The `runAction` helper (or equivalent function wrapping the toast show/dismiss pattern) becomes:

```tsx
async function runAction(
  name: string,
  fn: () => Promise<void>,
  messages?: { success?: string; error?: string },
): Promise<void> {
  setRunningAction(name)
  setActionError(null)
  try {
    await fn()
  } catch (e) {
    const msg = e instanceof Error ? e.message : (messages?.error ?? `${name} failed`)
    setActionError(msg)
  } finally {
    setRunningAction(null)
  }
}
```

Remove all `toast.show`, `toast.dismiss`, `toast.success`, `toast.error` calls. Remove the `toastId` variable entirely.

- [ ] **Step 3: Show spinner on the active menu item, error below menu**

In the menu item JSX for each action, add a spinner when that action is running. Example pattern for a menu item:

```tsx
<ContextMenuItem
  disabled={!!runningAction}
  onSelect={() => void runAction('Push', () => pushChanges(repoPath))}
>
  {runningAction === 'Push' ? (
    <span className="size-3.5 animate-spin rounded-full border border-transparent border-t-current mr-2" />
  ) : (
    <ArrowUp className="size-3.5 mr-2" />
  )}
  Push
</ContextMenuItem>
```

After the last menu item, before closing the menu, render the error:

```tsx
{actionError && (
  <div className="px-2 py-1.5 text-xs text-destructive border-t border-border mt-1">
    {actionError}
  </div>
)}
```

Remove `import { toast }` from the imports.

- [ ] **Step 4: Verify types**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-levitated-helium-84aa/web && npx tsc --noEmit 2>&1 | grep "git-actions-menu"
```

- [ ] **Step 5: Commit**

```bash
git add web/src/features/git/components/git-actions-menu.tsx
git commit -m "feat(git): replace actions-menu toasts with inline spinner + error"
```

---

### Task 4: git-tag-manager.tsx — Inline tag row feedback

**Files:**
- Modify: `web/src/features/git/components/git-tag-manager.tsx`

- [ ] **Step 1: Add per-action feedback state**

```tsx
const [tagFeedback, setTagFeedback] = useState<{ id: string; kind: 'ok' | 'err'; msg: string } | null>(null)
```

Helper to auto-clear after 2s:

```tsx
function showTagFeedback(id: string, kind: 'ok' | 'err', msg: string) {
  setTagFeedback({ id, kind, msg })
  setTimeout(() => setTagFeedback(null), 2000)
}
```

- [ ] **Step 2: Replace toast calls with showTagFeedback**

For each tag action (create, push, delete, checkout) replace:
- `toast.success(msg)` → `showTagFeedback(tagId, 'ok', msg)`  
- `toast.error(msg)` → `showTagFeedback(tagId, 'err', msg)`
- Copy actions: use `showTagFeedback('copy', 'ok', '✓ copied')` with the hash/tag name as context

- [ ] **Step 3: Render feedback on the relevant row**

In the tag row JSX, after the action buttons, add:

```tsx
{tagFeedback?.id === tag.name && (
  <span className={cn(
    'text-xs ml-auto',
    tagFeedback.kind === 'ok' ? 'text-green-500' : 'text-destructive'
  )}>
    {tagFeedback.msg}
  </span>
)}
```

For copy feedback (`tagFeedback?.id === 'copy'`), render it at the top of the dialog or near the copy button.

Remove `import { toast }`.

- [ ] **Step 4: Verify types**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-levitated-helium-84aa/web && npx tsc --noEmit 2>&1 | grep "git-tag-manager"
```

- [ ] **Step 5: Commit**

```bash
git add web/src/features/git/components/git-tag-manager.tsx
git commit -m "feat(git): replace tag-manager toasts with inline row feedback"
```

---

### Task 5: use-review-comment-layer.tsx — Inline comment errors

**Files:**
- Modify: `web/src/features/git/components/diff/use-review-comment-layer.tsx`

- [ ] **Step 1: Read the full hook to understand the error surface**

```bash
cat web/src/features/git/components/diff/use-review-comment-layer.tsx
```

The hook fires `toast.error` on: post reply, edit comment, delete message, delete thread, post comment. These fire from callbacks returned in the `ReviewCommentLayer` object.

- [ ] **Step 2: Add error state and replace toast calls**

At the top of the hook:

```tsx
const [commentError, setCommentError] = useState<{ threadId?: string; msg: string } | null>(null)
```

For each failing operation, replace:
- `toast.error('Failed to post reply', ...)` → `setCommentError({ threadId, msg: 'Failed to post reply' })`
- `toast.error('Failed to edit comment', ...)` → `setCommentError({ threadId, msg: 'Failed to edit comment' })`
- `toast.error('Failed to delete comment', ...)` → `setCommentError({ threadId, msg: 'Failed to delete comment' })`
- `toast.error('Failed to delete thread', ...)` → `setCommentError({ threadId, msg: 'Failed to delete thread' })`
- `toast.error('Failed to post comment', ...)` → `setCommentError({ msg: 'Failed to post comment' })`

- [ ] **Step 3: Surface commentError in the returned layer object**

The hook returns a `ReviewCommentLayer | null`. Add `commentError` to what it returns (or to the caller props). Check the type definition of `ReviewCommentLayer`:

```bash
grep -n "ReviewCommentLayer\|interface.*Layer\|type.*Layer" web/src/features/git/components/diff/use-review-comment-layer.tsx web/src/features/git/types/*.ts 2>/dev/null | head -20
```

Add `commentError: { threadId?: string; msg: string } | null` to the return type and return it. The component consuming this hook renders it inline near the affected thread.

Remove `import { toast }`.

- [ ] **Step 4: Verify types**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-levitated-helium-84aa/web && npx tsc --noEmit 2>&1 | grep "use-review-comment-layer\|ReviewCommentLayer"
```

- [ ] **Step 5: Commit**

```bash
git add web/src/features/git/components/diff/use-review-comment-layer.tsx
git commit -m "feat(git): replace review comment toasts with inline error state"
```

---

### Task 6: File explorer clipboard — Micro-feedback badges

**Files:**
- Modify: `web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx`

The hook's caller renders file rows. The hook needs to return a feedback map that the file row can read to show a transient badge.

- [ ] **Step 1: Add feedback state**

```tsx
const [fileFeedback, setFileFeedback] = useState<Map<string, 'copied-path' | 'copied-rel' | 'created' | 'err'>>(new Map())

function flashFeedback(path: string, kind: 'copied-path' | 'copied-rel' | 'created' | 'err') {
  setFileFeedback((prev) => new Map(prev).set(path, kind))
  setTimeout(() => {
    setFileFeedback((prev) => {
      const next = new Map(prev)
      next.delete(path)
      return next
    })
  }, 1500)
}
```

- [ ] **Step 2: Replace clipboard toast calls**

Find and replace in the hook:

```tsx
// copy path (was: toast.success('Copied path'))
navigator.clipboard.writeText(fullPath).then(
  () => flashFeedback(contextMenu.path, 'copied-path'),
  () => flashFeedback(contextMenu.path, 'err'),
)

// copy relative path (was: toast.success('Copied relative path'))
navigator.clipboard.writeText(relativePath).then(
  () => flashFeedback(contextMenu.path, 'copied-rel'),
  () => flashFeedback(contextMenu.path, 'err'),
)
```

For `toast.success('Refreshed')` (refresh action) — simply remove it; the file tree rerenders on completion.

For `toast.success('Copied path')` / `toast.error('Failed to copy path')` / `toast.success('Copied relative path')` / `toast.error('Failed to copy relative path')` / `toast.success('Copied ...')` / `toast.success('Cut ...')` — all replaced with `flashFeedback` or removed.

For `toast.success(`Created ${targetFileName}`)` → `flashFeedback(targetFilePath, 'created')`.

For `toast.error('Choose a different env file name')` → render inline validation in the rename input (or just leave as `console.warn` — this guards an edge case with `.env` files; the rename input is the right place for validation but that's a separate component; for now leave it as a no-op error that cancels the operation silently).

- [ ] **Step 3: Return fileFeedback from the hook and label map**

Add to the hook's return object:

```tsx
return {
  // ...existing returns
  fileFeedback,
}
```

- [ ] **Step 4: Render badges in the file row**

Find the component that renders file rows (likely `file-explorer.tsx` or similar). Import the feedback map and render:

```bash
grep -rn "useFileExplorerContextMenu\|fileFeedback\|contextMenu" web/src/features/file-explorer/ --include="*.tsx" -l
```

In the file row JSX, after the file name:

```tsx
{fileFeedback.get(file.path) === 'copied-path' && (
  <span className="ml-auto text-xs text-green-500 shrink-0">✓ path copied</span>
)}
{fileFeedback.get(file.path) === 'copied-rel' && (
  <span className="ml-auto text-xs text-green-500 shrink-0">✓ rel copied</span>
)}
{fileFeedback.get(file.path) === 'created' && (
  <span className="ml-auto text-xs text-green-500 shrink-0">✓ created</span>
)}
{fileFeedback.get(file.path) === 'err' && (
  <span className="ml-auto text-xs text-destructive shrink-0">✕ failed</span>
)}
```

Remove `import { toast }` from the hook (if no other calls remain).

- [ ] **Step 5: Verify types**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-levitated-helium-84aa/web && npx tsc --noEmit 2>&1 | grep "file-explorer-context-menu\|file-explorer"
```

- [ ] **Step 6: Commit**

```bash
git add web/src/features/file-explorer/
git commit -m "feat(files): replace clipboard toasts with micro-feedback badges on file rows"
```

---

### Task 7: Chat CRUD — Inline errors

**Files:**
- Modify: `web/src/components/layout/chat-tree-context.tsx`
- Modify: `web/src/components/layout/chat-tree.tsx`

- [ ] **Step 1: Add error state to chat tree context**

In `chat-tree-context.tsx`, in the provider component, add:

```tsx
const [chatErrors, setChatErrors] = useState<Map<string, string>>(new Map())

function setChatError(id: string, msg: string) {
  setChatErrors((prev) => new Map(prev).set(id, msg))
}
function clearChatError(id: string) {
  setChatErrors((prev) => { const n = new Map(prev); n.delete(id); return n })
}
```

- [ ] **Step 2: Replace toast.error calls in chat context**

In `performCreateChat`: on failure, call `setChatError('create', err message)`.
In `performForkChat`: on failure, call `setChatError(parentId, err message)`.
In `performRenameChat`: on failure, call `setChatError(chatId, err message)`.
In `performDeleteChat`: on failure, call `setChatError(chatId, err message)`.

Remove `toast.error` calls. Remove `import { toast }` if no other usage.

- [ ] **Step 3: Expose chatErrors via context**

Add `chatErrors: Map<string, string>` and `clearChatError: (id: string) => void` to the context value type and value object.

- [ ] **Step 4: Replace workspace-guard toasts in chat-tree.tsx**

In `chat-tree.tsx`, find the two `toast.error('Open a workspace to ...')` calls. Replace with an inline hint:

```tsx
// Before: toast.error('Open a workspace to view this chat')
// After: render a disabled state or skip navigation with no feedback
// (these are no-ops when no workspace is active — the sidebar already shows no active WS)
// Simply return early without any toast or UI change.
```

- [ ] **Step 5: Render chatErrors on chat rows**

In `chat-tree.tsx`, in the chat row render, read `chatErrors.get(chat.id)` and render inline:

```tsx
{chatErrors.get(chat.id) && (
  <span className="ml-auto text-xs text-destructive flex items-center gap-1">
    {chatErrors.get(chat.id)}
    <button type="button" onClick={() => clearChatError(chat.id)} className="hover:text-foreground">✕</button>
  </span>
)}
```

- [ ] **Step 6: Verify types**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-levitated-helium-84aa/web && npx tsc --noEmit 2>&1 | grep "chat-tree"
```

- [ ] **Step 7: Commit**

```bash
git add web/src/components/layout/chat-tree-context.tsx web/src/components/layout/chat-tree.tsx
git commit -m "feat(chat): replace chat CRUD toasts with inline row errors"
```

---

### Task 8: Workspace create — Optimistic spinner (run after Wave 1)

**Files:**
- Modify: `web/src/components/layout/workspace-tree-context.tsx`
- Modify: `web/src/components/layout/workspace-tree.tsx`

**Key facts:**
- `WorkspaceBranchIcon` from `workspace-branch-icon.tsx` shows `WorkspaceAgentSpinner` when `working={true}`.
- `confirmCreate` currently calls the external `performCreateWorkspace` and clears `creatingChildOf`.
- We need to add `pendingCreates` state so the pending row persists until WS confirms.

- [ ] **Step 1: Add PendingCreate type and state to WorkspaceTreeProvider**

In `workspace-tree-context.tsx`, after the existing state declarations:

```tsx
interface PendingCreate {
  repoId: string
  parentId: string
  branch: string
  error?: string
}

// In WorkspaceTreeActionsContextValue interface, add:
pendingCreates: Map<string, PendingCreate>
clearPendingCreate: (tempId: string) => void
```

In `WorkspaceTreeProvider` body:

```tsx
const [pendingCreates, setPendingCreates] = useState<Map<string, PendingCreate>>(new Map())

function addPendingCreate(tempId: string, entry: PendingCreate) {
  setPendingCreates((prev) => new Map(prev).set(tempId, entry))
}
function setPendingCreateError(tempId: string, error: string) {
  setPendingCreates((prev) => {
    const entry = prev.get(tempId)
    if (!entry) return prev
    return new Map(prev).set(tempId, { ...entry, error })
  })
}
function clearPendingCreate(tempId: string) {
  setPendingCreates((prev) => { const n = new Map(prev); n.delete(tempId); return n })
}
```

- [ ] **Step 2: Rewrite confirmCreate to be optimistic**

Import `crypto` is available in the browser; use `crypto.randomUUID()`.

Replace `confirmCreate`:

```tsx
const confirmCreate = useCallback(
  (branch: string) => {
    if (!creatingChildOf) return
    const { repoId, parentId } = creatingChildOf
    const tempId = crypto.randomUUID()
    setCreatingChildOf(null) // hide input immediately

    addPendingCreate(tempId, { repoId, parentId, branch })

    // Subscribe to sidebar store: when real workspace arrives, remove pending
    const unsub = useSidebarStore.subscribe((state) => {
      const repo = state.repos.find((r) => r.id === repoId)
      if (!repo) return
      const found = repo.workspaces.find(
        (w) => w.branch === branch && w.parentId === parentId,
      )
      if (found) {
        clearPendingCreate(tempId)
        unsub()
      }
    })

    // Fire the API
    const projectId = projectIdForRepo(repoId)
    if (!projectId) {
      setPendingCreateError(tempId, 'Unknown project')
      unsub()
      return
    }
    postWorkspace(projectId, repoId, branch, parentId).catch((err) => {
      unsub()
      setPendingCreateError(tempId, err instanceof Error ? err.message : 'Create failed')
    })
  },
  [creatingChildOf], // eslint-disable-line react-hooks/exhaustive-deps
)
```

You need to import `postWorkspace` and `projectIdForRepo` at the top of the file — these were previously used only inside `performCreateWorkspace`. Import them:

```tsx
import { postWorkspace } from '@/lib/api/workspace' // check actual path
import { projectIdForRepo } from './workspace-tree-context' // it's a local helper
```

Actually `projectIdForRepo` is a local helper in `workspace-tree-context.tsx` already — no import needed, just use it directly.

Check the actual import paths:
```bash
grep -n "import.*postWorkspace\|import.*projectIdForRepo" web/src/components/layout/workspace-tree-context.tsx
```

- [ ] **Step 3: Expose pendingCreates via context**

Add `pendingCreates` and `clearPendingCreate` to the `actionsValue` object passed to the provider.

Update `WorkspaceTreeActionsContextValue` interface to include:
```tsx
pendingCreates: Map<string, PendingCreate>
clearPendingCreate: (tempId: string) => void
```

Export the `PendingCreate` type so `workspace-tree.tsx` can use it.

- [ ] **Step 4: Render pending creates in workspace-tree.tsx**

In `WorkspaceTreeInner`, consume `pendingCreates` from context:

```tsx
const { creatingChildOf, startCreating, confirmCreate, cancelCreate, pendingCreates, clearPendingCreate } = useWorkspaceTreeActions()
```

In the repo render loop, after the `{roots.map(...)}` block, add pending creates for this repo:

```tsx
{Array.from(pendingCreates.entries())
  .filter(([, p]) => p.repoId === repo.id)
  .map(([tempId, pending]) => (
    <div key={tempId} style={{ paddingLeft: 14 }}>
      <div className={cn(ROW_BASE, 'border-transparent opacity-60 pointer-events-none')}>
        {pending.error ? (
          <>
            <span className="size-4 shrink-0 text-destructive flex items-center justify-center text-xs">✕</span>
            <span className="min-w-0 flex-1 truncate font-mono text-muted-foreground text-[13px]">
              {pending.branch}
            </span>
            <span className="text-xs text-destructive">failed</span>
            <button
              type="button"
              className="pointer-events-auto text-xs text-muted-foreground hover:text-foreground ml-1"
              onClick={() => clearPendingCreate(tempId)}
            >
              ✕
            </button>
          </>
        ) : (
          <>
            <WorkspaceAgentSpinner />
            <span className="min-w-0 flex-1 truncate font-mono text-muted-foreground text-[13px]">
              {pending.branch}
            </span>
          </>
        )}
      </div>
    </div>
  ))}
```

Import `WorkspaceAgentSpinner` at the top of `workspace-tree.tsx`:

```tsx
import { WorkspaceAgentSpinner } from './workspace-branch-icon'
```

Note: pending creates at nested depths (i.e., child of a non-root workspace) need to appear inside `WorkspaceTreeItem` at the right depth. For the initial implementation, only handle root-level creates (parentId === repo.defaultWorkspaceId) here. Nested creates can be added in a follow-up — the pattern is the same but rendered inside `WorkspaceTreeItem` for `creatingChildOf.parentId === workspace.id`.

- [ ] **Step 5: Remove the old performCreateWorkspace toast calls**

In `workspace-tree-context.tsx`, in the standalone `performCreateWorkspace` function (lines 32–48), it still has `toast.error` calls. Remove them (replace with `console.error` only) since this function is no longer called from `confirmCreate`. Keep the function exported in case other callers exist:

```bash
grep -rn "performCreateWorkspace" web/src/ --include="*.tsx" --include="*.ts"
```

If only called from the old `confirmCreate`, the function can be removed entirely. If called elsewhere, just remove the toast calls.

- [ ] **Step 6: Verify types**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-levitated-helium-84aa/web && npx tsc --noEmit 2>&1 | grep "workspace-tree"
```

- [ ] **Step 7: Commit**

```bash
git add web/src/components/layout/workspace-tree-context.tsx web/src/components/layout/workspace-tree.tsx
git commit -m "feat(workspace): optimistic create — spinner in icon slot until WS confirms"
```

---

### Task 9: Workspace move — Optimistic snap-back (run after Task 8)

**Files:**
- Modify: `web/src/components/layout/workspace-tree-context.tsx`
- Modify: `web/src/components/layout/workspace-tree-item.tsx`

**Key facts:**
- `useSidebarStore.getState().reparentWorkspace(wsId, newParentId)` immediately updates the position in the store tree.
- `WorkspaceBranchIcon` shows a spinner when `working={true}`.
- `announceReparentOutcome` currently toasts success/warning — it can be deleted entirely.

- [ ] **Step 1: Add movingWsId to drag context**

In `workspace-tree-context.tsx`, update `WorkspaceTreeDragContextValue`:

```tsx
interface WorkspaceTreeDragContextValue {
  draggingWs: DraggingState | null
  hoverTargetId: string | null
  movingWsId: string | null  // wsId of item currently being moved (API in-flight)
}
```

Add state in provider:

```tsx
const [movingWsId, setMovingWsId] = useState<string | null>(null)
```

Add `movingWsId` to `dragValue` useMemo.

- [ ] **Step 2: Replace performReparentWorkspace with inline optimistic logic**

In the `onPointerUp` handler, the drop calls `void performReparentWorkspace(ws.id, targetWsId, ws.repoId)`. Replace with an inline async call:

```tsx
} else if (target?.startsWith('ws:')) {
  const targetWsId = target.slice(3)
  if (targetWsId !== ws.id) {
    const repos = useSidebarStore.getState().repos
    const targetRepo = repos.find((r) => r.workspaces.some((w) => w.id === targetWsId))
    if (targetRepo?.id === ws.repoId) {
      // Capture original parent before optimistic move
      const originalParentId = repos
        .flatMap((r) => r.workspaces)
        .find((w) => w.id === ws.id)?.parentId

      // Optimistic: move immediately in store
      useSidebarStore.getState().reparentWorkspace(ws.id, targetWsId)
      setMovingWsId(ws.id)

      const projectId = projectIdForRepo(ws.repoId)
      if (projectId) {
        reparentWorkspace(projectId, ws.repoId, ws.id, targetWsId)
          .catch(() => {
            // Snap back on failure
            useSidebarStore.getState().reparentWorkspace(ws.id, originalParentId)
          })
          .finally(() => {
            setMovingWsId(null)
          })
      } else {
        // Can't move — revert immediately
        useSidebarStore.getState().reparentWorkspace(ws.id, originalParentId)
        setMovingWsId(null)
      }
    }
  }
}
```

Check the actual import for `reparentWorkspace` API call:
```bash
grep -n "import.*reparentWorkspace" web/src/components/layout/workspace-tree-context.tsx
```

It's already imported from the workspace API — just use it directly.

- [ ] **Step 3: Delete announceReparentOutcome and its call**

Remove the entire `announceReparentOutcome` function (lines ~107–161 in the original file). It toasted success/warning; conflicts are handled by the WS pr-conflicts signal, and success is silent.

Also remove the `performReparentWorkspace` standalone exported function (or strip its `toast.error` calls if other callers exist — check with `grep -rn "performReparentWorkspace" web/src/`).

- [ ] **Step 4: Render spinner on moving item in workspace-tree-item.tsx**

In `WorkspaceTreeItem`, consume `movingWsId` from drag context:

```tsx
const { draggingWs, hoverTargetId, movingWsId } = useWorkspaceTreeDrag()
const isMoving = movingWsId === workspace.id
```

Pass to `WorkspaceBranchIcon`:

```tsx
<WorkspaceBranchIcon
  status={workspace.status ?? 'new'}
  working={workspace.working || isMoving}
/>
```

And dim the row:

```tsx
className={cn(
  ROW_BASE,
  variant,
  isDraggingThis && 'opacity-40',
  isMoving && 'opacity-50 pointer-events-none',
  isDropTarget && 'ring-1 ring-ring',
)}
```

- [ ] **Step 5: Remove toast calls from performDeleteWorkspace**

In `workspace-tree-context.tsx`, the standalone `performDeleteWorkspace` function (around line 56) has two `toast.error` calls. Remove them — delete failures are silent (the item remains in the list via WS, which is the signal that delete didn't land):

```tsx
export async function performDeleteWorkspace(wsId: string): Promise<void> {
  const repo = useSidebarStore
    .getState()
    .repos.find((r) => r.workspaces.some((w) => w.id === wsId))
  const ws = repo?.workspaces.find((w) => w.id === wsId)
  if (!repo || !ws || ws.status === 'locked') return
  const projectId = repo.projectId
  if (!projectId) return  // was: toast.error('Failed to delete workspace', 'unknown project for repo')
  try {
    await apiDeleteWorkspace(projectId, repo.id, wsId)
  } catch (err) {
    console.error('Failed to delete workspace:', err)
    // item stays in list via WS non-arrival — no toast needed
  }
}
```

- [ ] **Step 6: Verify types**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-levitated-helium-84aa/web && npx tsc --noEmit 2>&1 | grep "workspace-tree"
```

Expected: no errors.

- [ ] **Step 7: Full tsc clean build**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-levitated-helium-84aa/web && npx tsc --noEmit 2>&1 | head -40
```

Expected: no output.

- [ ] **Step 8: Commit**

```bash
git add web/src/components/layout/workspace-tree-context.tsx web/src/components/layout/workspace-tree-item.tsx
git commit -m "feat(workspace): optimistic move — snap-back on error, spinner during in-flight"
```

---

## Final Verification

- [ ] Run `cd web && npx tsc --noEmit` — must be clean.
- [ ] `grep -rn "toast\." web/src --include="*.tsx" --include="*.ts" | grep -v "toast-store\|toast\.tsx\|editor-app-store\|editor-status-actions\|developer-settings\|__tests__"` — should return only lines in files explicitly kept as toasts (add-repository-modal, import-project-modal).
- [ ] Verify in the running Tauri app: push a branch, watch the button show "Pushing…" and status line update to "Up to date" without any toast appearing.
