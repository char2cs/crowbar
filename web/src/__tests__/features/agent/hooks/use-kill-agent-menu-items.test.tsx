import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'

vi.mock('@/features/agent/lib/kill-agent', () => ({ confirmAndKillAgent: vi.fn() }))

import { useKillAgentMenuItems } from '@/features/agent/hooks/use-kill-agent-menu-items'
import { confirmAndKillAgent } from '@/features/agent/lib/kill-agent'
import { useActiveSessionsStore } from '@/lib/store/active-sessions'

const runThread = () =>
  useActiveSessionsStore.getState().applyFrame('repo:r1', {
    chatId: 'c1',
    kind: 'started',
    chat: { id: 'c1', workspaceId: 'ws-1', title: 'A thread', liveRunnerId: 'run-1' },
  })

beforeEach(() => {
  vi.clearAllMocks()
  useActiveSessionsStore.setState({ sessions: {}, attached: {} })
})

describe('useKillAgentMenuItems', () => {
  it('offers nothing for a chat whose agent is not running', () => {
    const { result } = renderHook(() => useKillAgentMenuItems('c1'))

    expect(result.current).toEqual([])
  })

  it('offers nothing without a chat', () => {
    runThread()

    const { result } = renderHook(() => useKillAgentMenuItems(undefined))

    expect(result.current).toEqual([])
  })

  it('kills the chat’s agent, with its viewer count, once chosen', () => {
    runThread()
    useActiveSessionsStore
      .getState()
      .applyFrame('repo:r1', { chatId: 'c1', kind: 'attached', attached: 2 })
    const { result } = renderHook(() => useKillAgentMenuItems('c1'))

    result.current.find((item) => item.id === 'kill-agent')!.onClick()

    expect(confirmAndKillAgent).toHaveBeenCalledWith(
      expect.objectContaining({ chatId: 'c1', workspaceId: 'ws-1', attached: 2 }),
    )
  })
})
