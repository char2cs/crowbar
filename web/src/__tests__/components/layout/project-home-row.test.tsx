/**
 * Contract pins for the project row.
 *
 * Two gestures share one row and must not blur into each other: the leading
 * slot folds the project's whole tree, everything else opens project home. The
 * slot holds the Library glyph and the collapse chevron, and exactly ONE of them
 * is ever in the flow — `display`, never opacity, or the label would shift as
 * they cross-fade. The chevron stays put for as long as the project is closed,
 * hover or not, because a folded project whose only affordance appeared on hover
 * would be a dead end.
 *
 * There is no trailing chevron and no "Switch project" button: every project has
 * a row now, so the pushed switcher panel had nothing left to show.
 */
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ProjectHomeRow } from '@/components/layout/project-home-row'
import { useProjectStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import type { WorkspaceDTO } from '@/lib/types'

// The row's glyphs are Lucide, which is dual-published and shares the process's
// React copy, so it needs no mock (unlike @phosphor-icons/react, whose pure-ESM
// build picks up its own React and throws "Cannot read properties of null
// (reading 'useContext')"). Lucide stamps `lucide-<name>` on every icon's class,
// which is what the leading-mark assertions below query.
const LEAD_GLYPH = '.lucide-library'

const navigateMock = vi.fn()
let mockMatch: { params: { projectId: string } } | null = null

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigateMock,
  useMatch: () => mockMatch,
}))

// The row joins the tree's drag system (it is a drop target for repos and a
// drag subject itself), so it reads both halves of the tree context. Driven
// directly here: these tests render the row on its own, with no provider.
const drag = vi.hoisted(() => ({
  draggingWs: null,
  draggingIds: new Set<string>(),
  dropTarget: null as { kind: string; id: string; mode: string } | null,
  movingIds: new Set<string>(),
}))
const actions = vi.hoisted(() => ({ onPointerDownDrag: vi.fn() }))

vi.mock('@/components/layout/workspace-tree-context', () => ({
  useWorkspaceTreeActions: () => actions,
  useWorkspaceTreeDrag: () => drag,
}))

const PROJECT = { id: 'p1', name: 'home' }

function renderRow(isCollapsed = false, project = PROJECT) {
  return render(<ProjectHomeRow project={project} isCollapsed={isCollapsed} />)
}

function homeDTO(working: boolean): WorkspaceDTO {
  return {
    id: 'home-1',
    repoId: '',
    projectId: 'p1',
    branch: 'home',
    status: 'new',
    working,
  } as WorkspaceDTO
}

beforeEach(() => {
  navigateMock.mockClear()
  mockMatch = null
  useProjectStore.setState({ activeProjectId: 'p1', projects: [] })
  useHomeWorkspaceStore.setState({ workspace: null })
  useSidebarStore.setState({ collapsedProjects: new Set<string>() })
})

describe('ProjectHomeRow', () => {
  it('is a treeitem, not a button — it lives inside the sidebar tree', () => {
    renderRow()
    expect(screen.getByRole('treeitem')).toBeInTheDocument()
    expect(screen.getByRole('treeitem').textContent).toContain('home')
  })

  it('reflects its own collapse state on the row', () => {
    renderRow()
    expect(screen.getByRole('treeitem')).toHaveAttribute('aria-expanded', 'true')
    renderRow(true)
    expect(screen.getAllByRole('treeitem')[1]).toHaveAttribute('aria-expanded', 'false')
  })

  it('navigates to home route on click', () => {
    renderRow()
    fireEvent.click(screen.getByRole('treeitem'))
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/ide/$projectId/home',
      params: { projectId: 'p1' },
    })
  })

  it('opens the row’s OWN project, not whichever one is active', () => {
    renderRow(false, { id: 'p9', name: 'other' })
    fireEvent.click(screen.getByRole('treeitem'))
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/ide/$projectId/home',
      params: { projectId: 'p9' },
    })
    expect(useProjectStore.getState().activeProjectId).toBe('p9')
  })

  it('applies ROW_ACTIVE class when the route matches THIS project', () => {
    mockMatch = { params: { projectId: 'p1' } }
    renderRow()
    expect(screen.getByRole('treeitem').className).toContain('bg-background')
  })

  it('stays inactive when the home route belongs to another project', () => {
    mockMatch = { params: { projectId: 'p2' } }
    renderRow()
    expect(screen.getByRole('treeitem').className).toContain('border-transparent')
  })

  it('navigates on Enter key', () => {
    renderRow()
    fireEvent.keyDown(screen.getByRole('treeitem'), { key: 'Enter' })
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/ide/$projectId/home',
      params: { projectId: 'p1' },
    })
  })

  it('navigates on Space key', () => {
    renderRow()
    fireEvent.keyDown(screen.getByRole('treeitem'), { key: ' ' })
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/ide/$projectId/home',
      params: { projectId: 'p1' },
    })
  })

  it('ignores an Enter that bubbled up from a nested control', () => {
    renderRow()
    fireEvent.keyDown(screen.getByLabelText('Import repository'), { key: 'Enter', bubbles: true })
    expect(navigateMock).not.toHaveBeenCalled()
  })
})

describe('ProjectHomeRow glyph ⇄ chevron swap', () => {
  it('folds the project when the glyph slot is clicked, and does not navigate', () => {
    renderRow()

    fireEvent.click(screen.getByLabelText('Collapse home'))

    expect(useSidebarStore.getState().collapsedProjects.has('p1')).toBe(true)
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it('re-opens from the same slot while collapsed', () => {
    useSidebarStore.setState({ collapsedProjects: new Set(['p1']) })
    renderRow(true)

    fireEvent.click(screen.getByLabelText('Expand home'))

    expect(useSidebarStore.getState().collapsedProjects.has('p1')).toBe(false)
  })

  it('shows the glyph at rest and swaps to the chevron on hover — by display', () => {
    const { container } = renderRow()

    const glyph = container.querySelector(LEAD_GLYPH)!
    const chevron = screen
      .getByLabelText('Collapse home')
      .querySelector('svg:not(.lucide-library)')!

    // Rest: glyph in the flow, chevron out of it.
    expect(glyph.getAttribute('class')).toContain('group-hover:hidden')
    expect(glyph.getAttribute('class')).not.toMatch(/(^|\s)hidden(\s|$)/)
    expect(chevron.getAttribute('class')).toMatch(/(^|\s)hidden(\s|$)/)
    expect(chevron.getAttribute('class')).toContain('group-hover:block')

    // Neither mark fades: opacity would leave both boxes in the flow.
    expect(glyph.getAttribute('class')).not.toMatch(/\bopacity-0\b/)
    expect(chevron.getAttribute('class')).not.toMatch(/\bopacity-0\b/)
  })

  it('keeps the chevron visible while collapsed, hover or not', () => {
    // A folded project must not be a dead end.
    const { container } = renderRow(true)

    const chevron = screen.getByLabelText('Expand home').querySelector('svg:not(.lucide-library)')!
    expect(chevron.getAttribute('class')).toMatch(/(^|\s)block(\s|$)/)
    expect(chevron.getAttribute('class')).not.toContain('group-hover:block')
    // ...and the Library glyph is gone entirely while it is.
    expect(container.querySelector(LEAD_GLYPH)?.getAttribute('class')).toContain('hidden')
  })

  it('rotates the chevron 90° open and 0° collapsed', () => {
    renderRow()
    expect(
      screen
        .getByLabelText('Collapse home')
        .querySelector('svg:not(.lucide-library)')!
        .getAttribute('class'),
    ).toContain('rotate-90')

    renderRow(true)
    expect(
      screen
        .getByLabelText('Expand home')
        .querySelector('svg:not(.lucide-library)')!
        .getAttribute('class'),
    ).toContain('rotate-0')
  })

  it('animates the swap at the spec’s timings', () => {
    const { container } = renderRow()
    expect(container.querySelector(LEAD_GLYPH)!.getAttribute('class')).toContain(
      '[transition:transform_0.1s,opacity_0.15s]',
    )
  })

  it('carries no trailing chevron — the leading slot is the only collapse control', () => {
    renderRow()

    const row = screen.getByRole('treeitem')
    const collapseControls = Array.from(row.querySelectorAll('button')).filter((b) =>
      /^(Collapse|Expand) /.test(b.getAttribute('aria-label') ?? ''),
    )
    expect(collapseControls).toHaveLength(1)
    // ...and it leads the row rather than closing it.
    expect(row.firstElementChild).toBe(collapseControls[0])
  })

  it('has dropped the "Switch project" button — every project has a row now', () => {
    renderRow()
    expect(screen.queryByLabelText('Switch project')).toBeNull()
  })
})

// The home row is the home workspace's tile: an agent working in project home
// must move its icon into the loading state, exactly as a worktree row's branch
// glyph becomes the spinner (WorkspaceBranchIcon).
describe('ProjectHomeRow working overlay', () => {
  it('renders the flicker spinner while the home workspace is working', () => {
    useHomeWorkspaceStore.setState({ workspace: homeDTO(true) })

    const { container } = renderRow()

    expect(screen.getByRole('status', { name: 'Loading' })).toBeInTheDocument()
    // The real flicker spinner (self-animating SVG), theme-token colored.
    expect(container.querySelector('svg animate')).not.toBeNull()
    expect(container.querySelector('.text-foreground')).not.toBeNull()
    // The leading mark yields to it for the duration of the turn.
    expect(container.querySelector(LEAD_GLYPH)).toBeNull()
  })

  it('renders the leading mark (no spinner) when the home workspace is idle', () => {
    useHomeWorkspaceStore.setState({ workspace: homeDTO(false) })

    const { container } = renderRow()

    expect(screen.queryByRole('status', { name: 'Loading' })).toBeNull()
    expect(container.querySelector(LEAD_GLYPH)).not.toBeNull()
  })

  it('renders the leading mark before the home workspace has been read', () => {
    useHomeWorkspaceStore.setState({ workspace: null })

    const { container } = renderRow()

    expect(screen.queryByRole('status', { name: 'Loading' })).toBeNull()
    expect(container.querySelector(LEAD_GLYPH)).not.toBeNull()
  })

  it('does not spin a NON-active project’s row', () => {
    // useHomeWorkspaceStore has a single slot, tracked for the active project
    // only — every other project's row would otherwise borrow its spinner.
    useHomeWorkspaceStore.setState({ workspace: homeDTO(true) })

    const { container } = renderRow(false, { id: 'p9', name: 'other' })

    expect(screen.queryByRole('status', { name: 'Loading' })).toBeNull()
    expect(container.querySelector(LEAD_GLYPH)).not.toBeNull()
  })
})
