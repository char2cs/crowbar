import { beforeEach, describe, expect, it, vi } from 'vitest'
import { dataOf } from '@/lib/loadable'

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/api', () => ({ apiFetch }))

import {
  blameKey,
  clearBlame,
  loadBlame,
  useGitBlameStore,
} from '@/features/git/stores/git-blame-store'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'

const ENTRY = {
  lineNumber: 1,
  commitHash: 'abc123',
  author: 'Ada',
  email: 'ada@example.com',
  date: '2026-09-01T10:00:00Z',
  commitMessage: 'initial',
}

beforeEach(() => {
  apiFetch.mockReset()
  apiFetch.mockResolvedValue([ENTRY])
  useGitBlameStore.setState({ blame: {} })
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'ws-1', owningChatId: 'chat-1' })
  recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'ws-2', owningChatId: 'chat-2' })
})

describe('git-blame-store', () => {
  it("loads a file's blame from its workspace's daemon route", async () => {
    await loadBlame('ws-1', 'src/a b.ts')

    expect(apiFetch).toHaveBeenCalledWith('/v0/chats/chat-1/blame?path=src%2Fa%20b.ts', {
      signal: undefined,
    })
    expect(dataOf(useGitBlameStore.getState().blame[blameKey('ws-1', 'src/a b.ts')])).toEqual([
      ENTRY,
    ])
  })

  it('loads once until invalidated', async () => {
    await loadBlame('ws-1', 'a.ts')
    await loadBlame('ws-1', 'a.ts')
    expect(apiFetch).toHaveBeenCalledTimes(1)

    clearBlame('ws-1', 'a.ts')
    await loadBlame('ws-1', 'a.ts')
    expect(apiFetch).toHaveBeenCalledTimes(2)
  })

  it('keys by workspace: sibling worktrees share relative paths', async () => {
    await loadBlame('ws-1', 'a.ts')
    await loadBlame('ws-2', 'a.ts')
    clearBlame('ws-1', 'a.ts')

    const { blame } = useGitBlameStore.getState()
    expect(blame[blameKey('ws-1', 'a.ts')]).toBeUndefined()
    expect(blame[blameKey('ws-2', 'a.ts')]?.status).toBe('success')
  })

  it('drops an answer that arrives after the file was invalidated', async () => {
    let finish: (v: unknown) => void = () => {}
    apiFetch.mockImplementationOnce(() => new Promise((r) => (finish = r)))

    const pending = loadBlame('ws-1', 'a.ts')
    clearBlame('ws-1', 'a.ts')
    finish([ENTRY])
    await pending

    expect(useGitBlameStore.getState().blame[blameKey('ws-1', 'a.ts')]).toBeUndefined()
  })

  it('records a failure without throwing', async () => {
    apiFetch.mockRejectedValueOnce(new Error('not a git repository'))
    await loadBlame('ws-1', 'a.ts')
    expect(useGitBlameStore.getState().blame[blameKey('ws-1', 'a.ts')]?.status).toBe('error')
  })
})
