import { beforeEach, describe, expect, test, vi } from 'vitest'

// §3: pin the WebSocket endpoints the frontend opens to the real hierarchical
// backend routes (the old flat /v0/ws/git?wsId= and /v0/ws/files?wsId= were
// folded into co-located .../git/status and .../files/ws leaves). The git store
// dials through wsManager.subscribe; the files topic is built by filesWsEndpoint
// and dialed from the workspace effects hook. Both resolve project/repo from the
// route-recorded workspace scope.
const subscribe = vi.fn(() => () => {})
vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe,
    send: vi.fn(),
  },
}))

// The scope lives in the dependency-free @/lib/workspace-scope module — record
// it there so the URL builders resolve the project/repo without importing the
// heavy registry.
import { setWorkspaceScope } from '@/lib/workspace-scope'

beforeEach(() => {
  subscribe.mockClear()
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-123' })
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-456' })
})

describe('WebSocket endpoint contract', () => {
  test('git store subscribes to the hierarchical .../git/status WS', async () => {
    const { useGitStore } = await import('@/features/git/stores/git-store')
    useGitStore.getState().startGitSync('ws-123')
    expect(subscribe).toHaveBeenCalledWith(
      '/v0/projects/p1/repos/r1/workspaces/ws-123/git/status',
      expect.any(Function),
    )
  })

  test('files topic builder targets the hierarchical .../files/ws', async () => {
    const { filesWsEndpoint } = await import('@/features/files/lib/file-tree-api')
    expect(filesWsEndpoint('ws-456')).toBe('/v0/projects/p1/repos/r1/workspaces/ws-456/files/ws')
  })
})
