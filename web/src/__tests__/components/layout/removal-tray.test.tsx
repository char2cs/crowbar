/**
 * The removal tray, end to end through the real sidebar.
 *
 * What this covers that the planner cannot: that holding a row actually takes
 * it off screen, that Cancel puts the row, its subtree and a repo's contents
 * back with nothing sent, and that the delete fires exactly once the hairline
 * has drained.
 *
 * The eight seconds are fake timers — the drain is a CSS animation and the
 * commit is one `setTimeout`, so there is nothing here that needs real time to
 * pass. jsdom runs no animations, which is precisely why the timer and the bar
 * are two separate things: the bar is what you see, the timer is what fires.
 *
 * Driven through `SidebarTreeSurface` (Task 30) — SpaceScroller's real mount
 * point, which now hoists `RemovalTray` above it exactly as `SidebarTreePanel`
 * (Task 8, retired this task) rendered it inside the one flat tree before —
 * so a held row is proven to disappear from a REAL tree and not just from
 * the tray's own list.
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, act, fireEvent } from '@testing-library/react'

const router = vi.hoisted(() => ({ pathname: '/', navigate: vi.fn() }))
// Renders are counted through the leading glyph, exactly as
// workspace-tree-selection-renders.test.tsx does — every row draws it once, and
// React may replay a callback passed IN as a prop.
const renders = vi.hoisted(() => ({ count: 0 }))

vi.mock('@/components/layout/workspace-branch-icon', () => ({
  WorkspaceBranchIcon: () => {
    renders.count += 1
    return null
  },
  WorkspaceAgentSpinner: () => null,
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => router.navigate,
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: router.pathname } } }),
  useMatch: () => null,
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  deleteWorkspace: vi.fn(() => Promise.resolve()),
  deleteRepo: vi.fn(() => Promise.resolve()),
}))

vi.mock('@/lib/api/sidebar-placement', () => ({
  placeWorkspace: vi.fn(() => Promise.resolve()),
  placeFolder: vi.fn(() =>
    Promise.resolve({
      folder: { id: 'f1', repoId: 'r1', projectId: 'p1', name: 'spikes', order: 0 },
      shifted: [],
    }),
  ),
  placeRepo: vi.fn(() => Promise.resolve()),
  placeProject: vi.fn(() => Promise.resolve()),
  createFolder: vi.fn(() =>
    Promise.resolve({
      folder: { id: 'new-folder', repoId: 'r1', projectId: 'p1', name: 'New folder', order: 0 },
      shifted: [],
    }),
  ),
  // Task 34: DELETE .../chats/folders/:folderId answers {shifted: [...]},
  // not void — commitRemoval applies it (and the deletion's own tombstone)
  // to the sidebar store directly, since folders have no dedicated push
  // channel any more.
  deleteFolder: vi.fn(() => Promise.resolve([])),
}))

import { deleteRepo, deleteWorkspace } from '@/lib/api'
import { deleteFolder } from '@/lib/api/sidebar-placement'
import { idle, success } from '@/lib/loadable'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { SidebarTreeSurface } from '@/components/layout/sidebar-tree-surface'
import { planRemoval, type DragSubject } from '@/components/layout/removal-plan'
import type { Project } from '@/lib/types'

const project: Project = {
  id: 'p1',
  name: 'crowbar-project',
  path: '/p1',
  lastActivity: new Date(0),
}

/** SpaceScroller's real mount point, single-project (this suite's fixtures
 *  only ever seed one) — the same wiring `ide-shell.tsx` gives
 *  `SidebarTreeSurface` for real. */
function TestSidebar() {
  return (
    <SidebarTreeSurface
      projects={[project]}
      activeProjectId={project.id}
      onActiveProjectChange={() => {}}
    />
  )
}

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'w-default',
  workspaces: [
    { id: 'a', branch: 'alpha', status: 'new', age: '', order: 0 },
    { id: 'kid', branch: 'alpha/one', parentId: 'a', status: 'new', age: '', order: 0 },
    { id: 'b', branch: 'beta', status: 'new', age: '', order: 1 },
  ],
  folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 2 }],
  ...over,
})

// SidebarRow carries no id-bearing attribute yet (Part D's drag wiring adds
// one) — read the tree back by the labels it draws, in document order, which
// is exactly the hierarchical order the tree renders in. Scoped to
// `[data-sidebar-row-label]` (sidebar-row.tsx's own double-click-to-rename
// delegation marker), not the whole treeitem's textContent: since Task 5
// (icon personalization) the project-home row's glyph draws real text too —
// the repo's own letter tile (e.g. "C" for crowbar) — so the unscoped
// textContent would read "Ccrowbar" instead of "crowbar".
const rows = () =>
  Array.from(document.querySelectorAll('[role="treeitem"] [data-sidebar-row-label]')).map(
    (el) => el.textContent?.trim(),
  )

/** Put `subjects` in the tray, exactly as a drop on the pane would. */
function hold(...subjects: DragSubject[]) {
  act(() => {
    useRemovalTrayStore.getState().hold(planRemoval(subjects, useSidebarStore.getState().repos))
  })
}

const trayRow = () => document.querySelector('[data-removal-entry]') as HTMLElement

/** The seconds a held row is showing. */
const secs = () => document.querySelector('[data-removal-secs]')?.textContent

beforeEach(() => {
  vi.clearAllMocks()
  renders.count = 0
  // jsdom does not implement scrollTo — SpaceScroller (mounted for real via
  // SidebarTreeSurface as of Task 30) calls it to align its panel on every
  // activeProjectId/projects change, including on mount.
  HTMLElement.prototype.scrollTo = vi.fn()
  // NOT `shouldAdvanceTime`: the deadline is a wall-clock instant, so a fake
  // clock that also creeps with the real one turns "7999ms have passed" into a
  // race with however long the test itself took to get there.
  vi.useFakeTimers()
  router.pathname = '/'
  useWorkspaceListStore.setState({ data: idle() })
  useHomeWorkspaceStore.setState({ workspace: null })
  useProjectDataStore.setState({ data: success([project]) })
  useRemovalTrayStore.setState(getInitialRemovalState())
  useSidebarStore.setState({
    repos: [repo()],
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
})

afterEach(() => {
  vi.useRealTimers()
})

describe('holding a row', () => {
  it('takes the row and its subtree off screen without deleting anything', () => {
    render(<TestSidebar />)

    hold({ kind: 'workspace', id: 'a', repoId: 'r1' })

    expect(rows()).toEqual(['crowbar', 'beta', 'spikes'])
    expect(deleteWorkspace).not.toHaveBeenCalled()
    expect(screen.getByText('Removing')).toBeInTheDocument()
  })

  it('draws the row as an ordinary row, with a hairline draining under it', () => {
    render(<TestSidebar />)

    hold({ kind: 'workspace', id: 'a', repoId: 'r1' })

    expect(trayRow().className).toContain('h-9')
    expect(trayRow().textContent).toContain('alpha')
    // The subtree it takes with it.
    expect(trayRow().textContent).toContain('+1')
    expect(trayRow().querySelector('[data-removal-drain]')?.className).toContain(
      'animate-tray-drain',
    )
  })

  it('keeps each row in the face it wore in the tree', () => {
    // A branch is a git ref and reads in mono; a folder name is prose and does
    // not. Changing typeface on the way into the tray would read as a different
    // kind of thing at the one moment the user is deciding whether to keep it.
    render(<TestSidebar />)

    hold({ kind: 'workspace', id: 'a', repoId: 'r1' })
    expect(trayRow().querySelector('span.font-mono')).not.toBeNull()

    hold({ kind: 'folder', id: 'f1', repoId: 'r1' })
    const folderLabel = [...document.querySelectorAll('[data-removal-entry]')]
      .flatMap((row) => [...row.querySelectorAll('span')])
      .find((el) => el.textContent === 'spikes')
    expect(folderLabel?.className).toContain('font-sans')
    expect(folderLabel?.className.split(/\s+/)).not.toContain('font-mono')
  })
})

describe('the countdown', () => {
  it('deletes when the hairline has drained, and only the root of the subtree', async () => {
    render(<TestSidebar />)
    hold({ kind: 'workspace', id: 'a', repoId: 'r1' })

    await act(async () => {
      vi.advanceTimersByTime(7999)
    })
    expect(deleteWorkspace).not.toHaveBeenCalled()

    await act(async () => {
      vi.advanceTimersByTime(1)
    })
    // The daemon owns the cascade — the descendant is not deleted twice.
    expect(deleteWorkspace).toHaveBeenCalledExactlyOnceWith('p1', 'r1', 'a')
    expect(document.querySelector('[data-removal-entry]')).toBeNull()
  })

  it('deletes a folder through the folder endpoint, which reparents its children', async () => {
    render(<TestSidebar />)
    hold({ kind: 'folder', id: 'f1', repoId: 'r1' })

    await act(async () => {
      vi.advanceTimersByTime(8000)
    })

    expect(deleteFolder).toHaveBeenCalledExactlyOnceWith('p1', 'r1', 'f1')
    expect(deleteWorkspace).not.toHaveBeenCalled()
  })
})

/**
 * The bar says roughly how long; the figures say exactly how long. They are the
 * same clock, and the figures cost nothing to keep — one `textContent` write per
 * held row, from a timer that belongs to the tray rather than to any row.
 */
describe('the seconds, in figures', () => {
  it('starts at the full eight and counts down with the hairline', async () => {
    render(<TestSidebar />)
    hold({ kind: 'workspace', id: 'a', repoId: 'r1' })

    expect(secs()).toBe('8')

    await act(async () => {
      vi.advanceTimersByTime(1000)
    })
    expect(secs()).toBe('7')

    await act(async () => {
      vi.advanceTimersByTime(6000)
    })
    expect(secs()).toBe('1')
  })

  // The whole reason the hairline is a CSS animation: nothing about a row
  // waiting to be deleted may repaint the sidebar thirty times a second — nor
  // once a second, which is what a numeral held in state would cost.
  it('costs NOTHING to count — the figures are written, not rendered', async () => {
    render(<TestSidebar />)
    hold({ kind: 'workspace', id: 'a', repoId: 'r1' })
    renders.count = 0

    await act(async () => {
      vi.advanceTimersByTime(7000)
    })

    expect(secs()).toBe('1')
    expect(renders.count).toBe(0)
  })

  it('runs out into the removal itself', async () => {
    render(<TestSidebar />)
    hold({ kind: 'workspace', id: 'a', repoId: 'r1' })

    await act(async () => {
      vi.advanceTimersByTime(8000)
    })

    expect(deleteWorkspace).toHaveBeenCalledExactlyOnceWith('p1', 'r1', 'a')
    expect(secs()).toBeUndefined()
  })

  it('shows no clock on a repo, which waits on an answer instead', () => {
    render(<TestSidebar />)

    hold({ kind: 'repo', id: 'r1' })

    expect(secs()).toBeUndefined()
    expect(screen.getByText('Remove')).toBeInTheDocument()
  })
})

describe('the route the removal leaves behind', () => {
  // Only ONCE the delete has actually fired: for the whole eight seconds the
  // workspace still exists, and leaving it early would make Cancel a promise
  // the app had already broken.
  it('stays put while the row is only being held', async () => {
    router.pathname = '/ide/p1/r1/kid'
    render(<TestSidebar />)

    hold({ kind: 'workspace', id: 'a', repoId: 'r1' })

    await act(async () => {
      vi.advanceTimersByTime(7999)
    })
    expect(router.navigate).not.toHaveBeenCalled()
  })

  it('falls back to the parent once the delete has fired', async () => {
    router.pathname = '/ide/p1/r1/kid'
    render(<TestSidebar />)
    hold({ kind: 'workspace', id: 'kid', repoId: 'r1' })

    await act(async () => {
      vi.advanceTimersByTime(8000)
    })

    expect(router.navigate).toHaveBeenCalledWith({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId: 'p1', repoId: 'r1', wsId: 'a' },
    })
  })

  it('leaves a workspace that was not in what went alone', async () => {
    router.pathname = '/ide/p1/r1/b'
    render(<TestSidebar />)
    hold({ kind: 'workspace', id: 'a', repoId: 'r1' })

    await act(async () => {
      vi.advanceTimersByTime(8000)
    })

    expect(router.navigate).not.toHaveBeenCalled()
  })
})

describe('cancelling', () => {
  it('puts the row and its whole subtree back, with nothing sent', async () => {
    render(<TestSidebar />)
    hold({ kind: 'workspace', id: 'a', repoId: 'r1' })

    fireEvent.click(screen.getByLabelText('Keep alpha'))

    expect(rows()).toEqual(['crowbar', 'alpha', 'alpha/one', 'beta', 'spikes'])
    await act(async () => {
      vi.advanceTimersByTime(20000)
    })
    expect(deleteWorkspace).not.toHaveBeenCalled()
  })
})

describe('a repo, which takes every worktree under it', () => {
  it('waits on an answer instead of running a clock', async () => {
    render(<TestSidebar />)
    hold({ kind: 'repo', id: 'r1' })

    expect(rows()).toEqual([])
    expect(screen.getByText('Waiting on you')).toBeInTheDocument()

    await act(async () => {
      vi.advanceTimersByTime(60000)
    })
    expect(deleteRepo).not.toHaveBeenCalled()
  })

  it('gives the repo and its contents back on Cancel', () => {
    render(<TestSidebar />)
    hold({ kind: 'repo', id: 'r1' })

    fireEvent.click(screen.getByText('Cancel'))

    expect(rows()).toEqual(['crowbar', 'alpha', 'alpha/one', 'beta', 'spikes'])
    expect(deleteRepo).not.toHaveBeenCalled()
  })

  it('asks once more before removing, and does not delete on Remove alone', async () => {
    // Eight seconds of undo is not a proportionate safety net for every worktree
    // in a repo, so this row never ran a clock — and pressing Remove opens a
    // dialog that spells the cascade out rather than sending the delete.
    render(<TestSidebar />)
    hold({ kind: 'repo', id: 'r1' })

    await act(async () => {
      fireEvent.click(screen.getByText('Remove'))
    })

    expect(deleteRepo).not.toHaveBeenCalled()
    expect(screen.getByText(/All workspaces in this repository will be deleted/)).toBeVisible()
  })

  it('removes it once the confirmation is accepted', async () => {
    render(<TestSidebar />)
    hold({ kind: 'repo', id: 'r1' })

    await act(async () => {
      fireEvent.click(screen.getByText('Remove'))
    })
    await act(async () => {
      fireEvent.click(screen.getByText('Delete repository'))
    })

    expect(deleteRepo).toHaveBeenCalledExactlyOnceWith('p1', 'r1')
  })

  it('deletes nothing when the confirmation is dismissed, and keeps the row held', async () => {
    render(<TestSidebar />)
    hold({ kind: 'repo', id: 'r1' })

    await act(async () => {
      fireEvent.click(screen.getByText('Remove'))
    })
    // Scoped to the dialog: the tray row has a Cancel of its own, and the two
    // mean different things — this one backs out of the confirmation, that one
    // keeps the row.
    const dialog = document.querySelector('[data-slot="alert-dialog-popup"]')!
    await act(async () => {
      fireEvent.click(
        [...dialog.querySelectorAll('button')].find((b) => b.textContent === 'Cancel')!,
      )
    })

    expect(deleteRepo).not.toHaveBeenCalled()
    // Backing out of the dialog is not the same as keeping the row: the tray row
    // is still there, still offering both answers.
    expect(screen.getByText('Remove')).toBeVisible()
  })
})

describe('a page that ends mid-drain', () => {
  it('sends the removal it was holding, rather than losing it', async () => {
    // The bug this pins: the tray holds a row for eight seconds before sending
    // the delete, and the tray is memory. A reload inside that window — or an
    // HMR update, or quitting — used to drop the intent silently. The row had
    // already been hidden, so it LOOKED deleted, and the next boot read it
    // straight back off the daemon.
    render(<TestSidebar />)
    hold({ kind: 'workspace', id: 'a', repoId: 'r1' })
    expect(deleteWorkspace).not.toHaveBeenCalled()

    await act(async () => {
      window.dispatchEvent(new Event('pagehide'))
    })

    expect(deleteWorkspace).toHaveBeenCalledExactlyOnceWith('p1', 'r1', 'a', {
      // Without keepalive the request is cancelled with the document, which is
      // the whole reason a pagehide handler normally cannot do this.
      keepalive: true,
    })
  })

  it('does NOT fire a removal that was waiting on an answer', async () => {
    // A repo (and a project) sits in the tray with no clock, waiting on an
    // explicit confirmation. An unload is not that answer, and these are the two
    // removals that cascade — so the safe direction is to drop them.
    render(<TestSidebar />)
    hold({ kind: 'repo', id: 'r1' })

    await act(async () => {
      window.dispatchEvent(new Event('pagehide'))
    })

    expect(deleteRepo).not.toHaveBeenCalled()
  })
})
