import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import {
  getHomeOwningChatId,
  getHomeWorkspaceId,
  getKnownHomeWorkspaceIds,
  useHomeWorkspaceState,
  useHomeWorkspaceStates,
} from '@/features/workspace/lib/home-workspace-resolver'
import {
  projectDTO,
  resetEntities,
  seedHome,
  seedProjects,
} from '@/__tests__/__fixtures__/sidebar-entities'

beforeEach(() => resetEntities())

describe('the home workspace resolver', () => {
  it('is unresolved, not missing, before the projects scope answers', () => {
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))
    expect(result.current).toEqual({
      wsId: null,
      owningChatId: null,
      localPath: null,
      error: false,
    })
  })

  it('names the home the projects scope carries, with its owner and directory', () => {
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))

    act(() => seedHome('p1', 'ws-home', { owningChatId: 'chat-home', localPath: '/p1' }))

    expect(result.current).toEqual({
      wsId: 'ws-home',
      owningChatId: 'chat-home',
      localPath: '/p1',
      error: false,
    })
    expect(getHomeWorkspaceId('p1')).toBe('ws-home')
    expect(getHomeOwningChatId('p1')).toBe('chat-home')
  })

  it('reports a synced project without a home as missing', () => {
    seedProjects([projectDTO({ id: 'p1' })])
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))
    expect(result.current.error).toBe(true)
    expect(result.current.wsId).toBeNull()
  })

  it('keeps the same state object while the home does not change', () => {
    seedHome('p1', 'ws-home')
    const { result } = renderHook(() => useHomeWorkspaceState('p1'))
    const first = result.current
    act(() => seedProjects([projectDTO({ id: 'p2' })]))
    expect(result.current).toBe(first)
  })

  it('knows every project home at once', () => {
    seedHome('p1', 'ws-p1')
    seedHome('p2', 'ws-p2')
    const { result } = renderHook(() => useHomeWorkspaceStates())
    expect([...result.current.keys()].sort()).toEqual(['p1', 'p2'])
    expect(getKnownHomeWorkspaceIds().sort()).toEqual(['ws-p1', 'ws-p2'])
  })
})
