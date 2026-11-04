import { beforeEach, describe, expect, it } from 'vitest'
import { renderHook } from '@testing-library/react'
import {
  liveSessionsFromRows,
  useActiveSessionList,
  useActiveSessionsStore,
} from '@/lib/store/active-sessions'

const store = () => useActiveSessionsStore.getState()
const list = () => renderHook(() => useActiveSessionList()).result.current

const live = (id: string, title = id) => ({
  chatId: id,
  kind: 'started',
  chat: { id, workspaceId: 'ws-1', title, liveRunnerId: 'run-1' },
})

beforeEach(() => {
  useActiveSessionsStore.setState({ sessions: {}, attached: {} })
})

describe('active sessions store', () => {
  it('lists a chat whose snapshot frame names a live runner', () => {
    store().applyFrame('repo:r1', live('c1', 'Fix login'))

    expect(list()).toEqual([
      { chatId: 'c1', workspaceId: 'ws-1', title: 'Fix login', scope: 'repo:r1', attached: 0 },
    ])
  })

  it('drops the chat when a later snapshot says the runner is gone', () => {
    store().applyFrame('repo:r1', live('c1'))
    store().applyFrame('repo:r1', {
      chatId: 'c1',
      kind: 'exited',
      chat: { id: 'c1', workspaceId: 'ws-1', title: 'c1', liveRunnerId: '' },
    })

    expect(list()).toEqual([])
  })

  it('drops a deleted chat', () => {
    store().applyFrame('repo:r1', live('c1'))
    store().applyFrame('repo:r1', { chatId: 'c1', kind: 'deleted' })

    expect(list()).toEqual([])
  })

  it('reports the viewer count, and zero for a running chat nobody is attached to', () => {
    store().applyFrame('repo:r1', live('c1'))
    store().applyFrame('repo:r1', live('c2'))
    store().applyFrame('repo:r1', { chatId: 'c1', kind: 'attached', attached: 2 })

    const counts = list().map((s) => [s.chatId, s.attached])
    expect(counts).toEqual([
      ['c1', 2],
      ['c2', 0],
    ])
  })

  it('forgets a count that fell back to zero', () => {
    store().applyFrame('repo:r1', { chatId: 'c1', kind: 'attached', attached: 1 })
    store().applyFrame('repo:r1', { chatId: 'c1', kind: 'attached', attached: 0 })

    expect(useActiveSessionsStore.getState().attached).toEqual({})
  })

  it('a seed replaces only its own scope', () => {
    store().applyFrame('repo:r1', live('c1'))
    store().applyFrame('repo:r2', live('c2'))

    store().seedScope('repo:r1', [])

    expect(list().map((s) => s.chatId)).toEqual(['c2'])
  })

  it('a reconnect forgets the scope’s counts, which the new socket restates', () => {
    store().applyFrame('repo:r1', { chatId: 'c1', kind: 'attached', attached: 3 })
    store().applyFrame('repo:r2', { chatId: 'c2', kind: 'attached', attached: 1 })

    store().resetAttached('repo:r1')

    expect(useActiveSessionsStore.getState().attached).toEqual({
      c2: { scope: 'repo:r2', count: 1 },
    })
  })

  it('clearing a scope removes its sessions and counts', () => {
    store().applyFrame('repo:r1', live('c1'))
    store().applyFrame('repo:r1', { chatId: 'c1', kind: 'attached', attached: 1 })

    store().clearScope('repo:r1')

    expect(useActiveSessionsStore.getState()).toMatchObject({ sessions: {}, attached: {} })
  })

  it('reads the live rows of a chat list', () => {
    expect(
      liveSessionsFromRows('repo:r1', [
        { id: 'c1', workspaceId: 'ws-1', title: 'A', liveRunnerId: 'run' },
        { id: 'c2', workspaceId: 'ws-1', title: 'B', liveRunnerId: '' },
        { id: 'c3', workspaceId: 'ws-1', title: 'C' },
      ]),
    ).toEqual([{ chatId: 'c1', workspaceId: 'ws-1', title: 'A', scope: 'repo:r1' }])
  })
})
