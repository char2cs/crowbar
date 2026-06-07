import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

// Pin the WebSocket endpoints the frontend opens to the real backend routes:
//   /v0/ws/git?wsId=        (not ?repo=)
//   /v0/ws/files?wsId=      (not ?workspaceId=)
// wsManager.subscribe is the single sink for every channel, so spying on it
// captures the exact path the store would dial.
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

  test('file-tree store subscribes to /v0/ws/files?wsId=', async () => {
    const { useFileTreeStore } = await import('@/features/files/stores/file-tree-store')
    useFileTreeStore.getState().startSync('ws-456')
    expect(subscribe).toHaveBeenCalledWith('/v0/ws/files?wsId=ws-456', expect.any(Function))
  })
})
