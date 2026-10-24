import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { SpaceScroller } from '@/components/sidebar/space-scroller'
import { handleCreateHomeThread } from '@/components/layout/home-actions'
import { performCreateHomeFolder } from '@/components/sidebar/lib/row-actions'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import { usePendingCreatesStore, getInitialPendingCreatesState } from '@/lib/store/pending-creates'
import { getWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'
import type { Project } from '@/lib/types'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

// SpacePanel's own useNavigate() call (used only to hand a navigate fn to
// handleCreateHomeThread below) — no Router is mounted in these unit tests.
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
}))

// The real resolver hits the network (fetchHomeWorkspace); every test here
// gets a pre-resolved home workspace id by default so the thread-button test
// below doesn't need to wait on an effect/fetch to land. A `vi.fn()`, not a
// bare arrow, so the not-yet-resolved regression test can override its
// return value for one call — `vi.hoisted`, since `vi.mock`'s own factory is
// hoisted above ordinary top-level `const`s and would otherwise close over
// this before it exists.
interface HomeWorkspaceStateMockResult {
  wsId: string | null
  owningChatId: string | null
  error: boolean
}
const useHomeWorkspaceStateMock = vi.hoisted(() =>
  vi.fn<(projectId: string | null) => HomeWorkspaceStateMockResult>(() => ({
    wsId: 'home-ws-1',
    owningChatId: null,
    error: false,
  })),
)
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  useHomeWorkspaceState: (projectId: string | null) => useHomeWorkspaceStateMock(projectId),
  ensureHomeWorkspaceResolved: vi.fn(),
  getHomeWorkspaceId: vi.fn(() => 'home-ws-1'),
}))

// SpacePanel's ONLY import from this module — mocked wholesale so the
// thread-button test can assert the call instead of exercising createChat.
vi.mock('@/components/layout/home-actions', () => ({
  handleCreateHomeThread: vi.fn(),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

// SpacePanel's only import from this module too — the rest of row-actions.ts
// (rename, lock, etc.) is exercised by row-context-menu.test.tsx, not here.
vi.mock('@/components/sidebar/lib/row-actions', () => ({
  performCreateHomeFolder: vi.fn(),
}))

// Task 21's drag wiring — no-op commit callbacks are enough for every test
// below, none of which exercises a live drag.
const onDrop = vi.fn()
const onPaneDrop = vi.fn()

// `SpacePanel` reads a narrow, project-scoped slice of the REAL sidebar
// store (for the "is any workspace under this project working" re-render
// signal) — the store's own default state (`repos: []`) is already exactly
// what every fixture here needs, so it is left unmocked (mocking the whole
// module would also replace `SidebarTree`'s own `collapsedChatRows` read,
// which every rendered row here depends on).
vi.mock('@/features/workspace/stores/workspace-store-registry', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('@/features/workspace/stores/workspace-store-registry')
  >()),
  getAllActiveWorkspaceIds: () => [],
  // Nothing is mounted here: a Recents member row falls back to the sidebar's
  // own chat record (`useRecentsChat`).
  getWorkspaceStore: () => undefined,
  getOrCreateWorkspaceStore: () => ({
    getState: () => ({
      panes: {},
      agentChats: { working: {}, chats: [] },
    }),
    subscribe: () => () => {},
  }),
  // `SidebarTreeRow`'s own per-row live-working subscription (sidebar-tree.tsx)
  // — every row carrying a `workspaceId` reads this on mount, home rows
  // included now that they carry a real one.
  subscribeChatWorking: () => () => {},
  readChatWorking: () => false,
}))

function makeProject(id: string): Project {
  return {
    id,
    name: id,
    path: `/repos/${id}`,
    lastActivity: new Date('2026-08-28T00:00:00Z'),
  }
}

function makeRow(id: string, label: string, over: Partial<SidebarRow> = {}): SidebarRow {
  return {
    id,
    kind: 'chat',
    parentId: null,
    order: 0,
    label,
    ownsWorktree: false,
    workspaceId: null,
    working: false,
    hasView: false,
    ...over,
  }
}

/**
 * jsdom lays nothing out, so `clientWidth` is 0 and the scroller's
 * panel-index arithmetic (`scrollLeft / clientWidth`) can't identify a panel.
 * `scrollLeft` itself is left a real, WRITABLE property — the re-align below
 * assigns to it, and a `value:`-defined one would throw under ESM strict mode
 * instead of recording the bounce.
 */
function givePanelGeometry(el: HTMLElement, panelWidth: number): void {
  Object.defineProperty(el, 'clientWidth', { value: panelWidth, configurable: true })
}

describe('SpaceScroller', () => {
  beforeEach(() => {
    // jsdom does not implement scrollTo
    HTMLElement.prototype.scrollTo = vi.fn()
    resetWindowPaneStoreForTests()
    useHomeTreeStore.setState({ trees: {} })
    usePendingCreatesStore.setState(getInitialPendingCreatesState())
    __resetWorkspaceScopesForTest()
    useHomeWorkspaceStateMock.mockReturnValue({
      wsId: 'home-ws-1',
      owningChatId: null,
      error: false,
    })
  })

  it('renders one panel per project, min-width 100%', () => {
    const projects = [makeProject('p1'), makeProject('p2')]
    render(
      <SpaceScroller
        projects={projects}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
        rowsForProject={() => []}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        onDrop={onDrop}
        onPaneDrop={onPaneDrop}
        onTrashProject={vi.fn()}
      />,
    )
    const panels = screen.getAllByTestId('space-panel')
    expect(panels).toHaveLength(2)
    expect(panels[0]).toHaveClass('min-w-full')
  })

  it('clicking a mark scrolls to that space', () => {
    const onChange = vi.fn()
    const projects = [makeProject('p1'), makeProject('p2')]
    render(
      <SpaceScroller
        projects={projects}
        activeProjectId="p1"
        onActiveProjectChange={onChange}
        rowsForProject={() => []}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        onDrop={onDrop}
        onPaneDrop={onPaneDrop}
        onTrashProject={vi.fn()}
      />,
    )
    const el = screen.getByTestId('space-scroll-region')
    fireEvent.wheel(el, { deltaX: 100 })
    givePanelGeometry(el, 400)
    el.scrollLeft = 400
    fireEvent.scroll(el)
    expect(onChange).toHaveBeenCalled()
  })

  // Regression: the vertical chat-list ScrollArea lives INSIDE this
  // horizontal carousel's own subtree, so a plain vertical wheel tick over
  // the chat list bubbles up here too. Left unguarded, that armed the "user
  // is swiping projects" flag exactly like a real horizontal swipe, and a
  // later scrollLeft change for any unrelated reason would silently swap
  // the active project mid-scroll — a vertical-dominant wheel event must
  // never arm it.
  it('a vertical-dominant wheel event does not arm the swipe gesture', () => {
    const onChange = vi.fn()
    const projects = [makeProject('p1'), makeProject('p2')]
    render(
      <SpaceScroller
        projects={projects}
        activeProjectId="p1"
        onActiveProjectChange={onChange}
        rowsForProject={() => []}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        onDrop={onDrop}
        onPaneDrop={onPaneDrop}
        onTrashProject={vi.fn()}
      />,
    )
    const el = screen.getByTestId('space-scroll-region')
    fireEvent.wheel(el, { deltaX: 0, deltaY: 100 })
    givePanelGeometry(el, 400)
    el.scrollLeft = 400
    fireEvent.scroll(el)
    expect(onChange).not.toHaveBeenCalled()
  })

  // Regression: the carousel could SETTLE on a panel the content area was not
  // showing. Only a horizontal wheel and a touchstart arm the swipe flag, but
  // `overflow-x: scroll` + mandatory x snapping + min-w-full panels means
  // anything else that moves scrollLeft — most ordinarily the browser
  // scrolling a focused element into view, and every panel is full of
  // tabbable row buttons — drags a whole panel into view while the route, the
  // content pane and the pane store's activeProjectId all stay behind. The
  // sidebar then names one space while the screen shows another.
  it('bounces back when something other than a swipe settles on another panel', () => {
    const onChange = vi.fn()
    const projects = [makeProject('p1'), makeProject('p2')]
    render(
      <SpaceScroller
        projects={projects}
        activeProjectId="p1"
        onActiveProjectChange={onChange}
        rowsForProject={() => []}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        onDrop={onDrop}
        onPaneDrop={onPaneDrop}
        onTrashProject={vi.fn()}
      />,
    )
    const el = screen.getByTestId('space-scroll-region')
    givePanelGeometry(el, 400)

    // No wheel, no touch — exactly what focus's own scrollIntoView does.
    el.scrollLeft = 400
    fireEvent.scroll(el)

    expect(el.scrollLeft).toBe(0)
    expect(onChange).not.toHaveBeenCalled()
  })

  it("leaves a settled scroll alone when it is already the active project's panel", () => {
    const projects = [makeProject('p1'), makeProject('p2')]
    render(
      <SpaceScroller
        projects={projects}
        activeProjectId="p2"
        onActiveProjectChange={vi.fn()}
        rowsForProject={() => []}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        onDrop={onDrop}
        onPaneDrop={onPaneDrop}
        onTrashProject={vi.fn()}
      />,
    )
    const el = screen.getByTestId('space-scroll-region')
    givePanelGeometry(el, 400)

    el.scrollLeft = 400
    fireEvent.scroll(el)

    expect(el.scrollLeft).toBe(400)
  })

  // The bounce must not fight the switch animation it is chasing: a
  // programmatic smooth scroll fires scroll events the whole way across, none
  // of them a user gesture, and re-aligning on an intermediate one would jump
  // straight to the destination — the switch would snap instead of glide.
  it('does not interrupt the smooth scroll of a switch already in flight', () => {
    const projects = [makeProject('p1'), makeProject('p2'), makeProject('p3')]
    const props = {
      projects,
      onActiveProjectChange: vi.fn(),
      rowsForProject: () => [],
      onOpen: vi.fn(),
      onTrash: vi.fn(),
      onCreate: vi.fn(),
      onDrop,
      onPaneDrop,
      onTrashProject: vi.fn(),
    }
    const { rerender } = render(<SpaceScroller {...props} activeProjectId="p1" />)
    const el = screen.getByTestId('space-scroll-region')
    givePanelGeometry(el, 400)

    rerender(<SpaceScroller {...props} activeProjectId="p3" />)
    expect(HTMLElement.prototype.scrollTo).toHaveBeenCalledWith({
      left: 800,
      behavior: 'smooth',
    })

    // Mid-flight, passing over p2's panel.
    el.scrollLeft = 400
    fireEvent.scroll(el)
    expect(el.scrollLeft).toBe(400)

    // Arrived. The next stray scroll is re-aligned again, not ignored.
    el.scrollLeft = 800
    fireEvent.scroll(el)
    el.scrollLeft = 0
    fireEvent.scroll(el)
    expect(el.scrollLeft).toBe(800)
  })

  // Addendum §1/§2: the row no longer carries a trash button at all (deleting
  // moved to drag-to-trash, built elsewhere), so `onTrash` — still threaded
  // through for type-compat with `SidebarTree`'s prop, see `sidebar-row.tsx`'s
  // own doc on it — has no control left to prove it reaches. Fork and Thread
  // are the two controls that took its place; both assert here.
  it("threads onOpen/onCreate through to each panel's SidebarTree, not stubbed no-ops", () => {
    const onOpen = vi.fn()
    const onCreate = vi.fn()
    const projects = [makeProject('p1')]
    const row = makeRow('row-1', 'Fix the thing', { kind: 'branch', parentId: 'parent-1' })
    render(
      <SpaceScroller
        projects={projects}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
        rowsForProject={() => [row]}
        onOpen={onOpen}
        onTrash={vi.fn()}
        onCreate={onCreate}
        onDrop={onDrop}
        onPaneDrop={onPaneDrop}
        onTrashProject={vi.fn()}
      />,
    )

    // Clicking the row body calls SidebarRow's onOpen, which SidebarTree wires
    // straight to whatever SidebarTree.props.onOpen was given — this only
    // fires with the real SpaceScroller-level onOpen, not a stubbed no-op.
    fireEvent.click(screen.getByText('Fix the thing'))
    expect(onOpen).toHaveBeenCalledWith('row-1')

    fireEvent.click(screen.getByRole('button', { name: `Fork ${row.label}` }))
    expect(onCreate).toHaveBeenCalledWith('row-1', 'workspace')

    fireEvent.click(screen.getByRole('button', { name: `Thread ${row.label}` }))
    expect(onCreate).toHaveBeenCalledWith('row-1', 'thread')
  })

  // The project row is gone (the switcher above names the space); its verbs
  // live in the foot under the tree instead.
  describe('tree foot', () => {
    const renderScroller = (
      overrides: Partial<{ onTrashProject: (id: string) => void; rows: SidebarRow[] }> = {},
    ) => {
      const projects = [makeProject('p1'), makeProject('p2')]
      render(
        <SpaceScroller
          projects={projects}
          activeProjectId="p1"
          onActiveProjectChange={vi.fn()}
          rowsForProject={() => overrides.rows ?? [makeRow('row-1', 'Fix the thing')]}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={vi.fn()}
          onDrop={onDrop}
          onPaneDrop={onPaneDrop}
          onTrashProject={overrides.onTrashProject ?? vi.fn()}
        />,
      )
      return within(screen.getAllByTestId('space-panel')[0])
    }
    const openMore = (panel: ReturnType<typeof renderScroller>) => {
      fireEvent.click(panel.getByRole('button', { name: 'More…' }))
      return screen.getAllByRole('menuitem')
    }

    it('draws no project row, and the tree still starts with its own rows', () => {
      const panel = renderScroller()
      expect(panel.queryByText('p1')).not.toBeInTheDocument()
      expect(panel.queryByRole('button', { name: /^(Expand|Collapse) p1$/ })).toBeNull()
      expect(panel.getByText('Fix the thing')).toBeInTheDocument()
    })

    it('puts a divider, then New thread and More…, after the last tree row inside the scroller', () => {
      const panel = renderScroller()
      const content = panel.getByTestId('space-scroll-content')
      const lastRow = panel.getByText('Fix the thing')
      const divider = panel.getByTestId('tree-foot-divider')
      const newThread = panel.getByRole('button', { name: 'New thread' })
      const more = panel.getByRole('button', { name: 'More…' })
      for (const el of [divider, newThread, more]) expect(content.contains(el)).toBe(true)
      expect(
        lastRow.compareDocumentPosition(divider) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy()
      expect(
        divider.compareDocumentPosition(newThread) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy()
      expect(
        newThread.compareDocumentPosition(more) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy()
    })

    it('an empty space still shows the divider and both rows', () => {
      const panel = renderScroller({ rows: [] })
      expect(panel.queryAllByRole('treeitem')).toHaveLength(0)
      expect(panel.getByTestId('tree-foot-divider')).toBeInTheDocument()
      expect(panel.getByRole('button', { name: 'New thread' })).toBeInTheDocument()
      expect(panel.getByRole('button', { name: 'More…' })).toBeInTheDocument()
    })

    // Not `onCreate(row, 'thread')`: that pipe resolves against the repo-scoped
    // store, which has no notion of project home at all.
    it("New thread starts a thread on the project's home workspace, not a repo's", () => {
      const onCreate = vi.fn()
      render(
        <SpaceScroller
          projects={[makeProject('p1')]}
          activeProjectId="p1"
          onActiveProjectChange={vi.fn()}
          rowsForProject={() => [makeRow('row-1', 'Fix the thing', { kind: 'branch' })]}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={onCreate}
          onDrop={onDrop}
          onPaneDrop={onPaneDrop}
          onTrashProject={vi.fn()}
        />,
      )
      fireEvent.click(screen.getByRole('button', { name: 'New thread' }))
      expect(handleCreateHomeThread).toHaveBeenCalledWith('p1', 'home-ws-1', expect.any(Function))
      expect(onCreate).not.toHaveBeenCalled()
    })

    it('New thread before the home workspace has resolved refuses with a toast, not silently', async () => {
      useHomeWorkspaceStateMock.mockReturnValue({ wsId: null, owningChatId: null, error: false })
      const { toast } = await import('@/features/window/stores/toast-store')
      const panel = renderScroller()
      // Mocks are never cleared between tests here: count, don't assert "never".
      const callsBefore = vi.mocked(handleCreateHomeThread).mock.calls.length
      fireEvent.click(panel.getByRole('button', { name: 'New thread' }))
      expect(handleCreateHomeThread).toHaveBeenCalledTimes(callsBefore)
      expect(toast.error).toHaveBeenCalledWith("Can't start a new thread yet")
    })

    it('More… opens a menu of exactly Import a repo, Create a new folder, Delete space', () => {
      const onTrashProject = vi.fn()
      const panel = renderScroller({ onTrashProject })
      const items = openMore(panel)
      expect(items.map((i) => i.textContent)).toEqual([
        'Import a repo',
        'Create a new folder',
        'Delete space',
      ])
      expect(onTrashProject).not.toHaveBeenCalled()
    })

    it("More…'s Import a repo opens the add-repository dialog", () => {
      const panel = renderScroller()
      fireEvent.click(openMore(panel)[0])
      expect(screen.getByRole('dialog', { name: 'Add repository' })).toBeInTheDocument()
    })

    it("More…'s Create a new folder starts a folder on the project's home workspace", () => {
      const panel = renderScroller()
      fireEvent.click(openMore(panel)[1])
      expect(performCreateHomeFolder).toHaveBeenCalledWith('p1')
    })

    it("More…'s Delete space trashes this space, not another", () => {
      const onTrashProject = vi.fn()
      const panel = renderScroller({ onTrashProject })
      fireEvent.click(openMore(panel)[2])
      expect(onTrashProject).toHaveBeenCalledExactlyOnceWith('p1')
    })
  })

  // The gap the user hit directly: a thread started via the header's Thread
  // button (or any other project-home chat) rendered nowhere once it left
  // Recents — the tree itself had no row for it at all.
  describe('the project-home tree (chats + folders)', () => {
    const HOME_ROW_ID = 'home-branch-row'

    // Explicit user correction: a first version drew a container "Home" row
    // (labelled with the project's own name, duplicating the project switcher
    // right above it) for these to nest under — rejected outright. Home's
    // chats/folders are FLAT TOP-LEVEL rows, exactly like a repo itself.
    it("renders the project's home chats and folders as flat top-level rows, not nested under anything", () => {
      useHomeTreeStore.setState({
        trees: {
          p1: {
            chats: [
              {
                id: HOME_ROW_ID,
                repoId: '',
                ownsWorktree: true,
                workspaceId: 'home-ws-1',
                title: '',
                order: 0,
              },
              {
                id: 'c-1',
                repoId: '',
                workspaceId: 'home-ws-1',
                title: 'Fix the thing',
                order: 0,
              },
            ],
            folders: [{ id: 'f-1', repoId: '', name: 'Notes', order: 1 }],
          },
        },
      })
      const projects = [makeProject('p1')]
      render(
        <SpaceScroller
          projects={projects}
          activeProjectId="p1"
          onActiveProjectChange={vi.fn()}
          rowsForProject={() => [makeRow('row-1', 'Repo thread', { kind: 'branch' })]}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={vi.fn()}
          onDrop={onDrop}
          onPaneDrop={onPaneDrop}
          onTrashProject={vi.fn()}
        />,
      )

      // No row names the project (the rejected "container" row, and the
      // retired project row, both did). Structural parentId:null / indent-depth coverage lives at the unit
      // level in rows-from-home.test.ts, which isn't tangled up in DOM
      // traversal the way an assertion here would be.
      expect(screen.queryByText('p1')).not.toBeInTheDocument()
      expect(screen.getByText('Fix the thing')).toBeInTheDocument()
      expect(screen.getByText('Notes')).toBeInTheDocument()
      expect(screen.getByText('Repo thread')).toBeInTheDocument()
    })

    // Regression, reported live: a create's mint and its placement are two
    // sequential backend writes (home-actions.ts's `waitForHomeChat`
    // own doc), so the REAL row can land in the home tree store — via a
    // reseed racing ahead of the placement write — before its placement is
    // actually correct. Rendering it the instant it merely EXISTS put a
    // second, wrongly-placed row on screen alongside the already-correctly-
    // placed pending placeholder, which is what still read as "a child
    // appears in the wrong place" even after the pending row's OWN clearing
    // was fixed to wait for the right parentId. The real row must stay
    // completely invisible — not just uncleared-pending — until its own
    // pending entry (linked by `realId`) is gone.
    it('hides the real row while its create is still unconfirmed, showing only the pending placeholder', () => {
      useHomeTreeStore.setState({
        trees: {
          p1: {
            chats: [
              {
                id: 'real-1',
                repoId: '',
                workspaceId: 'home-ws-1',
                title: 'Fresh thread',
                order: 0,
                // Still parented at root — its placement write has not
                // landed yet, exactly the frame that must never reach the
                // screen as its own visible row.
              },
            ],
            folders: [],
          },
        },
      })
      usePendingCreatesStore.setState({
        entries: [
          {
            tempId: 'pending-1',
            kind: 'chat',
            projectId: 'p1',
            parentId: '',
            order: 0,
            workspaceId: 'home-ws-1',
            ownsWorktree: false,
            status: 'creating',
            label: '',
            realId: 'real-1',
          },
        ],
      })
      const projects = [makeProject('p1')]
      render(
        <SpaceScroller
          projects={projects}
          activeProjectId="p1"
          onActiveProjectChange={vi.fn()}
          rowsForProject={() => []}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={vi.fn()}
          onDrop={onDrop}
          onPaneDrop={onPaneDrop}
          onTrashProject={vi.fn()}
        />,
      )

      // The real row is hidden — its title never reaches the screen while
      // unconfirmed, even though it already exists in the store.
      expect(screen.queryByText('Fresh thread')).not.toBeInTheDocument()

      // The create confirms (waitForHomeChat's own clear, elsewhere) — the
      // pending entry is gone, and the (now definitely correctly placed)
      // real row appears for the first time.
      act(() => {
        usePendingCreatesStore.setState({ entries: [] })
      })

      expect(screen.getByText('Fresh thread')).toBeInTheDocument()
    })

    // Caught live: a repo's own row kept its raw backend `order` while a home
    // chat's got recomputed into a positional index blind to repos — two
    // independently-dense sequences that collided the moment both shared the
    // project-home root, so a repo always rendered after every home row
    // regardless of what either one's `order` actually said. This renders
    // the real component (not just the row-building unit) to prove the fix
    // holds end to end: SpacePanel's own merge of rowsForProject + rowsFromHome.
    it('interleaves a repo among home chats by raw order, not "home always first"', () => {
      useHomeTreeStore.setState({
        trees: {
          p1: {
            chats: [
              {
                id: HOME_ROW_ID,
                repoId: '',
                ownsWorktree: true,
                workspaceId: 'home-ws-1',
                title: '',
                order: 0,
              },
              { id: 'c-testing', repoId: '', workspaceId: 'home-ws-1', title: 'testing', order: 1 },
            ],
            folders: [],
          },
        },
      })
      const projects = [makeProject('p1')]
      render(
        <SpaceScroller
          projects={projects}
          activeProjectId="p1"
          onActiveProjectChange={vi.fn()}
          rowsForProject={() => [
            makeRow('repo-row', 'checkout', {
              kind: 'branch',
              order: 0,
              repoIcon: {
                repoId: 'repo-1',
                projectId: 'p1',
                name: 'checkout',
                avatarLabel: 'C',
                avatarColor: 'bg-indigo-700',
              },
            }),
          ]}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={vi.fn()}
          onDrop={onDrop}
          onPaneDrop={onPaneDrop}
          onTrashProject={vi.fn()}
        />,
      )

      const labels = screen
        .getAllByRole('treeitem')
        .map((el) => el.textContent)
        .filter((t): t is string => t !== null)
      const repoIndex = labels.findIndex((t) => t.includes('checkout'))
      const testingIndex = labels.findIndex((t) => t.includes('testing'))
      expect(repoIndex).toBeGreaterThanOrEqual(0)
      expect(testingIndex).toBeGreaterThanOrEqual(0)
      // repo order 0 < chat order 1 — the repo must render FIRST.
      expect(repoIndex).toBeLessThan(testingIndex)
    })

    // Task 3's own backend regression
    // (TestRegression_UpdateRepo_SingleRepoDragDoesNotClampToZero) drags a
    // repo to a NON-ZERO position among real home siblings and asserts the
    // wire order lands exactly there. This is that same drag, asserted at
    // the RENDER boundary instead: the repo is dragged BETWEEN two home
    // chats, not to either edge, which is the one shape the deleted
    // `repoPositions` stand-in mechanism could get right for free (it fed
    // the repo into the very sort that produced the chats' own compacted
    // index) but a naive delete of just that mechanism cannot — a chat's
    // rendered `order` also has to carry ITS real wire value now (see
    // rows-from-home.test.ts's own "not a index compacted from array
    // position"), or the two rows collide the moment they share this root.
    it('a repo dragged BETWEEN two home chats renders between them, not always first or last', () => {
      useHomeTreeStore.setState({
        trees: {
          p1: {
            chats: [
              {
                id: HOME_ROW_ID,
                repoId: '',
                ownsWorktree: true,
                workspaceId: 'home-ws-1',
                title: '',
                order: 0,
              },
              { id: 'c-alpha', repoId: '', workspaceId: 'home-ws-1', title: 'alpha', order: 0 },
              // Gap at 1 is deliberate: after a repo-focused reorder, the
              // backend densifies the WHOLE merged sibling set (repo + home
              // chats/folders) together, so a chat sitting after the repo
              // keeps whatever slot that merge left it — never necessarily
              // adjacent to another chat's own order.
              { id: 'c-bravo', repoId: '', workspaceId: 'home-ws-1', title: 'bravo', order: 2 },
            ],
            folders: [],
          },
        },
      })
      const projects = [makeProject('p1')]
      render(
        <SpaceScroller
          projects={projects}
          activeProjectId="p1"
          onActiveProjectChange={vi.fn()}
          rowsForProject={() => [
            makeRow('repo-row', 'checkout', {
              kind: 'branch',
              order: 1,
              repoIcon: {
                repoId: 'repo-1',
                projectId: 'p1',
                name: 'checkout',
                avatarLabel: 'C',
                avatarColor: 'bg-indigo-700',
              },
            }),
          ]}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={vi.fn()}
          onDrop={onDrop}
          onPaneDrop={onPaneDrop}
          onTrashProject={vi.fn()}
        />,
      )

      const labels = screen
        .getAllByRole('treeitem')
        .map((el) => el.textContent)
        .filter((t): t is string => t !== null)
      const alphaIndex = labels.findIndex((t) => t.includes('alpha'))
      const repoIndex = labels.findIndex((t) => t.includes('checkout'))
      const bravoIndex = labels.findIndex((t) => t.includes('bravo'))
      expect(alphaIndex).toBeGreaterThanOrEqual(0)
      expect(repoIndex).toBeGreaterThanOrEqual(0)
      expect(bravoIndex).toBeGreaterThanOrEqual(0)
      expect(alphaIndex).toBeLessThan(repoIndex)
      expect(repoIndex).toBeLessThan(bravoIndex)
    })

    it('renders nothing extra before the daemon has backfilled the home workspace’s owning chat', () => {
      useHomeTreeStore.setState({ trees: { p1: { chats: [], folders: [] } } })
      const projects = [makeProject('p1')]
      render(
        <SpaceScroller
          projects={projects}
          activeProjectId="p1"
          onActiveProjectChange={vi.fn()}
          rowsForProject={() => [makeRow('row-1', 'Repo thread', { kind: 'branch' })]}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={vi.fn()}
          onDrop={onDrop}
          onPaneDrop={onPaneDrop}
          onTrashProject={vi.fn()}
        />,
      )

      expect(screen.getByText('Repo thread')).toBeInTheDocument()
    })

    // Every chat-scoped API call for a home workspace throws unless its
    // scope is RECORDED first (workspace-scope.ts) — `ide-shell.tsx` only
    // ever records the ACTIVE route's, but `SpacePanel` mounts one per
    // VISIBLE project. Without this, a project other than whichever one
    // happens to be on screen throws instead of working at all.
    it("records the home workspace's scope once resolved, even for a project that is not the active one", () => {
      const projects = [makeProject('p1')]
      render(
        <SpaceScroller
          projects={projects}
          activeProjectId="p2"
          onActiveProjectChange={vi.fn()}
          rowsForProject={() => []}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={vi.fn()}
          onDrop={onDrop}
          onPaneDrop={onPaneDrop}
          onTrashProject={vi.fn()}
        />,
      )

      expect(getWorkspaceScope('home-ws-1')).toEqual(
        expect.objectContaining({ projectId: 'p1', repoId: '' }),
      )
    })
  })
})
