import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

// Pin the WebSocket endpoints the frontend opens to the real backend routes:
//   /v0/ws/git?wsId=        (not ?repo=)
//   /v0/ws/files?wsId=      (not ?workspaceId=)
// The git store dials through wsManager.subscribe; the files topic is built by
// filesWsEndpoint and dialed from the workspace effects hook.
const subscribe = vi.fn(() => () => {})
vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe,
    send: vi.fn(),
  },
}))

beforeEach(() => subscribe.mockClear())
afterEach(() => vi.resetModules())

describe('WebSocket endpoint contract', () => {
  test('git store subscribes to /v0/ws/git?wsId=', async () => {
    const { useGitStore } = await import('@/features/git/stores/git-store')
    useGitStore.getState().startGitSync('ws-123')
    expect(subscribe).toHaveBeenCalledWith('/v0/ws/git?wsId=ws-123', expect.any(Function))
  })

  test('files topic builder targets /v0/ws/files?wsId=', async () => {
    const { filesWsEndpoint } = await import('@/features/files/lib/file-tree-api')
    expect(filesWsEndpoint('ws-456')).toBe('/v0/ws/files?wsId=ws-456')
  })
})
