/**
 * Contract pins for the project row.
 *
 * It discloses exactly like every other row in the tree: a leading mark that
 * only says what the row IS, and the shared trailing chevron
 * (RowDisclosureButton) that folds it. The row body opens project home.
 *
 * The pins below are the ones that regressed once already. The leading slot used
 * to BE the collapse button, swapping Library⇄chevron on `display` at hover —
 * two collapse affordances in one row, neither of them where the repo header one
 * row below put its own. If that swap ever comes back these tests fail: the
 * leading mark must carry no hover-`display` classes and no collapse label, and
 * the chevron must be the row's LAST child. The mark is a button again, but for
 * a different job: it opens the icon editor, the same one the repo avatar has.
 *
 * No "Switch project" button: every project has a row now, so the pushed
 * switcher panel had nothing left to show.
 */
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ProjectHomeRow } from '@/components/layout/project-home-row'
import { DISCLOSURE_GLYPH_PATH, ROW_SUB_ACTION } from '@/components/layout/workspace-row-base'
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
  actions.onPointerDownDrag.mockClear()
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

describe('ProjectHomeRow trailing disclosure', () => {
  it('folds the project from the trailing chevron, and does not navigate', () => {
    renderRow()

    fireEvent.click(screen.getByLabelText('Collapse home'))

    expect(useSidebarStore.getState().collapsedProjects.has('p1')).toBe(true)
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it('re-opens from the same control while collapsed', () => {
    useSidebarStore.setState({ collapsedProjects: new Set(['p1']) })
    renderRow(true)

    fireEvent.click(screen.getByLabelText('Expand home'))

    expect(useSidebarStore.getState().collapsedProjects.has('p1')).toBe(false)
  })

  it('does not arm a row drag when the chevron is pressed', () => {
    renderRow()

    fireEvent.pointerDown(screen.getByLabelText('Collapse home'), { bubbles: true })

    expect(actions.onPointerDownDrag).not.toHaveBeenCalled()
  })

  it('closes the row — the chevron is its LAST child, as on every other row', () => {
    renderRow()

    const row = screen.getByRole('treeitem')
    expect(row.lastElementChild).toBe(screen.getByLabelText('Collapse home'))
  })

  it('is the row’s ONLY collapse control', () => {
    renderRow()

    const row = screen.getByRole('treeitem')
    const collapseControls = Array.from(row.querySelectorAll('button')).filter((b) =>
      /^(Collapse|Expand)\b/.test(b.getAttribute('aria-label') ?? ''),
    )
    expect(collapseControls).toHaveLength(1)
  })

  it('rotates the chevron 90° open and back closed', () => {
    renderRow()
    expect(
      screen.getByLabelText('Collapse home').querySelector('svg')!.getAttribute('class'),
    ).toContain('rotate-90')

    renderRow(true)
    expect(
      screen.getByLabelText('Expand home').querySelector('svg')!.getAttribute('class'),
    ).not.toContain('rotate-90')
  })

  it('draws the same chevron, from the same shared control, as the tree below it', () => {
    renderRow()

    const chevron = screen.getByLabelText('Collapse home').querySelector('svg path')!
    expect(chevron.getAttribute('d')).toBe(DISCLOSURE_GLYPH_PATH)
    expect(screen.getByLabelText('Collapse home').className).toBe(ROW_SUB_ACTION)
  })

  it('has dropped the "Switch project" button — every project has a row now', () => {
    renderRow()
    expect(screen.queryByLabelText('Switch project')).toBeNull()
  })
})

describe('ProjectHomeRow leading mark', () => {
  it('is identity and its editor — never a second collapse control', () => {
    // It IS a button: the mark is the trigger for the icon editor, the same
    // mechanism the repo avatar has. What it must never be again is a COLLAPSE
    // control — that lived here once, giving the row two of them.
    renderRow()

    const trigger = screen.getByLabelText('Edit home icon')
    expect(trigger).toBeInTheDocument()
    expect(/^(Collapse|Expand)\b/.test(trigger.getAttribute('aria-label') ?? '')).toBe(false)
  })

  it('never swaps on hover — the swap forced a layout on every row crossed', () => {
    // The regression this file exists to catch: `display:none/block` toggled by
    // `group-hover` changed which box sat in the row's flex flow, so merely
    // passing the pointer over a project relaid the row out.
    const { container } = renderRow()

    const glyph = container.querySelector(LEAD_GLYPH)!
    expect(glyph.getAttribute('class') ?? '').not.toMatch(/group-hover:(hidden|block)/)
    expect(container.querySelector('[class*="group-hover:hidden"]')).toBeNull()
  })

  it('stays put while the project is collapsed', () => {
    // The mark says which project this is; folding it does not change that.
    const { container } = renderRow(true)
    expect(container.querySelector(LEAD_GLYPH)).not.toBeNull()
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
    // The real flicker spinner, theme-token colored.
    expect(container.querySelector('[data-flicker-spinner]')).not.toBeNull()
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
