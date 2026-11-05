// `SidebarTreeChrome` mounts `RepoImportDialog` and `SidebarRowContextMenu` ONCE at the shell
// level, not per `SpaceScroller` panel. New Project, RemovalTray and rename are not its concern:
// they live in the shell, the shell, and the inline editor.
import { createRef } from 'react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useRouterState: () => '/',
}))

import { SidebarTreeChrome } from '@/components/layout/sidebar-tree-chrome'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { resetEntities } from '@/__tests__/__fixtures__/sidebar-entities'

beforeEach(() => {
  vi.clearAllMocks()
  resetEntities()
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  treeUiStore.setState(treeUiStore.getInitialState(), true)
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

// Double-click only starts the inline-rename store; SidebarRow, several components away, draws
// the input (sidebar-row.test.tsx's "inline rename" suite).
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
    expect(treeUiStore.getState().renaming).toBe('chat-1')
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
    expect(treeUiStore.getState().renaming).toBeNull()
  })

  it('double-clicking a label for an id not in `rows` is a no-op', () => {
    const treeRef = renderChromeWithRows()
    const label = makeRowLabel(treeRef.current, 'nonexistent')
    fireEvent.doubleClick(label)
    expect(treeUiStore.getState().renaming).toBeNull()
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
    expect(treeUiStore.getState().renaming).toBeNull()
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
    expect(treeUiStore.getState().renaming).toBe('chat-1')
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
