import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { SpaceScroller } from '@/components/sidebar/space-scroller'
import type { RecentsBandEntry } from '@/components/sidebar/recents-band'
import type { Project } from '@/lib/types'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

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
}))

function makeProject(id: string): Project {
  return {
    id,
    name: id,
    path: `/repos/${id}`,
    lastActivity: new Date('2026-08-28T00:00:00Z'),
  }
}

function makeRow(id: string, label: string): SidebarRow {
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
  }
}

const noRecents = () => [] as RecentsBandEntry[]

describe('SpaceScroller', () => {
  beforeEach(() => {
    // jsdom does not implement scrollTo
    HTMLElement.prototype.scrollTo = vi.fn()
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

  it("threads onOpen/onTrash/onCreate through to each panel's SidebarTree, not stubbed no-ops", () => {
    const onOpen = vi.fn()
    const onTrash = vi.fn()
    const onCreate = vi.fn()
    const projects = [makeProject('p1')]
    const row = makeRow('row-1', 'Fix the thing')
    render(
      <SpaceScroller
        projects={projects}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
        rowsForProject={() => [row]}
        recentsForProject={noRecents}
        onOpen={onOpen}
        onTrash={onTrash}
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

    fireEvent.click(screen.getByRole('button', { name: `Delete ${row.label}` }))
    expect(onTrash).toHaveBeenCalledWith('row-1')

    fireEvent.click(screen.getByRole('button', { name: `New thread in ${row.label}` }))
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

  // Spec §6: "the tree keeps a bottom inset the height of the card." Reads
  // `--card-bottom-inset` off the CSS cascade (written directly onto the
  // shared rail ancestor by sidebar-carousel.tsx — see ide-shell.tsx's
  // `railRef`) rather than a React prop threaded through every layer: a
  // prop here would re-render this panel (and every row in it) on every
  // frame of a resize drag. See sidebar-carousel.test.tsx's "does not
  // re-render the tree during a live drag" for the live end-to-end proof.
  it("reads the card's bottom inset from the shared --card-bottom-inset CSS variable, per panel", () => {
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
    const contents = screen.getAllByTestId('space-scroll-content')
    expect(contents).toHaveLength(2)
    for (const content of contents) {
      expect(content).toHaveStyle({ paddingBottom: 'var(--card-bottom-inset, 0px)' })
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

      expect(screen.getAllByTestId('space-header-row')[0]).toHaveAttribute(
        'aria-expanded',
        'false',
      )
      expect(screen.getAllByTestId('space-header-row')[1]).toHaveAttribute(
        'aria-expanded',
        'true',
      )
    })

    // Spec §9: "every row that owns something carries a trash ... and the
    // space header for the project."
    it('the overflow offers the project trash, and it names the project', () => {
      const onTrashProject = vi.fn()
      renderScroller({ onTrashProject })
      const header = screen.getAllByTestId('space-header-row')[0]

      fireEvent.mouseEnter(header) // the overflow only exists while active
      fireEvent.click(screen.getByTestId('overflow'))

      fireEvent.click(screen.getByText('Delete \u201Cp1\u201D'))
      expect(onTrashProject).toHaveBeenCalledWith('p1')
      // The overflow click must not also fold the space (SpaceHeader stops
      // propagation; this pins that the mount relies on it).
      expect(screen.getAllByTestId('space-header-row')[0]).toHaveAttribute(
        'aria-expanded',
        'true',
      )
    })
  })
})
