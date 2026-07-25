import { describe, it, expect, beforeEach, vi } from 'vitest'

vi.mock('@/lib/api', () => ({
  postWorkspace: vi.fn(),
  deleteWorkspace: vi.fn(),
  importBranches: vi.fn(() => Promise.resolve()),
}))

vi.mock('@/lib/api/workspace', () => ({
  reparentWorkspace: vi.fn(),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn() },
}))

import { postWorkspace, deleteWorkspace as apiDeleteWorkspace, importBranches } from '@/lib/api'
import { toast } from '@/features/window/stores/toast-store'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import {
  performCreateWorkspace,
  performDeleteWorkspace,
  performImportBranches,
} from '@/components/layout/workspace-tree-actions'

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

  it('logs failure silently — no toast, no phantom node', async () => {
    vi.mocked(postWorkspace).mockRejectedValue(new Error('500 boom'))

    await performCreateWorkspace('r1', 'feat/x', 'ws-parent')

    expect(workspaceIds()).toEqual(['ws-parent', 'ws-locked'])
    expect(toast.error).not.toHaveBeenCalled()
    expect(console.error).toHaveBeenCalledWith('Failed to create workspace:', expect.any(Error))
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

  it('logs failure silently — no toast, item stays in list via WS non-arrival', async () => {
    vi.mocked(apiDeleteWorkspace).mockRejectedValue(new Error('409 conflict'))

    await performDeleteWorkspace('ws-parent')

    expect(toast.error).not.toHaveBeenCalled()
    expect(console.error).toHaveBeenCalledWith('Failed to delete workspace:', expect.any(Error))
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

describe('performImportBranches', () => {
  const hooks = () => ({ addPending: vi.fn(), setError: vi.fn(), clearPending: vi.fn() })

  const importRepo = (workspaces: Repo['workspaces']): Repo => ({
    id: 'r1',
    projectId: 'p1',
    name: 'repo',
    avatarLabel: 'R',
    avatarColor: '#fff',
    defaultWorkspaceId: 'ws-default',
    workspaces,
  })

  beforeEach(() => {
    useSidebarStore.setState({ repos: [importRepo([])] })
  })

  it('adds an optimistic spinner row per branch at the repo root and fires the batch import', () => {
    const h = hooks()
    performImportBranches('r1', ['feat/a', 'feat/b'], h)

    expect(h.addPending).toHaveBeenCalledTimes(2)
    expect(h.addPending).toHaveBeenCalledWith(expect.any(String), {
      repoId: 'r1',
      parentId: 'ws-default',
      branch: 'feat/a',
    })
    expect(h.addPending).toHaveBeenCalledWith(expect.any(String), {
      repoId: 'r1',
      parentId: 'ws-default',
      branch: 'feat/b',
    })
    expect(importBranches).toHaveBeenCalledWith('p1', 'r1', ['feat/a', 'feat/b'])
  })

  it('drops a row the instant its imported workspace lands on the WS stream (any parent)', () => {
    const h = hooks()
    performImportBranches('r1', ['feat/a'], h)
    const tempId = h.addPending.mock.calls[0][0]
    expect(h.clearPending).not.toHaveBeenCalled()

    // The daemon PR-parents feat/a under some other workspace — matched on branch.
    useSidebarStore.setState({
      repos: [
        importRepo([
          { id: 'ws-parent', branch: 'feat/base', age: 'now' },
          { id: 'ws-a', branch: 'feat/a', parentId: 'ws-parent', age: 'now' },
        ]),
      ],
    })

    expect(h.clearPending).toHaveBeenCalledWith(tempId)
  })

  it('marks every row errored when the request itself is rejected', async () => {
    vi.mocked(importBranches).mockRejectedValueOnce(new Error('403 forbidden'))
    const h = hooks()
    performImportBranches('r1', ['feat/a', 'feat/b'], h)

    await vi.waitFor(() => expect(h.setError).toHaveBeenCalledTimes(2))
    expect(h.setError).toHaveBeenCalledWith(expect.any(String), '403 forbidden')
  })

  it('errors the rows and never calls the API when the project is unknown', () => {
    useSidebarStore.setState({ repos: [] }) // r1 absent → no projectId resolves
    const h = hooks()
    performImportBranches('r1', ['feat/a'], h)

    expect(h.setError).toHaveBeenCalledWith(expect.any(String), 'Unknown project')
    expect(importBranches).not.toHaveBeenCalled()
  })

  it('is a no-op for an empty branch set', () => {
    const h = hooks()
    performImportBranches('r1', [], h)

    expect(h.addPending).not.toHaveBeenCalled()
    expect(importBranches).not.toHaveBeenCalled()
  })
})
