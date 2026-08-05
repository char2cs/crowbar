/**
 * Renaming a project, from the same gesture every other renameable row uses.
 *
 * A project row had no rename at all until now: a repo had one on its name's
 * double-click, a folder and a branch had one on theirs, and the row above all
 * of them had none. These pin that it is the SAME gesture and the same editor —
 * a second rename path in a tree that already has one is how the two drift.
 */
import { render, screen, fireEvent, cleanup } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'

const renameProject = vi.hoisted(() => vi.fn(() => Promise.resolve()))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  renameProject,
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigateMock,
  useMatch: () => null,
}))

const navigateMock = vi.hoisted(() => vi.fn())
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

import { ProjectHomeRow } from '@/components/layout/project-home-row'
import { useProjectStore } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'

const PROJECT = { id: 'p1', name: 'harbour' }

beforeEach(() => {
  vi.clearAllMocks()
  useProjectStore.setState({ activeProjectId: 'p1', projects: [] })
  useHomeWorkspaceStore.setState({ workspace: null })
})

function renderRow(isRenaming = false) {
  const startRenaming = vi.fn()
  const stopRenaming = vi.fn()
  const view = render(
    <ProjectHomeRow
      project={PROJECT}
      isCollapsed={false}
      isRenaming={isRenaming}
      startRenaming={startRenaming}
      stopRenaming={stopRenaming}
    />,
  )
  return { ...view, startRenaming, stopRenaming }
}

describe('renaming a project', () => {
  it('opens the editor on a double-click of the NAME', () => {
    const { startRenaming } = renderRow()

    fireEvent.doubleClick(screen.getByText('harbour'))

    expect(startRenaming).toHaveBeenCalledWith('p1')
  })

  it('replaces the label with the shared inline editor while renaming', () => {
    renderRow(true)

    expect(screen.getByDisplayValue('harbour')).toBeInTheDocument()
    expect(screen.queryByText('harbour')).toBeNull()
  })

  it('reads in the SAME face as the label it replaced', () => {
    // The rule for every inline editor in this tree: the row must not change
    // typeface under the cursor the moment you start typing. This row's label is
    // mono, so its editor is mono. (A folder's editor is prose because a
    // folder's LABEL is prose — the two travel together.)
    const { container } = renderRow()
    const label = Array.from(container.querySelectorAll('span')).find(
      (el) => el.textContent === 'harbour',
    )!
    expect(label.className.split(/\s+/)).toContain('font-mono')

    cleanup()
    renderRow(true)
    expect(screen.getByDisplayValue('harbour').className.split(/\s+/)).toContain('font-mono')
  })

  it('sends the rename and closes the editor on confirm', () => {
    const { stopRenaming } = renderRow(true)

    const input = screen.getByDisplayValue('harbour')
    fireEvent.change(input, { target: { value: 'atlas' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(renameProject).toHaveBeenCalledWith('p1', 'atlas')
    expect(stopRenaming).toHaveBeenCalled()
  })

  it('sends nothing on cancel', () => {
    const { stopRenaming } = renderRow(true)

    fireEvent.keyDown(screen.getByDisplayValue('harbour'), { key: 'Escape' })

    expect(renameProject).not.toHaveBeenCalled()
    expect(stopRenaming).toHaveBeenCalled()
  })

  it('does not navigate on a click inside the editor', () => {
    // The row's own click opens project home; a click landing in the editor
    // must not take the user somewhere else mid-rename.
    renderRow(true)

    fireEvent.click(screen.getByRole('treeitem'))

    expect(navigateMock).not.toHaveBeenCalled()
  })
})
