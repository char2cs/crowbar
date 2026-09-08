import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { SpaceScroller } from '@/components/sidebar/space-scroller'
import { handleCreateHomeThread } from '@/components/layout/space-content-actions'
import { performCreateHomeFolder } from '@/components/sidebar/lib/row-actions'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { getWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'
import type { RecentsBandEntry } from '@/components/sidebar/recents-band'
import type { Project } from '@/lib/types'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

// SpacePanel's own useNavigate() call (used only to hand a navigate fn to
// handleCreateHomeThread below) — no Router is mounted in these unit tests.
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
}))

// The real resolver hits the network (fetchHomeWorkspace); every test here
// gets a pre-resolved home workspace id so the thread-button test below
// doesn't need to wait on an effect/fetch to land.
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  useHomeWorkspaceState: () => ({ wsId: 'home-ws-1', owningChatId: null, error: false }),
  ensureHomeWorkspaceResolved: vi.fn(),
}))

// SpacePanel's ONLY import from this module — mocked wholesale so the
// thread-button test can assert the call instead of exercising createChat.
vi.mock('@/components/layout/space-content-actions', () => ({
  handleCreateHomeThread: vi.fn(),
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
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getAllActiveWorkspaceIds: () => [],
  getOrCreateWorkspaceStore: () => ({
    getState: () => ({
      panes: {},
      agentChats: { working: {}, chats: [] },
      dormantArrangements: [],
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

const noRecents = () => [] as RecentsBandEntry[]

describe('SpaceScroller', () => {
  beforeEach(() => {
    // jsdom does not implement scrollTo
    HTMLElement.prototype.scrollTo = vi.fn()
    useHomeTreeStore.setState({ trees: {} })
    __resetWorkspaceScopesForTest()
  })

  it('renders one panel per project, min-width 100%', () => {
    const projects = [makeProject('p1'), makeProject('p2')]
    render(
      <SpaceScroller
        projects={projects}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
        rowsForProject={() => []}
        recentsForProject={noRecents}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        onFocusRecent={vi.fn()}
        onCloseRecent={vi.fn()}
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
        recentsForProject={noRecents}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        onFocusRecent={vi.fn()}
        onCloseRecent={vi.fn()}
        onDrop={onDrop}
        onPaneDrop={onPaneDrop}
        onTrashProject={vi.fn()}
      />,
    )
    const el = screen.getByTestId('space-scroll-region')
    fireEvent.wheel(el, { deltaX: 100 })
    Object.defineProperty(el, 'clientWidth', { value: 400, configurable: true })
    Object.defineProperty(el, 'scrollLeft', { value: 400, configurable: true })
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
        recentsForProject={noRecents}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        onFocusRecent={vi.fn()}
        onCloseRecent={vi.fn()}
        onDrop={onDrop}
        onPaneDrop={onPaneDrop}
        onTrashProject={vi.fn()}
      />,
    )
    const el = screen.getByTestId('space-scroll-region')
    fireEvent.wheel(el, { deltaX: 0, deltaY: 100 })
    Object.defineProperty(el, 'clientWidth', { value: 400, configurable: true })
    Object.defineProperty(el, 'scrollLeft', { value: 400, configurable: true })
    fireEvent.scroll(el)
    expect(onChange).not.toHaveBeenCalled()
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
        recentsForProject={noRecents}
        onOpen={onOpen}
        onTrash={vi.fn()}
        onCreate={onCreate}
        onFocusRecent={vi.fn()}
        onCloseRecent={vi.fn()}
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

  it("renders each project's RecentsBand below its SidebarTree, in the same scroll region", () => {
    const projects = [makeProject('p1')]
    const row = makeRow('row-1', 'Fix the thing')
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    render(
      <SpaceScroller
        projects={projects}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
        rowsForProject={() => [row]}
        recentsForProject={() => [entry]}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        onFocusRecent={vi.fn()}
        onCloseRecent={vi.fn()}
        onDrop={onDrop}
        onPaneDrop={onPaneDrop}
        onTrashProject={vi.fn()}
      />,
    )
    const panel = screen.getByTestId('space-panel')
    const tree = screen.getByText('Fix the thing')
    const band = screen.getByTestId('recents-band')
    // Same scroll region (one `ScrollArea` per panel, not two) — both the
    // tree row and the band live under the one panel.
    expect(panel.contains(tree)).toBe(true)
    expect(panel.contains(band)).toBe(true)
    // Recents renders BELOW the tree, per spec §2's layout.
    expect(tree.compareDocumentPosition(band) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('calls recentsForProject/onFocusRecent/onCloseRecent with the right project and entry', () => {
    const recentsForProject = vi.fn(() => [] as RecentsBandEntry[])
    const projects = [makeProject('p1'), makeProject('p2')]
    render(
      <SpaceScroller
        projects={projects}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
        rowsForProject={() => []}
        recentsForProject={recentsForProject}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        onFocusRecent={vi.fn()}
        onCloseRecent={vi.fn()}
        onDrop={onDrop}
        onPaneDrop={onPaneDrop}
        onTrashProject={vi.fn()}
      />,
    )
    expect(recentsForProject).toHaveBeenCalledWith('p1')
    expect(recentsForProject).toHaveBeenCalledWith('p2')
  })

  // Spec §6: "the tree keeps a bottom inset the height of the card." An
  // OUTER, non-scrolling spacer now carries this — not padding on the
  // scrollable content, which counted straight toward `scrollHeight` and
  // could show a scrollbar for a list that visually didn't need one (the
  // card's default open height is a third of the whole sidebar rail). Reads
  // `--card-bottom-inset` off the CSS cascade (written directly onto the
  // shared rail ancestor by sidebar-carousel.tsx — see ide-shell.tsx's
  // `railRef`) rather than a React prop threaded through every layer: a
  // prop here would re-render this panel (and every row in it) on every
  // frame of a resize drag. See sidebar-carousel.test.tsx's "does not
  // re-render the tree during a live drag" for the live end-to-end proof.
  it("reserves the card's bottom inset as an outer spacer, not scroll-content padding", () => {
    const projects = [makeProject('p1'), makeProject('p2')]
    render(
      <SpaceScroller
        projects={projects}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
        rowsForProject={() => []}
        recentsForProject={noRecents}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        onFocusRecent={vi.fn()}
        onCloseRecent={vi.fn()}
        onDrop={onDrop}
        onPaneDrop={onPaneDrop}
        onTrashProject={vi.fn()}
      />,
    )
    const spacers = screen.getAllByTestId('space-scroll-bottom-spacer')
    expect(spacers).toHaveLength(2)
    for (const spacer of spacers) {
      expect(spacer).toHaveStyle({ height: 'var(--card-bottom-inset, 0px)' })
      expect(spacer).toHaveClass('shrink-0')
    }
    // The scroll content itself carries no padding for this any more — the
    // whole point is keeping it out of `scrollHeight`.
    for (const content of screen.getAllByTestId('space-scroll-content')) {
      expect(content).not.toHaveStyle({ paddingBottom: 'var(--card-bottom-inset, 0px)' })
    }
  })

  it('renders nothing extra for a project with no recents entries', () => {
    const projects = [makeProject('p1')]
    render(
      <SpaceScroller
        projects={projects}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
        rowsForProject={() => []}
        recentsForProject={noRecents}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        onFocusRecent={vi.fn()}
        onCloseRecent={vi.fn()}
        onDrop={onDrop}
        onPaneDrop={onPaneDrop}
        onTrashProject={vi.fn()}
      />,
    )
    expect(screen.queryByTestId('recents-band')).not.toBeInTheDocument()
  })

  // Spec §4: the space header IS the panel's first element, and it is what
  // says which project this space is. Built in Task 10 and left with zero
  // importers until the final fix wave.
  describe('space header (spec §4)', () => {
    const renderScroller = (overrides: Partial<{ onTrashProject: () => void }> = {}) => {
      const projects = [makeProject('p1'), makeProject('p2')]
      const entry: RecentsBandEntry = {
        id: 'e1',
        localId: 'e1',
        chatIds: ['chat-1'],
        state: 'dormant',
        workspaceId: 'ws-1',
      }
      render(
        <SpaceScroller
          projects={projects}
          activeProjectId="p1"
          onActiveProjectChange={vi.fn()}
          rowsForProject={() => [makeRow('row-1', 'Fix the thing')]}
          recentsForProject={() => [entry]}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={vi.fn()}
          onFocusRecent={vi.fn()}
          onCloseRecent={vi.fn()}
          onDrop={onDrop}
          onPaneDrop={onPaneDrop}
          onTrashProject={overrides.onTrashProject ?? vi.fn()}
        />,
      )
    }

    it('renders one header per space, above that space\u2019s scroller', () => {
      renderScroller()
      const headers = screen.getAllByTestId('space-header-row')
      expect(headers).toHaveLength(2)
      const panel = screen.getAllByTestId('space-panel')[0]
      const content = screen.getAllByTestId('space-scroll-content')[0]
      expect(panel.contains(headers[0])).toBe(true)
      // The header is NOT inside the scroll region — spec §2 draws it
      // `flex: none` above the scroller, so it never scrolls away.
      expect(content.contains(headers[0])).toBe(false)
      expect(
        headers[0].compareDocumentPosition(content) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy()
    })

    // "Clicking the header folds the space: the tree goes, Recents stays."
    it('folding hides the tree and keeps Recents', () => {
      renderScroller()
      expect(screen.getAllByText('Fix the thing')).toHaveLength(2)
      expect(screen.getAllByTestId('recents-band')).toHaveLength(2)

      fireEvent.click(screen.getAllByTestId('space-header-row')[0])

      // p1's tree is gone; p2's header was not clicked, so its own tree stays.
      expect(screen.queryAllByText('Fix the thing')).toHaveLength(1)
      // ...and Recents survives the fold, in both.
      expect(screen.getAllByTestId('recents-band')).toHaveLength(2)
    })

    it('folds only the space whose header was clicked', () => {
      renderScroller()
      const headers = screen.getAllByTestId('space-header-row')
      expect(headers[0]).toHaveAttribute('aria-expanded', 'true')

      fireEvent.click(headers[0])

      expect(screen.getAllByTestId('space-header-row')[0]).toHaveAttribute('aria-expanded', 'false')
      expect(screen.getAllByTestId('space-header-row')[1]).toHaveAttribute('aria-expanded', 'true')
    })

    // Addendum §4: "the dropdown never carries a Delete item" -- deletion's
    // only path is drag-to-trash now (addendum §2), superseding spec §9's
    // "every row that owns something carries a trash" for the project
    // header the same way it superseded it for a row's own trash button.
    it('the add-menu opens with Import a repo / Create a folder, never a Delete item', () => {
      const onTrashProject = vi.fn()
      renderScroller({ onTrashProject })
      const header = screen.getAllByTestId('space-header-row')[0]

      fireEvent.mouseEnter(header) // the add-menu button only exists while active
      fireEvent.click(screen.getByTestId('add-menu'))

      expect(screen.getByText('Import a repo')).toBeInTheDocument()
      expect(screen.getByText('Create a folder')).toBeInTheDocument()

      expect(screen.queryByText('Delete \u201Cp1\u201D')).not.toBeInTheDocument()
      expect(onTrashProject).not.toHaveBeenCalled()
      // The add-menu click must not fold the space either way (SpaceHeader
      // stops propagation; this pins that the mount relies on it).
      expect(screen.getAllByTestId('space-header-row')[0]).toHaveAttribute('aria-expanded', 'true')
    })

    // Not `onCreate('row-1', 'thread')`: that pipe resolves against the
    // REPO-scoped sidebar store, which has no notion of project home at all
    // — the button must reach the project-home-aware sibling instead (the
    // exact bug this test guards against: threads landing on a repo's own
    // home row rather than the project's).
    it("the header's thread button starts a thread on the project's home workspace, not a repo's", () => {
      const onCreate = vi.fn()
      const projects = [makeProject('p1')]
      render(
        <SpaceScroller
          projects={projects}
          activeProjectId="p1"
          onActiveProjectChange={vi.fn()}
          rowsForProject={() => [makeRow('row-1', 'Fix the thing', { kind: 'branch' })]}
          recentsForProject={noRecents}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={onCreate}
          onFocusRecent={vi.fn()}
          onCloseRecent={vi.fn()}
          onDrop={onDrop}
          onPaneDrop={onPaneDrop}
          onTrashProject={vi.fn()}
        />,
      )
      const header = screen.getAllByTestId('space-header-row')[0]

      fireEvent.mouseEnter(header)
      fireEvent.click(screen.getByTestId('new-thread'))

      expect(handleCreateHomeThread).toHaveBeenCalledWith('p1', 'home-ws-1', expect.any(Function))
      expect(onCreate).not.toHaveBeenCalled()
    })

    // "Create a folder" used to target the FIRST repo's own home row — folders
    // were once thought repo-internal only. The backend's `/home/chats/folders`
    // mount says otherwise, so the project-level add-menu item has to reach it.
    it("the add-menu's Create a folder starts a folder on the project's home workspace", () => {
      renderScroller()
      const header = screen.getAllByTestId('space-header-row')[0]

      fireEvent.mouseEnter(header)
      fireEvent.click(screen.getByTestId('add-menu'))
      fireEvent.click(screen.getByText('Create a folder'))

      expect(performCreateHomeFolder).toHaveBeenCalledWith('p1')
    })
  })

  // The gap the user hit directly: a thread started via the header's Thread
  // button (or any other project-home chat) rendered nowhere once it left
  // Recents — the tree itself had no row for it at all.
  describe('the project-home tree (chats + folders)', () => {
    const HOME_ROW_ID = 'home-branch-row'

    // Explicit user correction: a first version drew a container "Home" row
    // (labelled with the project's own name, duplicating the SpaceHeader
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
                type: 'branch',
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
          recentsForProject={noRecents}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={vi.fn()}
          onFocusRecent={vi.fn()}
          onCloseRecent={vi.fn()}
          onDrop={onDrop}
          onPaneDrop={onPaneDrop}
          onTrashProject={vi.fn()}
        />,
      )

      // 'p1' exactly once now — the SpaceHeader alone. No second row repeats
      // it (the rejected "container" row this test used to also assert).
      // Structural parentId:null / indent-depth coverage lives at the unit
      // level in rows-from-home.test.ts, which isn't tangled up in DOM
      // traversal the way an assertion here would be.
      expect(screen.getAllByText('p1')).toHaveLength(1)
      expect(screen.getByText('Fix the thing')).toBeInTheDocument()
      expect(screen.getByText('Notes')).toBeInTheDocument()
      expect(screen.getByText('Repo thread')).toBeInTheDocument()
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
                type: 'branch',
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
          recentsForProject={noRecents}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={vi.fn()}
          onFocusRecent={vi.fn()}
          onCloseRecent={vi.fn()}
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
                type: 'branch',
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
          recentsForProject={noRecents}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={vi.fn()}
          onFocusRecent={vi.fn()}
          onCloseRecent={vi.fn()}
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
          recentsForProject={noRecents}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={vi.fn()}
          onFocusRecent={vi.fn()}
          onCloseRecent={vi.fn()}
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
          recentsForProject={noRecents}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onCreate={vi.fn()}
          onFocusRecent={vi.fn()}
          onCloseRecent={vi.fn()}
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
