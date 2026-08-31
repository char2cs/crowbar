/**
 * `SidebarTreeChrome` hoists `RemovalTray`/`RenameDialog`/`RepoImportDialog`/
 * `SidebarRowContextMenu` so they mount ONCE at the ide-shell level rather
 * than once per `SpaceScroller` project panel. "New Project" used to live
 * here too, as a tree-foot row; Task 5 relocated it to a trailing `+` mark
 * in `SidebarProjectHeader`'s window-chrome row (spec §4.1) and lifted its
 * modal state up to `IDEShell` — this component no longer owns any of that.
 */
import { createRef } from 'react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  // RemovalTray needs both.
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useRouterState: () => '/',
}))

import { SidebarTreeChrome } from '@/components/layout/sidebar-tree-chrome'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import * as rowActions from '@/components/sidebar/lib/row-actions'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

vi.mock('@/components/sidebar/lib/row-actions', async (importOriginal) => ({
  ...(await importOriginal<typeof rowActions>()),
  performRenameRow: vi.fn().mockResolvedValue(undefined),
}))

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState(getInitialState())
  useRemovalTrayStore.setState(getInitialRemovalState())
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

  it('double-clicking a row label opens the rename dialog prefilled with its label', () => {
    const treeRef = renderChromeWithRows()
    const label = makeRowLabel(treeRef.current, 'chat-1')
    fireEvent.doubleClick(label)
    expect(screen.getByRole('textbox')).toHaveValue('Fix the thing')
  })

  it('confirming the dialog after a double-click calls performRenameRow with the row id', () => {
    const treeRef = renderChromeWithRows()
    const label = makeRowLabel(treeRef.current, 'chat-1')
    fireEvent.doubleClick(label)
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'New title' } })
    fireEvent.click(screen.getByRole('button', { name: /rename/i }))
    expect(rowActions.performRenameRow).toHaveBeenCalledWith('chat-1', 'New title')
  })

  it('double-clicking a trailing control (not the label) does not open the rename dialog', () => {
    const treeRef = renderChromeWithRows()
    const item = document.createElement('div')
    item.setAttribute('role', 'treeitem')
    item.setAttribute('data-sidebar-row-id', 'chat-1')
    const trash = document.createElement('button')
    trash.setAttribute('data-control', 'trash')
    item.appendChild(trash)
    treeRef.current.appendChild(item)
    fireEvent.doubleClick(trash)
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  })

  it('double-clicking a label for an id not in `rows` is a no-op', () => {
    const treeRef = renderChromeWithRows()
    const label = makeRowLabel(treeRef.current, 'nonexistent')
    fireEvent.doubleClick(label)
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  })
})
