import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { entityStore } from '@/core/entities/entity-store'
import { useActiveSessionList, viewersLabel } from '@/core/entities/active-sessions'
import { streamDaemon, streamDaemonFor } from '@/__tests__/__fixtures__/stream-daemon'
import { chatEntity, resetEntities } from '@/__tests__/__fixtures__/sidebar-entities'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

let v = 0
const daemon = (hostId: string) => (hostId === 'local' ? streamDaemon : streamDaemonFor(hostId))
const chat = (hostId: string, id: string, liveRunnerId: string, title = id) =>
  daemon(hostId).deliver({
    t: 'entity',
    scope: 'projects',
    kind: 'chat',
    id,
    v: ++v,
    data: chatEntity({ id, title, liveRunnerId, workspaceId: `ws-${id}` }),
  })
const viewers = (hostId: string, id: string, n: number) =>
  daemon(hostId).deliver(
    n === 0
      ? { t: 'entity', scope: 'projects', kind: 'presence', id, v: ++v, gone: true }
      : { t: 'entity', scope: 'projects', kind: 'presence', id, v: ++v, data: { viewers: n } },
  )
const sessions = () => renderHook(() => useActiveSessionList()).result.current

beforeEach(() => {
  resetHostWorld()
  resetEntities()
})
afterEach(() => resetHostWorld())

describe('the active sessions', () => {
  it('lists a chat with a live runner and two viewers with 2', () => {
    chat('local', 'c1', 'run-1')
    viewers('local', 'c1', 2)
    expect(sessions()).toEqual([
      { hostId: 'local', chatId: 'c1', workspaceId: 'ws-c1', title: 'c1', viewers: 2 },
    ])
  })

  it('lists a running chat nobody holds with 0', () => {
    chat('local', 'c1', 'run-1')
    expect(sessions().map((s) => s.viewers)).toEqual([0])
  })

  it('leaves out a viewed chat with no runner', () => {
    chat('local', 'c1', '')
    viewers('local', 'c1', 1)
    expect(sessions()).toEqual([])
  })

  it("lists host-1's running chat beside local's", async () => {
    pairRemoteHost('host-1')
    await act(async () => {})
    chat('local', 'c-l', 'run-l', 'Alpha')
    act(() => {
      chat('host-1', 'c-r', 'run-r', 'Beta')
      viewers('host-1', 'c-r', 1)
      streamDaemonFor('host-1').deliver({ t: 'synced', scope: 'projects' })
    })
    expect(sessions().map((s) => [s.hostId, s.chatId, s.viewers])).toEqual([
      ['local', 'c-l', 0],
      ['host-1', 'c-r', 1],
    ])
  })

  it('a presence gone drops the count, not the session', () => {
    chat('local', 'c1', 'run-1')
    viewers('local', 'c1', 1)
    viewers('local', 'c1', 0)
    expect(sessions().map((s) => [s.chatId, s.viewers])).toEqual([['c1', 0]])
  })

  it('drops a chat whose runner is gone, and a deleted chat', () => {
    chat('local', 'c1', 'run-1')
    chat('local', 'c2', 'run-2')
    chat('local', 'c1', '')
    streamDaemon.deliver({
      t: 'entity',
      scope: 'projects',
      kind: 'chat',
      id: 'c2',
      v: ++v,
      gone: true,
    })
    expect(sessions()).toEqual([])
  })

  it("drops a removed host's sessions with its rows", async () => {
    pairRemoteHost('host-1')
    await act(async () => {})
    act(() => {
      chat('host-1', 'c-r', 'run-r')
      streamDaemonFor('host-1').deliver({ t: 'synced', scope: 'projects' })
    })
    expect(sessions()).toHaveLength(1)
    act(() => useHostConfigStore.getState().removeHost('host-1'))
    expect(sessions()).toEqual([])
  })

  it('says how many view it', () => {
    expect([0, 1, 2].map(viewersLabel)).toEqual(['No viewers', '1 viewer', '2 viewers'])
  })

  it('answers the same array until a chat or a presence row changes', () => {
    chat('local', 'c1', 'run-1')
    const first = sessions()
    entityStore.setState({ epochs: { local: 'other' } })
    expect(sessions()).toBe(first)
  })
})
