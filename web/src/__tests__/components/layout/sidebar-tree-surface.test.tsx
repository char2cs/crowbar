/**
 * `SidebarTreeSurface` wires `SpaceScroller` + the hoisted chrome
 * (RemovalTray/dialogs/context menu) against the real removal-tray-filtered
 * `repos` — the thing `sidebar-tree-panel.tsx` used to do for one flat tree,
 * now scoped per project. Individual pieces (derivers, handlers, RecentsBand
 * wiring) have their own focused unit tests; this one checks the WIRING:
 * real multi-project repos genuinely produce one tree per project, each
 * excluding the other's rows.
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, act, waitFor, within } from '@testing-library/react'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useRouterState: () => '/',
}))
// SpaceScroller's/RecentsBand's cross-store reads — no pane/chat fixtures are
// exercised here, so the registry can safely report nothing active.
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getAllActiveWorkspaceIds: () => [],
  getOrCreateWorkspaceStore: () => ({
    getState: () => ({
      panes: {},
      dormantArrangements: [],
    }),
    subscribe: () => () => {},
  }),
}))
// Kept stubbed rather than removed: several rendered rows (the promote
// dropdown, the space header overflow) can reach a real `toast.error` call on
// a rejected promise, and the real store would otherwise queue a live toast
// during a render test.
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

import { SidebarTreeSurface } from '@/components/layout/sidebar-tree-surface'
import { getInitialState, useSidebarStore, type Chat, type Repo } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { planRemoval } from '@/components/layout/removal-plan'
import * as api from '@/lib/api'
import {
  __resetHomeWorkspaceResolverForTest,
  ensureHomeWorkspaceResolved,
  getHomeWorkspaceId,
} from '@/features/workspace/lib/home-workspace-resolver'
import {
  loadPinFixture,
  outline,
  PIN_OUTLINE,
  SIDEBAR_TREE_ROWS,
} from '@/__tests__/__fixtures__/hierarchy-pin'
import type { Project, WorkspaceDTO } from '@/lib/types'

const projectA: Project = { id: 'p1', name: 'proj-a', path: '/p1', lastActivity: new Date(0) }
const projectB: Project = { id: 'p2', name: 'proj-b', path: '/p2', lastActivity: new Date(0) }

/**
 * A repo as the tree actually receives one: its home workspace already owns the
 * `branch` row the daemon's boot backfill mints, because `SidebarTreeSurface`
 * only builds rows for a repo whose chat seed has landed (`seededRepoIds`), and
 * by then it always does.
 */
const repo = (over: Partial<Repo> = {}): Repo => {
  const base: Repo = {
    id: 'r1',
    projectId: 'p1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    defaultWorkspaceId: 'home-1',
    workspaces: [],
    ...over,
  }
  const home: Chat = {
    id: `${base.id}-home-row`,
    repoId: base.id,
    ownsWorktree: true,
    workspaceId: base.defaultWorkspaceId,
    title: '',
    order: 0,
  }
  return {
    ...base,
    defaultOwningChatId: base.defaultOwningChatId ?? home.id,
    chats: [home, ...(base.chats ?? [])],
  }
}

/** Put repos in the store AND declare their trees read, which is the only state
 *  in which rows are drawn at all. */
const seedRepos = (repos: Repo[]) => {
  useSidebarStore.setState({ repos })
  useFolderSignalStore.setState({ seededRepoIds: new Set(repos.map((r) => r.id)) })
}

beforeEach(() => {
  vi.clearAllMocks()
  // jsdom does not implement scrollTo — SpaceScroller calls it on mount to
  // align its panel.
  HTMLElement.prototype.scrollTo = vi.fn()
  useSidebarStore.setState(getInitialState())
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  useFolderSignalStore.setState({ generations: {}, seededRepoIds: new Set() })
  useHomeTreeStore.setState({ trees: {} })
  // GET /home: p1's home resolves, every other project's stays in flight.
  __resetHomeWorkspaceResolverForTest()
  vi.spyOn(api, 'fetchHomeWorkspace').mockImplementation((projectId) =>
    projectId === 'p1'
      ? Promise.resolve({ id: 'home-ws-1', owningChatId: '' } as WorkspaceDTO)
      : new Promise(() => {}),
  )
})

/** The chrome reads the home workspace id without subscribing, so it must resolve before render. */
const homeResolved = async (projectId: string) => {
  ensureHomeWorkspaceResolved(projectId)
  await waitFor(() => expect(getHomeWorkspaceId(projectId)).not.toBeNull())
}

describe('SidebarTreeSurface', () => {
  it("shows each project's own repo, excluding the other project's", () => {
    seedRepos([
      repo({ id: 'r1', projectId: 'p1', defaultWorkspaceId: 'home-a', name: 'repo-a' }),
      repo({ id: 'r2', projectId: 'p2', defaultWorkspaceId: 'home-b', name: 'repo-b' }),
    ])

    render(
      <SidebarTreeSurface
        projects={[projectA, projectB]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
      />,
    )

    const panels = screen.getAllByTestId('space-panel')
    expect(panels).toHaveLength(2)
    expect(panels[0]).toHaveTextContent('repo-a')
    expect(panels[0]).not.toHaveTextContent('repo-b')
    expect(panels[1]).toHaveTextContent('repo-b')
    expect(panels[1]).not.toHaveTextContent('repo-a')
  })

  // RemovalTray no longer mounts here — the drag-to-trash addendum (§2) moved
  // it into SidebarCarousel, rendered at the top of the file explorer card.
  // "Mounts once, not once per project" is now covered there
  // (sidebar-carousel.test.tsx), not at this component's level.

  // Explicit product request: an X on a row transforms it IN PLACE (its
  // countdown + Keep/undo control) instead of hiding it while a separate
  // tray shows the same thing elsewhere — so a held leaf row (no subtree of
  // its own) stays on screen, not gone. `descendantHiddenIds` is what
  // computes this: it strips a workspace/chat/folder entry's cascade but
  // keeps its own primary id visible, unlike a repo/project entry (which has
  // no row of its own to transform — see that function's own doc).
  it('a row held for removal stays on screen, transformed in place, rather than disappearing', () => {
    seedRepos([
      repo({
        id: 'r1',
        projectId: 'p1',
        defaultWorkspaceId: 'home-a',
        workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
      }),
    ])

    render(
      <SidebarTreeSurface
        projects={[projectA]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
      />,
    )

    act(() => {
      treeUiStore
        .getState()
        .hold(
          planRemoval(
            [{ kind: 'workspace', id: 'ws-a', repoId: 'r1' }],
            useSidebarStore.getState().repos,
          ),
        )
    })

    expect(screen.getByText('alpha')).toBeInTheDocument()
    expect(document.querySelector('[data-removal-secs]')).toBeInTheDocument()
  })

  // Addendum §1/§2: the row no longer carries a trash button at all — a
  // click-driven confirm-and-delete flow (Task 25's real-flow insertion
  // point, replaced here) is superseded by a drag-to-trash gesture built
  // elsewhere on the same removal-tray machinery. This is the wiring half of
  // that removal: no row here — a workspace, a locked branch, or a chat —
  // exposes a `Delete …` control any more, and `SidebarTreeSurface` no
  // longer mounts a `DeleteConfirmDialog` of its own to drive one.
  it('no row exposes a delete control any more — deleting moved off the row', () => {
    seedRepos([
      repo({
        id: 'r1',
        projectId: 'p1',
        defaultWorkspaceId: 'home-a',
        workspaces: [
          { id: 'ws-a', branch: 'alpha', age: '', order: 0 },
          { id: 'ws-locked', branch: 'locked-one', age: '', order: 1, status: 'locked' },
        ],
        chats: [
          { id: 'c1', repoId: 'r1', title: 'a chat', order: 0 },
          {
            id: 'locked-one-row',
            repoId: 'r1',
            ownsWorktree: true,
            workspaceId: 'ws-locked',
            title: '',
            order: 0,
          },
        ],
      }),
    ])

    render(
      <SidebarTreeSurface
        projects={[projectA]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
      />,
    )

    expect(screen.queryByRole('button', { name: /delete/i })).not.toBeInTheDocument()
    expect(document.querySelector('[data-control="trash"]')).not.toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})

// The `+` mark's own panel replaces SpaceScroller in place — not a panel
// inside its scroll-snap track — so the touchpad swipe between real spaces
// can never land on it by accident.
describe('SidebarTreeSurface — creatingSpace swaps SpaceScroller for CreateSpacePanel', () => {
  it('renders SpaceScroller when not creating a space', () => {
    render(
      <SidebarTreeSurface
        projects={[projectA]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
      />,
    )
    expect(screen.getByTestId('space-scroll-region')).toBeInTheDocument()
    expect(screen.queryByTestId('create-space-panel')).not.toBeInTheDocument()
  })

  it('renders CreateSpacePanel instead of SpaceScroller while creating a space', () => {
    render(
      <SidebarTreeSurface
        projects={[projectA]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
        creatingSpace
        onCreateSpace={vi.fn()}
        onCancelCreateSpace={vi.fn()}
      />,
    )
    expect(screen.getByTestId('create-space-panel')).toBeInTheDocument()
    expect(screen.queryByTestId('space-scroll-region')).not.toBeInTheDocument()
  })

  it('Cancel calls onCancelCreateSpace', () => {
    const onCancelCreateSpace = vi.fn()
    render(
      <SidebarTreeSurface
        projects={[projectA]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
        creatingSpace
        onCreateSpace={vi.fn()}
        onCancelCreateSpace={onCancelCreateSpace}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onCancelCreateSpace).toHaveBeenCalledOnce()
  })
})

/**
 * `Repo.chats`'s own contract: an absent chat list means "not yet", never
 * "this repo has no chats" — the chats/folders reseed loop is independent of
 * the repo and workspace streams. A branch row's identity comes from the chat
 * that owns its workspace, so building rows during that window would hand out
 * ids that change the moment the seed lands. A row id is the React key, the
 * collapse key and the selection key: the tree would silently drop the user's
 * folds a beat after painting. So it draws nothing until the seed is in.
 */
describe('SidebarTreeSurface — rows wait for the repo’s tree seed', () => {
  it('draws no rows for a repo whose chat seed has not landed', () => {
    useSidebarStore.setState({
      repos: [repo({ id: 'r1', projectId: 'p1', defaultWorkspaceId: 'home-a', name: 'repo-a' })],
    })
    // Deliberately NOT marked seeded.

    render(
      <SidebarTreeSurface
        projects={[projectA]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
      />,
    )

    expect(screen.queryByText('repo-a')).not.toBeInTheDocument()
  })

  it('draws them once the seed lands, and only for the repo that seeded', () => {
    useSidebarStore.setState({
      repos: [
        repo({ id: 'r1', projectId: 'p1', defaultWorkspaceId: 'home-a', name: 'repo-a' }),
        repo({ id: 'r2', projectId: 'p1', defaultWorkspaceId: 'home-b', name: 'repo-b' }),
      ],
    })
    useFolderSignalStore.setState({ generations: {}, seededRepoIds: new Set(['r1']) })

    render(
      <SidebarTreeSurface
        projects={[projectA]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
      />,
    )

    expect(screen.getByText('repo-a')).toBeInTheDocument()
    expect(screen.queryByText('repo-b')).not.toBeInTheDocument()
  })
})

/**
 * `SidebarTreeChrome`'s `rows` prop (which `SidebarRowContextMenu` and the
 * double-click-to-rename listener both look a right-clicked/double-clicked
 * id up in) used to come ONLY from `treeRepos.flatMap(deriveRows)` — a
 * project-home chat or folder was never a member of it, so right-clicking
 * (or double-clicking) one found no matching row and did nothing at all: no
 * menu, no inline editor. Caught live as "can't create folders with right
 * click" / "can't rename folders", for exactly the home-scoped ones.
 */
describe('SidebarTreeSurface — a project-home row is reachable by the context menu', () => {
  it('right-clicking a project-home folder opens Rename/New folder, not a silent no-op', async () => {
    useHomeTreeStore.setState({
      trees: {
        p1: {
          chats: [
            {
              id: 'home-branch-chat',
              repoId: '',
              ownsWorktree: true,
              workspaceId: 'home-ws-1',
              title: '',
              order: 0,
            },
          ],
          folders: [{ id: 'home-folder-1', repoId: '', name: 'Docs', order: 0 }],
        },
      },
    })
    await homeResolved('p1')

    render(
      <SidebarTreeSurface
        projects={[projectA]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
      />,
    )

    const row = screen.getByText('Docs').closest('[role="treeitem"]')!
    fireEvent.contextMenu(row)

    expect(screen.getByText('Rename')).toBeInTheDocument()
    expect(screen.getByText('New folder')).toBeInTheDocument()
  })
})

/**
 * The hierarchy invariant every later stage must keep: project-home rows flat at
 * the top, interleaved with repo rows; branches, threads, forks and folders under
 * their own parents; no ghost rows; no fork on a project-home row.
 */
describe('hierarchy pin', () => {
  const renderPin = async () => {
    loadPinFixture()
    await homeResolved('p1')
    return render(
      <SidebarTreeSurface
        projects={[projectA]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
      />,
    )
  }

  it('renders the pinned hierarchy', async () => {
    const { container } = await renderPin()
    await waitFor(() => expect(outline(container)).toEqual(PIN_OUTLINE))
  })

  it('never draws a row twice or a row without a label', async () => {
    const { container } = await renderPin()
    await waitFor(() => expect(outline(container)).toHaveLength(PIN_OUTLINE.length))
    const ids = outline(container).map((line) => /\[(.+)\]$/.exec(line)?.[1])
    expect(new Set(ids).size).toBe(ids.length)
    for (const row of container.querySelectorAll(SIDEBAR_TREE_ROWS)) {
      expect(row.getAttribute('aria-label')?.trim()).toBeTruthy()
    }
  })

  it('offers no fork on a project-home row', async () => {
    const { container } = await renderPin()
    await waitFor(() => expect(outline(container)).toEqual(PIN_OUTLINE))
    const rowOf = (id: string) =>
      container.querySelector<HTMLElement>(`[data-sidebar-row-id="${id}"]`)!
    // A repo branch row does offer one, so the absence below is not vacuous.
    expect(
      within(rowOf('oc-feat')).getByRole('button', { name: 'Fork Feature A' }),
    ).toBeInTheDocument()
    for (const id of ['c-notes', 'f-ideas']) {
      expect(rowOf(id).querySelector('[data-control="fork"]')).toBeNull()
      fireEvent.contextMenu(rowOf(id))
      const menu = await screen.findByRole('menu')
      expect(within(menu).getByRole('menuitem', { name: 'Rename' })).toBeInTheDocument()
      expect(within(menu).queryByRole('menuitem', { name: /fork/i })).not.toBeInTheDocument()
      fireEvent.keyDown(menu, { key: 'Escape' })
      await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    }
  })
})
