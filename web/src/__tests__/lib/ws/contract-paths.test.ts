import { beforeEach, describe, expect, test, vi } from 'vitest'

// Pin the WebSocket endpoints the frontend opens to the real backend routes.
// Both topics below are now named the same way, and that is the contract: a
// live stream over a worktree is addressed by a CHAT holding it (the flat
// /v0/chats/:chatId prefix), never by the workspace. The git store dials
// through wsManager.subscribe; the files topic is built by filesWsEndpoint and
// dialed from the workspace effects hook. Both resolve the owning chat from the
// route-recorded workspace scope.
const subscribe = vi.fn(() => () => {})
vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe,
    send: vi.fn(),
  },
}))

// The scope lives in the dependency-free @/lib/workspace-scope module — record
// it there so the URL builders resolve the owning chat without importing the
// heavy registry.
import { setWorkspaceScope } from '@/lib/workspace-scope'

beforeEach(() => {
  subscribe.mockClear()
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-123', owningChatId: 'chat-123' })
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-456', owningChatId: 'chat-456' })
})

describe('WebSocket endpoint contract', () => {
  // git has moved to the flat chat prefix: the stream is named by the chat
  // holding the worktree, never by the workspace. Every chat sharing that
  // worktree subscribes to its own URL and the daemon fans one push out to all
  // of them.
  test('git store subscribes to the chat-scoped .../git/status WS', async () => {
    const { useGitStore } = await import('@/features/git/stores/git-store')
    useGitStore.getState().startGitSync('ws-123')
    expect(subscribe).toHaveBeenCalledWith('/v0/chats/chat-123/git/status', expect.any(Function))
  })

  // files completes the shared bucket's move. Sibling chats over one worktree
  // each dial their own URL and the daemon fans one file-change push out to all
  // of them — the same shape git already has above.
  test('files topic builder targets the chat-scoped .../files/ws', async () => {
    const { filesWsEndpoint } = await import('@/features/files/lib/file-tree-api')
    expect(filesWsEndpoint('ws-456')).toBe('/v0/chats/chat-456/files/ws')
  })
})
