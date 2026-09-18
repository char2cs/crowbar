/**
 * `SidebarTreeChrome` hoists `RepoImportDialog`/`SidebarRowContextMenu` so
 * they mount ONCE at the ide-shell level rather than once per
 * `SpaceScroller` project panel. "New Project" used to live here too, as a
 * tree-foot row; Task 5 relocated it to a trailing `+` mark in
 * `SidebarProjectHeader`'s window-chrome row (spec §4.1) and lifted its
 * modal state up to `IDEShell` — this component no longer owns any of that.
 * `RemovalTray` doesn't either any more: addendum §2 step 4 moved it into
 * `SidebarCarousel`, at the top of the file explorer card. Neither does
 * `RenameDialog`: the right-click menu's Rename item now starts the same
 * inline editor double-click does, so there is no modal left to hoist.
 */
import { createRef } from 'react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useRouterState: () => '/',
}))

import { SidebarTreeChrome } from '@/components/layout/sidebar-tree-chrome'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import {
  getInitialInlineRenameState,
  useSidebarInlineRenameStore,
} from '@/lib/store/sidebar-inline-rename'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState(getInitialState())
  useRemovalTrayStore.setState(getInitialRemovalState())
  useSidebarInlineRenameStore.setState(getInitialInlineRenameState())
})

function renderChrome() {
  const treeRef = createRef<HTMLDivElement>()
  return render(<SidebarTreeChrome treeRef={treeRef} rows={[]} repos={[]} />)
}

describe('the leftover "New Project" row is gone', () => {
  it('no longer renders a "New Project" button', () => {
    renderChrome()
    expect(screen.queryByText('New Project')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /new project/i })).not.toBeInTheDocument()
  })

  it('does not mount ImportProjectModal itself', () => {
    renderChrome()
    expect(screen.queryByText('Import project')).not.toBeInTheDocument()
  })
})

// Task 11: double-click now enters INLINE rename mode (a real input drawn by
// SidebarRow itself, several components away) rather than opening the modal
// RenameDialog Task 4 wrongly wired here — SidebarTreeChrome doesn't render
// any row markup, so it can't draw the input; it only has to start the
// inline-rename store, which the row this DOM belongs to reads from
// independently. See sidebar-row.test.tsx's own "inline rename" suite for
// the input itself.
describe('double-click-to-rename', () => {
  const rows: SidebarRow[] = [
    {
      id: 'chat-1',
      kind: 'chat',
      parentId: null,
      order: 0,
      label: 'Fix the thing',
      ownsWorktree: false,
      workspaceId: null,
      working: false,
      hasView: false,
    },
  ]

  function renderChromeWithRows() {
    const treeRef = { current: document.createElement('div') }
    document.body.appendChild(treeRef.current)
    render(<SidebarTreeChrome treeRef={treeRef} rows={rows} repos={[]} />)
    return treeRef
  }

  /** Mirrors row-context-menu.test.tsx's own `rightClick` helper: a bare DOM
   *  treeitem, not a rendered `SidebarRow`, since the wiring under test is
   *  the delegated listener, not SidebarRow's own markup. */
  function makeRowLabel(tree: HTMLElement, rowId: string) {
    const item = document.createElement('div')
    item.setAttribute('role', 'treeitem')
    item.setAttribute('data-sidebar-row-id', rowId)
    const label = document.createElement('span')
    label.setAttribute('data-sidebar-row-label', '')
    item.appendChild(label)
    tree.appendChild(item)
    return label
  }

  it('double-clicking a row label starts inline rename for that row id in the store', () => {
    const treeRef = renderChromeWithRows()
    const label = makeRowLabel(treeRef.current, 'chat-1')
    fireEvent.doubleClick(label)
    expect(useSidebarInlineRenameStore.getState().renamingRowId).toBe('chat-1')
  })

  it('double-clicking does NOT open a rename dialog', () => {
    const treeRef = renderChromeWithRows()
    const label = makeRowLabel(treeRef.current, 'chat-1')
    fireEvent.doubleClick(label)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  })

  it('double-clicking a trailing control (not the label) does not start inline rename', () => {
    const treeRef = renderChromeWithRows()
    const item = document.createElement('div')
    item.setAttribute('role', 'treeitem')
    item.setAttribute('data-sidebar-row-id', 'chat-1')
    const trash = document.createElement('button')
    trash.setAttribute('data-control', 'trash')
    item.appendChild(trash)
    treeRef.current.appendChild(item)
    fireEvent.doubleClick(trash)
    expect(useSidebarInlineRenameStore.getState().renamingRowId).toBeNull()
  })

  it('double-clicking a label for an id not in `rows` is a no-op', () => {
    const treeRef = renderChromeWithRows()
    const label = makeRowLabel(treeRef.current, 'nonexistent')
    fireEvent.doubleClick(label)
    expect(useSidebarInlineRenameStore.getState().renamingRowId).toBeNull()
  })

  // performRenameWorkspaceBranch (row-actions.ts) already refuses a locked
  // workspace's rename write silently — starting the inline editor here just
  // meant typing a name into a box that discarded it.
  it('double-clicking a LOCKED branch row label does not start inline rename', () => {
    const lockedRows: SidebarRow[] = [
      {
        id: 'branch-1',
        kind: 'branch',
        parentId: null,
        order: 0,
        label: 'main',
        ownsWorktree: true,
        workspaceId: 'ws-1',
        working: false,
        hasView: false,
        locked: true,
      },
    ]
    const treeRef = { current: document.createElement('div') }
    document.body.appendChild(treeRef.current)
    render(<SidebarTreeChrome treeRef={treeRef} rows={lockedRows} repos={[]} />)
    const label = makeRowLabel(treeRef.current, 'branch-1')
    fireEvent.doubleClick(label)
    expect(useSidebarInlineRenameStore.getState().renamingRowId).toBeNull()
  })
})

// The right-click menu's Rename item starts the SAME inline editor
// double-click does — this app has exactly one rename gesture, never a
// modal (row-context-menu.tsx).
describe('right-click Rename starts the same inline rename as double-click', () => {
  const rows: SidebarRow[] = [
    {
      id: 'chat-1',
      kind: 'chat',
      parentId: null,
      order: 0,
      label: 'Fix the thing',
      ownsWorktree: false,
      workspaceId: null,
      working: false,
      hasView: false,
    },
  ]

  function rightClick(tree: HTMLElement, rowId: string) {
    const item = document.createElement('div')
    item.setAttribute('role', 'treeitem')
    item.setAttribute('data-sidebar-row-id', rowId)
    tree.appendChild(item)
    fireEvent.contextMenu(item)
  }

  it('right-click Rename starts inline rename for that row id in the store', () => {
    const treeRef = { current: document.createElement('div') }
    document.body.appendChild(treeRef.current)
    render(<SidebarTreeChrome treeRef={treeRef} rows={rows} repos={[]} />)
    rightClick(treeRef.current, 'chat-1')
    fireEvent.click(screen.getByText('Rename'))
    expect(useSidebarInlineRenameStore.getState().renamingRowId).toBe('chat-1')
  })

  it('right-click Rename does not open a dialog', () => {
    const treeRef = { current: document.createElement('div') }
    document.body.appendChild(treeRef.current)
    render(<SidebarTreeChrome treeRef={treeRef} rows={rows} repos={[]} />)
    rightClick(treeRef.current, 'chat-1')
    fireEvent.click(screen.getByText('Rename'))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  })
})
