/**
 * The two-state keep model, through the real tree.
 *
 * `keep-set.ts` pins the rules and `sidebar-selection.ts` pins the storage; this
 * pins what a user actually does — cmd-click two rows, fold their parent, go and
 * open something else, and find both rows still there.
 *
 * The failure being guarded against is the one that looks like a styling bug:
 * treat the multiselection and the keep set as one thing, and either a plain
 * click wipes the rows you deliberately kept, or the kept rows keep the
 * selection treatment and you can no longer tell which workspace you are in.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'

const router = vi.hoisted(() => ({ pathname: '' }))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => router.pathname,
  useRouter: () => ({ state: { location: { pathname: router.pathname } } }),
  useMatch: () => null,
}))

vi.mock('@/lib/api/sidebar-placement', () => ({
  placeWorkspace: vi.fn(() => Promise.resolve()),
  placeFolder: vi.fn(() => Promise.resolve()),
  placeRepo: vi.fn(() => Promise.resolve()),
  placeProject: vi.fn(() => Promise.resolve()),
  createFolder: vi.fn(() => Promise.resolve()),
  deleteFolder: vi.fn(() => Promise.resolve()),
}))

import { idle, success } from '@/lib/loadable'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { getInitialSelectionState, useSidebarSelectionStore } from '@/lib/store/sidebar-selection'
import { ROW_ACTIVE } from '@/components/layout/workspace-row-base'
import { WorkspaceTree } from '@/components/layout/workspace-tree'
import type { Project } from '@/lib/types'

//  develop          (locked root)
//    └─ Reviewing   (folder)
//         ├─ plate
//         └─ sidebar
//              └─ folders
const repo = (): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'w-default',
  folders: [{ id: 'f-rev', repoId: 'r1', parentId: 'w-develop', name: 'Reviewing', order: 0 }],
  workspaces: [
    { id: 'w-develop', branch: 'develop', status: 'locked', age: '1d', order: 0 },
    {
      id: 'w-plate',
      branch: 'feat/plate',
      parentId: 'w-develop',
      folderId: 'f-rev',
      status: 'new',
      age: '1d',
      order: 0,
    },
    {
      id: 'w-sidebar',
      branch: 'feat/sidebar',
      parentId: 'w-develop',
      folderId: 'f-rev',
      status: 'new',
      age: '1d',
      order: 1,
    },
    { id: 'w-folders', branch: 'feat/folders', parentId: 'w-sidebar', status: 'new', age: '1d' },
  ],
})

const project: Project = {
  id: 'p1',
  name: 'crowbar-project',
  path: '/p1',
  lastActivity: new Date(0),
}

const rowFor = (branchOrName: string) =>
  screen.getAllByRole('treeitem').find((r) => (r.textContent ?? '').includes(branchOrName))!

/** The row element itself, addressed the way the drag scan addresses it. */
const wsRow = (id: string) => document.querySelector<HTMLElement>(`[data-ws-drop="${id}"]`)
const folderRow = (id: string) => document.querySelector<HTMLElement>(`[data-folder-drop="${id}"]`)

const cmdClick = (el: HTMLElement) => fireEvent.click(el, { metaKey: true })

/** Fold `Reviewing` from its own chevron, which is what a user reaches for. */
function collapseReviewing() {
  fireEvent.click(within(folderRow('f-rev')!).getByLabelText('Collapse'))
}

beforeEach(() => {
  vi.clearAllMocks()
  router.pathname = ''
  useWorkspaceListStore.setState({ data: idle() })
  useHomeWorkspaceStore.setState({ workspace: null })
  useProjectDataStore.setState({ data: success([project]) })
  useSidebarSelectionStore.setState(getInitialSelectionState())
  useSidebarStore.setState({
    repos: [repo()],
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
})

describe('the multiselection is transient, and drawn as the open row', () => {
  it('cmd-click lights a row with the SAME treatment the open workspace wears', () => {
    router.pathname = '/ide/p1/r1/w-plate'
    render(<WorkspaceTree />)

    cmdClick(wsRow('w-sidebar')!)

    const active = wsRow('w-plate')!.className.split(/\s+/)
    const selected = wsRow('w-sidebar')!.className.split(/\s+/)
    for (const shared of ROW_ACTIVE.trim().split(/\s+/)) {
      expect(selected, `selected row lost ${shared}`).toContain(shared)
      expect(active, `open row lost ${shared}`).toContain(shared)
    }
  })

  it('publishes it on aria-selected, which is where the drag reads it', () => {
    render(<WorkspaceTree />)

    cmdClick(wsRow('w-plate')!)

    expect(wsRow('w-plate')).toHaveAttribute('aria-selected', 'true')
    expect(wsRow('w-sidebar')).toHaveAttribute('aria-selected', 'false')
  })

  it('a plain click clears it', () => {
    render(<WorkspaceTree />)

    cmdClick(wsRow('w-plate')!)
    cmdClick(wsRow('w-sidebar')!)
    expect(useSidebarSelectionStore.getState().selected.size).toBe(2)

    fireEvent.click(wsRow('w-folders')!)

    expect(useSidebarSelectionStore.getState().selected.size).toBe(0)
    expect(wsRow('w-plate')).toHaveAttribute('aria-selected', 'false')
  })

  it('is cleared by a plain click on a repo or project row too', () => {
    render(<WorkspaceTree />)

    cmdClick(wsRow('w-plate')!)
    fireEvent.click(screen.getByLabelText('Open crowbar'))
    expect(useSidebarSelectionStore.getState().selected.size).toBe(0)

    cmdClick(wsRow('w-plate')!)
    fireEvent.click(rowFor('crowbar-project'))
    expect(useSidebarSelectionStore.getState().selected.size).toBe(0)
  })

  it('cmd-click does not open the workspace it lands on', () => {
    render(<WorkspaceTree />)
    cmdClick(wsRow('w-plate')!)
    // Nothing navigated: the route is still where it started.
    expect(router.pathname).toBe('')
  })

  it('shift-click takes the range the user can SEE, folder rows included', () => {
    render(<WorkspaceTree />)

    cmdClick(wsRow('w-plate')!)
    fireEvent.click(wsRow('w-folders')!, { shiftKey: true })

    // develop → Reviewing → plate → sidebar → folders, so the range from
    // `plate` reaches `folders` through `sidebar` and stops there.
    expect([...useSidebarSelectionStore.getState().selected].sort()).toEqual([
      'w-folders',
      'w-plate',
      'w-sidebar',
    ])
  })
})

describe('the keep set is a snapshot, and it survives', () => {
  it('keeps the multiselected rows on screen when their parent folds', () => {
    render(<WorkspaceTree />)

    cmdClick(wsRow('w-plate')!)
    collapseReviewing()

    // The unselected sibling went with the fold; the kept one did not.
    expect(wsRow('w-plate')).not.toBeNull()
    expect(wsRow('w-sidebar')).toBeNull()
  })

  it('keeps the OPEN workspace even when nothing is selected', () => {
    router.pathname = '/ide/p1/r1/w-plate'
    render(<WorkspaceTree />)

    collapseReviewing()

    expect(wsRow('w-plate')).not.toBeNull()
  })

  it('draws a kept row one indent step under the parent holding it', () => {
    render(<WorkspaceTree />)
    // `Reviewing` sits at depth 1, so its own children are at depth 2 (42px);
    // hoisted, a kept row sits one step under the folder itself.
    cmdClick(wsRow('w-folders')!)
    collapseReviewing()

    expect(wsRow('w-folders')!.parentElement).toHaveStyle({ marginInlineStart: '42px' })
  })

  it('carries no treatment of its own — the row is simply still on screen', () => {
    render(<WorkspaceTree />)

    cmdClick(wsRow('w-plate')!)
    collapseReviewing()
    // Drop the multiselection: the row stays, and now wears nothing.
    fireEvent.click(rowFor('develop'))

    expect(wsRow('w-plate')).not.toBeNull()
    expect(wsRow('w-plate')).toHaveAttribute('aria-selected', 'false')
    expect(wsRow('w-plate')!.className).toContain('border-transparent')
  })

  it('survives navigating to an unrelated workspace', () => {
    const { rerender } = render(<WorkspaceTree />)

    cmdClick(wsRow('w-plate')!)
    collapseReviewing()

    router.pathname = '/ide/p1/r1/w-develop'
    rerender(<WorkspaceTree />)

    expect(wsRow('w-plate')).not.toBeNull()
    expect([...useSidebarSelectionStore.getState().kept]).toEqual(['w-plate'])
  })

  it('brings the kept row’s WHOLE subtree, and hoists only the outermost', () => {
    render(<WorkspaceTree />)

    cmdClick(wsRow('w-sidebar')!)
    cmdClick(wsRow('w-folders')!)
    collapseReviewing()

    // Both are kept, but `folders` is drawn under `sidebar` rather than twice:
    // one hoisted group, holding one row.
    expect(wsRow('w-sidebar')).not.toBeNull()
    expect(wsRow('w-folders')).not.toBeNull()
    const held = document.querySelectorAll('[data-held-rows]')
    expect(held).toHaveLength(1)
    expect(held[0].querySelectorAll('[data-ws-drop]')).toHaveLength(2)
  })

  it('is released by the parent’s fold-away control, and only by it', () => {
    render(<WorkspaceTree />)

    cmdClick(wsRow('w-plate')!)
    collapseReviewing()
    expect(wsRow('w-plate')).not.toBeNull()

    fireEvent.click(screen.getByLabelText('Fold away the rows Reviewing is holding'))

    expect(wsRow('w-plate')).toBeNull()
    expect(useSidebarSelectionStore.getState().kept.size).toBe(0)
  })

  it('offers the fold-away control only while there is something to fold', () => {
    render(<WorkspaceTree />)
    expect(screen.queryByLabelText('Fold away the rows Reviewing is holding')).toBeNull()

    cmdClick(wsRow('w-plate')!)
    collapseReviewing()

    expect(screen.getByLabelText('Fold away the rows Reviewing is holding')).toBeInTheDocument()
  })

  it('keeps fold-away row geometry stable and fades the control in', () => {
    render(<WorkspaceTree />)
    cmdClick(wsRow('w-plate')!)
    collapseReviewing()

    const classes = screen
      .getByLabelText('Fold away the rows Reviewing is holding')
      .className.split(/\s+/)
    expect(classes).toContain('hidden')
    expect(classes).toContain('group-hover:inline-flex')
    expect(classes).toContain('group-focus-within:inline-flex')
    expect(classes).toContain('group-hover:animate-row-action-in')
    expect(classes).toContain('group-focus-within:animate-row-action-in')
    expect(classes).not.toContain('invisible')
  })

  it('marks the folded folder with the three-dot state', () => {
    render(<WorkspaceTree />)
    expect(folderRow('f-rev')!.querySelector('[data-holding-rows]')).toBeNull()

    cmdClick(wsRow('w-plate')!)
    collapseReviewing()

    expect(folderRow('f-rev')!.querySelector('[data-holding-rows]')).not.toBeNull()
  })

  it('cmd-clicking a kept row off removes that one, leaving the rest', () => {
    render(<WorkspaceTree />)

    cmdClick(wsRow('w-plate')!)
    cmdClick(wsRow('w-sidebar')!)
    collapseReviewing()
    expect(wsRow('w-plate')).not.toBeNull()

    cmdClick(wsRow('w-plate')!)

    expect(wsRow('w-plate')).toBeNull()
    expect(wsRow('w-sidebar')).not.toBeNull()
  })

  it('expanding the parent releases what it was holding', () => {
    render(<WorkspaceTree />)

    cmdClick(wsRow('w-plate')!)
    collapseReviewing()
    fireEvent.click(within(folderRow('f-rev')!).getByLabelText('Expand'))

    expect(useSidebarSelectionStore.getState().kept.size).toBe(0)
    expect(document.querySelector('[data-held-rows]')).toBeNull()
  })
})

describe('a folded repo holds rows too', () => {
  it('keeps a selected row when the repo header folds over it', () => {
    render(<WorkspaceTree />)

    cmdClick(wsRow('w-plate')!)
    fireEvent.click(screen.getByLabelText('Collapse repo'))

    expect(wsRow('w-plate')).not.toBeNull()
    expect(wsRow('w-develop')).toBeNull()
    expect(screen.getByLabelText('Fold away the rows crowbar is holding')).toBeInTheDocument()
  })
})
