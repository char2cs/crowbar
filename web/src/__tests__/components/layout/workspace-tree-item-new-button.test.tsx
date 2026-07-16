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
const drag = { draggingWs: null, hoverTargetId: null, movingWsId: null }

vi.mock('@/components/layout/workspace-tree-context', () => ({
  useWorkspaceTreeActions: () => actions,
  useWorkspaceTreeDrag: () => drag,
}))

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

// A parent WITH a child, expanded, so the children section (and its trailing
// "New" add-child button) renders.
const parentWithChild = {
  workspace: { id: 'ws-parent', branch: 'feature/parent' },
  children: [{ workspace: { id: 'ws-child', branch: 'feature/child' }, children: [] }],
}

describe('WorkspaceTreeItem "New" child button width', () => {
  // Regression — twin of agent-chats-panel's NewChatRow: a <button> is
  // shrink-to-fit at display:flex in WebKit, so it must fill the row via its
  // container being flex-col (stretch to width − ROW_BASE's mx-1.5), NEVER via
  // `w-full`, which forces width:100% PLUS the 6px side margins and overflows
  // the Workspaces panel into a stray horizontal scrollbar.
  it('the New button fills width via flex-col stretch, never w-full', () => {
    render(
      <WorkspaceTreeItem
        node={parentWithChild as never}
        depth={0}
        repoId="r1"
        projectId="p1"
        activeWorkspaceId=""
        onWorkspaceClick={() => {}}
      />,
    )
    const btn = screen.getByText('New').closest('button')
    expect(btn).not.toBeNull()
    expect(btn!.className).not.toContain('w-full')
    expect(btn!.parentElement?.className).toContain('flex-col')
  })
})
