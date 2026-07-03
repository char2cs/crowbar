import { describe, it, expect, beforeEach, beforeAll, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

// Router hooks used deep in the row — stub so the item renders in isolation.
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
}))

vi.mock('@/lib/api/workspace', () => ({ retryProvision: vi.fn().mockResolvedValue(undefined) }))

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

beforeAll(() => {
  if (!window.matchMedia) {
    window.matchMedia = ((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener() {},
      removeEventListener() {},
      addListener() {},
      removeListener() {},
      dispatchEvent: () => false,
    })) as never
  }
})

beforeEach(() => {
  useSidebarStore.setState({
    repos: [{ id: 'r1', name: 'repo', workspaces: [] }] as never,
    collapsedWorkspaces: new Set<string>(),
  })
})

const placeholderNode = {
  workspace: {
    id: 'ws-ph',
    branch: 'develop',
    status: 'locked',
    heldByPath: '/repo/home',
  },
  children: [],
}

const normalNode = {
  workspace: { id: 'ws-n', branch: 'feature/x', status: 'new' },
  children: [],
}

function renderItem(node: unknown, onWorkspaceClick = vi.fn()) {
  render(
    <WorkspaceTreeItem
      node={node as never}
      depth={0}
      repoId="r1"
      projectId="p1"
      activeWorkspaceId=""
      onWorkspaceClick={onWorkspaceClick}
    />,
  )
  return onWorkspaceClick
}

describe('WorkspaceTreeItem placeholder details', () => {
  it('hides the reason and actions until the row is clicked', () => {
    renderItem(placeholderNode)
    expect(screen.queryByText(/checked out at/i)).toBeNull()
    expect(screen.queryByRole('button', { name: /detach/i })).toBeNull()
  })

  it('expands the details inside the row on click, without navigating', async () => {
    const onClick = renderItem(placeholderNode)
    await userEvent.click(screen.getByText('develop'))
    expect(screen.getByText(/checked out at \/repo\/home/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /detach/i })).toBeInTheDocument()
    expect(onClick).not.toHaveBeenCalled()
  })

  it('collapses the details on a second click', async () => {
    renderItem(placeholderNode)
    await userEvent.click(screen.getByText('develop'))
    await userEvent.click(screen.getByText('develop'))
    expect(screen.queryByText(/checked out at/i)).toBeNull()
  })

  it('still navigates on click for non-placeholder rows', async () => {
    const onClick = renderItem(normalNode)
    await userEvent.click(screen.getByText('feature/x'))
    expect(onClick).toHaveBeenCalledWith('ws-n', 'p1', 'r1')
  })
})
