import { beforeEach, describe, expect, test, vi } from 'vitest'

// Pin the WebSocket endpoints the frontend opens to the real backend routes.
// The two topics below are deliberately shaped DIFFERENTLY, and that is the
// contract: git is addressed by the chat holding the worktree (the flat
// /v0/chats/:chatId prefix), while files is still workspace-scoped and
// hierarchical until its own step moves it. The git store dials through
// wsManager.subscribe; the files topic is built by filesWsEndpoint and dialed
// from the workspace effects hook. Both resolve what they need from the
// route-recorded workspace scope — the project/repo for files, the owning chat
// for git.
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
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-123', owningChatId: 'chat-123' })
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-456' })
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

  test('files topic builder targets the hierarchical .../files/ws', async () => {
    const { filesWsEndpoint } = await import('@/features/files/lib/file-tree-api')
    expect(filesWsEndpoint('ws-456')).toBe('/v0/projects/p1/repos/r1/workspaces/ws-456/files/ws')
  })
})
