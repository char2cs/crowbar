import { renderHook, act, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { IdentityDTO } from '@/features/git/api/identity-api'

const mocks = vi.hoisted(() => ({
  getIdentity: vi.fn<() => Promise<IdentityDTO>>(),
}))
vi.mock('@/features/git/api/identity-api', () => ({
  getIdentity: mocks.getIdentity,
}))

import { useCurrentIdentity } from '@/features/git/hooks/use-current-identity'
import {
  __resetWorkspaceScopesForTest,
  recordWorkspaceScope,
  setWorkspaceScope,
} from '@/lib/workspace-scope'

function identity(login: string): IdentityDTO {
  return { login, displayName: login, avatarUrl: `https://x/${login}.png` }
}

beforeEach(() => {
  vi.clearAllMocks()
  __resetWorkspaceScopesForTest()
})

describe('useCurrentIdentity', () => {
  it('does not fetch and stays null before an owning chat id is recorded', async () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-race-1' })

    const { result } = renderHook(() => useCurrentIdentity('ws-race-1'))
    await act(async () => {})

    expect(result.current).toBeNull()
    expect(mocks.getIdentity).not.toHaveBeenCalled()
  })

  it('fetches identity when the scope already carries an owning chat id', async () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-ready-1', owningChatId: 'chat-1' })
    mocks.getIdentity.mockResolvedValue(identity('octocat'))

    const { result } = renderHook(() => useCurrentIdentity('ws-ready-1'))

    await waitFor(() => expect(result.current).toEqual(identity('octocat')))
    expect(mocks.getIdentity).toHaveBeenCalledWith('ws-ready-1')
  })

  // Regression: the route records a workspace's scope with NO chat id; only
  // the sidebar's own async chat-list fetch later attaches owningChatId.
  // identityBaseForWorkspace throws without one; that throw is caught inside
  // resolveIdentity (.catch(() => null)) so it never crashes, but the hook's
  // effect used to fire once per wsId and never re-run — identity stayed null
  // forever once raced, even after the id arrived moments later.
  it('retries and resolves identity once the owning chat id arrives after mount', async () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-race-2' })
    mocks.getIdentity.mockResolvedValue(identity('mona'))

    const { result } = renderHook(() => useCurrentIdentity('ws-race-2'))
    await act(async () => {})
    expect(result.current).toBeNull()
    expect(mocks.getIdentity).not.toHaveBeenCalled()

    act(() => {
      recordWorkspaceScope({
        projectId: 'p1',
        repoId: 'r1',
        wsId: 'ws-race-2',
        owningChatId: 'chat-2',
      })
    })

    await waitFor(() => expect(result.current).toEqual(identity('mona')))
    expect(mocks.getIdentity).toHaveBeenCalledWith('ws-race-2')
  })

  it('keeps returning null on a genuine fetch failure once scope is ready', async () => {
    setWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-error-1',
      owningChatId: 'chat-err',
    })
    mocks.getIdentity.mockRejectedValue(new Error('502'))

    const { result } = renderHook(() => useCurrentIdentity('ws-error-1'))
    await act(async () => {})

    expect(result.current).toBeNull()
    expect(mocks.getIdentity).toHaveBeenCalledTimes(1)
  })
})
