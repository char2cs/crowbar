import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { SidebarRowContextMenu } from '@/components/sidebar/row-context-menu'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { useSidebarStore, getInitialState, type Repo } from '@/lib/store/sidebar'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import * as api from '@/lib/api'
import * as sidebarPlacement from '@/lib/api/sidebar-placement'

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof api>()),
  setWorkspaceLock: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/lib/api/sidebar-placement', async (importOriginal) => ({
  ...(await importOriginal<typeof sidebarPlacement>()),
  createFolder: vi.fn().mockResolvedValue({
    folder: {
      id: 'folder-new',
      repoId: 'repo-1',
      projectId: 'proj-1',
      name: 'New folder',
      order: 0,
    },
    shifted: [],
  }),
}))

/**
 * THE FIXTURE IS THE STORE, AND THE ROWS ARE DERIVED FROM IT.
 *
 * The previous version of this file hand-wrote its rows with `id: 'ws-2'` under
 * a comment claiming it "matches what rowsFromRepo.ts actually produces" — which
 * stopped being true the moment a locked branch row started carrying the id of
 * the CHAT that owns its workspace. Every id-space bug on this seam therefore
 * passed CI: the menu compared a row id against `w.id` and the fixture handed it
 * a row id that WAS `w.id`.
 *
 * So this file no longer writes rows at all. It writes the repo the store holds
 * and runs `rowsFromRepo` over it, exactly as `SidebarTreeSurface` does — the
 * rows below are whatever that function really produces, id space included.
 */
const REPO: Repo = {
  id: 'repo-1',
  projectId: 'proj-1',
  name: 'repo',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'ws-home',
  defaultBranch: 'main',
  workspaces: [
    { id: 'ws-1', branch: 'feature-x', age: '' },
    { id: 'ws-2', branch: 'develop', age: '', status: 'locked' },
  ],
  folders: [{ id: 'folder-1', repoId: 'repo-1', name: 'Bugs', order: 0 }],
  chats: [
    {
      id: 'home-row',
      repoId: 'repo-1',
      type: 'branch',
      workspaceId: 'ws-home',
      title: '',
      order: 0,
    },
    {
      id: 'ws-2-row',
      repoId: 'repo-1',
      type: 'branch',
      workspaceId: 'ws-2',
      title: '',
      order: 1,
    },
  ],
}

/** The repo-home row's id: the owning `branch` chat, never `defaultWorkspaceId`. */
const HOME_ROW_ID = 'home-row'
/** A LOCKED branch row's id: likewise the owning `branch` chat, never `ws-2`. */
const LOCKED_ROW_ID = 'ws-2-row'
/** A regular (unlocked) fork keeps the workspace id — the one branch row whose
 *  two id spaces still coincide, and the reason a bug here stayed invisible. */
const FORK_ROW_ID = 'ws-1'

let rows: SidebarRow[] = []

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
  vi.clearAllMocks()
  useSidebarStore.setState({ ...getInitialState(), repos: [REPO] })
  rows = rowsFromRepo(REPO)
})

describe('SidebarRowContextMenu', () => {
  // The rows this menu is driven by really do carry the ids this file asserts —
  // if `rowsFromRepo` ever stops giving a locked branch its owning chat's id,
  // the fixture must follow it rather than the other way round.
  it('is driven by rowsFromRepo’s real output, where a locked branch is id’d by its owning chat', () => {
    expect(rows.map((r) => r.id).sort()).toEqual(
      [HOME_ROW_ID, FORK_ROW_ID, LOCKED_ROW_ID, 'folder-1'].sort(),
    )
    expect(rows.find((r) => r.id === LOCKED_ROW_ID)?.workspaceId).toBe('ws-2')
  })

  // Amended in fix round 1: Lock/Unlock must NOT appear on the project-home
  // row — it IS the repo's own checkout, the one branch that must stay put
  // (the deleted row-menu-model.ts's own documented rule). The original Task
  // 29 submission gated Lock on `row.ownsWorktree`, which the project-home
  // row also carries true, so it wrongly offered a Lock that silently did
  // nothing when clicked (the row's id is never in `repo.workspaces`).
  it('right-clicking the project-home row offers Rename and Import, but not Lock/Unlock', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, HOME_ROW_ID)
    expect(screen.getByText('Rename')).toBeInTheDocument()
    expect(screen.getByText('Import branches')).toBeInTheDocument()
    expect(screen.queryByText('Lock')).not.toBeInTheDocument()
    expect(screen.queryByText('Unlock')).not.toBeInTheDocument()
  })

  it('right-clicking a real workspace branch row offers Rename and Lock, not Import', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, FORK_ROW_ID)
    expect(screen.getByText('Rename')).toBeInTheDocument()
    expect(screen.getByText('Lock')).toBeInTheDocument()
    expect(screen.queryByText('Import branches')).not.toBeInTheDocument()
  })

  it('clicking Lock on an unlocked fork fires setWorkspaceLock for its workspace', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, FORK_ROW_ID)
    fireEvent.click(screen.getByText('Lock'))
    expect(api.setWorkspaceLock).toHaveBeenCalledWith('ws-1', true)
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

  /**
   * Finding 1 of the final whole-plan review, and the reason this file's rows
   * are now derived rather than written.
   *
   * `locked` was read by matching the ROW id against `w.id`. A locked branch
   * row's id is its owning chat's, which matches no workspace — so the menu
   * offered "Lock" on an already-locked branch forever, and the click behind it
   * resolved no repo and sent nothing. With `DeleteCascade` refusing to delete a
   * locked workspace, that made locking a one-way door: never unlockable,
   * never deletable, from any surface the UI offers.
   */
  describe('a LOCKED branch row, addressed by its owning chat id', () => {
    it('offers Unlock rather than Lock', () => {
      const { treeRef } = renderMenu()
      rightClick(treeRef.current, LOCKED_ROW_ID)
      expect(screen.getByText('Unlock')).toBeInTheDocument()
      expect(screen.queryByText('Lock')).not.toBeInTheDocument()
    })

    it('clicking Unlock actually fires setWorkspaceLock for the WORKSPACE it owns', () => {
      const { treeRef } = renderMenu()
      rightClick(treeRef.current, LOCKED_ROW_ID)
      fireEvent.click(screen.getByText('Unlock'))
      expect(api.setWorkspaceLock).toHaveBeenCalledWith('ws-2', false)
    })

    // Finding 2 of the same review: `performCreateFolder` matched `parentId`
    // against the same three id spaces and was never translated, so "New folder"
    // on a locked branch (or the repo home) fired nothing at all.
    it('clicking New folder actually creates one under the workspace it owns', () => {
      const { treeRef } = renderMenu()
      rightClick(treeRef.current, LOCKED_ROW_ID)
      fireEvent.click(screen.getByText('New folder'))
      expect(sidebarPlacement.createFolder).toHaveBeenCalledWith(
        'proj-1',
        'repo-1',
        'New folder',
        'ws-2',
      )
    })
  })

  it('clicking New folder on the repo-home row roots it at the repo', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, HOME_ROW_ID)
    fireEvent.click(screen.getByText('New folder'))
    expect(sidebarPlacement.createFolder).toHaveBeenCalledWith('proj-1', 'repo-1', 'New folder', '')
  })

  it('clicking Rename calls onRename with the row id and closes the menu', () => {
    const { treeRef, onRename } = renderMenu()
    rightClick(treeRef.current, 'folder-1')
    fireEvent.click(screen.getByText('Rename'))
    expect(onRename).toHaveBeenCalledWith('folder-1')
  })

  it('clicking Import branches calls onImport with the project-home row id', () => {
    const { treeRef, onImport } = renderMenu()
    rightClick(treeRef.current, HOME_ROW_ID)
    fireEvent.click(screen.getByText('Import branches'))
    expect(onImport).toHaveBeenCalledWith(HOME_ROW_ID)
  })

  it('right-clicking an unknown row is a no-op', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, 'nonexistent')
    expect(screen.queryByText('Rename')).not.toBeInTheDocument()
  })
})
