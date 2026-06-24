import { describe, it, expect, beforeEach, beforeAll, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

// Router hooks used deep in the row — stub so the item renders in isolation.
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
}))

// Controlled tree context: we drive pendingCreates directly so the test can put
// an in-flight create under a LEAF workspace without going through the API.
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
  pendingCreates: new Map<string, { repoId: string; parentId: string; branch: string; error?: string }>(),
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
  // WorkspaceAgentSpinner (vendored Spinner) reads matchMedia, absent in jsdom.
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
  actions.pendingCreates = new Map()
})

const leafNode = {
  workspace: { id: 'ws-leaf', branch: 'feature/leaf' },
  children: [],
}

function renderItem() {
  return render(
    <WorkspaceTreeItem
      node={leafNode as never}
      depth={0}
      repoId="r1"
      projectId="p1"
      activeWorkspaceId=""
      onWorkspaceClick={() => {}}
    />,
  )
}

describe('WorkspaceTreeItem optimistic pending create', () => {
  // Regression: confirmCreate clears creatingChildOf immediately, so for a leaf
  // workspace (no children, input already hidden) the children section used to
  // collapse and the optimistic spinner row vanished — a create showed nothing.
  it('renders the optimistic spinner row under a LEAF while a child create is in flight', () => {
    actions.pendingCreates = new Map([
      ['temp-1', { repoId: 'r1', parentId: 'ws-leaf', branch: 'feature/new' }],
    ])

    renderItem()

    expect(screen.getByText('feature/new')).toBeInTheDocument()
  })

  it('does not render a pending row that belongs to a different parent', () => {
    actions.pendingCreates = new Map([
      ['temp-1', { repoId: 'r1', parentId: 'some-other-ws', branch: 'feature/elsewhere' }],
    ])

    renderItem()

    expect(screen.queryByText('feature/elsewhere')).toBeNull()
  })
})
