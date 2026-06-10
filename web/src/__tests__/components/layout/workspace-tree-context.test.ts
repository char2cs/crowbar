import { describe, it, expect, beforeEach, vi } from 'vitest'

vi.mock('@/lib/api', () => ({
  postWorkspace: vi.fn(),
  deleteWorkspace: vi.fn(),
}))

vi.mock('@/components/ui/toast', () => ({
  toast: { error: vi.fn() },
}))

import { postWorkspace, deleteWorkspace as apiDeleteWorkspace } from '@/lib/api'
import { toast } from '@/components/ui/toast'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import {
  performCreateWorkspace,
  performDeleteWorkspace,
} from '@/components/layout/workspace-tree-context'

const repo = (workspaces: Repo['workspaces']): Repo => ({
  id: 'r1', name: 'repo', avatarLabel: 'R', avatarColor: '#fff', workspaces,
})

function workspaceIds(): string[] {
  return useSidebarStore.getState().repos.flatMap(r => r.workspaces.map(w => w.id))
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.spyOn(console, 'error').mockImplementation(() => {})
  useSidebarStore.setState({
    repos: [repo([
      { id: 'ws-parent', branch: 'main', age: 'now' },
      { id: 'ws-locked', branch: 'master', status: 'locked', age: 'now' },
    ])],
  })
})

describe('performCreateWorkspace', () => {
  it('calls POST /v0/workspaces and adds the workspace with the backend id', async () => {
    vi.mocked(postWorkspace).mockResolvedValue({ id: 'real-backend-id' })

    await performCreateWorkspace('r1', 'feat/x', 'ws-parent')

    expect(postWorkspace).toHaveBeenCalledWith('r1', 'feat/x', 'ws-parent')
    const created = useSidebarStore.getState().repos[0].workspaces
      .find(w => w.id === 'real-backend-id')
    expect(created).toBeDefined()
    expect(created?.branch).toBe('feat/x')
    expect(created?.parentId).toBe('ws-parent')
  })

  it('does not add a phantom node when the API call fails', async () => {
    vi.mocked(postWorkspace).mockRejectedValue(new Error('500 boom'))

    await performCreateWorkspace('r1', 'feat/x', 'ws-parent')

    expect(workspaceIds()).toEqual(['ws-parent', 'ws-locked'])
    expect(toast.error).toHaveBeenCalledWith('Failed to create workspace', '500 boom')
  })
})

describe('performDeleteWorkspace', () => {
  it('calls DELETE /v0/workspaces/:id and removes the workspace on success', async () => {
    vi.mocked(apiDeleteWorkspace).mockResolvedValue(undefined)

    await performDeleteWorkspace('ws-parent')

    expect(apiDeleteWorkspace).toHaveBeenCalledWith('ws-parent')
    expect(workspaceIds()).toEqual(['ws-locked'])
  })

  it('does not remove the workspace when the API call fails', async () => {
    vi.mocked(apiDeleteWorkspace).mockRejectedValue(new Error('409 conflict'))

    await performDeleteWorkspace('ws-parent')

    expect(workspaceIds()).toEqual(['ws-parent', 'ws-locked'])
    expect(toast.error).toHaveBeenCalledWith('Failed to delete workspace', '409 conflict')
  })

  it('never deletes a locked workspace (no API call, store untouched)', async () => {
    await performDeleteWorkspace('ws-locked')

    expect(apiDeleteWorkspace).not.toHaveBeenCalled()
    expect(workspaceIds()).toEqual(['ws-parent', 'ws-locked'])
  })

  it('is a no-op for unknown workspace ids', async () => {
    await performDeleteWorkspace('nope')

    expect(apiDeleteWorkspace).not.toHaveBeenCalled()
    expect(workspaceIds()).toEqual(['ws-parent', 'ws-locked'])
  })
})
