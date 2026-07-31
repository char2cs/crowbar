import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

// Router hooks used deep in the row — stub so the item renders in isolation.
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
}))

const actions = {
  creatingChildOf: null as unknown,
  startCreating: vi.fn(),
  confirmCreate: vi.fn(),
  cancelCreate: vi.fn(),
  renamingId: null as string | null,
  startRenaming: vi.fn(),
  confirmRename: vi.fn(),
  cancelRename: vi.fn(),
  onPointerDownDrag: vi.fn(),
  pendingCreates: new Map<
    string,
    { repoId: string; parentId: string; branch: string; error?: string }
  >(),
  clearPendingCreate: vi.fn(),
}
const drag = {
  draggingWs: null,
  draggingIds: new Set<string>(),
  dropTarget: null,
  movingIds: new Set<string>(),
}

vi.mock('@/components/layout/workspace-tree-context', () => ({
  useWorkspaceTreeActions: () => actions,
  useWorkspaceTreeDrag: () => drag,
}))

import { EMPTY_REPO_TREE } from '@/components/layout/workspace-tree-utils'
import { useSidebarStore } from '@/lib/store/sidebar'
import { WorkspaceTreeItem } from '@/components/layout/workspace-tree-item'

beforeEach(() => {
  useSidebarStore.setState({
    repos: [{ id: 'r1', name: 'repo', workspaces: [] }] as never,
    collapsedWorkspaces: new Set<string>(),
  })
  actions.creatingChildOf = null
  actions.pendingCreates = new Map()
})

// A parent WITH a child, expanded, so the children section renders in full.
const parentWithChild = {
  kind: 'workspace',
  id: 'ws-parent',
  workspace: { id: 'ws-parent', branch: 'feature/parent' },
  children: [
    {
      kind: 'workspace',
      id: 'ws-child',
      workspace: { id: 'ws-child', branch: 'feature/child' },
      children: [],
    },
  ],
}

function renderParent() {
  return render(
    <WorkspaceTreeItem
      node={parentWithChild as never}
      depth={0}
      repoId="r1"
      projectId="p1"
      activeWorkspaceId=""
      onWorkspaceClick={() => {}}
      tree={EMPTY_REPO_TREE}
    />,
  )
}

describe('the standing "New" row is gone', () => {
  // This test used to pin the row's WIDTH. The row itself has since been
  // removed: at three levels of nesting it stacked three deep at different
  // indents with nothing on screen saying which level you were adding to. The
  // row's own hover "+" answers that unambiguously, so the pin is inverted —
  // what must not come back is the row.
  it('closes an expanded children section with nothing at all', () => {
    renderParent()

    expect(screen.queryByText('New')).toBeNull()
  })

  it('still offers the row its own add-child "+"', () => {
    // The affordance did not go away with the row; it moved onto the row that
    // owns the level, which is the whole point of removing it.
    renderParent()

    expect(screen.getAllByLabelText('Add child workspace').length).toBeGreaterThan(0)
  })

  it('opens the inline create input in the section instead', () => {
    // The one thing that DOES close the section is the input the "+" opens, and
    // it keeps the flex-col stretch the removed row needed: a row is
    // shrink-to-fit at display:flex in WebKit, and `w-full` would force
    // width:100% PLUS ROW_BASE's 6px side margins into a stray scrollbar.
    actions.creatingChildOf = { repoId: 'r1', parentId: 'ws-parent' }
    renderParent()

    const input = screen.getByPlaceholderText('branch-name, or name/ for a folder')
    const indented = input.closest('[style*="margin-inline-start"]')
    expect(indented?.className).toContain('flex-col')
    expect(indented?.className).not.toContain('w-full')
  })
})
