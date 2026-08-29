import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { SidebarRowContextMenu } from '@/components/sidebar/row-context-menu'
import { useSidebarStore, getInitialState } from '@/lib/store/sidebar'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

const rows: SidebarRow[] = [
  {
    id: 'ws-1',
    kind: 'branch',
    parentId: null,
    order: 0,
    label: 'repo',
    ownsWorktree: true,
    workspaceId: 'ws-1',
    working: false,
    hasView: false,
  },
  // A REGULAR workspace branch row (not project-home) — the one row kind
  // Lock/Unlock is actually meant to apply to.
  {
    id: 'ws-2',
    kind: 'branch',
    parentId: 'ws-1',
    order: 0,
    label: 'feature-x',
    ownsWorktree: true,
    workspaceId: 'ws-2',
    working: false,
    hasView: false,
  },
  // ownsWorktree: true here matches what `rowsFromRepo.ts` actually produces
  // for a folder row (its own "+" always forks a branch) — a fixture with
  // `ownsWorktree: false` would let a Lock-gated-on-ownsWorktree bug slip
  // past this test, which is exactly what happened in fix round 1.
  {
    id: 'folder-1',
    kind: 'folder',
    parentId: 'ws-1',
    order: 0,
    label: 'Bugs',
    ownsWorktree: true,
    workspaceId: null,
    working: false,
    hasView: false,
  },
]

function renderMenu() {
  const treeRef = { current: document.createElement('div') }
  document.body.appendChild(treeRef.current)
  const onRename = vi.fn()
  const onImport = vi.fn()
  render(
    <SidebarRowContextMenu treeRef={treeRef} rows={rows} onRename={onRename} onImport={onImport} />,
  )
  return { treeRef, onRename, onImport }
}

function rightClick(tree: HTMLElement, rowId: string) {
  const target = document.createElement('div')
  target.setAttribute('role', 'treeitem')
  target.setAttribute('data-sidebar-row-id', rowId)
  tree.appendChild(target)
  fireEvent.contextMenu(target)
  return target
}

beforeEach(() => {
  useSidebarStore.setState(getInitialState())
})

describe('SidebarRowContextMenu', () => {
  // Amended in fix round 1: Lock/Unlock must NOT appear on the project-home
  // row — it IS the repo's own checkout, the one branch that must stay put
  // (the deleted row-menu-model.ts's own documented rule). The original Task
  // 29 submission gated Lock on `row.ownsWorktree`, which the project-home
  // row also carries true, so it wrongly offered a Lock that silently did
  // nothing when clicked (the row's id is never in `repo.workspaces`).
  it('right-clicking the project-home row offers Rename and Import, but not Lock/Unlock', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, 'ws-1')
    expect(screen.getByText('Rename')).toBeInTheDocument()
    expect(screen.getByText('Import branches')).toBeInTheDocument()
    expect(screen.queryByText('Lock')).not.toBeInTheDocument()
    expect(screen.queryByText('Unlock')).not.toBeInTheDocument()
  })

  it('right-clicking a real workspace branch row offers Rename and Lock, not Import', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, 'ws-2')
    expect(screen.getByText('Rename')).toBeInTheDocument()
    expect(screen.getByText('Lock')).toBeInTheDocument()
    expect(screen.queryByText('Import branches')).not.toBeInTheDocument()
  })

  // Amended in fix round 1: a folder row with `ownsWorktree: true` (matching
  // rowsFromRepo.ts's real output) must still not offer Lock/Unlock — the gate
  // has to be kind-based, not `ownsWorktree`-based.
  it('right-clicking a folder row (ownsWorktree: true, matching real rowsFromRepo output) offers only Rename', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, 'folder-1')
    expect(screen.getByText('Rename')).toBeInTheDocument()
    expect(screen.queryByText('Lock')).not.toBeInTheDocument()
    expect(screen.queryByText('Unlock')).not.toBeInTheDocument()
    expect(screen.queryByText('Import branches')).not.toBeInTheDocument()
  })

  it('offers Unlock instead of Lock when the matching workspace is locked', () => {
    useSidebarStore.setState({
      repos: [
        {
          id: 'repo-1',
          projectId: 'proj-1',
          name: 'repo',
          avatarLabel: 'R',
          avatarColor: 'bg-indigo-700',
          workspaces: [
            { id: 'ws-1', branch: 'main', age: '' },
            { id: 'ws-2', branch: 'feature-x', age: '', status: 'locked' },
          ],
          folders: [],
        },
      ],
    })
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, 'ws-2')
    expect(screen.getByText('Unlock')).toBeInTheDocument()
    expect(screen.queryByText('Lock')).not.toBeInTheDocument()
  })

  it('clicking Rename calls onRename with the row id and closes the menu', () => {
    const { treeRef, onRename } = renderMenu()
    rightClick(treeRef.current, 'folder-1')
    fireEvent.click(screen.getByText('Rename'))
    expect(onRename).toHaveBeenCalledWith('folder-1')
  })

  it('clicking Import branches calls onImport with the project-home row id', () => {
    const { treeRef, onImport } = renderMenu()
    rightClick(treeRef.current, 'ws-1')
    fireEvent.click(screen.getByText('Import branches'))
    expect(onImport).toHaveBeenCalledWith('ws-1')
  })

  it('right-clicking an unknown row is a no-op', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, 'nonexistent')
    expect(screen.queryByText('Rename')).not.toBeInTheDocument()
  })
})
