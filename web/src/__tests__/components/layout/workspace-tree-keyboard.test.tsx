/**
 * The keyboard, wired to the real tree.
 *
 * Two things are being pinned here that the pure model cannot reach. First,
 * that the keyboard walks the rows in the SAME order the shift-range and the
 * drag do — `visibleRowIds()` is the one function all three read, so the
 * assertion is literally that. Second, that walking them costs nothing: focus
 * and the single tab stop are element state, and an arrow key must not
 * re-render a row to move a focus ring.
 *
 * Renders are counted through the leading glyph, exactly as
 * workspace-tree-selection-renders.test.tsx does, and for the same reason —
 * React may replay render work, so a callback passed IN would be part of what
 * is replayed.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/react'

const renders = vi.hoisted(() => ({ count: 0 }))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useMatch: () => null,
}))

vi.mock('@/components/layout/workspace-branch-icon', () => ({
  WorkspaceBranchIcon: () => {
    renders.count += 1
    return null
  },
  WorkspaceAgentSpinner: () => null,
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  deleteWorkspace: vi.fn(() => Promise.resolve()),
  deleteRepo: vi.fn(() => Promise.resolve()),
}))

import { deleteWorkspace } from '@/lib/api'
import { idle, success } from '@/lib/loadable'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { getInitialSelectionState, useSidebarSelectionStore } from '@/lib/store/sidebar-selection'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { WorkspaceTree } from '@/components/layout/workspace-tree'
import { visibleRowIds, visibleRows } from '@/components/layout/row-selection'
import type { Project } from '@/lib/types'

const project: Project = {
  id: 'p1',
  name: 'crowbar-project',
  path: '/p1',
  lastActivity: new Date(0),
}

const repo = (): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'w-default',
  workspaces: [
    { id: 'a', branch: 'alpha', status: 'new', age: '', order: 0 },
    { id: 'kid', branch: 'apricot', parentId: 'a', status: 'new', age: '', order: 0 },
    { id: 'b', branch: 'beta', status: 'new', age: '', order: 1 },
    { id: 'c', branch: 'gamma', status: 'new', age: '', order: 2 },
  ],
  folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 3 }],
})

const rowEl = (id: string) =>
  document.querySelector<HTMLElement>(
    `[data-ws-drop="${id}"], [data-folder-drop="${id}"], [data-repo-drop="${id}"], [data-project-drop="${id}"]`,
  )!

const focusedId = () => {
  const el = document.activeElement
  return el instanceof HTMLElement
    ? (el.closest('[role="treeitem"]')?.getAttribute('data-row-label') ?? null)
    : null
}

/** The one tab stop, by label. */
const tabStops = () =>
  Array.from(document.querySelectorAll<HTMLElement>('[role="treeitem"]'))
    .filter((el) => el.tabIndex === 0)
    .map((el) => el.getAttribute('data-row-label'))

function press(id: string, key: string, init: Partial<KeyboardEventInit> = {}) {
  const el = rowEl(id)
  el.focus()
  fireEvent.keyDown(el, { key, ...init })
}

beforeEach(() => {
  vi.clearAllMocks()
  renders.count = 0
  useWorkspaceListStore.setState({ data: idle() })
  useHomeWorkspaceStore.setState({ workspace: null })
  useProjectDataStore.setState({ data: success([project]) })
  useSidebarSelectionStore.setState(getInitialSelectionState())
  useRemovalTrayStore.setState(getInitialRemovalState())
  useSidebarStore.setState({
    repos: [repo()],
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
})

describe('one tab stop for the whole tree', () => {
  it('leaves exactly one row tabbable', () => {
    render(<WorkspaceTree />)

    expect(tabStops()).toEqual(['crowbar-project'])
  })

  it('moves the stop with the focus rather than adding a second one', () => {
    render(<WorkspaceTree />)

    press('a', 'ArrowDown')

    expect(tabStops()).toEqual(['apricot'])
  })

  it('follows a row that took focus by any other route', () => {
    render(<WorkspaceTree />)

    fireEvent.focus(rowEl('b'), { target: rowEl('b') })

    expect(tabStops()).toEqual(['beta'])
  })
})

describe('the order the keyboard walks', () => {
  // The point of the assertion: one function answers for the keyboard, the
  // shift-range and the drag, so the three cannot drift apart.
  it('is the order rows are drawn, coarser rows included', () => {
    render(<WorkspaceTree />)

    expect(visibleRows().map((r) => r.id)).toEqual(['p1', 'r1', 'a', 'kid', 'b', 'c', 'f1'])
    expect(visibleRowIds()).toEqual(['a', 'kid', 'b', 'c', 'f1'])
  })

  it('steps down and up through it', () => {
    render(<WorkspaceTree />)

    press('a', 'ArrowDown')
    expect(focusedId()).toBe('apricot')

    press('kid', 'ArrowUp')
    expect(focusedId()).toBe('alpha')
  })

  it('jumps to the ends with Home and End', () => {
    render(<WorkspaceTree />)

    press('b', 'End')
    expect(focusedId()).toBe('spikes')

    press('f1', 'Home')
    expect(focusedId()).toBe('crowbar-project')
  })

  it('folds and unfolds with Left and Right', () => {
    render(<WorkspaceTree />)

    press('a', 'ArrowLeft')
    expect(useSidebarStore.getState().collapsedWorkspaces.has('a')).toBe(true)
    expect(document.querySelector('[data-ws-drop="kid"]')).toBeNull()

    press('a', 'ArrowRight')
    expect(useSidebarStore.getState().collapsedWorkspaces.has('a')).toBe(false)
  })

  it('jumps by label', () => {
    render(<WorkspaceTree />)

    press('a', 'g')

    expect(focusedId()).toBe('gamma')
  })
})

describe('Escape and Delete', () => {
  it('clears the multiselection on Escape, and leaves the rows alone', () => {
    render(<WorkspaceTree />)
    fireEvent.click(rowEl('b'), { metaKey: true })
    fireEvent.click(rowEl('c'), { metaKey: true })
    expect(useSidebarSelectionStore.getState().selected.size).toBe(2)

    press('c', 'Escape')

    expect(useSidebarSelectionStore.getState().selected.size).toBe(0)
    expect(visibleRowIds()).toEqual(['a', 'kid', 'b', 'c', 'f1'])
  })

  // Delete is the tray's gesture, not a delete. Nothing is sent until the
  // hairline drains, and the row can be kept the whole time.
  it('routes Delete through the tray rather than deleting', () => {
    render(<WorkspaceTree />)

    press('b', 'Delete')

    expect(deleteWorkspace).not.toHaveBeenCalled()
    expect(useRemovalTrayStore.getState().entries.map((e) => e.id)).toEqual(['b'])
    expect(document.querySelector('[data-ws-drop="b"]')).toBeNull()
    expect(useSidebarStore.getState().repos[0].workspaces.some((w) => w.id === 'b')).toBe(true)
  })

  it('takes the whole multiselection when the focused row is part of it', () => {
    render(<WorkspaceTree />)
    fireEvent.click(rowEl('b'), { metaKey: true })
    fireEvent.click(rowEl('c'), { metaKey: true })

    press('c', 'Delete')

    expect(
      useRemovalTrayStore
        .getState()
        .entries.map((e) => e.id)
        .sort(),
    ).toEqual(['b', 'c'])
  })
})

describe('what an interaction costs to paint', () => {
  it('costs NOTHING to walk the tree — focus is not React state', () => {
    render(<WorkspaceTree />)
    renders.count = 0

    press('a', 'ArrowDown')
    press('kid', 'ArrowDown')
    press('b', 'ArrowDown')
    press('c', 'End')
    press('f1', 'Home')

    expect(renders.count).toBe(0)
  })

  // The menu is a sibling of the tree, not a hook inside it. With its
  // open/closed state in the tree component this cost four rows — every one of
  // them — to draw a popup that is not part of the tree at all.
  it('costs NOTHING to open the context menu', () => {
    render(<WorkspaceTree />)
    renders.count = 0

    fireEvent.contextMenu(rowEl('b'))

    expect(document.querySelectorAll('[role="menuitem"]').length).toBeGreaterThan(0)
    expect(renders.count).toBe(0)
  })
})
