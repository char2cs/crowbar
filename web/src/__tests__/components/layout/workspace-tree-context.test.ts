import { describe, it, expect, beforeEach, vi } from 'vitest'

vi.mock('@/lib/api', () => ({
  postWorkspace: vi.fn(),
  deleteWorkspace: vi.fn(),
}))

vi.mock('@/lib/api/workspace', () => ({
  reparentWorkspace: vi.fn(),
}))

vi.mock('@/components/ui/toast', () => ({
  toast: { error: vi.fn() },
}))

import { postWorkspace, deleteWorkspace as apiDeleteWorkspace } from '@/lib/api'
import { reparentWorkspace } from '@/lib/api/workspace'
import { toast } from '@/components/ui/toast'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import {
  performCreateWorkspace,
  performDeleteWorkspace,
  performReparentWorkspace,
} from '@/components/layout/workspace-tree-context'

const repo = (workspaces: Repo['workspaces']): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'repo',
  avatarLabel: 'R',
  avatarColor: '#fff',
  workspaces,
})

function workspaceIds(): string[] {
  return useSidebarStore.getState().repos.flatMap((r) => r.workspaces.map((w) => w.id))
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.spyOn(console, 'error').mockImplementation(() => {})
  useSidebarStore.setState({
    repos: [
      repo([
        { id: 'ws-parent', branch: 'main', age: 'now' },
        { id: 'ws-locked', branch: 'master', status: 'locked', age: 'now' },
      ]),
    ],
  })
})

describe('performCreateWorkspace', () => {
  it('fires the hierarchical 202 mutation threading projectId+repoId', async () => {
    vi.mocked(postWorkspace).mockResolvedValue(undefined)

    await performCreateWorkspace('r1', 'feat/x', 'ws-parent')

    expect(postWorkspace).toHaveBeenCalledWith('p1', 'r1', 'feat/x', 'ws-parent')
  })

  it('does NOT optimistically add a node — the WS DTO drives the cache', async () => {
    vi.mocked(postWorkspace).mockResolvedValue(undefined)

    await performCreateWorkspace('r1', 'feat/x', 'ws-parent')

    // No optimistic insert: the tree is unchanged until the WS WorkspaceDTO lands.
    expect(workspaceIds()).toEqual(['ws-parent', 'ws-locked'])
  })

  it('surfaces a failure via toast and adds no phantom node', async () => {
    vi.mocked(postWorkspace).mockRejectedValue(new Error('500 boom'))

    await performCreateWorkspace('r1', 'feat/x', 'ws-parent')

    expect(workspaceIds()).toEqual(['ws-parent', 'ws-locked'])
    expect(toast.error).toHaveBeenCalledWith('Failed to create workspace', '500 boom')
  })
})

describe('performDeleteWorkspace', () => {
  it('fires the hierarchical 202 delete threading projectId+repoId', async () => {
    vi.mocked(apiDeleteWorkspace).mockResolvedValue(undefined)

    await performDeleteWorkspace('ws-parent')

    expect(apiDeleteWorkspace).toHaveBeenCalledWith('p1', 'r1', 'ws-parent')
  })

  it('does NOT optimistically remove — the WS tombstone drives the cache', async () => {
    vi.mocked(apiDeleteWorkspace).mockResolvedValue(undefined)

    await performDeleteWorkspace('ws-parent')

    // No optimistic removal: the node stays until the status:'deleted' DTO lands.
    expect(workspaceIds()).toEqual(['ws-parent', 'ws-locked'])
  })

  it('surfaces a failure via toast', async () => {
    vi.mocked(apiDeleteWorkspace).mockRejectedValue(new Error('409 conflict'))

    await performDeleteWorkspace('ws-parent')

    expect(toast.error).toHaveBeenCalledWith('Failed to delete workspace', '409 conflict')
  })

  it('never deletes a locked workspace (no API call)', async () => {
    await performDeleteWorkspace('ws-locked')

    expect(apiDeleteWorkspace).not.toHaveBeenCalled()
  })

  it('is a no-op for unknown workspace ids', async () => {
    await performDeleteWorkspace('nope')

    expect(apiDeleteWorkspace).not.toHaveBeenCalled()
  })
})

describe('performReparentWorkspace', () => {
  it('fires the hierarchical 202 reparent threading projectId+repoId', async () => {
    vi.mocked(reparentWorkspace).mockResolvedValue(undefined)

    await performReparentWorkspace('ws-parent', 'ws-locked', 'r1')

    expect(reparentWorkspace).toHaveBeenCalledWith('p1', 'r1', 'ws-parent', 'ws-locked')
  })

  it('skips the backend call when moving to the repo root (newParentId undefined)', async () => {
    await performReparentWorkspace('ws-parent', undefined, 'r1')

    expect(reparentWorkspace).not.toHaveBeenCalled()
  })

  it('surfaces a failure via toast', async () => {
    vi.mocked(reparentWorkspace).mockRejectedValue(new Error('cycle'))

    await performReparentWorkspace('ws-parent', 'ws-locked', 'r1')

    expect(toast.error).toHaveBeenCalledWith('Failed to reparent workspace', 'cycle')
  })
})
