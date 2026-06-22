# Git Sidebar — Unified Branch Section Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the "commit panel OR merge section" either/or at the bottom of the Git sidebar's Changes tab with one parent-anchored `BranchSection` whose primary action reflects the branch state (commit / merge / sync), with commit-in-a-modal and merge-via-popover.

**Architecture:** A pure `resolveBranchAction` state machine drives a `BranchSection` component; committing opens a `CommitDialog` (message + per-file staging), merging opens a `MergePopover` (strategy + confirm). All state comes from existing stores (`useGitStore`, the sidebar workspace record, `branchReview.mergeStrategy`); all mutations use existing APIs. The old `GitCommitPanel` + `MergeSection` are retired.

**Tech Stack:** React 19 + zustand + Tailwind v4 + base-ui (`@/components/ui/*`), Vitest + @testing-library/react.

## Global Constraints

- Use `@/components/ui/*` components and CSS-variable tokens only — never hardcode colors. kebab-case filenames; PascalCase component names.
- zustand: narrow selectors `useXxxStore((s) => s.field)`; `getState()` only in handlers/effects; stores never import components.
- Tests live in `web/src/__tests__/` mirroring `web/src/`; use `@/` imports. Run with `cd web && ./node_modules/.bin/vitest run <path>`.
- `cd web && ./node_modules/.bin/tsc --noEmit` must report NO new errors (one pre-existing `otp-field.tsx` error is unrelated — ignore only that one).
- pnpm only — never `npm install`. All needed deps already present.
- WorkspaceStatus conflict value is the exact string `'pr-conflicts'`.
- Merge strategies are exactly `'merge' | 'squash' | 'rebase'` (`MergeStrategy`).
- The single refresh signal after any mutation is `window.dispatchEvent(new Event('git-status-changed'))`.
- Toasts: import `toast` from `@/features/window/stores/toast-store` (API: `toast.error(msg, desc?)`, `toast.info(msg)`, `toast.success(msg)`, `toast.warning(msg, desc?)`).

## File Structure

- `web/src/features/git/lib/branch-action.ts` — pure `resolveBranchAction` state machine + types. (Task 1)
- `web/src/features/git/api/git-status-api.ts` — add `stagePaths` / `unstagePaths` exports. (Task 2)
- `web/src/features/git/components/commit-dialog.tsx` — the commit modal. (Task 2)
- `web/src/features/git/components/merge-popover.tsx` — the merge-strategy popover. (Task 3)
- `web/src/features/git/components/branch-section.tsx` — the unified section. (Task 4)
- `web/src/features/git/components/git-panel.tsx` — wire `BranchSection`; remove the either/or. (Task 5)
- Delete: `git-commit-panel.tsx`, `merge-section.tsx`, `lib/merge-section-state.ts` + their tests. (Task 5)

---

### Task 1: `resolveBranchAction` state machine

**Files:**
- Create: `web/src/features/git/lib/branch-action.ts`
- Test: `web/src/__tests__/features/git/lib/branch-action.test.ts`

**Interfaces:**
- Produces: `resolveBranchAction(input: BranchActionInput): BranchAction`, where
  `BranchActionInput = { hasUncommitted: boolean; hasParent: boolean; canMergeLocally: boolean; status: string; ahead: number; behind: number }`,
  `BranchAction = { kind: 'commit' | 'resolve' | 'pull-request' | 'merge' | 'sync-only'; remote: 'push' | 'pull' | null }`.

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/features/git/lib/branch-action.test.ts
import { describe, it, expect } from 'vitest'
import { resolveBranchAction } from '@/features/git/lib/branch-action'

const base = {
  hasUncommitted: false,
  hasParent: true,
  canMergeLocally: true,
  status: 'new',
  ahead: 0,
  behind: 0,
}

describe('resolveBranchAction', () => {
  it('uncommitted changes → commit (overrides everything, no remote secondary)', () => {
    expect(resolveBranchAction({ ...base, hasUncommitted: true, ahead: 2, status: 'pr-conflicts' }))
      .toEqual({ kind: 'commit', remote: null })
  })

  it('clean + conflicts → resolve', () => {
    expect(resolveBranchAction({ ...base, status: 'pr-conflicts' }).kind).toBe('resolve')
  })

  it('clean + protected parent → pull-request', () => {
    expect(resolveBranchAction({ ...base, canMergeLocally: false }).kind).toBe('pull-request')
  })

  it('clean + mergeable parent → merge', () => {
    expect(resolveBranchAction(base).kind).toBe('merge')
  })

  it('clean + no parent → sync-only', () => {
    expect(resolveBranchAction({ ...base, hasParent: false }).kind).toBe('sync-only')
  })

  it('remote secondary: ahead → push, behind → pull, diverged → pull, synced → null (clean only)', () => {
    expect(resolveBranchAction({ ...base, ahead: 1 }).remote).toBe('push')
    expect(resolveBranchAction({ ...base, behind: 1 }).remote).toBe('pull')
    expect(resolveBranchAction({ ...base, ahead: 1, behind: 1 }).remote).toBe('pull')
    expect(resolveBranchAction(base).remote).toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && ./node_modules/.bin/vitest run src/__tests__/features/git/lib/branch-action.test.ts`
Expected: FAIL — cannot import `resolveBranchAction`.

- [ ] **Step 3: Write the implementation**

```ts
// web/src/features/git/lib/branch-action.ts

/** Which primary action the branch section offers, given the repo state. */
export type BranchActionKind =
  | 'commit' // uncommitted changes → open the commit dialog
  | 'resolve' // merge conflicts must be resolved
  | 'pull-request' // parent is protected → open a PR
  | 'merge' // mergeable into parent → open the merge popover
  | 'sync-only' // no parent (or nothing to merge) → push/pull only

export interface BranchActionInput {
  hasUncommitted: boolean
  hasParent: boolean
  canMergeLocally: boolean
  status: string
  ahead: number
  behind: number
}

export interface BranchAction {
  kind: BranchActionKind
  /** Remote secondary action shown alongside the primary (only when clean). */
  remote: 'push' | 'pull' | null
}

/**
 * Resolve the branch section's primary + secondary action from the current
 * repo state. Precedence: uncommitted (commit first) > conflict > protected >
 * mergeable > sync-only. The remote secondary is only offered on a clean tree
 * (you commit before you push/pull); behind wins over ahead when diverged.
 */
export function resolveBranchAction(input: BranchActionInput): BranchAction {
  const { hasUncommitted, hasParent, canMergeLocally, status, ahead, behind } = input

  const remote: BranchAction['remote'] = hasUncommitted
    ? null
    : behind > 0
      ? 'pull'
      : ahead > 0
        ? 'push'
        : null

  if (hasUncommitted) return { kind: 'commit', remote }
  if (hasParent && status === 'pr-conflicts') return { kind: 'resolve', remote }
  if (hasParent && !canMergeLocally) return { kind: 'pull-request', remote }
  if (hasParent && canMergeLocally) return { kind: 'merge', remote }
  return { kind: 'sync-only', remote }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && ./node_modules/.bin/vitest run src/__tests__/features/git/lib/branch-action.test.ts`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/git/lib/branch-action.ts web/src/__tests__/features/git/lib/branch-action.test.ts
git commit -m "feat(git-sidebar): resolveBranchAction state machine"
```

---

### Task 2: `CommitDialog` (+ `stagePaths`/`unstagePaths` API)

**Files:**
- Modify: `web/src/features/git/api/git-status-api.ts` (add two exports near the existing `stageFile`/`unstageFile`)
- Create: `web/src/features/git/components/commit-dialog.tsx`
- Test: `web/src/__tests__/features/git/components/commit-dialog.test.tsx`

**Interfaces:**
- Consumes: `commitChanges(wsId, message): Promise<boolean>` (`../api/git-commits-api`); `stagePaths(wsId, paths)`, `unstagePaths(wsId, paths)` (`../api/git-status-api`); `GitFile = { path: string; status: string; staged: boolean }` (`../types/git-types`).
- Produces: `CommitDialog` with props `{ open: boolean; onOpenChange: (open: boolean) => void; wsId: string; files: GitFile[]; onCommitted: () => void }`.

- [ ] **Step 1: Add the staging-by-paths API helpers**

In `web/src/features/git/api/git-status-api.ts`, directly after the existing `unstageFile` export, add:

```ts
export const stagePaths = (wsId: string, paths: string[]): Promise<boolean> =>
  gitPost(wsId, 'stage', { paths })

export const unstagePaths = (wsId: string, paths: string[]): Promise<boolean> =>
  gitPost(wsId, 'unstage', { paths })
```

- [ ] **Step 2: Write the failing test**

```tsx
// web/src/__tests__/features/git/components/commit-dialog.test.tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { CommitDialog } from '@/features/git/components/commit-dialog'
import type { GitFile } from '@/features/git/types/git-types'

const { commitChanges, stagePaths, unstagePaths } = vi.hoisted(() => ({
  commitChanges: vi.fn().mockResolvedValue(true),
  stagePaths: vi.fn().mockResolvedValue(true),
  unstagePaths: vi.fn().mockResolvedValue(true),
}))
vi.mock('@/features/git/api/git-commits-api', () => ({ commitChanges }))
vi.mock('@/features/git/api/git-status-api', () => ({ stagePaths, unstagePaths }))

const files: GitFile[] = [
  { path: 'a.ts', status: 'modified', staged: false },
  { path: 'b.ts', status: 'modified', staged: false },
]

beforeEach(() => vi.clearAllMocks())

describe('CommitDialog', () => {
  it('renders the message box and a checkbox per file (all checked by default)', () => {
    render(<CommitDialog open onOpenChange={vi.fn()} wsId="w1" files={files} onCommitted={vi.fn()} />)
    expect(screen.getByPlaceholderText('Commit message…')).toBeDefined()
    expect(screen.getByText('a.ts')).toBeDefined()
    expect(screen.getByText('b.ts')).toBeDefined()
  })

  it('Commit is disabled until there is a message and at least one file', async () => {
    const user = userEvent.setup()
    render(<CommitDialog open onOpenChange={vi.fn()} wsId="w1" files={files} onCommitted={vi.fn()} />)
    const commit = screen.getByRole('button', { name: 'Commit' })
    expect(commit).toBeDisabled()
    await user.type(screen.getByPlaceholderText('Commit message…'), 'msg')
    expect(commit).not.toBeDisabled()
  })

  it('committing stages the checked files, unstages the unchecked, commits, then closes', async () => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    const onCommitted = vi.fn()
    render(<CommitDialog open onOpenChange={onOpenChange} wsId="w1" files={files} onCommitted={onCommitted} />)
    // Uncheck b.ts
    await user.click(screen.getByText('b.ts'))
    await user.type(screen.getByPlaceholderText('Commit message…'), 'my commit')
    await user.click(screen.getByRole('button', { name: 'Commit' }))

    expect(stagePaths).toHaveBeenCalledWith('w1', ['a.ts'])
    expect(unstagePaths).toHaveBeenCalledWith('w1', ['b.ts'])
    expect(commitChanges).toHaveBeenCalledWith('w1', 'my commit')
    expect(onCommitted).toHaveBeenCalled()
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd web && ./node_modules/.bin/vitest run src/__tests__/features/git/components/commit-dialog.test.tsx`
Expected: FAIL — cannot import `CommitDialog`.

- [ ] **Step 4: Write the implementation**

```tsx
// web/src/features/git/components/commit-dialog.tsx
import { useEffect, useState } from 'react'
import { Dialog, DialogPopup, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Textarea } from '@/components/ui/textarea'
import { commitChanges } from '../api/git-commits-api'
import { stagePaths, unstagePaths } from '../api/git-status-api'
import type { GitFile } from '../types/git-types'

interface CommitDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  wsId: string
  files: GitFile[]
  /** Fired after a successful commit (refresh the section + diffs). */
  onCommitted: () => void
}

export function CommitDialog({ open, onOpenChange, wsId, files, onCommitted }: CommitDialogProps) {
  const [message, setMessage] = useState('')
  const [checked, setChecked] = useState<Set<string>>(() => new Set(files.map((f) => f.path)))
  const [isCommitting, setIsCommitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Reset to a fresh state (all files checked, empty message) each time it opens.
  useEffect(() => {
    if (!open) return
    setChecked(new Set(files.map((f) => f.path)))
    setMessage('')
    setError(null)
    setIsCommitting(false)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const canCommit = message.trim().length > 0 && checked.size > 0 && !isCommitting

  const toggle = (path: string) =>
    setChecked((prev) => {
      const next = new Set(prev)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })

  const handleCommit = async () => {
    if (!canCommit) return
    setIsCommitting(true)
    setError(null)
    const stage = files.filter((f) => checked.has(f.path)).map((f) => f.path)
    const unstage = files.filter((f) => !checked.has(f.path)).map((f) => f.path)
    try {
      if (stage.length) await stagePaths(wsId, stage)
      if (unstage.length) await unstagePaths(wsId, unstage)
      const ok = await commitChanges(wsId, message.trim())
      if (!ok) {
        setError('Failed to commit changes')
        return
      }
      onOpenChange(false)
      onCommitted()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to commit changes')
    } finally {
      setIsCommitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPopup className="max-w-md">
        <DialogHeader>
          <DialogTitle>Commit changes</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-3 px-6">
          {error && (
            <div className="rounded-md border border-destructive/30 bg-destructive/10 px-2.5 py-1.5 ui-text-xs text-destructive">
              {error}
            </div>
          )}
          <Textarea
            autoFocus
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                e.preventDefault()
                void handleCommit()
              }
            }}
            placeholder="Commit message…"
            className="ui-font ui-text-sm min-h-20 resize-none"
          />
          <div>
            <div className="ui-text-xs mb-1.5 text-muted-foreground">
              {files.length} file{files.length !== 1 ? 's' : ''}
            </div>
            <div className="flex max-h-48 flex-col gap-1 overflow-auto">
              {files.map((f) => (
                <label key={f.path} className="ui-text-sm flex cursor-pointer items-center gap-2">
                  <Checkbox checked={checked.has(f.path)} onChange={() => toggle(f.path)} />
                  <span className="truncate">{f.path}</span>
                </label>
              ))}
            </div>
          </div>
        </div>
        <DialogFooter>
          <span className="ui-text-xs mr-auto self-center text-muted-foreground">⌘↵ to commit</span>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="default" size="sm" disabled={!canCommit} onClick={() => void handleCommit()}>
            {isCommitting ? 'Committing…' : 'Commit'}
          </Button>
        </DialogFooter>
      </DialogPopup>
    </Dialog>
  )
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd web && ./node_modules/.bin/vitest run src/__tests__/features/git/components/commit-dialog.test.tsx`
Expected: PASS (3 tests). If the base-ui Dialog needs the portal, the tests still find content via `screen` (base-ui portals to body).

- [ ] **Step 6: Commit**

```bash
git add web/src/features/git/api/git-status-api.ts web/src/features/git/components/commit-dialog.tsx web/src/__tests__/features/git/components/commit-dialog.test.tsx
git commit -m "feat(git-sidebar): CommitDialog with per-file staging"
```

---

### Task 3: `MergePopover`

**Files:**
- Create: `web/src/features/git/components/merge-popover.tsx`
- Test: `web/src/__tests__/features/git/components/merge-popover.test.tsx`

**Interfaces:**
- Consumes: `setMergeStrategy(wsId, strategy): Promise<MergeStrategy>` + `mergeIntoParent(wsId, strategy): Promise<void>` (`../api/review-api`); `MergeStrategy = 'merge' | 'squash' | 'rebase'` (`@/features/workspace/stores/slices/branch-review-slice`); `useWorkspaceStoreById` (`@/features/workspace/stores/hooks/use-workspace-store-by-id`); `getOrCreateWorkspaceStore` (`@/features/workspace/stores/workspace-store-registry`).
- Produces: `MergePopover` with props `{ wsId: string; parentBranch: string; trigger: React.ReactElement }`.

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/__tests__/features/git/components/merge-popover.test.tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { Button } from '@/components/ui/button'
import { MergePopover } from '@/features/git/components/merge-popover'

const { patchMergeStrategy, mergeIntoParent, setBranchReviewMergeStrategy } = vi.hoisted(() => ({
  patchMergeStrategy: vi.fn().mockResolvedValue('squash'),
  mergeIntoParent: vi.fn().mockResolvedValue(undefined),
  setBranchReviewMergeStrategy: vi.fn(),
}))
vi.mock('@/features/git/api/review-api', () => ({
  setMergeStrategy: patchMergeStrategy,
  mergeIntoParent,
}))
let strategy = 'merge'
vi.mock('@/features/workspace/stores/hooks/use-workspace-store-by-id', () => ({
  useWorkspaceStoreById: (_id: string, sel: (s: any) => unknown) =>
    sel({ branchReview: { mergeStrategy: strategy } }),
}))
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getOrCreateWorkspaceStore: () => ({ getState: () => ({ setBranchReviewMergeStrategy }) }),
}))

beforeEach(() => {
  vi.clearAllMocks()
  strategy = 'merge'
})

describe('MergePopover', () => {
  it('opens with the three strategies and a confirm matching the active strategy', async () => {
    const user = userEvent.setup()
    render(<MergePopover wsId="w1" parentBranch="develop" trigger={<Button>Merge into develop</Button>} />)
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    expect(await screen.findByText('Create a merge commit')).toBeDefined()
    expect(screen.getByText('Squash and merge')).toBeDefined()
    expect(screen.getByText('Rebase and merge')).toBeDefined()
    // Active strategy is 'merge' → confirm reads "Create merge commit"
    expect(screen.getByRole('button', { name: 'Create merge commit' })).toBeDefined()
  })

  it('confirm merges with the current strategy', async () => {
    const user = userEvent.setup()
    render(<MergePopover wsId="w1" parentBranch="develop" trigger={<Button>Merge into develop</Button>} />)
    await user.click(screen.getByRole('button', { name: 'Merge into develop' }))
    await user.click(await screen.findByRole('button', { name: 'Create merge commit' }))
    expect(mergeIntoParent).toHaveBeenCalledWith('w1', 'merge')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && ./node_modules/.bin/vitest run src/__tests__/features/git/components/merge-popover.test.tsx`
Expected: FAIL — cannot import `MergePopover`.

- [ ] **Step 3: Write the implementation**

```tsx
// web/src/features/git/components/merge-popover.tsx
import type { ReactElement } from 'react'
import { useState } from 'react'
import { Popover, PopoverTrigger, PopoverContent, PopoverTitle } from '@/components/ui/popover'
import { Button } from '@/components/ui/button'
import { RadioGroup, Radio } from '@/components/ui/radio-group'
import { toast } from '@/features/window/stores/toast-store'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { setMergeStrategy as patchMergeStrategy, mergeIntoParent } from '../api/review-api'
import type { MergeStrategy } from '@/features/workspace/stores/slices/branch-review-slice'

const STRATEGIES: { value: MergeStrategy; label: string; confirm: string; desc: string }[] = [
  { value: 'merge', label: 'Create a merge commit', confirm: 'Create merge commit', desc: 'All commits, plus a merge commit' },
  { value: 'squash', label: 'Squash and merge', confirm: 'Squash & merge', desc: 'One combined commit' },
  { value: 'rebase', label: 'Rebase and merge', confirm: 'Rebase & merge', desc: 'Replay commits, no merge commit' },
]

interface MergePopoverProps {
  wsId: string
  parentBranch: string
  /** The "Merge into …" button rendered as the popover trigger. */
  trigger: ReactElement
}

export function MergePopover({ wsId, parentBranch, trigger }: MergePopoverProps) {
  const [open, setOpen] = useState(false)
  const strategy = useWorkspaceStoreById(wsId, (s) => s.branchReview.mergeStrategy)
  const active = STRATEGIES.find((s) => s.value === strategy) ?? STRATEGIES[0]

  // Persist the chosen strategy so it becomes the default next time (optimistic).
  const selectStrategy = async (next: MergeStrategy) => {
    if (next === strategy) return
    const previous = strategy
    getOrCreateWorkspaceStore(wsId).getState().setBranchReviewMergeStrategy(next)
    try {
      await patchMergeStrategy(wsId, next)
    } catch {
      getOrCreateWorkspaceStore(wsId).getState().setBranchReviewMergeStrategy(previous)
      toast.error('Failed to update merge strategy')
    }
  }

  const handleMerge = async () => {
    setOpen(false)
    try {
      await mergeIntoParent(wsId, strategy)
      toast.info('Merging…')
    } catch {
      toast.error('Merge failed — check the logs for details')
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger render={trigger} />
      <PopoverContent align="start" className="w-64">
        <PopoverTitle className="text-sm">Merge into {parentBranch}</PopoverTitle>
        <p className="ui-text-xs mt-0.5 mb-3 text-muted-foreground">
          {parentBranch} is local &amp; unprotected
        </p>
        <RadioGroup
          value={strategy}
          onValueChange={(value) => void selectStrategy(value as MergeStrategy)}
          className="mb-3 gap-1"
        >
          {STRATEGIES.map((s) => (
            <label key={s.value} className="flex cursor-pointer items-start gap-2 rounded-md py-0.5">
              <Radio value={s.value} className="mt-0.5" />
              <div className="ui-text-sm">
                <div className="font-medium">{s.label}</div>
                <div className="ui-text-xs text-muted-foreground">{s.desc}</div>
              </div>
            </label>
          ))}
        </RadioGroup>
        <Button variant="default" size="sm" className="w-full" onClick={() => void handleMerge()}>
          {active.confirm}
        </Button>
      </PopoverContent>
    </Popover>
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && ./node_modules/.bin/vitest run src/__tests__/features/git/components/merge-popover.test.tsx`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/git/components/merge-popover.tsx web/src/__tests__/features/git/components/merge-popover.test.tsx
git commit -m "feat(git-sidebar): MergePopover (strategy + confirm)"
```

---

### Task 4: `BranchSection`

**Files:**
- Create: `web/src/features/git/components/branch-section.tsx`
- Test: `web/src/__tests__/features/git/components/branch-section.test.tsx`

**Interfaces:**
- Consumes: `resolveBranchAction` (`../lib/branch-action`); `CommitDialog` (`./commit-dialog`); `MergePopover` (`./merge-popover`); `pushChanges(wsId)`, `pullChanges(wsId)` → `{ success: boolean; error?: string }` (`../api/git-remotes-api`); `GitFile` (`../types/git-types`).
- Produces: `BranchSection` with props `{ wsId: string; branch: string; parentBranch?: string; canMergeLocally: boolean; status: string; ahead: number; behind: number; files: GitFile[] }`.

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/__tests__/features/git/components/branch-section.test.tsx
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { BranchSection } from '@/features/git/components/branch-section'
import type { GitFile } from '@/features/git/types/git-types'

// Stub the heavy children so this test isolates BranchSection's own rendering.
vi.mock('@/features/git/components/commit-dialog', () => ({ CommitDialog: () => null }))
vi.mock('@/features/git/components/merge-popover', () => ({
  MergePopover: ({ trigger }: { trigger: React.ReactElement }) => trigger,
}))
vi.mock('@/features/git/api/git-remotes-api', () => ({
  pushChanges: vi.fn().mockResolvedValue({ success: true }),
  pullChanges: vi.fn().mockResolvedValue({ success: true }),
}))

const base = {
  wsId: 'w1',
  branch: 'epoch/first-pr',
  parentBranch: 'develop',
  canMergeLocally: true,
  status: 'new',
  ahead: 0,
  behind: 0,
  files: [] as GitFile[],
}

describe('BranchSection', () => {
  it('shows the branch → parent header', () => {
    render(<BranchSection {...base} />)
    expect(screen.getByText('epoch/first-pr')).toBeDefined()
    expect(screen.getByText('develop')).toBeDefined()
  })

  it('uncommitted → Commit changes', () => {
    render(<BranchSection {...base} files={[{ path: 'a.ts', status: 'modified', staged: false }]} />)
    expect(screen.getByRole('button', { name: 'Commit changes' })).toBeDefined()
  })

  it('clean + mergeable → Merge into parent', () => {
    render(<BranchSection {...base} />)
    expect(screen.getByRole('button', { name: /Merge into develop/ })).toBeDefined()
  })

  it('clean + protected → Open a pull request', () => {
    render(<BranchSection {...base} canMergeLocally={false} />)
    expect(screen.getByRole('button', { name: /Open a pull request/ })).toBeDefined()
  })

  it('clean + conflicts → Resolve conflicts', () => {
    render(<BranchSection {...base} status="pr-conflicts" />)
    expect(screen.getByRole('button', { name: /Resolve conflicts/ })).toBeDefined()
  })

  it('clean + ahead → a Push secondary', () => {
    render(<BranchSection {...base} ahead={1} />)
    expect(screen.getByRole('button', { name: /Push/ })).toBeDefined()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && ./node_modules/.bin/vitest run src/__tests__/features/git/components/branch-section.test.tsx`
Expected: FAIL — cannot import `BranchSection`.

- [ ] **Step 3: Write the implementation**

```tsx
// web/src/features/git/components/branch-section.tsx
import { useState } from 'react'
import {
  GitBranch,
  ArrowUp,
  ArrowDown,
  ArrowSquareOut,
  Warning,
} from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { toast } from '@/features/window/stores/toast-store'
import { CommitDialog } from './commit-dialog'
import { MergePopover } from './merge-popover'
import { resolveBranchAction } from '../lib/branch-action'
import { pushChanges, pullChanges } from '../api/git-remotes-api'
import type { GitFile } from '../types/git-types'

interface BranchSectionProps {
  wsId: string
  branch: string
  parentBranch?: string
  canMergeLocally: boolean
  status: string
  ahead: number
  behind: number
  files: GitFile[]
}

export function BranchSection({
  wsId,
  branch,
  parentBranch,
  canMergeLocally,
  status,
  ahead,
  behind,
  files,
}: BranchSectionProps) {
  const [commitOpen, setCommitOpen] = useState(false)
  const [remoteBusy, setRemoteBusy] = useState(false)

  const action = resolveBranchAction({
    hasUncommitted: files.length > 0,
    hasParent: Boolean(parentBranch),
    canMergeLocally,
    status,
    ahead,
    behind,
  })

  const refresh = () => window.dispatchEvent(new Event('git-status-changed'))

  const runRemote = async (kind: 'push' | 'pull') => {
    setRemoteBusy(true)
    try {
      const res = kind === 'push' ? await pushChanges(wsId) : await pullChanges(wsId)
      if (res.success) {
        toast.success(kind === 'push' ? 'Changes pushed' : 'Changes pulled')
        refresh()
      } else {
        toast.error(res.error || `Failed to ${kind}`)
      }
    } catch (e) {
      toast.error(e instanceof Error ? e.message : `Failed to ${kind}`)
    } finally {
      setRemoteBusy(false)
    }
  }

  const statusLine = (() => {
    if (action.kind === 'commit') {
      return `${files.length} uncommitted change${files.length !== 1 ? 's' : ''}`
    }
    if (action.kind === 'resolve') return 'Merge conflicts'
    if (action.kind === 'pull-request') return `${parentBranch} is protected`
    if (behind > 0) return `Clean · ${behind} behind`
    if (ahead > 0) return `Clean · ${ahead} to push`
    return 'Up to date'
  })()

  return (
    <div className="flex flex-col gap-2 p-3">
      <div className="ui-text-sm flex items-center gap-1.5">
        <GitBranch className="size-3.5 text-muted-foreground" />
        <span className="font-mono font-medium">{branch}</span>
        {parentBranch && (
          <>
            <span className="text-muted-foreground">→</span>
            <span className="font-mono text-muted-foreground">{parentBranch}</span>
          </>
        )}
      </div>
      <div
        className={cn(
          'ui-text-xs',
          action.kind === 'resolve' ? 'text-destructive' : 'text-muted-foreground',
        )}
      >
        {statusLine}
      </div>

      <div className="flex items-center gap-2">
        {action.kind === 'commit' && (
          <Button variant="default" size="sm" className="flex-1" onClick={() => setCommitOpen(true)}>
            Commit changes
          </Button>
        )}

        {action.kind === 'resolve' && (
          <Button
            variant="destructive"
            size="sm"
            className="flex-1"
            onClick={() =>
              toast.warning(
                'Open the conflicting files and resolve conflicts, then commit.',
                'Merge conflicts detected',
              )
            }
          >
            <Warning className="size-3.5" />
            Resolve conflicts
          </Button>
        )}

        {action.kind === 'pull-request' && (
          <Button
            variant="outline"
            size="sm"
            className="flex-1"
            onClick={() => toast.info(`${parentBranch} is protected — open a pull request to merge.`)}
          >
            Open a pull request
            <ArrowSquareOut className="size-3.5" />
          </Button>
        )}

        {action.kind === 'merge' && parentBranch && (
          <MergePopover
            wsId={wsId}
            parentBranch={parentBranch}
            trigger={
              <Button variant="default" size="sm" className="flex-1">
                <GitBranch className="size-3.5" />
                Merge into {parentBranch}
              </Button>
            }
          />
        )}

        {action.remote && (
          <Button
            variant="outline"
            size="sm"
            disabled={remoteBusy}
            onClick={() => void runRemote(action.remote!)}
          >
            {action.remote === 'push' ? (
              <ArrowUp className="size-3.5" />
            ) : (
              <ArrowDown className="size-3.5" />
            )}
            {action.remote === 'push'
              ? `Push${ahead ? ` ${ahead}` : ''}`
              : `Pull${behind ? ` ${behind}` : ''}`}
          </Button>
        )}
      </div>

      <CommitDialog
        open={commitOpen}
        onOpenChange={setCommitOpen}
        wsId={wsId}
        files={files}
        onCommitted={refresh}
      />
    </div>
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && ./node_modules/.bin/vitest run src/__tests__/features/git/components/branch-section.test.tsx`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/git/components/branch-section.tsx web/src/__tests__/features/git/components/branch-section.test.tsx
git commit -m "feat(git-sidebar): BranchSection (unified commit/merge/sync)"
```

---

### Task 5: Wire `BranchSection` into `git-panel`; retire the old panels

**Files:**
- Modify: `web/src/features/git/components/git-panel.tsx`
- Delete: `web/src/features/git/components/git-commit-panel.tsx`, `web/src/features/git/components/merge-section.tsx`, `web/src/features/git/lib/merge-section-state.ts`
- Delete (tests for the above, if present): `web/src/__tests__/features/git/lib/merge-section-state.test.ts` and any `git-commit-panel` / `merge-section` test.

**Interfaces:**
- Consumes: `BranchSection` (`./branch-section`), `useGitStore`, `useSidebarStore` (already imported in git-panel).

- [ ] **Step 1: Verify nothing else imports the files being deleted**

Run:
```bash
cd web && grep -rln "git-commit-panel\|GitCommitPanel\|merge-section'\|MergeSection\|merge-section-state\|resolveMergeState" src | grep -v "src/features/git/components/git-panel.tsx"
```
Expected: no output (only `git-panel.tsx` references them). If anything else appears, STOP and report — it must be handled before deleting.

- [ ] **Step 2: Replace the either/or block in `git-panel.tsx`**

In `web/src/features/git/components/git-panel.tsx`:

Replace the imports
```tsx
import GitCommitPanel from './git-commit-panel'
import { MergeSection } from './merge-section'
```
with
```tsx
import { BranchSection } from './branch-section'
```

Replace the bottom-region block (the `{uncommittedCount > 0 || !(wsId && activeWs?.parentBranch) ? (… GitCommitPanel …) : (… MergeSection …)}` JSX) with:

```tsx
{wsId ? (
  <div className="shrink-0 border-t border-border">
    <BranchSection
      wsId={wsId}
      branch={activeWs?.branch ?? gitStatus?.branch ?? ''}
      parentBranch={activeWs?.parentBranch}
      canMergeLocally={activeWs?.canMergeLocally ?? false}
      status={activeWs?.status ?? 'new'}
      ahead={gitStatus?.ahead ?? 0}
      behind={gitStatus?.behind ?? 0}
      files={gitStatus?.files ?? []}
    />
  </div>
) : null}
```

`staged` and `uncommittedCount` may now be unused in `git-panel.tsx`; remove the now-dead `const staged = …` line and, if `uncommittedCount` is no longer referenced, drop it from the `useReviewDiff` destructure (keep `files` — `ChangedFilesTree` still uses it). Do not remove `useReviewDiff` itself.

- [ ] **Step 3: Delete the retired files**

```bash
cd web
git rm src/features/git/components/git-commit-panel.tsx src/features/git/components/merge-section.tsx src/features/git/lib/merge-section-state.ts
git rm src/__tests__/features/git/lib/merge-section-state.test.ts 2>/dev/null || true
```
(If `git rm` reports a test path that doesn't exist, ignore it — the `|| true` handles it. Also remove any `git-commit-panel`/`merge-section` component test the grep in Step 1 surfaced.)

- [ ] **Step 4: Typecheck + run the git suite**

Run:
```bash
cd web && ./node_modules/.bin/tsc --noEmit 2>&1 | grep -v otp-field.tsx | grep -c "error TS"
```
Expected: `0`.

Run:
```bash
cd web && ./node_modules/.bin/vitest run src/__tests__/features/git
```
Expected: all green (the deleted merge-section-state test is gone; the new branch-section/commit-dialog/merge-popover/branch-action tests pass).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/git/components/git-panel.tsx
git commit -m "feat(git-sidebar): wire BranchSection; retire commit panel + merge section"
```

---

## Post-plan verification (executor, not a TDD task)

After Task 5: rebuild nothing (FE only, HMR). In the running Tauri app, open the Changes tab and verify each state live: dirty → "Commit changes" → modal stages + commits; clean+ahead → "Merge into {parent}" opens the popover (strategy + confirm) and "Push" works; protected parent → "Open a pull request"; conflict → "Resolve conflicts". Then run the adversarial review workflow over the branch diff and fix confirmed findings. Per the standing rule, do not claim done until sampled live in Tauri.

## Self-Review

**Spec coverage:** unified parent-anchored section (Task 4) ✓; state machine incl. precedence + remote secondary (Task 1) ✓; commit modal with per-file staging (Task 2) ✓; merge popover with relabel-per-strategy + remembered default (Task 3) ✓; protected/conflict/no-parent edge states (Task 4) ✓; retire old components + wire git-panel (Task 5) ✓; `git-status-changed` refresh (Tasks 2/4) ✓; testing plan (each task) ✓. YAGNI: no conflict resolver / no PR-creation flow — both kept as the existing toast/info behavior ✓.

**Placeholder scan:** no TBD/TODO/"similar to"; every code step has complete code; commands have expected output.

**Type consistency:** `BranchAction.kind` values are identical across Task 1 (definition), Task 4 (`action.kind ===` checks). `CommitDialog` props `{open,onOpenChange,wsId,files,onCommitted}` match Task 4's usage. `MergePopover` props `{wsId,parentBranch,trigger}` match Task 4's usage. `pushChanges`/`pullChanges` return `{success,error?}` (used in Task 4). `GitFile.path` used consistently. `MergeStrategy` union matches the API.
