# Workspace Switcher Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the context pill open a searchable workspace switcher (an anchored Popover + the coss Command menu) that lists every workspace across repos and navigates to the selected one — without changing the sidebar tab/content.

**Architecture:** Pure, unit-tested helpers (`formatChangeCount`, `flattenWorkspaces`, `filterWorkspaces`) feed a `WorkspaceSwitcherMenu` (the Command menu: input + filtered list + select→navigate). `ContextPill` wraps its existing pill button as a `PopoverTrigger` and renders the menu in `PopoverPopup`. Navigation uses the same `navigate({ to: '/workspaces/$wsId' })` call the tree uses, which is orthogonal to the sidebar `activeTab`.

**Tech Stack:** React + TypeScript, Zustand, TanStack Router, base-ui `Popover`/`Autocomplete` (via `@/components/ui/popover` + `@/components/ui/command`), Vitest + Testing Library, Tailwind/coss tokens.

**Spec:** `docs/superpowers/specs/2026-06-15-workspace-switcher-design.md`

---

## File Structure

- **Create** `web/src/components/layout/format-change-count.ts` — `formatChangeCount(n)` (shared `+12k`/`69` formatting).
- **Create** `web/src/components/layout/workspace-switcher-model.ts` — `WorkspaceSwitcherItem`, `flattenWorkspaces`, `filterWorkspaces` (pure).
- **Create** `web/src/components/layout/workspace-switcher.tsx` — `WorkspaceSwitcherMenu` (Command menu; reads stores/route, navigates, calls `onClose`).
- **Modify** `web/src/components/layout/context-pill.tsx` — wrap the pill button as a `Popover` trigger that opens `WorkspaceSwitcherMenu`; drop the `setActiveTab` click; `aria-label` → "Switch workspace".
- **Modify** `web/src/components/layout/workspace-tree-item.tsx` — use `formatChangeCount` (DRY).
- **Tests** under `web/src/__tests__/components/layout/` mirroring the above.

Conventions (CLAUDE.md): kebab-case files, PascalCase exports, narrow store selectors in render, `getState()`/navigation only in handlers, `@/` imports in tests, tokens not hardcoded colors.

---

### Task 1: `formatChangeCount` helper + adopt in tree

**Files:**
- Create: `web/src/components/layout/format-change-count.ts`
- Test: `web/src/__tests__/components/layout/format-change-count.test.ts`
- Modify: `web/src/components/layout/workspace-tree-item.tsx:104-125`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/format-change-count.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { formatChangeCount } from '@/components/layout/format-change-count'

describe('formatChangeCount', () => {
  it('returns the number as-is at or below 999', () => {
    expect(formatChangeCount(0)).toBe('0')
    expect(formatChangeCount(69)).toBe('69')
    expect(formatChangeCount(999)).toBe('999')
  })

  it('compacts thousands with a rounded "k" suffix', () => {
    expect(formatChangeCount(1000)).toBe('1k')
    expect(formatChangeCount(1234)).toBe('1k')
    expect(formatChangeCount(1500)).toBe('2k')
    expect(formatChangeCount(12000)).toBe('12k')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/components/layout/format-change-count.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write minimal implementation**

Create `web/src/components/layout/format-change-count.ts`:

```ts
/**
 * Formats an added/deleted line count for compact display: values over 999
 * are rounded to a "k" suffix (e.g. 1500 → "2k"). Mirrors the formatting
 * used in the workspace tree so the switcher and tree render counts identically.
 */
export function formatChangeCount(n: number): string {
  return n > 999 ? `${Math.round(n / 1000)}k` : `${n}`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/components/layout/format-change-count.test.ts`
Expected: PASS (2 tests).

- [ ] **Step 5: Adopt the helper in the tree item (DRY)**

In `web/src/components/layout/workspace-tree-item.tsx`, add the import near the top with the other local imports:

```tsx
import { formatChangeCount } from './format-change-count'
```

Then replace the inline formatting at lines 104-125. Change:

```tsx
                {workspace.added !== undefined && workspace.added > 0 && (
                  <span className="text-green-300">
                    +
                    {workspace.added > 999
                      ? `${Math.round(workspace.added / 1000)}k`
                      : workspace.added}
                  </span>
                )}
                {workspace.deleted !== undefined && workspace.deleted > 0 && (
                  <span className="text-red-300">
                    -
                    {workspace.deleted > 999
                      ? `${Math.round(workspace.deleted / 1000)}k`
                      : workspace.deleted}
                  </span>
                )}
```

to:

```tsx
                {workspace.added !== undefined && workspace.added > 0 && (
                  <span className="text-green-300">+{formatChangeCount(workspace.added)}</span>
                )}
                {workspace.deleted !== undefined && workspace.deleted > 0 && (
                  <span className="text-red-300">-{formatChangeCount(workspace.deleted)}</span>
                )}
```

- [ ] **Step 6: Verify nothing broke**

Run: `cd web && npx tsc --noEmit && npx vitest run src/__tests__/components/layout/`
Expected: tsc clean (ignore the known pre-existing `semantic-tokens-provider.ts` error); layout tests pass.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/layout/format-change-count.ts web/src/__tests__/components/layout/format-change-count.test.ts web/src/components/layout/workspace-tree-item.tsx
git commit -m "refactor(sidebar): extract shared formatChangeCount helper"
```

---

### Task 2: Switcher model (flatten + filter)

**Files:**
- Create: `web/src/components/layout/workspace-switcher-model.ts`
- Test: `web/src/__tests__/components/layout/workspace-switcher-model.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/workspace-switcher-model.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import {
  flattenWorkspaces,
  filterWorkspaces,
} from '@/components/layout/workspace-switcher-model'
import type { Repo } from '@/lib/store/sidebar'

const repos: Repo[] = [
  {
    id: 'r1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ws1', branch: 'develop', status: 'pr-open', added: 1234, deleted: 5, age: '1d' },
      { id: 'ws2', branch: 'no-status', age: '2d' },
    ],
  },
  {
    id: 'r2',
    name: 'quiver.desktop',
    avatarLabel: 'Q',
    avatarColor: 'bg-teal-700',
    workspaces: [{ id: 'ws3', branch: 'feature/quiver-shell', status: 'new', age: '3d' }],
  },
]

describe('flattenWorkspaces', () => {
  it('flattens all repos into items with repo context', () => {
    const items = flattenWorkspaces(repos, 'ws3')
    expect(items).toHaveLength(3)
    expect(items[0]).toEqual({
      wsId: 'ws1',
      repoName: 'crowbar',
      branch: 'develop',
      status: 'pr-open',
      added: 1234,
      deleted: 5,
      isCurrent: false,
    })
  })

  it('marks the active workspace as current', () => {
    const items = flattenWorkspaces(repos, 'ws3')
    expect(items.find((i) => i.wsId === 'ws3')?.isCurrent).toBe(true)
    expect(items.filter((i) => i.isCurrent)).toHaveLength(1)
  })

  it('defaults a missing status to "new"', () => {
    const items = flattenWorkspaces(repos, undefined)
    expect(items.find((i) => i.wsId === 'ws2')?.status).toBe('new')
  })
})

describe('filterWorkspaces', () => {
  const items = flattenWorkspaces(repos, 'ws3')

  it('returns all items for an empty/whitespace query', () => {
    expect(filterWorkspaces(items, '')).toHaveLength(3)
    expect(filterWorkspaces(items, '   ')).toHaveLength(3)
  })

  it('matches on branch name', () => {
    const r = filterWorkspaces(items, 'quiver-shell')
    expect(r).toHaveLength(1)
    expect(r[0].wsId).toBe('ws3')
  })

  it('matches on repo name', () => {
    const r = filterWorkspaces(items, 'crowbar')
    expect(r.map((i) => i.wsId)).toEqual(['ws1', 'ws2'])
  })

  it('is case-insensitive', () => {
    expect(filterWorkspaces(items, 'DEVELOP')).toHaveLength(1)
  })

  it('returns empty when nothing matches', () => {
    expect(filterWorkspaces(items, 'zzzzz')).toEqual([])
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/components/layout/workspace-switcher-model.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write minimal implementation**

First confirm the matcher signature: open `web/src/utils/search-match.ts` and verify `matchesSearchQuery(query: string, candidates: string[]): boolean` exists and returns `true` for an empty query. If the empty-query behavior differs, keep the explicit empty-query guard below regardless.

Create `web/src/components/layout/workspace-switcher-model.ts`:

```ts
import type { Repo, WorkspaceStatus } from '@/lib/store/sidebar'
import { matchesSearchQuery } from '@/utils/search-match'

export interface WorkspaceSwitcherItem {
  wsId: string
  repoName: string
  branch: string
  status: WorkspaceStatus
  added?: number
  deleted?: number
  isCurrent: boolean
}

/** Flattens repos → a flat list of workspaces with repo context and current-state. */
export function flattenWorkspaces(
  repos: Repo[],
  activeWorkspaceId: string | undefined,
): WorkspaceSwitcherItem[] {
  return repos.flatMap((repo) =>
    repo.workspaces.map((ws) => ({
      wsId: ws.id,
      repoName: repo.name,
      branch: ws.branch,
      status: ws.status ?? 'new',
      added: ws.added,
      deleted: ws.deleted,
      isCurrent: ws.id === activeWorkspaceId,
    })),
  )
}

/** Filters items by a query against repo name and branch (empty query = all). */
export function filterWorkspaces(
  items: WorkspaceSwitcherItem[],
  query: string,
): WorkspaceSwitcherItem[] {
  if (!query.trim()) return items
  return items.filter((item) => matchesSearchQuery(query, [item.repoName, item.branch]))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/components/layout/workspace-switcher-model.test.ts`
Expected: PASS. If `filterWorkspaces` repo-name test fails because `matchesSearchQuery` argument order/behavior differs, fix the call to match the real signature found in Step 3 (do not change the tests' expectations).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/workspace-switcher-model.ts web/src/__tests__/components/layout/workspace-switcher-model.test.ts
git commit -m "feat(sidebar): workspace switcher model (flatten + filter)"
```

---

### Task 3: `WorkspaceSwitcherMenu` component

**Files:**
- Create: `web/src/components/layout/workspace-switcher.tsx`
- Test: `web/src/__tests__/components/layout/workspace-switcher.test.tsx`

**Context:** This is the command-menu *content* (input + filtered list + select). It reads `repos` and the active workspace id, owns the query string, and on select calls `navigate(...)` then `onClose()`. `ContextPill` (Task 4) mounts it inside a `PopoverPopup`. Mirror the proven wiring in `web/src/features/command-palette/components/command-palette.tsx` (lines 376-443): `Command` → `CommandInput value/onChange` → render filtered `CommandItem`s → `CommandEmpty`. `CommandItem` accepts `onClick`.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/workspace-switcher.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { WorkspaceSwitcherMenu } from '@/components/layout/workspace-switcher'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'

const navigateMock = vi.fn()
let mockPathname = '/workspaces/ws3'
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigateMock,
  useRouterState: ({ select }: { select: (s: { location: { pathname: string } }) => unknown }) =>
    select({ location: { pathname: mockPathname } }),
}))

const repos: Repo[] = [
  {
    id: 'r1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [{ id: 'ws1', branch: 'develop', status: 'pr-open', age: '1d' }],
  },
  {
    id: 'r2',
    name: 'quiver.desktop',
    avatarLabel: 'Q',
    avatarColor: 'bg-teal-700',
    workspaces: [{ id: 'ws3', branch: 'feature/quiver-shell', status: 'new', age: '3d' }],
  },
]

beforeEach(() => {
  navigateMock.mockClear()
  mockPathname = '/workspaces/ws3'
  useSidebarStore.setState({ repos })
})

describe('WorkspaceSwitcherMenu', () => {
  it('renders a row for every workspace', () => {
    render(<WorkspaceSwitcherMenu onClose={() => {}} />)
    expect(screen.getByText('develop')).toBeInTheDocument()
    expect(screen.getByText('feature/quiver-shell')).toBeInTheDocument()
  })

  it('filters rows as the query changes', () => {
    render(<WorkspaceSwitcherMenu onClose={() => {}} />)
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'develop' } })
    expect(screen.getByText('develop')).toBeInTheDocument()
    expect(screen.queryByText('feature/quiver-shell')).not.toBeInTheDocument()
  })

  it('navigates to the chosen workspace and closes on select', () => {
    const onClose = vi.fn()
    render(<WorkspaceSwitcherMenu onClose={onClose} />)
    fireEvent.click(screen.getByText('develop'))
    expect(navigateMock).toHaveBeenCalledWith({ to: '/workspaces/$wsId', params: { wsId: 'ws1' } })
    expect(onClose).toHaveBeenCalled()
  })

  it('shows an empty state when nothing matches', () => {
    render(<WorkspaceSwitcherMenu onClose={() => {}} />)
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'zzzzz' } })
    expect(screen.getByText(/no workspaces/i)).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/components/layout/workspace-switcher.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the implementation**

Create `web/src/components/layout/workspace-switcher.tsx`:

```tsx
import { useState } from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Check } from '@phosphor-icons/react'
import { useSidebarStore } from '@/lib/store/sidebar'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import { formatChangeCount } from './format-change-count'
import { flattenWorkspaces, filterWorkspaces } from './workspace-switcher-model'

interface WorkspaceSwitcherMenuProps {
  /** Called after a workspace is selected (host closes the popover). */
  onClose: () => void
}

/**
 * Searchable command menu listing every workspace across repos. Selecting one
 * navigates the route only — the sidebar tab/content is never touched.
 */
export function WorkspaceSwitcherMenu({ onClose }: WorkspaceSwitcherMenuProps) {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const repos = useSidebarStore((s) => s.repos)
  const [query, setQuery] = useState('')

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const items = filterWorkspaces(flattenWorkspaces(repos, activeWorkspaceId), query)

  function select(wsId: string) {
    void navigate({ to: '/workspaces/$wsId', params: { wsId } })
    onClose()
  }

  return (
    <Command className="w-full">
      <CommandInput value={query} onChange={setQuery} placeholder="Switch workspace…" />
      <CommandList>
        {items.length === 0 ? (
          <CommandEmpty>No workspaces found</CommandEmpty>
        ) : (
          items.map((item) => (
            <CommandItem
              key={item.wsId}
              onClick={() => select(item.wsId)}
              className="flex items-center gap-2 px-3 py-1.5 font-mono"
            >
              <WorkspaceBranchIcon status={item.status} />
              <span className="min-w-0 flex-1 truncate text-[13px]">
                <span className="text-muted-foreground">{item.repoName} / </span>
                <span className="text-foreground">{item.branch}</span>
              </span>
              {(item.added ?? 0) > 0 && (
                <span className="shrink-0 text-green-300">+{formatChangeCount(item.added ?? 0)}</span>
              )}
              {(item.deleted ?? 0) > 0 && (
                <span className="shrink-0 text-red-300">-{formatChangeCount(item.deleted ?? 0)}</span>
              )}
              {item.isCurrent && <Check aria-label="current" className="shrink-0 text-muted-foreground" />}
            </CommandItem>
          ))
        )}
      </CommandList>
    </Command>
  )
}
```

IMPORTANT — verify against the real component APIs before finalizing (this is integration work; adapt rather than fight):
- Open `web/src/components/ui/command.tsx` and confirm `Command`, `CommandInput` (accepts `value` + string `onChange`), `CommandList`, `CommandItem` (accepts `onClick`), `CommandEmpty` exports and prop shapes match this usage. `CommandInput` renders a real text input (the test uses `getByRole('textbox')`); if it isn't role=textbox, adjust the test selector to match what it renders (e.g. `getByPlaceholderText('Switch workspace…')`).
- Confirm `Check` is exported by `@phosphor-icons/react` (other icons in `workspace-branch-icon.tsx` come from there). If not, use an equivalent existing icon.
- Keep `select()` calling `navigate` then `onClose` exactly so the test passes.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/components/layout/workspace-switcher.test.tsx`
Expected: PASS (4 tests). If the input isn't `role="textbox"`, update the test's `getByRole('textbox')` to `getByPlaceholderText('Switch workspace…')` and re-run.

- [ ] **Step 5: Typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: no new errors (ignore the known pre-existing `semantic-tokens-provider.ts` error).

- [ ] **Step 6: Commit**

```bash
git add web/src/components/layout/workspace-switcher.tsx web/src/__tests__/components/layout/workspace-switcher.test.tsx
git commit -m "feat(sidebar): workspace switcher command menu"
```

---

### Task 4: Wire the switcher into the context pill (Popover)

**Files:**
- Modify: `web/src/components/layout/context-pill.tsx`
- Test: `web/src/__tests__/components/layout/context-pill.test.tsx` (update the click test)

**Context:** The pill button becomes the popover trigger. Remove the `setActiveTab('workspaces')` onClick (selection now happens inside the menu). The pill's content/markup/styling are unchanged.

- [ ] **Step 1: Update the component**

In `web/src/components/layout/context-pill.tsx`:

1. Add imports:

```tsx
import { useState } from 'react'
import { Popover, PopoverTrigger, PopoverPopup } from '@/components/ui/popover'
import { WorkspaceSwitcherMenu } from './workspace-switcher'
```

2. In `ContextPill`, add open state below the existing hooks:

```tsx
  const [open, setOpen] = useState(false)
```

3. Wrap the existing `<Button>...</Button>` in a `Popover`. The Button becomes the trigger; remove its `onClick` and change its `aria-label`. Replace the `return (...)` body's outer element:

```tsx
  return (
    <div className="shrink-0 px-2 pt-2 pb-1">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          render={
            <Button
              variant="ghost"
              aria-label="Switch workspace"
              className="h-auto w-full justify-start gap-2 rounded-lg bg-foreground/4 px-3 py-1.5 font-mono font-normal hover:bg-foreground/8 sm:h-auto"
            />
          }
        >
          {/* existing pill content (icon + text stack / project name) unchanged */}
        </PopoverTrigger>
        <PopoverPopup side="bottom" align="start" className="w-(--anchor-width) min-w-72 p-0">
          <WorkspaceSwitcherMenu onClose={() => setOpen(false)} />
        </PopoverPopup>
      </Popover>
    </div>
  )
```

Keep the existing inner content (the `model.kind === 'workspace' ? (...) : (...)` block) exactly as it is now — move it to be the `children` of `PopoverTrigger`.

IMPORTANT — verify against the real base-ui Popover API (read `web/src/components/ui/popover.tsx`, which you have, and base-ui usage):
- `Popover` (Root) controlled props: confirm it's `open` + `onOpenChange` (base-ui). Adapt if the prop names differ.
- `PopoverTrigger` composition: confirm how to render the trigger AS the existing `Button` (base-ui `render` prop vs `asChild`). The goal: the pill Button IS the trigger (one focusable button), with the pill content as its children. If `render` composition is awkward, an acceptable alternative is to put `<Button>` *inside* `PopoverTrigger` with `render`/`nativeButton` settings — whatever yields a single accessible button that opens the popover. Verify there is exactly one button and it opens the popover (live check in Task 5).
- Popover width: `--anchor-width` exposes the trigger width in base-ui positioner; if that var isn't available, use a sensible fixed width (e.g. `w-72`) — tokens/utilities only, no hardcoded px values outside Tailwind's scale.

- [ ] **Step 2: Update the existing click test**

The old context-pill test asserted clicking the pill calls `setActiveTab('workspaces')`. That behavior is intentionally removed. In `web/src/__tests__/components/layout/context-pill.test.tsx`, replace the test:

```tsx
  it('switches the sidebar to the workspaces tab on click', () => {
    mockPathname = '/workspaces/ws1'
    render(<ContextPill />)
    fireEvent.click(screen.getByRole('button'))
    expect(useSidebarStore.getState().activeTab).toBe('workspaces')
  })
```

with:

```tsx
  it('opens the workspace switcher on click', () => {
    mockPathname = '/workspaces/ws1'
    render(<ContextPill />)
    fireEvent.click(screen.getByRole('button', { name: 'Switch workspace' }))
    expect(screen.getByPlaceholderText('Switch workspace…')).toBeInTheDocument()
  })
```

Note: the popover renders in a portal; Testing Library's `screen` queries the whole document, so portalled content is found. If the popover content is lazily mounted and not present synchronously after click, wrap the assertion in `await screen.findByPlaceholderText('Switch workspace…')` and make the test `async`. Keep the other context-pill tests (workspace text, project name, empty) as-is — they assert rendered pill content, which is unchanged.

- [ ] **Step 3: Run the context-pill + switcher tests**

Run: `cd web && npx vitest run src/__tests__/components/layout/context-pill.test.tsx src/__tests__/components/layout/workspace-switcher.test.tsx`
Expected: all PASS. If portal/async issues arise, apply the `findBy*` adjustment noted above.

- [ ] **Step 4: Typecheck + full layout suite**

Run: `cd web && npx tsc --noEmit && npx vitest run src/__tests__/components/layout/`
Expected: tsc clean (ignore the known pre-existing `semantic-tokens-provider.ts` error); layout tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/context-pill.tsx web/src/__tests__/components/layout/context-pill.test.tsx
git commit -m "feat(sidebar): open workspace switcher from the context pill"
```

---

### Task 5: Live Tauri verification (REQUIRED — do not claim done without this)

Per the project rule (verify in the running Tauri app; tests/tsc ≠ visible result). Launch/observe the running app (Tauri MCP webview tools) and confirm:

- [ ] Clicking the context pill opens a popover anchored under it, search auto-focused.
- [ ] Typing filters the list across repo and branch; clearing restores all.
- [ ] Clicking a row navigates to that workspace (main content/route changes) and the popover closes.
- [ ] ↑/↓ highlights rows and Enter selects the highlighted one (keyboard nav). If Enter does not select with the current wiring, wire selection to the Command/Autocomplete keyboard behavior (base-ui forwards activation to the highlighted item) and re-verify — then update Task 3 if code changed.
- [ ] The **sidebar active tab does not change** when switching workspaces (open the switcher while on the Files or Git tab and confirm you stay there after selecting).
- [ ] Esc and outside-click close the popover without navigating.
- [ ] The current workspace row shows the ✓ marker; change counts render like the tree.
- [ ] Capture a screenshot of the open switcher.

- [ ] **If keyboard wiring changed**, commit:

```bash
git add -A && git commit -m "fix(sidebar): keyboard selection in workspace switcher"
```

---

## Self-Review

**Spec coverage:**
- Click pill → anchored Popover + coss Command menu → Task 4 + Task 3. ✓
- Flat list, all workspaces across repos → `flattenWorkspaces` (Task 2). ✓
- Search filters repo/branch via `matchesSearchQuery` → `filterWorkspaces` (Task 2). ✓
- Row = status icon + `repo / branch` + counts + current ✓ → Task 3 render. ✓
- Select → `navigate({ to: '/workspaces/$wsId' })`, close, sidebar tab untouched → Task 3 `select()` + Task 5 verification. ✓
- DRY count formatting shared with tree → `formatChangeCount` (Task 1). ✓
- Pill click no longer changes tab; `aria-label` updated → Task 4. ✓
- Tokens only, coss components reused → Tasks 3-4. ✓
- Tests mirror `src/` under `__tests__/`, `@/` imports → all test paths. ✓
- Live Tauri verification → Task 5. ✓

**Placeholder scan:** No TBD/"handle later"; pure-logic steps have complete code. The two integration points (base-ui Popover trigger composition; keyboard selection) are explicitly flagged with concrete intended code, a verification step, and a named fallback — not blanks.

**Type consistency:** `WorkspaceSwitcherItem` fields (`wsId`, `repoName`, `branch`, `status`, `added`, `deleted`, `isCurrent`) are identical across the model, its tests, and the component. `flattenWorkspaces`/`filterWorkspaces` signatures match between model, tests, and `WorkspaceSwitcherMenu`. Navigation call shape `{ to: '/workspaces/$wsId', params: { wsId } }` matches `workspace-tree.tsx` and the switcher test assertion. `formatChangeCount(n: number): string` is used identically in the tree and the switcher.

**Note:** Tasks 1-2 are pure and fully deterministic. Tasks 3-4 contain the only real integration risk (base-ui Popover/Command wiring + keyboard), which is why Task 5's live check is mandatory and the integration points carry explicit "verify & adapt" instructions.
