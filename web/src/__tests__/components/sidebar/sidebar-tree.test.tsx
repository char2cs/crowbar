import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'

vi.mock('@/lib/persistence/sidebar-ui', () => ({
  saveSidebarUI: vi.fn().mockResolvedValue(undefined),
  loadSidebarUI: vi.fn().mockResolvedValue(null),
}))

import { useSidebarStore } from '@/lib/store/sidebar'
import { SidebarTree } from '@/components/sidebar/sidebar-tree'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

// Task 21's drag wiring — a null scrollRef and no-op commit callbacks are
// enough for every test below, none of which exercises a live drag.
const DRAG_PROPS = {
  scrollRef: { current: null } as React.RefObject<HTMLElement | null>,
  onDrop: vi.fn(),
  onPaneDrop: vi.fn(),
}

const rows: SidebarRow[] = [
  {
    id: 'folder-1',
    kind: 'folder',
    parentId: null,
    order: 0,
    label: 'Bugs',
    ownsWorktree: false,
    workspaceId: null,
    working: false,
    hasView: false,
  },
  {
    id: 'chat-1',
    kind: 'chat',
    parentId: 'folder-1',
    order: 0,
    label: 'Fix the thing',
    ownsWorktree: false,
    workspaceId: null,
    working: false,
    hasView: false,
  },
]

beforeEach(() => {
  useSidebarStore.setState({ collapsedChatRows: new Set<string>() })
  resetWindowPaneStoreForTests()
})

describe('SidebarTree', () => {
  it('renders a row per entry, nested under its parent', () => {
    render(
      <SidebarTree
        rows={rows}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
    expect(screen.getByText('Bugs')).toBeInTheDocument()
    expect(screen.getByText('Fix the thing')).toBeInTheDocument()
  })

  it('folding a container hides its descendants', () => {
    render(
      <SidebarTree
        rows={rows}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
    // The brief's own step-1 test reaches for `getByTestId('fold-folder-1')`,
    // which does not exist: SidebarRow (Task 5, already committed) marks its
    // fold button with `data-control="fold"` and an aria-label, never a
    // per-row testid. Same intent, found for real: the fold button scoped to
    // folder-1's own treeitem.
    const folderRow = screen.getByText('Bugs').closest('[role="treeitem"]') as HTMLElement
    fireEvent.click(within(folderRow).getByRole('button', { name: /collapse bugs/i }))
    expect(screen.queryByText('Fix the thing')).not.toBeInTheDocument()
  })

  it('indents each level by ROW_INDENT_STEP (14px, from workspace-row-base.ts)', () => {
    render(
      <SidebarTree
        rows={rows}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
    const root = screen.getByText('Bugs').closest('[role="treeitem"]')?.parentElement
    const child = screen.getByText('Fix the thing').closest('[role="treeitem"]')?.parentElement
    expect(root?.getAttribute('style')).toContain('margin-inline-start: 0px')
    expect(child?.getAttribute('style')).toContain('margin-inline-start: 14px')
  })

  it('an empty container renders the affordance row instead of nothing', () => {
    const onCreate = vi.fn()
    // folder-1 with no children at all.
    render(
      <SidebarTree
        rows={[rows[0]]}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={onCreate}
        {...DRAG_PROPS}
      />,
    )
    // No dropdown: folder-1 does not own a worktree, so only a thread is legal.
    expect(screen.queryByTestId('affordance-dropdown')).not.toBeInTheDocument()
    screen.getByRole('button', { name: /create new thread/i }).click()
    expect(onCreate).toHaveBeenCalledWith('folder-1', 'thread')
  })

  it('an empty container that owns a worktree offers the split affordance', () => {
    const branch: SidebarRow = {
      id: 'branch-1',
      kind: 'branch',
      parentId: null,
      order: 0,
      label: 'main',
      ownsWorktree: true,
      workspaceId: 'ws-1',
      working: false,
      hasView: false,
    }
    render(
      <SidebarTree
        rows={[branch]}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
    expect(screen.getByTestId('affordance-dropdown')).toBeInTheDocument()
  })

  it('renders siblings with no rule between them', () => {
    const siblings: SidebarRow[] = [
      { ...rows[0], id: 'folder-a', label: 'Folder A' },
      { ...rows[0], id: 'folder-b', label: 'Folder B' },
    ]
    const { container } = render(
      <SidebarTree
        rows={siblings}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
    expect(container.querySelectorAll('hr')).toHaveLength(0)
    // Token-boundary check: `border-transparent` (ROW_INACTIVE's resting
    // border) must not false-positive as a `border-t*`/`border-b*` divider
    // utility.
    const hasDividerUtility = Array.from(container.querySelectorAll('[class]')).some((el) =>
      (el.getAttribute('class') ?? '').split(/\s+/).some((cls) => /^border-[tb](-|$)/.test(cls)),
    )
    expect(hasDividerUtility).toBe(false)
  })

  it('greys a chat row whose chat is live open in a pane, even though the row prop itself always arrives hasView: false', () => {
    // Every row in `rows` is seeded with `hasView: false` (rows-from-repo.ts
    // never seeds live state into the row object — see its own note). The
    // grey has to come from a LIVE subscription to pane membership, not from
    // the prop, so seed a pane holding chat-1's id directly on the window
    // pane store rather than passing hasView: true into `rows`.
    windowPaneStore.setState((s) => {
      s.panes[ROOT_PANE_ID] = { ...s.panes[ROOT_PANE_ID], chatId: 'chat-1' }
      return s
    })

    render(
      <SidebarTree
        rows={rows}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )

    expect(screen.getByText('Fix the thing').className).toContain('text-muted-foreground')
    // The folder row's chat never opened anywhere — no false-positive grey.
    expect(screen.getByText('Bugs').className).not.toContain('text-muted-foreground')
  })

  it('does not grey a chat row whose chat is not open in any pane', () => {
    render(
      <SidebarTree
        rows={rows}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )

    expect(screen.getByText('Fix the thing').className).not.toContain('text-muted-foreground')
  })
})
