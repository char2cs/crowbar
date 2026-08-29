import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { SpaceScroller } from '@/components/sidebar/space-scroller'
import type { RecentsBandEntry } from '@/components/sidebar/recents-band'
import type { Project } from '@/lib/types'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

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
      />,
    )
    expect(recentsForProject).toHaveBeenCalledWith('p1')
    expect(recentsForProject).toHaveBeenCalledWith('p2')
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
      />,
    )
    expect(screen.queryByTestId('recents-band')).not.toBeInTheDocument()
  })
})
